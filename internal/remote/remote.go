// Package remote centralizes GitHub raw content URLs for the project.
//
// Owner and repo are determined lazily on first access. Values set at build
// time via ldflags take precedence; otherwise the package derives them from
// the local git remote origin.
package remote

import (
	"context"
	"log/slog"
	"os/exec"
	"regexp"
	"sync"
	"time"
)

// Set at build time via:
//
//	-X tools.zach/dev/agentcord/internal/remote.ldOwner=...
//	-X tools.zach/dev/agentcord/internal/remote.ldRepo=...
var (
	ldOwner string
	ldRepo  string
)

var (
	initOnce sync.Once
	owner    string
	repo     string
)

// githubRemoteRe extracts owner and repo from GitHub remote URLs.
// Matches both HTTPS (github.com/) and SSH (github.com:) formats.
var githubRemoteRe = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/.]+)`)

// ensureInit lazily resolves owner and repo on first call. Build-time ldflags
// are preferred; otherwise the values are derived from the local git remote origin.
func ensureInit() {
	initOnce.Do(func() {
		if ldOwner != "" && ldRepo != "" {
			owner = ldOwner
			repo = ldRepo
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "git", "remote", "get-url", "origin").Output()
		if err != nil {
			slog.Debug("remote: ldflags not set and git remote unavailable", "error", err)
			return
		}
		m := githubRemoteRe.FindStringSubmatch(string(out))
		if len(m) == 3 {
			owner = m[1]
			repo = m[2]
		}
	})
}

// Owner returns the GitHub repository owner.
func Owner() string {
	ensureInit()
	return owner
}

// Repo returns the GitHub repository name.
func Repo() string {
	ensureInit()
	return repo
}

// RawURL returns the raw GitHub URL for a file on the main branch.
// Returns empty string if owner/repo could not be determined.
func RawURL(path string) string {
	ensureInit()
	if owner == "" || repo == "" {
		return ""
	}
	return "https://raw.githubusercontent.com/" + owner + "/" + repo + "/main/" + path
}

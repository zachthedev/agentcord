// Package main prints a SemVer-style build version string for use in ldflags.
// Cross-platform replacement for the Unix-only git describe + date pipeline.
//
// Output format depends on git state:
//
//	No tags, clean:     0.0.0-dev+05ffee5
//	No tags, dirty:     0.0.0-dev+05ffee5.dirty
//	On tag v0.1.0:      0.1.0
//	Dirty tag:          0.1.0-dirty
//	3 past v0.1.0:      0.1.0-dev.3+g1234567
//	Same but dirty:     0.1.0-dev.3+g1234567.dirty
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	fmt.Print(buildVersion())
}

// buildVersion assembles a SemVer build version string by querying git state.
// It first attempts git describe against v-prefixed tags; if no tags exist it
// falls back to a 0.0.0-dev+<hash> format using [baseVersion] as the prefix.
func buildVersion() string {
	base := baseVersion()

	// Try git describe with version tags
	if out, err := exec.Command("git", "describe", "--tags", "--match", "v*", "--dirty").Output(); err == nil {
		return formatTaggedVersion(strings.TrimSpace(string(out)))
	}

	// No version tags — build from commit hash
	out, err := exec.Command("git", "rev-parse", "--short=7", "HEAD").Output()
	if err != nil {
		return base + "-dev"
	}
	hash := strings.TrimSpace(string(out))

	if isDirty() {
		return fmt.Sprintf("%s-dev+%s.dirty", base, hash)
	}
	return fmt.Sprintf("%s-dev+%s", base, hash)
}

// formatTaggedVersion converts a git describe output (e.g. "v0.1.0-3-g1234567-dirty")
// into a clean SemVer string with optional dev/dirty suffixes. The "-dirty" flag
// and "v" prefix are stripped, and the <N>-g<hash> portion is reformatted as
// "-dev.<N>+g<hash>".
func formatTaggedVersion(desc string) string {
	dirty := strings.HasSuffix(desc, "-dirty")
	clean := strings.TrimSuffix(desc, "-dirty")
	clean = strings.TrimPrefix(clean, "v")

	// Try to find -N-gHASH suffix (commits past tag).
	// git describe format: <tag>-<N>-g<abbreviated-hash>
	lastDash := strings.LastIndex(clean, "-")
	if lastDash > 0 {
		hash := clean[lastDash+1:]
		rest := clean[:lastDash]
		secondLastDash := strings.LastIndex(rest, "-")
		if secondLastDash > 0 && strings.HasPrefix(hash, "g") {
			n := rest[secondLastDash+1:]
			tag := rest[:secondLastDash]
			meta := hash
			if dirty {
				meta += ".dirty"
			}
			return fmt.Sprintf("%s-dev.%s+%s", tag, n, meta)
		}
	}

	// Exact tag
	if dirty {
		return clean + "-dirty"
	}
	return clean
}

// isDirty reports whether the git working tree has uncommitted changes
// by checking the output of git status --porcelain.
func isDirty() bool {
	out, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// baseVersion reads the root version from .release-manifest.json (key ".").
// It returns "0.0.0" if the file is missing, malformed, or lacks a root entry.
func baseVersion() string {
	data, err := os.ReadFile(".release-manifest.json")
	if err != nil {
		return "0.0.0"
	}
	var manifest map[string]string
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "0.0.0"
	}
	if v, ok := manifest["."]; ok && v != "" {
		return v
	}
	return "0.0.0"
}

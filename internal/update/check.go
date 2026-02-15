// Package update checks for newer versions of Agentcord via the release manifest.
package update

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"tools.zach/dev/agentcord/internal/paths"
	"tools.zach/dev/agentcord/internal/remote"
)

var (
	manifestURL     string
	manifestURLOnce sync.Once
)

func getManifestURL() string {
	manifestURLOnce.Do(func() { manifestURL = remote.RawURL(paths.ReleaseManifest) })
	return manifestURL
}

// ///////////////////////////////////////////////
// Public API
// ///////////////////////////////////////////////

// Check fetches the remote release manifest and logs if a newer version is available.
// Non-blocking, non-fatal — failures are silently ignored.
func Check(current string) {
	if getManifestURL() == "" {
		slog.Debug("skipping version check: no remote URL configured")
		return
	}
	remoteVer, err := fetchLatest()
	if err != nil {
		slog.Debug("version check failed", "error", err)
		return
	}
	if remoteVer == "" || remoteVer == current {
		return
	}
	if semverLess(current, remoteVer) {
		slog.Info("new version available", "current", current, "latest", remoteVer)
	}
}

// ///////////////////////////////////////////////
// Internal helpers
// ///////////////////////////////////////////////

// fetchLatest downloads the release manifest from GitHub and returns the version
// string stored under the "." key, which represents the latest stable release.
func fetchLatest() (string, error) {
	url := getManifestURL()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	var manifest map[string]string
	if err := json.Unmarshal(body, &manifest); err != nil {
		return "", fmt.Errorf("parsing manifest: %w", err)
	}
	return manifest["."], nil
}

// semverLess returns true if a < b using simple numeric comparison.
// Handles versions like "0.1.0", "1.2.3". Non-semver strings are not compared.
// Per semver, a pre-release version is less than the same version without one
// (e.g., "0.1.0-dev" < "0.1.0").
func semverLess(a, b string) bool {
	pa := parseSemver(a)
	pb := parseSemver(b)
	if pa == nil || pb == nil {
		return false
	}
	for i := range 3 {
		if pa[i] < pb[i] {
			return true
		}
		if pa[i] > pb[i] {
			return false
		}
	}
	// Numeric parts are equal; a pre-release version is less than a release.
	aPre := hasPreRelease(a)
	bPre := hasPreRelease(b)
	if aPre && !bPre {
		return true
	}
	return false
}

// hasPreRelease reports whether a version string contains a pre-release suffix
// (e.g., "0.1.0-dev" or "v1.0.0-beta+build").
func hasPreRelease(s string) bool {
	s = strings.TrimPrefix(s, "v")
	return strings.ContainsAny(s, "-")
}

// parseSemver splits a version string like "v1.2.3" or "0.1.0-dev" into a
// three-element int slice [major, minor, patch]. Pre-release suffixes after
// "-" or "+" are stripped. Returns nil if the string is not valid semver.
func parseSemver(s string) []int {
	s = strings.TrimPrefix(s, "v")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return nil
	}
	result := make([]int, 3)
	for i, p := range parts {
		// Strip pre-release suffixes (e.g., "0-dev+abc")
		if idx := strings.IndexAny(p, "-+"); idx >= 0 {
			p = p[:idx]
		}
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				return nil
			}
			n = n*10 + int(c-'0')
		}
		result[i] = n
	}
	return result
}

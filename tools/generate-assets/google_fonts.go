// google_fonts.go downloads font files from the Google Fonts CSS API.
//
// Font specs use the format "google:FAMILY:WEIGHT" (e.g. "google:Inter:800").
// Downloaded fonts are cached locally so they aren't re-fetched on every run.

package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tdewolff/font"
)

// fontURLRe extracts the font file URL from the CSS response.
// Matches: url(https://fonts.gstatic.com/s/inter/v18/xxx.woff2)
var fontURLRe = regexp.MustCompile(`url\((https://fonts\.gstatic\.com/[^)]+)\)`)

// ParseGoogleFontSpec parses a "google:Family:Weight" spec into its parts.
// Returns family, weight, and whether the spec is valid.
func ParseGoogleFontSpec(spec string) (family, weight string, ok bool) {
	parts := strings.SplitN(spec, ":", 3)
	if len(parts) != 3 || parts[0] != "google" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

// FetchGoogleFont downloads a font from Google Fonts, caching the result.
// The cacheDir is created if it doesn't exist. Returns the raw font bytes
// in SFNT (TTF/OTF) format, converting from WOFF2 if necessary.
func FetchGoogleFont(spec, cacheDir string) ([]byte, error) {
	family, weight, ok := ParseGoogleFontSpec(spec)
	if !ok {
		return nil, fmt.Errorf("invalid google font spec %q: expected google:FAMILY:WEIGHT", spec)
	}

	// Check cache first
	cacheFile := filepath.Join(cacheDir, fmt.Sprintf("%s-%s.ttf", family, weight))
	if data, err := os.ReadFile(cacheFile); err == nil {
		return data, nil
	}

	// Build CSS API URL
	cssURL := fmt.Sprintf("https://fonts.googleapis.com/css2?family=%s:wght@%s",
		url.QueryEscape(family), weight)

	// Fetch CSS — Google returns WOFF2 URLs for modern User-Agents
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", cssURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	// Modern UA to get WOFF2 (we have a converter)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching CSS from Google Fonts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google Fonts CSS API returned status %d for %s wght@%s", resp.StatusCode, family, weight)
	}

	cssBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading CSS response: %w", err)
	}

	// Extract font file URL from CSS
	matches := fontURLRe.FindSubmatch(cssBody)
	if matches == nil {
		return nil, fmt.Errorf("no font URL found in Google Fonts CSS response for %s wght@%s", family, weight)
	}
	fontURL := string(matches[1])

	// Download the font file
	fontResp, err := client.Get(fontURL)
	if err != nil {
		return nil, fmt.Errorf("downloading font file: %w", err)
	}
	defer fontResp.Body.Close()

	if fontResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("font file download returned status %d", fontResp.StatusCode)
	}

	fontData, err := io.ReadAll(io.LimitReader(fontResp.Body, 10<<20)) // 10 MiB limit
	if err != nil {
		return nil, fmt.Errorf("reading font file: %w", err)
	}

	// Convert WOFF2 to SFNT if needed
	if isWOFF2Data(fontURL, fontData) {
		sfnt, err := font.ToSFNT(fontData)
		if err != nil {
			return nil, fmt.Errorf("converting WOFF2 to SFNT: %w", err)
		}
		fontData = sfnt
	}

	// Cache the converted font
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating font cache dir: %w", err)
	}
	if err := os.WriteFile(cacheFile, fontData, 0o644); err != nil {
		// Non-fatal: log but return the font data anyway
		fmt.Fprintf(os.Stderr, "  warning: failed to cache font: %v\n", err)
	}

	return fontData, nil
}

// isWOFF2Data checks whether font data is WOFF2 by URL extension or magic bytes.
func isWOFF2Data(url string, data []byte) bool {
	if strings.HasSuffix(strings.ToLower(url), ".woff2") {
		return true
	}
	if len(data) >= 4 && data[0] == 'w' && data[1] == 'O' && data[2] == 'F' && data[3] == '2' {
		return true
	}
	return false
}

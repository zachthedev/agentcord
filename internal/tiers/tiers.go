// Package tiers provides model tier configuration fetched from a remote source.
//
// Tiers determine which Discord Rich Presence asset is shown for a given model.
// Data is fetched with double fallback: remote GitHub -> local cache.
//
// Tiers are organized per-client: each client tool (claude-code, cursor, etc.)
// has its own set of tier names and styling. Each tier carries visual config
// (bg_color, fg_color, size, font_size) used by the gen-assets tool to render
// Discord presence icons. Styling is resolved with three layers of inheritance:
// global defaults -> client defaults -> tier overrides.
//
// # HOW TO ADD A NEW MODEL TIER
//
// When a provider releases a new model tier (e.g., "claude-nova-5-0"):
//  1. Add a "nova" entry to the appropriate client's tiers map in data/tiers.json
//     Use {} to inherit all defaults, or override specific fields.
//  2. Run `make gen-assets` to generate nova.png
//  3. Upload nova.png to Discord Developer Portal -> Rich Presence -> Art Assets
//  4. Push to main — all running daemons pick up the change on next restart
//  5. No binary release needed
package tiers

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tools.zach/dev/agentcord/internal/atomicfile"
	"tools.zach/dev/agentcord/internal/paths"
	"tools.zach/dev/agentcord/internal/remote"
)

var (
	remoteURL     string
	remoteURLOnce sync.Once
)

func getRemoteURL() string {
	remoteURLOnce.Do(func() { remoteURL = remote.RawURL(paths.TiersDataPath) })
	return remoteURL
}

// ///////////////////////////////////////////////
// Types
// ///////////////////////////////////////////////

// TierConfig holds the visual styling for a single tier's Discord presence asset.
type TierConfig struct {
	// BgColor is the background hex color (e.g. "#DA7756").
	BgColor string `json:"bg_color,omitempty"`
	// FgColor is the foreground (letter) hex color (e.g. "#FFFFFF").
	FgColor string `json:"fg_color,omitempty"`
	// Size is the square image dimension in pixels.
	Size int `json:"size,omitempty"`
	// FontSize is the font size in points at 72 DPI.
	FontSize int `json:"font_size,omitempty"`
}

// ClientTierConfig holds the tier configuration for a single client tool.
type ClientTierConfig struct {
	// DefaultIcon is the Discord asset key used for unrecognized models within
	// this client. If empty, falls back to the global TierData.DefaultIcon.
	DefaultIcon string `json:"default_icon,omitempty"`
	// Font is the local font file path relative to the repo root.
	Font string `json:"font,omitempty"`
	// FontFallback is a Google Fonts spec (e.g. "google:Inter:800") used when
	// the local font file is not found.
	FontFallback string `json:"font_fallback,omitempty"`
	// Defaults provides client-level styling inherited by all tiers in this client.
	// Overrides global defaults; overridden by per-tier settings.
	Defaults TierConfig `json:"defaults"`
	// Tiers maps tier names (e.g. "opus") to their styling overrides.
	Tiers map[string]TierConfig `json:"tiers"`
}

// TierData holds the model tier configuration fetched from the remote source.
type TierData struct {
	// DefaultIcon is the Discord asset key used for unrecognized models.
	DefaultIcon string `json:"default_icon"`
	// Defaults provides base styling inherited by all clients and tiers.
	Defaults TierConfig `json:"defaults"`
	// Clients maps client IDs to their tier configuration.
	Clients map[string]ClientTierConfig `json:"clients"`
}

// DefaultIconForClient returns the effective default icon for a client.
// Uses the client's default_icon if set, otherwise falls back to the global DefaultIcon.
func (d *TierData) DefaultIconForClient(client string) string {
	if cc, ok := d.Clients[client]; ok && cc.DefaultIcon != "" {
		return cc.DefaultIcon
	}
	return d.DefaultIcon
}

// TierNamesForClient returns the tier names for a client as a slice.
// Returns nil if the client is not found.
func (d *TierData) TierNamesForClient(client string) []string {
	cc, ok := d.Clients[client]
	if !ok {
		return nil
	}
	names := make([]string, 0, len(cc.Tiers))
	for name := range cc.Tiers {
		names = append(names, name)
	}
	return names
}

// ResolvedTier returns the effective config for a client's tier, with all
// inheritance applied: global defaults -> client defaults -> tier overrides.
// Returns global defaults if the client or tier is not found.
func (d *TierData) ResolvedTier(client, tier string) TierConfig {
	cfg := d.Defaults
	cc, ok := d.Clients[client]
	if !ok {
		return cfg
	}
	// Apply client defaults
	mergeTierConfig(&cfg, cc.Defaults)
	// Apply tier overrides
	if tc, ok := cc.Tiers[tier]; ok {
		mergeTierConfig(&cfg, tc)
	}
	return cfg
}

// mergeTierConfig applies non-zero fields from src onto dst.
func mergeTierConfig(dst *TierConfig, src TierConfig) {
	if src.BgColor != "" {
		dst.BgColor = src.BgColor
	}
	if src.FgColor != "" {
		dst.FgColor = src.FgColor
	}
	if src.Size != 0 {
		dst.Size = src.Size
	}
	if src.FontSize != 0 {
		dst.FontSize = src.FontSize
	}
}

// ///////////////////////////////////////////////
// Public API
// ///////////////////////////////////////////////

// Fetch loads tier data: remote -> cache.
// Returns nil with an error when both sources fail.
func Fetch(dataDir string) (*TierData, error) {
	// Try remote (skip if URL not derivable)
	if getRemoteURL() == "" {
		slog.Debug("skipping remote tier fetch: no remote URL configured")
	} else if data, err := fetchRemote(); err == nil {
		cacheWrite(dataDir, data)
		return data, nil
	}
	// Try cache
	if data, err := cacheRead(dataDir); err == nil {
		slog.Debug("using cached tier data")
		return data, nil
	}
	return nil, fmt.Errorf("no tier data available: remote and cache both failed")
}

// modelPrefixes lists known model family prefixes to strip before tier matching.
var modelPrefixes = []string{"claude-", "gpt-", "gemini-", "o1-", "o3-"}

// ExtractTier returns the Discord asset key for a model ID within a client's
// tier set. Strips known family prefixes before matching: "claude-opus-4-6" -> "opus".
// Unknown models return data.DefaultIcon.
func ExtractTier(model, client string, data *TierData) string {
	fallback := data.DefaultIconForClient(client)
	cc, ok := data.Clients[client]
	if !ok {
		return fallback
	}
	stripped := model
	for _, prefix := range modelPrefixes {
		if strings.HasPrefix(model, prefix) {
			stripped = strings.TrimPrefix(model, prefix)
			break
		}
	}
	for tier := range cc.Tiers {
		if strings.HasPrefix(stripped, tier) {
			return tier
		}
	}
	return fallback
}

// ///////////////////////////////////////////////
// Internal helpers
// ///////////////////////////////////////////////

// fetchRemote downloads tier data from the remote GitHub URL. The response
// body is limited to 1 MiB to guard against unexpectedly large payloads.
func fetchRemote() (*TierData, error) {
	url := getRemoteURL()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var data TierData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return &data, nil
}

// cacheWrite persists tier data to the local cache file using [atomicfile.Write]
// so that a subsequent [Fetch] can fall back to cached data when offline.
func cacheWrite(dataDir string, data *TierData) {
	b, err := json.Marshal(data)
	if err != nil {
		slog.Debug("failed to marshal tier data for cache", "error", err)
		return
	}
	path := filepath.Join(dataDir, paths.TiersCacheFile)
	if err := atomicfile.Write(path, b, 0o644); err != nil {
		slog.Debug("failed to write tier cache", "error", err)
	}
}

// cacheRead loads previously cached tier data from dataDir. Returns an error
// if the cache file is missing or contains invalid JSON.
func cacheRead(dataDir string) (*TierData, error) {
	path := filepath.Join(dataDir, paths.TiersCacheFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading tier cache %s: %w", path, err)
	}
	var data TierData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("parsing tier cache: %w", err)
	}
	return &data, nil
}

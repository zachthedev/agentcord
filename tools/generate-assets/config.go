// config.go defines tier configuration types and JSON loading for the
// gen-assets tool. [TierData] is the top-level structure deserialized from
// data/tiers.json; [TierConfig] holds per-tier visual styling fields.

package main

import (
	"encoding/json"
	"os"
)

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
	// DefaultIcon is the Discord asset key used for unrecognized models.
	DefaultIcon string `json:"default_icon,omitempty"`
	// Font is the local font file path relative to the repo root.
	Font string `json:"font,omitempty"`
	// FontFallback is a Google Fonts spec (e.g. "google:Inter:800") used when
	// the local font file is not found.
	FontFallback string `json:"font_fallback,omitempty"`
	// Defaults provides client-level styling inherited by all tiers in this client.
	Defaults TierConfig `json:"defaults"`
	// Tiers maps tier names (e.g. "opus") to their styling overrides.
	Tiers map[string]TierConfig `json:"tiers"`
}

// TierData holds the tier configuration read from data/tiers.json.
type TierData struct {
	// DefaultIcon is the Discord asset key used for unrecognized models.
	DefaultIcon string `json:"default_icon"`
	// Defaults provides base styling inherited by all clients and tiers.
	Defaults TierConfig `json:"defaults"`
	// Clients maps client IDs to their tier configuration.
	Clients map[string]ClientTierConfig `json:"clients"`
}

// ResolvedTier returns the effective config for a client's tier, with all
// inheritance applied: global defaults -> client defaults -> tier overrides.
func (d *TierData) ResolvedTier(client, tier string) TierConfig {
	cfg := d.Defaults
	cc, ok := d.Clients[client]
	if !ok {
		return cfg
	}
	mergeTierConfig(&cfg, cc.Defaults)
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

// LoadTierData reads and parses a tiers.json file.
func LoadTierData(path string) (*TierData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var td TierData
	if err := json.Unmarshal(data, &td); err != nil {
		return nil, err
	}
	return &td, nil
}

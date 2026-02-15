package tiers

import (
	"sort"
	"testing"
)

// ///////////////////////////////////////////////
// TierData.TierNamesForClient Tests
// ///////////////////////////////////////////////

func TestTierNamesForClient(t *testing.T) {
	data := TierData{
		Clients: map[string]ClientTierConfig{
			"test-client": {
				Tiers: map[string]TierConfig{
					"opus":   {},
					"sonnet": {},
					"haiku":  {},
				},
			},
		},
	}
	names := data.TierNamesForClient("test-client")
	sort.Strings(names)

	want := []string{"haiku", "opus", "sonnet"}
	if len(names) != len(want) {
		t.Fatalf("TierNamesForClient() returned %d names, want %d: got %v", len(names), len(want), names)
	}
	for i, name := range names {
		if name != want[i] {
			t.Errorf("TierNamesForClient()[%d] = %q, want %q", i, name, want[i])
		}
	}
}

func TestTierNamesForClientUnknown(t *testing.T) {
	data := TierData{}
	names := data.TierNamesForClient("unknown-client")
	if names != nil {
		t.Errorf("TierNamesForClient(unknown) = %v, want nil", names)
	}
}

// ///////////////////////////////////////////////
// TierData.DefaultIconForClient Tests
// ///////////////////////////////////////////////

func TestDefaultIconForClientWithOverride(t *testing.T) {
	data := TierData{
		DefaultIcon: "default",
		Clients: map[string]ClientTierConfig{
			"claude-code": {DefaultIcon: "claude"},
		},
	}
	got := data.DefaultIconForClient("claude-code")
	if got != "claude" {
		t.Errorf("DefaultIconForClient(claude-code) = %q, want %q", got, "claude")
	}
}

func TestDefaultIconForClientFallsBackToGlobal(t *testing.T) {
	data := TierData{
		DefaultIcon: "default",
		Clients: map[string]ClientTierConfig{
			"no-override": {},
		},
	}
	got := data.DefaultIconForClient("no-override")
	if got != "default" {
		t.Errorf("DefaultIconForClient(no-override) = %q, want %q (global)", got, "default")
	}
}

func TestDefaultIconForClientUnknown(t *testing.T) {
	data := TierData{DefaultIcon: "default"}
	got := data.DefaultIconForClient("unknown")
	if got != "default" {
		t.Errorf("DefaultIconForClient(unknown) = %q, want %q (global)", got, "default")
	}
}

// ///////////////////////////////////////////////
// TierData.ResolvedTier Tests
// ///////////////////////////////////////////////

func TestResolvedTierInheritsGlobalAndClientDefaults(t *testing.T) {
	data := TierData{
		Defaults: TierConfig{
			FgColor:  "#222222",
			Size:     512,
			FontSize: 300,
		},
		Clients: map[string]ClientTierConfig{
			"test-client": {
				Defaults: TierConfig{BgColor: "#111111"},
				Tiers: map[string]TierConfig{
					"opus": {}, // All zero values; should inherit all defaults.
				},
			},
		},
	}

	resolved := data.ResolvedTier("test-client", "opus")
	if resolved.BgColor != "#111111" {
		t.Errorf("BgColor = %q, want %q (inherited from client defaults)", resolved.BgColor, "#111111")
	}
	if resolved.FgColor != "#222222" {
		t.Errorf("FgColor = %q, want %q (inherited from global defaults)", resolved.FgColor, "#222222")
	}
	if resolved.Size != 512 {
		t.Errorf("Size = %d, want %d (inherited from global defaults)", resolved.Size, 512)
	}
	if resolved.FontSize != 300 {
		t.Errorf("FontSize = %d, want %d (inherited from global defaults)", resolved.FontSize, 300)
	}
}

func TestResolvedTierOverridesDefaults(t *testing.T) {
	data := TierData{
		Defaults: TierConfig{
			BgColor:  "#111111",
			FgColor:  "#222222",
			Size:     512,
			FontSize: 300,
		},
		Clients: map[string]ClientTierConfig{
			"test-client": {
				Defaults: TierConfig{BgColor: "#333333"},
				Tiers: map[string]TierConfig{
					"custom": {
						BgColor:  "#AABBCC",
						FontSize: 400,
						// FgColor and Size left at zero values; should inherit.
					},
				},
			},
		},
	}

	resolved := data.ResolvedTier("test-client", "custom")
	if resolved.BgColor != "#AABBCC" {
		t.Errorf("BgColor = %q, want %q (overridden by tier)", resolved.BgColor, "#AABBCC")
	}
	if resolved.FgColor != "#222222" {
		t.Errorf("FgColor = %q, want %q (inherited from global)", resolved.FgColor, "#222222")
	}
	if resolved.Size != 512 {
		t.Errorf("Size = %d, want %d (inherited from global)", resolved.Size, 512)
	}
	if resolved.FontSize != 400 {
		t.Errorf("FontSize = %d, want %d (overridden by tier)", resolved.FontSize, 400)
	}
}

func TestResolvedTierUnknownClient(t *testing.T) {
	data := TierData{
		Defaults: TierConfig{
			BgColor:  "#FFFFFF",
			FgColor:  "#000000",
			Size:     1024,
			FontSize: 680,
		},
		Clients: map[string]ClientTierConfig{
			"test-client": {
				Tiers: map[string]TierConfig{"opus": {}},
			},
		},
	}

	// Unknown client should return pure global defaults.
	resolved := data.ResolvedTier("nonexistent", "opus")
	if resolved.BgColor != "#FFFFFF" {
		t.Errorf("BgColor = %q, want global defaults for unknown client", resolved.BgColor)
	}
	if resolved.Size != 1024 {
		t.Errorf("Size = %d, want 1024 for unknown client", resolved.Size)
	}
}

func TestResolvedTierUnknownTier(t *testing.T) {
	data := TierData{
		Defaults: TierConfig{
			FgColor:  "#FFFFFF",
			Size:     1024,
			FontSize: 680,
		},
		Clients: map[string]ClientTierConfig{
			"test-client": {
				Defaults: TierConfig{BgColor: "#DA7756"},
				Tiers:    map[string]TierConfig{"opus": {}},
			},
		},
	}

	// Unknown tier should return global + client defaults.
	resolved := data.ResolvedTier("test-client", "nonexistent")
	if resolved.BgColor != "#DA7756" {
		t.Errorf("BgColor = %q, want %q (client default)", resolved.BgColor, "#DA7756")
	}
	if resolved.FgColor != "#FFFFFF" {
		t.Errorf("FgColor = %q, want %q (global default)", resolved.FgColor, "#FFFFFF")
	}
}

// ///////////////////////////////////////////////
// ExtractTier Tests
// ///////////////////////////////////////////////

func TestExtractTier(t *testing.T) {
	data := &TierData{
		DefaultIcon: "default",
		Clients: map[string]ClientTierConfig{
			"claude-code": {
				DefaultIcon: "claude",
				Tiers: map[string]TierConfig{
					"opus":   {},
					"sonnet": {},
					"haiku":  {},
				},
			},
		},
	}

	tests := []struct {
		model  string
		client string
		want   string
	}{
		{"claude-opus-4-6", "claude-code", "opus"},
		{"claude-sonnet-4-5-20250929", "claude-code", "sonnet"},
		{"claude-haiku-4-5-20251001", "claude-code", "haiku"},
		{"unknown-model", "claude-code", "claude"},       // per-client fallback
		{"claude-unknown-1-0", "claude-code", "claude"},  // per-client fallback
		{"", "claude-code", "claude"},                    // per-client fallback
		{"claude-opus-4-6", "unknown-client", "default"}, // global fallback
	}

	for _, tt := range tests {
		t.Run(tt.model+"/"+tt.client, func(t *testing.T) {
			got := ExtractTier(tt.model, tt.client, data)
			if got != tt.want {
				t.Errorf("ExtractTier(%q, %q) = %q, want %q", tt.model, tt.client, got, tt.want)
			}
		})
	}
}

func TestExtractTierEmptyDefault(t *testing.T) {
	data := &TierData{
		DefaultIcon: "",
		Clients: map[string]ClientTierConfig{
			"test-client": {
				Tiers: map[string]TierConfig{"opus": {}},
			},
		},
	}

	// Unknown model with empty default icon should return "".
	got := ExtractTier("claude-unknown-1-0", "test-client", data)
	if got != "" {
		t.Errorf("ExtractTier with empty default = %q, want empty string", got)
	}
}

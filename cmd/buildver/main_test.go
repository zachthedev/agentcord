package main

import (
	"testing"
)

// ///////////////////////////////////////////////
// formatTaggedVersion Tests
// ///////////////////////////////////////////////

func TestFormatTaggedVersionExactTag(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want string
	}{
		{"clean tag", "v0.1.0", "0.1.0"},
		{"dirty tag", "v0.1.0-dirty", "0.1.0-dirty"},
		{"major only", "v1.0.0", "1.0.0"},
		{"major dirty", "v1.0.0-dirty", "1.0.0-dirty"},
		{"prerelease tag", "v2.0.0-beta.1", "2.0.0-beta.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTaggedVersion(tt.desc)
			if got != tt.want {
				t.Errorf("formatTaggedVersion(%q) = %q, want %q", tt.desc, got, tt.want)
			}
		})
	}
}

func TestFormatTaggedVersionCommitsPastTag(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want string
	}{
		{"3 past tag", "v0.1.0-3-g1234567", "0.1.0-dev.3+g1234567"},
		{"3 past tag dirty", "v0.1.0-3-g1234567-dirty", "0.1.0-dev.3+g1234567.dirty"},
		{"1 past tag", "v1.0.0-1-gabcdef0", "1.0.0-dev.1+gabcdef0"},
		{"1 past tag dirty", "v1.0.0-1-gabcdef0-dirty", "1.0.0-dev.1+gabcdef0.dirty"},
		{"large count", "v2.5.0-42-g9999999", "2.5.0-dev.42+g9999999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTaggedVersion(tt.desc)
			if got != tt.want {
				t.Errorf("formatTaggedVersion(%q) = %q, want %q", tt.desc, got, tt.want)
			}
		})
	}
}

func TestFormatTaggedVersionStripsVPrefix(t *testing.T) {
	got := formatTaggedVersion("v3.2.1")
	if got != "3.2.1" {
		t.Errorf("formatTaggedVersion(%q) = %q, want v prefix stripped", "v3.2.1", got)
	}
}

// ///////////////////////////////////////////////
// baseVersion Tests
// ///////////////////////////////////////////////

func TestBaseVersionNoFile(t *testing.T) {
	// When .release-manifest.json doesn't exist (typical in test env),
	// baseVersion should return the fallback.
	got := baseVersion()
	if got == "" {
		t.Error("baseVersion() returned empty string")
	}
	// Either it reads the real manifest or falls back to 0.0.0.
	// Both are valid depending on test working directory.
}

// ///////////////////////////////////////////////
// isDirty Tests
// ///////////////////////////////////////////////

func TestIsDirtyReturnsBool(t *testing.T) {
	// isDirty shells out to git, so we just verify it doesn't panic
	// and returns a bool. The actual value depends on repo state.
	_ = isDirty()
}

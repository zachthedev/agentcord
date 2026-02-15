package main

import (
	"testing"
)

// ///////////////////////////////////////////////
// parseSectionPath Tests
// ///////////////////////////////////////////////

func TestParseSectionPath(t *testing.T) {
	tests := []struct {
		name    string
		section string
		want    []string
	}{
		{"single segment", "display", []string{"display"}},
		{"two segments", "display.assets", []string{"display", "assets"}},
		{"three segments", "display.format.cost", []string{"display", "format", "cost"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSectionPath(tt.section)
			if len(got) != len(tt.want) {
				t.Fatalf("parseSectionPath(%q) returned %d segments, want %d", tt.section, len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseSectionPath(%q)[%d] = %q, want %q", tt.section, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ///////////////////////////////////////////////
// sectionName Tests
// ///////////////////////////////////////////////

func TestSectionName(t *testing.T) {
	tests := []struct {
		name    string
		section string
		want    string
	}{
		{"single segment", "display", "Display"},
		{"last of two", "display.assets", "Assets"},
		{"last of three", "display.format.cost", "Cost"},
		{"already capitalized", "Display", "Display"},
		{"single char", "a", "A"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sectionName(tt.section)
			if got != tt.want {
				t.Errorf("sectionName(%q) = %q, want %q", tt.section, got, tt.want)
			}
		})
	}
}

func TestSectionNameEmpty(t *testing.T) {
	// A trailing dot produces an empty last segment.
	got := sectionName("")
	if got != "" {
		t.Errorf("sectionName(%q) = %q, want empty string", "", got)
	}
}

// ///////////////////////////////////////////////
// injectOmitted Tests
// ///////////////////////////////////////////////

func TestInjectOmittedNoSection(t *testing.T) {
	// When sectionStack is empty, injectOmitted should be a no-op.
	var out []string
	emitted := map[string]bool{}
	injectOmitted(&out, nil, emitted)
	if len(out) != 0 {
		t.Errorf("injectOmitted with nil sectionStack produced %d lines, want 0", len(out))
	}
}

// color_test.go tests [ParseHexColor] with valid inputs (with and without
// "#" prefix) and rejects malformed hex strings.

package main

import (
	"image/color"
	"testing"
)

func TestParseHexColor(t *testing.T) {
	tests := []struct {
		input string
		want  color.NRGBA
	}{
		{"#DA7756", color.NRGBA{R: 0xDA, G: 0x77, B: 0x56, A: 255}},
		{"#FFFFFF", color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 255}},
		{"#000000", color.NRGBA{R: 0, G: 0, B: 0, A: 255}},
		{"DA7756", color.NRGBA{R: 0xDA, G: 0x77, B: 0x56, A: 255}}, // no # prefix
	}

	for _, tt := range tests {
		c, err := ParseHexColor(tt.input)
		if err != nil {
			t.Errorf("ParseHexColor(%q) error: %v", tt.input, err)
			continue
		}
		if c != tt.want {
			t.Errorf("ParseHexColor(%q) = %v, want %v", tt.input, c, tt.want)
		}
	}
}

func TestParseHexColorInvalid(t *testing.T) {
	invalid := []string{"#FFF", "#GGGGGG", "", "12345"}
	for _, s := range invalid {
		_, err := ParseHexColor(s)
		if err == nil {
			t.Errorf("ParseHexColor(%q) expected error, got nil", s)
		}
	}
}

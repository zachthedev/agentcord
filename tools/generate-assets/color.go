// color.go provides hex color string parsing for the gen-assets tool.

package main

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// ParseHexColor parses a "#RRGGBB" hex color string into a color.NRGBA.
func ParseHexColor(hex string) (color.NRGBA, error) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return color.NRGBA{}, fmt.Errorf("invalid hex color %q: must be 6 hex digits", hex)
	}
	r, err := strconv.ParseUint(hex[0:2], 16, 8)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("invalid hex color %q: %w", hex, err)
	}
	g, err := strconv.ParseUint(hex[2:4], 16, 8)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("invalid hex color %q: %w", hex, err)
	}
	b, err := strconv.ParseUint(hex[4:6], 16, 8)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("invalid hex color %q: %w", hex, err)
	}
	return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}, nil
}

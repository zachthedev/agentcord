// render_test.go tests [RenderAsset] output: valid PNG encoding, correct
// image dimensions, and error handling for invalid color inputs.

package main

import (
	"image/png"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

func TestRenderAsset(t *testing.T) {
	otFont, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("parse gofont: %v", err)
	}

	style := TierConfig{
		BgColor:  "#DA7756",
		FgColor:  "#FFFFFF",
		Size:     256,
		FontSize: 170,
	}

	data, err := RenderAsset(style, "opus", otFont)
	if err != nil {
		t.Fatalf("RenderAsset: %v", err)
	}

	// Verify it's valid PNG
	img, err := png.Decode(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 256 || bounds.Dy() != 256 {
		t.Errorf("image size = %dx%d, want 256x256", bounds.Dx(), bounds.Dy())
	}
}

func TestRenderAssetBadColor(t *testing.T) {
	otFont, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("parse gofont: %v", err)
	}

	style := TierConfig{
		BgColor:  "not-a-color",
		FgColor:  "#FFFFFF",
		Size:     256,
		FontSize: 170,
	}

	_, err = RenderAsset(style, "opus", otFont)
	if err == nil {
		t.Error("expected error for bad color")
	}
}

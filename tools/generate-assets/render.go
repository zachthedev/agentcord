// render.go implements PNG asset rendering for the gen-assets tool.
// [RenderAsset] produces a square PNG image with a single centered capital
// letter drawn on a solid background, sized according to [TierConfig].

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// RenderAsset renders a single tier asset: a centered capital letter on a colored background.
// Returns the PNG bytes.
func RenderAsset(style TierConfig, tierName string, otFont *opentype.Font) ([]byte, error) {
	letter := strings.ToUpper(tierName[:1])

	bgColor, err := ParseHexColor(style.BgColor)
	if err != nil {
		return nil, fmt.Errorf("parse bg_color: %w", err)
	}
	fgColor, err := ParseHexColor(style.FgColor)
	if err != nil {
		return nil, fmt.Errorf("parse fg_color: %w", err)
	}

	size := style.Size
	fontSize := style.FontSize

	// Create face at the requested size
	face, err := opentype.NewFace(otFont, &opentype.FaceOptions{
		Size:    float64(fontSize),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("create font face: %w", err)
	}
	defer face.Close()

	// Measure the glyph bounding box for visual centering.
	// BoundString gives the actual pixel bounds of the rendered glyphs.
	bounds, _ := font.BoundString(face, letter)

	glyphW := (bounds.Max.X - bounds.Min.X).Ceil()
	glyphH := (bounds.Max.Y - bounds.Min.Y).Ceil()

	// Center the glyph on the canvas
	originX := (size-glyphW)/2 - bounds.Min.X.Floor()
	originY := (size-glyphH)/2 - bounds.Min.Y.Floor()

	// Create canvas and fill background
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), image.NewUniform(bgColor), image.Point{}, draw.Src)

	// Draw the letter
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(fgColor),
		Face: face,
		Dot:  fixed.P(originX, originY),
	}
	d.DrawString(letter)

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

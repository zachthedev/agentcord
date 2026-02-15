// gen-assets generates Discord Rich Presence model tier assets.
//
// Reads tier list and styling from data/tiers.json, resolves fonts per-client,
// and renders a centered capital letter on each tier's background. Output goes
// to assets/discord/{client}/presence/.
//
// Font resolution per client:
//  1. Local file path from tiers.json "font" field
//  2. Google Fonts download from "font_fallback" field (e.g. "google:Inter:800")
//  3. Skip client with warning if neither is available
//
// Usage:
//
//	cd tools/generate-assets && go run .
//	cd tools/generate-assets && go run . -tiers ../../data/tiers.json -out ../../assets/discord
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tdewolff/font"
	"golang.org/x/image/font/opentype"
)

func main() {
	// Default paths assume running from tools/generate-assets/
	tiersFile := flag.String("tiers", "../../data/tiers.json", "Path to tiers.json")
	outDir := flag.String("out", "../../assets/discord", "Base output directory (assets written to {out}/{client}/presence/)")
	flag.Parse()

	// Resolve repo root relative to the tool directory (for font paths in tiers.json)
	repoRoot, err := filepath.Abs(filepath.Join(filepath.Dir(*tiersFile), ".."))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: resolve repo root: %v\n", err)
		os.Exit(1)
	}

	fontCacheDir := filepath.Join(repoRoot, "assets", "fonts", ".cache")

	// Load tier data
	tierData, err := LoadTierData(*tiersFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load tiers: %v\n", err)
		os.Exit(1)
	}

	if len(tierData.Clients) == 0 {
		fmt.Fprintln(os.Stderr, "error: no clients defined in tiers.json")
		os.Exit(1)
	}

	totalAssets := 0

	for clientID, clientCfg := range tierData.Clients {
		fmt.Printf("[%s]\n", clientID)

		if len(clientCfg.Tiers) == 0 {
			fmt.Printf("  (no tiers defined, skipping)\n")
			continue
		}

		// Resolve font for this client
		fontBytes, err := resolveFont(clientCfg, repoRoot, fontCacheDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skipping: %v\n", err)
			continue
		}

		otFont, err := opentype.Parse(fontBytes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skipping: parse font: %v\n", err)
			continue
		}

		// Ensure per-client output directory
		clientOutDir := filepath.Join(*outDir, clientID, "presence")
		if err := os.MkdirAll(clientOutDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "  error: create output dir: %v\n", err)
			os.Exit(1)
		}

		// Generate assets for each tier
		for name := range clientCfg.Tiers {
			style := tierData.ResolvedTier(clientID, name)
			pngData, err := RenderAsset(style, name, otFont)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  error: render %s: %v\n", name, err)
				os.Exit(1)
			}

			outPath := filepath.Join(clientOutDir, name+".png")
			if err := os.WriteFile(outPath, pngData, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "  error: write %s: %v\n", outPath, err)
				os.Exit(1)
			}
			fmt.Printf("  %s.png (%s)\n", name, strings.ToUpper(name[:1]))
			totalAssets++
		}
	}

	fmt.Printf("Done. Generated %d assets for %d clients.\n", totalAssets, len(tierData.Clients))
}

// resolveFont tries to load a font for a client using this fallback chain:
//  1. Local file from clientCfg.Font (path relative to repoRoot)
//  2. Google Fonts from clientCfg.FontFallback (e.g. "google:Inter:800")
func resolveFont(clientCfg ClientTierConfig, repoRoot, fontCacheDir string) ([]byte, error) {
	// Try local font file
	if clientCfg.Font != "" {
		localPath := filepath.Join(repoRoot, clientCfg.Font)
		if data, err := os.ReadFile(localPath); err == nil {
			fmt.Printf("  font: %s (local)\n", clientCfg.Font)
			return maybeConvertWOFF2(localPath, data)
		}
	}

	// Try Google Fonts fallback
	if clientCfg.FontFallback != "" {
		family, weight, ok := ParseGoogleFontSpec(clientCfg.FontFallback)
		if ok {
			fmt.Printf("  font: %s wght@%s (Google Fonts)\n", family, weight)
			data, err := FetchGoogleFont(clientCfg.FontFallback, fontCacheDir)
			if err != nil {
				return nil, fmt.Errorf("google fonts fallback failed: %w", err)
			}
			return data, nil
		}
	}

	return nil, fmt.Errorf("no font configured (set \"font\" or \"font_fallback\" in tiers.json)")
}

// maybeConvertWOFF2 converts WOFF2 font data to SFNT format if needed.
func maybeConvertWOFF2(path string, data []byte) ([]byte, error) {
	if isWOFF2(path, data) {
		sfnt, err := font.ToSFNT(data)
		if err != nil {
			return nil, fmt.Errorf("convert woff2 to sfnt: %w", err)
		}
		return sfnt, nil
	}
	return data, nil
}

// isWOFF2 checks whether a font file is WOFF2 by extension or magic bytes.
// WOFF2 magic: 0x774F4632 ("wOF2")
func isWOFF2(path string, data []byte) bool {
	if strings.HasSuffix(strings.ToLower(path), ".woff2") {
		return true
	}
	if len(data) >= 4 && data[0] == 'w' && data[1] == 'O' && data[2] == 'F' && data[3] == '2' {
		return true
	}
	return false
}

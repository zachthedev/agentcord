// Package main implements the genconfig tool that writes config.default.toml
// from config.ExampleConfig().
//
// It is invoked by go generate via the directive in internal/config/config.go.
package main

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"tools.zach/dev/agentcord/internal/config"
)

func main() {
	cfg := config.ExampleConfig()

	// Marshal to TOML
	var raw bytes.Buffer
	enc := toml.NewEncoder(&raw)
	if err := enc.Encode(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
		os.Exit(1)
	}

	// Post-process: inject comments from ConfigDocs and strip indentation
	lines := strings.Split(raw.String(), "\n")
	var out []string

	// File header
	out = append(out,
		"# ///////////////////////////////////////////////",
		"# Agentcord Configuration",
		"# ///////////////////////////////////////////////",
		"",
	)

	// Track current TOML section path for field lookup
	var sectionStack []string
	// Track which doc keys we've emitted so we can inject omitted fields
	emittedKeys := map[string]bool{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines from the encoder (we manage spacing ourselves)
		if trimmed == "" {
			continue
		}

		// Track section headers: [foo] or [foo.bar]
		if strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "[[") {
			// Before changing sections, inject any omitted fields for the current section
			injectOmitted(&out, sectionStack, emittedKeys)

			section := strings.Trim(trimmed, "[] ")
			sectionStack = parseSectionPath(section)

			// Add section separator with blank line before
			sectionLabel := sectionName(section)
			out = append(out, "")
			out = append(out, fmt.Sprintf("# ///// %s /////", sectionLabel))
			out = append(out, "")

			// Look up section-level docs
			if doc, ok := config.ConfigDocs[section]; ok && doc.Comment != "" {
				for _, cl := range strings.Split(doc.Comment, "\n") {
					out = append(out, "# "+cl)
				}
			}

			// Write section header without indentation
			out = append(out, trimmed)
			continue
		}

		// Non key=value lines pass through unchanged
		if !strings.Contains(trimmed, "=") || strings.HasPrefix(trimmed, "#") {
			out = append(out, trimmed)
			continue
		}

		// Key = value lines (strip indentation)
		key := strings.TrimSpace(strings.SplitN(trimmed, "=", 2)[0])
		fullPath := key
		if len(sectionStack) > 0 {
			fullPath = strings.Join(sectionStack, ".") + "." + key
		}
		emittedKeys[fullPath] = true

		doc, ok := config.ConfigDocs[fullPath]
		if !ok {
			// No doc entry — just emit the line
			out = append(out, trimmed)
			continue
		}
		if doc.Comment != "" {
			for _, cl := range strings.Split(doc.Comment, "\n") {
				out = append(out, "# "+cl)
			}
		}
		out = append(out, trimmed)
		for _, alt := range doc.Alternatives {
			out = append(out, "# "+alt)
		}
	}

	// Inject any remaining omitted fields in the last section
	injectOmitted(&out, sectionStack, emittedKeys)

	result := strings.Join(out, "\n")
	result = strings.TrimRight(result, "\n") + "\n"

	// go generate runs from the package directory (internal/config/).
	// With go.mod at root, ../../ reaches the repo root where configdata.go
	// embeds config.default.toml — single source of truth.
	outPath := "../../config.default.toml"
	if err := os.WriteFile(outPath, []byte(result), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", outPath, err)
		os.Exit(1)
	}
	fmt.Printf("wrote config.default.toml\n")
}

// injectOmitted appends commented-out entries for [config.ConfigDocs] keys that
// belong to the current section but were not emitted by the TOML encoder (typically
// because the field has an omitempty tag and holds its zero value). This ensures
// every documented option appears in the generated file, even when its value is
// omitted from the encoded output. Keys are sorted for deterministic ordering.
func injectOmitted(out *[]string, sectionStack []string, emitted map[string]bool) {
	if len(sectionStack) == 0 {
		return
	}
	prefix := strings.Join(sectionStack, ".") + "."

	// Collect omitted keys and sort for deterministic output
	var omitted []string
	for path := range config.ConfigDocs {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		rest := strings.TrimPrefix(path, prefix)
		if strings.Contains(rest, ".") {
			continue
		}
		if emitted[path] {
			continue
		}
		omitted = append(omitted, path)
	}
	sort.Strings(omitted)

	for _, path := range omitted {
		doc := config.ConfigDocs[path]
		*out = append(*out, "")
		if doc.Comment != "" {
			for _, cl := range strings.Split(doc.Comment, "\n") {
				*out = append(*out, "# "+cl)
			}
		}
		if len(doc.Alternatives) > 0 {
			for _, alt := range doc.Alternatives {
				*out = append(*out, "# "+alt)
			}
		}
		emitted[path] = true
	}
}

// parseSectionPath splits a dotted TOML section header (e.g. "display.assets")
// into its component path segments (["display", "assets"]). The returned slice
// is used as a stack to track the current nesting depth during output generation.
func parseSectionPath(section string) []string {
	return strings.Split(section, ".")
}

// sectionName returns a human-readable display name for a TOML section header
// by extracting the last dotted segment and capitalizing its first letter.
// For example, "display.assets" yields "Assets".
func sectionName(section string) string {
	parts := strings.Split(section, ".")
	last := parts[len(parts)-1]
	if len(last) == 0 {
		return ""
	}
	return strings.ToUpper(last[:1]) + last[1:]
}

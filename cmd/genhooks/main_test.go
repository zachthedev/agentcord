package main

import (
	"strings"
	"testing"
)

// ///////////////////////////////////////////////
// genHeader Tests
// ///////////////////////////////////////////////

func TestGenHeaderContainsFilename(t *testing.T) {
	tests := []struct {
		filename string
	}{
		{"constants.sh"},
		{"constants.ps1"},
		{"test-file.sh"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			header := genHeader(tt.filename)
			if !strings.Contains(header, tt.filename) {
				t.Errorf("genHeader(%q) does not contain filename: %q", tt.filename, header)
			}
		})
	}
}

func TestGenHeaderContainsDoNotEdit(t *testing.T) {
	header := genHeader("constants.sh")
	if !strings.Contains(header, "DO NOT EDIT") {
		t.Error("genHeader output missing 'DO NOT EDIT' marker")
	}
}

func TestGenHeaderContainsSourceReference(t *testing.T) {
	header := genHeader("constants.sh")
	if !strings.Contains(header, "internal/paths/paths.go") {
		t.Error("genHeader output missing reference to paths.go")
	}
	if !strings.Contains(header, "internal/session/state.go") {
		t.Error("genHeader output missing reference to state.go")
	}
}

func TestGenHeaderFormat(t *testing.T) {
	header := genHeader("test.sh")

	lines := strings.Split(strings.TrimRight(header, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("genHeader produced %d lines, want 2", len(lines))
	}

	// First line should be a comment with the filename.
	if !strings.HasPrefix(lines[0], "# ") {
		t.Errorf("first line should start with '# ', got %q", lines[0])
	}
	if !strings.Contains(lines[0], "test.sh") {
		t.Errorf("first line should contain filename, got %q", lines[0])
	}

	// Second line should be a comment with source reference.
	if !strings.HasPrefix(lines[1], "# ") {
		t.Errorf("second line should start with '# ', got %q", lines[1])
	}
}

func TestGenHeaderEndsWithNewline(t *testing.T) {
	header := genHeader("constants.sh")
	if !strings.HasSuffix(header, "\n") {
		t.Error("genHeader output should end with a newline")
	}
}

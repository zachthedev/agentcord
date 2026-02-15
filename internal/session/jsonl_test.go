// Tests for JSONL parsing, discovery, and token formatting in the session package.
// Covers [ParseJSONL], [FindLatestJSONL], and [FormatTokenCount].
package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ///////////////////////////////////////////////
// ParseJSONL Tests
// ///////////////////////////////////////////////

func TestParseJSONL(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantModel  string
		wantInput  int64
		wantOutput int64
		wantErr    bool
	}{
		{
			name:       "single entry",
			content:    `{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":50}}` + "\n",
			wantModel:  "claude-opus-4-6",
			wantInput:  100,
			wantOutput: 50,
		},
		{
			name: "multiple entries aggregate tokens",
			content: `{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":50}}
{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":200,"output_tokens":75}}
{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":300,"output_tokens":100}}
`,
			wantModel:  "claude-opus-4-6",
			wantInput:  600,
			wantOutput: 225,
		},
		{
			name: "mixed models uses latest",
			content: `{"type":"assistant","model":"claude-sonnet-4-5-20250929","usage":{"input_tokens":100,"output_tokens":50}}
{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":200,"output_tokens":75}}
`,
			wantModel:  "claude-opus-4-6",
			wantInput:  300,
			wantOutput: 125,
		},
		{
			name: "malformed line skipped",
			content: `{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":50}}
{this is broken json}
{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":200,"output_tokens":75}}
`,
			wantModel:  "claude-opus-4-6",
			wantInput:  300,
			wantOutput: 125,
		},
		{
			name:       "empty file",
			content:    "",
			wantModel:  "",
			wantInput:  0,
			wantOutput: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "test.jsonl")
			os.WriteFile(path, []byte(tt.content), 0o644)

			data, err := ParseJSONL(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseJSONL: %v", err)
				return
			}
			if data.Model != tt.wantModel {
				t.Errorf("Model = %q, want %q", data.Model, tt.wantModel)
			}
			if data.InputTokens != tt.wantInput {
				t.Errorf("InputTokens = %d, want %d", data.InputTokens, tt.wantInput)
			}
			if data.OutputTokens != tt.wantOutput {
				t.Errorf("OutputTokens = %d, want %d", data.OutputTokens, tt.wantOutput)
			}
		})
	}
}

func TestParseJSONL_LargeLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	// Build a line with a 10KB content field to test scanner buffer handling
	large := `{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":50000,"output_tokens":25000},"content":"`
	large += strings.Repeat("x", 10000)
	large += "\"}\n"
	os.WriteFile(path, []byte(large), 0o644)

	data, err := ParseJSONL(path)
	if err != nil {
		t.Fatalf("ParseJSONL: %v", err)
		return
	}
	if data.InputTokens != 50000 {
		t.Errorf("InputTokens = %d, want 50000", data.InputTokens)
	}
}

// ///////////////////////////////////////////////
// FindLatestJSONL Tests
// ///////////////////////////////////////////////

func TestFindLatestJSONL(t *testing.T) {
	dir := t.TempDir()

	// Create two JSONL files with different modification times
	path1 := filepath.Join(dir, "session1.jsonl")
	path2 := filepath.Join(dir, "session2.jsonl")
	os.WriteFile(path1, []byte(`{"type":"assistant"}`), 0o644)
	os.Chtimes(path1, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	os.WriteFile(path2, []byte(`{"type":"assistant"}`), 0o644)

	latest, err := FindLatestJSONL(dir)
	if err != nil {
		t.Fatalf("FindLatestJSONL: %v", err)
		return
	}
	if filepath.Base(latest) != "session2.jsonl" {
		t.Errorf("latest = %q, want session2.jsonl", filepath.Base(latest))
	}
}

func TestFindLatestJSONL_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, err := FindLatestJSONL(dir)
	if err == nil {
		t.Fatal("expected error for empty directory, got nil")
	}
}

func TestFindLatestJSONL_NoJSONLFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(dir, "data.json"), []byte("{}"), 0o644)

	_, err := FindLatestJSONL(dir)
	if err == nil {
		t.Fatal("expected error when no .jsonl files exist, got nil")
	}
}

func TestFindLatestJSONL_SelectsNewest(t *testing.T) {
	dir := t.TempDir()

	// Create three JSONL files; make the middle one the newest.
	paths := []string{
		filepath.Join(dir, "a.jsonl"),
		filepath.Join(dir, "b.jsonl"),
		filepath.Join(dir, "c.jsonl"),
	}
	for _, p := range paths {
		os.WriteFile(p, []byte(`{}`), 0o644)
	}

	// Set a to 2 hours ago, c to 1 hour ago, b stays at now.
	os.Chtimes(paths[0], time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour))
	os.Chtimes(paths[2], time.Now().Add(-1*time.Hour), time.Now().Add(-1*time.Hour))

	latest, err := FindLatestJSONL(dir)
	if err != nil {
		t.Fatalf("FindLatestJSONL: %v", err)
	}
	if filepath.Base(latest) != "b.jsonl" {
		t.Errorf("latest = %q, want b.jsonl", filepath.Base(latest))
	}
}

func TestParseJSONL_MissingFile(t *testing.T) {
	_, err := ParseJSONL(filepath.Join(t.TempDir(), "nonexistent.jsonl"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestParseJSONL_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(path, []byte(""), 0o644)

	data, err := ParseJSONL(path)
	if err != nil {
		t.Fatalf("ParseJSONL: %v", err)
	}
	if data.InputTokens != 0 {
		t.Errorf("InputTokens = %d, want 0", data.InputTokens)
	}
	if data.OutputTokens != 0 {
		t.Errorf("OutputTokens = %d, want 0", data.OutputTokens)
	}
	if data.Model != "" {
		t.Errorf("Model = %q, want empty", data.Model)
	}
}

func TestParseJSONL_MalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jsonl")
	content := "{not valid json}\n{also bad\n\n"
	os.WriteFile(path, []byte(content), 0o644)

	data, err := ParseJSONL(path)
	if err != nil {
		t.Fatalf("ParseJSONL: %v", err)
	}
	if data.InputTokens != 0 {
		t.Errorf("InputTokens = %d, want 0 (all lines malformed)", data.InputTokens)
	}
	if data.OutputTokens != 0 {
		t.Errorf("OutputTokens = %d, want 0 (all lines malformed)", data.OutputTokens)
	}
}

func TestParseJSONL_AggregatesTokens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.jsonl")
	content := `{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":1000,"output_tokens":500}}
{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":2000,"output_tokens":1000}}
{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":3000,"output_tokens":1500}}
`
	os.WriteFile(path, []byte(content), 0o644)

	data, err := ParseJSONL(path)
	if err != nil {
		t.Fatalf("ParseJSONL: %v", err)
	}
	if data.InputTokens != 6000 {
		t.Errorf("InputTokens = %d, want 6000", data.InputTokens)
	}
	if data.OutputTokens != 3000 {
		t.Errorf("OutputTokens = %d, want 3000", data.OutputTokens)
	}
	if data.Model != "claude-opus-4-6" {
		t.Errorf("Model = %q, want %q", data.Model, "claude-opus-4-6")
	}
}

// ///////////////////////////////////////////////
// ParseJSONLCached Tests
// ///////////////////////////////////////////////

func TestParseJSONLCached_IncrementalParse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	// Write initial entries.
	initial := `{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":50}}
{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":200,"output_tokens":75}}
`
	os.WriteFile(path, []byte(initial), 0o644)

	cache := NewJSONLCache(path)

	// First parse: full scan.
	data, err := ParseJSONLCached(cache)
	if err != nil {
		t.Fatalf("first ParseJSONLCached: %v", err)
	}
	if data.InputTokens != 300 || data.OutputTokens != 125 {
		t.Errorf("first parse: input=%d output=%d, want 300/125", data.InputTokens, data.OutputTokens)
	}

	// Append more entries.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(`{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":500,"output_tokens":250}}` + "\n")
	f.Close()

	// Second parse: should pick up only the new entry.
	data, err = ParseJSONLCached(cache)
	if err != nil {
		t.Fatalf("second ParseJSONLCached: %v", err)
	}
	if data.InputTokens != 800 || data.OutputTokens != 375 {
		t.Errorf("second parse: input=%d output=%d, want 800/375", data.InputTokens, data.OutputTokens)
	}
}

func TestParseJSONLCached_NoChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	os.WriteFile(path, []byte(`{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":50}}`+"\n"), 0o644)

	cache := NewJSONLCache(path)

	data1, _ := ParseJSONLCached(cache)
	data2, _ := ParseJSONLCached(cache)

	if data1.InputTokens != data2.InputTokens || data1.OutputTokens != data2.OutputTokens {
		t.Errorf("unchanged file returned different results: %+v vs %+v", data1, data2)
	}
}

func TestParseJSONLCached_Truncation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	// Write a large file.
	large := `{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":1000,"output_tokens":500}}
{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":2000,"output_tokens":1000}}
`
	os.WriteFile(path, []byte(large), 0o644)

	cache := NewJSONLCache(path)
	ParseJSONLCached(cache)

	// Truncate to a smaller file (simulates rotation).
	small := `{"type":"assistant","model":"claude-opus-4-6","usage":{"input_tokens":50,"output_tokens":25}}` + "\n"
	os.WriteFile(path, []byte(small), 0o644)

	data, err := ParseJSONLCached(cache)
	if err != nil {
		t.Fatalf("ParseJSONLCached after truncation: %v", err)
	}
	if data.InputTokens != 50 || data.OutputTokens != 25 {
		t.Errorf("after truncation: input=%d output=%d, want 50/25", data.InputTokens, data.OutputTokens)
	}
}

func TestParseJSONLCached_MissingFile(t *testing.T) {
	cache := NewJSONLCache(filepath.Join(t.TempDir(), "nonexistent.jsonl"))
	_, err := ParseJSONLCached(cache)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// ///////////////////////////////////////////////
// FormatTokenCount Tests
// ///////////////////////////////////////////////

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		name   string
		tokens int64
		format string
		want   string
	}{
		{"short millions", 1500000, "short", "1.5M"},
		{"short thousands", 234000, "short", "234K"},
		{"short small", 500, "short", "500"},
		{"full millions", 1500000, "full", "1,500,000"},
		{"full thousands", 234000, "full", "234,000"},
		{"full small", 500, "full", "500"},
		{"short zero", 0, "short", "0"},
		{"short exact K boundary", 1000, "short", "1K"},
		{"short below K boundary", 999, "short", "999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTokenCount(tt.tokens, tt.format)
			if got != tt.want {
				t.Errorf("FormatTokenCount(%d, %q) = %q, want %q", tt.tokens, tt.format, got, tt.want)
			}
		})
	}
}

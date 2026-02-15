// Package logger tests verify the custom [Handler] output format, level
// filtering, attribute grouping, and the [ReadTail] utility.
package logger

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ///////////////////////////////////////////////
// Handler Output Format
// ///////////////////////////////////////////////

func TestHandler_Format(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, LevelInfo)
	logger := slog.New(h)

	logger.Info("test message", "key", "value")

	line := buf.String()
	// Trim platform-specific line ending
	line = strings.TrimRight(line, "\r\n")

	// Check format: timestamp [LEVEL] message | key=value
	if !strings.Contains(line, "[INFO]") {
		t.Errorf("expected [INFO] in output, got %q", line)
	}
	if !strings.Contains(line, "test message") {
		t.Errorf("expected message in output, got %q", line)
	}
	if !strings.Contains(line, "| key=value") {
		t.Errorf("expected key=value in output, got %q", line)
	}
	// Timestamp should end with Z (UTC)
	if !strings.HasSuffix(strings.Split(line, " [")[0], "Z") {
		t.Errorf("expected UTC timestamp ending with Z, got %q", line)
	}
}

func TestHandler_NoAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, LevelInfo)
	logger := slog.New(h)

	logger.Info("no attrs")

	line := strings.TrimRight(buf.String(), "\r\n")
	if strings.Contains(line, "|") {
		t.Errorf("expected no pipe separator without attrs, got %q", line)
	}
}

func TestHandler_MultipleAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, LevelInfo)
	logger := slog.New(h)

	logger.Info("multi", "a", "1", "b", "2")

	line := strings.TrimRight(buf.String(), "\r\n")
	if !strings.Contains(line, "a=1, b=2") {
		t.Errorf("expected comma-separated attrs, got %q", line)
	}
}

// ///////////////////////////////////////////////
// Level Filtering
// ///////////////////////////////////////////////

func TestHandler_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, LevelWarn)
	logger := slog.New(h)

	logger.Info("should be filtered")
	logger.Warn("should appear")

	output := buf.String()
	if strings.Contains(output, "should be filtered") {
		t.Error("info message should have been filtered at warn level")
	}
	if !strings.Contains(output, "should appear") {
		t.Error("warn message should appear at warn level")
	}
}

// ///////////////////////////////////////////////
// Custom Levels
// ///////////////////////////////////////////////

func TestHandler_CustomLevels(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, LevelTrace)
	logger := slog.New(h)

	Trace(logger, "trace msg")
	Fail(logger, "fail msg")

	output := buf.String()
	if !strings.Contains(output, "[TRACE]") {
		t.Errorf("expected [TRACE] in output, got %q", output)
	}
	if !strings.Contains(output, "[FAIL]") {
		t.Errorf("expected [FAIL] in output, got %q", output)
	}
}

func TestHandler_LevelNames(t *testing.T) {
	tests := []struct {
		name  string
		level slog.Level
		want  string
	}{
		{"trace", LevelTrace, "TRACE"},
		{"debug", LevelDebug, "DEBUG"},
		{"info", LevelInfo, "INFO"},
		{"warn", LevelWarn, "WARN"},
		{"error", LevelError, "ERROR"},
		{"fail", LevelFail, "FAIL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := levelName(tt.level); got != tt.want {
				t.Errorf("levelName(%d) = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

// ///////////////////////////////////////////////
// ParseLevel
// ///////////////////////////////////////////////

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  slog.Level
	}{
		{"trace_lower", "trace", LevelTrace},
		{"trace_upper", "TRACE", LevelTrace},
		{"debug", "debug", LevelDebug},
		{"info", "info", LevelInfo},
		{"warn", "warn", LevelWarn},
		{"error", "error", LevelError},
		{"fail", "fail", LevelFail},
		{"unknown_defaults_to_info", "unknown", LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseLevel(tt.input); got != tt.want {
				t.Errorf("ParseLevel(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// ///////////////////////////////////////////////
// WithAttrs
// ///////////////////////////////////////////////

func TestHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, LevelInfo)
	h2 := h.WithAttrs([]slog.Attr{slog.String("service", "agentcord")})
	logger := slog.New(h2)

	logger.Info("test")

	line := strings.TrimRight(buf.String(), "\r\n")
	if !strings.Contains(line, "service=agentcord") {
		t.Errorf("expected pre-applied attr, got %q", line)
	}
}

// ///////////////////////////////////////////////
// ReadTail
// ///////////////////////////////////////////////

func TestReadTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
		return
	}

	result, err := ReadTail(path, 3)
	if err != nil {
		t.Fatalf("ReadTail: %v", err)
		return
	}

	lines := strings.Split(result, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), result)
		return
	}
	if lines[0] != "line3" || lines[1] != "line4" || lines[2] != "line5" {
		t.Errorf("unexpected lines: %q", result)
	}
}

func TestReadTail_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.log")
	os.WriteFile(path, []byte(""), 0o644)

	result, err := ReadTail(path, 10)
	if err != nil {
		t.Fatalf("ReadTail: %v", err)
	}
	if result != "" {
		t.Errorf("ReadTail of empty file = %q, want empty string", result)
	}
}

func TestReadTail_MissingFile(t *testing.T) {
	_, err := ReadTail("/nonexistent/path/test.log", 10)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadTail_FewerLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	content := "line1\nline2\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
		return
	}

	result, err := ReadTail(path, 10)
	if err != nil {
		t.Fatalf("ReadTail: %v", err)
		return
	}
	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), result)
	}
}

// ///////////////////////////////////////////////
// NewLogger Constructor
// ///////////////////////////////////////////////

func TestNewLogger(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	logger, closer, err := NewLogger(path, LevelInfo, 10)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
		return
	}
	defer closer.Close()

	if logger == nil {
		t.Fatal("expected non-nil logger")
		return
	}

	logger.Info("constructor test")
	closer.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
		return
	}
	if !strings.Contains(string(data), "constructor test") {
		t.Errorf("expected log output in file, got %q", string(data))
	}
}

// ///////////////////////////////////////////////
// WithGroup
// ///////////////////////////////////////////////

func TestHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, LevelInfo)
	gh := h.WithGroup("request")
	logger := slog.New(gh)

	logger.Info("grouped", "method", "GET", "path", "/api")

	line := strings.TrimRight(buf.String(), "\r\n")
	if !strings.Contains(line, "request.method=GET") {
		t.Errorf("expected group prefix on key, got %q", line)
	}
	if !strings.Contains(line, "request.path=/api") {
		t.Errorf("expected group prefix on second key, got %q", line)
	}
}

func TestHandler_WithGroupNested(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, LevelInfo)
	gh := h.WithGroup("server").WithGroup("request")
	logger := slog.New(gh)

	logger.Info("nested", "method", "POST")

	line := strings.TrimRight(buf.String(), "\r\n")
	if !strings.Contains(line, "server.request.method=POST") {
		t.Errorf("expected nested group prefix, got %q", line)
	}
}

func TestHandler_WithGroupEmpty(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, LevelInfo)
	gh := h.WithGroup("")
	if gh != h {
		t.Error("WithGroup with empty string should return same handler")
	}
}

// ///////////////////////////////////////////////
// WithAttrs Shared Mutex
// ///////////////////////////////////////////////

func TestHandler_WithAttrsSharedMutex(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, LevelInfo)
	h2 := h.WithAttrs([]slog.Attr{slog.String("k", "v")}).(*Handler)

	if h.mu != h2.mu {
		t.Error("WithAttrs should share the same mutex pointer")
	}

	// Verify concurrent writes don't panic or interleave badly.
	logger1 := slog.New(h)
	logger2 := slog.New(h2)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			logger1.Info("from handler 1")
		}()
		go func() {
			defer wg.Done()
			logger2.Info("from handler 2")
		}()
	}
	wg.Wait()

	output := buf.String()
	lines := strings.Split(strings.TrimRight(output, "\r\n"), "\n")
	// With CRLF endings, trim each line
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	if len(lines) != 100 {
		t.Errorf("expected 100 log lines, got %d", len(lines))
	}
}

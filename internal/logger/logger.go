// Package logger provides structured logging with custom levels and formatting
// for the Agentcord daemon.
//
// Log output format:
//
//	2006-01-02T15:04:05.000Z [LEVEL] message | key=value, key2=value2
//
// Custom levels beyond the standard slog set:
//   - LevelTrace (-8): verbose diagnostic tracing
//   - LevelFail  (12): unrecoverable errors
package logger

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

// ///////////////////////////////////////////////
// Custom Levels
// ///////////////////////////////////////////////

const (
	LevelTrace slog.Level = -8
	LevelDebug slog.Level = slog.LevelDebug // -4
	LevelInfo  slog.Level = slog.LevelInfo  // 0
	LevelWarn  slog.Level = slog.LevelWarn  // 4
	LevelError slog.Level = slog.LevelError // 8
	LevelFail  slog.Level = 12
)

// levelName returns the display name for a log level.
func levelName(l slog.Level) string {
	switch {
	case l <= LevelTrace:
		return "TRACE"
	case l <= LevelDebug:
		return "DEBUG"
	case l <= LevelInfo:
		return "INFO"
	case l <= LevelWarn:
		return "WARN"
	case l <= LevelError:
		return "ERROR"
	default:
		return "FAIL"
	}
}

// ParseLevel converts a level string to slog.Level.
// Supports: trace, debug, info, warn, error, fail (case-insensitive).
// Returns LevelInfo for unrecognized strings.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "trace":
		return LevelTrace
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	case "fail":
		return LevelFail
	default:
		return LevelInfo
	}
}

// ///////////////////////////////////////////////
// Handler
// ///////////////////////////////////////////////

// lineEnding is CRLF on Windows, LF elsewhere.
var lineEnding = "\n"

func init() {
	if runtime.GOOS == "windows" {
		lineEnding = "\r\n"
	}
}

// Handler is a custom slog.Handler that formats log records as:
//
//	2006-01-02T15:04:05.000Z [LEVEL] message | key=value, ...
type Handler struct {
	// w is the destination writer for formatted log output.
	w io.Writer
	// mu serializes writes to w so concurrent log calls do not interleave.
	mu *sync.Mutex
	// level is the minimum severity that this handler will emit.
	level slog.Level
	// attrs holds pre-applied attributes added via [Handler.WithAttrs].
	attrs []slog.Attr
	// group is the dot-separated attribute key prefix set via [Handler.WithGroup].
	group string
}

// NewHandler creates a Handler that writes to w, filtering records below level.
func NewHandler(w io.Writer, level slog.Level) *Handler {
	return &Handler{w: w, level: level, mu: &sync.Mutex{}}
}

// Enabled reports whether the handler handles records at the given level.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle formats and writes a log record.
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	var buf strings.Builder

	// Timestamp
	buf.WriteString(r.Time.UTC().Format("2006-01-02T15:04:05.000Z"))

	// Level
	buf.WriteString(" [")
	buf.WriteString(levelName(r.Level))
	buf.WriteString("] ")

	// Message
	buf.WriteString(r.Message)

	// Attributes (pre-defined + record attrs)
	allAttrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	allAttrs = append(allAttrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		allAttrs = append(allAttrs, a)
		return true
	})

	if len(allAttrs) > 0 {
		buf.WriteString(" | ")
		for i, a := range allAttrs {
			if i > 0 {
				buf.WriteString(", ")
			}
			if h.group != "" {
				buf.WriteString(h.group)
				buf.WriteString(".")
			}
			buf.WriteString(a.Key)
			buf.WriteString("=")
			buf.WriteString(a.Value.String())
		}
	}

	buf.WriteString(lineEnding)

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, buf.String())
	return err
}

// WithAttrs returns a new Handler with the given attributes pre-applied.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)
	return &Handler{w: h.w, mu: h.mu, level: h.level, attrs: newAttrs, group: h.group}
}

// WithGroup returns a new Handler with the given group name.
// Attributes logged through the returned handler will have keys
// prefixed with the group name (e.g., "group.key").
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newGroup := name
	if h.group != "" {
		newGroup = h.group + "." + name
	}
	return &Handler{w: h.w, mu: h.mu, level: h.level, attrs: h.attrs, group: newGroup}
}

// ///////////////////////////////////////////////
// Logger Constructor
// ///////////////////////////////////////////////

// NewLogger creates a slog.Logger that writes to a rotating log file.
// The returned io.Closer must be closed to flush pending writes.
func NewLogger(logPath string, minLevel slog.Level, maxSizeMB int) (*slog.Logger, io.Closer, error) {
	lj := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    maxSizeMB,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   false,
	}

	handler := NewHandler(lj, minLevel)
	return slog.New(handler), lj, nil
}

// ///////////////////////////////////////////////
// Helper Functions
// ///////////////////////////////////////////////

// Trace logs a message at LevelTrace.
func Trace(logger *slog.Logger, msg string, args ...any) {
	logger.Log(context.Background(), LevelTrace, msg, args...)
}

// Fail logs a message at LevelFail.
func Fail(logger *slog.Logger, msg string, args ...any) {
	logger.Log(context.Background(), LevelFail, msg, args...)
}

// ///////////////////////////////////////////////
// ReadTail
// ///////////////////////////////////////////////

// ReadTail returns the last n lines from the file at path.
// Returns an error if the file doesn't exist or can't be read.
func ReadTail(path string, lines int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]string, 0, lines)
	idx := 0

	for scanner.Scan() {
		if len(buf) < lines {
			buf = append(buf, scanner.Text())
		} else {
			buf[idx%lines] = scanner.Text()
		}
		idx++
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading log file: %w", err)
	}

	// Reorder the circular buffer so lines are in chronological order.
	if len(buf) < lines {
		return strings.Join(buf, "\n"), nil
	}
	start := idx % lines
	ordered := make([]string, 0, lines)
	ordered = append(ordered, buf[start:]...)
	ordered = append(ordered, buf[:start]...)
	return strings.Join(ordered, "\n"), nil
}

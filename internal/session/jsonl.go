package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"tools.zach/dev/agentcord/internal/config"
)

// ///////////////////////////////////////////////
// JSONL Types
// ///////////////////////////////////////////////

// JSONLData holds aggregated data parsed from a JSONL conversation file.
type JSONLData struct {
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	TurnCount           int64
	ToolUseCount        int64
	UniqueModels        []string
}

// jsonlEntry represents a single line in a JSONL conversation log.
// Only the fields needed for token aggregation and model detection are decoded.
type jsonlEntry struct {
	// Type is the entry kind (e.g. "assistant", "user").
	Type string `json:"type"`
	// Model is the model identifier that produced this entry.
	Model string `json:"model"`
	// Message holds the content blocks for assistant messages (for tool use counting).
	Message struct {
		Content []struct {
			Type string `json:"type"`
		} `json:"content"`
	} `json:"message"`
	// Usage holds the token consumption for this entry.
	Usage struct {
		// InputTokens is the number of input tokens consumed.
		InputTokens int64 `json:"input_tokens"`
		// OutputTokens is the number of output tokens produced.
		OutputTokens int64 `json:"output_tokens"`
		// CacheCreationInputTokens is the number of tokens used to create cache entries.
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		// CacheReadInputTokens is the number of tokens read from cache.
		CacheReadInputTokens int64 `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

// ///////////////////////////////////////////////
// JSONL Parsing
// ///////////////////////////////////////////////

// ParseJSONL reads a JSONL file, aggregates token counts, and extracts the latest model.
// Malformed lines are silently skipped.
func ParseJSONL(path string) (*JSONLData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening JSONL file: %w", err)
	}
	defer f.Close()

	data := &JSONLData{}
	seenModels := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry jsonlEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		data.InputTokens += entry.Usage.InputTokens
		data.OutputTokens += entry.Usage.OutputTokens
		data.CacheCreationTokens += entry.Usage.CacheCreationInputTokens
		data.CacheReadTokens += entry.Usage.CacheReadInputTokens

		if entry.Model != "" {
			data.Model = entry.Model
			if !seenModels[entry.Model] {
				seenModels[entry.Model] = true
				data.UniqueModels = append(data.UniqueModels, entry.Model)
			}
		}

		if entry.Type == "assistant" {
			data.TurnCount++
			for _, block := range entry.Message.Content {
				if block.Type == "tool_use" {
					data.ToolUseCount++
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning JSONL file: %w", err)
	}

	return data, nil
}

// ///////////////////////////////////////////////
// Cached JSONL Parsing
// ///////////////////////////////////////////////

// JSONLCache tracks parse state for incremental JSONL parsing.
// It stores the last known file size and accumulated data so that
// subsequent calls to ParseJSONLCached only scan new entries.
type JSONLCache struct {
	mu       sync.Mutex
	path     string
	lastSize int64
	lastData JSONLData
}

// NewJSONLCache creates a cache for incremental parsing of the given JSONL file.
func NewJSONLCache(path string) *JSONLCache {
	return &JSONLCache{path: path}
}

// ParseJSONLCached reads only the new portion of the JSONL file since the last
// call. If the file has shrunk (truncation/rotation), it falls back to a full scan.
func ParseJSONLCached(cache *JSONLCache) (*JSONLData, error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	f, err := os.Open(cache.path)
	if err != nil {
		return nil, fmt.Errorf("opening JSONL file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat JSONL file: %w", err)
	}

	currentSize := info.Size()

	// If the file shrunk, reset and do a full scan.
	if currentSize < cache.lastSize {
		cache.lastSize = 0
		cache.lastData = JSONLData{}
	}

	// If unchanged, return cached data.
	if currentSize == cache.lastSize {
		result := cache.lastData
		return &result, nil
	}

	// Seek to where we left off.
	if cache.lastSize > 0 {
		if _, err := f.Seek(cache.lastSize, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seeking JSONL file: %w", err)
		}
	}

	data := cache.lastData
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry jsonlEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		data.InputTokens += entry.Usage.InputTokens
		data.OutputTokens += entry.Usage.OutputTokens
		data.CacheCreationTokens += entry.Usage.CacheCreationInputTokens
		data.CacheReadTokens += entry.Usage.CacheReadInputTokens

		if entry.Model != "" {
			data.Model = entry.Model
			// Track unique models (simple linear scan — list is small)
			found := false
			for _, m := range data.UniqueModels {
				if m == entry.Model {
					found = true
					break
				}
			}
			if !found {
				data.UniqueModels = append(data.UniqueModels, entry.Model)
			}
		}

		if entry.Type == "assistant" {
			data.TurnCount++
			for _, block := range entry.Message.Content {
				if block.Type == "tool_use" {
					data.ToolUseCount++
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning JSONL file: %w", err)
	}

	cache.lastSize = currentSize
	cache.lastData = data

	result := data
	return &result, nil
}

// ///////////////////////////////////////////////
// JSONL Discovery
// ///////////////////////////////////////////////

// FindLatestJSONL finds the most recently modified .jsonl file in the given directory.
func FindLatestJSONL(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var latest string
	var latestTime int64

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if t := info.ModTime().UnixNano(); t > latestTime {
			latestTime = t
			latest = filepath.Join(dir, e.Name())
		}
	}

	if latest == "" {
		return "", fmt.Errorf("no .jsonl files found in %s", dir)
	}
	return latest, nil
}

// ///////////////////////////////////////////////
// Token Formatting
// ///////////////////////////////////////////////

// FormatTokenCount formats a token count as a human-readable string.
// Format can be "short" (1.5M, 234K, 500) or "full" (1,500,000).
func FormatTokenCount(tokens int64, format string) string {
	if format == "full" {
		return config.FormatWithCommas(tokens)
	}
	return config.FormatShort(tokens)
}

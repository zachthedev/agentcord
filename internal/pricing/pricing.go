// Package pricing fetches, caches, and calculates per-token costs for models.
//
// Pricing data can come from three source types: a remote URL (OpenRouter, LiteLLM,
// or Agentcord format), a local file, or static inline values defined in config.
// For URL and file sources, a double-fallback strategy applies: primary source,
// then on-disk cache. If both fail, no pricing data is available and costs show
// as $0.
//
// The [Fetch] function is the main entry point. It accepts a [SourceConfig] that
// describes the pricing source and returns a [PricingData] value ready for use
// with [PricingData.Calculate].
package pricing

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"tools.zach/dev/agentcord/internal/atomicfile"
	"tools.zach/dev/agentcord/internal/paths"
)

// httpClient is a lazily-initialized retryablehttp client shared across all
// pricing fetches. Initialized once via httpClientOnce.
var (
	httpClient     *retryablehttp.Client
	httpClientOnce sync.Once
)

// getHTTPClient returns the shared retryable HTTP client, initializing it on
// first call.
func getHTTPClient() *retryablehttp.Client {
	httpClientOnce.Do(func() {
		httpClient = retryablehttp.NewClient()
		httpClient.RetryMax = 2
		httpClient.HTTPClient.Timeout = 10 * time.Second
		httpClient.Logger = nil // suppress retryablehttp's default logging
	})
	return httpClient
}

// formatDefaultURLs maps format names to their default pricing API endpoints.
var formatDefaultURLs = map[string]string{
	"openrouter": "https://openrouter.ai/api/v1/models",
	"litellm":    "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json",
}

// ///////////////////////////////////////////////
// Types
// ///////////////////////////////////////////////

// SourceConfig describes where and how to load pricing data.
// Built from config.PricingConfig at startup.
type SourceConfig struct {
	Source string // "url", "file", "static"
	Format string // "openrouter", "litellm", "agentcord"
	URL    string // custom URL (overrides format default)
	File   string // local file path (for source = "file")

	// Static model prices (for source = "static").
	Models map[string]ModelPricing
}

// PricingData holds pricing information for Claude models.
type PricingData struct {
	Models map[string]ModelPricing `json:"models"`
}

// ModelPricing holds per-token pricing for a model.
type ModelPricing struct {
	InputPerToken  float64 `json:"input_per_token"`
	OutputPerToken float64 `json:"output_per_token"`
}

// Calculate computes the cost for a given model and token counts.
// Returns 0 if the model is not found in pricing data.
func (pd *PricingData) Calculate(model string, inputTokens, outputTokens int64) float64 {
	if pd == nil {
		return 0
	}
	mp, ok := pd.Models[model]
	if !ok {
		return 0
	}
	return float64(inputTokens)*mp.InputPerToken + float64(outputTokens)*mp.OutputPerToken
}

// ///////////////////////////////////////////////
// Public API
// ///////////////////////////////////////////////

// Fetch retrieves pricing data based on the source config.
//
// For "url" and "file" sources, uses double fallback: primary -> cache.
// For "static" source, returns the inline prices directly (no fallback).
//
// Returns nil with an error when both primary and cache sources fail.
// The returned error is non-nil when the data came from a cache fallback.
func Fetch(src SourceConfig, cacheDir string) (*PricingData, error) {
	switch src.Source {
	case "static":
		return fetchStatic(src.Models)
	case "file":
		return fetchWithFallback(cacheDir, func() (*PricingData, error) {
			return fetchFromFile(src.File, src.Format)
		})
	default: // "url"
		url := src.URL
		if url == "" {
			url = formatDefaultURLs[src.Format]
		}
		if url == "" {
			return nil, fmt.Errorf("no URL configured and format %q has no default URL", src.Format)
		}
		format := src.Format
		return fetchWithFallback(cacheDir, func() (*PricingData, error) {
			return fetchFromURL(url, format)
		})
	}
}

// ///////////////////////////////////////////////
// Fallback Logic
// ///////////////////////////////////////////////

// fetchWithFallback attempts the primary fetch, then cache.
// Returns nil with an error when both sources fail.
func fetchWithFallback(cacheDir string, primary func() (*PricingData, error)) (*PricingData, error) {
	pd, err := primary()
	if err == nil {
		if len(pd.Models) == 0 {
			return nil, fmt.Errorf("primary source returned empty pricing data")
		}
		if cacheErr := WritePricingCache(cacheDir, pd); cacheErr != nil {
			slog.Warn("failed to write pricing cache", "error", cacheErr)
		}
		return pd, nil
	}
	slog.Warn("failed to fetch pricing from primary source, trying cache", "error", err)

	pd, cacheErr := ReadPricingCache(cacheDir)
	if cacheErr == nil {
		return pd, fmt.Errorf("using cached pricing: primary fetch failed: %w", err)
	}
	slog.Warn("no pricing cache available", "error", cacheErr)

	return nil, fmt.Errorf("all pricing sources failed: primary: %w; cache: %w", err, cacheErr)
}

// fetchStatic builds PricingData from config-defined prices. No fallback.
func fetchStatic(models map[string]ModelPricing) (*PricingData, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("static pricing has no models defined")
	}
	pd := &PricingData{Models: make(map[string]ModelPricing, len(models))}
	for k, v := range models {
		pd.Models[k] = v
	}
	return pd, nil
}

// fetchFromURL downloads pricing data from the given URL and parses it.
func fetchFromURL(url, format string) (*PricingData, error) {
	const maxResponseBytes = 10 << 20 // 10 MiB

	client := getHTTPClient()

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, maxResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", url, err)
	}
	if int64(len(body)) > maxResponseBytes {
		return nil, fmt.Errorf("response from %s exceeds %d bytes", url, maxResponseBytes)
	}

	return parseBody(body, format)
}

// fetchFromFile reads pricing data from a local file and parses it.
func fetchFromFile(path, format string) (*PricingData, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pricing file %s: %w", path, err)
	}
	return parseBody(body, format)
}

// parseBody dispatches to the appropriate format adapter.
func parseBody(body []byte, format string) (*PricingData, error) {
	switch format {
	case "litellm":
		return parseLiteLLM(body)
	case "agentcord":
		return parseAgentcord(body)
	default: // "openrouter"
		return parseOpenRouter(body)
	}
}

// ///////////////////////////////////////////////
// Format Adapters
// ///////////////////////////////////////////////

// openRouterResponse represents the top-level JSON payload from the OpenRouter
// /api/v1/models endpoint.
type openRouterResponse struct {
	Data []openRouterModel `json:"data"`
}

// openRouterModel represents a single model entry in an OpenRouter response.
type openRouterModel struct {
	ID      string                 `json:"id"`
	Pricing openRouterModelPricing `json:"pricing"`
}

// openRouterModelPricing holds the per-token price strings from OpenRouter.
// Prices are transmitted as string-encoded floats (e.g. "0.000015").
type openRouterModelPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

// parseOpenRouter parses OpenRouter's {"data": [...]} format.
// Strips the provider/ prefix from model IDs (e.g. "anthropic/claude-opus-4-6" -> "claude-opus-4-6").
func parseOpenRouter(body []byte) (*PricingData, error) {
	var resp openRouterResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing openrouter response: %w", err)
	}

	pd := &PricingData{Models: make(map[string]ModelPricing)}
	for _, m := range resp.Data {
		input, errIn := strconv.ParseFloat(m.Pricing.Prompt, 64)
		output, errOut := strconv.ParseFloat(m.Pricing.Completion, 64)
		if errIn != nil || errOut != nil {
			slog.Debug("skipping model with unparseable pricing", "id", m.ID)
			continue
		}
		// Strip provider/ prefix (e.g. "anthropic/", "openai/", "google/")
		id := m.ID
		if idx := strings.IndexByte(id, '/'); idx >= 0 {
			id = id[idx+1:]
		}
		pd.Models[id] = ModelPricing{
			InputPerToken:  input,
			OutputPerToken: output,
		}
	}
	return pd, nil
}

// liteLLMModel represents a single entry in LiteLLM's flat model pricing map.
// The upstream format is {"model-id": {"input_cost_per_token": N, "output_cost_per_token": N, ...}}.
type liteLLMModel struct {
	InputCostPerToken  float64 `json:"input_cost_per_token"`
	OutputCostPerToken float64 `json:"output_cost_per_token"`
}

// parseLiteLLM parses LiteLLM's flat model pricing map.
// Includes all models with non-zero pricing.
func parseLiteLLM(body []byte) (*PricingData, error) {
	var raw map[string]liteLLMModel
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing litellm response: %w", err)
	}

	pd := &PricingData{Models: make(map[string]ModelPricing)}
	for id, m := range raw {
		if m.InputCostPerToken == 0 && m.OutputCostPerToken == 0 {
			continue
		}
		pd.Models[id] = ModelPricing{
			InputPerToken:  m.InputCostPerToken,
			OutputPerToken: m.OutputCostPerToken,
		}
	}
	return pd, nil
}

// parseAgentcord parses our canonical format: {"models": {"model-id": {...}}}.
// Zero transformation needed.
func parseAgentcord(body []byte) (*PricingData, error) {
	var pd PricingData
	if err := json.Unmarshal(body, &pd); err != nil {
		return nil, fmt.Errorf("parsing agentcord response: %w", err)
	}
	return &pd, nil
}

// ///////////////////////////////////////////////
// Cache
// ///////////////////////////////////////////////

// WritePricingCache writes pricing data to a cache file in the given directory.
func WritePricingCache(cacheDir string, data *PricingData) error {
	if data == nil {
		return fmt.Errorf("pricing data is nil")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("creating pricing cache directory: %w", err)
	}
	path := filepath.Join(cacheDir, paths.PricingCacheFile)
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshalling pricing data: %w", err)
	}
	return atomicfile.Write(path, b, 0o644)
}

// ReadPricingCache reads pricing data from a cache file in the given directory.
func ReadPricingCache(cacheDir string) (*PricingData, error) {
	path := filepath.Join(cacheDir, paths.PricingCacheFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading pricing cache: %w", err)
	}
	var pd PricingData
	if err := json.Unmarshal(b, &pd); err != nil {
		return nil, fmt.Errorf("parsing pricing cache: %w", err)
	}
	return &pd, nil
}

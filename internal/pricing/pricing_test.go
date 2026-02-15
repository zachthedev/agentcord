// Package pricing tests cover format parsing (OpenRouter, LiteLLM, Agentcord),
// the parseBody dispatch, static/file/URL source fetching, cost calculation,
// cache round-tripping, and embedded defaults loading.
package pricing

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// ///////////////////////////////////////////////
// OpenRouter Format
// ///////////////////////////////////////////////

func TestParseOpenRouterResponse(t *testing.T) {
	body := []byte(`{"data":[
		{"id":"anthropic/claude-opus-4-6","pricing":{"prompt":"0.000015","completion":"0.000075"}},
		{"id":"anthropic/claude-sonnet-4-5-20250929","pricing":{"prompt":"0.000003","completion":"0.000015"}},
		{"id":"anthropic/claude-haiku-4-5-20251001","pricing":{"prompt":"0.0000008","completion":"0.000004"}}
	]}`)

	pd, err := parseOpenRouter(body)
	if err != nil {
		t.Fatalf("parseOpenRouter: %v", err)
	}
	if len(pd.Models) != 3 {
		t.Fatalf("Models count = %d, want 3", len(pd.Models))
	}
	opus, ok := pd.Models["claude-opus-4-6"]
	if !ok {
		t.Fatal("missing claude-opus-4-6")
	}
	if opus.InputPerToken != 0.000015 {
		t.Errorf("opus InputPerToken = %v, want 0.000015", opus.InputPerToken)
	}
	if opus.OutputPerToken != 0.000075 {
		t.Errorf("opus OutputPerToken = %v, want 0.000075", opus.OutputPerToken)
	}
}

func TestParseOpenRouterMalformed(t *testing.T) {
	body := []byte(`{"data":[{"id":"anthropic/claude-opus-4-6","pricing":{"prompt":"not-a-number","completion":"0.000075"}}]}`)
	pd, err := parseOpenRouter(body)
	if err != nil {
		t.Fatalf("parseOpenRouter: %v", err)
	}
	if _, ok := pd.Models["claude-opus-4-6"]; ok {
		t.Error("expected malformed model to be skipped, but it was stored")
	}
}

func TestFilterAnthropicModels(t *testing.T) {
	body := []byte(`{"data":[
		{"id":"anthropic/claude-opus-4-6","pricing":{"prompt":"0.000015","completion":"0.000075"}},
		{"id":"openai/gpt-4","pricing":{"prompt":"0.00003","completion":"0.00006"}},
		{"id":"anthropic/claude-haiku-4-5-20251001","pricing":{"prompt":"0.0000008","completion":"0.000004"}},
		{"id":"meta-llama/llama-3-70b","pricing":{"prompt":"0.0000008","completion":"0.000001"}}
	]}`)

	pd, err := parseOpenRouter(body)
	if err != nil {
		t.Fatalf("parseOpenRouter: %v", err)
	}
	if len(pd.Models) != 4 {
		t.Errorf("Models count = %d, want 4 (all models with valid pricing)", len(pd.Models))
	}
	if _, ok := pd.Models["claude-opus-4-6"]; !ok {
		t.Error("missing claude-opus-4-6")
	}
	if _, ok := pd.Models["claude-haiku-4-5-20251001"]; !ok {
		t.Error("missing claude-haiku-4-5-20251001")
	}
	if _, ok := pd.Models["gpt-4"]; !ok {
		t.Error("missing gpt-4")
	}
	if _, ok := pd.Models["llama-3-70b"]; !ok {
		t.Error("missing llama-3-70b")
	}
}

func TestMapModelIDs(t *testing.T) {
	body := []byte(`{"data":[
		{"id":"anthropic/claude-opus-4-6","pricing":{"prompt":"0.000015","completion":"0.000075"}}
	]}`)

	pd, err := parseOpenRouter(body)
	if err != nil {
		t.Fatalf("parseOpenRouter: %v", err)
	}
	if _, ok := pd.Models["anthropic/claude-opus-4-6"]; ok {
		t.Error("model ID should have anthropic/ prefix stripped")
	}
	if _, ok := pd.Models["claude-opus-4-6"]; !ok {
		t.Error("expected claude-opus-4-6 without prefix")
	}
}

// ///////////////////////////////////////////////
// LiteLLM Format
// ///////////////////////////////////////////////

func TestParseLiteLLM(t *testing.T) {
	body := []byte(`{
		"claude-opus-4-6": {"input_cost_per_token": 0.000015, "output_cost_per_token": 0.000075},
		"claude-sonnet-4-5-20250929": {"input_cost_per_token": 0.000003, "output_cost_per_token": 0.000015},
		"gpt-4": {"input_cost_per_token": 0.00003, "output_cost_per_token": 0.00006}
	}`)

	pd, err := parseLiteLLM(body)
	if err != nil {
		t.Fatalf("parseLiteLLM: %v", err)
	}
	if len(pd.Models) != 3 {
		t.Errorf("Models count = %d, want 3 (all models with non-zero pricing)", len(pd.Models))
	}
	opus, ok := pd.Models["claude-opus-4-6"]
	if !ok {
		t.Fatal("missing claude-opus-4-6")
	}
	if opus.InputPerToken != 0.000015 {
		t.Errorf("opus InputPerToken = %v, want 0.000015", opus.InputPerToken)
	}
}

func TestParseLiteLLMSkipsZeroCost(t *testing.T) {
	body := []byte(`{
		"claude-opus-4-6": {"input_cost_per_token": 0.000015, "output_cost_per_token": 0.000075},
		"claude-unknown": {"input_cost_per_token": 0, "output_cost_per_token": 0}
	}`)

	pd, err := parseLiteLLM(body)
	if err != nil {
		t.Fatalf("parseLiteLLM: %v", err)
	}
	if len(pd.Models) != 1 {
		t.Errorf("Models count = %d, want 1 (zero-cost filtered)", len(pd.Models))
	}
}

// ///////////////////////////////////////////////
// Agentcord Format
// ///////////////////////////////////////////////

func TestParseAgentcord(t *testing.T) {
	body := []byte(`{"models":{"claude-opus-4-6":{"input_per_token":0.000015,"output_per_token":0.000075}}}`)

	pd, err := parseAgentcord(body)
	if err != nil {
		t.Fatalf("parseAgentcord: %v", err)
	}
	if len(pd.Models) != 1 {
		t.Errorf("Models count = %d, want 1", len(pd.Models))
	}
	opus, ok := pd.Models["claude-opus-4-6"]
	if !ok {
		t.Fatal("missing claude-opus-4-6")
	}
	if opus.InputPerToken != 0.000015 {
		t.Errorf("opus InputPerToken = %v, want 0.000015", opus.InputPerToken)
	}
}

// ///////////////////////////////////////////////
// parseBody Dispatch
// ///////////////////////////////////////////////

func TestParseBodyOpenRouter(t *testing.T) {
	body := []byte(`{"data":[{"id":"anthropic/claude-opus-4-6","pricing":{"prompt":"0.000015","completion":"0.000075"}}]}`)
	pd, err := parseBody(body, "openrouter")
	if err != nil {
		t.Fatalf("parseBody openrouter: %v", err)
	}
	if _, ok := pd.Models["claude-opus-4-6"]; !ok {
		t.Error("missing claude-opus-4-6")
	}
}

func TestParseBodyLiteLLM(t *testing.T) {
	body := []byte(`{"claude-opus-4-6": {"input_cost_per_token": 0.000015, "output_cost_per_token": 0.000075}}`)
	pd, err := parseBody(body, "litellm")
	if err != nil {
		t.Fatalf("parseBody litellm: %v", err)
	}
	if _, ok := pd.Models["claude-opus-4-6"]; !ok {
		t.Error("missing claude-opus-4-6")
	}
}

func TestParseBodyAgentcord(t *testing.T) {
	body := []byte(`{"models":{"claude-opus-4-6":{"input_per_token":0.000015,"output_per_token":0.000075}}}`)
	pd, err := parseBody(body, "agentcord")
	if err != nil {
		t.Fatalf("parseBody agentcord: %v", err)
	}
	if _, ok := pd.Models["claude-opus-4-6"]; !ok {
		t.Error("missing claude-opus-4-6")
	}
}

// ///////////////////////////////////////////////
// Static Source
// ///////////////////////////////////////////////

func TestFetchStatic(t *testing.T) {
	models := map[string]ModelPricing{
		"claude-opus-4-6": {InputPerToken: 0.000015, OutputPerToken: 0.000075},
	}
	pd, err := fetchStatic(models)
	if err != nil {
		t.Fatalf("fetchStatic: %v", err)
	}
	if len(pd.Models) != 1 {
		t.Errorf("Models count = %d, want 1", len(pd.Models))
	}
}

func TestFetchStaticEmpty(t *testing.T) {
	_, err := fetchStatic(nil)
	if err == nil {
		t.Error("expected error for empty static models")
	}
}

// ///////////////////////////////////////////////
// File Source
// ///////////////////////////////////////////////

func TestFetchFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pricing.json")
	os.WriteFile(path, []byte(`{"models":{"claude-opus-4-6":{"input_per_token":0.000015,"output_per_token":0.000075}}}`), 0o644)

	pd, err := fetchFromFile(path, "agentcord")
	if err != nil {
		t.Fatalf("fetchFromFile: %v", err)
	}
	if _, ok := pd.Models["claude-opus-4-6"]; !ok {
		t.Error("missing claude-opus-4-6")
	}
}

func TestFetchFromFileMissing(t *testing.T) {
	_, err := fetchFromFile("/nonexistent/file.json", "agentcord")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// ///////////////////////////////////////////////
// Cost Calculation
// ///////////////////////////////////////////////

func TestCalculateCost(t *testing.T) {
	pd := &PricingData{
		Models: map[string]ModelPricing{
			"claude-opus-4-6": {InputPerToken: 0.000015, OutputPerToken: 0.000075},
		},
	}

	cost := pd.Calculate("claude-opus-4-6", 1000, 500)
	want := 0.0525
	if cost != want {
		t.Errorf("Calculate = %v, want %v", cost, want)
	}
}

func TestCalculateCostNoPricing(t *testing.T) {
	pd := &PricingData{
		Models: map[string]ModelPricing{},
	}

	cost := pd.Calculate("claude-unknown-model", 1000, 500)
	if cost != 0 {
		t.Errorf("Calculate for missing model = %v, want 0", cost)
	}
}

// ///////////////////////////////////////////////
// Cache
// ///////////////////////////////////////////////

func TestWriteAndReadPricingCache(t *testing.T) {
	dir := t.TempDir()

	original := &PricingData{
		Models: map[string]ModelPricing{
			"claude-opus-4-6":            {InputPerToken: 0.000015, OutputPerToken: 0.000075},
			"claude-sonnet-4-5-20250929": {InputPerToken: 0.000003, OutputPerToken: 0.000015},
		},
	}

	if err := WritePricingCache(dir, original); err != nil {
		t.Fatalf("WritePricingCache: %v", err)
	}

	loaded, err := ReadPricingCache(dir)
	if err != nil {
		t.Fatalf("ReadPricingCache: %v", err)
	}

	if len(loaded.Models) != 2 {
		t.Errorf("loaded Models count = %d, want 2", len(loaded.Models))
	}

	opus := loaded.Models["claude-opus-4-6"]
	if opus.InputPerToken != 0.000015 {
		t.Errorf("loaded opus InputPerToken = %v, want 0.000015", opus.InputPerToken)
	}
}

func TestReadPricingCacheMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadPricingCache(dir)
	if err == nil {
		t.Error("expected error for missing cache, got nil")
	}
}

// ///////////////////////////////////////////////
// URL Fetch (via httptest)
// ///////////////////////////////////////////////

func TestFetchFromURLOpenRouter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[
			{"id":"anthropic/claude-opus-4-6","pricing":{"prompt":"0.000015","completion":"0.000075"}},
			{"id":"anthropic/claude-sonnet-4-5-20250929","pricing":{"prompt":"0.000003","completion":"0.000015"}}
		]}`))
	}))
	defer server.Close()

	pd, err := fetchFromURL(server.URL, "openrouter")
	if err != nil {
		t.Fatalf("fetchFromURL: %v", err)
	}
	if len(pd.Models) != 2 {
		t.Errorf("Models count = %d, want 2", len(pd.Models))
	}
}

func TestFetchFromURLLiteLLM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"claude-opus-4-6": {"input_cost_per_token": 0.000015, "output_cost_per_token": 0.000075}
		}`))
	}))
	defer server.Close()

	pd, err := fetchFromURL(server.URL, "litellm")
	if err != nil {
		t.Fatalf("fetchFromURL: %v", err)
	}
	if len(pd.Models) != 1 {
		t.Errorf("Models count = %d, want 1", len(pd.Models))
	}
}

// ///////////////////////////////////////////////
// Fetch (integration)
// ///////////////////////////////////////////////

func TestFetchStaticSource(t *testing.T) {
	src := SourceConfig{
		Source: "static",
		Models: map[string]ModelPricing{
			"claude-opus-4-6": {InputPerToken: 0.000015, OutputPerToken: 0.000075},
		},
	}
	pd, err := Fetch(src, t.TempDir())
	if err != nil {
		t.Fatalf("Fetch static: %v", err)
	}
	if _, ok := pd.Models["claude-opus-4-6"]; !ok {
		t.Error("missing claude-opus-4-6")
	}
}

func TestFetchFileSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prices.json")
	os.WriteFile(path, []byte(`{"models":{"claude-opus-4-6":{"input_per_token":0.000015,"output_per_token":0.000075}}}`), 0o644)

	src := SourceConfig{
		Source: "file",
		Format: "agentcord",
		File:   path,
	}
	pd, err := Fetch(src, dir)
	if err != nil {
		t.Fatalf("Fetch file: %v", err)
	}
	if _, ok := pd.Models["claude-opus-4-6"]; !ok {
		t.Error("missing claude-opus-4-6")
	}
}

func TestFetchURLSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"anthropic/claude-opus-4-6","pricing":{"prompt":"0.000015","completion":"0.000075"}}]}`))
	}))
	defer server.Close()

	src := SourceConfig{
		Source: "url",
		Format: "openrouter",
		URL:    server.URL,
	}
	pd, err := Fetch(src, t.TempDir())
	if err != nil {
		t.Fatalf("Fetch url: %v", err)
	}
	if _, ok := pd.Models["claude-opus-4-6"]; !ok {
		t.Error("missing claude-opus-4-6")
	}
}

// ///////////////////////////////////////////////
// Fallback Chain
// ///////////////////////////////////////////////

func TestFetchWithFallback_PrimaryFailsCacheFallback(t *testing.T) {
	cacheDir := t.TempDir()

	// Pre-populate the cache so the fallback finds it.
	cached := &PricingData{
		Models: map[string]ModelPricing{
			"claude-opus-4-6": {InputPerToken: 0.000015, OutputPerToken: 0.000075},
		},
	}
	if err := WritePricingCache(cacheDir, cached); err != nil {
		t.Fatalf("WritePricingCache: %v", err)
	}

	// Primary always fails.
	pd, err := fetchWithFallback(cacheDir, func() (*PricingData, error) {
		return nil, fmt.Errorf("simulated primary failure")
	})
	if pd == nil {
		t.Fatal("expected pricing data from cache fallback, got nil")
	}
	if err == nil {
		t.Fatal("expected non-nil error indicating fallback was used")
	}
	if _, ok := pd.Models["claude-opus-4-6"]; !ok {
		t.Error("cached model missing from fallback result")
	}
}

func TestFetchWithFallback_PrimaryAndCacheFailReturnsNil(t *testing.T) {
	// Use a temp dir with no cache file.
	cacheDir := t.TempDir()

	pd, err := fetchWithFallback(cacheDir, func() (*PricingData, error) {
		return nil, fmt.Errorf("simulated primary failure")
	})
	if pd != nil {
		t.Fatal("expected nil pricing data when both sources fail, got non-nil")
	}
	if err == nil {
		t.Fatal("expected non-nil error when both sources fail")
	}
}

// ///////////////////////////////////////////////
// fetchFromURL Non-200
// ///////////////////////////////////////////////

func TestFetchFromURL_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := fetchFromURL(server.URL, "openrouter")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestFetchFromURL_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := fetchFromURL(server.URL, "agentcord")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

// ///////////////////////////////////////////////
// Calculate Edge Cases
// ///////////////////////////////////////////////

func TestCalculate_NilPricingData(t *testing.T) {
	var pd *PricingData
	cost := pd.Calculate("claude-opus-4-6", 1000, 500)
	if cost != 0 {
		t.Errorf("Calculate on nil PricingData = %v, want 0", cost)
	}
}

func TestCalculate_ZeroTokens(t *testing.T) {
	pd := &PricingData{
		Models: map[string]ModelPricing{
			"claude-opus-4-6": {InputPerToken: 0.000015, OutputPerToken: 0.000075},
		},
	}
	cost := pd.Calculate("claude-opus-4-6", 0, 0)
	if cost != 0 {
		t.Errorf("Calculate with zero tokens = %v, want 0", cost)
	}
}

func TestCalculate_NegativeTokens(t *testing.T) {
	pd := &PricingData{
		Models: map[string]ModelPricing{
			"claude-opus-4-6": {InputPerToken: 0.000015, OutputPerToken: 0.000075},
		},
	}
	// Negative tokens produce a negative cost (caller's responsibility to validate).
	cost := pd.Calculate("claude-opus-4-6", -100, 0)
	if cost >= 0 {
		t.Errorf("Calculate with negative input tokens = %v, expected negative", cost)
	}
}

func TestCalculate_OnlyInputTokens(t *testing.T) {
	pd := &PricingData{
		Models: map[string]ModelPricing{
			"claude-opus-4-6": {InputPerToken: 0.000015, OutputPerToken: 0.000075},
		},
	}
	cost := pd.Calculate("claude-opus-4-6", 1000, 0)
	want := 0.015
	if math.Abs(cost-want) > 1e-12 {
		t.Errorf("Calculate = %v, want %v", cost, want)
	}
}

func TestCalculate_OnlyOutputTokens(t *testing.T) {
	pd := &PricingData{
		Models: map[string]ModelPricing{
			"claude-opus-4-6": {InputPerToken: 0.000015, OutputPerToken: 0.000075},
		},
	}
	cost := pd.Calculate("claude-opus-4-6", 0, 1000)
	want := 0.075
	if cost != want {
		t.Errorf("Calculate = %v, want %v", cost, want)
	}
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tools.zach/dev/agentcord/internal/config"
	"tools.zach/dev/agentcord/internal/discord"
	"tools.zach/dev/agentcord/internal/paths"
	"tools.zach/dev/agentcord/internal/pricing"
	"tools.zach/dev/agentcord/internal/session"
	"tools.zach/dev/agentcord/internal/tiers"
)

// ///////////////////////////////////////////////
// resolveVersion Tests
// ///////////////////////////////////////////////

func TestResolveVersionWithLdflags(t *testing.T) {
	// When version is set to something other than "dev", it should be returned as-is.
	original := version
	defer func() { version = original }()

	version = "1.2.3"
	got := resolveVersion()
	if got != "1.2.3" {
		t.Errorf("resolveVersion() = %q, want %q", got, "1.2.3")
	}
}

func TestResolveVersionDev(t *testing.T) {
	// When version is "dev", resolveVersion falls through to debug.ReadBuildInfo.
	// In test binaries, ReadBuildInfo may or may not return VCS info.
	// We just verify it returns a non-empty string.
	original := version
	defer func() { version = original }()

	version = "dev"
	got := resolveVersion()
	if got == "" {
		t.Error("resolveVersion() returned empty string")
	}
	// It should either be "dev" (no VCS info) or "dev+<hash>" or "dev+<hash>.dirty".
	if !strings.HasPrefix(got, "dev") {
		t.Errorf("resolveVersion() = %q, expected to start with 'dev'", got)
	}
}

// ///////////////////////////////////////////////
// toDiscordActivity Tests
// ///////////////////////////////////////////////

func TestToDiscordActivityNil(t *testing.T) {
	got := toDiscordActivity(nil)
	if got != nil {
		t.Errorf("toDiscordActivity(nil) = %+v, want nil", got)
	}
}

func TestToDiscordActivityFull(t *testing.T) {
	input := &session.Activity{
		Details: "Working on agentcord",
		State:   "Cost: $1.23",
		Timestamps: session.Timestamps{
			Start: 1707900000,
		},
		Assets: session.Assets{
			LargeImage: "app_icon",
			LargeText:  "Claude Code",
			SmallImage: "opus",
			SmallText:  "Opus 4.6",
		},
		Buttons: []session.Button{
			{Label: "View Repo", URL: "https://github.com/user/repo"},
			{Label: "Website", URL: "https://example.com"},
		},
	}

	got := toDiscordActivity(input)
	if got == nil {
		t.Fatal("toDiscordActivity returned nil for non-nil input")
		return
	}

	if got.Details != "Working on agentcord" {
		t.Errorf("Details = %q, want %q", got.Details, "Working on agentcord")
	}
	if got.State != "Cost: $1.23" {
		t.Errorf("State = %q, want %q", got.State, "Cost: $1.23")
	}

	if got.Timestamps == nil {
		t.Fatal("Timestamps is nil, want non-nil")
	}
	if got.Timestamps.Start != 1707900000 {
		t.Errorf("Timestamps.Start = %d, want %d", got.Timestamps.Start, 1707900000)
	}

	if got.Assets == nil {
		t.Fatal("Assets is nil, want non-nil")
	}
	if got.Assets.LargeImage != "app_icon" {
		t.Errorf("Assets.LargeImage = %q, want %q", got.Assets.LargeImage, "app_icon")
	}
	if got.Assets.LargeText != "Claude Code" {
		t.Errorf("Assets.LargeText = %q, want %q", got.Assets.LargeText, "Claude Code")
	}
	if got.Assets.SmallImage != "opus" {
		t.Errorf("Assets.SmallImage = %q, want %q", got.Assets.SmallImage, "opus")
	}
	if got.Assets.SmallText != "Opus 4.6" {
		t.Errorf("Assets.SmallText = %q, want %q", got.Assets.SmallText, "Opus 4.6")
	}

	if len(got.Buttons) != 2 {
		t.Fatalf("Buttons count = %d, want 2", len(got.Buttons))
	}
	if got.Buttons[0].Label != "View Repo" {
		t.Errorf("Buttons[0].Label = %q, want %q", got.Buttons[0].Label, "View Repo")
	}
	if got.Buttons[0].URL != "https://github.com/user/repo" {
		t.Errorf("Buttons[0].URL = %q, want %q", got.Buttons[0].URL, "https://github.com/user/repo")
	}
	if got.Buttons[1].Label != "Website" {
		t.Errorf("Buttons[1].Label = %q, want %q", got.Buttons[1].Label, "Website")
	}
}

func TestToDiscordActivity_EmptyStrings(t *testing.T) {
	input := &session.Activity{
		Details: "",
		State:   "",
		// Timestamps.Start is 0, all Assets fields empty, no Buttons.
	}

	got := toDiscordActivity(input)
	if got == nil {
		t.Fatal("toDiscordActivity returned nil for non-nil input")
	}
	if got.Details != "" {
		t.Errorf("Details = %q, want empty", got.Details)
	}
	if got.State != "" {
		t.Errorf("State = %q, want empty", got.State)
	}
	if got.Timestamps != nil {
		t.Errorf("Timestamps = %+v, want nil for zero Start", got.Timestamps)
	}
	if got.Assets != nil {
		t.Errorf("Assets = %+v, want nil for all-empty fields", got.Assets)
	}
	if len(got.Buttons) != 0 {
		t.Errorf("Buttons count = %d, want 0", len(got.Buttons))
	}
}

func TestToDiscordActivityNoTimestamps(t *testing.T) {
	input := &session.Activity{
		Details: "Working",
		State:   "Coding",
		// Timestamps.Start is 0 -> Timestamps should be nil in output.
	}

	got := toDiscordActivity(input)
	if got == nil {
		t.Fatal("toDiscordActivity returned nil")
		return
	}
	if got.Timestamps != nil {
		t.Errorf("Timestamps = %+v, want nil when Start is 0", got.Timestamps)
	}
}

func TestToDiscordActivityNoAssets(t *testing.T) {
	input := &session.Activity{
		Details: "Working",
		State:   "Coding",
		// All asset fields are empty.
	}

	got := toDiscordActivity(input)
	if got == nil {
		t.Fatal("toDiscordActivity returned nil")
		return
	}
	if got.Assets != nil {
		t.Errorf("Assets = %+v, want nil when all fields empty", got.Assets)
	}
}

func TestToDiscordActivityNoButtons(t *testing.T) {
	input := &session.Activity{
		Details: "Working",
		State:   "Coding",
	}

	got := toDiscordActivity(input)
	if got == nil {
		t.Fatal("toDiscordActivity returned nil")
		return
	}
	if len(got.Buttons) != 0 {
		t.Errorf("Buttons count = %d, want 0", len(got.Buttons))
	}
}

func TestToDiscordActivityPartialAssets(t *testing.T) {
	input := &session.Activity{
		Details: "Working",
		State:   "Coding",
		Assets: session.Assets{
			LargeImage: "icon",
			// Other fields empty.
		},
	}

	got := toDiscordActivity(input)
	if got == nil {
		t.Fatal("toDiscordActivity returned nil")
		return
	}
	if got.Assets == nil {
		t.Fatal("Assets should be non-nil when LargeImage is set")
	}
	if got.Assets.LargeImage != "icon" {
		t.Errorf("Assets.LargeImage = %q, want %q", got.Assets.LargeImage, "icon")
	}
}

// ///////////////////////////////////////////////
// buildActivityConfig Tests
// ///////////////////////////////////////////////

func TestBuildActivityConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	tierData := &tiers.TierData{
		DefaultIcon: "default",
		Clients: map[string]tiers.ClientTierConfig{
			"claude-code": {
				DefaultIcon: "claude",
				Tiers: map[string]tiers.TierConfig{
					"opus":   {},
					"sonnet": {},
					"haiku":  {},
				},
			},
		},
	}

	actCfg := buildActivityConfig(cfg, tierData, "claude-code")

	// Verify key mappings from config to activity config.
	if actCfg.DetailsFormat != cfg.Display.Details {
		t.Errorf("DetailsFormat = %q, want %q", actCfg.DetailsFormat, cfg.Display.Details)
	}
	if actCfg.StateFormat != cfg.Display.State {
		t.Errorf("StateFormat = %q, want %q", actCfg.StateFormat, cfg.Display.State)
	}
	if actCfg.DetailsNoBranchFormat != cfg.Display.DetailsNoBranch {
		t.Errorf("DetailsNoBranchFormat = %q, want %q", actCfg.DetailsNoBranchFormat, cfg.Display.DetailsNoBranch)
	}
	if actCfg.StateNoCostFormat != cfg.Display.StateNoCost {
		t.Errorf("StateNoCostFormat = %q, want %q", actCfg.StateNoCostFormat, cfg.Display.StateNoCost)
	}
	if actCfg.CostFormat != cfg.Display.Format.CostFormat {
		t.Errorf("CostFormat = %q, want %q", actCfg.CostFormat, cfg.Display.Format.CostFormat)
	}
	if actCfg.LargeImage != cfg.Display.Assets.LargeImage {
		t.Errorf("LargeImage = %q, want %q", actCfg.LargeImage, cfg.Display.Assets.LargeImage)
	}
	if actCfg.ShowModelIcon != cfg.Display.Assets.ShowModelIcon {
		t.Errorf("ShowModelIcon = %v, want %v", actCfg.ShowModelIcon, cfg.Display.Assets.ShowModelIcon)
	}
	if actCfg.ShowRepoButton != cfg.Display.Buttons.ShowRepoButton {
		t.Errorf("ShowRepoButton = %v, want %v", actCfg.ShowRepoButton, cfg.Display.Buttons.ShowRepoButton)
	}
	if actCfg.ShowCost != cfg.Behavior.ShowCost {
		t.Errorf("ShowCost = %v, want %v", actCfg.ShowCost, cfg.Behavior.ShowCost)
	}
	if actCfg.ShowBranch != cfg.Behavior.ShowBranch {
		t.Errorf("ShowBranch = %v, want %v", actCfg.ShowBranch, cfg.Behavior.ShowBranch)
	}
	if actCfg.TimestampMode != cfg.Display.Timestamps.Mode {
		t.Errorf("TimestampMode = %q, want %q", actCfg.TimestampMode, cfg.Display.Timestamps.Mode)
	}
	if actCfg.IdleMinutes != cfg.Behavior.PresenceIdleMinutes {
		t.Errorf("IdleMinutes = %d, want %d", actCfg.IdleMinutes, cfg.Behavior.PresenceIdleMinutes)
	}
	if actCfg.DefaultTierIcon != "claude" {
		t.Errorf("DefaultTierIcon = %q, want %q (per-client default)", actCfg.DefaultTierIcon, "claude")
	}

	// ModelTiers should contain the tier names from tierData.
	if len(actCfg.ModelTiers) != 3 {
		t.Errorf("ModelTiers count = %d, want 3", len(actCfg.ModelTiers))
	}
}

// ///////////////////////////////////////////////
// buildPricingSource Tests
// ///////////////////////////////////////////////

func TestBuildPricingSourceDefault(t *testing.T) {
	cfg := config.DefaultConfig()

	src := buildPricingSource(cfg)
	if src.Source != "url" {
		t.Errorf("Source = %q, want %q", src.Source, "url")
	}
	if src.Format != "openrouter" {
		t.Errorf("Format = %q, want %q", src.Format, "openrouter")
	}
	if src.Models != nil {
		t.Errorf("Models should be nil for default config, got %v", src.Models)
	}
}

func TestBuildPricingSourceWithModels(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Pricing.Source = "static"
	cfg.Pricing.Models = map[string]config.PricingModelConfig{
		"claude-opus-4-6": {
			InputPerToken:  0.000015,
			OutputPerToken: 0.000075,
		},
	}

	src := buildPricingSource(cfg)
	if src.Source != "static" {
		t.Errorf("Source = %q, want %q", src.Source, "static")
	}
	if len(src.Models) != 1 {
		t.Fatalf("Models count = %d, want 1", len(src.Models))
	}

	model, ok := src.Models["claude-opus-4-6"]
	if !ok {
		t.Fatal("expected claude-opus-4-6 in Models")
	}
	if model.InputPerToken != 0.000015 {
		t.Errorf("InputPerToken = %v, want %v", model.InputPerToken, 0.000015)
	}
	if model.OutputPerToken != 0.000075 {
		t.Errorf("OutputPerToken = %v, want %v", model.OutputPerToken, 0.000075)
	}
}

// ///////////////////////////////////////////////
// defaultDataDir Tests
// ///////////////////////////////////////////////

func TestDefaultDataDir(t *testing.T) {
	dir := defaultDataDir()
	if dir == "" {
		t.Fatal("defaultDataDir() returned empty string")
	}
	// filepath.Join normalizes separators for the current OS.
	suffix := ".agentcord"
	if !strings.HasSuffix(dir, suffix) {
		t.Errorf("defaultDataDir() = %q, want path ending in %q", dir, suffix)
	}
}

// Verify the discord types are correctly mapped.
var (
	_ discord.Activity
	_ discord.Timestamps
	_ discord.Assets
	_ discord.Button
	_ pricing.SourceConfig
	_ pricing.ModelPricing
)

// ///////////////////////////////////////////////
// pidToken Tests
// ///////////////////////////////////////////////

func TestPidToken_Unique(t *testing.T) {
	a := pidToken()
	b := pidToken()
	if a == b {
		t.Errorf("pidToken() returned the same value twice: %q", a)
	}
}

func TestPidToken_Length(t *testing.T) {
	tok := pidToken()
	if len(tok) != 16 {
		t.Errorf("pidToken() length = %d, want 16", len(tok))
	}
}

// ///////////////////////////////////////////////
// writePID / removePID Tests
// ///////////////////////////////////////////////

func TestWritePID_CreatesFile(t *testing.T) {
	dp := DataPaths{Root: t.TempDir()}
	token := pidToken()

	f, err := writePID(dp, token)
	if err != nil {
		t.Fatalf("writePID() error: %v", err)
	}
	defer func() {
		_ = unlockFile(f)
		f.Close()
	}()

	if _, err := os.Stat(dp.PID()); os.IsNotExist(err) {
		t.Fatal("PID file was not created")
	}
}

func TestWritePID_FileContainsPID(t *testing.T) {
	dp := DataPaths{Root: t.TempDir()}
	token := pidToken()

	f, err := writePID(dp, token)
	if err != nil {
		t.Fatalf("writePID() error: %v", err)
	}
	defer func() {
		_ = unlockFile(f)
		f.Close()
	}()

	// Read through the open handle — on Windows the lock prevents os.ReadFile.
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("Seek() error: %v", err)
	}
	data := make([]byte, 256)
	n, err := f.Read(data)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}

	expected := fmt.Sprintf("%d:%s", os.Getpid(), token)
	if string(data[:n]) != expected {
		t.Errorf("PID file content = %q, want %q", string(data[:n]), expected)
	}
}

func TestRemovePID_MatchingToken(t *testing.T) {
	dp := DataPaths{Root: t.TempDir()}
	token := pidToken()

	f, err := writePID(dp, token)
	if err != nil {
		t.Fatalf("writePID() error: %v", err)
	}

	removePID(dp, token, f)

	if _, err := os.Stat(dp.PID()); !os.IsNotExist(err) {
		t.Error("PID file should have been removed with matching token")
	}
}

func TestRemovePID_MismatchedToken(t *testing.T) {
	dp := DataPaths{Root: t.TempDir()}
	token := pidToken()

	f, err := writePID(dp, token)
	if err != nil {
		t.Fatalf("writePID() error: %v", err)
	}

	removePID(dp, "wrong-token", f)

	if _, err := os.Stat(dp.PID()); os.IsNotExist(err) {
		t.Error("PID file should NOT have been removed with mismatched token")
	}

	// Clean up the file that was intentionally kept.
	os.Remove(dp.PID())
}

func TestRemovePID_NilFile(t *testing.T) {
	dp := DataPaths{Root: t.TempDir()}

	// Should not panic with a nil file handle.
	removePID(dp, "any-token", nil)
}

// ///////////////////////////////////////////////
// checkStalePID Tests
// ///////////////////////////////////////////////

func TestCheckStalePID_NoFile(t *testing.T) {
	dp := DataPaths{Root: t.TempDir()}

	alive, pid := checkStalePID(dp)
	if alive {
		t.Error("checkStalePID() returned alive=true with no PID file")
	}
	if pid != 0 {
		t.Errorf("checkStalePID() pid = %d, want 0", pid)
	}
}

func TestCheckStalePID_StalePID(t *testing.T) {
	dp := DataPaths{Root: t.TempDir()}

	// Write a PID file without holding a lock — simulates a dead process.
	if err := os.WriteFile(dp.PID(), []byte("99999:staletoken"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	alive, pid := checkStalePID(dp)
	if alive {
		t.Error("checkStalePID() returned alive=true for stale PID")
	}
	if pid != 0 {
		t.Errorf("checkStalePID() pid = %d, want 0 for stale", pid)
	}

	// Stale PID file should have been cleaned up.
	if _, err := os.Stat(dp.PID()); !os.IsNotExist(err) {
		t.Error("stale PID file should have been removed")
	}
}

// ///////////////////////////////////////////////
// checkDaemonIdle Tests
// ///////////////////////////////////////////////

func TestCheckDaemonIdle_DisabledWhenZero(t *testing.T) {
	ls := &loopState{
		lastActivityTime: time.Now().Add(-1 * time.Hour),
	}
	if checkDaemonIdle(ls, 0) {
		t.Error("checkDaemonIdle() should return false when daemonIdleMinutes=0")
	}
}

func TestCheckDaemonIdle_NotIdleYet(t *testing.T) {
	ls := &loopState{
		lastActivityTime: time.Now(),
	}
	if checkDaemonIdle(ls, 30) {
		t.Error("checkDaemonIdle() should return false when activity is recent")
	}
}

func TestCheckDaemonIdle_IdleTimeout(t *testing.T) {
	ls := &loopState{
		lastActivityTime: time.Now().Add(-31 * time.Minute),
	}
	if !checkDaemonIdle(ls, 30) {
		t.Error("checkDaemonIdle() should return true when idle exceeds timeout")
	}
}

// ///////////////////////////////////////////////
// cleanupOrphanedSessions Tests
// ///////////////////////////////////////////////

func TestCleanupOrphanedSessions_RemovesOldFiles(t *testing.T) {
	sessDir := filepath.Join(t.TempDir(), "sessions")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	oldFile := filepath.Join(sessDir, "old"+paths.SessionExt)
	if err := os.WriteFile(oldFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	// Set mtime to 25 hours ago.
	past := time.Now().Add(-25 * time.Hour)
	os.Chtimes(oldFile, past, past)

	ls := &loopState{}
	cleanupOrphanedSessions(sessDir, 24*time.Hour, ls)

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("old session file should have been removed")
	}
}

func TestCleanupOrphanedSessions_KeepsRecentFiles(t *testing.T) {
	sessDir := filepath.Join(t.TempDir(), "sessions")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	recentFile := filepath.Join(sessDir, "recent"+paths.SessionExt)
	if err := os.WriteFile(recentFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	ls := &loopState{}
	cleanupOrphanedSessions(sessDir, 24*time.Hour, ls)

	if _, err := os.Stat(recentFile); os.IsNotExist(err) {
		t.Error("recent session file should NOT have been removed")
	}
}

func TestCleanupOrphanedSessions_RateLimited(t *testing.T) {
	sessDir := filepath.Join(t.TempDir(), "sessions")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	ls := &loopState{}

	// First call runs the cleanup and sets lastCleanup.
	cleanupOrphanedSessions(sessDir, 24*time.Hour, ls)
	firstCleanup := ls.lastCleanup

	// Create an old file after the first cleanup.
	oldFile := filepath.Join(sessDir, "old"+paths.SessionExt)
	if err := os.WriteFile(oldFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	past := time.Now().Add(-25 * time.Hour)
	os.Chtimes(oldFile, past, past)

	// Second call should be rate-limited (no-op) because < 10 minutes elapsed.
	cleanupOrphanedSessions(sessDir, 24*time.Hour, ls)

	if ls.lastCleanup != firstCleanup {
		t.Error("lastCleanup should not have been updated on rate-limited call")
	}

	// The old file should still exist because cleanup was skipped.
	if _, err := os.Stat(oldFile); os.IsNotExist(err) {
		t.Error("old file should still exist because cleanup was rate-limited")
	}
}

// ///////////////////////////////////////////////
// applyClientOverrides Tests
// ///////////////////////////////////////////////

func TestApplyClientOverrides_Details(t *testing.T) {
	actCfg := session.ActivityConfig{
		DetailsFormat: "Working on: {project} ({branch})",
	}
	clientCfg := config.ClientConfig{
		Details: "Editing in Cursor: {project}",
	}

	applyClientOverrides(&actCfg, clientCfg)

	if actCfg.DetailsFormat != "Editing in Cursor: {project}" {
		t.Errorf("DetailsFormat = %q, want %q", actCfg.DetailsFormat, "Editing in Cursor: {project}")
	}
}

func TestApplyClientOverrides_State(t *testing.T) {
	actCfg := session.ActivityConfig{
		StateFormat: "{model} · ~${cost} API value",
	}
	clientCfg := config.ClientConfig{
		State: "Using {model}",
	}

	applyClientOverrides(&actCfg, clientCfg)

	if actCfg.StateFormat != "Using {model}" {
		t.Errorf("StateFormat = %q, want %q", actCfg.StateFormat, "Using {model}")
	}
}

func TestApplyClientOverrides_AllFields(t *testing.T) {
	actCfg := session.ActivityConfig{
		DetailsFormat: "Working on: {project}",
		StateFormat:   "{model} · ~${cost}",
		LargeImage:    "app_icon",
		LargeText:     "Claude Code",
	}
	clientCfg := config.ClientConfig{
		LargeImage: "cursor_icon",
		LargeText:  "Cursor",
		Details:    "Cursor: {project}",
		State:      "Powered by {model}",
	}

	applyClientOverrides(&actCfg, clientCfg)

	if actCfg.LargeImage != "cursor_icon" {
		t.Errorf("LargeImage = %q, want %q", actCfg.LargeImage, "cursor_icon")
	}
	if actCfg.LargeText != "Cursor" {
		t.Errorf("LargeText = %q, want %q", actCfg.LargeText, "Cursor")
	}
	if actCfg.DetailsFormat != "Cursor: {project}" {
		t.Errorf("DetailsFormat = %q, want %q", actCfg.DetailsFormat, "Cursor: {project}")
	}
	if actCfg.StateFormat != "Powered by {model}" {
		t.Errorf("StateFormat = %q, want %q", actCfg.StateFormat, "Powered by {model}")
	}
}

func TestApplyClientOverrides_EmptyFieldsNoOp(t *testing.T) {
	actCfg := session.ActivityConfig{
		DetailsFormat: "original details",
		StateFormat:   "original state",
		LargeImage:    "original_image",
		LargeText:     "original_text",
	}
	clientCfg := config.ClientConfig{} // all empty

	applyClientOverrides(&actCfg, clientCfg)

	if actCfg.DetailsFormat != "original details" {
		t.Errorf("DetailsFormat changed unexpectedly to %q", actCfg.DetailsFormat)
	}
	if actCfg.StateFormat != "original state" {
		t.Errorf("StateFormat changed unexpectedly to %q", actCfg.StateFormat)
	}
	if actCfg.LargeImage != "original_image" {
		t.Errorf("LargeImage changed unexpectedly to %q", actCfg.LargeImage)
	}
	if actCfg.LargeText != "original_text" {
		t.Errorf("LargeText changed unexpectedly to %q", actCfg.LargeText)
	}
}

// ///////////////////////////////////////////////
// applyClientActivityOverrides Tests
// ///////////////////////////////////////////////

func TestApplyClientActivityOverrides_SmallImage(t *testing.T) {
	activity := &session.Activity{
		Assets: session.Assets{
			SmallImage: "opus",
			SmallText:  "Opus 4.6",
		},
	}
	clientCfg := config.ClientConfig{
		SmallImage: "cursor_small",
	}

	applyClientActivityOverrides(activity, clientCfg)

	if activity.Assets.SmallImage != "cursor_small" {
		t.Errorf("SmallImage = %q, want %q", activity.Assets.SmallImage, "cursor_small")
	}
	// SmallText should be unchanged since clientCfg.SmallText is empty.
	if activity.Assets.SmallText != "Opus 4.6" {
		t.Errorf("SmallText changed unexpectedly to %q", activity.Assets.SmallText)
	}
}

func TestApplyClientActivityOverrides_SmallText(t *testing.T) {
	activity := &session.Activity{
		Assets: session.Assets{
			SmallImage: "opus",
			SmallText:  "Opus 4.6",
		},
	}
	clientCfg := config.ClientConfig{
		SmallText: "Custom tooltip",
	}

	applyClientActivityOverrides(activity, clientCfg)

	if activity.Assets.SmallText != "Custom tooltip" {
		t.Errorf("SmallText = %q, want %q", activity.Assets.SmallText, "Custom tooltip")
	}
	if activity.Assets.SmallImage != "opus" {
		t.Errorf("SmallImage changed unexpectedly to %q", activity.Assets.SmallImage)
	}
}

func TestApplyClientActivityOverrides_NilActivity(t *testing.T) {
	clientCfg := config.ClientConfig{
		SmallImage: "cursor_small",
		SmallText:  "Cursor",
	}

	// Should not panic.
	applyClientActivityOverrides(nil, clientCfg)
}

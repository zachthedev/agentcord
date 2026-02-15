// Package main implements the Agentcord daemon, which reads Claude Code session
// state and publishes Discord Rich Presence updates.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	rootpkg "tools.zach/dev/agentcord"
	"tools.zach/dev/agentcord/internal/config"
	"tools.zach/dev/agentcord/internal/discord"
	"tools.zach/dev/agentcord/internal/logger"
	"tools.zach/dev/agentcord/internal/paths"
	"tools.zach/dev/agentcord/internal/pricing"
	"tools.zach/dev/agentcord/internal/session"
	"tools.zach/dev/agentcord/internal/tiers"
	"tools.zach/dev/agentcord/internal/update"
)

// ///////////////////////////////////////////////
// Version
// ///////////////////////////////////////////////

// version is set at build time via ldflags:
//   - goreleaser: -X main.version={{.Version}}  -> "0.1.0"
//   - make build: -X main.version=$(VERSION)    -> "0.0.0-dev+05ffee5"
//
// When ldflags are not set (bare go build), resolveVersion reads the VCS info
// that Go embeds automatically, so dev builds get a useful version string
// without needing git at runtime.
var version = "dev"

// resolveVersion returns the build version string. If [version] was set via
// ldflags at build time it is returned as-is; otherwise VCS revision and dirty
// state embedded by the Go toolchain are used to construct a "dev+<hash>" tag.
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	var revision string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if revision == "" {
		return version
	}
	hash := revision[:min(7, len(revision))]
	if dirty {
		return "dev+" + hash + ".dirty"
	}
	return "dev+" + hash
}

// ///////////////////////////////////////////////
// PID Management
// ///////////////////////////////////////////////

// pidToken generates a random 16-character hex token used to prove ownership
// of the PID file, so [removePID] only deletes the file if this instance wrote it.
func pidToken() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// writePID creates or opens the PID file at [DataPaths.PID], acquires an
// advisory file lock, and writes "PID:TOKEN" content. The returned file handle
// must be kept open for the lifetime of the daemon to hold the lock; pass it to
// [removePID] on shutdown.
func writePID(paths DataPaths, token string) (*os.File, error) {
	f, err := os.OpenFile(paths.PID(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open PID file: %w", err)
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("lock PID file: %w", err)
	}
	if err := f.Truncate(0); err != nil {
		_ = unlockFile(f)
		f.Close()
		return nil, fmt.Errorf("truncate PID file: %w", err)
	}
	content := fmt.Sprintf("%d:%s", os.Getpid(), token)
	if _, err := f.WriteString(content); err != nil {
		_ = unlockFile(f)
		f.Close()
		return nil, fmt.Errorf("write PID file: %w", err)
	}
	return f, nil
}

// removePID releases the advisory lock, closes the file handle, and removes the
// PID file only if the stored token matches, preventing accidental removal of a
// file owned by a different daemon instance.
func removePID(paths DataPaths, token string, f *os.File) {
	if f != nil {
		_ = unlockFile(f)
		f.Close()
	}
	data, err := os.ReadFile(paths.PID())
	if err != nil {
		return
	}
	parts := strings.SplitN(string(data), ":", 2)
	if len(parts) == 2 && parts[1] == token {
		os.Remove(paths.PID())
	}
}

// checkStalePID checks whether another daemon instance is running. It attempts
// to acquire the advisory lock on the PID file; if the lock fails, another
// instance holds it. If the lock succeeds, any previous instance is dead and
// the stale file is cleaned up.
func checkStalePID(paths DataPaths) (alive bool, pid int) {
	f, err := os.OpenFile(paths.PID(), os.O_RDWR, 0o600)
	if err != nil {
		return false, 0
	}

	if lockErr := lockFile(f); lockErr != nil {
		data, _ := os.ReadFile(paths.PID())
		f.Close()
		parts := strings.SplitN(string(data), ":", 2)
		if len(parts) >= 1 {
			if p, convErr := strconv.Atoi(parts[0]); convErr == nil {
				return true, p
			}
		}
		return true, 0
	}

	// Lock acquired -- previous instance is dead. Clean up stale file.
	_ = unlockFile(f)
	f.Close()
	os.Remove(paths.PID())
	return false, 0
}

// ///////////////////////////////////////////////
// Activity Mapping
// ///////////////////////////////////////////////

// toDiscordActivity converts a [session.Activity] into the [discord.Activity]
// wire type, copying fields and omitting empty optional sections.
func toDiscordActivity(a *session.Activity) *discord.Activity {
	if a == nil {
		return nil
	}
	da := &discord.Activity{
		Details: a.Details,
		State:   a.State,
	}
	if a.Timestamps.Start != 0 {
		da.Timestamps = &discord.Timestamps{
			Start: a.Timestamps.Start,
		}
	}
	if a.Assets.LargeImage != "" || a.Assets.LargeText != "" || a.Assets.SmallImage != "" || a.Assets.SmallText != "" {
		da.Assets = &discord.Assets{
			LargeImage: a.Assets.LargeImage,
			LargeText:  a.Assets.LargeText,
			SmallImage: a.Assets.SmallImage,
			SmallText:  a.Assets.SmallText,
		}
	}
	for _, b := range a.Buttons {
		da.Buttons = append(da.Buttons, discord.Button{
			Label: b.Label,
			URL:   b.URL,
		})
	}
	return da
}

// ///////////////////////////////////////////////
// Config Builders
// ///////////////////////////////////////////////

// buildActivityConfig assembles a [session.ActivityConfig] by mapping fields
// from the loaded [config.Config] and [tiers.TierData] into the flat struct
// that [session.BuildActivityWithData] expects. The client parameter selects
// which client's tier set to use; pass "" for initial setup (tiers will be
// resolved in processState when the active client is known).
func buildActivityConfig(cfg *config.Config, tierData *tiers.TierData, client string) session.ActivityConfig {
	return session.ActivityConfig{
		DetailsFormat:         cfg.Display.Details,
		StateFormat:           cfg.Display.State,
		DetailsNoBranchFormat: cfg.Display.DetailsNoBranch,
		StateNoCostFormat:     cfg.Display.StateNoCost,
		CostFormat:            cfg.Display.Format.CostFormat,
		TokenFormat:           cfg.Display.Format.TokenFormat,
		ModelFormat:           cfg.Display.Format.ModelName,
		LargeImage:            cfg.Display.Assets.LargeImage,
		LargeText:             cfg.Display.Assets.LargeText,
		ShowModelIcon:         cfg.Display.Assets.ShowModelIcon,
		ShowRepoButton:        cfg.Display.Buttons.ShowRepoButton,
		RepoButtonLabel:       cfg.Display.Buttons.RepoButtonLabel,
		CustomButtonLabel:     cfg.Display.Buttons.CustomButtonLabel,
		CustomButtonURL:       cfg.Display.Buttons.CustomButtonURL,
		ShowCost:              cfg.Behavior.ShowCost,
		ShowTokens:            cfg.Behavior.ShowTokens,
		ShowBranch:            cfg.Behavior.ShowBranch,
		TimestampMode:         cfg.Display.Timestamps.Mode,
		IdleMinutes:           cfg.Behavior.PresenceIdleMinutes,
		IgnoredPatterns:       cfg.Privacy.Ignore,
		ModelTiers:            tierData.TierNamesForClient(client),
		DefaultTierIcon:       tierData.DefaultIconForClient(client),
		CostShowThreshold:     cfg.Behavior.CostShowThreshold,
		TokensShowThreshold:   cfg.Behavior.TokensShowThreshold,
		IdleMode:              cfg.Behavior.IdleMode,
		IdleDetails:           cfg.Behavior.IdleDetails,
		IdleState:             cfg.Behavior.IdleState,
	}
}

// buildPricingSource creates a [pricing.SourceConfig] from the loaded
// [config.Config], including any user-defined per-model pricing overrides.
func buildPricingSource(cfg *config.Config) pricing.SourceConfig {
	src := pricing.SourceConfig{
		Source: cfg.Pricing.Source,
		Format: cfg.Pricing.Format,
		URL:    cfg.Pricing.URL,
		File:   cfg.Pricing.File,
	}
	if len(cfg.Pricing.Models) > 0 {
		src.Models = make(map[string]pricing.ModelPricing, len(cfg.Pricing.Models))
		for k, v := range cfg.Pricing.Models {
			src.Models[k] = pricing.ModelPricing{
				InputPerToken:  v.InputPerToken,
				OutputPerToken: v.OutputPerToken,
			}
		}
	}
	return src
}

// ///////////////////////////////////////////////
// Default Data Directory
// ///////////////////////////////////////////////

// defaultDataDir returns the platform default directory for Agentcord data,
// typically ~/.claude/agentcord. Falls back to ./.agentcord if the home directory
// cannot be determined.
func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".agentcord")
	}
	return filepath.Join(home, paths.DataDirRel)
}

// ///////////////////////////////////////////////
// Main
// ///////////////////////////////////////////////

func main() {
	dataDir := flag.String("data-dir", defaultDataDir(), "Data directory for config, state, and logs")
	flag.Parse()

	paths := DataPaths{Root: *dataDir}

	if err := os.MkdirAll(paths.Root, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: create data dir: %v\n", err)
		os.Exit(1)
	}

	if alive, pid := checkStalePID(paths); alive {
		fmt.Fprintf(os.Stderr, "daemon already running (pid %d)\n", pid)
		os.Exit(1)
	}

	if _, err := os.Stat(paths.Config()); os.IsNotExist(err) {
		if writeErr := os.WriteFile(paths.Config(), rootpkg.DefaultConfigTOML, 0o644); writeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to write default config: %v\n", writeErr)
		}
	}

	cfg, err := config.Load(paths.Root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: load config: %v\n", err)
		os.Exit(1)
	}

	logLevel := logger.ParseLevel(cfg.Log.Level)
	log, logCloser, err := logger.NewLogger(paths.Log(), logLevel, cfg.Log.MaxSizeMB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: init logger: %v\n", err)
		os.Exit(1)
	}
	defer logCloser.Close()
	slog.SetDefault(log)

	ver := resolveVersion()
	slog.Info("agentcord starting", "version", ver, "data_dir", paths.Root)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("update check panic", "error", r)
			}
		}()
		update.Check(ver)
	}()

	token := pidToken()
	pidFile, err := writePID(paths, token)
	if err != nil {
		slog.Error("failed to write PID file", "error", err)
		os.Exit(1)
	}
	defer removePID(paths, token, pidFile)

	pricingSrc := buildPricingSource(cfg)
	pricingData, pricingErr := pricing.Fetch(pricingSrc, paths.Root)
	if pricingErr != nil {
		slog.Warn("pricing fetch used fallback", "error", pricingErr)
	}
	if pricingData == nil {
		slog.Error("no pricing data available")
		os.Exit(1)
	}

	tierData, tierErr := tiers.Fetch(paths.Root)
	if tierErr != nil {
		slog.Warn("no tier data available, model icons disabled", "error", tierErr)
		tierData = &tiers.TierData{DefaultIcon: "default"}
	} else {
		slog.Info("loaded model tiers", "clients", len(tierData.Clients))
	}

	client := discord.NewClient(cfg.Discord.AppID)
	reconnectInterval := time.Duration(cfg.Behavior.ReconnectIntervalSeconds) * time.Second
	if err := connectWithRetry(client, reconnectInterval); err != nil {
		slog.Error("failed to connect to Discord", "error", err)
		os.Exit(1)
	}
	defer func() { client.Close() }()
	slog.Info("connected to Discord")

	watcher, err := session.NewDirWatcher(paths.Root)
	if err != nil {
		slog.Error("failed to create watcher", "error", err)
		os.Exit(1)
	}
	defer watcher.Close()

	if watcher.Polling() {
		slog.Info("using polling mode for file watching")
	}

	run(&client, watcher, cfg, pricingData, tierData, paths, reconnectInterval)
}

// ///////////////////////////////////////////////
// Connect with Retry
// ///////////////////////////////////////////////

// connectWithRetry attempts to connect the [discord.Client] up to 10 times,
// sleeping the given interval between failures. Returns nil on success or an
// error if all attempts are exhausted.
func connectWithRetry(client *discord.Client, interval time.Duration) error {
	const maxAttempts = 10

	for i := range maxAttempts {
		err := client.Connect()
		if err == nil {
			return nil
		}
		slog.Warn("Discord connect attempt failed", "attempt", i+1, "error", err)
		if i < maxAttempts-1 {
			time.Sleep(interval)
		}
	}
	return fmt.Errorf("failed to connect after %d attempts", maxAttempts)
}

// ///////////////////////////////////////////////
// Event Loop
// ///////////////////////////////////////////////

// loopState holds mutable state carried across iterations of the main event loop.
type loopState struct {
	// daemonStart records when the daemon process started, used for the "daemon"
	// timestamp mode so the elapsed timer reflects total uptime.
	daemonStart time.Time

	// lastActivityTime is the wall-clock time of the most recent non-nil activity,
	// used to evaluate the daemon idle timeout.
	lastActivityTime time.Time

	// lastActivity is the most recently published [session.Activity], retained so
	// the "last_activity" idle mode can keep showing it after the session ends.
	lastActivity *session.Activity

	// lastHash caches the hash of the last activity sent to Discord so duplicate
	// updates are suppressed.
	lastHash string

	// idleCleared tracks whether presence has already been cleared for the
	// current idle period, preventing repeated ClearActivity calls.
	idleCleared bool

	// lastCleanup records when orphaned session cleanup last ran, so it only
	// executes at most once every 10 minutes even though it is called on each
	// poll tick.
	lastCleanup time.Time

	// activeClient is the client identifier from the most recently processed state,
	// used to detect client switches that may require an AppID change.
	activeClient string

	// activeAppID is the Discord application ID currently in use, tracked to
	// detect when a client switch requires reconnecting with a different AppID.
	activeAppID string
}

// run is the main event loop. It listens for file-system change events from
// the [session.Watcher], a periodic poll ticker, and OS signals, dispatching
// each to [processState] to rebuild and publish Discord presence. The loop runs
// until an OS interrupt/terminate signal is received or the daemon idle timeout
// fires.
func run(
	client **discord.Client,
	watcher *session.Watcher,
	cfg *config.Config,
	pricingData *pricing.PricingData,
	tierData *tiers.TierData,
	dataPaths DataPaths,
	reconnectInterval time.Duration,
) {
	actCfg := buildActivityConfig(cfg, tierData, "")
	pollInterval := time.Duration(cfg.Behavior.PollIntervalSeconds) * time.Second
	daemonIdleMinutes := int64(cfg.Behavior.DaemonIdleMinutes)
	cleanupMaxAge := time.Duration(cfg.Behavior.SessionCleanupHours) * time.Hour

	pollTicker := time.NewTicker(pollInterval)
	defer pollTicker.Stop()

	sigCh := signalChannel()

	ls := loopState{
		daemonStart: time.Now(),
		activeAppID: cfg.Discord.AppID,
	}

	processState(client, &actCfg, cfg, pricingData, tierData, dataPaths, &ls, reconnectInterval)

	for {
		select {
		case <-sigCh:
			slog.Info("received shutdown signal")
			return

		case <-watcher.Events():
			processState(client, &actCfg, cfg, pricingData, tierData, dataPaths, &ls, reconnectInterval)

		case <-pollTicker.C:
			processState(client, &actCfg, cfg, pricingData, tierData, dataPaths, &ls, reconnectInterval)
			cleanupOrphanedSessions(dataPaths.Sessions(), cleanupMaxAge, &ls)
			if checkDaemonIdle(&ls, daemonIdleMinutes) {
				return
			}
			if err := handleReconnect(*client, &ls, reconnectInterval); err != nil {
				return
			}
		}
	}
}

// checkDaemonIdle returns true if the daemon should exit due to idle timeout.
// A zero or negative daemonIdleMinutes value disables the check.
func checkDaemonIdle(ls *loopState, daemonIdleMinutes int64) bool {
	if daemonIdleMinutes <= 0 || ls.lastActivityTime.IsZero() {
		return false
	}
	idleDuration := time.Since(ls.lastActivityTime)
	if idleDuration > time.Duration(daemonIdleMinutes)*time.Minute {
		slog.Info("daemon idle timeout, exiting", "idle_minutes", int(idleDuration.Minutes()))
		return true
	}
	return false
}

// handleReconnect checks whether the [discord.Client] is still connected and,
// if not, attempts to re-establish the connection via [connectWithRetry]. On
// success it resets the activity hash so the next [processState] call
// re-publishes presence. Returns an error if reconnection fails permanently.
func handleReconnect(client *discord.Client, ls *loopState, interval time.Duration) error {
	if client.Connected() {
		return nil
	}
	slog.Warn("Discord disconnected, attempting reconnect")
	if err := connectWithRetry(client, interval); err != nil {
		slog.Error("reconnect failed", "error", err)
		return err
	}
	slog.Info("reconnected to Discord")
	ls.lastHash = ""
	return nil
}

// ///////////////////////////////////////////////
// Multi-Client State Resolution
// ///////////////////////////////////////////////

// findLatestState scans the data directory for per-client state files
// (state.*.json), parses each, and returns the one with the most recent
// lastActivity timestamp. Falls back to the legacy state.json if no
// per-client files exist.
func findLatestState(dataDir string) (*session.State, error) {
	pattern := filepath.Join(dataDir, "state.*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob state files: %w", err)
	}

	var best *session.State
	for _, path := range matches {
		s, readErr := session.ReadState(path)
		if readErr != nil {
			if s == nil {
				slog.Debug("skipping unreadable state file", "path", path, "error", readErr)
				continue
			}
		}
		if best == nil || s.LastActivity > best.LastActivity {
			best = s
		}
	}

	if best != nil {
		return best, nil
	}

	// Fall back to legacy state.json
	legacyPath := filepath.Join(dataDir, paths.StateFile)
	return session.ReadState(legacyPath)
}

// resolveDiscordAppID returns the Discord application ID for the given client.
// It checks for a per-client override in the config, falling back to the
// global discord.app_id setting.
func resolveDiscordAppID(cfg *config.Config, client string) string {
	if cfg.Clients != nil {
		if cc, ok := cfg.Clients[client]; ok && cc.AppID != "" {
			return cc.AppID
		}
	}
	return cfg.Discord.AppID
}

// ///////////////////////////////////////////////
// State Processing
// ///////////////////////////////////////////////

// processState reads the most recently active client's state file, computes
// token costs, builds a [session.Activity], and pushes it to Discord when
// the activity hash has changed. If the active client changed and requires a
// different Discord AppID, it triggers a reconnect. Called on every watcher
// event and poll tick.
func processState(
	client **discord.Client,
	actCfg *session.ActivityConfig,
	cfg *config.Config,
	pricingData *pricing.PricingData,
	tierData *tiers.TierData,
	dataPaths DataPaths,
	ls *loopState,
	reconnectInterval time.Duration,
) {
	state, err := findLatestState(dataPaths.Root)
	if err != nil {
		if state == nil {
			slog.Debug("state file not readable", "error", err)
			return
		}
		slog.Debug("state file recovered with warning", "error", err)
	}

	// Check if the active client changed and requires a different AppID.
	newAppID := resolveDiscordAppID(cfg, state.Client)
	if ls.activeClient != state.Client && ls.activeAppID != "" && newAppID != ls.activeAppID {
		slog.Info("active client changed, reconnecting with new AppID",
			"old_client", ls.activeClient,
			"new_client", state.Client,
			"new_app_id", newAppID,
		)
		(*client).Close()
		*client = discord.NewClient(newAppID)
		if connErr := connectWithRetry(*client, reconnectInterval); connErr != nil {
			slog.Error("reconnect with new AppID failed", "error", connErr)
			return
		}
		ls.lastHash = ""
	}
	// Update per-client settings when the active client changes.
	if ls.activeClient != state.Client {
		actCfg.ModelTiers = tierData.TierNamesForClient(state.Client)
		actCfg.DefaultTierIcon = tierData.DefaultIconForClient(state.Client)
		actCfg.LargeImage = config.ClientIcon(state.Client)
	}
	ls.activeClient = state.Client
	ls.activeAppID = newAppID

	applyPrivacyOverrides(actCfg, cfg, state)

	if cfg.Clients != nil {
		if clientCfg, ok := cfg.Clients[state.Client]; ok {
			applyClientOverrides(actCfg, clientCfg)
		}
	}

	cost, totalTokens, model, jsonlData := resolveTokenData(cfg, pricingData, dataPaths)

	activity := session.BuildActivityWithData(state, *actCfg, cost, totalTokens, model, jsonlData)

	if cfg.Clients != nil {
		if clientCfg, ok := cfg.Clients[state.Client]; ok {
			applyClientActivityOverrides(activity, clientCfg)
		}
	}

	if activity != nil && cfg.Display.Timestamps.Mode == "daemon" {
		activity.Timestamps.Start = ls.daemonStart.Unix()
	}

	activity = handleIdleState(*client, actCfg, ls, activity)
	if activity == nil {
		return
	}

	ls.idleCleared = false
	ls.lastActivityTime = time.Now()
	ls.lastActivity = activity

	hash := activity.Hash()
	if hash == ls.lastHash {
		return
	}
	ls.lastHash = hash

	da := toDiscordActivity(activity)
	if err := (*client).SetActivity(da); err != nil {
		slog.Warn("failed to set activity", "error", err)
		return
	}
	slog.Debug("presence updated",
		"details", activity.Details,
		"state", activity.State,
	)
}

// applyClientOverrides applies per-client display overrides (e.g. different
// large image for Cursor or Windsurf) to the activity config.
func applyClientOverrides(actCfg *session.ActivityConfig, clientCfg config.ClientConfig) {
	if clientCfg.LargeImage != "" {
		actCfg.LargeImage = clientCfg.LargeImage
	}
	if clientCfg.LargeText != "" {
		actCfg.LargeText = clientCfg.LargeText
	}
	if clientCfg.Details != "" {
		actCfg.DetailsFormat = clientCfg.Details
	}
	if clientCfg.State != "" {
		actCfg.StateFormat = clientCfg.State
	}
}

// applyClientActivityOverrides applies per-client overrides that must be set
// on the built [session.Activity] rather than the config. SmallImage and
// SmallText are applied here because [session.BuildActivityWithData] may
// overwrite them via applyModelIcon when ShowModelIcon is true.
func applyClientActivityOverrides(a *session.Activity, clientCfg config.ClientConfig) {
	if a == nil {
		return
	}
	if clientCfg.SmallImage != "" {
		a.Assets.SmallImage = clientCfg.SmallImage
	}
	if clientCfg.SmallText != "" {
		a.Assets.SmallText = clientCfg.SmallText
	}
}

// applyPrivacyOverrides applies configured privacy rules to the activity config
// and session state, resolving project name aliases via [config.Config.ProjectName]
// and formatting the branch string via [config.Config.FormatBranch].
func applyPrivacyOverrides(actCfg *session.ActivityConfig, cfg *config.Config, state *session.State) {
	projectName := cfg.ProjectName(state.Project, state.CWD)
	actCfg.ProjectName = ""
	if projectName != state.Project {
		actCfg.ProjectName = projectName
	}
	state.Branch = cfg.FormatBranch(state.Branch)
}

// resolveTokenData finds the latest JSONL conversation log, parses it, and
// returns the computed dollar cost, total token count, and model identifier.
// Returns zero values if the file cannot be found or parsed.
func resolveTokenData(cfg *config.Config, pricingData *pricing.PricingData, paths DataPaths) (cost float64, totalTokens int64, model string, jsonlData *session.JSONLData) {
	latest, findErr := session.FindLatestJSONL(paths.Conversations())
	if findErr != nil {
		slog.Debug("no conversation log found", "error", findErr)
		return 0, 0, "", nil
	}
	data, parseErr := session.ParseJSONL(latest)
	if parseErr != nil {
		slog.Debug("failed to parse conversation log", "error", parseErr)
		return 0, 0, "", nil
	}
	model = data.Model
	totalTokens = data.InputTokens + data.OutputTokens
	if cfg.Behavior.ShowCost && model != "" {
		cost = pricingData.Calculate(model, data.InputTokens, data.OutputTokens)
	}
	return cost, totalTokens, model, data
}

// cleanupOrphanedSessions removes session marker files whose mtime is older
// than maxAge. It is rate-limited internally so calling it on every poll tick
// is cheap — the actual scan only runs if at least 10 minutes have passed
// since the last run.
func cleanupOrphanedSessions(sessDir string, maxAge time.Duration, ls *loopState) {
	const cleanupInterval = 10 * time.Minute
	if time.Since(ls.lastCleanup) < cleanupInterval {
		return
	}
	ls.lastCleanup = time.Now()

	entries, err := os.ReadDir(sessDir)
	if err != nil {
		return // directory may not exist yet
	}
	cutoff := time.Now().Add(-maxAge)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), paths.SessionExt) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			fp := filepath.Join(sessDir, e.Name())
			if rmErr := os.Remove(fp); rmErr == nil {
				slog.Debug("removed orphaned session marker", "file", e.Name())
			}
		}
	}
}

// handleIdleState implements idle detection. When the incoming activity is
// non-nil it is returned directly. When nil, behavior depends on the configured
// idle mode: "last_activity" returns the most recent activity from [loopState],
// while the default mode clears Discord presence once and returns nil.
func handleIdleState(client *discord.Client, actCfg *session.ActivityConfig, ls *loopState, activity *session.Activity) *session.Activity {
	if activity != nil {
		return activity
	}

	if actCfg.IdleMode == "last_activity" && ls.lastActivity != nil {
		return ls.lastActivity
	}

	if !ls.idleCleared {
		slog.Debug("clearing presence (idle/stopped)")
		if clearErr := client.ClearActivity(); clearErr != nil {
			slog.Warn("failed to clear activity", "error", clearErr)
		}
		ls.idleCleared = true
		ls.lastHash = ""
	}
	return nil
}

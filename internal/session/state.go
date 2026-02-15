// Package session manages Claude Code session state and Discord Rich Presence
// activity construction.
//
// The package provides three core capabilities:
//
//   - State persistence: reading, writing, and migrating the JSON state file
//     that tracks per-session metadata (project, branch, timestamps).
//   - Activity building: converting [State] and [ActivityConfig] into a Discord
//     Rich Presence [Activity], with support for idle detection, ignore patterns,
//     and template-based formatting.
//   - File watching: monitoring the state file for changes via [Watcher].
//
// The state file schema is versioned (see [CurrentVersion]) and supports forward
// and backward migration through [StateMigrations].
package session

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"tools.zach/dev/agentcord/internal/atomicfile"
	"tools.zach/dev/agentcord/internal/config"
	"tools.zach/dev/agentcord/internal/migrate"
)

// ///////////////////////////////////////////////
// State Types
// ///////////////////////////////////////////////

// CurrentVersion is the latest state file schema version.
const CurrentVersion = 1

// DefaultClient is the client identifier written to the state file by hooks.
const DefaultClient = "unknown"

// MigrationEntry wraps migrate.Migration for the state file.
type MigrationEntry = migrate.Migration

// StateMigrations holds registered migrations for the state file.
// Tests can append to this slice to inject test migrations.
var StateMigrations []MigrationEntry

// State represents the session state file schema. It is persisted as JSON on disk
// and updated by the daemon whenever it detects a change in the session.
type State struct {
	// Version is the schema version, used for migration. See [CurrentVersion].
	Version int `json:"$version"`
	// SessionID is the unique identifier for the session.
	SessionID string `json:"sessionId"`
	// SessionStart is the Unix timestamp when the session began.
	SessionStart int64 `json:"sessionStart"`
	// LastActivity is the Unix timestamp of the most recent session activity.
	LastActivity int64 `json:"lastActivity"`
	// Project is the project name derived from the working directory.
	Project string `json:"project"`
	// Branch is the current git branch name, or empty outside a git repo.
	Branch string `json:"branch"`
	// CWD is the absolute path to the session's working directory.
	CWD string `json:"cwd"`
	// GitRemoteURL is the HTTPS URL of the git remote origin, used for the repo button.
	GitRemoteURL string `json:"gitRemoteUrl"`
	// Client identifies which client wrote the state (e.g. "claude-code").
	Client string `json:"client"`
	// Stopped indicates whether the session has ended. When true, [BuildActivity] returns nil.
	Stopped bool `json:"stopped"`

	// Tool context

	// ToolName is the name of the tool currently being used (e.g. "Bash", "Edit", "Read").
	ToolName string `json:"toolName,omitempty"`
	// ToolTarget is the primary argument of the current tool (file path, command, pattern).
	ToolTarget string `json:"toolTarget,omitempty"`
	// ActiveFile is the most recently touched file path from a file-based tool.
	ActiveFile string `json:"activeFile,omitempty"`

	// Agent context

	// AgentState describes the agent's current phase: "thinking", "tool", "waiting", "idle".
	AgentState string `json:"agentState,omitempty"`
	// PermissionMode is the agent's permission setting (e.g. "plan", "acceptEdits", "dontAsk").
	PermissionMode string `json:"permissionMode,omitempty"`
	// HookEvent is the name of the last hook event that triggered this state update.
	HookEvent string `json:"hookEvent,omitempty"`
}

// ///////////////////////////////////////////////
// Activity Types
// ///////////////////////////////////////////////

// Activity represents a Discord Rich Presence activity payload. It is the final
// output of [BuildActivity] and [BuildActivityWithData], ready for transmission
// over the Discord IPC socket.
type Activity struct {
	// Details is the top line of text displayed in the presence (e.g. "Working on agentcord").
	Details string
	// State is the second line of text (e.g. "Cost: $0.42").
	State string
	// Timestamps controls the elapsed time display.
	Timestamps Timestamps
	// Assets holds the image keys and hover text.
	Assets Assets
	// Buttons is the list of clickable buttons (max 2 per Discord API).
	Buttons []Button
}

// Timestamps holds the start time for a Discord activity's elapsed timer.
type Timestamps struct {
	// Start is the Unix timestamp from which Discord calculates the "elapsed" display.
	Start int64
}

// Assets holds the image and text assets for a Discord activity.
type Assets struct {
	// LargeImage is the Discord asset key for the large (main) image.
	LargeImage string
	// LargeText is the tooltip shown when hovering over the large image.
	LargeText string
	// SmallImage is the Discord asset key for the small overlay image (model tier icon).
	SmallImage string
	// SmallText is the tooltip shown when hovering over the small image.
	SmallText string
}

// Button represents a clickable button on a Discord Rich Presence activity.
type Button struct {
	// Label is the button text shown to viewers.
	Label string
	// URL is the link opened when the button is clicked.
	URL string
}

// ActivityConfig captures the configuration fields needed for building a
// Discord Rich Presence [Activity]. Fields are typically populated from the
// user's TOML configuration via [config.Config].
type ActivityConfig struct {
	// DetailsFormat is the template for the activity details line (e.g. "Working on {project} ({branch})").
	DetailsFormat string
	// StateFormat is the template for the activity state line (e.g. "Cost: {cost}").
	StateFormat string
	// DetailsNoBranchFormat is the details template used when branch info is unavailable.
	DetailsNoBranchFormat string
	// StateNoCostFormat is the state template used when cost display is disabled or zero.
	StateNoCostFormat string

	// CostFormat is the fmt.Sprintf verb for formatting cost values (e.g. "%.2f").
	CostFormat string
	// TokenFormat controls token count display: "short" for abbreviated (e.g. "1.2k")
	// or "full" for the exact number.
	TokenFormat string
	// ModelFormat controls model name display: "short" (e.g. "Opus 4.6"),
	// "full" (e.g. "Claude Opus 4.6"), or "raw" (the model ID as-is).
	ModelFormat string

	// ProjectName overrides the project name derived from the working directory,
	// allowing users to hide their real project name for privacy.
	ProjectName string
	// IgnoredPatterns is a list of filepath.Match glob patterns. If the session CWD
	// matches any pattern, [BuildActivity] returns nil.
	IgnoredPatterns []string

	// LargeImage is the Discord asset key for the large activity image.
	LargeImage string
	// LargeText is the hover text for the large activity image.
	LargeText string
	// ShowModelIcon enables the small image overlay showing the model tier icon.
	ShowModelIcon bool

	// ShowRepoButton enables a clickable button linking to the git remote URL.
	ShowRepoButton bool
	// RepoButtonLabel is the text displayed on the repository button.
	RepoButtonLabel string
	// CustomButtonLabel is the text for an optional user-defined button.
	CustomButtonLabel string
	// CustomButtonURL is the URL for the custom button.
	CustomButtonURL string

	// ShowCost enables the cost display in the state line.
	ShowCost bool
	// ShowTokens enables token count display.
	ShowTokens bool
	// ShowBranch enables branch name in the details line.
	ShowBranch bool

	// TimestampMode controls the elapsed timer origin: "session" uses the session
	// start time, "daemon" uses the daemon start time.
	TimestampMode string
	// IdleMinutes is the number of minutes without activity before the session is
	// considered idle. Zero disables idle detection.
	IdleMinutes int

	// ModelTiers is the ordered list of tier names (e.g. ["opus", "sonnet", "haiku"])
	// used by extractModelTier to match model IDs to Discord asset keys.
	ModelTiers []string
	// DefaultTierIcon is the fallback Discord asset key when the model does not match
	// any entry in ModelTiers.
	DefaultTierIcon string

	// CostShowThreshold is the minimum cost value before it appears in the activity.
	// Costs below this threshold are treated as zero.
	CostShowThreshold float64
	// TokensShowThreshold is the minimum token count before it appears in the activity.
	// Token counts below this threshold are treated as zero.
	TokensShowThreshold int64

	// IdleMode controls behavior when the session is idle: "clear" removes the activity,
	// "idle_text" shows IdleDetails/IdleState, "last_activity" keeps the last non-idle activity.
	IdleMode string
	// IdleDetails is the details line shown when IdleMode is "idle_text".
	IdleDetails string
	// IdleState is the state line shown when IdleMode is "idle_text".
	IdleState string
}

// ///////////////////////////////////////////////
// Template Types
// ///////////////////////////////////////////////

// templateVars holds all variables available for template rendering in
// applyTemplate. Each field maps to a {name} placeholder in format strings.
type templateVars struct {
	// Project is the display name for the current project.
	Project string
	// Branch is the current git branch, or empty if unavailable.
	Branch string
	// Model is the raw model identifier (e.g. "claude-opus-4-6").
	Model string
	// Cost is the accumulated session cost in USD.
	Cost float64
	// Tokens is the total token count (input + output) for the session.
	Tokens int64

	// Agentic context
	Tool       string // current tool name
	ToolTarget string // tool target (file path, command, pattern)
	File       string // active file path
	AgentState string // thinking, tool, waiting, idle
	Permission string // permission mode
	Client     string // client display name

	// Extended token data
	InputTokens  int64
	OutputTokens int64
	CacheTokens  int64
	Turns        int64

	// Git extended
	GitOwner string
	GitRepo  string

	// Defaults
	DefaultModelFormat string
	DefaultCostFormat  string
	DefaultTokenFormat string
}

// ///////////////////////////////////////////////
// State I/O
// ///////////////////////////////////////////////

// ReadState reads and parses the state file at the given path.
// If the file contains corrupted JSON, it backs up to .corrupted, writes a fresh state,
// and returns the fresh state along with an error describing the corruption.
// If the file has a future version, it backs up to .v{N}.bak before normalizing.
func ReadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return recoverCorruptedState(path, data, err)
	}

	if s.Version == 0 {
		s.Version = 1
	}

	if err := applyMigrations(&s, data); err != nil {
		return nil, err
	}

	if s.Version > CurrentVersion {
		normalizeToCurrentVersion(&s, path, data)
	}

	return &s, nil
}

// recoverCorruptedState backs up a corrupted state file and returns a fresh state.
func recoverCorruptedState(path string, data []byte, parseErr error) (*State, error) {
	slog.Warn("corrupted state file, backing up", "path", path, "error", parseErr)

	corruptedPath := path + ".corrupted"
	if wErr := os.WriteFile(corruptedPath, data, 0o600); wErr != nil {
		slog.Warn("failed to write backup", "path", corruptedPath, "error", wErr)
	}

	s := State{Version: CurrentVersion}
	if sErr := saveState(path, &s); sErr != nil {
		slog.Warn("failed to save fresh state", "path", path, "error", sErr)
	}

	return &s, fmt.Errorf("corrupted state file (backed up to %s): %w", corruptedPath, parseErr)
}

// applyMigrations runs any registered state migrations if needed.
func applyMigrations(s *State, data []byte) error {
	if len(StateMigrations) == 0 || !migrate.NeedsMigration(s.Version, CurrentVersion, false, StateMigrations) {
		return nil
	}

	migrated, newVersion, migrateErr := migrate.Run(data, s.Version, StateMigrations)
	if migrateErr != nil {
		return fmt.Errorf("state migration failed: %w", migrateErr)
	}

	if err := json.Unmarshal(migrated, s); err != nil {
		return fmt.Errorf("unmarshal migrated state: %w", err)
	}

	s.Version = newVersion
	return nil
}

// normalizeToCurrentVersion backs up a future-version state file and normalizes it.
func normalizeToCurrentVersion(s *State, path string, data []byte) {
	slog.Warn("future state version detected, normalizing", "version", s.Version, "current", CurrentVersion)

	bakPath := fmt.Sprintf("%s.v%d.bak", path, s.Version)
	if wErr := os.WriteFile(bakPath, data, 0o600); wErr != nil {
		slog.Warn("failed to write backup", "path", bakPath, "error", wErr)
	}

	s.Version = CurrentVersion
	if sErr := saveState(path, s); sErr != nil {
		slog.Warn("failed to save normalized state", "path", path, "error", sErr)
	}
}

// PeekVersion does a partial JSON parse to extract the $version field.
// Returns 1 if the field is missing (0 -> 1 normalization).
// Returns an error if the JSON is unparseable.
func PeekVersion(data []byte) (int, error) {
	var partial struct {
		Version int `json:"$version"`
	}
	if err := json.Unmarshal(data, &partial); err != nil {
		return 0, fmt.Errorf("peeking version: %w", err)
	}
	if partial.Version == 0 {
		return 1, nil
	}
	return partial.Version, nil
}

// saveState marshals s as JSON and atomically writes it to path using
// [atomicfile.Write] to prevent partial writes on crash.
func saveState(path string, s *State) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshalling state: %w", err)
	}
	return atomicfile.Write(path, data, 0o600)
}

// ///////////////////////////////////////////////
// Activity Building
// ///////////////////////////////////////////////

// BuildActivity constructs a Discord activity from state and config.
// Returns nil when state.Stopped is true, CWD matches an ignore pattern, or the session is idle.
func BuildActivity(s *State, cfg ActivityConfig) *Activity {
	return BuildActivityWithData(s, cfg, 0, 0, "", nil)
}

// BuildActivityWithData constructs a Discord [Activity] with cost, token, and model
// data from the JSONL conversation log. Returns nil when the session is stopped,
// the CWD matches an ignore pattern, or the session is idle (depending on [ActivityConfig.IdleMode]).
func BuildActivityWithData(s *State, cfg ActivityConfig, cost float64, totalTokens int64, model string, jsonl *JSONLData) *Activity {
	if s.Stopped {
		return nil
	}

	if matchesIgnorePattern(cfg.IgnoredPatterns, s.CWD) {
		return nil
	}

	if isIdle(cfg, s.LastActivity) {
		return buildIdleActivity(s, cfg)
	}

	vars := buildTemplateVars(s, cfg, cost, totalTokens, model, jsonl)
	details := resolveDetails(cfg, vars)
	state := resolveState(cfg, vars)

	a := &Activity{
		Details: details,
		State:   state,
		Timestamps: Timestamps{
			Start: s.SessionStart,
		},
		Assets: Assets{
			LargeImage: cfg.LargeImage,
			LargeText:  cfg.LargeText,
		},
		Buttons: buildButtons(cfg, s.GitRemoteURL),
	}

	applyModelIcon(a, cfg, model)
	return a
}

// matchesIgnorePattern reports whether cwd matches any of the configured ignore
// patterns using [filepath.Match] semantics. This allows users to suppress
// Rich Presence for specific working directories (e.g. private repos).
func matchesIgnorePattern(patterns []string, cwd string) bool {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, cwd)
		if err != nil {
			slog.Warn("invalid ignore pattern", "pattern", pattern, "error", err)
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

// isIdle reports whether the session has been idle longer than cfg.IdleMinutes.
// Returns false when idle detection is disabled (IdleMinutes == 0).
func isIdle(cfg ActivityConfig, lastActivity int64) bool {
	return cfg.IdleMinutes > 0 && time.Now().Unix()-lastActivity > int64(cfg.IdleMinutes)*60
}

// buildTemplateVars prepares [templateVars] from the current [State] and
// [ActivityConfig], applying display thresholds. Cost and token values below
// their respective thresholds are zeroed so templates render cleanly.
func buildTemplateVars(s *State, cfg ActivityConfig, cost float64, totalTokens int64, model string, jsonl *JSONLData) templateVars {
	project := s.Project
	if cfg.ProjectName != "" {
		project = cfg.ProjectName
	}

	if cfg.CostShowThreshold > 0 && cost < cfg.CostShowThreshold {
		cost = 0
	}
	if cfg.TokensShowThreshold > 0 && totalTokens < cfg.TokensShowThreshold {
		totalTokens = 0
	}

	gitOwner, gitRepo := parseGitRemote(s.GitRemoteURL)

	var inputTokens, outputTokens, cacheTokens, turns int64
	if jsonl != nil {
		inputTokens = jsonl.InputTokens
		outputTokens = jsonl.OutputTokens
		cacheTokens = jsonl.CacheCreationTokens + jsonl.CacheReadTokens
		turns = jsonl.TurnCount
	}

	return templateVars{
		Project:            project,
		Branch:             s.Branch,
		Model:              model,
		Cost:               cost,
		Tokens:             totalTokens,
		Tool:               s.ToolName,
		ToolTarget:         s.ToolTarget,
		File:               s.ActiveFile,
		AgentState:         s.AgentState,
		Permission:         s.PermissionMode,
		Client:             config.ClientDisplayName(s.Client),
		InputTokens:        inputTokens,
		OutputTokens:       outputTokens,
		CacheTokens:        cacheTokens,
		Turns:              turns,
		GitOwner:           gitOwner,
		GitRepo:            gitRepo,
		DefaultModelFormat: cfg.ModelFormat,
		DefaultCostFormat:  cfg.CostFormat,
		DefaultTokenFormat: cfg.TokenFormat,
	}
}

// resolveDetails selects and renders the details template. It uses
// DetailsNoBranchFormat when the branch is empty or ShowBranch is false,
// otherwise DetailsFormat.
func resolveDetails(cfg ActivityConfig, vars templateVars) string {
	if vars.Branch == "" || !cfg.ShowBranch {
		return applyTemplate(cfg.DetailsNoBranchFormat, vars)
	}
	return applyTemplate(cfg.DetailsFormat, vars)
}

// resolveState selects and renders the state template. It uses StateNoCostFormat
// when cost display is disabled or the cost is zero, otherwise StateFormat.
func resolveState(cfg ActivityConfig, vars templateVars) string {
	if !cfg.ShowCost || vars.Cost == 0 {
		return applyTemplate(cfg.StateNoCostFormat, vars)
	}
	return applyTemplate(cfg.StateFormat, vars)
}

// applyModelIcon sets the small image and hover text on the [Activity] assets
// when ShowModelIcon is enabled and a model is known. The image key is derived
// from the model tier via extractModelTier.
func applyModelIcon(a *Activity, cfg ActivityConfig, model string) {
	if !cfg.ShowModelIcon || model == "" {
		return
	}
	tier := extractModelTier(model, cfg.ModelTiers, cfg.DefaultTierIcon)
	a.Assets.SmallImage = tier
	a.Assets.SmallText = config.FormatModelName(model, cfg.ModelFormat)
}

// buildButtons constructs the [Activity] button list from config and remote URL.
// Up to two buttons can be returned: the repo link button and a custom button.
func buildButtons(cfg ActivityConfig, remoteURL string) []Button {
	var buttons []Button
	if cfg.ShowRepoButton && remoteURL != "" {
		buttons = append(buttons, Button{
			Label: cfg.RepoButtonLabel,
			URL:   remoteURL,
		})
	}
	if cfg.CustomButtonLabel != "" && cfg.CustomButtonURL != "" {
		buttons = append(buttons, Button{
			Label: cfg.CustomButtonLabel,
			URL:   cfg.CustomButtonURL,
		})
	}
	return buttons
}

// buildIdleActivity returns an [Activity] for idle state based on [ActivityConfig.IdleMode].
// For "idle_text" it returns a static activity with the configured idle strings.
// For "last_activity" and "clear" (default) it returns nil; the caller is responsible
// for either preserving the previous activity or clearing the presence.
func buildIdleActivity(s *State, cfg ActivityConfig) *Activity {
	switch cfg.IdleMode {
	case "idle_text":
		return &Activity{
			Details: cfg.IdleDetails,
			State:   cfg.IdleState,
			Timestamps: Timestamps{
				Start: s.SessionStart,
			},
			Assets: Assets{
				LargeImage: cfg.LargeImage,
				LargeText:  cfg.LargeText,
			},
		}
	case "last_activity":
		// Caller handles this: returns the last known non-nil activity.
		// We signal this by returning nil so the caller knows to use the cached activity.
		return nil
	default: // "clear"
		return nil
	}
}

// ///////////////////////////////////////////////
// Activity Hashing
// ///////////////////////////////////////////////

// Hash returns a SHA-256 hex digest of the activity for dedup comparison.
// Returns an empty string for nil activities.
func (a *Activity) Hash() string {
	if a == nil {
		return ""
	}
	data, err := json.Marshal(a)
	if err != nil {
		slog.Warn("failed to hash activity", "error", err)
		return ""
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

// ///////////////////////////////////////////////
// Template Engine
// ///////////////////////////////////////////////

// formatVarRegex matches {name:format} patterns in template strings, where name
// is the variable identifier and format is passed to the variable's formatter.
var formatVarRegex = regexp.MustCompile(`\{(\w+):([^}]+)\}`)

// discordMaxLen is the maximum character length for Discord activity Details and State fields.
const discordMaxLen = 128

// applyTemplate renders a template string by replacing variable placeholders
// with formatted values from vars. It performs two passes: first replacing
// {var:format} patterns (explicit format), then {var} patterns (default format).
// The result is truncated to [discordMaxLen] characters.
func applyTemplate(tmpl string, vars templateVars) string {
	s := tmpl

	// First pass: replace {var:format} patterns with explicit format
	s = formatVarRegex.ReplaceAllStringFunc(s, func(match string) string {
		parts := formatVarRegex.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		name, format := parts[1], parts[2]
		return resolveVar(name, format, vars)
	})

	// Second pass: replace {var} patterns with default format
	s = strings.ReplaceAll(s, "{model}", resolveVar("model", vars.DefaultModelFormat, vars))
	s = strings.ReplaceAll(s, "{cost}", resolveVar("cost", vars.DefaultCostFormat, vars))
	s = strings.ReplaceAll(s, "{tokens}", resolveVar("tokens", vars.DefaultTokenFormat, vars))
	s = strings.ReplaceAll(s, "{project}", vars.Project)
	s = strings.ReplaceAll(s, "{branch}", vars.Branch)
	s = strings.ReplaceAll(s, "{tool}", vars.Tool)
	s = strings.ReplaceAll(s, "{tool_target}", vars.ToolTarget)
	s = strings.ReplaceAll(s, "{file}", resolveVar("file", "", vars))
	s = strings.ReplaceAll(s, "{agent_state}", vars.AgentState)
	s = strings.ReplaceAll(s, "{permission}", vars.Permission)
	s = strings.ReplaceAll(s, "{client}", resolveVar("client", "", vars))
	s = strings.ReplaceAll(s, "{input_tokens}", resolveVar("input_tokens", vars.DefaultTokenFormat, vars))
	s = strings.ReplaceAll(s, "{output_tokens}", resolveVar("output_tokens", vars.DefaultTokenFormat, vars))
	s = strings.ReplaceAll(s, "{cache_tokens}", resolveVar("cache_tokens", vars.DefaultTokenFormat, vars))
	s = strings.ReplaceAll(s, "{turns}", fmt.Sprintf("%d", vars.Turns))
	s = strings.ReplaceAll(s, "{git_owner}", vars.GitOwner)
	s = strings.ReplaceAll(s, "{git_repo}", vars.GitRepo)

	// Truncate to Discord's character limit
	if len(s) > discordMaxLen {
		s = s[:discordMaxLen-1] + "…"
	}

	return s
}

// resolveVar resolves a single template variable by name and format string.
// Unknown names are returned as the literal "{name}" placeholder.
func resolveVar(name, format string, vars templateVars) string {
	switch name {
	case "model":
		if vars.Model == "" {
			return ""
		}
		return config.FormatModelName(vars.Model, format)
	case "cost":
		if format == "" {
			format = "%.2f"
		}
		return "$" + formatFloat(vars.Cost, format)
	case "tokens":
		return FormatTokenCount(vars.Tokens, format)
	case "input_tokens":
		return FormatTokenCount(vars.InputTokens, format)
	case "output_tokens":
		return FormatTokenCount(vars.OutputTokens, format)
	case "cache_tokens":
		return FormatTokenCount(vars.CacheTokens, format)
	case "project":
		return vars.Project
	case "branch":
		return vars.Branch
	case "tool":
		return vars.Tool
	case "tool_target":
		return formatPath(vars.ToolTarget, format)
	case "file":
		return formatPath(vars.File, format)
	case "agent_state":
		return vars.AgentState
	case "permission":
		return vars.Permission
	case "client":
		return vars.Client
	case "git_owner":
		return vars.GitOwner
	case "git_repo":
		return vars.GitRepo
	case "turns":
		return fmt.Sprintf("%d", vars.Turns)
	default:
		return "{" + name + "}"
	}
}

// formatFloat formats a float64 using the given fmt.Sprintf format verb
// (e.g. "%.2f"). It is a thin wrapper used by resolveVar for cost formatting.
func formatFloat(val float64, format string) string {
	return fmt.Sprintf(format, val)
}

// formatPath formats a file path according to the given format.
// Supported formats: "basename" (file name only), "dir" (directory only),
// "ext" (file extension), empty/default (full path).
func formatPath(path, format string) string {
	if path == "" {
		return ""
	}
	switch format {
	case "basename":
		return filepath.Base(path)
	case "dir":
		return filepath.Dir(path)
	case "ext":
		return filepath.Ext(path)
	default:
		return path
	}
}

// gitRemoteRegex parses owner and repo from GitHub remote URLs.
var gitRemoteRegex = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/.]+)`)

// parseGitRemote extracts the owner and repo name from a git remote URL.
// Returns empty strings if the URL doesn't match a recognized pattern.
func parseGitRemote(url string) (owner, repo string) {
	m := gitRemoteRegex.FindStringSubmatch(url)
	if len(m) == 3 {
		return m[1], m[2]
	}
	return "", ""
}

// ///////////////////////////////////////////////
// Model Helpers
// ///////////////////////////////////////////////

// extractModelTier derives the tier name from a model ID by stripping known
// family prefixes and matching against tierList. For example:
//
//	"claude-opus-4-6"             -> "opus"
//	"claude-sonnet-4-5-20250929"  -> "sonnet"
//
// If no tier matches, it returns defaultIcon or "default" as a last resort.
// The returned string is used as a Discord asset key for the small image.
func extractModelTier(model string, tierList []string, defaultIcon string) string {
	stripped := model
	for _, prefix := range []string{"claude-", "gpt-", "gemini-", "o1-", "o3-"} {
		if strings.HasPrefix(model, prefix) {
			stripped = strings.TrimPrefix(model, prefix)
			break
		}
	}
	for _, tier := range tierList {
		if strings.HasPrefix(stripped, tier) {
			return tier
		}
	}
	if defaultIcon != "" {
		return defaultIcon
	}
	return "default"
}

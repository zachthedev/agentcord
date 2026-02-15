// Package config provides configuration loading and defaults for the Agentcord daemon.
//
// Configuration is loaded from a TOML file in the user's data directory.
// The package handles Discord presence settings, display formatting,
// privacy controls, and daemon behavior with sensible defaults.
package config

//go:generate go run ../../cmd/genconfig

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/bmatcuk/doublestar/v4"
	"tools.zach/dev/agentcord/internal/atomicfile"
	"tools.zach/dev/agentcord/internal/migrate"
	"tools.zach/dev/agentcord/internal/paths"
)

// DefaultDiscordAppID is the official Agentcord Discord application ID.
const DefaultDiscordAppID = "1472319454909173911"

// ClientDefaults holds the built-in display defaults for a known client tool.
type ClientDefaults struct {
	// DisplayName is the human-readable name shown in tooltips and templates.
	DisplayName string
	// Icon is the Discord asset key for this client's large image.
	Icon string
}

// knownClients maps client IDs to their built-in display defaults.
// Unknown clients still work — the ID is title-cased for the display name
// and "default" is used as the icon.
var knownClients = map[string]ClientDefaults{
	"claude-code": {DisplayName: "Claude Code", Icon: "app_icon_claude_code"},
}

// clientIDRegex validates client identifiers: lowercase alphanumeric with hyphens.
var clientIDRegex = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// ValidateClientID reports whether id is a valid client identifier.
func ValidateClientID(id string) bool {
	return len(id) <= 48 && clientIDRegex.MatchString(id)
}

// ClientDisplayName returns the human-readable name for a client ID.
// Known clients return their registered name; unknown clients get title-cased.
func ClientDisplayName(id string) string {
	if d, ok := knownClients[id]; ok {
		return d.DisplayName
	}
	// Title-case the client ID: "my-tool" -> "My Tool"
	parts := strings.Split(id, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// ClientIcon returns the Discord large image asset key for a client ID.
// Known clients return their registered icon (e.g. "app_icon_claude_code");
// unknown clients return "app_icon".
func ClientIcon(id string) string {
	if d, ok := knownClients[id]; ok {
		return d.Icon
	}
	return "app_icon"
}

// ///////////////////////////////////////////////
// Configuration Types
// ///////////////////////////////////////////////

// Config represents the top-level application configuration.
type Config struct {
	// Version is the config schema version used for migrations.
	Version int `toml:"version"`
	// Discord holds Discord connection settings.
	Discord DiscordConfig `toml:"discord"`
	// Display holds presence display settings.
	Display DisplayConfig `toml:"display"`
	// Privacy holds privacy and project-hiding settings.
	Privacy PrivacyConfig `toml:"privacy"`
	// Behavior holds daemon behavior and idle settings.
	Behavior BehaviorConfig `toml:"behavior"`
	// Pricing holds model pricing data source settings.
	Pricing PricingConfig `toml:"pricing"`
	// Log holds logging settings.
	Log LogConfig `toml:"log"`
	// Clients holds per-client overrides keyed by client name (e.g. "cursor", "windsurf").
	Clients map[string]ClientConfig `toml:"clients,omitempty"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	// Level is the minimum log level (trace, debug, info, warn, error).
	Level string `toml:"level"`
	// MaxSizeMB is the maximum log file size in megabytes before rotation.
	MaxSizeMB int `toml:"max_size_mb"`
}

// ClientConfig holds per-client display overrides (e.g. for Cursor or Windsurf).
type ClientConfig struct {
	// LargeImage overrides the Discord large image asset key for this client.
	LargeImage string `toml:"large_image,omitempty"`
	// LargeText overrides the Discord large image tooltip for this client.
	LargeText string `toml:"large_text,omitempty"`
	// AppID overrides the Discord application ID for this client.
	AppID string `toml:"app_id,omitempty"`
	// Details overrides the details format template for this client.
	Details string `toml:"details,omitempty"`
	// State overrides the state format template for this client.
	State string `toml:"state,omitempty"`
	// SmallImage overrides the small image asset key for this client.
	SmallImage string `toml:"small_image,omitempty"`
	// SmallText overrides the small image tooltip for this client.
	SmallText string `toml:"small_text,omitempty"`
}

// DiscordConfig holds Discord connection settings.
type DiscordConfig struct {
	// AppID is the Discord application ID for Rich Presence.
	AppID string `toml:"app_id"`
}

// DisplayConfig holds presence display settings.
type DisplayConfig struct {
	// Details is the format string for the top line (supports {project}, {branch}).
	Details string `toml:"details"`
	// State is the format string for the bottom line (supports {model}, {cost}, {tokens}).
	State string `toml:"state"`
	// DetailsNoBranch is the details template used when no git branch is available.
	DetailsNoBranch string `toml:"details_no_branch"`
	// StateNoCost is the state template used when cost data is unavailable.
	StateNoCost string `toml:"state_no_cost"`
	// Assets holds Discord Rich Presence asset settings.
	Assets AssetsConfig `toml:"assets"`
	// Buttons holds Discord Rich Presence button settings.
	Buttons ButtonsConfig `toml:"buttons"`
	// Format holds formatting preferences for model names, costs, and branches.
	Format FormatConfig `toml:"format"`
	// Timestamps holds timestamp display settings.
	Timestamps TimestampsConfig `toml:"timestamps"`
}

// AssetsConfig holds Discord Rich Presence asset settings.
type AssetsConfig struct {
	// LargeImage is the key for the large image asset in Discord.
	LargeImage string `toml:"large_image"`
	// LargeText is the tooltip text for the large image.
	LargeText string `toml:"large_text"`
	// ShowModelIcon enables the small image overlay showing the active model tier.
	ShowModelIcon bool `toml:"show_model_icon"`
}

// ButtonsConfig holds Discord Rich Presence button settings.
type ButtonsConfig struct {
	// ShowRepoButton enables the auto-detected repository button.
	ShowRepoButton bool `toml:"show_repo_button"`
	// RepoButtonLabel is the label text for the repository button.
	RepoButtonLabel string `toml:"repo_button_label"`
	// CustomButtonLabel is the label for an optional custom button.
	CustomButtonLabel string `toml:"custom_button_label,omitempty"`
	// CustomButtonURL is the URL for the optional custom button.
	CustomButtonURL string `toml:"custom_button_url,omitempty"`
}

// FormatConfig holds formatting preferences for model names, costs, tokens, and branches.
type FormatConfig struct {
	// ModelName controls model name formatting: "short", "full", or "raw".
	ModelName string `toml:"model_name"`
	// CostFormat is a Go fmt-style format string for cost display (e.g. "%.2f").
	CostFormat string `toml:"cost_format"`
	// TokenFormat controls token count formatting: "short" or "full".
	TokenFormat string `toml:"token_format"`
	// Branch controls branch display: "show", "hide", or "hide_default".
	Branch string `toml:"branch"`
	// DefaultBranches lists branches hidden when Branch is "hide_default".
	DefaultBranches []string `toml:"default_branches"`
}

// TimestampsConfig holds timestamp display settings.
type TimestampsConfig struct {
	// Mode controls what the elapsed timer tracks: "session", "elapsed", or "none".
	Mode string `toml:"mode"`
}

// PrivacyOverride applies privacy settings to projects matching a glob pattern.
type PrivacyOverride struct {
	// Pattern is a glob pattern matched against the project's working directory.
	Pattern string `toml:"pattern"`
	// HideProjectName replaces the project name with HiddenText when true.
	HideProjectName bool `toml:"hide_project_name"`
	// HiddenText is the replacement text shown when HideProjectName is true.
	HiddenText string `toml:"hidden_text"`
}

// PrivacyConfig holds privacy settings for hiding project names and suppressing presence.
type PrivacyConfig struct {
	// HideProjectName replaces all project names with HiddenProjectText.
	HideProjectName bool `toml:"hide_project_name"`
	// HiddenProjectText is the generic text shown when HideProjectName is true.
	HiddenProjectText string `toml:"hidden_project_text"`
	// Ignore is a list of glob patterns for directories where presence is suppressed.
	Ignore []string `toml:"ignore"`
	// Overrides provides per-project privacy settings matched by glob pattern.
	Overrides []PrivacyOverride `toml:"overrides"`
}

// BehaviorConfig holds daemon behavior settings.
type BehaviorConfig struct {
	// ShowCost enables cost display in the state line.
	ShowCost bool `toml:"show_cost"`
	// ShowTokens enables token count display.
	ShowTokens bool `toml:"show_tokens"`
	// ShowBranch enables git branch display in the details line.
	ShowBranch bool `toml:"show_branch"`
	// UseStatusline reads session data from the Claude Code statusline instead of state.json.
	UseStatusline bool `toml:"use_statusline"`
	// CostShowThreshold is the minimum cost value before cost is displayed (0 = always).
	CostShowThreshold float64 `toml:"cost_show_threshold"`
	// TokensShowThreshold is the minimum token count before tokens are displayed (0 = always).
	TokensShowThreshold int64 `toml:"tokens_show_threshold"`
	// IdleMode controls idle behavior: "clear", "idle_text", or "last_activity".
	IdleMode string `toml:"idle_mode"`
	// IdleDetails is the details line shown in "idle_text" mode.
	IdleDetails string `toml:"idle_details"`
	// IdleState is the state line shown in "idle_text" mode.
	IdleState string `toml:"idle_state"`
	// PresenceIdleMinutes is the inactivity duration before presence is hidden.
	PresenceIdleMinutes int `toml:"presence_idle_minutes"`
	// DaemonIdleMinutes is the inactivity duration before the daemon exits.
	DaemonIdleMinutes int `toml:"daemon_idle_minutes"`
	// PollIntervalSeconds is the fallback polling interval for state changes.
	PollIntervalSeconds int `toml:"poll_interval_seconds"`
	// ReconnectIntervalSeconds is the Discord reconnect interval.
	ReconnectIntervalSeconds int `toml:"reconnect_interval_seconds"`
	// SessionCleanupHours is how old a session marker must be before it is removed.
	SessionCleanupHours int `toml:"session_cleanup_hours"`
}

// PricingConfig holds settings for where and how pricing data is loaded.
type PricingConfig struct {
	// Source selects the pricing data source: "url", "file", or "static".
	Source string `toml:"source"`
	// Format selects the response parser: "openrouter", "litellm", or "agentcord".
	Format string `toml:"format"`
	// URL is a custom pricing endpoint (overrides the format's default URL).
	URL string `toml:"url,omitempty"`
	// File is the local file path for source "file".
	File string `toml:"file,omitempty"`
	// Models holds inline per-model pricing for source "static".
	Models map[string]PricingModelConfig `toml:"models,omitempty"`
}

// PricingModelConfig holds per-token pricing for a model in static config.
type PricingModelConfig struct {
	// InputPerToken is the cost per input token in USD.
	InputPerToken float64 `toml:"input_per_token"`
	// OutputPerToken is the cost per output token in USD.
	OutputPerToken float64 `toml:"output_per_token"`
}

// ///////////////////////////////////////////////
// Default Configuration
// ///////////////////////////////////////////////

// DefaultConfig returns a Config populated with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Version: migrate.Config.CurrentVersion,
		Discord: DiscordConfig{
			AppID: DefaultDiscordAppID,
		},
		Display: DisplayConfig{
			Details:         "Working on: {project} ({branch})",
			State:           "{model} · ~${cost} API value",
			DetailsNoBranch: "Working on: {project}",
			StateNoCost:     "{model} · {tokens} tokens",
			Assets: AssetsConfig{
				LargeImage:    "app_icon",
				LargeText:     "Agentcord",
				ShowModelIcon: true,
			},
			Buttons: ButtonsConfig{
				ShowRepoButton:  true,
				RepoButtonLabel: "View Repository",
			},
			Format: FormatConfig{
				ModelName:       "short",
				CostFormat:      "%.2f",
				TokenFormat:     "short",
				Branch:          "show",
				DefaultBranches: []string{"main", "master"},
			},
			Timestamps: TimestampsConfig{
				Mode: "session",
			},
		},
		Privacy: PrivacyConfig{
			HideProjectName:   false,
			HiddenProjectText: "a project",
			Ignore:            []string{},
		},
		Behavior: BehaviorConfig{
			ShowCost:                 true,
			ShowTokens:               false,
			ShowBranch:               true,
			UseStatusline:            false,
			CostShowThreshold:        0,
			TokensShowThreshold:      0,
			IdleMode:                 "clear",
			IdleDetails:              "",
			IdleState:                "Idle",
			PresenceIdleMinutes:      5,
			DaemonIdleMinutes:        30,
			PollIntervalSeconds:      5,
			ReconnectIntervalSeconds: 15,
			SessionCleanupHours:      24,
		},
		Pricing: PricingConfig{
			Source: "url",
			Format: "openrouter",
		},
		Log: LogConfig{
			Level:     "info",
			MaxSizeMB: 10,
		},
	}
}

// ///////////////////////////////////////////////
// Example Configuration
// ///////////////////////////////////////////////

// ExampleConfig returns a Config suitable for generating config.default.toml.
// For this project all defaults are good examples.
func ExampleConfig() *Config {
	return DefaultConfig()
}

// ///////////////////////////////////////////////
// PeekVersion
// ///////////////////////////////////////////////

// PeekVersion reads just the version field from raw TOML bytes.
// Returns 1 if the version field is missing or zero.
func PeekVersion(data []byte) int {
	var v struct {
		Version int `toml:"version"`
	}
	if err := toml.Unmarshal(data, &v); err != nil {
		return 1
	}
	if v.Version == 0 {
		return 1
	}
	return v.Version
}

// ///////////////////////////////////////////////
// Loading and Saving
// ///////////////////////////////////////////////

// Load reads and parses the configuration file from dataDir/config.toml.
// If the file doesn't exist, returns DefaultConfig.
func Load(dataDir string) (*Config, error) {
	path := filepath.Join(dataDir, paths.ConfigFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config file: %w", err)
	}

	version := PeekVersion(data)

	// Apply migrations if needed
	shouldMigrate := version != migrate.Config.CurrentVersion
	if shouldMigrate {
		// Write backup before migration
		if backupErr := os.WriteFile(path+".bak", data, 0o644); backupErr != nil {
			slog.Warn("failed to write config backup", "error", backupErr)
		}
		var migrateErr error
		data, _, migrateErr = migrate.Config.Run(data, version)
		if migrateErr != nil {
			return nil, fmt.Errorf("migrate config: %w", migrateErr)
		}
	}

	// Auto-apply dev transforms
	if migrate.Config.HasDev() {
		var devErr error
		data, devErr = migrate.Config.RunDev(data)
		if devErr != nil {
			return nil, fmt.Errorf("apply dev transforms: %w", devErr)
		}
		shouldMigrate = true
	}

	cfg := DefaultConfig()
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.Version = migrate.Config.CurrentVersion

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	// Re-save after migration
	if shouldMigrate {
		if err := cfg.Save(path); err != nil {
			slog.Warn("failed to save migrated config", "error", err)
		}
	}

	return cfg, nil
}

// Save writes the config to disk as TOML using atomic file write.
func (c *Config) Save(path string) error {
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(c); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	return atomicfile.Write(path, buf.Bytes(), 0o644)
}

// ///////////////////////////////////////////////
// Validation
// ///////////////////////////////////////////////

// costFormatRe matches a valid fmt-style format string for a single float verb.
var costFormatRe = regexp.MustCompile(`^[^%]*%[0-9.*]*[fFeEgG][^%]*$`)

// validLogLevels is the set of accepted log level strings.
var validLogLevels = map[string]bool{
	"trace": true, "debug": true, "info": true, "warn": true, "error": true,
}

// Validate checks that all configuration values are within acceptable ranges.
func (c *Config) Validate() error {
	switch c.Behavior.IdleMode {
	case "clear", "idle_text", "last_activity":
	default:
		return fmt.Errorf("invalid idle_mode %q: must be clear, idle_text, or last_activity", c.Behavior.IdleMode)
	}

	switch c.Display.Timestamps.Mode {
	case "session", "elapsed", "none":
	default:
		return fmt.Errorf("invalid timestamps.mode %q: must be session, elapsed, or none", c.Display.Timestamps.Mode)
	}

	if !validLogLevels[strings.ToLower(c.Log.Level)] {
		return fmt.Errorf("invalid log.level %q: must be trace, debug, info, warn, or error", c.Log.Level)
	}

	if c.Behavior.PollIntervalSeconds <= 0 {
		return fmt.Errorf("poll_interval_seconds must be > 0, got %d", c.Behavior.PollIntervalSeconds)
	}

	if c.Behavior.ReconnectIntervalSeconds <= 0 {
		return fmt.Errorf("reconnect_interval_seconds must be > 0, got %d", c.Behavior.ReconnectIntervalSeconds)
	}

	if c.Behavior.DaemonIdleMinutes < 0 {
		return fmt.Errorf("daemon_idle_minutes must be >= 0, got %d", c.Behavior.DaemonIdleMinutes)
	}

	if c.Behavior.SessionCleanupHours <= 0 {
		return fmt.Errorf("session_cleanup_hours must be > 0, got %d", c.Behavior.SessionCleanupHours)
	}

	switch c.Pricing.Source {
	case "url", "file", "static":
	default:
		return fmt.Errorf("invalid pricing.source %q: must be url, file, or static", c.Pricing.Source)
	}

	switch c.Display.Format.Branch {
	case "show", "hide", "hide_default":
	default:
		return fmt.Errorf("invalid format.branch %q: must be show, hide, or hide_default", c.Display.Format.Branch)
	}

	switch c.Display.Format.ModelName {
	case "short", "full", "raw":
	default:
		return fmt.Errorf("invalid format.model_name %q: must be short, full, or raw", c.Display.Format.ModelName)
	}

	switch c.Display.Format.TokenFormat {
	case "short", "full":
	default:
		return fmt.Errorf("invalid format.token_format %q: must be short or full", c.Display.Format.TokenFormat)
	}

	switch c.Pricing.Format {
	case "openrouter", "litellm", "agentcord":
	default:
		return fmt.Errorf("invalid pricing.format %q: must be openrouter, litellm, or agentcord", c.Pricing.Format)
	}

	if !costFormatRe.MatchString(c.Display.Format.CostFormat) {
		return fmt.Errorf("invalid cost_format %q: must contain exactly one float format verb (%%f, %%e, %%g)", c.Display.Format.CostFormat)
	}

	return nil
}

// ///////////////////////////////////////////////
// Formatting Helpers
// ///////////////////////////////////////////////

// FormatDetails templates {project} and {branch} into the display details string.
// Uses details_no_branch when branch is empty.
func (c *Config) FormatDetails(project, branch string) string {
	tmpl := c.Display.Details
	if branch == "" {
		tmpl = c.Display.DetailsNoBranch
	}
	r := strings.NewReplacer("{project}", project, "{branch}", branch)
	return r.Replace(tmpl)
}

// FormatState templates model, cost, and tokens into the display state string.
// Uses state_no_cost when hasCost is false.
func (c *Config) FormatState(model string, cost float64, tokens int64, hasCost bool) string {
	tmpl := c.Display.State
	if !hasCost {
		tmpl = c.Display.StateNoCost
	}

	costStr := fmt.Sprintf(c.Display.Format.CostFormat, cost)
	tokenStr := formatTokens(tokens, c.Display.Format.TokenFormat)

	r := strings.NewReplacer(
		"{model}", model,
		"{cost}", costStr,
		"{tokens}", tokenStr,
	)
	return r.Replace(tmpl)
}

// formatTokens formats a token count according to the configured format.
func formatTokens(tokens int64, format string) string {
	switch format {
	case "full":
		return FormatWithCommas(tokens)
	default: // "short"
		return FormatShort(tokens)
	}
}

// FormatShort formats a number in abbreviated form: 1M, 1.5M, 234K, 500.
// Exact multiples omit the decimal: 1000 → "1K", 2000000 → "2M".
func FormatShort(n int64) string {
	switch {
	case n >= 1_000_000:
		val := float64(n) / 1_000_000
		if val == float64(int64(val)) {
			return fmt.Sprintf("%dM", int64(val))
		}
		return fmt.Sprintf("%.1fM", val)
	case n >= 1_000:
		val := float64(n) / 1_000
		if val == float64(int64(val)) {
			return fmt.Sprintf("%dK", int64(val))
		}
		return fmt.Sprintf("%.1fK", val)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// FormatWithCommas formats a number with comma separators: 1,500,000.
// Negative numbers are handled by stripping the sign, formatting, and re-adding it.
func FormatWithCommas(n int64) string {
	if n < 0 {
		return "-" + FormatWithCommas(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteByte(',')
		}
		result.WriteRune(c)
	}
	return result.String()
}

// FormatModelName formats a model ID according to the given style.
func FormatModelName(modelID, format string) string {
	switch format {
	case "raw":
		return modelID
	case "full":
		return formatModelFull(modelID)
	default: // "short"
		return formatModelShort(modelID)
	}
}

// FormatModelName is a convenience method that calls the package-level
// [FormatModelName] with the receiver's configured model name format.
func (c *Config) FormatModelName(modelID string) string {
	return FormatModelName(modelID, c.Display.Format.ModelName)
}

// formatModelShort strips known family prefixes, title-cases words, and normalizes
// version separators. "claude-opus-4-6" -> "Opus 4.6", "gpt-4o" -> "4o"
func formatModelShort(id string) string {
	name := id
	for _, prefix := range []string{"claude-", "gpt-", "gemini-", "o1-", "o3-"} {
		if strings.HasPrefix(id, prefix) {
			name = strings.TrimPrefix(id, prefix)
			break
		}
	}
	return titleCaseModel(name)
}

// formatModelFull title-cases the full model name.
// "claude-opus-4-6" -> "Claude Opus 4.6"
func formatModelFull(id string) string {
	return titleCaseModel(id)
}

// titleCaseModel converts a hyphenated model ID into a display name.
// Hyphens between digits become dots (version separator), others become spaces.
func titleCaseModel(s string) string {
	parts := strings.Split(s, "-")
	var result []string
	for i, p := range parts {
		if p == "" {
			continue
		}
		// If this part and the previous part are both numeric, join with dot
		if i > 0 && isNumeric(parts[i-1]) && isNumeric(p) {
			last := result[len(result)-1]
			result[len(result)-1] = last + "." + p
			continue
		}
		// Title case the first letter
		result = append(result, strings.ToUpper(p[:1])+p[1:])
	}
	return strings.Join(result, " ")
}

// isNumeric reports whether s consists entirely of ASCII digits.
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// ///////////////////////////////////////////////
// Privacy Helpers
// ///////////////////////////////////////////////

// IsIgnored reports whether cwd matches any of the configured ignore patterns.
func (c *Config) IsIgnored(cwd string) bool {
	for _, pattern := range c.Privacy.Ignore {
		matched, err := doublestar.Match(pattern, cwd)
		if err != nil {
			slog.Warn("invalid glob pattern", "pattern", pattern, "error", err)
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

// ProjectName returns the display name for a project, respecting privacy settings.
// Per-project overrides are checked first, then the global setting.
func (c *Config) ProjectName(realName, cwd string) string {
	for _, o := range c.Privacy.Overrides {
		matched, err := doublestar.Match(o.Pattern, cwd)
		if err != nil {
			slog.Warn("invalid glob pattern", "pattern", o.Pattern, "error", err)
			continue
		}
		if matched && o.HideProjectName {
			return o.HiddenText
		}
	}
	if c.Privacy.HideProjectName {
		return c.Privacy.HiddenProjectText
	}
	return realName
}

// FormatBranch applies the configured branch display format.
// Returns empty string when the branch should be hidden (triggering details_no_branch template).
func (c *Config) FormatBranch(branch string) string {
	switch c.Display.Format.Branch {
	case "hide":
		return ""
	case "hide_default":
		for _, def := range c.Display.Format.DefaultBranches {
			if branch == def {
				return ""
			}
		}
		return branch
	default: // "show"
		return branch
	}
}

package config

// ///////////////////////////////////////////////
// Documentation Types
// ///////////////////////////////////////////////

// FieldDoc holds documentation and alternative examples for a single config field.
// The genconfig tool uses [FieldDoc] values to annotate the generated config.default.toml.
type FieldDoc struct {
	// Comment is shown as a header comment above the field in the example config.
	Comment string

	// Alternatives are shown as commented-out lines below the active value.
	Alternatives []string
}

// ///////////////////////////////////////////////
// Field Documentation Map
// ///////////////////////////////////////////////

// ConfigDocs maps TOML field paths (dot-separated, e.g. "display.format.model_name")
// to their [FieldDoc] entries. The genconfig tool uses this map to annotate the
// generated config.default.toml with inline comments and alternative examples.
var ConfigDocs = map[string]FieldDoc{
	// ── Root ──────────────────────────────────────────────────────
	"version": {
		Comment: "Config schema version — do not edit.",
	},

	// ── Discord ──────────────────────────────────────────────────
	"discord.app_id": {
		Comment: "Application ID for Discord Rich Presence.\nOverride with your own Discord app if you want custom images.",
	},

	// ── Display ──────────────────────────────────────────────────
	"display.details": {
		Comment: "Format strings for the presence card.\nAvailable variables: {project}, {branch}, {model}, {cost}, {tokens}\nAgentic variables: {tool}, {tool_target}, {file}, {agent_state}, {permission}, {client}\nExtended tokens: {input_tokens}, {output_tokens}, {cache_tokens}, {turns}\nGit extended: {git_owner}, {git_repo}\nFormat suffixes: {file:basename}, {file:dir}, {file:ext}, {model:short}, {model:full}, {model:raw}\n\ndetails = top line, state = bottom line",
	},
	"display.state": {},
	"display.details_no_branch": {
		Comment: "What to show when there's no git branch",
	},
	"display.state_no_cost": {
		Comment: "What to show when cost is unavailable (pricing source unreachable, no pricing data)",
	},

	// ── Assets ───────────────────────────────────────────────────
	"display.assets.large_image": {
		Comment: "Discord image keys (must match assets uploaded to your Discord app)",
	},
	"display.assets.large_text": {},
	"display.assets.show_model_icon": {
		Comment: "Small image shows the active model tier.\nUpload icons named \"opus\", \"sonnet\", \"haiku\" to your Discord app's Rich Presence assets.\nThe daemon automatically sets small_image based on the current model.\nSet to false to disable the small image overlay entirely.",
	},

	// ── Buttons ──────────────────────────────────────────────────
	"display.buttons.show_repo_button": {
		Comment: "Auto-detect git remote URL and show a \"View Repository\" button on the card.\nOnly works when the project CWD has a git remote configured.",
	},
	"display.buttons.repo_button_label": {},
	"display.buttons.custom_button_label": {
		Comment: "Custom second button (optional). Both label and url must be set.",
		Alternatives: []string{
			`custom_button_label = "My Website"`,
		},
	},
	"display.buttons.custom_button_url": {
		Alternatives: []string{
			`custom_button_url = "https://example.com"`,
		},
	},

	// ── Format ───────────────────────────────────────────────────
	"display.format.model_name": {
		Comment: "How to format the model name. Options: \"short\", \"full\", \"raw\"\n  short: \"Opus 4.6\"  (strip \"claude-\" prefix, title case)\n  full:  \"Claude Opus 4.6\"\n  raw:   \"claude-opus-4-6\"  (exact model ID)",
		Alternatives: []string{
			`model_name = "full"`,
			`model_name = "raw"`,
		},
	},
	"display.format.cost_format": {
		Comment: "Cost display format (Go fmt syntax)",
	},
	"display.format.token_format": {
		Comment: "Token count format. Options: \"short\", \"full\"\n  short: \"1.5M\", \"234K\", \"500\"\n  full:  \"1,500,000\"",
		Alternatives: []string{
			`token_format = "full"`,
		},
	},
	"display.format.branch": {
		Comment: "Branch display format. Options: \"show\", \"hide\", \"hide_default\"\n  show: show branch name as-is\n  hide: never show branch\n  hide_default: hide branches listed in default_branches",
		Alternatives: []string{
			`branch = "hide"`,
			`branch = "hide_default"`,
		},
	},
	"display.format.default_branches": {
		Comment: "Branches hidden when branch format is \"hide_default\".",
	},

	// ── Timestamps ───────────────────────────────────────────────
	"display.timestamps.mode": {
		Comment: "What the elapsed timer tracks. Options: \"session\", \"elapsed\", \"none\"\n  session: resets when session_id changes (per Claude Code session)\n  elapsed: resets when daemon starts (persists across sessions)\n  none:    no timestamp shown",
		Alternatives: []string{
			`mode = "elapsed"`,
			`mode = "none"`,
		},
	},

	// ── Privacy ──────────────────────────────────────────────────
	"privacy.hide_project_name": {
		Comment: "Replace project name with generic text in the presence card.",
	},
	"privacy.hidden_project_text": {
		Comment: "Text shown instead of the real project name when hide_project_name is true.",
	},
	"privacy.ignore": {
		Comment: "Directories to completely ignore — no presence shown when working in these.\nAbsolute paths. Glob patterns supported.",
		Alternatives: []string{
			`# ignore = [`,
			`#   "C:/Users/Zach/work/secret-project",`,
			`#   "C:/Users/Zach/company/*",`,
			`# ]`,
		},
	},
	"privacy.overrides": {
		Comment: "Per-project privacy overrides. Each entry matches a glob pattern against the CWD.\n# [[privacy.overrides]]\n# pattern = \"*/work/*\"\n# hide_project_name = true\n# hidden_text = \"a work project\"",
	},

	// ── Behavior ─────────────────────────────────────────────────
	"behavior.show_cost": {
		Comment: "Show cost in the state line",
	},
	"behavior.cost_show_threshold": {
		Comment: "Only show cost if it's >= this value. 0 = always show.",
	},
	"behavior.tokens_show_threshold": {
		Comment: "Only show tokens if count is >= this value. 0 = always show.",
	},
	"behavior.idle_mode": {
		Comment: "What to show when idle. Options: \"clear\", \"idle_text\", \"last_activity\"\n  clear: hide presence entirely (default)\n  idle_text: show idle_details/idle_state text\n  last_activity: keep showing the last active presence",
		Alternatives: []string{
			`idle_mode = "idle_text"`,
			`idle_mode = "last_activity"`,
		},
	},
	"behavior.idle_details": {
		Comment: "Details line when idle (only used with idle_mode = \"idle_text\")",
	},
	"behavior.idle_state": {
		Comment: "State line when idle (only used with idle_mode = \"idle_text\")",
	},
	"behavior.use_statusline": {
		Comment: "Use Claude Code statusline instead of state.json for session data.\nRequires Claude Code v1.0.33+.",
	},
	"behavior.show_tokens": {
		Comment: "Show token count (used when cost is unavailable, or alongside cost)",
	},
	"behavior.show_branch": {
		Comment: "Show git branch in details line",
	},
	"behavior.presence_idle_minutes": {
		Comment: "Minutes of inactivity before presence hides from your Discord profile.\nDaemon stays alive — presence resumes on next activity with the original\nsession timer preserved (elapsed time reflects real session duration).",
	},
	"behavior.daemon_idle_minutes": {
		Comment: "Minutes of inactivity before the daemon process exits entirely.\nMust be >= presence_idle_minutes. Daemon auto-restarts on next hook fire.",
	},
	"behavior.poll_interval_seconds": {
		Comment: "How often to poll for state changes (seconds). fsnotify is primary,\nthis is the fallback interval.",
	},
	"behavior.reconnect_interval_seconds": {
		Comment: "Discord reconnect interval (seconds)",
	},
	"behavior.session_cleanup_hours": {
		Comment: "Remove orphaned session markers older than this many hours.\nOrphans appear when Claude Code exits without firing the stop hook.",
	},

	// ── Pricing ─────────────────────────────────────────────────
	"pricing.source": {
		Comment: "Where to get model pricing data. Options: \"url\", \"file\", \"static\"\n  url: fetch from a remote API (default)\n  file: read from a local JSON file\n  static: use inline prices defined in [pricing.models]",
		Alternatives: []string{
			`source = "file"`,
			`source = "static"`,
		},
	},
	"pricing.format": {
		Comment: "How to parse the pricing response. Options: \"openrouter\", \"litellm\", \"agentcord\"\nEach format has a default URL when source = \"url\":\n  openrouter -> https://openrouter.ai/api/v1/models\n  litellm    -> https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json\n  agentcord  -> (no default, url required)",
		Alternatives: []string{
			`format = "litellm"`,
			`format = "agentcord"`,
		},
	},
	"pricing.url": {
		Comment: "Custom URL (overrides the format's default URL).",
		Alternatives: []string{
			`# url = "https://my-proxy.internal/api/v1/models"`,
		},
	},
	"pricing.file": {
		Comment: "Local file path (for source = \"file\").",
		Alternatives: []string{
			`# file = "/path/to/pricing.json"`,
		},
	},
	"pricing.models": {
		Comment: "Inline prices (for source = \"static\").\n# [pricing.models.claude-opus-4-6]\n# input_per_token = 0.000015\n# output_per_token = 0.000075",
	},

	// ── Log ──────────────────────────────────────────────────────
	"log": {
		Comment: "Logging configuration",
	},
	"log.level": {
		Comment: "Minimum log level. Options: \"trace\", \"debug\", \"info\", \"warn\", \"error\"",
		Alternatives: []string{
			`level = "debug"`,
			`level = "warn"`,
		},
	},
	"log.max_size_mb": {
		Comment: "Maximum log file size in megabytes before rotation.",
	},
	"clients": {
		Comment: "Per-client display overrides keyed by client name (e.g. cursor, windsurf).",
		Alternatives: []string{
			`[clients.cursor]`,
			`large_image = "cursor"`,
			`large_text = "Cursor"`,
		},
	},
}

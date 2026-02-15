# Agentcord

Discord Rich Presence for agentic coding tools. Shows your current project, git branch, model, active tool, and estimated API cost on your Discord profile.

Works with [Claude Code](https://docs.anthropic.com/en/docs/claude-code) out of the box. Designed for contributors to add support for any tool.

<!-- screenshot -->

## Supported Tools

| Tool | Status | Install Method |
|------|--------|---------------|
| Claude Code | Supported | Plugin install |
| Cursor | Planned | Manual hooks |
| Windsurf | Planned | Manual hooks |
| Claude Desktop | Planned | Waiting for API |

See [`clients/`](clients/) for per-tool setup docs and [`clients/_template/`](clients/_template/) to add your own.

## Installation

### Claude Code (recommended)

```bash
claude plugin add zachthedev/agentcord
```

The daemon binary downloads automatically on first session.

### Manual install

```bash
git clone https://github.com/zachthedev/agentcord.git
claude --plugin-dir ./agentcord
```

### Prerequisites

- **Bun** — runs the hook dispatcher ([install](https://bun.sh))
- **jq** — used by hook scripts to build state JSON (Unix/macOS)
- **pwsh** — PowerShell 7+ (Windows)

## How It Works

Agentcord has two parts: **hook scripts** that capture session data, and a **daemon** that publishes it to Discord.

### Hook system

Your coding tool fires hooks on events. Each hook invokes `dispatch.ts`, which detects the OS and delegates to the matching platform script:

| Event | Script | What it does |
|---|---|---|
| `SessionStart` | `postinstall` | Verify dependencies, download daemon if missing |
| `PreToolUse` | `activity` | Write state file with tool context, start daemon if needed |
| `PostToolUse` | `activity` | Update state after tool execution |
| `PostToolUseFailure` | `activity` | Update state after tool failure |
| `Notification` | `activity` | Update state on idle/permission prompts |
| `UserPromptSubmit` | `activity` | Update state when user sends a message |
| `Stop` | `stop` | Mark session as stopped |
| `SessionEnd` | `sessionend` | Clean up session markers, stop daemon if idle |

Platform scripts live in `scripts/hooks/unix/` and `scripts/hooks/windows/`.

### Daemon

The daemon is a single Go binary (`~/.agentcord/agentcord`) that:

1. Watches `~/.agentcord/state.*.json` for changes (filesystem events + polling fallback)
2. Parses JSONL conversation logs for token counts and cost data
3. Connects to Discord via local IPC (Unix socket or Windows named pipe)
4. Publishes Rich Presence with project info, model, cost, and elapsed time
5. Idles and exits automatically when no sessions are active

### State files

Hook scripts write per-client state files named `state.{client}.json` (e.g. `state.claude-code.json`). Each file contains session ID, project name, git branch, active tool, and timestamps.

Session markers (`sessions/{SESSION_ID}.session`) track which sessions are alive. Orphaned markers are cleaned up automatically.

## Configuration

Config lives at `~/.agentcord/config.toml`. The daemon generates this file with documented defaults on first run.

```toml
[display]
details = "Working on: {project} ({branch})"
state = "{model} · ~${cost} API value"

[display.timestamps]
mode = "session"   # "session", "elapsed", or "none"
```

### Template variables

| Variable | Description |
|----------|-------------|
| `{project}` | Project name |
| `{branch}` | Git branch |
| `{model}` | Model name (`:short`, `:full`, `:raw`) |
| `{cost}` | API cost in USD |
| `{tokens}` | Total tokens (`:short`, `:full`) |
| `{tool}` | Current tool (Edit, Bash, Read, etc.) |
| `{tool_target}` | Tool target (`:basename`, `:dir`) |
| `{file}` | Active file (`:basename`, `:dir`, `:ext`) |
| `{agent_state}` | thinking, tool, waiting, idle |
| `{client}` | Client display name |
| `{turns}` | Conversation turn count |
| `{input_tokens}` | Input tokens |
| `{output_tokens}` | Output tokens |
| `{cache_tokens}` | Cache tokens |
| `{git_owner}` | Repo owner |
| `{git_repo}` | Repo name |

See [`config.default.toml`](config.default.toml) for all options with inline documentation.

## Multi-Client Support

Agentcord supports multiple tools simultaneously. Each client writes its own state file. The daemon always displays the most recently active session.

Per-client overrides (different icons, different Discord app IDs) are configured via `[clients.X]` sections:

```toml
[clients.cursor]
large_image = "cursor_icon"
large_text = "Cursor"
app_id = "YOUR_CURSOR_APP_ID"
```

## Privacy

- **Hide project names:** `privacy.hide_project_name = true`
- **Ignore directories:** `privacy.ignore = ["/path/to/secret"]` (glob patterns)
- **Per-project overrides:** `[[privacy.overrides]]` with pattern matching

```toml
[privacy]
ignore = ["/home/user/work/secret-project"]

[[privacy.overrides]]
pattern = "*/work/*"
hide_project_name = true
hidden_text = "a work project"
```

## WSL

WSL1 works out of the box. WSL2 requires [`npiperelay`](https://github.com/jstarks/npiperelay) to bridge Discord's Windows named pipe.

## Development

Requires Go 1.25+, Bun, jq, pwsh (for Windows tests).

```bash
make build       # Build daemon
make test-all    # Go + BATS + Bun tests
make lint        # golangci-lint
```

### Project structure

```
clients/                      Per-tool integration configs and docs
hooks/hooks.json              Hook definitions (loaded by Claude Code)
scripts/
  hooks/
    dispatch.ts               Cross-platform dispatcher (Bun/TypeScript)
    unix/                     Bash hook scripts
    windows/                  PowerShell hook scripts
    lib/                      Shared libraries and constants
  install.sh / install.ps1    Auto-download daemon binary
cmd/
  agentcord/                  Daemon entry point
internal/
  config/                     TOML config with codegen defaults
  discord/                    Discord IPC Rich Presence client
  paths/                      Data directory constants
  pricing/                    Model pricing (OpenRouter, LiteLLM, static)
  session/                    State watcher + JSONL parser + activity builder
  tiers/                      Model tier icons (remote -> cache -> embedded)
  logger/                     Structured slog with rotation
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for build instructions, code conventions, and how to add a new tool integration.

## License

[MIT](LICENSE)

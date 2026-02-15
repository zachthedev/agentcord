# Contributing to Agentcord

Thank you for your interest in contributing to Agentcord! This guide covers everything you need to get started.

## Prerequisites

- **Go 1.25+** -- [golang.org/dl](https://go.dev/dl/)
- **Bun** -- [bun.sh](https://bun.sh) (for TypeScript dispatch scripts)
- **jq** -- JSON processor used by hook scripts
- **pwsh** (PowerShell 7+) -- for Windows hook tests
- **golangci-lint** -- [golangci-lint.run](https://golangci-lint.run/usage/install/)
- **BATS** -- `bun install -g bats` (for shell tests)
- **Discord** -- running locally for end-to-end testing

## Quick Start

```bash
git clone https://github.com/zachthedev/agentcord.git
cd agentcord
make build       # Build the daemon binary
make test        # Run Go tests
make test-all    # Run all test suites (Go + BATS + dispatch)
make lint        # Run linters
make generate    # Regenerate generated files
```

## Architecture

Agentcord is a two-part system:

1. **Hook scripts** capture session data from agentic coding tools and write it to a JSON state file
2. **Daemon** watches the state file and publishes Discord Rich Presence updates

Data flow:
```
Tool Hook Event -> dispatch.ts -> platform script (activity.sh/ps1) -> state.json -> daemon -> Discord IPC
```

## Project Structure

```
cmd/agentcord/       # Daemon binary
cmd/genconfig/       # Config codegen tool
cmd/genhooks/        # Hook constants codegen tool
internal/            # Go packages (config, discord, session, pricing, tiers, etc.)
scripts/hooks/       # Hook scripts (dispatch.ts, unix/, windows/, lib/)
clients/             # Per-tool integration configs and scaffolds
assets/discord/      # Discord presence image assets
data/                # Remote-fetched data (tiers.json)
tools/generate-assets/  # Asset generation tool
```

## How to Add a New Tool Integration

This is the primary contribution use case. To add support for a new agentic coding tool:

1. **Choose a client ID** following the naming convention: `^[a-z][a-z0-9]*(-[a-z0-9]+)*$`, max 48 chars. Examples: `cursor`, `claude-code`, `roo-code`.

2. **Create `clients/<your-tool>/`** directory with:
   - `hooks.json` -- Hook definitions in the tool's format
   - `README.md` -- Setup instructions specific to your tool

3. **Add client defaults** in `internal/config/config.go`:
   ```go
   "your-tool": {DisplayName: "Your Tool", Icon: "your_tool"},
   ```

4. **Handle session ID field** if your tool uses a different JSON field name than `.session_id`, add a case to `scripts/hooks/lib/unix/common.sh` and `common.ps1`.

5. **Test** the integration end-to-end:
   - Pipe mock hook JSON through the activity script
   - Verify the state file is written correctly
   - Check that Discord presence shows the expected data

6. **Upload Discord assets** -- add your tool's icon to `assets/discord/clients/` and upload to the Discord Developer Portal.

7. **Submit a PR** using the new tool integration template.

## Code Conventions

### Go
- Format with `gofumpt` (enforced by CI)
- Wrap errors: `fmt.Errorf("context: %w", err)`
- Use `slog` for logging (never `log` or `fmt.Print`)
- Test files: `*_test.go` in the same package

### Shell (bash)
- Never use `set -e` in hook scripts (non-fatal contract)
- Always source `common.sh` via `$AGENTCORD_COMMON`
- Use `jq` for JSON manipulation
- Exit 0 on non-critical failures

### PowerShell
- Use `$ErrorActionPreference = 'Continue'` in hooks
- Mirror bash script structure 1:1
- Source `common.ps1` via `$env:AGENTCORD_COMMON`

### Commit Messages
Format: `type(scope): description`

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `ci`, `chore`
Scopes: `daemon`, `hooks`, `config`, `pricing`, `tiers`, `discord`, `install`, `assets`, `ci`

## Codegen Pipeline

Several files are generated from Go source:
- `config.default.toml` -- from `internal/config/config.go` via `cmd/genconfig`
- `scripts/hooks/lib/unix/constants.sh` -- from `internal/paths/paths.go` via `cmd/genhooks`
- `scripts/hooks/lib/windows/constants.ps1` -- same source

**Always run `go generate ./...` after changing paths or config structure.** CI verifies generated files are up to date.

## Running Tests

| Command | What it tests |
|---------|--------------|
| `make test` | Go unit tests |
| `make test-sh` | BATS shell script tests (Unix) |
| `make test-ps1` | Pester PowerShell tests (Windows) |
| `make test-ts` | TypeScript dispatch tests |
| `make test-all` | All of the above |
| `make lint` | golangci-lint |

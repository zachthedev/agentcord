# Claude Code Integration

Agentcord ships as a Claude Code plugin. The root `hooks/hooks.json` provides the hook definitions.

## Installation

```bash
claude plugin add zachthedev/agentcord
```

## How it works

Claude Code fires hooks on events like `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Stop`, etc. Each hook invokes `dispatch.ts`, which delegates to platform-specific scripts (bash on Unix, PowerShell on Windows).

The hook scripts:

1. Read hook input JSON from stdin (tool name, file paths, event type)
2. Detect git branch and remote
3. Write session state to `~/.agentcord/state.claude-code.json`
4. Ensure the daemon is running

The daemon watches the state file and publishes Discord Rich Presence via IPC.

## Data available from Claude Code hooks

| Field | Source | Notes |
|-------|--------|-------|
| `tool_name` | `hook_input.tool_name` | Edit, Read, Write, Bash, Grep, Glob, Task, etc. |
| `tool_input` | `hook_input.tool_input` | Tool-specific input (file_path, command, pattern) |
| `hook_event_name` | `hook_input.hook_event_name` | PreToolUse, PostToolUse, UserPromptSubmit, etc. |
| `session_id` | `hook_input.session_id` | Unique session identifier |
| `permission_mode` | `hook_input.permission_mode` | plan, acceptEdits, dontAsk |

## Session ID field

Claude Code uses `.session_id` in its hook input JSON.

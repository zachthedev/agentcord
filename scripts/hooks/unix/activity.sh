#!/usr/bin/env bash
# activity.sh — PreToolUse / PostToolUse hook.
# Updates session state and ensures the daemon is running.
# Invoked by Claude Code hook system; must NOT use set -e.

[ -z "$AGENTCORD_COMMON" ] && { echo "AGENTCORD_COMMON not set — run via dispatch.ts" >&2; exit 1; }
. "$AGENTCORD_COMMON"
require_jq
read_hook_input

mkdir -p "$SESSIONS_PATH"

# ///////////////////////////////////////////////
# Session State
# ///////////////////////////////////////////////

# Clean up orphaned session markers older than 24 hours
find "$SESSIONS_PATH" -name "*${SESSION_EXT}" -mmin +1440 -delete 2>/dev/null || true

# Write session marker file (always — must not be skipped by debounce)
touch "$SESSIONS_PATH/${SESSION_ID}${SESSION_EXT}"

# Debounce: skip state write if less than 5 seconds since last write
if [ -f "$STATE_PATH" ]; then
    LAST_MOD=$(stat -c %Y "$STATE_PATH" 2>/dev/null || stat -f %m "$STATE_PATH" 2>/dev/null || echo 0)
    NOW=$(date +%s)
    if [ $((NOW - LAST_MOD)) -lt 5 ]; then
        exit 0
    fi
fi

# Read existing state to preserve sessionStart
SESSION_START=""
OLD_SESSION_ID=""
if [ -f "$STATE_PATH" ]; then
    OLD_SESSION_ID=$(jq -r '.sessionId // empty' "$STATE_PATH")
    if [ "$OLD_SESSION_ID" = "$SESSION_ID" ]; then
        SESSION_START=$(jq -r '.sessionStart // empty' "$STATE_PATH")
    fi
fi

# New session or no prior state — set fresh start time
if [ -z "$SESSION_START" ]; then
    SESSION_START=$(date +%s)
fi

LAST_ACTIVITY=$(date +%s)
PROJECT=$(basename "$PWD")
CWD="$PWD"

# ///// Git Info /////

# Always check git branch (catches git checkout between hooks)
BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)
GIT_REMOTE=$(git remote get-url origin 2>/dev/null || true)

# ///// Tool Context /////

TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name // empty')
HOOK_EVENT=$(echo "$INPUT" | jq -r '.hook_event_name // empty')
PERMISSION_MODE=$(echo "$INPUT" | jq -r '.permission_mode // empty')

# Extract tool target based on tool name
TOOL_TARGET=""
ACTIVE_FILE=""
case "$TOOL_NAME" in
    Edit|Write|Read) TOOL_TARGET=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
                     ACTIVE_FILE="$TOOL_TARGET" ;;
    Bash)            TOOL_TARGET=$(echo "$INPUT" | jq -r '.tool_input.command // empty' | head -c 80) ;;
    Grep|Glob)       TOOL_TARGET=$(echo "$INPUT" | jq -r '.tool_input.pattern // empty') ;;
    *)               TOOL_TARGET="" ;;
esac

# Derive agent state from hook event
AGENT_STATE=""
case "$HOOK_EVENT" in
    PreToolUse)       AGENT_STATE="tool" ;;
    PostToolUse)      AGENT_STATE="thinking" ;;
    UserPromptSubmit) AGENT_STATE="thinking" ;;
    Notification)     AGENT_STATE="waiting" ;;
    *)                AGENT_STATE="" ;;
esac

# PostToolUse clears tool context so stale tool names don't persist
if [ "$HOOK_EVENT" = "PostToolUse" ] || [ "$HOOK_EVENT" = "PostToolUseFailure" ]; then
    TOOL_NAME=""
    TOOL_TARGET=""
fi

# ///// Write State /////

jq -n \
    --argjson version "$STATE_VERSION" \
    --arg client "$CLIENT" \
    --arg sid "$SESSION_ID" \
    --argjson start "$SESSION_START" \
    --argjson activity "$LAST_ACTIVITY" \
    --arg project "$PROJECT" \
    --arg branch "$BRANCH" \
    --arg cwd "$CWD" \
    --arg remote "$GIT_REMOTE" \
    --arg toolName "$TOOL_NAME" \
    --arg toolTarget "$TOOL_TARGET" \
    --arg activeFile "$ACTIVE_FILE" \
    --arg agentState "$AGENT_STATE" \
    --arg permissionMode "$PERMISSION_MODE" \
    --arg hookEvent "$HOOK_EVENT" \
    '{
        "$version": $version,
        "sessionId": $sid,
        "sessionStart": $start,
        "lastActivity": $activity,
        "project": $project,
        "branch": $branch,
        "cwd": $cwd,
        "gitRemoteUrl": $remote,
        "client": $client,
        "stopped": false,
        "toolName": $toolName,
        "toolTarget": $toolTarget,
        "activeFile": $activeFile,
        "agentState": $agentState,
        "permissionMode": $permissionMode,
        "hookEvent": $hookEvent
    }' | write_state

# ///////////////////////////////////////////////
# Daemon Health Check
# ///////////////////////////////////////////////

DAEMON_ALIVE=false
if [ -f "$PID_PATH" ]; then
    DAEMON_PID=$(cut -d: -f1 "$PID_PATH")
    if kill -0 "$DAEMON_PID" 2>/dev/null; then
        DAEMON_ALIVE=true
    fi
fi

if [ "$DAEMON_ALIVE" = false ]; then
    # Download binary if missing
    if [ ! -f "$DAEMON_BIN" ] && [ ! -f "$DAEMON_BIN_WIN" ]; then
        sh "${CLAUDE_PLUGIN_ROOT:-$(dirname "$0")/../../..}/scripts/install.sh"
    fi

    # Launch daemon in background
    BIN="$DAEMON_BIN"
    [ -f "$DAEMON_BIN_WIN" ] && BIN="$DAEMON_BIN_WIN"
    if [ -f "$BIN" ]; then
        "$BIN" --data-dir "$DATA_DIR" &
        disown 2>/dev/null || true
    fi
fi

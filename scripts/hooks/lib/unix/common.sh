#!/usr/bin/env bash
# common.sh — Shared config and functions for agentcord hook scripts.
# Sourced via: . "${AGENTCORD_LIB:-...}/common.sh"

_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load generated constants (source of truth: internal/paths/paths.go)
. "$_LIB_DIR/constants.sh"

# Construct full paths from constants
DATA_DIR="${AGENTCORD_DATA_DIR:-$HOME/$DATA_DIR_REL}"
PID_PATH="$DATA_DIR/$PID_FILE"
SESSIONS_PATH="$DATA_DIR/$SESSIONS_DIR"
DAEMON_BIN="$DATA_DIR/$BINARY_NAME"
DAEMON_BIN_WIN="$DATA_DIR/${BINARY_NAME}.exe"

# Client identity — set by dispatch.ts, required
CLIENT="${AGENTCORD_CLIENT:?AGENTCORD_CLIENT not set — run via dispatch.ts}"

# Session ID field — varies by client tool
case "$CLIENT" in
    claude-code) SESSION_ID_FIELD=".session_id" ;;
    *)           SESSION_ID_FIELD="${AGENTCORD_SESSION_ID_FIELD:-.session_id}" ;;
esac

# Per-client state file path
STATE_PATH="$DATA_DIR/state.${CLIENT}.json"

require_jq() {
    command -v jq >/dev/null 2>&1 || { echo "jq is required for agentcord hooks" >&2; exit 1; }
}

read_hook_input() {
    INPUT=$(cat)
    SESSION_ID=$(echo "$INPUT" | jq -r "$SESSION_ID_FIELD // empty")
    [ -z "$SESSION_ID" ] && exit 0
}

write_state() {
    local tmp="$STATE_PATH.tmp.$$"
    cat > "$tmp"
    mv -f "$tmp" "$STATE_PATH"
}

kill_daemon() {
    if [ -f "$PID_PATH" ]; then
        local pid
        pid=$(cut -d: -f1 "$PID_PATH")
        kill "$pid" 2>/dev/null || true
        rm -f "$PID_PATH"
    fi
}

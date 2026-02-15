#!/usr/bin/env bash
# postinstall.sh — SessionStart hook.
# Outputs daemon status to stdout so Claude has context about plugin state.
# Must NOT use set -e. No jq dependency.

[ -z "$AGENTCORD_COMMON" ] && { echo "AGENTCORD_COMMON not set — run via dispatch.ts" >&2; exit 1; }
. "$AGENTCORD_COMMON"

# Drain stdin (Claude Code sends hook input on stdin)
cat > /dev/null

# Check if daemon binary exists
if [ ! -f "$DAEMON_BIN" ] && [ ! -f "$DAEMON_BIN_WIN" ]; then
    echo "[agentcord] Discord Rich Presence will activate on first tool use (daemon downloading)"
fi

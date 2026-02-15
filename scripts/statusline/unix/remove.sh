#!/usr/bin/env bash
# remove.sh — Restore original Claude Code statusline configuration.
# Reverses setup-statusline.sh: restores the backed-up statusline or removes the key.
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
    echo "Error: jq is required but not installed." >&2
    exit 1
fi

# ///////////////////////////////////////////////
# Paths
# ///////////////////////////////////////////////

DATA_DIR="${AGENTCORD_DATA_DIR:-$HOME/.agentcord}"
CLAUDE_CONFIG="$HOME/.claude/settings.json"
BACKUP="$DATA_DIR/statusline-original.sh"

# ///////////////////////////////////////////////
# Restore
# ///////////////////////////////////////////////

if [ ! -f "$CLAUDE_CONFIG" ]; then
    echo "No Claude Code config found, nothing to restore."
    exit 0
fi

TMP="$CLAUDE_CONFIG.tmp.$$"

if [ -f "$BACKUP" ]; then
    jq --arg orig "$BACKUP" '.statusline = $orig' "$CLAUDE_CONFIG" > "$TMP"
    mv "$TMP" "$CLAUDE_CONFIG"
    rm -f "$BACKUP"
    echo "Restored original statusline command."
else
    jq 'del(.statusline)' "$CLAUDE_CONFIG" > "$TMP"
    mv "$TMP" "$CLAUDE_CONFIG"
    echo "Removed Agentcord statusline configuration."
fi

rm -f "$DATA_DIR/statusline.json"
echo "Restart Claude Code for changes to take effect."

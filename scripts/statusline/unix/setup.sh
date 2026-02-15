#!/usr/bin/env bash
# setup.sh — Configure Claude Code to pipe statusline data to Agentcord.
# Backs up any existing statusline command, then sets the wrapper as the handler.
# Requires jq to be installed. Idempotent — safe to run multiple times.
set -euo pipefail

# ///////////////////////////////////////////////
# Prerequisites
# ///////////////////////////////////////////////

if ! command -v jq >/dev/null 2>&1; then
    echo "Error: jq is required but not installed." >&2
    echo "  macOS:  brew install jq" >&2
    echo "  Linux:  sudo apt install jq  (or your package manager)" >&2
    exit 1
fi

# ///////////////////////////////////////////////
# Paths
# ///////////////////////////////////////////////

DATA_DIR="${AGENTCORD_DATA_DIR:-$HOME/.agentcord}"
CLAUDE_CONFIG="$HOME/.claude/settings.json"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WRAPPER="$SCRIPT_DIR/wrapper.sh"

mkdir -p "$DATA_DIR"

# ///////////////////////////////////////////////
# Backup existing statusline
# ///////////////////////////////////////////////

if [ -f "$CLAUDE_CONFIG" ]; then
    EXISTING=$(jq -r '.statusline // empty' "$CLAUDE_CONFIG" 2>/dev/null || true)
    if [ -n "$EXISTING" ] && [ "$EXISTING" != "$WRAPPER" ]; then
        cp "$EXISTING" "$DATA_DIR/statusline-original.sh" 2>/dev/null || true
        chmod +x "$DATA_DIR/statusline-original.sh" 2>/dev/null || true
        echo "Backed up existing statusline to $DATA_DIR/statusline-original.sh"
    fi
fi

# ///////////////////////////////////////////////
# Configure Claude Code
# ///////////////////////////////////////////////

if [ ! -f "$CLAUDE_CONFIG" ]; then
    echo "{}" > "$CLAUDE_CONFIG"
fi

TMP="$CLAUDE_CONFIG.tmp.$$"
jq --arg wrapper "$WRAPPER" '.statusline = $wrapper' "$CLAUDE_CONFIG" > "$TMP"
mv "$TMP" "$CLAUDE_CONFIG"

echo "Claude Code statusline configured to use Agentcord wrapper."
echo "Restart Claude Code for changes to take effect."

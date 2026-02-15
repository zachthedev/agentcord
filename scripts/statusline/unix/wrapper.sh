#!/usr/bin/env bash
# wrapper.sh — Pipe Claude Code statusline JSON to a file for Agentcord.
# Reads JSON from stdin, writes atomically to {dataDir}/statusline.json,
# then chains to the original statusline command if one was backed up.
set -euo pipefail

DATA_DIR="${AGENTCORD_DATA_DIR:-$HOME/.agentcord}"

# Read stdin (Claude Code statusline JSON)
INPUT=$(cat)

# Atomic write: tmp + mv
TMP="$DATA_DIR/statusline.json.tmp.$$"
printf '%s\n' "$INPUT" > "$TMP"
mv "$TMP" "$DATA_DIR/statusline.json"

# Chain to original statusline if backed up
BACKUP="$DATA_DIR/statusline-original.sh"
if [ -f "$BACKUP" ] && [ -x "$BACKUP" ]; then
    printf '%s' "$INPUT" | "$BACKUP"
fi

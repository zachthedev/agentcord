#!/usr/bin/env bash
# start.sh — Manually start the agentcord daemon.
# Run this to launch the daemon outside of Claude Code hooks.
set -euo pipefail

. "$(dirname "$0")/../hooks/lib/unix/constants.sh"

DATA_DIR="${AGENTCORD_DATA_DIR:-$HOME/$DATA_DIR_REL}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

mkdir -p "$DATA_DIR"

# ///////////////////////////////////////////////
# Ensure Binary Exists
# ///////////////////////////////////////////////

DAEMON_BIN="$DATA_DIR/$BINARY_NAME"
DAEMON_BIN_WIN="$DATA_DIR/${BINARY_NAME}.exe"

if [ ! -f "$DAEMON_BIN" ] && [ ! -f "$DAEMON_BIN_WIN" ]; then
    echo "Downloading agentcord daemon..."
    sh "$SCRIPT_DIR/../install.sh"
fi

BIN="$DAEMON_BIN"
[ -f "$DAEMON_BIN_WIN" ] && BIN="$DAEMON_BIN_WIN"

if [ ! -f "$BIN" ]; then
    echo "Error: daemon binary not found at $BIN" >&2
    exit 1
fi

# ///////////////////////////////////////////////
# Start Daemon
# ///////////////////////////////////////////////

PID_PATH="$DATA_DIR/$PID_FILE"

if [ -f "$PID_PATH" ]; then
    DAEMON_PID=$(cut -d: -f1 "$PID_PATH")
    if kill -0 "$DAEMON_PID" 2>/dev/null; then
        echo "Daemon is already running (PID $DAEMON_PID)"
        exit 0
    fi
    # Stale PID file
    rm -f "$PID_PATH"
fi

"$BIN" --data-dir "$DATA_DIR" &
disown 2>/dev/null || true
echo "Daemon started (PID $!)"

#!/usr/bin/env bash
# stop.sh — Manually stop the agentcord daemon.
# Run this to kill the daemon and clean up session files.
set -euo pipefail

. "$(dirname "$0")/../hooks/lib/unix/constants.sh"

DATA_DIR="${AGENTCORD_DATA_DIR:-$HOME/$DATA_DIR_REL}"

# ///////////////////////////////////////////////
# Stop Daemon
# ///////////////////////////////////////////////

PID_PATH="$DATA_DIR/$PID_FILE"

if [ -f "$PID_PATH" ]; then
    DAEMON_PID=$(cut -d: -f1 "$PID_PATH")
    if kill -0 "$DAEMON_PID" 2>/dev/null; then
        kill "$DAEMON_PID" 2>/dev/null || true
        for i in $(seq 1 10); do
            kill -0 "$DAEMON_PID" 2>/dev/null || break
            sleep 0.1
        done
        echo "Daemon stopped (PID $DAEMON_PID)"
    else
        echo "Daemon was not running (stale PID file)"
    fi
    rm -f "$PID_PATH"
else
    echo "No daemon PID file found"
fi

# ///////////////////////////////////////////////
# Clean Up Sessions
# ///////////////////////////////////////////////

SESSIONS_PATH="$DATA_DIR/$SESSIONS_DIR"

if [ -d "$SESSIONS_PATH" ]; then
    rm -f "$SESSIONS_PATH/"*"$SESSION_EXT" 2>/dev/null || true
    echo "Session files cleared"
fi

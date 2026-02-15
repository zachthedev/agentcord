#!/usr/bin/env bash
# stop.sh — Stop hook.
# Marks presence as idle. Session cleanup is handled by sessionend.sh.
# Invoked by Claude Code hook system; must NOT use set -e.

[ -z "$AGENTCORD_COMMON" ] && { echo "AGENTCORD_COMMON not set — run via dispatch.ts" >&2; exit 1; }
. "$AGENTCORD_COMMON"
require_jq
read_hook_input

# Guard against re-entry
if [ "${_STOP_HOOK_ACTIVE:-}" = "true" ]; then exit 0; fi
_STOP_HOOK_ACTIVE=true

# ///////////////////////////////////////////////
# Mark Presence as Idle
# ///////////////////////////////////////////////

if [ -f "$STATE_PATH" ]; then
    # Only update if this session owns the current state
    CURRENT_SID=$(jq -r '.sessionId // empty' "$STATE_PATH")
    if [ "$CURRENT_SID" = "$SESSION_ID" ]; then
        LAST_ACTIVITY=$(date +%s)
        jq --argjson activity "$LAST_ACTIVITY" \
            '.lastActivity = $activity | .stopped = true' \
            "$STATE_PATH" | write_state
    fi
fi

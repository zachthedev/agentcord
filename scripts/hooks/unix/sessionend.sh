#!/usr/bin/env bash
# sessionend.sh — SessionEnd hook.
# Cleans up session marker and kills daemon when no sessions remain.
# Invoked by Claude Code hook system; must NOT use set -e.

[ -z "$AGENTCORD_COMMON" ] && { echo "AGENTCORD_COMMON not set — run via dispatch.ts" >&2; exit 1; }
. "$AGENTCORD_COMMON"
require_jq
read_hook_input

# ///////////////////////////////////////////////
# Session Cleanup
# ///////////////////////////////////////////////

# Remove this session's marker file
rm -f "$SESSIONS_PATH/${SESSION_ID}${SESSION_EXT}"

# Count remaining sessions
REMAINING=0
if [ -d "$SESSIONS_PATH" ]; then
    REMAINING=$(find "$SESSIONS_PATH" -maxdepth 1 -name "*${SESSION_EXT}" | wc -l | tr -d '[:space:]')
fi

# ///////////////////////////////////////////////
# Last Session — Stop Daemon
# ///////////////////////////////////////////////

if [ "$REMAINING" -eq 0 ]; then
    LAST_ACTIVITY=$(date +%s)

    if [ -f "$STATE_PATH" ]; then
        jq --argjson activity "$LAST_ACTIVITY" \
            '.lastActivity = $activity | .stopped = true' \
            "$STATE_PATH" | write_state
    else
        jq -n \
            --argjson version "$STATE_VERSION" \
            --arg client "$CLIENT" \
            --arg sid "$SESSION_ID" \
            --argjson activity "$LAST_ACTIVITY" \
            '{
                "$version": $version,
                "sessionId": $sid,
                "sessionStart": $activity,
                "lastActivity": $activity,
                "project": "",
                "branch": "",
                "cwd": "",
                "gitRemoteUrl": "",
                "client": $client,
                "stopped": true
            }' | write_state
    fi

    kill_daemon
fi

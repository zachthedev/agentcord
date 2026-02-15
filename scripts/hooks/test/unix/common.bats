#!/usr/bin/env bats
# common.bats — Tests for lib/unix/common.sh shared functions and constants.

setup() {
    export AGENTCORD_LIB="$(cd "$(dirname "$BATS_TEST_FILENAME")/../../lib/unix" && pwd)"
    export AGENTCORD_COMMON="$AGENTCORD_LIB/common.sh"
    export AGENTCORD_DATA_DIR="$(mktemp -d)"
    export AGENTCORD_CLIENT="code"
    # Source common to get functions and constants
    source "$AGENTCORD_COMMON"
}

teardown() {
    rm -rf "$AGENTCORD_DATA_DIR"
}

# ---------------------------------------------------------------------------
# require_jq
# ---------------------------------------------------------------------------

@test "require_jq succeeds when jq is available" {
    # jq should be installed in CI / dev environments
    run require_jq
    [ "$status" -eq 0 ]
}

@test "require_jq fails when jq is missing" {
    # Run in a subshell with an empty PATH so jq cannot be found
    run bash -c 'export PATH=/nonexistent; source "$AGENTCORD_COMMON"; require_jq'
    [ "$status" -ne 0 ]
    [[ "$output" == *"jq is required"* ]]
}

# ---------------------------------------------------------------------------
# read_hook_input
# ---------------------------------------------------------------------------

@test "read_hook_input extracts session_id" {
    local result
    result=$(echo '{"session_id":"abc-123"}' | bash -c '
        source "$AGENTCORD_COMMON"
        read_hook_input
        echo "$SESSION_ID"
    ')
    [ "$result" = "abc-123" ]
}

# ---------------------------------------------------------------------------
# write_state
# ---------------------------------------------------------------------------

@test "write_state does atomic write" {
    mkdir -p "$AGENTCORD_DATA_DIR"
    echo '{"active":true}' | write_state
    [ -f "$STATE_PATH" ]
    run cat "$STATE_PATH"
    [ "$output" = '{"active":true}' ]
}

@test "write_state leaves no temp files" {
    mkdir -p "$AGENTCORD_DATA_DIR"
    echo '{"clean":true}' | write_state
    local tmp_count
    tmp_count=$(find "$AGENTCORD_DATA_DIR" -name '*.tmp.*' | wc -l)
    [ "$tmp_count" -eq 0 ]
}

# ---------------------------------------------------------------------------
# kill_daemon
# ---------------------------------------------------------------------------

@test "kill_daemon removes PID file" {
    mkdir -p "$AGENTCORD_DATA_DIR"
    # Use a PID that almost certainly does not exist
    echo "99999999:unused" > "$PID_PATH"
    [ -f "$PID_PATH" ]
    kill_daemon
    [ ! -f "$PID_PATH" ]
}

@test "kill_daemon is no-op when no PID file" {
    # Ensure no PID file exists
    rm -f "$PID_PATH"
    run kill_daemon
    [ "$status" -eq 0 ]
}

# ---------------------------------------------------------------------------
# constants
# ---------------------------------------------------------------------------

@test "constants are all set after sourcing" {
    [ -n "$DATA_DIR_REL" ]
    [ -n "$STATE_FILE" ]
    [ -n "$PID_FILE" ]
    [ -n "$SESSIONS_DIR" ]
    [ -n "$SESSION_EXT" ]
    [ -n "$BINARY_NAME" ]
    [ -n "$STATE_VERSION" ]
}

# ---------------------------------------------------------------------------
# write_state — concurrent writes stress test
# ---------------------------------------------------------------------------

@test "write_state handles concurrent writes safely" {
    mkdir -p "$AGENTCORD_DATA_DIR"
    local pids=()
    # Launch 10 concurrent writers
    for i in $(seq 1 10); do
        (
            echo "{\"writer\":$i,\"ok\":true}" | \
                bash -c "source \"$AGENTCORD_COMMON\"; write_state"
        ) &
        pids+=($!)
    done
    # Wait for all writers
    for p in "${pids[@]}"; do
        wait "$p"
    done
    # State file must exist and contain valid JSON
    [ -f "$STATE_PATH" ]
    run jq '.' "$STATE_PATH"
    [ "$status" -eq 0 ]
    # Verify the file has the expected shape from one of the writers
    run jq -r '.ok' "$STATE_PATH"
    [ "$output" = "true" ]
    # No leftover temp files
    local tmp_count
    tmp_count=$(find "$AGENTCORD_DATA_DIR" -name '*.tmp.*' | wc -l)
    [ "$tmp_count" -eq 0 ]
}

# ---------------------------------------------------------------------------
# Trap cleanup — temp files cleaned up
# ---------------------------------------------------------------------------

@test "write_state temp file does not persist on success" {
    mkdir -p "$AGENTCORD_DATA_DIR"
    echo '{"cleanup":true}' | write_state
    # The only file should be the state file itself, no .tmp. remnants
    local all_files
    all_files=$(ls "$AGENTCORD_DATA_DIR")
    [[ "$all_files" != *".tmp."* ]]
    [ -f "$STATE_PATH" ]
}

# ---------------------------------------------------------------------------
# read_hook_input — edge cases
# ---------------------------------------------------------------------------

@test "read_hook_input exits 0 on empty stdin" {
    run bash -c '
        echo "" | {
            source "$AGENTCORD_COMMON"
            read_hook_input
            echo "should-not-reach"
        }
    '
    # read_hook_input calls exit 0 when SESSION_ID is empty
    [ "$status" -eq 0 ]
    [[ "$output" != *"should-not-reach"* ]]
}

@test "read_hook_input exits 0 when session_id is null" {
    run bash -c '
        echo "{\"other_field\":\"value\"}" | {
            source "$AGENTCORD_COMMON"
            read_hook_input
            echo "should-not-reach"
        }
    '
    [ "$status" -eq 0 ]
    [[ "$output" != *"should-not-reach"* ]]
}

@test "read_hook_input works with minimal valid JSON" {
    local result
    result=$(echo '{"session_id":"test"}' | bash -c '
        source "$AGENTCORD_COMMON"
        read_hook_input
        echo "$SESSION_ID"
    ')
    [ "$result" = "test" ]
}

# ---------------------------------------------------------------------------
# Daemon health check — PID file logic
# ---------------------------------------------------------------------------

@test "daemon health check detects running process and skips relaunch" {
    mkdir -p "$AGENTCORD_DATA_DIR"
    mkdir -p "$SESSIONS_PATH"
    # Use our own shell PID as a "running" process
    echo "$$:unused" > "$PID_PATH"
    # Verify kill -0 succeeds for our PID (process is alive)
    run kill -0 "$$"
    [ "$status" -eq 0 ]
    # Simulate the health-check logic from activity.sh
    DAEMON_ALIVE=false
    if [ -f "$PID_PATH" ]; then
        DAEMON_PID=$(cut -d: -f1 "$PID_PATH")
        if kill -0 "$DAEMON_PID" 2>/dev/null; then
            DAEMON_ALIVE=true
        fi
    fi
    [ "$DAEMON_ALIVE" = "true" ]
}

@test "daemon health check detects dead process" {
    mkdir -p "$AGENTCORD_DATA_DIR"
    # Use a PID that almost certainly does not exist
    echo "99999999:unused" > "$PID_PATH"
    DAEMON_ALIVE=false
    if [ -f "$PID_PATH" ]; then
        DAEMON_PID=$(cut -d: -f1 "$PID_PATH")
        if kill -0 "$DAEMON_PID" 2>/dev/null; then
            DAEMON_ALIVE=true
        fi
    fi
    [ "$DAEMON_ALIVE" = "false" ]
}

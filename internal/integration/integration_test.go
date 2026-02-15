// Package integration provides integration tests for the Agentcord hook scripts.
// These tests invoke the shell scripts with mock data and verify their effects
// on the filesystem.
package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"tools.zach/dev/agentcord/internal/paths"
	"tools.zach/dev/agentcord/internal/session"
)

// ///////////////////////////////////////////////
// Helpers
// ///////////////////////////////////////////////

// findBash locates a bash-compatible shell. Returns the path, or an empty
// string if none is available. Tests call t.Skip() when bash is missing.
func findBash() string {
	// Unix: prefer bash, fall back to sh
	if runtime.GOOS != "windows" {
		if p, err := exec.LookPath("bash"); err == nil {
			return p
		}
		if p, err := exec.LookPath("sh"); err == nil {
			return p
		}
		return ""
	}

	// Windows: look for Git Bash
	candidates := []string{
		filepath.Join(os.Getenv("PROGRAMFILES"), "Git", "bin", "bash.exe"),
		filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "Git", "bin", "bash.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Git", "bin", "bash.exe"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if p, err := exec.LookPath("bash"); err == nil {
		return p
	}
	return ""
}

// scriptDir returns the absolute path to the scripts/ directory.
func scriptDir(t *testing.T) string {
	t.Helper()
	// integration_test.go lives in internal/integration/
	// scripts/ lives at repo root: ../../scripts/
	dir, err := filepath.Abs(filepath.Join("..", "..", "scripts"))
	if err != nil {
		t.Fatalf("resolve scripts dir: %v", err)
	}
	return dir
}

// placeFakeDaemon creates a no-op daemon binary so hook-activity.sh skips downloads.
func placeFakeDaemon(t *testing.T, dataDir string) {
	t.Helper()
	os.MkdirAll(dataDir, 0o755)
	if runtime.GOOS == "windows" {
		os.WriteFile(filepath.Join(dataDir, "agentcord.exe"), []byte("exit 0\n"), 0o755)
	} else {
		os.WriteFile(filepath.Join(dataDir, "agentcord"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}
}

// hookEnv returns the environment variables needed for hook scripts.
func hookEnv(scripts, dataDir string) []string {
	lib := filepath.Join(scripts, "hooks", "lib", "unix")
	return append(os.Environ(),
		"AGENTCORD_DATA_DIR="+dataDir,
		"AGENTCORD_LIB="+lib,
		"AGENTCORD_COMMON="+filepath.Join(lib, "common.sh"),
		"AGENTCORD_CLIENT="+session.DefaultClient,
	)
}

// runHook runs a hook script with a JSON payload on stdin.
func runHook(t *testing.T, bash, script, dataDir, inputJSON string) {
	t.Helper()
	scripts := scriptDir(t)
	cmd := exec.Command(bash, script)
	cmd.Stdin = strings.NewReader(inputJSON)
	cmd.Env = hookEnv(scripts, dataDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("running %s: %v\noutput: %s", filepath.Base(script), err, out)
	}
}

// runHookOutput runs a hook script with a JSON payload on stdin and returns
// stdout. Unlike runHook it does not fail on a non-zero exit code — it returns
// the combined output and any error so the caller can inspect both.
func runHookOutput(t *testing.T, bash, script, dataDir, inputJSON string) (string, error) {
	t.Helper()
	scripts := scriptDir(t)
	cmd := exec.Command(bash, script)
	cmd.Stdin = strings.NewReader(inputJSON)
	cmd.Env = hookEnv(scripts, dataDir)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ///////////////////////////////////////////////
// Tests
// ///////////////////////////////////////////////

func TestStateCreation(t *testing.T) {
	bash := findBash()
	if bash == "" {
		t.Skip("bash not available")
	}

	dataDir := t.TempDir()
	scripts := scriptDir(t)
	placeFakeDaemon(t, dataDir)

	input := `{"session_id": "test-session-001"}`
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "activity.sh"), dataDir, input)

	// Verify per-client state file exists
	stateFile := paths.StateFileForClient(session.DefaultClient)
	statePath := filepath.Join(dataDir, stateFile)
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("%s not created: %v", stateFile, err)
	}

	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("parse state.json: %v", err)
	}

	// Check required fields
	if state["sessionId"] != "test-session-001" {
		t.Errorf("sessionId = %v, want test-session-001", state["sessionId"])
	}
	if state["stopped"] != false {
		t.Errorf("stopped = %v, want false", state["stopped"])
	}
	if _, ok := state["$version"]; !ok {
		t.Error("missing $version field")
	}
	if _, ok := state["sessionStart"]; !ok {
		t.Error("missing sessionStart field")
	}
	if _, ok := state["lastActivity"]; !ok {
		t.Error("missing lastActivity field")
	}
}

func TestSessionPIDFile(t *testing.T) {
	bash := findBash()
	if bash == "" {
		t.Skip("bash not available")
	}

	dataDir := t.TempDir()
	scripts := scriptDir(t)
	placeFakeDaemon(t, dataDir)

	input := `{"session_id": "pid-test-session"}`
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "activity.sh"), dataDir, input)

	pidPath := filepath.Join(dataDir, "sessions", "pid-test-session.session")
	if _, err := os.Stat(pidPath); os.IsNotExist(err) {
		t.Fatalf("session marker file not created: %s", pidPath)
	}
}

func TestSessionLifecycle(t *testing.T) {
	bash := findBash()
	if bash == "" {
		t.Skip("bash not available")
	}

	dataDir := t.TempDir()
	scripts := scriptDir(t)
	placeFakeDaemon(t, dataDir)

	sessionID := "lifecycle-session"
	input := `{"session_id": "` + sessionID + `"}`

	stateFile := paths.StateFileForClient(session.DefaultClient)

	// Activity hook creates state with stopped=false
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "activity.sh"), dataDir, input)

	data, _ := os.ReadFile(filepath.Join(dataDir, stateFile))
	var state map[string]any
	json.Unmarshal(data, &state)
	if state["stopped"] != false {
		t.Fatalf("expected stopped=false after activity, got %v", state["stopped"])
	}

	// Stop hook sets stopped=true
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "stop.sh"), dataDir, input)

	data, _ = os.ReadFile(filepath.Join(dataDir, stateFile))
	json.Unmarshal(data, &state)
	if state["stopped"] != true {
		t.Fatalf("expected stopped=true after stop, got %v", state["stopped"])
	}
}

func TestMultiSession(t *testing.T) {
	bash := findBash()
	if bash == "" {
		t.Skip("bash not available")
	}

	dataDir := t.TempDir()
	scripts := scriptDir(t)
	placeFakeDaemon(t, dataDir)

	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "activity.sh"), dataDir, `{"session_id": "session-a"}`)
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "activity.sh"), dataDir, `{"session_id": "session-b"}`)

	// Both PID files should exist
	for _, sid := range []string{"session-a", "session-b"} {
		pidPath := filepath.Join(dataDir, "sessions", sid+".session")
		if _, err := os.Stat(pidPath); os.IsNotExist(err) {
			t.Errorf("missing session file for %s", sid)
		}
	}
}

func TestStopCleanup(t *testing.T) {
	bash := findBash()
	if bash == "" {
		t.Skip("bash not available")
	}

	dataDir := t.TempDir()
	scripts := scriptDir(t)
	placeFakeDaemon(t, dataDir)

	// Create two sessions
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "activity.sh"), dataDir, `{"session_id": "keep-session"}`)
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "activity.sh"), dataDir, `{"session_id": "remove-session"}`)

	// Create a fake daemon.pid
	os.WriteFile(filepath.Join(dataDir, "daemon.pid"), []byte("99999"), 0o644)

	// End one session — daemon should remain
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "sessionend.sh"), dataDir, `{"session_id": "remove-session"}`)

	// Session marker for ended session should be gone
	removedSession := filepath.Join(dataDir, "sessions", "remove-session.session")
	if _, err := os.Stat(removedSession); !os.IsNotExist(err) {
		t.Error("expected remove-session.session to be deleted")
	}

	// Keep session and daemon should still exist
	keepSession := filepath.Join(dataDir, "sessions", "keep-session.session")
	if _, err := os.Stat(keepSession); os.IsNotExist(err) {
		t.Error("expected keep-session.session to still exist")
	}
	daemonPID := filepath.Join(dataDir, "daemon.pid")
	if _, err := os.Stat(daemonPID); os.IsNotExist(err) {
		t.Error("expected daemon.pid to still exist (session still active)")
	}

	// End last session — daemon should be killed (pid file removed)
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "sessionend.sh"), dataDir, `{"session_id": "keep-session"}`)

	if _, err := os.Stat(daemonPID); !os.IsNotExist(err) {
		t.Error("expected daemon.pid to be removed after last session ends")
	}
}

func TestSessionStartPreserved(t *testing.T) {
	bash := findBash()
	if bash == "" {
		t.Skip("bash not available")
	}

	dataDir := t.TempDir()
	scripts := scriptDir(t)
	placeFakeDaemon(t, dataDir)

	input := `{"session_id": "preserve-start"}`
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "activity.sh"), dataDir, input)

	stateFile := paths.StateFileForClient(session.DefaultClient)

	data1, _ := os.ReadFile(filepath.Join(dataDir, stateFile))
	var s1 map[string]any
	json.Unmarshal(data1, &s1)
	start1 := s1["sessionStart"].(float64)

	time.Sleep(1100 * time.Millisecond)

	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "activity.sh"), dataDir, input)

	data2, _ := os.ReadFile(filepath.Join(dataDir, stateFile))
	var s2 map[string]any
	json.Unmarshal(data2, &s2)
	start2 := s2["sessionStart"].(float64)

	if start1 != start2 {
		t.Errorf("sessionStart changed on same session: %v → %v", start1, start2)
	}
}

func TestEmptySessionIDIgnored(t *testing.T) {
	bash := findBash()
	if bash == "" {
		t.Skip("bash not available")
	}

	dataDir := t.TempDir()
	scripts := scriptDir(t)

	// No session_id in payload — should be a no-op
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "activity.sh"), dataDir, `{"no_session": "here"}`)

	// No state file should be created
	stateFile := paths.StateFileForClient(session.DefaultClient)
	if _, err := os.Stat(filepath.Join(dataDir, stateFile)); !os.IsNotExist(err) {
		t.Errorf("expected no %s for empty session_id", stateFile)
	}
}

func TestInstallDryRun(t *testing.T) {
	bash := findBash()
	if bash == "" {
		t.Skip("bash not available")
	}

	scripts := scriptDir(t)
	cmd := exec.Command(bash, filepath.Join(scripts, "install.sh"))
	cmd.Env = append(os.Environ(), "AGENTCORD_DOWNLOAD_DRY_RUN=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh dry-run: %v\noutput: %s", err, out)
	}

	url := strings.TrimSpace(string(out))
	if !strings.HasPrefix(url, "https://github.com/zachthedev/agentcord/releases/latest/download/agentcord-") {
		t.Errorf("unexpected URL: %s", url)
	}

	// Should contain OS and arch
	if !strings.Contains(url, "linux") && !strings.Contains(url, "darwin") && !strings.Contains(url, "windows") {
		t.Errorf("URL missing OS identifier: %s", url)
	}
	if !strings.Contains(url, "amd64") && !strings.Contains(url, "arm64") {
		t.Errorf("URL missing arch identifier: %s", url)
	}
}

func TestNoTempFilesAfterWrite(t *testing.T) {
	bash := findBash()
	if bash == "" {
		t.Skip("bash not available")
	}

	dataDir := t.TempDir()
	scripts := scriptDir(t)
	placeFakeDaemon(t, dataDir)

	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "activity.sh"), dataDir, `{"session_id": "atomic-test"}`)

	// Check no temp files remain
	entries, _ := os.ReadDir(dataDir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp.") {
			t.Errorf("stale temp file found: %s", e.Name())
		}
	}
}

func TestClientFieldInState(t *testing.T) {
	bash := findBash()
	if bash == "" {
		t.Skip("bash not available")
	}

	dataDir := t.TempDir()
	scripts := scriptDir(t)
	placeFakeDaemon(t, dataDir)

	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "activity.sh"), dataDir, `{"session_id": "client-test"}`)

	stateFile := paths.StateFileForClient(session.DefaultClient)
	data, err := os.ReadFile(filepath.Join(dataDir, stateFile))
	if err != nil {
		t.Fatalf("%s not created: %v", stateFile, err)
	}

	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("parse %s: %v", stateFile, err)
	}

	if state["client"] != session.DefaultClient {
		t.Errorf("client = %v, want %q", state["client"], session.DefaultClient)
	}
}

func TestSessionEndCleanup(t *testing.T) {
	bash := findBash()
	if bash == "" {
		t.Skip("bash not available")
	}

	dataDir := t.TempDir()
	scripts := scriptDir(t)
	placeFakeDaemon(t, dataDir)

	sessionID := "end-cleanup-session"
	input := `{"session_id": "` + sessionID + `"}`

	// Create session via activity hook
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "activity.sh"), dataDir, input)

	// Verify session marker exists
	markerPath := filepath.Join(dataDir, "sessions", sessionID+".session")
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		t.Fatalf("session marker not created: %s", markerPath)
	}

	// End the session
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "sessionend.sh"), dataDir, input)

	// Verify session marker is removed
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Error("expected session marker to be removed after sessionend")
	}
}

func TestSessionEndLastSession(t *testing.T) {
	bash := findBash()
	if bash == "" {
		t.Skip("bash not available")
	}

	dataDir := t.TempDir()
	scripts := scriptDir(t)
	placeFakeDaemon(t, dataDir)

	sessionID := "last-session"
	input := `{"session_id": "` + sessionID + `"}`

	// Create a single session
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "activity.sh"), dataDir, input)

	// Place a fake daemon.pid
	pidPath := filepath.Join(dataDir, "daemon.pid")
	os.WriteFile(pidPath, []byte("99999"), 0o644)

	// End the only session — daemon.pid should be cleaned up
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "sessionend.sh"), dataDir, input)

	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("expected daemon.pid to be removed after last session ends")
	}
}

func TestPostinstallOutput(t *testing.T) {
	bash := findBash()
	if bash == "" {
		t.Skip("bash not available")
	}

	// Use a data dir with no daemon binary so postinstall prints a message
	dataDir := t.TempDir()
	scripts := scriptDir(t)

	out, _ := runHookOutput(t, bash, filepath.Join(scripts, "hooks", "unix", "postinstall.sh"), dataDir, `{}`)

	if !strings.Contains(out, "[agentcord]") {
		t.Errorf("postinstall output missing [agentcord] tag, got: %s", out)
	}
}

func TestStopIdempotent(t *testing.T) {
	bash := findBash()
	if bash == "" {
		t.Skip("bash not available")
	}

	dataDir := t.TempDir()
	scripts := scriptDir(t)
	placeFakeDaemon(t, dataDir)

	sessionID := "stop-idem"
	input := `{"session_id": "` + sessionID + `"}`

	// Create a session
	runHook(t, bash, filepath.Join(scripts, "hooks", "unix", "activity.sh"), dataDir, input)

	stopScript := filepath.Join(scripts, "hooks", "unix", "stop.sh")

	// First stop — should succeed
	runHook(t, bash, stopScript, dataDir, input)

	// Second stop — should also succeed (idempotent)
	runHook(t, bash, stopScript, dataDir, input)

	// Verify state still has stopped=true
	stateFile := paths.StateFileForClient(session.DefaultClient)
	data, err := os.ReadFile(filepath.Join(dataDir, stateFile))
	if err != nil {
		t.Fatalf("%s not found: %v", stateFile, err)
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("parse %s: %v", stateFile, err)
	}
	if state["stopped"] != true {
		t.Errorf("stopped = %v, want true after double stop", state["stopped"])
	}
}

func TestCommonShConstants(t *testing.T) {
	bash := findBash()
	if bash == "" {
		t.Skip("bash not available")
	}

	scripts := scriptDir(t)
	constantsPath := filepath.Join(scripts, "hooks", "lib", "unix", "constants.sh")

	vars := []string{
		"DATA_DIR_REL",
		"STATE_FILE",
		"PID_FILE",
		"SESSIONS_DIR",
		"SESSION_EXT",
		"BINARY_NAME",
		"STATE_VERSION",
	}

	// Build a bash command that sources constants.sh and prints each variable
	// on its own line.
	var printCmds []string
	for _, v := range vars {
		printCmds = append(printCmds, "echo \"$"+v+"\"")
	}
	script := ". " + strings.ReplaceAll(constantsPath, "\\", "/") + " && " + strings.Join(printCmds, " && ")

	cmd := exec.Command(bash, "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sourcing constants.sh: %v\noutput: %s", err, out)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < len(vars) {
		t.Fatalf("expected %d lines, got %d: %s", len(vars), len(lines), out)
	}

	for i, v := range vars {
		val := strings.TrimSpace(lines[i])
		if val == "" {
			t.Errorf("constant %s is empty", v)
		}
	}
}

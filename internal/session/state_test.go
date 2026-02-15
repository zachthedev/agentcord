// Tests for session state reading, activity building, idle detection, versioning,
// migration, and template formatting. Exercises [ReadState], [BuildActivity],
// [BuildActivityWithData], [PeekVersion], and [Activity.Hash].
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// defaultTiers returns the standard model tier list used across state tests.
// It mirrors the default production tiers so test [ActivityConfig] values
// produce predictable icon and text lookups.
func defaultTiers() []string { return []string{"opus", "sonnet", "haiku"} }

// ///////////////////////////////////////////////
// ReadState Tests
// ///////////////////////////////////////////////

func TestReadState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := State{
		Version:      1,
		SessionID:    "abc-123",
		SessionStart: 1707900000,
		LastActivity: 1707900060,
		Project:      "my-project",
		Branch:       "main",
		CWD:          "C:/Users/Zach/my-project",
		GitRemoteURL: "https://github.com/zachthedev/agentcord",
		Stopped:      false,
	}
	data, _ := json.Marshal(s)
	os.WriteFile(path, data, 0o644)

	got, err := ReadState(path)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
		return
	}
	if got.SessionID != "abc-123" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "abc-123")
	}
	if got.Project != "my-project" {
		t.Errorf("Project = %q, want %q", got.Project, "my-project")
	}
	if got.Branch != "main" {
		t.Errorf("Branch = %q, want %q", got.Branch, "main")
	}
	if got.CWD != "C:/Users/Zach/my-project" {
		t.Errorf("CWD = %q, want %q", got.CWD, "C:/Users/Zach/my-project")
	}
	if got.GitRemoteURL != "https://github.com/zachthedev/agentcord" {
		t.Errorf("GitRemoteURL = %q, want %q", got.GitRemoteURL, "https://github.com/zachthedev/agentcord")
	}
	if got.Stopped {
		t.Error("Stopped = true, want false")
	}
}

func TestReadStateMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	_, err := ReadState(path)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestReadStateMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte("{broken json"), 0o644)

	got, err := ReadState(path)
	if err == nil {
		t.Fatal("ReadState should return error for malformed JSON")
	}

	// Should have backed up to .corrupted
	corruptedPath := path + ".corrupted"
	if _, statErr := os.Stat(corruptedPath); os.IsNotExist(statErr) {
		t.Error("expected .corrupted backup file to exist")
	}

	// Should still return a fresh default state
	if got == nil {
		t.Fatal("expected non-nil state even with corrupted JSON")
		return
	}
	if got.Version != CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, CurrentVersion)
	}
}

// ///////////////////////////////////////////////
// BuildActivity Tests
// ///////////////////////////////////////////////

func TestBuildActivity(t *testing.T) {
	s := &State{
		Version:      1,
		SessionID:    "abc-123",
		SessionStart: time.Now().Unix() - 60,
		LastActivity: time.Now().Unix(),
		Project:      "my-project",
		Branch:       "main",
		CWD:          "C:/Users/Zach/my-project",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Branch: {branch} | {cost}",
		DetailsNoBranchFormat: "Working on {project}",
		StateNoCostFormat:     "Branch: {branch}",
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		ShowBranch:            true,
		ShowCost:              false,
		ShowTokens:            false,
		ShowRepoButton:        false,
		TimestampMode:         "session",
		IdleMinutes:           15,
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivity(s, cfg)
	if a == nil {
		t.Fatal("BuildActivity returned nil")
		return
	}
	if a.Details != "Working on my-project" {
		t.Errorf("Details = %q, want %q", a.Details, "Working on my-project")
	}
	if a.State != "Branch: main" {
		t.Errorf("State = %q, want %q", a.State, "Branch: main")
	}
	if a.Assets.LargeImage != "claude" {
		t.Errorf("LargeImage = %q, want %q", a.Assets.LargeImage, "claude")
	}
}

func TestBuildActivityNoBranch(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 60,
		LastActivity: time.Now().Unix(),
		Project:      "my-project",
		Branch:       "",
		CWD:          "/tmp/my-project",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Branch: {branch}",
		DetailsNoBranchFormat: "Editing {project}",
		StateNoCostFormat:     "Coding",
		ShowBranch:            true,
		ShowCost:              false,
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "session",
		IdleMinutes:           15,
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivity(s, cfg)
	if a == nil {
		t.Fatal("BuildActivity returned nil")
		return
	}
	if a.Details != "Editing my-project" {
		t.Errorf("Details = %q, want %q", a.Details, "Editing my-project")
	}
}

func TestBuildActivityWithCost(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 60,
		LastActivity: time.Now().Unix(),
		Project:      "my-project",
		Branch:       "main",
		CWD:          "/tmp/my-project",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Cost: {cost} | {tokens} tokens",
		DetailsNoBranchFormat: "Working on {project}",
		StateNoCostFormat:     "Coding",
		ShowBranch:            true,
		ShowCost:              true,
		ShowTokens:            true,
		CostFormat:            "%.2f",
		TokenFormat:           "short",
		ModelFormat:           "short",
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "session",
		IdleMinutes:           15,
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivityWithData(s, cfg, 1.2345, 150000, "claude-sonnet-4-5-20250929", nil)
	if a == nil {
		t.Fatal("BuildActivity returned nil")
		return
	}
	if a.State != "Cost: $1.23 | 150K tokens" {
		t.Errorf("State = %q, want %q", a.State, "Cost: $1.23 | 150K tokens")
	}
}

func TestBuildActivityNoCost(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 60,
		LastActivity: time.Now().Unix(),
		Project:      "my-project",
		Branch:       "main",
		CWD:          "/tmp/my-project",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Cost: {cost}",
		DetailsNoBranchFormat: "Working on {project}",
		StateNoCostFormat:     "Just coding",
		ShowBranch:            true,
		ShowCost:              false,
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "session",
		IdleMinutes:           15,
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivity(s, cfg)
	if a == nil {
		t.Fatal("BuildActivity returned nil")
		return
	}
	if a.State != "Just coding" {
		t.Errorf("State = %q, want %q", a.State, "Just coding")
	}
}

func TestBuildActivityCustomFormat(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 60,
		LastActivity: time.Now().Unix(),
		Project:      "cool-app",
		Branch:       "feature/xyz",
		CWD:          "/tmp/cool-app",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "{project} ({branch})",
		StateFormat:           "Model: {model} | {cost}",
		DetailsNoBranchFormat: "{project}",
		StateNoCostFormat:     "Model: {model}",
		ShowBranch:            true,
		ShowCost:              false,
		ModelFormat:           "short",
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "session",
		IdleMinutes:           15,
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivityWithData(s, cfg, 0, 0, "claude-opus-4-6", nil)
	if a == nil {
		t.Fatal("BuildActivity returned nil")
		return
	}
	if a.Details != "cool-app (feature/xyz)" {
		t.Errorf("Details = %q, want %q", a.Details, "cool-app (feature/xyz)")
	}
	if a.State != "Model: Opus 4.6" {
		t.Errorf("State = %q, want %q", a.State, "Model: Opus 4.6")
	}
}

func TestBuildActivityStopped(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 60,
		LastActivity: time.Now().Unix(),
		Project:      "my-project",
		Branch:       "main",
		CWD:          "/tmp/my-project",
		Stopped:      true,
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Branch: {branch}",
		DetailsNoBranchFormat: "Working on {project}",
		StateNoCostFormat:     "Coding",
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "session",
		IdleMinutes:           15,
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivity(s, cfg)
	if a != nil {
		t.Errorf("expected nil for stopped state, got %+v", a)
	}
}

func TestBuildActivityIgnoredDir(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 60,
		LastActivity: time.Now().Unix(),
		Project:      "secret-project",
		Branch:       "main",
		CWD:          "/tmp/secret-project",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Branch: {branch}",
		DetailsNoBranchFormat: "Working on {project}",
		StateNoCostFormat:     "Coding",
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "session",
		IdleMinutes:           15,
		IgnoredPatterns:       []string{"/tmp/secret-*"},
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivity(s, cfg)
	if a != nil {
		t.Errorf("expected nil for ignored dir, got %+v", a)
	}
}

func TestBuildActivityHiddenProject(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 60,
		LastActivity: time.Now().Unix(),
		Project:      "private-project",
		Branch:       "main",
		CWD:          "/tmp/private-project",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Branch: {branch}",
		DetailsNoBranchFormat: "Working on {project}",
		StateNoCostFormat:     "Coding",
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		ShowBranch:            true,
		TimestampMode:         "session",
		IdleMinutes:           15,
		ProjectName:           "a project",
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivity(s, cfg)
	if a == nil {
		t.Fatal("BuildActivity returned nil")
		return
	}
	if a.Details != "Working on a project" {
		t.Errorf("Details = %q, want %q", a.Details, "Working on a project")
	}
}

// ///////////////////////////////////////////////
// Button Tests
// ///////////////////////////////////////////////

func TestBuildActivityWithRepoButton(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 60,
		LastActivity: time.Now().Unix(),
		Project:      "my-project",
		Branch:       "main",
		CWD:          "/tmp/my-project",
		GitRemoteURL: "https://github.com/zachthedev/agentcord",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Branch: {branch}",
		DetailsNoBranchFormat: "Working on {project}",
		StateNoCostFormat:     "Coding",
		ShowBranch:            true,
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "session",
		IdleMinutes:           15,
		ShowRepoButton:        true,
		RepoButtonLabel:       "View Repo",
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivity(s, cfg)
	if a == nil {
		t.Fatal("BuildActivity returned nil")
		return
	}
	if len(a.Buttons) != 1 {
		t.Fatalf("Buttons count = %d, want 1", len(a.Buttons))
		return
	}
	if a.Buttons[0].Label != "View Repo" {
		t.Errorf("Button label = %q, want %q", a.Buttons[0].Label, "View Repo")
	}
	if a.Buttons[0].URL != "https://github.com/zachthedev/agentcord" {
		t.Errorf("Button URL = %q, want %q", a.Buttons[0].URL, "https://github.com/zachthedev/agentcord")
	}
}

func TestBuildActivityNoRemote(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 60,
		LastActivity: time.Now().Unix(),
		Project:      "my-project",
		Branch:       "main",
		CWD:          "/tmp/my-project",
		GitRemoteURL: "",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Branch: {branch}",
		DetailsNoBranchFormat: "Working on {project}",
		StateNoCostFormat:     "Coding",
		ShowBranch:            true,
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "session",
		IdleMinutes:           15,
		ShowRepoButton:        true,
		RepoButtonLabel:       "View Repo",
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivity(s, cfg)
	if a == nil {
		t.Fatal("BuildActivity returned nil")
		return
	}
	if len(a.Buttons) != 0 {
		t.Errorf("Buttons count = %d, want 0 (no remote)", len(a.Buttons))
	}
}

func TestBuildActivityModelIcon(t *testing.T) {
	tests := []struct {
		model    string
		wantIcon string
		wantText string
	}{
		{"claude-opus-4-6", "opus", "Opus 4.6"},
		{"claude-sonnet-4-5-20250929", "sonnet", "Sonnet 4.5.20250929"},
		{"claude-haiku-4-5-20251001", "haiku", "Haiku 4.5.20251001"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			s := &State{
				Version:      1,
				SessionStart: time.Now().Unix() - 60,
				LastActivity: time.Now().Unix(),
				Project:      "my-project",
				Branch:       "main",
				CWD:          "/tmp/my-project",
			}
			cfg := ActivityConfig{
				DetailsFormat:         "Working on {project}",
				StateFormat:           "Branch: {branch}",
				DetailsNoBranchFormat: "Working on {project}",
				StateNoCostFormat:     "Coding",
				ShowBranch:            true,
				LargeImage:            "claude",
				LargeText:             "Claude Code",
				ShowModelIcon:         true,
				ModelFormat:           "short",
				TimestampMode:         "session",
				IdleMinutes:           15,
				ModelTiers:            defaultTiers(),
				DefaultTierIcon:       "default",
			}

			a := BuildActivityWithData(s, cfg, 0, 0, tt.model, nil)
			if a == nil {
				t.Fatal("BuildActivity returned nil")
				return
			}
			if a.Assets.SmallImage != tt.wantIcon {
				t.Errorf("SmallImage = %q, want %q", a.Assets.SmallImage, tt.wantIcon)
			}
			if a.Assets.SmallText != tt.wantText {
				t.Errorf("SmallText = %q, want %q", a.Assets.SmallText, tt.wantText)
			}
		})
	}
}

// ///////////////////////////////////////////////
// Activity.Hash Tests
// ///////////////////////////////////////////////

func TestStateDedup(t *testing.T) {
	a1 := &Activity{
		Details: "Working on project",
		State:   "Branch: main",
		Assets:  Assets{LargeImage: "claude"},
	}
	a2 := &Activity{
		Details: "Working on project",
		State:   "Branch: main",
		Assets:  Assets{LargeImage: "claude"},
	}
	a3 := &Activity{
		Details: "Working on OTHER",
		State:   "Branch: dev",
		Assets:  Assets{LargeImage: "claude"},
	}

	h1 := a1.Hash()
	h2 := a2.Hash()
	h3 := a3.Hash()

	if h1 != h2 {
		t.Errorf("same activity produced different hashes: %q vs %q", h1, h2)
	}
	if h1 == h3 {
		t.Error("different activities produced the same hash")
	}
}

// ///////////////////////////////////////////////
// Idle and Presence Tests
// ///////////////////////////////////////////////

func TestPresenceIdle(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 3600,
		LastActivity: time.Now().Unix() - 1800, // 30 minutes ago
		Project:      "my-project",
		Branch:       "main",
		CWD:          "/tmp/my-project",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Branch: {branch}",
		DetailsNoBranchFormat: "Working on {project}",
		StateNoCostFormat:     "Coding",
		ShowBranch:            true,
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "session",
		IdleMinutes:           15,
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivity(s, cfg)
	if a != nil {
		t.Errorf("expected nil for idle state, got %+v", a)
	}
}

func TestPresenceIdleText(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 3600,
		LastActivity: time.Now().Unix() - 1800,
		Project:      "my-project",
		Branch:       "main",
		CWD:          "/tmp/my-project",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Branch: {branch}",
		DetailsNoBranchFormat: "Working on {project}",
		StateNoCostFormat:     "Coding",
		ShowBranch:            true,
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "session",
		IdleMinutes:           15,
		IdleMode:              "idle_text",
		IdleDetails:           "Away from keyboard",
		IdleState:             "Idle",
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivity(s, cfg)
	if a == nil {
		t.Fatal("expected idle_text activity, got nil")
		return
	}
	if a.Details != "Away from keyboard" {
		t.Errorf("Details = %q, want %q", a.Details, "Away from keyboard")
	}
	if a.State != "Idle" {
		t.Errorf("State = %q, want %q", a.State, "Idle")
	}
}

func TestPresenceResume(t *testing.T) {
	sessionStart := time.Now().Unix() - 3600
	s := &State{
		Version:      1,
		SessionStart: sessionStart,
		LastActivity: time.Now().Unix(), // just now
		Project:      "my-project",
		Branch:       "main",
		CWD:          "/tmp/my-project",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Branch: {branch}",
		DetailsNoBranchFormat: "Working on {project}",
		StateNoCostFormat:     "Coding",
		ShowBranch:            true,
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "session",
		IdleMinutes:           15,
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivity(s, cfg)
	if a == nil {
		t.Fatal("expected activity after idle clears, got nil")
		return
	}
	if a.Timestamps.Start != sessionStart {
		t.Errorf("Timestamps.Start = %d, want %d (original sessionStart)", a.Timestamps.Start, sessionStart)
	}
}

// ///////////////////////////////////////////////
// State Version and Migration Tests
// ///////////////////////////////////////////////

func TestStateVersionPeek(t *testing.T) {
	data := []byte(`{"$version":1,"sessionId":"abc"}`)
	v, err := PeekVersion(data)
	if err != nil {
		t.Fatalf("PeekVersion: %v", err)
		return
	}
	if v != 1 {
		t.Errorf("PeekVersion = %d, want 1", v)
	}
}

func TestStateVersionMissing(t *testing.T) {
	data := []byte(`{"sessionId":"abc"}`)
	v, err := PeekVersion(data)
	if err != nil {
		t.Fatalf("PeekVersion: %v", err)
		return
	}
	if v != 1 {
		t.Errorf("PeekVersion with missing $version = %d, want 1 (normalized from 0)", v)
	}
}

func TestStateVersionInvalid(t *testing.T) {
	data := []byte(`{broken json`)
	_, err := PeekVersion(data)
	if err == nil {
		t.Fatal("expected error for unparseable JSON")
	}
}

func TestStateMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Write a v1 state, register a migration from v1->v2
	origMigrations := StateMigrations
	defer func() { StateMigrations = origMigrations }()

	StateMigrations = append(StateMigrations, MigrationEntry{
		Version:     2,
		Description: "test migration",
		Upgrade: func(data []byte) ([]byte, error) {
			var m map[string]any
			json.Unmarshal(data, &m)
			m["$version"] = float64(2)
			return json.Marshal(m)
		},
	})

	os.WriteFile(path, []byte(`{"$version":1,"sessionId":"abc-123","sessionStart":1707900000,"lastActivity":1707900060,"project":"test","branch":"main","cwd":"/tmp","stopped":false}`), 0o644)

	got, err := ReadState(path)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
		return
	}
	if got.SessionID != "abc-123" {
		t.Errorf("SessionID = %q, want %q after migration", got.SessionID, "abc-123")
	}
}

func TestStateCorruptedBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	badJSON := []byte("{this is not valid json at all")
	os.WriteFile(path, badJSON, 0o644)

	got, err := ReadState(path)
	if err == nil {
		t.Fatal("ReadState should return error for corrupted state")
	}

	// Verify backup was created
	corruptedPath := path + ".corrupted"
	backed, readErr := os.ReadFile(corruptedPath)
	if readErr != nil {
		t.Fatalf("expected corrupted backup file: %v", readErr)
		return
	}
	if string(backed) != string(badJSON) {
		t.Errorf("backup content = %q, want %q", string(backed), string(badJSON))
	}

	// Verify a fresh state was returned
	if got == nil {
		t.Fatal("expected non-nil state even with corrupted JSON")
		return
	}
	if got.Version != CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, CurrentVersion)
	}
}

func TestStateFutureVersionNormalized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	future := `{"$version":999,"sessionId":"future-abc","sessionStart":1707900000,"lastActivity":1707900060,"project":"future","branch":"main","cwd":"/tmp","stopped":false}`
	os.WriteFile(path, []byte(future), 0o644)

	got, err := ReadState(path)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
		return
	}
	if got.Version != CurrentVersion {
		t.Errorf("Version = %d, want %d (normalized from future)", got.Version, CurrentVersion)
	}
	if got.SessionID != "future-abc" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "future-abc")
	}

	// Verify backup was created with version in name
	bakPath := fmt.Sprintf("%s.v%d.bak", path, 999)
	if _, statErr := os.Stat(bakPath); os.IsNotExist(statErr) {
		t.Error("expected .v999.bak backup file to exist")
	}

	// Verify the file was re-saved with current version
	data, _ := os.ReadFile(path)
	var check State
	json.Unmarshal(data, &check)
	if check.Version != CurrentVersion {
		t.Errorf("re-saved Version = %d, want %d", check.Version, CurrentVersion)
	}
}

// ///////////////////////////////////////////////
// Template Format Syntax Tests
// ///////////////////////////////////////////////

func TestTemplateModelFormats(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 60,
		LastActivity: time.Now().Unix(),
		Project:      "myapp",
		Branch:       "main",
		CWD:          "/tmp/myapp",
	}

	tests := []struct {
		name           string
		stateNoCostFmt string
		modelFmt       string
		model          string
		want           string
	}{
		{
			name:           "model:short",
			stateNoCostFmt: "{model:short}",
			modelFmt:       "full", // default should be overridden by :short
			model:          "claude-opus-4-6",
			want:           "Opus 4.6",
		},
		{
			name:           "model:full",
			stateNoCostFmt: "{model:full}",
			modelFmt:       "short",
			model:          "claude-opus-4-6",
			want:           "Claude Opus 4.6",
		},
		{
			name:           "model:raw",
			stateNoCostFmt: "{model:raw}",
			modelFmt:       "short",
			model:          "claude-opus-4-6",
			want:           "claude-opus-4-6",
		},
		{
			name:           "model default",
			stateNoCostFmt: "{model}",
			modelFmt:       "full",
			model:          "claude-opus-4-6",
			want:           "Claude Opus 4.6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ActivityConfig{
				DetailsFormat:         "{project}",
				StateFormat:           tt.stateNoCostFmt,
				DetailsNoBranchFormat: "{project}",
				StateNoCostFormat:     tt.stateNoCostFmt,
				ShowBranch:            true,
				ShowCost:              false,
				ModelFormat:           tt.modelFmt,
				LargeImage:            "claude",
				LargeText:             "Claude Code",
				TimestampMode:         "session",
				IdleMinutes:           15,
				ModelTiers:            defaultTiers(),
				DefaultTierIcon:       "default",
			}

			a := BuildActivityWithData(s, cfg, 0, 0, tt.model, nil)
			if a == nil {
				t.Fatal("BuildActivity returned nil")
				return
			}
			if a.State != tt.want {
				t.Errorf("State = %q, want %q", a.State, tt.want)
			}
		})
	}
}

func TestTemplateTokenFormats(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 60,
		LastActivity: time.Now().Unix(),
		Project:      "myapp",
		Branch:       "main",
		CWD:          "/tmp/myapp",
	}

	tests := []struct {
		name     string
		stateFmt string
		tokenFmt string
		tokens   int64
		want     string
	}{
		{"tokens:short", "{tokens:short}", "full", 1500000, "1.5M"},
		{"tokens:full", "{tokens:full}", "short", 1500000, "1,500,000"},
		{"tokens default short", "{tokens}", "short", 1500000, "1.5M"},
		{"tokens default full", "{tokens}", "full", 1500000, "1,500,000"},
		{"tokens exact K", "{tokens:short}", "short", 1000, "1K"},
		{"tokens exact M", "{tokens:short}", "short", 2000000, "2M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ActivityConfig{
				DetailsFormat:         "{project}",
				StateFormat:           tt.stateFmt,
				DetailsNoBranchFormat: "{project}",
				StateNoCostFormat:     tt.stateFmt,
				ShowBranch:            true,
				ShowCost:              true,
				TokenFormat:           tt.tokenFmt,
				CostFormat:            "%.2f",
				ModelFormat:           "short",
				LargeImage:            "claude",
				LargeText:             "Claude Code",
				TimestampMode:         "session",
				IdleMinutes:           15,
				ModelTiers:            defaultTiers(),
				DefaultTierIcon:       "default",
			}

			a := BuildActivityWithData(s, cfg, 1.0, tt.tokens, "claude-opus-4-6", nil)
			if a == nil {
				t.Fatal("BuildActivity returned nil")
				return
			}
			if a.State != tt.want {
				t.Errorf("State = %q, want %q", a.State, tt.want)
			}
		})
	}
}

func TestCostThreshold(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 60,
		LastActivity: time.Now().Unix(),
		Project:      "myapp",
		Branch:       "main",
		CWD:          "/tmp/myapp",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "{project}",
		StateFormat:           "Cost: {cost}",
		DetailsNoBranchFormat: "{project}",
		StateNoCostFormat:     "No cost",
		ShowBranch:            true,
		ShowCost:              true,
		CostFormat:            "%.2f",
		ModelFormat:           "short",
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "session",
		IdleMinutes:           15,
		CostShowThreshold:     1.0,
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	// Below threshold: should use no-cost template
	a := BuildActivityWithData(s, cfg, 0.50, 1000, "claude-opus-4-6", nil)
	if a == nil {
		t.Fatal("BuildActivity returned nil")
		return
	}
	if a.State != "No cost" {
		t.Errorf("State = %q, want %q (below threshold)", a.State, "No cost")
	}

	// Above threshold: should use cost template
	a = BuildActivityWithData(s, cfg, 2.50, 1000, "claude-opus-4-6", nil)
	if a == nil {
		t.Fatal("BuildActivity returned nil")
		return
	}
	if a.State != "Cost: $2.50" {
		t.Errorf("State = %q, want %q (above threshold)", a.State, "Cost: $2.50")
	}
}

func TestBuildActivityCustomButton(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 60,
		LastActivity: time.Now().Unix(),
		Project:      "my-project",
		Branch:       "main",
		CWD:          "/tmp/my-project",
		GitRemoteURL: "https://github.com/zachthedev/agentcord",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Branch: {branch}",
		DetailsNoBranchFormat: "Working on {project}",
		StateNoCostFormat:     "Coding",
		ShowBranch:            true,
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "session",
		IdleMinutes:           15,
		ShowRepoButton:        true,
		RepoButtonLabel:       "View Repo",
		CustomButtonLabel:     "My Website",
		CustomButtonURL:       "https://example.com",
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivity(s, cfg)
	if a == nil {
		t.Fatal("BuildActivity returned nil")
		return
	}
	if len(a.Buttons) != 2 {
		t.Fatalf("Buttons count = %d, want 2", len(a.Buttons))
		return
	}
	if a.Buttons[0].Label != "View Repo" {
		t.Errorf("Button[0] label = %q, want %q", a.Buttons[0].Label, "View Repo")
	}
	if a.Buttons[1].Label != "My Website" {
		t.Errorf("Button[1] label = %q, want %q", a.Buttons[1].Label, "My Website")
	}
	if a.Buttons[1].URL != "https://example.com" {
		t.Errorf("Button[1] URL = %q, want %q", a.Buttons[1].URL, "https://example.com")
	}
}

func TestBuildActivityCustomButtonOnly(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 60,
		LastActivity: time.Now().Unix(),
		Project:      "my-project",
		Branch:       "main",
		CWD:          "/tmp/my-project",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Branch: {branch}",
		DetailsNoBranchFormat: "Working on {project}",
		StateNoCostFormat:     "Coding",
		ShowBranch:            true,
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "session",
		IdleMinutes:           15,
		ShowRepoButton:        false,
		CustomButtonLabel:     "My Website",
		CustomButtonURL:       "https://example.com",
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivity(s, cfg)
	if a == nil {
		t.Fatal("BuildActivity returned nil")
		return
	}
	if len(a.Buttons) != 1 {
		t.Fatalf("Buttons count = %d, want 1", len(a.Buttons))
		return
	}
	if a.Buttons[0].Label != "My Website" {
		t.Errorf("Button label = %q, want %q", a.Buttons[0].Label, "My Website")
	}
}

func TestTokensShowThreshold(t *testing.T) {
	tests := []struct {
		name      string
		tokens    int64
		threshold int64
		wantState string
	}{
		{
			name:      "below threshold hides tokens",
			tokens:    500,
			threshold: 1000,
			wantState: "Cost: $1.00 | 0 tokens",
		},
		{
			name:      "above threshold shows tokens",
			tokens:    1500,
			threshold: 1000,
			wantState: "Cost: $1.00 | 1.5K tokens",
		},
		{
			name:      "at threshold shows tokens",
			tokens:    1000,
			threshold: 1000,
			wantState: "Cost: $1.00 | 1K tokens",
		},
		{
			name:      "no threshold shows all",
			tokens:    500,
			threshold: 0,
			wantState: "Cost: $1.00 | 500 tokens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &State{
				Version:      1,
				SessionStart: time.Now().Unix() - 60,
				LastActivity: time.Now().Unix(),
				Project:      "myapp",
				Branch:       "main",
				CWD:          "/tmp/myapp",
			}
			cfg := ActivityConfig{
				DetailsFormat:         "{project}",
				StateFormat:           "Cost: {cost} | {tokens} tokens",
				DetailsNoBranchFormat: "{project}",
				StateNoCostFormat:     "{tokens} tokens",
				ShowBranch:            true,
				ShowCost:              true,
				ShowTokens:            true,
				CostFormat:            "%.2f",
				TokenFormat:           "short",
				ModelFormat:           "short",
				LargeImage:            "claude",
				LargeText:             "Claude Code",
				TimestampMode:         "session",
				IdleMinutes:           15,
				TokensShowThreshold:   tt.threshold,
				ModelTiers:            defaultTiers(),
				DefaultTierIcon:       "default",
			}

			a := BuildActivityWithData(s, cfg, 1.00, tt.tokens, "claude-opus-4-6", nil)
			if a == nil {
				t.Fatal("BuildActivity returned nil")
				return
			}
			if a.State != tt.wantState {
				t.Errorf("State = %q, want %q", a.State, tt.wantState)
			}
		})
	}
}

func TestBuildActivityTimestampModeNone(t *testing.T) {
	sessionStart := time.Now().Unix() - 120
	s := &State{
		Version:      1,
		SessionStart: sessionStart,
		LastActivity: time.Now().Unix(),
		Project:      "my-project",
		Branch:       "main",
		CWD:          "/tmp/my-project",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Branch: {branch}",
		DetailsNoBranchFormat: "Working on {project}",
		StateNoCostFormat:     "Coding",
		ShowBranch:            true,
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "none",
		IdleMinutes:           15,
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	a := BuildActivityWithData(s, cfg, 0, 0, "", nil)
	if a == nil {
		t.Fatal("BuildActivityWithData returned nil")
		return
	}
	// TimestampMode "none" is not handled specially by BuildActivityWithData;
	// it always sets Timestamps.Start to SessionStart. This confirms current behavior.
	if a.Timestamps.Start != sessionStart {
		t.Errorf("Timestamps.Start = %d, want %d (sessionStart)", a.Timestamps.Start, sessionStart)
	}
}

func TestIdleModeLastActivity(t *testing.T) {
	s := &State{
		Version:      1,
		SessionStart: time.Now().Unix() - 3600,
		LastActivity: time.Now().Unix() - 1800, // 30 minutes ago, well past idle
		Project:      "my-project",
		Branch:       "main",
		CWD:          "/tmp/my-project",
	}
	cfg := ActivityConfig{
		DetailsFormat:         "Working on {project}",
		StateFormat:           "Branch: {branch}",
		DetailsNoBranchFormat: "Working on {project}",
		StateNoCostFormat:     "Coding",
		ShowBranch:            true,
		LargeImage:            "claude",
		LargeText:             "Claude Code",
		TimestampMode:         "session",
		IdleMinutes:           15,
		IdleMode:              "last_activity",
		ModelTiers:            defaultTiers(),
		DefaultTierIcon:       "default",
	}

	// In "last_activity" mode, BuildActivity returns nil to signal the caller
	// should use the cached last activity rather than clearing presence.
	a := BuildActivity(s, cfg)
	if a != nil {
		t.Errorf("expected nil for last_activity idle mode, got %+v", a)
	}
}

// ///////////////////////////////////////////////
// Model Tier Tests
// ///////////////////////////////////////////////

func TestExtractModelTier(t *testing.T) {
	tests := []struct {
		model    string
		tiers    []string
		fallback string
		want     string
	}{
		{"claude-opus-4-6", []string{"opus", "sonnet", "haiku"}, "claude", "opus"},
		{"claude-sonnet-4-5-20250929", []string{"opus", "sonnet", "haiku"}, "claude", "sonnet"},
		{"claude-haiku-4-5-20251001", []string{"opus", "sonnet", "haiku"}, "claude", "haiku"},
		{"claude-unknown-1-0", []string{"opus", "sonnet", "haiku"}, "claude", "claude"},
		{"claude-nova-5-0", []string{"opus", "sonnet", "haiku", "nova"}, "claude", "nova"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := extractModelTier(tt.model, tt.tiers, tt.fallback)
			if got != tt.want {
				t.Errorf("extractModelTier(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestExtractModelTier_EmptyTierList(t *testing.T) {
	got := extractModelTier("claude-opus-4-6", nil, "default-icon")
	if got != "default-icon" {
		t.Errorf("extractModelTier with empty tiers = %q, want %q", got, "default-icon")
	}
}

func TestExtractModelTier_MatchesFirst(t *testing.T) {
	got := extractModelTier("claude-opus-4-6", []string{"opus", "sonnet"}, "claude")
	if got != "opus" {
		t.Errorf("extractModelTier = %q, want %q", got, "opus")
	}
}

func TestHash_NilActivity(t *testing.T) {
	var a *Activity
	got := a.Hash()
	if got != "" {
		t.Errorf("Hash of nil activity = %q, want empty string", got)
	}
}

func TestHash_DeterministicForSameInput(t *testing.T) {
	a := &Activity{
		Details: "Working on project",
		State:   "Branch: main",
		Assets:  Assets{LargeImage: "claude"},
	}
	h1 := a.Hash()
	h2 := a.Hash()
	if h1 != h2 {
		t.Errorf("same activity produced different hashes: %q vs %q", h1, h2)
	}
	if h1 == "" {
		t.Error("expected non-empty hash for non-nil activity")
	}
}

func TestHash_DifferentForDifferentInput(t *testing.T) {
	a1 := &Activity{
		Details: "Working on project-A",
		State:   "Branch: main",
	}
	a2 := &Activity{
		Details: "Working on project-B",
		State:   "Branch: dev",
	}
	h1 := a1.Hash()
	h2 := a2.Hash()
	if h1 == h2 {
		t.Error("different activities produced the same hash")
	}
}

// ///////////////////////////////////////////////
// Client Field Tests
// ///////////////////////////////////////////////

func TestDefaultClientConstant(t *testing.T) {
	if DefaultClient != "unknown" {
		t.Errorf("DefaultClient = %q, want %q", DefaultClient, "unknown")
	}
}

func TestCurrentVersionConstant(t *testing.T) {
	if CurrentVersion != 1 {
		t.Errorf("CurrentVersion = %d, want %d", CurrentVersion, 1)
	}
}

func TestClientFieldJSONRoundTrip(t *testing.T) {
	original := State{
		Version:      CurrentVersion,
		SessionID:    "test-123",
		SessionStart: 1707900000,
		LastActivity: 1707900060,
		Project:      "my-project",
		Branch:       "main",
		CWD:          "/tmp/test",
		Client:       "code",
		Stopped:      false,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var roundTripped State
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if roundTripped.Client != original.Client {
		t.Errorf("Client after round-trip = %q, want %q", roundTripped.Client, original.Client)
	}
	if roundTripped.SessionID != original.SessionID {
		t.Errorf("SessionID after round-trip = %q, want %q", roundTripped.SessionID, original.SessionID)
	}
	if roundTripped.Version != original.Version {
		t.Errorf("Version after round-trip = %d, want %d", roundTripped.Version, original.Version)
	}
}

func TestClientFieldJSONKey(t *testing.T) {
	// Verify the JSON key is "client" (not "Client" or something else).
	s := State{Client: "code"}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	val, ok := raw["client"]
	if !ok {
		t.Fatal("expected 'client' key in JSON output")
	}
	if val != "code" {
		t.Errorf("JSON client = %v, want %q", val, "code")
	}
}

func TestReadStatePreservesClient(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := State{
		Version:      CurrentVersion,
		SessionID:    "sess-456",
		SessionStart: 1707900000,
		LastActivity: 1707900060,
		Project:      "test-project",
		Branch:       "feature",
		CWD:          "/tmp/test",
		Client:       "code",
		Stopped:      false,
	}
	data, _ := json.Marshal(s)
	os.WriteFile(path, data, 0o644)

	got, err := ReadState(path)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if got.Client != "code" {
		t.Errorf("Client = %q, want %q", got.Client, "code")
	}
}

func TestReadStateEmptyClient(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Write a state with no client field set.
	s := State{
		Version:      CurrentVersion,
		SessionID:    "sess-789",
		SessionStart: 1707900000,
		LastActivity: 1707900060,
		Project:      "test-project",
		Branch:       "main",
		CWD:          "/tmp/test",
	}
	data, _ := json.Marshal(s)
	os.WriteFile(path, data, 0o644)

	got, err := ReadState(path)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	// Client should be empty string when not set (the field has no default in ReadState).
	if got.Client != "" {
		t.Errorf("Client = %q, want empty string when not set", got.Client)
	}
}

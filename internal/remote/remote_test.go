package remote

import (
	"testing"
)

// ///////////////////////////////////////////////
// githubRemoteRe Tests
// ///////////////////////////////////////////////

func TestGithubRemoteReMatches(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOwner string
		wantRepo  string
	}{
		{
			name:      "HTTPS URL",
			input:     "https://github.com/user/repo",
			wantOwner: "user",
			wantRepo:  "repo",
		},
		{
			name:      "HTTPS URL with .git",
			input:     "https://github.com/user/repo.git",
			wantOwner: "user",
			wantRepo:  "repo",
		},
		{
			name:      "SSH URL",
			input:     "git@github.com:user/repo.git",
			wantOwner: "user",
			wantRepo:  "repo",
		},
		{
			name:      "SSH URL without .git",
			input:     "git@github.com:user/repo",
			wantOwner: "user",
			wantRepo:  "repo",
		},
		{
			name:      "HTTPS with org name",
			input:     "https://github.com/my-org/my-project",
			wantOwner: "my-org",
			wantRepo:  "my-project",
		},
		{
			name:      "SSH with org name",
			input:     "git@github.com:my-org/my-project.git",
			wantOwner: "my-org",
			wantRepo:  "my-project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := githubRemoteRe.FindStringSubmatch(tt.input)
			if len(m) != 3 {
				t.Fatalf("expected 3 groups, got %d: %v", len(m), m)
			}
			if m[1] != tt.wantOwner {
				t.Errorf("owner = %q, want %q", m[1], tt.wantOwner)
			}
			if m[2] != tt.wantRepo {
				t.Errorf("repo = %q, want %q", m[2], tt.wantRepo)
			}
		})
	}
}

func TestGithubRemoteReNoMatch(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"GitLab HTTPS", "https://gitlab.com/user/repo"},
		{"GitLab SSH", "git@gitlab.com:user/repo.git"},
		{"Bitbucket HTTPS", "https://bitbucket.org/user/repo"},
		{"random string", "just some text"},
		{"empty string", ""},
		{"partial URL", "github.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := githubRemoteRe.FindStringSubmatch(tt.input)
			if len(m) == 3 {
				t.Errorf("expected no match for %q, but got owner=%q repo=%q", tt.input, m[1], m[2])
			}
		})
	}
}

// ///////////////////////////////////////////////
// RawURL Tests
// ///////////////////////////////////////////////

// setOwnerRepo overrides the package-level owner and repo for testing.
// It first triggers ensureInit so the sync.Once is consumed (preventing
// git commands from running during test), then sets the desired values.
// Original values are restored via t.Cleanup.
func setOwnerRepo(t *testing.T, o, r string) {
	t.Helper()

	// Ensure initOnce is consumed so ensureInit is a no-op.
	ensureInit()

	origOwner, origRepo := owner, repo
	owner = o
	repo = r

	t.Cleanup(func() {
		owner = origOwner
		repo = origRepo
	})
}

func TestOwner(t *testing.T) {
	setOwnerRepo(t, "myowner", "myrepo")
	if got := Owner(); got != "myowner" {
		t.Errorf("Owner() = %q, want %q", got, "myowner")
	}
}

func TestRepo(t *testing.T) {
	setOwnerRepo(t, "myowner", "myrepo")
	if got := Repo(); got != "myrepo" {
		t.Errorf("Repo() = %q, want %q", got, "myrepo")
	}
}

func TestOwnerRepo_Empty(t *testing.T) {
	setOwnerRepo(t, "", "")
	if got := Owner(); got != "" {
		t.Errorf("Owner() = %q, want empty", got)
	}
	if got := Repo(); got != "" {
		t.Errorf("Repo() = %q, want empty", got)
	}
}

func TestRawURLEmptyWhenNotConfigured(t *testing.T) {
	setOwnerRepo(t, "", "")

	result := RawURL("some/path.json")
	if result != "" {
		t.Errorf("RawURL with empty owner/repo = %q, want empty", result)
	}
}

func TestRawURLFormat(t *testing.T) {
	setOwnerRepo(t, "testowner", "testrepo")

	got := RawURL("data/tiers.json")
	want := "https://raw.githubusercontent.com/testowner/testrepo/main/data/tiers.json"
	if got != want {
		t.Errorf("RawURL = %q, want %q", got, want)
	}
}

func TestRawURLOwnerOnly(t *testing.T) {
	setOwnerRepo(t, "testowner", "")

	got := RawURL("file.txt")
	if got != "" {
		t.Errorf("RawURL with repo empty = %q, want empty", got)
	}
}

func TestRawURLRepoOnly(t *testing.T) {
	setOwnerRepo(t, "", "testrepo")

	got := RawURL("file.txt")
	if got != "" {
		t.Errorf("RawURL with owner empty = %q, want empty", got)
	}
}

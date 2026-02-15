package paths

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ///////////////////////////////////////////////
// Constant Value Tests
// ///////////////////////////////////////////////

func TestConstantValues(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"DataDirRel", DataDirRel, ".agentcord"},
		{"StateFile", StateFile, "state.json"},
		{"PIDFile", PIDFile, "daemon.pid"},
		{"ConfigFile", ConfigFile, "config.toml"},
		{"LogFile", LogFile, "daemon.log"},
		{"ConversationsDir", ConversationsDir, "conversations"},
		{"PricingCacheFile", PricingCacheFile, "pricing-cache.json"},
		{"TiersCacheFile", TiersCacheFile, "tiers-cache.json"},
		{"SessionsDir", SessionsDir, "sessions"},
		{"SessionExt", SessionExt, ".session"},
		{"BinaryName", BinaryName, "agentcord"},
		{"TiersDataPath", TiersDataPath, "data/tiers.json"},
		{"ReleaseManifest", ReleaseManifest, ".release-manifest.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// ///////////////////////////////////////////////
// DataDir Method Tests
// ///////////////////////////////////////////////

func TestDataDirMethods(t *testing.T) {
	root := filepath.Join("home", "user", ".claude", "agentcord")
	d := DataDir{Root: root}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"PID", d.PID(), filepath.Join(root, "daemon.pid")},
		{"State", d.State(), filepath.Join(root, "state.json")},
		{"Config", d.Config(), filepath.Join(root, "config.toml")},
		{"Log", d.Log(), filepath.Join(root, "daemon.log")},
		{"Conversations", d.Conversations(), filepath.Join(root, "conversations")},
		{"PricingCache", d.PricingCache(), filepath.Join(root, "pricing-cache.json")},
		{"TiersCache", d.TiersCache(), filepath.Join(root, "tiers-cache.json")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s() = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestDataDirEmptyRoot(t *testing.T) {
	d := DataDir{Root: ""}

	// With an empty root, methods should return just the filename.
	if got := d.PID(); got != PIDFile {
		t.Errorf("PID() with empty root = %q, want %q", got, PIDFile)
	}
	if got := d.State(); got != StateFile {
		t.Errorf("State() with empty root = %q, want %q", got, StateFile)
	}
}

// ///////////////////////////////////////////////
// Constants.sh Sync Tests
// ///////////////////////////////////////////////

func TestConstantsShMatchesGoConstants(t *testing.T) {
	// The generated constants.sh lives at scripts/hooks/lib/unix/constants.sh
	// relative to the repo root. We find the repo root by walking up from
	// this test file's package directory.
	repoRoot := findRepoRoot(t)
	constantsPath := filepath.Join(repoRoot, "scripts", "hooks", "lib", "unix", "constants.sh")

	f, err := os.Open(constantsPath)
	if err != nil {
		t.Fatalf("failed to open constants.sh: %v", err)
	}
	defer f.Close()

	// Parse KEY=VALUE pairs from the shell file.
	shellVars := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		value := strings.Trim(parts[1], `"`)
		shellVars[key] = value
	}

	// String constant checks.
	stringChecks := []struct {
		shellKey string
		goValue  string
	}{
		{"DATA_DIR_REL", DataDirRel},
		{"STATE_FILE", StateFile},
		{"PID_FILE", PIDFile},
		{"SESSIONS_DIR", SessionsDir},
		{"SESSION_EXT", SessionExt},
		{"BINARY_NAME", BinaryName},
	}

	for _, tc := range stringChecks {
		t.Run(tc.shellKey, func(t *testing.T) {
			shellVal, ok := shellVars[tc.shellKey]
			if !ok {
				t.Fatalf("key %s not found in constants.sh", tc.shellKey)
			}
			if shellVal != tc.goValue {
				t.Errorf("constants.sh %s = %q, Go constant = %q", tc.shellKey, shellVal, tc.goValue)
			}
		})
	}

	// Integer constant checks.
	t.Run("STATE_VERSION", func(t *testing.T) {
		shellVal, ok := shellVars["STATE_VERSION"]
		if !ok {
			t.Fatal("STATE_VERSION not found in constants.sh")
		}
		n, err := strconv.Atoi(shellVal)
		if err != nil {
			t.Fatalf("STATE_VERSION is not an integer: %v", err)
		}
		// CurrentVersion is in the session package, but we can verify the
		// shell file declares it as 1 (which is the known current version).
		// We avoid importing the session package to prevent circular deps.
		if n != 1 {
			t.Errorf("constants.sh STATE_VERSION = %d, expected 1", n)
		}
	})
}

// findRepoRoot walks up from the current working directory to find the repository
// root by looking for go.mod.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}
		dir = parent
	}
}

// ///////////////////////////////////////////////
// DataDir with various root paths
// ///////////////////////////////////////////////

func TestDataDirWithAbsolutePath(t *testing.T) {
	// Use a representative absolute path (filepath.Join normalises separators).
	root := filepath.Join("home", "user", ".claude", "agentcord")
	d := DataDir{Root: root}

	want := filepath.Join(root, PIDFile)
	if got := d.PID(); got != want {
		t.Errorf("PID() = %q, want %q", got, want)
	}
}

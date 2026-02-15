// Tests for the config package covering [Load] behavior (defaults, overrides,
// missing files, malformed input, migration), field formatting methods
// ([Config.FormatDetails], [Config.FormatState], [Config.FormatModelName],
// [Config.FormatBranch]), privacy controls ([Config.IsIgnored],
// [Config.ProjectName]), validation ([Config.Validate]), serialization
// round-trips ([Config.Save]), and [ConfigDocs] completeness.

package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// ///////////////////////////////////////////////
// Load
// ///////////////////////////////////////////////

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		config  string // config file content; empty means no file written
		noFile  bool   // if true, skip writing a config file
		wantErr bool
		check   func(t *testing.T, cfg *Config)
	}{
		{
			name:   "defaults from minimal config",
			config: "version = 1\n",
			check: func(t *testing.T, cfg *Config) {
				t.Helper()
				def := DefaultConfig()
				if cfg.Display.Details != def.Display.Details {
					t.Errorf("Details = %q, want %q", cfg.Display.Details, def.Display.Details)
				}
				if cfg.Behavior.PresenceIdleMinutes != def.Behavior.PresenceIdleMinutes {
					t.Errorf("PresenceIdleMinutes = %d, want %d",
						cfg.Behavior.PresenceIdleMinutes, def.Behavior.PresenceIdleMinutes)
				}
			},
		},
		{
			name: "user overrides applied",
			config: `
version = 1

[discord]
app_id = "custom-app-id"

[behavior]
presence_idle_minutes = 10
daemon_idle_minutes = 60
`,
			check: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg.Discord.AppID != "custom-app-id" {
					t.Errorf("AppID = %q, want %q", cfg.Discord.AppID, "custom-app-id")
				}
				if cfg.Behavior.PresenceIdleMinutes != 10 {
					t.Errorf("PresenceIdleMinutes = %d, want 10", cfg.Behavior.PresenceIdleMinutes)
				}
				if cfg.Behavior.DaemonIdleMinutes != 60 {
					t.Errorf("DaemonIdleMinutes = %d, want 60", cfg.Behavior.DaemonIdleMinutes)
				}
			},
		},
		{
			name: "partial override preserves other defaults",
			config: `
version = 1

[display]
details = "Custom: {project}"
`,
			check: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg.Display.Details != "Custom: {project}" {
					t.Errorf("Details = %q, want %q", cfg.Display.Details, "Custom: {project}")
				}
				def := DefaultConfig()
				if cfg.Display.State != def.Display.State {
					t.Errorf("State = %q, want default %q", cfg.Display.State, def.Display.State)
				}
				if cfg.Discord.AppID != def.Discord.AppID {
					t.Errorf("AppID = %q, want default %q", cfg.Discord.AppID, def.Discord.AppID)
				}
			},
		},
		{
			name:   "missing file returns defaults",
			noFile: true,
			check: func(t *testing.T, cfg *Config) {
				t.Helper()
				def := DefaultConfig()
				if cfg.Version != def.Version {
					t.Errorf("Version = %d, want %d", cfg.Version, def.Version)
				}
			},
		},
		{
			name:    "malformed TOML returns error",
			config:  "this is not valid toml [[[",
			wantErr: true,
		},
		{
			name: "custom app_id",
			config: `
version = 1

[discord]
app_id = "9999999999"
`,
			check: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg.Discord.AppID != "9999999999" {
					t.Errorf("AppID = %q, want %q", cfg.Discord.AppID, "9999999999")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if !tt.noFile {
				writeConfig(t, dir, tt.config)
			}

			cfg, err := Load(dir)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
				return
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

// ///////////////////////////////////////////////
// FormatDetails
// ///////////////////////////////////////////////

func TestConfig_FormatDetails(t *testing.T) {
	tests := []struct {
		name    string
		project string
		branch  string
		wantIn  []string // substrings that must appear
		wantOut []string // substrings that must NOT appear
	}{
		{
			name:    "with branch",
			project: "myproject",
			branch:  "main",
			wantIn:  []string{"myproject", "main"},
		},
		{
			name:    "no branch uses fallback template",
			project: "myproject",
			branch:  "",
			wantIn:  []string{"myproject"},
			wantOut: []string{"{branch}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			result := cfg.FormatDetails(tt.project, tt.branch)
			for _, s := range tt.wantIn {
				if !strings.Contains(result, s) {
					t.Errorf("result %q missing expected substring %q", result, s)
				}
			}
			for _, s := range tt.wantOut {
				if strings.Contains(result, s) {
					t.Errorf("result %q should not contain %q", result, s)
				}
			}
		})
	}
}

// ///////////////////////////////////////////////
// FormatState
// ///////////////////////////////////////////////

func TestConfig_FormatState(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		cost    float64
		tokens  int64
		hasCost bool
		wantIn  []string
		wantOut []string
	}{
		{
			name:    "with cost",
			model:   "Opus 4.6",
			cost:    1.50,
			tokens:  15000,
			hasCost: true,
			wantIn:  []string{"Opus 4.6", "1.50"},
		},
		{
			name:    "without cost uses fallback template",
			model:   "Opus 4.6",
			cost:    0,
			tokens:  15000,
			hasCost: false,
			wantIn:  []string{"Opus 4.6"},
			wantOut: []string{"$"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			result := cfg.FormatState(tt.model, tt.cost, tt.tokens, tt.hasCost)
			for _, s := range tt.wantIn {
				if !strings.Contains(result, s) {
					t.Errorf("result %q missing expected substring %q", result, s)
				}
			}
			for _, s := range tt.wantOut {
				if strings.Contains(result, s) {
					t.Errorf("result %q should not contain %q", result, s)
				}
			}
		})
	}
}

// ///////////////////////////////////////////////
// FormatModelName
// ///////////////////////////////////////////////

func TestConfig_FormatModelName(t *testing.T) {
	tests := []struct {
		name   string
		format string
		input  string
		want   string
	}{
		{
			name:   "short format",
			format: "short",
			input:  "claude-opus-4-6",
			want:   "Opus 4.6",
		},
		{
			name:   "full format",
			format: "full",
			input:  "claude-opus-4-6",
			want:   "Claude Opus 4.6",
		},
		{
			name:   "raw format",
			format: "raw",
			input:  "claude-opus-4-6",
			want:   "claude-opus-4-6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Display.Format.ModelName = tt.format
			got := cfg.FormatModelName(tt.input)
			if got != tt.want {
				t.Errorf("FormatModelName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ///////////////////////////////////////////////
// IsIgnored
// ///////////////////////////////////////////////

func TestConfig_IsIgnored(t *testing.T) {
	tests := []struct {
		name   string
		ignore []string
		cwd    string
		want   bool
	}{
		{
			name:   "exact match",
			ignore: []string{"/home/user/secret"},
			cwd:    "/home/user/secret",
			want:   true,
		},
		{
			name:   "glob pattern match",
			ignore: []string{"/home/user/work/*"},
			cwd:    "/home/user/work/project1",
			want:   true,
		},
		{
			name:   "no match",
			ignore: []string{"/home/user/secret"},
			cwd:    "/home/user/public",
			want:   false,
		},
		{
			name:   "empty list",
			ignore: nil,
			cwd:    "/anything",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Privacy.Ignore = tt.ignore
			got := cfg.IsIgnored(tt.cwd)
			if got != tt.want {
				t.Errorf("IsIgnored(%q) = %v, want %v", tt.cwd, got, tt.want)
			}
		})
	}
}

// ///////////////////////////////////////////////
// ProjectName
// ///////////////////////////////////////////////

func TestConfig_ProjectName(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(cfg *Config)
		realName string
		cwd      string
		want     string
	}{
		{
			name: "hidden globally",
			setup: func(cfg *Config) {
				cfg.Privacy.HideProjectName = true
				cfg.Privacy.HiddenProjectText = "a project"
			},
			realName: "real-project",
			cwd:      "/tmp/real-project",
			want:     "a project",
		},
		{
			name: "visible",
			setup: func(cfg *Config) {
				cfg.Privacy.HideProjectName = false
			},
			realName: "real-project",
			cwd:      "/tmp/real-project",
			want:     "real-project",
		},
		{
			name: "override matching pattern",
			setup: func(cfg *Config) {
				cfg.Privacy.Overrides = []PrivacyOverride{
					{Pattern: "/tmp/work-*", HideProjectName: true, HiddenText: "a work project"},
				}
			},
			realName: "work-secret",
			cwd:      "/tmp/work-secret",
			want:     "a work project",
		},
		{
			name: "override non-matching path",
			setup: func(cfg *Config) {
				cfg.Privacy.Overrides = []PrivacyOverride{
					{Pattern: "/tmp/work-*", HideProjectName: true, HiddenText: "a work project"},
				}
			},
			realName: "personal",
			cwd:      "/tmp/personal",
			want:     "personal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.setup(cfg)
			got := cfg.ProjectName(tt.realName, tt.cwd)
			if got != tt.want {
				t.Errorf("ProjectName(%q, %q) = %q, want %q", tt.realName, tt.cwd, got, tt.want)
			}
		})
	}
}

// ///////////////////////////////////////////////
// FormatBranch
// ///////////////////////////////////////////////

func TestConfig_FormatBranch(t *testing.T) {
	tests := []struct {
		name            string
		branchMode      string
		defaultBranches []string
		input           string
		want            string
	}{
		{
			name:       "show mode returns as-is",
			branchMode: "show",
			input:      "main",
			want:       "main",
		},
		{
			name:            "hide_default hides main",
			branchMode:      "hide_default",
			defaultBranches: []string{"main", "master"},
			input:           "main",
			want:            "",
		},
		{
			name:            "hide_default hides master",
			branchMode:      "hide_default",
			defaultBranches: []string{"main", "master"},
			input:           "master",
			want:            "",
		},
		{
			name:            "hide_default shows feature branch",
			branchMode:      "hide_default",
			defaultBranches: []string{"main", "master"},
			input:           "feature/xyz",
			want:            "feature/xyz",
		},
		{
			name:       "hide mode returns empty string",
			branchMode: "hide",
			input:      "feature/abc",
			want:       "",
		},
		{
			name:       "hide mode returns empty for main",
			branchMode: "hide",
			input:      "main",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Display.Format.Branch = tt.branchMode
			if tt.defaultBranches != nil {
				cfg.Display.Format.DefaultBranches = tt.defaultBranches
			}
			got := cfg.FormatBranch(tt.input)
			if got != tt.want {
				t.Errorf("FormatBranch(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ///////////////////////////////////////////////
// Buttons
// ///////////////////////////////////////////////

func TestButtonConfig(t *testing.T) {
	tests := []struct {
		name   string
		config string
		check  func(t *testing.T, cfg *Config)
	}{
		{
			name:   "repo button enabled by default",
			config: "version = 1\n",
			check: func(t *testing.T, cfg *Config) {
				t.Helper()
				if !cfg.Display.Buttons.ShowRepoButton {
					t.Error("expected show_repo_button true by default")
				}
				if cfg.Display.Buttons.RepoButtonLabel != "View Repository" {
					t.Errorf("RepoButtonLabel = %q, want %q", cfg.Display.Buttons.RepoButtonLabel, "View Repository")
				}
			},
		},
		{
			name: "custom button",
			config: `
version = 1

[display.buttons]
custom_button_label = "My Site"
custom_button_url = "https://example.com"
`,
			check: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg.Display.Buttons.CustomButtonLabel != "My Site" {
					t.Errorf("CustomButtonLabel = %q, want %q", cfg.Display.Buttons.CustomButtonLabel, "My Site")
				}
				if cfg.Display.Buttons.CustomButtonURL != "https://example.com" {
					t.Errorf("CustomButtonURL = %q, want %q", cfg.Display.Buttons.CustomButtonURL, "https://example.com")
				}
			},
		},
		{
			name: "both buttons",
			config: `
version = 1

[display.buttons]
show_repo_button = true
repo_button_label = "Repo"
custom_button_label = "Blog"
custom_button_url = "https://blog.example.com"
`,
			check: func(t *testing.T, cfg *Config) {
				t.Helper()
				if !cfg.Display.Buttons.ShowRepoButton {
					t.Error("expected repo button enabled")
				}
				if cfg.Display.Buttons.CustomButtonLabel != "Blog" {
					t.Errorf("CustomButtonLabel = %q, want %q", cfg.Display.Buttons.CustomButtonLabel, "Blog")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, tt.config)

			cfg, err := Load(dir)
			if err != nil {
				t.Fatalf("Load: %v", err)
				return
			}
			tt.check(t, cfg)
		})
	}
}

// ///////////////////////////////////////////////
// Migration integration
// ///////////////////////////////////////////////

func TestLoad_Migration(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		wantVersion int
	}{
		{
			name: "migrates old version",
			config: `
[discord]
app_id = "test"
`, // version 0 (missing) -- should be normalized to 1
			wantVersion: 1,
		},
		{
			name:        "skips migration when current",
			config:      "version = 1",
			wantVersion: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, tt.config)

			cfg, err := Load(dir)
			if err != nil {
				t.Fatalf("Load: %v", err)
				return
			}
			if cfg.Version != tt.wantVersion {
				t.Errorf("Version = %d, want %d", cfg.Version, tt.wantVersion)
			}
		})
	}
}

// ///////////////////////////////////////////////
// PeekVersion
// ///////////////////////////////////////////////

func TestPeekVersion(t *testing.T) {
	tests := []struct {
		name string
		data string
		want int
	}{
		{
			name: "reads version from TOML",
			data: "version = 3\n[discord]\napp_id = \"test\"\n",
			want: 3,
		},
		{
			name: "missing version returns 1",
			data: "[discord]\napp_id = \"test\"\n",
			want: 1, // normalized from 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PeekVersion([]byte(tt.data))
			if got != tt.want {
				t.Errorf("PeekVersion() = %d, want %d", got, tt.want)
			}
		})
	}
}

// ///////////////////////////////////////////////
// ExampleConfig
// ///////////////////////////////////////////////

func TestExampleConfig(t *testing.T) {
	cfg := ExampleConfig()
	if cfg == nil {
		t.Fatal("ExampleConfig returned nil")
		return
	}
	if cfg.Version != 1 {
		t.Errorf("Version = %d, want 1", cfg.Version)
	}
	if cfg.Discord.AppID == "" {
		t.Error("expected non-empty app_id")
	}
	// Verify it can be marshaled
	var buf strings.Builder
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(cfg); err != nil {
		t.Fatalf("failed to marshal ExampleConfig: %v", err)
	}
}

// ///////////////////////////////////////////////
// ConfigDocs completeness
// ///////////////////////////////////////////////

func TestConfigDocsComplete(t *testing.T) {
	fields := collectTOMLFields(reflect.TypeOf(Config{}), "")
	for _, field := range fields {
		if _, ok := ConfigDocs[field]; !ok {
			t.Errorf("ConfigDocs missing entry for field %q", field)
		}
	}
}

// collectTOMLFields recursively walks a struct type and returns the
// dot-separated TOML key path for every tagged field. Used by
// TestConfigDocsComplete to verify that [ConfigDocs] covers all fields.
func collectTOMLFields(typ reflect.Type, prefix string) []string {
	var fields []string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		tag := f.Tag.Get("toml")
		if tag == "" || tag == "-" {
			continue
		}
		// Strip options like ",omitempty"
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}
		path := tag
		if prefix != "" {
			path = prefix + "." + tag
		}
		if f.Type.Kind() == reflect.Struct {
			fields = append(fields, collectTOMLFields(f.Type, path)...)
		} else {
			fields = append(fields, path)
		}
	}
	return fields
}

// ///////////////////////////////////////////////
// Marshal field order
// ///////////////////////////////////////////////

func TestConfigMarshalFieldOrder(t *testing.T) {
	cfg := DefaultConfig()
	var buf strings.Builder
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(cfg); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := buf.String()

	tests := []struct {
		name   string
		before string
		after  string
	}{
		{
			name:   "version before [discord]",
			before: "version",
			after:  "[discord]",
		},
		{
			name:   "[discord] before [display]",
			before: "[discord]",
			after:  "[display]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bIdx := strings.Index(out, tt.before)
			aIdx := strings.Index(out, tt.after)
			if bIdx < 0 || aIdx < 0 || bIdx > aIdx {
				t.Errorf("expected %q before %q in marshaled output", tt.before, tt.after)
			}
		})
	}
}

// ///////////////////////////////////////////////
// FormatShort
// ///////////////////////////////////////////////

func TestFormatShort(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want string
	}{
		{name: "zero", in: 0, want: "0"},
		{name: "hundreds", in: 500, want: "500"},
		{name: "just under 1K", in: 999, want: "999"},
		{name: "exact 1K", in: 1000, want: "1K"},
		{name: "1.5K", in: 1500, want: "1.5K"},
		{name: "exact 2K", in: 2000, want: "2K"},
		{name: "12.3K", in: 12345, want: "12.3K"},
		{name: "near 1M in K", in: 999999, want: "1000.0K"},
		{name: "exact 1M", in: 1000000, want: "1M"},
		{name: "1.5M", in: 1500000, want: "1.5M"},
		{name: "exact 2M", in: 2000000, want: "2M"},
		{name: "15.6M", in: 15600000, want: "15.6M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatShort(tt.in)
			if got != tt.want {
				t.Errorf("FormatShort(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// ///////////////////////////////////////////////
// FormatWithCommas
// ///////////////////////////////////////////////

func TestFormatWithCommas(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want string
	}{
		{name: "negative single digit", in: -1, want: "-1"},
		{name: "negative hundreds", in: -100, want: "-100"},
		{name: "negative thousands", in: -1000, want: "-1,000"},
		{name: "negative millions", in: -1500000, want: "-1,500,000"},
		{name: "positive millions", in: 1500000, want: "1,500,000"},
		{name: "zero", in: 0, want: "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatWithCommas(tt.in)
			if got != tt.want {
				t.Errorf("FormatWithCommas(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// ///////////////////////////////////////////////
// Config.Save round-trip
// ///////////////////////////////////////////////

func TestConfig_Save_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	orig := DefaultConfig()
	orig.Discord.AppID = "round-trip-test"
	orig.Behavior.PollIntervalSeconds = 10
	orig.Display.Format.CostFormat = "$%.4f"

	if err := orig.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
		return
	}

	loaded := DefaultConfig()
	if err := toml.Unmarshal(data, loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
		return
	}

	if loaded.Discord.AppID != orig.Discord.AppID {
		t.Errorf("AppID = %q, want %q", loaded.Discord.AppID, orig.Discord.AppID)
	}
	if loaded.Behavior.PollIntervalSeconds != orig.Behavior.PollIntervalSeconds {
		t.Errorf("PollIntervalSeconds = %d, want %d",
			loaded.Behavior.PollIntervalSeconds, orig.Behavior.PollIntervalSeconds)
	}
	if loaded.Display.Format.CostFormat != orig.Display.Format.CostFormat {
		t.Errorf("CostFormat = %q, want %q",
			loaded.Display.Format.CostFormat, orig.Display.Format.CostFormat)
	}
}

// ///////////////////////////////////////////////
// Validate
// ///////////////////////////////////////////////

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(cfg *Config)
		wantErr bool
	}{
		{
			name:    "default config passes",
			setup:   func(cfg *Config) {},
			wantErr: false,
		},
		{
			name:    "invalid idle_mode",
			setup:   func(cfg *Config) { cfg.Behavior.IdleMode = "bogus" },
			wantErr: true,
		},
		{
			name:    "invalid timestamps.mode",
			setup:   func(cfg *Config) { cfg.Display.Timestamps.Mode = "invalid" },
			wantErr: true,
		},
		{
			name:    "invalid log.level",
			setup:   func(cfg *Config) { cfg.Log.Level = "verbose" },
			wantErr: true,
		},
		{
			name:    "poll_interval_seconds = 0",
			setup:   func(cfg *Config) { cfg.Behavior.PollIntervalSeconds = 0 },
			wantErr: true,
		},
		{
			name:    "negative reconnect_interval_seconds",
			setup:   func(cfg *Config) { cfg.Behavior.ReconnectIntervalSeconds = -1 },
			wantErr: true,
		},
		{
			name:    "negative daemon_idle_minutes",
			setup:   func(cfg *Config) { cfg.Behavior.DaemonIdleMinutes = -5 },
			wantErr: true,
		},
		{
			name:    "invalid pricing.source",
			setup:   func(cfg *Config) { cfg.Pricing.Source = "network" },
			wantErr: true,
		},
		{
			name:    "invalid branch mode",
			setup:   func(cfg *Config) { cfg.Display.Format.Branch = "short" },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.setup(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_Validate_EnumPositive(t *testing.T) {
	tests := []struct {
		name  string
		setup func(cfg *Config)
	}{
		// idle_mode
		{name: "idle_mode clear", setup: func(cfg *Config) { cfg.Behavior.IdleMode = "clear" }},
		{name: "idle_mode idle_text", setup: func(cfg *Config) { cfg.Behavior.IdleMode = "idle_text" }},
		{name: "idle_mode last_activity", setup: func(cfg *Config) { cfg.Behavior.IdleMode = "last_activity" }},
		// timestamps.mode
		{name: "timestamps.mode session", setup: func(cfg *Config) { cfg.Display.Timestamps.Mode = "session" }},
		{name: "timestamps.mode elapsed", setup: func(cfg *Config) { cfg.Display.Timestamps.Mode = "elapsed" }},
		{name: "timestamps.mode none", setup: func(cfg *Config) { cfg.Display.Timestamps.Mode = "none" }},
		// format.branch
		{name: "format.branch show", setup: func(cfg *Config) { cfg.Display.Format.Branch = "show" }},
		{name: "format.branch hide", setup: func(cfg *Config) { cfg.Display.Format.Branch = "hide" }},
		{name: "format.branch hide_default", setup: func(cfg *Config) { cfg.Display.Format.Branch = "hide_default" }},
		// pricing.source
		{name: "pricing.source url", setup: func(cfg *Config) { cfg.Pricing.Source = "url" }},
		{name: "pricing.source file", setup: func(cfg *Config) { cfg.Pricing.Source = "file" }},
		{name: "pricing.source static", setup: func(cfg *Config) { cfg.Pricing.Source = "static" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.setup(cfg)
			if err := cfg.Validate(); err != nil {
				t.Errorf("Validate() returned error for valid enum: %v", err)
			}
		})
	}
}

func TestConfig_Validate_CostFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{name: "invalid format returns error", format: "no-format-verb", wantErr: true},
		{name: "empty string returns error", format: "", wantErr: true},
		{name: "%s string verb returns error", format: "%s", wantErr: true},
		{name: "invalid bare word returns error", format: "invalid", wantErr: true},
		{name: "%.2f", format: "%.2f"},
		{name: "$%.4f", format: "$%.4f"},
		{name: "Cost: %.1f USD", format: "Cost: %.1f USD"},
		{name: "%e", format: "%e"},
		{name: "%G", format: "%G"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Display.Format.CostFormat = tt.format
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ///////////////////////////////////////////////
// Helpers
// ///////////////////////////////////////////////

// writeConfig writes a TOML config string to config.toml in dir for use
// by [Load] in test cases.
func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}
}

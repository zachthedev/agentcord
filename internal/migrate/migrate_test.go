// Package migrate tests verify sequential migration application, version
// skipping, error propagation, [NeedsMigration] detection, and the
// [Registry] dev-transform path.
package migrate

import (
	"fmt"
	"strings"
	"testing"
)

// ///////////////////////////////////////////////
// Run (package-level)
// ///////////////////////////////////////////////

func TestRunSkipsOldVersions(t *testing.T) {
	called := false
	migrations := []Migration{
		{Version: 1, Description: "already applied", Upgrade: func(d []byte) ([]byte, error) {
			called = true
			return d, nil
		}},
	}
	out, version, err := Run([]byte("data"), 1, migrations)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("migration should have been skipped")
	}
	if version != 1 {
		t.Fatalf("expected version 1, got %d", version)
	}
	if string(out) != "data" {
		t.Fatalf("expected data unchanged, got %q", out)
	}
}

func TestRunAppliesSequentially(t *testing.T) {
	migrations := []Migration{
		{Version: 2, Description: "v1->v2", Upgrade: func(d []byte) ([]byte, error) {
			return append(d, []byte("-v2")...), nil
		}},
		{Version: 3, Description: "v2->v3", Upgrade: func(d []byte) ([]byte, error) {
			return append(d, []byte("-v3")...), nil
		}},
	}
	out, version, err := Run([]byte("data"), 1, migrations)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != 3 {
		t.Fatalf("expected version 3, got %d", version)
	}
	if string(out) != "data-v2-v3" {
		t.Fatalf("expected data-v2-v3, got %q", out)
	}
}

func TestRunStopsOnError(t *testing.T) {
	migrations := []Migration{
		{Version: 2, Description: "v1->v2", Upgrade: func(d []byte) ([]byte, error) {
			return append(d, []byte("-v2")...), nil
		}},
		{Version: 3, Description: "v2->v3 fails", Upgrade: func(d []byte) ([]byte, error) {
			return nil, fmt.Errorf("boom")
		}},
	}
	_, version, err := Run([]byte("data"), 1, migrations)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "migration to v3 failed") {
		t.Fatalf("expected migration error message, got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected wrapped error, got %v", err)
	}
	if version != 2 {
		t.Fatalf("expected version 2 (stopped before v3), got %d", version)
	}
}

func TestRunNoMigrations(t *testing.T) {
	out, version, err := Run([]byte("original"), 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected version 1, got %d", version)
	}
	if string(out) != "original" {
		t.Fatalf("expected original, got %q", out)
	}
}

// ///////////////////////////////////////////////
// NeedsMigration (package-level)
// ///////////////////////////////////////////////

func TestNeedsMigrationVersionMismatch(t *testing.T) {
	if !NeedsMigration(0, 1, false, nil) {
		t.Fatal("expected true for version 0 vs current 1")
	}
	if !NeedsMigration(2, 1, false, nil) {
		t.Fatal("expected true for version 2 vs current 1")
	}
}

func TestNeedsMigrationForce(t *testing.T) {
	migs := []Migration{{Version: 2, Description: "test"}}
	if !NeedsMigration(1, 1, true, migs) {
		t.Fatal("expected true with force and migrations present")
	}
	// force but no migrations
	if NeedsMigration(1, 1, true, nil) {
		t.Fatal("expected false with force but no migrations")
	}
}

func TestNeedsMigrationUpToDate(t *testing.T) {
	if NeedsMigration(1, 1, false, nil) {
		t.Fatal("expected false when up to date")
	}
	if NeedsMigration(1, 1, false, []Migration{}) {
		t.Fatal("expected false when up to date with empty migrations")
	}
}

// ///////////////////////////////////////////////
// Registry
// ///////////////////////////////////////////////

func TestRunDevTransforms(t *testing.T) {
	r := &Registry{
		CurrentVersion: 1,
		Dev: []Migration{
			{Description: "add suffix", Upgrade: func(d []byte) ([]byte, error) {
				return append(d, []byte("-dev")...), nil
			}},
		},
	}
	out, err := r.RunDev([]byte("data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "data-dev" {
		t.Fatalf("expected data-dev, got %q", out)
	}
}

func TestRunDevNoTransforms(t *testing.T) {
	r := &Registry{CurrentVersion: 1}
	out, err := r.RunDev([]byte("data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "data" {
		t.Fatalf("expected data unchanged, got %q", out)
	}
}

func TestRegistryExportedForOverride(t *testing.T) {
	// Verify Config and State registries exist with expected defaults
	if Config.CurrentVersion != 1 {
		t.Fatalf("expected Config.CurrentVersion=1, got %d", Config.CurrentVersion)
	}
	if State.CurrentVersion != 1 {
		t.Fatalf("expected State.CurrentVersion=1, got %d", State.CurrentVersion)
	}

	// Verify Migrations slice is exported and overridable
	orig := Config.Migrations
	Config.Migrations = []Migration{{Version: 99, Description: "test override"}}
	if len(Config.Migrations) != 1 || Config.Migrations[0].Version != 99 {
		t.Fatal("expected override to work")
	}
	Config.Migrations = orig
}

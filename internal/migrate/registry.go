package migrate

import "fmt"

// Registry holds the version and migrations for a single schema target
// (e.g. config TOML, state JSON). Each target gets its own instance so
// that version numbers and migration lists are fully independent.
type Registry struct {
	// CurrentVersion is the latest schema version that this registry targets.
	CurrentVersion int
	// Migrations is the ordered list of versioned upgrades. Exported so
	// tests can override the migration list for a given registry instance.
	Migrations []Migration
	// Dev holds development-only transforms that are applied without
	// advancing the schema version. See [Registry.RunDev].
	Dev []Migration
}

// Register appends a migration to the registry. It panics if a migration
// with the same version is already registered, preventing silent conflicts.
func (r *Registry) Register(m Migration) {
	for _, existing := range r.Migrations {
		if existing.Version == m.Version {
			panic(fmt.Sprintf("migrate: duplicate migration version %d (description: %q)", m.Version, m.Description))
		}
	}
	r.Migrations = append(r.Migrations, m)
}

// RegisterDev appends a dev transform to the registry. It panics if a dev
// transform with the same description is already registered.
func (r *Registry) RegisterDev(m Migration) {
	for _, existing := range r.Dev {
		if existing.Description == m.Description {
			panic(fmt.Sprintf("migrate: duplicate dev transform %q", m.Description))
		}
	}
	r.Dev = append(r.Dev, m)
}

// NeedsMigration reports whether a file at fileVersion would have any
// migrations applied given the registry's current version and registered
// migrations.
func (r *Registry) NeedsMigration(fileVersion int, force bool) bool {
	return NeedsMigration(fileVersion, r.CurrentVersion, force, r.Migrations)
}

// Run applies registered migrations sequentially where fromVersion < m.Version.
func (r *Registry) Run(data []byte, fromVersion int) ([]byte, int, error) {
	return Run(data, fromVersion, r.Migrations)
}

// RunDev applies dev transforms sequentially. No version tracking — the
// file version is left unchanged. Use for one-off local data fixes
// during development.
func (r *Registry) RunDev(data []byte) ([]byte, error) {
	for _, m := range r.Dev {
		var err error
		data, err = m.Upgrade(data)
		if err != nil {
			return nil, fmt.Errorf("dev transform %q: %w", m.Description, err)
		}
	}
	return data, nil
}

// HasDev reports whether any dev transforms are registered.
func (r *Registry) HasDev() bool {
	return len(r.Dev) > 0
}

// Config is the migration registry for config.toml files.
var Config = &Registry{CurrentVersion: 1}

// State is the migration registry for state.json files.
var State = &Registry{CurrentVersion: 1}

// Package migrate applies sequential schema migrations to on-disk data,
// upgrading from one version to the next.
package migrate

import (
	"fmt"
	"log/slog"
	"sort"
)

// ///////////////////////////////////////////////
// Types
// ///////////////////////////////////////////////

// Migration represents a schema migration that upgrades on-disk data
// from one version to the next.
type Migration struct {
	// Version is the schema version this migration produces.
	Version int
	// Description is a short human-readable label for log output.
	Description string
	// Upgrade transforms data from the prior version to [Migration.Version].
	Upgrade func(data []byte) ([]byte, error)
}

// ///////////////////////////////////////////////
// Public API
// ///////////////////////////////////////////////

// Run applies migrations sequentially where fromVersion < m.Version.
// Returns the transformed data, final version reached, and any error.
func Run(data []byte, fromVersion int, migrations []Migration) ([]byte, int, error) {
	sorted := make([]Migration, len(migrations))
	copy(sorted, migrations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Version < sorted[j].Version
	})
	version := fromVersion
	for _, m := range sorted {
		if version < m.Version {
			slog.Info("applying migration", "version", m.Version, "description", m.Description)
			var err error
			data, err = m.Upgrade(data)
			if err != nil {
				return nil, version, fmt.Errorf("migration to v%d failed: %w", m.Version, err)
			}
			version = m.Version
		}
	}
	return data, version, nil
}

// NeedsMigration reports whether a file at fileVersion would have any
// migrations applied given the currentVersion and registered migrations.
func NeedsMigration(fileVersion, currentVersion int, force bool, migrations []Migration) bool {
	if fileVersion != currentVersion {
		return true
	}
	if force && len(migrations) > 0 {
		return true
	}
	for _, m := range migrations {
		if fileVersion < m.Version {
			return true
		}
	}
	return false
}

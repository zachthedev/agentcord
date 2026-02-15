// Package paths centralizes file and directory names used across the project.
// All data directory file names are defined here as the single source of truth.
package paths

// Generate constants.sh and constants.ps1 for hook scripts.
//go:generate go run ../../cmd/genhooks

import "path/filepath"

// ///////////////////////////////////////////////
// Constants
// ///////////////////////////////////////////////

// Data directory file names.
const (
	PIDFile          = "daemon.pid"
	StateFile        = "state.json"
	ConfigFile       = "config.toml"
	LogFile          = "daemon.log"
	ConversationsDir = "conversations"
	PricingCacheFile = "pricing-cache.json"
	TiersCacheFile   = "tiers-cache.json"
)

// StateFileForClient returns the per-client state file name.
// For example, StateFileForClient("claude-code") returns "state.claude-code.json".
func StateFileForClient(client string) string {
	return "state." + client + ".json"
}

// Hook script constants — consumed by cmd/genhooks to generate
// _constants.sh and _constants.ps1 for the shell/PowerShell hooks.
const (
	SessionsDir = "sessions"
	SessionExt  = ".session"
	BinaryName  = "agentcord"
	DataDirRel  = ".agentcord" // relative to $HOME
)

// Remote-fetched file paths (relative to repo root).
const (
	TiersDataPath   = "data/tiers.json"
	ReleaseManifest = ".release-manifest.json"
)

// ///////////////////////////////////////////////
// DataDir
// ///////////////////////////////////////////////

// DataDir provides path construction methods rooted at a data directory.
type DataDir struct {
	Root string
}

// PID returns the full path to the PID file.
func (d DataDir) PID() string { return filepath.Join(d.Root, PIDFile) }

// State returns the full path to the state file.
func (d DataDir) State() string { return filepath.Join(d.Root, StateFile) }

// Config returns the full path to the config file.
func (d DataDir) Config() string { return filepath.Join(d.Root, ConfigFile) }

// Log returns the full path to the log file.
func (d DataDir) Log() string { return filepath.Join(d.Root, LogFile) }

// Conversations returns the full path to the conversations directory.
func (d DataDir) Conversations() string { return filepath.Join(d.Root, ConversationsDir) }

// PricingCache returns the full path to the pricing cache file.
func (d DataDir) PricingCache() string { return filepath.Join(d.Root, PricingCacheFile) }

// TiersCache returns the full path to the tiers cache file.
func (d DataDir) TiersCache() string { return filepath.Join(d.Root, TiersCacheFile) }

// Sessions returns the full path to the sessions directory.
func (d DataDir) Sessions() string { return filepath.Join(d.Root, SessionsDir) }

// StateForClient returns the full path to the per-client state file.
func (d DataDir) StateForClient(client string) string {
	return filepath.Join(d.Root, StateFileForClient(client))
}

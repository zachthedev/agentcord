// Package agentcord provides embedded assets for the Agentcord daemon.
//
// The root package exists solely to embed [config.default.toml] via
// [DefaultConfigTOML]. The config package reads this at startup to
// seed first-run defaults.
package agentcord

import _ "embed"

// DefaultConfigTOML holds the raw bytes of config.default.toml, embedded at
// build time. The [internal/config] package copies this file to the data
// directory on first run.
//
//go:embed config.default.toml
var DefaultConfigTOML []byte

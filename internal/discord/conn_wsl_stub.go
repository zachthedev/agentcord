// conn_wsl_stub.go is a no-op stub for platforms where WSL detection is irrelevant.

//go:build !linux && !windows

package discord

func isWSL() bool          { return false }
func wslSocketPaths() []string { return nil }

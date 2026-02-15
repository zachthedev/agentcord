// conn_wsl.go provides WSL-specific Discord IPC socket discovery.
//
// When running inside WSL (Windows Subsystem for Linux), Discord runs on the
// Windows host side. Its IPC socket is a Windows named pipe (\\.\pipe\discord-ipc-N),
// which is not directly accessible from WSL2 as a Unix socket.
//
// WSL1 may expose Windows named pipes transparently, but WSL2 does not.
// For WSL2, users need to set up a relay using socat + npiperelay.exe:
//
//	socat UNIX-LISTEN:/tmp/discord-ipc-0,fork EXEC:"npiperelay.exe -ep -s //./pipe/discord-ipc-0"
//
// This file adds the standard Unix socket paths that such a relay would create,
// so the daemon automatically finds them. If no relay is running, these paths
// simply won't exist and the connection falls through to ErrIPCNotAvailable.

//go:build linux

package discord

import (
	"fmt"
	"os"
	"strings"
)

// isWSL reports whether the current process is running inside WSL.
func isWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(data))
	return strings.Contains(lower, "microsoft")
}

// wslSocketPaths returns additional socket paths to try when running under WSL.
// These cover locations where a socat/npiperelay bridge would typically create
// the Unix socket, as well as WSLg paths used in newer WSL versions.
func wslSocketPaths() []string {
	if !isWSL() {
		return nil
	}

	var paths []string

	// Standard locations where a relay would place the socket.
	for i := range maxIPCSlots {
		paths = append(paths, fmt.Sprintf("/tmp/discord-ipc-%d", i))
	}

	// Some relay setups use /run/user/<uid>/ directly.
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		for i := range maxIPCSlots {
			paths = append(paths, fmt.Sprintf("%s/discord-ipc-%d", dir, i))
		}
	}

	return paths
}

// conn_unix.go implements Discord IPC socket discovery for Unix-like systems
// (Linux, macOS, FreeBSD). It probes XDG_RUNTIME_DIR, /tmp, Snap, and Flatpak
// socket paths.

//go:build !windows

package discord

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

// ///////////////////////////////////////////////
// Connection
// ///////////////////////////////////////////////

// connectToDiscord tries each known IPC socket path and returns the first
// successful connection. It checks XDG_RUNTIME_DIR, /tmp, Snap, and
// Flatpak socket locations.
func connectToDiscord() (net.Conn, error) {
	var paths []string

	// Socket name prefixes for Discord variants (stable, Canary, PTB).
	variants := []string{"discord-ipc", "discordcanary-ipc", "discordptb-ipc"}

	// XDG_RUNTIME_DIR is the preferred runtime directory on most Linux systems.
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		for _, v := range variants {
			for i := range maxIPCSlots {
				paths = append(paths, fmt.Sprintf("%s/%s-%d", dir, v, i))
			}
		}
	}

	// /tmp fallback for systems without XDG_RUNTIME_DIR.
	for _, v := range variants {
		for i := range maxIPCSlots {
			paths = append(paths, fmt.Sprintf("/tmp/%s-%d", v, i))
		}
	}

	// Snap-packaged Discord uses a distinct socket directory.
	uid := strconv.Itoa(os.Getuid())
	snapDirs := []string{"snap.discord", "snap.discord-canary", "snap.discord-ptb"}
	for _, sd := range snapDirs {
		for i := range maxIPCSlots {
			paths = append(paths, fmt.Sprintf("/run/user/%s/%s/discord-ipc-%d", uid, sd, i))
		}
	}

	// Flatpak-packaged Discord uses its own app-scoped directory.
	flatpakApps := []string{
		"com.discordapp.Discord",
		"com.discordapp.DiscordCanary",
		"com.discordapp.DiscordPTB",
	}
	for _, app := range flatpakApps {
		for i := range maxIPCSlots {
			paths = append(paths, fmt.Sprintf("/run/user/%s/app/%s/discord-ipc-%d", uid, app, i))
		}
	}

	// On WSL, append additional paths where a relay bridge (socat + npiperelay)
	// may have created the socket. Many of these overlap with the standard paths
	// above, but deduplication is not necessary since Dial on a missing path is
	// cheap and fast.
	paths = append(paths, wslSocketPaths()...)

	for _, path := range paths {
		conn, err := net.Dial("unix", path)
		if err == nil {
			return conn, nil
		}
	}

	if isWSL() {
		return nil, fmt.Errorf("%w: running under WSL — you may need to set up a relay with socat + npiperelay.exe (see project docs)", ErrIPCNotAvailable)
	}
	return nil, ErrIPCNotAvailable
}

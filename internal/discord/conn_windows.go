// conn_windows.go implements Discord IPC socket discovery for Windows.
// It connects via named pipes (\\.\pipe\discord-ipc-N) using the go-winio
// library.

//go:build windows

package discord

import (
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
)

// ///////////////////////////////////////////////
// Connection
// ///////////////////////////////////////////////

// connectToDiscord tries each Discord named pipe slot and returns the first
// successful connection.
func connectToDiscord() (net.Conn, error) {
	for i := range maxIPCSlots {
		conn, err := winio.DialPipe(fmt.Sprintf(`\\.\pipe\discord-ipc-%d`, i), nil)
		if err == nil {
			return conn, nil
		}
	}
	return nil, ErrIPCNotAvailable
}

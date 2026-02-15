// Unix/Darwin signal handling for graceful daemon shutdown.
//
// This file is compiled on all non-Windows platforms (Linux, macOS, *BSD).
// It listens for both SIGINT (Ctrl+C) and SIGTERM, the conventional signal
// sent by process managers (systemd, launchd) and container runtimes to
// request a graceful stop.

//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// ///////////////////////////////////////////////
// Signal Handling
// ///////////////////////////////////////////////

// signalChannel returns a buffered channel that receives SIGINT and SIGTERM.
// The buffer size of 1 ensures the signal is not lost if the receiver is
// briefly busy when the signal arrives.
func signalChannel() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	return ch
}

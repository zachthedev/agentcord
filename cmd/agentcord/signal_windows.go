// Windows signal handling for graceful daemon shutdown.
//
// This file is compiled only on Windows. Windows does not support POSIX
// signals like SIGTERM, so only [os.Interrupt] (Ctrl+C / CTRL_C_EVENT) is
// registered. The Go runtime translates CTRL_BREAK_EVENT and console-close
// events into os.Interrupt as well, providing adequate shutdown coverage.

//go:build windows

package main

import (
	"os"
	"os/signal"
)

// ///////////////////////////////////////////////
// Signal Handling
// ///////////////////////////////////////////////

// signalChannel returns a buffered channel that receives os.Interrupt
// (Ctrl+C). On Windows, SIGTERM is not available; the Go runtime maps
// CTRL_BREAK_EVENT and console-close events to os.Interrupt automatically.
// The buffer size of 1 ensures the signal is not lost if the receiver is
// briefly busy when the signal arrives.
func signalChannel() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	return ch
}

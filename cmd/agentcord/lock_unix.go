// Unix/Darwin file locking using flock(2).
//
// This file is compiled on all non-Windows platforms (Linux, macOS, *BSD).
// It uses POSIX advisory locking via [syscall.Flock] to enforce single-instance
// daemon execution through the PID file.

//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

// ///////////////////////////////////////////////
// File Locking
// ///////////////////////////////////////////////

// lockFile acquires an exclusive, non-blocking advisory lock on f using
// flock(2). The LOCK_NB flag causes an immediate error (EWOULDBLOCK) if
// another process already holds the lock, which the daemon uses to detect a
// running instance.
func lockFile(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("lock file %s: %w", f.Name(), err)
	}
	return nil
}

// unlockFile releases the advisory flock held on f. The lock is also
// implicitly released when the file descriptor is closed.
func unlockFile(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("unlock file %s: %w", f.Name(), err)
	}
	return nil
}

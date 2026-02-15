// Windows file locking using LockFileEx/UnlockFileEx.
//
// This file is compiled only on Windows. It uses the Win32 LockFileEx API via
// [golang.org/x/sys/windows] to enforce single-instance daemon execution
// through the PID file. The LOCKFILE_FAIL_IMMEDIATELY flag mirrors the
// non-blocking behavior of LOCK_NB on Unix.

//go:build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// ///////////////////////////////////////////////
// File Locking
// ///////////////////////////////////////////////

// lockFile acquires an exclusive, non-blocking lock on f using LockFileEx.
// The LOCKFILE_FAIL_IMMEDIATELY flag causes an immediate error if another
// process already holds the lock, which the daemon uses to detect a running
// instance. Only the first byte is locked (length 1, offset 0) since the
// lock exists purely for mutual exclusion, not data protection.
func lockFile(f *os.File) error {
	ol := new(windows.Overlapped)
	if err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1, 0,
		ol,
	); err != nil {
		return fmt.Errorf("lock file %s: %w", f.Name(), err)
	}
	return nil
}

// unlockFile releases the exclusive lock held on f via UnlockFileEx. The lock
// is also implicitly released when the file handle is closed.
func unlockFile(f *os.File) error {
	ol := new(windows.Overlapped)
	if err := windows.UnlockFileEx(
		windows.Handle(f.Fd()),
		0,
		1, 0,
		ol,
	); err != nil {
		return fmt.Errorf("unlock file %s: %w", f.Name(), err)
	}
	return nil
}

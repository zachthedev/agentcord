// Package atomicfile provides crash-safe file writing using temporary files
// and atomic renames.

package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write atomically writes data to path using a temporary-file-and-rename
// strategy. It creates a temp file in the same directory as path, writes
// data, calls [os.File.Sync] to flush to disk, sets permissions with
// [os.Chmod], and then atomically renames the temp file to the target path.
// If any step fails the temp file is removed via a deferred [os.Remove].
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	f, err := os.CreateTemp(dir, base+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := f.Name()
	var success bool
	defer func() {
		if !success {
			os.Remove(tmpName)
		}
	}()

	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	success = true
	return nil
}

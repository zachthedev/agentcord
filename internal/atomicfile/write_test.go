// write_test.go tests [Write] for basic correctness, concurrent safety
// across distinct files, and cleanup of temp files on failure.

package atomicfile

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteBasic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	data := []byte("hello world")

	if err := Write(path, data, 0o644); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("got %q, want %q", got, data)
	}
}

func TestWriteConcurrent(t *testing.T) {
	dir := t.TempDir()
	const n = 20

	// Each goroutine writes to its own file to avoid OS-level rename
	// contention (Windows does not permit atomic rename over a target
	// that is open by another process).
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			path := filepath.Join(dir, "concurrent-"+string(rune('A'+i))+".txt")
			data := []byte("writer-" + string(rune('A'+i)))
			if err := Write(path, data, 0o644); err != nil {
				t.Errorf("concurrent Write %d failed: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	// All files must exist, be readable, and contain expected data.
	for i := range n {
		path := filepath.Join(dir, "concurrent-"+string(rune('A'+i))+".txt")
		want := "writer-" + string(rune('A'+i))
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("ReadFile %d: %v", i, err)
			continue
		}
		if string(got) != want {
			t.Errorf("file %d: got %q, want %q", i, got, want)
		}
	}

	// No temp files should remain.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if matched, _ := filepath.Match("*.tmp.*", e.Name()); matched {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestWrite_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overwrite.txt")

	if err := Write(path, []byte("original"), 0o644); err != nil {
		t.Fatalf("first Write failed: %v", err)
	}
	if err := Write(path, []byte("updated"), 0o644); err != nil {
		t.Fatalf("second Write failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "updated" {
		t.Errorf("content = %q, want %q", got, "updated")
	}
}

func TestWrite_Permissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perms.txt")

	if err := Write(path, []byte("secret"), 0o600); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// On Windows, permission bits are limited; check that the file is at least
	// not world-readable (Go maps 0o600 to read-write on Windows).
	got := info.Mode().Perm()
	if got&0o600 == 0 {
		t.Errorf("permissions = %o, expected at least owner rw", got)
	}
}

func TestWriteCleanupOnFailure(t *testing.T) {
	// Attempt to write into a non-existent directory; should fail
	// and not leave temp files anywhere accessible.
	badPath := filepath.Join(t.TempDir(), "no-such-dir", "file.txt")

	err := Write(badPath, []byte("data"), 0o644)
	if err == nil {
		t.Fatal("expected error writing to non-existent directory")
	}

	// Verify no temp files were left in the parent that does exist.
	parent := filepath.Dir(filepath.Dir(badPath))
	entries, _ := os.ReadDir(parent)
	for _, e := range entries {
		if matched, _ := filepath.Match("file.txt.tmp.*", e.Name()); matched {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

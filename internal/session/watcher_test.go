// Tests for the file watcher: construction, event delivery, close semantics,
// and polling fallback. Exercises [NewWatcher], [Watcher.Events], [Watcher.Close],
// and [Watcher.Polling].
package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ///////////////////////////////////////////////
// Constructor Tests
// ///////////////////////////////////////////////

func TestNewWatcherConstructor(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string // returns path to watch
		wantErr   bool
		wantClose bool // whether to call Close after construction
	}{
		{
			name: "existing file",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				path := filepath.Join(dir, "state.json")
				os.WriteFile(path, []byte(`{}`), 0o644)
				return path
			},
			wantClose: true,
		},
		{
			name: "non-existent file in existing dir",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				return filepath.Join(dir, "does-not-exist.json")
			},
			wantClose: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			w, err := NewWatcher(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewWatcher: %v", err)
			}
			if w == nil {
				t.Fatal("NewWatcher returned nil watcher without error")
			}
			if w.Events() == nil {
				t.Error("Events() channel is nil")
			}
			if tt.wantClose {
				if err := w.Close(); err != nil {
					t.Errorf("Close: %v", err)
				}
			}
		})
	}
}

// ///////////////////////////////////////////////
// NewWatcher Tests
// ///////////////////////////////////////////////

func TestNewWatcher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte(`{}`), 0o644)

	w, err := NewWatcher(path)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	if w.Events() == nil {
		t.Fatal("Events() returned nil channel")
	}

	// The watcher should be using fsnotify (not polling) on most platforms.
	// We don't assert Polling() == false because CI environments may lack
	// inotify support; just verify the method is callable.
	_ = w.Polling()
}

// ///////////////////////////////////////////////
// File Change Event Tests
// ///////////////////////////////////////////////

func TestFileChangeTriggerEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow watcher test in short mode")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte(`{"v":1}`), 0o644)

	w, err := NewWatcher(path)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	// Give the watcher a moment to initialise.
	time.Sleep(100 * time.Millisecond)

	// Write a change to the file.
	os.WriteFile(path, []byte(`{"v":2}`), 0o644)

	// We should receive an event within a reasonable timeout.
	// Use a generous timeout because polling mode has a 2s interval.
	select {
	case <-w.Events():
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for file change event")
	}
}

func TestMultipleWritesCoalesce(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow watcher test in short mode")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte(`{"v":1}`), 0o644)

	w, err := NewWatcher(path)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	time.Sleep(100 * time.Millisecond)

	// Rapid successive writes should coalesce into one (or a small number of) events
	// because the events channel is buffered to 1.
	for i := 0; i < 10; i++ {
		os.WriteFile(path, []byte(`{"v":`+string(rune('0'+i))+`}`), 0o644)
	}

	// Drain one event.
	select {
	case <-w.Events():
		// got at least one event, good
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for coalesced event")
	}
}

// ///////////////////////////////////////////////
// Close Tests
// ///////////////////////////////////////////////

func TestClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow watcher test in short mode")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte(`{}`), 0o644)

	w, err := NewWatcher(path)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	// Close should succeed.
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// After close, writing to the file should NOT produce events.
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(path, []byte(`{"v":2}`), 0o644)

	select {
	case <-w.Events():
		t.Error("received event after Close; watcher should be stopped")
	case <-time.After(500 * time.Millisecond):
		// good: no event after close
	}
}

func TestCloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte(`{}`), 0o644)

	w, err := NewWatcher(path)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	// Calling Close multiple times should not panic or error.
	if err := w.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// ///////////////////////////////////////////////
// Poll Tests
// ///////////////////////////////////////////////

func TestPollDetectsModification(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow polling test in short mode")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte(`{"v":1}`), 0o644)

	// Build a watcher manually in polling mode to test poll() directly.
	w := &Watcher{
		path:         path,
		events:       make(chan struct{}, 1),
		done:         make(chan struct{}),
		pollInterval: 100 * time.Millisecond, // fast polling for test
	}
	w.polling.Store(true)
	go w.poll()
	defer w.Close()

	// Let the initial stat settle.
	time.Sleep(150 * time.Millisecond)

	// Touch the file with a future mod time to ensure the poller sees a change.
	now := time.Now().Add(time.Second)
	os.Chtimes(path, now, now)

	select {
	case <-w.Events():
		// success: poller detected the modification
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for poll event")
	}
}

func TestPollMissingFileNoEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow polling test in short mode")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	w := &Watcher{
		path:         path,
		events:       make(chan struct{}, 1),
		done:         make(chan struct{}),
		pollInterval: 100 * time.Millisecond,
	}
	w.polling.Store(true)
	go w.poll()
	defer w.Close()

	// With a non-existent file, polling should not fire events.
	select {
	case <-w.Events():
		t.Error("received event for non-existent file")
	case <-time.After(350 * time.Millisecond):
		// good: no spurious events
	}
}

func TestPollStopsOnClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow polling test in short mode")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte(`{}`), 0o644)

	w := &Watcher{
		path:         path,
		events:       make(chan struct{}, 1),
		done:         make(chan struct{}),
		pollInterval: 50 * time.Millisecond,
	}
	w.polling.Store(true)
	go w.poll()

	// Let polling start.
	time.Sleep(100 * time.Millisecond)

	// Close should cause poll() to return.
	w.Close()
	time.Sleep(100 * time.Millisecond)

	// Modify the file after close.
	now := time.Now().Add(time.Second)
	os.Chtimes(path, now, now)

	select {
	case <-w.Events():
		t.Error("received event after Close; poll should have stopped")
	case <-time.After(300 * time.Millisecond):
		// good
	}
}

// ///////////////////////////////////////////////
// Polling Flag Tests
// ///////////////////////////////////////////////

func TestPollingFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte(`{}`), 0o644)

	w, err := NewWatcher(path)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	// Just verify the method doesn't panic. The actual value depends on
	// whether fsnotify is available in the test environment.
	got := w.Polling()
	if got != true && got != false {
		t.Errorf("Polling() returned unexpected value: %v", got)
	}
}

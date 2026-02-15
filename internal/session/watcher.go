package session

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ///////////////////////////////////////////////
// Watcher
// ///////////////////////////////////////////////

// Watcher monitors a state file for changes using fsnotify with a polling fallback.
type Watcher struct {
	// path is the absolute path to the state file being monitored.
	path string
	// events delivers a signal each time the state file changes.
	// The channel is buffered to 1 so back-to-back writes coalesce.
	events chan struct{}
	// done is closed by [Watcher.Close] to signal goroutines to exit.
	done chan struct{}
	// fsw is the underlying fsnotify watcher; nil when polling.
	fsw *fsnotify.Watcher
	// once ensures [Watcher.Close] is idempotent.
	once sync.Once
	// polling is true when the watcher has fallen back to stat-based polling.
	polling atomic.Bool
	// pollInterval is the duration between stat calls in polling mode.
	pollInterval time.Duration
}

// NewWatcher creates a new Watcher for the given state file path.
// It uses fsnotify as the primary change detection mechanism and falls back
// to polling if fsnotify is unavailable.
func NewWatcher(stateFilePath string) (*Watcher, error) {
	w := &Watcher{
		path:         stateFilePath,
		events:       make(chan struct{}, 1),
		done:         make(chan struct{}),
		pollInterval: 2 * time.Second,
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Info("fsnotify unavailable, falling back to polling", "error", err)
		w.polling.Store(true)
		go w.poll()
		return w, nil
	}

	w.fsw = fsw
	if err := fsw.Add(stateFilePath); err != nil {
		slog.Info("cannot watch file, falling back to polling", "path", stateFilePath, "error", err)
		fsw.Close()
		w.fsw = nil
		w.polling.Store(true)
		go w.poll()
		return w, nil
	}

	go w.watch()
	return w, nil
}

// NewDirWatcher creates a Watcher that monitors a directory for state file changes.
// It fires events when any file matching "state.*.json" or the legacy "state.json"
// is written or created inside dir.
func NewDirWatcher(dir string) (*Watcher, error) {
	w := &Watcher{
		path:         dir,
		events:       make(chan struct{}, 1),
		done:         make(chan struct{}),
		pollInterval: 2 * time.Second,
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Info("fsnotify unavailable, falling back to directory polling", "error", err)
		w.polling.Store(true)
		go w.pollDir()
		return w, nil
	}

	w.fsw = fsw
	if err := fsw.Add(dir); err != nil {
		slog.Info("cannot watch directory, falling back to polling", "path", dir, "error", err)
		fsw.Close()
		w.fsw = nil
		w.polling.Store(true)
		go w.pollDir()
		return w, nil
	}

	go w.watchDir()
	return w, nil
}

// isStateFile reports whether name is a state file (legacy or per-client).
func isStateFile(name string) bool {
	base := filepath.Base(name)
	if base == "state.json" {
		return true
	}
	return strings.HasPrefix(base, "state.") && strings.HasSuffix(base, ".json")
}

// watchDir loops over fsnotify events on a directory, forwarding write/create
// notifications for state files to the events channel.
func (w *Watcher) watchDir() {
	for {
		select {
		case <-w.done:
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if (event.Has(fsnotify.Write) || event.Has(fsnotify.Create)) && isStateFile(event.Name) {
				w.notify()
			}
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			slog.Info("fsnotify error, switching to directory polling", "error", err)
			w.fsw.Close()
			w.fsw = nil
			w.polling.Store(true)
			go w.pollDir()
			return
		}
	}
}

// pollDir periodically scans the directory for state file changes and sends a
// notification when any state file's modification time advances.
func (w *Watcher) pollDir() {
	lastMod := w.latestStateMod()

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			mod := w.latestStateMod()
			if mod.After(lastMod) {
				lastMod = mod
				w.notify()
			}
		}
	}
}

// latestStateMod returns the most recent modification time among state files
// in the watched directory.
func (w *Watcher) latestStateMod() time.Time {
	var latest time.Time
	entries, err := os.ReadDir(w.path)
	if err != nil {
		return latest
	}
	for _, e := range entries {
		if e.IsDir() || !isStateFile(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest
}

// Polling reports whether the watcher is using polling instead of fsnotify.
func (w *Watcher) Polling() bool {
	return w.polling.Load()
}

// Events returns a channel that receives a signal when the state file changes.
func (w *Watcher) Events() <-chan struct{} {
	return w.events
}

// Close stops the watcher and releases resources.
func (w *Watcher) Close() error {
	var err error
	w.once.Do(func() {
		close(w.done)
		if w.fsw != nil {
			if closeErr := w.fsw.Close(); closeErr != nil {
				err = fmt.Errorf("closing fsnotify watcher: %w", closeErr)
			}
		}
	})
	return err
}

// watch loops over fsnotify events and forwards write/create notifications
// to the events channel. If fsnotify encounters an error, watch closes the
// native watcher and falls back to [Watcher.poll].
func (w *Watcher) watch() {
	for {
		select {
		case <-w.done:
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				w.notify()
			}
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			slog.Info("fsnotify error, switching to polling", "error", err)
			w.fsw.Close()
			w.fsw = nil
			w.polling.Store(true)
			go w.poll()
			return
		}
	}
}

// poll periodically stats the state file and sends a notification when the
// modification time advances. It runs as a fallback when fsnotify is unavailable.
func (w *Watcher) poll() {
	var lastMod time.Time
	info, err := os.Stat(w.path)
	if err == nil {
		lastMod = info.ModTime()
	}

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			info, err := os.Stat(w.path)
			if err != nil {
				continue
			}
			if info.ModTime().After(lastMod) {
				lastMod = info.ModTime()
				w.notify()
			}
		}
	}
}

// notify sends a single signal to the events channel. If a signal is already
// pending the call is a no-op, coalescing rapid successive changes.
func (w *Watcher) notify() {
	select {
	case w.events <- struct{}{}:
	default:
		// Channel already has a pending event, skip
	}
}

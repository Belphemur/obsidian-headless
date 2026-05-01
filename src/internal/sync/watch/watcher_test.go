package watch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func testLogger(t *testing.T) zerolog.Logger {
	return zerolog.New(zerolog.NewConsoleWriter()).Level(zerolog.Disabled)
}

// waitForEvent reads from the watcher channel until the predicate matches or timeout expires.
func waitForEvent(t *testing.T, ch <-chan ScanEvent, timeout time.Duration, desc string, pred func(ScanEvent) bool) ScanEvent {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("channel closed")
			}
			if pred(ev) {
				return ev
			}
		case <-timer.C:
			t.Fatalf("timed out after %v waiting for: %s", timeout, desc)
		}
	}
}

func TestWatcher_RenameDetection(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("inode tracking not available on Windows")
	}

	logger := testLogger(t)
	root := t.TempDir()

	w, err := New(root, nil, logger, 0)
	if err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()

	go w.Run(ctx)

	// Create initial file
	oldPath := filepath.Join(root, "old.md")
	if err := os.WriteFile(oldPath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for the Create event
	_ = waitForEvent(t, w.Out, 10*time.Second, "initial Create event", func(ev ScanEvent) bool {
		return ev.Type == EventCreate && ev.Path == oldPath
	})

	// Rename the file (atomic on same filesystem)
	newPath := filepath.Join(root, "new.md")
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}

	// Wait for the rename event
	ev := waitForEvent(t, w.Out, 10*time.Second, "rename event", func(ev ScanEvent) bool {
		return ev.Type == EventRename
	})
	if ev.Path != newPath {
		t.Fatalf("expected new path %s, got %s", newPath, ev.Path)
	}
	if ev.OldPath != oldPath {
		t.Fatalf("expected old path %s, got %s", oldPath, ev.OldPath)
	}
}

func TestWatcher_RealDeletion_NoMatchingCreate(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("inode tracking not available on Windows")
	}

	logger := testLogger(t)
	root := t.TempDir()

	w, err := New(root, nil, logger, 0)
	if err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()

	go w.Run(ctx)

	// Create file
	path := filepath.Join(root, "temp.md")
	if err := os.WriteFile(path, []byte("temp"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for Create
	_ = waitForEvent(t, w.Out, 10*time.Second, "Create event", func(ev ScanEvent) bool {
		return ev.Type == EventCreate && ev.Path == path
	})

	// Delete file
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	// Wait for delete after window expiry (150ms + quiescence 2s)
	ev := waitForEvent(t, w.Out, 10*time.Second, "Remove event", func(ev ScanEvent) bool {
		return ev.Type == EventRemove
	})
	if ev.Path != path {
		t.Fatalf("expected path %s, got %s", path, ev.Path)
	}
}

func TestWatcher_Shutdown_FlushesPendingRenames(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("inode tracking not available on Windows")
	}

	logger := testLogger(t)
	root := t.TempDir()

	w, err := New(root, nil, logger, 0)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go w.Run(ctx)

	// Create file so scanner knows about it
	path := filepath.Join(root, "temp.md")
	if err := os.WriteFile(path, []byte("temp"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for Create
	_ = waitForEvent(t, w.Out, 10*time.Second, "Create event", func(ev ScanEvent) bool {
		return ev.Type == EventCreate && ev.Path == path
	})

	// Remove file (should trigger deferDeletion)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	// Short wait to let the watcher process the fsnotify event
	time.Sleep(200 * time.Millisecond)

	// Immediately cancel (triggers shutdown -> flushPendingRenames)
	cancel()

	// Collect all remaining events
	ev := waitForEvent(t, w.Out, 10*time.Second, "EventRemove for pending deletion during shutdown", func(ev ScanEvent) bool {
		return ev.Type == EventRemove && ev.Path == path
	})
	_ = ev
}

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

func TestWatcher_IgnorePaths_SuppressesRemove(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w, err := New(dir, nil, zerolog.Nop(), 0)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	go w.Run(ctx)

	// Create a file first (before adding to ignore set)
	filePath := filepath.Join(dir, "old.md")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Drain initial create event — verify both event type and path (watcher emits absolute paths)
	_ = waitForEvent(t, w.Out, 5*time.Second, "initial create for old.md", func(ev ScanEvent) bool {
		return ev.Type == EventCreate && ev.Path == filePath
	})

	// Now add old path to ignored set
	w.AddIgnorePaths([]RenamePair{{OldPath: "old.md", NewPath: "new.md"}})

	// Now simulate a remove by the remote rename fixup
	if err := os.Remove(filePath); err != nil {
		t.Fatal(err)
	}

	// The remove should be SUPPRESSED
	select {
	case ev := <-w.Out:
		t.Fatalf("received unexpected event for suppressed path: %v", ev)
	case <-time.After(500 * time.Millisecond):
		// Expected: no event
	}
}

func TestWatcher_IgnorePaths_SuppressesCreate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w, err := New(dir, nil, zerolog.Nop(), 0)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	go w.Run(ctx)

	// Add new path to ignored set
	w.AddIgnorePaths([]RenamePair{{OldPath: "old.md", NewPath: "new.md"}})

	// Create file at new path (simulating remote rename creating new.md)
	filePath := filepath.Join(dir, "new.md")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// The create should be SUPPRESSED
	select {
	case ev := <-w.Out:
		t.Fatalf("received unexpected event for suppressed path: %v", ev)
	case <-time.After(2 * time.Second):
		// Expected: no event
	}
}

func TestWatcher_IgnorePaths_NotSuppressedWhenUnrelated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w, err := New(dir, nil, zerolog.Nop(), 0)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	go w.Run(ctx)

	// Add some paths to ignored set
	w.AddIgnorePaths([]RenamePair{{OldPath: "ignored-old.md", NewPath: "ignored-new.md"}})

	// Create an UNRELATED file
	filePath := filepath.Join(dir, "normal.md")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// The create should NOT be suppressed
	select {
	case ev := <-w.Out:
		if ev.Type != EventCreate {
			t.Fatalf("expected EventCreate, got %v", ev.Type)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for expected event")
	}
}

func TestWatcher_IgnorePaths_FlushIgnored(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w, err := New(dir, nil, zerolog.Nop(), 0)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	go w.Run(ctx)

	// Add path to ignored set, then flush
	w.AddIgnorePaths([]RenamePair{{OldPath: "old.md", NewPath: "new.md"}})
	w.FlushIgnored()

	// Now create file at new path — should NOT be suppressed (flush cleared it)
	filePath := filepath.Join(dir, "new.md")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-w.Out:
		if ev.Type != EventCreate {
			t.Fatalf("expected EventCreate after flush, got %v", ev.Type)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for expected event after flush")
	}
}

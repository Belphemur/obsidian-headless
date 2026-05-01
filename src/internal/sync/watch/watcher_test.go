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

func TestWatcher_RenameDetection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("inode tracking not available on Windows")
	}

	logger := testLogger(t)
	root := t.TempDir()

	w, err := New(root, nil, logger, 0)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go w.Run(ctx)

	// Create initial file
	oldPath := filepath.Join(root, "old.md")
	if err := os.WriteFile(oldPath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for the Create event
	var gotCreate bool
	for !gotCreate {
		select {
		case ev := <-w.Out:
			if ev.Type == EventCreate && ev.Path == oldPath {
				gotCreate = true
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for initial Create event")
		}
	}

	// Rename the file (atomic on same filesystem)
	newPath := filepath.Join(root, "new.md")
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}

	// Wait for the rename event
	select {
	case ev := <-w.Out:
		if ev.Type != EventRename {
			t.Fatalf("expected EventRename, got %s", ev.Type)
		}
		if ev.Path != newPath {
			t.Fatalf("expected new path %s, got %s", newPath, ev.Path)
		}
		if ev.OldPath != oldPath {
			t.Fatalf("expected old path %s, got %s", oldPath, ev.OldPath)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for rename event")
	}
}

func TestWatcher_RealDeletion_NoMatchingCreate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("inode tracking not available on Windows")
	}

	logger := testLogger(t)
	root := t.TempDir()

	w, err := New(root, nil, logger, 0)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go w.Run(ctx)

	// Create file
	path := filepath.Join(root, "temp.md")
	if err := os.WriteFile(path, []byte("temp"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for Create
	var gotCreate bool
	for !gotCreate {
		select {
		case ev := <-w.Out:
			if ev.Type == EventCreate && ev.Path == path {
				gotCreate = true
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for Create event")
		}
	}

	// Delete file
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	// Wait for delete after window expiry (150ms + quiescence 2s)
	select {
	case ev := <-w.Out:
		if ev.Type != EventRemove {
			t.Fatalf("expected EventRemove, got %s", ev.Type)
		}
		if ev.Path != path {
			t.Fatalf("expected path %s, got %s", path, ev.Path)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Remove event")
	}
}

func TestWatcher_Shutdown_FlushesPendingRenames(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("inode tracking not available on Windows")
	}

	logger := testLogger(t)
	root := t.TempDir()

	w, err := New(root, nil, logger, 0)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	go w.Run(ctx)

	// Create file so scanner knows about it
	path := filepath.Join(root, "temp.md")
	if err := os.WriteFile(path, []byte("temp"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for Create
	var gotCreate bool
	for !gotCreate {
		select {
		case ev := <-w.Out:
			if ev.Type == EventCreate && ev.Path == path {
				gotCreate = true
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for Create event")
		}
	}

	// Remove file (should trigger deferDeletion)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	// Short wait to let the watcher process the fsnotify event
	time.Sleep(200 * time.Millisecond)

	// Immediately cancel (triggers shutdown -> flushPendingRenames)
	cancel()

	// Collect all remaining events
	var foundRemove bool
	timeout := time.After(3 * time.Second)
loop:
	for {
		select {
		case ev, ok := <-w.Out:
			if !ok {
				break loop
			}
			if ev.Type == EventRemove && ev.Path == path {
				foundRemove = true
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	if !foundRemove {
		t.Fatal("expected EventRemove for pending deletion during shutdown")
	}
}

package watch

import (
	"context"
	"testing"
	"time"
)

func TestAggregator_PushRename(t *testing.T) {
	t.Parallel()
	out := make(chan ScanEvent, 10)
	agg := NewAggregator(out)

	agg.PushRename("/new/path", "/old/path")

	// Wait for quiescence delay to expire
	select {
	case ev := <-out:
		if ev.Type != EventRename {
			t.Fatalf("expected EventRename, got %s", ev.Type)
		}
		if ev.Path != "/new/path" {
			t.Fatalf("expected path /new/path, got %s", ev.Path)
		}
		if ev.OldPath != "/old/path" {
			t.Fatalf("expected oldPath /old/path, got %s", ev.OldPath)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for rename event")
	}
}

func TestAggregator_PushRename_Overwrites(t *testing.T) {
	out := make(chan ScanEvent, 10)
	agg := NewAggregator(out)

	// First push a regular create
	oldDelay := quiescenceDelay
	quiescenceDelay = 500 * time.Millisecond // reduce delay for test
	t.Cleanup(func() { quiescenceDelay = oldDelay })
	agg.Push("/test", EventCreate)

	// Then push a rename for the same path (should overwrite create)
	agg.PushRename("/test", "/old")

	select {
	case ev := <-out:
		if ev.Type != EventRename {
			t.Fatalf("expected EventRename (rename should overwrite create), got %s", ev.Type)
		}
		if ev.Path != "/test" {
			t.Fatalf("expected path /test, got %s", ev.Path)
		}
		if ev.OldPath != "/old" {
			t.Fatalf("expected oldPath /old, got %s", ev.OldPath)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for rename event")
	}
}

func TestAggregator_Shutdown_PreservesOldPath(t *testing.T) {
	t.Parallel()
	out := make(chan ScanEvent, 10)
	agg := NewAggregator(out)

	agg.PushRename("/new", "/old")

	// Shutdown immediately (don't wait for quiescence)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		agg.Shutdown(ctx)
		close(done)
	}()

	select {
	case ev := <-out:
		if ev.Type != EventRename {
			t.Fatalf("expected EventRename from shutdown, got %s", ev.Type)
		}
		if ev.OldPath != "/old" {
			t.Fatalf("expected oldPath /old, got %s", ev.OldPath)
		}
		if ev.Path != "/new" {
			t.Fatalf("expected path /new, got %s", ev.Path)
		}
	case <-done:
		t.Fatal("expected event from Shutdown")
	}
}

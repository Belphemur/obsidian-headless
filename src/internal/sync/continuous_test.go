package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Belphemur/obsidian-headless/internal/model"
	"github.com/Belphemur/obsidian-headless/internal/storage"
	watchpkg "github.com/Belphemur/obsidian-headless/internal/sync/watch"
	"github.com/rs/zerolog"
)

// waitFor polls cond every 100ms until it returns true or timeout expires.
// On timeout, t.Fatalf is called with a descriptive message.
func waitFor(t *testing.T, timeout time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting for: %s", timeout, desc)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestContinuousInitialSync(t *testing.T) {
	t.Parallel()
	mock := newMockSyncServer(t)
	mock.addRecord("remote.md", 1, []byte("remote content"))

	server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	wsURL := "ws://" + u.Host

	vault := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.db")

	e := &Engine{
		Config: model.SyncConfig{
			VaultID:   "test-continuous-initial-vault",
			VaultPath: vault,
			Host:      wsURL,
			StatePath: statePath,
			ConfigDir: ".obsidian",
		},
		Logger: testLogger(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- e.RunContinuous(ctx)
	}()

	// Wait for initial connection, handshake, debounce (500ms) and sync
	waitFor(t, 10*time.Second, "remote.md downloaded", func() bool {
		_, err := os.ReadFile(filepath.Join(vault, "remote.md"))
		return err == nil
	})

	// Verify remote file was downloaded without any local changes
	data, _ := os.ReadFile(filepath.Join(vault, "remote.md"))
	if string(data) != "remote content" {
		t.Fatalf("unexpected content: %q", string(data))
	}

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestContinuousWatcherSync(t *testing.T) {
	t.Parallel()
	mock := newMockSyncServer(t)

	server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	wsURL := "ws://" + u.Host

	vault := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.db")

	e := &Engine{
		Config: model.SyncConfig{
			VaultID:   "test-continuous-vault",
			VaultPath: vault,
			Host:      wsURL,
			StatePath: statePath,
			ConfigDir: ".obsidian",
		},
		Logger: testLogger(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- e.RunContinuous(ctx)
	}()

	// Wait briefly for connection establishment (the server needs a moment to accept)
	time.Sleep(200 * time.Millisecond)

	// Create local file
	mustWriteFile(t, filepath.Join(vault, "new.md"), []byte("new content"))

	// Wait for watcher quiescence + debounce + sync
	waitFor(t, 10*time.Second, "new.md uploaded to mock server", func() bool {
		mock.mu.Lock()
		defer mock.mu.Unlock()
		for _, content := range mock.contentByUID {
			if string(content) == "new content" {
				return true
			}
		}
		return false
	})

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestContinuousPushSync(t *testing.T) {
	t.Parallel()
	mock := newMockSyncServer(t)
	mock.addRecord("remote.md", 1, []byte("remote content"))

	server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	wsURL := "ws://" + u.Host

	vault := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.db")

	e := &Engine{
		Config: model.SyncConfig{
			VaultID:   "test-continuous-push-vault",
			VaultPath: vault,
			Host:      wsURL,
			StatePath: statePath,
			ConfigDir: ".obsidian",
		},
		Logger: testLogger(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- e.RunContinuous(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	mustWriteFile(t, filepath.Join(vault, "local.md"), []byte("local content"))

	waitFor(t, 10*time.Second, "local.md uploaded to mock server", func() bool {
		mock.mu.Lock()
		defer mock.mu.Unlock()
		for _, content := range mock.contentByUID {
			if string(content) == "local content" {
				return true
			}
		}
		return false
	})

	// Verify remote file was downloaded
	waitFor(t, 10*time.Second, "remote.md downloaded", func() bool {
		_, err := os.ReadFile(filepath.Join(vault, "remote.md"))
		return err == nil
	})
	data, _ := os.ReadFile(filepath.Join(vault, "remote.md"))
	if string(data) != "remote content" {
		t.Fatalf("unexpected content: %q", string(data))
	}

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestContinuousReconnection(t *testing.T) {
	t.Parallel()
	mock := newMockSyncServer(t)

	server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	wsURL := "ws://" + u.Host

	vault := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.db")

	e := &Engine{
		Config: model.SyncConfig{
			VaultID:   "test-continuous-reconnect-vault",
			VaultPath: vault,
			Host:      wsURL,
			StatePath: statePath,
			ConfigDir: ".obsidian",
		},
		Logger: testLogger(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- e.RunContinuous(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Force close all connections on the server side
	server.CloseClientConnections()

	// Wait for reconnection - use waitFor to detect when reconnect happens
	// by checking if we can upload a file successfully
	time.Sleep(500 * time.Millisecond) // brief wait for disconnect to propagate

	mustWriteFile(t, filepath.Join(vault, "after-reconnect.md"), []byte("after reconnect"))

	waitFor(t, 15*time.Second, "after-reconnect.md uploaded after reconnection", func() bool {
		mock.mu.Lock()
		defer mock.mu.Unlock()
		for _, content := range mock.contentByUID {
			if string(content) == "after reconnect" {
				return true
			}
		}
		return false
	})

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestRunContinuousInvalidPeriodicScan(t *testing.T) {
	t.Parallel()
	e := &Engine{
		Config: model.SyncConfig{
			VaultID:      "test-invalid-periodic",
			VaultPath:    t.TempDir(),
			PeriodicScan: "not-a-duration",
		},
		Logger: testLogger(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := e.RunContinuous(ctx)
	if err == nil {
		t.Fatal("expected error for invalid periodic-scan duration, got nil")
	}
	if !strings.Contains(err.Error(), "invalid periodic-scan duration") {
		t.Fatalf("expected invalid periodic-scan error, got: %v", err)
	}
}

func TestRunContinuousPeriodicScanZero(t *testing.T) {
	t.Parallel()
	mock := newMockSyncServer(t)
	server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	wsURL := "ws://" + u.Host

	vault := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.db")

	e := &Engine{
		Config: model.SyncConfig{
			VaultID:      "test-periodic-zero",
			VaultPath:    vault,
			Host:         wsURL,
			StatePath:    statePath,
			ConfigDir:    ".obsidian",
			PeriodicScan: "0",
		},
		Logger: testLogger(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- e.RunContinuous(ctx)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	if err := <-errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestRunContinuousPeriodicScanEmptyDefault(t *testing.T) {
	t.Parallel()
	mock := newMockSyncServer(t)
	server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	wsURL := "ws://" + u.Host

	vault := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.db")

	e := &Engine{
		Config: model.SyncConfig{
			VaultID:      "test-periodic-empty",
			VaultPath:    vault,
			Host:         wsURL,
			StatePath:    statePath,
			ConfigDir:    ".obsidian",
			PeriodicScan: "",
		},
		Logger: testLogger(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- e.RunContinuous(ctx)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	if err := <-errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestContinuousInitialSyncWithStaleState(t *testing.T) {
	t.Parallel()
	mock := newMockSyncServer(t)
	mock.addRecord("remote.md", 1, []byte("remote content"))

	server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	wsURL := "ws://" + u.Host

	vault := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.db")

	// Pre-populate state DB with stale local state (simulating a vault move)
	store, err := storage.Open(statePath)
	if err != nil {
		t.Fatal(err)
	}
	staleLocal := map[string]model.FileRecord{
		"remote.md": {Path: "remote.md", Hash: "stalehash", Size: 13, MTime: 1000},
	}
	staleRemote := map[string]model.FileRecord{
		"remote.md": {Path: "remote.md", Hash: "stalehash", Size: 13, MTime: 1000},
	}
	if err := store.ReplaceLocalFiles(staleLocal); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceServerFiles(staleRemote); err != nil {
		t.Fatal(err)
	}
	// Keep initial=true so the engine resets state and downloads
	if err := store.SetInitial(true); err != nil {
		t.Fatal(err)
	}
	store.Close()

	e := &Engine{
		Config: model.SyncConfig{
			VaultID:   "test-continuous-stale-state-vault",
			VaultPath: vault,
			Host:      wsURL,
			StatePath: statePath,
			ConfigDir: ".obsidian",
		},
		Logger: testLogger(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- e.RunContinuous(ctx)
	}()

	// Wait for initial connection, handshake, debounce (500ms) and sync
	waitFor(t, 10*time.Second, "remote.md downloaded (stale state)", func() bool {
		_, err := os.ReadFile(filepath.Join(vault, "remote.md"))
		return err == nil
	})

	// Verify remote file was downloaded (not deleted from remote)
	data, _ := os.ReadFile(filepath.Join(vault, "remote.md"))
	if string(data) != "remote content" {
		t.Fatalf("unexpected content: %q", string(data))
	}

	// Verify the mock server still has the remote file
	mock.mu.Lock()
	recordCount := len(mock.recordsByPath)
	mock.mu.Unlock()
	if recordCount != 1 {
		t.Fatalf("remote file was incorrectly deleted from server; expected 1 record, got %d", recordCount)
	}

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestContinuousHeartbeatAfterReconnect(t *testing.T) {
	t.Parallel()
	mock := newMockSyncServer(t)

	server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	wsURL := "ws://" + u.Host

	vault := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.db")

	e := &Engine{
		Config: model.SyncConfig{
			VaultID:   "test-heartbeat-reconnect-vault",
			VaultPath: vault,
			Host:      wsURL,
			StatePath: statePath,
			ConfigDir: ".obsidian",
		},
		Logger: testLogger(),
	}

	// The server will close the WebSocket after responding to the first ping.
	// This forces a reconnect so we can verify the heartbeat restarts.
	mock.closeAfterPing = true

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- e.RunContinuous(ctx)
	}()

	// Wait for first ping (heartbeat ticker is 20s, first ping sent at ~20s)
	var initialPings int
	waitFor(t, 30*time.Second, "first ping received", func() bool {
		mock.mu.Lock()
		initialPings = mock.pingCount
		mock.mu.Unlock()
		return initialPings > 0
	})

	// Wait for ping after reconnect (server closes after first pong, client reconnects)
	waitFor(t, 30*time.Second, "ping after reconnect", func() bool {
		mock.mu.Lock()
		pings := mock.pingCount
		mock.mu.Unlock()
		return pings > initialPings
	})

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestApplyRenameFixups(t *testing.T) {
	t.Parallel()
	vaultPath := "/tmp/vault"
	oldPath := "old/file.md"
	newPath := "new/file.md"
	absOldPath := filepath.Join(vaultPath, oldPath)
	absNewPath := filepath.Join(vaultPath, newPath)

	t.Run("local rename fixup", func(t *testing.T) {
		t.Parallel()
		local := map[string]model.FileRecord{
			oldPath: {Path: oldPath, Hash: "abcdef", Size: 100, MTime: 1000},
		}
		remote := map[string]model.FileRecord{}

		renames := []watchpkg.ScanEvent{
			{Path: absNewPath, OldPath: absOldPath, Type: watchpkg.EventRename},
		}
		applyRenameFixups(local, remote, nil, renames, vaultPath, zerolog.Nop())

		// Old path should be gone
		if _, ok := local[oldPath]; ok {
			t.Fatal("expected old path to be deleted from local")
		}
		// New path should have the record
		rec, ok := local[newPath]
		if !ok {
			t.Fatal("expected new path to exist in local")
		}
		if rec.Hash != "abcdef" {
			t.Fatalf("expected hash 'abcdef', got %s", rec.Hash)
		}
		if rec.Path != newPath {
			t.Fatalf("expected Path %s, got %s", newPath, rec.Path)
		}
		if rec.PreviousPath != oldPath {
			t.Fatalf("expected PreviousPath %s, got %s", oldPath, rec.PreviousPath)
		}
	})

	t.Run("remote rename fixup", func(t *testing.T) {
		t.Parallel()
		local := map[string]model.FileRecord{}
		remote := map[string]model.FileRecord{
			oldPath: {Path: oldPath, Hash: "fedcba", Size: 200, MTime: 2000},
		}

		renames := []watchpkg.ScanEvent{
			{Path: absNewPath, OldPath: absOldPath, Type: watchpkg.EventRename},
		}
		applyRenameFixups(local, remote, nil, renames, vaultPath, zerolog.Nop())

		rec, ok := remote[newPath]
		if !ok {
			t.Fatal("expected new path to exist in remote")
		}
		if rec.Hash != "fedcba" {
			t.Fatalf("expected hash 'fedcba', got %s", rec.Hash)
		}
		if rec.PreviousPath != oldPath {
			t.Fatalf("expected PreviousPath %s, got %s", oldPath, rec.PreviousPath)
		}
	})

	t.Run("no matching record", func(t *testing.T) {
		t.Parallel()
		local := map[string]model.FileRecord{}
		remote := map[string]model.FileRecord{}

		renames := []watchpkg.ScanEvent{
			{Path: absNewPath, OldPath: filepath.Join(vaultPath, "nonexistent.md"), Type: watchpkg.EventRename},
		}
		applyRenameFixups(local, remote, nil, renames, vaultPath, zerolog.Nop())

		// Should not panic, no records added
		if len(local) != 0 || len(remote) != 0 {
			t.Fatal("expected no side effects for unmatched rename")
		}
	})

	t.Run("non-rename event ignored", func(t *testing.T) {
		t.Parallel()
		local := map[string]model.FileRecord{
			oldPath: {Path: oldPath, Hash: "abc", Size: 100, MTime: 1000},
		}
		remote := map[string]model.FileRecord{}

		renames := []watchpkg.ScanEvent{
			{Path: absOldPath, Type: watchpkg.EventRemove},
			{Path: absNewPath, OldPath: absOldPath, Type: watchpkg.EventRename},
		}
		applyRenameFixups(local, remote, nil, renames, vaultPath, zerolog.Nop())

		// Only the rename should be applied, not the remove
		rec, ok := local[newPath]
		if !ok {
			t.Fatal("expected rename to be applied")
		}
		if rec.PreviousPath != oldPath {
			t.Fatalf("expected PreviousPath %s, got %s", oldPath, rec.PreviousPath)
		}
	})
}

func TestApplyRenameFixupsCsRemote(t *testing.T) {
	t.Parallel()
	vaultPath := "/tmp/vault"
	oldPath := "old/file.md"
	newPath := "new/file.md"
	absOldPath := filepath.Join(vaultPath, oldPath)
	absNewPath := filepath.Join(vaultPath, newPath)

	t.Run("removes stale old path from csRemote", func(t *testing.T) {
		t.Parallel()
		local := map[string]model.FileRecord{}
		remote := map[string]model.FileRecord{
			oldPath: {Path: oldPath, Hash: "abc", Size: 100, MTime: 1000},
		}
		csRemote := map[string]model.FileRecord{
			oldPath: {Path: oldPath, Hash: "abc", Size: 100, MTime: 1000},
		}

		renames := []watchpkg.ScanEvent{
			{Path: absNewPath, OldPath: absOldPath, Type: watchpkg.EventRename},
		}
		applyRenameFixups(local, remote, csRemote, renames, vaultPath, zerolog.Nop())

		if _, ok := csRemote[oldPath]; ok {
			t.Fatal("expected csRemote old path to be deleted")
		}
		rec, ok := remote[newPath]
		if !ok {
			t.Fatal("expected new path to exist in remote")
		}
		if rec.Hash != "abc" {
			t.Fatalf("expected hash 'abc', got %s", rec.Hash)
		}
		if rec.PreviousPath != oldPath {
			t.Fatalf("expected PreviousPath %s, got %s", oldPath, rec.PreviousPath)
		}
		if _, ok := remote[oldPath]; ok {
			t.Fatal("expected old path to be deleted from remote")
		}
	})

	t.Run("nil csRemote does not panic", func(t *testing.T) {
		t.Parallel()
		local := map[string]model.FileRecord{}
		remote := map[string]model.FileRecord{}

		renames := []watchpkg.ScanEvent{
			{Path: absNewPath, OldPath: absOldPath, Type: watchpkg.EventRename},
		}
		// Should not panic when csRemote is nil
		applyRenameFixups(local, remote, nil, renames, vaultPath, zerolog.Nop())
	})
}

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

type continuousTestEnv struct {
	mock      *mockSyncServer
	server    *httptest.Server
	wsURL     string
	vault     string
	statePath string
	engine    *Engine
	ctx       context.Context
	cancel    context.CancelFunc
	errCh     chan error
}

func startContinuousTest(t *testing.T, opts ...func(*continuousTestEnv)) *continuousTestEnv {
	t.Helper()

	env := &continuousTestEnv{}

	mock := newMockSyncServer(t)
	env.mock = mock

	server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	env.server = server
	t.Cleanup(server.Close)

	u, _ := url.Parse(server.URL)
	env.wsURL = "ws://" + u.Host

	env.vault = t.TempDir()
	env.statePath = filepath.Join(t.TempDir(), "state.db")

	e := &Engine{
		Config: model.SyncConfig{
			VaultID:   "test-continuous-vault",
			VaultPath: env.vault,
			Host:      env.wsURL,
			StatePath: env.statePath,
			ConfigDir: ".obsidian",
		},
		Logger: testLogger(),
	}
	env.engine = e

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	env.ctx = ctx
	env.cancel = cancel

	errCh := make(chan error, 1)
	env.errCh = errCh

	for _, opt := range opts {
		opt(env)
	}

	// Register cleanup after opts so withTimeout's replacement cancel is captured.
	t.Cleanup(func() { env.cancel() })

	go func() {
		errCh <- env.engine.RunContinuous(env.ctx)
	}()

	return env
}

func withTimeout(d time.Duration) func(*continuousTestEnv) {
	return func(env *continuousTestEnv) {
		if env.cancel != nil {
			env.cancel()
		}
		env.ctx, env.cancel = context.WithTimeout(context.Background(), d)
	}
}

func TestContinuousInitialSync(t *testing.T) {
	t.Parallel()
	env := startContinuousTest(t, func(env *continuousTestEnv) {
		env.mock.addRecord("remote.md", 1, []byte("remote content"))
	})

	// Wait for initial connection, handshake, debounce (500ms) and sync
	waitFor(t, 5*time.Second, "remote.md downloaded", func() bool {
		_, err := os.ReadFile(filepath.Join(env.vault, "remote.md"))
		return err == nil
	})

	// Verify remote file was downloaded without any local changes
	data, _ := os.ReadFile(filepath.Join(env.vault, "remote.md"))
	if string(data) != "remote content" {
		t.Fatalf("unexpected content: %q", string(data))
	}

	env.cancel()
	if err := <-env.errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestContinuousWatcherSync(t *testing.T) {
	t.Parallel()
	env := startContinuousTest(t)

	// Wait for connection establishment
	waitFor(t, 2*time.Second, "connection established", func() bool {
		env.mock.mu.Lock()
		defer env.mock.mu.Unlock()
		return len(env.mock.initMsgs) > 0
	})

	// Create local file
	mustWriteFile(t, filepath.Join(env.vault, "new.md"), []byte("new content"))

	// Wait for watcher quiescence + debounce + sync
	waitFor(t, 5*time.Second, "new.md uploaded to mock server", func() bool {
		env.mock.mu.Lock()
		defer env.mock.mu.Unlock()
		for _, content := range env.mock.contentByUID {
			if string(content) == "new content" {
				return true
			}
		}
		return false
	})

	env.cancel()
	if err := <-env.errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestContinuousPushSync(t *testing.T) {
	t.Parallel()
	env := startContinuousTest(t, func(env *continuousTestEnv) {
		env.mock.addRecord("remote.md", 1, []byte("remote content"))
	})

	waitFor(t, 2*time.Second, "connection established", func() bool {
		env.mock.mu.Lock()
		defer env.mock.mu.Unlock()
		return len(env.mock.initMsgs) > 0
	})

	mustWriteFile(t, filepath.Join(env.vault, "local.md"), []byte("local content"))

	waitFor(t, 5*time.Second, "local.md uploaded to mock server", func() bool {
		env.mock.mu.Lock()
		defer env.mock.mu.Unlock()
		for _, content := range env.mock.contentByUID {
			if string(content) == "local content" {
				return true
			}
		}
		return false
	})

	// Verify remote file was downloaded
	waitFor(t, 5*time.Second, "remote.md downloaded", func() bool {
		_, err := os.ReadFile(filepath.Join(env.vault, "remote.md"))
		return err == nil
	})
	data, _ := os.ReadFile(filepath.Join(env.vault, "remote.md"))
	if string(data) != "remote content" {
		t.Fatalf("unexpected content: %q", string(data))
	}

	env.cancel()
	if err := <-env.errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestContinuousReconnection(t *testing.T) {
	t.Parallel()
	env := startContinuousTest(t, withTimeout(20*time.Second))

	waitFor(t, 2*time.Second, "connection established", func() bool {
		env.mock.mu.Lock()
		defer env.mock.mu.Unlock()
		return len(env.mock.initMsgs) > 0
	})

	// Force close all connections on the server side
	env.server.CloseClientConnections()

	// Wait for reconnection - use waitFor to detect when reconnect happens
	// by checking if we can upload a file successfully
	time.Sleep(500 * time.Millisecond) // brief wait for disconnect to propagate

	mustWriteFile(t, filepath.Join(env.vault, "after-reconnect.md"), []byte("after reconnect"))

	waitFor(t, 15*time.Second, "after-reconnect.md uploaded after reconnection", func() bool {
		env.mock.mu.Lock()
		defer env.mock.mu.Unlock()
		for _, content := range env.mock.contentByUID {
			if string(content) == "after reconnect" {
				return true
			}
		}
		return false
	})

	env.cancel()
	if err := <-env.errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestRunContinuousInvalidPeriodicScan(t *testing.T) {
	t.Parallel()
	env := startContinuousTest(t, func(env *continuousTestEnv) {
		env.engine.Config.PeriodicScan = "not-a-duration"
	})

	err := <-env.errCh
	if err == nil {
		t.Fatal("expected error for invalid periodic-scan duration, got nil")
	}
	if !strings.Contains(err.Error(), "invalid periodic-scan duration") {
		t.Fatalf("expected invalid periodic-scan error, got: %v", err)
	}
}

func TestRunContinuousPeriodicScanZero(t *testing.T) {
	t.Parallel()
	env := startContinuousTest(t, func(env *continuousTestEnv) {
		env.engine.Config.PeriodicScan = "0"
	})

	waitFor(t, 2*time.Second, "connection established", func() bool {
		env.mock.mu.Lock()
		defer env.mock.mu.Unlock()
		return len(env.mock.initMsgs) > 0
	})
	env.cancel()

	if err := <-env.errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestRunContinuousPeriodicScanEmptyDefault(t *testing.T) {
	t.Parallel()
	env := startContinuousTest(t, func(env *continuousTestEnv) {
		env.engine.Config.PeriodicScan = ""
	})

	waitFor(t, 2*time.Second, "connection established", func() bool {
		env.mock.mu.Lock()
		defer env.mock.mu.Unlock()
		return len(env.mock.initMsgs) > 0
	})
	env.cancel()

	if err := <-env.errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestContinuousInitialSyncWithStaleState(t *testing.T) {
	t.Parallel()
	env := startContinuousTest(t, func(env *continuousTestEnv) {
		env.mock.addRecord("remote.md", 1, []byte("remote content"))

		// Pre-populate state DB with stale local state (simulating a vault move)
		store, err := storage.Open(env.statePath)
		if err != nil {
			t.Fatalf("failed to open state DB: %v", err)
		}
		staleLocal := map[string]model.FileRecord{
			"remote.md": {Path: "remote.md", Hash: "stalehash", Size: 13, MTime: 1000},
		}
		staleRemote := map[string]model.FileRecord{
			"remote.md": {Path: "remote.md", Hash: "stalehash", Size: 13, MTime: 1000},
		}
		if err := store.ReplaceLocalFiles(staleLocal); err != nil {
			t.Fatalf("failed to replace local files: %v", err)
		}
		if err := store.ReplaceServerFiles(staleRemote); err != nil {
			t.Fatalf("failed to replace server files: %v", err)
		}
		// Keep initial=true so the engine resets state and downloads
		if err := store.SetInitial(true); err != nil {
			t.Fatalf("failed to set initial flag: %v", err)
		}
		store.Close()
	})

	// Wait for initial connection, handshake, debounce (500ms) and sync
	waitFor(t, 5*time.Second, "remote.md downloaded (stale state)", func() bool {
		_, err := os.ReadFile(filepath.Join(env.vault, "remote.md"))
		return err == nil
	})

	// Verify remote file was downloaded (not deleted from remote)
	data, _ := os.ReadFile(filepath.Join(env.vault, "remote.md"))
	if string(data) != "remote content" {
		t.Fatalf("unexpected content: %q", string(data))
	}

	// Verify the mock server still has the remote file
	env.mock.mu.Lock()
	recordCount := len(env.mock.recordsByPath)
	env.mock.mu.Unlock()
	if recordCount != 1 {
		t.Fatalf("remote file was incorrectly deleted from server; expected 1 record, got %d", recordCount)
	}

	env.cancel()
	if err := <-env.errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestContinuousWorkerHandshakeVersion(t *testing.T) {
	t.Parallel()
	env := startContinuousTest(t, func(env *continuousTestEnv) {
		env.mock.addRecord("remote1.md", 1, []byte("remote content 1"))
		env.mock.addRecord("remote2.md", 2, []byte("remote content 2"))
		// Set concurrency to > 1 so workers are definitely used
		env.engine.Config.DownloadConcurrency = 2
	})

	// Wait for files to be downloaded
	waitFor(t, 5*time.Second, "remote files downloaded", func() bool {
		_, err1 := os.ReadFile(filepath.Join(env.vault, "remote1.md"))
		_, err2 := os.ReadFile(filepath.Join(env.vault, "remote2.md"))
		return err1 == nil && err2 == nil
	})

	// Wait for init messages to settle before cancelling
	waitFor(t, 5*time.Second, "worker init messages received", func() bool {
		env.mock.mu.Lock()
		defer env.mock.mu.Unlock()
		for _, msg := range env.mock.initMsgs {
			if initial, _ := msg["initial"].(bool); !initial {
				return true
			}
		}
		return false
	})

	env.cancel()
	if err := <-env.errCh; err != nil && err != context.Canceled &&
		!strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("RunContinuous error: %v", err)
	}

	env.mock.mu.Lock()
	defer env.mock.mu.Unlock()

	// We expect at least one init for the main connection (initial=true)
	// and at least one for a worker connection (initial=false)
	var workerInits []map[string]any
	for _, msg := range env.mock.initMsgs {
		initial, _ := msg["initial"].(bool)
		if !initial {
			workerInits = append(workerInits, msg)
		}
	}

	if len(workerInits) == 0 {
		t.Fatalf("no worker init messages received")
	}

	// Protocol version 1 as defined by the server's ready response
	const expectedVersion int64 = 1

	for _, msg := range workerInits {
		versionVal, ok := msg["version"].(float64)
		if !ok {
			t.Fatalf("worker init missing or invalid version: %v", msg)
		}
		if int64(versionVal) != expectedVersion {
			t.Fatalf("expected worker init version %d, got %v", expectedVersion, versionVal)
		}
	}
}

func TestContinuousHeartbeatAfterReconnect(t *testing.T) {
	t.Parallel()
	env := startContinuousTest(t, func(env *continuousTestEnv) {
		// The server will close the WebSocket after responding to the first ping.
		// This forces a reconnect so we can verify the heartbeat restarts.
		env.mock.closeAfterPing = true
		env.engine.testHeartbeatInterval = 500 * time.Millisecond
		env.engine.testHeartbeatSendThreshold = 250 * time.Millisecond
		env.engine.testHeartbeatTimeout = 3 * time.Second
	}, withTimeout(20*time.Second))

	// Wait for first ping (heartbeat ticker is 500ms)
	var initialPings int
	waitFor(t, 5*time.Second, "first ping received", func() bool {
		env.mock.mu.Lock()
		initialPings = env.mock.pingCount
		env.mock.mu.Unlock()
		return initialPings > 0
	})

	// Wait for ping after reconnect (server closes after first pong, client reconnects)
	// Reconnect backoff is 5s, so allow extra time beyond the heartbeat interval.
	// 15s = 5s backoff + 10s CI variability buffer to prevent flakiness on slow runners.
	waitFor(t, 15*time.Second, "ping after reconnect", func() bool {
		env.mock.mu.Lock()
		pings := env.mock.pingCount
		env.mock.mu.Unlock()
		return pings > initialPings
	})

	env.cancel()
	if err := <-env.errCh; err != nil && err != context.Canceled &&
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

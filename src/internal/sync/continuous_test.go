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

	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
	"github.com/Belphemur/obsidian-headless/src-go/internal/storage"
)

func TestContinuousInitialSync(t *testing.T) {
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
	time.Sleep(3 * time.Second)

	// Verify remote file was downloaded without any local changes
	data, err := os.ReadFile(filepath.Join(vault, "remote.md"))
	if err != nil {
		t.Fatalf("remote file was not downloaded during initial sync: %v", err)
	}
	if string(data) != "remote content" {
		t.Fatalf("unexpected content: %q", string(data))
	}

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestContinuousWatcherSync(t *testing.T) {
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

	// Wait for connection and initial handshake
	time.Sleep(500 * time.Millisecond)

	// Create a local file to trigger the watcher
	mustWriteFile(t, filepath.Join(vault, "new.md"), []byte("new content"))

	// Wait for watcher quiescence (2s) + debounce (500ms) + sync
	time.Sleep(4 * time.Second)

	// Verify file was uploaded to mock server
	found := false
	mock.mu.Lock()
	for _, content := range mock.contentByUID {
		if string(content) == "new content" {
			found = true
			break
		}
	}
	mock.mu.Unlock()
	if !found {
		t.Fatal("file was not uploaded by continuous sync")
	}

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestContinuousPushSync(t *testing.T) {
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

	// Wait for connection and initial handshake
	time.Sleep(500 * time.Millisecond)

	// Create a local file to trigger a sync
	mustWriteFile(t, filepath.Join(vault, "local.md"), []byte("local content"))

	// Wait for watcher quiescence (2s) + debounce (500ms) + sync
	time.Sleep(4 * time.Second)

	// Verify remote file was downloaded
	data, err := os.ReadFile(filepath.Join(vault, "remote.md"))
	if err != nil {
		t.Fatalf("remote file was not downloaded: %v", err)
	}
	if string(data) != "remote content" {
		t.Fatalf("unexpected content: %q", string(data))
	}

	// Verify local file was uploaded
	found := false
	mock.mu.Lock()
	for _, content := range mock.contentByUID {
		if string(content) == "local content" {
			found = true
			break
		}
	}
	mock.mu.Unlock()
	if !found {
		t.Fatal("local file was not uploaded")
	}

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestContinuousReconnection(t *testing.T) {
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

	// Wait for connection
	time.Sleep(500 * time.Millisecond)

	// Force close all connections on the server side
	server.CloseClientConnections()

	// Wait for reconnection backoff (5s) + reconnect + sync
	time.Sleep(7 * time.Second)

	// Create a file and verify it syncs after reconnect
	mustWriteFile(t, filepath.Join(vault, "after-reconnect.md"), []byte("after reconnect"))

	// Wait for watcher quiescence + debounce + sync
	time.Sleep(4 * time.Second)

	found := false
	mock.mu.Lock()
	for _, content := range mock.contentByUID {
		if string(content) == "after reconnect" {
			found = true
			break
		}
	}
	mock.mu.Unlock()
	if !found {
		t.Fatal("file was not uploaded after reconnection")
	}

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestRunContinuousInvalidPeriodicScan(t *testing.T) {
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

	time.Sleep(500 * time.Millisecond)
	cancel()

	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestRunContinuousPeriodicScanEmptyDefault(t *testing.T) {
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

	time.Sleep(500 * time.Millisecond)
	cancel()

	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestContinuousInitialSyncWithStaleState(t *testing.T) {
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
	time.Sleep(3 * time.Second)

	// Verify remote file was downloaded (not deleted from remote)
	data, err := os.ReadFile(filepath.Join(vault, "remote.md"))
	if err != nil {
		t.Fatalf("remote file was not downloaded during initial sync with stale state: %v", err)
	}
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
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

func TestContinuousHeartbeatAfterReconnect(t *testing.T) {
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

	// Wait for the first ping (server will close connection after pong)
	var initialPings int
	foundInitial := false
	for range 60 {
		time.Sleep(500 * time.Millisecond)
		mock.mu.Lock()
		initialPings = mock.pingCount
		mock.mu.Unlock()
		if initialPings > 0 {
			foundInitial = true
			break
		}
	}
	if !foundInitial {
		t.Fatal("expected at least one ping on initial connection")
	}

	// After the server closed the connection, the client should reconnect
	// and the heartbeat should resume, sending another ping.
	foundAfterReconnect := false
	for range 60 {
		time.Sleep(500 * time.Millisecond)
		mock.mu.Lock()
		finalPings := mock.pingCount
		mock.mu.Unlock()
		if finalPings > initialPings {
			foundAfterReconnect = true
			break
		}
	}
	if !foundAfterReconnect {
		t.Fatalf("expected ping after reconnect")
	}

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("RunContinuous error: %v", err)
	}
}

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
	for _, content := range mock.contentByUID {
		if string(content) == "new content" {
			found = true
			break
		}
	}
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
	for _, content := range mock.contentByUID {
		if string(content) == "local content" {
			found = true
			break
		}
	}
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
	for _, content := range mock.contentByUID {
		if string(content) == "after reconnect" {
			found = true
			break
		}
	}
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

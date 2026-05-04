package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Belphemur/obsidian-headless/internal/model"
)

func newTestEngine(t *testing.T, tmp string) *Engine {
	t.Helper()
	return &Engine{
		Config: model.SyncConfig{
			VaultID:   "test-vault",
			VaultPath: tmp,
			ConfigDir: ".obsidian",
		},
		Logger: testLogger(),
	}
}

func lockFileFor(tmp string) string {
	return filepath.Join(tmp, ".obsidian", ".sync.lock", "test-vault.lock")
}

func TestAcquireLockSuccess(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	e := newTestEngine(t, tmp)

	unlock, err := e.acquireLock()
	if err != nil {
		t.Fatalf("expected lock acquisition to succeed, got error: %v", err)
	}

	lockFile := lockFileFor(tmp)
	if _, err := os.Stat(lockFile); os.IsNotExist(err) {
		t.Fatalf("expected lock file to exist at %s", lockFile)
	}

	unlock()

	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Fatal("expected lock file to be removed after unlock")
	}
}

func TestAcquireLockExclusive(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	e := newTestEngine(t, tmp)

	unlock, err := e.acquireLock()
	if err != nil {
		t.Fatalf("first lock acquisition failed: %v", err)
	}
	defer unlock()

	_, err = e.acquireLock()
	if err == nil {
		t.Fatal("expected second lock acquisition to fail, but it succeeded")
	}
}

func TestAcquireLockConcurrent(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	e := newTestEngine(t, tmp)

	unlock, err := e.acquireLock()
	if err != nil {
		t.Fatalf("first lock acquisition failed: %v", err)
	}
	defer unlock()

	done := make(chan error, 1)
	go func() {
		_, err := e.acquireLock()
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected concurrent lock acquisition to fail")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for concurrent lock acquisition")
	}
}

func TestAcquireLockReacquireAfterUnlock(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	e := newTestEngine(t, tmp)

	unlock1, err := e.acquireLock()
	if err != nil {
		t.Fatalf("first lock acquisition failed: %v", err)
	}
	unlock1()

	unlock2, err := e.acquireLock()
	if err != nil {
		t.Fatalf("second lock acquisition after unlock failed: %v", err)
	}
	unlock2()
}

func TestAcquireLockStaleLockRecovery(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	e := newTestEngine(t, tmp)
	lockFile := lockFileFor(tmp)

	if err := os.MkdirAll(filepath.Dir(lockFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockFile, []byte("old lock"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-staleLockAge - time.Hour)
	if err := os.Chtimes(lockFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	unlock, err := e.acquireLock()
	if err != nil {
		t.Fatalf("expected lock acquisition to succeed with stale lock recovery, got error: %v", err)
	}
	unlock()
}

func TestAcquireLockFreshLockBlocksStaleRecovery(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	e := newTestEngine(t, tmp)
	lockFile := lockFileFor(tmp)

	if err := os.MkdirAll(filepath.Dir(lockFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockFile, []byte("fresh lock"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := e.acquireLock()
	if err == nil {
		t.Fatal("expected lock acquisition to fail with fresh lock file")
	}
}

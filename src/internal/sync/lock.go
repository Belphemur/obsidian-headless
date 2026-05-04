package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	configpkg "github.com/Belphemur/obsidian-headless/internal/config"
)

func (e *Engine) acquireLock() (func(), error) {
	lockDir := configpkg.LockPath(e.Config.VaultPath, e.configDir())
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, err
	}
	lockFile := filepath.Join(lockDir, e.Config.VaultID+".lock")

	f, err := openExclusive(lockFile)
	if err != nil {
		if !os.IsExist(err) {
			return nil, fmt.Errorf("cannot create lock file: %w", err)
		}

		info, statErr := os.Stat(lockFile)
		if statErr != nil || time.Since(info.ModTime()) <= staleLockAge {
			return nil, fmt.Errorf("sync in progress: %s", lockFile)
		}

		if remErr := os.Remove(lockFile); remErr != nil {
			return nil, fmt.Errorf("stale lock file but cannot remove: %w", remErr)
		}

		f, err = openExclusive(lockFile)
		if err != nil {
			return nil, fmt.Errorf("lock file removed but cannot recreate: %w", err)
		}
	}
	defer f.Close()

	host, _ := os.Hostname()
	fmt.Fprintf(f, "%d %s %s\n", os.Getpid(), host, time.Now().Format(time.RFC3339))

	return func() { _ = os.Remove(lockFile) }, nil
}

func openExclusive(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
}

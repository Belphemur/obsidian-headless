package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	configpkg "github.com/Belphemur/obsidian-headless/src-go/internal/config"
)

func (e *Engine) acquireLock() (func(), error) {
	lockDir := configpkg.LockPath(e.Config.VaultPath, e.configDir())
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, err
	}
	lockName := e.Config.VaultID + ".lock"
	lockFile := filepath.Join(lockDir, lockName)
	f, err := os.Create(lockFile)
	if err != nil {
		if os.IsExist(err) {
			info, statErr := os.Stat(lockFile)
			if statErr == nil && time.Since(info.ModTime()) > staleLockAge {
				if remErr := os.Remove(lockFile); remErr != nil {
					return nil, fmt.Errorf("stale lock file but cannot remove: %w", remErr)
				}
				f, err = os.Create(lockFile)
				if err != nil {
					return nil, fmt.Errorf("lock file removed but cannot recreate: %w", err)
				}
			} else {
				return nil, fmt.Errorf("sync in progress: %s", lockFile)
			}
		} else {
			return nil, fmt.Errorf("cannot create lock file: %w", err)
		}
	}
	host, _ := os.Hostname()
	fmt.Fprintf(f, "%d %s %s\n", os.Getpid(), host, time.Now().Format(time.RFC3339))
	f.Close()
	return func() {
		_ = os.Remove(lockFile)
	}, nil
}

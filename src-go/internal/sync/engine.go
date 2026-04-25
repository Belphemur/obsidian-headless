package sync

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	configpkg "github.com/Belphemur/obsidian-headless/src-go/internal/config"
	"github.com/Belphemur/obsidian-headless/src-go/internal/encryption"
	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
	"github.com/Belphemur/obsidian-headless/src-go/internal/storage"
	watchpkg "github.com/Belphemur/obsidian-headless/src-go/internal/sync/watch"
	"github.com/Belphemur/obsidian-headless/src-go/internal/util"
)

const (
	chunkSize         = 2 * 1024 * 1024
	maxRemoteFileSize = 200 * 1024 * 1024
	staleLockAge      = 24 * time.Hour
	syncInterval      = 30 * time.Second
)

type Engine struct {
	Config model.SyncConfig
	Token  string
	Logger zerolog.Logger

	enc    encryption.EncryptionProvider
	rawKey []byte

	conn      *websocket.Conn
	remote    map[string]model.FileRecord
	version   int64
	initial   bool
	stopClose func() bool

	mu sync.Mutex
}

func NewEngine(config model.SyncConfig, token string, logger zerolog.Logger) (*Engine, error) {
	e := &Engine{Config: config, Token: token, Logger: logger}

	var rawKey []byte
	if config.EncryptionVersion > 0 && config.EncryptionKey != "" {
		logger.Debug().Str("vaultID", config.VaultID).Str("encryptionVersion", fmt.Sprint(config.EncryptionVersion)).Msg("creating encryption provider")
		var err error
		rawKey, err = encryption.DeriveKey(config.EncryptionKey, config.EncryptionSalt)
		if err != nil {
			return nil, fmt.Errorf("failed to derive encryption key: %w", err)
		}
		logger.Debug().Int("rawKeyLen", len(rawKey)).Msg("derived raw key")
		enc, err := encryption.NewEncryptionProvider(
			encryption.EncryptionVersion(config.EncryptionVersion),
			rawKey,
			config.EncryptionSalt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create encryption provider: %w", err)
		}
		e.enc = enc
		e.rawKey = rawKey
	}

	e.remote = make(map[string]model.FileRecord)

	return e, nil
}

func (e *Engine) RunOnce(ctx context.Context) error {
	e.Logger.Info().Str("vault", e.Config.VaultID).Msg("sync start")
	e.mu.Lock()
	if e.conn == nil {
		e.mu.Unlock()
		if err := e.ensureConnected(ctx); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
	} else {
		e.mu.Unlock()
	}

	lock, err := e.acquireLock()
	if err != nil {
		return err
	}
	defer lock()
	statePath, err := configpkg.StatePath(e.Config.VaultID, e.Config.StatePath)
	if err != nil {
		return err
	}
	store, err := storage.Open(statePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = store.Close()
	}()
	previousLocal, err := store.LoadLocalFiles()
	if err != nil {
		return err
	}
	previousRemote, err := store.LoadServerFiles()
	if err != nil {
		return err
	}
	if e.rawKey != nil {
		decryptedRemote := make(map[string]model.FileRecord)
		for path, record := range previousRemote {
			decPath, err := e.enc.DecryptPath(path)
			if err != nil {
				e.Logger.Warn().Err(err).Str("path", path).Msg("failed to decrypt path from state")
				decPath = path
			}
			decryptedRemote[decPath] = record
		}
		previousRemote = decryptedRemote
	}
	currentLocal, err := util.ScanVault(e.Config.VaultPath, e.configDir(), e.Config.IgnoreFolders)
	if err != nil {
		return err
	}
	e.mu.Lock()
	currentRemote := make(map[string]model.FileRecord)
	for path, record := range e.remote {
		if !isValidPath(path) {
			e.Logger.Warn().Str("path", path).Msg("removing invalid path from remote")
			continue
		}
		currentRemote[path] = record
	}
	e.mu.Unlock()
	plan := buildPlan(currentLocal, previousLocal, currentRemote, previousRemote)
	e.Logger.Info().
		Int("planned_actions", len(plan)).
		Int("local_files", len(currentLocal)).
		Int("remote_files", len(currentRemote)).
		Int("previous_local", len(previousLocal)).
		Int("previous_remote", len(previousRemote)).
		Msg("sync plan created")
	for i, action := range plan {
		e.Logger.Debug().Int("action", i).Str("kind", action.Kind).Str("path", action.Path).Msg("action")
		if action.Path == "" {
			e.Logger.Error().Msg("EMPTY PATH IN ACTION!")
		}
	}
	session := newRemoteSession(e.conn, e.remote, e.version, ctx, e.enc, e.Logger, e.rawKey)
	for _, action := range plan {
		switch action.Kind {
		case "download":
			e.mu.Lock()
			record, exists := e.remote[action.Path]
			if !exists {
				record, exists = previousRemote[action.Path]
			}
			e.mu.Unlock()
			e.Logger.Debug().Bool("exists", exists).Str("path", action.Path).Msg("download lookup")
			if !exists {
				continue
			}
			record.Path = action.Path
			if record.Folder {
				if !filepath.IsLocal(filepath.FromSlash(record.Path)) {
					return fmt.Errorf("invalid remote directory path %q", record.Path)
				}
				dirPath, err := util.SafeJoin(e.Config.VaultPath, record.Path)
				if err != nil {
					return err
				}
				if err := os.MkdirAll(dirPath, 0o755); err != nil {
					return err
				}
				e.Logger.Debug().Str("path", action.Path).Msg("ensured remote directory exists locally")
				continue
			}
			content, err := session.pull(record.UID)
			if err != nil {
				return err
			}
			if err := util.WriteFileWithTimes(e.Config.VaultPath, record, content); err != nil {
				return err
			}
			e.Logger.Info().Str("path", action.Path).Msg("downloaded remote file")
		case "upload":
			record := currentLocal[action.Path]
			if !filepath.IsLocal(filepath.FromSlash(action.Path)) {
				return fmt.Errorf("invalid local file path %q", action.Path)
			}
			localPath, err := util.SafeJoin(e.Config.VaultPath, action.Path)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(localPath)
			if err != nil {
				return err
			}
			if err := session.push(record, data); err != nil {
				return err
			}
			e.Logger.Info().Str("path", action.Path).Msg("uploaded local file")
		case "delete-remote":
			if err := session.delete(action.Path); err != nil {
				return err
			}
			e.mu.Lock()
			delete(e.remote, action.Path)
			e.mu.Unlock()
			e.Logger.Info().Str("path", action.Path).Msg("deleted remote file")
		}
	}
	e.mu.Lock()
	e.version = session.version
	e.mu.Unlock()
	if err := store.SetVersion(e.version); err != nil {
		return err
	}

	// Save current local state
	if err := store.ReplaceLocalFiles(currentLocal); err != nil {
		return fmt.Errorf("failed to save local state: %w", err)
	}

	// Save current remote state (encrypt paths for e2ee vaults)
	e.mu.Lock()
	remoteToSave := make(map[string]model.FileRecord, len(e.remote))
	maps.Copy(remoteToSave, e.remote)
	e.mu.Unlock()

	if e.rawKey != nil {
		encryptedRemote := make(map[string]model.FileRecord, len(remoteToSave))
		for path, record := range remoteToSave {
			encPath, err := e.enc.EncryptPath(path)
			if err != nil {
				e.Logger.Warn().Err(err).Str("path", path).Msg("failed to encrypt path for state")
				encPath = path
			}
			record.Path = encPath
			encryptedRemote[encPath] = record
		}
		remoteToSave = encryptedRemote
	}

	if err := store.ReplaceServerFiles(remoteToSave); err != nil {
		return fmt.Errorf("failed to save remote state: %w", err)
	}

	e.Logger.Info().Str("vault", e.Config.VaultID).Msg("sync complete")
	return store.SetInitial(false)
}

func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stopClose != nil {
		e.stopClose()
		e.stopClose = nil
	}
	if e.conn != nil {
		err := e.conn.Close()
		e.conn = nil
		return err
	}
	return nil
}

func (e *Engine) RunContinuous(ctx context.Context) error {
	defer e.Close()

	statePath, err := configpkg.StatePath(e.Config.VaultID, e.Config.StatePath)
	if err != nil {
		return err
	}
	store, err := storage.Open(statePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = store.Close()
	}()
	e.version, _ = store.Version()
	e.initial, _ = store.Initial()

	if err := e.ensureConnected(ctx); err != nil {
		return err
	}

	if e.Config.SyncMode == "pull" || e.Config.SyncMode == "mirror" {
		ticker := time.NewTicker(syncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				if err := e.RunOnce(ctx); err != nil {
					e.Logger.Error().Err(err).Msg("periodic sync failed")
				}
			}
		}
	}
	watcher, err := watchpkg.New(e.Config.VaultPath, append([]string{e.configDir(), ".git"}, e.Config.IgnoreFolders...), e.Logger)
	if err != nil {
		return err
	}
	go watcher.Run(ctx)
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-watcher.Out:
			if !ok {
				return nil
			}
			if err := e.RunOnce(ctx); err != nil {
				e.Logger.Error().Err(err).Msg("watch-triggered sync failed")
			}
		case <-ticker.C:
			if err := e.RunOnce(ctx); err != nil {
				e.Logger.Error().Err(err).Msg("periodic sync failed")
			}
		}
	}
}

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

	previousLocal, previousRemote, err := e.loadState(store)
	if err != nil {
		return err
	}

	currentLocal, err := e.scanLocal()
	if err != nil {
		return err
	}

	e.mu.Lock()
	currentRemote := make(map[string]model.FileRecord)
	maps.Copy(currentRemote, previousRemote)
	for path, record := range e.remote {
		if !isValidPath(path) {
			e.Logger.Warn().Str("path", path).Msg("removing invalid path from remote")
			continue
		}
		currentRemote[path] = record
	}
	version := e.version
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
		e.Logger.Debug().Int("action", i).Str("kind", action.Kind.String()).Str("path", action.Path).Msg("action")
		if action.Path == "" {
			e.Logger.Error().Msg("EMPTY PATH IN ACTION!")
		}
	}

	session := newRemoteSession(e.conn, currentRemote, version, ctx, e.enc, e.Logger, e.rawKey)
	if err := e.executePlan(plan, currentLocal, previousRemote, session); err != nil {
		return err
	}

	e.mu.Lock()
	e.version = session.version
	maps.Copy(e.remote, currentRemote)
	e.mu.Unlock()

	// Rescan local after executing the plan so state reflects downloaded files
	currentLocal, err = e.scanLocal()
	if err != nil {
		return err
	}

	if err := e.saveState(store, currentLocal, currentRemote, session.version); err != nil {
		return err
	}

	e.Logger.Info().Str("vault", e.Config.VaultID).Msg("sync complete")
	return nil
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

// loadState loads previous local and remote state from the state DB.
func (e *Engine) loadState(store *storage.StateStore) (previousLocal, previousRemote map[string]model.FileRecord, err error) {
	previousLocal, err = store.LoadLocalFiles()
	if err != nil {
		return nil, nil, err
	}
	previousRemote, err = store.LoadServerFiles()
	if err != nil {
		return nil, nil, err
	}
	return previousLocal, previousRemote, nil
}

// scanLocal scans the vault for current local files.
func (e *Engine) scanLocal() (map[string]model.FileRecord, error) {
	return util.ScanVault(e.Config.VaultPath, e.configDir(), e.ignoreList())
}

// executePlan executes a list of sync actions.
func (e *Engine) executePlan(plan []syncAction, currentLocal map[string]model.FileRecord, previousRemote map[string]model.FileRecord, session *remoteSession) error {
	for _, action := range plan {
		e.Logger.Debug().Str("kind", action.Kind.String()).Str("path", action.Path).Msg("action")
		if action.Path == "" {
			e.Logger.Error().Msg("EMPTY PATH IN ACTION!")
			continue
		}
		switch action.Kind {
		case syncActionDownload:
			record, exists := session.remote[action.Path]
			if !exists {
				e.Logger.Warn().Str("path", action.Path).Msg("download: remote record not found, skipping")
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
		case syncActionUpload:
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
		case syncActionDeleteRemote:
			if err := session.delete(action.Path); err != nil {
				return err
			}
			delete(session.remote, action.Path)
			e.Logger.Info().Str("path", action.Path).Msg("deleted remote file")
		case syncActionDeleteLocal:
			localPath, err := util.SafeJoin(e.Config.VaultPath, action.Path)
			if err != nil {
				return err
			}
			if err := os.Remove(localPath); err != nil && !os.IsNotExist(err) {
				e.Logger.Warn().Err(err).Str("path", action.Path).Msg("failed to delete local file")
			}
			delete(session.remote, action.Path)
			delete(currentLocal, action.Path)
			e.Logger.Info().Str("path", action.Path).Msg("deleted local file")
		case syncActionMergeText:
			if err := e.mergeTextFile(action.Path, currentLocal, previousRemote, session); err != nil {
				e.Logger.Warn().Err(err).Str("path", action.Path).Msg("merge failed, falling back to download")
				record := session.remote[action.Path]
				record.Path = action.Path
				content, pullErr := session.pull(record.UID)
				if pullErr != nil {
					return pullErr
				}
				if writeErr := util.WriteFileWithTimes(e.Config.VaultPath, record, content); writeErr != nil {
					return writeErr
				}
				e.Logger.Info().Str("path", action.Path).Msg("downloaded remote file (merge fallback)")
			}
		case syncActionMergeJSON:
			if err := e.mergeJSONFile(action.Path, currentLocal, previousRemote, session); err != nil {
				e.Logger.Warn().Err(err).Str("path", action.Path).Msg("JSON merge failed, falling back to download")
				record := session.remote[action.Path]
				record.Path = action.Path
				content, pullErr := session.pull(record.UID)
				if pullErr != nil {
					return pullErr
				}
				if writeErr := util.WriteFileWithTimes(e.Config.VaultPath, record, content); writeErr != nil {
					return writeErr
				}
				e.Logger.Info().Str("path", action.Path).Msg("downloaded remote file (JSON merge fallback)")
			}
		}
	}
	return nil
}

// mergeTextFile performs a three-way merge for a Markdown file.
func (e *Engine) mergeTextFile(path string, currentLocal, previousRemote map[string]model.FileRecord, session *remoteSession) error {
	localPath, err := util.SafeJoin(e.Config.VaultPath, path)
	if err != nil {
		return err
	}
	localContent, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("read local: %w", err)
	}

	baseRecord, hasBase := previousRemote[path]
	var baseContent []byte
	if hasBase && baseRecord.UID > 0 {
		baseContent, err = session.pull(baseRecord.UID)
		if err != nil {
			e.Logger.Warn().Err(err).Str("path", path).Msg("failed to pull base version for merge")
			baseContent = localContent
		}
	} else {
		baseContent = localContent
	}

	remoteRecord := session.remote[path]
	remoteContent, err := session.pull(remoteRecord.UID)
	if err != nil {
		return fmt.Errorf("pull remote: %w", err)
	}

	merged := threeWayMerge(string(baseContent), string(localContent), string(remoteContent))
	mergedBytes := []byte(merged)

	record := model.FileRecord{
		Path:   path,
		Size:   int64(len(mergedBytes)),
		Hash:   util.HashBytes(mergedBytes),
		CTime:  remoteRecord.CTime,
		MTime:  max(remoteRecord.MTime, currentLocal[path].MTime),
		Folder: false,
	}
	if err := util.WriteFileWithTimes(e.Config.VaultPath, record, mergedBytes); err != nil {
		return err
	}

	currentLocal[path] = record
	e.Logger.Info().Str("path", path).Msg("merged text file")
	return nil
}

// mergeJSONFile performs a JSON object-key merge for a config file.
func (e *Engine) mergeJSONFile(path string, currentLocal, previousRemote map[string]model.FileRecord, session *remoteSession) error {
	localPath, err := util.SafeJoin(e.Config.VaultPath, path)
	if err != nil {
		return err
	}
	localContent, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("read local: %w", err)
	}

	remoteRecord := session.remote[path]
	remoteContent, err := session.pull(remoteRecord.UID)
	if err != nil {
		return fmt.Errorf("pull remote: %w", err)
	}

	merged, err := jsonMerge(string(localContent), string(remoteContent))
	if err != nil {
		return err
	}
	mergedBytes := []byte(merged)

	record := model.FileRecord{
		Path:   path,
		Size:   int64(len(mergedBytes)),
		Hash:   util.HashBytes(mergedBytes),
		CTime:  remoteRecord.CTime,
		MTime:  max(remoteRecord.MTime, currentLocal[path].MTime),
		Folder: false,
	}
	if err := util.WriteFileWithTimes(e.Config.VaultPath, record, mergedBytes); err != nil {
		return err
	}

	currentLocal[path] = record
	e.Logger.Info().Str("path", path).Msg("merged JSON config file")
	return nil
}

// saveState saves current local and remote state to the state DB.
func (e *Engine) saveState(store *storage.StateStore, currentLocal map[string]model.FileRecord, currentRemote map[string]model.FileRecord, version int64) error {
	if err := store.SetVersion(version); err != nil {
		return err
	}
	if err := store.ReplaceLocalFiles(currentLocal); err != nil {
		return fmt.Errorf("failed to save local state: %w", err)
	}

	if err := store.ReplaceServerFiles(currentRemote); err != nil {
		return fmt.Errorf("failed to save remote state: %w", err)
	}

	return store.SetInitial(false)
}

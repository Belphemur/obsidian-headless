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
	chunkSize                  = 2 * 1024 * 1024
	maxRemoteFileSize          = 200 * 1024 * 1024
	staleLockAge               = 24 * time.Hour
	syncInterval               = 30 * time.Second
	defaultDownloadConcurrency = 10
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

	initial, _ := store.Initial()
	if initial {
		// During initial sync, ignore previous local and remote state
		// so we download remote files instead of deleting them.
		previousLocal = map[string]model.FileRecord{}
		previousRemote = map[string]model.FileRecord{}
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

	plan := buildPlan(currentLocal, previousLocal, currentRemote, previousRemote, e.configDir())
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
	if err := e.executePlan(ctx, plan, currentLocal, previousRemote, session); err != nil {
		return err
	}

	e.mu.Lock()
	e.version = session.version
	for path := range e.remote {
		if _, ok := session.remote[path]; !ok {
			delete(e.remote, path)
		}
	}
	maps.Copy(e.remote, session.remote)
	e.mu.Unlock()

	// Rescan local after executing the plan so state reflects downloaded files
	currentLocal, err = e.scanLocal()
	if err != nil {
		return err
	}

	updatedRemote := make(map[string]model.FileRecord)
	for path, record := range session.remote {
		if isValidPath(path) {
			updatedRemote[path] = record
		}
	}

	if err := e.saveState(store, currentLocal, updatedRemote, session.version); err != nil {
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
	files, skipped, err := util.ScanVault(e.Config.VaultPath, e.configDir(), e.ignoreList())
	if err != nil {
		return nil, err
	}
	for _, path := range skipped {
		e.Logger.Warn().Str("path", path).Msg("skipping local file with illegal filename characters")
	}
	return files, nil
}

// executePlan executes a list of sync actions.
// Non-download actions run sequentially on the main connection.
// Download actions run in parallel using a worker pool of dedicated connections.
func (e *Engine) executePlan(ctx context.Context, plan []syncAction, currentLocal map[string]model.FileRecord, previousRemote map[string]model.FileRecord, session *remoteSession) error {
	var nonDownloads []syncAction
	var downloads []syncAction
	for _, action := range plan {
		if action.Path == "" {
			e.Logger.Error().Msg("EMPTY PATH IN ACTION!")
			continue
		}
		if action.Kind == syncActionDownload {
			downloads = append(downloads, action)
		} else {
			nonDownloads = append(nonDownloads, action)
		}
	}

	for _, action := range nonDownloads {
		if err := ctx.Err(); err != nil {
			e.Logger.Info().Err(err).Msg("sync cancelled, stopping plan execution")
			return err
		}
		e.Logger.Debug().Str("kind", action.Kind.String()).Str("path", action.Path).Msg("action")
		switch action.Kind {
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
		default:
		}
	}

	// Separate folder downloads (must happen before parallel file downloads)
	// and collect file downloads for the worker pool.
	var fileDownloads []syncAction
	for _, action := range downloads {
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
		fileDownloads = append(fileDownloads, action)
	}

	if len(fileDownloads) == 0 {
		return nil
	}

	return e.executeDownloadsParallel(ctx, fileDownloads, session)
}

type downloadJob struct {
	path   string
	record model.FileRecord
}

// executeDownloadsParallel processes download actions using a pool of worker
// goroutines, each with its own WebSocket connection. The number of workers
// is min(len(actions), configured download concurrency), so small syncs
// don't create idle connections.
func (e *Engine) executeDownloadsParallel(ctx context.Context, actions []syncAction, session *remoteSession) error {
	concurrency := e.Config.DownloadConcurrency
	if concurrency <= 0 {
		concurrency = defaultDownloadConcurrency
	}
	if concurrency > len(actions) {
		concurrency = len(actions)
	}

	jobs := make(chan downloadJob, len(actions))
	errs := make(chan error, 1)
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			conn, err := e.dialWorker(ctx)
			if err != nil {
				e.Logger.Warn().Err(err).Msg("worker dial failed, draining jobs")
				for range jobs {
				}
				return
			}
			defer conn.Close()

			workerSession := newRemoteSession(conn, session.remote, session.version, ctx, e.enc, e.Logger, e.rawKey)

			for job := range jobs {
				select {
				case <-ctx.Done():
					for range jobs {
					}
					return
				default:
				}

				content, err := workerSession.pull(job.record.UID)
				if err != nil {
					select {
					case errs <- fmt.Errorf("pull %q: %w", job.path, err):
					default:
					}
					for range jobs {
					}
					return
				}

				if err := util.WriteFileWithTimes(e.Config.VaultPath, job.record, content); err != nil {
					select {
					case errs <- fmt.Errorf("write %q: %w", job.path, err):
					default:
					}
					for range jobs {
					}
					return
				}

				e.Logger.Debug().Str("path", job.path).Msg("downloaded remote file")
			}
		}()
	}

	for _, action := range actions {
		record, exists := session.remote[action.Path]
		if !exists {
			e.Logger.Warn().Str("path", action.Path).Msg("download: remote record not found, skipping")
			continue
		}
		record.Path = action.Path

		select {
		case jobs <- downloadJob{path: action.Path, record: record}:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		}
	}
	close(jobs)

	wg.Wait()

	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
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

	localRecord, hasLocal := currentLocal[path]
	if !hasLocal {
		return fmt.Errorf("local record missing for merge of %q", path)
	}

	baseRecord, hasBase := previousRemote[path]
	var baseContent []byte
	if hasBase && baseRecord.UID > 0 {
		baseContent, err = session.pull(baseRecord.UID)
		if err != nil {
			return fmt.Errorf("pull base for three-way merge: %w", err)
		}
	} else {
		return fmt.Errorf("no base version available for three-way merge of %q", path)
	}

	remoteRecord := session.remote[path]
	remoteContent, err := session.pull(remoteRecord.UID)
	if err != nil {
		return fmt.Errorf("pull remote: %w", err)
	}

	merged, err := threeWayMerge(string(baseContent), string(localContent), string(remoteContent))
	if err != nil {
		return fmt.Errorf("three-way merge failed: %w", err)
	}
	mergedBytes := []byte(merged)

	record := model.FileRecord{
		Path:   path,
		Size:   int64(len(mergedBytes)),
		Hash:   util.HashBytes(mergedBytes),
		CTime:  remoteRecord.CTime,
		MTime:  max(remoteRecord.MTime, localRecord.MTime),
		Folder: false,
	}
	if err := util.WriteFileWithTimes(e.Config.VaultPath, record, mergedBytes); err != nil {
		return err
	}

	if err := session.push(record, mergedBytes); err != nil {
		return fmt.Errorf("push merged text file: %w", err)
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

	localRecord, hasLocal := currentLocal[path]
	if !hasLocal {
		return fmt.Errorf("local record missing for merge of %q", path)
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
		MTime:  max(remoteRecord.MTime, localRecord.MTime),
		Folder: false,
	}
	if err := util.WriteFileWithTimes(e.Config.VaultPath, record, mergedBytes); err != nil {
		return err
	}

	if err := session.push(record, mergedBytes); err != nil {
		return fmt.Errorf("push merged JSON config file: %w", err)
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

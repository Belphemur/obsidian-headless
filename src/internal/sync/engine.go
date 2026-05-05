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
	"github.com/sony/gobreaker/v2"

	"github.com/Belphemur/obsidian-headless/internal/circuitbreaker"
	configpkg "github.com/Belphemur/obsidian-headless/internal/config"
	"github.com/Belphemur/obsidian-headless/internal/encryption"
	"github.com/Belphemur/obsidian-headless/internal/model"
	"github.com/Belphemur/obsidian-headless/internal/storage"
	"github.com/Belphemur/obsidian-headless/internal/util"
)

const (
	chunkSize                  = 2 * 1024 * 1024
	maxRemoteFileSize          = 200 * 1024 * 1024
	staleLockAge               = 24 * time.Hour
	syncInterval               = 30 * time.Second
	defaultDownloadConcurrency = 5
)

type Engine struct {
	Config model.SyncConfig
	Token  string
	Logger zerolog.Logger

	enc    encryption.EncryptionProvider
	rawKey []byte

	conn   *websocket.Conn
	remote map[string]model.FileRecord
	// version stores the connection version negotiated by ensureConnected in
	// RunOnce mode. Continuous mode uses continuousState.version instead.
	// It is read only by RunOnce/ensureConnected; all other paths receive
	// version as an explicit parameter (see runSyncCycle).
	version   int64
	stopClose func() bool
	wsCB      *gobreaker.CircuitBreaker[struct{}]

	// test-only overrides for heartbeat timing (zero = use defaults)
	testHeartbeatInterval      time.Duration
	testHeartbeatSendThreshold time.Duration
	testHeartbeatTimeout       time.Duration

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
	e.wsCB = gobreaker.NewCircuitBreaker[struct{}](circuitbreaker.SyncWS(config.VaultID, logger))

	return e, nil
}

func (e *Engine) executeWithBreaker(fn func() (struct{}, error)) (struct{}, error) {
	if e.wsCB == nil {
		return fn()
	}
	return e.wsCB.Execute(fn)
}

func (e *Engine) RunOnce(ctx context.Context) error {
	e.Logger.Info().Str("vault", e.Config.VaultID).Msg("sync start")

	e.mu.Lock()
	if e.conn == nil {
		e.mu.Unlock()
		version, err := e.ensureConnected(ctx)
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		e.mu.Lock()
		e.version = version
	}
	version := e.version
	e.mu.Unlock()

	lock, err := e.acquireLock()
	if err != nil {
		e.mu.Lock()
		if e.stopClose != nil {
			e.stopClose()
			e.stopClose = nil
		}
		if e.conn != nil {
			_ = e.conn.Close()
			e.conn = nil
		}
		e.mu.Unlock()
		return err
	}
	defer lock()

	// Wrap e.conn shutdown: after RunOnce returns we want to close the
	// connection since it's not reused across runs. ensureConnected set
	// stopClose already; this defers the actual close for cleanup.
	defer func() {
		e.mu.Lock()
		if e.stopClose != nil {
			e.stopClose()
			e.stopClose = nil
		}
		if e.conn != nil {
			_ = e.conn.Close()
			e.conn = nil
		}
		e.mu.Unlock()
	}()

	statePath, err := configpkg.StatePath(e.Config.VaultID, e.Config.StatePath)
	if err != nil {
		return err
	}
	store, err := storage.Open(statePath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	previousLocal, previousRemote, err := e.loadState(store)
	if err != nil {
		return err
	}

	// Snapshot current Engine state for the cycle. The snapshot is
	// read-only inside runSyncCycle; the caller updates Engine fields
	// under the lock after it returns.
	e.mu.Lock()
	remoteSnapshot := make(map[string]model.FileRecord)
	maps.Copy(remoteSnapshot, e.remote)
	conn := e.conn
	e.mu.Unlock()

	version, updatedRemote, err := e.runSyncCycle(ctx, store, conn, version, remoteSnapshot, previousLocal, previousRemote, nil, nil, "once")
	if err != nil {
		return err
	}

	e.mu.Lock()
	e.version = version
	clear(e.remote)
	maps.Copy(e.remote, updatedRemote)
	e.mu.Unlock()

	e.Logger.Info().Str("vault", e.Config.VaultID).Msg("sync complete")
	return nil
}

// logRemoteRenameConflicts logs any paths that were preserved as conflicts
// during remote rename detection (locally modified files, destination collisions,
// missing previous state, filesystem errors, etc.).
func (e *Engine) logRemoteRenameConflicts(result *RemoteRenameResult, mode string) {
	for _, path := range result.Conflicts {
		e.Logger.Warn().
			Str("path", path).
			Str("mode", mode).
			Msg("remote rename conflict, preserving original path(s)")
	}
}

// runSyncCycle executes the core sync cycle shared by RunOnce and RunContinuous.
// It scans local files, merges remote state, detects renames, builds and executes
// a sync plan, then saves the resulting state.
//
// Parameters:
//   - conn: the WebSocket connection used for plan execution (may be nil if no
//     execution is needed; the function handles nil gracefully)
//   - version: the current negotiated version for this cycle
//   - remoteSnapshot: a read-only snapshot of the live remote map (e.remote or cs.remote)
//   - previousLocal/previousRemote: state loaded from the DB (may be pre-mutated by
//     the caller, e.g. local rename fixups in continuous mode)
//   - localRenames: optional map of oldPath→newPath for local renames (nil for RunOnce)
//   - onBeforeRename: optional callback called before filesystem rename (nil for RunOnce)
//   - mode: log label ("once" or "continuous")
//
// Returns the updated version (from session.version) and the filtered remote map
// suitable for saveState. Both callers then merge these back into their own
// version/remote tracking under their own locks.
func (e *Engine) runSyncCycle(
	ctx context.Context,
	store *storage.StateStore,
	conn *websocket.Conn,
	version int64,
	remoteSnapshot map[string]model.FileRecord,
	previousLocal, previousRemote map[string]model.FileRecord,
	localRenames map[string]string,
	onBeforeRename func(model.RenamePair),
	mode string,
) (int64, map[string]model.FileRecord, error) {
	// Clone previous state as the save-state baseline before any mutations.
	// applyRemoteRenameFixups mutates previousRemote in-place, so dbLocal/dbRemote
	// must be independent copies — otherwise the baseline used by saveState to
	// compute deletions would be corrupted.
	dbLocal := make(map[string]model.FileRecord)
	maps.Copy(dbLocal, previousLocal)
	dbRemote := make(map[string]model.FileRecord)
	maps.Copy(dbRemote, previousRemote)

	initial, _ := store.Initial()
	if initial {
		// During initial sync, ignore previous local and remote state
		// so we download remote files instead of deleting them.
		previousLocal = map[string]model.FileRecord{}
		previousRemote = map[string]model.FileRecord{}
	}

	currentLocal, err := e.scanLocal()
	if err != nil {
		return 0, nil, err
	}

	currentRemote := make(map[string]model.FileRecord)
	maps.Copy(currentRemote, previousRemote)
	for path, record := range remoteSnapshot {
		if !isValidPath(path) {
			e.Logger.Warn().Str("path", path).Msg("removing invalid path from remote")
			continue
		}
		currentRemote[path] = record
	}

	// Detect and apply remote renames before building the plan.
	remoteRenameResult := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, e.Config.VaultPath, e.Logger, onBeforeRename)
	e.logRemoteRenameConflicts(remoteRenameResult, mode)

	plan := buildPlan(currentLocal, previousLocal, currentRemote, previousRemote, e.configDir(), localRenames)
	e.Logger.Info().
		Int("planned_actions", len(plan)).
		Int("local_files", len(currentLocal)).
		Int("remote_files", len(currentRemote)).
		Int("previous_local", len(previousLocal)).
		Int("previous_remote", len(previousRemote)).
		Msg("sync plan created")
	logPlanActions(e.Logger, plan)

	for i, action := range plan {
		e.Logger.Debug().Int("action", i).Str("kind", action.Kind.String()).Str("path", action.Path).Msg("action")
		if action.Path == "" {
			e.Logger.Error().Msg("EMPTY PATH IN ACTION!")
		}
	}

	session := newRemoteSession(conn, currentRemote, version, ctx, e.enc, e.Logger, e.rawKey)
	if err := e.executePlan(ctx, plan, currentLocal, previousRemote, previousLocal, session, version); err != nil {
		return 0, nil, err
	}

	// Rescan local after executing the plan so state reflects downloaded files
	currentLocal, err = e.scanLocal()
	if err != nil {
		return 0, nil, err
	}

	updatedRemote := make(map[string]model.FileRecord)
	for path, record := range session.remote {
		if isValidPath(path) {
			updatedRemote[path] = record
		}
	}

	version = session.version
	if err := e.saveState(store, currentLocal, updatedRemote, dbLocal, dbRemote, version); err != nil {
		return 0, nil, err
	}

	return version, updatedRemote, nil
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

// logPlanActions counts and logs a breakdown of sync actions.
func logPlanActions(logger zerolog.Logger, plan []syncAction) {
	uploadCount, deleteRemoteCount, deleteLocalCount, mergeCount, downloadCount := 0, 0, 0, 0, 0
	for _, action := range plan {
		switch action.Kind {
		case syncActionUpload:
			uploadCount++
		case syncActionDeleteRemote:
			deleteRemoteCount++
		case syncActionDeleteLocal:
			deleteLocalCount++
		case syncActionMergeText, syncActionMergeJSON:
			mergeCount++
		case syncActionDownload:
			downloadCount++
		default:
			logger.Warn().
				Str("kind", action.Kind.String()).
				Msg("logPlanActions: unhandled action kind, may indicate a missing case")
		}
	}
	logger.Info().
		Int("uploads", uploadCount).
		Int("deleteRemote", deleteRemoteCount).
		Int("deleteLocal", deleteLocalCount).
		Int("merges", mergeCount).
		Int("downloads", downloadCount).
		Msg("sync plan")
}

// executePlan executes a list of sync actions.
// Non-download actions run sequentially on the main connection.
// Download actions run in parallel using a worker pool of dedicated connections.
func (e *Engine) executePlan(ctx context.Context, plan []syncAction, currentLocal map[string]model.FileRecord, previousRemote, previousLocal map[string]model.FileRecord, session *remoteSession, version int64) error {
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

	logPlanActions(e.Logger, plan)

	for _, action := range nonDownloads {
		if err := ctx.Err(); err != nil {
			e.Logger.Info().Err(err).Msg("sync cancelled, stopping plan execution")
			return err
		}
		e.Logger.Debug().Str("kind", action.Kind.String()).Str("path", action.Path).Msg("action")
		switch action.Kind {
		case syncActionUpload:
			record := currentLocal[action.Path]
			// If this action carries a RelatedPath (rename), propagate it to the push
			// so session.push() includes the relatedpath field in the WebSocket message.
			if action.RelatedPath != "" {
				record.PreviousPath = action.RelatedPath
			}
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
			// Clear PreviousPath as a safety net so the record doesn't
			// carry rename state into later processing within this cycle.
			// currentLocal is re-scanned from disk after executePlan returns
			// (RunOnce and RunContinuous both call scanLocal() post-execution),
			// which creates fresh FileRecords without PreviousPath. The real
			// prevention of re-sending relatedpath comes from that re-scan;
			// this in-memory clear is a belt-and-suspenders measure.
			record.PreviousPath = ""
			currentLocal[action.Path] = record
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

	// fileDownloads are already filtered for folders and missing records in executePlan.
	// Build downloadJob objects here to avoid a second remote lookup in executeDownloadsParallel.
	var downloadJobs []downloadJob
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
		downloadJobs = append(downloadJobs, downloadJob{path: action.Path, record: record})
	}

	if len(downloadJobs) == 0 {
		return nil
	}

	return e.executeDownloadsParallel(ctx, downloadJobs, session, version)
}

type downloadJob struct {
	path   string
	record model.FileRecord
}

// executeDownloadsParallel processes download jobs using a pool of worker
// goroutines, each with its own WebSocket connection. The number of workers
// is min(len(jobs), configured download concurrency), so small syncs
// don't create idle connections.
func (e *Engine) executeDownloadsParallel(ctx context.Context, jobs []downloadJob, session *remoteSession, version int64) error {
	concurrency := e.Config.DownloadConcurrency
	if concurrency <= 0 {
		concurrency = defaultDownloadConcurrency
	}
	if concurrency > len(jobs) {
		concurrency = len(jobs)
	}

	jobsCh := make(chan downloadJob, len(jobs))
	var wg sync.WaitGroup

	varmu := struct {
		mu      sync.Mutex
		done    int
		failed  int
		errMsgs []string
	}{}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		workerID := i + 1
		go func() {
			defer wg.Done()

			conn, err := e.dialWorker(ctx, version)
			if err != nil {
				e.Logger.Warn().Int("workerID", workerID).Err(err).Msg("worker dial failed")
				varmu.mu.Lock()
				varmu.failed++
				varmu.errMsgs = append(varmu.errMsgs, fmt.Sprintf("worker %d dial: %v", workerID, err))
				varmu.mu.Unlock()
				return
			}
			stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
			defer stop()
			defer conn.Close()

			workerSession := newRemoteSession(conn, session.remote, session.version, ctx, e.enc, e.Logger, e.rawKey)

			for job := range jobsCh {
				content, err := workerSession.pull(job.record.UID)
				if err != nil {
					e.Logger.Error().Int("workerID", workerID).Str("path", job.path).Err(err).Msg("worker pull failed")
					varmu.mu.Lock()
					varmu.failed++
					varmu.errMsgs = append(varmu.errMsgs, fmt.Sprintf("worker %d pull %q: %v", workerID, job.path, err))
					varmu.mu.Unlock()
					return
				}

				if err := util.WriteFileWithTimes(e.Config.VaultPath, job.record, content); err != nil {
					e.Logger.Error().Int("workerID", workerID).Str("path", job.path).Err(err).Msg("worker write failed")
					varmu.mu.Lock()
					varmu.failed++
					varmu.errMsgs = append(varmu.errMsgs, fmt.Sprintf("worker %d write %q: %v", workerID, job.path, err))
					varmu.mu.Unlock()
					return
				}

				done := func() int {
					varmu.mu.Lock()
					varmu.done++
					n := varmu.done
					varmu.mu.Unlock()
					return n
				}()
				e.Logger.Info().Int("workerID", workerID).Str("path", job.path).Int("done", done).Msg("downloaded file")
			}
		}()
	}

	for _, job := range jobs {
		select {
		case jobsCh <- job:
		case <-ctx.Done():
			close(jobsCh)
			wg.Wait()
			return ctx.Err()
		}
	}
	close(jobsCh)

	wg.Wait()

	varmu.mu.Lock()
	done := varmu.done
	failed := varmu.failed
	errMsgs := varmu.errMsgs
	varmu.mu.Unlock()

	e.Logger.Info().
		Int("completed_jobs", done).
		Int("worker_failures", failed).
		Int("total_jobs", len(jobs)).
		Msg("parallel download complete")

	if len(errMsgs) > 0 && done == 0 {
		return fmt.Errorf("all download workers failed: %s", errMsgs[0])
	}
	if len(errMsgs) > 0 {
		e.Logger.Warn().
			Int("completed_jobs", done).
			Int("worker_failures", failed).
			Int("total_jobs", len(jobs)).
			Msg("partial download failure, continuing")
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

// saveState saves current local and remote state to the state DB in a single
// atomic transaction. Computes local and remote diffs in parallel, then
// applies only changed records.
func (e *Engine) saveState(store *storage.StateStore, currentLocal, currentRemote map[string]model.FileRecord, previousLocal, previousRemote map[string]model.FileRecord, version int64) error {
	var localUpserts, remoteUpserts []model.FileRecord
	var localDeletes, remoteDeletes []string
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		localUpserts, localDeletes = diffRecords(currentLocal, previousLocal)
	}()
	go func() {
		defer wg.Done()
		remoteUpserts, remoteDeletes = diffRecords(currentRemote, previousRemote)
	}()
	wg.Wait()

	return store.SaveStateAtomic(version, false, localUpserts, localDeletes, remoteUpserts, remoteDeletes)
}

// diffRecords compares current against previous and returns records to upsert
// and paths to delete.
func diffRecords(current, previous map[string]model.FileRecord) (upserts []model.FileRecord, deletes []string) {
	upserts = make([]model.FileRecord, 0, len(current))
	for path, rec := range current {
		prev, had := previous[path]
		if !had || !rec.Equal(prev) {
			upserts = append(upserts, rec)
		}
	}
	for path := range previous {
		if _, has := current[path]; !has {
			deletes = append(deletes, path)
		}
	}
	return upserts, deletes
}

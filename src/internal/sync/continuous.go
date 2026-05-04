package sync

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/sony/gobreaker/v2"

	"github.com/Belphemur/obsidian-headless/internal/circuitbreaker"
	configpkg "github.com/Belphemur/obsidian-headless/internal/config"
	"github.com/Belphemur/obsidian-headless/internal/model"
	"github.com/Belphemur/obsidian-headless/internal/storage"
	watchpkg "github.com/Belphemur/obsidian-headless/internal/sync/watch"
)

const (
	continuousDebounce     = 500 * time.Millisecond
	reconnectBackoff       = 5 * time.Second
	maxReconnectBackoff    = 60 * time.Second
	heartbeatInterval      = 20 * time.Second
	heartbeatSendThreshold = 10 * time.Second
	heartbeatTimeout       = 120 * time.Second
)

// applyRenameFixups mutates previousLocal and previousRemote in-place to reflect
// file renames detected by the watcher. Without this, a rename from oldPath→newPath
// would appear as a delete of oldPath + create of newPath in buildPlan.
// The record's PreviousPath field is set to preserve the rename chain.
func applyRenameFixups(previousLocal, previousRemote map[string]model.FileRecord, renames []watchpkg.ScanEvent, logger zerolog.Logger) {
	for _, ev := range renames {
		if ev.Type != watchpkg.EventRename {
			continue
		}
		if oldLocal, ok := previousLocal[ev.OldPath]; ok {
			oldLocal.PreviousPath = ev.OldPath
			oldLocal.Path = ev.Path
			previousLocal[ev.Path] = oldLocal
			delete(previousLocal, ev.OldPath)
			logger.Info().
				Str("oldPath", ev.OldPath).
				Str("newPath", ev.Path).
				Msg("continuous: local rename applied")
		}
		if oldRemote, ok := previousRemote[ev.OldPath]; ok {
			oldRemote.PreviousPath = ev.OldPath
			oldRemote.Path = ev.Path
			previousRemote[ev.Path] = oldRemote
			delete(previousRemote, ev.OldPath)
			logger.Info().
				Str("oldPath", ev.OldPath).
				Str("newPath", ev.Path).
				Msg("continuous: remote rename applied")
		}
	}
}

type continuousState struct {
	mu             sync.Mutex
	syncInProgress atomic.Bool
	syncPending    atomic.Bool
	conn           *websocket.Conn
	remote         map[string]model.FileRecord
	version        int64
	stopClose      func() bool
	lastMessageTs  time.Time
}

func (e *Engine) RunContinuous(ctx context.Context) error {
	statePath, err := configpkg.StatePath(e.Config.VaultID, e.Config.StatePath)
	if err != nil {
		return err
	}
	store, err := storage.Open(statePath)
	if err != nil {
		return err
	}
	version, _ := store.Version()
	_ = store.Close()

	var rescanInterval time.Duration
	if e.Config.SyncMode == "" || e.Config.SyncMode == "bidirectional" {
		periodic := e.Config.PeriodicScan
		if periodic == "" {
			periodic = "1h"
		}
		rescanInterval, err = time.ParseDuration(periodic)
		if err != nil {
			return fmt.Errorf("invalid periodic-scan duration %q: %w", periodic, err)
		}
	}

	cs := &continuousState{
		remote:  make(map[string]model.FileRecord),
		version: version,
	}

	defer func() {
		cs.mu.Lock()
		if cs.stopClose != nil {
			cs.stopClose()
			cs.stopClose = nil
		}
		if cs.conn != nil {
			_ = cs.conn.Close()
			cs.conn = nil
		}
		cs.mu.Unlock()
	}()

	connect := func() error {
		cs.mu.Lock()
		defer cs.mu.Unlock()

		if cs.stopClose != nil {
			cs.stopClose()
			cs.stopClose = nil
		}
		if cs.conn != nil {
			_ = cs.conn.Close()
			cs.conn = nil
		}
		clear(cs.remote)

		store, err := storage.Open(statePath)
		if err != nil {
			return fmt.Errorf("failed to open state db: %w", err)
		}
		initial, _ := store.Initial()
		_ = store.Close()

		_, cbErr := e.executeWithBreaker(func() (struct{}, error) {
			conn, _, err := websocket.DefaultDialer.DialContext(ctx, normalizeWSURL(e.Config.Host), nil)
			if err != nil {
				return struct{}{}, fmt.Errorf("failed to dial websocket: %w", err)
			}

			stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })

			newVersion, remote, err := e.handshake(ctx, conn, cs.version, initial)
			if err != nil {
				stopClose()
				_ = conn.Close()
				return struct{}{}, err
			}

			cs.conn = conn
			cs.stopClose = stopClose
			cs.version = newVersion
			cs.remote = remote
			cs.lastMessageTs = time.Now()
			return struct{}{}, nil
		})

		if cbErr != nil {
			if errors.Is(cbErr, gobreaker.ErrOpenState) || errors.Is(cbErr, gobreaker.ErrTooManyRequests) {
				return &circuitbreaker.BreakerError{
					Message: fmt.Sprintf("Vault %s sync is temporarily unavailable (circuit open); retry in ~60s", e.Config.VaultID),
					Err:     cbErr,
				}
			}
			return cbErr
		}
		return nil
	}

	if err := connect(); err != nil {
		return err
	}

	trigger := make(chan struct{}, 1)
	var readPumpDone chan struct{}

	startReadPump := func() {
		done := make(chan struct{})
		readPumpDone = done
		go func() {
			defer close(done)
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				cs.mu.Lock()
				conn := cs.conn
				remote := cs.remote
				ver := cs.version
				cs.mu.Unlock()

				if conn == nil {
					return
				}

				_, data, err := conn.ReadMessage()
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						e.Logger.Debug().Err(err).Msg("continuous: websocket closed normally")
					} else {
						e.Logger.Error().Err(err).Msg("continuous: websocket read error")
					}
					return
				}

				cs.mu.Lock()
				cs.lastMessageTs = time.Now()
				cs.mu.Unlock()

				msg, err := decodeJSONMessage(data)
				if err != nil {
					e.Logger.Warn().Err(err).Msg("continuous: failed to decode message")
					continue
				}

				op, _ := msg["op"].(string)
				switch op {
				case "push":
					session := newRemoteSession(conn, remote, ver, ctx, e.enc, e.Logger, e.rawKey)
					record := session.parseRemoteRecord(msg)
					if record.Path != "" {
						cs.mu.Lock()
						cs.remote[record.Path] = record
						cs.mu.Unlock()
						e.Logger.Debug().Str("path", record.Path).Msg("continuous: received push")
					}

					select {
					case trigger <- struct{}{}:
					default:
					}
				case "ready":
					cs.mu.Lock()
					cs.version = int64Value(msg["version"])
					cs.mu.Unlock()

					e.Logger.Debug().Int64("version", cs.version).Msg("continuous: received ready")
				case "pong":
					// Ignore pong responses
				}
			}
		}()
	}

	startHeartbeat := func() {
		go func() {
			ticker := time.NewTicker(heartbeatInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					cs.mu.Lock()
					conn := cs.conn
					lastMsg := cs.lastMessageTs
					cs.mu.Unlock()

					if conn == nil {
						continue
					}

					elapsed := time.Since(lastMsg)
					e.Logger.Debug().Dur("elapsed", elapsed).Msg("continuous: heartbeat check")
					if elapsed >= heartbeatTimeout {
						e.Logger.Warn().Dur("elapsed", elapsed).Msg("continuous: heartbeat timeout, closing connection")
						_ = conn.Close()
						cs.mu.Lock()
						if cs.conn == conn {
							cs.conn = nil
						}
						cs.mu.Unlock()
						continue
					}
					if elapsed >= heartbeatSendThreshold {
						if err := conn.WriteJSON(map[string]any{"op": "ping"}); err != nil {
							e.Logger.Warn().Err(err).Msg("continuous: failed to send ping, closing connection")
							_ = conn.Close()
							cs.mu.Lock()
							if cs.conn == conn {
								cs.conn = nil
							}
							cs.mu.Unlock()
							continue
						}
						e.Logger.Debug().Dur("elapsed", elapsed).Msg("continuous: sent ping")
					}
				}
			}
		}()
	}

	startReadPump()
	startHeartbeat()

	watcher, err := watchpkg.New(e.Config.VaultPath, append([]string{".git"}, e.ignoreList()...), e.Logger, rescanInterval)
	if err != nil {
		return err
	}
	go watcher.Run(ctx)

	var debounceTimer *time.Timer
	var debounceMu sync.Mutex

	// Rename event collection for inode-based rename detection
	var pendingRenames []watchpkg.ScanEvent
	var renamesMu sync.Mutex

	var doSync func()

	scheduleSync := func() {
		debounceMu.Lock()
		defer debounceMu.Unlock()
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(continuousDebounce, func() {
			doSync()
		})
	}

	doSync = func() {
		if cs.syncInProgress.Swap(true) {
			e.Logger.Debug().Msg("continuous: sync already in progress, skipping")
			cs.syncPending.Store(true)
			return
		}
		defer cs.syncInProgress.Store(false)
		// If a sync request arrived while this one was running, re-trigger after completion.
		// Placed in a defer so it runs on ALL exit paths (plan-empty, errors, success).
		defer func() {
			if cs.syncPending.Swap(false) {
				e.Logger.Debug().Msg("continuous: re-triggering sync for pending changes")
				scheduleSync()
			}
		}()

		lock, err := e.acquireLock()
		if err != nil {
			e.Logger.Error().Err(err).Msg("continuous: failed to acquire lock")
			return
		}
		defer lock()

		store, err := storage.Open(statePath)
		if err != nil {
			e.Logger.Error().Err(err).Msg("continuous: failed to open state DB")
			return
		}
		defer func() { _ = store.Close() }()

		previousLocal, previousRemote, err := e.loadState(store)
		if err != nil {
			e.Logger.Error().Err(err).Msg("continuous: failed to load state")
			return
		}

		renamesMu.Lock()
		snapshot := make([]watchpkg.ScanEvent, len(pendingRenames))
		copy(snapshot, pendingRenames)
		pendingRenames = pendingRenames[:0]
		renamesMu.Unlock()

		applyRenameFixups(previousLocal, previousRemote, snapshot, e.Logger)

		// Flush stale ignorations from the previous sync cycle
		watcher.FlushIgnored()

		// Clone previous state as the save-state baseline before any mutations.
		// applyRemoteRenameFixups below mutates previousRemote in-place, so
		// dbLocal/dbRemote must be independent copies — otherwise the baseline
		// used by saveState to compute deletions would be corrupted.
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
			e.Logger.Error().Err(err).Msg("continuous: failed to scan local")
			return
		}

		cs.mu.Lock()
		currentRemote := make(map[string]model.FileRecord)
		maps.Copy(currentRemote, previousRemote)
		for path, record := range cs.remote {
			if !isValidPath(path) {
				continue
			}
			currentRemote[path] = record
		}
		version := cs.version
		cs.mu.Unlock()

		// Detect and apply remote renames before building the plan.
		// Pass a callback to register watcher ignore paths before os.Rename
		// so fsnotify events for our own rename are suppressed.
		remoteRenameResult := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, e.Config.VaultPath, e.Logger,
			func(pair model.RenamePair) {
				watcher.AddIgnorePaths([]model.RenamePair{pair})
			})
		e.logRemoteRenameConflicts(remoteRenameResult, "continuous")

		plan := buildPlan(currentLocal, previousLocal, currentRemote, previousRemote, e.configDir())
		e.Logger.Info().Int("planned_actions", len(plan)).Msg("continuous: sync plan created")

		if len(plan) == 0 {
			return
		}

		// Open a dedicated connection for plan execution
		var connB *websocket.Conn
		_, cbErr := e.executeWithBreaker(func() (struct{}, error) {
			var err error
			connB, _, err = websocket.DefaultDialer.DialContext(ctx, normalizeWSURL(e.Config.Host), nil)
			if err != nil {
				return struct{}{}, fmt.Errorf("continuous: failed to dial execution connection: %w", err)
			}
			return struct{}{}, nil
		})
		if cbErr != nil {
			e.Logger.Error().Err(cbErr).Msg("continuous: execution connection failed")
			return
		}
		stopCloseB := context.AfterFunc(ctx, func() { _ = connB.Close() })
		defer func() {
			stopCloseB()
			_ = connB.Close()
		}()

		execVersion, _, err := e.handshake(ctx, connB, version, false)
		if err != nil {
			e.Logger.Error().Err(err).Msg("continuous: failed to handshake execution connection")
			return
		}

		session := newRemoteSession(connB, currentRemote, execVersion, ctx, e.enc, e.Logger, e.rawKey)
		if err := e.executePlan(ctx, plan, currentLocal, previousRemote, session); err != nil {
			e.Logger.Error().Err(err).Msg("continuous: failed to execute plan")
			return
		}

		// Rescan local after executing the plan so state reflects downloaded files
		currentLocal, err = e.scanLocal()
		if err != nil {
			e.Logger.Error().Err(err).Msg("continuous: failed to rescan local after sync")
			return
		}

		cs.mu.Lock()
		// Merge changes made by executePlan (uploads/deletions) back into cs.remote
		// so that saveState captures the true post-sync remote state.
		for path := range currentRemote {
			if _, ok := session.remote[path]; !ok {
				delete(cs.remote, path)
			}
		}
		maps.Copy(cs.remote, session.remote)
		versionForSave := cs.version
		remoteForSave := make(map[string]model.FileRecord)
		for path, record := range cs.remote {
			if isValidPath(path) {
				remoteForSave[path] = record
			}
		}
		cs.mu.Unlock()

		if err := e.saveState(store, currentLocal, remoteForSave, dbLocal, dbRemote, versionForSave); err != nil {
			e.Logger.Error().Err(err).Msg("continuous: failed to save state")
			return
		}

		e.Logger.Info().Msg("continuous: sync complete")
	}

	// Always run a full sync on startup to catch local changes made
	// while the client was stopped.
	scheduleSync()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-trigger:
			scheduleSync()
		case ev, ok := <-watcher.Out:
			if !ok {
				return nil
			}
			if ev.Type == watchpkg.EventRename {
				renamesMu.Lock()
				pendingRenames = append(pendingRenames, ev)
				renamesMu.Unlock()
			}
			scheduleSync()
		case <-readPumpDone:
			e.Logger.Info().Msg("continuous: readPump exited, reconnecting...")
			cs.mu.Lock()
			if cs.stopClose != nil {
				cs.stopClose()
				cs.stopClose = nil
			}
			if cs.conn != nil {
				_ = cs.conn.Close()
				cs.conn = nil
			}
			clear(cs.remote)
			cs.mu.Unlock()

			backoff := reconnectBackoff

		reconnectLoop:
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(backoff):
					if err := connect(); err != nil {
						e.Logger.Error().Err(err).Msg("continuous: reconnection failed, retrying...")
						backoff *= 2
						if backoff > maxReconnectBackoff {
							backoff = maxReconnectBackoff
						}
						continue reconnectLoop
					}
					startReadPump()
					select {
					case trigger <- struct{}{}:
					default:
					}
					break reconnectLoop
				}
			}
		}
	}
}

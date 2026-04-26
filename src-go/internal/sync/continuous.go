package sync

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	configpkg "github.com/Belphemur/obsidian-headless/src-go/internal/config"
	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
	"github.com/Belphemur/obsidian-headless/src-go/internal/storage"
	watchpkg "github.com/Belphemur/obsidian-headless/src-go/internal/sync/watch"
)

const (
	continuousDebounce      = 500 * time.Millisecond
	reconnectBackoff        = 5 * time.Second
	heartbeatInterval       = 20 * time.Second
	heartbeatSendThreshold  = 10 * time.Second
	heartbeatTimeout        = 120 * time.Second
)

type continuousState struct {
	mu             sync.Mutex
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
	initial, _ := store.Initial()
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

		conn, _, err := websocket.DefaultDialer.DialContext(ctx, normalizeWSURL(e.Config.Host), nil)
		if err != nil {
			return fmt.Errorf("failed to dial websocket: %w", err)
		}

		stopClose := context.AfterFunc(ctx, func() {
			_ = conn.Close()
		})

		newVersion, remote, err := e.handshake(ctx, conn, cs.version, initial)
		if err != nil {
			stopClose()
			_ = conn.Close()
			return err
		}

		cs.conn = conn
		cs.stopClose = stopClose
		cs.version = newVersion
		cs.remote = remote
		cs.lastMessageTs = time.Now()
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
						return
					}

					elapsed := time.Since(lastMsg)
					e.Logger.Debug().Dur("elapsed", elapsed).Msg("continuous: heartbeat check")
					if elapsed >= heartbeatTimeout {
						e.Logger.Warn().Dur("elapsed", elapsed).Msg("continuous: heartbeat timeout, closing connection")
						_ = conn.Close()
						return
					}
					if elapsed >= heartbeatSendThreshold {
						if err := conn.WriteJSON(map[string]any{"op": "ping"}); err != nil {
							e.Logger.Warn().Err(err).Msg("continuous: failed to send ping")
						} else {
							e.Logger.Debug().Dur("elapsed", elapsed).Msg("continuous: sent ping")
						}
					}
				}
			}
		}()
	}

	startReadPump()
	startHeartbeat()

	watcher, err := watchpkg.New(e.Config.VaultPath, append([]string{".git"}, e.Config.IgnoreFolders...), e.Logger, rescanInterval)
	if err != nil {
		return err
	}
	go watcher.Run(ctx)

	var debounceTimer *time.Timer
	var debounceMu sync.Mutex
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

		plan := buildPlan(currentLocal, previousLocal, currentRemote, previousRemote)
		e.Logger.Info().Int("planned_actions", len(plan)).Msg("continuous: sync plan created")

		if len(plan) == 0 {
			return
		}

		// Open a dedicated connection for plan execution
		connB, _, err := websocket.DefaultDialer.DialContext(ctx, normalizeWSURL(e.Config.Host), nil)
		if err != nil {
			e.Logger.Error().Err(err).Msg("continuous: failed to dial execution connection")
			return
		}
		defer func() {
			_ = connB.Close()
		}()

		execVersion, _, err := e.handshake(ctx, connB, version, false)
		if err != nil {
			e.Logger.Error().Err(err).Msg("continuous: failed to handshake execution connection")
			return
		}

		session := newRemoteSession(connB, currentRemote, execVersion, ctx, e.enc, e.Logger, e.rawKey)
		if err := e.executePlan(plan, currentLocal, session); err != nil {
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

		if err := e.saveState(store, currentLocal, remoteForSave, versionForSave); err != nil {
			e.Logger.Error().Err(err).Msg("continuous: failed to save state")
			return
		}

		e.Logger.Info().Msg("continuous: sync complete")
	}

	if initial {
		scheduleSync()
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-trigger:
			scheduleSync()
		case <-watcher.Out:
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

		reconnectLoop:
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(reconnectBackoff):
					if err := connect(); err != nil {
						e.Logger.Error().Err(err).Msg("continuous: reconnection failed, retrying...")
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

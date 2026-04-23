package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	configpkg "github.com/Belphemur/obsidian-headless/src-go/internal/config"
	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
	"github.com/Belphemur/obsidian-headless/src-go/internal/storage"
	watchpkg "github.com/Belphemur/obsidian-headless/src-go/internal/sync/watch"
	"github.com/Belphemur/obsidian-headless/src-go/internal/util"
)

const (
	chunkSize          = 2 * 1024 * 1024
	maxRemoteFileSize  = 200 * 1024 * 1024
	websocketIOTimeout = 30 * time.Second
	// staleLockAge only removes lock files that are clearly abandoned while
	// still leaving ample room for legitimately long-running sync sessions.
	staleLockAge       = 24 * time.Hour
)

type Engine struct {
	Config model.SyncConfig
	Token  string
	Logger zerolog.Logger
}

type remoteSession struct {
	conn      *websocket.Conn
	remote    map[string]model.FileRecord
	version   int64
	ctx       context.Context
	stopClose func() bool
}

type syncAction struct {
	Path string
	Kind string
}

func NewEngine(config model.SyncConfig, token string, logger zerolog.Logger) *Engine {
	return &Engine{Config: config, Token: token, Logger: logger}
}

func (e *Engine) RunOnce(ctx context.Context) error {
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
	defer store.Close()
	previousLocal, err := store.LoadLocalFiles()
	if err != nil {
		return err
	}
	previousRemote, err := store.LoadServerFiles()
	if err != nil {
		return err
	}
	currentLocal, err := util.ScanVault(e.Config.VaultPath, e.configDir(), e.Config.IgnoreFolders)
	if err != nil {
		return err
	}
	version, err := store.Version()
	if err != nil {
		return err
	}
	initial, err := store.Initial()
	if err != nil {
		return err
	}
	session, err := e.openRemoteSession(ctx, version, initial)
	if err != nil {
		return err
	}
	defer session.Close()
	plan := buildPlan(currentLocal, previousLocal, session.remote, previousRemote)
	e.Logger.Info().Int("planned_actions", len(plan)).Msg("sync plan created")
	for _, action := range plan {
		switch action.Kind {
		case "download":
			record := session.remote[action.Path]
			if record.Deleted {
				if err := e.removeLocalPath(action.Path); err != nil && !os.IsNotExist(err) {
					return err
				}
				e.Logger.Info().Str("path", action.Path).Msg("deleted local file from remote tombstone")
				continue
			}
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
			session.remote[action.Path] = record
			e.Logger.Info().Str("path", action.Path).Msg("uploaded local file")
		case "delete-remote":
			if err := session.delete(action.Path); err != nil {
				return err
			}
			record := session.remote[action.Path]
			record.Deleted = true
			record.Size = 0
			record.Hash = ""
			session.remote[action.Path] = record
			e.Logger.Info().Str("path", action.Path).Msg("deleted remote file")
		}
	}
	refreshedLocal, err := util.ScanVault(e.Config.VaultPath, e.configDir(), e.Config.IgnoreFolders)
	if err != nil {
		return err
	}
	liveRemote := map[string]model.FileRecord{}
	for path, record := range session.remote {
		if !record.Deleted {
			liveRemote[path] = record
		}
	}
	if err := store.ReplaceLocalFiles(refreshedLocal); err != nil {
		return err
	}
	if err := store.ReplaceServerFiles(liveRemote); err != nil {
		return err
	}
	if err := store.SetVersion(session.version); err != nil {
		return err
	}
	return store.SetInitial(false)
}

func (e *Engine) RunContinuous(ctx context.Context) error {
	if err := e.RunOnce(ctx); err != nil {
		return err
	}
	if e.Config.SyncMode == "pull" || e.Config.SyncMode == "mirror" {
		ticker := time.NewTicker(30 * time.Second)
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
	ticker := time.NewTicker(30 * time.Second)
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

func (e *Engine) openRemoteSession(ctx context.Context, version int64, initial bool) (*remoteSession, error) {
	keyHash := ""
	if e.Config.EncryptionKey != "" {
		derivedKeyHash, err := util.DerivePasswordHash(e.Config.EncryptionKey, e.Config.EncryptionSalt)
		if err != nil {
			return nil, err
		}
		keyHash = derivedKeyHash
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, normalizeWSURL(e.Config.Host), nil)
	if err != nil {
		return nil, err
	}
	stopClose := context.AfterFunc(ctx, func() {
		_ = conn.Close()
	})
	initMessage := map[string]any{
		"op":                 "init",
		"token":              e.Token,
		"id":                 e.Config.VaultID,
		"keyhash":            keyHash,
		"version":            version,
		"initial":            initial,
		"device":             e.deviceName(),
		"encryption_version": e.Config.EncryptionVersion,
	}
	session := &remoteSession{conn: conn, remote: map[string]model.FileRecord{}, ctx: ctx, stopClose: stopClose}
	if err := session.writeJSON(initMessage); err != nil {
		stopClose()
		_ = conn.Close()
		return nil, err
	}
	var readyVersion int64
	for {
		messageType, payload, err := session.readMessage()
		if err != nil {
			stopClose()
			_ = conn.Close()
			return nil, err
		}
		if messageType != websocket.TextMessage {
			continue
		}
		message, err := decodeJSONMessage(payload)
		if err != nil {
			stopClose()
			_ = conn.Close()
			return nil, err
		}
		if response, ok := message["res"].(string); ok {
			if response == "err" {
				stopClose()
				_ = conn.Close()
				return nil, fmt.Errorf("sync init failed: %v", message["msg"])
			}
			if response == "ok" && message["user_id"] != nil {
				continue
			}
		}
		if operation, _ := message["op"].(string); operation == "push" {
			record := parseRemoteRecord(message)
			session.remote[record.Path] = record
			continue
		}
		if operation, _ := message["op"].(string); operation == "ready" {
			readyVersion = int64Value(message["version"])
			break
		}
	}
	if err := session.writeJSON(map[string]any{"op": "deleted", "suppressrenames": true}); err != nil {
		stopClose()
		_ = conn.Close()
		return nil, err
	}
	var deletedResponse struct {
		Res   string             `json:"res"`
		Items []model.FileRecord `json:"items"`
	}
	if err := session.readJSON(&deletedResponse); err != nil {
		stopClose()
		_ = conn.Close()
		return nil, err
	}
	for _, record := range deletedResponse.Items {
		record.Deleted = true
		session.remote[record.Path] = record
	}
	session.version = readyVersion
	return session, nil
}

func (s *remoteSession) pull(uid int64) ([]byte, error) {
	if err := s.writeJSON(map[string]any{"op": "pull", "uid": uid}); err != nil {
		return nil, err
	}
	var response struct {
		Res     string `json:"res"`
		Size    int64  `json:"size"`
		Pieces  int    `json:"pieces"`
		Deleted bool   `json:"deleted"`
		Msg     string `json:"msg"`
	}
	if err := s.readJSON(&response); err != nil {
		return nil, err
	}
	if response.Res == "err" {
		return nil, fmt.Errorf("%s", response.Msg)
	}
	if response.Deleted {
		return nil, nil
	}
	if response.Pieces == 0 && response.Size != 0 {
		return nil, fmt.Errorf("remote file declared size %d with no pieces", response.Size)
	}
	if response.Pieces == 0 {
		return nil, nil
	}
	if response.Size < 0 || response.Size > maxRemoteFileSize {
		return nil, fmt.Errorf("remote file size %d exceeds allowed maximum", response.Size)
	}
	var data bytes.Buffer
	data.Grow(int(response.Size))
	for index := 0; index < response.Pieces; index++ {
		messageType, payload, err := s.readMessage()
		if err != nil {
			return nil, err
		}
		if messageType != websocket.BinaryMessage {
			index--
			continue
		}
		if _, err := data.Write(payload); err != nil {
			return nil, err
		}
		if int64(data.Len()) > response.Size {
			return nil, fmt.Errorf("remote sent more data than declared: expected %d bytes, got %d", response.Size, data.Len())
		}
	}
	if int64(data.Len()) != response.Size {
		return nil, fmt.Errorf("remote file size mismatch: expected %d bytes, got %d", response.Size, data.Len())
	}
	return data.Bytes(), nil
}

func (s *remoteSession) push(record model.FileRecord, content []byte) error {
	pieces := int(math.Ceil(float64(len(content)) / float64(chunkSize)))
	if len(content) == 0 {
		pieces = 0
	}
	message := map[string]any{
		"op":        "push",
		"path":      record.Path,
		"extension": filepath.Ext(record.Path),
		"hash":      record.Hash,
		"ctime":     record.CTime,
		"mtime":     record.MTime,
		"folder":    false,
		"deleted":   false,
		"size":      len(content),
		"pieces":    pieces,
	}
	if err := s.writeJSON(message); err != nil {
		return err
	}
	var response map[string]any
	if err := s.readJSON(&response); err != nil {
		return err
	}
	if stringValue(response["res"]) == "err" {
		return fmt.Errorf("push failed: %s", stringValue(response["msg"]))
	}
	if pieces == 0 && stringValue(response["res"]) == "ok" {
		return nil
	}
	if pieces > 0 && stringValue(response["res"]) != "next" {
		return fmt.Errorf("push failed: unexpected initial response %q", stringValue(response["res"]))
	}
	for index, start := 0, 0; start < len(content); index, start = index+1, start+chunkSize {
		end := min(start+chunkSize, len(content))
		if err := s.writeMessage(websocket.BinaryMessage, content[start:end]); err != nil {
			return err
		}
		response = map[string]any{}
		if err := s.readJSON(&response); err != nil {
			return err
		}
		if stringValue(response["res"]) == "err" {
			return fmt.Errorf("push failed: %s", stringValue(response["msg"]))
		}
		if index < pieces-1 && stringValue(response["res"]) != "next" {
			return fmt.Errorf("push failed: unexpected chunk response %q", stringValue(response["res"]))
		}
		if index == pieces-1 && stringValue(response["res"]) != "ok" {
			return fmt.Errorf("push failed: unexpected final response %q", stringValue(response["res"]))
		}
	}
	return nil
}

func (s *remoteSession) delete(path string) error {
	if err := s.writeJSON(map[string]any{"op": "push", "path": path, "extension": filepath.Ext(path), "hash": "", "ctime": time.Now().UnixMilli(), "mtime": time.Now().UnixMilli(), "folder": false, "deleted": true, "size": 0, "pieces": 0}); err != nil {
		return err
	}
	var response map[string]any
	if err := s.readJSON(&response); err != nil {
		return err
	}
	if response["res"] == "err" {
		return fmt.Errorf("delete failed: %s", stringValue(response["msg"]))
	}
	return nil
}

func buildPlan(currentLocal, previousLocal, currentRemote, previousRemote map[string]model.FileRecord) []syncAction {
	pathsSet := map[string]struct{}{}
	for _, collection := range []map[string]model.FileRecord{currentLocal, previousLocal, currentRemote, previousRemote} {
		for path := range collection {
			pathsSet[path] = struct{}{}
		}
	}
	paths := make([]string, 0, len(pathsSet))
	for path := range pathsSet {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	actions := make([]syncAction, 0, len(paths))
	for _, path := range paths {
		currentL, hasCurrentL := currentLocal[path]
		previousL, hasPreviousL := previousLocal[path]
		currentR, hasCurrentR := currentRemote[path]
		previousR, hasPreviousR := previousRemote[path]
		localChanged := recordChanged(hasPreviousL, previousL, hasCurrentL, currentL)
		remoteChanged := recordChanged(hasPreviousR, previousR, hasCurrentR, currentR)
		switch {
		case remoteChanged && localChanged:
			if chooseRemote(hasCurrentL, currentL, hasCurrentR, currentR, hasPreviousL, previousL, hasPreviousR, previousR) {
				actions = append(actions, syncAction{Path: path, Kind: "download"})
			} else if hasCurrentL {
				actions = append(actions, syncAction{Path: path, Kind: "upload"})
			} else {
				actions = append(actions, syncAction{Path: path, Kind: "delete-remote"})
			}
		case remoteChanged:
			actions = append(actions, syncAction{Path: path, Kind: "download"})
		case localChanged:
			if hasCurrentL {
				actions = append(actions, syncAction{Path: path, Kind: "upload"})
			} else {
				actions = append(actions, syncAction{Path: path, Kind: "delete-remote"})
			}
		}
	}
	return actions
}

func recordChanged(hadBefore bool, before model.FileRecord, hasNow bool, now model.FileRecord) bool {
	if hadBefore != hasNow {
		return true
	}
	if !hadBefore && !hasNow {
		return false
	}
	return before.Hash != now.Hash || before.MTime != now.MTime || before.Size != now.Size || before.Deleted != now.Deleted
}

func chooseRemote(hasCurrentL bool, currentL model.FileRecord, hasCurrentR bool, currentR model.FileRecord, hasPreviousL bool, previousL model.FileRecord, hasPreviousR bool, previousR model.FileRecord) bool {
	localTime := int64(0)
	remoteTime := int64(0)
	if hasCurrentL {
		localTime = currentL.MTime
	} else if hasPreviousL {
		localTime = previousL.MTime
	}
	if hasCurrentR {
		remoteTime = currentR.MTime
	} else if hasPreviousR {
		remoteTime = previousR.MTime
	}
	if remoteTime == localTime {
		return hasCurrentR && (!hasCurrentL || currentR.Hash != currentL.Hash)
	}
	return remoteTime > localTime
}

func parseRemoteRecord(message map[string]any) model.FileRecord {
	return model.FileRecord{
		Path:    stringValue(message["path"]),
		Hash:    stringValue(message["hash"]),
		CTime:   int64Value(message["ctime"]),
		MTime:   int64Value(message["mtime"]),
		Size:    int64Value(message["size"]),
		Folder:  boolValue(message["folder"]),
		Deleted: boolValue(message["deleted"]),
		UID:     int64Value(message["uid"]),
		Device:  stringValue(message["device"]),
		User:    stringValue(message["user"]),
	}
}

func normalizeWSURL(host string) string {
	if strings.HasPrefix(host, "ws://") || strings.HasPrefix(host, "wss://") {
		return host
	}
	if after, ok := strings.CutPrefix(host, "http://"); ok {
		return "ws://" + after
	}
	if after, ok := strings.CutPrefix(host, "https://"); ok {
		return "wss://" + after
	}
	parsed, err := url.Parse(host)
	if err == nil && parsed.Host != "" {
		host = parsed.Host
	}
	if strings.HasPrefix(host, "127.0.0.1") || strings.HasPrefix(host, "localhost") {
		return "ws://" + host
	}
	return "wss://" + host
}

func (e *Engine) deviceName() string {
	if e.Config.DeviceName != "" {
		return e.Config.DeviceName
	}
	return configpkg.DefaultDeviceName()
}

func (e *Engine) configDir() string {
	if e.Config.ConfigDir != "" {
		return e.Config.ConfigDir
	}
	return ".obsidian"
}

func (e *Engine) acquireLock() (func(), error) {
	lockPath := configpkg.LockPath(e.Config.VaultPath, e.configDir())
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			stale, staleErr := removeStaleLock(lockPath)
			if staleErr != nil {
				return nil, staleErr
			}
			if stale {
				file, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("sync already running: %w", err)
	}
	_, _ = file.WriteString(fmt.Sprintf("%d\n%d\n", os.Getpid(), time.Now().Unix()))
	return func() {
		_ = file.Close()
		_ = os.Remove(lockPath)
	}, nil
}

func (e *Engine) removeLocalPath(path string) error {
	if !filepath.IsLocal(filepath.FromSlash(path)) {
		return fmt.Errorf("invalid local path %q", path)
	}
	fullPath, err := util.SafeJoin(e.Config.VaultPath, path)
	if err != nil {
		return err
	}
	if err := os.Remove(fullPath); err != nil {
		return err
	}
	vaultRoot, err := filepath.Abs(e.Config.VaultPath)
	if err != nil {
		return err
	}
	vaultRoot = filepath.Clean(vaultRoot)
	for dir := filepath.Dir(fullPath); dir != vaultRoot && dir != "." && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		_ = os.Remove(dir)
	}
	return nil
}

func stringValue(value any) string {
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

func boolValue(value any) bool {
	if raw, ok := value.(bool); ok {
		return raw
	}
	return false
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed
		}
		return 0
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func decodeJSONMessage(payload []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var message map[string]any
	if err := decoder.Decode(&message); err != nil {
		return nil, err
	}
	return message, nil
}

func (s *remoteSession) Close() error {
	if s.stopClose != nil {
		s.stopClose()
	}
	return s.conn.Close()
}

func (s *remoteSession) writeJSON(value any) error {
	if err := s.conn.SetWriteDeadline(s.ioDeadline()); err != nil {
		return err
	}
	return s.conn.WriteJSON(value)
}

func (s *remoteSession) readJSON(value any) error {
	if err := s.conn.SetReadDeadline(s.ioDeadline()); err != nil {
		return err
	}
	return s.conn.ReadJSON(value)
}

func (s *remoteSession) writeMessage(messageType int, data []byte) error {
	if err := s.conn.SetWriteDeadline(s.ioDeadline()); err != nil {
		return err
	}
	return s.conn.WriteMessage(messageType, data)
}

func (s *remoteSession) readMessage() (int, []byte, error) {
	if err := s.conn.SetReadDeadline(s.ioDeadline()); err != nil {
		return 0, nil, err
	}
	return s.conn.ReadMessage()
}

func (s *remoteSession) ioDeadline() time.Time {
	deadline := time.Now().Add(websocketIOTimeout)
	if ctxDeadline, ok := s.ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		return ctxDeadline
	}
	return deadline
}

func removeStaleLock(lockPath string) (bool, error) {
	info, err := os.Stat(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if time.Since(info.ModTime()) < staleLockAge {
		return false, nil
	}
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return true, nil
}

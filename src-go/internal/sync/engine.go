package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	chunkSize          = 2 * 1024 * 1024
	maxRemoteFileSize  = 200 * 1024 * 1024
	websocketIOTimeout = 30 * time.Second
	// staleLockAge only removes lock files that are clearly abandoned while
	// still leaving ample room for legitimately long-running sync sessions.
	staleLockAge = 24 * time.Hour
)

type Engine struct {
	Config model.SyncConfig
	Token  string
	Logger zerolog.Logger

	enc    encryption.EncryptionProvider
	rawKey []byte
}

type remoteSession struct {
	conn       *websocket.Conn
	remote     map[string]model.FileRecord
	version    int64
	ctx        context.Context
	stopClose  func() bool
	enc        encryption.EncryptionProvider
	Logger     zerolog.Logger
	rawKey     []byte
	justPushed *model.FileRecord // Track the file we just pushed to detect self-echoes
}

type syncAction struct {
	Path string
	Kind string
}

func NewEngine(config model.SyncConfig, token string, logger zerolog.Logger) (*Engine, error) {
	e := &Engine{Config: config, Token: token, Logger: logger}

	// Create encryption provider for encrypted vaults
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

	return e, nil
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
	// Decrypt paths in previousRemote if vault is encrypted
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
	defer func() {
		_ = session.Close()
	}()
	e.Logger.Debug().Int("remote_files", len(session.remote)).Msg("loaded remote files")
	for path := range session.remote {
		e.Logger.Debug().Str("path", path).Msg("remote file")
	}
	// Clean up session.remote to remove any invalid (hex-like) paths that resulted from failed decryption
	validRemote := make(map[string]model.FileRecord)
	for path, record := range session.remote {
		if isValidPath(path) {
			validRemote[path] = record
		} else {
			e.Logger.Warn().Str("path", path).Msg("removing invalid path from remote")
		}
	}
	session.remote = validRemote
	plan := buildPlan(currentLocal, previousLocal, session.remote, previousRemote)
	e.Logger.Info().Int("planned_actions", len(plan)).Msg("sync plan created")
	for i, action := range plan {
		e.Logger.Debug().Int("action", i).Str("kind", action.Kind).Str("path", action.Path).Msg("action")
		if action.Path == "" {
			e.Logger.Error().Msg("EMPTY PATH IN ACTION!")
		}
	}
	for _, action := range plan {
		switch action.Kind {
		case "download":
			record, exists := session.remote[action.Path]
			e.Logger.Debug().Bool("exists", exists).Str("path", action.Path).Msg("download lookup")
			if !exists {
				// Try previousRemote
				record, exists = previousRemote[action.Path]
				e.Logger.Debug().Bool("in_previous", exists).Msg("try previous remote")
			}
			// Fix: Override record.Path with action.Path to ensure we use the decrypted path
			record.Path = action.Path
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
	if e.rawKey != nil {
		derivedKeyHash, err := encryption.ComputeKeyHash(
			e.rawKey,
			e.Config.EncryptionSalt,
			encryption.EncryptionVersion(e.Config.EncryptionVersion),
		)
		if err != nil {
			return nil, err
		}
		e.Logger.Debug().Str("keyHash", derivedKeyHash).Msg("computed key hash for init")
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
	session := &remoteSession{conn: conn, remote: map[string]model.FileRecord{}, ctx: ctx, stopClose: stopClose, enc: e.enc, Logger: e.Logger, rawKey: e.rawKey}
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
			record := session.parseRemoteRecord(message)
			if record.Path == "" {
				session.Logger.Warn().Msg("skipping record with failed decryption")
				continue
			}
			// Check if this is a self-echo (we just pushed this file)
			if session.justPushed != nil && !session.justPushed.Deleted && !record.Deleted {
				if record.Path == session.justPushed.Path && record.MTime == session.justPushed.MTime {
					session.Logger.Debug().Str("path", record.Path).Msg("detected self-echo, skipping")
					session.justPushed = nil
					// Still add to remote since this is our authoritative record
					session.remote[record.Path] = record
					continue
				}
			}
			session.Logger.Debug().Str("path", record.Path).Msg("parsed remote record from push")
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
		// Parse the deleted record through parseRemoteRecord to decrypt path
		msg := map[string]any{
			"path":    record.Path,
			"hash":    record.Hash,
			"ctime":   record.CTime,
			"mtime":   record.MTime,
			"size":    record.Size,
			"folder":  record.Folder,
			"deleted": true,
			"uid":     record.UID,
			"device":  record.Device,
			"user":    record.User,
		}
		parsed := session.parseRemoteRecord(msg)
		parsed.Deleted = true
		session.remote[parsed.Path] = parsed
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

	// Decrypt data if vault is encrypted
	return s.decryptData(data.Bytes())
}

func (s *remoteSession) push(record model.FileRecord, content []byte) error {
	var pieces int
	if len(content) > 0 {
		pieces = (len(content) + chunkSize - 1) / chunkSize
	}

	// Track what we're about to push to detect self-echoes
	s.justPushed = &record

	// Encrypt path and content if vault is encrypted
	encryptedPath := s.encryptPath(record.Path)
	encryptedContent, err := s.encryptData(content)
	if err != nil {
		return fmt.Errorf("failed to encrypt content: %w", err)
	}

	message := map[string]any{
		"op":        "push",
		"path":      encryptedPath,
		"extension": filepath.Ext(record.Path),
		"hash":      record.Hash,
		"ctime":     record.CTime,
		"mtime":     record.MTime,
		"folder":    false,
		"deleted":   false,
		"size":      len(encryptedContent),
		"pieces":    pieces,
	}
	if err := s.writeJSON(message); err != nil {
		return err
	}
	var response map[string]any
	if err := s.readJSON(&response); err != nil {
		return fmt.Errorf("push readJSON error: %w", err)
	}
	s.Logger.Debug().Interface("response", response).Str("res", fmt.Sprintf("%v", response["res"])).Msg("push response")
	if stringValue(response["res"]) == "err" {
		return fmt.Errorf("push failed: %s", stringValue(response["msg"]))
	}
	// Handle server returning "ok" when it already has the content (deduplication)
	if stringValue(response["res"]) == "ok" || stringValue(response["op"]) == "ok" {
		return nil
	}
	if pieces > 0 && stringValue(response["res"]) != "next" {
		return fmt.Errorf("push failed: unexpected initial response %q", stringValue(response["res"]))
	}
	for index, start := 0, 0; start < len(encryptedContent); index, start = index+1, start+chunkSize {
		end := min(start+chunkSize, len(encryptedContent))
		if err := s.writeMessage(websocket.BinaryMessage, encryptedContent[start:end]); err != nil {
			return err
		}
		// Read response, processing any push message echoes
		var response map[string]any
		for {
			if err := s.readJSON(&response); err != nil {
				return fmt.Errorf("push chunk readJSON error: %w", err)
			}
			s.Logger.Debug().Interface("chunk_response", response).Msg("push chunk response")
			// If this is a push message echo, process it and add to remote
			if op, ok := response["op"].(string); ok && op == "push" {
				s.Logger.Debug().Msg("processing push echo")
				parsed := s.parseRemoteRecord(response)
				if parsed.Path != "" {
					s.remote[parsed.Path] = parsed
				}
				continue
			}
			break
		}
		if stringValue(response["res"]) == "err" {
			return fmt.Errorf("push failed: %s", stringValue(response["msg"]))
		}
		if index < pieces-1 {
			// Check for "next" in either res or op field
			if stringValue(response["res"]) != "next" && stringValue(response["op"]) != "next" {
				return fmt.Errorf("push failed: unexpected chunk response %q", stringValue(response["res"]))
			}
		}
		if index == pieces-1 {
			// Check for "ok" in either res or op field (push echoes use op:ok)
			if stringValue(response["res"]) == "ok" || stringValue(response["op"]) == "ok" {
				return nil
			}
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
			if isValidPath(path) {
				pathsSet[path] = struct{}{}
			}
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

// parseRemoteRecord decrypts path for encrypted vaults
func (s *remoteSession) parseRemoteRecord(message map[string]any) model.FileRecord {
	record := parseRemoteRecord(message)
	s.Logger.Debug().Str("path", record.Path).Msg("decrypting path")
	if s.enc != nil && record.Path != "" {
		decryptedPath, err := s.enc.DecryptPath(record.Path)
		if err != nil {
			s.Logger.Warn().Err(err).Str("path", record.Path).Msg("failed to decrypt path, skipping record")
			record.Path = ""
		} else {
			s.Logger.Debug().Str("decrypted_path", decryptedPath).Msg("path decrypted")
			record.Path = decryptedPath
		}
	}
	return record
}

// encryptPath encrypts path for encrypted vaults
func (s *remoteSession) encryptPath(path string) string {
	if s.enc == nil {
		return path
	}
	s.Logger.Debug().Str("plain_path", path).Msg("encrypting path")
	encPath, err := s.enc.EncryptPath(path)
	if err != nil {
		s.Logger.Warn().Err(err).Str("path", path).Msg("failed to encrypt path, using as-is")
		return path
	}
	s.Logger.Debug().Str("encrypted_path", encPath).Msg("path encrypted")
	return encPath
}

// encryptData encrypts data for encrypted vaults
func (s *remoteSession) encryptData(data []byte) ([]byte, error) {
	if s.enc == nil {
		return data, nil
	}
	return s.enc.EncryptData(data)
}

// decryptData decrypts data for encrypted vaults
func (s *remoteSession) decryptData(data []byte) ([]byte, error) {
	if s.enc == nil {
		return data, nil
	}
	return s.enc.DecryptData(data)
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
	_, _ = fmt.Fprintf(file, "%d\n%d\n", os.Getpid(), time.Now().Unix())
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

func isValidPath(path string) bool {
	if path == "" {
		return false
	}
	// Check for null bytes (invalid in all OSes)
	for i := 0; i < len(path); i++ {
		if path[i] == 0 {
			return false
		}
	}
	// Use filepath.Clean to normalize and check for traversal
	cleaned := filepath.Clean(path)
	// Reject if cleaning changes the path significantly or creates traversal
	if cleaned == ".." || cleaned[:2] == ".." {
		return false
	}
	return true
}

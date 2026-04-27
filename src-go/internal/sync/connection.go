package sync

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gorilla/websocket"

	configpkg "github.com/Belphemur/obsidian-headless/src-go/internal/config"
	"github.com/Belphemur/obsidian-headless/src-go/internal/encryption"
	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
	"github.com/Belphemur/obsidian-headless/src-go/internal/storage"
)

func (e *Engine) ensureConnected(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.conn != nil {
		return nil
	}

	statePath, err := configpkg.StatePath(e.Config.VaultID, e.Config.StatePath)
	if err != nil {
		return err
	}
	store, err := storage.Open(statePath)
	if err != nil {
		return fmt.Errorf("failed to open state db: %w", err)
	}
	initial, _ := store.Initial()
	_ = store.Close()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, normalizeWSURL(e.Config.Host), nil)
	if err != nil {
		return fmt.Errorf("failed to dial websocket: %w", err)
	}

	e.stopClose = context.AfterFunc(ctx, func() {
		_ = conn.Close()
	})

	version, remote, err := e.handshake(ctx, conn, e.version, initial)
	if err != nil {
		e.stopClose()
		_ = conn.Close()
		e.stopClose = nil
		return err
	}

	e.version = version
	e.remote = remote
	e.conn = conn
	return nil
}

func (e *Engine) handshake(ctx context.Context, conn *websocket.Conn, version int64, initial bool) (int64, map[string]model.FileRecord, error) {
	keyHash := ""
	if e.rawKey != nil {
		derivedKeyHash, err := encryption.ComputeKeyHash(e.rawKey, e.Config.EncryptionSalt, encryption.EncryptionVersion(e.Config.EncryptionVersion))
		if err != nil {
			return 0, nil, fmt.Errorf("failed to compute key hash: %w", err)
		}
		e.Logger.Debug().Str("keyHash", derivedKeyHash).Msg("computed key hash for init")
		keyHash = derivedKeyHash
	}

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

	if err := writeJSONLogged(conn, initMessage, e.Logger); err != nil {
		return 0, nil, fmt.Errorf("failed to send init: %w", err)
	}

	// Read init response
	var initResponse map[string]any
	if err := readJSONLogged(conn, &initResponse, e.Logger); err != nil {
		return 0, nil, fmt.Errorf("failed to read init response: %w", err)
	}
	e.Logger.Debug().Interface("initResponse", initResponse).Msg("init response received")
	if res, _ := initResponse["res"].(string); res == "err" || stringValue(initResponse["status"]) == "err" {
		return 0, nil, fmt.Errorf("init failed: %s", stringValue(initResponse["msg"]))
	}

	// Read existing files and ready message
	remote := make(map[string]model.FileRecord)
	for {
		var msg map[string]any
		if err := readJSONLogged(conn, &msg, e.Logger); err != nil {
			return 0, nil, fmt.Errorf("failed to read ready message: %w", err)
		}
		e.Logger.Debug().Str("op", stringValue(msg["op"])).Interface("msg", msg).Msg("init handshake message")
		if op, _ := msg["op"].(string); op == "ready" {
			version = int64Value(msg["version"])
			e.Logger.Info().Int64("version", version).Msg("received ready")
			break
		}
		if op, _ := msg["op"].(string); op == "push" {
			session := newRemoteSession(conn, remote, version, ctx, e.enc, e.Logger, e.rawKey)
			record := session.parseRemoteRecord(msg)
			if record.Path != "" {
				remote[record.Path] = record
				e.Logger.Debug().Str("path", record.Path).Int64("uid", record.UID).Msg("added remote file from init")
			}
		}
	}

	return version, remote, nil
}

func decodeJSONMessage(data []byte) (map[string]any, error) {
	var msg map[string]any
	err := json.Unmarshal(data, &msg)
	return msg, err
}

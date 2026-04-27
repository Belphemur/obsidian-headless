package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/Belphemur/obsidian-headless/src-go/internal/encryption"
	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
)

type remoteSession struct {
	conn       *websocket.Conn
	remote     map[string]model.FileRecord
	version    int64
	ctx        context.Context
	enc        encryption.EncryptionProvider
	Logger     zerolog.Logger
	rawKey     []byte
	justPushed *model.FileRecord
	mu         sync.Mutex
}

func newRemoteSession(conn *websocket.Conn, remote map[string]model.FileRecord, version int64, ctx context.Context, enc encryption.EncryptionProvider, logger zerolog.Logger, rawKey []byte) *remoteSession {
	return &remoteSession{
		conn:    conn,
		remote:  remote,
		version: version,
		ctx:     ctx,
		enc:     enc,
		Logger:  logger,
		rawKey:  rawKey,
	}
}

func (s *remoteSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return nil
}

func (s *remoteSession) writeJSON(msg map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONLogged(s.conn, msg, s.Logger)
}

func (s *remoteSession) readJSON(v any) error {
	return readJSONLogged(s.conn, v, s.Logger)
}

func (s *remoteSession) readMessage() (int, []byte, error) {
	return readMessageLogged(s.conn, s.Logger)
}

func (s *remoteSession) parseRemoteRecord(msg map[string]any) model.FileRecord {
	record := model.FileRecord{}
	if v, ok := msg["path"].(string); ok {
		record.Path = s.decryptPath(v)
	}
	if v, ok := msg["hash"].(string); ok {
		record.Hash = s.decryptHash(v)
	} else {
		record.Hash = ""
	}
	record.CTime = int64Value(msg["ctime"])
	record.MTime = int64Value(msg["mtime"])
	record.Size = int64Value(msg["size"])
	record.Folder, _ = msg["folder"].(bool)
	record.Deleted, _ = msg["deleted"].(bool)
	record.UID = int64Value(msg["uid"])
	record.Device, _ = msg["device"].(string)
	record.User, _ = msg["user"].(string)
	return record
}

func (s *remoteSession) decryptPath(encryptedPath string) string {
	if s.enc == nil || encryptedPath == "" {
		return encryptedPath
	}
	decPath, err := s.enc.DecryptPath(encryptedPath)
	if err != nil {
		s.Logger.Debug().Err(err).Str("path", encryptedPath).Msg("failed to decrypt path")
		return encryptedPath
	}
	return decPath
}

func (s *remoteSession) decryptHash(encryptedHash string) string {
	if s.enc == nil || encryptedHash == "" {
		return encryptedHash
	}
	decHash, err := s.enc.DecryptHash(encryptedHash)
	if err != nil {
		s.Logger.Debug().Err(err).Str("hash", encryptedHash).Msg("failed to decrypt hash")
		return encryptedHash
	}
	return decHash
}

func (s *remoteSession) encryptPath(path string) string {
	if s.enc == nil || path == "" {
		return path
	}
	encPath, err := s.enc.EncryptPath(path)
	if err != nil {
		s.Logger.Debug().Err(err).Str("path", path).Msg("failed to encrypt path")
		return path
	}
	return encPath
}

func (s *remoteSession) encryptHash(hash string) string {
	if s.enc == nil || hash == "" {
		return hash
	}
	encHash, err := s.enc.EncryptHash(hash)
	if err != nil {
		s.Logger.Debug().Err(err).Str("hash", hash).Msg("failed to encrypt hash")
		return hash
	}
	return encHash
}

func (s *remoteSession) encryptData(data []byte) ([]byte, error) {
	if s.enc == nil || len(data) == 0 {
		return data, nil
	}
	return s.enc.EncryptData(data)
}

func (s *remoteSession) decryptData(data []byte) ([]byte, error) {
	if s.enc == nil || len(data) == 0 {
		return data, nil
	}
	return s.enc.DecryptData(data)
}

func (s *remoteSession) pull(uid int64) ([]byte, error) {
	s.Logger.Debug().Int64("uid", uid).Msg("pull start")
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
	s.Logger.Debug().Str("res", response.Res).Int64("size", response.Size).Int("pieces", response.Pieces).Bool("deleted", response.Deleted).Msg("pull response")
	if response.Res == "err" {
		return nil, fmt.Errorf("%s", response.Msg)
	}
	if response.Deleted {
		return nil, nil
	}
	chunks := make([][]byte, 0, response.Pieces)
	for i := 0; i < response.Pieces; i++ {
		_, chunk, err := s.readMessage()
		if err != nil {
			return nil, err
		}
		s.Logger.Debug().Int("index", i).Int("chunkSize", len(chunk)).Msg("pull chunk")
		chunks = append(chunks, chunk)
	}
	data := mergeChunks(chunks)
	decrypted, err := s.decryptData(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt pulled content: %w", err)
	}
	s.Logger.Debug().Int("size", len(decrypted)).Msg("pull complete")
	return decrypted, nil
}

func (s *remoteSession) push(record model.FileRecord, data []byte) error {
	var pieces int
	if len(data) > 0 {
		pieces = (len(data) + chunkSize - 1) / chunkSize
	}

	s.Logger.Debug().Str("path", record.Path).Int("size", len(data)).Int("pieces", pieces).Msg("push start")

	s.justPushed = &model.FileRecord{
		Path:    record.Path,
		Hash:    record.Hash,
		MTime:   record.MTime,
		Deleted: record.Deleted,
	}

	encryptedPath := s.encryptPath(record.Path)
	encryptedHash := s.encryptHash(record.Hash)
	encryptedContent, err := s.encryptData(data)
	if err != nil {
		return fmt.Errorf("failed to encrypt content: %w", err)
	}

	message := map[string]any{
		"op":        "push",
		"path":      encryptedPath,
		"extension": filepath.Ext(record.Path),
		"hash":      encryptedHash,
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
	for {
		response = make(map[string]any)
		if err := s.readJSON(&response); err != nil {
			return fmt.Errorf("push readJSON error: %w", err)
		}
		s.Logger.Debug().Interface("response", response).Str("res", fmt.Sprintf("%v", response["res"])).Msg("push response")
		if op, ok := response["op"].(string); ok && op == "push" {
			s.Logger.Debug().Msg("processing push echo during initial response")
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
	if stringValue(response["res"]) == "ok" || stringValue(response["op"]) == "ok" {
		s.Logger.Debug().Str("path", record.Path).Msg("push complete (dedup)")
		return nil
	}
	if pieces > 0 && stringValue(response["res"]) != "next" {
		return fmt.Errorf("push failed: unexpected initial response %q", stringValue(response["res"]))
	}
	for index, start := 0, 0; start < len(encryptedContent); index, start = index+1, start+chunkSize {
		end := min(start+chunkSize, len(encryptedContent))
		s.Logger.Debug().Int("index", index).Int("chunkSize", end-start).Msg("push sending chunk")
		if err := s.writeMessageToConn(s.conn, websocket.BinaryMessage, encryptedContent[start:end]); err != nil {
			return err
		}
		var response map[string]any
		for {
			response = make(map[string]any)
			if err := s.readJSON(&response); err != nil {
				return fmt.Errorf("push chunk readJSON error: %w", err)
			}
			s.Logger.Debug().Interface("chunk_response", response).Msg("push chunk response")
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
			if stringValue(response["res"]) != "next" && stringValue(response["op"]) != "next" {
				return fmt.Errorf("push failed: unexpected chunk response %q", stringValue(response["res"]))
			}
		}
		if index == pieces-1 {
			if stringValue(response["res"]) == "ok" || stringValue(response["op"]) == "ok" {
				s.Logger.Debug().Str("path", record.Path).Msg("push complete")
				return nil
			}
			return fmt.Errorf("push failed: unexpected final response %q", stringValue(response["res"]))
		}
	}
	return nil
}

func (s *remoteSession) delete(path string) error {
	s.Logger.Debug().Str("path", path).Msg("delete start")
	encryptedPath := s.encryptPath(path)
	if err := s.writeJSON(map[string]any{"op": "push", "path": encryptedPath, "extension": filepath.Ext(path), "hash": "", "ctime": time.Now().UnixMilli(), "mtime": time.Now().UnixMilli(), "folder": false, "deleted": true, "size": 0, "pieces": 0}); err != nil {
		return err
	}
	var response map[string]any
	for {
		response = make(map[string]any)
		if err := s.readJSON(&response); err != nil {
			return fmt.Errorf("delete readJSON error: %w", err)
		}
		s.Logger.Debug().Interface("response", response).Msg("delete response")
		if op, ok := response["op"].(string); ok && op == "push" {
			s.Logger.Debug().Msg("processing push echo during delete")
			parsed := s.parseRemoteRecord(response)
			if parsed.Path != "" {
				s.remote[parsed.Path] = parsed
			}
			continue
		}
		break
	}
	if stringValue(response["res"]) == "err" {
		return fmt.Errorf("delete failed: %s", stringValue(response["msg"]))
	}
	if stringValue(response["res"]) == "ok" || stringValue(response["op"]) == "ok" {
		s.Logger.Debug().Str("path", path).Msg("delete complete")
		return nil
	}
	return fmt.Errorf("delete failed: unexpected response %q", stringValue(response["res"]))
}

func (s *remoteSession) writeMessageToConn(conn *websocket.Conn, msgType int, data []byte) error {
	return conn.WriteMessage(msgType, data)
}

func writeJSONLogged(conn *websocket.Conn, msg map[string]any, logger zerolog.Logger) error {
	data := mustMarshalJSON(msg)
	logger.Debug().Str("direction", "send").Str("type", "text").RawJSON("body", data).Msg("ws")
	return conn.WriteMessage(websocket.TextMessage, data)
}

func readJSONLogged(conn *websocket.Conn, v any, logger zerolog.Logger) error {
	_, data, err := readMessageLogged(conn, logger)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func readMessageLogged(conn *websocket.Conn, logger zerolog.Logger) (int, []byte, error) {
	msgType, data, err := conn.ReadMessage()
	if err != nil {
		logger.Debug().Str("direction", "recv").Err(err).Msg("ws read error")
		return msgType, nil, err
	}
	if msgType == websocket.TextMessage {
		logger.Debug().Str("direction", "recv").Str("type", "text").RawJSON("body", data).Msg("ws")
	} else {
		logger.Debug().Str("direction", "recv").Str("type", "binary").Int("size", len(data)).Msg("ws")
	}
	return msgType, data, nil
}

func mustMarshalJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func mergeChunks(chunks [][]byte) []byte {
	if len(chunks) == 0 {
		return nil
	}
	if len(chunks) == 1 {
		return chunks[0]
	}
	total := 0
	for _, c := range chunks {
		total += len(c)
	}
	result := make([]byte, 0, total)
	for _, c := range chunks {
		result = append(result, c...)
	}
	return result
}

func int64Value(v any) int64 {
	switch val := v.(type) {
	case int:
		return int64(val)
	case int64:
		return val
	case float64:
		return int64(val)
	default:
		return 0
	}
}

func stringValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		return ""
	}
}

func normalizeWSURL(host string) string {
	scheme := "wss"
	if host == "127.0.0.1" || host == "localhost" {
		scheme = "ws"
	}
	if host != "" && !hasScheme(host) {
		host = scheme + "://" + host
	}
	u, err := url.Parse(host)
	if err != nil {
		return host
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}
	return u.String()
}

func hasScheme(urlStr string) bool {
	return strings.HasPrefix(urlStr, "ws://") || strings.HasPrefix(urlStr, "wss://")
}

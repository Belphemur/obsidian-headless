package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
	"github.com/Belphemur/obsidian-headless/src-go/internal/storage"
)

func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

func TestLoadState(t *testing.T) {
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "state.db")
	store, err := storage.Open(statePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	local := map[string]model.FileRecord{
		"foo.md": {Path: "foo.md", Hash: "abc", Size: 3, MTime: 1000},
	}
	remote := map[string]model.FileRecord{
		"bar.md": {Path: "bar.md", Hash: "def", Size: 3, MTime: 2000},
	}
	if err := store.ReplaceLocalFiles(local); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceServerFiles(remote); err != nil {
		t.Fatal(err)
	}

	e := &Engine{Logger: testLogger()}
	loadedLocal, loadedRemote, err := e.loadState(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(loadedLocal) != 1 || loadedLocal["foo.md"].Hash != "abc" {
		t.Fatalf("unexpected local state: %+v", loadedLocal)
	}
	if len(loadedRemote) != 1 || loadedRemote["bar.md"].Hash != "def" {
		t.Fatalf("unexpected remote state: %+v", loadedRemote)
	}
}

func TestScanLocal(t *testing.T) {
	tmp := t.TempDir()
	mustWriteFile(t, filepath.Join(tmp, "a.md"), []byte("hello"))
	mustWriteFile(t, filepath.Join(tmp, "b.md"), []byte("world"))

	e := &Engine{
		Config: model.SyncConfig{VaultPath: tmp},
		Logger: testLogger(),
	}
	files, err := e.scanLocal()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if _, ok := files["a.md"]; !ok {
		t.Fatal("missing a.md")
	}
	if _, ok := files["b.md"]; !ok {
		t.Fatal("missing b.md")
	}
}

func TestSaveState(t *testing.T) {
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "state.db")
	store, err := storage.Open(statePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	local := map[string]model.FileRecord{
		"foo.md": {Path: "foo.md", Hash: "abc", Size: 3, MTime: 1000},
	}
	remote := map[string]model.FileRecord{
		"bar.md": {Path: "bar.md", Hash: "def", Size: 3, MTime: 2000},
	}

	e := &Engine{Logger: testLogger()}
	if err := e.saveState(store, local, remote, 42); err != nil {
		t.Fatal(err)
	}

	v, err := store.Version()
	if err != nil {
		t.Fatal(err)
	}
	if v != 42 {
		t.Fatalf("expected version 42, got %d", v)
	}

	loadedLocal, err := store.LoadLocalFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(loadedLocal) != 1 || loadedLocal["foo.md"].Hash != "abc" {
		t.Fatalf("unexpected local state: %+v", loadedLocal)
	}

	loadedRemote, err := store.LoadServerFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(loadedRemote) != 1 || loadedRemote["bar.md"].Hash != "def" {
		t.Fatalf("unexpected remote state: %+v", loadedRemote)
	}

	initial, err := store.Initial()
	if err != nil {
		t.Fatal(err)
	}
	if initial {
		t.Fatal("expected initial to be false")
	}
}

type mockSyncServer struct {
	t             *testing.T
	recordsByUID  map[int64]model.FileRecord
	recordsByPath map[string]model.FileRecord
	contentByUID  map[int64][]byte
	upgrader      websocket.Upgrader
}

func newMockSyncServer(t *testing.T) *mockSyncServer {
	return &mockSyncServer{
		t:             t,
		recordsByUID:  make(map[int64]model.FileRecord),
		recordsByPath: make(map[string]model.FileRecord),
		contentByUID:  make(map[int64][]byte),
		upgrader:      websocket.Upgrader{},
	}
}

func (s *mockSyncServer) addRecord(path string, uid int64, content []byte) {
	record := model.FileRecord{
		Path:    path,
		Hash:    fmt.Sprintf("%x", content),
		Size:    int64(len(content)),
		CTime:   time.Now().UnixMilli(),
		MTime:   time.Now().UnixMilli(),
		UID:     uid,
		Deleted: false,
	}
	s.recordsByUID[uid] = record
	s.recordsByPath[path] = record
	s.contentByUID[uid] = content
}

func (s *mockSyncServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.t.Logf("upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType != websocket.TextMessage {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		op, _ := msg["op"].(string)
		switch op {
		case "init":
			if err := conn.WriteJSON(map[string]any{"res": "ok"}); err != nil {
				return
			}
			// Send any existing records as pushes
			for _, record := range s.recordsByUID {
				if err := conn.WriteJSON(map[string]any{
					"op":      "push",
					"path":    record.Path,
					"hash":    record.Hash,
					"ctime":   record.CTime,
					"mtime":   record.MTime,
					"size":    record.Size,
					"folder":  record.Folder,
					"deleted": record.Deleted,
					"uid":     record.UID,
				}); err != nil {
					return
				}
			}
			if err := conn.WriteJSON(map[string]any{"op": "ready", "version": 1}); err != nil {
				return
			}
		case "pull":
			uid := int64Value(msg["uid"])
			record, ok := s.recordsByUID[uid]
			if !ok {
				if err := conn.WriteJSON(map[string]any{"res": "err", "msg": "not found"}); err != nil {
					return
				}
				continue
			}
			if record.Deleted {
				if err := conn.WriteJSON(map[string]any{"res": "ok", "deleted": true}); err != nil {
					return
				}
				continue
			}
			content := s.contentByUID[uid]
			pieces := 0
			if len(content) > 0 {
				pieces = (len(content) + chunkSize - 1) / chunkSize
			}
			if err := conn.WriteJSON(map[string]any{"res": "ok", "size": len(content), "pieces": pieces, "deleted": false}); err != nil {
				return
			}
			for i := 0; i < pieces; i++ {
				start := i * chunkSize
				end := min(start+chunkSize, len(content))
				if err := conn.WriteMessage(websocket.BinaryMessage, content[start:end]); err != nil {
					return
				}
			}
		case "push":
			path := stringValue(msg["path"])
			size := int(int64Value(msg["size"]))
			pieces := int(int64Value(msg["pieces"]))
			now := time.Now().UnixMilli()
			uid := int64(len(s.recordsByUID) + 1)

			record := model.FileRecord{
				Path:    path,
				Hash:    "",
				Size:    int64(size),
				CTime:   now,
				MTime:   now,
				UID:     uid,
				Deleted: false,
			}

			// Send push echo first
			if err := conn.WriteJSON(map[string]any{
				"op":      "push",
				"path":    path,
				"hash":    "",
				"ctime":   record.CTime,
				"mtime":   record.MTime,
				"size":    record.Size,
				"folder":  false,
				"deleted": false,
				"uid":     uid,
			}); err != nil {
				return
			}

			var content []byte
			if pieces == 0 {
				if err := conn.WriteJSON(map[string]any{"res": "ok"}); err != nil {
					return
				}
			} else {
				if err := conn.WriteJSON(map[string]any{"res": "next"}); err != nil {
					return
				}
				for i := 0; i < pieces; i++ {
					_, chunk, err := conn.ReadMessage()
					if err != nil {
						return
					}
					content = append(content, chunk...)

					if i < pieces-1 {
						if err := conn.WriteJSON(map[string]any{"res": "next"}); err != nil {
							return
						}
					} else {
						record.Hash = fmt.Sprintf("%x", content)
						s.recordsByUID[uid] = record
						s.recordsByPath[path] = record
						s.contentByUID[uid] = content
						if err := conn.WriteJSON(map[string]any{"res": "ok"}); err != nil {
							return
						}
					}
				}
			}
		case "delete":
			// delete is implemented as push with deleted:true in the actual protocol
		}
	}
}

func TestExecutePlan(t *testing.T) {
	mock := newMockSyncServer(t)
	mock.addRecord("remote.md", 1, []byte("remote content"))

	server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	defer server.Close()

	vault := t.TempDir()
	mustWriteFile(t, filepath.Join(vault, "local.md"), []byte("local content"))

	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send init handshake
	if err := conn.WriteJSON(map[string]any{"op": "init"}); err != nil {
		t.Fatal(err)
	}

	// Wait for ready message
	for {
		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		if msg["op"] == "ready" {
			break
		}
	}

	remote := make(map[string]model.FileRecord)
	for _, r := range mock.recordsByUID {
		remote[r.Path] = r
	}

	e := &Engine{
		Config: model.SyncConfig{VaultPath: vault},
		Logger: testLogger(),
	}

	currentLocal := map[string]model.FileRecord{
		"local.md": {Path: "local.md", Hash: "localhash", Size: 14, MTime: 1000},
	}
	previousLocal := map[string]model.FileRecord{}
	currentRemote := remote
	previousRemote := map[string]model.FileRecord{}

	plan := buildPlan(currentLocal, previousLocal, currentRemote, previousRemote)
	if len(plan) != 2 {
		t.Fatalf("expected 2 actions, got %d: %+v", len(plan), plan)
	}

	ctx := context.Background()
	session := newRemoteSession(conn, remote, 1, ctx, nil, testLogger(), nil)
	if err := e.executePlan(plan, currentLocal, session); err != nil {
		t.Fatal(err)
	}

	// Verify local file was downloaded
	data, err := os.ReadFile(filepath.Join(vault, "remote.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "remote content" {
		t.Fatalf("unexpected downloaded content: %q", string(data))
	}

	// Verify remote file was uploaded
	if len(mock.contentByUID) != 2 {
		t.Fatalf("expected 2 remote records, got %d", len(mock.contentByUID))
	}
	foundLocal := false
	for _, content := range mock.contentByUID {
		if string(content) == "local content" {
			foundLocal = true
			break
		}
	}
	if !foundLocal {
		t.Fatal("uploaded local content not found on mock server")
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

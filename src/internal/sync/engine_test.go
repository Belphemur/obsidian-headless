package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/Belphemur/obsidian-headless/internal/model"
	"github.com/Belphemur/obsidian-headless/internal/storage"
)

func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

func TestLoadState(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	if err := e.saveState(store, local, remote, nil, nil, 42); err != nil {
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

func TestSaveStateIncremental(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "state.db")
	store, err := storage.Open(statePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Seed previous state with two files
	prevLocal := map[string]model.FileRecord{
		"a.md": {Path: "a.md", Hash: "aaa", Size: 3, MTime: 1000},
		"b.md": {Path: "b.md", Hash: "bbb", Size: 3, MTime: 2000},
	}
	prevRemote := map[string]model.FileRecord{
		"a.md": {Path: "a.md", Hash: "aaa", Size: 3, MTime: 1000},
		"c.md": {Path: "c.md", Hash: "ccc", Size: 3, MTime: 3000, UID: 1},
	}
	if err := store.ReplaceLocalFiles(prevLocal); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceServerFiles(prevRemote); err != nil {
		t.Fatal(err)
	}
	if err := store.SetInitial(false); err != nil {
		t.Fatal(err)
	}

	// Now simulate an incremental save: a.md changed, b.md deleted, c.md unchanged, d.md added
	currentLocal := map[string]model.FileRecord{
		"a.md": {Path: "a.md", Hash: "aaa_new", Size: 6, MTime: 1100}, // hash changed
		"d.md": {Path: "d.md", Hash: "ddd", Size: 3, MTime: 4000},     // new file
	}
	currentRemote := map[string]model.FileRecord{
		"a.md": {Path: "a.md", Hash: "aaa", Size: 3, MTime: 1000, UID: 99}, // uid changed
		"c.md": {Path: "c.md", Hash: "ccc", Size: 3, MTime: 3000, UID: 1},  // unchanged
	}

	e := &Engine{Logger: testLogger()}
	if err := e.saveState(store, currentLocal, currentRemote, prevLocal, prevRemote, 43); err != nil {
		t.Fatal(err)
	}

	// Verify version
	v, err := store.Version()
	if err != nil {
		t.Fatal(err)
	}
	if v != 43 {
		t.Fatalf("expected version 43, got %d", v)
	}

	// Verify local state: a.md should have new hash, b.md should be gone, d.md should exist
	loadedLocal, err := store.LoadLocalFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(loadedLocal) != 2 {
		t.Fatalf("expected 2 local files, got %d: %+v", len(loadedLocal), loadedLocal)
	}
	if r, ok := loadedLocal["a.md"]; !ok || r.Hash != "aaa_new" || r.Size != 6 {
		t.Fatalf("a.md not updated correctly: %+v", loadedLocal["a.md"])
	}
	if _, ok := loadedLocal["b.md"]; ok {
		t.Fatal("b.md should have been deleted")
	}
	if r, ok := loadedLocal["d.md"]; !ok || r.Hash != "ddd" {
		t.Fatalf("d.md not created: %+v", loadedLocal["d.md"])
	}

	// Verify remote state: a.md should have new uid, c.md should still exist
	loadedRemote, err := store.LoadServerFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(loadedRemote) != 2 {
		t.Fatalf("expected 2 remote files, got %d: %+v", len(loadedRemote), loadedRemote)
	}
	if r, ok := loadedRemote["a.md"]; !ok || r.UID != 99 {
		t.Fatalf("a.md uid not updated: %+v", loadedRemote["a.md"])
	}
	if r, ok := loadedRemote["c.md"]; !ok || r.UID != 1 {
		t.Fatalf("c.md should be unchanged: %+v", loadedRemote["c.md"])
	}
}

type mockSyncServer struct {
	t              *testing.T
	mu             sync.Mutex
	recordsByUID   map[int64]model.FileRecord
	recordsByPath  map[string]model.FileRecord
	contentByUID   map[int64][]byte
	pushMsgs       []map[string]any
	upgrader       websocket.Upgrader
	pingCount      int
	closeAfterPing bool
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
	s.mu.Lock()
	defer s.mu.Unlock()
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

func (s *mockSyncServer) cloneRecordsByPath() map[string]model.FileRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]model.FileRecord, len(s.recordsByPath))
	maps.Copy(result, s.recordsByPath)
	return result
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
			// Snapshot records under lock to avoid holding it during writes.
			s.mu.Lock()
			records := make([]model.FileRecord, 0, len(s.recordsByUID))
			for _, record := range s.recordsByUID {
				records = append(records, record)
			}
			s.mu.Unlock()
			for _, record := range records {
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
			s.mu.Lock()
			record, ok := s.recordsByUID[uid]
			content := s.contentByUID[uid]
			s.mu.Unlock()
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
			s.mu.Lock()
			s.pushMsgs = append(s.pushMsgs, msg)
			s.mu.Unlock()
			path := stringValue(msg["path"])
			size := int(int64Value(msg["size"]))
			pieces := int(int64Value(msg["pieces"]))
			now := time.Now().UnixMilli()

			s.mu.Lock()
			uid := int64(len(s.recordsByUID) + 1)
			s.mu.Unlock()

			record := model.FileRecord{
				Path:    path,
				Hash:    "",
				Size:    int64(size),
				CTime:   now,
				MTime:   now,
				UID:     uid,
				Deleted: false,
			}

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
				for i := range pieces {
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
						s.mu.Lock()
						s.recordsByUID[uid] = record
						s.recordsByPath[path] = record
						s.contentByUID[uid] = content
						s.mu.Unlock()
						if err := conn.WriteJSON(map[string]any{"res": "ok"}); err != nil {
							return
						}
					}
				}
			}
		case "ping":
			s.mu.Lock()
			s.pingCount++
			shouldClose := s.closeAfterPing
			s.mu.Unlock()
			if err := conn.WriteJSON(map[string]any{"op": "pong"}); err != nil {
				return
			}
			if shouldClose {
				return
			}
		case "delete":
			// delete is implemented as push with deleted:true in the actual protocol
		}
	}
}

func TestExecutePlan(t *testing.T) {
	t.Parallel()
	mock := newMockSyncServer(t)
	mock.addRecord("remote.md", 1, []byte("remote content"))

	server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	defer server.Close()

	vault := t.TempDir()
	mustWriteFile(t, filepath.Join(vault, "local.md"), []byte("local content"))
	wsURL := "ws" + server.URL[4:]

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{"op": "init"}); err != nil {
		t.Fatal(err)
	}

	for {
		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		if msg["op"] == "ready" {
			break
		}
	}

	remote := mock.cloneRecordsByPath()

	e := &Engine{
		Config: model.SyncConfig{
			VaultPath: vault,
			Host:      wsURL,
		},
		Logger: testLogger(),
	}

	currentLocal := map[string]model.FileRecord{
		"local.md": {Path: "local.md", Hash: "localhash", Size: 14, MTime: 1000},
	}
	previousLocal := map[string]model.FileRecord{}
	currentRemote := remote
	previousRemote := map[string]model.FileRecord{}

	plan := buildPlan(currentLocal, previousLocal, currentRemote, previousRemote, ".obsidian")
	if len(plan) != 2 {
		t.Fatalf("expected 2 actions, got %d: %+v", len(plan), plan)
	}

	ctx := context.Background()
	session := newRemoteSession(conn, remote, 1, ctx, nil, testLogger(), nil)
	if err := e.executePlan(ctx, plan, currentLocal, previousRemote, previousLocal, session); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(vault, "remote.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "remote content" {
		t.Fatalf("unexpected downloaded content: %q", string(data))
	}

	mock.mu.Lock()
	numRecords := len(mock.contentByUID)
	mock.mu.Unlock()
	if numRecords != 2 {
		t.Fatalf("expected 2 remote records, got %d", numRecords)
	}
	foundLocal := false
	mock.mu.Lock()
	for _, content := range mock.contentByUID {
		if string(content) == "local content" {
			foundLocal = true
			break
		}
	}
	mock.mu.Unlock()
	if !foundLocal {
		t.Fatal("uploaded local content not found on mock server")
	}
}

func TestExecutePlanParallelDownloads(t *testing.T) {
	t.Parallel()
	const numFiles = 200

	mock := newMockSyncServer(t)
	for i := range numFiles {
		path := fmt.Sprintf("file-%04d.md", i)
		content := fmt.Appendf(nil, "content for file %d\n", i)
		mock.addRecord(path, int64(i+1), content)
	}

	server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	defer server.Close()

	vault := t.TempDir()
	wsURL := "ws" + server.URL[4:]

	currentLocal := map[string]model.FileRecord{}
	previousLocal := map[string]model.FileRecord{}
	currentRemote := mock.cloneRecordsByPath()
	previousRemote := map[string]model.FileRecord{}

	plan := buildPlan(currentLocal, previousLocal, currentRemote, previousRemote, ".obsidian")
	if len(plan) != numFiles {
		t.Fatalf("expected %d actions, got %d", numFiles, len(plan))
	}

	e := &Engine{
		Config: model.SyncConfig{
			VaultPath:           vault,
			Host:                wsURL,
			DownloadConcurrency: 10,
		},
		Logger: testLogger(),
	}

	mainConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mainConn.Close()

	if err := mainConn.WriteJSON(map[string]any{"op": "init"}); err != nil {
		t.Fatal(err)
	}
	for {
		var msg map[string]any
		if err := mainConn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		if msg["op"] == "ready" {
			break
		}
	}

	ctx := context.Background()
	session := newRemoteSession(mainConn, currentRemote, 1, ctx, nil, testLogger(), nil)
	if err := e.executePlan(ctx, plan, currentLocal, previousRemote, previousLocal, session); err != nil {
		t.Fatal(err)
	}

	for i := range numFiles {
		path := fmt.Sprintf("file-%04d.md", i)
		expected := fmt.Sprintf("content for file %d\n", i)
		data, err := os.ReadFile(filepath.Join(vault, path))
		if err != nil {
			t.Fatalf("missing downloaded file %s: %v", path, err)
		}
		if string(data) != expected {
			t.Fatalf("file %s content mismatch: got %q, want %q", path, string(data), expected)
		}
	}
}

func TestExecutePlanParallelSmallSync(t *testing.T) {
	t.Parallel()
	mock := newMockSyncServer(t)
	mock.addRecord("a.md", 1, []byte("file a"))
	mock.addRecord("b.md", 2, []byte("file b"))
	mock.addRecord("c.md", 3, []byte("file c"))

	server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	defer server.Close()

	vault := t.TempDir()
	wsURL := "ws" + server.URL[4:]

	currentLocal := map[string]model.FileRecord{}
	previousLocal := map[string]model.FileRecord{}
	currentRemote := mock.cloneRecordsByPath()
	previousRemote := map[string]model.FileRecord{}

	plan := buildPlan(currentLocal, previousLocal, currentRemote, previousRemote, ".obsidian")
	if len(plan) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(plan))
	}

	e := &Engine{
		Config: model.SyncConfig{
			VaultPath:           vault,
			Host:                wsURL,
			DownloadConcurrency: 10,
		},
		Logger: testLogger(),
	}

	mainConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mainConn.Close()

	if err := mainConn.WriteJSON(map[string]any{"op": "init"}); err != nil {
		t.Fatal(err)
	}
	for {
		var msg map[string]any
		if err := mainConn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		if msg["op"] == "ready" {
			break
		}
	}

	ctx := context.Background()
	session := newRemoteSession(mainConn, currentRemote, 1, ctx, nil, testLogger(), nil)

	if err := e.executePlan(ctx, plan, currentLocal, previousRemote, previousLocal, session); err != nil {
		t.Fatal(err)
	}

	for name, expected := range map[string]string{
		"a.md": "file a",
		"b.md": "file b",
		"c.md": "file c",
	} {
		data, err := os.ReadFile(filepath.Join(vault, name))
		if err != nil {
			t.Fatalf("missing downloaded file %s: %v", name, err)
		}
		if string(data) != expected {
			t.Fatalf("file %s content mismatch: got %q, want %q", name, string(data), expected)
		}
	}
}

func TestPushRelatedPath(t *testing.T) {
	t.Parallel()
	mock := newMockSyncServer(t)

	server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{"op": "init"}); err != nil {
		t.Fatal(err)
	}
	for {
		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		if msg["op"] == "ready" {
			break
		}
	}

	ctx := context.Background()
	session := newRemoteSession(conn, make(map[string]model.FileRecord), 1, ctx, nil, testLogger(), nil)

	// Push with PreviousPath set → relatedpath should be present.
	record := model.FileRecord{
		Path:         "new.md",
		Hash:         "abc",
		Size:         0,
		MTime:        1000,
		PreviousPath: "old.md",
	}
	if err := session.push(record, []byte{}); err != nil {
		t.Fatalf("push with PreviousPath failed: %v", err)
	}

	mock.mu.Lock()
	if len(mock.pushMsgs) != 1 {
		t.Fatalf("expected 1 push message, got %d", len(mock.pushMsgs))
	}
	msg := mock.pushMsgs[0]
	mock.mu.Unlock()

	rp, ok := msg["relatedpath"]
	if !ok {
		t.Fatal("expected relatedpath in push message when PreviousPath is set")
	}
	if rp != "old.md" {
		t.Fatalf("expected relatedpath='old.md', got %v", rp)
	}

	// Push with empty PreviousPath → relatedpath should NOT be present.
	record2 := model.FileRecord{
		Path:         "other.md",
		Hash:         "def",
		Size:         0,
		MTime:        2000,
		PreviousPath: "",
	}
	if err := session.push(record2, []byte{}); err != nil {
		t.Fatalf("push with empty PreviousPath failed: %v", err)
	}

	mock.mu.Lock()
	if len(mock.pushMsgs) != 2 {
		t.Fatalf("expected 2 push messages, got %d", len(mock.pushMsgs))
	}
	msg2 := mock.pushMsgs[1]
	mock.mu.Unlock()

	if _, exists := msg2["relatedpath"]; exists {
		t.Fatal("expected no relatedpath in push message when PreviousPath is empty")
	}

	// Push with invalid PreviousPath (starts with /) → relatedpath should NOT be present.
	record3 := model.FileRecord{
		Path:         "third.md",
		Hash:         "ghi",
		Size:         0,
		MTime:        3000,
		PreviousPath: "/absolute/path.md",
	}
	if err := session.push(record3, []byte{}); err != nil {
		t.Fatalf("push with absolute PreviousPath failed: %v", err)
	}

	mock.mu.Lock()
	if len(mock.pushMsgs) != 3 {
		t.Fatalf("expected 3 push messages, got %d", len(mock.pushMsgs))
	}
	msg3 := mock.pushMsgs[2]
	mock.mu.Unlock()

	if _, exists := msg3["relatedpath"]; exists {
		t.Fatal("expected no relatedpath in push message when PreviousPath starts with /")
	}

	// Push with invalid PreviousPath (contains ..) → relatedpath should NOT be present.
	record4 := model.FileRecord{
		Path:         "fourth.md",
		Hash:         "jkl",
		Size:         0,
		MTime:        4000,
		PreviousPath: "../escape.md",
	}
	if err := session.push(record4, []byte{}); err != nil {
		t.Fatalf("push with PreviousPath containing .. failed: %v", err)
	}

	mock.mu.Lock()
	if len(mock.pushMsgs) != 4 {
		t.Fatalf("expected 4 push messages, got %d", len(mock.pushMsgs))
	}
	msg4 := mock.pushMsgs[3]
	mock.mu.Unlock()

	if _, exists := msg4["relatedpath"]; exists {
		t.Fatal("expected no relatedpath in push message when PreviousPath contains ..")
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

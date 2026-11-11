package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/Belphemur/obsidian-headless/internal/model"
)

func planRecord(path, hash string, mtime int64) model.FileRecord {
	return model.FileRecord{Path: path, Hash: hash, MTime: mtime, Size: int64(len(hash))}
}

func planRecords(records ...model.FileRecord) map[string]model.FileRecord {
	out := map[string]model.FileRecord{}
	for _, r := range records {
		out[r.Path] = r
	}
	return out
}

// TestBuildPlanForMode checks what each sync mode plans for every kind of
// change. In pull and mirror the remote vault must never be written to.
func TestBuildPlanForMode(t *testing.T) {
	t.Parallel()
	deleted := func(path string) model.FileRecord {
		r := planRecord(path, "", 50)
		r.Deleted = true
		return r
	}
	folder := model.FileRecord{Path: "new-folder", Folder: true}

	up := func(p string) syncAction { return syncAction{Path: p, Kind: syncActionUpload} }
	down := func(p string) syncAction { return syncAction{Path: p, Kind: syncActionDownload} }
	delLocal := func(p string) syncAction { return syncAction{Path: p, Kind: syncActionDeleteLocal} }
	delRemote := func(p string) syncAction { return syncAction{Path: p, Kind: syncActionDeleteRemote} }

	cases := []struct {
		name                                                       string
		currentLocal, previousLocal, currentRemote, previousRemote map[string]model.FileRecord
		renames                                                    map[string]string
		bidirectional, pull, mirror                                []syncAction
	}{
		{
			name:          "file created locally",
			currentLocal:  planRecords(planRecord("new.md", "h1", 10)),
			bidirectional: []syncAction{up("new.md")},
			mirror:        []syncAction{delLocal("new.md")},
		},
		{
			name:           "file modified locally",
			currentLocal:   planRecords(planRecord("a.md", "h2", 20)),
			previousLocal:  planRecords(planRecord("a.md", "h1", 10)),
			currentRemote:  planRecords(planRecord("a.md", "h1", 10)),
			previousRemote: planRecords(planRecord("a.md", "h1", 10)),
			bidirectional:  []syncAction{up("a.md")},
			mirror:         []syncAction{down("a.md")},
		},
		{
			name:           "file deleted locally",
			previousLocal:  planRecords(planRecord("a.md", "h1", 10)),
			currentRemote:  planRecords(planRecord("a.md", "h1", 10)),
			previousRemote: planRecords(planRecord("a.md", "h1", 10)),
			bidirectional:  []syncAction{delRemote("a.md")},
			mirror:         []syncAction{down("a.md")},
		},
		{
			name:           "note changed on both sides",
			currentLocal:   planRecords(planRecord("a.md", "h2", 20)),
			previousLocal:  planRecords(planRecord("a.md", "h1", 10)),
			currentRemote:  planRecords(planRecord("a.md", "h3", 15)),
			previousRemote: planRecords(planRecord("a.md", "h1", 10)),
			bidirectional:  []syncAction{{Path: "a.md", Kind: syncActionMergeText}},
			pull:           []syncAction{down("a.md")},
			mirror:         []syncAction{down("a.md")},
		},
		{
			name:           "attachment changed on both sides, local newer",
			currentLocal:   planRecords(planRecord("a.png", "h2", 20)),
			previousLocal:  planRecords(planRecord("a.png", "h1", 10)),
			currentRemote:  planRecords(planRecord("a.png", "h3", 15)),
			previousRemote: planRecords(planRecord("a.png", "h1", 10)),
			bidirectional:  []syncAction{up("a.png")},
			pull:           []syncAction{down("a.png")},
			mirror:         []syncAction{down("a.png")},
		},
		{
			name:           "deleted on the server, modified locally",
			currentLocal:   planRecords(planRecord("a.md", "h2", 20)),
			previousLocal:  planRecords(planRecord("a.md", "h1", 10)),
			currentRemote:  planRecords(deleted("a.md")),
			previousRemote: planRecords(planRecord("a.md", "h1", 10)),
			bidirectional:  []syncAction{up("a.md")},
			pull:           []syncAction{delLocal("a.md")},
			mirror:         []syncAction{delLocal("a.md")},
		},
		{
			name:           "changed on the server, deleted locally",
			previousLocal:  planRecords(planRecord("a.md", "h1", 30)),
			currentRemote:  planRecords(planRecord("a.md", "h3", 15)),
			previousRemote: planRecords(planRecord("a.md", "h1", 10)),
			bidirectional:  []syncAction{delRemote("a.md")},
			pull:           []syncAction{down("a.md")},
			mirror:         []syncAction{down("a.md")},
		},
		{
			name:           "changed on the server only",
			currentLocal:   planRecords(planRecord("a.md", "h1", 10)),
			previousLocal:  planRecords(planRecord("a.md", "h1", 10)),
			currentRemote:  planRecords(planRecord("a.md", "h3", 15)),
			previousRemote: planRecords(planRecord("a.md", "h1", 10)),
			bidirectional:  []syncAction{down("a.md")},
			pull:           []syncAction{down("a.md")},
			mirror:         []syncAction{down("a.md")},
		},
		{
			name:           "deleted on the server only",
			currentLocal:   planRecords(planRecord("a.md", "h1", 10)),
			previousLocal:  planRecords(planRecord("a.md", "h1", 10)),
			currentRemote:  planRecords(deleted("a.md")),
			previousRemote: planRecords(planRecord("a.md", "h1", 10)),
			bidirectional:  []syncAction{delLocal("a.md")},
			pull:           []syncAction{delLocal("a.md")},
			mirror:         []syncAction{delLocal("a.md")},
		},
		{
			name:           "file created locally over an old server tombstone",
			currentLocal:   planRecords(planRecord("a.md", "h2", 20)),
			currentRemote:  planRecords(deleted("a.md")),
			previousRemote: planRecords(deleted("a.md")),
			bidirectional:  []syncAction{up("a.md")},
			mirror:         []syncAction{delLocal("a.md")},
		},
		{
			name:          "folder created locally",
			currentLocal:  planRecords(folder),
			bidirectional: []syncAction{up("new-folder")},
		},
		{
			name:           "file renamed locally",
			currentLocal:   planRecords(planRecord("new.md", "h1", 10)),
			previousLocal:  planRecords(planRecord("old.md", "h1", 10)),
			currentRemote:  planRecords(planRecord("old.md", "h1", 10)),
			previousRemote: planRecords(planRecord("old.md", "h1", 10)),
			renames:        map[string]string{"old.md": "new.md"},
			bidirectional:  []syncAction{{Path: "new.md", Kind: syncActionUpload, RelatedPath: "old.md"}},
			mirror:         []syncAction{delLocal("new.md"), down("old.md")},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			for _, m := range []struct {
				mode string
				want []syncAction
			}{{"", c.bidirectional}, {"bidirectional", c.bidirectional}, {syncModePull, c.pull}, {syncModeMirror, c.mirror}} {
				got := buildPlanForMode(m.mode, c.currentLocal, c.previousLocal, c.currentRemote, c.previousRemote, ".obsidian", c.renames)
				if len(got) == 0 && len(m.want) == 0 {
					continue
				}
				if !reflect.DeepEqual(got, m.want) {
					t.Errorf("mode %q: got %+v, want %+v", m.mode, got, m.want)
				}
				if isReadOnlyMode(m.mode) {
					for _, a := range got {
						if a.Kind != syncActionDownload && a.Kind != syncActionDeleteLocal {
							t.Errorf("mode %q planned a write to the remote vault: %s %q", m.mode, a.Kind, a.Path)
						}
					}
				}
			}
			// buildPlan is the bidirectional plan.
			if got := buildPlan(c.currentLocal, c.previousLocal, c.currentRemote, c.previousRemote, ".obsidian", c.renames); len(got)+len(c.bidirectional) > 0 && !reflect.DeepEqual(got, c.bidirectional) {
				t.Errorf("buildPlan: got %+v, want %+v", got, c.bidirectional)
			}
		})
	}
}

func TestIsReadOnlyMode(t *testing.T) {
	t.Parallel()
	for mode, want := range map[string]bool{"pull": true, "mirror": true, "bidirectional": false, "": false} {
		if got := isReadOnlyMode(mode); got != want {
			t.Errorf("isReadOnlyMode(%q) = %v, want %v", mode, got, want)
		}
	}
}

// A read-only session refuses to write before anything is sent: this one has
// no connection at all.
func TestReadOnlySessionRefusesWrites(t *testing.T) {
	t.Parallel()
	s := &remoteSession{readOnly: true, Logger: testLogger()}
	if err := s.push(model.FileRecord{Path: "note.md"}, []byte("local edit")); err == nil {
		t.Error("push was not refused")
	}
	if err := s.delete("note.md"); err == nil {
		t.Error("remote delete was not refused")
	}
}

// End to end against the mock server: a local-only file and a remote-only file.
// Neither read-only mode uploads anything; mirror also removes the local-only file.
func TestExecutePlanReadOnlyModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{syncModePull, syncModeMirror} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
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
			e := &Engine{Config: model.SyncConfig{VaultPath: vault, Host: wsURL, SyncMode: mode}, Logger: testLogger()}
			currentLocal := map[string]model.FileRecord{"local.md": {Path: "local.md", Hash: "localhash", Size: 13, MTime: 1000}}
			previousLocal := map[string]model.FileRecord{}
			previousRemote := map[string]model.FileRecord{}

			plan := buildPlanForMode(mode, currentLocal, previousLocal, remote, previousRemote, ".obsidian", nil)
			ctx := context.Background()
			session := newRemoteSession(conn, remote, 1, ctx, nil, testLogger(), nil)
			session.readOnly = isReadOnlyMode(mode)
			if err := e.executePlan(ctx, plan, currentLocal, previousRemote, previousLocal, session, session.version); err != nil {
				t.Fatal(err)
			}

			if data, err := os.ReadFile(filepath.Join(vault, "remote.md")); err != nil || string(data) != "remote content" {
				t.Fatalf("remote.md was not downloaded: %q, %v", data, err)
			}
			_, statErr := os.Stat(filepath.Join(vault, "local.md"))
			if mode == syncModeMirror && !os.IsNotExist(statErr) {
				t.Error("mirror: the file that is not on the server is still there")
			}
			if mode == syncModePull && statErr != nil {
				t.Errorf("pull: the local file should be left alone: %v", statErr)
			}

			mock.mu.Lock()
			defer mock.mu.Unlock()
			if len(mock.contentByUID) != 1 {
				t.Fatalf("the remote vault changed: %d records, want 1", len(mock.contentByUID))
			}
			for _, content := range mock.contentByUID {
				if string(content) != "remote content" {
					t.Fatalf("unexpected content on the server: %q", content)
				}
			}
		})
	}
}

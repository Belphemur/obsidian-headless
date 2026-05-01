package storage

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
)

func TestMigrationFromV1WithExistingData(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.db")

	// Create a v1-style database with the old JSON blob schema
	oldDB, err := createV1Database(path)
	if err != nil {
		t.Fatal(err)
	}

	// Insert records in old JSON format
	oldLocal := model.FileRecord{Path: "note.md", Hash: "hash1", Size: 100, MTime: 1000, CTime: 900}
	oldRemote := model.FileRecord{Path: "note.md", Hash: "hash2", Size: 200, MTime: 2000, CTime: 1900, UID: 7, Device: "dev", User: "usr"}

	localJSON, _ := json.Marshal(oldLocal)
	remoteJSON, _ := json.Marshal(oldRemote)
	if _, err := oldDB.Exec(`INSERT INTO local_files (path, data) VALUES (?, ?)`, "note.md", string(localJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := oldDB.Exec(`INSERT INTO server_files (path, data) VALUES (?, ?)`, "note.md", string(remoteJSON)); err != nil {
		t.Fatal(err)
	}
	oldDB.Close()

	// Open with new code — should migrate and return typed data
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	loadedLocal, err := store.LoadLocalFiles()
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := loadedLocal["note.md"]; !ok || r.Hash != "hash1" || r.Size != 100 || r.MTime != 1000 || r.CTime != 900 {
		t.Fatalf("local migration failed: %+v", loadedLocal["note.md"])
	}

	loadedRemote, err := store.LoadServerFiles()
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := loadedRemote["note.md"]; !ok || r.Hash != "hash2" || r.Size != 200 || r.MTime != 2000 || r.CTime != 1900 || r.UID != 7 || r.Device != "dev" || r.User != "usr" {
		t.Fatalf("remote migration failed: %+v", loadedRemote["note.md"])
	}
}

func createV1Database(path string) (*sql.DB, error) {
	dsn := path +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=cache_size(-64000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS local_files (path TEXT PRIMARY KEY, data TEXT NOT NULL)`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS server_files (path TEXT PRIMARY KEY, data TEXT NOT NULL)`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO meta (key, value) VALUES ('schema_version', '1')`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

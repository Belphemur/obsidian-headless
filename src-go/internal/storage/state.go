package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
)

type StateStore struct {
	db *sql.DB
}

func Open(path string) (*StateStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &StateStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *StateStore) Close() error {
	return s.db.Close()
}

func (s *StateStore) init() error {
	statements := []string{
		`PRAGMA journal_mode = WAL;`,
		`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT);`,
		`CREATE TABLE IF NOT EXISTS local_files (path TEXT PRIMARY KEY, data TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS server_files (path TEXT PRIMARY KEY, data TEXT NOT NULL);`,
		`INSERT OR IGNORE INTO meta (key, value) VALUES ('schema_version', '1');`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *StateStore) Version() (int64, error) {
	return s.metaInt("version")
}

func (s *StateStore) SetVersion(version int64) error {
	return s.setMeta("version", fmt.Sprintf("%d", version))
}

func (s *StateStore) Initial() (bool, error) {
	value, err := s.metaValue("initial")
	if err != nil {
		return false, err
	}
	return value != "false", nil
}

func (s *StateStore) SetInitial(initial bool) error {
	if initial {
		return s.setMeta("initial", "true")
	}
	return s.setMeta("initial", "false")
}

func (s *StateStore) LoadLocalFiles() (map[string]model.FileRecord, error) {
	return s.loadTable("local_files")
}

func (s *StateStore) ReplaceLocalFiles(records map[string]model.FileRecord) error {
	return s.replaceTable("local_files", records)
}

func (s *StateStore) LoadServerFiles() (map[string]model.FileRecord, error) {
	return s.loadTable("server_files")
}

func (s *StateStore) ReplaceServerFiles(records map[string]model.FileRecord) error {
	return s.replaceTable("server_files", records)
}

func (s *StateStore) metaValue(key string) (string, error) {
	var value sql.NullString
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	if !value.Valid {
		return "", nil
	}
	return value.String, nil
}

func (s *StateStore) metaInt(key string) (int64, error) {
	value, err := s.metaValue(key)
	if err != nil || value == "" {
		return 0, err
	}
	var parsed int64
	_, err = fmt.Sscanf(value, "%d", &parsed)
	return parsed, err
}

func (s *StateStore) setMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)`, key, value)
	return err
}

func (s *StateStore) loadTable(table string) (map[string]model.FileRecord, error) {
	rows, err := s.db.Query(`SELECT path, data FROM ` + table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]model.FileRecord{}
	for rows.Next() {
		var path string
		var payload string
		if err := rows.Scan(&path, &payload); err != nil {
			return nil, err
		}
		var record model.FileRecord
		if err := json.Unmarshal([]byte(payload), &record); err != nil {
			return nil, err
		}
		result[path] = record
	}
	return result, rows.Err()
}

func (s *StateStore) replaceTable(table string, records map[string]model.FileRecord) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.Exec(`DELETE FROM ` + table); err != nil {
		return err
	}
	statement, err := tx.Prepare(`INSERT INTO ` + table + ` (path, data) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for path, record := range records {
		payload, marshalErr := json.Marshal(record)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = statement.Exec(path, string(payload)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

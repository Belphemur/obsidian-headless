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
	validatedTable, err := validateTableName(table)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT path, data FROM ` + validatedTable)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
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

func (s *StateStore) replaceTable(table string, records map[string]model.FileRecord) (retErr error) {
	validatedTable, err := validateTableName(table)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			_ = tx.Rollback()
		}
	}()

	// Upsert every record.
	upsertSQL := `INSERT INTO ` + validatedTable + ` (path, data) VALUES (?, ?) ON CONFLICT(path) DO UPDATE SET data = excluded.data`
	stmt, err := tx.Prepare(upsertSQL)
	if err != nil {
		retErr = err
		return
	}
	defer func() {
		_ = stmt.Close()
	}()

	for path, record := range records {
		payload, err := json.Marshal(record)
		if err != nil {
			retErr = err
			return
		}
		if _, err = stmt.Exec(path, string(payload)); err != nil {
			retErr = err
			return
		}
	}

	// Delete orphans — entries in the DB that are no longer in the new set.
	rows, err := tx.Query(`SELECT path FROM ` + validatedTable)
	if err != nil {
		retErr = err
		return
	}
	var toDelete []string
	for rows.Next() {
		var p string
		if scanErr := rows.Scan(&p); scanErr != nil {
			_ = rows.Close()
			retErr = scanErr
			return
		}
		if _, exists := records[p]; !exists {
			toDelete = append(toDelete, p)
		}
	}
	if closeErr := rows.Close(); closeErr != nil {
		retErr = closeErr
		return
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		retErr = rowsErr
		return
	}
	for _, p := range toDelete {
		if _, err = tx.Exec(`DELETE FROM `+validatedTable+` WHERE path = ?`, p); err != nil {
			retErr = err
			return
		}
	}

	retErr = tx.Commit()
	return
}

func validateTableName(table string) (string, error) {
	switch table {
	case "local_files", "server_files":
		return table, nil
	default:
		return "", fmt.Errorf("invalid table name %q", table)
	}
}

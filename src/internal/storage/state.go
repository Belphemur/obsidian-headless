package storage

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	_ "modernc.org/sqlite"

	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
)

//go:embed migrations/sqlite/*.sql
var migrationsFS embed.FS

type StateStore struct {
	db *sql.DB
}

func Open(path string) (*StateStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// PRAGMAs set via DSN so every pooled connection inherits them.
	dsn := path +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=cache_size(-64000)" +
		"&_pragma=temp_store(MEMORY)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	store := &StateStore{db: db}

	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return store, nil
}

func (s *StateStore) Close() error {
	return s.db.Close()
}

func (s *StateStore) init() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT);`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *StateStore) migrate() error {
	driver, err := WithInstance(s.db)
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}

	subFS, err := fs.Sub(migrationsFS, "migrations/sqlite")
	if err != nil {
		return fmt.Errorf("create migrations sub-filesystem: %w", err)
	}

	src, err := iofs.New(subFS, ".")
	if err != nil {
		return fmt.Errorf("create iofs source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "modernc-sqlite3", driver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
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

func (s *StateStore) UpsertLocalFile(record model.FileRecord) error {
	return s.upsertRecord("local_files", record)
}

func (s *StateStore) UpsertServerFile(record model.FileRecord) error {
	return s.upsertRecord("server_files", record)
}

func (s *StateStore) DeleteLocalFile(path string) error {
	return s.deleteRecord("local_files", path)
}

func (s *StateStore) DeleteServerFile(path string) error {
	return s.deleteRecord("server_files", path)
}

func (s *StateStore) SaveLocalDiff(upserts []model.FileRecord, deletes []string) (err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, rec := range upserts {
		if err = s.upsertInTx(tx, "local_files", rec); err != nil {
			return err
		}
	}
	for _, path := range deletes {
		if err = s.deleteInTx(tx, "local_files", path); err != nil {
			return err
		}
	}

	err = tx.Commit()
	return err
}

func (s *StateStore) SaveServerDiff(upserts []model.FileRecord, deletes []string) (err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, rec := range upserts {
		if err = s.upsertInTx(tx, "server_files", rec); err != nil {
			return err
		}
	}
	for _, path := range deletes {
		if err = s.deleteInTx(tx, "server_files", path); err != nil {
			return err
		}
	}

	err = tx.Commit()
	return err
}

func (s *StateStore) SaveStateAtomic(version int64, initial bool,
	localUpserts []model.FileRecord, localDeletes []string,
	remoteUpserts []model.FileRecord, remoteDeletes []string) (err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = s.setMetaInTx(tx, "version", fmt.Sprintf("%d", version)); err != nil {
		return err
	}

	for _, rec := range localUpserts {
		if err = s.upsertInTx(tx, "local_files", rec); err != nil {
			return err
		}
	}
	for _, path := range localDeletes {
		if err = s.deleteInTx(tx, "local_files", path); err != nil {
			return err
		}
	}

	for _, rec := range remoteUpserts {
		if err = s.upsertInTx(tx, "server_files", rec); err != nil {
			return err
		}
	}
	for _, path := range remoteDeletes {
		if err = s.deleteInTx(tx, "server_files", path); err != nil {
			return err
		}
	}

	initVal := "false"
	if initial {
		initVal = "true"
	}
	if err = s.setMetaInTx(tx, "initial", initVal); err != nil {
		return err
	}

	err = tx.Commit()
	return err
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

func (s *StateStore) setMetaInTx(tx *sql.Tx, key, value string) error {
	_, err := tx.Exec(`INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)`, key, value)
	return err
}

func (s *StateStore) loadTable(table string) (map[string]model.FileRecord, error) {
	validatedTable, err := validateTableName(table)
	if err != nil {
		return nil, err
	}
	if validatedTable == "local_files" {
		return s.loadLocalFiles()
	}
	return s.loadServerFiles()
}

func (s *StateStore) loadLocalFiles() (map[string]model.FileRecord, error) {
	rows, err := s.db.Query(`SELECT path, size, hash, ctime, mtime, folder, deleted FROM local_files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]model.FileRecord)
	for rows.Next() {
		var rec model.FileRecord
		if err := rows.Scan(&rec.Path, &rec.Size, &rec.Hash, &rec.CTime, &rec.MTime, &rec.Folder, &rec.Deleted); err != nil {
			return nil, err
		}
		result[rec.Path] = rec
	}
	return result, rows.Err()
}

func (s *StateStore) loadServerFiles() (map[string]model.FileRecord, error) {
	rows, err := s.db.Query(`SELECT path, size, hash, ctime, mtime, folder, deleted, uid, device, user FROM server_files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]model.FileRecord)
	for rows.Next() {
		var rec model.FileRecord
		if err := rows.Scan(&rec.Path, &rec.Size, &rec.Hash, &rec.CTime, &rec.MTime, &rec.Folder, &rec.Deleted, &rec.UID, &rec.Device, &rec.User); err != nil {
			return nil, err
		}
		result[rec.Path] = rec
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

	isServer := validatedTable == "server_files"

	var upsertSQL string
	if isServer {
		upsertSQL = `INSERT INTO server_files (path, size, hash, ctime, mtime, folder, deleted, uid, device, user, raw)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, jsonb(?))
			ON CONFLICT(path) DO UPDATE SET
				size = excluded.size,
				hash = excluded.hash,
				ctime = excluded.ctime,
				mtime = excluded.mtime,
				folder = excluded.folder,
				deleted = excluded.deleted,
				uid = excluded.uid,
				device = excluded.device,
				user = excluded.user,
				raw = excluded.raw`
	} else {
		upsertSQL = `INSERT INTO local_files (path, size, hash, ctime, mtime, folder, deleted, raw)
			VALUES (?, ?, ?, ?, ?, ?, ?, jsonb(?))
			ON CONFLICT(path) DO UPDATE SET
				size = excluded.size,
				hash = excluded.hash,
				ctime = excluded.ctime,
				mtime = excluded.mtime,
				folder = excluded.folder,
				deleted = excluded.deleted,
				raw = excluded.raw`
	}
	stmt, err := tx.Prepare(upsertSQL)
	if err != nil {
		retErr = err
		return
	}
	defer stmt.Close()

	for path, record := range records {
		payload, err := json.Marshal(record)
		if err != nil {
			retErr = err
			return
		}
		jsonStr := string(payload)
		if isServer {
			_, err = stmt.Exec(path, record.Size, record.Hash, record.CTime, record.MTime, record.Folder, record.Deleted, record.UID, record.Device, record.User, jsonStr)
		} else {
			_, err = stmt.Exec(path, record.Size, record.Hash, record.CTime, record.MTime, record.Folder, record.Deleted, jsonStr)
		}
		if err != nil {
			retErr = err
			return
		}
	}

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

func (s *StateStore) upsertRecord(table string, record model.FileRecord) (err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = s.upsertInTx(tx, table, record); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (s *StateStore) upsertInTx(tx *sql.Tx, table string, record model.FileRecord) error {
	validatedTable, err := validateTableName(table)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	jsonStr := string(payload)

	if validatedTable == "server_files" {
		_, err = tx.Exec(`INSERT INTO server_files (path, size, hash, ctime, mtime, folder, deleted, uid, device, user, raw)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, jsonb(?))
			ON CONFLICT(path) DO UPDATE SET
				size = excluded.size,
				hash = excluded.hash,
				ctime = excluded.ctime,
				mtime = excluded.mtime,
				folder = excluded.folder,
				deleted = excluded.deleted,
				uid = excluded.uid,
				device = excluded.device,
				user = excluded.user,
				raw = excluded.raw`,
			record.Path, record.Size, record.Hash, record.CTime, record.MTime, record.Folder, record.Deleted, record.UID, record.Device, record.User, jsonStr)
	} else {
		_, err = tx.Exec(`INSERT INTO local_files (path, size, hash, ctime, mtime, folder, deleted, raw)
			VALUES (?, ?, ?, ?, ?, ?, ?, jsonb(?))
			ON CONFLICT(path) DO UPDATE SET
				size = excluded.size,
				hash = excluded.hash,
				ctime = excluded.ctime,
				mtime = excluded.mtime,
				folder = excluded.folder,
				deleted = excluded.deleted,
				raw = excluded.raw`,
			record.Path, record.Size, record.Hash, record.CTime, record.MTime, record.Folder, record.Deleted, jsonStr)
	}
	return err
}

func (s *StateStore) deleteInTx(tx *sql.Tx, table, path string) error {
	validatedTable, err := validateTableName(table)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM `+validatedTable+` WHERE path = ?`, path)
	return err
}

func (s *StateStore) deleteRecord(table, path string) error {
	validatedTable, err := validateTableName(table)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM `+validatedTable+` WHERE path = ?`, path)
	return err
}

func validateTableName(table string) (string, error) {
	switch table {
	case "local_files", "server_files":
		return table, nil
	default:
		return "", fmt.Errorf("invalid table name %q", table)
	}
}

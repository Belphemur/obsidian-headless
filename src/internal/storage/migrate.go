package storage

import (
	"database/sql"
	"fmt"
	"io"
	"sync"

	"github.com/golang-migrate/migrate/v4/database"
)

func init() {
	database.Register("modernc-sqlite3", &sqliteDriver{})
}

type sqliteDriver struct {
	db *sql.DB
	mu sync.Mutex
}

func (d *sqliteDriver) Open(url string) (database.Driver, error) {
	db, err := sql.Open("sqlite", url)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return WithInstance(db)
}

func (d *sqliteDriver) Close() error {
	return d.db.Close()
}

func (d *sqliteDriver) Lock() error {
	d.mu.Lock()
	return nil
}

func (d *sqliteDriver) Unlock() error {
	d.mu.Unlock()
	return nil
}

func (d *sqliteDriver) Run(reader io.Reader) error {
	script, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(string(script)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (d *sqliteDriver) SetVersion(version int, dirty bool) (err error) {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`DELETE FROM schema_migrations`); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (?, ?)`, version, dirty); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (d *sqliteDriver) Version() (version int, dirty bool, err error) {
	row := d.db.QueryRow(`SELECT version, dirty FROM schema_migrations LIMIT 1`)
	if err := row.Scan(&version, &dirty); err != nil {
		if err == sql.ErrNoRows {
			return database.NilVersion, false, nil
		}
		return 0, false, err
	}
	return version, dirty, nil
}

func (d *sqliteDriver) Drop() error {
	rows, err := d.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name != 'schema_migrations'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, table := range tables {
		if _, err := d.db.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS %q`, table)); err != nil {
			return err
		}
	}
	return nil
}

func WithInstance(db *sql.DB) (database.Driver, error) {
	if err := ensureVersionTable(db); err != nil {
		return nil, err
	}
	return &sqliteDriver{db: db}, nil
}

func ensureVersionTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER NOT NULL, dirty BOOLEAN NOT NULL)`)
	return err
}

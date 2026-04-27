package storage

import (
	"database/sql"
	"os"
	"path/filepath"
)

// CredentialStore provides encrypted secret storage in a dedicated SQLite database.
type CredentialStore struct {
	db *sql.DB
}

// OpenCredentials opens a CredentialStore at the given path, creating the database
// and secrets table if they do not exist.
func OpenCredentials(path string) (*CredentialStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &CredentialStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// Close closes the underlying database connection.
func (c *CredentialStore) Close() error {
	return c.db.Close()
}

func (c *CredentialStore) init() error {
	statements := []string{
		`PRAGMA journal_mode = WAL;`,
		`CREATE TABLE IF NOT EXISTS secrets (name TEXT PRIMARY KEY, value TEXT NOT NULL);`,
	}
	for _, stmt := range statements {
		if _, err := c.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// SetSecret stores a plaintext secret value under name, encrypted with AES-GCM
// using masterKey (must be 32 bytes).
func (c *CredentialStore) SetSecret(name string, plaintext string, masterKey []byte) error {
	enc, err := encrypt(masterKey, []byte(plaintext))
	if err != nil {
		return err
	}
	_, err = c.db.Exec(`INSERT OR REPLACE INTO secrets (name, value) VALUES (?, ?)`, name, enc)
	return err
}

// GetSecret retrieves and decrypts a secret previously stored with SetSecret.
// Returns ("", nil) when the secret does not exist.
func (c *CredentialStore) GetSecret(name string, masterKey []byte) (string, error) {
	var value sql.NullString
	if err := c.db.QueryRow(`SELECT value FROM secrets WHERE name = ?`, name).Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	if !value.Valid {
		return "", nil
	}
	plain, err := decrypt(masterKey, value.String)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// DeleteSecret removes a secret from the store.
func (c *CredentialStore) DeleteSecret(name string) error {
	_, err := c.db.Exec(`DELETE FROM secrets WHERE name = ?`, name)
	return err
}

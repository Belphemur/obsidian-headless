package config

import (
	"sync"

	"github.com/Belphemur/obsidian-headless/src-go/internal/storage"
	"github.com/rs/zerolog"
	"github.com/zalando/go-keyring"
)

const secretServiceName = "obsidian-headless"

// SecretStore provides unified access to OS keyring with encrypted-file fallback.
// All methods are safe to call concurrently.
type SecretStore struct {
	mu        sync.Mutex
	masterKey []byte
	fallback  *storage.StateStore
	logger    zerolog.Logger
}

// NewSecretStore creates a new SecretStore, loading or creating the master key
// needed for the encrypted-file fallback.
func NewSecretStore(logger zerolog.Logger) (*SecretStore, error) {
	masterKey, err := LoadOrCreateMasterKey()
	if err != nil {
		return nil, err
	}
	return &SecretStore{masterKey: masterKey, logger: logger}, nil
}

// Close closes the fallback database connection if it was opened.
func (s *SecretStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallback != nil {
		return s.fallback.Close()
	}
	return nil
}

// Get tries the OS keyring first. If the secret is not found or the keyring
// is unavailable, it falls back to the encrypted credentials.db.
func (s *SecretStore) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, err := keyring.Get(secretServiceName, key)
	if err == nil {
		return val, nil
	}
	s.logger.Debug().Str("key", key).Err(err).Msg("keyring get failed, falling back to encrypted db")
	// keyring unavailable or not found — fall back to credentials.db
	store, err := s.fallbackStore()
	if err != nil {
		return "", err
	}
	return store.GetSecret(key, s.masterKey)
}

// Set stores a secret in the OS keyring if available. On success, it also
// removes any stale copy from the fallback database. If the keyring fails,
// it stores in the encrypted credentials.db instead.
func (s *SecretStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := keyring.Set(secretServiceName, key, value)
	if err == nil {
		s.clearFallbackSecret(key)
		s.logger.Debug().Str("key", key).Msg("keyring set successfully stored secret")

		return nil
	}
	s.logger.Debug().Str("key", key).Err(err).Msg("keyring set failed, falling back to encrypted db")
	// keyring unavailable — fall back to credentials.db
	store, err := s.fallbackStore()
	if err != nil {
		return err
	}
	return store.SetSecret(key, value, s.masterKey)
}

// Delete removes a secret from both the keyring and the fallback database.
func (s *SecretStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := keyring.Delete(secretServiceName, key); err != nil {
		s.logger.Debug().Str("key", key).Err(err).Msg("keyring delete failed")
	}
	s.logger.Debug().Str("key", key).Msg("keyring delete successfull")

	s.clearFallbackSecret(key)
	return nil
}

func (s *SecretStore) fallbackStore() (*storage.StateStore, error) {
	if s.fallback != nil {
		return s.fallback, nil
	}
	dbPath, err := CredentialsDBPath()
	if err != nil {
		return nil, err
	}
	store, err := storage.Open(dbPath)
	if err != nil {
		return nil, err
	}
	s.fallback = store
	return store, nil
}

func (s *SecretStore) clearFallbackSecret(key string) {
	if s.fallback != nil {
		_ = s.fallback.SetSecret(key, "", s.masterKey)
		return
	}
	dbPath, err := CredentialsDBPath()
	if err != nil {
		return
	}
	store, err := storage.Open(dbPath)
	if err != nil {
		return
	}
	defer store.Close()
	_ = store.SetSecret(key, "", s.masterKey)
}

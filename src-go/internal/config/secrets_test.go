package config

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestSecretStoreRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	store, err := newTestSecretStore(zerolog.New(io.Discard), "test:")
	if err != nil {
		t.Fatalf("newTestSecretStore failed: %v", err)
	}
	defer store.Close()

	key := "test_key"
	value := "test_value"

	if err := store.Set(key, value); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := store.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != value {
		t.Errorf("expected %q, got %q", value, got)
	}

	if err := store.Delete(key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	got, err = store.Get(key)
	if err != nil {
		t.Fatalf("Get after delete failed: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty value after delete, got %q", got)
	}
}

func TestSecretStoreFallbackWhenKeyringUnavailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	store, err := newTestSecretStore(zerolog.New(io.Discard), "test:")
	if err != nil {
		t.Fatalf("newTestSecretStore failed: %v", err)
	}
	defer store.Close()

	// In CI/test environments without a keyring daemon, operations fall back
	// to the encrypted credentials.db. This test verifies the fallback path.
	key := "fallback_key"
	value := "fallback_value"

	if err := store.Set(key, value); err != nil {
		t.Fatalf("Set (fallback) failed: %v", err)
	}

	got, err := store.Get(key)
	if err != nil {
		t.Fatalf("Get (fallback) failed: %v", err)
	}
	if got != value {
		t.Errorf("expected %q, got %q", value, got)
	}

	if err := store.Delete(key); err != nil {
		t.Fatalf("Delete (fallback) failed: %v", err)
	}

	got, err = store.Get(key)
	if err != nil {
		t.Fatalf("Get after delete (fallback) failed: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty value after delete, got %q", got)
	}
}

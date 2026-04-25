package storage

import (
	"path/filepath"
	"testing"
)

func TestCredentialStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "creds.db")
	masterKey := make([]byte, 32)
	for i := range masterKey {
		masterKey[i] = byte(i)
	}

	store, err := OpenCredentials(path)
	if err != nil {
		t.Fatalf("OpenCredentials failed: %v", err)
	}
	defer store.Close()

	name := "my_secret"
	plaintext := "super_secret_value"

	if err := store.SetSecret(name, plaintext, masterKey); err != nil {
		t.Fatalf("SetSecret failed: %v", err)
	}

	got, err := store.GetSecret(name, masterKey)
	if err != nil {
		t.Fatalf("GetSecret failed: %v", err)
	}
	if got != plaintext {
		t.Errorf("expected %q, got %q", plaintext, got)
	}

	if err := store.DeleteSecret(name); err != nil {
		t.Fatalf("DeleteSecret failed: %v", err)
	}

	got, err = store.GetSecret(name, masterKey)
	if err != nil {
		t.Fatalf("GetSecret after delete failed: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty value after delete, got %q", got)
	}
}

func TestCredentialStoreGetMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "creds.db")
	masterKey := make([]byte, 32)

	store, err := OpenCredentials(path)
	if err != nil {
		t.Fatalf("OpenCredentials failed: %v", err)
	}
	defer store.Close()

	got, err := store.GetSecret("does_not_exist", masterKey)
	if err != nil {
		t.Fatalf("GetSecret missing failed: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty value for missing secret, got %q", got)
	}
}

func TestCredentialStoreWrongKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "creds.db")
	masterKey := make([]byte, 32)
	for i := range masterKey {
		masterKey[i] = byte(i)
	}
	wrongKey := make([]byte, 32)
	for i := range wrongKey {
		wrongKey[i] = byte(i + 1)
	}

	store, err := OpenCredentials(path)
	if err != nil {
		t.Fatalf("OpenCredentials failed: %v", err)
	}
	defer store.Close()

	if err := store.SetSecret("secret", "value", masterKey); err != nil {
		t.Fatalf("SetSecret failed: %v", err)
	}

	_, err = store.GetSecret("secret", wrongKey)
	if err == nil {
		t.Error("expected error decrypting with wrong key, got nil")
	}
}

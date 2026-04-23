package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	email := "test@example.com"
	password := "secret-password"

	err := SaveCredentials(email, password)
	if err != nil {
		t.Fatalf("SaveCredentials failed: %v", err)
	}

 LoadedEmail, LoadedPassword, err := LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials failed: %v", err)
	}
	if LoadedEmail != email {
		t.Errorf("expected email %q, got %q", email, LoadedEmail)
	}
	if LoadedPassword != password {
		t.Errorf("expected password %q, got %q", password, LoadedPassword)
	}

	err = ClearCredentials()
	if err != nil {
		t.Fatalf("ClearCredentials failed: %v", err)
	}

	clearedEmail, clearedPassword, err := LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials after clear failed: %v", err)
	}
	if clearedEmail != "" {
		t.Errorf("expected cleared email, got %q", clearedEmail)
	}
	if clearedPassword != "" {
		t.Errorf("expected cleared password, got %q", clearedPassword)
	}
}

func TestCredentialsPersistMasterKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	email := "test@example.com"
	password := "secret-password"

	err := SaveCredentials(email, password)
	if err != nil {
		t.Fatalf("SaveCredentials failed: %v", err)
	}

	masterKeyPath, err := MasterKeyPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(masterKeyPath); os.IsNotExist(err) {
		t.Error("master key was not created")
	}
}
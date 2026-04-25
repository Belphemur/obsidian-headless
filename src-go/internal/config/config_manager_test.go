package config

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestConfigManagerAuthTokenRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	cm := NewConfigManager(zerolog.New(io.Discard))

	token := "my-secret-token"
	if err := cm.SaveAuthToken(token); err != nil {
		t.Fatalf("SaveAuthToken failed: %v", err)
	}

	loaded, err := cm.LoadAuthToken()
	if err != nil {
		t.Fatalf("LoadAuthToken failed: %v", err)
	}
	if loaded != token {
		t.Errorf("expected token %q, got %q", token, loaded)
	}

	if err := cm.ClearAuthToken(); err != nil {
		t.Fatalf("ClearAuthToken failed: %v", err)
	}

	cleared, err := cm.LoadAuthToken()
	if err != nil {
		t.Fatalf("LoadAuthToken after clear failed: %v", err)
	}
	if cleared != "" {
		t.Errorf("expected empty token after clear, got %q", cleared)
	}
}

func TestConfigManagerLoadAuthTokenFromEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("OBSIDIAN_AUTH_TOKEN", "env-token")

	cm := NewConfigManager(zerolog.New(io.Discard))

	token, err := cm.LoadAuthToken()
	if err != nil {
		t.Fatalf("LoadAuthToken failed: %v", err)
	}
	if token != "env-token" {
		t.Errorf("expected env token %q, got %q", "env-token", token)
	}
}

func TestConfigManagerCredentialsRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	cm := NewConfigManager(zerolog.New(io.Discard))

	email := "user@example.com"
	password := "hunter2"

	if err := cm.SaveCredentials(email, password); err != nil {
		t.Fatalf("SaveCredentials failed: %v", err)
	}

	loadedEmail, loadedPassword, err := cm.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials failed: %v", err)
	}
	if loadedEmail != email {
		t.Errorf("expected email %q, got %q", email, loadedEmail)
	}
	if loadedPassword != password {
		t.Errorf("expected password %q, got %q", password, loadedPassword)
	}

	if err := cm.ClearCredentials(); err != nil {
		t.Fatalf("ClearCredentials failed: %v", err)
	}

	clearedEmail, clearedPassword, err := cm.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials after clear failed: %v", err)
	}
	if clearedEmail != "" {
		t.Errorf("expected empty email after clear, got %q", clearedEmail)
	}
	if clearedPassword != "" {
		t.Errorf("expected empty password after clear, got %q", clearedPassword)
	}
}

func TestConfigManagerMasterKeyCreated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	cm := NewConfigManager(zerolog.New(io.Discard))

	if err := cm.SaveAuthToken("token"); err != nil {
		t.Fatalf("SaveAuthToken failed: %v", err)
	}

	masterKeyPath, err := MasterKeyPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(masterKeyPath); os.IsNotExist(err) {
		t.Error("master key was not created")
	}
}

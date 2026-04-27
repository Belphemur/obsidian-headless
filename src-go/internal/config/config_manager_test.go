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

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/Belphemur/obsidian-headless/internal/model"
)

func TestRegions(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"regions": []map[string]any{{"id": "us", "name": "US"}}})
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second, zerolog.Nop())
	regions, err := client.Regions(context.Background(), "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(regions) != 1 || regions[0].ID != "us" {
		t.Errorf("unexpected regions: %+v", regions)
	}
}

func TestListVaults(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"vaults": []map[string]any{{"id": "v1", "name": "Test"}}})
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second, zerolog.Nop())
	vaults, err := client.ListVaults(context.Background(), "tok", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vaults) != 1 || vaults[0].ID != "v1" {
		t.Errorf("unexpected vaults: %+v", vaults)
	}
}

func TestCreateVault(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/vault/create" {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "v1", "name": "New"})
			return
		}
		if r.URL.Path == "/vault/list" {
			_ = json.NewEncoder(w).Encode(map[string]any{"vaults": []model.Vault{{ID: "v1", UID: "v1", Name: "New"}}})
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second, zerolog.Nop())
	vault, err := client.CreateVault(context.Background(), "tok", "New", "hash", "salt", "us", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vault.ID != "v1" {
		t.Errorf("id = %q, want %q", vault.ID, "v1")
	}
}

func TestValidateVaultAccess(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second, zerolog.Nop())
	if err := client.ValidateVaultAccess(context.Background(), "tok", "v1", "hash", "host", 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

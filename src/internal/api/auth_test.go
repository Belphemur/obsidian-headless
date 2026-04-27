package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSignIn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if body["email"] != "test@example.com" {
			t.Errorf("email = %v", body["email"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "tok123"})
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second)
	resp, err := client.SignIn(context.Background(), "test@example.com", "pass", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Token != "tok123" {
		t.Errorf("token = %q, want %q", resp.Token, "tok123")
	}
}

func TestSignOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second)
	if err := client.SignOut(context.Background(), "tok"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"email": "u@example.com"})
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second)
	info, err := client.UserInfo(context.Background(), "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Email != "u@example.com" {
		t.Errorf("email = %q, want %q", info.Email, "u@example.com")
	}
}

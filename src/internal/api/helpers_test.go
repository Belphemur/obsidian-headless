package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHostAPIURL_WithHTTP(t *testing.T) {
	t.Parallel()
	got := hostAPIURL("http://example.com", "/api")
	want := "http://example.com/api"
	if got != want {
		t.Errorf("hostAPIURL() = %q, want %q", got, want)
	}
}

func TestHostAPIURL_WithHTTPS(t *testing.T) {
	t.Parallel()
	got := hostAPIURL("https://example.com", "/api")
	want := "https://example.com/api"
	if got != want {
		t.Errorf("hostAPIURL() = %q, want %q", got, want)
	}
}

func TestHostAPIURL_Localhost(t *testing.T) {
	t.Parallel()
	got := hostAPIURL("localhost:8080", "/api")
	want := "http://localhost:8080/api"
	if got != want {
		t.Errorf("hostAPIURL() = %q, want %q", got, want)
	}
}

func TestHostAPIURL_127_0_0_1(t *testing.T) {
	t.Parallel()
	got := hostAPIURL("127.0.0.1:3000", "/api")
	want := "http://127.0.0.1:3000/api"
	if got != want {
		t.Errorf("hostAPIURL() = %q, want %q", got, want)
	}
}

func TestHostAPIURL_NoProtocol(t *testing.T) {
	t.Parallel()
	got := hostAPIURL("example.com", "/api")
	want := "https://example.com/api"
	if got != want {
		t.Errorf("hostAPIURL() = %q, want %q", got, want)
	}
}

func TestHostAPIURL_TrailingSlash(t *testing.T) {
	t.Parallel()
	got := hostAPIURL("https://example.com/", "/api")
	want := "https://example.com/api"
	if got != want {
		t.Errorf("hostAPIURL() = %q, want %q", got, want)
	}
}

func TestPostJSON_Success(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if req["key"] != "value" {
			t.Errorf("key = %v, want %v", req["key"], "value")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"result": "ok"})
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second)
	var resp map[string]any
	err := client.postJSON(context.Background(), server.URL+"/test", map[string]any{"key": "value"}, &resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp["result"] != "ok" {
		t.Errorf("result = %v, want %v", resp["result"], "ok")
	}
}

func TestPostJSON_APIError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "bad request", "code": "BAD"})
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second)
	err := client.postJSON(context.Background(), server.URL+"/test", map[string]any{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Message != "bad request" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "bad request")
	}
	if apiErr.Code != "BAD" {
		t.Errorf("Code = %q, want %q", apiErr.Code, "BAD")
	}
}

func TestPostJSON_CustomHeaders(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		if h := r.Header.Get("X-Custom"); h != "val" {
			t.Errorf("X-Custom = %q, want %q", h, "val")
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second)
	err := client.postJSON(context.Background(), server.URL+"/test", map[string]any{}, nil, &RequestOptions{
		Headers: map[string]string{"X-Custom": "val"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

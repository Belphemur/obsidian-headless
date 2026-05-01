package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
)

func TestListPublishSites(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"sites": []model.PublishSite{{ID: "s1", Slug: "site"}}})
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second, zerolog.Nop())
	sites, err := client.ListPublishSites(context.Background(), "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sites) != 1 || sites[0].ID != "s1" {
		t.Errorf("unexpected sites: %+v", sites)
	}
}

func TestCreatePublishSite(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "s1", "name": "Site"})
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second, zerolog.Nop())
	site, err := client.CreatePublishSite(context.Background(), "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if site.ID != "s1" {
		t.Errorf("id = %q, want %q", site.ID, "s1")
	}
}

func TestSetPublishSlug(t *testing.T) {
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
	if err := client.SetPublishSlug(context.Background(), "tok", "s1", server.URL, "my-slug"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetPublishSlugs(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"s1": "slug1"})
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second, zerolog.Nop())
	slugs, err := client.GetPublishSlugs(context.Background(), "tok", []string{"s1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slugs["s1"] != "slug1" {
		t.Errorf("slug = %q, want %q", slugs["s1"], "slug1")
	}
}

func TestListPublishedFiles(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"files": []model.PublishFile{{Path: "index.md"}}})
	}))
	defer server.Close()

	client := New(server.URL, 5*time.Second, zerolog.Nop())
	files, err := client.ListPublishedFiles(context.Background(), "tok", model.PublishSite{ID: "s1", Host: server.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 || files[0].Path != "index.md" {
		t.Errorf("unexpected files: %+v", files)
	}
}

func TestDeletePublishedFile(t *testing.T) {
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
	if err := client.DeletePublishedFile(context.Background(), "tok", model.PublishSite{ID: "s1", Host: server.URL}, "index.md"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

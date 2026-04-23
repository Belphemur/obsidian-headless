package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Belphemur/obsidian-headless/src-go/internal/cli"
	configpkg "github.com/Belphemur/obsidian-headless/src-go/internal/config"
)

const (
	apiBase   = "http://127.0.0.1:3000"
	testEmail = "test@example.com"
)

func TestGoCLIWorksWithMockServer(t *testing.T) {
	cleanup := ensureMockServer(t)
	defer cleanup()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("OBSIDIAN_API_BASE", apiBase)

	vaultA := filepath.Join(home, "vault-a")
	mustMkdir(t, vaultA)
	mustWriteFile(t, filepath.Join(vaultA, "hello.md"), []byte("# Hello from A\n"))

	runCLI(t, "login", "--email", testEmail, "--password", "secret")
	createOutput := runCLI(t, "sync-create-remote", "--name", "Go Port Vault", "--password", "sync-secret")
	vaultID := parseTrailingID(createOutput)
	if vaultID == "" {
		t.Fatalf("expected vault id in %q", createOutput)
	}

	runCLI(t, "sync-setup", "--vault", vaultID, "--path", vaultA, "--password", "sync-secret")
	runCLI(t, "sync", "--path", vaultA)
	token, err := configpkg.LoadAuthToken()
	if err != nil || token == "" {
		t.Fatalf("failed to load auth token: %v", err)
	}
	pushRemoteFile(t, token, vaultID, "hello.md", []byte("# Updated from Remote\n"))
	runCLI(t, "sync", "--path", vaultA)
	content, err := os.ReadFile(filepath.Join(vaultA, "hello.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Updated from Remote\n" {
		t.Fatalf("unexpected pulled content: %q", string(content))
	}

	mustWriteFile(t, filepath.Join(vaultA, "hello.md"), []byte("# Updated Locally Again\n"))
	runCLI(t, "sync", "--path", vaultA)
	pushRemoteFile(t, token, vaultID, "remote-only.md", []byte("# Remote only\n"))
	runCLI(t, "sync", "--path", vaultA)
	content, err = os.ReadFile(filepath.Join(vaultA, "remote-only.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Remote only\n" {
		t.Fatalf("unexpected synced content: %q", string(content))
	}

	mustWriteFile(t, filepath.Join(vaultA, "public.md"), []byte("---\npublish: true\n---\n\n# Public\n"))
	runCLI(t, "publish-create-site", "--slug", "go-port-site")
	runCLI(t, "publish-setup", "--site", "go-port-site", "--path", vaultA)
	runCLI(t, "publish", "--path", vaultA, "--yes")
	sites := struct {
		Sites []struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
			Host string `json:"host"`
		} `json:"sites"`
	}{}
	postJSON(t, apiBase+"/publish/list", map[string]any{"token": token}, &sites)
	var siteID, siteHost string
	for _, site := range sites.Sites {
		if site.Slug == "go-port-site" {
			siteID = site.ID
			siteHost = site.Host
			break
		}
	}
	if siteID == "" {
		t.Fatal("publish site not found on mock server")
	}
	published := struct {
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
	}{}
	postJSON(t, siteHost+"/api/list", map[string]any{"token": token, "id": siteID, "host": siteHost}, &published)
	found := false
	for _, file := range published.Files {
		if file.Path == "public.md" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("public.md was not published")
	}
}

func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	application := cli.New(bytes.NewReader(nil), &stdout, &stderr)
	if err := application.ExecuteArgs(context.Background(), args); err != nil {
		t.Fatalf("command %v failed: %v\nstdout:\n%s\nstderr:\n%s", args, err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func ensureMockServer(t *testing.T) func() {
	t.Helper()
	if isPortOpen("127.0.0.1:3000") && isPortOpen("127.0.0.1:3001") {
		return func() {}
	}
	repoRoot, err := filepath.Abs(filepath.Join(".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(ctx, "node", "mock-server/server.mjs")
	command.Dir = repoRoot
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if isPortOpen("127.0.0.1:3000") && isPortOpen("127.0.0.1:3001") {
			return func() {
				cancel()
				_ = command.Wait()
			}
		}
	}
	cancel()
	_ = command.Wait()
	t.Fatalf("mock server did not start: %s", output.String())
	return nil
}

func isPortOpen(address string) bool {
	conn, err := net.DialTimeout("tcp", address, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func postJSON(t *testing.T, endpoint string, body any, target any) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		t.Fatalf("request %s failed: %s", endpoint, response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func pushRemoteFile(t *testing.T, token, vaultID, path string, content []byte) {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:3001", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(map[string]any{
		"op":                 "init",
		"token":              token,
		"id":                 vaultID,
		"keyhash":            "",
		"version":            0,
		"initial":            false,
		"device":             "test-remote",
		"encryption_version": 3,
	}); err != nil {
		t.Fatal(err)
	}
	for {
		var message map[string]any
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatal(err)
		}
		if message["op"] == "ready" {
			break
		}
	}
	now := time.Now().UnixMilli()
	if err := conn.WriteJSON(map[string]any{
		"op":        "push",
		"path":      path,
		"extension": filepath.Ext(path),
		"hash":      fmt.Sprintf("%x", content),
		"ctime":     now,
		"mtime":     now,
		"folder":    false,
		"deleted":   false,
		"size":      len(content),
		"pieces":    1,
	}); err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	if response["res"] != "next" {
		t.Fatalf("expected next response, got %#v", response)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, content); err != nil {
		t.Fatal(err)
	}
	response = map[string]any{}
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	if response["res"] != "ok" {
		t.Fatalf("expected ok response, got %#v", response)
	}
}

func parseTrailingID(output string) string {
	start := strings.LastIndex(output, "(")
	end := strings.LastIndex(output, ")")
	if start < 0 || end <= start {
		return ""
	}
	return output[start+1 : end]
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

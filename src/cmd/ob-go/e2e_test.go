//go:build integration

package main_test

import (
	"bytes"
	"context"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Belphemur/obsidian-headless/src-go/internal/cli"
)

const apiBase = "http://127.0.0.1:3000"

func ensureMockServer(t *testing.T) func() {
	t.Helper()
	if isPortOpen("127.0.0.1:3000") && isPortOpen("127.0.0.1:3001") {
		return func() {}
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "node", "mock-server/server.mjs")
	cmd.Dir = repoRoot
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	require.NoError(t, cmd.Start())
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if isPortOpen("127.0.0.1:3000") && isPortOpen("127.0.0.1:3001") {
			return func() {
				cancel()
				_ = cmd.Wait()
			}
		}
	}
	cancel()
	_ = cmd.Wait()
	t.Skipf("mock server did not start: %s", output.String())
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

func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := cli.New(bytes.NewReader(nil), &stdout, &stderr)
	if err := app.ExecuteArgs(context.Background(), args); err != nil {
		t.Fatalf("command %v failed: %v\nstdout:\n%s\nstderr:\n%s", args, err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func TestE2E_Version(t *testing.T) {
	output := runCLI(t, "--version")
	assert.Contains(t, output, "0.1.0")
}

func TestE2E_Help(t *testing.T) {
	output := runCLI(t, "--help")
	assert.Contains(t, output, "login")
	assert.Contains(t, output, "sync")
	assert.Contains(t, output, "publish")
}

func TestE2E_LoginAndListRemote(t *testing.T) {
	cleanup := ensureMockServer(t)
	defer cleanup()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("OBSIDIAN_API_BASE", apiBase)

	output := runCLI(t, "login", "--email", "test@example.com", "--password", "test")
	assert.Contains(t, output, "Login successful")

	output = runCLI(t, "sync-list-remote")
	// The mock server has no vaults for this newly created user token,
	// so output should be empty (no error).
	assert.Equal(t, "", strings.TrimSpace(output))
}

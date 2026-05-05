---
name: test-runner
description: Guidelines and patterns for the obsidian-headless test suite
---

# Test Runner Skill

## Quick Start

Run the full suite locally (from the `src/` directory):

```bash
cd src
go test -race -count=1 -timeout=10m -shuffle=on ./...
```

Run a single package:

```bash
go test -race -count=1 -timeout=5m -shuffle=on ./internal/sync/...
```

**Key flags explained:**

| Flag | Purpose |
|------|---------|
| `-race` | Enables the Go race detector. Catches data races between goroutines. ~2-3x slower. |
| `-count=1` | Disables test caching. Essential for catching timing-dependent failures. |
| `-timeout=10m` | Per-package timeout. Increase for slow packages (`./internal/sync/...`). |
| `-shuffle=on` | Randomizes test execution order within a package. Catches hidden inter-test dependencies. |
| `-run=TestName` | Filters to a specific test or sub-test. |
| `-v` | Verbose output (useful for debugging a single test). |

**Before committing, run the full quality gate:**

```bash
cd src
go fmt ./...
go vet ./...
go fix ./...
go build ./...
go test -race -count=1 -timeout=10m -shuffle=on ./...
golangci-lint run
```

---

## Test Philosophy

### Parallel by Default

**Every test function must call `t.Parallel()` unless there is a documented exception.** This keeps the suite fast as it grows. The CI runs with `-shuffle=on` which only works correctly when tests are independent.

```go
func TestMyFeature(t *testing.T) {
    t.Parallel()
    // ...
}
```

Sub-tests should also parallelize when independent:

```go
func TestAggregator(t *testing.T) {
    t.Run("push rename", func(t *testing.T) {
        t.Parallel()
        // ...
    })
}
```

### When NOT to Parallelize

Skipping `t.Parallel()` is acceptable only in these situations:

1. **`t.Setenv()` is used** — Go panics at runtime if `t.Parallel()` is combined with `t.Setenv()`. If a package needs env vars, use `TestMain` + `os.Setenv` for package-global vars (e.g. secret prefix), and avoid `t.Setenv` in individual tests.

2. **Global mutable state is mutated** — If a test modifies a package-level variable (e.g. reducing `quiescenceDelay` for speed), it must run serially to avoid data races.

   ```go
   func TestAggregator_PushRename_Overwrites(t *testing.T) {
       // No t.Parallel() — mutates package-level quiescenceDelay
       oldDelay := quiescenceDelay
       quiescenceDelay = 500 * time.Millisecond
       t.Cleanup(func() { quiescenceDelay = oldDelay })
       // ...
   }
   ```

3. **Shared external resources** — Tests that depend on a single external process or fixed port must obtain a unique port per test or run serially.

---

## Common Patterns

### `mockSyncServer` — Full WebSocket Protocol Mock

`engine_test.go` defines `mockSyncServer`, a stateful WebSocket server that implements the Obsidian sync protocol (init, push, pull, ping, delete). Use it for any test that exercises `Engine.executePlan`, `RunContinuous`, or WebSocket session logic.

```go
mock := newMockSyncServer(t)
mock.addRecord("remote.md", 1, []byte("remote content"))

server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
defer server.Close()

wsURL := "ws" + server.URL[4:]
// Pass wsURL to Engine.Config.Host
```

The mock tracks:
- `recordsByUID` / `recordsByPath` — server-side file state
- `contentByUID` — raw file content
- `pushMsgs` — all push operations received from the client
- `initMsgs` — all init messages (useful for verifying worker handshakes)
- `pingCount` — heartbeat pings received
- `closeAfterPing` — force-close the connection after first ping (for reconnection tests)

### `startContinuousTest` — Integration Helper for Continuous Sync

`continuous_test.go` provides `startContinuousTest`, a helper that spins up a mock server, creates a temp vault, initializes an `Engine`, and starts `RunContinuous` in a background goroutine. Use it for end-to-end continuous sync scenarios.

```go
func TestContinuousWatcherSync(t *testing.T) {
    t.Parallel()
    env := startContinuousTest(t)

    mustWriteFile(t, filepath.Join(env.vault, "new.md"), []byte("new content"))

    waitFor(t, 5*time.Second, "new.md uploaded to mock server", func() bool {
        env.mock.mu.Lock()
        defer env.mock.mu.Unlock()
        for _, content := range env.mock.contentByUID {
            if string(content) == "new content" {
                return true
            }
        }
        return false
    })

    env.cancel()
    <-env.errCh
}
```

Options can customize the environment before start:

```go
env := startContinuousTest(t, func(env *continuousTestEnv) {
    env.mock.addRecord("remote.md", 1, []byte("remote content"))
    env.engine.Config.DownloadConcurrency = 2
}, withTimeout(10*time.Second))
```

### `waitFor` — Polling Assertion

`continuous_test.go` defines `waitFor`, which polls a condition every 100ms until it returns true or the timeout expires.

```go
func waitFor(t *testing.T, timeout time.Duration, desc string, cond func() bool) {
    t.Helper()
    deadline := time.Now().Add(timeout)
    for {
        if cond() {
            return
        }
        if time.Now().After(deadline) {
            t.Fatalf("timed out after %v waiting for: %s", timeout, desc)
        }
        time.Sleep(100 * time.Millisecond)
    }
}
```

Use this instead of `time.Sleep` for any asynchronous condition (file writes, WebSocket messages, state transitions).

### `waitForEvent` — Channel-Based Polling

`watcher_test.go` defines `waitForEvent` for reading from the watcher event channel until a predicate matches.

```go
func waitForEvent(t *testing.T, ch <-chan ScanEvent, timeout time.Duration, desc string, pred func(ScanEvent) bool) ScanEvent
```

Example:

```go
_ = waitForEvent(t, w.Out, 10*time.Second, "initial Create event", func(ev ScanEvent) bool {
    return ev.Type == EventCreate && ev.Path == oldPath
})
```

### Table-Driven Tests

Use table-driven tests for pure functions with multiple input/output pairs. Always run sub-tests in parallel.

```go
func TestIsJSONConfigPath(t *testing.T) {
    t.Parallel()
    tests := []struct {
        path      string
        configDir string
        want      bool
    }{
        {".obsidian/app.json", ".obsidian", true},
        {"notes/config.json", ".obsidian", false},
    }
    for _, tt := range tests {
        t.Run(tt.path, func(t *testing.T) {
            t.Parallel()
            if got := isJSONConfigPath(tt.path, tt.configDir); got != tt.want {
                t.Errorf("isJSONConfigPath(%q, %q) = %v, want %v", tt.path, tt.configDir, got, tt.want)
            }
        })
    }
}
```

### `t.TempDir()` for Filesystem Isolation

**Always** use `t.TempDir()` instead of manual `os.MkdirTemp` / `defer os.RemoveAll`. It auto-cleans and is race-safe.

```go
func TestScanLocal(t *testing.T) {
    t.Parallel()
    tmp := t.TempDir()
    mustWriteFile(t, filepath.Join(tmp, "a.md"), []byte("hello"))
    // ...
}
```

### Config Isolation via `t.Setenv`

Tests that exercise `config` or `storage` must isolate the user's real home directory to avoid polluting `~/.config`.

```go
home := t.TempDir()
t.Setenv("HOME", home)
t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
```

**Do not call `t.Parallel()` in tests that use `t.Setenv()`.** Instead, the package should set `_OBSIDIAN_HEADLESS_TEST_SECRET_PREFIX` in `TestMain`:

```go
func TestMain(m *testing.M) {
    os.Setenv("_OBSIDIAN_HEADLESS_TEST_SECRET_PREFIX", "test:")
    code := m.Run()
    os.Exit(code)
}
```

### API Mocking with `httptest.NewServer`

For REST API tests, use `httptest.NewServer` with inline handlers. Always handle `OPTIONS` preflight requests.

```go
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodOptions {
        w.WriteHeader(http.StatusOK)
        return
    }
    _ = json.NewEncoder(w).Encode(map[string]any{"token": "tok123"})
}))
defer server.Close()

client := New(server.URL, 5*time.Second, zerolog.Nop())
```

---

## Speeding Up Tests

### Test-Only Fields on `Engine`

`Engine` has three `test*` fields that override heartbeat timing defaults when non-zero:

```go
type Engine struct {
    // ...
    testHeartbeatInterval      time.Duration
    testHeartbeatSendThreshold time.Duration
    testHeartbeatTimeout       time.Duration
}
```

Set them in `startContinuousTest` options to avoid waiting for production intervals (60s+):

```go
env.engine.testHeartbeatInterval = 500 * time.Millisecond
env.engine.testHeartbeatSendThreshold = 250 * time.Millisecond
env.engine.testHeartbeatTimeout = 3 * time.Second
```

### Override Package-Level Constants

For tests that depend on timers, temporarily override package-level constants and restore them with `t.Cleanup`:

```go
func TestAggregator_PushRename_Overwrites(t *testing.T) {
    oldDelay := quiescenceDelay
    quiescenceDelay = 500 * time.Millisecond
    t.Cleanup(func() { quiescenceDelay = oldDelay })
    // ...
}
```

**Do not parallelize tests that mutate package-level variables.**

### Custom Timeouts via `withTimeout`

`continuous_test.go` provides `withTimeout` to extend the default context timeout for slow scenarios (e.g. reconnection with backoff):

```go
env := startContinuousTest(t, withTimeout(10*time.Second))
```

### Reduce `waitFor` Timeouts

When writing new tests, keep `waitFor` timeouts as tight as possible. A 5-second timeout is the default; if the operation reliably completes in <1s, use `2*time.Second`. This makes failures faster and reduces CI wall time.

---

## Coverage Map

### Well-Tested

| Package | Coverage | Notes |
|---------|----------|-------|
| `internal/sync` | High | Engine, plan builder, renames, merge logic, lock files, continuous sync |
| `internal/sync/watch` | High | Scanner, aggregator, fsnotify watcher, rename detection |
| `internal/api` | High | REST client, auth, vault ops, retry logic, error handling |
| `internal/storage` | Medium | SQLite state store, typed columns, incremental save, migration from v1 |
| `internal/config` | Medium | Config manager round-trips, secret storage isolation |
| `internal/util` | Medium | File utilities |
| `internal/circuitbreaker` | Medium | Circuit breaker config |
| `internal/publish` | Medium | Publish flag detection, glob matching, probe reading, scanLocal |
| `internal/1passwordstub` | Medium | WASM core and imported stub tests |

### Untested / Low Coverage

| Package | Why Untested |
|---------|-------------|
| `internal/encryption` | No tests. Key derivation and AES-GCM are thin wrappers over `golang.org/x/crypto`; integration tests exercise them indirectly. Unit tests would require large fixture data. |
| `internal/model` | Pure types / structs. No logic to test. |
| `internal/logging` | Thin zerolog wrapper. No business logic. |
| `internal/buildinfo` | Static variables populated at build time. |
| `internal/cli` | Cobra command handlers are thin wrappers over `api` / `sync` / `publish` packages. E2E tests cover the critical paths. |
| Real WebSocket edge cases | The mock server covers the happy path and basic reconnection. Complex scenarios (server-initiated disconnect, chunked upload failures, mid-stream encryption key rotation) are not exercised. |
| File conflict resolution UI | `threeWayMerge` and `jsonMerge` are tested, but the user-facing "choose local / remote" flow is CLI-only and covered only by E2E. |

---

## CI Matrix

The `.github/workflows/go-ci.yml` splits tests across a matrix of packages for parallelism and faster feedback:

```yaml
strategy:
  matrix:
    package:
      - ./internal/sync/...
      - ./internal/api/...
      - ./internal/storage/...
      - ./internal/config/...
      - ./internal/circuitbreaker/...
      - ./internal/util/...
      - ./internal/publish/...
      - ./internal/model/...
      - ./internal/encryption/...
      - ./internal/logging/...
      - ./internal/buildinfo/...
      - ./internal/1passwordstub/...
```

Each matrix job runs:

```bash
go test -race -count=1 -timeout=5m -shuffle=on ${{ matrix.package }}
```

**Implications for developers:**
- Keep package test times under ~5 minutes or the CI job will time out.
- The matrix means a failure in one package does not block others, but you must check all matrix cells.
- New packages must be added to the matrix or they will not run in CI.

---

## Flakiness Prevention

### Race Detector (`-race`)

Always run with `-race` locally and in CI. It catches:
- Unsynchronized access to `mockSyncServer.mu`
- Concurrent map access in `Engine.remote`
- Missing locks in watcher event handling

**Known safe pattern:** `mockSyncServer` uses `sync.Mutex` around all shared state. Copy data out of the lock before asserting:

```go
env.mock.mu.Lock()
pings := env.mock.pingCount
env.mock.mu.Unlock()
```

### Shuffle (`-shuffle=on`)

Randomizes test order within a package. Catches tests that depend on side effects from earlier tests (e.g. leaking temp files, global state). If a test fails only under shuffle, it is not properly isolated.

### Count (`-count=1`)

Disables Go's test cache. Without this, `go test` may skip running tests if the binary and source have not changed. This hides timing-dependent failures.

### Avoiding Time-Based Flakiness

- **Never use `time.Sleep` for synchronization.** Use `waitFor` or `waitForEvent` instead.
- If you must `time.Sleep`, keep it minimal (e.g. `200ms` for goroutine startup) and document why.
- Use `t.Cleanup` to restore global state (timers, env vars) so shuffle does not propagate state.
- Use `t.TempDir()` for all filesystem state to prevent cross-test pollution.

### Retry Policy for CI

If CI flakes on a specific test:
1. Re-run the failed job (GitHub Actions matrix cell).
2. If it fails again, reproduce locally with the same flags: `go test -race -count=1 -shuffle=on -run=TestName ./package`.
3. If it only fails under race/shuffle, it is likely a concurrency or isolation bug — fix it, do not disable the flag.

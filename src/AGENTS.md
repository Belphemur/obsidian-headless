# Agent Configuration for src

## Overview

Go implementation of the Obsidian Headless Client. Provides CLI for syncing and publishing Obsidian vaults without the desktop app.

## Project Structure

```
src/
├── cmd/ob-go/main.go              # Entry point
├── internal/
│   ├── api/client.go             # HTTP client for Obsidian REST API
│   ├── cli/
│   │   ├── app.go               # Application bootstrap
│   │   ├── root.go              # Root command setup
│   │   ├── auth.go             # login/logout commands
│   │   ├── sync.go             # Sync subcommands
│   │   └── publish.go          # Publish subcommands
│   ├── config/config.go         # Config management (auth, vaults, sites, master key)
│   ├── logging/logger.go        # File logger using zerolog
│   ├── model/types.go           # Data types (UserInfo, Vault, SyncConfig, etc.)
│   ├── storage/
│   │   ├── state.go           # SQLite state store (typed columns, incremental save)
│   │   ├── migrate.go         # Custom migration driver for modernc.org/sqlite
│   │   ├── crypto.go          # AES-GCM encryption for secrets
│   │   └── migrations/sqlite/ # Embedded schema migration files
│   ├── sync/
│   │   ├── engine.go         # WebSocket sync engine
│   │   └── watch/           # File system watcher
│   ├── publish/engine.go     # Publish engine
│   └── util/files.go         # File utilities (scan, hash, safe join)
└── go.mod
```

## Key Commands

### Auth

- `ob login [--email] [--password] [--mfa]` - Login to Obsidian account
- `ob logout` - Logout

### Sync

- `ob sync-list-remote` - List remote vaults
- `ob sync-list-local` - List configured vaults
- `ob sync-create-remote --name [--encryption] [--password]` - Create vault
- `ob sync-setup --vault [--path] [--password]` - Setup sync
- `ob sync [--path] [--continuous]` - Run sync
- `ob sync-config [--path]` - View/update sync settings
- `ob sync-status [--path]` - Show sync config
- `ob sync-unlink [--path]` - Remove sync config

### Publish

- `ob publish-list-sites` - List publish sites
- `ob publish-create-site --slug` - Create site
- `ob publish-setup --site [--path]` - Setup publish
- `ob publish [--path] [--dry-run] [--yes] [--all]` - Publish
- `ob publish-config [--path]` - View/update settings
- `ob publish-unlink [--path]` - Remove publish config

## Dependencies

- **spf13/cobra** - CLI framework
- **spf13/viper** - Configuration
- **gorilla/websocket** - WebSocket for sync
- **fsnotify** - File system watching
- **rs/zerolog** - Logging
- **modernc.org/sqlite** - Pure Go SQLite
- **golang-migrate/migrate/v4** - Schema migrations
- **golang.org/x/crypto** - Scrypt key derivation
- **gopkg.in/yaml.v3** - YAML parsing (frontmatter)
- **bmatcuk/doublestar/v4** - Glob patterns

## Configuration Paths

- Base: `~/.config/obsidian-headless/` (Linux) / `~/.obsidian-headless/` (macOS)
- Auth token: `{base}/auth_token`
- Credentials DB: `{base}/credentials.db`
- Master key: `{base}/master.key`
- Vault config: `{base}/sync/{vaultID}/config.json`
- State DB: `{base}/sync/{vaultID}/state.db`
- Site config: `{base}/publish/{siteID}/config.json`

## Database Schema

### state.db (per vault)

Managed by `golang-migrate/migrate/v4` with embedded SQL files in
`internal/storage/migrations/sqlite/`. Migrations run automatically
on `storage.Open()`.

```sql
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);

CREATE TABLE local_files (
    path    TEXT PRIMARY KEY,
    size    INTEGER NOT NULL DEFAULT 0,
    hash    TEXT    NOT NULL DEFAULT '',
    ctime   INTEGER NOT NULL DEFAULT 0,
    mtime   INTEGER NOT NULL DEFAULT 0,
    folder  INTEGER NOT NULL DEFAULT 0,
    deleted INTEGER NOT NULL DEFAULT 0,
    raw     BLOB    NOT NULL DEFAULT (jsonb('{}'))
);

CREATE TABLE server_files (
    path    TEXT PRIMARY KEY,
    size    INTEGER NOT NULL DEFAULT 0,
    hash    TEXT    NOT NULL DEFAULT '',
    ctime   INTEGER NOT NULL DEFAULT 0,
    mtime   INTEGER NOT NULL DEFAULT 0,
    folder  INTEGER NOT NULL DEFAULT 0,
    deleted INTEGER NOT NULL DEFAULT 0,
    uid     INTEGER NOT NULL DEFAULT 0,
    device  TEXT    NOT NULL DEFAULT '',
    user    TEXT    NOT NULL DEFAULT '',
    raw     BLOB    NOT NULL DEFAULT (jsonb('{}'))
);

CREATE INDEX idx_local_files_hash ON local_files(hash);
CREATE INDEX idx_server_files_hash ON server_files(hash);
CREATE INDEX idx_server_files_uid ON server_files(uid);
```

The `raw` column stores the full `FileRecord` JSON as SQLite JSONB binary
(via `jsonb()`), preserved for forward compatibility. Typed columns are
used for all sync operations; `raw` is never read by Go code.

## API Endpoints

- `POST /user/signin` - Sign in
- `POST /user/signout` - Sign out
- `POST /user/info` - Get user info
- `POST /vault/regions` - List regions
- `POST /vault/list` - List vaults
- `POST /vault/create` - Create vault
- `POST /vault/access` - Validate vault access
- `POST /publish/list` - List sites
- `POST /publish/create` - Create site

Sync uses WebSocket at the vault host for real-time file transfer.

## Running Locally

```bash
cd src
GOTOOLCHAIN=go1.26.0 go run ./cmd/ob-go --help
```

## Key Implementation Details

### Encryption

- Master key (32 bytes) encrypts credentials and vault keys in SQLite
- Vault encryption uses scrypt (2^15, 8, 1) with salt
- AES-256-GCM for secret storage

### Sync Protocol

- WebSocket connection for real-time sync
- Chunked file transfer (2MB chunks)
- Version tracking for conflict resolution
- Lock file mechanism to prevent concurrent syncs

### Publish Selection

- `publish: true/false` frontmatter flag (highest priority)
- Include/exclude patterns from config
- `--all` flag to publish untagged files

## Memory Management

When making design decisions, architectural changes, or significant implementation choices, save a memory using the `serena_write_memory` tool. Use descriptive topic paths (e.g., `src/logging/log-rotation`).

Before proposing or implementing new design changes, check existing memories with `serena_list_memories` and `serena_read_memory` to ensure consistency with prior decisions.

## Secret Storage

Sensitive values (auth tokens, vault encryption keys, encryption salts) are stored via the OS keyring (with an encrypted SQLite fallback). By default, `NewSecretStore` and `NewConfigManager` read/write keys unprefixed.

### Test Isolation via `_OBSIDIAN_HEADLESS_TEST_SECRET_PREFIX`

To prevent tests from overwriting real user secrets in the OS keyring, **every test package that exercises secret storage must set the environment variable `_OBSIDIAN_HEADLESS_TEST_SECRET_PREFIX`** (e.g. to `test:`). The constructors automatically detect this variable and prepend the prefix to every secret key.

**Why this matters:** Tests that create vaults, log in, or set encryption passwords would otherwise write keys like `auth_token` and `vault:<id>:encryption_key` directly into the user's OS keyring, polluting or overwriting their actual production credentials.

**Enforce it in `TestMain` so it applies to every test in the package:**

```go
func TestMain(m *testing.M) {
    os.Setenv("_OBSIDIAN_HEADLESS_TEST_SECRET_PREFIX", "test:")
    code := m.Run()
    os.Exit(code)
}
```

This is already in place for:
- `src/internal/config` package tests
- `src` integration tests

## Testing

### Parallel by Default

All test functions **must** call `t.Parallel()` unless there is a clear, documented reason not to. This keeps the test suite fast as it grows.

```go
func TestMyFeature(t *testing.T) {
    t.Parallel()
    // ...
}
```

### When NOT to Parallelize

Skipping `t.Parallel()` is only acceptable when:

- **`t.Setenv()` is used** — Go's testing package panics at runtime if `t.Parallel()` is combined with `t.Setenv()`. Tests in packages that need environment variables should use `TestMain` + `os.Setenv` instead.
- **Global mutable state is mutated** — If a test modifies a package-level variable (e.g., reducing a timing constant for test speed), it must run serially to avoid data races.
- **Shared external resources** — Tests that depend on a single external process or fixed port must either obtain a unique port per test or run serially.

### CI Flags

The test suite runs with these flags for maximum catch:

```
go test -race -count=1 -timeout=10m -shuffle=on ./...
```

- `-race` — catches data races
- `-count=1` — disables caching (prevents cache-based flakiness)
- `-shuffle=on` — randomizes test order to catch hidden inter-test dependencies

CI also runs `golangci-lint run` to enforce style, bug, and complexity checks automatically.

## Code Quality

Run the following commands before committing:

```bash
cd src
go fmt ./...
go vet ./...
go fix ./...
go build ./...
go test -race -count=1 -timeout=10m -shuffle=on ./...
golangci-lint run
```

**golangci-lint** runs a collection of linters that catch style issues, potential bugs, unused code, and complexity problems that `go vet` alone does not detect. It is the final gate before opening a pull request.

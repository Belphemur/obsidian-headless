# Agent Configuration for src-go

## Overview

Go implementation of the Obsidian Headless Client. Provides CLI for syncing and publishing Obsidian vaults without the desktop app.

## Project Structure

```
src-go/
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
│   │   ├── state.go           # SQLite state store (local/server files)
│   │   └── crypto.go          # AES-GCM encryption for secrets
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
```sql
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE local_files (path TEXT PRIMARY KEY, data TEXT NOT NULL);
CREATE TABLE server_files (path TEXT PRIMARY KEY, data TEXT NOT NULL);
```

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
cd src-go
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
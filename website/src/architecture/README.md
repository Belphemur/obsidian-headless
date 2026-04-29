---
title: Architecture
---

# Architecture

This page provides a high-level overview of how Obsidian Headless is structured, how data moves through the system, and where configuration lives.

For deeper dives into specific areas, see the [architecture subpages](#further-reading).

## Module Layout

The source code lives under `src/` and is split into focused packages:

```text
src/
├── cmd/ob-go/     # CLI entry point
├── internal/
│   ├── api/       # REST client for Obsidian HTTP API
│   ├── cli/       # Cobra command definitions (auth, sync, publish)
│   ├── config/    # Configuration management (auth, secrets, vault configs)
│   ├── encryption/# EncryptionProvider interface, V0 & V2/V3 implementations
│   ├── logging/   # zerolog console + file logger with rotation
│   ├── model/     # Shared data types (UserInfo, Vault, SyncConfig, etc.)
│   ├── publish/   # Publish engine (scan, hash, upload, remove)
│   ├── storage/   # SQLite state store (modernc.org/sqlite), migrations, credential encryption
│   ├── sync/      # WebSocket sync engine, plan builder, three-way merge, lock
│   └── util/      # File scanning, hashing, path safety, password derivation
└── go.mod
```

| Package | Key Files | Purpose |
|---------|-----------|---------|
| `api` | `client.go` | HTTP REST client for Obsidian cloud services |
| `cli` | `root.go`, `sync.go` | Cobra CLI entry point and command tree |
| `config` | `config.go`, `secrets.go` | Configuration management (auth, sync, publish) |
| `encryption` | `provider.go` | EncryptionProvider interface, V0 AES-GCM, V2/V3 AES-SIV + AES-GCM |
| `logging` | `logger.go` | zerolog console + file logger with lumberjack rotation |
| `model` | `types.go` | Shared types (UserInfo, Vault, SyncConfig, FileRecord, etc.) |
| `publish` | `engine.go` | Publish scanning, upload, and removal engine |
| `storage` | `state.go`, `crypto.go` | SQLite state store via modernc.org/sqlite, credential encryption |
| `sync` | `engine.go`, `connection.go`, `plan.go`, `merge.go`, `lock.go` | Sync engine, WebSocket connection, plan builder, three-way merge, file locking |
| `util` | `files.go` | File scanning, SHA-256 hashing, safe path join, random hex |

## Data Flow

### Sync Flow

The sync flow keeps a local vault in sync with the Obsidian Sync server over a WebSocket connection.

```text
┌──────────┐     WebSocket      ┌───────────────┐
│  CLI      │◄──────────────────►│  Obsidian     │
│ (ob-go)   │   JSON + binary   │  Sync Server  │
└─────┬─────┘                   └───────────────┘
      │
      ├── SyncEngine
      │     ├── SyncServerConnection  (WebSocket management)
      │     ├── fsnotify watcher      (local file change detection)
      │     ├── StateStore            (SQLite sync metadata)
      │     ├── SyncPlan              (upload/download/delete/merge actions)
      │     ├── EncryptionProvider    (encrypt/decrypt content + paths)
      │     └── merge.go             (three-way merge for conflicts)
      │
      └── Config (auth token, vault settings, log setup)
```

WebSocket connection, engine.RunOnce/RunContinuous, file watcher (fsnotify), plan builder (upload/download/delete/merge actions), parallel downloads with worker pool, three-way merge for text, JSON key-level merge for configs, 2MB chunks, 200MB max file, 30s interval, 5 concurrent downloads.

For details on the sync protocol, see [Sync Protocol](./sync-protocol.md). For encryption specifics, see [Encryption](./encryption.md).

### Publish Flow

The publish flow scans local files and uploads them to the Obsidian Publish API.

```text
┌──────────┐     HTTP POST      ┌───────────────┐
│  CLI      │──────────────────►│  Obsidian     │
│ (ob-go)   │   multipart       │  Publish API  │
└─────┬─────┘                   └───────────────┘
      │
      ├── PublishEngine
      │     ├── Local file scanning
      │     ├── Frontmatter parsing (publish: true/false)
      │     ├── Hash comparison with server
      │     └── Upload/remove operations
      │
      └── Config (publish site settings, cache)
```

For details on the REST API used by publish (and sync), see [REST API](./rest-api.md).

## File Watching Strategy

The sync engine uses `fsnotify` (cross-platform Go file system notifications), with a periodic full-rescan for consistency. Event aggregation with debounce is used to batch rapid changes. In continuous mode, the watcher triggers sync cycles on file changes.

### Watch disabled for read-only modes

In pull-only and mirror sync modes, local file changes are never uploaded. The fsnotify watcher is not started; only an initial scan is performed. This eliminates filesystem event overhead on machines that only download.

## Configuration Storage

All configuration is stored under a platform-specific base directory:

- **Linux**: `$XDG_CONFIG_HOME/obsidian-headless` or `~/.config/obsidian-headless`
- **macOS**: `~/.obsidian-headless`

Directory structure:

```text
~/.config/obsidian-headless/
├── credentials.db          # Encrypted SQLite fallback for auth token
├── sync/
│   └── <vault-id>/
│       ├── config.json     # SyncConfig
│       ├── state.db        # SQLite state database
│       ├── sync.lock/      # Directory-based file lock
│       └── sync.log        # Log output
└── publish/
    └── <site-id>/
        ├── config.json     # PublishConfig
        └── cache.json      # File hash cache
```

- **auth token**: OS keyring (with encrypted SQLite fallback at `credentials.db`)
- **vault config**: `sync/{vaultID}/config.json` + `state.db`
- **site config**: `publish/{siteID}/config.json` + `cache.json`

## Dependencies

| Package | Purpose |
|---------|---------|
| `spf13/cobra` | CLI framework for command parsing and flag management |
| `modernc.org/sqlite` | Pure-Go SQLite driver for sync state storage |
| `gopkg.in/yaml.v3` | YAML parsing for frontmatter extraction |

## Runtime Requirements

- **Go 1.21+** — Required for building the CLI binary. Uses Go 1.26 in CI.
- **Platforms**: Linux, macOS, Windows (amd64, arm64)

## Further Reading

- [Sync Protocol](./sync-protocol.md) — Deep dive into the WebSocket sync protocol
- [Encryption](./encryption.md) — Encryption providers, AES-SIV, and key derivation
- [REST API](./rest-api.md) — HTTP REST client and API endpoints

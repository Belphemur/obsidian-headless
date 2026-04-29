---
title: Architecture
---

# Architecture

This page provides a high-level overview of how Obsidian Headless is structured, how data moves through the system, and where configuration lives.

For deeper dives into specific areas, see the [architecture subpages](#further-reading).

## Module Layout

The source code lives under `src/` and is split into focused packages:

```
src/
├── api/          # HTTP REST client for Obsidian cloud services
├── cli/          # Commander.js CLI entry point
├── config/       # Configuration management (auth, sync, publish)
├── encryption/   # Encryption providers and AES-SIV implementation
├── fs/           # File system adapter with inode-aware watch
├── publish/      # Publish scanning, upload, and removal engine
├── storage/      # SQLite state store for sync metadata
├── sync/         # Sync engine, WebSocket connection, filters, and merge
└── utils/        # Shared helpers (crypto, encoding, debounce, paths, etc.)
```

| Package | Key Files | Purpose |
|---------|-----------|---------|
| `api` | `client.ts` | HTTP REST client for Obsidian cloud services |
| `cli` | `main.ts` | Commander.js CLI entry point |
| `config` | `index.ts` | Configuration management (auth, sync, publish) |
| `encryption` | `types.ts`, `aes-siv.ts`, `providers.ts` | Encryption providers and AES-SIV (RFC 5297) implementation |
| `fs` | `adapter.ts` | File system adapter with inode-aware file watching |
| `publish` | `engine.ts` | Publish scanning, upload, and removal engine |
| `storage` | `state-store.ts` | SQLite state store for sync metadata |
| `sync` | `engine.ts`, `connection.ts`, `filter.ts`, `merge.ts`, `lock.ts`, `backoff.ts` | Sync engine, WebSocket connection, filters, three-way merge, file locking, and backoff |
| `utils` | `crypto.ts`, `encoding.ts`, `debounce.ts`, `paths.ts`, etc. | Shared helpers for crypto, encoding, debounce, path handling, and more |

## Data Flow

### Sync Flow

The sync flow keeps a local vault in sync with the Obsidian Sync server over a WebSocket connection.

```
┌──────────┐     WebSocket      ┌───────────────┐
│  CLI      │◄──────────────────►│  Obsidian     │
│  (main)   │   JSON + binary   │  Sync Server  │
└─────┬─────┘                   └───────────────┘
      │
      ├── SyncEngine
      │     ├── SyncServerConnection  (WebSocket management)
      │     ├── FileSystemAdapter     (local file I/O + watch)
      │     ├── StateStore            (SQLite sync metadata)
      │     ├── SyncFilter            (which files to sync)
      │     ├── EncryptionProvider    (encrypt/decrypt content + paths)
      │     └── merge.ts             (three-way merge for conflicts)
      │
      └── Config (auth token, vault settings, log setup)
```

For details on the sync protocol, see [Sync Protocol](./sync-protocol.md). For encryption specifics, see [Encryption](./encryption.md).

### Publish Flow

The publish flow scans local files and uploads them to the Obsidian Publish API.

```
┌──────────┐     HTTP POST      ┌───────────────┐
│  CLI      │──────────────────►│  Obsidian     │
│  (main)   │   multipart       │  Publish API  │
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

The `FileSystemAdapter` uses `fs.watch({ recursive: true })` under the hood, with OS-specific optimisations:

| Platform | Backend | Inode tracking | Notes |
|----------|---------|:--:|-------|
| **Linux** | inotify (recursive since Node 19) | ✅ | Rename detection via inode matching. Subject to `fs.inotify.max_user_watches` limit. |
| **macOS** | FSEvents | ✅ | Rename detection via inode matching. Very efficient native event stream. |
| **Windows** | ReadDirectoryChangesW | ❌ | No inode tracking — NTFS file IDs can be reused. Renames are reported as delete + create. |

### Inode-based rename detection (Linux / macOS)

When a file disappears, its inode is held in a pending-renames buffer for 150 ms. If a new file appears with the same inode within that window, the adapter emits a single `"renamed"` event (with both old and new paths) instead of separate `"file-removed"` + `"file-created"` events. This lets the sync engine move the metadata record without re-hashing or re-uploading the unchanged content.

### Watch disabled for read-only modes

In `pull-only` and `mirror-remote` sync modes, local file changes are never uploaded. The adapter's `watch()` call is skipped entirely; only an initial `listAll()` scan is performed. This eliminates inotify/FSEvents overhead on machines that only download.

## Configuration Storage

All configuration is stored under a platform-specific base directory:

- **Linux**: `$XDG_CONFIG_HOME/obsidian-headless` or `~/.config/obsidian-headless`
- **macOS / Windows**: `~/.obsidian-headless`

Directory structure:

```
~/.config/obsidian-headless/
├── auth_token              # Stored authentication token
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

## Dependencies

| Package | Purpose |
|---------|---------|
| `commander` | CLI framework for command parsing |
| `better-sqlite3` | SQLite3 binding for sync state storage |
| `yaml` | YAML parsing for frontmatter extraction |

## Runtime Requirements

- **Node.js 24+** — Required for native WebSocket, Web Crypto, `Promise.withResolvers`, and `node:crypto` scrypt
- **Platforms**: macOS, Linux, Windows (x64, arm64)

## Further Reading

- [Sync Protocol](./sync-protocol.md) — Deep dive into the WebSocket sync protocol
- [Encryption](./encryption.md) — Encryption providers, AES-SIV, and key derivation
- [REST API](./rest-api.md) — HTTP REST client and API endpoints

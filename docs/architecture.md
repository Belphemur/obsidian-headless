# Architecture Overview

## Module Layout

The codebase is organized into the following modules under `src/`:

```
src/
├── api/
│   └── client.ts          # HTTP REST client for Obsidian cloud services
├── cli/
│   └── main.ts            # Commander.js CLI entry point
├── config/
│   └── index.ts           # Configuration management (auth, sync, publish)
├── encryption/
│   ├── types.ts            # EncryptionProvider interface
│   ├── aes-siv.ts          # Pure-JS AES-SIV (RFC 5297) implementation
│   ├── providers.ts        # V0, V2, V3 encryption provider implementations
│   └── index.ts            # Re-exports
├── fs/
│   └── adapter.ts          # File system adapter with watch support
├── publish/
│   └── engine.ts           # Publish scanning, upload, removal engine
├── storage/
│   └── state-store.ts      # SQLite state store for sync metadata
├── sync/
│   ├── backoff.ts          # Exponential backoff with jitter
│   ├── connection.ts       # WebSocket sync server connection
│   ├── engine.ts           # Main sync engine (orchestrates everything)
│   ├── filter.ts           # File sync filter (type, folder, config rules)
│   ├── lock.ts             # Directory-based file lock
│   └── merge.ts            # Three-way merge (diff-match-patch)
└── utils/
    ├── async.ts            # Deferred promises, AsyncQueue, sleep
    ├── crypto.ts           # SHA-256, AES-GCM encrypt/decrypt wrappers
    ├── debounce.ts         # Debounce with keep-alive semantics
    ├── encoding.ts         # Buffer ↔ hex/base64/string conversions
    ├── format.ts           # Human-readable byte formatting
    └── paths.ts            # Vault-relative path helpers
```

## Data Flow

### Sync Flow

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

### Publish Flow

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

## Dependencies

| Package | Purpose |
|---------|---------|
| `commander` | CLI framework for command parsing |
| `better-sqlite3` | SQLite3 binding for sync state storage |
| `yaml` | YAML parsing for frontmatter extraction |

## Runtime Requirements

- **Node.js 24+** — Required for native WebSocket, Web Crypto, `Promise.withResolvers`, and `node:crypto` scrypt
- **Platforms**: macOS, Linux, Windows (x64, arm64)

## Configuration Storage

All configuration is stored under a platform-specific base directory:

- **Linux**: `$XDG_CONFIG_HOME/obsidian-headless` or `~/.config/obsidian-headless`
- **macOS/Windows**: `~/.obsidian-headless`

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

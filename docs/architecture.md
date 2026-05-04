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
│   └── adapter.ts          # File system adapter with inode-aware watch
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

## File Watching Strategy

The `FileSystemAdapter` uses `fs.watch({ recursive: true })` under the hood, with OS-specific optimisations:

| Platform | Backend | Inode tracking | Notes |
|----------|---------|:---:|-------|
| **Linux** | inotify (recursive since Node 19) | ✅ | Rename detection via inode matching. Subject to `fs.inotify.max_user_watches` limit. |
| **macOS** | FSEvents | ✅ | Rename detection via inode matching. Very efficient native event stream. |
| **Windows** | ReadDirectoryChangesW | ❌ | No inode tracking — NTFS file IDs can be reused. Renames are reported as delete + create. |

### Inode-based rename detection (Linux / macOS)

When a file disappears, its inode is held in a pending-renames buffer for 150 ms.
If a new file appears with the same inode within that window, the adapter emits a
single `"renamed"` event (with both old and new paths) instead of separate
`"file-removed"` + `"file-created"` events. This lets the sync engine move the
metadata record without re-hashing or re-uploading the unchanged content.

### Remote rename detection (UID + hash-based fallback)

When a file is renamed on another device, the Obsidian Sync server does not
have a native "rename" protocol message. Instead it sends **two separate push
notifications** that share the same UID:

1. A push for `newPath` with the file content (`deleted: false`)
2. A push for `oldPath` with `deleted: true`

The client detects remote renames by correlating these two pushes via UID
matching in `applyRemoteRenameFixups()` (see `src/internal/sync/rename.go`):

- If the local file at `oldPath` is **unmodified** (hash matches previous
  state), parent directories are created and the file is renamed in-place on
  disk via `os.Rename` to `newPath`. The sync state metadata moves with it —
  `PreviousPath` is set to preserve the rename chain. No re-download is needed.
- If the local file at `oldPath` was **modified** locally, or if there is no
  previous state for `oldPath` (missing from both `previousLocal` and
  `previousRemote`), it is preserved at its original path and the conflict is
  logged. The remote
  version at `newPath` is downloaded normally, and `buildPlan` handles the two
  files independently.
- Rename or directory-creation failures are recorded as conflicts rather than
  returned as errors.

When the Obsidian desktop app performs a rename, it sometimes executes a
**delete + upload** as separate operations, causing the server to assign a
**new UID** to the uploaded file. In these cases UID matching fails. The
client falls back to **hash-based detection**: for deleted records that were
not matched by UID, it compares file hashes between the deleted and active
entries. If the deleted record's hash is empty, it falls back to
`previousRemote[path].Hash`. When exactly one active entry shares the same
hash, the rename is detected. If multiple candidates match (ambiguous hash),
the fallback is skipped with a warning and the entries are treated as
independent operations. This ensures pure renames — where content is unchanged
— are detected even when UIDs differ.

After renaming, the watcher is notified of the affected paths via
`AddIgnorePaths()` so that the resulting filesystem events (from `os.Rename`)
are suppressed and not misinterpreted as user-initiated renames.

This detection runs in both `RunOnce` and continuous sync modes, before
`buildPlan` is called.

### Local rename propagation to remote

When a local file rename is detected by the filesystem watcher, the rename
source path must be communicated to the server via the `relatedpath` field
in the WebSocket push message. This propagation flows through three stages:

1. **Watcher detection** — The `FileSystemAdapter` detects renames via
   inode tracking (Linux/macOS) and emits a `ScanEvent` with both old and
   new paths.

2. **`applyRenameFixups()`** (`continuous.go`) — Runs before plan
   construction. It moves the renamed file's record from its old path to its
   new path in `previousLocal` and sets `PreviousPath` to preserve the
   rename chain. This ensures that when `scanLocal()` later creates a fresh
   `FileRecord` for the new path (without `PreviousPath`), the pipeline can
   recover the rename source from the previous state.

3. **`executePlan()`** (`engine.go`) — Accepts `previousLocal` as an
   additional parameter (alongside the standard `currentLocal`,
   `previousRemote` maps). In the `syncActionUpload` case, it enriches
   the upload record with `PreviousPath` from the previous local state
   before calling `session.push()`. After a successful push,
   `PreviousPath` is cleared on the record in `currentLocal` to prevent
   re-sending `relatedpath` on subsequent sync cycles.

4. **`session.push()`** (`session.go`) — When `record.PreviousPath` is
   non-empty, it encrypts the value via `s.encryptPath()` and includes it
   as `relatedpath` in the WebSocket push message.

The `previousLocal` map is loaded once per sync cycle from the state store
and reused across plan building and execution. Each new cycle reloads it
fresh, so clearing `PreviousPath` in `currentLocal` after push is sufficient
to prevent retransmission.

### Watch disabled for read-only modes

In `pull-only` and `mirror-remote` sync modes, local file changes are never
uploaded. The adapter's `watch()` call is skipped entirely; only an initial
`listAll()` scan is performed. This eliminates inotify/FSEvents overhead on
machines that only download.

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

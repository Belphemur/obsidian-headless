# Go Port Progress

- [x] Review the existing TypeScript architecture, CLI docs, REST API docs, sync protocol docs, and mock-server docs
- [x] Scaffold a dedicated `src/` Go module with package boundaries aligned to the TypeScript modules
- [x] Split the new Go CLI into focused packages and command files instead of a single root implementation
- [x] Finish the Go REST client, config management, SQLite state store, and zerolog-based logging
- [x] Finish the sync engine, including the Syncthing-style watcher, continuous mode, and mock-server compatibility
- [x] Finish the publish engine and remaining CLI command behaviors
- [x] Add and pass Go integration tests against the existing Node mock server
- [x] Update repository documentation for the Go application
- [ ] Run `go fix`, `go fmt`, `go vet`, Go tests, and the existing repository validations before each commit/finalization

## Current status

- The Go module scaffold exists under `src/`.
- Package structure now mirrors the TypeScript app at a high level: CLI, API, config, storage, sync, publish, utils, and logging.
- The module now targets Go `1.26.0` and uses Cobra/Viper, `modernc.org/sqlite`, `zerolog`, `fsnotify`, `gorilla/websocket`, `doublestar`, and `yaml.v3`.
- The Go CLI now has working login/logout, sync configuration and execution, publish configuration and execution, a SQLite-backed sync state store, and a Syncthing-style watcher pipeline for continuous sync.
- Integration coverage now exercises the Go CLI end to end against the existing Node mock server for login, sync upload/download, and publish.
- Follow-up review fixes hardened the Go port with validated SQLite table names, safer vault path joining, bounded remote allocations, and `scrypt`-based password hashing.
- TypeScript `npm run lint` and `npm run build` pass in the current environment.
- `npm test` reaches the mock-server suite, but the WebSocket assertions still fail under the available Node `v20.20.2` runtime because `WebSocket` is not globally defined there; the repository itself documents a newer Node runtime requirement.
- Remaining work is focused on final repository-wide validation bookkeeping and opening the PR.

## Production Fixes (April 24, 2026)

The Go CLI has been validated against the production Obsidian Sync service:

- **API client**: `ValidateVaultAccess` now sends correct `uid` field (was
  `vault_uid`) and uses the vault's actual `encryption_version`.
- **Sync engine**: Replaced concurrent `readLoop` with synchronous init handshake
  to eliminate races where two goroutines read from the same WebSocket.
- **State persistence**: `RunOnce` now saves local and remote file records after
  sync to the SQLite state DB, preventing false re-syncs on subsequent runs.
- **Content decryption**: `session.pull` now decrypts downloaded file content
  before writing to disk (files were previously saved as encrypted blobs).
- **WebSocket hang**: `Engine.Close` now cancels the `context.AfterFunc`
  goroutine, allowing one-shot sync to exit cleanly.
- **Verbose logging**: All WebSocket text/binary messages are logged with
  direction, type, size, and payload for protocol reverse engineering.
- **Mock server**: Updated to broadcast push echoes to all clients (including
  sender) and send deleted files during init handshake.
- **Protocol docs**: Updated `sync-protocol.md` with observed production
  behavior: synchronous init flow, push echo semantics, and dual error formats.

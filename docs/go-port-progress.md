# Go Port Progress

- [x] Review the existing TypeScript architecture, CLI docs, REST API docs, sync protocol docs, and mock-server docs
- [x] Scaffold a dedicated `src-go/` Go module with package boundaries aligned to the TypeScript modules
- [x] Split the new Go CLI into focused packages and command files instead of a single root implementation
- [x] Finish the Go REST client, config management, SQLite state store, and zerolog-based logging
- [x] Finish the sync engine, including the Syncthing-style watcher, continuous mode, and mock-server compatibility
- [x] Finish the publish engine and remaining CLI command behaviors
- [x] Add and pass Go integration tests against the existing Node mock server
- [x] Update repository documentation for the Go application
- [ ] Run `go fix`, `go fmt`, `go vet`, Go tests, and the existing repository validations before each commit/finalization

## Current status

- The Go module scaffold exists under `src-go/`.
- Package structure now mirrors the TypeScript app at a high level: CLI, API, config, storage, sync, publish, utils, and logging.
- The module now targets Go `1.26.0` and uses Cobra/Viper, `modernc.org/sqlite`, `zerolog`, `fsnotify`, `gorilla/websocket`, `doublestar`, and `yaml.v3`.
- The Go CLI now has working login/logout, sync configuration and execution, publish configuration and execution, a SQLite-backed sync state store, and a Syncthing-style watcher pipeline for continuous sync.
- Integration coverage now exercises the Go CLI end to end against the existing Node mock server for login, sync upload/download, and publish.
- Follow-up review fixes hardened the Go port with validated SQLite table names, safer vault path joining, bounded remote allocations, and `scrypt`-based password hashing.
- TypeScript `npm run lint` and `npm run build` pass in the current environment.
- `npm test` reaches the mock-server suite, but the WebSocket assertions still fail under the available Node `v20.20.2` runtime because `WebSocket` is not globally defined there; the repository itself documents a newer Node runtime requirement.
- Remaining work is focused on final repository-wide validation bookkeeping and opening the PR.

## Review Fixes (April 2026)

All open threads from copilot-pull-request-reviewer, gemini-code-assist, and
coderabbitai have been addressed:

- **Secrets storage**: `EncryptionKey` is no longer written to the plain-text
  config JSON (`json:"-"`). Instead it is AES-GCM encrypted with a
  per-installation master key and stored in the vault's SQLite state DB.
- **Auth token**: `LoadOrCreateMasterKey()` added to config package; ready for
  future migration of auth token to the encrypted store.
- **Config ID validation**: `validateConfigID` added to reject path-escaping
  vault/site IDs in `SyncDir` / `PublishDir`.
- **ValidateConfigDir**: now rejects `"."` and `".."`.
- **api/client**: nil-target endpoints now decode and surface application-level
  errors; `extractApplicationError` replaced with a single body-byte decode path.
- **cli/sync**: unknown encryption modes now return an error (switch instead of
  if); `ValidateVaultAccess` uses `vault.EncryptionVersion`.
- **storage/state**: rollback guard fixed with named return; upsert strategy
  (`INSERT … ON CONFLICT DO UPDATE`) replaces DELETE+INSERT.
- **util/files**: `ScanVault` skips symlinks and streams SHA-256 via
  `HashReader`; `SafeJoin` now rejects paths containing symlinked components.
- **publish/engine**: symlink entries skipped in `scanLocal`.
- **sync/engine**: watcher channel closure handled; WebSocket connections are
  closed on context cancellation; `pull` validates actual byte count; cleanup
  loop uses absolute vault root.
- **sync/watch/watcher**: fullRescan guarded by `atomic.Bool`; per-subdirectory
  goroutines; `addDirsRecursive` rewritten with `filepath.WalkDir` (no
  deadlock); files in moved-in directories emit EventCreate.
- **sync/watch/aggregator**: `emit` is now blocking so no events are silently
  dropped.
- **CI**: `.github/workflows/copilot-setup-steps.yml` provisions Node 24 and
  Go 1.26.

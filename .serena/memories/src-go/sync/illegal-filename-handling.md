## Decision: Ignore files with illegal filename characters

**Context:** GitHub issue #22 — `ob sync` was uploading files with illegal characters (e.g. `:`) that the Obsidian app rejects.

**Decision:** Skip files/directories with illegal characters and log a clear warning. Do NOT auto-rename or delete.

**Rationale:**
- Non-destructive — preserves user data.
- Matches Obsidian app behavior.
- Warns the user so they can manually fix filenames.

**Implementation:**
- Added `util.IsLegalPath` checking each path component against `:*?"<>|\` and control characters.
- `ScanVault` now skips illegal files/dirs and returns them as `[]string`.
- `engine.scanLocal` logs `WARN` for each skipped path.
- `sync.isValidPath` now delegates to `util.IsLegalPath`, so remote records with illegal chars are also filtered from sync plans and state.

**Files changed:**
- `src/internal/util/files.go` — `IsLegalPath`, `ScanVault`
- `src/internal/sync/engine.go` — `scanLocal`
- `src/internal/sync/plan.go` — `isValidPath`
- `src/internal/util/files_test.go` — new tests
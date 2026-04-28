# Sync Stability Fixes

Branch: `fix/sync-stability-and-resilience` → PR #27

## Changes

### engine.go — defaultDownloadConcurrency
- `10` → `3`

### connection.go — dialWorker
- Exponential backoff with jitter: base 200ms, max 8s, 4 retries
- Handles transient I/O timeouts and server overload

### state.go — StateStore init()
- WAL mode already set (was there before)
- Added: `busy_timeout=5000`, `synchronous=NORMAL`, `cache_size=-64000` (64MB), `temp_store=MEMORY`

### engine.go — executeDownloadsParallel
- Worker ID per goroutine (1-indexed)
- Per-file log: `workerID`, `path`, `done` count
- Summary log: `completed=N, failed=N`
- All worker errors collected into `errMsgs []string` (not just first)
- **Partial failure logic**: if `len(errMsgs) > 0 && done == 0` → fail hard; if some succeeded → warn and continue
- Removed single-error-channel pattern; uses mutex + counters instead

### engine.go — executePlan
- Added action breakdown log at start of non-download phase:
  `uploads=X, deleteRemote=Y, deleteLocal=Z, merges=W, downloads=N`

## Commits (4 total)

1. `c5ac118` — Reduce concurrency, dial backoff, WAL pragmas
2. `ea4f1e9` — Per-worker logging, action breakdown
3. `647f30a` — Partial failure tolerance

## Key Context
- Live sync log: `~/.config/obsidian-headless/sync/29ccc4c266e13c9c8b92be46456cc2c9/sync.log`
- Error: `"database is locked (5) (SQLITE_BUSY)"`
- Error: `"worker dial: read tcp ... i/o timeout"`

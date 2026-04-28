# Parallel Downloads for Initial Sync

## Overview

During sync plan execution, download actions run in parallel using a connection
pool. Each worker goroutine owns a dedicated WebSocket connection and pulls files
sequentially on that connection. This dramatically reduces the time required for
initial syncs or syncs with many pending downloads.

## Architecture

```
               ┌─────────────────┐
               │   executePlan   │
               └────────┬────────┘
         ┌──────────────┴──────────────┐
         │   Non-downloads (sequential) │  uploads, deletes, merges
         │   on main WebSocket conn     │  run FIRST
         └──────────────┬──────────────┘
         ┌──────────────┴──────────────┐
         │   executeDownloadsParallel   │
         │                              │
         │   Worker 1 ──► conn1 ──► pull() ──► write disk
         │   Worker 2 ──► conn2 ──► pull() ──► write disk
         │   ...                        │
         │   Worker N ──► connN ──► pull() ──► write disk
         │                              │
         │   (N = min(files, concurrency))│
         └──────────────────────────────┘
```

## Why Connection Pool

The Obsidian sync `pull` protocol is request-response without request IDs:

```
Client: {"op": "pull", "uid": 42}
Server: {"res": "ok", "size": 1024, "pieces": 1}
Server: <binary chunk>
```

Without a correlation ID, concurrent pulls on the same WebSocket connection
would interleave responses, making it impossible to match results to requests.
Each worker must have its own WebSocket connection.

## Execution Order

**Non-download actions run sequentially first**, then downloads run in parallel.
This is safe because:

| Property | Guarantee |
|----------|-----------|
| One action per path | The plan builder produces at most one action per path |
| No cross-path dependencies | Merges, uploads, and deletes only affect their own path |
| `session.remote` mutations | Only modified for paths with no subsequent actions |

### Action Flow

1. **Uploads** — read local file, push to server via main connection
2. **Deletes** — delete remote or local file via main connection
3. **Merges** — three-way merge or JSON merge on main connection
4. **Folder creation** — ensure all remote directories exist locally
5. **Parallel downloads** — workers pull files concurrently

## Worker Sizing (Lazy Pool)

Workers are created as `min(number_of_download_actions, configured_concurrency)`:

| Sync Type | Files to download | Concurrency | Workers created |
|-----------|-------------------|-------------|-----------------|
| Incremental | 3 | 10 | 3 |
| Initial | 2,000 | 10 | 10 |
| Small update | 1 | 10 | 1 |

No idle connections for small syncs. Each worker creates its connection on first
use via `dialWorker()` which performs a full init handshake.

## Worker Handshake

Each worker calls `dialWorker()` which:

1. Dials a new WebSocket connection to the vault host
2. Computes the key hash (if encryption is enabled)
3. Sends an `init` message with `"initial": false` (workers are not the primary
   initiator; they join an existing session)
4. Reads the init response
5. Reads and discards push messages (file listings) until `"ready"`
6. Returns the connected WebSocket

The worker's `remoteSession` shares the main session's `remote` map, which is
read-only during the parallel download phase.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Worker dial fails | Retries up to 4 times with exponential backoff (200ms base, 8s max) + jitter. After max retries, worker exits with logged error. |
| Pull fails (network error) | Worker logs error with worker ID and path, then exits. Remaining workers continue. First error wins. |
| Write to disk fails | Same as pull failure. |
| Context cancelled | `context.AfterFunc` closes each worker's WebSocket connection, unblocking any in-progress reads/writes. Workers exit. `ctx.Err()` returned. |

When a worker exits early (dial or pull/write error), it does **not** drain the jobs channel — the remaining jobs stay in the channel and are processed by workers that haven't exited yet. The `context.AfterFunc` on each connection ensures workers don't hang if the context is cancelled while waiting on a network read.

After all workers finish, a summary log is emitted with `completed=N` and `failed=N` counts.

## Per-Worker Logging

Each worker logs with a `workerID` field (1-indexed) for correlation:

```
debug: worker downloaded file workerID=2 path="notes/tasks.md" done=47
info:  parallel download complete completed=187 failed=1
error: worker pull failed workerID=3 path="notes/secret.md" error="..." workerID=3
```

## Action Breakdown

At the start of plan execution, `executePlan` logs the full action breakdown:

```
info:  sync plan uploads=12 deleteRemote=3 deleteLocal=1 merges=5 downloads=203
```

Non-download actions (uploads, deletes, merges) run sequentially first. Then parallel downloads run with the worker pool.

## Configuration

| Location | Field | Default |
|----------|-------|---------|
| `SyncConfig` struct | `DownloadConcurrency int` | `3` |
| Code constant | `defaultDownloadConcurrency` (engine.go) | `3` |

The `SyncConfig.DownloadConcurrency` field serializes as `"downloadConcurrency"`
in JSON. A value of `0` or negative defaults to 3.

## Testing

### Thread-Safe Mock Server

The `mockSyncServer` in tests was updated with `sync.Mutex` to support
concurrent connections safely. All map accesses are guarded.

### Test Coverage

| Test | Files | Concurrency | Purpose |
|------|-------|-------------|---------|
| `TestExecutePlan` | 1 download, 1 upload | 1 (default) | Original mixed-action test |
| `TestExecutePlanParallelDownloads` | 200 downloads | 10 | Large-scale parallel download correctness |
| `TestExecutePlanParallelSmallSync` | 3 downloads | 10 | Verifies workers = min(files, concurrency) |

## Future Considerations

- **CLI flag**: `--download-concurrency` for per-run tuning
- **Progressive ramp-up**: Start with 1 worker, add more as queue depth grows
- **Connection reuse**: Maintain a persistent pool of idle connections across
  sync cycles to avoid handshake overhead
- **Per-vault limits**: Option to cap concurrency per vault to avoid server
  rate limiting

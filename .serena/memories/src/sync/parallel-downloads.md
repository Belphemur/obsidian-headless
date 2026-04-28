# Parallel Downloads for Initial Sync

## Decision
Download actions during sync are executed in parallel using a connection pool pattern. Each worker goroutine owns a dedicated WebSocket connection and pulls files sequentially on that connection. The number of workers = min(number of download actions, configured `DownloadConcurrency`), defaulting to 10.

## Why Connection Pool
The Obsidian sync WebSocket protocol uses request-response without request IDs. The `pull` op sends `{"op":"pull","uid":uid}` and expects `{"res":"ok",...}` followed by binary chunks. Without request IDs, concurrent pulls on the same connection would interleave responses. Each worker needs its own WebSocket connection.

## Execution Order
Non-download actions (uploads, deletes, merges) run SEQUENTIALLY first on the main connection. This preserves operation ordering guarantees. Downloads run in parallel AFTER all non-download actions complete. This is safe because:
- Each path has at most one action in the plan
- No cross-path data dependencies exist
- `session.remote` mutations during pushes/deletes only affect paths with no subsequent actions

## Lazy Worker Count
Workers = min(len(download_actions), configured_concurrency). For a 3-file sync, we create 3 workers (not 10). For a 2000-file initial sync, we create up to the configured max. No idle connections for small syncs.

## Error Handling
Workers share a buffered error channel (size 1). On error, the failing worker drains remaining jobs from the channel inline (so the sender isn't blocked) and exits. Other workers also drain and exit. First error wins and is returned.

## Configuration
- `SyncConfig.DownloadConcurrency` (int, json: "downloadConcurrency")
- Default: 10 (`defaultDownloadConcurrency` constant in engine.go)
- If <= 0, defaults to 10

## Files Modified
- `src/internal/model/types.go`: Added DownloadConcurrency field
- `src/internal/sync/connection.go`: Added dialWorker() method
- `src/internal/sync/engine.go`: Refactored executePlan(), added executeDownloadsParallel()
- `src/internal/sync/engine_test.go`: Thread-safe mock, parallel download tests

## Future Considerations
- Progressive connection ramp-up (currently all workers start immediately)
- Connection reuse for downloads across sync cycles
- CLI flag for download concurrency
- Per-vault concurrency limits to avoid overwhelming the server
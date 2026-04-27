# Graceful Shutdown for Sync

## Decision
Implemented clean shutdown on SIGINT/SIGTERM for the sync command to prevent data corruption and ensure resources are released properly.

## Implementation Details

### Signal Handling
- `cmd/ob-go/main.go` uses `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`
- Context propagates through Cobra to all commands

### Sync Command Cleanup
- `internal/cli/sync.go`: `defer engine.Close()` added for both one-time and continuous sync

### Context-Aware Plan Execution
- `internal/sync/engine.go`: `executePlan` now takes `ctx context.Context` as first parameter
- Checks `ctx.Err()` before each sync action
- Returns immediately if context is cancelled, preventing mid-batch state saves

### Continuous Sync Specifics
- `internal/sync/continuous.go`: `context.AfterFunc(ctx, connB.Close)` added for execution connection
- This unblocks any hanging websocket I/O when signal fires
- All goroutines (read pump, heartbeat, watcher, debounce timer) listen on `ctx.Done()`

### Safety Guarantees
- Lock file is removed via `defer` in both modes
- State is NOT saved for incomplete batches (next run resumes from last known good state)
- Websocket connections are closed to unblock I/O immediately

## Files Modified
- `src-go/cmd/ob-go/main.go`
- `src-go/internal/cli/sync.go`
- `src-go/internal/sync/engine.go`
- `src-go/internal/sync/continuous.go`
- `src-go/internal/sync/engine_test.go`
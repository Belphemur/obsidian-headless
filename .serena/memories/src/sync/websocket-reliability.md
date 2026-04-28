## WebSocket Reliability Fix (2026-04-27)

### Problem
Continuous sync experienced hourly WebSocket disconnections (close 1006). After the first reconnect, the client entered a rapid reconnect/disconnect cycle (~every minute).

### Root Cause
`startHeartbeat()` in `src/internal/sync/continuous.go` was called only once at startup. When the connection dropped, the heartbeat goroutine saw `conn == nil` and `return`ed permanently. Subsequent reconnects had no keep-alive traffic, so proxies/load balancers dropped them after short idle timeouts (e.g., 60s).

### Fix
1. **Surviving heartbeat**: Changed both `return`s inside `startHeartbeat` to `continue` so the goroutine persists across reconnects. After a heartbeat timeout close, it also nils `cs.conn` under the lock to avoid double-close.
2. **Exponential backoff**: Replaced fixed 5s reconnect delay with exponential backoff capped at 60s.
3. **Test coverage**: Added `TestContinuousHeartbeatAfterReconnect` and mock server `ping`/`pong` support to verify keep-alive resumes after reconnect.

### Files Changed
- `src/internal/sync/continuous.go`
- `src/internal/sync/continuous_test.go`
- `src/internal/sync/engine_test.go`

### PR
https://github.com/Belphemur/obsidian-headless/pull/15

### Issue
https://github.com/Belphemur/obsidian-headless/issues/16
# cenkalti/backoff v4 → v5 Migration (2026-04-29)

## Changes Made
- Updated `src/go.mod`: `github.com/cenkalti/backoff/v4 v4.3.0` → `github.com/cenkalti/backoff/v5 v5.0.3`
- Updated all 4 Go source files with v4 imports to v5:
  - `src/internal/api/retry.go` — rewrote `withRetry()` to use generic `Retry[struct{}]` API
  - `src/internal/api/retry_test.go` — import only
  - `src/internal/api/helpers.go` — import only
  - `src/internal/api/publish.go` — import only

## Key API Changes
1. `RetryNotify()` removed → `Retry[T any](ctx, operation, opts...)` returns `(T, error)`
2. `WithContext()` removed → context is first arg to `Retry`
3. `ExponentialBackOff.MaxElapsedTime` field removed → use `WithMaxElapsedTime(d)` option
4. `backoff.Permanent(err)` unchanged
5. `backoff.NewExponentialBackOff()` unchanged

## Build/Test Status
- Build: PASS (all packages compile)
- Tests: PASS (all packages, including retry_test.go)
- No remaining v4 imports anywhere in repo
- Zero behavior change; all retry semantics preserved identically

# cenkalti/backoff v5 → v7 Migration (2026-07-03)

## Changes Made
- Updated `src/go.mod`: `github.com/cenkalti/backoff/v5 v5.0.3` → `github.com/cenkalti/backoff/v7 v7.0.0`
  (also removed an accidental duplicate `require` line for the module).
- Updated all 4 Go source files with v5 imports to v7 (import path only, API unchanged):
  - `src/internal/api/retry.go`
  - `src/internal/api/retry_test.go`
  - `src/internal/api/helpers.go`
  - `src/internal/api/publish.go`
- Ran `go mod tidy` to refresh `go.sum`.

## Key API Changes (v5 → v6 → v7)
1. `Retry` now returns a `*RetryError` on every failure (success/permanent/exhausted/timeout)
   instead of the bare operation error. `RetryError` has `LastErr` (last operation error) and
   `Cause` (`ErrPermanent`, `ErrExhausted`, `ErrMaxElapsedTime`, or a context cancellation cause).
2. `backoff.AsRetryError(err)` extracts the `*RetryError` from an error chain.
3. `WithMaxElapsedTime` now reports `ErrMaxElapsedTime` cause instead of `context.DeadlineExceeded`.
4. `PermanentError` type removed; use `Permanent(err)` (unchanged) + `errors.Is(err, ErrPermanent)`.
5. (v7 only) `RetryAfter` now takes `(d time.Duration, cause error)` — not used in this repo.

## Fix Required for Behavior Preservation
- `src/internal/api/retry.go`'s `withRetry()` unwraps the `*RetryError` via `backoff.AsRetryError`
  and returns `retryErr.LastErr` instead of the raw `*RetryError`. This preserves the previous
  (v5) behavior where callers (e.g. `helpers_test.go`'s `TestPostJSON_APIError`) type-assert the
  returned error directly to `*APIError`/`*circuitbreaker.BreakerError` rather than needing
  `errors.As` through an extra wrapper.

## Build/Test Status
- Build: PASS (`go build ./...`)
- Vet: PASS (`go vet ./...`)
- Tests: PASS (`go test ./...`), including the `TestPostJSON_APIError` case that failed until
  `withRetry` was updated to unwrap `*RetryError`.
- No remaining v5 imports anywhere in repo.

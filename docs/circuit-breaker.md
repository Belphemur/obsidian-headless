# Circuit Breaker

The circuit breaker pattern protects the Obsidian headless CLI from cascading failures when the Obsidian API or sync servers are overloaded or unreachable. It uses [`sony/gobreaker/v2`](https://github.com/sony/gobreaker) to detect unhealthy backends and fail fast, while allowing automatic recovery.

## Overview

When a remote service degrades, continuing to hammer it with requests wastes resources and delays user feedback. A circuit breaker sits between the caller and the service, tracking recent failures. Once failures exceed a threshold, the breaker "opens" and immediately rejects new requests without touching the network. After a cooldown period, it enters a "half-open" probing state to test if the service has recovered.

This project deploys circuit breakers at two levels:

1. **HTTP API breaker** — a single shared breaker protecting all REST API calls (authentication, vault management, publish).
2. **WebSocket sync breaker** — one breaker per vault protecting WebSocket sync connections.

## Architecture

The circuit breaker is layered inside the existing retry mechanism, following the ["Polly pattern"](https://github.com/App-vNext/Polly#policywrap): retry wraps breaker wraps transport.

```
Request → Retry (exponential backoff) → Circuit Breaker → HTTP/WS call
```

Each retry attempt passes through the breaker. When the breaker is open, `gobreaker.ErrOpenState` is returned immediately and treated as a permanent error by the retrier, so no more retries are attempted until the breaker transitions to half-open.

### Retry + Breaker Flow

```
┌─────────────────────────────────────────────────────────┐
│  Retry Policy (cenkalti/backoff)                        │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │  Circuit Breaker (sony/gobreaker)                 │  │
│  │                                                   │  │
│  │  ┌─────────────────────────────────────────────┐  │  │
│  │  │  HTTP Call (api.Client.postJSON)            │  │  │
│  │  │  — or —                                     │  │  │
│  │  │  WS Connect (sync.Engine.dialWorker)        │  │  │
│  │  └─────────────────────────────────────────────┘  │  │
│  │                                                   │  │
│  │  On gobreaker.ErrOpenState → backoff.Permanent    │  │
│  └───────────────────────────────────────────────────┘  │
│                                                         │
│  On timeout / network error → retry with backoff        │
└─────────────────────────────────────────────────────────┘
```

### HTTP Breaker (Shared)

All REST API endpoints hit the same backend (`api.obsidian.md`), so a single breaker instance on `api.Client` protects auth, vault, and publish calls alike. If the API backend is overloaded, there is no point distinguishing between endpoint failures — they all indicate the same underlying problem.

### WebSocket Breaker (Per-Vault)

Each vault may connect to a different sync host (e.g., `sync-1.obsidian.md` vs `sync-2.obsidian.md`). A per-vault breaker on `sync.Engine` isolates failures: one overloaded sync host does not block connections to others.

## Configuration

### HTTP API Breaker

| Setting | Value | Rationale |
|---------|-------|-----------|
| **Name** | `"obsidian-api"` | Identifies the breaker in logs |
| **MaxRequests** | `3` | Number of probes allowed in half-open state |
| **Interval** | `30s` | Rolling window for failure counting |
| **Timeout** | `30s` | Duration the breaker stays open before probing |
| **ReadyToTrip** | 5 consecutive failures | Opens after 5 failures in a row |
| **IsExcluded** | `context.Canceled`, `context.DeadlineExceeded` | Client-side cancellation is not a service health indicator |
| **IsSuccessful** | `nil` error = success; any error = failure | Includes "overloaded" responses as failures |

### WebSocket Breaker (Per-Vault)

| Setting | Value | Rationale |
|---------|-------|-----------|
| **Name** | `"obsidian-sync-ws-{vaultID}"` | Identifies the vault in logs |
| **MaxRequests** | `1` | Binary state — one probe in half-open is enough |
| **Interval** | `0` | No rolling window needed; binary open/closed |
| **Timeout** | `60s` | Longer cooldown for WS reconnection cycles |
| **ReadyToTrip** | 3 consecutive failures | Opens after 3 consecutive connect failures |
| **IsExcluded** | None | All WS failures count toward the threshold |

## State Machine

The circuit breaker follows a three-state cycle:

### Closed (Normal Operation)

- All requests pass through to the underlying service.
- Successes and failures are tracked within the rolling window (`Interval`).
- When `ReadyToTrip` failures are detected (consecutive), the breaker transitions to **Open**.

### Open (Failing Fast)

- All requests are rejected immediately with `gobreaker.ErrOpenState`.
- The breaker remains open for the `Timeout` duration (30s for HTTP, 60s for WS).
- No network calls are made — the client fails fast and conserves resources.
- After `Timeout` expires, the breaker transitions to **Half-Open**.

### Half-Open (Probing)

- A limited number of requests (`MaxRequests`) are allowed through as probes.
- If a probe succeeds, the breaker transitions back to **Closed** (service recovered).
- If a probe fails, the breaker returns to **Open** (service still degraded).
- This prevents a recovered-but-fragile service from being immediately overwhelmed.

```
                  ReadyToTrip met
  ┌────────┐  ─────────────────►  ┌──────┐
  │        │                      │      │
  │ CLOSED │                      │ OPEN │
  │        │◄──────────────────── │      │
  └───┬────┘   Probe succeeds     └──┬───┘
      │                              │
      │                              │ Timeout expires
      │                              │
      │           Probe fails        ▼
      │     ┌──────────────►  ┌─────────────┐
      │     │                 │             │
      └─────┴─────────────────│  HALF-OPEN  │
                              │             │
                              └─────────────┘
```

### State Change Logging

All state transitions are logged via `zerolog` at **Warn** level:

```
circuit breaker obsidian-api state changed from closed to open
circuit breaker obsidian-api state changed from open to half-open
circuit breaker obsidian-api state changed from half-open to closed
```

## Retry Integration

The retry and circuit breaker work together via the Polly pattern:

1. **Retry** (outer layer) manages `cenkalti/backoff` exponential backoff with jitter.
2. **Circuit Breaker** (inner layer) decides whether a request should even attempt the network.
3. **Transport** (HTTP/WS) performs the actual I/O.

When the breaker is open, it returns `gobreaker.ErrOpenState`. The retry layer detects this error and wraps it as `backoff.Permanent`, which stops all retries immediately. This is correct because:

- Retrying while the breaker is open would only produce the same error.
- The breaker will transition to half-open on its own timer.
- Stopping retries gives the user immediate feedback instead of an artificial delay.

### Consecutive Failure Counting

Overloaded 200 responses count as a failure per retry attempt. After 5 consecutive overloaded responses (each one a separate retry attempt within the rolling window), the breaker opens. Subsequent retry attempts hit the open breaker and stop immediately.

## Package Wiring

### `src/internal/circuitbreaker/`

New package containing:

- Factory functions for creating HTTP and WebSocket breakers with the correct configuration.
- `BreakerError` type — wraps `gobreaker.ErrOpenState` with a user-friendly message.
- `IsBreakerError(err error) bool` — helper to detect when an error originated from an open circuit.

### `src/internal/api/`

The HTTP breaker is attached to the `api.Client` struct. It wraps:

- `postJSON()` — all POST requests (auth, vault, publish metadata).
- `uploadPublishedFile()` — file uploads to publish hosts.

Every outgoing HTTP call passes through the shared breaker before reaching the network.

### `src/internal/sync/`

The WebSocket breaker is attached to the per-vault `sync.Engine` struct. It wraps:

- `ensureConnected()` — the high-level connection gate.
- `connect()` — the init handshake sequence.
- `dialWorker()` — the low-level WebSocket dial operation.

A per-vault breaker name is constructed at engine creation time: `"obsidian-sync-ws-{vaultID}"`.

### `src/internal/cli/`

The `App` struct caches the `api.Client` (with its breaker) across commands. When a breaker error surfaces to the CLI layer, it is translated into a user-friendly message rather than a raw Go error.

## Error Handling

### BreakerError Type

The `circuitbreaker` package defines a `BreakerError` type that wraps the underlying `gobreaker.ErrOpenState` with context:

```go
type BreakerError struct {
    Message string
    Err     error // gobreaker.ErrOpenState
}

func (e *BreakerError) Error() string { return e.Message }
```

### IsBreakerError Helper

```go
func IsBreakerError(err error) bool
```

Returns `true` if the error is or wraps a `BreakerError`, or if it is an unwrapped `gobreaker.ErrOpenState` or `gobreaker.ErrTooManyRequests` sentinel error. Used by the CLI and retry layers to detect open-circuit conditions.

### User-Facing Messages

The CLI translates breaker errors into actionable messages:

| Breaker | Message |
|---------|---------|
| HTTP API | `Obsidian API is temporarily unavailable (circuit open); retry in ~30s` |
| WS Sync | `Vault {id} sync is temporarily unavailable (circuit open); retry in ~60s` |

These messages indicate the nature of the problem (not the user's connection) and provide an expected recovery timeframe.

## Overloaded Server Handling

The Obsidian API can return `200 OK` with `"overloaded"` in the response body when the server is under strain. The circuit breaker treats this as a **failure**, not a success:

1. The `api.Client` parses the response body after receiving a 200 status.
2. If the body contains `"overloaded"`, the caller returns a non-nil error.
3. The `IsSuccessful` function on the breaker sees a non-nil error and counts it as a failure.
4. After enough consecutive overloaded responses, the breaker opens and new requests fail fast.

This design is intentional: an overloaded server is already struggling, and reducing request volume gives it time to recover. A raw 200 would otherwise be counted as success by the breaker, hiding the real server state.

package circuitbreaker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Belphemur/obsidian-headless/src-go/internal/circuitbreaker"
	"github.com/rs/zerolog"
	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
)

func TestHTTPDefault(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	s := circuitbreaker.HTTPDefault(logger)

	assert.Equal(t, "obsidian-api", s.Name)
	assert.Equal(t, uint32(3), s.MaxRequests)
	assert.Equal(t, 30*time.Second, s.Interval)
	assert.Equal(t, 30*time.Second, s.Timeout)
}

func TestSyncWS(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	vaultID := "test-vault-123"
	s := circuitbreaker.SyncWS(vaultID, logger)

	assert.Equal(t, "obsidian-sync-ws-test-vault-123", s.Name)
	assert.Equal(t, uint32(1), s.MaxRequests)
	assert.Equal(t, time.Duration(0), s.Interval)
	assert.Equal(t, 60*time.Second, s.Timeout)
}

func TestHTTPDefault_ReadyToTrip(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	s := circuitbreaker.HTTPDefault(logger)

	assert.False(t, s.ReadyToTrip(gobreaker.Counts{ConsecutiveFailures: 4}))
	assert.True(t, s.ReadyToTrip(gobreaker.Counts{ConsecutiveFailures: 5}))
	assert.True(t, s.ReadyToTrip(gobreaker.Counts{ConsecutiveFailures: 6}))
}

func TestSyncWS_ReadyToTrip(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	s := circuitbreaker.SyncWS("vault", logger)

	assert.False(t, s.ReadyToTrip(gobreaker.Counts{ConsecutiveFailures: 2}))
	assert.True(t, s.ReadyToTrip(gobreaker.Counts{ConsecutiveFailures: 3}))
	assert.True(t, s.ReadyToTrip(gobreaker.Counts{ConsecutiveFailures: 4}))
}

func TestBreakerError_WrapsSentinel(t *testing.T) {
	t.Parallel()
	breakerErr := &circuitbreaker.BreakerError{
		Message: "circuit is open",
		Err:     gobreaker.ErrOpenState,
	}

	assert.True(t, errors.Is(breakerErr, gobreaker.ErrOpenState))
}

func TestIsBreakerError(t *testing.T) {
	t.Parallel()
	// nil error should not be a breaker error
	assert.False(t, circuitbreaker.IsBreakerError(nil))

	// random error should not be a breaker error
	assert.False(t, circuitbreaker.IsBreakerError(errors.New("random error")))

	// BreakerError wrapping ErrOpenState
	breakerErr := &circuitbreaker.BreakerError{
		Message: "circuit is open",
		Err:     gobreaker.ErrOpenState,
	}
	assert.True(t, circuitbreaker.IsBreakerError(breakerErr))

	// BreakerError wrapping ErrTooManyRequests
	breakerErr2 := &circuitbreaker.BreakerError{
		Message: "too many requests",
		Err:     gobreaker.ErrTooManyRequests,
	}
	assert.True(t, circuitbreaker.IsBreakerError(breakerErr2))

	// wrapped BreakerError
	wrapped := errors.New("wrapped")
	wrappedBreaker := &circuitbreaker.BreakerError{
		Message: "wrapped breaker error",
		Err:     wrapped,
	}
	assert.True(t, circuitbreaker.IsBreakerError(wrappedBreaker))

	// Sentinel errors directly
	assert.True(t, circuitbreaker.IsBreakerError(gobreaker.ErrOpenState))
	assert.True(t, circuitbreaker.IsBreakerError(gobreaker.ErrTooManyRequests))

	// IsExcluded should not count as breaker error
	assert.False(t, circuitbreaker.IsBreakerError(context.Canceled))
}

func TestHTTPDefault_IsSuccessful(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	s := circuitbreaker.HTTPDefault(logger)

	assert.True(t, s.IsSuccessful(nil))
	assert.False(t, s.IsSuccessful(errors.New("some error")))
}

func TestSyncWS_IsSuccessful(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	s := circuitbreaker.SyncWS("vault", logger)

	assert.True(t, s.IsSuccessful(nil))
	assert.False(t, s.IsSuccessful(errors.New("some error")))
}

func TestHTTPDefault_IsExcluded(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	s := circuitbreaker.HTTPDefault(logger)

	assert.True(t, s.IsExcluded(context.Canceled))
	assert.False(t, s.IsExcluded(errors.New("some error")))
	assert.False(t, s.IsExcluded(nil))
}

func TestSyncWS_IsExcluded(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	s := circuitbreaker.SyncWS("vault", logger)

	assert.True(t, s.IsExcluded(context.Canceled))
	assert.False(t, s.IsExcluded(errors.New("some error")))
	assert.False(t, s.IsExcluded(nil))
}

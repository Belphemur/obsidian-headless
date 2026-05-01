// Package circuitbreaker provides factory functions for gobreaker.Settings
// and a custom BreakerError type for user-facing error messages.
package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/sony/gobreaker/v2"
)

// BreakerError wraps gobreaker sentinel errors (ErrOpenState, ErrTooManyRequests)
// with user-friendly messages that indicate the circuit breaker state.
type BreakerError struct {
	Message string
	Err     error
}

// Error returns the user-friendly message.
func (e *BreakerError) Error() string { return e.Message }

// Unwrap returns the underlying gobreaker error for errors.Is checks.
func (e *BreakerError) Unwrap() error { return e.Err }

// IsBreakerError returns true if err is or wraps a *BreakerError,
// gobreaker.ErrOpenState, or gobreaker.ErrTooManyRequests.
func IsBreakerError(err error) bool {
	if err == nil {
		return false
	}

	var breakerErr *BreakerError
	if errors.As(err, &breakerErr) {
		return true
	}

	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return true
	}

	return false
}

// HTTPDefault returns gobreaker.Settings for the shared REST API breaker.
func HTTPDefault(logger zerolog.Logger) gobreaker.Settings {
	return gobreaker.Settings{
		Name:        "obsidian-api",
		MaxRequests: 3,
		Interval:    30 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
		IsSuccessful: func(err error) bool {
			return err == nil
		},
		IsExcluded: func(err error) bool {
			return errors.Is(err, context.Canceled)
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			logger.Warn().
				Str("name", name).
				Str("from", from.String()).
				Str("to", to.String()).
				Msgf("circuit breaker %s state changed from %s to %s", name, from.String(), to.String())
		},
	}
}

// SyncWS returns gobreaker.Settings for a per-vault WebSocket breaker.
func SyncWS(vaultID string, logger zerolog.Logger) gobreaker.Settings {
	return gobreaker.Settings{
		Name:        fmt.Sprintf("obsidian-sync-ws-%s", vaultID),
		MaxRequests: 1,
		Interval:    0,
		Timeout:     60 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
		IsSuccessful: func(err error) bool {
			return err == nil
		},
		IsExcluded: func(err error) bool {
			return errors.Is(err, context.Canceled)
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			logger.Warn().
				Str("name", name).
				Str("from", from.String()).
				Str("to", to.String()).
				Msgf("circuit breaker %s state changed from %s to %s", name, from.String(), to.String())
		},
	}
}

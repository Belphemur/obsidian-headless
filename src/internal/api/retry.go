package api

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v7"
)

func (c *Client) withRetry(ctx context.Context, operation func() error) error {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 1 * time.Second
	b.Multiplier = 2
	b.RandomizationFactor = 0.5
	b.MaxInterval = 5 * time.Minute

	_, err := backoff.Retry[struct{}](ctx, func() (struct{}, error) {
		return struct{}{}, operation()
	}, backoff.WithBackOff(b), backoff.WithMaxElapsedTime(5*time.Minute))
	if err == nil {
		return nil
	}
	// backoff v6+ wraps every failure in a *RetryError. Unwrap it so callers
	// keep seeing the original operation error, preserving prior behavior.
	if retryErr := backoff.AsRetryError(err); retryErr != nil {
		return retryErr.LastErr
	}
	return err
}

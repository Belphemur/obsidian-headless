package api

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
)

func (c *Client) withRetry(ctx context.Context, operation func() error) error {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 1 * time.Second
	b.Multiplier = 2
	b.RandomizationFactor = 0.5
	b.MaxInterval = 5 * time.Minute
	b.MaxElapsedTime = 5 * time.Minute

	return backoff.RetryNotify(operation, backoff.WithContext(b, ctx), nil)
}

package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
)

func TestWithRetry_SuccessFirstTry(t *testing.T) {
	client := New("", 5*time.Second)
	calls := 0
	op := func() error {
		calls++
		return nil
	}
	if err := client.withRetry(context.Background(), op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestWithRetry_PermanentError(t *testing.T) {
	client := New("", 5*time.Second)
	calls := 0
	op := func() error {
		calls++
		return backoff.Permanent(errors.New("permanent"))
	}
	if err := client.withRetry(context.Background(), op); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestWithRetry_ContextCancelled(t *testing.T) {
	client := New("", 5*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	op := func() error {
		calls++
		cancel()
		return errors.New("transient")
	}
	if err := client.withRetry(ctx, op); err == nil {
		t.Fatal("expected error")
	}
	if calls < 1 {
		t.Errorf("calls = %d, want >= 1", calls)
	}
}

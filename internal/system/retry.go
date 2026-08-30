package system

import (
	"context"
	"errors"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

// DefaultBackoff returns the exponential-backoff policy shared by okdctl's
// retry call sites: 5s initial delay, factor 2, 0.5 jitter, 3 steps, capped
// at 5 minutes.
func DefaultBackoff() wait.Backoff {
	return wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   2,
		Jitter:   0.5,
		Steps:    3,
		Cap:      5 * time.Minute,
	}
}

// Retry runs op under wait.ExponentialBackoffWithContext using backoff b,
// calling retryable(err) to decide whether each failure aborts (false) or
// consumes another step (true). On exhaustion it returns the last op error
// instead of wait's internal timeout sentinel, except a ctx
// cancellation/deadline error from op itself, which passes through unchanged.
func Retry(ctx context.Context, b wait.Backoff, retryable func(error) bool, op func(ctx context.Context) error) error {
	var lastErr error
	err := wait.ExponentialBackoffWithContext(ctx, b, func(ctx context.Context) (bool, error) {
		if opErr := op(ctx); opErr != nil {
			lastErr = opErr
			if !retryable(opErr) {
				return false, opErr
			}
			return false, nil
		}
		return true, nil
	})
	if err == nil {
		return nil
	}
	if lastErr != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return lastErr
	}
	return err
}

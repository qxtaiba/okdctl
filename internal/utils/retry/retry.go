package retry

import (
	"context"
	"time"
)

// Do retries fn up to attempts times with exponential backoff starting at initialBackoff.
// It returns nil on the first successful call, or the last error after all attempts are exhausted.
// Context cancellation is checked between retries.
func Do(ctx context.Context, attempts int, initialBackoff time.Duration, fn func() error) error {
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	backoff := initialBackoff

	for i := 0; i < attempts; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := fn(); err != nil {
			lastErr = err
			if i < attempts-1 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
				}
				backoff *= 2
			}
			continue
		}
		return nil
	}

	return lastErr
}

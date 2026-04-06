// Package retry provides context-aware retry helpers with exponential
// backoff, used for transient failures against cluster APIs, the Proxmox
// provider, and remote downloads.
package retry

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// Do retries fn up to attempts times with exponential backoff starting at initialBackoff.
// It returns nil on the first successful call, or the last error after all attempts are exhausted.
// Context cancellation is checked between retries.
func Do(ctx context.Context, attempts int, initialBackoff time.Duration, fn func() error) error {
	if attempts < 1 {
		attempts = 1
	}

	b := backoff.NewExponentialBackOff()
	b.InitialInterval = initialBackoff
	b.Multiplier = 2
	b.RandomizationFactor = 0.5 // jitter to avoid thundering herd
	b.MaxInterval = 5 * time.Minute
	b.MaxElapsedTime = 0 // no time limit; retry count is the sole termination condition

	return backoff.Retry(fn, backoff.WithContext(
		backoff.WithMaxRetries(b, uint64(attempts-1)),
		ctx,
	))
}

package system

import (
	"context"
	"math/rand/v2"
	"time"
)

// Backoff is the policy Retry drives: Steps attempts, the first retry after
// Duration, each later delay multiplied by Factor and capped at Cap, with up
// to Jitter×delay of extra random wait.
type Backoff struct {
	Duration time.Duration
	Factor   float64
	Jitter   float64
	Steps    int
	Cap      time.Duration
}

// DefaultBackoff returns the policy shared by okdctl's retry call sites: 5s
// initial delay, factor 2, 0.5 jitter, 3 steps, capped at 5 minutes.
func DefaultBackoff() Backoff {
	return Backoff{
		Duration: 5 * time.Second,
		Factor:   2,
		Jitter:   0.5,
		Steps:    3,
		Cap:      5 * time.Minute,
	}
}

// Retry runs op up to b.Steps times (at least once), waiting per b between
// attempts and stopping as soon as retryable(err) is false. It returns nil on
// success, ctx.Err() when cancelled while waiting, else the last op error.
func Retry(ctx context.Context, b Backoff, retryable func(error) bool, op func(ctx context.Context) error) error {
	steps := max(b.Steps, 1)
	delay := b.Duration
	var lastErr error
	for attempt := range steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = op(ctx)
		if lastErr == nil {
			return nil
		}
		if !retryable(lastErr) || attempt == steps-1 {
			return lastErr
		}
		wait := delay
		if b.Jitter > 0 {
			wait += time.Duration(rand.Float64() * b.Jitter * float64(delay)) //nolint:gosec // G404: jitter, not a secret
		}
		if b.Factor > 0 {
			delay = time.Duration(float64(delay) * b.Factor)
		}
		if b.Cap > 0 && delay > b.Cap {
			delay = b.Cap
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return lastErr
}

package download

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

// httpStatusError carries the HTTP status returned from a download attempt
// so isRetryable can tell 4xx (fail fast) from 5xx (retry). Callers wrap
// this with description context before returning it.
type httpStatusError struct {
	Status int
	URL    string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("HTTP %d from %s", e.Status, e.URL)
}

// isRetryable reports whether err should trigger another download attempt.
// 5xx responses and transport errors (net.Dial, TLS, reset, DNS) are
// retryable. 4xx responses and context cancellation are not.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var httpErr *httpStatusError
	if errors.As(err, &httpErr) {
		return httpErr.Status >= http.StatusInternalServerError
	}
	return true
}

// retryDownload runs fn with exponential backoff (5s base, factor 2, jitter
// 0.5, 3 attempts, 5-minute cap) and returns how many attempts were made
// and the final error. Non-retryable failures abort immediately; context
// cancellation returns ctx.Err().
func retryDownload(ctx context.Context, fn func() error) (int, error) {
	var attempts int
	var lastErr error
	backoff := wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   2,
		Jitter:   0.5,
		Steps:    3,
		Cap:      5 * time.Minute,
	}
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(_ context.Context) (bool, error) {
		attempts++
		if fnErr := fn(); fnErr != nil {
			lastErr = fnErr
			if !isRetryable(fnErr) {
				return false, fnErr
			}
			return false, nil
		}
		return true, nil
	})
	if err == nil {
		return attempts, nil
	}
	if lastErr != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return attempts, lastErr
	}
	return attempts, err
}

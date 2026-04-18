package download

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"k8s.io/apimachinery/pkg/util/wait"
)

// httpStatusError carries the HTTP status returned from a download attempt
// so isRetryable can tell 4xx (fail fast) from 5xx (retry).
type httpStatusError struct {
	Status int
	Method string
	URL    string
	// Body is a ≤256-byte excerpt of the response body with non-printable
	// bytes stripped. No credential scrubbing — callers who persist these
	// errors (debug-bundle, log files) are responsible for redaction.
	Body string
}

func (e *httpStatusError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("HTTP %d %s %s: %s", e.Status, e.Method, e.URL, e.Body)
	}
	return fmt.Sprintf("HTTP %d %s %s", e.Status, e.Method, e.URL)
}

// bodySnippet trims a response-body read down to a printable string. Control
// bytes are stripped so terminal escape sequences in an error body cannot
// corrupt the display. truncated appends "..." when the caller stopped at
// the read cap.
func bodySnippet(raw []byte, truncated bool) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) || r == '\n' || r == '\r' || r == '\t' {
			return r
		}
		return -1
	}, string(raw))
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return ""
	}
	if truncated {
		return clean + "..."
	}
	return clean
}

// isRetryable reports whether err should trigger another download attempt.
// 5xx responses and transport errors (net.Dial, TLS, reset, DNS) are
// retryable, as are 408 Request Timeout and 429 Too Many Requests. Other
// 4xx responses and context cancellation are not.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var httpErr *httpStatusError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.Status >= http.StatusInternalServerError:
			return true
		case httpErr.Status == http.StatusRequestTimeout,
			httpErr.Status == http.StatusTooManyRequests:
			return true
		default:
			return false
		}
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

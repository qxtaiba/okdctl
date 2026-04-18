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
	// Body is a ≤256-byte excerpt of the response body, redacted when it
	// matches a bare-token heuristic to avoid leaking credentials in logs.
	Body string
}

func (e *httpStatusError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("HTTP %d %s %s: %s", e.Status, e.Method, e.URL, e.Body)
	}
	return fmt.Sprintf("HTTP %d %s %s", e.Status, e.Method, e.URL)
}

// redactBodySnippet returns a log-safe rendering of a response-body read.
// Non-printable control bytes are stripped; if the trimmed result looks like
// a bare secret (≥16 chars, URL-safe base64 alphabet, no whitespace) it is
// replaced with "<redacted>" so an error printed to logs does not leak a
// token. truncated means the caller stopped reading at the 256-byte cap and
// is used to append "..." so the reader knows the excerpt is partial.
func redactBodySnippet(raw []byte, truncated bool) string {
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
	if len(clean) >= 16 && !strings.ContainsAny(clean, " \t\n\r") {
		const urlSafeBase64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=_-"
		allSafe := true
		for _, r := range clean {
			if !strings.ContainsRune(urlSafeBase64, r) {
				allSafe = false
				break
			}
		}
		if allSafe {
			return "<redacted>"
		}
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

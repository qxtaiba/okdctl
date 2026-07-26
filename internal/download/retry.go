package download

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/qxtaiba/okdctl/internal/system"
)

// HTTPStatusError carries the HTTP status returned from a failed request so
// isRetryable can tell 4xx (fail fast) from 5xx (retry). Body is a ≤256-byte
// excerpt of the response body with non-printable bytes stripped; Error()
// applies no credential scrubbing — callers who persist its output are
// responsible for redaction. Structured log sinks are covered by Redacted.
type HTTPStatusError struct {
	Status int
	Method string
	URL    string
	Body   string
}

func (e *HTTPStatusError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("HTTP %d %s %s: %s", e.Status, e.Method, e.URL, e.Body)
	}
	return fmt.Sprintf("HTTP %d %s %s", e.Status, e.Method, e.URL)
}

// Redacted omits URL and Body so a tokenized/pre-signed URL or a
// credential-echoing response body never reaches a structured log sink via
// slog attrs; mirrors executor.ExitError.Redacted.
func (e *HTTPStatusError) Redacted() any {
	return struct {
		Status int
		Method string
	}{e.Status, e.Method}
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
	var httpErr *HTTPStatusError
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

// retryDownload runs fn with system.DefaultBackoff() and returns how many
// attempts were made and the final error. Non-retryable failures abort
// immediately; context cancellation returns ctx.Err().
func retryDownload(ctx context.Context, fn func() error) (int, error) {
	var attempts int
	err := system.Retry(ctx, system.DefaultBackoff(), isRetryable, func(context.Context) error {
		attempts++
		return fn()
	})
	return attempts, err
}

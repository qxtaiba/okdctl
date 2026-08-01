package download

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// HTTPStatusError carries the HTTP status returned from a failed request so
// isRetryable can tell 4xx (fail fast) from 5xx (retry). Body is a ≤256-byte
// excerpt of the response body with non-printable bytes stripped. Error()
// scrubs at the string layer — URL userinfo and query are stripped and Body
// runs through logutil's credential scrub — so the raw text stays safe even
// when a wrap (errtypes.NetworkError) hides HTTPStatusError from the
// Redacted() dispatch a structured log sink performs on the top-level value.
// Redacted omits URL and Body entirely for the slog-attr path.
type HTTPStatusError struct {
	Status int
	Method string
	URL    string
	Body   string
}

func (e *HTTPStatusError) Error() string {
	if e.Body != "" {
		body := fmt.Sprint(logutil.RedactableStderr(e.Body).Redacted())
		return fmt.Sprintf("HTTP %d %s %s: %s", e.Status, e.Method, redactURL(e.URL), body)
	}
	return fmt.Sprintf("HTTP %d %s %s", e.Status, e.Method, redactURL(e.URL))
}

// redactURL strips userinfo and the query string from raw so a pre-signed
// token in the query or credentials in userinfo never reach an error string
// or the log/debug-bundle sinks that stringify the wrap chain. An unparseable
// URL is replaced with a placeholder rather than echoed verbatim.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[unparseable-url]"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
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
// retryable, as are 408 Request Timeout, 429 Too Many Requests, and a
// per-attempt HTTP client timeout — net/http makes Client.Timeout satisfy
// errors.Is(err, context.DeadlineExceeded), so classifying DeadlineExceeded
// as non-retryable would disable the retry loop for the exact slow-mirror
// failure it exists for. Only genuine caller cancellation (context.Canceled,
// e.g. Ctrl-C) aborts here without consuming a backoff step; a cancelled
// caller deadline is still caught immediately by system.Retry's own ctx.Done
// check. Other 4xx responses stay fail-fast.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
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

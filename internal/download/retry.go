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

// HTTPStatusError carries a failed request's status for retry classification;
// Body is a ≤256-byte sanitized excerpt. Error() scrubs URL/body itself so a
// wrapping NetworkError can't hide HTTPStatusError from a log sink's Redacted()
// dispatch.
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

// redactURL strips userinfo/query so tokens or credentials never reach an error
// string or log sink; unparseable input yields a placeholder.
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

// Redacted omits URL and Body so a tokenized URL or credential-echoing body
// never reaches a slog sink; mirrors executor.ExitError.Redacted.
func (e *HTTPStatusError) Redacted() any {
	return struct {
		Status int
		Method string
	}{e.Status, e.Method}
}

// bodySnippet trims raw to a printable string, stripping control bytes so
// terminal escapes can't corrupt the display; truncated appends "..." at the
// read cap.
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

// isRetryable classifies err for another attempt: 5xx, transport errors, 408/429, and per-attempt
// timeouts retry — Client.Timeout satisfies
// errors.Is(context.DeadlineExceeded), so treating that as non-retryable would
// break the slow-mirror case it exists for. Only context.Canceled aborts
// outright; other 4xx stay fail-fast.
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

// retryDownload runs fn with system.DefaultBackoff, returning the attempt count
// and final error; non-retryable failures abort immediately.
func retryDownload(ctx context.Context, fn func() error) (int, error) {
	var attempts int
	err := system.Retry(ctx, system.DefaultBackoff(), isRetryable, func(context.Context) error {
		attempts++
		return fn()
	})
	return attempts, err
}

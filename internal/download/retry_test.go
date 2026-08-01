package download

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestIsRetryable_PerAttemptTimeoutRetries locks the fix for the
// cancellation-identity trap: net/http makes a Client.Timeout error satisfy
// errors.Is(err, context.DeadlineExceeded), so a per-attempt HTTP deadline
// must stay retryable rather than aborting the loop it exists for.
func TestIsRetryable_PerAttemptTimeoutRetries(t *testing.T) {
	timeout := fmt.Errorf("Get %q: %w", "https://mirror.invalid/f", context.DeadlineExceeded)
	if !isRetryable(timeout) {
		t.Fatalf("isRetryable(client-timeout) = false; want true (per-attempt HTTP deadline must retry)")
	}
}

// TestIsRetryable_CallerCancelAborts holds the must-preserve contract:
// genuine caller cancellation (Ctrl-C) aborts immediately.
func TestIsRetryable_CallerCancelAborts(t *testing.T) {
	if isRetryable(fmt.Errorf("Get: %w", context.Canceled)) {
		t.Fatalf("isRetryable(context.Canceled) = true; want false (caller cancel must abort)")
	}
}

func TestIsRetryable_ClientErrorFailsFast(t *testing.T) {
	notFound := &HTTPStatusError{Status: http.StatusNotFound, Method: http.MethodGet, URL: "https://mirror.invalid/f"}
	if isRetryable(notFound) {
		t.Fatalf("isRetryable(404) = true; want false (4xx fails fast)")
	}
}

func TestIsRetryable_ServerErrorRetries(t *testing.T) {
	serverErr := &HTTPStatusError{Status: http.StatusBadGateway, Method: http.MethodGet, URL: "https://mirror.invalid/f"}
	if !isRetryable(serverErr) {
		t.Fatalf("isRetryable(502) = false; want true")
	}
}

// TestHTTPStatusError_ErrorScrubsURLAndBody guards the sink path a
// NetworkError wrap defeats: Error() must strip the URL query token and mask
// key-shaped secrets in the body so the raw string is safe wherever the wrap
// chain is stringified.
func TestHTTPStatusError_ErrorScrubsURLAndBody(t *testing.T) {
	e := &HTTPStatusError{
		Status: http.StatusForbidden,
		Method: http.MethodGet,
		URL:    "https://user:pass@mirror.invalid/f?token=hunter2",
		Body:   `{"token":"hunter2"}`,
	}
	got := e.Error()
	if strings.Contains(got, "hunter2") {
		t.Fatalf("Error() = %q; must not leak the query token or body secret", got)
	}
	if strings.Contains(got, "pass") {
		t.Fatalf("Error() = %q; must not leak URL userinfo", got)
	}
	if !strings.Contains(got, "403") || !strings.Contains(got, "mirror.invalid") {
		t.Errorf("Error() = %q; want status and host preserved", got)
	}
}

func TestRedactURL_Unparseable(t *testing.T) {
	got := redactURL("https://exa mple.invalid/\x7f")
	if strings.Contains(got, "example") {
		t.Fatalf("redactURL(bad) = %q; want placeholder for unparseable input", got)
	}
}

func TestHTTPStatusError_RedactedStillOmitsURLAndBody(t *testing.T) {
	e := &HTTPStatusError{
		Status: http.StatusForbidden,
		Method: http.MethodGet,
		URL:    "https://mirror.invalid/f?token=hunter2",
		Body:   "hunter2 rejected",
	}
	got := fmt.Sprint(e.Redacted())
	if strings.Contains(got, "hunter2") || strings.Contains(got, "mirror.invalid") {
		t.Fatalf("Redacted() = %q; must omit URL and body", got)
	}
}

package download

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Locks the DeadlineExceeded-vs-Canceled retry-classification trap (see isRetryable).
func TestIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "per-attempt timeout retries",
			err:  fmt.Errorf("Get %q: %w", "https://mirror.invalid/f", context.DeadlineExceeded),
			want: true,
		},
		{
			name: "caller cancel aborts",
			err:  fmt.Errorf("Get: %w", context.Canceled),
			want: false,
		},
		{
			name: "client error fails fast",
			err:  &HTTPStatusError{Status: http.StatusNotFound, Method: http.MethodGet, URL: "https://mirror.invalid/f"},
			want: false,
		},
		{
			name: "server error retries",
			err:  &HTTPStatusError{Status: http.StatusBadGateway, Method: http.MethodGet, URL: "https://mirror.invalid/f"},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryable(tc.err); got != tc.want {
				t.Fatalf("isRetryable(%v) = %v; want %v", tc.err, got, tc.want)
			}
		})
	}
}

// Guards the sink path a NetworkError wrap defeats — Error() must scrub URL/body itself.
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

// Locks the slog-sink contract: Redacted carries status/method only.
func TestHTTPStatusError_RedactedOmitsURLAndBody(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"body with token keyword", "token hunter2 rejected"},
		{"body without token keyword", "hunter2 rejected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &HTTPStatusError{
				Status: http.StatusForbidden,
				Method: http.MethodGet,
				URL:    "https://mirror.invalid/f?token=hunter2",
				Body:   tc.body,
			}
			got := fmt.Sprint(e.Redacted())
			if strings.Contains(got, "hunter2") || strings.Contains(got, "mirror.invalid") {
				t.Fatalf("Redacted() = %q; must omit URL and body", got)
			}
			if !strings.Contains(got, "403") || !strings.Contains(got, http.MethodGet) {
				t.Errorf("Redacted() = %q; want status and method preserved", got)
			}
		})
	}
}

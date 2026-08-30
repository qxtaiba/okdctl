package download

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func serveBody(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDownload_HappyPath(t *testing.T) {
	body := []byte("binary-content")
	srv := serveBody(t, body)

	dir := t.TempDir()
	out := filepath.Join(dir, "artifact.bin")

	if err := Fetch(
		context.Background(), srv.URL+"/artifact.bin", out,
		WithFetchChecksum(sha256Hex(body)),
		WithLogger(logutil.NopLogger),
	); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("content = %q; want %q", got, body)
	}

	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode = %o; want 0600", perm)
	}
}

// The backoff sleep here (~2.5-7.5s, jittered) is intentional and ungated so CI
// exercises real retry timing.
func TestRetryDownload_RetriableHTTPErrorSecondAttemptWins(t *testing.T) {
	calls := 0
	retryable := &HTTPStatusError{Status: http.StatusServiceUnavailable, Method: http.MethodGet, URL: "http://example.invalid/f"}

	attempts, err := retryDownload(context.Background(), func() error {
		calls++
		if calls == 1 {
			return retryable
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryDownload: %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d; want 2", attempts)
	}
}

func TestDownload_NonOKStatusReturnsHTTPStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	err := Fetch(
		context.Background(), srv.URL+"/missing", filepath.Join(dir, "out.bin"),
		WithLogger(logutil.NopLogger),
	)
	if err == nil {
		t.Fatal("expected error for HTTP 404; got nil")
	}

	var httpErr *HTTPStatusError
	if !errors.As(err, &httpErr) {
		t.Fatalf("err = %v; want errors.As(*HTTPStatusError) to succeed", err)
	}
	if httpErr.Status != http.StatusNotFound {
		t.Errorf("HTTPStatusError.Status = %d; want %d", httpErr.Status, http.StatusNotFound)
	}
}

func TestDownload_CtxCancelCleansPartialFile(t *testing.T) {
	started := make(chan struct{})
	unblock := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("chunk1"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(started)
		<-unblock
	}))
	defer srv.Close()
	defer close(unblock)

	dir := t.TempDir()
	out := filepath.Join(dir, "partial.bin")

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- Fetch(
			ctx, srv.URL+"/big", out,
			WithTimeout(30*time.Second),
			WithLogger(logutil.NopLogger),
		)
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("server never started streaming")
	}
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after ctx cancel; got nil")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Download did not return after ctx cancel")
	}

	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("partial file must not exist after ctx cancel; Stat err = %v", err)
	}
}

func TestDownload_SymlinkAtOutputPath(t *testing.T) {
	srv := serveBody(t, []byte("symlink-content"))

	dir := t.TempDir()
	target := filepath.Join(dir, "real-target.bin")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "link.bin")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := Fetch(
		context.Background(), srv.URL+"/artifact.bin", link,
		// WithOverwrite bypasses canSkipDownload's stat-through-symlink short-circuit.
		WithOverwrite(true),
		WithLogger(logutil.NopLogger),
	); err == nil {
		t.Fatal("expected error when OutputPath is a symlink; O_NOFOLLOW must reject it")
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile target: %v", err)
	}
	if string(got) != "original" {
		t.Errorf("symlink target must be unchanged; got %q", got)
	}
}

func TestCanSkipDownload(t *testing.T) {
	body := []byte("payload")
	goodSum := sha256Hex(body)
	badSum := strings.Repeat("0", 64)

	cases := []struct {
		name        string
		write       []byte // nil leaves the file absent
		sum         string
		want        bool
		wantRemoved bool
	}{
		{name: "file absent returns false", write: nil, sum: goodSum, want: false},
		{name: "zero-size file returns false", write: []byte{}, sum: goodSum, want: false},
		{name: "matching checksum returns true", write: body, sum: goodSum, want: true},
		{name: "checksum mismatch returns false and removes file", write: body, sum: badSum, want: false, wantRemoved: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f.bin")
			if tc.write != nil {
				if err := os.WriteFile(path, tc.write, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg := &dlConfig{
				outputPath:       path,
				expectedChecksum: tc.sum,
				logger:           logutil.NopLogger,
			}
			if got := canSkipDownload(context.Background(), cfg); got != tc.want {
				t.Errorf("canSkipDownload() = %v; want %v", got, tc.want)
			}
			if tc.wantRemoved {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("mismatched file must be removed; Stat err = %v", err)
				}
			}
		})
	}
}

package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestDownload_HappyPath(t *testing.T) {
	body := []byte("binary-content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	out := filepath.Join(dir, "artifact.bin")

	if err := Fetch(context.Background(), srv.URL+"/artifact.bin", out,
		WithChecksum(sha256Hex(body)),
		WithLogger(logutil.NopLogger),
	); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(body) {
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

// TestRetryDownload_RetriableHTTPErrorSecondAttemptWins drives retryDownload
// with a 503 (retriable transport error). The roadmap framed this as
// "checksum-mismatch retry" but verifyDownloadedFile does not retry on
// mismatch — it removes the file and returns. Retry is a transport-tier
// concern only; this test exercises that path. First-failure backoff is
// ~2.5-7.5 s with jitter; the sleep is intentional, not gated, so CI sees it.
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
	err := Fetch(context.Background(), srv.URL+"/missing", filepath.Join(dir, "out.bin"),
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
		done <- Fetch(ctx, srv.URL+"/big", out,
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
	body := []byte("symlink-content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "real-target.bin")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "link.bin")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := Fetch(context.Background(), srv.URL+"/artifact.bin", link,
		// WithOverwrite bypasses canSkipDownload, which would otherwise stat
		// through the symlink, see size>0, and short-circuit (no ExpectedChecksum).
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
	badSum := "0000000000000000000000000000000000000000000000000000000000000000"

	t.Run("file absent returns false", func(t *testing.T) {
		dir := t.TempDir()
		cfg := &dlConfig{
			outputPath:       filepath.Join(dir, "nope.bin"),
			expectedChecksum: goodSum,
			logger:           logutil.NopLogger,
		}
		if canSkipDownload(cfg) {
			t.Error("expected false for absent file")
		}
	})

	t.Run("zero-size file returns false", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.bin")
		if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := &dlConfig{
			outputPath:       path,
			expectedChecksum: goodSum,
			logger:           logutil.NopLogger,
		}
		if canSkipDownload(cfg) {
			t.Error("expected false for zero-size file")
		}
	})

	t.Run("matching checksum returns true", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ok.bin")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := &dlConfig{
			outputPath:       path,
			expectedChecksum: goodSum,
			logger:           logutil.NopLogger,
		}
		if !canSkipDownload(cfg) {
			t.Error("expected true for matching checksum")
		}
	})

	t.Run("checksum mismatch returns false and removes file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.bin")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := &dlConfig{
			outputPath:       path,
			expectedChecksum: badSum,
			logger:           logutil.NopLogger,
		}
		if canSkipDownload(cfg) {
			t.Error("expected false for checksum mismatch")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("mismatched file must be removed; Stat err = %v", err)
		}
	})
}

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
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

func TestCalculateChecksum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	data := []byte("payload")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	sum, err := CalculateChecksum(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(data)
	if sum != hex.EncodeToString(want[:]) {
		t.Errorf("sum = %s; want %x", sum, want)
	}
}

func TestValidateChecksum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	data := []byte("payload")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(data)
	good := hex.EncodeToString(want[:])

	t.Run("empty expected bypasses check", func(t *testing.T) {
		if err := ValidateChecksum(context.Background(), path, ""); err != nil {
			t.Errorf("empty checksum must disable validation; got %v", err)
		}
	})

	t.Run("matching checksum succeeds", func(t *testing.T) {
		if err := ValidateChecksum(context.Background(), path, good); err != nil {
			t.Errorf("good checksum failed: %v", err)
		}
	})

	t.Run("mismatching checksum fails", func(t *testing.T) {
		if err := ValidateChecksum(context.Background(), path, strings.Repeat("0", 64)); err == nil {
			t.Error("expected mismatch error")
		}
	})

	t.Run("missing file surfaces error", func(t *testing.T) {
		if err := ValidateChecksum(context.Background(), filepath.Join(dir, "nope"), good); err == nil {
			t.Error("expected open error")
		}
	})
}

func TestFetchChecksum(t *testing.T) {
	validHex := strings.Repeat("a", 64)
	otherHex := strings.Repeat("b", 64)

	tests := []struct {
		name     string
		body     string
		status   int
		filename string
		want     string
		wantErr  bool
	}{
		{
			name:     "plain line",
			body:     validHex + "  okdctl.tar.gz\n",
			status:   200,
			filename: "okdctl.tar.gz",
			want:     validHex,
		},
		{
			name:     "leading asterisk stripped (binary mode)",
			body:     validHex + " *okdctl.tar.gz\n",
			status:   200,
			filename: "okdctl.tar.gz",
			want:     validHex,
		},
		{
			name:     "leading ./ stripped",
			body:     validHex + "  ./okdctl.tar.gz\n",
			status:   200,
			filename: "okdctl.tar.gz",
			want:     validHex,
		},
		{
			name:     "skips blanks and comments",
			body:     "# header\n\n" + validHex + "  okdctl.tar.gz\n",
			status:   200,
			filename: "okdctl.tar.gz",
			want:     validHex,
		},
		{
			name:     "picks matching filename when multiple entries",
			body:     otherHex + "  other.tar.gz\n" + validHex + "  okdctl.tar.gz\n",
			status:   200,
			filename: "okdctl.tar.gz",
			want:     validHex,
		},
		{
			name:     "filename not present",
			body:     validHex + "  other.tar.gz\n",
			status:   200,
			filename: "okdctl.tar.gz",
			wantErr:  true,
		},
		{
			name:     "malformed checksum - wrong length",
			body:     "abc okdctl.tar.gz\n",
			status:   200,
			filename: "okdctl.tar.gz",
			wantErr:  true,
		},
		{
			name:     "malformed checksum - non-hex",
			body:     strings.Repeat("z", 64) + "  okdctl.tar.gz\n",
			status:   200,
			filename: "okdctl.tar.gz",
			wantErr:  true,
		},
		{
			name:     "HTTP non-200 is error",
			body:     "",
			status:   404,
			filename: "okdctl.tar.gz",
			wantErr:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			got, err := FetchChecksum(context.Background(), srv.URL, tc.filename)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error; got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("context cancelled before request", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(200)
		}))
		defer srv.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := FetchChecksum(ctx, srv.URL, "x"); err == nil {
			t.Error("expected ctx error")
		}
	})
}

func TestVerifyDownloadedFile(t *testing.T) {
	nop := logutil.NopLogger
	data := []byte("artifact content")
	sum := sha256.Sum256(data)
	goodHex := hex.EncodeToString(sum[:])
	badHex := strings.Repeat("0", 64)

	t.Run("empty expected checksum is a no-op", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "artifact.tar.gz")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := verifyDownloadedFile(context.Background(), path, "", nop); err != nil {
			t.Errorf("empty checksum must be a no-op; got %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("file must be untouched; got %v", err)
		}
	})

	t.Run("matching checksum leaves file intact", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "artifact.tar.gz")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := verifyDownloadedFile(context.Background(), path, goodHex, nop); err != nil {
			t.Errorf("good checksum failed: %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("file must be untouched; got %v", err)
		}
	})

	t.Run("mismatching checksum returns error and removes file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "artifact.tar.gz")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := verifyDownloadedFile(context.Background(), path, badHex, nop); err == nil {
			t.Error("expected mismatch error")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("file must be removed after mismatch; stat err = %v", err)
		}
	})
}

func TestFetchChecksum_NonOKStatusReturnsHTTPStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := FetchChecksum(context.Background(), srv.URL, "okdctl.tar.gz")
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

func TestCalculateChecksum_CtxCancelled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.bin")
	if err := os.WriteFile(path, make([]byte, 4<<20), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := CalculateChecksum(ctx, path); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want context.Canceled", err)
	}
}

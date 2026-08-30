package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

func writeTempFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestValidateChecksum(t *testing.T) {
	data := []byte("payload")
	path := writeTempFile(t, "a.txt", data)
	good := sha256Hex(data)

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
		if err := ValidateChecksum(context.Background(), filepath.Join(filepath.Dir(path), "nope"), good); err == nil {
			t.Error("expected open error")
		}
	})
}

func TestVerifyDownloadedFile(t *testing.T) {
	nop := logutil.NopLogger
	data := []byte("artifact content")
	goodHex := sha256Hex(data)
	badHex := strings.Repeat("0", 64)

	t.Run("matching checksum leaves file intact", func(t *testing.T) {
		path := writeTempFile(t, "artifact.tar.gz", data)
		if err := verifyDownloadedFile(context.Background(), path, goodHex, nop); err != nil {
			t.Errorf("good checksum failed: %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("file must be untouched; got %v", err)
		}
	})

	t.Run("mismatching checksum returns error and removes file", func(t *testing.T) {
		path := writeTempFile(t, "artifact.tar.gz", data)
		if err := verifyDownloadedFile(context.Background(), path, badHex, nop); err == nil {
			t.Error("expected mismatch error")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("file must be removed after mismatch; stat err = %v", err)
		}
	})
}

func TestCalculateChecksum_CtxCancelled(t *testing.T) {
	path := writeTempFile(t, "big.bin", make([]byte, 4<<20))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := CalculateChecksum(ctx, path); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want context.Canceled", err)
	}
}

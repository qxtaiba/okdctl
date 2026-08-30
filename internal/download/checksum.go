package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// checksumChunkSize bounds reads between ctx checks so hashing a multi-GB ISO
// can be cancelled promptly.
const checksumChunkSize = 2 << 20 // 2 MiB

// ctxReader fails with ctx.Err() once ctx is done, checked before every Read.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (cr *ctxReader) Read(p []byte) (int, error) {
	if err := cr.ctx.Err(); err != nil {
		return 0, err
	}
	return cr.r.Read(p)
}

// CalculateChecksum returns the hex-encoded SHA-256 of the file at path.
func CalculateChecksum(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s for checksum: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	buf := make([]byte, checksumChunkSize)
	if _, err := io.CopyBuffer(hasher, &ctxReader{ctx: ctx, r: file}, buf); err != nil {
		return "", fmt.Errorf("read file for checksum: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// ValidateChecksum compares the SHA-256 of path against expectedChecksum.
// An empty expectedChecksum disables the check and returns nil.
func ValidateChecksum(ctx context.Context, path, expectedChecksum string) error {
	if expectedChecksum == "" {
		return nil
	}

	actualChecksum, err := CalculateChecksum(ctx, path)
	if err != nil {
		return err
	}

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch:\n  expected: %s\n  actual:   %s",
			expectedChecksum, actualChecksum)
	}

	return nil
}

// verifyDownloadedFile checks path's checksum and removes it on failure so a
// corrupt download never survives on disk.
func verifyDownloadedFile(ctx context.Context, path, expectedChecksum string, logger *slog.Logger) error {
	if expectedChecksum == "" {
		return nil
	}

	filename := filepath.Base(path)

	logger.Info("download: verifying checksum", "file", filename)

	if err := ValidateChecksum(ctx, path, expectedChecksum); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("verify %s: %w", filename, err)
	}

	logger.Info("download: checksum verified", "file", filename)

	return nil
}

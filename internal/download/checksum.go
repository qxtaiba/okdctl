package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okdctl/internal/httputil"
)

const maxChecksumFileSize = 1024 * 1024

// CalculateChecksum returns the hex-encoded SHA-256 of the file at path.
func CalculateChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open %s for checksum: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("failed to read file for checksum: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// ValidateChecksum compares the SHA-256 of path against expectedChecksum.
// An empty expectedChecksum disables the check and returns nil.
func ValidateChecksum(path, expectedChecksum string) error {
	if expectedChecksum == "" {
		return nil
	}

	actualChecksum, err := CalculateChecksum(path)
	if err != nil {
		return err
	}

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch:\n  expected: %s\n  actual:   %s",
			expectedChecksum, actualChecksum)
	}

	return nil
}

// FetchChecksum downloads the sha256sum.txt at checksumsURL and extracts
// the hex-encoded SHA-256 for filename. The response body is capped to
// maxChecksumFileSize to prevent memory exhaustion.
func FetchChecksum(ctx context.Context, checksumsURL, filename string) (string, error) {
	client := httputil.New(httputil.TimeoutMedium)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumsURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("failed to build checksum request for %s: %w", checksumsURL, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch checksums from %s: %w", checksumsURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch checksums: HTTP %d", resp.StatusCode)
	}

	limitedReader := io.LimitReader(resp.Body, maxChecksumFileSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read checksums response: %w", err)
	}

	for line := range strings.Lines(string(body)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			checksumValue := parts[0]
			checksumFilename := parts[len(parts)-1]
			checksumFilename = strings.TrimPrefix(checksumFilename, "*")
			checksumFilename = strings.TrimPrefix(checksumFilename, "./")

			if checksumFilename == filename || strings.HasSuffix(checksumFilename, "/"+filename) {
				if len(checksumValue) != sha256.Size*2 {
					return "", fmt.Errorf("malformed checksum for %s: expected %d hex chars, got %d",
						filename, sha256.Size*2, len(checksumValue))
				}
				if _, err := hex.DecodeString(checksumValue); err != nil {
					return "", fmt.Errorf("malformed checksum for %s: %w", filename, err)
				}
				return checksumValue, nil
			}
		}
	}

	return "", fmt.Errorf("checksum not found for file: %s", filename)
}

func verifyDownloadedFile(path, expectedChecksum string, logger *slog.Logger) error {
	if expectedChecksum == "" {
		return nil
	}

	filename := filepath.Base(path)

	logger.Info("download: verifying checksum", "file", filename)

	actualChecksum, err := CalculateChecksum(path)
	if err != nil {
		_ = os.Remove(path)
		return err
	}

	if actualChecksum != expectedChecksum {
		_ = os.Remove(path)
		return fmt.Errorf("checksum verification failed for %s:\n  expected: %s\n  actual:   %s",
			filename, expectedChecksum, actualChecksum)
	}

	logger.Info("download: checksum verified", "file", filename)

	return nil
}

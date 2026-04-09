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

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/httputil"
)

const maxChecksumFileSize = 1024 * 1024

func CalculateChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("failed to read file for checksum: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

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

func FetchChecksum(ctx context.Context, checksumsURL, filename string) (string, error) {
	client := httputil.NewClient(httputil.WithTimeout(httputil.TimeoutMedium))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumsURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch checksums: HTTP %d", resp.StatusCode)
	}

	// Limit response size to avoid memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxChecksumFileSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read checksums response: %w", err)
	}

	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			checksumValue := parts[0]
			checksumFilename := parts[len(parts)-1]

			// Remove leading * or ./ from filename
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

	logger.Info(fmt.Sprintf("download: verifying checksum for %s", filename))

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

	logger.Info(fmt.Sprintf("download: checksum verified for %s", filename))

	return nil
}

package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// maxChecksumFileSize is the maximum size for checksum files (1MB limit).
const maxChecksumFileSize = 1024 * 1024

// CalculateChecksum calculates the SHA256 checksum of a file.
func CalculateChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", utils.WrapError("failed to read file for checksum", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// ValidateChecksum verifies a file's checksum against an expected value.
func ValidateChecksum(path, expectedChecksum string) error {
	if expectedChecksum == "" {
		return nil // No checksum to validate
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

// FetchChecksum downloads a checksums file and extracts the checksum for a specific file.
func FetchChecksum(ctx context.Context, checksumsURL, filename string) (string, error) {
	client := system.NewClient(system.WithTimeout(system.TimeoutMedium))

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
		return "", utils.WrapError("failed to read checksums response", err)
	}

	// Parse checksums file (format: checksum  filename)
	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on whitespace (handle both single and double space)
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			checksumValue := parts[0]
			checksumFilename := parts[len(parts)-1]

			// Remove leading * or ./ from filename
			checksumFilename = strings.TrimPrefix(checksumFilename, "*")
			checksumFilename = strings.TrimPrefix(checksumFilename, "./")

			if checksumFilename == filename || strings.HasSuffix(checksumFilename, "/"+filename) {
				return checksumValue, nil
			}
		}
	}

	return "", fmt.Errorf("checksum not found for file: %s", filename)
}

// verifyDownloadedFile validates checksum after download.
// If verification fails, the file is removed and an error is returned.
func verifyDownloadedFile(path, expectedChecksum string) error {
	if expectedChecksum == "" {
		return nil
	}

	filename := filepath.Base(path)

	utils.GetLogger().Info(fmt.Sprintf("download: verifying checksum for %s", filename))

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

	utils.GetLogger().Info(fmt.Sprintf("download: checksum verified for %s", filename))

	return nil
}

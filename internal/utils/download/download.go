// Package download provides utilities for downloading files with
// checksum verification and archive extraction.
package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// Options configures a download operation.
type Options struct {
	// URL is the URL to download from.
	URL string

	// OutputPath is where to save the downloaded file.
	OutputPath string

	// ExpectedChecksum is the expected SHA256 checksum (optional).
	// If provided, the download will be verified against this checksum.
	ExpectedChecksum string

	// Description is a human-readable description for logging.
	Description string

	// Timeout is the maximum time for the download.
	// Default: 5 minutes.
	Timeout time.Duration

	// Overwrite controls whether to overwrite existing files.
	// If false and file exists with correct checksum, download is skipped.
	Overwrite bool
}

// DefaultTimeout is the default download timeout.
const DefaultTimeout = 5 * time.Minute

// canSkipDownload checks if file exists with valid checksum.
// Returns true if the file exists and either has valid checksum or no checksum is required.
func canSkipDownload(opts Options) (bool, error) {
	info, err := os.Stat(opts.OutputPath)
	if err != nil || info.Size() == 0 {
		return false, nil
	}

	filename := filepath.Base(opts.OutputPath)

	// No checksum to validate, use existing file
	if opts.ExpectedChecksum == "" {
		utils.GetLogger().Info(fmt.Sprintf("download: using existing file %s (no checksum)", filename))
		return true, nil
	}

	// Validate checksum of existing file
	utils.GetLogger().Info(fmt.Sprintf("download: validating existing file %s", filename))

	actualChecksum, err := CalculateChecksum(opts.OutputPath)
	if err != nil {
		return false, nil // Can't read file, need to re-download
	}

	if actualChecksum == opts.ExpectedChecksum {
		utils.GetLogger().Info(fmt.Sprintf("download: checksum verified for %s", filename))
		return true, nil
	}

	// Checksum mismatch, need to re-download
	utils.GetLogger().Warn(fmt.Sprintf("download: checksum mismatch, re-downloading %s", filename))
	if err := os.Remove(opts.OutputPath); err != nil && !os.IsNotExist(err) {
		utils.GetLogger().Warn(fmt.Sprintf("download: failed to remove mismatched file %s: %v", filename, err))
	}
	return false, nil
}

// Download downloads a file from a URL with optional checksum verification.
// If the file already exists and has the correct checksum, the download is skipped.
func Download(ctx context.Context, opts Options) error {
	if opts.Timeout == 0 {
		opts.Timeout = DefaultTimeout
	}

	if opts.Description == "" {
		opts.Description = filepath.Base(opts.OutputPath)
	}

	if !opts.Overwrite {
		skip, err := canSkipDownload(opts)
		if err != nil {
			return err
		}
		if skip {
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0755); err != nil {
		return utils.WrapError("failed to create output directory", err)
	}

	filename := filepath.Base(opts.OutputPath)
	utils.GetLogger().Info(fmt.Sprintf("download: %s", filename))

	client := system.NewClient(system.WithTimeout(opts.Timeout))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return utils.WrapErrorf(err, "download failed for %s", opts.Description)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed for %s: HTTP %d", opts.Description, resp.StatusCode)
	}

	outFile, err := os.Create(opts.OutputPath)
	if err != nil {
		return utils.WrapError("failed to create output file", err)
	}
	defer func() { _ = outFile.Close() }()

	pw := &progressWriter{
		writer: outFile,
		total:  resp.ContentLength,
	}

	_, err = io.Copy(pw, resp.Body)
	if err != nil {
		// Stop progress output before returning error (prevents overwriting terminal)
		pw.stop()
		fmt.Print("\n") // Ensure we're on a new line after partial progress
		_ = os.Remove(opts.OutputPath)
		return utils.WrapError("failed to write file", err)
	}
	pw.finish()

	// Close file before checksum verification
	_ = outFile.Close()

	return verifyDownloadedFile(opts.OutputPath, opts.ExpectedChecksum)
}

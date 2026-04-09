// Package download fetches remote artifacts (ISOs, release binaries,
// checksums) with progress reporting and handles archive extraction for
// OKD installer tooling.
package download

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/httputil"
)

type Options struct {
	URL              string
	OutputPath       string
	ExpectedChecksum string // SHA256 checksum; if set, download is verified against it
	Description      string
	Timeout          time.Duration // Default: 5 minutes
	Overwrite        bool          // If false and file exists with correct checksum, download is skipped
	Logger           *slog.Logger
}

func (o Options) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return slog.New(slog.DiscardHandler)
}

const DefaultTimeout = 5 * time.Minute

func canSkipDownload(opts Options) (bool, error) {
	info, err := os.Stat(opts.OutputPath)
	if err != nil || info.Size() == 0 {
		return false, nil
	}

	filename := filepath.Base(opts.OutputPath)

	if opts.ExpectedChecksum == "" {
		opts.logger().Info(fmt.Sprintf("download: using existing file %s (no checksum)", filename))
		return true, nil
	}

	opts.logger().Info(fmt.Sprintf("download: validating existing file %s", filename))

	actualChecksum, err := CalculateChecksum(opts.OutputPath)
	if err != nil {
		return false, nil // Can't read file, need to re-download
	}

	if actualChecksum == opts.ExpectedChecksum {
		opts.logger().Info(fmt.Sprintf("download: checksum verified for %s", filename))
		return true, nil
	}

	opts.logger().Warn(fmt.Sprintf("download: checksum mismatch, re-downloading %s", filename))
	if err := os.Remove(opts.OutputPath); err != nil && !os.IsNotExist(err) {
		opts.logger().Warn(fmt.Sprintf("download: failed to remove mismatched file %s: %v", filename, err))
	}
	return false, nil
}

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
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	filename := filepath.Base(opts.OutputPath)
	opts.logger().Info(fmt.Sprintf("download: %s", filename))

	client := httputil.NewClient(httputil.WithTimeout(opts.Timeout))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download failed for %s: %w", opts.Description, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed for %s: HTTP %d", opts.Description, resp.StatusCode)
	}

	outFile, err := os.Create(opts.OutputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	pw := &progressWriter{
		writer: outFile,
		total:  resp.ContentLength,
		isTTY:  stdoutIsTTY,
	}

	_, err = io.Copy(pw, resp.Body)
	if err != nil {
		pw.stop()
		if stdoutIsTTY {
			fmt.Print("\n")
		}
		_ = os.Remove(opts.OutputPath)
		return fmt.Errorf("failed to write file: %w", err)
	}
	pw.finish()

	if err := verifyDownloadedFile(opts.OutputPath, opts.ExpectedChecksum, opts.logger()); err != nil {
		_ = os.Remove(opts.OutputPath)
		return err
	}
	return nil
}

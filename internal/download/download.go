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

	"github.com/schollz/progressbar/v3"
	"golang.org/x/term"

	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// stderrIsTTY gates progress-bar rendering. When the process is piped or
// run in CI, we skip the bar so logs don't fill with carriage-return spam.
var stderrIsTTY = term.IsTerminal(int(os.Stderr.Fd()))

type Options struct {
	URL              string
	OutputPath       string
	ExpectedChecksum string // SHA256 checksum; if set, download is verified against it
	Description      string
	Timeout          time.Duration // Default: 5 minutes
	Overwrite        bool          // If false and file exists with correct checksum, download is skipped
	Logger           *slog.Logger
}

func (o *Options) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return logutil.NopLogger
}

const DefaultTimeout = 5 * time.Minute

func canSkipDownload(opts *Options) bool {
	info, err := os.Stat(opts.OutputPath)
	if err != nil || info.Size() == 0 {
		return false
	}

	filename := filepath.Base(opts.OutputPath)

	if opts.ExpectedChecksum == "" {
		opts.logger().Info(fmt.Sprintf("download: using existing file %s (no checksum)", filename))
		return true
	}

	opts.logger().Info(fmt.Sprintf("download: validating existing file %s", filename))

	actualChecksum, err := CalculateChecksum(opts.OutputPath)
	if err != nil {
		return false // can't read file; re-download instead of failing
	}

	if actualChecksum == opts.ExpectedChecksum {
		opts.logger().Info(fmt.Sprintf("download: checksum verified for %s", filename))
		return true
	}

	opts.logger().Warn(fmt.Sprintf("download: checksum mismatch, re-downloading %s", filename))
	if err := os.Remove(opts.OutputPath); err != nil && !os.IsNotExist(err) {
		opts.logger().Warn(fmt.Sprintf("download: failed to remove mismatched file %s: %v", filename, err))
	}
	return false
}

func Download(ctx context.Context, opts *Options) error {
	if opts.Timeout == 0 {
		opts.Timeout = DefaultTimeout
	}

	if opts.Description == "" {
		opts.Description = filepath.Base(opts.OutputPath)
	}

	if !opts.Overwrite && canSkipDownload(opts) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	filename := filepath.Base(opts.OutputPath)
	opts.logger().Info(fmt.Sprintf("download: %s", filename))

	client := httputil.New(opts.Timeout)

	attempts, err := retryDownload(ctx, func() error {
		return fetchToFile(ctx, client, opts, filename)
	})
	if err != nil {
		opts.logger().Error(fmt.Sprintf("download: giving up on %s after %d attempt(s): %v", opts.Description, attempts, err))
		return fmt.Errorf("download failed for %s: %w", opts.Description, err)
	}

	if err := verifyDownloadedFile(opts.OutputPath, opts.ExpectedChecksum, opts.logger()); err != nil {
		_ = os.Remove(opts.OutputPath)
		return err
	}
	return nil
}

// fetchToFile runs one download attempt. On any mid-attempt failure the
// partial file is removed so the next retry starts from a clean slate.
func fetchToFile(ctx context.Context, client *http.Client, opts *Options, filename string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, http.NoBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return &httpStatusError{Status: resp.StatusCode, URL: opts.URL}
	}

	outFile, err := os.OpenFile(opts.OutputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}

	dst := io.Writer(outFile)
	var bar *progressbar.ProgressBar
	if stderrIsTTY {
		bar = progressbar.DefaultBytes(resp.ContentLength, filename)
		dst = io.MultiWriter(outFile, bar)
	}
	if _, err := io.Copy(dst, resp.Body); err != nil {
		if bar != nil {
			_ = bar.Exit()
		}
		_ = outFile.Close()
		_ = os.Remove(opts.OutputPath)
		return fmt.Errorf("write file: %w", err)
	}
	if bar != nil {
		_ = bar.Finish()
	}

	if err := outFile.Sync(); err != nil {
		_ = outFile.Close()
		_ = os.Remove(opts.OutputPath)
		return fmt.Errorf("sync output file: %w", err)
	}
	if err := outFile.Close(); err != nil {
		_ = os.Remove(opts.OutputPath)
		return fmt.Errorf("close output file: %w", err)
	}
	return nil
}

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

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// Options configures a Download call. An empty ExpectedChecksum disables
// checksum verification; an Overwrite=false call skips the fetch when a
// matching file already exists.
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

// DefaultTimeout bounds a single Download call when Options.Timeout is zero.
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
		opts.logger().Warn("download: failed to remove mismatched file", "file", filename, "err", err)
	}
	return false
}

// Download fetches opts.URL to opts.OutputPath with bounded retries and
// optional SHA-256 verification. A partially-written file is removed on any
// mid-attempt failure so retries start clean.
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
		opts.logger().Error("download: giving up after retries", "desc", opts.Description, "attempts", attempts, "err", err)
		return &errtypes.NetworkError{Msg: fmt.Sprintf("download failed for %s", opts.Description), Err: err}
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
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return &HTTPStatusError{
			Status: resp.StatusCode,
			Method: http.MethodGet,
			URL:    opts.URL,
			Body:   bodySnippet(raw, len(raw) == 256),
		}
	}

	// 0o600 — downloaded artifacts may include release tarballs that
	// contain the okdctl binary itself before signature verification.
	// install.sh + setup.installBinaryToPath copy the binary to its
	// final mode, so a tighter download mode is harmless to consumers.
	outFile, err := os.OpenFile(opts.OutputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}

	pw := newProgressWriter(outFile, resp.ContentLength, filename)
	if _, err := io.Copy(pw, resp.Body); err != nil {
		_ = pw.Close()
		_ = outFile.Close()
		_ = os.Remove(opts.OutputPath)
		return fmt.Errorf("write file: %w", err)
	}
	_ = pw.Close()

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

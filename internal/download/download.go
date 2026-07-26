// Package download fetches remote artifacts (ISOs, release binaries,
// checksums) with progress reporting and handles archive extraction for
// OKD installer tooling.
package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// DefaultTimeout bounds a single Fetch call when no WithTimeout option is set.
// Aliases httputil.TimeoutDownload so the download tier has one owner.
const DefaultTimeout = httputil.TimeoutDownload

// dlConfig holds the resolved configuration for a Fetch call. logger is
// normalised via logutil.OrNop once at Fetch construction.
type dlConfig struct {
	url              string
	outputPath       string
	expectedChecksum string
	description      string
	timeout          time.Duration
	overwrite        bool
	progress         bool
	logger           *slog.Logger
}

// FetchOption configures a Fetch call.
type FetchOption func(*dlConfig)

// WithFetchChecksum sets the expected SHA-256 hex digest; empty disables verification.
func WithFetchChecksum(sum string) FetchOption {
	return func(c *dlConfig) { c.expectedChecksum = sum }
}

// WithDescription sets the human-readable name used in log and error messages.
func WithDescription(d string) FetchOption { return func(c *dlConfig) { c.description = d } }

// WithTimeout overrides the per-fetch HTTP timeout (default: DefaultTimeout).
func WithTimeout(d time.Duration) FetchOption { return func(c *dlConfig) { c.timeout = d } }

// WithOverwrite forces a re-download even when a file with a matching checksum exists.
func WithOverwrite(v bool) FetchOption { return func(c *dlConfig) { c.overwrite = v } }

// WithProgress enables the stderr progress bar. Default is off; TTY gating
// is the caller's job — pass logutil.ProgressBarsEnabled() (or equivalent) so
// this package stays free of presentation-layer imports.
func WithProgress(v bool) FetchOption { return func(c *dlConfig) { c.progress = v } }

// WithLogger injects a structured logger; nil falls back to logutil.NopLogger.
func WithLogger(l *slog.Logger) FetchOption {
	return func(c *dlConfig) { c.logger = logutil.OrNop(l) }
}

func canSkipDownload(ctx context.Context, cfg *dlConfig) bool {
	info, err := os.Stat(cfg.outputPath)
	if err != nil || info.Size() == 0 {
		return false
	}

	filename := filepath.Base(cfg.outputPath)

	if cfg.expectedChecksum == "" {
		cfg.logger.Info("download: using existing file (no checksum)", "file", filename)
		return true
	}

	cfg.logger.Info("download: validating existing file", "file", filename)

	actualChecksum, err := CalculateChecksum(ctx, cfg.outputPath)
	if err != nil {
		return false // can't read file; re-download instead of failing
	}

	if actualChecksum == cfg.expectedChecksum {
		cfg.logger.Info("download: checksum verified", "file", filename)
		return true
	}

	cfg.logger.Warn("download: checksum mismatch, re-downloading", "file", filename)
	if err := os.Remove(cfg.outputPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		cfg.logger.Warn("download: failed to remove mismatched file", "file", filename, "err", err)
	}
	return false
}

// Fetch downloads the artifact at url to dst with bounded retries and optional
// SHA-256 verification. A partially-written file is removed on any mid-attempt
// failure so retries start clean.
func Fetch(ctx context.Context, url, dst string, opts ...FetchOption) error {
	cfg := &dlConfig{
		url:        url,
		outputPath: dst,
		logger:     logutil.NopLogger,
	}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.timeout == 0 {
		cfg.timeout = DefaultTimeout
	}
	if cfg.description == "" {
		cfg.description = filepath.Base(dst)
	}

	if !cfg.overwrite && canSkipDownload(ctx, cfg) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	filename := filepath.Base(dst)
	cfg.logger.Info("download: fetching", "file", filename)

	client := httputil.New(cfg.timeout)

	attempts, err := retryDownload(ctx, func() error {
		return fetchToFile(ctx, client, cfg, filename)
	})
	if err != nil {
		cfg.logger.Warn("download: giving up after retries", "file", filename, "attempts", attempts, "err", err)
		return &errtypes.NetworkError{Msg: fmt.Sprintf("download failed for %s", cfg.description), Err: err}
	}

	if err := verifyDownloadedFile(ctx, dst, cfg.expectedChecksum, cfg.logger); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}

// fetchToFile runs one download attempt. On any mid-attempt failure the
// partial file is removed so the next retry starts from a clean slate.
func fetchToFile(ctx context.Context, client *http.Client, cfg *dlConfig, filename string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.url, http.NoBody)
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
			URL:    cfg.url,
			Body:   bodySnippet(raw, len(raw) == 256),
		}
	}

	// 0o600 — downloaded artifacts may include release tarballs that
	// contain the okdctl binary itself before signature verification.
	// install.sh + setup.installBinaryToPath copy the binary to its
	// final mode, so a tighter download mode is harmless to consumers.
	// O_NOFOLLOW rejects a symlink at OutputPath; under the sudo re-exec
	// model the open runs as root, so following a symlink would write
	// binary content to an attacker-chosen path.
	outFile, err := os.OpenFile(cfg.outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}

	pw := newProgressWriter(outFile, resp.ContentLength, filename, cfg.progress)
	if _, err := io.Copy(pw, resp.Body); err != nil {
		_ = pw.Close()
		_ = outFile.Close()
		_ = os.Remove(cfg.outputPath)
		return fmt.Errorf("write file: %w", err)
	}
	_ = pw.Close()

	if err := outFile.Sync(); err != nil {
		_ = outFile.Close()
		_ = os.Remove(cfg.outputPath)
		return fmt.Errorf("sync output file: %w", err)
	}
	if err := outFile.Close(); err != nil {
		_ = os.Remove(cfg.outputPath)
		return fmt.Errorf("close output file: %w", err)
	}
	return nil
}

// Package download fetches remote artifacts with retries and extracts
// archives, for OKD installer tooling.
package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// responseHeaderTimeout bounds dial, TLS handshake and the wait for headers
// per attempt; the body read is bounded by ctx alone so a slow mirror is
// never cut off mid-stream and retried from zero.
var responseHeaderTimeout = 30 * time.Second

type dlConfig struct {
	url              string
	outputPath       string
	expectedChecksum string
	logger           *slog.Logger
}

// FetchOption configures a Fetch call.
type FetchOption func(*dlConfig)

// WithFetchChecksum sets the expected SHA-256 hex digest; empty disables verification.
func WithFetchChecksum(sum string) FetchOption {
	return func(c *dlConfig) { c.expectedChecksum = sum }
}

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
		cfg.logger.Warn("download: could not remove mismatched file", "file", filename, "err", err)
	}
	return false
}

// Fetch downloads the artifact at url to dst with bounded retries and optional
// SHA-256 verification. A partial file is removed on any failure so retries
// start clean.
func Fetch(ctx context.Context, url, dst string, opts ...FetchOption) error {
	cfg := &dlConfig{
		url:        url,
		outputPath: dst,
		logger:     logutil.NopLogger,
	}
	for _, o := range opts {
		o(cfg)
	}
	if canSkipDownload(ctx, cfg) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	filename := filepath.Base(dst)
	cfg.logger.Info("download: fetching", "file", filename)

	client := newClient()

	attempts, err := retryDownload(ctx, func() error {
		return fetchToFile(ctx, client, cfg)
	})
	if err != nil {
		cfg.logger.Warn("download: giving up after retries", "file", filename, "attempts", attempts, "err", err)
		return &errtypes.NetworkError{Msg: "download " + filename, Err: err}
	}

	if err := verifyDownloadedFile(ctx, dst, cfg.expectedChecksum, cfg.logger); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}

func newClient() *http.Client {
	client := httputil.New(0)
	client.Transport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: responseHeaderTimeout}).DialContext,
		TLSHandshakeTimeout:   responseHeaderTimeout,
		ResponseHeaderTimeout: responseHeaderTimeout,
	}
	return client
}

// fetchToFile runs one attempt; a mid-attempt failure removes the partial file
// so the next retry starts clean.
func fetchToFile(ctx context.Context, client *http.Client, cfg *dlConfig) error {
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

	// 0o600 is tightened only for transit; consumers set their own final mode.
	// O_NOFOLLOW rejects a symlink here — the open runs as root under sudo
	// re-exec, so following one would write attacker content elsewhere.
	outFile, err := os.OpenFile(cfg.outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		_ = outFile.Close()
		_ = os.Remove(cfg.outputPath)
		return fmt.Errorf("write file: %w", err)
	}

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

// Package system provides host OS operations used during OKD provisioning:
// filesystem helpers, privileged file writes, command execution with sudo,
// permission management, and systemd unit control.
package system

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// RunCaptured runs bin with args, capturing stderr into the returned error on
// non-zero exit. Context cancellation is respected; stdout is discarded
// (callers that need it should use internal/executor.Executor instead).
//
// env is filtered through executor.DefaultEnvAllowlist so unrelated shell
// tokens exported by the caller do not reach privileged child processes.
func RunCaptured(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return fmt.Errorf("%s: %w", bin, err)
		}
		return fmt.Errorf("%s: %w: %s", bin, err, msg)
	}
	return nil
}

// OutputCaptured runs bin with args and returns stdout. On non-zero exit,
// stderr is captured into the returned error so callers see ip/nmcli
// diagnostics rather than a bare exit-status.
//
// env is filtered through executor.DefaultEnvAllowlist.
func OutputCaptured(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return nil, fmt.Errorf("%s: %w", bin, err)
		}
		return nil, fmt.Errorf("%s: %w: %s", bin, err, msg)
	}
	return out, nil
}

// WaitForOptions configures the polling loop driven by WaitFor.
type WaitForOptions struct {
	Interval time.Duration // Default: 30 seconds
	Timeout  time.Duration // Default: no timeout (0)
	Logger   *slog.Logger
}

// DefaultWaitForOptions returns WaitForOptions with Interval=30s and no
// timeout, suitable as a starting point for callers that tweak one field.
func DefaultWaitForOptions() WaitForOptions {
	return WaitForOptions{
		Interval: 30 * time.Second,
		Timeout:  0,
	}
}

// WaitFor polls check at opts.Interval until it returns true, ctx is
// cancelled, or opts.Timeout elapses. A timeout that races with ctx
// cancellation reports ctx.Err as the primary cause.
func WaitFor(ctx context.Context, prefix, description string, check func() bool, opts WaitForOptions) error {
	if opts.Interval == 0 {
		opts.Interval = 30 * time.Second
	}

	logger := opts.Logger
	if logger == nil {
		logger = logutil.NopLogger
	}

	waitMsg := fmt.Sprintf("%s: waiting for %s...", prefix, description)
	readyMsg := fmt.Sprintf("%s: %s is ready", prefix, description)
	logger.Info(waitMsg)

	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	var timeoutCh <-chan time.Time
	if opts.Timeout > 0 {
		timer := time.NewTimer(opts.Timeout)
		defer timer.Stop()
		timeoutCh = timer.C
	}

	startTime := time.Now()
	polls := 0

	if check() {
		logger.Info(readyMsg, "polls", polls, "elapsed", time.Since(startTime).Round(time.Second))
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for %s %s: %w", prefix, description, ctx.Err())
		case <-timeoutCh:
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("waiting for %s %s: %w", prefix, description, err)
			}
			// Wrap context.DeadlineExceeded so callers can errors.Is the
			// timeout shape — the elapsed budget IS a deadline-exceeded.
			return fmt.Errorf("timeout waiting for %s %s after %v (%d polls): %w",
				prefix, description, opts.Timeout, polls, context.DeadlineExceeded)
		case <-ticker.C:
			polls++
			elapsed := time.Since(startTime)
			if check() {
				logger.Info(readyMsg, "polls", polls, "elapsed", elapsed.Round(time.Second))
				return nil
			}
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("waiting for %s %s: %w", prefix, description, err)
			}
			logger.Debug(prefix+": waiting", "for", description, "elapsed", elapsed.Round(time.Second))
		}
	}
}

// WaitForWithTimeout is a convenience wrapper around WaitFor that sets
// opts.Timeout and opts.Logger without the caller building WaitForOptions.
func WaitForWithTimeout(ctx context.Context, prefix, description string, check func() bool, timeout time.Duration, logger *slog.Logger) error {
	opts := DefaultWaitForOptions()
	opts.Timeout = timeout
	opts.Logger = logger
	return WaitFor(ctx, prefix, description, check, opts)
}

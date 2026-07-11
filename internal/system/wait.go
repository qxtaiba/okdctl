package system

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// WaitForOptions configures the polling loop driven by WaitFor. It is a
// per-call config struct rather than functional options: WaitFor is a
// one-shot operation, not a long-lived object built once via a constructor,
// so there is no natural attachment point for the WithX(...) pattern used
// by terraform.PlanOptions or proxmox.ProvisionOptions. WaitForWithTimeout
// wraps the common single-field case so most callers never build this
// struct by hand.
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
// cancellation reports ctx.Err as the primary cause. Lives here as a
// generic, dependency-light polling primitive shared by phase/kubectl.go
// and postinstall's ingress-termination wait — narrower than a
// subprocess-exec concern, so it stays out of internal/executor.
func WaitFor(ctx context.Context, prefix, description string, check func(context.Context) bool, opts WaitForOptions) error {
	if opts.Interval == 0 {
		opts.Interval = 30 * time.Second
	}

	logger := opts.Logger
	if logger == nil {
		logger = logutil.NopLogger
	}

	logger.Info(prefix+": waiting", "target", description)

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

	if check(ctx) {
		logger.Info(prefix+": ready", "target", description, "polls", polls, "elapsed", time.Since(startTime).Round(time.Second))
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
			// Return a ClusterError so exitCodeFor maps this to exit 4 rather
			// than 130. Unwrap chains to context.DeadlineExceeded so
			// errors.Is checks still work.
			return &errtypes.ClusterError{
				Msg: fmt.Sprintf("timeout waiting for %s %s after %v (%d polls)", prefix, description, opts.Timeout, polls),
				Err: context.DeadlineExceeded,
			}
		case <-ticker.C:
			polls++
			elapsed := time.Since(startTime)
			if check(ctx) {
				logger.Info(prefix+": ready", "target", description, "polls", polls, "elapsed", elapsed.Round(time.Second))
				return nil
			}
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("waiting for %s %s: %w", prefix, description, err)
			}
			logger.Debug(prefix+": waiting", "target", description, "elapsed", elapsed.Round(time.Second))
		}
	}
}

// WaitForWithTimeout is a convenience wrapper around WaitFor that sets
// opts.Timeout and opts.Logger without the caller building WaitForOptions.
func WaitForWithTimeout(ctx context.Context, prefix, description string, check func(context.Context) bool, timeout time.Duration, logger *slog.Logger) error {
	opts := DefaultWaitForOptions()
	opts.Timeout = timeout
	opts.Logger = logger
	return WaitFor(ctx, prefix, description, check, opts)
}

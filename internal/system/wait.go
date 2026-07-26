package system

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

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

// probeGrace bounds how far a final in-flight probe may outlive opts.Timeout.
// A probe launched just before the poll deadline is allowed to finish rather
// than being cancelled mid-call, but one that hangs (oc against a blackholed
// API) must not stall WaitFor forever past its configured timeout.
const probeGrace = 30 * time.Second

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

	startTime := time.Now()
	polls := 0
	first := true

	// The library passes its own deadline-bound context to the condition; we
	// ignore it so a final in-flight probe isn't cancelled mid-call the
	// instant the poll deadline lands. probeCtx keeps that intent bounded:
	// probes run against the caller's ctx extended only by probeGrace past
	// opts.Timeout, so a hung probe dies at deadline+grace instead of never.
	probeCtx := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		probeCtx, cancel = context.WithDeadline(ctx, startTime.Add(opts.Timeout+probeGrace))
		defer cancel()
	}

	// PollUntilContextTimeout/Cancel invoke this once immediately (the
	// "first" branch, mirroring the old pre-loop check) and then once per
	// tick thereafter. Only the tick branch counts toward polls, matching
	// the original ticker-driven accounting.
	condition := func(context.Context) (bool, error) {
		if first {
			first = false
			ready := check(probeCtx)
			if ready {
				logger.Info(prefix+": ready", "target", description, "polls", polls, "elapsed", time.Since(startTime).Round(time.Second))
			}
			return ready, nil
		}

		polls++
		elapsed := time.Since(startTime)
		if check(probeCtx) {
			logger.Info(prefix+": ready", "target", description, "polls", polls, "elapsed", elapsed.Round(time.Second))
			return true, nil
		}
		logger.Debug(prefix+": waiting", "target", description, "elapsed", elapsed.Round(time.Second))
		return false, nil
	}

	var pollErr error
	if opts.Timeout > 0 {
		pollErr = wait.PollUntilContextTimeout(ctx, opts.Interval, opts.Timeout, true, condition)
	} else {
		pollErr = wait.PollUntilContextCancel(ctx, opts.Interval, true, condition)
	}
	if pollErr == nil {
		return nil
	}

	// A race between ctx and our own timeout reports ctx.Err as the primary
	// cause; PollUntilContextTimeout derives its deadline from ctx, so ctx's
	// own error (if any) always takes precedence over the synthetic one.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("waiting for %s %s: %w", prefix, description, err)
	}

	// Return a ClusterError so exitCodeFor maps this to exit 4 rather than
	// 130. ErrWaitTimeout chains to context.DeadlineExceeded so existing
	// errors.Is checks still work, while letting callers tell a WaitFor
	// poll timeout from a genuine context deadline.
	return &errtypes.ClusterError{
		Msg: fmt.Sprintf("timeout waiting for %s %s after %v (%d polls)", prefix, description, opts.Timeout, polls),
		Err: errtypes.ErrWaitTimeout,
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

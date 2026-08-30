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

// WaitForOptions configures the polling loop driven by WaitFor.
// WaitForWithTimeout wraps the common single-field case so most callers
// never build this struct by hand.
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

// probeGrace bounds how far a final in-flight probe may outlive opts.Timeout,
// so a hung probe (e.g. oc against a blackholed API) can't stall WaitFor forever.
const probeGrace = 30 * time.Second

// WaitFor polls check at opts.Interval until it returns true, ctx is
// cancelled, or opts.Timeout elapses. A timeout that races with ctx
// cancellation reports ctx.Err as the primary cause.
func WaitFor(ctx context.Context, prefix, description string, check func(context.Context) bool, opts WaitForOptions) error {
	if opts.Interval == 0 {
		opts.Interval = 30 * time.Second
	}

	logger := opts.Logger
	if logger == nil {
		logger = logutil.NopLogger
	}

	logger.Info("waiting", "component", prefix, "target", description)

	startTime := time.Now()
	polls := 0
	first := true

	// Ignores the library's own deadline context so an in-flight probe isn't
	// cut off right at the poll deadline; probeCtx bounds it to ctx+probeGrace
	// instead, so a hung probe dies at deadline+grace, not never.
	probeCtx := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		probeCtx, cancel = context.WithDeadline(ctx, startTime.Add(opts.Timeout+probeGrace))
		defer cancel()
	}

	// PollUntilContextTimeout/Cancel call this once immediately (the first
	// branch) then once per tick; only tick calls count toward polls.
	condition := func(context.Context) (bool, error) {
		if first {
			first = false
			ready := check(probeCtx)
			if ready {
				logger.Info("ready", "component", prefix, "target", description, "polls", polls, "duration", time.Since(startTime).Round(time.Second))
			}
			return ready, nil
		}

		polls++
		elapsed := time.Since(startTime)
		if check(probeCtx) {
			logger.Info("ready", "component", prefix, "target", description, "polls", polls, "duration", elapsed.Round(time.Second))
			return true, nil
		}
		logger.Debug("waiting", "component", prefix, "target", description, "duration", elapsed.Round(time.Second))
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

	// PollUntilContextTimeout derives its own deadline from ctx, so ctx's own
	// error always takes precedence over the synthetic timeout.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("waiting for %s %s: %w", prefix, description, err)
	}

	// ClusterError maps to exit 4 (not 130); ErrWaitTimeout still chains to
	// context.DeadlineExceeded so existing errors.Is checks work, while
	// letting callers distinguish a poll timeout from a real ctx deadline.
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

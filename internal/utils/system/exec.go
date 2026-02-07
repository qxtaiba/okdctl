package system

import (
	"context"
	"fmt"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

// WaitForOptions configures the WaitFor function behavior.
type WaitForOptions struct {
	// Interval is the time between checks. Default: 30 seconds.
	Interval time.Duration
	// Timeout is the maximum time to wait. Default: no timeout (0).
	Timeout time.Duration
}

// DefaultWaitForOptions returns sensible defaults.
func DefaultWaitForOptions() WaitForOptions {
	return WaitForOptions{
		Interval: 30 * time.Second,
		Timeout:  0,
	}
}

// WaitFor polls a condition until it succeeds or the context is cancelled.
// Prefix identifies the component (e.g. "metallb"), description identifies what is being waited on (e.g. "namespace").
func WaitFor(ctx context.Context, prefix, description string, check func() bool, opts WaitForOptions) error {
	if opts.Interval == 0 {
		opts.Interval = 30 * time.Second
	}

	waitMsg := fmt.Sprintf("%s: waiting for %s...", prefix, description)
	readyMsg := fmt.Sprintf("%s: %s is ready", prefix, description)
	utils.GetLogger().Info(waitMsg)

	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	var timeoutCh <-chan time.Time
	if opts.Timeout > 0 {
		timer := time.NewTimer(opts.Timeout)
		defer timer.Stop()
		timeoutCh = timer.C
	}

	startTime := time.Now()

	// Check immediately first
	if check() {
		utils.GetLogger().Info(readyMsg)
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return utils.WrapErrorf(ctx.Err(), "waiting for %s %s", prefix, description)
		case <-timeoutCh:
			return fmt.Errorf("timeout waiting for %s %s after %v", prefix, description, opts.Timeout)
		case <-ticker.C:
			elapsed := time.Since(startTime)
			if check() {
				utils.GetLogger().Info(readyMsg)
				return nil
			}
			utils.GetLogger().Info(fmt.Sprintf("%s: waiting for %s... (%v elapsed)", prefix, description, elapsed.Round(time.Second)))
		}
	}
}

// WaitForWithTimeout is a convenience wrapper for WaitFor with a timeout.
func WaitForWithTimeout(ctx context.Context, prefix, description string, check func() bool, timeout time.Duration) error {
	opts := DefaultWaitForOptions()
	opts.Timeout = timeout
	return WaitFor(ctx, prefix, description, check, opts)
}

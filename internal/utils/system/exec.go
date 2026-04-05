// Package system provides host OS operations used during OKD provisioning:
// filesystem helpers, privileged file writes, command execution with sudo,
// permission management, and systemd unit control.
package system

import (
	"context"
	"fmt"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

type WaitForOptions struct {
	Interval time.Duration  // Default: 30 seconds
	Timeout  time.Duration  // Default: no timeout (0)
	Logger   utils.Logger
}

func DefaultWaitForOptions() WaitForOptions {
	return WaitForOptions{
		Interval: 30 * time.Second,
		Timeout:  0,
	}
}

func WaitFor(ctx context.Context, prefix, description string, check func() bool, opts WaitForOptions) error {
	if opts.Interval == 0 {
		opts.Interval = 30 * time.Second
	}

	logger := opts.Logger
	if logger == nil {
		logger = utils.NoopLogger()
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

	if check() {
		logger.Info(readyMsg)
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
				logger.Info(readyMsg)
				return nil
			}
			logger.Info(fmt.Sprintf("%s: waiting for %s... (%v elapsed)", prefix, description, elapsed.Round(time.Second)))
		}
	}
}

func WaitForWithTimeout(ctx context.Context, prefix, description string, check func() bool, timeout time.Duration, logger utils.Logger) error {
	opts := DefaultWaitForOptions()
	opts.Timeout = timeout
	opts.Logger = logger
	return WaitFor(ctx, prefix, description, check, opts)
}

// Package system provides host OS operations used during OKD provisioning:
// filesystem helpers, privileged file writes, command execution with sudo,
// permission management, and systemd unit control.
package system

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// outputCapturedMaxBytes caps stdout returned by OutputCaptured at 4 MiB to
// prevent a runaway subprocess from buffering unbounded data into the caller.
const outputCapturedMaxBytes = 4 * 1024 * 1024

// SubprocessError is returned by RunCaptured and OutputCaptured when a
// subprocess fails. StderrTail carries the raw subprocess stderr; callers
// that need it programmatically can errors.As to this type. The Redacted
// method omits StderrTail from structured log output, preventing subprocess
// stderr from leaking through slog attrs.
type SubprocessError struct {
	Bin        string
	Err        error
	StderrTail string
}

func (e *SubprocessError) Error() string {
	if e.StderrTail == "" {
		return e.Bin + ": " + e.Err.Error()
	}
	tail := fmt.Sprint(logutil.RedactableStderr(e.StderrTail).Redacted())
	return e.Bin + ": " + e.Err.Error() + ": " + tail
}

func (e *SubprocessError) Unwrap() error { return e.Err }

// Redacted omits StderrTail so subprocess stderr never reaches a
// structured log sink via slog attrs.
func (e *SubprocessError) Redacted() any {
	return e.Bin + ": " + e.Err.Error()
}

// RunCaptured runs bin with args, capturing stderr into the returned error on
// non-zero exit. Context cancellation is respected; stdout is discarded
// (callers that need it should use internal/executor.Executor instead).
//
// env is filtered through executor.DefaultEnvAllowlist so unrelated shell
// tokens exported by the caller do not reach privileged child processes.
func RunCaptured(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	// SIGTERM (not SIGINT) is the correct soft-cancel for non-terraform binaries;
	// SIGINT is reserved for terraform's documented state-lock release path.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 30 * time.Second
	cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return &SubprocessError{
			Bin:        bin,
			Err:        err,
			StderrTail: strings.TrimSpace(stderr.String()),
		}
	}
	return nil
}

// OutputCaptured runs bin with args and returns stdout. On non-zero exit,
// stderr is captured into the returned error so callers see ip/nmcli
// diagnostics rather than a bare exit-status. Stdout is capped at
// outputCapturedMaxBytes; output that exceeds the cap returns a SubprocessError.
//
// env is filtered through executor.DefaultEnvAllowlist.
func OutputCaptured(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	// SIGTERM (not SIGINT) is the correct soft-cancel for non-terraform binaries;
	// SIGINT is reserved for terraform's documented state-lock release path.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 30 * time.Second
	cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, &SubprocessError{Bin: bin, Err: err}
	}
	if err := cmd.Start(); err != nil {
		return nil, &SubprocessError{Bin: bin, Err: err}
	}

	out, readErr := io.ReadAll(io.LimitReader(stdoutPipe, outputCapturedMaxBytes+1))
	waitErr := cmd.Wait()

	if len(out) > outputCapturedMaxBytes {
		return nil, &SubprocessError{
			Bin: bin,
			Err: fmt.Errorf("output exceeded %d bytes", outputCapturedMaxBytes),
		}
	}
	if readErr != nil {
		return nil, &SubprocessError{Bin: bin, Err: readErr}
	}
	if waitErr != nil {
		return nil, &SubprocessError{
			Bin:        bin,
			Err:        waitErr,
			StderrTail: strings.TrimSpace(stderr.String()),
		}
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

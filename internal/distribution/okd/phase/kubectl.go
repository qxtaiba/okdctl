package phase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/system"
)

// OcResourceExists returns true if `oc get <args...>` produces non-empty
// output, wrapping transport errors with errPrefix. --no-headers and
// --ignore-not-found are appended automatically.
func (p *BasePhase) OcResourceExists(ctx context.Context, errPrefix string, args ...string) (bool, error) {
	full := append([]string{"get"}, args...)
	full = append(full, "--no-headers", "--ignore-not-found")
	result, err := p.Exec.Run(ctx, "oc", full...)
	if err != nil {
		return false, fmt.Errorf("%s: %w", errPrefix, err)
	}
	return result.ExitCode == 0 && strings.TrimSpace(result.Stdout) != "", nil
}

// OcOutput runs `oc <args...>` once and returns trimmed stdout. A non-zero
// exit code is returned as an *executor.ExitError (callers can errors.As to
// inspect ExitCode) unless ctx is cancelled, in which case the ctx error
// propagates so SIGINT maps to exit 130.
func (p *BasePhase) OcOutput(ctx context.Context, args ...string) (string, error) {
	result, err := p.Exec.Run(ctx, "oc", args...)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", executor.NewExitError(ctx, "oc "+args[0], result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return strings.TrimSpace(result.Stdout), nil
}

// OcPollOutput polls `oc <args...>` at the WaitFor default interval (30s)
// until predicate matches the trimmed stdout, and returns the first
// matching value. timeout bounds the wait.
func (p *BasePhase) OcPollOutput(ctx context.Context, prefix, desc string, timeout time.Duration, predicate func(stdout string) bool, args ...string) (string, error) {
	return p.OcPollOutputInterval(ctx, prefix, desc, timeout, 0, predicate, args...)
}

// OcPollOutputInterval is the test-injection seam used by phase/kubectl_test.go
// to override the default polling cadence. Production code MUST use OcPollOutput,
// which fixes interval=0 (immediate first probe). Renaming or deleting this
// method requires updating phase/kubectl_test.go. Retained as scaffolding
// (api:9ce5434c); do not delete without a replacement test-injection path.
func (p *BasePhase) OcPollOutputInterval(ctx context.Context, prefix, desc string, timeout, interval time.Duration, predicate func(stdout string) bool, args ...string) (string, error) {
	var captured string
	opts := system.DefaultWaitForOptions()
	opts.Timeout = timeout
	if interval > 0 {
		opts.Interval = interval
	}
	opts.Logger = p.Log
	err := system.WaitFor(ctx, prefix, desc, func(context.Context) bool {
		result, _ := p.Exec.Run(ctx, "oc", args...)
		if result.ExitCode != 0 {
			return false
		}
		value := strings.TrimSpace(result.Stdout)
		if !predicate(value) {
			return false
		}
		captured = value
		return true
	}, opts)
	return captured, err
}

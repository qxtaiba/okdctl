package phase

import (
	"context"
	"fmt"
	"strings"
	"time"

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

// OcOutput runs `oc <args...>` once and returns trimmed stdout.
// A non-zero exit code is returned as an error wrapping the stderr text.
func (p *BasePhase) OcOutput(ctx context.Context, args ...string) (string, error) {
	result, err := p.Exec.Run(ctx, "oc", args...)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("oc %s failed (exit %d): %s", args[0], result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return strings.TrimSpace(result.Stdout), nil
}

// OcPollOutput polls `oc <args...>` until predicate matches the trimmed
// stdout, and returns the first matching value. Polls on the default
// WaitFor interval bounded by timeout.
func (p *BasePhase) OcPollOutput(ctx context.Context, prefix, desc string, timeout time.Duration, predicate func(stdout string) bool, args ...string) (string, error) {
	var captured string
	err := system.WaitForWithTimeout(ctx, prefix, desc, func() bool {
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
	}, timeout, p.Log)
	return captured, err
}

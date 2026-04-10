package phase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// OcResourceExists runs `oc get <args...> --no-headers --ignore-not-found`
// and returns true if the command succeeds and produces non-empty stdout.
// errPrefix is prepended to any transport error so callers get a useful
// wrapped error instead of a bare exec failure.
//
// This is the canonical "does this k8s resource exist?" check for OKD phases
// that go through p.Exec rather than a typed client.
func (p *BasePhase) OcResourceExists(ctx context.Context, errPrefix string, args ...string) (bool, error) {
	full := append([]string{"get"}, args...)
	full = append(full, "--no-headers", "--ignore-not-found")
	result, err := p.Exec.Run(ctx, "oc", full...)
	if err != nil {
		return false, fmt.Errorf("%s: %w", errPrefix, err)
	}
	return result.ExitCode == 0 && strings.TrimSpace(result.Stdout) != "", nil
}

// OcPollOutput polls `oc <args...>` on the default WaitFor interval until
// predicate returns true for the trimmed stdout. The first matching value is
// returned along with nil error. Use this for "wait until resource has value
// X" patterns where the value should be captured and returned.
//
// prefix/desc are passed through to system.WaitForWithTimeout for logging.
func (p *BasePhase) OcPollOutput(ctx context.Context, prefix, desc string, timeout time.Duration, predicate func(stdout string) bool, args ...string) (string, error) {
	var captured string
	err := system.WaitForWithTimeout(ctx, prefix, desc, func() bool {
		result, _ := p.Exec.Run(ctx, "oc", args...)
		if result == nil || result.ExitCode != 0 {
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

package phase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/system"
)

// oc wraps the phase's executor in a cluster.Client so every Oc* method
// below shares one invocation/error-formatting path; see
// install.Phase.SetupKubeconfig for how KUBECONFIG reaches p.Exec.
func (p *BasePhase) oc() *cluster.Client {
	return cluster.New(cluster.WithExecutor(p.Exec), cluster.WithCLI("oc"), cluster.WithLogger(p.Log))
}

// OcResourceExists reports whether `oc get <args...>` (with --no-headers
// --ignore-not-found appended) exits 0 with non-empty stdout, wrapping
// transport errors with errPrefix.
func (p *BasePhase) OcResourceExists(ctx context.Context, errPrefix string, args ...string) (bool, error) {
	full := append([]string{"get"}, args...)
	full = append(full, "--no-headers", "--ignore-not-found")
	result, err := p.oc().Run(ctx, full...)
	if err != nil {
		return false, fmt.Errorf("%s: %w", errPrefix, err)
	}
	return result.ExitCode == 0 && strings.TrimSpace(result.Stdout) != "", nil
}

// OcOutputFull runs `oc <args...>` once, buffering up to 4 MiB of stdout,
// and errors instead of silently returning a truncated payload.
func (p *BasePhase) OcOutputFull(ctx context.Context, args ...string) (string, error) {
	stdout, truncated, err := p.oc().GetJSON(ctx, args...)
	if err != nil {
		return "", err
	}
	if truncated {
		return "", fmt.Errorf("oc output truncated after %d bytes; payload too large for machine parsing", len(stdout))
	}
	return stdout, nil
}

// OcOutput runs `oc <args...>` once and returns trimmed stdout. A non-zero
// exit code returns an *executor.ExitError unless ctx is cancelled, in which
// case the ctx error propagates so SIGINT maps to exit 130.
func (p *BasePhase) OcOutput(ctx context.Context, args ...string) (string, error) {
	result, err := p.oc().Run(ctx, args...)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", executor.NewExitError(ctx, "oc "+args[0], result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return strings.TrimSpace(result.Stdout), nil
}

// OcPatch patches a cluster-scoped resource via `oc patch <resource> <name>
// --type=<patchType> -p <patch>`.
func (p *BasePhase) OcPatch(ctx context.Context, resource, name, patchType, patch string) error {
	return p.oc().Patch(ctx, resource, name, patchType, patch)
}

// OcApply applies a manifest via `oc apply -f -`, feeding it on stdin.
func (p *BasePhase) OcApply(ctx context.Context, manifest []byte) error {
	return p.oc().Apply(ctx, manifest)
}

// OcPollOutput polls `oc <args...>` every 30s until predicate matches
// trimmed stdout, returning the first match within timeout.
func (p *BasePhase) OcPollOutput(ctx context.Context, prefix, desc string, timeout time.Duration, predicate func(stdout string) bool, args ...string) (string, error) {
	return p.OcPollOutputInterval(ctx, prefix, desc, timeout, 0, predicate, args...)
}

// OcPollOutputInterval is the test seam for polling cadence; production
// code MUST use OcPollOutput, which fixes interval=0 (immediate first probe).
func (p *BasePhase) OcPollOutputInterval(ctx context.Context, prefix, desc string, timeout, interval time.Duration, predicate func(stdout string) bool, args ...string) (string, error) {
	var captured string
	opts := system.DefaultWaitForOptions()
	opts.Timeout = timeout
	if interval > 0 {
		opts.Interval = interval
	}
	opts.Logger = p.Log
	err := system.WaitFor(ctx, prefix, desc, func(ctx context.Context) bool {
		// Result is nil on transport failure here (unlike executor.Run); ctx
		// is WaitFor's probe deadline so a hung oc can't outlive it.
		result, runErr := p.oc().Run(ctx, args...)
		if runErr != nil || result.ExitCode != 0 {
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

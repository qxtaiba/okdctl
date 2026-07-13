package install

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// defaultStartMonitorCmd starts "openshift-install wait-for install-complete"
// via the canonical Executor.StartStreamed, sharing buildEnv/cancelSignal/
// WaitDelay with every other subprocess this phase runs. The executor's
// stdout/stderr are wired by deploy to the persistent log file (the TTY shows
// the curated status line instead); --verbose tees them back to the terminal.
func (p *Phase) defaultStartMonitorCmd(ctx context.Context, clusterDir string) (done <-chan error, kill func(), err error) {
	return p.Exec.StartStreamed(ctx, "openshift-install", "wait-for", "install-complete", "--dir", clusterDir, "--log-level=debug")
}

// timeoutNextSteps names the diagnosis surfaces for a wait timeout: the
// openshift-install debug log, one representative oc probe, and the okdctl
// bundle collector. Embedded in timeout error messages so the operator sees
// them wherever the error surfaces — the message is the contract.
func timeoutNextSteps(clusterDir string) string {
	return fmt.Sprintf("check %s, inspect the cluster with 'oc --kubeconfig %s get clusteroperators', or collect diagnostics with 'okdctl debug-bundle'",
		filepath.Join(clusterDir, ".openshift_install.log"), filepath.Join(clusterDir, "auth", "kubeconfig"))
}

// WaitForBootstrap runs "openshift-install wait-for bootstrap-complete",
// bounded by opts.BootstrapTimeout, streaming output to the current TTY.
func (p *Phase) WaitForBootstrap(ctx context.Context, clusterDir string, opts *Options) error {
	ctx, cancel := context.WithTimeout(ctx, opts.BootstrapTimeout)
	defer cancel()

	stopSpinner := p.Reporter("waiting for bootstrap complete")
	_, err := p.Exec.RunStreamedChecked(ctx, "openshift-install", "wait-for", "bootstrap-complete", "--dir", clusterDir, "--log-level=debug")
	stopSpinner()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			// Preserve cancellation identity through the typed error so
			// callers can errors.Is(err, context.DeadlineExceeded) to
			// distinguish "we ran out of budget" from "command failed".
			return &errtypes.ClusterError{
				Msg: fmt.Sprintf("bootstrap timed out after %v — %s", opts.BootstrapTimeout, timeoutNextSteps(clusterDir)),
				Err: ctx.Err(),
			}
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			// Bare wrap intentional: cli/root.go::signalExitCode walks the chain
			// via errors.Is(err, context.Canceled) before exitCodeFor runs,
			// mapping SIGINT→130 / SIGTERM→143 without a typed error.
			return fmt.Errorf("bootstrap cancelled: %w", ctx.Err())
		}
		return &errtypes.ClusterError{Msg: "bootstrap failed", Err: err}
	}

	p.Log.Info("bootstrap: completed - control plane is ready")
	return nil
}

// csrApprover is the subset of cluster.Client MonitorInstallation uses.
// Accepting the interface instead of the concrete type lets tests inject a
// stub without a real kubeconfig.
type csrApprover interface {
	ApprovePendingCSRs(ctx context.Context) (int, error)
}

// operatorCounter is the optional cluster-operator-health surface a real
// cluster.Client satisfies. MonitorInstallation type-asserts the approver for
// it; a stub approver that omits it simply drops the operator count from the
// status line (the CSR count still shows).
type operatorCounter interface {
	ClusterOperatorHealth(ctx context.Context) (cluster.OperatorHealth, error)
}

// operatorStatusDetail builds the status-line detail from the live
// cluster-operator count and the running CSR-approval total. A missing counter
// or a transient count failure (API not yet reachable) degrades to the CSR
// count alone rather than blanking the line.
func operatorStatusDetail(ctx context.Context, counter operatorCounter, csrs int) string {
	if counter != nil {
		if h, err := counter.ClusterOperatorHealth(ctx); err == nil && h.Total > 0 {
			return fmt.Sprintf("cluster operators %d/%d available · %d CSRs approved", h.Available, h.Total, csrs)
		}
	}
	return fmt.Sprintf("%d CSRs approved", csrs)
}

// MonitorInstallation watches the post-bootstrap install until all cluster
// operators are Available, bounded by opts.InstallTimeout. If approver is
// nil a real cluster.Client is constructed from clusterDir/auth/kubeconfig.
func (p *Phase) MonitorInstallation(ctx context.Context, clusterDir string, opts *Options, approver csrApprover) error {
	ctx, cancel := context.WithTimeout(ctx, opts.InstallTimeout)
	defer cancel()

	if approver == nil {
		kubeconfigPath := filepath.Join(clusterDir, "auth", "kubeconfig")
		approver = cluster.New(
			cluster.WithCLI("oc"),
			cluster.WithKubeconfig(kubeconfigPath),
			cluster.WithLogger(p.Log),
		)
	}

	startCmd := p.startMonitorCmd
	if startCmd == nil {
		startCmd = p.defaultStartMonitorCmd
	}

	counter, _ := approver.(operatorCounter)
	setStatus, stopStatus := p.StatusLine("waiting for cluster operators")
	defer stopStatus()

	installDone, _, err := startCmd(ctx, clusterDir)
	if err != nil {
		return &errtypes.ClusterError{Msg: "failed to start installation monitor", Err: err}
	}

	// time.NewTicker panics on a non-positive duration; fall back to the default.
	interval := opts.CSRApprovalInterval
	if interval <= 0 {
		interval = DefaultCSRApprovalInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	totalApproved := 0
	setStatus(operatorStatusDetail(ctx, counter, totalApproved))
	// Dedup identical consecutive tick errors: Warn once, then Debug the
	// repeats so a 60-minute install doesn't spam the log with the same
	// transient approve-check failure.
	var lastCSRWarnMsg string

	for {
		select {
		case err := <-installDone:
			if err != nil {
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return &errtypes.ClusterError{
						Msg: fmt.Sprintf("installation timed out after %v — %s", opts.InstallTimeout, timeoutNextSteps(clusterDir)),
						Err: ctx.Err(),
					}
				}
				if errors.Is(ctx.Err(), context.Canceled) {
					// Bare wrap intentional: cli/root.go::signalExitCode walks the
					// chain via errors.Is(err, context.Canceled) before exitCodeFor
					// runs, mapping SIGINT→130 / SIGTERM→143 without a typed error.
					return fmt.Errorf("installation cancelled: %w", ctx.Err())
				}
				return &errtypes.ClusterError{Msg: "installation failed", Err: err}
			}

			approved, csrErr := approver.ApprovePendingCSRs(ctx)
			if csrErr != nil {
				p.Log.Warn("csr: final approval had issues", "err", csrErr)
			}
			totalApproved += approved

			p.Log.Info("install: completed", "csrs_approved", totalApproved)
			return nil

		case <-ticker.C:
			approved, err := approver.ApprovePendingCSRs(ctx)
			if err != nil {
				msg := err.Error()
				if msg != lastCSRWarnMsg {
					p.Log.Warn("csr: approval check failed", "err", err)
					lastCSRWarnMsg = msg
				} else {
					p.Log.Debug("csr: approval check failed (repeated)", "err", err)
				}
			} else {
				lastCSRWarnMsg = ""
			}
			if approved > 0 {
				totalApproved += approved
				p.Log.Info("csr: approved pending requests", "approved", approved, "total", totalApproved)
			}
			setStatus(operatorStatusDetail(ctx, counter, totalApproved))

		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				// Bare wrap intentional: cli/root.go::signalExitCode walks the chain
				// via errors.Is(err, context.Canceled) before exitCodeFor runs,
				// mapping SIGINT→130 / SIGTERM→143 without a typed error.
				return fmt.Errorf("installation cancelled: %w", ctx.Err())
			}
			return &errtypes.ClusterError{
				Msg: fmt.Sprintf("installation timed out after %v — %s", opts.InstallTimeout, timeoutNextSteps(clusterDir)),
				Err: ctx.Err(),
			}
		}
	}
}

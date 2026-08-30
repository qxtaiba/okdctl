package install

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// defaultStartMonitorCmd starts wait-for install-complete; --verbose tees output to the terminal.
func (p *Phase) defaultStartMonitorCmd(ctx context.Context, clusterDir string) (done <-chan error, kill func(), err error) {
	return p.Exec.StartStreamed(ctx, "openshift-install", "wait-for", "install-complete", "--dir", clusterDir, "--log-level=debug")
}

// timeoutNextSteps names diagnosis surfaces embedded in timeout error messages
// — the message text is the contract.
func timeoutNextSteps(clusterDir string) string {
	return fmt.Sprintf("check %s, inspect the cluster with 'oc --kubeconfig %s get clusteroperators', or collect diagnostics with 'okdctl debug-bundle'",
		filepath.Join(clusterDir, ".openshift_install.log"), workspace.KubeconfigPath(clusterDir))
}

// WaitForBootstrap runs wait-for bootstrap-complete, bounded by
// opts.BootstrapTimeout, streaming to the TTY.
func (p *Phase) WaitForBootstrap(ctx context.Context, clusterDir string, opts *Options) error {
	ctx, cancel := context.WithTimeout(ctx, opts.BootstrapTimeout)
	defer cancel()

	stopSpinner := p.Reporter("waiting for bootstrap complete")
	_, err := p.Exec.RunStreamedChecked(ctx, "openshift-install", "wait-for", "bootstrap-complete", "--dir", clusterDir, "--log-level=debug")
	stopSpinner()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			// Preserve cancellation identity so callers can errors.Is(err, context.DeadlineExceeded).
			return &errtypes.ClusterError{
				Msg: fmt.Sprintf("bootstrap timed out after %v — %s", opts.BootstrapTimeout, timeoutNextSteps(clusterDir)),
				Err: ctx.Err(),
			}
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			// Bare wrap intentional: preserves context.Canceled for
			// cli/root.go::signalExitCode (SIGINT→130/SIGTERM→143).
			return fmt.Errorf("bootstrap cancelled: %w", ctx.Err())
		}
		return &errtypes.ClusterError{Msg: "bootstrap failed", Err: err}
	}

	p.Log.Info("bootstrap: completed - control plane is ready")
	return nil
}

// csrApprover is the subset of cluster.Client MonitorInstallation uses, letting
// tests inject a stub.
type csrApprover interface {
	ApprovePendingCSRs(ctx context.Context) (int, error)
}

// operatorCounter is the optional cluster-operator-health surface; a stub
// approver that omits it just drops the count from the status line.
type operatorCounter interface {
	ClusterOperatorHealth(ctx context.Context) (cluster.OperatorHealth, error)
}

// operatorStatusDetail builds the status line; a missing or failing counter
// degrades to the CSR count alone rather than blanking the line.
func operatorStatusDetail(ctx context.Context, counter operatorCounter, csrs int) string {
	if counter != nil {
		if h, err := counter.ClusterOperatorHealth(ctx); err == nil && h.Total > 0 {
			return fmt.Sprintf("cluster operators %d/%d available · %d CSRs approved", h.Available, h.Total, csrs)
		}
	}
	return fmt.Sprintf("%d CSRs approved", csrs)
}

// MonitorInstallation watches the post-bootstrap install until all operators
// are Available, bounded by opts.InstallTimeout. A nil approver builds a
// real cluster.Client from clusterDir/auth/kubeconfig.
func (p *Phase) MonitorInstallation(ctx context.Context, clusterDir string, opts *Options, approver csrApprover) error {
	ctx, cancel := context.WithTimeout(ctx, opts.InstallTimeout)
	defer cancel()

	if approver == nil {
		kubeconfigPath := workspace.KubeconfigPath(clusterDir)
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
		return &errtypes.ClusterError{Msg: "start installation monitor", Err: err}
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
	csrWarn := logutil.NewDedupWarner(p.Log)

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
					// Bare wrap intentional: preserves context.Canceled for
					// cli/root.go::signalExitCode (SIGINT→130/SIGTERM→143).
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
				csrWarn.Warn(err.Error(), "csr: approval check failed", "err", err)
			} else {
				csrWarn.Reset()
			}
			if approved > 0 {
				totalApproved += approved
				p.Log.Info("csr: approved pending requests", "approved", approved, "csrs_approved", totalApproved)
			}
			setStatus(operatorStatusDetail(ctx, counter, totalApproved))

		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				// Bare wrap intentional: preserves context.Canceled for
				// cli/root.go::signalExitCode (SIGINT→130/SIGTERM→143).
				return fmt.Errorf("installation cancelled: %w", ctx.Err())
			}
			return &errtypes.ClusterError{
				Msg: fmt.Sprintf("installation timed out after %v — %s", opts.InstallTimeout, timeoutNextSteps(clusterDir)),
				Err: ctx.Err(),
			}
		}
	}
}

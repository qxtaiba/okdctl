package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	osExec "os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
)

// defaultStartMonitorCmd starts "openshift-install wait-for install-complete",
// wires stdout/stderr to the current TTY, and returns a buffered done channel
// and a no-op kill function. cmd.Cancel + cmd.WaitDelay handle the
// SIGTERM-then-SIGKILL escalation natively; the returned kill func remains
// for API compatibility with injected stubs.
// con:ae5b624c — canonical cmd.Cancel pattern; sub:7b2829bb mirrors this shape.
func defaultStartMonitorCmd(ctx context.Context, clusterDir string) (done <-chan error, kill func(), err error) {
	cmd := osExec.CommandContext(ctx, "openshift-install", "wait-for", "install-complete", "--dir", clusterDir, "--log-level=debug")
	// Filter env so openshift-install does not inherit AWS_*/GCP_*/AZURE_* etc. from the user shell.
	cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Send SIGTERM on ctx cancellation so openshift-install can flush its
	// in-flight diagnostics. WaitDelay gives it 30 s before SIGKILL fires.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 30 * time.Second
	if startErr := cmd.Start(); startErr != nil {
		return nil, func() {}, startErr
	}
	doneCh := make(chan error, 1)
	go func() {
		defer close(doneCh)
		doneCh <- cmd.Wait()
	}()
	return doneCh, func() {}, nil
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
		startCmd = defaultStartMonitorCmd
	}

	stopSpinner := p.Reporter("monitoring cluster operators")
	defer stopSpinner()

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

			p.Log.Info("install: completed successfully", "csrs_approved", totalApproved)
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

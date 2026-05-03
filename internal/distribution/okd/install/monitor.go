package install

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	osExec "os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// defaultStartMonitorCmd starts "openshift-install wait-for install-complete",
// wires stdout/stderr to the current TTY, and returns a buffered done channel
// and an idempotent kill function. log receives any kill-error warning when
// Kill() itself fails.
func defaultStartMonitorCmd(ctx context.Context, clusterDir string, log *slog.Logger) (<-chan error, func(), error) {
	cmd := osExec.CommandContext(ctx, "openshift-install", "wait-for", "install-complete", "--dir", clusterDir, "--log-level=debug")
	// Filter env so openshift-install does not inherit AWS_*/GCP_*/AZURE_* etc. from the user shell.
	cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, func() {}, err
	}
	done := make(chan error, 1)
	go func() {
		defer close(done)
		done <- cmd.Wait()
	}()
	kill := sync.OnceFunc(func() {
		if cmd.Process != nil {
			if killErr := cmd.Process.Kill(); killErr != nil {
				log.Warn("install: failed to kill process", "err", killErr)
			}
		}
	})
	return done, kill, nil
}

// WaitForBootstrap runs "openshift-install wait-for bootstrap-complete",
// bounded by opts.BootstrapTimeout, streaming output to the current TTY.
func (p *Phase) WaitForBootstrap(ctx context.Context, clusterDir string, opts *Options) error {
	ctx, cancel := context.WithTimeout(ctx, opts.BootstrapTimeout)
	defer cancel()

	stopSpinner := tui.StartSpinner(ctx, "waiting for bootstrap complete")
	_, err := p.Exec.RunStreamedChecked(ctx, "openshift-install", "wait-for", "bootstrap-complete", "--dir", clusterDir, "--log-level=debug")
	stopSpinner()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			// Preserve cancellation identity through the typed error so
			// callers can errors.Is(err, context.DeadlineExceeded) to
			// distinguish "we ran out of budget" from "command failed".
			return &errtypes.ClusterError{
				Msg: fmt.Sprintf("bootstrap timed out after %v", opts.BootstrapTimeout),
				Err: ctx.Err(),
			}
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return fmt.Errorf("bootstrap cancelled: %w", ctx.Err())
		}
		return &errtypes.ClusterError{Msg: "bootstrap failed", Err: err}
	}

	p.Log.Info("bootstrap: completed - control plane is ready")
	return nil
}

// csrApprover is the subset of cluster.K8sClient MonitorInstallation uses.
// Accepting the interface instead of the concrete type lets tests inject a
// stub without a real kubeconfig.
type csrApprover interface {
	ApprovePendingCSRs(ctx context.Context) (int, error)
}

// MonitorInstallation watches the post-bootstrap install until all cluster
// operators are Available, bounded by opts.InstallTimeout. If approver is
// nil a real cluster.K8sClient is constructed from clusterDir/auth/kubeconfig.
func (p *Phase) MonitorInstallation(ctx context.Context, clusterDir string, opts *Options, approver csrApprover) error {
	ctx, cancel := context.WithTimeout(ctx, opts.InstallTimeout)
	defer cancel()

	if approver == nil {
		kubeconfigPath := filepath.Join(clusterDir, "auth", "kubeconfig")
		approver = cluster.NewK8sClient(
			cluster.WithCLI("oc"),
			cluster.WithKubeconfig(kubeconfigPath),
			cluster.WithLogger(p.Log),
		)
	}

	startCmd := p.startMonitorCmd
	if startCmd == nil {
		log := p.Log
		startCmd = func(ctx context.Context, dir string) (<-chan error, func(), error) {
			return defaultStartMonitorCmd(ctx, dir, log)
		}
	}

	stopSpinner := tui.StartSpinner(ctx, "monitoring cluster operators")
	defer stopSpinner()

	installDone, killInstall, err := startCmd(ctx, clusterDir)
	if err != nil {
		return &errtypes.ClusterError{Msg: "failed to start installation monitor", Err: err}
	}

	ticker := time.NewTicker(opts.CSRApprovalInterval)
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
						Msg: fmt.Sprintf("installation timed out after %v", opts.InstallTimeout),
						Err: ctx.Err(),
					}
				}
				if errors.Is(ctx.Err(), context.Canceled) {
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
			killInstall()
			// Give the just-killed openshift-install 30s to exit and flush
			// its final output; then give up rather than blocking shutdown.
			// The goroutine above still holds Wait() on the dead process —
			// it will eventually return and send to installDone, but the
			// buffered channel means we don't leak a blocked sender if we
			// abandon early.
			reapTimer := time.NewTimer(30 * time.Second)
			select {
			case <-installDone:
				reapTimer.Stop()
			case <-reapTimer.C:
				p.Log.Warn("install: process did not exit after kill, abandoning reap")
			}
			if errors.Is(ctx.Err(), context.Canceled) {
				return fmt.Errorf("installation cancelled: %w", ctx.Err())
			}
			return &errtypes.ClusterError{
				Msg: fmt.Sprintf("installation timed out after %v", opts.InstallTimeout),
				Err: ctx.Err(),
			}
		}
	}
}

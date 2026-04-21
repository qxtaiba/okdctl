package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	osExec "os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// WaitForBootstrap runs "openshift-install wait-for bootstrap-complete",
// bounded by opts.BootstrapTimeout, streaming output to the current TTY.
func (p *Phase) WaitForBootstrap(ctx context.Context, clusterDir string, opts *Options) error {
	ctx, cancel := context.WithTimeout(ctx, opts.BootstrapTimeout)
	defer cancel()

	cmd := osExec.CommandContext(ctx, "openshift-install", "wait-for", "bootstrap-complete", "--dir", clusterDir, "--log-level=debug")

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	stopSpinner := tui.StartSpinner(ctx, "waiting for bootstrap complete")
	err := cmd.Run()
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

// MonitorInstallation watches the post-bootstrap install until all cluster
// operators are Available, bounded by opts.InstallTimeout.
func (p *Phase) MonitorInstallation(ctx context.Context, clusterDir string, opts *Options) error {
	ctx, cancel := context.WithTimeout(ctx, opts.InstallTimeout)
	defer cancel()

	kubeconfigPath := filepath.Join(clusterDir, "auth", "kubeconfig")
	k8sClient := cluster.NewK8sClient(
		cluster.WithCLI("oc"),
		cluster.WithKubeconfig(kubeconfigPath),
		cluster.WithLogger(p.Log),
	)

	installCmd := osExec.CommandContext(ctx, "openshift-install", "wait-for", "install-complete", "--dir", clusterDir, "--log-level=debug")
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr

	stopSpinner := tui.StartSpinner(ctx, "monitoring cluster operators")
	defer stopSpinner()

	if err := installCmd.Start(); err != nil {
		return &errtypes.ClusterError{Msg: "failed to start installation monitor", Err: err}
	}

	installDone := make(chan error, 1)
	go func() {
		defer close(installDone)
		installDone <- installCmd.Wait()
	}()

	// sync.OnceFunc keeps kill idempotent if a future signal handler or
	// additional select case ends up invoking killInstall twice.
	killInstall := sync.OnceFunc(func() {
		if installCmd.Process != nil {
			if killErr := installCmd.Process.Kill(); killErr != nil {
				p.Log.Warn("install: failed to kill process", "err", killErr)
			}
		}
	})

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
					return fmt.Errorf("installation timed out after %v: %w", opts.InstallTimeout, ctx.Err())
				}
				if errors.Is(ctx.Err(), context.Canceled) {
					return fmt.Errorf("installation cancelled: %w", ctx.Err())
				}
				return &errtypes.ClusterError{Msg: "installation failed", Err: err}
			}

			approved, csrErr := k8sClient.ApprovePendingCSRs(ctx)
			if csrErr != nil {
				p.Log.Warn("csr: final approval had issues", "err", csrErr)
			}
			totalApproved += approved

			p.Log.Info("install: completed successfully", "csrs_approved", totalApproved)
			return nil

		case <-ticker.C:
			approved, err := k8sClient.ApprovePendingCSRs(ctx)
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
			return fmt.Errorf("installation timed out after %v: %w", opts.InstallTimeout, ctx.Err())
		}
	}
}

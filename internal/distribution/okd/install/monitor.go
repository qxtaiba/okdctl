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
	"github.com/qxtaiba/okdctl/internal/tui"
)

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
			return fmt.Errorf("bootstrap timed out after %v", opts.BootstrapTimeout)
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return fmt.Errorf("bootstrap cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("bootstrap failed: %w", err)
	}

	p.Log.Info("bootstrap: completed - control plane is ready")
	return nil
}

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
		return fmt.Errorf("failed to start installation monitor: %w", err)
	}

	installDone := make(chan error, 1)
	go func() {
		defer close(installDone)
		installDone <- installCmd.Wait()
	}()

	// sync.Once guards against a future second caller of killInstall
	// (e.g. if a signal handler or additional select case is added). Under
	// the current single-kill-path control flow, the Once is not load-
	// bearing — it is idempotency-by-construction for the next developer.
	var killOnce sync.Once
	killInstall := func() {
		killOnce.Do(func() {
			if installCmd.Process != nil {
				if killErr := installCmd.Process.Kill(); killErr != nil {
					p.Log.Warn(fmt.Sprintf("install: failed to kill process: %v", killErr))
				}
			}
		})
	}

	ticker := time.NewTicker(opts.CSRApprovalInterval)
	defer ticker.Stop()

	totalApproved := 0

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
				return fmt.Errorf("installation failed: %w", err)
			}

			approved, csrErr := k8sClient.ApprovePendingCSRs(ctx)
			if csrErr != nil {
				p.Log.Warn(fmt.Sprintf("csr: final approval had issues: %v", csrErr))
			}
			totalApproved += approved

			p.Log.Info(fmt.Sprintf("install: completed successfully - approved %d csrs total", totalApproved))
			return nil

		case <-ticker.C:
			approved, err := k8sClient.ApprovePendingCSRs(ctx)
			if err != nil {
				p.Log.Warn(fmt.Sprintf("csr: approval check failed: %v", err))
			}
			if approved > 0 {
				totalApproved += approved
				p.Log.Info(fmt.Sprintf("csr: approved %d pending requests (%d total)", approved, totalApproved))
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

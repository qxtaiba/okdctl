package install

import (
	"context"
	"fmt"
	"os"
	osExec "os/exec"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/cluster"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

// WaitForBootstrap waits for the bootstrap node to complete.
func (p *Phase) WaitForBootstrap(ctx context.Context, clusterDir string, opts Options) error {
	ctx, cancel := context.WithTimeout(ctx, opts.BootstrapTimeout)
	defer cancel()

	cmd := osExec.CommandContext(ctx, "openshift-install", "wait-for", "bootstrap-complete", "--dir", clusterDir, "--log-level=debug")

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("bootstrap timed out after %v", opts.BootstrapTimeout)
		}
		if ctx.Err() == context.Canceled {
			return utils.WrapError("bootstrap cancelled", context.Canceled)
		}
		return utils.WrapError("bootstrap failed", err)
	}

	p.LogInfo("bootstrap: completed - control plane is ready")
	return nil
}

// MonitorInstallation monitors the installation process and approves CSRs.
func (p *Phase) MonitorInstallation(ctx context.Context, clusterDir string, opts Options) error {
	ctx, cancel := context.WithTimeout(ctx, opts.InstallTimeout)
	defer cancel()

	kubeconfigPath := filepath.Join(clusterDir, "auth", "kubeconfig")
	k8sClient := cluster.NewK8sClient(
		cluster.WithCLI("oc"),
		cluster.WithKubeconfig(kubeconfigPath),
	)

	installCmd := osExec.CommandContext(ctx, "openshift-install", "wait-for", "install-complete", "--dir", clusterDir, "--log-level=debug")
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr

	if err := installCmd.Start(); err != nil {
		return utils.WrapError("failed to start installation monitor", err)
	}

	installDone := make(chan error, 1)
	go func() {
		defer close(installDone)
		installDone <- installCmd.Wait()
	}()

	ticker := time.NewTicker(opts.CSRApprovalInterval)
	defer ticker.Stop()

	totalApproved := 0

	for {
		select {
		case err := <-installDone:
			if err != nil {
				return utils.WrapError("installation failed", err)
			}

			approved, csrErr := k8sClient.ApprovePendingCSRs(ctx)
			if csrErr != nil {
				p.LogWarn(fmt.Sprintf("csr: final approval had issues: %v", csrErr))
			}
			totalApproved += approved

			p.LogInfo(fmt.Sprintf("install: completed successfully - approved %d csrs total", totalApproved))
			return nil

		case <-ticker.C:
			approved, err := k8sClient.ApprovePendingCSRs(ctx)
			if err != nil {
				p.LogWarn(fmt.Sprintf("csr: approval check failed: %v", err))
			}
			if approved > 0 {
				totalApproved += approved
				p.LogInfo(fmt.Sprintf("csr: approved %d pending requests (%d total)", approved, totalApproved))
			}

		case <-ctx.Done():
			if installCmd.Process != nil {
				if killErr := installCmd.Process.Kill(); killErr != nil {
					p.LogWarn(fmt.Sprintf("install: failed to kill process: %v", killErr))
				}
			}
			select {
			case <-installDone:
			case <-time.After(5 * time.Second):
				p.LogWarn("install: timed out waiting for process cleanup")
			}
			return fmt.Errorf("installation timed out after %v", opts.InstallTimeout)
		}
	}
}

package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// ValidateClusterAccess verifies that the cluster is accessible using the kubeconfig.
func (p *Phase) ValidateClusterAccess(ctx context.Context) error {
	cmdRunner := p.Exec

	p.LogInfo("cluster: validating access with oc whoami")

	result, err := cmdRunner.Run(ctx, "oc", "whoami")
	if err != nil {
		return utils.WrapError("failed to run oc whoami", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("cluster authentication failed: %s", result.Stderr)
	}

	user := strings.TrimSpace(result.Stdout)
	if user == "" {
		return fmt.Errorf("cluster authentication returned empty user")
	}

	p.LogInfo(fmt.Sprintf("cluster: authenticated as %s", user))

	result, err = cmdRunner.Run(ctx, "oc", "version")
	if err == nil && result.ExitCode == 0 {
		lines := strings.Split(result.Stdout, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "Server Version:") {
				p.LogInfo(fmt.Sprintf("cluster: %s", strings.ToLower(strings.TrimSpace(line))))
				break
			}
		}
	}

	return nil
}

// SetupClusterAccess configures persistent kubeconfig for the user.
func (p *Phase) SetupClusterAccess(ctx context.Context, clusterDir string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return utils.WrapError("failed to get user home directory", err)
	}

	kubeDir := filepath.Join(homeDir, ".kube")
	if err := system.EnsureDir(kubeDir); err != nil {
		return utils.WrapError("failed to create .kube directory", err)
	}

	srcKubeconfig := filepath.Join(clusterDir, "auth", "kubeconfig")
	destKubeconfig := filepath.Join(kubeDir, "config")

	if system.FileExists(destKubeconfig) {
		backupPath := destKubeconfig + ".backup." + time.Now().Format("20060102-150405")
		if err := system.CopyFile(destKubeconfig, backupPath); err != nil {
			p.Log.Warn(fmt.Sprintf("kubeconfig: could not backup existing file: %v", err))
		} else {
			p.Log.Info(fmt.Sprintf("kubeconfig: backed up existing file to %s", backupPath))
		}
	}

	if err := system.CopyFile(srcKubeconfig, destKubeconfig); err != nil {
		return utils.WrapError("failed to copy kubeconfig", err)
	}

	if err := os.Chmod(destKubeconfig, 0600); err != nil {
		return utils.WrapError("failed to set kubeconfig permissions", err)
	}

	if err := p.addKubeconfigToBashrc(homeDir, destKubeconfig); err != nil {
		p.Log.Warn(fmt.Sprintf("kubeconfig: could not update .bashrc: %v", err))
	}

	return nil
}

// addKubeconfigToBashrc appends KUBECONFIG export to .bashrc if not present.
func (p *Phase) addKubeconfigToBashrc(homeDir, kubeconfigPath string) error {
	bashrcPath := filepath.Join(homeDir, ".bashrc")
	exportLine := fmt.Sprintf("export KUBECONFIG=%s", kubeconfigPath)

	content, err := os.ReadFile(bashrcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return system.AtomicWriteString(bashrcPath, exportLine+"\n", 0644)
		}
		return err
	}

	if strings.Contains(string(content), "export KUBECONFIG=") {
		return nil
	}

	f, err := os.OpenFile(bashrcPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(f, "\n# Added by okd-proxmox-cli\n%s\n", exportLine); err != nil {
		return err
	}

	return nil
}


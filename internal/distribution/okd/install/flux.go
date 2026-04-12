package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/system"
)

func (p *Phase) ValidateClusterAccess(ctx context.Context) error {
	cmdRunner := p.Exec

	p.Log.Info("cluster: validating access with oc whoami")

	result, err := cmdRunner.RunChecked(ctx, "oc", "whoami")
	if err != nil {
		return fmt.Errorf("failed to run oc whoami: %w", err)
	}

	user := strings.TrimSpace(result.Stdout)
	if user == "" {
		return fmt.Errorf("cluster authentication returned empty user")
	}

	p.Log.Info(fmt.Sprintf("cluster: authenticated as %s", user))

	result, err = cmdRunner.Run(ctx, "oc", "version")
	if err == nil && result.ExitCode == 0 {
		lines := strings.Split(result.Stdout, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "Server Version:") {
				p.Log.Info(fmt.Sprintf("cluster: %s", strings.ToLower(strings.TrimSpace(line))))
				break
			}
		}
	}

	return nil
}

func (p *Phase) SetupClusterAccess(_ context.Context, clusterDir string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	kubeDir := filepath.Join(homeDir, ".kube")
	if err := system.EnsureDir(kubeDir); err != nil {
		return fmt.Errorf("failed to create .kube directory: %w", err)
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

	if err := system.CopyFileMode(srcKubeconfig, destKubeconfig, 0o600); err != nil {
		return fmt.Errorf("failed to copy kubeconfig: %w", err)
	}

	if err := p.addKubeconfigToBashrc(homeDir, destKubeconfig); err != nil {
		p.Log.Warn(fmt.Sprintf("kubeconfig: could not update .bashrc: %v", err))
	}

	return nil
}

func (p *Phase) addKubeconfigToBashrc(homeDir, kubeconfigPath string) error {
	bashrcPath := filepath.Join(homeDir, ".bashrc")
	exportLine := fmt.Sprintf("export KUBECONFIG=%s", kubeconfigPath)

	// Preserve the existing .bashrc mode so appending an export line can't
	// silently relax stricter perms the user may have set. 0644 is only
	// used when the file does not yet exist (sane default for bashrc).
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(bashrcPath); err == nil {
		mode = fi.Mode().Perm()
	}

	content, err := os.ReadFile(bashrcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return system.AtomicWriteString(bashrcPath, exportLine+"\n", mode)
		}
		return err
	}

	if strings.Contains(string(content), "export KUBECONFIG=") {
		return nil
	}

	f, err := os.OpenFile(bashrcPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(f, "\n# Added by okdctl\n%s\n", exportLine); err != nil {
		return err
	}

	return nil
}

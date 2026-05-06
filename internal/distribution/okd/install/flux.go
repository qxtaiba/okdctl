package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// ValidateClusterAccess runs "oc whoami" to confirm the kubeconfig points at
// a live cluster and logs the server version when available.
func (p *Phase) ValidateClusterAccess(ctx context.Context) error {
	cmdRunner := p.Exec

	p.Log.Info("cluster: validating access with oc whoami")

	result, err := cmdRunner.RunChecked(ctx, "oc", "whoami")
	if err != nil {
		return &errtypes.ClusterError{Msg: "failed to run oc whoami", Err: err}
	}

	user := strings.TrimSpace(result.Stdout)
	if user == "" {
		return &errtypes.ClusterError{Msg: "cluster authentication returned empty user"}
	}

	p.Log.Info(fmt.Sprintf("cluster: authenticated as %s", user))

	result, err = cmdRunner.Run(ctx, "oc", "version")
	if err == nil && result.ExitCode == 0 {
		for line := range strings.Lines(result.Stdout) {
			if strings.HasPrefix(line, "Server Version:") {
				p.Log.Info(fmt.Sprintf("cluster: %s", strings.ToLower(strings.TrimSpace(line))))
				break
			}
		}
	}

	return nil
}

// SetupClusterAccess installs the generated kubeconfig into the invoking
// user's ~/.kube/config, chowning paths so the file is usable after any
// sudo re-exec returns.
func (p *Phase) SetupClusterAccess(ctx context.Context, clusterDir string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("setup cluster access: %w", err)
	}
	// Resolve the invoking user's home (not root's) so files land where
	// the user will look for them after the re-exec'd deploy returns.
	homeDir, err := system.InvokingUserHomeDir()
	if err != nil {
		return &errtypes.ConfigError{Msg: "failed to resolve invoking user home", Err: err}
	}

	kubeDir := filepath.Join(homeDir, ".kube")
	if err := system.EnsureDir(kubeDir); err != nil {
		return &errtypes.ConfigError{Msg: "failed to create .kube directory", Err: err}
	}
	if err := system.ChownToInvokingUser(kubeDir); err != nil {
		p.Log.Warn("kubeconfig: could not chown .kube dir", "err", err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("setup cluster access: %w", err)
	}
	srcKubeconfig := filepath.Join(clusterDir, "auth", "kubeconfig")
	destKubeconfig := filepath.Join(kubeDir, "config")

	if system.FileExists(destKubeconfig) {
		backupPath := destKubeconfig + ".backup." + time.Now().Format("20060102-150405")
		if err := system.CopyFileMode(destKubeconfig, backupPath, 0o600); err != nil {
			p.Log.Warn("kubeconfig: could not backup existing file", "err", err)
		} else {
			_ = system.ChownToInvokingUser(backupPath)
			p.Log.Info(fmt.Sprintf("kubeconfig: backed up existing file to %s", backupPath))
		}
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("setup cluster access: %w", err)
	}
	if err := system.CopyFileMode(srcKubeconfig, destKubeconfig, 0o600); err != nil {
		return &errtypes.ConfigError{Msg: "failed to copy kubeconfig", Err: err}
	}
	if err := system.ChownToInvokingUser(destKubeconfig); err != nil {
		p.Log.Warn("kubeconfig: could not chown config", "err", err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("setup cluster access: %w", err)
	}
	if err := p.addKubeconfigToBashrc(homeDir, destKubeconfig); err != nil {
		p.Log.Warn("kubeconfig: could not update .bashrc", "err", err)
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
	created := false
	if fi, err := os.Stat(bashrcPath); err == nil {
		mode = fi.Mode().Perm()
	} else if os.IsNotExist(err) {
		created = true
	}

	// Lstat before ReadFile: refuse to follow a symlink that could redirect
	// a privileged write to an attacker-controlled path under sudo re-exec.
	if lfi, lstatErr := os.Lstat(bashrcPath); lstatErr == nil {
		if lfi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to modify %s: path is a symlink", bashrcPath)
		}
	}

	content, err := os.ReadFile(bashrcPath)
	if err != nil {
		if os.IsNotExist(err) {
			if writeErr := system.AtomicWriteString(bashrcPath, exportLine+"\n", mode); writeErr != nil {
				return writeErr
			}
			if created {
				return system.ChownToInvokingUser(bashrcPath)
			}
			return nil
		}
		return err
	}

	if strings.Contains(string(content), "export KUBECONFIG=") {
		return nil
	}

	newContent := string(content)
	if newContent != "" && newContent[len(newContent)-1] != '\n' {
		newContent += "\n"
	}
	newContent += "\n# Added by okdctl\n" + exportLine + "\n"
	if err := system.AtomicWriteString(bashrcPath, newContent, mode); err != nil {
		return err
	}
	// AtomicWrite under sudo leaves the file root-owned — restore invoking-user ownership.
	return system.ChownToInvokingUser(bashrcPath)
}

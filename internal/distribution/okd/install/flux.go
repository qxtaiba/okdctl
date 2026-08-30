package install

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// invokingUserHomeDirFn resolves the invoking user's home; tests override it.
var invokingUserHomeDirFn = system.InvokingUserHomeDir

// ValidateClusterAccess confirms the kubeconfig points at a live cluster via oc whoami.
func (p *Phase) ValidateClusterAccess(ctx context.Context) error {
	p.Log.Debug("cluster: validating access with oc whoami")

	user, err := p.OcOutput(ctx, "whoami")
	if err != nil {
		return &errtypes.ClusterError{Msg: "run oc whoami", Err: err}
	}
	if user == "" {
		return &errtypes.ClusterError{Msg: "cluster authentication returned empty user"}
	}

	p.Log.Info("cluster: authenticated", "user", user)

	versionOut, err := p.OcOutput(ctx, "version")
	if err == nil {
		for line := range strings.Lines(versionOut) {
			if strings.HasPrefix(line, "Server Version:") {
				version := strings.TrimSpace(strings.TrimPrefix(line, "Server Version:"))
				p.Log.Info("cluster: server version", "version", version)
				break
			}
		}
	}

	return nil
}

// SetupClusterAccess installs the generated kubeconfig into the invoking user's
// ~/.kube/config, chowning paths for post-sudo use.
func (p *Phase) SetupClusterAccess(ctx context.Context, clusterDir string) error {
	// Bare return preserves context.Canceled identity for signalExitCode.
	if err := ctx.Err(); err != nil {
		return err
	}
	// Invoking user's home, not root's, so files land where expected post re-exec.
	homeDir, err := invokingUserHomeDirFn()
	if err != nil {
		return &errtypes.ConfigError{Msg: "resolve invoking user home", Err: err}
	}

	kubeDir := filepath.Join(homeDir, ".kube")
	if err := system.EnsureDir(kubeDir); err != nil {
		return &errtypes.ConfigError{Msg: "create .kube directory", Err: err}
	}
	if err := system.ChownToInvokingUser(kubeDir); err != nil {
		p.Log.Warn("kubeconfig: could not chown .kube dir", "err", err)
	}

	srcKubeconfig := workspace.KubeconfigPath(clusterDir)
	destKubeconfig := filepath.Join(kubeDir, "config")

	// Skip backup when content already matches, so idempotent re-runs don't pile up backups.
	if system.FileExists(destKubeconfig) && !sameFileContent(srcKubeconfig, destKubeconfig) {
		backupPath := destKubeconfig + ".backup." + time.Now().Format("20060102-150405")
		if err := system.CopyFileMode(destKubeconfig, backupPath, 0o600); err != nil {
			p.Log.Warn("kubeconfig: could not backup existing file", "err", err)
		} else {
			_ = system.ChownToInvokingUser(backupPath)
			p.Log.Info("kubeconfig: backed up existing file", "path", backupPath)
		}
	}

	if err := system.CopyFileMode(srcKubeconfig, destKubeconfig, 0o600); err != nil {
		return &errtypes.ConfigError{Msg: "copy kubeconfig", Err: err}
	}
	if err := system.ChownToInvokingUser(destKubeconfig); err != nil {
		p.Log.Warn("kubeconfig: could not chown config", "err", err)
	}

	if err := p.addKubeconfigToBashrc(homeDir, destKubeconfig); err != nil {
		p.Log.Warn("kubeconfig: could not update .bashrc", "err", err)
	}

	return nil
}

// sameFileContent treats a read error as false so the caller takes a backup.
func sameFileContent(a, b string) bool {
	da, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	db, err := os.ReadFile(b)
	if err != nil {
		return false
	}
	return bytes.Equal(da, db)
}

func (p *Phase) addKubeconfigToBashrc(homeDir, kubeconfigPath string) error {
	bashrcPath := filepath.Join(homeDir, ".bashrc")
	exportLine := fmt.Sprintf("export KUBECONFIG=%s", kubeconfigPath)

	// Preserve existing .bashrc mode; 0644 only applies when creating a new file.
	mode := os.FileMode(0o644)
	created := false
	if fi, err := os.Stat(bashrcPath); err == nil {
		mode = fi.Mode().Perm()
	} else if errors.Is(err, os.ErrNotExist) {
		created = true
	}

	// Lstat gives a clear symlink diagnostic; O_NOFOLLOW below closes the TOCTOU window.
	if lfi, lstatErr := os.Lstat(bashrcPath); lstatErr == nil {
		if lfi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to modify %s: path is a symlink", bashrcPath)
		}
	}

	f, err := os.OpenFile(bashrcPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
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
	defer f.Close()
	content, err := io.ReadAll(f)
	if err != nil {
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

package cluster

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestValidateKubeconfigEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink and path tests rely on POSIX semantics")
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	kubeDir := filepath.Join(tmpHome, ".kube")
	if err := os.MkdirAll(kubeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	kubeconfig := filepath.Join(kubeDir, "config")
	if err := os.WriteFile(kubeconfig, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	linkTarget := filepath.Join(tmpHome, "real-file")
	if err := os.WriteFile(linkTarget, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(tmpHome, "link-to-real")
	if err := os.Symlink(linkTarget, symlinkPath); err != nil {
		t.Fatal(err)
	}

	t.Run("dev_zero_rejected", func(t *testing.T) {
		err := validateKubeconfigEnv("/dev/zero")
		if err == nil {
			t.Error("expected /dev/zero to be rejected; got nil")
		}
	})

	t.Run("proc_self_environ_rejected", func(t *testing.T) {
		err := validateKubeconfigEnv("/proc/self/environ")
		if err == nil {
			t.Error("expected /proc/self/environ to be rejected; got nil")
		}
	})

	t.Run("symlink_rejected", func(t *testing.T) {
		err := validateKubeconfigEnv(symlinkPath)
		if err == nil {
			t.Fatal("expected symlink inside $HOME to be rejected; got nil")
		}
		if !strings.Contains(err.Error(), "symlink") {
			t.Errorf("error %q does not contain 'symlink'", err.Error())
		}
	})

	t.Run("home_kube_config_accepted", func(t *testing.T) {
		if err := validateKubeconfigEnv(kubeconfig); err != nil {
			t.Errorf("expected $HOME/.kube/config to be accepted; got %v", err)
		}
	})

	t.Run("traversal_outside_prefix_rejected", func(t *testing.T) {
		// /etc/foo/../../../tmp/attack cleans to /tmp/attack, outside both prefixes.
		err := validateKubeconfigEnv("/etc/foo/../../../tmp/attack")
		if err == nil {
			t.Error("expected traversal-to-/tmp path to be rejected; got nil")
		}
	})

	t.Run("missing_path_rejected", func(t *testing.T) {
		missing := filepath.Join(tmpHome, "does-not-exist", "kubeconfig")
		err := validateKubeconfigEnv(missing)
		if err == nil {
			t.Fatal("expected missing path to be rejected; got nil")
		}
		if !strings.Contains(err.Error(), "inaccessible") {
			t.Errorf("error %q does not contain 'inaccessible'", err.Error())
		}
	})

	t.Run("prefix_spoof_etcd_rejected", func(t *testing.T) {
		// /etcd/foo shares the '/etc' byte prefix but not '/etc/'; even if the
		// path existed the sep-guarded HasPrefix check would reject it.
		err := validateKubeconfigEnv("/etcd/foo")
		if err == nil {
			t.Error("expected /etcd/foo to be rejected; got nil")
		}
	})

	t.Run("perm_0600_accepted", func(t *testing.T) {
		f := filepath.Join(kubeDir, "config-0600")
		if err := os.WriteFile(f, []byte(""), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := validateKubeconfigEnv(f); err != nil {
			t.Errorf("expected 0o600 kubeconfig to be accepted; got %v", err)
		}
	})

	t.Run("perm_0644_rejected_with_auth_error", func(t *testing.T) {
		f := filepath.Join(kubeDir, "config-0644")
		if err := os.WriteFile(f, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
		err := validateKubeconfigEnv(f)
		if err == nil {
			t.Fatal("expected 0o644 kubeconfig to be rejected; got nil")
		}
		var ae *errtypes.AuthError
		if !errors.As(err, &ae) {
			t.Errorf("expected *errtypes.AuthError; got %T: %v", err, err)
		}
		if !strings.Contains(err.Error(), "insecure permissions") {
			t.Errorf("error %q does not contain 'insecure permissions'", err.Error())
		}
	})

	t.Run("perm_0620_rejected_with_auth_error", func(t *testing.T) {
		f := filepath.Join(kubeDir, "config-0620")
		if err := os.WriteFile(f, []byte(""), 0o620); err != nil {
			t.Fatal(err)
		}
		err := validateKubeconfigEnv(f)
		if err == nil {
			t.Fatal("expected 0o620 kubeconfig to be rejected; got nil")
		}
		var ae *errtypes.AuthError
		if !errors.As(err, &ae) {
			t.Errorf("expected *errtypes.AuthError; got %T: %v", err, err)
		}
	})
}

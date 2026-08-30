package cluster

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/testutil"
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
	writeKubeconfig := func(name string, mode os.FileMode) string {
		t.Helper()
		f := filepath.Join(kubeDir, name)
		if err := os.WriteFile(f, []byte(""), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(f, mode); err != nil {
			t.Fatal(err)
		}
		return f
	}

	linkTarget := filepath.Join(tmpHome, "real-file")
	if err := os.WriteFile(linkTarget, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(tmpHome, "link-to-real")
	if err := os.Symlink(linkTarget, symlinkPath); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		path       string
		wantErr    bool
		wantSubstr string // asserted on the error only when wantErr
	}{
		{name: "dev_zero_rejected", path: "/dev/zero", wantErr: true},
		{name: "proc_self_environ_rejected", path: "/proc/self/environ", wantErr: true},
		{name: "symlink_rejected", path: symlinkPath, wantErr: true, wantSubstr: "symlink"},
		{name: "home_kube_config_accepted", path: writeKubeconfig("config", 0o600)},
		// /etc/foo/../../../tmp/attack cleans to /tmp/attack, outside both prefixes.
		{name: "traversal_outside_prefix_rejected", path: "/etc/foo/../../../tmp/attack", wantErr: true},
		{name: "missing_path_rejected", path: filepath.Join(tmpHome, "does-not-exist", "kubeconfig"), wantErr: true, wantSubstr: "inaccessible"},
		// /etcd/foo shares the '/etc' prefix but not '/etc/'; sep-guard rejects it.
		{name: "prefix_spoof_etcd_rejected", path: "/etcd/foo", wantErr: true},
		{name: "perm_0600_accepted", path: writeKubeconfig("config-0600", 0o600)},
		{name: "perm_0644_rejected", path: writeKubeconfig("config-0644", 0o644), wantErr: true, wantSubstr: "insecure permissions"},
		{name: "perm_0620_rejected", path: writeKubeconfig("config-0620", 0o620), wantErr: true, wantSubstr: "insecure permissions"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateKubeconfigEnv(tc.path)
			if !tc.wantErr {
				if err != nil {
					t.Errorf("expected %s to be accepted; got %v", tc.path, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected %s to be rejected; got nil", tc.path)
			}
			if tc.wantSubstr != "" && !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}

func TestWithExecutor_SharesInjectedExecutorEnv(t *testing.T) {
	testutil.InstallFakeBin(t, "oc", `#!/bin/sh
echo "KUBECONFIG=$KUBECONFIG"
exit 0
`)

	exec := executor.New()
	c := New(WithCLI("oc"), WithExecutor(exec))
	// AppendEnv runs after construction; Run must still see it (shared pointer, not a copy).
	exec.AppendEnv("KUBECONFIG=/tmp/shared-kubeconfig")

	result, err := c.Run(context.Background(), "get", "pods")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Stdout, "KUBECONFIG=/tmp/shared-kubeconfig") {
		t.Errorf("stdout = %q; want KUBECONFIG env visible via the shared executor", result.Stdout)
	}
}

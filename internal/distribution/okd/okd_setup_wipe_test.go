package okd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// These tests exercise only the pre-phase contract (guard, cleanup abort,
// wipe-vs-retain) since Setup's real phase steps touch the host; none reach
// setupPhase.Execute.

func seedEmptyTFState(t *testing.T, projectRoot string) {
	t.Helper()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"version":4,"resources":[]}`)
	if err := os.WriteFile(filepath.Join(envDir, "terraform.tfstate"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func seedAuthCreds(t *testing.T, workDir string) (kubeconfig, kubeadmin string) {
	t.Helper()
	authDir := filepath.Join(workspace.ClusterConfigDir(workDir), "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	kubeconfig = filepath.Join(authDir, "kubeconfig")
	kubeadmin = filepath.Join(authDir, "kubeadmin-password")
	for _, p := range []string{kubeconfig, kubeadmin} {
		if err := os.WriteFile(p, []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return kubeconfig, kubeadmin
}

func seedGeneratedDir(t *testing.T, workDir, name string) string {
	t.Helper()
	dir := filepath.Join(workDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "artifact"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	t.Fatalf("stat %s: %v", path, err)
	return false
}

func TestSetupRefusesLiveClusterBeforeWipe(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, workspace.WorkDirName)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seedTFState(t, root)
	kubeconfig, kubeadmin := seedAuthCreds(t, workDir)
	downloads := seedGeneratedDir(t, workDir, "downloads")

	p := New(WithProjectRoot(root), WithLogger(logutil.NopLogger))
	_, err := p.Setup(context.Background(), config.DefaultConfig(), SetupOpts{})

	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("err = %v; want *errtypes.ConfigError", err)
	}
	for _, path := range []string{kubeconfig, kubeadmin, downloads} {
		if !exists(t, path) {
			t.Errorf("guard refused but %s was removed", path)
		}
	}
}

func TestSetupCleanupIncompleteAborts(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the directory permission that forces the cleanup failure")
	}
	root := t.TempDir()
	workDir := filepath.Join(root, workspace.WorkDirName)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 0o500 on the parent blocks unlinking the file inside, so RemoveAll
	// fails and the cleanup step reports incomplete.
	locked := filepath.Join(workDir, "downloads", "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "artifact"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	p := New(WithProjectRoot(root), WithLogger(logutil.NopLogger))
	_, err := p.Setup(context.Background(), config.DefaultConfig(), SetupOpts{})

	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Fatalf("err = %v; want *errtypes.ClusterError", err)
	}
	if !strings.Contains(clusterErr.Msg, "pre-deploy cleanup incomplete") {
		t.Errorf("ClusterError.Msg = %q; want it to name 'pre-deploy cleanup incomplete'", clusterErr.Msg)
	}
}

func TestSetupWipeDecision(t *testing.T) {
	tests := []struct {
		name         string
		seedState    func(t *testing.T, projectRoot string)
		forceWipe    bool
		wantRetained bool
	}{
		{
			name:         "populated state no force retains credentials",
			seedState:    seedTFState,
			forceWipe:    false,
			wantRetained: true,
		},
		{
			name:         "empty state no force wipes everything",
			seedState:    seedEmptyTFState,
			forceWipe:    false,
			wantRetained: false,
		},
		{
			name:         "populated state with force wipes credentials",
			seedState:    seedTFState,
			forceWipe:    true,
			wantRetained: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			workDir := filepath.Join(root, workspace.WorkDirName)
			if err := os.MkdirAll(workDir, 0o755); err != nil {
				t.Fatal(err)
			}
			tc.seedState(t, root)
			kubeconfig, _ := seedAuthCreds(t, workDir)
			downloads := seedGeneratedDir(t, workDir, "downloads")

			cfg := config.DefaultConfig()
			p := New(WithProjectRoot(root), WithLogger(logutil.NopLogger))
			opts := cleanup.NewOptions(cfg, root, cleanup.WorkOnly)
			opts.ForceCredentialWipe = tc.forceWipe

			if err := p.Cleanup(context.Background(), &opts); err != nil {
				t.Fatalf("Cleanup: %v", err)
			}

			// downloads is always wiped regardless of retention.
			if exists(t, downloads) {
				t.Errorf("downloads survived; want it wiped")
			}
			if got := exists(t, kubeconfig); got != tc.wantRetained {
				t.Errorf("kubeconfig exists = %v; want %v", got, tc.wantRetained)
			}
			if got := exists(t, workDir); got != tc.wantRetained {
				t.Errorf("work dir exists = %v; want %v", got, tc.wantRetained)
			}
		})
	}
}

package cleanup

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func seedCredentialWorkDir(t *testing.T) string {
	t.Helper()
	workDir := t.TempDir()
	authDir := filepath.Join(workDir, "cluster-config", "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"kubeconfig", "kubeadmin-password"} {
		if err := os.WriteFile(filepath.Join(authDir, name), []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(workDir, "downloads"), 0o755); err != nil {
		t.Fatal(err)
	}
	return workDir
}

func seedStateFile(t *testing.T, projectRoot, body string) {
	t.Helper()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "terraform.tfstate"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func credentialGuardOpts(workDir, projectRoot string) *Options {
	return &Options{
		BaseOptions: phase.BaseOptions{
			WorkDir:      workDir,
			ProjectRoot:  projectRoot,
			TerraformEnv: "production",
		},
		Kind: WorkOnly,
	}
}

// TestExecute_LiveStatePreservesClusterCredentials locks the blocker fix:
// a Full/WorkOnly cleanup against populated terraform state must not delete
// cluster-config (the only copy of kubeconfig/kubeadmin-password) while the
// VMs it grants access to are still deployed.
func TestExecute_LiveStatePreservesClusterCredentials(t *testing.T) {
	workDir := seedCredentialWorkDir(t)
	projectRoot := t.TempDir()
	seedStateFile(t, projectRoot, `{"version":4,"resources":[{"type":"proxmox_virtual_environment_vm"}]}`)

	opts := credentialGuardOpts(workDir, projectRoot)
	h := &testutil.CaptureHandler{}

	if err := executeWithRecorder(context.Background(), opts, slog.New(h), nil); err != nil {
		t.Fatalf("cleanup against live state must succeed with preservation, got: %v", err)
	}

	kubeconfig := filepath.Join(workDir, "cluster-config", "auth", "kubeconfig")
	if _, err := os.Stat(kubeconfig); err != nil {
		t.Errorf("kubeconfig deleted while terraform state has resources (DATA LOSS): %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "downloads")); !os.IsNotExist(err) {
		t.Errorf("non-credential artifacts should still be removed: %v", err)
	}
	if !h.HasLevel(slog.LevelWarn) {
		t.Error("preservation must be announced at Warn level")
	}
}

func TestExecute_ForceCredentialWipeRemovesEverything(t *testing.T) {
	workDir := seedCredentialWorkDir(t)
	projectRoot := t.TempDir()
	seedStateFile(t, projectRoot, `{"version":4,"resources":[{"type":"proxmox_virtual_environment_vm"}]}`)

	opts := credentialGuardOpts(workDir, projectRoot)
	opts.ForceCredentialWipe = true

	if err := executeWithRecorder(context.Background(), opts, logutil.NopLogger, nil); err != nil {
		t.Fatalf("forced wipe errored: %v", err)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Errorf("ForceCredentialWipe must remove the whole work directory: %v", err)
	}
}

func TestExecute_EmptyStateWipesWorkDir(t *testing.T) {
	workDir := seedCredentialWorkDir(t)
	projectRoot := t.TempDir()
	seedStateFile(t, projectRoot, `{"version":4,"resources":[]}`)

	opts := credentialGuardOpts(workDir, projectRoot)
	if err := executeWithRecorder(context.Background(), opts, logutil.NopLogger, nil); err != nil {
		t.Fatalf("cleanup with destroyed state errored: %v", err)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Errorf("empty state means the cluster is gone; workDir must be removed: %v", err)
	}
}

// TestExecute_CorruptStateFailsClosed: corrupt state cannot vouch that the
// cluster is destroyed, so the credential wipe is refused with a diagnostic
// instead of proceeding (or silently succeeding).
func TestExecute_CorruptStateFailsClosed(t *testing.T) {
	workDir := seedCredentialWorkDir(t)
	projectRoot := t.TempDir()
	seedStateFile(t, projectRoot, "not-json")

	opts := credentialGuardOpts(workDir, projectRoot)
	err := executeWithRecorder(context.Background(), opts, logutil.NopLogger, nil)
	if err == nil {
		t.Fatal("cleanup with corrupt terraform state must fail closed")
	}
	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Errorf("err = %v; want *errtypes.ClusterError in chain", err)
	}
	if !strings.Contains(err.Error(), "corrupt") {
		t.Errorf("error must name the corrupt state: %v", err)
	}
	kubeconfig := filepath.Join(workDir, "cluster-config", "auth", "kubeconfig")
	if _, statErr := os.Stat(kubeconfig); statErr != nil {
		t.Errorf("kubeconfig deleted despite corrupt state (DATA LOSS): %v", statErr)
	}
}

package postinstall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

// installFakeTerraformForBootstrap writes a POSIX sh script named "terraform"
// into a temp dir and prepends it to PATH. Behaviour switches on TF_FAKE_MODE:
// "success" — both plan and apply exit 0; "plan-fail" — plan exits 1; and
// "apply-fail" — plan exits 0, apply exits 1.
func installFakeTerraformForBootstrap(t *testing.T) {
	t.Helper()
	script := `#!/bin/sh
case "$1" in
  init) exit 0 ;;
  plan)
    case "${TF_FAKE_MODE:-success}" in
      plan-fail) echo "fake: plan error" >&2; exit 1 ;;
      *)         exit 0 ;;
    esac ;;
  apply)
    case "${TF_FAKE_MODE:-success}" in
      apply-fail) echo "fake: apply error" >&2; exit 1 ;;
      *)          exit 0 ;;
    esac ;;
  *) exit 0 ;;
esac
`
	testutil.InstallFakeBin(t, "terraform", script)
}

func seedBootstrapEnvDir(t *testing.T, projectRoot string) string {
	t.Helper()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production")
	for _, sub := range []string{
		envDir,
		filepath.Join(envDir, ".terraform", "providers"),
	} {
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(envDir, ".terraform.lock.hcl"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	return envDir
}

func newBootstrapPhase(t *testing.T) *Phase {
	t.Helper()
	return &Phase{
		BasePhase: phase.NewBasePhase(
			phase.WithExecutor(executor.New()),
			phase.WithLogger(logutil.NopLogger),
		),
	}
}

func TestCleanupBootstrap_Success(t *testing.T) {
	installFakeTerraformForBootstrap(t)
	t.Setenv("TF_FAKE_MODE", "success")

	projectRoot := t.TempDir()
	envDir := seedBootstrapEnvDir(t, projectRoot)
	planPath := filepath.Join(envDir, "bootstrap-destroy.tfplan")
	if err := os.WriteFile(planPath, []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := newBootstrapPhase(t)
	opts := &Options{
		BaseOptions: phase.BaseOptions{
			ProjectRoot:  projectRoot,
			TerraformEnv: "production",
		},
	}
	cfg := &config.Config{Cluster: config.ClusterConfig{Name: "test-cluster"}}

	if err := p.CleanupBootstrap(context.Background(), cfg, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(planPath); !os.IsNotExist(err) {
		t.Errorf("planPath still exists after success; defer SafeRemove did not fire")
	}
}

func TestCleanupBootstrap_PlanFails(t *testing.T) {
	installFakeTerraformForBootstrap(t)
	t.Setenv("TF_FAKE_MODE", "plan-fail")

	projectRoot := t.TempDir()
	envDir := seedBootstrapEnvDir(t, projectRoot)
	planPath := filepath.Join(envDir, "bootstrap-destroy.tfplan")
	if err := os.WriteFile(planPath, []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := newBootstrapPhase(t)
	opts := &Options{
		BaseOptions: phase.BaseOptions{
			ProjectRoot:  projectRoot,
			TerraformEnv: "production",
		},
	}
	cfg := &config.Config{Cluster: config.ClusterConfig{Name: "test-cluster"}}

	err := p.CleanupBootstrap(context.Background(), cfg, opts)
	if err == nil {
		t.Fatal("expected error when plan fails")
	}
	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Errorf("err = %v; want *errtypes.ClusterError", err)
	}
	if _, statErr := os.Stat(planPath); !os.IsNotExist(statErr) {
		t.Errorf("planPath still exists after plan failure; defer SafeRemove did not fire")
	}
}

func TestCleanupBootstrap_ApplyFails(t *testing.T) {
	installFakeTerraformForBootstrap(t)
	t.Setenv("TF_FAKE_MODE", "apply-fail")

	projectRoot := t.TempDir()
	envDir := seedBootstrapEnvDir(t, projectRoot)
	planPath := filepath.Join(envDir, "bootstrap-destroy.tfplan")
	if err := os.WriteFile(planPath, []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := newBootstrapPhase(t)
	opts := &Options{
		BaseOptions: phase.BaseOptions{
			ProjectRoot:  projectRoot,
			TerraformEnv: "production",
		},
	}
	cfg := &config.Config{Cluster: config.ClusterConfig{Name: "test-cluster"}}

	err := p.CleanupBootstrap(context.Background(), cfg, opts)
	if err == nil {
		t.Fatal("expected error when apply fails")
	}
	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Errorf("err = %v; want *errtypes.ClusterError", err)
	}
	if _, statErr := os.Stat(planPath); !os.IsNotExist(statErr) {
		t.Errorf("planPath still exists after apply failure; defer SafeRemove did not fire")
	}
}

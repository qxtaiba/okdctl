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
	"github.com/qxtaiba/okdctl/internal/testutil"
)

// installFakeTerraformForBootstrap installs a fake terraform gated by TF_FAKE_MODE.
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

func TestCleanupBootstrap(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		wantErr bool
	}{
		{"success", "success", false},
		{"plan fails", "plan-fail", true},
		{"apply fails", "apply-fail", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installFakeTerraformForBootstrap(t)
			t.Setenv("TF_FAKE_MODE", tc.mode)

			projectRoot := t.TempDir()
			envDir := seedBootstrapEnvDir(t, projectRoot)
			planPath := filepath.Join(envDir, "bootstrap-destroy.tfplan")
			if err := os.WriteFile(planPath, []byte("stub"), 0o600); err != nil {
				t.Fatal(err)
			}

			p := newTestPhase(t)
			opts := &Options{
				BaseOptions: phase.BaseOptions{
					ProjectRoot:  projectRoot,
					TerraformEnv: "production",
				},
			}
			cfg := &config.Config{Cluster: config.ClusterConfig{Name: "test-cluster"}}

			err := p.CleanupBootstrap(context.Background(), cfg, opts)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error in mode %s", tc.mode)
				}
				var clusterErr *errtypes.ClusterError
				if !errors.As(err, &clusterErr) {
					t.Errorf("err = %v; want *errtypes.ClusterError", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, statErr := os.Stat(planPath); !os.IsNotExist(statErr) {
				t.Errorf("planPath still exists after %s; defer SafeRemove did not fire", tc.mode)
			}
		})
	}
}

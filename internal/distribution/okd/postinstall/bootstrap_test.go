package postinstall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// installFakeTerraformBackupOrderForBootstrap records, for every terraform
// invocation (not just apply), whether a state backup already exists — so a
// test can assert the backup precedes the FIRST invocation.
func installFakeTerraformBackupOrderForBootstrap(t *testing.T, backupLog string) {
	t.Helper()
	testutil.InstallFakeBin(t, "terraform", `#!/bin/sh
if ls terraform.tfstate.*.bak >/dev/null 2>&1; then
  printf 'present\n' >> "$TF_TEST_BACKUP_LOG"
else
  printf 'absent\n' >> "$TF_TEST_BACKUP_LOG"
fi
exit 0
`)
	t.Setenv("TF_TEST_BACKUP_LOG", backupLog)
}

// seedBootstrapStateOnly seeds a state file but no .terraform/lock
// scaffolding, so Init actually shells out to terraform (unlike
// seedBootstrapEnvDir, whose scaffolding makes Init's already-initialized
// shortcut skip the subprocess entirely).
func seedBootstrapStateOnly(t *testing.T, projectRoot string) {
	t.Helper()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "terraform.tfstate"), []byte(`{"version":4,"resources":[{"type":"x"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
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

func TestCleanupBootstrap_BackupPrecedesFirstTerraformInvocation(t *testing.T) {
	projectRoot := t.TempDir()
	seedBootstrapStateOnly(t, projectRoot)
	backupLog := filepath.Join(t.TempDir(), "backup.log")
	installFakeTerraformBackupOrderForBootstrap(t, backupLog)

	p := newTestPhase(t)
	opts := &Options{BaseOptions: phase.BaseOptions{ProjectRoot: projectRoot, TerraformEnv: "production"}}
	cfg := &config.Config{Cluster: config.ClusterConfig{Name: "test-cluster"}}

	if err := p.CleanupBootstrap(context.Background(), cfg, opts); err != nil {
		t.Fatalf("CleanupBootstrap() = %v; want nil", err)
	}

	data, err := os.ReadFile(backupLog)
	if err != nil {
		t.Fatalf("backup log missing: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("no terraform invocations recorded")
	}
	if lines[0] != "present" {
		t.Errorf("backup status at first terraform invocation = %q; want %q — the state backup must precede init/plan, not just apply", lines[0], "present")
	}
}

package install

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

func installFakeTerraformCapture(t *testing.T, argvLog, snapshotLog string) {
	t.Helper()
	testutil.InstallFakeBin(t, "terraform", `#!/bin/sh
printf '%s\n' "$*" >> "$TF_TEST_ARGV_LOG"
if [ "$1" = "apply" ] && ls terraform.tfstate.*.bak >/dev/null 2>&1; then
  printf 'snapshot-present\n' >> "$TF_TEST_SNAPSHOT_LOG"
fi
exit 0
`)
	t.Setenv("TF_TEST_ARGV_LOG", argvLog)
	t.Setenv("TF_TEST_SNAPSHOT_LOG", snapshotLog)
}

func installFakeTerraformApplyFails(t *testing.T) {
	t.Helper()
	testutil.InstallFakeBin(t, "terraform", `#!/bin/sh
case "$1" in
  apply) echo "fake terraform: apply failed" >&2; exit 1 ;;
  *) exit 0 ;;
esac
`)
}

func installFakeOc(t *testing.T) {
	t.Helper()
	testutil.InstallFakeBin(t, "oc", `#!/bin/sh
if [ -n "$OC_FAKE_FAIL" ]; then
  exit 1
fi
if [ -n "$OC_FAKE_NODES" ]; then
  printf '%s\n' "$OC_FAKE_NODES"
fi
exit 0
`)
}

func seedWorkerTerraformEnvDir(t *testing.T, projectRoot, env string) {
	t.Helper()
	envDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", env)
	for _, sub := range []string{
		envDir,
		filepath.Join(envDir, ".terraform", "providers"),
	} {
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(envDir, "terraform.tfstate"), []byte(`{"version":4,"resources":[{"type":"x"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, ".terraform.lock.hcl"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStartWorkerVMs_TargetsScopedAndSnapshotsBeforeApply(t *testing.T) {
	projectRoot := t.TempDir()
	seedWorkerTerraformEnvDir(t, projectRoot, "production")

	argvLog := filepath.Join(t.TempDir(), "argv.log")
	snapshotLog := filepath.Join(t.TempDir(), "snapshot.log")
	installFakeTerraformCapture(t, argvLog, snapshotLog)

	p := newInstallPhase(t)
	cfg := &config.Config{Topology: config.TopologyConfig{Workers: config.NodeConfig{Count: 2}}}
	opts := &Options{BaseOptions: phase.BaseOptions{ProjectRoot: projectRoot, TerraformEnv: "production"}}

	if err := p.StartWorkerVMs(context.Background(), cfg, opts); err != nil {
		t.Fatalf("StartWorkerVMs() = %v; want nil", err)
	}

	argv, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatalf("read argv log: %v", err)
	}
	var applyLine string
	for _, line := range strings.Split(strings.TrimRight(string(argv), "\n"), "\n") {
		if strings.HasPrefix(line, "apply ") {
			applyLine = line
		}
	}
	if applyLine == "" {
		t.Fatalf("argv log = %q; want a line starting with 'apply'", argv)
	}
	if !strings.Contains(applyLine, "-target=module.okd_cluster.proxmox_virtual_environment_vm.worker") {
		t.Errorf("apply argv = %q; want -target=module.okd_cluster.proxmox_virtual_environment_vm.worker", applyLine)
	}
	if !strings.Contains(applyLine, "start_workers_immediately=true") {
		t.Errorf("apply argv = %q; want start_workers_immediately=true", applyLine)
	}

	if _, err := os.Stat(snapshotLog); err != nil {
		t.Errorf("snapshot log missing; state snapshot was not present when apply ran: %v", err)
	}
}

func TestStartWorkerVMs_NoWorkersSkips(t *testing.T) {
	p := newInstallPhase(t)
	cfg := &config.Config{Topology: config.TopologyConfig{Workers: config.NodeConfig{Count: 0}}}
	opts := &Options{BaseOptions: phase.BaseOptions{ProjectRoot: t.TempDir(), TerraformEnv: "production"}}

	if err := p.StartWorkerVMs(context.Background(), cfg, opts); err != nil {
		t.Fatalf("StartWorkerVMs() = %v; want nil (no workers configured)", err)
	}
}

func TestStartWorkerVMs_ApplyFailureWrapsClusterError(t *testing.T) {
	projectRoot := t.TempDir()
	seedWorkerTerraformEnvDir(t, projectRoot, "production")
	installFakeTerraformApplyFails(t)

	p := newInstallPhase(t)
	cfg := &config.Config{Topology: config.TopologyConfig{Workers: config.NodeConfig{Count: 1}}}
	opts := &Options{BaseOptions: phase.BaseOptions{ProjectRoot: projectRoot, TerraformEnv: "production"}}

	err := p.StartWorkerVMs(context.Background(), cfg, opts)
	if err == nil {
		t.Fatal("expected error when terraform apply fails")
	}
	var clusterErr *errtypes.ClusterError
	if !errors.As(err, &clusterErr) {
		t.Fatalf("err = %v; want *errtypes.ClusterError", err)
	}
	if !strings.Contains(clusterErr.Msg, "state backup") {
		t.Errorf("Msg = %q; want state backup path embedded", clusterErr.Msg)
	}
}

func TestWorkersAlreadyRunning(t *testing.T) {
	tests := []struct {
		name     string
		required int
		fail     bool
		nodes    string
		want     bool
	}{
		{name: "zero workers configured", required: 0, want: true},
		{name: "exact count registered", required: 2, nodes: "worker-0\nworker-1", want: true},
		{name: "fewer than required", required: 3, nodes: "worker-0\nworker-1", want: false},
		{name: "cluster unreachable", required: 2, fail: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installFakeOc(t)
			if tt.fail {
				t.Setenv("OC_FAKE_FAIL", "1")
			}
			if tt.nodes != "" {
				t.Setenv("OC_FAKE_NODES", tt.nodes)
			}

			p := newInstallPhase(t)
			cfg := &config.Config{Topology: config.TopologyConfig{Workers: config.NodeConfig{Count: tt.required}}}

			got, err := p.workersAlreadyRunning(context.Background(), cfg)
			if err != nil {
				t.Fatalf("workersAlreadyRunning() error = %v; want nil", err)
			}
			if got != tt.want {
				t.Errorf("workersAlreadyRunning() = %v; want %v", got, tt.want)
			}
		})
	}
}

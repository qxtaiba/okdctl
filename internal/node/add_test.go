package node

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// fakeISO records BuildCustomISOs/UploadCustomISOsToProxmox calls so a
// dry-run can be proven to trigger neither.
type fakeISO struct {
	buildCalls  int
	uploadCalls int
}

func (f *fakeISO) BuildCustomISOs(context.Context, *config.Config, *setup.Options) error {
	f.buildCalls++
	return nil
}

func (f *fakeISO) UploadCustomISOsToProxmox(context.Context, *config.Config, *setup.Options) error {
	f.uploadCalls++
	return nil
}

// fakeIgnition records ConfigureApache/TeardownIgnitionServer calls so a
// dry-run can be proven to revive/teardown neither.
type fakeIgnition struct {
	configureCalls int
	teardownCalls  int
}

func (f *fakeIgnition) ConfigureApache(context.Context, *config.Config, string) error {
	f.configureCalls++
	return nil
}

func (f *fakeIgnition) TeardownIgnitionServer(context.Context) {
	f.teardownCalls++
}

const addTestClusterName = "mycluster"

func addTestConfig(workerCount, workerMemMB int) *config.Config {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = addTestClusterName
	cfg.Topology.Workers.Count = workerCount
	cfg.Topology.Workers.MemoryMB = workerMemMB
	cfg.Provider.Proxmox = &config.ProxmoxConfig{ISOStorage: "iso-store"}
	return cfg
}

// seedAddRunner builds a Runner rooted at a fresh temp dir, with sentinel
// tfvars/config files so a dry-run's zero-mutation contract can be checked,
// plus the fake ISO/ignition collaborators wired so their call counts prove
// (or disprove) mutation.
func seedAddRunner(t *testing.T, fc *fakeCluster, ftf *fakeTF, fiso *fakeISO, fign *fakeIgnition, cfg *config.Config) (r *Runner, tfvarsPath, cfgPath string) {
	t.Helper()
	dir := t.TempDir()
	tfvarsPath = filepath.Join(dir, "terraform.tfvars")
	cfgPath = filepath.Join(dir, "okdctl.yaml")
	if err := os.WriteFile(tfvarsPath, []byte("SENTINEL_TFVARS\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("SENTINEL_CONFIG\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r = &Runner{
		Cluster:     fc,
		TF:          ftf,
		ISO:         fiso,
		Ignition:    fign,
		Cfg:         cfg,
		ConfigPath:  cfgPath,
		ProjectRoot: dir,
		WorkDir:     filepath.Join(dir, "okd-install"),
		EnvDir:      dir,
		RunID:       "test-run",
		DryRun:      true,
		Log:         logutil.NopLogger,
	}
	return r, tfvarsPath, cfgPath
}

// writeIgnitionArtifacts creates worker.ign and the ignition TLS cert under
// r's WorkDir/ProjectRoot so preflightIgnitionArtifacts passes.
func writeIgnitionArtifacts(t *testing.T, r *Runner) {
	t.Helper()
	clusterDir := phase.ClusterConfigDir(r.WorkDir)
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clusterDir, "worker.ign"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	certPath, _ := setup.IgnitionCertPaths(r.ProjectRoot)
	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, []byte("cert"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func addWorkerNodes(n int) []cluster.NodeDetail {
	nodes := make([]cluster.NodeDetail, n)
	for i := range nodes {
		nodes[i] = cluster.NodeDetail{Name: "worker-placeholder", Role: nodetypes.RoleWorker, Ready: true}
	}
	return nodes
}

func TestAddWorkersDryRunMakesNoMutation(t *testing.T) {
	fc := &fakeCluster{nodes: addWorkerNodes(2)}
	ftf := &fakeTF{action: terraform.PlanActionCreate}
	fiso := &fakeISO{}
	fign := &fakeIgnition{}
	cfg := addTestConfig(2, 16384)

	r, tfvars, cfgPath := seedAddRunner(t, fc, ftf, fiso, fign, cfg)
	writeIgnitionArtifacts(t, r)

	if err := r.AddWorkers(context.Background(), AddOptions{Count: 2}); err != nil {
		t.Fatalf("dry-run add: %v", err)
	}

	if fc.cordon != 0 || fc.drain != 0 || fc.deleteNode != 0 {
		t.Errorf("dry-run add mutated the cluster: cordon=%d drain=%d deleteNode=%d", fc.cordon, fc.drain, fc.deleteNode)
	}
	if ftf.applyCalls != 0 || ftf.snapshots != 0 {
		t.Errorf("dry-run add applied terraform: apply=%d snapshot=%d", ftf.applyCalls, ftf.snapshots)
	}
	if fiso.buildCalls != 0 || fiso.uploadCalls != 0 {
		t.Errorf("dry-run add built/uploaded an iso: build=%d upload=%d", fiso.buildCalls, fiso.uploadCalls)
	}
	if fign.configureCalls != 0 || fign.teardownCalls != 0 {
		t.Errorf("dry-run add touched the ignition server: configure=%d teardown=%d", fign.configureCalls, fign.teardownCalls)
	}
	if ftf.planCalls != 2 {
		t.Errorf("dry-run add must plan-gate every new node: planCalls=%d want 2", ftf.planCalls)
	}
	// #C3 fidelity: the preview must widen worker_count AND worker_isos
	// together to the batch's final total (startIdx 2 + count 2 = 4), or the
	// module's length(worker_isos) >= worker_count assertion trips the plan.
	if ftf.lastVars["worker_count"] != "4" {
		t.Errorf("dry-run add plan did not widen worker_count: vars=%v", ftf.lastVars)
	}
	wantISOs := `["iso-store:iso/worker0.iso", "iso-store:iso/worker1.iso", "iso-store:iso/worker2.iso", "iso-store:iso/worker3.iso"]`
	if ftf.lastVars["worker_isos"] != wantISOs {
		t.Errorf("dry-run add plan did not widen worker_isos in lockstep: got %q want %q", ftf.lastVars["worker_isos"], wantISOs)
	}
	assertUnchanged(t, tfvars, "SENTINEL_TFVARS\n")
	assertUnchanged(t, cfgPath, "SENTINEL_CONFIG\n")
	if _, err := os.Stat(filepath.Join(r.WorkDir, OpMarkerFileName)); !os.IsNotExist(err) {
		t.Error("dry-run add wrote an op marker")
	}
}

func TestAddWorkersMemoryBudgetRejected(t *testing.T) {
	fc := &fakeCluster{nodes: addWorkerNodes(2)}
	ftf := &fakeTF{action: terraform.PlanActionCreate}
	fiso := &fakeISO{}
	fign := &fakeIgnition{}
	cfg := addTestConfig(2, 40960)

	r, _, _ := seedAddRunner(t, fc, ftf, fiso, fign, cfg)
	writeIgnitionArtifacts(t, r)

	// Host nearly full: adding 2 more 40 GiB workers must be refused.
	err := r.AddWorkers(context.Background(), AddOptions{
		Count:            2,
		HostTotalMiB:     96000,
		HostAllocatedMiB: 93000,
	})
	if err == nil {
		t.Fatal("want memory-budget refusal when the probe reports the host is full")
	}
	if ftf.planCalls != 0 {
		t.Errorf("budget refusal must precede any plan; planCalls=%d", ftf.planCalls)
	}
	if fiso.buildCalls != 0 || fign.configureCalls != 0 {
		t.Errorf("budget refusal must precede any mutation: build=%d configure=%d", fiso.buildCalls, fign.configureCalls)
	}
}

func TestAddWorkersPreflightArtifactMissingRejected(t *testing.T) {
	fc := &fakeCluster{nodes: addWorkerNodes(1)}
	ftf := &fakeTF{action: terraform.PlanActionCreate}
	fiso := &fakeISO{}
	fign := &fakeIgnition{}
	cfg := addTestConfig(1, 16384)

	// No writeIgnitionArtifacts call: worker.ign and the TLS cert are absent,
	// modelling `cleanup --kind web-only` having removed them.
	r, _, _ := seedAddRunner(t, fc, ftf, fiso, fign, cfg)

	err := r.AddWorkers(context.Background(), AddOptions{Count: 1})
	if err == nil {
		t.Fatal("want preflight refusal when worker.ign/tls cert are missing")
	}
	if fc.listNodesCalls != 0 {
		t.Errorf("preflight must precede ListNodes; listNodesCalls=%d", fc.listNodesCalls)
	}
	if ftf.planCalls != 0 || fiso.buildCalls != 0 {
		t.Errorf("preflight refusal must precede any plan or mutation: planCalls=%d build=%d", ftf.planCalls, fiso.buildCalls)
	}
}

func TestAddWorkersPlanShape(t *testing.T) {
	cases := []struct {
		name        string
		startCount  int
		addCount    int
		wantIndices []int
	}{
		{name: "count 1", startCount: 2, addCount: 1, wantIndices: []int{2}},
		{name: "count 3", startCount: 2, addCount: 3, wantIndices: []int{2, 3, 4}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			fc := &fakeCluster{nodes: addWorkerNodes(tt.startCount)}
			ftf := &fakeTF{action: terraform.PlanActionCreate}
			fiso := &fakeISO{}
			fign := &fakeIgnition{}
			cfg := addTestConfig(tt.startCount, 16384)

			r, _, _ := seedAddRunner(t, fc, ftf, fiso, fign, cfg)
			writeIgnitionArtifacts(t, r)

			var captured *OpPlan
			r.Preview = func(p *OpPlan) { captured = p }

			if err := r.AddWorkers(context.Background(), AddOptions{Count: tt.addCount}); err != nil {
				t.Fatalf("dry-run add: %v", err)
			}
			if captured == nil {
				t.Fatal("preview was never called")
			}
			if len(captured.Nodes) != len(tt.wantIndices) {
				t.Fatalf("got %d plan nodes, want %d", len(captured.Nodes), len(tt.wantIndices))
			}
			for i, idx := range tt.wantIndices {
				n := captured.Nodes[i]
				if n.Name != fmt.Sprintf("mycluster-worker%d", idx) {
					t.Errorf("node %d name = %q, want %q", i, n.Name, fmt.Sprintf("mycluster-worker%d", idx))
				}
				if n.TFAddress != workerAddress(idx) {
					t.Errorf("node %d TFAddress = %q, want %q", i, n.TFAddress, workerAddress(idx))
				}
				if n.Action != terraform.PlanActionCreate {
					t.Errorf("node %d Action = %q, want %q", i, n.Action, terraform.PlanActionCreate)
				}
				if n.Role != nodetypes.RoleWorker {
					t.Errorf("node %d Role = %q, want %q", i, n.Role, nodetypes.RoleWorker)
				}
			}
			if captured.DestroysData() {
				t.Error("an add plan must never report DestroysData")
			}
		})
	}
}

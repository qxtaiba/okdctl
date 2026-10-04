package node

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

var errInterruptedAfterApply = errors.New("interrupted after external apply")

type recoveryTF struct {
	fakeTF
	state          map[string]map[string]string
	failAfterApply bool
	onApply        func()
}

func (f *recoveryTF) ShowPlanChanges(context.Context, string) ([]terraform.ResourceChange, error) {
	current, exists := f.state[f.lastTarget]
	if (f.action == terraform.PlanActionDelete && !exists) ||
		(f.action == terraform.PlanActionCreate && exists) ||
		(f.action == terraform.PlanActionUpdate && exists && maps.Equal(current, f.lastVars)) {
		return nil, nil
	}
	return []terraform.ResourceChange{{Address: f.lastTarget, Action: f.action}}, nil
}

func (f *recoveryTF) StateHasResource(_ context.Context, address string) (bool, error) {
	_, exists := f.state[address]
	return exists, nil
}

func (f *recoveryTF) Apply(context.Context, terraform.ApplyOptions) error {
	f.applyCalls++
	if f.action == terraform.PlanActionDelete {
		delete(f.state, f.lastTarget)
	} else {
		f.state[f.lastTarget] = maps.Clone(f.lastVars)
	}
	if f.onApply != nil {
		f.onApply()
	}
	if f.failAfterApply {
		f.failAfterApply = false
		return errInterruptedAfterApply
	}
	return nil
}

func reloadRecoveryRunner(t *testing.T, old *Runner) *Runner {
	t.Helper()
	cfg, err := config.NewLoader().LoadFile(old.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	return &Runner{
		Cluster: old.Cluster, TF: old.TF, Power: old.Power, Disk: old.Disk, ISO: old.ISO, Ignition: old.Ignition,
		Cfg: cfg, ConfigPath: old.ConfigPath, projectRoot: old.projectRoot, workDir: old.workDir, envDir: old.envDir,
		RunID: "second-run", Log: logutil.NopLogger, NodeReadyTimeout: old.NodeReadyTimeout,
		EtcdGateTimeout: old.EtcdGateTimeout, CephGateTimeout: old.CephGateTimeout,
	}
}

func saveRecoveryConfig(t *testing.T, r *Runner) {
	t.Helper()
	r.DryRun = false
	if err := config.NewLoader().Save(r.Cfg, r.ConfigPath); err != nil {
		t.Fatal(err)
	}
}

func assertRecoveryComplete(t *testing.T, r *Runner, tfvars string) {
	t.Helper()
	if m, err := ReadOpMarker(r.workDir, r.Cfg.Cluster.Name); err != nil || m != nil {
		t.Fatalf("completion marker remains: %+v %v", m, err)
	}
	saved := reloadRecoveryRunner(t, r)
	if saved.Cfg.Topology.Workers != r.Cfg.Topology.Workers {
		t.Fatalf("saved sizing differs: %+v != %+v", saved.Cfg.Topology.Workers, r.Cfg.Topology.Workers)
	}
	body, err := os.ReadFile(tfvars)
	if err != nil || strings.Contains(string(body), "SENTINEL") {
		t.Fatalf("tfvars not reconciled: %v", err)
	}
	for key, value := range map[string]int{"worker_count": r.Cfg.Topology.Workers.Count, "worker_memory_mb": r.Cfg.Topology.Workers.MemoryMB} {
		pattern := fmt.Sprintf(`(?m)^%s\s*=\s*%d\s*$`, key, value)
		if !regexp.MustCompile(pattern).Match(body) {
			t.Fatalf("tfvars missing %s = %d", key, value)
		}
	}
}

func TestResizeTwoRunsAfterAppliedMutation(t *testing.T) {
	for _, boundary := range []string{"apply", "tfvars", "persisted"} {
		t.Run(boundary, func(t *testing.T) {
			fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
			cfg := config.DefaultConfig()
			cfg.Topology.Workers.Count = 1
			cfg.Topology.Workers.MemoryMB = 8192
			cfg.Provider.Proxmox.Node = testProxmoxNode
			r, tfvars, _ := seedRunner(t, fc, &fakeTF{}, cfg)
			state := &recoveryTF{fakeTF: fakeTF{action: terraform.PlanActionUpdate}, state: map[string]map[string]string{workerAddress(0): {"worker_memory_mb": "8192"}}, failAfterApply: true}
			r.TF = state
			r.Power = &fakePower{}
			saveRecoveryConfig(t, r)
			repair := recoveryBoundary(t, r, state, tfvars, boundary)
			scope, opts := ResizeScope{Role: nodetypes.RoleWorker}, ResizeOptions{MemoryMB: 16384}
			if err := r.Resize(t.Context(), scope, opts); err == nil {
				t.Fatalf("first run: %v", err)
			}
			repair()
			next := reloadRecoveryRunner(t, r)
			for _, changed := range []ResizeOptions{{MemoryMB: 24576}, {MemoryMB: 16384, CPU: 96}, {MemoryMB: 16384, SkipDrain: true}} {
				if err := next.Resize(t.Context(), scope, changed); err == nil {
					t.Fatalf("changed request resumed: %+v", changed)
				}
			}
			if err := next.Resize(t.Context(), ResizeScope{Node: "worker0"}, opts); err == nil {
				t.Fatal("changed scope inherited checkpoint")
			}
			assertResumePreviewReadOnly(t, next, tfvars, func() error { return next.Resize(t.Context(), scope, opts) })
			if err := next.Resize(t.Context(), scope, opts); err != nil {
				t.Fatalf("second run: %v", err)
			}
			if state.applyCalls != 1 || next.Cfg.Topology.Workers.MemoryMB != 16384 || fc.cordonState["worker0"] || next.Power.(*fakePower).calls != 1 {
				t.Fatal("resize did not converge without repeating apply")
			}
			assertRecoveryComplete(t, next, tfvars)
		})
	}
}

func TestRemoveTwoRunsAfterAppliedMutation(t *testing.T) {
	for _, boundary := range []string{"apply", "tfvars", "persisted"} {
		t.Run(boundary, func(t *testing.T) {
			fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}, {Name: "master0", Role: nodetypes.RoleMaster, Ready: true}}, schedulable: true}
			cfg := config.DefaultConfig()
			cfg.Topology.Workers.Count = 1
			r, tfvars, _ := seedRunner(t, fc, &fakeTF{}, cfg)
			state := &recoveryTF{fakeTF: fakeTF{action: terraform.PlanActionDelete}, state: map[string]map[string]string{workerAddress(0): {}}, failAfterApply: true}
			r.TF = state
			saveRecoveryConfig(t, r)
			repair := recoveryBoundary(t, r, state, tfvars, boundary)
			if err := r.RemoveWorker(t.Context(), "worker0", RemoveOptions{}); err == nil {
				t.Fatalf("first run: %v", err)
			}
			repair()
			next := reloadRecoveryRunner(t, r)
			assertResumePreviewReadOnly(t, next, tfvars, func() error { return next.RemoveWorker(t.Context(), "worker0", RemoveOptions{}) })
			if err := next.RemoveWorker(t.Context(), "worker0", RemoveOptions{}); err != nil {
				t.Fatalf("second run: %v", err)
			}
			if state.applyCalls != 1 || next.Cfg.Topology.Workers.Count != 0 || fc.deleteNode != 1 || len(state.state) != 0 || len(fc.nodes) != 1 {
				t.Fatal("remove did not converge")
			}
			assertRecoveryComplete(t, next, tfvars)
		})
	}
}

func TestAddTwoRunsAfterAppliedMutation(t *testing.T) {
	for _, boundary := range []string{"apply", "tfvars", "persisted"} {
		t.Run(boundary, func(t *testing.T) {
			fc := &fakeCluster{}
			h := seedAddTest(t, fc, addTestConfig(0, 8192))
			r := h.r
			writeIgnitionArtifacts(t, r)
			state := &recoveryTF{fakeTF: fakeTF{action: terraform.PlanActionCreate}, state: map[string]map[string]string{}, failAfterApply: true}
			state.onApply = func() {
				fc.nodes = []cluster.NodeDetail{{Name: "mycluster-worker0", Role: nodetypes.RoleWorker, Ready: true}}
			}
			r.TF = state
			saveRecoveryConfig(t, r)
			repair := recoveryBoundary(t, r, state, h.tfvars, boundary)
			if err := r.AddWorkers(t.Context(), AddOptions{Count: 1}); err == nil {
				t.Fatalf("first run: %v", err)
			}
			repair()
			next := reloadRecoveryRunner(t, r)
			if err := next.AddWorkers(t.Context(), AddOptions{Count: 2}); err == nil {
				t.Fatal("changed batch inherited checkpoint")
			}
			assertResumePreviewReadOnly(t, next, h.tfvars, func() error { return next.AddWorkers(t.Context(), AddOptions{Count: 1}) })
			if err := next.AddWorkers(t.Context(), AddOptions{Count: 1}); err != nil {
				t.Fatalf("second run: %v", err)
			}
			if state.applyCalls != 1 || next.Cfg.Topology.Workers.Count != 1 || len(state.state) != 1 || len(fc.nodes) != 1 {
				t.Fatal("add did not converge")
			}
			assertRecoveryComplete(t, next, h.tfvars)
		})
	}
}

func recoveryBoundary(t *testing.T, r *Runner, state *recoveryTF, tfvars, boundary string) func() {
	t.Helper()
	state.failAfterApply = boundary == "apply"
	switch boundary {
	case "tfvars":
		applied := state.onApply
		state.onApply = func() {
			if applied != nil {
				applied()
			}
			if err := os.Remove(tfvars); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(tfvars, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		return func() {
			if err := os.Remove(tfvars); err != nil {
				t.Fatal(err)
			}
		}
	case "persisted":
		r.afterPersist = func() error { return errInterruptedAfterApply }
	}
	return func() {}
}

type interruptedIgnition struct{ fakeIgnition }

func (*interruptedIgnition) ReviveIgnitionServer(context.Context, *config.Config, string, string) error {
	return errors.New("ignition unavailable during resume")
}

func TestAddInterruptedAgainDuringResume(t *testing.T) {
	fc := &fakeCluster{}
	h := seedAddTest(t, fc, addTestConfig(0, 8192))
	r := h.r
	writeIgnitionArtifacts(t, r)
	state := &recoveryTF{fakeTF: fakeTF{action: terraform.PlanActionCreate}, state: map[string]map[string]string{}, failAfterApply: true}
	state.onApply = func() {
		fc.nodes = []cluster.NodeDetail{{Name: "mycluster-worker0", Role: nodetypes.RoleWorker, Ready: true}}
	}
	r.TF = state
	saveRecoveryConfig(t, r)
	opts := AddOptions{Count: 1}
	if err := r.AddWorkers(t.Context(), opts); err == nil {
		t.Fatal("first interruption missing")
	}
	before, err := ReadOpMarker(r.workDir, r.Cfg.Cluster.Name)
	if err != nil || before == nil {
		t.Fatalf("checkpoint missing: %v", err)
	}
	next := reloadRecoveryRunner(t, r)
	next.Ignition = &interruptedIgnition{}
	if err := next.AddWorkers(t.Context(), opts); err == nil {
		t.Fatal("second interruption missing")
	}
	after, err := ReadOpMarker(r.workDir, r.Cfg.Cluster.Name)
	if err != nil || after == nil || after.Step != before.Step {
		t.Fatalf("resume rewound checkpoint: %+v -> %+v (%v)", before, after, err)
	}
	last := reloadRecoveryRunner(t, r)
	if err := last.AddWorkers(t.Context(), opts); err != nil {
		t.Fatal(err)
	}
	if state.applyCalls != 1 {
		t.Fatal("resumed operation repeated apply")
	}
	assertRecoveryComplete(t, last, h.tfvars)
}

func assertResumePreviewReadOnly(t *testing.T, r *Runner, tfvars string, preview func() error) {
	t.Helper()
	before := make(map[string][]byte)
	for _, path := range []string{r.marker(), r.ConfigPath, tfvars} {
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		before[path] = data
	}
	state := r.TF.(*recoveryTF)
	calls := state.applyCalls
	r.DryRun, r.ResumePreview = true, true
	err := preview()
	r.DryRun, r.ResumePreview = false, false
	if err != nil {
		t.Fatalf("resume preview: %v", err)
	}
	if state.applyCalls != calls {
		t.Fatal("preview applied Terraform")
	}
	for path, data := range before {
		after, err := os.ReadFile(path)
		if (err != nil && !os.IsNotExist(err)) || !bytes.Equal(data, after) {
			t.Fatalf("preview changed %s: %v", path, err)
		}
	}
}

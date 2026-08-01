package node

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// TestTargetedApplyRefusesReplaceWhenUpdateWanted locks the plan gate's core
// guarantee: a plan that would REPLACE (destroy-and-recreate) the VM when an
// in-place update was intended is refused before snapshot and apply. For a
// master, a replace wipes the etcd member's disk and breaks quorum.
func TestTargetedApplyRefusesReplaceWhenUpdateWanted(t *testing.T) {
	fc := &fakeCluster{}
	ftf := &fakeTF{action: terraform.PlanActionReplace}
	cfg := config.DefaultConfig()

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false

	err := r.targetedApply(context.Background(), masterAddress(0), terraform.PlanActionUpdate, nil, false)
	if err == nil {
		t.Fatal("want gate refusal when the plan is a replace but an update was intended")
	}
	if !strings.Contains(err.Error(), "plan safety gate refused") {
		t.Errorf("refusal should name the gate: %v", err)
	}
	if ftf.applyCalls != 0 || ftf.snapshots != 0 {
		t.Errorf("gate refusal must precede snapshot and apply: apply=%d snapshot=%d", ftf.applyCalls, ftf.snapshots)
	}
	if ftf.stateCalls != 0 {
		t.Errorf("a non-empty refused plan must not probe state: stateCalls=%d", ftf.stateCalls)
	}
}

func TestTargetedApplyRefusesDeleteWhenUpdateWanted(t *testing.T) {
	fc := &fakeCluster{}
	ftf := &fakeTF{action: terraform.PlanActionDelete}
	cfg := config.DefaultConfig()

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false

	err := r.targetedApply(context.Background(), masterAddress(0), terraform.PlanActionUpdate, nil, false)
	if err == nil {
		t.Fatal("want gate refusal when the plan deletes the resource an update was intended for")
	}
	if ftf.applyCalls != 0 || ftf.snapshots != 0 {
		t.Errorf("gate refusal must precede snapshot and apply: apply=%d snapshot=%d", ftf.applyCalls, ftf.snapshots)
	}
}

// TestResizeMasterReplacePlanRefusedBeforeApply drives the full Resize flow
// against a plan that folds to a replace. The gate must abort at the terraform
// step: no apply, no state snapshot consumed by an apply, no power-cycle, and
// no sizing persisted to config/tfvars — while the op marker survives so the
// operator can resume after fixing the module.
func TestResizeMasterReplacePlanRefusedBeforeApply(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster, Ready: true}},
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionReplace}
	fp := &fakePower{}
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.MemoryMB = 12288

	r, tfvars, cfgPath := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = fp

	err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{MemoryMB: 24576})
	if err == nil {
		t.Fatal("want gate refusal when the resize plan is a replace")
	}
	if !strings.Contains(err.Error(), "plan safety gate refused") {
		t.Errorf("refusal should name the gate: %v", err)
	}
	if ftf.applyCalls != 0 {
		t.Errorf("replace plan must never be applied: apply=%d", ftf.applyCalls)
	}
	if fp.calls != 0 {
		t.Errorf("no power-cycle without a landed apply: calls=%d", fp.calls)
	}
	if cfg.Topology.ControlPlane.MemoryMB != 12288 {
		t.Errorf("failed resize must not persist sizing: MemoryMB=%d", cfg.Topology.ControlPlane.MemoryMB)
	}
	assertUnchanged(t, tfvars, "SENTINEL_TFVARS\n")
	assertUnchanged(t, cfgPath, "SENTINEL_CONFIG\n")
	if _, statErr := os.Stat(filepath.Join(r.workDir, OpMarkerFileName)); statErr != nil {
		t.Errorf("op marker must survive the refusal for resume: %v", statErr)
	}
}

// TestResizeDryRunSurfacesReplaceRefusal ensures the dry-run preview is
// truthful: when the targeted plan folds to a replace, --dry-run reports the
// gate refusal instead of a green preview — with zero cluster mutation.
func TestResizeDryRunSurfacesReplaceRefusal(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster, Ready: true}},
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionReplace}
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.MemoryMB = 12288

	r, tfvars, cfgPath := seedRunner(t, fc, ftf, cfg)

	err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{MemoryMB: 24576})
	if err == nil {
		t.Fatal("dry-run must surface the replace refusal, not preview it as safe")
	}
	if fc.cordon != 0 || fc.drain != 0 || ftf.applyCalls != 0 || ftf.snapshots != 0 {
		t.Errorf("dry-run refusal mutated something: cordon=%d drain=%d apply=%d snapshot=%d",
			fc.cordon, fc.drain, ftf.applyCalls, ftf.snapshots)
	}
	assertUnchanged(t, tfvars, "SENTINEL_TFVARS\n")
	assertUnchanged(t, cfgPath, "SENTINEL_CONFIG\n")
}

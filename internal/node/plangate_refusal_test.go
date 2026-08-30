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

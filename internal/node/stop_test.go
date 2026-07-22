package node

import (
	"context"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func stopTestNodes() []cluster.NodeDetail {
	return []cluster.NodeDetail{
		{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "worker1", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "master0", Role: nodetypes.RoleMaster, Ready: true},
		{Name: "master1", Role: nodetypes.RoleMaster, Ready: true},
	}
}

func TestStopDryRunMakesNoMutation(t *testing.T) {
	fc := &fakeCluster{
		nodes:          stopTestNodes(),
		signerNotAfter: time.Now().Add(60 * 24 * time.Hour),
	}
	ftf := &fakeTF{}
	fp := &fakePower{}
	cfg := config.DefaultConfig()

	r, tfvars, cfgPath := seedRunner(t, fc, ftf, cfg)

	if err := r.Stop(context.Background()); err != nil {
		t.Fatalf("dry-run stop: %v", err)
	}

	if fc.cordon != 0 || fc.uncordon != 0 || fc.drain != 0 {
		t.Errorf("dry-run stop mutated the cluster: cordon=%d uncordon=%d drain=%d", fc.cordon, fc.uncordon, fc.drain)
	}
	if fp.shutdownCalls != 0 || fp.calls != 0 {
		t.Errorf("dry-run stop powered off a vm: shutdownCalls=%d powerCycleCalls=%d", fp.shutdownCalls, fp.calls)
	}
	assertUnchanged(t, tfvars, "SENTINEL_TFVARS\n")
	assertUnchanged(t, cfgPath, "SENTINEL_CONFIG\n")
}

func TestStopDryRunDoesNotRequirePower(t *testing.T) {
	fc := &fakeCluster{
		nodes:          stopTestNodes(),
		signerNotAfter: time.Now().Add(60 * 24 * time.Hour),
	}
	ftf := &fakeTF{}
	cfg := config.DefaultConfig()

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.Power = nil

	if err := r.Stop(context.Background()); err != nil {
		t.Fatalf("dry-run stop without a power-cycler must not fail: %v", err)
	}
}

func TestStopRefusesWithoutPowerCycler(t *testing.T) {
	fc := &fakeCluster{
		nodes:          stopTestNodes(),
		signerNotAfter: time.Now().Add(60 * 24 * time.Hour),
	}
	ftf := &fakeTF{}
	cfg := config.DefaultConfig()

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = nil

	err := r.Stop(context.Background())
	if err == nil {
		t.Fatal("expected refusal when no power-cycler is wired")
	}
	if fc.cordon != 0 {
		t.Errorf("refusal must precede any disruption: cordon=%d", fc.cordon)
	}
}

func TestStopShutsWorkersBeforeMasters(t *testing.T) {
	fc := &fakeCluster{
		nodes:          stopTestNodes(),
		signerNotAfter: time.Now().Add(60 * 24 * time.Hour),
	}
	ftf := &fakeTF{}
	fp := &fakePower{}
	cfg := config.DefaultConfig()
	cfg.Topology.VMIDBase = 6000
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = fp

	if err := r.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}

	if fc.cordon != 4 {
		t.Errorf("expected all 4 nodes cordoned, got %d", fc.cordon)
	}
	if fc.uncordon != 0 {
		t.Errorf("stop must never uncordon: uncordon=%d", fc.uncordon)
	}
	want := []int{6100, 6101, 6010, 6011}
	if len(fp.shutdownOrder) != len(want) {
		t.Fatalf("shutdownOrder = %v, want %v", fp.shutdownOrder, want)
	}
	for i, vmid := range want {
		if fp.shutdownOrder[i] != vmid {
			t.Errorf("shutdownOrder[%d] = %d, want %d (workers ascending then masters ascending): full order %v",
				i, fp.shutdownOrder[i], vmid, fp.shutdownOrder)
		}
	}
}

func TestStopLeavesCordonedOnShutdownFailure(t *testing.T) {
	fc := &fakeCluster{
		nodes:          stopTestNodes(),
		signerNotAfter: time.Now().Add(60 * 24 * time.Hour),
	}
	ftf := &fakeTF{}
	fp := &fakePower{shutdownFailsAtCall: 1}
	cfg := config.DefaultConfig()
	cfg.Topology.VMIDBase = 6000
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = fp

	err := r.Stop(context.Background())
	if err == nil {
		t.Fatal("expected error when the first shutdown fails")
	}
	if fc.cordon != 4 {
		t.Errorf("every node must be cordoned before shutdown begins: cordon=%d", fc.cordon)
	}
	if fc.uncordon != 0 {
		t.Errorf("node must be left cordoned on shutdown failure; uncordon=%d", fc.uncordon)
	}
	if fp.shutdownCalls != 1 {
		t.Errorf("expected exactly one shutdown attempt before the failure short-circuits, got %d", fp.shutdownCalls)
	}
}

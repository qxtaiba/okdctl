package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
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

// stopTestCluster sets a signer expiry far enough out that the certificate-window guard passes.
func stopTestCluster() *fakeCluster {
	return &fakeCluster{
		nodes:          stopTestNodes(),
		signerNotAfter: time.Now().Add(60 * 24 * time.Hour),
	}
}

func TestStopDryRunMakesNoMutation(t *testing.T) {
	fc := stopTestCluster()
	ftf := &fakeTF{}
	fp := &fakePower{}
	cfg := config.DefaultConfig()

	r, tfvars, cfgPath := seedRunner(t, fc, ftf, cfg)

	if err := r.Stop(context.Background(), StopOptions{}); err != nil {
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
	r, _, _ := seedRunner(t, stopTestCluster(), &fakeTF{}, config.DefaultConfig())
	r.Power = nil

	if err := r.Stop(context.Background(), StopOptions{}); err != nil {
		t.Fatalf("dry-run stop without a power-cycler must not fail: %v", err)
	}
}

func TestStopRefusesWithoutPowerCycler(t *testing.T) {
	fc := stopTestCluster()
	r, _, _ := seedRunner(t, fc, &fakeTF{}, config.DefaultConfig())
	r.DryRun = false
	r.Power = nil

	err := r.Stop(context.Background(), StopOptions{})
	if err == nil {
		t.Fatal("expected refusal when no power-cycler is wired")
	}
	if fc.cordon != 0 {
		t.Errorf("refusal must precede any disruption: cordon=%d", fc.cordon)
	}
}

func TestStopShutsWorkersBeforeMasters(t *testing.T) {
	fc := stopTestCluster()
	fp := &fakePower{}
	cfg := config.DefaultConfig()
	cfg.Topology.VMIDBase = 6000
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r, _, _ := seedRunner(t, fc, &fakeTF{}, cfg)
	r.DryRun = false
	r.Power = fp

	if err := r.Stop(context.Background(), StopOptions{}); err != nil {
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
	fc := stopTestCluster()
	fp := &fakePower{shutdownFailsAtCall: 1}
	cfg := config.DefaultConfig()
	cfg.Topology.VMIDBase = 6000
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r, _, _ := seedRunner(t, fc, &fakeTF{}, cfg)
	r.DryRun = false
	r.Power = fp

	err := r.Stop(context.Background(), StopOptions{})
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

func TestStopRefusesForeignMarkerWithoutAck(t *testing.T) {
	fc := stopTestCluster()
	fp := &fakePower{}
	cfg := config.DefaultConfig()
	cfg.Topology.VMIDBase = 6000
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r, _, _ := seedRunner(t, fc, &fakeTF{}, cfg)
	r.DryRun = false
	r.Power = fp
	seedMarker(t, r, OpRemove, "worker5", StepDrain)

	err := r.Stop(context.Background(), StopOptions{})
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("want *errtypes.ConfigError refusing the foreign marker, got %v", err)
	}
	if fc.cordon != 0 || fp.shutdownCalls != 0 {
		t.Errorf("refused stop must make zero mutation: cordon=%d shutdown=%d", fc.cordon, fp.shutdownCalls)
	}

	if err := r.Stop(context.Background(), StopOptions{Acknowledge: true}); err != nil {
		t.Fatalf("acknowledged stop must proceed fresh: %v", err)
	}
	if fc.cordon != 4 || fp.shutdownCalls != 4 {
		t.Errorf("acknowledged stop should run the full sequence: cordon=%d shutdown=%d", fc.cordon, fp.shutdownCalls)
	}
}

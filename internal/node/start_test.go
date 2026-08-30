package node

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func startTestNodes() []cluster.NodeDetail {
	return []cluster.NodeDetail{
		{Name: "master0", Role: nodetypes.RoleMaster, Ready: true},
		{Name: "master1", Role: nodetypes.RoleMaster, Ready: true},
		{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "worker1", Role: nodetypes.RoleWorker, Ready: true},
	}
}

func startTestConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.Count = 2
	cfg.Topology.Workers.Count = 2
	cfg.Topology.VMIDBase = 6000
	cfg.Provider.Proxmox.Node = testProxmoxNode
	return cfg
}

// seedStartRunner builds a non-dry-run Runner with a short cluster-ready
// timeout; the never-converges tests tighten it further.
func seedStartRunner(t *testing.T, fc *fakeCluster, fp *fakePower) *Runner {
	t.Helper()
	r, _, _ := seedRunner(t, fc, &fakeTF{}, startTestConfig())
	r.DryRun = false
	r.Power = fp
	r.ClusterReadyTimeout = 5 * time.Second
	return r
}

func TestStartDryRunMakesNoMutation(t *testing.T) {
	fc := &fakeCluster{nodes: startTestNodes()}
	ftf := &fakeTF{}
	fp := &fakePower{}
	cfg := startTestConfig()

	r, tfvars, cfgPath := seedRunner(t, fc, ftf, cfg)
	r.Power = fp

	if err := r.Start(context.Background(), StartOptions{}); err != nil {
		t.Fatalf("dry-run start: %v", err)
	}

	if fp.startCalls != 0 || fp.calls != 0 || fp.shutdownCalls != 0 {
		t.Errorf("dry-run start touched the hypervisor: start=%d powerCycle=%d shutdown=%d", fp.startCalls, fp.calls, fp.shutdownCalls)
	}
	if fc.listNodesCalls != 0 || fc.cordon != 0 || fc.uncordon != 0 || fc.approveCalls != 0 {
		t.Errorf("dry-run start called the cluster: list=%d cordon=%d uncordon=%d approve=%d", fc.listNodesCalls, fc.cordon, fc.uncordon, fc.approveCalls)
	}
	assertUnchanged(t, tfvars, "SENTINEL_TFVARS\n")
	assertUnchanged(t, cfgPath, "SENTINEL_CONFIG\n")
}

func TestStartDryRunDoesNotRequirePower(t *testing.T) {
	fc := &fakeCluster{nodes: startTestNodes()}
	r, _, _ := seedRunner(t, fc, &fakeTF{}, startTestConfig())
	r.Power = nil

	if err := r.Start(context.Background(), StartOptions{}); err != nil {
		t.Fatalf("dry-run start without a power-cycler must not fail: %v", err)
	}
}

func TestStartRefusesWithoutPowerCycler(t *testing.T) {
	fc := &fakeCluster{nodes: startTestNodes()}
	r, _, _ := seedRunner(t, fc, &fakeTF{}, startTestConfig())
	r.DryRun = false
	r.Power = nil

	err := r.Start(context.Background(), StartOptions{})
	if err == nil {
		t.Fatal("expected refusal when no power-cycler is wired")
	}
	if fc.listNodesCalls != 0 {
		t.Errorf("refusal must precede any cluster call: list=%d", fc.listNodesCalls)
	}
}

func TestStartPowersMastersBeforeWorkers(t *testing.T) {
	fc := &fakeCluster{nodes: startTestNodes()}
	fp := &fakePower{}
	r := seedStartRunner(t, fc, fp)

	if err := r.Start(context.Background(), StartOptions{}); err != nil {
		t.Fatalf("start: %v", err)
	}

	want := []int{6010, 6011, 6100, 6101}
	if len(fp.startOrder) != len(want) {
		t.Fatalf("startOrder = %v, want %v", fp.startOrder, want)
	}
	for i, vmid := range want {
		if fp.startOrder[i] != vmid {
			t.Errorf("startOrder[%d] = %d, want %d (masters ascending then workers ascending): full order %v",
				i, fp.startOrder[i], vmid, fp.startOrder)
		}
	}
	if fc.uncordon != 4 {
		t.Errorf("start must uncordon every node from real ListNodes names: uncordon=%d", fc.uncordon)
	}
	if fc.cordon != 0 {
		t.Errorf("start must never cordon: cordon=%d", fc.cordon)
	}
}

func TestStartWaitKeepsApprovingBeforeReady(t *testing.T) {
	// Nodes never report Ready; a tight timeout means only the immediate poll
	// runs, which must still attempt CSR approval.
	nodes := startTestNodes()
	for i := range nodes {
		nodes[i].Ready = false
	}
	fc := &fakeCluster{nodes: nodes}
	fp := &fakePower{}
	r := seedStartRunner(t, fc, fp)
	r.ClusterReadyTimeout = 50 * time.Millisecond

	err := r.Start(context.Background(), StartOptions{})
	if err == nil {
		t.Fatal("expected a timeout error when nodes never become Ready")
	}
	if fc.approveCalls < 1 {
		t.Errorf("CSR approval must run while waiting even before nodes are Ready, got %d", fc.approveCalls)
	}
	if fc.uncordon != 0 {
		t.Errorf("start must not uncordon when the readiness wait never converges: uncordon=%d", fc.uncordon)
	}
}

func TestStartWaitSkipsApproveWhenAPIDown(t *testing.T) {
	fc := &fakeCluster{nodes: startTestNodes(), listErr: errors.New("connection refused")}
	fp := &fakePower{}
	r := seedStartRunner(t, fc, fp)
	r.ClusterReadyTimeout = 50 * time.Millisecond

	err := r.Start(context.Background(), StartOptions{})
	if err == nil {
		t.Fatal("expected a timeout error when the API is unreachable")
	}
	if fc.approveCalls != 0 {
		t.Errorf("CSR approval must be skipped while ListNodes fails (API not up): approve=%d", fc.approveCalls)
	}
}

// start is non-resumable: a foreign marker (stranded remove) must refuse before
// any cluster call or power-on, unless acknowledged.
func TestStartRefusesForeignMarkerWithoutAck(t *testing.T) {
	fc := &fakeCluster{nodes: startTestNodes()}
	fp := &fakePower{}
	r := seedStartRunner(t, fc, fp)
	seedMarker(t, r, OpRemove, "worker5", StepDrain)

	err := r.Start(context.Background(), StartOptions{})
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("want *errtypes.ConfigError refusing the foreign marker, got %v", err)
	}
	for _, want := range []string{"worker5", "drain", "remove"} {
		if !strings.Contains(cfgErr.Error(), want) {
			t.Errorf("refusal must name the stranded op: %q does not contain %q", cfgErr.Error(), want)
		}
	}
	if fc.listNodesCalls != 0 || fp.startCalls != 0 {
		t.Errorf("refused start must make zero mutation: listNodes=%d start=%d", fc.listNodesCalls, fp.startCalls)
	}

	if err := r.Start(context.Background(), StartOptions{Acknowledge: true}); err != nil {
		t.Fatalf("acknowledged start must proceed fresh: %v", err)
	}
	if fp.startCalls != 4 {
		t.Errorf("acknowledged start should power on every vm: start=%d", fp.startCalls)
	}
}

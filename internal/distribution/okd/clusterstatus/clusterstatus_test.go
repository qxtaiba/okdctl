package clusterstatus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

const (
	readyMasterJSON = `{"metadata":{"name":"master-0","labels":{"node-role.kubernetes.io/master":""}},
		"status":{"conditions":[{"type":"Ready","status":"True"}]}}`
	notReadyWorkerJSON = `{"metadata":{"name":"worker-0","labels":{"node-role.kubernetes.io/worker":""}},
		"status":{"conditions":[{"type":"Ready","status":"False"}]}}`
)

type fakeClient struct {
	healthzErr    error
	nodesJSON     string
	nodesErr      error
	operatorsJSON string
	operatorsErr  error
}

func (f *fakeClient) RawGet(context.Context, string) (string, error) {
	return "", f.healthzErr
}

func (f *fakeClient) GetJSON(_ context.Context, args ...string) (out string, found bool, err error) {
	if len(args) >= 2 && args[1] == "nodes" {
		return f.nodesJSON, false, f.nodesErr
	}
	return f.operatorsJSON, false, f.operatorsErr
}

type fakeVerifier struct {
	results []addon.VerifyResult
}

func (f *fakeVerifier) VerifyAll(context.Context) ([]addon.VerifyResult, error) {
	return f.results, nil
}

type fakePower struct {
	states map[int]nodetypes.VMState
	err    error
}

func (f *fakePower) VMStates(context.Context) (map[int]nodetypes.VMState, error) {
	return f.states, f.err
}

func boolSource(v bool) func() bool { return func() bool { return v } }

func TestParseNode(t *testing.T) {
	n, err := ParseNode([]byte(readyMasterJSON))
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	if n.Name != "master-0" {
		t.Errorf("Name = %q; want master-0", n.Name)
	}
	if n.Role != nodetypes.RoleMaster {
		t.Errorf("Role = %q; want %q", n.Role, nodetypes.RoleMaster)
	}
	if !n.Ready {
		t.Error("Ready = false; want true")
	}

	if _, err := ParseNode([]byte("{broken")); err == nil {
		t.Error("corrupt JSON: want error, got nil")
	}
}

func TestCollect_RunningCluster(t *testing.T) {
	cl := &fakeClient{
		nodesJSON:     `{"items":[` + readyMasterJSON + `]}`,
		operatorsJSON: `{"items":[{"status":{"conditions":[{"type":"Degraded","status":"False"}]}}]}`,
	}
	v := &fakeVerifier{results: []addon.VerifyResult{
		{Name: "flux"},
		{Name: "metallb", Err: errors.New("pods not ready")},
	}}

	cs := Collect(context.Background(), cl, v, LifecycleSources{})

	if cs.Phase != okd.PhaseRunning {
		t.Errorf("Phase = %q; want %q", cs.Phase, okd.PhaseRunning)
	}
	if !cs.APIReachable {
		t.Error("APIReachable = false; want true")
	}
	if len(cs.Nodes) != 1 || cs.Nodes[0].Status != nodetypes.NodeStatusReady {
		t.Errorf("Nodes = %+v; want one ready node", cs.Nodes)
	}
	if cs.DegradedOperators != 0 {
		t.Errorf("DegradedOperators = %d; want 0", cs.DegradedOperators)
	}
	if len(cs.Addons) != 2 || !cs.Addons[0].Healthy || cs.Addons[1].Healthy {
		t.Errorf("Addons = %+v; want [healthy flux, unhealthy metallb]", cs.Addons)
	}
	if cs.Addons[1].Error != "pods not ready" {
		t.Errorf("Addons[1].Error = %q; want pods not ready", cs.Addons[1].Error)
	}
}

func TestCollect_DegradedCluster(t *testing.T) {
	cl := &fakeClient{
		nodesJSON: `{"items":[` + readyMasterJSON + `,` + notReadyWorkerJSON + `]}`,
		operatorsJSON: `{"items":[
			{"status":{"conditions":[{"type":"Degraded","status":"True"}]}},
			{"status":{"conditions":[{"type":"Degraded","status":"False"}]}}]}`,
	}

	cs := Collect(context.Background(), cl, &fakeVerifier{}, LifecycleSources{})

	if cs.Phase != okd.PhaseDegraded {
		t.Errorf("Phase = %q; want %q", cs.Phase, okd.PhaseDegraded)
	}
	if cs.DegradedOperators != 1 {
		t.Errorf("DegradedOperators = %d; want 1", cs.DegradedOperators)
	}
	if len(cs.Nodes) != 2 || cs.Nodes[1].Ready || cs.Nodes[1].Status != nodetypes.NodeStatusNotReady {
		t.Errorf("Nodes = %+v; want second node not ready", cs.Nodes)
	}
	if cs.Nodes[1].Role != nodetypes.RoleWorker {
		t.Errorf("Nodes[1].Role = %q; want %q", cs.Nodes[1].Role, nodetypes.RoleWorker)
	}
}

func TestCollect_APIUnreachable(t *testing.T) {
	cl := &fakeClient{
		healthzErr:   errors.New("connection refused"),
		nodesErr:     errors.New("connection refused"),
		operatorsErr: errors.New("connection refused"),
	}

	// No lifecycle sources at all, so an unreachable API reads as Pending
	// (pre-install) rather than Installing (deploy in flight).
	cs := Collect(context.Background(), cl, &fakeVerifier{}, LifecycleSources{})

	if cs.Phase != okd.PhasePending {
		t.Errorf("Phase = %q; want %q", cs.Phase, okd.PhasePending)
	}
	if cs.APIReachable {
		t.Error("APIReachable = true; want false")
	}
	if cs.Nodes != nil || cs.DegradedOperators != 0 {
		t.Errorf("want empty sections; got nodes=%v degraded=%d", cs.Nodes, cs.DegradedOperators)
	}
}

func TestCollect_CorruptPayloadsDegradeToEmpty(t *testing.T) {
	cl := &fakeClient{nodesJSON: "{broken", operatorsJSON: "{broken"}

	cs := Collect(context.Background(), cl, &fakeVerifier{}, LifecycleSources{})

	if cs.Phase != okd.PhaseUnknown {
		t.Errorf("Phase = %q; want %q", cs.Phase, okd.PhaseUnknown)
	}
	if cs.Nodes != nil || cs.DegradedOperators != 0 {
		t.Errorf("want empty sections; got nodes=%v degraded=%d", cs.Nodes, cs.DegradedOperators)
	}
}

func TestCollect_NilClientDerivesFromLifecycleSources(t *testing.T) {
	cases := []struct {
		name string
		src  LifecycleSources
		want okd.ClusterPhase
	}{
		{"no-kubeconfig-no-infra", LifecycleSources{}, okd.PhasePending},
		{"no-kubeconfig-deploy-in-flight", LifecycleSources{DeployInProgress: boolSource(true)}, okd.PhaseInstalling},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs := Collect(context.Background(), nil, &fakeVerifier{}, tc.src)
			if cs.Phase != tc.want {
				t.Errorf("Phase = %q; want %q", cs.Phase, tc.want)
			}
			if cs.APIReachable {
				t.Error("APIReachable = true; want false")
			}
			if cs.Nodes != nil {
				t.Errorf("Nodes = %v; want nil", cs.Nodes)
			}
		})
	}
}

// TestCollect_StoppedCluster locks the 'okdctl cluster stop' → 'okdctl
// status' contract: API down, no deploy in flight, infra present, every VM
// powered off reads as Stopped.
func TestCollect_StoppedCluster(t *testing.T) {
	cl := &fakeClient{
		healthzErr:   errors.New("connection refused"),
		nodesErr:     errors.New("connection refused"),
		operatorsErr: errors.New("connection refused"),
	}
	src := LifecycleSources{
		DeployInProgress: boolSource(false),
		InfraPresent:     boolSource(true),
		Power: &fakePower{states: map[int]nodetypes.VMState{
			110: nodetypes.StateStopped,
			111: nodetypes.StateStopped,
			200: nodetypes.StateStopped,
		}},
	}

	cs := Collect(context.Background(), cl, &fakeVerifier{}, src)

	if cs.Phase != okd.PhaseStopped {
		t.Errorf("Phase = %q; want %q", cs.Phase, okd.PhaseStopped)
	}
	if cs.APIReachable {
		t.Error("APIReachable = true; want false")
	}
}

func TestNewClient_MissingKubeconfig(t *testing.T) {
	if _, err := NewClient(t.TempDir()); err == nil {
		t.Error("want error for missing kubeconfig, got nil")
	}
}

func TestDerivePhase(t *testing.T) {
	ready := okd.NodeStatus{Ready: true}
	notReady := okd.NodeStatus{Ready: false}
	allStopped := &fakePower{states: map[int]nodetypes.VMState{110: nodetypes.StateStopped, 200: nodetypes.StateStopped}}
	someRunning := &fakePower{states: map[int]nodetypes.VMState{110: nodetypes.StateRunning, 200: nodetypes.StateStopped}}

	cases := []struct {
		name     string
		apiOK    bool
		nodes    []okd.NodeStatus
		degraded int
		src      LifecycleSources
		want     okd.ClusterPhase
	}{
		{"running", true, []okd.NodeStatus{ready}, 0, LifecycleSources{}, okd.PhaseRunning},
		{"degraded-operators", true, []okd.NodeStatus{ready}, 1, LifecycleSources{}, okd.PhaseDegraded},
		{"degraded-notready-node-zero-degraded", true, []okd.NodeStatus{ready, notReady}, 0, LifecycleSources{}, okd.PhaseDegraded},
		{"unknown-api-up-no-node-listing", true, nil, 0, LifecycleSources{}, okd.PhaseUnknown},
		{"pending-no-marker-no-infra", false, nil, 0, LifecycleSources{
			DeployInProgress: boolSource(false), InfraPresent: boolSource(false),
		}, okd.PhasePending},
		{"installing-marker-mid-install-api-down", false, nil, 0, LifecycleSources{
			DeployInProgress: boolSource(true),
		}, okd.PhaseInstalling},
		{"stopped-marker-done-vms-off", false, nil, 0, LifecycleSources{
			DeployInProgress: boolSource(false), InfraPresent: boolSource(true), Power: allStopped,
		}, okd.PhaseStopped},
		{"unknown-api-down-vms-running", false, nil, 0, LifecycleSources{
			DeployInProgress: boolSource(false), InfraPresent: boolSource(true), Power: someRunning,
		}, okd.PhaseUnknown},
		{"unknown-infra-present-no-prober", false, nil, 0, LifecycleSources{
			DeployInProgress: boolSource(false), InfraPresent: boolSource(true),
		}, okd.PhaseUnknown},
		{"unknown-power-probe-fails", false, nil, 0, LifecycleSources{
			DeployInProgress: boolSource(false), InfraPresent: boolSource(true),
			Power: &fakePower{err: errors.New("api unreachable")},
		}, okd.PhaseUnknown},
		{"unknown-power-probe-finds-no-vms", false, nil, 0, LifecycleSources{
			DeployInProgress: boolSource(false), InfraPresent: boolSource(true),
			Power: &fakePower{states: map[int]nodetypes.VMState{}},
		}, okd.PhaseUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := derivePhase(context.Background(), tc.apiOK, tc.nodes, tc.degraded, tc.src)
			if got != tc.want {
				t.Fatalf("derivePhase() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTerraformStateHasResources(t *testing.T) {
	seed := func(t *testing.T, tfEnv, content string) string {
		t.Helper()
		root := t.TempDir()
		dir := filepath.Join(root, "infrastructure", "terraform", "environments", tfEnv)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if content != "" {
			if err := os.WriteFile(filepath.Join(dir, "terraform.tfstate"), []byte(content), 0o600); err != nil {
				t.Fatalf("write tfstate: %v", err)
			}
		}
		return root
	}

	if TerraformStateHasResources(t.TempDir(), "production") {
		t.Error("missing state file must not count as infra")
	}
	if TerraformStateHasResources(seed(t, "production", `{"resources":[]}`), "production") {
		t.Error("empty post-destroy state must not count as infra")
	}
	if TerraformStateHasResources(seed(t, "production", "{broken"), "production") {
		t.Error("unparseable state must not count as infra")
	}
	root := seed(t, "production", `{"resources":[{"type":"proxmox_virtual_environment_vm"}]}`)
	if !TerraformStateHasResources(root, "production") {
		t.Error("state with resources must count as infra")
	}
	// A populated state in a different environment than the configured one
	// must not count — the pre-M12 glob would have matched it.
	if TerraformStateHasResources(root, "staging") {
		t.Error("state under another environment must not count for the configured env")
	}
}

func TestStatusNodeStatusPhase(t *testing.T) {
	cases := []struct {
		name       string
		conditions []statusCondition
		wantPhase  nodetypes.NodeStatusPhase
		wantReady  bool
	}{
		{"ready", []statusCondition{{Type: nodetypes.ConditionTypeReady, Status: nodetypes.ConditionStatusTrue}}, nodetypes.NodeStatusReady, true},
		{"not-ready", []statusCondition{{Type: nodetypes.ConditionTypeReady, Status: nodetypes.ConditionStatusFalse}}, nodetypes.NodeStatusNotReady, false},
		{"unknown-condition", []statusCondition{{Type: nodetypes.ConditionTypeReady, Status: nodetypes.ConditionStatusUnknown}}, nodetypes.NodeStatusUnknown, false},
		{"missing-condition", nil, nodetypes.NodeStatusUnknown, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := &statusNode{}
			n.Status.Conditions = tc.conditions
			if got := n.statusPhase(); got != tc.wantPhase {
				t.Fatalf("statusPhase() = %q, want %q", got, tc.wantPhase)
			}
			if got := n.isReady(); got != tc.wantReady {
				t.Fatalf("isReady() = %v, want %v", got, tc.wantReady)
			}
		})
	}
}

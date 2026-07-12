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

	cs := Collect(context.Background(), cl, v, t.TempDir())

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

	cs := Collect(context.Background(), cl, &fakeVerifier{}, t.TempDir())

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

	// No terraform state under this root, so an unreachable API reads as
	// Pending (pre-install) rather than Installing (infra up, no cluster yet).
	cs := Collect(context.Background(), cl, &fakeVerifier{}, t.TempDir())

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

	cs := Collect(context.Background(), cl, &fakeVerifier{}, t.TempDir())

	if cs.Phase != okd.PhaseUnknown {
		t.Errorf("Phase = %q; want %q", cs.Phase, okd.PhaseUnknown)
	}
	if cs.Nodes != nil || cs.DegradedOperators != 0 {
		t.Errorf("want empty sections; got nodes=%v degraded=%d", cs.Nodes, cs.DegradedOperators)
	}
}

func TestCollect_NilClientDerivesFromInfra(t *testing.T) {
	withState := t.TempDir()
	stateDir := filepath.Join(withState, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "terraform.tfstate"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write tfstate: %v", err)
	}

	cases := []struct {
		name        string
		projectRoot string
		want        okd.ClusterPhase
	}{
		{"no-kubeconfig-no-infra", t.TempDir(), okd.PhasePending},
		{"no-kubeconfig-infra-present", withState, okd.PhaseInstalling},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs := Collect(context.Background(), nil, &fakeVerifier{}, tc.projectRoot)
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

func TestNewClient_MissingKubeconfig(t *testing.T) {
	if _, err := NewClient(t.TempDir()); err == nil {
		t.Error("want error for missing kubeconfig, got nil")
	}
}

func TestDerivePhase(t *testing.T) {
	withState := t.TempDir()
	stateDir := filepath.Join(withState, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "terraform.tfstate"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write tfstate: %v", err)
	}
	withoutState := t.TempDir()

	ready := okd.NodeStatus{Ready: true}
	notReady := okd.NodeStatus{Ready: false}

	cases := []struct {
		name        string
		apiOK       bool
		nodes       []okd.NodeStatus
		degraded    int
		projectRoot string
		want        okd.ClusterPhase
	}{
		{"running", true, []okd.NodeStatus{ready}, 0, withoutState, okd.PhaseRunning},
		{"degraded", true, []okd.NodeStatus{ready}, 1, withoutState, okd.PhaseDegraded},
		{"pending-no-infra", false, nil, 0, withoutState, okd.PhasePending},
		{"installing-infra-present", false, nil, 0, withState, okd.PhaseInstalling},
		{"unknown-partial-ready", true, []okd.NodeStatus{ready, notReady}, 0, withoutState, okd.PhaseUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := derivePhase(tc.apiOK, tc.nodes, tc.degraded, tc.projectRoot)
			if got != tc.want {
				t.Fatalf("derivePhase() = %q, want %q", got, tc.want)
			}
		})
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

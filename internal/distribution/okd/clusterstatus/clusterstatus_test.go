package clusterstatus

import (
	"context"
	"errors"
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

func (f *fakeClient) GetJSON(_ context.Context, args ...string) (string, bool, error) {
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

	cs := Collect(context.Background(), cl, v)

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

	cs := Collect(context.Background(), cl, &fakeVerifier{})

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

	cs := Collect(context.Background(), cl, &fakeVerifier{})

	if cs.Phase != okd.PhaseUnknown {
		t.Errorf("Phase = %q; want %q", cs.Phase, okd.PhaseUnknown)
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

	cs := Collect(context.Background(), cl, &fakeVerifier{})

	if cs.Phase != okd.PhaseUnknown {
		t.Errorf("Phase = %q; want %q", cs.Phase, okd.PhaseUnknown)
	}
	if cs.Nodes != nil || cs.DegradedOperators != 0 {
		t.Errorf("want empty sections; got nodes=%v degraded=%d", cs.Nodes, cs.DegradedOperators)
	}
}

func TestNewClient_MissingKubeconfig(t *testing.T) {
	if _, err := NewClient(t.TempDir()); err == nil {
		t.Error("want error for missing kubeconfig, got nil")
	}
}

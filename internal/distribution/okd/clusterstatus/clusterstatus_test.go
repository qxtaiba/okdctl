package clusterstatus

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	healthzErr         error
	nodesJSON          string
	nodesErr           error
	nodesTruncated     bool
	operatorsJSON      string
	operatorsErr       error
	operatorsTruncated bool
	healthzStarted     chan struct{}
	healthzRelease     chan struct{}
}

func (f *fakeClient) RawGet(_ context.Context, path string) (string, error) {
	if path == "/healthz" && f.healthzStarted != nil {
		close(f.healthzStarted)
		<-f.healthzRelease
	}
	return "", f.healthzErr
}

func (f *fakeClient) GetJSON(_ context.Context, args ...string) (out string, found bool, err error) {
	if len(args) >= 2 && args[1] == "nodes" {
		return f.nodesJSON, f.nodesTruncated, f.nodesErr
	}
	return f.operatorsJSON, f.operatorsTruncated, f.operatorsErr
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
	if !cs.APIAvailable || !cs.NodesAvailable || !cs.OperatorsAvailable {
		t.Errorf("availability flags = api:%v nodes:%v operators:%v; want all true", cs.APIAvailable, cs.NodesAvailable, cs.OperatorsAvailable)
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

func TestCollectMeasuresAPILatency(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	cl := &fakeClient{
		nodesJSON:      `{"items":[` + readyMasterJSON + `]}`,
		operatorsJSON:  `{"items":[]}`,
		healthzStarted: started,
		healthzRelease: release,
	}
	result := make(chan okd.ClusterStatus, 1)
	go func() { result <- Collect(context.Background(), cl, &fakeVerifier{}, LifecycleSources{}) }()
	<-started
	time.Sleep(20 * time.Millisecond)
	close(release)
	cs := <-result
	if cs.APILatency <= 0 {
		t.Errorf("API latency = %s, want measured positive duration", cs.APILatency)
	}
	if cs.APILatency < 10*time.Millisecond {
		t.Errorf("API latency = %s, want to include the delayed /healthz round trip", cs.APILatency)
	}
}

func TestReadLastDeployRunUsesTrustedHistory(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, "okd-install")
	kubeconfig := filepath.Join(workDir, "cluster-config", "auth", "kubeconfig")
	if err := os.MkdirAll(filepath.Dir(kubeconfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kubeconfig, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, time.September, 27, 8, 0, 0, 0, time.UTC)
	history := map[string]any{"schema_version": "v1", "run_id": "run-abc", "timestamp": stamp, "cluster_name": "prod-cluster", "steps": map[string]float64{"setup-configure": 12}}
	data, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".okdctl-step-history.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	got := readLastDeployRun(kubeconfig, "prod-cluster")
	if got.RunID != "run-abc" || got.ClusterName != "prod-cluster" || !got.At.Equal(stamp) {
		t.Errorf("last run = %+v, want run-abc at %s", got, stamp)
	}
	if got := readLastDeployRun(kubeconfig, "other-cluster"); got.RunID != "" || !got.At.IsZero() {
		t.Errorf("foreign-cluster history = %+v, want unavailable", got)
	}
}

func TestReadLastDeployRunRejectsSymlinkAndInvalidHistory(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, "okd-install")
	kubeconfig := filepath.Join(workDir, "cluster-config", "auth", "kubeconfig")
	if err := os.MkdirAll(filepath.Dir(kubeconfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kubeconfig, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	historyPath := filepath.Join(workDir, ".okdctl-step-history.json")
	if err := os.WriteFile(historyPath, []byte(strings.Repeat("x", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readLastDeployRun(kubeconfig, "prod-cluster"); got.RunID != "" {
		t.Errorf("invalid history = %+v, want unavailable", got)
	}

	outside := filepath.Join(root, "outside.json")
	if err := os.WriteFile(outside, []byte(`{"schema_version":"v1","run_id":"run-x","cluster_name":"prod-cluster"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(historyPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, historyPath); err != nil {
		t.Fatal(err)
	}
	if got := readLastDeployRun(kubeconfig, "prod-cluster"); got.RunID != "" {
		t.Errorf("symlinked history = %+v, want unavailable", got)
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

	// No lifecycle sources: an unreachable API reads as Pending, not Installing.
	cs := Collect(context.Background(), cl, &fakeVerifier{}, LifecycleSources{})

	if cs.Phase != okd.PhasePending {
		t.Errorf("Phase = %q; want %q", cs.Phase, okd.PhasePending)
	}
	if cs.APIReachable {
		t.Error("APIReachable = true; want false")
	}
	if !cs.APIAvailable || cs.NodesAvailable || cs.OperatorsAvailable {
		t.Errorf("availability flags = api:%v nodes:%v operators:%v; want queried api and unavailable sections", cs.APIAvailable, cs.NodesAvailable, cs.OperatorsAvailable)
	}
	if cs.Nodes != nil || cs.DegradedOperators != 0 {
		t.Errorf("want empty sections; got nodes=%v degraded=%d", cs.Nodes, cs.DegradedOperators)
	}
}

func TestCollect_TruncatedSectionsAreUnavailable(t *testing.T) {
	cl := &fakeClient{
		nodesJSON: `{"items":[]}`, nodesTruncated: true,
		operatorsJSON: `{"items":[]}`, operatorsTruncated: true,
	}
	cs := Collect(context.Background(), cl, &fakeVerifier{}, LifecycleSources{})
	if cs.NodesAvailable || cs.OperatorsAvailable {
		t.Errorf("truncated sections marked available: nodes=%v operators=%v", cs.NodesAvailable, cs.OperatorsAvailable)
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
	cs := Collect(context.Background(), nil, &fakeVerifier{}, LifecycleSources{})
	if cs.Phase != okd.PhasePending {
		t.Errorf("Phase = %q; want %q", cs.Phase, okd.PhasePending)
	}
	if cs.APIReachable {
		t.Error("APIReachable = true; want false")
	}
	if cs.Nodes != nil {
		t.Errorf("Nodes = %v; want nil", cs.Nodes)
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
	// A different environment's state must not count — a glob across envs would match it.
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

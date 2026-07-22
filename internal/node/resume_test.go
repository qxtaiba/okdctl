package node

import (
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

const (
	testWorkerNode = "worker2"
	testMasterNode = "master0"
)

func TestShouldRunStepFreshRunsEveryStep(t *testing.T) {
	for step := range stepOrder {
		if !shouldRunStep(step, "") {
			t.Errorf("fresh op (from=\"\") must run step %q", step)
		}
	}
}

func TestShouldRunStepResumesAtAndAfter(t *testing.T) {
	cases := []struct {
		step, from Step
		want       bool
	}{
		{StepCordon, StepTFApply, false},
		{StepDrain, StepTFApply, false},
		{StepTFApply, StepTFApply, true},
		{StepPowerCycle, StepTFApply, true},
		{StepDeleteK8s, StepTFApply, true},
		{StepUncordon, StepTFApply, true},
		{StepCordon, StepUncordon, false},
		{StepUncordon, StepUncordon, true},
		{StepCordon, StepCordon, true},
	}
	for _, c := range cases {
		if got := shouldRunStep(c.step, c.from); got != c.want {
			t.Errorf("shouldRunStep(%q, from=%q) = %v; want %v", c.step, c.from, got, c.want)
		}
	}
}

// seedMarker writes an op marker under r.WorkDir so beginOp reads it.
func seedMarker(t *testing.T, r *Runner, op Op, target string, step Step) {
	t.Helper()
	if err := markStep(r.marker(), op, target, step, r.RunID, r.Cfg.Cluster.Name); err != nil {
		t.Fatalf("seed marker: %v", err)
	}
}

func matchTarget(name string) OpMatch {
	return func(m *OpMarker) bool { return m.Target == name }
}

func TestBeginOpNoMarkerRunsFresh(t *testing.T) {
	r, _, _ := seedRunner(t, &fakeCluster{}, &fakeTF{}, config.DefaultConfig())

	marker, err := r.beginOp(OpRemove, matchTarget(testWorkerNode), false)
	if err != nil {
		t.Fatalf("beginOp: %v", err)
	}
	if marker != nil {
		t.Errorf("absent marker must yield nil (fresh run), got %+v", marker)
	}
}

func TestBeginOpMatchingMarkerResumes(t *testing.T) {
	r, _, _ := seedRunner(t, &fakeCluster{}, &fakeTF{}, config.DefaultConfig())
	seedMarker(t, r, OpRemove, testWorkerNode, StepTFApply)

	marker, err := r.beginOp(OpRemove, matchTarget(testWorkerNode), false)
	if err != nil {
		t.Fatalf("beginOp: %v", err)
	}
	if marker == nil {
		t.Fatal("matching marker must resume (non-nil marker)")
	}
	if marker.Step != StepTFApply || marker.Target != testWorkerNode {
		t.Errorf("resumed marker = %+v; want target %s step tf-apply", marker, testWorkerNode)
	}
}

// TestBeginOpMatchingResizeMarkerResumes also exercises beginOp/matchTarget with
// a second op and target so the resume seam is proven op-agnostic (C3 threads a
// third op through the same call).
func TestBeginOpMatchingResizeMarkerResumes(t *testing.T) {
	r, _, _ := seedRunner(t, &fakeCluster{}, &fakeTF{}, config.DefaultConfig())
	seedMarker(t, r, OpResize, testMasterNode, StepPowerCycle)

	marker, err := r.beginOp(OpResize, matchTarget(testMasterNode), false)
	if err != nil {
		t.Fatalf("beginOp: %v", err)
	}
	if marker == nil || marker.Target != testMasterNode || marker.Step != StepPowerCycle {
		t.Fatalf("resize marker must resume: %+v", marker)
	}
}

func TestBeginOpForeignMarkerRefusedWithoutAck(t *testing.T) {
	r, _, _ := seedRunner(t, &fakeCluster{}, &fakeTF{}, config.DefaultConfig())
	seedMarker(t, r, OpResize, testMasterNode, StepPowerCycle)

	marker, err := r.beginOp(OpRemove, matchTarget(testWorkerNode), false)
	if marker != nil {
		t.Errorf("foreign marker must not resume, got %+v", marker)
	}
	var cfgErr *errtypes.ConfigError
	if err == nil || !errors.As(err, &cfgErr) {
		t.Fatalf("want *errtypes.ConfigError, got %v", err)
	}
}

func TestBeginOpForeignMarkerAcknowledgedProceedsFresh(t *testing.T) {
	r, _, _ := seedRunner(t, &fakeCluster{}, &fakeTF{}, config.DefaultConfig())
	seedMarker(t, r, OpResize, testMasterNode, StepPowerCycle)

	marker, err := r.beginOp(OpRemove, matchTarget(testWorkerNode), true)
	if err != nil {
		t.Fatalf("beginOp with ack: %v", err)
	}
	if marker != nil {
		t.Errorf("acknowledged foreign marker must run fresh (nil marker), got %+v", marker)
	}
}

func TestResizeScopeMatch(t *testing.T) {
	nodes := []cluster.NodeDetail{
		{Name: testMasterNode, Role: nodetypes.RoleMaster},
		{Name: "worker1", Role: nodetypes.RoleWorker},
	}
	roleScope := resizeScopeMatch(ResizeScope{Role: nodetypes.RoleMaster}, nodes)
	if !roleScope(&OpMarker{Target: testMasterNode}) {
		t.Error("role scope must match a marker naming a node of that role")
	}
	if roleScope(&OpMarker{Target: "worker1"}) {
		t.Error("role scope must not match a marker naming a different-role node")
	}
	nodeScope := resizeScopeMatch(ResizeScope{Node: "worker1"}, nodes)
	if !nodeScope(&OpMarker{Target: "worker1"}) {
		t.Error("single-node scope must match its own node")
	}
	if nodeScope(&OpMarker{Target: testMasterNode}) {
		t.Error("single-node scope must not match another node")
	}
}

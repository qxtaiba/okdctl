package steps

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// statusFixture is the credential-free snapshot the status screen's tests and
// goldens render: three masters and three workers, all ready.
func statusFixture() *okd.ClusterStatus {
	nodes := make([]okd.NodeStatus, 0, 6)
	for i := range 3 {
		nodes = append(nodes, okd.NodeStatus{Name: "homelab-master" + string(rune('0'+i)), Role: nodetypes.RoleMaster, Ready: true})
	}
	for i := range 3 {
		nodes = append(nodes, okd.NodeStatus{Name: "homelab-worker" + string(rune('0'+i)), Role: nodetypes.RoleWorker, Ready: true})
	}
	return &okd.ClusterStatus{
		Phase:               okd.PhaseRunning,
		APIReachable:        true,
		APIAvailable:        true,
		APILatencyAvailable: true,
		APILatency:          42 * time.Millisecond,
		NodesAvailable:      true,
		OperatorsAvailable:  true,
		Nodes:               nodes,
		Addons:              []okd.AddonStatus{{Name: "flux", Healthy: true}},
	}
}

func TestStatusStepShowsLatencyAndRoleGroupedSelectableTable(t *testing.T) {
	s := NewStatusStep(&countingSource{status: statusFixture()})
	s.Update(s.Init()())
	body := tuitest.StripANSI(s.View(100, 30))
	for _, want := range []string{"42ms", "NODES 6/6 ready", "master", "worker", "NODE", "READINESS"} {
		if !strings.Contains(body, want) {
			t.Errorf("status board is missing %q:\n%s", want, body)
		}
	}
	tuitest.AssertFits(t, body, 100, 30)
	if strings.Index(body, "homelab-master0") > strings.Index(body, "homelab-worker0") {
		t.Errorf("node groups are not ordered master before worker:\n%s", body)
	}
}

func TestStatusStepSelectsNodesAndTogglesInlineDetails(t *testing.T) {
	s := NewStatusStep(&countingSource{status: statusFixture()})
	s.Update(s.Init()())
	if s.selectedNode != "homelab-master0" {
		t.Fatalf("initial selection = %q", s.selectedNode)
	}
	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.selectedNode != "homelab-master1" {
		t.Fatalf("selection after down = %q", s.selectedNode)
	}
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	body := tuitest.StripANSI(s.View(100, 30))
	for _, want := range []string{">   homelab-master1", "Selected node", "homelab-master1", "Ready condition"} {
		if !strings.Contains(body, want) {
			t.Errorf("inline node detail is missing %q:\n%s", want, body)
		}
	}
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if s.detailOpen {
		t.Error("second enter left inline details open")
	}
}

func TestStatusStepIgnoresStaleProbeAndCancelsPeriodicRefresh(t *testing.T) {
	s := NewStatusStep(&countingSource{status: statusFixture()})
	first := s.probe()()
	second := s.probe()()
	s.Update(second)
	s.Update(statusLoadedMsg{generation: s.generation - 1, err: errors.New("stale")})
	if s.err != nil || s.status == nil {
		t.Fatalf("stale probe replaced current snapshot: err=%v status=%v", s.err, s.status)
	}
	refresh := s.scheduleRefresh()
	s.SetFocused(false)
	if msg := refresh(); msg != nil {
		t.Errorf("cancelled refresh returned %T, want nil", msg)
	}
	s.Update(first)
	if s.generation == 0 {
		t.Fatal("focus loss did not invalidate pending messages")
	}
}

func TestStatusStepRefreshKeepsSelectionByNodeName(t *testing.T) {
	status := statusFixture()
	s := NewStatusStep(&countingSource{status: status})
	s.Update(s.Init()())
	s.selectedNode = "homelab-worker1"
	s.status = &okd.ClusterStatus{Nodes: []okd.NodeStatus{
		{Name: "homelab-worker1", Role: nodetypes.RoleWorker, Ready: false},
		{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true},
	}}
	s.reconcileSelection()
	if s.selectedNode != "homelab-worker1" {
		t.Errorf("selection after refresh = %q, want same node", s.selectedNode)
	}
}

func TestStatusStepNarrowTableFits(t *testing.T) {
	st := statusFixture()
	st.Nodes = append(st.Nodes, okd.NodeStatus{Name: strings.Repeat("worker-name-", 8), Role: nodetypes.RoleWorker, Ready: true})
	for _, width := range []int{40, 80} {
		body := tuitest.StripANSI(fitStatusLines(statusBoardLines(st, false, nil, "", false, width), width, 24))
		tuitest.AssertFits(t, body, width, 24)
	}
}

func TestStatusStepDoesNotInventUnavailableReadiness(t *testing.T) {
	st := statusFixture()
	st.APIAvailable = false
	st.APIReachable = false
	st.NodesAvailable = false
	st.OperatorsAvailable = false
	body := tuitest.StripANSI(fitStatusLines(statusBoardLines(st, false, nil, "", false, 100), 100, 30))
	for _, want := range []string{"API", "unavailable", "NODES unavailable", "OPERATORS unavailable", "node inventory unavailable"} {
		if !strings.Contains(body, want) {
			t.Errorf("status board is missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "6/6 ready") || strings.Contains(body, "0 degraded") {
		t.Errorf("status board invented readiness for unavailable data:\n%s", body)
	}
}

// countingSource records how many times the step probed it.
type countingSource struct {
	status *okd.ClusterStatus
	err    error
	probes int
}

func (c *countingSource) ClusterStatus(context.Context) (*okd.ClusterStatus, error) {
	c.probes++
	return c.status, c.err
}

func TestStatusStepProbesOnInitAndRendersTheBox(t *testing.T) {
	src := &countingSource{status: statusFixture()}
	s := NewStatusStep(src)

	cmd := s.Init()
	if cmd == nil {
		t.Fatal("Init must start a probe")
	}
	if !strings.Contains(s.View(70, 14), "reading cluster status") {
		t.Error("the in-flight view must say a probe is running")
	}

	s.Update(cmd())
	if src.probes != 1 {
		t.Fatalf("source probed %d times, want 1", src.probes)
	}

	body := s.View(70, 14)
	for _, want := range []string{"CLUSTER STATUS", "Running", "homelab-master0", "worker"} {
		if !strings.Contains(body, want) {
			t.Errorf("status body is missing %q:\n%s", want, body)
		}
	}
}

func TestStatusStepRefreshKeyReprobes(t *testing.T) {
	src := &countingSource{status: statusFixture()}
	s := NewStatusStep(src)
	s.Update(s.Init()())

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r must start another probe")
	}
	s.Update(cmd())

	if src.probes != 2 {
		t.Errorf("source probed %d times after a refresh, want 2", src.probes)
	}
}

func TestStatusStepPeriodicRefreshReprobesOnlyForCurrentGeneration(t *testing.T) {
	src := &countingSource{status: statusFixture()}
	s := NewStatusStep(src)
	s.SetFocused(true)
	s.Update(s.Init()())

	_, cmd := s.Update(statusRefreshMsg{generation: s.generation})
	if cmd == nil {
		t.Fatal("current periodic refresh did not start a probe")
	}
	s.Update(cmd())
	if src.probes != 2 {
		t.Errorf("source probed %d times, want initial and periodic probes", src.probes)
	}
	_, cmd = s.Update(statusRefreshMsg{generation: s.generation - 1})
	if cmd != nil {
		t.Error("stale periodic refresh started a probe")
	}
}

func TestStatusStepRefreshFailureKeepsTheLastSnapshot(t *testing.T) {
	src := &countingSource{status: statusFixture()}
	s := NewStatusStep(src)
	s.Update(s.Init()())
	src.err = errors.New("probe timed out")

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r must start another probe")
	}
	s.Update(cmd())

	body := tuitest.StripANSI(s.View(80, 24))
	tuitest.AssertFits(t, body, 80, 24)
	for _, want := range []string{"refresh failed", "probe timed out", "homelab-master0"} {
		if !strings.Contains(body, want) {
			t.Errorf("refresh failure view is missing %q:\n%s", want, body)
		}
	}
}

func TestStatusStepRendersProbeFailure(t *testing.T) {
	s := NewStatusStep(&countingSource{err: errors.New("read cluster status: oc not found")})
	s.Update(s.Init()())

	body := s.View(70, 14)
	for _, want := range []string{"cluster status unavailable", "press r to retry", "oc not found"} {
		if !strings.Contains(body, want) {
			t.Errorf("failure body is missing %q:\n%s", want, body)
		}
	}
}

func TestStatusStepWithoutASourceReportsIt(t *testing.T) {
	s := NewStatusStep(nil)
	s.Update(s.Init()())

	if !errors.Is(s.err, errNoStatusSource) {
		t.Errorf("err = %v, want errNoStatusSource", s.err)
	}
}

func TestStatusStepSuppressesTheWideSplit(t *testing.T) {
	if !NewStatusStep(nil).SuppressesSplit() {
		t.Error("SuppressesSplit() = false; a full-width status box has no room for a context pane")
	}
}

func TestStatusFlowIsOneScreenWithNoTrail(t *testing.T) {
	flowSteps, chrome := StatusFlow(StaticStatusSource{Status: statusFixture()})
	if len(flowSteps) != 1 {
		t.Fatalf("StatusFlow returned %d steps, want 1", len(flowSteps))
	}
	if got := flowSteps[0].ID(); got != StepIDClusterStatus {
		t.Errorf("flow step ID = %q, want %q", got, StepIDClusterStatus)
	}
	if got := chrome.Trail(wizard.ProgressInfo{Current: 1, Total: 1}); got != "" {
		t.Errorf("trail = %q, want empty for a single-screen flow", got)
	}
}

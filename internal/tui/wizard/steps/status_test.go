package steps

import (
	"context"
	"errors"
	"strings"
	"testing"

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
		Phase:              okd.PhaseRunning,
		APIReachable:       true,
		APIAvailable:       true,
		NodesAvailable:     true,
		OperatorsAvailable: true,
		Nodes:              nodes,
		Addons:             []okd.AddonStatus{{Name: "flux", Healthy: true}},
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

package steps

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
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
	body := tuitest.StripANSI(s.View(96, 30))
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

func TestStatusStepWidePaneTracksSelectionAndDetails(t *testing.T) {
	s := NewStatusStep(&countingSource{status: statusFixture()})
	s.Update(s.Init()())

	body := tuitest.StripANSI(s.PaneContent(64, 24))
	for _, want := range []string{"homelab-master0", "CLUSTER FACTS", "42ms", "6/6 ready", "0 degraded"} {
		if !strings.Contains(body, want) {
			t.Errorf("wide pane is missing %q:\n%s", want, body)
		}
	}
	lines := strings.Split(body, "\n")
	clusterLine, addonLine := strings.Index(body, "CLUSTER FACTS"), strings.Index(body, "ADD-ONS")
	if clusterLine < 0 || addonLine < 0 || addonLine < clusterLine {
		t.Fatalf("health sections are missing or out of order:\n%s", body)
	}
	addonRow := strings.Count(body[:addonLine], "\n")
	if addonRow < len(lines)*2/3 {
		t.Errorf("add-on health should use the lower context pane, got row %d of %d:\n%s", addonRow, len(lines), body)
	}

	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	body = tuitest.StripANSI(s.PaneContent(64, 24))
	for _, want := range []string{"homelab-master1", "Selected node", "Ready condition"} {
		if !strings.Contains(body, want) {
			t.Errorf("expanded wide pane is missing %q:\n%s", want, body)
		}
	}
	// A terminal this size actually splits (wizard.SplitsFrame), so View's
	// own body content must defer the detail to the pane.
	s.SetTerminalSize(180, 48)
	if strings.Contains(s.View(100, 30), "Ready condition") {
		t.Errorf("wide detail should stay in the pane, not duplicate under the table:\n%s", s.View(100, 30))
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
		body := tuitest.StripANSI(fitStatusLines(statusBoardLines(st, false, nil, "", false, width, false), width, 24))
		tuitest.AssertFits(t, body, width, 24)
	}
}

func TestStatusStepDoesNotInventUnavailableReadiness(t *testing.T) {
	st := statusFixture()
	st.APIAvailable = false
	st.APIReachable = false
	st.NodesAvailable = false
	st.OperatorsAvailable = false
	body := tuitest.StripANSI(fitStatusLines(statusBoardLines(st, false, nil, "", false, 100, false), 100, 30))
	for _, want := range []string{"API", "unavailable", "NODES unavailable", "OPERATORS unavailable", "node inventory unavailable"} {
		if !strings.Contains(body, want) {
			t.Errorf("status board is missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "6/6 ready") || strings.Contains(body, "0 degraded") {
		t.Errorf("status board invented readiness for unavailable data:\n%s", body)
	}
	pane := tuitest.StripANSI(strings.Join(statusPaneLines(st, "", false, 64, 24), "\n"))
	for _, want := range []string{"API: unavailable", "Nodes: unavailable", "Operators: unavailable"} {
		if !strings.Contains(pane, want) {
			t.Errorf("cluster pane is missing %q:\n%s", want, pane)
		}
	}
	if strings.Contains(pane, "6/6 ready") || strings.Contains(pane, "0 degraded") {
		t.Errorf("cluster pane invented readiness for unavailable data:\n%s", pane)
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

// TestGolden_StatusBoardBanners pins three previously goldenless states: the
// initial in-flight probe (no snapshot yet), a probe failure with no
// snapshot yet, and a refresh failure that keeps showing the last good
// snapshot under its "showing last snapshot" banner.
func TestGolden_StatusBoardBanners(t *testing.T) {
	cases := []struct {
		name  string
		build func() *StatusStep
		want  []string
	}{
		{
			name: "loading",
			build: func() *StatusStep {
				s := NewStatusStep(&countingSource{status: statusFixture()})
				s.Init()
				return s
			},
			want: []string{"reading cluster status"},
		},
		{
			name: "probe-failure",
			build: func() *StatusStep {
				s := NewStatusStep(&countingSource{err: errors.New("read cluster status: oc not found")})
				s.Update(s.Init()())
				return s
			},
			want: []string{"cluster status unavailable", "press r to retry", "oc not found"},
		},
		{
			name: "stale-snapshot-banner",
			build: func() *StatusStep {
				src := &countingSource{status: statusFixture()}
				s := NewStatusStep(src)
				s.Update(s.Init()())
				src.err = errors.New("probe timed out")
				_, cmd := s.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
				s.Update(cmd())
				return s
			},
			want: []string{"refresh failed", "showing last snapshot", "probe timed out", "homelab-master0"},
		},
	}

	for _, tc := range cases {
		for _, sz := range []struct{ w, h int }{{80, 24}, {180, 48}} {
			t.Run(fmt.Sprintf("%s_%dx%d", tc.name, sz.w, sz.h), func(t *testing.T) {
				s := tc.build()
				s.SetTerminalSize(sz.w, sz.h)

				frame := s.View(sz.w, sz.h)
				tuitest.Golden(t, fmt.Sprintf("status-banner-%s_%dx%d", tc.name, sz.w, sz.h), frame)
				tuitest.AssertFits(t, frame, sz.w, sz.h)

				plain := tuitest.StripANSI(frame)
				for _, want := range tc.want {
					if !strings.Contains(plain, want) {
						t.Errorf("%s at %dx%d is missing %q:\n%s", tc.name, sz.w, sz.h, want, plain)
					}
				}
			})
		}
	}
}

func TestStatusStepUsesTheWideSplit(t *testing.T) {
	if NewStatusStep(nil).SuppressesSplit() {
		t.Error("SuppressesSplit() = true; the status board has selected-node context to show")
	}
}

func TestGolden_StatusBoardResponsive(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {100, 30}, {120, 40}, {140, 40}, {180, 48}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			forceHeroColor(t)
			tui.SetTerminalWidth(size.width)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, size.width, size.height)
			seedHubSaveSlot(m)
			m.CurrentStep().(*WelcomeStep).SetFlows(HubFlows{ClusterStatus: func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
				flow, chrome := StatusFlow(StaticStatusSource{Status: statusFixture()})
				return flow, chrome, nil
			}})
			for range 3 {
				m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
			}
			_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			_, initCmd := m.Update(resolveCmd(t, cmd))
			m.Update(resolveCmd(t, initCmd))

			frame := tuitest.RenderAt(t, m, size.width, size.height)
			tuitest.Golden(t, fmt.Sprintf("status_%dx%d", size.width, size.height), frame)
			tuitest.AssertFits(t, frame, size.width, size.height)
			if size.width == 80 {
				assertNoScrollIndicator(t, frame)
			}
			if size.width >= 150 && !strings.Contains(tuitest.StripANSI(frame), "CLUSTER FACTS") {
				t.Fatal("wide status frame omitted its context pane")
			}
			if size.width < 150 && strings.Contains(tuitest.StripANSI(frame), "CLUSTER FACTS") {
				t.Fatal("single-column status frame unexpectedly rendered a context pane")
			}

			// Selecting a node must surface its detail somewhere on screen
			// — inline in the body below splitMinHeight's/wideSplitWidth's
			// threshold, in the context pane at and above it — never
			// neither, which a content-width threshold that disagreed
			// with the frame's own split gate used to do at 120x40.
			m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			detail := tuitest.RenderAt(t, m, size.width, size.height)
			tuitest.Golden(t, fmt.Sprintf("status-details_%dx%d", size.width, size.height), detail)
			tuitest.AssertFits(t, detail, size.width, size.height)
			if !strings.Contains(tuitest.StripANSI(detail), "Selected node") {
				t.Fatalf("selecting a node produced no visible detail at %dx%d:\n%s", size.width, size.height, detail)
			}
		})
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

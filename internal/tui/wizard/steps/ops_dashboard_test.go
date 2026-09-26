package steps

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

type blockingOpsSource struct {
	started chan struct{}
}

type countingOpsSource struct {
	status *okd.ClusterStatus
	calls  int
}

func (s *countingOpsSource) ClusterStatus(context.Context) (*okd.ClusterStatus, error) {
	s.calls++
	return s.status, nil
}

func (s blockingOpsSource) ClusterStatus(ctx context.Context) (*okd.ClusterStatus, error) {
	close(s.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestWelcomeOpsSnapshotUsesUpdateMessages(t *testing.T) {
	s := NewWelcomeStep()
	s.SetOpsDashboard(StaticStatusSource{Status: &okd.ClusterStatus{
		APIAvailable: true, APIReachable: true, NodesAvailable: true,
		OperatorsAvailable: true,
		Nodes: []okd.NodeStatus{
			{Name: "master0", Role: nodetypes.RoleMaster, Ready: true},
			{Name: "worker0", Role: nodetypes.RoleWorker, Ready: false},
		},
		DegradedOperators: 1,
	}})
	s.opsCtx, s.opsCancel = context.WithCancel(context.Background())
	defer s.opsCancel()
	s.opsActive = true
	s.opsGeneration = 4
	cmd := s.probeOps(4)
	msg := cmd()
	if _, ok := msg.(opsSnapshotMsg); !ok {
		t.Fatalf("probe command returned %T, want opsSnapshotMsg", msg)
	}
	s.Update(msg)
	if s.opsStatus == nil || s.opsStatus.status.DegradedOperators != 1 {
		t.Fatal("snapshot update did not store the returned cluster status")
	}
	view := renderOpsDashboard(s.opsStatus, false, nil, 76, 24)
	for _, want := range []string{"CLUSTER OPERATIONS", "API reachable", "NODES 1/2 ready", "OPERATORS 1 degraded", "RTT"} {
		if !strings.Contains(view, want) {
			t.Errorf("dashboard is missing %q:\n%s", want, view)
		}
	}
	if s.opsStatus.updated.IsZero() {
		t.Error("snapshot update time is zero")
	}
	if len(s.opsLatency) != 1 || !s.opsLatency[0].available {
		t.Errorf("successful reachable probe history = %#v, want one available sample", s.opsLatency)
	}
	status := &okd.ClusterStatus{
		APIAvailable: true, APIReachable: true, LastDeployRunID: "run-abc",
		LastDeployCluster: "prod-cluster", LastDeployAt: time.Now().Add(-2 * time.Hour),
	}
	view = renderOpsDashboard(&opsSnapshot{status: status, updated: time.Now()}, false, nil, 160, 48)
	if !strings.Contains(view, "LAST RUN prod-cluster · run-abc · 2h ago") {
		t.Errorf("dashboard is missing truthful last-run detail:\n%s", view)
	}
}

func TestWelcomeOpsDashboardShowsUnavailableSections(t *testing.T) {
	view := renderOpsDashboard(&opsSnapshot{status: &okd.ClusterStatus{}}, false, nil, 76, 24)
	for _, want := range []string{"API unavailable", "NODES unavailable", "OPERATORS unavailable"} {
		if !strings.Contains(view, want) {
			t.Errorf("dashboard is missing honest unavailable state %q:\n%s", want, view)
		}
	}
}

func TestWelcomeOpsDashboardWideLayoutUsesStatusDetail(t *testing.T) {
	status := &okd.ClusterStatus{
		Phase: okd.PhaseRunning, APIAvailable: true, APIReachable: true,
		NodesAvailable: true, OperatorsAvailable: true,
		Nodes: []okd.NodeStatus{
			{Name: "master-a", Role: nodetypes.RoleMaster, Ready: true},
			{Name: "worker-a", Role: nodetypes.RoleWorker, Ready: false},
		},
		Addons: []okd.AddonStatus{{Name: "flux", Healthy: true}},
	}
	view := renderOpsDashboard(&opsSnapshot{status: status, updated: time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)}, false, nil, 160, 40)
	for _, want := range []string{
		"CLUSTER OPERATIONS", "CLUSTER PHASE", "API", "NODES", "OPERATORS",
		"NODE FLEET", "master-a", "worker-a", "not ready", "ADD-ONS & OPERATORS", "flux",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("wide dashboard is missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "LIVE OPERATIONS") {
		t.Error("wide dashboard retained the undersized live-operations card")
	}
}

func TestOpsLatencySparklineMarksUnavailableSamples(t *testing.T) {
	snapshot := &opsSnapshot{
		latency:          70 * time.Millisecond,
		latencyAvailable: true,
		latencyHistory: []opsLatencySample{
			{duration: 30 * time.Millisecond, available: true},
			{available: false},
			{duration: 70 * time.Millisecond, available: true},
		},
	}
	if got := renderOpsLatency(snapshot); !strings.Contains(got, "30") && !strings.Contains(got, "70ms") {
		t.Errorf("latency readout missing latest value: %q", got)
	}
	if got := renderOpsLatency(snapshot); !strings.Contains(got, "·") {
		t.Errorf("latency sparkline does not mark the unavailable sample: %q", got)
	}
	if got := renderOpsLatency(&opsSnapshot{}); got != "probe latency unavailable" {
		t.Errorf("latency without samples = %q, want honest unavailable state", got)
	}
}

func TestAppendOpsLatencyKeepsLatestTwelveSamples(t *testing.T) {
	var history []opsLatencySample
	for i := range opsLatencyHistoryLimit + 4 {
		history = appendOpsLatency(history, opsLatencySample{duration: time.Duration(i) * time.Millisecond, available: true})
	}
	if len(history) != opsLatencyHistoryLimit {
		t.Fatalf("latency history has %d samples, want %d", len(history), opsLatencyHistoryLimit)
	}
	if got := history[0].duration; got != 4*time.Millisecond {
		t.Errorf("oldest retained sample = %s, want 4ms", got)
	}
}

func TestOpsRefreshFailureAddsUnavailableLatencySample(t *testing.T) {
	s := NewWelcomeStep()
	s.opsActive = true
	s.opsGeneration = 3
	s.opsStatus = &opsSnapshot{
		status:  &okd.ClusterStatus{APIAvailable: true, APIReachable: true},
		latency: 60 * time.Millisecond, latencyAvailable: true,
		latencyHistory: []opsLatencySample{{duration: 60 * time.Millisecond, available: true}},
	}
	s.Update(opsSnapshotMsg{generation: 3, err: context.DeadlineExceeded})
	if s.opsStatus.latencyAvailable {
		t.Fatal("failed refresh retained a current API latency")
	}
	if got := renderOpsLatency(s.opsStatus); !strings.Contains(got, "latest unavailable") || !strings.HasSuffix(got, "·") {
		t.Errorf("failed refresh latency = %q, want an unavailable newest sample", got)
	}
}

func TestWelcomeRefreshMessageProbesWithoutRenderingPolls(t *testing.T) {
	source := &countingOpsSource{status: &okd.ClusterStatus{APIAvailable: true, APIReachable: true}}
	s := NewWelcomeStep()
	s.SetOpsDashboard(source)
	s.opsCtx, s.opsCancel = context.WithCancel(context.Background())
	defer s.opsCancel()
	s.opsActive = true
	s.opsGeneration = 8
	s.SetTerminalSize(100, 30)
	s.View(96, 24)
	s.View(96, 24)
	if source.calls != 0 {
		t.Fatalf("render polled status source %d times", source.calls)
	}
	_, cmd := s.Update(opsRefreshMsg{generation: 8})
	if cmd == nil {
		t.Fatal("refresh update did not schedule a probe")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("refresh command = %T with %d entries, want probe and timer", cmd(), len(batch))
	}
	msg := batch[0]()
	s.Update(msg)
	if source.calls != 1 {
		t.Errorf("refresh update called source %d times, want 1", source.calls)
	}
}

func TestWelcomeCancelsOpsProbeWhenFocusLeaves(t *testing.T) {
	source := blockingOpsSource{started: make(chan struct{})}
	s := NewWelcomeStep()
	s.SetOpsDashboard(source)
	s.opsCtx, s.opsCancel = context.WithCancel(context.Background())
	s.opsActive = true
	s.opsGeneration = 2
	cmd := s.probeOps(2)
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	select {
	case <-source.started:
	case <-time.After(time.Second):
		t.Fatal("probe did not start")
	}
	s.SetFocused(false)
	select {
	case msg := <-result:
		s.Update(msg)
		if s.opsErr != nil {
			t.Fatalf("cancelled result surfaced as a probe failure: %v", s.opsErr)
		}
		if s.opsLoading {
			t.Fatal("cancelled probe left the dashboard loading")
		}
	case <-time.After(time.Second):
		t.Fatal("probe context was not cancelled when focus left the hub")
	}
}

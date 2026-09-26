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
	view := renderOpsDashboard(s.opsStatus, false, nil, 76)
	for _, want := range []string{"API reachable", "Nodes 1/2 ready", "Operators 1 degraded", "Updated"} {
		if !strings.Contains(view, want) {
			t.Errorf("dashboard is missing %q:\n%s", want, view)
		}
	}
	if s.opsStatus.updated.IsZero() {
		t.Error("snapshot update time is zero")
	}
}

func TestWelcomeOpsDashboardShowsUnavailableSections(t *testing.T) {
	view := renderOpsDashboard(&opsSnapshot{status: &okd.ClusterStatus{}}, false, nil, 76)
	for _, want := range []string{"API unavailable", "Nodes unavailable", "Operators unavailable"} {
		if !strings.Contains(view, want) {
			t.Errorf("dashboard is missing honest unavailable state %q:\n%s", want, view)
		}
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

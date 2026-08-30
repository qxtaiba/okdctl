package lifecycle

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
)

func pressDown(s *OpStep) { _, _ = s.Update(tea.KeyPressMsg{Code: 'j', Text: "j"}) }

func TestOpStepSelectsOperations(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig()}
	s := NewOpStep(st)
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if st.Op != node.OpResize || st.Resume || st.Ack {
		t.Fatalf("default selection: Op=%v Resume=%v Ack=%v, want resize/false/false", st.Op, st.Resume, st.Ack)
	}
	pressDown(s)
	pressDown(s)
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if st.Op != node.OpRemove {
		t.Fatalf("Op = %v, want remove", st.Op)
	}
}

func TestOpStepMarkerAddsResumeOptionAndArmsAck(t *testing.T) {
	st := &State{
		Cfg:    config.DefaultConfig(),
		Marker: &node.OpMarker{Op: node.OpResize, Target: "homelab-master0", Step: node.StepPowerCycle},
	}
	s := NewOpStep(st)
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if !st.Resume || st.Op != node.OpResize || st.Ack {
		t.Fatalf("resume selection: Resume=%v Op=%v Ack=%v", st.Resume, st.Op, st.Ack)
	}
	if st.Scope.Node != "homelab-master0" {
		t.Fatalf("resume must seed the marker target into scope: %+v", st.Scope)
	}

	pressDown(s)
	pressDown(s)
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if st.Resume || st.Op != node.OpAdd || !st.Ack {
		t.Fatalf("foreign op over marker must arm Ack: Resume=%v Op=%v Ack=%v", st.Resume, st.Op, st.Ack)
	}
	if st.Scope.Node != "" {
		t.Fatalf("non-resume selection must not keep the marker scope: %+v", st.Scope)
	}
}

func TestOpStepResumeRemoveSeedsTarget(t *testing.T) {
	st := &State{
		Cfg:    config.DefaultConfig(),
		Marker: &node.OpMarker{Op: node.OpRemove, Target: "homelab-worker2", Step: node.StepDrain},
	}
	s := NewOpStep(st)
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if !st.Resume || st.Op != node.OpRemove || st.Target != "homelab-worker2" {
		t.Fatalf("resume remove: Resume=%v Op=%v Target=%q", st.Resume, st.Op, st.Target)
	}
}

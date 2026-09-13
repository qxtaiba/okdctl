package lifecycle

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
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

func TestOpStepWrapsDescriptionsToWidth(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig()}
	s := NewOpStep(st)
	out := s.View(60, 24)
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 56 {
			t.Errorf("op view line %d cols wide, want <= 56: %q", w, line)
		}
	}
	if !strings.Contains(out, "gates") {
		t.Errorf("op view must still contain the word %q after wrapping:\n%s", "gates", out)
	}
}

func TestOpStepResumeBannerUsesInjectedClock(t *testing.T) {
	marker := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	st := &State{
		Cfg:    config.DefaultConfig(),
		Marker: &node.OpMarker{Op: node.OpResize, Target: "homelab-master0", Step: node.StepPowerCycle, Timestamp: marker},
	}
	s := NewOpStep(st)
	s.now = func() time.Time { return marker.Add(2 * time.Hour) }
	out := s.View(100, 24)
	if !strings.Contains(out, "2h ago") {
		t.Errorf("resume banner must render the injected clock's age as %q:\n%s", "2h ago", out)
	}
}

func TestOpStepEntryScreenStaysCenteredAtWidth(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig()}
	m := wizard.NewFlowModel(NewSteps(st, Hooks{}), st.Cfg, lifecycleChrome())
	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 100, 30))

	var titleLine string
	for _, line := range strings.Split(frame, "\n") {
		if strings.Contains(line, "cluster lifecycle") {
			titleLine = line
			break
		}
	}
	if titleLine == "" {
		t.Fatalf("could not find the entry screen title in the rendered frame:\n%s", frame)
	}
	idx := strings.Index(titleLine, "cluster lifecycle")
	barIdx := strings.LastIndex(titleLine[:idx], "│")
	indent := idx - barIdx - 1
	if indent <= 15 {
		t.Errorf("entry screen title indent = %d, want > 15 (centered, not stretched hard-left): %q", indent, titleLine)
	}
}

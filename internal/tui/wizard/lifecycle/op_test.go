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

// TestOpStepEntryScreenFitsWithoutScrollingAt80x24 guards E-L8(d): the
// no-marker entry screen (three operations, no resume banner) is short
// enough to fit an 80x24 terminal outright — it must never force the
// operator to scroll to see "remove worker" just because a title/subtitle
// pair carried a spare blank row.
func TestOpStepEntryScreenFitsWithoutScrollingAt80x24(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig()}
	m := wizard.NewFlowModel(NewSteps(st, Hooks{}), st.Cfg, Chrome())
	frame := tuitest.RenderAt(t, m, 80, 24)
	stripped := tuitest.StripANSI(frame)

	if strings.Contains(stripped, "scroll") {
		t.Fatalf("entry screen scrolls needlessly at 80x24:\n%s", stripped)
	}
	if !strings.Contains(stripped, "(highest-numbered worker only)") {
		t.Fatalf("entry screen must show the full remove-worker description without scrolling:\n%s", stripped)
	}
}

func TestOpStepMarkerAddsResumeOptionAndArmsAck(t *testing.T) {
	st := &State{
		Cfg:    config.DefaultConfig(),
		Marker: &node.OpMarker{Op: node.OpResize, Target: "homelab-master0", Step: node.StepPowerCycle, Intent: &node.OpIntent{Scope: "/homelab-master0", MemoryMB: 24576, CPU: 4}},
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

// TestOpStepMarkerScreenWarnsAbandonmentIsPermanent guards the silent
// marker-abandonment bug: Apply arms Ack on any non-resume pick (proven
// above), which permanently overwrites the marker — but the entry screen
// must say so before the operator picks, not just record the choice.
func TestOpStepMarkerScreenWarnsAbandonmentIsPermanent(t *testing.T) {
	st := &State{
		Cfg:    config.DefaultConfig(),
		Marker: &node.OpMarker{Op: node.OpResize, Target: "homelab-master0", Step: node.StepPowerCycle, Intent: &node.OpIntent{Scope: "/homelab-master0", MemoryMB: 24576, CPU: 4}},
	}
	s := NewOpStep(st)
	out := tuitest.StripANSI(s.View(100, 30))

	if !strings.Contains(out, "abandons") {
		t.Fatalf("entry screen must warn that a non-resume pick abandons the marker:\n%s", out)
	}
	if !strings.Contains(out, "cannot be resumed") {
		t.Fatalf("entry screen must warn the abandoned marker cannot be resumed afterward:\n%s", out)
	}
}

func TestOpStepResumeRemoveSeedsTarget(t *testing.T) {
	st := &State{
		Cfg:    config.DefaultConfig(),
		Marker: &node.OpMarker{Op: node.OpRemove, Target: "homelab-worker2", Step: node.StepDrain, Intent: &node.OpIntent{Scope: "homelab-worker2"}},
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
	m := wizard.NewFlowModel(NewSteps(st, Hooks{}), st.Cfg, Chrome())
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

func TestResumeRestoresRoleAndDisruption(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Marker: &node.OpMarker{Op: node.OpResize, Target: "homelab-master1", Intent: &node.OpIntent{Scope: "master/", MemoryMB: 24576, CPU: 4, OSDiskGB: 100, RequestedMemoryMB: 24576, RequestedOSDiskGB: 100, SkipDrain: true}}}
	s := NewOpStep(st)
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if st.Scope.Role != "master" || st.Scope.Node != "" || st.MemoryMB != 24576 || st.OSDiskGB != 100 || !st.SkipDrain {
		t.Fatalf("lost approved request: %+v", st)
	}
	if NewParamsStep(st).ShouldShow(st.Cfg) {
		t.Fatal("resume can edit recorded intent")
	}
	st.Marker.Intent.DiskOnly = true
	st.Marker.Intent.RequestedMemoryMB = 0
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if st.MemoryMB != 0 || st.CPU != 0 || !st.DiskOnly() {
		t.Fatal("disk-only resume became disruptive")
	}
}

func TestLegacyMarkerOffersFreshOperationOnly(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Marker: &node.OpMarker{Op: node.OpResize}}
	s := NewOpStep(st)
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if st.Resume || !st.Ack {
		t.Fatal("legacy intent was inferred")
	}
}

func TestResumePreservesOmittedCPU(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Marker: &node.OpMarker{Op: node.OpResize, Intent: &node.OpIntent{Scope: "worker/", MemoryMB: 16384, CPU: 4, RequestedMemoryMB: 16384}}}
	s := NewOpStep(st)
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if st.MemoryMB != 16384 || st.CPU != 0 || st.OSDiskGB != 0 {
		t.Fatal("resume manufactured an unrequested CPU or disk change")
	}
}

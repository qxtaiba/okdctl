package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func doneState() *State {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "homelab"
	return &State{
		Cfg: cfg, Op: node.OpResize,
		Plan: masterResizePlan(), Proceed: true,
	}
}

func TestDoneStepRendersOutcomeAndNextSteps(t *testing.T) {
	st := doneState()
	st.Elapsed = 90 * time.Second
	s := NewDoneStep(st)
	out := s.View(90, 40)
	for _, want := range []string{"resize complete", "homelab-master0", "1m30s", "power-cycled"} {
		if !strings.Contains(out, want) {
			t.Errorf("done view missing %q:\n%s", want, out)
		}
	}
}

func TestDoneStepFailureCarriesError(t *testing.T) {
	st := doneState()
	st.Result = errors.New("etcd health gate (post-master0) failed: quorum lost")
	out := NewDoneStep(st).View(90, 40)
	if !strings.Contains(out, "quorum lost") {
		t.Errorf("failure view must carry the backend error:\n%s", out)
	}
	if !strings.Contains(out, "resume") {
		t.Errorf("failure view must point at the resume path:\n%s", out)
	}
}

// TestDoneStepCancelledRendersInterrupted pins bug 13: a graceful ctrl+c
// cancel ends on "interrupted", not on a failure card claiming the resize
// failed — the same distinction deployexec's done screen already draws.
func TestDoneStepCancelledRendersInterrupted(t *testing.T) {
	st := doneState()
	st.Result = fmt.Errorf("run resize: %w", context.Canceled)
	out := tuitest.StripANSI(NewDoneStep(st).View(90, 40))
	if !strings.Contains(out, "resize interrupted") {
		t.Fatalf("cancelled view does not say interrupted:\n%s", out)
	}
	if strings.Contains(out, "resize failed") {
		t.Fatalf("cancelled view claims the resize failed:\n%s", out)
	}
	if !strings.Contains(out, "resume") {
		t.Fatalf("cancelled view must point at the resume path:\n%s", out)
	}
}

func TestDoneStepFitsNarrowWidth(t *testing.T) {
	st := doneState()
	st.Elapsed = 90 * time.Second
	out := NewDoneStep(st).View(70, 24)
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 70 {
			t.Errorf("done view line %d cols wide, want <= 70: %q", w, line)
		}
	}
}

func TestDoneStepFailureFitsNarrowWidth(t *testing.T) {
	st := doneState()
	st.Result = errors.New("etcd health gate (post-master0) failed: quorum lost")
	out := NewDoneStep(st).View(70, 24)
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 70 {
			t.Errorf("failure view line %d cols wide, want <= 70: %q", w, line)
		}
	}
}

func TestDoneStepFailureShowsChipAndPointer(t *testing.T) {
	st := doneState()
	st.Result = errors.New("etcd health gate (post-master0) failed: quorum lost")
	out := NewDoneStep(st).View(90, 40)
	if !strings.Contains(out, "✗  resize failed") {
		t.Errorf("failure view must show the failed-op chip:\n%s", out)
	}
	if !strings.Contains(out, "→") {
		t.Errorf("failure view must point at the resume hint:\n%s", out)
	}
	if strings.Contains(out, "run_id") {
		t.Errorf("failure view must not carry the exit/run_id footer:\n%s", out)
	}
}

func TestDoneStepEnterCompletes(t *testing.T) {
	s := NewDoneStep(doneState())
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must complete the wizard")
	}
	if _, ok := cmd().(wizard.StepCompleteMsg); !ok {
		t.Fatal("want StepCompleteMsg")
	}
}

// TestDoneScreenPagesWithPgKeys mirrors the deploy flow's paging pin on its
// lifecycle sibling: the overflowing done screen's pgdn/pgup move the window.
func TestDoneScreenPagesWithPgKeys(t *testing.T) {
	tui.SetTerminalWidth(80)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	st := doneState()
	st.Elapsed = 90 * time.Second
	m := wizard.NewFlowModel(NewSteps(st, Hooks{}), st.Cfg, Chrome())
	_ = tuitest.RenderAt(t, m, 80, 24)
	m.Update(wizard.JumpToStepMsg{StepID: StepIDDone})

	before := tuitest.StripANSI(tuitest.RenderAt(t, m, 80, 24))
	if !strings.Contains(before, "scroll down for more") {
		t.Fatalf("done screen at 80x24 must overflow the viewport:\n%s", before)
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	after := tuitest.StripANSI(next.View().Content)
	if after == before {
		t.Fatal("pgdn did not move the visible window")
	}

	next, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if got := tuitest.StripANSI(next.View().Content); got != before {
		t.Fatalf("pgup did not return the window to the top:\n%s", got)
	}
}

// TestDoneScreenScrollsWithArrowKeys pins the arrowScroller opt-in beside the
// pgup/pgdn pin above: on the lifecycle done screen ↑/↓ move the viewport one
// line, exactly as the footer's arrow glyphs promise.
func TestDoneScreenScrollsWithArrowKeys(t *testing.T) {
	tui.SetTerminalWidth(80)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	st := doneState()
	st.Elapsed = 90 * time.Second
	m := wizard.NewFlowModel(NewSteps(st, Hooks{}), st.Cfg, Chrome())
	_ = tuitest.RenderAt(t, m, 80, 24)
	m.Update(wizard.JumpToStepMsg{StepID: StepIDDone})

	before := tuitest.StripANSI(tuitest.RenderAt(t, m, 80, 24))
	if !strings.Contains(before, "scroll down for more") {
		t.Fatalf("done screen at 80x24 must overflow the viewport:\n%s", before)
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	after := tuitest.StripANSI(next.View().Content)
	if after == before {
		t.Fatal("↓ did not move the visible window")
	}

	next, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := tuitest.StripANSI(next.View().Content); got != before {
		t.Fatalf("↑ did not return the window to the top:\n%s", got)
	}
}

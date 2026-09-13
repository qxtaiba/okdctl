package lifecycle

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func doneState() *State {
	return &State{
		Cfg: config.DefaultConfig(), Op: node.OpResize,
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

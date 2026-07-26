package lifecycle

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func TestDoneStepRendersOutcomeAndNextSteps(t *testing.T) {
	st := &State{
		Cfg: config.DefaultConfig(), Op: node.OpResize,
		Plan: masterResizePlan(), Proceed: true, Elapsed: 90 * time.Second,
	}
	s := NewDoneStep(st)
	out := s.View(90, 40)
	for _, want := range []string{"resize complete", "homelab-master0", "1m30s", "power-cycled"} {
		if !strings.Contains(out, want) {
			t.Errorf("done view missing %q:\n%s", want, out)
		}
	}
}

func TestDoneStepFailureCarriesError(t *testing.T) {
	st := &State{
		Cfg: config.DefaultConfig(), Op: node.OpResize,
		Plan: masterResizePlan(), Proceed: true,
		Result: errors.New("etcd health gate (post-master0) failed: quorum lost"),
	}
	out := NewDoneStep(st).View(90, 40)
	if !strings.Contains(out, "quorum lost") {
		t.Errorf("failure view must carry the backend error:\n%s", out)
	}
	if !strings.Contains(out, "resume") {
		t.Errorf("failure view must point at the resume path:\n%s", out)
	}
}

func TestDoneStepEnterCompletes(t *testing.T) {
	st := &State{
		Cfg: config.DefaultConfig(), Op: node.OpResize,
		Plan: masterResizePlan(), Proceed: true,
	}
	s := NewDoneStep(st)
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must complete the wizard")
	}
	if _, ok := cmd().(wizard.StepCompleteMsg); !ok {
		t.Fatal("want StepCompleteMsg")
	}
}

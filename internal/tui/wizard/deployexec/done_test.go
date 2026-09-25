package deployexec

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func doneState() *State {
	st := streamState()
	st.Started, st.Executed = true, true
	st.Elapsed = 42 * time.Minute
	st.Summary = &postinstall.Result{BootstrapCleaned: true, DNSDeployed: true, KubeVipIP: DemoKubeVipIP}
	st.Steps = []distribution.StepResult{
		{StepID: install.StepDeployInfra, Success: true, Duration: 90 * time.Second},
		{StepID: install.StepWaitBootstrap, Success: true, Duration: 12 * time.Minute},
	}
	return st
}

func TestDoneViewRendersTheDeploySummaryBoxInFrame(t *testing.T) {
	s := NewDoneStep(doneState(), Hooks{})
	out := tuitest.StripANSI(s.View(96, 40))

	for _, want := range []string{"DEPLOYMENT COMPLETE", "cluster deployed", "console", "kubeadmin"} {
		if !strings.Contains(out, want) {
			t.Errorf("done view missing %q:\n%s", want, out)
		}
	}
}

func TestDoneViewFitsANarrowerFrame(t *testing.T) {
	s := NewDoneStep(doneState(), Hooks{})
	const width = 70
	tuitest.AssertFits(t, s.View(width, 40), width, 0)
}

func TestDoneViewRendersTheErrorCardOnFailure(t *testing.T) {
	st := doneState()
	st.Result = errors.New("terraform apply failed: vm 9001 already exists")
	out := tuitest.StripANSI(NewDoneStep(st, Hooks{}).View(96, 40))

	if !strings.Contains(out, "deploy failed") {
		t.Errorf("failure view must lead with the failure kind:\n%s", out)
	}
	if !strings.Contains(out, "vm 9001 already exists") {
		t.Errorf("failure view must carry the engine error:\n%s", out)
	}
	if !strings.Contains(out, "okdctl deploy") {
		t.Errorf("failure view must carry the resume hint:\n%s", out)
	}
}

func TestDoneViewNamesACancelledRunInterrupted(t *testing.T) {
	st := doneState()
	st.Result = context.Canceled
	if out := tuitest.StripANSI(NewDoneStep(st, Hooks{}).View(96, 40)); !strings.Contains(out, "deploy interrupted") {
		t.Errorf("a cancelled run must read as interrupted, not failed:\n%s", out)
	}
}

func TestDoneCompletesOnEnter(t *testing.T) {
	s := NewDoneStep(doneState(), Hooks{})
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must complete the flow")
	}
	if _, ok := cmd().(wizard.StepCompleteMsg); !ok {
		t.Errorf("enter must emit StepCompleteMsg, got %T", cmd())
	}
}

// TestDoneScreenPagesWithPgKeys pins the paging contract the footer
// advertises: at 80x24 the done screen overflows the viewport, and pgdn/pgup
// move the visible window rather than being swallowed anywhere between the
// key decode and the frame's viewport.
func TestDoneScreenPagesWithPgKeys(t *testing.T) {
	tui.SetTerminalWidth(80)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	st := doneState()
	m := wizard.NewFlowModel(NewSteps(st, goldenHooks()), st.Cfg, Chrome())
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

// errStreamFailed is the engine failure the done-screen tests render.
var errStreamFailed = errors.New("deploy infrastructure failed: proxmox task refused")

// TestDoneScreenScrollsWithArrowKeys pins the arrowScroller opt-in beside the
// pgup/pgdn pin above: on the read-only done screen ↑/↓ move the viewport one
// line, exactly as the footer's arrow glyphs promise.
func TestDoneScreenScrollsWithArrowKeys(t *testing.T) {
	tui.SetTerminalWidth(80)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	st := doneState()
	m := wizard.NewFlowModel(NewSteps(st, goldenHooks()), st.Cfg, Chrome())
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

// TestDoneFailureScreenPagesTheLogRegion pins failure navigability: digging
// into the error must not require having pressed f before the run died —
// pgup on the failure screen pages the log region back through the ring, and
// pgdn returns it to the tail.
func TestDoneFailureScreenPagesTheLogRegion(t *testing.T) {
	tui.SetTerminalWidth(80)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	st := doneState()
	st.Result = errStreamFailed
	m := wizard.NewFlowModel(NewSteps(st, Hooks{Logs: seededRing(40)}), st.Cfg, Chrome())
	_ = tuitest.RenderAt(t, m, 80, 24)
	m.Update(wizard.JumpToStepMsg{StepID: StepIDDone})

	before := tuitest.StripANSI(tuitest.RenderAt(t, m, 80, 24))
	if !strings.Contains(before, "step-34") {
		t.Fatalf("the failure tail must be following the newest window:\n%s", before)
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	paged := tuitest.StripANSI(tuitest.RenderAt(t, m, 80, 24))
	if strings.Contains(paged, "step-34") || !strings.Contains(paged, "of 40") {
		t.Fatalf("pgup must page the log region back with honest coordinates:\n%s", paged)
	}

	for range 12 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	if top := tuitest.StripANSI(tuitest.RenderAt(t, m, 80, 24)); !strings.Contains(top, "step-00") {
		t.Errorf("paging to the top must reach the ring's first line:\n%s", top)
	}

	for range 20 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	if got := tuitest.StripANSI(tuitest.RenderAt(t, m, 80, 24)); !strings.Contains(got, "step-34") {
		t.Errorf("paging back down must return the tail:\n%s", got)
	}
}

// TestDoneFailureCarriesTheSinkPath pins the escape hatch on the failure
// screen: the tail is a window, the named file keeps every byte.
func TestDoneFailureCarriesTheSinkPath(t *testing.T) {
	st := doneState()
	st.Result = errStreamFailed
	s := NewDoneStep(st, Hooks{Logs: seededRing(8), LogPath: "okd-install/okdctl.log"})
	s.SetTerminalSize(100, 30)

	if out := tuitest.StripANSI(s.View(96, 1000)); !strings.Contains(out, "full log okd-install/okdctl.log") {
		t.Errorf("failure view must name the sink path:\n%s", out)
	}

	bare := NewDoneStep(st, Hooks{Logs: seededRing(8)})
	bare.SetTerminalSize(100, 30)
	if out := tuitest.StripANSI(bare.View(96, 1000)); strings.Contains(out, "full log") {
		t.Errorf("with no sink open there is no path to point at:\n%s", out)
	}
}

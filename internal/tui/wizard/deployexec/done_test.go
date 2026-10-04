package deployexec

import (
	"context"
	"errors"
	"image/color"
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

func TestDoneViewRendersTheDeployFinishScreen(t *testing.T) {
	s := NewDoneStep(doneState(), Hooks{})
	out := tuitest.StripANSI(s.View(96, 40))

	for _, want := range []string{"D E P L O Y E D", "homelab.lab.example.com", "console", "kubeadmin", "NEXT", "oc login"} {
		if !strings.Contains(out, want) {
			t.Errorf("done view missing %q:\n%s", want, out)
		}
	}
}

type finishTestStep struct{}

func (finishTestStep) ID() wizard.StepID                           { return "finish-test" }
func (finishTestStep) Title() string                               { return "finish test" }
func (finishTestStep) Init() tea.Cmd                               { return nil }
func (finishTestStep) Update(tea.Msg) (wizard.WizardStep, tea.Cmd) { return finishTestStep{}, nil }
func (finishTestStep) View(int, int) string                        { return "finish test" }

func TestDoneFinishVerbsReachTheirProviders(t *testing.T) {
	status := func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
		return []wizard.WizardStep{finishTestStep{}}, wizard.FlowChrome{Tagline: "status"}, nil
	}
	manage := func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
		return []wizard.WizardStep{finishTestStep{}}, wizard.FlowChrome{Tagline: "manage"}, nil
	}
	open := false
	s := NewDoneStep(doneState(), Hooks{
		Finish: &FinishHooks{ClusterStatus: status, ManageNodes: manage, OpenConsole: func() tea.Cmd {
			return func() tea.Msg { open = true; return nil }
		}},
	})

	for _, key := range []struct {
		text string
		want string
	}{{"s", "cluster status"}, {"n", "manage nodes"}, {"o", "open console"}} {
		if !strings.Contains(strings.ToLower(tuitest.StripANSI(s.View(120, 40))), key.want) {
			t.Errorf("finish screen omits %q", key.want)
		}
	}

	for _, key := range []struct {
		text string
		want string
	}{{"s", "status"}, {"n", "manage"}} {
		_, cmd := s.Update(tea.KeyPressMsg{Text: key.text})
		if cmd == nil {
			t.Fatalf("%q returned no flow command", key.text)
		}
		swap, ok := cmd().(wizard.SwapFlowMsg)
		if !ok || swap.Chrome.Tagline != key.want || len(swap.Steps) != 1 {
			t.Errorf("%q returned %#v, want the %q flow", key.text, swap, key.want)
		}
	}
	_, cmd := s.Update(tea.KeyPressMsg{Text: "o"})
	if cmd == nil {
		t.Fatal("o returned no console command")
	}
	cmd()
	if !open {
		t.Error("o did not call the console opener")
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

// TestDoneViewSanitizesHostileEngineErrorText drives a deploy-engine error
// whose Error() text carries an OSC sequence (the window-title/clipboard
// family) through the incident report's real View path. The error reaches
// the screen twice — the error card (failureMessage) and the incident
// facts' "cause" row (leadingClause) — so this also proves neither call
// site was missed.
func TestDoneViewSanitizesHostileEngineErrorText(t *testing.T) {
	const payload = "\x1b]0;pwned\x07"

	st := doneState()
	st.Result = errors.New("terraform apply failed" + payload + ": vm 9001 already exists")
	out := NewDoneStep(st, Hooks{}).View(96, 40)

	if strings.Contains(out, payload) {
		t.Fatalf("incident report carries the raw osc payload:\n%q", out)
	}
	if strings.Contains(out, "pwned") {
		t.Fatalf("osc payload text leaked into the incident report:\n%q", out)
	}
	if !strings.Contains(out, "�") {
		t.Fatalf("incident report shows no sanitization marker:\n%q", out)
	}
	if !strings.Contains(out, "vm 9001 already exists") {
		t.Fatalf("incident report lost the legitimate engine error text:\n%q", out)
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

func TestFinishMotionIsOneShotAndFullMotionOnly(t *testing.T) {
	prev := tui.Motion()
	t.Cleanup(func() { tui.SetMotion(prev) })
	tui.SetMotion(tui.MotionFull)
	s := NewDoneStep(doneState(), Hooks{})
	s.Init()
	if !s.Animating() {
		t.Fatal("successful finish must animate once under full motion")
	}
	base := finishGradient(0)
	for frame := uint64(1); frame < finishAnimationFrames; frame++ {
		_, _ = s.Update(wizard.FrameMsg{Frame: frame})
		if !s.Animating() {
			t.Fatalf("animation stopped at frame %d", frame)
		}
		highlight := int((frame - 1) * uint64(len(base)-1) / uint64(finishAnimationFrames-1)) //nolint:gosec // the test iterates only the fixed animation frame range.
		if equalColor(finishGradient(frame)[highlight], base[highlight]) {
			t.Fatalf("frame %d did not sweep the wordmark", frame)
		}
	}
	_, _ = s.Update(wizard.FrameMsg{Frame: finishAnimationFrames})
	if s.Animating() || s.frame != 0 || !equalGradient(finishGradient(0), finishGradient(s.frame)) {
		t.Fatal("finish motion must stop on the original still frame")
	}
	s.Init()
	if s.Animating() {
		t.Fatal("returning from a chained flow must not replay the finish motion")
	}

	tui.SetMotion(tui.MotionReduced)
	s = NewDoneStep(doneState(), Hooks{})
	s.Init()
	if s.Animating() {
		t.Fatal("reduced motion must keep the finish screen still")
	}
}

func equalGradient(a, b []color.Color) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalColor(a[i], b[i]) {
			return false
		}
	}
	return true
}

func equalColor(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
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

	// The keys go through the frame, so its own paging gate is under test; the
	// assertions read the step's body directly, since the incident report is
	// taller than an 80x24 viewport and its evidence rides below the fold.
	report := func() string {
		s, ok := m.CurrentStep().(*DoneStep)
		if !ok {
			t.Fatalf("current step is %T, want the done screen", m.CurrentStep())
		}
		return tuitest.StripANSI(s.View(76, 1000))
	}

	if before := report(); !strings.Contains(before, "step-34") {
		t.Fatalf("the failure tail must be following the newest window:\n%s", before)
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	paged := report()
	if strings.Contains(paged, "step-34") || !strings.Contains(paged, "of 40") {
		t.Fatalf("pgup must page the log region back with honest coordinates:\n%s", paged)
	}

	for range 12 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	if top := report(); !strings.Contains(top, "step-00") {
		t.Errorf("paging to the top must reach the ring's first line:\n%s", top)
	}

	for range 20 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	if got := report(); !strings.Contains(got, "step-34") {
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
	if out := tuitest.StripANSI(bare.View(96, 1000)); strings.Contains(out, "full log okd-install") {
		t.Errorf("with no sink open there is no path to point at:\n%s", out)
	}
}

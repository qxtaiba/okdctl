package wizard

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

// hubStep stands in for the hero-hub: confirming emits the SwapFlowMsg its
// manage verb would, building the sub-flow fresh on every entry the way the
// real hub's HubFlow providers do.
type hubStep struct {
	fakeStep
	build  func() []WizardStep
	builds int
}

func (h *hubStep) Update(msg tea.Msg) (WizardStep, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok && key.Code == tea.KeyEnter {
		h.builds++
		flow := h.build()
		return h, func() tea.Msg { return SwapFlowMsg{Steps: flow, Chrome: FlowChrome{Tagline: "sub-flow"}} }
	}
	return h, nil
}

// opLikeStep mimics the lifecycle flow's entry screen: a HelpProvider whose own
// ribbon binds no esc, so the model's hub hint has somewhere to appear.
type opLikeStep struct {
	fakeStep
}

func (o *opLikeStep) ShortHelp() []KeyBinding {
	return []KeyBinding{
		{Key: "↑↓", Help: "select"},
		{Key: HelpEnter, Help: HelpConfirm},
		{Key: HelpCtrlC, Help: HelpQuit},
	}
}

// escKey and enterKey are the two keystrokes the hub round-trip is driven by.
var (
	escKey   = tea.KeyPressMsg{Code: tea.KeyEsc}
	enterKey = tea.KeyPressMsg{Code: tea.KeyEnter}
)

// swapModel builds a one-step hub whose sub-flow is two fresh steps per entry.
func swapModel(t *testing.T) (*Model, *hubStep) {
	t.Helper()
	hub := &hubStep{fakeStep: fakeStep{id: StepIDWelcome}}
	hub.build = func() []WizardStep {
		return []WizardStep{&opLikeStep{fakeStep{id: "sub-first"}}, &fakeStep{id: "sub-second"}}
	}
	m := NewFlowModel([]WizardStep{hub}, config.DefaultConfig(), DefaultChrome())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m, hub
}

// swapIn drives the hub's confirm key and delivers the SwapFlowMsg its command
// produces, the way the tea runtime would.
func swapIn(t *testing.T, m *Model) {
	t.Helper()
	_, cmd := m.Update(enterKey)
	if cmd == nil {
		t.Fatal("confirming the hub produced no command")
	}
	msg := cmd()
	if _, ok := msg.(SwapFlowMsg); !ok {
		t.Fatalf("hub command produced %T, want SwapFlowMsg", msg)
	}
	m.Update(msg)
}

func TestSwapFlowEntersTheSubFlowAtItsFirstStep(t *testing.T) {
	m, _ := swapModel(t)

	swapIn(t, m)

	if got := m.CurrentStep().ID(); got != "sub-first" {
		t.Errorf("current step after swap = %q, want sub-first", got)
	}
	if got := m.chrome.Tagline; got != "sub-flow" {
		t.Errorf("chrome tagline after swap = %q, want the sub-flow's", got)
	}
}

func TestSwapFlowEscapeFromFirstStepReturnsToTheHub(t *testing.T) {
	m, _ := swapModel(t)
	swapIn(t, m)

	m.Update(escKey)

	if got := m.CurrentStep().ID(); got != StepIDWelcome {
		t.Fatalf("current step after esc = %q, want the hub", got)
	}
	if m.suspended != nil {
		t.Error("returning to the hub must clear the suspended flow")
	}
	if got := m.chrome.Tagline; got != DefaultChrome().Tagline {
		t.Errorf("chrome tagline after return = %q, want the hub's", got)
	}
	if !m.CurrentStep().(*hubStep).focused {
		t.Error("the hub must regain focus on return")
	}
}

func TestSwapFlowEscapeWithinTheSubFlowStepsBackFirst(t *testing.T) {
	m, _ := swapModel(t)
	swapIn(t, m)

	m.Update(StepCompleteMsg{StepID: "sub-first"})
	if got := m.CurrentStep().ID(); got != "sub-second" {
		t.Fatalf("current step = %q, want sub-second", got)
	}

	m.Update(escKey)
	if got := m.CurrentStep().ID(); got != "sub-first" {
		t.Fatalf("esc inside the sub-flow = %q, want a step back to sub-first", got)
	}
	if m.suspended == nil {
		t.Error("stepping back inside the sub-flow must keep the hub suspended")
	}

	m.Update(escKey)
	if got := m.CurrentStep().ID(); got != StepIDWelcome {
		t.Errorf("esc at the sub-flow's first step = %q, want the hub", got)
	}
}

func TestSwapFlowRebuildsStepsOnEveryEntry(t *testing.T) {
	m, hub := swapModel(t)

	swapIn(t, m)
	first := m.CurrentStep()
	m.Update(StepCompleteMsg{StepID: "sub-first"})
	m.Update(escKey)
	m.Update(escKey)

	swapIn(t, m)
	second := m.CurrentStep()

	if hub.builds != 2 {
		t.Errorf("sub-flow built %d times, want one build per entry", hub.builds)
	}
	if first == second {
		t.Error("re-entering reused the previous flow's step instance; state from the swapped-out flow can bleed into the next entry")
	}
	if got := m.CurrentStep().ID(); got != "sub-first" {
		t.Errorf("re-entry landed on %q, want the sub-flow's first step", got)
	}
}

func TestEscapeWithNoSuspendedFlowIsInert(t *testing.T) {
	m, _ := swapModel(t)

	m.Update(escKey)

	if got := m.CurrentStep().ID(); got != StepIDWelcome {
		t.Errorf("current step = %q, want the hub unchanged", got)
	}
}

func TestSwapFlowFooterAdvertisesTheHubEscape(t *testing.T) {
	m, _ := swapModel(t)

	if hasEscBinding(m.footerBindings()) {
		t.Error("the hub itself must not advertise an esc-to-hub binding")
	}

	swapIn(t, m)
	if !hasEscBinding(m.footerBindings()) {
		t.Error("a swapped-in flow's first screen must advertise the esc round-trip")
	}

	m.Update(StepCompleteMsg{StepID: "sub-first"})
	for _, b := range m.footerBindings() {
		if b.Key == HelpEsc && b.Help == "hub" {
			t.Error("deeper in the sub-flow esc steps back, not to the hub; the ribbon must not claim otherwise")
		}
	}
}

// hasEscBinding reports whether bindings advertise esc as the hub round-trip.
func hasEscBinding(bindings []KeyBinding) bool {
	return escHelp(bindings) == "hub"
}

// escHelp returns the Help text of bindings' esc entry, or "" if none.
func escHelp(bindings []KeyBinding) string {
	for _, b := range bindings {
		if b.Key == HelpEsc {
			return b.Help
		}
	}
	return ""
}

// TestSwapFlowFromANonHubScreenAdvertisesBackNotHub pins the chained-flow
// case the hub round-trip assumed away: a flow swapped in from a screen
// that is not itself the hub (deployexec's finish screen opening cluster
// status via its "s" key, say) returns esc to that screen, not to the
// five-verb hub — the footer must say so rather than reusing the hub's own
// wording, which a step's own ShortHelp cannot know to avoid on its own.
func TestSwapFlowFromANonHubScreenAdvertisesBackNotHub(t *testing.T) {
	origin := &fakeStep{id: "deploy-done"}
	m := NewFlowModel([]WizardStep{origin}, config.DefaultConfig(), DefaultChrome())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	sub := &opLikeStep{fakeStep{id: "status-first"}}
	mm, _ := m.Update(SwapFlowMsg{Steps: []WizardStep{sub}, Chrome: FlowChrome{Tagline: "status"}})
	m = mm.(*Model)

	if got := m.CurrentStep().ID(); got != "status-first" {
		t.Fatalf("current step after swap = %q, want status-first", got)
	}
	if help := escHelp(m.footerBindings()); help == "hub" {
		t.Error("esc from a flow chained off a non-hub screen must not claim it returns to the hub")
	}

	m.Update(escKey)
	if got := m.CurrentStep().ID(); got != "deploy-done" {
		t.Errorf("esc = %q, want back to the origin screen deploy-done", got)
	}
}

// The fake hub here carries no confirm guard of its own, so this pins the
// model's own defence: a swap arriving while a flow is already live is dropped
// rather than overwriting the one return target with the flow it displaces.
func TestSwapFlowRefusesASecondSwapWhileOneIsLive(t *testing.T) {
	m, _ := swapModel(t)

	// Two enters land before the first flow's command has been delivered —
	// the window the "opening …" notice covers in the real hub.
	_, first := m.Update(enterKey)
	_, second := m.Update(enterKey)

	m.Update(first())
	firstFlow := m.CurrentStep()
	if second != nil {
		m.Update(second())
	}

	if got := m.CurrentStep(); got != firstFlow {
		t.Errorf("the second swap displaced the live flow: current step = %q", got.ID())
	}

	m.Update(escKey)
	if got := m.CurrentStep().ID(); got != StepIDWelcome {
		t.Fatalf("esc after a double enter = %q, want the hub — the operator must never be stranded", got)
	}

	swapIn(t, m)
	if got := m.CurrentStep().ID(); got != "sub-first" {
		t.Errorf("re-entry after a double enter = %q, want the sub-flow's first step", got)
	}
}

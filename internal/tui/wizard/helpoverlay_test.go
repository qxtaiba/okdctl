package wizard

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

// textConsumingStep is a TextInputConsumer double: consuming toggles whether
// "?" should be treated as typed text (true) or the help-overlay toggle
// (false), and every Update call is recorded so tests can assert whether a
// keypress reached the step or was swallowed by the wizard.
type textConsumingStep struct {
	nopStep
	consuming bool
	received  []tea.Msg
}

func (s *textConsumingStep) ConsumesTextInput() bool { return s.consuming }

func (s *textConsumingStep) Update(msg tea.Msg) (WizardStep, tea.Cmd) {
	s.received = append(s.received, msg)
	return s, nil
}

func questionMark() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: '?', Text: "?"}
}

func TestHelpOverlay_TogglesOnQuestionMark(t *testing.T) {
	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)

	if m.helpOpen {
		t.Fatal("overlay must start closed")
	}

	m = update(t, m, questionMark())
	if !m.helpOpen {
		t.Fatal("? must open the overlay")
	}

	m = update(t, m, questionMark())
	if m.helpOpen {
		t.Fatal("? must close the overlay when already open")
	}
}

func TestHelpOverlay_EscClosesWithoutNavigatingBack(t *testing.T) {
	first, second := newNopStep(), newNopStep()
	m := NewModel([]WizardStep{first, second}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)
	m.currentStep = 1

	m = update(t, m, questionMark())
	if !m.helpOpen {
		t.Fatal("setup: overlay should be open")
	}

	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.helpOpen {
		t.Fatal("esc must close the open overlay")
	}
	if m.currentStep != 1 {
		t.Fatalf("esc that closed the overlay must not also navigate back, currentStep=%d", m.currentStep)
	}
}

func TestHelpOverlay_FocusedTextInputPassesQuestionMarkThrough(t *testing.T) {
	s := &textConsumingStep{nopStep: *newNopStep(), consuming: true}
	m := NewModel([]WizardStep{s}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)

	m = update(t, m, questionMark())
	if m.helpOpen {
		t.Fatal("? must type into a focused text field, not open the overlay")
	}
	if len(s.received) != 1 {
		t.Fatalf("expected the keypress forwarded to the step, got %d messages", len(s.received))
	}
	if got, ok := s.received[0].(tea.KeyPressMsg); !ok || got.Text != "?" {
		t.Fatalf("forwarded message = %#v, want the \"?\" keypress", s.received[0])
	}
}

func TestHelpOverlay_NonConsumingStepOpensOverlay(t *testing.T) {
	s := &textConsumingStep{nopStep: *newNopStep(), consuming: false}
	m := NewModel([]WizardStep{s}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)

	m = update(t, m, questionMark())
	if !m.helpOpen {
		t.Fatal("? must open the overlay when the step isn't consuming text input")
	}
	if len(s.received) != 0 {
		t.Fatalf("the opening keypress must not also reach the step, got %v", s.received)
	}
}

// TestHelpOverlay_UnassertedStepAlwaysOpens pins the documented default:
// nopStep doesn't implement TextInputConsumer at all, so "?" always opens
// the overlay there.
func TestHelpOverlay_UnassertedStepAlwaysOpens(t *testing.T) {
	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)

	m = update(t, m, questionMark())
	if !m.helpOpen {
		t.Fatal("a step with no TextInputConsumer opinion must default to opening the overlay")
	}
}

func TestHelpOverlay_InertWhileOpen(t *testing.T) {
	s := &textConsumingStep{nopStep: *newNopStep(), consuming: false}
	second := newNopStep()
	m := NewModel([]WizardStep{s, second}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)

	m = update(t, m, questionMark())
	if !m.helpOpen {
		t.Fatal("setup: overlay should be open")
	}
	s.received = nil

	for _, msg := range []tea.Msg{
		tea.KeyPressMsg{Code: tea.KeyDown},
		tea.KeyPressMsg{Code: tea.KeyEnter},
		tea.KeyPressMsg{Code: tea.KeyTab},
		tea.KeyPressMsg{Code: tea.KeyPgUp},
		tea.KeyPressMsg{Code: 'a', Text: "a"},
	} {
		m = update(t, m, msg)
	}

	if !m.helpOpen {
		t.Fatal("only esc/? may close the overlay")
	}
	if m.currentStep != 0 {
		t.Fatalf("navigation keys must be inert while the overlay is open, currentStep=%d", m.currentStep)
	}
	if len(s.received) != 0 {
		t.Fatalf("no keys but esc/?/ctrl+c may reach the step while the overlay is open, got %v", s.received)
	}
}

func TestHelpOverlay_CtrlCStillQuitsWhileOpen(t *testing.T) {
	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)

	m = update(t, m, questionMark())
	if !m.helpOpen {
		t.Fatal("setup: overlay should be open")
	}

	m = update(t, m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !m.quitting {
		t.Fatal("ctrl+c must still quit while the overlay is open")
	}
}

func TestHelpOverlay_QuitGuardInterceptionClosesOverlay(t *testing.T) {
	g := &guardedStep{nopStep: *newNopStep(), intercepts: true}
	m := NewModel([]WizardStep{g}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)

	m = update(t, m, questionMark())
	if !m.helpOpen {
		t.Fatal("setup: overlay should be open")
	}

	m = update(t, m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if m.quitting {
		t.Fatal("guarded step must still prevent quit on first ctrl+c")
	}
	if m.helpOpen {
		t.Fatal("a guard's own feedback needs the viewport back — the overlay must close")
	}
}

func TestHelpOverlay_ReplacesOnlyTheViewportRegion(t *testing.T) {
	s := &titledStep{nopStep: *newNopStep(), title: "welcome"}
	m := NewModel([]WizardStep{s}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)

	m = update(t, m, questionMark())
	frame := tuitest.StripANSI(m.View().Content)

	if !strings.Contains(frame, "welcome") {
		t.Fatalf("header must still render while the overlay is open:\n%s", frame)
	}
	if !strings.Contains(frame, "step 1 of 1") {
		t.Fatalf("header trail must still render while the overlay is open:\n%s", frame)
	}
	if strings.Contains(frame, "body") {
		t.Fatalf("the step's own body must not render under the overlay:\n%s", frame)
	}
	tuitest.AssertFits(t, m.View().Content, 100, 30)
}

// wideHelpStep advertises far more bindings than an 80-column ribbon can
// show, so the footer truncates — the overlay must still list every one.
type wideHelpStep struct{ nopStep }

func (s *wideHelpStep) ShortHelp() []KeyBinding {
	return []KeyBinding{
		{Key: "↑↓", Help: "navigate rows"},
		{Key: "tab", Help: "next section entirely"},
		{Key: "space", Help: "toggle the current selection"},
		{Key: "a", Help: "add a brand new row"},
		{Key: "d", Help: "delete the current row"},
		{Key: HelpEnter, Help: "continue to the next step"},
		{Key: HelpEsc, Help: HelpBack},
		{Key: HelpCtrlC, Help: HelpQuit},
	}
}

func TestHelpOverlay_ListsBindingsTheRibbonTruncated(t *testing.T) {
	s := &wideHelpStep{nopStep: *newNopStep()}
	m := NewModel([]WizardStep{s}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 80, 24)

	ribbon := tuitest.StripANSI(m.renderHelpRow())
	for _, want := range []string{"add a brand new row", "delete the current row"} {
		if strings.Contains(ribbon, want) {
			t.Fatalf("setup: expected the narrow ribbon to have already dropped %q: %q", want, ribbon)
		}
	}

	m = update(t, m, questionMark())
	frame := tuitest.StripANSI(m.View().Content)

	for _, want := range []string{
		"navigate rows", "next section entirely", "toggle the current selection",
		"add a brand new row", "delete the current row", "continue to the next step",
		HelpBack, HelpQuit, HelpOverlay,
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("overlay missing binding %q that the ribbon truncated:\n%s", want, frame)
		}
	}
	tuitest.AssertFits(t, m.View().Content, 80, 24)
}

func TestHelpOverlay_FitsAtEverySize(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {100, 30}, {120, 40}} {
		s := &wideHelpStep{nopStep: *newNopStep()}
		m := NewModel([]WizardStep{s}, config.DefaultConfig())
		tuitest.RenderAt(t, m, sz[0], sz[1])

		m = update(t, m, questionMark())
		frame := m.View().Content
		tuitest.AssertFits(t, frame, sz[0], sz[1])
	}
}

func TestFooterBindings_AlwaysAdvertisesHelp(t *testing.T) {
	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)

	bindings := m.footerBindings()
	last := bindings[len(bindings)-1]
	if last.Key != HelpQuestion || last.Help != HelpOverlay {
		t.Fatalf("footerBindings must end with the %q hint, got %+v", HelpQuestion, last)
	}
}

// TestHelpOverlay_ListsVimGroupFooterSilently pins the vim vocabulary's home:
// absent from the footer ribbon, present in the overlay under its own group.
func TestHelpOverlay_ListsVimGroup(t *testing.T) {
	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)

	if ribbon := tuitest.StripANSI(m.renderHelpRow()); strings.Contains(ribbon, "gg/G") {
		t.Fatalf("footer ribbon advertises the vim keys: %q", ribbon)
	}

	m = update(t, m, questionMark())
	frame := tuitest.StripANSI(m.View().Content)
	for _, want := range []string{"vim", "j/k", "ctrl+d/u", "gg/G"} {
		if !strings.Contains(frame, want) {
			t.Errorf("overlay missing %q from the vim group:\n%s", want, frame)
		}
	}
}

// overlayOnlyStep is an OverlayHelpProvider double: it advertises "x extra"
// only to the "?" overlay, never via ShortHelp, mirroring how a step reuses
// a keystroke across phases without repeating it in the footer ribbon.
type overlayOnlyStep struct{ nopStep }

func (s *overlayOnlyStep) OverlayHelp() []KeyBinding {
	return []KeyBinding{{Key: "x", Help: "extra"}}
}

// TestHelpOverlay_ListsStepOverlayHelpFooterSilently pins the overlay's
// aggregation of a step's own OverlayHelpProvider bindings: absent from the
// footer ribbon (ShortHelp never saw them), present in the overlay's screen
// section alongside the step's regular bindings.
func TestHelpOverlay_ListsStepOverlayHelpFooterSilently(t *testing.T) {
	s := &overlayOnlyStep{nopStep: *newNopStep()}
	m := NewModel([]WizardStep{s}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)

	if ribbon := tuitest.StripANSI(m.renderHelpRow()); strings.Contains(ribbon, "extra") {
		t.Fatalf("footer ribbon advertises the overlay-only binding: %q", ribbon)
	}

	m = update(t, m, questionMark())
	frame := tuitest.StripANSI(m.View().Content)
	if !strings.Contains(frame, "extra") {
		t.Errorf("overlay missing the step's OverlayHelp binding:\n%s", frame)
	}
}

// TestHelpOverlay_StepWithoutOverlayHelpListsNothingExtra is the negative
// case: a step that implements neither ShortHelp's "x" entry nor
// OverlayHelpProvider must not have "x" appear in the overlay at all — the
// aggregation must not invent bindings a step never declared.
func TestHelpOverlay_StepWithoutOverlayHelpListsNothingExtra(t *testing.T) {
	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)

	m = update(t, m, questionMark())
	frame := tuitest.StripANSI(m.View().Content)
	if strings.Contains(frame, "extra") {
		t.Fatalf("overlay lists a binding the current step never declared:\n%s", frame)
	}
}

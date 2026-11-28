package wizard

import (
	"context"
	"os"
	"sync"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

const (
	minTerminalWidth  = 60
	minTerminalHeight = 20

	headerHeight = 3 // brand row + title/trail row + bottom rule
	statusHeight = 1
	// scroll-indicator line plus help bar
	footerHeight         = 2
	outerVerticalPadding = 4 // wizard border (2) + outer padding (2)

	outerHorizontalPadding = 4

	wizardBorderHorizontal = 2

	fixedLayoutOverhead = headerHeight + statusHeight + footerHeight + outerVerticalPadding

	// singleFormMaxWidth keeps a readable column inside a full-width frame.
	singleFormMaxWidth = 110

	// formMaxWidth is the measure FieldWidthAuto and FieldWidthPath derive from.
	formMaxWidth = 104
)

type earlyExiter interface {
	ShouldExitEarly() bool
}

type centerable interface {
	IsCentered() bool
}

// heroRenderer is implemented by steps that draw the wordmark themselves,
// so the frame drops its header.
type heroRenderer interface {
	RendersHero() bool
}

// frameWidthOwner is implemented by steps that render across the frame's
// whole width instead of the capped form measure.
type frameWidthOwner interface {
	OwnsFrameWidth() bool
}

// displayTitler is implemented by steps with a header prompt distinct from
// Title().
type displayTitler interface {
	DisplayTitle() string
}

// Model is the bubbletea model backing the configuration wizard.
type Model struct {
	workerMu    sync.Mutex
	workers     sync.WaitGroup
	closing     bool
	flowContext context.Context
	cancelVisit context.CancelFunc
	generation  uint64
	width       int
	height      int

	viewport viewport.Model
	ready    bool

	// contentRows[i] is the viewport row View line i starts on (a wide line
	// wraps into several rows); length is line count + 1 and only non-centered
	// steps are mapped.
	contentRows []int

	steps       []WizardStep
	currentStep int

	// returnToReview routes next/previous back to review after a jump.
	returnToReview bool

	config *config.Config
	chrome FlowChrome

	// saved is the digest the flow opened with; discardPending is the question
	// a ctrl+c or hub exit raises once the live config differs from it.
	saved          savedState
	discardPending bool

	// The shared frame clock (motion.go): clockGen/clockRunning identify the
	// single tea.Tick chain.
	motion       tui.MotionMode
	frame        uint64
	clockGen     uint64
	clockRunning bool
	// blurred idles the clock at 1Hz while the terminal reports no focus.
	blurred bool

	quitting bool
	result   Result
	err      error

	keyMap KeyMap

	// helpOpen makes every key but ctrl+c, esc and "?" inert.
	helpOpen bool

	// pendingG holds a lone "g" one keystroke for the gg chord.
	pendingG bool
}

// Result is what the wizard returns when it exits.
type Result struct {
	Outcome Outcome
	Config  *config.Config
	Secrets Secrets
	// ExitStep is the step that completed the flow; empty when cancelled
	ExitStep StepID
}

// Outcome identifies how the wizard terminated.
type Outcome uint8

// Outcomes.
const (
	OutcomeUnset Outcome = iota
	OutcomeCompleted
	OutcomeCancelled
)

// KeyMap binds the wizard-level keys; the Vim* bindings are footer-silent
// and listed only in the "?" overlay.
type KeyMap struct {
	Back        key.Binding
	Quit        key.Binding
	Help        key.Binding
	PageUp      key.Binding
	PageDown    key.Binding
	Home        key.Binding
	End         key.Binding
	VimHalfUp   key.Binding
	VimHalfDown key.Binding
	VimTop      key.Binding
	VimBottom   key.Binding
}

func defaultKeyMap() KeyMap {
	return KeyMap{
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys(HelpQuestion),
			key.WithHelp(HelpQuestion, HelpOverlay),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "scroll up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "scroll down"),
		),
		Home: key.NewBinding(
			key.WithKeys("ctrl+home"),
			key.WithHelp("ctrl+home", "top"),
		),
		End: key.NewBinding(
			key.WithKeys("ctrl+end"),
			key.WithHelp("ctrl+end", "bottom"),
		),
		VimHalfUp:   key.NewBinding(key.WithKeys("ctrl+u")),
		VimHalfDown: key.NewBinding(key.WithKeys("ctrl+d")),
		VimTop:      key.NewBinding(key.WithKeys("g")),
		VimBottom:   key.NewBinding(key.WithKeys("G", "shift+g")),
	}
}

// NewModel constructs a wizard Model with the default chrome. steps must be
// non-empty.
func NewModel(steps []WizardStep, cfg *config.Config) *Model {
	return NewFlowModel(steps, cfg, DefaultChrome())
}

// NewFlowModel constructs a wizard Model with flow-specific chrome.
func NewFlowModel(steps []WizardStep, cfg *config.Config, chrome FlowChrome) *Model {
	w, h := getTerminalSize()

	m := &Model{
		width:       w,
		height:      h,
		steps:       steps,
		currentStep: 0,
		config:      cfg,
		chrome:      chrome,
		saved:       newSavedState(cfg, steps),
		motion:      tui.Motion(),
		keyMap:      defaultKeyMap(),
	}

	if len(steps) > 0 {
		m.sizeCurrentStep()
		if f, ok := steps[0].(FocusableStep); ok {
			f.SetFocused(true)
		}
	}

	return m
}

func getTerminalSize() (width, height int) {
	_, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		h = 24
	}
	return tui.TerminalWidth(), h
}

// Init fires the first step's Init alongside a background-color request and
// the frame clock.
func (m *Model) Init() tea.Cmd {
	var stepCmd tea.Cmd
	if len(m.steps) > 0 {
		m.beginVisit()
		stepCmd = m.ownCommand(m.steps[m.currentStep].Init())
	}
	return tea.Batch(tea.RequestBackgroundColor, stepCmd, m.armClock())
}

// Update handles wizard-level messages, delegates the rest to the active
// step and keeps the frame clock armed while that step animates.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if tick, ok := msg.(clockTickMsg); ok {
		return m.handleClockTick(tick)
	}
	m.trackFocus(msg)
	if result, ok := msg.(visitResult); ok {
		if result.generation != m.generation || !m.currentStepMatches(result.step) {
			return m, nil
		}
		msg = result.message
	}
	model, cmd := m.update(msg)
	if clockCmd := m.armClock(); clockCmd != nil {
		cmd = tea.Batch(cmd, clockCmd)
	}
	return model, cmd
}

// trackFocus drops the clock to 1Hz on blur and retires the pending slow
// tick on focus so the full cadence re-arms at once.
func (m *Model) trackFocus(msg tea.Msg) {
	switch msg.(type) {
	case tea.BlurMsg:
		m.blurred = true
	case tea.FocusMsg:
		m.blurred = false
		if m.clockRunning {
			m.clockGen++
			m.clockRunning = false
		}
	}
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.handleResize(msg)
		return m, nil

	case tea.BackgroundColorMsg:
		// This case is the wizard's only SetDarkBackground call site, so the
		// terminal's one reply lands before the user can act on the frame.
		tui.SetDarkBackground(msg.IsDark())
		rebuildWizardStyles()
		components.RebuildStyles()
		return m, nil

	case tea.KeyPressMsg:
		if model, cmd, handled := m.handleWizardKey(msg); handled {
			return model, cmd
		}

	case StepCompleteMsg:
		// A late completion from a step the user already left must not advance,
		// and Apply, the current one.
		if !m.currentStepMatches(msg.StepID) {
			return m, nil
		}
		return m.goToNextStep()

	case StepBackMsg:
		return m.goToPreviousStep()

	case LayoutChangedMsg:
		if m.ready {
			m.sizeCurrentStep()
			m.resizeViewport()
			m.syncViewportContent()
		}
		return m, nil

	case JumpToStepMsg:
		return m.jumpToStep(msg.StepID)

	case ErrorSetMsg:
		m.setError(msg.Error)
		return m, nil

	case FocusChangedMsg:
		// Resync first: the focus move may have changed the step's content, so the
		// previous render's spans would point at stale lines.
		if m.ready {
			m.syncViewportContent()
			m.scrollToFocusedField()
			m.autoScrollToField(0, 0)
		}
		return m, nil

	case ConfigSyncMsg:
		return m.handleConfigSync(msg)
	}

	if len(m.steps) > 0 && m.currentStep < len(m.steps) {
		if _, editing := msg.(tea.KeyPressMsg); editing {
			m.err = nil
		}
		updatedStep, cmd := m.steps[m.currentStep].Update(msg)
		m.steps[m.currentStep] = updatedStep
		cmds = append(cmds, m.ownCommand(cmd))

		if m.ready {
			m.syncViewportContent()
			m.followEditedFocus(msg)
		}
	}

	return m, tea.Batch(cmds...)
}

// handleConfigSync applies a step's ConfigSyncMsg to the shared config; a
// mid-edit failure is the field's to show inline, never the status row's.
func (m *Model) handleConfigSync(msg ConfigSyncMsg) (tea.Model, tea.Cmd) {
	if !m.currentStepMatches(msg.StepID) {
		return m, nil
	}
	if a, ok := m.CurrentStep().(ConfigApplier); ok {
		_ = a.Apply(m.config)
	}
	return m, nil
}

// handleWizardKey handles the wizard-level keys ahead of the active step;
// handled false means the caller falls through to the step's Update.
func (m *Model) handleWizardKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	m.err = nil

	if key.Matches(msg, m.keyMap.Quit) {
		if !m.discardPending && m.asksBeforeDiscarding() {
			m.discardPending = true
			m.helpOpen = false
			return m, nil, true
		}
		return m.cancel()
	}

	if m.discardPending {
		return m.handleDiscardKey(msg)
	}

	if m.helpOpen {
		if key.Matches(msg, m.keyMap.Help) || key.Matches(msg, m.keyMap.Back) {
			m.helpOpen = false
		}
		return m, nil, true
	}

	if key.Matches(msg, m.keyMap.Help) && !m.currentStepConsumesTextInput() {
		m.helpOpen = true
		return m, nil, true
	}

	if m.handleScrollKey(msg) {
		return m, nil, true
	}

	if key.Matches(msg, m.keyMap.Back) {
		if m.currentStep > 0 {
			model, cmd := m.goToPreviousStep()
			return model, cmd, true
		}
	}

	return m, nil, false
}

func stepShouldShow(step WizardStep, cfg *config.Config) bool {
	if c, ok := step.(ConditionalStep); ok {
		return c.ShouldShow(cfg)
	}
	return true
}

func (m *Model) currentStepConsumesTextInput() bool {
	if len(m.steps) == 0 || m.currentStep < 0 || m.currentStep >= len(m.steps) {
		return false
	}
	tc, ok := m.steps[m.currentStep].(TextInputConsumer)
	return ok && tc.ConsumesTextInput()
}

// Result returns the wizard's terminal state, valid only after tea.Quit.
func (m *Model) Result() Result {
	return m.result
}

// Config returns the live config steps mutate through their Apply hooks.
func (m *Model) Config() *config.Config {
	return m.config
}

// CurrentStep returns the active step, or nil when the cursor is out of range.
func (m *Model) CurrentStep() WizardStep {
	if len(m.steps) > 0 && m.currentStep < len(m.steps) {
		return m.steps[m.currentStep]
	}
	return nil
}

func (m *Model) currentStepMatches(id StepID) bool {
	return m.CurrentStep() != nil && m.CurrentStep().ID() == id
}

func (m *Model) setError(err error) {
	m.err = err
	if m.ready {
		m.syncViewportContent()
		m.autoScrollToField(0, 0)
	}
}

func (m *Model) followEditedFocus(msg tea.Msg) {
	if _, editing := msg.(tea.KeyPressMsg); editing {
		m.autoScrollToField(0, 0)
	}
}

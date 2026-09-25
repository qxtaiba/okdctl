package wizard

import (
	"os"

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
	// footer is 2 rows: the scroll-indicator line (also the top divider) + the help bar.
	footerHeight         = 2
	outerVerticalPadding = 4 // wizard border (2) + outer padding (2)

	// outerHorizontalPadding: OuterContainerStyle.Padding(1, 2) = 2 left + 2 right.
	outerHorizontalPadding = 4

	// wizardBorderHorizontal: WizardBorderStyle.Border() = 1 left + 1 right.
	wizardBorderHorizontal = 2

	fixedLayoutOverhead = headerHeight + statusHeight + footerHeight + outerVerticalPadding

	// wideSplitWidth is the terminal width at and above which the wizard
	// splits into a form column and a dim context pane; below it the frame
	// stays a single column.
	wideSplitWidth = 150

	// maxFrameWidth caps the bordered frame's outer width (terminal minus
	// outerHorizontalPadding) below wideSplitWidth, so a wide-but-unsplit
	// terminal doesn't stretch the form past a comfortable measure — at and
	// above wideSplitWidth the surplus becomes the context pane instead.
	maxFrameWidth = 112

	// formMaxWidth caps the split layout's form column measure.
	formMaxWidth = 104

	// paneRuleWidth is the single-column divider between the form and the
	// context pane.
	paneRuleWidth = 1

	// paneMinWidth and paneMaxWidth bound the split layout's context pane.
	paneMinWidth = 28
	paneMaxWidth = 44

	// paneStepsHeaderRows is the STEPS section's own header row, counted
	// separately from its one-row-per-step body in splitMinHeight.
	paneStepsHeaderRows = 1
)

// splitMinHeight is the terminal height at and above which a stepCount-step
// wizard's context pane has room for its STEPS section — the header plus one
// row per step — without truncating it: fixedLayoutOverhead's fixed chrome
// rows, plus the header, plus stepCount. Below it, splitLayout falls back to
// the capped single-column tier rather than splitting into an unusably
// short pane; at or above it, renderContextPane may still drop SO FAR and
// FOCUSED FIELD (and, defensively, truncate the step list itself) if their
// content doesn't fit — the STEPS section's own minimum is the one thing
// this floor guarantees room for.
func splitMinHeight(stepCount int) int {
	return fixedLayoutOverhead + paneStepsHeaderRows + stepCount
}

type earlyExiter interface {
	ShouldExitEarly() bool
}

type actionGetter interface {
	GetSelectedAction() Action
}

type centerable interface {
	IsCentered() bool
}

// heroRenderer is implemented by steps that draw the product wordmark
// themselves, so the frame drops its own header — brand row, tagline, progress
// trail — entirely.
type heroRenderer interface {
	RendersHero() bool
}

// splitSuppressor is implemented by steps that own the frame's whole width
// however wide the terminal is — a centered launcher, where the context pane
// would be chrome describing work the screen isn't doing.
type splitSuppressor interface {
	SuppressesSplit() bool
}

// paneRenderer is implemented by steps that fill the split layout's right pane
// themselves, in place of the context pane — a live log beside the work it
// narrates says more than a step list the screen isn't walking. The same height
// clamp the context pane obeys applies: content must fit the rows it is given.
type paneRenderer interface {
	PaneContent(width, height int) string
}

// displayTitler is implemented by steps with a header prompt distinct from
// their Title(); an empty DisplayTitle falls back to Title() instead.
type displayTitler interface {
	DisplayTitle() string
}

// Model is the bubbletea model backing the configuration wizard.
type Model struct {
	width  int
	height int

	viewport viewport.Model
	ready    bool

	// contentRows[i] is the viewport row the active step's View line i starts
	// on; a step line wider than the content column wraps into several rows,
	// so the two index spaces differ. Length is line count + 1. Valid only
	// for non-centered steps: a centered step's content is re-rendered with
	// PaddingTop first, so the rows describe the shifted lines instead.
	contentRows []int

	steps       []WizardStep
	currentStep int

	// suspended is the flow SwapFlow put aside — the hub — restored when the
	// swapped-in flow's first screen is escaped. One level deep by design: the
	// hub is the only screen that swaps, and a sub-flow never swaps again.
	suspended *suspendedFlow

	// returnToReview: set on a review jump (JumpToStepMsg), cleared on confirm/escape;
	// while set, next/previous route to review.
	returnToReview bool

	config *config.Config
	chrome FlowChrome

	quitting bool
	result   Result
	err      error

	keyMap KeyMap

	// helpOpen: the "?" help overlay is showing over the viewport region.
	// While true every key but ctrl+c (still the global quit guard), esc,
	// and "?" itself (both close it) is inert — the overlay owns input.
	helpOpen bool
}

// suspendedFlow is a flow SwapFlow put aside: its steps, its chrome, and the
// screen the operator was on, so restoring it lands exactly where they left.
type suspendedFlow struct {
	steps       []WizardStep
	chrome      FlowChrome
	currentStep int
}

// SwapFlow replaces the live step set and chrome with another flow's inside the
// same program, suspending the current flow so escaping the new flow's first
// screen returns to it. The caller passes freshly built steps on every entry:
// the swapped-out instances are dropped on return, so nothing a sub-flow
// collected can bleed into the next entry.
func (m *Model) SwapFlow(steps []WizardStep, chrome FlowChrome) tea.Cmd {
	// A second swap is refused outright: accepting it would overwrite the
	// single suspended return target with the sub-flow being displaced,
	// leaving esc with nowhere correct to land.
	if len(steps) == 0 || m.suspended != nil {
		return nil
	}
	if f, ok := m.CurrentStep().(FocusableStep); ok {
		f.SetFocused(false)
	}

	m.suspended = &suspendedFlow{steps: m.steps, chrome: m.chrome, currentStep: m.currentStep}
	m.steps, m.chrome = steps, chrome
	m.returnToReview = false
	m.err = nil

	_, cmd := m.focusStep(0)
	return cmd
}

// restoreFlow returns to the flow SwapFlow suspended, discarding the swapped-in
// flow's steps so the next entry rebuilds them.
func (m *Model) restoreFlow() (tea.Model, tea.Cmd) {
	prev := m.suspended
	if prev == nil {
		return m, nil
	}
	if f, ok := m.CurrentStep().(FocusableStep); ok {
		f.SetFocused(false)
	}

	m.suspended = nil
	m.steps, m.chrome = prev.steps, prev.chrome
	m.returnToReview = false
	m.err = nil

	return m.focusStep(min(prev.currentStep, len(prev.steps)-1))
}

// SwapFlowMsg asks the wizard to replace its live step set and chrome with
// another flow's, in the same program and the same terminal session.
type SwapFlowMsg struct {
	Steps  []WizardStep
	Chrome FlowChrome
}

// Result is what the wizard returns when it exits.
type Result struct {
	Completed bool
	Cancelled bool
	Config    *config.Config
	Action    Action
}

// Action names the user's choice at the wizard's terminal step.
type Action string

// Actions the user can pick at the wizard's terminal step.
const (
	ActionDeploy Action = "deploy"
	ActionExit   Action = "exit"
)

// KeyMap binds wizard-level actions (quit, back, scroll) to keystrokes;
// everything else is handled inside the active step.
type KeyMap struct {
	Back     key.Binding
	Quit     key.Binding
	Help     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding
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
			key.WithKeys("home"),
			key.WithHelp("home", "top"),
		),
		End: key.NewBinding(
			key.WithKeys("end"),
			key.WithHelp("end", "bottom"),
		),
	}
}

// NewModel constructs a wizard Model bound to cfg with the default chrome.
// steps must be non-empty.
func NewModel(steps []WizardStep, cfg *config.Config) *Model {
	return NewFlowModel(steps, cfg, DefaultChrome())
}

// NewFlowModel constructs a wizard Model with flow-specific chrome, letting
// a second top-level flow rebrand tagline/badge without forking rendering.
func NewFlowModel(steps []WizardStep, cfg *config.Config, chrome FlowChrome) *Model {
	w, h := getTerminalSize()

	m := &Model{
		width:       w,
		height:      h,
		steps:       steps,
		currentStep: 0,
		config:      cfg,
		chrome:      chrome,
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

// Init implements tea.Model; it fires the first step's Init command
// alongside a terminal background-color request.
func (m *Model) Init() tea.Cmd {
	var stepCmd tea.Cmd
	if len(m.steps) > 0 {
		stepCmd = m.steps[m.currentStep].Init()
	}
	return tea.Batch(tea.RequestBackgroundColor, stepCmd)
}

// Update processes wizard-level messages (navigation, resize, quit) and
// delegates the rest to the currently-active step.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.handleResize(msg)
		return m, nil

	case tea.BackgroundColorMsg:
		// SetDarkBackground must be called exactly once, not concurrently
		// with itself: Init fires RequestBackgroundColor alongside the first
		// step's Init, and this case is the wizard's only call site, so the
		// terminal's one reply lands here as the sole caller, early — before
		// the user has had any chance to act on the rendered wizard.
		tui.SetDarkBackground(msg.IsDark())
		rebuildWizardStyles()
		components.RebuildStyles()
		return m, nil

	case tea.KeyPressMsg:
		if model, cmd, handled := m.handleWizardKey(msg); handled {
			return model, cmd
		}

	case StepCompleteMsg:
		return m.goToNextStep()

	case StepBackMsg:
		return m.goToPreviousStep()

	case LayoutChangedMsg:
		if m.ready {
			m.sizeCurrentStep()
			m.resizeViewport()
			m.syncViewportContent()
			m.notifyIfAtBottom()
		}
		return m, nil

	case SwapFlowMsg:
		cmd := m.SwapFlow(msg.Steps, msg.Chrome)
		return m, cmd

	case JumpToStepMsg:
		return m.jumpToStep(msg.StepID)

	case ErrorSetMsg:
		m.err = msg.Error
		return m, nil

	case FocusChangedMsg:
		// Resync first: the focus move may itself have changed the step's
		// content (an expanded dropdown, a new validation row), so spans
		// recorded by the previous render would point at stale lines.
		if m.ready {
			m.syncViewportContent()
			m.scrollToFocusedField()
		}
		return m, nil

	case ConfigSyncMsg:
		if len(m.steps) > 0 && m.currentStep >= 0 && m.currentStep < len(m.steps) {
			if a, ok := m.steps[m.currentStep].(ConfigApplier); ok {
				_ = a.Apply(m.config)
			}
		}
		return m, nil
	}

	if len(m.steps) > 0 && m.currentStep < len(m.steps) {
		updatedStep, cmd := m.steps[m.currentStep].Update(msg)
		m.steps[m.currentStep] = updatedStep
		cmds = append(cmds, cmd)

		if m.ready {
			m.syncViewportContent()
			m.notifyIfAtBottom()
		}
	}

	return m, tea.Batch(cmds...)
}

// handleWizardKey processes the wizard-level key bindings (quit, the help
// overlay, scroll, back) ahead of the active step. handled reports whether
// it fully handled msg — model/cmd are then Update's result — or whether
// the caller should fall through to the step's own Update instead.
func (m *Model) handleWizardKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	m.err = nil

	if key.Matches(msg, m.keyMap.Quit) {
		if len(m.steps) > 0 && m.currentStep < len(m.steps) {
			if g, ok := m.steps[m.currentStep].(QuitGuard); ok && g.InterceptQuit() {
				// The guard's own feedback (e.g. "press again to
				// force-quit") renders in the viewport region the
				// overlay would otherwise cover.
				m.helpOpen = false
				return m, nil, true
			}
		}
		m.quitting = true
		m.result = Result{Cancelled: true}
		return m, tea.Quit, true
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
		if g, ok := m.CurrentStep().(BackGuard); ok && g.InterceptBack() {
			return m, nil, true
		}
		if m.currentStep > 0 {
			model, cmd := m.goToPreviousStep()
			return model, cmd, true
		}
		// Escaping a swapped-in flow's first screen leaves the sub-flow and
		// returns to the hub that launched it.
		if m.suspended != nil {
			model, cmd := m.restoreFlow()
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

func stepAutoCompletes(step WizardStep) bool {
	if a, ok := step.(AutoCompletingStep); ok {
		return a.AutoCompletes()
	}
	return false
}

// currentStepConsumesTextInput reports whether the active step's focused
// field would consume a "?" keypress as literal typed text — see
// TextInputConsumer. A step that doesn't implement the interface (no text
// fields to speak for) always reports false, so "?" opens the help overlay
// there.
func (m *Model) currentStepConsumesTextInput() bool {
	if len(m.steps) == 0 || m.currentStep < 0 || m.currentStep >= len(m.steps) {
		return false
	}
	tc, ok := m.steps[m.currentStep].(TextInputConsumer)
	return ok && tc.ConsumesTextInput()
}

// Result returns the wizard's terminal state. Valid only after tea.Quit.
func (m *Model) Result() Result {
	return m.result
}

// Config returns the live config being assembled; steps mutate it in place via their Apply hooks.
func (m *Model) Config() *config.Config {
	return m.config
}

// CurrentStep returns the step the user is interacting with now, or nil
// if steps is empty or the cursor is out of range.
func (m *Model) CurrentStep() WizardStep {
	if len(m.steps) > 0 && m.currentStep < len(m.steps) {
		return m.steps[m.currentStep]
	}
	return nil
}

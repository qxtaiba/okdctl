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
)

type earlyExiter interface {
	ShouldExitEarly() bool
}

type actionGetter interface {
	GetSelectedAction() Action
}

type centerable interface {
	IsCentered() bool
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

	// returnToReview: set on a review jump (JumpToStepMsg), cleared on confirm/escape;
	// while set, next/previous route to review.
	returnToReview bool

	config *config.Config
	chrome FlowChrome

	quitting bool
	result   Result
	err      error

	keyMap KeyMap
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
		contentWidth, contentHeight := m.contentDimensions()
		if r, ok := steps[0].(ResizableStep); ok {
			r.SetSize(contentWidth, contentHeight)
		}
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
		m.err = nil

		if key.Matches(msg, m.keyMap.Quit) {
			if len(m.steps) > 0 && m.currentStep < len(m.steps) {
				if g, ok := m.steps[m.currentStep].(QuitGuard); ok && g.InterceptQuit() {
					return m, nil
				}
			}
			m.quitting = true
			m.result = Result{Cancelled: true}
			return m, tea.Quit
		}

		if m.handleScrollKey(msg) {
			return m, nil
		}

		if key.Matches(msg, m.keyMap.Back) && m.currentStep > 0 {
			if g, ok := m.steps[m.currentStep].(BackGuard); ok && g.InterceptBack() {
				return m, nil
			}
			return m.goToPreviousStep()
		}

	case StepCompleteMsg:
		return m.goToNextStep()

	case StepBackMsg:
		return m.goToPreviousStep()

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
		}
	}

	return m, tea.Batch(cmds...)
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

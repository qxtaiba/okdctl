package wizard

import (
	"os"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/qxtaiba/okdctl/internal/config"
)

const (
	minWidth  = 80
	minHeight = 24

	headerHeight = 3 // logo + tagline + step indicator
	// footer is 2 rows: scroll-indicator/divider + help bar. The
	// scroll-indicator now doubles as the footer's top-border divider so
	// what was previously a separate scrollIndicatorHeight row plus a
	// footerHeight=2 BorderTop+helpBar collapses into a single 2-row block.
	footerHeight         = 2
	outerVerticalPadding = 4 // wizard border (2) + outer padding (2)

	// outerHorizontalPadding is the horizontal cells consumed by
	// OuterContainerStyle.Padding(1, 2): 2 left + 2 right.
	outerHorizontalPadding = 4

	// wizardBorderHorizontal is the horizontal cells consumed by
	// WizardBorderStyle.Border(): 1 left + 1 right.
	wizardBorderHorizontal = 2

	fixedLayoutOverhead = headerHeight + footerHeight + outerVerticalPadding
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

// Model is the bubbletea model backing the configuration wizard.
type Model struct {
	width  int
	height int

	viewport viewport.Model
	ready    bool

	steps       []WizardStep
	currentStep int

	// returnToReview is set when the review screen jumps to an edit step
	// (JumpToStepMsg) and cleared once that step is confirmed or escaped;
	// while set, goToNextStep/goToPreviousStep route back to the review
	// step instead of the normal forward/backward step.
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
	ActionDeploy    Action = "deploy"
	ActionPreflight Action = "preflight"
	ActionExit      Action = "exit"
)

// KeyMap binds wizard-level actions to keystrokes: quit, back-navigation,
// and viewport scrolling. Everything else (field navigation, selection,
// confirm) is handled inside the active step; per-step help is contributed
// via the HelpProvider interface.
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

// NewModel constructs a wizard Model bound to cfg with the configure
// wizard's default chrome. steps must be non-empty; the first step is
// focused and sized to the terminal.
func NewModel(steps []WizardStep, cfg *config.Config) *Model {
	return NewFlowModel(steps, cfg, DefaultChrome())
}

// NewFlowModel constructs a wizard Model with flow-specific chrome, so a
// second top-level flow can rebrand the tagline and context badge without
// forking the header rendering.
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
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return minWidth, minHeight
	}
	if w < minWidth {
		w = minWidth
	}
	if h < minHeight {
		h = minHeight
	}
	return w, h
}

// Init implements tea.Model; it fires the first step's Init command.
func (m *Model) Init() tea.Cmd {
	var cmds []tea.Cmd

	if len(m.steps) > 0 {
		cmds = append(cmds, m.steps[m.currentStep].Init())
	}

	return tea.Batch(cmds...)
}

// Update processes wizard-level messages (navigation, resize, quit) and
// delegates the rest to the currently-active step.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.handleResize(msg)
		return m, nil

	case tea.KeyPressMsg:
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
		if m.ready {
			m.autoScrollToField(msg.FieldIndex, msg.TotalFields)
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

// Config returns the live config being assembled. Steps mutate it in place
// via their Apply hooks.
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

package wizard

import (
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
)

// ═══════════════════════════════════════════════════════════════════════════════
// CONSTANTS
// ═══════════════════════════════════════════════════════════════════════════════

const (
	// Minimum terminal dimensions
	minWidth  = 80
	minHeight = 24

	// Layout heights for fixed UI elements
	headerHeight          = 3 // brand + tagline + border
	scrollIndicatorHeight = 1
	footerHeight          = 2 // help bar + padding
	outerVerticalPadding  = 4 // border (2) + outer padding (2)
	fixedLayoutOverhead   = headerHeight + scrollIndicatorHeight + footerHeight + outerVerticalPadding
)

// ═══════════════════════════════════════════════════════════════════════════════
// INTERNAL INTERFACES FOR TYPE ASSERTIONS
// ═══════════════════════════════════════════════════════════════════════════════

// earlyExiter is implemented by steps that can exit early (e.g., welcome step).
type earlyExiter interface {
	ShouldExitEarly() bool
}

// actionGetter is implemented by steps that provide a selected action.
type actionGetter interface {
	GetSelectedAction() Action
}

// centerable is implemented by steps that want centered content.
type centerable interface {
	IsCentered() bool
}

// ═══════════════════════════════════════════════════════════════════════════════
// WIZARD MODEL
// ═══════════════════════════════════════════════════════════════════════════════

// Model is the main wizard model that orchestrates the step-based flow.
type Model struct {
	width  int
	height int

	viewport viewport.Model
	ready    bool // true after first WindowSizeMsg

	steps       []WizardStep
	currentStep int

	config *config.Config

	quitting bool
	result   Result
	err      error

	keyMap KeyMap
}

// Result holds the wizard completion result.
type Result struct {
	Completed bool
	Cancelled bool
	Config    *config.Config
	Action    Action
}

// Action represents what to do after wizard completes.
type Action string

const (
	ActionDeploy    Action = "deploy"
	ActionPreflight Action = "preflight"
	ActionExit      Action = "exit"
)

// KeyMap defines the wizard key bindings.
type KeyMap struct {
	Next     key.Binding
	Back     key.Binding
	Quit     key.Binding
	Help     key.Binding
	Up       key.Binding
	Down     key.Binding
	Select   key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Next: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Select: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "select"),
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

// ═══════════════════════════════════════════════════════════════════════════════
// CONSTRUCTOR
// ═══════════════════════════════════════════════════════════════════════════════

// NewModel creates a new wizard model with the given steps.
func NewModel(steps []WizardStep, cfg *config.Config) Model {
	w, h := getTerminalSize()

	m := Model{
		width:       w,
		height:      h,
		steps:       steps,
		currentStep: 0,
		config:      cfg,
		keyMap:      DefaultKeyMap(),
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

// getTerminalSize returns the current terminal dimensions.
func getTerminalSize() (int, int) {
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

// ═══════════════════════════════════════════════════════════════════════════════
// TEA.MODEL IMPLEMENTATION
// ═══════════════════════════════════════════════════════════════════════════════

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd

	if len(m.steps) > 0 {
		cmds = append(cmds, m.steps[m.currentStep].Init())
	}

	return tea.Batch(cmds...)
}

// Update handles messages and updates state.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.handleResize(msg)
		return m, nil

	case tea.KeyMsg:
		if key.Matches(msg, m.keyMap.Quit) {
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

	case StepErrorMsg:
		m.err = msg.Error
		return m, nil

	case ErrorSetMsg:
		m.err = msg.Error
		return m, nil

	case ConfigUpdatedMsg:
		return m.rebuildSteps()

	case FocusChangedMsg:
		if m.ready {
			m.autoScrollToField(msg.FieldIndex, msg.TotalFields)
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

// ═══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS FOR OPTIONAL INTERFACES
// ═══════════════════════════════════════════════════════════════════════════════

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

// ═══════════════════════════════════════════════════════════════════════════════
// PUBLIC METHODS
// ═══════════════════════════════════════════════════════════════════════════════

// Result returns the wizard result.
func (m Model) Result() Result {
	return m.result
}

// Config returns the current configuration.
func (m Model) Config() *config.Config {
	return m.config
}

// CurrentStep returns the current step.
func (m Model) CurrentStep() WizardStep {
	if len(m.steps) > 0 && m.currentStep < len(m.steps) {
		return m.steps[m.currentStep]
	}
	return nil
}

// AddStep adds a step to the wizard.
func (m *Model) AddStep(step WizardStep) {
	m.steps = append(m.steps, step)
}

// InsertStepAfter inserts a step after the specified step ID.
func (m *Model) InsertStepAfter(afterID StepID, step WizardStep) {
	for i, s := range m.steps {
		if s.ID() == afterID {
			newSteps := make([]WizardStep, 0, len(m.steps)+1)
			newSteps = append(newSteps, m.steps[:i+1]...)
			newSteps = append(newSteps, step)
			newSteps = append(newSteps, m.steps[i+1:]...)
			m.steps = newSteps
			return
		}
	}
	m.steps = append(m.steps, step)
}

// RemoveStep removes a step by ID.
func (m *Model) RemoveStep(id StepID) {
	for i, s := range m.steps {
		if s.ID() == id {
			m.steps = append(m.steps[:i], m.steps[i+1:]...)
			if m.currentStep >= len(m.steps) {
				m.currentStep = len(m.steps) - 1
			}
			if m.currentStep < 0 {
				m.currentStep = 0
			}
			return
		}
	}
}

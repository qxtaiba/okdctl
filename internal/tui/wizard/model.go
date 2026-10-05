package wizard

import (
	"context"
	"crypto/sha256"
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
	// footer is 2 rows: the scroll-indicator line (also the top divider) + the help bar.
	footerHeight         = 2
	outerVerticalPadding = 4 // wizard border (2) + outer padding (2)

	// outerHorizontalPadding: OuterContainerStyle.Padding(1, 2) = 2 left + 2 right.
	outerHorizontalPadding = 4

	// wizardBorderHorizontal: WizardBorderStyle.Border() = 1 left + 1 right.
	wizardBorderHorizontal = 2

	fixedLayoutOverhead = headerHeight + statusHeight + footerHeight + outerVerticalPadding

	// singleFormMaxWidth caps the form measure — the frame itself always
	// spans the terminal, so a wide terminal keeps a readable column inside
	// the full-width frame rather than stretching the form.
	singleFormMaxWidth = 110

	// formMaxWidth is the measure FieldWidthAuto and FieldWidthPath derive from.
	formMaxWidth = 104
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

// heroRenderer is implemented by steps that draw the product wordmark
// themselves, so the frame drops its own header — brand row, tagline, progress
// trail — entirely.
type heroRenderer interface {
	RendersHero() bool
}

// frameWidthOwner is implemented by steps that render across the frame's
// whole width instead of the capped form measure — a centered launcher, a
// full-screen log.
type frameWidthOwner interface {
	OwnsFrameWidth() bool
}

// displayTitler is implemented by steps with a header prompt distinct from
// their Title(); an empty DisplayTitle falls back to Title() instead.
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

	// savedDigest fingerprints the config the flow opened with; discardPending
	// is the open question a first ctrl+c raises once the live config differs.
	savedDigest    [sha256.Size]byte
	discardPending bool

	// The shared frame clock (see motion.go): motion is the resolved dial,
	// frame the monotonic counter, clockGen/clockRunning the identity and
	// liveness of the single tea.Tick chain.
	motion       tui.MotionMode
	frame        uint64
	clockGen     uint64
	clockRunning bool
	// blurred is away mode's flag: the terminal reported losing focus, so
	// the clock idles at 1Hz and cosmetic animators stand suspended.
	blurred bool

	quitting bool
	result   Result
	err      error

	keyMap KeyMap

	// helpOpen: the "?" help overlay is showing over the viewport region.
	// While true every key but ctrl+c (still the global quit guard), esc,
	// and "?" itself (both close it) is inert — the overlay owns input.
	helpOpen bool

	// pendingG: a lone "g" is held one keystroke, completing the vim gg
	// chord if the next key is "g" again and clearing otherwise.
	pendingG bool
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
	Outcome Outcome
	Config  *config.Config
	Action  Action
	// ExitStep is the step that completed the flow; empty when cancelled.
	ExitStep StepID
}

// Outcome identifies how the wizard terminated.
type Outcome uint8

// OutcomeUnset and the terminal outcomes are mutually exclusive.
const (
	OutcomeUnset Outcome = iota
	OutcomeCompleted
	OutcomeCancelled
)

// Action names the user's choice at the wizard's terminal step.
type Action string

// Actions the user can pick at the wizard's terminal step.
const (
	ActionDeploy Action = "deploy"
	ActionExit   Action = "exit"
)

// KeyMap binds wizard-level actions (quit, back, scroll) to keystrokes;
// everything else is handled inside the active step. The Vim* bindings are
// footer-silent additions listed only in the "?" overlay's vim group.
type KeyMap struct {
	Back        key.Binding
	Quit        key.Binding
	Help        key.Binding
	PageUp      key.Binding
	PageDown    key.Binding
	Home        key.Binding
	End         key.Binding
	Up          key.Binding
	Down        key.Binding
	VimUp       key.Binding
	VimDown     key.Binding
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
		Up: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "scroll up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓", "scroll down"),
		),
		VimUp:       key.NewBinding(key.WithKeys("k")),
		VimDown:     key.NewBinding(key.WithKeys("j")),
		VimHalfUp:   key.NewBinding(key.WithKeys("ctrl+u")),
		VimHalfDown: key.NewBinding(key.WithKeys("ctrl+d")),
		VimTop:      key.NewBinding(key.WithKeys("g")),
		VimBottom:   key.NewBinding(key.WithKeys("G", "shift+g")),
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
		savedDigest: configDigest(cfg),
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

// Init implements tea.Model; it fires the first step's Init command
// alongside a terminal background-color request and, when that first step
// is already animating, the frame clock.
func (m *Model) Init() tea.Cmd {
	var stepCmd tea.Cmd
	if len(m.steps) > 0 {
		m.beginVisit()
		stepCmd = m.ownCommand(m.steps[m.currentStep].Init())
	}
	return tea.Batch(tea.RequestBackgroundColor, stepCmd, m.armClock())
}

// Update processes wizard-level messages (navigation, resize, quit),
// delegates the rest to the currently-active step, and keeps the frame
// clock armed exactly while the active step animates.
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

// trackFocus keeps the away-mode state: a blur drops the shared clock to
// 1Hz (the running chain finishes its pending tick at the old cadence), and
// a focus retires the pending slow tick so the full-cadence chain — and the
// step's catch-up sweep riding on it — re-arms immediately. Both messages
// still reach the active step through the normal delegation below.
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

// update is Update's body, split out so handleClockTick can route a frame
// through the same step-delegation path without re-entering the clock.
func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		// A late async completion from a step the user has already left
		// (esc, SwapFlow) must not advance — and Apply — whichever step is
		// current now.
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
			m.notifyIfAtBottom()
		}
		return m, nil

	case SwapFlowMsg:
		cmd := m.SwapFlow(msg.Steps, msg.Chrome)
		return m, cmd

	case JumpToStepMsg:
		return m.jumpToStep(msg.StepID)

	case ErrorSetMsg:
		m.setError(msg.Error)
		return m, nil

	case FocusChangedMsg:
		// Resync first: the focus move may itself have changed the step's
		// content (an expanded dropdown, a new validation row), so spans
		// recorded by the previous render would point at stale lines.
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
			m.notifyIfAtBottom()
			m.followEditedFocus(msg)
		}
	}

	return m, tea.Batch(cmds...)
}

// handleConfigSync applies a step's ConfigSyncMsg to the shared config.
func (m *Model) handleConfigSync(msg ConfigSyncMsg) (tea.Model, tea.Cmd) {
	if !m.currentStepMatches(msg.StepID) {
		return m, nil
	}
	if len(m.steps) > 0 && m.currentStep >= 0 && m.currentStep < len(m.steps) {
		if a, ok := m.steps[m.currentStep].(ConfigApplier); ok {
			if err := a.Apply(m.config); err != nil {
				m.err = err
				return m, nil
			}
		}
	}
	return m, nil
}

// handleWizardKey processes the wizard-level key bindings (quit, the discard
// question, the help overlay, scroll, back) ahead of the active step. handled reports whether
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
		if owner, ok := m.CurrentStep().(interface{ OwnsKey(tea.KeyPressMsg) bool }); ok && owner.OwnsKey(msg) {
			return m, nil, false
		}
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

// arrowScroller is implemented by steps that opt in to ↑/↓ scrolling the
// frame's viewport one line — read-only screens with no field for the arrows
// to drive; a form or list step keeps the keys for its own navigation.
type arrowScroller interface {
	ScrollsWithArrows() bool
}

// currentStepScrollsWithArrows reports whether the active step has opted its
// viewport into line-by-line arrow scrolling, mirroring the
// currentStepConsumesTextInput opt-in: an unimplemented interface leaves the
// arrows to the step's own Update.
func (m *Model) currentStepScrollsWithArrows() bool {
	if len(m.steps) == 0 || m.currentStep < 0 || m.currentStep >= len(m.steps) {
		return false
	}
	a, ok := m.steps[m.currentStep].(arrowScroller)
	return ok && a.ScrollsWithArrows()
}

// logPager is implemented by steps that page a log region of their own with
// pgup/pgdn; while it reports true the frame leaves those keys to the step
// instead of scrolling the viewport.
type logPager interface {
	ConsumesPaging() bool
}

// currentStepConsumesPaging reports whether the active step is paging a log
// region of its own right now.
func (m *Model) currentStepConsumesPaging() bool {
	if len(m.steps) == 0 || m.currentStep < 0 || m.currentStep >= len(m.steps) {
		return false
	}
	p, ok := m.steps[m.currentStep].(logPager)
	return ok && p.ConsumesPaging()
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

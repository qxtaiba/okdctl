package wizard

import (
	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

// StepID identifies a specific WizardStep instance at runtime (for
// StepCompleteMsg routing and similar), distinct from StepType (config.go)
// which names factory-registry entries.
type StepID string

// Built-in StepID values for each wizard step in DefaultConfig.
const (
	StepIDWelcome       StepID = "welcome"
	StepIDDistribution  StepID = "distribution"
	StepIDBasics        StepID = "basics"
	StepIDProxmox       StepID = "proxmox"
	StepIDNodePlacement StepID = "node-placement"
	StepIDNetworking    StepID = "networking"
	StepIDResources     StepID = "resources"
	StepIDAddons        StepID = "addons"
	StepIDFiles         StepID = "files"
	StepIDAdvanced      StepID = "advanced"
	StepIDReview        StepID = "review"
)

// WizardStep is the contract every wizard step must satisfy.
//
//nolint:revive // stutter-named interface is the established internal API; rename deferred to a dedicated refactor
type WizardStep interface {
	ID() StepID
	Title() string
	Init() tea.Cmd
	Update(msg tea.Msg) (WizardStep, tea.Cmd)
	View(width, height int) string
}

// ConfigApplier is implemented by steps that write their values into cfg on
// step advance.
type ConfigApplier interface {
	Apply(cfg *config.Config) error
}

// ConditionalStep is implemented by steps that may be skipped based on cfg.
type ConditionalStep interface {
	ShouldShow(cfg *config.Config) bool
}

// FocusableStep is implemented by steps that track their own focus state.
type FocusableStep interface {
	IsFocused() bool
	SetFocused(focused bool)
}

// ResizableStep is implemented by steps that respond to viewport resizes.
type ResizableStep interface {
	SetSize(width, height int)
}

// AutoCompletingStep marks steps that complete without user interaction; they
// are skipped when navigating back with ESC.
type AutoCompletingStep interface {
	AutoCompletes() bool
}

// HelpProvider is implemented by steps that supply their own help footer.
type HelpProvider interface {
	ShortHelp() []KeyBinding
}

// QuitGuard is implemented by steps that must intercept ctrl+c (e.g. graceful
// cancel on first press); returning true consumes the keypress, false quits normally.
type QuitGuard interface {
	InterceptQuit() bool
}

// BackGuard is implemented by forward-only steps that must intercept esc
// (navigating away would orphan an in-flight mutation); true consumes the
// keypress, false navigates back normally.
type BackGuard interface {
	InterceptBack() bool
}

// KeyBinding is a key/help pair used in the wizard footer.
type KeyBinding struct {
	Key  string
	Help string
}

// BaseStep implements common WizardStep fields and defaults; embed it in
// concrete steps to avoid boilerplate.
type BaseStep struct {
	id           StepID
	title        string
	displayTitle string
	description  string
	focused      bool
	width        int
	height       int
}

// NewBaseStep returns a BaseStep with the given id, title, and description.
func NewBaseStep(id StepID, title, description string) BaseStep {
	return NewBaseStepWithDisplayTitle(id, title, "", description)
}

// NewBaseStepWithDisplayTitle returns a BaseStep with a separate
// displayTitle (above the step body) plus title (in the progress indicator).
func NewBaseStepWithDisplayTitle(id StepID, title, displayTitle, description string) BaseStep {
	return BaseStep{
		id:           id,
		title:        title,
		displayTitle: displayTitle,
		description:  description,
		width:        80,
		height:       24,
	}
}

// ID returns the step's identifier.
func (b *BaseStep) ID() StepID { return b.id }

// Title returns the progress-indicator title.
func (b *BaseStep) Title() string { return b.title }

// DisplayTitle returns the title shown above the step body; an empty
// string skips title rendering.
func (b *BaseStep) DisplayTitle() string { return b.displayTitle }

// IsFocused reports whether the step currently owns input focus.
func (b *BaseStep) IsFocused() bool { return b.focused }

// ShouldShow always returns true; override in concrete steps to skip.
func (b *BaseStep) ShouldShow(_ *config.Config) bool {
	return true
}

// SetFocused updates the step's focus state.
func (b *BaseStep) SetFocused(focused bool) {
	b.focused = focused
}

// SetSize updates the step's inner width and height.
func (b *BaseStep) SetSize(width, height int) {
	b.width = width
	b.height = height
}

// ShortHelp returns the default key bindings shown in the wizard footer;
// concrete steps override to contribute step-specific keys.
func (b *BaseStep) ShortHelp() []KeyBinding {
	return []KeyBinding{
		{Key: "↑↓", Help: HelpNavigate},
		{Key: HelpEnter, Help: HelpConfirm},
		{Key: HelpEsc, Help: HelpBack},
		{Key: HelpCtrlC, Help: HelpQuit},
	}
}

// AutoCompletes reports whether the step auto-advances without user input; default false.
func (b *BaseStep) AutoCompletes() bool {
	return false
}

// StepCompleteMsg signals that the named step is finished and the wizard
// should advance.
type StepCompleteMsg struct {
	StepID StepID
}

// StepBackMsg signals that the wizard should step back one position.
type StepBackMsg struct{}

// ErrorSetMsg signals an error to display in the wizard's footer, emitted
// when a ConfigApplier returns an error during a step transition.
type ErrorSetMsg struct {
	Error error
}

// FocusChangedMsg signals that focus has moved within the active step; the
// wizard uses this to auto-scroll the focused field into view.
type FocusChangedMsg struct {
	FieldIndex  int
	TotalFields int
}

// ConfigSyncMsg requests step.Apply(cfg) on the active step without
// advancing, so a step can publish a tentative selection (e.g. a status
// badge) while still focused.
type ConfigSyncMsg struct {
	StepID StepID
}

// JumpTarget pairs a StepID with the 1-based digit that jumps to it from
// review. Digits are compacted: a ShouldShow-hidden step consumes none.
type JumpTarget struct {
	StepID StepID
	Digit  int
}

// ReviewJumper is implemented by the review step. JumpOrder declares which
// steps section headers may route a digit to; the wizard delivers the
// ShouldShow-compacted result via SetJumpTargets on every focus.
type ReviewJumper interface {
	JumpOrder() []StepID
	SetJumpTargets(targets []JumpTarget)
}

// JumpToStepMsg jumps directly to the named step; confirming or escaping
// it there returns straight to review instead of replaying steps between.
type JumpToStepMsg struct {
	StepID StepID
}

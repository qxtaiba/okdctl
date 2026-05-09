package wizard

import (
	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

// StepID is an opaque identifier used within the wizard runtime to identify
// a specific WizardStep instance (for StepCompleteMsg routing, InsertStepAfter
// lookups, and similar). It is distinct from StepType (see config.go), which
// names entries in the external factory registry used when constructing a
// wizard from a Config.
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

// AutoCompletingStep marks steps that complete without user interaction.
// Steps that auto-complete are skipped when navigating backward with ESC.
type AutoCompletingStep interface {
	AutoCompletes() bool
}

// HelpProvider is implemented by steps that supply their own help footer.
type HelpProvider interface {
	ShortHelp() []KeyBinding
}

// DescribedStep is implemented by steps that supply descriptive header text.
type DescribedStep interface {
	Description() string

	// DisplayTitle returns the prompt text above the step content.
	// Return empty string to skip title rendering.
	DisplayTitle() string
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
	return BaseStep{
		id:          id,
		title:       title,
		description: description,
		width:       80,
		height:      24,
	}
}

// NewBaseStepWithDisplayTitle returns a BaseStep with a separate
// displayTitle (shown above the step body) in addition to title (shown in
// the progress indicator).
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

// DisplayTitle returns the title shown above the step body.
func (b *BaseStep) DisplayTitle() string { return b.displayTitle }

// Description returns the step's descriptive header text.
func (b *BaseStep) Description() string { return b.description }

// IsFocused reports whether the step currently owns input focus.
func (b *BaseStep) IsFocused() bool { return b.focused }

// Width returns the step's current inner width in terminal columns.
func (b *BaseStep) Width() int { return b.width }

// Height returns the step's current inner height in terminal rows.
func (b *BaseStep) Height() int { return b.height }

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
		{Key: "↑↓", Help: helpNavigate},
		{Key: helpEnter, Help: helpConfirm},
		{Key: helpEsc, Help: helpBack},
		{Key: helpCtrlC, Help: helpQuit},
	}
}

// AutoCompletes reports whether the step auto-advances without user input;
// the default is false. Override in steps that complete themselves.
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

// ErrorSetMsg signals an error that should be displayed in the wizard's
// footer. Emitted from the wizard core when a ConfigApplier returns an error
// during step transitions (see goToNextStep).
type ErrorSetMsg struct {
	Error error
}

// FocusChangedMsg signals that focus has moved within the active step; the
// wizard uses this to auto-scroll the focused field into view.
type FocusChangedMsg struct {
	FieldIndex  int
	TotalFields int
}

// ConfigSyncMsg requests the wizard to call step.Apply(cfg) on the active
// step *without* advancing — used so a step can publish a tentative
// selection (e.g. for a status badge) while the user is still on the step.
type ConfigSyncMsg struct {
	StepID StepID
}

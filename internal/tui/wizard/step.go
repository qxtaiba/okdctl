package wizard

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
)

// StepID is an opaque identifier used within the wizard runtime to identify
// a specific WizardStep instance (for StepCompleteMsg routing, InsertStepAfter
// lookups, and similar). It is distinct from StepType (see config.go), which
// names entries in the external factory registry used when constructing a
// wizard from a Config.
type StepID string

const (
	StepIDWelcome      StepID = "welcome"
	StepIDDistribution StepID = "distribution"
	StepIDBasics       StepID = "basics"
	StepIDReview       StepID = "review"
)

type WizardStep interface {
	ID() StepID
	Title() string
	Init() tea.Cmd
	Update(msg tea.Msg) (WizardStep, tea.Cmd)
	View(width, height int) string
}

type ConfigApplier interface {
	Apply(cfg *config.Config) error
}

type ConditionalStep interface {
	ShouldShow(cfg *config.Config) bool
}

type FocusableStep interface {
	IsFocused() bool
	SetFocused(focused bool)
}

type ResizableStep interface {
	SetSize(width, height int)
}

// AutoCompletingStep marks steps that complete without user interaction.
// Steps that auto-complete are skipped when navigating backward with ESC.
type AutoCompletingStep interface {
	AutoCompletes() bool
}

type HelpProvider interface {
	ShortHelp() []KeyBinding
}

type DescribedStep interface {
	Description() string

	// DisplayTitle returns the prompt text above the step content.
	// Return empty string to skip title rendering.
	DisplayTitle() string
}

type KeyBinding struct {
	Key  string
	Help string
}

type BaseStep struct {
	id           StepID
	title        string
	displayTitle string
	description  string
	focused      bool
	width        int
	height       int
}

func NewBaseStep(id StepID, title, description string) BaseStep {
	return BaseStep{
		id:          id,
		title:       title,
		description: description,
		width:       80,
		height:      24,
	}
}

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

func (b *BaseStep) ID() StepID          { return b.id }
func (b *BaseStep) Title() string        { return b.title }
func (b *BaseStep) DisplayTitle() string { return b.displayTitle }
func (b *BaseStep) Description() string  { return b.description }
func (b *BaseStep) IsFocused() bool      { return b.focused }
func (b *BaseStep) Width() int           { return b.width }
func (b *BaseStep) Height() int          { return b.height }

func (b *BaseStep) ShouldShow(cfg *config.Config) bool {
	return true
}

func (b *BaseStep) SetFocused(focused bool) {
	b.focused = focused
}

func (b *BaseStep) SetSize(width, height int) {
	b.width = width
	b.height = height
}

func (b *BaseStep) ShortHelp() []KeyBinding {
	return []KeyBinding{
		{Key: "↑↓", Help: "navigate"},
		{Key: "enter", Help: "confirm"},
		{Key: "esc", Help: "back"},
		{Key: "ctrl+c", Help: "quit"},
	}
}

func (b *BaseStep) AutoCompletes() bool {
	return false
}

type StepCompleteMsg struct {
	StepID StepID
}

type StepBackMsg struct{}

type StepSkipMsg struct {
	StepID StepID
}

// ErrorSetMsg signals an error that should be displayed in the wizard's
// footer. Emitted from the wizard core when a ConfigApplier returns an error
// during step transitions (see goToNextStep).
type ErrorSetMsg struct {
	Error error
}

type FocusChangedMsg struct {
	FieldIndex  int
	TotalFields int
}

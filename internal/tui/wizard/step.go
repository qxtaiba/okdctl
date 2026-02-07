// Package wizard provides the multi-step configuration wizard.
package wizard

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
)

// ═══════════════════════════════════════════════════════════════════════════════
// WIZARD STEP INTERFACE
// ═══════════════════════════════════════════════════════════════════════════════

// StepID uniquely identifies a wizard step.
type StepID string

// Core step IDs that are always present.
const (
	StepIDWelcome      StepID = "welcome"
	StepIDDistribution StepID = "distribution"
	StepIDBasics       StepID = "basics"
	StepIDReview       StepID = "review"
	StepIDDeploy       StepID = "deploy"
)

// WizardStep is the core interface that all wizard steps must implement.
type WizardStep interface {
	ID() StepID
	Title() string
	Init() tea.Cmd
	Update(msg tea.Msg) (WizardStep, tea.Cmd)
	View(width, height int) string
}

// ═══════════════════════════════════════════════════════════════════════════════
// OPTIONAL STEP INTERFACES
// ═══════════════════════════════════════════════════════════════════════════════

// ConfigApplier is implemented by steps that modify the configuration.
type ConfigApplier interface {
	Apply(cfg *config.Config) error
}

// ConditionalStep is implemented by steps that may be shown or hidden
// based on the current configuration state.
type ConditionalStep interface {
	ShouldShow(cfg *config.Config) bool
}

// FocusableStep is implemented by steps that manage focus state.
type FocusableStep interface {
	IsFocused() bool
	SetFocused(focused bool)
}

// ResizableStep is implemented by steps that need to respond to size changes.
type ResizableStep interface {
	SetSize(width, height int)
}

// AutoCompletingStep is implemented by steps that complete automatically
// without user interaction (e.g., loading steps).
type AutoCompletingStep interface {
	// Steps that auto-complete are skipped when navigating backward with ESC.
	AutoCompletes() bool
}

// HelpProvider is implemented by steps that provide custom help text.
type HelpProvider interface {
	ShortHelp() []KeyBinding
}

// DescribedStep is implemented by steps that provide extended description
// and display title for rendering.
type DescribedStep interface {
	Description() string

	// DisplayTitle returns the prompt text displayed above the step content.
	// Return empty string to skip title rendering (e.g., for welcome step).
	DisplayTitle() string
}

// KeyBinding represents a keyboard shortcut with its description.
type KeyBinding struct {
	Key  string
	Help string
}

// ═══════════════════════════════════════════════════════════════════════════════
// BASE STEP IMPLEMENTATION
// ═══════════════════════════════════════════════════════════════════════════════

// BaseStep provides common functionality for all wizard steps.
type BaseStep struct {
	id           StepID
	title        string
	displayTitle string
	description  string
	focused      bool
	width        int
	height       int
}

// NewBaseStep creates a new BaseStep with the given properties.
func NewBaseStep(id StepID, title, description string) BaseStep {
	return BaseStep{
		id:          id,
		title:       title,
		description: description,
		width:       80,
		height:      24,
	}
}

// NewBaseStepWithDisplayTitle creates a BaseStep with a custom display title.
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

// ShouldShow returns true by default - override for conditional steps.
func (b *BaseStep) ShouldShow(cfg *config.Config) bool {
	return true
}

// SetFocused sets the focus state.
func (b *BaseStep) SetFocused(focused bool) {
	b.focused = focused
}

// SetSize updates the available dimensions.
func (b *BaseStep) SetSize(width, height int) {
	b.width = width
	b.height = height
}

// ShortHelp returns default key bindings.
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

// ═══════════════════════════════════════════════════════════════════════════════
// NAVIGATION MESSAGES
// ═══════════════════════════════════════════════════════════════════════════════

// StepCompleteMsg signals that a step has completed and wants to advance.
type StepCompleteMsg struct {
	StepID StepID
}

// StepBackMsg signals that the user wants to go back.
type StepBackMsg struct{}

// StepSkipMsg signals that a step should be skipped.
type StepSkipMsg struct {
	StepID StepID
}

// StepErrorMsg signals a validation or processing error.
type StepErrorMsg struct {
	StepID StepID
	Error  error
}

// ErrorSetMsg triggers a UI refresh after an error is set.
type ErrorSetMsg struct {
	Error error
}

// ConfigUpdatedMsg signals that the config has been modified.
type ConfigUpdatedMsg struct{}

// FocusChangedMsg signals that focus has changed to a different field.
// Used for auto-scrolling the viewport.
type FocusChangedMsg struct {
	FieldIndex  int
	TotalFields int
}

package wizard

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
)

// StepID identifies a WizardStep instance at runtime; StepType names a
// factory-registry entry.
type StepID string

// Built-in StepID values.
const (
	StepIDWelcome       StepID = "welcome"
	StepIDDistribution  StepID = "distribution"
	StepIDBasics        StepID = "basics"
	StepIDProxmox       StepID = "proxmox"
	StepIDNodePlacement StepID = "node-placement"
	StepIDNetworking    StepID = "networking"
	StepIDResources     StepID = "resources"
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

// ConfigApplier is implemented by steps that write their values into cfg.
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

// TerminalSizer is implemented by steps whose layout gate is stated in
// terminal columns and rows, which the frame's caps flatten out of the
// content box.
type TerminalSizer interface {
	SetTerminalSize(width, height int)
}

// HelpProvider is implemented by steps that supply their own help footer.
type HelpProvider interface {
	ShortHelp() []KeyBinding
}

// OverlayHelpProvider is implemented by steps with bindings that work but
// are listed only under the "?" overlay, not the footer.
type OverlayHelpProvider interface {
	OverlayHelp() []KeyBinding
}

// TextInputConsumer is implemented by steps whose focused field would take
// "?" as typed text rather than the help-overlay toggle; a step that does
// not implement it never consumes text input.
type TextInputConsumer interface {
	ConsumesTextInput() bool
}

// Secrets is wizard input that never enters the Config: the Proxmox API
// token and TLS toggle written to okdctl.env.
type Secrets struct {
	ProxmoxToken    credentials.SecretBytes
	ProxmoxInsecure bool
}

// SecretsProvider is implemented by steps that collect Secrets.
type SecretsProvider interface {
	Secrets() Secrets
}

// KeyBinding is a key/help pair used in the wizard footer.
type KeyBinding struct {
	Key  string
	Help string
}

// BaseStep implements the common WizardStep fields and defaults.
type BaseStep struct {
	visitContext context.Context
	id           StepID
	title        string
	displayTitle string
	description  string
	focused      bool
	width        int
	height       int
}

// NewBaseStep returns a BaseStep with the given id, title and description.
func NewBaseStep(id StepID, title, description string) BaseStep {
	return NewBaseStepWithDisplayTitle(id, title, "", description)
}

// NewBaseStepWithDisplayTitle returns a BaseStep whose header shows
// displayTitle while the progress indicator shows title.
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

// DisplayTitle returns the header title; empty falls back to Title().
func (b *BaseStep) DisplayTitle() string { return b.displayTitle }

// IsFocused reports whether the step owns input focus.
func (b *BaseStep) IsFocused() bool { return b.focused }

// ShouldShow always returns true.
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

// ShortHelp returns the default footer bindings.
func (b *BaseStep) ShortHelp() []KeyBinding {
	return []KeyBinding{
		{Key: "↑↓", Help: HelpNavigate},
		{Key: HelpEnter, Help: HelpConfirm},
		{Key: HelpEsc, Help: HelpBack},
		{Key: HelpCtrlC, Help: HelpQuit},
	}
}

// StepCompleteMsg signals that the named step is finished.
type StepCompleteMsg struct {
	StepID StepID
}

// StepBackMsg signals that the wizard should step back one position.
type StepBackMsg struct{}

// LayoutChangedMsg asks the wizard to re-measure the active step after a
// layout gate flipped without a terminal resize.
type LayoutChangedMsg struct{}

// ErrorSetMsg signals an error to display in the wizard's status row.
type ErrorSetMsg struct {
	Error error
}

// FocusChangedMsg signals that focus moved within the active step.
type FocusChangedMsg struct{}

// LineSpan is an inclusive range of 0-based lines into a step's View().
type LineSpan struct {
	Start int
	End   int
}

// SpanProvider is implemented by steps that report which View() lines the
// focused field occupies. Spans are recorded during View, so an unrendered
// step reports false.
type SpanProvider interface {
	FocusedSpan() (LineSpan, bool)
}

// ConfigSyncMsg applies the active step without advancing.
type ConfigSyncMsg struct {
	StepID StepID
}

// JumpTarget pairs a StepID with the 1-based digit that jumps to it from
// review; a hidden step consumes no digit.
type JumpTarget struct {
	StepID StepID
	Digit  int
}

// ReviewJumper is implemented by the review step: JumpOrder declares the
// jumpable steps and SetJumpTargets delivers the compacted digits on focus.
type ReviewJumper interface {
	JumpOrder() []StepID
	SetJumpTargets(targets []JumpTarget)
}

// JumpToStepMsg jumps to the named step; confirming or escaping there
// returns straight to review.
type JumpToStepMsg struct {
	StepID StepID
}

// FocusedBounds reports the active control's rendered line bounds.
type FocusedBounds interface {
	FocusBounds(width, height int) (top, bottom int, ok bool)
}

// SetVisitContext attaches the current visit's lifetime to step requests.
func (b *BaseStep) SetVisitContext(ctx context.Context) { b.visitContext = ctx }

// Context returns the current visit context.
func (b *BaseStep) Context() context.Context {
	if b.visitContext != nil {
		return b.visitContext
	}
	// Direct widget tests run outside the flow owner.
	return context.Background()
}

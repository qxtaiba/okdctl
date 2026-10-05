package wizard

import (
	"context"

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

// TerminalSizer is implemented by steps that lay themselves out against the
// terminal's own dimensions rather than the content box they render into — a
// gate stated in terminal columns and rows cannot be re-derived from the
// content width, which the frame's own caps flatten.
type TerminalSizer interface {
	SetTerminalSize(width, height int)
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

// OverlayHelpProvider is implemented by steps with a binding that works but
// is deliberately left out of the footer ribbon, listed instead under the
// "?" overlay's own screen section alongside ShortHelp's bindings.
type OverlayHelpProvider interface {
	OverlayHelp() []KeyBinding
}

// QuitGuard is implemented by steps that must intercept ctrl+c (e.g. graceful
// cancel on first press); returning true consumes the keypress, false quits normally.
type QuitGuard interface {
	InterceptQuit() bool
}

// TextInputConsumer is implemented by steps whose currently focused field
// would consume a "?" keypress as literal typed text rather than the
// wizard's help-overlay toggle; a step with no text fields need not
// implement it — an unasserted step never consumes text input, so "?"
// always opens the overlay there.
type TextInputConsumer interface {
	ConsumesTextInput() bool
}

// PaletteTargetKind distinguishes wizard steps, fields, and actions.
type PaletteTargetKind string

// PaletteTargetStep, PaletteTargetField, and PaletteTargetAction are command-palette target kinds.
const (
	PaletteTargetStep   PaletteTargetKind = "step"
	PaletteTargetField  PaletteTargetKind = "field"
	PaletteTargetAction PaletteTargetKind = "action"
)

// PaletteTarget is a safe navigation destination exposed by a step.
type PaletteTarget struct {
	ID     string
	Kind   PaletteTargetKind
	Label  string
	Detail string
}

// PaletteProvider exposes fields or actions without exposing their values.
type PaletteProvider interface {
	PaletteTargets() []PaletteTarget
	FocusPaletteTarget(id string) tea.Cmd
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
	visitContext context.Context
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
// displayTitle (shown in the header) plus title (the progress-indicator
// fallback used when displayTitle is empty).
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

// DisplayTitle returns the title shown in the header; an empty string
// falls back to Title().
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

// DraftResumeMsg resumes the configure flow at a saved step and field.
type DraftResumeMsg struct {
	StepID   StepID
	FieldKey string
}

// StepBackMsg signals that the wizard should step back one position.
type StepBackMsg struct{}

// LayoutChangedMsg asks the wizard to re-measure the active step: a step that
// flips its own layout gate — a full-screen toggle turning SuppressesSplit on —
// changes the body width without the terminal changing at all, and nothing else
// resizes the viewport before the next real resize.
type LayoutChangedMsg struct{}

// ErrorSetMsg signals an error to display in the wizard's status row, emitted
// on a failed step-level validation or when a ConfigApplier returns an error
// during a step transition.
type ErrorSetMsg struct {
	Error error
}

// FocusChangedMsg signals that focus has moved within the active step; the
// wizard resyncs the viewport and scrolls the focused field into view.
type FocusChangedMsg struct{}

// LineSpan is an inclusive range of 0-based line indices into a step's
// View() output.
type LineSpan struct {
	Start int
	End   int
}

// SpanProvider is implemented by steps that can report which lines of their
// View() the focused field occupies; the wizard scrolls that span into view
// on FocusChangedMsg. Spans are recorded during View, so a step that has
// not rendered yet reports false.
type SpanProvider interface {
	FocusedSpan() (LineSpan, bool)
}

// BottomNotifiable is implemented by a step that must not trust content
// below an initial fold until the wizard confirms the viewport has shown
// its last line at least once — a step has no visibility into the
// viewport's own scroll offset, so this is the only guarantee it can act on.
type BottomNotifiable interface {
	NotifyViewportAtBottom()
}

// ConfigSyncMsg applies the active step without advancing and persists its draft state.
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

// FocusedBounds identifies the active control in the step's rendered line coordinates.
type FocusedBounds interface {
	FocusBounds(width, height int) (top, bottom int, ok bool)
}

// SetVisitContext attaches the lifetime of the current visit to step requests.
func (b *BaseStep) SetVisitContext(ctx context.Context) { b.visitContext = ctx }

// Context returns the current visit context.
func (b *BaseStep) Context() context.Context {
	if b.visitContext != nil {
		return b.visitContext
	}
	// Direct widget tests and render probes run outside the flow owner.
	return context.Background()
}

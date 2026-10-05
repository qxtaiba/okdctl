package wizard

// Help-text labels shared across wizard keymaps so footer wording stays consistent.
//
// The enter-key label follows one rule across every step's ShortHelp:
// HelpContinue ("continue") on a form step — one that collects values across
// one or more fields before the wizard can advance, e.g. a DataDrivenStep or
// ParamsStep; HelpConfirm ("confirm") on a decision or summary step — one
// where enter commits a single already-made choice or a review, e.g. picking
// one item from a list (OpStep, TargetStep, DistributionStep) or the final
// review/type-to-confirm gates; and HelpStart ("start") nowhere but the
// welcome hub, which begins the wizard rather than advancing or confirming
// anything. Every step's ShortHelp must also include HelpCtrlC — the
// essentials system needs the binding present in the slice to render it, even
// though the keystroke itself is handled globally regardless.
const (
	HelpNavigate  = "navigate"
	HelpEnter     = "enter"
	HelpEsc       = "esc"
	HelpConfirm   = "confirm"
	HelpBack      = "back"
	HelpQuit      = "quit"
	HelpCtrlC     = "ctrl+c"
	HelpContinue  = "continue"
	HelpStart     = "start"
	HelpJump      = "jump to section"
	HelpLeftRight = "←/→"
	HelpChoose    = "choose"
	HelpQuestion  = "?"
	HelpOverlay   = "help"
)

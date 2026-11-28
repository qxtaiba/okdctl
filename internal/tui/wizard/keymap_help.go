package wizard

// Help labels shared across step keymaps. The enter label is HelpContinue on
// a form step, HelpConfirm on a decision or summary step and HelpStart on
// the hub alone; every ShortHelp must include HelpCtrlC for the essentials
// reserve to render it.
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

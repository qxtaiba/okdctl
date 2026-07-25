package tui

// Status glyphs rendered next to completion messages. One frozen set is
// shared by the CLI and the wizard so a single semantic never renders under
// two glyphs (the wizard previously carried its own ✓ for success).
const (
	IconSuccess = "✔"
	IconError   = "✖"
	IconWarning = "⚠"
	IconSkip    = "↷"
	IconPending = "○"
	IconActive  = "●"
	IconPointer = "→"
)

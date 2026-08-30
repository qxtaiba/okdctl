package tui

// Status glyphs shared by the CLI and wizard so a semantic never renders
// under two glyphs (the wizard previously had its own ✓ for success).
const (
	IconSuccess = "✔"
	IconError   = "✖"
	IconWarning = "⚠"
	IconSkip    = "↷"
	IconPending = "○"
	IconActive  = "●"
	IconPointer = "→"
)

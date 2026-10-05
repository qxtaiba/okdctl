package tui

// Status glyphs shared by the CLI and wizard; the only place a status glyph may be spelled.
const (
	IconSuccess    = "✓"
	IconError      = "✗"
	IconWarning    = "⚠"
	IconSkip       = "↷"
	IconPending    = "○"
	IconActive     = "●"
	IconPointer    = "→"
	IconCaretLeft  = "◂"
	IconCaretRight = "▸"
	IconBar        = "┃"
	IconBullet     = "•"
)

// IconLevelInfo is the log gutter's quiet-stream mark; warn and error rows
// carry their initial letter instead, so severity survives NO_COLOR in one
// column. The AST guard cannot police this glyph — a middle dot is also the
// house's text separator — so spell it from here by discipline.
const IconLevelInfo = "·"

// Progress-bar glyphs: the filled cell, the empty track, and the
// phase-boundary separator. The log minimap uses IconBarTick as its window
// thumb and IconBarSegment as its track, with IconBar marking a severity.
const (
	IconBarFill    = "█"
	IconBarTrack   = "░"
	IconBarSegment = "│"
	IconBarTick    = "▎"
)

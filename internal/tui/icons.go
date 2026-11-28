package tui

// Status glyphs shared by the CLI and wizard; the only place one may be spelled.
const (
	IconSuccess    = "✓"
	IconError      = "✗"
	IconWarning    = "⚠"
	IconPending    = "○"
	IconActive     = "●"
	IconPointer    = "→"
	IconCaretLeft  = "◂"
	IconCaretRight = "▸"
	IconBar        = "┃"
	IconBullet     = "•"
)

// IconLevelInfo marks the log gutter's quiet stream; warn and error rows
// carry their initial so severity survives NO_COLOR.
const IconLevelInfo = "·"

// Progress-bar and minimap glyphs.
const (
	IconBarFill    = "█"
	IconBarTrack   = "░"
	IconBarSegment = "│"
	IconBarTick    = "▎"
)

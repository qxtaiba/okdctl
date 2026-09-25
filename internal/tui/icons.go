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

// Progress-bar glyphs: the filled cell, the empty track, the phase-boundary
// separator, the last-run tick, and the eighth-block ramp that renders a
// fractional final cell.
const (
	IconBarFill    = "█"
	IconBarTrack   = "░"
	IconBarSegment = "│"
	IconBarTick    = "▎"
	IconBarEighths = "▏▎▍▌▋▊▉"
)

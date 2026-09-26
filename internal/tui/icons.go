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
	IconTextCursor = "▏"
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

const IconLatencySparkline = "▁▂▃▄▅▆▇█"

// Progress-bar glyphs: the filled cell, the empty track, the phase-boundary
// separator, the last-run tick, and the eighth-block ramp that renders a
// fractional final cell. The log minimap reuses IconBarTick as its window
// thumb and IconBarSegment as its track, with IconBar marking a severity.
const (
	IconBarFill    = "█"
	IconBarTrack   = "░"
	IconBarSegment = "│"
	IconBarTick    = "▎"
	IconBarEighths = "▏▎▍▌▋▊▉"
)

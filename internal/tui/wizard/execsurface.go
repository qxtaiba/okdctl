package wizard

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/logview"
)

// ExecStyles is an exec surface's themed style set — the deploy stream, the
// lifecycle execution screen, and the completion screens either one ends on
// all paint from this one set, so the two long-running flows read as one
// system. Resolve it through ExecStyleCache at render time, never at
// construction: a CLI-launched flow assembles its steps before the
// terminal's background reply flips the theme.
type ExecStyles struct {
	Bold   lipgloss.Style
	Done   lipgloss.Style
	Fail   lipgloss.Style
	Pend   lipgloss.Style
	Dim    lipgloss.Style
	Warn   lipgloss.Style
	Active lipgloss.Style
}

// NewExecStyles resolves the exec surfaces' style set against the active theme.
func NewExecStyles() ExecStyles {
	return ExecStyles{
		Bold:   lipgloss.NewStyle().Foreground(tui.ColorText()).Bold(true),
		Done:   lipgloss.NewStyle().Foreground(tui.ColorSuccess()),
		Fail:   lipgloss.NewStyle().Foreground(tui.ColorError()),
		Pend:   lipgloss.NewStyle().Foreground(tui.ColorSubtle()),
		Dim:    lipgloss.NewStyle().Foreground(tui.ColorTextFaint()),
		Warn:   lipgloss.NewStyle().Foreground(tui.ColorWarning()),
		Active: lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true),
	}
}

// ExecStyleCache holds one ExecStyles rebuilt whenever the theme generation
// moves; embed it in an exec step and resolve through Styles inside View.
type ExecStyleCache struct {
	cache ExecStyles
	gen   uint64
}

// Styles returns the cached exec style set, rebuilt when the theme
// generation has moved since the last call.
func (c *ExecStyleCache) Styles() *ExecStyles {
	if gen := tui.ThemeGeneration(); gen != c.gen {
		c.cache = NewExecStyles()
		c.gen = gen
	}
	return &c.cache
}

// LogHelp assembles the help ribbon for a step whose body carries a log
// surface, followed by the step's own trailing bindings. The surface's keys
// are ordered so the one that leaves the current mode leads the list: the
// ribbon drops entries front to back, and losing the way out would strand the
// operator on whichever window they opened.
func LogHelp(s *logview.Surface, trailing ...KeyBinding) []KeyBinding {
	lock := KeyBinding{Key: string(rune(logview.KeyLock)), Help: s.LockHelp()}
	full := KeyBinding{Key: string(rune(logview.KeyFull)), Help: s.FullHelp()}
	page := KeyBinding{Key: "pgup/pgdn", Help: "page the log"}
	find := KeyBinding{Key: string(rune(logview.KeyFilter)), Help: s.FilterHelp()}
	jump := KeyBinding{Key: "n/N", Help: s.JumpHelp()}

	var keys []KeyBinding
	switch {
	case s.Filtering():
		keys = []KeyBinding{{Key: HelpEnter, Help: s.FilterHelp()}, {Key: HelpEsc, Help: "cancel it"}}
	case s.Full():
		keys = []KeyBinding{full, lock, page, find, jump}
	case s.Locked():
		keys = []KeyBinding{lock, full, page, jump, find}
	case s.Filtered():
		keys = []KeyBinding{find, jump, lock, full}
	default:
		// No mode to leave here, so the ribbon leads with the key an operator
		// is least likely to guess: the lock is what paging engages by itself.
		keys = []KeyBinding{find, lock, full}
	}
	return append(keys, trailing...)
}

const (
	// sideLogMinWidth is the terminal width from which an exec screen puts
	// its log beside its body instead of under it.
	sideLogMinWidth = 150
	sideLogInset    = 2
	sideLogGutter   = 2

	// SideLogBodyWidth is the width an exec screen renders its own body at
	// while the log sits beside it.
	SideLogBodyWidth = 100
)

// FrameSize records the terminal width an exec step is laid out against and
// the body box the frame gives it — neither of which View's own arguments
// report, since the frame calls View with a fixed 1000-row scratch budget
// and a width its caps have already flattened.
type FrameSize struct {
	termWidth  int
	bodyHeight int
}

// SetTerminalSize records the terminal's own width, which gates the side log.
func (f *FrameSize) SetTerminalSize(width, _ int) {
	f.termWidth = width
}

// SetBodyHeight records the rows the frame gave the step's body.
func (f *FrameSize) SetBodyHeight(height int) {
	f.bodyHeight = height
}

// BodyHeight reports the body box's rows, zero until the frame has sized the
// step at least once.
func (f *FrameSize) BodyHeight() int {
	return f.bodyHeight
}

// SideLog reports whether log sits beside the step's body as a column of its
// own: the terminal is wide enough, and the log is neither absent nor
// full-screen. A step for which it holds must own the frame's width.
func (f *FrameSize) SideLog(log *logview.Surface) bool {
	return f.termWidth >= sideLogMinWidth && log.Src != nil && !log.Full()
}

// WithSideLog joins body, rendered at SideLogBodyWidth, with a rule and the
// log as a column of the body box's height, filling width columns. A body
// taller than the box scrolls the log column with it.
func (f *FrameSize) WithSideLog(body string, log *logview.Surface, width int) string {
	rows := max(lipgloss.Height(body), f.bodyHeight)
	logHeight := f.bodyHeight
	if logHeight < 1 {
		logHeight = rows
	}
	bodyWidth := SideLogBodyWidth + sideLogInset
	logWidth := max(width-bodyWidth-lipgloss.Width(tui.IconBarSegment)-sideLogGutter, 1)

	rule := lipgloss.NewStyle().Foreground(tui.ColorRule()).
		Render(strings.TrimSuffix(strings.Repeat(tui.IconBarSegment+"\n", rows), "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(bodyWidth).Render(body),
		rule,
		strings.Repeat(" ", sideLogGutter),
		log.RenderSide(logWidth, logHeight),
	)
}

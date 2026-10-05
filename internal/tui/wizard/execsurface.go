package wizard

import (
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

// FrameSize records the body box the frame gives an exec step, which View's
// own arguments do not report, since the frame calls View with a fixed
// 1000-row scratch budget.
type FrameSize struct {
	bodyHeight int
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

package wizard

import (
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
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

// FrameSize records the terminal an exec step is laid out against plus the
// body box the frame gives it — neither of which View's own arguments
// report, since the frame calls View with a fixed 1000-row scratch budget
// and a width its caps have already flattened.
type FrameSize struct {
	termWidth, termHeight int
	bodyHeight            int
}

// SetTerminalSize records the terminal's own dimensions.
func (f *FrameSize) SetTerminalSize(width, height int) {
	f.termWidth, f.termHeight = width, height
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

// SplitsFrame reports whether this terminal gives a stepCount-step flow a
// right-hand pane; the flow passes its own screen count, the one part of the
// gate that is not shared.
func (f *FrameSize) SplitsFrame(stepCount int) bool {
	return SplitsFrame(f.termWidth, f.termHeight, stepCount)
}

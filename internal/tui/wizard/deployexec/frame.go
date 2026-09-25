package deployexec

import (
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// flowStepCount is how many screens NewSteps assembles, the step count the
// frame's split gate is evaluated against; TestFlowStepCountMatchesNewSteps
// pins it.
const flowStepCount = 2

// frameSize records the terminal a step is laid out against plus the body box
// the frame gives it — neither of which View's own arguments report, since the
// frame calls View with a fixed 1000-row budget and a width its caps have
// already flattened.
type frameSize struct {
	termWidth, termHeight int
	bodyHeight            int
}

// SetTerminalSize records the terminal's own dimensions.
func (f *frameSize) SetTerminalSize(width, height int) {
	f.termWidth, f.termHeight = width, height
}

// splitsFrame reports whether this terminal gives the flow a right-hand pane.
func (f *frameSize) splitsFrame() bool {
	return wizard.SplitsFrame(f.termWidth, f.termHeight, flowStepCount)
}

package wizard

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// framePeriod is the full dial's cadence, one glyph per frame.
const framePeriod = 80 * time.Millisecond

// blurredFramePeriod keeps elapsed readings honest at near-zero CPU.
const blurredFramePeriod = time.Second

// FrameMsg advances every live animation on the active step by one frame;
// Frame is monotonic for the life of the model. Tests send FrameMsg with
// pinned counters, never wait on wall time.
type FrameMsg struct {
	Frame uint64
}

// Animator is implemented by steps that need frame ticks; the clock runs
// only while the active step reports true.
type Animator interface {
	Animating() bool
}

// clockTickMsg is the clock chain's wire message; gen drops ticks from a
// stopped chain.
type clockTickMsg struct {
	gen uint64
}

func (m *Model) currentStepAnimating() bool {
	if len(m.steps) == 0 || m.currentStep < 0 || m.currentStep >= len(m.steps) {
		return false
	}
	a, ok := m.steps[m.currentStep].(Animator)
	return ok && a.Animating()
}

// armClock starts the frame clock when the active step animates and no
// chain is live; nil under MotionOff.
func (m *Model) armClock() tea.Cmd {
	if m.motion == tui.MotionOff || m.clockRunning || !m.currentStepAnimating() {
		return nil
	}
	m.clockRunning = true
	m.clockGen++
	return m.clockTickCmd()
}

func (m *Model) clockTickCmd() tea.Cmd {
	gen := m.clockGen
	return tea.Tick(m.clockPeriod(), func(time.Time) tea.Msg { return clockTickMsg{gen: gen} })
}

func (m *Model) clockPeriod() time.Duration {
	if m.blurred {
		return blurredFramePeriod
	}
	return framePeriod
}

// handleClockTick advances the frame counter, delivers the frame through
// the normal update path and re-arms while the step keeps animating.
func (m *Model) handleClockTick(msg clockTickMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.clockGen {
		return m, nil
	}
	m.frame++
	model, cmd := m.update(FrameMsg{Frame: m.frame})
	if m.currentStepAnimating() {
		return model, tea.Batch(cmd, m.clockTickCmd())
	}
	m.clockRunning = false
	return model, cmd
}

// Spinner renders the shared clock's spinner glyph for frame.
func Spinner(frame uint64) string {
	return lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Render(tui.SpinnerGlyph(tui.Motion(), frame) + " ")
}

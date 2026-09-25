package wizard

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// framePeriod is the shared clock's tick interval: 12.5Hz, one glyph per
// frame at the full motion dial.
const framePeriod = 80 * time.Millisecond

// blurredFramePeriod is the shared clock's cadence while the terminal is
// unfocused: 1Hz keeps elapsed and stall readings honest at near-zero CPU
// while the cosmetic animators stand suspended.
const blurredFramePeriod = time.Second

// FrameMsg advances every live animation on the active step by one frame of
// the wizard's shared clock; Frame is monotonic for the life of the model.
// Tests drive animations deterministically by sending FrameMsg with pinned
// counters — never by waiting on wall time.
type FrameMsg struct {
	Frame uint64
}

// Animator is implemented by steps that need frame ticks while something on
// them is moving; the wizard's clock runs only while the active step reports
// true and stops on the first tick it doesn't.
type Animator interface {
	Animating() bool
}

// clockTickMsg is the clock chain's wall-time wire message; gen identifies
// the chain so a tick from a stopped chain is dropped instead of double-arming.
type clockTickMsg struct {
	gen uint64
}

// currentStepAnimating reports whether the active step wants frame ticks.
func (m *Model) currentStepAnimating() bool {
	if len(m.steps) == 0 || m.currentStep < 0 || m.currentStep >= len(m.steps) {
		return false
	}
	a, ok := m.steps[m.currentStep].(Animator)
	return ok && a.Animating()
}

// armClock starts the frame clock when the active step is animating and no
// chain is live; it returns nil under MotionOff (the dial's hard stop) and
// while a chain is already running.
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

// clockPeriod is the shared clock's active tick interval: away mode's 1Hz
// while the terminal is blurred, the full 12.5Hz otherwise.
func (m *Model) clockPeriod() time.Duration {
	if m.blurred {
		return blurredFramePeriod
	}
	return framePeriod
}

// handleClockTick advances the monotonic frame counter, delivers the frame
// to the active step through the normal update path (so the viewport
// resyncs), and re-arms the chain only while the step keeps animating.
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

// Spinner renders the shared clock's spinner glyph for frame — brand
// colored, trailing-space padded the way checklist rows compose it.
func Spinner(frame uint64) string {
	return lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Render(tui.SpinnerGlyph(tui.Motion(), frame) + " ")
}

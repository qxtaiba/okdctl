package deployexec

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// barCapFrac caps the bar until the final event: the last percent belongs
// to the engine's own completion, never to an optimistic weight sum.
const barCapFrac = 0.96

// etaEWMAAlpha is the weight one settled step's actual/predicted ratio
// carries in the ETA's schedule correction.
const etaEWMAAlpha = 0.3

// quietAfter is how long the activity meter waits after the last observed
// output before the running glyph decays to the slow pulse.
const quietAfter = 5 * time.Second

// slowPulseDivisor slows the quiet pulse (and the stall marker's blink) to
// one glyph advance per eight frames of the shared clock.
const slowPulseDivisor = 8

// settleFrames is how many shared-clock frames the settle-to-100% (and the
// away-mode catch-up sweep) plays over: five frames ≈ 400ms at 12.5Hz.
const settleFrames = 5

// settleOmegaPerFrame is the critically-damped easing's natural frequency
// expressed per frame (ω·Δt with ω=15/s at the 80ms frame period), chosen
// so the fill reaches ~95% of the way by settleFrames.
const settleOmegaPerFrame = 1.2

// defaultStallAfter is the silence threshold for steps without their own
// entry: three minutes covers terraform applies and ISO builds that work
// quietly, while a genuinely wedged step still surfaces within the wait.
const defaultStallAfter = 180 * time.Second

// stallThresholds names the per-step-class silence thresholds; the
// bootstrap wait logs continuously when healthy, so 90s of silence there is
// a wedge signal worth surfacing minutes earlier.
var stallThresholds = map[distribution.StepID]time.Duration{
	install.StepWaitBootstrap: 90 * time.Second,
}

// buildWeights seeds the per-step weight table from the persisted duration
// history: a known step weighs its historical seconds, an unknown one the
// median of the known weights, and a run with no history at all weighs
// every step 1 — the bar then advances by honest step count, and the ETA
// stays off entirely.
func (s *StreamStep) buildWeights() {
	s.hasHistory = len(s.st.History) > 0
	s.weights = make(map[distribution.StepID]float64, len(s.st.Plan))
	s.totalWeight = 0

	var known []float64
	for _, m := range s.st.Plan {
		if d, ok := s.st.History[m.ID]; ok && d > 0 {
			known = append(known, d.Seconds())
		}
	}
	fallback := 1.0
	if len(known) > 0 {
		fallback = median(known)
	}
	for _, m := range s.st.Plan {
		w := fallback
		if d, ok := s.st.History[m.ID]; ok && d > 0 {
			w = d.Seconds()
		}
		s.weights[m.ID] = w
		s.totalWeight += w
	}
}

func median(v []float64) float64 {
	sorted := append([]float64(nil), v...)
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted[len(sorted)/2]
}

// settledFrac is the run's weighted completion fraction, advanced only by
// settled rows — real StepResult events, never wall time — and monotonic
// through maxFrac even if a weight re-derivation would move it backwards.
func (s *StreamStep) settledFrac() float64 {
	if s.totalWeight <= 0 {
		return 0
	}
	settled := 0.0
	for i := range s.phases {
		for j := range s.phases[i].rows {
			r := &s.phases[i].rows[j]
			if r.status == rowDone || r.status == rowSkipped || r.status == rowFailed {
				settled += s.weights[r.id]
			}
		}
	}
	if frac := settled / s.totalWeight; frac > s.maxFrac {
		s.maxFrac = frac
	}
	return s.maxFrac
}

// percent is the whole-number progress the bar's right side and the window
// title carry: weighted, capped at 96 until the final event, 100 only once
// the engine itself has finished cleanly.
func (s *StreamStep) percent() int {
	if s.finished && s.st.Result == nil {
		return 100
	}
	return int(math.Floor(min(s.settledFrac(), barCapFrac) * 100))
}

// barTarget is the fill fraction the bar aims at; the settle to 1.0 happens
// through barFillFrac's easing, everything before it renders the capped
// weighted fraction directly.
func (s *StreamStep) barTarget() float64 {
	if s.finished && s.st.Result == nil {
		return 1
	}
	return min(s.settledFrac(), barCapFrac)
}

// barFillFrac is the fraction of the bar painted this frame: the target,
// except during the settle-to-100% and the away-mode catch-up sweep, which
// ease toward it on the shared clock — a critically-damped step, pure in
// (from, target, frames elapsed).
func (s *StreamStep) barFillFrac() float64 {
	target := s.barTarget()
	if (s.settling || s.sweeping) && tui.Motion() == tui.MotionFull {
		return damped(s.animFrom, target, s.frame-s.animStart)
	}
	return target
}

// damped moves from toward target along the critically-damped step
// response 1−(1+ωn)e^(−ωn), n frames in.
func damped(from, target float64, frames uint64) float64 {
	wn := settleOmegaPerFrame * float64(frames)
	k := 1 - (1+wn)*math.Exp(-wn)
	return from + (target-from)*k
}

// etaLeft estimates the remaining run time from the unsettled weights,
// EWMA-corrected by how the settled steps ran against their own history. It
// reports false — and the bar shows no ETA — until the schedule is honest:
// a history exists, the first phase has settled, and the run is still going.
func (s *StreamStep) etaLeft() (time.Duration, bool) {
	if !s.hasHistory || s.finished || len(s.phases) == 0 || !s.firstPhaseSettled() {
		return 0, false
	}
	if id, ok := s.runningStepID(); ok && id == install.StepWaitBootstrap {
		return 0, false
	}
	now := s.now()
	rem := 0.0
	for i := range s.phases {
		for j := range s.phases[i].rows {
			r := &s.phases[i].rows[j]
			switch r.status {
			case rowPending:
				rem += s.weights[r.id]
			case rowRunning:
				rem += max(s.weights[r.id]-now.Sub(r.start).Seconds(), 0)
			}
		}
	}
	rem *= s.ewmaRatio
	if rem <= 0 {
		return 0, false
	}
	return time.Duration(rem * float64(time.Second)), true
}

// firstPhaseSettled reports whether the run is past its first phase — the
// point the proposal lets the ETA start speaking.
func (s *StreamStep) firstPhaseSettled() bool {
	if s.currentPhase > 0 {
		return true
	}
	ph := &s.phases[0]
	for i := range ph.rows {
		if st := ph.rows[i].status; st == rowPending || st == rowRunning {
			return false
		}
	}
	return true
}

// updateETAShown refreshes the displayed ETA under the no-upward-twitch
// contract: a lower estimate shows immediately, a higher one only once it
// exceeds the shown value by a quarter — a genuine schedule slip, not
// jitter the coarse units would flicker on.
func (s *StreamStep) updateETAShown() {
	eta, ok := s.etaLeft()
	if !ok {
		s.etaShown = 0
		return
	}
	if s.etaShown == 0 || eta < s.etaShown || eta > s.etaShown*5/4 {
		s.etaShown = eta
	}
}

// noteActivity records one unit of observed throughput at the injected
// clock's now, re-arming the stall bell.
func (s *StreamStep) noteActivity() {
	s.activityCount++
	s.lastActivity = s.now()
	s.stallRung = false
}

// sampleActivity folds new log lines into the activity meter: the ring's
// absolute total is monotonic, so its growth since the last sample is
// exactly the output the operator would have seen scroll by.
func (s *StreamStep) sampleActivity() {
	if s.hooks.Logs == nil {
		return
	}
	lines, first := s.hooks.Logs.Snapshot()
	total := first + int64(len(lines))
	if total > s.lastLogTotal {
		s.activityCount += uint64(total - s.lastLogTotal) //nolint:gosec // G115: guarded by the comparison above
		s.lastLogTotal = total
		s.lastActivity = s.now()
		s.stallRung = false
	}
}

// runningGlyph renders the running row's activity meter: the spinner glyph
// advances one frame per observed event while output flows — motion
// frequency IS throughput — and decays to a slow, dim, frame-clock pulse
// once the step has been quiet past quietAfter. Off-motion renders frame
// zero either way (tui.SpinnerGlyph's own contract), and a blurred terminal
// freezes the pulse — away mode suspends the cosmetic animators.
func (s *StreamStep) runningGlyph() string {
	idx := s.activityCount
	quiet := s.now().Sub(s.lastActivity) >= quietAfter
	if quiet {
		idx = s.frame / slowPulseDivisor
		if s.blurred {
			idx = 0
		}
	}
	g := tui.SpinnerGlyph(tui.Motion(), idx)
	if quiet {
		return s.Styles().Dim.Render(g + " ")
	}
	return s.Styles().Active.Render(g + " ")
}

// stalled reports whether the running step has been silent past its
// step-class threshold — a wedged install made visually distinct minutes
// before a timeout would say so.
func (s *StreamStep) stalled(id distribution.StepID) bool {
	after := defaultStallAfter
	if t, ok := stallThresholds[id]; ok {
		after = t
	}
	return s.now().Sub(s.lastActivity) >= after
}

// stallMarker renders the stalled row's amber boundary marker, pulsing on
// the slow cadence under full motion — steady on the calmer dials and while
// the terminal is blurred.
func (s *StreamStep) stallMarker() string {
	glyph := tui.IconActive
	if tui.Motion() == tui.MotionFull && !s.blurred && (s.frame/slowPulseDivisor)%2 == 1 {
		glyph = tui.IconPending
	}
	return s.Styles().Warn.Render(glyph + " ")
}

// runningStepID names the step currently running, false when none is.
func (s *StreamStep) runningStepID() (distribution.StepID, bool) {
	if len(s.phases) == 0 {
		return "", false
	}
	ph := &s.phases[s.currentPhase]
	for i := range ph.rows {
		if ph.rows[i].status == rowRunning {
			return ph.rows[i].id, true
		}
	}
	return "", false
}

// TerminalProgress drives the terminal's own progress indication (OSC 9;4)
// per the proposal: the weight model's percent while running, indeterminate
// during the bootstrap wait, the error state on failure, and 100 on a clean
// finish — the frame clears it once the flow moves on.
func (s *StreamStep) TerminalProgress() (state tea.ProgressBarState, value int) {
	switch {
	case s.finished && s.st.Result != nil:
		return tea.ProgressBarError, s.percent()
	case s.finished:
		return tea.ProgressBarDefault, 100
	case len(s.phases) == 0:
		return tea.ProgressBarNone, 0
	default:
		if id, ok := s.runningStepID(); ok && id == install.StepWaitBootstrap {
			return tea.ProgressBarIndeterminate, 0
		}
		return tea.ProgressBarDefault, s.percent()
	}
}

// bell rings the terminal bell, gated the way every escape emission is: a
// pipe or NO_COLOR run degrades to nothing and stays byte-identical.
func (s *StreamStep) bell() tea.Cmd {
	if !tui.ColorEnabled() {
		return nil
	}
	return func() tea.Msg {
		_, _ = os.Stdout.WriteString("\a")
		return nil
	}
}

// bellWhileBlurred is the attention escape for real events that land while
// the operator is away; a focused terminal needs no bell.
func (s *StreamStep) bellWhileBlurred() tea.Cmd {
	if !s.blurred {
		return nil
	}
	return s.bell()
}

func (s *StreamStep) notifyWhileBlurred(message string) tea.Cmd {
	if !s.blurred || s.awayCancel == nil || !tui.ColorEnabled() {
		return nil
	}
	cancel := s.awayCancel
	return func() tea.Msg {
		select {
		case <-cancel:
			return nil
		default:
		}
		_, _ = os.Stdout.WriteString("\x1b]9;" + message + "\x1b\\")
		return nil
	}
}

func deployFinishedText(err error) string {
	if err != nil {
		return "Deploy needs attention"
	}
	return "Deploy complete"
}

func (s *StreamStep) phaseNotificationText() string {
	text := "Deploy phase complete"
	if eta, ok := s.etaLeft(); ok {
		text += " · ~" + fmtETA(eta) + " left"
	}
	return text
}

func (s *StreamStep) cancelAwayNotifications() {
	if s.awayCancel != nil {
		close(s.awayCancel)
		s.awayCancel = nil
	}
}

// stallBell rings once per stall while blurred: the first frame past the
// running step's silence threshold, re-armed only by new output.
func (s *StreamStep) stallBell() tea.Cmd {
	if !s.blurred || s.stallRung || s.finished {
		return nil
	}
	id, ok := s.runningStepID()
	if !ok || !s.stalled(id) {
		return nil
	}
	s.stallRung = true
	return s.bellWhileBlurred()
}

// renderBar renders the full-width segmented progress bar: gradient fill
// blended across the hero's LogoGradient endpoints, phase boundaries marked
// by dim separators at their weight positions, the empty track subtle, and
// the weighted percent (plus the ETA once it is honest) right of it.
func (s *StreamStep) renderBar(col int) string {
	if len(s.phases) == 0 || s.totalWeight <= 0 {
		return ""
	}
	right := s.barRightText()
	barW := col - lipgloss.Width(right) - 2
	if barW < 10 {
		return right
	}
	return s.renderBarCells(barW) + "  " + right
}

// barRightText is the bar's right-side reading: "42%", with "· ~28m left"
// beside it once the ETA is speaking.
func (s *StreamStep) barRightText() string {
	pct := fmt.Sprintf("%d%%", s.percent())
	if s.etaShown > 0 && !s.finished {
		return s.Styles().Dim.Render(pct + " · ~" + fmtETA(s.etaShown) + " left")
	}
	return s.Styles().Dim.Render(pct)
}

// renderBarCells paints the bar's barW cells: separators at the cumulative
// phase-weight boundaries, gradient fill up to the eased fill fraction, and
// the lighten band drifting leftward across the filled region under full
// motion only — the reduced and off dials render a static fill.
func (s *StreamStep) renderBarCells(barW int) string {
	seps := s.segmentCells(barW)
	fillW := int(math.Round(s.barFillFrac() * float64(barW)))

	bandStart, bandEnd := -1, -1
	if tui.Motion() == tui.MotionFull && !s.blurred && fillW > 3 && !s.finished {
		const bandW = 3
		offset := int(s.frame) % (fillW + bandW) //nolint:gosec // G115: frame modulo a small span
		bandEnd = fillW - 1 - offset + bandW - 1
		bandStart = fillW - 1 - offset
	}

	sepStyle := lipgloss.NewStyle().Foreground(tui.ColorRule())
	trackStyle := lipgloss.NewStyle().Foreground(tui.ColorSubtle())
	var b strings.Builder
	for i := range barW {
		switch {
		case seps[i]:
			b.WriteString(sepStyle.Render(tui.IconBarSegment))
		case i < fillW:
			c := tui.BlendAt(tui.LogoGradient[0], tui.LogoGradient[len(tui.LogoGradient)-1], float64(i)/float64(barW))
			if i >= bandStart && i <= bandEnd {
				c = tui.Lighten(c, 0.35)
			}
			b.WriteString(lipgloss.NewStyle().Foreground(c).Render(tui.IconBarFill))
		default:
			b.WriteString(trackStyle.Render(tui.IconBarTrack))
		}
	}
	return b.String()
}

// segmentCells marks the bar cells that carry a phase-boundary separator:
// one per boundary between rendered phases, at its cumulative weight
// position, clamped inside the bar and deduplicated.
func (s *StreamStep) segmentCells(barW int) map[int]bool {
	seps := make(map[int]bool, len(s.phases))
	cum := 0.0
	for i := range s.phases[:len(s.phases)-1] {
		for j := range s.phases[i].rows {
			cum += s.weights[s.phases[i].rows[j].id]
		}
		pos := min(max(int(math.Round(cum/s.totalWeight*float64(barW))), 1), barW-2)
		seps[pos] = true
	}
	return seps
}

// phasePredicted is the phase's scheduled duration from history; zero on a
// first install, where no honest number exists.
func (s *StreamStep) phasePredicted(ph *phaseProgress) time.Duration {
	if !s.hasHistory {
		return 0
	}
	secs := 0.0
	for i := range ph.rows {
		secs += s.weights[ph.rows[i].id]
	}
	return time.Duration(secs * float64(time.Second))
}

// phaseBarRows is how many body rows the phase duration block needs: a
// blank spacer, the header, and one row per phase.
func (s *StreamStep) phaseBarRows() int {
	return 2 + len(s.phases)
}

// renderPhaseBars fills the checklist's dead lower rows on the split tier
// with per-phase duration bars: eighth-block fill for time already spent,
// a dim tick marking last run's duration, elapsed or scheduled time on the
// right — the checklist doubles as a schedule.
func (s *StreamStep) renderPhaseBars(col int) []string {
	now := s.now()
	const labelW, barW = 10, 24
	if col < labelW+barW+8 {
		return nil
	}

	scale := 1.0
	elapsed := make([]float64, len(s.phases))
	predicted := make([]float64, len(s.phases))
	for i := range s.phases {
		ph := &s.phases[i]
		switch {
		case !ph.start.IsZero() && !ph.end.IsZero():
			elapsed[i] = ph.end.Sub(ph.start).Seconds()
		case !ph.start.IsZero():
			elapsed[i] = now.Sub(ph.start).Seconds()
		}
		predicted[i] = s.phasePredicted(ph).Seconds()
		scale = max(scale, elapsed[i], predicted[i])
	}

	head := lipgloss.NewStyle().Foreground(tui.ColorTextFaint()).Render("PHASES")
	lines := []string{"", head}
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorTextDim())
	for i := range s.phases {
		bar := eighthBar(elapsed[i]/scale, barW, tickCell(predicted[i]/scale, barW))
		right := s.phaseBarReading(i, elapsed[i])
		label := labelStyle.Render(fmt.Sprintf("%-*s", labelW, string(s.phases[i].name)))
		lines = append(lines, justify(label+bar, right, col))
	}
	return lines
}

// phaseBarReading is a phase bar's right-side number: measured time for a
// phase that has run, its schedule for one still ahead, nothing without
// history.
func (s *StreamStep) phaseBarReading(i int, elapsedSecs float64) string {
	if elapsedSecs > 0 {
		return s.Styles().Dim.Render(fmtDur(time.Duration(elapsedSecs * float64(time.Second))))
	}
	if d := s.phasePredicted(&s.phases[i]); d > 0 {
		return s.Styles().Dim.Render("~" + fmtETA(d))
	}
	return ""
}

// tickCell is the bar cell the last-run tick lands on, -1 when there is no
// schedule to mark.
func tickCell(frac float64, barW int) int {
	if frac <= 0 {
		return -1
	}
	return min(int(frac*float64(barW)), barW-1)
}

// eighthBar renders frac of barW cells as an eighth-block bar, overlaying
// the dim last-run tick on its cell when that cell is not already filled.
func eighthBar(frac float64, barW, tick int) string {
	frac = min(max(frac, 0), 1)
	eighths := int(math.Round(frac * float64(barW) * 8))
	full := eighths / 8
	rem := eighths % 8

	fillStyle := lipgloss.NewStyle().Foreground(tui.ColorAccent())
	tickStyle := lipgloss.NewStyle().Foreground(tui.ColorTextFaint())
	partials := []rune(tui.IconBarEighths)

	var b strings.Builder
	for i := range barW {
		switch {
		case i < full:
			b.WriteString(fillStyle.Render(tui.IconBarFill))
		case i == full && rem > 0:
			b.WriteString(fillStyle.Render(string(partials[rem-1])))
		case i == tick:
			b.WriteString(tickStyle.Render(tui.IconBarTick))
		default:
			b.WriteString(" ")
		}
	}
	return b.String()
}

// fmtETA renders a schedule duration in deliberately coarse units: whole
// minutes, hours and minutes above an hour, and a one-minute floor — a
// seconds-precise forecast would be a lie.
func fmtETA(d time.Duration) string {
	if d < 90*time.Second {
		return "1m"
	}
	m := int(d.Round(time.Minute).Minutes())
	if m < 60 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh%02dm", m/60, m%60)
}

// fmtAgo renders how long ago the last output arrived, coarse: whole
// minutes past the first, whole seconds under it.
func fmtAgo(d time.Duration) string {
	if d >= time.Minute {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

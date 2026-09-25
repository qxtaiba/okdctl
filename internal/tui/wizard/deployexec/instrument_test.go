package deployexec

import (
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// seededHistory weights the seven-step streamPlan so wait-bootstrap
// dominates the schedule the way a real install's history does.
func seededHistory() map[distribution.StepID]time.Duration {
	return map[distribution.StepID]time.Duration{
		setup.StepInstallPackages:    30 * time.Second,
		setup.StepDownloadTools:      60 * time.Second,
		setup.StepGenerateIgnition:   30 * time.Second,
		setup.StepBuildISOs:          120 * time.Second,
		install.StepDeployInfra:      240 * time.Second,
		install.StepWaitBootstrap:    1200 * time.Second,
		postinstall.StepVerifyHealth: 60 * time.Second,
	}
}

func TestWeightedPercentAdvancesByHistoryNotStepCount(t *testing.T) {
	st := streamState()
	st.History = seededHistory()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)

	if got := s.percent(); got != 0 {
		t.Fatalf("percent before any event = %d, want 0", got)
	}

	// The two prep steps (30s + 60s of a 1740s schedule) settle: ~5%,
	// nothing like the 2/7 ≈ 28% a step count would claim.
	for _, m := range st.Plan[:2] {
		s.applyEvent(&Event{StepID: m.ID})
		s.applyEvent(&Event{StepID: m.ID, Done: true, Took: 20 * time.Second})
	}
	if got := s.percent(); got != 5 {
		t.Errorf("percent after prep = %d, want the weighted 5", got)
	}
}

func TestPercentIsMonotonicAndCappedUntilFinal(t *testing.T) {
	st := streamState()
	st.History = seededHistory()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)

	for _, m := range st.Plan {
		s.applyEvent(&Event{StepID: m.ID})
		s.applyEvent(&Event{StepID: m.ID, Done: true, Took: time.Second})
	}
	if got := s.percent(); got != 96 {
		t.Errorf("percent with every step settled but no final event = %d, want the 96 cap", got)
	}

	s.finished = true
	if got := s.percent(); got != 100 {
		t.Errorf("percent after the final event = %d, want 100", got)
	}
}

func TestNoHistoryRunRendersEvenWeightsAndNoETA(t *testing.T) {
	st := streamState() // History nil: a first install
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)

	for _, m := range st.Plan[:2] {
		s.applyEvent(&Event{StepID: m.ID})
		s.applyEvent(&Event{StepID: m.ID, Done: true, Took: 20 * time.Second})
	}
	// Even weights: 2 of 7 steps ≈ 28%.
	if got := s.percent(); got != 28 {
		t.Errorf("no-history percent = %d, want the even-weight 28", got)
	}
	if eta, ok := s.etaLeft(); ok {
		t.Errorf("a first install has no honest schedule; got ETA %v", eta)
	}
	if out := tuitest.StripANSI(s.View(100, 40)); strings.Contains(out, "left") {
		t.Errorf("no-history view must not render an ETA:\n%s", out)
	}
}

func TestETASuppressedUntilFirstPhaseSettles(t *testing.T) {
	st := streamState()
	st.History = seededHistory()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)

	if _, ok := s.etaLeft(); ok {
		t.Fatal("ETA must stay suppressed until the first phase settles")
	}

	// Settle prep (both steps), on schedule.
	for _, m := range st.Plan[:2] {
		s.applyEvent(&Event{StepID: m.ID})
		s.applyEvent(&Event{StepID: m.ID, Done: true, Took: st.History[m.ID]})
	}
	eta, ok := s.etaLeft()
	if !ok {
		t.Fatal("ETA must show once the first phase settles")
	}
	// Remaining schedule: 1740s − 90s = 1650s, on-schedule correction ≈ 1.
	if eta < 1500*time.Second || eta > 1800*time.Second {
		t.Errorf("eta = %v, want about the remaining 1650s schedule", eta)
	}
}

func TestETANeverTwitchesUpward(t *testing.T) {
	st := streamState()
	st.History = seededHistory()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)

	for _, m := range st.Plan[:2] {
		s.applyEvent(&Event{StepID: m.ID})
		s.applyEvent(&Event{StepID: m.ID, Done: true, Took: st.History[m.ID]})
	}
	_ = s.View(100, 40)
	first := s.etaShown

	// A run drifting mildly behind schedule re-estimates a bit higher; the
	// shown value must hold rather than twitch upward.
	s.applyEvent(&Event{StepID: st.Plan[2].ID})
	s.applyEvent(&Event{StepID: st.Plan[2].ID, Done: true,
		Took: st.History[st.Plan[2].ID] + 10*time.Second})
	_ = s.View(100, 40)
	if s.etaShown > first {
		t.Errorf("etaShown rose %v → %v on a mild re-estimate", first, s.etaShown)
	}

	// A genuine slip (novel estimate far above the shown one) must replace it.
	s.ewmaRatio = 3
	_ = s.View(100, 40)
	if s.etaShown <= first {
		t.Errorf("a real schedule slip must move the ETA, still %v", s.etaShown)
	}
}

func TestStallDetectorFreezesTheRunningRow(t *testing.T) {
	st := streamState()
	st.History = seededHistory()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)

	// Drive the run into wait-bootstrap.
	for _, m := range st.Plan[:5] {
		s.applyEvent(&Event{StepID: m.ID})
		s.applyEvent(&Event{StepID: m.ID, Done: true, Took: time.Second})
	}
	s.applyEvent(&Event{StepID: install.StepWaitBootstrap})

	cur = cur.Add(60 * time.Second)
	if out := tuitest.StripANSI(s.View(100, 40)); strings.Contains(out, "last output") {
		t.Errorf("60s of silence is under wait-bootstrap's 90s threshold:\n%s", out)
	}

	cur = cur.Add(60 * time.Second)
	out := tuitest.StripANSI(s.View(100, 40))
	if !strings.Contains(out, "last output 2m ago") {
		t.Errorf("120s of silence during wait-bootstrap must read as a stall:\n%s", out)
	}
}

// TestActivityGlyphDecaysToASlowPulse pins the meter's contract: the glyph
// index rides the event count while output flows (motion frequency IS
// throughput) and decays to a slow frame-driven pulse when quiet.
func TestActivityGlyphDecaysToASlowPulse(t *testing.T) {
	st := streamState()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)
	s.applyEvent(&Event{StepID: st.Plan[0].ID})

	busy := s.runningGlyph()
	s.applyEvent(&Event{StepID: st.Plan[0].ID}) // one more unit of throughput
	if next := s.runningGlyph(); next == busy {
		t.Error("the glyph must advance with throughput")
	}

	cur = cur.Add(30 * time.Second) // quiet: past the decay threshold
	quiet0 := s.runningGlyph()
	s.frame = 4
	if got := s.runningGlyph(); got != quiet0 {
		t.Errorf("a quiet glyph must pulse slowly: frame 4 moved it %q → %q", quiet0, got)
	}
	s.frame = slowPulseDivisor
	if got := s.runningGlyph(); got == quiet0 {
		t.Error("the quiet pulse must still advance on the slow cadence")
	}
}

// TestSettleHoldsTheScreenThenCompletes pins the finish: under full motion
// the final event starts a frame-driven settle to 100% and the completion
// message fires on its last frame, never before.
func TestSettleHoldsTheScreenThenCompletes(t *testing.T) {
	st := streamState()
	s := NewStreamStep(st, Hooks{Execute: func(_ *State, ch chan<- Event) error {
		for _, m := range st.Plan {
			ch <- Event{StepID: m.ID}
			ch <- Event{StepID: m.ID, Done: true, Took: time.Second}
		}
		return nil
	}})

	if _, ok := pump(t, s, s.Init()).(wizard.StepCompleteMsg); !ok {
		t.Fatal("the settled run must still advance to the done screen")
	}
	if !s.finished || s.settling {
		t.Errorf("finished=%v settling=%v after completion, want finished and settled", s.finished, s.settling)
	}
	if got := s.percent(); got != 100 {
		t.Errorf("percent after settle = %d, want 100", got)
	}
}

// TestSettleSkippedUnderReducedMotion keeps the reduced and off dials
// instant: the final event completes immediately, no held frames.
func TestSettleSkippedUnderReducedMotion(t *testing.T) {
	tui.SetMotion(tui.MotionReduced)
	t.Cleanup(func() { tui.SetMotion(tui.MotionFull) })

	st := streamState()
	s := NewStreamStep(st, Hooks{})
	s.buildRows()
	_, cmd := s.Update(streamEventMsg{ev: Event{Final: true}})
	if cmd == nil {
		t.Fatal("the final event must complete immediately under reduced motion")
	}
	if _, ok := cmd().(wizard.StepCompleteMsg); !ok {
		t.Errorf("final event emitted %T, want StepCompleteMsg", cmd())
	}
}

func TestWindowTitleUsesTheWeightModel(t *testing.T) {
	st := streamState()
	st.History = seededHistory()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)

	for _, m := range st.Plan[:2] {
		s.applyEvent(&Event{StepID: m.ID})
		s.applyEvent(&Event{StepID: m.ID, Done: true, Took: time.Second})
	}
	if got := s.WindowTitle(); !strings.Contains(got, "5%") {
		t.Errorf("window title = %q, want the weighted 5%%", got)
	}
}

// TestBarRendersSegmentsAndPendingSchedule pins the instrument's static
// frame: the segmented bar under the headline, and pending phases
// annotated with their historical durations so the trail doubles as a
// schedule.
func TestBarRendersSegmentsAndPendingSchedule(t *testing.T) {
	st := streamState()
	st.History = seededHistory()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)
	for _, m := range st.Plan[:2] {
		s.applyEvent(&Event{StepID: m.ID})
		s.applyEvent(&Event{StepID: m.ID, Done: true, Took: st.History[m.ID]})
	}

	out := tuitest.StripANSI(s.View(100, 40))
	if !strings.Contains(out, tui.IconBarFill) || !strings.Contains(out, tui.IconBarTrack) {
		t.Fatalf("the bar must render filled and empty regions:\n%s", out)
	}
	if got := strings.Count(rowContaining(out, tui.IconBarTrack), tui.IconBarSegment); got != 4 {
		t.Errorf("bar carries %d segment boundaries, want 4 (five phases):\n%s", got, out)
	}
	if !strings.Contains(out, "5%") {
		t.Errorf("the bar row must carry the weighted percent:\n%s", out)
	}
	// install's pending bullet carries its ~20m schedule from history.
	if !strings.Contains(out, "~20m") {
		t.Errorf("pending phases must be annotated with their historical durations:\n%s", out)
	}
}

// rowContaining returns the first line of frame carrying sub.
func rowContaining(frame, sub string) string {
	for _, l := range strings.Split(frame, "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}

// TestBarStaysTextIdenticalWhileTheBandDrifts pins the lighten band as
// color-only motion: frames advance, the stripped frame does not change.
func TestBarStaysTextIdenticalWhileTheBandDrifts(t *testing.T) {
	st := streamState()
	st.History = seededHistory()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)
	for _, m := range st.Plan[:2] {
		s.applyEvent(&Event{StepID: m.ID})
		s.applyEvent(&Event{StepID: m.ID, Done: true, Took: time.Second})
	}

	s.frame = 0
	frame0 := tuitest.StripANSI(s.View(100, 40))
	s.frame = 3
	frame3 := tuitest.StripANSI(s.View(100, 40))
	if bar0, bar3 := rowContaining(frame0, tui.IconBarTrack), rowContaining(frame3, tui.IconBarTrack); bar0 != bar3 {
		t.Errorf("the drifting band must be color-only:\n%q\n%q", bar0, bar3)
	}
}

// TestPhaseDurationBarsFillTheDeadRows pins the checklist's lower rows on
// the split tier: per-phase duration bars with the last-run tick.
func TestPhaseDurationBarsFillTheDeadRows(t *testing.T) {
	st := streamState()
	st.History = seededHistory()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)
	s.hooks.Logs = seededRing(4)
	s.log.Src = s.hooks.Logs
	s.SetTerminalSize(180, 48)
	s.SetSize(100, 34)
	for _, m := range st.Plan[:2] {
		s.applyEvent(&Event{StepID: m.ID})
		cur = cur.Add(30 * time.Second)
		s.applyEvent(&Event{StepID: m.ID, Done: true, Took: 30 * time.Second})
	}

	out := tuitest.StripANSI(s.View(100, 1000))
	if !strings.Contains(out, "PHASES") {
		t.Fatalf("the split tier's dead rows must carry the phase duration bars:\n%s", out)
	}
	if !strings.Contains(out, tui.IconBarTick) {
		t.Errorf("phase bars must carry the last-run tick:\n%s", out)
	}

	// A narrow frame spends its slack on the log tail instead.
	s.SetTerminalSize(120, 40)
	if out := tuitest.StripANSI(s.View(104, 1000)); strings.Contains(out, "PHASES") {
		t.Errorf("the narrow tier's slack belongs to the log tail:\n%s", out)
	}
}

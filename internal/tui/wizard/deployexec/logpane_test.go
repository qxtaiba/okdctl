package deployexec

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/logview"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// logBase is the fixed clock every seeded log fixture stamps from.
var logBase = time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)

// seededRing fills a real ring with n deterministic lines, so step tests and
// goldens read the same fixture the production tee would hand them.
func seededRing(n int) *logview.Ring {
	r := logview.NewRing(logview.DefaultCap)
	for i := range n {
		r.Append(logview.Line{
			At:    logBase.Add(time.Duration(i*7) * time.Second),
			Level: "INFO",
			Text:  fmt.Sprintf("deploy step started step=step-%02d phase=setup", i),
		})
	}
	return r
}

func TestFlowStepCountMatchesNewSteps(t *testing.T) {
	if got := len(NewSteps(streamState(), Hooks{})); got != flowStepCount {
		t.Errorf("NewSteps builds %d screens, flowStepCount says %d", got, flowStepCount)
	}
}

func TestStreamLockKeyTogglesFollow(t *testing.T) {
	s := NewStreamStep(streamState(), Hooks{Logs: seededRing(10)})

	_, cmd := s.Update(tea.KeyPressMsg{Code: logview.KeyLock, Text: "l"})
	if cmd == nil {
		t.Fatal("locking the log must nudge the outer viewport toward the tail")
	}
	if _, ok := cmd().(wizard.FocusChangedMsg); !ok {
		t.Errorf("l emitted %T, want FocusChangedMsg", cmd())
	}
	if !s.log.Locked() {
		t.Fatal("l must lock the log")
	}
	if s.log.LockPoint() != 10 {
		t.Errorf("lock point = %d, want the stream index at lock time (10)", s.log.LockPoint())
	}
	if got := tuitest.StripANSI(s.PaneContent(44, 6)); !strings.Contains(got, "LOG · 6–10 of 10") {
		t.Errorf("a locked pane must name its window position:\n%s", got)
	}

	s.Update(tea.KeyPressMsg{Code: logview.KeyLock, Text: "l"})
	if s.log.Locked() {
		t.Error("a second l must release the lock")
	}
}

func TestStreamFullKeySwapsTheLogFullScreenAndBack(t *testing.T) {
	s := NewStreamStep(streamState(), Hooks{Logs: seededRing(30)})
	s.buildRows()
	s.SetSize(100, 20)
	s.SetTerminalSize(180, 48)

	_, cmd := s.Update(tea.KeyPressMsg{Code: logview.KeyFull, Text: "f"})
	if cmd == nil {
		t.Fatal("f must ask the frame to re-measure")
	}
	if _, ok := cmd().(wizard.LayoutChangedMsg); !ok {
		t.Errorf("f emitted %T, want LayoutChangedMsg", cmd())
	}
	if !s.SuppressesSplit() {
		t.Error("a full-screen log must claim the whole frame")
	}

	body := tuitest.StripANSI(s.View(104, 1000))
	if strings.Contains(body, string(PhasePrep)) {
		t.Errorf("a full-screen log must replace the checklist:\n%s", body)
	}
	if !strings.Contains(body, "deploying homelab") {
		t.Errorf("a full-screen log must keep the run's headline:\n%s", body)
	}
	if rows := strings.Count(body, "\n") + 1; rows > 20 {
		t.Errorf("full-screen log rendered %d rows, want <= the body's 20", rows)
	}

	s.Update(tea.KeyPressMsg{Code: logview.KeyFull, Text: "f"})
	if s.SuppressesSplit() {
		t.Error("a second f must give the checklist back")
	}
	if !strings.Contains(tuitest.StripANSI(s.View(104, 1000)), string(PhasePrep)) {
		t.Error("the checklist must come back with it")
	}
}

func TestStreamNarrowFrameCarriesTheTailUnderTheChecklist(t *testing.T) {
	s := NewStreamStep(streamState(), Hooks{Logs: seededRing(20)})
	s.buildRows()
	s.SetTerminalSize(120, 40)

	body := tuitest.StripANSI(s.View(104, 1000))
	if !strings.Contains(body, "step-19") {
		t.Errorf("a narrow frame must carry the log tail:\n%s", body)
	}
	if strings.Contains(body, "step-10") {
		t.Errorf("the tail must be capped at %d rows:\n%s", logview.NarrowTailRows, body)
	}

	s.SetTerminalSize(180, 48)
	if strings.Contains(tuitest.StripANSI(s.View(100, 1000)), "step-19") {
		t.Error("a frame with a log pane must not repeat the tail in the body")
	}
}

// TestStreamTailSpanFollowsTheNewestLine pins the viewport contract per tier:
// a narrow frame follows the tail so the newest line is always on screen, while
// a frame whose pane carries the log follows the running row instead.
func TestStreamTailSpanFollowsTheNewestLine(t *testing.T) {
	base := logBase
	cur := base
	st := streamState()
	s := NewStreamStep(st, Hooks{Logs: seededRing(20)})
	s.now = func() time.Time { return cur }
	s.started = base
	s.buildRows()
	s.applyEvent(&Event{StepID: st.Plan[0].ID})

	s.SetTerminalSize(120, 40)
	_ = s.View(104, 1000)
	span, ok := s.FocusedSpan()
	if !ok {
		t.Fatal("FocusedSpan must report ok once rows exist")
	}
	if span.Start != s.lastLine || span.End != s.lastLine {
		t.Errorf("narrow span = %+v, want the tail's end line %d", span, s.lastLine)
	}

	s.SetTerminalSize(180, 48)
	_ = s.View(100, 1000)
	span, _ = s.FocusedSpan()
	if span.Start != s.focusLine {
		t.Errorf("wide span = %+v, want the running row %d — the pane carries the log there", span, s.focusLine)
	}
}

func TestDoneFailureKeepsTheLastLogLinesOnScreen(t *testing.T) {
	st := doneState()
	st.Result = errStreamFailed
	s := NewDoneStep(st, Hooks{Logs: seededRing(20)})
	s.SetTerminalSize(100, 30)

	out := tuitest.StripANSI(s.View(96, 1000))
	if !strings.Contains(out, "deploy failed") {
		t.Fatalf("failure view must render the error card:\n%s", out)
	}
	if !strings.Contains(out, "step-19") {
		t.Errorf("failure view must keep the last log lines on screen:\n%s", out)
	}

	// A wide frame carries them in the pane instead, so the body must not repeat them.
	s.SetTerminalSize(180, 48)
	if strings.Contains(tuitest.StripANSI(s.View(96, 1000)), "step-19") {
		t.Error("a frame with a log pane must not repeat the tail under the card")
	}
	if !strings.Contains(tuitest.StripANSI(s.PaneContent(47, 8)), "step-19") {
		t.Error("the pane must carry the tail on the completion screen too")
	}
}

// TestStreamShortHelpAdvertisesTheLogKeys is what puts them in the help overlay:
// it renders exactly the step's own bindings.
func TestStreamShortHelpAdvertisesTheLogKeys(t *testing.T) {
	s := NewStreamStep(streamState(), Hooks{Logs: seededRing(2)})

	keys := map[string]string{}
	for _, b := range s.ShortHelp() {
		keys[b.Key] = b.Help
	}
	if keys["l"] != "lock the log here" {
		t.Errorf("l help = %q, want the lock instruction", keys["l"])
	}
	if keys["f"] != "full-screen log" {
		t.Errorf("f help = %q, want the full-screen instruction", keys["f"])
	}
	if _, ok := keys[wizard.HelpCtrlC]; !ok {
		t.Error("the cancel binding must survive alongside the log keys")
	}

	s.Update(tea.KeyPressMsg{Code: logview.KeyLock, Text: "l"})
	s.Update(tea.KeyPressMsg{Code: logview.KeyFull, Text: "f"})
	after := map[string]string{}
	for _, b := range s.ShortHelp() {
		after[b.Key] = b.Help
	}
	if after["l"] != "follow the log tail" || after["f"] != "back to checklist" {
		t.Errorf("help must name what the key does next, got %v", after)
	}
}

// TestStreamWithoutALogSourceAdvertisesNoLogKeys keeps the ribbon honest on a
// run with no log stream wired (a test or a demo without one).
func TestStreamWithoutALogSourceAdvertisesNoLogKeys(t *testing.T) {
	s := NewStreamStep(streamState(), Hooks{})
	for _, b := range s.ShortHelp() {
		if b.Key == "l" || b.Key == "f" {
			t.Errorf("advertised %q with no log source", b.Key)
		}
	}
	if s.PaneContent(44, 8) != "" {
		t.Error("with no log source the pane must fall back to the context pane")
	}
	if s.SuppressesSplit() {
		t.Error("with no log source there is nothing to go full-screen")
	}
}

// streamModelAt builds the deploy flow around a seeded ring and renders it once
// at w×h, so pane tests exercise the real frame rather than the step alone.
func streamModelAt(t *testing.T, st *State, hooks Hooks, w, h int) *wizard.Model {
	t.Helper()
	tui.SetTerminalWidth(w)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })
	m := wizard.NewFlowModel(NewSteps(st, hooks), st.Cfg, Chrome())
	tuitest.RenderAt(t, m, w, h)
	return m
}

// longPlan mirrors the real 34-step registry's shape, so the narrow tier's
// checklist genuinely overflows an 80x24 viewport the way a live deploy does.
func longPlan() []tui.StepMeta {
	plan := make([]tui.StepMeta, 0, 34)
	plan = append(plan, streamPlan()...)
	for i := len(plan); i < 34; i++ {
		plan = append(plan, tui.StepMeta{
			ID:    distribution.StepID(fmt.Sprintf("filler-%02d", i)),
			Name:  fmt.Sprintf("filler step %02d", i),
			Phase: "setup",
		})
	}
	return plan
}

// TestStreamFullScreenRibbonKeepsTheExitKeyAt80Cols pins that `f` — the only way
// out of full-screen mode — survives the help ribbon's squeeze at the narrowest
// supported terminal, where every non-essential binding is dropped first.
func TestStreamFullScreenRibbonKeepsTheExitKeyAt80Cols(t *testing.T) {
	st := streamState()
	m := streamModelAt(t, st, Hooks{Logs: seededRing(24)}, 80, 24)

	m.Update(tea.KeyPressMsg{Code: logview.KeyFull, Text: "f"})
	m.Update(wizard.LayoutChangedMsg{})

	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 80, 24))
	ribbon := lastNonEmptyRow(frame)
	if !strings.Contains(ribbon, "f back to checklist") {
		t.Errorf("the only exit from full-screen mode was squeezed out of the ribbon:\n%s", ribbon)
	}
	if !strings.Contains(ribbon, wizard.HelpCtrlC) {
		t.Errorf("the cancel binding must survive too:\n%s", ribbon)
	}
	tuitest.AssertFits(t, tuitest.RenderAt(t, m, 80, 24), 80, 24)
}

// lastNonEmptyRow returns the frame's help ribbon: the last row carrying text
// inside the wizard's bottom border.
func lastNonEmptyRow(frame string) string {
	rows := strings.Split(frame, "\n")
	for i := len(rows) - 1; i >= 0; i-- {
		row := strings.Trim(rows[i], " │╰╯─")
		if strings.TrimSpace(row) != "" {
			return row
		}
	}
	return ""
}

// TestStreamNarrowTailStaysVisibleAt80Cols pins the narrow tier's contract: with
// a checklist far longer than the viewport, the newest log line must still be on
// screen without the operator scrolling for it.
func TestStreamNarrowTailStaysVisibleAt80Cols(t *testing.T) {
	st := streamState()
	st.Plan = longPlan()
	m := streamModelAt(t, st, Hooks{Logs: seededRing(30)}, 80, 24)

	s, ok := m.CurrentStep().(*StreamStep)
	if !ok {
		t.Fatal("the stream step must be current")
	}
	base := logBase
	cur := base
	s.now = func() time.Time { return cur }
	s.started = base
	s.buildRows()
	s.applyEvent(&Event{StepID: st.Plan[0].ID})
	m.Update(wizard.FocusChangedMsg{})

	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 80, 24))
	if !strings.Contains(frame, "step-29") {
		t.Errorf("the newest log line must be on screen without scrolling:\n%s", frame)
	}
}

// TestStreamFinalSendAbortsOnACancelledRun pins the last unguarded send on the
// feed: a force-quit must never strand the engine goroutine on a channel nobody
// drains.
func TestStreamFinalSendAbortsOnACancelledRun(t *testing.T) {
	gone := make(chan struct{})
	close(gone)

	s := NewStreamStep(streamState(), Hooks{Done: gone})
	for len(s.events) < cap(s.events) {
		s.events <- Event{}
	}

	returned := make(chan struct{})
	go func() {
		s.sendFinal(nil)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("the final send blocked on a feed nobody drains")
	}
}

// TestStreamFullScreenPagingWalksTheRing pins the full-screen pager: pgup
// engages the lock and pages back through lines the tail had already
// scrolled past, the header names honest coordinates, and paging back down
// releases the lock and resumes following.
func TestStreamFullScreenPagingWalksTheRing(t *testing.T) {
	s := NewStreamStep(streamState(), Hooks{Logs: seededRing(40)})
	s.buildRows()
	s.SetSize(100, 20)
	s.SetTerminalSize(120, 40)

	s.Update(tea.KeyPressMsg{Code: logview.KeyFull, Text: "f"})
	_ = s.View(104, 1000)
	if !s.ConsumesPaging() {
		t.Fatal("the full-screen log must claim the paging keys")
	}

	s.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if !s.log.Locked() {
		t.Fatal("pgup from follow must engage the lock")
	}
	body := tuitest.StripANSI(s.View(104, 1000))
	if !strings.Contains(body, "of 40") {
		t.Errorf("a paged window must name its position:\n%s", body)
	}
	if strings.Contains(body, "step-39") {
		t.Errorf("one page up must have scrolled past the newest line:\n%s", body)
	}

	for range 10 {
		s.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	if !strings.Contains(tuitest.StripANSI(s.View(104, 1000)), "step-00") {
		t.Error("paging to the top must reach the ring's first line")
	}

	for range 20 {
		s.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	if s.log.Locked() {
		t.Error("paging back past the tail must release the lock")
	}
	if !strings.Contains(tuitest.StripANSI(s.View(104, 1000)), "step-39") {
		t.Error("a released window must follow the tail again")
	}
}

// TestStreamFullScreenArrowsScrollByLine pins the arrows' one-line walk in
// full-screen mode; outside it the step leaves them to the frame's viewport
// (ScrollsWithArrows).
func TestStreamFullScreenArrowsScrollByLine(t *testing.T) {
	s := NewStreamStep(streamState(), Hooks{Logs: seededRing(40)})
	s.buildRows()
	s.SetSize(100, 20)
	s.SetTerminalSize(120, 40)

	if !s.ScrollsWithArrows() {
		t.Fatal("the checklist must leave the arrows to the frame's viewport")
	}

	s.Update(tea.KeyPressMsg{Code: logview.KeyFull, Text: "f"})
	_ = s.View(104, 1000)
	if s.ScrollsWithArrows() {
		t.Fatal("the full-screen log must keep the arrows for itself")
	}

	s.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if !s.log.Locked() || s.log.LockPoint() != 39 {
		t.Fatalf("one arrow up must lock one line back, got locked=%v lockAt=%d", s.log.Locked(), s.log.LockPoint())
	}
	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.log.Locked() {
		t.Error("one arrow back down must release the lock at the tail")
	}
}

// TestStreamLockedPanePagesThroughTheRing pins item: the locked side pane
// pages with pgup/pgdn; while following, the checklist keeps those keys.
func TestStreamLockedPanePagesThroughTheRing(t *testing.T) {
	s := NewStreamStep(streamState(), Hooks{Logs: seededRing(40)})
	s.buildRows()
	s.SetTerminalSize(180, 48)

	if s.ConsumesPaging() {
		t.Fatal("a following pane must leave paging to the checklist viewport")
	}
	s.Update(tea.KeyPressMsg{Code: logview.KeyLock, Text: "l"})
	if !s.ConsumesPaging() {
		t.Fatal("a locked pane must claim the paging keys")
	}

	_ = s.PaneContent(44, 10)
	s.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	pane := tuitest.StripANSI(s.PaneContent(44, 10))
	if strings.Contains(pane, "step-39") {
		t.Errorf("a paged pane must have scrolled past the tail:\n%s", pane)
	}
	if !strings.Contains(pane, "of 40") {
		t.Errorf("a paged pane must name its position:\n%s", pane)
	}
}

// TestStreamFullScreenPgUpPagesInsideTheFrame drives the paging through the
// whole wizard frame, proving the frame's viewport steps aside for a step
// that pages its own log region.
func TestStreamFullScreenPgUpPagesInsideTheFrame(t *testing.T) {
	st := streamState()
	m := streamModelAt(t, st, Hooks{Logs: seededRing(40)}, 80, 24)

	m.Update(tea.KeyPressMsg{Code: logview.KeyFull, Text: "f"})
	m.Update(wizard.LayoutChangedMsg{})
	_ = tuitest.RenderAt(t, m, 80, 24)

	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 80, 24))
	if !strings.Contains(frame, "of 40") {
		t.Errorf("pgup must reach the full-screen log and page it:\n%s", frame)
	}
	if strings.Contains(frame, "step-39") {
		t.Errorf("the paged frame must show the window, not the tail:\n%s", frame)
	}
}

// TestStreamFullScreenHeaderCarriesTheSinkPath pins the escape hatch: the
// ring holds a window, so the full-screen header names the file that keeps
// every byte — the real resolved path, or nothing when no sink is open.
func TestStreamFullScreenHeaderCarriesTheSinkPath(t *testing.T) {
	s := NewStreamStep(streamState(), Hooks{Logs: seededRing(8), LogPath: "okd-install/okdctl.log"})
	s.buildRows()
	s.SetSize(100, 20)
	s.Update(tea.KeyPressMsg{Code: logview.KeyFull, Text: "f"})

	body := tuitest.StripANSI(s.View(104, 1000))
	if !strings.Contains(body, "full log: okd-install/okdctl.log") {
		t.Errorf("full-screen header must carry the sink path:\n%s", body)
	}

	bare := NewStreamStep(streamState(), Hooks{Logs: seededRing(8)})
	bare.buildRows()
	bare.SetSize(100, 20)
	bare.Update(tea.KeyPressMsg{Code: logview.KeyFull, Text: "f"})
	if body := tuitest.StripANSI(bare.View(104, 1000)); strings.Contains(body, "full log") {
		t.Errorf("with no sink open there is no path to point at:\n%s", body)
	}
}

// TestStreamFootnoteNamesTheResolvedSinkPath pins the checklist footnote's
// pointer at the path the run actually writes, never a guessed filename.
func TestStreamFootnoteNamesTheResolvedSinkPath(t *testing.T) {
	s := NewStreamStep(streamState(), Hooks{Logs: seededRing(4), LogPath: "/srv/lab/okdctl.log"})
	s.buildRows()
	s.SetTerminalSize(120, 40)

	body := tuitest.StripANSI(s.View(104, 1000))
	if !strings.Contains(body, "full log /srv/lab/okdctl.log") {
		t.Errorf("footnote must name the resolved sink path:\n%s", body)
	}

	bare := NewStreamStep(streamState(), Hooks{Logs: seededRing(4)})
	bare.buildRows()
	bare.SetTerminalSize(120, 40)
	if body := tuitest.StripANSI(bare.View(104, 1000)); strings.Contains(body, "full log") {
		t.Errorf("with no sink open the footnote must not invent one:\n%s", body)
	}
}

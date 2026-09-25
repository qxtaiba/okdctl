package deployexec

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// logBase is the fixed clock every seeded log fixture stamps from.
var logBase = time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)

// seededRing fills a real ring with n deterministic lines, so pane tests and
// goldens read the same fixture the production type would hand them.
func seededRing(n int) *LogRing {
	r := NewLogRing(LogRingCap)
	for i := range n {
		r.append(LogLine{
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

func TestLogPaneFollowsTheTailWithinItsHeight(t *testing.T) {
	const width, height = 44, 8
	out := renderLogPane(seededRing(40), logView{}, width, height, false)
	lines := strings.Split(tuitest.StripANSI(out), "\n")

	if len(lines) > height {
		t.Fatalf("pane rendered %d rows, want <= %d", len(lines), height)
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("row %d is %d cols, want <= %d: %q", i, w, width, l)
		}
	}
	if !strings.HasPrefix(lines[0], "LOG") {
		t.Errorf("pane must lead with its section header, got %q", lines[0])
	}
	if !strings.Contains(out, "step-39") {
		t.Errorf("a following pane must show the newest line:\n%s", tuitest.StripANSI(out))
	}
	if strings.Contains(out, "step-00") {
		t.Errorf("a following pane must have scrolled past the oldest line:\n%s", tuitest.StripANSI(out))
	}
	newest := logBase.Add(39 * 7 * time.Second).Format(logStampFormat)
	if !strings.Contains(tuitest.StripANSI(out), newest) {
		t.Errorf("rows must carry their own timestamp; %q missing from:\n%s", newest, tuitest.StripANSI(out))
	}
}

func TestLogPaneEmptyRingSaysSoInsteadOfRenderingBlank(t *testing.T) {
	out := tuitest.StripANSI(renderLogPane(NewLogRing(8), logView{}, 40, 6, false))
	if !strings.Contains(out, "waiting for the first log line") {
		t.Errorf("an empty ring must say so:\n%s", out)
	}
}

func TestLogWindowLockHoldsItsPointWhileTheTailMovesOn(t *testing.T) {
	r := seededRing(20)
	view := logView{locked: true, lockAt: lockedAt(r)}

	for i := 20; i < 40; i++ {
		r.append(LogLine{At: logBase, Text: fmt.Sprintf("later step-%02d", i)})
	}

	lines, first := r.Snapshot()
	window := logWindow(lines, first, view, 4)
	if len(window) != 4 {
		t.Fatalf("locked window holds %d rows, want 4", len(window))
	}
	if !strings.Contains(window[3].Text, "step-19") {
		t.Errorf("locked window ends at %q, want the line the lock pinned", window[3].Text)
	}

	following := logWindow(lines, first, logView{}, 4)
	if !strings.Contains(following[3].Text, "step-39") {
		t.Errorf("a released window ends at %q, want the newest line", following[3].Text)
	}
}

// TestLogWindowLockOlderThanTheRingFallsBackToWhatIsLeft keeps a long-held lock
// from blanking the pane once the ring has evicted its point.
func TestLogWindowLockOlderThanTheRingFallsBackToWhatIsLeft(t *testing.T) {
	r := NewLogRing(4)
	for i := range 20 {
		r.append(LogLine{At: logBase, Text: fmt.Sprintf("step-%02d", i)})
	}
	lines, first := r.Snapshot()

	window := logWindow(lines, first, logView{locked: true, lockAt: 1}, 3)
	if len(window) == 0 {
		t.Fatal("a lock the ring has outrun must still show what it holds")
	}
}

func TestLogPaneHeaderMarksALockedWindow(t *testing.T) {
	if got := tuitest.StripANSI(logPaneHeader(logView{locked: true}, 40)); !strings.Contains(got, "LOCKED") {
		t.Errorf("header = %q, want it to mark the lock", got)
	}
	if got := tuitest.StripANSI(logPaneHeader(logView{}, 40)); strings.Contains(got, "LOCKED") {
		t.Errorf("header = %q, want no lock marker while following", got)
	}
}

func TestStreamLockKeyTogglesFollow(t *testing.T) {
	s := NewStreamStep(streamState(), Hooks{Logs: seededRing(10)})

	if _, cmd := s.Update(tea.KeyPressMsg{Code: keyLogLock, Text: "l"}); cmd != nil {
		t.Error("locking the log must not emit a command")
	}
	if !s.log.locked {
		t.Fatal("l must lock the log")
	}
	if s.log.lockAt != 10 {
		t.Errorf("lockAt = %d, want the stream index at lock time (10)", s.log.lockAt)
	}
	if got := tuitest.StripANSI(s.PaneContent(44, 6)); !strings.Contains(got, "LOCKED") {
		t.Errorf("a locked pane must say so:\n%s", got)
	}

	s.Update(tea.KeyPressMsg{Code: keyLogLock, Text: "l"})
	if s.log.locked {
		t.Error("a second l must release the lock")
	}
}

func TestStreamFullKeySwapsTheLogFullScreenAndBack(t *testing.T) {
	s := NewStreamStep(streamState(), Hooks{Logs: seededRing(30)})
	s.buildRows()
	s.SetSize(100, 20)
	s.SetTerminalSize(180, 48)

	_, cmd := s.Update(tea.KeyPressMsg{Code: keyLogFull, Text: "f"})
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

	s.Update(tea.KeyPressMsg{Code: keyLogFull, Text: "f"})
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
		t.Errorf("the tail must be capped at %d rows:\n%s", narrowTailRows, body)
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
	if !strings.Contains(tuitest.StripANSI(s.PaneContent(44, 8)), "step-19") {
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

	s.Update(tea.KeyPressMsg{Code: keyLogLock, Text: "l"})
	s.Update(tea.KeyPressMsg{Code: keyLogFull, Text: "f"})
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

	m.Update(tea.KeyPressMsg{Code: keyLogFull, Text: "f"})
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

// TestLogRowsCarryTextualLevelTags pins bug 24's minimal fix: warn and
// error lines carry their level as text, so the failure screen's evidence
// tail reads under NO_COLOR instead of riding on tint alone.
func TestLogRowsCarryTextualLevelTags(t *testing.T) {
	at := logBase
	lines := []LogLine{
		{At: at, Level: "INFO", Text: "deploy step started"},
		{At: at, Level: "warn", Text: "etcd member slow"},
		{At: at, Level: "ERROR", Text: "bootstrap wait failed"},
	}

	rows := logRows(lines, 70, 3, false)

	if plain := tuitest.StripANSI(rows[1]); !strings.Contains(plain, "WARN") {
		t.Errorf("warn row carries no textual tag: %q", plain)
	}
	if plain := tuitest.StripANSI(rows[2]); !strings.Contains(plain, "ERROR") {
		t.Errorf("error row carries no textual tag: %q", plain)
	}
	if plain := tuitest.StripANSI(rows[0]); strings.Contains(plain, "INFO") {
		t.Errorf("info row must stay untagged to keep the stream quiet: %q", plain)
	}
}

func TestLogRowsStyleWarnAndErrorApart(t *testing.T) {
	at := logBase
	lines := []LogLine{
		{At: at, Level: "INFO", Text: "deploy step started"},
		{At: at, Level: "WARN", Text: "deploy step started"},
		{At: at, Level: "ERROR", Text: "deploy step started"},
	}

	rows := logRows(lines, 60, 3, false)
	if len(rows) != 3 {
		t.Fatalf("logRows returned %d rows, want 3", len(rows))
	}
	if rows[0] == rows[1] || rows[1] == rows[2] || rows[0] == rows[2] {
		t.Error("warn and error lines must be styled apart from info so a failure screen reads")
	}
	for i, row := range rows {
		if !strings.Contains(tuitest.StripANSI(row), "deploy step started") {
			t.Errorf("row %d lost its text: %q", i, tuitest.StripANSI(row))
		}
	}
}

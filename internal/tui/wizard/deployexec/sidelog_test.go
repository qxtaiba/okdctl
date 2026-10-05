package deployexec

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func frameRow(t *testing.T, frame, needle string) (row int, text string) {
	t.Helper()
	plain := tuitest.StripANSI(frame)
	for i, line := range strings.Split(plain, "\n") {
		if strings.Contains(line, needle) {
			return i, line
		}
	}
	t.Fatalf("frame has no row containing %q:\n%s", needle, plain)
	return 0, ""
}

func streamFrameAt(t *testing.T, w int) string {
	t.Helper()
	const h = 40
	st := streamState()
	m := streamModelAt(t, st, goldenHooks(), w, h)
	m.Update(wizard.JumpToStepMsg{StepID: StepIDStream})
	seedMidRun(m, st)
	frame := tuitest.RenderAt(t, m, w, h)
	tuitest.AssertFits(t, frame, w, h)
	return frame
}

func failureFrameAt(t *testing.T, w int) string {
	t.Helper()
	const h = 40
	st := doneState()
	st.Result = errGoldenFailure
	m := streamModelAt(t, st, goldenHooks(), w, h)
	m.Update(wizard.JumpToStepMsg{StepID: StepIDDone})
	frame := tuitest.RenderAt(t, m, w, h)
	tuitest.AssertFits(t, frame, w, h)
	return frame
}

func TestStreamLogSitsBesideTheChecklistFrom150Columns(t *testing.T) {
	const headline, logHeader, newest, lastPhase = "2 / 7", "LOG", "09:02:41", "○ verify"

	t.Run("150 columns", func(t *testing.T) {
		frame := streamFrameAt(t, 150)
		headRow, head := frameRow(t, frame, headline)
		if !strings.Contains(head, logHeader) {
			t.Errorf("the log header must share the checklist's first row at 150 columns, got %q", head)
		}
		logRow, _ := frameRow(t, frame, logHeader)
		if logRow != headRow {
			t.Errorf("log header on row %d, checklist headline on row %d; want the same row", logRow, headRow)
		}
		if _, phase := frameRow(t, frame, lastPhase); !strings.Contains(phase, "· 09:0") {
			t.Errorf("the log column must run the height of the checklist, got %q beside the last phase", phase)
		}
		frameRow(t, frame, newest)
		if strings.Contains(tuitest.StripANSI(frame), "PHASES") {
			t.Errorf("the phase duration bars must stay cut:\n%s", tuitest.StripANSI(frame))
		}
	})

	t.Run("149 columns", func(t *testing.T) {
		frame := streamFrameAt(t, 149)
		headRow, head := frameRow(t, frame, headline)
		if strings.Contains(head, logHeader) {
			t.Errorf("below 150 columns the log must not share the checklist's row, got %q", head)
		}
		phaseRow, phase := frameRow(t, frame, lastPhase)
		if strings.Contains(phase, "· 09:0") {
			t.Errorf("below 150 columns no log line may sit beside the checklist, got %q", phase)
		}
		logRow, _ := frameRow(t, frame, logHeader)
		if logRow <= phaseRow || logRow <= headRow {
			t.Errorf("log header on row %d; want it below the checklist, which ends on row %d", logRow, phaseRow)
		}
	})
}

func TestStreamViewportFollowsTheRunningRowWhileTheLogSitsBeside(t *testing.T) {
	st := streamState()
	m := streamModelAt(t, st, goldenHooks(), 150, 40)
	seedMidRun(m, st)
	_ = tuitest.RenderAt(t, m, 150, 40)

	s := m.CurrentStep().(*StreamStep)
	span, ok := s.FocusedSpan()
	if !ok || s.focusLine < 0 || span.Start != s.focusLine {
		t.Errorf("FocusedSpan() = %+v (ok=%v), want the running row %d: the side column carries the log", span, ok, s.focusLine)
	}

	_ = tuitest.RenderAt(t, m, 149, 40)
	if span, _ := s.FocusedSpan(); span.Start != s.lastLine {
		t.Errorf("FocusedSpan() = %+v below 150 columns, want the tail's end line %d", span, s.lastLine)
	}
}

func TestDoneFailureLogSitsBesideTheReportFrom150Columns(t *testing.T) {
	const logHeader, nextMoves = "LOG", "NEXT MOVES"

	t.Run("150 columns", func(t *testing.T) {
		frame := failureFrameAt(t, 150)
		logRow, _ := frameRow(t, frame, logHeader)
		movesRow, _ := frameRow(t, frame, nextMoves)
		if logRow >= movesRow {
			t.Errorf("log header on row %d, next moves on row %d; want the log beside the top of the report", logRow, movesRow)
		}
		if n := strings.Count(tuitest.StripANSI(frame), "09:02:41"); n != 1 {
			t.Errorf("the newest log line appears %d times; want it once, in the side column only", n)
		}
	})

	t.Run("149 columns", func(t *testing.T) {
		frame := failureFrameAt(t, 149)
		logRow, _ := frameRow(t, frame, logHeader)
		movesRow, _ := frameRow(t, frame, nextMoves)
		if logRow <= movesRow {
			t.Errorf("log header on row %d, next moves on row %d; want the evidence under the report", logRow, movesRow)
		}
	})
}

func TestDoneSuccessKeepsTheWholeFrameAt150Columns(t *testing.T) {
	st := doneState()
	m := streamModelAt(t, st, goldenHooks(), 150, 40)
	m.Update(wizard.JumpToStepMsg{StepID: StepIDDone})
	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 150, 40))
	if strings.Contains(frame, "LOG") {
		t.Errorf("the payoff screen owns the whole frame and shows no log column:\n%s", frame)
	}
}

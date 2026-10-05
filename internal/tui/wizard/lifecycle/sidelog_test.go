package lifecycle

import (
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/tui"
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

func lifecycleFrameAt(t *testing.T, st *State, id wizard.StepID, seed func(*wizard.Model, *State), w int) string {
	t.Helper()
	const h = 40
	tui.SetTerminalWidth(w)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })
	m := wizard.NewFlowModel(NewSteps(st, goldenHooks()), st.Cfg, Chrome())
	_ = tuitest.RenderAt(t, m, w, h)
	m.Update(wizard.JumpToStepMsg{StepID: id})
	if seed != nil {
		seed(m, st)
	}
	frame := tuitest.RenderAt(t, m, w, h)
	tuitest.AssertFits(t, frame, w, h)
	return frame
}

func TestExecLogSitsBesideTheChecklistFrom150Columns(t *testing.T) {
	const headline, logHeader, newest, lastNode = "2 / 3", "LOG", "09:02:41", "○ homelab-master2"

	t.Run("150 columns", func(t *testing.T) {
		frame := lifecycleFrameAt(t, threeMasterState(), StepIDExec, seedExecMidRun, 150)
		headRow, head := frameRow(t, frame, headline)
		if !strings.Contains(head, logHeader) {
			t.Errorf("the log header must share the checklist's first row at 150 columns, got %q", head)
		}
		logRow, _ := frameRow(t, frame, logHeader)
		if logRow != headRow {
			t.Errorf("log header on row %d, checklist headline on row %d; want the same row", logRow, headRow)
		}
		if _, last := frameRow(t, frame, lastNode); !strings.Contains(last, "· 09:0") {
			t.Errorf("the log column must run the height of the checklist, got %q beside the last node", last)
		}
		frameRow(t, frame, newest)
	})

	t.Run("149 columns", func(t *testing.T) {
		frame := lifecycleFrameAt(t, threeMasterState(), StepIDExec, seedExecMidRun, 149)
		headRow, head := frameRow(t, frame, headline)
		if strings.Contains(head, logHeader) {
			t.Errorf("below 150 columns the log must not share the checklist's row, got %q", head)
		}
		nodeRow, last := frameRow(t, frame, lastNode)
		if strings.Contains(last, "· 09:0") {
			t.Errorf("below 150 columns no log line may sit beside the checklist, got %q", last)
		}
		logRow, _ := frameRow(t, frame, logHeader)
		if logRow <= nodeRow || logRow <= headRow {
			t.Errorf("log header on row %d; want it below the checklist, which ends on row %d", logRow, nodeRow)
		}
	})
}

func TestDoneFailureLogSitsBesideTheReportFrom150Columns(t *testing.T) {
	const logHeader, nextMoves = "LOG", "NEXT MOVES"
	failed := func() *State {
		st := doneState()
		st.Elapsed = 90 * time.Second
		st.Result = errGoldenFailure
		return st
	}

	t.Run("150 columns", func(t *testing.T) {
		frame := lifecycleFrameAt(t, failed(), StepIDDone, nil, 150)
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
		frame := lifecycleFrameAt(t, failed(), StepIDDone, nil, 149)
		logRow, _ := frameRow(t, frame, logHeader)
		movesRow, _ := frameRow(t, frame, nextMoves)
		if logRow <= movesRow {
			t.Errorf("log header on row %d, next moves on row %d; want the evidence under the report", logRow, movesRow)
		}
	})
}

func TestDoneSuccessLogSitsBesideTheSummaryFrom150Columns(t *testing.T) {
	const logHeader, summary = "LOG", "NODE RESIZE"
	succeeded := func() *State {
		st := doneState()
		st.Elapsed = 90 * time.Second
		return st
	}

	t.Run("150 columns", func(t *testing.T) {
		frame := lifecycleFrameAt(t, succeeded(), StepIDDone, nil, 150)
		if _, top := frameRow(t, frame, summary); !strings.Contains(top, logHeader) {
			t.Errorf("the log header must share the summary box's first row at 150 columns, got %q", top)
		}
	})

	t.Run("149 columns", func(t *testing.T) {
		frame := tuitest.StripANSI(lifecycleFrameAt(t, succeeded(), StepIDDone, nil, 149))
		if strings.Contains(frame, logHeader) {
			t.Errorf("below 150 columns a successful run shows no log:\n%s", frame)
		}
	})
}

package deployexec

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// Handoff lines the failure report prints for the operator to run themselves.
// A destructive move is never an in-TUI key: the screen that just failed is
// the wrong place to arm a teardown, and a command the operator types is one
// they have read.
const (
	handoffResume  = "okdctl deploy"
	handoffFresh   = "okdctl deploy --fresh"
	handoffDestroy = "okdctl destroy"
)

// incidentKeyCol is the key column the next-moves rows align their help in,
// and factKeyCol the one the incident facts share.
const (
	incidentKeyCol = 4
	factKeyCol     = 16
)

// frozenChecklist renders the phase checklist as the run left it: passed
// phases collapsed with their totals, the phase that was in flight expanded
// around the row that failed, and the phases the run never reached pending.
// Empty before the stream screen has handed anything over.
func frozenChecklist(st *State, sty *wizard.ExecStyles, col int) []string {
	var lines []string
	for i := range st.frozen {
		ph := &st.frozen[i]
		switch {
		case i < st.frozenAt:
			lines = append(lines, justify(
				sty.Done.Render(tui.IconSuccess+" "+string(ph.name)),
				sty.Dim.Render(fmtDur(ph.end.Sub(ph.start))),
				col,
			))
		case i == st.frozenAt:
			lines = append(lines, sty.Fail.Render(tui.IconError+" "+string(ph.name)))
			for j := range ph.rows {
				lines = append(lines, "    "+settledRow(sty, &ph.rows[j], col-4))
			}
		default:
			lines = append(lines, sty.Pend.Render(tui.IconPending+" "+string(ph.name)))
		}
	}
	return lines
}

// settledRow renders one checklist row of a finished run. A failure's finish
// pass converts whatever was running into a failed row, so no row here is ever
// still in flight and none needs a spinner.
func settledRow(sty *wizard.ExecStyles, r *stepRow, col int) string {
	switch r.status {
	case rowDone:
		return justify(sty.Done.Render(tui.IconSuccess+" "+r.label), sty.Dim.Render(rowDur(r)), col)
	case rowSkipped:
		return justify(sty.Dim.Render(tui.IconSkip+" "+r.label), sty.Dim.Render("skipped"), col)
	case rowFailed:
		return justify(sty.Fail.Render(tui.IconError+" "+r.label), sty.Dim.Render(rowDur(r)), col)
	default:
		return sty.Pend.Render(tui.IconPending + " " + r.label)
	}
}

// incidentFacts renders the failure's identity through the shared facts
// renderer: the run it belongs to, the step and phase it died in, how long it
// had been going, and the failure's own leading clause.
func incidentFacts(st *State, col int) []string {
	step, phase := failurePoint(st)
	rows := []tui.FactRow{{Key: "run_id", Value: st.RunID}}
	if step != "" {
		rows = append(rows, tui.FactRow{Key: "failed step", Value: step, Highlight: true})
	}
	if phase != "" {
		rows = append(rows, tui.FactRow{Key: "phase", Value: phase})
	}
	rows = append(rows,
		tui.FactRow{Key: "elapsed", Value: fmtDur(st.Elapsed)},
		tui.FactRow{Key: "cause", Value: leadingClause(st.Result)},
	)
	return tui.RenderFacts(rows, &tui.FactLayout{
		Leader: tui.FactLeaderDots, KeyWidth: factKeyCol, TotalWidth: col, Styles: tui.DefaultFactStyles(),
	})
}

// failurePoint names the step and phase the run died in, read off the frozen
// checklist; both empty when the run failed before any row had started.
func failurePoint(st *State) (step, phase string) {
	for i := range st.frozen {
		ph := &st.frozen[i]
		for j := range ph.rows {
			if ph.rows[j].status == rowFailed {
				return ph.rows[j].label, string(ph.name)
			}
		}
	}
	if len(st.frozen) > 0 && st.frozenAt < len(st.frozen) {
		return "", string(st.frozen[st.frozenAt].name)
	}
	return "", ""
}

// leadingClause is the failure's own summary: everything the engine wrote
// before it started naming the underlying call, which is what the error card
// below already prints in full.
func leadingClause(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if i := strings.Index(msg, ": "); i > 0 {
		return msg[:i]
	}
	return msg
}

// nextMoves renders the actionable rows the report ends on: the keys this
// screen answers to, then the commands the operator runs themselves. The
// split is the campaign's destroy-handoff ruling — a resume re-enters the
// engine and a teardown destroys machines, and neither may hide behind a
// single keystroke on the screen that just failed.
func nextMoves(sty *wizard.ExecStyles, col int, copied bool) []string {
	keys := tui.RenderFacts([]tui.FactRow{
		{Key: string(rune(keyFullLog)), Value: "open the full log, filter it, jump between errors"},
		{Key: string(rune(keyCopyRunID)), Value: "copy the run id to the clipboard"},
	}, &tui.FactLayout{
		Leader: tui.FactLeaderPad, KeyWidth: incidentKeyCol, TotalWidth: col,
		Styles: tui.FactStyles{Key: sty.Active, Value: sty.Dim},
	})

	hand := tui.RenderFacts([]tui.FactRow{
		{Key: handoffResume, Value: "resume from the phase the marker recorded"},
		{Key: handoffFresh, Value: "restart from scratch (wipes cluster credentials)"},
		{Key: handoffDestroy, Value: "tear down whatever was built"},
	}, &tui.FactLayout{
		Leader: tui.FactLeaderPad, KeyWidth: factKeyCol + 8, TotalWidth: col,
		Styles: tui.FactStyles{Key: lipgloss.NewStyle().Foreground(tui.ColorText()), Value: sty.Dim},
	})

	lines := append([]string{sty.Dim.Render("NEXT MOVES")}, keys...)
	if copied {
		lines = append(lines, sty.Warn.Render("    sent the run id to the clipboard (OSC 52)"))
	}
	return append(append(lines, "", sty.Dim.Render("run these yourself:")), hand...)
}

// section joins a report's blocks with one blank row between them, dropping
// the blocks that came back empty so no screen ends on a hole.
func section(blocks ...[]string) string {
	var out []string
	for _, b := range blocks {
		if len(b) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, b...)
	}
	return strings.Join(out, "\n")
}

package lifecycle

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// Handoff lines the failure report prints for the operator to run themselves.
// A resume re-enters the backend against live machines, so it stays a command
// the operator types rather than a key this screen answers to.
const (
	handoffResume = "okdctl node manage"
	handoffStatus = "okdctl status"
)

// incidentKeyCol is the key column the next-moves rows align their help in,
// and factKeyCol the one the incident facts share.
const (
	incidentKeyCol = 4
	factKeyCol     = 16
)

// frozenChecklist renders the per-node gate checklist as the operation left
// it: finished nodes collapsed with their totals, the node that was in flight
// expanded around the gate that failed, and untouched nodes pending. Empty
// before the execution screen has handed anything over.
func frozenChecklist(st *State, sty *wizard.ExecStyles, col int) []string {
	var lines []string
	for i := range st.frozen {
		np := &st.frozen[i]
		switch {
		case i < st.frozenAt:
			lines = append(lines, justify(
				sty.Done.Render(tui.IconSuccess+" "+np.name),
				sty.Dim.Render(fmtDur(np.end.Sub(np.start))),
				col,
			))
		case i == st.frozenAt:
			lines = append(lines, sty.Fail.Render(tui.IconError+" "+np.name))
			for j := range np.rows {
				lines = append(lines, "    "+settledRow(sty, &np.rows[j], col-4))
			}
		default:
			lines = append(lines, sty.Pend.Render(tui.IconPending+" "+np.name))
		}
	}
	return lines
}

// settledRow renders one gate row of a finished operation. A failure's finish
// pass converts whatever was running into a failed row, so no row here is ever
// still in flight and none needs a spinner.
func settledRow(sty *wizard.ExecStyles, r *execRow, col int) string {
	switch r.status {
	case rowDone:
		return justify(sty.Done.Render(tui.IconSuccess+" "+r.label), sty.Dim.Render(rowDur(r)), col)
	case rowFailed:
		return justify(sty.Fail.Render(tui.IconError+" "+r.label), sty.Dim.Render(rowDur(r)), col)
	default:
		return sty.Pend.Render(tui.IconPending + " " + r.label)
	}
}

// incidentFacts renders the failure's identity through the shared facts
// renderer: the operation, the gate and node it died in, how long it had been
// going, and the failure's own leading clause.
func incidentFacts(st *State, col int) []string {
	gate, node := failurePoint(st)
	rows := []tui.FactRow{{Key: factKeyOperation, Value: string(st.Op)}}
	if gate != "" {
		rows = append(rows, tui.FactRow{Key: "failed gate", Value: gate, Highlight: true})
	}
	if node != "" {
		rows = append(rows, tui.FactRow{Key: "node", Value: node})
	}
	rows = append(rows,
		tui.FactRow{Key: "elapsed", Value: fmtDur(st.Elapsed)},
		tui.FactRow{Key: "cause", Value: leadingClause(st.Result)},
	)
	return tui.RenderFacts(rows, &tui.FactLayout{
		Leader: tui.FactLeaderDots, KeyWidth: factKeyCol, TotalWidth: col, Styles: tui.DefaultFactStyles(),
	})
}

// failurePoint names the gate and node the operation died in, read off the
// frozen checklist; both empty when it failed before any gate had started.
func failurePoint(st *State) (gate, node string) {
	for i := range st.frozen {
		np := &st.frozen[i]
		for j := range np.rows {
			if np.rows[j].status == rowFailed {
				return np.rows[j].label, np.name
			}
		}
	}
	if len(st.frozen) > 0 && st.frozenAt < len(st.frozen) {
		return "", st.frozen[st.frozenAt].name
	}
	return "", ""
}

// leadingClause is the failure's own summary: everything the backend wrote
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
// screen answers to, then the commands the operator runs themselves. A
// resume drives terraform and oc against live nodes, so it never hides behind
// a single keystroke on the screen that just failed.
func nextMoves(sty *wizard.ExecStyles, col int) []string {
	keys := tui.RenderFacts([]tui.FactRow{
		{Key: string(rune(keyFullLog)), Value: "open the full log, filter it, jump between errors"},
	}, &tui.FactLayout{
		Leader: tui.FactLeaderPad, KeyWidth: incidentKeyCol, TotalWidth: col,
		Styles: tui.FactStyles{Key: sty.Active, Value: sty.Dim},
	})

	hand := tui.RenderFacts([]tui.FactRow{
		{Key: handoffResume, Value: "resume the interrupted operation at the recorded step"},
		{Key: handoffStatus, Value: "check what the cluster makes of the nodes now"},
	}, &tui.FactLayout{
		Leader: tui.FactLeaderPad, KeyWidth: factKeyCol + 8, TotalWidth: col,
		Styles: tui.FactStyles{Key: lipgloss.NewStyle().Foreground(tui.ColorText()), Value: sty.Dim},
	})

	lines := append([]string{sty.Dim.Render("NEXT MOVES")}, keys...)
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

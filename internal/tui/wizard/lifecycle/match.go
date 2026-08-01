package lifecycle

import (
	"strings"

	"github.com/qxtaiba/okdctl/internal/node"
)

// rowCordonDrain is the gate-row label shared by the cordon and drain
// steps; they render as one row.
const rowCordonDrain = "cordon + drain"

// stepRowHints maps a structured OnStep transition to the substring of the
// gate row it belongs to.
var stepRowHints = map[node.Step]string{
	node.StepCordon:     rowCordonDrain,
	node.StepDrain:      rowCordonDrain,
	node.StepTFApply:    "terraform apply",
	node.StepPowerCycle: "power-cycle",
	node.StepUncordon:   "uncordon",
	node.StepDeleteK8s:  "delete kubernetes node",
	node.StepBuildISO:   "build iso",
	node.StepUploadISO:  "upload iso",
	node.StepWaitJoin:   "wait for join",
}

// descRowHints maps a Reporter description prefix to its gate-row
// substring, in match order (first hit wins). Prefixes track the
// startProgress strings in internal/node — matchRow appends unmatched
// descriptions verbatim upstream, so a renamed backend string degrades
// visibly instead of silently.
var descRowHints = []struct{ prefix, row string }{
	{"waiting for etcd health (pre-", "etcd health gate (pre)"},
	{"waiting for etcd health (post-", "etcd health gate (post)"},
	{"waiting for node ", "wait for node ready"},
	{"waiting for ceph health", "ceph health gate"},
	{"cordoning and draining", rowCordonDrain},
	{"applying terraform change", "terraform apply"},
	{"power-cycling vm", "power-cycle"},
	{"waiting for ", "wait for join"},
}

// matchRow resolves an execution event to its gate-row index in rows, or
// -1 when nothing matches. Ceph descriptions fall back to the combined
// "uncordon + ceph" row when the standalone ceph row is absent (resize).
func matchRow(rows []string, ev *ExecEvent) int {
	if hint, ok := stepRowHints[ev.Step]; ok && ev.Step != "" {
		return rowIndex(rows, hint)
	}
	for _, h := range descRowHints {
		if strings.HasPrefix(ev.Desc, h.prefix) {
			if idx := rowIndex(rows, h.row); idx >= 0 {
				return idx
			}
		}
	}
	return -1
}

func rowIndex(rows []string, substr string) int {
	for i, r := range rows {
		if strings.Contains(r, substr) {
			return i
		}
	}
	return -1
}

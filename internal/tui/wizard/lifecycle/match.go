package lifecycle

import (
	"strings"

	"github.com/qxtaiba/okdctl/internal/node"
)

// rowCordonDrain: cordon and drain render as a single combined row.
const rowCordonDrain = "cordon + drain"

var stepRowHints = map[node.Step]string{
	node.StepCordon:     rowCordonDrain,
	node.StepDrain:      rowCordonDrain,
	node.StepTFApply:    "terraform apply",
	node.StepPowerCycle: "power-cycle",
	node.StepDiskGrow:   "grow os disk",
	node.StepUncordon:   "uncordon",
	node.StepDeleteK8s:  "delete kubernetes node",
	node.StepBuildISO:   "build iso",
	node.StepUploadISO:  "upload iso",
	node.StepWaitJoin:   "wait for join",
}

// descRowHints maps a Reporter prefix to its gate-row substring, in match
// order (first hit wins); prefixes track internal/node's startProgress
// strings, so a renamed backend string degrades visibly, not silently.
var descRowHints = []struct{ prefix, row string }{
	{"waiting for etcd health (pre-", "etcd health gate (pre)"},
	{"waiting for etcd health (post-", "etcd health gate (post)"},
	{"waiting for node ", "wait for node ready"},
	{"waiting for ceph health", "ceph health gate"},
	{"cordoning and draining", rowCordonDrain},
	{"applying terraform change", "terraform apply"},
	{"power-cycling vm", "power-cycle"},
	{"growing os disk", "grow os disk"},
	{"waiting for ", "wait for join"},
}

// matchRow resolves an event to its row index, or -1; ceph descriptions
// fall back to "uncordon + ceph" when the standalone row is absent (resize).
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

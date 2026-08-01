package lifecycle

import (
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// GateRows returns the ordered per-node gate checklist for op — the single
// source for both the preview screen's gates line and the execution
// checklist, so the promise and the live view can never diverge.
func GateRows(op node.Op, role nodetypes.NodeRole, skipDrain bool) []string {
	switch op {
	case node.OpResize:
		var rows []string
		if role == nodetypes.RoleMaster {
			rows = append(rows, "etcd health gate (pre)")
		}
		if !skipDrain {
			rows = append(rows, rowCordonDrain)
		}
		rows = append(rows, "terraform apply (in-place update)", "power-cycle vm", "wait for node ready")
		if role == nodetypes.RoleMaster {
			rows = append(rows, "etcd health gate (post)")
		}
		return append(rows, "uncordon + ceph health gate")
	case node.OpRemove:
		var rows []string
		if !skipDrain {
			rows = append(rows, rowCordonDrain)
		}
		return append(rows, "terraform apply (destroy)", "delete kubernetes node", "ceph health gate")
	case node.OpAdd:
		return []string{"build iso", "upload iso", "terraform apply (create)", "wait for join + ready"}
	default:
		return nil
	}
}

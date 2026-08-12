package lifecycle

import (
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// DiskMode describes whether a resize includes an os-disk grow and whether
// that grow is the only dimension (the live, no-power-cycle path).
type DiskMode int

// DiskMode values, in escalating order of disruption avoided.
const (
	DiskNone       DiskMode = iota // no disk change
	DiskWithReboot                 // disk + mem/cpu: grow rides the power-cycle roll
	DiskOnly                       // disk alone: live path
)

// GateRows returns the ordered per-node gate checklist for op — the single
// source for both the preview screen's gates line and the execution
// checklist, so the promise and the live view can never diverge. disk is
// ignored outside OpResize.
func GateRows(op node.Op, role nodetypes.NodeRole, skipDrain bool, disk DiskMode) []string {
	switch op {
	case node.OpResize:
		var rows []string
		if role == nodetypes.RoleMaster {
			rows = append(rows, "etcd health gate (pre)")
		}
		if disk == DiskOnly {
			rows = append(rows, "terraform apply (in-place update)", "grow os disk (live)")
			if role == nodetypes.RoleMaster {
				rows = append(rows, "etcd health gate (post)")
			}
			return rows
		}
		if !skipDrain {
			rows = append(rows, rowCordonDrain)
		}
		rows = append(rows, "terraform apply (in-place update)", "power-cycle vm", "wait for node ready")
		if disk == DiskWithReboot {
			rows = append(rows, "grow os disk (live)")
		}
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

// diskModeFor derives the DiskMode a GateRows caller needs from the
// wizard-collected state: no disk change, a grow riding the power-cycle
// roll, or the disk-only live path.
func diskModeFor(st *State) DiskMode {
	disk := DiskNone
	if st.OSDiskGB > 0 {
		disk = DiskWithReboot
		if st.DiskOnly() {
			disk = DiskOnly
		}
	}
	return disk
}

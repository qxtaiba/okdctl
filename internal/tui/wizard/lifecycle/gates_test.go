package lifecycle

import (
	"slices"
	"testing"

	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func TestGateRows(t *testing.T) {
	cases := []struct {
		name      string
		op        node.Op
		role      nodetypes.NodeRole
		skipDrain bool
		disk      DiskMode
		want      []string
	}{
		{"resize master", node.OpResize, nodetypes.RoleMaster, false, DiskNone, []string{
			"etcd health gate (pre)", "cordon + drain", "terraform apply (in-place update)",
			"power-cycle vm", "wait for node ready", "etcd health gate (post)",
			"uncordon + ceph health gate",
		}},
		{"resize worker skip-drain", node.OpResize, nodetypes.RoleWorker, true, DiskNone, []string{
			"terraform apply (in-place update)", "power-cycle vm",
			"wait for node ready", "uncordon + ceph health gate",
		}},
		{"remove", node.OpRemove, nodetypes.RoleWorker, false, DiskNone, []string{
			"cordon + drain", "terraform apply (destroy)",
			"delete kubernetes node", "ceph health gate",
		}},
		{"add", node.OpAdd, nodetypes.RoleWorker, false, DiskNone, []string{
			"build iso", "upload iso", "terraform apply (create)", "wait for join + ready",
		}},
		{"unknown", node.Op("bogus"), nodetypes.RoleWorker, false, DiskNone, nil},
	}
	for _, tc := range cases {
		if got := GateRows(tc.op, tc.role, tc.skipDrain, tc.disk); !slices.Equal(got, tc.want) {
			t.Errorf("%s: GateRows = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestGateRowsResizeDiskOnly(t *testing.T) {
	rows := GateRows(node.OpResize, nodetypes.RoleMaster, false, DiskOnly)
	want := []string{
		"etcd health gate (pre)",
		"terraform apply (in-place update)",
		"grow os disk (live)",
		"etcd health gate (post)",
	}
	if !slices.Equal(rows, want) {
		t.Fatalf("rows = %q, want %q", rows, want)
	}
}

func TestGateRowsResizeCombinedIncludesGrow(t *testing.T) {
	rows := GateRows(node.OpResize, nodetypes.RoleWorker, false, DiskWithReboot)
	// combined keeps today's rows and inserts the grow after wait-ready
	if idx := rowIndex(rows, "grow os disk"); idx < 0 || rows[idx-1] != "wait for node ready" {
		t.Fatalf("grow row misplaced in %q", rows)
	}
}

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
		want      []string
	}{
		{"resize master", node.OpResize, nodetypes.RoleMaster, false, []string{
			"etcd health gate (pre)", "cordon + drain", "terraform apply (in-place update)",
			"power-cycle vm", "wait for node ready", "etcd health gate (post)",
			"uncordon + ceph health gate",
		}},
		{"resize worker skip-drain", node.OpResize, nodetypes.RoleWorker, true, []string{
			"terraform apply (in-place update)", "power-cycle vm",
			"wait for node ready", "uncordon + ceph health gate",
		}},
		{"remove", node.OpRemove, nodetypes.RoleWorker, false, []string{
			"cordon + drain", "terraform apply (destroy)",
			"delete kubernetes node", "ceph health gate",
		}},
		{"add", node.OpAdd, nodetypes.RoleWorker, false, []string{
			"build iso", "upload iso", "terraform apply (create)", "wait for join + ready",
		}},
		{"unknown", node.Op("bogus"), nodetypes.RoleWorker, false, nil},
	}
	for _, tc := range cases {
		if got := GateRows(tc.op, tc.role, tc.skipDrain); !slices.Equal(got, tc.want) {
			t.Errorf("%s: GateRows = %v, want %v", tc.name, got, tc.want)
		}
	}
}

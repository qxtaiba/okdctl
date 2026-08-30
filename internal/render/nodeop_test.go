package render

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func removePlan() node.OpPlan {
	return node.OpPlan{
		Op:           node.OpRemove,
		Cluster:      "grappleberry",
		DrainTimeout: "10m",
		Nodes: []node.PlanNode{{
			Name:      "worker2",
			Role:      nodetypes.RoleWorker,
			TFAddress: "module.okd_cluster.proxmox_virtual_environment_vm.worker[2]",
			Action:    terraform.PlanActionDelete,
			OSDs:      []string{"rook-ceph/osd-3"},
		}},
	}
}

func resizePlan() node.OpPlan {
	return node.OpPlan{
		Op:      node.OpResize,
		Cluster: "grappleberry",
		Nodes: []node.PlanNode{{
			Name:      "master0",
			Role:      nodetypes.RoleMaster,
			TFAddress: "module.okd_cluster.proxmox_virtual_environment_vm.master[0]",
			Action:    terraform.PlanActionUpdate,
		}},
		MemoryMB: 24576,
	}
}

func addPlan() node.OpPlan {
	return node.OpPlan{
		Op:      node.OpAdd,
		Cluster: "grappleberry",
		Nodes: []node.PlanNode{{
			Name:      "grappleberry-worker2",
			Role:      nodetypes.RoleWorker,
			TFAddress: "module.okd_cluster.proxmox_virtual_environment_vm.worker[2]",
			Action:    terraform.PlanActionCreate,
		}},
	}
}

func powerPlan(op node.Op) node.OpPlan {
	return node.OpPlan{
		Op:      op,
		Cluster: "grappleberry",
		Nodes: []node.PlanNode{
			{Name: "master0", Role: nodetypes.RoleMaster, Action: terraform.PlanActionNoop},
			{Name: "worker0", Role: nodetypes.RoleWorker, Action: terraform.PlanActionNoop},
		},
	}
}

func TestNodeOpBoxes(t *testing.T) {
	cases := []struct {
		name       string
		render     func() string
		want       []string
		wantAbsent []string
	}{
		{
			name:   "confirm remove flags irreversible destroy",
			render: func() string { p := removePlan(); return NodeOpConfirm(&p) },
			want: []string{
				"confirm worker removal", "grappleberry", "worker[2]", "10m",
				"rook-ceph OSD", "irreversible", "data disk",
			},
		},
		{
			name:       "confirm resize has no irreversible line",
			render:     func() string { p := resizePlan(); return NodeOpConfirm(&p) },
			want:       []string{"24576 MiB"},
			wantAbsent: []string{"irreversible"},
		},
		{
			name: "confirm reports blocked verdict",
			render: func() string {
				p := removePlan()
				p.Nodes[0].Blocked = errors.New("holds 1 rook-ceph OSD")
				return NodeOpConfirm(&p)
			},
			want: []string{"blocked", "rook-ceph OSD"},
		},
		{
			name:       "confirm add has no irreversible line",
			render:     func() string { p := addPlan(); return NodeOpConfirm(&p) },
			want:       []string{"confirm node add", "grappleberry", "worker[2]", "ignition server", "revived"},
			wantAbsent: []string{"irreversible"},
		},
		{
			name:       "confirm stop has no tf address or irreversible",
			render:     func() string { p := powerPlan(node.OpStop); return NodeOpConfirm(&p) },
			want:       []string{"confirm cluster stop", "grappleberry", "worker0", "shut down"},
			wantAbsent: []string{"irreversible", "no-op"},
		},
		{
			name:       "confirm start has no tf address or irreversible",
			render:     func() string { p := powerPlan(node.OpStart); return NodeOpConfirm(&p) },
			want:       []string{"confirm cluster start", "grappleberry", "master0", "powered on"},
			wantAbsent: []string{"irreversible", "no-op"},
		},
		{
			name:   "complete remove lists nodes and next steps",
			render: func() string { p := removePlan(); return NodeOpComplete(&p, 90*time.Second) },
			want:   []string{"worker removed", "worker2", "1m30s", "haproxy"},
		},
		{
			name:   "complete add lists nodes and next steps",
			render: func() string { p := addPlan(); return NodeOpComplete(&p, 5*time.Minute) },
			want:   []string{"worker(s) added", "grappleberry-worker2", "added", "haproxy", "joined"},
		},
		{
			name:   "complete stop uses stopped verb",
			render: func() string { p := powerPlan(node.OpStop); return NodeOpComplete(&p, 45*time.Second) },
			want:   []string{"cluster stopped", "worker0", "stopped", "okdctl cluster start"},
		},
		{
			name:   "complete start uses started verb",
			render: func() string { p := powerPlan(node.OpStart); return NodeOpComplete(&p, 45*time.Second) },
			want:   []string{"cluster started", "master0", "started", "okdctl status"},
		},
		{
			name:   "dry-run remove marks no changes",
			render: func() string { p := removePlan(); return NodeOpDryRun(&p) },
			want:   []string{"dry-run — no changes made", "worker[2]"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.render()
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("box missing %q:\n%s", want, got)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("box must not contain %q:\n%s", absent, got)
				}
			}
		})
	}
}

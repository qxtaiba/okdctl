package render

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func removePlan() node.OpPlan {
	return node.OpPlan{
		Op:           node.OpRemove,
		Cluster:      "grappleberry",
		DrainTimeout: "10m",
		Nodes: []node.PlanNode{{
			Name:      "worker2",
			Role:      nodetypes.RoleWorker,
			TFAddress: "module.vm.worker[2]",
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
			TFAddress: "module.vm.master[0]",
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
			TFAddress: "module.vm.worker[2]",
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

func TestNodeOpConfirmTableHeadersAndFit(t *testing.T) {
	for _, w := range []int{80, 120} {
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			tui.SetTerminalWidth(w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			p := removePlan()
			out := NodeOpConfirm(&p)

			for _, header := range []string{"NODE", "ROLE", "ADDRESS", "ACTION"} {
				if !strings.Contains(out, header) {
					t.Errorf("confirm box missing table header %q:\n%s", header, out)
				}
			}
			tuitest.AssertFits(t, out, w, 0)
			if w == 80 {
				tuitest.Golden(t, "nodeop-confirm-remove-80", out)
			}
		})
	}
}

// Regression guard for the box-growth-era truncation bug: a real-length
// terraform address must keep its trailing [N] index once the ADDRESS
// column middle-truncates it, instead of losing it to a trailing ellipsis.
func TestNodeOpConfirmTableAddressTruncationKeepsIndexTail(t *testing.T) {
	const realAddress = "module.okd_cluster.proxmox_virtual_environment_vm.worker[3]"
	p := node.OpPlan{
		Op:      node.OpRemove,
		Cluster: "grappleberry",
		Nodes: []node.PlanNode{{
			Name:      "worker3",
			Role:      nodetypes.RoleWorker,
			TFAddress: realAddress,
			Action:    terraform.PlanActionDelete,
		}},
	}
	out := NodeOpConfirm(&p)

	if !strings.Contains(out, "worker[3]") {
		t.Errorf("ADDRESS column must middle-truncate a long address and keep the [N] index tail:\n%s", out)
	}
	if strings.Contains(out, realAddress) {
		t.Errorf("a %d-char address should have been truncated in the ADDRESS column, not rendered whole:\n%s", len(realAddress), out)
	}
	tuitest.AssertFits(t, out, tui.DefaultBoxWidth, 0)
}

// Regression guard: a "dry-run — no changes made" box is a self-contradiction
// if its table claims a node was already "removed" — the pre-execution
// table speaks the raw plan action, not the completion verb.
func TestNodeOpDryRunTableSpeaksPlanVoiceNotCompletionVoice(t *testing.T) {
	p := removePlan()
	out := NodeOpDryRun(&p)

	if !strings.Contains(out, "delete") {
		t.Errorf("dry-run table must show the raw plan action %q:\n%s", "delete", out)
	}
	if strings.Contains(out, "removed") {
		t.Errorf("dry-run box must not speak completion voice (%q); nothing has changed yet:\n%s", "removed", out)
	}
}

// Regression guard: the completion box speaks the past-tense completion
// verb, not the raw plan action the table showed before the op ran.
func TestNodeOpCompleteTableSpeaksCompletionVoiceNotPlanVoice(t *testing.T) {
	p := removePlan()
	out := NodeOpComplete(&p, 90*time.Second)

	if !strings.Contains(out, "removed") {
		t.Errorf("completion box must show the completion verb %q:\n%s", "removed", out)
	}
	if strings.Contains(out, "delete") {
		t.Errorf("completion box must not show the raw plan action %q:\n%s", "delete", out)
	}
}

func TestShortHost(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"dotted FQDN keeps the first label", "worker2.cluster.local", "worker2"},
		{"no dot returns the name unchanged", "worker2", "worker2"},
		{"IPv4 address returns unchanged", "192.168.1.24", "192.168.1.24"},
		{"IPv6 address returns unchanged", "fe80::1", "fe80::1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortHost(tc.in); got != tc.want {
				t.Errorf("shortHost(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNodeOpCompleteWidthShrinksBox(t *testing.T) {
	p := resizePlan()
	const viewport = 70
	got := NodeOpCompleteWidth(&p, 90*time.Second, viewport-2)
	for _, line := range strings.Split(got, "\n") {
		if w := lipgloss.Width(line); w > viewport {
			t.Errorf("box line %d cols wide, want <= %d: %q", w, viewport, line)
		}
	}
}

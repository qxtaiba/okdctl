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

func TestNodeOpConfirmFlagsIrreversibleDestroy(t *testing.T) {
	plan := removePlan()
	got := NodeOpConfirm(&plan)
	for _, want := range []string{
		"confirm worker removal", "grappleberry", "worker[2]", "10m",
		"rook-ceph OSD", "irreversible", "data disk",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("confirm box missing %q:\n%s", want, got)
		}
	}
}

func TestNodeOpConfirmResizeHasNoIrreversibleLine(t *testing.T) {
	plan := node.OpPlan{
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
	got := NodeOpConfirm(&plan)
	if strings.Contains(got, "irreversible") {
		t.Errorf("in-place resize must not carry the irreversible warning:\n%s", got)
	}
	if !strings.Contains(got, "24576 MiB") {
		t.Errorf("resize box should name the target memory:\n%s", got)
	}
}

func TestNodeOpConfirmReportsBlockedVerdict(t *testing.T) {
	plan := removePlan()
	plan.Nodes[0].Blocked = errors.New("holds 1 rook-ceph OSD")
	got := NodeOpConfirm(&plan)
	if !strings.Contains(got, "blocked") || !strings.Contains(got, "rook-ceph OSD") {
		t.Errorf("blocked verdict should surface in the box:\n%s", got)
	}
}

func TestNodeOpCompleteListsNodesAndNextSteps(t *testing.T) {
	plan := removePlan()
	got := NodeOpComplete(&plan, 90*time.Second)
	for _, want := range []string{"worker removed", "worker2", "1m30s", "haproxy"} {
		if !strings.Contains(got, want) {
			t.Errorf("completion box missing %q:\n%s", want, got)
		}
	}
}

func TestNodeOpDryRunMarksNoChanges(t *testing.T) {
	plan := removePlan()
	got := NodeOpDryRun(&plan)
	if !strings.Contains(got, "dry-run — no changes made") {
		t.Errorf("dry-run box missing the no-changes banner:\n%s", got)
	}
	if !strings.Contains(got, "worker[2]") {
		t.Errorf("dry-run box should list the planned operation:\n%s", got)
	}
}

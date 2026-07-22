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

func TestNodeOpConfirmAddHasNoIrreversibleLine(t *testing.T) {
	plan := addPlan()
	got := NodeOpConfirm(&plan)
	for _, want := range []string{
		"confirm node add", "grappleberry", "worker[2]", "ignition server", "revived",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("add confirm box missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "irreversible") {
		t.Errorf("node add must not carry the irreversible warning:\n%s", got)
	}
}

func TestNodeOpDryRunAddMarksNoChanges(t *testing.T) {
	plan := addPlan()
	got := NodeOpDryRun(&plan)
	if !strings.Contains(got, "dry-run — no changes made") {
		t.Errorf("dry-run box missing the no-changes banner:\n%s", got)
	}
	if !strings.Contains(got, "worker[2]") {
		t.Errorf("dry-run box should list the planned operation:\n%s", got)
	}
}

func TestNodeOpCompleteAddListsNodesAndNextSteps(t *testing.T) {
	plan := addPlan()
	got := NodeOpComplete(&plan, 5*time.Minute)
	for _, want := range []string{"worker(s) added", "grappleberry-worker2", "added", "haproxy", "joined"} {
		if !strings.Contains(got, want) {
			t.Errorf("add completion box missing %q:\n%s", want, got)
		}
	}
}

func stopPlan() node.OpPlan {
	return node.OpPlan{
		Op:      node.OpStop,
		Cluster: "grappleberry",
		Nodes: []node.PlanNode{
			{Name: "worker0", Role: nodetypes.RoleWorker, Action: terraform.PlanActionNoop},
			{Name: "master0", Role: nodetypes.RoleMaster, Action: terraform.PlanActionNoop},
		},
	}
}

func startPlan() node.OpPlan {
	return node.OpPlan{
		Op:      node.OpStart,
		Cluster: "grappleberry",
		Nodes: []node.PlanNode{
			{Name: "master0", Role: nodetypes.RoleMaster, Action: terraform.PlanActionNoop},
			{Name: "worker0", Role: nodetypes.RoleWorker, Action: terraform.PlanActionNoop},
		},
	}
}

func TestNodeOpConfirmStopHasNoTFAddressOrIrreversible(t *testing.T) {
	plan := stopPlan()
	got := NodeOpConfirm(&plan)
	for _, want := range []string{"confirm cluster stop", "grappleberry", "worker0", "shut down"} {
		if !strings.Contains(got, want) {
			t.Errorf("stop confirm box missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "irreversible") {
		t.Errorf("cluster stop must not carry the irreversible warning:\n%s", got)
	}
	if strings.Contains(got, "no-op") {
		t.Errorf("cluster stop must not print a terraform action/address:\n%s", got)
	}
}

func TestNodeOpConfirmStartHasNoTFAddressOrIrreversible(t *testing.T) {
	plan := startPlan()
	got := NodeOpConfirm(&plan)
	for _, want := range []string{"confirm cluster start", "grappleberry", "master0", "powered on"} {
		if !strings.Contains(got, want) {
			t.Errorf("start confirm box missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "irreversible") {
		t.Errorf("cluster start must not carry the irreversible warning:\n%s", got)
	}
	if strings.Contains(got, "no-op") {
		t.Errorf("cluster start must not print a terraform action/address:\n%s", got)
	}
}

func TestNodeOpCompleteStopUsesStoppedVerb(t *testing.T) {
	plan := stopPlan()
	got := NodeOpComplete(&plan, 45*time.Second)
	for _, want := range []string{"cluster stopped", "worker0", "stopped", "okdctl cluster start"} {
		if !strings.Contains(got, want) {
			t.Errorf("stop completion box missing %q:\n%s", want, got)
		}
	}
}

func TestNodeOpCompleteStartUsesStartedVerb(t *testing.T) {
	plan := startPlan()
	got := NodeOpComplete(&plan, 45*time.Second)
	for _, want := range []string{"cluster started", "master0", "started", "okdctl status"} {
		if !strings.Contains(got, want) {
			t.Errorf("start completion box missing %q:\n%s", want, got)
		}
	}
}

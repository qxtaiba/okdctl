package render

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
)

func TestPlanPreview_Clean(t *testing.T) {
	got := PlanPreview(nil)
	for _, want := range []string{"PLAN PREVIEW", "no drift"} {
		if !strings.Contains(got, want) {
			t.Errorf("clean plan preview missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "drift detected") {
		t.Errorf("clean plan preview must not claim drift:\n%s", got)
	}
}

func TestPlanPreview_Drifted(t *testing.T) {
	changes := []terraform.ResourceChange{
		{Address: "module.okd_cluster.proxmox_virtual_environment_vm.worker[2]", Action: terraform.PlanActionUpdate},
		{Address: "module.okd_cluster.proxmox_virtual_environment_vm.worker[3]", Action: terraform.PlanActionDelete},
	}
	got := PlanPreview(changes)
	for _, want := range []string{
		"PLAN PREVIEW", "drift detected", "2 pending change(s)",
		"worker[2]", "worker[3]", "update", "delete",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("drifted plan preview missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "no drift") {
		t.Errorf("drifted plan preview must not claim clean:\n%s", got)
	}
}

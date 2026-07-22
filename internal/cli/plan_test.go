package cli

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
)

func TestNewPlanJSONOutput_Clean(t *testing.T) {
	out := newPlanJSONOutput(nil)
	if out.Drift {
		t.Error("Drift = true for an empty change list")
	}
	if out.Changes == nil || len(out.Changes) != 0 {
		t.Errorf("Changes = %#v; want empty non-nil slice", out.Changes)
	}
}

func TestNewPlanJSONOutput_Drifted(t *testing.T) {
	changes := []terraform.ResourceChange{
		{Address: "module.okd_cluster.proxmox_virtual_environment_vm.worker[2]", Action: terraform.PlanActionUpdate},
		{Address: "module.okd_cluster.proxmox_virtual_environment_vm.worker[3]", Action: terraform.PlanActionDelete},
	}
	out := newPlanJSONOutput(changes)
	if !out.Drift {
		t.Error("Drift = false for a non-empty change list")
	}
	if len(out.Changes) != 2 {
		t.Fatalf("len(Changes) = %d; want 2", len(out.Changes))
	}
	if out.Changes[0].Address != changes[0].Address || out.Changes[0].Action != string(changes[0].Action) {
		t.Errorf("Changes[0] = %+v; want %+v", out.Changes[0], changes[0])
	}
}

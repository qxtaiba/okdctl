package render

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
)

func TestPlanPreview(t *testing.T) {
	drift := []terraform.ResourceChange{
		{Address: "module.okd_cluster.proxmox_virtual_environment_vm.worker[2]", Action: terraform.PlanActionUpdate},
		{Address: "module.okd_cluster.proxmox_virtual_environment_vm.worker[3]", Action: terraform.PlanActionDelete},
	}
	cases := []struct {
		name    string
		changes []terraform.ResourceChange
		want    []string
		absent  string
	}{
		{"clean", nil, []string{"PLAN PREVIEW", "no drift"}, "drift detected"},
		{"drifted", drift, []string{
			"PLAN PREVIEW", "drift detected", "2 pending change(s)",
			"worker[2]", "worker[3]", "update", "delete",
		}, "no drift"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PlanPreview(tc.changes)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("%s plan preview missing %q:\n%s", tc.name, want, got)
				}
			}
			if strings.Contains(got, tc.absent) {
				t.Errorf("%s plan preview must not contain %q:\n%s", tc.name, tc.absent, got)
			}
		})
	}
}

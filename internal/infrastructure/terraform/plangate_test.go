package terraform

import (
	"strings"
	"testing"
)

func TestFoldActions(t *testing.T) {
	cases := []struct {
		in   []string
		want PlanAction
	}{
		{[]string{"no-op"}, PlanActionNoop},
		{[]string{"read"}, PlanActionNoop},
		{[]string{"create"}, PlanActionCreate},
		{[]string{"update"}, PlanActionUpdate},
		{[]string{"delete"}, PlanActionDelete},
		{[]string{"delete", "create"}, PlanActionReplace},
		{[]string{"create", "delete"}, PlanActionReplace},
		{[]string{"update", "delete"}, PlanActionUnknown},
		{nil, PlanActionUnknown},
	}
	for _, c := range cases {
		if got := foldActions(c.in); got != c.want {
			t.Errorf("foldActions(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

const planJSON = `{
  "resource_changes": [
    {"address": "module.okd_cluster.proxmox_virtual_environment_vm.worker[2]", "change": {"actions": ["delete"]}},
    {"address": "module.okd_cluster.proxmox_virtual_environment_vm.worker[0]", "change": {"actions": ["no-op"]}},
    {"address": "module.okd_cluster.proxmox_virtual_environment_vm.master[0]", "change": {"actions": ["update"]}}
  ]
}`

func TestParsePlanChangesDropsNoop(t *testing.T) {
	changes, err := ParsePlanChanges([]byte(planJSON))
	if err != nil {
		t.Fatalf("ParsePlanChanges: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 non-noop changes, got %d: %+v", len(changes), changes)
	}
}

func TestParsePlanChangesInvalidJSON(t *testing.T) {
	if _, err := ParsePlanChanges([]byte("{not json")); err == nil {
		t.Fatal("expected error on invalid json")
	}
}

func TestAssertOnlyChange(t *testing.T) {
	addr := "module.okd_cluster.proxmox_virtual_environment_vm.worker[2]"

	t.Run("exact match passes", func(t *testing.T) {
		changes := []ResourceChange{{Address: addr, Action: PlanActionDelete}}
		if err := AssertOnlyChange(changes, addr, PlanActionDelete); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})

	t.Run("empty plan fails with variable hint", func(t *testing.T) {
		err := AssertOnlyChange(nil, addr, PlanActionDelete)
		if err == nil || !strings.Contains(err.Error(), "empty") {
			t.Fatalf("want empty-plan error, got %v", err)
		}
	})

	t.Run("extra change fails", func(t *testing.T) {
		changes := []ResourceChange{
			{Address: addr, Action: PlanActionDelete},
			{Address: "module.okd_cluster.proxmox_virtual_environment_vm.master[0]", Action: PlanActionUpdate},
		}
		if err := AssertOnlyChange(changes, addr, PlanActionDelete); err == nil {
			t.Fatal("want error on extra change")
		}
	})

	t.Run("wrong action fails", func(t *testing.T) {
		changes := []ResourceChange{{Address: addr, Action: PlanActionReplace}}
		if err := AssertOnlyChange(changes, addr, PlanActionUpdate); err == nil {
			t.Fatal("want error on replace when update expected")
		}
	})

	t.Run("wrong address fails", func(t *testing.T) {
		changes := []ResourceChange{{Address: "other", Action: PlanActionDelete}}
		if err := AssertOnlyChange(changes, addr, PlanActionDelete); err == nil {
			t.Fatal("want error on wrong address")
		}
	})
}

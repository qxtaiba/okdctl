package node

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func TestResolveResizeTargetsRole(t *testing.T) {
	nodes := []cluster.NodeDetail{
		{Name: "master2", Role: nodetypes.RoleMaster},
		{Name: "master0", Role: nodetypes.RoleMaster},
		{Name: "master1", Role: nodetypes.RoleMaster},
		{Name: "worker0", Role: nodetypes.RoleWorker},
	}
	targets, role, err := resolveResizeTargets(nodes, ResizeScope{Role: nodetypes.RoleMaster})
	if err != nil {
		t.Fatalf("resolveResizeTargets: %v", err)
	}
	if role != nodetypes.RoleMaster || len(targets) != 3 {
		t.Fatalf("want 3 masters, got role=%s n=%d", role, len(targets))
	}
	// sorted ascending by index so control-plane rollout is deterministic
	if targets[0].index != 0 || targets[1].index != 1 || targets[2].index != 2 {
		t.Fatalf("targets not sorted ascending: %+v", targets)
	}
}

func TestResolveResizeTargetsSingleNode(t *testing.T) {
	nodes := []cluster.NodeDetail{
		{Name: "worker3", Role: nodetypes.RoleWorker},
	}
	targets, role, err := resolveResizeTargets(nodes, ResizeScope{Node: "worker3"})
	if err != nil {
		t.Fatalf("resolveResizeTargets: %v", err)
	}
	if role != nodetypes.RoleWorker || len(targets) != 1 || targets[0].index != 3 {
		t.Fatalf("unexpected single-node target: role=%s %+v", role, targets)
	}
}

func TestResolveResizeTargetsErrors(t *testing.T) {
	nodes := []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker}}
	if _, _, err := resolveResizeTargets(nodes, ResizeScope{Node: "ghost"}); err == nil {
		t.Fatal("want not-found error for missing node")
	}
	if _, _, err := resolveResizeTargets(nodes, ResizeScope{Role: nodetypes.RoleMaster}); err == nil {
		t.Fatal("want error when no nodes of role")
	}
}

func TestCompactOrdering(t *testing.T) {
	nodes := []cluster.NodeDetail{
		{Name: "worker0", Role: nodetypes.RoleWorker},
		{Name: "worker2", Role: nodetypes.RoleWorker},
		{Name: "worker1", Role: nodetypes.RoleWorker},
		{Name: "master0", Role: nodetypes.RoleMaster},
		{Name: "master1", Role: nodetypes.RoleMaster},
	}
	desc := workersByIndexDesc(nodes, logutil.NopLogger)
	if len(desc) != 3 || desc[0] != "worker2" || desc[2] != "worker0" {
		t.Fatalf("workers must be removed top-down: %v", desc)
	}
	asc := mastersByIndexAsc(nodes, logutil.NopLogger)
	if len(asc) != 2 || asc[0] != "master0" || asc[1] != "master1" {
		t.Fatalf("masters must grow low-to-high: %v", asc)
	}
}

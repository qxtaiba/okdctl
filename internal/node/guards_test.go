package node

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func workers(names ...string) []cluster.NodeDetail {
	out := make([]cluster.NodeDetail, len(names))
	for i, n := range names {
		out[i] = cluster.NodeDetail{Name: n, Role: nodetypes.RoleWorker}
	}
	return out
}

func TestValidateWorkerRemovable(t *testing.T) {
	nodes := append(workers("worker0", "worker1", "worker2"),
		cluster.NodeDetail{Name: "master0", Role: nodetypes.RoleMaster})

	t.Run("top index passes", func(t *testing.T) {
		if err := validateWorkerRemovable(nodes, "worker2", 3); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})

	t.Run("non-top index refused", func(t *testing.T) {
		err := validateWorkerRemovable(nodes, "worker0", 3)
		if err == nil || !strings.Contains(err.Error(), "top-down") {
			t.Fatalf("want top-down error, got %v", err)
		}
	})

	t.Run("master refused", func(t *testing.T) {
		err := validateWorkerRemovable(nodes, "master0", 3)
		if err == nil || !strings.Contains(err.Error(), "only worker") {
			t.Fatalf("want master-refusal, got %v", err)
		}
	})

	t.Run("missing node refused", func(t *testing.T) {
		if err := validateWorkerRemovable(nodes, "worker9", 3); err == nil {
			t.Fatal("want not-found error")
		}
	})

	t.Run("count mismatch refused", func(t *testing.T) {
		err := validateWorkerRemovable(nodes, "worker2", 5)
		if err == nil || !strings.Contains(err.Error(), "worker_count") {
			t.Fatalf("want count-mismatch error, got %v", err)
		}
	})
}

func TestOsdPodsOnNode(t *testing.T) {
	pods := []cluster.PodPlacement{
		{Name: "osd-0", Namespace: "rook-ceph", NodeName: "worker2"},
		{Name: "osd-1", Namespace: "rook-ceph", NodeName: "worker0"},
		{Name: "osd-2", Namespace: "rook-ceph", NodeName: "worker2"},
	}
	got := osdPodsOnNode(pods, "worker2")
	if len(got) != 2 {
		t.Fatalf("want 2 osds on worker2, got %v", got)
	}
	if len(osdPodsOnNode(pods, "worker1")) != 0 {
		t.Fatal("want 0 osds on worker1")
	}
}

func TestIngressPodsOnWorkers(t *testing.T) {
	pods := []cluster.PodPlacement{
		{Name: "router-a", NodeName: "worker0"},
		{Name: "router-b", NodeName: "master0"},
	}
	set := map[string]bool{"worker0": true, "worker1": true}
	got := ingressPodsOnWorkers(pods, set)
	if len(got) != 1 || got[0].Name != "router-a" {
		t.Fatalf("want router-a on a worker, got %+v", got)
	}
}

func TestValidateMemoryBudget(t *testing.T) {
	t.Run("shrink always passes", func(t *testing.T) {
		if err := validateMemoryBudget(96000, 90000, -4000); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})
	t.Run("fits", func(t *testing.T) {
		if err := validateMemoryBudget(96000, 60000, 8000); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})
	t.Run("overcommit refused", func(t *testing.T) {
		err := validateMemoryBudget(96000, 90000, 8000)
		if err == nil || !strings.Contains(err.Error(), "memory budget") {
			t.Fatalf("want budget error, got %v", err)
		}
	})
}

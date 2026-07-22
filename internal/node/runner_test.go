package node

import (
	"context"
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func TestResolveVMID(t *testing.T) {
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "master0", Role: nodetypes.RoleMaster, Ready: true},
			{Name: "worker1", Role: nodetypes.RoleWorker, Ready: false},
		},
	}
	ftf := &fakeTF{}
	cfg := config.DefaultConfig()
	cfg.Topology.VMIDBase = 6000
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r, _, _ := seedRunner(t, fc, ftf, cfg)

	vmid, role, ready, err := r.resolveVMID(context.Background(), "master0")
	if err != nil {
		t.Fatalf("resolveVMID(master0): %v", err)
	}
	if vmid != 6010 || role != nodetypes.RoleMaster || !ready {
		t.Errorf("resolveVMID(master0) = (%d, %q, %v); want (6010, master, true)", vmid, role, ready)
	}

	vmid, role, ready, err = r.resolveVMID(context.Background(), "worker1")
	if err != nil {
		t.Fatalf("resolveVMID(worker1): %v", err)
	}
	if vmid != 6101 || role != nodetypes.RoleWorker || ready {
		t.Errorf("resolveVMID(worker1) = (%d, %q, %v); want (6101, worker, false)", vmid, role, ready)
	}
}

func TestResolveVMID_NotFound(t *testing.T) {
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster}}}
	ftf := &fakeTF{}
	cfg := config.DefaultConfig()
	r, _, _ := seedRunner(t, fc, ftf, cfg)

	if _, _, _, err := r.resolveVMID(context.Background(), "worker9"); err == nil {
		t.Fatal("expected error for a node not present in the cluster")
	}
}

func TestResolveVMID_ListNodesError(t *testing.T) {
	fc := &fakeCluster{listErr: errors.New("api unreachable")}
	ftf := &fakeTF{}
	cfg := config.DefaultConfig()
	r, _, _ := seedRunner(t, fc, ftf, cfg)

	if _, _, _, err := r.resolveVMID(context.Background(), "master0"); err == nil {
		t.Fatal("expected error when ListNodes fails")
	}
}

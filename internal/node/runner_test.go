package node

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
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

// TestNewRunner_OptionsDeriveDirsAndDefaults locks the constructor contract:
// WorkDir and EnvDir derive from the option-supplied project root and
// terraform env regardless of option order, a nil logger normalizes to
// no-op, and the default timeouts and snapshot client are wired.
func TestNewRunner_OptionsDeriveDirsAndDefaults(t *testing.T) {
	cfg := config.DefaultConfig()
	projRoot := t.TempDir()
	configPath := filepath.Join(projRoot, "okdctl.yaml")
	r := NewRunner(
		nil, nil, cfg,
		WithLogger(nil),
		WithTerraformEnv("production"),
		WithProjectRoot(projRoot),
		WithConfigPath(configPath),
		WithRunID("run-42"),
	)

	if r.WorkDir != filepath.Join(projRoot, system.WorkDirName) {
		t.Errorf("WorkDir = %q; want derived from project root", r.WorkDir)
	}
	if r.EnvDir != system.TerraformEnvDir(projRoot, "production") {
		t.Errorf("EnvDir = %q; want derived from project root + tf env", r.EnvDir)
	}
	if r.ConfigPath != configPath || r.RunID != "run-42" {
		t.Errorf("ConfigPath/RunID = %q/%q; want option values", r.ConfigPath, r.RunID)
	}
	if r.Log == nil {
		t.Error("nil logger must normalize to a no-op logger")
	}
	if r.Reporter == nil || r.Snapshot == nil {
		t.Error("Reporter and Snapshot defaults must be wired")
	}
	if r.NodeReadyTimeout != DefaultNodeReadyTimeout || r.SnapshotTaskTimeout != DefaultSnapshotTaskTimeout {
		t.Errorf("default timeouts not applied: %v %v", r.NodeReadyTimeout, r.SnapshotTaskTimeout)
	}
}

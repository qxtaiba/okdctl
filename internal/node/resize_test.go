package node

import (
	"errors"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
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

func TestRoleSizingVarsEmitsDiskVar(t *testing.T) {
	vars := roleSizingVars(nodetypes.RoleMaster, ResizeOptions{MemoryMB: 16384, OSDiskGB: 100})
	if got := vars["master_os_disk_size_gb"]; got != "100" {
		t.Fatalf("master_os_disk_size_gb = %q, want 100", got)
	}
	vars = roleSizingVars(nodetypes.RoleWorker, ResizeOptions{MemoryMB: 8192, OSDiskGB: 80})
	if got := vars["worker_os_disk_size_gb"]; got != "80" {
		t.Fatalf("worker_os_disk_size_gb = %q, want 80", got)
	}
	// omitted disk emits no key at all — an absent -var must not shrink to zero
	vars = roleSizingVars(nodetypes.RoleMaster, ResizeOptions{MemoryMB: 16384})
	if _, ok := vars["master_os_disk_size_gb"]; ok {
		t.Fatal("disk var emitted without --os-disk-gb")
	}
}

func TestApplyRoleSizingPersistsDiskGB(t *testing.T) {
	r := &Runner{Cfg: &config.Config{}}
	r.Cfg.Topology.ControlPlane.DiskGB = 50
	r.applyRoleSizing(nodetypes.RoleMaster, ResizeOptions{MemoryMB: 16384, OSDiskGB: 100})
	if got := r.Cfg.Topology.ControlPlane.DiskGB; got != 100 {
		t.Fatalf("ControlPlane.DiskGB = %d, want 100", got)
	}
	// omitted disk keeps the current value
	r.applyRoleSizing(nodetypes.RoleMaster, ResizeOptions{MemoryMB: 16384})
	if got := r.Cfg.Topology.ControlPlane.DiskGB; got != 100 {
		t.Fatalf("ControlPlane.DiskGB = %d after no-disk resize, want 100", got)
	}
}

func TestResizeRefusesDiskShrink(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster}},
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	cfg := config.DefaultConfig()

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.Cfg.Topology.ControlPlane.DiskGB = 100
	err := r.Resize(t.Context(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{OSDiskGB: 80})
	if err == nil || !strings.Contains(err.Error(), "grow-only") {
		t.Fatalf("shrink not refused: %v", err)
	}
}

func TestResizeRefusesOSDiskGBOnSingleNode(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster}},
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	cfg := config.DefaultConfig()

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	err := r.Resize(t.Context(), ResizeScope{Node: "master0"}, ResizeOptions{OSDiskGB: 100})
	if err == nil {
		t.Fatal("want error resizing --os-disk-gb against a single node")
	}
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("want *errtypes.ConfigError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "role-scoped") || !strings.Contains(err.Error(), "masters") || !strings.Contains(err.Error(), "workers") {
		t.Errorf("error must explain disk resizes are role-scoped and name the role verbs: %v", err)
	}
	if ftf.planCalls != 0 || fc.cordon != 0 {
		t.Errorf("refusal must precede any plan or mutation: planCalls=%d cordon=%d", ftf.planCalls, fc.cordon)
	}
}

// A single-node resize that only touches memory/cpu is unaffected — the
// role-scope refusal is specific to --os-disk-gb.
func TestResizeAllowsSingleNodeWithoutOSDiskGB(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster}},
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	cfg := config.DefaultConfig()

	r, _, _ := seedRunner(t, fc, ftf, cfg) // seedRunner sets DryRun: true
	if err := r.Resize(t.Context(), ResizeScope{Node: "master0"}, ResizeOptions{MemoryMB: 24576}); err != nil {
		t.Fatalf("single-node memory resize refused: %v", err)
	}
}

func TestValidateDiskScope(t *testing.T) {
	cases := []struct {
		name    string
		scope   ResizeScope
		opts    ResizeOptions
		wantErr bool
	}{
		{"role scope with disk grow allowed", ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{OSDiskGB: 100}, false},
		{"single node with disk grow refused", ResizeScope{Node: "master0"}, ResizeOptions{OSDiskGB: 100}, true},
		{"single node without disk grow allowed", ResizeScope{Node: "master0"}, ResizeOptions{MemoryMB: 16384}, false},
		{"single node with disk grow zero allowed", ResizeScope{Node: "master0"}, ResizeOptions{OSDiskGB: 0, MemoryMB: 16384}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDiskScope(tc.scope, tc.opts)
			if tc.wantErr && err == nil {
				t.Fatal("want error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("want nil error, got %v", err)
			}
		})
	}
}

func TestResizeDiskOnlyIsAccepted(t *testing.T) {
	// disk-only must pass the at-least-one gate (mem/cpu both zero)
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster}},
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	cfg := config.DefaultConfig()

	r, _, _ := seedRunner(t, fc, ftf, cfg) // seedRunner sets DryRun: true
	r.Cfg.Topology.ControlPlane.DiskGB = 50
	if err := r.Resize(t.Context(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{OSDiskGB: 100}); err != nil {
		t.Fatalf("disk-only dry-run resize refused: %v", err)
	}
	if got := ftf.lastVars["master_os_disk_size_gb"]; got != "100" {
		t.Fatalf("plan vars missing disk size, got %q", got)
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

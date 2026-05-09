package cli

import (
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestExpandOnlyFlag(t *testing.T) {
	cfg := &config.Config{}
	cfg.Topology.ControlPlane.Count = 3
	cfg.Topology.Workers.Count = 2

	const prefix = "module.okd_cluster.proxmox_virtual_environment_vm."

	tests := []struct {
		only string
		want []string
	}{
		{only: "bootstrap", want: []string{prefix + "bootstrap[0]"}},
		{only: "masters", want: []string{prefix + "master[0]", prefix + "master[1]", prefix + "master[2]"}},
		{only: "workers", want: []string{prefix + "worker[0]", prefix + "worker[1]"}},
		{only: "vms", want: []string{prefix + "bootstrap[0]", prefix + "master[0]", prefix + "master[1]", prefix + "master[2]", prefix + "worker[0]", prefix + "worker[1]"}},
	}

	for _, tc := range tests {
		got, err := expandOnlyFlag(tc.only, cfg)
		if err != nil {
			t.Errorf("--only=%s: unexpected error: %v", tc.only, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("--only=%s: got %d targets, want %d: %v", tc.only, len(got), len(tc.want), got)
			continue
		}
		for i, addr := range got {
			if addr != tc.want[i] {
				t.Errorf("--only=%s [%d]: got %q, want %q", tc.only, i, addr, tc.want[i])
			}
			if err := validateDestroyTargets([]string{addr}, cfg); err != nil {
				t.Errorf("--only=%s [%d]: expanded addr %q fails destroyTargetRE: %v", tc.only, i, addr, err)
			}
		}
	}
}

func TestExpandOnlyFlagInvalid(t *testing.T) {
	cfg := &config.Config{}
	cfg.Topology.ControlPlane.Count = 3
	cfg.Topology.Workers.Count = 2
	_, err := expandOnlyFlag("nodes", cfg)
	if err == nil {
		t.Fatal("--only=nodes: want error, got nil")
	}
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Errorf("--only=nodes: want *errtypes.ConfigError, got %T", err)
	}
}

func TestExpandOnlyFlagZeroCount(t *testing.T) {
	cfg := &config.Config{}
	cfg.Topology.Workers.Count = 0
	_, err := expandOnlyFlag("workers", cfg)
	if err == nil {
		t.Error("--only=workers with zero workers: want error, got nil")
	}
}

func TestValidateDestroyTargets_Valid(t *testing.T) {
	cfg := &config.Config{}
	cfg.Topology.ControlPlane.Count = 3
	cfg.Topology.Workers.Count = 3
	valid := []string{
		"module.okd_cluster.proxmox_virtual_environment_vm.bootstrap",
		"module.okd_cluster.proxmox_virtual_environment_vm.master",
		"module.okd_cluster.proxmox_virtual_environment_vm.worker",
		"module.okd_cluster.proxmox_virtual_environment_vm.master[0]",
		"module.okd_cluster.proxmox_virtual_environment_vm.worker[2]",
		"module.okd_cluster.proxmox_virtual_environment_vm.bootstrap[0]",
	}
	for _, addr := range valid {
		if err := validateDestroyTargets([]string{addr}, cfg); err != nil {
			t.Errorf("addr %q: want nil, got %v", addr, err)
		}
	}
}

func TestValidateDestroyTargets_Invalid(t *testing.T) {
	invalid := []string{
		"module.okd_cluster.proxmox_virtual_environment_vm.control_plane",
		"proxmox_virtual_environment_vm.bootstrap",
		"module.okd_cluster.proxmox_virtual_environment_vm.master[-1]",
		"module.okd_cluster.proxmox_virtual_environment_vm.MASTER",
		"",
		"module.okd_cluster.*",
		"module.okd_cluster.proxmox_virtual_environment_vm.worker[abc]",
	}
	cfg := &config.Config{}
	cfg.Topology.ControlPlane.Count = 3
	cfg.Topology.Workers.Count = 2
	for _, addr := range invalid {
		err := validateDestroyTargets([]string{addr}, cfg)
		if err == nil {
			t.Errorf("addr %q: want error, got nil", addr)
			continue
		}
		var cfgErr *errtypes.ConfigError
		if !errors.As(err, &cfgErr) {
			t.Errorf("addr %q: want *errtypes.ConfigError, got %T: %v", addr, err, err)
		}
	}
}

func TestValidateDestroyTargets_Empty(t *testing.T) {
	cfg := &config.Config{}
	cfg.Topology.ControlPlane.Count = 3
	cfg.Topology.Workers.Count = 2
	if err := validateDestroyTargets(nil, cfg); err != nil {
		t.Errorf("nil targets: want nil, got %v", err)
	}
	if err := validateDestroyTargets([]string{}, cfg); err != nil {
		t.Errorf("empty targets: want nil, got %v", err)
	}
}

func TestValidateDestroyTargets_Bounds(t *testing.T) {
	cfg := &config.Config{}
	cfg.Topology.ControlPlane.Count = 3
	cfg.Topology.Workers.Count = 2

	oob := []string{
		"module.okd_cluster.proxmox_virtual_environment_vm.master[3]",
		"module.okd_cluster.proxmox_virtual_environment_vm.master[7]",
		"module.okd_cluster.proxmox_virtual_environment_vm.worker[2]",
		"module.okd_cluster.proxmox_virtual_environment_vm.worker[9]",
		"module.okd_cluster.proxmox_virtual_environment_vm.bootstrap[1]",
	}
	for _, addr := range oob {
		err := validateDestroyTargets([]string{addr}, cfg)
		if err == nil {
			t.Errorf("addr %q: want error, got nil", addr)
			continue
		}
		var usageErr *errtypes.UsageError
		if !errors.As(err, &usageErr) {
			t.Errorf("addr %q: want *errtypes.UsageError, got %T: %v", addr, err, err)
		}
	}

	inRange := []string{
		"module.okd_cluster.proxmox_virtual_environment_vm.master[0]",
		"module.okd_cluster.proxmox_virtual_environment_vm.master[2]",
		"module.okd_cluster.proxmox_virtual_environment_vm.worker[0]",
		"module.okd_cluster.proxmox_virtual_environment_vm.worker[1]",
		"module.okd_cluster.proxmox_virtual_environment_vm.bootstrap[0]",
	}
	for _, addr := range inRange {
		if err := validateDestroyTargets([]string{addr}, cfg); err != nil {
			t.Errorf("addr %q (in range): want nil, got %v", addr, err)
		}
	}
}

// requireConfirmClusterWithTarget mirrors the runtime guard inside runDestroy
// so the rule is independently testable without spinning up a full cobra +
// loadConfig context.
func requireConfirmClusterWithTarget(targets []string, confirmCluster, clusterName string) error {
	if len(targets) > 0 && confirmCluster == "" {
		return &errtypes.ConfigError{
			Msg: "--target requires --confirm-cluster=" + clusterName,
		}
	}
	return nil
}

func TestRequireConfirmClusterWithTarget(t *testing.T) {
	target := []string{"module.okd_cluster.proxmox_virtual_environment_vm.master[0]"}

	if err := requireConfirmClusterWithTarget(target, "", "prod"); err == nil {
		t.Error("target without confirm-cluster: want error, got nil")
	}
	if err := requireConfirmClusterWithTarget(target, "prod", "prod"); err != nil {
		t.Errorf("target with confirm-cluster: want nil, got %v", err)
	}
	if err := requireConfirmClusterWithTarget(nil, "", "prod"); err != nil {
		t.Errorf("no target no confirm-cluster: want nil, got %v", err)
	}
}

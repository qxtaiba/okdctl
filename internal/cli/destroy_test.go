package cli

import (
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestValidateDestroyTargets_Valid(t *testing.T) {
	valid := []string{
		"module.okd_cluster.proxmox_virtual_environment_vm.bootstrap",
		"module.okd_cluster.proxmox_virtual_environment_vm.master",
		"module.okd_cluster.proxmox_virtual_environment_vm.worker",
		"module.okd_cluster.proxmox_virtual_environment_vm.master[0]",
		"module.okd_cluster.proxmox_virtual_environment_vm.worker[2]",
		"module.okd_cluster.proxmox_virtual_environment_vm.bootstrap[0]",
	}
	for _, addr := range valid {
		if err := validateDestroyTargets([]string{addr}); err != nil {
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
	for _, addr := range invalid {
		err := validateDestroyTargets([]string{addr})
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
	if err := validateDestroyTargets(nil); err != nil {
		t.Errorf("nil targets: want nil, got %v", err)
	}
	if err := validateDestroyTargets([]string{}); err != nil {
		t.Errorf("empty targets: want nil, got %v", err)
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

package cli

import (
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

const vmAddrPrefix = "module.okd_cluster.proxmox_virtual_environment_vm."

func TestExpandOnlyFlag(t *testing.T) {
	cfg := &config.Config{}
	cfg.Topology.ControlPlane.Count = 3
	cfg.Topology.Workers.Count = 2

	tests := []struct {
		only string
		want []string
	}{
		{only: "bootstrap", want: []string{vmAddrPrefix + "bootstrap[0]"}},
		{only: "masters", want: []string{vmAddrPrefix + "master[0]", vmAddrPrefix + "master[1]", vmAddrPrefix + "master[2]"}},
		{only: "workers", want: []string{vmAddrPrefix + "worker[0]", vmAddrPrefix + "worker[1]"}},
		{only: "vms", want: []string{vmAddrPrefix + "bootstrap[0]", vmAddrPrefix + "master[0]", vmAddrPrefix + "master[1]", vmAddrPrefix + "master[2]", vmAddrPrefix + "worker[0]", vmAddrPrefix + "worker[1]"}},
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
	var usageErr *errtypes.UsageError
	if !errors.As(err, &usageErr) {
		t.Errorf("--only=nodes: want *errtypes.UsageError, got %T", err)
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

func TestValidateDestroyTargets(t *testing.T) {
	cases := []struct {
		name             string
		targets          []string
		masters, workers int
		wantErr          bool
	}{
		{"valid bootstrap unindexed", []string{vmAddrPrefix + "bootstrap"}, 3, 3, false},
		{"valid master unindexed", []string{vmAddrPrefix + "master"}, 3, 3, false},
		{"valid worker unindexed", []string{vmAddrPrefix + "worker"}, 3, 3, false},
		{"valid worker[2]", []string{vmAddrPrefix + "worker[2]"}, 3, 3, false},

		{"invalid role control_plane", []string{vmAddrPrefix + "control_plane"}, 3, 2, true},
		{"invalid missing module prefix", []string{"proxmox_virtual_environment_vm.bootstrap"}, 3, 2, true},
		{"invalid negative index", []string{vmAddrPrefix + "master[-1]"}, 3, 2, true},
		{"invalid uppercase role", []string{vmAddrPrefix + "MASTER"}, 3, 2, true},
		{"invalid empty address", []string{""}, 3, 2, true},
		{"invalid wildcard", []string{"module.okd_cluster.*"}, 3, 2, true},
		{"invalid non-numeric index", []string{vmAddrPrefix + "worker[abc]"}, 3, 2, true},

		{"nil targets", nil, 3, 2, false},

		{"oob master[3]", []string{vmAddrPrefix + "master[3]"}, 3, 2, true},
		{"oob worker[2]", []string{vmAddrPrefix + "worker[2]"}, 3, 2, true},
		{"oob bootstrap[1]", []string{vmAddrPrefix + "bootstrap[1]"}, 3, 2, true},

		{"in-range master[0]", []string{vmAddrPrefix + "master[0]"}, 3, 2, false},
		{"in-range master[2]", []string{vmAddrPrefix + "master[2]"}, 3, 2, false},
		{"in-range worker[0]", []string{vmAddrPrefix + "worker[0]"}, 3, 2, false},
		{"in-range worker[1]", []string{vmAddrPrefix + "worker[1]"}, 3, 2, false},
		{"in-range bootstrap[0]", []string{vmAddrPrefix + "bootstrap[0]"}, 3, 2, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Topology.ControlPlane.Count = tc.masters
			cfg.Topology.Workers.Count = tc.workers

			err := validateDestroyTargets(tc.targets, cfg)
			if !tc.wantErr {
				if err != nil {
					t.Errorf("targets %v: want nil, got %v", tc.targets, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("targets %v: want error, got nil", tc.targets)
			}
			var usageErr *errtypes.UsageError
			if !errors.As(err, &usageErr) {
				t.Errorf("targets %v: want *errtypes.UsageError, got %T: %v", tc.targets, err, err)
			}
		})
	}
}

package config

import "time"

// DefaultConfig returns a 3+2 cluster on 192.168.1.0/24 placeholders.
func DefaultConfig() *Config {
	return &Config{
		SchemaVersion: SchemaVersionCurrent,
		Cluster:       ClusterConfig{Name: "lab", Domain: "k8s.local"},
		Distribution:  DistributionConfig{Type: DistributionOKD, Version: DefaultOKDVersion},
		Provider: ProviderConfig{
			Type: ProviderProxmox,
			Proxmox: ProxmoxConfig{
				Endpoint:   "https://192.168.1.100:8006",
				Nodes:      []string{"pve"},
				Storage:    "local-lvm",
				ISOStorage: "local",
				Bridge:     "vmbr0",
				CPUType:    "host",
				VMIDBase:   DefaultVMIDBase,
			},
		},
		Networking: NetworkingConfig{
			MachineCIDR: "192.168.1.0/24",
			Gateway:     "192.168.1.1",
			DNS:         []string{"192.168.1.1"},
			APIVIP:      "192.168.1.158",
			IngressVIP:  "192.168.1.159",
			NodeIPStart: "192.168.1.150",
			PodCIDR:     "10.128.0.0/14",
			ServiceCIDR: "172.30.0.0/16",
			HostPrefix:  23,
		},
		ControlPlane: NodeDefaults{CPU: 4, MemoryMiB: 16384, Disks: []DiskSpec{{SizeGiB: 120}}},
		Workers: WorkersConfig{
			Count: 2,
			NodeDefaults: NodeDefaults{
				CPU:       8,
				MemoryMiB: 16384,
				Disks:     []DiskSpec{{SizeGiB: 120}, {SizeGiB: 500, Storage: "local-lvm", Wipe: new(true)}},
			},
		},
		Files: FilesConfig{PullSecret: "./pull-secret.json", SSHPublicKey: "~/.ssh/id_ed25519.pub"}, //nolint:gosec // G101: file path, not a credential
		Lifecycle: LifecycleConfig{
			BeforeNextNode: Hook{Timeout: 15 * time.Minute},
			DrainTimeout:   10 * time.Minute,
		},
		Handoff:  HandoffConfig{Enabled: true},
		Defaults: DefaultsConfig{DisableRedHatCatalogs: true},
	}
}

// MinimalConfig returns a 3+0 cluster whose control plane runs the workloads.
func MinimalConfig() *Config {
	cfg := DefaultConfig()
	cfg.Cluster.Name = "minimal"
	cfg.Workers = WorkersConfig{
		Count:        0,
		NodeDefaults: NodeDefaults{CPU: 2, MemoryMiB: 8192, Disks: []DiskSpec{{SizeGiB: 100}}},
	}
	return cfg
}

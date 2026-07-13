package config

import "github.com/qxtaiba/okdctl/internal/netutil"

const (
	defaultBastionIP = "192.168.1.20"
	// defaultStaticIPStart is deliberately distant from the default
	// provider.proxmox.host (192.168.1.100): the bootstrap VM boots at
	// this address and must not ARP-fight the hypervisor.
	defaultStaticIPStart = "192.168.1.140"
	defaultFluxAddon     = "flux"
)

// DefaultConfig returns a Config with defaults for a typical homelab environment.
func DefaultConfig() *Config {
	return &Config{
		SchemaVersion: SchemaVersionCurrent,
		Cluster: ClusterConfig{
			Name:   "mycluster",
			Domain: "k8s.local",
		},
		Distribution: DistributionConfig{
			Type:    DistributionOKD,
			Version: "4.22.0-okd-scos.7",
		},
		Provider: ProviderConfig{
			Type: ProviderProxmox,
			Proxmox: &ProxmoxConfig{
				Host:        "192.168.1.100:8006",
				Node:        "pve",
				Storage:     "local-lvm",
				DataStorage: "local-lvm",
				ISOStorage:  "local",
				Bridge:      "vmbr0",
				Insecure:    false,
				CPUType:     "host",
			},
		},
		Topology: TopologyConfig{
			ControlPlane: NodeConfig{
				Count:    3,
				CPU:      4,
				MemoryMB: 12288,
				DiskGB:   50,
			},
			Workers: NodeConfig{
				Count:    3,
				CPU:      8,
				MemoryMB: 20480,
				DiskGB:   50,
			},
			Bootstrap: NodeConfig{
				Count:    1,
				CPU:      4,
				MemoryMB: 8192,
				DiskGB:   50,
			},
			VMIDBase: DefaultVMIDBase,
		},
		Networking: NetworkingConfig{
			MachineCIDR: "192.168.1.0/24",
			PodCIDR:     "10.128.0.0/14",
			ServiceCIDR: "172.30.0.0/16",
			HostPrefix:  23,
			Gateway:     "192.168.1.1",
			DNS:         []string{"192.168.1.1"},
			StaticIP: StaticIPConfig{
				Start:     defaultStaticIPStart,
				Netmask:   netutil.DefaultNetmask,
				Interface: "ens18",
				DNS:       defaultBastionIP,
			},
			Bastion: BastionConfig{
				IP: defaultBastionIP,
			},
		},
		Addons: map[string]AddonConfig{
			defaultFluxAddon: {Enabled: false, Settings: map[string]string{
				"provider": defaultFluxAddon, "branch": "main", "path": "kubernetes/clusters/production",
			}},
			"secretstore": {Enabled: false, Settings: map[string]string{ //nolint:gosec // G101: addon name, not a credential
				"secrets_dir":              "automation/config/secrets",
				"provider":                 "onepassword",
				"onepassword_vaults":       "homelab=1",
				"vault_path":               "secret",
				"vault_version":            "v2",
				"bitwarden_api_url":        "https://api.bitwarden.com",
				"bitwarden_identity_url":   "https://identity.bitwarden.com",
				"bitwarden_sdk_server_url": "https://bitwarden-sdk-server.external-secrets.svc.cluster.local:9998",
			}},
		},
		Files: FilesConfig{
			PullSecret:   "",
			SSHPublicKey: "",
		},
		HTTPServer: HTTPServerConfig{
			Port:             443,
			Root:             "/var/www/html",
			IgnitionServerIP: defaultBastionIP,
		},
		Deployment: DeploymentConfig{
			TerraformEnv:     "production",
			AutoApprove:      false,
			BootstrapTimeout: 3600,
			InstallTimeout:   7200,
		},
		Disks: DisksConfig{
			WorkerDataSizeGB:       500,
			ControlPlaneDataSizeGB: 0,
		},
	}
}

// MinimalConfig returns a single-node configuration for testing.
func MinimalConfig() *Config {
	cfg := DefaultConfig()
	cfg.Cluster.Name = "minimal"
	cfg.Topology = TopologyConfig{
		ControlPlane: NodeConfig{Count: 1, CPU: 4, MemoryMB: 8192, DiskGB: 50},
		Workers:      NodeConfig{Count: 0, CPU: 0, MemoryMB: 0, DiskGB: 0},
	}
	cfg.Addons = map[string]AddonConfig{
		defaultFluxAddon: {Enabled: false},
	}
	return cfg
}

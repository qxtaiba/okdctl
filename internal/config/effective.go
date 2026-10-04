package config

// Effective resolves inherited resources and derived networking without changing cfg.
func Effective(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	resolved := *cfg
	topology := &resolved.Topology
	if topology.ControlPlane.DiskGB == 0 {
		topology.ControlPlane.DiskGB = DefaultOSDiskGB
	}
	if topology.Workers.DiskGB == 0 {
		topology.Workers.DiskGB = topology.ControlPlane.DiskGB
	}
	if topology.Bootstrap.Count == 0 {
		topology.Bootstrap.Count = 1
	}
	if topology.Bootstrap.CPU == 0 {
		topology.Bootstrap.CPU = topology.ControlPlane.CPU
	}
	if topology.Bootstrap.MemoryMB == 0 {
		topology.Bootstrap.MemoryMB = topology.ControlPlane.MemoryMB
	}
	if topology.Bootstrap.DiskGB == 0 {
		topology.Bootstrap.DiskGB = topology.ControlPlane.DiskGB
	}
	deriveStaticNetmask(&resolved)
	return &resolved
}

func fileDefaults() *Config {
	cfg := DefaultConfig()
	cfg.Topology.Bootstrap = NodeConfig{}
	cfg.Disks = DisksConfig{}
	cfg.Topology.Workers.DiskGB = 0
	return cfg
}

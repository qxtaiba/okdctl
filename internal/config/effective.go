package config

import "github.com/qxtaiba/okdctl/internal/netutil"

// Effective resolves inherited resources and derived networking without
// changing cfg; it is the single point-of-use resolver — callers that load
// or mutate a Config (wizard steps, node add/resize, terraform rendering)
// must call Effective rather than materializing these fields themselves, or
// a save freezes a stale value (see NormalizeTopology).
func Effective(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	resolved := *cfg
	resolved.Topology = NormalizeTopology(&cfg.Topology)
	_ = DeriveStaticNetmask(&resolved) // invalid/IPv6 CIDR is left for validators here
	return &resolved
}

// NormalizeTopology fills omitted bootstrap/worker fields from control-plane
// values, returning a new TopologyConfig; it never mutates *t. This is the
// one place the bootstrap-mirrors-control-plane invariant is computed —
// callers must not reimplement it, and must not write its result back into
// a Config that gets saved, or an operator's later edit to control-plane
// desyncs from a now-frozen bootstrap value.
func NormalizeTopology(t *TopologyConfig) TopologyConfig {
	result := *t
	if result.ControlPlane.DiskGB == 0 {
		result.ControlPlane.DiskGB = DefaultOSDiskGB
	}
	if result.Workers.DiskGB == 0 {
		result.Workers.DiskGB = result.ControlPlane.DiskGB
	}
	if result.Bootstrap.Count == 0 {
		result.Bootstrap.Count = 1
	}
	if result.Bootstrap.CPU == 0 {
		result.Bootstrap.CPU = result.ControlPlane.CPU
	}
	if result.Bootstrap.MemoryMB == 0 {
		result.Bootstrap.MemoryMB = result.ControlPlane.MemoryMB
	}
	if result.Bootstrap.DiskGB == 0 {
		result.Bootstrap.DiskGB = result.ControlPlane.DiskGB
	}
	return result
}

// DeriveStaticNetmask overwrites StaticIP.Netmask with MachineCIDR's dotted
// form so hand-edits can't desync them, returning CIDRToNetmask's error (if
// any) so a caller that can still reject early (e.g. the networking wizard
// step's Apply) may do so; a caller that must leave it for validators (e.g.
// the loader) should discard the error. Unlike NormalizeTopology this always
// overwrites rather than filling only when zero, so it self-heals on every
// call and is safe to run at load time, point of use, or both.
func DeriveStaticNetmask(cfg *Config) error {
	netmask, err := netutil.CIDRToNetmask(cfg.Networking.MachineCIDR)
	if err != nil {
		return err
	}
	cfg.Networking.StaticIP.Netmask = netmask
	return nil
}

func fileDefaults() *Config {
	cfg := DefaultConfig()
	cfg.Topology.Bootstrap = NodeConfig{}
	cfg.Disks = DisksConfig{}
	cfg.Topology.Workers.DiskGB = 0
	return cfg
}

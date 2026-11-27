package model

import (
	"regexp"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

func testInputs(t *testing.T, cfg *config.Config) Inputs {
	t.Helper()
	return Inputs{
		Inventory:  mustBuild(t, cfg),
		Day1:       []string{"lab-master0", "lab-master1", "lab-master2", "lab-worker0", "lab-worker1"},
		Domain:     cfg.Cluster.Domain,
		Network:    cfg.Networking,
		VLAN:       cfg.Provider.Proxmox.VLAN,
		Release:    "4.20.0",
		PullSecret: []byte(`{"auths":{}}`),
		SSHKey:     []byte("ssh-ed25519 AAAA"),
		ExtraManifests: map[string][]byte{
			"99-b.yaml": []byte("b: 1\n"),
			"98-a.yaml": []byte("a: 1\n"),
		},
	}
}

func TestInputsHash_Stable(t *testing.T) {
	first, second := testInputs(t, testConfig(2)), testInputs(t, testConfig(2))
	a, b := first.Hash(), second.Hash()
	if a != b {
		t.Fatalf("Hash differs across builds: %s vs %s", a, b)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(a) {
		t.Errorf("Hash = %q; want sha256 hex", a)
	}
}

func TestInputsHash_Sensitivity(t *testing.T) {
	baseInputs := testInputs(t, testConfig(2))
	base := baseInputs.Hash()
	for _, tc := range []struct {
		name   string
		cfg    func(*config.Config)
		in     func(*Inputs)
		change bool
	}{
		{name: "infra id", change: true, in: func(in *Inputs) { in.Inventory.InfraID = "lab-000000" }},
		{name: "master address", change: true, cfg: func(c *config.Config) {
			c.Nodes = map[string]config.NodeOverride{"lab-master0": {IP: "192.168.1.200"}}
		}},
		{name: "api vip", change: true, cfg: func(c *config.Config) { c.Networking.APIVIP = "192.168.1.157" }},
		{name: "ingress vip", change: true, cfg: func(c *config.Config) { c.Networking.IngressVIP = "192.168.1.157" }},
		{name: "release", change: true, in: func(in *Inputs) { in.Release = "4.21.0" }},
		{name: "domain", change: true, cfg: func(c *config.Config) { c.Cluster.Domain = "other.local" }},
		{name: "machine cidr", change: true, cfg: func(c *config.Config) { c.Networking.MachineCIDR = "192.168.0.0/16" }},
		{name: "gateway", change: true, cfg: func(c *config.Config) { c.Networking.Gateway = "192.168.1.2" }},
		{name: "dns", change: true, cfg: func(c *config.Config) { c.Networking.DNS = []string{"192.168.1.1", "1.1.1.1"} }},
		{name: "ntp", change: true, cfg: func(c *config.Config) { c.Networking.NTP = []string{"pool.ntp.org"} }},
		{name: "vlan", change: true, cfg: func(c *config.Config) { c.Provider.Proxmox.VLAN = 12 }},
		{name: "pod cidr", change: true, cfg: func(c *config.Config) { c.Networking.PodCIDR = "10.132.0.0/14" }},
		{name: "service cidr", change: true, cfg: func(c *config.Config) { c.Networking.ServiceCIDR = "172.31.0.0/16" }},
		{name: "host prefix", change: true, cfg: func(c *config.Config) { c.Networking.HostPrefix = 24 }},
		{name: "manifest byte", change: true, in: func(in *Inputs) { in.ExtraManifests["98-a.yaml"] = []byte("a: 2\n") }},
		{name: "pull secret", change: true, in: func(in *Inputs) { in.PullSecret = []byte(`{"auths":{"x":{}}}`) }},
		{name: "ssh key", change: true, in: func(in *Inputs) { in.SSHKey = []byte("ssh-ed25519 BBBB") }},
		{name: "worker address", change: true, cfg: func(c *config.Config) {
			c.Nodes = map[string]config.NodeOverride{"lab-worker0": {IP: "192.168.1.200"}}
		}},
		{name: "worker added after day 1", cfg: func(c *config.Config) { c.Workers.Count = 3 }},
		{name: "day-1 worker removed", change: true, cfg: func(c *config.Config) { c.Workers.Count = 1 }},
		{name: "later worker address", cfg: func(c *config.Config) {
			c.Workers.Count = 3
			c.Nodes = map[string]config.NodeOverride{"lab-worker2": {IP: "192.168.1.200"}}
		}},
		{name: "memory", cfg: func(c *config.Config) { c.Workers.MemoryMiB = 65536 }},
		{name: "cpu", cfg: func(c *config.Config) { c.ControlPlane.CPU = 2 }},
		{name: "host", cfg: func(c *config.Config) {
			c.Nodes = map[string]config.NodeOverride{"lab-worker0": {Host: "pve9"}}
		}},
		{name: "disks", cfg: func(c *config.Config) {
			c.Nodes = map[string]config.NodeOverride{"lab-worker0": {Disks: []config.DiskSpec{{SizeGiB: 999}}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig(2)
			if tc.cfg != nil {
				tc.cfg(cfg)
			}
			in := testInputs(t, cfg)
			if tc.in != nil {
				tc.in(&in)
			}
			if changed := in.Hash() != base; changed != tc.change {
				t.Errorf("hash changed = %v; want %v", changed, tc.change)
			}
		})
	}
}

func TestInputsHash_Golden(t *testing.T) {
	in := testInputs(t, testConfig(2))
	const want = "8d39b6c0f32b560492d7c1337e206ccce09199752f14fff3310ec4bb6989cb1f"
	if got := in.Hash(); got != want {
		t.Errorf("Hash = %s; want %s (a change here stales every record: bump the record schema version)", got, want)
	}
}

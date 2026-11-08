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
		{name: "worker address", change: true, cfg: func(c *config.Config) {
			c.Nodes = map[string]config.NodeOverride{"lab-worker0": {IP: "192.168.1.200"}}
		}},
		{name: "api vip", change: true, cfg: func(c *config.Config) { c.Networking.APIVIP = "192.168.1.157" }},
		{name: "ingress vip", change: true, cfg: func(c *config.Config) { c.Networking.IngressVIP = "192.168.1.157" }},
		{name: "release", change: true, in: func(in *Inputs) { in.Release = "4.21.0" }},
		{name: "manifest byte", change: true, in: func(in *Inputs) { in.ExtraManifests["98-a.yaml"] = []byte("a: 2\n") }},
		{name: "pull secret", change: true, in: func(in *Inputs) { in.PullSecret = []byte(`{"auths":{"x":{}}}`) }},
		{name: "ssh key", change: true, in: func(in *Inputs) { in.SSHKey = []byte("ssh-ed25519 BBBB") }},
		{name: "worker count", change: true, cfg: func(c *config.Config) { c.Workers.Count = 3 }},
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

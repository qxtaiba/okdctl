package config

import "testing"

func TestValidateCluster(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
		want   finding
	}{
		{"name required", func(c *Config) { c.Cluster.Name = "" }, finding{FieldClusterName, "required"}},
		{"name too long", func(c *Config) { c.Cluster.Name = "a123456789b123456789c123456789d123456789e123456789f1234" }, finding{FieldClusterName, "at most 54 characters"}},
		{"name with dot", func(c *Config) { c.Cluster.Name = "lab.prod" }, finding{FieldClusterName, "lowercase letters, digits and hyphens"}},
		{"name uppercase", func(c *Config) { c.Cluster.Name = "Lab" }, finding{FieldClusterName, "lowercase letters, digits and hyphens"}},
		{"domain required", func(c *Config) { c.Cluster.Domain = "" }, finding{FieldClusterDomain, "required"}},
		{"domain invalid", func(c *Config) { c.Cluster.Domain = "k8s_.local" }, finding{FieldClusterDomain, "not a valid DNS name"}},
		{"distribution type", func(c *Config) { c.Distribution.Type = "openshift" }, finding{FieldDistributionType, "must be okd"}},
		{"version unparseable", func(c *Config) { c.Distribution.Version = "4.22" }, finding{FieldDistributionVersion, "4.22.0-okd-scos.10"}},
		{"version below floor", func(c *Config) { c.Distribution.Version = "4.22.0-okd-scos.8" }, finding{FieldDistributionVersion, "below the floor " + MinOKDVersion}},
		{"version older minor", func(c *Config) { c.Distribution.Version = "4.19.0-okd-scos.15" }, finding{FieldDistributionVersion, "below the floor " + MinOKDVersion}},
		{"pull secret required", func(c *Config) { c.Files.PullSecret = "" }, finding{FieldFilesPullSecret, "required"}},
		{"ssh key required", func(c *Config) { c.Files.SSHPublicKey = "" }, finding{FieldFilesSSHPublicKey, "required"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.mutate(cfg)
			assertFinding(t, cfg, tc.want)
		})
	}

	passing := map[string]func(*Config){
		"floor version":   func(c *Config) { c.Distribution.Version = MinOKDVersion },
		"newer minor":     func(c *Config) { c.Distribution.Version = "4.23.0-okd-scos.1" },
		"54 char name":    func(c *Config) { c.Cluster.Name = "a123456789b123456789c123456789d123456789e123456789f123" },
		"hyphenated name": func(c *Config) { c.Cluster.Name = "my-lab-1" },
		"deep domain":     func(c *Config) { c.Cluster.Domain = "okd.lab.example.com" },
	}
	for name, mutate := range passing {
		t.Run("accepts "+name, func(t *testing.T) {
			cfg := DefaultConfig()
			mutate(cfg)
			assertValid(t, cfg)
		})
	}
}

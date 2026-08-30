package config

import (
	"strings"
	"testing"
)

func hasFieldError(r *ValidationResult, field string) bool {
	for _, e := range r.Errors {
		if e.Field == field {
			return true
		}
	}
	return false
}

func checkAcceptReject(t *testing.T, fn func(string) error, accept, reject []string) {
	t.Helper()
	for _, s := range accept {
		if err := fn(s); err != nil {
			t.Errorf("%q rejected: %v", s, err)
		}
	}
	for _, s := range reject {
		if err := fn(s); err == nil {
			t.Errorf("%q accepted; want error", s)
		}
	}
}

func checkBoolValidator(t *testing.T, fn func(string) bool, good, bad []string) {
	t.Helper()
	for _, s := range good {
		if !fn(s) {
			t.Errorf("%q = false; want true", s)
		}
	}
	for _, s := range bad {
		if fn(s) {
			t.Errorf("%q = true; want false", s)
		}
	}
}

func TestIsValidNetmask(t *testing.T) {
	checkBoolValidator(t, isValidNetmask,
		[]string{
			"/24", "/32", "/0",
			"255.255.255.0",
			"255.255.255.255",
			"128.0.0.0",
			"255.255.255.254",
		},
		[]string{
			"",
			"/33",
			"/-1",
			"0.0.0.0", // canonical but disallowed (would claim whole space)
			"255.255.255.1",
			"255.0.255.0",
			"fe80::/10",
			"not-an-address",
		})
}

func TestValidateProxmoxHost(t *testing.T) {
	checkAcceptReject(t, ValidateProxmoxHost,
		[]string{
			"pve.example.com",
			"pve.example.com:8006",
			"10.0.0.1",
			"10.0.0.1:22",
			"proxmox",
			"[2001:db8::1]:8006",
		},
		[]string{
			"",
			":8006",
			"!bad!.example",
			"space in host",
			"https://pve.example.com",
			"gopher://pve.example.com",
			"file:///etc/passwd",
			"user:pass@pve.example.com",
			strings.Repeat("a", 254), // over the 253-char domain cap
			"h\x00st",                // embedded NUL
		})
}

func TestValidateHAMasters(t *testing.T) {
	cases := []struct {
		count   int
		wantErr bool
	}{
		{0, false},
		{1, false},
		{2, true},
		{3, false},
	}
	for _, tc := range cases {
		err := validateHAMasters(tc.count)
		if tc.wantErr && err == nil {
			t.Errorf("validateHAMasters(%d) accepted; want rejection", tc.count)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("validateHAMasters(%d) error: %v", tc.count, err)
		}
	}
}

func TestValidateTerraformEnv(t *testing.T) {
	checkAcceptReject(t, ValidateTerraformEnv,
		[]string{
			"",
			"production",
			"staging",
			"dev1",
			"_private",
			"A",
			"Env-Name_01",
		},
		[]string{
			"1starts-with-digit",
			"-starts-with-dash",
			"has space",
			"has/slash",
			"has..dots",
			"../escape",
			"/absolute",
			"env\x00null",
			"unicodé",
			"env.tf",
		})
}

func TestValidateProxmoxConfigFields(t *testing.T) {
	newCfg := func() *ProxmoxConfig {
		return &ProxmoxConfig{Host: "pve:8006", Node: "pve", Storage: "local-lvm"}
	}
	check := func(t *testing.T, set func(*ProxmoxConfig, string), value string, fields []string, wantErr bool) {
		t.Helper()
		cfg := newCfg()
		set(cfg, value)
		r := &ValidationResult{}
		validateProxmoxConfig(cfg, r)
		for _, f := range fields {
			if got := hasFieldError(r, f); got != wantErr {
				if wantErr {
					t.Errorf("%s %q accepted; want rejection", f, value)
				} else {
					t.Errorf("%s %q rejected", f, value)
				}
			}
		}
	}

	cases := []struct {
		name   string
		set    func(*ProxmoxConfig, string)
		fields []string
		good   []string
		bad    []string
	}{
		{
			name:   "storage names",
			set:    func(p *ProxmoxConfig, s string) { p.ISOStorage = s; p.DataStorage = s },
			fields: []string{FieldProxmoxISOStorage, FieldProxmoxDataStorage},
			good:   []string{"local", "local-lvm", "ceph-pool", "storage1", "Tank", "has.dot"},
			bad:    []string{`local"inject`, "has space", "has/slash", ".leading-dot"},
		},
		{
			name:   "cpu type",
			set:    func(p *ProxmoxConfig, s string) { p.CPUType = s },
			fields: []string{FieldProxmoxCPUType},
			good:   []string{"host", "kvm64", "x86-64-v2", "x86-64-v2-AES", "Skylake-Server-noTSX-IBRS", "x86-64-v2+pge", "x86-64-v2,flags=+pge"},
			bad:    []string{`host"inject`, "has space", "has\nnewline", `"; rm -rf /`},
		},
		{
			name:   "bridge",
			set:    func(p *ProxmoxConfig, s string) { p.Bridge = s },
			fields: []string{FieldProxmoxBridge},
			good:   []string{"vmbr0", "vmbr1", "vmbr100", "eth0"},
			bad:    []string{`vmbr0"inject`, "has space", "0starts-digit"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, s := range tc.good {
				check(t, tc.set, s, tc.fields, false)
			}
			for _, s := range tc.bad {
				check(t, tc.set, s, tc.fields, true)
			}
		})
	}

	emptyResult := &ValidationResult{}
	validateProxmoxConfig(newCfg(), emptyResult)
	for _, e := range emptyResult.Errors {
		switch e.Field {
		case FieldProxmoxISOStorage, FieldProxmoxDataStorage, FieldProxmoxBridge, FieldProxmoxCPUType:
			t.Errorf("empty optional field %s rejected: %s", e.Field, e.Message)
		}
	}
}

func TestValidatePlacementCounts(t *testing.T) {
	cases := []struct {
		name    string
		cpNodes []string
		wNodes  []string
		cpCount int
		wCount  int
		wantErr []string
	}{
		{name: "empty lists valid", cpCount: 3, wCount: 2},
		{name: "shorter lists pad", cpNodes: []string{"pve1"}, wNodes: []string{"pve2"}, cpCount: 3, wCount: 2},
		{name: "exact lengths valid", cpNodes: []string{"pve1", "pve2", "pve3"}, wNodes: []string{"pve1", "pve2"}, cpCount: 3, wCount: 2},
		{
			name:    "longer lists rejected",
			cpNodes: []string{"pve1", "pve2", "pve3", "pve4"},
			wNodes:  []string{"pve1", "pve2", "pve3"},
			cpCount: 3, wCount: 2,
			wantErr: []string{FieldProxmoxControlPlaneNodes, FieldProxmoxWorkerNodes},
		},
		{
			name:    "zero workers with placement rejected",
			wNodes:  []string{"pve1"},
			cpCount: 1, wCount: 0,
			wantErr: []string{FieldProxmoxWorkerNodes},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Provider: ProviderConfig{
					Type:    ProviderProxmox,
					Proxmox: &ProxmoxConfig{ControlPlaneNodes: tc.cpNodes, WorkerNodes: tc.wNodes},
				},
				Topology: TopologyConfig{
					ControlPlane: NodeConfig{Count: tc.cpCount},
					Workers:      NodeConfig{Count: tc.wCount},
				},
			}
			r := &ValidationResult{}
			validatePlacementCounts(cfg, r)
			if len(r.Errors) != len(tc.wantErr) {
				t.Fatalf("got %d errors (%v); want %d", len(r.Errors), r.Errors, len(tc.wantErr))
			}
			for i, field := range tc.wantErr {
				if r.Errors[i].Field != field {
					t.Errorf("Errors[%d].Field = %q; want %q", i, r.Errors[i].Field, field)
				}
			}
		})
	}
}

func TestIsValidDNSLabel(t *testing.T) {
	checkBoolValidator(t, IsValidDNSLabel,
		[]string{
			"a",
			"abc",
			"a-b",
			"abc-def",
			"a0",
			"0abc",
			strings.Repeat("a", 63),
		},
		[]string{
			"",
			"-abc",
			"abc-",
			"A-UPPER",
			`a"b`,
			"../etc",
			strings.Repeat("a", 64),
		})
}

func TestValidateClusterName(t *testing.T) {
	checkAcceptReject(t, ValidateClusterName,
		[]string{"ab", "my-cluster", "0abc", strings.Repeat("a", 63)},
		[]string{"", "a", "-abc", "ABC", `a"b`, "../etc", strings.Repeat("a", 64)})
}

func TestValidateCIDR(t *testing.T) {
	checkAcceptReject(t, ValidateCIDR,
		[]string{
			"192.168.1.0/24",
			"::/0",
			"2001:db8::/32",
		},
		[]string{
			"",
			"10.0.0.0/40",
			"::/129",
			"192.168.1.1",
			"not-a-cidr",
			"256.0.0.0/8",
		})
}

func TestValidateGatewayInCIDR(t *testing.T) {
	cases := []struct {
		gateway string
		cidr    string
		wantErr bool
	}{
		{"192.168.1.1", "192.168.1.0/24", false},
		{"10.0.0.254", "10.0.0.0/24", false},
		{"10.0.1.1", "10.0.0.0/24", true},
		{"", "10.0.0.0/24", false},
		{"10.0.0.1", "", false},
		{"not-an-ip", "10.0.0.0/24", false},
		{"10.0.0.1", "not-a-cidr", false},
	}
	for _, tc := range cases {
		err := ValidateGatewayInCIDR(tc.gateway, tc.cidr)
		if tc.wantErr && err == nil {
			t.Errorf("ValidateGatewayInCIDR(%q, %q) accepted; want error", tc.gateway, tc.cidr)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("ValidateGatewayInCIDR(%q, %q) error: %v", tc.gateway, tc.cidr, err)
		}
	}
}

func TestValidateSSHFingerprint(t *testing.T) {
	checkAcceptReject(t, ValidateSSHFingerprint,
		[]string{
			"",
			"SHA256:abcdefghijklmnopqrstuvwxyz012345678901234567",
			"SHA256:x",
		},
		[]string{
			"SHA256:",
			"MD5:abcd1234",
			"abcdefghijklmnopqrstuvwxyz012345678901234567",
			"sha256:lowercase",
		})
}

func TestValidateBinDir(t *testing.T) {
	checkAcceptReject(t, ValidateBinDir,
		[]string{"", "/usr/local/bin", "/home/user/bin", "/"},
		[]string{"relative/path", "bin", "./bin", "../bin"})
}

func TestValidateNTPServer(t *testing.T) {
	checkAcceptReject(t, ValidateNTPServer,
		[]string{"", "192.168.1.20", "pool.ntp.org", "2001:db8::1"},
		[]string{"!bad!.example", "space in host", "192.168.1.20:123"})
}

func TestValidateNetmaskMatchesMachineCIDR(t *testing.T) {
	cases := []struct {
		name    string
		netmask string
		wantErr bool
	}{
		{"matching dotted form", "255.255.255.0", false},
		{"matching slash form", "/24", false},
		{"mismatched dotted form", "255.255.0.0", true},
		{"mismatched slash form", "/16", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Networking.StaticIP.Netmask = tc.netmask
			result := &ValidationResult{}
			validateAdvancedNetworking(cfg, result)
			if gotErr := hasFieldError(result, FieldNetworkingStaticIPNetmask); gotErr != tc.wantErr {
				t.Errorf("netmask %q: gotErr = %v, want %v; errors: %v", tc.netmask, gotErr, tc.wantErr, result.Errors)
			}
		})
	}
}

func TestValidateStaticIPCollisions(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{
			name:    "start equals proxmox host ip",
			mutate:  func(c *Config) { c.Networking.StaticIP.Start = "192.168.1.100" },
			wantErr: true,
		},
		{
			name: "start equals http-scheme proxmox host ip",
			mutate: func(c *Config) {
				c.Provider.Proxmox.Host = "http://192.168.1.100:8006"
				c.Networking.StaticIP.Start = "192.168.1.100"
			},
			wantErr: true,
		},
		{
			name: "start equals https-scheme proxmox host ip with port",
			mutate: func(c *Config) {
				c.Provider.Proxmox.Host = "https://192.168.1.100:8006"
				c.Networking.StaticIP.Start = "192.168.1.100"
			},
			wantErr: true,
		},
		{
			name: "start equals https-scheme proxmox host ip no port",
			mutate: func(c *Config) {
				c.Provider.Proxmox.Host = "https://192.168.1.100"
				c.Networking.StaticIP.Start = "192.168.1.100"
			},
			wantErr: true,
		},
		{
			name:    "start equals ignition server ip",
			mutate:  func(c *Config) { c.Networking.StaticIP.Start = "192.168.1.20" },
			wantErr: true,
		},
		{
			name: "hostname proxmox host is not comparable",
			mutate: func(c *Config) {
				c.Provider.Proxmox.Host = "pve.example.com:8006"
				c.Networking.StaticIP.Start = "192.168.1.100"
			},
			wantErr: false,
		},
		{
			name:    "distinct start accepted",
			mutate:  func(_ *Config) {},
			wantErr: false,
		},
		{
			name:    "nil proxmox config tolerated",
			mutate:  func(c *Config) { c.Provider.Proxmox = nil },
			wantErr: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.mutate(cfg)
			result := &ValidationResult{}
			validateStaticIPCollisions(cfg, result)
			if gotErr := hasFieldError(result, FieldNetworkingStaticIPStart); gotErr != tc.wantErr {
				t.Errorf("gotErr = %v, want %v; errors: %v", gotErr, tc.wantErr, result.Errors)
			}
		})
	}
}

func TestValidateBastionAndStaticIPDNS(t *testing.T) {
	cases := []struct {
		name    string
		bastion string
		dns     string
		field   string
		wantErr bool
	}{
		{"bastion ip with trailing karg", "192.168.1.20 rd.break", "192.168.1.20", FieldNetworkingBastionIP, true},
		{"static dns with trailing karg empty bastion", "", "192.168.1.20 coreos.inst.insecure", FieldNetworkingStaticIPDNS, true},
		{"non-ip dns", "192.168.1.20", "not-an-ip", FieldNetworkingStaticIPDNS, true},
		{"empty pair tolerated", "", "", "", false},
		{"valid pair accepted", "192.168.1.20", "192.168.1.20", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Networking.Bastion.IP = tc.bastion
			cfg.Networking.StaticIP.DNS = tc.dns
			result := &ValidationResult{}
			validateNetworking(cfg, result)
			if tc.wantErr {
				if !hasFieldError(result, tc.field) {
					t.Errorf("expected error on %s; errors: %v", tc.field, result.Errors)
				}
			} else if hasFieldError(result, FieldNetworkingBastionIP) || hasFieldError(result, FieldNetworkingStaticIPDNS) {
				t.Errorf("unexpected bastion/dns error; errors: %v", result.Errors)
			}
		})
	}
}

func TestValidateHTTPServer(t *testing.T) {
	cases := []struct {
		name    string
		root    string
		ip      string
		wantErr bool
	}{
		{"absolute html root", "/var/www/html", "192.168.1.20", false},
		{"srv ignition root", "/srv/ignition", "192.168.1.20", false},
		{"empty root", "", "192.168.1.20", false},
		{"relative path", "relative/path", "192.168.1.20", true},
		{"quote breakout", `/var/www"html`, "192.168.1.20", true},
		{"command substitution", "/var/www/$(id)", "192.168.1.20", true},
		{"embedded space", "/var/www html", "192.168.1.20", true},
		{"embedded newline", "/var/www\nhtml", "192.168.1.20", true},
		{"dotdot traversal", "/var/www/html/../../etc", "192.168.1.20", true},
		{"non-ip ignition ip", "/var/www/html", "not-an-ip", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{}
			cfg.HTTPServer.Root = tc.root
			cfg.HTTPServer.IgnitionServerIP = tc.ip
			result := &ValidationResult{}
			validateHTTPServer(cfg, result)
			if got := !result.IsValid(); got != tc.wantErr {
				t.Errorf("validateHTTPServer(root=%q, ip=%q) gotErr=%v, want %v; errors: %v",
					tc.root, tc.ip, got, tc.wantErr, result.Errors)
			}
		})
	}
}

func TestDefaultConfigSelfConsistent(t *testing.T) {
	cases := []struct {
		name string
		cfg  *Config
	}{
		{"default", DefaultConfig()},
		{"minimal", MinimalConfig()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if result := tc.cfg.Validate(); !result.IsValid() {
				t.Errorf("config fails its own validation: %v", result.Errors)
			}
		})
	}
}

func TestValidateEndToEnd(t *testing.T) {
	cases := []struct {
		name       string
		mutateCfg  func(*Config)
		wantFields []string
	}{
		{
			name: "invalid cluster name rejected",
			mutateCfg: func(c *Config) {
				c.Cluster.Name = "UPPER-CASE"
			},
			wantFields: []string{FieldClusterName},
		},
		{
			name: "invalid machine cidr rejected",
			mutateCfg: func(c *Config) {
				c.Networking.MachineCIDR = "10.0.0.0/40"
			},
			wantFields: []string{FieldNetworkingMachineCIDR},
		},
		{
			name: "unsupported distribution rejected",
			mutateCfg: func(c *Config) {
				c.Distribution.Type = "k3s"
			},
			wantFields: []string{FieldDistributionType},
		},
		{
			name: "invalid ntp server rejected",
			mutateCfg: func(c *Config) {
				c.Networking.NTPServer = "!not valid!"
			},
			wantFields: []string{FieldNetworkingNTPServer},
		},
		{
			name:       "empty config fails required checks",
			mutateCfg:  func(_ *Config) {},
			wantFields: []string{FieldClusterName, FieldNetworkingMachineCIDR},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{}
			tc.mutateCfg(cfg)
			result := cfg.Validate()
			for _, field := range tc.wantFields {
				if !hasFieldError(result, field) {
					t.Errorf("expected error on field %q, not found; errors: %v", field, result.Errors)
				}
			}
		})
	}
}

func TestValidateAdditionalNetworks(t *testing.T) {
	cases := []struct {
		name       string
		networks   []AdditionalNetwork
		wantFields []string
	}{
		{"empty list", nil, nil},
		{"valid full entry", []AdditionalNetwork{{Bridge: "vmbr1", Model: "virtio", VLANTag: 100}}, nil},
		{"empty model uses virtio default", []AdditionalNetwork{{Bridge: "vmbr1"}}, nil},
		{"vlan bounds", []AdditionalNetwork{{Bridge: "vmbr1", VLANTag: 4094}}, nil},
		{
			"missing bridge",
			[]AdditionalNetwork{{Model: "virtio"}},
			[]string{"provider.proxmox.additional_networks[0].bridge"},
		},
		{
			"hcl interpolation in bridge",
			[]AdditionalNetwork{{Bridge: `${file("/etc/passwd")}`}},
			[]string{"provider.proxmox.additional_networks[0].bridge"},
		},
		{
			"quote breakout in bridge",
			[]AdditionalNetwork{{Bridge: `vmbr0", extra = "x`}},
			[]string{"provider.proxmox.additional_networks[0].bridge"},
		},
		{
			"model outside allowlist",
			[]AdditionalNetwork{{Bridge: "vmbr1", Model: "virtio${run}"}},
			[]string{"provider.proxmox.additional_networks[0].model"},
		},
		{
			"vlan too high",
			[]AdditionalNetwork{{Bridge: "vmbr1", VLANTag: 4095}},
			[]string{"provider.proxmox.additional_networks[0].vlan_tag"},
		},
		{
			"vlan negative",
			[]AdditionalNetwork{{Bridge: "vmbr1", VLANTag: -1}},
			[]string{"provider.proxmox.additional_networks[0].vlan_tag"},
		},
		{
			"second entry indexed independently",
			[]AdditionalNetwork{{Bridge: "vmbr1"}, {Bridge: "bad bridge"}},
			[]string{"provider.proxmox.additional_networks[1].bridge"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &ValidationResult{}
			validateAdditionalNetworks(tc.networks, r)
			if len(tc.wantFields) == 0 {
				if !r.IsValid() {
					t.Fatalf("want valid, got errors: %v", r.Errors)
				}
				return
			}
			for _, field := range tc.wantFields {
				if !hasFieldError(r, field) {
					t.Errorf("missing error for field %s; got %v", field, r.Errors)
				}
			}
		})
	}
}

func TestValidateVMIDBase(t *testing.T) {
	newCfg := func(base, masters, workers int) *Config {
		cfg := DefaultConfig()
		cfg.Topology.VMIDBase = base
		cfg.Topology.ControlPlane.Count = masters
		cfg.Topology.Workers.Count = workers
		return cfg
	}

	cases := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{"default base valid", newCfg(DefaultVMIDBase, 3, 2), false},
		{"minimum valid", newCfg(100, 1, 0), false},
		{"zero rejected", newCfg(0, 1, 0), true},
		{"negative rejected", newCfg(-5, 1, 0), true},
		{"below proxmox floor rejected", newCfg(99, 1, 0), true},
		{"above ceiling rejected", newCfg(1000000000, 1, 0), true},
		{"overflow past ceiling rejected", newCfg(999999997, 3, 2), true},
		{"just under ceiling valid", newCfg(999999990, 3, 2), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &ValidationResult{}
			validateVMIDBase(tc.cfg, r)
			if gotErr := !r.IsValid(); gotErr != tc.wantErr {
				t.Errorf("validateVMIDBase err = %v, want %v (errors: %v)", gotErr, tc.wantErr, r.Errors)
			}
			for _, e := range r.Errors {
				if e.Field != "topology.vm_id_base" {
					t.Errorf("unexpected error field %s", e.Field)
				}
			}
		})
	}
}

func TestValidateBootstrap(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*TopologyConfig)
		wantField string
	}{
		{"count 1 accepted", func(_ *TopologyConfig) {}, ""},
		{"omitted block accepted", func(tp *TopologyConfig) { tp.Bootstrap = NodeConfig{} }, ""},
		{"count above 1 rejected", func(tp *TopologyConfig) { tp.Bootstrap.Count = 3 }, FieldTopologyBootstrapCount},
		{"negative count rejected", func(tp *TopologyConfig) { tp.Bootstrap.Count = -1 }, FieldTopologyBootstrapCount},
		{"disk matching control plane accepted", func(tp *TopologyConfig) {
			tp.ControlPlane.DiskGB = 100
			tp.Bootstrap.DiskGB = 100
		}, ""},
		{"divergent disk rejected", func(tp *TopologyConfig) { tp.Bootstrap.DiskGB = 60 }, FieldTopologyBootstrapDisk},
		{"disk matching default fallback accepted", func(tp *TopologyConfig) {
			tp.ControlPlane.DiskGB = 0
			tp.Bootstrap.DiskGB = DefaultOSDiskGB
		}, ""},
		{"divergent disk with zero control plane rejected", func(tp *TopologyConfig) {
			tp.ControlPlane.DiskGB = 0
			tp.Bootstrap.DiskGB = 60
		}, FieldTopologyBootstrapDisk},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.mutate(&cfg.Topology)
			r := &ValidationResult{}
			validateBootstrap(cfg, r)
			if tc.wantField == "" {
				if !r.IsValid() {
					t.Errorf("unexpected errors: %v", r.Errors)
				}
				return
			}
			if !hasFieldError(r, tc.wantField) {
				t.Errorf("expected error on %q; got %v", tc.wantField, r.Errors)
			}
		})
	}
}

func TestValidateDeploymentFields(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*DeploymentConfig)
		wantField string
	}{
		{"zero timeouts use install defaults", func(_ *DeploymentConfig) {}, ""},
		{"short bootstrap timeout rejected", func(d *DeploymentConfig) { d.BootstrapTimeout = 10 }, FieldDeploymentBootstrapTimeout},
		{"oversized install timeout rejected", func(d *DeploymentConfig) { d.InstallTimeout = 90000 }, FieldDeploymentInstallTimeout},
		{"relative bin dir rejected", func(d *DeploymentConfig) { d.BinDir = "relative/bin" }, FieldDeploymentBinDir},
		{"tilde bin dir accepted after expansion", func(d *DeploymentConfig) { d.BinDir = "~/bin" }, ""},
		{"dotdot bin dir rejected", func(d *DeploymentConfig) { d.BinDir = "/usr/local/bin/../../etc" }, FieldDeploymentBinDir},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.mutate(&cfg.Deployment)
			r := &ValidationResult{}
			validateDeployment(cfg, r)
			if tc.wantField == "" {
				if !r.IsValid() {
					t.Errorf("unexpected errors: %v", r.Errors)
				}
				return
			}
			if !hasFieldError(r, tc.wantField) {
				t.Errorf("expected error on %q; got %v", tc.wantField, r.Errors)
			}
		})
	}
}

func TestWizardWrapperValidators(t *testing.T) {
	cases := []struct {
		name   string
		fn     func(string) error
		accept []string
		reject []string
	}{
		{
			name:   "ValidateDomain",
			fn:     ValidateDomain,
			accept: []string{"example.com", "okd.local", "a-b.c-d.io"},
			reject: []string{"", "ab", "exa mple.com", "EXAMPLE.COM", "foo..bar", "-bad.com", "a.b;rm -rf /", strings.Repeat("a", 254)},
		},
		{
			name:   "ValidateProxmoxNodeName",
			fn:     ValidateProxmoxNodeName,
			accept: []string{"pve1", "node-a", "n_1"},
			reject: []string{"", "1pve", "-pve", "pve;reboot", "pve node", "pve$(id)"},
		},
		{
			name:   "ValidateIP",
			fn:     ValidateIP,
			accept: []string{"192.168.1.1", "10.0.0.254", "::1", "fe80::1"},
			reject: []string{"", "999.1.1.1", "1.2.3", "1.2.3.4/24", "host.example.com", "1.2.3.4;ls"},
		},
		{
			name:   "ValidateCPU",
			fn:     ValidateCPU,
			accept: []string{"1", "128"},
			reject: []string{"0", "129", "four", ""},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkAcceptReject(t, tc.fn, tc.accept, tc.reject)
		})
	}
}

package config

import (
	"strings"
	"testing"
)

func TestIsValidNetmask(t *testing.T) {
	good := []string{
		"/8", "/16", "/24", "/32", "/0",
		"255.255.255.0",
		"255.255.255.255",
		"128.0.0.0",
		"255.255.0.0",
		"255.255.255.254",
	}
	for _, s := range good {
		if !isValidNetmask(s) {
			t.Errorf("isValidNetmask(%q) = false; want true", s)
		}
	}

	bad := []string{
		"",
		"/33",           // prefix too large
		"/-1",           // invalid prefix
		"0.0.0.0",       // canonical but disallowed (would claim whole space)
		"255.255.255.1", // non-contiguous
		"128.0.0.1",     // non-contiguous
		"255.0.255.0",   // non-contiguous
		"fe80::/10",     // ipv6
		"not-an-address",
	}
	for _, s := range bad {
		if isValidNetmask(s) {
			t.Errorf("isValidNetmask(%q) = true; want false", s)
		}
	}
}

func TestValidateProxmoxHost(t *testing.T) {
	good := []string{
		"pve.example.com",
		"pve.example.com:8006",
		"10.0.0.1",
		"10.0.0.1:22",
		"proxmox",
		"[2001:db8::1]:8006",
	}
	for _, s := range good {
		if err := ValidateProxmoxHost(s); err != nil {
			t.Errorf("ValidateProxmoxHost(%q) error: %v", s, err)
		}
	}

	bad := []string{
		"",
		":8006", // empty host
		"!bad!.example",
		"space in host",
	}
	for _, s := range bad {
		if err := ValidateProxmoxHost(s); err == nil {
			t.Errorf("ValidateProxmoxHost(%q) accepted; expected error", s)
		}
	}
}

func TestValidateHAMasters(t *testing.T) {
	cases := []struct {
		count   int
		wantErr bool
	}{
		{0, false}, // 0 is not > 1 so passes
		{1, false},
		{2, true}, // even >1
		{3, false},
		{4, true},
		{5, false},
		{7, false},
		{6, true},
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
	good := []string{
		"", // empty allowed (runtime default)
		"production",
		"staging",
		"dev1",
		"_private",
		"A",
		"Env-Name_01",
	}
	for _, s := range good {
		if err := ValidateTerraformEnv(s); err != nil {
			t.Errorf("ValidateTerraformEnv(%q) error: %v", s, err)
		}
	}

	bad := []string{
		"1starts-with-digit",
		"-starts-with-dash",
		"has space",
		"has/slash",
		"has..dots",
		"../escape",
		"/absolute",
		"env\x00null", // null byte attempt
		"unicodé",     // non-ASCII
		"env.tf",
	}
	for _, s := range bad {
		if err := ValidateTerraformEnv(s); err == nil {
			t.Errorf("ValidateTerraformEnv(%q) accepted; want rejection", s)
		} else if !strings.Contains(err.Error(), "letter") {
			// Sanity: the error message mentions the rule shape.
			t.Logf("ValidateTerraformEnv(%q) err = %v (shape acceptable)", s, err)
		}
	}
}

func TestValidateProxmoxConfigFields(t *testing.T) {
	goodStorage := []string{"local", "local-lvm", "ceph-pool", "storage1", "Tank", "has.dot"}
	badStorage := []string{`local"inject`, "has space", "has/slash", ".leading-dot"}

	hasFieldError := func(r *ValidationResult, field string) bool {
		for _, e := range r.Errors {
			if e.Field == field {
				return true
			}
		}
		return false
	}

	for _, s := range goodStorage {
		cfg := &ProxmoxConfig{Host: "pve:8006", Node: "pve", Storage: "local-lvm", ISOStorage: s, DataStorage: s}
		r := &ValidationResult{}
		validateProxmoxConfig(cfg, r)
		if hasFieldError(r, FieldProxmoxISOStorage) {
			t.Errorf("ISOStorage %q rejected", s)
		}
		if hasFieldError(r, FieldProxmoxDataStorage) {
			t.Errorf("DataStorage %q rejected", s)
		}
	}

	for _, s := range badStorage {
		cfg := &ProxmoxConfig{Host: "pve:8006", Node: "pve", Storage: "local-lvm", ISOStorage: s, DataStorage: s}
		r := &ValidationResult{}
		validateProxmoxConfig(cfg, r)
		if !hasFieldError(r, FieldProxmoxISOStorage) {
			t.Errorf("ISOStorage %q accepted; want rejection", s)
		}
		if !hasFieldError(r, FieldProxmoxDataStorage) {
			t.Errorf("DataStorage %q accepted; want rejection", s)
		}
	}

	goodCPU := []string{"host", "kvm64", "x86-64-v2", "x86-64-v2-AES", "Skylake-Server-noTSX-IBRS", "x86-64-v2+pge", "x86-64-v2,flags=+pge"}
	badCPU := []string{`host"inject`, "has space", "has\nnewline", `"; rm -rf /`}

	for _, s := range goodCPU {
		cfg := &ProxmoxConfig{Host: "pve:8006", Node: "pve", Storage: "local-lvm", CPUType: s}
		r := &ValidationResult{}
		validateProxmoxConfig(cfg, r)
		if hasFieldError(r, FieldProxmoxCPUType) {
			t.Errorf("CPUType %q rejected", s)
		}
	}

	for _, s := range badCPU {
		cfg := &ProxmoxConfig{Host: "pve:8006", Node: "pve", Storage: "local-lvm", CPUType: s}
		r := &ValidationResult{}
		validateProxmoxConfig(cfg, r)
		if !hasFieldError(r, FieldProxmoxCPUType) {
			t.Errorf("CPUType %q accepted; want rejection", s)
		}
	}

	goodBridge := []string{"vmbr0", "vmbr1", "vmbr100", "eth0"}
	badBridge := []string{`vmbr0"inject`, "has space", "0starts-digit"}

	for _, s := range goodBridge {
		cfg := &ProxmoxConfig{Host: "pve:8006", Node: "pve", Storage: "local-lvm", Bridge: s}
		r := &ValidationResult{}
		validateProxmoxConfig(cfg, r)
		if hasFieldError(r, FieldProxmoxBridge) {
			t.Errorf("Bridge %q rejected", s)
		}
	}

	for _, s := range badBridge {
		cfg := &ProxmoxConfig{Host: "pve:8006", Node: "pve", Storage: "local-lvm", Bridge: s}
		r := &ValidationResult{}
		validateProxmoxConfig(cfg, r)
		if !hasFieldError(r, FieldProxmoxBridge) {
			t.Errorf("Bridge %q accepted; want rejection", s)
		}
	}

	// empty optional fields must pass without errors on the new fields
	emptyCfg := &ProxmoxConfig{Host: "pve:8006", Node: "pve", Storage: "local-lvm"}
	emptyResult := &ValidationResult{}
	validateProxmoxConfig(emptyCfg, emptyResult)
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
	good := []string{
		"a",
		"abc",
		"a-b",
		"abc-def",
		"a0",
		"0abc",
		strings.Repeat("a", 63),
	}
	for _, s := range good {
		if !IsValidDNSLabel(s) {
			t.Errorf("IsValidDNSLabel(%q) = false; want true", s)
		}
	}

	bad := []string{
		"",
		"-abc",
		"abc-",
		"A-UPPER",
		`a"b`,
		"../etc",
		strings.Repeat("a", 64),
	}
	for _, s := range bad {
		if IsValidDNSLabel(s) {
			t.Errorf("IsValidDNSLabel(%q) = true; want false", s)
		}
	}
}

func TestValidateClusterName(t *testing.T) {
	cases := []struct {
		input   string
		wantErr bool
	}{
		{"ab", false},
		{"my-cluster", false},
		{"0abc", false},
		{strings.Repeat("a", 63), false},
		{"", true},
		{"a", true},
		{"-abc", true},
		{"ABC", true},
		{`a"b`, true},
		{"../etc", true},
		{strings.Repeat("a", 64), true},
	}
	for _, tc := range cases {
		err := ValidateClusterName(tc.input)
		if tc.wantErr && err == nil {
			t.Errorf("ValidateClusterName(%q) accepted; want error", tc.input)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("ValidateClusterName(%q) error: %v", tc.input, err)
		}
	}
}

func TestValidateCIDR(t *testing.T) {
	good := []string{
		"192.168.1.0/24",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"::/0",
		"2001:db8::/32",
	}
	for _, s := range good {
		if err := ValidateCIDR(s); err != nil {
			t.Errorf("ValidateCIDR(%q) error: %v", s, err)
		}
	}

	bad := []string{
		"",
		"10.0.0.0/40",
		"::/129",
		"192.168.1.1",
		"not-a-cidr",
		"256.0.0.0/8",
	}
	for _, s := range bad {
		if err := ValidateCIDR(s); err == nil {
			t.Errorf("ValidateCIDR(%q) accepted; want error", s)
		}
	}
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
	good := []string{
		"",
		"SHA256:abcdefghijklmnopqrstuvwxyz012345678901234567",
		"SHA256:x",
	}
	for _, s := range good {
		if err := ValidateSSHFingerprint(s); err != nil {
			t.Errorf("ValidateSSHFingerprint(%q) error: %v", s, err)
		}
	}

	bad := []string{
		"SHA256:",
		"MD5:abcd1234",
		"abcdefghijklmnopqrstuvwxyz012345678901234567",
		"sha256:lowercase",
	}
	for _, s := range bad {
		if err := ValidateSSHFingerprint(s); err == nil {
			t.Errorf("ValidateSSHFingerprint(%q) accepted; want error", s)
		}
	}
}

func TestValidateBinDir(t *testing.T) {
	good := []string{
		"",
		"/usr/local/bin",
		"/home/user/bin",
		"/",
	}
	for _, s := range good {
		if err := ValidateBinDir(s); err != nil {
			t.Errorf("ValidateBinDir(%q) error: %v", s, err)
		}
	}

	bad := []string{
		"relative/path",
		"bin",
		"./bin",
		"../bin",
	}
	for _, s := range bad {
		if err := ValidateBinDir(s); err == nil {
			t.Errorf("ValidateBinDir(%q) accepted; want error", s)
		}
	}
}

func TestValidateNTPServer(t *testing.T) {
	good := []string{
		"",
		"192.168.1.20",
		"pool.ntp.org",
		"2001:db8::1",
	}
	for _, s := range good {
		if err := ValidateNTPServer(s); err != nil {
			t.Errorf("ValidateNTPServer(%q) error: %v", s, err)
		}
	}

	bad := []string{
		"!bad!.example",
		"space in host",
		"192.168.1.20:123",
	}
	for _, s := range bad {
		if err := ValidateNTPServer(s); err == nil {
			t.Errorf("ValidateNTPServer(%q) accepted; want error", s)
		}
	}
}

func TestValidateNetworkingNTPServer(t *testing.T) {
	cases := []struct {
		name    string
		server  string
		wantErr bool
	}{
		{"empty accepted (bastion default applies)", "", false},
		{"valid ip accepted", "192.168.1.20", false},
		{"valid hostname accepted", "ntp.example.com", false},
		{"invalid host rejected", "!not valid!", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Networking.NTPServer = tc.server
			result := &ValidationResult{}
			validateNetworking(cfg, result)
			gotErr := false
			for _, e := range result.Errors {
				if e.Field == FieldNetworkingNTPServer {
					gotErr = true
				}
			}
			if gotErr != tc.wantErr {
				t.Errorf("server %q: gotErr = %v, want %v; errors: %v", tc.server, gotErr, tc.wantErr, result.Errors)
			}
		})
	}
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
			gotErr := false
			for _, e := range result.Errors {
				if e.Field == FieldNetworkingStaticIPNetmask {
					gotErr = true
				}
			}
			if gotErr != tc.wantErr {
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
			gotErr := false
			for _, e := range result.Errors {
				if e.Field == FieldNetworkingStaticIPStart {
					gotErr = true
				}
			}
			if gotErr != tc.wantErr {
				t.Errorf("gotErr = %v, want %v; errors: %v", gotErr, tc.wantErr, result.Errors)
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

	hasField := func(r *ValidationResult, field string) bool {
		for _, e := range r.Errors {
			if e.Field == field {
				return true
			}
		}
		return false
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{}
			tc.mutateCfg(cfg)
			result := cfg.Validate()
			for _, field := range tc.wantFields {
				if !hasField(result, field) {
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
				found := false
				for _, e := range r.Errors {
					if e.Field == field {
						found = true
						break
					}
				}
				if !found {
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

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

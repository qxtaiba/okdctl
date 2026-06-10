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
		"/33",            // prefix too large
		"/-1",            // invalid prefix
		"0.0.0.0",        // canonical but disallowed (would claim whole space)
		"255.255.255.1",  // non-contiguous
		"128.0.0.1",      // non-contiguous
		"255.0.255.0",    // non-contiguous
		"fe80::/10",      // ipv6
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
		"env\x00null",  // null byte attempt
		"unicodé",       // non-ASCII
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
	goodStorage := []string{"local", "local-lvm", "ceph-pool", "storage1", "Tank"}
	badStorage := []string{`local"inject`, "has space", "has/slash", "has.dot"}

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

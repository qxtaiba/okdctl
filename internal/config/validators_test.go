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

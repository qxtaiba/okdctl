package dns

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateConfigName(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid simple", input: "okd-prod"},
		{name: "valid alphanumeric", input: "mycluster1"},
		{name: "valid with underscore", input: "okd_dev"},
		{name: "max length 64", input: strings.Repeat("a", 64)},
		{name: "empty", input: "", wantErr: true},
		{name: "leading dot", input: ".hidden", wantErr: true},
		{name: "path traversal", input: "../etc/passwd", wantErr: true},
		{name: "slash", input: "etc/passwd", wantErr: true},
		{name: "dot inside", input: "okd.prod", wantErr: true},
		{name: "space", input: "okd prod", wantErr: true},
		{name: "special chars", input: "okd@prod!", wantErr: true},
		{name: "leading hyphen", input: "-leading", wantErr: true},
		{name: "too long", input: strings.Repeat("a", 65), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConfigName(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("validateConfigName(%q): expected error, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validateConfigName(%q): unexpected error: %v", tc.input, err)
			}
		})
	}
}

func TestDnsmasqConfigPath(t *testing.T) {
	t.Run("valid name returns expected path", func(t *testing.T) {
		got, err := DnsmasqConfigPath("okd-prod")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(dnsmasqConfigDir, "okd-prod.conf")
		if got != want {
			t.Errorf("DnsmasqConfigPath = %q; want %q", got, want)
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		_, err := DnsmasqConfigPath("../etc/passwd")
		if err == nil {
			t.Error("expected error for path traversal input, got nil")
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		_, err := DnsmasqConfigPath("")
		if err == nil {
			t.Error("expected error for empty name, got nil")
		}
	})
}

func TestConfigName(t *testing.T) {
	got := configName("prod")
	if got != "okd-prod" {
		t.Errorf("configName(%q) = %q; want %q", "prod", got, "okd-prod")
	}
}

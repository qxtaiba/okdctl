package dns

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
		{name: "single char", input: "a"},
		{name: "two chars", input: "a1"},
		{name: "unicode", input: "é", wantErr: true},
		{name: "null byte", input: "a\x00b", wantErr: true},
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

func installFakeNmcli(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-nmcli script relies on POSIX sh")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "nmcli")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestGetActiveConnectionStderr(t *testing.T) {
	installFakeNmcli(t, "#!/bin/sh\necho 'Error: NetworkManager is not running' >&2\nexit 10\n")
	_, err := getActiveConnection(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "NetworkManager is not running") {
		t.Errorf("error does not contain nmcli stderr: %q", err.Error())
	}
}

func TestValidateConnectionName(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "simple", input: "eth0"},
		{name: "with space", input: "Wired connection 1"},
		{name: "hyphen underscore", input: "my-conn_1"},
		{name: "dot and colon", input: "br0:1"},
		{name: "empty", input: "", wantErr: true},
		{name: "semicolon", input: "eth0;id", wantErr: true},
		{name: "newline", input: "eth0\nid", wantErr: true},
		{name: "carriage return", input: "eth0\rid", wantErr: true},
		{name: "null byte", input: "eth0\x00id", wantErr: true},
		{name: "backtick", input: "eth`0", wantErr: true},
		{name: "dollar sign", input: "eth$0", wantErr: true},
		{name: "less than", input: "eth<0", wantErr: true},
		{name: "greater than", input: "eth>0", wantErr: true},
		{name: "pipe", input: "eth0|id", wantErr: true},
		{name: "ampersand", input: "eth0&id", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConnectionName(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("validateConnectionName(%q): expected error, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validateConnectionName(%q): unexpected error: %v", tc.input, err)
			}
		})
	}
}

func TestGetActiveConnectionRejectsUnsafeName(t *testing.T) {
	installFakeNmcli(t, "#!/bin/sh\nprintf 'eth0;evil\\n'\n")
	_, err := getActiveConnection(context.Background())
	if err == nil {
		t.Fatal("expected error for unsafe connection name, got nil")
	}
	if !strings.Contains(err.Error(), "unsafe character") {
		t.Errorf("error does not mention unsafe character: %q", err.Error())
	}
}

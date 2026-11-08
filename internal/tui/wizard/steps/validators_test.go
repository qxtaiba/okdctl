package steps

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateFilePath(t *testing.T) {
	if err := ValidateFilePath(""); err == nil {
		t.Fatal("ValidateFilePath(empty) = nil, want error")
	}
	dir := t.TempDir()
	if err := ValidateFilePath(filepath.Join(dir, "does-not-exist")); err == nil {
		t.Fatal("ValidateFilePath(missing file) = nil, want error")
	}
	existing := filepath.Join(dir, "present.txt")
	if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFilePath(existing); err != nil {
		t.Fatalf("ValidateFilePath(existing file) = %v, want nil", err)
	}
	if err := ValidateFilePath(dir); err == nil {
		t.Fatal("ValidateFilePath(directory) = nil, want error")
	}
}

func TestFieldValidators(t *testing.T) {
	cases := []struct {
		name     string
		validate func(string) error
		good     []string
		bad      []string
	}{
		{"cluster name", validateClusterName, []string{"lab", "prod-1"}, []string{"", "Lab", "a.b", "-lab", "lab_1"}},
		{"domain", validateDomain, []string{"k8s.local", "example.com"}, []string{"", "bad..domain", "Example.com", "-x.com"}},
		{"ip", validateIP, []string{"192.168.1.1", "fd00::1"}, []string{"", "999.1.1.1", "pve"}},
		{"cidr", validateCIDR, []string{"192.168.1.0/24", "10.128.0.0/14"}, []string{"", "10.0.0.0/40", "192.168.1.1"}},
		{"ip list", validateIPList, []string{"", "192.168.1.1", "192.168.1.1, 8.8.8.8", " 1.1.1.1 ,"}, []string{"not-an-ip", "192.168.1.1,not-an-ip"}},
		{"endpoint", validateEndpoint, []string{"https://pve:8006", "http://10.0.0.5"}, []string{"", "pve", "pve:8006", "ftp://pve:8006", "ssh://pve"}},
		{"duration", validateDuration, []string{"15m", "1h30m", " 90s "}, []string{"", "soon", "15"}},
		{"min int", minInt(4, "vcpu"), []string{"4", "128"}, []string{"3", "", "four"}},
		{"int range", intRange(0, 4094), []string{"0", "4094"}, []string{"-1", "4095", "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range tc.good {
				if err := tc.validate(v); err != nil {
					t.Errorf("%q rejected: %v", v, err)
				}
			}
			for _, v := range tc.bad {
				if err := tc.validate(v); err == nil {
					t.Errorf("%q accepted", v)
				}
			}
		})
	}
}

func TestDurationText(t *testing.T) {
	cases := map[time.Duration]string{
		15 * time.Minute:             "15m",
		time.Hour:                    "1h",
		90 * time.Minute:             "1h30m",
		90 * time.Second:             "1m30s",
		2*time.Hour + 15*time.Second: "2h0m15s",
		0:                            "0s",
	}
	for d, want := range cases {
		if got := durationText(d); got != want {
			t.Errorf("durationText(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestFieldLabel(t *testing.T) {
	if got := fieldLabel("networking.api_vip"); got != "api vip" {
		t.Errorf("fieldLabel = %q, want %q", got, "api vip")
	}
}

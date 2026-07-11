package steps

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateFilePath(t *testing.T) {
	if err := ValidateFilePath(""); err == nil {
		t.Fatal("ValidateFilePath(empty) = nil, want error")
	}

	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")
	if err := ValidateFilePath(missing); err == nil {
		t.Fatal("ValidateFilePath(missing file) = nil, want error")
	}

	existing := filepath.Join(dir, "present.txt")
	if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := ValidateFilePath(existing); err != nil {
		t.Fatalf("ValidateFilePath(existing file) = %v, want nil", err)
	}
}

func TestValidateDNSServers(t *testing.T) {
	good := []string{
		"",
		"192.168.1.1",
		"192.168.1.1,8.8.8.8",
		" 192.168.1.1 , 8.8.8.8 ",
	}
	for _, s := range good {
		if err := validateDNSServers(s); err != nil {
			t.Errorf("validateDNSServers(%q) = %v, want nil", s, err)
		}
	}

	bad := []string{
		"not-an-ip",
		"192.168.1.1,not-an-ip",
	}
	for _, s := range bad {
		if err := validateDNSServers(s); err == nil {
			t.Errorf("validateDNSServers(%q) = nil, want error", s)
		}
	}
}

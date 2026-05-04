package dns

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

func TestIsBootstrapDNS(t *testing.T) {
	dir := t.TempDir()
	origDir := dnsmasqConfigDir
	dnsmasqConfigDir = dir
	defer func() { dnsmasqConfigDir = origDir }()

	cfg := &config.Config{}
	cfg.Cluster.Name = "okd"
	cfg.Cluster.Domain = "example.com"
	cfg.Networking.Bastion.IP = "10.0.0.1"

	t.Run("absent config returns false", func(t *testing.T) {
		got, err := IsBootstrapDNS(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Error("expected false for missing config file")
		}
	})

	t.Run("bootstrap-pointed config returns true", func(t *testing.T) {
		content := "address=/api.okd.example.com/10.0.0.1\n"
		p := filepath.Join(dir, "okd-okd.conf")
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(p)

		got, err := IsBootstrapDNS(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Error("expected true for bootstrap-pointed config")
		}
	})

	t.Run("production config returns false", func(t *testing.T) {
		content := "address=/api.okd.example.com/10.0.0.10\n"
		p := filepath.Join(dir, "okd-okd.conf")
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(p)

		got, err := IsBootstrapDNS(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Error("expected false for production config pointing at vip")
		}
	})
}

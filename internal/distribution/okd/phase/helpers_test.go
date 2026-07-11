package phase

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

func TestResolveClusterVIP(t *testing.T) {
	t.Run("explicit VIP wins", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Networking.Bastion.VIP = "10.0.0.5"
		cfg.Networking.StaticIP.Start = "10.0.0.100"

		got, err := ResolveClusterVIP(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "10.0.0.5" {
			t.Errorf("got %q, want %q", got, "10.0.0.5")
		}
	})

	t.Run("static-IP-derived VIP when no explicit", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Networking.StaticIP.Start = "192.168.1.50"

		got, err := ResolveClusterVIP(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "192.168.1.10" {
			t.Errorf("got %q, want %q", got, "192.168.1.10")
		}
	})

	t.Run("malformed VIP wraps with prefix", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Networking.Bastion.VIP = "not-an-ip"

		_, err := ResolveClusterVIP(cfg)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		const wantPrefix = "resolve VIP:"
		if !strings.HasPrefix(err.Error(), wantPrefix) {
			t.Errorf("error %q does not start with %q", err.Error(), wantPrefix)
		}
	})
}

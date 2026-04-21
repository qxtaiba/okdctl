package cli

import (
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/config"
)

func TestRedactConfig(t *testing.T) {
	t.Run("TokenID masked in output", func(t *testing.T) {
		cfg := &config.Config{
			Provider: config.ProviderConfig{
				Type: config.ProviderProxmox,
				Proxmox: &config.ProxmoxConfig{
					Host:    "pve.example",
					TokenID: "root@pam!terraform-secret-id",
				},
			},
		}
		got := redactConfig(cfg)
		if got.Provider.Proxmox.TokenID != "***" {
			t.Errorf("TokenID = %q; want ***", got.Provider.Proxmox.TokenID)
		}
	})

	t.Run("empty TokenID left alone", func(t *testing.T) {
		cfg := &config.Config{
			Provider: config.ProviderConfig{
				Proxmox: &config.ProxmoxConfig{TokenID: ""},
			},
		}
		got := redactConfig(cfg)
		if got.Provider.Proxmox.TokenID != "" {
			t.Errorf("empty TokenID must not become ***; got %q", got.Provider.Proxmox.TokenID)
		}
	})

	t.Run("nil Proxmox provider leaves config unchanged", func(t *testing.T) {
		cfg := &config.Config{}
		got := redactConfig(cfg)
		if got.Provider.Proxmox != nil {
			t.Errorf("nil Proxmox became %+v", got.Provider.Proxmox)
		}
	})

	t.Run("original config is not mutated", func(t *testing.T) {
		cfg := &config.Config{
			Provider: config.ProviderConfig{
				Proxmox: &config.ProxmoxConfig{TokenID: "id-live"},
			},
		}
		_ = redactConfig(cfg)
		if cfg.Provider.Proxmox.TokenID != "id-live" {
			t.Errorf("redactConfig mutated source; TokenID now %q", cfg.Provider.Proxmox.TokenID)
		}
	})

	t.Run("yaml marshalling omits Username/Password/APIToken", func(t *testing.T) {
		// Defense-in-depth for the regression trap the finding called out:
		// lock that the json:"-" exclusion tags survive whatever reshuffling
		// happens to ProxmoxConfig.
		cfg := &config.Config{
			Provider: config.ProviderConfig{
				Proxmox: &config.ProxmoxConfig{
					Host:     "pve.example",
					Username: "root@pam",
					Password: "plaintext-pw",
					APIToken: "plaintext-token",
					TokenID:  "tid",
				},
			},
		}
		out, err := yaml.Marshal(redactConfig(cfg))
		if err != nil {
			t.Fatal(err)
		}
		s := string(out)
		for _, forbidden := range []string{"plaintext-pw", "plaintext-token", "root@pam"} {
			if strings.Contains(s, forbidden) {
				t.Errorf("leaked %q in YAML output:\n%s", forbidden, s)
			}
		}
		if !strings.Contains(s, "***") {
			t.Errorf("expected *** placeholder in YAML; got:\n%s", s)
		}
	})
}

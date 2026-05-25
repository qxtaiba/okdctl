package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/config"
)

func TestRunConfigShow_JSONOutput(t *testing.T) {
	cfg := &config.Config{
		Provider: config.ProviderConfig{
			Type: config.ProviderProxmox,
			Proxmox: &config.ProxmoxConfig{
				Host:    "pve.example",
				TokenID: "secret-token-id",
			},
		},
	}

	redacted := redactConfig(cfg)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(redacted); err != nil {
		t.Fatalf("json.Encode: %v", err)
	}

	out := buf.String()

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}

	if strings.Contains(out, "secret-token-id") {
		t.Errorf("JSON output leaks unredacted TokenID:\n%s", out)
	}
}

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
		cfg := &config.Config{
			Provider: config.ProviderConfig{
				Proxmox: &config.ProxmoxConfig{
					Host:     "pve.example",
					Username: "root@pam",
					TokenID:  "tid",
				},
			},
		}
		cfg.Provider.Proxmox.Password.Set("EXAMPLE-PLAINTEXT-PW")
		cfg.Provider.Proxmox.APIToken.Set("EXAMPLE-PLAINTEXT-TOKEN")
		out, err := yaml.Marshal(redactConfig(cfg))
		if err != nil {
			t.Fatal(err)
		}
		s := string(out)
		for _, forbidden := range []string{"EXAMPLE-PLAINTEXT-PW", "EXAMPLE-PLAINTEXT-TOKEN", "root@pam"} {
			if strings.Contains(s, forbidden) {
				t.Errorf("leaked %q in YAML output:\n%s", forbidden, s)
			}
		}
		if !strings.Contains(s, "***") {
			t.Errorf("expected *** placeholder in YAML; got:\n%s", s)
		}
	})

	t.Run("non-sensitive field passes through unchanged", func(t *testing.T) {
		cfg := &config.Config{
			Provider: config.ProviderConfig{
				Proxmox: &config.ProxmoxConfig{
					Host: "pve.example",
				},
			},
		}
		got := redactConfig(cfg)
		if got.Provider.Proxmox.Host != "pve.example" {
			t.Errorf("Host = %q; want pve.example", got.Provider.Proxmox.Host)
		}
	})

	t.Run("nested struct secret field masked via reflection walker", func(t *testing.T) {
		cfg := &config.Config{
			Provider: config.ProviderConfig{
				Proxmox: &config.ProxmoxConfig{
					Host:    "pve.example",
					TokenID: "nested-token-value",
				},
			},
		}
		got := redactConfig(cfg)
		if got.Provider.Proxmox.TokenID != "***" {
			t.Errorf("nested TokenID = %q; want ***", got.Provider.Proxmox.TokenID)
		}
		if got.Provider.Proxmox.Host != "pve.example" {
			t.Errorf("non-sensitive Host altered to %q", got.Provider.Proxmox.Host)
		}
	})

	t.Run("nil pointer fields do not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("redactConfig panicked on nil pointer: %v", r)
			}
		}()
		cfg := &config.Config{}
		_ = redactConfig(cfg)
	})
}

package config

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestRedactedJSONOutput(t *testing.T) {
	cfg := &Config{
		Provider: ProviderConfig{
			Type: ProviderProxmox,
			Proxmox: &ProxmoxConfig{
				Host:    "pve.example",
				TokenID: "secret-token-id",
			},
		},
	}

	redacted := Redacted(cfg)

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

func TestRedacted(t *testing.T) {
	t.Run("TokenID masked in output", func(t *testing.T) {
		cfg := &Config{
			Provider: ProviderConfig{
				Type: ProviderProxmox,
				Proxmox: &ProxmoxConfig{
					Host:    "pve.example",
					TokenID: "root@pam!terraform-secret-id",
				},
			},
		}
		got := Redacted(cfg)
		if got.Provider.Proxmox.TokenID != "***" {
			t.Errorf("TokenID = %q; want ***", got.Provider.Proxmox.TokenID)
		}
	})

	t.Run("empty TokenID left alone", func(t *testing.T) {
		cfg := &Config{
			Provider: ProviderConfig{
				Proxmox: &ProxmoxConfig{TokenID: ""},
			},
		}
		got := Redacted(cfg)
		if got.Provider.Proxmox.TokenID != "" {
			t.Errorf("empty TokenID must not become ***; got %q", got.Provider.Proxmox.TokenID)
		}
	})

	t.Run("nil Proxmox provider leaves config unchanged", func(t *testing.T) {
		cfg := &Config{}
		got := Redacted(cfg)
		if got.Provider.Proxmox != nil {
			t.Errorf("nil Proxmox became %+v", got.Provider.Proxmox)
		}
	})

	t.Run("original config is not mutated", func(t *testing.T) {
		cfg := &Config{
			Provider: ProviderConfig{
				Proxmox: &ProxmoxConfig{TokenID: "id-live"},
			},
		}
		_ = Redacted(cfg)
		if cfg.Provider.Proxmox.TokenID != "id-live" {
			t.Errorf("Redacted mutated source; TokenID now %q", cfg.Provider.Proxmox.TokenID)
		}
	})

	t.Run("yaml marshalling omits Username/Password/APIToken", func(t *testing.T) {
		cfg := &Config{
			Provider: ProviderConfig{
				Proxmox: &ProxmoxConfig{
					Host:     "pve.example",
					Username: "root@pam",
					TokenID:  "tid",
				},
			},
		}
		cfg.Provider.Proxmox.Password.Set("EXAMPLE-PLAINTEXT-PW")
		cfg.Provider.Proxmox.APIToken.Set("EXAMPLE-PLAINTEXT-TOKEN")
		out, err := yaml.Marshal(Redacted(cfg))
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
		cfg := &Config{
			Provider: ProviderConfig{
				Proxmox: &ProxmoxConfig{
					Host: "pve.example",
				},
			},
		}
		got := Redacted(cfg)
		if got.Provider.Proxmox.Host != "pve.example" {
			t.Errorf("Host = %q; want pve.example", got.Provider.Proxmox.Host)
		}
	})

	t.Run("nested struct secret field masked via reflection walker", func(t *testing.T) {
		cfg := &Config{
			Provider: ProviderConfig{
				Proxmox: &ProxmoxConfig{
					Host:    "pve.example",
					TokenID: "nested-token-value",
				},
			},
		}
		got := Redacted(cfg)
		if got.Provider.Proxmox.TokenID != "***" {
			t.Errorf("nested TokenID = %q; want ***", got.Provider.Proxmox.TokenID)
		}
		if got.Provider.Proxmox.Host != "pve.example" {
			t.Errorf("non-sensitive Host altered to %q", got.Provider.Proxmox.Host)
		}
	})

	t.Run("secret-keyed addon setting masked in map", func(t *testing.T) {
		cfg := &Config{
			Addons: map[string]AddonConfig{
				"secretstore": {
					Enabled: true,
					Settings: map[string]string{
						"registry_password": "hunter2",
						"provider":          "onepassword",
					},
				},
			},
		}
		got := Redacted(cfg)
		s := got.Addons["secretstore"].Settings
		if s["registry_password"] != "***" {
			t.Errorf("Settings[registry_password] = %q; want ***", s["registry_password"])
		}
		if s["provider"] != "onepassword" {
			t.Errorf("Settings[provider] = %q; want onepassword", s["provider"])
		}
	})

	t.Run("map masking does not mutate source config", func(t *testing.T) {
		cfg := &Config{
			Addons: map[string]AddonConfig{
				"a": {Settings: map[string]string{"api_token": "live-tok"}},
			},
		}
		_ = Redacted(cfg)
		if cfg.Addons["a"].Settings["api_token"] != "live-tok" {
			t.Errorf("Redacted mutated source map; Settings[api_token] = %q",
				cfg.Addons["a"].Settings["api_token"])
		}
	})

	t.Run("empty secret-keyed map value left alone", func(t *testing.T) {
		cfg := &Config{
			Addons: map[string]AddonConfig{
				"a": {Settings: map[string]string{"api_token": ""}},
			},
		}
		got := Redacted(cfg)
		if got.Addons["a"].Settings["api_token"] != "" {
			t.Errorf("empty map value must not become ***; got %q",
				got.Addons["a"].Settings["api_token"])
		}
	})

	t.Run("struct in slice masked without mutating source", func(t *testing.T) {
		cfg := &Config{
			Provider: ProviderConfig{
				Proxmox: &ProxmoxConfig{
					Host: "pve.example",
					AdditionalNetworks: []AdditionalNetwork{
						{Bridge: "vmbr1", Model: "virtio"},
					},
				},
			},
		}
		got := Redacted(cfg)
		if got.Provider.Proxmox.AdditionalNetworks[0].Bridge != "vmbr1" {
			t.Errorf("non-secret slice element altered: %+v",
				got.Provider.Proxmox.AdditionalNetworks[0])
		}
		got.Provider.Proxmox.AdditionalNetworks[0].Bridge = "changed"
		if cfg.Provider.Proxmox.AdditionalNetworks[0].Bridge != "vmbr1" {
			t.Error("slice backing shared between Redacted copy and source")
		}
	})

	t.Run("nil pointer fields do not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Redacted panicked on nil pointer: %v", r)
			}
		}()
		cfg := &Config{}
		_ = Redacted(cfg)
	})
}

func TestProxmoxConfig_Redacted(t *testing.T) {
	p := &ProxmoxConfig{
		Host:      "pve.example",
		Username:  "root@pam",
		TokenID:   "tid",
		HAEnabled: true,
	}
	got, ok := p.Redacted().(redactedProxmoxConfig)
	if !ok {
		t.Fatalf("Redacted() type = %T, want redactedProxmoxConfig", p.Redacted())
	}
	if !got.HAEnabled {
		t.Error("redactedProxmoxConfig.HAEnabled = false, want true")
	}
	if got.Host != p.Host {
		t.Errorf("redactedProxmoxConfig.Host = %q, want %q", got.Host, p.Host)
	}

	if (*ProxmoxConfig)(nil).Redacted() != nil {
		t.Error("Redacted() on nil *ProxmoxConfig must return nil")
	}
}

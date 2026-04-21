package credentials

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

func TestProxmoxCredentials_Zeroize(t *testing.T) {
	t.Run("wipes password and api token", func(t *testing.T) {
		pw := []byte("s3cret")
		tok := []byte("token-abc")
		c := &ProxmoxCredentials{Password: pw, APIToken: tok}

		c.Zeroize()

		if c.Password != nil {
			t.Errorf("Password slice not nilled: %v", c.Password)
		}
		if c.APIToken != nil {
			t.Errorf("APIToken slice not nilled: %v", c.APIToken)
		}
		// Original backing arrays should be zero — Zeroize wrote zeros into
		// the elements before nilling the header.
		for i, b := range pw {
			if b != 0 {
				t.Errorf("Password[%d] not zeroed: %d", i, b)
			}
		}
		for i, b := range tok {
			if b != 0 {
				t.Errorf("APIToken[%d] not zeroed: %d", i, b)
			}
		}
	})

	t.Run("nil receiver is safe", func(t *testing.T) {
		var c *ProxmoxCredentials
		c.Zeroize() // must not panic
	})

	t.Run("empty slices are safe", func(t *testing.T) {
		c := &ProxmoxCredentials{}
		c.Zeroize()
	})
}

func TestProxmoxCredentials_StringMasks(t *testing.T) {
	c := &ProxmoxCredentials{
		Endpoint: "https://pve.example:8006",
		Username: "root@pam",
		Password: []byte("hunter2"),
		APIToken: []byte("deadbeef-token"),
		Source:   SourceEnv,
	}

	for _, fmtStr := range []string{"%s", "%v", "%+v", "%#v"} {
		rendered := fmt.Sprintf(fmtStr, c)
		if strings.Contains(rendered, "hunter2") {
			t.Errorf("%s leaked password: %s", fmtStr, rendered)
		}
		if strings.Contains(rendered, "deadbeef-token") {
			t.Errorf("%s leaked api token: %s", fmtStr, rendered)
		}
	}

	// Non-secret fields should still render.
	s := c.String()
	if !strings.Contains(s, "root@pam") || !strings.Contains(s, "pve.example") {
		t.Errorf("String() dropped non-secret fields: %s", s)
	}

	// Nil receiver renders a sentinel, not a panic.
	var nilCreds *ProxmoxCredentials
	if got := nilCreds.String(); got != "ProxmoxCredentials(nil)" {
		t.Errorf("nil String() = %q", got)
	}
}

func TestProxmoxCredentials_Env(t *testing.T) {
	tests := []struct {
		name    string
		creds   ProxmoxCredentials
		wantHas []string
		wantNot []string
	}{
		{
			name:    "invalid creds return nil",
			creds:   ProxmoxCredentials{Endpoint: "https://pve:8006"},
			wantHas: nil,
			wantNot: []string{"PROXMOX_VE_ENDPOINT"},
		},
		{
			name: "api token path",
			creds: ProxmoxCredentials{
				Endpoint: "https://pve:8006",
				APIToken: []byte("tok"),
			},
			wantHas: []string{"PROXMOX_VE_ENDPOINT=https://pve:8006", "PROXMOX_VE_API_TOKEN=tok"},
			wantNot: []string{"PROXMOX_VE_USERNAME", "PROXMOX_VE_PASSWORD"},
		},
		{
			name: "username+password path",
			creds: ProxmoxCredentials{
				Endpoint: "https://pve:8006",
				Username: "root@pam",
				Password: []byte("pw"),
			},
			wantHas: []string{"PROXMOX_VE_USERNAME=root@pam", "PROXMOX_VE_PASSWORD=pw"},
			wantNot: []string{"PROXMOX_VE_API_TOKEN"},
		},
		{
			name: "insecure flag appended",
			creds: ProxmoxCredentials{
				Endpoint: "https://pve:8006",
				APIToken: []byte("tok"),
				Insecure: true,
			},
			wantHas: []string{"PROXMOX_VE_INSECURE=true"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.creds.Env()
			joined := strings.Join(env, "\x00")
			for _, must := range tc.wantHas {
				if !strings.Contains(joined, must) {
					t.Errorf("Env() missing %q; got %v", must, env)
				}
			}
			for _, forbid := range tc.wantNot {
				if strings.Contains(joined, forbid+"=") {
					t.Errorf("Env() contained %q unexpectedly; got %v", forbid, env)
				}
			}
		})
	}

	t.Run("password backing not shared with env string", func(t *testing.T) {
		pw := []byte("pw-alive")
		creds := ProxmoxCredentials{
			Endpoint: "https://pve:8006",
			Username: "root@pam",
			Password: pw,
		}
		env := creds.Env()
		// Wipe underlying buffer — string(pw) copies, so the env entry must
		// survive the wipe.
		for i := range pw {
			pw[i] = 0
		}
		found := false
		for _, kv := range env {
			if kv == "PROXMOX_VE_PASSWORD=pw-alive" {
				found = true
			}
		}
		if !found {
			t.Errorf("env password corrupted after Zeroize-style wipe: %v", env)
		}
	})
}

func TestGetProxmoxCredentials(t *testing.T) {
	clearProxmoxEnv(t)

	cfgWithHost := func() *config.Config {
		return &config.Config{
			Provider: config.ProviderConfig{
				Type:    config.ProviderProxmox,
				Proxmox: &config.ProxmoxConfig{Host: "pve.example"},
			},
		}
	}

	t.Run("nil config returns SourceNone", func(t *testing.T) {
		creds := GetProxmoxCredentials(nil)
		if creds.Source != SourceNone {
			t.Errorf("Source = %v, want SourceNone", creds.Source)
		}
	})

	t.Run("empty host returns SourceNone", func(t *testing.T) {
		cfg := &config.Config{Provider: config.ProviderConfig{Proxmox: &config.ProxmoxConfig{}}}
		creds := GetProxmoxCredentials(cfg)
		if creds.Source != SourceNone {
			t.Errorf("Source = %v, want SourceNone", creds.Source)
		}
	})

	t.Run("env api token wins over config", func(t *testing.T) {
		clearProxmoxEnv(t)
		t.Setenv("PROXMOX_VE_API_TOKEN", "env-token")
		cfg := cfgWithHost()
		cfg.Provider.Proxmox.APIToken = "cfg-token"
		cfg.Provider.Proxmox.Username = "cfg-user"
		cfg.Provider.Proxmox.Password = "cfg-pw"

		creds := GetProxmoxCredentials(cfg)

		if got := string(creds.APIToken); got != "env-token" {
			t.Errorf("APIToken = %q, want env-token", got)
		}
		if creds.Source != SourceEnv {
			t.Errorf("Source = %v, want SourceEnv", creds.Source)
		}
		if !creds.ConfigCredentialsOverridden {
			t.Errorf("ConfigCredentialsOverridden = false; expected true (config also had creds)")
		}
		if !creds.EndpointFromConfig {
			t.Errorf("EndpointFromConfig = false; expected true (PROXMOX_VE_ENDPOINT unset)")
		}
	})

	t.Run("env endpoint override sets EndpointFromConfig=false", func(t *testing.T) {
		clearProxmoxEnv(t)
		t.Setenv("PROXMOX_VE_API_TOKEN", "env-token")
		t.Setenv("PROXMOX_VE_ENDPOINT", "https://other.example:8006")

		creds := GetProxmoxCredentials(cfgWithHost())

		if creds.EndpointFromConfig {
			t.Errorf("EndpointFromConfig = true; expected false")
		}
		if creds.Endpoint != "https://other.example:8006" {
			t.Errorf("Endpoint = %q, want explicit env value", creds.Endpoint)
		}
	})

	t.Run("env username+password path", func(t *testing.T) {
		clearProxmoxEnv(t)
		t.Setenv("PROXMOX_VE_USERNAME", "root@pam")
		t.Setenv("PROXMOX_VE_PASSWORD", "pw")

		creds := GetProxmoxCredentials(cfgWithHost())

		if creds.Source != SourceEnv {
			t.Errorf("Source = %v, want SourceEnv", creds.Source)
		}
		if !bytes.Equal(creds.Password, []byte("pw")) {
			t.Errorf("Password = %q, want pw", creds.Password)
		}
	})

	t.Run("no env creds returns SourceNone even when config has credentials", func(t *testing.T) {
		clearProxmoxEnv(t)
		cfg := cfgWithHost()
		cfg.Provider.Proxmox.APIToken = "cfg-token"
		cfg.Provider.Proxmox.Password = "cfg-pw"
		cfg.Provider.Proxmox.Username = "cfg-user"

		creds := GetProxmoxCredentials(cfg)

		if creds.Source != SourceNone {
			t.Errorf("Source = %v, want SourceNone (config-file fallback removed)", creds.Source)
		}
		if len(creds.APIToken) != 0 {
			t.Errorf("APIToken should be empty without env creds; got %q", creds.APIToken)
		}
	})

	t.Run("host without scheme gets https prefix and :8006 port", func(t *testing.T) {
		clearProxmoxEnv(t)
		cfg := cfgWithHost() // host = "pve.example" (no scheme, no port)
		cfg.Provider.Proxmox.APIToken = "t"

		creds := GetProxmoxCredentials(cfg)

		if creds.Endpoint != "https://pve.example:8006" {
			t.Errorf("Endpoint = %q, want https://pve.example:8006", creds.Endpoint)
		}
	})
}

// clearProxmoxEnv ensures no PROXMOX_VE_* vars leak from the host shell into
// test scope. We Setenv first (so t auto-restores on test completion) then
// Unsetenv — Setenv("", "") leaves the var present-and-empty, which
// LookupEnv reports as ok=true and would flip Insecure logic.
func clearProxmoxEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"PROXMOX_VE_ENDPOINT",
		"PROXMOX_VE_USERNAME",
		"PROXMOX_VE_PASSWORD",
		"PROXMOX_VE_API_TOKEN",
		"PROXMOX_VE_INSECURE",
	} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
}

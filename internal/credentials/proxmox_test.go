package credentials

import (
	"fmt"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

func TestGetProxmoxCredentials(t *testing.T) {
	cfg := config.DefaultConfig()

	t.Run("token and tls flag come from the environment, endpoint from the config", func(t *testing.T) {
		t.Setenv(envProxmoxAPIToken, "root@pam!okdctl=secret")
		t.Setenv(envProxmoxInsecure, "true")

		creds := GetProxmoxCredentials(cfg)

		if !creds.IsValid() || string(creds.APIToken) != "root@pam!okdctl=secret" {
			t.Fatalf("APIToken = %q, want the environment's token", creds.APIToken)
		}
		if !creds.Insecure {
			t.Error("Insecure = false, want PROXMOX_VE_INSECURE=true honoured")
		}
		if creds.Endpoint != cfg.Provider.Proxmox.Endpoint {
			t.Errorf("Endpoint = %q, want %q from the config", creds.Endpoint, cfg.Provider.Proxmox.Endpoint)
		}
	})

	t.Run("no token in the environment is invalid", func(t *testing.T) {
		t.Setenv(envProxmoxAPIToken, "")
		t.Setenv(envProxmoxInsecure, "false")

		creds := GetProxmoxCredentials(cfg)

		if creds.IsValid() || creds.Insecure {
			t.Fatalf("creds = %+v, want no token and tls verification on", creds.Redacted())
		}
		if creds.Endpoint != cfg.Provider.Proxmox.Endpoint {
			t.Errorf("Endpoint = %q, want the config's even without a token", creds.Endpoint)
		}
	})

	t.Run("nil config keeps the token and leaves the endpoint empty", func(t *testing.T) {
		t.Setenv(envProxmoxAPIToken, "root@pam!okdctl=secret")

		creds := GetProxmoxCredentials(nil)

		if !creds.IsValid() || creds.Endpoint != "" {
			t.Fatalf("creds = %+v, want a token and no endpoint", creds.Redacted())
		}
	})
}

func TestProxmoxCredentialsMaskAndZeroize(t *testing.T) {
	creds := &ProxmoxCredentials{Endpoint: "https://pve:8006", APIToken: []byte("tok-secret"), Insecure: true}

	for name, rendered := range map[string]string{
		"String":   creds.String(),
		"%+v":      fmt.Sprintf("%+v", creds),
		"%#v":      fmt.Sprintf("%#v", creds),
		"Redacted": fmt.Sprint(creds.Redacted()),
	} {
		if strings.Contains(rendered, "tok-secret") {
			t.Errorf("%s leaks the token: %s", name, rendered)
		}
		if !strings.Contains(rendered, "https://pve:8006") {
			t.Errorf("%s dropped the endpoint: %s", name, rendered)
		}
	}

	alias := creds.APIToken
	creds.Zeroize()
	if creds.APIToken != nil || creds.IsValid() {
		t.Errorf("APIToken after Zeroize = %v, want nil", creds.APIToken)
	}
	for i, b := range alias {
		if b != 0 {
			t.Fatalf("token byte %d not zeroed", i)
		}
	}

	var none *ProxmoxCredentials
	none.Zeroize()
	if none.String() != "ProxmoxCredentials(nil)" || none.Redacted() != nil {
		t.Errorf("nil receiver: String = %q, Redacted = %v", none.String(), none.Redacted())
	}
}

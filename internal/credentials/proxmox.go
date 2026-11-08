// Package credentials resolves the Proxmox API token from the environment,
// okdctl.env included; okdctl.yaml carries the endpoint and nothing secret.
package credentials

import (
	"fmt"
	"os"

	"github.com/qxtaiba/okdctl/internal/config"
)

// Env-var names shared by the okdctl.env writer and reader.
const (
	envProxmoxAPIToken = "PROXMOX_VE_API_TOKEN" //nolint:gosec // G101: env-var name, not a credential value
	envProxmoxInsecure = "PROXMOX_VE_INSECURE"
)

// ProxmoxCredentials pairs the config's endpoint with the token and TLS
// flag from the environment; Zeroize the token after use.
type ProxmoxCredentials struct {
	Endpoint string
	APIToken []byte
	Insecure bool
}

// IsValid reports whether a token is present.
func (c *ProxmoxCredentials) IsValid() bool {
	return len(c.APIToken) > 0
}

// Zeroize overwrites the token with zeros and nils it.
func (c *ProxmoxCredentials) Zeroize() {
	if c == nil {
		return
	}
	clear(c.APIToken)
	c.APIToken = nil
}

type redactedCredentials struct {
	Endpoint string
	Insecure bool
}

// Redacted returns the non-secret fields, which logutil.RedactHandler logs
// in place of c.
func (c *ProxmoxCredentials) Redacted() any {
	if c == nil {
		return nil
	}
	return redactedCredentials{Endpoint: c.Endpoint, Insecure: c.Insecure}
}

// String masks the token so %v and %s cannot leak it.
func (c *ProxmoxCredentials) String() string {
	if c == nil {
		return "ProxmoxCredentials(nil)"
	}
	return fmt.Sprintf("%+v", c.Redacted())
}

// GoString mirrors String so %#v also masks the token.
func (c *ProxmoxCredentials) GoString() string {
	return c.String()
}

// GetProxmoxCredentials reads provider.proxmox.endpoint from cfg and the
// token and TLS flag from PROXMOX_VE_API_TOKEN and PROXMOX_VE_INSECURE; call
// LoadEnvFile first so okdctl.env is in the environment.
func GetProxmoxCredentials(cfg *config.Config) *ProxmoxCredentials {
	creds := &ProxmoxCredentials{Insecure: os.Getenv(envProxmoxInsecure) == "true"}
	if cfg != nil {
		creds.Endpoint = cfg.Provider.Proxmox.Endpoint
	}
	if token := os.Getenv(envProxmoxAPIToken); token != "" {
		creds.APIToken = []byte(token)
	}
	return creds
}

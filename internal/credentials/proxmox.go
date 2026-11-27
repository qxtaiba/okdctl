// Package credentials resolves the Proxmox API token from okdctl.env and the
// environment; okdctl.yaml carries the endpoint and nothing secret.
package credentials

import (
	"bytes"
	"fmt"
	"os"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// Env-var names shared by the okdctl.env writer and reader.
const (
	envProxmoxAPIToken = "PROXMOX_VE_API_TOKEN" //nolint:gosec // G101: env-var name, not a credential value
	envProxmoxInsecure = "PROXMOX_VE_INSECURE"
)

// ProxmoxCredentials pairs the endpoint with the token and TLS flag; Zeroize
// the token after use.
type ProxmoxCredentials struct {
	Endpoint string
	APIToken []byte
	Insecure bool
}

// IsValid reports whether a token is present.
func (c ProxmoxCredentials) IsValid() bool {
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
func (c ProxmoxCredentials) Redacted() any {
	return redactedCredentials{Endpoint: c.Endpoint, Insecure: c.Insecure}
}

// String masks the token so %v and %s cannot leak it.
func (c ProxmoxCredentials) String() string {
	return fmt.Sprintf("%+v", c.Redacted())
}

// GoString mirrors String so %#v also masks the token.
func (c ProxmoxCredentials) GoString() string {
	return c.String()
}

// FromEnv reads the token and TLS flag from PROXMOX_VE_API_TOKEN and
// PROXMOX_VE_INSECURE, ignoring any sidecar.
func FromEnv(endpoint string) *ProxmoxCredentials {
	creds := &ProxmoxCredentials{Endpoint: endpoint, Insecure: os.Getenv(envProxmoxInsecure) == "true"}
	if token := os.Getenv(envProxmoxAPIToken); token != "" {
		creds.APIToken = []byte(token)
	}
	return creds
}

// Load returns the sidecar at envPath overlaid by the environment: a
// non-empty PROXMOX_VE_API_TOKEN or PROXMOX_VE_INSECURE wins over the file's
// value, and a token that differs from the file's is reported by key.
func Load(envPath, endpoint string) (*ProxmoxCredentials, error) {
	creds, err := ReadEnvFile(envPath)
	if err != nil {
		return nil, err
	}
	creds.Endpoint = endpoint
	env := FromEnv(endpoint)
	if os.Getenv(envProxmoxInsecure) != "" {
		creds.Insecure = env.Insecure
	}
	if env.IsValid() {
		if creds.IsValid() && !bytes.Equal(creds.APIToken, env.APIToken) {
			logutil.Warn(envProxmoxAPIToken+" from the environment overrides the saved token", logutil.LF("path", envPath))
		}
		creds.Zeroize()
		creds.APIToken = env.APIToken
	}
	return creds, nil
}

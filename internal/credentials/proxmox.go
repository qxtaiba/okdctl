// Package credentials resolves the Proxmox API token from okdctl.env and the
// environment; okdctl.yaml carries the endpoint and nothing secret.
package credentials

import (
	"bytes"
	"fmt"
	"os"
	"strconv"

	"github.com/qxtaiba/okdctl/internal/errtypes"
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

func fromEnv(endpoint string) (*ProxmoxCredentials, error) {
	creds := &ProxmoxCredentials{Endpoint: endpoint}
	if v := os.Getenv(envProxmoxInsecure); v != "" {
		insecure, err := strconv.ParseBool(v)
		if err != nil {
			return nil, &errtypes.ConfigError{Msg: envProxmoxInsecure + " is not a boolean", Err: err}
		}
		creds.Insecure = insecure
	}
	if token := os.Getenv(envProxmoxAPIToken); token != "" {
		creds.APIToken = []byte(token)
	}
	return creds, nil
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
	env, err := fromEnv(endpoint)
	if err != nil {
		creds.Zeroize()
		return nil, err
	}
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

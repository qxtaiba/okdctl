// Package credentials provides credential management for infrastructure providers.
package credentials

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
)

type Source int

const (
	SourceNone Source = iota
	SourceEnv
	SourceConfig
)

func (s Source) String() string {
	switch s {
	case SourceEnv:
		return "environment variables"
	case SourceConfig:
		return "configuration file"
	default:
		return "not found"
	}
}

// ProxmoxCredentials holds Proxmox authentication material. Password and
// APIToken are []byte (not string) so they can be wiped with Zeroize once
// the caller is done — Go strings are immutable and can't be overwritten,
// which leaves secrets lingering in memory and in any stray %+v dump.
//
// EndpointFromConfig and ConfigCredentialsOverridden surface credential
// provenance mismatches that would otherwise be silent:
//   - EndpointFromConfig is true when Source == SourceEnv but the endpoint
//     fell back to the config file because PROXMOX_VE_ENDPOINT was unset.
//   - ConfigCredentialsOverridden is true when both the environment and
//     the config file held credentials and the environment won.
//
// The caller is expected to warn on either flag so operators are not
// surprised by "credentials from env" messages hiding a mixed source.
type ProxmoxCredentials struct {
	Endpoint                    string
	Username                    string
	Password                    []byte
	APIToken                    []byte
	Insecure                    bool
	Source                      Source
	EndpointFromConfig          bool
	ConfigCredentialsOverridden bool
}

// IsValid returns true if either username/password or API token is set.
func (c *ProxmoxCredentials) IsValid() bool {
	return (c.Username != "" && len(c.Password) > 0) || len(c.APIToken) > 0
}

func (c *ProxmoxCredentials) UseAPIToken() bool {
	return len(c.APIToken) > 0
}

// Zeroize overwrites the secret byte slices with zeros and nils them out.
// Call this (typically via defer) once the credentials have been consumed —
// e.g. after Env() has been snapshotted for a subprocess.
func (c *ProxmoxCredentials) Zeroize() {
	if c == nil {
		return
	}
	for i := range c.Password {
		c.Password[i] = 0
	}
	c.Password = nil
	for i := range c.APIToken {
		c.APIToken[i] = 0
	}
	c.APIToken = nil
}

// String masks secret fields so accidental %v / %s / log calls can't leak
// the password or token.
func (c *ProxmoxCredentials) String() string {
	if c == nil {
		return "ProxmoxCredentials(nil)"
	}
	return fmt.Sprintf(
		"ProxmoxCredentials{Endpoint: %s, Username: %s, Password: ***, APIToken: ***, Source: %s}",
		c.Endpoint, c.Username, c.Source,
	)
}

// GoString mirrors String so %#v also masks secrets.
func (c *ProxmoxCredentials) GoString() string {
	return c.String()
}

// Env returns credential env vars for subprocess execution, avoiding
// modification of the global process environment. The returned strings
// contain the secret bytes — they are copies, and the original byte
// slices remain wipeable via Zeroize.
func (c *ProxmoxCredentials) Env() []string {
	if !c.IsValid() {
		return nil
	}

	env := []string{"PROXMOX_VE_ENDPOINT=" + c.Endpoint}

	if c.UseAPIToken() {
		env = append(env, "PROXMOX_VE_API_TOKEN="+string(c.APIToken))
	} else {
		env = append(env,
			"PROXMOX_VE_USERNAME="+c.Username,
			"PROXMOX_VE_PASSWORD="+string(c.Password),
		)
	}

	if c.Insecure {
		env = append(env, "PROXMOX_VE_INSECURE=true")
	}

	return env
}

// configHasCredentials reports whether the config file carries a full
// credential set (API token or username+password). Used to detect when
// environment credentials silently override a populated config.
func configHasCredentials(px *config.ProxmoxConfig) bool {
	if px.APIToken != "" {
		return true
	}
	return px.Username != "" && px.Password != ""
}

func applyEnvSource(creds *ProxmoxCredentials, configHadCreds bool) {
	creds.Source = SourceEnv
	creds.ConfigCredentialsOverridden = configHadCreds
	if endpoint := os.Getenv("PROXMOX_VE_ENDPOINT"); endpoint != "" {
		creds.Endpoint = endpoint
	} else {
		creds.EndpointFromConfig = true
	}
	if v, ok := os.LookupEnv("PROXMOX_VE_INSECURE"); ok {
		creds.Insecure = v == "true"
	}
}

// GetProxmoxCredentials resolves credentials with priority:
// 1. Environment variables (incl. .env file), 2. Config file (legacy).
//
// When env credentials are used, two provenance flags surface mismatches
// the caller should warn about:
//   - EndpointFromConfig: PROXMOX_VE_ENDPOINT was unset, so the endpoint
//     still comes from the config file (mixed source).
//   - ConfigCredentialsOverridden: the config file also held credentials
//     and they were silently ignored in favour of the environment.
func GetProxmoxCredentials(cfg *config.Config) *ProxmoxCredentials {
	creds := &ProxmoxCredentials{
		Source: SourceNone,
	}

	if cfg == nil || cfg.Provider.Proxmox == nil {
		return creds
	}
	px := cfg.Provider.Proxmox

	host := px.Host
	if host == "" {
		return creds
	}

	if !strings.HasPrefix(host, "https://") && !strings.HasPrefix(host, "http://") {
		host = "https://" + host
	}
	if u, err := url.Parse(host); err == nil && u.Port() == "" {
		u.Host = u.Hostname() + ":8006"
		host = u.String()
	}
	creds.Endpoint = host
	creds.Insecure = px.Insecure

	configHadCreds := configHasCredentials(px)

	// Priority 1: Environment variables (includes values loaded from .env file)
	if token := os.Getenv("PROXMOX_VE_API_TOKEN"); token != "" {
		creds.APIToken = []byte(token)
		applyEnvSource(creds, configHadCreds)
		return creds
	}

	if username, password := os.Getenv("PROXMOX_VE_USERNAME"), os.Getenv("PROXMOX_VE_PASSWORD"); username != "" && password != "" {
		creds.Username = username
		creds.Password = []byte(password)
		applyEnvSource(creds, configHadCreds)
		return creds
	}

	// Priority 2: Config file fields (legacy support)
	if px.APIToken != "" {
		token := px.APIToken
		if strings.Contains(px.APIToken, "=") && px.TokenID != "" {
			// APIToken already contains the full "tokenid=secret" format;
			// ignore the separate TokenID to avoid "tokenid=tokenid=secret".
			_ = px.TokenID
		} else if px.TokenID != "" {
			token = px.TokenID + "=" + px.APIToken
		}
		creds.APIToken = []byte(token)
		creds.Source = SourceConfig
		return creds
	}

	if px.Username != "" && px.Password != "" {
		creds.Username = px.Username
		creds.Password = []byte(px.Password)
		creds.Source = SourceConfig
		return creds
	}

	return creds
}

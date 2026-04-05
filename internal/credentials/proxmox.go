// Package credentials provides credential management for infrastructure providers.
package credentials

import (
	"fmt"
	"os"
	"strings"
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
		env = append(env, "PROXMOX_VE_USERNAME="+c.Username)
		env = append(env, "PROXMOX_VE_PASSWORD="+string(c.Password))
	}

	if c.Insecure {
		env = append(env, "PROXMOX_VE_INSECURE=true")
	}

	return env
}

// ProxmoxConfigProvider abstracts config access to avoid importing the config package.
type ProxmoxConfigProvider interface {
	GetProxmoxHost() string
	GetProxmoxInsecure() bool
	GetProxmoxAPIToken() string
	GetProxmoxTokenID() string
	GetProxmoxUsername() string
	GetProxmoxPassword() string
}

// configHasCredentials reports whether the config file carries a full
// credential set (API token or username+password). Used to detect when
// environment credentials silently override a populated config.
func configHasCredentials(cfg ProxmoxConfigProvider) bool {
	if cfg.GetProxmoxAPIToken() != "" {
		return true
	}
	return cfg.GetProxmoxUsername() != "" && cfg.GetProxmoxPassword() != ""
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
func GetProxmoxCredentials(cfg ProxmoxConfigProvider) *ProxmoxCredentials {
	creds := &ProxmoxCredentials{
		Source: SourceNone,
	}

	applyInsecureOverride := func(creds *ProxmoxCredentials) {
		if os.Getenv("PROXMOX_VE_INSECURE") == "true" {
			creds.Insecure = true
		}
	}

	if cfg == nil {
		return creds
	}

	host := cfg.GetProxmoxHost()
	if host == "" {
		return creds
	}

	if !strings.HasPrefix(host, "http") {
		host = "https://" + host
	}
	hostPart := strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	if !strings.Contains(hostPart, ":") {
		host = host + ":8006"
	}
	creds.Endpoint = host
	creds.Insecure = cfg.GetProxmoxInsecure()

	configHadCreds := configHasCredentials(cfg)

	// Priority 1: Environment variables (includes values loaded from .env file)
	if token := os.Getenv("PROXMOX_VE_API_TOKEN"); token != "" {
		creds.APIToken = []byte(token)
		creds.Source = SourceEnv
		creds.ConfigCredentialsOverridden = configHadCreds
		if endpoint := os.Getenv("PROXMOX_VE_ENDPOINT"); endpoint != "" {
			creds.Endpoint = endpoint
		} else {
			creds.EndpointFromConfig = true
		}
		applyInsecureOverride(creds)
		return creds
	}

	if username, password := os.Getenv("PROXMOX_VE_USERNAME"), os.Getenv("PROXMOX_VE_PASSWORD"); username != "" && password != "" {
		creds.Username = username
		creds.Password = []byte(password)
		creds.Source = SourceEnv
		creds.ConfigCredentialsOverridden = configHadCreds
		if endpoint := os.Getenv("PROXMOX_VE_ENDPOINT"); endpoint != "" {
			creds.Endpoint = endpoint
		} else {
			creds.EndpointFromConfig = true
		}
		applyInsecureOverride(creds)
		return creds
	}

	// Priority 2: Config file fields (legacy support)
	if apiToken := cfg.GetProxmoxAPIToken(); apiToken != "" {
		token := apiToken
		if tokenID := cfg.GetProxmoxTokenID(); tokenID != "" {
			token = tokenID + "=" + apiToken
		}
		creds.APIToken = []byte(token)
		creds.Source = SourceConfig
		return creds
	}

	if username, password := cfg.GetProxmoxUsername(), cfg.GetProxmoxPassword(); username != "" && password != "" {
		creds.Username = username
		creds.Password = []byte(password)
		creds.Source = SourceConfig
		return creds
	}

	return creds
}

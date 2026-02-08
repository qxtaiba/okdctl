// Package credentials provides credential management for infrastructure providers.
package credentials

import (
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

type ProxmoxCredentials struct {
	Endpoint string
	Username string
	Password string
	APIToken string
	Insecure bool
	Source   Source
}

// IsValid returns true if either username/password or API token is set.
func (c *ProxmoxCredentials) IsValid() bool {
	return (c.Username != "" && c.Password != "") || c.APIToken != ""
}

func (c *ProxmoxCredentials) UseAPIToken() bool {
	return c.APIToken != ""
}

// Env returns credential env vars for subprocess execution, avoiding
// modification of the global process environment.
func (c *ProxmoxCredentials) Env() []string {
	if !c.IsValid() {
		return nil
	}

	env := []string{"PROXMOX_VE_ENDPOINT=" + c.Endpoint}

	if c.UseAPIToken() {
		env = append(env, "PROXMOX_VE_API_TOKEN="+c.APIToken)
	} else {
		env = append(env, "PROXMOX_VE_USERNAME="+c.Username)
		env = append(env, "PROXMOX_VE_PASSWORD="+c.Password)
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

// GetProxmoxCredentials resolves credentials with priority:
// 1. Environment variables (incl. .env file), 2. Config file (legacy).
func GetProxmoxCredentials(cfg ProxmoxConfigProvider) *ProxmoxCredentials {
	creds := &ProxmoxCredentials{
		Source: SourceNone,
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

	// Priority 1: Environment variables (includes values loaded from .env file)
	if token := os.Getenv("PROXMOX_VE_API_TOKEN"); token != "" {
		creds.APIToken = token
		creds.Source = SourceEnv
		if endpoint := os.Getenv("PROXMOX_VE_ENDPOINT"); endpoint != "" {
			creds.Endpoint = endpoint
		}
		if insecure := os.Getenv("PROXMOX_VE_INSECURE"); insecure == "true" {
			creds.Insecure = true
		}
		return creds
	}

	if username, password := os.Getenv("PROXMOX_VE_USERNAME"), os.Getenv("PROXMOX_VE_PASSWORD"); username != "" && password != "" {
		creds.Username = username
		creds.Password = password
		creds.Source = SourceEnv
		if endpoint := os.Getenv("PROXMOX_VE_ENDPOINT"); endpoint != "" {
			creds.Endpoint = endpoint
		}
		if insecure := os.Getenv("PROXMOX_VE_INSECURE"); insecure == "true" {
			creds.Insecure = true
		}
		return creds
	}

	// Priority 2: Config file fields (legacy support)
	if apiToken := cfg.GetProxmoxAPIToken(); apiToken != "" {
		creds.APIToken = apiToken
		if tokenID := cfg.GetProxmoxTokenID(); tokenID != "" {
			creds.APIToken = tokenID + "=" + apiToken
		}
		creds.Source = SourceConfig
		return creds
	}

	if username := cfg.GetProxmoxUsername(); username != "" {
		if password := cfg.GetProxmoxPassword(); password != "" {
			creds.Username = username
			creds.Password = password
			creds.Source = SourceConfig
			return creds
		}
	}

	return creds
}


// Package credentials provides credential management for infrastructure providers.
package credentials

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
)

// Proxmox env-var names consumed by the bpg/proxmox terraform provider and
// read by GetProxmoxCredentials. Writes (envfile.go) and reads (proxmox.go)
// must use the same identifiers — a typo on either side silently breaks
// env-file round-trip, so both sites reference these constants.
const (
	envProxmoxEndpoint = "PROXMOX_VE_ENDPOINT"
	envProxmoxUsername = "PROXMOX_VE_USERNAME"
	envProxmoxPassword = "PROXMOX_VE_PASSWORD" //nolint:gosec // G101: env-var name, not a credential value
	envProxmoxAPIToken = "PROXMOX_VE_API_TOKEN" //nolint:gosec // G101: env-var name, not a credential value
	envProxmoxInsecure = "PROXMOX_VE_INSECURE"
)

// Source tracks where a credential came from so the CLI can warn on mixed-
// provenance situations (env overriding config silently).
type Source int

const (
	// SourceNone means no credential was located.
	SourceNone Source = iota
	// SourceEnv means PROXMOX_VE_PASSWORD / PROXMOX_VE_API_TOKEN supplied it.
	SourceEnv
	// SourceConfig means the credential came from the YAML config file.
	SourceConfig
)

// String returns a human-readable label used in status output.
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

// UseAPIToken reports whether API-token auth should be used. Callers branch
// on this to decide between basic-auth (username+password) and bearer-token
// flows without inspecting the token field directly.
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
	clear(c.Password)
	c.Password = nil
	clear(c.APIToken)
	c.APIToken = nil
}

// redactedCredentials is the safe projection of ProxmoxCredentials returned
// by Redacted(). It omits Password and APIToken so that any code path that
// receives a ProxmoxCredentials value — including slog's redactAny switch —
// cannot reach the secret bytes. Future safe fields belong here; future
// secret fields must be omitted.
type redactedCredentials struct {
	Endpoint                    string
	Username                    string
	Insecure                    bool
	Source                      Source
	EndpointFromConfig          bool
	ConfigCredentialsOverridden bool
}

// Redacted returns a struct that contains only the non-secret fields of c,
// satisfying the interface{ Redacted() any } that logutil.redactAny detects.
// This makes credential redaction structural: new secret fields default to
// absent from the safe view unless explicitly added here.
func (c *ProxmoxCredentials) Redacted() any {
	if c == nil {
		return nil
	}
	return redactedCredentials{
		Endpoint:                    c.Endpoint,
		Username:                    c.Username,
		Insecure:                    c.Insecure,
		Source:                      c.Source,
		EndpointFromConfig:          c.EndpointFromConfig,
		ConfigCredentialsOverridden: c.ConfigCredentialsOverridden,
	}
}

// String masks secret fields so accidental %v / %s / log calls can't leak
// the password or token.
func (c *ProxmoxCredentials) String() string {
	if c == nil {
		return "ProxmoxCredentials(nil)"
	}
	return fmt.Sprintf("%+v", c.Redacted())
}

// GoString mirrors String so %#v also masks secrets.
func (c *ProxmoxCredentials) GoString() string {
	return c.String()
}

// Env returns credential env vars for subprocess execution, avoiding
// modification of the global process environment.
//
// os/exec requires []string for cmd.Env, so each secret byte slice is
// converted to an immutable Go string that Zeroize cannot overwrite.
// Callers MUST not retain the returned slice beyond the cmd.Run (or
// equivalent) call — pass it directly to WithEnv and let it go out of
// scope. The source []byte fields remain wipeable via Zeroize.
func (c *ProxmoxCredentials) Env() []string {
	if !c.IsValid() {
		return nil
	}

	env := []string{envProxmoxEndpoint + "=" + c.Endpoint}

	if c.UseAPIToken() {
		env = append(env, envProxmoxAPIToken+"="+string(c.APIToken))
	} else {
		env = append(env,
			envProxmoxUsername+"="+c.Username,
			envProxmoxPassword+"="+string(c.Password),
		)
	}

	if c.Insecure {
		env = append(env, envProxmoxInsecure+"=true")
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
	if endpoint := os.Getenv(envProxmoxEndpoint); endpoint != "" {
		creds.Endpoint = endpoint
	} else {
		creds.EndpointFromConfig = true
	}
	if v, ok := os.LookupEnv(envProxmoxInsecure); ok {
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
	if token := os.Getenv(envProxmoxAPIToken); token != "" {
		creds.APIToken = []byte(token)
		applyEnvSource(creds, configHadCreds)
		return creds
	}

	if username, password := os.Getenv(envProxmoxUsername), os.Getenv(envProxmoxPassword); username != "" && password != "" {
		creds.Username = username
		creds.Password = []byte(password)
		applyEnvSource(creds, configHadCreds)
		return creds
	}

	// Config-file credentials are NO LONGER a fallback. The okdctl design is
	// env/.env-only so string residue of px.Password / px.APIToken does not
	// linger in heap for the Config's lifetime. configHasCredentials still
	// reads the fields to set ConfigCredentialsOverridden (a provenance flag,
	// not a credential copy).
	return creds
}

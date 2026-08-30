// Package credentials owns the Proxmox credential lifecycle: secret fields
// are []byte so callers can Zeroize them, and Redacted() keeps them out of
// structured logs. Call LoadEnvFile before GetProxmoxCredentials —
// credentials resolve from env vars only, never the config file.
package credentials

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
)

// Proxmox env-var names consumed by the bpg/proxmox terraform provider;
// envfile.go writes and proxmox.go reads must share these constants to
// avoid a silent round-trip break.
const (
	envProxmoxEndpoint = "PROXMOX_VE_ENDPOINT"
	envProxmoxUsername = "PROXMOX_VE_USERNAME"
	envProxmoxPassword = "PROXMOX_VE_PASSWORD"  //nolint:gosec // G101: env-var name, not a credential value
	envProxmoxAPIToken = "PROXMOX_VE_API_TOKEN" //nolint:gosec // G101: env-var name, not a credential value
	envProxmoxInsecure = "PROXMOX_VE_INSECURE"
)

// Source tracks where a credential came from so the CLI can warn on mixed-provenance situations.
type Source int

const (
	// SourceNone means no credential was located.
	SourceNone Source = iota
	// SourceEnv means PROXMOX_VE_PASSWORD / PROXMOX_VE_API_TOKEN supplied it.
	SourceEnv
)

// String returns a human-readable label used in status output.
func (s Source) String() string {
	if s == SourceEnv {
		return "environment variables"
	}
	return "not found"
}

// ProxmoxCredentials holds Proxmox authentication material; Password and
// APIToken are []byte so they can be wiped with Zeroize rather than
// lingering as immutable strings. EndpointFromConfig and
// ConfigCredentialsOverridden surface silent provenance mismatches the
// caller should warn on.
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

// UseAPIToken reports whether an API token is present — token auth wins
// over username/password when it is.
func (c *ProxmoxCredentials) UseAPIToken() bool {
	return len(c.APIToken) > 0
}

// Zeroize overwrites the secret byte slices with zeros and nils them. Call
// it (typically via defer) after Env() has been snapshotted for a subprocess.
func (c *ProxmoxCredentials) Zeroize() {
	if c == nil {
		return
	}
	clear(c.Password)
	c.Password = nil
	clear(c.APIToken)
	c.APIToken = nil
}

// redactedCredentials is the safe Redacted() projection: omits Password and
// APIToken so slog's redactAny switch never reaches secret bytes.
type redactedCredentials struct {
	Endpoint                    string
	Username                    string
	Insecure                    bool
	Source                      Source
	EndpointFromConfig          bool
	ConfigCredentialsOverridden bool
}

// Redacted returns only the non-secret fields of c, satisfying the
// interface{ Redacted() any } that logutil.redactAny detects — new secret
// fields default to absent from this view unless explicitly added.
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

// String masks secret fields so accidental %v/%s/log calls can't leak the password or token.
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

// Env returns credential env vars for subprocess execution without
// touching the global process environment. The returned strings are
// immutable copies that Zeroize cannot wipe, so callers MUST pass the
// result directly into a WithEnv option in the same frame as defer
// Zeroize() and MUST NOT let it outlive the call stack (goroutine, cache,
// persistent config). TestEnvCallSiteRegistry enforces this statically.
func (c *ProxmoxCredentials) Env() []string {
	if !c.IsValid() {
		return nil
	}

	env := []string{envProxmoxEndpoint + "=" + c.Endpoint}

	if c.UseAPIToken() {
		env = append(env, envProxmoxAPIToken+"="+string(c.APIToken))
	} else {
		env = append(
			env,
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
// credential set (API token or username+password).
func configHasCredentials(px *config.ProxmoxConfig) bool {
	if !px.APIToken.IsEmpty() {
		return true
	}
	return px.Username != "" && !px.Password.IsEmpty()
}

// normalizeEndpoint applies the scheme guard shared by a config host and
// PROXMOX_VE_ENDPOINT: http:// is refused unless allowHTTP, schemeless hosts
// get an https:// prefix, and portless hosts default to :8006. ok is false
// only on a refused http:// endpoint — the caller must not attach plaintext
// credentials then.
func normalizeEndpoint(host string, allowHTTP bool) (endpoint string, ok bool) {
	if strings.HasPrefix(host, "http://") && !allowHTTP {
		return "", false
	}
	if !strings.HasPrefix(host, "https://") && !strings.HasPrefix(host, "http://") {
		host = "https://" + host
	}
	if u, err := url.Parse(host); err == nil && u.Port() == "" {
		u.Host = u.Hostname() + ":8006"
		host = u.String()
	}
	return host, true
}

// applyEnvSource records env provenance and, when PROXMOX_VE_ENDPOINT is set,
// re-resolves it through the same scheme guard as the config host; returns
// false when http:// lacks the insecure_http opt-in, so an env override
// can't bypass the config-file gate.
func applyEnvSource(creds *ProxmoxCredentials, configHadCreds, allowHTTP bool) bool {
	creds.Source = SourceEnv
	creds.ConfigCredentialsOverridden = configHadCreds
	if endpoint := os.Getenv(envProxmoxEndpoint); endpoint != "" {
		normalized, ok := normalizeEndpoint(endpoint, allowHTTP)
		if !ok {
			return false
		}
		creds.Endpoint = normalized
	} else {
		creds.EndpointFromConfig = true
	}
	if v, ok := os.LookupEnv(envProxmoxInsecure); ok {
		creds.Insecure = v == "true"
	}
	return true
}

// GetProxmoxCredentials resolves credentials from environment variables
// (including a loaded .env file) only — config-file credentials are not a
// fallback. Two provenance flags, EndpointFromConfig and
// ConfigCredentialsOverridden, surface silent mismatches the caller should
// warn about.
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

	// Validators already reject http:// without insecure_http; enforce it here
	// too in case a caller skipped validation.
	endpoint, ok := normalizeEndpoint(host, px.InsecureHTTP)
	if !ok {
		return creds
	}
	creds.Endpoint = endpoint
	creds.Insecure = px.Insecure

	configHadCreds := configHasCredentials(px)

	token := os.Getenv(envProxmoxAPIToken)
	username, password := os.Getenv(envProxmoxUsername), os.Getenv(envProxmoxPassword)
	switch {
	case token != "":
		creds.APIToken = []byte(token)
	case username != "" && password != "":
		creds.Username = username
		creds.Password = []byte(password)
	default:
		// Config-file credentials are not a fallback — env/.env-only avoids
		// leaving password/token string residue in heap for the Config's
		// lifetime; configHasCredentials only reads the fields to set the
		// provenance flag.
		return creds
	}

	if !applyEnvSource(creds, configHadCreds, px.InsecureHTTP) {
		creds.Zeroize()
		return &ProxmoxCredentials{Source: SourceNone}
	}
	return creds
}

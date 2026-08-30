package secretstore

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/system"
)

// validVaultName limits vault names to a safe charset (defense in depth atop escaping).
var validVaultName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// validateHTTPURL returns an error string for a malformed http/https URL, or "" when valid.
func validateHTTPURL(field, raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Sprintf("%s must be a valid http:// or https:// URL, got %q", field, raw)
	}
	return ""
}

// provider is the per-ESO-backend contract; buildResources must return auth
// Secrets before the SecretStore CRD.
type provider interface {
	validate(s Settings) []string
	buildResources(ctx context.Context, env *addon.Environment, s Settings) ([]string, error)
	secretNames() []string
}

// providerKind enumerates supported ESO backends; values are the lowercase wire
// strings from settings.
type providerKind string

const (
	providerOnepassword providerKind = "onepassword"
	providerVault       providerKind = "vault"
	providerBitwarden   providerKind = "bitwarden"
)

var providers = map[providerKind]provider{
	providerOnepassword: &onepasswordProvider{},
	providerVault:       &vaultProvider{},
	providerBitwarden:   &bitwardenProvider{},
}

// resolveProvider looks up the provider for the "provider" setting, defaulting
// to onepassword when unset.
func resolveProvider(settings map[string]string) (impl provider, kind providerKind) {
	kind = providerKind(settings[SettingProvider])
	if kind == "" {
		kind = providerOnepassword
	}
	p, ok := providers[kind]
	if !ok {
		return nil, kind
	}
	return p, kind
}

// Manifest field keys shared across provider builders; constants to keep goconst quiet.
const (
	mfName     = "name"
	mfAuth     = "auth"
	mfKey      = "key"
	mfTokenVal = "token"
)

const (
	opCredentialsFile = "1password-credentials.json"
	opTokenFile       = "1password-token.txt"

	opCredentialsSecretName = "onepassword-connect-credentials"
	opTokenSecretName       = "onepassword-connect-token"

	defaultOPConnectHost = "http://onepassword-connect:8080"
	defaultOPVaults      = "homelab=1"

	vaultTokenFile       = "vault-token.txt" //nolint:gosec // filename, not a credential
	vaultTokenSecretName = "vault-token"

	bitwardenTokenFile       = "bitwarden-token.txt"
	bitwardenTokenSecretName = "bitwarden-access-token"

	defaultBitwardenAPIURL       = "https://api.bitwarden.com"
	defaultBitwardenIdentityURL  = "https://identity.bitwarden.com"
	defaultBitwardenSDKServerURL = "https://bitwarden-sdk-server.external-secrets.svc.cluster.local:9998"
)

type onepasswordProvider struct{}

func (p *onepasswordProvider) validate(s Settings) []string {
	if s.OnePassword == nil {
		return nil
	}
	var errs []string
	if e := validateHTTPURL("onepassword_connect_host", s.OnePassword.ConnectHost); e != "" {
		errs = append(errs, e)
	}
	// Defensive re-check: decode-time validation (parseOnepasswordVaults) could
	// be bypassed by a hand-built Settings.
	for name := range s.OnePassword.Vaults {
		if !validVaultName.MatchString(name) {
			errs = append(errs, fmt.Sprintf("onepassword vault name %q is invalid (allowed: alphanumeric, ., _, -)", name))
		}
	}
	return errs
}

func (p *onepasswordProvider) secretNames() []string {
	return []string{opCredentialsSecretName, opTokenSecretName}
}

func (p *onepasswordProvider) buildResources(ctx context.Context, env *addon.Environment, s Settings) ([]string, error) {
	dir := resolveSecretsDir(env)
	credPath := filepath.Join(dir, opCredentialsFile)
	tokenPath := filepath.Join(dir, opTokenFile)

	var manifests []string

	if system.FileExists(credPath) {
		m, err := secretManifestFromFile(ctx, env, credPath, opCredentialsSecretName, "credentials_base64")
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, m)
		env.Logger.Info("secretstore: onepassword credentials secret prepared")
	}

	if system.FileExists(tokenPath) {
		m, err := secretManifestFromFile(ctx, env, tokenPath, opTokenSecretName, "token")
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, m)
		env.Logger.Info("secretstore: onepassword token secret prepared")
	}

	crd, err := buildOPSecretStoreCRD(s.OnePassword.ConnectHost, s.OnePassword.Vaults)
	if err != nil {
		return nil, err
	}
	manifests = append(manifests, crd)
	return manifests, nil
}

// parseOnepasswordVaults parses "name=priority,..." CSV (settings values are
// strings); empty input yields the default single vault.
func parseOnepasswordVaults(input string) (map[string]int, error) {
	if strings.TrimSpace(input) == "" {
		return map[string]int{"homelab": 1}, nil
	}
	out := make(map[string]int)
	for pair := range strings.SplitSeq(input, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		eq := strings.IndexByte(pair, '=')
		if eq < 0 {
			return nil, fmt.Errorf("invalid onepassword_vaults entry %q: expected name=priority", pair)
		}
		name := strings.TrimSpace(pair[:eq])
		priStr := strings.TrimSpace(pair[eq+1:])
		if name == "" {
			return nil, fmt.Errorf("invalid onepassword_vaults entry %q: empty vault name", pair)
		}
		if !validVaultName.MatchString(name) {
			return nil, fmt.Errorf("invalid onepassword_vaults vault name %q: allowed characters are alphanumeric, ., _, -", name)
		}
		pri, err := strconv.Atoi(priStr)
		if err != nil {
			return nil, fmt.Errorf("invalid onepassword_vaults priority %q in entry %q: %w", priStr, pair, err)
		}
		out[name] = pri
	}
	if len(out) == 0 {
		return map[string]int{"homelab": 1}, nil
	}
	return out, nil
}

// secretStoreManifest marshals an ESO SecretStore CRD via sigs.k8s.io/yaml,
// which escapes values and blocks YAML injection from settings.
func secretStoreManifest(providerBlock map[string]any) (string, error) {
	doc := map[string]any{
		"apiVersion": "external-secrets.io/v1beta1",
		"kind":       "SecretStore",
		"metadata": map[string]any{
			mfName:      esoSecretStoreName,
			"namespace": defaultNamespace,
		},
		"spec": map[string]any{
			"provider": providerBlock,
		},
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal secretstore manifest: %w", err)
	}
	return string(out), nil
}

func buildOPSecretStoreCRD(connectHost string, vaults map[string]int) (string, error) {
	return secretStoreManifest(map[string]any{
		"onepassword": map[string]any{
			"connectHost": connectHost,
			"vaults":      vaults,
			mfAuth: map[string]any{
				"secretRef": map[string]any{
					"connectTokenSecretRef": map[string]any{
						mfName: opTokenSecretName,
						mfKey:  mfTokenVal,
					},
				},
			},
		},
	})
}

type vaultProvider struct{}

func (p *vaultProvider) validate(s Settings) []string {
	var errs []string
	srv := s.Vault.Server
	if srv == "" {
		errs = append(errs, "vault_server is required for the vault provider (e.g. https://vault.example.com)")
	} else if e := validateHTTPURL("vault_server", srv); e != "" {
		errs = append(errs, e)
	}
	return errs
}

func (p *vaultProvider) secretNames() []string { return []string{vaultTokenSecretName} }

func (p *vaultProvider) buildResources(ctx context.Context, env *addon.Environment, s Settings) ([]string, error) {
	dir := resolveSecretsDir(env)
	tokenPath := filepath.Join(dir, vaultTokenFile)

	tokenManifest, err := secretManifestFromFile(ctx, env, tokenPath, vaultTokenSecretName, "token")
	if err != nil {
		return nil, err
	}
	env.Logger.Info("secretstore: vault token secret prepared")

	crd, err := buildVaultSecretStoreCRD(s.Vault.Server, s.Vault.Path, s.Vault.Version)
	if err != nil {
		return nil, err
	}
	return []string{tokenManifest, crd}, nil
}

func buildVaultSecretStoreCRD(server, path, version string) (string, error) {
	return secretStoreManifest(map[string]any{
		"vault": map[string]any{
			"server":  server,
			"path":    path,
			"version": version,
			mfAuth: map[string]any{
				"tokenSecretRef": map[string]any{
					mfName: vaultTokenSecretName,
					mfKey:  mfTokenVal,
				},
			},
		},
	})
}

// bitwardenProvider backs ESO's bitwardensecretsmanager, supporting Bitwarden
// SaaS or self-hosted Vaultwarden; the required bitwarden-sdk-server sidecar is
// not provisioned here.
type bitwardenProvider struct{}

func (p *bitwardenProvider) validate(s Settings) []string {
	var errs []string
	if s.Bitwarden.OrganizationID == "" {
		errs = append(errs, "bitwarden_organization_id is required for the bitwarden provider")
	}
	if s.Bitwarden.ProjectID == "" {
		errs = append(errs, "bitwarden_project_id is required for the bitwarden provider")
	}
	for _, u := range []struct{ field, val string }{
		{"bitwarden_api_url", s.Bitwarden.APIURL},
		{"bitwarden_identity_url", s.Bitwarden.IdentityURL},
		{"bitwarden_sdk_server_url", s.Bitwarden.SDKServerURL},
	} {
		if e := validateHTTPURL(u.field, u.val); e != "" {
			errs = append(errs, e)
		}
	}
	return errs
}

func (p *bitwardenProvider) secretNames() []string { return []string{bitwardenTokenSecretName} }

func (p *bitwardenProvider) buildResources(ctx context.Context, env *addon.Environment, s Settings) ([]string, error) {
	dir := resolveSecretsDir(env)
	tokenPath := filepath.Join(dir, bitwardenTokenFile)

	tokenManifest, err := secretManifestFromFile(ctx, env, tokenPath, bitwardenTokenSecretName, "token")
	if err != nil {
		return nil, err
	}
	env.Logger.Info("secretstore: bitwarden access-token secret prepared")

	crd, err := buildBitwardenSecretStoreCRD(
		s.Bitwarden.APIURL, s.Bitwarden.IdentityURL, s.Bitwarden.SDKServerURL,
		s.Bitwarden.OrganizationID, s.Bitwarden.ProjectID,
	)
	if err != nil {
		return nil, err
	}
	return []string{tokenManifest, crd}, nil
}

func buildBitwardenSecretStoreCRD(apiURL, identityURL, sdkServerURL, orgID, projectID string) (string, error) {
	return secretStoreManifest(map[string]any{
		"bitwardensecretsmanager": map[string]any{
			"apiURL":                apiURL,
			"identityURL":           identityURL,
			"bitwardenServerSDKURL": sdkServerURL,
			"organizationID":        orgID,
			"projectID":             projectID,
			mfAuth: map[string]any{
				"secretRef": map[string]any{
					"credentials": map[string]any{
						mfName: bitwardenTokenSecretName,
						mfKey:  mfTokenVal,
					},
				},
			},
		},
	})
}

func settingOrDefault(settings map[string]string, key, fallback string) string {
	if v := settings[key]; v != "" {
		return v
	}
	return fallback
}

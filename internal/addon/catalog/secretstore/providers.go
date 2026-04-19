package secretstore

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/system"
)

// provider is the internal contract each ESO backend must satisfy.
// buildResources returns ordered YAML manifests (auth Secrets first, then
// the ESO SecretStore CRD) to apply via `oc apply`. secretNames returns the
// Opaque Secret names the provider creates — used by Verify and Uninstall.
type provider interface {
	validate(s Settings) []string
	buildResources(ctx context.Context, env *addon.Environment, s Settings) ([]string, error)
	secretNames() []string
}

// Provider values accepted by the secretstore addon.
const (
	providerOnepassword = "onepassword"
	providerVault       = "vault"
	providerBitwarden   = "bitwarden"
)

var providers = map[string]provider{
	providerOnepassword: &onepasswordProvider{},
	providerVault:       &vaultProvider{},
	providerBitwarden:   &bitwardenProvider{},
}

// resolveProvider returns the provider implementation for the `provider`
// setting, defaulting to "onepassword" when unset. The second return value
// is the resolved provider name (useful for error messages on miss).
func resolveProvider(settings map[string]string) (impl provider, name string) {
	name = settings[SettingProvider]
	if name == "" {
		name = providerOnepassword
	}
	p, ok := providers[name]
	if !ok {
		return nil, name
	}
	return p, name
}

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

func (p *onepasswordProvider) validate(_ Settings) []string {
	return nil
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

	manifests = append(manifests, buildOPSecretStoreCRD(s.OnePassword.ConnectHost, s.OnePassword.Vaults))
	return manifests, nil
}

// parseOnepasswordVaults parses the `onepassword_vaults` setting. The wire
// format is CSV of `name=priority` pairs (e.g. "homelab=1,shared=2") since
// the addon settings map is map[string]string. Empty input falls back to
// the default single-vault shape.
func parseOnepasswordVaults(input string) (map[string]int, error) {
	if strings.TrimSpace(input) == "" {
		return map[string]int{"homelab": 1}, nil
	}
	out := make(map[string]int)
	for _, pair := range strings.Split(input, ",") {
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

func buildOPSecretStoreCRD(connectHost string, vaults map[string]int) string {
	var vaultLines strings.Builder
	for _, name := range slices.Sorted(maps.Keys(vaults)) {
		fmt.Fprintf(&vaultLines, "        %s: %d\n", name, vaults[name])
	}
	return fmt.Sprintf(`apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: %s
  namespace: %s
spec:
  provider:
    onepassword:
      connectHost: %s
      vaults:
%s      auth:
        secretRef:
          connectTokenSecretRef:
            name: %s
            key: token
`, esoSecretStoreName, defaultNamespace, connectHost, vaultLines.String(), opTokenSecretName)
}

type vaultProvider struct{}

func (p *vaultProvider) validate(s Settings) []string {
	var errs []string
	srv := s.Vault.Server
	if srv == "" {
		errs = append(errs, "vault_server is required for the vault provider (e.g. https://vault.example.com)")
	} else if !strings.HasPrefix(srv, "http://") && !strings.HasPrefix(srv, "https://") {
		errs = append(errs, "vault_server must start with http:// or https://")
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

	return []string{tokenManifest, buildVaultSecretStoreCRD(s.Vault.Server, s.Vault.Path, s.Vault.Version)}, nil
}

func buildVaultSecretStoreCRD(server, path, version string) string {
	return fmt.Sprintf(`apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: %s
  namespace: %s
spec:
  provider:
    vault:
      server: %s
      path: %s
      version: %s
      auth:
        tokenSecretRef:
          name: %s
          key: token
`, esoSecretStoreName, defaultNamespace, server, path, version, vaultTokenSecretName)
}

// bitwardenProvider backs ESO's bitwardensecretsmanager provider. It works
// against Bitwarden Secrets Manager SaaS as well as self-hosted Vaultwarden
// (point the three URL settings at the self-hosted endpoints). Requires a
// machine-account access token in bitwarden-token.txt and an in-cluster
// bitwarden-sdk-server sidecar (ESO ships one; not provisioned here).
type bitwardenProvider struct{}

func (p *bitwardenProvider) validate(s Settings) []string {
	var errs []string
	if s.Bitwarden.OrganizationID == "" {
		errs = append(errs, "bitwarden_organization_id is required for the bitwarden provider")
	}
	if s.Bitwarden.ProjectID == "" {
		errs = append(errs, "bitwarden_project_id is required for the bitwarden provider")
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

	return []string{tokenManifest, buildBitwardenSecretStoreCRD(
		s.Bitwarden.APIURL, s.Bitwarden.IdentityURL, s.Bitwarden.SDKServerURL,
		s.Bitwarden.OrganizationID, s.Bitwarden.ProjectID,
	)}, nil
}

func buildBitwardenSecretStoreCRD(apiURL, identityURL, sdkServerURL, orgID, projectID string) string {
	return fmt.Sprintf(`apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: %s
  namespace: %s
spec:
  provider:
    bitwardensecretsmanager:
      apiURL: %s
      identityURL: %s
      bitwardenServerSDKURL: %s
      organizationID: %s
      projectID: %s
      auth:
        secretRef:
          credentials:
            name: %s
            key: token
`, esoSecretStoreName, defaultNamespace, apiURL, identityURL, sdkServerURL, orgID, projectID, bitwardenTokenSecretName)
}

func settingOrDefault(settings map[string]string, key, fallback string) string {
	if v := settings[key]; v != "" {
		return v
	}
	return fallback
}

// Package secretstore provides the External Secrets Operator secret-bootstrap
// addon. It supports multiple ESO backends (onepassword, vault, bitwarden) via
// a provider setting and applies both auth Secrets and an ESO SecretStore CRD.
package secretstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/system"
)

const (
	defaultSecretsDir  = "automation/config/secrets" //nolint:gosec // directory path, not a credential
	defaultNamespace   = "external-secrets"
	esoSecretStoreName = "okdctl-secretstore" //nolint:gosec // resource name, not a credential
)

// Settings keys consumed by the SecretStore addon.
const (
	SettingSecretsDir             = "secrets_dir"
	SettingProvider               = "provider"
	SettingOnepasswordConnectHost = "onepassword_connect_host"
	SettingOnepasswordVaults      = "onepassword_vaults"
	SettingVaultServer            = "vault_server"
	SettingVaultPath              = "vault_path"
	SettingVaultVersion           = "vault_version"

	SettingBitwardenOrganizationID = "bitwarden_organization_id"
	SettingBitwardenProjectID      = "bitwarden_project_id"
	SettingBitwardenAPIURL         = "bitwarden_api_url"
	SettingBitwardenIdentityURL    = "bitwarden_identity_url"
	SettingBitwardenSDKServerURL   = "bitwarden_sdk_server_url"
)

func init() {
	if err := addon.Register(&SecretStore{}); err != nil {
		panic(err)
	}
}

// SecretStore is the multi-provider ESO secret-bootstrap addon.
type SecretStore struct{}

// Info returns the addon metadata block used by the registry.
func (s *SecretStore) Info() addon.Metadata {
	return addon.Metadata{
		Name:           "secretstore",
		DisplayName:    "External Secrets Operator Secret Store",
		Description:    "Bootstrap ESO provider credentials and SecretStore CRD (onepassword, vault, bitwarden)",
		Category:       "secrets",
		Dependencies:   nil,
		Priority:       50,
		DefaultEnabled: false,
	}
}

// Install creates the auth Secrets and the ESO SecretStore CRD for the
// configured provider. When provider prerequisites (e.g., credential files)
// are absent it logs setup instructions and returns nil.
func (s *SecretStore) Install(ctx context.Context, env *addon.Environment) error {
	decoded, err := s.DecodeSettings(env.AddonConfig.Settings)
	if err != nil {
		return fmt.Errorf("secretstore: invalid settings: %w", err)
	}
	ts := decoded.(Settings)
	p, _ := resolveProvider(env.AddonConfig.Settings)
	if p == nil {
		return fmt.Errorf("secretstore: unknown provider %q", ts.Provider)
	}

	skip, err := s.installPrereqCheck(env, ts.Provider)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	if err := addon.EnsureNamespace(ctx, env, defaultNamespace); err != nil {
		return err
	}

	env.Logger.Info("secretstore: installing provider", "provider", ts.Provider)

	manifests, err := p.buildResources(ctx, env, ts)
	if err != nil {
		return err
	}
	for _, manifest := range manifests {
		m := manifest
		if err := addon.RetryDefault(ctx, func() error {
			if _, err := env.Exec.RunWithStdinChecked(ctx, m, "oc", "apply", "-f", "-"); err != nil {
				return fmt.Errorf("secretstore: apply failed: %w", err)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	env.Logger.Info("secretstore: provider installed", "provider", ts.Provider)
	return nil
}

// installPrereqCheck validates provider-specific file prerequisites. It
// returns (true, nil) to signal a non-fatal skip when required files are
// absent — install logs setup instructions and returns success so the
// caller can rerun after placing the files.
func (s *SecretStore) installPrereqCheck(env *addon.Environment, providerName string) (skip bool, err error) {
	dir := resolveSecretsDir(env)
	switch providerName {
	case providerOnepassword:
		credPath := filepath.Join(dir, opCredentialsFile)
		tokenPath := filepath.Join(dir, opTokenFile)
		if !system.FileExists(credPath) && !system.FileExists(tokenPath) {
			env.Logger.Warn("secretstore: no secret files found, skipping", "dir", dir)
			env.Logger.Info("secretstore: to set up 1password connect secrets:")
			env.Logger.Info("  1. download 1password-credentials.json from Settings > Automation in 1password.com")
			env.Logger.Info("  2. create a connect token and save it:")
			env.Logger.Info("     echo -n 'YOUR_TOKEN' > " + filepath.Join(dir, opTokenFile))
			env.Logger.Info("  3. copy the credentials file:")
			env.Logger.Info("     cp ~/Downloads/1password-credentials.json " + dir + "/")
			env.Logger.Info("  4. (optional) encrypt with sops: sops -e -i <file>")
			env.Logger.Info("  5. re-run: okdctl addon install secretstore")
			return true, nil
		}
		if (isSopsEncrypted(credPath) || isSopsEncrypted(tokenPath)) && !executor.CommandExists("sops") {
			return false, fmt.Errorf("sops-encrypted secret files detected but sops is not installed: install with 'brew install sops'")
		}
	case providerVault:
		tokenPath := filepath.Join(dir, vaultTokenFile)
		if !system.FileExists(tokenPath) {
			env.Logger.Warn("secretstore: vault-token.txt not found, skipping", "dir", dir)
			env.Logger.Info("secretstore: write your Vault token to " + tokenPath)
			env.Logger.Info("secretstore: re-run: okdctl addon install secretstore")
			return true, nil
		}
		if isSopsEncrypted(tokenPath) && !executor.CommandExists("sops") {
			return false, fmt.Errorf("sops-encrypted vault token detected but sops is not installed: install with 'brew install sops'")
		}
	case providerBitwarden:
		tokenPath := filepath.Join(dir, bitwardenTokenFile)
		if !system.FileExists(tokenPath) {
			env.Logger.Warn("secretstore: bitwarden-token.txt not found, skipping", "dir", dir)
			env.Logger.Info("secretstore: write your Bitwarden machine-account access token to " + tokenPath)
			env.Logger.Info("secretstore: re-run: okdctl addon install secretstore")
			return true, nil
		}
		if isSopsEncrypted(tokenPath) && !executor.CommandExists("sops") {
			return false, fmt.Errorf("sops-encrypted bitwarden token detected but sops is not installed: install with 'brew install sops'")
		}
	}
	return false, nil
}

// Verify checks that each auth Secret created by Install exists in the cluster.
func (s *SecretStore) Verify(ctx context.Context, env *addon.Environment) error {
	p, providerName := resolveProvider(env.AddonConfig.Settings)
	if p == nil {
		return fmt.Errorf("secretstore: unknown provider %q", providerName)
	}
	ns := defaultNamespace
	for _, name := range p.secretNames() {
		result, err := env.Exec.Run(ctx, "oc", "get", "secret", name, "-n", ns)
		if err != nil || result.ExitCode != 0 {
			return fmt.Errorf("secret %s not found in namespace %s", name, ns)
		}
	}
	return nil
}

// Uninstall deletes the provider's auth Secrets and the ESO SecretStore CRD.
// Deletion failures are logged but do not abort the sequence.
func (s *SecretStore) Uninstall(ctx context.Context, env *addon.Environment) error {
	p, providerName := resolveProvider(env.AddonConfig.Settings)
	ns := defaultNamespace
	env.Logger.Info("secretstore: removing provider resources", "provider", providerName)
	if p != nil {
		for _, name := range p.secretNames() {
			if _, err := env.Exec.Run(ctx, "oc", "delete", "secret", name, "-n", ns); err != nil {
				env.Logger.Warn("secretstore: delete secret failed", "secret", name, "err", err)
			}
		}
	}
	if _, err := env.Exec.Run(ctx, "oc", "delete", "secretstore", esoSecretStoreName, "-n", ns); err != nil {
		env.Logger.Warn("secretstore: delete SecretStore CRD failed", "name", esoSecretStoreName, "err", err)
	}
	return nil
}

// RequiredTools lists the external binaries needed to decrypt source files.
func (s *SecretStore) RequiredTools() []addon.ToolSpec {
	return []addon.ToolSpec{
		{Name: "sops", Description: "Mozilla SOPS for decrypting secret files (used if files are sops-encrypted)"},
	}
}

// DefaultSettings returns the built-in defaults for secretstore's settings.
func (s *SecretStore) DefaultSettings() map[string]string {
	return map[string]string{
		SettingSecretsDir:            defaultSecretsDir,
		SettingProvider:              providerOnepassword,
		SettingOnepasswordVaults:     defaultOPVaults,
		SettingVaultPath:             "secret",
		SettingVaultVersion:          "v2",
		SettingBitwardenAPIURL:       defaultBitwardenAPIURL,
		SettingBitwardenIdentityURL:  defaultBitwardenIdentityURL,
		SettingBitwardenSDKServerURL: defaultBitwardenSDKServerURL,
	}
}

// ValidateSettings dispatches to the selected provider's validator and
// returns human-readable error strings for any invalid settings.
func (s *SecretStore) ValidateSettings(settings map[string]string) []string {
	decoded, err := s.DecodeSettings(settings)
	if err != nil {
		return []string{err.Error()}
	}
	ts := decoded.(Settings)
	p, name := resolveProvider(settings)
	if p == nil {
		return []string{fmt.Sprintf("provider %q is not supported; valid values: onepassword, vault, bitwarden", name)}
	}
	return p.validate(ts)
}

// WizardFields returns the wizard input fields the secretstore contributes.
// Fields are annotated with Group to associate them with their provider
// section; common fields have an empty Group.
func (s *SecretStore) WizardFields() []addon.WizardField {
	return []addon.WizardField{
		{Key: SettingProvider, Label: "Provider", Default: providerOnepassword, Help: "ESO backend provider: onepassword, vault, bitwarden"},
		{Key: SettingSecretsDir, Label: "Secrets Directory", Default: defaultSecretsDir, Help: "Directory containing provider credential files (plaintext or sops-encrypted)"},
		{Key: SettingOnepasswordConnectHost, Label: "Connect Host", Default: defaultOPConnectHost, Help: "1Password Connect server URL", Group: providerOnepassword},
		{Key: SettingOnepasswordVaults, Label: "Vaults", Default: defaultOPVaults, Help: "CSV of name=priority pairs, e.g. \"homelab=1,shared=2\"", Group: providerOnepassword},
		{Key: SettingVaultServer, Label: "Server URL", Help: "Vault server URL (e.g. https://vault.example.com)", Group: providerVault},
		{Key: SettingVaultPath, Label: "Secret Path", Default: "secret", Help: "Vault KV mount path", Group: providerVault},
		{Key: SettingVaultVersion, Label: "KV Version", Default: "v2", Help: "Vault KV engine version: v1 or v2", Group: providerVault},
		{Key: SettingBitwardenOrganizationID, Label: "Organization ID", Help: "Bitwarden organization UUID", Group: providerBitwarden},
		{Key: SettingBitwardenProjectID, Label: "Project ID", Help: "Bitwarden project UUID", Group: providerBitwarden},
		{Key: SettingBitwardenAPIURL, Label: "API URL", Default: defaultBitwardenAPIURL, Help: "Bitwarden Secrets Manager API URL", Group: providerBitwarden},
		{Key: SettingBitwardenIdentityURL, Label: "Identity URL", Default: defaultBitwardenIdentityURL, Help: "Bitwarden identity service URL", Group: providerBitwarden},
		{Key: SettingBitwardenSDKServerURL, Label: "SDK Server URL", Default: defaultBitwardenSDKServerURL, Help: "In-cluster bitwarden-sdk-server URL", Group: providerBitwarden},
	}
}

func isSopsEncrypted(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, `"sops"`) || strings.Contains(content, "sops_version=")
}

func readSecret(ctx context.Context, env *addon.Environment, path string) (string, error) {
	if !isSopsEncrypted(path) {
		// Refuse plaintext secret files that any other user can read —
		// mirrors the check in internal/credentials/envfile.go:loadEnvFileOnce.
		fi, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if perm := fi.Mode().Perm(); perm&0o077 != 0 {
			return "", fmt.Errorf("secret file %s has insecure permissions %#o; run 'chmod 600 %s' to fix",
				filepath.Base(path), perm, path)
		}
		env.Logger.Info("secretstore: reading plaintext file", "file", filepath.Base(path))
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	env.Logger.Info("secretstore: decrypting with sops", "file", filepath.Base(path))
	result, err := env.Exec.RunChecked(ctx, "sops", "-d", path)
	if err != nil {
		return "", fmt.Errorf("sops decryption failed (is the age key at ~/.config/sops/age/keys.txt?): %w", err)
	}
	return result.Stdout, nil
}

func resolveSecretsDir(env *addon.Environment) string {
	dir := env.AddonConfig.Settings[SettingSecretsDir]
	if dir == "" {
		dir = defaultSecretsDir
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(env.ProjectRoot, dir)
	}
	return dir
}

func secretManifestFromFile(ctx context.Context, env *addon.Environment, filePath, secretName, dataKey string) (string, error) {
	plaintext, err := readSecret(ctx, env, filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", filepath.Base(filePath), err)
	}
	manifest, err := addon.BuildOpaqueSecret(defaultNamespace, secretName, map[string][]byte{
		dataKey: []byte(strings.TrimSpace(plaintext)),
	})
	if err != nil {
		return "", fmt.Errorf("build %s secret: %w", secretName, err)
	}
	return manifest, nil
}

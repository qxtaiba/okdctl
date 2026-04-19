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
func (s *SecretStore) Info() addon.AddonInfo {
	return addon.AddonInfo{
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
	p, providerName := resolveProvider(env.AddonConfig.Settings)
	if p == nil {
		return fmt.Errorf("secretstore: unknown provider %q", providerName)
	}

	skip, err := s.installPrereqCheck(env, providerName)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	if err := addon.EnsureNamespace(ctx, env, defaultNamespace); err != nil {
		return err
	}

	env.Logger.Info(fmt.Sprintf("secretstore: installing %s provider", providerName))

	manifests, err := p.buildResources(ctx, env)
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

	env.Logger.Info(fmt.Sprintf("secretstore: %s provider installed", providerName))
	return nil
}

// installPrereqCheck validates provider-specific file prerequisites. It
// returns (true, nil) to signal a non-fatal skip when required files are
// absent — install logs setup instructions and returns success so the
// caller can rerun after placing the files.
func (s *SecretStore) installPrereqCheck(env *addon.Environment, providerName string) (skip bool, err error) {
	dir := resolveSecretsDir(env)
	switch providerName {
	case "onepassword":
		credPath := filepath.Join(dir, opCredentialsFile)
		tokenPath := filepath.Join(dir, opTokenFile)
		if !system.FileExists(credPath) && !system.FileExists(tokenPath) {
			env.Logger.Warn(fmt.Sprintf("secretstore: no secret files found in %s, skipping", dir))
			env.Logger.Warn("secretstore: to set up 1password connect secrets:")
			env.Logger.Warn("  1. download 1password-credentials.json from Settings > Automation in 1password.com")
			env.Logger.Warn("  2. create a connect token and save it:")
			env.Logger.Warn("     echo -n 'YOUR_TOKEN' > " + filepath.Join(dir, opTokenFile))
			env.Logger.Warn("  3. copy the credentials file:")
			env.Logger.Warn("     cp ~/Downloads/1password-credentials.json " + dir + "/")
			env.Logger.Warn("  4. (optional) encrypt with sops: sops -e -i <file>")
			env.Logger.Warn("  5. re-run: okdctl addon install secretstore")
			return true, nil
		}
		if (isSopsEncrypted(credPath) || isSopsEncrypted(tokenPath)) && !executor.CommandExists("sops") {
			return false, fmt.Errorf("sops-encrypted secret files detected but sops is not installed: install with 'brew install sops'")
		}
	case "vault":
		tokenPath := filepath.Join(dir, vaultTokenFile)
		if !system.FileExists(tokenPath) {
			env.Logger.Warn(fmt.Sprintf("secretstore: vault-token.txt not found in %s, skipping", dir))
			env.Logger.Warn("secretstore: write your Vault token to " + tokenPath)
			env.Logger.Warn("secretstore: re-run: okdctl addon install secretstore")
			return true, nil
		}
		if isSopsEncrypted(tokenPath) && !executor.CommandExists("sops") {
			return false, fmt.Errorf("sops-encrypted vault token detected but sops is not installed: install with 'brew install sops'")
		}
	case "bitwarden":
		tokenPath := filepath.Join(dir, bitwardenTokenFile)
		if !system.FileExists(tokenPath) {
			env.Logger.Warn(fmt.Sprintf("secretstore: bitwarden-token.txt not found in %s, skipping", dir))
			env.Logger.Warn("secretstore: write your Bitwarden machine-account access token to " + tokenPath)
			env.Logger.Warn("secretstore: re-run: okdctl addon install secretstore")
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
	env.Logger.Info(fmt.Sprintf("secretstore: removing %s provider resources", providerName))
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
		SettingProvider:              "onepassword",
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
	p, name := resolveProvider(settings)
	if p == nil {
		return []string{fmt.Sprintf("provider %q is not supported; valid values: onepassword, vault, bitwarden", name)}
	}
	return p.validateSettings(settings)
}

// WizardFields returns the wizard input fields the secretstore contributes.
func (s *SecretStore) WizardFields() []addon.WizardField {
	return []addon.WizardField{
		{Key: SettingProvider, Label: "Provider", Default: "onepassword", Help: "ESO backend provider: onepassword, vault, bitwarden"},
		{Key: SettingSecretsDir, Label: "Secrets Directory", Default: defaultSecretsDir, Help: "Directory containing provider credential files (plaintext or sops-encrypted)"},
		{Key: SettingOnepasswordVaults, Label: "1Password Vaults", Default: defaultOPVaults, Help: "CSV of name=priority pairs (onepassword only), e.g. \"homelab=1,shared=2\""},
		{Key: SettingVaultServer, Label: "Vault Server URL", Help: "Required for vault provider (e.g. https://vault.example.com)"},
		{Key: SettingVaultPath, Label: "Vault Secret Path", Default: "secret", Help: "Vault KV mount path (vault provider only)"},
		{Key: SettingVaultVersion, Label: "Vault KV Version", Default: "v2", Help: "Vault KV engine version: v1 or v2 (vault provider only)"},
		{Key: SettingBitwardenOrganizationID, Label: "Bitwarden Organization ID", Help: "Required for bitwarden provider (UUID)"},
		{Key: SettingBitwardenProjectID, Label: "Bitwarden Project ID", Help: "Required for bitwarden provider (UUID)"},
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
		env.Logger.Info(fmt.Sprintf("secretstore: reading plaintext file %s", filepath.Base(path)))
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	env.Logger.Info(fmt.Sprintf("secretstore: decrypting %s with sops", filepath.Base(path)))
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

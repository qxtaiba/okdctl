// Package secretstore provides the External Secrets Operator secret-bootstrap
// addon. It supports multiple ESO backends (onepassword, vault, bitwarden) via
// a provider setting and applies both auth Secrets and an ESO SecretStore CRD.
package secretstore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/errtypes"
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
	if err := addon.Register(&secretStore{}); err != nil {
		panic(err)
	}
}

// secretStore is the multi-provider ESO secret-bootstrap addon.
type secretStore struct{}

// Info returns the addon metadata block used by the registry.
func (s *secretStore) Info() addon.Metadata {
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
func (s *secretStore) Install(ctx context.Context, env *addon.Environment) error {
	ts, err := s.decodeSettings(env.AddonConfig.Settings)
	if err != nil {
		return &errtypes.ConfigError{Msg: "secretstore: invalid settings", Err: err}
	}
	p, _ := resolveProvider(env.AddonConfig.Settings)
	if p == nil {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("secretstore: unknown provider %q", ts.Provider)}
	}

	skip, err := s.installPrereqCheck(env, string(ts.Provider))
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	if err := addon.EnsureNamespace(ctx, env, defaultNamespace); err != nil {
		return err
	}

	env.Logger.Info("secretstore: installing provider", "provider", string(ts.Provider))

	manifests, err := p.buildResources(ctx, env, ts)
	if err != nil {
		return err
	}
	for _, manifest := range manifests {
		m := manifest
		if err := addon.RetryDefault(ctx, func() error {
			if _, err := env.Exec.RunWithStdinChecked(ctx, m, "oc", "apply", "-f", "-"); err != nil {
				return fmt.Errorf("apply %s manifest: %w", ts.Provider, err)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	env.Logger.Info("secretstore: provider installed", "provider", string(ts.Provider))
	return nil
}

// installPrereqCheck validates provider-specific file prerequisites. It
// returns (true, nil) to signal a non-fatal skip when required files are
// absent — install warns and points at docs/addons/secretstore.md so the
// caller can rerun after placing the files.
func (s *secretStore) installPrereqCheck(env *addon.Environment, providerName string) (skip bool, err error) {
	dir := resolveSecretsDir(env)
	switch providerKind(providerName) {
	case providerOnepassword:
		credPath := filepath.Join(dir, opCredentialsFile)
		tokenPath := filepath.Join(dir, opTokenFile)
		if !system.FileExists(credPath) && !system.FileExists(tokenPath) {
			env.Logger.Warn("secretstore: no secret files found, skipping",
				"dir", dir,
				"docs", "docs/addons/secretstore.md")
			return true, nil
		}
		if (isSopsEncrypted(credPath) || isSopsEncrypted(tokenPath)) && !executor.CommandExists("sops") {
			return false, fmt.Errorf("sops-encrypted secret files detected but sops is not installed: install with 'brew install sops'")
		}
	case providerVault:
		tokenPath := filepath.Join(dir, vaultTokenFile)
		if !system.FileExists(tokenPath) {
			env.Logger.Warn("secretstore: vault-token.txt not found, skipping",
				"dir", dir,
				"docs", "docs/addons/secretstore.md")
			return true, nil
		}
		if isSopsEncrypted(tokenPath) && !executor.CommandExists("sops") {
			return false, fmt.Errorf("sops-encrypted vault token detected but sops is not installed: install with 'brew install sops'")
		}
	case providerBitwarden:
		tokenPath := filepath.Join(dir, bitwardenTokenFile)
		if !system.FileExists(tokenPath) {
			env.Logger.Warn("secretstore: bitwarden-token.txt not found, skipping",
				"dir", dir,
				"docs", "docs/addons/secretstore.md")
			return true, nil
		}
		if isSopsEncrypted(tokenPath) && !executor.CommandExists("sops") {
			return false, fmt.Errorf("sops-encrypted bitwarden token detected but sops is not installed: install with 'brew install sops'")
		}
	}
	return false, nil
}

// Verify checks that each auth Secret created by Install exists in the cluster.
func (s *secretStore) Verify(ctx context.Context, env *addon.Environment) error {
	p, kind := resolveProvider(env.AddonConfig.Settings)
	if p == nil {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("secretstore: unknown provider %q", kind)}
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
func (s *secretStore) Uninstall(ctx context.Context, env *addon.Environment) error {
	p, kind := resolveProvider(env.AddonConfig.Settings)
	ns := defaultNamespace
	env.Logger.Info("secretstore: removing provider resources", "provider", string(kind))
	if p != nil {
		for _, name := range p.secretNames() {
			// Run returns a nil error for non-zero exits (only start/ctx
			// failures error), so the exit code must be checked or a failed
			// delete passes silently.
			if res, err := env.Exec.Run(ctx, "oc", "delete", "secret", name, "-n", ns); err != nil || res.ExitCode != 0 {
				env.Logger.Warn("secretstore: delete secret failed", "name", name, "exit", res.ExitCode, "err", err)
			}
		}
	}
	if res, err := env.Exec.Run(ctx, "oc", "delete", "secretstore", esoSecretStoreName, "-n", ns); err != nil || res.ExitCode != 0 {
		env.Logger.Warn("secretstore: delete SecretStore CRD failed", "name", esoSecretStoreName, "exit", res.ExitCode, "err", err)
	}
	return nil
}

// RequiredTools lists the external binaries needed to decrypt source files.
func (s *secretStore) RequiredTools() []addon.ToolSpec {
	return []addon.ToolSpec{
		{Name: "sops", Description: "Mozilla SOPS for decrypting secret files (used if files are sops-encrypted)"},
	}
}

// DefaultSettings returns the built-in defaults for secretstore's settings.
func (s *secretStore) DefaultSettings() map[string]string {
	return map[string]string{
		SettingSecretsDir:            defaultSecretsDir,
		SettingProvider:              string(providerOnepassword),
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
func (s *secretStore) ValidateSettings(settings map[string]string) []string {
	ts, err := s.decodeSettings(settings)
	if err != nil {
		return []string{err.Error()}
	}
	p, kind := resolveProvider(settings)
	if p == nil {
		return []string{fmt.Sprintf("provider %q is not supported; valid values: onepassword, vault, bitwarden", kind)}
	}
	return p.validate(ts)
}

func isSopsEncrypted(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return isSopsEncryptedBytes(data)
}

func isSopsEncryptedBytes(data []byte) bool {
	content := string(data)
	return strings.Contains(content, `"sops"`) || strings.Contains(content, "sops_version=")
}

// readSecret reads and returns the secret at path, decrypting via sops when
// the file is sops-encrypted. The symlink refusal and O_NOFOLLOW open guard
// both branches: the sops-vs-plaintext classification reads through the same
// guarded descriptor rather than re-reading by path, so it cannot follow a
// symlink planted during a sudo re-exec. The 0o077 permission floor applies
// only to plaintext secrets (a sops ciphertext is safe at looser modes).
func readSecret(ctx context.Context, env *addon.Environment, path string) (string, error) {
	// lstat (not stat) so the perm gate and the open see the same inode.
	fi, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("secret file %q is a symlink; refusing to follow", path)
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	if isSopsEncryptedBytes(data) {
		env.Logger.Info("secretstore: decrypting with sops", "file", filepath.Base(path))
		result, err := env.Exec.RunOutputChecked(ctx, 0, "sops", "-d", path)
		if err != nil {
			return "", fmt.Errorf("sops decryption failed (is the age key at ~/.config/sops/age/keys.txt?): %w", err)
		}
		if result.Truncated {
			return "", fmt.Errorf("sops decryption output truncated after %d bytes; secret may be corrupted", len(result.Stdout))
		}
		return result.Stdout, nil
	}

	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		return "", fmt.Errorf("secret file %s has insecure permissions %#o; run 'chmod 600 %s' to fix",
			filepath.Base(path), perm, path)
	}
	env.Logger.Info("secretstore: reading plaintext file", "file", filepath.Base(path))
	return string(data), nil
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
		return "", fmt.Errorf("read %s: %w", filepath.Base(filePath), err)
	}
	manifest, err := addon.BuildOpaqueSecret(defaultNamespace, secretName, map[string][]byte{
		dataKey: []byte(strings.TrimSpace(plaintext)),
	})
	if err != nil {
		return "", fmt.Errorf("build %s secret: %w", secretName, err)
	}
	return manifest, nil
}

// Package secretstore provides the 1Password Connect secret bootstrap addon.
package secretstore

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/addon"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const (
	defaultSecretsDir = "automation/config/secrets"
	defaultNamespace  = "external-secrets"

	credentialsFile = "1password-credentials.json"
	tokenFile       = "1password-token.txt"

	credentialsSecretName = "onepassword-connect-credentials"
	tokenSecretName       = "onepassword-connect-token"
)

func init() {
	addon.Register(&SecretStore{})
}

// SecretStore implements the addon.Addon interface for 1Password Connect bootstrap.
type SecretStore struct{}

func (s *SecretStore) Info() addon.AddonInfo {
	return addon.AddonInfo{
		Name:           "secretstore",
		DisplayName:    "1Password Secret Store",
		Description:    "Bootstrap 1Password Connect secrets from sops-encrypted files",
		Category:       "secrets",
		Dependencies:   nil,
		Priority:       50,
		DefaultEnabled: false,
	}
}

func (s *SecretStore) Install(ctx context.Context, env *addon.Environment) error {
	if !executor.CommandExists("sops") {
		return fmt.Errorf("sops is required to decrypt secret files — install with: brew install sops (macOS) or enable secretstore during setup to auto-install")
	}

	secretsDir := s.secretsDir(env)

	credPath := filepath.Join(secretsDir, credentialsFile)
	tokenPath := filepath.Join(secretsDir, tokenFile)

	// If no secret files exist, warn with setup instructions and return (non-fatal)
	if !system.FileExists(credPath) && !system.FileExists(tokenPath) {
		env.Logger.Warn(fmt.Sprintf("secretstore: no sops-encrypted files found in %s, skipping", secretsDir))
		env.Logger.Warn("secretstore: to set up 1password connect secrets:")
		env.Logger.Warn("  1. download 1password-credentials.json from Settings > Automation in 1password.com")
		env.Logger.Warn("  2. create a connect token and save it:")
		env.Logger.Warn("     echo -n 'YOUR_TOKEN' > " + filepath.Join(secretsDir, tokenFile))
		env.Logger.Warn("  3. copy the credentials file:")
		env.Logger.Warn("     cp ~/Downloads/1password-credentials.json " + secretsDir + "/")
		env.Logger.Warn("  4. encrypt both files with sops:")
		env.Logger.Warn("     sops -e -i " + credPath)
		env.Logger.Warn("     sops -e -i " + tokenPath)
		env.Logger.Warn("  5. re-run: openshitctl addon install secretstore")
		env.Logger.Warn("  note: sops requires an age key at ~/.config/sops/age/keys.txt")
		env.Logger.Warn("  setup: age-keygen -o ~/.config/sops/age/keys.txt (local), then scp to bastion")
		return nil
	}

	if err := s.ensureNamespace(ctx, env); err != nil {
		return err
	}

	env.Logger.Info("secretstore: creating 1password connect secrets from sops-encrypted files")

	if system.FileExists(credPath) {
		if err := s.createCredentialsSecret(ctx, env, credPath); err != nil {
			return err
		}
	}

	if system.FileExists(tokenPath) {
		if err := s.createTokenSecret(ctx, env, tokenPath); err != nil {
			return err
		}
	}

	env.Logger.Info("secretstore: all connect secrets created successfully")
	return nil
}

func (s *SecretStore) Verify(ctx context.Context, env *addon.Environment) error {
	ns := defaultNamespace

	result, err := env.Exec.Run(ctx, "oc", "get", "secret", credentialsSecretName, "-n", ns)
	if err != nil || result == nil || result.ExitCode != 0 {
		return fmt.Errorf("secret %s not found in namespace %s", credentialsSecretName, ns)
	}

	result, err = env.Exec.Run(ctx, "oc", "get", "secret", tokenSecretName, "-n", ns)
	if err != nil || result == nil || result.ExitCode != 0 {
		return fmt.Errorf("secret %s not found in namespace %s", tokenSecretName, ns)
	}

	env.Logger.Info("secretstore: both 1password connect secrets exist")
	return nil
}

func (s *SecretStore) Uninstall(ctx context.Context, env *addon.Environment) error {
	ns := defaultNamespace
	env.Logger.Info("secretstore: removing 1password connect secrets")
	_, _ = env.Exec.Run(ctx, "oc", "delete", "secret", credentialsSecretName, "-n", ns)
	_, _ = env.Exec.Run(ctx, "oc", "delete", "secret", tokenSecretName, "-n", ns)
	return nil
}

// RequiredTools implements addon.ToolProvider.
func (s *SecretStore) RequiredTools() []addon.ToolSpec {
	return []addon.ToolSpec{
		{Name: "sops", Description: "Mozilla SOPS for decrypting secret files"},
	}
}

// DefaultSettings implements addon.ConfigurableAddon.
func (s *SecretStore) DefaultSettings() map[string]string {
	return map[string]string{
		"secrets_dir": defaultSecretsDir,
	}
}

// ValidateSettings implements addon.ConfigurableAddon.
func (s *SecretStore) ValidateSettings(settings map[string]string) []string {
	// No validation errors — secrets_dir defaults are fine, path is checked at install time
	return nil
}

// WizardFields implements addon.WizardProvider.
func (s *SecretStore) WizardFields() []addon.WizardField {
	return []addon.WizardField{
		{Key: "secrets_dir", Label: "Secrets Directory", Default: defaultSecretsDir, Help: "Directory containing sops-encrypted 1password-credentials.json and 1password-token.txt"},
	}
}

// ════════════════════════════════════════════════════════════════════════════════
// INTERNAL HELPERS
// ════════════════════════════════════════════════════════════════════════════════

func (s *SecretStore) secretsDir(env *addon.Environment) string {
	dir := env.AddonConfig.Settings["secrets_dir"]
	if dir == "" {
		dir = defaultSecretsDir
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(env.ProjectRoot, dir)
	}
	return dir
}

func (s *SecretStore) ensureNamespace(ctx context.Context, env *addon.Environment) error {
	result, err := env.Exec.Run(ctx, "oc", "get", "namespace", defaultNamespace)
	if err == nil && result != nil && result.ExitCode == 0 {
		return nil
	}

	env.Logger.Info(fmt.Sprintf("secretstore: creating %s namespace", defaultNamespace))
	createResult, createErr := env.Exec.Run(ctx, "oc", "create", "namespace", defaultNamespace)
	if createErr != nil {
		return utils.WrapError(fmt.Sprintf("failed to create %s namespace", defaultNamespace), createErr)
	}
	if createResult == nil || createResult.ExitCode != 0 {
		stderr := ""
		if createResult != nil {
			stderr = createResult.Stderr
		}
		return fmt.Errorf("failed to create %s namespace: %s", defaultNamespace, stderr)
	}
	return nil
}

func (s *SecretStore) secretExists(ctx context.Context, env *addon.Environment, name string) bool {
	result, err := env.Exec.Run(ctx, "oc", "get", "secret", name, "-n", defaultNamespace)
	return err == nil && result != nil && result.ExitCode == 0
}

func (s *SecretStore) createCredentialsSecret(ctx context.Context, env *addon.Environment, credPath string) error {
	if s.secretExists(ctx, env, credentialsSecretName) {
		env.Logger.Info("secretstore: credentials secret already exists, skipping")
		return nil
	}

	env.Logger.Info("secretstore: decrypting credentials file with sops")
	sopsResult, err := env.Exec.Run(ctx, "sops", "-d", credPath)
	if err != nil {
		return utils.WrapError("failed to decrypt 1password credentials with sops", err)
	}
	if sopsResult.ExitCode != 0 {
		return fmt.Errorf("sops decryption failed (is the age key at ~/.config/sops/age/keys.txt?): %s", sopsResult.Stderr)
	}

	// Base64-encoded because the HelmRelease mounts it as a pre-encoded value
	credentialsBase64 := base64.StdEncoding.EncodeToString([]byte(sopsResult.Stdout))

	result, err := env.Exec.Run(ctx, "oc", "create", "secret", "generic", credentialsSecretName,
		"--namespace", defaultNamespace,
		"--from-literal=credentials_base64="+credentialsBase64)
	if err != nil {
		return utils.WrapError("failed to create 1password credentials secret", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to create 1password credentials secret: %s", result.Stderr)
	}

	env.Logger.Info("secretstore: credentials secret created")
	return nil
}

func (s *SecretStore) createTokenSecret(ctx context.Context, env *addon.Environment, tokenPath string) error {
	if s.secretExists(ctx, env, tokenSecretName) {
		env.Logger.Info("secretstore: token secret already exists, skipping")
		return nil
	}

	env.Logger.Info("secretstore: decrypting token file with sops")
	sopsResult, err := env.Exec.Run(ctx, "sops", "-d", tokenPath)
	if err != nil {
		return utils.WrapError("failed to decrypt 1password token with sops", err)
	}
	if sopsResult.ExitCode != 0 {
		return fmt.Errorf("sops decryption failed (is the age key at ~/.config/sops/age/keys.txt?): %s", sopsResult.Stderr)
	}
	token := strings.TrimSpace(sopsResult.Stdout)

	result, err := env.Exec.Run(ctx, "oc", "create", "secret", "generic", tokenSecretName,
		"--namespace", defaultNamespace,
		"--from-literal=token="+token)
	if err != nil {
		return utils.WrapError("failed to create 1password token secret", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to create 1password token secret: %s", result.Stderr)
	}

	env.Logger.Info("secretstore: token secret created")
	return nil
}

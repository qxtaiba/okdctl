// Package secretstore provides the 1Password Connect secret bootstrap addon.
package secretstore

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/addon"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const (
	defaultSecretsDir = "automation/config/secrets" //nolint:gosec // directory path, not a credential
	defaultNamespace  = "external-secrets"

	credentialsFile = "1password-credentials.json"
	tokenFile       = "1password-token.txt"

	credentialsSecretName = "onepassword-connect-credentials"
	tokenSecretName       = "onepassword-connect-token"
)

func init() {
	if err := addon.Register(&SecretStore{}); err != nil {
		panic(err)
	}
}

type SecretStore struct{}

func (s *SecretStore) Info() addon.AddonInfo {
	return addon.AddonInfo{
		Name:           "secretstore",
		DisplayName:    "1Password Secret Store",
		Description:    "Bootstrap 1Password Connect secrets from plaintext or sops-encrypted files",
		Category:       "secrets",
		Dependencies:   nil,
		Priority:       50,
		DefaultEnabled: false,
	}
}

func (s *SecretStore) Install(ctx context.Context, env *addon.Environment) error {
	secretsDir, credPath, tokenPath := s.secretFilePaths(env)

	// If no secret files exist, warn with setup instructions and return (non-fatal)
	if !system.FileExists(credPath) && !system.FileExists(tokenPath) {
		env.Logger.Warn(fmt.Sprintf("secretstore: no secret files found in %s, skipping", secretsDir))
		env.Logger.Warn("secretstore: to set up 1password connect secrets:")
		env.Logger.Warn("  1. download 1password-credentials.json from Settings > Automation in 1password.com")
		env.Logger.Warn("  2. create a connect token and save it:")
		env.Logger.Warn("     echo -n 'YOUR_TOKEN' > " + filepath.Join(secretsDir, tokenFile))
		env.Logger.Warn("  3. copy the credentials file:")
		env.Logger.Warn("     cp ~/Downloads/1password-credentials.json " + secretsDir + "/")
		env.Logger.Warn("  4. (optional) encrypt with sops: sops -e -i <file>")
		env.Logger.Warn("  5. re-run: openshitctl addon install secretstore")
		return nil
	}

	// Only require sops if any file is actually encrypted
	encrypted := isSopsEncrypted(credPath) || isSopsEncrypted(tokenPath)
	if encrypted && !executor.CommandExists("sops") {
		return fmt.Errorf("sops-encrypted secret files detected but sops is not installed — install with: brew install sops")
	}

	if err := addon.EnsureNamespace(ctx, env, defaultNamespace); err != nil {
		return err
	}

	env.Logger.Info("secretstore: creating 1password connect secrets")

	if system.FileExists(credPath) {
		if err := addon.RetryDefault(ctx, func() error {
			return s.createSecretFromFile(ctx, env, credPath, credentialsSecretName, "credentials_base64")
		}); err != nil {
			return err
		}
	}

	if system.FileExists(tokenPath) {
		if err := addon.RetryDefault(ctx, func() error {
			return s.createSecretFromFile(ctx, env, tokenPath, tokenSecretName, "token")
		}); err != nil {
			return err
		}
	}

	env.Logger.Info("secretstore: all connect secrets created successfully")
	return nil
}

func (s *SecretStore) Verify(ctx context.Context, env *addon.Environment) error {
	// Verify mirrors Install's per-file contract — Install creates each secret
	// independently based on which source file is present, so Verify must gate
	// each in-cluster check the same way. This avoids contradictory
	// "Install OK / Verify fail" states on partial installs.
	_, credPath, tokenPath := s.secretFilePaths(env)
	ns := defaultNamespace

	if system.FileExists(credPath) {
		result, err := env.Exec.Run(ctx, "oc", "get", "secret", credentialsSecretName, "-n", ns)
		if err != nil || result == nil || result.ExitCode != 0 {
			return fmt.Errorf("secret %s not found in namespace %s", credentialsSecretName, ns)
		}
	} else {
		env.Logger.Warn("secretstore: credentials file not configured, skipping credentials secret verification")
	}

	if system.FileExists(tokenPath) {
		result, err := env.Exec.Run(ctx, "oc", "get", "secret", tokenSecretName, "-n", ns)
		if err != nil || result == nil || result.ExitCode != 0 {
			return fmt.Errorf("secret %s not found in namespace %s", tokenSecretName, ns)
		}
	} else {
		env.Logger.Warn("secretstore: token file not configured, skipping token secret verification")
	}

	return nil
}

func (s *SecretStore) Uninstall(ctx context.Context, env *addon.Environment) error {
	ns := defaultNamespace
	env.Logger.Info("secretstore: removing 1password connect secrets")
	if _, err := env.Exec.Run(ctx, "oc", "delete", "secret", credentialsSecretName, "-n", ns); err != nil {
		env.Logger.Warn(fmt.Sprintf("secretstore: delete %s: %v", credentialsSecretName, err))
	}
	if _, err := env.Exec.Run(ctx, "oc", "delete", "secret", tokenSecretName, "-n", ns); err != nil {
		env.Logger.Warn(fmt.Sprintf("secretstore: delete %s: %v", tokenSecretName, err))
	}
	return nil
}

func (s *SecretStore) RequiredTools() []addon.ToolSpec {
	return []addon.ToolSpec{
		{Name: "sops", Description: "Mozilla SOPS for decrypting secret files (used if files are sops-encrypted)"},
	}
}

func (s *SecretStore) DefaultSettings() map[string]string {
	return map[string]string{
		"secrets_dir": defaultSecretsDir,
	}
}

func (s *SecretStore) ValidateSettings(_ map[string]string) []string {
	// No validation errors — secrets_dir defaults are fine, path is checked at install time
	return nil
}

func (s *SecretStore) WizardFields() []addon.WizardField {
	return []addon.WizardField{
		{Key: "secrets_dir", Label: "Secrets Directory", Default: defaultSecretsDir, Help: "Directory containing 1password-credentials.json and 1password-token.txt (plaintext or sops-encrypted)"},
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

func (s *SecretStore) readSecret(ctx context.Context, env *addon.Environment, path string) (string, error) {
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

// secretFilePaths returns the resolved secrets directory along with the
// credentials and token file paths. Install and Verify both use this so the
// "skip if no files configured" check stays in sync between the two.
func (s *SecretStore) secretFilePaths(env *addon.Environment) (secretsDir, credPath, tokenPath string) {
	secretsDir = s.secretsDir(env)
	credPath = filepath.Join(secretsDir, credentialsFile)
	tokenPath = filepath.Join(secretsDir, tokenFile)
	return
}

func (s *SecretStore) createSecretFromFile(ctx context.Context, env *addon.Environment, filePath, secretName, dataKey string) error {
	plaintext, err := s.readSecret(ctx, env, filePath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", filepath.Base(filePath), err)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(plaintext)))
	manifest := addon.BuildOpaqueSecret(defaultNamespace, secretName, map[string]string{dataKey: encoded})
	if _, err := env.Exec.RunWithStdinChecked(ctx, manifest, "oc", "apply", "-f", "-"); err != nil {
		return fmt.Errorf("failed to apply %s secret: %w", secretName, err)
	}
	env.Logger.Info(fmt.Sprintf("secretstore: %s secret applied", secretName))
	return nil
}

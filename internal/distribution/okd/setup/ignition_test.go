package setup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func makeCfgWithFiles(t *testing.T, pullSecretContent, sshKeyContent string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	pullSecretPath := filepath.Join(dir, "pull-secret.json")
	if err := os.WriteFile(pullSecretPath, []byte(pullSecretContent), 0o600); err != nil {
		t.Fatalf("write pull-secret: %v", err)
	}
	sshKeyPath := filepath.Join(dir, "id_rsa.pub")
	if err := os.WriteFile(sshKeyPath, []byte(sshKeyContent), 0o600); err != nil {
		t.Fatalf("write ssh key: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.Files.PullSecret = pullSecretPath
	cfg.Files.SSHPublicKey = sshKeyPath
	return cfg
}

func TestGenerateInstallConfig_Perms(t *testing.T) {
	cfg := makeCfgWithFiles(t, `{"auths":{}}`, "ssh-rsa AAAAB3 test@host")
	outputDir := t.TempDir()
	p := newTestPhase(t)

	if err := p.GenerateInstallConfig(t.Context(), cfg, outputDir); err != nil {
		t.Fatalf("GenerateInstallConfig: %v", err)
	}

	for _, name := range []string{"install-config.yaml", "install-config.yaml.backup"} {
		path := filepath.Join(outputDir, name)
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if got := fi.Mode().Perm(); got != 0o600 {
			t.Errorf("%s perm = %04o, want 0600", name, got)
		}
	}
}

func TestGenerateInstallConfig_PullSecretReadFail(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Files.PullSecret = filepath.Join(t.TempDir(), "does-not-exist.json")
	cfg.Files.SSHPublicKey = filepath.Join(t.TempDir(), "id_rsa.pub")

	p := newTestPhase(t)
	err := p.GenerateInstallConfig(t.Context(), cfg, t.TempDir())
	if err == nil {
		t.Fatal("want error for missing pull-secret, got nil")
	}
	var authErr *errtypes.AuthError
	if !errors.As(err, &authErr) {
		t.Errorf("error type = %T, want *errtypes.AuthError", err)
	}
}

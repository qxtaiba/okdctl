package setup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
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

func TestGenerateInstallConfig_PullSecretInFileAndZeroed(t *testing.T) {
	const pullSecretJSON = `{"auths":{"registry.example.com":{"auth":"dXNlcjpwYXNz"}}}`

	var (
		captured []byte
		zeroed   bool
	)
	orig := zeroBytesFn
	zeroBytesFn = func(b []byte) {
		captured = b
		system.ZeroBytes(b)
		zeroed = true
	}
	t.Cleanup(func() { zeroBytesFn = orig })

	cfg := makeCfgWithFiles(t, pullSecretJSON, "ssh-rsa AAAAB3 test@host")
	outputDir := t.TempDir()
	p := newTestPhase(t)

	if err := p.GenerateInstallConfig(t.Context(), cfg, outputDir); err != nil {
		t.Fatalf("GenerateInstallConfig: %v", err)
	}

	rendered, err := os.ReadFile(filepath.Join(outputDir, "install-config.yaml"))
	if err != nil {
		t.Fatalf("read install-config.yaml: %v", err)
	}
	if !strings.Contains(string(rendered), pullSecretJSON) {
		t.Errorf("install-config.yaml does not contain pull-secret JSON")
	}

	if !zeroed {
		t.Fatal("zeroBytesFn was not called; defer zeroBytesFn(pullSecret) may have been removed")
	}
	for i, b := range captured {
		if b != 0 {
			t.Errorf("pullSecret[%d] = %#x after zeroize, want 0x00", i, b)
		}
	}
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

func TestGenerateInstallConfig_SymlinkRejected(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("not a secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	pullSymlink := filepath.Join(dir, "pull-secret.json")
	if err := os.Symlink(target, pullSymlink); err != nil {
		t.Fatal(err)
	}
	sshKeyPath := filepath.Join(dir, "id_rsa.pub")
	if err := os.WriteFile(sshKeyPath, []byte("ssh-rsa AAAAB3 test@host"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.Files.PullSecret = pullSymlink
	cfg.Files.SSHPublicKey = sshKeyPath
	p := newTestPhase(t)

	err := p.GenerateInstallConfig(t.Context(), cfg, t.TempDir())
	if err == nil {
		t.Fatal("want error for symlinked pull-secret, got nil")
	}
	var authErr *errtypes.AuthError
	if !errors.As(err, &authErr) {
		t.Errorf("pull-secret symlink: error type = %T, want *errtypes.AuthError", err)
	}

	realPS := filepath.Join(dir, "pull-secret-real.json")
	if err := os.WriteFile(realPS, []byte(`{"auths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	sshSymlink := filepath.Join(dir, "id_rsa-sym.pub")
	if err := os.Symlink(target, sshSymlink); err != nil {
		t.Fatal(err)
	}

	cfg2 := config.DefaultConfig()
	cfg2.Files.PullSecret = realPS
	cfg2.Files.SSHPublicKey = sshSymlink

	err = p.GenerateInstallConfig(t.Context(), cfg2, t.TempDir())
	if err == nil {
		t.Fatal("want error for symlinked SSH key, got nil")
	}
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Errorf("ssh-key symlink: error type = %T, want *errtypes.ConfigError", err)
	}
}

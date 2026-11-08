package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
)

func TestRunDeployWriteConfigWritesWithoutDeploying(t *testing.T) {
	t.Chdir(t.TempDir())
	deployWriteConfig = true
	t.Cleanup(func() { deployWriteConfig = false })

	if err := runDeploy(deployCmd, nil); err != nil {
		t.Fatalf("runDeploy --write-config: %v", err)
	}
	if _, err := os.Stat("okdctl.yaml"); err != nil {
		t.Fatalf("--write-config must write okdctl.yaml: %v", err)
	}
}

func TestWriteCredentialsEnv(t *testing.T) {
	cases := []struct {
		name        string
		password    string
		apiToken    string
		wantFile    bool
		wantContent []string
	}{
		{"password only", "pw-secret", "", true, []string{"PROXMOX_VE_PASSWORD=pw-secret"}},
		{"token only", "", "tok-secret", true, []string{"PROXMOX_VE_API_TOKEN=tok-secret"}},
		{"both", "pw-secret", "tok-secret", true, []string{"PROXMOX_VE_PASSWORD=pw-secret", "PROXMOX_VE_API_TOKEN=tok-secret"}},
		{"neither", "", "", false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				Provider: config.ProviderConfig{
					Type:    config.ProviderProxmox,
					Proxmox: &config.ProxmoxConfig{Host: "px.local", Username: "root@pam"},
				},
			}
			if tc.password != "" {
				cfg.Provider.Proxmox.Password.Set(tc.password)
			}
			if tc.apiToken != "" {
				cfg.Provider.Proxmox.APIToken.Set(tc.apiToken)
			}

			configPath := filepath.Join(t.TempDir(), "okdctl.yaml")
			if err := writeCredentialsEnv(cfg, configPath); err != nil {
				t.Fatalf("writeCredentialsEnv: %v", err)
			}

			envPath := credentials.EnvFilePath(configPath)
			data, readErr := os.ReadFile(envPath)
			if !tc.wantFile {
				if readErr == nil {
					t.Fatalf("neither credential set must write no env file; found %s", envPath)
				}
				return
			}
			if readErr != nil {
				t.Fatalf("expected env file at %s: %v", envPath, readErr)
			}
			for _, want := range tc.wantContent {
				if !strings.Contains(string(data), want) {
					t.Errorf("env file missing %q; got:\n%s", want, data)
				}
			}
		})
	}
}

func TestWriteCredentialsEnvNilProxmox(t *testing.T) {
	cfg := &config.Config{Provider: config.ProviderConfig{Type: config.ProviderProxmox}}
	configPath := filepath.Join(t.TempDir(), "okdctl.yaml")

	if err := writeCredentialsEnv(cfg, configPath); err != nil {
		t.Fatalf("nil Proxmox must be a no-op, got %v", err)
	}
	if _, err := os.Stat(credentials.EnvFilePath(configPath)); err == nil {
		t.Fatal("nil Proxmox must write no env file")
	}
}

func TestClearConfigCredentials(t *testing.T) {
	t.Run("wipes password and token", func(t *testing.T) {
		cfg := &config.Config{Provider: config.ProviderConfig{Proxmox: &config.ProxmoxConfig{}}}
		cfg.Provider.Proxmox.Password.Set("pw-secret")
		cfg.Provider.Proxmox.APIToken.Set("tok-secret")

		pwAlias := cfg.Provider.Proxmox.Password.Bytes()
		tokAlias := cfg.Provider.Proxmox.APIToken.Bytes()

		clearConfigCredentials(cfg)

		for i, b := range pwAlias {
			if b != 0 {
				t.Fatalf("password byte %d not zeroed: %d", i, b)
			}
		}
		for i, b := range tokAlias {
			if b != 0 {
				t.Fatalf("token byte %d not zeroed: %d", i, b)
			}
		}
		if !cfg.Provider.Proxmox.Password.IsEmpty() {
			t.Error("password not empty after clear")
		}
		if !cfg.Provider.Proxmox.APIToken.IsEmpty() {
			t.Error("token not empty after clear")
		}
	})

	t.Run("nil Proxmox does not panic", func(_ *testing.T) {
		cfg := &config.Config{Provider: config.ProviderConfig{}}
		clearConfigCredentials(cfg)
	})
}

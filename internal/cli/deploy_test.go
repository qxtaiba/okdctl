package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// Bogus-type case: validateProvider no-ops on a non-proxmox type, so the gate
// also needs the required/enum scopes.
func TestRunFullDeployment_RejectsInvalidProvider(t *testing.T) {
	cases := []struct {
		name         string
		providerType config.ProviderType
		isoStorage   string
	}{
		{"invalid provider config", config.ProviderProxmox, "${inject}"},
		{"bogus provider type", "Proxmox", `x"inject`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				Provider: config.ProviderConfig{
					Type: tc.providerType,
					Proxmox: &config.ProxmoxConfig{
						Host:       "px.local",
						Node:       "pve",
						Storage:    "local-lvm",
						ISOStorage: tc.isoStorage,
					},
				},
			}
			err := runFullDeployment(deployCmd, context.Background(), cfg, io.Discard)
			var ce *errtypes.ConfigError
			if !errors.As(err, &ce) {
				t.Fatalf("want *errtypes.ConfigError, got %T: %v", err, err)
			}
		})
	}
}

func TestDeployGateScope_RejectsUnsafeHTTPRoot(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.HTTPServer.Root = "/var/www/$(reboot)"
	result := config.ValidateWithOptions(cfg, config.ValidationOptions{Scope: deployGateScope})
	if result.IsValid() {
		t.Fatal("gate accepted an http_server.root with shell metacharacters")
	}
}

func TestDeployDryRunSteps_DerivedFromLivePhaseSteps(t *testing.T) {
	cfg := config.DefaultConfig()
	root := t.TempDir()

	got := deployDryRunSteps(cfg, root)

	if len(got) == 0 {
		t.Fatal("expected at least one step")
	}
	if got[0].ID != string(setup.StepInstallPackages) {
		t.Errorf("first step = %q; want %q (setup phase must lead)", got[0].ID, setup.StepInstallPackages)
	}
	last := got[len(got)-1]
	if last.ID != string(postinstall.StepDisableRHDefaults) {
		t.Errorf("last step = %q; want %q (postinstall phase must trail)", last.ID, postinstall.StepDisableRHDefaults)
	}

	var sawInstallPhase bool
	for _, s := range got {
		if s.ID == string(install.StepDeployInfra) {
			sawInstallPhase = true
		}
		if s.Name == "" {
			t.Errorf("step %q has empty display name", s.ID)
		}
	}
	if !sawInstallPhase {
		t.Error("install phase steps missing from dry-run listing")
	}
}

func TestPrintDeployDryRunBoxesHasOneBlankLineBetweenBoxes(t *testing.T) {
	tui.SetColorProfileFor(&bytes.Buffer{})
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })

	cfg := config.DefaultConfig()
	root := t.TempDir()
	changes := []terraform.ResourceChange{{Address: "module.vm.worker[2]", Action: terraform.PlanActionUpdate}}

	var buf bytes.Buffer
	printDeployDryRunBoxes(&buf, changes, cfg, root)
	out := buf.String()

	if !strings.Contains(out, "╯\n\n╭") {
		t.Errorf("plan-preview and step-listing boxes must be separated by exactly one blank line:\n%q", out)
	}
	if strings.Contains(out, "╯\n\n\n") {
		t.Errorf("plan-preview and step-listing boxes must not carry a double blank line:\n%q", out)
	}
}

func TestRunDeployYesWithoutConfigExitsNoInput(t *testing.T) {
	t.Chdir(t.TempDir())
	deployYes = true
	t.Cleanup(func() { deployYes = false })

	err := runDeploy(deployCmd, nil)
	if !errors.Is(err, errtypes.ErrConfigMissing) {
		t.Fatalf("want ErrConfigMissing (exit 66), got %v", err)
	}
	if exitCodeFor(err) != 66 {
		t.Fatalf("exitCodeFor = %d, want 66", exitCodeFor(err))
	}
}

// TestRunDeployDryRunWithoutConfigFailsFast guards item 6 of the
// second-cut safety findings: with no okdctl.yaml, --dry-run used to fall
// back to DefaultConfig and run a real terraform init (network + disk).
// It must now fail fast with the same family message loadConfig produces
// for destroy/cleanup, touching terraform zero times — asserted via the
// terraformPlanPreviewFn seam, not by timing.
func TestRunDeployDryRunWithoutConfigFailsFast(t *testing.T) {
	t.Chdir(t.TempDir())
	deployDryRun = true
	t.Cleanup(func() { deployDryRun = false })

	calls := 0
	prev := terraformPlanPreviewFn
	terraformPlanPreviewFn = func(context.Context, *config.Config, planPreviewOptions) ([]terraform.ResourceChange, error) {
		calls++
		return nil, nil
	}
	t.Cleanup(func() { terraformPlanPreviewFn = prev })

	err := runDeploy(deployCmd, nil)
	if !errors.Is(err, errtypes.ErrConfigMissing) {
		t.Fatalf("want ErrConfigMissing, got %v", err)
	}
	want := errConfigNotFound("okdctl.yaml")
	if err.Error() != want.Error() {
		t.Errorf("message must match the destroy/cleanup family shape:\ngot  %v\nwant %v", err, want)
	}
	if calls != 0 {
		t.Errorf("dry-run must not touch terraform without a config, got %d plan-preview calls", calls)
	}
}

// TestRunDeployDryRunWithInvalidConfigFailsFast is the fold-in half of item
// 6: an existing-but-unparseable okdctl.yaml is the identical hazard as a
// missing one (arguably worse — a typo'd config plans compiled-in defaults
// instead of refusing outright), so --dry-run must fail fast on it too,
// with the same invalid-config message the --yes path already uses.
func TestRunDeployDryRunWithInvalidConfigFailsFast(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("okdctl.yaml", []byte("cluster: [this is not valid yaml"), 0o600); err != nil {
		t.Fatalf("seed invalid config: %v", err)
	}
	deployDryRun = true
	t.Cleanup(func() { deployDryRun = false })

	calls := 0
	prev := terraformPlanPreviewFn
	terraformPlanPreviewFn = func(context.Context, *config.Config, planPreviewOptions) ([]terraform.ResourceChange, error) {
		calls++
		return nil, nil
	}
	t.Cleanup(func() { terraformPlanPreviewFn = prev })

	err := runDeploy(deployCmd, nil)
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("want *errtypes.ConfigError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "cannot proceed in non-interactive mode with invalid config") {
		t.Errorf("message must match the --yes path's invalid-config shape, got %v", err)
	}
	if calls != 0 {
		t.Errorf("dry-run must not touch terraform on an invalid config, got %d plan-preview calls", calls)
	}
}

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

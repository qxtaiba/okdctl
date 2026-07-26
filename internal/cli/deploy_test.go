package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestRunFullDeployment_RejectsInvalidProviderConfig(t *testing.T) {
	cfg := &config.Config{
		Provider: config.ProviderConfig{
			Type: config.ProviderProxmox,
			Proxmox: &config.ProxmoxConfig{
				Host:       "px.local",
				Node:       "pve",
				Storage:    "local-lvm",
				ISOStorage: "${inject}",
			},
		},
	}
	err := runFullDeployment(context.Background(), cfg, io.Discard)
	var ce *errtypes.ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("want *errtypes.ConfigError, got %T: %v", err, err)
	}
}

func TestRunFullDeployment_RejectsBogusProviderType(t *testing.T) {
	// validateProvider no-ops on a non-proxmox type, so the gate must also
	// run the required/enum scopes that reject the type itself.
	cfg := &config.Config{
		Provider: config.ProviderConfig{
			Type: "Proxmox",
			Proxmox: &config.ProxmoxConfig{
				Host:       "px.local",
				Node:       "pve",
				Storage:    "local-lvm",
				ISOStorage: `x"inject`,
			},
		},
	}
	err := runFullDeployment(context.Background(), cfg, io.Discard)
	var ce *errtypes.ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("want *errtypes.ConfigError, got %T: %v", err, err)
	}
}

func TestDeployGateScope_CoversRenderSurfaces(t *testing.T) {
	for name, scope := range map[string]config.ValidationScope{
		"required":            config.ScopeRequired,
		"enums":               config.ScopeEnums,
		"provider":            config.ScopeProvider,
		"advanced networking": config.ScopeAdvancedNetworking,
		"networking":          config.ScopeNetworking,
		"http server":         config.ScopeHTTPServer,
	} {
		if !deployGateScope.HasScope(scope) {
			t.Errorf("deploy gate is missing the %s scope", name)
		}
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

// TestDeployYesWriteConfigFlagContract locks the assume-yes alignment:
// --yes deploys (like every sibling command's --yes), --write-config carries
// the old write-only meaning, and the two are mutually exclusive.
func TestDeployYesWriteConfigFlagContract(t *testing.T) {
	yes := deployCmd.Flags().Lookup("yes")
	if yes == nil || yes.Shorthand != "y" {
		t.Fatal("deploy must keep --yes with the -y shorthand")
	}
	wc := deployCmd.Flags().Lookup("write-config")
	if wc == nil {
		t.Fatal("deploy must register --write-config")
	}
	if wc.Shorthand != "" {
		t.Error("--write-config must stay long-form only (boolean tail flags carry no shorthand)")
	}
}

// TestRunDeployYesWithoutConfigExitsNoInput verifies that a non-interactive
// deploy refuses to run against compiled-in defaults: --yes with no config
// file on disk must exit 66 (ErrConfigMissing), not silently write config
// and exit 0 like the pre-v0.2.0 --yes did.
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

// TestRunDeployWriteConfigWritesWithoutDeploying verifies --write-config
// saves the configuration file and returns without deploying.
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

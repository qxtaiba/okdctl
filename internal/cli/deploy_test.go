package cli

import (
	"context"
	"errors"
	"io"
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

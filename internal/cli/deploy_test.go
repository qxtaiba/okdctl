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

func TestDeployDryRunSteps_IDs(t *testing.T) {
	want := []string{
		string(setup.StepInstallPackages),
		string(setup.StepInstallTools),
		string(setup.StepEnsureWorkDir),
		string(setup.StepDownloadTools),
		string(setup.StepGenerateConfig),
		string(setup.StepGenerateManifests),
		string(setup.StepGenerateKubeVIP),
		string(setup.StepInjectManifests),
		string(setup.StepCompactCluster),
		string(setup.StepGenerateIgnition),
		string(setup.StepInstallApache),
		string(setup.StepDeployIgnition),
		string(setup.StepVerifyWebServer),
		string(setup.StepBuildISOs),
		string(setup.StepUploadISOs),
		string(setup.StepGenerateTfvars),
		string(setup.StepConfigureHAProxy),
		string(setup.StepConfigureFirewall),
		string(setup.StepConfigureDNS),
		string(install.StepDeployInfra),
		string(install.StepWaitBootstrap),
		string(install.StepStartWorkers),
		string(install.StepSetupKubeconfig),
		string(install.StepValidateAccess),
		string(install.StepMonitorInstall),
		string(install.StepSetupAccess),
		string(postinstall.StepVerifyHealth),
		string(postinstall.StepCleanupBootstrap),
		string(postinstall.StepVerifyKubeVIP),
		string(postinstall.StepDeployProductionDNS),
		string(postinstall.StepInstallAddons),
	}

	got := deployDryRunSteps()
	if len(got) != len(want) {
		t.Fatalf("len = %d; want %d", len(got), len(want))
	}
	for i, step := range got {
		if step.ID != want[i] {
			t.Errorf("step[%d].ID = %q; want %q", i, step.ID, want[i])
		}
	}
}

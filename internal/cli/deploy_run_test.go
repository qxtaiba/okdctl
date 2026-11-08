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

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

const (
	fixturePassword = "s3cret-pw-bytes"
	fixtureAPIToken = "root@pam!ci=s3cret-token-bytes"
)

func resetDeployState(t *testing.T) {
	t.Helper()
	savedOutputFile, savedMinimal, savedWriteConfig := deployOutputFile, deployMinimal, deployWriteConfig
	t.Cleanup(func() {
		deployOutputFile, deployMinimal, deployWriteConfig = savedOutputFile, savedMinimal, savedWriteConfig
		runWizardFn = runWizardWithMode
		deployCmd.SetOut(nil)
	})
	deployOutputFile, deployMinimal, deployWriteConfig = "okdctl.yaml", false, false
	deployCmd.SetContext(context.Background())
	deployCmd.SetOut(io.Discard)
}

type wizardCapture struct {
	called       bool
	cfg          *config.Config
	configExists bool
}

func stubWizard(t *testing.T, res wizard.Result, verb steps.HubVerb, err error) *wizardCapture {
	t.Helper()
	rec := &wizardCapture{}
	runWizardFn = func(_ *cobra.Command, cfg *config.Config, exists bool) (hubOutcome, error) {
		rec.called = true
		rec.cfg = cfg
		rec.configExists = exists
		return hubOutcome{Result: res, Verb: verb}, err
	}
	return rec
}

func seedDeployConfig(t *testing.T) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "prod"
	if err := config.NewLoader().Save(cfg, "okdctl.yaml"); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

func TestPersistWizardConfig_SecretHygiene(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "okdctl.yaml")

	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Username = "root@pam"
	cfg.Provider.Proxmox.Password.Set(fixturePassword)
	cfg.Provider.Proxmox.APIToken.Set(fixtureAPIToken)

	if err := persistWizardConfig(cfg, path, io.Discard); err != nil {
		t.Fatalf("persistWizardConfig: %v", err)
	}

	yamlBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	for _, secret := range []string{fixturePassword, fixtureAPIToken} {
		if bytes.Contains(yamlBytes, []byte(secret)) {
			t.Errorf("saved YAML contains credential bytes %q", secret)
		}
	}

	envPath := credentials.EnvFilePath(path)
	fi, err := os.Stat(envPath)
	if err != nil {
		t.Fatalf(".env sidecar missing: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf(".env perms = %#o; want 0600", perm)
	}
	envBytes, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	for _, secret := range []string{fixturePassword, fixtureAPIToken} {
		if !bytes.Contains(envBytes, []byte(secret)) {
			t.Errorf(".env sidecar missing credential %q — sidecar write must precede the in-memory clear", secret)
		}
	}

	if !cfg.Provider.Proxmox.Password.IsEmpty() || !cfg.Provider.Proxmox.APIToken.IsEmpty() {
		t.Error("in-memory credentials must be cleared before saveConfig runs")
	}
}

func TestPersistWizardConfig_NoCredentialsWritesNoSidecar(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "okdctl.yaml")

	if err := persistWizardConfig(config.DefaultConfig(), path, io.Discard); err != nil {
		t.Fatalf("persistWizardConfig: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config must still be saved: %v", err)
	}
	if _, err := os.Stat(credentials.EnvFilePath(path)); !os.IsNotExist(err) {
		t.Errorf("no credentials must mean no sidecar, got stat err %v", err)
	}
}

func TestRunDeploy_WizardCancelMakesNoChanges(t *testing.T) {
	resetDeployState(t)
	t.Chdir(t.TempDir())
	stubWizard(t, wizard.Result{Outcome: wizard.OutcomeCancelled}, steps.HubVerbGetStarted, nil)

	if err := runDeploy(deployCmd, nil); err != nil {
		t.Fatalf("cancelled wizard must exit 0: %v", err)
	}
	if _, err := os.Stat("okdctl.yaml"); !os.IsNotExist(err) {
		t.Errorf("cancelled wizard must not write config, got stat err %v", err)
	}
}

func TestRunDeploy_ExistingConfigSeedsWizard(t *testing.T) {
	resetDeployState(t)
	t.Chdir(t.TempDir())
	seedDeployConfig(t)
	rec := stubWizard(t, wizard.Result{Outcome: wizard.OutcomeCancelled}, steps.HubVerbEditConfig, nil)

	if err := runDeploy(deployCmd, nil); err != nil {
		t.Fatalf("runDeploy: %v", err)
	}
	if !rec.called || !rec.configExists {
		t.Fatalf("wizard must receive configExists=true, got called=%v exists=%v", rec.called, rec.configExists)
	}
	if rec.cfg.Cluster.Name != "prod" {
		t.Errorf("wizard seeded with %q; want the on-disk config", rec.cfg.Cluster.Name)
	}
}

func TestRunDeploy_CorruptConfigFallsBackToDefaultsInteractively(t *testing.T) {
	resetDeployState(t)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("okdctl.yaml", []byte("{{{ not yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := stubWizard(t, wizard.Result{Outcome: wizard.OutcomeCancelled}, steps.HubVerbGetStarted, nil)

	if err := runDeploy(deployCmd, nil); err != nil {
		t.Fatalf("interactive run must fall back to defaults, got: %v", err)
	}
	if rec.configExists {
		t.Error("corrupt config must present as configExists=false to the wizard")
	}
	if want := config.DefaultConfig().Cluster.Name; rec.cfg.Cluster.Name != want {
		t.Errorf("wizard seeded with %q; want default %q", rec.cfg.Cluster.Name, want)
	}
}

func TestRunDeploy_MinimalSeedsMinimalDefaults(t *testing.T) {
	resetDeployState(t)
	t.Chdir(t.TempDir())
	rec := stubWizard(t, wizard.Result{Outcome: wizard.OutcomeCancelled}, steps.HubVerbGetStarted, nil)
	deployMinimal = true

	if err := runDeploy(deployCmd, nil); err != nil {
		t.Fatalf("runDeploy --minimal: %v", err)
	}
	if want := config.MinimalConfig().Cluster.Name; rec.cfg.Cluster.Name != want {
		t.Errorf("wizard seeded with %q; want minimal default %q", rec.cfg.Cluster.Name, want)
	}
	if rec.cfg.Topology.Workers.Count != 0 {
		t.Errorf("minimal config must have zero workers, got %d", rec.cfg.Topology.Workers.Count)
	}
}

func TestRunDeploy_WizardErrorIsConfigError(t *testing.T) {
	resetDeployState(t)
	t.Chdir(t.TempDir())
	stubWizard(t, wizard.Result{}, steps.HubVerbGetStarted, errors.New("tty exploded"))

	err := runDeploy(deployCmd, nil)
	var ce *errtypes.ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("want *errtypes.ConfigError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "wizard failed") {
		t.Errorf("error must name the wizard: %v", err)
	}
}

func TestRunDeploy_WizardSaveExitPersistsConfigAndSidecar(t *testing.T) {
	resetDeployState(t)
	t.Chdir(t.TempDir())

	wizardCfg := config.DefaultConfig()
	wizardCfg.Cluster.Name = "wizarded"
	wizardCfg.Provider.Proxmox.Username = "root@pam"
	wizardCfg.Provider.Proxmox.Password.Set(fixturePassword)
	stubWizard(t, wizard.Result{Outcome: wizard.OutcomeCompleted, Config: wizardCfg, Action: wizard.ActionExit}, steps.HubVerbGetStarted, nil)

	if err := runDeploy(deployCmd, nil); err != nil {
		t.Fatalf("runDeploy: %v", err)
	}

	yamlBytes, err := os.ReadFile("okdctl.yaml")
	if err != nil {
		t.Fatalf("config not saved: %v", err)
	}
	if bytes.Contains(yamlBytes, []byte(fixturePassword)) {
		t.Error("okdctl.yaml contains the wizard-entered password")
	}
	if !bytes.Contains(yamlBytes, []byte("wizarded")) {
		t.Error("okdctl.yaml must carry the wizard-returned config")
	}

	fi, err := os.Stat("okdctl.env")
	if err != nil {
		t.Fatalf("okdctl.env sidecar missing: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("okdctl.env perms = %#o; want 0600", perm)
	}
	if !wizardCfg.Provider.Proxmox.Password.IsEmpty() {
		t.Error("in-memory password must be cleared after the save pipeline")
	}
}

func TestRunDeploy_HubQuitVerbChangesNothing(t *testing.T) {
	resetDeployState(t)
	t.Chdir(t.TempDir())
	stubWizard(t, wizard.Result{Outcome: wizard.OutcomeCompleted}, steps.HubVerbQuit, nil)

	if err := runDeploy(deployCmd, nil); err != nil {
		t.Fatalf("runDeploy: %v", err)
	}
	if _, err := os.Stat("okdctl.yaml"); !os.IsNotExist(err) {
		t.Errorf("the quit verb must not write a config, got stat err %v", err)
	}
}

func TestDeploy_SaveAndExitPrintsLaterRelease(t *testing.T) {
	resetDeployState(t)
	t.Chdir(t.TempDir())
	var out bytes.Buffer
	deployCmd.SetOut(&out)
	stubWizard(t, wizard.Result{Outcome: wizard.OutcomeCompleted, Config: config.DefaultConfig(), Action: wizard.ActionDeploy}, steps.HubVerbGetStarted, nil)

	if err := runDeploy(deployCmd, nil); err != nil {
		t.Fatalf("runDeploy: %v", err)
	}
	if want := "installation returns in a later release; the config was saved to okdctl.yaml\n"; !strings.Contains(out.String(), want) {
		t.Errorf("stdout = %q, want it to contain %q", out.String(), want)
	}
	if _, err := os.Stat("okdctl.yaml"); err != nil {
		t.Errorf("the config must be saved before the handoff line: %v", err)
	}
}

func TestRunDeploy_HubVerbsPrintHandoffsAndChangeNothing(t *testing.T) {
	for verb, want := range map[steps.HubVerb]string{
		steps.HubVerbDeploy:        "installation returns in a later release",
		steps.HubVerbDestroy:       "run: okdctl destroy",
		steps.HubVerbClusterStatus: "run: okdctl status",
		steps.HubVerbManageNodes:   "node management returns in a later release",
	} {
		t.Run(want, func(t *testing.T) {
			resetDeployState(t)
			t.Chdir(t.TempDir())
			seedDeployConfig(t)
			var out bytes.Buffer
			deployCmd.SetOut(&out)
			stubWizard(t, wizard.Result{Outcome: wizard.OutcomeCompleted}, verb, nil)

			if err := runDeploy(deployCmd, nil); err != nil {
				t.Fatalf("runDeploy: %v", err)
			}
			if !strings.Contains(out.String(), want) {
				t.Errorf("stdout = %q, want %q", out.String(), want)
			}
		})
	}
}

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
	"github.com/qxtaiba/okdctl/internal/deploy"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/testutil"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

const (
	fixturePassword = "s3cret-pw-bytes"
	fixtureAPIToken = "root@pam!ci=s3cret-token-bytes"
)

// resetDeployState zeroes the deploy command's package-level flag variables
// and seams, restoring caller-time values on cleanup, mirroring
// resetDestroyFlags.
func resetDeployState(t *testing.T) {
	t.Helper()
	savedOutputFile, savedConfirm := deployOutputFile, deployConfirmCluster
	savedMinimal, savedYes, savedWriteConfig := deployMinimal, deployYes, deployWriteConfig
	savedDryRun, savedFresh, savedKeep := deployDryRun, deployFresh, deployKeepRedHatCatalogs
	t.Cleanup(func() {
		deployOutputFile, deployConfirmCluster = savedOutputFile, savedConfirm
		deployMinimal, deployYes, deployWriteConfig = savedMinimal, savedYes, savedWriteConfig
		deployDryRun, deployFresh, deployKeepRedHatCatalogs = savedDryRun, savedFresh, savedKeep
		runWizardFn = runWizardWithMode
		deployExecuteFn = deploy.Execute
		deployCmd.SetOut(nil)
	})
	deployOutputFile, deployConfirmCluster = "okdctl.yaml", ""
	deployMinimal, deployYes, deployWriteConfig = false, false, false
	deployDryRun, deployFresh, deployKeepRedHatCatalogs = false, false, false
	deployCmd.SetContext(context.Background())
	deployCmd.SetOut(io.Discard)
}

// isolateProxmoxEnv pins every credential env var to the empty string so
// runFullDeployment's LoadEnvFile pass can never plant values that outlive
// the test (LoadEnvFile skips keys that already exist, and t.Setenv
// restores them).
func isolateProxmoxEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"PROXMOX_VE_ENDPOINT", "PROXMOX_VE_USERNAME", "PROXMOX_VE_PASSWORD",
		"PROXMOX_VE_API_TOKEN", "PROXMOX_VE_INSECURE",
	} {
		t.Setenv(k, "")
	}
}

// wizardCapture records what runDeploy handed the wizard seam.
type wizardCapture struct {
	called       bool
	cfg          *config.Config
	configExists bool
}

func stubWizard(t *testing.T, res wizard.Result, mode steps.WelcomeMode, err error) *wizardCapture {
	t.Helper()
	rec := &wizardCapture{}
	runWizardFn = func(_ context.Context, cfg *config.Config, exists bool) (wizard.Result, steps.WelcomeMode, error) {
		rec.called = true
		rec.cfg = cfg
		rec.configExists = exists
		return res, mode, err
	}
	return rec
}

func forbidWizard(t *testing.T) {
	t.Helper()
	runWizardFn = func(context.Context, *config.Config, bool) (wizard.Result, steps.WelcomeMode, error) {
		t.Error("wizard must not be constructed on this path")
		return wizard.Result{Cancelled: true}, steps.WelcomeModeEdit, nil
	}
}

// executeCapture records the deployExecuteFn seam's invocation. credsValid
// snapshots opts.Credentials.IsValid() at call time, because the caller's
// deferred Zeroize wipes the credential bytes before the test can assert.
type executeCapture struct {
	called     bool
	cfg        *config.Config
	opts       deploy.Options
	credsValid bool
}

func stubExecute(t *testing.T, ret error) *executeCapture {
	t.Helper()
	rec := &executeCapture{}
	deployExecuteFn = func(_ context.Context, cfg *config.Config, opts deploy.Options, _ io.Writer) error {
		rec.called = true
		rec.cfg = cfg
		rec.opts = opts
		rec.credsValid = opts.Credentials != nil && opts.Credentials.IsValid()
		return ret
	}
	return rec
}

func forbidExecute(t *testing.T) {
	t.Helper()
	deployExecuteFn = func(context.Context, *config.Config, deploy.Options, io.Writer) error {
		t.Error("deployment engine must not run on this path")
		return nil
	}
}

func seedDeployConfig(t *testing.T) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "prod"
	if err := config.NewLoader().Save(cfg, "okdctl.yaml"); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

// TestPersistWizardConfig_SecretHygiene pins the save-pipeline ordering:
// credentials reach the .env sidecar (0600), the in-memory secrets are
// cleared, and the YAML written afterwards carries zero credential bytes.
// Reordering the save before the clear, or the clear before the sidecar
// write, fails this test.
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
	forbidExecute(t)
	stubWizard(t, wizard.Result{Cancelled: true}, steps.WelcomeModeFresh, nil)

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
	forbidExecute(t)
	rec := stubWizard(t, wizard.Result{Cancelled: true}, steps.WelcomeModeEdit, nil)

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
	forbidExecute(t)
	rec := stubWizard(t, wizard.Result{Cancelled: true}, steps.WelcomeModeFresh, nil)

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

func TestRunDeploy_YesWithCorruptConfigIsConfigError(t *testing.T) {
	resetDeployState(t)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("okdctl.yaml", []byte("{{{ not yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	forbidExecute(t)
	forbidWizard(t)
	deployYes = true

	err := runDeploy(deployCmd, nil)
	var ce *errtypes.ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("want *errtypes.ConfigError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "non-interactive") {
		t.Errorf("refusal should say the non-interactive mode is the reason: %v", err)
	}
}

func TestRunDeploy_MinimalSeedsMinimalDefaults(t *testing.T) {
	resetDeployState(t)
	t.Chdir(t.TempDir())
	forbidExecute(t)
	rec := stubWizard(t, wizard.Result{Cancelled: true}, steps.WelcomeModeFresh, nil)
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
	forbidExecute(t)
	stubWizard(t, wizard.Result{}, steps.WelcomeModeFresh, errors.New("tty exploded"))

	err := runDeploy(deployCmd, nil)
	var ce *errtypes.ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("want *errtypes.ConfigError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "wizard failed") {
		t.Errorf("error must name the wizard: %v", err)
	}
}

// TestRunDeploy_WizardSaveExitPersistsConfigAndSidecar drives the full
// post-wizard save pipeline through runDeploy: the wizard-returned config's
// credentials land in okdctl.env (0600), okdctl.yaml holds none of them,
// and the deployment engine is not invoked on the exit action.
func TestRunDeploy_WizardSaveExitPersistsConfigAndSidecar(t *testing.T) {
	resetDeployState(t)
	t.Chdir(t.TempDir())
	forbidExecute(t)

	wizardCfg := config.DefaultConfig()
	wizardCfg.Cluster.Name = "wizarded"
	wizardCfg.Provider.Proxmox.Username = "root@pam"
	wizardCfg.Provider.Proxmox.Password.Set(fixturePassword)
	stubWizard(t, wizard.Result{Completed: true, Config: wizardCfg, Action: wizard.ActionExit}, steps.WelcomeModeFresh, nil)

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

func TestRunDeploy_WizardDeployActionExecutes(t *testing.T) {
	resetDeployState(t)
	isolateProxmoxEnv(t)
	t.Chdir(t.TempDir())
	exec := stubExecute(t, nil)

	wizardCfg := config.DefaultConfig()
	wizardCfg.Cluster.Name = "deployme"
	stubWizard(t, wizard.Result{Completed: true, Config: wizardCfg, Action: wizard.ActionDeploy}, steps.WelcomeModeFresh, nil)

	if err := runDeploy(deployCmd, nil); err != nil {
		t.Fatalf("runDeploy: %v", err)
	}
	if !exec.called {
		t.Fatal("ActionDeploy must invoke the deployment engine")
	}
	if exec.cfg.Cluster.Name != "deployme" {
		t.Errorf("engine received cluster %q; want the wizard config", exec.cfg.Cluster.Name)
	}
	if !exec.opts.ShowStartMessage {
		t.Error("cli deploy must request the start message")
	}
	if exec.opts.ProjectRoot == "" || !filepath.IsAbs(exec.opts.ProjectRoot) {
		t.Errorf("ProjectRoot must be a resolved absolute path, got %q", exec.opts.ProjectRoot)
	}
	if _, err := os.Stat("okdctl.yaml"); err != nil {
		t.Errorf("config must be saved before deploying: %v", err)
	}
}

func TestRunDeploy_WelcomeModeDeploySkipsSave(t *testing.T) {
	resetDeployState(t)
	isolateProxmoxEnv(t)
	t.Chdir(t.TempDir())
	seedDeployConfig(t)
	exec := stubExecute(t, nil)
	stubWizard(t, wizard.Result{Completed: true}, steps.WelcomeModeDeploy, nil)

	if err := runDeploy(deployCmd, nil); err != nil {
		t.Fatalf("runDeploy: %v", err)
	}
	if !exec.called {
		t.Fatal("welcome-mode deploy must invoke the deployment engine")
	}
	if exec.cfg.Cluster.Name != "prod" {
		t.Errorf("engine received cluster %q; want the pre-wizard on-disk config", exec.cfg.Cluster.Name)
	}
	if _, err := os.Stat("okdctl.env"); !os.IsNotExist(err) {
		t.Errorf("welcome-mode deploy must skip the save pipeline, got stat err %v", err)
	}
}

// TestRunDeploy_DryRunShortCircuitsBeforeWizard verifies --dry-run previews
// the plan and step listing without constructing the wizard or the
// deployment engine.
func TestRunDeploy_DryRunShortCircuitsBeforeWizard(t *testing.T) {
	resetDeployState(t)
	isolateProxmoxEnv(t)
	t.Chdir(t.TempDir())
	forbidWizard(t)
	forbidExecute(t)
	testutil.InstallFakeBin(t, "terraform", "#!/bin/sh\nexit 0\n")
	deployDryRun = true

	var out bytes.Buffer
	deployCmd.SetOut(&out)

	if err := runDeploy(deployCmd, nil); err != nil {
		t.Fatalf("dry-run must succeed with a clean plan: %v", err)
	}
	if !strings.Contains(out.String(), "dry-run — no changes made") {
		t.Errorf("dry-run summary missing from output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "DEPLOY STEP LISTING") {
		t.Errorf("step listing missing from output:\n%s", out.String())
	}
}

// TestRunDeploy_HeadlessGuard pins the non-interactive contract:
// 'deploy --yes --confirm-cluster <name>' executes the deployment engine
// without constructing the wizard; --yes alone, or with a mismatched name,
// refuses before any deploy work.
func TestRunDeploy_HeadlessGuard(t *testing.T) {
	t.Run("--yes without --confirm-cluster names the required flag", func(t *testing.T) {
		resetDeployState(t)
		t.Chdir(t.TempDir())
		seedDeployConfig(t)
		forbidWizard(t)
		forbidExecute(t)
		deployYes = true

		err := runDeploy(deployCmd, nil)
		var ce *errtypes.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("want *errtypes.ConfigError, got %T: %v", err, err)
		}
		if !strings.Contains(err.Error(), "--confirm-cluster") {
			t.Errorf("refusal must point at --confirm-cluster: %v", err)
		}
	})

	t.Run("--yes with mismatched --confirm-cluster refuses", func(t *testing.T) {
		resetDeployState(t)
		t.Chdir(t.TempDir())
		seedDeployConfig(t)
		forbidWizard(t)
		forbidExecute(t)
		deployYes = true
		deployConfirmCluster = "staging"

		err := runDeploy(deployCmd, nil)
		var ce *errtypes.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("want *errtypes.ConfigError, got %T: %v", err, err)
		}
		if !strings.Contains(err.Error(), "does not match") {
			t.Errorf("refusal must state the mismatch: %v", err)
		}
	})

	t.Run("--yes --confirm-cluster deploys headless with env credentials", func(t *testing.T) {
		resetDeployState(t)
		isolateProxmoxEnv(t)
		t.Setenv("PROXMOX_VE_API_TOKEN", fixtureAPIToken)
		t.Chdir(t.TempDir())
		seedDeployConfig(t)
		forbidWizard(t)
		exec := stubExecute(t, nil)
		deployYes = true
		deployConfirmCluster = "prod"

		if err := runDeploy(deployCmd, nil); err != nil {
			t.Fatalf("headless deploy: %v", err)
		}
		if !exec.called {
			t.Fatal("headless deploy must invoke the deployment engine")
		}
		if exec.cfg.Cluster.Name != "prod" {
			t.Errorf("engine received cluster %q; want the on-disk config", exec.cfg.Cluster.Name)
		}
		if !exec.credsValid {
			t.Error("engine must receive the resolved env credentials")
		}
		if !exec.opts.ShowStartMessage {
			t.Error("headless deploy must keep the start message")
		}
	})
}

// TestDeployConfirmClusterFlagContract mirrors the sibling-command checks:
// --confirm-cluster is registered and stays long-form only.
func TestDeployConfirmClusterFlagContract(t *testing.T) {
	f := deployCmd.Flags().Lookup("confirm-cluster")
	if f == nil {
		t.Fatal("deploy must register --confirm-cluster")
	}
	if f.Shorthand != "" {
		t.Error("--confirm-cluster must stay long-form only (not in the shorthand allowlist)")
	}
}

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

const fixtureAPIToken = "root@pam!ci=s3cret-token-bytes"

func resetInitState(t *testing.T) {
	t.Helper()
	savedOutputFile, savedMinimal := initOutputFile, initMinimal
	t.Cleanup(func() {
		initOutputFile, initMinimal = savedOutputFile, savedMinimal
		runWizardFn = runWizardWithMode
		initCmd.SetOut(nil)
	})
	initOutputFile, initMinimal = "okdctl.yaml", false
	initCmd.SetContext(context.Background())
	initCmd.SetOut(io.Discard)
	t.Chdir(t.TempDir())
}

type wizardCapture struct {
	called       bool
	cfg          *config.Config
	configExists bool
}

func stubWizard(t *testing.T, res *wizard.Result, verb steps.HubVerb, err error) *wizardCapture {
	t.Helper()
	rec := &wizardCapture{}
	runWizardFn = func(_ *cobra.Command, cfg *config.Config, exists bool) (hubOutcome, error) {
		rec.called = true
		rec.cfg = cfg
		rec.configExists = exists
		return hubOutcome{Result: *res, Verb: verb}, err
	}
	return rec
}

func seedInitConfig(t *testing.T) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "prod"
	if err := config.NewLoader().Save(cfg, "okdctl.yaml"); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

func TestRunInit_MinimalWritesWithoutTheWizard(t *testing.T) {
	resetInitState(t)
	initMinimal = true
	rec := stubWizard(t, &wizard.Result{}, steps.HubVerbGetStarted, nil)

	if err := runInit(initCmd, nil); err != nil {
		t.Fatalf("runInit --minimal: %v", err)
	}
	if rec.called {
		t.Fatal("--minimal must not open the wizard")
	}
	cfg, err := config.NewLoader().LoadFile("okdctl.yaml")
	if err != nil {
		t.Fatalf("--minimal must write a loadable okdctl.yaml: %v", err)
	}
	if cfg.Cluster.Name != "minimal" || cfg.Workers.Count != 0 {
		t.Errorf("wrote %q with %d workers, want the minimal config", cfg.Cluster.Name, cfg.Workers.Count)
	}

	err = runInit(initCmd, nil)
	var ce *errtypes.ConfigError
	if !errors.As(err, &ce) || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("a second --minimal must refuse to overwrite, got %v", err)
	}
}

func TestWriteCredentialsEnv(t *testing.T) {
	const loadedToken = "root@pam!ci=loaded-token"
	const shellToken = "root@pam!shell=exported-token"
	cases := []struct {
		name          string
		envToken      string
		fileToken     string
		envInsecure   string
		typedToken    string
		toggle        bool
		wantFile      bool
		wantLines     []string
		unwantedLines []string
	}{
		{"untouched session writes nothing", loadedToken, loadedToken, "true", "", true, false, nil, nil},
		{"no token anywhere and toggle off writes nothing", "", "", "", "", false, false, nil, nil},
		{
			"flip insecure only keeps the file's token", loadedToken, loadedToken, "", "", true, true,
			[]string{"PROXMOX_VE_API_TOKEN=" + loadedToken, "PROXMOX_VE_INSECURE=true"},
			nil,
		},
		{
			"flip insecure never copies a shell export over the file's token", shellToken, loadedToken, "", "", true, true,
			[]string{"PROXMOX_VE_API_TOKEN=" + loadedToken, "PROXMOX_VE_INSECURE=true"},
			[]string{shellToken},
		},
		{
			"flip insecure with a shell export and no file writes no token", shellToken, "", "", "", true, true,
			[]string{"PROXMOX_VE_INSECURE=true"},
			[]string{"PROXMOX_VE_API_TOKEN"},
		},
		{
			"flip insecure back off drops the flag", loadedToken, loadedToken, "true", "", false, true,
			[]string{"PROXMOX_VE_API_TOKEN=" + loadedToken},
			[]string{"PROXMOX_VE_INSECURE"},
		},
		{
			"rotate token preserves insecure", loadedToken, loadedToken, "true", fixtureAPIToken, true, true,
			[]string{"PROXMOX_VE_API_TOKEN=" + fixtureAPIToken, "PROXMOX_VE_INSECURE=true"},
			[]string{loadedToken},
		},
		{
			"first token with verification on", "", "", "", fixtureAPIToken, false, true,
			[]string{"PROXMOX_VE_API_TOKEN=" + fixtureAPIToken},
			[]string{"PROXMOX_VE_INSECURE"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PROXMOX_VE_API_TOKEN", tc.envToken)
			t.Setenv("PROXMOX_VE_INSECURE", tc.envInsecure)
			var secrets wizard.Secrets
			secrets.ProxmoxToken.Set(tc.typedToken)
			secrets.ProxmoxInsecure = tc.toggle
			configPath := filepath.Join(t.TempDir(), "okdctl.yaml")
			if tc.fileToken != "" {
				saved := &credentials.ProxmoxCredentials{APIToken: []byte(tc.fileToken), Insecure: tc.envInsecure == "true"}
				if err := credentials.WriteEnvFile(credentials.EnvFilePath(configPath), saved); err != nil {
					t.Fatal(err)
				}
			}

			envPath := credentials.EnvFilePath(configPath)
			before, _ := os.ReadFile(envPath)

			if err := writeCredentialsEnv(&secrets, configPath); err != nil {
				t.Fatalf("writeCredentialsEnv: %v", err)
			}
			fi, err := os.Stat(envPath)
			if !tc.wantFile {
				after, _ := os.ReadFile(envPath)
				if !bytes.Equal(after, before) {
					t.Fatalf("sidecar changed by an untouched session:\n%s", after)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected env file at %s: %v", envPath, err)
			}
			if perm := fi.Mode().Perm(); perm != 0o600 {
				t.Errorf("env perms = %#o, want 0600", perm)
			}
			data, _ := os.ReadFile(envPath)
			for _, want := range tc.wantLines {
				if !strings.Contains(string(data), want) {
					t.Errorf("env file missing %q; got:\n%s", want, data)
				}
			}
			for _, unwanted := range tc.unwantedLines {
				if strings.Contains(string(data), unwanted) {
					t.Errorf("env file carries %q; got:\n%s", unwanted, data)
				}
			}
		})
	}
}

func TestRunInit_WizardCancelMakesNoChanges(t *testing.T) {
	resetInitState(t)
	stubWizard(t, &wizard.Result{Outcome: wizard.OutcomeCancelled}, steps.HubVerbGetStarted, nil)

	if err := runInit(initCmd, nil); err != nil {
		t.Fatalf("cancelled wizard must exit 0: %v", err)
	}
	if _, err := os.Stat("okdctl.yaml"); !os.IsNotExist(err) {
		t.Errorf("cancelled wizard must not write config, got stat err %v", err)
	}
}

func TestRunInit_ExistingConfigSeedsWizard(t *testing.T) {
	resetInitState(t)
	seedInitConfig(t)
	rec := stubWizard(t, &wizard.Result{Outcome: wizard.OutcomeCancelled}, steps.HubVerbEditConfig, nil)

	if err := runInit(initCmd, nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if !rec.called || !rec.configExists || rec.cfg.Cluster.Name != "prod" {
		t.Fatalf("wizard seeded with called=%v exists=%v name=%q; want the on-disk config", rec.called, rec.configExists, rec.cfg.Cluster.Name)
	}
}

func TestRunInit_RejectedConfigIsRefused(t *testing.T) {
	cases := map[string]struct{ seed, want string }{
		"schema v2":    {"schemaVersion: v2\ncluster:\n  name: old\n", "okdctl.yaml is schema v2; this release reads v3 only"},
		"no schema":    {"cluster:\n  name: old\n", "okdctl.yaml has no schemaVersion; this release reads v3 only"},
		"corrupt yaml": {"{{{ not yaml", "parse config file okdctl.yaml"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resetInitState(t)
			if err := os.WriteFile("okdctl.yaml", []byte(tc.seed), 0o600); err != nil {
				t.Fatal(err)
			}
			rec := stubWizard(t, &wizard.Result{Outcome: wizard.OutcomeCancelled}, steps.HubVerbGetStarted, nil)

			err := runInit(initCmd, nil)
			var ce *errtypes.ConfigError
			if !errors.As(err, &ce) || exitCodeFor(err) != 2 || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "--output-file") {
				t.Fatalf("err = %v (exit %d); want a ConfigError exiting 2 with %q and the --output-file hint", err, exitCodeFor(err), tc.want)
			}
			if rec.called {
				t.Error("wizard opened over a rejected config")
			}
			if data, _ := os.ReadFile("okdctl.yaml"); string(data) != tc.seed {
				t.Errorf("init rewrote the rejected config:\n%s", data)
			}
		})
	}
}

func TestRunInit_WizardErrorIsConfigError(t *testing.T) {
	resetInitState(t)
	stubWizard(t, &wizard.Result{}, steps.HubVerbGetStarted, errors.New("tty exploded"))

	err := runInit(initCmd, nil)
	var ce *errtypes.ConfigError
	if !errors.As(err, &ce) || !strings.Contains(err.Error(), "wizard failed") {
		t.Fatalf("want a ConfigError naming the wizard, got %T: %v", err, err)
	}
}

func TestRunInit_SaveAndExitPersistsConfigAndSidecar(t *testing.T) {
	resetInitState(t)
	wizardCfg := config.DefaultConfig()
	wizardCfg.Cluster.Name = "wizarded"
	res := wizard.Result{Outcome: wizard.OutcomeCompleted, Config: wizardCfg, Action: wizard.ActionExit}
	res.Secrets.ProxmoxToken.Set(fixtureAPIToken)
	res.Secrets.ProxmoxInsecure = true
	stubWizard(t, &res, steps.HubVerbGetStarted, nil)

	if err := runInit(initCmd, nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	yamlBytes, err := os.ReadFile("okdctl.yaml")
	if err != nil {
		t.Fatalf("config not saved: %v", err)
	}
	if bytes.Contains(yamlBytes, []byte("s3cret")) || !bytes.Contains(yamlBytes, []byte("wizarded")) {
		t.Errorf("okdctl.yaml must carry the wizard config and never the token:\n%s", yamlBytes)
	}
	fi, err := os.Stat("okdctl.env")
	if err != nil {
		t.Fatalf("okdctl.env sidecar missing: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("okdctl.env perms = %#o; want 0600", perm)
	}
	envBytes, _ := os.ReadFile("okdctl.env")
	for _, want := range []string{"PROXMOX_VE_API_TOKEN=" + fixtureAPIToken, "PROXMOX_VE_INSECURE=true"} {
		if !bytes.Contains(envBytes, []byte(want)) {
			t.Errorf("okdctl.env missing %q:\n%s", want, envBytes)
		}
	}
	for _, b := range res.Secrets.ProxmoxToken.Bytes() {
		if b != 0 {
			t.Fatal("the token must be zeroized once the sidecar is written")
		}
	}
}

func TestRunInit_HubQuitVerbChangesNothing(t *testing.T) {
	resetInitState(t)
	stubWizard(t, &wizard.Result{Outcome: wizard.OutcomeCompleted}, steps.HubVerbQuit, nil)

	if err := runInit(initCmd, nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if _, err := os.Stat("okdctl.yaml"); !os.IsNotExist(err) {
		t.Errorf("the quit verb must not write a config, got stat err %v", err)
	}
}

func TestRunInit_DeployNowSavesThenHandsOffToUp(t *testing.T) {
	resetInitState(t)
	var out bytes.Buffer
	initCmd.SetOut(&out)
	stubWizard(t, &wizard.Result{Outcome: wizard.OutcomeCompleted, Config: config.DefaultConfig(), Action: wizard.ActionDeploy}, steps.HubVerbGetStarted, nil)

	if err := runInit(initCmd, nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if got := out.String(); got != "run: okdctl up\n" {
		t.Errorf("stdout = %q, want the up handoff line alone", got)
	}
	if _, err := os.Stat("okdctl.yaml"); err != nil {
		t.Errorf("the config must be saved before the handoff line: %v", err)
	}
	if _, err := os.Stat("okdctl.env"); !os.IsNotExist(err) {
		t.Errorf("no token typed means no sidecar, got stat err %v", err)
	}
}

func TestRunInit_HubVerbsPrintHandoffsAndChangeNothing(t *testing.T) {
	for verb, want := range map[steps.HubVerb]string{
		steps.HubVerbDeploy:        "run: okdctl up",
		steps.HubVerbDestroy:       "destroy returns in a later release",
		steps.HubVerbClusterStatus: "cluster status returns in a later release",
		steps.HubVerbManageNodes:   "node management returns in a later release",
	} {
		t.Run(want, func(t *testing.T) {
			resetInitState(t)
			seedInitConfig(t)
			var out bytes.Buffer
			initCmd.SetOut(&out)
			stubWizard(t, &wizard.Result{Outcome: wizard.OutcomeCompleted}, verb, nil)

			if err := runInit(initCmd, nil); err != nil {
				t.Fatalf("runInit: %v", err)
			}
			if !strings.Contains(out.String(), want) {
				t.Errorf("stdout = %q, want %q", out.String(), want)
			}
		})
	}
}

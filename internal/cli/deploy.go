package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

var (
	deployOutputFile  string
	deployMinimal     bool
	deployWriteConfig bool
)

// runWizardFn is a seam for TTY-free tests; production never reassigns it.
var runWizardFn = runWizardWithMode

var deployCmd = &cobra.Command{
	Use:   cmdNameDeploy,
	Short: "Configure an OKD cluster",
	Long: `Configure an OKD cluster through an interactive wizard and save the
result to a configuration file. Installation on the agent-based installer
returns in a later release; this release writes the configuration only.`,
	Example: `  okdctl deploy
  okdctl deploy --config my-cluster.yaml
  okdctl deploy --write-config --output-file my-cluster.yaml  # writes config only`,
	Args: cobra.NoArgs,
	RunE: runDeploy,
}

func init() {
	deployCmd.Flags().StringVar(&deployOutputFile, flagOutputFile, "okdctl.yaml", "config file to write wizard output to (reused if present; overrides --config)")
	deployCmd.Flags().BoolVar(&deployMinimal, "minimal", false, "use minimal defaults (single-node cluster)")
	deployCmd.Flags().BoolVar(&deployWriteConfig, "write-config", false, "write configuration non-interactively")
}

// laterReleaseHandoff is printed where an install would have started.
const laterReleaseHandoff = "installation returns in a later release"

func runDeploy(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	projectRoot, err := resolveWorkspaceRoot()
	if err != nil {
		return err
	}

	// --output-file wins when explicit; otherwise --config; both default to "okdctl.yaml".
	if !cmd.Flags().Changed(flagOutputFile) && cmd.Root().PersistentFlags().Changed(flagConfig) {
		deployOutputFile = cfgFile
	}

	cfg, configExists, err := loadDeployConfig(deployOutputFile)
	if err != nil {
		return err
	}

	if deployWriteConfig {
		return withProjectLock(projectRoot, "deploy --write-config", func() error {
			return saveConfig(cfg, deployOutputFile, out)
		})
	}

	outcome, err := runWizardFn(cmd, cfg, configExists)
	if err != nil {
		return (&errtypes.ConfigError{Msg: "wizard failed", Err: err}).
			WithHint("try again, or use --write-config to save a configuration without the wizard")
	}

	if outcome.Result.Outcome == wizard.OutcomeCancelled {
		logutil.Info("wizard cancelled, no changes made")
		return nil
	}

	if handled := runHubVerb(outcome.Verb, out); handled {
		return nil
	}

	cfg = outcome.Result.Config

	defer clearConfigCredentials(cfg)

	if err := withProjectLock(projectRoot, "deploy", func() error {
		return persistWizardConfig(cfg, deployOutputFile, out)
	}); err != nil {
		return err
	}

	switch outcome.Result.Action {
	case wizard.ActionDeploy:
		fmt.Fprintf(out, "%s; the config was saved to %s\n", laterReleaseHandoff, deployOutputFile)
	case wizard.ActionExit:
		fmt.Fprintln(out)
		logutil.Info("configuration saved", logutil.LF("path", deployOutputFile))
	}

	return nil
}

// runHubVerb handles the hub verbs that never walk the configure flow,
// reporting whether verb was one of them so the caller skips the save
// pipeline. Each prints the line that stands in for the command this release
// does not carry.
func runHubVerb(verb steps.HubVerb, out io.Writer) bool {
	switch verb {
	case steps.HubVerbDeploy:
		fmt.Fprintln(out, laterReleaseHandoff)
	case steps.HubVerbDestroy:
		fmt.Fprintln(out, "run: okdctl destroy")
	case steps.HubVerbClusterStatus:
		fmt.Fprintln(out, "run: okdctl status")
	case steps.HubVerbManageNodes:
		fmt.Fprintln(out, "node management returns in a later release")
	case steps.HubVerbQuit:
		logutil.Info("no changes made")
	default:
		return false
	}
	return true
}

// loadDeployConfig resolves the config the wizard starts from: an existing,
// valid on-disk file, or compiled-in defaults when none exists. An invalid
// file is fatal for --write-config and a warn-and-fall-back for the wizard.
func loadDeployConfig(path string) (cfg *config.Config, configExists bool, err error) {
	if _, statErr := os.Stat(path); statErr == nil {
		configExists = true
		loadedCfg, loadErr := config.NewLoader().LoadFile(path)
		if loadErr != nil {
			logutil.Warn("existing config could not be loaded", logutil.LF("err", loadErr))
			if deployWriteConfig {
				return nil, false, (&errtypes.ConfigError{Msg: "cannot proceed in non-interactive mode with invalid config", Err: loadErr}).
					WithHint("run 'okdctl config validate' to see what's wrong, or drop --write-config to use the wizard")
			}
			logutil.Info("starting fresh with defaults")
			configExists = false
		} else {
			cfg = loadedCfg
		}
	}

	if cfg == nil {
		if deployMinimal {
			cfg = config.MinimalConfig()
		} else {
			cfg = config.DefaultConfig()
		}
	}
	return cfg, configExists, nil
}

// withProjectLock serializes shared-file writes against concurrent okdctl
// invocations.
func withProjectLock(projectRoot, verb string, fn func() error) error {
	lock, err := runlock.Acquire(projectRoot, verb)
	if err != nil {
		return err
	}
	defer lock.Release()
	return fn()
}

// persistWizardConfig writes credentials to the .env sidecar, clears them, then
// saves YAML — in that order, so credential bytes never reach okdctl.yaml.
func persistWizardConfig(cfg *config.Config, path string, w io.Writer) error {
	if err := writeCredentialsEnv(cfg, path); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	clearConfigCredentials(cfg)
	return saveConfig(cfg, path, w)
}

func saveConfig(cfg *config.Config, path string, w io.Writer) error {
	if result := validateConfig(cfg, w); !result.IsValid() {
		logutil.Warn("configuration has validation warnings but will still be saved")
	}

	loader := config.NewLoader()
	if err := loader.Save(cfg, path); err != nil {
		return fmt.Errorf("save configuration: %w", err)
	}

	return nil
}

func writeCredentialsEnv(cfg *config.Config, configPath string) error {
	if cfg.Provider.Proxmox == nil {
		return nil
	}
	px := cfg.Provider.Proxmox

	if px.Password.IsEmpty() && px.APIToken.IsEmpty() {
		return nil
	}

	// Resolve the normalized endpoint so the .env file is self-contained.
	resolved := credentials.GetProxmoxCredentials(cfg)
	defer resolved.Zeroize()

	creds := &credentials.ProxmoxCredentials{
		Endpoint: resolved.Endpoint,
		Username: px.Username,
		Password: append([]byte(nil), px.Password.Bytes()...),
		APIToken: append([]byte(nil), px.APIToken.Bytes()...),
		Insecure: px.Insecure,
	}
	defer creds.Zeroize()

	envPath := credentials.EnvFilePath(configPath)
	if err := credentials.WriteEnvFile(envPath, creds); err != nil {
		return err
	}

	logutil.Info("credentials saved", logutil.LF("path", envPath))
	return nil
}

// clearConfigCredentials zeroizes credential bytes so they are never serialized
// or left lingering on the heap.
func clearConfigCredentials(cfg *config.Config) {
	if cfg.Provider.Proxmox == nil {
		return
	}
	cfg.Provider.Proxmox.Password.Zeroize()
	cfg.Provider.Proxmox.APIToken.Zeroize()
}

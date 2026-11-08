package cli

import (
	"bytes"
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
	initOutputFile string
	initMinimal    bool
)

// runWizardFn is a seam for TTY-free tests; production never reassigns it.
var runWizardFn = runWizardWithMode

var initCmd = &cobra.Command{
	Use:   cmdNameInit,
	Short: "Define a cluster and write okdctl.yaml",
	Long: `Define an OKD cluster through the interactive wizard and write it to
okdctl.yaml; the Proxmox API token goes to okdctl.env beside it. Installing
the cluster (okdctl up) returns in a later release.`,
	Example: `  okdctl init
  okdctl init --output-file my-cluster.yaml
  okdctl init --minimal   # write the minimal three-node config without the wizard`,
	Args: cobra.NoArgs,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVar(&initOutputFile, flagOutputFile, "okdctl.yaml", "config file to write (reused if present; overrides --config)")
	initCmd.Flags().BoolVar(&initMinimal, "minimal", false, "write the minimal three-node configuration without the wizard")
}

// upHandoff is printed where the install would start.
const upHandoff = "run: okdctl up"

func runInit(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	root, err := resolveWorkspaceRoot()
	if err != nil {
		return err
	}
	if !cmd.Flags().Changed(flagOutputFile) && cmd.Root().PersistentFlags().Changed(flagConfig) {
		initOutputFile = cfgFile
	}
	if initMinimal {
		return withProjectLock(root, cmdNameInit, func() error { return writeMinimalConfig(initOutputFile, out) })
	}

	cfg, configExists := loadInitConfig(initOutputFile)
	// A saved token lets node placement discover the cluster without retyping it.
	if err := credentials.LoadEnvFile(credentials.EnvFilePath(initOutputFile)); err != nil {
		return err
	}
	outcome, err := runWizardFn(cmd, cfg, configExists)
	if err != nil {
		return (&errtypes.ConfigError{Msg: "wizard failed", Err: err}).
			WithHint("try again, or use --minimal to write a configuration without the wizard")
	}
	if outcome.Result.Outcome == wizard.OutcomeCancelled {
		logutil.Info("wizard cancelled, no changes made")
		return nil
	}
	if runHubVerb(outcome.Verb, out) {
		return nil
	}

	result := &outcome.Result
	defer result.Secrets.ProxmoxToken.Zeroize()
	if err := withProjectLock(root, cmdNameInit, func() error { return persistWizardConfig(result, initOutputFile, out) }); err != nil {
		return err
	}
	switch result.Action {
	case wizard.ActionDeploy:
		fmt.Fprintln(out, upHandoff)
	case wizard.ActionExit:
		fmt.Fprintln(out)
		logutil.Info("configuration saved", logutil.LF("path", initOutputFile))
	}
	return nil
}

// runHubVerb prints the handoff line for a hub verb that never walks the
// configure flow, reporting whether verb was one of them.
func runHubVerb(verb steps.HubVerb, out io.Writer) bool {
	switch verb {
	case steps.HubVerbDeploy:
		fmt.Fprintln(out, upHandoff)
	case steps.HubVerbDestroy:
		fmt.Fprintln(out, "destroy returns in a later release")
	case steps.HubVerbClusterStatus:
		fmt.Fprintln(out, "cluster status returns in a later release")
	case steps.HubVerbManageNodes:
		fmt.Fprintln(out, "node management returns in a later release")
	case steps.HubVerbQuit:
		logutil.Info("no changes made")
	default:
		return false
	}
	return true
}

// loadInitConfig returns the valid on-disk config, or the compiled-in
// defaults with a warning when the file is missing or unreadable.
func loadInitConfig(path string) (cfg *config.Config, configExists bool) {
	if _, statErr := os.Stat(path); statErr == nil {
		loaded, loadErr := config.NewLoader().LoadFile(path)
		if loadErr == nil {
			return loaded, true
		}
		logutil.Warn("existing config could not be loaded; starting fresh with defaults", logutil.LF("err", loadErr))
	}
	return config.DefaultConfig(), false
}

func writeMinimalConfig(path string, w io.Writer) error {
	if _, err := os.Stat(path); err == nil {
		return (&errtypes.ConfigError{Msg: path + " already exists"}).
			WithHint("remove it, or pass --output-file to write elsewhere")
	}
	return saveConfig(config.MinimalConfig(), path, w)
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

func persistWizardConfig(result *wizard.Result, path string, w io.Writer) error {
	if err := saveConfig(result.Config, path, w); err != nil {
		return err
	}
	return writeCredentialsEnv(&result.Secrets, path)
}

func saveConfig(cfg *config.Config, path string, w io.Writer) error {
	if result := validateConfig(cfg, w); !result.IsValid() {
		logutil.Warn("configuration has validation warnings but will still be saved")
	}
	if err := config.NewLoader().Save(cfg, path); err != nil {
		return fmt.Errorf("save configuration: %w", err)
	}
	return nil
}

// writeCredentialsEnv rewrites the sidecar when a token was typed or the TLS
// toggle moved from the loaded value, carrying the loaded token forward when
// none was typed; an untouched session leaves the file alone.
func writeCredentialsEnv(secrets *wizard.Secrets, configPath string) error {
	loaded := credentials.GetProxmoxCredentials(nil)
	defer loaded.Zeroize()
	typed := !secrets.ProxmoxToken.IsEmpty()
	if !typed && secrets.ProxmoxInsecure == loaded.Insecure {
		return nil
	}
	creds := &credentials.ProxmoxCredentials{
		APIToken: bytes.Clone(loaded.APIToken),
		Insecure: secrets.ProxmoxInsecure,
	}
	if typed {
		creds.APIToken = bytes.Clone(secrets.ProxmoxToken.Bytes())
	}
	defer creds.Zeroize()
	envPath := credentials.EnvFilePath(configPath)
	if err := credentials.WriteEnvFile(envPath, creds); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	logutil.Info("credentials saved", logutil.LF("path", envPath))
	return nil
}

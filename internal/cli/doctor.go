package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/proxmox"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

var doctorOutput string

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check the Proxmox cluster against the configuration",
	Long: `Run the Proxmox preflight checks the installer relies on: API token
privileges, PVE version, CPU flags, free memory per host, VMID and address
collisions, storage content types and the bridge on every target node. Node
identities come from the configuration through the node model. The MAC
collision check runs once cluster/record.json names the infra-ID, and the
ISO size check runs at install time, once the agent ISO exists.

Read-only: nothing is created or changed. Exits 0 when every check passes or
only warns, 1 when any check fails, 2 when the configuration is invalid, 3
when Proxmox cannot be reached, 5 when the API token is missing or rejected,
64 on a usage error and 66 when the configuration file does not exist. With
--output json the findings array alone goes to stdout; it is empty when the
checks could not run.`,
	Example: `  okdctl doctor
  okdctl doctor --output json`,
	Args: cobra.NoArgs,
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().StringVarP(&doctorOutput, flagOutput, flagOutputShort, tui.FormatText, "output format: text|json")
	_ = doctorCmd.RegisterFlagCompletionFunc(flagOutput,
		cobra.FixedCompletions([]string{tui.FormatText, tui.FormatJSON}, cobra.ShellCompDirectiveNoFileComp))
	rootCmd.AddCommand(doctorCmd)
}

// runPreflight is the seam tests replace so doctor runs without a Proxmox server.
var runPreflight = func(ctx context.Context, d proxmox.Driver, in *proxmox.PreflightInput) ([]proxmox.Finding, error) {
	return proxmox.Preflight(ctx, d, proxmox.ExecPinger(executor.New()), *in)
}

// openDriver builds the Proxmox driver from creds and zeroizes the token;
// it is a variable so tests can observe the credentials doctor resolved.
var openDriver = func(creds *credentials.ProxmoxCredentials) (proxmox.Driver, error) {
	defer creds.Zeroize()
	d, err := proxmox.New(creds.Endpoint, creds.APIToken, creds.Insecure)
	if err != nil {
		return nil, &errtypes.ConfigError{Msg: "connect proxmox: " + err.Error(), Err: err}
	}
	return d, nil
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	if err := validateOutputFormat(doctorOutput); err != nil {
		return err
	}
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}
	if result := validateConfig(cfg, cmd.ErrOrStderr()); !result.IsValid() {
		return (&errtypes.ConfigError{Msg: "config validation failed", Err: result}).WithHint("review the errors listed above")
	}
	in, err := preflightInput(cfg, cfgFile)
	if err != nil {
		return err
	}
	creds, err := doctorCredentials(cfg, cfgFile)
	if err != nil {
		return err
	}
	d, err := openDriver(creds)
	if err != nil {
		return noFindings(cmd, err)
	}
	findings, err := runPreflight(cmd.Context(), d, &in)
	if err != nil {
		return noFindings(cmd, preflightError(err))
	}
	if err := printFindings(cmd.OutOrStdout(), doctorOutput, findings); err != nil {
		return err
	}
	return findingsError(findings)
}

// noFindings returns err after printing the empty array a json consumer
// expects when the checks could not run.
func noFindings(cmd *cobra.Command, err error) error {
	if doctorOutput == tui.FormatJSON {
		fmt.Fprintln(cmd.OutOrStdout(), "[]")
	}
	return err
}

// preflightError types a preflight that could not run: a rejected token is
// an AuthError, anything else a NetworkError, so failed checks alone exit 1.
func preflightError(err error) error {
	if errors.Is(err, proxmox.ErrPermission) {
		return (&errtypes.AuthError{Msg: "proxmox rejected the api token", Err: err}).
			WithHint("check PROXMOX_VE_API_TOKEN and the privileges granted to it")
	}
	scrubbed := fmt.Sprint(logutil.RedactableStderr(err.Error()).Redacted())
	return (&errtypes.NetworkError{Msg: "preflight: " + scrubbed, Err: err}).
		WithHint("check provider.proxmox.endpoint and that the Proxmox API answers")
}

func validateOutputFormat(format string) error {
	switch format {
	case tui.FormatText, tui.FormatJSON:
		return nil
	default:
		return &errtypes.UsageError{Msg: fmt.Sprintf("invalid --output %q (want text|json)", format)}
	}
}

func doctorCredentials(cfg *config.Config, configPath string) (*credentials.ProxmoxCredentials, error) {
	root, err := workspaceRoot(configPath)
	if err != nil {
		return nil, err
	}
	envPath := workspace.EnvPath(root)
	creds, err := credentials.Load(envPath, cfg.Provider.Proxmox.Endpoint)
	if err != nil {
		return nil, err
	}
	if !creds.IsValid() {
		creds.Zeroize()
		return nil, &errtypes.AuthError{Msg: "no proxmox api token: set PROXMOX_VE_API_TOKEN in " + envPath}
	}
	return creds, nil
}

type findingJSON struct {
	Check    string `json:"check"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

func printFindings(w io.Writer, format string, findings []proxmox.Finding) error {
	if format == tui.FormatJSON {
		out := make([]findingJSON, 0, len(findings))
		for _, f := range findings {
			out = append(out, findingJSON(f))
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal findings: %w", err)
		}
		_, err = fmt.Fprintln(w, string(data))
		return err
	}
	if len(findings) == 0 {
		_, err := fmt.Fprintln(w, "preflight passed")
		return err
	}
	rows := make([][]string, 0, len(findings))
	for _, f := range findings {
		rows = append(rows, []string{f.Check, f.Severity, f.Message})
	}
	return printTable(w, []string{"CHECK", "SEVERITY", "MESSAGE"}, rows)
}

func findingsError(findings []proxmox.Finding) error {
	n := 0
	for _, f := range findings {
		if f.Severity == proxmox.SeverityFail {
			n++
		}
	}
	switch n {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("preflight: 1 failing check")
	default:
		return fmt.Errorf("preflight: %d failing checks", n)
	}
}

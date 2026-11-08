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
	"github.com/qxtaiba/okdctl/internal/proxmox"
	"github.com/qxtaiba/okdctl/internal/tui"
)

var doctorOutput string

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check the Proxmox cluster against the configuration",
	Long: `Run the Proxmox preflight checks the installer relies on: API token
privileges, PVE version, CPU flags, free memory per host, ISO storage space,
VMID, MAC and address collisions, storage content types and the bridge on
every target node. Node identities come from the configuration through the
node model; the MAC check compares real addresses once cluster/record.json
names the infra-ID and passes vacuously before okdctl up writes it.

Read-only: nothing is created or changed. Exits 0 when every check passes or
only warns, 1 when any check fails.`,
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

// doctorChecks is how many checks Preflight runs, named in the all-clear line.
const doctorChecks = 10

// runPreflight is the seam tests replace so doctor runs without a Proxmox server.
var runPreflight = func(ctx context.Context, creds *credentials.ProxmoxCredentials, in *proxmox.PreflightInput) ([]proxmox.Finding, error) {
	d, err := proxmox.New(creds.Endpoint, creds.APIToken, creds.Insecure)
	creds.Zeroize()
	if err != nil {
		return nil, fmt.Errorf("connect proxmox: %w", err)
	}
	return proxmox.Preflight(ctx, d, proxmox.ExecPinger(executor.New()), *in)
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
	defer creds.Zeroize()
	findings, err := runPreflight(cmd.Context(), creds, &in)
	switch {
	case errors.Is(err, proxmox.ErrPermission):
		return (&errtypes.AuthError{Msg: "proxmox rejected the api token", Err: err}).
			WithHint("check PROXMOX_VE_API_TOKEN and the privileges granted to it")
	case err != nil:
		return fmt.Errorf("preflight: %w", err)
	}
	if err := printFindings(cmd.OutOrStdout(), doctorOutput, findings); err != nil {
		return err
	}
	return findingsError(findings)
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
	envPath := credentials.EnvFilePath(configPath)
	if err := credentials.LoadEnvFile(envPath); err != nil {
		return nil, err
	}
	creds := credentials.GetProxmoxCredentials(cfg)
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
		_, err := fmt.Fprintf(w, "preflight passed (%d checks)\n", doctorChecks)
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
		if f.Severity == "fail" {
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

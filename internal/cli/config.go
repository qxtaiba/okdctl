package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/model"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
)

var configValidateOutput string

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect okdctl configuration",
	Long:  "Show and validate the resolved okdctl configuration; start with 'config show' to see the active values.",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the configuration as the loader read it",
	Long: `Print the okdctl configuration as YAML. The file carries no secrets; the
Proxmox API token lives in okdctl.env beside it.`,
	Example: "  okdctl config show",
	Args:    cobra.NoArgs,
	RunE:    runConfigShow,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the configuration file and report errors",
	Long: `Validate the configuration file against the schema (required fields,
enum values, provider settings, topology constraints) and print every
error found. A valid file is followed by the node table it produces:
name, role, VMID, MAC, address, host, CPU, memory and disks. With
--output json the inventory alone goes to stdout and the summary to
stderr.

Read-only: nothing is written or deployed. Exits 0 when the config is
valid, 2 when validation reports errors, and 66 when the file does not
exist.`,
	Example: `  okdctl config validate
  okdctl config validate --output json`,
	Args: cobra.NoArgs,
	RunE: runConfigValidate,
}

func init() {
	configValidateCmd.Flags().StringVarP(&configValidateOutput, flagOutput, flagOutputShort, tui.FormatText, "output format: text|json")
	_ = configValidateCmd.RegisterFlagCompletionFunc(flagOutput,
		cobra.FixedCompletions([]string{tui.FormatText, tui.FormatJSON}, cobra.ShellCompDirectiveNoFileComp))
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configValidateCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigValidate(cmd *cobra.Command, _ []string) error {
	asJSON := configValidateOutput == tui.FormatJSON
	if !asJSON && configValidateOutput != tui.FormatText {
		return &errtypes.UsageError{Msg: fmt.Sprintf("invalid --output %q (want text|json)", configValidateOutput)}
	}
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	summaryOut := cmd.OutOrStdout()
	if asJSON {
		summaryOut = cmd.ErrOrStderr()
	}
	result := cfg.Validate()
	fmt.Fprintln(summaryOut, render.ValidationSummary(result))
	if !result.IsValid() {
		// The result stays in the Unwrap chain so callers can inspect the findings.
		return (&errtypes.ConfigError{Msg: "config validation failed", Err: result}).WithHint("review the errors listed above")
	}
	inv, err := model.Build(cfg, "preview")
	if err != nil {
		return &errtypes.ConfigError{Msg: "derive node inventory: " + err.Error(), Err: err}
	}
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(inv)
	}
	return printNodeTable(cmd.OutOrStdout(), &inv)
}

func printNodeTable(w io.Writer, inv *model.Inventory) error {
	rows := make([][]string, 0, len(inv.Nodes))
	for i := range inv.Nodes {
		n := &inv.Nodes[i]
		sizes := make([]string, 0, len(n.Disks))
		for _, d := range n.Disks {
			sizes = append(sizes, strconv.Itoa(d.SizeGiB))
		}
		rows = append(rows, []string{
			n.Name, string(n.Role), strconv.Itoa(n.VMID), n.MAC, n.IP.String(), n.Host,
			strconv.Itoa(n.CPU), strconv.Itoa(n.MemoryMiB), strings.Join(sizes, ","),
		})
	}
	return printTable(w, []string{"NAME", "ROLE", "VMID", "MAC", "IP", "HOST", "CPU", "MEM", "DISKS"}, rows, tui.TableOptions{})
}

func runConfigShow(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	_, err = cmd.OutOrStdout().Write(out)
	return err
}

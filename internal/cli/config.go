package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/render"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect okdctl configuration",
	Long:  "Show and validate the resolved okdctl configuration; start with 'config show' to see the active values.",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the resolved configuration with secrets redacted",
	Long: `Print the fully-resolved okdctl configuration as YAML with all secret
fields replaced by "***".`,
	Example: "  okdctl config show",
	Args:    cobra.NoArgs,
	RunE:    runConfigShow,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the configuration file and report errors",
	Long: `Validate the configuration file against the schema (required fields,
enum values, provider settings, topology constraints) and print every
error and warning found.

Read-only: nothing is written or deployed. Exits 0 when the config is
valid (warnings alone do not fail), 2 when validation reports errors,
and 66 when the file does not exist.`,
	Example: "  okdctl config validate",
	Args:    cobra.NoArgs,
	RunE:    runConfigValidate,
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configValidateCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigValidate(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	result := cfg.Validate()
	fmt.Fprintln(cmd.OutOrStdout(), render.ValidationSummary(result))
	if result == nil || result.IsValid() {
		return nil
	}
	// Mirrors runFullDeployment's gate shape: result stays in the Unwrap chain.
	return (&errtypes.ConfigError{Msg: "config validation failed", Err: result}).WithHint("review the errors listed above")
}

func runConfigShow(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	out, err := yaml.Marshal(config.Redacted(cfg))
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	_, err = cmd.OutOrStdout().Write(out)
	return err
}

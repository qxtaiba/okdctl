package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

const cfgVerb = "config"

var configCmd = &cobra.Command{
	Use:   cfgVerb,
	Short: "Inspect okdctl configuration",
	Long:  "Show and validate the resolved okdctl configuration; start with 'config show' to see the active values.",
}

var configShowOutput string

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the resolved configuration with secrets redacted",
	Long: `Print the fully-resolved okdctl configuration with all secret fields
replaced by "***". Text mode emits YAML; pass --output=json to get a JSON
object suitable for piping to jq.`,
	Example: `  okdctl config show
  okdctl config show --output json | jq '.provider'`,
	RunE: runConfigShow,
}

var configValidateCmd = &cobra.Command{
	Use:     "validate",
	Short:   "Validate the configuration file and report errors",
	Example: "  okdctl config validate",
	RunE:    runConfigValidate,
}

func init() {
	configShowCmd.Flags().StringVarP(&configShowOutput, flagOutput, flagOutputShort, outputText, "output format: text|json")
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
	fmt.Fprintln(cmd.OutOrStdout(), ValidationSummary(result))
	if result == nil || result.IsValid() {
		return nil
	}
	return &errtypes.ConfigError{Msg: result.Error()}
}

func runConfigShow(cmd *cobra.Command, _ []string) error {
	if err := validateFormat(configShowOutput); err != nil {
		return err
	}
	quietForJSON(configShowOutput)

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	redacted := config.Redacted(cfg)

	if configShowOutput == outputJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(redacted)
	}

	out, err := yaml.Marshal(redacted)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	_, err = cmd.OutOrStdout().Write(out)
	return err
}

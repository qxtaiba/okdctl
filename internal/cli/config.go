package cli

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
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

	redacted := redactConfig(cfg)

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

// redactConfig returns a deep copy of cfg with every string field whose JSON
// tag name matches the secret-key denylist replaced by "***". Fields tagged
// json:"-" (Password, APIToken, Username) are skipped — they never marshal
// into the bundle.
func redactConfig(cfg *config.Config) config.Config {
	out := *cfg
	redactValue(reflect.ValueOf(&out))
	return out
}

// redactValue walks v (must be addressable) masking secret-keyed string fields.
func redactValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		redactValue(v.Elem())
	case reflect.Struct:
		t := v.Type()
		for i := range t.NumField() {
			f := t.Field(i)
			jsonTag := f.Tag.Get("json")
			if jsonTag == "-" {
				continue
			}
			name := strings.SplitN(jsonTag, ",", 2)[0]
			fv := v.Field(i)
			if !fv.CanSet() {
				continue
			}
			switch fv.Kind() {
			case reflect.String:
				if logutil.KeyIsSecret(name) && fv.String() != "" {
					fv.SetString("***")
				}
			case reflect.Pointer:
				if fv.IsNil() {
					continue
				}
				clone := reflect.New(fv.Type().Elem())
				clone.Elem().Set(fv.Elem())
				fv.Set(clone)
				redactValue(fv)
			case reflect.Struct:
				redactValue(fv)
			}
		}
	}
}

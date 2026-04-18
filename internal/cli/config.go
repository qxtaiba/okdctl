package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect okdctl configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the resolved configuration with secrets redacted",
	RunE:  runConfigShow,
}

func init() {
	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigShow(_ *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	out, err := yaml.Marshal(redactConfig(cfg))
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	_, err = os.Stdout.Write(out)
	return err
}

// redactConfig returns a copy of cfg with sensitive Proxmox credential fields
// replaced by "***". Username, Password, and APIToken already carry json:"-"
// and are excluded from marshaling; only TokenID requires explicit redaction
// because it is emitted under the token_id key.
func redactConfig(cfg *config.Config) config.Config {
	out := *cfg
	if cfg.Provider.Proxmox != nil {
		px := *cfg.Provider.Proxmox
		if px.TokenID != "" {
			px.TokenID = "***"
		}
		out.Provider.Proxmox = &px
	}
	return out
}

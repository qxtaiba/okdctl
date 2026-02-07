package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View and manage cluster configuration",
	Long: `View and manage cluster configuration.

Subcommands:
  view    - Display the current configuration
  edit    - Open configuration in editor
  set     - Set a configuration value
  get     - Get a configuration value`,
}

var configViewCmd = &cobra.Command{
	Use:   "view",
	Short: "Display the current configuration",
	RunE:  runConfigView,
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit configuration interactively",
	Long:  "Open an interactive TUI to edit the configuration",
	RunE:  runConfigEdit,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value using dot notation.

Examples:
  openshitctl config set cluster.name mycluster
  openshitctl config set topology.workers.count 5
  openshitctl config set distribution.version 4.18.0-okd-scos.10`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long: `Get a configuration value using dot notation.

Examples:
  openshitctl config get cluster.name
  openshitctl config get topology.workers.count`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigGet,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show configuration file path",
	RunE:  runConfigPath,
}

func init() {
	configCmd.AddCommand(configViewCmd)
	configCmd.AddCommand(configEditCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configPathCmd)
}

func runConfigView(cmd *cobra.Command, args []string) error {
	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		return err
	}

	tui.Info("configuration file: " + cfgFile)
	fmt.Println()

	kvWidth := defaultContentWidth - 2

	var clusterContent string
	clusterContent = "\n"
	clusterContent += "  " + tui.DottedKeyValueHighlightFull("  name", cfg.Cluster.Name, defaultKeyColWidth, kvWidth) + "\n"
	clusterContent += "  " + tui.DottedKeyValueFull("  domain", cfg.Cluster.Domain, defaultKeyColWidth, kvWidth) + "\n"
	clusterContent += "\n"
	fmt.Println(tui.BoxedSectionCompact(clusterContent, "CLUSTER", tui.DefaultBoxWidth))
	fmt.Println()

	var distContent string
	distContent = "\n"
	distContent += "  " + tui.DottedKeyValueHighlightFull("  type", string(cfg.Distribution.Type), defaultKeyColWidth, kvWidth) + "\n"
	distContent += "  " + tui.DottedKeyValueFull("  version", cfg.Distribution.Version, defaultKeyColWidth, kvWidth) + "\n"
	distContent += "\n"
	fmt.Println(tui.BoxedSectionCompact(distContent, "DISTRIBUTION", tui.DefaultBoxWidth))
	fmt.Println()

	var providerContent string
	providerContent = "\n"
	providerContent += "  " + tui.DottedKeyValueHighlightFull("  type", string(cfg.Provider.Type), defaultKeyColWidth, kvWidth) + "\n"
	if cfg.Provider.Proxmox != nil {
		providerContent += "  " + tui.DottedKeyValueFull("  host", cfg.Provider.Proxmox.Host, defaultKeyColWidth, kvWidth) + "\n"
	}
	providerContent += "\n"
	fmt.Println(tui.BoxedSectionCompact(providerContent, "PROVIDER", tui.DefaultBoxWidth))
	fmt.Println()

	var cpContent string
	cpContent = "\n"
	cpContent += "  " + tui.DottedKeyValueHighlightFull("  nodes", fmt.Sprintf("%d", cfg.Topology.ControlPlane.Count), defaultKeyColWidth, kvWidth) + "\n"
	cpContent += "  " + tui.DottedKeyValueFull("  cpu", fmt.Sprintf("%dvCPU/node", cfg.Topology.ControlPlane.CPU), defaultKeyColWidth, kvWidth) + "\n"
	cpMemGB := cfg.Topology.ControlPlane.Memory / 1024
	cpContent += "  " + tui.DottedKeyValueFull("  memory", fmt.Sprintf("%dGB/node", cpMemGB), defaultKeyColWidth, kvWidth) + "\n"
	cpContent += "  " + tui.DottedKeyValueFull("  disk", fmt.Sprintf("%dGB/node", cfg.Topology.ControlPlane.Disk), defaultKeyColWidth, kvWidth) + "\n"
	cpContent += "\n"
	fmt.Println(tui.BoxedSectionCompact(cpContent, "CONTROL PLANE", tui.DefaultBoxWidth))
	fmt.Println()

	var wContent string
	wContent = "\n"
	wContent += "  " + tui.DottedKeyValueHighlightFull("  nodes", fmt.Sprintf("%d", cfg.Topology.Workers.Count), defaultKeyColWidth, kvWidth) + "\n"
	wContent += "  " + tui.DottedKeyValueFull("  cpu", fmt.Sprintf("%dvCPU/node", cfg.Topology.Workers.CPU), defaultKeyColWidth, kvWidth) + "\n"
	wMemGB := cfg.Topology.Workers.Memory / 1024
	wContent += "  " + tui.DottedKeyValueFull("  memory", fmt.Sprintf("%dGB/node", wMemGB), defaultKeyColWidth, kvWidth) + "\n"
	wContent += "  " + tui.DottedKeyValueFull("  disk", fmt.Sprintf("%dGB/node", cfg.Topology.Workers.Disk), defaultKeyColWidth, kvWidth) + "\n"
	wContent += "\n"
	fmt.Println(tui.BoxedSectionCompact(wContent, "WORKERS", tui.DefaultBoxWidth))

	return nil
}

func runConfigEdit(cmd *cobra.Command, args []string) error {
	loader := config.NewLoader()
	cfg, err := loader.LoadFile(cfgFile)
	if err != nil {
		if os.IsNotExist(err) {
			tui.Info("no configuration found, starting with defaults")
			cfg = config.DefaultConfig()
		} else {
			return utils.WrapError("failed to load configuration", err)
		}
	}

	result, err := runWizard(cfg)
	if err != nil {
		return utils.WrapError("configuration wizard", err)
	}

	if result.Cancelled {
		tui.Info("edit cancelled, no changes made")
		return nil
	}

	if err := writeCredentialsEnv(result.Config, cfgFile); err != nil {
		return utils.WrapError("failed to save credentials", err)
	}

	clearConfigCredentials(result.Config)
	return saveConfig(result.Config, cfgFile)
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	loader := config.NewLoader()
	if _, err := loader.LoadFile(cfgFile); err != nil {
		return utils.WrapError("failed to load config", err)
	}

	loader.Set(key, value)

	cfg, err := loader.Load()
	if err != nil {
		return utils.WrapError("failed to apply changes", err)
	}

	if err := loader.Save(cfg, cfgFile); err != nil {
		return utils.WrapError("failed to save config", err)
	}

	tui.Info(fmt.Sprintf("set %s = %s", key, value))
	tui.Info(fmt.Sprintf("saved to %s", cfgFile))

	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	_, err := LoadConfig(cfgFile)
	if err != nil {
		return err
	}

	loader := config.NewLoader()
	if _, err := loader.LoadFile(cfgFile); err != nil {
		tui.Debug("failed to load config file for get: " + err.Error())
	}
	value := loader.Get(key)
	if value == nil {
		tui.Warn(fmt.Sprintf("key not found: %s", key))
		return nil
	}

	tui.Info(fmt.Sprintf("%s: %v", key, value))

	return nil
}

func runConfigPath(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(cfgFile); os.IsNotExist(err) {
		tui.Warn("configuration file does not exist")
		tui.Info("expected path: " + cfgFile)
		tui.Info("run 'openshitctl init' to create a configuration file")
		return nil
	}

	tui.Info("configuration file: " + cfgFile)

	return nil
}

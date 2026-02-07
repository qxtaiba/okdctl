package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okd-proxmox-cli/internal/addon"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
)

var addonCmd = &cobra.Command{
	Use:   "addon",
	Short: "Manage cluster addons",
	Long:  `List, install, verify, and uninstall cluster addons.`,
}

var addonListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available addons",
	RunE:  runAddonList,
}

var addonVerifyCmd = &cobra.Command{
	Use:   "verify [name]",
	Short: "Verify addon health",
	Long:  `Verify one or all enabled addons are healthy.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runAddonVerify,
}

var addonInstallCmd = &cobra.Command{
	Use:   "install <name>",
	Short: "Install a single addon",
	Args:  cobra.ExactArgs(1),
	RunE:  runAddonInstall,
}

var addonUninstallCmd = &cobra.Command{
	Use:   "uninstall <name>",
	Short: "Uninstall an addon",
	Args:  cobra.ExactArgs(1),
	RunE:  runAddonUninstall,
}

func init() {
	addonCmd.AddCommand(addonListCmd)
	addonCmd.AddCommand(addonVerifyCmd)
	addonCmd.AddCommand(addonInstallCmd)
	addonCmd.AddCommand(addonUninstallCmd)
}

// newAddonManager creates an addon manager using the config file and local executor.
func newAddonManager(cfgPath string) (*addon.Manager, error) {
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		return nil, err
	}
	projectRoot, _ := os.Getwd()
	exec := executor.New()
	return addon.NewManager(cfg, exec, CLILogger(), projectRoot), nil
}

func runAddonList(cmd *cobra.Command, args []string) error {
	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		return err
	}

	allAddons := addon.All()
	sort.Slice(allAddons, func(i, j int) bool {
		return allAddons[i].Info().Priority < allAddons[j].Info().Priority
	})

	enabledMap := make(map[string]bool)
	for _, a := range addon.Enabled(cfg) {
		enabledMap[a.Info().Name] = true
	}

	fmt.Println()
	fmt.Printf("  %-18s %-30s %-14s %s\n", "NAME", "DISPLAY", "CATEGORY", "STATUS")
	fmt.Printf("  %-18s %-30s %-14s %s\n",
		strings.Repeat("─", 18), strings.Repeat("─", 30), strings.Repeat("─", 14), strings.Repeat("─", 8))

	for _, a := range allAddons {
		info := a.Info()
		status := "disabled"
		if enabledMap[info.Name] {
			status = "enabled"
		}
		fmt.Printf("  %-18s %-30s %-14s %s\n", info.Name, info.DisplayName, info.Category, status)
	}
	fmt.Println()
	return nil
}

func runAddonVerify(cmd *cobra.Command, args []string) error {
	mgr, err := newAddonManager(cfgFile)
	if err != nil {
		return err
	}

	if len(args) == 1 {
		name := args[0]
		a := addon.Get(name)
		if a == nil {
			return fmt.Errorf("unknown addon: %s", name)
		}
		cfg, _ := LoadConfig(cfgFile)
		projectRoot, _ := os.Getwd()
		env := &addon.Environment{
			Config:      cfg,
			AddonConfig: cfg.Addons[name],
			Exec:        executor.New(),
			Logger:      CLILogger(),
			Outputs:     mgr.OutputStore(),
			ProjectRoot: projectRoot,
		}
		if err := a.Verify(cmd.Context(), env); err != nil {
			return fmt.Errorf("addon %s verification failed: %w", name, err)
		}
		tui.Info(fmt.Sprintf("addon %s: verified", name))
		return nil
	}

	if err := mgr.VerifyAll(cmd.Context()); err != nil {
		return err
	}
	tui.Info("all enabled addons verified")
	return nil
}

func runAddonInstall(cmd *cobra.Command, args []string) error {
	mgr, err := newAddonManager(cfgFile)
	if err != nil {
		return err
	}

	if err := mgr.InstallOne(cmd.Context(), args[0]); err != nil {
		return err
	}
	tui.Info(fmt.Sprintf("addon %s installed", args[0]))
	return nil
}

func runAddonUninstall(cmd *cobra.Command, args []string) error {
	mgr, err := newAddonManager(cfgFile)
	if err != nil {
		return err
	}

	if err := mgr.Uninstall(cmd.Context(), args[0]); err != nil {
		return err
	}
	tui.Info(fmt.Sprintf("addon %s uninstalled", args[0]))
	return nil
}

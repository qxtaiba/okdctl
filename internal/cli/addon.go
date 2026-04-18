package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/tui"
)

var addonInstallAll bool

var addonCmd = &cobra.Command{
	Use:   "addon",
	Short: "Manage cluster addons",
	Long:  "List, install, uninstall, and verify optional cluster addons.",
}

var addonListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered addons and their config state",
	RunE:  runAddonList,
}

var addonInstallCmd = &cobra.Command{
	Use:         "install [name]",
	Short:       "Install one addon (or all enabled addons with --all)",
	Annotations: map[string]string{"requiresRoot": "true"},
	Long: `Install an addon onto the live cluster.

install <name>  installs the named addon and its transitive dependencies.
                If any addon in the dependency closure fails, all addons
                installed in this invocation are uninstalled in reverse order
                before the error is returned (all-or-nothing rollback).

install --all   installs every addon enabled in the configuration file in
                dependency order. If an individual addon fails it is rolled back
                in isolation; unrelated addons continue installing
                (per-addon continuation).`,
	Args: func(_ *cobra.Command, args []string) error {
		if addonInstallAll {
			if len(args) != 0 {
				return fmt.Errorf("--all and a named addon are mutually exclusive")
			}
			return nil
		}
		if len(args) != 1 {
			return fmt.Errorf("expected exactly one addon name, or use --all")
		}
		return nil
	},
	RunE: runAddonInstall,
}

var addonUninstallCmd = &cobra.Command{
	Use:         "uninstall <name>",
	Short:       "Uninstall a named addon",
	Annotations: map[string]string{"requiresRoot": "true"},
	Long: `Remove an addon from the cluster.

Uninstall is blocked when any other enabled addon transitively depends on the
target. Disable or uninstall the dependent addon first.`,
	Args: cobra.ExactArgs(1),
	RunE: runAddonUninstall,
}

var addonVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify health of all enabled addons",
	RunE:  runAddonVerify,
}

func init() {
	addonInstallCmd.Flags().BoolVar(&addonInstallAll, "all", false, "install all enabled addons (per-addon continuation on failure)")

	addonCmd.AddCommand(addonListCmd)
	addonCmd.AddCommand(addonInstallCmd)
	addonCmd.AddCommand(addonUninstallCmd)
	addonCmd.AddCommand(addonVerifyCmd)
	rootCmd.AddCommand(addonCmd)
}

func runAddonList(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}
	return printAddonList(cmd.OutOrStdout(), cfg)
}

func runAddonInstall(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}
	mgr := newAddonManager(cfg, projectRoot)
	if addonInstallAll {
		return mgr.InstallAll(cmd.Context())
	}
	return mgr.InstallOne(cmd.Context(), args[0])
}

func runAddonUninstall(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}
	mgr := newAddonManager(cfg, projectRoot)
	if err := mgr.Uninstall(cmd.Context(), args[0]); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "addon %s uninstalled\n", args[0])
	return nil
}

func runAddonVerify(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}
	exec := executor.New(executor.WithWorkDir(projectRoot))
	logger := tui.SimpleLogger()

	enabled := addon.Enabled(cfg)
	if len(enabled) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no addons enabled")
		return nil
	}

	type row struct {
		name   string
		status string
	}
	rows := make([]row, 0, len(enabled))
	var errs []error

	for _, a := range enabled {
		if ctxErr := cmd.Context().Err(); ctxErr != nil {
			return ctxErr
		}
		info := a.Info()
		ac := cfg.Addons[info.Name]
		env := &addon.Environment{
			AddonConfig: ac,
			Exec:        exec,
			Logger:      logger,
			ProjectRoot: projectRoot,
		}
		if vErr := a.Verify(cmd.Context(), env); vErr != nil {
			rows = append(rows, row{name: info.Name, status: "FAIL: " + vErr.Error()})
			errs = append(errs, fmt.Errorf("addon %s verify failed: %w", info.Name, vErr))
		} else {
			rows = append(rows, row{name: info.Name, status: "OK"})
		}
	}

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tSTATUS")
	for _, r := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", r.name, r.status)
	}
	_ = tw.Flush()

	return errors.Join(errs...)
}

func newAddonManager(cfg *config.Config, projectRoot string) *addon.Manager {
	exec := executor.New(executor.WithWorkDir(projectRoot))
	return addon.NewManager(cfg, exec, tui.SimpleLogger(), projectRoot)
}

func printAddonList(w io.Writer, cfg *config.Config) error {
	all := addon.All()
	if len(all) == 0 {
		_, err := fmt.Fprintln(w, "no addons registered")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tDISPLAY-NAME\tDEPS\tCONFIG-ENABLED")
	for _, a := range all {
		info := a.Info()
		deps := "-"
		if len(info.Dependencies) > 0 {
			deps = strings.Join(info.Dependencies, ",")
		}
		ac := cfg.Addons[info.Name]
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", info.Name, info.DisplayName, deps, yesNo(ac.Enabled))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(w, "\nCONFIG-ENABLED reflects the configuration file only. Run 'addon verify' for live cluster state.")
	return nil
}

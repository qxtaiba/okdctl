package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/tui"
)

var (
	addonInstallAll              bool
	addonUninstallYes            bool
	addonUninstallConfirmCluster string
	addonListOutput              string
	addonVerifyOutput            string
)

var addonCmd = &cobra.Command{
	Use:     "addon",
	Aliases: []string{"addons"},
	Short:   "Manage cluster addons",
	Long:    "List, install, uninstall, and verify optional cluster addons.",
}

var addonListCmd = &cobra.Command{
	Use:   cmdNameList,
	Short: "List registered addons and their config state",
	Long: `List all registered addons with their display name, dependencies, and
whether they are enabled in the configuration file.

See also: addon verify`,
	Example: "  okdctl addon list",
	RunE:    runAddonList,
}

var addonInstallCmd = &cobra.Command{
	Use:         "install [name]",
	Short:       "Install one addon (or all enabled addons with --all)",
	Example:     "  okdctl addon install flux\n  okdctl addon install --all",
	Annotations: map[string]string{annotationKeyRequiresRoot: annotationValueTrue},
	ValidArgsFunction: func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return addon.Names(), cobra.ShellCompDirectiveNoFileComp
	},
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
				return &errtypes.UsageError{Msg: "--all and a named addon are mutually exclusive"}
			}
			return nil
		}
		if len(args) != 1 {
			return &errtypes.UsageError{Msg: "expected exactly one addon name, or use --all"}
		}
		return nil
	},
	RunE: runAddonInstall,
}

var addonUninstallCmd = &cobra.Command{
	Use:   "uninstall <name>",
	Short: "Uninstall a named addon",
	Example: "  okdctl addon uninstall flux\n" +
		"  okdctl addon uninstall flux --yes --confirm-cluster=prod",
	Annotations: map[string]string{annotationKeyRequiresRoot: annotationValueTrue},
	ValidArgsFunction: func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return addon.Names(), cobra.ShellCompDirectiveNoFileComp
	},
	Long: `Remove an addon from the cluster.

Uninstall is blocked when any other enabled addon transitively depends on the
target. Disable or uninstall the dependent addon first.`,
	Args: cobra.ExactArgs(1),
	RunE: runAddonUninstall,
}

var addonVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify health of all enabled addons",
	Long: `Run each enabled addon's Verify() probe against the live cluster and
report pass/fail for every addon. The output lists each addon name alongside
OK or a FAIL reason. Exit code is non-zero if any probe fails or if the
configuration cannot be loaded.

See also: addon list`,
	Example: "  okdctl addon verify",
	RunE:    runAddonVerify,
}

func init() {
	addonListCmd.Flags().StringVarP(&addonListOutput, flagOutput, flagOutputShort, outputText, "output format: text|json")
	registerOutputCompletion(addonListCmd)
	addonVerifyCmd.Flags().StringVarP(&addonVerifyOutput, flagOutput, flagOutputShort, outputText, "output format: text|json")
	registerOutputCompletion(addonVerifyCmd)
	addonInstallCmd.Flags().BoolVar(&addonInstallAll, "all", false, "install all enabled addons (per-addon continuation on failure)")
	addonUninstallCmd.Flags().BoolVarP(&addonUninstallYes, "yes", "y", false, "skip confirmation prompt")
	addonUninstallCmd.Flags().StringVar(&addonUninstallConfirmCluster, "confirm-cluster", "",
		"required with --yes; must equal cfg.Cluster.Name (typo guard for scripted uninstalls)")

	addonCmd.AddCommand(addonListCmd)
	addonCmd.AddCommand(addonInstallCmd)
	addonCmd.AddCommand(addonUninstallCmd)
	addonCmd.AddCommand(addonVerifyCmd)
	rootCmd.AddCommand(addonCmd)
}

type addonListEntry struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Deps        []string `json:"deps"`
	InConfig    bool     `json:"in_config"`
}

func runAddonList(cmd *cobra.Command, _ []string) error {
	if err := validateFormat(addonListOutput); err != nil {
		return err
	}
	quietForJSON(addonListOutput)

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	if addonListOutput == outputJSON {
		all := addon.All()
		entries := make([]addonListEntry, 0, len(all))
		for _, a := range all {
			info := a.Info()
			deps := make([]string, 0, len(info.Dependencies))
			deps = append(deps, info.Dependencies...)
			entries = append(entries, addonListEntry{
				Name:        info.Name,
				DisplayName: info.DisplayName,
				Deps:        deps,
				InConfig:    cfg.Addons[info.Name].Enabled,
			})
		}
		return writeJSON(cmd.OutOrStdout(), entries)
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
	// Addon installs mutate the live cluster; hold the project runlock like
	// every other mutating verb (deploy, node ops, update-ingress) so
	// concurrent invocations serialize.
	lock, err := runlock.Acquire(projectRoot, "addon install")
	if err != nil {
		return err
	}
	defer lock.Release()
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

	tui.Warn("this will uninstall addon from cluster", tui.LF("addon", args[0]), tui.LF("cluster", cfg.Cluster.Name))

	if err := confirmClusterMatches(addonUninstallYes, addonUninstallConfirmCluster, cfg.Cluster.Name, "uninstall"); err != nil {
		return err
	}
	if !addonUninstallYes {
		ok, err := promptForConfirmation(cmd.Context(), "proceed with uninstall? [y/N]: ")
		if err != nil {
			return err
		}
		if !ok {
			tui.Info("cancelled")
			return nil
		}
	}

	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}
	// Acquired after the interactive confirmation so a prompt left waiting
	// never holds the lock against another invocation.
	lock, err := runlock.Acquire(projectRoot, "addon uninstall")
	if err != nil {
		return err
	}
	defer lock.Release()
	mgr := newAddonManager(cfg, projectRoot)
	if err := mgr.Uninstall(cmd.Context(), args[0]); err != nil {
		return err
	}
	tui.Info("addon uninstalled", tui.LF("addon", args[0]))
	return nil
}

func runAddonVerify(cmd *cobra.Command, _ []string) error {
	if err := validateFormat(addonVerifyOutput); err != nil {
		return err
	}
	quietForJSON(addonVerifyOutput)

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}
	mgr := newAddonManager(cfg, projectRoot)
	results, vErr := mgr.VerifyAll(cmd.Context())

	if addonVerifyOutput == outputJSON {
		entries := make([]okd.AddonStatus, 0, len(results))
		for _, r := range results {
			e := okd.AddonStatus{Name: r.Name, Healthy: r.Err == nil}
			if r.Err != nil {
				e.Error = r.Err.Error()
			}
			entries = append(entries, e)
		}
		if err := writeJSON(cmd.OutOrStdout(), entries); err != nil {
			return err
		}
		return vErr
	}

	if len(results) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no addons enabled")
		return vErr
	}

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tSTATUS")
	failed := 0
	for _, r := range results {
		status := "OK"
		if r.Err != nil {
			status = "FAIL: " + r.Err.Error()
			failed++
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", r.Name, status)
	}
	_ = tw.Flush()

	if failed > 0 {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("%d addon(s) failed verification", failed)}
	}
	return vErr
}

func newAddonManager(cfg *config.Config, projectRoot string) *addon.Manager {
	exec := executor.New(executor.WithWorkDir(projectRoot))
	return addon.NewManager(
		cfg,
		addon.WithExecutor(exec),
		addon.WithLogger(tui.SimpleLogger()),
		addon.WithProjectRoot(projectRoot),
	)
}

func printAddonList(w io.Writer, cfg *config.Config) error {
	all := addon.All()
	if len(all) == 0 {
		_, err := fmt.Fprintln(w, "no addons registered")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tDISPLAY-NAME\tDEPS\tIN-CONFIG")
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
	_, _ = fmt.Fprintln(w, "\nIN-CONFIG reflects the configuration file only. Run 'addon verify' for live cluster state.")
	return nil
}

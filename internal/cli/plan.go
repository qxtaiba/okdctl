package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// errPlanDrift is a drift-only sentinel (not errtypes) mapped to exit code 7;
// see docs/cli/exit-codes.md.
var errPlanDrift = errors.New("plan: drift detected")

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Preview infrastructure drift without applying changes",
	Long: `Run a read-only terraform plan against the current workspace and report
whether the Proxmox infrastructure has drifted from the configuration and
terraform state on disk. okdctl plan never applies changes and never leaves
a usable plan file behind.

Exit code is 0 when the plan is clean, 7 when a create/update/replace/delete
is pending. Run 'okdctl deploy' to reconcile drift.`,
	Example: "  okdctl plan",
	Args:    cobra.NoArgs,
	RunE:    runPlan,
}

func init() {
	rootCmd.AddCommand(planCmd)
}

func runPlan(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	// plan never materializes terraform sources, so a not-yet-deployed
	// workspace must fail here with a pointer at deploy, not a raw terraform
	// error.
	envDir := workspace.TerraformEnvDir(projectRoot, cfg.TerraformEnvName())
	if !system.DirExists(envDir) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf(
			"terraform workspace not found at %s; run 'okdctl deploy' to create it before previewing drift", envDir)}
	}

	announceInFlightNodeOp(projectRoot, cfg)

	logutil.Info("plan: running terraform plan (no changes will be made)")

	changes, err := runTerraformPlanPreview(cmd.Context(), cfg, planPreviewOptions{
		ConfigPath:  cfgFile,
		ProjectRoot: projectRoot,
		Caller:      "plan",
	})
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.OutOrStdout(), render.PlanPreview(changes))

	if len(changes) > 0 {
		logutil.Warn("plan: drift detected", logutil.LF("changes", len(changes)))
		return errPlanDrift
	}
	logutil.Info("plan: no drift detected")
	return nil
}

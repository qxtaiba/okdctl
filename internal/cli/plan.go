package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// errPlanDrift signals a plan run that found pending infrastructure changes.
// It is a bare sentinel local to cli (not an errtypes category) because
// drift is not a failure — exitCodeFor maps it to the dedicated code 7 so
// scripted callers can distinguish "clean" from "drifted" from "failed"
// (see docs/cli/exit-codes.md); execute() also skips its usual "command
// failed" announcement for this sentinel.
var errPlanDrift = errors.New("plan: drift detected")

var planOutput string

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Preview infrastructure drift without applying changes",
	Long: `Run a read-only terraform plan against the current workspace and report
whether the Proxmox infrastructure has drifted from the configuration and
terraform state on disk. okdctl plan never applies changes and never leaves
a usable plan file behind.

Exit code is 0 when the plan is clean, 7 when a create/update/replace/delete
is pending. Run 'okdctl deploy' to reconcile drift.

Pass --output=json for machine-readable output (see docs/cli/json-schema.md).`,
	Example: `  okdctl plan
  okdctl plan --output json | jq '.drift'`,
	RunE: runPlan,
}

func init() {
	planCmd.Flags().StringVarP(&planOutput, flagOutput, flagOutputShort, outputText, "output format: text|json")
	registerOutputCompletion(planCmd)
	rootCmd.AddCommand(planCmd)
}

// planJSONChange is one entry in planJSONOutput.Changes; see
// docs/cli/json-schema.md for the documented, stable shape.
type planJSONChange struct {
	Address string `json:"address"`
	Action  string `json:"action"`
}

// planJSONOutput is the top-level envelope emitted by `okdctl plan --output=json`.
type planJSONOutput struct {
	Drift   bool             `json:"drift"`
	Changes []planJSONChange `json:"changes"`
}

func runPlan(cmd *cobra.Command, _ []string) error {
	if err := validateFormat(planOutput); err != nil {
		return err
	}
	quietForJSON(planOutput)

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	tui.Info("plan: running terraform plan (no changes will be made)")

	changes, err := runTerraformPlanPreview(cmd.Context(), cfg, planPreviewOptions{
		ConfigPath:  cfgFile,
		ProjectRoot: projectRoot,
		Caller:      "plan",
	})
	if err != nil {
		return err
	}

	if planOutput == outputJSON {
		if err := writeJSON(cmd.OutOrStdout(), newPlanJSONOutput(changes)); err != nil {
			return err
		}
	} else {
		fmt.Fprint(cmd.OutOrStdout(), render.PlanPreview(changes))
	}

	if len(changes) > 0 {
		tui.Warn("plan: drift detected", tui.LF("changes", len(changes)))
		return errPlanDrift
	}
	tui.Info("plan: no drift detected")
	return nil
}

func newPlanJSONOutput(changes []terraform.ResourceChange) planJSONOutput {
	out := planJSONOutput{Drift: len(changes) > 0, Changes: make([]planJSONChange, len(changes))}
	for i, c := range changes {
		out.Changes[i] = planJSONChange{Address: c.Address, Action: string(c.Action)}
	}
	return out
}

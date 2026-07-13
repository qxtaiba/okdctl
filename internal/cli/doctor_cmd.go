package cli

import "github.com/spf13/cobra"

// doctorCmd is the user-facing 'okdctl doctor' command. It is separate
// from main.preflight() (which is a startup guardrail) — doctor runs the
// host preflight checks and reports each result.
// runDoctor refuses non-linux hosts at runtime; see doctor.go.
var doctorOutput string

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check that your environment is ready to deploy a cluster",
	Long: `Run preflight checks on the local environment before a deploy.

Each check prints a title line with a status icon and a result line
with a bracketed label:

  ✓ [ok]   : the check passed, no action needed
  ⚠ [warn] : something is suboptimal or missing but can be handled
             during deploy (e.g., 'oc' will be auto-downloaded into
             /usr/local/bin)
  ✗ [fail] : this must be fixed before 'okdctl deploy' will
             succeed

Exit code is 0 when every check passes, 6 when one or more checks warn
but none fail, and 2 (configuration error) when any check fails.
Designed to be rerun until clean.

Pass --output=json for machine-readable output (see docs/cli/json-schema.md).

See docs/doctor-checks.md for per-check fail messages and fix guidance.`,
	Example: `  okdctl doctor
  okdctl doctor --output json | jq '.failed'`,
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().StringVarP(&doctorOutput, flagOutput, flagOutputShort, outputText, "output format: text|json")
	registerOutputCompletion(doctorCmd)
	rootCmd.AddCommand(doctorCmd)
}

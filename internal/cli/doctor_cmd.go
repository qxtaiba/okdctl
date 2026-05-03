package cli

import "github.com/spf13/cobra"

// doctorCmd is the user-facing 'okdctl doctor' command. It is separate
// from main.preflight() (which is a startup guardrail) — doctor runs a
// comprehensive environment audit and reports status for each check.
// The command metadata lives here so the cobra tree is platform-consistent
// for offline tooling (notably cmd/okdctl-gen-docs); the RunE body is
// wired per-platform in doctor.go (Linux) and doctor_stub.go (non-Linux).
var doctorCmd = &cobra.Command{
	Use:   categoryDoctor,
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

Exit code is 0 if there are no [fail] results ([warn] is tolerated),
1 otherwise. Designed to be rerun until clean.

See docs/doctor-checks.md for per-check fail messages and fix guidance.`,
	Example: "  okdctl " + categoryDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

package cli

import "github.com/spf13/cobra"

// Flag names referenced both at registration (BoolVar/StringVarP) and at
// read-back (GetBool/GetString). A typo on either side silently returns
// the zero value at runtime — Go has no compile-time check across the
// flag-set string key — so both sites reference these constants.
const (
	flagConfig      = "config"
	flagConfigShort = "c"
	flagDryRun      = "dry-run"
	flagLogFile     = "log-file"
	flagLogFormat   = "log-format"
	flagLogLevel    = "log-level"
	flagOnly        = "only"
	flagOutput      = "output"
	flagOutputFile  = "output-file"
	flagOutputShort = "o"
	flagQuiet       = "quiet"
	flagTarget      = "target"
	flagVerbose     = "verbose"
)

// Output-format values for the --output/-o flag. Mirrors kubectl/oc:
// "text" is the human-readable default; "json" is the machine-readable
// schema documented in docs/cli/json-schema.md.
const (
	outputText = "text"
	outputJSON = "json"
)

// Subcommand names referenced both at cobra registration (Use:) and in
// policy tables (rootRequiredCmds, defaultLogSinkCmds); a typo between the
// two sites would silently drop a command from the policy.
const (
	cmdNameDeploy  = "deploy"
	cmdNameDestroy = "destroy"
	cmdNameCleanup = "cleanup"
)

// annotationKeyRequiresRoot tags cobra commands whose body must run as
// root (writes to /etc, /usr/local/bin, /var/www/html, systemd, firewalls).
// The PersistentPreRunE in elevation.go re-execs under sudo when the
// caller's euid is non-zero and this annotation (or rootRequiredCmds
// ancestry) is set.
const annotationKeyRequiresRoot = "requiresRoot"

// registerOutputCompletion wires shell completion for the --output/-o flag
// on cmd to the text|json enum. Call immediately after StringVarP binds
// flagOutput on cmd's own FlagSet.
func registerOutputCompletion(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc(flagOutput,
		cobra.FixedCompletions([]string{outputText, outputJSON}, cobra.ShellCompDirectiveNoFileComp))
}

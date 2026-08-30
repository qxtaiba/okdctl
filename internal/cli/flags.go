package cli

import "github.com/spf13/cobra"

// Kept as consts: a typo between flag registration and read-back would silently
// return the zero value.
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

// Output-format values for --output/-o; mirrors kubectl/oc convention (see
// docs/cli/json-schema.md).
const (
	outputText = "text"
	outputJSON = "json"
)

// Kept as consts: a typo between cobra registration and the policy tables would
// silently drop a command.
const (
	cmdNameDeploy  = "deploy"
	cmdNameDestroy = "destroy"
	cmdNameCleanup = "cleanup"
	cmdNameManage  = "manage"
	cmdNameList    = "list"
)

// annotationKeyRequiresRoot tags commands that must run as root; elevation.go's
// PersistentPreRunE re-execs under sudo when it's set.
const annotationKeyRequiresRoot = "requiresRoot"

// registerOutputCompletion wires --output/-o completion to text|json; call
// immediately after StringVarP binds flagOutput on cmd's FlagSet.
func registerOutputCompletion(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc(flagOutput,
		cobra.FixedCompletions([]string{outputText, outputJSON}, cobra.ShellCompDirectiveNoFileComp))
}

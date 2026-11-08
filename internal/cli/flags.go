package cli

import (
	"github.com/spf13/cobra"
)

// Kept as consts: a typo between flag registration and read-back would silently
// return the zero value.
const (
	flagConfig      = "config"
	flagConfigShort = "c"
	flagLogFile     = "log-file"
	flagLogFormat   = "log-format"
	flagLogLevel    = "log-level"
	flagNoColor     = "no-color"
	flagNoMotion    = "no-motion"
	flagOutput      = "output"
	flagOutputFile  = "output-file"
	flagOutputShort = "o"
	flagQuiet       = "quiet"
	flagVerbose     = "verbose"
)

// Output-format values for --output/-o; mirrors kubectl/oc convention (see
// docs/cli/json-schema.md). Deliberately separate from tui.FormatText/FormatJSON:
// --output selects a data encoding, --log-format selects a log encoding.
const (
	outputText = "text"
	outputJSON = "json"
)

// Kept as consts: a typo between cobra registration and the policy tables would
// silently drop a command.
const (
	cmdNameDeploy = "deploy"
	cmdNameList   = "list"
)

// registerOutputCompletion wires --output/-o completion to text|json; call
// immediately after StringVarP binds flagOutput on cmd's FlagSet.
func registerOutputCompletion(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc(flagOutput,
		cobra.FixedCompletions([]string{outputText, outputJSON}, cobra.ShellCompDirectiveNoFileComp))
}

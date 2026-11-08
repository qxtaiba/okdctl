package cli

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

// Kept as consts: a typo between cobra registration and the policy tables would
// silently drop a command.
const (
	cmdNameInit = "init"
	cmdNameList = "list"
)

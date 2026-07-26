package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// logFileCloser holds the open log-file handle so Execute can close it.
// nil when no file sink is active (--log-file unset and the command has
// no default sink).
var logFileCloser io.Closer

// runLogPath is the path of the active file sink (--log-file or the
// default workspace okdctl.log); "" when no file sink is active. execute()
// prints it on failure so the operator knows a persistent log exists.
var runLogPath string

// runLogSink is the active file sink as a plain writer; nil when no file sink
// is active. deploy routes the openshift-install firehose here so the TTY
// carries only the curated status line. It aliases the same handle as
// logFileCloser — do not close it separately.
var runLogSink io.Writer

// defaultLogSinkCmds lists the commands that tee their log stream to
// <workspace>/okdctl.log by default. Scoped to the commands that mutate a
// workspace; read-only commands (status, version, releases) must not start
// writing files. Matching walks the cobra parent chain, mirroring
// rootRequiredCmds.
var defaultLogSinkCmds = []string{cmdNameDeploy, cmdNameDestroy, cmdNameCleanup, cmdNameManage}

func wantsDefaultLogSink(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if slices.Contains(defaultLogSinkCmds, c.Name()) {
			return true
		}
	}
	return false
}

// openLogFile refuses a symlink path via lstat, then opens with
// O_NOFOLLOW so a symlink planted between lstat and open still loses
// the race. Needed because configureLogging runs twice on root-required
// commands (invoking user + sudo re-exec) and a pre-sudo attacker could
// otherwise redirect root-authored log lines via a planted symlink.
// Privilege contract: the path is either operator-supplied (--log-file)
// or derived from the operator's working directory (the default
// okdctl.log sink), and the file is opened as root post-sudo-re-exec.
// The operator is trusted; no path-location restriction is enforced.
// O_APPEND + 0o600 bound the risk: existing file content cannot be
// overwritten, and callers restore invoking-user ownership where needed.
func openLogFile(path string) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("log file path %q is a symlink; refusing to follow", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat log file: %w", err)
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW, 0o600)
}

// openDefaultLogSink opens <workspace>/okdctl.log for append and chowns it
// to the invoking user. Under the sudo re-exec model the pre-sudo pass
// creates the file as the invoking user and the root pass only appends;
// the chown covers direct `sudo okdctl deploy` invocations where root
// creates it. A chown failure closes the sink — a root-owned 0600 log the
// operator cannot read afterwards is worse than no file sink.
func openDefaultLogSink() (string, *os.File, error) {
	root, err := resolveWorkspaceRoot()
	if err != nil {
		return "", nil, err
	}
	path := filepath.Join(root, logutil.DefaultLogFileName)
	f, err := openLogFile(path)
	if err != nil {
		return "", nil, err
	}
	if err := system.ChownToInvokingUser(path); err != nil {
		_ = f.Close()
		return "", nil, fmt.Errorf("chown log file to invoking user: %w", err)
	}
	return path, f, nil
}

func configureLogging(cmd *cobra.Command) error {
	stdoutW := io.Writer(os.Stdout)
	stderrW := io.Writer(os.Stderr)

	var sink *os.File
	var sinkErr error
	switch {
	case logFile != "":
		sink, sinkErr = openLogFile(logFile)
		if sinkErr != nil {
			return fmt.Errorf("open log file: %w", sinkErr)
		}
		runLogPath = logFile
	case wantsDefaultLogSink(cmd):
		// Best-effort: a read-only or otherwise unwritable cwd must not
		// block the run, unlike an explicit --log-file which hard-fails
		// above. The warning is emitted after ConfigureLoggers below so
		// it uses the fully-configured formatter.
		runLogPath, sink, sinkErr = openDefaultLogSink()
	}
	if sink != nil {
		logFileCloser = sink
		runLogSink = sink
		stdoutW = io.MultiWriter(os.Stdout, sink)
		stderrW = io.MultiWriter(os.Stderr, sink)
	}

	// --quiet and --verbose are sugar over --log-level; mutual exclusion is
	// enforced at flag registration so at most one is set here.
	effectiveLevel := logLevel
	switch {
	case logQuiet:
		effectiveLevel = "error"
	case logVerbose:
		effectiveLevel = "debug"
	}

	stderrIsTTY := term.IsTerminal(int(os.Stderr.Fd()))
	stdoutIsTTY := term.IsTerminal(int(os.Stdout.Fd()))
	// Honor https://no-color.org and FORCE_COLOR; either disables progress
	// bars regardless of TTY detection.
	noColor := os.Getenv("NO_COLOR") != ""

	// Auto-switch to json when stderr is piped and the user has not
	// explicitly set --log-format, mirroring the progress-bar TTY gate.
	if !cmd.Root().PersistentFlags().Changed(flagLogFormat) && !stderrIsTTY {
		logFormat = outputJSON
	}

	progressBars := stderrIsTTY && stdoutIsTTY && logFormat != outputJSON && !noColor

	// Pin the box/leader render profile to stdout's real capabilities so a
	// piped or NO_COLOR run strips escapes from boxes the same way charm/log
	// already strips them from level badges.
	tui.SetColorProfileFor(os.Stdout)

	if err := tui.ConfigureLoggers(effectiveLevel, logFormat, stdoutW, stderrW, progressBars); err != nil {
		return err
	}
	if sinkErr != nil {
		logutil.Warn("default log file unavailable; continuing without persistent log", logutil.LF("err", sinkErr))
	}
	// Suppress Info/Warn chatter under json so status-style `2>&1 | jq`
	// pipelines see a clean stream — but never for the deploy-family flows,
	// whose non-TTY contract keeps milestones at Info and degraded-operator
	// notices at Warn (json-formatted). Their firehose already goes to the log
	// file, so what reaches stderr is the curated milestone/Warn set, not chatter.
	if logFormat == outputJSON && !logVerbose && !wantsDefaultLogSink(cmd) {
		tui.SuppressInfo()
	}
	return nil
}

// quietForJSON raises the stderr log level to error when --output=json is
// active and the user has not requested verbose output. Without this,
// 2>&1 | jq pipelines see Info chatter mixed into the JSON stream.
func quietForJSON(format string) {
	if format == outputJSON && !logVerbose {
		tui.SuppressInfo()
	}
}

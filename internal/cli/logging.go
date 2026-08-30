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

// logFileCloser holds the open log-file handle for Execute to close; nil when
// no file sink is active.
var logFileCloser io.Closer

// runLogPath is the active file sink's path ("" if none); execute() prints it on failure.
var runLogPath string

// runLogSink aliases logFileCloser as a writer (do not close separately);
// deploy routes the install-log firehose here so the TTY shows only the curated
// status line.
var runLogSink io.Writer

// defaultLogSinkCmds lists commands that tee logs to <workspace>/okdctl.log by
// default; matching walks the parent chain.
var defaultLogSinkCmds = []string{cmdNameDeploy, cmdNameDestroy, cmdNameCleanup, cmdNameManage}

func wantsDefaultLogSink(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if slices.Contains(defaultLogSinkCmds, c.Name()) {
			return true
		}
	}
	return false
}

// openLogFile lstats to refuse a symlink then opens O_NOFOLLOW, closing a
// pre-sudo TOCTOU race on root-authored log lines.
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

// openDefaultLogSink opens <workspace>/okdctl.log and chowns it via the open fd
// (TOCTOU-safe), closing the sink on chown failure instead of leaving a
// root-owned log.
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
	if err := system.ChownFileToInvokingUser(f); err != nil {
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
		// best-effort: an unwritable cwd must not block the run (unlike
		// --log-file); warning emitted after ConfigureLoggers so it's formatted
		runLogPath, sink, sinkErr = openDefaultLogSink()
	}
	if sink != nil {
		logFileCloser = sink
		runLogSink = sink
		stdoutW = io.MultiWriter(os.Stdout, sink)
		stderrW = io.MultiWriter(os.Stderr, sink)
	}

	// --quiet/--verbose are sugar over --log-level; mutual exclusion is enforced at flag registration.
	effectiveLevel := logLevel
	switch {
	case logQuiet:
		effectiveLevel = "error"
	case logVerbose:
		effectiveLevel = "debug"
	}

	stderrIsTTY := term.IsTerminal(int(os.Stderr.Fd()))
	stdoutIsTTY := term.IsTerminal(int(os.Stdout.Fd()))
	// NO_COLOR (no-color.org) disables progress bars regardless of TTY;
	// FORCE_COLOR only affects colorprofile styling, not this gate.
	noColor := os.Getenv("NO_COLOR") != ""

	// auto-switch to json when stderr is piped and --log-format wasn't set
	// explicitly, mirroring the progress-bar TTY gate
	if !cmd.Root().PersistentFlags().Changed(flagLogFormat) && !stderrIsTTY {
		logFormat = outputJSON
	}

	progressBars := stderrIsTTY && stdoutIsTTY && logFormat != outputJSON && !noColor

	// pin the render profile to stdout's real capabilities so a piped/NO_COLOR
	// run strips box escapes like charm/log strips level badges
	tui.SetColorProfileFor(os.Stdout)

	if err := tui.ConfigureLoggers(effectiveLevel, logFormat, stdoutW, stderrW, progressBars); err != nil {
		return err
	}
	if sinkErr != nil {
		logutil.Warn("default log file unavailable; continuing without persistent log", logutil.LF("err", sinkErr))
	}
	// suppress Info/Warn under json for clean pipelines, except deploy-family
	// flows which keep milestones/degraded-notices visible
	if logFormat == outputJSON && !logVerbose && !wantsDefaultLogSink(cmd) {
		tui.SuppressInfo()
	}
	return nil
}

// quietForJSON raises the stderr log level to error when --output=json is
// active and verbose logging was not requested.
func quietForJSON(format string) {
	if format == outputJSON && !logVerbose {
		tui.SuppressInfo()
	}
}

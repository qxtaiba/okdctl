package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// logFileCloser holds the open log-file handle so Execute can close it.
// nil when --log-file is not set.
var logFileCloser io.Closer

// openLogFile refuses a symlink path via lstat, then opens with
// O_NOFOLLOW so a symlink planted between lstat and open still loses
// the race. Needed because configureLogging runs twice on root-required
// commands (invoking user + sudo re-exec) and a pre-sudo attacker could
// otherwise redirect root-authored log lines via a planted symlink.
// Privilege contract: --log-file is operator-supplied and the file is
// opened as root post-sudo-re-exec. The operator is trusted; no
// path-location restriction is enforced. O_APPEND + 0o600 bound the
// risk: existing file content cannot be overwritten, and the resulting
// file is readable only by root.
func openLogFile(path string) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("--log-file path %q is a symlink; refusing to follow", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat log file: %w", err)
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW, 0o600)
}

func configureLogging(cmd *cobra.Command) error {
	stdoutW := io.Writer(os.Stdout)
	stderrW := io.Writer(os.Stderr)

	if logFile != "" {
		f, err := openLogFile(logFile)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		logFileCloser = f
		stdoutW = io.MultiWriter(os.Stdout, f)
		stderrW = io.MultiWriter(os.Stderr, f)
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

	if err := tui.ConfigureLoggers(effectiveLevel, logFormat, stdoutW, stderrW, progressBars); err != nil {
		return err
	}
	if logFormat == outputJSON && !logVerbose {
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

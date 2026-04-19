package cli

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// logFileCloser holds the open log-file handle so Execute can close it.
// nil when --log-file is not set.
var logFileCloser io.Closer

func configureLogging() error {
	stdoutW := io.Writer(os.Stdout)
	stderrW := io.Writer(os.Stderr)

	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
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

	stderrIsTTY := term.IsTerminal(int(os.Stderr.Fd())) //nolint:gosec // G115: Fd() always fits int on supported platforms
	progressBars := stderrIsTTY && logFormat != "json"

	return tui.ConfigureLoggers(effectiveLevel, logFormat, stdoutW, stderrW, progressBars)
}

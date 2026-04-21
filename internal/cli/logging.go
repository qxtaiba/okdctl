package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"golang.org/x/term"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// logFileCloser holds the open log-file handle so Execute can close it.
// nil when --log-file is not set.
var logFileCloser io.Closer

// openLogFile refuses to open a path that already resolves through a
// symlink, then opens with O_NOFOLLOW so a symlink planted between the
// lstat and the open still loses the race. Matters because configureLogging
// runs twice on root-required commands — once as the invoking user and
// once post-sudo-re-exec — and a pre-sudo attacker with write access to
// the invoking user's PWD could otherwise redirect root-authored log
// lines onto an arbitrary file.
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

func configureLogging() error {
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

	stderrIsTTY := term.IsTerminal(int(os.Stderr.Fd())) //nolint:gosec // G115: Fd() always fits int on supported platforms
	stdoutIsTTY := term.IsTerminal(int(os.Stdout.Fd())) //nolint:gosec // G115: Fd() always fits int on supported platforms
	// Honor https://no-color.org and FORCE_COLOR; either disables progress
	// bars regardless of TTY detection.
	noColor := os.Getenv("NO_COLOR") != ""
	progressBars := stderrIsTTY && stdoutIsTTY && logFormat != "json" && !noColor

	return tui.ConfigureLoggers(effectiveLevel, logFormat, stdoutW, stderrW, progressBars)
}

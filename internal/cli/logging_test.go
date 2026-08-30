package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/tui"
)

func TestOpenLogFileRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.log")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	link := filepath.Join(dir, "link.log")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	f, err := openLogFile(link)
	if err == nil {
		_ = f.Close()
		t.Fatal("openLogFile accepted a symlink; symlink guard regressed")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error does not name the rejection reason: %v", err)
	}
}

func TestOpenLogFileAcceptsRegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.log")

	f, err := openLogFile(path)
	if err != nil {
		t.Fatalf("openLogFile on fresh path: %v", err)
	}
	_ = f.Close()

	f, err = openLogFile(path)
	if err != nil {
		t.Fatalf("openLogFile on existing regular file: %v", err)
	}
	_ = f.Close()
}

// resetLoggingState snapshots and restores logging globals so file-sink tests
// don't bleed into each other.
func resetLoggingState(t *testing.T) {
	t.Helper()
	prevLogFile, prevFormat, prevLevel := logFile, logFormat, logLevel
	t.Cleanup(func() {
		if logFileCloser != nil {
			_ = logFileCloser.Close()
		}
		logFileCloser = nil
		runLogPath = ""
		logFile, logFormat, logLevel = prevLogFile, prevFormat, prevLevel
		if err := tui.ConfigureLoggers("info", "text", os.Stdout, os.Stderr, false); err != nil {
			t.Errorf("restore loggers: %v", err)
		}
	})
	logFileCloser = nil
	runLogPath = ""
}

func TestWantsDefaultLogSink(t *testing.T) {
	cases := []struct {
		cmd  *cobra.Command
		want bool
	}{
		{deployCmd, true},
		{destroyCmd, true},
		{cleanupCmd, true},
		{versionCmd, false},
		{statusCmd, false},
		{debugBundleCmd, false},
		{rootCmd, false},
	}
	for _, tc := range cases {
		if got := wantsDefaultLogSink(tc.cmd); got != tc.want {
			t.Errorf("wantsDefaultLogSink(%s) = %v, want %v", tc.cmd.Name(), got, tc.want)
		}
	}
}

func TestConfigureLogging_DefaultSinkRedacts(t *testing.T) {
	resetLoggingState(t)
	dir := t.TempDir()
	t.Chdir(dir)

	if err := configureLogging(deployCmd); err != nil {
		t.Fatalf("configureLogging: %v", err)
	}
	if runLogPath == "" || filepath.Base(runLogPath) != logutil.DefaultLogFileName {
		t.Fatalf("runLogPath = %q, want <workspace>/%s", runLogPath, logutil.DefaultLogFileName)
	}
	if logFileCloser == nil {
		t.Fatal("logFileCloser not set for default sink")
	}

	// Error always emits regardless of level, independent of the deploy-family Info/Warn policy.
	logutil.Error("connect failed", logutil.LF("password", "hunter2"), logutil.LF("cluster", "prod"))

	data, err := os.ReadFile(runLogPath)
	if err != nil {
		t.Fatalf("read default log: %v", err)
	}
	out := string(data)
	if strings.Contains(out, "hunter2") {
		t.Errorf("secret value leaked into default log file:\n%s", out)
	}
	if !strings.Contains(out, "[redacted]") {
		t.Errorf("redaction marker missing from default log file:\n%s", out)
	}
	if !strings.Contains(out, "prod") {
		t.Errorf("non-secret attr dropped from default log file:\n%s", out)
	}
}

// stderr is piped under go test, so configureLogging auto-selects json here.
func TestConfigureLogging_DeployMilestonesSurviveNonTTY(t *testing.T) {
	resetLoggingState(t)
	dir := t.TempDir()
	t.Chdir(dir)

	if err := configureLogging(deployCmd); err != nil {
		t.Fatalf("configureLogging: %v", err)
	}

	logutil.Info("bootstrap complete")
	logutil.Warn("cluster operator degraded", logutil.LF("operator", "ingress"))

	data, err := os.ReadFile(runLogPath)
	if err != nil {
		t.Fatalf("read default log: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "bootstrap complete") {
		t.Errorf("milestone Info silenced on non-TTY deploy:\n%s", out)
	}
	if !strings.Contains(out, "cluster operator degraded") {
		t.Errorf("degraded-operator Warn silenced on non-TTY deploy:\n%s", out)
	}
}

func TestConfigureLogging_LogFilePrecedence(t *testing.T) {
	resetLoggingState(t)
	dir := t.TempDir()
	t.Chdir(dir)
	logFile = filepath.Join(dir, "custom.log")

	if err := configureLogging(deployCmd); err != nil {
		t.Fatalf("configureLogging: %v", err)
	}
	if runLogPath != logFile {
		t.Fatalf("runLogPath = %q, want %q", runLogPath, logFile)
	}
	if _, err := os.Stat(logFile); err != nil {
		t.Fatalf("--log-file target not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, logutil.DefaultLogFileName)); !os.IsNotExist(err) {
		t.Fatalf("default sink created despite --log-file: stat err = %v", err)
	}
}

func TestConfigureLogging_NoDefaultSinkForReadOnlyCmds(t *testing.T) {
	resetLoggingState(t)
	dir := t.TempDir()
	t.Chdir(dir)

	if err := configureLogging(versionCmd); err != nil {
		t.Fatalf("configureLogging: %v", err)
	}
	if runLogPath != "" || logFileCloser != nil {
		t.Fatalf("file sink active for version: runLogPath=%q closer=%v", runLogPath, logFileCloser)
	}
	if _, err := os.Stat(filepath.Join(dir, logutil.DefaultLogFileName)); !os.IsNotExist(err) {
		t.Fatalf("okdctl.log created for a read-only command: stat err = %v", err)
	}
}

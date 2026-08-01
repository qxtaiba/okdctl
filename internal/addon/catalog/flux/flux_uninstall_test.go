package flux

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

// installFakeTools writes fake helm and oc binaries to a TempDir and prepends
// the dir to PATH. Each binary appends its argv as a colon-separated line to
// the file at $ARGV_LOG when set, then exits with $EXIT_CODE (default 0).
func installFakeTools(t *testing.T) {
	t.Helper()
	script := "#!/bin/sh\n[ -n \"$ARGV_LOG\" ] && printf '%s\\n' \"$(basename \"$0\"):$*\" >> \"$ARGV_LOG\"\nexit \"${EXIT_CODE:-0}\"\n"
	for _, name := range []string{"helm", "oc"} {
		testutil.InstallFakeBin(t, name, script)
	}
}

func makeEnv(t *testing.T, argvLog, exitCode string, log *slog.Logger) *addon.Environment {
	t.Helper()
	extraEnv := []string{"ARGV_LOG=" + argvLog}
	if exitCode != "" {
		extraEnv = append(extraEnv, "EXIT_CODE="+exitCode)
	}
	return &addon.Environment{
		AddonConfig: config.AddonConfig{},
		Exec:        executor.New(executor.WithEnv(extraEnv)),
		Logger:      log,
	}
}

func readArgvLog(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("argv log not written: %v", err)
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func TestUninstall_HappyPath(t *testing.T) {
	installFakeTools(t)
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	h := &testutil.CaptureHandler{}
	env := makeEnv(t, argvLog, "", slog.New(h))

	f := &fluxAddon{}
	if err := f.Uninstall(context.Background(), env); err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}

	lines := readArgvLog(t, argvLog)
	if len(lines) != 3 {
		t.Fatalf("expected 3 argv records, got %d: %v", len(lines), lines)
	}

	want := []string{
		"helm:uninstall flux-instance --namespace flux-system",
		"helm:uninstall flux-operator --namespace flux-system",
		"oc:delete ns flux-system",
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line[%d] = %q, want %q", i, lines[i], w)
		}
	}

	if got := h.CountLevel(slog.LevelWarn); got != 0 {
		t.Errorf("warnCount = %d; want 0 on success path", got)
	}
}

func TestUninstall_FailuresDoNotAbort(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH override semantics differ on windows")
	}
	// Empty PATH so helm/oc cannot be resolved — each Run returns a real
	// exec lookup error. warnOnErr also fires on non-zero exit codes (Run
	// folds those into Result with a nil error); the exit-code branch is
	// covered separately below. This exercises the warnOnErr branch 3
	// times and proves the sequence does not abort early.
	t.Setenv("PATH", t.TempDir())
	h := &testutil.CaptureHandler{}
	env := &addon.Environment{
		AddonConfig: config.AddonConfig{},
		Exec:        executor.New(),
		Logger:      slog.New(h),
	}

	f := &fluxAddon{}
	if err := f.Uninstall(context.Background(), env); err != nil {
		t.Fatalf("Uninstall must return nil even when all commands fail; got: %v", err)
	}

	if got := h.CountLevel(slog.LevelWarn); got != 3 {
		t.Errorf("warnCount = %d; want 3 (one per failing command)", got)
	}
}

// TestUninstall_NonZeroExitWarns locks the exit-code half of warnOnErr:
// Run returns a nil error for a non-zero helm/oc exit, so Uninstall must
// inspect Result.ExitCode to notice the failure at all.
func TestUninstall_NonZeroExitWarns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH override semantics differ on windows")
	}
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	installFakeTools(t)
	h := &testutil.CaptureHandler{}
	env := makeEnv(t, argvLog, "1", slog.New(h))

	f := &fluxAddon{}
	if err := f.Uninstall(context.Background(), env); err != nil {
		t.Fatalf("Uninstall must return nil on non-zero tool exits; got %v", err)
	}
	if got := h.CountLevel(slog.LevelWarn); got != 3 {
		t.Errorf("warnCount = %d; want 3 (every delete exits non-zero)", got)
	}
}

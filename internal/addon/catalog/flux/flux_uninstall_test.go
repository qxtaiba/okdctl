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
)

// captureHandler records every slog.Record so tests can assert Warn count.
type captureHandler struct {
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *captureHandler) warnCount() int {
	n := 0
	for _, r := range h.records {
		if r.Level == slog.LevelWarn {
			n++
		}
	}
	return n
}

// installFakeTools writes fake helm and oc binaries to a TempDir, prepends
// the dir to PATH, and returns the dir. Each binary appends its argv as a
// colon-separated line to the file at $ARGV_LOG when set, then exits with
// $EXIT_CODE (default 0).
func installFakeTools(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake binaries rely on POSIX sh")
	}
	dir := t.TempDir()
	script := []byte("#!/bin/sh\n[ -n \"$ARGV_LOG\" ] && printf '%s\\n' \"$(basename \"$0\"):$*\" >> \"$ARGV_LOG\"\nexit \"${EXIT_CODE:-0}\"\n")
	for _, name := range []string{"helm", "oc"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, script, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func makeEnv(t *testing.T, argvLog string, exitCode string, log *slog.Logger) *addon.Environment {
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
	h := &captureHandler{}
	env := makeEnv(t, argvLog, "", slog.New(h))

	f := &Flux{}
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

	if got := h.warnCount(); got != 0 {
		t.Errorf("warnCount = %d; want 0 on success path", got)
	}
}

func TestUninstall_FailuresDoNotAbort(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH override semantics differ on windows")
	}
	// Empty PATH so helm/oc cannot be resolved — each Run returns a real
	// exec lookup error, which is the only failure mode warnOnErr observes
	// (Uninstall uses Exec.Run, which folds non-zero exit into Result and
	// returns nil error). This exercises the warnOnErr branch 3 times and
	// proves the sequence does not abort early.
	t.Setenv("PATH", t.TempDir())
	h := &captureHandler{}
	env := &addon.Environment{
		AddonConfig: config.AddonConfig{},
		Exec:        executor.New(),
		Logger:      slog.New(h),
	}

	f := &Flux{}
	if err := f.Uninstall(context.Background(), env); err != nil {
		t.Fatalf("Uninstall must return nil even when all commands fail; got: %v", err)
	}

	if got := h.warnCount(); got != 3 {
		t.Errorf("warnCount = %d; want 3 (one per failing command)", got)
	}
}

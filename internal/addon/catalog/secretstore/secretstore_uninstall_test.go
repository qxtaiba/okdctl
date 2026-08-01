package secretstore

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
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler interface requires value receiver
	h.records = append(h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *captureHandler) warnCount() int {
	n := 0
	for i := range h.records {
		if h.records[i].Level == slog.LevelWarn {
			n++
		}
	}
	return n
}

// installFakeOC writes a fake oc binary to a TempDir and prepends the dir to
// PATH. Each invocation appends "oc:<argv>" to $ARGV_LOG when set, then
// exits 1 if $FAIL_ARG is a substring of the argv, 0 otherwise.
func installFakeOC(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake binaries rely on POSIX sh")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"[ -n \"$ARGV_LOG\" ] && printf '%s\\n' \"$(basename \"$0\"):$*\" >> \"$ARGV_LOG\"\n" +
		"if [ -n \"$FAIL_ARG\" ]; then\n" +
		"  case \"$*\" in\n" +
		"    *\"$FAIL_ARG\"*) exit 1 ;;\n" +
		"  esac\n" +
		"fi\n" +
		"exit 0\n"
	p := filepath.Join(dir, "oc")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func makeUninstallEnv(argvLog, failArg string, log *slog.Logger) *addon.Environment {
	extraEnv := []string{"ARGV_LOG=" + argvLog}
	if failArg != "" {
		extraEnv = append(extraEnv, "FAIL_ARG="+failArg)
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

// TestUninstall_HappyPath asserts the delete argv targets exactly the
// addon-owned secret and SecretStore names for the default (onepassword)
// provider — no wildcard deletes.
func TestUninstall_HappyPath(t *testing.T) {
	installFakeOC(t)
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	h := &captureHandler{}
	env := makeUninstallEnv(argvLog, "", slog.New(h))

	s := &secretStore{}
	if err := s.Uninstall(context.Background(), env); err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}

	lines := readArgvLog(t, argvLog)
	want := []string{
		"oc:delete secret onepassword-connect-credentials -n external-secrets",
		"oc:delete secret onepassword-connect-token -n external-secrets",
		"oc:delete secretstore okdctl-secretstore -n external-secrets",
	}
	if len(lines) != len(want) {
		t.Fatalf("expected %d argv records, got %d: %v", len(want), len(lines), lines)
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

// TestUninstall_PartialSecretFailureContinues asserts that a single failed
// secret delete is warned, not fatal — the loop continues to the remaining
// secret and the SecretStore CRD delete still runs.
func TestUninstall_PartialSecretFailureContinues(t *testing.T) {
	installFakeOC(t)
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	h := &captureHandler{}
	env := makeUninstallEnv(argvLog, opCredentialsSecretName, slog.New(h))

	s := &secretStore{}
	if err := s.Uninstall(context.Background(), env); err != nil {
		t.Fatalf("Uninstall must return nil even when a secret delete fails; got: %v", err)
	}

	lines := readArgvLog(t, argvLog)
	if len(lines) != 3 {
		t.Fatalf("expected 3 argv records (loop must continue past the failed delete), got %d: %v", len(lines), lines)
	}

	if got := h.warnCount(); got != 1 {
		t.Errorf("warnCount = %d; want 1 (one failing secret delete)", got)
	}
}

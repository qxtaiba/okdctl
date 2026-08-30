package secretstore

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func installFakeOC(t *testing.T) {
	t.Helper()
	testutil.InstallFakeBin(t, "oc", "#!/bin/sh\n"+
		"[ -n \"$ARGV_LOG\" ] && printf '%s\\n' \"$(basename \"$0\"):$*\" >> \"$ARGV_LOG\"\n"+
		"if [ -n \"$FAIL_ARG\" ]; then\n"+
		"  case \"$*\" in\n"+
		"    *\"$FAIL_ARG\"*) exit 1 ;;\n"+
		"  esac\n"+
		"fi\n"+
		"exit 0\n")
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

func TestUninstall_HappyPath(t *testing.T) {
	installFakeOC(t)
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	h := &testutil.CaptureHandler{}
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

	if got := h.CountLevel(slog.LevelWarn); got != 0 {
		t.Errorf("warnCount = %d; want 0 on success path", got)
	}
}

func TestUninstall_PartialSecretFailureContinues(t *testing.T) {
	installFakeOC(t)
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	h := &testutil.CaptureHandler{}
	env := makeUninstallEnv(argvLog, opCredentialsSecretName, slog.New(h))

	s := &secretStore{}
	if err := s.Uninstall(context.Background(), env); err != nil {
		t.Fatalf("Uninstall must return nil even when a secret delete fails; got: %v", err)
	}

	lines := readArgvLog(t, argvLog)
	if len(lines) != 3 {
		t.Fatalf("expected 3 argv records (loop must continue past the failed delete), got %d: %v", len(lines), lines)
	}

	if got := h.CountLevel(slog.LevelWarn); got != 1 {
		t.Errorf("warnCount = %d; want 1 (one failing secret delete)", got)
	}
}

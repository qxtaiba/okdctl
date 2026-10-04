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

// installFakeOC installs an "oc" stub whose namespace-ownership query
// (the "-l ... --ignore-not-found -o name" shape NamespaceOwnedByOkdctl
// issues) answers owned when NS_OWNED=1, foreign otherwise.
func installFakeOC(t *testing.T) {
	t.Helper()
	testutil.InstallFakeBin(t, "oc", "#!/bin/sh\n"+
		"[ -n \"$ARGV_LOG\" ] && printf '%s\\n' \"$(basename \"$0\"):$*\" >> \"$ARGV_LOG\"\n"+
		"if [ -n \"$FAIL_ARG\" ]; then\n"+
		"  case \"$*\" in\n"+
		"    *\"$FAIL_ARG\"*) exit 1 ;;\n"+
		"  esac\n"+
		"fi\n"+
		"case \"$*\" in\n"+
		"  *\" -l \"*\"--ignore-not-found -o name\")\n"+
		"    [ \"$NS_OWNED\" = \"1\" ] && echo \"namespace/external-secrets\"\n"+
		"    ;;\n"+
		"esac\n"+
		"exit 0\n")
}

func makeUninstallEnv(argvLog, failArg string, log *slog.Logger) *addon.Environment {
	return makeUninstallEnvOwned(argvLog, failArg, true, log)
}

func makeUninstallEnvOwned(argvLog, failArg string, nsOwned bool, log *slog.Logger) *addon.Environment {
	extraEnv := []string{"ARGV_LOG=" + argvLog}
	if failArg != "" {
		extraEnv = append(extraEnv, "FAIL_ARG="+failArg)
	}
	if nsOwned {
		extraEnv = append(extraEnv, "NS_OWNED=1")
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
		"oc:delete secret onepassword-connect-credentials -n external-secrets --ignore-not-found",
		"oc:delete secret onepassword-connect-token -n external-secrets --ignore-not-found",
		"oc:delete secretstore okdctl-secretstore -n external-secrets --ignore-not-found",
		"oc:get namespace external-secrets -l okdctl.io/managed-by=okdctl --ignore-not-found -o name",
		"oc:delete namespace external-secrets --ignore-not-found",
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
	if err := s.Uninstall(context.Background(), env); err == nil {
		t.Fatal("cleanup failure lost")
	}

	lines := readArgvLog(t, argvLog)
	if len(lines) != 5 {
		t.Fatalf("expected 5 argv records (loop must continue past the failed delete), got %d: %v", len(lines), lines)
	}
}

// TestUninstall_PreservesForeignNamespace proves the destructive-compensation
// invariant's other direction: a namespace that exists but was never labeled
// by EnsureNamespace must never be deleted by Uninstall.
func TestUninstall_PreservesForeignNamespace(t *testing.T) {
	installFakeOC(t)
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	h := &testutil.CaptureHandler{}
	env := makeUninstallEnvOwned(argvLog, "", false, slog.New(h))

	s := &secretStore{}
	if err := s.Uninstall(context.Background(), env); err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}

	lines := readArgvLog(t, argvLog)
	for _, l := range lines {
		if strings.Contains(l, "delete namespace") {
			t.Fatalf("Uninstall deleted a namespace it did not create: %v", lines)
		}
	}
}

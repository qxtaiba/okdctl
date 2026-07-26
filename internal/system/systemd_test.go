package system

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/testutil"
)

// installFakeSystemctl writes a PATH-shadow systemctl that appends its argv
// to a log file and exits $SYSTEMCTL_EXIT (default 0). Returns the log path.
func installFakeSystemctl(t *testing.T) string {
	t.Helper()
	script := `#!/bin/sh
echo "$@" >> "$SYSTEMCTL_ARGV_LOG"
exit "${SYSTEMCTL_EXIT:-0}"
`
	dir := testutil.InstallFakeBin(t, "systemctl", script)
	argvLog := filepath.Join(dir, "argv.log")
	t.Setenv("SYSTEMCTL_ARGV_LOG", argvLog)
	return argvLog
}

func readSystemctlArgv(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("argv log not written: %v", err)
	}
	return strings.TrimSpace(string(data))
}

func TestManageService_ArgvShape(t *testing.T) {
	if runtime.GOOS != osLinux {
		t.Skip("argv-shape half needs the Linux exec path; the non-Linux half is tested below")
	}

	cases := []struct {
		action ServiceAction
		want   string
	}{
		{ServiceEnable, "enable haproxy"},
		{ServiceDisable, "disable haproxy"},
		{ServiceStart, "start haproxy"},
		{ServiceStop, "stop haproxy"},
		{ServiceRestart, "restart haproxy"},
		{ServiceReload, "reload haproxy"},
		{ServiceStatus, "is-active haproxy"},
	}
	for _, tc := range cases {
		t.Run(string(tc.action), func(t *testing.T) {
			argvLog := installFakeSystemctl(t)
			if err := ManageService(context.Background(), tc.action, "haproxy"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := readSystemctlArgv(t, argvLog); got != tc.want {
				t.Errorf("argv = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestManageService_NonZeroExitReturnsError(t *testing.T) {
	if runtime.GOOS != osLinux {
		t.Skip("needs the Linux exec path")
	}
	installFakeSystemctl(t)
	t.Setenv("SYSTEMCTL_EXIT", "1")
	if err := ManageService(context.Background(), ServiceRestart, "haproxy"); err == nil {
		t.Fatal("expected error for systemctl exit 1")
	}
}

func TestServiceProbes_ExitCodeMapsToBool(t *testing.T) {
	if runtime.GOOS != osLinux {
		t.Skip("needs the Linux exec path")
	}

	t.Run("exit 0 reports true with quiet argv", func(t *testing.T) {
		argvLog := installFakeSystemctl(t)
		if !IsServiceActive(context.Background(), "dnsmasq") {
			t.Error("IsServiceActive = false for exit 0; want true")
		}
		if got, want := readSystemctlArgv(t, argvLog), "is-active --quiet dnsmasq"; got != want {
			t.Errorf("argv = %q; want %q", got, want)
		}
	})

	t.Run("enabled probe uses is-enabled", func(t *testing.T) {
		argvLog := installFakeSystemctl(t)
		if !IsServiceEnabled(context.Background(), "dnsmasq") {
			t.Error("IsServiceEnabled = false for exit 0; want true")
		}
		if got, want := readSystemctlArgv(t, argvLog), "is-enabled --quiet dnsmasq"; got != want {
			t.Errorf("argv = %q; want %q", got, want)
		}
	})

	t.Run("exit 1 reports false", func(t *testing.T) {
		installFakeSystemctl(t)
		t.Setenv("SYSTEMCTL_EXIT", "1")
		if IsServiceActive(context.Background(), "dnsmasq") {
			t.Error("IsServiceActive = true for exit 1; want false")
		}
		if IsServiceEnabled(context.Background(), "dnsmasq") {
			t.Error("IsServiceEnabled = true for exit 1; want false")
		}
	})
}

// TestSystemd_NonLinuxFailsClosed locks the non-Linux semantics: ManageService
// errors and the probes report false without ever invoking systemctl — even
// when a systemctl binary is reachable on PATH.
func TestSystemd_NonLinuxFailsClosed(t *testing.T) {
	if runtime.GOOS == osLinux {
		t.Skip("locks the non-Linux branch; runs on darwin dev machines")
	}
	argvLog := installFakeSystemctl(t)

	if err := ManageService(context.Background(), ServiceRestart, "haproxy"); err == nil {
		t.Error("ManageService must error on non-Linux hosts")
	}
	if IsServiceActive(context.Background(), "haproxy") {
		t.Error("IsServiceActive must report false on non-Linux hosts")
	}
	if IsServiceEnabled(context.Background(), "haproxy") {
		t.Error("IsServiceEnabled must report false on non-Linux hosts")
	}
	if _, err := os.Stat(argvLog); err == nil {
		t.Errorf("systemctl was invoked on a non-Linux host: %s", readSystemctlArgv(t, argvLog))
	}
}

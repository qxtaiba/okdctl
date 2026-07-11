package phase

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// fakeTeardownBin writes an executable shell script that appends its argv to
// callLog and prepends its dir to PATH, mirroring the kubectl_test.go stub
// pattern.
func fakeTeardownBin(t *testing.T, name, script string) (callLog string) {
	t.Helper()
	dir := t.TempDir()
	callLog = filepath.Join(dir, "calls.log")
	body := "#!/bin/sh\necho \"$@\" >> " + callLog + "\n" + script + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return callLog
}

func readCalls(t *testing.T, callLog string) string {
	t.Helper()
	b, err := os.ReadFile(callLog)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	return string(b)
}

func TestStopAndDisableServiceActiveEnabled(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemctl branches are linux-only; darwin takes the GOOS gate")
	}
	callLog := fakeTeardownBin(t, "systemctl", "exit 0")

	StopAndDisableService(context.Background(), "haproxy", logutil.NopLogger)

	calls := readCalls(t, callLog)
	for _, want := range []string{"is-active --quiet haproxy", "stop haproxy", "is-enabled --quiet haproxy", "disable haproxy"} {
		if !strings.Contains(calls, want) {
			t.Errorf("systemctl calls missing %q; got:\n%s", want, calls)
		}
	}
}

func TestStopAndDisableServiceInactive(t *testing.T) {
	var callLog string
	if runtime.GOOS == "linux" {
		callLog = fakeTeardownBin(t, "systemctl", "case \"$1\" in is-*) exit 1;; *) exit 0;; esac")
	}

	StopAndDisableService(context.Background(), "haproxy", logutil.NopLogger)

	if runtime.GOOS == "linux" {
		calls := readCalls(t, callLog)
		if strings.Contains(calls, "stop haproxy") || strings.Contains(calls, "disable haproxy") {
			t.Errorf("inactive service must not be stopped or disabled; got:\n%s", calls)
		}
	}
}

func TestReleaseVIPEmptyIsNoop(t *testing.T) {
	callLog := fakeTeardownBin(t, "ip", "exit 0")

	ReleaseVIP(context.Background(), "", logutil.NopLogger)

	if calls := readCalls(t, callLog); calls != "" {
		t.Errorf("empty vip must not shell out; got:\n%s", calls)
	}
}

func TestReleaseVIPChecksDefaultInterface(t *testing.T) {
	callLog := fakeTeardownBin(t, "ip",
		"case \"$1\" in route) echo \"default via 192.168.1.1 dev eth0\";; esac\nexit 0")

	ReleaseVIP(context.Background(), "192.168.1.50", logutil.NopLogger)

	calls := readCalls(t, callLog)
	if !strings.Contains(calls, "route show default") {
		t.Errorf("expected default-route lookup; got:\n%s", calls)
	}
	// The stub reports no addresses on eth0, so removal stops at the
	// presence check — asserting the no-op contract when the vip is absent.
	if !strings.Contains(calls, "addr show dev eth0") {
		t.Errorf("expected vip presence check on eth0; got:\n%s", calls)
	}
	if strings.Contains(calls, "del") {
		t.Errorf("absent vip must not be deleted; got:\n%s", calls)
	}
}

func TestReleaseVIPDetectFailureIsBestEffort(t *testing.T) {
	callLog := fakeTeardownBin(t, "ip", "exit 1")

	ReleaseVIP(context.Background(), "192.168.1.50", logutil.NopLogger)

	calls := readCalls(t, callLog)
	if strings.Contains(calls, "del") {
		t.Errorf("removal must not run when detection fails; got:\n%s", calls)
	}
}

package firewall

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/testutil"
)

// setBackendSeams overrides the platform/firewalld seams so tests can run off Linux.
func setBackendSeams(t *testing.T, osName string, firewalldActive bool) {
	t.Helper()
	origGoos, origSvc := goos, isServiceActiveFn
	goos = osName
	isServiceActiveFn = func(_ context.Context, svc string) bool {
		return firewalldActive && svc == "firewalld"
	}
	t.Cleanup(func() {
		goos = origGoos
		isServiceActiveFn = origSvc
	})
}

// installRecordingFirewallCmd plants a fake firewall-cmd that logs argv to a
// file and runs extra shell logic.
func installRecordingFirewallCmd(t *testing.T, extra string) string {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "firewall-cmd.log")
	testutil.InstallFakeBin(t, "firewall-cmd", "#!/bin/sh\necho \"$@\" >> "+logPath+"\n"+extra+"exit 0\n")
	return logPath
}

func recordedArgv(t *testing.T, logPath string) []string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func TestModifyPortArgv(t *testing.T) {
	cases := []struct {
		name      string
		port      Port
		permanent bool
		action    string
		want      string
	}{
		{"add permanent", Port{Number: 6443, Protocol: "tcp"}, true, actionAdd, "--add-port=6443/tcp --permanent"},
		{"add runtime", Port{Number: 6443, Protocol: "tcp"}, false, actionAdd, "--add-port=6443/tcp"},
		{"remove permanent", Port{Number: 6443, Protocol: "tcp"}, true, actionRemove, "--remove-port=6443/tcp --permanent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logPath := installRecordingFirewallCmd(t, "")
			if err := modifyPort(context.Background(), tc.port, tc.permanent, tc.action); err != nil {
				t.Fatalf("modifyPort: %v", err)
			}
			calls := recordedArgv(t, logPath)
			if len(calls) != 1 || calls[0] != tc.want {
				t.Errorf("firewall-cmd argv = %q, want exactly [%q]", calls, tc.want)
			}
		})
	}
}

func TestModifyPort_ValidatesBeforeExec(t *testing.T) {
	logPath := installRecordingFirewallCmd(t, "")
	bad := []Port{
		{Number: 6443, Protocol: "tcp; rm -rf /"},
		{Number: 0, Protocol: "tcp"},
		{Number: 65536, Protocol: "udp"},
	}
	for _, p := range bad {
		if err := modifyPort(context.Background(), p, false, actionAdd); err == nil {
			t.Errorf("port %+v must be rejected", p)
		}
	}
	if calls := recordedArgv(t, logPath); len(calls) != 0 {
		t.Fatalf("rejected ports must never reach the firewall binary, got %q", calls)
	}
}

func TestConfigure_AbortsOnFirstFailure(t *testing.T) {
	setBackendSeams(t, "linux", true)
	logPath := installRecordingFirewallCmd(t, "case \"$*\" in --add-port=*) exit 1 ;; esac\n")

	ports := []Port{
		{Number: 80, Protocol: "tcp"},
		{Number: 443, Protocol: "tcp"},
	}
	err := New().Configure(context.Background(), ports, false)
	if err == nil || !strings.Contains(err.Error(), "open port 80") {
		t.Fatalf("want failure on the first port, got: %v", err)
	}
	calls := recordedArgv(t, logPath)
	if len(calls) != 1 || calls[0] != "--add-port=80/tcp" {
		t.Errorf("firewall-cmd calls = %q; the second port must not be attempted after the first fails", calls)
	}
}

func TestRemoveRules_ContinuesPastFailure(t *testing.T) {
	setBackendSeams(t, "linux", true)
	logPath := installRecordingFirewallCmd(t, "case \"$*\" in --remove-port=80/tcp) exit 1 ;; esac\n")

	ports := []Port{
		{Number: 80, Protocol: "tcp"},
		{Number: 443, Protocol: "tcp"},
	}
	if err := New().RemoveRules(context.Background(), ports, false); err != nil {
		t.Fatalf("RemoveRules must warn-and-continue, got: %v", err)
	}
	calls := recordedArgv(t, logPath)
	want := []string{"--remove-port=80/tcp", "--remove-port=443/tcp"}
	if len(calls) != len(want) {
		t.Fatalf("firewall-cmd calls = %q, want %q", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("firewall-cmd call %d = %q, want %q", i, calls[i], want[i])
		}
	}
}

func TestConfigure_FirewalldPermanentReloads(t *testing.T) {
	setBackendSeams(t, "linux", true)
	logPath := installRecordingFirewallCmd(t, "")

	err := New().Configure(context.Background(), []Port{{Number: 6443, Protocol: "tcp"}}, true)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	calls := recordedArgv(t, logPath)
	want := []string{"--add-port=6443/tcp --permanent", "--reload"}
	if len(calls) != len(want) || calls[0] != want[0] || calls[1] != want[1] {
		t.Errorf("firewall-cmd calls = %q, want %q", calls, want)
	}
}

func TestConfigure_NoneBackendIsNoOp(t *testing.T) {
	setBackendSeams(t, "plan9", false)
	if err := New().Configure(context.Background(), OKDRequiredPorts, true); err != nil {
		t.Fatalf("Configure with no backend must no-op, got: %v", err)
	}
	if err := New().RemoveRules(context.Background(), OKDRequiredPorts, true); err != nil {
		t.Fatalf("RemoveRules with no backend must no-op, got: %v", err)
	}
}

func TestDetectBackend(t *testing.T) {
	t.Run("non-linux is None", func(t *testing.T) {
		setBackendSeams(t, "darwin", true)
		if got := New().DetectBackend(context.Background()); got != None {
			t.Errorf("DetectBackend = %v, want None off Linux", got)
		}
	})

	t.Run("active firewalld is Firewalld", func(t *testing.T) {
		setBackendSeams(t, "linux", true)
		installRecordingFirewallCmd(t, "")
		if got := New().DetectBackend(context.Background()); got != Firewalld {
			t.Errorf("DetectBackend = %v, want Firewalld", got)
		}
	})

	t.Run("inactive firewalld is None", func(t *testing.T) {
		setBackendSeams(t, "linux", false)
		installRecordingFirewallCmd(t, "")
		if got := New().DetectBackend(context.Background()); got != None {
			t.Errorf("DetectBackend = %v, want None with firewalld inactive", got)
		}
	})

	t.Run("empty PATH is None", func(t *testing.T) {
		setBackendSeams(t, "linux", true)
		t.Setenv("PATH", t.TempDir())
		if got := New().DetectBackend(context.Background()); got != None {
			t.Errorf("DetectBackend = %v, want None with no binaries present", got)
		}
	})
}

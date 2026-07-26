package firewall

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/testutil"
)

// setBackendSeams overrides the DetectBackend platform gate and firewalld
// probe, which are otherwise hard-gated off on non-Linux dev hosts.
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

// installRecordingFirewallBin plants a fake firewall binary that appends its
// argv to a log file, runs extra shell logic (may exit non-zero), and
// returns the log path.
func installRecordingFirewallBin(t *testing.T, name, extra string) string {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), name+".log")
	testutil.InstallFakeBin(t, name, "#!/bin/sh\necho \"$@\" >> "+logPath+"\n"+extra+"exit 0\n")
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

// TestModifyPortArgv locks the exact per-backend rule dialects. A silent
// argv regression either fails to open required OKD ports or leaves them
// open after cleanup.
func TestModifyPortArgv(t *testing.T) {
	cases := []struct {
		name      string
		backend   Backend
		bin       string
		port      Port
		permanent bool
		action    string
		want      string
	}{
		{"firewalld add permanent", Firewalld, "firewall-cmd", Port{Number: 6443, Protocol: "tcp"}, true, actionAdd, "--add-port=6443/tcp --permanent"},
		{"firewalld add runtime", Firewalld, "firewall-cmd", Port{Number: 6443, Protocol: "tcp"}, false, actionAdd, "--add-port=6443/tcp"},
		{"firewalld remove permanent", Firewalld, "firewall-cmd", Port{Number: 6443, Protocol: "tcp"}, true, actionRemove, "--remove-port=6443/tcp --permanent"},
		{"ufw add", UFW, "ufw", Port{Number: 6443, Protocol: "tcp"}, false, actionAdd, "allow 6443/tcp"},
		{"ufw remove", UFW, "ufw", Port{Number: 6443, Protocol: "tcp"}, false, actionRemove, "delete allow 6443/tcp"},
		{"iptables add", IPTables, "iptables", Port{Number: 6443, Protocol: "tcp"}, false, actionAdd, "-I INPUT -p tcp --dport 6443 -j ACCEPT"},
		{"iptables remove udp", IPTables, "iptables", Port{Number: 53, Protocol: "udp"}, false, actionRemove, "-D INPUT -p udp --dport 53 -j ACCEPT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logPath := installRecordingFirewallBin(t, tc.bin, "")
			if err := modifyPort(context.Background(), tc.backend, tc.port, tc.permanent, tc.action); err != nil {
				t.Fatalf("modifyPort: %v", err)
			}
			calls := recordedArgv(t, logPath)
			if len(calls) != 1 || calls[0] != tc.want {
				t.Errorf("%s argv = %q, want exactly [%q]", tc.bin, calls, tc.want)
			}
		})
	}
}

// TestModifyPort_ValidatesBeforeExec locks the ordering contract: the
// validatePort allowlist must run before any argv construction, so a hostile
// protocol string never reaches a firewall binary.
func TestModifyPort_ValidatesBeforeExec(t *testing.T) {
	logPath := installRecordingFirewallBin(t, "iptables", "")
	bad := []Port{
		{Number: 6443, Protocol: "tcp; rm -rf /"},
		{Number: 0, Protocol: "tcp"},
		{Number: 65536, Protocol: "udp"},
	}
	for _, p := range bad {
		if err := modifyPort(context.Background(), IPTables, p, false, actionAdd); err == nil {
			t.Errorf("port %+v must be rejected", p)
		}
	}
	if calls := recordedArgv(t, logPath); len(calls) != 0 {
		t.Fatalf("rejected ports must never reach the firewall binary, got %q", calls)
	}
}

// TestConfigure_AbortsOnFirstFailure: Configure must stop at the first port
// that fails to open (partial firewall state is surfaced, not skipped past).
func TestConfigure_AbortsOnFirstFailure(t *testing.T) {
	setBackendSeams(t, "linux", false)
	logPath := installRecordingFirewallBin(t, "ufw",
		"case \"$*\" in status) echo 'Status: active' ;; allow*) exit 1 ;; esac\n")

	ports := []Port{
		{Number: 80, Protocol: "tcp"},
		{Number: 443, Protocol: "tcp"},
	}
	err := New().Configure(context.Background(), ports, false)
	if err == nil || !strings.Contains(err.Error(), "open port 80") {
		t.Fatalf("want failure on the first port, got: %v", err)
	}
	calls := recordedArgv(t, logPath)
	// One detection probe (status) plus exactly one allow attempt.
	if len(calls) != 2 || calls[1] != "allow 80/tcp" {
		t.Errorf("ufw calls = %q; the second port must not be attempted after the first fails", calls)
	}
}

// TestRemoveRules_ContinuesPastFailure locks the warn-and-continue contract:
// a rule that fails to delete must not stop later rules from being removed.
func TestRemoveRules_ContinuesPastFailure(t *testing.T) {
	setBackendSeams(t, "linux", false)
	logPath := installRecordingFirewallBin(t, "ufw",
		"case \"$*\" in status) echo 'Status: active' ;; 'delete allow 80/tcp') exit 1 ;; esac\n")

	ports := []Port{
		{Number: 80, Protocol: "tcp"},
		{Number: 443, Protocol: "tcp"},
	}
	if err := New().RemoveRules(context.Background(), ports, false); err != nil {
		t.Fatalf("RemoveRules must warn-and-continue, got: %v", err)
	}
	calls := recordedArgv(t, logPath)
	want := []string{"status", "delete allow 80/tcp", "delete allow 443/tcp"}
	if len(calls) != len(want) {
		t.Fatalf("ufw calls = %q, want %q", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("ufw call %d = %q, want %q", i, calls[i], want[i])
		}
	}
}

// TestConfigure_FirewalldPermanentReloads: permanent firewalld rules must be
// followed by a --reload or they never take effect in the running config.
func TestConfigure_FirewalldPermanentReloads(t *testing.T) {
	setBackendSeams(t, "linux", true)
	logPath := installRecordingFirewallBin(t, "firewall-cmd", "")

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

func TestDetectBackend_Precedence(t *testing.T) {
	t.Run("non-linux is None", func(t *testing.T) {
		setBackendSeams(t, "darwin", true)
		if got := New().DetectBackend(context.Background()); got != None {
			t.Errorf("DetectBackend = %v, want None off Linux", got)
		}
	})

	t.Run("active firewalld wins over ufw and iptables", func(t *testing.T) {
		setBackendSeams(t, "linux", true)
		installRecordingFirewallBin(t, "firewall-cmd", "")
		installRecordingFirewallBin(t, "ufw", "echo 'Status: active'\n")
		installRecordingFirewallBin(t, "iptables", "")
		if got := New().DetectBackend(context.Background()); got != Firewalld {
			t.Errorf("DetectBackend = %v, want Firewalld", got)
		}
	})

	t.Run("active ufw wins over iptables", func(t *testing.T) {
		setBackendSeams(t, "linux", false)
		installRecordingFirewallBin(t, "ufw", "echo 'Status: active'\n")
		installRecordingFirewallBin(t, "iptables", "")
		if got := New().DetectBackend(context.Background()); got != UFW {
			t.Errorf("DetectBackend = %v, want UFW", got)
		}
	})

	t.Run("inactive ufw falls through to iptables", func(t *testing.T) {
		setBackendSeams(t, "linux", false)
		t.Setenv("PATH", t.TempDir())
		installRecordingFirewallBin(t, "ufw", "echo 'Status: inactive'\n")
		installRecordingFirewallBin(t, "iptables", "")
		if got := New().DetectBackend(context.Background()); got != IPTables {
			t.Errorf("DetectBackend = %v, want IPTables", got)
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

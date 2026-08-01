package dns

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/testutil"
)

// setResolverProbes overrides the NetworkManager/systemd-resolved activity
// probes, which are hard-gated off by runtime.GOOS on non-Linux dev hosts.
func setResolverProbes(t *testing.T, nmActive bool, activeServices map[string]bool) {
	t.Helper()
	origNM, origSvc := isNetworkManagerActiveFn, isServiceActiveFn
	isNetworkManagerActiveFn = func(_ context.Context) bool { return nmActive }
	isServiceActiveFn = func(_ context.Context, svc string) bool { return activeServices[svc] }
	t.Cleanup(func() {
		isNetworkManagerActiveFn = origNM
		isServiceActiveFn = origSvc
	})
}

func redirectResolvedConf(t *testing.T) string {
	t.Helper()
	orig := resolvedConf
	resolvedConf = filepath.Join(t.TempDir(), "resolved.conf.d", "dnsmasq.conf")
	t.Cleanup(func() { resolvedConf = orig })
	return resolvedConf
}

// installRecordingBin plants a fake bin that appends its argv to a log file
// and returns the log path for later assertions.
func installRecordingBin(t *testing.T, name, extra string) string {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), name+".log")
	testutil.InstallFakeBin(t, name, "#!/bin/sh\necho \"$@\" >> "+logPath+"\n"+extra+"exit 0\n")
	return logPath
}

func recordedCalls(t *testing.T, logPath string) []string {
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

func TestConfigureSystemResolver_InvalidFallbackDNSRefused(t *testing.T) {
	setResolverProbes(t, true, nil)
	confPath := redirectResolvedConf(t)
	nmcliLog := installRecordingBin(t, "nmcli", "")

	err := ConfigureSystemResolver(context.Background(), []string{"8.8.8.8", "not-an-ip"}, logutil.NopLogger)
	if err == nil || !strings.Contains(err.Error(), "invalid fallback DNS") {
		t.Fatalf("hostile fallback DNS must be refused before any mutation, got: %v", err)
	}
	if calls := recordedCalls(t, nmcliLog); len(calls) != 0 {
		t.Errorf("nmcli must not run on the refusal path, got %q", calls)
	}
	if _, err := os.Stat(confPath); !os.IsNotExist(err) {
		t.Error("no resolved drop-in may be written on the refusal path")
	}
}

// TestConfigureSystemResolver_NetworkManagerPath locks the exact nmcli argv
// the forward path issues: DNS override to local dnsmasq plus fallbacks,
// auto-DNS off, then connection re-up.
func TestConfigureSystemResolver_NetworkManagerPath(t *testing.T) {
	setResolverProbes(t, true, nil)
	nmcliLog := installRecordingBin(t, "nmcli",
		"case \"$*\" in *'connection show --active'*) echo 'eth-conn' ;; esac\n")

	err := ConfigureSystemResolver(context.Background(), []string{"8.8.8.8"}, logutil.NopLogger)
	if err != nil {
		t.Fatalf("ConfigureSystemResolver: %v", err)
	}

	calls := recordedCalls(t, nmcliLog)
	want := []string{
		"-t -f NAME connection show --active",
		"-g ipv4.dns connection show eth-conn",
		"-g ipv4.ignore-auto-dns connection show eth-conn",
		"connection modify eth-conn ipv4.dns 127.0.0.1,8.8.8.8 ipv4.ignore-auto-dns yes",
		"connection up eth-conn",
	}
	if len(calls) != len(want) {
		t.Fatalf("nmcli calls = %q, want %q", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("nmcli call %d = %q, want %q", i, calls[i], want[i])
		}
	}
}

// TestConfigureSystemResolver_SystemdResolvedPath locks the drop-in install:
// exact [Resolve] content, world-readable-but-not-writable mode, and the
// follow-up daemon restart.
func TestConfigureSystemResolver_SystemdResolvedPath(t *testing.T) {
	setResolverProbes(t, false, map[string]bool{"systemd-resolved": true})
	confPath := redirectResolvedConf(t)
	systemctlLog := installRecordingBin(t, "systemctl", "")

	err := ConfigureSystemResolver(context.Background(), []string{"8.8.8.8"}, logutil.NopLogger)
	if err != nil {
		t.Fatalf("ConfigureSystemResolver: %v", err)
	}

	got, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("drop-in not installed: %v", err)
	}
	if want := "[Resolve]\nDNS=127.0.0.1\nDomains=~.\n"; string(got) != want {
		t.Errorf("drop-in content = %q, want %q", got, want)
	}
	fi, err := os.Stat(confPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o644 {
		t.Errorf("drop-in perm = %#o, want 0644", perm)
	}
	calls := recordedCalls(t, systemctlLog)
	if len(calls) != 1 || calls[0] != "restart systemd-resolved" {
		t.Errorf("systemctl calls = %q, want exactly [restart systemd-resolved]", calls)
	}
}

// TestConfigureSystemResolver_NetworkManagerUpFailureReverts asserts that a
// failed `connection up` reverts the profile to the captured DNS rather than
// leaving the host pinned at a possibly-dead 127.0.0.1, and names the revert.
func TestConfigureSystemResolver_NetworkManagerUpFailureReverts(t *testing.T) {
	setResolverProbes(t, true, nil)
	nmcliLog := installRecordingBin(t, "nmcli",
		"case \"$*\" in "+
			"*'connection show --active'*) echo 'eth-conn' ;; "+
			"*'-g ipv4.dns'*) echo '9.9.9.9' ;; "+
			"*'-g ipv4.ignore-auto-dns'*) echo 'no' ;; "+
			"*'connection up'*) exit 1 ;; "+
			"esac\n")

	err := ConfigureSystemResolver(context.Background(), []string{"8.8.8.8"}, logutil.NopLogger)
	if err == nil || !strings.Contains(err.Error(), "reverted to previous DNS") {
		t.Fatalf("up failure must revert the profile and say so, got: %v", err)
	}

	calls := recordedCalls(t, nmcliLog)
	revert := "connection modify eth-conn ipv4.dns 9.9.9.9 ipv4.ignore-auto-dns no"
	if !slices.Contains(calls, revert) {
		t.Errorf("expected revert call %q in %q", revert, calls)
	}
}

// TestConfigureSystemResolver_SystemdRestartFailureRemovesDropIn asserts that a
// failed systemd-resolved restart removes the drop-in so the host falls back to
// its prior resolver instead of a dead one.
func TestConfigureSystemResolver_SystemdRestartFailureRemovesDropIn(t *testing.T) {
	setResolverProbes(t, false, map[string]bool{"systemd-resolved": true})
	confPath := redirectResolvedConf(t)
	installRecordingBin(t, "systemctl", "case \"$*\" in *restart*) exit 1 ;; esac\n")

	err := ConfigureSystemResolver(context.Background(), []string{"8.8.8.8"}, logutil.NopLogger)
	if err == nil || !strings.Contains(err.Error(), "drop-in") {
		t.Fatalf("restart failure must report the drop-in outcome, got: %v", err)
	}
	if _, statErr := os.Stat(confPath); !os.IsNotExist(statErr) {
		t.Errorf("expected drop-in removed after restart failure, stat err = %v", statErr)
	}
}

func TestConfigureSystemResolver_NeitherManagerIsWarnOnlyNoOp(t *testing.T) {
	setResolverProbes(t, false, nil)
	confPath := redirectResolvedConf(t)

	if err := ConfigureSystemResolver(context.Background(), []string{"8.8.8.8"}, logutil.NopLogger); err != nil {
		t.Fatalf("no resolver manager must be a warn-only no-op, got: %v", err)
	}
	if _, err := os.Stat(confPath); !os.IsNotExist(err) {
		t.Error("no drop-in may be written when neither manager is active")
	}
}

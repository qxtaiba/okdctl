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

// setResolverProbes overrides the NetworkManager probe, which is
// runtime.GOOS-gated off on non-Linux dev hosts.
func setResolverProbes(t *testing.T, nmActive bool) {
	t.Helper()
	orig := isNetworkManagerActiveFn
	isNetworkManagerActiveFn = func(_ context.Context) bool { return nmActive }
	t.Cleanup(func() { isNetworkManagerActiveFn = orig })
}

func installRecordingNmcli(t *testing.T, extra string) string {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "nmcli.log")
	testutil.InstallFakeBin(t, "nmcli", "#!/bin/sh\necho \"$@\" >> "+logPath+"\n"+extra+"exit 0\n")
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
	setResolverProbes(t, true)
	nmcliLog := installRecordingNmcli(t, "")

	err := ConfigureSystemResolver(context.Background(), []string{"8.8.8.8", "not-an-ip"}, logutil.NopLogger)
	if err == nil || !strings.Contains(err.Error(), "invalid fallback DNS") {
		t.Fatalf("hostile fallback DNS must be refused before any mutation, got: %v", err)
	}
	if calls := recordedCalls(t, nmcliLog); len(calls) != 0 {
		t.Errorf("nmcli must not run on the refusal path, got %q", calls)
	}
}

func TestConfigureSystemResolver_NetworkManagerPath(t *testing.T) {
	setResolverProbes(t, true)
	nmcliLog := installRecordingNmcli(t, "case \"$*\" in *'connection show --active'*) echo 'eth-conn' ;; esac\n")

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

func TestConfigureSystemResolver_NetworkManagerUpFailureReverts(t *testing.T) {
	setResolverProbes(t, true)
	nmcliLog := installRecordingNmcli(t, "case \"$*\" in "+
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

func TestConfigureSystemResolver_WithoutNetworkManagerIsWarnOnlyNoOp(t *testing.T) {
	setResolverProbes(t, false)
	nmcliLog := installRecordingNmcli(t, "")

	if err := ConfigureSystemResolver(context.Background(), []string{"8.8.8.8"}, logutil.NopLogger); err != nil {
		t.Fatalf("inactive NetworkManager must be a warn-only no-op, got: %v", err)
	}
	if calls := recordedCalls(t, nmcliLog); len(calls) != 0 {
		t.Errorf("nmcli must not run when NetworkManager is inactive, got %q", calls)
	}
}

package dns

import (
	"context"
	"slices"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

func TestRestoreSystemResolver_ClearsNetworkManagerOverride(t *testing.T) {
	setResolverProbes(t, true)
	nmcliLog := installRecordingNmcli(t, "case \"$*\" in *'connection show --active'*) echo 'eth-conn' ;; esac\n")

	if err := RestoreSystemResolver(context.Background(), logutil.NopLogger); err != nil {
		t.Fatalf("RestoreSystemResolver: %v", err)
	}

	want := []string{
		"-t -f NAME connection show --active",
		"connection modify eth-conn ipv4.dns  ipv4.ignore-auto-dns no",
		"connection up eth-conn",
	}
	if calls := recordedCalls(t, nmcliLog); !slices.Equal(calls, want) {
		t.Errorf("nmcli calls = %q, want %q", calls, want)
	}
}

func TestRestoreSystemResolver_FailuresAreNotPropagated(t *testing.T) {
	setResolverProbes(t, true)
	installRecordingNmcli(t, "case \"$*\" in *'connection show --active'*) echo 'eth-conn' ;; *) exit 1 ;; esac\n")

	if err := RestoreSystemResolver(context.Background(), logutil.NopLogger); err != nil {
		t.Fatalf("RestoreSystemResolver must not propagate the failure, got: %v", err)
	}
}

func TestRestoreSystemResolver_WithoutNetworkManagerIsNoOp(t *testing.T) {
	setResolverProbes(t, false)
	nmcliLog := installRecordingNmcli(t, "")

	if err := RestoreSystemResolver(context.Background(), logutil.NopLogger); err != nil {
		t.Fatalf("RestoreSystemResolver: %v", err)
	}
	if calls := recordedCalls(t, nmcliLog); len(calls) != 0 {
		t.Errorf("nmcli must not run when NetworkManager is inactive, got %q", calls)
	}
}

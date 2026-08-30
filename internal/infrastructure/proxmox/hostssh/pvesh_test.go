package hostssh

import (
	"context"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
)

// TestPveshRun_ComposesNodeScopedPath pins the chokepoint contract:
// /nodes/<node>/ is composed from the same p.Node validateProxmoxName checks.
func TestPveshRun_ComposesNodeScopedPath(t *testing.T) {
	installFakeSSHEcho(t)
	p := &RemoteISOParams{Node: "pve-01", Host: "pve-test", Exec: executor.New()}

	stdout, err := PveshRun(context.Background(), p, "get", "qemu")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(stdout, "pvesh get /nodes/pve-01/qemu") {
		t.Errorf("argv = %q; want composed path /nodes/pve-01/qemu", stdout)
	}
}

// A hand-edited config bypassing ValidateOKDConfig must still be refused at
// the pvesh boundary, before ssh runs (p.Exec/p.Host go unused because
// validateProxmoxName fires first).
func TestPveshRun_RejectsInvalidNode(t *testing.T) {
	p := &RemoteISOParams{Node: "bad;rm -rf /", Host: "ignored"}
	if _, err := PveshRun(t.Context(), p, "get", "qemu"); err == nil {
		t.Fatal("expected error for malformed node name; got nil")
	}
}

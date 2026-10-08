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
	p := &PveshParams{Node: "pve-01", Host: "pve-test", Exec: executor.New()}

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
	p := &PveshParams{Node: "bad;rm -rf /", Host: "ignored"}
	if _, err := PveshRun(t.Context(), p, "get", "qemu"); err == nil {
		t.Fatal("expected error for malformed node name; got nil")
	}
}

func TestValidateProxmoxName(t *testing.T) {
	accept := []string{"pve-1", "node_a", "PVE0", "1pve", "A"}
	for _, name := range accept {
		if err := validateProxmoxName(name); err != nil {
			t.Errorf("validateProxmoxName(%q) rejected; want nil: %v", name, err)
		}
	}

	reject := []string{
		"",
		"pve.example",
		"pve/etc",
		"pve;rm",
		"pve`id`",
		"pve$(id)",
		"pve space",
		"pvé",
		"pve\x00",
		"pve\ttab",
		"..",
		"/",
		"node|pipe",
		"node&bg",
	}
	for _, name := range reject {
		if err := validateProxmoxName(name); err == nil {
			t.Errorf("validateProxmoxName(%q) accepted; want error", name)
		}
	}
}

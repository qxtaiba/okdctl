package phase

import (
	"strings"
	"testing"
)

func TestPveshQEMUPath(t *testing.T) {
	got := pveshQEMUPath("pve-01")
	want := "/nodes/pve-01/qemu"
	if got != want {
		t.Errorf("pveshQEMUPath(%q) = %q; want %q", "pve-01", got, want)
	}
}

func TestPveshConfigPath(t *testing.T) {
	got := pveshConfigPath("pve-01", 102)
	want := "/nodes/pve-01/qemu/102/config"
	if got != want {
		t.Errorf("pveshConfigPath = %q; want %q", got, want)
	}
}

// TestPveshRun_RejectsInvalidNode covers the centralized validation guard:
// a hand-edited config that bypasses ValidateOKDConfig must still be refused
// here before the ssh argv reaches the remote shell.
func TestPveshRun_RejectsInvalidNode(t *testing.T) {
	// p.Exec / p.Host are not used because validateProxmoxName fires first.
	p := &RemoteISOParams{Node: "bad;rm -rf /", Host: "ignored"}
	_, err := pveshRun(t.Context(), p, "get", "/nodes/x/qemu")
	if err == nil {
		t.Fatal("expected error for malformed node name; got nil")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("err = %q; want substring 'invalid'", err.Error())
	}
}

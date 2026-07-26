package hostssh

import (
	"context"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/executor"
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

// TestPveshRun_ComposesNodeScopedPath pins the chokepoint contract: callers
// hand PveshRun a node-relative resource and the /nodes/<node>/ prefix is
// composed here, from the same p.Node that validateProxmoxName checks.
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

// TestPveshRun_ExportedRejectsInvalidNode proves the exported entry point
// cannot be used to smuggle a bad node atom into the composed path.
func TestPveshRun_ExportedRejectsInvalidNode(t *testing.T) {
	p := &RemoteISOParams{Node: "bad;rm -rf /", Host: "ignored"}
	if _, err := PveshRun(t.Context(), p, "get", "qemu"); err == nil {
		t.Fatal("expected error for malformed node name; got nil")
	}
}

func TestValidateProxmoxName_RejectsBadNode(t *testing.T) {
	bad := []string{
		"",
		".",
		"..",
		"/",
		"node/name",
		"node;name",
		"node name",
		"node\tname",
		"node`cmd`",
		"$(reboot)",
		"node|pipe",
		"node&bg",
	}
	for _, name := range bad {
		if err := validateProxmoxName(name); err == nil {
			t.Errorf("validateProxmoxName(%q) accepted; want error", name)
		}
	}
}

func TestValidateProxmoxName_AcceptsValidNames(t *testing.T) {
	good := []string{
		"pve",
		"pve-01",
		"pve_01",
		"PVE01",
		"proxmox-node-1",
		"A",
		"node123",
	}
	for _, name := range good {
		if err := validateProxmoxName(name); err != nil {
			t.Errorf("validateProxmoxName(%q) rejected; want nil: %v", name, err)
		}
	}
}

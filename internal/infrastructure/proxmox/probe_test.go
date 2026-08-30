package proxmox

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/luthermonson/go-proxmox"

	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func TestSelectNodeMem(t *testing.T) {
	nodes := proxmox.NodeStatuses{
		{Node: "pve1", MaxMem: 100},
		{Node: "pve2", MaxMem: 200},
	}
	total, err := selectNodeMem(nodes, "pve2")
	if err != nil {
		t.Fatalf("selectNodeMem: %v", err)
	}
	if total != 200 {
		t.Fatalf("want 200, got %d", total)
	}
	if _, err := selectNodeMem(nodes, "ghost"); err == nil {
		t.Fatal("want error for missing node")
	}
}

func TestSumRunningGuestMem(t *testing.T) {
	resources := proxmox.ClusterResources{
		{Type: "qemu", Node: "pve1", Status: "running", MaxMem: 1000},
		{Type: "qemu", Node: "pve1", Status: "running", MaxMem: 2000},
		{Type: "qemu", Node: "pve1", Status: "stopped", MaxMem: 4000},
		{Type: "qemu", Node: "pve2", Status: "running", MaxMem: 8000},
		{Type: "lxc", Node: "pve1", Status: "running", MaxMem: 500},
	}
	got := sumRunningGuestMem(resources, "pve1")
	if got != 3000 {
		t.Fatalf("want 3000 (only running qemu on pve1), got %d", got)
	}
}

func TestMapVMStates(t *testing.T) {
	resources := proxmox.ClusterResources{
		{Type: "qemu", VMID: 110, Status: "running"},
		{Type: "qemu", VMID: 111, Status: "stopped"},
		{Type: "qemu", VMID: 200, Status: "suspended"}, // outside the wire vocabulary
		{Type: "qemu", VMID: 999, Status: "running"},   // not one of ours
		{Type: "lxc", VMID: 112, Status: "running"},
	}
	got := mapVMStates(resources, []int{110, 111, 112, 200, 300})
	if len(got) != 3 {
		t.Fatalf("mapVMStates = %v; want 3 entries", got)
	}
	if got[110] != nodetypes.StateRunning || got[111] != nodetypes.StateStopped {
		t.Errorf("wire states = %v/%v; want running/stopped", got[110], got[111])
	}
	if got[200] != nodetypes.StateUnknown {
		t.Errorf("unrecognized status = %v; want %v", got[200], nodetypes.StateUnknown)
	}
	if _, ok := got[300]; ok {
		t.Error("vm absent from the listing must be omitted, not defaulted")
	}
}

func TestDedupe(t *testing.T) {
	got := dedupe([]string{"a", "a", "", "b", "a"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("dedupe = %v", got)
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	cases := map[string]string{
		"https://pve:8006/":    "https://pve:8006",
		"pve.example.com":      "https://pve.example.com",
		"pve.example.com/":     "https://pve.example.com",
		"http://10.0.0.1:8006": "http://10.0.0.1:8006",
	}
	for in, want := range cases {
		if got := normalizeEndpoint(in); got != want {
			t.Errorf("normalizeEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewProxmoxClientRequiresCreds(t *testing.T) {
	if _, err := newProxmoxClient("https://pve:8006", "", nil, nil, false, defaultProbeTimeout); err == nil {
		t.Fatal("want error when neither password nor token is set")
	}
	if _, err := newProxmoxClient("https://pve:8006", "", nil, []byte("no-equals"), false, defaultProbeTimeout); err == nil {
		t.Fatal("want error for malformed api token")
	}
	if _, err := newProxmoxClient("https://pve:8006", "", nil, []byte("user@pam!t=secret"), false, defaultProbeTimeout); err != nil {
		t.Fatalf("valid token should build client: %v", err)
	}
}

func TestProbeHostRequiresNode(t *testing.T) {
	_, err := ProbeHost(t.Context(), &ProbeOptions{Endpoint: "https://pve:8006", APIToken: []byte("user@pam!t=secret")})
	if err == nil || !strings.Contains(err.Error(), "node is required") {
		t.Fatalf("ProbeHost without node: err = %v; want node-is-required error", err)
	}
}

func TestNewProxmoxClientWarnsOnInsecureOnce(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	build := func() {
		t.Helper()
		if _, err := newProxmoxClient("https://pve:8006", "", nil, []byte("user@pam!t=secret"), true, time.Second); err != nil {
			t.Fatalf("newProxmoxClient: %v", err)
		}
	}

	build()
	if !strings.Contains(buf.String(), "tls verification disabled") {
		t.Fatalf("expected insecure tls warning, log = %q", buf.String())
	}
	buf.Reset()
	build()
	if strings.Contains(buf.String(), "tls verification disabled") {
		t.Errorf("warning must fire once per process, log = %q", buf.String())
	}
}

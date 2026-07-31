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
		{Node: "pve1", MaxMem: 100, Mem: 40},
		{Node: "pve2", MaxMem: 200, Mem: 80},
	}
	total, used, err := selectNodeMem(nodes, "pve2")
	if err != nil {
		t.Fatalf("selectNodeMem: %v", err)
	}
	if total != 200 || used != 80 {
		t.Fatalf("want 200/80, got %d/%d", total, used)
	}
	if _, _, err := selectNodeMem(nodes, "ghost"); err == nil {
		t.Fatal("want error for missing node")
	}
}

func TestSumRunningGuestMem(t *testing.T) {
	resources := proxmox.ClusterResources{
		{Type: "qemu", Node: "pve1", Status: "running", MaxMem: 1000},
		{Type: "qemu", Node: "pve1", Status: "running", MaxMem: 2000},
		{Type: "qemu", Node: "pve1", Status: "stopped", MaxMem: 4000}, // not running
		{Type: "qemu", Node: "pve2", Status: "running", MaxMem: 8000}, // other node
		{Type: "lxc", Node: "pve1", Status: "running", MaxMem: 500},   // not qemu
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
		{Type: "lxc", VMID: 112, Status: "running"},    // not qemu
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

func TestBytesToMiB(t *testing.T) {
	if got := bytesToMiB(96 * 1024 * 1024 * 1024); got != 96*1024 {
		t.Fatalf("want 98304 MiB, got %d", got)
	}
}

func TestHostProbeMiBHelpers(t *testing.T) {
	h := &HostProbe{
		HostMemTotalBytes:   96 * 1024 * 1024 * 1024,
		GuestAllocatedBytes: 60 * 1024 * 1024 * 1024,
	}
	if h.HostMemTotalMiB() != 96*1024 {
		t.Errorf("HostMemTotalMiB = %d", h.HostMemTotalMiB())
	}
	if h.GuestAllocatedMiB() != 60*1024 {
		t.Errorf("GuestAllocatedMiB = %d", h.GuestAllocatedMiB())
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

func TestAPIBaseURL(t *testing.T) {
	cases := map[string]string{
		"pve.example.test":          "https://pve.example.test/api2/json",
		"https://pve.example.test/": "https://pve.example.test/api2/json",
		"http://pve:8006":           "http://pve:8006/api2/json",
	}
	for in, want := range cases {
		if got := APIBaseURL(in); got != want {
			t.Errorf("APIBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewProbeClientRequiresCreds(t *testing.T) {
	if _, err := newProbeClient(&ProbeOptions{Endpoint: "https://pve:8006", Node: "pve"}, defaultProbeTimeout); err == nil {
		t.Fatal("want error when neither password nor token is set")
	}
	if _, err := newProbeClient(&ProbeOptions{Endpoint: "https://pve:8006", Node: "pve", APIToken: []byte("no-equals")}, defaultProbeTimeout); err == nil {
		t.Fatal("want error for malformed api token")
	}
	if _, err := newProbeClient(&ProbeOptions{Endpoint: "https://pve:8006", Node: "pve", APIToken: []byte("user@pam!t=secret")}, defaultProbeTimeout); err != nil {
		t.Fatalf("valid token should build client: %v", err)
	}
}

// TestNewProxmoxClientWarnsOnInsecureOnce asserts the connection-time
// warning for insecure: true fires, and only once per process — repeats
// from per-operation client builds would be noise.
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

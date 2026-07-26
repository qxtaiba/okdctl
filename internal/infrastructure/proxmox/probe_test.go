package proxmox

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/luthermonson/go-proxmox"

	"github.com/qxtaiba/okdctl/internal/httputil"
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
		"http://10.0.0.1:8006": "http://10.0.0.1:8006",
	}
	for in, want := range cases {
		if got := normalizeEndpoint(in); got != want {
			t.Errorf("normalizeEndpoint(%q) = %q, want %q", in, got, want)
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

func redirectReq(t *testing.T, host string, withAuth bool) *http.Request {
	t.Helper()
	u, err := url.Parse("https://" + host + "/api2/json")
	if err != nil {
		t.Fatal(err)
	}
	req := &http.Request{URL: u, Header: http.Header{}}
	if withAuth {
		req.Header.Set("Authorization", "PVEAPIToken=user@pam!t=secret")
	}
	return req
}

func redirectVia(t *testing.T, n int, host string) []*http.Request {
	t.Helper()
	via := make([]*http.Request, n)
	for i := range via {
		via[i] = redirectReq(t, host, false)
	}
	return via
}

// TestAPIClientRedirectPolicy pins the redirect policy on the client the
// probe hand-builds: 5-hop cap and cross-host Authorization refusal — the
// load-bearing guard for the credentialed go-proxmox client. Exercised
// through CheckRedirect so a factory swap that drops the policy fails here.
func TestAPIClientRedirectPolicy(t *testing.T) {
	check := newAPIHTTPClient(false, time.Second).CheckRedirect
	if check == nil {
		t.Fatal("CheckRedirect not installed; redirect cap policy is missing")
	}
	if err := check(redirectReq(t, "pve.example", false), redirectVia(t, 1, "pve.example")); err != nil {
		t.Errorf("same-host hop 1 refused: %v", err)
	}
	if err := check(redirectReq(t, "pve.example", false), redirectVia(t, 5, "pve.example")); !errors.Is(err, httputil.ErrTooManyRedirects) {
		t.Errorf("hop 5 = %v; want ErrTooManyRedirects", err)
	}
	if err := check(redirectReq(t, "evil.example", true), redirectVia(t, 1, "pve.example")); !errors.Is(err, httputil.ErrCrossHostAuthHeader) {
		t.Errorf("cross-host with auth = %v; want ErrCrossHostAuthHeader", err)
	}
	if err := check(redirectReq(t, "mirror.example", false), redirectVia(t, 1, "pve.example")); err != nil {
		t.Errorf("cross-host without auth refused: %v", err)
	}
}

func TestNewAPIHTTPClientInstallsRedirectCap(t *testing.T) {
	c := newAPIHTTPClient(true, 3*time.Second)
	if c.CheckRedirect == nil {
		t.Error("CheckRedirect not installed; redirect cap policy is missing")
	}
	if c.Timeout != 3*time.Second {
		t.Errorf("Timeout = %v; want 3s", c.Timeout)
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T; want *http.Transport", c.Transport)
	}
	if !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("insecure=true not carried into TLSClientConfig")
	}
	if newAPIHTTPClient(false, time.Second).Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify {
		t.Error("insecure=false must keep TLS verification on")
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

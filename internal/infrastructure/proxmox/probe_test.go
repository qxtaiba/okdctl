package proxmox

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
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

// probeServerStatus maps a datastore name to the HTTP status its
// /storage/<name>/status endpoint returns; 0 (or absent) serves a healthy
// response, any other code makes that one datastore's read fail.
func newProbeServer(t *testing.T, nodeStatusFails bool, storageStatus map[string]int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api2/json/nodes", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":[{"node":"pve1","maxmem":17179869184}]}`)
	})
	mux.HandleFunc("GET /api2/json/cluster/status", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":[{"type":"cluster","id":"cluster","name":"test","version":1,"quorate":1}]}`)
	})
	mux.HandleFunc("GET /api2/json/cluster/resources", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":[]}`)
	})
	mux.HandleFunc("GET /api2/json/nodes/pve1/status", func(w http.ResponseWriter, _ *http.Request) {
		if nodeStatusFails {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, `{"data":{}}`)
	})
	for name, status := range storageStatus {
		path := "GET /api2/json/nodes/pve1/storage/" + name + "/status"
		code := status
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			if code != 0 {
				w.WriteHeader(code)
				return
			}
			fmt.Fprint(w, `{"data":{"total":1000,"avail":500,"shared":0}}`)
		})
	}
	return httptest.NewServer(mux)
}

func probeOpts(endpoint string, datastores []string) *ProbeOptions {
	return &ProbeOptions{
		Endpoint:   endpoint,
		APIToken:   []byte("user@pam!t=secret"),
		Node:       "pve1",
		Datastores: datastores,
	}
}

func TestProbeHost_DatastoreSuccess(t *testing.T) {
	server := newProbeServer(t, false, map[string]int{"local-lvm": 0})
	defer server.Close()

	probe, err := ProbeHost(t.Context(), probeOpts(server.URL, []string{"local-lvm"}))
	if err != nil {
		t.Fatalf("ProbeHost: %v", err)
	}
	if len(probe.Datastores) != 1 || probe.Datastores[0].Name != "local-lvm" {
		t.Fatalf("Datastores = %+v; want [local-lvm]", probe.Datastores)
	}
	if len(probe.FailedDatastores) != 0 {
		t.Fatalf("FailedDatastores = %v; want empty on full success", probe.FailedDatastores)
	}
}

// TestProbeHost_PartialDatastoreFailureIsVisible is the Item 5 reproduction:
// a failing per-datastore read must be recorded, not silently skipped, so
// an empty Datastores for that name is distinguishable from "no datastore".
func TestProbeHost_PartialDatastoreFailureIsVisible(t *testing.T) {
	server := newProbeServer(t, false, map[string]int{
		"local-lvm": 0,
		"ghost":     http.StatusInternalServerError,
	})
	defer server.Close()

	probe, err := ProbeHost(t.Context(), probeOpts(server.URL, []string{"local-lvm", "ghost"}))
	if err != nil {
		t.Fatalf("ProbeHost: %v", err)
	}
	if len(probe.Datastores) != 1 || probe.Datastores[0].Name != "local-lvm" {
		t.Fatalf("Datastores = %+v; want only [local-lvm]", probe.Datastores)
	}
	if !slices.Contains(probe.FailedDatastores, "ghost") {
		t.Fatalf("FailedDatastores = %v; want it to contain the failed read %q", probe.FailedDatastores, "ghost")
	}
	if slices.Contains(probe.FailedDatastores, "local-lvm") {
		t.Fatalf("FailedDatastores = %v; must not include the successful read", probe.FailedDatastores)
	}
}

func TestProbeHost_NodeLookupFailureFailsAllRequestedDatastores(t *testing.T) {
	server := newProbeServer(t, true, nil)
	defer server.Close()

	probe, err := ProbeHost(t.Context(), probeOpts(server.URL, []string{"local-lvm", "fast-nvme"}))
	if err != nil {
		t.Fatalf("ProbeHost: %v", err)
	}
	if len(probe.Datastores) != 0 {
		t.Fatalf("Datastores = %+v; want none when the node lookup itself failed", probe.Datastores)
	}
	for _, name := range []string{"local-lvm", "fast-nvme"} {
		if !slices.Contains(probe.FailedDatastores, name) {
			t.Errorf("FailedDatastores = %v; want it to contain %q", probe.FailedDatastores, name)
		}
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

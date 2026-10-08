package steps

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luthermonson/go-proxmox"

	"github.com/qxtaiba/okdctl/internal/config"
)

func writeData(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

// newFakeProxmoxServer mocks the go-proxmox endpoints, wrapping responses in
// the {"data": ...} envelope the client expects.
func newFakeProxmoxServer(t *testing.T, targetNode string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api2/json/access/ticket", func(w http.ResponseWriter, _ *http.Request) {
		writeData(w, map[string]any{"ticket": "PVE:test", "CSRFPreventionToken": "tok", "username": "root@pam"})
	})
	mux.HandleFunc("GET /api2/json/nodes", func(w http.ResponseWriter, _ *http.Request) {
		writeData(w, []map[string]any{
			{"node": "pve1", "status": "offline", "maxcpu": 8, "maxmem": 17179869184},
			{"node": "pve2", "status": "online", "maxcpu": 4, "maxmem": 8589934592},
		})
	})
	mux.HandleFunc("GET /api2/json/nodes/"+targetNode+"/status", func(w http.ResponseWriter, _ *http.Request) {
		writeData(w, map[string]any{})
	})
	mux.HandleFunc("GET /api2/json/nodes/"+targetNode+"/storage", func(w http.ResponseWriter, _ *http.Request) {
		writeData(w, []map[string]any{
			{"storage": "local", "type": "dir", "content": "iso,vztmpl", "enabled": 1, "total": 107374182400, "used_fraction": 0.42},
			{"storage": "local-lvm", "type": "lvmthin", "content": "images", "enabled": 1, "total": 214748364800, "used_fraction": 0.1},
			{"storage": "disabled", "type": "dir", "content": "iso", "enabled": 0, "total": 1, "used_fraction": 0},
		})
	})
	mux.HandleFunc("GET /api2/json/nodes/"+targetNode+"/network", func(w http.ResponseWriter, _ *http.Request) {
		writeData(w, []map[string]any{
			{"iface": "vmbr0", "active": 1, "cidr": "192.168.1.1/24"},
		})
	})
	return httptest.NewServer(mux)
}

func testProxmoxConfig(host string) *config.Config {
	var pw config.SecretBytes
	pw.Set("testpass")
	return &config.Config{Provider: config.ProviderConfig{Proxmox: &config.ProxmoxConfig{
		Host:     host,
		Username: "root@pam",
		Password: pw,
	}}}
}

func TestDiscoverProxmox_Success(t *testing.T) {
	server := newFakeProxmoxServer(t, "pve2")
	defer server.Close()

	got, err := discoverProxmox(t.Context(), testProxmoxConfig(server.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Nodes) != 2 {
		t.Fatalf("len(Nodes) = %d; want 2", len(got.Nodes))
	}
	if got.Nodes[0].Name != "pve1" || got.Nodes[0].Status != "offline" {
		t.Errorf("Nodes[0] = %+v", got.Nodes[0])
	}
	if got.Nodes[1].Name != "pve2" || got.Nodes[1].Status != "online" || got.Nodes[1].CPUs != 4 || got.Nodes[1].MemGB != 8 {
		t.Errorf("Nodes[1] = %+v", got.Nodes[1])
	}
	if !got.Nodes[1].StorageKnown || len(got.Nodes[1].Storage) != 2 ||
		got.Nodes[1].Storage[0].Name != "local" || got.Nodes[1].Storage[0].TotalGB != 100 {
		t.Errorf("Nodes[1].Storage = %+v, known=%v", got.Nodes[1].Storage, got.Nodes[1].StorageKnown)
	}
	if !got.Nodes[1].BridgesKnown || len(got.Nodes[1].Bridges) != 1 ||
		got.Nodes[1].Bridges[0].Name != "vmbr0" {
		t.Errorf("Nodes[1].Bridges = %+v, known=%v", got.Nodes[1].Bridges, got.Nodes[1].BridgesKnown)
	}

	if len(got.Storage) != 2 {
		t.Fatalf("len(Storage) = %d; want 2 (disabled storage excluded); got %+v", len(got.Storage), got.Storage)
	}
	if got.Storage[0].Name != "local" || got.Storage[0].TotalGB != 100 {
		t.Errorf("Storage[0] = %+v", got.Storage[0])
	}

	if len(got.Bridges) != 1 || got.Bridges[0].Name != "vmbr0" || got.Bridges[0].CIDR != "192.168.1.1/24" {
		t.Errorf("Bridges = %+v", got.Bridges)
	}
}

// newFakeHeterogeneousServer mocks a two-online-node cluster whose
// inventories differ: pve1 carries an extra storage pool and bridge that
// pve2 lacks.
func newFakeHeterogeneousServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api2/json/access/ticket", func(w http.ResponseWriter, _ *http.Request) {
		writeData(w, map[string]any{"ticket": "PVE:test", "CSRFPreventionToken": "tok", "username": "root@pam"})
	})
	mux.HandleFunc("GET /api2/json/nodes", func(w http.ResponseWriter, _ *http.Request) {
		writeData(w, []map[string]any{
			{"node": "pve1", "status": "online", "maxcpu": 8, "maxmem": 17179869184},
			{"node": "pve2", "status": "online", "maxcpu": 4, "maxmem": 8589934592},
		})
	})
	for _, n := range []string{"pve1", "pve2"} {
		node := n
		mux.HandleFunc("GET /api2/json/nodes/"+node+"/status", func(w http.ResponseWriter, _ *http.Request) {
			writeData(w, map[string]any{})
		})
		mux.HandleFunc("GET /api2/json/nodes/"+node+"/storage", func(w http.ResponseWriter, _ *http.Request) {
			stores := []map[string]any{
				{"storage": "local", "type": "dir", "content": "iso,vztmpl", "enabled": 1, "total": 107374182400},
				{"storage": "local-lvm", "type": "lvmthin", "content": "images", "enabled": 1, "total": 214748364800},
			}
			if node == "pve1" {
				stores = append(stores, map[string]any{"storage": "tank", "type": "zfspool", "content": "images", "enabled": 1, "total": 4294967296000})
			}
			writeData(w, stores)
		})
		mux.HandleFunc("GET /api2/json/nodes/"+node+"/network", func(w http.ResponseWriter, _ *http.Request) {
			bridges := []map[string]any{{"iface": "vmbr0", "active": 1, "cidr": "192.168.1.1/24"}}
			if node == "pve1" {
				bridges = append(bridges, map[string]any{"iface": "vmbr1", "active": 1, "cidr": "10.10.0.1/24"})
			}
			writeData(w, bridges)
		})
	}
	return httptest.NewServer(mux)
}

// TestDiscoverProxmox_IntersectsAcrossOnlineNodes pins bug 10: discovery
// reads every online node, offers only the storage/bridges common to
// all of them, and flags the cluster heterogeneous so the wizard can warn.
func TestDiscoverProxmox_IntersectsAcrossOnlineNodes(t *testing.T) {
	server := newFakeHeterogeneousServer(t)
	defer server.Close()

	got, err := discoverProxmox(t.Context(), testProxmoxConfig(server.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Storage) != 2 || got.Storage[0].Name != "local" || got.Storage[1].Name != "local-lvm" {
		t.Errorf("Storage = %+v; want the two pools common to both nodes", got.Storage)
	}
	if len(got.Bridges) != 1 || got.Bridges[0].Name != "vmbr0" {
		t.Errorf("Bridges = %+v; want only vmbr0", got.Bridges)
	}
	if !got.Heterogeneous {
		t.Error("Heterogeneous = false, want true for differing inventories")
	}
}

// TestDiscoverProxmox_FlagsPlacementUnreachableStorage exercises the
// placement-validation half of the discovery fix: a config that already
// targets worker_nodes on a node lacking the chosen storage must surface
// that mismatch from discovery, not silently accept it — "tank" exists
// only on pve1 in newFakeHeterogeneousServer's fixture, so pinning the
// worker to pve2 while storage="tank" must fail.
func TestDiscoverProxmox_FlagsPlacementUnreachableStorage(t *testing.T) {
	server := newFakeHeterogeneousServer(t)
	defer server.Close()

	cfg := testProxmoxConfig(server.URL)
	cfg.Provider.Proxmox.Storage = "tank"
	cfg.Provider.Proxmox.WorkerNodes = []string{"pve2"}
	cfg.Topology.Workers.Count = 1

	_, err := discoverProxmox(t.Context(), cfg)
	if err == nil || !strings.Contains(err.Error(), "tank") {
		t.Fatalf("err = %v; want a validation error naming storage %q as unreachable on pve2", err, "tank")
	}
}

func TestDiscoverProxmox_SingleOnlineNodeIsHomogeneous(t *testing.T) {
	server := newFakeProxmoxServer(t, "pve2")
	defer server.Close()

	got, err := discoverProxmox(t.Context(), testProxmoxConfig(server.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Heterogeneous {
		t.Error("Heterogeneous = true for a single online node, want false")
	}
}

func TestDiscoverProxmox_NodesEndpointFailures(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{"no nodes found", func(w http.ResponseWriter, _ *http.Request) {
			writeData(w, []map[string]any{})
		}, "no nodes found"},
		// want "connection failed": the classifyError default branch.
		{"nodes request fails", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}, "connection failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api2/json/access/ticket", func(w http.ResponseWriter, _ *http.Request) {
				writeData(w, map[string]any{"ticket": "PVE:test", "CSRFPreventionToken": "tok", "username": "root@pam"})
			})
			mux.HandleFunc("GET /api2/json/nodes", tc.handler)
			server := httptest.NewServer(mux)
			defer server.Close()

			_, err := discoverProxmox(t.Context(), testProxmoxConfig(server.URL))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v; want substring %q", err, tc.want)
			}
		})
	}
}

func TestDiscoverProxmox_ValidationBranches(t *testing.T) {
	var pw config.SecretBytes
	pw.Set("x")
	cases := []struct {
		name string
		cfg  *config.Config
		want string
	}{
		{"nil proxmox config", &config.Config{}, "no proxmox config"},
		{"missing host", &config.Config{Provider: config.ProviderConfig{Proxmox: &config.ProxmoxConfig{Username: "root"}}}, "missing credentials"},
		{"token id without password", &config.Config{Provider: config.ProviderConfig{Proxmox: &config.ProxmoxConfig{Host: "pve", Username: "root", TokenID: "tok"}}}, "discovery uses password auth"},
		{"missing password", &config.Config{Provider: config.ProviderConfig{Proxmox: &config.ProxmoxConfig{Host: "pve", Username: "root"}}}, "missing credentials — enter host, username, and password"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := discoverProxmox(t.Context(), tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v; want substring %q", err, tc.want)
			}
		})
	}
}

func TestFetchNodeDetails_PartialFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api2/json/nodes/pve1/status", func(w http.ResponseWriter, _ *http.Request) {
		writeData(w, map[string]any{})
	})
	mux.HandleFunc("GET /api2/json/nodes/pve1/storage", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("GET /api2/json/nodes/pve1/network", func(w http.ResponseWriter, _ *http.Request) {
		writeData(w, []map[string]any{{"iface": "vmbr0", "active": 1, "cidr": "10.0.0.1/24"}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := proxmox.NewClient(server.URL + "/api2/json")
	details := fetchNodeDetails(context.Background(), client, "pve1")

	if details.StorageKnown || details.Storage != nil {
		t.Errorf("storage = %+v known=%v; want unknown after storage endpoint 500", details.Storage, details.StorageKnown)
	}
	if !details.BridgesKnown || len(details.Bridges) != 1 || details.Bridges[0].Name != "vmbr0" {
		t.Errorf("bridges = %+v known=%v; want [{vmbr0 ...}] despite storage failure", details.Bridges, details.BridgesKnown)
	}
}

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		// Untyped tls/x509 failures the typed checks below don't catch
		// (e.g. a protocol-level handshake failure unrelated to any
		// certificate) get an honest generic message, not a guessed cause.
		{"tls certificate error", errors.New("x509: certificate signed by unknown authority"), "tls handshake failed"},
		{"tls lowercase prefix", errors.New("tls: handshake failure"), "tls handshake failed"},
		{"connection refused", errors.New("dial tcp 10.0.0.1:8006: connect: connection refused"), "connection refused"},
		{"no such host", errors.New("dial tcp: lookup pve.invalid: no such host"), "host not found"},
		{"io timeout", errors.New("dial tcp 10.0.0.1:8006: i/o timeout"), "connection timed out"},
		{"context deadline", fmt.Errorf("get: %w", context.DeadlineExceeded), "connection timed out"},
		{"status 401", errors.New("status 401 unauthorized"), "authentication failed"},
		{"authentication failure literal", errors.New("authentication failure"), "authentication failed"},
		{"unmapped error passthrough", errors.New("some other transport error"), "connection failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyError(tc.err)
			if got == nil || !strings.Contains(got.Error(), tc.want) {
				t.Errorf("classifyError(%v) = %v; want substring %q", tc.err, got, tc.want)
			}
			if strings.Contains(got.Error(), "skip tls verify") {
				t.Errorf("classifyError(%v) = %v; must not nudge toward disabling tls verification", tc.err, got)
			}
		})
	}
}

// TestClassifyError_TLSCausesGetDistinctSafeRemedies pins the three
// distinguishable TLS failure causes, each wrapped the way crypto/tls
// actually returns it (tls.CertificateVerificationError wrapping the real
// x509 cause): every remedy must be specific to the real cause, safe (never
// "disable verification"), and must not swallow the underlying error.
func TestClassifyError_TLSCausesGetDistinctSafeRemedies(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "untrusted issuer",
			err:  &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}},
			want: "ca certificate",
		},
		{
			name: "hostname mismatch",
			err:  &tls.CertificateVerificationError{Err: x509.HostnameError{Certificate: &x509.Certificate{}, Host: "10.0.0.5"}},
			want: "name on the certificate",
		},
		{
			name: "certificate expired",
			err:  &tls.CertificateVerificationError{Err: x509.CertificateInvalidError{Reason: x509.Expired}},
			want: "renew",
		},
		{
			name: "certificate invalid, other reason: honest generic, no guess",
			err:  &tls.CertificateVerificationError{Err: x509.CertificateInvalidError{Reason: x509.NotAuthorizedToSign}},
			want: "tls certificate invalid",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyError(tc.err)
			if got == nil || !strings.Contains(got.Error(), tc.want) {
				t.Errorf("classifyError(%v) = %v; want substring %q", tc.err, got, tc.want)
			}
			if strings.Contains(got.Error(), "skip tls verify") {
				t.Errorf("classifyError(%v) = %v; must not nudge toward disabling tls verification", tc.err, got)
			}
			if !errors.Is(got, tc.err) {
				t.Errorf("classifyError(%v) = %v; lost the underlying error (not wrapped with %%w)", tc.err, got)
			}
		})
	}
}

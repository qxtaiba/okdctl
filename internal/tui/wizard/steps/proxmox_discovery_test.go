package steps

import (
	"context"
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

// newFakeProxmoxServer answers the small slice of Proxmox REST endpoints
// discoverProxmox and fetchNodeDetails exercise, matching the go-proxmox
// client's request paths and {"data": ...} envelope. targetNode is
// whichever node discoverProxmox is expected to pick (first node
// reporting "online", else nodes[0]).
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
	mux.HandleFunc("GET /api2/json/nodes/"+targetNode+"/storage/local/status", func(w http.ResponseWriter, _ *http.Request) {
		writeData(w, map[string]any{})
	})
	mux.HandleFunc("GET /api2/json/nodes/"+targetNode+"/storage/local/content", func(w http.ResponseWriter, _ *http.Request) {
		writeData(w, []map[string]any{
			{"volid": "local:iso/fcos-38.iso"},
			{"volid": "local:iso/notes.txt"},
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

	got, err := discoverProxmox(testProxmoxConfig(server.URL))
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

	if len(got.Storage) != 2 {
		t.Fatalf("len(Storage) = %d; want 2 (disabled storage excluded); got %+v", len(got.Storage), got.Storage)
	}
	if got.Storage[0].Name != "local" || got.Storage[0].TotalGB != 100 {
		t.Errorf("Storage[0] = %+v", got.Storage[0])
	}

	if len(got.Bridges) != 1 || got.Bridges[0].Name != "vmbr0" || got.Bridges[0].CIDR != "192.168.1.1/24" {
		t.Errorf("Bridges = %+v", got.Bridges)
	}

	if len(got.ISOs) != 1 || got.ISOs[0] != "local:iso/fcos-38.iso" {
		t.Errorf("ISOs = %+v; want [local:iso/fcos-38.iso]", got.ISOs)
	}
}

func TestDiscoverProxmox_NoNodesFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api2/json/access/ticket", func(w http.ResponseWriter, _ *http.Request) {
		writeData(w, map[string]any{"ticket": "PVE:test", "CSRFPreventionToken": "tok", "username": "root@pam"})
	})
	mux.HandleFunc("GET /api2/json/nodes", func(w http.ResponseWriter, _ *http.Request) {
		writeData(w, []map[string]any{})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := discoverProxmox(testProxmoxConfig(server.URL))
	if err == nil || !strings.Contains(err.Error(), "no nodes found") {
		t.Fatalf("err = %v; want substring \"no nodes found\"", err)
	}
}

func TestDiscoverProxmox_NodesRequestFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api2/json/access/ticket", func(w http.ResponseWriter, _ *http.Request) {
		writeData(w, map[string]any{"ticket": "PVE:test", "CSRFPreventionToken": "tok", "username": "root@pam"})
	})
	mux.HandleFunc("GET /api2/json/nodes", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := discoverProxmox(testProxmoxConfig(server.URL))
	if err == nil || !strings.Contains(err.Error(), "connection failed") {
		t.Fatalf("err = %v; want substring \"connection failed\" (classifyError default branch)", err)
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
		{"missing username", &config.Config{Provider: config.ProviderConfig{Proxmox: &config.ProxmoxConfig{Host: "pve"}}}, "missing credentials"},
		{"token id without password", &config.Config{Provider: config.ProviderConfig{Proxmox: &config.ProxmoxConfig{Host: "pve", Username: "root", TokenID: "tok"}}}, "discovery uses password auth"},
		{"missing password", &config.Config{Provider: config.ProviderConfig{Proxmox: &config.ProxmoxConfig{Host: "pve", Username: "root"}}}, "missing credentials — enter host, username, and password"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := discoverProxmox(tc.cfg)
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
	storage, bridges, isos := fetchNodeDetails(context.Background(), client, "pve1")

	if storage != nil {
		t.Errorf("storage = %+v; want nil after storage endpoint 500", storage)
	}
	if len(bridges) != 1 || bridges[0].Name != "vmbr0" {
		t.Errorf("bridges = %+v; want [{vmbr0 ...}] despite storage failure", bridges)
	}
	if isos != nil {
		t.Errorf("isos = %+v; want nil (no iso-tagged storage)", isos)
	}
}

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"tls certificate error", errors.New("x509: certificate signed by unknown authority"), "tls certificate verification failed"},
		{"tls lowercase prefix", errors.New("tls: handshake failure"), "tls certificate verification failed"},
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
		})
	}
}

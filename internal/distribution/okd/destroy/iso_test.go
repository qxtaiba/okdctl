package destroy

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

const (
	isoToken = "root@pam!okdctl=secret"
	isoUPID  = "UPID:pve:00001234:00005678:66aabbcc:imgdel::root@pam!okdctl:"
)

type fakeISOCleanup struct {
	mu      sync.Mutex
	content map[string]string
	vms     string
	configs map[string]string
	deletes []string
}

func (f *fakeISOCleanup) serve(t *testing.T) *credentials.ProxmoxCredentials {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api2/json/nodes/pve/storage/{storage}/status", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":{"type":"dir","content":"iso","active":1,"enabled":1}}`)
	})
	mux.HandleFunc("GET /api2/json/nodes/pve/storage/{storage}/content", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"data":%s}`, f.content[r.PathValue("storage")])
	})
	mux.HandleFunc("GET /api2/json/cluster/resources", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"data":%s}`, f.vms)
	})
	mux.HandleFunc("GET /api2/json/nodes/pve/qemu/{vmid}/config", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"data":%s}`, f.configs[r.PathValue("vmid")])
	})
	mux.HandleFunc("DELETE /api2/json/nodes/pve/storage/{storage}/content/{volid...}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.deletes = append(f.deletes, r.PathValue("volid"))
		f.mu.Unlock()
		fmt.Fprintf(w, `{"data":%q}`, isoUPID)
	})
	mux.HandleFunc("GET /api2/json/nodes/pve/tasks/{upid}/status", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"data":{"upid":%q,"node":"pve","status":"stopped","exitstatus":"OK"}}`, isoUPID)
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "PVEAPIToken="+isoToken {
			t.Errorf("Authorization = %q; want the api token", got)
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return &credentials.ProxmoxCredentials{Endpoint: srv.URL, APIToken: []byte(isoToken)}
}

func isoCleanupConfig() *config.Config {
	cfg := destroyableConfig()
	cfg.Provider.Proxmox.ISOStorage = "nfs-iso"
	cfg.Topology.ControlPlane.Count = 1
	return cfg
}

func TestRemoveRemoteISOs(t *testing.T) {
	f := &fakeISOCleanup{
		content: map[string]string{
			"local": `[
				{"volid":"local:iso/fedora-coreos-40.20240416.3.1-live.x86_64.iso","size":1},
				{"volid":"local:iso/debian-12.iso","size":1},
				{"volid":"local:iso/master0.iso","size":1}
			]`,
			"nfs-iso": `[
				{"volid":"nfs-iso:iso/bootstrap.iso","size":1},
				{"volid":"nfs-iso:iso/master0.iso","size":1},
				{"volid":"nfs-iso:iso/master1.iso","size":1},
				{"volid":"nfs-iso:iso/fedora-coreos-40.20240416.3.1-live.x86_64.iso","size":1}
			]`,
		},
		vms:     `[{"vmid":900,"type":"qemu","node":"pve","status":"running"}]`,
		configs: map[string]string{"900": `{"ide2":"nfs-iso:iso/bootstrap.iso,media=cdrom"}`},
	}
	p := New(phase.WithLogger(logutil.NopLogger), phase.WithProxmoxCredentials(f.serve(t)))

	if err := p.removeRemoteISOs(t.Context(), isoCleanupConfig()); err != nil {
		t.Fatalf("removeRemoteISOs: %v", err)
	}
	want := []string{
		"local:iso/fedora-coreos-40.20240416.3.1-live.x86_64.iso",
		"nfs-iso:iso/master0.iso",
	}
	if !slices.Equal(f.deletes, want) {
		t.Errorf("deleted volumes = %v; want %v", f.deletes, want)
	}
}

func TestRemoveRemoteISOsRequiresCredentials(t *testing.T) {
	p := New(phase.WithLogger(logutil.NopLogger))
	var cfgErr *errtypes.ConfigError
	if err := p.removeRemoteISOs(t.Context(), isoCleanupConfig()); !errors.As(err, &cfgErr) {
		t.Fatalf("err = %v; want a ConfigError for missing credentials", err)
	}
}

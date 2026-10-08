package provision

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

type fakePlacement struct {
	mu       sync.Mutex
	status   map[string]string
	content  map[string]string
	perms    string
	requests []string
}

func (f *fakePlacement) log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

func (f *fakePlacement) serve(t *testing.T) *credentials.ProxmoxCredentials {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api2/json/nodes/{node}/storage/shared/status", func(w http.ResponseWriter, r *http.Request) {
		body, ok := f.status[r.PathValue("node")]
		if !ok {
			http.Error(w, "storage 'shared' is not available on node", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, `{"data":%s}`, body)
	})
	mux.HandleFunc("GET /api2/json/nodes/{node}/storage/shared/content", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"data":%s}`, f.content[r.PathValue("node")])
	})
	mux.HandleFunc("GET /api2/json/access/permissions", func(w http.ResponseWriter, r *http.Request) {
		if f.perms == "" {
			http.Error(w, "permission listing unavailable", http.StatusInternalServerError)
			return
		}
		if got := r.URL.Query().Get("path"); got != "/storage/shared" {
			http.Error(w, "unexpected path "+got, http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, `{"data":%s}`, f.perms)
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		f.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return &credentials.ProxmoxCredentials{Endpoint: srv.URL, APIToken: []byte(pveToken)}
}

func multiHostConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = "pve1"
	cfg.Provider.Proxmox.ISOStorage = "shared"
	cfg.Provider.Proxmox.WorkerNodes = []string{"pve2"}
	return cfg
}

const nfsActive = `{"type":"nfs","content":"iso,images","active":1,"enabled":1,"shared":1}`

func TestValidateISOPlacement(t *testing.T) {
	cases := []struct {
		name    string
		status  map[string]string
		wantErr string
	}{
		{name: "nfs active on every node", status: map[string]string{"pve1": nfsActive, "pve2": nfsActive}},
		{
			name:    "shared directory storage",
			status:  map[string]string{"pve1": `{"type":"dir","content":"iso","active":1,"enabled":1,"shared":1}`, "pve2": nfsActive},
			wantErr: "requires nfs, cifs, or cephfs",
		},
		{
			name:    "iso content disabled",
			status:  map[string]string{"pve1": `{"type":"nfs","content":"images","active":1,"enabled":1}`, "pve2": nfsActive},
			wantErr: "ISO content enabled",
		},
		{
			name:    "inactive on a destination",
			status:  map[string]string{"pve1": nfsActive, "pve2": `{"type":"nfs","content":"iso","active":0,"enabled":1}`},
			wantErr: "unavailable on pve2",
		},
		{
			name:    "missing on a destination",
			status:  map[string]string{"pve1": nfsActive},
			wantErr: "pve2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakePlacement{status: tc.status}
			p := New(phase.WithExecutor(newUploadExecutor()), phase.WithProxmoxCredentials(f.serve(t)))
			err := p.validateISOPlacement(t.Context(), multiHostConfig())
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateISOPlacement: %v", err)
				}
				for _, node := range []string{"pve1", "pve2"} {
					if want := "GET /api2/json/nodes/" + node + "/storage/shared/status"; !slices.Contains(f.log(), want) {
						t.Errorf("requests = %v; want %s", f.log(), want)
					}
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v; want %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateISOStorageSingleHostWithoutCredentials(t *testing.T) {
	cfg := multiHostConfig()
	cfg.Provider.Proxmox.WorkerNodes = nil
	p := New(phase.WithExecutor(newUploadExecutor()))
	if err := p.ValidateISOStorage(t.Context(), cfg); err != nil {
		t.Fatalf("ValidateISOStorage: %v", err)
	}
}

func TestVerifySharedISOsRejectsMissingDestination(t *testing.T) {
	f := &fakePlacement{content: map[string]string{
		"pve1": `[{"volid":"shared:iso/worker0.iso","size":1}]`,
		"pve2": `[]`,
	}, status: map[string]string{"pve1": nfsActive, "pve2": nfsActive}}
	p := New(phase.WithExecutor(newUploadExecutor()), phase.WithProxmoxCredentials(f.serve(t)))
	err := p.verifySharedISOs(t.Context(), multiHostConfig(), []string{"/work/custom-isos/worker0.iso"})
	if err == nil || !strings.Contains(err.Error(), "shared:iso/worker0.iso is not visible on pve2") {
		t.Fatalf("err = %v; want worker0.iso reported missing on pve2", err)
	}
}

func TestVerifySharedISOsAcceptsEveryDestination(t *testing.T) {
	visible := `[{"volid":"shared:iso/worker0.iso","size":1}]`
	f := &fakePlacement{content: map[string]string{"pve1": visible, "pve2": visible}, status: map[string]string{"pve1": nfsActive, "pve2": nfsActive}}
	p := New(phase.WithExecutor(newUploadExecutor()), phase.WithProxmoxCredentials(f.serve(t)))
	if err := p.verifySharedISOs(t.Context(), multiHostConfig(), []string{"/work/custom-isos/worker0.iso"}); err != nil {
		t.Fatalf("verifySharedISOs: %v", err)
	}
}

func TestValidateISOStoragePrivileges(t *testing.T) {
	cases := []struct {
		name    string
		perms   string
		wantErr string
	}{
		{name: "upload and delete rights", perms: `{"/storage/shared":{"Datastore.AllocateTemplate":1,"Datastore.Audit":1,"Datastore.Allocate":1}}`},
		{name: "allocate space instead of audit", perms: `{"/storage/shared":{"Datastore.AllocateTemplate":1,"Datastore.AllocateSpace":1}}`},
		{name: "no delete right only warns", perms: `{"/storage/shared":{"Datastore.AllocateTemplate":1,"Datastore.Audit":1}}`},
		{name: "listing unavailable defers to upload"},
		{
			name:    "no upload right",
			perms:   `{"/storage/shared":{"Datastore.Audit":1,"Datastore.Allocate":1}}`,
			wantErr: "lack Datastore.AllocateTemplate on /storage/shared",
		},
		{
			name:    "nothing on the storage",
			perms:   `{}`,
			wantErr: "lack Datastore.AllocateTemplate, Datastore.Audit on /storage/shared",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakePlacement{perms: tc.perms}
			p := New(phase.WithExecutor(newUploadExecutor()), phase.WithProxmoxCredentials(f.serve(t)))
			cfg := multiHostConfig()
			cfg.Provider.Proxmox.WorkerNodes = nil
			err := p.ValidateISOStorage(t.Context(), cfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateISOStorage: %v", err)
				}
				return
			}
			var cfgErr *errtypes.ConfigError
			if !errors.As(err, &cfgErr) || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v; want a ConfigError containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateISOStorageChecksPlacementAfterPrivileges(t *testing.T) {
	f := &fakePlacement{
		perms:  `{"/storage/shared":{"Datastore.AllocateTemplate":1,"Datastore.Audit":1,"Datastore.Allocate":1}}`,
		status: map[string]string{"pve1": nfsActive},
	}
	p := New(phase.WithExecutor(newUploadExecutor()), phase.WithProxmoxCredentials(f.serve(t)))
	if err := p.ValidateISOStorage(t.Context(), multiHostConfig()); err == nil || !strings.Contains(err.Error(), "pve2") {
		t.Fatalf("err = %v; want the storage missing on pve2 reported", err)
	}
}

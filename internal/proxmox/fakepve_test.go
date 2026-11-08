package proxmox

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var fakeFixtures = map[string]string{
	"/version":                           "version.json",
	"/nodes":                             "nodes.json",
	"/nodes/pve/status":                  "nodes_pve_status.json",
	"/nodes/pve/storage":                 "nodes_pve_storage.json",
	"/nodes/pve/network?type=any_bridge": "nodes_pve_network_any_bridge.json",
	"/cluster/resources?type=vm":         "cluster_resources_vm.json",
	"/access/permissions":                "access_permissions.json",
}

type fakeAPI struct {
	srv       *httptest.Server
	overrides map[string]http.HandlerFunc

	mu    sync.Mutex
	calls []string
}

func newFakeAPI(t *testing.T, overrides map[string]http.HandlerFunc) *fakeAPI {
	t.Helper()
	f := &fakeAPI{overrides: overrides}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "PVEAPIToken="+fakeToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/api2/json")
	if r.URL.RawQuery != "" {
		key += "?" + r.URL.RawQuery
	}
	f.mu.Lock()
	f.calls = append(f.calls, r.Method+" "+key)
	f.mu.Unlock()

	if h, ok := f.overrides[key]; ok {
		h(w, r)
		return
	}
	name, ok := fakeFixtures[key]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"data":null}`))
		return
	}
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (f *fakeAPI) called(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c == key {
			return true
		}
	}
	return false
}

func (f *fakeAPI) driver(t *testing.T) Driver {
	t.Helper()
	d, err := New(f.srv.URL, []byte(fakeToken), false)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	return d
}

func jsonHandler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

package proxmox

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	fakeToken = "root@pam!tok=secret"

	upidCreate   = "UPID:pve:00001234:00005678:66aabbcc:qmcreate:105:root@pam!tok:"
	upidConfig   = "UPID:pve:00001234:00005678:66aabbcc:qmconfig:101:root@pam!tok:"
	upidResize   = "UPID:pve:00001234:00005678:66aabbcc:qmresize:101:root@pam!tok:"
	upidDestroy  = "UPID:pve:00001234:00005678:66aabbcc:qmdestroy:101:root@pam!tok:"
	upidStart    = "UPID:pve:00001234:00005678:66aabbcc:qmstart:101:root@pam!tok:"
	upidShutdown = "UPID:pve:00001234:00005678:66aabbcc:qmshutdown:101:root@pam!tok:"
	upidStop     = "UPID:pve:00001234:00005678:66aabbcc:qmstop:101:root@pam!tok:"
	upidReset    = "UPID:pve:00001234:00005678:66aabbcc:qmreset:101:root@pam!tok:"
	upidUpload   = "UPID:pve:00001234:00005678:66aabbcc:imgcopy::root@pam!tok:"
	upidImgDel   = "UPID:pve:00001234:00005678:66aabbcc:imgdel:grappleberry-worker.iso@local:root@pam!tok:"
)

var fakeFixtures = map[string]string{
	"GET /version":                                     "version.json",
	"GET /nodes":                                       "nodes.json",
	"GET /nodes/pve/status":                            "nodes_pve_status.json",
	"GET /nodes/pve/storage":                           "nodes_pve_storage.json",
	"GET /nodes/pve/network?type=any_bridge":           "nodes_pve_network_any_bridge.json",
	"GET /cluster/resources?type=vm":                   "cluster_resources_vm.json",
	"GET /access/permissions":                          "access_permissions.json",
	"GET /nodes/pve/qemu/101/status/current":           "nodes_pve_qemu_101_status_current.json",
	"GET /nodes/pve/qemu/101/config":                   "nodes_pve_qemu_101_config.json",
	"GET /nodes/pve/qemu/101/pending":                  "nodes_pve_qemu_101_pending.json",
	"GET /nodes/pve/tasks/" + upidStart + "/status":    "nodes_pve_tasks_qmstart_status.json",
	"GET /cluster/ha/resources/vm:101":                 "cluster_ha_resources_vm_101.json",
	"GET /nodes/pve/storage/local/content?content=iso": "nodes_pve_storage_local_content_iso.json",
}

// fakeWrites maps a write call to the UPID it returns; "" answers {"data":null}.
var fakeWrites = map[string]string{
	"POST /nodes/pve/qemu":                                                      upidCreate,
	"POST /nodes/pve/qemu/101/config":                                           upidConfig,
	"PUT /nodes/pve/qemu/101/resize":                                            upidResize,
	"DELETE /nodes/pve/qemu/101?purge=1&destroy-unreferenced-disks=1":           upidDestroy,
	"POST /nodes/pve/qemu/101/status/start":                                     upidStart,
	"POST /nodes/pve/qemu/101/status/shutdown":                                  upidShutdown,
	"POST /nodes/pve/qemu/101/status/stop":                                      upidStop,
	"POST /nodes/pve/qemu/101/status/reset":                                     upidReset,
	"POST /cluster/ha/resources":                                                "",
	"PUT /cluster/ha/resources/vm:101":                                          "",
	"DELETE /cluster/ha/resources/vm:101?purge=1":                               "",
	"POST /nodes/pve/storage/local/upload":                                      upidUpload,
	"DELETE /nodes/pve/storage/local/content/local:iso/grappleberry-worker.iso": upidImgDel,
}

type fakeCall struct {
	Method        string
	Key           string
	Form          url.Values
	ContentType   string
	ContentLength int64
	Body          []byte
}

type fakeAPI struct {
	t         *testing.T
	srv       *httptest.Server
	overrides map[string]http.HandlerFunc

	mu    sync.Mutex
	calls []fakeCall
}

// newFakeAPI serves the fixtures; overrides are keyed "METHOD /path[?query]"
// or, for any method, "/path[?query]". Unmapped paths answer 500.
func newFakeAPI(t *testing.T, overrides map[string]http.HandlerFunc) *fakeAPI {
	t.Helper()
	f := &fakeAPI{t: t, overrides: overrides}
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
	f.record(r, key)

	if h, ok := f.overrides[r.Method+" "+key]; ok {
		h(w, r)
		return
	}
	if h, ok := f.overrides[key]; ok {
		h(w, r)
		return
	}
	if name, ok := fakeFixtures[r.Method+" "+key]; ok {
		body, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			f.t.Errorf("fixture %s: %v", name, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
		return
	}
	if upid, ok := fakeWrites[r.Method+" "+key]; ok {
		if upid == "" {
			jsonHandler(http.StatusOK, `{"data":null}`)(w, r)
			return
		}
		jsonHandler(http.StatusOK, `{"data":"`+upid+`"}`)(w, r)
		return
	}
	f.t.Errorf("unmapped request %s %s", r.Method, key)
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte(`{"data":null,"message":"unmapped path"}`))
}

func (f *fakeAPI) record(r *http.Request, key string) {
	call := fakeCall{Method: r.Method, Key: key, ContentType: r.Header.Get("Content-Type"), ContentLength: r.ContentLength}
	switch {
	case strings.HasPrefix(call.ContentType, "multipart/form-data"):
		body, err := io.ReadAll(r.Body)
		if err != nil {
			f.t.Errorf("%s %s: read multipart body: %v", r.Method, key, err)
		}
		call.Body = body
	case r.Body != nil && r.ContentLength != 0:
		var fields map[string]string
		if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
			f.t.Errorf("%s %s: body is not a flat JSON object: %v", r.Method, key, err)
		}
		call.Form = url.Values{}
		for k, v := range fields {
			call.Form.Set(k, v)
		}
	}
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
}

func (f *fakeAPI) called(key string) bool {
	return f.count(key) > 0
}

func (f *fakeAPI) count(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.Method+" "+c.Key == key {
			n++
		}
	}
	return n
}

// form returns the body of the last call matching "METHOD key".
func (f *fakeAPI) form(key string) url.Values {
	f.t.Helper()
	return f.last(key).Form
}

// multipartBody parses the last multipart call matching "METHOD key".
func (f *fakeAPI) multipartBody(key string) (fakeCall, *multipart.Reader) {
	f.t.Helper()
	call := f.last(key)
	_, params, err := mime.ParseMediaType(call.ContentType)
	if err != nil || params["boundary"] == "" {
		f.t.Fatalf("%s content type %q is not multipart: %v", key, call.ContentType, err)
	}
	return call, multipart.NewReader(bytes.NewReader(call.Body), params["boundary"])
}

func (f *fakeAPI) last(key string) fakeCall {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if c := f.calls[i]; c.Method+" "+c.Key == key {
			return c
		}
	}
	f.t.Fatalf("no call %s recorded; calls: %+v", key, f.calls)
	return fakeCall{}
}

func (f *fakeAPI) driver(t *testing.T) Driver {
	t.Helper()
	d, err := New(f.srv.URL, []byte(fakeToken), false)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	return d
}

// pveErrorHandler answers the way pveproxy reports a die(): HTTP 500 with the
// message in the body (the reason phrase, which pveproxy also sets, cannot be
// written through net/http).
func pveErrorHandler(msg string) http.HandlerFunc {
	body, _ := json.Marshal(map[string]any{"data": nil, "message": msg + "\n"})
	return jsonHandler(http.StatusInternalServerError, string(body))
}

func jsonHandler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

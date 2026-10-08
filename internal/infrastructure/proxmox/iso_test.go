package proxmox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/credentials"
)

const isoTaskUPID = "UPID:pve1:00001234:00005678:66aabbcc:imgcopy::root@pam!tok:"

type isoUpload struct {
	fields   map[string]string
	filename string
	body     []byte
}

type fakeISOPVE struct {
	mu          sync.Mutex
	requests    []string
	uploads     []isoUpload
	content     map[string]string
	status      map[string]string
	taskExit    string
	taskHangs   bool
	uploadHangs bool
}

func (f *fakeISOPVE) log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

func (f *fakeISOPVE) start(t *testing.T) *ISOStore {
	t.Helper()
	released := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api2/json/nodes/{node}/storage/local/status", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		body, ok := f.status[r.PathValue("node")]
		f.mu.Unlock()
		if !ok {
			body = `{"type":"dir","content":"iso,vztmpl","active":1,"enabled":1}`
		}
		fmt.Fprintf(w, `{"data":%s}`, body)
	})
	mux.HandleFunc("GET /api2/json/nodes/{node}/storage/local/content", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		body, ok := f.content[r.PathValue("node")]
		f.mu.Unlock()
		if !ok {
			body = "[]"
		}
		fmt.Fprintf(w, `{"data":%s}`, body)
	})
	mux.HandleFunc("POST /api2/json/nodes/pve1/storage/local/upload", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		hangs := f.uploadHangs
		f.mu.Unlock()
		if hangs {
			_, _ = io.Copy(io.Discard, r.Body)
			select {
			case <-r.Context().Done():
			case <-released:
			}
			return
		}
		fields, filename, body, err := readUploadForm(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		up := isoUpload{fields: fields, filename: filename, body: body}
		f.mu.Lock()
		f.uploads = append(f.uploads, up)
		f.mu.Unlock()
		fmt.Fprintf(w, `{"data":%q}`, isoTaskUPID)
	})
	mux.HandleFunc("GET /api2/json/nodes/pve1/tasks/{upid}/status", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		exit, hangs := f.taskExit, f.taskHangs
		f.mu.Unlock()
		if hangs {
			fmt.Fprintf(w, `{"data":{"upid":%q,"node":"pve1","status":"running"}}`, isoTaskUPID)
			return
		}
		if exit == "" {
			exit = "OK"
		}
		fmt.Fprintf(w, `{"data":{"upid":%q,"node":"pve1","status":"stopped","exitstatus":%q}}`, isoTaskUPID, exit)
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "PVEAPIToken="+fakeToken {
			t.Errorf("Authorization = %q; want token auth on every request", got)
		}
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		f.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(released) })

	return NewISOStore(&credentials.ProxmoxCredentials{Endpoint: srv.URL, APIToken: []byte(fakeToken)}, "local")
}

func readUploadForm(r *http.Request) (fields map[string]string, filename string, body []byte, err error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, "", nil, err
	}
	fields = map[string]string{}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return fields, filename, body, nil
		}
		if err != nil {
			return nil, "", nil, err
		}
		data, err := io.ReadAll(io.LimitReader(part, 1<<20))
		if err != nil {
			return nil, "", nil, err
		}
		if part.FormName() == "filename" && part.FileName() != "" {
			filename, body = part.FileName(), data
			continue
		}
		fields[part.FormName()] = string(data)
	}
}

func writeISO(t *testing.T) (path, sum string) {
	const content = "iso-bytes"
	t.Helper()
	path = filepath.Join(t.TempDir(), "master0.iso")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(content))
	return path, hex.EncodeToString(digest[:])
}

func TestISOStoreVolumes(t *testing.T) {
	f := &fakeISOPVE{content: map[string]string{"pve1": `[
		{"volid":"local:iso/master0.iso","size":2048,"format":"iso"},
		{"volid":"local:vztmpl/debian.tar.zst","size":9},
		{"volid":"other:iso/master1.iso","size":7}
	]`}}
	got, err := f.start(t).Volumes(t.Context(), "pve1")
	if err != nil {
		t.Fatalf("Volumes: %v", err)
	}
	if want := map[string]uint64{"master0.iso": 2048}; len(got) != 1 || got["master0.iso"] != want["master0.iso"] {
		t.Errorf("Volumes = %v; want %v", got, want)
	}
	if want := "GET /api2/json/nodes/pve1/storage/local/content"; !slices.Contains(f.log(), want) {
		t.Errorf("requests = %v; want %s", f.log(), want)
	}
}

func TestISOStoreUpload(t *testing.T) {
	f := &fakeISOPVE{}
	store := f.start(t)
	path, sum := writeISO(t)

	if err := store.Upload(t.Context(), "pve1", path, sum); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if len(f.uploads) != 1 {
		t.Fatalf("uploads = %d; want 1", len(f.uploads))
	}
	up := f.uploads[0]
	wantFields := map[string]string{"content": "iso", "checksum": sum, "checksum-algorithm": "sha256"}
	for k, v := range wantFields {
		if up.fields[k] != v {
			t.Errorf("multipart field %s = %q; want %q", k, up.fields[k], v)
		}
	}
	if _, dup := up.fields["filename"]; dup {
		t.Error("filename sent as a form field; PVE rejects it alongside the file part")
	}
	if up.filename != "master0.iso" || string(up.body) != "iso-bytes" {
		t.Errorf("file part = %q (%q); want master0.iso (iso-bytes)", up.filename, up.body)
	}
	if want := "GET /api2/json/nodes/pve1/tasks/" + isoTaskUPID + "/status"; !slices.Contains(f.log(), want) {
		t.Errorf("requests = %v; want the upload task polled via %s", f.log(), want)
	}
}

func TestISOStoreUploadReportsFailedTask(t *testing.T) {
	const mismatch = "checksum mismatch: got 'aa' != expect 'bb'"
	f := &fakeISOPVE{taskExit: mismatch}
	path, sum := writeISO(t)

	err := f.start(t).Upload(t.Context(), "pve1", path, sum)
	if err == nil || !strings.Contains(err.Error(), mismatch) || !strings.Contains(err.Error(), "master0.iso") {
		t.Fatalf("Upload err = %v; want the failed task's exit status for master0.iso", err)
	}
}

func TestISOStoreUploadHonoursContext(t *testing.T) {
	f := &fakeISOPVE{taskHangs: true}
	store := f.start(t)
	path, sum := writeISO(t)
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(100*time.Millisecond, cancel)

	if err := store.Upload(ctx, "pve1", path, sum); !errors.Is(err, context.Canceled) {
		t.Fatalf("Upload err = %v; want context.Canceled", err)
	}
}

func TestISOStoreUploadRequestHonoursContext(t *testing.T) {
	f := &fakeISOPVE{uploadHangs: true}
	store := f.start(t)
	path, sum := writeISO(t)
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(100*time.Millisecond, cancel)

	done := make(chan error, 1)
	go func() { done <- store.Upload(ctx, "pve1", path, sum) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Upload err = %v; want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Upload ignored cancellation while the request body was in flight")
	}
}

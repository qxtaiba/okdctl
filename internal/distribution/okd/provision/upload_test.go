package provision

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

const (
	pveToken   = "root@pam!okdctl=secret"
	uploadUPID = "UPID:pve:00001234:00005678:66aabbcc:imgcopy::root@pam!okdctl:"
)

func newUploadExecutor() *executor.Executor {
	return executor.New(
		executor.WithInheritedEnv(),
		executor.WithLogger(logutil.NopLogger),
	)
}

type pveUpload struct {
	fields   map[string]string
	filename string
	body     string
}

type fakeISOStorage struct {
	mu      sync.Mutex
	content string
	uploads []pveUpload
}

func (f *fakeISOStorage) uploaded() []pveUpload {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.uploads)
}

func (f *fakeISOStorage) serve(t *testing.T) *credentials.ProxmoxCredentials {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api2/json/nodes/pve/storage/{storage}/status", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":{"type":"dir","content":"iso","active":1,"enabled":1}}`)
	})
	mux.HandleFunc("GET /api2/json/nodes/pve/storage/{storage}/content", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		fmt.Fprintf(w, `{"data":%s}`, f.content)
	})
	mux.HandleFunc("POST /api2/json/nodes/pve/storage/{storage}/upload", func(w http.ResponseWriter, r *http.Request) {
		fields, filename, body, err := readUploadForm(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		up := pveUpload{fields: fields, filename: filename, body: string(body)}
		f.mu.Lock()
		f.uploads = append(f.uploads, up)
		f.mu.Unlock()
		fmt.Fprintf(w, `{"data":%q}`, uploadUPID)
	})
	mux.HandleFunc("GET /api2/json/nodes/pve/tasks/{upid}/status", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"data":{"upid":%q,"node":"pve","status":"stopped","exitstatus":"OK"}}`, uploadUPID)
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "PVEAPIToken="+pveToken {
			t.Errorf("Authorization = %q; want the api token", got)
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return &credentials.ProxmoxCredentials{Endpoint: srv.URL, APIToken: []byte(pveToken)}
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

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func uploadFixture(t *testing.T, isos map[string]string) Options {
	t.Helper()
	opts := NewOptions(t.TempDir())
	isoDir := filepath.Join(opts.WorkDir, "custom-isos")
	if err := os.MkdirAll(isoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range isos {
		if err := os.WriteFile(filepath.Join(isoDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return opts
}

func uploadConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = "pve"
	cfg.Provider.Proxmox.ISOStorage = "local"
	return cfg
}

func writeRecords(t *testing.T, opts Options, records map[string]string) {
	t.Helper()
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workspace.ISOUploadRecordPath(opts.ProjectRoot), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readRecords(t *testing.T, opts Options) map[string]string {
	t.Helper()
	data, err := os.ReadFile(workspace.ISOUploadRecordPath(opts.ProjectRoot))
	if err != nil {
		t.Fatal(err)
	}
	records := map[string]string{}
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatal(err)
	}
	return records
}

func TestUploadCustomISOsUploadsOnlyMissingOrChanged(t *testing.T) {
	opts := uploadFixture(t, map[string]string{
		"bootstrap.iso": "boot",
		"master0.iso":   "m0",
		"master1.iso":   "m1-new",
	})
	writeRecords(t, opts, map[string]string{
		"local:iso/master0.iso": sha256Hex("m0"),
		"local:iso/master1.iso": sha256Hex("m1-old"),
	})
	f := &fakeISOStorage{content: `[
		{"volid":"local:iso/master0.iso","size":2},
		{"volid":"local:iso/master1.iso","size":6}
	]`}
	p := New(phase.WithExecutor(newUploadExecutor()), phase.WithProxmoxCredentials(f.serve(t)))

	if err := p.UploadCustomISOsToProxmox(t.Context(), uploadConfig(), opts); err != nil {
		t.Fatalf("UploadCustomISOsToProxmox: %v", err)
	}

	ups := f.uploaded()
	var names []string
	for _, up := range ups {
		names = append(names, up.filename)
		want := map[string]string{"content": "iso", "checksum-algorithm": "sha256", "checksum": sha256Hex(up.body)}
		for k, v := range want {
			if up.fields[k] != v {
				t.Errorf("%s: multipart field %s = %q; want %q", up.filename, k, up.fields[k], v)
			}
		}
	}
	if want := []string{"bootstrap.iso", "master1.iso"}; !slices.Equal(names, want) {
		t.Fatalf("uploaded %v; want %v", names, want)
	}
	want := map[string]string{
		"local:iso/bootstrap.iso": sha256Hex("boot"),
		"local:iso/master0.iso":   sha256Hex("m0"),
		"local:iso/master1.iso":   sha256Hex("m1-new"),
	}
	if got := readRecords(t, opts); !maps.Equal(got, want) {
		t.Errorf("upload records = %v; want %v", got, want)
	}
}

func TestUploadCustomISOsSameSizeWithoutRecordReuploads(t *testing.T) {
	opts := uploadFixture(t, map[string]string{"master0.iso": "m0"})
	f := &fakeISOStorage{content: `[{"volid":"local:iso/master0.iso","size":2}]`}
	p := New(phase.WithExecutor(newUploadExecutor()), phase.WithProxmoxCredentials(f.serve(t)))

	if err := p.UploadCustomISOsToProxmox(t.Context(), uploadConfig(), opts); err != nil {
		t.Fatalf("UploadCustomISOsToProxmox: %v", err)
	}
	if got := len(f.uploaded()); got != 1 {
		t.Fatalf("uploads = %d; want 1 (size match alone is not proof of content)", got)
	}
}

func TestUploadCustomISOsRequiresCredentials(t *testing.T) {
	opts := uploadFixture(t, map[string]string{"master0.iso": "m0"})
	p := New(phase.WithExecutor(newUploadExecutor()))

	err := p.UploadCustomISOsToProxmox(t.Context(), uploadConfig(), opts)
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("err = %v; want a credentials ConfigError", err)
	}
}

func TestISOUploadAlreadyDone(t *testing.T) {
	opts := uploadFixture(t, map[string]string{"master0.iso": "m0", "worker0.iso": "w0"})
	writeRecords(t, opts, map[string]string{
		"local:iso/master0.iso": sha256Hex("m0"),
		"local:iso/worker0.iso": sha256Hex("w0"),
	})

	t.Run("all present and verified", func(t *testing.T) {
		f := &fakeISOStorage{content: `[{"volid":"local:iso/master0.iso","size":2},{"volid":"local:iso/worker0.iso","size":2}]`}
		p := New(phase.WithExecutor(newUploadExecutor()), phase.WithProxmoxCredentials(f.serve(t)))
		done, err := p.ISOUploadAlreadyDone(t.Context(), uploadConfig(), opts)
		if err != nil || !done {
			t.Fatalf("ISOUploadAlreadyDone = %v, %v; want true, nil", done, err)
		}
	})

	t.Run("one missing", func(t *testing.T) {
		f := &fakeISOStorage{content: `[{"volid":"local:iso/master0.iso","size":2}]`}
		p := New(phase.WithExecutor(newUploadExecutor()), phase.WithProxmoxCredentials(f.serve(t)))
		done, err := p.ISOUploadAlreadyDone(t.Context(), uploadConfig(), opts)
		if err != nil || done {
			t.Fatalf("ISOUploadAlreadyDone = %v, %v; want false, nil", done, err)
		}
	})

	t.Run("record for another storage", func(t *testing.T) {
		cfg := uploadConfig()
		cfg.Provider.Proxmox.ISOStorage = "nfs-iso"
		f := &fakeISOStorage{content: `[{"volid":"nfs-iso:iso/master0.iso","size":2},{"volid":"nfs-iso:iso/worker0.iso","size":2}]`}
		p := New(phase.WithExecutor(newUploadExecutor()), phase.WithProxmoxCredentials(f.serve(t)))
		done, err := p.ISOUploadAlreadyDone(t.Context(), cfg, opts)
		if err != nil || done {
			t.Fatalf("ISOUploadAlreadyDone = %v, %v; want false, nil", done, err)
		}
	})

	t.Run("no credentials", func(t *testing.T) {
		p := New(phase.WithExecutor(newUploadExecutor()))
		done, err := p.ISOUploadAlreadyDone(t.Context(), uploadConfig(), opts)
		if err != nil || done {
			t.Fatalf("ISOUploadAlreadyDone = %v, %v; want false, nil", done, err)
		}
	})
}

package provision

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/platform"
)

type isoCapture struct {
	records []slog.Record
	attrs   []slog.Attr
}

func (h *isoCapture) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *isoCapture) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler interface requires value receiver
	r.AddAttrs(h.attrs...)
	h.records = append(h.records, r)
	return nil
}

func (h *isoCapture) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &isoCapture{records: h.records, attrs: merged}
}

func (h *isoCapture) WithGroup(_ string) slog.Handler { return h }

func newPhaseWithISOCapture(h *isoCapture) *Provisioner {
	return &Provisioner{
		BasePhase: phase.NewBasePhase(
			phase.WithExecutor(executor.New(executor.WithLogger(logutil.NopLogger))),
			phase.WithLogger(slog.New(h)),
		),
	}
}

func TestLogISOFound(t *testing.T) {
	h := &isoCapture{}
	p := newPhaseWithISOCapture(h)

	p.logISOFound("/path/a/foo.iso")
	p.logISOFound("/path/b/foo.iso")
	if got := len(h.records); got != 1 {
		t.Fatalf("after two calls with basename foo.iso: got %d records, want 1", got)
	}

	p.logISOFound("/path/a/bar.iso")
	if got := len(h.records); got != 2 {
		t.Fatalf("after adding bar.iso: got %d records, want 2", got)
	}

	p.logISOFound("/different/dir/foo.iso")
	if got := len(h.records); got != 2 {
		t.Fatalf("after repeat foo.iso from different dir: got %d records, want 2 (basename dedup)", got)
	}
}

func makeStreamJSON(arch, release, isoURL string) []byte {
	type disk struct {
		Location string `json:"location"`
		SHA256   string `json:"sha256"`
	}
	type isoFmt struct {
		Disk disk `json:"disk"`
	}
	type formats struct {
		ISO isoFmt `json:"iso"`
	}
	type metal struct {
		Release string  `json:"release"`
		Formats formats `json:"formats"`
	}
	type artifacts struct {
		Metal metal `json:"metal"`
	}
	type archEntry struct {
		Artifacts artifacts `json:"artifacts"`
	}
	type payload struct {
		Stream        string               `json:"stream"`
		Architectures map[string]archEntry `json:"architectures"`
	}
	p := payload{
		Stream: "stable",
		Architectures: map[string]archEntry{
			arch: {Artifacts: artifacts{Metal: metal{
				Release: release,
				Formats: formats{ISO: isoFmt{Disk: disk{
					Location: isoURL,
					SHA256:   "aabbccdd",
				}}},
			}}},
		},
	}
	b, _ := json.Marshal(p)
	return b
}

func newTestPhase(t *testing.T) *Provisioner {
	t.Helper()
	return &Provisioner{BasePhase: phase.NewBasePhase(
		phase.WithLogger(logutil.NopLogger),
		phase.WithExecutor(executor.New(executor.WithLogger(logutil.NopLogger))),
	)}
}

// TestFindOrDownloadFCOSISO_globDetectsCoreOSNames asserts local ISO
// auto-detect finds both the pre-4.19 fedora-coreos-*.iso shape and the
// 4.19+ scos-*.iso shape without hitting the network. The real
// hostssh.DefaultProxmoxISODir is checked first; the fixture lives under
// opts.WorkDir/downloads so the test only exercises that second glob loop.
func TestFindOrDownloadFCOSISO_globDetectsCoreOSNames(t *testing.T) {
	if _, err := os.Stat(hostssh.DefaultProxmoxISODir); err == nil {
		t.Skipf("%s exists on this machine; test assumes no local proxmox iso dir", hostssh.DefaultProxmoxISODir)
	}

	cases := []struct {
		name    string
		isoName string
	}{
		{"fedora-coreos shape", "fedora-coreos-40.20240101.3.0-x86_64.iso"},
		{"scos shape", "scos-10.0.20251103-0-live-iso.x86_64.iso"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			workDir := t.TempDir()
			downloadsDir := filepath.Join(workDir, "downloads")
			if err := os.MkdirAll(downloadsDir, 0o755); err != nil {
				t.Fatal(err)
			}
			isoPath := filepath.Join(downloadsDir, tt.isoName)
			if err := os.WriteFile(isoPath, []byte("fake"), 0o644); err != nil {
				t.Fatal(err)
			}

			p := newTestPhase(t)
			opts := Options{WorkDir: workDir}

			got, err := p.findOrDownloadFCOSISO(context.Background(), &config.Config{}, opts)
			if err != nil {
				t.Fatalf("findOrDownloadFCOSISO: %v", err)
			}
			if got != isoPath {
				t.Errorf("findOrDownloadFCOSISO = %q, want %q", got, isoPath)
			}
		})
	}
}

func TestParseOKDVersion(t *testing.T) {
	cases := []struct {
		in        string
		wantMajor int
		wantMinor int
		wantOK    bool
	}{
		{"4.19.0-0.okd-2025-05-01-123456", 4, 19, true},
		{"4.21.0-okd-scos.10", 4, 21, true},
		{"4.20.0-0.okd-2025-07-01-000000", 4, 20, true},
		{"4.15.0-0.okd-2024-01-27-040212", 4, 15, true},
		{"5.0.0-okd-scos.ec.4", 5, 0, true},
		{"not-a-version", 0, 0, false},
		{"", 0, 0, false},
	}
	for _, tt := range cases {
		gotMajor, gotMinor, ok := parseOKDVersion(tt.in)
		if ok != tt.wantOK || gotMajor != tt.wantMajor || gotMinor != tt.wantMinor {
			t.Errorf("parseOKDVersion(%q) = (%d, %d, %v), want (%d, %d, %v)",
				tt.in, gotMajor, gotMinor, ok, tt.wantMajor, tt.wantMinor, tt.wantOK)
		}
	}
}

func TestDetectCoreOSVersion_malformedVersion(t *testing.T) {
	p := newTestPhase(t)
	for _, v := range []string{"not-a-version", "", "x.y.0"} {
		_, err := p.DetectCoreOSVersion(context.Background(), v)
		if err == nil {
			t.Errorf("DetectCoreOSVersion(%q): expected error, got nil", v)
			continue
		}
		var ce *errtypes.ConfigError
		if !errors.As(err, &ce) {
			t.Errorf("DetectCoreOSVersion(%q): want *errtypes.ConfigError, got %T: %v", v, err, err)
		}
	}
}

func TestStreamFileForVersion(t *testing.T) {
	cases := []struct {
		major, minor int
		want         string
	}{
		{4, 0, "fcos.json"},
		{4, 15, "fcos.json"},
		{4, 18, "fcos.json"},
		{4, 19, "scos.json"},
		{4, 20, "scos.json"},
		{4, 99, "scos.json"},
		{5, 0, "scos.json"},
		{5, 19, "scos.json"},
		{6, 0, "scos.json"},
	}
	for _, tt := range cases {
		if got := streamFileForVersion(tt.major, tt.minor); got != tt.want {
			t.Errorf("streamFileForVersion(%d, %d) = %q, want %q", tt.major, tt.minor, got, tt.want)
		}
	}
}

func TestFetchCoreOSStream_ok(t *testing.T) {
	const (
		release = "9.0.20250510-0"
		isoURL  = "https://example.com/scos.iso"
	)
	arch := platform.CoreOSArch()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(makeStreamJSON(arch, release, isoURL))
	}))
	t.Cleanup(srv.Close)

	sd, err := fetchCoreOSStream(context.Background(), srv.URL+"/scos.json", "")
	if err != nil {
		t.Fatalf("fetchCoreOSStream: %v", err)
	}
	entry, ok := sd.Architectures[arch]
	if !ok {
		t.Fatalf("arch %q not in result", arch)
	}
	if got := entry.Artifacts.Metal.Release; got != release {
		t.Errorf("release = %q, want %q", got, release)
	}
	if got := entry.Artifacts.Metal.Formats.ISO.Disk.Location; got != isoURL {
		t.Errorf("iso location = %q, want %q", got, isoURL)
	}
}

func TestFetchCoreOSStream_non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	if _, err := fetchCoreOSStream(context.Background(), srv.URL+"/scos.json", ""); err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestDetectCoreOSVersion_scosFor419(t *testing.T) {
	arch := platform.CoreOSArch()
	body := makeStreamJSON(arch, "9.0.20250510-0", "https://example.com/scos419.iso")
	sum := sha256.Sum256(body)
	const testSHA = "testpin0000000000000000000000000000000419"

	var requestedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	old := streamRawBaseURL
	streamRawBaseURL = srv.URL
	t.Cleanup(func() { streamRawBaseURL = old })

	oldPins := streamPins
	streamPins = map[okdVersionKey]coreOSStreamPin{
		{4, 19}: {CommitSHA: testSHA, JSONSHA256: hex.EncodeToString(sum[:])},
	}
	t.Cleanup(func() { streamPins = oldPins })

	p := newTestPhase(t)
	info, err := p.DetectCoreOSVersion(context.Background(), "4.19.0-0.okd-2025-05-01-000000")
	if err != nil {
		t.Fatalf("DetectCoreOSVersion 4.19: %v", err)
	}
	if !strings.Contains(requestedPath, "/openshift/installer/"+testSHA+"/data/data/coreos/scos.json") {
		t.Errorf("4.19 should fetch scos.json at pinned commit; got %q", requestedPath)
	}
	if info.ISOUrl != "https://example.com/scos419.iso" {
		t.Errorf("ISOUrl = %q, want https://example.com/scos419.iso", info.ISOUrl)
	}
}

func TestDetectCoreOSVersion_fcosFor418(t *testing.T) {
	arch := platform.CoreOSArch()
	body := makeStreamJSON(arch, "39.20231101.3.0", "https://example.com/fcos418.iso")
	sum := sha256.Sum256(body)
	const testSHA = "testpin0000000000000000000000000000000418"

	var requestedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	old := streamRawBaseURL
	streamRawBaseURL = srv.URL
	t.Cleanup(func() { streamRawBaseURL = old })

	oldPins := streamPins
	streamPins = map[okdVersionKey]coreOSStreamPin{
		{4, 18}: {CommitSHA: testSHA, JSONSHA256: hex.EncodeToString(sum[:])},
	}
	t.Cleanup(func() { streamPins = oldPins })

	p := newTestPhase(t)
	info, err := p.DetectCoreOSVersion(context.Background(), "4.18.0-0.okd-2024-12-01-000000")
	if err != nil {
		t.Fatalf("DetectCoreOSVersion 4.18: %v", err)
	}
	if !strings.Contains(requestedPath, "/openshift/installer/"+testSHA+"/data/data/coreos/fcos.json") {
		t.Errorf("4.18 should fetch fcos.json at pinned commit; got %q", requestedPath)
	}
	if info.ISOUrl != "https://example.com/fcos418.iso" {
		t.Errorf("ISOUrl = %q, want https://example.com/fcos418.iso", info.ISOUrl)
	}
}

func TestDetectCoreOSVersion_fetchFailErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	old := streamRawBaseURL
	streamRawBaseURL = srv.URL
	t.Cleanup(func() { streamRawBaseURL = old })

	oldPins := streamPins
	streamPins = map[okdVersionKey]coreOSStreamPin{
		{4, 19}: {CommitSHA: "testpin0000000000000000000000000000000419", JSONSHA256: "aaaa"},
	}
	t.Cleanup(func() { streamPins = oldPins })

	p := newTestPhase(t)
	if _, err := p.DetectCoreOSVersion(context.Background(), "4.19.0-0.okd-2025-05-01-000000"); err == nil {
		t.Fatal("expected error when upstream fetch fails, got nil")
	}
}

// TestDetectCoreOSVersion_majorMinorDistinctness pins both "5.0" and a
// synthetic "4.0" to different commits/checksums with the same minor number
// (0) and asserts each resolves to its own pin — the (major, minor) key
// prevents a 5.x version from colliding with an unrelated 4.x entry.
func TestDetectCoreOSVersion_majorMinorDistinctness(t *testing.T) {
	arch := platform.CoreOSArch()
	body4 := makeStreamJSON(arch, "39.20240101.3.0", "https://example.com/fcos4.iso")
	sum4 := sha256.Sum256(body4)
	body5 := makeStreamJSON(arch, "9.0.20260601-0", "https://example.com/scos5.iso")
	sum5 := sha256.Sum256(body5)

	const (
		testSHA4 = "testpin0000000000000000000000000000000400"
		testSHA5 = "testpin0000000000000000000000000000000500"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, testSHA4):
			_, _ = w.Write(body4)
		case strings.Contains(r.URL.Path, testSHA5):
			_, _ = w.Write(body5)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	old := streamRawBaseURL
	streamRawBaseURL = srv.URL
	t.Cleanup(func() { streamRawBaseURL = old })

	oldPins := streamPins
	streamPins = map[okdVersionKey]coreOSStreamPin{
		{4, 0}: {CommitSHA: testSHA4, JSONSHA256: hex.EncodeToString(sum4[:])},
		{5, 0}: {CommitSHA: testSHA5, JSONSHA256: hex.EncodeToString(sum5[:])},
	}
	t.Cleanup(func() { streamPins = oldPins })

	p := newTestPhase(t)

	info4, err := p.DetectCoreOSVersion(context.Background(), "4.0.0-0.okd-2024-01-01-000000")
	if err != nil {
		t.Fatalf("DetectCoreOSVersion 4.0: %v", err)
	}
	if info4.ISOUrl != "https://example.com/fcos4.iso" {
		t.Errorf("4.0 ISOUrl = %q, want fcos4.iso pin", info4.ISOUrl)
	}

	info5, err := p.DetectCoreOSVersion(context.Background(), "5.0.0-okd-scos.ec.4")
	if err != nil {
		t.Fatalf("DetectCoreOSVersion 5.0: %v", err)
	}
	if info5.ISOUrl != "https://example.com/scos5.iso" {
		t.Errorf("5.0 ISOUrl = %q, want scos5.iso pin", info5.ISOUrl)
	}

	if info4.ISOUrl == info5.ISOUrl {
		t.Fatal("4.0 and 5.0 pins resolved to the same ISO; major.minor key is not distinguishing them")
	}
}

// TestDetectCoreOSVersion_unpinned5x asserts an unpinned 5.x version fails
// with the requested major.minor in the error, not a stale "4.%d" message.
func TestDetectCoreOSVersion_unpinned5x(t *testing.T) {
	p := newTestPhase(t)
	_, err := p.DetectCoreOSVersion(context.Background(), "5.0.0-okd-scos.ec.4")
	if err == nil {
		t.Fatal("expected error for unpinned 5.0, got nil")
	}
	var ce *errtypes.ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *errtypes.ConfigError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "5.0") {
		t.Errorf("error %q does not mention requested version 5.0", err.Error())
	}
}

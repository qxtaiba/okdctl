package provision

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

func overrideStream(t *testing.T, baseURL string, pins map[okdVersionKey]coreOSStreamPin) {
	t.Helper()
	oldURL := streamRawBaseURL
	streamRawBaseURL = baseURL
	t.Cleanup(func() { streamRawBaseURL = oldURL })
	oldPins := streamPins
	streamPins = pins
	t.Cleanup(func() { streamPins = oldPins })
}

// The real hostssh.DefaultProxmoxISODir is checked first; the fixture lives
// under opts.WorkDir/downloads, exercising only the second glob loop.
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
		{4, 18, "fcos.json"},
		{4, 19, "scos.json"},
		{5, 0, "scos.json"},
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

func TestDetectCoreOSVersion_streamFile(t *testing.T) {
	const (
		okdVersion = "4.19.0-0.okd-2025-05-01-000000"
		isoURL     = "https://example.com/scos419.iso"
		testSHA    = "testpin0000000000000000000000000000000419"
	)
	body := makeStreamJSON(platform.CoreOSArch(), "9.0.20250510-0", isoURL)
	sum := sha256.Sum256(body)

	var requestedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	overrideStream(t, srv.URL, map[okdVersionKey]coreOSStreamPin{
		{Major: 4, Minor: 19}: {CommitSHA: testSHA, JSONSHA256: hex.EncodeToString(sum[:])},
	})

	p := newTestPhase(t)
	info, err := p.DetectCoreOSVersion(context.Background(), okdVersion)
	if err != nil {
		t.Fatalf("DetectCoreOSVersion %s: %v", okdVersion, err)
	}
	wantPath := "/openshift/installer/" + testSHA + "/data/data/coreos/scos.json"
	if !strings.Contains(requestedPath, wantPath) {
		t.Errorf("%s should fetch scos.json at the pinned commit; got %q", okdVersion, requestedPath)
	}
	if info.ISOUrl != isoURL {
		t.Errorf("ISOUrl = %q, want %q", info.ISOUrl, isoURL)
	}
}

func TestDetectCoreOSVersion_fetchFailErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	overrideStream(t, srv.URL, map[okdVersionKey]coreOSStreamPin{
		{4, 19}: {CommitSHA: "testpin0000000000000000000000000000000419", JSONSHA256: "aaaa"},
	})

	p := newTestPhase(t)
	if _, err := p.DetectCoreOSVersion(context.Background(), "4.19.0-0.okd-2025-05-01-000000"); err == nil {
		t.Fatal("expected error when upstream fetch fails, got nil")
	}
}

// Pins "5.0" and a synthetic "4.0" to different commits with the same minor
// (0) to prove the (major, minor) key doesn't collide.
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

	overrideStream(t, srv.URL, map[okdVersionKey]coreOSStreamPin{
		{4, 0}: {CommitSHA: testSHA4, JSONSHA256: hex.EncodeToString(sum4[:])},
		{5, 0}: {CommitSHA: testSHA5, JSONSHA256: hex.EncodeToString(sum5[:])},
	})

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

// Guards against a stale "4.%d" error message when the unpinned version is
// actually 5.x.
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

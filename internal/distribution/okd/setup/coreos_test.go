package setup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
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

func newTestPhase(t *testing.T) *Phase {
	t.Helper()
	return &Phase{BasePhase: phase.NewBasePhase("test",
		phase.WithLogger(logutil.NopLogger),
		phase.WithExecutor(executor.New(executor.WithLogger(logutil.NopLogger))),
	)}
}

func TestParseOKDMinor(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"4.19.0-0.okd-2025-05-01-123456", 19},
		{"4.21.0-okd-scos.10", 21},
		{"4.20.0-0.okd-2025-07-01-000000", 20},
		{"4.15.0-0.okd-2024-01-27-040212", 15},
		{"not-a-version", 0},
		{"", 0},
	}
	for _, tt := range cases {
		got := parseOKDMinor(tt.in)
		if got != tt.want {
			t.Errorf("parseOKDMinor(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestStreamFileForMinor(t *testing.T) {
	cases := []struct {
		minor int
		want  string
	}{
		{0, "fcos.json"},
		{15, "fcos.json"},
		{18, "fcos.json"},
		{19, "scos.json"},
		{20, "scos.json"},
		{99, "scos.json"},
	}
	for _, tt := range cases {
		if got := streamFileForMinor(tt.minor); got != tt.want {
			t.Errorf("streamFileForMinor(%d) = %q, want %q", tt.minor, got, tt.want)
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

	sd, err := fetchCoreOSStream(context.Background(), srv.URL+"/scos.json")
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

	if _, err := fetchCoreOSStream(context.Background(), srv.URL+"/scos.json"); err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestDetectCoreOSVersion_scosFor419(t *testing.T) {
	arch := platform.CoreOSArch()
	var requestedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		_, _ = w.Write(makeStreamJSON(arch, "9.0.20250510-0", "https://example.com/scos419.iso"))
	}))
	t.Cleanup(srv.Close)

	old := streamRawBaseURL
	streamRawBaseURL = srv.URL
	t.Cleanup(func() { streamRawBaseURL = old })

	p := newTestPhase(t)
	info, err := p.DetectCoreOSVersion(context.Background(), "4.19.0-0.okd-2025-05-01-000000")
	if err != nil {
		t.Fatalf("DetectCoreOSVersion 4.19: %v", err)
	}
	if !strings.HasSuffix(requestedPath, "/release-4.19/data/data/coreos/scos.json") {
		t.Errorf("4.19 should fetch scos.json; got %q", requestedPath)
	}
	if info.ISOUrl != "https://example.com/scos419.iso" {
		t.Errorf("ISOUrl = %q, want https://example.com/scos419.iso", info.ISOUrl)
	}
}

func TestDetectCoreOSVersion_fcosFor418(t *testing.T) {
	arch := platform.CoreOSArch()
	var requestedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		_, _ = w.Write(makeStreamJSON(arch, "39.20231101.3.0", "https://example.com/fcos418.iso"))
	}))
	t.Cleanup(srv.Close)

	old := streamRawBaseURL
	streamRawBaseURL = srv.URL
	t.Cleanup(func() { streamRawBaseURL = old })

	p := newTestPhase(t)
	info, err := p.DetectCoreOSVersion(context.Background(), "4.18.0-0.okd-2024-12-01-000000")
	if err != nil {
		t.Fatalf("DetectCoreOSVersion 4.18: %v", err)
	}
	if !strings.HasSuffix(requestedPath, "/release-4.18/data/data/coreos/fcos.json") {
		t.Errorf("4.18 should fetch fcos.json; got %q", requestedPath)
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

	p := newTestPhase(t)
	if _, err := p.DetectCoreOSVersion(context.Background(), "4.19.0-0.okd-2025-05-01-000000"); err == nil {
		t.Fatal("expected error when upstream fetch fails, got nil")
	}
}

package setup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		Stream: "c9s",
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

func TestFetchSCOSStream_ok(t *testing.T) {
	const (
		release = "9.0.20250510-0"
		isoURL  = "https://example.com/scos.iso"
	)
	arch := platform.CoreOSArch()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(makeStreamJSON(arch, release, isoURL))
	}))
	t.Cleanup(srv.Close)

	sd, err := fetchSCOSStream(context.Background(), srv.URL+"/scos.json")
	if err != nil {
		t.Fatalf("fetchSCOSStream: %v", err)
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

func TestFetchSCOSStream_non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	if _, err := fetchSCOSStream(context.Background(), srv.URL+"/scos.json"); err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestDetectCoreOSVersion_directFetch419(t *testing.T) {
	arch := platform.CoreOSArch()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(makeStreamJSON(arch, "9.0.20250510-0", "https://example.com/fcos419.iso"))
	}))
	t.Cleanup(srv.Close)

	old := scosRawBaseURL
	scosRawBaseURL = srv.URL
	t.Cleanup(func() { scosRawBaseURL = old })

	p := newTestPhase(t)
	info, err := p.DetectCoreOSVersion(context.Background(), "4.19.0-0.okd-2025-05-01-000000")
	if err != nil {
		t.Fatalf("DetectCoreOSVersion 4.19: %v", err)
	}
	if info.ISOUrl != "https://example.com/fcos419.iso" {
		t.Errorf("ISOUrl = %q, want https://example.com/fcos419.iso", info.ISOUrl)
	}
	if info.Architecture != arch {
		t.Errorf("Architecture = %q, want %q", info.Architecture, arch)
	}
}

func TestDetectCoreOSVersion_directFetch420(t *testing.T) {
	arch := platform.CoreOSArch()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(makeStreamJSON(arch, "10.0.20250701-0", "https://example.com/scos420.iso"))
	}))
	t.Cleanup(srv.Close)

	old := scosRawBaseURL
	scosRawBaseURL = srv.URL
	t.Cleanup(func() { scosRawBaseURL = old })

	p := newTestPhase(t)
	info, err := p.DetectCoreOSVersion(context.Background(), "4.20.0-0.okd-2025-07-01-000000")
	if err != nil {
		t.Fatalf("DetectCoreOSVersion 4.20: %v", err)
	}
	if info.ISOUrl != "https://example.com/scos420.iso" {
		t.Errorf("ISOUrl = %q, want https://example.com/scos420.iso", info.ISOUrl)
	}
}

func TestDetectCoreOSVersion_oldMinor_skipsDirectFetch(t *testing.T) {
	directFetched := false
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		directFetched = true
	}))
	t.Cleanup(srv.Close)

	old := scosRawBaseURL
	scosRawBaseURL = srv.URL
	t.Cleanup(func() { scosRawBaseURL = old })

	p := newTestPhase(t)
	_, _ = p.DetectCoreOSVersion(context.Background(), "4.15.0-0.okd-2024-01-27-040212")
	if directFetched {
		t.Fatal("4.15 must not hit the direct-fetch URL; fell through to shellout instead")
	}
}

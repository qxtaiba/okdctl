package provision

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/qxtaiba/okdctl/internal/testutil"
)

func makeStreamJSON(arch, release, isoURL, isoSHA256 string) []byte {
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
		Stream: "c10s",
		Architectures: map[string]archEntry{
			arch: {Artifacts: artifacts{Metal: metal{
				Release: release,
				Formats: formats{ISO: isoFmt{Disk: disk{
					Location: isoURL,
					SHA256:   isoSHA256,
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

func installFakeInstaller(t *testing.T, stdout, stderr string, code int) (argvLog string) {
	t.Helper()
	dir := t.TempDir()
	argvLog = filepath.Join(dir, "argv.log")
	outFile, errFile := filepath.Join(dir, "stdout"), filepath.Join(dir, "stderr")
	for path, body := range map[string]string{outFile: stdout, errFile: stderr} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	testutil.InstallFakeBin(t, "openshift-install", fmt.Sprintf(
		"#!/bin/sh\necho \"$@\" >> '%s'\ncat '%s'\ncat '%s' >&2\nexit %d\n", argvLog, outFile, errFile, code))
	return argvLog
}

func TestDetectCoreOSVersion_AsksInstallerForItsOwnStream(t *testing.T) {
	stream := makeStreamJSON(platform.ClusterCoreOSArch, "10.0.20251103-0", "https://example.com/scos.iso", "aabbccdd")
	argvLog := installFakeInstaller(t, string(stream), "", 0)

	info, err := newTestPhase(t).DetectCoreOSVersion(t.Context())
	if err != nil {
		t.Fatalf("DetectCoreOSVersion: %v", err)
	}
	want := CoreOSInfo{Version: "10.0.20251103-0", ISOUrl: "https://example.com/scos.iso", ISOChecksum: "aabbccdd"}
	if *info != want {
		t.Errorf("info = %+v, want %+v", *info, want)
	}
	argv, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatalf("installer was never invoked: %v", err)
	}
	if got := strings.TrimSpace(string(argv)); got != "coreos print-stream-json" {
		t.Errorf("installer argv = %q, want %q", got, "coreos print-stream-json")
	}
}

func TestDetectCoreOSVersion_PicksClusterArchFromRealStream(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "coreos-stream-4.22.0-okd-scos.10.json"))
	if err != nil {
		t.Fatal(err)
	}
	installFakeInstaller(t, string(body), "", 0)

	want := CoreOSInfo{
		Version:     "10.0.20251103-0",
		ISOUrl:      "https://rhcos.mirror.openshift.com/art/storage/prod/streams/c10s/builds/10.0.20251103-0/x86_64/scos-10.0.20251103-0-live-iso.x86_64.iso",
		ISOChecksum: "aff9c4a263d51356584d8334a20f13e24d04803e4eb9b49c2b499e0ad908e94a",
	}

	info, err := newTestPhase(t).DetectCoreOSVersion(t.Context())
	if err != nil {
		t.Fatalf("DetectCoreOSVersion: %v", err)
	}
	if *info != want {
		t.Errorf("info = %+v, want %+v", *info, want)
	}
}

func TestDetectCoreOSVersion_UnusableInstallerIsOneClearError(t *testing.T) {
	cases := []struct {
		name     string
		install  func(t *testing.T)
		wantExit bool
	}{
		{name: "not on PATH", install: func(t *testing.T) { t.Setenv("PATH", t.TempDir()) }},
		{
			name:     "subcommand fails",
			install:  func(t *testing.T) { installFakeInstaller(t, "", `level=fatal msg="unknown command"`, 1) },
			wantExit: true,
		},
		{name: "output is not a stream document", install: func(t *testing.T) { installFakeInstaller(t, "not json", "", 0) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.install(t)

			_, err := newTestPhase(t).DetectCoreOSVersion(t.Context())
			var ce *errtypes.ConfigError
			if !errors.As(err, &ce) {
				t.Fatalf("want *errtypes.ConfigError, got %T: %v", err, err)
			}
			for _, want := range []string{"openshift-install", "coreos stream", "print-stream-json"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
			var exitErr *executor.ExitError
			if tc.wantExit && !errors.As(err, &exitErr) {
				t.Errorf("installer exit status lost from the chain: %v", err)
			}
		})
	}
}

func TestDetectCoreOSVersion_RefusesStreamWithoutVerifiableISO(t *testing.T) {
	arch := platform.ClusterCoreOSArch
	cases := []struct {
		name   string
		stream []byte
		want   string
	}{
		{"architecture absent", makeStreamJSON("riscv64", "10.0", "https://example.com/scos.iso", "aabbccdd"), arch + " architecture not found"},
		{"iso location absent", makeStreamJSON(arch, "10.0", "", "aabbccdd"), "iso location not found"},
		{"iso checksum absent", makeStreamJSON(arch, "10.0", "https://example.com/scos.iso", ""), "iso sha256 not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installFakeInstaller(t, string(tc.stream), "", 0)

			_, err := newTestPhase(t).DetectCoreOSVersion(t.Context())
			var ce *errtypes.ConfigError
			if !errors.As(err, &ce) {
				t.Fatalf("want *errtypes.ConfigError, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestEnsureCoreOSISO_VerifiesTheDownloadAgainstTheStreamChecksum(t *testing.T) {
	iso := []byte("fake coreos live iso")
	sum := sha256.Sum256(iso)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(iso)
	}))
	t.Cleanup(srv.Close)
	isoURL := srv.URL + "/scos-10.0.20251103-0-live-iso.iso"

	t.Run("matching checksum is kept", func(t *testing.T) {
		installFakeInstaller(t, string(makeStreamJSON(platform.ClusterCoreOSArch, "10.0.20251103-0", isoURL, hex.EncodeToString(sum[:]))), "", 0)
		workDir := t.TempDir()

		got, err := newTestPhase(t).EnsureCoreOSISO(t.Context(), Options{WorkDir: workDir})
		if err != nil {
			t.Fatalf("EnsureCoreOSISO: %v", err)
		}
		if want := filepath.Join(workDir, "downloads", "scos-10.0.20251103-0-live-iso.iso"); got != want {
			t.Errorf("iso path = %q, want %q", got, want)
		}
		if body, err := os.ReadFile(got); err != nil || !bytes.Equal(body, iso) {
			t.Errorf("downloaded iso = %q (err %v), want the served bytes", body, err)
		}
	})

	t.Run("mismatched checksum is refused and removed", func(t *testing.T) {
		installFakeInstaller(t, string(makeStreamJSON(platform.ClusterCoreOSArch, "10.0.20251103-0", isoURL, strings.Repeat("0", 64))), "", 0)
		workDir := t.TempDir()

		if _, err := newTestPhase(t).EnsureCoreOSISO(t.Context(), Options{WorkDir: workDir}); err == nil {
			t.Fatal("EnsureCoreOSISO accepted an iso that does not match the stream checksum")
		}
		if _, err := os.Stat(filepath.Join(workDir, "downloads", "scos-10.0.20251103-0-live-iso.iso")); !os.IsNotExist(err) {
			t.Errorf("unverified iso left on disk (stat err = %v)", err)
		}
	})
}

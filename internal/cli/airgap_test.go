package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/fetchplan"
)

// fixtureShas is the canned per-purpose SHA map the golden-file test uses.
// Keys match fetchplan Purpose values; values are stable 64-hex placeholders.
var fixtureShas = map[string]string{
	"tool-helm":    "1111111111111111111111111111111111111111111111111111111111111111",
	"tool-sops":    "2222222222222222222222222222222222222222222222222222222222222222",
	"tool-yq":      "3333333333333333333333333333333333333333333333333333333333333333",
	"bootstrap-oc": "4444444444444444444444444444444444444444444444444444444444444444",
}

type fixtureShaFetcher struct{}

func (fixtureShaFetcher) fetch(_ context.Context, b fetchplan.Blob) (string, error) {
	return fixtureShas[b.Purpose], nil
}

const (
	fixtureVersion = "4.21.0-okd-scos.10"
	fixtureDigest  = "sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	fixtureChannel = "stable-4.21"
	fixtureISOURL  = "https://rhcos.mirror.openshift.com/art/storage/releases/scos-9.20250401.0/x86_64/scos-live.x86_64.iso"
	fixtureISOSHA  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

// fixtureStream is a canned airgapStreamData used in golden-file tests.
type fixtureStream struct{}

func (fixtureStream) fetch(_ context.Context, _ int) (*airgapStreamData, error) {
	sd := &airgapStreamData{
		Architectures: map[string]airgapArchData{
			"x86_64": {
				Artifacts: struct {
					Metal airgapMetalData `json:"metal"`
				}{
					Metal: airgapMetalData{
						Release: "9.20250401.0",
						Formats: airgapFormatsData{
							ISO: struct {
								Disk airgapDiskData `json:"disk"`
							}{
								Disk: airgapDiskData{
									Location: fixtureISOURL,
									SHA256:   fixtureISOSHA,
								},
							},
						},
					},
				},
			},
		},
	}
	return sd, nil
}

func goldenPath(name string) string {
	return filepath.Join("testdata", "airgap", name)
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(goldenPath(name))
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return string(data)
}

func TestAirgapPlan_pinRelease_golden(t *testing.T) {
	outDir := t.TempDir()
	blobs, err := buildAirgapPlan(context.Background(), fixtureVersion, 21, fixtureStream{}, fixtureShaFetcher{})
	if err != nil {
		t.Fatalf("buildAirgapPlan: %v", err)
	}
	if wErr := writeAirgapArtifacts(outDir, fixtureVersion, fixtureDigest, "", blobs); wErr != nil {
		t.Fatalf("writeAirgapArtifacts: %v", wErr)
	}

	checkFileMatchesGolden(t, outDir, "isc.yaml", "isc_pinrelease.golden.yaml")
	checkFileMatchesGolden(t, outDir, "airgap.yaml", "airgap.golden.yaml")
}

func TestAirgapPlan_graph_golden(t *testing.T) {
	outDir := t.TempDir()
	blobs, err := buildAirgapPlan(context.Background(), fixtureVersion, 21, fixtureStream{}, fixtureShaFetcher{})
	if err != nil {
		t.Fatalf("buildAirgapPlan: %v", err)
	}
	if wErr := writeAirgapArtifacts(outDir, fixtureVersion, fixtureDigest, fixtureChannel, blobs); wErr != nil {
		t.Fatalf("writeAirgapArtifacts: %v", wErr)
	}

	checkFileMatchesGolden(t, outDir, "isc.yaml", "isc_graph.golden.yaml")
}

func TestAirgapPlan_scripts_golden(t *testing.T) {
	outDir := t.TempDir()
	blobs, err := buildAirgapPlan(context.Background(), fixtureVersion, 21, fixtureStream{}, fixtureShaFetcher{})
	if err != nil {
		t.Fatalf("buildAirgapPlan: %v", err)
	}
	if wErr := writeAirgapArtifacts(outDir, fixtureVersion, fixtureDigest, "", blobs); wErr != nil {
		t.Fatalf("writeAirgapArtifacts: %v", wErr)
	}

	checkScriptMatchesGolden(t, outDir, "run-oc-mirror.sh", "run-oc-mirror.sh.golden")
	checkScriptMatchesGolden(t, outDir, "fetch-blobs.sh", "fetch-blobs.sh.golden")
}

func TestAirgapMinor(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"4.21.0-okd-scos.10", 21},
		{"4.19.0-0.okd-2025-05-01-000000", 19},
		{"4.15.0-0.okd-2024-01-01-000000", 15},
		{"", 0},
	}
	for _, tt := range cases {
		if got := airgapMinor(tt.in); got != tt.want {
			t.Errorf("airgapMinor(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestMirrorPath(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{
			"https://get.helm.sh/helm-v3.17.3-linux-amd64.tar.gz",
			"helm/helm-v3.17.3-linux-amd64.tar.gz",
		},
		{
			"https://rhcos.mirror.openshift.com/art/storage/releases/scos.iso",
			"rhcos/art/storage/releases/scos.iso",
		},
		{
			"https://mirror.openshift.com/pub/openshift-v4/clients/oc/latest/linux/oc.tar.gz",
			"openshift-mirror/pub/openshift-v4/clients/oc/latest/linux/oc.tar.gz",
		},
	}
	for _, tc := range cases {
		if got := mirrorPath(tc.url); got != tc.want {
			t.Errorf("mirrorPath(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestIsoFromStream_missingArch(t *testing.T) {
	sd := &airgapStreamData{Architectures: map[string]airgapArchData{}}
	if _, err := isoFromStream(sd); err == nil {
		t.Fatal("expected error for missing x86_64 arch, got nil")
	}
}

func TestSidecarFor_helm(t *testing.T) {
	b := fetchplan.Blob{URL: "https://get.helm.sh/helm-v3.17.3-linux-amd64.tar.gz", Purpose: "tool-helm"}
	sidecar, filename := sidecarFor(b)
	if sidecar != "https://get.helm.sh/helm-v3.17.3-linux-amd64.tar.gz.sha256sum" {
		t.Errorf("helm sidecar = %q", sidecar)
	}
	if filename != "helm-v3.17.3-linux-amd64.tar.gz" {
		t.Errorf("helm filename = %q", filename)
	}
}

func TestSidecarFor_sops(t *testing.T) {
	b := fetchplan.Blob{URL: "https://github.com/getsops/sops/releases/download/v3.9.4/sops-v3.9.4.linux.amd64", Purpose: "tool-sops"}
	sidecar, filename := sidecarFor(b)
	if !strings.HasSuffix(sidecar, "/sops-v3.9.4.checksums.txt") {
		t.Errorf("sops sidecar = %q", sidecar)
	}
	if filename != "sops-v3.9.4.linux.amd64" {
		t.Errorf("sops filename = %q", filename)
	}
}

func TestSidecarFor_bootstrapOC(t *testing.T) {
	b := fetchplan.Blob{URL: "https://mirror.openshift.com/pub/openshift-v4/clients/oc/latest/linux/oc.tar.gz", Purpose: "bootstrap-oc"}
	sidecar, filename := sidecarFor(b)
	if sidecar != "https://mirror.openshift.com/pub/openshift-v4/clients/oc/latest/sha256sum.txt" {
		t.Errorf("oc sidecar = %q", sidecar)
	}
	if filename != "oc.tar.gz" {
		t.Errorf("oc filename = %q", filename)
	}
}

func TestSidecarFor_unknown(t *testing.T) {
	b := fetchplan.Blob{URL: "https://example.com/file", Purpose: "unknown"}
	sidecar, _ := sidecarFor(b)
	if sidecar != "" {
		t.Errorf("unknown purpose sidecar = %q, want empty", sidecar)
	}
}

func checkFileMatchesGolden(t *testing.T, outDir, fileName, goldenName string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(outDir, fileName))
	if err != nil {
		t.Fatalf("read %s: %v", fileName, err)
	}
	want := readGolden(t, goldenName)
	if string(got) != want {
		t.Errorf("%s mismatch:\n--- got ---\n%s\n--- want ---\n%s", fileName, got, want)
	}
}

func checkScriptMatchesGolden(t *testing.T, outDir, fileName, goldenName string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(outDir, fileName))
	if err != nil {
		t.Fatalf("read %s: %v", fileName, err)
	}
	want := readGolden(t, goldenName)
	gotNorm := strings.ReplaceAll(string(got), outDir, "<OUTDIR>")
	if gotNorm != want {
		t.Errorf("%s mismatch:\n--- got (normalised) ---\n%s\n--- want ---\n%s", fileName, gotNorm, want)
	}
}

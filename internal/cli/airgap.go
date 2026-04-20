package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/fetchplan"
	"github.com/qxtaiba/okdctl/internal/system"
)

var (
	airgapPlanVersion        string
	airgapPlanDigest         string
	airgapPlanChannel        string
	airgapPlanOutDir         string
	airgapPlanStreamJSONPath string
)

var airgapCmd = &cobra.Command{
	Use:   "airgap",
	Short: "Tools for air-gap deployments",
}

var airgapPlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Emit oc-mirror ImageSetConfiguration and blob manifest for an air-gap deploy",
	Long: `Generate the four artifacts needed to mirror an OKD release into a
disconnected environment:

  isc.yaml           mirror.openshift.io/v2alpha1 ImageSetConfiguration
  airgap.yaml        HTTPS blob list with pinned SHA256s
  run-oc-mirror.sh   wrapper script exporting OKD CI signature env vars
  fetch-blobs.sh     helper to stage blobs into a local directory

The --version flag selects the OKD release. --release-digest must supply
the quay.io/okd/scos-release image digest for that version (obtain it via
"oc adm release info quay.io/okd/scos-release:<version> --output=jsonpath='{.digest}'").`,
	RunE: runAirgapPlan,
}

func init() {
	airgapPlanCmd.Flags().StringVar(&airgapPlanVersion, "version", "",
		"OKD version to plan for (e.g. 4.21.0-okd-scos.10)")
	airgapPlanCmd.Flags().StringVar(&airgapPlanDigest, "release-digest", "",
		"digest of quay.io/okd/scos-release:<version> (sha256:<hex>)")
	airgapPlanCmd.Flags().StringVar(&airgapPlanChannel, "channel", "",
		"when set, emit graph-based ISC for this channel (e.g. stable-4.21)")
	airgapPlanCmd.Flags().StringVar(&airgapPlanOutDir, "out-dir", "airgap",
		"directory to write artifacts into (created if absent)")
	airgapPlanCmd.Flags().StringVar(&airgapPlanStreamJSONPath, "stream-json", "",
		"path to a local scos.json/fcos.json file (skips network fetch; useful offline)")
	_ = airgapPlanCmd.MarkFlagRequired("version")
	_ = airgapPlanCmd.MarkFlagRequired("release-digest")

	airgapCmd.AddCommand(airgapPlanCmd)
	rootCmd.AddCommand(airgapCmd)
}

func runAirgapPlan(cmd *cobra.Command, _ []string) error {
	if airgapPlanVersion == "" {
		return &errtypes.ConfigError{Msg: "--version is required"}
	}
	if airgapPlanDigest == "" {
		return &errtypes.ConfigError{Msg: "--release-digest is required; obtain via: oc adm release info quay.io/okd/scos-release:<version> --output=jsonpath='{.digest}'"}
	}

	outDir := airgapPlanOutDir
	if outDir == "" {
		outDir = "airgap"
	}
	if err := system.EnsureDir(outDir); err != nil {
		return fmt.Errorf("create output directory %s: %w", outDir, err)
	}

	minor := airgapMinor(airgapPlanVersion)
	var fetcher streamFetcher = httpStreamFetcher{}
	if airgapPlanStreamJSONPath != "" {
		fetcher = fileStreamFetcher{path: airgapPlanStreamJSONPath}
	}

	p, err := buildAirgapPlan(cmd.Context(), airgapPlanVersion, minor, fetcher)
	if err != nil {
		return err
	}

	if wErr := writeAirgapArtifacts(outDir, airgapPlanVersion, airgapPlanDigest, airgapPlanChannel, p); wErr != nil {
		return wErr
	}

	fmt.Fprintf(cmd.OutOrStdout(), "artifacts written to %s/\n", outDir)
	return nil
}

// streamFetcher abstracts the CoreOS stream JSON fetch so tests can inject
// canned data without network access.
type streamFetcher interface {
	fetch(ctx context.Context, minor int) (*airgapStreamData, error)
}

type airgapDiskData struct {
	Location string `json:"location"`
	SHA256   string `json:"sha256"`
}

type airgapFormatsData struct {
	ISO struct {
		Disk airgapDiskData `json:"disk"`
	} `json:"iso"`
}

type airgapMetalData struct {
	Release string            `json:"release"`
	Formats airgapFormatsData `json:"formats"`
}

type airgapArchData struct {
	Artifacts struct {
		Metal airgapMetalData `json:"metal"`
	} `json:"artifacts"`
}

// airgapStreamData is the subset of scos.json/fcos.json the planner needs.
type airgapStreamData struct {
	Architectures map[string]airgapArchData `json:"architectures"`
}

type httpStreamFetcher struct{}

func (httpStreamFetcher) fetch(ctx context.Context, minor int) (*airgapStreamData, error) {
	streamURL := fetchplan.BuildCoreOSStreamPlan(minor).HTTPS[0].URL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, http.NoBody)
	if err != nil {
		return nil, &errtypes.NetworkError{Msg: "build coreos stream request", Err: err}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, &errtypes.NetworkError{Msg: "fetch coreos stream", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &errtypes.NetworkError{Msg: fmt.Sprintf("coreos stream HTTP %d", resp.StatusCode)}
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if readErr != nil {
		return nil, &errtypes.NetworkError{Msg: "read coreos stream body", Err: readErr}
	}
	var sd airgapStreamData
	if jsonErr := json.Unmarshal(body, &sd); jsonErr != nil {
		return nil, &errtypes.NetworkError{Msg: "parse coreos stream JSON", Err: jsonErr}
	}
	return &sd, nil
}

type fileStreamFetcher struct{ path string }

func (f fileStreamFetcher) fetch(_ context.Context, _ int) (*airgapStreamData, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("read stream JSON %s", f.path), Err: err}
	}
	var sd airgapStreamData
	if jsonErr := json.Unmarshal(data, &sd); jsonErr != nil {
		return nil, &errtypes.ConfigError{Msg: "parse stream JSON", Err: jsonErr}
	}
	return &sd, nil
}

// airgapBlob is one entry in the airgap.yaml blobs list.
type airgapBlob struct {
	Purpose    string `json:"purpose"`
	URL        string `json:"url"`
	SHA256     string `json:"sha256"`
	MirrorPath string `json:"mirror_path"`
}

func buildAirgapPlan(ctx context.Context, _ string, minor int, fetcher streamFetcher) ([]airgapBlob, error) {
	in := fetchplan.ResolveM5Input("amd64", nil)
	m5 := fetchplan.BuildM5Plan(&in)
	oc := fetchplan.BuildM22BootstrapOCPlan()

	all := make([]fetchplan.Blob, 0, len(m5.HTTPS)+len(oc.HTTPS)+1)
	all = append(all, m5.HTTPS...)
	all = append(all, oc.HTTPS...)

	sd, err := fetcher.fetch(ctx, minor)
	if err != nil {
		return nil, err
	}
	isoBlob, isoErr := isoFromStream(sd)
	if isoErr != nil {
		return nil, isoErr
	}

	all = append(all, isoBlob)

	blobs := make([]airgapBlob, 0, len(all))
	for _, b := range all {
		blobs = append(blobs, airgapBlob{
			Purpose:    b.Purpose,
			URL:        b.URL,
			SHA256:     blobSHA(b),
			MirrorPath: mirrorPath(b.URL),
		})
	}
	return blobs, nil
}

// blobSHA returns the blob's known SHA256 or the sentinel "<tbd>" when
// no checksum is pre-computed. Real checksums require a live fetch from
// per-tool sidecar checksum URLs; that is deferred to a follow-up.
func blobSHA(b fetchplan.Blob) string {
	if b.SHA256 != "" {
		return b.SHA256
	}
	return "<tbd>"
}

// mirrorPath derives the operator-facing staging path by stripping the
// upstream host and prepending the MirrorResolver prefix from L15 §6.1.
func mirrorPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	prefixTable := map[string]string{
		"get.helm.sh":                "helm",
		"github.com":                 "github",
		"api.github.com":             "okdctl-api",
		"rhcos.mirror.openshift.com": "rhcos",
		"raw.githubusercontent.com":  "raw-github",
		"mirror.openshift.com":       "openshift-mirror",
	}
	prefix, ok := prefixTable[u.Host]
	if !ok {
		return strings.TrimLeft(u.Path, "/")
	}
	return prefix + u.Path
}

// isoFromStream extracts the x86_64 ISO URL and SHA256 from stream data.
func isoFromStream(sd *airgapStreamData) (fetchplan.Blob, error) {
	arch, ok := sd.Architectures["x86_64"]
	if !ok {
		return fetchplan.Blob{}, &errtypes.ConfigError{Msg: "x86_64 not found in CoreOS stream data"}
	}
	iso := arch.Artifacts.Metal.Formats.ISO.Disk
	if iso.Location == "" {
		return fetchplan.Blob{}, &errtypes.ConfigError{Msg: "iso location not found in CoreOS stream data"}
	}
	return fetchplan.Blob{
		URL:     iso.Location,
		SHA256:  iso.SHA256,
		Purpose: "scos-iso",
	}, nil
}

// airgapMinor parses the minor version from an OKD version string like
// "4.21.0-okd-scos.10". Returns 0 when the version does not match.
func airgapMinor(version string) int {
	var major, minor int
	_, _ = fmt.Sscanf(version, "%d.%d", &major, &minor)
	return minor
}

func writeAirgapArtifacts(outDir, version, digest, channel string, blobs []airgapBlob) error {
	iscBytes := renderISC(version, digest, channel)
	if wErr := system.AtomicWrite(filepath.Join(outDir, "isc.yaml"), iscBytes, 0o644); wErr != nil {
		return fmt.Errorf("write isc.yaml: %w", wErr)
	}

	airgapBytes := renderAirgapYAML(version, blobs)
	if wErr := system.AtomicWrite(filepath.Join(outDir, "airgap.yaml"), airgapBytes, 0o644); wErr != nil {
		return fmt.Errorf("write airgap.yaml: %w", wErr)
	}

	ocMirrorSh := renderOCMirrorSh(outDir)
	if wErr := system.AtomicWrite(filepath.Join(outDir, "run-oc-mirror.sh"), []byte(ocMirrorSh), 0o755); wErr != nil {
		return fmt.Errorf("write run-oc-mirror.sh: %w", wErr)
	}

	fetchSh := renderFetchBlobsSh(outDir)
	if wErr := system.AtomicWrite(filepath.Join(outDir, "fetch-blobs.sh"), []byte(fetchSh), 0o755); wErr != nil {
		return fmt.Errorf("write fetch-blobs.sh: %w", wErr)
	}

	return nil
}

func renderISC(version, digest, channel string) []byte {
	if channel != "" {
		return fmt.Appendf(nil, `apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  platform:
    graph: true
    architectures:
      - amd64
    channels:
      - name: %s
        type: okd
        minVersion: %s
        maxVersion: %s
`, channel, version, version)
	}

	return fmt.Appendf(nil, `apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  platform:
    release: quay.io/okd/scos-release@%s
    architectures:
      - amd64
  additionalImages: []
  helm:
    repositories: []
`, digest)
}

func renderAirgapYAML(version string, blobs []airgapBlob) []byte {
	var sb strings.Builder
	sb.WriteString("apiVersion: okdctl/v1\n")
	sb.WriteString("kind: AirgapBlobs\n")
	fmt.Fprintf(&sb, "version: %s\n", version)
	sb.WriteString("blobs:\n")
	for _, b := range blobs {
		fmt.Fprintf(&sb, "  - purpose: %s\n", b.Purpose)
		fmt.Fprintf(&sb, "    url: %s\n", b.URL)
		fmt.Fprintf(&sb, "    sha256: %s\n", b.SHA256)
		fmt.Fprintf(&sb, "    mirror_path: %s\n", b.MirrorPath)
	}
	return []byte(sb.String())
}

func renderOCMirrorSh(outDir string) string {
	iscPath := filepath.Join(outDir, "isc.yaml")
	return fmt.Sprintf(`#!/bin/sh
# run-oc-mirror.sh — generated by okdctl airgap plan
# Edit OCP_SIGNATURE_VERIFICATION_PK to point at the OKD CI public key file.
export OCP_SIGNATURE_URL=https://storage.googleapis.com/openshift-ci-release/releases/signatures/openshift/release/
export OCP_SIGNATURE_VERIFICATION_PK=/path/to/openshift-ci-4-verifier-pk
exec oc-mirror --v2 -c %s file:///mirror
`, iscPath)
}

func renderFetchBlobsSh(outDir string) string {
	airgapYAML := filepath.Join(outDir, "airgap.yaml")
	return fmt.Sprintf(`#!/bin/sh
# fetch-blobs.sh — generated by okdctl airgap plan
# Stages every blob listed in airgap.yaml into ./blobs/<mirror_path>.
set -eu
STAGING="${1:-./blobs}"
AIRGAP_YAML=%s
python3 - "$STAGING" "$AIRGAP_YAML" <<'PYEOF'
import sys, urllib.request, pathlib, yaml as _yaml
staging = pathlib.Path(sys.argv[1])
blobs = _yaml.safe_load(open(sys.argv[2]))["blobs"]
for b in blobs:
    dest = staging / b["mirror_path"]
    dest.parent.mkdir(parents=True, exist_ok=True)
    if dest.exists():
        print(f"skip (exists): {dest}")
        continue
    print(f"fetch: {b['url']} -> {dest}")
    urllib.request.urlretrieve(b["url"], dest)
print("done")
PYEOF
`, airgapYAML)
}

// Package fetchplan declares every external artifact okdctl reaches for
// during deploy as typed data, and routes resolution through a Resolver
// so air-gap mirror rewrites (M24) can be applied uniformly. The current
// release ships M4 (OKD release tarballs), M5 (helm/sops/yq), and M23
// (scos.json stream metadata) plan builders; OCI source kinds are
// declared but unwired pending M22.
package fetchplan

import (
	"fmt"
	"os"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
)

// Plan is the set of remote artifacts a phase needs to fetch.
type Plan struct {
	OCI   []OCIArtifact
	HTTPS []Blob
}

// OCIArtifact describes one OCI image or chart reference. ExtractVia names
// the extractor a downstream caller should use ("oc-adm-release-extract",
// "helm-pull"); empty when the resolver only returns the ref.
type OCIArtifact struct {
	Ref        string
	Digest     string
	ExtractVia string
	Purpose    string
}

// Blob describes one HTTPS artifact. SHA256 is required for blobs whose
// upstream publishes a checksum; empty means the caller verifies elsewhere
// (e.g. via a sidecar checksums file).
type Blob struct {
	URL     string
	SHA256  string
	Purpose string
}

// Resolver maps Plan entries to the URLs that should actually be fetched.
// Implementations must be safe for concurrent use.
type Resolver interface {
	ResolveOCI(a OCIArtifact) (string, error)
	ResolveBlob(b Blob) (string, error)
}

// DefaultResolver returns upstream refs and URLs unchanged.
type DefaultResolver struct{}

// ResolveOCI returns a.Ref unchanged.
func (DefaultResolver) ResolveOCI(a OCIArtifact) (string, error) { return a.Ref, nil }

// ResolveBlob returns b.URL unchanged.
func (DefaultResolver) ResolveBlob(b Blob) (string, error) { return b.URL, nil }

// MirrorResolver rewrites fetches through a mirror base. The rewrite
// table arrives in M24; until then MirrorBase is stored but resolution
// returns the upstream value unchanged so the type composes cleanly with
// the rest of the air-gap workstream.
type MirrorResolver struct {
	MirrorBase string
}

// ResolveOCI returns a.Ref unchanged pending M24's rewrite table.
func (r MirrorResolver) ResolveOCI(a OCIArtifact) (string, error) { return a.Ref, nil }

// ResolveBlob returns b.URL unchanged pending M24's rewrite table.
func (r MirrorResolver) ResolveBlob(b Blob) (string, error) { return b.URL, nil }

// Purpose tags identify Plan entries across the M4/M5 workstream so
// callers can pick the entry they need without index magic.
const (
	M4Purpose              = "okd-release"
	M5PurposeHelm          = "tool-helm"
	M5PurposeSops          = "tool-sops"
	M5PurposeYQ            = "tool-yq"
	M22PurposeBootstrapOC  = "bootstrap-oc"
	M23PurposeCoreOSStream = "coreos-stream"
)

const (
	defaultOKDReleaseBaseURL = "https://github.com/okd-project/okd/releases/download"

	defaultHelmVersion = "v3.17.3"
	defaultSopsVersion = "v3.9.4"

	helmURLTemplate     = "https://get.helm.sh/helm-{version}-linux-{arch}.tar.gz"
	sopsURLTemplate     = "https://github.com/getsops/sops/releases/download/{version}/sops-{version}.linux.{arch}"
	yqVersionedTemplate = "https://github.com/mikefarah/yq/releases/download/{version}/yq_linux_{arch}"

	// yqLatestRedirect is GitHub's "latest release" redirect; it resolves
	// to the current yq tag without a version pin. M5's original URL used
	// this and silently no-op'd OKDCTL_YQ_VERSION; the gap is now closed
	// by switching to yqVersionedTemplate when a version is configured.
	yqLatestRedirect = "https://github.com/mikefarah/yq/releases/latest/download/yq_linux_{arch}"
)

// ResolveM4BaseURL returns the OKD release base URL applying the standard
// precedence: OKDCTL_OKD_RELEASE_URL env > cfg.Deployment.OKDReleaseBaseURL >
// upstream GitHub default. Trailing slashes are stripped.
func ResolveM4BaseURL(cfg *config.Config) string {
	if v := os.Getenv("OKDCTL_OKD_RELEASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	if cfg != nil && cfg.Deployment.OKDReleaseBaseURL != "" {
		return strings.TrimRight(cfg.Deployment.OKDReleaseBaseURL, "/")
	}
	return defaultOKDReleaseBaseURL
}

// M4Input carries the resolved parameters for BuildM4Plan.
type M4Input struct {
	BaseURL string
	Version string
	Arch    string
}

// BuildM4Plan returns the three OKD release Blobs (sha256sum.txt and the
// two tool tarballs) for the given input. Callers typically obtain BaseURL
// via ResolveM4BaseURL.
func BuildM4Plan(in M4Input) Plan {
	base := fmt.Sprintf("%s/%s", strings.TrimRight(in.BaseURL, "/"), in.Version)
	return Plan{
		HTTPS: []Blob{
			{URL: fmt.Sprintf("%s/sha256sum.txt", base), Purpose: M4Purpose},
			{URL: fmt.Sprintf("%s/openshift-install-linux-%s.tar.gz", base, in.Version), Purpose: M4Purpose},
			{URL: fmt.Sprintf("%s/openshift-client-linux-%s.tar.gz", base, in.Version), Purpose: M4Purpose},
		},
	}
}

// M5Input carries the resolved parameters for BuildM5Plan.
type M5Input struct {
	Arch string

	HelmURL     string
	HelmVersion string

	SopsURL     string
	SopsVersion string

	// YQVersion is empty when no override is configured; in that case the
	// latest-redirect URL is used. Setting YQVersion (env or config) flips
	// the resolver onto a versioned URL — closes the M5 yq gap.
	YQURL     string
	YQVersion string
}

// ResolveM5Input constructs an M5Input applying env > cfg.Deployment.ToolVersions
// > default precedence. YQVersion intentionally has no default so the
// latest-redirect URL stays the fallback for unconfigured deploys.
func ResolveM5Input(arch string, cfg *config.Config) M5Input {
	in := M5Input{
		Arch:        arch,
		HelmVersion: defaultHelmVersion,
		SopsVersion: defaultSopsVersion,
	}

	applyConfigOverride(&in.HelmURL, &in.HelmVersion, cfg, "helm")
	applyConfigOverride(&in.SopsURL, &in.SopsVersion, cfg, "sops")
	applyConfigOverride(&in.YQURL, &in.YQVersion, cfg, "yq")

	applyEnvOverride(&in.HelmURL, "OKDCTL_HELM_URL")
	applyEnvOverride(&in.HelmVersion, "OKDCTL_HELM_VERSION")
	applyEnvOverride(&in.SopsURL, "OKDCTL_SOPS_URL")
	applyEnvOverride(&in.SopsVersion, "OKDCTL_SOPS_VERSION")
	applyEnvOverride(&in.YQURL, "OKDCTL_YQ_URL")
	applyEnvOverride(&in.YQVersion, "OKDCTL_YQ_VERSION")

	return in
}

func applyConfigOverride(url, version *string, cfg *config.Config, tool string) {
	if cfg == nil {
		return
	}
	ov, ok := cfg.Deployment.ToolVersions[tool]
	if !ok {
		return
	}
	if ov.URLTemplate != "" {
		*url = ov.URLTemplate
	}
	if ov.Version != "" {
		*version = ov.Version
	}
}

func applyEnvOverride(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

// BuildM5Plan returns Blobs for helm, sops, and yq with placeholders
// expanded. URL overrides (from env/config) are used verbatim except for
// {version}/{arch} substitution, matching pre-M21 behaviour.
func BuildM5Plan(in *M5Input) Plan {
	helmURL := in.HelmURL
	if helmURL == "" {
		helmURL = helmURLTemplate
	}
	helmURL = expandTemplate(helmURL, in.HelmVersion, in.Arch)

	sopsURL := in.SopsURL
	if sopsURL == "" {
		sopsURL = sopsURLTemplate
	}
	sopsURL = expandTemplate(sopsURL, in.SopsVersion, in.Arch)

	yqURL := in.YQURL
	if yqURL == "" {
		if in.YQVersion != "" {
			yqURL = yqVersionedTemplate
		} else {
			yqURL = yqLatestRedirect
		}
	}
	yqURL = expandTemplate(yqURL, in.YQVersion, in.Arch)

	return Plan{
		HTTPS: []Blob{
			{URL: helmURL, Purpose: M5PurposeHelm},
			{URL: sopsURL, Purpose: M5PurposeSops},
			{URL: yqURL, Purpose: M5PurposeYQ},
		},
	}
}

func expandTemplate(tmpl, version, arch string) string {
	return strings.NewReplacer("{version}", version, "{arch}", arch).Replace(tmpl)
}

// bootstrapOCURL is the mirror.openshift.com path for the universal oc client
// used to run `oc adm release extract`. No upstream checksum is published for
// this URL; post-extraction binary-exists verification is the integrity gate.
// All final binaries come from the digest-pinned release image.
const bootstrapOCURL = "https://mirror.openshift.com/pub/openshift-v4/clients/oc/latest/linux/oc.tar.gz"

// BuildM22BootstrapOCPlan returns a Plan with the bootstrap oc Blob.
// The URL is routed through the caller's Resolver so MirrorResolver
// (M24) can redirect it via OKDCTL_BOOTSTRAP_OC_URL.
func BuildM22BootstrapOCPlan() Plan {
	return Plan{
		HTTPS: []Blob{{
			URL:     bootstrapOCURL,
			Purpose: M22PurposeBootstrapOC,
		}},
	}
}

// OKDReleaseImageRef builds an OCIArtifact for the given OKD version tag.
// Pass a non-empty digest to produce a digest-pinned ref from the GitHub
// release body's "Pull From:" line; pass "" for a tag-only ref.
func OKDReleaseImageRef(version, digest string) OCIArtifact {
	ref := "quay.io/okd/scos-release:" + version
	if digest != "" {
		ref = "quay.io/okd/scos-release@" + digest
	}
	return OCIArtifact{
		Ref:        ref,
		Digest:     digest,
		ExtractVia: "oc-adm-release-extract",
		Purpose:    M4Purpose,
	}
}

// coreOSStreamRawBase is the GitHub raw-content root for openshift/installer.
const coreOSStreamRawBase = "https://raw.githubusercontent.com/openshift/installer"

// minSCOSStreamMinor is the first OKD minor that publishes scos.json
// (Stream CoreOS); 4.15-4.18 ship Fedora CoreOS via fcos.json.
const minSCOSStreamMinor = 19

// BuildCoreOSStreamPlan returns a Plan with the upstream CoreOS stream
// metadata Blob for the given OKD minor: fcos.json for 4.15-4.18,
// scos.json for 4.19+. The URL is pinned to a release-4.<minor> branch so
// MirrorResolver (M24) can rewrite it uniformly with other plan entries.
func BuildCoreOSStreamPlan(minor int) Plan {
	file := "fcos.json"
	if minor >= minSCOSStreamMinor {
		file = "scos.json"
	}
	return Plan{
		HTTPS: []Blob{{
			URL:     fmt.Sprintf("%s/release-4.%d/data/data/coreos/%s", coreOSStreamRawBase, minor, file),
			Purpose: M23PurposeCoreOSStream,
		}},
	}
}

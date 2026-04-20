// Package fetchplan declares every external artifact okdctl reaches for
// during deploy as typed data, and routes resolution through a Resolver
// so air-gap mirror rewrites (M24) can be applied uniformly. Plan builders
// cover M5 tool binaries (helm/sops/yq), M22 bootstrap-oc and OKD release
// image, and M23 CoreOS stream metadata.
package fetchplan

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
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

// MirrorResolver rewrites fetches through a mirror base using the 1:1
// host-prefix table from L15 §6.1. Unrecognised upstream hosts pass through
// unchanged so future fetch sites don't silently break.
type MirrorResolver struct {
	MirrorBase string
}

// mirrorBlobRules maps upstream HTTPS host to the path prefix appended under
// MirrorBase. Covers the L15 §6.1 table plus raw.githubusercontent.com
// (coreos stream) and mirror.openshift.com (bootstrap-oc fetch).
var mirrorBlobRules = map[string]string{
	"get.helm.sh":                "helm",
	"github.com":                 "github",
	"api.github.com":             "okdctl-api",
	"rhcos.mirror.openshift.com": "rhcos",
	"raw.githubusercontent.com":  "raw-github",
	"mirror.openshift.com":       "openshift-mirror",
}

// mirrorOCIRules maps upstream OCI registry host to the path prefix under
// MirrorBase. The rewritten ref uses the MirrorBase host (scheme stripped).
var mirrorOCIRules = map[string]string{
	"quay.io":                   "quay",
	"ghcr.io":                   "ghcr",
	"registry.ci.openshift.org": "registry-ci",
}

// ResolveOCI rewrites an OCI image reference through the mirror base. The
// upstream registry host is replaced by the MirrorBase host-and-path with a
// fixed prefix (e.g. quay.io/okd/… → <mirror-host>/<base-path>/quay/okd/…).
// Refs whose registry is not in the rewrite table are returned unchanged.
func (r MirrorResolver) ResolveOCI(a OCIArtifact) (string, error) {
	ref := a.Ref
	slash := strings.IndexByte(ref, '/')
	if slash < 0 {
		return ref, nil
	}
	host := ref[:slash]
	rest := ref[slash+1:]
	prefix, ok := mirrorOCIRules[host]
	if !ok {
		return ref, nil
	}
	base, err := parseMirrorBase(r.MirrorBase)
	if err != nil {
		return "", err
	}
	return base.Host + base.Path + "/" + prefix + "/" + rest, nil
}

// ResolveBlob rewrites an HTTPS blob URL through the mirror base. The
// upstream host is replaced with MirrorBase (host + path) and a fixed prefix
// (e.g. get.helm.sh/… → <base>/helm/…). URLs whose host is not in the
// rewrite table are returned unchanged.
func (r MirrorResolver) ResolveBlob(b Blob) (string, error) {
	u, parseErr := url.Parse(b.URL)
	if parseErr != nil || u.Host == "" {
		return b.URL, nil //nolint:nilerr // unparseable URL falls through unchanged
	}
	prefix, ok := mirrorBlobRules[u.Host]
	if !ok {
		return b.URL, nil
	}
	base, err := parseMirrorBase(r.MirrorBase)
	if err != nil {
		return "", err
	}
	rewritten := *base
	rewritten.Path = base.Path + "/" + prefix + u.Path
	rewritten.RawQuery = u.RawQuery
	return rewritten.String(), nil
}

// parseMirrorBase validates and parses the MirrorBase URL. Returns a
// *ConfigError when the value is empty, unparseable, or scheme/host are absent.
// A trailing slash on the path is trimmed; any other path is preserved so
// operators can point at a sub-directory mirror (https://mirror.local/okdctl).
func parseMirrorBase(base string) (*url.URL, error) {
	if base == "" {
		return nil, &errtypes.ConfigError{Msg: "MirrorBase is not set"}
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, &errtypes.ConfigError{Msg: "MirrorBase must be an absolute URL (e.g. https://mirror.local)"}
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u, nil
}

// ResolveMirrorBase returns the effective MirrorBase applying env > config
// precedence: OKDCTL_MIRROR_BASE env var, then cfg.Deployment.MirrorBase.
// Returns empty string when neither is set.
func ResolveMirrorBase(cfg *config.Config) string {
	if v := os.Getenv("OKDCTL_MIRROR_BASE"); v != "" {
		return strings.TrimRight(v, "/")
	}
	if cfg != nil && cfg.Deployment.MirrorBase != "" {
		return strings.TrimRight(cfg.Deployment.MirrorBase, "/")
	}
	return ""
}

// IsAirgap reports whether air-gap mode is active for this invocation. Any
// of OKDCTL_AIRGAP=1, airgapFlag=true, or a non-empty MirrorBase (env or
// config) activates air-gap mode.
func IsAirgap(cfg *config.Config, airgapFlag bool) bool {
	if os.Getenv("OKDCTL_AIRGAP") == "1" {
		return true
	}
	if airgapFlag {
		return true
	}
	return ResolveMirrorBase(cfg) != ""
}

// PickResolver composes the active resolver chain: Default or Mirror as
// the inner resolver, wrapped by EnvOverrideResolver with the standard
// escape-hatch map. Returns a *ConfigError when air-gap is active but
// MirrorBase is not configured.
func PickResolver(cfg *config.Config, airgapFlag bool) (Resolver, error) {
	var inner Resolver
	if IsAirgap(cfg, airgapFlag) {
		base := ResolveMirrorBase(cfg)
		if base == "" {
			return nil, &errtypes.ConfigError{Msg: "air-gap mode is active but OKDCTL_MIRROR_BASE (or deployment.mirror_base) is not set"}
		}
		inner = MirrorResolver{MirrorBase: base}
	} else {
		inner = DefaultResolver{}
	}
	return EnvOverrideResolver{Inner: inner, Overrides: DefaultEnvOverrides()}, nil
}

// Purpose tags identify Plan entries so callers can pick what they need
// without index magic.
const (
	PurposeOKDRelease   = "okd-release"
	PurposeHelm         = "tool-helm"
	PurposeSops         = "tool-sops"
	PurposeYQ           = "tool-yq"
	PurposeBootstrapOC  = "bootstrap-oc"
	PurposeCoreOSStream = "coreos-stream"
	PurposeCoreOSISO    = "coreos-iso"
	PurposeUpdateCheck  = "update-check"
	PurposeAddonChart   = "addon-chart"
)

// Per-fetch env-var override keys (L15 §6.2 — final escape hatches).
// Values in these vars take precedence over MirrorBase rewrites.
const (
	EnvUpdateCheckURL = "OKDCTL_UPDATE_CHECK_URL"
	EnvSCOSStreamURL  = "OKDCTL_SCOS_STREAM_URL"
	EnvSCOSISOURL     = "OKDCTL_SCOS_ISO_URL"
	EnvBootstrapOCURL = "OKDCTL_BOOTSTRAP_OC_URL"
)

// EnvOverrideResolver wraps an inner Resolver and consults Overrides
// (a Purpose → env-var-name map) before delegating. When the named env
// var is non-empty its value is returned directly, bypassing the inner
// resolver and any MirrorBase rewrite.
type EnvOverrideResolver struct {
	Inner     Resolver
	Overrides map[string]string
}

// ResolveOCI checks Overrides for a.Purpose before delegating to Inner.
func (r EnvOverrideResolver) ResolveOCI(a OCIArtifact) (string, error) {
	if envKey, ok := r.Overrides[a.Purpose]; ok {
		if v := os.Getenv(envKey); v != "" {
			return v, nil
		}
	}
	return r.Inner.ResolveOCI(a)
}

// ResolveBlob checks Overrides for b.Purpose before delegating to Inner.
func (r EnvOverrideResolver) ResolveBlob(b Blob) (string, error) {
	if envKey, ok := r.Overrides[b.Purpose]; ok {
		if v := os.Getenv(envKey); v != "" {
			return v, nil
		}
	}
	return r.Inner.ResolveBlob(b)
}

// DefaultEnvOverrides returns the standard Purpose → env-var-name map
// covering the four L15 §6.2 escape hatches.
func DefaultEnvOverrides() map[string]string {
	return map[string]string{
		PurposeUpdateCheck:  EnvUpdateCheckURL,
		PurposeCoreOSStream: EnvSCOSStreamURL,
		PurposeCoreOSISO:    EnvSCOSISOURL,
		PurposeBootstrapOC:  EnvBootstrapOCURL,
	}
}

const (
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

func applyConfigOverride(urlPtr, version *string, cfg *config.Config, tool string) {
	if cfg == nil {
		return
	}
	ov, ok := cfg.Deployment.ToolVersions[tool]
	if !ok {
		return
	}
	if ov.URLTemplate != "" {
		*urlPtr = ov.URLTemplate
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
			{URL: helmURL, Purpose: PurposeHelm},
			{URL: sopsURL, Purpose: PurposeSops},
			{URL: yqURL, Purpose: PurposeYQ},
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
// The URL is routed through the caller's Resolver so EnvOverrideResolver
// can redirect it via OKDCTL_BOOTSTRAP_OC_URL.
func BuildM22BootstrapOCPlan() Plan {
	return Plan{
		HTTPS: []Blob{{
			URL:     bootstrapOCURL,
			Purpose: PurposeBootstrapOC,
		}},
	}
}

// BuildCoreOSISOPlan returns a Plan with the CoreOS ISO Blob. isoURL is
// the location field from the CoreOS stream JSON; sha256 is the matching
// checksum (empty when the caller verifies separately).
func BuildCoreOSISOPlan(isoURL, sha256 string) Plan {
	return Plan{
		HTTPS: []Blob{{
			URL:     isoURL,
			SHA256:  sha256,
			Purpose: PurposeCoreOSISO,
		}},
	}
}

// defaultUpdateCheckURL is the GitHub releases API endpoint for okdctl.
const defaultUpdateCheckURL = "https://api.github.com/repos/qxtaiba/okdctl/releases/latest"

// BuildUpdateCheckPlan returns a Plan for the okdctl update-check endpoint.
func BuildUpdateCheckPlan() Plan {
	return Plan{
		HTTPS: []Blob{{
			URL:     defaultUpdateCheckURL,
			Purpose: PurposeUpdateCheck,
		}},
	}
}

// BuildAddonChartPlan returns a Plan with the OCI chart reference. ref is
// the bare registry path without an oci:// scheme; callers that require the
// scheme must re-prepend it after resolution.
func BuildAddonChartPlan(ref string) Plan {
	return Plan{
		OCI: []OCIArtifact{{
			Ref:     ref,
			Purpose: PurposeAddonChart,
		}},
	}
}

// OKDReleaseImageRef builds an OCIArtifact pointing at the upstream OKD
// release image for the given version tag. The trust anchor is TLS to
// quay.io plus oc's manifest pinning at extract time.
func OKDReleaseImageRef(version string) OCIArtifact {
	return OCIArtifact{
		Ref:        "quay.io/okd/scos-release:" + version,
		ExtractVia: "oc-adm-release-extract",
		Purpose:    PurposeOKDRelease,
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
			Purpose: PurposeCoreOSStream,
		}},
	}
}

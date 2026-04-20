package fetchplan_test

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/fetchplan"
)

func TestDefaultResolver_passthrough(t *testing.T) {
	r := fetchplan.DefaultResolver{}
	const url = "https://example.com/file.tar.gz"
	got, err := r.ResolveBlob(fetchplan.Blob{URL: url, Purpose: "test"})
	if err != nil || got != url {
		t.Fatalf("ResolveBlob: got (%q,%v), want (%q,nil)", got, err, url)
	}
	const ref = "ghcr.io/example/image:latest"
	got, err = r.ResolveOCI(fetchplan.OCIArtifact{Ref: ref, Purpose: "test"})
	if err != nil || got != ref {
		t.Fatalf("ResolveOCI: got (%q,%v), want (%q,nil)", got, err, ref)
	}
}

func TestMirrorResolver_blobRewrite(t *testing.T) {
	r := fetchplan.MirrorResolver{MirrorBase: "https://mirror.corp"}
	cases := []struct {
		url  string
		want string
	}{
		{
			"https://get.helm.sh/helm-v3.17.3-linux-amd64.tar.gz",
			"https://mirror.corp/helm/helm-v3.17.3-linux-amd64.tar.gz",
		},
		{
			"https://rhcos.mirror.openshift.com/art/storage/scos.iso",
			"https://mirror.corp/rhcos/art/storage/scos.iso",
		},
		{
			"https://raw.githubusercontent.com/openshift/installer/release-4.21/data/data/coreos/scos.json",
			"https://mirror.corp/raw-github/openshift/installer/release-4.21/data/data/coreos/scos.json",
		},
		{
			"https://mirror.openshift.com/pub/openshift-v4/clients/oc/latest/linux/oc.tar.gz",
			"https://mirror.corp/openshift-mirror/pub/openshift-v4/clients/oc/latest/linux/oc.tar.gz",
		},
	}
	for _, tc := range cases {
		got, err := r.ResolveBlob(fetchplan.Blob{URL: tc.url})
		if err != nil {
			t.Fatalf("ResolveBlob(%q): unexpected error: %v", tc.url, err)
		}
		if got != tc.want {
			t.Errorf("ResolveBlob(%q)\n  got  %q\n  want %q", tc.url, got, tc.want)
		}
	}
}

func TestMirrorResolver_blobUnknownHostPassthrough(t *testing.T) {
	r := fetchplan.MirrorResolver{MirrorBase: "https://mirror.corp"}
	const unknownURL = "https://unknown-host.example.com/file.tar.gz"
	got, err := r.ResolveBlob(fetchplan.Blob{URL: unknownURL})
	if err != nil || got != unknownURL {
		t.Fatalf("unknown host should pass through unchanged; got (%q,%v)", got, err)
	}
}

func TestMirrorResolver_ociRewrite(t *testing.T) {
	r := fetchplan.MirrorResolver{MirrorBase: "https://mirror.corp"}
	cases := []struct {
		ref  string
		want string
	}{
		{"quay.io/okd/scos-release:4.21.0", "mirror.corp/quay/okd/scos-release:4.21.0"},
		{"ghcr.io/controlplaneio-fluxcd/charts/flux-operator:2.4.0", "mirror.corp/ghcr/controlplaneio-fluxcd/charts/flux-operator:2.4.0"},
		{"registry.ci.openshift.org/origin/release-scos@sha256:abc", "mirror.corp/registry-ci/origin/release-scos@sha256:abc"},
	}
	for _, tc := range cases {
		got, err := r.ResolveOCI(fetchplan.OCIArtifact{Ref: tc.ref})
		if err != nil {
			t.Fatalf("ResolveOCI(%q): unexpected error: %v", tc.ref, err)
		}
		if got != tc.want {
			t.Errorf("ResolveOCI(%q)\n  got  %q\n  want %q", tc.ref, got, tc.want)
		}
	}
}

func TestMirrorResolver_honoursBasePath(t *testing.T) {
	r := fetchplan.MirrorResolver{MirrorBase: "https://mirror.corp/okdctl"}
	blob, err := r.ResolveBlob(fetchplan.Blob{URL: "https://get.helm.sh/helm.tar.gz"})
	if err != nil {
		t.Fatalf("ResolveBlob: %v", err)
	}
	if blob != "https://mirror.corp/okdctl/helm/helm.tar.gz" {
		t.Errorf("MirrorBase path not honoured for blob; got %q", blob)
	}
	oci, err := r.ResolveOCI(fetchplan.OCIArtifact{Ref: "quay.io/okd/scos-release:4.21"})
	if err != nil {
		t.Fatalf("ResolveOCI: %v", err)
	}
	if oci != "mirror.corp/okdctl/quay/okd/scos-release:4.21" {
		t.Errorf("MirrorBase path not honoured for OCI; got %q", oci)
	}
}

func TestMirrorResolver_badBase(t *testing.T) {
	r := fetchplan.MirrorResolver{MirrorBase: "not-a-url"}
	_, err := r.ResolveBlob(fetchplan.Blob{URL: "https://get.helm.sh/helm.tar.gz"})
	if err == nil {
		t.Fatal("expected error for malformed MirrorBase, got nil")
	}
}

func TestPickResolver_default(t *testing.T) {
	t.Setenv("OKDCTL_AIRGAP", "")
	t.Setenv("OKDCTL_MIRROR_BASE", "")
	r, err := fetchplan.PickResolver(nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	er, ok := r.(fetchplan.EnvOverrideResolver)
	if !ok {
		t.Fatalf("want EnvOverrideResolver wrapping DefaultResolver, got %T", r)
	}
	if _, ok := er.Inner.(fetchplan.DefaultResolver); !ok {
		t.Fatalf("want inner DefaultResolver, got %T", er.Inner)
	}
}

func TestPickResolver_airgapEnv(t *testing.T) {
	t.Setenv("OKDCTL_AIRGAP", "1")
	t.Setenv("OKDCTL_MIRROR_BASE", "https://mirror.corp")
	r, err := fetchplan.PickResolver(nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	er, ok := r.(fetchplan.EnvOverrideResolver)
	if !ok {
		t.Fatalf("want EnvOverrideResolver wrapping MirrorResolver, got %T", r)
	}
	if _, ok := er.Inner.(fetchplan.MirrorResolver); !ok {
		t.Fatalf("want inner MirrorResolver, got %T", er.Inner)
	}
}

func TestPickResolver_airgapMissingBase(t *testing.T) {
	t.Setenv("OKDCTL_AIRGAP", "1")
	t.Setenv("OKDCTL_MIRROR_BASE", "")
	_, err := fetchplan.PickResolver(nil, false)
	if err == nil {
		t.Fatal("expected ConfigError when air-gap active but no base; got nil")
	}
}

func TestResolveMirrorBase_envWins(t *testing.T) {
	t.Setenv("OKDCTL_MIRROR_BASE", "https://env.mirror/")
	cfg := &config.Config{}
	cfg.Deployment.MirrorBase = "https://config.mirror"
	got := fetchplan.ResolveMirrorBase(cfg)
	if got != "https://env.mirror" {
		t.Errorf("env should win over config; got %q", got)
	}
}

func TestHelmEnvURLAndVersionOverride(t *testing.T) {
	t.Setenv("OKDCTL_HELM_URL", "https://mirror.example.com/helm-{version}-linux-{arch}.tar.gz")
	t.Setenv("OKDCTL_HELM_VERSION", "v3.99.0")
	in := fetchplan.ResolveM5Input("amd64", nil)
	p := fetchplan.BuildM5Plan(&in)

	helm := blobByPurpose(t, p, fetchplan.PurposeHelm)
	if !strings.Contains(helm.URL, "mirror.example.com") {
		t.Errorf("helm URL %q does not honour env URL override", helm.URL)
	}
	if !strings.Contains(helm.URL, "v3.99.0") {
		t.Errorf("helm URL %q does not honour env VERSION override", helm.URL)
	}
}

func TestYQVersionedURLWhenVersionSet(t *testing.T) {
	t.Setenv("OKDCTL_YQ_VERSION", "v4.45.4")
	in := fetchplan.ResolveM5Input("amd64", nil)
	p := fetchplan.BuildM5Plan(&in)

	yq := blobByPurpose(t, p, fetchplan.PurposeYQ)
	if !strings.Contains(yq.URL, "v4.45.4") {
		t.Errorf("yq URL %q missing configured version — gap fix failed", yq.URL)
	}
	if strings.Contains(yq.URL, "/latest/") {
		t.Errorf("yq URL %q still uses /latest/ redirect — gap fix failed", yq.URL)
	}
}

func TestYQLatestRedirectWhenNoVersion(t *testing.T) {
	t.Setenv("OKDCTL_YQ_VERSION", "")
	t.Setenv("OKDCTL_YQ_URL", "")
	in := fetchplan.ResolveM5Input("amd64", nil)
	p := fetchplan.BuildM5Plan(&in)
	yq := blobByPurpose(t, p, fetchplan.PurposeYQ)
	if !strings.Contains(yq.URL, "/latest/") {
		t.Errorf("expected /latest/ redirect when no version configured; got %q", yq.URL)
	}
}

func TestSopsConfigVersionOverride(t *testing.T) {
	t.Setenv("OKDCTL_SOPS_URL", "")
	t.Setenv("OKDCTL_SOPS_VERSION", "")
	cfg := &config.Config{}
	cfg.Deployment.ToolVersions = map[string]config.ToolVersionOverride{
		"sops": {Version: "v3.10.0"},
	}
	in := fetchplan.ResolveM5Input("arm64", cfg)
	p := fetchplan.BuildM5Plan(&in)
	sops := blobByPurpose(t, p, fetchplan.PurposeSops)
	if !strings.Contains(sops.URL, "v3.10.0") {
		t.Errorf("sops URL %q does not honour config version override", sops.URL)
	}
	if !strings.Contains(sops.URL, "arm64") {
		t.Errorf("sops URL %q missing arch substitution", sops.URL)
	}
}

func TestBuildCoreOSStreamPlan_urlShape(t *testing.T) {
	cases := []struct {
		minor    int
		wantPath string
	}{
		{15, "release-4.15/data/data/coreos/fcos.json"},
		{18, "release-4.18/data/data/coreos/fcos.json"},
		{19, "release-4.19/data/data/coreos/scos.json"},
		{20, "release-4.20/data/data/coreos/scos.json"},
		{23, "release-4.23/data/data/coreos/scos.json"},
	}
	for _, tt := range cases {
		p := fetchplan.BuildCoreOSStreamPlan(tt.minor)
		if len(p.HTTPS) != 1 {
			t.Fatalf("minor %d: expected 1 blob, got %d", tt.minor, len(p.HTTPS))
		}
		b := p.HTTPS[0]
		if !strings.HasSuffix(b.URL, tt.wantPath) {
			t.Errorf("minor %d: URL %q missing suffix %q", tt.minor, b.URL, tt.wantPath)
		}
		if b.Purpose != fetchplan.PurposeCoreOSStream {
			t.Errorf("minor %d: purpose %q, want %q", tt.minor, b.Purpose, fetchplan.PurposeCoreOSStream)
		}
	}
}

func blobByPurpose(t *testing.T, p fetchplan.Plan, purpose string) fetchplan.Blob {
	t.Helper()
	for _, b := range p.HTTPS {
		if b.Purpose == purpose {
			return b
		}
	}
	t.Fatalf("no blob with purpose %q in plan", purpose)
	return fetchplan.Blob{}
}

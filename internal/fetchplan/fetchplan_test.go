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

func TestMirrorResolver_stubPassthrough(t *testing.T) {
	r := fetchplan.MirrorResolver{MirrorBase: "https://mirror.corp"}
	const url = "https://get.helm.sh/helm-v3.17.3-linux-amd64.tar.gz"
	got, err := r.ResolveBlob(fetchplan.Blob{URL: url, Purpose: fetchplan.M5PurposeHelm})
	if err != nil || got != url {
		t.Fatalf("ResolveBlob with MirrorBase set should currently passthrough; got (%q,%v)", got, err)
	}
}

func TestHelmEnvURLAndVersionOverride(t *testing.T) {
	t.Setenv("OKDCTL_HELM_URL", "https://mirror.example.com/helm-{version}-linux-{arch}.tar.gz")
	t.Setenv("OKDCTL_HELM_VERSION", "v3.99.0")
	in := fetchplan.ResolveM5Input("amd64", nil)
	p := fetchplan.BuildM5Plan(&in)

	helm := blobByPurpose(t, p, fetchplan.M5PurposeHelm)
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

	yq := blobByPurpose(t, p, fetchplan.M5PurposeYQ)
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
	yq := blobByPurpose(t, p, fetchplan.M5PurposeYQ)
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
	sops := blobByPurpose(t, p, fetchplan.M5PurposeSops)
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
		if b.Purpose != fetchplan.M23PurposeCoreOSStream {
			t.Errorf("minor %d: purpose %q, want %q", tt.minor, b.Purpose, fetchplan.M23PurposeCoreOSStream)
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

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

func TestMirrorResolver_passthroughUntilM24(t *testing.T) {
	r := fetchplan.MirrorResolver{MirrorBase: "https://mirror.corp"}
	const url = "https://get.helm.sh/helm-v3.17.3-linux-amd64.tar.gz"
	got, err := r.ResolveBlob(fetchplan.Blob{URL: url, Purpose: fetchplan.M5PurposeHelm})
	if err != nil || got != url {
		t.Fatalf("ResolveBlob with MirrorBase set should return upstream until M24; got (%q,%v)", got, err)
	}
}

func TestResolveM4BaseURL_envOverride(t *testing.T) {
	t.Setenv("OKDCTL_OKD_RELEASE_URL", "https://mirror.example.com/okd/")
	got := fetchplan.ResolveM4BaseURL(&config.Config{})
	if got != "https://mirror.example.com/okd" {
		t.Errorf("got %q, want trailing slash trimmed", got)
	}
}

func TestResolveM4BaseURL_configOverride(t *testing.T) {
	t.Setenv("OKDCTL_OKD_RELEASE_URL", "")
	cfg := &config.Config{}
	cfg.Deployment.OKDReleaseBaseURL = "https://config.example.com/okd"
	if got := fetchplan.ResolveM4BaseURL(cfg); got != "https://config.example.com/okd" {
		t.Errorf("got %q, want config value", got)
	}
}

func TestResolveM4BaseURL_default(t *testing.T) {
	t.Setenv("OKDCTL_OKD_RELEASE_URL", "")
	const want = "https://github.com/okd-project/okd/releases/download"
	if got := fetchplan.ResolveM4BaseURL(&config.Config{}); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildM4Plan_urlsContainVersion(t *testing.T) {
	in := fetchplan.M4Input{
		BaseURL: "https://github.com/okd-project/okd/releases/download",
		Version: "4.16.0-0.okd-2024-01-27-040212",
		Arch:    "amd64",
	}
	p := fetchplan.BuildM4Plan(in)
	if len(p.HTTPS) != 3 {
		t.Fatalf("expected 3 blobs, got %d", len(p.HTTPS))
	}
	for _, b := range p.HTTPS {
		if !strings.Contains(b.URL, in.Version) {
			t.Errorf("blob URL %q missing version %q", b.URL, in.Version)
		}
		if b.Purpose != fetchplan.M4Purpose {
			t.Errorf("blob purpose %q, want %q", b.Purpose, fetchplan.M4Purpose)
		}
	}
}

func TestResolveM5Input_helmEnvOverride(t *testing.T) {
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

func TestBuildM5Plan_yqVersionGapFix(t *testing.T) {
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

func TestBuildM5Plan_yqLatestWhenNoVersion(t *testing.T) {
	t.Setenv("OKDCTL_YQ_VERSION", "")
	t.Setenv("OKDCTL_YQ_URL", "")
	in := fetchplan.ResolveM5Input("amd64", nil)
	p := fetchplan.BuildM5Plan(&in)
	yq := blobByPurpose(t, p, fetchplan.M5PurposeYQ)
	if !strings.Contains(yq.URL, "/latest/") {
		t.Errorf("expected /latest/ redirect when no version configured; got %q", yq.URL)
	}
}

func TestResolveM5Input_configOverride(t *testing.T) {
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

func TestBuildSCOSStreamPlan_urlShape(t *testing.T) {
	cases := []struct {
		minor   int
		wantSub string
	}{
		{19, "release-4.19"},
		{20, "release-4.20"},
		{23, "release-4.23"},
	}
	for _, tt := range cases {
		p := fetchplan.BuildSCOSStreamPlan(tt.minor)
		if len(p.HTTPS) != 1 {
			t.Fatalf("minor %d: expected 1 blob, got %d", tt.minor, len(p.HTTPS))
		}
		b := p.HTTPS[0]
		if !strings.Contains(b.URL, tt.wantSub) {
			t.Errorf("minor %d: URL %q missing %q", tt.minor, b.URL, tt.wantSub)
		}
		if !strings.HasSuffix(b.URL, "/data/data/coreos/scos.json") {
			t.Errorf("minor %d: URL %q missing expected path suffix", tt.minor, b.URL)
		}
		if b.Purpose != fetchplan.M23PurposeSCOSStream {
			t.Errorf("minor %d: purpose %q, want %q", tt.minor, b.Purpose, fetchplan.M23PurposeSCOSStream)
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

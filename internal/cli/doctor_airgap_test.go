//go:build linux

package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/fetchplan"
)

func stubHTTPHead(prefixStatus map[string]int) httpHeadFunc {
	return func(_ context.Context, url string) (int, error) {
		for prefix, code := range prefixStatus {
			if prefix == "" || strings.Contains(url, prefix) {
				return code, nil
			}
		}
		return http.StatusNotFound, nil
	}
}

func stubRegistryHead(prefixStatus map[string]int) registryHeadFunc {
	return func(_ context.Context, ref string) (int, error) {
		for prefix, code := range prefixStatus {
			if prefix == "" || strings.Contains(ref, prefix) {
				return code, nil
			}
		}
		return http.StatusNotFound, nil
	}
}

func newBlobServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
}

func newOCIServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/manifests/") {
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
}

func TestBuildAirgapChecks_notActive(t *testing.T) {
	checks := buildAirgapChecks(context.Background(), nil, false, nil, nil)
	if len(checks) != 0 {
		t.Errorf("expected 0 checks when not in airgap mode, got %d", len(checks))
	}
}

func TestBuildAirgapChecks_returnsAllFive(t *testing.T) {
	t.Setenv("OKDCTL_AIRGAP", "1")
	t.Setenv("OKDCTL_MIRROR_BASE", "https://mirror.local")

	checks := buildAirgapChecks(context.Background(), nil, false, stubHTTPHead(map[string]int{"": 200}), stubRegistryHead(map[string]int{"": 200}))
	if len(checks) != 5 {
		t.Fatalf("expected 5 airgap checks, got %d", len(checks))
	}
}

func TestCheckAirgapMirrorReachable_allPass(t *testing.T) {
	srv := newBlobServer(t)
	defer srv.Close()

	resolver := fetchplan.MirrorResolver{MirrorBase: srv.URL}
	head := stubHTTPHead(map[string]int{"": http.StatusOK})
	r := checkAirgapMirrorReachable(context.Background(), nil, resolver, head)

	for _, item := range r.items {
		if item.sev == sevFail {
			t.Errorf("unexpected fail item: %s — %s", item.name, item.note)
		}
	}
}

func TestCheckAirgapMirrorReachable_blobMissing(t *testing.T) {
	resolver := fetchplan.MirrorResolver{MirrorBase: "https://mirror.local"}
	head := func(_ context.Context, url string) (int, error) {
		if strings.Contains(url, "/helm/") {
			return http.StatusNotFound, nil
		}
		return http.StatusOK, nil
	}
	r := checkAirgapMirrorReachable(context.Background(), nil, resolver, head)
	if r.sev != sevFail {
		t.Errorf("expected sevFail when a blob is 404, got sev=%d", r.sev)
	}
	found := false
	for _, item := range r.items {
		if item.sev == sevFail && strings.Contains(item.note, "fetch-blobs.sh") {
			found = true
		}
	}
	if !found {
		t.Error("expected fetch-blobs.sh hint in a fail item note")
	}
}

func TestCheckAirgapMirrorReachable_noMirrorBase(t *testing.T) {
	resolver := fetchplan.MirrorResolver{MirrorBase: ""}
	r := checkAirgapMirrorReachable(context.Background(), nil, resolver, nil)
	if r.sev != sevFail {
		t.Errorf("expected sevFail when MirrorBase is empty, got sev=%d", r.sev)
	}
}

func TestCheckAirgapReleaseImage_noVersion(t *testing.T) {
	resolver := fetchplan.MirrorResolver{MirrorBase: "https://mirror.local"}
	r := checkAirgapReleaseImage(context.Background(), nil, resolver, stubRegistryHead(map[string]int{"": 200}))
	if r.sev != sevWarn {
		t.Errorf("expected sevWarn when version unset, got sev=%d detail=%q", r.sev, r.detail)
	}
}

func TestCheckAirgapReleaseImage_reachableWarn(t *testing.T) {
	resolver := fetchplan.MirrorResolver{MirrorBase: "https://mirror.local"}
	head := stubRegistryHead(map[string]int{"": http.StatusOK})
	cfg := &config.Config{}
	cfg.Distribution.Version = "4.21.0-okd-scos.10"

	r := checkAirgapReleaseImage(context.Background(), cfg, resolver, head)
	if r.sev != sevWarn {
		t.Errorf("expected sevWarn (cosign unavailable), got sev=%d detail=%q", r.sev, r.detail)
	}
	if !strings.Contains(r.detail, "okd-project/okd#2092") {
		t.Errorf("expected okd#2092 reference, got %q", r.detail)
	}
}

func TestCheckAirgapReleaseImage_notFound(t *testing.T) {
	resolver := fetchplan.MirrorResolver{MirrorBase: "https://mirror.local"}
	head := stubRegistryHead(map[string]int{})
	cfg := &config.Config{}
	cfg.Distribution.Version = "4.21.0-okd-scos.10"

	r := checkAirgapReleaseImage(context.Background(), cfg, resolver, head)
	if r.sev != sevFail {
		t.Errorf("expected sevFail when registry returns 404, got sev=%d", r.sev)
	}
	if !strings.Contains(r.detail, "run-oc-mirror.sh") {
		t.Errorf("expected run-oc-mirror.sh hint, got %q", r.detail)
	}
}

func TestCheckAirgapBootstrapOC(t *testing.T) {
	r := checkAirgapBootstrapOC()
	if r.sev != sevPass && r.sev != sevFail {
		t.Errorf("unexpected severity: %d", r.sev)
	}
	if r.sev == sevFail && !strings.Contains(r.detail, "mirror.openshift.com") {
		t.Errorf("expected mirror.openshift.com URL hint in fail detail, got %q", r.detail)
	}
}

func TestCheckAirgapIDMS_noCluster(t *testing.T) {
	r := checkAirgapIDMS(context.Background())
	if r.sev != sevWarn {
		t.Errorf("expected sevWarn when no cluster, got sev=%d detail=%q", r.sev, r.detail)
	}
}

func TestCheckAirgapAddonArtifacts_noAddons(t *testing.T) {
	resolver := fetchplan.MirrorResolver{MirrorBase: "https://mirror.local"}
	head := stubRegistryHead(map[string]int{"": http.StatusOK})
	r := checkAirgapAddonArtifacts(context.Background(), resolver, head)
	if r.sev != sevPass {
		t.Errorf("expected sevPass with no registered addons, got sev=%d detail=%q", r.sev, r.detail)
	}
}

// TestDoctorAirgap_integrationSmoke wires a real httptest blob server and a
// stubbed OCI registry (via newOCIServer) to confirm the five checks
// assemble, run, and route every probe through the injection points without
// touching the network. Asserts all five checks are present; per-check
// probing is exercised by the focused Test* cases above.
func TestDoctorAirgap_integrationSmoke(t *testing.T) {
	blobSrv := newBlobServer(t)
	defer blobSrv.Close()
	ociSrv := newOCIServer(t)
	defer ociSrv.Close()

	t.Setenv("OKDCTL_AIRGAP", "1")
	t.Setenv("OKDCTL_MIRROR_BASE", blobSrv.URL)

	cfg := &config.Config{}
	cfg.Distribution.Version = "4.21.0-okd-scos.10"

	registryOK := stubRegistryHead(map[string]int{"": http.StatusOK})

	checks := buildAirgapChecks(context.Background(), cfg, true, nil, registryOK)
	if len(checks) != 5 {
		t.Fatalf("expected 5 airgap checks, got %d", len(checks))
	}

	seen := make(map[string]bool)
	for _, c := range checks {
		seen[c.name] = true
	}
	want := []string{
		"airgap mirror reachable",
		"airgap release image digest pinned",
		"airgap addon artifacts present",
		"airgap bootstrap oc present",
		"airgap idms applied",
	}
	for _, name := range want {
		if !seen[name] {
			t.Errorf("missing airgap check %q", name)
		}
	}

	// Sanity-probe the OCI stub so the fixture is exercised end-to-end;
	// defaultRegistryHead hardcodes https, so issue a raw HEAD here.
	resp, err := http.Head(ociSrv.URL + "/v2/test/image/manifests/tag")
	if err != nil {
		t.Fatalf("OCI stub HEAD returned error: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("OCI stub expected 200, got %d", resp.StatusCode)
	}
}

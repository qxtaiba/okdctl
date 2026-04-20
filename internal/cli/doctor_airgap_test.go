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
	r := checkAirgapMirrorReachable(context.Background(), nil, resolver, defaultHTTPHead)

	for _, item := range r.items {
		if item.sev == sevFail {
			t.Errorf("unexpected fail item: %s — %s", item.name, item.note)
		}
	}
	if r.sev == sevFail {
		t.Errorf("expected non-fail aggregate sev when all blobs resolve to a 200 httptest server, got sev=%d", r.sev)
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

// TestDoctorAirgap_integrationSmoke wires a real httptest blob server as
// MirrorBase, drives every check function, and asserts each resolves without
// a hard failure. The OCI probes use an injected registryHead that rewrites
// the upstream scheme/host to the httptest OCI server, so the check code
// path — not a disconnected stub — is what exercises the fixtures.
func TestDoctorAirgap_integrationSmoke(t *testing.T) {
	blobSrv := newBlobServer(t)
	defer blobSrv.Close()
	ociSrv := newOCIServer(t)
	defer ociSrv.Close()

	ociHost := strings.TrimPrefix(ociSrv.URL, "http://")

	t.Setenv("OKDCTL_AIRGAP", "1")
	t.Setenv("OKDCTL_MIRROR_BASE", blobSrv.URL)

	cfg := &config.Config{}
	cfg.Distribution.Version = "4.21.0-okd-scos.10"

	// The production registryHead speaks https; the httptest fixture is
	// http-only. Route each ref through the OCI stub by rewriting the host
	// and forcing http, matching the shape defaultRegistryHead would build.
	ociHead := func(ctx context.Context, ref string) (int, error) {
		slash := strings.IndexByte(ref, '/')
		if slash < 0 {
			return 0, nil
		}
		rest := ref[slash+1:]
		var name, reference string
		if idx := strings.LastIndex(rest, "@"); idx >= 0 {
			name, reference = rest[:idx], rest[idx+1:]
		} else if idx := strings.LastIndex(rest, ":"); idx >= 0 {
			name, reference = rest[:idx], rest[idx+1:]
		} else {
			name, reference = rest, "latest"
		}
		url := "http://" + ociHost + "/v2/" + name + "/manifests/" + reference
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, http.NoBody)
		if err != nil {
			return 0, err
		}
		resp, doErr := http.DefaultClient.Do(req)
		if doErr != nil {
			return 0, doErr
		}
		_ = resp.Body.Close()
		return resp.StatusCode, nil
	}

	checks := buildAirgapChecks(context.Background(), cfg, true, nil, ociHead)
	if len(checks) != 5 {
		t.Fatalf("expected 5 airgap checks, got %d", len(checks))
	}

	results := make(map[string]checkResult, 5)
	for _, c := range checks {
		results[c.name] = c.fn(context.Background())
	}

	// Mirror-reachable: httptest serves 200 on every HEAD, so every blob
	// must pass.
	mirror := results["airgap mirror reachable"]
	for _, item := range mirror.items {
		if item.sev == sevFail {
			t.Errorf("mirror-reachable item unexpectedly failed: %s — %s", item.name, item.note)
		}
	}

	// Release-image: ociHead returns 200, but the check always warns per
	// okd-project/okd#2092 until cosign is available.
	rel := results["airgap release image digest pinned"]
	if rel.sev != sevWarn {
		t.Errorf("release-image expected sevWarn on reachable, got sev=%d detail=%q", rel.sev, rel.detail)
	}
	if !strings.Contains(rel.detail, "okd#2092") {
		t.Errorf("release-image detail missing okd#2092 pointer: %q", rel.detail)
	}

	// Addon artifacts: no catalog import in the test binary, so addon.All
	// is empty and the check reports sevPass with the "no entries" detail.
	addons := results["airgap addon artifacts present"]
	if addons.sev != sevPass {
		t.Errorf("addon-artifacts expected sevPass with no registered addons, got sev=%d detail=%q", addons.sev, addons.detail)
	}

	// Bootstrap-oc depends on the host environment; accept either pass or
	// fail but require a remediation hint when it fails.
	boc := results["airgap bootstrap oc present"]
	if boc.sev == sevFail && !strings.Contains(boc.detail, "mirror.openshift.com") {
		t.Errorf("bootstrap-oc fail detail missing remediation hint: %q", boc.detail)
	}

	// IDMS: test environment has no kubeconfig, so the check self-skips
	// with sevWarn.
	idms := results["airgap idms applied"]
	if idms.sev != sevWarn {
		t.Errorf("idms expected sevWarn pre-deploy, got sev=%d detail=%q", idms.sev, idms.detail)
	}
}

//go:build linux

package cli

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/addon"
	addonmirror "github.com/qxtaiba/okdctl/internal/addon/mirror"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/fetchplan"
)

// ociManifestAccept is the Accept header for OCI manifest HEAD probes —
// registries gate manifest responses on the Accept type.
const ociManifestAccept = "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"

// httpHeadFunc probes a URL by HTTP HEAD and returns the status code.
// Tests inject deterministic stubs; nil in buildAirgapChecks falls back
// to defaultHTTPHead.
type httpHeadFunc func(ctx context.Context, url string) (int, error)

// registryHeadFunc probes an OCI manifest endpoint by HEAD and returns the
// status code. The ref is a bare registry path without a scheme prefix
// (e.g. "mirror.local/quay/okd/scos-release:4.21.0-okd-scos.10").
// nil in buildAirgapChecks falls back to defaultRegistryHead.
type registryHeadFunc func(ctx context.Context, ref string) (int, error)

func defaultHTTPHead(ctx context.Context, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, http.NoBody)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

func defaultRegistryHead(ctx context.Context, ref string) (int, error) {
	slash := strings.IndexByte(ref, '/')
	if slash < 0 {
		return 0, fmt.Errorf("invalid OCI ref (no slash): %s", ref)
	}
	host := ref[:slash]
	rest := ref[slash+1:]

	var name, reference string
	if idx := strings.LastIndex(rest, "@"); idx >= 0 {
		name, reference = rest[:idx], rest[idx+1:]
	} else if idx := strings.LastIndex(rest, ":"); idx >= 0 {
		name, reference = rest[:idx], rest[idx+1:]
	} else {
		name, reference = rest, "latest"
	}

	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", host, name, reference)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, manifestURL, http.NoBody)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", ociManifestAccept)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

// buildAirgapChecks returns the five air-gap doctor checks when air-gap mode
// is active (--airgap flag or OKDCTL_AIRGAP=1), or nil otherwise. httpHead
// and registryHead may be nil; nil resolves to the real net/http
// implementations — tests inject deterministic stubs.
func buildAirgapChecks(ctx context.Context, cfg *config.Config, airgapFlag bool, httpHead httpHeadFunc, registryHead registryHeadFunc) []check {
	if !fetchplan.IsAirgap(cfg, airgapFlag) {
		return nil
	}
	if httpHead == nil {
		httpHead = defaultHTTPHead
	}
	if registryHead == nil {
		registryHead = defaultRegistryHead
	}

	mirrorBase := fetchplan.ResolveMirrorBase(cfg)
	resolver := fetchplan.MirrorResolver{MirrorBase: mirrorBase}

	return []check{
		{
			"airgap mirror reachable",
			"HEAD probe every FetchPlan blob against the configured mirror",
			func(c context.Context) checkResult {
				return checkAirgapMirrorReachable(c, cfg, resolver, httpHead)
			},
		},
		{
			"airgap release image digest pinned",
			"OCI manifest probe for quay.io/okd/scos-release in the mirror",
			func(c context.Context) checkResult {
				return checkAirgapReleaseImage(c, cfg, resolver, registryHead)
			},
		},
		{
			"airgap addon artifacts present",
			"helm-template image extraction + mirror HEAD for each MirrorableAddon",
			func(c context.Context) checkResult {
				return checkAirgapAddonArtifacts(c, resolver, registryHead)
			},
		},
		{
			"airgap bootstrap oc present",
			"oc binary on $PATH (required for release-image extraction)",
			func(_ context.Context) checkResult { return checkAirgapBootstrapOC() },
		},
		{
			"airgap idms applied",
			"ImageDigestMirrorSet / ImageTagMirrorSet on cluster (post-deploy only)",
			func(c context.Context) checkResult { return checkAirgapIDMS(c) },
		},
	}
}

func checkAirgapMirrorReachable(ctx context.Context, cfg *config.Config, resolver fetchplan.MirrorResolver, head httpHeadFunc) checkResult {
	if resolver.MirrorBase == "" {
		return checkResult{
			sev:    sevFail,
			detail: "OKDCTL_MIRROR_BASE (or deployment.mirror_base) is not set; set it to your mirror URL and re-run",
		}
	}

	minor := airgapMinor(resolveOKDVersionForDoctor(cfg))
	in := fetchplan.ResolveM5Input("amd64", cfg)
	m5 := fetchplan.BuildM5Plan(&in)
	ocPlan := fetchplan.BuildM22BootstrapOCPlan()
	streamPlan := fetchplan.BuildCoreOSStreamPlan(minor)

	blobs := make([]fetchplan.Blob, 0, len(m5.HTTPS)+len(ocPlan.HTTPS)+1)
	blobs = append(blobs, m5.HTTPS...)
	blobs = append(blobs, ocPlan.HTTPS...)
	blobs = append(blobs, streamPlan.HTTPS...)

	var items []checkItem
	worst := sevPass

	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for _, b := range blobs {
		resolved, err := resolver.ResolveBlob(b)
		if err != nil {
			items = append(items, checkItem{
				sev:  sevFail,
				name: b.Purpose,
				note: fmt.Sprintf("resolve failed: %v; check OKDCTL_MIRROR_BASE", err),
			})
			worst = sevFail
			continue
		}
		code, headErr := head(probeCtx, resolved)
		switch {
		case headErr != nil:
			items = append(items, checkItem{
				sev:  sevFail,
				name: b.Purpose,
				note: fmt.Sprintf("%s — %v; stage via fetch-blobs.sh", resolved, headErr),
			})
			worst = sevFail
		case code != http.StatusOK && code != http.StatusFound:
			items = append(items, checkItem{
				sev:  sevFail,
				name: b.Purpose,
				note: fmt.Sprintf("HTTP %d at %s; stage via fetch-blobs.sh", code, resolved),
			})
			worst = sevFail
		default:
			items = append(items, checkItem{sev: sevPass, name: b.Purpose})
		}
	}

	return checkResult{sev: worst, items: items}
}

func checkAirgapReleaseImage(ctx context.Context, cfg *config.Config, resolver fetchplan.MirrorResolver, head registryHeadFunc) checkResult {
	ver := resolveOKDVersionForDoctor(cfg)
	if ver == "" {
		return checkResult{
			sev:    sevWarn,
			detail: "okd version not configured; set distribution.version in okdctl.yaml to enable this check",
		}
	}

	artifact := fetchplan.OKDReleaseImageRef(ver)
	resolved, err := resolver.ResolveOCI(artifact)
	if err != nil {
		return checkResult{
			sev:    sevFail,
			detail: fmt.Sprintf("resolve release image ref: %v; check OKDCTL_MIRROR_BASE", err),
		}
	}

	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	code, headErr := head(probeCtx, resolved)
	switch {
	case headErr != nil:
		return checkResult{
			sev:    sevFail,
			detail: fmt.Sprintf("cannot reach %s: %v; push quay.io/okd/scos-release:%s to your mirror, then re-run run-oc-mirror.sh", resolved, headErr, ver),
		}
	case code == http.StatusUnauthorized:
		return checkResult{
			sev:    sevWarn,
			detail: fmt.Sprintf("%s reachable but auth required (HTTP 401); configure imagePullSecret if the mirror is private", resolved),
		}
	case code != http.StatusOK:
		return checkResult{
			sev:    sevFail,
			detail: fmt.Sprintf("HTTP %d from %s; push quay.io/okd/scos-release:%s to your mirror via run-oc-mirror.sh", code, resolved, ver),
		}
	}
	// cosign not yet available for OKD release images (okd-project/okd#2092);
	// digest-pinning via OCI manifest HEAD is the available integrity gate.
	return checkResult{
		sev:    sevWarn,
		detail: fmt.Sprintf("%s reachable — digest-pinning only; cosign verification pending okd-project/okd#2092", resolved),
	}
}

func checkAirgapAddonArtifacts(ctx context.Context, resolver fetchplan.MirrorResolver, head registryHeadFunc) checkResult {
	execr := executor.New()
	addons := addon.All()

	var items []checkItem
	worst := sevPass

	probeCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	for _, a := range addons {
		m, ok := a.(addon.MirrorableAddon)
		if !ok {
			continue
		}
		spec := m.MirrorArtifacts()
		if len(spec.Charts) == 0 && len(spec.StaticImages) == 0 {
			continue
		}

		imgs, err := addonmirror.SpecImages(probeCtx, execr, spec)
		if err != nil {
			items = append(items, checkItem{
				sev:  sevWarn,
				name: a.Info().Name,
				note: fmt.Sprintf("helm template failed (%v); ensure helm is on PATH and charts are reachable upstream", err),
			})
			if sevWarn > worst {
				worst = sevWarn
			}
			continue
		}

		for _, img := range imgs {
			resolved, resolveErr := resolver.ResolveOCI(fetchplan.OCIArtifact{
				Ref:     img,
				Purpose: fetchplan.PurposeAddonChart,
			})
			if resolveErr != nil {
				items = append(items, checkItem{
					sev:  sevFail,
					name: a.Info().Name + "/" + img,
					note: fmt.Sprintf("resolve: %v", resolveErr),
				})
				worst = sevFail
				continue
			}
			code, headErr := head(probeCtx, resolved)
			switch {
			case headErr != nil:
				items = append(items, checkItem{
					sev:  sevFail,
					name: a.Info().Name + "/" + img,
					note: fmt.Sprintf("%s — %v; re-run ./run-oc-mirror.sh", resolved, headErr),
				})
				worst = sevFail
			case code == http.StatusNotFound:
				items = append(items, checkItem{
					sev:  sevFail,
					name: a.Info().Name + "/" + img,
					note: fmt.Sprintf("HTTP 404 at %s; add to isc.yaml additionalImages and re-run oc-mirror", resolved),
				})
				worst = sevFail
			case code != http.StatusOK && code != http.StatusUnauthorized:
				items = append(items, checkItem{
					sev:  sevFail,
					name: a.Info().Name + "/" + img,
					note: fmt.Sprintf("HTTP %d at %s; re-run ./run-oc-mirror.sh", code, resolved),
				})
				worst = sevFail
			default:
				items = append(items, checkItem{sev: sevPass, name: a.Info().Name + "/" + img})
			}
		}
	}

	if len(items) == 0 {
		return checkResult{sev: sevPass, detail: "no MirrorableAddon entries to check"}
	}
	return checkResult{sev: worst, items: items}
}

func checkAirgapBootstrapOC() checkResult {
	if _, err := exec.LookPath("oc"); err != nil {
		return checkResult{
			sev:    sevFail,
			detail: "oc not found on $PATH; fetch from https://mirror.openshift.com/pub/openshift-v4/clients/oc/latest/linux/oc.tar.gz and extract to a directory on $PATH",
		}
	}
	return checkResult{sev: sevPass, detail: "oc found on $PATH"}
}

func checkAirgapIDMS(ctx context.Context) checkResult {
	projectRoot, err := resolveProjectRoot()
	if err != nil {
		return checkResult{
			sev:    sevWarn,
			detail: "cannot resolve project root; run after okdctl deploy",
		}
	}

	bp, bpErr := newStatusPhase(projectRoot)
	if bpErr != nil {
		return checkResult{
			sev:    sevWarn,
			detail: "cluster not reachable (no kubeconfig); run this check after okdctl deploy",
		}
	}

	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	idmsExists, _ := bp.OcResourceExists(probeCtx, "idms probe", "imagedigestmirrorset", "--all-namespaces")
	itmsExists, _ := bp.OcResourceExists(probeCtx, "itms probe", "imagetagmirrorset", "--all-namespaces")

	if idmsExists || itmsExists {
		return checkResult{sev: sevPass, detail: "ImageDigestMirrorSet / ImageTagMirrorSet applied on cluster"}
	}
	return checkResult{
		sev:    sevFail,
		detail: "no ImageDigestMirrorSet or ImageTagMirrorSet found; apply oc-mirror output: oc apply -f oc-mirror-workspace/results-*/",
	}
}

func resolveOKDVersionForDoctor(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.Distribution.Version
}

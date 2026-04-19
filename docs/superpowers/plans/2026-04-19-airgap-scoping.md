# L15 — Air-gap feasibility + scoping

> **Scope of this document:** scoping + architecture, not implementation. Per
> the roadmap, L15's Acceptance is *"design doc reviewed and approved before
> any code."* Implementation lands as a series of roadmap items (M21–M28),
> each its own PR. This doc fixes the contract they implement against.

**Status:** draft
**Date:** 2026-04-19
**Roadmap item:** L15
**Preceded by:** M4 (`OKDReleaseBaseURL`), M5 (`ToolVersions`)
**Blocks:** M21–M28 implementation items
**Adjacent:** M29 (GitHub Artifact Attestations), M30 (`oras-go/v2` deferred)

---

## 1. Summary

okdctl gains full air-gap support via two intertwined moves:

1. **A `FetchPlan` abstraction** — every external artifact okdctl fetches is
   declared as data (`OCIArtifact` or `Blob`) with a purpose, an upstream
   source, and a redirect hook. Current ad-hoc fetches in
   `internal/download/`, `internal/distribution/okd/setup/artifacts.go`, and
   `internal/distribution/okd/releases/fetcher.go` are migrated into the
   plan. Air-gap is then just a different resolver over the same plan.

2. **An OCI-centric pivot for the OKD release payload.** okdctl stops
   fetching `openshift-install` / `oc` from GitHub release tarballs and
   instead extracts them from the upstream OKD release container image
   (`quay.io/okd/scos-release@sha256:<digest>`) via
   `oc adm release extract --tools`. The release image is the build's
   canonical source; GitHub tarballs are a repackaged convenience wrapper.
   This change (a) makes okdctl's binaries bit-identical to what the cluster
   runs, (b) reduces air-gap to a standard
   `ImageDigestMirrorSet` hostname rewrite — one rule for all images, and
   (c) aligns okdctl with Red Hat's documented disconnected-install
   workflow so operators can reuse existing tooling (`oc-mirror v2`).

The **operator stages the mirror** — okdctl does not orchestrate it. Instead
okdctl emits an `ImageSetConfiguration` the operator feeds into their own
`oc-mirror --v2` run, plus a tiny `airgap.yaml` listing the HTTPS blobs (the
SCOS ISO, the tool tarballs) that `oc-mirror` doesn't handle. `okdctl doctor
--airgap` verifies the staged mirror before deploy.

All three scenarios named in the roadmap — fully air-gapped, caching proxy,
private mirror for speed/compliance — are supported by this shape.

## 2. Research foundation

This document is backed by four audit-grade passes run on 2026-04-19:

- **Inventory pass** — every external fetch site in the repo, audited with
  file:line refs. Identified five distinct fetch mechanisms (okdctl's HTTP
  client, distro package manager, `openshift-install` subprocess,
  `terraform init` subprocess, `helm install` subprocess) and the subset
  each is already redirectable through today.
- **Source-alternatives pass** — surveyed OKD/OpenShift ecosystem tooling
  (quay.io release registries, `oc adm release extract`, stream metadata
  files, `oc-mirror` v2, `oras-go`). Revealed that the GitHub tarballs
  okdctl fetches are a convenience wrapper over the release image, and
  that OKD's stream metadata is a standalone public file.
- **Technical verification pass** — four parallel agents verified:
  - `cosign` coverage on OKD release images (**no** — legacy GPG via
    `maintainers@okd.io`, signatures not at a stable public URL;
    tracked upstream as [okd#2092](https://github.com/okd-project/okd/issues/2092));
  - `oc-mirror v2` `type: okd` support (**yes**, since PR #117 in 2021,
    survived v1→v2 rewrite; `TypeOKD` enum in
    `internal/pkg/api/v2alpha1/type_platform.go`; Cincinnati via a
    dedicated OKD client at `internal/pkg/release/client.go`);
  - `openshift/installer` branch `scos.json` availability
    (4.19+ only; 4.15–4.18 have `fcos.json` / `rhcos.json` instead;
    4.19 ships with stream name `c9s`, 4.20+ with `c10s`);
  - Follow-up items (M29 / M30) scoping and pre-provisioning checks.
- **Product-decision pass** — user locked four strategic choices (§3) that
  bound the design.

Agent output archived at `/tmp/claude-501/...` (ephemeral); the findings that
matter are quoted inline in the sections below.

## 3. Product decisions (locked)

| Decision | Choice | Rationale |
|---|---|---|
| Supported scenarios | All three (fully air-gapped, caching proxy, private mirror for speed/compliance) | User intent: build once for the full air-gap audience; avoid a second pass to extend coverage. |
| Responsibility model | Hybrid — **operator stages, okdctl verifies** | Avoids inventing a mirror tool; `oc-mirror v2` + `skopeo sync` already cover the staging side. |
| Rearchitecture | `FetchPlan` as target shape; refactor ships as its own item (**M21**) before new fetch-redirects | Prevents the "five more ad-hoc patches" outcome of extending M4/M5 piecemeal. |
| Primary source | OCI-centric — OKD binaries via release-image extraction | Bit-identical to cluster runtime; one `IDMS` rule handles all images; aligns with upstream disconnected-install workflow. |
| Version support | **No change** — continue to support OKD 4.15+ as before | User explicit direction: don't throttle users. M23 uses a dual path (direct fetch ≥4.19, shellout fallback <4.19). |
| FCOS → SCOS rename | **Deferred** out of L15 | Unrelated to air-gap; bundling invites scope creep. Can land as its own cleanup PR. |

## 4. External fetch inventory

Audit-grade table; file:line refs are the repo state as of 2026-04-19.

### 4.1 okdctl-controlled (redirectable by okdctl)

| Fetch | Source today | file:line | Redirect status |
|---|---|---|---|
| OKD release binaries (`openshift-install`, `oc`, `sha256sum.txt`) | `github.com/okd-project/okd/releases/download/{version}/` | `internal/distribution/okd/releases/fetcher.go:31`, `internal/distribution/okd/setup/artifacts.go:90` | **Via M4** (`OKDCTL_OKD_RELEASE_URL` + `Deployment.OKDReleaseBaseURL`). Being replaced by OCI extraction in M22. |
| helm | `https://get.helm.sh/helm-{version}-linux-{arch}.tar.gz` | `internal/distribution/okd/setup/tools.go:82` | **Via M5** (`OKDCTL_HELM_URL`, `OKDCTL_HELM_VERSION`). |
| sops | `github.com/getsops/sops/releases/download/{version}/…` | `internal/distribution/okd/setup/tools.go:87` | **Via M5** (`OKDCTL_SOPS_URL`, `OKDCTL_SOPS_VERSION`). |
| yq | `github.com/mikefarah/yq/releases/latest/download/yq_linux_{arch}` | `internal/distribution/okd/setup/tools.go:78` | **Via M5** — **but** `/releases/latest/download/` redirect makes version overrides a silent no-op unless `URLTemplate` is set. Fix in M21 (FetchPlan adopts the existing override; fix the template gap). |
| SCOS ISO (metadata discovery) | `openshift-install coreos print-stream-json` subprocess | `internal/distribution/okd/setup/coreos.go:91-143` | **No redirect today.** Replaced in M23 by direct GET of `scos.json` for 4.19+; shellout fallback for <4.19. |
| SCOS ISO (download) | URL embedded in stream metadata (`rhcos.mirror.openshift.com/art/storage/prod/streams/c10s/builds/…`) | `internal/distribution/okd/setup/coreos.go:149-188` | **No redirect today.** Escape hatch: `Provider.Proxmox.FCOSIso` accepts a local path. M24 adds URL-prefix rewrite under `MirrorBase`. |
| okdctl self-update check | `api.github.com/repos/qxtaiba/okdctl/releases/latest` | `internal/version/updatecheck.go:85-114` | **Disable-only** (`OKDCTL_NO_UPDATE_CHECK=1`). M24 adds `OKDCTL_UPDATE_CHECK_URL` override. |
| okdctl installer (distribution) | `github.com/qxtaiba/okdctl/releases/…` | `scripts/install.sh:71-82` | `VERSION=vX.Y.Z` pin available; **no** alternate URL override. Out of L15 scope — installer is not part of the deploy flow. |

### 4.2 Subprocess-controlled (not directly redirectable by okdctl)

| Fetch | Host(s) | Source | Operator redirect path |
|---|---|---|---|
| Addon Helm charts — flux | `oci://ghcr.io/controlplaneio-fluxcd/charts/flux-{operator,instance}` | `internal/addon/catalog/flux/flux.go:134,151` | Cluster-level mirror registry + `ImagePullSecret` + `ImageDigestMirrorSet`. okdctl declares via `MirrorableAddon` (M25). |
| Container images in flux chart (source-controller, kustomize-controller, etc.) | `ghcr.io` | Resolved at cluster runtime via helm template | IDMS rewrite at cluster level. Images declared via `MirrorableAddon.MirrorArtifacts()` helm-template expansion (M25). |
| Terraform provider | `registry.terraform.io/bpg/proxmox` | `infrastructure/terraform/environments/production/versions.tf:5-7` | `TF_CLI_CONFIG_FILE` with `filesystem_mirror` block. okdctl documents the pin; operator configures. |
| OS packages (apt: `coreos-installer`, `haproxy`, `apache2`, `dnsmasq`, `terraform`; dnf equivalents) | Distro mirrors + `apt.releases.hashicorp.com` / `rpm.releases.hashicorp.com` | `internal/distribution/okd/setup/tools.go:127-130,222-273`, `internal/platform/packages.go` | Operator's distro package manager config (`sources.list.d/*`, `dnf` repo config). okdctl documents the package list. |
| HashiCorp GPG key | `apt.releases.hashicorp.com/gpg` | `internal/distribution/okd/setup/tools.go:222` | Operator mirrors the GPG key alongside the apt repo. |

### 4.3 Cluster-runtime (not visible to okdctl)

| Fetch | Host | Operator concern |
|---|---|---|
| Kubelet pulling addon images (e.g. `ghcr.io/fluxcd/source-controller:v1.x.y`) | `ghcr.io`, varies per addon | `ImageDigestMirrorSet` / `ImageTagMirrorSet` on cluster + `ImagePullSecrets`. okdctl inventories images via `MirrorableAddon`; operator applies cluster-level config. |
| ESO controllers (if user installs the External Secrets Operator separately) | varies | User's cluster config; not okdctl's surface. |

### 4.4 Not-relevant-to-deploy

SSH keyscan of Git hosts (flux addon host-key validation), Proxmox API
(user-configured), CI / dev tools (go install `air`, `golangci-lint`,
`yamlfmt`, `govulncheck`) — all listed in the research report but
out-of-scope for a deployed okdctl binary.

## 5. Architecture — `FetchPlan`

The canonical declaration-as-data pattern in this repo is `StepDef` +
`BuildSteps` (see CLAUDE.md §Architecture notes). `FetchPlan` is the same
pattern applied to external artifacts.

```go
// Package fetchplan (internal/fetchplan/) declares every external artifact
// okdctl reaches for during setup and install. The plan is built once per
// deploy; resolvers compose over it to produce either direct-upstream URLs
// (connected mode) or mirror-redirected URLs (air-gap mode).
package fetchplan

type Plan struct {
    OCI   []OCIArtifact // release image, addon charts
    HTTPS []Blob        // SCOS ISO, tool tarballs, stream metadata
}

type OCIArtifact struct {
    Ref        string // "quay.io/okd/scos-release:4.21.0-okd-scos.10"
    Digest     string // "sha256:..." when known; resolver pins on first fetch
    ExtractVia string // "oc-adm-release-extract" | "helm-pull"
    Purpose    string // "okd-release" | "flux-operator-chart" | ...
}

type Blob struct {
    URL     string
    SHA256  string  // required — verifier rejects on mismatch
    Purpose string  // "scos-iso" | "tool-binary-helm" | "stream-metadata"
}

// A Resolver converts a Plan entry into a concrete URL/ref to fetch.
// Stock resolver returns upstream; AirGapResolver rewrites via MirrorBase.
type Resolver interface {
    ResolveOCI(a OCIArtifact) (string, error)
    ResolveBlob(b Blob) (string, error)
}
```

Two stock resolvers ship:

- `DefaultResolver` — returns the upstream ref/URL as-is.
- `MirrorResolver` — rewrites per §6 rules using `OKDCTL_MIRROR_BASE` and
  per-fetch overrides.

Choosing between them is one env-var check at CLI entry. M4/M5's existing
per-fetch env vars continue to work because `MirrorResolver` honors them
as final overrides.

`doctor --airgap` receives the active resolver and calls HEAD (or registry
probe) on every resolved ref to produce a reachability report.

## 6. Mirror contract

### 6.1 The `MirrorBase` default (1:1 upstream paths)

Operators set one env var:

```
OKDCTL_MIRROR_BASE=https://mirror.local
```

okdctl's rewrite rules:

| Upstream | Rewritten |
|---|---|
| `quay.io/<path>` | `<base>/quay/<path>` |
| `ghcr.io/<path>` | `<base>/ghcr/<path>` |
| `registry.ci.openshift.org/<path>` | `<base>/registry-ci/<path>` |
| `rhcos.mirror.openshift.com/<path>` | `<base>/rhcos/<path>` |
| `get.helm.sh/<path>` | `<base>/helm/<path>` |
| `github.com/qxtaiba/okdctl/releases/<path>` | `<base>/okdctl/<path>` |
| `api.github.com/repos/qxtaiba/okdctl/<path>` | `<base>/okdctl-api/<path>` |

Rationale: the layout is a 1:1 map of the upstream host/path suffix.
Operators producing a mirror via `oc-mirror --v2` (for OCI) + `rsync` /
`skopeo sync` / `aws s3 sync` (for blobs) end up with exactly this tree
without rewrite scripting.

### 6.2 Per-fetch overrides (escape hatch)

M4/M5's existing env vars still work. `OKDCTL_OKD_RELEASE_URL` overrides
the OKD release base path; `OKDCTL_{HELM,SOPS,YQ}_URL` override tool
binary fetches. New additions from L15:

- `OKDCTL_UPDATE_CHECK_URL` — overrides the self-update endpoint (M24).
- `OKDCTL_SCOS_STREAM_URL` — overrides the `scos.json` GET endpoint (M23).
- `OKDCTL_SCOS_ISO_URL` — overrides the SCOS ISO base URL (M24).
- `OKDCTL_BOOTSTRAP_OC_URL` — overrides the bootstrap-`oc` fetch (M22).
- `OKDCTL_RELEASE_SOURCE` — `image` (default) or `github`; forces the
  legacy GitHub-tarball path for OKD binaries if release-image extraction
  is unreachable (M22 fallback).

All new env vars follow M5's precedent: a `Deployment.ToolVersions`-style
YAML field complements each, with env > config > default resolution.

### 6.3a How air-gap mode is activated

Setting `OKDCTL_MIRROR_BASE` (or the equivalent `Deployment.MirrorBase`
YAML field) activates air-gap mode for the invocation — commands that
fetch external artifacts use `MirrorResolver` instead of `DefaultResolver`.
The explicit `--airgap` flag on `doctor`, `deploy`, and `destroy` is a
convenience equivalent, and `OKDCTL_AIRGAP=1` works for scripted flows
where the flag isn't ergonomic.

### 6.3 Backwards compatibility

M4 (`OKDCTL_OKD_RELEASE_URL`) and M5 (`OKDCTL_{HELM,SOPS,YQ}_URL`,
`Deployment.ToolVersions`) are preserved verbatim. `MirrorResolver`
consults them *after* `MirrorBase` rewriting, so an operator with a
mixed-layout mirror (common URL shape for most things + a one-off
non-standard path for, say, an internal helm repo) configures it exactly
as they do today. No breaking change to already-shipped surfaces.

## 7. Primary source changes (the OCI-centric pivot)

### 7.1 OKD binaries — release-image extraction (M22)

**Current:** HTTP GET of
`github.com/okd-project/okd/releases/download/{version}/openshift-install-linux-{version}.tar.gz`
and `openshift-client-linux-{version}.tar.gz`, plus `sha256sum.txt`
(`internal/distribution/okd/setup/artifacts.go:90`).

**Target:** `oc adm release extract --tools quay.io/okd/scos-release:<tag>`
extracts `openshift-install-*.tar.gz`, `oc-*.tar.gz`, and checksums into
a directory by pulling layers from the release image. The image is the
canonical build artifact — GitHub tarballs are a repackaged wrapper (every
OKD release body on GitHub carries a `Pull From: quay.io/okd/scos-release@sha256:<digest>` line).

**Bootstrap problem.** `oc adm release extract` needs an `oc` binary.
M22's approach: a one-time small `oc` fetched from
`https://mirror.openshift.com/pub/openshift-v4/clients/oc/latest/linux/oc.tar.gz`
(~20 MB). This is the `oc` Red Hat ships as the universal client for both
OCP and OKD; it handles `release extract` against either. After the
bootstrap fetch, all further binary acquisition flows through digest-pinned
image extraction.

**Air-gap story.** `quay.io/okd/scos-release:*` is rewritten via the
`MirrorBase` rule (→ `<base>/quay/okd/scos-release:*`). No per-version
config needed; the image digest pins the build. An operator staging the
mirror via `oc-mirror --v2` gets the release image for free; `oc adm
release extract` against the mirror registry just works.

**Fallback.** If `MirrorResolver` or bootstrap `oc` fetch fails,
M22's `RELEASE_SOURCE=github` env var (or `--release-source github` flag)
falls back to the GitHub-tarball path. This is a safety net for
connected-mode users with restrictive mirror proxies.

**M22 Acceptance requires:**
- Bootstrap `oc` fetch + verify via upstream checksum.
- `oc adm release extract --tools` wrapper producing the same output files
  today's code consumes (`openshift-install`, `oc`).
- The GitHub-tarball path survives as a fallback with a deprecation log
  line, removed in a later minor once the release-image path is trusted.

### 7.2 SCOS stream metadata — direct fetch for 4.19+ (M23)

**Current:** `openshift-install coreos print-stream-json` subprocess
(`internal/distribution/okd/setup/coreos.go:91-143`), stdout parsed as
JSON. Creates a bootstrap dependency — stream metadata can't be fetched
until `openshift-install` is already on disk.

**Target for OKD 4.19+:** HTTP GET of
`https://raw.githubusercontent.com/openshift/installer/release-<minor>/data/data/coreos/scos.json`,
parsed via a typed Go struct. Parallelizable with installer fetch.

**Verified availability** (inventory pass, 2026-04-19):

| minor | branch exists | `scos.json` present | size |
|---|---|---|---|
| 4.15 | yes | **no** (`fcos.json` only) | — |
| 4.16 | yes | **no** | — |
| 4.17 | yes | **no** | — |
| 4.18 | yes | **no** | — |
| 4.19 | yes | yes | 23,296 bytes, stream `c9s` |
| 4.20 | yes | yes | 24,247 bytes, stream `c10s` |
| 4.21 | yes | yes | 24,246 bytes, stream `c10s` |
| 4.22 | yes | yes | 24,246 bytes, stream `c10s` |
| 4.23 | yes | yes | 24,246 bytes, stream `c10s` |
| main | n/a | yes | same as release-4.22/4.21 |

Schema is consistent across versions that have the file (`stream`,
`metadata`, `architectures.<arch>.artifacts.metal.formats.iso.disk.{location,sha256}`).
Drop-in compatible with the existing parser at `coreos.go:103-119`.

**Dual-path for <4.19.** For 4.15–4.18 (which lack `scos.json` in the
installer repo), M23 keeps the existing `openshift-install coreos
print-stream-json` subprocess. One `if minor >= 419` at the top of
`DetectCoreOSVersion`. Users continue to see no behavior change.

> **M23 implementer note.** Before coding the conditional, re-verify the
> 4.19 floor is still the right cutover point. OKD timelines shift;
> `scos.json` may land on earlier branches retroactively, or the upstream
> path may move. Run: `curl -I https://raw.githubusercontent.com/openshift/installer/release-4.18/data/data/coreos/scos.json`
> for each minor okdctl supports, confirm the table above, update the
> `minScosDirectFetch` constant if the floor has moved.

**Never fall back to `main`** for a version-pinned request. `main` tracks
whichever release branch most recently ran `cosa2stream` — it's
at-or-newer than any versioned branch but not *specifically* matched to
a requested minor. Fallback order: requested `release-X.Y` → shellout.

**Stream name handling.** 4.19 uses `c9s`, 4.20+ uses `c10s`. Parser
accepts both; no code changes needed beyond not hard-coding the stream
name.

### 7.3 Addon Helm charts — unchanged (already OCI)

Flux pulls `oci://ghcr.io/controlplaneio-fluxcd/charts/flux-{operator,instance}`
via `helm install`. This is already OCI; air-gap redirect happens at
cluster level via mirror registry + `ImagePullSecrets`. okdctl's
contribution is the inventory declaration (§8), not a redirect mechanism.

### 7.4 Tool binaries (helm/sops/yq) — stay HTTPS

Research confirms: helm has no OCI distribution of its own binary
(`get.helm.sh` only; OCI support is for charts, not the helm CLI). sops
and yq publish OCI images, but extracting a single binary from a
container filesystem layer is worse than an HTTPS tarball — it requires
tar-layer handling, defeats the single-file-pull simplicity, and adds
`oras-go/v2` as a dep for marginal gain. Tool binaries remain HTTPS;
M21/M24 cover them under `MirrorBase` + per-fetch overrides.

## 8. Addon interface extension — `MirrorableAddon` (M25)

### 8.1 Interface shape

```go
// MirrorableAddon declares the external artifacts an addon needs for an
// air-gap deploy to succeed. Opt-in; addons that don't implement it are
// assumed to apply only in-cluster-resident manifests.
type MirrorableAddon interface {
    Addon
    MirrorArtifacts() MirrorSpec
}

type MirrorSpec struct {
    // Charts the addon pulls via helm. okdctl expands transitive images
    // at verify time via `helm template | grep image:` — addon maintainers
    // do not track images manually.
    Charts []ChartRef

    // StaticImages applies to addons that apply raw manifests with
    // image: refs (rare). For chart-based addons, leave empty.
    StaticImages []string
}

type ChartRef struct {
    OCIRef  string // "oci://ghcr.io/controlplaneio-fluxcd/charts/flux-operator"
    Version string // pinned version for digest resolution
}
```

Follows the established pattern of opt-in capability interfaces
(`ConfigurableAddon`, `ToolProvider`, `WizardProvider`) in
`internal/addon/addon.go`.

### 8.2 Initial implementers

- **`flux`** returns both chart refs (`flux-operator`, `flux-instance`)
  at the versions it pulls.
- **`secretstore`** returns an empty `MirrorSpec` — it applies CRDs and
  Secrets only; ESO itself is user-installed outside the addon's control.
  Its mirror contract surfaces via docs: operator mirrors their chosen
  secrets-store operator's chart + images at cluster level.

### 8.3 Transitive image discovery

At mirror-plan generation (M26) and at `doctor --airgap` verification
(M27), okdctl runs `helm template <chart-ref> --version <version>` and
greps `image:` refs out of the rendered YAML. The union of these images,
scoped per addon, is what the operator's mirror must contain.

This keeps the contract minimal: addon maintainers declare charts, not
transitive images, so chart-version bumps don't require code changes to
the addon.

## 9. Verification (doctor + CI)

### 9.1 `okdctl doctor --airgap` new checks (M27)

Extends the existing 9-check doctor (see `docs/doctor-checks.md`). New
checks apply only when `--airgap` is set (or `OKDCTL_AIRGAP=1`):

1. **`airgap mirror reachable`** — HEAD/registry probe each `FetchPlan`
   entry against the resolved mirror URL. Fails if any artifact is
   unreachable. Reports the concrete missing path.
2. **`airgap release image digest pinned`** — verifies
   `quay.io/okd/scos-release:<tag>` resolves to a digest and matches the
   configured pin. Cosign verification is **not available** today
   (see §12 open questions); check emits a `WARNING` noting
   digest-pinning is the only available verification mechanism, with a
   pointer at [okd#2092](https://github.com/okd-project/okd/issues/2092).
3. **`airgap addon artifacts present`** — iterates every
   `MirrorableAddon`; for each declared chart, runs the helm-template
   image extraction (cached per `(ref, version)`); HEAD-checks every
   resulting image ref in the mirror registry.
4. **`airgap bootstrap oc present`** — confirms a compatible `oc` exists
   on `$PATH` (used by M22 for release-image extraction).
5. **`airgap idms applied`** — skipped during pre-deploy doctor; run
   post-deploy to verify `ImageDigestMirrorSet` / `ImageTagMirrorSet`
   resources are applied on cluster and point at the configured mirror.

Each check produces a WARN or FAIL with a concrete remediation hint
(which URL to check, which `oc-mirror` sub-operation to re-run, etc).

### 9.2 CI verification strategy

- **Unit tests** — `FetchPlan` resolver paths (connected, air-gap,
  per-fetch override, M4/M5 backwards-compat); `airgap plan` output
  (golden-file test producing a pinned-release `ImageSetConfiguration`).
- **Integration smoke** — `httptest.NewServer` for HTTPS blob mock +
  `distribution/registry` Go library for an in-process OCI registry
  mock. doctor --airgap runs against both, validates every plan entry.
- **Explicitly out of scope** — a full air-gapped OKD deploy in CI.
  Requires a real mirror, a real cluster, and realistic network
  conditions — the maintenance cost exceeds the value at this stage.

## 10. Operator contract — `okdctl airgap plan` (M26)

### 10.1 Emitted artifacts

```
$ okdctl airgap plan --version 4.21.0-okd-scos.10 \
    --out-isc isc.yaml \
    --out-blobs airgap.yaml
```

Produces two files.

**`isc.yaml`** — valid `mirror.openshift.io/v2alpha1`
`ImageSetConfiguration`. Default form uses `mirror.platform.release`
(digest-pinned, skips Cincinnati):

```yaml
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  platform:
    release: quay.io/okd/scos-release@sha256:<pinned-digest>
    architectures: [amd64]
  additionalImages: []  # addons' transitive images populated here
  helm:
    repositories: []    # flux / secretstore chart refs populated here
```

`--channel stable-4.21` switches to graph-based mode:

```yaml
mirror:
  platform:
    graph: true
    architectures: [amd64]
    channels:
      - name: stable-4.21        # NOT 4-scos-stable (Cincinnati rejects)
        type: okd
        minVersion: 4.21.0-okd-scos.10
        maxVersion: 4.21.0-okd-scos.10
```

**Footguns the emitter handles for the operator:**

- Channel naming: always emits `stable-<minor>`. Cincinnati rejects
  `4-scos-stable`, `4-stable`, `4.21-scos-stable`. The in-tree oc-mirror
  docs show the wrong example; the unit-test fixture at
  `internal/pkg/release/cincinnati_test.go:89-94` is authoritative.
- Registry coordinate: Cincinnati returns
  `registry.ci.openshift.org/origin/release-scos@sha256:...`, not
  `quay.io/okd/scos-release`. The pin-release form avoids this by
  pinning the public `quay.io` coordinate directly.
- Signature env vars: oc-mirror's defaults target OCP signatures.
  Operators running `oc-mirror` against OKD must export:
  ```
  OCP_SIGNATURE_URL=https://storage.googleapis.com/openshift-ci-release/releases/signatures/openshift/release/
  OCP_SIGNATURE_VERIFICATION_PK=/path/to/openshift-ci-4-verifier-pk
  ```
  `okdctl airgap plan` emits a small wrapper script `run-oc-mirror.sh`
  that sets these before invoking `oc-mirror`, so the operator doesn't
  have to remember.

**`airgap.yaml`** — okdctl-internal schema listing the HTTPS blobs
`oc-mirror` doesn't handle:

```yaml
apiVersion: okdctl/v1
kind: AirgapBlobs
version: 4.21.0-okd-scos.10
blobs:
  - purpose: scos-iso
    url: https://rhcos.mirror.openshift.com/art/storage/prod/streams/c10s/builds/10.0.20260414-0/x86_64/scos-10.0.20260414-0-live-iso.x86_64.iso
    sha256: <pinned>
    mirror_path: scos/c10s/10.0.20260414-0/x86_64/scos-10.0.20260414-0-live-iso.x86_64.iso
  - purpose: tool-binary-helm
    url: https://get.helm.sh/helm-v3.14.4-linux-amd64.tar.gz
    sha256: <pinned>
    mirror_path: helm/helm-v3.14.4-linux-amd64.tar.gz
  # etc. for sops, yq, bootstrap oc, okdctl self-update metadata
```

Operators pull blobs via any method (`curl`, `wget`, `aws s3 sync`,
`rsync`); the `mirror_path` lines up with `MirrorBase`'s default 1:1
layout so nothing needs rewriting post-staging.

### 10.2 Operator workflow (documented in M28)

1. Install okdctl on the bastion. Fetch a bootstrap `oc` binary (one-time,
   ~20 MB) — either directly from `mirror.openshift.com` while the
   bastion still has internet, or from the operator's own mirror once
   staged. okdctl fails fast if `oc` is missing; a helper command
   (`okdctl airgap fetch-bootstrap-oc`) wraps the download.
2. `okdctl airgap plan --version <v> --out-isc isc.yaml --out-blobs airgap.yaml`.
3. `./run-oc-mirror.sh` (wrapper sets signature env, runs
   `oc-mirror --v2 -c isc.yaml file:///mirror`).
4. Mirror blobs: `./fetch-blobs.sh` (okdctl-emitted helper that walks
   `airgap.yaml` and rsyncs to the operator's blob server).
5. `okdctl doctor --airgap` — verifies every plan entry resolves.
6. `okdctl deploy --airgap` — runs the standard deploy against the
   mirror.

## 11. Implementation sequencing

### 11.0 Pre-implementation verification — MUST run before any item starts

The assumptions in this doc were captured 2026-04-19. OKD release cadence
and upstream tooling shift fast. **Every picker-upper of an L15
implementation item (M21–M28) must run the checks below before writing
code.** Run them as a dedicated research agent inside the roadmap-pickup
planner phase; include findings in the returned plan.

1. **Floor / release-landscape check.** Confirm which OKD minors are
   currently active on Cincinnati stable, and whether the `scos.json`
   floor (4.19 as of 2026-04-19) has moved. Concretely:
   - `GET https://origin-release.ci.openshift.org/graph?channel=stable&arch=amd64` → which minors show up?
   - For each `release-<minor>` on `openshift/installer` between 4.15 and
     current, check `data/data/coreos/scos.json` via the GitHub API.
     Update the table in §7.2 if retroactive additions landed.
   - What's the current latest stable GA? M23's `minScosDirectFetch`
     constant and M26's `--version` default adjust accordingly.
2. **`oc-mirror --v2` schema + channel check.** Re-run the `type: okd` +
   `stable-4.X` channel verification against the current `oc-mirror`
   binary. The `PlatformType` enum and channel names may drift.
   Authoritative fixture: `internal/pkg/release/cincinnati_test.go` in
   `openshift/oc-mirror@main`.
3. **Mirror rewrite rule coverage.** Re-run the §4 external-fetch
   inventory pass. If a new fetch has landed since 2026-04-19 (grep
   `http[s]?://`, `oci://`, `quay.io`, `ghcr.io`, `registry.`, `.iso`,
   `apt-get install`, `dnf install` across the repo), add it to M24's
   `MirrorBase` rule table or document the operator-side redirect.
4. **Bootstrap `oc` URL stability.** Confirm
   `https://mirror.openshift.com/pub/openshift-v4/clients/oc/latest/linux/oc.tar.gz`
   still serves a binary compatible with current OKD SCOS releases.
   Historically stable; not a formal contract.
5. **Cosign status on OKD release images.** Run `cosign tree
   quay.io/okd/scos-release:<current-tag>`. If sigstore signatures have
   landed since 2026-04-19, upgrade M27's release-image check from
   digest-pinning-with-warning to `cosign verify`. Track
   [okd#2092](https://github.com/okd-project/okd/issues/2092).

Skipping this pass = shipping code against stale assumptions. The
roadmap-pickup planner phase is the right place — the planner agent
runs these checks and returns findings alongside the implementation
plan.

### 11.1 Item sequence

Each item is one PR against `develop`. Items are written in dependency
order; later items assume earlier ones have merged.

### Core sequence

| Item | Title | Depends on | Scope | Effort |
|---|---|---|---|---|
| **M21** | FetchPlan abstraction + resolver | none | New `internal/fetchplan` package; `DefaultResolver` + `MirrorResolver`; migrate M4/M5 fetch sites into the plan; no OCI source kind wired yet; fixes yq `/releases/latest/download/` URLTemplate gap as part of the migration. | days |
| **M22** | OKD binaries via release-image extraction | M21 | Bootstrap `oc` from `mirror.openshift.com/pub/openshift-v4/clients/oc/latest/linux/oc.tar.gz`; `oc adm release extract --tools` wrapper; retire GitHub-tarball path as a fallback with deprecation log. `FetchPlan` OCI source kind fully wired. | days |
| **M23** | Direct `scos.json` fetch for 4.19+ (dual-path) | M21 | `internal/distribution/okd/setup/coreos.go` grows `minScosDirectFetch` constant; `if minor >= 419` direct GET, else existing shellout; typed Go struct for parsing. **Implementer MUST re-verify the 4.19 floor against current `openshift/installer` branches before coding the conditional.** | hours |
| **M24** | Mirror contract (`MirrorBase` + rewrite rules) | M21 | `OKDCTL_MIRROR_BASE` env + `Deployment.MirrorBase` YAML; rewrite rules in `MirrorResolver`; `OKDCTL_UPDATE_CHECK_URL`, `OKDCTL_SCOS_STREAM_URL`, `OKDCTL_SCOS_ISO_URL`, `OKDCTL_BOOTSTRAP_OC_URL` additions; backwards-compat with M4/M5 per-fetch overrides. | days |
| **M25** | `MirrorableAddon` interface + migrations | M21 | New opt-in sub-interface in `internal/addon/addon.go`; flux returns its two charts; secretstore returns empty. Helm-template image extraction helper in `internal/addon/mirror/`. | days |
| **M26** | `okdctl airgap plan` subcommand | M22, M24, M25 | New `internal/cli/airgap.go`; emits `ImageSetConfiguration` + `airgap.yaml` + `run-oc-mirror.sh` + `fetch-blobs.sh` wrapper scripts; pinned-release default with `--channel` opt-in to graph mode. | days |
| **M27** | `okdctl doctor --airgap` | M21, M25, M26 | New doctor checks per §9.1; walks FetchPlan + MirrorableAddon specs; HEAD/registry-probe reachability; digest-pinning + warning for OKD release image. | days |
| **M28** | Docs: mirror contract reference + operator runbook | M21–M27 | `docs/airgap/mirror-contract.md` (the `MirrorBase` rewrite table + per-fetch override cheat sheet); `docs/airgap/operator-runbook.md` (the 6-step workflow from §10.2); README air-gap section pointer. | hours |

### Adjacent items (file alongside L15 items, independent of the sequence)

| Item | Title | Depends on | Notes |
|---|---|---|---|
| **M29** | GitHub Artifact Attestations on okdctl releases | none | ~10 lines of YAML in `.github/workflows/release.yml` (permissions already pre-provisioned at lines 7-10). Additive to existing cosign/SLSA flows. Effort: hours. |
| **M30** | `oras-go/v2` as direct OCI client | deferred | Filed deferred — no current use case. Triggers: non-helm OCI artifacts, air-gap proxy semantics, BYO-OCI addon flow. |
| **M32** | Embed OKD maintainer GPG pubkey for tarball verification | blocked on okd#2092 | Once OKD publishes the `maintainers@okd.io` pubkey canonically, okdctl can GPG-verify `sha256sum.txt.asc` against `openshift-install` tarballs. Tracking upstream; not actionable today. |

### Sequencing diagram

```
M21 (FetchPlan)
 ├─ M22 (release-image extraction)
 ├─ M23 (scos.json direct)
 ├─ M24 (mirror contract)
 └─ M25 (MirrorableAddon)
      └─ M26 (airgap plan)
           └─ M27 (doctor --airgap)
                └─ M28 (docs)

M29 (attestations) — independent; file anytime
M30 (oras-go) — deferred
M32 (OKD pubkey) — blocked upstream
```

### Estimated total effort

Sequence: ~6 `days`-effort items + 2 `hours`-effort items = **roughly a
month of single-engineer focused work**, consistent with the roadmap's
"full implementation is quarters" rubric after accounting for review
cycles and inter-item validation.

## 12. Out of scope / explicitly deferred

- **Orchestrating the mirror itself.** No `okdctl mirror init|sync`.
  `oc-mirror v2` + operator tooling cover this.
- **Distro package redirect.** apt/dnf pulling `coreos-installer`,
  `haproxy`, `httpd`, `dnsmasq`, `terraform`, `apt.releases.hashicorp.com`
  GPG + repo — operator's distro package-manager config handles this.
  okdctl documents the package list (M28).
- **Terraform provider redirect.** `terraform init` pulling
  `bpg/proxmox` from `registry.terraform.io` — operator configures
  `TF_CLI_CONFIG_FILE` with a `filesystem_mirror` block. okdctl
  documents the pin (M28).
- **Cluster-runtime image pulls.** Addon container image pulls happen
  at cluster runtime via kubelet. Operator applies `IDMS` / `ITMS`
  (from `oc-mirror` output) + configures `ImagePullSecrets`. okdctl
  declares the inventory via `MirrorableAddon`; cluster config handles
  the rewrite.
- **FCOS → SCOS rename.** Internal/field renaming is a separate
  cleanup; unrelated to air-gap. Current supported range (4.15+)
  spans both families, so naming stays generic.
- **Full air-gap CI smoke test.** Real-mirror + real-cluster in CI is
  quarters of work on its own. Unit + mock-server integration is the
  L15 bar.
- **`oras-go/v2` library adoption.** Deferred to M30 pending a real
  use case; helm and oc cover all current OCI needs.

## 13. Open questions (track, verify before implementation PRs)

1. **Cosign coverage on OKD release images.** Currently: no sigstore
   signatures attached to `quay.io/okd/scos-release` or
   `quay.io/openshift-release-dev/ocp-release`. Legacy atomic
   signatures exist (GPG, `maintainers@okd.io`) but the store is not
   at a public HTTPS endpoint. Tracked upstream as
   [okd#2092](https://github.com/okd-project/okd/issues/2092). M27's
   check emits a WARNING and relies on digest pinning; upgrade to
   `cosign verify` when/if upstream adopts it.
2. **`oc-mirror --v2` signature env-var resilience.** The
   `OCP_SIGNATURE_URL` + `OCP_SIGNATURE_VERIFICATION_PK` dance is
   brittle. M26's `run-oc-mirror.sh` wrapper scripts it, but the
   URLs/keys may drift; run a smoke against current oc-mirror before
   M26 ships and update the wrapper.
3. **`scos.json` availability on pre-4.19 installer branches.**
   Re-verify the 4.19 floor before M23 codes the conditional. OKD
   timelines shift; `scos.json` may land on earlier branches
   retroactively, or the upstream path may move. One curl per minor;
   update `minScosDirectFetch` if the floor has changed.
4. **`mirror.openshift.com` bootstrap-`oc` stability.** M22 fetches
   `https://mirror.openshift.com/pub/openshift-v4/clients/oc/latest/linux/oc.tar.gz`
   as the bootstrap. Red Hat has historically kept this path stable,
   but it's an OCP distribution point shared with OKD by convention.
   Fallback: `--release-source github` uses the GitHub-tarball path
   (which still works until removed in a later minor).
5. **MirrorBase rewrite rule coverage.** The §6.1 table covers every
   fetch cataloged in §4.1. Before M24 merges, re-run the inventory
   (grep `http[s]?://`, `oci://`, `quay.io`, `ghcr.io`, `registry.`
   across the repo) and confirm no new fetch has slipped in since
   2026-04-19.

## 14. Prior art

- **`oc-mirror` v2** — `github.com/openshift/oc-mirror`. OKD support
  via `TypeOKD` enum + `okdClient`; Cincinnati at
  `https://origin-release.ci.openshift.org/graph`. Authoritative
  implementation of `ImageSetConfiguration`.
- **Red Hat disconnected-install docs** — the `oc adm release extract
  --tools` + `IDMS` pattern. okdctl's M22/M24 design mirrors this
  workflow so operators reuse muscle memory.
- **OKD release pipeline** — `github.com/okd-project/okd-release-pipeline`;
  `github.com/openshift/cluster-update-keys` for signature-store URLs
  and verifier keys. Context for §13 open question 1.
- **okdctl's own M4/M5 precedent** — the env > config > default
  pattern ships; `FetchPlan` generalizes it. M21 migrates M4/M5 into
  the plan without breaking their surfaces.

## 15. References

- [OKD project README](https://github.com/okd-project/okd)
- [OKD disconnected install docs](https://docs.okd.io/latest/disconnected/about-installing-oc-mirror-v2.html)
- [oc-mirror upstream](https://github.com/openshift/oc-mirror)
- [oc-mirror OKD channel PR #117](https://github.com/openshift/oc-mirror/pull/117) (closed #102)
- [openshift/installer scos.json](https://raw.githubusercontent.com/openshift/installer/main/data/data/coreos/scos.json)
- [Cincinnati graph for OKD](https://origin-release.ci.openshift.org/graph?channel=stable&arch=amd64)
- [`mirror.openshift.com` bootstrap oc](https://mirror.openshift.com/pub/openshift-v4/clients/oc/latest/linux/oc.tar.gz)
- [okd-project/okd#2092 — sigstore adoption tracking](https://github.com/okd-project/okd/issues/2092)
- [oras-go v2](https://pkg.go.dev/oras.land/oras-go/v2) (for M30 if promoted)
- [GitHub Artifact Attestations](https://docs.github.com/en/actions/concepts/security/artifact-attestations) (for M29)
- CLAUDE.md §Architecture notes (this repo) — canonical helpers and
  declaration-as-data pattern (`StepDef` / `BuildSteps`) that `FetchPlan`
  follows.

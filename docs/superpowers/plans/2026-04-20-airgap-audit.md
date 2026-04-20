# M34 — Air-gap architecture review + scoping

> **Scope of this document:** zoom-out architectural review of okdctl's
> air-gap surface, post-M21–M27 ship (2026-04-20). The deliverable is this
> scoping doc; no code ships from M34. Follow-ups land as subsequent
> roadmap items.

**Status:** draft
**Date:** 2026-04-20
**Roadmap item:** M34
**Preceded by:** L15 (2026-04-19 architecture), M21–M27 + M33 (implementation, merged 2026-04-20)
**Blocks:** M28 final docs; shapes M35/M36/M37 follow-ups filed below

---

## 1. Summary

Three months into the L15 air-gap workstream, the abstraction earns its
keep but has **two structural issues** worth addressing before M28 closes
the sprint: a duplicated mirror-rewrite table between `internal/cli/airgap.go`
and `internal/fetchplan/fetchplan.go`, and a surprise-prone activation
trigger where a bare `MirrorBase` config entry silently flips the deploy
into air-gap mode. Everything else is defensible through ship. Peer
research (RKE2, k3s, k0s, Talos, oc-mirror, Hauler) confirms okdctl is
positioned correctly as a **tool-orchestrates-plan, operator-stages-mirror**
hybrid; the specific bets (one `MirrorBase` + host-prefix rewrite; `ISC`
wrapper that delegates OCI mirroring to `oc-mirror`) are internally
consistent and align with the industry's hybrid consensus. No
re-architecture is warranted; three small tightening items and two new
concrete follow-ups are.

This doc is the pivot point: M28 absorbs the documentation bullets, the
three tightening items file as M35 (activation), M36 (rewrite-table
de-duplication), M37 (Go-subcommand replacements for the emitted shell
scripts).

## 2. Scope

**In scope** — the air-gap workstream surface as of 2026-04-20:

- `internal/fetchplan/` — `Plan`, `Resolver` interface, `DefaultResolver`,
  `MirrorResolver`, `EnvOverrideResolver`, `PickResolver`, 9 `Purpose`
  constants, 4 `Env*URL` escape-hatch constants, 9 `Build*Plan`
  functions.
- `internal/addon/mirror/` — `ChartImages`, `SpecImages`, `AddonImages`
  helpers that run `helm template` and walk rendered YAML.
- `internal/distribution/okd/setup/release_extract.go` — bootstrap `oc`
  fetch, `oc adm release extract --tools` wrapper, auth-error heuristic.
- `internal/distribution/okd/setup/coreos.go` — `DetectCoreOSVersion`,
  dual fcos/scos path.
- `internal/distribution/okd/setup/tools.go` — `installTool` M5 resolver
  path.
- `internal/distribution/okd/setup/artifacts.go` — `DownloadOKDTools`
  driving `bootstrapOC` + `extractReleaseImage`.
- `internal/distribution/okd/okd.go` — `WithAirgap`, `PickResolver`
  threading through `Prepare` and `Configure`.
- `internal/cli/airgap.go` — `okdctl airgap plan`; emits `isc.yaml`,
  `airgap.yaml`, `run-oc-mirror.sh`, `fetch-blobs.sh`.
- `internal/cli/doctor_airgap.go` — 5 `--airgap` doctor checks.
- `internal/cli/deploy.go`, `destroy.go`, `helpers.go`, `root.go`, `doctor_cmd.go` — `--airgap` flag wiring.
- `internal/version/updatecheck.go` — resolver-mediated update-check.
- `internal/addon/catalog/flux/flux.go` — `resolveChartRef`,
  `MirrorArtifacts`, `helmUpgradeInstall`.
- `internal/addon/addon.go` — `Environment.Resolver`, `MirrorableAddon`,
  `MirrorSpec`, `ChartRef`.

**Out of scope** — code paths untouched by the air-gap workstream:
wizard, netutil, validators, tui, provisioners/proxmox, cleanup,
destroy (beyond the `--airgap` flag itself).

**Pivot note.** M34's original acceptance text called for `audit-all`
with a 14-skill fan-out. After scoping-conversation course-correction,
the pass was redirected toward architectural zoom-out: peer research,
shape assessment, alternatives enumeration. The bug-level sweep is
deferred to a future release-readiness pass (new item **M38** below).

## 3. Method

Three strategic agents ran in parallel:

1. **Peer research** (web) — surveyed RKE2, k3s, k0s, cluster-api,
   openshift-installer disconnected, oc-mirror v2, Flatcar / SCOS,
   Talos / Sidero, Hauler, zot, ORAS-go, Helm OCI, containerd
   `hosts.toml`. Goal: benchmark okdctl's contract against the field.
2. **Architectural code-read** — read the ~20 in-scope files with an
   architectural lens. Goal: name the *shape*-level smells, not the
   call-site nits. Delivered a 10-entry smells table + answers to 12
   pre-seeded questions.
3. **Alternatives evaluation** — for each named decision, enumerated
   two concrete alternatives with trade-offs. Delivered a
   10-decision matrix + explicit recommendations (including "keep
   current" where that's the call).

The findings those agents produced are summarised inline below in §5–§10.
No code was modified.

## 4. Current architecture — recap

One paragraph for the reader who hasn't read L15. okdctl declares
every external artifact as typed data (`fetchplan.Plan` with `[]Blob`
and `[]OCIArtifact`). A `Resolver` interface maps each plan entry to
the URL actually fetched. `DefaultResolver` returns upstream refs
verbatim; `MirrorResolver` rewrites known upstream hosts (quay, ghcr,
registry.ci.openshift.org, get.helm.sh, github.com, api.github.com,
rhcos.mirror.openshift.com, raw.githubusercontent.com,
mirror.openshift.com) under a single `MirrorBase` URL using a 1:1
host-prefix layout (`<base>/quay/...`, `<base>/ghcr/...`).
`EnvOverrideResolver` wraps either, honouring four `OKDCTL_*_URL`
per-fetch escape hatches. `PickResolver(cfg, airgapFlag)` chooses the
chain at CLI entry; `okd.WithAirgap(true)` threads it through
provisioner phases. `okdctl airgap plan` emits operator artifacts that
`oc-mirror --v2` consumes; `okdctl doctor --airgap` HEAD-probes each
plan entry through `MirrorResolver`.

## 5. Peer comparison

| Project | Who owns mirror | Redirect primitive | Bootstrap binary | Helm story | Cosign story |
|---|---|---|---|---|---|
| **oc-mirror v2** | tool orchestrates | emits IDMS+ITMS | N/A (library+CLI) | first-class `helm.charts[]` in ISC | `OCP_SIGNATURE_URL` env, brittle |
| **openshift-installer disconnected** | operator stages via oc-mirror | `install-config.imageDigestSources` → cluster IDMS | `oc` fetched separately | oc-mirror owns it | OCP signed; OKD isn't (okd#2092) |
| **RKE2** | tool (tarball) or operator (registries.yaml) | `registries.yaml` + embedded distributed registry mirror | single bash installer + air-gap tarball | helm-controller re-templates via `registries.yaml` | Chainguard variants; not enforced |
| **k3s** | tarball OR registries.yaml | `/var/lib/rancher/k3s/agent/images/` auto-import + `registries.yaml` | single binary | inherits from containerd mirror | inherited from RKE2 |
| **k0s** | tool (`k0s airgap bundle`) | OCI Image Layout tarball auto-imported | single binary | declared in `k0s.yaml` | no in-tree signing |
| **cluster-api** | operator (per-provider) | `clusterctl` per-component image overrides | N/A | per-provider | not enforced |
| **Talos** | operator (machine config) | `machine.registries.mirrors` | install image ref in config | N/A (OS layer) | Talos binaries signed; not enforced |
| **Flatcar** | operator (Nebraska or local payload) | update-server URL override | `flatcar-update` | N/A | Omaha-protocol sigs |
| **Hauler** | tool-serves (embedded registry + fileserver) | one static binary with `:5000` + `:8080` | one static binary | first-class content type | cosign-verify on store |
| **zot** | registry-as-product | pull-through mirroring | N/A | OCI-native | sigstore artifacts |
| **okdctl (this repo)** | **operator stages, okdctl verifies** | `MirrorBase` + host-prefix rewrite + IDMS at cluster | bootstrap-oc from `mirror.openshift.com` | `resolveChartRef` (bastion) + IDMS (cluster) | WARN + digest-pin (okd#2092) |

**Where okdctl sits.** Industry consensus in 2026 clusters around two
poles: tool-owns-it-all (RKE2 / k3s / k0s tarballs) and
operator-owns-it-all (Talos / CAPI config-driven). okdctl occupies the
hybrid middle — the same slot as `oc-mirror v2` — and is
architecturally correct for its product niche (OKD follows OCP, which
normalised this model). The middle is not novel, but it is the right
lane.

**Where okdctl is distinctive.** Two bets nobody else makes identically:
(a) **one `MirrorBase` URL + host-prefix rewrite** — a variant of
Hauler's "one store, many upstreams" but configured at the client not
the registry; (b) **an `ImageSetConfiguration` wrapper that includes
addon charts** — no peer emits a wrapper ISC. The first is
internally-consistent and testable; the second is arguably novel value.

**Where okdctl is an outlier.** **Four per-fetch env-var escape
hatches** (`OKDCTL_{UPDATE_CHECK,SCOS_STREAM,SCOS_ISO,BOOTSTRAP_OC}_URL`).
No peer ships more than one. k3s ships zero (tarball drop-in); RKE2
ships one (`INSTALL_RKE2_ARTIFACT_PATH`). The field treats per-fetch
vars as debug surface, not supported contract. See finding §6.4.

## 6. Architectural smells

Ten findings, severity-ranked. Cites the audit that produced each one:
**AR** (architectural code-read), **PR** (peer research). Out of
scope: general bug-hunt findings (deferred to M38).

| # | Smell | Severity | Where | Why it smells | Audit |
|---|---|---|---|---|---|
| 1 | **Three fetch patterns coexist in the air-gap path** | arch-blocker | `fetchplan.go` (Plan builders); `coreos.go:191-217` (`DetectCoreOSVersion` calls `resolver.ResolveBlob` inline); `airgap.go:140-165` + `airgap.go:262-302` (own `httpStreamFetcher` + `buildAirgapPlan` that doesn't use the resolver chain for the operator-facing plan build) | Post-M33 the coexistence is **structural**, not transitional. `coreos.go` receives a resolver and fetches directly; `airgap.go` reaches past `PickResolver` to build its plan with a private fetcher. Every fetch should flow through `PickResolver` at CLI entry. | AR |
| 2 | **Mirror-rewrite table duplicated across two files** | high | `fetchplan.go:69-84` (`mirrorBlobRules`); `airgap.go:311-318` (identical `prefixTable` in `mirrorPath`) | Single source of truth violated. A typo or new host added to one and not the other silently defeats air-gap for that host. Drift vector matures on the next rewrite-rule addition. | AR |
| 3 | **MirrorResolver pass-through on unrecognised hosts** | medium | `fetchplan.go:98-101`, `fetchplan.go:118-121` | Unknown host falls through unchanged — future-proofing the table, but a typo in `mirrorOCIRules` silently defeats air-gap for *that* registry. Fail-closed (return an explicit `ConfigError` naming the missing host) is safer. | AR |
| 4 | **Per-fetch env-var escape hatches are an outlier pattern** | medium | `fetchplan.go:208-213`, `fetchplan.go:247-253` | No peer ships more than one per-fetch override. okdctl ships four and has pressure to add a fifth (CoreOS mirror, once that lands). The env-var surface is a debug contract masquerading as happy-path config. | PR |
| 5 | **Air-gap activation has three independent triggers** | medium | `fetchplan.go:164-172` (`IsAirgap` ORs env + flag + MirrorBase-set) | Setting `MirrorBase` once in config silently air-gaps every subsequent command — surprise vector. Explicit-only (flag or env) is cleaner; `MirrorBase` becomes the *target* of air-gap mode, not its *trigger*. | AR |
| 6 | **Shell-script artifact emission is a maintenance surface** | medium | `airgap.go:423-457` (`renderOCMirrorSh`, `renderFetchBlobsSh`) — the latter embeds Python-3 inside `sh` heredoc | Python-in-sh heredoc is unit-untested, drifts with PyYAML installer defaults, adds a Python-3 dependency to the bastion. Go subcommands (`okdctl airgap fetch-blobs`, `okdctl airgap run-oc-mirror`) keep the operator UX identical and are testable. | AR |
| 7 | **EnvOverrideResolver precedence is undocumented** | low | `fetchplan.go:189-190` (env beats MirrorBase) | Current order (env > config > default) is correct for break-glass flows but not explained in CLAUDE.md or L15 §6.3. Operators who reason about it read source. | AR |
| 8 | **doctor_airgap / airgap / fetchplan have no shared airgap package** | low | `doctor_airgap.go` owns `httpHeadFunc`/`registryHeadFunc`; `airgap.go` owns `streamFetcher`/`shaFetcher`/`mirrorPath`; `fetchplan.go` owns the rules | Separation-of-concerns is defensible; but the duplicated rewrite table (#2) is the tell that the seam is wrong. A future `internal/airgap` package owning probes + rewrite + ISC emission is the right north star. | AR |
| 9 | **bootstrap-oc URL is hardcoded** | low | `fetchplan.go:376` (`const bootstrapOCURL`) | Only the `Env*URL` override (`EnvBootstrapOCURL`) redirects it; no config-file knob. Consistent with current env-var pattern; will be revisited when env vars consolidate. | AR |
| 10 | **Resolver has no introspection** | low | `fetchplan.go:44-48` | No way to ask the resolver "what rules do you have?" for debugging / docs generation. Could help `doctor --airgap` report which hosts are mirrored before probing. Low-priority nice-to-have. | AR |

**Load-bearing strengths** (do not lose in follow-up refactors):

- `Purpose` + `Plan` declaration-as-data follows the repo's `StepDef` /
  `BuildSteps` convention and composes well across the resolver chain.
- `EnvOverrideResolver` as a decorator over `Inner` is the right
  pattern for break-glass overrides; kills a big class of
  fetch-site `os.Getenv` calls.
- Two-method `Resolver` interface (`ResolveOCI` / `ResolveBlob`)
  asymmetry is justified: OCI refs and HTTPS URLs genuinely have
  different transforms; a single method would force a type-switch
  into every implementation.

## 7. Decision analysis

Ten decisions with concrete alternatives. Per the alternatives agent.
"Current cost" / "Migration cost" rated against the audit scale: hours
/ days / weeks.

| # | Decision | Current | A | B | Recommendation |
|---|---|---|---|---|---|
| 1 | Resolver interface shape | 2 methods (ResolveOCI/Blob) | single polymorphic Resolve | no interface, plain function | **keep current** — two transforms genuinely differ |
| 2 | Rewrite-rule representation | hardcoded map[string]string | user-supplied `Deployment.MirrorRules` | operator writes per-fetch URLs in YAML | **keep current through M28; revisit A in a later minor** — 1:1 invariant is L15's product thesis |
| 3 | Escape-hatch env vars | 4 per-fetch vars | zero env (YAML only) | single var `OKDCTL_AIRGAP_OVERRIDES=k=v,k=v` | **keep current** — 4 vars is under the ergonomic cliff; revisit at 8+ |
| 4 | Air-gap activation | env OR flag OR MirrorBase-set | explicit-only | mode-less | **A (explicit-only)** — closes surprise-vector cheaply before M27 rollouts mature |
| 5 | Operator contract depth | emit ISC + blobs + scripts; verify | thinner (inventory only) | thicker (okdctl runs oc-mirror) | **keep current, but replace shell scripts** — cross-refs #9 below |
| 6 | Addon chart-pull redirect | `resolveChartRef` at bastion + IDMS at runtime | bastion-native (`helm pull` → `helm install` tarball) | cluster-native (IDMS + registry-mirror CR handles everything) | **keep current** — B is blocked by deploy-order chicken-and-egg; A adds cache layer for marginal gain |
| 7 | Release-image extraction | bootstrap-oc + `oc adm release extract` | skopeo | oras-go/v2 in-process | **keep current through M27; revisit B when M30 unblocks** — `oc` knows which layer holds tools |
| 8 | Cosign verification gap | WARN + doc note | accept indefinitely | self-sign okdctl sidecar | **accept (A)** + 6-month upstream re-check; B is security theater; M32 remains blocked on okd#2092 |
| 9 | airgap plan emission | 4 artifacts (2 YAML + 2 scripts) | drop shell scripts | fold blobs into ISC via blob→OCI coercion | **hybrid** — replace scripts with Go subcommands; keep both YAMLs (different consumer lifecycles) |
| 10 | File split across doctor_airgap / airgap / fetchplan | 3 files, rule table duplicated | extract `internal/airgap` package | accept split + doc anchors | **fix duplication first (A scoped to rules), defer full package extraction** — fold into M28 or a dedicated M36 |

### Effort matrix

| Decision | Current cost | → A | → B | Payoff |
|---|---|---|---|---|
| 1 | low | hours | days | minimal — keep |
| 2 | low | days | weeks | medium — defer |
| 3 | low | days | hours | low — keep |
| 4 | medium (surprise) | hours | days | **high — A now** |
| 5 | medium (bash) | days | weeks | **high — hybrid with #9** |
| 6 | low | days | weeks | low — keep |
| 7 | medium (bootstrap-oc dep) | days | weeks | defer to M30 trigger |
| 8 | low | hours | days | low — accept |
| 9 | medium (bash smells) | hours | weeks | **high — combined with #5** |
| 10 | medium (rule-table drift) | days | hours | **high — fix dup first** |

## 8. Chart-pull determination

Per M34 acceptance §3: "does flux's bastion-side `helm install
oci://ghcr.io/...` require ghcr.io reachability today, or does the
resolver rewrite cover it?"

**Determination: covered.** M33's `resolveChartRef` at `flux.go:132-147`
routes bare chart refs through `env.Resolver.ResolveOCI(...)`; the
rewritten ref is prefixed with `oci://` before `helm install`.
Cluster-runtime container-image pulls are a separate concern, handled
by `ImageDigestMirrorSet` / `ImageTagMirrorSet` that the operator
applies from `oc-mirror`'s output (L15 §10.2 step 6).

**Residual risk.** The fallback path at `flux.go:143-144` silently
degrades to the upstream ref on resolver error, logging a `Warn`.
In air-gap mode that's a silent failure mode — the bastion reaches
for `ghcr.io`, times out, operator sees an addon-install failure
instead of a clear "resolver returned error" message. Propose a
follow-up: when `IsAirgap` is active, the fallback should **fail
loudly**, not warn-and-upstream. Filed as **M35** below.

**No standalone item needed.** The chart-pull was a gap when M34 was
filed (2026-04-20); M33 closed it earlier that same day. The one
hardening delta (fail-loud in air-gap) rolls into M35.

## 9. Integration smoke plan

Per M34 acceptance §4. This is a **plan**, not a prescription to ship
code from M34.

**Harness shape.** A single `TestAirgapDeploy_smoke` test in
`internal/fetchplan/airgap_integration_test.go` (new file) or under
`internal/cli/airgap_test.go` (existing file):

1. **Fixture 1 — HTTPS blob mock.** `httptest.NewServer` serving:
   `/helm/...` (200 + tarball bytes), `/rhcos/...` (200 + ISO bytes),
   `/github/...` (200 + release-tarball-lookalike),
   `/raw-github/...` (200 + `scos.json` payload from
   `internal/cli/testdata/airgap/`), `/openshift-mirror/...` (200 +
   bootstrap-oc tarball stub).
2. **Fixture 2 — OCI registry mock.** Start an in-process OCI
   registry using `github.com/distribution/distribution/v3` (the
   acceptance named the dep). **Dep justification per CLAUDE.md
   §Dependencies**: Apache-2.0, permissive; supply-chain signal is
   `distribution/distribution` (the reference implementation of the
   OCI distribution spec). Minimum version pin to the latest stable.
   No in-stdlib replacement (Go 1.25 has no OCI registry). Added as
   a **test-only import** — ships in `_test.go` files; does not
   inflate the release binary. Alternative: stdlib
   `httptest.Server` handling `/v2/*/manifests/` and
   `/v2/*/blobs/*` with `application/vnd.oci.image.manifest.v1+json`
   content type. The stdlib path covers the HEAD-probe check code
   paths today (per M27's existing hermetic test), so the
   `distribution/v3` dep is only justified if a future extract-via-registry
   path (M30) lands. **Plan-level recommendation: start with the
   stdlib path; only add the `distribution/v3` dep when a real
   OCI-pull test surfaces.**
3. **Stubbed subprocess executors.** `oc adm release extract` and
   `helm install/template/pull` are exec'd today. For the smoke,
   wrap them in interfaces (or use the existing `executor.Executor`
   with an injected `CommandRunner`) that record their argv and
   return canned success. Assertions: `oc`'s argv contains the
   mirror-rewritten ref (not upstream); `helm template`'s argv
   contains the oci-rewritten chart ref.
4. **Flow exercised.** `PickResolver(cfg, true)` →
   `DownloadOKDTools(ctx, version, opts)` (resolver-mediated
   bootstrap-oc fetch + release-image extract) → addon chart
   discovery via `addon.All()` → `resolveChartRef` rewrite →
   `addonmirror.SpecImages` with stubbed helm → assertion that
   every resolved URL/ref points at the fixture servers, **no
   upstream host contacted**.
5. **Assertions.** (a) The fixture servers receive every expected
   request with the expected path prefix. (b) No request reaches
   `quay.io`, `ghcr.io`, `github.com`, `get.helm.sh`,
   `rhcos.mirror.openshift.com`, `mirror.openshift.com`, or
   `raw.githubusercontent.com`. Use `httptest`'s Go-level network
   isolation plus an explicit "no upstream" guard (a custom
   `http.RoundTripper` that fails on any non-fixture host).

**CI time budget.** Target <10s for the smoke; the fixtures are
entirely in-process. Acceptable for `test-go` CI job.

**Where the harness lives.** `internal/cli/airgap_test.go` already
has a `fixtureStream` / `fixtureShaFetcher` pattern; extend that
for the in-process OCI + blob fixtures. Avoids a new package.

**Deliverable when executed.** Ships as **M36** below. Not part of
M34.

## 10. Top three actionable follow-ups

Concrete bullets the next roadmap session can pick up as-is.

### M35 — Air-gap hardening sweep (merge mode + fail-loud resolve)

- **Status:** not started
- **Category:** refactor / correctness
- **State:** scoping done (M34)
- **Effort:** days
- **Impact:** medium (closes 3 surprise-vectors before M28 docs land)
- **Acceptance:**
  - §6.5 activation: `IsAirgap` drops the `MirrorBase != ""` OR
    clause; air-gap requires explicit `--airgap` flag or
    `OKDCTL_AIRGAP=1`. `MirrorBase` stays as the target of air-gap
    mode, not its trigger. L15 §6.3a amended.
  - §8 chart-pull fail-loud: `flux.go:resolveChartRef` returns an
    error (not a warn-and-upstream log) when `env.IsAirgap` is
    true and the resolver fails. Tested.
  - §6.3 MirrorResolver fail-closed on unknown host: when air-gap
    is active and the resolved artifact's host is not in
    `mirrorBlobRules`/`mirrorOCIRules`, return a `*ConfigError`
    naming the host instead of passing through silently.
  - EnvOverrideResolver precedence documented (env > config > default;
    break-glass-friendly) either in CLAUDE.md or as an amendment to L15
    §6.3 — picker-upper's call.
- **Depends on:** none (all in-file fixes).

### M36 — De-duplicate mirror-rewrite rule table

- **Status:** not started
- **Category:** refactor / code health
- **State:** scoping done (M34)
- **Effort:** hours
- **Impact:** medium (eliminates drift vector §6.2)
- **Acceptance:**
  - One canonical `mirrorBlobRules` + `mirrorOCIRules` source in
    `internal/fetchplan/fetchplan.go` (exported or via helper).
  - `internal/cli/airgap.go:mirrorPath` consumes the canonical
    source; no private copy of the prefix table.
  - Test: if a new host is added to the canonical rules, both
    `MirrorResolver.ResolveBlob` and `airgap plan`'s `mirror_path`
    field reflect the new prefix.
  - Full package extraction (`internal/airgap`) is **not** in
    scope for M36; file as a subsequent item only if M37's
    Go-subcommand refactor surfaces additional shared surface.
- **Depends on:** none.

### M37 — Replace shell-script emission with Go subcommands

- **Status:** not started
- **Category:** refactor / UX
- **State:** scoping done (M34)
- **Effort:** days
- **Impact:** medium (removes bash+Python surface; testability)
- **Acceptance:**
  - `run-oc-mirror.sh` → `okdctl airgap run-oc-mirror` (Go; sets
    `OCP_SIGNATURE_URL` + `OCP_SIGNATURE_VERIFICATION_PK` in the
    child env; shells to `oc-mirror --v2 -c isc.yaml
    file:///mirror`).
  - `fetch-blobs.sh` → `okdctl airgap fetch-blobs [--staging
    ./blobs]` (Go; reads `airgap.yaml`; downloads each blob to
    `./blobs/<mirror_path>`; reuses `internal/download` for
    retries + checksums).
  - Both subcommands emit deterministic status to stderr; both are
    unit-tested with the existing golden-file harness.
  - `airgap plan` continues to emit the two YAML files but no
    longer emits shell scripts. The operator-runbook (M28)
    documents the two subcommands in place of the scripts.
  - Removes the Python 3 + PyYAML runtime requirement from the
    bastion.
- **Depends on:** none, but coordinates with M28 which documents
  the operator runbook.

## 11. Other follow-ups filed

### M38 — Air-gap release-readiness bug sweep

- **Status:** deferred
- **Category:** audit / quality
- **State:** deferred (M34 pivoted away from bug-level sweep)
- **Effort:** days
- **Impact:** medium (pre-release polish; not blocking)
- **Evidence:** M34's original acceptance called for `audit-all`
  across 14 skills. The scoping pivot redirected the pass toward
  architectural zoom-out; the bug-level sweep is still worth
  running once M28 docs land, before any public "air-gap supported"
  messaging.
- **Acceptance:**
  - Run `audit-all` scoped to the air-gap files listed in M34 §2.
  - File actionable findings as M39+ items (or close as accept).
  - Target findings: credential-leak patterns in error logs;
    TLS-validation on mirror HEAD probes; subprocess timeouts on
    `oc adm release extract` under slow mirrors; error-wrapping
    consistency; test coverage gaps beyond what M36/M37/the M34 §9
    smoke plan addresses.
- **Depends on:** M28 (runs after docs land so the sweep includes
  doc-drift checks).

### M39 — Evaluate k3s-style tarball drop-in pattern for HTTPS blobs

- **Status:** deferred
- **Category:** feature / UX
- **State:** deferred (backlog, post-M28)
- **Effort:** weeks
- **Impact:** medium-large (lowest-friction UX in the field; would
  replace `fetch-blobs.sh` flow and obviate multiple `Env*URL`
  overrides)
- **Evidence:** Peer research (M34 §5) flagged k3s's
  `/var/lib/rancher/k3s/agent/images/` auto-import as the
  lowest-friction air-gap UX. An okdctl analog — a
  `<workdir>/airgap-blobs/` directory scanned at deploy start — would
  replace the operator-runs-script step. Only worth picking up if
  operator feedback says `fetch-blobs.sh`/M37's equivalent is
  ergonomically insufficient.
- **Acceptance:** **design-doc-first.** Scoping doc that covers:
  directory layout, discovery semantics, interaction with the
  existing `fetchplan`+`Resolver` chain (does a drop-in supersede
  the resolver entirely? or is it a last-resort fallback?),
  compatibility with the 1:1 `MirrorBase` invariant.
- **Depends on:** M28 ship + operator feedback.

## 12. Open questions (not blocking; track for future)

1. **`oc-mirror v2` helm-version format.** Does
   `mirror.helm.charts[].version` accept both `v1.2.3` and `1.2.3`
   shapes? Flux chart refs use both conventions. 5-minute smoke
   worth running before M25 accrues deployed users.
2. **OCI-artifact shape for blob manifests.** ORAS supports
   arbitrary blobs as OCI artifacts; Hauler does this. Would
   `airgap.yaml` become redundant if tool tarballs were pushed as
   OCI artifacts? Need to confirm oc-mirror's arbitrary-ORAS
   support before considering a v2 scheme.
3. **Flatcar / Nebraska signature scheme vs SCOS.** Flatcar ships
   Omaha-protocol-signed update payloads. SCOS via
   `rhcos.mirror.openshift.com` publishes only `sha256`. If SCOS
   signs payloads anywhere, that's a cosign-free integrity story
   okdctl could lean on. Couldn't verify from public sources.
4. **Containerd `hosts.toml` wildcard support.** The wildcards
   discussion (containerd#6444) flagged them as unsupported.
   Matters for whether the `<base>/quay/...` layout is deployable
   at cluster runtime without per-registry config files.
5. **Plan-emit command novelty.** Only oc-mirror emits an
   `ImageSetConfiguration`. Hauler emits a `manifest.yaml`; k0s
   emits plaintext image lists. okdctl's `airgap plan` — an ISC
   *wrapper* that includes third-party addon charts — is novel.
   Is novelty a smell (nobody else needed it) or a gap
   (everyone else punts to the operator)?

## 13. Explicitly not recommending

Things considered and rejected after analysis:

- **Full `internal/airgap` package extraction now.** Right
  north-star; wrong time. Fix the rule-table duplication (M36)
  first; let the seam emerge once M37 ships.
- **Replacing the Resolver interface.** Two methods justified by
  genuinely different transforms. A single polymorphic
  `Resolve(Artifact)` would force type-switches into every
  implementation for no payoff.
- **Dropping per-fetch env vars.** Four vars is under the
  ergonomic cliff; CI/scripted flows rely on env-var shape. L15
  §6.3a already calls out the use case.
- **Bastion-native helm (`helm pull` → `helm install` tarball).**
  Loses "pull latest version" convenience; forces `ChartRef.Version`
  pinning; adds cache-management overhead for marginal gain over
  the current `resolveChartRef` + IDMS bifurcation.
- **Cluster-native chart pull only (no bastion rewrite).** Deploy
  chicken-and-egg: the cluster doesn't exist during bastion setup
  where the rewrites need to apply.
- **oras-go/v2 for release extraction now.** `oc adm release
  extract --tools` encodes layer-selection knowledge okdctl would
  have to re-implement. Revisit when M30's trigger condition lands
  (non-helm OCI artifact or BYO-OCI addon flow).
- **Self-signing an okdctl-packaged cosign identity for OKD
  artifacts.** Security theater; okdctl signing OKD's bits doesn't
  attest those bits, only our handling of them. Wait on M32 /
  okd#2092 upstream.
- **Blob-into-ISC coercion.** SCOS ISO is ~1 GB; wrapping it in an
  OCI artifact is absurd; tool tarballs are tiny but wrapping them
  creates phantom OCI artifacts IDMS can't sensibly target.
  oc-mirror's own architecture pushes non-image artifacts out for
  this reason.

## 14. Sequencing and roadmap impact

The L15 sequence (M21–M28) lands unchanged in shape. M34's output
reshapes M28's doc scope and files three follow-ups + one deferred
backlog item:

```
L15 sequence (unchanged):
  M21 ✓ → M22 ✓ → M23 ✓ → M24 ✓ → M25 ✓ → M26 ✓ → M27 ✓ → M28 (docs)
  M29 ✓ (independent)
  M33 ✓ (unification)

M34 output (this doc):
  ├─ M35 (activation + fail-loud + fail-closed)
  ├─ M36 (rule-table de-duplication)
  ├─ M37 (Go subcommands replace shell scripts)
  ├─ M38 (deferred: bug-level audit sweep post-M28)
  └─ M39 (deferred: k3s-style tarball drop-in)
```

M28 absorbs two documentation deltas from this audit:

1. Document `EnvOverrideResolver` precedence (env > config >
   default) and its break-glass rationale.
2. Document the `--airgap` / `OKDCTL_AIRGAP` activation semantics
   after M35's tightening (flag-or-env required; `MirrorBase` is
   target not trigger).

Estimated added effort on M28: hours, not days.

## 15. References

- L15 scoping doc: `docs/superpowers/plans/2026-04-19-airgap-scoping.md`.
- L15 item history: roadmap.md `## Completed` entries for M21, M22,
  M23, M24, M25, M26, M27, M29, M33 (all merged 2026-04-20).
- Peer research sources:
  [oc-mirror v2](https://github.com/openshift/oc-mirror),
  [oc-mirror OKD docs](https://docs.okd.io/latest/disconnected/about-installing-oc-mirror-v2.html),
  [RKE2 air-gap](https://docs.rke2.io/install/airgap),
  [k3s air-gap](https://docs.k3s.io/installation/airgap),
  [k3s import-images](https://docs.k3s.io/add-ons/import-images),
  [k0s airgap](https://docs.k0sproject.io/head/airgap-install/),
  [Talos air-gapped](https://docs.siderolabs.com/talos/v1.7/platform-specific-installations/air-gapped),
  [Hauler intro](https://docs.hauler.dev/docs/intro),
  [zot registry](https://github.com/project-zot/zot),
  [containerd hosts.toml](https://github.com/containerd/containerd/blob/main/docs/hosts.md),
  [ImageDigestMirrorSet API](https://docs.redhat.com/en/documentation/openshift_container_platform/4.16/html/config_apis/imagedigestmirrorset-config-openshift-io-v1).
- CLAUDE.md §Architecture notes (`StepDef` / `BuildSteps` pattern
  `FetchPlan` follows) and §Dependencies (permissive-license rule,
  v0.x justification format, no-`@latest` tool installs).

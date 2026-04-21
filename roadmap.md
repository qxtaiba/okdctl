# okdctl roadmap

Captured 2026-04-18 from a full-repo audit (12 parallel Explore subagents) and a
follow-up triage interview. Items are agent-actionable: each includes concrete
file references, acceptance criteria, and dependencies.

## How to use this file

- Items are grouped by schedule band (Sprint 1 / Addon-refactor workstream /
  Deferred / Skipped), then by theme inside each band.
- Every item has a stable ID (`U1`, `N5`, `M12`, `L5`) so PRs, commits, and
  issues can cross-reference it.
- "Acceptance" states what makes the item done — use it as a PR checklist.
- "Depends on" lists prerequisite items by ID.
- Effort estimates follow audit scale: `hours` / `days` / `weeks`.
- Evidence refs use `path:line` format pointing at the repo state on
  2026-04-18 (post–elevation-refactor, post-yaml-migration, post-netip).

### Status lifecycle

Every actionable item carries a `**Status:**` field. Sessions updating an
item **must** edit this field so other sessions know the item is claimed.

| Value | Meaning |
|---|---|
| `not started` | Free to pick up. |
| `in progress — branch: <name>` | A session is actively working on it. |
| `in progress — worktree: <path>` | A parallel agent owns an isolated worktree. |
| `in review — PR #<n>` | Implementation done, awaiting merge. |
| `done — PR #<n>` | Merged. After merge, move the item entry under a `## Completed` section at the bottom of this file and record the merge date. |
| `blocked — waiting on <ID>` | Dependency not yet in `done`; skip for now. |
| `deferred` | Scheduled for a later quarter; do not pick up. |

To find eligible work: grep for `**Status:** not started`, then filter by
`Depends on:` being `none` or only IDs already in `done`.

## Product philosophy (captured in triage)

These are durable constraints the roadmap assumes. They supersede prior
ambitions found in the code.

1. **Single provider: Proxmox.** Multi-provider support (libvirt / vSphere /
   AWS / Equinix / bare-metal / Vagrant) and multi-distribution (RKE2,
   vanilla k8s) are explicitly **skipped**. No provider interface will be
   extracted. The `ProviderType` enum stays single-variant; `ProviderConfig`
   stays a Proxmox-shaped struct.
2. **Linux-only deploy.** macOS/Windows builds are by design removed; wizard
   is cross-platform but deploy targets Linux. `okdctl doctor` stays
   Linux-gated. Documented further, not relaxed.
3. **Curated + alternatives addon catalog with per-category wizard
   dropdowns.** The catalog will grow via a *category model*: each addon
   slots into a category (ingress, load-balancer, GitOps, certs, monitoring,
   storage, backup, policy, service-mesh) and the wizard renders a
   per-category dropdown. Every category includes a "None / BYO" option.
   The specific addons that populate each category are a **later
   conversation** — this roadmap only covers the refactor + scaffolding.
4. **GitOps is not default.** Flux is not auto-installed. Users explicitly
   opt into either Flux or ArgoCD per cluster. Existing Flux addon stays
   available as a category option once the refactor lands.
5. **Tests are not priority.** Zero tests exist today. Unit tests for
   `netutil` and `config/validators` and any integration harness are
   explicitly deferred to quarter+ despite high ROI; the CI test-go gate
   passes vacuously for now.

## Sprint 1 — active work

**Scope warning:** Sprint 1 currently contains ~40 items selected during
triage. This is realistically 3–6 months of work at one engineer. Whoever
picks this up should further sequence using the themes below. Recommended
intra-sprint order: Theme A (urgent correctness) → Theme B (CLI surface) →
Theme D (wizard) → Theme F (errors/types) → remaining themes in parallel.

### Theme A — urgent correctness bugs

### Theme B — CLI subcommand expansion

Motto for this theme: "finish the last mile of the manager/fetcher
scaffolding the internal code already holds."

### Theme E — config ergonomics, rootless

### Theme G — CI, tooling, distribution

## Addon category refactor — dedicated workstream

This is **design-doc-first**. No code lands until the plan is reviewed.

### R1 — Addon category model + design doc

- **Status:** not started
- **Category:** feature-gap / half-done
- **State:** design needed
- **Effort:** weeks
- **Impact:** large
- **Scope:**
  - Design document in `docs/superpowers/plans/YYYY-MM-DD-addon-category-model.md`
    covering:
    - Category enum (ingress, load-balancer, gitops, cert, monitoring,
      storage, backup, policy, service-mesh, secrets).
    - Extension to the `Addon` interface at `internal/addon/addon.go:13-19`
      to require a `Category()` method.
    - Per-category "None / BYO" option as a first-class zero value.
    - Wizard UX: per-category dropdown with "None" / "recommended" /
      "alternative 1" / "alternative 2" / "BYO (pin Helm chart)".
    - Migration path for existing `flux` and `secretstore` addons.
    - BYO contract: what settings a user must provide to install an
      arbitrary Helm chart through the category slot.
    - CLI: `okdctl addon categories` lists categories; `okdctl addon list
      --category ingress` filters.
- **Acceptance:** design doc reviewed and approved before any code.
  Once approved, refactor lands in phases: (a) interface + `Category()`
  method, (b) wizard dropdown rewrite, (c) migrate existing addons, (d)
  unblock R2.
- **Depends on:** none. Gates R2 and all specific-addon work.

### R2 — Specific addons (deferred conversation)

- **Status:** deferred (menu open; no item scheduled)

When the refactor lands, these are the candidates captured during
triage. **None of these are scheduled yet.** They are the menu for a
later decision.

- **Ingress:** ingress-nginx, Envoy Gateway. (OKD built-in router is the
  "None" option.)
- **Load-balancer:** MetalLB, kube-vip.
- **GitOps:** Flux (existing), Argo CD. Neither is a default; users
  opt in.
- **Certs:** cert-manager.
- **Monitoring:** kube-prometheus-stack, Loki, "use OKD built-in" as an
  option.
- **Storage:** Rook-Ceph.
- **Backup:** Velero. VolSync considered (user familiarity) but not
  decided.
- **Policy:** Kyverno, OPA Gatekeeper — both flagged for possible future
  support but not scoped here.
- **Service mesh:** skipped (user BYO).

## Deferred — revisit next quarter

These made the audit but are not scheduled now. Re-evaluate on
2026-07-01 or when a concrete use case surfaces.

| ID | Item | Why deferred |
|---|---|---|
| N12 | Unit tests for `internal/netutil` | User priority: not testing now |
| N13 | Unit tests for `internal/config/validators.go` | Same |
| N21 | `CONTRIBUTING.md` | Pre-1.0; CLAUDE.md covers AI agents |
| N22 | Troubleshooting / FAQ doc | README section is sufficient today |
| M15 | Integration test harness (wizard end-to-end) | Consistent with N12/N13 |
| M18 | Homebrew tap | Post-1.0 distribution item |
| L4 | `okdctl upgrade` (in-place OKD upgrade) | Strategic; needs design |
| L6 | OpenTelemetry tracing hooks | N8 step-timing covers 80% of value |
| N27 | `--log-file` sudo-re-exec hardening (abs-path resolution + chown post-write) | Defense-in-depth on top of `sec:00000002` lstat + `O_NOFOLLOW` fix; attack window closed, broader refactor pending |

### Tier F — documentation findings from 2026-04-21 audit

Items from `.claude/audits/audit-documentation.jsonl` (27 findings;
artifact run 2026-04-21). **F1 is urgent** — revive's `exported` rule
is enabled in `.golangci.yml:66` and the 2026-04-21 comment-hygiene
pass stripped doc comments from ~21 exported symbols. CI will fail on
the next push until F1 is resolved. F2–F4 are polish.

#### F1 — Restore exported-symbol docs stripped by comment-hygiene

**Status:** in progress — branch: fix/f1-restore-exported-docs
**Audit:** 21 findings with prefix `doc:*:exported-doc-missing`
**Evidence:** `golangci-lint run` reports ~50 revive `exported`
violations across 16 files after the 2026-04-21 hygiene pass.
**Problem:** The `/comment-hygiene` skill prunes comments when the
`WHY` isn't clearly articulated. When applied repo-wide it stripped
contract docs from exported symbols the revive `exported` rule
requires a comment on. The stripped symbols include canonical types
(`install.Phase`, `postinstall.Phase`, `setup.Phase`), validator
helpers (`IsValidIP`, `ValidateClusterName`, etc.), logger entry
points (`tui.Info`/`Warn`/`Error`), and security-adjacent helpers
(`system.FileExists`, `AtomicWriteString`).
**Scope:** Restore concise verb-first doc comments on each flagged
symbol. Per-file fix actions with canonical restore phrasing are
captured in each finding's `fix_summary` field in the JSONL artifact.
Group-header comments on `const` blocks cover multiple sites at once.
Net LOC delta: ≈+110.
**Effort:** hours (mechanical). The hygiene-pass skill should also
gain a "skip exported-symbol docs" carve-out before being re-run
against the whole repo.

#### F2 — Trim 3 over-long package docs

**Status:** not started
**Audit:** `doc:a9ea115f:pkg-doc-too-long`,
`doc:a4001485:pkg-doc-too-long`,
`doc:48688e63:pkg-doc-too-long`
**Evidence:** `internal/addon/catalog/catalog.go:1-15`,
`internal/errtypes/errtypes.go:1-8`,
`internal/infrastructure/proxmox/proxmox.go:1-13`.
**Problem:** Three package doc blocks run past the CLAUDE.md 1-3
sentence ceiling — catalog carries a 5-step "adding an addon"
walkthrough, errtypes embeds a credential-redaction invariant that
belongs on the type not the package, proxmox carries a lifecycle
example block.
**Scope:** Trim each to 1-3 sentences. Move the catalog walkthrough
to `docs/addons/` (directory already exists). Move errtypes
redaction invariant to a type-level comment. Move proxmox lifecycle
example to method docs on `Connect`/`Provision`/`Disconnect`.
**Effort:** one sitting.

#### F3 — Regenerate docs/cli/okdctl_destroy.md

**Status:** done — 2026-04-21 (WIP, pre-commit)
**Audit:** `doc:54654337:destroy-cli-ref-stale`
**Evidence:** `docs/cli/okdctl_destroy.md:26-32`.
**Problem:** The generated CLI reference is missing the three
resume-after-partial-destroy flags added in afa579b
(`--skip-terraform`, `--skip-cleanup`, `--skip-firewall`). README
already prescribes `make docs` before tagging; this is the output of
skipping that step when landing afa579b.
**Scope:** `make docs` + commit. No code changes.
**Effort:** minutes.

#### F4 — Fix BuildOpaqueSecret arg-order doc drift

**Status:** done — 2026-04-21 (WIP, pre-commit)
**Audit:** `doc:66cb1c69:addons-buildopaquesecret-sig`
**Evidence:** `docs/architecture/addons.md:141-142` vs
`internal/addon/helpers.go:46`.
**Problem:** Architecture doc shows
`BuildOpaqueSecret(name, namespace, data)` but the canonical helper
signature is `(namespace, name, data)`. A new addon author following
the doc constructs a Secret in the wrong namespace.
**Scope:** One-line doc edit. Do NOT flip the Go signature —
CLAUDE.md §architecture-notes names BuildOpaqueSecret as canonical;
fix the doc, not the API.
**Effort:** minutes.

#### F5 — Fix wizard-registration doc stale path

**Status:** done — 2026-04-21 (WIP, pre-commit)
**Audit:** `doc:70b3bae2:wizard-registration-stale`
**Evidence:** `docs/architecture/wizard.md:38-39`.
**Problem:** Doc tells step authors to register in
`internal/tui/wizard/wizard.go` — file doesn't exist; real site is
`StepBuilder.Register` in `internal/tui/wizard/builder.go:24`.
**Scope:** Swap the filename in the doc. No code changes.
**Effort:** minutes.

### Tier E — architectural deferrals from 2026-04-20 audit

Items from `.claude/audits/FULL_REPORT-2026-04-20.md` whose fix requires
material design work (new config fields, workflow changes, new
subsystems) rather than a one-file surgical edit. Filed here so
`/roadmap-pickup` can schedule them; each carries the originating
audit finding ID so diff tracking stays tight.

#### E1 — Concurrent-run lock with stale-PID detection

**Status:** not started
**Audit:** `state:4c092fce:no-concurrent-run-guard`
**Evidence:** `internal/infrastructure/terraform/terraform.go:119`
**Problem:** Two concurrent `okdctl deploy` or `okdctl destroy` runs in
the same project root both target
`infrastructure/terraform/environments/<env>/terraform.tfstate` with no
mutual exclusion. Terraform's own state lock fires only per-operation,
so racing `okdctl apply` → `tf plan` → `tf apply` against a sibling
`okdctl destroy` yields interleaved applies and a corrupted state.
**Scope:** Add a process-level advisory lock under
`<projectRoot>/.okdctl.lock` taken in `cli/deploy.go` and
`cli/destroy.go` before the phase orchestrator runs. Must detect stale
locks (prior run died via SIGKILL) without racing a live sibling; the
usual PID-file + `kill -0` trick has a reuse window. Consider
`flock(LOCK_EX|LOCK_NB)` with the PID written into the file for
human diagnostics. Must unlock on normal exit and on ctx cancel.
**Effort:** days. Why it's filed here not done inline: stale-detection
design has to be thought through (PID reuse, cross-host NFS homes, the
sudo-re-exec crossing).

#### E2 — Ring-buffered / streamed executor output

**Status:** not started
**Audit:** `sub:7b2829bb:unbounded-output-buffer`,
`sub:4c092fce:terraform-buffered-through-executor`
**Evidence:** `internal/executor/executor.go:116`,
`internal/infrastructure/terraform/terraform.go:148`
**Problem:** `executor.Executor.Run` buffers full stdout+stderr into
`bytes.Buffer` with no cap. `terraform apply` on a cluster with many
VMs, or `openshift-install` on a long bootstrap, materializes tens of
MB in RAM before returning. Plus the user sees nothing until it
completes.
**Scope:** Ring-buffered trail (keep only the last N lines) for error
messages plus a streaming variant (`RunStreamed`) that pipes live to
stdout/stderr. Already have `PlanStreamed` and `ApplyStreamed` on the
terraform wrapper for progress visibility; generalize the pattern at
the executor layer. Keep `RunChecked` semantics for short-output
callers.
**Effort:** days.

#### E3 — HTTPS ignition + pinned CA kargs

**Status:** not started
**Audit:** `sec:00000001:http-ignition-pullsecret`
**Evidence:** `internal/distribution/okd/setup/phase.go:48`,
`internal/distribution/okd/setup/apache.go:27`
**Problem:** `BuildIgnitionURL` returns `http://…/ignition`, and the
rendered `.ign` files embed the full OKD pullSecret. Every node boot
broadcasts the pullSecret cleartext over the machine-network VLAN.
**Scope:** Serve ignition over HTTPS with a self-signed cert, pin the
cert via `coreos.inst.ca` kargs, and flip
`coreos.inst.insecure=no`. Apache needs a vhost on :443 with the cert;
the wizard needs a field (or implicit decision) for the cert CN; the
Terraform cloud-init kargs need the CA material. Cross-cutting: apache
cert lifecycle, kargs templating, wizard.
**Effort:** days.

#### E4 — SSH/SCP host-key pinning for Proxmox

**Status:** not started
**Audit:** `sec:27088eab:ssh-accept-new-proxmox`,
`sec:eb479d86:scp-accept-new-proxmox`
**Evidence:** `internal/distribution/okd/phase/ssh.go:27`,
`internal/distribution/okd/setup/upload.go:42`
**Problem:** `SSHRun` and `uploadISOsViaSCP` use
`StrictHostKeyChecking=accept-new` — TOFU. A MITM on the first handshake
pins the attacker as root@proxmox forever.
**Scope:** Add `provider.proxmox.ssh_host_fingerprint` config field
(SHA256 hex). When set, write a single-entry known_hosts to a temp
path, pass `-o UserKnownHostsFile=<file> -o StrictHostKeyChecking=yes`,
and refuse on mismatch. When unset, keep accept-new with an explicit
one-time warning. The wizard should learn and persist the fingerprint
after the first successful handshake.
**Effort:** hours.

#### E5 — Flux SSH known-hosts fingerprint pinning

**Status:** not started
**Audit:** `sec:98723e5d:ssh-keyscan-tofu`
**Evidence:** `internal/addon/catalog/flux/flux.go:329`
**Problem:** `createDeployKeySecret` runs `ssh-keyscan <host>` and
stuffs the raw output verbatim into the Flux deploy-key Secret. A DNS
poisoner at install time pins themselves as the git host forever,
enabling silent GitOps code substitution.
**Scope:** Add `addons.flux.settings.known_hosts_fingerprint` config
field. Compute SHA256 of ssh-keyscan output, compare against the
configured value, refuse on mismatch. Without the config, emit a WARN
with the observed fingerprint and require `--accept-hostkey` to
proceed.
**Effort:** hours.

#### E6 — kube-vip probe TLS: use cluster CA once available

**Status:** not started
**Audit:** `sec:cfcdee2d:tls-insecure-vip-probe`
**Evidence:** `internal/httputil/httputil.go:22`,
`internal/distribution/okd/postinstall/haproxy.go:65`,
`internal/distribution/okd/postinstall/verify.go:193`
**Problem:** `httputil.NewInsecure` is used at two post-install sites
that target the kube-vip healthz. At those points the cluster CA is
already available under `clusterDir/auth/kubeconfig` but the code
doesn't consume it.
**Scope:** Add `httputil.NewWithCA(pool, timeout)`. Flip the post-install
call sites to parse the CA bundle out of kubeconfig and use NewWithCA.
Keep `NewInsecure` only for the strict pre-install-config window (VIP
not yet in cert SANs) and audit each remaining call site for
reachability of the post-install CA.
**Effort:** hours.

### Tier D — dependency items from 2026-04-18 audit

Filed as roadmap items so `/roadmap-pickup` can fan them out when
bandwidth opens. Each references the audit finding ID for diff tracking.

### Tier G — findings from 2026-04-21 /audit-all run

Filed by the orchestrator aggregation so `/roadmap-pickup` can fan them out when bandwidth opens. Each references the audit finding ID for diff tracking; when a finding recurs in a later run, its entry Status+Evidence updates here rather than being duplicated.


#### audit-api-design

##### `api:fde34e0c:opt-kubeconfig-env-binding` — opt kubeconfig env binding

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/cluster/k8s.go:52-81`  
**Problem:** cluster.NewK8sClient reads KUBECONFIG from os.Getenv at construction time, then builds the cmd runner. This couples the constructor to process env state at call time and means an Exec.Env mutation elsewhere (install.Phase.SetupKubeconfig appends KUBECONFIG= to Exec.Env) is NOT seen by a later-constructed K8sClient.  
**Fix:** Move the os.Getenv('KUBECONFIG') read inside c.run — evaluate the env lazily on each exec.Run so a mid-process KUBECONFIG mutation IS seen. Alternatively drop the env fallback and require WithKubeconfig explicitly.  
**Effort:** hours

##### `api:262af6e4:zero-value-usable-cleanup` — zero value usable cleanup

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:50-107`  
**Problem:** cleanup.Execute takes *Options whose zero Kind yields a bare '*errtypes.ConfigError{Msg: "unknown cleanup type: ..."}' with no sentinel callers can match. An Options{} zero-value (no Logger, no Kind) also silently defaults to NopLogger.  
**Fix:** Add an exported `var ErrKindNotSet = errors.New(...)` sentinel and have Execute return it wrapped when opts.Kind == ''. Alternatively, switch to `NewOptions(kind Kind) *Options` so the required field is a constructor parameter, mirroring destroy.NewOptions / install.NewOptions.  
**Effort:** hours

##### `api:125729c4:opt-inconsistent-cfg-opts` — opt inconsistent cfg opts

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/destroy/phase.go:40-50`  
**Problem:** Phase NewOptions factory shapes still diverge across siblings. setup.DefaultOptions(projectRoot) takes ONLY projectRoot; install.NewOptions, postinstall.NewOptions, destroy.NewOptions take (cfg, projectRoot).  
**Fix:** Rename setup.DefaultOptions to setup.NewOptions(cfg, projectRoot) and fold any cfg-driven defaults into it. Matches the (cfg, projectRoot) signature the other three phase packages share.  
**Effort:** hours

##### `api:c287d5c0:withenv-order-coupling` — withenv order coupling

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/okd.go:61-98`  
**Problem:** okd.WithEnv still encodes an order-dependency contract in New: WithEnv may construct the executor before WithLogger runs, and New compensates after the loop by re-applying WithLogger to the now-existing executor. Option functions should be commutative; any future lazy-building option (e.g.  
**Fix:** Defer executor construction until after the option loop: WithEnv stores pendingEnv []string on *Provisioner, then New builds p.executor = executor.New(executor.WithLogger(p.logger), executor.WithEnv(p.pendingEnv)) once. Options stay pure setters; the constructor owns ordering.  
**Effort:** hours

##### `api:4c092fce:opt-inconsistent-terraform-ctors` — opt inconsistent terraform ctors

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/infrastructure/terraform/terraform.go:109-136`  
**Problem:** terraform package still exports two constructors — New(workDir, opts...) and NewWithVarFile(workDir, varFile, opts...) — that differ only in one default. The second is a thin wrapper solely to preset VarFile.  
**Fix:** Add `func WithVarFile(path string) Option` that sets e.VarFile. Delete NewWithVarFile.  
**Effort:** hours

##### `api:830d4653:export-no-caller-installed-lists` — export no caller installed lists (scaffolding — verify intent only)

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/cleanup/packages.go:34-53`  
**Problem:** cleanup.InstalledPackages and cleanup.InstalledBinaries are exported but their only callers are the package-private Packages() function at line 66 and 72. No external caller in-tree.  
**Fix:** Verify intent — if a preview/plan CLI verb is planned, keep and document. Otherwise unexport (installedPackages, installedBinaries).  
**Effort:** hours

##### `api:ed55ee90:export-no-caller-generate-summary` — export no caller generate summary

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/cleanup/summary.go:11-57`  
**Problem:** cleanup.GenerateSummary and cleanup.Summary struct are exported but the only caller is the package-private printSummary(). No external caller.  
**Fix:** Either unexport (GenerateSummary -> generateSummary, Summary -> summary) if no CLI surface planned, or keep both exported and add a one-line doc-comment pointing to the intended caller (e.g. 'used by okdctl cleanup status; see roadmap.md').  
**Effort:** hours

##### `api:d7ce9d16:export-no-caller-dns-config-helpers` — export no caller dns config helpers

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/dns/dns.go:23-128`  
**Problem:** dns.BuildConfigData, dns.ConfigName, and dns.WriteDnsmasqConfig remain exported with callers only inside the dns package. dns.GenerateBootstrapConfig has a single external caller in setup/steps.go:368.  
**Fix:** Unexport: buildConfigData, configName, writeDnsmasqConfig. Keep GenerateBootstrapConfig, DeployBootstrap, DeployProduction, Setup, RestoreSystemResolver as the package's external API.  
**Effort:** hours

##### `api:de572c63:ctx-not-first-write-dnsmasq` — ctx not first write dnsmasq

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/dns/dnsmasq.go:54-92`  
**Problem:** WriteDnsmasqConfig now takes ctx and checks ctx.Err() at entry (progress from prior run), but still does not thread ctx into os.MkdirAll / system.WriteTempFile / system.CopyFile — the body advertises cancellation only via the entry-gate, not per-step. Either plumb ctx into the underlying helpers or select on ctx.Done between the steps.  
**Fix:** Add `select { case <-ctx.Done(): return ctx.Err(); default: }` between the mkdir / WriteTempFile / CopyFile steps so a mid-op cancellation is honored. Alternatively accept the entry-check as sufficient and add a one-line comment explaining why later operations are not gated.  
**Effort:** hours

##### `api:ae5b624c:concrete-return-k8s` — concrete return k8s

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/install/monitor.go:54-63`  
**Problem:** K8sClient is used in monitor.go only for ApprovePendingCSRs. Rather than accepting a concrete *cluster.K8sClient in MonitorInstallation, the caller could define a tiny consumer-side interface `type csrApprover interface { ApprovePendingCSRs(context.Context) (int, error) }` at the install package.  
**Fix:** Inside install package define `type csrApprover interface { ApprovePendingCSRs(ctx context.Context) (int, error) }`. Accept it as a parameter to MonitorInstallation, defaulting to NewK8sClient(...).  
**Effort:** hours

##### `api:73ad30ef:export-no-caller-external-tool-binaries` — export no caller external tool binaries

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/phase/paths.go:96-105`  
**Problem:** phase.ExternalToolBinaries has one in-tree caller (cleanup/packages.go:52). Exported for the sole purpose of avoiding a setup→cleanup import.  
**Fix:** Move ExternalToolBinaries to a new phase/tools.go with a one-line doc clarifying 'these binaries are installed by setup; cleanup removes them.' No callsite change.  
**Effort:** hours

##### `api:dd75bdeb:stutter-postinstall-context` — stutter postinstall context

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/postinstall/context.go:1-10`  
**Problem:** postinstall.PostInstallContext stutters (package.PostInstall…). The struct is already suppressed with //nolint:revive and a 'rename deferred to a dedicated refactor' note, so this finding is a reminder that the deferred rename is still pending.  
**Fix:** Rename postinstall.PostInstallContext -> postinstall.State (preferred) or postinstall.Context. Callers: phase.go:76 (distribution.NewPhaseContext(State{})), steps.go (4x pctx.Update(func(c *State) {...})), and the PhaseContext[State] type parameter.  
**Effort:** hours

##### `api:761e5126:export-no-caller-removehaproxy` — export no caller removehaproxy (scaffolding — verify intent only)

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/postinstall/haproxy.go:23-97`  
**Problem:** postinstall.Phase.RemoveHAProxy is exported but the only caller is the package-private finalizeIngress path in update_ingress.go:214. No external consumer references it.  
**Fix:** Verify intent against roadmap.md. If a standalone `okdctl haproxy remove` verb is planned, keep exported and add a one-line doc referencing it.  
**Effort:** hours

##### `api:beabab0c:mix-default-new-naming` — mix default new naming

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/setup/phase.go:34-42`  
**Problem:** setup.DefaultOptions continues the Default* naming pattern common for 'zero-arg constructor of a defaulted options struct'. Other phase packages use NewOptions(cfg, projectRoot).  
**Fix:** Rename setup.DefaultOptions -> setup.NewOptions; accept (cfg, projectRoot) and fold any cfg-driven defaults inside. Single call site in okd.go updates.  
**Effort:** hours

##### `api:4f69fc9d:iface-fragmented-step` — iface fragmented step

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/step.go:31-69`  
**Problem:** Step / Skipper / FatalChecker / StepCallbacks remain four interfaces that ProvisioningStep always composes together. The builtStep impl implements all four.  
**Fix:** Keep Step (the 'id+name+execute' core could plausibly stand alone). Inline Skipper, FatalChecker, StepCallbacks into ProvisioningStep directly.  
**Effort:** hours

##### `api:48688e63:iface-in-consumer` — iface in consumer

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:33-101`  
**Problem:** Provider struct still has public methods (Connect, Disconnect, Provision, PlanOnly) but no consumer-side interface. cli/helpers.go and install/phase.go take the concrete *okd.Provisioner.  
**Fix:** If/when a second provider lands, define InfrastructureProvider interface in the distribution package (the consumer). Proxmox implements it structurally — no code change in proxmox/.  
**Effort:** hours


#### audit-cli-ux

##### `ux:024a2c32:json-schema-doc-drift` — json schema doc drift

**Status:** done — 2026-04-21 (WIP, pre-commit) — docs/cli/json-schema.md rewritten to match actual marshaled shapes; golden-test deferral tracked via audit-tests gap entry  
**Severity:** major  
**Evidence:** `docs/cli/json-schema.md:12-67`  
**Problem:** docs/cli/json-schema.md documents field shapes that do not match what the code emits. `okdctl status --format=json` is documented with cluster_name/version/ready_nodes/total_nodes but emits api_reachable/nodes/degraded_operators/addons.  
**Fix:** Option (a, preferred): update docs/cli/json-schema.md to match code — for status enumerate api_reachable, nodes[{name,role,ready}], degraded_operators, addons[{name,healthy,error?}]; for releases list enumerate the flat OKDVersion array including release_type (int, document the 0-4 encoding or switch code to emit the string label). Add fixture-based golden tests (status_test.go, releases_test.go) that compare marshaled output against the doc so future drift fails CI.  
**Effort:** hours

##### `ux:54654337:readme-flag-drift` — readme flag drift

**Status:** done — 2026-04-21 (WIP, pre-commit) — `make docs` regenerated docs/cli/okdctl_destroy.md with the three skip-* flags  
**Severity:** minor  
**Evidence:** `docs/cli/okdctl_destroy.md:26-33`  
**Problem:** Generated CLI reference for `okdctl destroy` is stale: commit afsd79b added --skip-terraform, --skip-cleanup, --skip-firewall to destroy.go, but docs/cli/okdctl_destroy.md still lists only --confirm-cluster, --dry-run, -h/--help, --keep-isos, -y/--yes. CI's docs-drift check (.github/workflows/ci.yml: `git diff --quiet docs/cli/`) would fail on this state.  
**Fix:** Run `make docs` (or `go run ./cmd/okdctl-gen-docs`) and commit docs/cli/. The regenerator is already in CI (.github/workflows/ci.yml:65); the drift is pre-commit residue from the afsd79b work.  
**Effort:** hours

##### `ux:073d24ed:dry-run-yes-short-circuit` — dry run yes short circuit

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/cli/deploy.go:78-80`  
**Problem:** runDeploy checks deployYes before deployDryRun and returns after saving the config, so `okdctl deploy --yes --dry-run` silently skips the dry-run preview the user asked for. --yes is documented as 'skip prompts, use defaults' and --dry-run as 'preview terraform plan and step listing without deploying' — the combination should still preview, not no-op into a config save.  
**Fix:** Reorder the guard: if deployDryRun { return runDeployDryRun(ctx, cfg) } BEFORE the deployYes short-circuit, or gate the --yes fast-path on !deployDryRun. Matches runDestroy (destroy.go:71-73) which checks destroyDryRun first.  
**Effort:** hours

##### `ux:d31d1b9d:json-key-hyphenated` — json key hyphenated

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/cli/status.go:338-353`  
**Problem:** runDescribeAddon emits JSON with a hyphen-cased key `display-name` while every other field in the same payload and every other JSON endpoint uses snake_case (api_reachable, ready_nodes, degraded_operators, release_date, release_type). jq consumers have to quote the field: `jq '."display-name"'`, which is a pain-point.  
**Fix:** Rename the JSON key to display_name in the lines slice when describeFormat == outputJSON. Text mode can keep the hyphenated label (it is human-facing and reads as a single phrase).  
**Effort:** hours

##### `ux:e45c2239:sig-not-handled-preflight` — sig not handled preflight

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `cmd/okdctl/main.go:20-23`  
**Problem:** main() calls preflight() before cli.Execute(); signal.Notify setup lives inside internal/cli/root.go:execute(). If the user hits Ctrl-C during preflight's euid check, OKDCTL_BIN_DIR validation, or PATH mutation, the process dies with SIGINT default (no partial summary, undocumented behavior).  
**Fix:** Either (a) accept current behavior (document in main's package comment: 'preflight runs before signal setup; it is small enough that interruption is racy but not harmful') or (b) move signal.Notify earlier into main() and pass ctx through preflight. Only pay for (b) if preflight grows (e.g.  
**Effort:** hours

##### `ux:93957c53:cleanup-no-dry-run` — cleanup no dry run

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/cli/cleanup.go:18-34`  
**Problem:** cleanupCmd has no --dry-run flag while its destructive siblings (deploy, destroy, update-ingress) all do. cleanup removes packages, dnsmasq/haproxy configs, ignition files, and terraform state — destructive enough that a preview flag has the same value proposition.  
**Fix:** Add cleanupDryRun bool and --dry-run flag. Branch to a runCleanupDryRun before the confirmation prompt; enumerate what would be removed (work directory path, haproxy config block, dnsmasq drop-in path, packages to uninstall) via the existing cleanup.Options struct — do not call cleanup.Execute.  
**Effort:** hours

##### `ux:8d8faa80:completion-use-bracket-optional` — completion use bracket optional

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/cli/completion.go:11-11`  
**Problem:** completionCmd.Use is 'completion [bash|zsh|fish|powershell]' — square brackets per man(1) convention mean optional, but cobra.ExactArgs(1) rejects zero-arg. The shell token is required; Use should render `<bash|zsh|fish|powershell>`.  
**Fix:** Change Use to 'completion <bash|zsh|fish|powershell>'. Same pattern as internal/cli/addon.go:67 ('uninstall <name>') and releases.go:53 ('show <version>').  
**Effort:** hours

##### `ux:e7db1220:releases-show-no-completion` — releases show no completion

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/cli/releases.go:52-59`  
**Problem:** addon install/uninstall and describe-addon gained ValidArgsFunction for tab-completion; releasesShowCmd still has none. Tab-completing `okdctl releases show <TAB>` does filesystem completion instead of version suggestions.  
**Fix:** Add ValidArgsFunction that reads the disk cache (releases.NewOKDVersionFetcher has a cache-backed path) and returns Versions + ShellCompDirectiveNoFileComp. Fall through to ShellCompDirectiveError when the cache is empty rather than fetching on tab — keeps completion latency under the 1s shell threshold.  
**Effort:** hours

##### `ux:aa84670c:exit-code-bsd-sysexits-partial` — exit code bsd sysexits partial

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/cli/root.go:144-162`  
**Problem:** exitCodeFor maps ConfigError=2 (not EX_DATAERR=65 or EX_CONFIG=78), NetworkError=3 (not EX_UNAVAILABLE=69), ClusterError=4 (not EX_UNAVAILABLE=69), AuthError=5 (not EX_NOPERM=77). The taxonomy IS published at the package doc (root.go:1-8).  
**Fix:** Keep the current mapping for backward compatibility (scripts may pin 2/3/4/5); add a regression test in root_test.go asserting each typed error reaches the right code, and --version / help exit 0. Optionally introduce --exit-code-mode={compat|sysexits} for opt-in BSD mapping.  
**Effort:** hours


#### audit-code-smells

##### `smell:daf5bee9:yaml-tree-walk-repeat-assertion` — yaml tree walk repeat assertion

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/cli/kubeconfig.go:141-168`  
**Problem:** mergeNamedList has four nested type-assertion chains to walk a generic YAML tree (any → []any → map[string]any → map[string]any['name'] → string). Function works and the any is load-bearing (YAML unmarshal targets `any` for open schemas), but the walk is tightly coupled to one semantic (merge-by-name) so the any-ness doesn't buy reuse.  
**Fix:** Either (a) declare a minimal kubeconfig schema (ClustersList, UsersList, ContextsList) and yaml-unmarshal into typed slices, then merge; or (b) extract a `namedEntries(v any) map[string]any` helper so the tree walk lives in one place. (a) is the clean fix but adds types the package doesn't need elsewhere; (b) preserves the any-based approach but shrinks the walk to one site.  
**Effort:** hours

##### `smell:004ad79b:helper-pkg-thin-wrap` — helper pkg thin wrap

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/packages/packages.go:1-42`  
**Problem:** Package `packages` wraps `platform.PackageManager.Install`/`Remove` with an extra logger.Info() envelope and a single `fmt.Errorf` rewrap. Three call sites consume it (setup/steps.go:305, cleanup/packages.go:67, cleanup/services.go:41).  
**Fix:** Inline the two functions at their call sites (setup/steps.go:305, cleanup/packages.go:67, cleanup/services.go:41) and delete internal/distribution/okd/packages. The logger.Info lines are already repeated in the callers' surrounding context.  
**Effort:** hours

##### `smell:1d5afa08:enum-via-sscanf-int-parse` — enum via sscanf int parse

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/releases/types.go:59-94`  
**Problem:** OKDVersion.Major() and OKDVersion.Minor() parse the Version string via fmt.Sscanf on every call. ShortVersion calls both (two parses per call), and callers invoke the methods inside filter loops (fetcher.go parseReleases and sortAndClassifySeries, cli/releases.go), so a list of ~40 releases runs dozens of Sscanf parses per display even though the version string is immutable per OKDVersion.  
**Fix:** Parse once at unmarshal time (or memoize). Either (a) add unexported `major, minor int` fields and populate them in the fetcher's parseVersionTag flow (fetcher.go:241 already runs Sscanf — fold the result into the struct), or (b) use `strings.Cut(v.Version, ".")` + strconv.Atoi, which is faster and avoids the fmt machinery.  
**Effort:** hours

##### `smell:c5e5c304:build-role-helper-near-duplicate` — build role helper near duplicate

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/setup/terraform.go:20-34`  
**Problem:** buildISOStrings and buildNodeNames in setup/terraform.go are structurally identical: allocate []string of length count, loop `for i := range count`, format `"%s:iso/%s%d.iso"` vs `"%s-%s%d"`. Both take (isoStorage/clusterName, phase.NodeRole, count).  
**Fix:** Introduce `buildQuotedRoleList(format string, prefix string, role phase.NodeRole, count int) []string` that takes a format string with two %s + one %d and renders count elements. Both sites collapse to one-liners.  
**Effort:** hours

##### `smell:c5e5c304:named-return-unnecessary` — named return unnecessary

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/setup/terraform.go:36-48`  
**Problem:** getDiskSizes returns `(cpDisk, workerDisk, workerDataDisk, masterDataDisk int)` — four unnamed integers with no semantic ordering. The named returns document the positional identity, but the real signal a caller needs is 'which is which'.  
**Fix:** Replace the 4-int tuple with a `type diskSizes struct { cpOS, workerOS, workerData, masterData int }` and return the struct. The single caller (buildTerraformVarsData) then assigns by field name, not by position.  
**Effort:** hours

##### `smell:4f69fc9d:stepbuilder-build-no-callers` — stepbuilder build no callers (scaffolding — verify intent only)

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/step.go:155-173`  
**Problem:** distribution.StepBuilder.Build() has no external callers; every production path goes through BuildSteps → MustBuild, and MustBuild is Build's only caller. Build's stated value is 'returns an error only when b is nil' but NewStepBuilder is the only way to get a *StepBuilder and it never returns nil, so the error is unreachable.  
**Fix:** Keep. Build/MustBuild is the canonical pair for fluent builders in Go (errors.New + errors.Must, template.New + template.Must, etc.), and CLAUDE.md §architecture-notes names StepDef + BuildSteps as canonical.  
**Effort:** hours

##### `smell:0934cf1b:query-match-mini-dsl` — query match mini dsl

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/platform/packages.go:100-115`  
**Problem:** Manager.IsInstalled uses a bespoke `queryMatch` string substring to distinguish "installed" from "purged" on dpkg output. The logic is correct but the two-branch design (empty → exit-code-only, non-empty → substring match) is a mini-DSL inside a single method.  
**Fix:** Replace the `queryMatch` string field with `postCheck func(stdout []byte, pkg string) bool` on Manager. Set it to a no-op in the RHEL constructor and a dpkg-ii-prefix check in the Debian constructor.  
**Effort:** hours


#### audit-concurrency

##### `con:39c75e91:go-no-wait` — go no wait

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/cli/confirm.go:22-45`  
**Problem:** promptForConfirmation spawns a reader goroutine that blocks on bufio.Reader.ReadString, races against ctx.Done, and on ctx cancel the goroutine remains blocked on Stdin.Read until the user presses enter or the process exits. Thoroughly documented in the function header; the capacity-1 inputCh means the goroutine's eventual send never deadlocks — but this is still an unowned goroutine whose lifetime is bounded only by the parent process.  
**Fix:** Go 1.25 has no portable cross-platform stdin cancellation; the current design is the least-bad option for a CLI prompt. CLAUDE.md §Concurrency already names "documented leak bound" as an accepted exception — this site satisfies it.  
**Effort:** hours

##### `con:484b40f0:lock-held-during-write` — lock held during write

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/deploymetrics/metrics.go:75-84`  
**Problem:** Handler holds r.mu.Lock() across fmt.Fprint(w, b.String()) — writing to an http.ResponseWriter under the mutex. A slow Prometheus scraper or stalled network connection blocks every StepStarted/StepFinished call in the deploy path until the write completes, coupling scrape latency to deploy latency.  
**Fix:** Build the rendered metrics string under the lock, release the lock, then write to w: r.mu.Lock(); var b strings.Builder; r.writeMetrics(&b); out := b.String(); r.mu.Unlock(); fmt.Fprint(w, out). The renderer writes to a local Builder so it can't race; the net write happens outside the critical section.  
**Effort:** hours

##### `con:ae5b624c:synctest-opportunity` — synctest opportunity

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/install/monitor.go:52-150`  
**Problem:** MonitorInstallation has a ticker-driven CSR-approval loop, a reap timer, and ctx.Done/DeadlineExceeded paths — exactly the shape testing/synctest is designed for. Currently untested because real-time tests would take minutes; the exec_test.go suite landing in this release proved the pattern works, so this is the last holdout.  
**Fix:** Extract the select-loop body into a testable helper (csrApprovalLoop(ctx, ticker, installDone, k8sClient)) and cover the three exit paths (installDone success, installDone timeout-error, ctx cancel → kill → reap) with testing/synctest — mirror the internal/system/exec_test.go shape that landed this release. Requires a k8sClient fake; audit-tests already flags that fake as missing for CSR-related coverage.  
**Effort:** hours

##### `con:ae5b624c:go-leak-on-error` — go leak on error

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/install/monitor.go:65-150`  
**Problem:** MonitorInstallation spawns a goroutine holding installCmd.Wait(). On ctx cancel the function calls killInstall, waits up to 30s via reapTimer for Wait() to return, then abandons — leaving the goroutine still blocked on the (now-killed) process's Wait() until the OS reaps it.  
**Fix:** Pattern is sound and CLAUDE.md §Concurrency now names it as the canonical cmd.Wait reap-with-deadline example. Optional improvement: promote the reap-with-deadline shape to a shared helper in internal/distribution/okd/phase/ when a second caller (e.g.  
**Effort:** hours

##### `con:8e65d574:go-no-wait` — go no wait

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/version/updatecheck.go:40-53`  
**Problem:** BackgroundCheck spawns a fire-and-forget goroutine that runs runCheck(ctx); printUpdateNotice in cli/root.go waits at most 100ms before returning, so on the happy path the goroutine races to completion and on the slow path it leaks until the process exits. CLAUDE.md §Concurrency now names this as the canonical fire-and-forget example, so the pattern is fully grounded — kept as a long-term advisory that future cross-references should pin line numbers rather than re-raise.  
**Fix:** No code change required — CLAUDE.md §Concurrency now pins this site as the canonical example. Optional future improvement: expose a Done() <-chan struct{} so an integration test can synchronously wait on BackgroundCheck to finish (currently testable only via cache-populate workarounds).  
**Effort:** hours


#### audit-dependencies

##### `dep:33ef32bf:yaml-quad-engines` — yaml quad engines

**Status:** not started  
**Severity:** minor  
**Evidence:** `go.mod:20-60`  
**Problem:** Four YAML engines in the tree: sigs.k8s.io/yaml (direct), go.yaml.in/yaml/v2 (via k8s), go.yaml.in/yaml/v3 (via cobra/doc + kube-openapi), gopkg.in/yaml.v3 (via go-proxmox + testify + charm/log). Binary ships N engines even though only sigs.k8s.io/yaml is directly imported.  
**Fix:** Document the split in CLAUDE.md §Dependencies: sigs.k8s.io/yaml is REQUIRED for k8s Secret marshaling (JSON-tag respect); the three transitive engines are pulled by upstream deps we can't control (k8s, cobra, testify). No action on the code; action is on documentation so future PRs don't accidentally try to 'consolidate' and break the k8s addon path.  
**Effort:** hours

##### `dep:33ef32bf:ultraviolet-pseudo-version` — ultraviolet pseudo version

**Status:** not started  
**Severity:** minor  
**Evidence:** `go.mod:27-27`  
**Problem:** github.com/charmbracelet/ultraviolet is pinned to a pseudo-version (commit SHA, not a tagged release) — the project has never cut a tag. Pulled at three different pseudo-versions by charm.land/bubbles, lipgloss+log, and bubbletea; MVS picks the newest.  
**Fix:** Acceptance note. Charm ecosystem convention is that ultraviolet (the internal renderer) is not publicly-tagged.  
**Effort:** hours

##### `dep:b803fcb7:workflow-pin-hygiene-clean` — workflow pin hygiene clean

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `.github/workflows/ci.yml:1-119`  
**Problem:** Pin hygiene audit: every GitHub Action in .github/workflows/ is pinned by full 40-char SHA with the version tag in a trailing comment (actions/checkout, setup-go, golangci-lint-action, codeql-action, goreleaser-action, cosign-installer, sbom-action, setup-terraform, label-sync, labeler, slsa-github-generator, shellcheck, setup-tflint, attest-build-provenance). Go-install tools pinned by exact version (govulncheck v1.1.4, yamlfmt v0.14.0, terraform 1.10.3, golangci-lint v2.11.4).  
**Fix:** Optional tripwire: add a CI guard that fails if any workflow introduces a non-SHA action ref. Example — a lightweight rule in a new lint job running a regex over .github/workflows/ that flags `uses: org/name@tag` (where tag is not 40 hex).  
**Effort:** hours

##### `dep:87db21a9:goreleaser-action-version-tag` — goreleaser action version tag

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `.github/workflows/release.yml:25-29`  
**Problem:** goreleaser-action is SHA-pinned (good), but the version parameter it resolves IS a tag, not a SHA — version: v2.15.2 in both release.yml and release-prep.yml. This is the goreleaser CLI binary version, not the GH Action.  
**Fix:** Minor tightening: if goreleaser publishes binary SHA256s (it does, as part of its own release process), add a post-install `sha256sum goreleaser` step and match against a pinned hash. Alternatively, accept the v2.15.2 tag trust model — goreleaser signs its own releases with cosign.  
**Effort:** hours

##### `dep:33ef32bf:copyleft-audit-clean` — copyleft audit clean

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `go.mod:1-72`  
**Problem:** License compatibility audit: NO copyleft (GPL/AGPL/LGPL) or custom/unclear licenses in the transitive dep tree. All direct and indirect deps carry permissive licenses (MIT / Apache-2.0 / BSD-3).  
**Fix:** CLAUDE.md §Dependencies already codifies the MIT/Apache/BSD-only policy as of 2026-04-19. This row stays as a tripwire reference so future PR reviewers see the baseline; no code change needed.  
**Effort:** hours

##### `dep:33ef32bf:go-yaml-in-fork-risk` — go yaml in fork risk

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `go.mod:58-59`  
**Problem:** go.yaml.in/yaml/v2 and go.yaml.in/yaml/v3 are a vanity-domain fork of the original gopkg.in/yaml.v{2,3}. The domain (go.yaml.in) is a 2024+ rehosting that the k8s/cobra ecosystems migrated to after gopkg.in archived yaml.v2.  
**Fix:** Acceptance note only. The go.yaml.in move is the same maintainer collective as gopkg.in (kubernetes-sigs).  
**Effort:** hours

##### `dep:33ef32bf:golang-x-exp-stale` — golang x exp stale

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `go.mod:60-60`  
**Problem:** golang.org/x/exp pinned at v0.0.0-20231006140011 (Oct 2023) — almost 2.5 years old. Pulled transitively by charm.land/log/v2, which only imports golang.org/x/exp/slog (a BACKPORT of log/slog that the stdlib now provides since Go 1.21 — and this repo targets 1.25 per go.mod).  
**Fix:** File upstream issue at github.com/charmbracelet/log requesting a drop of the x/exp/slog import in favor of stdlib log/slog. Until that lands, the stale pin persists.  
**Effort:** hours


#### audit-documentation

##### `doc:a55b4592:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/config/loader.go:15-15`  
**Problem:** NewLoader (line 15) lost its doc. Zero-arg constructor returning a pointer — the trivial signature masks the fact that the Loader has lifecycle state (caching, YAML defaults).  
**Fix:** Restore: '// NewLoader returns a Loader suitable for reading okdctl YAML configs. Loaders cache parsed schemas; reuse one per process to avoid re-parsing defaults.'  
**Effort:** hours

##### `doc:cf43073b:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/config/types.go:7-23`  
**Problem:** 4 exported symbols missing docs: DistributionOKD const (7), ProviderProxmox const (14), SupportedDistributions func (17), SupportedProviders func (23). The const values encode the supported-distributions/providers whitelist — semantic meaning is implicit.  
**Fix:** Restore group docs on each const block and one-line docs on SupportedDistributions / SupportedProviders. Example: '// Distributions okdctl can deploy.' before the DistributionOKD block covers revive's exported rule for the whole group.  
**Effort:** hours

##### `doc:297adb3e:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/config/validation_types.go:41-132`  
**Problem:** 5 exported validation types/methods missing docs: ValidationResult.IsValid (41), ValidationResult.AddError (45), ScopeRequired const (66), ValidationScope.HasScope (81), ValidateWithOptions (132). ScopeRequired is a bitflag const — semantic meaning is NOT evident from the name.  
**Fix:** Restore one-line contract docs. For bitflag consts, use a group header: '// Validation scope flags.' above the const block covers the whole group per revive's exported rule.  
**Effort:** hours

##### `doc:aa0f50f5:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/config/validators.go:369-481`  
**Problem:** 6 exported validators at lines 369 (IsValidIP), 374 (IsValidCIDR), 421 (ValidateClusterName), 431 (ValidateDomain), 457 (ValidateIP), 481 (ValidateCIDR) lack doc comments. Is- prefixed returns bool, Validate- prefixed returns error — the naming encodes the behavior but CLAUDE.md §code-comments item 2 still requires a contract doc on exported helpers to clarify failure modes.  
**Fix:** Add one-line verb-first docs. Example: '// IsValidIP reports whether s parses as an IPv4 or IPv6 literal.' '// ValidateClusterName returns a descriptive error if value violates the DNS-1123 cluster-name grammar.'  
**Effort:** hours

##### `doc:125729c4:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/destroy/phase.go:56-56`  
**Problem:** The New constructor (line 56) on destroy.Phase lost its doc in the hygiene pass. revive:exported will fail CI.  
**Fix:** Restore: '// New constructs a destroy.Phase bound to cfg. The Phase is safe to call multiple times — each step idempotently skips if its resource is absent.'  
**Effort:** hours

##### `doc:d5915b0c:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/install/phase.go:80-84`  
**Problem:** 2 exported symbols missing docs: Phase type (80), New func (84). Phase is part of the canonical per-phase pattern per CLAUDE.md §architecture-notes.  
**Fix:** Restore one-line docs mirroring the surviving cleanup/destroy phase patterns. Example: '// Phase drives the install flow: openshift-install wrapper, bootstrap monitor, cluster-up poll.'  
**Effort:** hours

##### `doc:0139cb3f:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/phase/paths.go:70-132`  
**Problem:** 2 exported symbols missing docs: BinDirOrDefault func (70), BasePhaseOption type (132). BasePhase helpers are canonical per CLAUDE.md §architecture-notes — these are the shared cross-phase APIs.  
**Fix:** Restore docs. Example: '// BinDirOrDefault returns s when non-empty, else the default bin dir (from ResolveBinDir).' '// BasePhaseOption configures a BasePhase at construction time.'  
**Effort:** hours

##### `doc:f99eddfa:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/postinstall/phase.go:26-60`  
**Problem:** 5 exported postinstall symbols missing docs: Options type (26), NewOptions func (35), Result type (48), Phase type (56), New func (60). Phase is the canonical per-phase type pattern — CLAUDE.md §architecture-notes explicitly names this.  
**Fix:** Restore one-line docs. Mirror the surviving setup/phase.go or install/phase.go pattern.  
**Effort:** hours

##### `doc:fb54208a:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/postinstall/steps.go:15-15`  
**Problem:** StepVerifyHealth const (line 15) lost its group-header doc. The const is a StepID — part of the canonical distribution.StepID enum used across the phase-step orchestration.  
**Fix:** Restore a group-level doc on the const block: '// Postinstall StepIDs. These identify the steps in Phase.Run order and appear in distribution.Orchestrator events.'  
**Effort:** hours

##### `doc:632c9087:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:18-39`  
**Problem:** 3 exported symbols missing docs: DefaultIngressLBTimeout const (18), IngressEntry type (31), UpdateIngressResult type (39). DefaultIngressLBTimeout encodes a 10-minute operational value — semantic meaning (why 10 not 5) is not evident from the name.  
**Fix:** Restore docs. For DefaultIngressLBTimeout include the rationale inline: '// DefaultIngressLBTimeout caps how long update-ingress waits for the ingress LB service to report a ready external IP.  
**Effort:** hours

##### `doc:ab9b764a:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/ignition.go:85-85`  
**Problem:** Phase.GenerateManifests (line 85) lost its doc. Manifest generation is an externally-visible step — callers need to know the failure mode.  
**Fix:** Restore: '// GenerateManifests invokes openshift-install to expand install-config.yaml into the full manifest set. Returns wrapped ConfigError for validation failures and wrapped ExecError for binary failures.'  
**Effort:** hours

##### `doc:2f70d7df:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/kargs.go:41-41`  
**Problem:** ExtractNetworkConfig (line 41) lost its doc. The name suggests extraction but the semantics (from which input?  
**Fix:** Restore: '// ExtractNetworkConfig parses the Ignition JSON and returns the first storage-files entry matching the NetworkManager connection path. Returns a typed ConfigError for malformed JSON.'  
**Effort:** hours

##### `doc:beabab0c:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/phase.go:19-111`  
**Problem:** 8 exported symbols in setup.Phase carry no doc comment after the 2026-04-21 comment-hygiene pass: DefaultIgnitionPort const (19), Options type (23), DefaultOptions func (34), CoreOSInfo type (57), NodeInfo type (64), Phase type (71), Phase.Execute method (94), Phase.PrintSetupCompletionSummary method (111). revive:exported enabled in .golangci.yml will fail CI on next push.  
**Fix:** Restore concise verb-first doc comments on each of the 8 sites. For the Phase type, lead with 'Phase drives the setup phase of an OKD install — artifact download, config generation, ignition upload.' Mirror existing docs in sibling install/destroy packages for consistency.  
**Effort:** hours

##### `doc:6fc3d91e:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/platform/platform.go:18-18`  
**Problem:** FamilyRHEL const (line 18) lost its group-header doc. Part of the const block defining platform family identifiers.  
**Fix:** Restore a group-level comment on the const block: '// Platform OS-family identifiers and supported arch literals.' covers revive's exported-block requirement.  
**Effort:** hours

##### `doc:e3782ee7:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/system/fs.go:14-235`  
**Problem:** 4 exported filesystem helpers missing docs: FileExists (14), DirExists (22), EnsureDirForFile (35), AtomicWriteString (235). AtomicWriteString is a wrapper around the canonical AtomicWrite — the wrapper's TOCTOU/fsync semantics should inherit from the underlying contract.  
**Fix:** Restore docs. Example: '// FileExists reports whether path refers to an existing regular file (returns false for directories).' '// AtomicWriteString is a string-typed convenience wrapper around AtomicWrite; the fsync + rename invariants are the same.'  
**Effort:** hours

##### `doc:e2343d2c:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/system/systemd.go:17-17`  
**Problem:** ServiceEnable const (line 17) lost its group-header doc. Part of the ServiceAction enum driving systemctl operations.  
**Fix:** Restore a group-level comment: '// Actions passable to SystemdCtl. Each value maps to a systemctl subcommand.'  
**Effort:** hours

##### `doc:c14fdd9d:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/tui/base_styles.go:9-9`  
**Problem:** TitleStyle var (line 9) lost its doc. Part of the base-styles palette; caller code in wizard steps imports it.  
**Fix:** Restore: '// TitleStyle is the bold Blue400 header style used at the top of each TUI step.'  
**Effort:** hours

##### `doc:588ce79e:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/tui/colors.go:13-18`  
**Problem:** 2 exported color symbols missing docs: ThemeDefault const (13), ColorPurple600 var (18). These are the theme system's public palette — downstream TUI components import them.  
**Fix:** Add group header on ThemeDefault's const block: '// Built-in color themes.' Add: '// ColorPurple600 is the purple-600 palette color used by the default theme.'  
**Effort:** hours

##### `doc:983f67f0:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/tui/layouts.go:11-11`  
**Problem:** DefaultBoxWidth const (line 11) lost its doc. A layout-constant encoding a semantic choice (78 columns?  
**Fix:** Restore: '// DefaultBoxWidth is the content width used by BoxedSection; 78 leaves room for the 1-col lipgloss border on each side in an 80-col terminal.'  
**Effort:** hours

##### `doc:660d83a5:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/tui/logger.go:22-161`  
**Problem:** 5 exported logger helpers missing docs: LF (line 22), Info (68), Warn (70), Error (72), RunID (161). Info/Warn/Error in particular encode contract invariants (stderr redirect + RedactHandler wiring per the kept comments elsewhere in this file).  
**Fix:** Restore one-line docs that reference the stderr + RedactHandler invariant already kept on stderrSlog. Example: '// Info logs at INFO through the redact-handling stderr slog.  
**Effort:** hours

##### `doc:bc9ba9bc:exported-doc-missing` — exported doc missing

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/tui/rendering.go:3-11`  
**Problem:** 3 exported rendering helpers missing docs: SubsectionLabel (3), CompletionSuccess (7), CompletionError (11). User-facing TUI output helpers — caller code needs to know formatting guarantees.  
**Fix:** Restore one-line docs. Example: '// CompletionSuccess formats msg with the green-check success prefix and the configured base style.'  
**Effort:** hours


#### audit-errors

##### `err:48688e63:typed-err-fallthrough` — typed err fallthrough

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:181-237`  
**Problem:** Provider.Provision and Provider.retrieveProvisionResult still raise bare fmt.Errorf for config-class / cluster-runtime failures ('no VMs provisioned; check config', 'static IP start address is required for OKD deployments'). The prior sweep fixed Connect (line 85, 88 now use ConfigError); these two adjacent sites were missed.  
**Fix:** Line 182: wrap with &errtypes.ClusterError{Msg: "terraform apply succeeded but no VMs were provisioned; check config"} (cluster-runtime failure, exit 4). Line 237: wrap with &errtypes.ConfigError{Msg: "static IP start address is required for OKD deployments"} (config, exit 2).  
**Effort:** hours

##### `err:40d315ad:wrap-tool-prereq-untyped` — wrap tool prereq untyped

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/addon/catalog/flux/flux.go:72-72`  
**Problem:** Flux.Install returns a bare fmt.Errorf('helm is required to install Flux') when helm is missing. The message is user-friendly but the error carries no chain or type — it's a tool-prerequisite failure that semantically matches ConfigError (missing external dep is a configuration/environment issue, exit 2).  
**Fix:** For tool-prerequisite errors (line 72) and config errors (:148, :332, :391), wrap with &errtypes.ConfigError{Msg: ...} so the underlying chain carries correct classification. addon/manager.go:installAndVerify should then errors.As for ConfigError before wrapping as ClusterError, preserving the 'tool missing' vs 'install failed' distinction at the outer boundary.  
**Effort:** hours

##### `err:ddf885f4:errors-join-ctx-lost` — errors join ctx lost

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/addon/manager.go:83-111`  
**Problem:** InstallAll aggregates failures via errors.Join(errs...) after wrapping each with ClusterError at installAndVerify:120. Good pattern.  
**Fix:** After the for-loop, add `if err := ctx.Err(); err != nil && len(errs) > 0 { errs = append(errs, err) }` so the joined error includes the ctx sentinel when cancellation contributed. Alternatively, make installAndVerify itself wrap ctx.Err via %w when Install/Verify return a ctx-related error.  
**Effort:** hours

##### `err:aa84670c:ctx-err-check-on-ctx` — ctx err check on ctx

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/cli/root.go:110-116`  
**Problem:** execute() still checks `if ctx.Err() != nil` to decide whether to return 130 (SIGINT) or 143 (SIGTERM). This works today because the hand-rolled signal handler always cancels the ctx before ExecuteContext returns, but it's fragile: a future subcommand that returns context.Canceled WITHOUT the parent ctx being canceled hits exitCodeFor instead of the 130/143 branch.  
**Fix:** Change to `if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) { /* ... SIGTERM/SIGINT branch ...  
**Effort:** hours

##### `err:7b2829bb:typed-no-error-iface` — typed no error iface

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/executor/executor.go:184-199`  
**Problem:** executor.ExitError doc still claims 'errors.Is to compare against Unwrap chain values' but the type has no Unwrap() method and no Err field. The claim is aspirational — there is nothing in the chain to traverse.  
**Fix:** Option A (recommended, -1 LOC): remove 'errors.Is to compare against Unwrap chain values' from the doc comment — the type doesn't currently chain. Option B (+5 LOC): add `Err error` field, populate from executor.run's `err` var when cmd.Run fails, implement Unwrap().  
**Effort:** hours

##### `err:f51f85bb:err-stringified-loses-type` — err stringified loses type

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/netutil/ip.go:43-46`  
**Problem:** Four sites still use `if err != nil || !X.Is4()` and return a synthetic fmt.Errorf that drops the netip.ParseAddr / netip.ParsePrefix error entirely. Debugging 'invalid IPv4 address: 192.168' gives no hint whether netip rejected the format, the IP-version check rejected it, or a whitespace issue.  
**Fix:** Split the conditional so parse-err and is4-check produce different messages, or wrap on the err-present case: `if err != nil { return fmt.Errorf("invalid IPv4 address %q: %w", startIP, err) } if !addr.Is4() { return fmt.Errorf("IPv6 not supported: %q", startIP) }`. Matches the 'invalid CIDR %q: %w' pattern at internal/netutil/ip.go:18.  
**Effort:** hours


#### audit-iac-and-shell

##### `iac:b803fcb7:ci-no-tflint-tfsec` — ci no tflint tfsec

**Status:** not started  
**Severity:** minor  
**Evidence:** `.github/workflows/ci.yml:97-109`  
**Problem:** `validate-terraform` + `lint-terraform` jobs now run `terraform fmt`, `terraform validate`, and `tflint -f compact` — but no secret/policy scanner (tfsec, checkov, or trivy config). tflint catches terraform_* idiom issues; tfsec/checkov catch misconfigured provider secrets, missing `sensitive = true`, and public-exposure antipatterns that the HCL surface will grow into as the module adds network/firewall rules.  
**Fix:** Add a `tfsec` or `trivy config` step to the validate-terraform/lint-terraform job. tfsec has a maintained action `aquasecurity/tfsec-action@...`; `trivy config infrastructure/terraform` is a single call.  
**Effort:** hours

##### `iac:b803fcb7:tflint-no-config` — tflint no config

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `.github/workflows/ci.yml:102-109`  
**Problem:** CI runs `tflint --init && tflint -f compact` with no `.tflint.hcl` config file in either module or environment directory. Without a config, tflint loads only the default language ruleset — the `terraform-linters/tflint-ruleset-terraform` plugin (recommended preset: module_pinned_source, required_providers, required_version, naming conventions, unused_declarations) is therefore silent.  
**Fix:** Add `infrastructure/terraform/.tflint.hcl` with `plugin "terraform" { enabled = true, preset = "recommended" }`. Point CI to it via `--config=$GITHUB_WORKSPACE/infrastructure/terraform/.tflint.hcl` (shared across module + env) or per-directory symlinks.  
**Effort:** hours

##### `iac:18a795d5:hcl-no-prevent-destroy-masters` — hcl no prevent destroy masters

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/main.tf:140-255`  
**Problem:** Master VMs (OKD control plane carrying etcd quorum state) have no `lifecycle { prevent_destroy = true }` guard. A misconfigured `terraform apply` that perturbs a force-new attribute (e.g.  
**Fix:** Add `prevent_destroy = true` to the master VM resource's `lifecycle` block, gated by a variable (e.g. `var.allow_master_destroy`, default false) that `okdctl destroy` flips before running Terraform.  
**Effort:** hours

##### `iac:e076e43c:sh-posix-not-bash` — sh posix not bash

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `scripts/install.sh:1-1`  
**Problem:** Shebang `#!/bin/sh` constrains the script to POSIX sh (dash on Debian/Ubuntu, ash on Alpine), which prevents unconditional `set -o pipefail`, `[[ ]]`, and other bash conveniences. Script now mitigates with a conditional `(set -o pipefail 2>/dev/null) && set -o pipefail` probe, but future contributors may still introduce bashisms that break silently under dash/ash.  
**Fix:** Either (a) switch shebang to `#!/usr/bin/env bash` and drop the pipefail probe — bash is available on every supported install target (Debian/Ubuntu/Fedora/RHEL), Alpine is not a target platform per `uname -s` gate; or (b) keep `/bin/sh` and document in a one-line comment above the shebang that POSIX-only constructs are required, so future contributors don't accidentally introduce bashisms. The current hybrid works but sits on a portability knife-edge.  
**Effort:** hours


#### audit-modernization

##### `mod:d31d1b9d:use-map-index` — use map index

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/cli/status.go:97-107`  
**Problem:** `statusNode.role()` iterates every key of `Labels` to check for two specific well-known strings. This is a map lookup dressed as a scan — O(n) in label count when a direct `if _, ok := Labels["node-role.kubernetes.io/master"]; ok` is O(1) and reads straight.  
**Fix:** Replace with direct map index: `if _, ok := n.Metadata.Labels["node-role.kubernetes.io/master"]; ok { return "master" }; if _, ok := n.Metadata.Labels["node-role.kubernetes.io/worker"]; ok { return "worker" }; return "unknown"`. No imports needed.  
**Effort:** hours

##### `mod:6fc3d91e:use-strings-lines` — use strings lines

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/cli/status.go:171-171`  
**Problem:** `for _, line := range strings.Split(strings.TrimSpace(coRaw), "\n")` materializes the split slice only to walk it. Go 1.24's `strings.Lines` iterator skips the allocation.  
**Fix:** Replace `for _, line := range strings.Split(s, "\n")` with `for line := range strings.Lines(s)`. Go 1.24 stdlib, no import change.  
**Effort:** hours

##### `mod:9d79b841:use-slices-max` — use slices max

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/setup/coreos.go:70-74`  
**Problem:** One of two near-identical blocks in `findOrDownloadFCOSISO` still does `slices.Sort(matches); matches[len(matches)-1]` to fetch the lexicographically-largest filename. The sibling block at lines 57-61 was already rewritten to `slices.Max(matches)`; this one (lines 70-74) was left behind.  
**Fix:** Replace `slices.Sort(matches); isoPath := matches[len(matches)-1]` with `isoPath := slices.Max(matches)`. Go 1.21+.  
**Effort:** hours

##### `mod:0934cf1b:use-slices-concat` — use slices concat

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/platform/packages.go:101-101`  
**Problem:** `append(append([]string{}, m.queryArgs...), pkg)` nests two `append`s to clone-then-extend a slice. Go 1.22's `slices.Concat` expresses the same intent in one call and also matches the repo's existing `slices.Concat(a, b)` style (see internal/cli/helpers.go:213, internal/distribution/okd/dns/dnsmasq.go:155).  
**Fix:** Replace with `args := slices.Concat(m.queryArgs, []string{pkg})`. Requires `slices` import (not currently imported in this file — the sibling InstallPackages uses a plain `append([]string{...}, installed...)` which is already idiomatic).  
**Effort:** hours

##### `mod:983f67f0:use-builtin-max-innerwidth` — use builtin max innerwidth

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/tui/layouts.go:54-60`  
**Problem:** Two sequential `if X > innerWidth { innerWidth = X }` blocks compute a running max over two candidates. Go 1.21's `max` builtin collapses both into `innerWidth = max(innerWidth-ContentPadding+maxContentWidth+ContentPadding, minWidthForTitle)` — or more readably, `innerWidth = max(innerWidth, maxContentWidth+ContentPadding, minWidthForTitle)`.  
**Fix:** Replace with `innerWidth := max(width-ContentPadding, maxContentWidth+ContentPadding, minWidthForTitle)`. `max` takes variadic ordered args in Go 1.21+.  
**Effort:** hours

##### `mod:983f67f0:use-builtin-max-padding` — use builtin max padding

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/tui/layouts.go:100-104`  
**Problem:** `padding := innerWidth - lineWidth; if padding < 0 { padding = 0 }` is a hand-rolled `max(padding, 0)` — the exact floor `max` was added (Go 1.21) to express. This pattern has been flagged and fixed in at least three sibling files already; layouts.go is the last holdout in the tui package.  
**Fix:** Replace with `padding := max(innerWidth-lineWidth, 0)`. Drops 3 lines to 0.  
**Effort:** hours


#### audit-observability

##### `obs:19a715fd:level-warn-help-text` — level warn help text

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/addon/catalog/secretstore/secretstore.go:122-152`  
**Problem:** secretstore.installPrereqCheck still logs multi-line HOW-TO guides (onepassword: 6 Warn lines, vault: 3 Warn lines, bitwarden: 3 Warn lines) via env.Logger.Warn when credential files are missing. Warn is for recoverable degradation; this is user education.  
**Fix:** Emit the first line ('no secret files found ... skipping') at Warn as the actual advisory.  
**Effort:** hours

##### `obs:0d318f5c:handler-no-tty-switch` — handler no tty switch

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/cli/logging.go:35-67`  
**Problem:** configureLogging still does not auto-select JSON format when stderr is not a TTY. Operators piping `okdctl deploy 2>&1 | jq .` get charmlog text with ANSI escapes by default and must remember `--log-format json`.  
**Fix:** Route cobra's cmd into configureLogging so `cmd.Flags().Changed("log-format")` is available. If not set and !stderrIsTTY, default logFormat to "json".  
**Effort:** hours

##### `obs:15ba17da:err-stringified-into-label` — err stringified into label

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/destroy/steps.go:32-37`  
**Problem:** destroy.steps.go builds its per-step OnError callback as `phase.WarnOnError(p.Log, label+": "+err.Error())(err)`, which concatenates err.Error() into the Warn message AND passes err again as the structured attr (WarnOnError body: `logger.Warn(msg, "err", err)`). Result: the error text appears twice — once inlined into `msg` (bypassing RedactHandler's attr-walk) and once as the structured `err` attr.  
**Fix:** Drop the `+ ": " + err.Error()` concatenation: `phase.WarnOnError(p.Log, label)(err)`. WarnOnError already emits `logger.Warn(label, "err", err)`, which gives structured consumers the label and a redaction-eligible err attr.  
**Effort:** hours

##### `obs:00000002:inconsistent-domain-prefix-keys` — inconsistent domain prefix keys

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:140-285`  
**Problem:** The codebase still leans on the `prefix: message` convention ('update-ingress:', 'haproxy:', 'kubevip:', 'cluster:', 'cleanup:', 'terraform:', 'coreos:', 'iso:', 'csr:', 'addons:') in message bodies, but no call-site pins a structured `component` or `phase` attr via logger.With(). Only `run_id` is propagated (tui.SetRunID).  
**Fix:** At each phase constructor (setup/install/postinstall/destroy/cleanup) wrap p.Log via logger.With("phase", "install"); at sub-component boundaries (haproxy, dns, kubevip, terraform, iso, packages, cleanup.services, cleanup.packages, addon.manager) narrow with logger.With("component", "haproxy"). Retain the human prefix in the message for TTY readability — the attr is additive.  
**Effort:** hours

##### `obs:9d79b841:duplicate-iso-exists-log` — duplicate iso exists log

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/setup/coreos.go:59-265`  
**Problem:** coreos.go logs `coreos: found existing iso at X` (L59, L73) and `coreos: iso already exists at X` (L201, L265) in four distinct sites. A single setup run can fire more than one because the lookups happen across different layers (local iso dir, work-dir cache, download destination, upload destination), producing near-identical Info lines.  
**Fix:** Consolidate the four sites into one helper (`logISOFound(path)`) that emits the Info line once per iso-path in a run — keep a set of already-logged paths keyed off filepath.Base. If the two message variants (found vs already-exists) carry distinct operator semantics, rename them so the distinction is legible.  
**Effort:** hours

##### `obs:366b3f2d:span-no-start-end-per-step` — span no start end per step

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/distribution/orchestrator.go:113-154`  
**Problem:** Orchestrator.executeStep still does not emit a structured start/finish log pair per step. Skipping is logged (L90) but success/duration is not — Duration is captured in StepResult but never reaches the logger.  
**Fix:** In executeStep, before step.OnStart() log `o.logger.Info("step started", "step", step.ID(), "name", step.Name())`. After Execute (both success and error branches) log `o.logger.Info("step completed", "step", step.ID(), "duration", time.Since(startedAt), "success", err == nil)`.  
**Effort:** hours

##### `obs:7b2829bb:executor-no-output-span` — executor no output span

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/executor/executor.go:213-273`  
**Problem:** executor.run and RunInteractive still only log `+ <name> <args>` at Debug when Verbose is true — nothing bookends the call in the structured stream. For a 15-minute terraform apply or oc poll the JSON sink sees nothing until completion.  
**Fix:** At start of executor.run log `e.logger.Debug("exec started", "cmd", name, "argc", len(args))` (omit argv itself — terraform invocation argv can contain a credential substitution in rare configs). After cmd.Run log `e.logger.Debug("exec completed", "cmd", name, "exit", result.ExitCode, "duration", result.Duration)`.  
**Effort:** hours

##### `obs:48688e63:message-embedded-counts` — message embedded counts

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:217-217`  
**Problem:** The prior audit flagged three terraform count-in-message lines in proxmox.go; L158 and L185 are now structured (`"count", n`) but L217 remains `fmt.Sprintf("terraform: plan will preview %d virtual machines", totalNodes)`. The pattern also spreads to cleanup/summary.go L73 / L80 / L94 and addon/manager.go L78 / L101 / L117 / L125 / L181 — numeric counts wedged into the message string that JSON consumers cannot index by `count`.  
**Fix:** Replace the remaining `fmt.Sprintf(...%d...)` log sites with structured attrs: proxmox.go:L217 → `p.logger.Info("terraform: plan preview", "vm_count", totalNodes)`; cleanup/summary.go:L73/L80/L94 → match L67's shape `logger.Info("cleanup: ignition files", "count", n)`; addon/manager.go:L78/L101/L117/L125/L181 → `"addons: installing", "count", len(ordered)` etc. Keep human prefix in the message for TTY readability.  
**Effort:** hours

##### `obs:aa84670c:root-error-stringified` — root error stringified

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/cli/root.go:187-187`  
**Problem:** The ctx-done-miss branch at L120 was migrated to structured form `tui.Error("command failed", tui.LF("err", err))` — prior audit's core case. The SetFlagErrorFunc handler at L187 still stringifies: `tui.Error(err.Error())`.  
**Fix:** Replace `tui.Error(err.Error())` with `tui.Error("flag error", tui.LF("err", err))`. Keep the exit-code logic unchanged.  
**Effort:** hours

##### `obs:ae5b624c:monitor-retry-log-per-tick` — monitor retry log per tick

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/install/monitor.go:119-127`  
**Problem:** MonitorInstallation's CSR approval tick runs every 30s for up to 60 minutes. On each tick: on error it Warns structured, on approved>0 it Infos structured.  
**Fix:** Optional: de-dup identical consecutive Warns via a `lastWarnErrMsg` tracker, downgrading repeats to Debug after the first. Keep `approved>0` Info as-is (state transition).  
**Effort:** hours


#### audit-security

##### `sec:88fd3050:cred-as-string-in-config` — cred as string in config

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/config/cluster.go:107-134`  
**Problem:** ProxmoxConfig.Password and ProxmoxConfig.APIToken are typed as `string` (with `json:"-"`). The credentials.GetProxmoxCredentials legacy fallback reads them when the env path is empty (proxmox.go:213-228), converting via []byte(px.Password) — the new slice is wipeable but the original string residue persists for the Config's lifetime.  
**Fix:** Option A (safer): remove the config-file credential path entirely — env/.env is the documented mechanism and the comment already says 'never persisted'; honour that by deleting the legacy fallback branch in GetProxmoxCredentials. Option B (if kept): retype ProxmoxConfig.Password and APIToken to []byte, adjust the loader path, and Zeroize during Config teardown.  
**Effort:** hours

##### `sec:f55b9c27:cred-string-copy-envfile` — cred string copy envfile

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/credentials/envfile.go:42-68`  
**Problem:** WriteEnvFile converts password and API-token []byte to an immutable Go string via string concatenation before calling AtomicWrite. The string copy survives Zeroize on the source []byte.  
**Fix:** Use a []byte buffer (bytes.Buffer / manual append) keyed off the raw []byte fields, then pass the buffer to AtomicWrite and scrub the buffer after the call returns. Keeps credential bytes on the wipeable path throughout.  
**Effort:** hours

##### `sec:35abd54e:cred-string-copy-env` — cred string copy env

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/credentials/proxmox.go:113-134`  
**Problem:** ProxmoxCredentials.Env() builds subprocess env entries via string concatenation: "PROXMOX_VE_PASSWORD="+string(c.Password). The resulting Go string is an immutable heap copy of the password that Zeroize cannot overwrite, leaving an unwipeable residue for the entire lifetime of the returned slice (and beyond, via GC).  
**Fix:** Return a [][]byte (or a keyed byte-slice struct) for the credential-bearing entries and have the caller build cmd.Env at the final moment; or at minimum scope the Env() slice to a tight defer-clear. The current pattern violates the design intent of keeping passwords as []byte across the lifecycle.  
**Effort:** hours

##### `sec:06f00bcb:ignition-dir-perms` — ignition dir perms

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/apache.go:28-45`  
**Problem:** ensureIgnitionDir creates /var/www/html/ignition at 0o755 and then explicitly re-chmods to 0o755 if pre-existing. The ignition files inside (bootstrap.ign, master.ign, worker.ign) carry the pullSecret.  
**Fix:** Tighten the ignition directory and file perms: dir → 0o750 owner apache:apache; files → 0o640 via CopyFileMode with 0o640. Apache serves them fine under its own uid; local non-apache users can no longer grep out the pullSecret.  
**Effort:** hours

##### `sec:00000005:bootstrap-oc-no-integrity` — bootstrap oc no integrity

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/release_extract.go:24-76`  
**Problem:** bootstrapOC downloads oc.tar.gz from mirror.openshift.com with no checksum or cosign signature verification. The docstring admits 'no upstream checksum is published for this URL; post-extraction binary-exists verification is the integrity gate'.  
**Fix:** Either (a) pin bootstrapOCURL to a specific release tag and ship a baked-in sha256 in the okdctl binary (matches the 'explicit versions — never @latest' rule in CLAUDE.md §Dependencies), or (b) verify a cosign signature on the tarball if Red Hat publishes one for the client tarball set, or (c) fall through to `oc adm release extract` via the distribution-packaged `openshift-client` rpm/deb instead of curl-to-bash. Document the trust decision in CLAUDE.md §security-invariants.  
**Effort:** hours

##### `sec:19a715fd:secretstore-plaintext-disk` — secretstore plaintext disk

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/addon/catalog/secretstore/secretstore.go:253-278`  
**Problem:** The secretstore addon reads 1password-credentials.json and 1password-token.txt (plus the vault/bitwarden equivalents) from automation/config/secrets/ and applies them as Kubernetes secrets. The code neither checks nor enforces restrictive file permissions on these on-disk credential files: a user who followed the setup instructions with `echo -n 'TOKEN' > file` gets default umask (often 0o644).  
**Fix:** Before os.ReadFile(path), Stat the path and reject any file whose perm bits exceed 0o600 unless it's sops-encrypted. Mirror the pattern used in internal/credentials/envfile.go loadEnvFileOnce.  
**Effort:** hours

##### `sec:00000006:debug-bundle-redact-partial` — debug bundle redact partial

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/cli/config.go:65-79`  
**Problem:** redactConfig in cli/config.go only masks Provider.Proxmox.TokenID and leaves every other config field unchanged. Password and APIToken carry `json:"-"` so they never marshal into the bundle (correct today), but the function signature encourages a future 'add a field, forget to redact' regression.  
**Fix:** Walk the config via reflection and mask every string field whose struct-tag name matches the RedactHandler denylist (password, token, secret, api_key, apikey). Alternative: add an explicit `okdctl:"sensitive"` struct tag and have redactConfig honour it — future fields opt in by tagging.  
**Effort:** hours

##### `sec:26a430ee:syscall-exec-env-leak` — syscall exec env leak

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/cli/elevation.go:54-77`  
**Problem:** ensureRoot re-execs via syscall.Exec(sudoPath, args, os.Environ()). The full inherited environment is handed to sudo → the new okdctl process.  
**Fix:** Filter os.Environ() before passing to syscall.Exec: keep PATH, HOME, USER, LANG, LC_*, SUDO_*, PROXMOX_VE_* (needed downstream), OKDCTL_*, KUBECONFIG, and reject everything else. The downstream Executor now applies a similar allowlist (internal/executor/executor.go:85-121), so this layer is additive defense-in-depth — but the sudo boundary is the highest-value place to enforce.  
**Effort:** hours

##### `sec:d66c3d7f:bashrc-no-nofollow` — bashrc no nofollow

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/install/flux.go:93-143`  
**Problem:** addKubeconfigToBashrc opens ~/.bashrc with os.OpenFile(O_APPEND|O_WRONLY, 0o644) under the sudo re-exec (running as root, HOME resolved via InvokingUserHomeDir). No lstat + O_NOFOLLOW guard.  
**Fix:** Mirror the logging.go fix: lstat the path first and refuse if it is a symlink; then open with syscall.O_NOFOLLOW so any concurrent symlink-plant races fail. Alternatively, use system.AtomicWrite to read-modify-rewrite instead of O_APPEND on a user-owned file — AtomicWrite already has the fsync/rename guarantees this site could benefit from.  
**Effort:** hours

##### `sec:7b2829bb:env-append-os-environ` — env append os environ

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/executor/executor.go:85-174`  
**Problem:** Executor now applies a defaultEnvAllowlist (good — previously-flagged broadcast of unrelated env vars is closed). But PROXMOX_ is in the prefix allowlist, so PROXMOX_VE_PASSWORD / PROXMOX_VE_API_TOKEN still reach EVERY subprocess the executor spawns — including coreutils shellouts that don't need Proxmox credentials (lsb_release, dpkg, gpg, rpm, ss, systemctl, semanage, find, rm, ssh-keyscan).  
**Fix:** Split Executor.Env into two slices: AuthEnv (credential-bearing PROXMOX_*, KUBECONFIG, GIT_*, GITHUB_TOKEN) and Env (general). Add WithAuthEnv(...) and a per-Run toggle so credential vars only reach terraform + oc + helm + sops.  
**Effort:** hours

##### `sec:451be4fa:chowntree-symlink-audit` — chowntree symlink audit

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/system/elevation.go:100-131`  
**Problem:** ChownTreeToInvokingUser uses filepath.WalkDir + os.Lchown (symlink-safe). The docstring explicitly requires the caller to only pass paths whose subtree okdctl itself created in this process.  
**Fix:** Add a runtime guard: ChownTreeToInvokingUser should refuse root if it does not match a short allowlist (projectRoot/okd-install, projectRoot/infrastructure, user-home subdirs). Alternative: introduce a typed workdir handle (type WorkDir string) produced only by the orchestrator, so the function signature statically excludes callers that pass cfg.HTTPServer.Root.  
**Effort:** hours

##### `sec:d5915b0c:kubeconfig-env-leak` — kubeconfig env leak

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/install/phase.go:151-162`  
**Problem:** SetupKubeconfig appends `KUBECONFIG=<path>` to p.Exec.Env, making the kubeconfig path visible to every subprocess the executor spawns from that point forward — including unrelated tools (helm, ssh-keyscan, lsb_release, dpkg, rpm). The kubeconfig file contains bearer credentials for the cluster.  
**Fix:** Couple with sec:7b2829bb: split Executor.Env into AuthEnv + Env, and push KUBECONFIG into AuthEnv. Only oc / helm / openshift-install invocations see AuthEnv; dpkg / rpm / lsb_release / ssh-keyscan do not.  
**Effort:** hours


#### audit-state-and-recovery

##### `state:93957c53:cleanup-no-confirm-cluster` — cleanup no confirm cluster

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/cli/cleanup.go:37-103`  
**Problem:** `okdctl cleanup` has only `--yes` with no typo-guard against the wrong config. Unlike `okdctl destroy` which requires `--confirm-cluster=<name>` when `--yes` is passed, cleanup stops and uninstalls haproxy/dnsmasq/apache, drops the VIP secondary IP, wipes terraform.tfstate.backup + .terraform.lock.hcl + bin-dir binaries (coreos-installer/terraform/oc/kubectl) without asserting the cluster name in scripted invocations.  
**Fix:** Mirror the destroy guards: add `--confirm-cluster` (required with `--yes`, must match cfg.Cluster.Name) and `--dry-run` (prints the list of services that would be stopped and files that would be removed without mutating). Promote the destroy.go guard block into a shared helper (e.g.  
**Effort:** hours

##### `state:fb54208a:postinstall-no-rollback-path` — postinstall no rollback path

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/postinstall/steps.go:42-93`  
**Problem:** postinstall steps (cleanup-bootstrap, deploy-production-dns) are NonFatal and mutate cluster-external state (bootstrap VM destroyed via targeted tf apply, /etc/dnsmasq.d/*.conf replaced). If StepCleanupBootstrap succeeds but StepVerifyKubeVIP fails and StepDeployProductionDNS is skipped, the cluster is left with: bootstrap VM gone, kube-vip not verified, DNS still pointed at bootstrap.  
**Fix:** Two options: (a) add `okdctl postinstall --step=dns` subcommand that re-runs just the DNS sub-phase once kube-vip is confirmed healthy; or (b) expand update-ingress to handle the bootstrap->production DNS transition when it's still pointing at bootstrap IP. Prefer (b) since update-ingress already owns DNS re-deploys.  
**Effort:** hours

##### `state:4f69fc9d:no-resume-checkpoint` — no resume checkpoint

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/distribution/step.go:178-188`  
**Problem:** StepDef has no 'already-done' precondition hook or checkpoint. If okdctl crashes mid-setup, the next run starts from step 1, repeating work or reconciling side-effects ad-hoc.  
**Fix:** Option A (lightweight): add `ReRunSafe bool` on StepDef, default false, and require every StepDef to declare it (lint-time enforcement that a step sets ReRunSafe explicitly). Document the contract in step.go.  
**Effort:** hours

##### `state:48688e63:provision-leaves-tfplan` — provision leaves tfplan

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:149-174`  
**Problem:** `Provider.Provision` writes `<workDir>/tfplan` via Plan then applies it, but never sweeps the plan file on success or failure — only `destroyInfrastructure` calls `tf.Cleanup()`. After a successful deploy the operator is left with a stale `tfplan` that no longer matches state; after a failed apply it's doubly-stale.  
**Fix:** Add `defer func() { _ = p.terraformExec.Cleanup() }()` immediately after the Plan call succeeds in `Provider.Provision`, matching the bootstrap.go pattern. Plan file removal on failure also helps operators inspecting `<workDir>` after a failed apply because nothing stale is left.  
**Effort:** hours

##### `state:48688e63:proxmox-no-retry-layer` — proxmox no retry layer

**Status:** not started  
**Severity:** suggestion  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:131-193`  
**Problem:** Provider.Provision delegates 100% to terraform — no Go-side retry on transient Proxmox API failures, no 409-already-exists handling beyond what the bpg/proxmox provider does internally. retrieveProvisionResult derives VM IPs from config (not from Proxmox), so eventual-consistency 'VM created but not yet listed' gaps are dodged by not querying Proxmox — but any future code that enumerates VMs from the API will need retry.  
**Fix:** Document the invariant in proxmox.go header: 'All Proxmox mutation MUST go through terraform.Executor. Direct HTTP calls are forbidden on deploy/destroy.' If/when status queries are added, use internal/download's retry helper (5xx/408/429 with exponential backoff, 4xx fail-fast).  
**Effort:** hours


#### audit-subprocess

##### `sub:e2343d2c:systemd-stderr-dropped` — systemd stderr dropped

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/system/systemd.go:36-43`  
**Problem:** ManageService runs systemctl enable/disable/start/stop/restart/reload via exec.CommandContext(...).Run() with both stdout and stderr left nil. On failure the caller gets a bare *exec.ExitError with no systemctl diagnostic ('Failed to enable unit: Unit file haproxy.service does not exist' / 'Job for X failed because the control process exited').  
**Fix:** Route the default (state-changing) branch through system.RunCaptured so the returned error carries systemctl's stderr diagnostic. The is-active/is-enabled probe branches can stay on bare .Run() — exit code alone is the signal and --quiet already suppresses stderr noise.  
**Effort:** hours


#### audit-tests

##### `tst:daf5bee9:no-test-kubeconfig-merge-full` — no test kubeconfig merge full

**Status:** not started  
**Severity:** blocker  
**Evidence:** `internal/cli/kubeconfig.go:77-125`  
**Problem:** mergeNamedList now has unit coverage (TestMergeNamedList) but mergeKubeconfig itself — the full merge pipeline including (a) source/dest YAML parse, (b) three-key merge (clusters/users/contexts), (c) current-context preservation invariant (set from src only when dest has none), (d) AtomicWrite at mode 0o600 — remains untested end-to-end. The current-context and 0o600 perm guarantees are the load-bearing invariants for kubectl-default-cluster preservation and on-disk kubeconfig perms.  
**Fix:** Extend internal/cli/kubeconfig_test.go: TestMergeKubeconfig_PreservesCurrentContext — seed dest YAML with current-context=prod + one cluster 'prod', pass srcData with current-context=okd-test + clusters [okd-test,dev] via t.Setenv(KUBECONFIG, tmp) to redirect mergeTargetPath, call mergeKubeconfig(srcData), read-back YAML, assert current-context == 'prod' AND clusters contains both 'prod' and 'okd-test'. TestMergeKubeconfig_EmptyDestTakesSrcCurrentContext — empty dest → dest's current-context becomes src's.  
**Effort:** days

##### `tst:6b533f2d:no-test-approve-pending-csrs` — no test approve pending csrs

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/cluster/k8s_csrs.go:51-74`  
**Problem:** ApprovePendingCSRs drives MonitorInstallation's CSR-approval loop. No test covers (a) PendingCSRs returns [] → (0, nil) fast path, (b) non-empty list → single `oc adm certificate approve` with all names in one argv (the batching is load-bearing — N separate approve calls per tick would rate-limit the API), (c) PendingCSRs error → (0, err) propagates; (d) runCheck failure wraps with "failed to approve CSRs" prefix.  
**Fix:** Use the fake-oc pattern already landed in phase/kubectl_test.go: install a PATH-shadowed 'oc' that records argv to a temp file, then assert (a) 0 CSRs → 0 runs; (b) 3 CSRs → 1 run with argv ["adm","certificate","approve","csr-1","csr-2","csr-3"]; (c) PendingCSRs returns error → propagate; (d) approve exit !=0 → *errtypes.ClusterError wrapping. Shares the test-harness idiom with the existing kubectl_test.go suite.  
**Effort:** hours

##### `tst:830d4653:no-test-packages-cleanup-guard` — no test packages cleanup guard

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/cleanup/packages.go:59-96`  
**Problem:** cleanup.Packages composes ResolveBinDir → filepath.Join → refuseCriticalPath → os.RemoveAll for each installer-managed binary (yq/helm/sops/oc/kubectl/openshift-install). The per-iter refuseCriticalPath guard is the only thing stopping an OKDCTL_BIN_DIR=/etc environment variable from walking os.RemoveAll into /etc/yq.  
**Fix:** Add internal/distribution/okd/cleanup/packages_test.go: (1) TestPackages_RefusesCriticalBinDir — pass binDir="/" (via option or env t.Setenv), stub detectPackageManager to a no-op, assert returned error is *errtypes.ClusterError (guard fires per iter, hasErrors=true); (2) TestPackages_HappyPath — binDir=t.TempDir() populated with fake `yq`, `helm`, `sops` executables, assert each is gone post-call; (3) TestPackages_MissingBinariesNoError — empty binDir → no error. Stub the package-manager dnf path to avoid requiring root.  
**Effort:** days

##### `tst:33579dd5:no-test-cleanup-haproxy` — no test cleanup haproxy

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/cleanup/services.go:50-87`  
**Problem:** cleanup.HAProxy deletes the live haproxy config, globs *.backup.* siblings, removes okdctl firewall rules, releases the bastion VIP, and uninstalls the haproxy package. The wildcard glob `haproxyConfig + ".backup.*"` is not tested for attacker-shaped haproxyConfig (e.g.  
**Fix:** Add cleanup/services_test.go::TestHAProxy_GlobOnlyMatchesBackups — populate t.TempDir with `haproxy.cfg`, `haproxy.cfg.backup.1`, `haproxy.cfg.backup.2`, `haproxy.cfg.orig` (not a .backup.*); pass tmpDir/haproxy.cfg as haproxyConfig with a nop logger and stubbed firewall/netutil/packages; assert only haproxy.cfg + .backup.* are removed, .orig survives. Include a TestHAProxy_RefusesCriticalPath with haproxyConfig="/etc/passwd" asserting the guard blocks removal.  
**Effort:** days

##### `tst:33579dd5:no-test-dnsmasq-cleanup-glob` — no test dnsmasq cleanup glob

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/cleanup/services.go:137-182`  
**Problem:** Dnsmasq runs os.RemoveAll against paths produced by filepath.Glob("/etc/dnsmasq.d/okd-*.conf") and a secondary backup glob. Each match is guarded by refuseCriticalPath, but the glob + guard composition is untested — the guarded-glob pairing is exactly where a regression (dropping the per-iter guard, switching Glob to a non-absolute pattern) would surface as an arbitrary-path delete primitive.  
**Fix:** Because the hard-coded /etc/dnsmasq.d/... globs need root to exercise, refactor the glob/prefix pair into dnsmasqConfigGlobs() returning the two patterns, then test the guard-loop with a fake glob list: feed ["/etc/dnsmasq.d/okd-foo.conf", "/etc", "/"] into the guarded-remove helper and assert only the first reaches the (mocked) removeFn.  
**Effort:** days

##### `tst:15ba17da:no-test-destroy-orchestration` — no test destroy orchestration

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/destroy/steps.go:24-133`  
**Problem:** destroySteps orchestrates Terraform destroy + ISO removal + file cleanup + firewall cleanup, now with the 'failures' tracker and the SkipTerraform/SkipCleanup/SkipFirewall flags landed in afsa79b. No test covers (a) SkipTerraform=true skips only the first step, (b) KeepISOs=true skips ISO removal with the correct SkipReason string, (c) a non-fatal step failure is recorded in failures[] and the summary warn-logs it (the misleading-success regression guard), (d) SkipCleanup=true short-circuits StepCleanupFiles independently of the CleanupKind check.  
**Fix:** Refactor destroySteps to accept a testable dependency surface (terraform-destroyer interface, iso-remover interface, cleanup-runner interface, firewall-remover interface) via struct injection. Then test: (1) SkipTerraform=true, KeepISOs=true → only StepCleanupFiles, StepCleanupFirewall, StepPrintSummary enabled; (2) KeepISOs=false, Proxmox=nil → StepRemoveRemoteISO skipped with correct reason; (3) NonFatal step returns error → orchestrator proceeds AND summary step sees failures[] non-empty → warn emitted.  
**Effort:** days

##### `tst:25fa1be8:no-test-validateport-attacker` — no test validateport attacker

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/firewall/firewall.go:124-140`  
**Problem:** validatePort is the explicit defense-in-depth guard preventing Port.Protocol from flowing unchecked into fmt.Sprintf("%d/%s", ...) and onward into firewall-cmd / ufw / iptables argv. The doc comment explicitly warns: "keeping the guard here prevents a future caller from sneaking an unvalidated protocol string into the rendered rule".  
**Fix:** Add internal/distribution/okd/firewall/firewall_test.go::TestValidatePort — table-driven: valid [{6443,tcp}, {53,udp}] → nil; invalid port number [0, -1, 65536, 99999] → "invalid port number"; invalid protocol ["", "TCP", "sctp", "tcp/ip", "tcp; rm", "icmp"] → "invalid protocol". Twenty lines.  
**Effort:** hours

##### `tst:98723e5d:no-test-setup-cluster-access` — no test setup cluster access

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/install/flux.go:50-91`  
**Problem:** SetupClusterAccess installs the generated kubeconfig into ~/.kube/config under the invoking user's home (after sudo re-exec), backing up any existing file at destPath+".backup.<timestamp>" and chowning each output to the invoking user. The invariants — (a) backup uses 0o600 via CopyFileMode, (b) destKubeconfig is copied at 0o600, (c) ChownToInvokingUser is called on destKubeconfig AND the new .kube dir — are the only defenses for a credential-bearing file that lands in a directory writable by root.  
**Fix:** Add internal/distribution/okd/install/flux_test.go::TestSetupClusterAccess_Perms — override HOME via t.Setenv and InvokingUserHomeDir fallback; create a srcKubeconfig under a fake clusterDir/auth/; call SetupClusterAccess; assert destKubeconfig perm == 0o600 and content round-trips; with a pre-existing destKubeconfig, assert the .backup.<ts> file also exists at 0o600. Skip the actual ChownToInvokingUser assertion (root-required) but note the code path is exercised in the test harness.  
**Effort:** days

##### `tst:ae5b624c:test-missing-synctest-monitor` — test missing synctest monitor

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/install/monitor.go:43-150`  
**Problem:** MonitorInstallation is the canonical ctx-cancel-reap-goroutine pattern for openshift-install monitoring. Has a ticker-driven CSR approval loop, a kill-with-reap path, and three exit branches (installDone success, installDone error with ctx-wrapping, ctx-done kill-and-reap).  
**Fix:** Requires the csrApprover interface extraction from api:fde34e0c. Once MonitorInstallation accepts a csrApprover + an injected command runner, use testing/synctest to cover: (a) installDone(nil) → final ApprovePendingCSRs called, returns nil; (b) installDone(err) under ctx.DeadlineExceeded → error wraps DeadlineExceeded; (c) installDone(err) under ctx.Canceled → error wraps Canceled; (d) ctx cancel → kill + reap within 30s succeeds; (e) ctx cancel + kill ignored → 30s elapses + warn logged.  
**Effort:** days

##### `tst:761e5126:no-test-removehaproxy` — no test removehaproxy

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/postinstall/haproxy.go:23-97`  
**Problem:** RemoveHAProxy calls os.RemoveAll(phase.DefaultHAProxyConfigPath) (= /etc/haproxy/haproxy.cfg) then tears down firewall rules, the bastion VIP, and verifies API reachability. The /etc removal has no guard against an attacker-influenced DefaultHAProxyConfigPath (currently a const, but consumed indirectly), no partial-failure test (what if firewall.RemoveRules fails?), no idempotency test (second call on an already-removed haproxy).  
**Fix:** Add postinstall/haproxy_test.go with a fake Exec / Log and an injectable haproxyConfigPath variable (apply the same test-injection pattern setup/haproxy.go uses). Cases: (a) happy path — service stopped, config file gone, firewall rules removed; (b) empty VIP skips the kube-vip verification branch; (c) os.RemoveAll error is logged but does not abort (resilience); (d) API-via-VIP wait returning non-ok yields *errtypes.NetworkError.  
**Effort:** days

##### `tst:632c9087:no-test-buildlb-ingresscontroller` — no test buildlb ingresscontroller

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:371-467`  
**Problem:** convertToLoadBalancer is a destructive conversion (`oc delete ingresscontroller` then `oc create` a rebuilt one) with an explicit rollback path via attemptRollback. The two load-bearing JSON transforms — buildLBIngressController (which must preserve domain/replicas/defaultCertificate/routeSelector/routeAdmission/nodePlacement from the original spec while swapping strategy to LoadBalancerService) and buildRollbackJSON (which must strip server-managed fields to let `oc create` succeed) — are pure, in-memory functions that feed a destructive external call, and neither has a test.  
**Fix:** Add internal/distribution/okd/postinstall/update_ingress_test.go with stdlib testing only: (1) TestBuildLBIngressController_PreservesSpecFields — craft an ingressControllerInfo with RawJSON containing all six optional spec fields populated; assert the returned JSON unmarshals to a doc whose spec.endpointPublishingStrategy.type == LoadBalancerService AND each of domain/replicas/defaultCertificate/routeSelector/routeAdmission/nodePlacement round-trips intact; (2) TestBuildLBIngressController_EmptyNamespaceDefaults — Metadata.Namespace="" → output namespace == "openshift-ingress-operator"; (3) TestBuildRollbackJSON_StripsServerFields — seed RawJSON with creationTimestamp/generation/resourceVersion/uid/managedFields + a status block; assert each field is absent from the result AND non-server fields (spec, name, namespace) remain.  
**Effort:** days

##### `tst:29293401:no-test-haproxy-rollback` — no test haproxy rollback

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/haproxy.go:87-146`  
**Problem:** ConfigureHAProxy writes to /etc/haproxy/haproxy.cfg — a root-required file on the live system — and has a rollback path that restores from backup on validation/restart failure. No test covers the rollback: (a) no prior config → no backup taken, no rollback; (b) validation fails → backup restored, service restarted with old config; (c) rollback chmod/restart failure surfaces joined errors.  
**Fix:** Requires swapping the hard-coded /etc/haproxy paths + ManageService subprocess calls for injected seams. Practical test: extract the rollback lambda into a package-local helper attemptHAProxyRollback(cause, haproxyCfgPath, backupPath, chmodFn, restartFn) error and table-drive: (a) restore fails → joined error; (b) restore OK, restart fails → joined; (c) happy rollback → cause returned.  
**Effort:** days

##### `tst:ab9b764a:no-test-installconfig-perms` — no test installconfig perms

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/ignition.go:34-83`  
**Problem:** GenerateInstallConfig reads the pull-secret and writes install-config.yaml (containing the raw pull-secret JSON) at mode 0o600 via AtomicWriteString, then duplicates it to install-config.yaml.backup via CopyFileMode at 0o600. Both perm values are critical — the pull-secret is a Red Hat registry credential.  
**Fix:** Add setup/ignition_test.go::TestGenerateInstallConfig_Perms — build a minimal cfg with cfg.Files.PullSecret + cfg.Files.SSHPublicKey pointing at tmp files; call GenerateInstallConfig with outputDir=t.TempDir; stat both install-config.yaml and install-config.yaml.backup; assert os.FileMode.Perm() == 0o600 for each. Also TestGenerateInstallConfig_PullSecretReadFail asserts *errtypes.AuthError on missing pull-secret.  
**Effort:** hours

##### `tst:41a9d4eb:no-test-redact-handler` — no test redact handler

**Status:** not started  
**Severity:** major  
**Evidence:** `internal/logutil/redact.go:30-123`  
**Problem:** RedactHandler is the canonical slog redaction middleware — CLAUDE.md §credentials-and-secrets explicitly calls it out as the mechanism "so credentials in structured attrs never reach the sink". Its direct unit tests are absent; coverage today is indirect via tui/logger_test.go.  
**Fix:** Add internal/logutil/redact_test.go with stdlib testing + bytes.Buffer + slog.NewTextHandler as the wrapped inner. Cases: (1) TestRedactAttr_SecretKeys — feed password/PASSWORD/api_token/bearer_token; assert all replaced with "[redacted]"; (2) TestRedactAttr_NonSecret — cluster/user (non-secret) pass through; (3) TestRedactAny_URL — *url.URL with User=url.UserPassword("u","p") → output has u@ but no :p@; (4) TestRedactAny_RedactedInterface — struct with Redacted() any returning "<masked>" → replaced; (5) TestWithAttrs_RedactsDerivedLogger — logger.With("password", "x").Info(...) → output has [redacted], never "x"; (6) TestWithGroup — group propagation preserves redaction; (7) TestGroupKind — nested slog.Group with a secret key inside is redacted.  
**Effort:** days

##### `tst:98723e5d:no-test-add-kubeconfig-bashrc` — no test add kubeconfig bashrc

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/install/flux.go:93-143`  
**Problem:** addKubeconfigToBashrc appends `export KUBECONFIG=<path>` to the invoking user's ~/.bashrc. It preserves the existing file mode explicitly (doc: "appending an export line can't silently relax stricter perms the user may have set") and is idempotent (skips if `export KUBECONFIG=` already present).  
**Fix:** Extend the flux_test.go with: (1) TestAddKubeconfigToBashrc_Idempotent — pre-populate bashrc with `export KUBECONFIG=/old`, call addKubeconfigToBashrc, assert file content is byte-identical (the idempotency short-circuit); (2) TestAddKubeconfigToBashrc_PreservesMode — create bashrc at 0o600, call, stat, assert perm still 0o600; (3) TestAddKubeconfigToBashrc_CreatesIfMissing — no prior bashrc, call, assert it exists at 0o644 with the export line.  
**Effort:** hours

##### `tst:451be4fa:no-test-writeasinvoking` — no test writeasinvoking

**Status:** not started  
**Severity:** minor  
**Evidence:** `internal/system/elevation.go:82-98`  
**Problem:** WriteAsInvokingUser combines AtomicWrite + chown-back. The "parent dir chowned iff it did not pre-exist" logic (line 84-86 + 94-96) is a subtle invariant — exists to avoid silently chowning a pre-existing dir the user created with different ownership.  
**Fix:** Skip the actual chown (root required); test only the parentExisted flag path by extracting the existence probe into a seam OR by checking behaviour via fs inspection. Minimal value unless the chown-back is mocked — consider this an acknowledgement rather than an emit-to-fix.  
**Effort:** hours


## Explicitly skipped

Do not revisit these without a strong new signal.

| ID | Item | Rationale |
|---|---|---|
| L1 | libvirt / KVM provider | Proxmox-only is a product constraint |
| L2 | vSphere / AWS / Equinix / bare-metal / Vagrant | Same |
| L3 | Multi-distribution (RKE2, vanilla k8s) | Same |
| M7 | Provider interface extraction | No longer needed given L1–L3 skipped |
| L12 | Container image distribution (ghcr.io/...) | Deploy model is host-shell; container fit is poor |

Also skipped by design (documented in audit, not user-driven):

- macOS / Windows build targets (goreleaser ships linux/amd64 + linux/arm64 only).
- `okdctl doctor` on non-Linux (Linux-only build tag is intentional).
- Bootstrap as a separate wizard section (wizard intentionally mirrors
  control-plane specs via `internal/tui/wizard/steps/resources.go:56`).
- Manual addon registration via blank-import (pattern is documented in
  `docs/architecture/addons.md:50-58`; scale does not yet justify codegen).
- Terraform state file preservation during cleanup — deliberate in
  `internal/distribution/okd/cleanup/infra.go:52-61`.
- Hardcoded k8s ports (6443, 22623) — OKD spec-level; configurability
  would break installer assumptions.

## Completed

Items that have reached `done` status, ordered by close date. New
entries land here when a PR merges, or when an item is closed without
code (audit error, done-by-prior-work). Keep the explanation terse
but link evidence.

- **M40 — Air-gap workstream removed** — done 2026-04-20. The entire
  L15 air-gap workstream (M21–M27, M33) was ripped out in one pass.
  Deleted: `internal/fetchplan/`, `internal/addon/mirror/`,
  `internal/cli/airgap.go` + test + `testdata/airgap/`,
  `internal/cli/doctor_airgap.go` + test, the `--airgap` flag on
  deploy/destroy/doctor, `OKDCTL_AIRGAP` / `OKDCTL_MIRROR_BASE` /
  `OKDCTL_SCOS_*` / `OKDCTL_BOOTSTRAP_OC_URL` / `OKDCTL_UPDATE_CHECK_URL`
  env vars, `Deployment.MirrorBase` + `Deployment.ToolVersions` config
  fields, `okd.WithAirgap`, `addon.MirrorableAddon` / `MirrorSpec` /
  `ChartRef`, `addon.Environment.Resolver`, `flux.resolveChartRef` +
  `flux.MirrorArtifacts`, `secretstore.MirrorArtifacts`,
  `addon.WithResolver`, `fetchplan.PickResolver` / `IsAirgap` /
  `ResolveMirrorBase`, both resolver chains, the 9 Plan builders, the
  9 Purpose constants, the 4 per-fetch Env*URL constants, the two
  scoping docs under `docs/superpowers/plans/`, the airgap doctor
  checks section in `docs/doctor-checks.md`. Kept: M22's
  `oc adm release extract --tools` path (it's the current OKD-binary
  fetch mechanism; the release-image URL is now a hardcoded constant,
  no resolver wrap) and M23's direct scos.json/fcos.json fetch (same
  deal — URLs inlined). Tool URLs (helm, sops, yq) moved from fetchplan
  overrides back to hardcoded template constants in
  `setup/tools.go`. `version.BackgroundCheck` signature reverted to
  `(ctx) <-chan CheckResult`; direct `api.github.com` call. M29 (GitHub
  Attestations) stays — it was an adjacent supply-chain item, not an
  L15 item. **Rationale:** after shipping M21–M27 + M33 and writing
  the M34 architectural review (PR #102), the conclusion was that
  okdctl should not own mirror configuration. Operators doing air-gap
  installs are sophisticated users who already own their staging
  pipeline (`oc-mirror`, `skopeo`, Harbor, zot, Hauler, whatever);
  okdctl's `MirrorBase` + rewrite-table + `airgap plan` emission was
  paternalism the user base didn't need. The `Plan`/`Resolver`
  abstraction was elegant but earned its keep only to support the
  rewrite logic being deleted; without air-gap it was empty ceremony.
  Net deletion: ~1,800 LOC of production code + 4 goldens + 2 scoping
  docs + 2 CLI reference entries. Build and test suite green after the
  sweep. **Postmortem lesson:** L15 (scoping doc, 2026-04-19) locked
  an architecture the product didn't want. The failure mode was
  premature commitment: a well-researched scoping doc became
  load-bearing for M21–M27 implementation weeks before any operator
  had deployed the feature. Next time: ship a one-knob MVP (a single
  per-purpose URL override, wizard-configurable) before designing a
  resolver chain. Zero operator feedback is a strong signal to keep
  the abstraction surface tiny.

- **M29 — GitHub Artifact Attestations for release binaries** — done
  PR #94, merged 2026-04-20. New `actions/attest-build-provenance@v4.1.0`
  step in `.github/workflows/release.yml` after goreleaser (SHA-pinned
  `a2bbfa25...`); permissions were pre-provisioned at lines 7-10 so no
  scope change. `subject-path` covers all four shipped artifact globs
  (`dist/okdctl_*.tar.gz`, `*.deb`, `*.rpm`, `SHA256SUMS`); SBOMs
  intentionally excluded per acceptance. Additive to the existing
  cosign + SLSA flows; `install.sh` untouched. README gains a one-line
  `gh attestation verify <file> --repo qxtaiba/okdctl` snippet next to
  the existing cosign block. Tagged release will publish attestations
  at `https://github.com/qxtaiba/okdctl/attestations/<n>`.
- **U1 — Cleanup phase leaves FCOS ISO cache** — closed 2026-04-18 as
  audit error; no code change required. Local `downloads/` cache is
  already removed by `internal/distribution/okd/cleanup/artifacts.go:84,92`
  in both `preserveConfig` branches. The real gap — remote Proxmox
  ISO cleanup — is now tracked as **U1b**.
- **U5 — `cmd.Run()` without `CommandContext` in two sites** — closed
  2026-04-18 as done-by-prior-work. Both sites already use
  `exec.CommandContext(ctx, ...)`: `internal/distribution/okd/setup/tools.go`
  builds every cmd with `CommandContext` across lines 112/193/210/223/227/248,
  and `internal/system/elevation.go:142` constructs its cmd via
  `exec.CommandContext(ctx, "sudo", "-n", "true")` — the `cmd.Run()` at
  line 146 is the method call on that ctx-aware cmd (the audit misread
  it). Fix landed as a side-effect of commit `65d8fce refactor(platform):
  thread ctx through PackageManager and tool version lookup`
  (elevation-refactor-and-hardening plan).
- **U3 — HTTP downloads have no retry** — done PR #49, merged 2026-04-18.
  `internal/download` now retries 5xx, 408, 429, and transport errors with
  exponential backoff (5s base, factor 2, jitter 0.5, 3 steps, 5-minute
  cap). 4xx and context cancellation fail fast. Retry helper kept local
  to `internal/download/retry.go` rather than importing `addon` so the
  package layering stays low-level → nothing. Attempt count and last
  error are logged on exhaustion.
- **U6 — `BuildOpaqueSecret` panics on YAML marshal error** — done PR #50,
  merged 2026-04-18. `addon.BuildOpaqueSecret` now returns
  `(string, error)`; secretstore and flux callers propagate. The two
  remaining `panic(err)` sites in `internal/addon/catalog/*` init paths
  are `addon.Register` duplicate-name guards — distinct from the
  YAML-marshal panic U6 addressed. Pre-existing doc drift in
  `docs/architecture/addons.md:109` (arg order shown reversed) left for
  a follow-up.
- **N2 — Wire `okdctl releases list/show`** — done PR #51, merged
  2026-04-18. New `internal/cli/releases.go` wires
  `releases.OKDVersionFetcher` to two subcommands. `list --channel
  stable|all --output text|json` uses `text/tabwriter` so alignment holds
  for long tags like `4.21.0-okd-scos.10`. `show <version>` matches by
  `Version` or `Tag` and prints via `tui.DottedKeyValueFull`. Both honour
  the fetcher's existing disk cache; neither is added to
  `rootRequiredCmds` (read-only commands).
- **N5 — `okdctl kubeconfig`** — done PR #55, merged 2026-04-18. New
  `internal/cli/kubeconfig.go` prints the post-install cluster kubeconfig
  to stdout (default), writes it to a file via `--output`, or merges it
  into the first `$KUBECONFIG` path (falling back to `~/.kube/config`)
  via `--merge`. Structural YAML merge through `sigs.k8s.io/yaml`
  deduplicates `clusters`/`users`/`contexts` by `.name`; `current-context`
  is only adopted when the destination has none. Read-only — not added
  to `rootRequiredCmds`.
- **N17 — Wizard review renders all fields** — done PR #56, merged
  2026-04-18. `internal/tui/wizard/steps/review.go` now renders
  `HostPrefix` and `StaticIP.DNS`, and displays the API VIP with an
  `(auto)` suffix when the user left it blank but static IPs are
  configured — derived via `netutil.DeriveVIPFromStaticIP` at render
  time so the review doesn't silently omit the effective value.
- **N18 — Gateway-in-CIDR wizard validation** — done PR #57, merged
  2026-04-18. New exported `config.ValidateGatewayInCIDR(gateway, cidr)`
  in `internal/config/validators.go`, invoked from
  `NetworkingStepDefinition.Validate` so users cannot advance past the
  networking wizard page when the gateway falls outside the machine
  CIDR. Defers to per-field validators when either input is empty or
  malformed so the user doesn't see two errors for one bad field.
- **N4 — `okdctl cleanup` standalone** — done PR #58, merged 2026-04-18.
  New `internal/cli/cleanup.go` runs the full cleanup phase
  (`cleanup.Execute` with `Kind=Full`) against a config without a
  destroy flow, reusing `phase.ResolveClusterVIP`, `phase.BaseOptions`,
  `phase.GetTerraformEnv`, and `phase.DefaultHAProxyConfigPath`. Named
  `cleanup` so the existing `rootRequiredCmds` entry at
  `internal/cli/elevation.go:23` drives the sudo re-exec gate; no
  elevation edits needed. `--yes`/`-y` skips the confirmation prompt.
- **N6 — `okdctl config show`** — done PR #59, merged 2026-04-18.
  New `internal/cli/config.go` adds a `config` parent with a `show`
  subcommand that prints the resolved YAML to stdout with Proxmox
  `TokenID` redacted to `***`. `Username`/`Password`/`APIToken` on
  `ProxmoxConfig` already carry `json:"-"` so they never marshal —
  `redactConfig` only needs to shallow-copy and scrub `TokenID`. Read-only,
  not added to `rootRequiredCmds`.
- **N7 — `okdctl completion`** — done PR #60, merged 2026-04-18. New
  `internal/cli/completion.go` exposes `okdctl completion
  <bash|zsh|fish|powershell>` via cobra's built-in generators
  (`GenBashCompletionV2`, `GenZshCompletion`, `GenFishCompletion`,
  `GenPowerShellCompletionWithDesc`). Cobra's auto-registered bare
  `completion` command is suppressed via
  `rootCmd.CompletionOptions.DisableDefaultCmd = true` in favour of one
  with activation docs. README install section gains per-shell
  one-liners.
- **N9 — `--log-level`/`--log-format`/`--log-file` flags** — done PR #65,
  merged 2026-04-18. Three persistent flags on `rootCmd` wire through a
  new `configureLogging` helper called from `PersistentPreRunE`, which
  calls `tui.ConfigureLoggers` to mutate `stdoutLogger`/`stderrLogger`
  in place (charmlog's `SetLevel`/`SetFormatter`/`SetOutput`).
  `--log-format=json` uses `charmlog.JSONFormatter` (NDJSON).
  `--log-file` opens with `O_CREATE|O_WRONLY|O_APPEND` mode 0600 and
  duplicates both streams via `io.MultiWriter`. `tui.ProgressBarsEnabled()`
  is the new predicate consulted by `internal/download/download.go`,
  gated on stderr TTY (where `progressbar.DefaultBytes` renders) AND
  non-JSON format. A follow-up fix commit corrected an initial version
  that probed stdout TTY — the gate was wrong because the bar writes
  to stderr.
- **N15 — Wizard collects `Deployment` fields** — done PR #66, merged
  2026-04-18. New "deployment options" section in the advanced step of
  `internal/tui/wizard/steps/advanced.go` captures `Debug`,
  `SkipDepsCheck`, `TerraformEnv`, and `AutoApprove`. Bool fields use
  the existing `FieldTypeSelect` + `valYes`/`valNo` + `wizard.SetBool`
  pattern; `TerraformEnv` is a text input validated by new exported
  `config.ValidateTerraformEnv` (accepts empty or a terraform-workspace
  identifier `^[A-Za-z_][A-Za-z0-9_-]*$`). Review step renders each
  with `skip` gates so the all-defaults common case stays clean. Blank
  `TerraformEnv` is semantically correct for "no override" — the
  `GetTerraformEnv` helper in `phase/paths.go` already supplies the
  `production` fallback at runtime.
- **N20 — Doctor-check reference doc** — done PR #67, merged 2026-04-18.
  New `docs/doctor-checks.md` documents all 9 preflight checks (`host
  os`, `root check`, `path`, `tools and packages`, `sudo`, `ssh public
  key`, `pull secret`, `disk space`, `host ports`) with what each
  checks, the exact fail/warn strings extracted from
  `internal/cli/doctor.go`, and concrete fix commands. The doctor
  command's `Long` help gains a one-line pointer to the doc. No check
  implementations were touched. The roadmap cited `doctor.go:16-24`
  for the check list but the actual registry is at `:98-108` — stale
  line reference, valid item.
- **M4 — OKD release URL override** — done PR #61, merged 2026-04-18.
  New `Deployment.OKDReleaseBaseURL` YAML field and
  `OKDCTL_OKD_RELEASE_URL` env var override the hardcoded GitHub
  release URL. Resolution order in `setup.ResolveReleaseBaseURL`:
  env > config > default
  `https://github.com/okd-project/okd/releases/download`.
  `strings.TrimRight` normalizes trailing slashes. Mirrors that
  preserve upstream path layout (`<base>/<version>/<filename>`) work
  out of the box; full URL-template overrides for non-standard mirror
  shapes are scoped to M5.
- **N23 — HTTP error surface** — done PR #62, merged 2026-04-18.
  `httpStatusError` now carries `Method` and a ≤256-byte `Body`
  excerpt; `Error()` emits `HTTP <status> <method> <url>: <body>`.
  `fetchToFile` reads up to 256 bytes via `io.LimitReader` before
  returning the error. `bodySnippet` strips non-printable bytes
  (defense against terminal escape sequences in an error body) and
  trims whitespace; no credential scrubbing — an earlier bare-token
  heuristic was dropped because it caught only one narrow case while
  missing JSON/JWT/prose embeddings, and logs are not shipped today.
  A key-based scrubber (`token=`, `password=`, etc.) is the right
  follow-up when M2 (debug-bundle) or `--log-file` persistence lands.
  Retry behaviour unchanged — `isRetryable` still switches on
  `httpErr.Status`.
- **N8 — Step timing + per-step deploy summary** — done PR #64,
  merged 2026-04-18. `StepResult` gains `StartedAt time.Time` and
  `Duration time.Duration`. `Orchestrator.executeStep` captures
  `time.Now()` before the skip/execute branch and sets both fields
  on all three return paths (skip, fail, success); new `Results()`
  getter returns a copy of the slice under `RLock`. Each phase's
  `Execute` returns `[]distribution.StepResult` alongside existing
  returns; `executeFullDeployment` concatenates across setup +
  install + postinstall and passes the slice to `PostDeploySummary`,
  which renders a "steps" section (step id | ok/skip/fail | duration)
  with a total row. Destroy not instrumented — it does not flow
  through the post-deploy summary. Unblocks L5 (Prometheus metrics),
  N10 (Ctrl-C partial-progress summary), and M1 (`okdctl status`).
- **N11 — `--yes`/`--force` parity on deploy and update-ingress** — done
  PR #68, merged 2026-04-19. `internal/cli/deploy.go` gains `--yes`/`-y`
  that sets `deployNonInteractive = true` at `runDeploy` entry so the
  flag surface matches destroy (`--force`/`-y`) and update-ingress
  (`--yes`/`-y`). `internal/cli/confirm.go:promptForConfirmation`
  short-circuits to `(false, nil)` when
  `term.IsTerminal(os.Stdin.Fd())` is false, preventing the prompt
  goroutine from dead-locking in CI or piped invocations. All three
  existing call sites (destroy, update-ingress main, update-ingress
  HostNetwork-conversion) inherit the fix through the shared helper.
  `//nolint:gosec // G115` matches the existing
  `internal/tui/wizard/model.go:163` suppression for the uintptr→int
  cast.
- **L13 — Auto-update version check on startup** — done PR #69, merged
  2026-04-19. New `internal/version/updatecheck.go` with
  `BackgroundCheck(ctx)` fires a goroutine that queries
  `/repos/qxtaiba/okdctl/releases/latest` under a 4s
  `context.WithTimeout`; results are cached 24h under
  `$UserCacheDir/okdctl/update-check.json` (atomic tmp + `os.Rename`,
  mode 0600). `OKDCTL_NO_UPDATE_CHECK=1` short-circuits before the
  goroutine starts. `internal/cli/root.go:execute()` fires the check
  before `rootCmd.ExecuteContext` and drains the buffered channel via
  a `select` with `time.After(100ms)` after the command returns —
  only on exit 0, so error paths stay clean. Non-2xx responses,
  non-semver current version, and cache I/O errors all fail silently
  via `slog.Debug`.
- **M13 — Typed error hierarchy** — done PR #70, merged 2026-04-18.
  New `internal/errtypes` package defines `ConfigError`, `NetworkError`,
  `ClusterError`, and `AuthError` as concrete pointer-receiver structs
  (`Msg string`, `Err error`) with `Error()` and `Unwrap()` so
  `errors.As`/`errors.Is` both work across the chain.
  `WrapValidation(*config.ValidationResult) error` bridges the existing
  validation output into `*ConfigError` without touching the
  `ValidationResult` API. Five exemplar sites wrapped in this PR
  (`loadConfig`, `Download`, `ValidateClusterAccess`, `ensureRoot`,
  envfile insecure-perms check). Broader migration across remaining
  phase/addon error returns is tracked as **M13b** so U4's dispatch
  eventually covers every error path.
- **U4 — Exit codes collapse everything to 0/1/130** — done PR #70,
  merged 2026-04-18. `internal/cli/root.go:execute()` now dispatches
  exit codes via `errors.As` against the `internal/errtypes` types:
  `ConfigError`=2, `NetworkError`=3, `ClusterError`=4, `AuthError`=5,
  other=1. SIGINT/SIGTERM still return 130 via the existing
  `ctx.Err()` guard. The exit-code table is documented in the
  `internal/cli` package doc (authoritative for operators and CI
  scripts); the `internal/errtypes` package doc mirrors the same
  mapping from the type-producer side. Coverage is partial until
  M13b lands — sites not yet wrapped fall through to exit 1.
- **N19 — Addon-specific docs in `docs/addons/`** — done PR #71,
  merged 2026-04-18. New `docs/addons/flux.md` and
  `docs/addons/secretstore.md` cover purpose, when-to-use, defaults,
  configuration, common failure modes, and uninstall behaviour. All
  defaults are quoted from `internal/addon/catalog/{flux,secretstore}/`
  with source line refs. The flux doc explicitly distinguishes the
  warn-on-query-fail vs fatal-zero-replicas paths in
  `Verify` (an earlier draft conflated them). README's addon-system
  sentence gains two per-addon links; no new addon section was created
  since one did not exist.
- **M17 — Architecture diagrams (Mermaid)** — done PR #72, merged
  2026-04-18. Inline Mermaid `flowchart` blocks added to the three
  existing `docs/architecture/*.md`: `phases.md` shows
  setup→install→postinstall plus the inverse destroy→cleanup path;
  `wizard.md` shows all 11 wizard steps with explicit conditional
  routing for `node-placement` (Proxmox-only) and `files` (OKD-only)
  via separate "step or skip" branches; `addons.md` mirrors the
  `Manager.InstallAll` loop including dep-failed skip and rollback on
  install/verify error. No new docs created.
- **N1 — Wire `okdctl addon list/install/uninstall/verify`** — done
  PR #73, merged 2026-04-18. New `internal/cli/addon.go` registers
  four subcommands. `list` enumerates `addon.All()` + `cfg.Addons` and
  prints a tabwriter table named `CONFIG-ENABLED` (with a footer
  pointing at `verify` for live cluster state — the column is config
  truth, not cluster truth). `install [name]` and `install --all` map
  to `Manager.InstallOne` / `InstallAll`; the `Long` text documents
  the asymmetric rollback (single = all-or-nothing, `--all` =
  per-addon continuation). `uninstall <name>` surfaces
  `Manager.Uninstall`'s dependency-block error verbatim. `verify`
  consumes a new `Manager.VerifyAll` shape — `([]VerifyResult, error)`
  — so the CLI can render NAME/STATUS rows without losing per-addon
  detail; aggregated error is replaced by a sentinel
  `N addon(s) failed verification` so cobra doesn't double-print.
  Elevation gate refactored: instead of adding `"addon"` to
  `rootRequiredCmds` (which would force sudo on `list`/`verify`), the
  mutating leaves carry `Annotations["requiresRoot"] = "true"`, and
  `requiresRoot` checks the leaf annotation before walking ancestors.
  Other commands (`deploy`, `destroy`, `cleanup`, `update-ingress`)
  unchanged.
- **N3 — `okdctl config validate` standalone** — done PR #74, merged
  2026-04-18. New `configValidateCmd` under the existing `config` parent
  (`internal/cli/config.go`) loads the config, prints
  `ValidationSummary(result)` (the same renderer the deploy flow uses),
  and returns `errtypes.WrapValidation(result)` — nil on success (exit 0),
  `*ConfigError` on failure (exit 2 via `root.exitCodeFor`). No new flag
  needed; the persistent `--config`/`-c` on rootCmd is inherited.
- **M5 — Tool binary versions / URLs overridable** — done PR #75, merged
  2026-04-18. New `Deployment.ToolVersions` YAML map
  (`ToolVersionOverride{Version, URLTemplate}`) plus env vars
  `OKDCTL_{HELM,SOPS,YQ}_{VERSION,URL}`. `ResolveToolURL` and
  `ResolveToolVersion` in
  `internal/distribution/okd/setup/artifacts.go` mirror M4's
  env > config > default resolution. URL templates use named `{version}`
  and `{arch}` placeholders substituted via `strings.NewReplacer` — safe
  with zero, one, or both placeholders. An earlier draft used
  `fmt.Sprintf`; it emitted `%!(EXTRA …)` on verbatim-URL overrides and
  was dropped in the review-round refactor. yq keeps the
  `/releases/latest/download/` GitHub redirect (the only path that
  resolves without a concrete tag), so its `Version` override is a silent
  no-op unless the operator also supplies a URLTemplate containing
  `{version}`. Scope is intentionally narrow: terraform, OS packages, and
  FCOS still hit upstream repos.
- **M16 — Autogenerated CLI reference** — done PR #76, merged 2026-04-18.
  New `cmd/okdctl-gen-docs` binary generates 18 Markdown files under
  `docs/cli/` via `doc.GenMarkdownTree`. Exported `cli.RootCmd()` in
  `internal/cli/docs.go` surfaces the package-private root tree for
  offline tooling. `DisableAutoGenTag = true` suppresses cobra's
  date-stamped footer so regeneration is deterministic — without it the
  drift check is a false-positive factory. Makefile `docs`/`docs-check`
  targets and the new CI `docs-go` job fail on drift, checking both
  tracked-file diff and `git ls-files --others` (so new subcommands can't
  land without updating docs). README gains a release-checklist section.
  Doctor command metadata split out of `doctor.go` (still Linux-only,
  untouched) into `doctor_cmd.go` (shared, no build tag) and
  `doctor_stub.go` (`!linux`) so the cobra tree is platform-consistent
  for doc gen — preserving the "doctor is Linux-only at runtime"
  invariant while fixing a macOS/Linux drift that would have made the
  drift check unresolvable.
- **N10 — Ctrl-C partial-progress summary + resume hint** — done PR #77,
  merged 2026-04-19. New `InterruptSummary` in `internal/cli/summary.go`
  reuses the N8 `StepResult` plumbing to render a partial-progress box
  plus "resume with okdctl deploy/destroy" hint. `executeFullDeployment`
  (helpers.go) and `runDestroy` (destroy.go) detect `errors.Is(err,
  context.Canceled)` and print the box before returning the bare
  cancellation error so root.go's `ctx.Err() != nil → exit 130` dispatch
  still fires. `destroy.Phase.Execute` and `okd.Provisioner.Destroy`
  widened to return `([]distribution.StepResult, error)` so destroy has
  the same step data the deploy summary already used.
- **N25 — Progress bars for long-running operations** — done PR #78,
  merged 2026-04-19. New `tui.StartSpinner(ctx, desc) func()` in
  `internal/tui/spinner.go` renders a stderr spinner gated on
  `tui.ProgressBarsEnabled()` (the N9 predicate). Terraform apply
  (`internal/infrastructure/proxmox/proxmox.go`), bootstrap wait, and
  install monitor (`internal/distribution/okd/install/monitor.go`) wrap
  their long-running call with start/stop. No new third-party dep —
  stdlib goroutine + 120ms ticker; `sync.Once` guards the stop closure;
  `ctx.Done()` is one of the select cases so the spinner exits on
  cancel. Determinate progress wasn't viable: terraform buffers output
  through `executor.Run`, and openshift-install emits unparseable
  log lines.
- **N16 — Wizard collects `FCOSIso` / `TokenID` / `AdditionalNetworks`** —
  done PR #79, merged 2026-04-19. Three new fields wired into the
  wizard: `token_id` text field in the Proxmox credentials section;
  `fcos_iso` storage-ref `FieldTypeSelect` populated by walking
  ISO-capable storage pools and listing volids with `.iso` suffix
  (proxmox_discovery.go uses go-proxmox `Storage(...).GetContent(...)`);
  `additional_networks` `FieldTypeMultiSelect` over discovered bridges,
  with a new `MultiSelectField` component (j/k cursor, space toggle).
  `parseAdditionalNetworks` is a bridge-keyed merge — hand-authored
  `Model` and `VLANTag` survive a wizard re-run (caught in code review).
  Discovery error path distinguishes "token-only credentials" from
  generic missing-credentials so users understand wizard discovery
  still requires password auth (token id is saved for deploy use with
  `PROXMOX_VE_API_TOKEN_SECRET`).
- **D1 — document go-proxmox v0.x abandonment plan** — closed 2026-04-19
  as done-by-prior-work. `CLAUDE.md:154-182` already contains a complete
  `## Dependencies` section covering the permissive-license rule
  (MIT/Apache-2.0/BSD only), the v0.x justification format with the
  go-proxmox v0.4.x entry and its ~200-LOC REST-only fallback, the
  GitHub-Actions SHA-pin expectation with version-trailer format, and
  the stdlib-first rule. Landed in commit `d69c36d refactor(repo):
  resolve 2026-04-18 audit findings (tiers a+b)`.
- **D3 — pin tool-install @latest references** — closed 2026-04-19 as
  done-by-prior-work. `Makefile:60` pins `air@v1.61.7`, `Makefile:80`
  pins `golangci-lint@v2.11.4`, `ci.yml:54` pins `govulncheck@v1.1.4`,
  `ci.yml:80` pins `yamlfmt@v0.14.0`. Zero `@latest` left in tool-install
  sites. Landed in commit `d69c36d`.
- **D4 — tighten terraform version floor in CI** — closed 2026-04-19 as
  done-by-prior-work. `ci.yml:89` is `terraform_version: "1.10.3"` (was
  `"1.10"`). Landed in commit `d69c36d`.
- **D5 — plan gorilla/websocket removal path** — closed 2026-04-19 as
  done-by-prior-work. `CLAUDE.md:167-171` documents non-reachability
  (okdctl's Go source contains zero `websocket` references outside
  `go.mod`/`go.sum`) and records the tracking signal for the
  go-proxmox → `coder/websocket` upstream bump so the transitive
  update lands without local code changes. Audit ledger
  `.claude/audits/resolved-2026-04-18.jsonl:75` records this as
  resolved under `task-21-D5`. Landed in commit `d69c36d`.
- **M14 — Correlation ID per deploy run** — done PR #82, merged
  2026-04-19. `uuid.NewString()` is minted at the top of `runDeploy`
  and `runDestroy` (before credential/config/wizard log lines) and
  pinned on the package-level charmlog loggers via new
  `tui.SetRunID`. Subsequent `tui.X` calls and every slog record from
  the provisioner's `SimpleLogger()` snapshot carry `run_id`
  automatically. `tui.RunID()` reads the pinned value back for the
  summary renderer; `PostDeploySummary` and `InterruptSummary` gained
  a `runID string` parameter and render it via `sb.kv("run_id",
  runID)`. `github.com/google/uuid` promoted from transitive (via
  go-proxmox) to direct require.
- **M13b — Complete errtypes migration across phase code** — done PR
  #83, merged 2026-04-19. Wraps every exported phase/addon boundary
  in `internal/distribution/okd/{setup,install,postinstall,destroy,
  cleanup}`, `internal/addon/manager.go`, and
  `internal/credentials/envfile.go` with the appropriate `errtypes.*`
  type. ~30 files touched, ~100 wrapping sites. `ctx.Err()` paths in
  `install/monitor.go` left as raw `fmt.Errorf` so
  `errors.Is(err, context.Canceled)` still resolves and root.go's
  exit-130 dispatch stays intact. U4's `errors.As` now routes the
  full failure surface to exit codes 2–5 instead of falling through
  to 1. Cleanup destroy paths are NonFatal steps so those wraps are
  belt-and-braces; every other site is a Fatal boundary. Sweep took
  three review rounds — gap narrowed from ~30 sites (round 1) → 14
  (round 2) → 9 (round 3), all addressed inline.
- **M12 — Generalize SecretStore beyond 1Password** — done PR #81,
  merged 2026-04-19. New package-private `provider` interface in
  `internal/addon/catalog/secretstore/providers.go` with three impls:
  `onepassword` (default, preserves existing behavior and file
  names), `vault` (full — `vault-token.txt` + ESO SecretStore CRD
  with token auth), `bitwarden` (full — Bitwarden Secrets Manager /
  Vaultwarden-compatible, requires an in-cluster
  `bitwarden-sdk-server` sidecar not provisioned by this addon).
  Install now applies both the provider's auth Secrets AND an ESO
  `SecretStore` CRD named `okdctl-secretstore` (previously only
  Opaque Secrets). `ValidateSettings` dispatches to the provider's
  validator so misconfig surfaces before `oc apply`.
  `onepassword_vaults` setting exposes the 1P vault map as CSV
  (`"homelab=1,shared=2"`) with default `"homelab=1"`; a structured
  key-value editor is tracked as N26. Design investigation returned
  M19 (typed decoder) and M20 (grouped wizard fields) as the
  follow-on items.
- **M19 — Typed addon settings via per-addon decoder method** — done
  PR #89, merged 2026-04-19. `ConfigurableAddon` grows
  `DecodeSettings(map[string]string) (any, error)`; `flux.Settings`
  and `secretstore.Settings` are the per-addon typed structs.
  `secretstore.Settings` carries three provider sub-structs
  (`OnePasswordSettings`, `VaultSettings`, `BitwardenSettings`);
  `DecodeSettings` populates only the sub-struct matching the active
  `Provider`, so `s.Bitwarden.OrganizationID == ""` is structurally
  scoped to the bitwarden provider — no more string-prefix matching.
  `Install` and `ValidateSettings` on both addons call `DecodeSettings`
  once at entry and operate on typed fields. Linter required two
  renames: `flux.FluxSettings` → `flux.Settings` and
  `secretstore.SecretStoreSettings` → `secretstore.Settings` (revive
  stutter); provider names (`onepassword`/`vault`/`bitwarden`) got
  package-private constants (goconst). Design choice B (added to
  `ConfigurableAddon` directly) over design choice A (sub-interface
  `TypedConfigurableAddon`) — repo has only two addon implementers,
  both in-tree, no external type assertions to break.
- **M20 — Grouped wizard fields for structured addon settings** — done
  PR #89, merged 2026-04-19. `addon.WizardField` grows an optional
  `Group string`. Secretstore's `WizardFields()` annotates each field
  with its provider group and surfaces 10 provider-specific settings
  that were previously absent from the hardcoded wizard. The
  `AddonsStepDefinition` at `internal/tui/wizard/steps/addons.go`
  splits the single "1password secret store" section into four
  sections — common (`enabled`, `provider` dropdown, `secrets_dir`),
  onepassword, vault, bitwarden — each with a group-level title.
  Approach A (static `SectionDefinition` entries) chosen over approach
  B (dynamic renderer walking `WizardProvider`) — every other wizard
  step is hand-authored; a dynamic renderer just for secretstore would
  create asymmetry. Optional per-group hiding based on the selected
  provider is deferred: `DataDrivenStep` has no per-section
  `ShouldShow` and plumbing one exceeds M20 scope. Group headers alone
  materially improve UX over the previous flat 2-field view. Flux
  unchanged.
- **M2 — `okdctl debug-bundle`** — done PR #90, merged 2026-04-19.
  New `internal/cli/debug_bundle.go` collects redacted config
  (via N6's `redactConfig`), the `--log-file` from N9, `oc adm
  must-gather` output, `terraform state list` (raw `terraform.tfstate`
  excluded — it carries Proxmox credentials), `okdctl doctor` output,
  and runtime/version metadata into a gzip tarball with a top-level
  `manifest.yaml`. Each section returns a `manifestEntry` instead of
  fatally erroring, so a partial bundle is still useful in the exact
  scenario (broken cluster) where bundles are needed most.
  Doctor collection is build-tag-split (`debug_bundle_doctor.go` /
  `debug_bundle_doctor_stub.go`), mirroring the `doctor_cmd.go` /
  `doctor_stub.go` pattern from M16. Must-gather is bounded by a
  5-minute context timeout and a `--skip-must-gather` flag; output
  is archived through `os.OpenRoot`-scoped reads so symlinks cannot
  redirect reads outside the temp dir (TOCTOU-safe). Bundle
  correlation id minted via `uuid.NewString()`; `github.com/google/uuid`
  promoted from indirect to direct in `go.mod` (M14 had left this
  drift). Not added to `rootRequiredCmds` — read-only collection.
  Review round 1 caught three issues: double `loadConfig` print,
  tar/gzip not deferred (truncation risk on mid-run failure), and
  the go.mod tidy drift; round 2 PASSed.
- **L14 — Coverage thresholds + codecov in CI** — done PR #85, merged
  2026-04-19. New `.github/scripts/coverage-check.sh` reads `coverage.out`
  and enforces per-package floors from `.github/coverage-floors.conf`
  (key=value, `*` is default, `total` gates the aggregate). All floors
  start at 0 so the scaffolding passes vacuously today; N12/N13 tighten
  specific packages one line at a time when tests land. Self-contained
  shell check chosen over codecov SaaS — no token, no third-party
  dashboard, acceptance's "or equivalent" allows the substitution.
- **D2 — evaluate progressbar swap for bubbles/progress** — done PR #86,
  merged 2026-04-19. Dropped `schollz/progressbar/v3` in favour of a
  ~60 LOC hand-rolled `io.WriteCloser` in
  `internal/download/progress.go` that reuses `tui.ProgressBarsEnabled()`
  and `golang.org/x/term` for width. Cleanup removes
  `mitchellh/colorstring` (2019-stale) and `chengxilo/virtualterm` from
  the transitive graph. `bubbles/v2/progress` swap was re-rejected as
  still-strictly-heavier (requires bubbletea Program); CLAUDE.md §Deps
  note updated from "Kept" to "Removed".
- **M1 — `okdctl status` / `describe`** — done PR #84, merged 2026-04-19.
  New `internal/cli/status.go` wires three subcommands. `status` prints
  API reachability (`oc get --raw /healthz`), node counts by role
  (master/worker from `node-role.kubernetes.io/*` labels via
  `oc get nodes -o json`), cluster-operator degraded count
  (`oc get clusteroperators --no-headers`), and addon VerifyAll results.
  `describe node <name>` and `describe addon <name>` drill into a single
  resource with tabwriter output. Reuses `phase.BasePhase` + executor
  via a new `OcOutput` one-shot helper added next to `OcPollOutput` in
  `phase/kubectl.go` (polling helper was the wrong shape for a one-shot
  describe). Read-only — not added to `rootRequiredCmds`.
- **M3 — `--dry-run` / `--plan` mode** — done PR #87, merged 2026-04-19.
  New `--dry-run` flag on `deploy`, `destroy`, and `update-ingress`.
  Re-exec-as-root gate in `cli/elevation.go` already probed
  `cmd.Flags().GetBool("dry-run")` — adding the flag on the three
  commands activates the bypass. New `terraform.Executor.PlanStreamed`
  wires terraform stdout/stderr directly to the terminal via
  `executor.RunInteractive`; new `proxmox.Provider.PlanOnly` does
  Init + PlanStreamed. Deploy dry-run renders a 31-entry step listing
  through new `DryRunSummary`. Plan failures wrap as
  `*errtypes.ConfigError` → exit 2 via the U4 taxonomy.
- **L5 — Prometheus metrics endpoint during deploy** — done PR #87,
  merged 2026-04-19. New `--metrics-addr :9090` on deploy starts an
  HTTP server serving `/metrics` in Prometheus text format for the
  lifetime of the run; disabled when empty. Four metric families —
  `okdctl_deploy_step_total` (counter, success/failure labels),
  `okdctl_deploy_step_duration_seconds` (histogram, 12 buckets),
  `okdctl_deploy_current_step` (gauge, set by StepStarted/StepFinished
  on the new `MetricsRecorder` interface), `okdctl_deploy_duration_seconds`
  (gauge). Hand-rolled text renderer in `internal/deploymetrics/`
  (no `prometheus/client_golang` dep — the four metrics don't justify
  ~15 transitive packages). Orchestrator `MetricsRecorder` interface
  with a no-op default means existing callers are unaffected;
  `BasePhase.Recorder` propagates through setup/install/postinstall.
- **N26 — TUI key-value map editor component** — done PR #92, merged
  2026-04-19. New `components.KeyValueField` in
  `internal/tui/wizard/components/key_value_field.go` renders a focused
  (key, value) table mirroring the `MultiSelectField` shape — `j/k`
  moves rows, `h/l` switches column, `a` adds, `d` deletes, `ctrl+e`
  toggles edit mode (the host `DataDrivenStep` consumes `enter`/`tab`/
  `shift+tab` for inter-field navigation, same constraint documented
  on `MultiSelectField`). `FieldDefinition` gains `Type:
  FieldTypeKeyValue` and `KVAsDelimitedString bool` — true = CSV
  `"k1=v1,k2=v2"`, false = YAML-map `"k1: v1\nk2: v2"`. Secretstore
  wizard's `secretstore_op_vaults` retrofit as the first consumer in
  CSV mode; `Default: "homelab=1"` round-trips unchanged so existing
  YAMLs keep working. Review round 1 flagged 11 findings (empty-key
  pair emission in `Value()`, sentinel-vs-dynamic error, redundant doc
  comments, dead `defaultValue` field, file-name snake_case, host-step
  key-consumption type doc, one-frame width drift in `addRow`,
  delimiter-round-trip doc on `Value`/`SetValue`, blink-cmd plumbing
  through `syncInputFocus`/`Focus`/`toggleEditMode`, 73→60-char commit
  subject) — all addressed in round 2. Develop merged 7 items
  (M19/M20/M2/M1/M3/L5/L14/D2) during this session; rebase caught the
  drift and moved the retrofit target onto M20's grouped
  `secretstore_op_vaults` field in the onepassword section rather than
  adding a duplicate-binding field in the earlier "common" layout.
- **U2 — Wizard never sets `Provider.Type`** — done PR #52, merged
  2026-04-18. `ProxmoxStepDefinition` replaced its `ShouldShow`
  catch-22 (hidden when `Provider.Type` was unset → step never ran →
  type stayed unset) with an `Apply` hook that assigns
  `config.ProviderProxmox`. Mirrors `DistributionStep.Apply` at
  `internal/tui/wizard/steps/distribution.go:236` and the Apply hooks
  already in `networking.go` / `files.go` / `node_placement.go`.
  Single-distribution assumption is encoded in the hook body; revisit
  if L1–L3 ever move out of Skipped.
- **U1b — Clean remote Proxmox FCOS ISO on destroy** — done PR #54,
  merged 2026-04-18. New `StepRemoveRemoteISO` in the destroy phase
  SSHs to the Proxmox host, enumerates `<isoDir>/fedora-coreos-*.iso`
  via `find -print0`, checks the running-VM set via `pvesh` per-vmid
  config queries, and removes the file only if no running VM
  references it. Shared SSH plumbing extracted to new `phase/ssh.go`
  (`ProxmoxBareHost`, `SSHRun`); `setup/upload.go` now uses the same
  helper. Safety layers: `validateISODir` rejects shell
  metacharacters / whitespace / quotes; `refuseUnsafeISOPath`
  restricts filenames to `<isoDir>/fedora-coreos-*.iso`; paths are
  single-quoted before shell interpolation. VM-reference scan walks
  device fields (`ide*`, `sata*`, `scsi*`, `virtio*`, `boot`,
  `bootdisk`) with `file=`-prefix strip and suffix match; fails
  closed on any pvesh error. New `--keep-isos` flag on `destroy`
  preserves the ISO for users chaining destroy → re-deploy. Four
  review rounds resolved 15+ findings (helper duplication, `rm`/
  `find` injection, wrong pvesh endpoint, fail-open parse, comment
  density).
- **N14 — `go vet ./...` in CI** — done PR #53, merged 2026-04-18.
  New `vet-go` job in `.github/workflows/ci.yml` mirrors
  `lint-go` / `build-go` (same pinned action SHAs, `go-version-file:
  go.mod`, `ubuntu-latest`). Closes the gap where `make vet` existed
  locally but CI never invoked it.
- **L15 — Air-gap feasibility + scoping doc** — superseded by **M40**
  (2026-04-20). L15 locked a `FetchPlan` + `Resolver` + `oc-mirror`-wrapper
  architecture that was subsequently ripped out. The scoping doc is
  deleted alongside the implementation. Retained here for roadmap
  archaeology; see M40 for the postmortem.
- **M6 — `DefaultBinDir` configurable (rootless support)** — done PR
  #93, merged 2026-04-20. New `DeploymentConfig.BinDir` YAML field and
  `OKDCTL_BIN_DIR` env var override the hardcoded `/usr/local/bin`,
  resolved via a new `phase.ResolveBinDir(cfg)` helper
  (env > config > default, mirroring M4's `ResolveReleaseBaseURL`
  pattern). `system.ExpandPath` is applied before validation so
  `~/bin` matches pull_secret / ssh_public_key ergonomics. Setup
  install sites (`InstallToolsToSystem`, `installBinaryToPath`) and
  the cleanup binary-removal path thread the resolved value through
  `okd.go:Prepare`, `cli/cleanup.go`, and `destroy/steps.go`; a shared
  `phase.BinDirOrDefault` replaces three copies of the zero-value
  fallback. `phase.PreflightBinDir` encapsulates the env-only
  resolution `main.preflight` uses (config isn't parsed at startup)
  so doctor's renamed `bin dir on path` check can compare against
  exactly what preflight chose. New doctor `bin dir` check probes
  existence and writability separately (stat errors reported with
  raw error); user-configured fail text makes the sudo re-exec
  semantics explicit (binaries are root-owned; chown to manage
  later). `resolveBinDirForDoctor` memoises the config load via
  `sync.OnceValue` and surfaces load failures via a detail suffix
  plus pass→warn demotion so a malformed YAML never reads as green.
  Elevation is **not** rewired: acceptance bullet 2 is satisfied
  machinery-only (ResolveBinDir / IsDirWritable / checkBinDir all
  ship) — a blanket re-exec skip would bypass sudo for
  deploy/destroy/cleanup/update-ingress, which also write to
  `/etc/haproxy`, `/etc/dnsmasq.d`, `/var/www/html` and run `dnf`.
  Future standalone `okdctl install-tools` subcommand is the
  correct home for the scoped skip. Seven review rounds resolved
  the full surface: round 3 (destroy cleanup.Options missing
  BinDir, setup zero-value), round 4 (`ResolveBinDir` bypassed
  `ValidateBinDir`, path-vs-default equality fragility,
  preflight/doctor contradiction when env set), round 5
  (missing-dir vs not-writable distinction, `checkPath` warn-gate
  on post-validation env, docs parametric fix snippet, tilde
  expansion, `PreflightBinDir` vs `checkPath` consistency), round
  6 (malformed-config demote, stat error branch, comment density),
  final PASS on round 7.

## Appendix — full item ledger

| ID | Item | Disposition |
|---|---|---|
| U1 | Cleanup leaves FCOS ISO cache | **Done** (audit error — see Completed) |
| U1b | Clean remote Proxmox FCOS ISO on destroy | **Done** (PR #54) |
| U2 | Wizard never sets `Provider.Type` | **Done** (PR #52) |
| U3 | HTTP downloads no retry | Sprint 1 |
| U4 | Exit codes only 0/1/130 | **Done** (PR #70) |
| U5 | `cmd.Run()` without ctx (2 sites) | **Done** (landed in 65d8fce — see Completed) |
| U6 | `BuildOpaqueSecret` panic | Sprint 1 |
| N1 | `okdctl addon list/install/uninstall/verify` | **Done** (PR #73) |
| N2 | `okdctl releases list/show` | Sprint 1 |
| N3 | `okdctl config validate` standalone | **Done** (PR #74) |
| N4 | `okdctl cleanup` standalone | **Done** (PR #58) |
| N5 | `okdctl kubeconfig` | **Done** (PR #55) |
| N6 | `okdctl config show` | **Done** (PR #59) |
| N7 | `okdctl completion` | **Done** (PR #60) |
| N8 | Step timing + deploy summary | **Done** (PR #64) |
| N9 | `--log-level/--log-format/--log-file` flags | **Done** (PR #65) |
| N10 | Ctrl-C partial-progress summary | **Done** (PR #77) |
| N11 | `--yes` parity on deploy/update-ingress | **Done** (PR #68) |
| N12 | Unit tests for `netutil` | Deferred |
| N13 | Unit tests for `config/validators` | Deferred |
| N14 | `go vet` in CI | **Done** (PR #53) |
| N15 | Wizard: Deployment fields | **Done** (PR #66) |
| N16 | Wizard: FCOSIso/TokenID/AdditionalNetworks | **Done** (PR #79) |
| N17 | Wizard review completeness | **Done** (PR #56) |
| N18 | Gateway-in-CIDR validator | **Done** (PR #57) |
| N19 | Addon-specific docs | **Done** (PR #71) |
| N20 | Doctor-check reference doc | **Done** (PR #67) |
| N21 | `CONTRIBUTING.md` | Deferred |
| N22 | Troubleshooting / FAQ | Deferred |
| N23 | HTTP error context | **Done** (PR #62) |
| N25 | Progress bars for long ops | **Done** (PR #78) |
| N26 | TUI key-value map editor component | **Done** (PR #92) |
| M1 | `okdctl status` / `describe` | **Done** (PR #84) |
| M2 | `okdctl debug-bundle` | **Done** (PR #90) |
| M3 | `--dry-run` / `--plan` mode | **Done** (PR #87) |
| M4 | OKD release URL override | **Done** (PR #61) |
| M5 | Tool binary versions override | **Done** (PR #75) |
| M6 | `DefaultBinDir` rootless | **Done** (PR #93) |
| M7 | Provider interface extraction | **Skipped** |
| M8 | MetalLB addon | Deferred (under R2) |
| M9 | cert-manager addon | Deferred (under R2) |
| M10 | ingress-nginx addon | Deferred (under R2) |
| M11 | kube-prometheus-stack addon | Deferred (under R2) |
| M12 | SecretStore multi-provider | **Done** (PR #81) |
| M13 | Typed error hierarchy | **Done** (PR #70) |
| M13b | Complete errtypes migration across phase code | **Done** (PR #83) |
| M14 | Correlation ID | **Done** (PR #82) |
| M15 | Integration test harness | Deferred |
| M16 | Autogenerated CLI reference | **Done** (PR #76) |
| M17 | Architecture diagrams | **Done** (PR #72) |
| M18 | Homebrew tap | Deferred |
| M19 | Typed addon settings (decoder method) | **Done** (PR #89) |
| M20 | Grouped wizard fields for addons | **Done** (PR #89) |
| M21-M27, M33 | Air-gap workstream (all superseded) | **Reverted** (M40) |
| M29 | GitHub Artifact Attestations for release binaries | **Done** (PR #94) |
| M31 | *(unassigned — was OKD version floor, reversed 2026-04-20)* | n/a |
| M34 | *(unassigned — was air-gap colonoscopy audit, reverted under M40)* | n/a |
| M40 | Air-gap workstream removed | **Done** |
| L1 | libvirt/KVM provider | **Skipped** |
| L2 | vSphere/AWS/Equinix/bare-metal/Vagrant | **Skipped** |
| L3 | Multi-distribution (RKE2, vanilla) | **Skipped** |
| L4 | `okdctl upgrade` | Deferred |
| L5 | Prometheus metrics endpoint during deploy | **Done** (PR #87) |
| L6 | OpenTelemetry tracing | Deferred |
| L7 | Storage addons (Rook-Ceph) | Deferred (under R2) |
| L8 | Backup addons (Velero / VolSync) | Deferred (under R2) |
| L9 | Policy addons (Kyverno / Gatekeeper) | Deferred (under R2) |
| L10 | Argo CD addon | Deferred (under R2) |
| L11 | Service mesh addons | Deferred (under R2) |
| L12 | Container image distribution | **Skipped** |
| L13 | Auto-update version check | **Done** (PR #69) |
| L14 | Coverage thresholds + codecov | **Done** (PR #85) |
| L15 | Air-gap feasibility + scoping doc | **Done** (scoping complete — see Completed; M21–M28 filed) |
| R1 | Addon category model + design doc | Workstream (design-doc-first) |
| R2 | Specific addons after R1 | Deferred conversation |

## Scaffolding — verify intent (no code change)

Captured from the 2026-04-18 full audit. Per MEMORY.md §scaffolding, these
are exported-but-unreferenced symbols that MAY be future-API-shaped. Do
NOT delete without first confirming against the roadmap (either with a
code owner or by resolving the symmetric-sibling it pairs with). Each
entry lists the stable finding ID so subsequent audit runs mark it
resolved when the symmetric caller lands.

| Finding ID | Location | Why kept | Symmetric sibling |
|---|---|---|---|
| `api:25fa1be8:export-no-caller-configure` | `internal/distribution/okd/firewall/firewall.go:88` | `Configure` has no caller; `RemoveRules` (sibling) has one. Shaped for a future "configure an arbitrary port set" verb. | `RemoveRules` |
| `api:66f217c9:export-no-caller-getlatestforminor` | `internal/distribution/okd/releases/okd.go:54` | `GetLatestForMinor` / `GetLatestStable` unreferenced; `FetchVersions` is used. Shaped for `okdctl releases latest [--minor N.M]`. | `FetchVersions` |
| `api:1d5afa08:export-no-caller-shortversion` | `internal/distribution/okd/releases/types.go:76` | `ShortVersion` unreferenced; `DisplayName`, `Major`, `Minor` are used. | `DisplayName` |
| `api:98723e5d:export-no-caller-validateclusteraccess` | `internal/distribution/okd/install/flux.go:15` | `ValidateClusterAccess` / `SetupClusterAccess` / `SetupKubeconfig` only called in-package; Phase itself is exported one hop away from the CLI. | `Phase` |
| `smell:2c4d8e6b:unused-metadata-field` | `internal/addon/addon.go:24` | `AddonInfo.Category` populated ("gitops", "secrets") but unread. Shaped for a category-grouped addon listing. | — (see R1 Addon category model) |
| `smell:2be6306e:scaffolding-registry-api` | `internal/addon/registry.go:78` | `addon.IsRegistered` unreferenced but is the symmetric sibling of `Register`/`Get`/`All`/`Enabled`/`Names`. | `Register` |
| `err:d6b325cb:vocab-ad-hoc-sentinel` | `internal/infrastructure/proxmox/types.go:5` | `ErrNotConnected`, `ErrTerraformNotConfigured` exist with no `errors.Is` callers but pair symmetrically with a future `Connected()` probe. | `Connect` |
| `err:a4001485:vocab-gap-cert-pending` | `internal/errtypes/errtypes.go:5` | errtypes vocabulary covers Config/Network/Cluster/Auth but has no typed error for `Recoverable` or `CertPending` — both concrete error states named in the coordinator memo. | `ConfigError`, `ClusterError` |

**Review cadence:** re-check on every audit sweep. If a symmetric
sibling never materialises for ≥6 months and no roadmap item targets
the symbol, downgrade to a delete candidate then.

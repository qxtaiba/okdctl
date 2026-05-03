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

The 2026-04-18 Sprint 1 triage (~40 items across Themes A/B/D/E/F/G) has
drained. Every N/M/U/L/D item from that triage is now Done, Deferred, or
Skipped — see the Appendix ledger at the bottom of this file.

Remaining active work has migrated to the tiered structure under "Deferred
— revisit next quarter" below, which groups findings by audit run rather
than by theme. Recommended pickup order for the live tiers:

1. **Tier 0 (live bugs)** below — anything blocker-severity here jumps
   the queue.
2. Tier F (docs, 2026-04-21 audit) — smallest, mostly minutes-to-hours.
3. Tier E (architectural deferrals, 2026-04-20 audit) — E1–E6, days each.
4. Tier G (full `/audit-all` findings, 2026-04-21) — triage by severity
   (critical → major → minor → suggestion) before picking up.
5. Tier H (full `/audit-all` findings, 2026-04-25) — 226 items
   (3 blocker, 44 major, 100 minor, 79 suggestion). Triage by severity;
   the 3 blockers are all `audit-tests` gaps on the credential / destroy
   path and should land first.

### Tier 0 — live bugs

Bugs found by running the binary, not by an audit. Blocker-severity
items here gate the next release.

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

**Status:** done — 2026-04-22 (commit 73912b5)
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

**Status:** done — PR #113
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

**Status:** done — docs commit d30866a
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

**Status:** done — docs commit d30866a
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

**Status:** done — docs commit d30866a
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

**Status:** done — PR #118
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

**Status:** done — PR #119
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

**Status:** not started (first attempt closed; second attempt PR #142 closed by maintainer call — see postmortem)
**Audit:** `sec:27088eab:ssh-accept-new-proxmox`,
`sec:eb479d86:scp-accept-new-proxmox`
**Evidence:** `internal/distribution/okd/phase/ssh.go:27`,
`internal/distribution/okd/setup/upload.go:42`
**Problem:** `SSHRun` and `uploadISOsViaSCP` use
`StrictHostKeyChecking=accept-new` — TOFU. A MITM on the first handshake
pins the attacker as root@proxmox forever.
**Scope:** Add `provider.proxmox.ssh_host_fingerprint` config field
accepting the standard `SHA256:<base64>` format from `ssh-keygen -lf` /
Proxmox UI / `ssh-keyscan host | ssh-keygen -lf -`. Implementation must
compute per-key fingerprints via `golang.org/x/crypto/ssh.FingerprintSHA256`
on parsed keys from `ssh-keyscan` output (NOT a single SHA256 over the
raw stdout — see postmortem). When the pinned value matches any one of
the host's advertised keys, accept; otherwise refuse. When unset, log
the observed fingerprints at WARN so the operator can pin one. Pass
the matched key to ssh via a temp `known_hosts` file with
`-o UserKnownHostsFile=<file> -o StrictHostKeyChecking=yes`. Unit tests
must cover a fixed keyscan-output string so non-determinism regressions
get caught.
**Effort:** hours.
**First attempt (PR #117, closed 2026-04-22):** Hashed the entire
`ssh-keyscan -H -T 5 <host>` stdout with SHA256. Two blockers: (1) `-H`
uses a random salt per invocation, so the computed value changes every
run and every deploy-after-first aborts; (2) even without `-H`, the
banner comment line position and occasional key re-ordering make the
whole-stdout hash flaky. Plus the computed value matched nothing a user
could produce via standard SSH tooling. See PR #117 review for detail.
**Second attempt (PR #142, closed 2026-04-26):** Implementation passed
independent review — `internal/sshpin` package with per-key
`ssh.FingerprintSHA256`, fixture-based tests, refuse-on-no-match. Closed
by maintainer call (not a technical regression); recoverable from the
PR #142 diff if the team revisits.

#### E5 — Flux SSH known-hosts fingerprint pinning

**Status:** not started (first attempt closed; second attempt PR #142 closed by maintainer call — see E4 postmortem)
**Audit:** `sec:98723e5d:ssh-keyscan-tofu`
**Evidence:** `internal/addon/catalog/flux/flux.go:329`
**Problem:** `createDeployKeySecret` runs `ssh-keyscan <host>` and
stuffs the raw output verbatim into the Flux deploy-key Secret. A DNS
poisoner at install time pins themselves as the git host forever,
enabling silent GitOps code substitution.
**Scope:** Add `addons.flux.settings.known_hosts_sha256` accepting the
standard `SHA256:<base64>` format (same vocabulary as E4). Parse each
line of `ssh-keyscan` output, compute per-key fingerprints via
`golang.org/x/crypto/ssh.FingerprintSHA256`, match against the pin.
Without the config, require `addons.flux.settings.accept_host_key=true`
and log the observed fingerprints so the operator can pin one. The
known_hosts bytes written into the Flux Secret must be the key-only
lines (filter `#` comments) so flux's sync is not sensitive to
keyscan's banner-line ordering. Unit tests with a fixed keyscan string
are required.
**Effort:** hours.
**First attempt (PR #117, closed 2026-04-22):** Same whole-stdout
SHA256 approach as E4. Non-deterministic in the wild because
`ssh-keyscan` output interleaves a `# host SSH-2.0-…` banner whose
position shifts between runs, and occasional key-line re-ordering. The
helper would work ~75% of the time and abort the remaining 25% with a
spurious mismatch.

#### E6 — kube-vip probe TLS: use cluster CA once available

**Status:** done — PR #124 (moved to Completed)

### Tier G — findings from 2026-04-21 /audit-all run

Filed by the orchestrator aggregation so `/roadmap-pickup` can fan them out when bandwidth opens. Each references the audit finding ID for diff tracking; when a finding recurs in a later run, its entry Status+Evidence updates here rather than being duplicated.


#### audit-api-design

##### `api:fde34e0c:opt-kubeconfig-env-binding` — opt kubeconfig env binding

**Status:** not planned (see caveat)  
**Severity:** minor  
**Evidence:** `internal/cluster/k8s.go:52-81`  
**Problem:** cluster.NewK8sClient reads KUBECONFIG from os.Getenv at construction time, then builds the cmd runner. This couples the constructor to process env state at call time and means an Exec.Env mutation elsewhere (install.Phase.SetupKubeconfig appends KUBECONFIG= to Exec.Env) is NOT seen by a later-constructed K8sClient.  
**Fix:** Move the os.Getenv('KUBECONFIG') read inside c.run — evaluate the env lazily on each exec.Run so a mid-process KUBECONFIG mutation IS seen. Alternatively drop the env fallback and require WithKubeconfig explicitly.  
**Effort:** hours

##### `api:262af6e4:zero-value-usable-cleanup` — zero value usable cleanup

**Status:** done — PR #115  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:50-107`  
**Problem:** cleanup.Execute takes *Options whose zero Kind yields a bare '*errtypes.ConfigError{Msg: "unknown cleanup type: ..."}' with no sentinel callers can match. An Options{} zero-value (no Logger, no Kind) also silently defaults to NopLogger.  
**Fix:** Add an exported `var ErrKindNotSet = errors.New(...)` sentinel and have Execute return it wrapped when opts.Kind == ''. Alternatively, switch to `NewOptions(kind Kind) *Options` so the required field is a constructor parameter, mirroring destroy.NewOptions / install.NewOptions.  
**Effort:** hours

##### `api:125729c4:opt-inconsistent-cfg-opts` — opt inconsistent cfg opts

**Status:** done — PR #115  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/destroy/phase.go:40-50`  
**Problem:** Phase NewOptions factory shapes still diverge across siblings. setup.DefaultOptions(projectRoot) takes ONLY projectRoot; install.NewOptions, postinstall.NewOptions, destroy.NewOptions take (cfg, projectRoot).  
**Fix:** Rename setup.DefaultOptions to setup.NewOptions(cfg, projectRoot) and fold any cfg-driven defaults into it. Matches the (cfg, projectRoot) signature the other three phase packages share.  
**Effort:** hours

##### `api:c287d5c0:withenv-order-coupling` — withenv order coupling

**Status:** done — PR #115  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/okd.go:61-98`  
**Problem:** okd.WithEnv still encodes an order-dependency contract in New: WithEnv may construct the executor before WithLogger runs, and New compensates after the loop by re-applying WithLogger to the now-existing executor. Option functions should be commutative; any future lazy-building option (e.g.  
**Fix:** Defer executor construction until after the option loop: WithEnv stores pendingEnv []string on *Provisioner, then New builds p.executor = executor.New(executor.WithLogger(p.logger), executor.WithEnv(p.pendingEnv)) once. Options stay pure setters; the constructor owns ordering.  
**Effort:** hours

##### `api:4c092fce:opt-inconsistent-terraform-ctors` — opt inconsistent terraform ctors

**Status:** done — PR #115  
**Severity:** minor  
**Evidence:** `internal/infrastructure/terraform/terraform.go:109-136`  
**Problem:** terraform package still exports two constructors — New(workDir, opts...) and NewWithVarFile(workDir, varFile, opts...) — that differ only in one default. The second is a thin wrapper solely to preset VarFile.  
**Fix:** Add `func WithVarFile(path string) Option` that sets e.VarFile. Delete NewWithVarFile.  
**Effort:** hours

##### `api:830d4653:export-no-caller-installed-lists` — export no caller installed lists (scaffolding — verify intent only)

**Status:** done — PR #115  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/cleanup/packages.go:34-53`  
**Problem:** cleanup.InstalledPackages and cleanup.InstalledBinaries are exported but their only callers are the package-private Packages() function at line 66 and 72. No external caller in-tree.  
**Fix:** Verify intent — if a preview/plan CLI verb is planned, keep and document. Otherwise unexport (installedPackages, installedBinaries).  
**Effort:** hours

##### `api:ed55ee90:export-no-caller-generate-summary` — export no caller generate summary

**Status:** done — PR #115  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/cleanup/summary.go:11-57`  
**Problem:** cleanup.GenerateSummary and cleanup.Summary struct are exported but the only caller is the package-private printSummary(). No external caller.  
**Fix:** Either unexport (GenerateSummary -> generateSummary, Summary -> summary) if no CLI surface planned, or keep both exported and add a one-line doc-comment pointing to the intended caller (e.g. 'used by okdctl cleanup status; see roadmap.md').  
**Effort:** hours

##### `api:d7ce9d16:export-no-caller-dns-config-helpers` — export no caller dns config helpers

**Status:** done — PR #115  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/dns/dns.go:23-128`  
**Problem:** dns.BuildConfigData, dns.ConfigName, and dns.WriteDnsmasqConfig remain exported with callers only inside the dns package. dns.GenerateBootstrapConfig has a single external caller in setup/steps.go:368.  
**Fix:** Unexport: buildConfigData, configName, writeDnsmasqConfig. Keep GenerateBootstrapConfig, DeployBootstrap, DeployProduction, Setup, RestoreSystemResolver as the package's external API.  
**Effort:** hours

##### `api:de572c63:ctx-not-first-write-dnsmasq` — ctx not first write dnsmasq

**Status:** deferred  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/dns/dnsmasq.go:54-92`  
**Problem:** WriteDnsmasqConfig now takes ctx and checks ctx.Err() at entry (progress from prior run), but still does not thread ctx into os.MkdirAll / system.WriteTempFile / system.CopyFile — the body advertises cancellation only via the entry-gate, not per-step. Either plumb ctx into the underlying helpers or select on ctx.Done between the steps.  
**Fix:** Add `select { case <-ctx.Done(): return ctx.Err(); default: }` between the mkdir / WriteTempFile / CopyFile steps so a mid-op cancellation is honored. Alternatively accept the entry-check as sufficient and add a one-line comment explaining why later operations are not gated.  
**Effort:** hours

##### `api:ae5b624c:concrete-return-k8s` — concrete return k8s

**Status:** done — PR #115  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/install/monitor.go:54-63`  
**Problem:** K8sClient is used in monitor.go only for ApprovePendingCSRs. Rather than accepting a concrete *cluster.K8sClient in MonitorInstallation, the caller could define a tiny consumer-side interface `type csrApprover interface { ApprovePendingCSRs(context.Context) (int, error) }` at the install package.  
**Fix:** Inside install package define `type csrApprover interface { ApprovePendingCSRs(ctx context.Context) (int, error) }`. Accept it as a parameter to MonitorInstallation, defaulting to NewK8sClient(...).  
**Effort:** hours

##### `api:73ad30ef:export-no-caller-external-tool-binaries` — export no caller external tool binaries

**Status:** done — PR #115  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/phase/paths.go:96-105`  
**Problem:** phase.ExternalToolBinaries has one in-tree caller (cleanup/packages.go:52). Exported for the sole purpose of avoiding a setup→cleanup import.  
**Fix:** Move ExternalToolBinaries to a new phase/tools.go with a one-line doc clarifying 'these binaries are installed by setup; cleanup removes them.' No callsite change.  
**Effort:** hours

##### `api:dd75bdeb:stutter-postinstall-context` — stutter postinstall context

**Status:** deferred  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/postinstall/context.go:1-10`  
**Problem:** postinstall.PostInstallContext stutters (package.PostInstall…). The struct is already suppressed with //nolint:revive and a 'rename deferred to a dedicated refactor' note, so this finding is a reminder that the deferred rename is still pending.  
**Fix:** Rename postinstall.PostInstallContext -> postinstall.State (preferred) or postinstall.Context. Callers: phase.go:76 (distribution.NewPhaseContext(State{})), steps.go (4x pctx.Update(func(c *State) {...})), and the PhaseContext[State] type parameter.  
**Effort:** hours

##### `api:761e5126:export-no-caller-removehaproxy` — export no caller removehaproxy (scaffolding — verify intent only)

**Status:** deferred  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/postinstall/haproxy.go:23-97`  
**Problem:** postinstall.Phase.RemoveHAProxy is exported but the only caller is the package-private finalizeIngress path in update_ingress.go:214. No external consumer references it.  
**Fix:** Verify intent against roadmap.md. If a standalone `okdctl haproxy remove` verb is planned, keep exported and add a one-line doc referencing it.  
**Effort:** hours

##### `api:beabab0c:mix-default-new-naming` — mix default new naming

**Status:** done — PR #115  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/setup/phase.go:34-42`  
**Problem:** setup.DefaultOptions continues the Default* naming pattern common for 'zero-arg constructor of a defaulted options struct'. Other phase packages use NewOptions(cfg, projectRoot).  
**Fix:** Rename setup.DefaultOptions -> setup.NewOptions; accept (cfg, projectRoot) and fold any cfg-driven defaults inside. Single call site in okd.go updates.  
**Effort:** hours

##### `api:4f69fc9d:iface-fragmented-step` — iface fragmented step

**Status:** deferred  
**Severity:** suggestion  
**Evidence:** `internal/distribution/step.go:31-69`  
**Problem:** Step / Skipper / FatalChecker / StepCallbacks remain four interfaces that ProvisioningStep always composes together. The builtStep impl implements all four.  
**Fix:** Keep Step (the 'id+name+execute' core could plausibly stand alone). Inline Skipper, FatalChecker, StepCallbacks into ProvisioningStep directly.  
**Effort:** hours

##### `api:48688e63:iface-in-consumer` — iface in consumer

**Status:** deferred  
**Severity:** suggestion  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:33-101`  
**Problem:** Provider struct still has public methods (Connect, Disconnect, Provision, PlanOnly) but no consumer-side interface. cli/helpers.go and install/phase.go take the concrete *okd.Provisioner.  
**Fix:** If/when a second provider lands, define InfrastructureProvider interface in the distribution package (the consumer). Proxmox implements it structurally — no code change in proxmox/.  
**Effort:** hours


#### audit-cli-ux

##### `ux:024a2c32:json-schema-doc-drift` — json schema doc drift

**Status:** done — docs commit d30866a — docs/cli/json-schema.md rewritten to match actual marshaled shapes; golden-test deferral tracked via audit-tests gap entry  
**Severity:** major  
**Evidence:** `docs/cli/json-schema.md:12-67`  
**Problem:** docs/cli/json-schema.md documents field shapes that do not match what the code emits. `okdctl status --format=json` is documented with cluster_name/version/ready_nodes/total_nodes but emits api_reachable/nodes/degraded_operators/addons.  
**Fix:** Option (a, preferred): update docs/cli/json-schema.md to match code — for status enumerate api_reachable, nodes[{name,role,ready}], degraded_operators, addons[{name,healthy,error?}]; for releases list enumerate the flat OKDVersion array including release_type (int, document the 0-4 encoding or switch code to emit the string label). Add fixture-based golden tests (status_test.go, releases_test.go) that compare marshaled output against the doc so future drift fails CI.  
**Effort:** hours

##### `ux:54654337:readme-flag-drift` — readme flag drift

**Status:** done — docs commit d30866a — `make docs` regenerated docs/cli/okdctl_destroy.md with the three skip-* flags  
**Severity:** minor  
**Evidence:** `docs/cli/okdctl_destroy.md:26-33`  
**Problem:** Generated CLI reference for `okdctl destroy` is stale: commit afsd79b added --skip-terraform, --skip-cleanup, --skip-firewall to destroy.go, but docs/cli/okdctl_destroy.md still lists only --confirm-cluster, --dry-run, -h/--help, --keep-isos, -y/--yes. CI's docs-drift check (.github/workflows/ci.yml: `git diff --quiet docs/cli/`) would fail on this state.  
**Fix:** Run `make docs` (or `go run ./cmd/okdctl-gen-docs`) and commit docs/cli/. The regenerator is already in CI (.github/workflows/ci.yml:65); the drift is pre-commit residue from the afsd79b work.  
**Effort:** hours

##### `ux:073d24ed:dry-run-yes-short-circuit` — dry run yes short circuit

**Status:** done — PR #108  
**Severity:** minor  
**Evidence:** `internal/cli/deploy.go:78-80`  
**Problem:** runDeploy checks deployYes before deployDryRun and returns after saving the config, so `okdctl deploy --yes --dry-run` silently skips the dry-run preview the user asked for. --yes is documented as 'skip prompts, use defaults' and --dry-run as 'preview terraform plan and step listing without deploying' — the combination should still preview, not no-op into a config save.  
**Fix:** Reorder the guard: if deployDryRun { return runDeployDryRun(ctx, cfg) } BEFORE the deployYes short-circuit, or gate the --yes fast-path on !deployDryRun. Matches runDestroy (destroy.go:71-73) which checks destroyDryRun first.  
**Effort:** hours

##### `ux:d31d1b9d:json-key-hyphenated` — json key hyphenated

**Status:** done — PR #105  
**Severity:** minor  
**Evidence:** `internal/cli/status.go:338-353`  
**Problem:** runDescribeAddon emits JSON with a hyphen-cased key `display-name` while every other field in the same payload and every other JSON endpoint uses snake_case (api_reachable, ready_nodes, degraded_operators, release_date, release_type). jq consumers have to quote the field: `jq '."display-name"'`, which is a pain-point.  
**Fix:** Rename the JSON key to display_name in the lines slice when describeFormat == outputJSON. Text mode can keep the hyphenated label (it is human-facing and reads as a single phrase).  
**Effort:** hours

##### `ux:e45c2239:sig-not-handled-preflight` — sig not handled preflight

**Status:** done — PR #108  
**Severity:** suggestion  
**Evidence:** `cmd/okdctl/main.go:20-23`  
**Problem:** main() calls preflight() before cli.Execute(); signal.Notify setup lives inside internal/cli/root.go:execute(). If the user hits Ctrl-C during preflight's euid check, OKDCTL_BIN_DIR validation, or PATH mutation, the process dies with SIGINT default (no partial summary, undocumented behavior).  
**Fix:** Either (a) accept current behavior (document in main's package comment: 'preflight runs before signal setup; it is small enough that interruption is racy but not harmful') or (b) move signal.Notify earlier into main() and pass ctx through preflight. Only pay for (b) if preflight grows (e.g.  
**Effort:** hours

##### `ux:93957c53:cleanup-no-dry-run` — cleanup no dry run

**Status:** done — PR #108  
**Severity:** suggestion  
**Evidence:** `internal/cli/cleanup.go:18-34`  
**Problem:** cleanupCmd has no --dry-run flag while its destructive siblings (deploy, destroy, update-ingress) all do. cleanup removes packages, dnsmasq/haproxy configs, ignition files, and terraform state — destructive enough that a preview flag has the same value proposition.  
**Fix:** Add cleanupDryRun bool and --dry-run flag. Branch to a runCleanupDryRun before the confirmation prompt; enumerate what would be removed (work directory path, haproxy config block, dnsmasq drop-in path, packages to uninstall) via the existing cleanup.Options struct — do not call cleanup.Execute.  
**Effort:** hours

##### `ux:8d8faa80:completion-use-bracket-optional` — completion use bracket optional

**Status:** done — PR #108  
**Severity:** suggestion  
**Evidence:** `internal/cli/completion.go:11-11`  
**Problem:** completionCmd.Use is 'completion [bash|zsh|fish|powershell]' — square brackets per man(1) convention mean optional, but cobra.ExactArgs(1) rejects zero-arg. The shell token is required; Use should render `<bash|zsh|fish|powershell>`.  
**Fix:** Change Use to 'completion <bash|zsh|fish|powershell>'. Same pattern as internal/cli/addon.go:67 ('uninstall <name>') and releases.go:53 ('show <version>').  
**Effort:** hours

##### `ux:e7db1220:releases-show-no-completion` — releases show no completion

**Status:** done — PR #108  
**Severity:** suggestion  
**Evidence:** `internal/cli/releases.go:52-59`  
**Problem:** addon install/uninstall and describe-addon gained ValidArgsFunction for tab-completion; releasesShowCmd still has none. Tab-completing `okdctl releases show <TAB>` does filesystem completion instead of version suggestions.  
**Fix:** Add ValidArgsFunction that reads the disk cache (releases.NewOKDVersionFetcher has a cache-backed path) and returns Versions + ShellCompDirectiveNoFileComp. Fall through to ShellCompDirectiveError when the cache is empty rather than fetching on tab — keeps completion latency under the 1s shell threshold.  
**Effort:** hours

##### `ux:aa84670c:exit-code-bsd-sysexits-partial` — exit code bsd sysexits partial

**Status:** done — PR #108  
**Severity:** suggestion  
**Evidence:** `internal/cli/root.go:144-162`  
**Problem:** exitCodeFor maps ConfigError=2 (not EX_DATAERR=65 or EX_CONFIG=78), NetworkError=3 (not EX_UNAVAILABLE=69), ClusterError=4 (not EX_UNAVAILABLE=69), AuthError=5 (not EX_NOPERM=77). The taxonomy IS published at the package doc (root.go:1-8).  
**Fix:** Keep the current mapping for backward compatibility (scripts may pin 2/3/4/5); add a regression test in root_test.go asserting each typed error reaches the right code, and --version / help exit 0. Optionally introduce --exit-code-mode={compat|sysexits} for opt-in BSD mapping.  
**Effort:** hours


#### audit-code-smells

##### `smell:daf5bee9:yaml-tree-walk-repeat-assertion` — yaml tree walk repeat assertion

**Status:** done — PR #110  
**Severity:** suggestion  
**Evidence:** `internal/cli/kubeconfig.go:141-168`  
**Problem:** mergeNamedList has four nested type-assertion chains to walk a generic YAML tree (any → []any → map[string]any → map[string]any['name'] → string). Function works and the any is load-bearing (YAML unmarshal targets `any` for open schemas), but the walk is tightly coupled to one semantic (merge-by-name) so the any-ness doesn't buy reuse.  
**Fix:** Either (a) declare a minimal kubeconfig schema (ClustersList, UsersList, ContextsList) and yaml-unmarshal into typed slices, then merge; or (b) extract a `namedEntries(v any) map[string]any` helper so the tree walk lives in one place. (a) is the clean fix but adds types the package doesn't need elsewhere; (b) preserves the any-based approach but shrinks the walk to one site.  
**Effort:** hours

##### `smell:004ad79b:helper-pkg-thin-wrap` — helper pkg thin wrap

**Status:** done — PR #110  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/packages/packages.go:1-42`  
**Problem:** Package `packages` wraps `platform.PackageManager.Install`/`Remove` with an extra logger.Info() envelope and a single `fmt.Errorf` rewrap. Three call sites consume it (setup/steps.go:305, cleanup/packages.go:67, cleanup/services.go:41).  
**Fix:** Inline the two functions at their call sites (setup/steps.go:305, cleanup/packages.go:67, cleanup/services.go:41) and delete internal/distribution/okd/packages. The logger.Info lines are already repeated in the callers' surrounding context.  
**Effort:** hours

##### `smell:1d5afa08:enum-via-sscanf-int-parse` — enum via sscanf int parse

**Status:** done — PR #110  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/releases/types.go:59-94`  
**Problem:** OKDVersion.Major() and OKDVersion.Minor() parse the Version string via fmt.Sscanf on every call. ShortVersion calls both (two parses per call), and callers invoke the methods inside filter loops (fetcher.go parseReleases and sortAndClassifySeries, cli/releases.go), so a list of ~40 releases runs dozens of Sscanf parses per display even though the version string is immutable per OKDVersion.  
**Fix:** Parse once at unmarshal time (or memoize). Either (a) add unexported `major, minor int` fields and populate them in the fetcher's parseVersionTag flow (fetcher.go:241 already runs Sscanf — fold the result into the struct), or (b) use `strings.Cut(v.Version, ".")` + strconv.Atoi, which is faster and avoids the fmt machinery.  
**Effort:** hours

##### `smell:c5e5c304:build-role-helper-near-duplicate` — build role helper near duplicate

**Status:** done — PR #110  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/setup/terraform.go:20-34`  
**Problem:** buildISOStrings and buildNodeNames in setup/terraform.go are structurally identical: allocate []string of length count, loop `for i := range count`, format `"%s:iso/%s%d.iso"` vs `"%s-%s%d"`. Both take (isoStorage/clusterName, phase.NodeRole, count).  
**Fix:** Introduce `buildQuotedRoleList(format string, prefix string, role phase.NodeRole, count int) []string` that takes a format string with two %s + one %d and renders count elements. Both sites collapse to one-liners.  
**Effort:** hours

##### `smell:c5e5c304:named-return-unnecessary` — named return unnecessary

**Status:** done — PR #110  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/setup/terraform.go:36-48`  
**Problem:** getDiskSizes returns `(cpDisk, workerDisk, workerDataDisk, masterDataDisk int)` — four unnamed integers with no semantic ordering. The named returns document the positional identity, but the real signal a caller needs is 'which is which'.  
**Fix:** Replace the 4-int tuple with a `type diskSizes struct { cpOS, workerOS, workerData, masterData int }` and return the struct. The single caller (buildTerraformVarsData) then assigns by field name, not by position.  
**Effort:** hours

##### `smell:4f69fc9d:stepbuilder-build-no-callers` — stepbuilder build no callers (scaffolding — verify intent only)

**Status:** done — scaffolding retained per MEMORY.md  
**Severity:** suggestion  
**Evidence:** `internal/distribution/step.go:155-173`  
**Problem:** distribution.StepBuilder.Build() has no external callers; every production path goes through BuildSteps → MustBuild, and MustBuild is Build's only caller. Build's stated value is 'returns an error only when b is nil' but NewStepBuilder is the only way to get a *StepBuilder and it never returns nil, so the error is unreachable.  
**Fix:** Keep. Build/MustBuild is the canonical pair for fluent builders in Go (errors.New + errors.Must, template.New + template.Must, etc.), and CLAUDE.md §architecture-notes names StepDef + BuildSteps as canonical.  
**Effort:** hours

##### `smell:0934cf1b:query-match-mini-dsl` — query match mini dsl

**Status:** done — PR #110  
**Severity:** suggestion  
**Evidence:** `internal/platform/packages.go:100-115`  
**Problem:** Manager.IsInstalled uses a bespoke `queryMatch` string substring to distinguish "installed" from "purged" on dpkg output. The logic is correct but the two-branch design (empty → exit-code-only, non-empty → substring match) is a mini-DSL inside a single method.  
**Fix:** Replace the `queryMatch` string field with `postCheck func(stdout []byte, pkg string) bool` on Manager. Set it to a no-op in the RHEL constructor and a dpkg-ii-prefix check in the Debian constructor.  
**Effort:** hours


#### audit-concurrency

##### `con:39c75e91:go-no-wait` — go no wait

**Status:** done — acceptance note (CLAUDE.md §Concurrency)  
**Severity:** suggestion  
**Evidence:** `internal/cli/confirm.go:22-45`  
**Problem:** promptForConfirmation spawns a reader goroutine that blocks on bufio.Reader.ReadString, races against ctx.Done, and on ctx cancel the goroutine remains blocked on Stdin.Read until the user presses enter or the process exits. Thoroughly documented in the function header; the capacity-1 inputCh means the goroutine's eventual send never deadlocks — but this is still an unowned goroutine whose lifetime is bounded only by the parent process.  
**Fix:** Go 1.25 has no portable cross-platform stdin cancellation; the current design is the least-bad option for a CLI prompt. CLAUDE.md §Concurrency already names "documented leak bound" as an accepted exception — this site satisfies it.  
**Effort:** hours

##### `con:484b40f0:lock-held-during-write` — lock held during write

**Status:** done — PR #109  
**Severity:** suggestion  
**Evidence:** `internal/deploymetrics/metrics.go:75-84`  
**Problem:** Handler holds r.mu.Lock() across fmt.Fprint(w, b.String()) — writing to an http.ResponseWriter under the mutex. A slow Prometheus scraper or stalled network connection blocks every StepStarted/StepFinished call in the deploy path until the write completes, coupling scrape latency to deploy latency.  
**Fix:** Build the rendered metrics string under the lock, release the lock, then write to w: r.mu.Lock(); var b strings.Builder; r.writeMetrics(&b); out := b.String(); r.mu.Unlock(); fmt.Fprint(w, out). The renderer writes to a local Builder so it can't race; the net write happens outside the critical section.  
**Effort:** hours

##### `con:ae5b624c:synctest-opportunity` — synctest opportunity

**Status:** deferred  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/install/monitor.go:52-150`  
**Problem:** MonitorInstallation has a ticker-driven CSR-approval loop, a reap timer, and ctx.Done/DeadlineExceeded paths — exactly the shape testing/synctest is designed for. Currently untested because real-time tests would take minutes; the exec_test.go suite landing in this release proved the pattern works, so this is the last holdout.  
**Fix:** Extract the select-loop body into a testable helper (csrApprovalLoop(ctx, ticker, installDone, k8sClient)) and cover the three exit paths (installDone success, installDone timeout-error, ctx cancel → kill → reap) with testing/synctest — mirror the internal/system/exec_test.go shape that landed this release. Requires a k8sClient fake; audit-tests already flags that fake as missing for CSR-related coverage.  
**Effort:** hours

##### `con:ae5b624c:go-leak-on-error` — go leak on error

**Status:** done — acceptance note (CLAUDE.md §Concurrency)  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/install/monitor.go:65-150`  
**Problem:** MonitorInstallation spawns a goroutine holding installCmd.Wait(). On ctx cancel the function calls killInstall, waits up to 30s via reapTimer for Wait() to return, then abandons — leaving the goroutine still blocked on the (now-killed) process's Wait() until the OS reaps it.  
**Fix:** Pattern is sound and CLAUDE.md §Concurrency now names it as the canonical cmd.Wait reap-with-deadline example. Optional improvement: promote the reap-with-deadline shape to a shared helper in internal/distribution/okd/phase/ when a second caller (e.g.  
**Effort:** hours

##### `con:8e65d574:go-no-wait` — go no wait

**Status:** done — acceptance note (CLAUDE.md §Concurrency)  
**Severity:** suggestion  
**Evidence:** `internal/version/updatecheck.go:40-53`  
**Problem:** BackgroundCheck spawns a fire-and-forget goroutine that runs runCheck(ctx); printUpdateNotice in cli/root.go waits at most 100ms before returning, so on the happy path the goroutine races to completion and on the slow path it leaks until the process exits. CLAUDE.md §Concurrency now names this as the canonical fire-and-forget example, so the pattern is fully grounded — kept as a long-term advisory that future cross-references should pin line numbers rather than re-raise.  
**Fix:** No code change required — CLAUDE.md §Concurrency now pins this site as the canonical example. Optional future improvement: expose a Done() <-chan struct{} so an integration test can synchronously wait on BackgroundCheck to finish (currently testable only via cache-populate workarounds).  
**Effort:** hours


#### audit-dependencies

##### `dep:33ef32bf:yaml-quad-engines` — yaml quad engines

**Status:** done — acceptance note (CLAUDE.md §Dependencies)  
**Severity:** minor  
**Evidence:** `go.mod:20-60`  
**Problem:** Four YAML engines in the tree: sigs.k8s.io/yaml (direct), go.yaml.in/yaml/v2 (via k8s), go.yaml.in/yaml/v3 (via cobra/doc + kube-openapi), gopkg.in/yaml.v3 (via go-proxmox + testify + charm/log). Binary ships N engines even though only sigs.k8s.io/yaml is directly imported.  
**Fix:** Document the split in CLAUDE.md §Dependencies: sigs.k8s.io/yaml is REQUIRED for k8s Secret marshaling (JSON-tag respect); the three transitive engines are pulled by upstream deps we can't control (k8s, cobra, testify). No action on the code; action is on documentation so future PRs don't accidentally try to 'consolidate' and break the k8s addon path.  
**Effort:** hours

##### `dep:33ef32bf:ultraviolet-pseudo-version` — ultraviolet pseudo version

**Status:** done — acceptance note (charm ecosystem convention)  
**Severity:** minor  
**Evidence:** `go.mod:27-27`  
**Problem:** github.com/charmbracelet/ultraviolet is pinned to a pseudo-version (commit SHA, not a tagged release) — the project has never cut a tag. Pulled at three different pseudo-versions by charm.land/bubbles, lipgloss+log, and bubbletea; MVS picks the newest.  
**Fix:** Acceptance note. Charm ecosystem convention is that ultraviolet (the internal renderer) is not publicly-tagged.  
**Effort:** hours

##### `dep:b803fcb7:workflow-pin-hygiene-clean` — workflow pin hygiene clean

**Status:** done — acceptance note (tripwire)  
**Severity:** suggestion  
**Evidence:** `.github/workflows/ci.yml:1-119`  
**Problem:** Pin hygiene audit: every GitHub Action in .github/workflows/ is pinned by full 40-char SHA with the version tag in a trailing comment (actions/checkout, setup-go, golangci-lint-action, codeql-action, goreleaser-action, cosign-installer, sbom-action, setup-terraform, label-sync, labeler, slsa-github-generator, shellcheck, setup-tflint, attest-build-provenance). Go-install tools pinned by exact version (govulncheck v1.1.4, yamlfmt v0.14.0, terraform 1.10.3, golangci-lint v2.11.4).  
**Fix:** Optional tripwire: add a CI guard that fails if any workflow introduces a non-SHA action ref. Example — a lightweight rule in a new lint job running a regex over .github/workflows/ that flags `uses: org/name@tag` (where tag is not 40 hex).  
**Effort:** hours

##### `dep:87db21a9:goreleaser-action-version-tag` — goreleaser action version tag

**Status:** done — acceptance note (cosign trust model)  
**Severity:** suggestion  
**Evidence:** `.github/workflows/release.yml:25-29`  
**Problem:** goreleaser-action is SHA-pinned (good), but the version parameter it resolves IS a tag, not a SHA — version: v2.15.2 in both release.yml and release-prep.yml. This is the goreleaser CLI binary version, not the GH Action.  
**Fix:** Minor tightening: if goreleaser publishes binary SHA256s (it does, as part of its own release process), add a post-install `sha256sum goreleaser` step and match against a pinned hash. Alternatively, accept the v2.15.2 tag trust model — goreleaser signs its own releases with cosign.  
**Effort:** hours

##### `dep:33ef32bf:copyleft-audit-clean` — copyleft audit clean

**Status:** done — acceptance note (CLAUDE.md §Dependencies)  
**Severity:** suggestion  
**Evidence:** `go.mod:1-72`  
**Problem:** License compatibility audit: NO copyleft (GPL/AGPL/LGPL) or custom/unclear licenses in the transitive dep tree. All direct and indirect deps carry permissive licenses (MIT / Apache-2.0 / BSD-3).  
**Fix:** CLAUDE.md §Dependencies already codifies the MIT/Apache/BSD-only policy as of 2026-04-19. This row stays as a tripwire reference so future PR reviewers see the baseline; no code change needed.  
**Effort:** hours

##### `dep:33ef32bf:go-yaml-in-fork-risk` — go yaml in fork risk

**Status:** done — acceptance note  
**Severity:** suggestion  
**Evidence:** `go.mod:58-59`  
**Problem:** go.yaml.in/yaml/v2 and go.yaml.in/yaml/v3 are a vanity-domain fork of the original gopkg.in/yaml.v{2,3}. The domain (go.yaml.in) is a 2024+ rehosting that the k8s/cobra ecosystems migrated to after gopkg.in archived yaml.v2.  
**Fix:** Acceptance note only. The go.yaml.in move is the same maintainer collective as gopkg.in (kubernetes-sigs).  
**Effort:** hours

##### `dep:33ef32bf:golang-x-exp-stale` — golang x exp stale

**Status:** done — acceptance note (transitive upstream)  
**Severity:** suggestion  
**Evidence:** `go.mod:60-60`  
**Problem:** golang.org/x/exp pinned at v0.0.0-20231006140011 (Oct 2023) — almost 2.5 years old. Pulled transitively by charm.land/log/v2, which only imports golang.org/x/exp/slog (a BACKPORT of log/slog that the stdlib now provides since Go 1.21 — and this repo targets 1.25 per go.mod).  
**Fix:** File upstream issue at github.com/charmbracelet/log requesting a drop of the x/exp/slog import in favor of stdlib log/slog. Until that lands, the stale pin persists.  
**Effort:** hours


#### audit-documentation

##### `doc:a55b4592:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/config/loader.go:15-15`  
**Problem:** NewLoader (line 15) lost its doc. Zero-arg constructor returning a pointer — the trivial signature masks the fact that the Loader has lifecycle state (caching, YAML defaults).  
**Fix:** Restore: '// NewLoader returns a Loader suitable for reading okdctl YAML configs. Loaders cache parsed schemas; reuse one per process to avoid re-parsing defaults.'  
**Effort:** hours

##### `doc:cf43073b:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/config/types.go:7-23`  
**Problem:** 4 exported symbols missing docs: DistributionOKD const (7), ProviderProxmox const (14), SupportedDistributions func (17), SupportedProviders func (23). The const values encode the supported-distributions/providers whitelist — semantic meaning is implicit.  
**Fix:** Restore group docs on each const block and one-line docs on SupportedDistributions / SupportedProviders. Example: '// Distributions okdctl can deploy.' before the DistributionOKD block covers revive's exported rule for the whole group.  
**Effort:** hours

##### `doc:297adb3e:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/config/validation_types.go:41-132`  
**Problem:** 5 exported validation types/methods missing docs: ValidationResult.IsValid (41), ValidationResult.AddError (45), ScopeRequired const (66), ValidationScope.HasScope (81), ValidateWithOptions (132). ScopeRequired is a bitflag const — semantic meaning is NOT evident from the name.  
**Fix:** Restore one-line contract docs. For bitflag consts, use a group header: '// Validation scope flags.' above the const block covers the whole group per revive's exported rule.  
**Effort:** hours

##### `doc:aa0f50f5:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/config/validators.go:369-481`  
**Problem:** 6 exported validators at lines 369 (IsValidIP), 374 (IsValidCIDR), 421 (ValidateClusterName), 431 (ValidateDomain), 457 (ValidateIP), 481 (ValidateCIDR) lack doc comments. Is- prefixed returns bool, Validate- prefixed returns error — the naming encodes the behavior but CLAUDE.md §code-comments item 2 still requires a contract doc on exported helpers to clarify failure modes.  
**Fix:** Add one-line verb-first docs. Example: '// IsValidIP reports whether s parses as an IPv4 or IPv6 literal.' '// ValidateClusterName returns a descriptive error if value violates the DNS-1123 cluster-name grammar.'  
**Effort:** hours

##### `doc:125729c4:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/destroy/phase.go:56-56`  
**Problem:** The New constructor (line 56) on destroy.Phase lost its doc in the hygiene pass. revive:exported will fail CI.  
**Fix:** Restore: '// New constructs a destroy.Phase bound to cfg. The Phase is safe to call multiple times — each step idempotently skips if its resource is absent.'  
**Effort:** hours

##### `doc:d5915b0c:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/install/phase.go:80-84`  
**Problem:** 2 exported symbols missing docs: Phase type (80), New func (84). Phase is part of the canonical per-phase pattern per CLAUDE.md §architecture-notes.  
**Fix:** Restore one-line docs mirroring the surviving cleanup/destroy phase patterns. Example: '// Phase drives the install flow: openshift-install wrapper, bootstrap monitor, cluster-up poll.'  
**Effort:** hours

##### `doc:0139cb3f:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/phase/paths.go:70-132`  
**Problem:** 2 exported symbols missing docs: BinDirOrDefault func (70), BasePhaseOption type (132). BasePhase helpers are canonical per CLAUDE.md §architecture-notes — these are the shared cross-phase APIs.  
**Fix:** Restore docs. Example: '// BinDirOrDefault returns s when non-empty, else the default bin dir (from ResolveBinDir).' '// BasePhaseOption configures a BasePhase at construction time.'  
**Effort:** hours

##### `doc:f99eddfa:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/postinstall/phase.go:26-60`  
**Problem:** 5 exported postinstall symbols missing docs: Options type (26), NewOptions func (35), Result type (48), Phase type (56), New func (60). Phase is the canonical per-phase type pattern — CLAUDE.md §architecture-notes explicitly names this.  
**Fix:** Restore one-line docs. Mirror the surviving setup/phase.go or install/phase.go pattern.  
**Effort:** hours

##### `doc:fb54208a:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/postinstall/steps.go:15-15`  
**Problem:** StepVerifyHealth const (line 15) lost its group-header doc. The const is a StepID — part of the canonical distribution.StepID enum used across the phase-step orchestration.  
**Fix:** Restore a group-level doc on the const block: '// Postinstall StepIDs. These identify the steps in Phase.Run order and appear in distribution.Orchestrator events.'  
**Effort:** hours

##### `doc:632c9087:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:18-39`  
**Problem:** 3 exported symbols missing docs: DefaultIngressLBTimeout const (18), IngressEntry type (31), UpdateIngressResult type (39). DefaultIngressLBTimeout encodes a 10-minute operational value — semantic meaning (why 10 not 5) is not evident from the name.  
**Fix:** Restore docs. For DefaultIngressLBTimeout include the rationale inline: '// DefaultIngressLBTimeout caps how long update-ingress waits for the ingress LB service to report a ready external IP.  
**Effort:** hours

##### `doc:ab9b764a:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/ignition.go:85-85`  
**Problem:** Phase.GenerateManifests (line 85) lost its doc. Manifest generation is an externally-visible step — callers need to know the failure mode.  
**Fix:** Restore: '// GenerateManifests invokes openshift-install to expand install-config.yaml into the full manifest set. Returns wrapped ConfigError for validation failures and wrapped ExecError for binary failures.'  
**Effort:** hours

##### `doc:2f70d7df:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/kargs.go:41-41`  
**Problem:** ExtractNetworkConfig (line 41) lost its doc. The name suggests extraction but the semantics (from which input?  
**Fix:** Restore: '// ExtractNetworkConfig parses the Ignition JSON and returns the first storage-files entry matching the NetworkManager connection path. Returns a typed ConfigError for malformed JSON.'  
**Effort:** hours

##### `doc:beabab0c:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/phase.go:19-111`  
**Problem:** 8 exported symbols in setup.Phase carry no doc comment after the 2026-04-21 comment-hygiene pass: DefaultIgnitionPort const (19), Options type (23), DefaultOptions func (34), CoreOSInfo type (57), NodeInfo type (64), Phase type (71), Phase.Execute method (94), Phase.PrintSetupCompletionSummary method (111). revive:exported enabled in .golangci.yml will fail CI on next push.  
**Fix:** Restore concise verb-first doc comments on each of the 8 sites. For the Phase type, lead with 'Phase drives the setup phase of an OKD install — artifact download, config generation, ignition upload.' Mirror existing docs in sibling install/destroy packages for consistency.  
**Effort:** hours

##### `doc:6fc3d91e:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/platform/platform.go:18-18`  
**Problem:** FamilyRHEL const (line 18) lost its group-header doc. Part of the const block defining platform family identifiers.  
**Fix:** Restore a group-level comment on the const block: '// Platform OS-family identifiers and supported arch literals.' covers revive's exported-block requirement.  
**Effort:** hours

##### `doc:e3782ee7:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/system/fs.go:14-235`  
**Problem:** 4 exported filesystem helpers missing docs: FileExists (14), DirExists (22), EnsureDirForFile (35), AtomicWriteString (235). AtomicWriteString is a wrapper around the canonical AtomicWrite — the wrapper's TOCTOU/fsync semantics should inherit from the underlying contract.  
**Fix:** Restore docs. Example: '// FileExists reports whether path refers to an existing regular file (returns false for directories).' '// AtomicWriteString is a string-typed convenience wrapper around AtomicWrite; the fsync + rename invariants are the same.'  
**Effort:** hours

##### `doc:e2343d2c:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/system/systemd.go:17-17`  
**Problem:** ServiceEnable const (line 17) lost its group-header doc. Part of the ServiceAction enum driving systemctl operations.  
**Fix:** Restore a group-level comment: '// Actions passable to SystemdCtl. Each value maps to a systemctl subcommand.'  
**Effort:** hours

##### `doc:c14fdd9d:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/tui/base_styles.go:9-9`  
**Problem:** TitleStyle var (line 9) lost its doc. Part of the base-styles palette; caller code in wizard steps imports it.  
**Fix:** Restore: '// TitleStyle is the bold Blue400 header style used at the top of each TUI step.'  
**Effort:** hours

##### `doc:588ce79e:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/tui/colors.go:13-18`  
**Problem:** 2 exported color symbols missing docs: ThemeDefault const (13), ColorPurple600 var (18). These are the theme system's public palette — downstream TUI components import them.  
**Fix:** Add group header on ThemeDefault's const block: '// Built-in color themes.' Add: '// ColorPurple600 is the purple-600 palette color used by the default theme.'  
**Effort:** hours

##### `doc:983f67f0:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/tui/layouts.go:11-11`  
**Problem:** DefaultBoxWidth const (line 11) lost its doc. A layout-constant encoding a semantic choice (78 columns?  
**Fix:** Restore: '// DefaultBoxWidth is the content width used by BoxedSection; 78 leaves room for the 1-col lipgloss border on each side in an 80-col terminal.'  
**Effort:** hours

##### `doc:660d83a5:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/tui/logger.go:22-161`  
**Problem:** 5 exported logger helpers missing docs: LF (line 22), Info (68), Warn (70), Error (72), RunID (161). Info/Warn/Error in particular encode contract invariants (stderr redirect + RedactHandler wiring per the kept comments elsewhere in this file).  
**Fix:** Restore one-line docs that reference the stderr + RedactHandler invariant already kept on stderrSlog. Example: '// Info logs at INFO through the redact-handling stderr slog.  
**Effort:** hours

##### `doc:bc9ba9bc:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/tui/rendering.go:3-11`  
**Problem:** 3 exported rendering helpers missing docs: SubsectionLabel (3), CompletionSuccess (7), CompletionError (11). User-facing TUI output helpers — caller code needs to know formatting guarantees.  
**Fix:** Restore one-line docs. Example: '// CompletionSuccess formats msg with the green-check success prefix and the configured base style.'  
**Effort:** hours


#### audit-errors

##### `err:48688e63:typed-err-fallthrough` — typed err fallthrough

**Status:** done — PR #105  
**Severity:** minor  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:181-237`  
**Problem:** Provider.Provision and Provider.retrieveProvisionResult still raise bare fmt.Errorf for config-class / cluster-runtime failures ('no VMs provisioned; check config', 'static IP start address is required for OKD deployments'). The prior sweep fixed Connect (line 85, 88 now use ConfigError); these two adjacent sites were missed.  
**Fix:** Line 182: wrap with &errtypes.ClusterError{Msg: "terraform apply succeeded but no VMs were provisioned; check config"} (cluster-runtime failure, exit 4). Line 237: wrap with &errtypes.ConfigError{Msg: "static IP start address is required for OKD deployments"} (config, exit 2).  
**Effort:** hours

##### `err:40d315ad:wrap-tool-prereq-untyped` — wrap tool prereq untyped

**Status:** done — PR #105  
**Severity:** suggestion  
**Evidence:** `internal/addon/catalog/flux/flux.go:72-72`  
**Problem:** Flux.Install returns a bare fmt.Errorf('helm is required to install Flux') when helm is missing. The message is user-friendly but the error carries no chain or type — it's a tool-prerequisite failure that semantically matches ConfigError (missing external dep is a configuration/environment issue, exit 2).  
**Fix:** For tool-prerequisite errors (line 72) and config errors (:148, :332, :391), wrap with &errtypes.ConfigError{Msg: ...} so the underlying chain carries correct classification. addon/manager.go:installAndVerify should then errors.As for ConfigError before wrapping as ClusterError, preserving the 'tool missing' vs 'install failed' distinction at the outer boundary.  
**Effort:** hours

##### `err:ddf885f4:errors-join-ctx-lost` — errors join ctx lost

**Status:** done — PR #105  
**Severity:** suggestion  
**Evidence:** `internal/addon/manager.go:83-111`  
**Problem:** InstallAll aggregates failures via errors.Join(errs...) after wrapping each with ClusterError at installAndVerify:120. Good pattern.  
**Fix:** After the for-loop, add `if err := ctx.Err(); err != nil && len(errs) > 0 { errs = append(errs, err) }` so the joined error includes the ctx sentinel when cancellation contributed. Alternatively, make installAndVerify itself wrap ctx.Err via %w when Install/Verify return a ctx-related error.  
**Effort:** hours

##### `err:aa84670c:ctx-err-check-on-ctx` — ctx err check on ctx

**Status:** done — PR #105  
**Severity:** suggestion  
**Evidence:** `internal/cli/root.go:110-116`  
**Problem:** execute() still checks `if ctx.Err() != nil` to decide whether to return 130 (SIGINT) or 143 (SIGTERM). This works today because the hand-rolled signal handler always cancels the ctx before ExecuteContext returns, but it's fragile: a future subcommand that returns context.Canceled WITHOUT the parent ctx being canceled hits exitCodeFor instead of the 130/143 branch.  
**Fix:** Change to `if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) { /* ... SIGTERM/SIGINT branch ...  
**Effort:** hours

##### `err:7b2829bb:typed-no-error-iface` — typed no error iface

**Status:** done — PR #105  
**Severity:** suggestion  
**Evidence:** `internal/executor/executor.go:184-199`  
**Problem:** executor.ExitError doc still claims 'errors.Is to compare against Unwrap chain values' but the type has no Unwrap() method and no Err field. The claim is aspirational — there is nothing in the chain to traverse.  
**Fix:** Option A (recommended, -1 LOC): remove 'errors.Is to compare against Unwrap chain values' from the doc comment — the type doesn't currently chain. Option B (+5 LOC): add `Err error` field, populate from executor.run's `err` var when cmd.Run fails, implement Unwrap().  
**Effort:** hours

##### `err:f51f85bb:err-stringified-loses-type` — err stringified loses type

**Status:** done — PR #105  
**Severity:** suggestion  
**Evidence:** `internal/netutil/ip.go:43-46`  
**Problem:** Four sites still use `if err != nil || !X.Is4()` and return a synthetic fmt.Errorf that drops the netip.ParseAddr / netip.ParsePrefix error entirely. Debugging 'invalid IPv4 address: 192.168' gives no hint whether netip rejected the format, the IP-version check rejected it, or a whitespace issue.  
**Fix:** Split the conditional so parse-err and is4-check produce different messages, or wrap on the err-present case: `if err != nil { return fmt.Errorf("invalid IPv4 address %q: %w", startIP, err) } if !addr.Is4() { return fmt.Errorf("IPv6 not supported: %q", startIP) }`. Matches the 'invalid CIDR %q: %w' pattern at internal/netutil/ip.go:18.  
**Effort:** hours


#### audit-iac-and-shell

##### `iac:b803fcb7:ci-no-tflint-tfsec` — ci no tflint tfsec

**Status:** deferred  
**Severity:** minor  
**Evidence:** `.github/workflows/ci.yml:97-109`  
**Problem:** `validate-terraform` + `lint-terraform` jobs now run `terraform fmt`, `terraform validate`, and `tflint -f compact` — but no secret/policy scanner (tfsec, checkov, or trivy config). tflint catches terraform_* idiom issues; tfsec/checkov catch misconfigured provider secrets, missing `sensitive = true`, and public-exposure antipatterns that the HCL surface will grow into as the module adds network/firewall rules.  
**Fix:** Add a `tfsec` or `trivy config` step to the validate-terraform/lint-terraform job. tfsec has a maintained action `aquasecurity/tfsec-action@...`; `trivy config infrastructure/terraform` is a single call.  
**Effort:** hours

##### `iac:b803fcb7:tflint-no-config` — tflint no config

**Status:** done — PR #114  
**Severity:** suggestion  
**Evidence:** `.github/workflows/ci.yml:102-109`  
**Problem:** CI runs `tflint --init && tflint -f compact` with no `.tflint.hcl` config file in either module or environment directory. Without a config, tflint loads only the default language ruleset — the `terraform-linters/tflint-ruleset-terraform` plugin (recommended preset: module_pinned_source, required_providers, required_version, naming conventions, unused_declarations) is therefore silent.  
**Fix:** Add `infrastructure/terraform/.tflint.hcl` with `plugin "terraform" { enabled = true, preset = "recommended" }`. Point CI to it via `--config=$GITHUB_WORKSPACE/infrastructure/terraform/.tflint.hcl` (shared across module + env) or per-directory symlinks.  
**Effort:** hours

##### `iac:b803fcb7b:tflint-recommended-findings` — tflint recommended findings block CI

**Status:** done — 2026-04-22 (commit f6abdb2) — wired numa through to VM cpu blocks, deleted deprecated `data_disk_size_gb`  
**Severity:** minor  
**Evidence:** `.github/workflows/ci.yml:102-109`, `infrastructure/terraform/modules/proxmox-okd/`, `infrastructure/terraform/environments/production/`  
**Problem:** Once PR #114 pointed tflint at the `recommended` preset, the `lint-terraform` CI job started failing with real findings on the existing HCL tree (exit code 2). The findings were not enumerated during this session because tflint isn't available locally; CI's log has the specifics.  
**Fix:** Run `tflint --init && tflint --config=infrastructure/terraform/.tflint.hcl` locally against both module and environment directories, fix each reported issue (module_pinned_source, required_providers, required_version, naming, unused_declarations are the usual suspects). Alternatively, narrow the preset from `recommended` to a curated rule list if some findings are intentional (e.g., module paths pinned via branch rather than tag).  
**Effort:** hours

##### `iac:18a795d5:hcl-no-prevent-destroy-masters` — hcl no prevent destroy masters

**Status:** deferred  
**Severity:** suggestion  
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/main.tf:140-255`  
**Problem:** Master VMs (OKD control plane carrying etcd quorum state) have no `lifecycle { prevent_destroy = true }` guard. A misconfigured `terraform apply` that perturbs a force-new attribute (e.g.  
**Fix:** Add `prevent_destroy = true` to the master VM resource's `lifecycle` block, gated by a variable (e.g. `var.allow_master_destroy`, default false) that `okdctl destroy` flips before running Terraform.  
**Effort:** hours

##### `iac:e076e43c:sh-posix-not-bash` — sh posix not bash

**Status:** done — PR #114  
**Severity:** suggestion  
**Evidence:** `scripts/install.sh:1-1`  
**Problem:** Shebang `#!/bin/sh` constrains the script to POSIX sh (dash on Debian/Ubuntu, ash on Alpine), which prevents unconditional `set -o pipefail`, `[[ ]]`, and other bash conveniences. Script now mitigates with a conditional `(set -o pipefail 2>/dev/null) && set -o pipefail` probe, but future contributors may still introduce bashisms that break silently under dash/ash.  
**Fix:** Either (a) switch shebang to `#!/usr/bin/env bash` and drop the pipefail probe — bash is available on every supported install target (Debian/Ubuntu/Fedora/RHEL), Alpine is not a target platform per `uname -s` gate; or (b) keep `/bin/sh` and document in a one-line comment above the shebang that POSIX-only constructs are required, so future contributors don't accidentally introduce bashisms. The current hybrid works but sits on a portability knife-edge.  
**Effort:** hours


#### audit-modernization

##### `mod:d31d1b9d:use-map-index` — use map index

**Status:** done — PR #105  
**Severity:** minor  
**Evidence:** `internal/cli/status.go:97-107`  
**Problem:** `statusNode.role()` iterates every key of `Labels` to check for two specific well-known strings. This is a map lookup dressed as a scan — O(n) in label count when a direct `if _, ok := Labels["node-role.kubernetes.io/master"]; ok` is O(1) and reads straight.  
**Fix:** Replace with direct map index: `if _, ok := n.Metadata.Labels["node-role.kubernetes.io/master"]; ok { return "master" }; if _, ok := n.Metadata.Labels["node-role.kubernetes.io/worker"]; ok { return "worker" }; return "unknown"`. No imports needed.  
**Effort:** hours

##### `mod:6fc3d91e:use-strings-lines` — use strings lines

**Status:** done — PR #105  
**Severity:** suggestion  
**Evidence:** `internal/cli/status.go:171-171`  
**Problem:** `for _, line := range strings.Split(strings.TrimSpace(coRaw), "\n")` materializes the split slice only to walk it. Go 1.24's `strings.Lines` iterator skips the allocation.  
**Fix:** Replace `for _, line := range strings.Split(s, "\n")` with `for line := range strings.Lines(s)`. Go 1.24 stdlib, no import change.  
**Effort:** hours

##### `mod:9d79b841:use-slices-max` — use slices max

**Status:** done — PR #107  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/setup/coreos.go:70-74`  
**Problem:** One of two near-identical blocks in `findOrDownloadFCOSISO` still does `slices.Sort(matches); matches[len(matches)-1]` to fetch the lexicographically-largest filename. The sibling block at lines 57-61 was already rewritten to `slices.Max(matches)`; this one (lines 70-74) was left behind.  
**Fix:** Replace `slices.Sort(matches); isoPath := matches[len(matches)-1]` with `isoPath := slices.Max(matches)`. Go 1.21+.  
**Effort:** hours

##### `mod:0934cf1b:use-slices-concat` — use slices concat

**Status:** done — PR #107  
**Severity:** suggestion  
**Evidence:** `internal/platform/packages.go:101-101`  
**Problem:** `append(append([]string{}, m.queryArgs...), pkg)` nests two `append`s to clone-then-extend a slice. Go 1.22's `slices.Concat` expresses the same intent in one call and also matches the repo's existing `slices.Concat(a, b)` style (see internal/cli/helpers.go:213, internal/distribution/okd/dns/dnsmasq.go:155).  
**Fix:** Replace with `args := slices.Concat(m.queryArgs, []string{pkg})`. Requires `slices` import (not currently imported in this file — the sibling InstallPackages uses a plain `append([]string{...}, installed...)` which is already idiomatic).  
**Effort:** hours

##### `mod:983f67f0:use-builtin-max-innerwidth` — use builtin max innerwidth

**Status:** done — PR #107  
**Severity:** suggestion  
**Evidence:** `internal/tui/layouts.go:54-60`  
**Problem:** Two sequential `if X > innerWidth { innerWidth = X }` blocks compute a running max over two candidates. Go 1.21's `max` builtin collapses both into `innerWidth = max(innerWidth-ContentPadding+maxContentWidth+ContentPadding, minWidthForTitle)` — or more readably, `innerWidth = max(innerWidth, maxContentWidth+ContentPadding, minWidthForTitle)`.  
**Fix:** Replace with `innerWidth := max(width-ContentPadding, maxContentWidth+ContentPadding, minWidthForTitle)`. `max` takes variadic ordered args in Go 1.21+.  
**Effort:** hours

##### `mod:983f67f0:use-builtin-max-padding` — use builtin max padding

**Status:** done — PR #107  
**Severity:** suggestion  
**Evidence:** `internal/tui/layouts.go:100-104`  
**Problem:** `padding := innerWidth - lineWidth; if padding < 0 { padding = 0 }` is a hand-rolled `max(padding, 0)` — the exact floor `max` was added (Go 1.21) to express. This pattern has been flagged and fixed in at least three sibling files already; layouts.go is the last holdout in the tui package.  
**Fix:** Replace with `padding := max(innerWidth-lineWidth, 0)`. Drops 3 lines to 0.  
**Effort:** hours


#### audit-observability

##### `obs:19a715fd:level-warn-help-text` — level warn help text

**Status:** done — PR #106  
**Severity:** minor  
**Evidence:** `internal/addon/catalog/secretstore/secretstore.go:122-152`  
**Problem:** secretstore.installPrereqCheck still logs multi-line HOW-TO guides (onepassword: 6 Warn lines, vault: 3 Warn lines, bitwarden: 3 Warn lines) via env.Logger.Warn when credential files are missing. Warn is for recoverable degradation; this is user education.  
**Fix:** Emit the first line ('no secret files found ... skipping') at Warn as the actual advisory.  
**Effort:** hours

##### `obs:0d318f5c:handler-no-tty-switch` — handler no tty switch

**Status:** deferred  
**Severity:** minor  
**Evidence:** `internal/cli/logging.go:35-67`  
**Problem:** configureLogging still does not auto-select JSON format when stderr is not a TTY. Operators piping `okdctl deploy 2>&1 | jq .` get charmlog text with ANSI escapes by default and must remember `--log-format json`.  
**Fix:** Route cobra's cmd into configureLogging so `cmd.Flags().Changed("log-format")` is available. If not set and !stderrIsTTY, default logFormat to "json".  
**Effort:** hours

##### `obs:15ba17da:err-stringified-into-label` — err stringified into label

**Status:** done — PR #106  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/destroy/steps.go:32-37`  
**Problem:** destroy.steps.go builds its per-step OnError callback as `phase.WarnOnError(p.Log, label+": "+err.Error())(err)`, which concatenates err.Error() into the Warn message AND passes err again as the structured attr (WarnOnError body: `logger.Warn(msg, "err", err)`). Result: the error text appears twice — once inlined into `msg` (bypassing RedactHandler's attr-walk) and once as the structured `err` attr.  
**Fix:** Drop the `+ ": " + err.Error()` concatenation: `phase.WarnOnError(p.Log, label)(err)`. WarnOnError already emits `logger.Warn(label, "err", err)`, which gives structured consumers the label and a redaction-eligible err attr.  
**Effort:** hours

##### `obs:00000002:inconsistent-domain-prefix-keys` — inconsistent domain prefix keys

**Status:** done — PR #106  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:140-285`  
**Problem:** The codebase still leans on the `prefix: message` convention ('update-ingress:', 'haproxy:', 'kubevip:', 'cluster:', 'cleanup:', 'terraform:', 'coreos:', 'iso:', 'csr:', 'addons:') in message bodies, but no call-site pins a structured `component` or `phase` attr via logger.With(). Only `run_id` is propagated (tui.SetRunID).  
**Fix:** At each phase constructor (setup/install/postinstall/destroy/cleanup) wrap p.Log via logger.With("phase", "install"); at sub-component boundaries (haproxy, dns, kubevip, terraform, iso, packages, cleanup.services, cleanup.packages, addon.manager) narrow with logger.With("component", "haproxy"). Retain the human prefix in the message for TTY readability — the attr is additive.  
**Effort:** hours

##### `obs:9d79b841:duplicate-iso-exists-log` — duplicate iso exists log

**Status:** done — PR #106  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/setup/coreos.go:59-265`  
**Problem:** coreos.go logs `coreos: found existing iso at X` (L59, L73) and `coreos: iso already exists at X` (L201, L265) in four distinct sites. A single setup run can fire more than one because the lookups happen across different layers (local iso dir, work-dir cache, download destination, upload destination), producing near-identical Info lines.  
**Fix:** Consolidate the four sites into one helper (`logISOFound(path)`) that emits the Info line once per iso-path in a run — keep a set of already-logged paths keyed off filepath.Base. If the two message variants (found vs already-exists) carry distinct operator semantics, rename them so the distinction is legible.  
**Effort:** hours

##### `obs:366b3f2d:span-no-start-end-per-step` — span no start end per step

**Status:** done — PR #106  
**Severity:** minor  
**Evidence:** `internal/distribution/orchestrator.go:113-154`  
**Problem:** Orchestrator.executeStep still does not emit a structured start/finish log pair per step. Skipping is logged (L90) but success/duration is not — Duration is captured in StepResult but never reaches the logger.  
**Fix:** In executeStep, before step.OnStart() log `o.logger.Info("step started", "step", step.ID(), "name", step.Name())`. After Execute (both success and error branches) log `o.logger.Info("step completed", "step", step.ID(), "duration", time.Since(startedAt), "success", err == nil)`.  
**Effort:** hours

##### `obs:7b2829bb:executor-no-output-span` — executor no output span

**Status:** done — PR #106  
**Severity:** minor  
**Evidence:** `internal/executor/executor.go:213-273`  
**Problem:** executor.run and RunInteractive still only log `+ <name> <args>` at Debug when Verbose is true — nothing bookends the call in the structured stream. For a 15-minute terraform apply or oc poll the JSON sink sees nothing until completion.  
**Fix:** At start of executor.run log `e.logger.Debug("exec started", "cmd", name, "argc", len(args))` (omit argv itself — terraform invocation argv can contain a credential substitution in rare configs). After cmd.Run log `e.logger.Debug("exec completed", "cmd", name, "exit", result.ExitCode, "duration", result.Duration)`.  
**Effort:** hours

##### `obs:48688e63:message-embedded-counts` — message embedded counts

**Status:** done — PR #106  
**Severity:** minor  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:217-217`  
**Problem:** The prior audit flagged three terraform count-in-message lines in proxmox.go; L158 and L185 are now structured (`"count", n`) but L217 remains `fmt.Sprintf("terraform: plan will preview %d virtual machines", totalNodes)`. The pattern also spreads to cleanup/summary.go L73 / L80 / L94 and addon/manager.go L78 / L101 / L117 / L125 / L181 — numeric counts wedged into the message string that JSON consumers cannot index by `count`.  
**Fix:** Replace the remaining `fmt.Sprintf(...%d...)` log sites with structured attrs: proxmox.go:L217 → `p.logger.Info("terraform: plan preview", "vm_count", totalNodes)`; cleanup/summary.go:L73/L80/L94 → match L67's shape `logger.Info("cleanup: ignition files", "count", n)`; addon/manager.go:L78/L101/L117/L125/L181 → `"addons: installing", "count", len(ordered)` etc. Keep human prefix in the message for TTY readability.  
**Effort:** hours

##### `obs:aa84670c:root-error-stringified` — root error stringified

**Status:** done — PR #106  
**Severity:** suggestion  
**Evidence:** `internal/cli/root.go:187-187`  
**Problem:** The ctx-done-miss branch at L120 was migrated to structured form `tui.Error("command failed", tui.LF("err", err))` — prior audit's core case. The SetFlagErrorFunc handler at L187 still stringifies: `tui.Error(err.Error())`.  
**Fix:** Replace `tui.Error(err.Error())` with `tui.Error("flag error", tui.LF("err", err))`. Keep the exit-code logic unchanged.  
**Effort:** hours

##### `obs:ae5b624c:monitor-retry-log-per-tick` — monitor retry log per tick

**Status:** done — PR #106  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/install/monitor.go:119-127`  
**Problem:** MonitorInstallation's CSR approval tick runs every 30s for up to 60 minutes. On each tick: on error it Warns structured, on approved>0 it Infos structured.  
**Fix:** Optional: de-dup identical consecutive Warns via a `lastWarnErrMsg` tracker, downgrading repeats to Debug after the first. Keep `approved>0` Info as-is (state transition).  
**Effort:** hours


#### audit-security

##### `sec:88fd3050:cred-as-string-in-config` — cred as string in config

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/config/cluster.go:107-134`  
**Problem:** ProxmoxConfig.Password and ProxmoxConfig.APIToken are typed as `string` (with `json:"-"`). The credentials.GetProxmoxCredentials legacy fallback reads them when the env path is empty (proxmox.go:213-228), converting via []byte(px.Password) — the new slice is wipeable but the original string residue persists for the Config's lifetime.  
**Fix:** Option A (safer): remove the config-file credential path entirely — env/.env is the documented mechanism and the comment already says 'never persisted'; honour that by deleting the legacy fallback branch in GetProxmoxCredentials. Option B (if kept): retype ProxmoxConfig.Password and APIToken to []byte, adjust the loader path, and Zeroize during Config teardown.  
**Effort:** hours

##### `sec:f55b9c27:cred-string-copy-envfile` — cred string copy envfile

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/credentials/envfile.go:42-68`  
**Problem:** WriteEnvFile converts password and API-token []byte to an immutable Go string via string concatenation before calling AtomicWrite. The string copy survives Zeroize on the source []byte.  
**Fix:** Use a []byte buffer (bytes.Buffer / manual append) keyed off the raw []byte fields, then pass the buffer to AtomicWrite and scrub the buffer after the call returns. Keeps credential bytes on the wipeable path throughout.  
**Effort:** hours

##### `sec:35abd54e:cred-string-copy-env` — cred string copy env

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/credentials/proxmox.go:113-134`  
**Problem:** ProxmoxCredentials.Env() builds subprocess env entries via string concatenation: "PROXMOX_VE_PASSWORD="+string(c.Password). The resulting Go string is an immutable heap copy of the password that Zeroize cannot overwrite, leaving an unwipeable residue for the entire lifetime of the returned slice (and beyond, via GC).  
**Fix:** Return a [][]byte (or a keyed byte-slice struct) for the credential-bearing entries and have the caller build cmd.Env at the final moment; or at minimum scope the Env() slice to a tight defer-clear. The current pattern violates the design intent of keeping passwords as []byte across the lifecycle.  
**Effort:** hours

##### `sec:06f00bcb:ignition-dir-perms` — ignition dir perms

**Status:** done — PR #111  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/apache.go:28-45`  
**Problem:** ensureIgnitionDir creates /var/www/html/ignition at 0o755 and then explicitly re-chmods to 0o755 if pre-existing. The ignition files inside (bootstrap.ign, master.ign, worker.ign) carry the pullSecret.  
**Fix:** Tighten the ignition directory and file perms: dir → 0o750 owner apache:apache; files → 0o640 via CopyFileMode with 0o640. Apache serves them fine under its own uid; local non-apache users can no longer grep out the pullSecret.  
**Effort:** hours

##### `sec:00000005:bootstrap-oc-no-integrity` — bootstrap oc no integrity

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/release_extract.go:24-76`  
**Problem:** bootstrapOC downloads oc.tar.gz from mirror.openshift.com with no checksum or cosign signature verification. The docstring admits 'no upstream checksum is published for this URL; post-extraction binary-exists verification is the integrity gate'.  
**Fix:** Either (a) pin bootstrapOCURL to a specific release tag and ship a baked-in sha256 in the okdctl binary (matches the 'explicit versions — never @latest' rule in CLAUDE.md §Dependencies), or (b) verify a cosign signature on the tarball if Red Hat publishes one for the client tarball set, or (c) fall through to `oc adm release extract` via the distribution-packaged `openshift-client` rpm/deb instead of curl-to-bash. Document the trust decision in CLAUDE.md §security-invariants.  
**Effort:** hours

##### `sec:19a715fd:secretstore-plaintext-disk` — secretstore plaintext disk

**Status:** done — PR #111  
**Severity:** minor  
**Evidence:** `internal/addon/catalog/secretstore/secretstore.go:253-278`  
**Problem:** The secretstore addon reads 1password-credentials.json and 1password-token.txt (plus the vault/bitwarden equivalents) from automation/config/secrets/ and applies them as Kubernetes secrets. The code neither checks nor enforces restrictive file permissions on these on-disk credential files: a user who followed the setup instructions with `echo -n 'TOKEN' > file` gets default umask (often 0o644).  
**Fix:** Before os.ReadFile(path), Stat the path and reject any file whose perm bits exceed 0o600 unless it's sops-encrypted. Mirror the pattern used in internal/credentials/envfile.go loadEnvFileOnce.  
**Effort:** hours

##### `sec:00000006:debug-bundle-redact-partial` — debug bundle redact partial

**Status:** deferred  
**Severity:** minor  
**Evidence:** `internal/cli/config.go:65-79`  
**Problem:** redactConfig in cli/config.go only masks Provider.Proxmox.TokenID and leaves every other config field unchanged. Password and APIToken carry `json:"-"` so they never marshal into the bundle (correct today), but the function signature encourages a future 'add a field, forget to redact' regression.  
**Fix:** Walk the config via reflection and mask every string field whose struct-tag name matches the RedactHandler denylist (password, token, secret, api_key, apikey). Alternative: add an explicit `okdctl:"sensitive"` struct tag and have redactConfig honour it — future fields opt in by tagging.  
**Effort:** hours

##### `sec:26a430ee:syscall-exec-env-leak` — syscall exec env leak

**Status:** done — PR #111  
**Severity:** minor  
**Evidence:** `internal/cli/elevation.go:54-77`  
**Problem:** ensureRoot re-execs via syscall.Exec(sudoPath, args, os.Environ()). The full inherited environment is handed to sudo → the new okdctl process.  
**Fix:** Filter os.Environ() before passing to syscall.Exec: keep PATH, HOME, USER, LANG, LC_*, SUDO_*, PROXMOX_VE_* (needed downstream), OKDCTL_*, KUBECONFIG, and reject everything else. The downstream Executor now applies a similar allowlist (internal/executor/executor.go:85-121), so this layer is additive defense-in-depth — but the sudo boundary is the highest-value place to enforce.  
**Effort:** hours

##### `sec:d66c3d7f:bashrc-no-nofollow` — bashrc no nofollow

**Status:** done — PR #111  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/install/flux.go:93-143`  
**Problem:** addKubeconfigToBashrc opens ~/.bashrc with os.OpenFile(O_APPEND|O_WRONLY, 0o644) under the sudo re-exec (running as root, HOME resolved via InvokingUserHomeDir). No lstat + O_NOFOLLOW guard.  
**Fix:** Mirror the logging.go fix: lstat the path first and refuse if it is a symlink; then open with syscall.O_NOFOLLOW so any concurrent symlink-plant races fail. Alternatively, use system.AtomicWrite to read-modify-rewrite instead of O_APPEND on a user-owned file — AtomicWrite already has the fsync/rename guarantees this site could benefit from.  
**Effort:** hours

##### `sec:7b2829bb:env-append-os-environ` — env append os environ

**Status:** deferred  
**Severity:** minor  
**Evidence:** `internal/executor/executor.go:85-174`  
**Problem:** Executor now applies a defaultEnvAllowlist (good — previously-flagged broadcast of unrelated env vars is closed). But PROXMOX_ is in the prefix allowlist, so PROXMOX_VE_PASSWORD / PROXMOX_VE_API_TOKEN still reach EVERY subprocess the executor spawns — including coreutils shellouts that don't need Proxmox credentials (lsb_release, dpkg, gpg, rpm, ss, systemctl, semanage, find, rm, ssh-keyscan).  
**Fix:** Split Executor.Env into two slices: AuthEnv (credential-bearing PROXMOX_*, KUBECONFIG, GIT_*, GITHUB_TOKEN) and Env (general). Add WithAuthEnv(...) and a per-Run toggle so credential vars only reach terraform + oc + helm + sops.  
**Effort:** hours

##### `sec:451be4fa:chowntree-symlink-audit` — chowntree symlink audit

**Status:** deferred  
**Severity:** minor  
**Evidence:** `internal/system/elevation.go:100-131`  
**Problem:** ChownTreeToInvokingUser uses filepath.WalkDir + os.Lchown (symlink-safe). The docstring explicitly requires the caller to only pass paths whose subtree okdctl itself created in this process.  
**Fix:** Add a runtime guard: ChownTreeToInvokingUser should refuse root if it does not match a short allowlist (projectRoot/okd-install, projectRoot/infrastructure, user-home subdirs). Alternative: introduce a typed workdir handle (type WorkDir string) produced only by the orchestrator, so the function signature statically excludes callers that pass cfg.HTTPServer.Root.  
**Effort:** hours

##### `sec:d5915b0c:kubeconfig-env-leak` — kubeconfig env leak

**Status:** deferred  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/install/phase.go:151-162`  
**Problem:** SetupKubeconfig appends `KUBECONFIG=<path>` to p.Exec.Env, making the kubeconfig path visible to every subprocess the executor spawns from that point forward — including unrelated tools (helm, ssh-keyscan, lsb_release, dpkg, rpm). The kubeconfig file contains bearer credentials for the cluster.  
**Fix:** Couple with sec:7b2829bb: split Executor.Env into AuthEnv + Env, and push KUBECONFIG into AuthEnv. Only oc / helm / openshift-install invocations see AuthEnv; dpkg / rpm / lsb_release / ssh-keyscan do not.  
**Effort:** hours


#### audit-state-and-recovery

##### `state:93957c53:cleanup-no-confirm-cluster` — cleanup no confirm cluster

**Status:** done — PR #109  
**Severity:** major  
**Evidence:** `internal/cli/cleanup.go:37-103`  
**Problem:** `okdctl cleanup` has only `--yes` with no typo-guard against the wrong config. Unlike `okdctl destroy` which requires `--confirm-cluster=<name>` when `--yes` is passed, cleanup stops and uninstalls haproxy/dnsmasq/apache, drops the VIP secondary IP, wipes terraform.tfstate.backup + .terraform.lock.hcl + bin-dir binaries (coreos-installer/terraform/oc/kubectl) without asserting the cluster name in scripted invocations.  
**Fix:** Mirror the destroy guards: add `--confirm-cluster` (required with `--yes`, must match cfg.Cluster.Name) and `--dry-run` (prints the list of services that would be stopped and files that would be removed without mutating). Promote the destroy.go guard block into a shared helper (e.g.  
**Effort:** hours

##### `state:fb54208a:postinstall-no-rollback-path` — postinstall no rollback path

**Status:** deferred  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/postinstall/steps.go:42-93`  
**Problem:** postinstall steps (cleanup-bootstrap, deploy-production-dns) are NonFatal and mutate cluster-external state (bootstrap VM destroyed via targeted tf apply, /etc/dnsmasq.d/*.conf replaced). If StepCleanupBootstrap succeeds but StepVerifyKubeVIP fails and StepDeployProductionDNS is skipped, the cluster is left with: bootstrap VM gone, kube-vip not verified, DNS still pointed at bootstrap.  
**Fix:** Two options: (a) add `okdctl postinstall --step=dns` subcommand that re-runs just the DNS sub-phase once kube-vip is confirmed healthy; or (b) expand update-ingress to handle the bootstrap->production DNS transition when it's still pointing at bootstrap IP. Prefer (b) since update-ingress already owns DNS re-deploys.  
**Effort:** hours

##### `state:4f69fc9d:no-resume-checkpoint` — no resume checkpoint

**Status:** deferred  
**Severity:** minor  
**Evidence:** `internal/distribution/step.go:178-188`  
**Problem:** StepDef has no 'already-done' precondition hook or checkpoint. If okdctl crashes mid-setup, the next run starts from step 1, repeating work or reconciling side-effects ad-hoc.  
**Fix:** Option A (lightweight): add `ReRunSafe bool` on StepDef, default false, and require every StepDef to declare it (lint-time enforcement that a step sets ReRunSafe explicitly). Document the contract in step.go.  
**Effort:** hours

##### `state:48688e63:provision-leaves-tfplan` — provision leaves tfplan

**Status:** done — PR #109  
**Severity:** minor  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:149-174`  
**Problem:** `Provider.Provision` writes `<workDir>/tfplan` via Plan then applies it, but never sweeps the plan file on success or failure — only `destroyInfrastructure` calls `tf.Cleanup()`. After a successful deploy the operator is left with a stale `tfplan` that no longer matches state; after a failed apply it's doubly-stale.  
**Fix:** Add `defer func() { _ = p.terraformExec.Cleanup() }()` immediately after the Plan call succeeds in `Provider.Provision`, matching the bootstrap.go pattern. Plan file removal on failure also helps operators inspecting `<workDir>` after a failed apply because nothing stale is left.  
**Effort:** hours

##### `state:48688e63:proxmox-no-retry-layer` — proxmox no retry layer

**Status:** deferred  
**Severity:** suggestion  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:131-193`  
**Problem:** Provider.Provision delegates 100% to terraform — no Go-side retry on transient Proxmox API failures, no 409-already-exists handling beyond what the bpg/proxmox provider does internally. retrieveProvisionResult derives VM IPs from config (not from Proxmox), so eventual-consistency 'VM created but not yet listed' gaps are dodged by not querying Proxmox — but any future code that enumerates VMs from the API will need retry.  
**Fix:** Document the invariant in proxmox.go header: 'All Proxmox mutation MUST go through terraform.Executor. Direct HTTP calls are forbidden on deploy/destroy.' If/when status queries are added, use internal/download's retry helper (5xx/408/429 with exponential backoff, 4xx fail-fast).  
**Effort:** hours


#### audit-subprocess

##### `sub:e2343d2c:systemd-stderr-dropped` — systemd stderr dropped

**Status:** done — PR #112  
**Severity:** minor  
**Evidence:** `internal/system/systemd.go:36-43`  
**Problem:** ManageService runs systemctl enable/disable/start/stop/restart/reload via exec.CommandContext(...).Run() with both stdout and stderr left nil. On failure the caller gets a bare *exec.ExitError with no systemctl diagnostic ('Failed to enable unit: Unit file haproxy.service does not exist' / 'Job for X failed because the control process exited').  
**Fix:** Route the default (state-changing) branch through system.RunCaptured so the returned error carries systemctl's stderr diagnostic. The is-active/is-enabled probe branches can stay on bare .Run() — exit code alone is the signal and --quiet already suppresses stderr noise.  
**Effort:** hours


#### audit-tests

##### `tst:daf5bee9:no-test-kubeconfig-merge-full` — no test kubeconfig merge full

**Status:** deferred  
**Severity:** blocker  
**Evidence:** `internal/cli/kubeconfig.go:77-125`  
**Problem:** mergeNamedList now has unit coverage (TestMergeNamedList) but mergeKubeconfig itself — the full merge pipeline including (a) source/dest YAML parse, (b) three-key merge (clusters/users/contexts), (c) current-context preservation invariant (set from src only when dest has none), (d) AtomicWrite at mode 0o600 — remains untested end-to-end. The current-context and 0o600 perm guarantees are the load-bearing invariants for kubectl-default-cluster preservation and on-disk kubeconfig perms.  
**Fix:** Extend internal/cli/kubeconfig_test.go: TestMergeKubeconfig_PreservesCurrentContext — seed dest YAML with current-context=prod + one cluster 'prod', pass srcData with current-context=okd-test + clusters [okd-test,dev] via t.Setenv(KUBECONFIG, tmp) to redirect mergeTargetPath, call mergeKubeconfig(srcData), read-back YAML, assert current-context == 'prod' AND clusters contains both 'prod' and 'okd-test'. TestMergeKubeconfig_EmptyDestTakesSrcCurrentContext — empty dest → dest's current-context becomes src's.  
**Effort:** days

##### `tst:6b533f2d:no-test-approve-pending-csrs` — no test approve pending csrs

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/cluster/k8s_csrs.go:51-74`  
**Problem:** ApprovePendingCSRs drives MonitorInstallation's CSR-approval loop. No test covers (a) PendingCSRs returns [] → (0, nil) fast path, (b) non-empty list → single `oc adm certificate approve` with all names in one argv (the batching is load-bearing — N separate approve calls per tick would rate-limit the API), (c) PendingCSRs error → (0, err) propagates; (d) runCheck failure wraps with "failed to approve CSRs" prefix.  
**Fix:** Use the fake-oc pattern already landed in phase/kubectl_test.go: install a PATH-shadowed 'oc' that records argv to a temp file, then assert (a) 0 CSRs → 0 runs; (b) 3 CSRs → 1 run with argv ["adm","certificate","approve","csr-1","csr-2","csr-3"]; (c) PendingCSRs returns error → propagate; (d) approve exit !=0 → *errtypes.ClusterError wrapping. Shares the test-harness idiom with the existing kubectl_test.go suite.  
**Effort:** hours

##### `tst:830d4653:no-test-packages-cleanup-guard` — no test packages cleanup guard

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/distribution/okd/cleanup/packages.go:59-96`  
**Problem:** cleanup.Packages composes ResolveBinDir → filepath.Join → refuseCriticalPath → os.RemoveAll for each installer-managed binary (yq/helm/sops/oc/kubectl/openshift-install). The per-iter refuseCriticalPath guard is the only thing stopping an OKDCTL_BIN_DIR=/etc environment variable from walking os.RemoveAll into /etc/yq.  
**Fix:** Add internal/distribution/okd/cleanup/packages_test.go: (1) TestPackages_RefusesCriticalBinDir — pass binDir="/" (via option or env t.Setenv), stub detectPackageManager to a no-op, assert returned error is *errtypes.ClusterError (guard fires per iter, hasErrors=true); (2) TestPackages_HappyPath — binDir=t.TempDir() populated with fake `yq`, `helm`, `sops` executables, assert each is gone post-call; (3) TestPackages_MissingBinariesNoError — empty binDir → no error. Stub the package-manager dnf path to avoid requiring root.  
**Effort:** days

##### `tst:33579dd5:no-test-cleanup-haproxy` — no test cleanup haproxy

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/distribution/okd/cleanup/services.go:50-87`  
**Problem:** cleanup.HAProxy deletes the live haproxy config, globs *.backup.* siblings, removes okdctl firewall rules, releases the bastion VIP, and uninstalls the haproxy package. The wildcard glob `haproxyConfig + ".backup.*"` is not tested for attacker-shaped haproxyConfig (e.g.  
**Fix:** Add cleanup/services_test.go::TestHAProxy_GlobOnlyMatchesBackups — populate t.TempDir with `haproxy.cfg`, `haproxy.cfg.backup.1`, `haproxy.cfg.backup.2`, `haproxy.cfg.orig` (not a .backup.*); pass tmpDir/haproxy.cfg as haproxyConfig with a nop logger and stubbed firewall/netutil/packages; assert only haproxy.cfg + .backup.* are removed, .orig survives. Include a TestHAProxy_RefusesCriticalPath with haproxyConfig="/etc/passwd" asserting the guard blocks removal.  
**Effort:** days

##### `tst:33579dd5:no-test-dnsmasq-cleanup-glob` — no test dnsmasq cleanup glob

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/distribution/okd/cleanup/services.go:137-182`  
**Problem:** Dnsmasq runs os.RemoveAll against paths produced by filepath.Glob("/etc/dnsmasq.d/okd-*.conf") and a secondary backup glob. Each match is guarded by refuseCriticalPath, but the glob + guard composition is untested — the guarded-glob pairing is exactly where a regression (dropping the per-iter guard, switching Glob to a non-absolute pattern) would surface as an arbitrary-path delete primitive.  
**Fix:** Because the hard-coded /etc/dnsmasq.d/... globs need root to exercise, refactor the glob/prefix pair into dnsmasqConfigGlobs() returning the two patterns, then test the guard-loop with a fake glob list: feed ["/etc/dnsmasq.d/okd-foo.conf", "/etc", "/"] into the guarded-remove helper and assert only the first reaches the (mocked) removeFn.  
**Effort:** days

##### `tst:15ba17da:no-test-destroy-orchestration` — no test destroy orchestration

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/distribution/okd/destroy/steps.go:24-133`  
**Problem:** destroySteps orchestrates Terraform destroy + ISO removal + file cleanup + firewall cleanup, now with the 'failures' tracker and the SkipTerraform/SkipCleanup/SkipFirewall flags landed in afsa79b. No test covers (a) SkipTerraform=true skips only the first step, (b) KeepISOs=true skips ISO removal with the correct SkipReason string, (c) a non-fatal step failure is recorded in failures[] and the summary warn-logs it (the misleading-success regression guard), (d) SkipCleanup=true short-circuits StepCleanupFiles independently of the CleanupKind check.  
**Fix:** Refactor destroySteps to accept a testable dependency surface (terraform-destroyer interface, iso-remover interface, cleanup-runner interface, firewall-remover interface) via struct injection. Then test: (1) SkipTerraform=true, KeepISOs=true → only StepCleanupFiles, StepCleanupFirewall, StepPrintSummary enabled; (2) KeepISOs=false, Proxmox=nil → StepRemoveRemoteISO skipped with correct reason; (3) NonFatal step returns error → orchestrator proceeds AND summary step sees failures[] non-empty → warn emitted.  
**Effort:** days

##### `tst:25fa1be8:no-test-validateport-attacker` — no test validateport attacker

**Status:** done — PR #112  
**Severity:** major  
**Evidence:** `internal/distribution/okd/firewall/firewall.go:124-140`  
**Problem:** validatePort is the explicit defense-in-depth guard preventing Port.Protocol from flowing unchecked into fmt.Sprintf("%d/%s", ...) and onward into firewall-cmd / ufw / iptables argv. The doc comment explicitly warns: "keeping the guard here prevents a future caller from sneaking an unvalidated protocol string into the rendered rule".  
**Fix:** Add internal/distribution/okd/firewall/firewall_test.go::TestValidatePort — table-driven: valid [{6443,tcp}, {53,udp}] → nil; invalid port number [0, -1, 65536, 99999] → "invalid port number"; invalid protocol ["", "TCP", "sctp", "tcp/ip", "tcp; rm", "icmp"] → "invalid protocol". Twenty lines.  
**Effort:** hours

##### `tst:98723e5d:no-test-setup-cluster-access` — no test setup cluster access

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/distribution/okd/install/flux.go:50-91`  
**Problem:** SetupClusterAccess installs the generated kubeconfig into ~/.kube/config under the invoking user's home (after sudo re-exec), backing up any existing file at destPath+".backup.<timestamp>" and chowning each output to the invoking user. The invariants — (a) backup uses 0o600 via CopyFileMode, (b) destKubeconfig is copied at 0o600, (c) ChownToInvokingUser is called on destKubeconfig AND the new .kube dir — are the only defenses for a credential-bearing file that lands in a directory writable by root.  
**Fix:** Add internal/distribution/okd/install/flux_test.go::TestSetupClusterAccess_Perms — override HOME via t.Setenv and InvokingUserHomeDir fallback; create a srcKubeconfig under a fake clusterDir/auth/; call SetupClusterAccess; assert destKubeconfig perm == 0o600 and content round-trips; with a pre-existing destKubeconfig, assert the .backup.<ts> file also exists at 0o600. Skip the actual ChownToInvokingUser assertion (root-required) but note the code path is exercised in the test harness.  
**Effort:** days

##### `tst:ae5b624c:test-missing-synctest-monitor` — test missing synctest monitor

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/distribution/okd/install/monitor.go:43-150`  
**Problem:** MonitorInstallation is the canonical ctx-cancel-reap-goroutine pattern for openshift-install monitoring. Has a ticker-driven CSR approval loop, a kill-with-reap path, and three exit branches (installDone success, installDone error with ctx-wrapping, ctx-done kill-and-reap).  
**Fix:** Requires the csrApprover interface extraction from api:fde34e0c. Once MonitorInstallation accepts a csrApprover + an injected command runner, use testing/synctest to cover: (a) installDone(nil) → final ApprovePendingCSRs called, returns nil; (b) installDone(err) under ctx.DeadlineExceeded → error wraps DeadlineExceeded; (c) installDone(err) under ctx.Canceled → error wraps Canceled; (d) ctx cancel → kill + reap within 30s succeeds; (e) ctx cancel + kill ignored → 30s elapses + warn logged.  
**Effort:** days

##### `tst:761e5126:no-test-removehaproxy` — no test removehaproxy

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/distribution/okd/postinstall/haproxy.go:23-97`  
**Problem:** RemoveHAProxy calls os.RemoveAll(phase.DefaultHAProxyConfigPath) (= /etc/haproxy/haproxy.cfg) then tears down firewall rules, the bastion VIP, and verifies API reachability. The /etc removal has no guard against an attacker-influenced DefaultHAProxyConfigPath (currently a const, but consumed indirectly), no partial-failure test (what if firewall.RemoveRules fails?), no idempotency test (second call on an already-removed haproxy).  
**Fix:** Add postinstall/haproxy_test.go with a fake Exec / Log and an injectable haproxyConfigPath variable (apply the same test-injection pattern setup/haproxy.go uses). Cases: (a) happy path — service stopped, config file gone, firewall rules removed; (b) empty VIP skips the kube-vip verification branch; (c) os.RemoveAll error is logged but does not abort (resilience); (d) API-via-VIP wait returning non-ok yields *errtypes.NetworkError.  
**Effort:** days

##### `tst:632c9087:no-test-buildlb-ingresscontroller` — no test buildlb ingresscontroller

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:371-467`  
**Problem:** convertToLoadBalancer is a destructive conversion (`oc delete ingresscontroller` then `oc create` a rebuilt one) with an explicit rollback path via attemptRollback. The two load-bearing JSON transforms — buildLBIngressController (which must preserve domain/replicas/defaultCertificate/routeSelector/routeAdmission/nodePlacement from the original spec while swapping strategy to LoadBalancerService) and buildRollbackJSON (which must strip server-managed fields to let `oc create` succeed) — are pure, in-memory functions that feed a destructive external call, and neither has a test.  
**Fix:** Add internal/distribution/okd/postinstall/update_ingress_test.go with stdlib testing only: (1) TestBuildLBIngressController_PreservesSpecFields — craft an ingressControllerInfo with RawJSON containing all six optional spec fields populated; assert the returned JSON unmarshals to a doc whose spec.endpointPublishingStrategy.type == LoadBalancerService AND each of domain/replicas/defaultCertificate/routeSelector/routeAdmission/nodePlacement round-trips intact; (2) TestBuildLBIngressController_EmptyNamespaceDefaults — Metadata.Namespace="" → output namespace == "openshift-ingress-operator"; (3) TestBuildRollbackJSON_StripsServerFields — seed RawJSON with creationTimestamp/generation/resourceVersion/uid/managedFields + a status block; assert each field is absent from the result AND non-server fields (spec, name, namespace) remain.  
**Effort:** days

##### `tst:29293401:no-test-haproxy-rollback` — no test haproxy rollback

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/haproxy.go:87-146`  
**Problem:** ConfigureHAProxy writes to /etc/haproxy/haproxy.cfg — a root-required file on the live system — and has a rollback path that restores from backup on validation/restart failure. No test covers the rollback: (a) no prior config → no backup taken, no rollback; (b) validation fails → backup restored, service restarted with old config; (c) rollback chmod/restart failure surfaces joined errors.  
**Fix:** Requires swapping the hard-coded /etc/haproxy paths + ManageService subprocess calls for injected seams. Practical test: extract the rollback lambda into a package-local helper attemptHAProxyRollback(cause, haproxyCfgPath, backupPath, chmodFn, restartFn) error and table-drive: (a) restore fails → joined error; (b) restore OK, restart fails → joined; (c) happy rollback → cause returned.  
**Effort:** days

##### `tst:ab9b764a:no-test-installconfig-perms` — no test installconfig perms

**Status:** done — PR #112  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/ignition.go:34-83`  
**Problem:** GenerateInstallConfig reads the pull-secret and writes install-config.yaml (containing the raw pull-secret JSON) at mode 0o600 via AtomicWriteString, then duplicates it to install-config.yaml.backup via CopyFileMode at 0o600. Both perm values are critical — the pull-secret is a Red Hat registry credential.  
**Fix:** Add setup/ignition_test.go::TestGenerateInstallConfig_Perms — build a minimal cfg with cfg.Files.PullSecret + cfg.Files.SSHPublicKey pointing at tmp files; call GenerateInstallConfig with outputDir=t.TempDir; stat both install-config.yaml and install-config.yaml.backup; assert os.FileMode.Perm() == 0o600 for each. Also TestGenerateInstallConfig_PullSecretReadFail asserts *errtypes.AuthError on missing pull-secret.  
**Effort:** hours

##### `tst:41a9d4eb:no-test-redact-handler` — no test redact handler

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/logutil/redact.go:30-123`  
**Problem:** RedactHandler is the canonical slog redaction middleware — CLAUDE.md §credentials-and-secrets explicitly calls it out as the mechanism "so credentials in structured attrs never reach the sink". Its direct unit tests are absent; coverage today is indirect via tui/logger_test.go.  
**Fix:** Add internal/logutil/redact_test.go with stdlib testing + bytes.Buffer + slog.NewTextHandler as the wrapped inner. Cases: (1) TestRedactAttr_SecretKeys — feed password/PASSWORD/api_token/bearer_token; assert all replaced with "[redacted]"; (2) TestRedactAttr_NonSecret — cluster/user (non-secret) pass through; (3) TestRedactAny_URL — *url.URL with User=url.UserPassword("u","p") → output has u@ but no :p@; (4) TestRedactAny_RedactedInterface — struct with Redacted() any returning "<masked>" → replaced; (5) TestWithAttrs_RedactsDerivedLogger — logger.With("password", "x").Info(...) → output has [redacted], never "x"; (6) TestWithGroup — group propagation preserves redaction; (7) TestGroupKind — nested slog.Group with a secret key inside is redacted.  
**Effort:** days

##### `tst:98723e5d:no-test-add-kubeconfig-bashrc` — no test add kubeconfig bashrc

**Status:** done — PR #112  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/install/flux.go:93-143`  
**Problem:** addKubeconfigToBashrc appends `export KUBECONFIG=<path>` to the invoking user's ~/.bashrc. It preserves the existing file mode explicitly (doc: "appending an export line can't silently relax stricter perms the user may have set") and is idempotent (skips if `export KUBECONFIG=` already present).  
**Fix:** Extend the flux_test.go with: (1) TestAddKubeconfigToBashrc_Idempotent — pre-populate bashrc with `export KUBECONFIG=/old`, call addKubeconfigToBashrc, assert file content is byte-identical (the idempotency short-circuit); (2) TestAddKubeconfigToBashrc_PreservesMode — create bashrc at 0o600, call, stat, assert perm still 0o600; (3) TestAddKubeconfigToBashrc_CreatesIfMissing — no prior bashrc, call, assert it exists at 0o644 with the export line.  
**Effort:** hours

##### `tst:451be4fa:no-test-writeasinvoking` — no test writeasinvoking

**Status:** deferred  
**Severity:** minor  
**Evidence:** `internal/system/elevation.go:82-98`  
**Problem:** WriteAsInvokingUser combines AtomicWrite + chown-back. The "parent dir chowned iff it did not pre-exist" logic (line 84-86 + 94-96) is a subtle invariant — exists to avoid silently chowning a pre-existing dir the user created with different ownership.  
**Fix:** Skip the actual chown (root required); test only the parentExisted flag path by extracting the existence probe into a seam OR by checking behaviour via fs inspection. Minimal value unless the chown-back is mocked — consider this an acknowledgement rather than an emit-to-fix.  
**Effort:** hours


### Tier H — findings from 2026-04-25 /audit-all run

Filed by the orchestrator aggregation so `/roadmap-pickup` can fan them out when bandwidth opens. Each references the audit finding ID for diff tracking; when a finding recurs in a later run, its entry Status+Evidence updates here rather than being duplicated. Total: 226 findings (3 blocker, 44 major, 100 minor, 79 suggestion).

#### audit-security

##### `sec:6424733c:cred-as-string` — cred as string

**Status:** not started  
**Severity:** major  
**Cluster:** credentials  
**Evidence:** `internal/cli/helpers.go:272-286`  
**Problem:** writeCredentialsEnv copies px.Password (a Go string) into a fresh []byte for ProxmoxCredentials.Password. The source string lives in the *config.Config heap object until clearConfigCredentials runs — and Go strings are immutable so the original string bytes remain on the heap until GC. Same applies to px.APIToken. The defer clearConfigCredentials in runDeploy closes most of this window, but the field-level *string* type cannot be Zeroize'd.  
**Fix:** Change config.ProxmoxConfig.Password / APIToken to a wrapper type that owns []byte and a Zeroize method, populated only by the wizard's input fields. The wizard's input.Validate already scrubs; the field type just needs to never become a Go string in the first place. Keep `json:"-"` so YAML Marshal still excludes them. The clearConfigCredentials() helper then becomes Zeroize() on the wrapper.  
**Effort:** hours

##### `sec:6424733c:cred-no-zeroize` — cred no zeroize

**Status:** not started  
**Severity:** major  
**Cluster:** credentials  
**Evidence:** `internal/cli/helpers.go:117-119`  
**Problem:** createOKDProvisionerWithOpts calls creds.Env() which returns []string of "PROXMOX_VE_PASSWORD=<plaintext>" entries. The slice flows into okd.WithEnv() which appends to the persistent Provisioner.executor.Env that lives for the duration of the deploy (Prepare → Install → Configure, often 30-60 minutes). Even though the underlying creds.Password []byte is Zeroize'd via the deploy.go defer, the Env() output materialised an immutable Go string that is now resident in the executor.Env slice — and the Zeroize on creds does not reach that slice.  
**Fix:** Add a `ZeroizeEnv` method on Provisioner (or executor.Executor) that overwrites the bytes of every string entry whose key matches the secret-key allowlist (`PROXMOX_VE_PASSWORD`, `PROXMOX_VE_API_TOKEN`, `KUBECONFIG` if it inlined creds). Call it from deploy.go's defer alongside creds.Zeroize. Better: thread the credentials object directly into terraform.WithEnv so each subprocess Run rebuilds env from the still-zeroizable []byte at exec time. The Env() return-as-strings contract is the structural problem.  
**Effort:** hours

##### `sec:35abd54e:input-url-scheme-not-checked` — input url scheme not checked

**Status:** done — PR #128 (moved to Completed)

##### `sec:98723e5d:bashrc-chown-leak` — bashrc chown leak

**Status:** done — PR #123 (moved to Completed)

##### `sec:8ea706f6:dl-no-checksum` — dl no checksum

**Status:** not started  
**Severity:** major  
**Cluster:** tls-network  
**Evidence:** `internal/distribution/okd/setup/tools.go:224-275`  
**Problem:** installHashiCorpDebianRepo fetches https://apt.releases.hashicorp.com/gpg over HTTPS with no checksum or signature verification, then writes it to /usr/share/keyrings/hashicorp-archive-keyring.gpg via sudo. The GPG key itself becomes the trust anchor for every subsequent apt update — a one-time MITM during deploy plants a permanent trust root.  
**Fix:** Hard-code HashiCorp's published key fingerprint (`AA16FCBC A621E701 39936A4C 798AEC65 4FA7E1A1`) and verify with `gpg --with-fingerprint --with-colons` before installing to /usr/share/keyrings. Or ship the key bytes in the binary (it changes rarely) and skip the network fetch entirely.  
**Effort:** hours

##### `sec:40d315ad:cred-flux-deploykey-as-string` — cred flux deploykey as string

**Status:** not started  
**Severity:** minor  
**Cluster:** credentials  
**Evidence:** `internal/addon/catalog/flux/flux.go:353-380`  
**Problem:** createDeployKeySecret reads the SSH private key (id_ed25519 / flux-deploy-key) via os.ReadFile, materialising it as a []byte that is immediately wrapped in a string for `addon.BuildOpaqueSecret`. The wrapper marshals via sigs.k8s.io/yaml which round-trips through Go strings; the raw private key never gets a Zeroize call. Same path also leaks the privateKey bytes into the cmd.Env-style stdin pipeline (`oc apply -f -` via `RunWithStdinChecked` which uses strings.NewReader).  
**Fix:** Change buildFluxDeployKeySecret to take []byte for privateKey/publicKey/knownHosts and pass through unchanged. addon.BuildOpaqueSecret already takes map[string][]byte. Then zero the privateKey buffer after the oc-apply RunWithStdinChecked returns. The string conversion is purely a stop on the way to []byte; eliminate it.  
**Effort:** hours

##### `sec:40d315ad:cred-flux-helm-set-leak` — cred flux helm set leak

**Status:** not started  
**Severity:** minor  
**Cluster:** credentials  
**Evidence:** `internal/addon/catalog/flux/flux.go:78-113`  
**Problem:** Flux Install passes the full git repository URL (which may include `https://USER:TOKEN@host/` form on private mirrors) into helm --set instance.sync.url=%s through an unredacted fmt.Sprintf. Helm's `--set` arguments are visible in /proc/<pid>/cmdline to other local users, and helm's verbose log output may echo them. The repository value comes from cfg.Addons.flux.settings.repository which is loaded from config; a user pasting a tokenised URL puts that token on every other local user's process listing.  
**Fix:** Refuse repository URLs that contain `://user:password@` userinfo at validate-time (already partially constrained by ValidateSettings). Document that flux SSH-key auth is the only supported credential channel. If basic-auth must be supported, plumb it through a Kubernetes Secret (which BuildOpaqueSecret already supports) instead of helm --set.  
**Effort:** hours

##### `sec:0f076161:cred-no-zeroize` — cred no zeroize

**Status:** not started  
**Severity:** minor  
**Cluster:** credentials — related: sec:6424733c:cred-no-zeroize  
**Evidence:** `internal/cli/destroy.go:172-175`  
**Problem:** runDestroyDryRun also appends creds.Env() to terraform.WithEnv. Same lifecycle issue as the deploy createOKDProvisionerWithOpts site: the env strings outlive the Zeroize call. Less impact than the long-running deploy because dry-run is short, but the same architectural pattern.  
**Fix:** Same fix as the deploy site (sec:6424733c:cred-no-zeroize). One canonical helper that builds and zeros credential-bearing env strings together.  
**Effort:** hours

##### `sec:6424733c:input-path-not-prefix-checked` — input path not prefix checked

**Status:** not started  
**Severity:** minor  
**Cluster:** input-validation  
**Evidence:** `internal/cli/helpers.go:76-92`  
**Problem:** resolveProjectRoot does `filepath.EvalSymlinks(abs)` and falls back to the un-resolved abs path on EvalSymlinks failure (with a //nolint:nilerr). The fallback path is then handed to `runlock.Acquire`, every cleanup helper, and ChownTreeToInvokingUser — the entire deploy/destroy assumes the projectRoot has been symlink-resolved. A symlink in the cwd that points outside the workdir gets the un-resolved path back, and root-mode cleanup operates on attacker-influenced paths.  
**Fix:** Differentiate resolution failures: macOS-temp-dir noise (an os.IsNotExist component) is the documented benign case; everything else is an error. Or always resolve and return; failure to resolve a project root before mutating its descendants under sudo is itself a refusal condition. Add a final check that the resolved root contains a marker (the okdctl.yaml or .git directory) so a path-traversal symlink doesn't redirect the workdir.  
**Effort:** hours

##### `sec:15ba17da:cred-no-zeroize` — cred no zeroize

**Status:** not started  
**Severity:** minor  
**Cluster:** credentials — related: sec:6424733c:cred-no-zeroize  
**Evidence:** `internal/distribution/okd/destroy/steps.go:24-133`  
**Problem:** Destroy cleanup uses opts.SkipFirewall / opts.SkipCleanup / opts.SkipTerraform flags wired from the CLI. The credential lifecycle on destroy: handleCredentials creates ProxmoxCredentials, defers creds.Zeroize, then plumbs creds.Env() into createOKDProvisionerWithOpts. Same Env() string-residue issue as the deploy path (sec:6424733c:cred-no-zeroize) — destroy holds the credential strings on the executor for its full duration. Less long-running than deploy (terraform destroy is faster), but the credential is held for the entire teardown sequence including ssh-based ISO removal.  
**Fix:** Companion fix to sec:6424733c:cred-no-zeroize. Once a ZeroizeEnv helper exists on the provisioner, destroy.go calls it in the same defer chain.  
**Effort:** hours

##### `sec:696d6b0e:input-url-scheme-not-checked` — input url scheme not checked

**Status:** not started  
**Severity:** minor  
**Cluster:** input-validation  
**Evidence:** `internal/distribution/okd/phase/iso_cleanup.go:52-79`  
**Problem:** validateProxmoxName + validateISODir + shellSingleQuote pattern is solid for the destroy ISO path. But anyVMReferencesISO interpolates `vmid` (an int parsed from JSON) and `p.Node` directly into a remote shell `pvesh get /nodes/%s/qemu/%d/config` string — vmid is type-safe int, but if Node validation is bypassed via a future code path, the interpolation reaches the remote shell. Defense-in-depth requires the validateProxmoxName guard at every call site that hits the same buffer.  
**Fix:** The validateProxmoxName guards already exist in vmConfigReferencesISO and listProxmoxVMIDs. As a hardening pass, change the SSH transport to pvesh-via-argv: `ssh root@host pvesh get /nodes/<node>/qemu/<vmid>/config -- --output-format json` where ssh treats the trailing args as a single shlex-quoted argv, removing the format-string vector entirely. Or extract the cmd string builder into one helper so adding new pvesh calls cannot drift from the validation contract.  
**Effort:** hours

##### `sec:27088eab:input-kubeconfig-not-resolved` — input kubeconfig not resolved

**Status:** not started  
**Severity:** minor  
**Cluster:** input-validation  
**Evidence:** `internal/distribution/okd/phase/ssh.go:29-41`  
**Problem:** SSHRun uses `-o StrictHostKeyChecking=accept-new` everywhere (uploads, ISO removal, custom commands), which is TOFU. There is no provision for a per-cluster known_hosts file and no enforcement that the Proxmox host fingerprint match an operator-pinned value. A first-deploy MITM permanently locks in an attacker's host key; the destroy path also relies on this same SSH transport and inherits the trust.  
**Fix:** Add an opt-in `proxmox.host_fingerprint` config field (sha256-of-pubkey form). When set, run `ssh-keyscan` once at first contact, validate the fingerprint matches the configured value, write to a per-project known_hosts file, and pass `-o StrictHostKeyChecking=yes -o UserKnownHostsFile=<path>` for every subsequent ssh/scp call. accept-new should be the explicit fallback only when the fingerprint is unset.  
**Effort:** hours

##### `sec:761e5126:tls-insecure-skip` — tls insecure skip

**Status:** not started  
**Severity:** minor  
**Cluster:** tls-network  
**Evidence:** `internal/distribution/okd/postinstall/haproxy.go:61-76`  
**Problem:** RemoveHAProxy uses httputil.NewInsecure to GET https://<vip>:6443/healthz — the doc on httputil.NewInsecure constrains its use to bootstrap-phase self-signed kube-vip checks, but RemoveHAProxy runs at update-ingress time when the cluster is fully up and kube-apiserver has its own valid cert. The TLS-skip is structurally permanent for this code path; a kube-apiserver cert mismatch (rotation, mis-renewal, MITM) goes unflagged.  
**Fix:** After bootstrap, the kube-vip endpoint serves the same kube-apiserver cert. Read the kubeconfig's certificate-authority-data and construct an http.Client that trusts only that CA. The 'vip not in SAN' note in verify.go's verifyKubeVIPAPIHealth is true during the bootstrap-to-kube-vip transition; by the time RemoveHAProxy runs the SAN includes the VIP. Worst case, fall back to the in-cluster `oc get --raw /healthz` check that already exists in the same function.  
**Effort:** hours

##### `sec:5013fea6:cred-env-leak-to-child` — cred env leak to child

**Status:** not started  
**Severity:** minor  
**Cluster:** credentials — seam→audit-subprocess  
**Evidence:** `internal/distribution/okd/setup/release_extract.go:94-117`  
**Problem:** extractReleaseImage uses raw exec.CommandContext (not the Executor) to run oc adm release extract. This bypasses Executor.buildEnv's allowlist filter — the child process inherits the FULL parent env (os.Environ pass-through, unfiltered). Under the deploy sudo re-exec, that includes whatever env was preserved through ensureRoot's FilterParentEnv, which still includes KUBE*, PROXMOX_*, etc. — but also any env the user exported that started with those prefixes (e.g. KUBE_TOKEN). Subprocess sees them.  
**Fix:** Switch this site to use the Executor (p.Exec.Run) so the env-allowlist filter applies. The current bypass is intentional because oc-extract needs registry-auth env vars, but those are already covered by KUBE/OC_/PROXMOX prefixes in DefaultEnvAllowlist. Audit-subprocess seam owns the per-call hygiene; this finding is the policy companion.  
**Effort:** hours

##### `sec:8ea706f6:input-path-not-prefix-checked` — input path not prefix checked

**Status:** not started  
**Severity:** minor  
**Cluster:** file-toctou  
**Evidence:** `internal/distribution/okd/setup/tools.go:160-210`  
**Problem:** installBinary writes the downloaded binary to `os.TempDir() + "/" + spec.name + "-download"` — a predictable path under /tmp. Two parallel okdctl invocations (or a malicious local user racing on /tmp) could collide on this exact filename. The defer cleanup is fine, but the predictable filename in /tmp is a TOCTOU vector before the install runs `system.CopyFile` to /usr/local/bin under sudo.  
**Fix:** Use system.WriteTempFile (the canonical helper) which produces a random-suffixed name. Or os.CreateTemp(os.TempDir(), spec.name+"-download-*"). The predictable-name pattern is already not used by the dnsmasq drop-in code — apply uniformly.  
**Effort:** hours

##### `sec:7b2829bb:cred-env-leak-to-child` — cred env leak to child

**Status:** not started  
**Severity:** minor  
**Cluster:** credentials — seam→audit-subprocess  
**Evidence:** `internal/executor/executor.go:111-122`  
**Problem:** DefaultEnvAllowlist permits the prefix `GIT_` and `GITHUB_` to flow through to every subprocess (including `oc`, `helm`, `terraform`, `dnf`, `apt-get`). GIT_ASKPASS, GITHUB_TOKEN, GH_TOKEN, GIT_SSH_COMMAND etc. carry user-level creds that none of these subprocesses need. KUBE prefix similarly forwards KUBECONFIG (intended) but also any KUBE-prefixed token a user may have exported.  
**Fix:** Narrow GIT_ to only the paths/auth keys actually needed (GIT_TERMINAL_PROMPT, GIT_SSH_COMMAND if used). Move GITHUB_/GH_ to an addon-only allowlist that activates only when the addon flux is enabled — the deploy phase doesn't speak GitHub. The audit-subprocess seam owns per-call allowlisting; this finding is the policy companion.  
**Effort:** hours

##### `sec:6424733c:cred-in-log` — cred in log

**Status:** not started  
**Severity:** suggestion  
**Cluster:** redaction — seam→audit-observability  
**Evidence:** `internal/cli/helpers.go:56-63`  
**Problem:** handleCredentials logs `tui.Info(fmt.Sprintf("using credentials from %s", creds.Source))`. The fmt.Sprintf bypasses the structured logging path that logutil.RedactHandler operates on. creds.Source is a Source enum (no secrets), so this specific call is safe — but the pattern of `fmt.Sprintf` into a log message is the umbrella concern: a future field added to Source.String() that interpolates a credential would silently leak. The codebase repeatedly mixes `tui.Info(fmt.Sprintf(...))` with `tui.Info("...", tui.LF("k", v))`.  
**Fix:** This is the audit-observability seam — the redaction handler scrubs structured attrs but cannot inspect a fmt-Sprintf message. Codify in CLAUDE.md (already partially done) and add a `forbidigo` lint rule that bans `tui.Info(fmt.Sprintf(...))` so the structured path is the only path. Defer to audit-observability for the per-site cleanup.  
**Effort:** hours

##### `sec:6424733c:input-validation` — input validation

**Status:** not started  
**Severity:** suggestion  
**Cluster:** input-validation  
**Evidence:** `internal/cli/helpers.go:140-159`  
**Problem:** startMetricsServer rewrites bare `:port` to `127.0.0.1:port`, but does not refuse `0.0.0.0:port` — the doc says operators 'who explicitly want a wildcard bind can pass 0.0.0.0:port', which is a documented escape hatch. The metrics endpoint is unauthenticated. An operator passing --metrics-addr=0.0.0.0:9090 (or a host-only string parsed as 0.0.0.0) exposes the Prometheus endpoint to the network.  
**Fix:** As-documented. If hardening is desired: refuse `0.0.0.0:` and IPv6 wildcard (`[::]:`); require an explicit allow flag like `--metrics-allow-network` to bind beyond loopback.  
**Effort:** hours

##### `sec:0d318f5c:cred-no-zeroize` — cred no zeroize

**Status:** not started  
**Severity:** suggestion  
**Cluster:** credentials  
**Evidence:** `internal/cli/logging.go:35-67`  
**Problem:** configureLogging is called by PersistentPreRunE — including for the deploy/destroy/cleanup/update-ingress paths under sudo re-exec. If --log-file is set and the file already exists, configureLogging opens it append-mode 0o600. Under sudo re-exec, the file existed before the re-exec (created by the unprivileged invocation) and is now opened by root. Subsequent log lines (which include redacted attrs but also raw env / path strings) get appended to a file the user owns. After the re-exec returns, the file is still user-owned — no chown back needed because root only appended. ...  
**Fix:** Already-hardened. Documenting as a counter-example reference: this file (cli/logging.go:25-32) is the canonical pattern other sites in this audit reference for O_NOFOLLOW + lstat. No action needed.  
**Effort:** hours

##### `sec:f55b9c27:err-type-carries-cred` — err type carries cred

**Status:** not started  
**Severity:** suggestion  
**Cluster:** redaction — seam→audit-errors  
**Evidence:** `internal/credentials/envfile.go:121-134`  
**Problem:** loadEnvFileOnce constructs error messages embedding `path` (e.g. fmt.Sprintf("failed to stat env file %s", path)) — path may be the deploy-output-derived .env path. Not credential-bearing in the typical case (path is a filesystem location), but if a user pointed --output at a path that *was* derived from a credential string (an unusual but possible misuse), the error chain leaks it. Stronger: errors.Is/As checks downstream cannot distinguish 'file does not exist' from 'permission denied' without re-parsing the wrapped error string.  
**Fix:** Move `path` to a structured field in errtypes.AuthError (e.g. add Path string to the struct). Then logutil.RedactHandler can apply path-redaction policy uniformly across error types. Defers to audit-errors for the type-layer fix.  
**Effort:** hours

##### `sec:d7ce9d16:input-validation` — input validation

**Status:** not started  
**Severity:** suggestion  
**Cluster:** input-validation  
**Evidence:** `internal/distribution/okd/dns/dns.go:143-197`  
**Problem:** ConfigureSystemResolver passes `nmcli connection modify <conn> ipv4.dns <dnsConfig>` where conn is from `getActiveConnection()` (parsed nmcli output). The connection name is not validated for shell-metacharacters because RunCaptured uses argv (no shell). However, the dnsConfig string is built from `slices.Concat([]string{"127.0.0.1"}, fallbackDNS)` and fallbackDNS is validated via validateDNSAddresses. The nmcli command path is safe. Documenting because the connection-name path mixes a parsed external value into argv — argv-safe today, but a future shell-style invocation wo...  
**Fix:** Document the argv-only rule on this path. Or validate `conn` to refuse names containing whitespace, semicolons, or newlines as defense-in-depth — getActiveConnection's strings.Lines + TrimSpace is brittle to NetworkManager output drift.  
**Effort:** hours

##### `sec:696d6b0e:input-path-not-prefix-checked` — input path not prefix checked

**Status:** not started  
**Severity:** suggestion  
**Cluster:** file-toctou  
**Evidence:** `internal/distribution/okd/phase/iso_cleanup.go:215-265`  
**Problem:** RemoveFCOSISOFromProxmox fail-closed pattern is good: it validates isoDir, requires fedora-coreos-*.iso prefix, single-quotes the path, and skips files referenced by running VMs. But the skip-on-in-use logic uses the filename only (filepath.Base) for VM-config matching — two ISOs with the same basename in different paths would be aliased. Low-likelihood since isoDir is fixed at /var/lib/vz/template/iso, but the basename equality breaks if Proxmox Storage layouts use non-default ISO directories.  
**Fix:** Pass the full f (already validated by refuseUnsafeISOPath) into anyVMReferencesISO and compare against the full Proxmox storage:iso/<file> form rather than basename. Defense-in-depth for non-default storage layouts.  
**Effort:** hours

##### `sec:1e8ffb91:input-validation` — input validation

**Status:** not started  
**Severity:** suggestion  
**Cluster:** input-validation  
**Evidence:** `internal/distribution/okd/postinstall/verify.go:75-113`  
**Problem:** VerifyClusterHealth parses `oc get clusteroperators --no-headers` line-by-line and indexes `fields[4]` as the DEGRADED column. The column index is positional and unstable across oc versions — newer oc may emit additional columns or reorder them. The check is functional but brittle; in the worst case a column shift causes false-negatives (degraded operators reported as healthy) which is a security-adjacent visibility issue.  
**Fix:** Use `oc get clusteroperators -o json` and walk status.conditions for type=Degraded status=True. The same pattern is already used elsewhere in this file for nodes (parseNodeReadiness) — apply it here for consistency. Documented as the lesson learned in parseNodeReadiness's own comment: 'Replaces the prior strings.Contains(line, ...) text-parse which misclassified ...'  
**Effort:** hours

##### `sec:1e8ffb91:tls-insecure-permanent-skip` — tls insecure permanent skip

**Status:** not started  
**Severity:** suggestion  
**Cluster:** tls-network — related: sec:761e5126:tls-insecure-skip  
**Evidence:** `internal/distribution/okd/postinstall/verify.go:118-138`  
**Problem:** VerifyKubeVIP / verifyKubeVIPAPIHealth pair lives in the kube-vip handoff path. The TLS skip is structurally tied to the VIP-not-in-SAN window. After the kube-apiserver re-issues its serving cert (typically 1-3 minutes after kube-vip takes over, controlled by kubelet/cluster-version-operator), the SAN includes the VIP and TLS verification could succeed. The current implementation never retries with verification — it always skips. A continuous monitoring path that calls this function later (e.g. status command, debug-bundle) inherits the InsecureSkip even after the cert is valid.  
**Fix:** Try-with-verification first, fall back to insecure only if the cert is missing the VIP SAN. Or move the InsecureSkip into a one-time bootstrap helper and have post-bootstrap callers use the kubeconfig-CA verified client. Same shape as the haproxy.go finding (sec:761e5126:tls-insecure-skip) — companion fix.  
**Effort:** hours

##### `sec:8ea706f6:cred-env-leak-to-child` — cred env leak to child

**Status:** not started  
**Severity:** suggestion  
**Cluster:** credentials — seam→audit-subprocess  
**Evidence:** `internal/distribution/okd/setup/tools.go:211-222`  
**Problem:** getToolVersion runs `terraform --version` / `oc --version` etc. via raw exec.CommandContext (no Executor). Same env-leak issue as the release_extract finding: the process inherits the full parent env. Trivially low-impact for `--version`, but the pattern is widespread enough to call out as a policy issue.  
**Fix:** Use a tiny exec.New() instance just for version queries, with WithEnv([]string{}) to pass empty env (these tools don't need creds for --version). Or accept this as low-impact and document.  
**Effort:** hours

##### `sec:8ea706f6:cred-no-zeroize` — cred no zeroize

**Status:** not started  
**Severity:** suggestion  
**Cluster:** credentials  
**Evidence:** `internal/distribution/okd/setup/tools.go:227-248`  
**Problem:** installHashiCorpDebianRepo writes the GPG key to a temp file via system.WriteTempFile with mode 0o600 then runs `gpg --dearmor -o /usr/share/keyrings/...gpg`. The temp-file handler closes the file before gpg reads it (per WriteTempFile semantics), but the os.Remove on defer happens after gpg has succeeded. The dearmored output goes to /usr/share/keyrings (world-readable 0o644 by gpg's default) — not a defect since it's a public key, but the original tmp may briefly carry the armored key in /tmp where any local user could race it via inotify before defer cleanup. Minor — sam...  
**Fix:** Acceptable as-is — the GPG key is public. Document the cleanup contract for symmetric WriteTempFile usage.  
**Effort:** hours

##### `sec:8ea706f6:dl-hashicorp-gpg-overwrite` — dl hashicorp gpg overwrite

**Status:** not started  
**Severity:** suggestion  
**Cluster:** tls-network  
**Evidence:** `internal/distribution/okd/setup/tools.go:228-250`  
**Problem:** installHashiCorpDebianRepo's gpg --dearmor command writes to /usr/share/keyrings/hashicorp-archive-keyring.gpg unconditionally. If a previous okdctl deploy (or a system administrator) has placed a different key there, this overwrites it without warning. The path is also writable only via sudo; under the deploy re-exec model the call runs as root.  
**Fix:** If gpgPath exists, run `gpg --with-colons --import-options show-only --import gpgPath` and compare the imported fingerprint to the expected HashiCorp fingerprint before re-writing. Refuse to overwrite if a different key is present.  
**Effort:** hours

##### `sec:8ea706f6:input-validation` — input validation

**Status:** not started  
**Severity:** suggestion  
**Cluster:** input-validation  
**Evidence:** `internal/distribution/okd/setup/tools.go:129-133`  
**Problem:** installTerraform on RHEL: the repoURL `https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo` is hardcoded. dnf config-manager --add-repo trusts the repo file fetched at this URL — the file declares the gpgkey URL inside it. dnf signature-check then validates packages with that gpgkey. The chain is: HTTPS-trust-on-fetch → gpgkey-trust-on-fetch → package-signature. The first link is HTTPS-only; no signature on the .repo file itself.  
**Fix:** Embed the .repo file content in the binary and write it via WriteAsInvokingUser to /etc/yum.repos.d/hashicorp.repo with the gpgkey URL pinned to a HashiCorp-controlled HTTPS path. Removes the on-the-fly fetch step entirely. Same pattern as the deb-side installHashiCorpDebianRepo, just consistent across families.  
**Effort:** hours

##### `sec:e3782ee7:toctou-chmod` — toctou chmod

**Status:** not started  
**Severity:** suggestion  
**Cluster:** file-toctou  
**Evidence:** `internal/system/fs.go:49-71`  
**Problem:** WriteTempFile creates a temp file via os.CreateTemp (mode 0o600 by default), then `f.Chmod(mode)` to widen — this opens a window between create and chmod. The window is microscopic (single goroutine) and CreateTemp's default 0o600 is already tight, but the canonical helper documents itself as 'creates a temp file matching pattern, chmods it to mode'. If a caller passes mode 0o600 the chmod is a no-op (no window); if a caller passes 0o644 there is a brief 0o600 window where world cannot read — that direction is safe. The reverse direction (caller asks for 0o400) is also safe...  
**Fix:** As-is is acceptable today (no setuid callers). For uniformity with CopyFileMode, switch to os.OpenFile(<random-tempname>, O_RDWR|O_CREATE|O_EXCL, mode) — set the mode at open time. CreateTemp doesn't take a mode argument, so this is a small wrapper.  
**Effort:** hours

#### audit-subprocess

##### `sub:97cb8adf:no-cmd-env` — no cmd env

**Status:** done — 2026-04-26 — PR #147 (moved to Completed)

##### `sub:ae5b624c:bypass-canonical-executor` — bypass canonical executor

**Status:** done — 2026-04-26 — PR #148 (moved to Completed)

##### `sub:ae5b624c:no-cmd-env-install` — no cmd env install

**Status:** not started  
**Severity:** minor  
**Cluster:** io-handling  
**Evidence:** `internal/distribution/okd/install/monitor.go:75-78`  
**Problem:** openshift-install's install-complete monitor inherits the parent process's full environment unfiltered. Although deploy is sudo-re-exec'd through cli/elevation.go (which DOES filter env via DefaultEnvAllowlist), running as already-root or via test harnesses bypasses that re-exec — and `openshift-install` reads AWS_*/GCP_*/AZURE_* envs which DefaultEnvAllowlist deliberately omits.  
**Fix:** Either route through p.Exec which applies DefaultEnvAllowlist via buildEnv, or set installCmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist) directly. KUBECONFIG, KUBE_*, OC_*, TF_* prefixes are all on the allowlist already so openshift-install's legitimate consumers keep working.  
**Effort:** hours

##### `sub:5013fea6:unbounded-stderr-builder` — unbounded stderr builder

**Status:** not started  
**Severity:** minor  
**Cluster:** io-handling  
**Evidence:** `internal/distribution/okd/setup/release_extract.go:104-113`  
**Problem:** `oc adm release extract --tools` is bounded by a 10-minute context but its stderr is captured into an unbounded strings.Builder. The same package's executor.Executor uses ringWriter capped at 200 lines for exactly this reason; here a misbehaving registry or chatty oc release could grow the buffer unbounded over the 10-minute window.  
**Fix:** Either (a) route through p.Exec.RunStreamedChecked which uses the existing ring-buffer + stream pattern, or (b) wrap the strings.Builder in an io.LimitWriter capped at e.g. 64 KiB for error reporting purposes — the human-readable failure tail is what matters, not the full multi-minute stream.  
**Effort:** hours

##### `sub:de572c63:nmcli-output-discards-stderr` — nmcli output discards stderr

**Status:** not started  
**Severity:** suggestion  
**Cluster:** io-handling  
**Evidence:** `internal/distribution/okd/dns/dnsmasq.go:115-128`  
**Problem:** getActiveConnection runs `nmcli ... --active` via .Output() which discards stderr-not-captured-by-ExitError. nmcli prints diagnostic context to stderr (e.g. `Error: NetworkManager is not running`) that would help the user understand why connection-discovery failed; the wrapped error here loses it.  
**Fix:** Switch to system.RunCaptured-style capture: build the cmd, set cmd.Stderr = &bytes.Buffer{}, then read both. Or accept the *exec.ExitError.Stderr fallback (which Output() does populate when Stderr is unset, up to a small cap) and unwrap it via errors.As(err, &ee) at the call site.  
**Effort:** hours

##### `sub:25fa1be8:ufw-output-discards-stderr` — ufw output discards stderr

**Status:** not started  
**Severity:** suggestion  
**Cluster:** io-handling  
**Evidence:** `internal/distribution/okd/firewall/firewall.go:79-86`  
**Problem:** DetectBackend probes `ufw status` via cmd.Output() and only inspects stdout for the 'Status: active' substring. The error path discards stderr, so a ufw failure (e.g. permission denied without sudo) silently falls through to the IPTables branch. Probe-style usage is intentional, but the silent fallthrough degrades the user-visible error.  
**Fix:** Optional: log a Debug-level message with the captured stderr when the probe fails so doctor / debug-bundle output reflects the reason ufw was skipped. Probe-style fall-through is acceptable since DetectBackend deliberately tries multiple backends; the cost is just observability.  
**Effort:** hours

#### audit-state-and-recovery

##### `state:b804b2ec:bootstrap-destroy-skip-tfvars-silent` — bootstrap destroy skip tfvars silent

**Status:** done — 2026-04-26 — PR #149 (moved to Completed)

##### `state:fb54208a:postinstall-no-rollback-path` — postinstall no rollback path

**Status:** not started  
**Severity:** major  
**Cluster:** crash-recoverability  
**Evidence:** `internal/distribution/okd/postinstall/steps.go:42-94`  
**Problem:** StepCleanupBootstrap, StepVerifyKubeVIP, and StepDeployProductionDNS are all NonFatal. If StepCleanupBootstrap succeeds (bootstrap VM destroyed, terraform state mutated) but StepVerifyKubeVIP fails, StepDeployProductionDNS is skipped (gated on KubeVIPVerified). The cluster is left with bootstrap gone, VIP unverified, /etc/dnsmasq.d/ still pointing at bootstrap IP — no resume path is exposed (no `okdctl postinstall` subcommand or scoped re-run flag).  
**Fix:** Two options (roadmap state:fb54208a documents both): (a) add `okdctl postinstall --step=dns` so the DNS substep is independently re-runnable once kube-vip is healthy; (b) extend update-ingress to detect bootstrap-pointed DNS and re-run dns.DeployProduction. Prefer (b) — update-ingress already owns DNS reconciliation.  
**Effort:** hours

##### `state:4c092fce:tf-state-backup-removed-on-success` — tf state backup removed on success

**Status:** done — PR #154 (moved to Completed)  
**Severity:** major  
**Cluster:** tf-state-atomicity  
**Evidence:** `internal/infrastructure/terraform/terraform.go:314-328`  
**Problem:** Executor.Cleanup() unconditionally deletes terraform.tfstate.backup along with tfplan/destroy.tfplan after a successful destroy. The .backup file is the operator's only built-in rollback artefact if the live tfstate is later corrupted; sweeping it on success leaves the workdir in a state where a subsequent stale-state recovery has to be reconstructed from Proxmox by hand.  
**Fix:** Split Cleanup() into two methods: CleanupPlans() removes only tfplan + destroy.tfplan, CleanupBackup() removes terraform.tfstate.backup. Call CleanupPlans() at the existing site (destroy/helpers.go:46, proxmox/proxmox.go:147). Never call CleanupBackup() — let the operator decide. Or: keep .backup until the *next* successful run, mirroring git's reflog policy.  
**Effort:** hours

##### `state:15ba17da:destroy-no-precondition-resume` — destroy no precondition resume

**Status:** not started  
**Severity:** minor  
**Cluster:** phase-idempotency  
**Evidence:** `internal/distribution/okd/destroy/steps.go:24-133`  
**Problem:** destroySteps() correctly carved out --skip-terraform / --skip-cleanup / --skip-firewall flags so a partial-failed destroy can be resumed. But the steps themselves do not auto-detect 'already done' state. After a successful tf.Destroy() the next destroy run still calls Init+HasState — HasState() returns false (state file absent), step skips with a Warn. Other steps (StepCleanupFiles, StepCleanupFirewall) blindly re-execute even when their target is already absent. Forces operators to combine flags rather than letting destroy converge.  
**Fix:** Add a per-step AlreadyDone hook (see state:4f69fc9d). For destroy specifically: StepCleanupFirewall queries firewall backend before issuing remove; StepCleanupFiles checks workDir presence first. Today these steps already log warnings on absent targets — fold the same logic into a SkipWhen-style check so the orchestrator emits Skipped instead of Success-with-warning. Improves resume UX without changing destroy semantics.  
**Effort:** hours

##### `state:4f69fc9d:no-resume-checkpoint` — no resume checkpoint

**Status:** not started  
**Severity:** minor  
**Cluster:** phase-idempotency  
**Evidence:** `internal/distribution/step.go:178-212`  
**Problem:** StepDef has no 'already-done' precondition hook. The Orchestrator runs every step in order; on a mid-phase crash the next invocation starts from step 1 and re-runs every step (download tools, regenerate manifests, regenerate ignition, rebuild ISOs). Some steps tolerate the re-run; others (StepBuildISOs, StepUploadISOs, StepDeployIgnition) are slow or wasteful. Idempotency today is implicit per-Exec rather than declared and verified.  
**Fix:** Add `ReRunSafe bool` to StepDef (default false) — every StepDef must declare it. BuildSteps panics if a step omits it. For false-marked steps, also require an `AlreadyDone func(ctx) (bool, error)` hook the orchestrator consults before Exec. Stretch: persist completed StepIDs to <workDir>/.okdctl/run-state.json (AtomicWrite, 0o600) so resume is durable across PID restarts. See roadmap state:4f69fc9d:no-resume-checkpoint.  
**Effort:** hours

##### `state:262af6e4:cleanup-tfstate-removal-window` — cleanup tfstate removal window

**Status:** not started  
**Severity:** suggestion  
**Cluster:** tf-state-atomicity  
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:54-113`  
**Problem:** `cleanup.Execute` (Full kind) calls Terraform() AFTER WorkDirectory(), WebServer(), HAProxy(), Apache(), Dnsmasq(). cleanupTerraformEnv documents 'preserve tfstate so destroy can run' (good), but if WorkDirectory()'s removal of <workDir> partially fails midway, the operator may already have lost setup-time artefacts (kubeconfig, install-config) that are required to re-run a destroy IF tfstate is then ALSO partially removed by a future invocation. Order is correct (services down before files, files before terraform-cache); the residual gap is that there is no transaction bou...  
**Fix:** Two-pass cleanup: pass 1 = compute every removal target and capture an inventory snapshot to <workDir>/.cleanup-plan.json (AtomicWrite, 0o600). Pass 2 = execute removals against that snapshot. On crash mid-pass-2, the next invocation reads .cleanup-plan.json and resumes. This converts cleanup from 'best-effort' to 'declarative checkpointed'. Out-of-scope for a release fix; document the failure mode in the package doc as a known limitation.  
**Effort:** hours

##### `state:15ba17da:destroy-summary-misleading-on-skip` — destroy summary misleading on skip

**Status:** not started  
**Severity:** suggestion  
**Cluster:** crash-recoverability  
**Evidence:** `internal/distribution/okd/destroy/steps.go:118-131`  
**Problem:** StepPrintSummary classifies the destroy as 'completed' iff len(failures)==0. But Skipped steps don't append to failures, so SkipTerraform=true (resume-after-tf-destroy) leads to the summary saying 'cluster teardown completed' even though the operator skipped the only step that touches infra. The hint 're-run okdctl destroy to retry the failed steps' is also misleading on the skip path.  
**Fix:** Track skipped steps in addition to failed steps. Summary message should distinguish: (a) all attempted, all succeeded → 'cluster teardown completed', (b) some skipped → 'cluster teardown completed (skipped: terraform, firewall)', (c) some failed → 'teardown finished with non-fatal failures (...)'. The Orchestrator already records Skipped per StepResult — read it back at summary time instead of the local failures slice.  
**Effort:** hours

##### `state:c19ee328:setup-no-precondition-for-iso-rebuild` — setup no precondition for iso rebuild

**Status:** not started  
**Severity:** suggestion  
**Cluster:** phase-idempotency  
**Evidence:** `internal/distribution/okd/setup/steps.go:199-222`  
**Problem:** StepBuildISOs and StepUploadISOs both run unconditionally on every setup invocation (only SkipISOs gates them). On a partial-fail-and-resume scenario, ISOs are rebuilt from scratch (slow, ~5min) and re-uploaded over SSH (also slow, multi-GB) even when the existing ones are byte-identical. There is no checksum / mtime / sha256-cache fast-path.  
**Fix:** For StepBuildISOs: hash the (kargs, ignition-file-content, base-iso-checksum) tuple, write the hash to <customISOdir>/.iso-build-fingerprint, skip rebuild if fingerprint matches. For StepUploadISOs: a remote sha256 over SSH plus local sha256; skip upload when they match. Both are pure performance optimisations — no security or correctness impact, just developer-quality-of-life on partial resumes.  
**Effort:** hours

##### `state:48688e63:proxmox-no-retry-layer` — proxmox no retry layer

**Status:** not started  
**Severity:** suggestion  
**Cluster:** proxmox-api-idempotency  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:131-193`  
**Problem:** Provider.Provision delegates 100% to the bpg/proxmox terraform provider for retry semantics. retrieveProvisionResult derives VM IPs from config, not from Proxmox — so eventual-consistency 'VM created but not yet listed' gaps are dodged. But there is no documented invariant ('Proxmox API mutation MUST go through terraform') so a future status-query patch could trip an unprotected 5xx/429.  
**Fix:** Add an authoritative comment on proxmox.Provider header: 'All Proxmox mutation MUST flow through terraform.Executor. Direct HTTP calls are forbidden in deploy/destroy paths.' If/when status reads land, route them through internal/download's retry helper (5xx/408/429 with exponential backoff, 4xx fail-fast). roadmap.md state:48688e63:proxmox-no-retry-layer documents this.  
**Effort:** hours

#### audit-iac-and-shell

#### audit-errors

##### `err:a55b4592:vocab-ad-hoc-config-perm` — vocab ad hoc config perm

**Status:** done — PR #204 (moved to Completed)  
**Severity:** minor  
**Cluster:** domain-vocabulary  
**Evidence:** `internal/config/loader.go:22-47`  
**Problem:** Loader.LoadFile wraps insecure-perm and parse failures with bare fmt.Errorf("...%w", err) instead of typing them as errtypes.ConfigError. The exact same security check in internal/credentials/envfile.go:122-128 returns &errtypes.AuthError{Err: os.ErrPermission}. Two security-critical perm checks; two different error shapes; one mapped to exit 2 (or 5), one to exit 1.  
**Fix:** Wrap insecure-perm in &errtypes.AuthError{Msg, Err: os.ErrPermission} (matches envfile.go:124-127); wrap parse/read in &errtypes.ConfigError{Msg, Err: err}. Both preserve %w identity through Unwrap so callers can still errors.Is(err, os.ErrPermission).  
**Effort:** hours

##### `err:45cf4e29:wrap-double-context-typed` — wrap double context typed

**Status:** not started  
**Severity:** minor  
**Cluster:** wrapping  
**Evidence:** `internal/distribution/okd/install/steps.go:33-83`  
**Problem:** Phase step Exec closures re-wrap errors that the underlying function ALREADY typed. DeployInfrastructure already returns &errtypes.NetworkError or &errtypes.ClusterError, but installSteps wraps it AGAIN as &errtypes.ClusterError. errors.As still walks past the outer wrap, but the surface message duplicates context and the outer ClusterError silently reclassifies a NetworkError, drifting exit code 3 → 4 at cli/root.go.  
**Fix:** Drop the outer typed-wrap in step closures whose inner function already returns errtypes — let the inner error pass through. If the step needs additional context, use &errtypes.ClusterError{Msg: "step %s failed", Err: err} ONLY when the inner is a bare error. Same pattern at postinstall/steps.go:34 (VerifyClusterHealth already typed), setup/steps.go:82 (DownloadOKDTools double-wraps NetworkError), destroy/steps.go:47.  
**Effort:** hours

##### `err:c287d5c0:vocab-ad-hoc-distribution-type` — vocab ad hoc distribution type

**Status:** done — PR #153 (moved to Completed)  
**Severity:** minor  
**Cluster:** domain-vocabulary  
**Evidence:** `internal/distribution/okd/okd.go:99-103`  
**Problem:** Provisioner.Validate returns bare fmt.Errorf("invalid distribution type: ...") for a config-shape error. cli/root.go's exitCodeFor maps this to 1; the documented "config error → 2" contract (errtypes.go package doc) is broken. validation.WrapValidation exists for this exact pattern (config/cluster.go), but Provisioner.Validate doesn't use it.  
**Fix:** return &errtypes.ConfigError{Msg: fmt.Sprintf("invalid distribution type: expected okd, got %s", cfg.Distribution.Type)}.  
**Effort:** hours

##### `err:5013fea6:str-sniff-tool-msg` — str sniff tool msg

**Status:** done — PR #205 (moved to Completed)  
**Severity:** minor  
**Cluster:** string-sniffing  
**Evidence:** `internal/distribution/okd/setup/release_extract.go:81-127`  
**Problem:** isAuthError() does strings.Contains(lower, marker) over a fixed list against `oc adm release extract` stderr to choose between AuthError (exit 5) and ClusterError (exit 4). The exact wording of these markers ("unauthorized", "401", "no basic auth") is upstream-tooling output; oc has changed wording across minors. The author flagged this as best-effort in the comment, but the user-visible exit-code branches on it.  
**Fix:** Two-step: (1) parse the executor.ExitError exit code — non-zero exit codes 1 and 125 are typical for auth failure on most container runtimes; (2) keep the string match as a secondary heuristic but downgrade unmatched-fail to ClusterError. The single-source-of-truth is the exit code; stderr-text is the fallback. Document the risk in a TODO with a roadmap link if the exit-code path needs upstream investigation.  
**Effort:** hours

##### `err:ddf885f4:errors-join-opportunity` — errors join opportunity

**Status:** not started  
**Severity:** suggestion  
**Cluster:** wrapping  
**Evidence:** `internal/addon/manager.go:99-113`  
**Problem:** InstallAll already uses errors.Join correctly. NOT a finding — included to verify positive cluster compliance. Audit confirms `errors.Join(errs...)` and `errors.Join(err, fmt.Errorf("addon %s rollback: %w", info.Name, unErr))` at line 187 are the canonical pattern across the codebase. Other multi-error sites (cleanup/cleanup.go:113, dns/dns.go:228) also use errors.Join. No errors-join-opportunity findings.  
**Fix:** No fix; audit-positive note. Documented as a baseline so future contributors keep using errors.Join.  
**Effort:** hours

##### `err:262af6e4:sentinel-double-wrapped` — sentinel double wrapped (scaffolding — verify intent only)

**Status:** not started  
**Severity:** suggestion  
**Cluster:** sentinel-vs-typed  
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:47-108`  
**Problem:** ErrKindNotSet is exported ("Callers can test for it with errors.Is") but NO caller uses errors.Is(err, cleanup.ErrKindNotSet) anywhere in the repo. Internal use at line 106 wraps it inside ConfigError{Err: ErrKindNotSet}, which preserves identity correctly — but no consumer leverages that. Same scaffolding-vs-stale tension as err:d6b325cb.  
**Fix:** Verify intent: search roadmap.md for cli/cleanup work that would consume errors.Is(err, ErrKindNotSet); if absent, drop ErrKindNotSet entirely and replace line 106 with bare &errtypes.ConfigError{Msg}. Keep iff a doctor/preflight surfaces it.  
**Effort:** hours

##### `err:b804b2ec:bootstrap-skip-tfvars-nil-as-success` — bootstrap skip tfvars nil as success

**Status:** not started  
**Severity:** suggestion  
**Cluster:** sentinel-vs-typed — related: state:b804b2ec:bootstrap-destroy-skip-tfvars-silent  
**Evidence:** `internal/distribution/okd/postinstall/bootstrap.go:17-24`  
**Problem:** CleanupBootstrap returns nil when terraform.tfvars is missing but logs a Warn. The orchestrator step at postinstall/steps.go:46-54 then sets BootstrapCleaned=true on nil-return. From the error-axis: there's no sentinel (e.g. errtypes.RecoverableSkipped) to distinguish "skipped because not applicable" from "successfully cleaned". Cross-references audit-state-and-recovery's state:b804b2ec:bootstrap-destroy-skip-tfvars-silent.  
**Fix:** Either add a typed errtypes.SkipReason{Msg} and let the orchestrator surface skip-state separately from success, OR rely on the existing distribution.StepDef.SkipWhen + SkipReason mechanism (used elsewhere) instead of the in-body Warn+nil pattern. The latter is the canonical approach; converting CleanupBootstrap to a SkipWhen-returning-bool aligns with destroy/steps.go:43-44.  
**Effort:** hours

##### `err:9d79b841:fcos-stream-status-bare` — fcos stream status bare

**Status:** done — PR #203 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** wrapping  
**Evidence:** `internal/distribution/okd/setup/coreos.go:150-172`  
**Problem:** fetchCoreOSStream returns bare fmt.Errorf("coreos stream: HTTP %d", resp.StatusCode) for non-200 responses. The download package has a typed httpStatusError (download/retry.go:17-32) for exactly this pattern, used by isRetryable to drive retry behavior. Coreos stream HTTP failures here are NOT retryable via the same path, and a future caller wanting to errors.As(err, &httpStatusError{}) on the coreos endpoint can't.  
**Fix:** Reuse internal/download.httpStatusError (or export it as download.HTTPStatusError) so the coreos stream fetch surfaces the same shape as the rest of the download retry layer. Bonus: makes isRetryable's logic shareable.  
**Effort:** hours

##### `err:d6b325cb:sentinel-not-matched` — sentinel not matched (scaffolding — verify intent only)

**Status:** not started  
**Severity:** suggestion  
**Cluster:** sentinel-vs-typed  
**Evidence:** `internal/infrastructure/proxmox/types.go:10-13`  
**Problem:** ErrNotConnected and ErrTerraformNotConfigured are exported sentinels but no caller uses errors.Is on them — every callsite returns them BARE (proxmox.go:124, 132, 192, 200), and the cli boundary maps bare error → exit 1. They are sentinels in name only; from cli/root.go's perspective they're indistinguishable from any other error. Either wrap them in errtypes.ConfigError so the exit code reflects the user-fixable nature, or downgrade them to package-local strings.  
**Fix:** Two paths: (a) wrap each return in &errtypes.ConfigError{Msg: "proxmox provider not connected — call Connect() first", Err: ErrNotConnected} so the cli exits 2 AND callers retain errors.Is(err, ErrNotConnected) matching; or (b) make these unexported (errNotConnected) and let cli's outer wrap take over. Path (a) preserves both identity and exit code.  
**Effort:** hours

#### audit-concurrency

##### `con:bdf5a873:safe-remove-ignores-ctx` — safe remove ignores ctx

**Status:** done — PR #175 (moved to Completed)  
**Severity:** minor  
**Cluster:** ctx-ignored  
**Evidence:** `internal/distribution/okd/cleanup/artifacts.go:33-57`  
**Problem:** SafeRemoveWithLogger accepts ctx but discards it (`_ context.Context`). The body issues os.Stat / os.RemoveAll which can stall indefinitely on a slow / hung NFS or stuck mount during destroy — the long-running operation a destroy ctx is supposed to cancel. Either thread ctx through (Go 1.25 has no ctx-aware RemoveAll, so the canonical fix is to wrap the call in WaitFor + a ctx-cancel race) or drop the parameter so callers don't expect cancellation honoured.  
**Fix:** Either: (a) drop the ctx parameter (the four call sites in cleanup.go are sequential and don't supply cancellation expectations), or (b) before each os.RemoveAll, do `if err := ctx.Err(); err != nil { return err }`. (a) is honest about what the function does; (b) costs ~3 lines and gives a destroy ctx a meaningful cancel point.  
**Effort:** hours

##### `con:f5d703ab:install-tools-to-system-no-ctx` — install tools to system no ctx

**Status:** done — PR #182 (moved to Completed)

##### `con:ab9b764a:validate-ignition-only-checks-ctx-once` — validate ignition only checks ctx once

**Status:** done — PR #183 (moved to Completed)

##### `con:6424733c:metrics-shutdown-bg-ctx` — metrics shutdown bg ctx

**Status:** done — PR #202 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** ctx-todo  
**Evidence:** `internal/cli/helpers.go:140-159`  
**Problem:** startMetricsServer's stop closure builds `shutCtx` with `context.Background()` even though the caller (executeFullDeployment) has a ctx in hand. The ctx-from-caller would be cancelled by SIGINT first, which is exactly what we don't want here (we want the 5-second graceful drain to run after parent ctx cancel). So Background is *correct* — but there is no comment justifying it, which CLAUDE.md §concurrency requires ("`context.Background()` / `context.TODO()` in production code needs a justification comment").  
**Fix:** Add a one-line comment above `context.WithTimeout(context.Background(), ...)` explaining: "Use Background, not the caller's ctx, so the 5s graceful drain runs even after SIGINT cancelled the parent." Same wording as monitor.go's reapTimer comment.  
**Effort:** hours

##### `con:e7db1220:releases-completion-bg-ctx` — releases completion bg ctx

**Status:** done — PR #172 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** ctx-todo  
**Evidence:** `internal/cli/releases.go:58-71`  
**Problem:** The shell-completion ValidArgsFunction passes context.Background() to fetcher.FetchVersions. cobra exposes a Context-bearing variant via the (cmd, args, toComplete) signature — completion can get cmd.Context() which honours the user's Ctrl-C. Today, a hung GitHub call during tab-completion blocks the shell until the underlying http client's own timeout fires.  
**Fix:** Change the closure to `func(cmd *cobra.Command, _ []string, _ string)` and call `fetcher.FetchVersions(cmd.Context())`. Adds proper cancellation if the user Ctrl-C's during completion.  
**Effort:** hours

##### `con:aa84670c:time-after-update-notice-ok` — time after update notice ok

**Status:** done — PR #179  
**Severity:** suggestion  
**Cluster:** time-sleep-retry  
**Evidence:** `internal/cli/root.go:128-134`  
**Problem:** printUpdateNotice uses `case <-time.After(100 * time.Millisecond)` in a select. Bare time.After leaks the underlying Timer until it fires (per CLAUDE.md, the canonical alternative is time.NewTimer + Stop on the win path). Here the cap is 100ms and the function is called once at process exit, so the 'leak' has zero observable cost — but the pattern is the one the doc calls out.  
**Fix:** Replace with: `t := time.NewTimer(100 * time.Millisecond); defer t.Stop(); select { case result = <-ch: case <-t.C: return }`. Two extra lines for pattern consistency with internal/distribution/okd/install/monitor.go's reapTimer.  
**Effort:** hours

##### `con:98723e5d:monitor-installation-no-test` — monitor installation no test

**Status:** not started  
**Severity:** suggestion  
**Cluster:** time-sleep-retry — seam→audit-tests  
**Evidence:** `internal/distribution/okd/install/monitor.go:62-172`  
**Problem:** MonitorInstallation is the most concurrency-dense function in the codebase: a Wait-reaper goroutine, a sync.OnceFunc kill, a CSR-approval ticker, a reap timer with deadline, and a select with three cases. internal/system/exec_test.go already uses Go 1.25 testing/synctest (six tests covering WaitFor). Identical patterns in MonitorInstallation have NO tests — this is the canonical synctest opportunity per audit-concurrency rule catalog.  
**Fix:** Add monitor_test.go using `synctest.Test` to fake the openshift-install subprocess (extract a `wait()` interface to inject), advance virtual time across the CSRApprovalInterval, and assert: (a) ticker tick triggers ApprovePendingCSRs, (b) ctx-cancel triggers killInstall + reapTimer race, (c) reap-timeout path logs the abandon message. The exec_test.go pattern is the template.  
**Effort:** hours

##### `con:48688e63:proxmox-connect-discards-ctx` — proxmox connect discards ctx (scaffolding — verify intent only)

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/con-48688e63-proxmox-ctx  
**Severity:** suggestion  
**Cluster:** ctx-ignored  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:74-92`  
**Problem:** Provider.Connect and Provider.Disconnect each accept `_ context.Context` for interface symmetry but never use it. Connect's body is pure validation + struct mutation (no I/O happens until Provision). The CLAUDE.md scaffolding-as-roadmap rule (MEMORY.md §scaffolding) covers this: the ctx is API scaffolding for the symmetric Connect/Disconnect shape used by future providers (the alternative would be a Connect signature without ctx, which then breaks symmetry with a network-bound Connect on a hypothetical AWS provider). Keep the param, but flag for review.  
**Fix:** Verify intent: keep the ctx parameter on the Connect/Disconnect contract. No code change required. (Optionally add a comment: "// ctx accepted for symmetry with future network-bound providers; this implementation is local-only.")  
**Effort:** hours

#### audit-api-design

##### `api:d6b325cb:pkg-sibling-reach-through` — pkg sibling reach through

**Status:** not started  
**Severity:** major  
**Cluster:** package-boundary  
**Evidence:** `internal/infrastructure/proxmox/types.go:3-50`  
**Problem:** Generic infrastructure package internal/infrastructure/proxmox imports OKD-specific internal/distribution/okd/phase to alias VMRole = phase.NodeRole and re-export Role* constants. The directional invariant is that distribution depends on infrastructure, not the reverse — making infrastructure pull a sibling distribution package's domain types couples future providers (Hyper-V, vSphere) to OKD vocabulary.  
**Fix:** Define VMRole as a string type local to internal/infrastructure/proxmox (or move it to a neutral internal/cluster/role package) and let okd's setup/destroy code translate between phase.NodeRole and proxmox.VMRole at the boundary. The two enums currently share string values ('bootstrap','master','worker'); a thin string-typed alias in proxmox plus a one-line ParseRole call site at the okd→proxmox edge keeps the canonical phase.NodeRole role enum intact.  
**Effort:** hours

##### `api:262af6e4:opt-inconsistent` — opt inconsistent

**Status:** not started  
**Severity:** minor  
**Cluster:** option-consistency  
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:54-54`  
**Problem:** cleanup is the only OKD phase exposed as a package-level function (cleanup.Execute(ctx, opts)) rather than the {New(...)+Phase.Execute(ctx, ...)} object-oriented shape used by setup, install, postinstall, and destroy. The cleanup package even defines an Options struct embedding phase.BaseOptions like the others; only the constructor is missing. This forces callers to dance: destroy and the deploy pre-clean both fabricate cleanup.Options inline rather than calling cleanup.New(...).  
**Fix:** Either (a) bring cleanup into line: add cleanup.New(exec, logger, version) *Phase with a Phase.Execute(ctx, *Options) — exactly mirroring setup/install/postinstall/destroy — and update the two callers (okd.go::Prepare, destroy/steps.go), OR (b) acknowledge cleanup is the odd one out by name (it's truly stateless; no executor/version needed) and leave it. Pick one; the current half-state where cleanup looks like a phase but isn't constructable as one is the worst of both. If keeping (b), drop the phase.BaseOptions embedding from cleanup.Options to make the divergence honest.  
**Effort:** hours

##### `api:dd75bdeb:export-no-caller` — export no caller

**Status:** done — PR #181  
**Severity:** minor  
**Cluster:** exported-surface  
**Evidence:** `internal/distribution/okd/postinstall/context.go:4-10`  
**Problem:** PostInstallContext is exported (with a //nolint:revive stutter suppression) but is only ever consumed inside the postinstall package as the type parameter of distribution.PhaseContext[PostInstallContext]. No external caller references the type by name. The nolint comment itself names it 'established internal API; rename deferred' — the established-internal status is the giveaway: if it's internal-only it should be lowercased and the stutter goes away naturally.  
**Fix:** Lowercase to `postInstallContext`; the type only flows through distribution.PhaseContext[T any] which is generic over any T. The nolint:revive comment can be deleted in the same patch. distribution.PhaseContext can hold an unexported type just fine — generics don't care.  
**Effort:** hours

##### `api:48688e63:pkg-facade-bypassed` — pkg facade bypassed

**Status:** not started  
**Severity:** minor  
**Cluster:** package-boundary  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:153-159`  
**Problem:** Domain logic in infrastructure/proxmox imports internal/tui (a UI/presentation package) to call tui.StartSpinner. The same pattern recurs in internal/distribution/okd/install/monitor.go:L29,L79. UI concerns leaking into provisioning logic prevent reusing these packages from a non-TTY caller (an HTTP API or a test) without dragging in lipgloss/bubbletea via tui.  
**Fix:** Inject a 'progress reporter' callback (or accept a context that an outer layer wraps with progress) so domain code emits a typed event and the CLI/TUI layer renders the spinner. Concretely: add a ProgressReporter func(desc string) (stop func()) field to terraform.Executor / proxmox.Provider Options, default to a no-op, and have CLI bind it to tui.StartSpinner. install/monitor.go can pass a progress reporter through the Options struct it already takes. Keeps tui out of the domain import graph and lets headless callers run silent.  
**Effort:** hours

##### `api:0934cf1b:should-be-exported` — should be exported

**Status:** done — PR #184 (moved to Completed)

##### `api:35abd54e:export-no-caller-scaffolding` — export no caller scaffolding (scaffolding — verify intent only)

**Status:** not started  
**Severity:** suggestion  
**Cluster:** exported-surface  
**Evidence:** `internal/credentials/proxmox.go:15-36`  
**Problem:** credentials.Source enum (SourceNone, SourceEnv, SourceConfig) plus String() are exported, but only ProxmoxCredentials.Source uses them and the enum's only consumer of the string form is ProxmoxCredentials.String() in this same file. The field Source is exposed for callers to display credential provenance, and the enum String() method is the natural rendering — but no external caller reads the field today (search shows zero hits in cli/ for credentials.Source/SourceEnv/etc.). The provenance flags EndpointFromConfig and ConfigCredentialsOverridden are similarly unused externally.  
**Fix:** Verify intent. The struct doc-comment says callers SHOULD warn on EndpointFromConfig / ConfigCredentialsOverridden — that warning isn't wired anywhere yet. Either land the warning (cli/deploy.go and cli/destroy.go are the natural sites) so the surface earns its keep, or downgrade Source/EndpointFromConfig/ConfigCredentialsOverridden to package-private until a consumer materialises. Don't silently delete — the doc comment is a roadmap commitment.  
**Effort:** hours

##### `api:8e65d574:iface-in-producer` — iface in producer

**Status:** not started  
**Severity:** suggestion  
**Cluster:** interface-location  
**Evidence:** `internal/distribution/okd/install/monitor.go:52-56`  
**Problem:** csrApprover is defined consumer-side (good — Go idiom), but cluster.K8sClient (the producer) returns a concrete type that requires NewK8sClient(...) at the call site. The producer-side option struct WithCLI/WithKubeconfig/WithLogger is fine; the issue is that this consumer-side interface is unique to monitor.go yet ApprovePendingCSRs is exactly the kind of operation that other phases (postinstall verify, status) already use through the same K8sClient. If/when a second consumer needs the same shape, they'll re-declare it.  
**Fix:** Leave as-is for now (single consumer = correct Go idiom). Watch for a second consumer; promote to internal/cluster.CSRApprover only when a second site declares the same shape. Filing a tracking item is enough.  
**Effort:** hours

##### `api:0fc0041d:export-no-caller-scaffolding` — export no caller scaffolding (scaffolding — verify intent only)

**Status:** not started  
**Severity:** suggestion  
**Cluster:** exported-surface  
**Evidence:** `internal/distribution/okd/phase/condition.go:10-35`  
**Problem:** Several Condition* and NodeStatus* constants in phase/condition.go are declared but only a subset is referenced (ConditionTypeReady, ConditionStatusTrue used in cli/status.go and postinstall/verify.go). ConditionTypeAvailable, ConditionTypeProgressing, ConditionTypeDegraded, ConditionStatusFalse, ConditionStatusUnknown, NodeStatusReady, NodeStatusNotReady, NodeStatusUnknown have no in-scope callers.  
**Fix:** Keep — these are part of the symmetric Kubernetes condition enum (Ready/Available/Progressing/Degraded × True/False/Unknown). Removing partials would make the enum lopsided and break the next caller. Same scaffolding rationale as okd.ClusterStatus (api:a7f4383d): future status verb will surface non-Ready conditions in operator-degraded reporting.  
**Effort:** hours

##### `api:859eea6f:export-no-caller-scaffolding` — export no caller scaffolding (scaffolding — verify intent only)

**Status:** not started  
**Severity:** suggestion  
**Cluster:** exported-surface  
**Evidence:** `internal/distribution/okd/phase/noderole.go:22-29`  
**Problem:** phase.ParseNodeRole is exported with no in-scope callers. NodeRole has String() and the Role* constants, so ParseNodeRole completes the symmetric serializer pair. Future deserialization (status JSON, terraform output, persisted state) will need it.  
**Fix:** Keep — symmetric String() / Parse pair is canonical. Verify intent: if no roadmap item adds a JSON-deserialization path within 6 months, demote to a test-only helper.  
**Effort:** hours

##### `api:0139cb3f:export-no-caller-scaffolding` — export no caller scaffolding (scaffolding — verify intent only)

**Status:** not started  
**Severity:** suggestion  
**Cluster:** exported-surface  
**Evidence:** `internal/distribution/okd/phase/paths.go:150-152`  
**Problem:** phase.WithRecorder is exported but every existing caller assigns BasePhase.Recorder directly via field write (e.g. setupPhase.Recorder = p.recorder in okd.go::Prepare). The functional-option exists in symmetric pair with WithExecutor and WithLogger, both of which are heavily used.  
**Fix:** Keep. Optional follow-up: prefer `phase.NewBasePhase(version, phase.WithExecutor(exec), phase.WithLogger(log), phase.WithRecorder(rec))` consistently in okd.go, install/postinstall/destroy New() funcs — that erases the field-assignment dance and exercises the symmetric option.  
**Effort:** hours

##### `api:a7f4383d:export-no-caller-scaffolding` — export no caller scaffolding (scaffolding — verify intent only)

**Status:** not started  
**Severity:** suggestion  
**Cluster:** exported-surface  
**Evidence:** `internal/distribution/okd/types.go:1-57`  
**Problem:** okd.ClusterStatus, NodeStatus, Condition, ClusterPhase plus six PhaseXxx constants are exported but no in-scope code references them. internal/cli/status.go has a parallel local clusterStatus type instead of consuming okd.ClusterStatus. Either status.go should use okd.ClusterStatus (and the exported types are scaffolding for a future status verb) or okd/types.go is dead code.  
**Fix:** Verify intent before any change. If the status verb is on the roadmap (the file shape — ClusterStatus + ClusterPhase enum + lifecycle states matching `okdctl status` output — strongly suggests this), keep it as scaffolding; ideally migrate cli/status.go to use okd.ClusterStatus so the exported type has at least one consumer. If no roadmap entry references it, file an item to either adopt or delete in the next sprint.  
**Effort:** hours

#### audit-cli-ux

##### `ux:fd2125dd:addon-uninstall-no-confirm` — addon uninstall no confirm

**Status:** done — PR #206 (moved to Completed)  
**Severity:** major  
**Cluster:** verb-noun  
**Evidence:** `internal/cli/addon.go:66-80`  
**Problem:** `okdctl addon uninstall <name>` is a destructive op (deletes manifests, namespaces, secrets) but has no `--yes` flag and no confirmation prompt. Sibling destructive verbs (destroy, cleanup, update-ingress) all gate on either a TTY prompt or a `--yes`+`--confirm-cluster` pair. addon uninstall is a hole.  
**Fix:** Add `--yes` boolean (default false) and `--confirm-cluster` (typo guard, same pattern as destroy.go L82-94). When non-TTY without --yes, refuse. When --yes, require --confirm-cluster matches cfg.Cluster.Name. Copy promptForConfirmation pattern.  
**Effort:** hours

##### `ux:6424733c:no-tty-prompt-returns-false-silently` — no tty prompt returns false silently

**Status:** done — PR #186 (moved to Completed)

##### `ux:0f076161:destroy-force-deprecated-but-still-default-binding` — destroy force deprecated but still default binding

**Status:** done — PRs #185 + #192 (moved to Completed)

##### `ux:aa84670c:exit-taxonomy-doc-only-in-package-doc` — exit taxonomy doc only in package doc

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/ux-aa84670c  
**Severity:** minor  
**Cluster:** exit-codes  
**Evidence:** `internal/cli/root.go:1-8`  
**Problem:** Exit-code taxonomy lives only in the cli package's package-doc comment. There is no docs/cli/exit-codes.md, no README mention, no man-page snippet — operators writing `okdctl deploy && X` cannot discover it without reading source.  
**Fix:** Add docs/cli/exit-codes.md with the full table (codes, meanings, examples), reference it from README.md Usage section, and ensure cmd/okdctl-gen-docs picks it up if it adds a non-cobra reference page generator. Keep the package doc as a code-side anchor.  
**Effort:** hours

##### `ux:aa84670c:flag-error-os-exit-bypasses-defers` — flag error os exit bypasses defers

**Status:** not started  
**Severity:** minor  
**Cluster:** exit-codes  
**Evidence:** `internal/cli/root.go:189-193`  
**Problem:** SetFlagErrorFunc calls os.Exit(64) directly, bypassing Execute()'s `os.Exit(code)` — the logFileCloser deferred close in Execute (L72-78) never runs on a usage error. A user who passes `--log-file=/path/to/log -- garbage` loses the ability to flush partial logs because the file isn't closed before the process exits.  
**Fix:** Return a sentinel error like errtypes.UsageError{Err: err} from SetFlagErrorFunc; map it to 64 in exitCodeFor. Then Execute's deferred logFileCloser.Close() runs before exit. Removes a hidden non-local-exit path.  
**Effort:** hours

##### `ux:d31d1b9d:status-degraded-operators-parsing-fragile` — status degraded operators parsing fragile

**Status:** not started  
**Severity:** minor  
**Cluster:** json-stability  
**Evidence:** `internal/cli/status.go:167-175`  
**Problem:** runStatus parses `oc get clusteroperators --no-headers` text output by field index (fields[4] for Degraded). This is a UX issue because the JSON schema promises stable output even as oc's text format may evolve (oc 4.16 vs 4.21 columns are not guaranteed stable). Switch to `-o json` parsing for the JSON-emitting code path so the published schema's degraded_operators stays accurate.  
**Fix:** Use `oc get clusteroperators -o json` and unmarshal the items[].status.conditions list, filtering for type=Degraded status=True. Mirrors the Node parsing already at L153-L165. Stable across oc versions.  
**Effort:** hours

##### `ux:08c49fc4:remove-haproxy-no-x-bool-default-true` — remove haproxy no x bool default true

**Status:** done — PRs #187 + #193 (moved to Completed)

##### `ux:024a2c32:json-schema-display-name-hyphen-inconsistent` — json schema display name hyphen inconsistent

**Status:** not started  
**Severity:** suggestion  
**Cluster:** json-stability  
**Evidence:** `docs/cli/json-schema.md:107-121`  
**Problem:** `describe addon --format=json` emits `display-name` (kebab-case) while every sibling field uses snake_case. The docs even call out the inconsistency as 'historical reasons' but ship it. Consumers piping `okdctl describe addon X --format=json | okdctl status --format=json` cross-command get a key-style mix that can't be jq-merged cleanly.  
**Fix:** Pre-1.0, rename to `display_name` in the encoder (status.go runDescribeAddon at L337-L342). Update json-schema.md and add a note in CHANGELOG. README pins versions until 1.0 so this is the right window. Alternative (smaller blast radius): emit BOTH keys for one minor cycle, then drop the hyphen variant.  
**Effort:** hours

##### `ux:fd2125dd:addon-list-config-enabled-column-cryptic` — addon list config enabled column cryptic

**Status:** not started  
**Severity:** suggestion  
**Cluster:** help-text  
**Evidence:** `internal/cli/addon.go:191-204`  
**Problem:** `addon list` prints a table with NAME / DISPLAY-NAME / DEPS / CONFIG-ENABLED. The footnote at L204 explains CONFIG-ENABLED reflects config only. But the column header doesn't hint at the difference from runtime state, and there's no companion CLUSTER-INSTALLED column. Operators read `addon verify` for cluster state — but `list` and `verify` should at minimum cross-link in their --help.  
**Fix:** Add 'See also: addon verify' line to addonListCmd.Long. Keep the footnote. Optionally rename column to 'IN-CONFIG' (8 chars, less ambiguous) — minor consistency win across `addon list` / `addon verify` / `describe addon`.  
**Effort:** hours

##### `ux:8d8faa80:completion-powershell-on-linux-only-tool` — completion powershell on linux only tool

**Status:** done — PR #164 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** verb-noun  
**Evidence:** `internal/cli/completion.go:11-31`  
**Problem:** completionCmd advertises `powershell` as a valid arg, but okdctl is Linux-only (README L24-L26, MEMORY.md). The shell completion is generated correctly by cobra but operators on Windows literally can't run okdctl, so listing powershell at all is dishonest help text.  
**Fix:** Drop `powershell` from Use and ValidArgs. Update Long to 3 shells. Drop the powershell hint from README L70-72. (User memory says: skip Windows-compat suggestions; this is the inverse direction — *removing* a Windows-flavored thing on a Linux-only tool, which aligns with the memory note.)  
**Effort:** hours

##### `ux:d9f7733e:debug-bundle-skip-must-gather-no-quiet-suppress` — debug bundle skip must gather no quiet suppress

**Status:** not started  
**Severity:** suggestion  
**Cluster:** streams  
**Evidence:** `internal/cli/debug_bundle.go:84-84`  
**Problem:** debug-bundle writes the bundle to a file (`-o`) but tui.Info on L84/L155 still chatters on stderr. The output flag suggests the *primary* output is the bundle file; progress/status logs to stderr is fine, but there's no `--quiet`-style flag scoped to this command. Combined with the global --quiet (root.go L184), users CAN suppress, so this is a minor docs-affordance issue.  
**Fix:** Document the global --quiet in this command's Long, or just leave as-is. The streams discipline is already correct (data → file, progress → stderr). Closing this as 'verify intent'.  
**Effort:** hours

##### `ux:073d24ed:metrics-addr-no-bind-tty-gating` — metrics addr no bind tty gating

**Status:** not started  
**Severity:** suggestion  
**Cluster:** flag-conventions  
**Evidence:** `internal/cli/deploy.go:44-44`  
**Problem:** `--metrics-addr` help says 'e.g. :9090' but the helpers.go at L141-L146 transparently rewrites bare ':9090' → '127.0.0.1:9090' for safety. The flag help should mention this, otherwise a user expecting wildcard bind from `:9090` is confused when prom doesn't scrape from off-host.  
**Fix:** Update help to: 'address for Prometheus metrics endpoint; bare ":9090" binds 127.0.0.1; use "0.0.0.0:9090" for wildcard bind; disabled when empty'. The behavior is correct and security-sensible — only the docs are off.  
**Effort:** hours

##### `ux:8154ab0f:doctor-exits-1-on-fails-no-typed-error` — doctor exits 1 on fails no typed error

**Status:** not started  
**Severity:** suggestion  
**Cluster:** exit-codes — seam→audit-errors  
**Evidence:** `internal/cli/doctor.go:102-110`  
**Problem:** runDoctor returns `fmt.Errorf("preflight checks failed")` on any [fail] check. That generic error misses ConfigError/AuthError/etc., so exitCodeFor returns 1 — not the documented 'help/version=0 vs failure' contract. Doctor's own help text (doctor_cmd.go L26) commits to 'exit 0 if no fails, 1 otherwise'. That's actually preserved, but doctor would benefit from a structured exit (e.g. EX_CONFIG=78) to let `doctor || install_oc.sh` distinguish 'env not ready' from a doctor-itself crash.  
**Fix:** Wrap with `&errtypes.ConfigError{Msg: "preflight checks failed"}` so doctor exits 2 (config-error). Or add a new errtypes.PreflightError → 78 (EX_CONFIG). Update doctor_cmd.go Long to match.  
**Effort:** hours

##### `ux:daf5bee9:kubeconfig-merge-no-y-flag-no-prompt` — kubeconfig merge no y flag no prompt

**Status:** not started  
**Severity:** suggestion  
**Cluster:** flag-conventions  
**Evidence:** `internal/cli/kubeconfig.go:21-36`  
**Problem:** `okdctl kubeconfig --merge` mutates ~/.kube/config or $KUBECONFIG (writes via system.AtomicWrite at L120) without confirmation, even on a TTY. The merge logic preserves existing entries, so this is non-destructive in practice — but it differs from destroy/cleanup/update-ingress which all confirm. Either document 'merge is non-destructive, skip prompt' in --help, or add a confirmation when an existing destination has entries.  
**Fix:** Update --merge help: 'merge into $KUBECONFIG or ~/.kube/config (non-destructive: existing entries preserved)'. No code change needed — the doc is the gap.  
**Effort:** hours

##### `ux:e7db1220:json-output-not-suppressed-by-quiet` — json output not suppressed by quiet

**Status:** not started  
**Severity:** suggestion  
**Cluster:** streams  
**Evidence:** `internal/cli/releases.go:104-106`  
**Problem:** `releases list --format=json` writes structured JSON to stdout, but `tui.Info(...)` calls in fetchFlatVersions / dependent helpers still emit informational lines to stderr regardless of `--format`. A consumer doing `okdctl releases list --format=json | jq .` is fine (stderr separated) but `okdctl ... 2>&1 | jq` (common in CI) breaks. clig.dev advises that --json mode imply --quiet (suppress info chatter on stderr) unless the user overrides.  
**Fix:** In runReleasesList/runReleasesShow/runStatus when format==json, set effective log level to error (or skip tui.Info chatter) unless --verbose is explicit. Centralize via a helper in logging.go: `func quietForJSON(format string)`.  
**Effort:** hours

#### audit-observability

##### `obs:6424733c:string-concat-err-error-in-tui` — string concat err error in tui

**Status:** done — PR #155 (moved to Completed)  
**Severity:** minor  
**Cluster:** field-stability  
**Evidence:** `cmd/okdctl/main.go:52-52`  
**Problem:** `tui.Warn("failed to prepend " + binDir + " to PATH: " + err.Error())` builds the full message via string concatenation, so the err's chain reaches the sink as a flat string. RedactHandler scrubs structured attr values, not the message body — any future error wrapping a cred would slip through this site.  
**Fix:** Replace with `tui.Warn("OKDCTL_BIN_DIR ignored", tui.LF("value", v), tui.LF("err", detail))` and `tui.Warn("failed to prepend bin dir to PATH", tui.LF("bin_dir", binDir), tui.LF("err", err))`. Two-line edit each.  
**Effort:** hours

##### `obs:33579dd5:err-stringified-bypasses-handler` — err stringified bypasses handler

**Status:** done — PR #176 (moved to Completed)  
**Severity:** minor  
**Cluster:** field-stability — seam→audit-errors; related: err:a4001485:errtype-msg-vs-error-asymmetry  
**Evidence:** `internal/distribution/okd/cleanup/services.go:147-147`  
**Problem:** Four cleanup sites pass `guardErr.Error()` as the slog message rather than as a structured `err` attr. The handler sees only a free-form string, loses error-chain identity, and can't apply the cred/userinfo/Redacted() sweep that fires on attr values.  
**Fix:** Rewrite as `logger.Warn("cleanup: refusing critical path", "err", guardErr)` (matches the existing idiom at services.go:139 and artifacts.go:36). One mechanical edit per site.  
**Effort:** hours

##### `obs:366b3f2d:step-completed-info-on-failure` — step completed info on failure

**Status:** done — PR #180 (follow-up PR #191)  
**Severity:** minor  
**Cluster:** level-discipline — seam→audit-cli-ux  
**Evidence:** `internal/distribution/orchestrator.go:142-142`  
**Problem:** `step: completed` is logged at Info level whether the step succeeded or failed. A failed fatal step is a user-visible failure — it should surface at Warn or Error so log-level filtering at the sink (e.g. `--log-level=warn`) reaches it.  
**Fix:** Branch on result: `if !result.Success { o.logger.Warn("step: failed", ...) } else { o.logger.Info("step: completed", ...) }`. If `step.IsFatal()`, escalate to Error. Keeps the structured shape; flips the level so log-filter consumers can grep.  
**Effort:** hours

##### `obs:48688e63:apply-failure-no-err-attr` — apply failure no err attr

**Status:** done — PR #165 (moved to Completed)  
**Severity:** minor  
**Cluster:** field-stability  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:164-164`  
**Problem:** On terraform apply failure, the warn log carries the user-facing recovery hint ("run 'okdctl destroy'") but does NOT carry the apply error itself as an attr. The error then surfaces only via `fmt.Errorf(... %w ...)` two lines later — the slog record has no `err` field, so log-aggregation can't filter by error class.  
**Fix:** `p.logger.Warn("terraform: apply failed; run 'okdctl destroy' to clean up partial infrastructure", "err", applyErr)`. Single-attr addition; recovery hint stays in the message so TTY users still see it.  
**Effort:** hours

##### `obs:660d83a5:run-id-mutation-race` — run id mutation race

**Status:** done — PR #188 (moved to Completed)

##### `obs:ed55ee90:summary-keys-leading-whitespace` — summary keys leading whitespace

**Status:** done — PR #189 (moved to Completed)

##### `obs:c287d5c0:cleanup-warning-key-vague` — cleanup warning key vague

**Status:** done — PR #166 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** field-stability  
**Evidence:** `internal/distribution/okd/okd.go:122-122`  
**Problem:** `logger.Warn("cleanup warning", "err", err)` uses an unkeyed message that doesn't identify which cleanup operation failed (the parent context is `Prepare → cleanup.Execute`). Compare to the rest of the cleanup package (`cleanup: failed to stop service`, `cleanup: failed to remove package`) — this one breaks the established `cleanup: <verb> <object>` key convention.  
**Fix:** `p.logger.Warn("cleanup: pre-deploy artifact removal incomplete", "phase", "prepare", "err", err)`. Adds a `phase` attr matching orchestrator's `step` key style.  
**Effort:** hours

##### `obs:97cb8adf:waitfor-no-retry-count` — waitfor no retry count

**Status:** done — PR #173 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** span-retry-boundary  
**Evidence:** `internal/system/exec.go:54-108`  
**Problem:** `WaitFor` logs `prefix: waiting` with `for` and `elapsed` on every poll iteration at Debug, but the start log ("waiting for X") and the ready log ("X is ready") form the span; the iteration count never appears in either bookend. Operators tailing logs can't tell from the structured record how many polls fired before ready.  
**Fix:** Track iteration count in the loop and append `"polls", i, "elapsed", elapsed` to the readyMsg at L81/L100 and to the timeout error at L96. Three structured attrs on existing log calls; no new sites.  
**Effort:** hours

#### audit-modernization

##### `mod:f55b9c27:use-builtin-clear` — use builtin clear

**Status:** done — PR #159 (moved to Completed)  
**Severity:** minor  
**Cluster:** any-interface-builtins  
**Evidence:** `internal/credentials/envfile.go:81-83`  
**Problem:** `WriteEnvFile` hand-rolls a `for i := range data { data[i] = 0 }` to zero the buffer's backing store after the credential bytes have been flushed to disk. Go 1.21's `clear` builtin expresses the same operation in one call and reads as "wipe", which matters on a credential-handling path where the loop's intent is load-bearing.  
**Fix:** Replace the three-line loop with `clear(data)`. Identical semantics on `[]byte`; preserves the `// Zero the buffer's backing store` comment as the WHY explanation. Keep the comment — `clear(data)` reads as the operation, the comment still adds the security context.  
**Effort:** hours

##### `mod:35abd54e:use-builtin-clear` — use builtin clear

**Status:** done — PR #160 (moved to Completed)  
**Severity:** minor  
**Cluster:** any-interface-builtins  
**Evidence:** `internal/credentials/proxmox.go:82-89`  
**Problem:** `Zeroize` hand-rolls two `for i := range slice { slice[i] = 0 }` zero-fill loops where the Go 1.21 `clear` builtin does exactly this — and signals intent at the call site ("this is a wipe, not a fill"). The current loop is correct but verbose, and a maintainer scanning for credential-handling sites benefits from the named operation.  
**Fix:** Replace each loop with `clear(c.Password)` / `clear(c.APIToken)`. Go spec guarantees `clear` on a `[]T` sets every element to T's zero value, which for `[]byte` is the same byte-by-byte zeroize. Keep the subsequent `c.Password = nil` / `c.APIToken = nil` assignments unchanged — they release the backing array.  
**Effort:** hours

##### `mod:7b2829bb:use-slices-containsfunc` — use slices containsfunc

**Status:** done — PR #161 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** slices-maps  
**Evidence:** `internal/executor/executor.go:132-142`  
**Problem:** `EnvAllowlist.allows` hand-rolls `for _, p := range a.Prefixes { if strings.HasPrefix(key, p) { return true } } return false` — the canonical shape of `slices.ContainsFunc`. Go 1.21 stdlib captures exactly this pattern. The codebase already adopts it elsewhere (`internal/cli/status.go:92`, `internal/logutil/redact.go:120`), so the swap aligns with house style.  
**Fix:** Replace the loop body with `return a.Exact[key] || slices.ContainsFunc(a.Prefixes, func(p string) bool { return strings.HasPrefix(key, p) })`. Add `"slices"` to imports. Net result: -7 LOC, same behavior, matches in-tree usage.  
**Effort:** hours

#### audit-code-smells

##### `smell:073d24ed:duplicate-step-id-table` — duplicate step id table

**Status:** not started  
**Severity:** minor  
**Cluster:** magic-strings  
**Evidence:** `internal/cli/deploy.go:172-206`  
**Problem:** deployDryRunSteps() hand-rolls the entire phase step list as raw "install-packages"/"deploy-infrastructure"/"verify-kubevip" string literals while the canonical StepID constants already exist in setup/steps.go, install/steps.go, postinstall/steps.go (StepInstallPackages, StepDeployInfra, StepVerifyKubeVIP, etc.). The dry-run summary will silently drift from the real phase order whenever a phase reorders or renames a step.  
**Fix:** Reuse the canonical step constants. Either (a) add a phase-level `Steps() []DryRunStep` method on each Phase that returns ID + Name from the actual StepDef list, or (b) build deployDryRunSteps from `string(setup.StepInstallPackages)` etc. Option (a) is the durable fix — dry-run should walk the same source-of-truth StepDef tables that Run uses, eliminating the double-entry book-keeping.  
**Effort:** hours

##### `smell:d31d1b9d:role-string-instead-of-enum` — role string instead of enum

**Status:** done — PR #178  
**Severity:** minor  
**Cluster:** magic-strings  
**Evidence:** `internal/cli/status.go:97-222`  
**Problem:** statusNode.role() returns the bare strings "master"/"worker"/"unknown" and printClusterStatus switch-cases on the same string literals, ignoring the typed phase.NodeRole / phase.RoleMaster / phase.RoleWorker enum that already exists in internal/distribution/okd/phase/noderole.go. The two sites are one rename away from drifting (e.g. someone introduces 'control-plane' upstream) — a typed enum would catch it at compile time.  
**Fix:** Change role()'s return type to phase.NodeRole and return phase.RoleMaster / phase.RoleWorker. Add a phase.RoleUnknown constant or return ("", false). The status counter switch then becomes `case phase.RoleMaster: ... case phase.RoleWorker:` which the compiler enforces against the canonical enum.  
**Effort:** hours

##### `smell:830d4653:duplicate-os-fallback` — duplicate os fallback

**Status:** done — PR #177  
**Severity:** minor  
**Cluster:** helper-package-no-value  
**Evidence:** `internal/distribution/okd/cleanup/packages.go:17-25`  
**Problem:** detectOS in cleanup/packages.go and the inline equivalent in setup/phase.go::New both call platform.Detect(), Warn on error, and fall back to platform.OS{Family: FamilyRHEL, ID: "unknown"}. The exact fallback literal is duplicated in two phases. A canonical platform.DetectOrDefault helper is one-of-each.  
**Fix:** Add `func DetectOrDefault(logger *slog.Logger) OS` to internal/platform/platform.go encapsulating the warn+fallback. Replace the cleanup detectOS function and the setup inline block with a call. Net: -8 LOC, single source of truth for the 'platform-detect failed' decision.  
**Effort:** hours

##### `smell:9d79b841:strconv-fallback-to-zero` — strconv fallback to zero

**Status:** done — PR #174 (moved to Completed)  
**Severity:** minor  
**Cluster:** stringified-numbers  
**Evidence:** `internal/distribution/okd/setup/coreos.go:134-138`  
**Problem:** parseOKDMinor uses fmt.Sscanf with %d and discards the err — a malformed version like '"4.x.0"' resolves to minor=0 and the caller proceeds to fetch fcos.json (the legacy file). Silent fallback through a parse failure rather than typed minor.Parse(...) (uint, ok). Compounded by the doc-comment claiming 'parses to minor 0' as if that were a feature.  
**Fix:** Either (a) return (int, bool) so the caller can refuse the request with a typed errtypes.ConfigError, or (b) at minimum reject minor==0 inside DetectCoreOSVersion before formatting the URL. Today an unparseable version sends okdctl to https://raw.githubusercontent.com/openshift/installer/release-4.0/... which 404s; better to fail-fast at parse time.  
**Effort:** hours

##### `smell:2f70d7df:magic-default-port` — magic default port

**Status:** done — PR #162 (moved to Completed)  
**Severity:** minor  
**Cluster:** magic-strings  
**Evidence:** `internal/distribution/okd/setup/kargs.go:73-73`  
**Problem:** BuildIgnitionURLForNode falls back to the literal 8080 when cfg.HTTPServer.Port is unset, but the canonical DefaultIgnitionPort = 8080 constant lives one file over in setup/phase.go. Two sources of truth — bump the constant and this fallback drifts silently.  
**Fix:** Replace `ignitionPort = 8080` with `ignitionPort = DefaultIgnitionPort`. Both files are in package setup so no import is needed.  
**Effort:** hours

##### `smell:8aa632a6:duplicate-platform-string` — duplicate platform string

**Status:** done — PR #163 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** helper-package-no-value  
**Evidence:** `internal/cli/debug_bundle.go:144-144`  
**Problem:** debug_bundle.go:144 rebuilds `fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)` even though version.Platform — a package-level var holding the exact same expression — is already imported at line 24. Two byte-identical builds of the same string from the same inputs.  
**Fix:** Replace `Platform: fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)` with `Platform: version.Platform` and remove the unused `runtime` import (debug_bundle.go:12) if no other code references it.  
**Effort:** hours

##### `smell:7f86cbe2:any-return-second-value` — any return second value

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/smell-7f86cbe2-wizard-step-state  
**Severity:** suggestion  
**Cluster:** interfaceany-lazy  
**Evidence:** `internal/cli/wizard_setup.go:35-52`  
**Problem:** stepRegistration.factory returns `(wizard.WizardStep, any)` but the second value is either a typed *State pointer (BasicsStep, ProxmoxStep, …) or the literal nil (WelcomeStep, DistributionStep, ReviewStep). The `any` defers all type checks to BuildSteps and forces every consumer to type-assert. A stronger signature is `(wizard.WizardStep, wizard.StepState)` with a marker interface or a generic factory keyed by StepType.  
**Fix:** Define `type WizardStepState interface{ wizardStateMarker() }` (or use a generic StepBuilder.Register[T any]) so the factory's second return is structurally constrained. Alternative: split into two registration tables — one for steps with state (require non-nil), one for stateless. The current `any` lets a consumer accidentally register a string and only fail at runtime on the BuildSteps type assertion.  
**Effort:** hours

##### `smell:125729c4:unused-public-field-force` — unused public field force (scaffolding — verify intent only)

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/smell-125729c4-destroy-force  
**Severity:** suggestion  
**Cluster:** helper-package-no-value  
**Evidence:** `internal/distribution/okd/destroy/phase.go:24-24`  
**Problem:** destroy.Options.Force is exported, set in okd.go::Destroy via `destroyOpts.Force = true`, but never read anywhere — the field's only consumer is the doc-comment that says "by the time Execute runs, opts.Force is expected to be true". The actual confirmation guard is at the CLI layer using destroyForce, not via this field. Likely scaffolding for a future destroy --force semantic distinct from --yes.  
**Fix:** Either (a) verify intent against roadmap.md and add a doc-comment naming the future verb, or (b) drop the field entirely. AutoApprove already covers the 'skip prompt' axis; Force as a distinct concept needs a written spec or it stays dead. Tagged scaffolding: a `--force` destroy that bypasses live-cluster guards is a plausible future verb.  
**Effort:** hours

##### `smell:696d6b0e:redundant-vmstatus-enum` — redundant vmstatus enum

**Status:** not started  
**Severity:** suggestion  
**Cluster:** magic-strings — related: api:262af6e4:opt-inconsistent  
**Evidence:** `internal/distribution/okd/phase/iso_cleanup.go:81-105`  
**Problem:** phase/iso_cleanup.go declares `type vmStatus string; const vmStatusRunning vmStatus = "running"` while internal/infrastructure/proxmox/types.go already exposes `type VMState string; StateRunning VMState = "running"` for the same Proxmox concept. Two parallel enums for the same wire-protocol value. The phase-package shape is private (single value, single use site), but it is still a logical duplicate of the infra enum.  
**Fix:** Either (a) move VMState into a shared package (e.g. phase/) so iso_cleanup.go and proxmox/proxmox.go both reference proxmox.StateRunning, or (b) accept that iso_cleanup parses pvesh JSON (a Proxmox-specific surface) and document the duplication intentionally with a // matches proxmox.StateRunning comment. The current state has neither share-of-truth nor a written reason for the split.  
**Effort:** hours

##### `smell:9ce5434c:single-caller-poll-wrapper` — single caller poll wrapper (scaffolding — verify intent only)

**Status:** not started  
**Severity:** suggestion  
**Cluster:** helper-package-no-value  
**Evidence:** `internal/distribution/okd/phase/kubectl.go:47-49`  
**Problem:** OcPollOutput is a one-line wrapper that calls OcPollOutputInterval with interval=0. OcPollOutputInterval is itself only called from this wrapper and from kubectl_test.go. Two near-identical methods on BasePhase where one variadic-option-style helper would suffice. (Kept as 'suggestion' per MEMORY.md scaffolding rule — the test-only OcPollOutputInterval is a test-family helper.)  
**Fix:** Either keep both as a deliberate test-injection seam (current state — explicitly tag the test-only purpose in OcPollOutputInterval's doc), or fold the two into a single OcPollOutput with a poll-interval option struct. Current naming hints at API symmetry, but only OcPollOutput has production callers, so it is functionally a test-family helper.  
**Effort:** hours

#### audit-dependencies

##### `dep:33ef32bf:atotto-clipboard-stale` — atotto clipboard stale

**Status:** not started  
**Severity:** minor  
**Cluster:** maintenance-signal  
**Evidence:** `go.mod:24-24`  
**Problem:** `github.com/atotto/clipboard v0.1.4` is dated 2021-02-24 (5+ years), v0.x, BSD-3-Clause-style permissive license. Pulled transitively via `charm.land/bubbles/v2/textinput` for paste-into-input support. Bus-factor 1 (single maintainer @atotto). Charm libs depend on it directly so okdctl has no agency over the choice — it stays as long as bubbles does. Recorded for traceability; the abandonment plan is `wait for charm to swap or fork their dep tree`.  
**Fix:** No action. okdctl has no direct usage; the dep is a transitive of `charm.land/bubbles/v2`. Track via the same charm-libs pin policy already in CLAUDE.md §must-preserve (`Charm libs (charm.land/*) — intentional UI stack; don't propose swapping`). Re-evaluate if charm.land/bubbles releases a version that drops the clipboard dep.  
**Effort:** hours

##### `dep:33ef32bf:gorilla-websocket-stale` — gorilla websocket stale

**Status:** not started  
**Severity:** minor  
**Cluster:** maintenance-signal  
**Evidence:** `go.mod:41-41`  
**Problem:** `github.com/gorilla/websocket v1.4.2` is dated 2020-03-19 — six years old at audit time. Pulled transitively via `github.com/luthermonson/go-proxmox` for shell/console websocket support. CLAUDE.md §dependencies explicitly addresses this (`okdctl does not reach it — wizard uses REST discovery only`), so the dep is documented as transitive-weight only and the abandonment plan defers to go-proxmox migrating to `coder/websocket`. This finding is recorded so the entry surfaces in the audit history each run, but no action is required.  
**Fix:** No action. CLAUDE.md §dependencies already documents the policy: `Safe to keep until go-proxmox migrates to coder/websocket, at which point take the bump without local code changes`. Re-evaluate every six months alongside the go-proxmox v0.4.x abandonment plan.  
**Effort:** hours

##### `dep:33ef32bf:proxmox-v0x-bus-factor` — proxmox v0x bus factor

**Status:** not started  
**Severity:** minor  
**Cluster:** maintenance-signal  
**Evidence:** `go.mod:14-14`  
**Problem:** `github.com/luthermonson/go-proxmox v0.4.1` (released 2026-04-03) is the only path to Proxmox VE node discovery (1 file: internal/tui/wizard/steps/proxmox_discovery.go). v0.x means minor bumps may break the API; bus-factor 1. CLAUDE.md §dependencies names this as the canonical v0.x exposure with a documented ~200 LOC REST-only fallback. The dep also drags in 12 transitives (buger/goterm, jinzhu/copier, diskfs/go-diskfs, djherbis/times, gorilla/websocket, etc.) for narrow REST use. Umbrella entry for traceability.  
**Fix:** No action this run. Track go-proxmox releases each audit cycle (currently v0.4.1, last commit 2026-04-03). When a v1.0 lands, evaluate the bump. If go-proxmox is abandoned for >12 months, execute the CLAUDE.md fallback: replace with a hand-rolled REST-only client (~200 LOC) — drops 12 transitive packages and removes the v0.x exposure. Severity is `minor` not `blocker` because the abandonment plan is documented and recent (2026-04-03) commit activity shows the upstream is still alive.  
**Effort:** hours

##### `dep:33ef32bf:xo-terminfo-untagged` — xo terminfo untagged

**Status:** not started  
**Severity:** minor  
**Cluster:** maintenance-signal  
**Evidence:** `go.mod:57-57`  
**Problem:** `github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e` is an untagged pseudo-version from 2022-09-10 (3+ years), MIT-licensed, pulled transitively via `charm.land/lipgloss/v2 -> github.com/charmbracelet/colorprofile`. No upstream releases since 2022 means new Go versions / new terminal escape sequences land downstream without an upstream cut. Same pattern as gorilla/websocket and atotto/clipboard: okdctl has no agency, charm-libs control the pin.  
**Fix:** No action. Same policy as atotto-clipboard-stale: charm-libs control the choice. Re-evaluate if charm.land/lipgloss/v2 swaps or vendors the terminfo lookup.  
**Effort:** hours

##### `dep:b803fcb7:tflint-action-no-version-trailer` — tflint action no version trailer

**Status:** not started  
**Severity:** suggestion  
**Cluster:** pin-stability  
**Evidence:** `.github/workflows/ci.yml:101-101`  
**Problem:** `terraform-linters/setup-tflint@b480b8fcdaa6f2c577f8e4fa799e89e756bb7c93 # v6.2.2` is SHA-pinned correctly, but the convention CLAUDE.md §dependencies states is `uses: owner/action@<40-hex-sha> # vX.Y.Z` — every other action in this repo's workflows follows it. This one is the only `setup-tflint` line and follows the pattern, so the pin is correct. Re-checked: pin is valid. NO ACTION. Recorded for audit-history completeness as a pin-audit walkthrough confirmation.  
**Fix:** No action. The pin matches the SHA + version-trailer convention. This row exists so future audit runs can trace the pin-audit decision.  
**Effort:** hours

##### `dep:33ef32bf:dup-log-engines` — dup log engines

**Status:** not started  
**Severity:** suggestion  
**Cluster:** duplicate-engine — seam→audit-modernization  
**Evidence:** `go.mod:11-66`  
**Problem:** Four log engines link into the binary: stdlib `log/slog` (canonical), `charm.land/log/v2 v2.0.0` (one call site, internal/tui/logger.go), `k8s.io/klog/v2` and `go-logr/logr` (transitive via k8s.io/api). klog/logr are unavoidable while k8s.io/api is direct. charm.land/log/v2 exists only to color level prefixes; a hand-rolled slog.Handler in internal/logutil (~40 LOC, lipgloss already direct) could replace it and drop charm.land/log/v2 + go-logfmt/logfmt from the binary.  
**Fix:** Optional: replace `charm.land/log/v2` with a hand-rolled `slog.Handler` in `internal/logutil` that colors level prefixes via lipgloss (already a direct dep). Drops charm.land/log/v2 + go-logfmt/logfmt from the binary. Risk: bubbletea TUI integration relies on the styled output — verify visual parity in dev/staging before merge. Keep `log/slog` + the k8s.io transitives as-is.  
**Effort:** hours

##### `dep:33ef32bf:dup-yaml-engines` — dup yaml engines

**Status:** not started  
**Severity:** suggestion  
**Cluster:** duplicate-engine  
**Evidence:** `go.mod:20-59`  
**Problem:** Three YAML decoders link into the release binary: `sigs.k8s.io/yaml v1.6.0` (direct, used by internal/{addon,cli,config}), `go.yaml.in/yaml/v2 v2.4.3` (transitive — sigs.k8s.io/yaml's parser), and `go.yaml.in/yaml/v3 v3.0.4` (transitive — `cobra/doc` in cmd/okdctl-gen-docs). The v2 and v3 paths are not callable from okdctl code. sigs.k8s.io/yaml is must-preserve. The gen-docs v3 tax is avoidable by gating cobra/doc behind a `//go:build docs` tag.  
**Fix:** Optional: gate the cobra/doc import behind a build tag (`//go:build docs`) so `go.yaml.in/yaml/v3` does not enter the release binary's linker graph; today it is pulled in because `cmd/okdctl-gen-docs` lives in the same module. The v2 path stays — it is sigs.k8s.io/yaml's runtime parser. No change to direct deps.  
**Effort:** hours

##### `dep:33ef32bf:godotenv-license-filename` — godotenv license filename

**Status:** not started  
**Severity:** suggestion  
**Cluster:** license-compat  
**Evidence:** `go.mod:13-13`  
**Problem:** `github.com/joho/godotenv@v1.5.1` ships its license under the British-English filename `LICENCE` (not `LICENSE`). Naïve license scanners (`find . -iname LICENSE`, some SBOM tools) will report a missing-license false positive. The file itself is a clean MIT header (`Copyright (c) 2013 John Barton  MIT License ...`). Flagging because okdctl's release pipeline runs `syft` for SBOMs (.goreleaser.yaml `sboms` section) and downstream apt/rpm packagers may use simpler scanners; a one-line note in CLAUDE.md or the SBOM verification step prevents future false-positive churn.  
**Fix:** Either (a) drop godotenv per `dep:33ef32bf:transitive-narrow-godotenv` (then the issue evaporates), or (b) add a one-line note in `CLAUDE.md §dependencies` documenting that `godotenv` ships LICENCE-with-a-C, so future SBOM scanner work knows it is intentional.  
**Effort:** hours

##### `dep:33ef32bf:transitive-narrow-godotenv` — transitive narrow godotenv

**Status:** not started  
**Severity:** suggestion  
**Cluster:** transitive-weight — seam→audit-modernization  
**Evidence:** `go.mod:13-13`  
**Problem:** `github.com/joho/godotenv v1.5.1` is a direct dep used at exactly one call site (`internal/credentials/envfile.go:132`: `godotenv.Load(path)`). The library is small (~400 LOC, MIT-licenced — file is `LICENCE`, British spelling, but a valid MIT header on read). Replacing it would be ~30 LOC of `bufio.Scanner` + `os.Setenv`. Suggesting because okdctl's `.env` consumption is narrow (single key=value reader, no template expansion, no overrides) and the dep is the kind of thing CLAUDE.md §dependencies asks to question (`check whether Go 1.25 stdlib covers it`).  
**Fix:** Optional: replace `godotenv.Load` with a ~30 LOC `bufio.Scanner` reader in `internal/credentials/envfile.go` that handles `key=value`, comments, and `os.Setenv` (skipping already-set keys to match godotenv's no-overwrite semantics — already documented in the comment at envfile.go:130). Drops one direct dep. Seam to audit-modernization since the fix is a stdlib swap, not a dep policy issue. NOTE: godotenv's LICENSE file is named `LICENCE` (British spelling); this trips the naïve `find LICENSE` license-scanner pattern. Confirmed valid MIT.  
**Effort:** hours

##### `dep:33ef32bf:transitive-narrow-uuid` — transitive narrow uuid

**Status:** not started  
**Severity:** suggestion  
**Cluster:** transitive-weight — seam→audit-modernization  
**Evidence:** `go.mod:12-12`  
**Problem:** `github.com/google/uuid v1.6.0` is a direct dep used in three files (`internal/cli/{deploy,destroy,debug_bundle}.go`) at three call sites — all `uuid.NewString()` for run-IDs / bundle-IDs. UUID v4 from `crypto/rand` is ~10 LOC stdlib. Flagging as suggestion only because the dep is small, BSD-3-clause, well-maintained, and the savings are marginal (one dep entry). Worth listing because it falls inside CLAUDE.md's `check whether stdlib covers it` policy.  
**Fix:** Optional: replace with `crypto/rand` + `fmt.Sprintf` UUIDv4 helper in `internal/system` (~15 LOC) and drop the dep. Risk is low because UUIDs are non-load-bearing here (run-ID telemetry only, not security tokens). Seam to audit-modernization. Lower priority than godotenv because google/uuid is a stable, widely-vendored dep with no maintenance signal issues.  
**Effort:** hours

#### audit-documentation

##### `doc:dd75bdeb:exported-doc-missing-type` — exported doc missing type

**Status:** not started  
**Severity:** minor  
**Cluster:** exported-doc — seam→audit-api-design; related: api:dd75bdeb:export-no-caller  
**Evidence:** `internal/distribution/okd/postinstall/context.go:4-10`  
**Problem:** Exported PostInstallContext struct has no doc comment despite a //nolint:revive that suppresses the lint enforcement of the same rule. CLAUDE.md §code-comments rule 2 requires docs on exported API with non-obvious behavior; the five fields (ClusterHealth, KubeVIPVerified, KubeVipIP, BootstrapCleaned, DNSDeployed) are state flags whose semantics are not evident from the names — what does KubeVIPVerified=false mean, who clears these, and is the zero-value valid?  
**Fix:** Add a type doc above the //nolint:revive directive: 'PostInstallContext threads per-step verification state through the post-install phase: which check has run and what it found. Fields are zero-valued until the corresponding step populates them; nil ClusterHealth means cluster-health-check did not run yet.' Also doc each field with one line on what the populator step is.  
**Effort:** hours

##### `doc:35abd54e:doc-claim-vs-impl-drift` — doc claim vs impl drift (scaffolding — verify intent only)

**Status:** not started  
**Severity:** suggestion  
**Cluster:** exported-doc — seam→audit-api-design; related: api:35abd54e:export-no-caller-scaffolding  
**Evidence:** `internal/credentials/proxmox.go:13-24`  
**Problem:** Source enum doc claims SourceConfig is a reachable state ('SourceConfig means the credential came from the YAML config file') but the implementation never sets creds.Source = SourceConfig — the comment at L213 explicitly says 'Config-file credentials are NO LONGER a fallback'. Doc and implementation disagree, which makes the type misleading: callers reading the doc would write switch arms that are dead code.  
**Fix:** Either (a) drop SourceConfig + its String() arm and accept this is env-or-nothing per the L213 comment, or (b) reinstate config-file fallback wired up to set creds.Source = SourceConfig. Option (a) is consistent with the security stance the rest of the package takes (no string residue of px.Password in heap). Per MEMORY.md scaffolding rule, prefer 'verify intent' (grep roadmap.md, ask the owner) before deletion. If kept, document it as reserved-for-future on the enum value.  
**Effort:** hours

#### audit-tests

##### `tst:79e2cbc4:resolver-circular-deps-untested` — resolver circular deps untested

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-79e2cbc4-resolver-circular  
**Severity:** major  
**Cluster:** destructive-untested  
**Evidence:** `internal/addon/resolver.go:12-70`  
**Problem:** Resolve implements Kahn's topological sort over addon dependencies. The function is the gate between operator-declared addons and install ordering — a false negative on circular dependency would cause Manager.InstallAll to deadlock or install in the wrong order (e.g. SecretStore before Flux that depends on it). No test today covers (a) circular detection, (b) priority ordering, (c) missing-dependency error.  
**Fix:** Add resolver_test.go with a tiny stubAddon: cases — (1) no deps + same priority sorts by name; (2) priority breaks ties; (3) A→B→C orders C before B before A; (4) missing dep returns error containing "depends on" and addon names; (5) circular A→B→A returns "circular dependency detected". Pure logic — a fakeAddon{name, deps, priority} struct + addon.Addon interface stub is enough.  
**Effort:** hours

##### `tst:de572c63:validate-config-name-no-test` — validate config name no test

**Status:** not started  
**Severity:** major  
**Cluster:** trust-boundary-untested  
**Evidence:** `internal/distribution/okd/dns/dnsmasq.go:44-52`  
**Problem:** validateConfigName is the trust boundary between cluster.Name (operator-supplied YAML) and a path written under /etc/dnsmasq.d/. The regex enforces ^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$ but no test asserts it actually rejects the obvious attacker shapes (empty, ../escape, dots, slashes, null bytes, unicode, leading hyphen, length > 64).  
**Fix:** Add TestValidateConfigName to a new dnsmasq_test.go: accept ("okd-prod", "a", "a1", strings.Repeat("a", 64)); reject ("", "-leading", "a/b", "../escape", "a.b", strings.Repeat("a", 65), "\u00e9", "a\x00b"). Pure function — no fixtures needed.  
**Effort:** hours

##### `tst:696d6b0e:remove-fcos-iso-from-proxmox-no-test` — remove fcos iso from proxmox no test

**Status:** not started  
**Severity:** major  
**Cluster:** destructive-untested  
**Evidence:** `internal/distribution/okd/phase/iso_cleanup.go:218-265`  
**Problem:** RemoveFCOSISOFromProxmox is the destroy-phase remote rm-f orchestrator: SSH find on the Proxmox host, refuseUnsafeISOPath gate, anyVMReferencesISO fail-closed check, then ssh rm -f. The unit-shaped helpers (refuseUnsafeISOPath, validateISODir, parseVMIDsFromSummary) are tested in iso_cleanup_test.go but the orchestrator itself isn't. A regression that swaps the order — call rm before checking inUse — would yank ISOs out of running VMs and crash them.  
**Fix:** Refactor SSHRun calls behind a small interface so a test can inject a fake transport. The phase already accepts *executor.Executor — install a fake `ssh` script in PATH per kubectl_test.go's installFakeOC pattern that responds to find / pvesh qemu list / pvesh qemu config / rm -f based on argv. Test: (1) ISO with no VM reference is removed; (2) ISO referenced by running VM is skipped; (3) refuseUnsafeISOPath rejects /etc/passwd before rm fires.  
**Effort:** hours

##### `tst:696d6b0e:validate-proxmox-name-no-test` — validate proxmox name no test

**Status:** not started  
**Severity:** major  
**Cluster:** trust-boundary-untested  
**Evidence:** `internal/distribution/okd/phase/iso_cleanup.go:52-64`  
**Problem:** validateProxmoxName gates pvesh interpolation over ssh — the function comment names this as defense-in-depth for hand-edited YAML. The byte-by-byte allowlist enforces [A-Za-z0-9_-] but no test asserts the actual rejection set: empty string, leading digit, dot, slash, semicolon, backtick, dollar sign, space, unicode, null byte. Without coverage, a future refactor that swaps to a regex with the wrong anchor (^[a-z]+ vs ^[a-z]+$) silently relaxes the gate.  
**Fix:** Add TestValidateProxmoxName cases: accept ("pve", "pve-1", "node_a", "PVE0"); reject ("", "1pve", "pve.example", "pve/etc", "pve;rm", "pve`id`", "pve$(id)", "pve space", "pvé", "pve\x00"). Pure function — no fixtures.  
**Effort:** hours

##### `tst:451be4fa:chown-tree-error-aggregation-untested` — chown tree error aggregation untested

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-451be4fa-chown-tree  
**Severity:** major  
**Cluster:** canonical-helper-untested  
**Evidence:** `internal/system/elevation.go:111-131`  
**Problem:** ChownTreeToInvokingUser is called with `defer` from cli/destroy.go, cli/cleanup.go, and cli/deploy.go to put the workdir back into the invoking user's hands after a sudo'd run. The function uses errors.Join to aggregate per-entry chown failures and continues the walk so a single bad symlink doesn't strand the whole tree root-owned. The existing test only covers the no-SUDO_UID short-circuit. The error-aggregation path — the actual point of the function — is untested.  
**Fix:** Add TestChownTreeToInvokingUser_AggregatesErrors that (1) sets SUDO_UID/GID to the current process's uid/gid (no-op chown), (2) creates a tree with a regular file and a broken symlink, (3) asserts the walk completes and errors.Join returns nil for the no-op case. Then a parallel test that uses an unprivileged uid (e.g. 65534) and asserts errors.Join wraps multiple errs but the walk still visits all entries (count via a probe).  
**Effort:** hours

##### `tst:97cb8adf:run-captured-no-test` — run captured no test

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-97cb8adf-run-captured  
**Severity:** major  
**Cluster:** canonical-helper-untested  
**Evidence:** `internal/system/exec.go:21-33`  
**Problem:** system.RunCaptured is the canonical 'run a command, surface stderr in the err' helper used by every firewall, dnsmasq, and netutil call site (15+ sites). It has no test. The stderr-into-err shape is the load-bearing detail — consumers errors.As(err, &cfgErr) on the returned error and rely on the wrapped stderr being human-readable. A regression that drops the stderr-prefix or uses fmt.Errorf without %w breaks every error message downstream.  
**Fix:** Add TestRunCaptured cases: (1) command exits 0 → nil; (2) command exits 1 with stderr="oops" → err contains "oops" and errors.Is unwraps to *exec.ExitError; (3) command exits 1 with empty stderr → err carries the bin name only; (4) ctx cancel returns ctx err. Use a fake script in PATH per kubectl_test.go's installFakeOC pattern.  
**Effort:** hours

##### `tst:40d315ad:git-host-no-test` — git host no test

**Status:** not started  
**Severity:** minor  
**Cluster:** trust-boundary-untested  
**Evidence:** `internal/addon/catalog/flux/flux.go:389-415`  
**Problem:** gitHost parses operator-supplied addons.flux.settings.repository (which crosses an external trust boundary at config-load time) and feeds the result to ssh-keyscan host. ssh-keyscan does not interpret the host as a shell argument, but the wider invariant (host must be a real DNS name, not an arbitrary string) is unchecked. No test asserts the parser handles edge cases: ssh:// without user, ssh://[ipv6]:port, scp-style with port, malformed.  
**Fix:** Add TestGitHost (alongside TestBuildFluxDeployKeySecret in a new flux_test.go): accept ssh://git@github.com/o/r, https://github.com/o/r, git@github.com:o/r, ssh://git@host:2222/o/r → all return correct host. Reject "", "   ", "no-host", "://nope", "http://". Pure function.  
**Effort:** hours

##### `tst:0b188cab:retry-default-cancel-untested` — retry default cancel untested

**Status:** not started  
**Severity:** minor  
**Cluster:** canonical-helper-untested  
**Evidence:** `internal/addon/helpers.go:23-41`  
**Problem:** addon.RetryDefault wraps wait.ExponentialBackoffWithContext with hardcoded count/backoff/factor/jitter/cap. The function is called by every addon Install and createDeployKeySecret. There is no test asserting (a) the retry budget actually fires, (b) ctx cancellation aborts mid-retry, (c) all-failures-retried semantics (the comment names that errors are intentionally not surfaced as 'don't retry').  
**Fix:** Add TestRetryDefault using testing/synctest: (1) fn that returns nil on attempt N — assert RetryDefault returns nil and counter==N (≤ Steps); (2) fn that always errors — assert RetryDefault returns the timeout-shaped wait.ErrWaitTimeout and counter==Steps; (3) ctx.Cancel mid-retry — assert RetryDefault returns ctx err.  
**Effort:** hours

##### `tst:26a430ee:requires-root-dryrun-escape-untested` — requires root dryrun escape untested

**Status:** not started  
**Severity:** minor  
**Cluster:** canonical-helper-untested  
**Evidence:** `internal/cli/elevation.go:33-46`  
**Problem:** requiresRoot returns false when --dry-run is set so 'okdctl destroy --dry-run' does NOT prompt for sudo. The flag-lookup uses cmd.Flags().GetBool("dry-run") which silently falls through (err != nil → false branch) for commands without the flag. There is no test asserting (a) --dry-run on destroy/cleanup escapes the gate; (b) a command without a dry-run flag still triggers the gate.  
**Fix:** Add elevation_test.go: build cobra commands with/without dry-run flag and rootRequiredCmds membership; call requiresRoot, assert: destroyCmd with dry-run=true → false; destroyCmd with dry-run=false → true; deployCmd without dry-run flag set → true (rootRequiredCmds membership); statusCmd → false. Pure function on cobra.Command.  
**Effort:** hours

##### `tst:08c49fc4:update-ingress-confirm-callback-untested` — update ingress confirm callback untested

**Status:** not started  
**Severity:** minor  
**Cluster:** destructive-untested  
**Evidence:** `internal/cli/update_ingress.go:107-124`  
**Problem:** runUpdateIngress wires a ConfirmConversion callback that calls promptForConfirmation when the user did NOT pass --yes. The conversion is destructive (oc delete + oc create with rollback). There is no test asserting (a) --yes auto-confirms; (b) without --yes, a 'n' answer returns false; (c) prompt error returns false (callback returns false on err — silently aborts the conversion).  
**Fix:** Refactor the inline closure into a small helper buildConvertConfirm(ctx, yes bool) func([]string) bool. Add test cases: yes=true returns true regardless; yes=false delegates to promptForConfirmation. Stub stdin via os.Pipe to feed 'y\n' / 'n\n' / EOF.  
**Effort:** hours

##### `tst:35abd54e:env-method-zeroize-survives-no-explicit-test` — env method zeroize survives no explicit test

**Status:** not started  
**Severity:** minor  
**Cluster:** cred-path-untested  
**Evidence:** `internal/credentials/proxmox.go:117-138`  
**Problem:** Env() builds os/exec env strings via string([]byte) — the immutable copy survives caller Zeroize. Existing test TestProxmoxCredentials_Env/password_backing_not_shared_with_env_string covers password but NOT APIToken. The contract is identical (string copy) but if a future refactor replaces string(c.APIToken) with bytesconv.BString or similar zero-copy trick, the env entry would become Zeroize-fragile. No test asserts the APIToken survives a wipe.  
**Fix:** Extend TestProxmoxCredentials_Env's password_backing subtest into two parallel subtests: api_token_backing_not_shared and password_backing_not_shared. Each wipes the underlying []byte after Env() and asserts the env entry still carries the original literal. Pure mechanical extension of the existing test.  
**Effort:** hours

##### `tst:bdf5a873:work-directory-preserve-config-untested` — work directory preserve config untested

**Status:** not started  
**Severity:** minor  
**Cluster:** destructive-untested  
**Evidence:** `internal/distribution/okd/cleanup/artifacts.go:62-92`  
**Problem:** WorkDirectory has two distinct branches: preserveConfig=true keeps okdctl.yaml at the workDir root and removes only sub-trees, preserveConfig=false rm -rfs the whole tree. There is no test for the preserveConfig=true branch despite the artifact-test file existing. A regression that swaps the branches (e.g. preserveConfig accidentally inverted) silently destroys operator-edited okdctl.yaml during a partial cleanup.  
**Fix:** Add TestWorkDirectory_PreservesConfigYaml that seeds workDir with okdctl.yaml + tmp/ + downloads/ + custom-isos/, calls WorkDirectory(ctx, workDir, true, log), then asserts okdctl.yaml is still present at the root and the four sub-trees are gone.  
**Effort:** hours

##### `tst:de572c63:dnsmasq-config-path-no-test` — dnsmasq config path no test

**Status:** not started  
**Severity:** minor  
**Cluster:** trust-boundary-untested — related: tst:de572c63:validate-config-name-no-test  
**Evidence:** `internal/distribution/okd/dns/dnsmasq.go:94-101`  
**Problem:** DnsmasqConfigPath is the public-API gate that builds /etc/dnsmasq.d/<name>.conf paths consumed by cleanup.Dnsmasq's _ = os.RemoveAll(configPath). It validates the name through validateConfigName but no test asserts (a) a clean okd-prod returns the canonical path, (b) ../escape returns an error not a path, (c) the returned path is always under the configured dnsmasqConfigDir (no traversal).  
**Fix:** Once the dns package gets its first test file (per tst:d7ce9d16), add TestDnsmasqConfigPath: (1) okd-prod → /etc/dnsmasq.d/okd-prod.conf; (2) ../etc/passwd → error; (3) empty → error.  
**Effort:** hours

##### `tst:73ad30ef:resolve-cluster-vip-no-test` — resolve cluster vip no test

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-73ad30ef-resolve-cluster-vip  
**Severity:** minor  
**Cluster:** canonical-helper-untested  
**Evidence:** `internal/distribution/okd/phase/helpers.go:24-30`  
**Problem:** ResolveClusterVIP is the canonical 'resolve VIP from cfg' wrapper used by destroy, postinstall, dns, update_ingress (5 sites per the comment). It thin-wraps netutil.ResolveVIP with a fixed error prefix. netutil.ResolveVIP IS tested but the prefix wrap and the netutil-to-config field mapping (cfg.Networking.Bastion.VIP first, then StaticIP.Start) are not.  
**Fix:** Add TestResolveClusterVIP: (1) explicit VIP wins; (2) static-IP-derived VIP when no explicit; (3) malformed VIP wraps with "failed to resolve VIP" prefix and underlying error stays errors.Is-able. Pure function — no fixtures.  
**Effort:** hours

##### `tst:9ce5434c:oc-output-typed-exit-error-untested` — oc output typed exit error untested

**Status:** not started  
**Severity:** minor  
**Cluster:** canonical-helper-untested  
**Evidence:** `internal/distribution/okd/phase/kubectl.go:29-42`  
**Problem:** OcOutput is the third canonical Oc* helper alongside OcResourceExists and OcPollOutput. Tests cover the latter two but not OcOutput's typed *executor.ExitError return. Callers (e.g. update_ingress, addon catalog) errors.As against ExitError to read ExitCode; a regression that returns a plain fmt.Errorf would break the typed-error contract silently.  
**Fix:** Add TestOcOutput cases reusing installFakeOC: (1) OC_FAKE_MODE=exists → trimmed stdout; (2) OC_FAKE_MODE=error → errors.As to *executor.ExitError, ExitCode==1, Stderr contains "cluster unreachable"; (3) ctx cancel → propagates ctx error.  
**Effort:** hours

##### `tst:27088eab:ssh-run-no-test` — ssh run no test

**Status:** not started  
**Severity:** minor  
**Cluster:** canonical-helper-untested  
**Evidence:** `internal/distribution/okd/phase/ssh.go:30-41`  
**Problem:** SSHRun wraps every remote command in destroy / setup / iso-cleanup with a fixed flag set (StrictHostKeyChecking=accept-new + BatchMode=yes). The flag set is load-bearing — a future caller that copies+modifies it could downgrade to AcceptHostKey=no (MITM). There is no test asserting the canonical flags appear in the exec.Command argv.  
**Fix:** Install a fake `ssh` script in PATH that prints argv to stdout. Call SSHRun with a fixed host+cmd, parse the resulting Result.Stdout, assert the canonical flag set and root@host appear verbatim. Mirrors kubectl_test.go's installFakeOC pattern.  
**Effort:** hours

##### `tst:b804b2ec:cleanup-bootstrap-plan-file-leak-untested` — cleanup bootstrap plan file leak untested

**Status:** not started  
**Severity:** minor  
**Cluster:** destructive-untested  
**Evidence:** `internal/distribution/okd/postinstall/bootstrap.go:17-66`  
**Problem:** CleanupBootstrap defers system.SafeRemove(planPath) — the comment specifically names a regression where a leftover .tfplan file 'refused to overwrite' on the next run. There is no test asserting the defer fires on every error path: (1) plan failure, (2) apply failure, (3) success. A refactor that moves SafeRemove out of defer (e.g. only calling it on success) re-introduces the named regression silently.  
**Fix:** Inject a fake `terraform` binary in PATH that responds to init/plan/apply per OC_FAKE_MODE-style env var. Test (1) all-success: planPath gone after; (2) plan exits 1: planPath gone (the regression case); (3) plan succeeds, apply exits 1: planPath gone. Reuse phase.NewBasePhase + executor.New() pattern from existing destroy/helpers_test.go.  
**Effort:** hours

##### `tst:eb479d86:upload-iso-via-scp-no-test` — upload iso via scp no test

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-eb479d86-upload-iso-scp  
**Severity:** minor  
**Cluster:** trust-boundary-untested  
**Evidence:** `internal/distribution/okd/setup/upload.go:42-51`  
**Problem:** uploadISOsViaSCP composes scp argv from operator-supplied isoFiles + user/host/remotePath. Local file paths are joined into one scp invocation 'scp -o StrictHostKeyChecking=accept-new f1 f2 ... root@host:/path/'. Argv is os/exec — no shell interpolation — but the lack of a test means a future refactor that adds an `sh -c` for retry logic would silently introduce CWE-78 via filenames. There is no test asserting the argv shape.  
**Fix:** Refactor uploadISOsViaSCP to take an Executor injection and add a test using a fake `scp` script in PATH that prints argv to stdout; assert (a) StrictHostKeyChecking flag present, (b) ISO paths appear as separate argv entries (not joined), (c) destination is exactly user@host:remotePath/ with trailing slash, (d) filenames containing spaces survive as one argv entry.  
**Effort:** hours

##### `tst:5e892064:download-checksum-fetch-paths` — download checksum fetch paths

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-5e892064-download-checksum  
**Severity:** minor  
**Cluster:** trust-boundary-untested  
**Evidence:** `internal/download/checksum.go:59-111`  
**Problem:** FetchChecksum has reasonable test coverage of the parser (TestFetchChecksum) but the caller-side verifyDownloadedFile path (which os.Removes the downloaded file on mismatch) is untested. A mismatch leaves the user with no artifact and a generic error — testing locks that the file is actually removed (so the next run re-downloads instead of trusting a corrupt cache).  
**Fix:** Add TestVerifyDownloadedFile cases: (1) empty expected → nil, file untouched; (2) match → nil, file untouched; (3) mismatch → err, file gone. Pure function tests against a t.TempDir.  
**Effort:** hours

##### `tst:7b2829bb:run-streamed-checked-no-test` — run streamed checked no test

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-7b2829bb-run-streamed  
**Severity:** minor  
**Cluster:** canonical-helper-untested  
**Evidence:** `internal/executor/executor.go:300-311`  
**Problem:** executor.RunStreamedChecked is the canonical 'stream stdout+stderr live AND return ExitError on non-zero' helper used by terraform.Plan/Apply/Destroy. The buildEnv allowlist and ringbuffer are tested, but the Streamed-Checked flow itself isn't — there's no assertion that on non-zero exit the returned err is *ExitError carrying the ringbuffer tail of stderr. A regression that returns nil on exit==0 with a write error to e.Stdout silently swallows the failure.  
**Fix:** Add TestRunStreamedChecked cases parallel to TestBuildEnv_EndToEndWithEcho: (1) command exits 0 → result.ExitCode==0, err==nil, stdout streamed to e.Stdout AND captured in result.Stdout; (2) command exits 1 → err is *ExitError, ExitError.Stderr contains the last lines (ring tail); (3) ctx cancel mid-stream returns ctx error wrapped via cmd.Run.  
**Effort:** hours

##### `tst:4c092fce:terraform-build-var-args-untested` — terraform build var args untested

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-4c092fce-tf-var-args  
**Severity:** minor  
**Cluster:** canonical-helper-untested  
**Evidence:** `internal/infrastructure/terraform/terraform.go:172-192`  
**Problem:** buildVarArgs sorts vars alphabetically (slices.Sorted(maps.Keys(vars))) before composing -var k=v. Deterministic ordering is the contract — terraform's plan output diffing relies on stable arg order for reproducible plans. There is no test asserting the sorted order, so a refactor swapping to a non-sorted iteration silently breaks plan-output stability.  
**Fix:** Add TestExecutor_BuildVarArgs_DeterministicOrder: vars := {"z":"3", "a":"1", "m":"2"}; call buildVarArgs("", vars); assert returned slice matches ["-var","a=1","-var","m=2","-var","z=3"]. Also asserts varFile-not-found path issues a Warn and skips -var-file.  
**Effort:** hours

##### `tst:e552bb7d:remove-secondary-ip-no-test` — remove secondary ip no test

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-e552bb7d-remove-secondary-ip  
**Severity:** minor  
**Cluster:** destructive-untested  
**Evidence:** `internal/netutil/iface.go:17-48`  
**Problem:** RemoveSecondaryIP is called from postinstall.RemoveHAProxy and cleanup.HAProxy to strip the bastion VIP. The flow is (ip addr show → grep ip+/ → nmcli connection modify -ipv4.addresses → nmcli device reapply). The early-return on 'IP not present' prevents needless reconfigure; a test would lock that this short-circuit fires (rather than silently issuing a remove on every cleanup).  
**Fix:** Install fake `ip` and `nmcli` scripts in PATH per kubectl_test.go's installFakeOC pattern. Cases: (1) ip addr output contains "192.168.1.10/24" + ip arg "192.168.1.10" → fast-path nil, no nmcli invocation; (2) ip addr empty + ip arg → nmcli connection modify + nmcli device reapply called; (3) `ip` exits 1 → returns wrapped error. Verify fake invocation count via a counter file.  
**Effort:** hours

##### `tst:881d089e:runlock-write-failure-untested` — runlock write failure untested

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-881d089e-runlock-write  
**Severity:** minor  
**Cluster:** canonical-helper-untested  
**Evidence:** `internal/runlock/runlock.go:29-54`  
**Problem:** Acquire's existing test covers create and conflict but not the diagnostic-write path: the function truncates the file, writes PID/VERB/TIME diagnostics, and ignores the write errors with `_, _ = fmt.Fprintf(...)`. A regression that fails the truncate-then-write would leave stale diagnostics from a previous holder, breaking the conflict error message ('another okdctl process holds the project lock: <stale>').  
**Fix:** Extend the existing TestAcquireAndRelease — after acquire, read the file and assert the diagnostics carry PID=<currentPid>, VERB=deploy, TIME=<recent>. A second test acquires after a conflict and asserts the error string contains the prior holder's PID (the lock file body was actually read).  
**Effort:** hours

##### `tst:e3782ee7:expand-path-no-test` — expand path no test

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-e3782ee7-expand-path  
**Severity:** minor  
**Cluster:** canonical-helper-untested  
**Evidence:** `internal/system/fs.go:154-165`  
**Problem:** ExpandPath resolves leading ~/ via InvokingUserHomeDir so config-supplied PullSecret / SSHPublicKey paths (~/pull-secret.json) work correctly under sudo re-exec. The function gates a config-validation site (validators.go validateFiles) and an ignition-rendering site. There is no test asserting (a) ~/foo expands when SUDO_USER points at a real user, (b) ~ alone (no slash) is left intact, (c) absolute and relative paths pass through unchanged.  
**Fix:** Add TestExpandPath cases: (1) SUDO_USER=current, ~/x → /home/<user>/x; (2) bare ~ → ~ unchanged; (3) ~user/foo → ~user/foo (only ~/ prefix expands); (4) /abs/path → unchanged; (5) relative/path → unchanged.  
**Effort:** hours

##### `tst:e3782ee7:safe-remove-no-test` — safe remove no test

**Status:** not started  
**Severity:** minor  
**Cluster:** canonical-helper-untested  
**Evidence:** `internal/system/fs.go:146-152`  
**Problem:** system.SafeRemove is the canonical "rm -rf if exists" helper used by terraform.Cleanup, postinstall.CleanupBootstrap, and the cleanup package. It has no test. The (existence check, then RemoveAll) pattern has the obvious TOCTOU foot-gun the comment doesn't acknowledge — if a symlink is created between Stat and RemoveAll, RemoveAll follows it. A test would lock the no-op-on-missing branch and document the TOCTOU behavior so a future caller doesn't assume protection.  
**Fix:** Add TestSafeRemove subtests: missing → nil; regular file → removed; directory tree → removed recursively; symlink → target NOT followed (this asserts the TOCTOU window is small but the function does follow Stat-then-RemoveAll, so document via assertion). Stdlib testing/fstest can fake the FS for the symlink case.  
**Effort:** hours

##### `tst:98bcb208:collect-doctor-output-no-test` — collect doctor output no test

**Status:** not started  
**Severity:** suggestion  
**Cluster:** canonical-helper-untested  
**Evidence:** `internal/cli/debug_bundle_doctor.go:17-28`  
**Problem:** collectDoctorOutput re-execs the current binary as 'doctor' to embed the doctor preflight in the debug bundle. The function ignores cmd.Run's error (intentionally — failing preflight should still be in the bundle) and returns the buffer regardless. There is no test asserting (a) buffer is non-empty even when the subprocess fails, (b) os.Executable error is wrapped.  
**Fix:** Add a build-tagged test that injects a fake-self via a tiny TestMain trick: write a test binary that, when invoked with argv[1]=="doctor", prints a known string and exits 1. Call collectDoctorOutput, assert the known string is in the buffer despite the non-zero exit.  
**Effort:** hours

##### `tst:9d79b841:logged-iso-once-untested` — logged iso once untested

**Status:** not started  
**Severity:** suggestion  
**Cluster:** canonical-helper-untested  
**Evidence:** `internal/distribution/okd/setup/coreos.go:25-35`  
**Problem:** logISOFound dedups 'coreos: iso found' to once-per-base-filename across the Phase. The other coreos_test.go cases cover the JSON parsers but not this dedup behavior, so a refactor that shared the loggedISOs map across phases (a likely future change) would silently break the cardinality contract — emitting the same ISO N times again.  
**Fix:** Extend coreos_test.go with a TestLogISOFound that uses a slog.Handler-as-counter to count records keyed on iso=. Call logISOFound twice with the same path, once with another path, and assert exactly two records emitted.  
**Effort:** hours

##### `tst:f51f85bb:cidr-to-netmask-edge-no-test` — cidr to netmask edge no test

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-f51f85bb-cidr-netmask  
**Severity:** suggestion  
**Cluster:** trust-boundary-untested  
**Evidence:** `internal/netutil/ip.go:15-29`  
**Problem:** CIDRToNetmask is well-tested for typical CIDRs but missing edge-cases that cross the boundary into HAProxy/dnsmasq template substitution: /31, /32 (now passes), /1, /0 (now passes), and pathological inputs like 192.168.1.0/-1 (rejected) vs 192.168.1.0/33 (rejected). The existing test covers /0 /8 /12 /24 /32 but not /1 or /31, the off-by-one cases that template-substitution downstream could mis-render.  
**Fix:** Extend TestCIDRToNetmask: 0.0.0.0/1 → 128.0.0.0; 192.168.1.0/31 → 255.255.255.254; 10.0.0.0/30 → 255.255.255.252.  
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

Completed items are archived to **`docs/roadmap/completed-archive.md`**
to keep this file readable. The archive preserves every postmortem
verbatim — search there for incident review or pattern reuse.

**Adding a new completion:** append the entry to the archive (same
shape as existing entries — close date, PR / merge commit, terse
evidence, postmortem lesson when one exists), then flip the item's
Status in this file from `in review — PR #N` to `done`. Update the
Appendix ledger row at the bottom of this file to `**Done** (PR #N)`
so the ledger remains the canonical "is dep X done?" lookup.

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

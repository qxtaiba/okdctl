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
| `done — PR #<n>` | Merged. After merge, append a postmortem entry to `docs/roadmap/completed-archive.md` and remove the item block from this file. |
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

- **Status:** in review — PR #337
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

### Tier E — architectural deferrals from 2026-04-20 audit

Items from `.claude/audits/FULL_REPORT-2026-04-20.md` whose fix requires
material design work (new config fields, workflow changes, new
subsystems) rather than a one-file surgical edit. Filed here so
`/roadmap-pickup` can schedule them; each carries the originating
audit finding ID so diff tracking stays tight.

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

##### `api:dd75bdeb:stutter-postinstall-context` — stutter postinstall context

**Status:** deferred  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/postinstall/context.go:1-10`  
**Problem:** postinstall.PostInstallContext stutters (package.PostInstall…). The struct is already suppressed with //nolint:revive and a 'rename deferred to a dedicated refactor' note, so this finding is a reminder that the deferred rename is still pending.  
**Fix:** Rename postinstall.PostInstallContext -> postinstall.State (preferred) or postinstall.Context. Callers: phase.go:76 (distribution.NewPhaseContext(State{})), steps.go (4x pctx.Update(func(c *State) {...})), and the PhaseContext[State] type parameter.  
**Effort:** hours

##### `api:4f69fc9d:iface-fragmented-step` — iface fragmented step

**Status:** done — PR #923
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

#### audit-code-smells

#### audit-concurrency

##### `con:ae5b624c:synctest-opportunity` — synctest opportunity

**Status:** deferred  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/install/monitor.go:52-150`  
**Problem:** MonitorInstallation has a ticker-driven CSR-approval loop, a reap timer, and ctx.Done/DeadlineExceeded paths — exactly the shape testing/synctest is designed for. Currently untested because real-time tests would take minutes; the exec_test.go suite landing in this release proved the pattern works, so this is the last holdout.  
**Fix:** Extract the select-loop body into a testable helper (csrApprovalLoop(ctx, ticker, installDone, k8sClient)) and cover the three exit paths (installDone success, installDone timeout-error, ctx cancel → kill → reap) with testing/synctest — mirror the internal/system/exec_test.go shape that landed this release. Requires a k8sClient fake; audit-tests already flags that fake as missing for CSR-related coverage.  
**Effort:** hours

#### audit-dependencies

#### audit-documentation

#### audit-errors

#### audit-iac-and-shell

#### audit-modernization

#### audit-observability

#### audit-security

##### `sec:88fd3050:cred-as-string-in-config` — cred as string in config

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/config/cluster.go:107-134`  
**Problem:** ProxmoxConfig.Password and ProxmoxConfig.APIToken are typed as `string` (with `json:"-"`). The credentials.GetProxmoxCredentials legacy fallback reads them when the env path is empty (proxmox.go:213-228), converting via []byte(px.Password) — the new slice is wipeable but the original string residue persists for the Config's lifetime.  
**Fix:** Option A (safer): remove the config-file credential path entirely — env/.env is the documented mechanism and the comment already says 'never persisted'; honour that by deleting the legacy fallback branch in GetProxmoxCredentials. Option B (if kept): retype ProxmoxConfig.Password and APIToken to []byte, adjust the loader path, and Zeroize during Config teardown.  
**Effort:** hours

##### `sec:35abd54e:cred-string-copy-env` — cred string copy env

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/credentials/proxmox.go:113-134`  
**Problem:** ProxmoxCredentials.Env() builds subprocess env entries via string concatenation: "PROXMOX_VE_PASSWORD="+string(c.Password). The resulting Go string is an immutable heap copy of the password that Zeroize cannot overwrite, leaving an unwipeable residue for the entire lifetime of the returned slice (and beyond, via GC).  
**Fix:** Return a [][]byte (or a keyed byte-slice struct) for the credential-bearing entries and have the caller build cmd.Env at the final moment; or at minimum scope the Env() slice to a tight defer-clear. The current pattern violates the design intent of keeping passwords as []byte across the lifecycle.  
**Effort:** hours

##### `sec:00000005:bootstrap-oc-no-integrity` — bootstrap oc no integrity

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/release_extract.go:24-76`  
**Problem:** bootstrapOC downloads oc.tar.gz from mirror.openshift.com with no checksum or cosign signature verification. The docstring admits 'no upstream checksum is published for this URL; post-extraction binary-exists verification is the integrity gate'.  
**Fix:** Either (a) pin bootstrapOCURL to a specific release tag and ship a baked-in sha256 in the okdctl binary (matches the 'explicit versions — never @latest' rule in CLAUDE.md §Dependencies), or (b) verify a cosign signature on the tarball if Red Hat publishes one for the client tarball set, or (c) fall through to `oc adm release extract` via the distribution-packaged `openshift-client` rpm/deb instead of curl-to-bash. Document the trust decision in CLAUDE.md §security-invariants.  
**Effort:** hours

##### `sec:7b2829bb:env-append-os-environ` — env append os environ

**Status:** deferred  
**Severity:** minor  
**Evidence:** `internal/executor/executor.go:85-174`  
**Problem:** Executor now applies a defaultEnvAllowlist (good — previously-flagged broadcast of unrelated env vars is closed). But PROXMOX_ is in the prefix allowlist, so PROXMOX_VE_PASSWORD / PROXMOX_VE_API_TOKEN still reach EVERY subprocess the executor spawns — including coreutils shellouts that don't need Proxmox credentials (lsb_release, dpkg, gpg, rpm, ss, systemctl, semanage, find, rm, ssh-keyscan).  
**Fix:** Split Executor.Env into two slices: AuthEnv (credential-bearing PROXMOX_*, KUBECONFIG, GIT_*, GITHUB_TOKEN) and Env (general). Add WithAuthEnv(...) and a per-Run toggle so credential vars only reach terraform + oc + helm + sops.  
**Effort:** hours

##### `sec:d5915b0c:kubeconfig-env-leak` — kubeconfig env leak

**Status:** deferred  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/install/phase.go:151-162`  
**Problem:** SetupKubeconfig appends `KUBECONFIG=<path>` to p.Exec.Env, making the kubeconfig path visible to every subprocess the executor spawns from that point forward — including unrelated tools (helm, ssh-keyscan, lsb_release, dpkg, rpm). The kubeconfig file contains bearer credentials for the cluster.  
**Fix:** Couple with sec:7b2829bb: split Executor.Env into AuthEnv + Env, and push KUBECONFIG into AuthEnv. Only oc / helm / openshift-install invocations see AuthEnv; dpkg / rpm / lsb_release / ssh-keyscan do not.  
**Effort:** hours

#### audit-state-and-recovery

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

#### audit-tests

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

##### `tst:41a9d4eb:no-test-redact-handler` — no test redact handler

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/logutil/redact.go:30-123`  
**Problem:** RedactHandler is the canonical slog redaction middleware — CLAUDE.md §credentials-and-secrets explicitly calls it out as the mechanism "so credentials in structured attrs never reach the sink". Its direct unit tests are absent; coverage today is indirect via tui/logger_test.go.  
**Fix:** Add internal/logutil/redact_test.go with stdlib testing + bytes.Buffer + slog.NewTextHandler as the wrapped inner. Cases: (1) TestRedactAttr_SecretKeys — feed password/PASSWORD/api_token/bearer_token; assert all replaced with "[redacted]"; (2) TestRedactAttr_NonSecret — cluster/user (non-secret) pass through; (3) TestRedactAny_URL — *url.URL with User=url.UserPassword("u","p") → output has u@ but no :p@; (4) TestRedactAny_RedactedInterface — struct with Redacted() any returning "<masked>" → replaced; (5) TestWithAttrs_RedactsDerivedLogger — logger.With("password", "x").Info(...) → output has [redacted], never "x"; (6) TestWithGroup — group propagation preserves redaction; (7) TestGroupKind — nested slog.Group with a secret key inside is redacted.  
**Effort:** days

### Tier H — findings from 2026-04-25 /audit-all run

Filed by the orchestrator aggregation so `/roadmap-pickup` can fan them out when bandwidth opens. Each references the audit finding ID for diff tracking; when a finding recurs in a later run, its entry Status+Evidence updates here rather than being duplicated. Total: 226 findings (3 blocker, 44 major, 100 minor, 79 suggestion).

#### audit-security

#### audit-subprocess

##### `err:ddf885f4:errors-join-opportunity` — errors join opportunity

**Status:** deferred (audit-positive baseline; no code change needed — PR #448 closed by reviewer call)  
**Severity:** suggestion  
**Cluster:** wrapping  
**Evidence:** `internal/addon/manager.go:99-113`  
**Problem:** InstallAll already uses errors.Join correctly. NOT a finding — included to verify positive cluster compliance. Audit confirms `errors.Join(errs...)` and `errors.Join(err, fmt.Errorf("addon %s rollback: %w", info.Name, unErr))` at line 187 are the canonical pattern across the codebase. Other multi-error sites (cleanup/cleanup.go:113, dns/dns.go:228) also use errors.Join. No errors-join-opportunity findings.  
**Fix:** No fix; audit-positive note. Documented as a baseline so future contributors keep using errors.Join.  
**Effort:** hours

#### audit-concurrency

#### audit-api-design

#### audit-cli-ux

#### audit-modernization

#### audit-code-smells

#### audit-dependencies

#### audit-documentation

#### audit-tests

### Tier I — findings from 2026-05-05 /audit-all run

Filed by the orchestrator aggregation so `/roadmap-pickup` can fan them out when bandwidth opens. Each references the audit finding ID for diff tracking; when a finding recurs in a later run, its entry Status+Evidence updates here rather than being duplicated. Total: 171 findings (3 blocker, 32 major, 77 minor, 59 suggestion).

#### audit-security

#### audit-subprocess

#### audit-state-and-recovery

#### audit-iac-and-shell

#### audit-errors

#### audit-concurrency

#### audit-api-design

#### audit-cli-ux

#### audit-observability

#### audit-modernization

#### audit-code-smells

#### audit-dependencies

#### audit-documentation

#### audit-tests

### Tier J — findings from 2026-05-08 /audit-all run

Filed by the orchestrator aggregation so `/roadmap-pickup` can fan them out when bandwidth opens. Each references the audit finding ID for diff tracking; when a finding recurs in a later run, its entry Status+Evidence updates here rather than being duplicated. Total: 164 findings (3 blocker, 28 major, 61 minor, 72 suggestion).

#### audit-security

#### audit-subprocess

#### audit-state-and-recovery

#### audit-iac-and-shell

#### audit-errors

#### audit-concurrency

#### audit-api-design

#### audit-cli-ux

#### audit-observability

#### audit-modernization

#### audit-code-smells

#### audit-dependencies

#### audit-documentation

#### audit-tests

### Tier K — findings from 2026-05-21 /audit-all run

Filed by the orchestrator aggregation so `/roadmap-pickup` can fan them out when bandwidth opens. Each references the audit finding ID for diff tracking; when a finding recurs in a later run, its entry Status+Evidence updates here rather than being duplicated. Total: 159 findings (2 blocker, 24 major, 53 minor, 80 suggestion). Aggregated report: `.claude/audits/audit-all-2026-05-21.md`. Per-skill JSONL: `.claude/audits/audit-<skill>.jsonl`.

#### audit-security

##### `sec:e076e43c:dl-no-signature` — dl no signature

**Status:** deferred (already implemented by commit 83bc57e — verified 2026-07-11)
**Severity:** suggestion
**Cluster:** tls-network
**Evidence:** `scripts/install.sh:113-119`
**Problem:** GitHub releases-API resolution at L113-L119 fetches the latest tag via TLS to api.github.com with no integrity check beyond the TLS handshake. A compromised api.github.com cert or attacker-controlled DNS resolving to a TLS-MITM proxy would let the resolver return any tag; the subsequent cosign verification of SHA256SUMS only verifies the file at $BASE_URL — but $BASE_URL is built from the attacker-influenced VERSION. Once VERSION is malicious, the trust chain follows. The cosign cert-identity-regexp does anchor artefacts to qxtaiba/okdctl, so the attacker must serve a real signed release.
**Fix:** Defense-in-depth: after VERSION is resolved, validate it matches the regex ^v[0-9]+\.[0-9]+\.[0-9]+ before constructing URLs. This won't stop a creative attacker but bounds the URL grammar. The cosign cert-identity-regexp at L156 is the real trust root.
**Effort:** hours

#### audit-subprocess

##### `sub:ae5b624c:parallel-exec-wrapper` — parallel exec wrapper

**Status:** done — PR #926
**Severity:** suggestion
**Cluster:** io-handling — seam→`audit-api-design` — related: `sub:97cb8adf:no-cancel-func`
**Evidence:** `internal/distribution/okd/install/monitor.go:25-44`
**Problem:** defaultStartMonitorCmd reaches past the canonical executor.Executor to call osExec.CommandContext directly because the caller needs Start+Wait independence (the parent loop ticks at csrApprovalInterval while openshift-install runs for 30-60min). The env filter, cmd.Cancel SIGTERM, and WaitDelay are re-implemented inline; the embedded comment on L24 even acknowledges that this duplicates the canonical pattern. There is no Executor method that returns a done-channel for background subprocesses, so the duplication is structural rather than careless.
**Fix:** Add an Executor method along the lines of `StartStreamed(ctx, name, args...) (done <-chan error, err error)` that wires Start + a Wait-into-channel goroutine while sharing buildEnv / Cancel / WaitDelay from the existing run pattern. Then defaultStartMonitorCmd shrinks to a single call: `return p.Exec.StartStreamed(ctx, "openshift-install", "wait-for", "install-complete", "--dir", clusterDir, "--log-level=debug")`. Net LOC delta ~-10 at this site (+15 in executor) for a real interface gain. The new method is symmetric with RunStreamed.
**Effort:** hours

#### audit-state-and-recovery

##### `state:0f076161:destroy-no-cluster-confirm-without-yes` — destroy no cluster confirm without yes

**Status:** done — PR #908
**Severity:** minor
**Cluster:** destroy-safety
**Evidence:** `internal/cli/destroy.go:230-245`
**Problem:** Interactive destroy (no --yes flag) only requires answering 'y' or 'yes' to the prompt — no cluster-name typing requirement, unlike kubectl/eksctl. A misread terminal tab (wrong cluster's CLI history) plus a single 'y' executes destroy against the wrong cluster. The --confirm-cluster guard is only enforced when --yes is set.
**Fix:** In the interactive branch (line 237), prompt 'type cluster name to confirm destroy: ' and require the response to match cfg.Cluster.Name (case-sensitive). Match the --confirm-cluster=NAME contract used for scripted destroys. Skip when --target/--only is present because those branches already require --confirm-cluster at line 204-208. Optional: behind a TUI flag if it disrupts CI snapshot testing.
**Effort:** hours

##### `state:fb54208a:postinstall-mid-phase-no-checkpoint` — postinstall mid phase no checkpoint

**Status:** deferred (audit-positive baseline verified 2026-07-11 — no code change needed)
**Severity:** minor
**Cluster:** crash-recoverability
**Evidence:** `internal/distribution/okd/postinstall/steps.go:23-115`
**Problem:** PhaseContext (pctx) is in-process memory only. The 5-step postinstall sequence — verify-health, cleanup-bootstrap, verify-kubevip, deploy-production-dns, install-addons — has no persistent checkpoint. A crash between StepCleanupBootstrap and StepDeployProductionDNS loses the KubeVipIP that StepVerifyKubeVIP computed; the next run starts at verify-health and recomputes everything from scratch, relying on filesystem sentinels (bootstrap-state.auto.tfvars.json for bootstrap, dnsmasq config for DNS state) for idempotency. Resume is fragile because the recovery path depends on multiple independent sentinels staying in sync.
**Fix:** Two-stage fix tracked separately on roadmap: (1) lightweight — write a per-phase JSON checkpoint at <workDir>/.<phase>-checkpoint.json after each successful step containing the StepID and any pctx state needed for resume. AlreadyDone reads the checkpoint at orchestrator start. (2) heavier — extend StepDef with a Checkpoint hook and have Orchestrator persist after each success. Today's mitigation is the on-disk sentinels (bootstrap-state.auto.tfvars.json, dnsmasq conf state, kubeconfig presence) which each step's AlreadyDone consults independently. Defer per roadmap state:4f69fc9d; do not act now.
**Effort:** hours

##### `state:b38ec9cc:workers-targeted-apply-skips-other-drift` — workers targeted apply skips other drift

**Status:** done — PR #907
**Severity:** suggestion
**Cluster:** phase-idempotency
**Evidence:** `internal/distribution/okd/install/workers.go:46-76`
**Problem:** StartWorkerVMs runs `terraform apply -target=module.okd_cluster.proxmox_virtual_environment_vm.worker -var start_workers_immediately=true`. The targeted apply intentionally scopes to workers — but if a user hand-edited terraform.tfvars between setup and install (changing master CPU, network bridge), that drift is silently NOT applied. A subsequent un-scoped apply (e.g. via `okdctl deploy` re-run) suddenly applies all the drift at once.
**Fix:** No code change. The targeted apply is the right choice — un-scoped apply during worker-start would risk applying mid-cluster drift on master VMs. Document this trade-off in the function-level docstring (it's already a comment block, just lift to the function doc) so future readers don't mistake it for a forgotten target restriction.
**Effort:** hours

##### `state:4c092fce:snapshot-pruner-not-locked` — snapshot pruner not locked

**Status:** deferred (audit-positive baseline verified 2026-07-11 — no code change needed)
**Severity:** suggestion
**Cluster:** tf-state-atomicity
**Evidence:** `internal/infrastructure/terraform/terraform.go:420-442`
**Problem:** pruneSnapshots reads the workDir, sorts terraform.tfstate.*.bak entries lexicographically, and removes the oldest beyond the 5-retain limit. Two concurrent okdctl runs in the same workDir could each prune the same files — both os.Remove calls succeed (or one ENOENT-no-ops), and both walk an interleaved view of the directory. Runlock prevents the concurrent case within a host; cross-host NFS does not. Low-impact because the file content (state snapshot) is fully written before rename.
**Fix:** No code change. The acceptable failure mode (concurrent prunes producing slightly-different retention counts) is bounded by the 5-retain limit. The error path tolerates ENOENT. Verified: pruneSnapshots is only called from SnapshotState which is already gated by runlock at the CLI layer (destroy/deploy/bootstrap-destroy all Acquire the runlock).
**Effort:** hours

##### `state:48688e63:proxmox-api-no-direct-409-path` — proxmox api no direct 409 path

**Status:** deferred (audit-positive baseline verified 2026-07-11 — no code change needed)
**Severity:** suggestion
**Cluster:** proxmox-api-idempotency
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:30-36`
**Problem:** Provider docstring declares 'all Proxmox mutations MUST flow through terraform.Executor. Direct Proxmox HTTP calls are forbidden in deploy/destroy paths.' Verified: the only direct Proxmox-host interactions are SSH/pvesh for ISO listing/removal (idempotent: rm -f silent on missing) and a read-only enumeration probe in probeVMEnumeration. No 409/already-exists handling is needed because terraform owns mutation retry/backoff.
**Fix:** No code change. Confirm the invariant holds in future PRs: any new Proxmox API consumer in deploy/destroy must route through terraform.Executor, not net/http. The probeVMEnumeration helper at proxmox.go:470-503 is the documented exception (read-only SSH/pvesh enumeration).
**Effort:** hours

##### `state:881d089e:runlock-flock-cross-host-documented` — runlock flock cross host documented

**Status:** deferred (audit-positive baseline verified 2026-07-11 — no code change needed)
**Severity:** suggestion
**Cluster:** tf-state-atomicity
**Evidence:** `internal/runlock/runlock.go:1-10`
**Problem:** runlock uses syscall.Flock (advisory) for cross-process exclusion in the same project root. The package docstring and CLAUDE.md §architecture-notes both document the NFSv3 cross-host caveat. crossHostHint() at line 95-108 even surfaces the HOST= field mismatch in the error message. Terraform's own state lock (-lock-timeout=120s on every locking subcommand) is the authoritative guard.
**Fix:** No code change. The dual-lock model is correct: runlock catches the common case (same-host concurrent okdctl), terraform's -lock-timeout=120s catches the cross-host case with a queue-then-fail-with-diagnostic. Verify: every state-locking subcommand in terraform.go (Plan, PlanStreamed, Apply, Destroy direct/with-plan) passes -lock-timeout=120s — confirmed at lines 244, 264, 284, 347.
**Effort:** hours

##### `state:0f076161:destroy-skip-flag-orthogonal-with-dryrun` — destroy skip flag orthogonal with dryrun

**Status:** done — PR #908
**Severity:** suggestion
**Cluster:** destroy-safety
**Evidence:** `internal/cli/destroy.go:210-227`
**Problem:** --dry-run combined with --skip-terraform/--skip-cleanup/--skip-firewall returns a ConfigError. The error message is clear ('skip flags have no effect') but the orthogonality between dry-run (preview) and skip-flags (resume) is implicit. Operators resuming a partial destroy via --skip-terraform may try --dry-run first to preview and hit this error.
**Fix:** No code change. Optionally augment the docstring on destroyCmd.Long (line 138-156) to name the dry-run/skip orthogonality explicitly: 'dry-run is for previewing terraform-destroy; skip flags are for resuming after a partial terraform-destroy.'
**Effort:** hours

##### `state:d7ce9d16:dns-deploy-restart-fail-restore` — dns deploy restart fail restore

**Status:** deferred (audit-positive baseline verified 2026-07-11 — no code change needed)
**Severity:** suggestion
**Cluster:** phase-idempotency
**Evidence:** `internal/distribution/okd/dns/dns.go:242-277`
**Problem:** validateAndRestartDnsmasq has a backup-and-restore path: on validation OR restart failure, it copies the .backup file back over the live config. Verified clean: backup happens in writeDnsmasqConfig (line 89-93) BEFORE AtomicWriteString, so the backup is the prior config. On restart success the backup is removed. The restore() closure correctly uses system.CopyFile (preserves source mode at open time).
**Fix:** No change. The backup-validate-restart-restore-on-fail pattern is the canonical recovery shape for service-config writes. Cross-reference with internal/distribution/okd/setup/haproxy.go:147-166 (attemptHAProxyRollback) for the symmetric template.
**Effort:** hours

#### audit-iac-and-shell

#### audit-errors

##### `err:48688e63:proxmox-apply-cancel-bare-wrap` — proxmox apply cancel bare wrap

**Status:** done — PR #921
**Severity:** suggestion
**Cluster:** cancellation-identity — seam→`audit-concurrency`
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:232-242`
**Problem:** On ctx-cancelled terraform apply: fmt.Errorf('terraform apply interrupted: %w', errors.Join(ctx.Err(), applyErr)). errors.Is(err, context.Canceled) walks correctly because errors.Join exposes Unwrap() []error. But the outer fmt.Errorf is the only object visible to errors.As, so callers cannot recover a typed *errtypes.ClusterError here (the non-cancel branch at L241 returns one). Mixed-shape return is a smell — callers must branch on errors.Is before errors.As to handle both shapes.
**Fix:** The bare wrap is intentional and load-bearing — internal/cli/root.go::signalExitCode walks the chain via errors.Is(err, context.Canceled) before exitCodeFor runs, mapping SIGINT→130. The pattern matches the install/monitor.go canon. Add an inline comment (matching install/monitor.go:L65-68) so reviewers do not wrap it in ClusterError and break the SIGINT→130 mapping.
**Effort:** hours

##### `err:b38ec9cc:lock-hint-exit-code-flip` — lock hint exit code flip

**Status:** done — PR #907
**Severity:** major
**Cluster:** sentinel-vs-typed — seam→`audit-cli-ux`
**Evidence:** `internal/distribution/okd/install/workers.go:39-71` + 3 more
**Problem:** errors.Join(hint, wrapped) where hint is *errtypes.ConfigError and wrapped is *errtypes.ClusterError works because errors.Join's Unwrap() []error lets errors.As walk to both. But exitCodeFor walks the declaration order: ConfigError → 2, ClusterError → 4. errors.As returns on the first match. The hint matches ConfigError → exit 2; the ClusterError → exit 4 mapping is unreachable. terraform init failure → ClusterError → 4 normally; same failure under a stale lock → ConfigError → 2. Operators scripting against exit codes will see flaky behaviour.
**Fix:** Either (a) downgrade LockHint to return a *string* that the wrapped ClusterError appends to Msg (exit code stays uniform), or (b) restructure to wrap the hint message inside the ClusterError's Msg with the underlying err: `&errtypes.ClusterError{Msg: msg + '; ' + hintMsg, Err: err}` so only one typed error is in the chain. (c) Or accept the exit-2-for-locked-state mapping and document it in cli/root.go's exit-code table. Today the chain produces inconsistent exit codes silently.
**Effort:** hours

##### `err:f55b9c27:envfile-loadonce-no-sentinel` — envfile loadonce no sentinel

**Status:** done — PR #916
**Severity:** suggestion
**Cluster:** sentinel-vs-typed
**Evidence:** `internal/credentials/envfile.go:120-129`
**Problem:** LoadEnvFile returns a ConfigError when called twice with different paths. There is no way for the caller to distinguish 'already initialized, original error' from 'already initialized with a different path' without string-sniffing the Msg.
**Fix:** Add a package-local sentinel `var ErrEnvFileAlreadyLoaded = errors.New("env file already loaded with different path")` and use it as Err on the ConfigError. Callers can errors.Is to detect the double-init case without parsing Msg. Today there's exactly one caller chain (cli/helpers.go::handleCredentials, cli/destroy.go) so practical risk is zero.
**Effort:** hours

##### `err:366b3f2d:orchestrator-classifysteperr-canonical` — orchestrator classifysteperr canonical

**Status:** deferred (audit-positive baseline verified 2026-07-11 — no code change needed)
**Severity:** suggestion
**Cluster:** sentinel-vs-typed
**Evidence:** `internal/distribution/orchestrator.go:115-133`
**Problem:** classifyStepErr is the load-bearing safety net: it correctly preserves cancellation identity and skips wrapping for already-typed errtypes. The smell isn't in this function but in its presence — it catches 14+ bare fmt.Errorf sites in dns/postinstall/setup/install packages. Those packages' contracts are silently orchestrator-dependent. Documented as the canonical example so reviewers know why surrounding findings are minor not major.
**Fix:** No change. Documented as the canonical fallback so future audits know it exists. If classifyStepErr is ever removed or moved, the 14 bare-fmt.Errorf sites above must each be hardened first.
**Effort:** hours

##### `err:ae5b624c:monitor-asymmetric-cancel-handling` — monitor asymmetric cancel handling

**Status:** deferred (audit-positive baseline verified 2026-07-11 — no code change needed)
**Severity:** suggestion
**Cluster:** cancellation-identity — seam→`audit-concurrency`
**Evidence:** `internal/distribution/okd/install/monitor.go:48-75`
**Problem:** WaitForBootstrap handles three ctx-error branches asymmetrically: DeadlineExceeded wraps into ClusterError{Err: ctx.Err()} (preserves chain), Canceled bare-wraps via fmt.Errorf (preserves SIGINT→130), other err returns generic ClusterError. The asymmetry is intentional per the load-bearing comment at L65-68: the DeadlineExceeded path runs through exitCodeFor (ClusterError → 4), the Canceled path runs through signalExitCode (SIGINT → 130). Documented here for cross-reference.
**Fix:** No change. The asymmetry is deliberate and load-bearing per the in-code comments at L65-68 and L137-139. Documented here so reviewers know the DeadlineExceeded/Canceled split is intentional, not an inconsistency. The SIGINT path goes through cli/root.go::signalExitCode; the deadline path goes through exitCodeFor. Both yield correct exit codes (130 vs 4 respectively).
**Effort:** hours

#### audit-concurrency

##### `con:48688e63:disconnect-ctx-unused` — disconnect ctx unused

**Status:** deferred (scaffolding verified 2026-07-11 — kept per MEMORY.md; annotation present)
**Severity:** suggestion
**Cluster:** ctx-ignored
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:147-159`
**Problem:** Provider.Disconnect accepts context.Context but the body uses none of it - no I/O, no select, no ctx.Err check. The signature is shaped for a future network-bound disconnect handshake (Connect/Disconnect symmetry), and the receiver name underscores intent. Belongs on the scaffolding list, not the ranked table - fix is verify intent against roadmap, not delete the parameter.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

#### audit-api-design

##### `api:262af6e4:opt-no-newoptions` — opt no newoptions

**Status:** done — PR #908
**Severity:** minor
**Cluster:** option-consistency
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:68-83`
**Problem:** cleanup.Options is the only phase Options struct without a NewOptions(cfg,projectRoot) constructor — setup, install, postinstall, destroy all expose NewOptions for the same purpose. The three call sites that build cleanup.Options (cli/cleanup.go:115, okd/okd.go:122, destroy/steps.go:131) all hand-assemble BaseOptions and Kind separately, duplicating phase.GetTerraformEnv(cfg) plumbing.
**Fix:** Add func NewOptions(cfg *config.Config, projectRoot string, kind Kind) Options to internal/distribution/okd/cleanup/cleanup.go that sets BaseOptions{ProjectRoot,WorkDir,TerraformEnv} and Kind, mirroring destroy.NewOptions. Caller code (internal/cli/cleanup.go:115, internal/distribution/okd/okd.go:122, internal/distribution/okd/destroy/steps.go:131) replaces the inline literal with the call. Caller-specific overrides (HTTPServerRoot, HAProxyConfig, VIP, BinDir) still set field-by-field after construction.
**Effort:** hours

##### `api:d6b325cb:pkg-types-direction` — pkg types direction

**Status:** done — PR #921
**Severity:** minor
**Cluster:** package-boundary
**Evidence:** `internal/infrastructure/proxmox/types.go:1-49`
**Problem:** internal/infrastructure/proxmox imports internal/distribution/okd/phase for NodeRole, VMState, and the RemoteISOParams/PveshRun helpers. Per CLAUDE.md the dependency direction should be cli → distribution/okd → infrastructure/proxmox. The current shape inverts that on shared domain types: proxmox is parametrised by phase's NodeRole/VMState, and phase/pvesh.go contains Proxmox-specific subprocess plumbing (PveshRun, RemoteISOParams) that semantically belongs to the proxmox package. The package doc at vmstate.go:6 acknowledges the inversion.
**Fix:** Extract NodeRole/VMState/RemoteISOParams + the Proxmox SSH/pvesh subprocess primitives into a new internal/cluster/types or internal/infrastructure/proxmox/types-only sub-package that both phase and proxmox can import without forming a cycle. Today's compromise (cluster-domain enums in 'phase') works but every new shared concept widens the wrong-direction surface. Verify intent on the roadmap before refactor — multi-provider expansion would justify it, otherwise the cycle-break is the right call.
**Effort:** hours

##### `api:262af6e4:opt-execute-receiver-unused` — opt execute receiver unused

**Status:** done — PR #908
**Severity:** minor
**Cluster:** option-consistency
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:97-102`
**Problem:** cleanup.Phase.Execute is a one-line forwarder to a package-level execute() that only uses p.Log from the BasePhase. Other phases (setup, install, postinstall, destroy) use the full BasePhase shape — Exec/Log/Recorder — through orchestrator.SetMetricsRecorder, but cleanup discards everything except Log. The receiver becomes vestigial: a caller building cleanup via cleanup.New() with WithExecutor or WithRecorder gets a silent no-op for those options.
**Fix:** In internal/distribution/okd/cleanup/cleanup.go change execute() to take a *Phase or accept logger+recorder, and wire orchestrator.SetMetricsRecorder(p.Recorder) so cleanup emits step metrics like the other phases. Alternatively, document at Phase.Execute that cleanup intentionally skips metrics and have New() panic on phase.WithRecorder.
**Effort:** hours

##### `api:0fc0041d:export-no-caller-conditions` — export no caller conditions

**Status:** deferred (scaffolding verified 2026-07-11 — kept per MEMORY.md; annotation present)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/condition.go:9-40`
**Problem:** ConditionType constants ConditionTypeAvailable, ConditionTypeProgressing and NodeStatusPhase.NodeStatusUnknown have no in-repo caller outside the test build. The package doc names them as scaffolding for a future status verb that surfaces non-Ready operator conditions.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:0139cb3f:export-three-bindir-functions` — export three bindir functions

**Status:** deferred (superseded by PR #869 bindir relocation — re-point or close after merge)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/paths.go:53-92`
**Problem:** phase.ResolveBinDir, phase.PreflightBinDir, and phase.BinDirOrDefault expose three overlapping resolution functions that differ only in their input sources (config+env, env-only, struct-field+default). The 'three-function surface rationale' is documented inline. Whether the symmetry survives the next refactor depends on whether the env-only preflight path stays distinct from main-flow resolution.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:48688e63:zero-value-disconnect-noop` — zero value disconnect noop

**Status:** deferred (scaffolding verified 2026-07-11 — kept per MEMORY.md; annotation present)
**Severity:** suggestion
**Cluster:** zero-value-usability
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:147-159`
**Problem:** Provider.Connect/Disconnect form a symmetric pair, but Disconnect's body ignores ctx (the parameter name is `_`) and resets two struct fields without any genuine teardown. The shape is scaffolding for 'future network-bound providers' per the inline doc. Until a real teardown lands, callers who write ctx-aware code (defer prov.Disconnect(ctx)) get no benefit and the symmetric API hides a no-op behind a context-shaped signature.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:c287d5c0:export-no-caller-zeroize-env` — export no caller zeroize env

**Status:** deferred (finding invalid — Provisioner.ZeroizeEnv has production callers cli/destroy.go + cli/helpers.go; verified 2026-07-11)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/okd.go:198-207`
**Problem:** Provisioner.ZeroizeEnv delegates to the underlying executor's ZeroizeEnv and is exposed for credential-lifecycle scaffolding (annotated api:c287d5c0). No external caller in the current code; the field owner (executor.Executor.ZeroizeEnv) is called directly from the cli layer instead.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:859eea6f:export-no-caller-parsenoderole` — export no caller parsenoderole

**Status:** deferred (scaffolding verified 2026-07-11 — kept per MEMORY.md; annotation present)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/noderole.go:21-32`
**Problem:** phase.ParseNodeRole is the deserialization counterpart to NodeRole.String() with no in-repo caller today. Inline doc names it as scaffolding for upcoming status JSON / terraform-output deserialization.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:7b2829bb:opt-name-allowlist-redundant` — opt name allowlist redundant

**Status:** deferred (scaffolding verified 2026-07-11 — kept per MEMORY.md; annotation present)
**Severity:** suggestion
**Cluster:** option-consistency
**Evidence:** `internal/executor/executor.go:103-143`
**Problem:** executor exposes DefaultEnvAllowlist (var) plus EnvAllowlist (type) plus FilterParentEnv(EnvAllowlist) free function. The var is the only EnvAllowlist value in the binary; callers always pass DefaultEnvAllowlist. The type+function surface is shaped like a reusable allowlist API, but the sole external caller (cli/elevation.go) passes the same default. The shape suggests multi-allowlist support that never materialised.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:97cb8adf:opt-struct-vs-functional-waitfor` — opt struct vs functional waitfor

**Status:** in review — PR #941
**Severity:** suggestion
**Cluster:** option-consistency
**Evidence:** `internal/system/exec.go:89-102`
**Problem:** system.WaitForOptions is the only configuration-via-struct in the repo's runtime/operation surface — every other 'configure X at call time' surface either uses inline arguments or a struct-of-options *passed to a constructor* (terraform.PlanOptions, proxmox.ProvisionOptions). WaitFor's signature takes (ctx, prefix, description, check, opts) — the options struct is materialized once via DefaultWaitForOptions() + field assignment, then passed. Consider whether WaitFor should follow the functional-options pattern of the constructors instead.
**Fix:** Choice: (a) keep WaitForOptions as the canonical 'per-call config struct' (and document it as the deliberate choice for one-shot configurable operations); (b) rewrite to func WaitFor(ctx, prefix, desc, check, opts ...WaitForOption) for full symmetry with constructor options. Two callers exist today (kubectl.go OcPollOutputInterval, exec_test.go); the WaitForWithTimeout wrapper exists precisely because the struct shape is verbose. Functional options would simplify the wrapper away.
**Effort:** hours

##### `api:7b2829bb:opt-with-inherited-env-noarg` — opt with inherited env noarg

**Status:** done — PR #926
**Severity:** suggestion
**Cluster:** option-consistency
**Evidence:** `internal/executor/executor.go:79-87`
**Problem:** executor.WithInheritedEnv is a no-arg toggle option that flips a boolean (inheritEnv). It pairs with WithEnv (appends) but its lack of a 'b bool' argument makes the option set asymmetric — there is no WithInheritedEnv(false) inverse. The current shape is one-way: a caller that wants 'maybe inherit, decided dynamically' must rebuild the option list rather than passing a single boolean.
**Fix:** Optional: change signature to WithInheritedEnv(b bool) Option { return func(e *Executor) { e.inheritEnv = b } } to match the bool-toggle shape used by download.WithOverwrite(v bool). Today's no-arg form is fine for the single use site that always wants 'true' — the asymmetry is only visible to a future caller wanting dynamic dispatch. Document the call-side intent inline if keeping the no-arg form.
**Effort:** hours

##### `api:a55b4592:zero-value-loader` — zero value loader

**Status:** deferred (scaffolding verified 2026-07-11 — kept per MEMORY.md; annotation present)
**Severity:** suggestion
**Cluster:** zero-value-usability
**Evidence:** `internal/config/loader.go:13-21`
**Problem:** config.Loader is an empty struct{} whose constructor returns &Loader{}. The type doc admits 'intentionally stateless today; the struct shape is the canonical surface so a future stateful Loader can land without breaking call-site shapes'. The empty struct + NewLoader is scaffolding for a future caching/decryption Loader.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:beabab0c:opt-execute-takes-cfg-twice` — opt execute takes cfg twice

**Status:** in review — PR #941
**Severity:** suggestion
**Cluster:** option-consistency
**Evidence:** `internal/distribution/okd/setup/phase.go:120-135`
**Problem:** All four phase Execute methods take cfg *config.Config in addition to opts *Options, but opts is built by NewOptions(cfg, projectRoot) which already consulted cfg. The double-passing leaves callers responsible for keeping the (cfg, opts) pair consistent: if a caller calls NewOptions(cfg1, root) and then Execute(ctx, cfg2, &opts), the phase sees opts.TerraformEnv from cfg1 but cfg2 elsewhere. No bug today (callers always pass the same cfg), but the signature does not enforce the invariant.
**Fix:** Option A: store cfg in Options at NewOptions time, drop the cfg argument from Execute. Option B: leave as-is and document the invariant 'cfg passed to Execute must be the same cfg passed to NewOptions'. Option B is the lower-risk choice for now since 4 phases × Execute signatures × all callers is a lot of churn; consider as a v1.0 surface-stabilization item.
**Effort:** hours

##### `api:4f69fc9d:iface-fragmented-step` — iface fragmented step

**Status:** done — PR #923
**Severity:** minor
**Cluster:** interface-location
**Evidence:** `internal/distribution/step.go:33-67`
**Problem:** distribution.Step, Skipper, FatalChecker, AlreadyDoneChecker, StepCallbacks are all exported small interfaces. They are consumed implicitly by Orchestrator via type assertion (step.(AlreadyDoneChecker)). No external package implements these interfaces directly — every step is built through StepBuilder/StepDef which produces builtStep that satisfies them all. The fragmented interface set is a 'role interfaces' pattern but with one implementation, so the splits add cognitive load without value.
**Fix:** Two options: (a) collapse the five role interfaces into one ProvisioningStep interface (already exported at L71); the role interfaces become an unexported implementation detail used by Orchestrator. (b) Keep the role interfaces but document at the package doc that they are 'for orchestrator-side type assertion, not for external implementation'. Orchestrator.executeStep currently does step.(AlreadyDoneChecker) probes — these stay either way. Choose based on whether the project wants role interfaces as a public 'extensibility surface' or as orchestrator plumbing.
**Effort:** hours

##### `api:c287d5c0:opt-destroyopts-duplication` — opt destroyopts duplication

**Status:** done — PR #908
**Severity:** minor
**Cluster:** option-consistency
**Evidence:** `internal/distribution/okd/okd.go:181-222`
**Problem:** okd.DestroyOpts and destroy.Options carry nearly the same field set (AutoApprove, RemovePackages, KeepISOs, SkipTerraform, SkipCleanup, SkipFirewall, TerraformTargets). okd.Provisioner.Destroy unpacks DestroyOpts field-by-field into destroy.Options. The two structs differ by destroy.Options embedding phase.BaseOptions and adding CleanupKind/Parallelism. The duplication forces every new destroy flag to land in two places.
**Fix:** Pick one shape: (a) okd.Provisioner.Destroy takes destroy.Options directly (collapses DestroyOpts entirely); (b) DestroyOpts becomes the CLI-facing struct and the provisioner injects the BaseOptions+CleanupKind defaults internally before calling destroy.Phase.Execute. Option (a) is cleaner — the CLI builds destroy.Options once via destroy.NewOptions. The trade-off is exposing destroy.Options' embedded BaseOptions through the CLI surface; if BaseOptions internals shouldn't bleed into the CLI, prefer (b).
**Effort:** hours

##### `api:ae5b624c:iface-csr-approver-positive` — iface csr approver positive

**Status:** done — PR #922
**Severity:** suggestion
**Cluster:** interface-location
**Evidence:** `internal/distribution/okd/install/monitor.go:78-83`
**Problem:** install.csrApprover is a one-method interface (ApprovePendingCSRs) defined at the consumer side (install package) where cluster.Client is the real implementation. This is the correct shape per Go proverb 'Accept interfaces, return structs'. The interface is unexported (csrApprover), which is the right call for a single-package consumer. Logged as a positive counter-example for the api-design audit — every interface that abstracts a cluster operation should follow this shape.
**Fix:** No action — this is the canonical pattern. Document explicitly in cluster.Client's package doc that 'consumers may define their own narrow interfaces to test against; cluster.Client returns concrete types, never interfaces'. Future cluster-method additions (e.g. Client.GetNodes) should leave consumer-side interface definition to the consumer.
**Effort:** hours

#### audit-cli-ux

##### `ux:fd2125dd:concept-named-twice` — concept named twice

**Status:** done — PR #896
**Severity:** minor
**Cluster:** verb-noun
**Evidence:** `internal/cli/addon.go:28-31` + 1 more
**Problem:** Parent group nouns are inconsistently singular vs plural across the command tree: 'addon list/install/uninstall/verify' (singular) but 'releases list/show' (plural). Internal references compound the drift — addon.go uses 'addon list/verify' in messages while releases.go uses 'releases list'. Pick one: kubectl/oc convention is singular (kubectl get pod, kubectl describe service).
**Fix:** Rename releases → release for consistency with addon. The fix breaks anyone scripting okdctl releases list, but okdctl is pre-1.0 (per README §status) and the CHANGELOG can document the rename. Alternative: rename addon → addons. Either way, document the convention in CLAUDE.md so new groups follow it.
**Effort:** hours

##### `ux:0f076161:exit-taxonomy-not-published` — exit taxonomy not published

**Status:** done — PR #908
**Severity:** minor
**Cluster:** exit-codes — seam→`audit-errors`
**Evidence:** `internal/cli/destroy.go:204-227`
**Problem:** destroy returns errtypes.ConfigError (exit 2) for what are really --target/--only argument errors. Per the taxonomy in docs/cli/exit-codes.md, 64 (EX_USAGE) is the BSD code for command-line usage error and is what UsageError maps to. The destroy command does use UsageError for some checks (the index-out-of-range ones at L113-L128) but returns ConfigError for the --target-without-confirm-cluster check at L205 and for --dry-run incompatibility at L222. Same risk class (operator-supplied flags), different exit code.
**Fix:** Change ConfigError to UsageError at internal/cli/destroy.go:L205 and L222 (and similar patterns in deploy.go:L59-L62 for --metrics-allow-network without --metrics-addr). All three are flag-combination violations that should exit 64 (EX_USAGE), not 2 (config). Update docs/cli/exit-codes.md to clarify the boundary between ConfigError (file/schema problem) and UsageError (flag combo problem).
**Effort:** hours

##### `ux:073d24ed:flag-shortcut-collision` — flag shortcut collision

**Status:** done — PR #896
**Severity:** minor
**Cluster:** flag-conventions
**Evidence:** `internal/cli/deploy.go:46-51` + 2 more
**Problem:** deployCmd uses --output-file (no shorthand, per CLAUDE.md policy — kubeconfigCmd does the same). But deployOutputFile defaults to 'okdctl.yaml' while debugBundleOutput defaults to '' (interpreted as a generated filename) and kubeconfigOutput defaults to '-' (stdout). Three sibling commands that use the same flag give three different semantics for the empty/default state. Inconsistent UX across siblings sharing a flag.
**Fix:** Document a per-command default-value convention for --output-file in CLAUDE.md §flag-naming-convention: '-' for stdout-by-default (kubeconfig), '' for auto-generated (debug-bundle), and a literal default file for deploy. Currently a reader of okdctl deploy --help cannot tell whether '' would create or overwrite. Consider unifying on '-' = stdout for all three and a separate --auto-name flag for debug-bundle.
**Effort:** hours

#### audit-observability

#### audit-modernization

##### `mod:b38ec9cc:use-strings-lines` — use strings lines

**Status:** done — PR #907
**Severity:** minor
**Cluster:** slices-maps
**Evidence:** `internal/distribution/okd/install/workers.go:91-97`
**Problem:** Materialises strings.Split(out, newline) into a slice when only iterating to count non-blank lines. Go 1.24's strings.Lines is the iterator form; the repo already uses it in eight other sites.
**Fix:** Replace `for _, line := range strings.Split(out, "\n")` with `for line := range strings.Lines(out)`. Matches the dominant pattern in internal/platform/platform.go:L85, internal/distribution/okd/install/flux.go:L36, internal/distribution/okd/setup/tools.go:L287. Note strings.Lines retains the trailing newline on each emitted line, so the TrimSpace check still works.
**Effort:** hours

#### audit-code-smells

##### `smell:0f076161:stringly-typed-enum` — stringly typed enum

**Status:** done — PR #908
**Severity:** minor
**Cluster:** magic-strings
**Evidence:** `internal/cli/destroy.go:109-130`
**Problem:** validateDestroyTargets re-derives role identity from a regex capture and switches on raw bootstrap/master/worker strings even though the canonical phase.NodeRole typed enum carries those exact literals. Adjacent code in the same file already uses destroyScope (phase-aligned typed values); the role compare drops back to bare strings.
**Fix:** Replace the bare-string switch with `switch phase.NodeRole(nodeType)` and `case phase.RoleBootstrap, phase.RoleMaster, phase.RoleWorker`. destroyTargetRE already constrains the captured value to the three role literals, so the conversion is lossless. Net change is three case-label updates; phase is already imported on file (L17).
**Effort:** hours

##### `smell:48688e63:bool-should-be-3state` — bool should be 3state

**Status:** done — PR #921
**Severity:** suggestion
**Cluster:** bool-should-be-enum
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:470-503`
**Problem:** probeVMEnumeration returns bool but the doc-comment names three distinct semantic outcomes (found in pvesh list, not in pvesh list, probe could not run treat as found). The boolean collapses two true cases that mean opposite things and forces every caller to read the doc to understand which branch they are in.
**Fix:** Introduce a vmEnumerationState int-iota with enumYes, enumNo, enumProbeSkipped constants. The single caller in Provision (L254-L263) currently uses `if vmidEnumerable` for log suppression; branching on the typed enum lets that code distinguish probe-confirmed-visible from probe-skipped-default-visible, which a future caller may need for retry decisions.
**Effort:** hours

##### `smell:262af6e4:abstraction-single-caller` — abstraction single caller

**Status:** done — PR #908
**Severity:** suggestion
**Cluster:** helper-package-no-value
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:24-56`
**Problem:** cleanup.Kind defines five values (Full, WorkOnly, WebOnly, HAProxyOnly, TerraformOnly) plus ValidKinds/KindStrings/IsValid/Validate helpers, but only Full and WorkOnly are referenced outside the package. WebOnly, HAProxyOnly, and TerraformOnly carry no callers; the switch in cleanupSteps covers them only to maintain the full enum shape.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

#### audit-dependencies

##### `dep:e8f33f61:maint-single-bus-go-proxmox` — maint single bus go proxmox

**Status:** done — PR #895
**Severity:** major
**Cluster:** maintenance-signal
**Evidence:** `go.mod:12-12`
**Problem:** github.com/luthermonson/go-proxmox v0.5.1 has bus factor 1 (luthermonson 191 commits vs next-best 20). v0.x semver allows breaking changes on minor bumps; sole Proxmox API path for the wizard. Re-confirm per CLAUDE.md §dependencies, do not propose ripping out.
**Fix:** Re-confirm only — do NOT propose removal. Track upstream releases; bump on each. Fallback documented: ~200 LOC REST-only rewrite under internal/proxmox using net/http + the documented Proxmox REST API. Treat any 3-month upstream-silence window as the trigger to start the rewrite.
**Effort:** hours

##### `dep:33ef32bf:yaml-prod-binary-engines` — yaml prod binary engines

**Status:** done — PR #895
**Severity:** minor
**Cluster:** duplicate-engine
**Evidence:** `go.mod:19-55`
**Problem:** Production binary ships two YAML engines: sigs.k8s.io/yaml v1.6.0 (direct) plus go.yaml.in/yaml/v2 v2.4.3 (transitive via k8s.io/apimachinery). Smaller than the previously documented quad — gopkg.in/yaml.v3 is now test-only and go.yaml.in/yaml/v3 ships only in cmd/okdctl-gen-docs (build-tool).
**Fix:** Re-confirm acceptance. Both engines are dictated by upstream (sigs.k8s.io/yaml needs go.yaml.in/yaml/v2 for the k8s schemas; sigs.k8s.io/yaml is the direct entry point). Update CLAUDE.md §dependencies to record the corrected two-engine prod baseline so the tripwire fires only when a fifth engine lands or the direct dep changes shape. Do not attempt consolidation.
**Effort:** hours

##### `dep:33ef32bf:gorilla-ws-version-floor` — gorilla ws version floor

**Status:** done — PR #895
**Severity:** minor
**Cluster:** justified-version-floor
**Evidence:** `go.mod:39-39`
**Problem:** gorilla/websocket v1.4.2 (from 2020) lags 1.5.3 latest by three minor releases. Pulled transitively via go-proxmox; okdctl does NOT reach it (wizard uses REST discovery only). Per CLAUDE.md §dependencies the version is upstream-locked, but the floor diverges enough that govulncheck silence is the only thing standing between us and a stale CVE surface.
**Fix:** Hold — no action. CLAUDE.md §dependencies pre-flags this: 'safe to keep until go-proxmox migrates to coder/websocket, at which point take the bump without local code changes.' File an upstream issue / open a tracking PR on go-proxmox advocating the migration so the freeze ends. Do not bump unilaterally — go.mod minimum-version semantics mean a local bump may not change what go-proxmox sees.
**Effort:** hours

##### `dep:33ef32bf:json-iterator-stale` — json iterator stale

**Status:** done — PR #895
**Severity:** suggestion
**Cluster:** maintenance-signal
**Evidence:** `go.mod:42-42`
**Problem:** json-iterator/go v1.1.12 (last release Sep 2021, last commit May 2024) is effectively in maintenance freeze. Pulled transitively via k8s.io/apimachinery + sigs.k8s.io/structured-merge-diff/v6 — okdctl does not import it directly. Upstream k8s is gradually migrating to encoding/json + cbor, but the dep persists in the binary.
**Fix:** No action — track k8s upstream's migration off json-iterator. Removal will come naturally on a k8s.io/api+apimachinery bump (likely k8s 1.32+ as the CBOR path matures). Do not propose a Go-side replace; the dep is k8s-mandated.
**Effort:** hours

##### `dep:33ef32bf:modern-go-concurrent-abandoned` — modern go concurrent abandoned

**Status:** done — PR #895
**Severity:** suggestion
**Cluster:** maintenance-signal
**Evidence:** `go.mod:47-48`
**Problem:** modern-go/concurrent (last commit Aug 2019, 6.5y stale) and modern-go/reflect2 (last release 2021, last commit Mar 2025) are abandoned-but-shipping. Both transitively required by json-iterator/go → k8s.io/apimachinery. License Apache-2.0 (clean). Pure tripwire: if upstream go-namespace ever lapses, k8s ecosystem breaks for everyone.
**Fix:** No action; can't dislodge transitively. Removal comes when json-iterator/go is removed (see dep:33ef32bf:json-iterator-stale). Record the tripwire so a future GitHub-namespace-vacate event triggers an immediate k8s.io bump.
**Effort:** hours

##### `dep:33ef32bf:claude-md-godotenv-stale` — claude md godotenv stale

**Status:** done — PR #895
**Severity:** suggestion
**Cluster:** maintenance-signal
**Evidence:** `CLAUDE.md:1-1`
**Problem:** CLAUDE.md §dependencies notes 'github.com/joho/godotenv ships its license file as LICENCE (British spelling) — a valid MIT license; SBOM scanners that grep for LICENSE will flag a false positive.' godotenv is no longer in go.mod or go.sum — the note is documenting a non-existent dep.
**Fix:** Remove the godotenv LICENCE note from CLAUDE.md §dependencies; it documents a dep no longer in the tree. Keep one line elsewhere (e.g. a deprecation log section) if the false-positive precedent is worth retaining for the next dep that ships its license under a non-LICENSE name.
**Effort:** hours

##### `dep:b803fcb7:tflint-pin-format` — tflint pin format

**Status:** deferred (first attempt closed by maintainer — PR #701)
**Severity:** suggestion
**Cluster:** pin-stability
**Evidence:** `.github/workflows/ci.yml:103-103`
**Problem:** tflint setup uses `tflint_version: "v0.62.0"` (with leading 'v') while every other tool in the workflow uses the canonical format without leading v (`version: v2.12.2`, `terraform_version: "1.10.3"`). Cosmetic inconsistency, not a correctness issue — terraform-linters/setup-tflint accepts both forms.
**Fix:** Normalize to `tflint_version: "0.62.0"` to match the in-repo pattern for terraform_version. Cosmetic only — both forms are accepted by setup-tflint.
**Effort:** hours

##### `dep:660d83a5:charm-log-vs-slog-policy` — charm log vs slog policy

**Status:** done — PR #895
**Severity:** suggestion
**Cluster:** duplicate-engine — seam→`audit-modernization`
**Evidence:** `internal/tui/logger.go:12-196`
**Problem:** charm.land/log/v2 is a third-party logger wrapping a slog.Handler facade, used purely for colored level styling on stderr. log/slog (stdlib since 1.21) plus a small lipgloss-styled handler could replicate this; the dep adds maintenance debt and a second logging engine alongside the stdlib slog default. CLAUDE.md does NOT pre-clear charm.land/log/v2 the way SKILL.md §5 pre-clears charm.land/bubbletea (which carries 'Charm libs — intentional UI stack').
**Fix:** Optional — replace charm.land/log/v2 with a hand-rolled slog.Handler that renders levels through lipgloss styles. ~50 LOC swap for the level-styling logic, removes one transitive subtree (go-logfmt, etc.). If the charm-ecosystem coherence is valued, keep as-is. SKIM ONLY: seam #10 says 'drop dep, stdlib covers it' = modernization-owned. Cross-listed here because the CHOICE of charm.land/log is a dependency-policy decision (charm coherence vs minimization), not a Go-version-driven migration. Defer to audit-modernization if it flags the same site.
**Effort:** hours

##### `dep:33ef32bf:diskfs-transitive-weight` — diskfs transitive weight

**Status:** done — PR #895
**Severity:** minor
**Cluster:** transitive-weight
**Evidence:** `go.mod:34-34`
**Problem:** github.com/diskfs/go-diskfs v1.9.3 ships in the production binary purely because go-proxmox imports it (likely for ISO9660 manipulation in the Proxmox iso upload flow). okdctl does not call go-diskfs directly, but the dep pulls anchore/go-lzo + pkg/xattr + ulikunitz/xz + pierrec/lz4/v4 + klauspost/compress into the binary — five compression libs for a code path okdctl never reaches.
**Fix:** Re-confirm — keep. Pulled by go-proxmox; the only ways to drop it are (a) the documented REST-only rewrite (see dep:e8f33f61:maint-single-bus-go-proxmox), (b) a go-proxmox upstream PR splitting the ISO9660 helper into a separate module. Document the transitive-weight cost (~5 compression libs, +diskfs surface area) as a tally toward the rewrite trigger so future-self has the receipt.
**Effort:** hours

#### audit-documentation

#### audit-tests

### Tier L — findings from 2026-06-10 /audit-all run

Filed by the orchestrator aggregation so `/roadmap-pickup` can fan them out when bandwidth opens. Each references the audit finding ID for diff tracking; when a finding recurs in a later run, its entry Status+Evidence updates here rather than being duplicated. Total: 141 findings (4 blocker, 33 major, 55 minor, 49 suggestion). Aggregated report: `.claude/audits/audit-all-2026-06-10.md`. Per-skill JSONL: `.claude/audits/audit-<skill>.jsonl`.

Recurring findings already tracked in earlier tiers (entries NOT duplicated here; status lives at the original entry):

- `state:0f076161:destroy-no-cluster-confirm-without-yes` — in progress (Tier K — worktree open)
- `state:fb54208a:postinstall-no-rollback-path` — deferred (earlier tier)
- `state:4f69fc9d:no-resume-checkpoint` — deferred (earlier tier)
- `api:4f69fc9d:iface-fragmented` — deferred (earlier tier)

#### audit-security

#### audit-subprocess

##### `sub:0934cf1b:no-timeout` — no timeout

**Status:** done — PR #926
**Severity:** suggestion
**Cluster:** timeout-cancel
**Evidence:** `internal/platform/packages.go:60-90` + 2 more
**Problem:** dnf/apt-get install/remove/update run via RunCaptured with no ctx deadline and stdout discarded: a wedged mirror or stale repo metadata hangs the deploy indefinitely with zero visible progress. Cancel wiring exists (cmd.Cancel SIGTERM + WaitDelay 30s, signal-watched root ctx), so Ctrl-C recovers — the gap is deadline + operator visibility only.
**Fix:** Wrap package-manager invocations in context.WithTimeout (generous, e.g. 15-20 min) mirroring ocExtractTimeout in release_extract.go, or stream output via the executor so a stall is at least visible. Risk: an aggressive timeout flakes slow mirrors — keep it generous.
**Effort:** hours

##### `sub:4c092fce:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** done — PR #926
**Severity:** minor
**Cluster:** io-handling — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/infrastructure/terraform/terraform.go:467-484`
**Problem:** Executor.Output unmarshals `terraform output -json` from the 200-line ring tail. Pretty-printed output maps beyond ~200 lines lose their head and fail the JSON parse. Fails loudly (invalid json error), but the failure mode is a latent capacity cliff unrelated to the actual terraform state.
**Fix:** Use a full-capture executor variant for `terraform output -json` (see sub:1e8ffb91). Current module output count fits in 200 lines, so this is a cliff, not an active break.
**Effort:** hours

##### `sub:19a715fd:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** done — PR #926
**Severity:** minor
**Cluster:** io-handling — seam→audit-security — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/addon/catalog/secretstore/secretstore.go:276-282`
**Problem:** readSecret returns `sops -d` plaintext from the ring tail. A decrypted secret longer than 200 lines (or a single line over the 64 KiB partial cap, which gets newline-split) is silently truncated/corrupted before being embedded in a cluster Secret manifest — no error, no truncation marker. Typical tokens are one short line, so likelihood is low, but the failure is silent credential corruption.
**Fix:** Decrypt through a full-capture executor variant (see sub:1e8ffb91) so secret material cannot be silently truncated; keep the stdout-not-argv channel for the plaintext.
**Effort:** hours

##### `sub:696d6b0e:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** done — PR #926
**Severity:** suggestion
**Cluster:** io-handling — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/distribution/okd/phase/iso_cleanup.go:138-159` + 1 more
**Problem:** listProxmoxVMIDs / vmConfigReferencesISO parse pvesh JSON, and RemoveFCOSISOFromProxmox parses find -print0 output, all from the ring tail. pvesh emits single-line JSON: past 64 KiB the partial-buffer split inserts newlines mid-token and breaks the parse. Every consumer fails closed (skip removal / treat as in-use), so impact is a skipped cleanup, not data loss.
**Fix:** When the full-capture executor variant lands (sub:1e8ffb91), route pvesh/find SSH reads through it; current fail-closed behavior makes this non-urgent.
**Effort:** hours

##### `sub:29293401:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** done — PR #926
**Severity:** suggestion
**Cluster:** io-handling — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/distribution/okd/setup/haproxy.go:186-200`
**Problem:** VerifyHAProxyPorts substring-scans `ss -tlnp` output from the ring tail; on a host with >200 listening sockets the head rows vanish and the check logs spurious 'may not be listening' warnings. Diagnostic-only path, warn-only outcome.
**Fix:** Narrow the query (`ss -tln sport = :6443` per port) or use the full-capture variant; warn-only today so opportunistic.
**Effort:** hours

#### audit-state-and-recovery

##### `state:0f076161:destroy-scoped-cleanup-unscoped` — destroy scoped cleanup unscoped

**Status:** done — PR #908
**Severity:** blocker
**Cluster:** destroy-safety — related: state:0f076161:destroy-no-cluster-confirm-without-yes
**Evidence:** `internal/cli/destroy.go:282-290` + 2 more
**Problem:** A scoped destroy (--only=bootstrap/masters/workers/vms or --target) only scopes the terraform step. StepCleanupFiles (cleanup.Full: deletes okd-install incl. kubeconfig + kubeadmin-password, haproxy config, dnsmasq drop-in, terraform.tfvars), StepCleanupFirewall, and StepRemoveRemoteISO still run cluster-wide, so `okdctl destroy --only=workers` tears down the bastion DNS/LB plumbing and credential files of the still-running control plane.
**Fix:** When destroyTargets is non-empty (from --only or --target), default SkipCleanup=true, SkipFirewall=true, KeepISOs=true (or set CleanupKind="") in runDestroy, and say so in the flag help; full bastion teardown stays exclusive to the unscoped destroy. Alternatively reject the combination unless the skip flags are passed explicitly.
**Effort:** days

##### `state:15ba17da:destroy-orphans-custom-isos` — destroy orphans custom isos

**Status:** done — PR #908
**Severity:** minor
**Cluster:** destroy-safety — related: state:0f076161:destroy-scoped-cleanup-unscoped
**Evidence:** `internal/distribution/okd/destroy/steps.go:93-115` + 2 more
**Problem:** StepRemoveRemoteISO only matches fedora-coreos-*.iso, but the VMs boot from the per-node custom ISOs setup uploads (bootstrap0.iso, master<N>.iso, worker<N>.iso — referenced as unmanaged cdrom file_ids in HCL, so terraform destroy never deletes them either). A full `okdctl destroy` therefore strands every multi-GB custom ISO on Proxmox storage indefinitely. Note the generated names carry no cluster prefix, so naive widening of the removal pattern could delete another cluster's ISOs on shared storage.
**Fix:** Extend the destroy ISO step to remove the exact node ISO names derived from cfg topology (bootstrap0.iso, master0..N.iso, worker0..N.iso) through the existing allowlist-validation layering; longer term, prefix generated ISO names with the cluster name so shared-storage collisions are impossible and removal stays name-exact.
**Effort:** hours

##### `state:4c092fce:snapshot-bak-retention-after-destroy` — snapshot bak retention after destroy

**Status:** done — PR #908
**Severity:** suggestion
**Cluster:** tf-state-atomicity — seam→audit-security — related: state:62cb8a95:corrupt-state-silent-destroy-noop
**Evidence:** `internal/infrastructure/terraform/terraform.go:391-453` + 1 more
**Problem:** SnapshotState retains up to 5 terraform.tfstate.<ts>.bak files that nothing ever removes after teardown: Full cleanup deletes the empty live tfstate post-destroy but leaves the .bak snapshots (which contain the full pre-destroy resource state) in the env dir indefinitely. Recoverability-positive during destroy, but after a completed destroy+cleanup they are stale sensitive residue, and warnIfTfStateOnly's recovery-hint logic ignores them.
**Fix:** In the PostDestroy branch of StepCleanupTerraform, when the live tfstate is empty and removed, also remove terraform.tfstate.*.bak (or keep exactly the newest one and log its path as the rollback artefact, matching the CleanupPlans doc-comment philosophy).
**Effort:** hours

##### `state:4c092fce:destroy-direct-no-caller` — destroy direct no caller

**Status:** deferred (audit-positive baseline verified 2026-07-11 — no code change needed)
**Severity:** suggestion
**Cluster:** destroy-safety
**Evidence:** `internal/infrastructure/terraform/terraform.go:343-363`
**Problem:** destroyDirect has no production caller — Destroy is always invoked with UsePlan=true. The doc comment declares it the retained 'emergency destroy' argv shape under regression coverage for a future opt-in caller. Scaffolding per MEMORY.md; recorded for the verify-intent ledger, not for deletion.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

#### audit-iac-and-shell

##### `iac:18a795d5:dup-insecure-comment` — dup insecure comment

**Status:** done — PR #894
**Severity:** suggestion
**Cluster:** hcl-doc-hygiene
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/main.tf:9-10` + 3 more
**Problem:** The PROXMOX_VE_INSECURE env var is documented twice on adjacent comment lines with conflicting framing — one calls it '(optional, set to true to disable tls verification)', the next '(DEV ONLY ... never set in prod)'. A copy-paste artifact duplicated verbatim across four .tf files; the softer line undercuts the security warning.
**Fix:** Delete the softer 'optional, set to true' line in all four files (modules/.../main.tf, modules/.../variables.tf, environments/production/variables.tf, environments/production/versions.tf header), keeping only the DEV-ONLY warning so the security framing is unambiguous.
**Effort:** hours

##### `iac:e076e43c:token-in-argv` — token in argv

**Status:** done — PR #894
**Severity:** suggestion
**Cluster:** install-sh-integrity
**Evidence:** `scripts/install.sh:109-117`
**Problem:** GITHUB_TOKEN is passed to curl as an -H 'Authorization: Bearer ...' argv element. During the brief curl exec it is visible in the process table (ps / /proc/PID/cmdline) to other local users. Low blast radius (the user's own token, short-lived, only when GITHUB_TOKEN is set), but argv is the wrong channel for a bearer secret.
**Fix:** Pass the header off-argv via a curl config file on stdin: `printf 'header = "Authorization: Bearer %s"\n' "$GITHUB_TOKEN" | curl_safe --config - ...`. Keeps the token out of the process table on shared/CI hosts.
**Effort:** hours

#### audit-errors

##### `err:a4001485:vocab-gap-transient` — vocab gap transient

**Status:** deferred (audit-positive baseline verified 2026-07-11 — no code change needed)
**Severity:** suggestion
**Cluster:** domain-vocabulary
**Evidence:** `internal/errtypes/errtypes.go:5-13` + 3 more
**Problem:** Three near-identical ad-hoc retryability classifiers (proxmox.initIsRetryable, addon.addonIsRetryable, download.isRetryable) encode the same transient-vs-permanent concept outside the errtypes vocabulary. The package doc records this as a deliberate deferral until a retry-aware consumer lands (roadmap err:9f8e7d6c); snapshot row keeps the gap visible — it persists this run.
**Fix:** When the roadmap consumer lands, add errtypes.TransientError{Msg, Err} (Unwrap-chaining) and collapse the three classifiers into errors.As-based checks; until then no action — the deferral is documented at the type-definition site.
**Effort:** hours

##### `err:5e892064:vocab-ad-hoc-synonym` — vocab ad hoc synonym

**Status:** done — PR #916
**Severity:** minor
**Cluster:** domain-vocabulary
**Evidence:** `internal/download/checksum.go:74-74` + 1 more
**Problem:** HTTP-status failures rendered as bare strings ('failed to fetch checksums: HTTP %d', 'github api returned status %d') while the same package defines HTTPStatusError specifically so isRetryable can fail-fast on 4xx and retry 5xx. setup/coreos.go already wraps download.HTTPStatusError cross-package. If FetchChecksum or the releases fetcher are ever placed under retryDownload (as Fetch is), 404s silently degrade from fail-fast to retry-everything.
**Fix:** Return &HTTPStatusError{Status: resp.StatusCode, Method: http.MethodGet, URL: checksumsURL} in FetchChecksum, and fmt.Errorf("github api: %w", &download.HTTPStatusError{...}) in releases/fetcher.go, matching the fetchToFile and setup/coreos.go idiom.
**Effort:** hours

##### `err:aa84670c:exit-mapping-nesting-precedence` — exit mapping nesting precedence

**Status:** done — PR #896
**Severity:** suggestion
**Cluster:** typed-error-exit-mapping — seam→audit-cli-ux
**Evidence:** `internal/cli/root.go:212-251`
**Problem:** exitCodeFor checks the five category types in fixed order, and each errors.As walks the whole chain — so an inner ConfigError outranks an outer ClusterError regardless of nesting depth (ClusterError{Msg:'addon flux install failed', Err: …ConfigError} exits 2, not 4). The sentinel-over-category precedence is documented in-line; the category-over-category nesting precedence is not, in either the package doc or docs/cli/exit-codes.md.
**Fix:** Document the precedence ('sentinels outrank categories; among categories, Config > Network > Cluster > Auth > Usage wins anywhere in the chain — root-cause type, not outermost wrap, decides the code') in the exitCodeFor doc comment and docs/cli/exit-codes.md; alternatively switch to an outermost-wins single-Unwrap walk if wrap-level classification is the intended contract.
**Effort:** hours

#### audit-concurrency

#### audit-api-design

##### `api:2c4d8e6b:iface-in-producer` — iface in producer

**Status:** done — PR #910
**Severity:** minor
**Cluster:** interface-location
**Evidence:** `internal/addon/addon.go:50-92` + 3 more
**Problem:** ConfigurableAddon and WizardProvider are implemented by both catalog addons (DefaultSettings/ValidateSettings/DecodeSettings/WizardFields) but no code anywhere type-asserts or consumes them. The intended consumer (wizard addons step) instead imports flux/secretstore concretely and hand-builds the same field catalog, so addon wizard fields, defaults, and validation now live in two places that can drift. Not scaffolding: the consumer exists and bypassed the contract, so the duplication cost is current, not future.
**Fix:** Either wire the wizard addons step to iterate addon.All() and type-assert WizardProvider/ConfigurableAddon to render fields generically (deleting the hand-built duplicates), or record a decision that the wizard owns its field layout and retire the unconsumed interfaces. Verify intent with the owner before either move (MEMORY.md scaffolding protocol).
**Effort:** hours

##### `api:7b2829bb:ring-tail-contract` — ring tail contract

**Status:** deferred (verified 2026-07-11 — already satisfied by commits e293828/48dc087/64196c4; recorded in PR #926)
**Severity:** minor
**Cluster:** exported-surface — seam→audit-subprocess — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/executor/executor.go:25-31` + 2 more
**Problem:** Executor.Run's Result.Stdout/Stderr silently carry only the last 200 lines with no truncation signal and no full-capture sibling API; callers that parse stdout as JSON (8 sites per the wave-1 subprocess audit) consume the tail as if complete. The per-site misuse is owned by audit-subprocess; the API gap — no additive full-capture path — is the design defect here.
**Fix:** Add an additive full-capture API: either RunFull(ctx, name, args...) with a byte-capped full buffer, or a WithCaptureLimit/CaptureAll per-call option, plus a Result.Truncated bool so parse-callers can fail loudly. Migrate the 8 JSON-parsing call sites onto it; streaming/log paths keep the ring.
**Effort:** hours

##### `api:d6b325cb:pkg-import-cycle-adj` — pkg import cycle adj

**Status:** done — PR #921
**Severity:** minor
**Cluster:** package-boundary
**Evidence:** `internal/infrastructure/proxmox/types.go:1-51` + 4 more
**Problem:** internal/infrastructure/proxmox imports internal/distribution/okd/phase for domain vocabulary (NodeRole, VMState) and remote pvesh helpers, inverting infra-under-distribution layering; pvesh.go's own comment concedes the helpers are stranded in phase/ only because moving them would cycle (proxmox already imports phase).
**Fix:** Extract NodeRole/VMState/Condition* plus the pvesh/SSH remote-op helpers (pvesh.go, ssh.go, iso_cleanup.go remote bits) into a neutral leaf package (e.g. internal/proxmoxops) imported by both phase and infrastructure/proxmox. Reverses the infra->distribution edge and dissolves the alias re-export block in types.go.
**Effort:** hours

##### `api:c287d5c0:pkg-facade-bypassed` — pkg facade bypassed

**Status:** done — PR #923
**Severity:** minor
**Cluster:** package-boundary
**Evidence:** `internal/distribution/okd/okd.go:146-154` + 2 more
**Problem:** okd.Provisioner facade is asymmetric: Prepare/Configure build their phase Options internally, but Install requires cli to import okd/install and hand it install.NewOptions(cfg, projectRoot) verbatim. Separately, cli/deploy.go deployDryRunSteps hand-duplicates 32 step IDs AND display names already declared in the phases' StepDefs, importing setup/install/postinstall just for the constants - a drift-prone second source of truth.
**Fix:** (1) Have Provisioner.Install build install.NewOptions(cfg, p.projectRoot) internally, matching Configure; (2) add Provisioner.DeploySteps() returning ID+Name derived from the same xSteps() StepDef slices so the dry-run listing cannot drift. cli/deploy.go then drops its okd/setup, okd/install, okd/postinstall imports (postinstall.Result/UpdateIngressResult remain legitimately exposed).
**Effort:** hours

##### `api:5e892064:ctx-missing-on-io` — ctx missing on io

**Status:** done — PR #916
**Severity:** minor
**Cluster:** ctx-first
**Evidence:** `internal/download/checksum.go:21-54` + 2 more
**Problem:** CalculateChecksum/ValidateChecksum stream entire files through sha256 with no ctx; on multi-GB CoreOS ISOs this runs tens of seconds inside Fetch's skip-check and post-download verification, so Ctrl-C cannot interrupt the hash even though every caller already holds a ctx.
**Fix:** Add ctx as first parameter (CalculateChecksum(ctx, path)) and copy via a ctx-checking reader (check ctx.Err() per 1-4 MiB chunk). Callers Fetch, canSkipDownload, verifyDownloadedFile, and ExtractTarGz already have ctx in scope.
**Effort:** hours

##### `api:d31d1b9d:pkg-facade-bypassed` — pkg facade bypassed

**Status:** deferred (verified 2026-07-11 — already resolved by PR #856, no code change needed; recorded in PR #922)
**Severity:** minor
**Cluster:** package-boundary — seam→audit-code-smells
**Evidence:** `internal/cli/status.go:388-404` + 1 more
**Problem:** cli/status constructs phase.NewBasePhase to reach BasePhase.OcOutput for oc queries, while internal/cluster.Client exists as the thin oc wrapper (with WithKubeconfig built for exactly this). CLAUDE.md scopes BasePhase.Oc* to phase code; cluster.Client's surface is too narrow (no exported output method), so cli reached for distribution internals instead.
**Fix:** Add an exported Output(ctx, args ...string) (string, error) to cluster.Client (it already wraps executor + KUBECONFIG injection) and switch cli status/describe to cluster.New(cluster.WithKubeconfig(kcPath)). BasePhase.Oc* stays untouched for phase code.
**Effort:** hours

##### `api:0934cf1b:iface-in-producer` — iface in producer

**Status:** done — PR #926
**Severity:** suggestion
**Cluster:** interface-location
**Evidence:** `internal/platform/packages.go:18-56`
**Problem:** NewPackageManager returns the PackageManager interface rather than *Manager (violating 'accept interfaces, return structs'), and the interface methods take a per-call *slog.Logger (Remove even ignores it) while every other repo type injects the logger at construction.
**Fix:** Return *Manager from NewPackageManager and move the logger to a Manager field (constructor arg or option), dropping the per-call logger params. Keep the PackageManager interface only if a consumer-side fake needs it — then declare it consumer-side (setup/cleanup).
**Effort:** hours

##### `api:92553fff:export-no-caller` — export no caller

**Status:** deferred (superseded 2026-07-12 — the six summary helpers moved to internal/render as its public API via the cli→render refactor; unexport no longer applies; commit dropped from PR #940)
**Severity:** minor
**Cluster:** exported-surface
**Evidence:** `internal/cli/summary.go:77-277`
**Problem:** internal/cli exports six presentation helpers (DryRunStep, DryRunSummary, ValidationSummary, PostDeploySummary, InterruptSummary, UpdateIngressSummary) with zero callers outside the package; cli's external surface is only Execute/RootCmd/DeferWarn. Not future-CLI-verb shaped (they are called today, in-package).
**Fix:** Unexport the six summary symbols (mechanical rename within package cli). Keeps the package's public surface at the three symbols cmd/ actually consumes.
**Effort:** hours

##### `api:cfcdee2d:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** done — PR #897
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/httputil/httputil.go:18-26`
**Problem:** TimeoutDownload has no caller and is marked scaffolding for a future file-download caller — but the natural caller already exists: download.DefaultTimeout hand-rolls the identical 5-minute value instead of consuming the tier constant, so the scaffolding premise is stale.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:fde34e0c:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** deferred (scaffolding verified 2026-07-11 — kept per MEMORY.md; annotation present)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/cluster/k8s.go:51-72`
**Problem:** cluster.WithEnvFallback has zero call sites anywhere (production or test); it completes the Option family with an env-driven discovery mode documented for non-phase callers that have not landed.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:2be6306e:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** deferred (scaffolding verified 2026-07-11 — annotation already present via 8d17082; PR #910 records it)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/addon/registry.go:86-94`
**Problem:** addon.IsRegistered has no caller; its doc names the future 'okdctl addon validate' verb. The Registry type is also exported although all access flows through package-level functions over the unexported global.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:859eea6f:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** deferred (scaffolding verified 2026-07-11 — kept per MEMORY.md; annotation present)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/noderole.go:21-32`
**Problem:** ParseNodeRole has no caller; retained as the documented deserialization counterpart to NodeRole.String() for upcoming status-JSON/terraform-output parsing.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:3e02f6b8:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** deferred (scaffolding verified 2026-07-11 — kept per MEMORY.md; annotation present)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/vmstate.go:10-19`
**Problem:** Only StateRunning is consumed; StateStopped/Creating/Deleting/Unknown complete the pvesh status matrix for the future partial-cluster status path, per the in-code scaffolding marker.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:0fc0041d:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** deferred (scaffolding verified 2026-07-11 — kept per MEMORY.md; annotation present)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/condition.go:14-29`
**Problem:** ConditionTypeAvailable/Progressing and ConditionStatusUnknown are unreferenced; the const groups intentionally mirror the full Kubernetes condition matrix for the future status verb that surfaces non-Ready operator conditions.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:9ce5434c:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** deferred (scaffolding verified 2026-07-11 — kept per MEMORY.md; annotation present)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/kubectl.go:44-53`
**Problem:** OcPollOutputInterval is exported solely as the test-injection seam for polling cadence; production code must use OcPollOutput per its doc. Exported-but-test-only surface, retained by the in-code scaffolding marker.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:48688e63:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** deferred (scaffolding verified 2026-07-11 — kept per MEMORY.md; annotation present)
**Severity:** suggestion
**Cluster:** ctx-first
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:150-162`
**Problem:** Disconnect accepts a ctx it never uses, kept for signature symmetry with Connect (which now genuinely uses ctx via sshpin.Verify) and for a future network-bound teardown; documented in-code with the same tracking ID.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:de572c63:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** done — PR #897
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/dns/dnsmasq.go:49-64` + 17 more
**Problem:** Pattern-wide: the dns, cleanup, and setup phase packages export symmetric helper families with no callers outside their own package — dns service ops (Enable/Restart/ValidateDnsmasqConfig, ConfigureSystemResolver, IsNetworkManagerActive), cleanup subsystem funcs (WorkDirectory, WebServer, Dnsmasq, Packages, IgnitionCerts, GenerateSummary, ValidKinds...), setup builders (BuildLiveKargs, BuildDestKargs, ExtractNetworkConfig, EnsureIgnitionCert...). Symmetric-API shaped, so severity capped; alternative is a mechanical unexport sweep.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:588ce79e:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started (owner decision needed: 'okdctl theme' scaffolding vs rejected --color wiring — 2026-07-11 verify pass)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/tui/colors.go:9-18`
**Problem:** ColorTheme/ThemeDefault/ThemeHighContrast are exported for a future 'okdctl theme' verb; today only the unexported setTheme consumes them via the HOMELAB_HIGH_CONTRAST env init. MEMORY.md records the user rejecting --color/lipgloss profile wiring, so the future-verb premise deserves an explicit owner check — argued here, not silently upgraded.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

#### audit-cli-ux

##### `ux:024a2c32:json-schema-undoc` — json schema undoc

**Status:** done — PR #896
**Severity:** suggestion
**Cluster:** json-stability — related: sub:d31d1b9d:ring-truncated-stdout-parse
**Evidence:** `docs/cli/json-schema.md:56-67` + 2 more
**Problem:** `okdctl status --output=json` always emits nodes[].status (NodeStatusReady/NotReady, set unconditionally in runStatus) and the NodeStatus struct can additionally emit version, internal_ip, and conditions via omitempty, but the documented field table stops at name/role/ready. The stability promise ('field names are stable') covers fields consumers cannot discover from the doc.
**Fix:** Add nodes[].status to the field table (values: Ready|NotReady|Unknown) and note version/internal_ip/conditions as optional fields reserved for future population, or strip the unused omitempty fields from the CLI-facing projection.
**Effort:** hours

##### `ux:aa84670c:exit-code-undefined` — exit code undefined

**Status:** done — PR #896
**Severity:** suggestion
**Cluster:** exit-codes
**Evidence:** `internal/cli/root.go:157-170`
**Problem:** signalLoop's second-strike path calls exit(130) unconditionally, so a double SIGTERM (e.g. systemd stop escalation) exits 130 (SIGINT code) instead of 143, contradicting the published taxonomy that maps SIGTERM to 143.
**Fix:** Capture the second signal value and exit 143 when it is syscall.SIGTERM, 130 otherwise. Preserve the documented close(sigCh)-after-signal.Stop ordering (con:aa84670c).
**Effort:** hours

##### `ux:e7db1220:flag-completion-inconsistent` — flag completion inconsistent

**Status:** done — PR #896
**Severity:** suggestion
**Cluster:** flag-conventions
**Evidence:** `internal/cli/releases.go:77-83` + 2 more
**Problem:** Enum-valued flags get shell completion inconsistently: --channel (releases list) and --only (destroy) register RegisterFlagCompletionFunc, but the eight --output text|json flags plus --log-level and --log-format complete nothing despite having closed value sets validated at runtime.
**Fix:** Add a shared helper that registers cobra.FixedCompletions([]string{outputText, outputJSON}, NoFileComp) wherever flagOutput is bound, plus log-level (debug,info,warn,error) and log-format (text,json) on the root persistent flags.
**Effort:** hours

##### `ux:fd2125dd:verb-noun-inconsistent` — verb noun inconsistent

**Status:** done — PR #896
**Severity:** suggestion
**Cluster:** verb-noun
**Evidence:** `internal/cli/addon.go:27-31` + 1 more
**Problem:** Sibling noun groups disagree on number: `okdctl addon list` (singular) vs `okdctl releases list` (plural). Same grammatical position, two conventions — users must remember which group pluralizes.
**Fix:** Both names are shipped; renaming is a breaking change bigger than the smell. If desired, add a `release` alias (cobra Aliases) to releasesCmd or `addons` alias to addonCmd and standardize in docs; otherwise record the choice and keep new noun groups singular.
**Effort:** hours

#### audit-observability

#### audit-modernization

##### `mod:48688e63:use-slices-containsfunc` — use slices containsfunc

**Status:** done — PR #921
**Severity:** suggestion
**Cluster:** slices-maps
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:499-503`
**Problem:** Hand-rolled membership loop checking whether any enumerated VM carries vmidBase; slices.ContainsFunc (Go 1.21) collapses it to one expression.
**Fix:** return slices.ContainsFunc(vms, func(v vmIDProbe) bool { return v.VMID == vmidBase }) after naming the anonymous struct type — or keep the loop if naming the type costs more than it saves; behavior identical including the fall-through Info log when false.
**Effort:** hours

##### `mod:e3782ee7:use-errors-is` — use errors is

**Status:** done — PR #932
**Severity:** minor
**Cluster:** errors-and-deprecated-stdlib
**Evidence:** `internal/system/fs.go:91-210` + 16 more
**Problem:** 24 in-scope sites still use os.IsNotExist/os.IsExist, which the os package docs mark as predating errors.Is and which do not unwrap wrapped errors; newer repo code (internal/cli, internal/runlock) already uses errors.Is(err, os.ErrNotExist), and internal/errtypes/errtypes.go:35 explicitly documents that wrapped-sentinel matching relies on errors.Is.
**Fix:** Mechanical swap: os.IsNotExist(err) -> errors.Is(err, fs.ErrNotExist) (or os.ErrNotExist, matching internal/cli usage), os.IsExist(err) -> errors.Is(err, fs.ErrExist); add errors / io/fs imports as needed. One-for-one, no behavior change for unwrapped os errors, fixes latent mismatch for wrapped ones.
**Effort:** hours

#### audit-code-smells

##### `smell:4ded56d3:retry-scaffold-triplicated` — retry scaffold triplicated

**Status:** in review — PR #940
**Severity:** minor
**Cluster:** dual-impl-same-job
**Evidence:** `internal/download/retry.go:84-112` + 2 more
**Problem:** Three packages hand-roll the identical retry scaffold around wait.ExponentialBackoffWithContext: same backoff constants (5s, factor 2, jitter 0.5, 3 steps, 5m cap), same lastErr-preservation tail so backoff exhaustion returns the real error instead of the wait sentinel, and three near-identical isRetryable classifiers. The copies even cross-reference each other in comments ("mirrors retryDownload"), confirming they are meant to be one thing.
**Fix:** Extract one helper, e.g. system.RetryBackoff(ctx, backoff wait.Backoff, retryable func(error) bool, fn func() error) (attempts int, err error) — internal/system already hosts the analogous WaitFor family and is imported by all three packages. Each caller keeps its own retryable classifier (download's HTTP-status logic, addon's exec.ErrNotFound case, proxmox's ConfigError/AuthError case). Risk medium: the three classifiers differ deliberately; only the backoff-loop shell is shared.
**Effort:** hours

##### `smell:4c092fce:pipeline-explicit-errors` — pipeline explicit errors

**Status:** done — PR #907
**Severity:** minor
**Cluster:** arrow-anti
**Evidence:** `internal/infrastructure/terraform/terraform.go:212-227` + 8 more
**Problem:** Eight call sites across four packages repeat the same 5-line wrap dance: `if hint := tf.LockHint(); hint != nil { return errors.Join(hint, wrapped) } return wrapped`. The lock-hint enrichment is part of the terraform.Executor error contract but is hand-assembled by every caller, so new callers (and one existing destroy path: destroyInfrastructure's Destroy call at destroy/helpers.go:49-60 skips it) can forget the hint.
**Fix:** Add `func (t *Executor) WithLockHint(err error) error` to terraform.go: nil-in/nil-out, joins LockHint when present. Call sites become `return t.WithLockHint(&errtypes.ClusterError{...})`. Alternatively fold into Executor.run so every state-locking subcommand error carries the hint automatically.
**Effort:** hours

##### `smell:0139cb3f:magic-path-literal` — magic path literal

**Status:** in review — PR #940
**Severity:** minor
**Cluster:** magic-strings
**Evidence:** `internal/distribution/okd/phase/paths.go:130-137` + 16 more
**Problem:** The terraform environment directory is assembled inline as filepath.Join(root, "infrastructure", "terraform", "environments", env) at 16 sites across 8 packages, even though phase/paths.go already hosts the sibling helpers GetTerraformEnv and ClusterConfigDir. Any layout change (or a typo in one segment) drifts per-site with no compile check.
**Fix:** Add `func TerraformEnvDir(projectRoot, env string) string` next to GetTerraformEnv in phase/paths.go (CLAUDE.md names phase/ as the home for cross-phase helpers) and replace the 16 Join sites. infrastructure/proxmox already imports phase, so no import-cycle risk. cli/debug_bundle.go's bare "production" fallback (L262) collapses into the same helper path or stays with a comment.
**Effort:** hours

##### `smell:6424733c:pipeline-explicit-errors` — pipeline explicit errors

**Status:** in review — PR #940
**Severity:** suggestion
**Cluster:** arrow-anti
**Evidence:** `internal/cli/helpers.go:349-385` + 2 more
**Problem:** executeFullDeployment repeats a near-identical 8-line cancellation block after each of the three phase calls (errors.Is(context.Canceled) -> InterruptSummary -> phase-specific hint -> return) differing only in the hint string and the steps slice. Separately, the tfstate-recovery glob is pasted twice in the same file (hasProjectMarker and warnIfTfStateOnly).
**Fix:** Extract `func deployPhaseErr(w io.Writer, err error, steps []distribution.StepResult, runID, cancelHint string) error` collapsing the three blocks; extract `tfStateGlob(root string) []string` for the duplicated glob. Pure consolidation, no behavior change.
**Effort:** hours

##### `smell:451be4fa:magic-path-literal` — magic path literal

**Status:** in review — PR #940
**Severity:** major
**Cluster:** magic-strings
**Evidence:** `internal/system/elevation.go:140-144` + 13 more
**Problem:** The workdir name "okd-install" is a bare string literal at 13 filepath.Join sites across cli and all five okd phase packages, and the same literal is independently duplicated inside the sudo chown-back security allowlist (isAllowedChownRoot). A rename at the Join sites silently stops matching the allowlist, disabling the chown-back guard without any compile error.
**Fix:** Declare a single exported constant in a leaf package both sides can import (internal/system, which phase/ and cli/ already import: `const WorkDirName = "okd-install"` plus optionally `func WorkDir(projectRoot string) string`). Replace all 13 Join literals and the elevation.go allowlist comparison with the constant. "infrastructure" in the same allowlist gets the same treatment.
**Effort:** hours

##### `smell:2c4d8e6b:interfaceany-lazy-exported` — interfaceany lazy exported

**Status:** done — PR #910
**Severity:** suggestion
**Cluster:** interfaceany-lazy — seam→audit-api-design
**Evidence:** `internal/addon/addon.go:58-58` + 4 more
**Problem:** ConfigurableAddon.DecodeSettings returns (any, error), but grep shows zero generic consumers: the only callers are the implementing addons themselves, each immediately type-asserting its own concrete Settings (decoded.(Settings)) — an unchecked assertion that would panic if the method ever returned a different type. The any in the interface buys no polymorphism and costs a panic path per addon.
**Fix:** Either drop DecodeSettings from the interface (each addon keeps a package-private typed decode — nothing external calls it generically today), or keep the interface method but have each addon call an unexported typed decodeSettings internally so the any round-trip and unchecked assertions disappear. If a future generic consumer (wizard validation) is planned, document it on the interface; the interface-shape decision itself is audit-api-design territory.
**Effort:** hours

##### `smell:40d315ad:settings-stringified-numbers` — settings stringified numbers

**Status:** done — PR #910
**Severity:** suggestion
**Cluster:** stringified-numbers
**Evidence:** `internal/addon/catalog/flux/flux.go:581-588` + 4 more
**Problem:** controller_timeout/git_sync_timeout defaults are stored as the strings "300"/"180" and re-parsed with strconv.Atoi at each use via getTimeout reading the raw settings map — bypassing the typed Settings struct that DecodeSettings exists to produce (the struct simply omits both fields). A malformed value silently falls back to the default instead of failing validation.
**Fix:** Add ControllerTimeout/GitSyncTimeout time.Duration fields to flux.Settings, parse once in DecodeSettings (returning an error on malformed input so ValidateSettings surfaces it), and have waitForControllers/waitForGitSync read the struct. Delete getTimeout. The map[string]string wire format stays — only the parse point moves.
**Effort:** hours

##### `smell:0f076161:enum-ad-hoc` — enum ad hoc

**Status:** done — PR #908
**Severity:** suggestion
**Cluster:** magic-strings
**Evidence:** `internal/cli/destroy.go:109-130`
**Problem:** validateDestroyTargets switches on the raw regex capture with case "bootstrap" / "master" / "worker" string literals even though phase.NodeRole typed constants (RoleBootstrap/RoleMaster/RoleWorker) and ParseNodeRole exist exactly for this vocabulary, and destroy.go already imports phase. The same literals also appear baked into destroyTargetRE — acceptable there (regex alternation), but the switch should speak the typed vocabulary.
**Fix:** switch phase.NodeRole(m[1]) { case phase.RoleBootstrap: ... case phase.RoleMaster: ... case phase.RoleWorker: } — or run the capture through phase.ParseNodeRole, which exists as the canonical deserializer (its doc note "currently no caller" resolves itself).
**Effort:** hours

##### `smell:90fa855c:coreos-iso-glob-dup` — coreos iso glob dup

**Status:** in review — PR #942
**Severity:** suggestion
**Cluster:** dual-impl-same-job — re-queued from PR #884's skipped-items section (reverted there by a stale-local-linter false gate)
**Evidence:** `internal/distribution/okd/setup/coreos.go:100-123`
**Problem:** The pattern-glob → slices.Max → logISOFound scan loop is duplicated byte-for-byte for isoDir (L100-110) and workISODir (L113-123); any change to the pattern list or newest-selection rule must be made twice or the two search paths silently diverge.
**Fix:** Extract a findNewestISO(dir string, patterns []string) (string, bool) helper and call it for both directories.
**Effort:** hours

#### audit-dependencies

##### `dep:b803fcb7:version-floor-unjustified` — version floor unjustified

**Status:** done — PR #894
**Severity:** minor
**Cluster:** pin-stability
**Evidence:** `.github/workflows/ci.yml:56-56` + 2 more
**Problem:** Go-tool pins in CI and the Makefile are explicit versions (policy-compliant) but sit outside every renovate manager — the custom regex manager only matches annotated lines in .env/.sh/.yaml and the gomod manager only reads go.mod — so they rot silently: govulncheck pinned v1.1.4 vs latest v1.3.0 (the security gate itself is two minors stale), yamlfmt v0.14.0 vs v0.21.0, air v1.61.7 vs v1.65.3.
**Fix:** Add renovate annotations above each pin ('# renovate: datasource=go depName=golang.org/x/vuln') and extend customManagers.json5 managerFilePatterns to cover /\.yml$/ and /Makefile$/; or move govulncheck/yamlfmt to go.mod 'tool' directives (Go 1.24+) so the gomod manager tracks them; then bump govulncheck to v1.3.0, yamlfmt to v0.21.0, air to v1.65.3.
**Effort:** hours

##### `dep:33ef32bf:dup-log-engines` — dup log engines

**Status:** done — PR #895
**Severity:** suggestion
**Cluster:** duplicate-engine
**Evidence:** `go.mod:11-11` + 3 more
**Problem:** Four log engines compile into the production binary: stdlib log/slog (canonical sink per CLAUDE.md), charm.land/log/v2 (direct, single call-site styled stderr formatter), and go-logr/logr + k8s.io/klog/v2 (pinned by k8s.io/apimachinery, both linked per go list -deps). CLAUDE.md carries a YAML-engine baseline tripwire but no equivalent log-engine baseline, so a fifth engine could land without a recorded justification.
**Fix:** Record a log-engine baseline in CLAUDE.md §dependencies mirroring the YAML tripwire: slog = canonical, charm.land/log/v2 = intentional UI formatter (1 file), logr/klog = k8s-pinned indirects; do not add a fifth without justification. No code change — charm libs are the intentional UI stack and klog/logr are upstream-locked.
**Effort:** hours

##### `dep:6ebdb617:dep-registry-drift` — dep registry drift

**Status:** done — PR #895
**Severity:** minor
**Cluster:** maintenance-signal
**Evidence:** `CLAUDE.md:234-247` + 1 more
**Problem:** CLAUDE.md dependency registry has drifted from go.mod in three places: go-proxmox is documented at v0.5.x but go.mod pins v0.7.1; the joho/godotenv LICENCE-spelling entry remains but the dep is no longer in go.mod or go.sum; §tooling says 'Go 1.25 / toolchain 1.26.2' while go.mod declares go 1.26.0 / toolchain go1.26.4.
**Fix:** Update CLAUDE.md: bump go-proxmox registry row to v0.7.x, delete the godotenv entry (dep removed), and correct §tooling to 'Go 1.26 / toolchain 1.26.4' (or make it version-agnostic: 'go.mod go/toolchain directives are authoritative; don't downgrade').
**Effort:** hours

##### `dep:33ef32bf:version-floor-unjustified` — version floor unjustified

**Status:** done — PR #895
**Severity:** minor
**Cluster:** justified-version-floor — related: dep:6ebdb617:dep-registry-drift
**Evidence:** `go.mod:39-39` + 1 more
**Problem:** Transitive version floors inherited from go-proxmox are years stale and both packages compile into the shipped binary: gorilla/websocket v1.4.2 (Mar 2020; latest v1.5.3, Jun 2024) and jinzhu/copier v0.3.4 (2021; latest v0.4.0). CLAUDE.md claims okdctl 'does not reach' websocket — true at the call-graph level, but `go list -deps ./cmd/okdctl` shows both modules linked into the release artifact.
**Fix:** go get github.com/gorilla/websocket@v1.5.3 github.com/jinzhu/copier@v0.4.0 && go mod tidy — okdctl calls neither API directly, so the floor lift is behavior-neutral; also sharpen the CLAUDE.md websocket entry to say 'linked but never called' rather than 'does not reach it'.
**Effort:** hours

##### `dep:b803fcb7:pin-action-trailer-imprecise` — pin action trailer imprecise

**Status:** done — PR #894
**Severity:** minor
**Cluster:** pin-stability — related: dep:6ebdb617:dep-registry-drift
**Evidence:** `.github/workflows/ci.yml:15-19` + 6 more
**Problem:** All actions are SHA-pinned (policy-compliant) but several version trailers are major-only ('# v6', '# v9', '# v7', '# v4', '# v2') where CLAUDE.md prescribes 'uses: owner/action@<40-hex-sha> # vX.Y.Z' — a reviewer cannot tell which minor/patch a 40-hex digest corresponds to without resolving it. The same files contain the precise form as counter-examples (setup-tflint '# v6.2.2', cosign-installer '# v4.1.2', slsa generator '# v2.1.0').
**Fix:** Point renovate at precise tags so the trailer renders as vX.Y.Z (pin actions to the full release tag before digest-pinning, e.g. actions/checkout@<sha> # v6.0.2), or amend CLAUDE.md to permit major-tag trailers for renovate-managed digests — pick one so policy and practice agree.
**Effort:** hours

##### `dep:33ef32bf:transitive-heavy-narrow` — transitive heavy narrow

**Status:** done — PR #895
**Severity:** suggestion
**Cluster:** transitive-weight — related: dep:33ef32bf:version-floor-unjustified
**Evidence:** `go.mod:12-12` + 1 more
**Problem:** go-proxmox v0.7.1 is imported in exactly one file (wizard REST discovery) yet links 5 extra modules into the release binary: diskfs/go-diskfs (full ISO9660/MBR filesystem stack), buger/goterm, jinzhu/copier, djherbis/times, gorilla/websocket. Known case per CLAUDE.md §dependencies (bus-factor 1, ~200 LOC REST-rewrite fallback); re-confirmed this run: upstream is active (v0.7.1 released 2026-06-02, 267 stars, Apache-2.0, 0 open issues) and the abandonment plan remains valid.
**Fix:** No action now — re-confirmation of the accepted trade. If upstream stalls or a v0.8 minor breaks the wizard, execute the documented fallback: ~200 LOC net/http REST client for the discovery endpoints, dropping 6 modules from the binary.
**Effort:** hours

#### audit-documentation

##### `doc:aa84670c:doc-comment-stale` — doc comment stale

**Status:** done — PR #896
**Severity:** minor
**Cluster:** exported-doc — related: doc:b3356305:readme-flag-ghost, ux:073d24ed:concept-named-twice
**Evidence:** `internal/cli/root.go:33-38`
**Problem:** The cfgFile doc comment says it is "read by subcommand RunE handlers (deploy, destroy, update-ingress)". deploy never reads cfgFile (it uses deployOutputFile), while six undocumented commands do read it (addon, cleanup, config, debug-bundle, doctor, status). The stale reader list misleads at exactly the spot where the deploy/--config drift (ux:073d24ed) lives.
**Fix:** Drop the parenthetical command list (it rots on every new subcommand) or replace with "read by every config-consuming subcommand except deploy, which manages its own file via --output-file".
**Effort:** hours

##### `doc:acb745e5:stepbuilder-doc-self-referential` — stepbuilder doc self referential

**Status:** deferred (superseded 2026-07-11 — PR #923 deletes NewStepBuilder and the inline echo comment entirely)
**Severity:** suggestion
**Cluster:** exported-doc — re-queued from PR #884's skipped-items section (reverted there by a stale-local-linter false gate)
**Evidence:** `internal/distribution/step.go:97-109`
**Problem:** NewStepBuilder's doc states the fatal-by-default contract and the constructor body restates it inline (`fatal: true, // default to fatal`) — a narrating echo of the doc one screen up, against the comment policy's echo/self-reference rules.
**Fix:** Keep the doc sentence; delete the inline `// default to fatal` echo.
**Effort:** hours

##### `doc:c3dc10bb:flux-gettimeout-doc-orphaned` — flux gettimeout doc orphaned

**Status:** deferred (superseded 2026-07-12 — develop refactored getTimeout into settings.go::parseTimeoutSetting; the orphaned doc no longer exists; commit dropped from PR #942)
**Severity:** suggestion
**Cluster:** exported-doc — re-queued from PR #884's skipped-items section (reverted there by a stale-local-linter false gate)
**Evidence:** `internal/addon/catalog/flux/flux.go:565-568`
**Problem:** getTimeout's doc comment ("reads a timeout setting (in seconds)…") is stranded directly above readKeyFile's own doc — the function it describes sits at L581 with no doc, so readKeyFile carries two doc blocks and godoc attributes the wrong one.
**Fix:** Move the two-line doc onto getTimeout at L581, or delete it if the signature is judged to carry the signal.
**Effort:** hours

#### audit-tests

##### `tst:b38ec9cc:destructive-happy-untested` — destructive happy untested

**Status:** done — PR #907
**Severity:** major
**Cluster:** destructive-untested — seam→audit-state-and-recovery — related: state:0f076161:destroy-scoped-cleanup-unscoped
**Evidence:** `internal/distribution/okd/install/workers.go:22-76`
**Problem:** StartWorkerVMs runs a live terraform apply and nothing locks its two safety properties: the -target scoping to the worker VM resource (the in-code comment admits an unscoped apply would reconcile the full state) and the snapshot-before-apply ordering. workersAlreadyRunning's node-count parse is also untested.
**Fix:** Use the fake-terraform-binary harness from destroy/helpers_test.go: capture argv, assert -target=module.okd_cluster.proxmox_virtual_environment_vm.worker and -var start_workers_immediately=true are present, and that a state snapshot exists before apply runs. Table-test workersAlreadyRunning line counting (0 workers, exact count, cluster-unreachable→false,nil).
**Effort:** hours

##### `tst:40d315ad:destructive-happy-untested` — destructive happy untested

**Status:** done — PR #910
**Severity:** minor
**Cluster:** destructive-untested
**Evidence:** `internal/addon/catalog/flux/flux.go:255-270` + 1 more
**Problem:** The addon uninstall paths — flux's `oc delete ns flux-system` and secretstore's per-secret `oc delete secret` + `oc delete secretstore` loop — have no tests, while the install-side builders in the same packages are well covered. Partial-failure semantics (one secret fails to delete) are unlocked.
**Fix:** Fake-oc harness: assert delete argv targets exactly the addon-owned namespace/secret names (no wildcard), and that a single failed secret delete continues/aggregates per the intended semantics rather than aborting the loop silently.
**Effort:** hours

### Tier A — holistic review 2026-06-10

Captured from `holistic-review` run on 2026-06-10 (HEAD `46d11fa`). Items are
judgment-shaped (not audit atoms); each has a 1-3 sentence rationale inline.
Run focus: stripping AI-generated code smells. Headline: comment slop is
already gone (zero hits for narration/dividers/peacock/bare-TODO patterns);
what remains is structural — dead knobs, fabricated data, unreachable states,
and ceremony layers.

#### A1 — Collapse the step framework's triple representation

- **Status:** done — PR #923
- **Category:** architecture
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/distribution/step.go:33-79`, `internal/distribution/step.go:238-263`
- **Rationale:** The same step data exists in three shapes (StepDef struct, fluent StepBuilder, builtStep wrapper) plus a capability-interface tower (Step/Skipper/FatalChecker/StepCallbacks) whose members are all mandatory with a single implementation and single consumer. Two review agents converged on this independently.
- **Acceptance:**
  - StepDef + BuildSteps remains the only construction surface; the 13-method fluent StepBuilder is deleted or unexported — grep confirms it has zero callers outside BuildSteps itself
  - The Step/Skipper/FatalChecker/StepCallbacks interface decomposition collapses: keep one flat interface or have Orchestrator consume a concrete step type; AlreadyDoneChecker stays only if the optional type-assertion at orchestrator.go:151 keeps paying for itself
  - BuildSteps stops re-translating StepDef field-by-field through builder setters with nil guards; a step is built directly from the def
  - Stale doc references to the builder (e.g. phase/helpers.go WarnOnError "Use with StepBuilder.OnError()") updated to StepDef vocabulary
- **Depends on:** none

#### A2 — Unify the two parallel kubectl/oc invocation layers

- **Status:** done — PR #922
- **Category:** architecture
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/cluster/k8s.go:141-160`, `internal/distribution/okd/phase/kubectl.go:16-44`
- **Rationale:** internal/cluster is a 3-file package wrapping executor to run oc, consumed by exactly one file (install/monitor.go) — which is phase code, where BasePhase.Oc* is canonical. Two independently-derived oc conventions for the same job. Shelling out itself is deliberate (oc is already a required binary; no client-go dep; version-skew safety) — only the duplication goes.
- **Acceptance:**
  - One convention for shelling out to oc/kubectl: either internal/cluster becomes the single client that BasePhase.Oc* delegates to, or cluster's CSR methods move into the phase layer and the package is dissolved — decide once and record it in CLAUDE.md architecture notes
  - cluster.Client's unique value (KUBECONFIG env wiring, WithEnvFallback validation, subcommand-only error formatting to avoid --from-literal secret leakage) is preserved wherever the surviving layer lives
  - install/monitor.go remains testable via its existing csrApprover seam
- **Depends on:** none

#### A3 — Fold system.RunCaptured/OutputCaptured into executor

- **Status:** done — PR #926
- **Category:** refactor
- **State:** well-specified
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/system/exec.go:59-122`, `internal/executor/executor.go:243-288`
- **Rationale:** Two packages independently maintain capped stderr capture, allowlist env filtering, soft-cancel + 30s WaitDelay, and a Redacted() subprocess error — and have already drifted (SubprocessError's uncapped StderrTail vs ExitError's 400-byte truncation). Drift between duplicate redaction paths is how a credential leaks through one stack but not the other. The differences are option-shaped, not architecture-shaped.
- **Acceptance:**
  - One subprocess stack: executor gains cancel-signal selection (SIGTERM default, SIGINT for terraform's state-lock release) and a capped/discarded-stdout capture mode; the 9 RunCaptured/OutputCaptured call sites migrate
  - One typed subprocess-failure error survives, keeping the errors.As contract used at platform/packages.go:101
  - SIGTERM-vs-SIGINT rationale comments and the env-allowlist guarantee survive the merge verbatim
  - internal/system's package doc no longer claims command execution
- **Depends on:** none

#### A4 — Fix double-cumulated histogram buckets in deploymetrics

- **Status:** done — PR #893
- **Category:** correctness
- **State:** well-specified
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/deploymetrics/metrics.go:107`, `internal/deploymetrics/metrics.go:113`
- **Rationale:** The histogram serializer cumulates bucket counts twice: counts[i] is already cumulative (incremented for every bucket with s <= bound, no break), then a second `cumulative +=` pass accumulates again — one 0.05s sample renders buckets 1,2,3,...,12 against +Inf=1. Every scrape of okdctl_deploy_step_duration_seconds is currently wrong, and the package has zero tests.
- **Acceptance:**
  - counts[i] is emitted directly and the second accumulation is removed — or the inner loop breaks after the first matching bucket
  - a test feeds known samples and asserts bucket monotonicity and that no le<+Inf bucket exceeds the +Inf bucket
  - coverage floor added for internal/deploymetrics in .github/coverage-floors.conf
- **Depends on:** none

#### A5 — Strip audit-hash comment tokens from source

- **Status:** done — PR #932
- **Category:** docs hygiene
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/cluster/types.go:7`, `internal/infrastructure/proxmox/proxmox.go:32`
- **Rationale:** ~25 source comments carry opaque audit-pipeline hash tokens (`api:bb4fb1a3`, `state:48688e63`, `roadmap err:9f8e7d6c`) that an outside reader cannot resolve — the most distinctive AI-workflow residue left in the source. The invariants behind them are real and stay; only the tokens go (maintainer directive: remove, don't standardize).
- **Acceptance:**
  - all ~25 in-source tokens are removed; the surviving comment states the invariant in plain English (delete the whole comment only when the token was its sole content)
  - CLAUDE.md comment policy gains one line forbidding audit/roadmap hash tokens in source comments so future audit loops don't reintroduce them
- **Depends on:** none

#### A6 — Reconcile the 3% comment target with revive `exported`

- **Status:** in review — PR #941
- **Category:** architecture / policy
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `.golangci.yml:67`, `internal/distribution/okd/okd.go:43`
- **Rationale:** Production comment density is 11.9% (3,730/31,402 lines) and cannot fall below ~8% while revive `exported` mandates a doc comment on all 668 exported funcs in internal/ — 56% of functions in internal-only packages are exported. The CLAUDE.md 3% target and the lint config contradict each other, so every grooming pass chases an unreachable number.
- **Acceptance:**
  - decide the policy: revise the target to match reality, scope the revive rule, or shrink the export surface
  - unexport sweep limited to identifiers with zero cross-package consumers AND no scaffolding/API-shape annotation, which deletes their mandated echo-docs as a side effect
  - the scaffolding carve-out is honored: annotated symmetric APIs and future-command shapes keep their exports
- **Depends on:** none

#### A7 — Delete the two accidental leftovers in the wizard stack

- **Status:** done — PR #928
- **Category:** refactor
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/tui/wizard/components/selector.go:388`, `internal/tui/wizard/steps/keymap_help.go:1`
- **Rationale:** Two genuine accidental-scaffolding artifacts: CompactSelector.ViewHorizontal is a generated alternate-layout method never wired into any view (zero call sites; review.go uses View()), and a help-label const block is copy-pasted across wizard/keymap_help.go and wizard/steps/keymap_help.go — the copies have already drifted by one entry.
- **Acceptance:**
  - CompactSelector.ViewHorizontal and its supporting SetWidth/width field are removed — not future-API shape, so the scaffolding carve-out does not apply
  - the help-label const block lives once in wizard (steps already imports wizard for KeyBinding)
- **Depends on:** none

#### A8 — Put a test floor under the wizard's data-driven core

- **Status:** done — PR #928
- **Category:** test honesty
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/tui/wizard/datadriven.go:455`, `internal/tui/wizard/steps/node_placement.go:380`
- **Rationale:** The wizard stack is 6,916 LOC with zero tests and no floor while every adjacent layer is tested — and its Apply/Validate logic writes the config that drives destructive deploys. The framework half is plain data transformation; leaving it untested is the maturity heatmap's one inconsistency with how the repo treats critical surface.
- **Acceptance:**
  - pure-logic surfaces get terminal-free tests: DataDrivenStep Value/setValue/Validate/Apply round-trips, steps/validators.go, parseAdditionalNetworks, per-step Apply(cfg) mutations
  - coverage floors added for internal/tui/wizard and internal/tui/wizard/steps (components/view rendering explicitly out of scope — no teatest requirement)
  - scope targets only the config-integrity path, not TUI rendering
- **Depends on:** none

#### A9 — Remove or wire the dead Deployment.Debug and SkipDepsCheck knobs

- **Status:** done — PR #928
- **Category:** refactor
- **State:** well-specified
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/distribution/okd/phase/paths.go:26`, `internal/tui/wizard/steps/advanced.go:115`
- **Rationale:** The wizard sets these knobs and the review screen displays them, but nothing downstream reads them — BaseOptions.Debug is never written anywhere, so install's opts.Debug branch is provably dead. Users toggling "debug mode" or "skip deps check" get silent no-ops.
- **Acceptance:**
  - cfg.Deployment.Debug and cfg.Deployment.SkipDepsCheck either gain a real consumer in deploy logic or are deleted from the schema, defaults, wizard advanced step, and review screen
  - phase.BaseOptions.Debug is either populated from cfg at every NewOptions site or removed; the always-false opts.Debug branch in install DeployInfrastructure is resolved either way
- **Depends on:** none

#### A10 — Stop fabricating ProvisionResult VM state in proxmox.Provision

- **Status:** done — PR #921
- **Category:** domain-model accuracy
- **State:** design needed
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/infrastructure/proxmox/proxmox.go:304`, `internal/distribution/okd/install/phase.go:151`
- **Rationale:** retrieveProvisionResult builds a VMStatus list where every VM is unconditionally StateRunning (before any VM was observed running) and APIServerIP is set to the network gateway — a different machine — then the sole caller discards the result with `_, err :=`. Fabricated domain data is worse than scaffolding: a future consumer would trust values that are wrong by construction.
- **Acceptance:**
  - Provision either returns error-only, or ProvisionResult carries values that are true (real VM status from pvesh/terraform output, real API endpoint) rather than config arithmetic
  - no VMStatus is hardcoded to StateRunning before any VM is observed running; APIServerIP is no longer the network gateway
- **Depends on:** none

#### A11 — Collapse the zero-consumer ValidationScope bitmask

- **Status:** done — PR #928
- **Category:** refactor
- **State:** well-specified
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/config/validation_types.go:70`, `internal/cli/config.go:55`
- **Rationale:** A 10-flag bitmask with composite ScopeQuick, an options struct, and two exported entry points — and every production caller invokes plain cfg.Validate() (ScopeAll). ScopeFeatures maps to no validator entry, and the doc comment describes a wizard integration that doesn't exist. Textbook speculative configuration.
- **Acceptance:**
  - either cfg.Validate() is the only entry point (bitmask, ScopeQuick, ValidationOptions, ValidateWithOptions, HasScope deleted) or a real caller exercises a non-ScopeAll scope
  - ScopeFeatures no longer exists as an enum value; doc comments no longer claim ScopeQuick is "used during interactive editing" unless it is
- **Depends on:** none

#### A12 — Make ClusterPhase lifecycle states reachable or trim them

- **Status:** in review — PR #942
- **Category:** domain-model accuracy
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/distribution/okd/types.go:42`, `internal/cli/status.go:189`
- **Rationale:** Six documented lifecycle states, three forever unreachable: status.go only ever produces Unknown/Running/Degraded, NodeStatusUnknown is never assigned, and the documented exit-code mapping doesn't exist. The JSON contract advertises states consumers will never see.
- **Acceptance:**
  - every ClusterPhase constant is producible by `okdctl status` (derive Pending from missing infra, Installing from terraform state present + API down, Failed from an install-failure marker) or the unreachable values and phase.NodeStatusUnknown are removed
  - the ClusterPhase doc comment's exit-code-mapping claim matches reality — implement the mapping or drop the claim
- **Depends on:** none

#### A13 — Expose or explicitly shelve the three unreachable cleanup kinds

- **Status:** done — PR #908
- **Category:** cli surface
- **State:** design needed
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/distribution/okd/cleanup/cleanup.go:30`, `internal/cli/cleanup.go:121`
- **Rationale:** Three of five cleanup kinds (WebOnly, HAProxyOnly, TerraformOnly) are fully implemented through the step-selection switch and summary rendering, yet no flag or caller can select them — finished work that's unshippable. PreserveConfig is a separate constant-false thread-through knob.
- **Acceptance:**
  - WebOnly, HAProxyOnly, and TerraformOnly are reachable from the CLI (e.g. a long-form `--kind` flag on `okdctl cleanup`, validated by ValidKinds) or carry a scaffolding tag explaining the intended future command
  - cleanup.Options.PreserveConfig gains a caller or is removed from Options and the WorkDirectory signature
- **Depends on:** none

#### A14 — Validate the triple-encoded bastion identity in config

- **Status:** done — PR #928
- **Category:** config coherence
- **State:** design needed
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/tui/wizard/steps/files.go:79`, `internal/tui/wizard/steps/networking.go:181`
- **Rationale:** One physical host (the bastion) is encoded in three independent config keys — Networking.Bastion.IP, HTTPServer.IgnitionServerIP, Networking.StaticIP.DNS — kept consistent only by wizard ConfigSet hooks. A user hand-editing okdctl.yaml (the documented non-wizard path) can desync them and get a deploy that fails deep in install with no config-time diagnostic.
- **Acceptance:**
  - a validator (or load-time defaulting) covers the relationship between the three fields — desync produces a clear ValidationError or a documented intentional-override path
  - the wizard-only sync hooks remain, but they are no longer the sole coherence mechanism
- **Depends on:** none

#### A15 — Fix the TerraformEnv knob's fictional value space and mislabeled help

- **Status:** done — PR #928
- **Category:** config coherence
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/tui/wizard/steps/advanced.go:142`, `internal/distribution/okd/phase/paths.go:22`
- **Rationale:** Only environments/production exists on disk, the doc comment advertises "production|staging|...", and the wizard help mislabels the concept as a Terraform workspace — wrong domain vocabulary that sends users to `terraform workspace` commands that do nothing here.
- **Acceptance:**
  - wizard help and doc comments stop calling TerraformEnv a "terraform workspace name" (it selects a directory under infrastructure/terraform/environments/) and stop suggesting "staging" exists
  - a non-default TerraformEnv value is validated against an existing environment directory at config-validation time instead of failing mid-deploy
- **Depends on:** none

#### A16 — Rewrite root command help: strip marketing slop and false feature claims

- **Status:** done — PR #896
- **Category:** docs / CLI UX
- **State:** well-specified
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/cli/root.go:61-74`, `README.md:18`
- **Rationale:** The root help says "production-ready" (the README explicitly disclaims it), "delightful CLI tool", "beautiful TUI", a Highlights bullet list, an addon list naming nonexistent addons, and a hardcoded version range that drifts — the purest LLM fingerprint in the repo, propagated into every generated doc page footer.
- **Acceptance:**
  - Short/Long no longer say "production-ready", "delightful CLI tool", "beautiful TUI", or carry a Highlights bullet list — match the README's first-paragraph register
  - drop the addon list "(Flux, secrets, storage, cert-manager)" (only flux and secretstore exist) and the hardcoded "OKD/OpenShift 4.15-4.21" range
  - regenerate docs/cli (`make docs`) so the footer disappears from all 25 reference pages
- **Depends on:** none

#### A17 — Fix inverted sudo/elevation paragraph in exit-codes.md and reconcile README

- **Status:** done — PR #896
- **Category:** docs
- **State:** well-specified
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `docs/cli/exit-codes.md:25-27`, `internal/cli/elevation.go:78-84`
- **Rationale:** The paragraph is plausible-sounding, self-contradictory ("rejected as root; use sudo"), and factually inverted against elevation.go's truth table — euid=0 + requiresRoot is allowed, and the binary re-execs itself INTO sudo, never "under the original user". An operator following it on destroy gets advice the binary will not honor.
- **Acceptance:**
  - the rewritten paragraph states the real contract: privileged commands self-elevate via sudo re-exec; sudo/root invocation of non-privileged commands is what exits 5
  - README's "It refuses to start under sudo" is narrowed to match (it only refuses sudo on commands that don't need root)
- **Depends on:** none

#### A18 — Make deploy honor --config instead of silently ignoring it

- **Status:** done — PR #928
- **Category:** CLI UX / architecture
- **State:** design needed
- **Effort:** days
- **Impact:** large
- **Evidence:** `internal/cli/deploy.go:39`, `internal/cli/root.go:33-36`
- **Rationale:** Three independent surfaces (deploy's own Example, README line 99, a load-bearing root.go comment) all assert deploy uses --config, and all three are wrong — runDeploy reads/writes only --output-file. The failure mode is a silent wrong-cluster deploy with mismatched credentials, for the exact multi-cluster audience the README advertises.
- **Acceptance:**
  - `okdctl deploy --config staging.yaml` loads/creates staging.yaml and its sibling .env
  - deploy's Example, README's multi-cluster claim, and the root.go comment all become true statements
  - design decision recorded: either --output-file becomes an alias/override of --config for deploy, or it is repurposed strictly as "save here when diverging from --config"; its help text discloses that the file is read on re-runs
- **Depends on:** none

#### A19 — Drop stacked "failed to" prefixes from error-wrap chains

- **Status:** done — PR #932
- **Category:** refactor / failure legibility
- **State:** well-specified
- **Effort:** days
- **Impact:** small
- **Evidence:** `internal/distribution/okd/dns/dns.go:105-119`, `internal/cli/helpers.go:111`
- **Rationale:** ~130 wrap sites use the templated "failed to X: %w" prefix that compounds into chains burying the actionable cause. Both the Uber and Google Go style guides explicitly call out "failed to" context as the antipattern; the repo already contains the terse "verb noun: %w" style, so this normalizes toward its own better half and documented convention.
- **Acceptance:**
  - error wraps use the terse "verb noun: %w" form (~130 sites, concentrated in system/fs.go, okd/dns/, download/, okd/setup/)
  - a rendered failure chain reads "configure dns: render bootstrap dns config: open /etc/...: permission denied" rather than four consecutive "failed to" clauses
  - convention recorded in CLAUDE.md so new wraps don't regress
- **Depends on:** none

#### A20 — Replace deploy dry-run mirror test with a drift guard against real phase steps

- **Status:** done — PR #923
- **Category:** test honesty
- **State:** design needed
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/cli/deploy_test.go:11`, `internal/cli/deploy.go:192`
- **Rationale:** TestDeployDryRunSteps_IDs transcribes deploy.go's 31-entry literal back at itself — the one real regression on this surface (--dry-run output drifting from what deploy actually runs) is exactly what a literal-vs-literal mirror cannot catch.
- **Acceptance:**
  - the test (or a derivation in production code) verifies deployDryRunSteps() IDs match the step IDs actually registered by the setup/install/postinstall phases, so adding/reordering a phase step without updating dry-run output fails CI
  - if deriving from live StepDefs is impractical, each phase exports its canonical ordered StepID slice used by both its xSteps() method and deployDryRunSteps
- **Depends on:** none

#### A21 — Delete padded/tautological cli validator tests

- **Status:** done — PR #916
- **Category:** test honesty
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/cli/status_test.go:9`, `internal/cli/status_test.go:18`
- **Rationale:** TestValidateFormat_DescribeNode and TestValidateFormat_DescribeAddon run an identical table against the same single validateFormat function already covered by releases_test.go — copy-pasted scaffolding named after call sites as if they exercised different code. TestConditionStatusLiterals asserts a constant equals its own literal from the wrong package with no documented contract.
- **Acceptance:**
  - the two duplicate validateFormat tests are deleted
  - TestConditionStatusLiterals either moves next to the phase constants with a why-comment matching the noderole_test.go precedent (k8s wire values), or is deleted
- **Depends on:** none

#### A22 — Consolidate fake-binary PATH-stub and slog-capture test scaffolding

- **Status:** in review — PR #941
- **Category:** refactor (test infrastructure)
- **State:** well-specified
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/distribution/okd/phase/kubectl_test.go:21`, `internal/distribution/okd/destroy/steps_test.go:16`
- **Rationale:** The fake-binary-on-PATH pattern — the suite's best idea — is hand-rolled 17 times with identical mechanics and a vestigial Windows guard each time, drifting in small ways. Three test packages each carry their own captureHandler slog implementation.
- **Acceptance:**
  - a single internal test helper (e.g. internal/testutil) provides InstallFakeBin(t, name, script); the 17 per-package copies call it, keeping only package-specific script bodies local
  - the three duplicated captureHandler slog implementations are replaced by one shared capture handler
  - the copy-pasted `runtime.GOOS == "windows"` skips disappear with the consolidation
- **Depends on:** none

#### A23 — Drop redundant "successfully" suffixes from completion logs

- **Status:** done — PR #932
- **Category:** log hygiene
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/cli/summary.go:133`
- **Rationale:** ~11 completion logs carry a redundant "successfully" suffix ("terraform: proxmox infrastructure deployed successfully") plus one exclamation ("cluster deployed successfully!") — mild LLM filler; an Info-level completion log already implies success.
- **Acceptance:**
  - the ~11 "completed successfully"/"deployed successfully" messages drop the suffix; the exclamation goes
  - messages stay lowercase and structured per the existing log conventions
- **Depends on:** none

#### A24 — Re-scope internal/system before it becomes a util gravity well

- **Status:** done — PR #926
- **Category:** architecture
- **State:** design needed
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/system/system.go:1`
- **Rationale:** The package doc claims "host OS operations" but the package also hosts WaitFor (a generic polling loop that phase/kubectl.go reaches into), NewUUIDv4, and ZeroBytes. With A3 removing exec.go, decide what internal/system IS — and where the misfits live — before the next helper lands there by default.
- **Acceptance:**
  - package doc matches actual contents; WaitFor/NewUUIDv4/ZeroBytes either justified under the stated scope or moved to a fitting home
  - CLAUDE.md architecture notes name the package's boundary so future helpers don't default into it
- **Depends on:** A3

#### A25 — Put a test floor under infrastructure/proxmox before any go-proxmox change

- **Status:** done — PR #921
- **Category:** test honesty
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/infrastructure/proxmox/proxmox.go:32`, `internal/tui/wizard/steps/proxmox_discovery.go:1`
- **Rationale:** The package is doubly exposed: zero tests AND sole consumer of the bus-factor-1 v0.x go-proxmox dep that CLAUDE.md flags for a possible ~200 LOC REST rewrite. Any forced rewrite or version bump currently lands on a 557-LOC surface with no safety net.
- **Acceptance:**
  - httptest-backed tests cover the discovery path's request/response handling and error mapping (no live Proxmox needed)
  - coverage floor added for internal/infrastructure/proxmox in .github/coverage-floors.conf
- **Depends on:** none

#### A26 — Replace log-message-equality assertions with attr-based assertions

- **Status:** done — PR #916
- **Category:** test honesty
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/distribution/okd/destroy/steps_test.go:16`, `internal/cli/debug_bundle_test.go:1`
- **Rationale:** Two test files assert exact slog message strings, so rewording a log line breaks tests with no behavior change. The attr assertions (failed_steps/skipped_steps membership) are the honest part — keep those, drop message equality.
- **Acceptance:**
  - tests assert on structured attrs and level, not message-string equality (or match a stable substring where the message itself is the contract)
  - pairs naturally with the shared capture handler from A22
- **Depends on:** none

#### A27 — Hoist the "okd-install" workdir literal into a phase constant

- **Status:** in review — PR #940
- **Category:** refactor
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/distribution/okd/phase/paths.go:1`, `internal/cli/cleanup.go:1`
- **Rationale:** The literal "okd-install" workdir name is hardcoded at ~15 call sites (every phase's NewOptions, cli/cleanup.go, cli/status.go) even though phase owns path constants — a one-line constant removes a silent-rename hazard where one missed site orphans state.
- **Acceptance:**
  - a single exported constant in internal/distribution/okd/phase replaces all ~15 literals
  - grep for the raw string finds only the constant definition
- **Depends on:** none

### Tier B — holistic review 2026-07-11

Captured from `holistic-review` run on 2026-07-11 (HEAD `419c83e`). Items are
judgment-shaped (not audit atoms); each has a 1-3 sentence rationale inline.
Run focus: open-source launch readiness plus remaining structural AI residue.
Headline: leaf-level code and docs are production-grade; the gaps are one
level up — decorative enforcement (coverage gate), dark orchestration layer
(Execute paths untested), twice-encoded domain facts, and a launch story
(releases, templates, artifact policy) that fails on first pull. 31 agent
candidates landed as 28 items (B2 and B3 each merge overlapping findings;
one candidate dropped as a duplicate of open A5).

#### B1 — Fix the proxmox→phase layering inversion

- **Status:** done — PR #918
- **Category:** architecture
- **State:** design needed
- **Effort:** days
- **Impact:** large
- **Evidence:** `internal/infrastructure/proxmox/types.go:44`, `cmd/okdctl/main.go:44`
- **Rationale:** internal/distribution/okd/phase is a grab-bag of four unrelated things (kubectl helpers, path layout, Proxmox SSH access, domain enums), which forces the only inverted edge in the graph — infrastructure importing a distribution phase package — papered over with a type alias and constant re-exports.
- **Acceptance:**
  - internal/infrastructure/proxmox no longer imports internal/distribution/okd/phase; the `type VMRole = phase.NodeRole` alias and Role* constant re-exports are deleted
  - pure domain vocabulary (NodeRole, VMState, Condition*) lives in a leaf package that phase, proxmox, and cli/status all import downward
  - Proxmox host access over SSH (ssh.go SSHRun/SSHRunArgv, pvesh.go, iso_cleanup.go) lives beside the Proxmox provider; CLAUDE.md's SSH-policy pointer updated with the validateXxx + shellSingleQuote pattern intact
  - cmd/okdctl/main.go no longer imports distribution/okd/phase for PreflightBinDir
  - BasePhase.Oc* helpers remain in phase and keep their canonical status
- **Depends on:** none

#### B2 — Decompose internal/cli: command bodies and deploy engine behind testable seams

- **Status:** done — PR #929
- **Category:** architecture / cognitive-load
- **State:** design needed
- **Effort:** days
- **Impact:** large
- **Evidence:** `internal/cli/status.go:398`, `internal/cli/helpers.go:328`
- **Rationale:** internal/cli is the largest package at ~7k LOC across 42 files because whole features live inside it: status aggregation, doctor (536 LOC), debug-bundle (428 LOC), and — most consequentially — the deploy engine (executeFullDeployment, runGuardedPrepare, marker transitions), all untestable behind cobra glue with a zero coverage floor. Merges two independent findings (Structure + Test honesty).
- **Acceptance:**
  - cluster status aggregation (statusNode, condition folding, ClusterPhase derivation) lives with the okd domain types it produces; cli keeps flag parsing and rendering only
  - doctor's checks and debug-bundle collection move to owned packages testable without cobra plumbing
  - executeFullDeployment / runGuardedPrepare / marker-transition logic lives in a package drivable with a stubbed provisioner; crash-recovery transitions get direct tests (cancel during install leaves the install-phase marker; success clears it)
  - internal/cli shrinks toward flag-parsing + wiring; no non-root file over ~250 lines of non-cobra logic
  - elevation re-exec and deploystate marker may stay in cli if judged inherently command-shaped — record the judgment either way
- **Depends on:** B1

#### B3 — Unify haproxy/service teardown and the backup contract across phases

- **Status:** done — PR #905
- **Category:** correctness / refactor
- **State:** well-specified
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/distribution/okd/postinstall/haproxy.go:74`, `internal/distribution/okd/cleanup/services.go:64`
- **Rationale:** Three packages each invented their own naming for the same haproxy backup file and the globs don't intersect: setup's fixed haproxy.cfg.backup is invisible to postinstall's .backup.<ts> restore path and survives cleanup's .backup.* glob — semantic duplication matured into a behavioral bug (root-owned residue after uninstall). The same packages hand-roll identical systemd stop/disable and VIP-release blocks. Merges three overlapping findings (Structure + Authenticity ×2).
- **Acceptance:**
  - one documented backup contract for /etc/haproxy/haproxy.cfg: fixed pristine snapshot vs timestamped rolling backups either merged or their split roles stated at the constant and honored by all consumers; the backup suffix is a single exported constant written and globbed via the same symbol
  - postinstall restore (update_ingress.go) either falls back to the pristine setup snapshot or documents why it must not
  - cleanup removes (or intentionally preserves, with a comment) the fixed .backup file
  - the IsServiceActive→Stop / IsServiceEnabled→Disable idempotent guard exists once in phase/ (per CLAUDE.md's cross-phase-helper rule); cleanup and postinstall both call it
  - the verbatim-duplicated VIP release block (GetDefaultInterface → RemoveSecondaryIP, guarded by vip != "") collapses to one helper
  - divergent behavior that remains (postinstall backs up, cleanup purges) is intentional and stated in the helper's doc comment
- **Depends on:** none

#### B4 — Split netutil into pure IP math and host-network mutation

- **Status:** done — PR #904
- **Category:** refactor
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/netutil/iface.go:35`, `internal/config/validators.go:14`
- **Rationale:** A package named netutil that both validates CIDRs and reconfigures NetworkManager misleads readers about blast radius: config validation — the most innocent layer in the repo — transitively links code that runs `nmcli connection modify` as root.
- **Acceptance:**
  - CIDR/IP arithmetic and privileged host mutation via ip/nmcli shellouts no longer share a package
  - internal/config imports only the pure half; the mutation half sits beside platform/system in the host-management layer and keeps its validateConnectionName guard
  - no import cycle is introduced (mutation half may keep importing system)
- **Depends on:** none

#### B5 — Cut the download→tui edge by injecting progress enablement

- **Status:** done — PR #901
- **Category:** refactor
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/download/progress.go:28`
- **Rationale:** The only utility→presentation edge in the graph: a byte-transfer package consults the TUI layer's global at runtime. Inverting it makes download a clean leaf and removes the objection to reusing its retry policy from non-TUI contexts.
- **Acceptance:**
  - internal/download no longer imports internal/tui; the progress writer takes an enabled flag or writer at construction and callers pass tui.ProgressBarsEnabled()
  - TTY-gating behavior is unchanged for deploy; download's import set reduces to errtypes/httputil/logutil
- **Depends on:** none

#### B6 — Wire or delete the dead phase Options knobs and terraform.WithVarFile

- **Status:** done — PR #899
- **Category:** cleanup
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/distribution/okd/setup/phase.go:49`, `internal/distribution/okd/install/phase.go:43`
- **Rationale:** Fresh instances of the exact fabricated-knob shape the 2026-06-10 run caught (A9/A15) but missed here: configuration surface that promises behavior the code never delivers.
- **Acceptance:**
  - setup.Options.AutoDownloadISO either gates the documented "skip the ISO-present prompt" behavior or is deleted along with its doc (documented, set once to constant true, never read — a knob whose doc lies is outside the scaffolding carve-out)
  - setup.Options.Verbose (never assigned, never read) is deleted
  - install.Options.StreamBootstrapLogs (constant true, gates nothing) is wired or deleted
  - terraform.WithVarFile (zero call sites) gets an explicit keep-as-API-shape decision recorded or is removed
- **Depends on:** none

#### B7 — Reconcile the wizard KeyMap with the keys actually handled

- **Status:** done — PR #898
- **Category:** cli-ux / cleanup
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/tui/wizard/model.go:121`, `internal/tui/wizard/model.go:108`
- **Rationale:** Half of the registered key bindings are dead config duplicated inline across steps, and one ('?' help) advertises a feature that does not exist — a user-visible authenticity gap in the first screen an adopter touches.
- **Acceptance:**
  - the '?' Help binding either gets a real help overlay or is removed from the KeyMap and its advertised help text
  - the other never-matched bindings (Next, Up, Down, Select) are either routed through model.Update so steps reference KeyMap instead of hand-rolling inline key.NewBinding, or deleted
  - every binding remaining in defaultKeyMap is matched somewhere; no help text advertises a nonexistent feature
- **Depends on:** none

#### B8 — Right-size speculative generality in tui/wizard/components

- **Status:** done — PR #913
- **Category:** refactor
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/tui/wizard/components/key_value_field.go:25`, `internal/tui/wizard/components/selector.go:18`
- **Rationale:** components/ is where the repo's remaining AI-slop concentrates: widgets engineered for a marketplace of steps that never materialized, per-object state that nothing reads, and fallback branches that cannot fire. The declarative wizard core around it is excellent, which makes this pocket stand out more.
- **Acceptance:**
  - for each speculative surface — KeyValueField's 410-line editable-table mode set serving one field, Selector's ~600 lines with a 6-variant OptionStyle serving one step, the write-only defaultValue trio, InputGroup's uncalled AddField/IsValid/bordered View, unread ColorBorder, Selector's unreachable all-disabled fallback — record an explicit keep-as-future-API decision (MEMORY.md carve-out) or delete
  - write-only state (defaultValue trio, ColorBorder) is deleted outright: stored-never-read fields are not API shape
  - components/ line count drops or every surviving mode has a named prospective consumer in a comment
- **Depends on:** none

#### B9 — Share single-select navigation between welcome and review steps

- **Status:** done — PR #917
- **Category:** refactor
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/tui/wizard/steps/review.go:124`, `internal/tui/wizard/steps/welcome.go:70`
- **Rationale:** The two hand-rolled wizard steps duplicate the exact navigation plumbing the components package exists to own — the same parallel-shape duplication pattern as the addons, at smaller scale.
- **Acceptance:**
  - the identical enter/up/down + StepCompleteMsg loop hand-rolled in WelcomeStep and ReviewStep lives in one shared single-select step type or in CompactSelector itself
  - both steps consume the shared component; no per-step key loop remains
- **Depends on:** B8

#### B10 — Decouple cluster state and Terraform sources from the okdctl repo checkout

- **Status:** done — PR #933
- **Category:** architecture
- **State:** design needed
- **Effort:** weeks
- **Impact:** large
- **Evidence:** `internal/cli/helpers.go:119-147`, `internal/distribution/okd/okd.go:190-199`
- **Rationale:** Every piece of cluster state — tfstate, okd-install/ artifacts, auth material, the deploy marker — lives in cwd, and the Terraform HCL must pre-exist at `<cwd>/infrastructure/terraform/environments/<env>`, meaning the shipped binary is unusable outside a source checkout and cfg.Cluster.Name namespaces nothing on disk. The single largest gap between what the packaging advertises and what the domain model can do.
- **Acceptance:**
  - a packaged binary (apt/rpm per README) can deploy a cluster without a git checkout — the Terraform module/environment is go:embed-ed and materialized into the workspace, or an explicit `okdctl init` scaffolds it
  - the workspace contract is explicit and cluster-keyed: okd-install/, terraform state, and the deploy marker live under a directory derived from (or validated against) cfg.Cluster.Name rather than bare cwd, or the docs state plainly that one working directory == one cluster and the checkout requirement is intentional
  - resolveProjectRootOrDie's error message ("run okdctl deploy to initialise") stops being circular — deploy in an empty directory either works or names the real prerequisite
- **Depends on:** none

#### B11 — Adopt one vocabulary for the deploy lifecycle stages

- **Status:** done — PR #931
- **Category:** domain-model accuracy
- **State:** well-specified
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/cli/deploystate.go:18-22`, `internal/distribution/okd/okd.go:135-225`
- **Rationale:** One lifecycle, two parallel vocabularies: Provisioner.Prepare wraps the setup package and Configure wraps postinstall; the crash marker says prepare/configure while logs say phase=setup and destroy hints mix both. "Cancelled during prepare" leads to a package that doesn't exist.
- **Acceptance:**
  - the same three stage names appear in the Provisioner API, the deploy-state marker values, user-facing interrupt/destroy hints, step logs, and docs/architecture/phases.md
  - marker schema compatibility is handled (v1 values mapped or schema bumped per the existing deployStateSchemaV1 mechanism)
  - docs/architecture/phases.md and cobra help need no translation table between Provisioner.Prepare/Configure and the setup/postinstall packages they call
- **Depends on:** none

#### B12 — Unify control-plane naming and colocate per-node-group config facts

- **Status:** done — PR #919
- **Category:** config coherence
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/config/cluster.go:40-53`, `internal/config/cluster.go:126-131`
- **Rationale:** The control-plane node group is spelled three ways in three schema sections (topology.control_plane vs provider.proxmox.master_nodes vs disks.master_data_size_gb), and its resource facts are scattered with inconsistent unit conventions — the same domain object never appears whole.
- **Acceptance:**
  - one name at the YAML surface (control_plane; RoleMaster may stay internal where OKD's own MCP/label vocabulary demands it)
  - per-group sizing facts live together, or the split is documented as deliberate with consistent unit-naming
  - VM placement (master_nodes/worker_nodes → Proxmox target nodes) is validated for count coherence against topology counts or documented for its padding/default semantics
- **Depends on:** none

#### B13 — Single-source the static IP plan and de-overload static_ip.start

- **Status:** done — PR #912
- **Category:** config coherence
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/distribution/okd/setup/nodes.go:19-32`, `internal/config/defaults.go:26-33`
- **Rationale:** The machine subnet is encoded twice (CIDR and dotted netmask) with different consumers, the sequential-allocation base doubles as three other domain facts (bootstrap IP, VIP seed, install-monitor target), and the shipped defaults collide with themselves — a first-run user who accepts defaults boots a bootstrap VM that ARP-fights the Proxmox host. Complements but does not overlap A14.
- **Acceptance:**
  - networking.static_ip.netmask is derived from machine_cidr at load time or a validator rejects disagreement
  - the invariant that static_ip.start IS the bootstrap node's IP (and the VIP-derivation base) is stated in the schema doc or replaced by an explicit bootstrap_ip field
  - DefaultConfig's IP plan is self-consistent: provider.proxmox.host no longer shares 192.168.1.100 with static_ip.start
- **Depends on:** none

#### B14 — Resolve the tool-version vs OKD-release-version confusion

- **Status:** done — PR #906
- **Category:** domain-model accuracy
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/cli/helpers.go:204`, `internal/cli/cleanup.go:135`
- **Rationale:** Two distinct domain facts — which okdctl produced an artifact, and which OKD release is being deployed — flow through a single string whose value differs by CLI verb and whose three doc sites each claim a different meaning.
- **Acceptance:**
  - one field, one fact: BasePhase.Version carries either the okdctl build version or the OKD release version, never both depending on entry path
  - the three contradictory docs converge (okd.New, phase.WithVersion, docs/architecture/phases.md)
  - if nothing consumes the field for provenance, it is removed or given a real consumer rather than kept as a lie
- **Depends on:** none

#### B15 — Stop the resume path from wiping live-cluster identity material

- **Status:** done — PR #914
- **Category:** state & lifecycle
- **State:** design needed
- **Effort:** days
- **Impact:** large
- **Evidence:** `internal/distribution/okd/okd.go:127-154`, `internal/cli/helpers.go:213-233`
- **Rationale:** PrepareOpts.ResumeInProgress bypasses the live-cluster guard and then Prepare unconditionally runs the WorkOnly wipe, so the documented "re-run deploy to resume" flow is only truthful for prepare-stage interruptions; after bootstrap the same flow destroys the only copy of the cluster's auth bundle and regenerates a CA the running VMs never saw. Needs no new persistence — only consulting the phase granularity the marker already records before choosing to wipe.
- **Acceptance:**
  - re-running deploy after an interruption during install or configure does not silently wipe cluster-config/auth (kubeconfig, kubeadmin-password) or regenerate ignition/CA material live VMs booted with — the marker's Phase field gates whether the pre-setup wipe is safe
  - an interruption during configure resumes to a working cluster or fails fast with a precise instruction
  - the guardLiveCluster doc's admission that the marker "cannot vouch for a setup-phase interruption" is resolved by design rather than bypass
- **Depends on:** none

#### B16 — Restore community templates the docs claim exist, and add CONTRIBUTING.md

- **Status:** done — PR #902
- **Category:** oss-readiness / docs
- **State:** design needed
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `README.md:288`, `CHANGELOG.md:22`
- **Rationale:** An outside contributor can learn the architecture from docs/ but cannot pass the CI gates without reading an AI instruction file, and a bug reporter is directed to issue forms that were deleted (added in ad06796, removed in e57de57 with no rationale). The CHANGELOG "Added: community templates" entry describing removed files is exactly the docs-vs-reality gap an OSS launch cannot afford.
- **Acceptance:**
  - issue forms, PR template, CODEOWNERS, FUNDING are restored or the dangling README/CHANGELOG claims are scrubbed
  - bug-report form asks for the triage inputs the tool already produces: okdctl version, doctor output, debug-bundle attachment, Proxmox VE version
  - CONTRIBUTING.md distills the human-relevant contributor gates that today live only in CLAUDE.md: lefthook install, coverage floors, lint thresholds, make check/docs targets, conventional-commit format, flag-shorthand policy
- **Depends on:** none

#### B17 — Define a public-tree policy for AI-workflow artifacts

- **Status:** done — decision recorded 2026-07-11: .claude/ and docs/superpowers/ untracked (local-only); roadmap.md + completed-archive stay public as working-in-the-open; no history rewrite
- **Category:** oss-readiness / governance
- **State:** design needed
- **Effort:** days
- **Impact:** large
- **Evidence:** `roadmap.md:286`, `.claude/audits/audit-security.jsonl` (git-tracked)
- **Rationale:** The tree ships a machine-generated map of known-unfixed weaknesses — including security ones with locations and exploit-shaped descriptions — which contradicts the project's own coordinated-disclosure policy and reads as internal AI-orchestration residue to any outside observer. The single largest open-source publishing decision, currently being made by default.
- **Acceptance:**
  - a deliberate decision is recorded per artifact class: roadmap.md, .claude/audits/*.{jsonl,md}, docs/superpowers/{plans,specs}, docs/roadmap/completed-archive.md, .claude/scheduled_tasks.lock — ship, relocate, or rewrite as a public-facing ROADMAP
  - open security-weakness writeups with fix plans do not ship in the public default branch while unfixed
  - docs/superpowers/ is removed from the shipped docs set
  - .claude/scheduled_tasks.lock is untracked and gitignored
- **Depends on:** none

#### B18 — Reconcile the "phones home to nothing" claim with the default-on update check

- **Status:** done — PR #909
- **Category:** docs-reality / trust
- **State:** design needed
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `README.md:20`, `internal/version/updatecheck.go:22`
- **Rationale:** okdctl markets privacy ("phones home to nothing") to a homelab audience that cares about exactly that, while BackgroundCheck hits api.github.com on every eligible invocation by default. The kind of contradiction that becomes a hostile HN comment; either behavior or claim must move.
- **Acceptance:**
  - either the update check becomes opt-in, or README is corrected to disclose the api.github.com call, its cache, and the OKDCTL_NO_UPDATE_CHECK=1 opt-out
  - the opt-out env var is documented in user-facing docs
  - packaging notes and install.sh output are consistent with whichever posture is chosen
- **Depends on:** none

#### B19 — Cut v0.1.0 and exercise the entire release story end-to-end

- **Status:** prep done — PR #925; v0.1.0 tag push is the maintainer action
- **Category:** release engineering
- **State:** well-specified
- **Effort:** days
- **Impact:** large
- **Evidence:** `CHANGELOG.md:42`, `README.md:189`
- **Rationale:** The README's strongest open-source signal — signed, attested, reproducible releases — is entirely untested: no tag has ever been pushed, so the verify instructions, the update check's releases/latest query, and the curl|bash installer all point at nothing.
- **Acceptance:**
  - a real v0.1.0 tag exists and the release workflow produces the artifact set README promises (tarballs, SHA256SUMS + sigstore sig/pem, CycloneDX SBOMs, intoto provenance)
  - the cosign verify-blob and gh attestation verify commands in README are executed verbatim against the real artifacts and pass; install.sh is run against the live release
  - CHANGELOG Unreleased is backfilled with user-facing changes and moved under [0.1.0]; an ongoing changelog rule is adopted
- **Depends on:** B17

#### B20 — Give non-cancel deploy failures the same summary and next-steps as Ctrl-C

- **Status:** done — PR #934
- **Category:** operator ux / failure legibility
- **State:** design needed
- **Effort:** days
- **Impact:** large
- **Evidence:** `internal/cli/helpers.go:391`, `internal/distribution/okd/install/monitor.go:60`
- **Rationale:** The recovery machinery (resume marker, debug-bundle, graceful status) all exists, but a deploy that dies at minute 40 cross-references none of it and actively steers the operator toward destroy. The Ctrl-C path proves the codebase can narrate partial progress well — failures deserve at least parity.
- **Acceptance:**
  - a step failure ends with a failure summary equivalent to the existing InterruptSummary box: failed phase+step, elapsed time, run_id — today that box renders only on errors.Is(err, context.Canceled)
  - the failure epilogue leads with the resume path ("re-run okdctl deploy to resume from <step>") instead of pointing solely at destructive destroy
  - monitor timeout errors name where to look next: the openshift-install log, a representative oc command, and okdctl debug-bundle
- **Depends on:** none

#### B21 — Write a persistent per-run deploy log by default

- **Status:** done — PR #935
- **Category:** operator ux / diagnosability
- **State:** design needed
- **Effort:** days
- **Impact:** large
- **Evidence:** `internal/cli/logging.go:45`, `internal/cli/debug_bundle.go:200`
- **Rationale:** A 40-minute deploy failure is currently undiagnosable once terminal scrollback is gone unless the operator predicted the failure and passed --log-file up front — which nothing tells them to do. Every comparable installer persists its log unconditionally; the support-bundle story is hollow without it.
- **Acceptance:**
  - deploy/destroy/cleanup always tee their stream to a log file in the work directory (per run_id or rotated), analogous to openshift-install's .openshift_install.log
  - debug-bundle picks up the default log location without configuration, and the failure epilogue prints the log path
  - redaction guarantees are re-verified for the file sink and the log location respects the sudo-re-exec hardening constraints already tracked for --log-file
- **Depends on:** B20

#### B22 — Fix the coverage gate: wrong metric, delete-tests bypass, decorative floors

- **Status:** done — PR #900
- **Category:** ci / test-infrastructure
- **State:** well-specified
- **Effort:** hours
- **Impact:** large
- **Evidence:** `.github/scripts/coverage-check.sh:44-56`, `.github/coverage-floors.conf:8`
- **Rationale:** The coverage gate is the repo's only anti-regression contract and it currently measures the wrong number (unweighted mean of per-function percentages, not statement coverage), can be bypassed by deleting tests (package vanishes from profile, floor silently unchecked), and has floors 30–45 points below measured coverage. A green gate that is decorative is worse than none.
- **Acceptance:**
  - gate compares per-package statement coverage, not unweighted per-function means
  - gate iterates over the floors file's keys and fails if a floored package is absent from the profile
  - floors re-baselined to measured-minus-5 (destroy 5→~45, phase 35→~75, download 25→~70, cleanup 35→~70); a non-zero total floor is set
  - well-covered but unfloored packages (errtypes, sshpin, cluster, logutil, runlock, addon) get floors
- **Depends on:** none

#### B23 — Test phase orchestration: Execute paths and skip-flag wiring, destructive steps enabled

- **Status:** done — PR #924
- **Category:** tests
- **State:** well-specified
- **Effort:** days
- **Impact:** large
- **Evidence:** `internal/distribution/okd/destroy/steps_test.go:60-71`, `internal/distribution/okd/destroy/phase.go:68`
- **Rationale:** Every destructive decision okdctl makes — which steps a destroy runs and in what order — is encoded in xSteps()/Execute functions that have never executed under test; the one "success path" test disables every destructive step first. The single most consequential honesty gap in the suite.
- **Acceptance:**
  - destroy's success-path test no longer runs with SkipTerraform+SkipCleanup+SkipFirewall+KeepISOs all true; at least one test drives destroy Execute through the terraform-destroy step against a fake terraform binary and asserts argv/ordering
  - each phase has a table test over Options permutations asserting the produced step-ID list; Execute/New/NewOptions/xSteps stop being 0.0% in all five packages
  - cli runDestroy/runCleanup/runDeploy reachable under test via a stubbed provisioner seam, covering the confirm-prompt-to-Execute handoff
  - complementary to A20 — this covers real Execute paths, not dry-run output
- **Depends on:** none

#### B24 — Put tests under update_ingress rollback and wait state machine

- **Status:** done — PR #920
- **Category:** tests / risk-reduction
- **State:** design needed
- **Effort:** days
- **Impact:** large
- **Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:69`, `internal/distribution/okd/postinstall/update_ingress.go:597`
- **Rationale:** update_ingress mutates a live cluster's ingress with a hand-rolled rollback path, and everything except two pure builders is 0% covered — the code most likely to brick a user's cluster on partial failure is exactly the code with no tests. Simultaneously the repo's biggest file and its riskiest untested surface.
- **Acceptance:**
  - attemptRollback, restoreHAProxyBackup, waitForRouterGone, waitForServiceLB, handleHostNetworkConversion exercised via a stubbed BasePhase.Oc* seam (extend phase/kubectl.go per CLAUDE.md rather than a local wrapper)
  - a failure injected mid-conversion asserts the rollback actually restores the prior ingress strategy and HAProxy backup
  - file stops being the repo's largest with only its 2 pure JSON builders covered; split acceptable if it falls out naturally
- **Depends on:** none

#### B25 — Shrink the //go:build linux surface in internal/cli to the syscall layer

- **Status:** done — PR #911
- **Category:** developer-experience / architecture
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/cli/doctor.go:1`, `internal/cli/debug_bundle_doctor_stub.go:1`
- **Rationale:** The sole maintainer develops on darwin, where the biggest file in the biggest package neither compiles, lints, nor tests locally — a structural blind spot that already causes CI round-trips. Not a Windows-compat item; it makes the Linux-only product's largest subsystem visible to its own dev loop.
- **Acceptance:**
  - doctor orchestration, check-result modeling, and rendering compile on darwin; only the genuinely Linux probes (proc/syscall/systemd) stay behind //go:build linux with thin stubs
  - darwin make lint and go build ./... cover the doctor/debug-bundle logic
  - the GOOS=linux-lint round-trip pain shrinks to the probe files only
- **Depends on:** none

#### B26 — Delete the return-step,step two-value ceremony in wizard step constructors

- **Status:** done — PR #903
- **Category:** refactor / ai-slop
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/tui/wizard/steps/basics.go:69-71`, `internal/tui/wizard/steps/addons.go:253-258`
- **Rationale:** Six constructors declare `(step, state *wizard.DataDrivenStep)` and `return step, step` — the same pointer twice under two names, implying a step/state separation that does not exist. AI-generated ceremony, not the MEMORY.md future-API scaffolding carve-out.
- **Acceptance:**
  - all six New*Step constructors return a single *wizard.DataDrivenStep; callers use the one value
  - no behavior change; wizard flow unaffected
- **Depends on:** none

#### B27 — Enable golangci-lint on _test.go files

- **Status:** done — PR #915
- **Category:** ci / test-infrastructure
- **State:** well-specified
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `.golangci.yml:102`, `.golangci.yml:92-94`
- **Rationale:** The config contains test-file lint exclusions AND run.tests:false — the exclusions are dead config, a tell that the blanket disable was expedient rather than chosen. With tests now load-bearing, leaving ~13k LOC unlinted invites exactly the rot the gate exists to prevent.
- **Acceptance:**
  - run.tests: true; test code gets vet/staticcheck/errcheck coverage
  - the dead test-path exclusions for dupl/funlen/goconst become live; further test-scoped exclusions added as needed instead of blanket-disabling
  - CI lint-go stays green after a one-time cleanup pass
- **Depends on:** none

#### B28 — Build the launch surface: repo metadata, wizard demo capture, launch checklist

- **Status:** done — PRs #927 #930 #936; repo metadata set
- **Category:** oss-readiness / strategy
- **State:** sketch
- **Effort:** days
- **Impact:** medium
- **Evidence:** `README.md:9`
- **Rationale:** The wizard is the product's centerpiece and the README contains zero screenshots or captures; the GitHub repo has no description or topics, so it is undiscoverable even by someone searching for exactly this tool. Everything else in this review makes the code worthy of attention — this item turns attention into users.
- **Acceptance:**
  - GitHub repo has a description and topics (proxmox, okd, openshift, kubernetes, homelab, terraform, golang, tui)
  - README opens with a demo capture of the wizard (VHS tape or asciinema-to-GIF) plus a screenshot of deploy progress and `okdctl status` output; the tape file is committed so the demo regenerates when the wizard changes
  - a short launch checklist exists sequenced after B16/B17/B19: release shipped and verified, templates restored, demo live, announcement targets (r/homelab, r/Proxmox, OKD community forum)
- **Depends on:** B16, B19

### Tier C — holistic review 2026-07-13

Captured from a holistic-review run on 2026-07-13 (HEAD `509ff51`, branch
`feat/node-lifecycle`). Items are judgment-shaped (not audit atoms); each has
a 1-3 sentence rationale inline. Two waves: C1-C18 from a five-theme review
fleet (node-lifecycle surface, day-2 gaps, deploy-output UX, trust), each
item surviving an adversarial verification pass that checked evidence at the
cited lines, searched for prior solutions, and grepped this file for
duplicates; C19-C34 from a follow-up wave (web research on the OKD/Proxmox
landscape, a build-vs-buy review, a deletion-biased simplification sweep).
Headlines: the node-op layer's architecture is sound (plan gate, etcd gates,
inert dry-runs all verified) but its crash-resume story is write-only — the
op marker is recorded before every mutating step and read by nothing; and
OKD's SCOS-only reality since 4.19 has drifted out from under the FCOS-named
ISO globs while the stream-pin lookup discards the major version with OKD
5.0 already in engineering candidates.

#### C1 — Make interrupted node ops resumable

- **Status:** done — PR #950
- **Category:** trust / state-recovery
- **State:** design needed
- **Effort:** days
- **Impact:** large
- **Evidence:** `internal/node/opstate.go:87`, `internal/node/guards.go:59-61`
- **Rationale:** The opstate package doc promises interrupted ops resume with context, but `readOpState` has zero production callers: config is persisted before apply, and the empty-plan gate hard-errors on any already-applied re-run — so every crash window mid-remove/resize strands the operator with a cordoned node, a live VM, and a config that says it's gone. The tool recorded exactly what it needs and then ignores it.
- **Acceptance:**
  - Every node op reads the op marker at start and either resumes from the recorded step or prints an actionable "interrupted op X at step Y on node Z; node left cordoned" diagnostic stating what re-running will do
  - SIGKILL injected between persistTopology and tf apply, between tf apply and DeleteNode, and mid master-roll resize: re-running the same command completes the op instead of failing validateWorkerRemovable or the plan gate — one test per crash window
  - AssertOnlyChange distinguishes "plan empty because already at target" (skip apply, uncordon, continue; dry-run reports no changes needed) from "variable never reached the module" (still fatal for remove's delete gate)
  - A node command run while another op's marker is present requires explicit acknowledgment; if the journal is instead judged not worth reading, the marker writes and the docs' resume claims are removed together
- **Depends on:** none

#### C2 — Preflight compact guards and make its dry-run real

- **Status:** in review — branch feat/tier-c-batch
- **Category:** trust / destructive-op safety
- **State:** well-specified
- **Effort:** days
- **Impact:** large
- **Evidence:** `internal/node/compact.go:45-50`, `internal/node/compact.go:52-59`
- **Rationale:** Compact is the most destructive verb on the branch — it serially destroys every worker — yet its dry-run holds a strictly weaker contract than the sub-ops it composes: a green three-count summary, then the real run mutates the control plane and ingress before discovering worker1's OSD guard blocks, leaving a half-compacted cluster the preview never predicted.
- **Acceptance:**
  - `cluster compact --dry-run` runs the read-only storage/ingress guards and memory-budget projection for every worker plus the per-worker delete plan gates (progressively decremented worker_count -var overrides, the mechanism remove's dry-run already uses), and prints the ordered action list with per-node verdicts
  - A real compact evaluates all worker guards (notably rook-ceph OSD presence) before SetMastersSchedulable or the ingress apply mutate anything
  - A guard refusal after N workers were removed tells the operator exactly what hybrid state they are in and how to proceed
  - Dry-run stays zero-mutation and zero-disk-write, covered in dryrun_test.go alongside the remove/resize inertness tests
- **Depends on:** none

#### C3 — Complete okdctl node add

- **Status:** done — PR #950
- **Category:** feature-gap / day-2
- **State:** well-specified
- **Effort:** weeks
- **Impact:** large
- **Evidence:** `internal/cli/node.go:84-86`, `internal/distribution/okd/setup/iso.go:38`
- **Rationale:** This branch ships remove/resize/compact while add remains a UsageError stub, so the lifecycle story is asymmetric: operators can shrink but must redeploy to grow. Scale-out is the most-asked day-2 operation in every peer tool, and every ingredient — the BuildCustomISOs/buildNodeISO per-node ISO pipeline, the ignition HTTPS server, the CSR approval loop, the plan gate, the memory-budget probe — is already in the tree.
- **Acceptance:**
  - `okdctl node add --role worker` builds the per-node CoreOS ISO via the existing BuildCustomISOs path, uploads it via the existing hostssh path, and revives the ignition HTTPS server only for the join window (then tears it down, matching the pull-secret exposure mitigation)
  - Bumps worker_count with the plan gate asserting exactly one create; waits for join via the CSR approval loop; updates okdctl.yaml + tfvars
  - `--dry-run` previews; the Proxmox memory-budget probe guards oversubscription like resize does
  - A cluster deployed with N workers reaches N+1 Ready workers with one command
- **Depends on:** none

#### C4 — Add cluster stop/start with CSR recovery

- **Status:** done — PR #950
- **Category:** feature-gap / day-2
- **State:** design needed
- **Effort:** days
- **Impact:** large
- **Evidence:** `internal/cluster/k8s_csrs.go:59`, `internal/cli/node.go:190`
- **Rationale:** Powering the cluster down is the single most common homelab day-2 operation, and the restart is where OKD's cert-rotation cliff bites with the notorious pending-CSR dance. okdctl uniquely owns both sides — Proxmox VM power and oc/CSR access — which today the operator interleaves by hand between the Proxmox UI and the graceful-shutdown runbook.
- **Acceptance:**
  - `okdctl cluster stop` cordons nodes, gracefully shuts down guests (workers before masters), confirms VMs stopped via the Proxmox API, and prints the kube-apiserver-to-kubelet-signer expiry with a warning if planned downtime crosses it
  - `okdctl cluster start` powers VMs masters-first, waits for the API, then runs the CSR approval loop until all nodes are Ready and uncordoned
  - Both verbs support --yes/--confirm-cluster and --dry-run consistent with node remove/resize
  - A cluster restarted after >30 days offline comes back Ready without hand-approving CSRs
- **Depends on:** none

#### C5 — Coordinate the spinner with the stderr logger

- **Status:** in review — branch feat/tier-c-batch
- **Category:** tui / deploy-output
- **State:** well-specified
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/tui/spinner.go:46-48`, `internal/distribution/okd/install/monitor.go:91`
- **Rationale:** Every long phase today produces garbled lines like `| monitoring cluster operators (2m30s)[INFO] csr: approved 3` because two writers share stderr with zero coordination. It is the most visible rough edge in real runs and the foundation every other deploy-output improvement builds on.
- **Acceptance:**
  - tui keeps an atomic registry of the active spinner; the stderr handler erases the spinner line (`\r\x1b[2K`) under a shared mutex before writing a record, and the spinner repaints on its next tick — log lines always start at column 0 on their own row
  - Spinner clearing switches from len(desc)-based space math to `\r\x1b[2K` (correct for any width and non-ASCII desc)
  - Frames upgrade from ASCII `| / - \` to braille dots styled with existing theme styles; elapsed-time suffix kept
  - Non-TTY path unchanged (StartSpinner still no-ops when ProgressBarsEnabled is false)
- **Depends on:** none

#### C6 — Route the openshift-install firehose to the log file, curate the TTY

- **Status:** in review — branch feat/tier-c-batch
- **Category:** tui / deploy-output
- **State:** design needed
- **Effort:** days
- **Impact:** large
- **Evidence:** `internal/distribution/okd/install/monitor.go:19`, `internal/cli/logging.go:113-114`
- **Rationale:** The longest phase of the deploy is currently unreadable: a `--log-level=debug` subprocess firehose buries every okdctl-styled line while the spinner stomps on partial lines. The okdctl.log sink already exists but its MultiWriter tees only tui-logger output — subprocess streams bypass it because the executor defaults to bare os.Stdout/os.Stderr and no production call site uses WithStdout/WithStderr; routing the firehose there is the missing plumbing.
- **Acceptance:**
  - By default the openshift-install stdout/stderr stream routes only to the persistent log file (plumb the sink writer into the phase executor via the existing WithStdout/WithStderr options); --verbose restores live streaming to the TTY
  - The TTY shows one owned status line — spinner, elapsed, clusteroperator available count, CSRs approved — refreshed via the existing cluster.Client polling the CSR ticker loop already runs
  - Milestone lines parsed from the stream (bootstrap complete, install complete, degraded-operator warnings) are promoted to normal tui.Info lines
  - On failure/timeout the error still points at .openshift_install.log (timeoutNextSteps already does) and okdctl.log so nothing diagnostic is lost
- **Depends on:** C5

#### C7 — Render a live deploy step checklist via MetricsRecorder

- **Status:** in review — branch feat/tier-c-batch
- **Category:** tui / deploy-output
- **State:** well-specified
- **Effort:** days
- **Impact:** large
- **Evidence:** `internal/distribution/orchestrator.go:168`, `internal/distribution/okd/okd.go:259`
- **Rationale:** During the 40-minute deploy the operator stares at unnumbered slog lines with no idea how far along they are. Both seams (MetricsRecorder, DeploySteps) already exist, so this is pure rendering work with the single biggest demo payoff.
- **Acceptance:**
  - A tui.StepProgress type implements distribution.MetricsRecorder, wired in internal/deploy.Execute alongside the existing deploymetrics recorder (deploy.go:200) behind a fan-out MultiRecorder, since Orchestrator/Provisioner hold a single recorder slot
  - DeployStep/DeploySteps gain a Phase field so the checklist can render "[ 4/17] create vms · install"; StepStarted shows the dim line, StepFinished rewrites it in place with duration and success/failure/skip styling
  - Resume runs subset phases and skipped/already-done steps emit StepFinished without StepStarted — totals are seeded from the actually-run phases and the renderer tolerates finish-without-start
  - When the checklist is active, the orchestrator's own step Info lines demote to Debug so they land in okdctl.log but not the TTY; non-TTY / json behavior unchanged; Ctrl-C and failure paths still render InterruptSummary/FailureSummary with completed lines left in scrollback
- **Depends on:** C5, C6

#### C8 — Show progress during long node-op waits

- **Status:** in review — branch feat/tier-c-batch
- **Category:** operator-ux / node-ops
- **State:** well-specified
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/cli/node.go:171`, `internal/node/resize.go:193-198`
- **Rationale:** Prior tiers cured the 40-minute-deploy blindness; the new surface reintroduces it — a master resize is a multi-hour near-silent wall exactly when the operator is most anxious, with their control plane cordoned. Reporter is wired to tui.StartSpinner at the CLI and never invoked in internal/node; drain and the targeted apply run fully captured, and the 15m/10m wait gates print one Info line then demote every poll tick to Debug.
- **Acceptance:**
  - Each long-running step (cordon/drain, targeted plan+apply, node-ready wait, etcd gate) shows a spinner or step line while in flight, matching deploy's presentation
  - A 3-master resize (potentially 60+ minutes) never goes more than the poll interval without visible evidence of life at default verbosity
  - Reporter invocation is covered by a test using the recording fake; dry-run inertness tests still pass
- **Depends on:** none

#### C9 — Make destructive node/cluster confirmations informed and destroy-grade

- **Status:** in review — branch feat/tier-c-batch
- **Category:** trust / operator-ux
- **State:** well-specified
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/cli/node.go:264`, `internal/cli/destroy.go:230-240`
- **Rationale:** Compact irreversibly destroys N worker VMs and their data disks yet gates on a single y/N issued before okdctl has computed anything, while destroy requires exact-name typing after a preview. The rendering primitives all exist; rebuilding the consent flow is also the natural moment to stop routing it through shared package-level flag globals.
- **Acceptance:**
  - Before the prompt, node remove/resize and cluster compact print a render.Builder box with node, role, tf address, drain timeout, guard verdicts, and an amber irreversible line naming the VM + data disk — shown after guards and the plan gate so consent is informed
  - Interactive cluster compact and node remove require typing the cluster name (reusing destroy's two-stage gate), not just y/N
  - --dry-run and completion render deploy-family boxes (DryRunSummary-style ordered operations; completion box with per-stage durations and next-steps advisories) instead of bare slog lines
  - buildNodeRunner takes consent state as explicit parameters; the `nodeYes = compactYes` cross-command flag aliasing is deleted
- **Depends on:** none

#### C10 — Keep --dry-run from rewriting the terraform root

- **Status:** in review — branch feat/tier-c-batch
- **Category:** trust / dry-run honesty
- **State:** well-specified
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/cli/node.go:133`, `internal/cli/node.go:92`
- **Rationale:** The "make resize --dry-run inert" commit fixed persistence and cluster mutation but missed the workspace migration, which rewrites operator-editable HCL on a path the flag explicitly promises is inert. Scripted `--dry-run --yes` probes are exactly how operators will test these commands before trusting them.
- **Acceptance:**
  - `node remove --dry-run --yes` against a pre-nodeops root leaves infrastructure/terraform/ byte-identical: either report "migration required, re-run without --dry-run" and exit cleanly, or preview against the embedded root in a scratch dir
  - Interactive dry-run no longer prompts the operator to consent to a real HCL rewrite mid-preview
  - Regression test alongside TestRemoveDryRunPreviewIsTruthfulAndInert covering the pre-migration root
- **Depends on:** none

#### C11 — Version-stamp the materialized terraform root and fix half-migration detection

- **Status:** in review — branch feat/tier-c-batch
- **Category:** trust / state-recovery
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/deploy/migrate.go:27-33`, `internal/deploy/migrate.go:54-75`
- **Rationale:** A crash between the two AtomicWrites bricks node ops permanently: the sniff test passes forever while main.tf never passes worker_count to the module, and the gate error even anticipates the state the migration code created. Config and deploy-state both have version stories; the terraform root's era is inferred by grepping files operators are explicitly told they may edit.
- **Acceptance:**
  - Immediate fix: a workspace with migrated variables.tf but pre-migration main.tf is detected as needing migration and re-offered the idempotent migrate; a crash injected between the two AtomicWrites self-heals on re-run instead of permanently dying at the plan gate
  - Materialization writes a manifest (format version + per-file content hash) alongside the root; a newer binary reports "workspace format N, binary expects M" with the migration it will perform, instead of grepping HCL
  - Operator-modified files are distinguished from stale-embedded files, so migration prompts say "you edited this; your version will be backed up" only when true
- **Depends on:** none

#### C12 — Add node list and per-node status visibility

- **Status:** in review — branch feat/tier-c-batch
- **Category:** feature-gap / observability
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/cli/status.go:126-138`, `internal/node/resize.go:125`
- **Rationale:** The branch makes node topology mutable but not inspectable: the two states the new commands create — role-config drift and an in-flight op — have no surface at all, and status hides exactly the thing operators check it for (which node is not ready) behind a jq incantation. All data is already in memory; resize's own error text says "run 'okdctl status' to list nodes" but status text output never prints node names.
- **Acceptance:**
  - `okdctl node list` exists with text and --output json: name, role, ready, terraform index, a pending-resize/drift indicator derived from config-vs-applied sizing, and any in-flight op marker
  - status's nodes section becomes an aligned per-node table inside the existing box (NotReady rows in error style), keeping the count summary as footer
  - describe node drops tabwriter for the dotted-KV form describe addon uses, so the describe group has one voice; --output json unchanged
  - Error messages that tell the operator to list nodes point at a command whose default output actually lists them
- **Depends on:** none

#### C13 — Add a day-2 cluster section to doctor

- **Status:** in review — branch feat/tier-c-batch
- **Category:** feature-gap / observability
- **State:** well-specified
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/cli/doctor_cmd.go:13`, `internal/cluster/k8s_etcd.go:31`
- **Rationale:** OKD's cert-expiry cliff is invisible until it hits, and okdctl already computes etcd health for node ops — surfacing it in doctor turns existing primitives into the monitoring story for an operator with no Prometheus stack yet.
- **Acceptance:**
  - When a deployed cluster's kubeconfig is present, doctor adds checks: Degraded/Progressing ClusterOperators, NotReady nodes, pending CSR count, etcd health, and the kube-apiserver-to-kubelet-signer NotAfter date with days remaining
  - Warns prominently when cert expiry is within 30 days; suggests CSR recovery when pending CSRs plus NotReady nodes appear together
  - Exit codes distinguish healthy/warn/fail so operators can cron it; --output json stays stable per the existing json-schema doc
- **Depends on:** none

#### C14 — Add okdctl plan (read-only drift preview)

- **Status:** done — PR #950
- **Category:** feature-gap / day-2
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/cli/node.go:69-71`, `internal/infrastructure/terraform/plangate.go:74-129`
- **Rationale:** The branch's per-role resize semantics manufacture drift as a feature, but there is no scriptable, parsed, dedicated drift audit: `deploy --dry-run` streams raw unscoped terraform output, exits 0 regardless, and buries drift in deploy-preview framing, while ParsePlanChanges/ShowPlanChanges already parse and classify plan output but are used only by the node-op plan gate.
- **Acceptance:**
  - `okdctl plan` runs terraform plan (with -lock-timeout, never apply) and prints per-resource create/update/replace/delete via the ParsePlanChanges rendering
  - Flags nodes whose live sizing differs from the role knob in okdctl.yaml — the exact pending set node resize leaves behind
  - Exits with a distinct nonzero code when drift exists (plan gains -detailed-exitcode plumbing), zero when clean; mutates nothing; help text explains how deploy reconciles what plan shows
  - The design reconciles with deploy --dry-run — extend it or absorb its plan step — instead of duplicating runDeployDryRun's creds/runlock/connect plumbing as a parallel path
- **Depends on:** none

#### C15 — Let the wizard review step jump to sections

- **Status:** in review — branch feat/tier-c-batch
- **Category:** tui / wizard
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/tui/wizard/steps/review.go:95-116`, `internal/tui/wizard/model_navigation.go`
- **Rationale:** The review screen is the wizard's last impression and its edit path is its worst ergonomic: a typo in step 3 of 11 costs ~16 keypresses to fix. Jump-and-return is the standard installer-wizard pattern and is contained entirely in the navigation model plus the review view.
- **Acceptance:**
  - Each rendered review section gains a bracketed index in its header; pressing that digit on the review screen jumps straight to the corresponding step
  - A returnToReview flag makes confirming (or Esc-ing) the edited step jump straight back to review instead of replaying intermediate steps
  - The review footer advertises it via the existing HelpProvider/ShortHelp seam; hidden steps (stepShouldShow false) are never assigned an index
- **Depends on:** none

#### C16 — Replace internals-facing node-op messages with operator actions

- **Status:** in review — branch feat/tier-c-batch
- **Category:** operator-ux / messages
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/node/resize.go:91`, `internal/node/remove.go:89`
- **Rationale:** Both messages fire at the moment of success, when the tool knows exactly what residual work remains, and hand the operator internals ("TODO: spec §11", "a root-capable path") instead of an action. The haproxy step constant and doc claim describe work the code demoted to a log hint — wire it or make the docs honest.
- **Acceptance:**
  - The resize message tells the operator how to verify the guest saw the new memory (`oc debug node/<name> -- free -m`, or the Proxmox UI) with no TODO/spec tokens
  - The remove message carries the concrete fix for the stale HAProxy backend (exact file/section and reload command, or the okdctl command that re-renders it); decide deliberately whether non-root node ops offer a sudo-gated haproxy refresh step
  - StepHAProxy (opstate.go:47) is either backed by a real refresh step or removed together with RemoveWorker's doc-comment claim of "a best-effort HAProxy backend refresh" that the code does not perform
  - CLAUDE.md's bare-TODO comment rule is applied to log strings too
- **Depends on:** none

#### C17 — Allow CPU-only resize without restating memory

- **Status:** in review — branch feat/tier-c-batch
- **Category:** operator-ux / flags
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/cli/node.go:296-298`, `internal/node/resize.go:53-62`
- **Rationale:** The flag surface advertises CPU as optional but memory as mandatory, so changing one knob means asserting the other — and a mis-typed memory value during a routine CPU bump rolls the whole role through cordon/drain for a change nobody asked for.
- **Acceptance:**
  - `okdctl node resize workers --cpu 8` succeeds, leaving memory unchanged in config, tfvars, and the plan-time -var overrides
  - Usage error only when neither --memory-mb nor --cpu is given; help text says at least one is required
  - Memory-budget guard skips cleanly on a CPU-only change
- **Depends on:** none

#### C18 — Route cluster package output checks through getJSONChecked

- **Status:** in review — branch feat/tier-c-batch
- **Category:** refactor / dedup
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/cluster/k8s_etcd.go:81`, `internal/cluster/k8s_nodeops.go:121-133`
- **Rationale:** The branch added the right helper and then didn't use it in the sibling files written in the same commit series — four inline copies of the same three-clause check in one package, one of which (MastersSchedulable) omits the Truncated guard entirely, a latent partial-JSON parse bug the fold fixes.
- **Acceptance:**
  - ListNodes, PodsForSelector, MastersSchedulable, and PendingCSRs (k8s_csrs.go:16-30) call getJSONChecked (or a promoted equivalent taking a msg-prefix parameter) instead of inlining exit-code/truncation checks
  - MastersSchedulable gains the Truncated guard it currently lacks; PendingCSRs loses its convention-violating "failed to" prefix
  - Error text for each caller keeps its verb-noun prefix; existing k8s_nodeops_test cases still pass
- **Depends on:** none

#### C19 — Make CoreOS stream pin lookup major-version aware before OKD 5.0 goes stable

- **Status:** in review — branch feat/tier-c-batch
- **Category:** correctness / okd-landscape
- **State:** well-specified
- **Effort:** hours
- **Impact:** large
- **Evidence:** `internal/distribution/okd/setup/coreos.go:217-221`, `internal/distribution/okd/setup/coreos.go:176-191`
- **Rationale:** parseOKDMinor parses "major.minor" and discards major; streamPins is keyed by minor alone. OKD 5.0 is already five engineering candidates deep (5.0.0-okd-scos.ec.0 through ec.4, openshift/installer has release-5.0/5.1 branches cut), so a 5.x version today fails with an error that hardcodes "4.%d" — and a future 5.x minor that collides with a pinned 4.x key would silently resolve to the wrong installer commit and SHA256, defeating the exact tamper-detection streamPins exists to provide.
- **Acceptance:**
  - streamPins (or its replacement) is keyed by an unambiguous (major, minor) pair, not minor alone; parseOKDMinor returns and uses both
  - The "not pinned" error reports the actual requested major.minor, never a hardcoded "4."
  - A test asserts "5.0.0-okd-scos.ec.4" and a "4.19.0-okd-scos.6"-style fixture resolve to distinct pins even though they collide under a minor-only key
  - scripts/update-coreos-pins.sh and the doc comment in coreos.go record major alongside minor for new pins
- **Depends on:** none

#### C20 — Extend ISO auto-detect and destroy cleanup to SCOS filenames

- **Status:** in review — branch feat/tier-c-batch
- **Category:** correctness / okd-landscape
- **State:** well-specified
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/distribution/okd/setup/coreos.go:96-100`, `internal/infrastructure/proxmox/hostssh/iso_cleanup.go:42`
- **Rationale:** OKD has shipped only SCOS-named boot ISOs since 4.19 (the current 4.22 artifact is literally scos-10.0.…-live-iso.x86_64.iso), but the local auto-detect globs match only fedora-coreos-*/fcos-* names, and the destroy-time removal path both searches for and — via its path-safety guard — actively refuses anything not named fedora-coreos-*.iso. On every SCOS cluster, a cached ISO is invisible to the fast path and `okdctl destroy` silently leaks the base ISO on the Proxmox host on every teardown.
- **Acceptance:**
  - Both call sites recognize scos-*.iso names, ideally by deriving the expected filename from the same stream metadata coreos.go already fetches instead of hardcoding a second drifting pattern list
  - The iso_cleanup.go safety guard accepts the SCOS name shape while still refusing arbitrary paths; a destroy-phase test proves an SCOS-named fixture ISO is found and removed
  - User-facing log/desc strings ("removing fedora-coreos iso", "no fedora-coreos-*.iso found") are accurate for SCOS-only deployments
- **Depends on:** none

#### C21 — Bump the default OKD version pin to current stable

- **Status:** in review — branch feat/tier-c-batch
- **Category:** freshness / okd-landscape
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/config/defaults.go`
- **Rationale:** New clusters default to 4.18.0-okd-scos.10 — the last pre-SCOS-boot-image release — while stable is at 4.22 with weekly patch drops. A fresh homelab deploy in mid-2026 should not start four minors behind on the default path.
- **Acceptance:**
  - Default version moves to the current stable (4.22.x) and the pin-bump procedure in scripts/update-coreos-pins.sh is exercised as part of the change
  - The wizard's version step and configs/examples/*.yaml agree with the new default
- **Depends on:** none

#### C22 — Proxmox HA anti-affinity rules for control-plane VM placement

- **Status:** done — PR #950
- **Category:** feature-gap / proxmox-native
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `infrastructure/terraform/modules/proxmox-okd/main.tf:65-69`
- **Rationale:** bpg/proxmox ships first-class HA resources (proxmox_virtual_environment_haresource, _harule with negative resource-affinity, PVE 9+), but the module registers none even though master_target_nodes already lets operators spread masters across physical hosts — and the startup{order} blocks it does use are documented host-local-only, so cross-host ordering silently degrades. Without enforced anti-affinity, a Proxmox HA failover can relocate two masters onto one surviving host, breaking the etcd failure domain the operator believed they had.
- **Acceptance:**
  - Opt-in variable (ha_enabled, default false — single-node PVE has no HA) creates one haresource per master plus one negative resource-affinity harule grouping all master VM IDs
  - PVE 9+ requirement stated explicitly; enabling on an older cluster fails the plan with a clear message
  - startup{} blocks documented as host-local-only and superseded when ha_enabled=true
  - terraform plan/apply idempotent with ha_enabled toggled both ways — no unrelated VM churn
- **Depends on:** none

#### C23 — Fix the silent FCOS fstrim failure so thin-storage discard reclaims space

- **Status:** done — PR #950
- **Category:** correctness / proxmox-native
- **State:** well-specified
- **Effort:** days
- **Impact:** medium
- **Evidence:** `infrastructure/terraform/modules/proxmox-okd/main.tf:77`
- **Rationale:** Every disk sets discard="on", but Fedora CoreOS's stock fstrim.timer runs `fstrim --fstab`, which unconditionally fails because FCOS ships no /etc/fstab (open upstream bug coreos/fedora-coreos-tracker#468) — so guest-side space reclaim on ZFS/Ceph/LVM-thin Proxmox storage silently never happens. A MachineConfig systemd override that trims real mountpoints closes it with manifest-injection plumbing okdctl already has.
- **Acceptance:**
  - A MachineConfig for both pools ships a systemd unit+timer trimming actual FCOS mountpoints instead of relying on --fstab
  - Comment links the upstream issue per CLAUDE.md's workaround convention
  - Verified against a thin-provisioned backend: deleting a large file followed by the timer firing measurably shrinks host-side usage versus the stock no-op baseline
- **Depends on:** none

#### C24 — Proxmox-native node snapshot/rollback safety net

- **Status:** done — PR #950
- **Category:** feature-gap / proxmox-native
- **State:** design needed
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/infrastructure/proxmox/hostssh/pvesh.go`, `infrastructure/terraform/modules/proxmox-okd/main.tf:56`
- **Rationale:** "Snapshot before I do something risky, roll back if it breaks" is a hypervisor-native undo button no generic installer can offer, and the hostssh/pvesh layer is its natural home. Distinct from the rejected etcd-backup scope: manual, single-node, short-lived, no scheduling or retention. Note qemu-guest-agent is currently disabled fleet-wide ("disabled for faster terraform operations"), so snapshots are crash-consistent unless agent enablement is revisited.
- **Acceptance:**
  - okdctl node snapshot create/list/rollback/delete <node> wraps pvesh snapshot endpoints, extending hostssh per the argv-mode SSH policy
  - Creation reuses cluster.Client Cordon/Drain when targeting a live node; warns crash-consistent-only when the VM has no guest agent
  - Rollback re-verifies node health (kubelet Ready, rejoined) and fails loudly rather than leaving a half-rolled-back node
  - Documented as a bounded manual safety net — no scheduling, no retention
- **Depends on:** none

#### C25 — Disable Red-Hat-subscription-gated defaults post-install

- **Status:** in review — branch feat/tier-c-batch
- **Category:** feature-gap / okd-polish
- **State:** well-specified
- **Effort:** days
- **Impact:** small
- **Evidence:** `internal/distribution/okd/postinstall/`
- **Rationale:** Every fresh OKD cluster throws a permanent, unresolvable InsightsDisabled alert (okd-project/okd#2058 has the maintainer on record that pull-secret-gated operators are non-functional on OKD) and runs three catalog-index pods (redhat-operators, certified-operators, redhat-marketplace) pulling indexes no OKD user can install from. okdctl can ship the cluster in the state OKD users end up hand-patching it into.
- **Acceptance:**
  - A postinstall step patches operatorhub.config.openshift.io/cluster to disable the three subscription-gated CatalogSources, leaving community-operators untouched
  - The InsightsDisabled alert is silenced so it stops presenting as actionable
  - Gated behind a default-on flag (e.g. --keep-redhat-catalogs to opt out) and documented as a deliberate deviation from stock installer defaults
- **Depends on:** none

#### C26 — Ship a chrony MachineConfig tuned for VM clock drift

- **Status:** in review — branch feat/tier-c-batch
- **Category:** feature-gap / okd-polish
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/distribution/okd/setup/ignition.go`, `README.md:185-186`
- **Rationale:** Clock skew after Proxmox snapshot/pause/resume cycles causes etcd election failures and "certificate is not yet valid" errors (RH KB 7033287) — a VM-specific failure class bare metal doesn't hit, and the README's current answer is "run ntpdate by hand and retry." A chrony MachineConfig with makestep tuned for fast step-correction fixes it structurally.
- **Acceptance:**
  - Control-plane + worker MachineConfig sets an explicit chrony server (default: the bastion, overridable in config) with makestep tuned to step-correct quickly after pause/resume
  - Wizard/config exposes the NTP source as an optional field with a sane default
  - README troubleshooting entry updated to reflect the structural fix
- **Depends on:** none

#### C27 — Rebuild ExtractTarGz containment on os.Root

- **Status:** in review — branch feat/tier-c-batch
- **Category:** build-vs-buy / security
- **State:** well-specified
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/download/extract.go:54-64`, `internal/download/extract.go:83-150`
- **Rationale:** ~40 LOC of hand-rolled path containment (HasPrefix checks, post-write EvalSymlinks re-verification, gosec suppressions) can be replaced by stdlib os.Root's kernel-enforced openat2-style containment — which also closes the EvalSymlinks-then-mkdir TOCTOU window the comments acknowledge. Zero new deps, net −30 LOC, strictly stronger invariant.
- **Acceptance:**
  - processTarEntry writes via an os.Root opened on destDir (Root.MkdirAll/OpenFile/Symlink); verifyResolvedPath is deleted
  - Explicit rejection of absolute/escaping symlink Linkname targets is retained (extracted trees are consumed later by non-Root code)
  - All existing zip-slip/symlink-traversal tests in extract_test.go pass unchanged
- **Depends on:** none

#### C28 — Reimplement system.WaitFor internals on apimachinery wait

- **Status:** in review — branch feat/tier-c-batch
- **Category:** build-vs-buy
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/system/wait.go:47-92`, `internal/download/retry.go:12`
- **Rationale:** WaitFor hand-rolls ticker+timer race handling — the subtlest concurrency code in internal/system — while the repo already standardized on k8s.io/apimachinery/pkg/util/wait at three production sites. Folding the internals deletes ~35 LOC and leaves one polling vocabulary instead of two.
- **Acceptance:**
  - WaitFor keeps its exact signature, polls/elapsed logging, ClusterError-on-timeout, and ctx-error-primary semantics; only the internal machinery moves to wait.PollUntilContextTimeout(immediate=true)
  - wait_test.go passes without behavioral edits; no new dependency
- **Depends on:** none

#### C29 — Replace system.NewUUIDv4 with crypto/rand.Text

- **Status:** in review — branch feat/tier-c-batch
- **Category:** build-vs-buy
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/system/runid.go:15-37`, `internal/cli/root.go:98`
- **Rationale:** The hand-rolled UUIDv4 encoder plus its test (~60 LOC) reduces to stdlib rand.Text() with identical entropy and panic-on-entropy-failure semantics. Only cost is cosmetic: run_id changes from 36-char UUID to 26-char base32.
- **Acceptance:**
  - run_id and debug-bundle ID call rand.Text(); runid.go and runid_test.go are deleted
  - Confirmed no consumer parses or validates the UUID shape before the format changes
- **Depends on:** none

#### C30 — Decide keep/kill on the --metrics-addr Prometheus endpoint

- **Status:** in review — branch feat/tier-c-batch
- **Category:** simplification / product-decision
- **State:** design needed
- **Effort:** hours
- **Impact:** large
- **Evidence:** `internal/deploymetrics/metrics.go:1`, `internal/cli/deploy.go:56`
- **Rationale:** A Prometheus scrape endpoint on a one-shot deploy CLI is the largest cohesive feature with the thinnest real-world justification — deliberate (PR #87), but no doc or test shows an actual scraping workflow. Killing it deletes ~550 LOC, two flags, and the MetricsRecorder plumbing's second consumer; keeping it deserves a written rationale so it stops resurfacing in reviews. Note the C7 checklist item adds a new MetricsRecorder consumer, so decide this first.
- **Acceptance:**
  - An explicit keep or kill verdict is recorded
  - If killed: internal/deploymetrics, internal/deploy/metrics.go, the --metrics-addr/--metrics-allow-network flags, and their tests are deleted; pending metrics-related roadmap items are closed as obsolete; MetricsRecorder itself stays (C7 consumes it)
  - If kept: the package doc states who scrapes a run-once CLI and why
- **Depends on:** none

#### C31 — Collapse the triplicated exponential-backoff retry wrapper

- **Status:** in review — branch feat/tier-c-batch
- **Category:** simplification
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/addon/helpers.go:32`, `internal/infrastructure/proxmox/proxmox.go:405`
- **Rationale:** Three near-identical wrappers around wait.ExponentialBackoffWithContext share the same backoff literals and ctx-cancellation epilogue — one comments that it "mirrors retryDownload" — and the errtypes package doc already names all three as consolidation targets. One shared helper deletes ~60-70 LOC and one copy-paste concept.
- **Acceptance:**
  - One Retry(ctx, backoff, classifier, fn) helper exists (natural home: internal/system next to WaitFor); download/retry.go, addon RetryDefault, and proxmox's inline loop all call it
  - The duplicated wait.Backoff literals and the "mirrors retryDownload" comment are gone; the three per-package isRetryable classifiers remain
- **Depends on:** none

#### C32 — Eliminate the hybrid third path in wizard step authoring

- **Status:** done — PR #950
- **Category:** simplification / wizard
- **State:** sketch
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/tui/wizard/steps/node_placement.go:51`, `internal/tui/wizard/datadriven.go:39`
- **Rationale:** Seven steps use the declarative StepDefinition framework, four hand-roll Update/View, and node_placement straddles both — embedding a DataDrivenStep and wrapping it in forwarding shims, proof the paths don't compose. Deleting the hybrid collapses step authoring from three paths to two documented ones.
- **Acceptance:**
  - node_placement is either fully declarative (framework gains the hook it needed) or fully hand-rolled; the forwarding shims are deleted
  - The declarative/hand-rolled boundary is stated in datadriven.go's package doc; ~100-150 LOC removed if the new hook lets 1-2 hand-rolled steps migrate
- **Depends on:** none

#### C33 — Delete redundant test restatements in postinstall and destroy suites

- **Status:** in review — branch feat/tier-c-batch
- **Category:** simplification / tests
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/distribution/okd/postinstall/update_ingress_test.go:163`, `internal/distribution/okd/destroy/steps_test.go:92`
- **Rationale:** Four tests hit the same buildLBIngressController builder re-asserting overlapping invariants, and destroy's FailurePath asserts a strict subset of its PartialFailure sibling. ~250 LOC of restatement deleted with zero loss of pinned behavior; verified against the coverage floors (same production lines stay covered by surviving tests).
- **Acceptance:**
  - The three redundant buildLBIngressController tests fold into the existing PreservesFields checkOpt table; TestDestroySteps_FailurePath is deleted; TestCustomISONames pair collapses to one two-row table
  - go test ./... passes and no .github/coverage-floors.conf floor regresses
- **Depends on:** none

#### C34 — Extract one shared slog capture-handler test fixture

- **Status:** done — superseded by PR #941 (slog-capture consolidation landed there first)
- **Category:** simplification / tests
- **State:** well-specified
- **Effort:** hours
- **Impact:** small
- **Evidence:** `internal/distribution/okd/destroy/steps_test.go:17`, `internal/distribution/okd/setup/coreos_test.go:23`
- **Rationale:** The same ~35-40-line hand-rolled slog.Handler is copied verbatim in five test packages — the one genuine fixture duplication the earlier test-scaffolding consolidation missed. One shared helper deletes ~140 LOC and stops the sixth paste.
- **Acceptance:**
  - A single CaptureHandler lives in a logutil-adjacent test helper package (e.g. internal/logutil/logtest) with records/WithAttrs-merge/last() semantics
  - The five verbatim copies (destroy, terraform, flux, secretstore, setup/isoCapture) are deleted; no production package imports the helper
- **Depends on:** none

### Tier D — findings from 2026-07-26 /audit-all run

Full 14-audit sweep (waves 1-2 audited a4d193d4 / rebased into develop; wave 3 audited develop@9b38c02b). 192 findings; aggregated report at `.claude/audits/audit-all-2026-07-26.md`. Fix campaign round 1 (waves 1-2, branch `fix/audit-w12`) applied same-day; statuses below reflect it. Scaffolding rows are verify-intent only.

#### audit-security

##### `sec:7b2829bb:cred-env-leak-to-child` — cred env leak to child

**Status:** done (fix/audit-w12 645944ad)
**Severity:** major
**Cluster:** credentials
**Evidence:** `internal/executor/executor.go:150-158`
**Problem:** DefaultEnvAllowlist's PROXMOX_ prefix forwards PROXMOX_VE_PASSWORD / PROXMOX_VE_API_TOKEN (which LoadEnvFile os.Setenv's into the process env) to every Executor subprocess — oc, ssh, scp, dnsmasq, package managers — not just the terraform/proxmox consumers that need them.
**Fix:** Split the allowlist by purpose: keep PROXMOX_ passthrough on the sudo re-exec in cli/elevation.go (the elevated process must re-see shell-env credentials) and keep explicit terraform.WithEnv(creds.Env()) wiring, but drop the PROXMOX_ prefix from the default subprocess allowlist (or filter just PROXMOX_VE_PASSWORD/PROXMOX_VE_API_TOKEN via logutil.KeyIsSecret) so oc/ssh/dnf/helm shellouts stop inheriting hypervisor credentials.
**Effort:** hours

##### `sec:35abd54e:cred-registry-drift` — cred registry drift

**Status:** done — already fixed upstream (c4b677d7)
**Severity:** minor
**Cluster:** credentials
**Evidence:** `internal/credentials/proxmox.go:158-163`
**Problem:** The Env() doc-comment registry of known call sites lists cli/deploy.go for the proxmox.WithEnv(creds.Env()) site, but that call now lives in cli/helpers.go:113. CLAUDE.md's reviewer checklist depends on this registry being accurate for ZeroizeEnv audits.
**Fix:** Update the registry to cli/helpers.go (and re-verify the other three entries: cli/destroy.go:391, cli/node.go:223, deploy/deploy.go:100 — all confirmed present with ZeroizeEnv coverage this run).
**Effort:** hours

##### `sec:5cb4c62a:sudo-partial-success` — sudo partial success

**Status:** done (fix/audit-w12 9569d111)
**Severity:** minor
**Cluster:** privilege-escalation
**Evidence:** `internal/hostnet/hostnet.go:56-62`
**Problem:** RemoveSecondaryIP runs two privileged mutations — nmcli connection modify (persistent profile) then nmcli device reapply (runtime) — with no rollback: a failure after modify leaves the NetworkManager profile changed but unapplied, so host runtime and persistent config silently diverge until the next reconnect.
**Fix:** On reapply failure, attempt a compensating `nmcli connection modify conn +ipv4.addresses ip/32` to restore the profile before returning, or at minimum name the divergence in the returned error so the operator knows the profile changed but was not applied.
**Effort:** hours

##### `sec:39672b27:re-exec-env-unfiltered` — re exec env unfiltered

**Status:** done (fix/audit-w12 cc7d0940)
**Severity:** suggestion
**Cluster:** privilege-escalation
**Evidence:** `internal/debugbundle/doctor.go:15-26`
**Problem:** collectDoctorOutput re-execs the current binary with cmd.Env unset, inheriting the full parent environment instead of the executor.FilterParentEnv allowlist every other subprocess and the sudo re-exec use. The child is okdctl itself, but the inconsistency bypasses the repo's env-hygiene chokepoint.
**Fix:** Set cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist) to match the sudo re-exec in cli/elevation.go; doctor consumes only allowlisted variables.
**Effort:** hours

##### `sec:9c7cfdc5:shellinj-argv-over-ssh-unquoted` — shellinj argv over ssh unquoted

**Status:** done (fix/audit-w12 77ba70a0)
**Severity:** suggestion
**Cluster:** shell-injection — seam→subprocess
**Evidence:** `internal/infrastructure/proxmox/hostssh/ssh.go:47-63`
**Problem:** SSHRunArgv documents that ssh joins argv with spaces and hands one string to the remote login shell — argv mode does NOT bypass the shell — so safety rests entirely on every caller validating each atom. All current atoms are allowlist-validated at the pveshRun/validateVMID/validateUPID chokepoints, but the function itself applies no quoting, so one future unvalidated caller reintroduces remote injection.
**Fix:** Defense-in-depth: shellSingleQuote each argv atom inside SSHRunArgv (keeping the existing validate-first discipline as the primary guard), or add a metacharacter assert that panics/errors on unvalidated atoms so a future caller fails closed instead of open.
**Effort:** hours

##### `sec:48688e63:shellinj-validate-path-decoupled` — shellinj validate path decoupled

**Status:** done (fix/audit-w12 0b7701b9)
**Severity:** suggestion
**Cluster:** shell-injection — seam→subprocess
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:480-486`
**Problem:** probeVMEnumeration string-builds the pvesh path "/nodes/"+p.node+"/qemu" at the call site; safety depends on pveshRun validating params.Node == p.node inside the helper. The coupling holds today but is implicit — a future caller interpolating a different unvalidated value into path would pass through pveshRun's node-only check.
**Fix:** Have PveshRun build node-scoped paths itself (e.g. accept a relative resource like "qemu" and compose "/nodes/<validated-node>/"+resource), so path atoms can never bypass the validation chokepoint.
**Effort:** hours

##### `sec:1cb08008:tls-no-redirect-cap` — tls no redirect cap

**Status:** done (fix/audit-w12 00e638ca)
**Severity:** minor
**Cluster:** tls-network
**Evidence:** `internal/infrastructure/proxmox/probe.go:140-145`
**Problem:** newProxmoxClient hand-rolls an http.Client (Timeout set) without the httputil capRedirects policy, so the credentialed go-proxmox client follows up to Go's default 10 redirects and lacks the cross-host Authorization-header guard every httputil-built client gets — a compromised/misconfigured endpoint could redirect the token cross-host.
**Fix:** Set CheckRedirect: httputil-style cap (export capRedirects or duplicate the 5-hop + cross-host-Authorization refusal) on this client; go-proxmox attaches auth per request, so the cross-host guard is the load-bearing part.
**Effort:** hours

##### `sec:1cb08008:tls-insecure-skip` — tls insecure skip

**Status:** done (fix/audit-w12 00e638ca)
**Severity:** suggestion
**Cluster:** tls-network
**Evidence:** `internal/infrastructure/proxmox/probe.go:141-144`
**Problem:** InsecureSkipVerify is set from the operator's config Insecure flag — a documented opt-in, nolint-annotated — but nothing warns at connection time that TLS verification is off, so a long-forgotten insecure: true keeps silently disabling verification on every probe and power-cycle.
**Fix:** Log one structured warning (logger.Warn("proxmox tls verification disabled", "endpoint", host)) when insecure is true, mirroring the provenance warnings GetProxmoxCredentials asks callers to surface. Do not change the opt-in mechanics.
**Effort:** hours

##### `sec:35abd54e:input-url-scheme-not-checked` — input url scheme not checked

**Status:** done (fix/audit-w12 3d5053fb)
**Severity:** suggestion
**Cluster:** tls-network
**Evidence:** `internal/credentials/proxmox.go:235-241`
**Problem:** GetProxmoxCredentials preserves an explicit http:// prefix on the Proxmox host without protest. config validators gate http:// behind insecure_http: true at load time, so the default path is safe — but the credentials layer itself would happily emit PROXMOX_VE_ENDPOINT=http://… for any caller that skips config validation.
**Fix:** Defense-in-depth: have GetProxmoxCredentials reject (or at least flag, like EndpointFromConfig) http:// endpoints unless cfg.Provider.Proxmox.InsecureHTTP is set, so the scheme gate does not depend on every caller having run ValidateOKDConfig first.
**Effort:** hours

##### `sec:8ea706f6:dl-no-signature` — dl no signature

**Status:** done — already fixed upstream (embedded checksum pins, tools.go)
**Severity:** suggestion
**Cluster:** tls-network
**Evidence:** `internal/distribution/okd/setup/tools.go:200-230`
**Problem:** Tool binaries are integrity-checked via a SHA-256 fetched from a checksumURL on the same origin as the artifact — protection against corruption, not against origin compromise — and no signature verification exists anywhere in the download path. The same-origin dependency is acknowledged in a comment at tools.go:57.
**Fix:** Where upstreams publish signatures (cosign for oc/okd release images, GPG for helm), verify them; otherwise pin known-good SHA-256 values in-repo per pinned tool version instead of fetching the digest from the same origin at run time (the CoreOS stream JSON already does this via a commit-pinned JSONSHA256).
**Effort:** hours

##### `sec:fe5a42c5:cred-in-log` — cred in log

**Status:** done (fix/audit-w12 6345eb0a)
**Severity:** major
**Cluster:** redaction — seam→errors
**Evidence:** `internal/infrastructure/proxmox/hostssh/snapshot.go:135-137`
**Problem:** pveshTaskCall folds raw remote pvesh stderr verbatim into fmt.Errorf on non-zero exit, bypassing executor.NewExitError whose Error() scrubs+truncates via logutil.RedactableStderr and whose Redacted() omits stderr from slog. The resulting plain wrapError reaches render.ErrorSummary and slog "err" attrs unscrubbed.
**Fix:** Replace with executor.NewExitError(ctx, "pvesh "+subcommand+" "+path, result.ExitCode, result.Stderr) — preserves exit-code context and the loud-fail semantics while routing stderr through RedactableStderr; or wrap the stderr in logutil.RedactableStderr before interpolation.
**Effort:** hours

##### `sec:41a9d4eb:keyset-incomplete` — keyset incomplete

**Status:** done (fix/audit-w12 b73492bb)
**Severity:** minor
**Cluster:** redaction
**Evidence:** `internal/logutil/redact.go:24-24`
**Problem:** secretKeyFragments omits credential, passphrase, and authorization — the last inconsistently, since stderrScrubRe (line 187) does scrub authorization. A future slog attr keyed passphrase/credential/authorization would pass the key-sweep (and config.Redacted, which shares the list via KeyIsSecret) unredacted. No current call site uses these keys.
**Fix:** Add "credential", "passphrase", "authorization" to secretKeyFragments so the key-sweep matches the stderr regex's coverage; deliberately leave "key"/"auth" out (over-redacts public_key/oauth_flow).
**Effort:** hours

##### `sec:41a9d4eb:stderr-scrub-shape-limit` — stderr scrub shape limit

**Status:** done (fix/audit-w12 b73492bb)
**Severity:** suggestion
**Cluster:** redaction
**Evidence:** `internal/logutil/redact.go:186-208`
**Problem:** RedactableStderr scrubs only key-adjacent shapes (key=value, key: value, quoted-JSON, Authorization: Bearer). A secret with no adjacent key token — a bare JWT, a base64 pull-secret blob, prose like 'auth failed for token abc123' where the value is not glued by [:=] — survives in the retained 200-byte head/tail window. Positive: PROXMOX_VE_PASSWORD=x IS caught (unanchored fragment match); scrub runs before truncation.
**Fix:** No code change required — record the guarantee honestly ("key-shaped secrets + bounded size", not "all secret-looking tokens") in the doc comment; optionally add an eyJ-prefix/long-base64-run masking pass before truncation, weighing false positives. Never shrink current coverage.
**Effort:** hours

##### `sec:c5e5c304:input-config-hcl-unvalidated` — input config hcl unvalidated

**Status:** done (fix/audit-w12 cb5b33c9)
**Severity:** major
**Cluster:** input-validation
**Evidence:** `internal/distribution/okd/setup/terraform.go:151-169` + 1 more
**Problem:** formatAdditionalNetworks renders operator-authored n.Model and n.Bridge into terraform.tfvars HCL with only %q escaping — which neutralizes quotes/newlines but not ${…} interpolation — and validators.go never iterates AdditionalNetworks (the primary proxmox.Bridge IS charset-validated at validators.go:374). VLANTag also lacks a 1-4094 bound.
**Fix:** In validateProxmoxConfig, loop proxmox.AdditionalNetworks: run interfaceNamePattern on each .Bridge, an allowlist (virtio|e1000|rtl8139|vmxnet3) on .Model, and bound .VLANTag to 1-4094 — mirroring the primary-Bridge validation.
**Effort:** hours

##### `sec:aa0f50f5:input-vmid-base-unbounded` — input vmid base unbounded

**Status:** done (fix/audit-w12 cb5b33c9) — bonus: MinimalConfig VMIDBase=0 latent bug fixed
**Severity:** minor
**Cluster:** input-validation
**Evidence:** `internal/config/validators.go:696-696` + 1 more
**Problem:** ValidateVMID (range 100-999999999) exists but is wired only to a wizard field; cfg.Topology.VMIDBase loaded from YAML is never range-checked before flowing into tfvars where terraform computes vmid = base + index. A zero/negative/huge base fails deep in terraform/proxmox instead of at config load. The dangerous pvesh sink is independently guarded by validateVMID at the hostssh boundary.
**Fix:** Add a validateTopology check applying the 100-999999999 bound to VMIDBase and rejecting base+maxNodeCount overflow past the ceiling at config load.
**Effort:** hours

##### `sec:a59109c1:input-path-not-prefix-checked` — input path not prefix checked

**Status:** done (fix/audit-w12 7f594f8c)
**Severity:** suggestion
**Cluster:** input-validation
**Evidence:** `internal/config/bindir.go:65-71`
**Problem:** ResolveBinDir (OKDCTL_BIN_DIR env or cfg.Deployment.BinDir) requires an absolute path and filepath.Cleans it, but its own doc admits '.. traversal is not rejected' — /usr/local/bin/../../etc resolves to /etc. Operator already controls the value, so no privilege boundary is crossed; defense-in-depth only.
**Fix:** Reject inputs containing a .. element before Clean (belt-and-suspenders), or prefix-allowlist the result to /usr, /opt, and $HOME like validateKubeconfigEnv does.
**Effort:** hours

#### audit-subprocess

##### `sub:40d315ad:combined-out-for-stdout-parse` — combined out for stdout parse

**Status:** done (fix/audit-w12 61700a78)
**Severity:** minor
**Cluster:** io-handling
**Evidence:** `internal/addon/catalog/flux/flux.go:331-353` + 1 more
**Problem:** Two flux sites machine-parse the ring-buffered (200-line tail) Result.Stdout of Run/RunChecked with no Truncated check: waitForControllers strings.Split-parses deployment readiness lines, and the deploy-key path feeds ssh-keyscan output into fingerprint verification and known_hosts filtering. Every JSON/multi-line parser in internal/cluster deliberately uses the byte-capped RunOutput + Truncated guard instead.
**Fix:** Swap env.Exec.Run -> env.Exec.RunOutput(ctx, 0, ...) (and RunChecked -> RunOutputChecked for the keyscan site), then treat result.Truncated as not-ready / hard error, mirroring internal/cluster/k8s_etcd.go getJSONChecked and internal/addon/catalog/secretstore/secretstore.go:270-276 (sops path).
**Effort:** hours

##### `sub:39672b27:unbounded-combined-output` — unbounded combined output

**Status:** done (fix/audit-w12 cc7d0940)
**Severity:** minor
**Cluster:** io-handling
**Evidence:** `internal/debugbundle/doctor.go:20-24`
**Problem:** The doctor re-exec merges stdout (--output json document) and stderr (--log-format json warn lines) into one unbounded bytes.Buffer that is archived as the bundle's doctor JSON artifact; a single warn-level log line interleaves into and corrupts the JSON document, and the buffer bypasses the executor's byte cap.
**Fix:** Give stdout and stderr separate buffers (archive stderr as a sibling .txt entry in the bundle), or route the re-exec through executor.RunOutput and store Result.Stdout, keeping the non-zero-exit-is-not-an-error semantics.
**Effort:** hours

##### `sub:39672b27:no-cmd-env` — no cmd env

**Status:** done (fix/audit-w12 cc7d0940)
**Severity:** suggestion
**Cluster:** io-handling — seam→security
**Evidence:** `internal/debugbundle/doctor.go:21-21`
**Problem:** The only subprocess spawn in the tree that neither goes through the Executor allowlist nor sets a filtered cmd.Env — the doctor re-exec inherits the full parent environment. The other two direct-exec sites (cli/elevation.go:122, system/elevation.go:215) both filter through executor.FilterParentEnv(DefaultEnvAllowlist).
**Fix:** Set cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist) before cmd.Run(), matching the two elevation call sites.
**Effort:** hours

#### audit-state-and-recovery

##### `state:bdf5a873:cleanup-deletes-live-artifacts` — cleanup deletes live artifacts

**Status:** done (fix/audit-w12 a4973027)
**Severity:** blocker
**Cluster:** destroy-safety — related: state:93957c53:cleanup-weak-confirm-gate
**Evidence:** `internal/distribution/okd/cleanup/artifacts.go:75-98`
**Problem:** cleanup Full/WorkOnly's first step deletes ClusterConfigDir (auth/kubeconfig + auth/kubeadmin-password, the only admin credentials) with no check that the infrastructure was actually destroyed; terraform.tfstate is correctly preserved, so the VMs keep running while the operator loses admin access. `okdctl cleanup` defaults to --kind full and frames the warning as removing 'local artifacts'.
**Fix:** Before removing cluster-config in Full/WorkOnly, probe tf.HasState() for the env: if state is populated, skip auth/ (+metadata.json) removal, require an explicit force flag, or issue a distinct confirmation naming kubeconfig/kubeadmin-password loss - mirroring the PostDestroy && !tf.HasState() gate the terraform step already uses (cleanup.go:269-281) and guardLiveCluster's explicit 'credentials will be lost' consent (okd/okd.go:211-232).
**Effort:** days

##### `state:fb54208a:postinstall-no-rollback-path` — postinstall no rollback path

**Status:** done (fix/audit-w12 2a5de4fc)
**Severity:** major
**Cluster:** crash-recoverability
**Evidence:** `internal/distribution/okd/postinstall/steps.go:55-112`
**Problem:** The fatal, irreversible cleanup-bootstrap step (destroys the bootstrap VM via targeted tf apply) runs BEFORE the NonFatal verify-kubevip; if verification then fails, deploy-production-dns is skipped (SkipWhen !KubeVIPVerified) and the phase still returns success, leaving api.* DNS pointed at the destroyed bootstrap with no rollback and no diagnostic telling the operator to run `okdctl update-ingress`.
**Fix:** Verify kube-vip is serving the API before destroying the bootstrap fallback (promote verify-kubevip ahead of cleanup-bootstrap as a gating precondition), and/or emit an end-of-phase diagnostic whenever BootstrapCleaned && (!KubeVIPVerified || !DNSDeployed): 'DNS still bootstrap-pointed but bootstrap destroyed - run okdctl update-ingress'. update_ingress.go:76-97 already detects bootstrap-DNS and can re-cut to the VIP; only the pointer to it is missing.
**Effort:** hours

##### `state:632c9087:crash-no-ondisk-rollback` — crash no ondisk rollback

**Status:** done (fix/audit-w12 6a4b2172)
**Severity:** major
**Cluster:** crash-recoverability
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:462-490`
**Problem:** convertToLoadBalancer deletes the IngressController, waits for the router to terminate, then recreates it via stdin-fed `oc create -f -`. The only copy of the original spec is in-memory (ic.RawJSON) and rollback is in-process; a SIGKILL/OOM between delete and create leaves the controller (including 'default') permanently gone with no on-disk backup - cluster-wide route outage recoverable only by hand-reconstructing the CR.
**Fix:** Persist the original IngressController JSON (and the built replacement) to a workdir backup file before issuing the delete; on update-ingress startup, detect controller-missing + leftover backup and offer/perform restore. Mirrors the on-disk backup+restore RemoveHAProxy already does (haproxy.go:75-80, update_ingress.go:629-648). Keeps the CLAUDE.md-documented stdin-fed oc create exception unchanged.
**Effort:** hours

##### `state:c19ee328:resume-breaks-after-ignition` — resume breaks after ignition

**Status:** done (fix/audit-w12 61924eee)
**Severity:** major
**Cluster:** phase-idempotency
**Evidence:** `internal/distribution/okd/setup/steps.go:156-174` + 1 more
**Problem:** generate-manifests' AlreadyDone (manifests/ dir + .complete sentinel) is defeated by the downstream generate-ignition step: `openshift-install create ignition-configs` consumes and deletes manifests/, taking the sentinel with it, and generate-config's guard keys on install-config.yaml.backup without ever re-materializing install-config.yaml. A setup interrupted after generate-ignition re-runs `create manifests` with no install-config and hard-fails, forcing --fresh.
**Fix:** When manifests/ is absent but install-config.yaml.backup exists and the ignition sentinel (.ignition.complete) is present, treat generate-manifests as already done; alternatively restore install-config.yaml from .backup (CopyFileMode 0600) before re-running create manifests, or move the manifests sentinel outside the consumable directory (clusterDir/.manifests.complete) like the ignition sentinel already is.
**Effort:** hours

##### `state:06f00bcb:ignition-webroot-weak-sentinel` — ignition webroot weak sentinel

**Status:** done (fix/audit-w12 21b8440f)
**Severity:** major
**Cluster:** phase-idempotency
**Evidence:** `internal/distribution/okd/setup/apache.go:177-188` + 1 more
**Problem:** deploy-ignition (ReRunSafeNo) copies bootstrap/master/worker.ign into the webroot with non-atomic system.CopyFileMode (O_TRUNC + io.Copy, cleanup only on error return, no temp+rename), while its AlreadyDone guard checks bare FileExists. A hard kill mid-copy leaves a truncated .ign the resume run skips - nodes then fetch corrupt ignition over HTTPS and fail first boot with no trail back to setup.
**Fix:** Write each webroot .ign atomically (read bytes then system.AtomicWrite with 0640, or temp+rename inside the webroot), and/or strengthen AlreadyDone to require minimum size + valid JSON (reuse the ignition validation used for the source files) instead of existence only.
**Effort:** hours

##### `state:f327eaf4:tf-module-drift-no-refresh` — tf module drift no refresh

**Status:** done (fix/audit-w12 e7439a4e) — warn-level drift detection; auto-refresh needs cli consent wiring (future item)
**Severity:** major
**Cluster:** state-schema-evolution
**Evidence:** `internal/deploy/migrate.go:24-39`
**Problem:** The materialize/migrate machinery (root manifest, format generation, backup-before-overwrite) tracks only environments/production/{variables,main}.tf. The proxmox-okd module files and the other env files have no manifest entry and no refresh path; MaterializeTerraform is write-once, so after an okdctl upgrade whose embedded module changed, an existing workspace silently keeps deploying the stale on-disk module while the new binary renders tfvars against the new one.
**Fix:** Extend the root manifest to hash every managed file (module + all env files) and have MaterializeTerraform (or a deploy preflight) diff on-disk hashes against embedded hashes, offering the same backup-then-refresh migration used for the two node-ops files. At minimum warn when the stamped root format predates the binary's embedded module.
**Effort:** hours

##### `state:c19ee328:download-tools-weak-sentinel` — download tools weak sentinel

**Status:** done (fix/audit-w12 713af41b)
**Severity:** minor
**Cluster:** phase-idempotency
**Evidence:** `internal/distribution/okd/setup/steps.go:115-122`
**Problem:** download-tools (ReRunSafeNo) guards on bare FileExists of openshift-install/oc/kubectl in binDir, and installation uses non-atomic system.CopyFile; a crash mid-copy leaves a truncated binary the guard treats as done.
**Fix:** Install binaries via temp-file+rename (AtomicWrite of the copied bytes preserving the executable mode) so an interrupted install leaves no partial file; optionally re-verify installed checksums in AlreadyDone.
**Effort:** hours

##### `state:29293401:haproxy-pristine-backup-clobbered` — haproxy pristine backup clobbered

**Status:** done (fix/audit-w12 7082ed77)
**Severity:** minor
**Cluster:** phase-idempotency
**Evidence:** `internal/distribution/okd/setup/haproxy.go:120-126`
**Problem:** ConfigureHAProxy (ReRunSafeYes) unconditionally overwrites the fixed pristine-backup path with the current live config on every run; from the second run onward the 'pristine' snapshot is okdctl's own generated config, so a later rollback restores okdctl's config instead of the operator's original.
**Fix:** Only write the pristine backup when it does not already exist (if !FileExists(haproxyBackupPath)); rely on postinstall's timestamped rolling backups for subsequent snapshots.
**Effort:** hours

##### `state:ab9b764a:backup-sentinel-nonatomic` — backup sentinel nonatomic

**Status:** done (fix/audit-w12 21b8440f)
**Severity:** minor
**Cluster:** phase-idempotency — related: state:c19ee328:resume-breaks-after-ignition
**Evidence:** `internal/distribution/okd/setup/ignition.go:117-120`
**Problem:** install-config.yaml.backup - which doubles as generate-config's AlreadyDone sentinel and the only rollback artifact for the consumed install-config - is written with non-atomic CopyFileMode; a crash mid-copy leaves a truncated .backup the guard accepts as clean completion.
**Fix:** Write the .backup atomically (read outputPath bytes, system.AtomicWrite to backupPath with 0600); this also hardens any future restore-from-backup fix for the resume-breaks-after-ignition finding.
**Effort:** hours

##### `state:d7ce9d16:restore-without-restart` — restore without restart

**Status:** done (fix/audit-w12 c21739c0)
**Severity:** minor
**Cluster:** crash-recoverability
**Evidence:** `internal/distribution/okd/dns/dns.go:251-268`
**Problem:** validateAndRestartDnsmasq's restart-failure branch restores the previous config from .backup but never re-attempts a restart, and the error text claims 'previous config restored' - the on-disk config is restored but dnsmasq may be left down or running the rejected config until something else restarts it.
**Fix:** After restoring the backup on the restart-failure path, attempt one best-effort restart with the restored config and log the outcome, so the running service converges to the known-good config.
**Effort:** hours

##### `state:073d24ed:prelock-materialize-window` — prelock materialize window

**Status:** done (fix/audit-w12 aadece3f)
**Severity:** minor
**Cluster:** tf-state-atomicity
**Evidence:** `internal/cli/deploy.go:64-76`
**Problem:** MaterializeTerraform (embedded .tf sources + root manifest + chown) and the wizard's okdctl.yaml/okdctl.env writes all run in runDeploy before deploy.Execute acquires the project runlock, so two concurrent deploys can both materialize and both rewrite config/env before either takes the lock. Harm is bounded: every write is per-file AtomicWrite and materialization is write-once idempotent, so worst case is last-writer-wins, not torn state.
**Fix:** Acquire the runlock at the top of runDeploy (before MaterializeTerraform and config/env writes) instead of deep inside deploy.Execute; the interactive wizard can stay outside the lock, only the write paths need it.
**Effort:** hours

##### `state:93957c53:cleanup-weak-confirm-gate` — cleanup weak confirm gate

**Status:** done (fix/audit-w12 e6a05c57)
**Severity:** minor
**Cluster:** destroy-safety — related: state:bdf5a873:cleanup-deletes-live-artifacts
**Evidence:** `internal/cli/cleanup.go:100-109`
**Problem:** The interactive gate for `okdctl cleanup` - which by default (kind=full) deletes the cluster's kubeconfig/kubeadmin-password and terraform tfvars - is a single y/N prompt, while destroy and node snapshot rollback both require typing the exact cluster name for comparably destructive operations.
**Fix:** Use the two-stage gate (promptForClusterNameConfirmation then y/N) for cleanup kinds that remove credential files (full, work-only), matching confirmDestroyInteractive; scoped kinds can keep the single prompt.
**Effort:** hours

##### `state:0f076161:destroy-topology-drift-blind` — destroy topology drift blind

**Status:** done (fix/audit-w12 402cdccc)
**Severity:** minor
**Cluster:** state-schema-evolution
**Evidence:** `internal/cli/destroy.go:62-95` + 1 more
**Problem:** expandOnlyFlag and customISONames derive scoped-destroy targets and ISO names from the CURRENT config topology counts; nothing records the deployed topology. If the operator edits worker/master counts between deploy and destroy, `destroy --only=workers` under-enumerates (higher-index VMs stay, though still in tf state) and custom-ISO removal misses files - silent under-coverage rather than an error.
**Fix:** Derive scoped targets from `terraform state list` (the authoritative record of deployed resources) instead of config counts, or cross-check config counts against state and warn on drift; same for customISONames (enumerate <role><n>.iso by listing state or the remote ISO dir).
**Effort:** hours

##### `state:eff8657e:crash-marker-clear-best-effort` — crash marker clear best effort

**Status:** done (fix/audit-w12 5debfe62)
**Severity:** suggestion
**Cluster:** crash-recoverability
**Evidence:** `internal/deploy/state.go:62-66`
**Problem:** clearDeployMarker removes the deploy-state marker best-effort (warn on failure). If the remove fails after a fully successful deploy, the next `okdctl deploy` reads the stale phase=postinstall marker and silently routes through resume (postinstall-only) instead of a fresh run.
**Fix:** Stamp the marker with a terminal completed state (treated as absent by resolveResumePhase) instead of relying on remove, or cross-check the marker's phase against terraform-state presence before granting a skip-setup resume.
**Effort:** hours

##### `state:fd2125dd:concurrent-run-addon-unlocked` — concurrent run addon unlocked

**Status:** done (fix/audit-w12 24005722)
**Severity:** suggestion
**Cluster:** tf-state-atomicity
**Evidence:** `internal/cli/addon.go:165-180`
**Problem:** addon install/uninstall mutate the live cluster (oc/helm apply-delete) without acquiring the project runlock, while the sibling cluster-mutating verbs (update-ingress, node ops, deploy's own addon step) all serialize; concurrent addon vs deploy-postinstall runs are unserialized.
**Fix:** Acquire the project runlock in runAddonInstall/runAddonUninstall for symmetry; server-side applies are idempotent so harm is low, but the lock keeps the 'mutating command holds the lock' invariant uniform.
**Effort:** hours

##### `state:853644c7:schema-no-version` — schema no version

**Status:** done (fix/audit-w12 35502d02)
**Severity:** suggestion
**Cluster:** state-schema-evolution
**Evidence:** `internal/distribution/okd/releases/cache.go:43-53`
**Problem:** The on-disk releases cache JSON carries no schema-version field; json.Unmarshal tolerates unknown/missing fields, so after a struct-shape change an old cache is silently served (wrong-shaped Series) for up to the 1h TTL instead of being discarded.
**Fix:** Add a schema field to diskCache and treat a mismatch like corruption (return nil, refetch), matching the versioned-marker pattern used by deployState/opState.
**Effort:** hours

##### `state:fe5a42c5:api-no-conflict-handling` — api no conflict handling

**Status:** done (fix/audit-w12 8370bcc6)
**Severity:** suggestion
**Cluster:** proxmox-api-idempotency
**Evidence:** `internal/infrastructure/proxmox/hostssh/snapshot.go:218-241`
**Problem:** CreateSnapshot performs no duplicate-name pre-check; creating a snapshot whose name already exists surfaces only as the raw pvesh task exitstatus string, and the auto-generated okdctl-<timestamp> default makes accidental reuse of a hand-picked --name the only collision path.
**Fix:** Pre-check via ListSnapshots and return a typed already-exists error naming the snapshot (or document the pvesh error passthrough); keeps the manual-primitive scope per the maintainer's Tier C triage.
**Effort:** hours

#### audit-iac-and-shell

##### `iac:e076e43c:sh-release-asset-name-drift` — sh release asset name drift

**Status:** done (fix/audit-w12 c94fd49d)
**Severity:** blocker
**Cluster:** install-sh-integrity
**Evidence:** `scripts/install.sh:89-127` + 2 more
**Problem:** install.sh constructs the release asset name as okdctl_<v>_Linux_x86_64.tar.gz, but .goreleaser.yaml's explicit name_template emits lowercase okdctl_<v>_linux_amd64.tar.gz (README 'Verifying a release' documents the lowercase names). Every download URL the script builds 404s, so the documented curl|bash install path dies on first use for both arches.
**Fix:** Align install.sh with the goreleaser template: map OS to lowercase 'linux' and x86_64->'amd64' (keep aarch64|arm64->'arm64'), i.e. edit the two case statements and nothing else. README.md L221 already documents the lowercase scheme, so install.sh is the one-file fix; alternatively change .goreleaser.yaml name_template to the title-case/x86_64 default the script was written against, but that would also require a README edit.
**Effort:** days

##### `iac:ef8f2924:hcl-noop-data-disk-guard` — hcl noop data disk guard

**Status:** done (fix/audit-w12 21712934)
**Severity:** major
**Cluster:** hcl-destroy-ordering — seam→state-and-recovery
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/variables.tf:71-79` + 2 more
**Problem:** minimum_data_disk_size_gb's description claims setting it to 1 'prevents a re-apply that zeros master_data_disk_size_gb or worker_data_disk_size_gb from silently destroying existing disks', but the dynamic-disk gate `size >= minimum && size > 0` is behaviorally identical for minimum 0 and 1 (integer size >= 1 iff size > 0), and any zeroing still empties the for_each — terraform removes the Ceph OSD disk in place regardless. The guard is a provable no-op, and the production root never exposes the variable, so it cannot even be set.
**Fix:** Either make the guard real or delete the false promise: add lifecycle preconditions on master/worker failing when 0 < size < minimum (catches accidental shrink-typos), rewrite minimum_data_disk_size_gb's description to state that zeroing always removes the disk and no variable can prevent it, or drop the variable entirely and rely on the existing INVARIANT comments. Any change to the for_each gate must be planned against existing state to avoid forcing disk churn on applied clusters.
**Effort:** hours

##### `iac:e076e43c:sh-curl-pipe-sh` — sh curl pipe sh

**Status:** done (fix/audit-w12 f416bf48)
**Severity:** minor
**Cluster:** install-sh-integrity
**Evidence:** `scripts/install.sh:40-204`
**Problem:** README and the script header instruct `curl -sSfL ... | bash`, and the body executes top-to-bottom as it streams — there is no main() wrapper, so a connection dropped mid-transfer executes a truncated prefix of the script. Linear ordering keeps all verification ahead of any privileged operation, so exposure is low, but the standard wrapper closes the truncation window entirely.
**Fix:** Wrap the executable body in `main() { ... }` and invoke `main "$@"` as the final line so bash parses the entire script before executing anything (rustup/get.helm.sh installer pattern). Also consider serving install.sh from release assets so the piped script is itself a tagged artifact rather than the develop branch tip.
**Effort:** hours

##### `iac:b611e9fe:hcl-resource-summary-drift` — hcl resource summary drift

**Status:** done (fix/audit-w12 2d8e04bd)
**Severity:** minor
**Cluster:** hcl-correctness
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/output.tf:145-166`
**Problem:** cluster_resources.total_storage_gb adds var.os_disk_size_gb for the bootstrap node unconditionally, while the cpu/memory terms in the same output are gated on var.bootstrap_enabled. bootstrap_enabled=false is the steady state after install, so every standing cluster over-reports storage by one OS disk; data-disk sizes are also counted whenever nonzero even when 0 < size < minimum_data_disk_size_gb omits the disk block.
**Fix:** Gate the bootstrap term with var.bootstrap_enabled ? var.os_disk_size_gb : 0, and mirror the dynamic-disk for_each condition when summing data-disk contributions so the summary matches what is actually provisioned.
**Effort:** hours

##### `iac:18a795d5:hcl-no-prevent-destroy` — hcl no prevent destroy

**Status:** done as policy (3742e9fe) — prevent_destroy NOT added (breaks node remove count-driven destroy); asymmetry + OSD footgun documented; new tripwire test
**Severity:** suggestion
**Cluster:** hcl-destroy-ordering — seam→state-and-recovery
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/main.tf:280-399`
**Problem:** Worker VMs carry the cluster's Ceph OSD data by default (worker_data_disk_size_gb defaults to 500, master data disk defaults to 0) yet have no prevent_destroy, while the diskless-by-default masters are hard-protected. Partially deliberate — okdctl node remove drives the worker set by count and hashicorp/terraform#3116 makes prevent_destroy unconditional — but the resource holding the most user data is the least protected.
**Fix:** Do NOT add prevent_destroy to workers (it would break okdctl node remove's count-driven destroy). Instead document at the node-remove seam that removing a worker destroys its 500GB OSD disk, and/or add a drain-and-purge-OSD runbook step ahead of worker destroy; revisit if finding iac:ef8f2924:hcl-noop-data-disk-guard's fix lands a real disk guard.
**Effort:** hours

##### `iac:54c1681c:sh-poll-loop-no-fail` — sh poll loop no fail

**Status:** done (fix/audit-w12 ea166038)
**Severity:** suggestion
**Cluster:** shell-fail-closed
**Evidence:** `scripts/demo/record.sh:29-32`
**Problem:** The fakepve readiness loop silently falls through after 20 attempts (~10s): if the fake API never comes up, vhs still runs and fails minutes into the ~4-minute recording with no hint that the API was the cause.
**Fix:** After the loop, probe once more and `exit 1` with a clear 'fakepve did not become ready' message when the API is still unreachable, so the failure surfaces in seconds instead of mid-recording.
**Effort:** hours

##### `iac:ef8f2924:hcl-missing-validation` — hcl missing validation

**Status:** done (fix/audit-w12 8ff47446) — regex mirror of Go validator, not contains-list (arbitrary QEMU models deliberately admitted)
**Severity:** suggestion
**Cluster:** hcl-correctness
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/variables.tf:285-289`
**Problem:** cpu_type's description enumerates the legal set (host, x86-64-v2, x86-64-v3, kvm64) but the variable has no validation block, unlike nearly every sibling variable in the module; a typo surfaces only as a provider error mid-apply instead of at plan time.
**Fix:** Add `validation { condition = contains(["host", "x86-64-v2", "x86-64-v3", "kvm64"], var.cpu_type) ... }` — or widen the description if arbitrary QEMU cpu models are intentionally allowed, in which case say so instead of listing four.
**Effort:** hours

##### `iac:e076e43c:sh-install-dir-missing` — sh install dir missing

**Status:** done (fix/audit-w12 84c224c2)
**Severity:** suggestion
**Cluster:** install-sh-fail-closed
**Evidence:** `scripts/install.sh:195-201`
**Problem:** When a user-supplied INSTALL_DIR does not exist, `[ -w "$INSTALL_DIR" ]` is false and the script silently escalates into the sudo branch, where `sudo install` fails with a raw coreutils error (or the script dies about sudo being absent) instead of a clear 'INSTALL_DIR does not exist' diagnostic.
**Fix:** Before the writability check, `[ -d "$INSTALL_DIR" ] || die "INSTALL_DIR does not exist: $INSTALL_DIR"` (or create it explicitly with install -d when that is the intended contract).
**Effort:** hours

##### `iac:18a795d5:hcl-section-dividers` — hcl section dividers

**Status:** done (fix/audit-w12 b657b11c)
**Severity:** suggestion
**Cluster:** hcl-correctness — seam→documentation
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/main.tf:1-3` + 25 more
**Problem:** Every .tf file is organized with `# ====` banner section dividers, which CLAUDE.md's comment policy lists as a delete category ('Use file splits or function names. Delete dividers.'). The policy's examples are Go, and banner comments are common terraform style, so this is offered as a consistency nicety, not a defect.
**Fix:** If CLAUDE.md's divider rule is intended to be repo-wide, drop the banner framing and keep the content lines as plain `# provider configuration` headers (or split files); if terraform files are meant to be exempt, note the exemption in CLAUDE.md so future audits skip this.
**Effort:** hours

#### audit-errors

##### `err:fe5a42c5:err-wraps-cred-downstream` — err wraps cred downstream

**Status:** done (fix/audit-w12 6345eb0a)
**Severity:** major
**Cluster:** redaction-in-error — seam→security — related: sec:fe5a42c5:cred-in-log, sec:41a9d4eb:stderr-scrub-shape-limit
**Evidence:** `internal/infrastructure/proxmox/hostssh/snapshot.go:135-136`
**Problem:** pveshTaskCall folds the raw remote pvesh stderr tail (up to 200 ring-buffered lines) verbatim into its error's Error() string via fmt.Errorf, bypassing executor.NewExitError whose Error() truncates+scrubs via logutil.RedactableStderr and whose Redacted() omits stderr from slog. The same bypass also drops the ctx.Err() short-circuit, so a cancelled snapshot task loses SIGINT-identity (130/143 mapping).
**Fix:** Replace the fmt.Errorf with executor.NewExitError(ctx, "pvesh "+subcommand, result.ExitCode, result.Stderr): stderr becomes truncated+scrubbed in Error(), omitted from slog via Redacted(), and ctx cancellation returns ctx.Err() so cli/root.go::signalExitCode maps SIGINT→130/SIGTERM→143 again. Keep path out of the Command label per TestExitErrorCommandNoArgvLeak.
**Effort:** hours

##### `err:366b3f2d:wrap-cause-dropped-at-sink` — wrap cause dropped at sink

**Status:** done (fix/audit-w12 2a8a1f97)
**Severity:** major
**Cluster:** wrapping — related: err:8ea706f6:vocab-untyped-phase-error
**Evidence:** `internal/distribution/orchestrator.go:143-157` + 3 more
**Problem:** classifyStepErr wraps every untyped step error in ClusterError{Msg:"step failed"}. Because errtypes Error() surfaces Msg only (redaction contract), the root-cause text of any bare step error is dropped from every sink: the orchestrator's own Warn logs err as "cluster error: step failed", render.FailureSummary prints only the failed step ID, and the run log carries the same Msg-only string. An operator gets "step failed" with no cause unless the origin site separately logged it.
**Fix:** Include the bare error's text in the wrap Msg — e.g. &errtypes.ClusterError{Msg: "step failed: " + err.Error(), Err: err} — which is no less safe than the prior exit-1 path (stderr-bearing errors in the chain are executor.ExitError, whose Error() already scrubs+truncates); alternatively log the pre-classification err at the orchestrator.go:208 Warn site.
**Effort:** hours

##### `err:6b533f2d:wrap-double-context` — wrap double context

**Status:** done (fix/audit-w12 8b7c138f)
**Severity:** minor
**Cluster:** wrapping
**Evidence:** `internal/cluster/k8s_csrs.go:32-32` + 85 more
**Problem:** Typed-error Msg strings are pervasively prefixed with "failed to" (~86 error-value sites), violating CLAUDE.md's terse verb-noun error style. errtypes Error() surfaces only Msg, so these are the actual user-facing wrap text, while the repo's ~500 fmt.Errorf sites already follow the terse form — an internal inconsistency.
**Fix:** Mechanically strip the "failed to " prefix from the ~86 error-value Msg strings ("failed to parse CSRs" → "parse CSRs", "failed to write config to %s" → "write config %s"). Pure string edits; types, Unwrap chains, and exitCodeFor classification unchanged.
**Effort:** hours

##### `err:8ea706f6:vocab-untyped-phase-error` — vocab untyped phase error

**Status:** done (fix/audit-w12 9b76cb03)
**Severity:** minor
**Cluster:** domain-vocabulary — related: err:366b3f2d:wrap-cause-dropped-at-sink
**Evidence:** `internal/distribution/okd/setup/tools.go:186-348` + 3 more
**Problem:** Environment/config-class failures in the setup phase are returned as bare fmt.Errorf and rely on classifyStepErr defaulting them to ClusterError (exit 4), while sibling failures in the same package are explicitly typed ConfigError (exit 2). The exit code a user sees for a comparable failure depends on whether the author remembered to construct a typed error.
**Fix:** Retype environment/tool-verification failures in setup as *errtypes.ConfigError (or NetworkError where a fetch is involved) so exit codes match the explicitly-typed siblings: tools.go:188, tools.go:259, tools.go:347, apache.go:58. No new vocabulary needed; keep classifyStepErr's default.
**Effort:** hours

##### `err:4ded56d3:err-formats-cred` — err formats cred

**Status:** done (fix/audit-w12 e264736f)
**Severity:** minor
**Cluster:** redaction-in-error — seam→observability
**Evidence:** `internal/download/retry.go:18-30` + 1 more
**Problem:** HTTPStatusError.Error() interpolates the request URL and a ≤256-byte raw excerpt of the untrusted server response body, with a doc comment explicitly declining credential scrubbing and no Redacted() method. It has a concrete slog sink: download.go:135 logs the raw error as an attr, where RedactHandler's Redacted()-dispatch cannot scrub a pre-rendered string.
**Fix:** Add a Redacted() any method to HTTPStatusError returning {Status, Method} (omitting URL and Body), mirroring executor.ExitError, so the slog attr path at download.go:135 cannot surface raw body/URL; the terminal path is already fronted by a Msg-only NetworkError.
**Effort:** hours

##### `err:1cb08008:err-wraps-cred-downstream` — err wraps cred downstream

**Status:** rejected — Msg-only NetworkError wrap would erase transport diagnostics on warn-and-degrade + power-cycle paths; leak undemonstrated (row self-rated low-confidence)
**Severity:** suggestion
**Cluster:** redaction-in-error
**Evidence:** `internal/infrastructure/proxmox/probe.go:62-94` + 1 more
**Problem:** ProbeHost/PowerCycler wrap third-party go-proxmox client errors with plain %w at the boundary where the client is constructed from live PROXMOX_VE_PASSWORD/API_TOKEN bytes. The wrapped downstream error type has no Redacted() and okdctl cannot guarantee its Error() excludes auth material. Boundary-hygiene note; no concrete leak demonstrated in observed go-proxmox versions.
**Fix:** Optional hardening: wrap go-proxmox call errors in errtypes.NetworkError/AuthError (Msg-only, third-party error in Err) at the ProbeHost/PowerCycler boundary so a future go-proxmox error rendering auth context stays out of stringified sinks.
**Effort:** hours

##### `err:451be4fa:ctx-err-unwrapped` — ctx err unwrapped

**Status:** done (fix/audit-w12 e0ec2ddb)
**Severity:** suggestion
**Cluster:** cancellation-identity — seam→concurrency
**Evidence:** `internal/system/elevation.go:213-220`
**Problem:** HasPasswordlessSudo returns cmd.Run() directly: a ctx-timeout-killed `sudo -n true` surfaces *exec.ExitError ("signal: killed") rather than ctx.Err(), unlike the executor.run convention that substitutes ctx.Err() when ctx is done. Harmless today — the sole caller (doctor.go:249) swallows the error into a Warn Result and no downstream errors.Is matcher observes it.
**Fix:** Mirror executor.go:439: after cmd.Run() fails, return ctx.Err() when non-nil — or leave as-is given the advisory-only, swallowed-error call site.
**Effort:** hours

##### `err:f2998668:ctx-err-fabricated-identity` — ctx err fabricated identity

**Status:** done (fix/audit-w12 6d541452)
**Severity:** suggestion
**Cluster:** cancellation-identity
**Evidence:** `internal/system/wait.go:99-111`
**Problem:** WaitFor synthesizes context.DeadlineExceeded as the Err of its timeout ClusterError even when the timeout is WaitFor-internal and no context deadline fired (the ctx.Err()==nil guard proves it). This fabricates rather than loses sentinel identity: a future caller using errors.Is(err, context.DeadlineExceeded) to mean "a context deadline fired" gets a true positive no context produced. Documented and deliberate; signalExitCode is safe because it also requires a caught signal.
**Fix:** If any consumer ever needs to distinguish ctx-deadline from poll-timeout, introduce a dedicated sentinel (e.g. errtypes.ErrWaitTimeout) instead of borrowing context.DeadlineExceeded; otherwise leave — current behavior is intentional and commented.
**Effort:** hours

#### audit-concurrency

##### `con:632c9087:cancelled-ctx-cleanup` — cancelled ctx cleanup

**Status:** done (fix/audit-w12 6a4b2172)
**Severity:** major
**Cluster:** ctx-ignored — seam→state-and-recovery — related: state:632c9087:crash-no-ondisk-rollback, con:ddf885f4:cancelled-ctx-cleanup
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:482-484` + 1 more
**Problem:** attemptRollback runs `oc create -f -` with the same ctx whose cancellation may be exactly why createReplacement failed. On Ctrl-C in the delete-then-create window, exec.CommandContext under a cancelled ctx fails before starting, so the rollback is guaranteed to fail and the cluster is left with no IngressController.
**Fix:** In the rollback branch derive a detached bounded ctx exactly as node/add.go does: rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute); defer cancel(); p.attemptRollback(rbCtx, ic). Same for the dns/haproxy rollback path around L279.
**Effort:** hours

##### `con:ddf885f4:cancelled-ctx-cleanup` — cancelled ctx cleanup

**Status:** done (fix/audit-w12 e1ecb03a)
**Severity:** minor
**Cluster:** ctx-ignored — related: con:632c9087:cancelled-ctx-cleanup
**Evidence:** `internal/addon/manager.go:107-111` + 1 more
**Problem:** InstallAll and InstallOne roll back a failed addon via a.Uninstall(ctx, env) with the caller ctx. When the install failed because ctx was cancelled (Ctrl-C), every Uninstall exec fails before starting, so rollback silently degrades to a Warn log and a half-installed addon remains.
**Fix:** Wrap the rollback calls in a detached bounded ctx (context.WithTimeout(context.WithoutCancel(ctx), d)) mirroring internal/node/add.go's teardown, so a SIGINT mid-install still unwinds the partial addon.
**Effort:** hours

##### `con:f2998668:ctx-not-threaded` — ctx not threaded

**Status:** done (fix/audit-w12 6d541452)
**Severity:** minor
**Cluster:** ctx-ignored
**Evidence:** `internal/system/wait.go:62-92`
**Problem:** WaitFor's condition deliberately ignores the deadline-bound ctx PollUntilContextTimeout supplies and calls check with the caller's original ctx, so opts.Timeout never bounds an in-flight probe. A probe that hangs (oc against a blackholed API; deploy's caller ctx has no deadline) stalls WaitFor arbitrarily past its configured timeout.
**Fix:** Keep the dont-cancel-mid-probe intent but bound it: derive probeCtx := context.WithDeadline(callerCtx, pollDeadline.Add(grace)) once, pass probeCtx to check; a hung probe then dies at deadline+grace instead of never.
**Effort:** hours

##### `con:2845ab58:synctest-opportunity` — synctest opportunity

**Status:** done (fix/audit-w12 6cf53e17)
**Severity:** suggestion
**Cluster:** time-sleep-retry
**Evidence:** `internal/tui/spinner_test.go:176-194` + 2 more
**Problem:** The last real-time waits in the test suite: TestStatusLine_SetUpdatesDesc sleeps 300ms wall-clock to let the spinner ticker repaint, and two more tests sleep 5ms/2ms per iteration. The repo already runs kubectl, monitor, retry, and update-ingress wait tests under testing/synctest; these ticker-driven tests are the remaining candidates and the 300ms sleep is both slow and load-sensitive.
**Fix:** Wrap the ticker-dependent assertions in synctest.Test; replace the sleeps with synctest.Wait plus fake-clock time.Sleep advances, mirroring internal/distribution/okd/install/monitor_test.go:L193-L215. Medium risk: the lineReg mutex and buffer writers must stay inside the bubble.
**Effort:** hours

##### `con:c4182b1c:speculative-lock` — speculative lock

**Status:** not started
**Severity:** suggestion
**Cluster:** waitgroup-vs-errgroup — related: con:15ba17da:speculative-lock
**Evidence:** `internal/distribution/context.go:12-16`
**Problem:** PhaseContext carries an RWMutex although Orchestrator.Run is serial today, so the lock has no concurrent callers. The doc comment declares it forward-looking for a parallel-step mode; under MEMORY.md scaffolding rules this stays, but intent should be re-verified if no parallel-step roadmap item materializes.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `con:15ba17da:speculative-lock` — speculative lock

**Status:** not started
**Severity:** suggestion
**Cluster:** waitgroup-vs-errgroup — related: con:c4182b1c:speculative-lock
**Evidence:** `internal/distribution/okd/destroy/steps.go:34-43`
**Problem:** destroyTracker guards its errs/failures/skipped slices with an RWMutex although onError/skipWhen have no concurrent callers under today's serial Orchestrator.Run. Documented forward-looking with the same parallel-step rationale as PhaseContext; stays per scaffolding rules, verify intent alongside it.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

#### audit-api-design

##### `api:66f217c9:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/releases/okd.go:47-71` + 1 more
**Problem:** OKDVersionFetcher.GetLatestStable and GetLatestForMinor have no caller anywhere in the repo (docs-gen build included); only FetchVersions is consumed (cli/releases.go, wizard). They form a symmetric latest/latest-for-minor query family shaped for a future 'okdctl releases latest' verb.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:ab9b764a:export-no-caller` — export no caller

**Status:** done (fix/audit-w12 77fc6b20)
**Severity:** minor
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/setup/ignition.go:64-64` + 59 more
**Problem:** House-wide over-export across all five OKD phase packages: ~60 step methods, helper funcs, and types (e.g. setup.GenerateInstallConfig, install.WaitForBootstrap, postinstall.VerifyClusterHealth/RemoveHAProxy, dns.EnableDnsmasq, cleanup.HAProxy, firewall.Configure/OKDRequiredPorts) are exported but wired only by their own package's steps.go/Execute; same-package tests do not require export.
**Fix:** Decide once as a house convention: unexport phase-internal step methods/helpers, keeping New/NewOptions/Execute/StepDefs plus the externally-called members (setup node-add methods, install.SetupKubeconfig, postinstall.UpdateIngress*, dns.Setup/Deploy*, cleanup.Kind*/Execute, firewall.ConfigureOKD/RemoveOKDRules). cleanup.go's doc claims its helpers as 'public API' — verify that intent before unexporting there.
**Effort:** hours

##### `api:125729c4:phase-surface-divergence` — phase surface divergence

**Status:** done as recorded decision (51698c05) — StepDefs export doesn't land cleanly (ctx-dependent SkipWhen, no consumer); exclusion documented on both Phase docs
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/destroy/phase.go:71-71` + 2 more
**Problem:** The five phase packages share the New/NewOptions/Execute shape, but destroy and cleanup expose no StepDefs (their step catalogs are unexported), so they can never feed the deploy dry-run plan that setup/install/postinstall serve via Provisioner.DeploySteps; Execute signatures also diverge (postinstall prepends *Result, cleanup drops cfg and StepResults).
**Fix:** Either add StepDefs to destroy/cleanup and align Execute to (ctx, cfg, opts) ([]distribution.StepResult, error), or record an explicit decision that destructive phases are excluded from plan listings. Alignment touches cli/destroy.go and cli/cleanup.go call sites.
**Effort:** hours

##### `api:2be6306e:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/addon/registry.go:85-92`
**Problem:** addon.IsRegistered has no caller outside its package; its doc comment self-declares it as kept for a future 'okdctl addon validate' verb and wizard pre-checks. The Get/All/Names siblings are externally used.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:2c4d8e6b:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/addon/addon.go:47-64` + 4 more
**Problem:** ConfigurableAddon is never consumed polymorphically: no external call site invokes DecodeSettings/DefaultSettings/ValidateSettings through the interface (wizard included), and each addon's exported implementations are reached only via unexported typed paths. The interface doc self-declares the symmetry intent.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:2be6306e:export-no-caller` — export no caller

**Status:** done (fix/audit-w12 72ec8899)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/addon/registry.go:16-23`
**Problem:** The Registry type is exported but the only instance is the package-private 'registry' singleton; every external interaction goes through the package-level Register/Get/All/Names funcs, so the type name never crosses the package boundary.
**Fix:** Unexport the type to 'registry' unless a caller-owned Registry value is planned.
**Effort:** hours

##### `api:fde34e0c:export-no-caller` — export no caller

**Status:** deferred — owner decision needed: wire WithEnvFallback (runtime behavior change) or drop documented+tested option; 2026-06-10 audit classified same symbol as kept scaffolding
**Severity:** minor
**Cluster:** exported-surface
**Evidence:** `internal/cluster/k8s.go:63-83`
**Problem:** cluster.WithEnvFallback (the $KUBECONFIG / oc-on-PATH fallback Option) has no caller anywhere while every other With* option is used — the documented fallback behavior is never activated, which reads as missing wiring rather than surplus surface.
**Fix:** Verify intent with the owner: either pass WithEnvFallback() at the cluster.New sites that should honor $KUBECONFIG (likely cli/status.go-adjacent constructors), or drop the option. Wiring it changes runtime kubeconfig resolution, so it needs a deliberate decision, not a mechanical fix.
**Effort:** hours

##### `api:cf43073b:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/config/types.go:25-40`
**Problem:** SupportedDistributions/SupportedProviders are consumed only inside config (validators.go); no CLI or wizard code enumerates them. The symmetric Supported* pair over public enum types is shaped for choice enumeration in a future wizard/help surface.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:aa0f50f5:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/config/validators.go:515-519`
**Problem:** config.IsValidCIDR has no external caller; siblings IsValidIP/IsValidDNSLabel in the same IsValid* predicate family are externally referenced, making this the symmetric completion member.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:c8b28673:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface — seam→security — related: sec:8ea706f6:dl-no-signature
**Evidence:** `internal/download/extract.go:36-40`
**Problem:** download.WithExtractChecksum has no caller: ExtractTarGz is always invoked without a checksum, so archive-integrity verification during extract is never exercised. Symmetric with the fetch-side checksum option; the absence may indicate a missing integrity check at extract sites rather than pure surplus.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:f5eb0ca4:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/infrastructure/proxmox/power.go:160-168`
**Problem:** PowerCycler.VMRunning has zero callers (internal or external); siblings PowerCycleVM/ShutdownVM/StartVM are used. It completes the VM-lifecycle query family.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:1cb08008:export-no-caller` — export no caller

**Status:** done (fix/audit-w12 d94167cc)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/infrastructure/proxmox/probe.go:14-15` + 1 more
**Problem:** DefaultProbeTimeout and DefaultPowerCycleTimeout are exported consts used only as in-package fallbacks; no external package references either default knob.
**Fix:** Unexport both unless callers are meant to reference the defaults when building ProbeOptions/PowerCycleOptions.
**Effort:** hours

##### `api:9c7cfdc5:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/infrastructure/proxmox/hostssh/ssh.go:65-90`
**Problem:** SSHRunOutput/SSHRunArgvOutput (full-capture variants) are called only within hostssh (iso_cleanup, pvesh, snapshot); external packages use only SSHRun/SSHRunArgv. The four-way Run/RunArgv x tail/full matrix is a symmetric helper family whose full-capture half never crosses the package boundary.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:0934cf1b:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/platform/packages.go:127-155`
**Problem:** platform.Manager.AddRepo is a fully implemented ~30-line dnf/apt repo-registration method with zero callers and no test; siblings Install/Remove are used. Heaviest dead body in the exported-surface sweep.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:0934cf1b:export-no-caller` — export no caller

**Status:** done (fix/audit-w12 7e18f059)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/platform/packages.go:109-123`
**Problem:** platform.Manager.IsInstalled is consumed only within platform; no external caller. Part of the Install/Remove/IsInstalled query family whose mutating members are the externally-used half.
**Fix:** Verify intent: keep as Manager query-family completion or unexport.
**Effort:** hours

##### `api:6fc3d91e:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/platform/platform.go:141-147` + 1 more
**Problem:** OS.ApacheConfigPath and OS.HasSELinux have zero callers; the sibling Apache* accessors are consumed by okd/setup. HasSELinux in particular looks like it should gate SELinux-specific setup but is never consulted.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:5f5527e7:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/render/summary.go:71-74`
**Problem:** render.Builder.KVHighlight is called only within render/summary.go; external Builder consumers use KV/Section. Symmetric with KV.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:74f7ee95:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/node/confirm.go:63-65`
**Problem:** node.PreviewFunc is referenced only inside node; its pair ConfirmFunc is the half that crosses the boundary (cli passes it in). Symmetric callback-type pair with only one externally-used member.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:7b99a3dd:export-no-caller` — export no caller

**Status:** done (fix/audit-w12 7e18f059)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/node/resume.go:15-15`
**Problem:** node.OpMatch predicate type is used only within node (add.go, resume.go); nothing external names it.
**Fix:** Unexport to a package-internal predicate type unless resume matching is meant to be caller-extensible.
**Effort:** hours

##### `api:451be4fa:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/system/elevation.go:29-36`
**Problem:** system.InvokingUser is called only by InvokingUserHomeDir in the same package; external sites use the derived helpers (ChownToInvokingUser/WriteAsInvokingUser/InvokingUserHomeDir), never the bare getter.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:bf684414:export-no-caller` — export no caller

**Status:** done (fix/audit-w12 7e18f059)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/infrastructure/terraform/plangate.go:71-92`
**Problem:** terraform.ParsePlanChanges is called only by the same package's ShowPlanChanges (and own tests); the externally-used plangate surface is AssertOnlyChange/EmptyPlanMeansAlreadyAtTarget.
**Fix:** Unexport unless external callers are expected to parse raw plan JSON directly.
**Effort:** hours

##### `api:a59bcd89:export-no-caller` — export no caller

**Status:** done (fix/audit-w12 fa3c7ec6)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/tui/colorprofile.go:36-49`
**Problem:** tui.ColorProfile, ColorEnabled, and Downsample are referenced only within package tui (wizard included in the caller scan); SetColorProfileFor is the only externally-used entry point of the color-profile group.
**Fix:** Unexport the tui-internal color/render helpers; keep SetColorProfileFor.
**Effort:** hours

##### `api:f51f85bb:export-no-caller` — export no caller

**Status:** done (fix/audit-w12 7e18f059)
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/netutil/ip.go:11-13`
**Problem:** netutil.DefaultVIPLastOctet is used only inside netutil (DeriveVIPFromStaticIP); no external reference to the exported const.
**Fix:** Unexport unless callers should reference the VIP-octet default when explaining derived VIPs.
**Effort:** hours

##### `api:4fe429e4:opt-inconsistent` — opt inconsistent

**Status:** done (fix/audit-w12 ee9e7abc)
**Severity:** minor
**Cluster:** option-consistency
**Evidence:** `internal/node/runner.go:208-228` + 3 more
**Problem:** node.NewRunner is the only top-level orchestrator constructor built as a long positional list (8 args, four adjacent bare strings: projectRoot, configPath, tfEnv, runID) plus post-construction public-field mutation, while sibling orchestrators okd.New(...ProvisionerOption) and addon.NewManager(cfg, ...Option) use functional options for the same class of knobs.
**Fix:** Adopt the sibling style: keep required deps (cl, tf, cfg) positional, move projectRoot/configPath/tfEnv/runID/log behind WithProjectRoot/WithConfigPath/WithTerraformEnv/WithRunID/WithLogger, and fold the post-construct sets (Reporter, DryRun at cli/node.go:250-251) into the same option vocabulary okd already uses (WithProgressReporter). Removes the 4-adjacent-string transposition hazard.
**Effort:** hours

##### `api:c287d5c0:opt-inconsistent` — opt inconsistent

**Status:** done as recorded decision (51698c05) — convention documented on Provisioner; bool grandfathered with migration trigger
**Severity:** suggestion
**Cluster:** option-consistency
**Evidence:** `internal/distribution/okd/okd.go:252-260` + 3 more
**Problem:** The Provisioner facade's phase entry points disagree on options shape: Setup takes a facade-owned SetupOpts value, Install/Destroy/UpdateIngress pass phase-package option structs through raw (*install.Options, *destroy.Options, postinstall.UpdateIngressOptions — leaking phase types to cli), and PostInstall takes a bare keepRedHatCatalogs bool.
**Fix:** Pick one facade convention: either facade-owned opts types for every verb (as SetupOpts does) or documented pass-through everywhere. The bare bool on PostInstall is the first knob to migrate — a second flag forces a signature break. Destroy's pass-through is self-documented as deliberate ('no separate CLI-facing options type to keep in sync'), so record the decision rather than churn it.
**Effort:** hours

##### `api:7b2829bb:zero-value-unusable` — zero value unusable

**Status:** done (fix/audit-w12 beddef9f)
**Severity:** minor
**Cluster:** zero-value-usability
**Evidence:** `internal/executor/executor.go:288-295` + 3 more
**Problem:** Systemic pattern: executor.Executor, cluster.Client, terraform.Executor, and platform.Manager all compile on their zero value but nil-deref on first method call (logger and/or exec fields are set only in New), and none of the four type docs states a construct-via-New contract.
**Fix:** Cheapest fix: add a 'must be constructed via New; the zero value is not usable' sentence to each type doc (matching addon.Manager and distribution.PhaseContext, which already carry the contract), or lazy-guard logger via logutil.OrNop at method entry. No signature changes.
**Effort:** hours

##### `api:a4001485:iface-in-producer` — iface in producer

**Status:** rejected — lean-skip: consumer-side inlining out of proportion; leaf package documenting its types' capability is valid
**Severity:** suggestion
**Cluster:** interface-location — seam→errors
**Evidence:** `internal/errtypes/errtypes.go:36-38` + 1 more
**Problem:** HintAppender is defined in errtypes, the package that also implements it (ConfigError/ClusterError.WithHint), while its single consumer is terraform.Executor's lock-hint path — accept-interfaces-return-structs places a one-method contract consumer-side.
**Fix:** Optional: declare the one-method interface inline at the terraform consumer and drop the exported errtypes.HintAppender until a second consumer exists. Counter-argument (valid): errtypes is a leaf package and the interface documents a capability of its own error types — leaving it is defensible; this is a judgment call, not a defect.
**Effort:** hours

##### `api:93957c53:pkg-facade-bypassed` — pkg facade bypassed

**Status:** done (fix/audit-w12 f526b65b)
**Severity:** minor
**Cluster:** package-boundary — seam→state-and-recovery — related: state:93957c53:cleanup-weak-confirm-gate
**Evidence:** `internal/cli/cleanup.go:135-142` + 2 more
**Problem:** cli constructs the cleanup phase directly (cleanup.New(phase.WithExecutor...).Execute) because okd.Provisioner exposes no Cleanup verb, while every other phase verb routes through the facade (cli/destroy.go:340-350 goes deploy.NewProvisioner -> p.Destroy). The facade itself already wires cleanup internally for pre-deploy cleanup (okd.go:174-177), so the construction knowledge now lives in two places.
**Fix:** Add Provisioner.Cleanup(ctx, cfg, opts *cleanup.Options) delegating to cleanup.New with the provisioner's executor/logger (mirroring Destroy), and point cli/cleanup.go at it — or record cleanup as a deliberate phase-level command in CLAUDE.md architecture notes.
**Effort:** hours

##### `api:0139cb3f:pkg-sibling-reach-through` — pkg sibling reach through

**Status:** done (fix/audit-w12 ddf433ab)
**Severity:** minor
**Cluster:** package-boundary
**Evidence:** `internal/distribution/okd/phase/paths.go:94-112` + 7 more
**Problem:** Non-phase packages (7 cli files plus debugbundle) import distribution/okd/phase solely for cluster-layout path helpers (GetTerraformEnv, TerraformEnvDir, WorkDirName, ClusterConfigDir, Default*Path consts, ResolveClusterVIP). paths.go itself documents that WorkDirName/TerraformEnvDir are re-exports from internal/system added because proxmox 'cannot import phase' — the same constraint the cli/debugbundle importers sidestep by pulling in the whole phase package.
**Fix:** Finish the migration paths.go already started: move the layout helpers to their config/system homes (GetTerraformEnv is pure config derivation; ClusterConfigDir/Default* consts are filesystem layout like system.WorkDirName), keep thin re-exports in phase for phase code, and repoint cli/debugbundle at the narrow homes so 'phase' stays a phase-code-only package per CLAUDE.md.
**Effort:** hours

##### `api:db8cf553:pkg-import-cycle-adj` — pkg import cycle adj

**Status:** rejected — lean-skip: DTO move = ~75 LOC cross-package churn for a cycle-adjacent (not broken) shape
**Severity:** suggestion
**Cluster:** package-boundary
**Evidence:** `internal/distribution/okd/clusterstatus/clusterstatus.go:103-132` + 2 more
**Problem:** clusterstatus (child) imports its parent okd for the ClusterStatus/NodeStatus/AddonStatus DTOs it alone produces (okd/types.go), while the parent never imports the child — cli wires Collect by hand. The status DTOs living one level above their sole producer means any future Provisioner.Status verb on the parent closes an import cycle.
**Fix:** Either move the status DTOs into clusterstatus (consumers cli/status.go, cli/addon.go, render already import freely) so the child stops depending on the parent, or accept that status stays a cli-wired verb and note the constraint on okd/types.go. Decide before adding a Provisioner.Status method.
**Effort:** hours

##### `api:4fe429e4:pkg-sibling-reach-through` — pkg sibling reach through

**Status:** rejected — seams are load-bearing for node test fakes; full decoupling requires phase/ move (broad churn)
**Severity:** suggestion
**Cluster:** package-boundary
**Evidence:** `internal/node/runner.go:135-138` + 3 more
**Problem:** node defines consumer-side seams for the setup phase (isoProvisioner/ignitionServer) but the seam methods still take *setup.Options and Runner keeps a SetupOpts *setup.Options field, so the abstraction does not actually decouple node from distribution/okd/setup; node also calls setup.WriteTerraformVars/IgnitionCertPaths/WorkerISOsPlanVar directly.
**Fix:** Either commit to the coupling (drop the two seam interfaces and use *setup.Phase directly — honest and simpler) or complete the decoupling by moving the shared vars/path helpers node needs into phase/ (the CLAUDE.md-sanctioned cross-phase home) and giving the seams setup-free signatures. Verify intent: the half-abstraction may exist only to fake setup in node tests.
**Effort:** hours

#### audit-cli-ux

##### `ux:073d24ed:flag-meaning-diverges` — flag meaning diverges

**Status:** done (fix/audit-w12 9c6e0d58) — BREAKING: --yes now deploys; old behavior = --write-config (release-notes item)
**Severity:** major
**Cluster:** flag-conventions
**Evidence:** `internal/cli/deploy.go:37-50`
**Problem:** --yes means "skip confirmation and perform the operation" on every other command (destroy, cleanup, update-ingress, node *, cluster *, addon uninstall), but on deploy it means "write the config file non-interactively and do NOT deploy". The same allowlisted shorthand carries opposite operational intent across siblings: a scripted `okdctl deploy --yes` in CI writes config and exits 0 without deploying.
**Fix:** Verify intent. If the write-config-only mode stays, move it to a purpose-named flag (e.g. --write-config) and reserve --yes for assume-yes semantics, or record the divergence as settled in CLAUDE.md. Changing semantics now is a breaking change; a deprecation cycle via a new flag plus a warning on the old meaning is the safe path.
**Effort:** hours

##### `ux:aa84670c:panic-exit-aliases-config-code` — panic exit aliases config code

**Status:** done (fix/audit-w12 8ef0aaa8)
**Severity:** major
**Cluster:** exit-codes
**Evidence:** `internal/cli/root.go:91-144`
**Problem:** Execute()/execute() install no top-level recover(). An unrecovered panic exits with the Go runtime's code 2, which the published taxonomy reserves for ConfigError, and it bypasses logFileCloser.Close() and announceFailure — a script branching on 2 ("fix your config") is misled and the run-log tail is lost.
**Fix:** Add a deferred recover() in execute() that re-logs the panic (structured, through RedactHandler), flushes logFileCloser, and returns a dedicated code 70 (EX_SOFTWARE); add code 70 to docs/cli/exit-codes.md. Must not swallow the stack trace — print it to stderr and the file sink.
**Effort:** hours

##### `ux:fd2125dd:exit-code-argcount-drift` — exit code argcount drift

**Status:** done (fix/audit-w12 748f89e3)
**Severity:** major
**Cluster:** exit-codes
**Evidence:** `internal/cli/addon.go:64-75` + 9 more
**Problem:** addon install's hand-rolled Args func returns *errtypes.UsageError for a wrong arg count (exit 64), while every command using cobra.ExactArgs/NoArgs surfaces a plain cobra error (exit 1). docs/cli/exit-codes.md L9 files "arg-count violation" under code 1, so the shipped table is contradicted by addon and both disagree with BSD sysexits (EX_USAGE covers wrong arg count).
**Fix:** Pick one policy: wrap cobra Args errors into UsageError via a shared Args decorator so all arg-count failures exit 64 (sysexits-correct) and move "arg-count violation" from 1 to 64 in docs/cli/exit-codes.md; or drop addon's hand-rolled UsageError and accept 1 everywhere. Behavior and table must agree.
**Effort:** hours

##### `ux:0f076161:flag-help-wording-drift` — flag help wording drift

**Status:** done (fix/audit-w12 a3ff1f2f)
**Severity:** minor
**Cluster:** help-text
**Evidence:** `internal/cli/destroy.go:183-184` + 3 more
**Problem:** The identical --confirm-cluster flag is documented two ways across 13 registrations, and 4 of them leak the internal Go field identifier cfg.Cluster.Name into user-facing --help text; the other 9 use the operator-readable "must equal the config cluster name".
**Fix:** Standardize on "must equal the config cluster name" and hoist the usage string (and ideally the whole --confirm-cluster registration) into a shared helper in flags.go so the flag registers identically at every site.
**Effort:** hours

##### `ux:aa84670c:log-before-configure` — log before configure

**Status:** done (fix/audit-w12 8afad749)
**Severity:** minor
**Cluster:** streams
**Evidence:** `internal/cli/root.go:100-109`
**Problem:** tui.Info("okdctl: started", ...) fires in execute() before PersistentPreRunE runs configureLogging, so it ignores --quiet, prints a text-formatted line under --log-format=json, and misses the piped-stderr auto-switch. The deferred "okdctl: finished" bookend runs after configuration and is correctly suppressed — the asymmetry confirms the ordering bug.
**Fix:** Defer the "started" line to PersistentPreRunE after configureLogging (mirroring the DeferWarn mechanism that already exists for preflight warnings), keeping the started/finished bookend symmetric.
**Effort:** hours

##### `ux:8edf3817:stream-writer-bypass` — stream writer bypass

**Status:** done (fix/audit-w12 a4ba9e63)
**Severity:** minor
**Cluster:** streams
**Evidence:** `internal/cli/node.go:279-297`
**Problem:** The node dry-run preview writes to os.Stdout and the confirm box to os.Stderr directly instead of cmd.OutOrStdout()/cmd.ErrOrStderr() used by every other RunE. Stream choice is correct (result->stdout, chrome->stderr, deliberately non-interleaving); only the writer plumbing bypasses cobra, hurting testability and consistency.
**Fix:** Thread cmd.OutOrStdout()/cmd.ErrOrStderr() into buildNodeRunner's Preview/Confirm hooks instead of referencing package-level os streams.
**Effort:** hours

##### `ux:073d24ed:exit-precedence-hides-network` — exit precedence hides network

**Status:** done (fix/audit-w12 4d160ee0)
**Severity:** minor
**Cluster:** exit-codes — seam→errors
**Evidence:** `internal/cli/deploy.go:181-183` + 1 more
**Problem:** deploy --dry-run and destroy --dry-run wrap every plan-preview failure in an outer ConfigError, including Proxmox-unreachable NetworkErrors from tf.Init/connect. Because exitCodeFor's precedence is Config(2) > Network(3) over the whole chain, dry-run transport failures exit 2, making the exit-codes.md example's "3) network unreachable" branch unreachable for these paths.
**Fix:** Taxonomy side: document in docs/cli/exit-codes.md that dry-run transport/connect failures surface as 2 (Config-wrapped). Defer the "should dry-run wrap in ConfigError at all" reclassification to audit-errors.
**Effort:** hours

##### `ux:1013f4e8:exit-code-doc-drift` — exit code doc drift

**Status:** done (fix/audit-w12 4d160ee0)
**Severity:** minor
**Cluster:** exit-codes
**Evidence:** `docs/cli/exit-codes.md:42-47`
**Problem:** The published precedence prose ("sentinels 65/66/71 outrank every category, then Config > Network > Cluster > Auth > Usage") never places errDoctorWarn(6) and errPlanDrift(7), which exitCodeFor checks after the sentinels and before the category errors.As checks — the published precedence is incomplete versus the code.
**Fix:** Extend the precedence sentence to: sentinels (65/66/71) > doctor-warn (6) / plan-drift (7) > Config(2) > Network(3) > Cluster(4) > Auth(5) > Usage(64).
**Effort:** hours

##### `ux:26a430ee:exit-code-doc-drift` — exit code doc drift

**Status:** done (fix/audit-w12 4d160ee0)
**Severity:** minor
**Cluster:** exit-codes
**Evidence:** `internal/cli/elevation.go:114-122`
**Problem:** ensureRoot re-execs via syscall.Exec(sudo, ...), replacing the process image; if the operator fails sudo's password prompt the shell observes sudo's own exit code (typically 1), not okdctl's 5/71. docs/cli/exit-codes.md frames privilege problems as 5|71 without mentioning this gap.
**Fix:** Add a note to docs/cli/exit-codes.md: after self-elevation begins, the shell observes the sudo/child exit code; a failed sudo password yields sudo's code (1), not 5. Do not pre-check auth — that would break the re-exec design.
**Effort:** hours

##### `ux:1013f4e8:exit-refinement-category-ambiguous` — exit refinement category ambiguous

**Status:** done (fix/audit-w12 4d160ee0)
**Severity:** minor
**Cluster:** exit-codes
**Evidence:** `docs/cli/exit-codes.md:23-25`
**Problem:** The doc says 65/66/71 are "refinements within the broader categories 2 (config) and 5 (auth)" without saying which refines which, and the example groups 65 with "fix your config". In code ErrPullSecretInvalid(65) is wrapped in an AuthError (category 5), not ConfigError. Exit code is unaffected (sentinels checked first) but the prose is ambiguous.
**Fix:** State explicitly: 66 refines Config(2); 65 and 71 refine Auth(5). Optionally regroup the example's case arms accordingly.
**Effort:** hours

##### `ux:4583b75b:help-long-missing` — help long missing

**Status:** done (fix/audit-w12 7c48c3c6)
**Severity:** minor
**Cluster:** help-text
**Evidence:** `internal/cli/config.go:36-41`
**Problem:** config validate has Short + Example + RunE but no Long, while its sibling config show (same file) and every other non-trivial leaf command carry one.
**Fix:** Add a Long describing what validate checks, that it is read-only, and that it exits 2 on an invalid config.
**Effort:** hours

##### `ux:8edf3817:concept-named-twice` — concept named twice

**Status:** not started
**Severity:** suggestion
**Cluster:** verb-noun
**Evidence:** `internal/cli/node.go:104-147`
**Problem:** Node-scope selection is expressed three ways inside the node group: `node add --role worker` (flag, singular), `node resize masters|workers|<name>` (positional, plural), `node remove <name>` (positional instance); `destroy --only vms|workers|masters|bootstrap` adds a fourth vocabulary. --role only accepts "worker" today and is shaped for future master-add support.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `ux:577afcf0:help-short-not-imperative` — help short not imperative

**Status:** done (fix/audit-w12 2abbee39)
**Severity:** suggestion
**Cluster:** help-text
**Evidence:** `internal/cli/cluster.go:34-34` + 1 more
**Problem:** Group-command Short strings are mostly imperative ("Manage...", "Inspect...", "Show...") but two are noun phrases ("Cluster-wide lifecycle operations", "Manual, single-node Proxmox VM snapshots"), giving inconsistent voice across peer group commands.
**Fix:** Reword to imperative voice, e.g. "Manage cluster-wide lifecycle" and "Take and manage single-node Proxmox snapshots".
**Effort:** hours

##### `ux:577afcf0:concept-named-twice` — concept named twice

**Status:** done (fix/audit-w12 2abbee39)
**Severity:** suggestion
**Cluster:** flag-conventions
**Evidence:** `internal/cli/cluster.go:104-104` + 1 more
**Problem:** Master memory target is --memory-mb on node resize but --grow-master-memory-mb on cluster compact. Borderline: compact's flag adds grow-only, interleaved semantics, so the longer name is partly justified.
**Fix:** Verify intent; if unification is wanted, --master-memory-mb reads closer to its sibling while keeping the grow-only note in the usage string.
**Effort:** hours

##### `ux:8154ab0f:exit-code-undefined` — exit code undefined

**Status:** done (fix/audit-w12 86f7da97)
**Severity:** suggestion
**Cluster:** exit-codes
**Evidence:** `internal/cli/doctor.go:58-61`
**Problem:** doctor's runtime OS gate returns a bare fmt.Errorf (exit 1) where the conceptual class is a usage/environment error. Dead on the supported Linux target — the binary only ships linux/amd64+arm64 — so effectively a dev-host-only path.
**Fix:** Wrap in &errtypes.UsageError{...} (exit 64) for consistency with other invalid-invocation gates, or leave as a documented dev-only path.
**Effort:** hours

##### `ux:ed572b03:json-redact-map-gap` — json redact map gap

**Status:** done (fix/audit-w12 8ea5a214)
**Severity:** suggestion
**Cluster:** json-stability — seam→security — related: sec:41a9d4eb:keyset-incomplete
**Evidence:** `internal/config/redact.go:21-58`
**Problem:** redactValue recurses only into Pointer and Struct kinds — Map and Slice fall through. Config.Addons is map[string]AddonConfig and AddonConfig.Settings is map[string]string, so a secret-keyed addon setting would escape the "***" masking that docs/cli/json-schema.md promises for `config show --output=json` and that the debug-bundle relies on. Latent today: the only shipped Settings consumer is a directory path and managed creds are json:"-".
**Fix:** Add reflect.Map (and reflect.Slice) arms to redactValue that mask string values whose map key or field tag hits logutil.KeyIsSecret. Do not change the emitted JSON shape — the schema is a shipped contract.
**Effort:** hours

#### audit-observability

##### `obs:8ea706f6:level-info-should-debug` — level info should debug

**Status:** not started
**Severity:** minor
**Cluster:** level-discipline
**Evidence:** `internal/distribution/okd/setup/tools.go:119-122`
**Problem:** The 'already installed' no-op status is logged at Info per tool, while the identical concept for system packages is logged at Debug in the same phase (steps.go L424 'packages: already installed'). Re-runs emit one Info line per tool for zero state change.
**Fix:** Demote to p.Log.Debug to match the packages precedent at steps.go L422-L425; keep Info only for actual installs.
**Effort:** hours

##### `obs:29293401:level-info-should-debug` — level info should debug

**Status:** not started
**Severity:** minor
**Cluster:** level-discipline
**Evidence:** `internal/distribution/okd/setup/haproxy.go:196-203`
**Problem:** Four per-port 'haproxy: listening' Info lines confirm the expected state; the milestone already exists at L147 and the interesting case (not listening) is the Warn branch. Per-port success detail is Debug-grade.
**Fix:** Demote the success branch to Debug (or emit one Info summarizing all listening ports); keep the per-port Warn.
**Effort:** hours

##### `obs:06f00bcb:key-inconsistent-casing` — key inconsistent casing

**Status:** not started
**Severity:** minor
**Cluster:** field-stability
**Evidence:** `internal/distribution/okd/setup/apache.go:108-108`
**Problem:** The just-written config file path is keyed 'conf' here, while every other written-file site in the phase uses 'path' (apache.go L149, steps.go L351, steps.go L505). One-off synonym breaks path-keyed log queries.
**Fix:** Rename the attr key from 'conf' to 'path'.
**Effort:** hours

##### `obs:9d79b841:key-inconsistent-casing` — key inconsistent casing

**Status:** not started
**Severity:** minor
**Cluster:** field-stability
**Evidence:** `internal/distribution/okd/setup/coreos.go:30-40` + 1 more
**Problem:** ISO base filenames are keyed 'iso' in coreos.go but 'file' in upload.go for the same artifacts flowing through the same pipeline; a query for one ISO's lifecycle needs two keys.
**Fix:** Pick one key ('file' matches release_extract.go L165) for base-filename attrs across coreos.go and upload.go.
**Effort:** hours

##### `obs:f5d703ab:key-inconsistent-casing` — key inconsistent casing

**Status:** not started
**Severity:** suggestion
**Cluster:** field-stability
**Evidence:** `internal/distribution/okd/setup/artifacts.go:69-69` + 1 more
**Problem:** Installed-executable name is keyed 'binary' here but 'tool' in tools.go (L120/L208/L261) for near-identical 'installed' messages. Concepts arguably differ (release binaries vs external tools), so borderline.
**Fix:** Standardize on one key (e.g. 'tool') for the name of an installed executable across both files.
**Effort:** hours

##### `obs:ae5b624c:key-inconsistent-casing` — key inconsistent casing

**Status:** not started
**Severity:** minor
**Cluster:** field-stability
**Evidence:** `internal/distribution/okd/install/monitor.go:162-162` + 1 more
**Problem:** The same running total of approved CSRs is keyed 'csrs_approved' at completion but 'total' in the per-tick log 18 lines later; one value, two names within one function.
**Fix:** Use one key ('csrs_approved' or 'total') for totalApproved in both records.
**Effort:** hours

##### `obs:98723e5d:value-embeds-label` — value embeds label

**Status:** not started
**Severity:** suggestion
**Cluster:** field-stability
**Evidence:** `internal/distribution/okd/install/flux.go:39-44`
**Problem:** The 'version' attr value is the whole lowered oc output line including its own label - it logs as version="server version: 4.x.y", duplicating the key inside the value and diverging from bare-version values under the same key elsewhere (coreos.go L387, tools.go L192).
**Fix:** Strip the 'Server Version:' prefix (strings.TrimPrefix + TrimSpace) so the attr carries only the version string.
**Effort:** hours

##### `obs:06f00bcb:warn-drops-err` — warn drops err

**Status:** not started
**Severity:** suggestion
**Cluster:** field-stability
**Evidence:** `internal/distribution/okd/setup/apache.go:69-73` + 1 more
**Problem:** The dial error is discarded before the Warn, so the log cannot distinguish connection-refused from timeout from a bad bindIP; every other Warn in the file carries 'err'. Same shape at coreos.go L342-L344 where the checksum-mismatch Warn drops both the error and the file path.
**Fix:** Add "err", err (and "addr", addr) to the Warn; at coreos.go L342-L344 add "err", err and "path", destPath.
**Effort:** hours

##### `obs:9d79b841:msg-stale-content` — msg stale content

**Status:** not started
**Severity:** suggestion
**Cluster:** msg-hygiene
**Evidence:** `internal/distribution/okd/setup/coreos.go:376-376`
**Problem:** Message says version detection comes 'from openshift-install', but DetectCoreOSVersion actually fetches the SHA-pinned stream JSON from raw.githubusercontent.com - the message misleads an operator debugging a network failure at this step.
**Fix:** Reword to match reality, e.g. 'coreos: resolving iso from pinned installer stream metadata'.
**Effort:** hours

##### `obs:5013fea6:span-no-start-end` — span no start end

**Status:** not started
**Severity:** suggestion
**Cluster:** span-retry-boundary
**Evidence:** `internal/distribution/okd/setup/release_extract.go:114-143`
**Problem:** The 4-8 minute (10-min-capped) 'oc adm release extract' has a start log but no explicit success/elapsed record; completion is only implied by per-tarball Info lines from extractReleaseTarballs, which can be zero-match if the glob misses.
**Fix:** After the ExitCode==0 check, log one Info (e.g. 'tools: release extract completed', 'ref', ref, 'elapsed', time.Since(start)) to close the span deterministically.
**Effort:** hours

##### `obs:830d4653:warn-drops-err` — warn drops err

**Status:** not started
**Severity:** minor
**Cluster:** field-stability
**Evidence:** `internal/distribution/okd/cleanup/packages.go:60-63`
**Problem:** pm.Remove's error is dropped: the Warn logs a fixed message with no 'err' attr, so the dnf/stderr failure detail is unrecoverable from the log; sibling removePackage (services.go L29) does log 'err', err.
**Fix:** Add "err", err to the Warn call to match removePackage in services.go L29.
**Effort:** hours

##### `obs:ed55ee90:key-inconsistent-casing` — key inconsistent casing

**Status:** not started
**Severity:** suggestion
**Cluster:** field-stability
**Evidence:** `internal/distribution/okd/cleanup/summary.go:88-89` + 1 more
**Problem:** Failed-step names are logged as a comma-joined string under 'subsystems', while destroy's equivalent summary logs a slice under 'failed_steps' - the two teardown summaries are not machine-comparable.
**Fix:** Pass the []string directly under a shared key (e.g. 'failed_steps') so both teardown summaries use one encoding.
**Effort:** hours

##### `obs:15ba17da:msg-stale-content` — msg stale content

**Status:** not started
**Severity:** minor
**Cluster:** msg-hygiene
**Evidence:** `internal/distribution/okd/destroy/steps.go:165-165`
**Problem:** Success message hardcodes 'firewalld' but fw.DetectBackend may have selected ufw or iptables; the log claims a backend that may not be the one acted on.
**Fix:** Say 'firewall: okd rules removed' or carry the backend as an attr (firewall.Configure already logs 'backend').
**Effort:** hours

##### `obs:de572c63:completion-log-unconditional` — completion log unconditional

**Status:** not started
**Severity:** minor
**Cluster:** span-retry-boundary
**Evidence:** `internal/distribution/okd/dns/dnsmasq.go:254-262` + 1 more
**Problem:** Unconditional 'restored' success Info after failures were only warned - the completion log can directly contradict the two Warn lines above it. Same shape at L266-L275 where 'systemd-resolved configuration restored' logs even when removeAllFn failed.
**Fix:** Gate the success line on both nmcli calls succeeding; best-effort semantics can stay, but do not claim 'restored' after a logged failure.
**Effort:** hours

##### `obs:632c9087:key-inconsistent-casing` — key inconsistent casing

**Status:** not started
**Severity:** suggestion
**Cluster:** field-stability
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:112-113` + 4 more
**Problem:** The 'controllers' key carries a []string via slog.Any at L112-113 but a comma-joined string at L314-315 - same key, two value types. Also in this file: backup path keyed 'backup' at L646 while L639/L643 use 'path', and the VIP keyed 'api' at L178/L262 while every other site uses 'vip'.
**Fix:** Pick one encoding (slice) for 'controllers' at both sites; unify 'backup'->'path' and 'api'->'vip'.
**Effort:** hours

##### `obs:1e8ffb91:msg-prefix-drift` — msg prefix drift

**Status:** not started
**Severity:** suggestion
**Cluster:** msg-hygiene
**Evidence:** `internal/distribution/okd/postinstall/verify.go:233-272`
**Problem:** One flow, two message prefixes: verifyKubeVIPAPIHealthBootstrap logs 'verify:' at L233/L259 and 'kubevip:' at L272 for the same subsystem, splitting grep-ability of the kube-vip verification path.
**Fix:** Standardise on the 'kubevip:' prefix used by every other log line in the kube-vip verification path.
**Effort:** hours

##### `obs:fdccb07e:err-stringified` — err stringified

**Status:** not started
**Severity:** minor
**Cluster:** field-stability — seam→errors
**Evidence:** `internal/node/compact.go:369-383`
**Problem:** reportEtcdDryRunVerdict logs the EtcdHealthy error as "reason", err.Error() - a pre-rendered string under a non-err key. Every other error in the package uses the structured form ("err", err); stringifying defeats any future type-level Redacted() dispatch. The message also hardcodes 'wait up to 10m' while the actual bound is the configurable r.EtcdGateTimeout.
**Fix:** Log the error branch as "err", err; keep "reason" for the plain h.Reason string branch. Interpolate r.EtcdGateTimeout into the message or drop the hardcoded number.
**Effort:** hours

##### `obs:02fe484f:key-inconsistent-casing` — key inconsistent casing

**Status:** not started
**Severity:** suggestion
**Cluster:** field-stability
**Evidence:** `internal/node/stop.go:127-134`
**Problem:** The same signer NotAfter date is logged under 'expired' in the already-expired branch but 'expires' in the other two branches; a consumer filtering on 'expires' misses the most important branch.
**Fix:** Use 'expires' on all three branches (optionally add 'days_remaining' as a negative number to the expired branch).
**Effort:** hours

##### `obs:fdccb07e:key-inconsistent-casing` — key inconsistent casing

**Status:** not started
**Severity:** suggestion
**Cluster:** field-stability
**Evidence:** `internal/node/compact.go:427-427`
**Problem:** The 'role' attr is passed as raw nodetypes.NodeRole while every other site in the package passes string(role); the rendered output is identical today, but a future NodeRole method (LogValuer/Stringer) would silently fork the two encodings.
**Fix:** Pass string(role) at compact.go L427 to match resize.go L116/L129/L145/L278/L357.
**Effort:** hours

##### `obs:65a14362:msg-text-drift` — msg text drift

**Status:** not started
**Severity:** suggestion
**Cluster:** msg-hygiene
**Evidence:** `internal/node/resize.go:98-99` + 2 more
**Problem:** The identical no-probe budget warning uses '(no Proxmox probe)' here but '(no proxmox probe)' in the two sibling sites (compact.go L258, add.go L154) - same-concept message text drift breaks grep-ability across the three ops.
**Fix:** Align all three sites on one casing (repo trend is lowercase 'proxmox' in log messages).
**Effort:** hours

##### `obs:fdccb07e:span-no-start-end` — span no start end

**Status:** not started
**Severity:** suggestion
**Cluster:** span-retry-boundary
**Evidence:** `internal/node/compact.go:85-174` + 7 more
**Problem:** Long mutating node ops emit a completion log only, never a started log; with the default logutil.NopProgressReporter wiring, a crash mid-op leaves zero slog evidence the op began. Same finish-only shape across add/remove/resize/snapshot/stop/start.
**Fix:** Emit one Info at op start (after confirm, before first mutation) per mutating op - e.g. 'node: compact starting', 'workers', N - mirroring system.WaitFor's waiting/ready pair. On-disk op markers and the interactive reporter mitigate.
**Effort:** hours

##### `obs:881d089e:default-logger-undocumented` — default logger undocumented

**Status:** not started
**Severity:** suggestion
**Cluster:** handler-setup
**Evidence:** `internal/runlock/runlock.go:83-91`
**Problem:** Logs via package-global slog.Warn (slog.Default()) with no injected logger and no documented caveat that the caller must have installed RedactHandler; updatecheck.go sets the precedent for slog.Default() use but documents it explicitly (L42-44). Values here are benign (hostname/chown errors, lock path) - a convention gap, not a leak.
**Fix:** Either accept a *slog.Logger in Acquire (defaulting via logutil.OrNop) or add the updatecheck-style doc caveat.
**Effort:** hours

##### `obs:91935445:key-inconsistent-casing` — key inconsistent casing

**Status:** not started
**Severity:** minor
**Cluster:** field-stability
**Evidence:** `internal/debugbundle/debugbundle.go:101-101` + 3 more
**Problem:** The bundle output path is logged under key 'output' at operation start and key 'path' at completion of the same operation; additionally, key 'files' is an int count at cli/deploy.go L75 but a comma-joined string list at debugbundle.go L325.
**Fix:** Use 'path' for the bundle path at both lines (matches kubeconfig.go L78, deploy.go L267); pick one type for 'files'.
**Effort:** hours

##### `obs:eff8657e:value-format-drift` — value format drift

**Status:** not started
**Severity:** minor
**Cluster:** field-stability
**Evidence:** `internal/deploy/state.go:183-187` + 1 more
**Problem:** The 'marker_age' key carries three different value formats across sites - a rounded Duration string, an 'N days' string, and an 'N days — likely stale' string with prose embedded in the value.
**Fix:** Standardize marker_age on one duration rendering; move 'likely stale' into the message or a separate boolean field (tui.LF("stale", true)).
**Effort:** hours

##### `obs:08c49fc4:key-inconsistent-casing` — key inconsistent casing

**Status:** not started
**Severity:** minor
**Cluster:** field-stability
**Evidence:** `internal/cli/update_ingress.go:60-61` + 4 more
**Problem:** The 'cluster' key carries the bare cluster name in some sites (destroy.go L296, cleanup.go L94, addon.go L186) and the FQDN name.domain in others (update_ingress.go L61, deploy/deploy.go L323), so log queries on cluster=X match inconsistently. Related: 'node' is a Proxmox host node at cli/node.go L351 but a k8s node at node_snapshot.go L251.
**Fix:** Standardize 'cluster' on the bare name and add a separate 'domain' or 'fqdn' field where the FQDN matters.
**Effort:** hours

##### `obs:e45c2239:err-stringified` — err stringified

**Status:** not started
**Severity:** minor
**Cluster:** field-stability — seam→errors
**Evidence:** `cmd/okdctl/main.go:36-40`
**Problem:** The error is pre-stringified via err.Error() and string-prefixed before being passed under the 'err' key, defeating RedactHandler's value inspection and Redacted() dispatch for future error types. ValidateBinDir errors cannot carry credentials today, so hygiene rather than a leak.
**Fix:** Pass tui.LF("err", err) and carry the tilde note as a separate field, e.g. tui.LF("note", "tilde expansion failed (home dir unresolved)").
**Effort:** hours

##### `obs:91935445:redaction-helper-bypassed` — redaction helper bypassed

**Status:** not started
**Severity:** minor
**Cluster:** redaction-sink — seam→security
**Evidence:** `internal/debugbundle/debugbundle.go:212-222` + 9 more
**Problem:** safeMessage() exists precisely to run Redacted() dispatch on errors before they enter the operator-shared bundle manifest, but most manifest sites bypass it with fmt.Sprintf("...: %v", err) - a future credential-bearing error type would land unredacted in a tarball documented as safe to attach to a support ticket. The highest-risk values (terraform/oc stderr) are already scrubbed via logutil.RedactableStderr at L272/L314.
**Fix:** Route every manifestEntry.Message error through safeMessage, wrapping the static prefix around it, matching the sites that already do (L222, L249, L279).
**Effort:** hours

##### `obs:6424733c:level-error-duplicates-return` — level error duplicates return

**Status:** not started
**Severity:** minor
**Cluster:** level-discipline — related: ux:aa84670c:log-before-configure
**Evidence:** `internal/cli/helpers.go:26-33`
**Problem:** The only Error-level log site in the CLI command surface logs 'configuration file not found' and then immediately returns a ConfigError carrying the identical message, so the root handler presents the same failure twice; every other command returns the typed error and lets the root present it once.
**Fix:** Drop the tui.Error line (keep the Info hint lines); the returned ConfigError already carries the file path and is user-visible via the root error box.
**Effort:** hours

##### `obs:6424733c:msg-multiline-split` — msg multiline split

**Status:** not started
**Severity:** suggestion
**Cluster:** msg-hygiene
**Evidence:** `internal/cli/helpers.go:52-54`
**Problem:** Three consecutive Info events fake a multi-line help block; the continuation lines start with literal indent spaces and carry no fields, so in okdctl.log and any structured consumer they appear as standalone events whose messages begin with whitespace.
**Fix:** Collapse into one event: tui.Info("set credentials via environment variables or env file", tui.LF("path", envPath), tui.LF("vars", "PROXMOX_VE_USERNAME+PROXMOX_VE_PASSWORD or PROXMOX_VE_API_TOKEN")).
**Effort:** hours

##### `obs:08c49fc4:msg-trailing-period` — msg trailing period

**Status:** not started
**Severity:** suggestion
**Cluster:** msg-hygiene
**Evidence:** `internal/cli/update_ingress.go:88-89`
**Problem:** L89's message ends with a trailing period - the only log message in the audited set that does - and the L88/L89 pair is one logical warning split into two events.
**Fix:** Drop the trailing period and fold the outage note into the first Warn (message or an 'outage' field).
**Effort:** hours

##### `obs:f327eaf4:default-logger-undocumented` — default logger undocumented

**Status:** not started
**Severity:** suggestion
**Cluster:** handler-setup — related: obs:881d089e:default-logger-undocumented
**Evidence:** `internal/deploy/migrate.go:90-94`
**Problem:** Sole direct slog.Warn in the CLI/deploy surface (every sibling site uses tui.Warn). It is redaction-safe only because cli/root.go L92 rebinds slog's default to tui.SimpleLogger before commands run; a future library-context caller invoked before Execute would hit the stock text handler and skip the file sink. Values are a path and two ints - no reachable leak.
**Fix:** Use tui.Warn with tui.LF fields (and key 'expected' to match state.go L100, which currently drifts vs 'supported' here); migrate_test.go L151 swaps the slog default to capture this line and needs adjusting.
**Effort:** hours

##### `obs:fd2125dd:span-no-start-end` — span no start end

**Status:** not started
**Severity:** suggestion
**Cluster:** span-retry-boundary
**Evidence:** `internal/cli/addon.go:164-210`
**Problem:** Addon uninstall logs a completion event ('addon uninstalled', L210) but addon install logs neither start nor completion at the CLI layer - asymmetric span coverage for sibling verbs; deploy/destroy/cleanup/update-ingress all have started/finished pairs. addon.Manager logs its own per-addon installs, which may be judged sufficient.
**Fix:** Add a matching 'addon installed' Info after successful InstallOne/InstallAll (verify addon.Manager's own logging first).
**Effort:** hours

##### `obs:7b2829bb:key-inconsistent-casing` — key inconsistent casing

**Status:** not started
**Severity:** suggestion
**Cluster:** field-stability
**Evidence:** `internal/executor/executor.go:352-352`
**Problem:** RunStreamed's 'exec: completed' Debug omits the 'exit' key that every other Run* variant includes (run L317, RunInteractive L437, RunDiscard L515, runOutput L625), because it logs before the exit code is extracted - the one streaming variant is not queryable by exit code.
**Fix:** Move the completed log after the exitErr extraction (or compute exitCodeOf(err) inline) and add "exit", result.ExitCode to match the sibling variants.
**Effort:** hours

##### `obs:7f696ed8:key-inconsistent-casing` — key inconsistent casing

**Status:** not started
**Severity:** minor
**Cluster:** field-stability
**Evidence:** `internal/infrastructure/terraform/state.go:29-38` + 2 more
**Problem:** The same terraform.tfstate path variable is keyed 'file' in checkStateMajorVersion (state.go L30/L37) but 'path' in StateStatus (terraform.go L423/L431) - identical concept, identical variable, two keys within one package.
**Fix:** Rename state.go's 'file' attrs to 'path' to match the four 'path' sites in terraform.go.
**Effort:** hours

##### `obs:21dc1103:key-inconsistent-casing` — key inconsistent casing

**Status:** not started
**Severity:** suggestion
**Cluster:** field-stability — related: err:4ded56d3:err-formats-cred
**Evidence:** `internal/download/download.go:127-135`
**Problem:** The start-of-download Info uses the bare message 'download' (every sibling in the package uses the 'download: <action>' shape) and keys the artifact 'file', while the giving-up Warn eight lines later keys the same artifact 'desc' - the span's start and end records are not joinable on one key.
**Fix:** Rename the message to 'download: fetching' and use 'file', filename on the giving-up Warn (description defaults to the filename anyway).
**Effort:** hours

##### `obs:40d315ad:msg-dynamic` — msg dynamic

**Status:** not started
**Severity:** suggestion
**Cluster:** msg-hygiene
**Evidence:** `internal/addon/catalog/flux/flux.go:262-262`
**Problem:** The Warn message is built by concatenating a variable step description ('flux: '+desc), so the message text is not a stable constant - each uninstall step produces a differently-named event, and the variable part belongs in an attr.
**Fix:** Use a constant message with the step as a field: env.Logger.Warn("flux: uninstall step failed", "step", desc, "exit", res.ExitCode, "err", err).
**Effort:** hours

#### audit-modernization

##### `mod:65a14362:use-slices-sort` — use slices sort

**Status:** not started
**Severity:** minor
**Cluster:** slices-maps
**Evidence:** `internal/node/resize.go:221-221`
**Problem:** sort.Slice with an index-based less closure where slices.SortFunc + cmp.Compare on the element type is the Go 1.21 idiom; single-key int comparator is the textbook shape.
**Fix:** slices.SortFunc(targets, func(a, b resizeTarget) int { return cmp.Compare(a.index, b.index) }); swap import sort -> slices, add cmp.
**Effort:** hours

##### `mod:c5e5c304:use-slices-sort` — use slices sort

**Status:** not started
**Severity:** suggestion
**Cluster:** slices-maps
**Evidence:** `internal/distribution/okd/setup/terraform.go:261-261`
**Problem:** sort.Strings on a []string where slices.Sort is the Go 1.21 idiom; last sort holdout in the setup package, whose siblings already import slices.
**Fix:** slices.Sort(missing); swap the file's sort import for slices.
**Effort:** hours

##### `mod:46a2d36e:use-slices-sort` — use slices sort

**Status:** not started
**Severity:** suggestion
**Cluster:** slices-maps
**Evidence:** `internal/node/guards.go:78-78`
**Problem:** sort.Strings on a []string of pod names where slices.Sort is the Go 1.21 idiom.
**Fix:** slices.Sort(names); swap import sort -> slices (file gains slices for the range-int and max findings too).
**Effort:** hours

##### `mod:fdccb07e:use-slices-sort` — use slices sort

**Status:** not started
**Severity:** suggestion
**Cluster:** slices-maps
**Evidence:** `internal/node/compact.go:432-437`
**Problem:** sort.Slice with a direction-toggled index closure; slices.SortFunc with cmp.Compare(a,b)/cmp.Compare(b,a) per branch removes the index-into-slice comparisons.
**Fix:** slices.SortFunc(items, func(a, b ni) int { if ascending { return cmp.Compare(a.idx, b.idx) }; return cmp.Compare(b.idx, a.idx) }) - descending branch must swap args, not negate.
**Effort:** hours

##### `mod:4fe429e4:use-maps-copy` — use maps copy

**Status:** not started
**Severity:** minor
**Cluster:** slices-maps
**Evidence:** `internal/node/runner.go:274-276`
**Problem:** Hand-rolled map-copy loop with overwrite-on-collision semantics that maps.Copy (Go 1.21) encodes exactly; the doc comment even states 'planVars override these on a key collision'.
**Fix:** maps.Copy(vars, planVars); add maps import.
**Effort:** hours

##### `mod:2465a34a:use-slices-contains` — use slices contains

**Status:** not started
**Severity:** minor
**Cluster:** slices-maps
**Evidence:** `internal/cluster/k8s_etcd.go:170-175`
**Problem:** Hand-rolled any-match loop over conditions with a pure boolean predicate; canonical slices.ContainsFunc shape.
**Fix:** return slices.ContainsFunc(e.Status.Conditions, func(cond k8sCondition) bool { return cond.Type == "NodeInstallerProgressing" && cond.Status == string(nodetypes.ConditionStatusTrue) }), nil - first slices use in package cluster.
**Effort:** hours

##### `mod:7f03549f:use-slices-contains` — use slices contains

**Status:** not started
**Severity:** minor
**Cluster:** slices-maps
**Evidence:** `internal/doctor/doctor.go:222-228`
**Problem:** Flag-and-break any-match loop over candidate binary names; slices.ContainsFunc gives the same short-circuit without the mutable flag.
**Fix:** apacheFound := slices.ContainsFunc([]string{"httpd", "apache2"}, func(bin string) bool { _, err := exec.LookPath(bin); return err == nil })
**Effort:** hours

##### `mod:588ce79e:use-slices-contains` — use slices contains

**Status:** not started
**Severity:** suggestion
**Cluster:** slices-maps
**Evidence:** `internal/tui/colors.go:94-99`
**Problem:** Any-match loop over env-var names; convertible to slices.ContainsFunc, though the predicate performs an env read so the gain is stylistic.
**Fix:** return slices.ContainsFunc([]string{...}, func(name string) bool { v := os.Getenv(name); return v == "1" || v == "true" })
**Effort:** hours

##### `mod:6424733c:use-slices-contains` — use slices contains

**Status:** not started
**Severity:** suggestion
**Cluster:** slices-maps
**Evidence:** `internal/cli/helpers.go:205-211`
**Problem:** Any-match loop over marker filenames with an os.Stat predicate; convertible to slices.ContainsFunc, predicate-with-syscall makes it a style call.
**Fix:** return slices.ContainsFunc([]string{filepath.Base(cfgFile), "okdctl.env"}, func(name string) bool { _, err := os.Stat(filepath.Join(root, name)); return err == nil })
**Effort:** hours

##### `mod:f4ab7641:use-fmt-errorf` — use fmt errorf

**Status:** not started
**Severity:** suggestion
**Cluster:** errors-and-deprecated-stdlib
**Evidence:** `internal/doctor/statfs_stub.go:12-14`
**Problem:** errors.New with string concatenation instead of fmt.Errorf; pure idiom, and the surrounding comment notes the path is unreachable at runtime.
**Fix:** return 0, fmt.Errorf("disk space probe unsupported on %s", runtime.GOOS); swap errors import for fmt.
**Effort:** hours

##### `mod:ca06979c:use-errorf-w` — use errorf w

**Status:** not started
**Severity:** minor
**Cluster:** errors-and-deprecated-stdlib — seam→errors — related: err:366b3f2d:wrap-cause-dropped-at-sink
**Evidence:** `internal/node/add.go:145-145` + 5 more
**Problem:** errtypes.ConfigError{Msg: err.Error()} stringifies the validator error and drops the Unwrap chain; six sites across node add/remove/compact/resize lose errors.Is/As reachability to the inner error.
**Fix:** Preserve the chain per the existing pattern at internal/cli/root.go:386 (UsageError{Msg: err.Error(), Err: err}): add Err: err at each of the six sites; errtypes never string-interpolates Err so redaction is unaffected.
**Effort:** hours

##### `mod:af5b4ad0:use-range-int` — use range int

**Status:** not started
**Severity:** minor
**Cluster:** range-idioms
**Evidence:** `internal/tui/table.go:55-55`
**Problem:** Counted loop over a plain int bound where Go 1.22 range-over-int applies; the sibling loop at line 44 has a compound condition and legitimately stays.
**Fix:** for c := range cols {
**Effort:** hours

##### `mod:46a2d36e:use-range-int` — use range int

**Status:** not started
**Severity:** minor
**Cluster:** range-idioms
**Evidence:** `internal/node/guards.go:111-111`
**Problem:** Counted loop whose index is never referenced in the body; for range numWorkers is the Go 1.22 idiom.
**Fix:** for range numWorkers {
**Effort:** hours

##### `mod:46a2d36e:use-builtins` — use builtins

**Status:** not started
**Severity:** minor
**Cluster:** any-interface-builtins
**Evidence:** `internal/node/guards.go:117-119` + 1 more
**Problem:** Two identical hand-rolled max blocks in projectCompactPeakMiB; the max builtin (Go 1.21) is house style elsewhere.
**Fix:** peak = max(peak, allocatedMiB) at both sites.
**Effort:** hours

##### `mod:7352ede7:use-builtins` — use builtins

**Status:** not started
**Severity:** suggestion
**Cluster:** any-interface-builtins
**Evidence:** `internal/render/errorbox.go:118-120`
**Problem:** Floor clamp via if; width = max(width, 1) is the builtin form. Single site, if-form arguably as readable.
**Fix:** width = max(width, 1)
**Effort:** hours

##### `mod:2845ab58:use-synctest` — use synctest

**Status:** not started
**Severity:** minor
**Cluster:** any-interface-builtins — seam→concurrency — related: con:2845ab58:synctest-opportunity
**Evidence:** `internal/tui/spinner_test.go:176-185` + 1 more
**Problem:** TestStatusLine_SetUpdatesDesc sleeps 300ms of wall clock to let a 120ms ticker repaint (load-dependent flake window), and TestSpinner_NoDeadlockUnderConcurrentLogs guards with a 10s time.After; both goroutine sets touch only an in-memory bytes.Buffer, so testing/synctest (stable Go 1.25, already used in 3 other test files here) makes them deterministic.
**Fix:** Wrap bodies in synctest.Test(t, func(t *testing.T){...}): the 300ms sleep becomes virtual (use synctest.Wait() after set() to drain the repaint); in the deadlock test replace the done-channel + 10s time.After guard with plain wg.Wait() - synctest fails deterministically on bubble-wide deadlock. Caution: the deadlock test is also a -race interleaving stress; synctest's deterministic scheduler narrows interleavings, so consider keeping a non-synctest -race variant.
**Effort:** hours

#### audit-code-smells

##### `smell:c287d5c0:stringly-typed-enum` — stringly typed enum

**Status:** not started
**Severity:** minor
**Cluster:** magic-strings — seam→api-design — related: api:c287d5c0:opt-inconsistent
**Evidence:** `internal/distribution/okd/okd.go:291-298` + 3 more
**Problem:** PhaseSetup/PhaseInstall/PhasePostInstall are untyped string constants and DeployStep.Phase is a bare string, while the sibling deploy package models the same three-value set as a typed enum deployPhase (state.go). Nothing prevents assigning an arbitrary phase string to DeployStep.Phase.
**Fix:** Declare a defined string type in okd.go, type the three constants against it, and change DeployStep.Phase to that type; keep the string values verbatim (they equal deploy's on-disk marker phase names and the map keys in deploy.go:225). Cross-package unification of the two copies is deferred to audit-api-design.
**Effort:** hours

##### `smell:166385d6:enum-ad-hoc` — enum ad hoc

**Status:** not started
**Severity:** minor
**Cluster:** magic-strings
**Evidence:** `internal/distribution/okd/setup/fstrim.go:15-15` + 1 more
**Problem:** MachineConfig pool roles are spelled as raw []string{"master", "worker"} literals although the package already uses the typed nodetypes.RoleMaster/RoleWorker enum for the same vocabulary (nodes.go, haproxy.go, terraform.go). The role enum exists nearby but these two sites bypass it.
**Fix:** Build both role slices from the enum: []string{string(nodetypes.RoleMaster), string(nodetypes.RoleWorker)} so pool names route through the one role vocabulary. Rendered values are unchanged.
**Effort:** hours

##### `smell:f5d703ab:enum-ad-hoc` — enum ad hoc

**Status:** not started
**Severity:** minor
**Cluster:** magic-strings — seam→api-design
**Evidence:** `internal/distribution/okd/setup/artifacts.go:42-42` + 2 more
**Problem:** The fixed OKD tool-binary trio {openshift-install, oc, kubectl} has named same-package constants (setup/phase.go openshiftInstallBin/ocBin/kubectlBin) yet InstallToolsToSystem re-spells all three as raw literals, and cleanup/packages.go repeats the raw trio again — a partial const set with sites that ignore it and no single source of truth.
**Fix:** Use openshiftInstallBin/ocBin/kubectlBin in artifacts.go; optionally expose one shared list (mirroring phase.ExternalToolBinaries()) so cleanup's InstalledBinaries reuses it instead of re-spelling the trio. Cross-package hoist is audit-api-design's call.
**Effort:** hours

##### `smell:ed55ee90:arrow-pyramid` — arrow pyramid

**Status:** not started
**Severity:** suggestion
**Cluster:** arrow-anti
**Evidence:** `internal/distribution/okd/cleanup/summary.go:41-56`
**Problem:** GenerateSummary's terraform-file tally nests six levels (if Kind > if ReadDir > for entries > if IsDir > for filenames > if Stat). nestif never scores it because the two for-ranges break its consecutive-if chain, but a human reads it as an arrow; a continue-guard plus an extracted per-dir counter flattens it to two levels.
**Fix:** Invert to `if !entry.IsDir() { continue }`, then extract the inner filename loop into countTerraformArtifacts(dir string) int so GenerateSummary reads: guard scope, read dir, sum per-dir counts.
**Effort:** hours

##### `smell:2c4d8e6b:interfaceany-lazy-exported` — interfaceany lazy exported

**Status:** not started
**Severity:** suggestion
**Cluster:** interfaceany-lazy — seam→api-design — related: api:2c4d8e6b:export-no-caller-scaffolding
**Evidence:** `internal/addon/addon.go:55-64` + 2 more
**Problem:** Exported interface method ConfigurableAddon.DecodeSettings(map[string]string) (any, error) returns any. Every implementation has a concrete typed decodeSettings that Install/ValidateSettings call directly; the any-returning wrapper has no polymorphic consumer and forces an unchecked type assertion on any future caller.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

#### audit-dependencies

##### `dep:33ef32bf:version-major-stuck` — version major stuck

**Status:** not started
**Severity:** major
**Cluster:** justified-version-floor
**Evidence:** `go.mod:14-16` + 1 more
**Problem:** golang.org/x/crypto is held at v0.52.0 for a v0.53.0-specific CI runner OOM, but the renovate rule disables ALL x/crypto updates (enabled: false) with no expiry; upstream is now at v0.54.0, so the security-critical SSH stack (sshpin host-key pinning, flux addon deploy keys) drifts behind with no automated update path.
**Fix:** Narrow the renovate rule from enabled:false to "allowedVersions": "!/^v0\\.53\\.0$/" so v0.54.0+ PRs flow again; run one CI canary bumping x/crypto to v0.54.0 to check whether the v0.53.0 OOM reproduces, then update the go.mod hold comment (and CLAUDE.md record) with the outcome either way.
**Effort:** hours

##### `dep:b803fcb7:maint-stale` — maint stale

**Status:** not started
**Severity:** major
**Cluster:** maintenance-signal — seam→iac-and-shell
**Evidence:** `.github/workflows/ci.yml:120-126`
**Problem:** The scan-terraform job runs aquasecurity/tfsec-action v1.0.3 (last released 2022); upstream tfsec is deprecated in favor of Trivy ('our engineering attention will be directed at Trivy going forward'), so the misconfiguration rule set silently decays for new provider/HCL patterns while CI keeps reporting green.
**Fix:** Replace aquasecurity/tfsec-action with a SHA-pinned aquasecurity/trivy-action (scan-type: config, scan-ref: infrastructure/terraform) — Trivy embeds the same Terraform engine and keeps receiving rules; triage any new findings once, keep the SHA-pin + version-trailer convention.
**Effort:** hours

##### `dep:33ef32bf:v0-dep-unjustified` — v0 dep unjustified

**Status:** not started
**Severity:** minor
**Cluster:** justified-version-floor
**Evidence:** `go.mod:24-24`
**Problem:** github.com/charmbracelet/colorprofile v0.4.3 is a direct v0.x dependency with no justification/abandonment entry in CLAUDE.md §dependencies, which requires one for every v0.x dep; the recorded registry lists only go-proxmox and bpg/proxmox.
**Fix:** Add a CLAUDE.md §dependencies v0.x entry for colorprofile: charm-ecosystem terminal color detection, single call site (internal/tui/colorprofile.go), fallback = inline ~30 LOC of env/TTY profile detection or adopt lipgloss-native profile API if charm.land absorbs it; track upstream releases (v0.4.3 is current latest).
**Effort:** hours

##### `dep:6ebdb617:baseline-record-stale` — baseline record stale

**Status:** not started
**Severity:** minor
**Cluster:** maintenance-signal — related: api:f5eb0ca4:export-no-caller-scaffolding, sec:1cb08008:tls-insecure-skip
**Evidence:** `CLAUDE.md:305-313`
**Problem:** The recorded go-proxmox baseline has drifted: the entry says v0.7.x and 'sole Proxmox discovery path' in the wizard, but go.mod is at v0.8.1 and the dep is now also consumed by internal/infrastructure/proxmox/probe.go and power.go (VM power operations), so the documented ~200 LOC REST-only rewrite fallback is under-scoped.
**Fix:** Refresh the entry: version v0.8.x, call-site list += internal/infrastructure/proxmox/{probe,power}.go, re-estimate the rewrite fallback to cover discovery + probe + power. Re-confirmed this run: bus-factor 1 still holds (top contributor 236 vs 20 commits), release cadence healthy (v0.8.1 on 2026-07-15, monthly releases since Feb), 3-month rewrite trigger NOT met.
**Effort:** hours

##### `dep:33ef32bf:transitive-heavy-narrow` — transitive heavy narrow

**Status:** not started
**Severity:** minor
**Cluster:** transitive-weight — related: dep:6ebdb617:baseline-record-stale, api:1cb08008:export-no-caller
**Evidence:** `go.mod:12-12`
**Problem:** go-proxmox's linked transitive set in cmd/okdctl includes buger/goterm (a terminal UI library) and jinzhu/copier (reflection deep-copier) alongside the recorded diskfs + gorilla/websocket — all linked-but-never-called by okdctl; CLAUDE.md's transitive-weight tally names only diskfs, understating the weight that counts toward the go-proxmox rewrite trigger.
**Fix:** Extend the CLAUDE.md transitive-weight tally to name goterm + copier as linked-uncalled weight (and note mage stays go.mod-only under pruning); optionally open an upstream issue asking go-proxmox to split terminal/ISO helpers out of the root package so REST-only consumers avoid them.
**Effort:** hours

##### `dep:b803fcb7:pin-tool-unannotated` — pin tool unannotated

**Status:** not started
**Severity:** minor
**Cluster:** pin-stability
**Evidence:** `.github/workflows/ci.yml:19-21` + 5 more
**Problem:** Six tool-version pins (golangci-lint v2.12.2 in ci.yml AND Makefile, goreleaser v2.15.2 in two workflows, terraform 1.10.3, tflint v0.62.0) carry no renovate annotation, so the repo's regex customManager cannot see them: they rot silently, and the two golangci-lint copies can drift apart — local-vs-CI lint parity has bitten before per MEMORY.md.
**Fix:** Add '# renovate: datasource=github-releases depName=...' (or datasource=go) annotations above each pin, matching the GOVULNCHECK_VERSION / YAMLFMT_VERSION pattern already in ci.yml and AIR_VERSION in the Makefile; the customManagers regex already matches ':'- and '='-style assignments in .yml, Makefile, and .sh files.
**Effort:** hours

##### `dep:ad6a01e5:pin-go-tool-latest` — pin go tool latest

**Status:** not started
**Severity:** suggestion
**Cluster:** pin-stability
**Evidence:** `lefthook.yml:4-7`
**Problem:** The gofumpt pre-commit hook runs whatever gofumpt is on PATH with no version control at all — weaker even than an @latest install — unlike every other tool in the repo; formatter output can differ across gofumpt versions, producing commit-time churn that CI's pinned golangci-lint v2.12.2 formatter check may then dispute.
**Fix:** Pin gofumpt like the Makefile pins air (install-on-miss at an annotated explicit version, e.g. go install mvdan.cc/gofumpt@vX.Y.Z with a renovate comment), or run the hook through the pinned golangci-lint ('golangci-lint fmt') so it inherits the v2.12.2 pin.
**Effort:** hours

#### audit-documentation

##### `doc:b8750296:readme-flag-missing` — readme flag missing

**Status:** not started
**Severity:** minor
**Cluster:** readme-drift
**Evidence:** `docs/cli/okdctl_node_resize.md:19-45`
**Problem:** The committed generated reference page for `node resize` predates commit c4ea8f4f (which added --skip-drain to node.go without running `make docs`): the Options block lacks --skip-drain, the Synopsis lacks its six-line memory-pressure paragraph, and the third example still shows `workers --cpu 8` instead of the --skip-drain example the code now emits.
**Fix:** Run `make docs` and commit the regenerated docs/cli/okdctl_node_resize.md. The docs-go CI job already blocks merge on this drift; to stop it landing in commits at all, add the Makefile docs-check target to lefthook pre-push (lefthook.yml currently runs only build+test there).
**Effort:** hours

##### `doc:b3356305:readme-flag-missing` — readme flag missing

**Status:** not started
**Severity:** minor
**Cluster:** readme-drift — seam→cli-ux
**Evidence:** `README.md:88-105`
**Problem:** The hand-maintained README Usage block enumerates every top-level command but omits `plan` — a first-class registered command with its own dedicated exit code (7) and generated reference page. A reader scanning the README never learns `okdctl plan` exists.
**Fix:** Add `okdctl plan          preview infrastructure drift without applying changes` to the README Usage block between the node and releases lines, mirroring the cobra Short as the other 16 entries do.
**Effort:** hours

##### `doc:5f5527e7:exported-doc-echo-sig` — exported doc echo sig

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-doc — related: api:5f5527e7:export-no-caller-scaffolding
**Evidence:** `internal/render/summary.go:76-77` + 2 more
**Problem:** `// Newline writes a blank line.` on a one-line `WriteString("\n")` method is a pure name-echo adding zero signal — CLAUDE.md's canonical 'the function doc just echoes the signature' case. Sibling Builder docs Section (L61) and KV (L66) are the same shape but at least name the output format.
**Fix:** Deletion would trip revive's exported rule, so enrich instead: fold the Builder-wide output contract (two-space indent, dotted alignment against keyWidth/kvWidth) into the type doc and keep the method docs minimal, or state what each writer contributes to the summary layout.
**Effort:** hours

##### `doc:de572c63:exported-doc-echo-sig` — exported doc echo sig

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-doc
**Evidence:** `internal/distribution/okd/dns/dnsmasq.go:50-53` + 1 more
**Problem:** `// EnableDnsmasq enables and starts the dnsmasq service.` and `// RestartDnsmasq restarts the dnsmasq service.` restate their identifiers on one-line ManageService wrappers; the docs add nothing the names don't already say.
**Fix:** Enrich with the one non-obvious fact (EnableDnsmasq both enables AND starts — a semantic ServiceEnable carries; when in the deploy sequence each is invoked) or accept the minimal echo since revive exported mandates a doc comment on both.
**Effort:** hours

##### `doc:b3356305:readme-summary-drift` — readme summary drift

**Status:** not started
**Severity:** suggestion
**Cluster:** readme-drift — seam→cli-ux
**Evidence:** `README.md:96-96`
**Problem:** README describes `describe` as 'drill into a specific node or addon' while the cobra Short (and generated okdctl.md:36) says 'Show details for a cluster node or addon' — semantically equivalent, but it is the only Usage-block line that does not track its command's Short verbatim.
**Fix:** Reword the README line to 'show details for a cluster node or addon' so all Usage entries mirror their cobra Shorts, keeping the block greppable against `okdctl --help` output.
**Effort:** hours

#### audit-tests

##### `tst:c287d5c0:cred-zeroize-untested` — cred zeroize untested

**Status:** not started
**Severity:** major
**Cluster:** cred-path-untested
**Evidence:** `internal/distribution/okd/okd.go:350-359` + 2 more
**Problem:** okd.Provisioner.ZeroizeEnv has no test anywhere in the repo, and a test would immediately expose that it only delegates to the executor while p.pendingEnv (populated by WithEnv with creds.Env() strings from deploy/deploy.go:100) is never blanked or nilled — credential plaintext stays reachable for the Provisioner's lifetime despite the documented defer p.ZeroizeEnv() contract.
**Fix:** Add TestProvisioner_ZeroizeEnv in internal/distribution/okd (in-package, stdlib testing): construct p := New(WithEnv([]string{"PROXMOX_VE_PASSWORD=hunter2","PROXMOX_VE_API_TOKEN=tok"})), snapshot the pendingEnv backing slice, call p.ZeroizeEnv(), then assert (a) p.pendingEnv is nil and every snapshotted entry is "", and (b) p.executor's env is drained (add an in-package accessor or assert via executor.SnapshotEnv() returning empty). Table cases: nil executor (no panic — mirror TestExecutor_ZeroizeEnv_NilSafe), empty pendingEnv no-op, idempotent second call. Writing this test today FAILS on (a): fix ZeroizeEnv to also blank secret-keyed pendingEnv entries via logutil.KeyIsSecret then clear+nil the slice, matching the CLAUDE.md ZeroizeEnv pattern implemented in internal/infrastructure/proxmox/proxmox.go:91-100 and internal/executor/executor.go:665-674.
**Effort:** hours

##### `tst:fde34e0c:cred-scrub-untested` — cred scrub untested

**Status:** not started
**Severity:** major
**Cluster:** cred-path-untested
**Evidence:** `internal/cluster/k8s.go:142-188` + 1 more
**Problem:** cluster/k8s.go::subcommand is designated by errtypes_credleak_test.go as the SOLE guard preventing full argv (e.g. --from-literal=password=X) from reaching ExitError.Command and wrapped errors, yet no test exercises the guard through Client.Run/runOutput/runCheck — the errtypes canary only proves ExitError does not self-redact, so replacing subcommand(args) with strings.Join(args, " ") would leak secrets into errors and logs with zero test failures.
**Fix:** Add TestClientRun_NoArgvLeakInErrors in internal/cluster (stdlib, in-package): build c := New(WithCLI("definitely-not-on-path-xyz"), WithExecutor(executor.New())) and call c.Run(ctx, "create", "secret", "generic", "s", "--from-literal=password=s3cr3t"); assert err != nil, strings.Contains(err.Error(), "create") and !strings.Contains(err.Error(), "s3cr3t"). Second sub-test for the non-zero-exit path: use the package's existing fake-oc InstallFakeBin helper (see k8s_ceph/csrs tests) with a script that exits 1, call runCheck via an exported wrapper (e.g. ApplyManifest or another runCheck-backed method) with a secret-bearing arg, errors.As to *executor.ExitError and assert Command == "oc create" exactly and the secret is absent from Error(). Third: subcommand(nil) == "(no args)" table case. This turns the credleak canary's cross-package contract into an enforced invariant at the guard itself.
**Effort:** hours

##### `tst:35abd54e:cred-zeroize-untested` — cred zeroize untested

**Status:** not started
**Severity:** major
**Cluster:** cred-path-untested
**Evidence:** `internal/credentials/proxmox.go:146-186` + 4 more
**Problem:** CLAUDE.md mandates that every creds.Env() call site appear in the known-call-sites registry in this doc comment AND carry a defer ZeroizeEnv(), but no test enforces either claim — and the registry is already stale (it lists cli/deploy.go for the proxmox.WithEnv site, which actually lives at cli/helpers.go:113), proving the drift the missing test would catch.
**Fix:** Add an AST-sweep test modeled on errtypes_credleak_test.go (stdlib go/parser, no type-checking needed): walk internal/ and cmd/ non-test .go files, find every CallExpr whose Fun is a SelectorExpr with Sel.Name=="Env" on a receiver identifier matching creds/credentials naming (and whose enclosing file imports internal/credentials), then assert (1) the enclosing function declaration also contains at least one call to ZeroizeEnv (defer or explicit on all return paths — a simple 'function body contains ZeroizeEnv' check is enough to catch omission), and (2) the file's repo-relative path appears verbatim in the Known-call-sites block of the ProxmoxCredentials.Env doc comment (read this source file, extract lines between 'Known call sites' and 'func (c *ProxmoxCredentials) Env'). Test fails today on the stale cli/deploy.go entry — fix the doc comment to cli/helpers.go in the same change. This converts the CLAUDE.md reviewer-checklist item from manual review into an enforced invariant.
**Effort:** hours

##### `tst:4c092fce:cred-zeroize-untested` — cred zeroize untested

**Status:** not started
**Severity:** minor
**Cluster:** cred-path-untested
**Evidence:** `internal/infrastructure/terraform/terraform.go:556-564`
**Problem:** terraform.Executor.ZeroizeEnv is the method every cli destroy/node defer relies on, but its only test (TestExecutor_ZeroizeEnv_NilSafe) asserts nil-safety and never verifies the delegation actually blanks secret-keyed entries in the inner executor — the method could regress to a no-op and no terraform-package test would fail.
**Fix:** Extend TestExecutor_ZeroizeEnv_NilSafe (or add TestExecutor_ZeroizeEnv_BlanksInnerEnv) in internal/infrastructure/terraform destroy_test.go, in-package: e := New(t.TempDir(), WithEnv([]string{"PROXMOX_VE_PASSWORD=hunter2","PROXMOX_VE_API_TOKEN=tok"})); snap := e.exec.SnapshotEnv() to confirm the entries arrived; e.ZeroizeEnv(); then assert len(e.exec.SnapshotEnv()) == 0 (executor nils its slice, so SnapshotEnv returns an empty copy). This pins the WithEnv→AppendEnv→ZeroizeEnv wiring that the defer tf.ZeroizeEnv() contract in cli/destroy.go:394 and cli/node.go:238 depends on, without duplicating the byte-level blanking assertions already locked in executor_test.go::TestZeroizeEnv.
**Effort:** hours

##### `tst:daf5bee9:cred-scrub-untested` — cred scrub untested

**Status:** not started
**Severity:** minor
**Cluster:** cred-path-untested
**Evidence:** `internal/cli/kubeconfig.go:67-79`
**Problem:** The okdctl kubeconfig --output-file write path persists cluster-admin credentials to an arbitrary path with 0o600 via AtomicWrite, but kubeconfig_test.go only asserts perms on the mergeKubeconfig path — nothing locks the direct-write call site's 0o600 mode, so a mode-constant regression (e.g. 0o644) would ship silently.
**Fix:** Add TestRunKubeconfig_OutputFilePerms in internal/cli (stdlib, in-package): seed a temp projectRoot with <root>/okd-install/cluster-config/auth/kubeconfig content (mirror how other cli tests fabricate phase.WorkDirName layout), point resolveProjectRoot at it (chdir via t.Chdir or the package's existing project-root test seam), set kubeconfigOutput to a temp dest, call runKubeconfig with a cobra.Command whose OutOrStdout is a buffer, then os.Stat the dest and assert Mode().Perm() == 0o600 and the bytes round-tripped. Add a second case for the "-" stdout path asserting the file content reaches the buffer and no file is created. Restore the package-level kubeconfigOutput/kubeconfigMerge globals via t.Cleanup to avoid cross-test bleed.
**Effort:** hours

##### `tst:7b2829bb:cred-scrub-untested` — cred scrub untested

**Status:** not started
**Severity:** major
**Cluster:** cred-path-untested — seam→observability — related: sec:fe5a42c5:cred-in-log, err:fe5a42c5:err-wraps-cred-downstream
**Evidence:** `internal/executor/executor.go:242-248`
**Problem:** ExitError.Redacted() — the sole mechanism keeping subprocess stderr out of structured slog sinks — has 0% coverage and no test reference anywhere. Error()'s 400-byte truncation is tested, but the slog-side contract (Stderr omitted entirely from the Redacted() shape) is locked by nothing.
**Fix:** Add TestExitError_Redacted in executor_test.go: build &ExitError{Command:"terraform apply",ExitCode:1,Stderr:"provider auth: password=hunter2"}, assert the Redacted() value marshals/prints without the stderr text, then an end-to-end case logging the error through logutil.RedactHandler into a bytes.Buffer slog.Handler and asserting "hunter2" is absent. Table-driven, stdlib only.
**Effort:** hours

##### `tst:7b2829bb:canonical-helper-untested` — canonical helper untested

**Status:** not started
**Severity:** blocker
**Cluster:** canonical-helper-untested — related: con:632c9087:cancelled-ctx-cleanup, tst:c287d5c0:cred-zeroize-untested
**Evidence:** `internal/executor/executor.go:267-270` + 3 more
**Problem:** RunWithStdin, RunChecked, RunWithStdinChecked and AppendEnv all sit at 0% coverage with no test references. RunWithStdin is the transport for the stdin-fed `oc create -f -` apply+rollback in postinstall/update_ingress.go (the CLAUDE.md-documented Client exception), and AppendEnv carries KUBECONFIG (install/phase.go:177) and the terraform credential env (terraform.go:70) — the stdin wiring and env-append/Zeroize interplay are locked by nothing.
**Fix:** Fake-binary PATH tests in executor_test.go (existing repo pattern): (1) RunWithStdin round-trip through `cat`, asserting stdout==input and stdin is fully drained past the pipe buffer; (2) RunChecked/RunWithStdinChecked exit-nonzero cases asserting a typed *ExitError with captured stderr; (3) AppendEnv followed by SnapshotEnv asserting the appended entry is visible to the child and that ZeroizeEnv afterwards blanks a secret-keyed appended entry (locks the append/Zeroize interplay).
**Effort:** days

##### `tst:e2343d2c:canonical-helper-untested` — canonical helper untested

**Status:** not started
**Severity:** blocker
**Cluster:** canonical-helper-untested — related: state:d7ce9d16:restore-without-restart, tst:de572c63:destructive-partial-untested
**Evidence:** `internal/system/systemd.go:32-69`
**Problem:** internal/system's systemd control primitives (ManageService, IsServiceActive, IsServiceEnabled) — named as part of the package's charter in CLAUDE.md — have no test file at all. They gate root-required service mutations on the setup path (apache/haproxy enable+restart), the dns path (EnableDnsmasq/RestartDnsmasq) and the cleanup restore path (systemd-resolved restart); the argv mapping (status→is-active, everything else verbatim) and non-Linux error/false semantics are all unlocked.
**Fix:** New systemd_test.go: fake `systemctl` shell script on a prepended PATH recording argv to a file (pattern from executor/dns tests); table over every ServiceAction asserting exact argv (ServiceStatus → is-active; others verbatim), plus IsServiceActive/IsServiceEnabled true/false on exit 0/1. On non-Linux, assert ManageService returns an error and the Is* probes return false — this half runs on darwin dev machines too.
**Effort:** days

##### `tst:25fa1be8:destructive-happy-untested` — destructive happy untested

**Status:** not started
**Severity:** major
**Cluster:** destructive-untested
**Evidence:** `internal/distribution/okd/firewall/firewall.go:134-252`
**Problem:** Configure, RemoveRules, modifyPort and DetectBackend are all at 0% coverage — only validatePort and the port tables are tested. The per-backend argv construction (`ufw delete allow N/proto`, `iptables -D INPUT ...`, `firewall-cmd --remove-port=...`) and RemoveRules' warn-and-continue partial-failure semantics for root-required firewall mutation are locked by no test.
**Fix:** Fake firewall-cmd/ufw/iptables scripts on PATH recording argv: table over backend × add/remove × permanent asserting exact argv per dialect; a RemoveRules case where the fake fails on the first port and the test asserts the second port is still attempted (warn-only contract) and Configure conversely aborts on first failure; DetectBackend precedence via which-fake-exists.
**Effort:** days

##### `tst:de572c63:destructive-partial-untested` — destructive partial untested

**Status:** not started
**Severity:** major
**Cluster:** destructive-untested — seam→state-and-recovery — related: state:d7ce9d16:restore-without-restart
**Evidence:** `internal/distribution/okd/dns/dnsmasq.go:80-108` + 1 more
**Problem:** writeDnsmasqConfig (backup-before-overwrite of /etc/dnsmasq.d drop-ins) and ConfigureSystemResolver (nmcli DNS override / systemd-resolved drop-in install) are at 0% coverage even though the package planted test seams for exactly this (dnsmasqConfigDir, resolvedConf, restartDnsmasqFn vars). The inverse RestoreSystemResolver has three tests — the destructive forward path has none, so the .backup file contract that validateAndRestartDnsmasq's tested rollback depends on is itself unverified.
**Fix:** Redirect dnsmasqConfigDir to t.TempDir() (seam exists): assert (a) first write creates <name>.conf with 0644 and no .backup, (b) overwrite copies the prior bytes to <name>.conf.backup before the atomic replace, (c) invalid name is rejected before any file op, (d) cancelled ctx writes nothing. For ConfigureSystemResolver, point resolvedConf into t.TempDir() and fake systemctl/nmcli on PATH; assert the [Resolve] drop-in content, 0644 mode, and temp-file cleanup.
**Effort:** days

##### `tst:ddf885f4:destructive-confirm-untested` — destructive confirm untested

**Status:** not started
**Severity:** major
**Cluster:** destructive-untested — related: state:fd2125dd:concurrent-run-addon-unlocked, con:ddf885f4:cancelled-ctx-cleanup
**Evidence:** `internal/addon/manager.go:238-283` + 1 more
**Problem:** Manager.Uninstall, its dependent-guard (findTransitiveDependent/dependsOn, including the cycle-protection visited map) and VerifyAll are all at 0% with no test references. The guard is the only check stopping a destructive uninstall (flux: `oc delete ns flux-system`; secretstore: oc delete secret/secretstore) of an addon that another enabled addon still depends on — while the install-side rollback logic has six dedicated tests.
**Fix:** In-package manager_test.go additions using the existing fake-addon registry harness: (a) uninstall refused when a direct dependent is enabled, (b) refused for a transitive chain a→b→c, (c) a dependency cycle terminates (visited map) rather than recursing forever, (d) uninstall proceeds and calls the addon's Uninstall exactly once when no dependent exists, (e) unknown addon name returns ConfigError. Mirror the InstallAll test style already in the file.
**Effort:** hours

##### `tst:0f076161:destructive-confirm-untested` — destructive confirm untested

**Status:** not started
**Severity:** major
**Cluster:** destructive-untested — seam→state-and-recovery — related: state:0f076161:destroy-topology-drift-blind, ux:0f076161:flag-help-wording-drift
**Evidence:** `internal/cli/destroy.go:268-366`
**Problem:** Every destroy guard is unit-tested in isolation (confirmClusterMatches, validateDestroyTargets, validateDestroyFlagCombos, buildDestroyOptions), but runDestroy itself is executed by no test (0% coverage), so nothing locks that the highest-blast-radius command actually invokes the confirm gate, runlock acquisition, and AnnounceState before calling p.Destroy — deleting the confirmClusterMatches call from runDestroy would pass the entire suite.
**Fix:** Cobra execute-level test (pattern: internal/cli/root_test.go / deploy_test.go): temp workspace + minimal config file, run the destroy command with --yes and a wrong/missing --confirm-cluster, assert the typed refusal error surfaces BEFORE any credentials load or terraform invocation (fake terraform on PATH that fails the test if executed); a second case asserts the interactive prompt path cancels cleanly on 'n' using the promptForConfirmation stdin harness from confirm_test.go.
**Effort:** hours

##### `tst:93957c53:destructive-confirm-untested` — destructive confirm untested

**Status:** not started
**Severity:** major
**Cluster:** destructive-untested — seam→state-and-recovery — related: state:93957c53:cleanup-weak-confirm-gate, state:bdf5a873:cleanup-deletes-live-artifacts
**Evidence:** `internal/cli/cleanup.go:68-151`
**Problem:** runCleanup and runCleanupDryRun are at 0% coverage. cleanup's default kind=full deletes the cluster's only auth artifacts (cluster-config/auth kubeconfig + kubeadmin password per state:bdf5a873), and the command's confirm gate (confirmClusterMatches + interactive prompt), --dry-run routing, and invalid --kind rejection have no execute-level test — the dry-run branch returning before any destructive call is exactly the kind of wiring a refactor can silently break.
**Fix:** Execute-level cobra test: (a) `cleanup --dry-run` in a temp workspace asserts only the dry-run preview lines print and no file under the workspace is removed; (b) `cleanup --yes` without --confirm-cluster asserts refusal before runlock/phase construction; (c) invalid --kind returns the UsageError. Reuse the temp-config + stdin harness from confirm_test.go.
**Effort:** hours

##### `tst:c5e5c304:destructive-happy-untested` — destructive happy untested

**Status:** not started
**Severity:** major
**Cluster:** destructive-untested — related: state:c19ee328:resume-breaks-after-ignition
**Evidence:** `internal/distribution/okd/setup/terraform.go:173-189` + 1 more
**Problem:** GenerateTerraformVars is at 0% coverage while every helper it calls is at 100%. Its one distinct behavior — removing the bootstrap-state sentinel so deploy (and only deploy) resurrects the bootstrap VM, while WriteTerraformVars must preserve the sentinel so node add/remove/resize cannot — is an invariant documented in a WHY-comment but locked by no test on either side.
**Fix:** Two table cases in setup/terraform_test.go with a t.TempDir() envDir seeded with phase.BootstrapStateSentinelFile: (1) GenerateTerraformVars(cfg, opts) removes the sentinel and renders terraform.tfvars 0600; (2) WriteTerraformVars(cfg, envDir) leaves the sentinel bytes untouched. Assert both directions so the split cannot silently collapse.
**Effort:** hours

##### `tst:61f4d731:destructive-confirm-untested` — destructive confirm untested

**Status:** not started
**Severity:** minor
**Cluster:** destructive-untested
**Evidence:** `internal/cluster/k8s_ceph.go:34-70` + 1 more
**Problem:** The health-gate orchestration (CephHealthy: toolbox-pod selection, oc exec plumbing, Applicable=false routing; EtcdHealthy: probe sequencing) is at 0% coverage, while only the pure parse layers underneath are tested. These gates are the go/no-go check for destructive node ops (compact, remove, snapshot rollback, resize) via waitCephHealthy/waitEtcdHealthy.
**Fix:** Fake-executor Client tests (pattern: k8s_nodeops_test.go): (a) no toolbox pod → Applicable=false without error; (b) exec non-zero exit → typed ClusterError, never healthy; (c) firstScheduledPod skips unscheduled pods; (d) EtcdHealthy propagates an unavailable operator as unhealthy. The node-side wait loops are already tested against a fake Cluster interface — this closes the layer below them.
**Effort:** hours

##### `tst:40d315ad:cred-scrub-untested` — cred scrub untested

**Status:** not started
**Severity:** minor
**Cluster:** cred-path-untested
**Evidence:** `internal/addon/catalog/flux/flux.go:383-428` + 1 more
**Problem:** createDeployKeySecret's credential preflight — the lstat symlink refusal on ~/.ssh/flux-deploy-key and readKeyFile's lstat+O_NOFOLLOW guard on the private-key read — is at 0% coverage. The pure secret builder (buildFluxDeployKeySecret) and the keyscan fingerprint pinning are tested, but the guard that stops a symlinked deploy key from exfiltrating an arbitrary root-readable file into a cluster secret is not.
**Fix:** Table test for readKeyFile with t.TempDir(): regular file returns bytes; symlink at final component refused; missing file errors with the ssh-keygen hint intact. The sibling readNoFollow in setup/ignition.go has exactly this test shape (ignition_test.go SymlinkRejected) — mirror it.
**Effort:** hours

##### `tst:aa0f50f5:trust-boundary-untested` — trust boundary untested

**Status:** not started
**Severity:** suggestion
**Cluster:** trust-boundary-untested
**Evidence:** `internal/config/validators.go:573-698`
**Problem:** The exported wizard-facing wrapper validators (ValidateDomain, ValidateProxmoxNodeName, ValidateIP, ValidatePortNumber, and the ValidateIntRange preset family) are at 0% coverage. Residual risk is low — the primitives underneath (isValidDomain, IsValidIP, proxmoxNamePattern) are tested, and the shell-facing twin (hostssh validateProxmoxName) carries attacker-shaped tests — but the wrappers are one bad refactor (inverted condition, wrong pattern var) away from waving hostile wizard input through, and nothing would fail.
**Fix:** One table test sweeping the wrapper family with a short accept/reject list each (empty string, oversized input, metacharacters, boundary numbers 0/1/65535/65536), mirroring TestValidateClusterName in the same file. Cheap insurance; primitives stay the single source of truth.
**Effort:** hours

#### follow-ups discovered during the fix campaign

##### `iac:b611e9fe:root-vars-silently-ignored` — production root omits template-rendered variables

**Status:** not started
**Severity:** major
**Cluster:** hcl-wiring (discovered by fix agent G7, not an audit row)
**Evidence:** `infrastructure/terraform/environments/production/`
**Problem:** The production root omits many variables the tfvars template renders (bridge, os_storage, data_storage, cpu_type, worker_data_disk_size_gb, master_count, ...), so those tfvars values are silently ignored and module defaults win.
**Fix:** Wire every template-rendered variable through the production root (pattern: minimum_data_disk_size_gb wiring in 21712934); add a tripwire test diffing template variable names against root variable declarations.
**Effort:** hours

##### `sec:1cb08008:redirect-cap-dedup` — probe redirect-cap mirrors unexported httputil helpers

**Status:** not started
**Severity:** suggestion
**Cluster:** dedup (discovered by fix agent G1)
**Evidence:** `internal/infrastructure/proxmox/probe.go`
**Problem:** capAPIRedirects locally mirrors internal/httputil's unexported redirect-cap because the exported factories can't carry the insecure transport.
**Fix:** Export a transport-accepting variant from internal/httputil and dedupe.
**Effort:** hours


## Completed

Completed items live in [`docs/roadmap/completed-archive.md`](docs/roadmap/completed-archive.md). Grep there for the canonical "is dep X done?" lookup. The previous in-line pointer index (144 entries, mirroring archive contents) was removed on 2026-05-09 to keep `roadmap.md` focused on active work.

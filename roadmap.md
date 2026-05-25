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

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-88fd3050-cred-config  
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

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-00000005-oc-integrity  
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

**Status:** not started
**Severity:** suggestion
**Cluster:** tls-network
**Evidence:** `scripts/install.sh:113-119`
**Problem:** GitHub releases-API resolution at L113-L119 fetches the latest tag via TLS to api.github.com with no integrity check beyond the TLS handshake. A compromised api.github.com cert or attacker-controlled DNS resolving to a TLS-MITM proxy would let the resolver return any tag; the subsequent cosign verification of SHA256SUMS only verifies the file at $BASE_URL — but $BASE_URL is built from the attacker-influenced VERSION. Once VERSION is malicious, the trust chain follows. The cosign cert-identity-regexp does anchor artefacts to qxtaiba/okdctl, so the attacker must serve a real signed release.
**Fix:** Defense-in-depth: after VERSION is resolved, validate it matches the regex ^v[0-9]+\.[0-9]+\.[0-9]+ before constructing URLs. This won't stop a creative attacker but bounds the URL grammar. The cosign cert-identity-regexp at L156 is the real trust root.
**Effort:** hours

##### `sec:35abd54e:cred-env-leak-to-child` — cred env leak to child

**Status:** not started
**Severity:** suggestion
**Cluster:** credentials
**Evidence:** `internal/credentials/proxmox.go:157-178`
**Problem:** ProxmoxCredentials.Env() converts the secret []byte fields to Go strings via string(c.APIToken) and string(c.Password) on KEY=VALUE concatenation. Per the file's own doc at L150-L156, these Go strings can outlive the []byte sources and cannot be wiped. Executor.ZeroizeEnv (executor.go:L440) and Provider.ZeroizeEnv (proxmox.go:L91) blank the slice entries and clear() the slice — but only on the Executor's owned slice. If a copy of the returned []string is retained anywhere (logged, cached, sent to another goroutine), the immutable string headers persist. The architectural comment is good; one runtime mitigation would help.
**Fix:** Defense-in-depth: add a runtime check that env-slice receivers (terraform.Executor, proxmox.Provider) install ZeroizeEnv defers immediately. Could be enforced by a small linter rule that flags any call site that uses creds.Env() without defer x.ZeroizeEnv() in the same function. The doc at L150-L156 already nails the rule; missing piece is mechanical enforcement.
**Effort:** hours

##### `sec:4c092fce:toctou-chmod` — toctou chmod

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-4c092fce-toctou
**Severity:** minor
**Cluster:** file-toctou
**Evidence:** `internal/infrastructure/terraform/terraform.go:395-414`
**Problem:** SnapshotState reads terraform.tfstate at the working dir then AtomicWrites a backup to <workDir>/terraform.tfstate.<ts>.bak with perm 0o600. terraform.tfstate itself contains the proxmox credentials marshalled by the bpg provider — and the backup also lands at 0o600. However, the os.ReadFile at L403 does not Lstat-then-O_NOFOLLOW: a planted symlink at the state-file path would let the read follow into an attacker-chosen file (which is then copied to the backup path under workDir). Workdir is chown'd back to the invoking user, so the threat is the same pre-sudo symlink redirection as in flux.go and ignition.go.
**Fix:** Open the state file with syscall.O_NOFOLLOW (read-only). The workdir is created by okdctl phases so legitimate operators won't have symlinks here, but the file is root-owned under sudo and a pre-sudo attacker could plant one. Same pattern as runlock.Acquire.
**Effort:** hours

##### `sec:48688e63:input-cidr-not-parsed` — input cidr not parsed

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-48688e63-cidr
**Severity:** suggestion
**Cluster:** input-validation
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:131-145`
**Problem:** Provider.Connect reads cfg.Provider.Proxmox.Host and immediately calls config.ValidateProxmoxHost, but uses cfg.Provider.Proxmox.Node verbatim at L135 with no validateProxmoxName check. p.node then flows downstream into phase.RemoteISOParams.Node (proxmox.go:L482) and through pveshRun (pvesh.go:L13) where validateProxmoxName fires — so the gate exists. But the validation happens late; if a future caller bypasses pveshRun and uses p.node directly in an ssh argv, an unvalidated node name reaches the remote shell.
**Fix:** Add a validateProxmoxName(p.node) check in Connect, mirroring the ValidateProxmoxHost gate above it. The downstream pveshRun fires today, but defense-in-depth at the API boundary catches the misuse case where a new caller bypasses pveshRun.
**Effort:** hours


#### audit-subprocess

##### `sub:ae5b624c:parallel-exec-wrapper` — parallel exec wrapper

**Status:** not started
**Severity:** suggestion
**Cluster:** io-handling — seam→`audit-api-design` — related: `sub:97cb8adf:no-cancel-func`
**Evidence:** `internal/distribution/okd/install/monitor.go:25-44`
**Problem:** defaultStartMonitorCmd reaches past the canonical executor.Executor to call osExec.CommandContext directly because the caller needs Start+Wait independence (the parent loop ticks at csrApprovalInterval while openshift-install runs for 30-60min). The env filter, cmd.Cancel SIGTERM, and WaitDelay are re-implemented inline; the embedded comment on L24 even acknowledges that this duplicates the canonical pattern. There is no Executor method that returns a done-channel for background subprocesses, so the duplication is structural rather than careless.
**Fix:** Add an Executor method along the lines of `StartStreamed(ctx, name, args...) (done <-chan error, err error)` that wires Start + a Wait-into-channel goroutine while sharing buildEnv / Cancel / WaitDelay from the existing run pattern. Then defaultStartMonitorCmd shrinks to a single call: `return p.Exec.StartStreamed(ctx, "openshift-install", "wait-for", "install-complete", "--dir", clusterDir, "--log-level=debug")`. Net LOC delta ~-10 at this site (+15 in executor) for a real interface gain. The new method is symmetric with RunStreamed.
**Effort:** hours

#### audit-state-and-recovery

##### `state:0f076161:destroy-no-cluster-confirm-without-yes` — destroy no cluster confirm without yes

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/state-0f076161-confirm-cluster
**Severity:** minor
**Cluster:** destroy-safety
**Evidence:** `internal/cli/destroy.go:230-245`
**Problem:** Interactive destroy (no --yes flag) only requires answering 'y' or 'yes' to the prompt — no cluster-name typing requirement, unlike kubectl/eksctl. A misread terminal tab (wrong cluster's CLI history) plus a single 'y' executes destroy against the wrong cluster. The --confirm-cluster guard is only enforced when --yes is set.
**Fix:** In the interactive branch (line 237), prompt 'type cluster name to confirm destroy: ' and require the response to match cfg.Cluster.Name (case-sensitive). Match the --confirm-cluster=NAME contract used for scripted destroys. Skip when --target/--only is present because those branches already require --confirm-cluster at line 204-208. Optional: behind a TUI flag if it disrupts CI snapshot testing.
**Effort:** hours

##### `state:fb54208a:postinstall-mid-phase-no-checkpoint` — postinstall mid phase no checkpoint

**Status:** not started
**Severity:** minor
**Cluster:** crash-recoverability
**Evidence:** `internal/distribution/okd/postinstall/steps.go:23-115`
**Problem:** PhaseContext (pctx) is in-process memory only. The 5-step postinstall sequence — verify-health, cleanup-bootstrap, verify-kubevip, deploy-production-dns, install-addons — has no persistent checkpoint. A crash between StepCleanupBootstrap and StepDeployProductionDNS loses the KubeVipIP that StepVerifyKubeVIP computed; the next run starts at verify-health and recomputes everything from scratch, relying on filesystem sentinels (bootstrap-state.auto.tfvars.json for bootstrap, dnsmasq config for DNS state) for idempotency. Resume is fragile because the recovery path depends on multiple independent sentinels staying in sync.
**Fix:** Two-stage fix tracked separately on roadmap: (1) lightweight — write a per-phase JSON checkpoint at <workDir>/.<phase>-checkpoint.json after each successful step containing the StepID and any pctx state needed for resume. AlreadyDone reads the checkpoint at orchestrator start. (2) heavier — extend StepDef with a Checkpoint hook and have Orchestrator persist after each success. Today's mitigation is the on-disk sentinels (bootstrap-state.auto.tfvars.json, dnsmasq conf state, kubeconfig presence) which each step's AlreadyDone consults independently. Defer per roadmap state:4f69fc9d; do not act now.
**Effort:** hours

##### `state:b5a79fda:deploy-state-marker-not-version-tagged` — deploy state marker not version tagged

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/state-b5a79fda-schema-version
**Severity:** suggestion
**Cluster:** state-schema-evolution
**Evidence:** `internal/cli/deploystate.go:26-32`
**Problem:** deployState JSON schema (phase/run_id/timestamp/cluster_name) has no schemaVersion field. If the format evolves (e.g. add a step_id or a partial-progress field), an old marker file is silently unmarshaled with zero values for new fields — the recovery announce path can degrade to wrong-cluster-warnings or stale-marker false positives.
**Fix:** Add `SchemaVersion string \`json:"schema_version"\`` to deployState with a package constant `deployStateSchemaV1 = "v1"`. writeDeployState sets it; readDeployState rejects mismatched versions with a Warn+ignore (the file is advisory, not authoritative). Mirrors config.SchemaVersionV1 pattern. Consider deferring until the schema actually evolves.
**Effort:** hours

##### `state:b38ec9cc:workers-targeted-apply-skips-other-drift` — workers targeted apply skips other drift

**Status:** not started
**Severity:** suggestion
**Cluster:** phase-idempotency
**Evidence:** `internal/distribution/okd/install/workers.go:46-76`
**Problem:** StartWorkerVMs runs `terraform apply -target=module.okd_cluster.proxmox_virtual_environment_vm.worker -var start_workers_immediately=true`. The targeted apply intentionally scopes to workers — but if a user hand-edited terraform.tfvars between setup and install (changing master CPU, network bridge), that drift is silently NOT applied. A subsequent un-scoped apply (e.g. via `okdctl deploy` re-run) suddenly applies all the drift at once.
**Fix:** No code change. The targeted apply is the right choice — un-scoped apply during worker-start would risk applying mid-cluster drift on master VMs. Document this trade-off in the function-level docstring (it's already a comment block, just lift to the function doc) so future readers don't mistake it for a forgotten target restriction.
**Effort:** hours

##### `state:bdf5a873:work-dir-cleanup-no-resume` — work dir cleanup no resume

**Status:** not started
**Severity:** suggestion
**Cluster:** crash-recoverability
**Evidence:** `internal/distribution/okd/cleanup/artifacts.go:70-100`
**Problem:** WorkDirectory removes per-subdirectory via SafeRemoveWithLogger with errs accumulated, but no transactional or resume semantics. A SIGKILL between `remove(opts.WorkDir, cluster configuration)` and `remove(workDir, work directory)` leaves a partially-stripped okd-install/ — the next cleanup run sees the missing children, treats AlreadyDone as 'not done' (DirExists check), and re-runs against the partial state. This is fine because os.RemoveAll is idempotent on missing children, but the package-level docstring already documents this gap ('Cleanup is best-effort: a mid-run crash leaves workDir in a partially-removed state with no resume capability').
**Fix:** No code change. The package docstring at cleanup.go:5-6 already names this limitation and documents the mitigation (terraform.tfstate last). Verify the audit-positive baseline holds: cleanup steps after StepCleanupWorkDir do not depend on workDir/* content. Cross-check: webServer/haproxy/dnsmasq/terraform/packages steps reference cfg.HTTPServerRoot, /etc/haproxy/haproxy.cfg, /etc/dnsmasq.d/, infrastructure/terraform/, binDir — all OUTSIDE workDir. Confirmed safe.
**Effort:** hours

##### `state:4c092fce:snapshot-pruner-not-locked` — snapshot pruner not locked

**Status:** not started
**Severity:** suggestion
**Cluster:** tf-state-atomicity
**Evidence:** `internal/infrastructure/terraform/terraform.go:420-442`
**Problem:** pruneSnapshots reads the workDir, sorts terraform.tfstate.*.bak entries lexicographically, and removes the oldest beyond the 5-retain limit. Two concurrent okdctl runs in the same workDir could each prune the same files — both os.Remove calls succeed (or one ENOENT-no-ops), and both walk an interleaved view of the directory. Runlock prevents the concurrent case within a host; cross-host NFS does not. Low-impact because the file content (state snapshot) is fully written before rename.
**Fix:** No code change. The acceptable failure mode (concurrent prunes producing slightly-different retention counts) is bounded by the 5-retain limit. The error path tolerates ENOENT. Verified: pruneSnapshots is only called from SnapshotState which is already gated by runlock at the CLI layer (destroy/deploy/bootstrap-destroy all Acquire the runlock).
**Effort:** hours

##### `state:48688e63:proxmox-api-no-direct-409-path` — proxmox api no direct 409 path

**Status:** not started
**Severity:** suggestion
**Cluster:** proxmox-api-idempotency
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:30-36`
**Problem:** Provider docstring declares 'all Proxmox mutations MUST flow through terraform.Executor. Direct Proxmox HTTP calls are forbidden in deploy/destroy paths.' Verified: the only direct Proxmox-host interactions are SSH/pvesh for ISO listing/removal (idempotent: rm -f silent on missing) and a read-only enumeration probe in probeVMEnumeration. No 409/already-exists handling is needed because terraform owns mutation retry/backoff.
**Fix:** No code change. Confirm the invariant holds in future PRs: any new Proxmox API consumer in deploy/destroy must route through terraform.Executor, not net/http. The probeVMEnumeration helper at proxmox.go:470-503 is the documented exception (read-only SSH/pvesh enumeration).
**Effort:** hours

##### `state:881d089e:runlock-flock-cross-host-documented` — runlock flock cross host documented

**Status:** not started
**Severity:** suggestion
**Cluster:** tf-state-atomicity
**Evidence:** `internal/runlock/runlock.go:1-10`
**Problem:** runlock uses syscall.Flock (advisory) for cross-process exclusion in the same project root. The package docstring and CLAUDE.md §architecture-notes both document the NFSv3 cross-host caveat. crossHostHint() at line 95-108 even surfaces the HOST= field mismatch in the error message. Terraform's own state lock (-lock-timeout=120s on every locking subcommand) is the authoritative guard.
**Fix:** No code change. The dual-lock model is correct: runlock catches the common case (same-host concurrent okdctl), terraform's -lock-timeout=120s catches the cross-host case with a queue-then-fail-with-diagnostic. Verify: every state-locking subcommand in terraform.go (Plan, PlanStreamed, Apply, Destroy direct/with-plan) passes -lock-timeout=120s — confirmed at lines 244, 264, 284, 347.
**Effort:** hours

##### `state:fb54208a:postinstall-no-rollback-recurring` — postinstall no rollback recurring

**Status:** not started
**Severity:** minor
**Cluster:** crash-recoverability — related: `state:632c9087:update-ingress-haproxy-rollback-missing`
**Evidence:** `internal/distribution/okd/postinstall/steps.go:42-96`
**Problem:** Postinstall steps cleanup-bootstrap (destroys bootstrap VM via targeted apply) and deploy-production-dns (replaces /etc/dnsmasq.d/*.conf) are both NonFatal and mutate external state. If StepCleanupBootstrap succeeds, StepVerifyKubeVIP fails, StepDeployProductionDNS is skipped (SkipWhen reads !pctx.Get().KubeVIPVerified), the cluster is left with: bootstrap VM gone, kube-vip unverified, DNS still pointed at bootstrap. There is no rollback step that restores bootstrap DNS pointing at bastion. Already filed in roadmap (state:fb54208a:postinstall-no-rollback-path) as deferred minor.
**Fix:** Recurring — see roadmap state:fb54208a:postinstall-no-rollback-path. Preferred fix: extend update-ingress to detect 'bootstrap-DNS + bootstrap-VM-gone' state and re-issue production DNS without depending on the postinstall pctx (update-ingress already owns dns.IsBootstrapDNS + dns.DeployProduction). No code change in this audit run; defer per roadmap.
**Effort:** hours

##### `state:eb479d86:iso-upload-already-done-sha256` — iso upload already done sha256

**Status:** not started
**Severity:** suggestion
**Cluster:** phase-idempotency
**Evidence:** `internal/distribution/okd/setup/upload.go:179-205`
**Problem:** isoUploadAlreadyDone walks every local ISO and SSHes to the Proxmox host to compute sha256 of the remote file — but a SSH-error or a missing-file is conservatively returned as (false, nil), making the step Exec run. Verified correct: the conservative-not-done choice means the step re-runs and the real failure mode surfaces during Exec rather than being silently skipped.
**Fix:** No change. The sha256 + skip-if-match pattern is idempotent and resilient. The nolint:nilerr comment correctly justifies the policy.
**Effort:** hours

##### `state:0f076161:destroy-skip-flag-orthogonal-with-dryrun` — destroy skip flag orthogonal with dryrun

**Status:** not started
**Severity:** suggestion
**Cluster:** destroy-safety
**Evidence:** `internal/cli/destroy.go:210-227`
**Problem:** --dry-run combined with --skip-terraform/--skip-cleanup/--skip-firewall returns a ConfigError. The error message is clear ('skip flags have no effect') but the orthogonality between dry-run (preview) and skip-flags (resume) is implicit. Operators resuming a partial destroy via --skip-terraform may try --dry-run first to preview and hit this error.
**Fix:** No code change. Optionally augment the docstring on destroyCmd.Long (line 138-156) to name the dry-run/skip orthogonality explicitly: 'dry-run is for previewing terraform-destroy; skip flags are for resuming after a partial terraform-destroy.'
**Effort:** hours

##### `state:d7ce9d16:dns-deploy-restart-fail-restore` — dns deploy restart fail restore

**Status:** not started
**Severity:** suggestion
**Cluster:** phase-idempotency
**Evidence:** `internal/distribution/okd/dns/dns.go:242-277`
**Problem:** validateAndRestartDnsmasq has a backup-and-restore path: on validation OR restart failure, it copies the .backup file back over the live config. Verified clean: backup happens in writeDnsmasqConfig (line 89-93) BEFORE AtomicWriteString, so the backup is the prior config. On restart success the backup is removed. The restore() closure correctly uses system.CopyFile (preserves source mode at open time).
**Fix:** No change. The backup-validate-restart-restore-on-fail pattern is the canonical recovery shape for service-config writes. Cross-reference with internal/distribution/okd/setup/haproxy.go:147-166 (attemptHAProxyRollback) for the symmetric template.
**Effort:** hours


#### audit-iac-and-shell

##### `iac:e076e43c:sh-cosign-cert-identity-regex-loose` — sh cosign cert identity regex loose

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/iac-e076e43c-cosign
**Severity:** minor
**Cluster:** install-sh-integrity
**Evidence:** `scripts/install.sh:156-157`
**Problem:** --certificate-identity-regexp is `https://github\.com/qxtaiba/okdctl/` — matches any workflow path under the repo. Defense-in-depth would pin to the specific release workflow (e.g., `\.github/workflows/release\.yml@refs/tags/v.*`) so a future stray signed workflow can't satisfy the check. The OIDC issuer check is still the real gate, but the cert-identity should narrow as tightly as the release process tolerates.
**Fix:** Tighten to `--certificate-identity-regexp='^https://github\.com/qxtaiba/okdctl/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+'` (or whatever the goreleaser release workflow path is). Verify the regex matches the cert identity that goreleaser-action embeds in the SHA256SUMS signature; iterate in a draft release if needed.
**Effort:** hours


#### audit-errors

##### `err:d7ce9d16:dns-bare-fmt-errorf-not-classified` — dns bare fmt errorf not classified

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/err-d7ce9d16-dns-classify
**Severity:** minor
**Cluster:** sentinel-vs-typed
**Evidence:** `internal/distribution/okd/dns/dns.go:24-215`
**Problem:** buildConfigData and the surrounding DNS package return bare fmt.Errorf('cluster name is required') / fmt.Errorf('invalid apps IP address: %s', appsIP) etc. for config-shaped failures. They reach exit-code mapping only because orchestrator.classifyStepErr wraps non-typed errors in ClusterError (exit 4). These are config-shaped failures that should map to ConfigError (exit 2). When DeployBootstrap/DeployProduction are called outside the orchestrator (no current direct callers, but on the symmetric-API path) they lose the right exit code.
**Fix:** Replace each bare fmt.Errorf in dns.go validation paths with &errtypes.ConfigError{Msg: ...}. classifyStepErr will still pass them through (no double-wrap because the As checks fire). Today this is invisible (exit code remains 4 via classify) but the audit calls it out so the dns package's contract matches the rest of the phase tree.
**Effort:** hours

##### `err:48688e63:proxmox-apply-cancel-bare-wrap` — proxmox apply cancel bare wrap

**Status:** not started
**Severity:** suggestion
**Cluster:** cancellation-identity — seam→`audit-concurrency`
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:232-242`
**Problem:** On ctx-cancelled terraform apply: fmt.Errorf('terraform apply interrupted: %w', errors.Join(ctx.Err(), applyErr)). errors.Is(err, context.Canceled) walks correctly because errors.Join exposes Unwrap() []error. But the outer fmt.Errorf is the only object visible to errors.As, so callers cannot recover a typed *errtypes.ClusterError here (the non-cancel branch at L241 returns one). Mixed-shape return is a smell — callers must branch on errors.Is before errors.As to handle both shapes.
**Fix:** The bare wrap is intentional and load-bearing — internal/cli/root.go::signalExitCode walks the chain via errors.Is(err, context.Canceled) before exitCodeFor runs, mapping SIGINT→130. The pattern matches the install/monitor.go canon. Add an inline comment (matching install/monitor.go:L65-68) so reviewers do not wrap it in ClusterError and break the SIGINT→130 mapping.
**Effort:** hours

##### `err:d6b325cb:proxmox-sentinel-ad-hoc` — proxmox sentinel ad hoc

**Status:** not started
**Severity:** suggestion
**Cluster:** sentinel-vs-typed
**Evidence:** `internal/infrastructure/proxmox/types.go:10-13`
**Problem:** Package proxmox defines two exported sentinels (ErrNotConnected, ErrTerraformNotConfigured) outside errtypes. Both are reachable from external callers via errors.Is. The repo's stated vocabulary lives in errtypes — these sentinels are wrapped in ConfigError at every call site (proxmox.go:L191, L199, L273, L281) so the chain is correct, but the placement diverges from the errtypes-as-home convention.
**Fix:** Leave as-is OR move to errtypes as ErrProviderNotConnected / ErrProviderUnconfigured for vocabulary uniformity. Package-local sentinels for package-internal preconditions are idiomatic Go (see io.EOF, fs.ErrNotExist) and the caller-facing class is ConfigError (correctly classified by errors.As).
**Effort:** hours

##### `err:97cb8adf:subprocess-stderr-tail-latent-leak` — subprocess stderr tail latent leak

**Status:** not started
**Severity:** minor
**Cluster:** redaction-in-error — seam→`audit-observability`
**Evidence:** `internal/system/exec.go:25-44`
**Problem:** SubprocessError stores raw StderrTail and Error() concatenates it: `e.Bin + ': ' + e.Err.Error() + ': ' + e.StderrTail`. Redacted() omits StderrTail so slog attrs are safe. But callers may wrap SubprocessError into outer errors with %w and then stringify. Symmetric concern with executor.ExitError; same fix shape.
**Fix:** Apply logutil.RedactableStderr head/tail truncation inside Error() so the eyeball-rendered error caps at <=400 bytes regardless of sink. Today there is no proven leak — call sites for OutputCaptured (ssh-keyscan, ip-addr) do not handle credentials. The finding lives at the type to make the contract symmetric with the security invariant.
**Effort:** hours

##### `err:fde34e0c:k8s-subcommand-load-bearing-comment` — k8s subcommand load bearing comment

**Status:** not started
**Severity:** suggestion
**Cluster:** redaction-in-error — seam→`audit-observability`
**Evidence:** `internal/cluster/k8s.go:126-138`
**Problem:** subcommand() returns args[0] when present, used to construct error messages: `fmt.Errorf('%s %s failed: %w', c.CLI, subcommand(args), err)`. The comment claims this prevents 'arbitrary arg values (which could carry --from-literal=... style secrets) in wrapped errors or logs'. The contract holds — but it is enforced by reviewer discipline, not a test. A future change to executor.ExitError that adds full argv to its message would break this redaction story.
**Fix:** No code change required. Add an invariant test or document on executor.NewExitError that the Command field never includes argv beyond name+' '+args[0] for cluster.* call sites. Alternative: extend the existing TestMsgFieldNoCredentialInterpolation in errtypes_credleak_test.go to also scan executor.ExitError.Command formats.
**Effort:** hours

##### `err:b804b2ec:bootstrap-tfvars-classification` — bootstrap tfvars classification

**Status:** not started
**Severity:** suggestion
**Cluster:** sentinel-vs-typed
**Evidence:** `internal/distribution/okd/postinstall/bootstrap.go:89-92`
**Problem:** bootstrap-state.auto.tfvars.json write failure surfaces as &errtypes.ClusterError. Symmetric concern with destroy/helpers.go:L41 (state snapshot → ClusterError). Documented here as the canonical example so reviewers do not reclassify state-write failures as ConfigError just because the file is config-shaped — the write happens after a successful bootstrap-vm destroy and is part of the cluster lifecycle, not a config the user edits.
**Fix:** Leave as-is. ClusterError → exit 4 is correct here. Flagged for completeness because the rule slug suggests reviewers may second-guess; do not change.
**Effort:** hours

##### `err:b38ec9cc:lock-hint-exit-code-flip` — lock hint exit code flip

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/err-b38ec9cc-lock-hint
**Severity:** major
**Cluster:** sentinel-vs-typed — seam→`audit-cli-ux`
**Evidence:** `internal/distribution/okd/install/workers.go:39-71` + 3 more
**Problem:** errors.Join(hint, wrapped) where hint is *errtypes.ConfigError and wrapped is *errtypes.ClusterError works because errors.Join's Unwrap() []error lets errors.As walk to both. But exitCodeFor walks the declaration order: ConfigError → 2, ClusterError → 4. errors.As returns on the first match. The hint matches ConfigError → exit 2; the ClusterError → exit 4 mapping is unreachable. terraform init failure → ClusterError → 4 normally; same failure under a stale lock → ConfigError → 2. Operators scripting against exit codes will see flaky behaviour.
**Fix:** Either (a) downgrade LockHint to return a *string* that the wrapped ClusterError appends to Msg (exit code stays uniform), or (b) restructure to wrap the hint message inside the ClusterError's Msg with the underlying err: `&errtypes.ClusterError{Msg: msg + '; ' + hintMsg, Err: err}` so only one typed error is in the chain. (c) Or accept the exit-2-for-locked-state mapping and document it in cli/root.go's exit-code table. Today the chain produces inconsistent exit codes silently.
**Effort:** hours

##### `err:a4001485:errtypes-no-recoverable-type` — errtypes no recoverable type

**Status:** not started
**Severity:** suggestion
**Cluster:** domain-vocabulary
**Evidence:** `internal/errtypes/errtypes.go:1-11` + 3 more
**Problem:** Package doc explicitly defers a Transient/Recoverable error type until a retry-aware consumer exists. Three sites today implement ad-hoc retry-classification by case-on-type (proxmox/proxmox.go::initIsRetryable, addon/helpers.go::addonIsRetryable, download/retry.go::isRetryable). Each duplicates the 'cancel/deadline/cfg/auth → non-retryable' shape. The roadmap consumer (err:9f8e7d6c) is the right time to land a TransientError — but the three duplicate sites already exist and are now divergent.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `err:40d315ad:addon-flux-error-as-string` — addon flux error as string

**Status:** not started
**Severity:** suggestion
**Cluster:** string-sniffing — seam→`audit-api-design`
**Evidence:** `internal/addon/catalog/flux/flux.go:284-312`
**Problem:** ValidateSettings returns []string by calling err.Error() on the DecodeSettings result. The user-facing wizard validation path expects strings, but the decoder may return errtypes.ConfigError — flattening to string drops the typed-error chain. Today decoder errors come from yaml.UnmarshalStrict (bare errors), so the loss is theoretical. Flagged because the Addon.ValidateSettings([]string) signature dictates that all addon validators must drop typed errors.
**Fix:** Leave the Addon.ValidateSettings signature as []string — it serves the wizard UI, not orchestrator exit-code branching. Add a comment on the Addon interface noting that callers must NOT use this for exit-code branching; the orchestrator validates separately via cfg.Validate(). The smell is api-design-shaped (the interface returns []string instead of []error).
**Effort:** hours

##### `err:0b188cab:addon-helpers-lasterr-asymmetric` — addon helpers lasterr asymmetric

**Status:** not started
**Severity:** suggestion
**Cluster:** wrapping
**Evidence:** `internal/addon/helpers.go:29-45`
**Problem:** RetryDefault returns the *last* fn() error or wait.ErrWaitTimeout (whichever ExponentialBackoffWithContext yields). Compared with internal/download/retry.go::retryDownload which captures lastErr separately and returns it on cap-out so callers see the original failure rather than the timeout sentinel. The two retry helpers diverge on the lastErr-preservation pattern even though they are otherwise symmetric.
**Fix:** Capture lastErr like internal/download/retry.go does, then on Backoff exhaustion return lastErr (wrapped) rather than the wait.ErrWaitTimeout. Symmetric helpers should have symmetric error surfaces. Note: not a fix-mandatory finding — the loop in retryDownload is the canonical pattern; this one diverges silently.
**Effort:** hours

##### `err:f55b9c27:envfile-loadonce-no-sentinel` — envfile loadonce no sentinel

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/err-f55b9c27-envfile-sentinel
**Severity:** suggestion
**Cluster:** sentinel-vs-typed
**Evidence:** `internal/credentials/envfile.go:120-129`
**Problem:** LoadEnvFile returns a ConfigError when called twice with different paths. There is no way for the caller to distinguish 'already initialized, original error' from 'already initialized with a different path' without string-sniffing the Msg.
**Fix:** Add a package-local sentinel `var ErrEnvFileAlreadyLoaded = errors.New("env file already loaded with different path")` and use it as Err on the ConfigError. Callers can errors.Is to detect the double-init case without parsing Msg. Today there's exactly one caller chain (cli/helpers.go::handleCredentials, cli/destroy.go) so practical risk is zero.
**Effort:** hours

##### `err:d9f7733e:debug-bundle-stringified-errors` — debug bundle stringified errors

**Status:** not started
**Severity:** suggestion
**Cluster:** redaction-in-error — seam→`audit-observability`
**Evidence:** `internal/cli/debug_bundle.go:211-410`
**Problem:** bundleConfig/bundleLogFile/bundleTerraformState/bundleMustGather/bundleDoctor/bundleSystemMeta build manifest entries by calling err.Error() — they stringify potentially-typed errors into a string field of manifestEntry, dropping the chain. The manifest is YAML-serialized and embedded in the bundle. A typed error carrying credential-bearing inner is reduced to its Error() string; errtypes.*.Error() omits inner err so the leak is bounded. Flagged because the pattern is fragile against a future error type that includes credentials in Error() text.
**Fix:** Route err through a debug_bundle-local 'safeMessage(err)' helper that calls Redacted() if the type implements it before falling back to Error(). Symmetric with internal/logutil/redact.go::redactAny's dispatch. The bundle is operator-facing and may be shared upstream, so redaction discipline matters here more than slog (RedactHandler already covers slog).
**Effort:** hours

##### `err:366b3f2d:orchestrator-classifysteperr-canonical` — orchestrator classifysteperr canonical

**Status:** not started
**Severity:** suggestion
**Cluster:** sentinel-vs-typed
**Evidence:** `internal/distribution/orchestrator.go:115-133`
**Problem:** classifyStepErr is the load-bearing safety net: it correctly preserves cancellation identity and skips wrapping for already-typed errtypes. The smell isn't in this function but in its presence — it catches 14+ bare fmt.Errorf sites in dns/postinstall/setup/install packages. Those packages' contracts are silently orchestrator-dependent. Documented as the canonical example so reviewers know why surrounding findings are minor not major.
**Fix:** No change. Documented as the canonical fallback so future audits know it exists. If classifyStepErr is ever removed or moved, the 14 bare-fmt.Errorf sites above must each be hardened first.
**Effort:** hours

##### `err:aa84670c:root-signalexitcode-invariant-test` — root signalexitcode invariant test

**Status:** not started
**Severity:** suggestion
**Cluster:** cancellation-identity — seam→`audit-concurrency`
**Evidence:** `internal/cli/root.go:181-192`
**Problem:** signalExitCode resolves SIGINT→130 / SIGTERM→143 by checking errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded). This is the documented intentional bare-wrap pattern. The implicit smell: this only works because EVERY cancellation site uses %w (or bare-wrap-without-typed-error) to preserve ctx identity. A single future site that wraps ctx.Err inside an errtypes.ClusterError WITHOUT %w would silently break SIGINT mapping.
**Fix:** Extend the existing TestUnwrapChainIntact (errtypes_test.go:L44-57) to cover all five errtypes × both ctx sentinels (Canceled and DeadlineExceeded). Locks the invariant that all errtypes' Unwrap chains preserve ctx identity for signalExitCode's chain walk.
**Effort:** hours

##### `err:ae5b624c:monitor-asymmetric-cancel-handling` — monitor asymmetric cancel handling

**Status:** not started
**Severity:** suggestion
**Cluster:** cancellation-identity — seam→`audit-concurrency`
**Evidence:** `internal/distribution/okd/install/monitor.go:48-75`
**Problem:** WaitForBootstrap handles three ctx-error branches asymmetrically: DeadlineExceeded wraps into ClusterError{Err: ctx.Err()} (preserves chain), Canceled bare-wraps via fmt.Errorf (preserves SIGINT→130), other err returns generic ClusterError. The asymmetry is intentional per the load-bearing comment at L65-68: the DeadlineExceeded path runs through exitCodeFor (ClusterError → 4), the Canceled path runs through signalExitCode (SIGINT → 130). Documented here for cross-reference.
**Fix:** No change. The asymmetry is deliberate and load-bearing per the in-code comments at L65-68 and L137-139. Documented here so reviewers know the DeadlineExceeded/Canceled split is intentional, not an inconsistency. The SIGINT path goes through cli/root.go::signalExitCode; the deadline path goes through exitCodeFor. Both yield correct exit codes (130 vs 4 respectively).
**Effort:** hours


#### audit-concurrency

##### `con:48688e63:disconnect-ctx-unused` — disconnect ctx unused

**Status:** not started
**Severity:** suggestion
**Cluster:** ctx-ignored
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:147-159`
**Problem:** Provider.Disconnect accepts context.Context but the body uses none of it - no I/O, no select, no ctx.Err check. The signature is shaped for a future network-bound disconnect handshake (Connect/Disconnect symmetry), and the receiver name underscores intent. Belongs on the scaffolding list, not the ranked table - fix is verify intent against roadmap, not delete the parameter.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours


#### audit-api-design

##### `api:262af6e4:opt-no-newoptions` — opt no newoptions

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/api-262af6e4-newoptions
**Severity:** minor
**Cluster:** option-consistency
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:68-83`
**Problem:** cleanup.Options is the only phase Options struct without a NewOptions(cfg,projectRoot) constructor — setup, install, postinstall, destroy all expose NewOptions for the same purpose. The three call sites that build cleanup.Options (cli/cleanup.go:115, okd/okd.go:122, destroy/steps.go:131) all hand-assemble BaseOptions and Kind separately, duplicating phase.GetTerraformEnv(cfg) plumbing.
**Fix:** Add func NewOptions(cfg *config.Config, projectRoot string, kind Kind) Options to internal/distribution/okd/cleanup/cleanup.go that sets BaseOptions{ProjectRoot,WorkDir,TerraformEnv} and Kind, mirroring destroy.NewOptions. Caller code (internal/cli/cleanup.go:115, internal/distribution/okd/okd.go:122, internal/distribution/okd/destroy/steps.go:131) replaces the inline literal with the call. Caller-specific overrides (HTTPServerRoot, HAProxyConfig, VIP, BinDir) still set field-by-field after construction.
**Effort:** hours

##### `api:d31d1b9d:pkg-facade-bypass-status` — pkg facade bypass status

**Status:** not started
**Severity:** minor
**Cluster:** package-boundary — seam→`audit-code-smells`
**Evidence:** `internal/cli/status.go:388-404`
**Problem:** cli/status.go constructs a phase.BasePhase directly (newStatusPhase) and uses bp.OcOutput/bp.OcResourceExists to query cluster state from the CLI layer. BasePhase is a sub-phase abstraction shared by setup/install/postinstall/destroy — it carries Exec/Log/Version/Recorder/Reporter for the orchestrator. A CLI status command shouldn't reach into a phase primitive to run oc; the cluster.Client (internal/cluster) is the canonical kubectl/oc surface and already exposes Run-style helpers.
**Fix:** Promote OcOutput/OcResourceExists-style 'kubectl raw query' helpers onto internal/cluster/Client (e.g. Client.RawGet(ctx, path string), Client.GetJSON(ctx, args ...string)). cli/status.go switches from newStatusPhase + bp.OcOutput to cluster.New(WithKubeconfig(kcPath)) + client.GetJSON. This keeps phase as the orchestration primitive and gives 'thin kubectl wrapper' a single owner.
**Effort:** hours

##### `api:d6b325cb:pkg-types-direction` — pkg types direction

**Status:** not started
**Severity:** minor
**Cluster:** package-boundary
**Evidence:** `internal/infrastructure/proxmox/types.go:1-49`
**Problem:** internal/infrastructure/proxmox imports internal/distribution/okd/phase for NodeRole, VMState, and the RemoteISOParams/PveshRun helpers. Per CLAUDE.md the dependency direction should be cli → distribution/okd → infrastructure/proxmox. The current shape inverts that on shared domain types: proxmox is parametrised by phase's NodeRole/VMState, and phase/pvesh.go contains Proxmox-specific subprocess plumbing (PveshRun, RemoteISOParams) that semantically belongs to the proxmox package. The package doc at vmstate.go:6 acknowledges the inversion.
**Fix:** Extract NodeRole/VMState/RemoteISOParams + the Proxmox SSH/pvesh subprocess primitives into a new internal/cluster/types or internal/infrastructure/proxmox/types-only sub-package that both phase and proxmox can import without forming a cycle. Today's compromise (cluster-domain enums in 'phase') works but every new shared concept widens the wrong-direction surface. Verify intent on the roadmap before refactor — multi-provider expansion would justify it, otherwise the cycle-break is the right call.
**Effort:** hours

##### `api:262af6e4:opt-execute-receiver-unused` — opt execute receiver unused

**Status:** not started
**Severity:** minor
**Cluster:** option-consistency
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:97-102`
**Problem:** cleanup.Phase.Execute is a one-line forwarder to a package-level execute() that only uses p.Log from the BasePhase. Other phases (setup, install, postinstall, destroy) use the full BasePhase shape — Exec/Log/Recorder — through orchestrator.SetMetricsRecorder, but cleanup discards everything except Log. The receiver becomes vestigial: a caller building cleanup via cleanup.New() with WithExecutor or WithRecorder gets a silent no-op for those options.
**Fix:** In internal/distribution/okd/cleanup/cleanup.go change execute() to take a *Phase or accept logger+recorder, and wire orchestrator.SetMetricsRecorder(p.Recorder) so cleanup emits step metrics like the other phases. Alternatively, document at Phase.Execute that cleanup intentionally skips metrics and have New() panic on phase.WithRecorder.
**Effort:** hours

##### `api:0fc0041d:export-no-caller-conditions` — export no caller conditions

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/condition.go:9-40`
**Problem:** ConditionType constants ConditionTypeAvailable, ConditionTypeProgressing and NodeStatusPhase.NodeStatusUnknown have no in-repo caller outside the test build. The package doc names them as scaffolding for a future status verb that surfaces non-Ready operator conditions.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:3e02f6b8:export-no-caller-vmstate` — export no caller vmstate

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/vmstate.go:11-17`
**Problem:** VMState constants StateStopped, StateCreating, StateDeleting, StateUnknown have no in-repo caller — only StateRunning is referenced by proxmox.Provider mapping pvesh output. The full state matrix is documented as a single source of truth for a future status path that surfaces partial-running clusters.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:0139cb3f:export-three-bindir-functions` — export three bindir functions

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/paths.go:53-92`
**Problem:** phase.ResolveBinDir, phase.PreflightBinDir, and phase.BinDirOrDefault expose three overlapping resolution functions that differ only in their input sources (config+env, env-only, struct-field+default). The 'three-function surface rationale' is documented inline. Whether the symmetry survives the next refactor depends on whether the env-only preflight path stays distinct from main-flow resolution.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:48688e63:zero-value-disconnect-noop` — zero value disconnect noop

**Status:** not started
**Severity:** suggestion
**Cluster:** zero-value-usability
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:147-159`
**Problem:** Provider.Connect/Disconnect form a symmetric pair, but Disconnect's body ignores ctx (the parameter name is `_`) and resets two struct fields without any genuine teardown. The shape is scaffolding for 'future network-bound providers' per the inline doc. Until a real teardown lands, callers who write ctx-aware code (defer prov.Disconnect(ctx)) get no benefit and the symmetric API hides a no-op behind a context-shaped signature.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:c287d5c0:export-no-caller-zeroize-env` — export no caller zeroize env

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/okd.go:198-207`
**Problem:** Provisioner.ZeroizeEnv delegates to the underlying executor's ZeroizeEnv and is exposed for credential-lifecycle scaffolding (annotated api:c287d5c0). No external caller in the current code; the field owner (executor.Executor.ZeroizeEnv) is called directly from the cli layer instead.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:859eea6f:export-no-caller-parsenoderole` — export no caller parsenoderole

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/noderole.go:21-32`
**Problem:** phase.ParseNodeRole is the deserialization counterpart to NodeRole.String() with no in-repo caller today. Inline doc names it as scaffolding for upcoming status JSON / terraform-output deserialization.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:9ce5434c:export-test-only-pollinterval` — export test only pollinterval

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/kubectl.go:48-52`
**Problem:** BasePhase.OcPollOutputInterval is exported solely as a test-injection seam for phase/kubectl_test.go — production code must use OcPollOutput which forwards with interval=0. The inline doc names the constraint. The exported surface lets any caller bypass the production interval-pinning, but it's the documented test pattern.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:7b2829bb:opt-name-allowlist-redundant` — opt name allowlist redundant

**Status:** not started
**Severity:** suggestion
**Cluster:** option-consistency
**Evidence:** `internal/executor/executor.go:103-143`
**Problem:** executor exposes DefaultEnvAllowlist (var) plus EnvAllowlist (type) plus FilterParentEnv(EnvAllowlist) free function. The var is the only EnvAllowlist value in the binary; callers always pass DefaultEnvAllowlist. The type+function surface is shaped like a reusable allowlist API, but the sole external caller (cli/elevation.go) passes the same default. The shape suggests multi-allowlist support that never materialised.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:f99eddfa:opt-options-pointer-asymmetry` — opt options pointer asymmetry

**Status:** not started
**Severity:** suggestion
**Cluster:** option-consistency
**Evidence:** `internal/distribution/okd/postinstall/phase.go:74-75`
**Problem:** All four phase Execute methods accept *Options but the construction shape diverges: NewOptions returns the value (Options, not *Options) for setup, install, postinstall, destroy. Callers must do opts := setup.NewOptions(...); phase.Execute(ctx, &opts). The double indirection (value-return → pointer-arg) is consistent across siblings but odd: either return *Options or take Options by value. The current shape forces every caller through the address-of dance.
**Fix:** Pick one: return *Options from NewOptions and accept *Options in Execute (current pattern, but document why the value-return is intentional), OR accept Options by value in Execute (simpler, lets the option-set live in the caller frame). Either choice is fine; the asymmetry between 'returns value' and 'takes pointer' is the issue. Mass-edit four phases simultaneously if you change it.
**Effort:** hours

##### `api:7f2bf677:pkg-pvesh-in-phase` — pkg pvesh in phase

**Status:** not started
**Severity:** minor
**Cluster:** package-boundary
**Evidence:** `internal/distribution/okd/phase/pvesh.go:1-47`
**Problem:** phase/pvesh.go contains Proxmox-specific subprocess primitives (pveshRun, PveshRun, pveshQEMUPath, pveshConfigPath, pveshResult). These are not 'cross-phase helpers' as the phase package doc claims — they are infrastructure provider plumbing that landed in phase to break the proxmox→phase import cycle (the same compromise vmstate.go and noderole.go document). Symptom: phase imports executor (subprocess) for an SSH/pvesh helper that only one consumer (infrastructure/proxmox) actually calls.
**Fix:** Long-term: extract Proxmox-specific helpers into infrastructure/proxmox/internal/pvesh (or similar) and let phase iso_cleanup call into infrastructure/proxmox for the pvesh subprocess primitives. The break-cycle compromise was justified when phase needed iso_cleanup; if iso_cleanup moves out of phase (it's already in its own file), the rationale weakens.
**Effort:** hours

##### `api:97cb8adf:opt-struct-vs-functional-waitfor` — opt struct vs functional waitfor

**Status:** not started
**Severity:** suggestion
**Cluster:** option-consistency
**Evidence:** `internal/system/exec.go:89-102`
**Problem:** system.WaitForOptions is the only configuration-via-struct in the repo's runtime/operation surface — every other 'configure X at call time' surface either uses inline arguments or a struct-of-options *passed to a constructor* (terraform.PlanOptions, proxmox.ProvisionOptions). WaitFor's signature takes (ctx, prefix, description, check, opts) — the options struct is materialized once via DefaultWaitForOptions() + field assignment, then passed. Consider whether WaitFor should follow the functional-options pattern of the constructors instead.
**Fix:** Choice: (a) keep WaitForOptions as the canonical 'per-call config struct' (and document it as the deliberate choice for one-shot configurable operations); (b) rewrite to func WaitFor(ctx, prefix, desc, check, opts ...WaitForOption) for full symmetry with constructor options. Two callers exist today (kubectl.go OcPollOutputInterval, exec_test.go); the WaitForWithTimeout wrapper exists precisely because the struct shape is verbose. Functional options would simplify the wrapper away.
**Effort:** hours

##### `api:7b2829bb:opt-with-inherited-env-noarg` — opt with inherited env noarg

**Status:** not started
**Severity:** suggestion
**Cluster:** option-consistency
**Evidence:** `internal/executor/executor.go:79-87`
**Problem:** executor.WithInheritedEnv is a no-arg toggle option that flips a boolean (inheritEnv). It pairs with WithEnv (appends) but its lack of a 'b bool' argument makes the option set asymmetric — there is no WithInheritedEnv(false) inverse. The current shape is one-way: a caller that wants 'maybe inherit, decided dynamically' must rebuild the option list rather than passing a single boolean.
**Fix:** Optional: change signature to WithInheritedEnv(b bool) Option { return func(e *Executor) { e.inheritEnv = b } } to match the bool-toggle shape used by download.WithOverwrite(v bool). Today's no-arg form is fine for the single use site that always wants 'true' — the asymmetry is only visible to a future caller wanting dynamic dispatch. Document the call-side intent inline if keeping the no-arg form.
**Effort:** hours

##### `api:dd75bdeb:export-no-caller-phasecontext` — export no caller phasecontext

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/context.go:1-12`
**Problem:** distribution.PhaseContext is exported and generic (typed via [T any]) with one consumer today (postinstall). The package doc names it scaffolded for symmetric use across phases (state:4f69fc9d, state:262af6e4). The exported surface includes a Unit-style API (Get/Update) that justifies the generic and RWMutex.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:cfcdee2d:export-no-caller-timeout-download` — export no caller timeout download

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface — seam→`audit-modernization`
**Evidence:** `internal/httputil/httputil.go:18-22`
**Problem:** httputil.TimeoutDownload is exported with no in-repo caller — TimeoutShort and TimeoutMedium are both used, but the 5-minute download timeout is unreferenced. The trio forms a symmetric tier; deleting the unused one breaks the tier shape.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:bb4fb1a3:zero-value-csr` — zero value csr

**Status:** not started
**Severity:** suggestion
**Cluster:** zero-value-usability
**Evidence:** `internal/cluster/types.go:1-9`
**Problem:** cluster.CSR is a single-field struct (Name string) whose only consumer is PendingCSRs returning a []CSR. The single-field wrap adds an indirection where a []string would suffice; the type doc admits 'Pending bool would always be true and add nothing'. The struct exists for future fields but is currently a 1-field box.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:a55b4592:zero-value-loader` — zero value loader

**Status:** not started
**Severity:** suggestion
**Cluster:** zero-value-usability
**Evidence:** `internal/config/loader.go:13-21`
**Problem:** config.Loader is an empty struct{} whose constructor returns &Loader{}. The type doc admits 'intentionally stateless today; the struct shape is the canonical surface so a future stateful Loader can land without breaking call-site shapes'. The empty struct + NewLoader is scaffolding for a future caching/decryption Loader.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:4c092fce:opt-execerror-alias` — opt execerror alias

**Status:** not started
**Severity:** suggestion
**Cluster:** option-consistency
**Evidence:** `internal/infrastructure/terraform/terraform.go:35-38`
**Problem:** terraform.ExecError is a type alias to executor.ExitError. The alias makes terraform.ExecError and executor.ExitError compatible for errors.As, but the surface is asymmetric: the rest of the terraform API exports its own types (Executor, PlanOptions, etc.) while ExecError forwards. Callers may not realize the two are the same type, leading to either-or errors.As branches.
**Fix:** Optional: either (a) keep the alias and document that errors.As(&t.ExecError) and errors.As(&executor.ExitError) are equivalent — both at the type-doc and in package docs — or (b) drop the alias and ask callers to errors.As against executor.ExitError directly. Today no caller branches against ExecError specifically; the alias adds clarity at the cost of one extra type name.
**Effort:** hours

##### `api:588ce79e:export-color-theme-enum` — export color theme enum

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/tui/colors.go:9-16`
**Problem:** tui.ColorTheme + ThemeDefault + ThemeHighContrast are exported but the only consumer of ColorTheme is the package-private setTheme(). No external caller; the enum exists for a future 'okdctl theme' CLI verb that would let users pick a palette.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:bbc23e42:pkg-progress-reporter-location` — pkg progress reporter location

**Status:** not started
**Severity:** suggestion
**Cluster:** package-boundary
**Evidence:** `internal/logutil/logutil.go:23-29`
**Problem:** logutil.ProgressReporter type and logutil.NopProgressReporter live in logutil (a logger-helpers package) while the real implementation StartSpinner lives in internal/tui. The exposed surface is fine; the smell is that the *type* lives in logutil when ProgressReporter has no logging connection and could equally live in tui (where StartSpinner is the real implementation).
**Fix:** Optional: move ProgressReporter type and NopProgressReporter to internal/tui (where StartSpinner already lives). Today's placement in logutil splits one concept across two packages. If left as-is, document at the logutil package doc why ProgressReporter lives there (likely: 'avoid the phase → tui dependency').
**Effort:** hours

##### `api:beabab0c:opt-execute-takes-cfg-twice` — opt execute takes cfg twice

**Status:** not started
**Severity:** suggestion
**Cluster:** option-consistency
**Evidence:** `internal/distribution/okd/setup/phase.go:120-135`
**Problem:** All four phase Execute methods take cfg *config.Config in addition to opts *Options, but opts is built by NewOptions(cfg, projectRoot) which already consulted cfg. The double-passing leaves callers responsible for keeping the (cfg, opts) pair consistent: if a caller calls NewOptions(cfg1, root) and then Execute(ctx, cfg2, &opts), the phase sees opts.TerraformEnv from cfg1 but cfg2 elsewhere. No bug today (callers always pass the same cfg), but the signature does not enforce the invariant.
**Fix:** Option A: store cfg in Options at NewOptions time, drop the cfg argument from Execute. Option B: leave as-is and document the invariant 'cfg passed to Execute must be the same cfg passed to NewOptions'. Option B is the lower-risk choice for now since 4 phases × Execute signatures × all callers is a lot of churn; consider as a v1.0 surface-stabilization item.
**Effort:** hours

##### `api:4f69fc9d:iface-fragmented-step` — iface fragmented step

**Status:** not started
**Severity:** minor
**Cluster:** interface-location
**Evidence:** `internal/distribution/step.go:33-67`
**Problem:** distribution.Step, Skipper, FatalChecker, AlreadyDoneChecker, StepCallbacks are all exported small interfaces. They are consumed implicitly by Orchestrator via type assertion (step.(AlreadyDoneChecker)). No external package implements these interfaces directly — every step is built through StepBuilder/StepDef which produces builtStep that satisfies them all. The fragmented interface set is a 'role interfaces' pattern but with one implementation, so the splits add cognitive load without value.
**Fix:** Two options: (a) collapse the five role interfaces into one ProvisioningStep interface (already exported at L71); the role interfaces become an unexported implementation detail used by Orchestrator. (b) Keep the role interfaces but document at the package doc that they are 'for orchestrator-side type assertion, not for external implementation'. Orchestrator.executeStep currently does step.(AlreadyDoneChecker) probes — these stay either way. Choose based on whether the project wants role interfaces as a public 'extensibility surface' or as orchestrator plumbing.
**Effort:** hours

##### `api:c287d5c0:opt-destroyopts-duplication` — opt destroyopts duplication

**Status:** not started
**Severity:** minor
**Cluster:** option-consistency
**Evidence:** `internal/distribution/okd/okd.go:181-222`
**Problem:** okd.DestroyOpts and destroy.Options carry nearly the same field set (AutoApprove, RemovePackages, KeepISOs, SkipTerraform, SkipCleanup, SkipFirewall, TerraformTargets). okd.Provisioner.Destroy unpacks DestroyOpts field-by-field into destroy.Options. The two structs differ by destroy.Options embedding phase.BaseOptions and adding CleanupKind/Parallelism. The duplication forces every new destroy flag to land in two places.
**Fix:** Pick one shape: (a) okd.Provisioner.Destroy takes destroy.Options directly (collapses DestroyOpts entirely); (b) DestroyOpts becomes the CLI-facing struct and the provisioner injects the BaseOptions+CleanupKind defaults internally before calling destroy.Phase.Execute. Option (a) is cleaner — the CLI builds destroy.Options once via destroy.NewOptions. The trade-off is exposing destroy.Options' embedded BaseOptions through the CLI surface; if BaseOptions internals shouldn't bleed into the CLI, prefer (b).
**Effort:** hours

##### `api:fde34e0c:opt-with-env-fallback-side-effect` — opt with env fallback side effect

**Status:** not started
**Severity:** minor
**Cluster:** option-consistency
**Evidence:** `internal/cluster/k8s.go:52-71`
**Problem:** cluster.WithEnvFallback() is a side-effect option that reads $KUBECONFIG and probes PATH for 'oc'. Every other WithX option in cluster.New is a pure setter. The asymmetry is documented inline ('Pass this option only when env-driven discovery is intentional'), but the env-read happens at option-application time rather than at New() — meaning the option's effect depends on the order options apply.
**Fix:** Two options: (a) move the env-fallback to a separate factory NewWithEnvFallback(opts ...Option) *Client that applies options then runs env-fallback as a finalizer; (b) keep WithEnvFallback as an option but document at its callsite that it MUST be the last option in the list. Today's two callers put it implicitly via the option order — fragile if a third caller arrives.
**Effort:** hours

##### `api:21dc1103:opt-name-clash-extract-option` — opt name clash extract option

**Status:** not started
**Severity:** minor
**Cluster:** option-consistency
**Evidence:** `internal/download/download.go:37-54`
**Problem:** internal/download exposes two option types: Option (for Fetch) and ExtractOption (for ExtractTarGz). The two surfaces also have parallel With* names that differ — WithChecksum on Fetch vs WithExtractChecksum on Extract. Within one package, two adjacent functions with diverging option-type-naming conventions.
**Fix:** Pick one shape: (a) rename Option → FetchOption and keep ExtractOption (both qualified); (b) rename ExtractOption → Option and split into download/extract sub-package. Option (a) is the safer choice — keeps the two surfaces in one package but makes the asymmetry explicit. Today's With* options also have a name disambiguation issue (WithChecksum on Fetch vs WithExtractChecksum on Extract) that the rename would address.
**Effort:** hours

##### `api:ae5b624c:iface-csr-approver-positive` — iface csr approver positive

**Status:** not started
**Severity:** suggestion
**Cluster:** interface-location
**Evidence:** `internal/distribution/okd/install/monitor.go:78-83`
**Problem:** install.csrApprover is a one-method interface (ApprovePendingCSRs) defined at the consumer side (install package) where cluster.Client is the real implementation. This is the correct shape per Go proverb 'Accept interfaces, return structs'. The interface is unexported (csrApprover), which is the right call for a single-package consumer. Logged as a positive counter-example for the api-design audit — every interface that abstracts a cluster operation should follow this shape.
**Fix:** No action — this is the canonical pattern. Document explicitly in cluster.Client's package doc that 'consumers may define their own narrow interfaces to test against; cluster.Client returns concrete types, never interfaces'. Future cluster-method additions (e.g. Client.GetNodes) should leave consumer-side interface definition to the consumer.
**Effort:** hours


#### audit-cli-ux

##### `ux:8d8faa80:help-no-example` — help no example

**Status:** not started
**Severity:** suggestion
**Cluster:** help-text
**Evidence:** `internal/cli/completion.go:11-30`
**Problem:** completionCmd has Long but no Example field. Every other user-facing subcommand (deploy, destroy, status, releases list, addon list, kubeconfig, debug-bundle) sets Example; completion is the only outlier even though its Long contains shell-by-shell snippets that would render more idiomatically as Example.
**Fix:** Move the bash/zsh/fish snippets from Long into Example and shorten Long to a one-paragraph description. Cobra renders Example in a dedicated section in --help, which puts the snippets where users look for runnable commands.
**Effort:** hours

##### `ux:d31d1b9d:help-short-tone` — help short tone

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/ux-d31d1b9d-describe-tone
**Severity:** minor
**Cluster:** help-text
**Evidence:** `internal/cli/status.go:38-42`
**Problem:** describeCmd.Short uses 'Drill into a specific node or addon' — colloquial 'drill into' is out of voice with the rest of the surface where Shorts are clinical imperatives ('Print', 'List', 'Show', 'Manage', 'Verify'). The Long is also a meta-sentence about how to start, not what the verb does.
**Fix:** Rewrite Short to a clean imperative such as 'Show details for a cluster node or addon' to match the tone of statusCmd.Short ('Print a post-deploy cluster summary'). Tighten Long to a single descriptive sentence (drop the 'start with' meta-prose).
**Effort:** hours

##### `ux:fd2125dd:concept-named-twice` — concept named twice

**Status:** not started
**Severity:** minor
**Cluster:** verb-noun
**Evidence:** `internal/cli/addon.go:28-31` + 1 more
**Problem:** Parent group nouns are inconsistently singular vs plural across the command tree: 'addon list/install/uninstall/verify' (singular) but 'releases list/show' (plural). Internal references compound the drift — addon.go uses 'addon list/verify' in messages while releases.go uses 'releases list'. Pick one: kubectl/oc convention is singular (kubectl get pod, kubectl describe service).
**Fix:** Rename releases → release for consistency with addon. The fix breaks anyone scripting okdctl releases list, but okdctl is pre-1.0 (per README §status) and the CHANGELOG can document the rename. Alternative: rename addon → addons. Either way, document the convention in CLAUDE.md so new groups follow it.
**Effort:** hours

##### `ux:4583b75b:output-flag-inconsistent` — output flag inconsistent

**Status:** not started
**Severity:** suggestion
**Cluster:** flag-conventions
**Evidence:** `internal/cli/config.go:25-42`
**Problem:** configShowCmd emits YAML unconditionally and exposes no --output flag, but every other read-only command (status, releases list/show, describe node/addon, addon list/verify, doctor) accepts --output=text|json. A user piping config show through jq cannot.
**Fix:** Add --output=text|json to configShowCmd. Default stays YAML (text), but --output=json emits the same redacted config as JSON via json.Marshal(redactConfig(cfg)). Document the JSON shape in docs/cli/json-schema.md. This is a pure addition — existing scripts continue to work.
**Effort:** hours

##### `ux:e7db1220:flag-completion-inconsistent` — flag completion inconsistent

**Status:** not started
**Severity:** suggestion
**Cluster:** flag-conventions
**Evidence:** `internal/cli/releases.go:75-80`
**Problem:** --channel on 'releases list' takes one of two literal strings (stable, all) but does not register a flag-completion function. Sibling commands that take a small enumeration (destroyCmd's --only at internal/cli/destroy.go:L177) DO register RegisterFlagCompletionFunc so shell tab-completion offers the values. Inconsistent within the surface.
**Fix:** Register a flag-completion function on --channel that returns []string{channelStable, channelAll}, mirroring the destroy --only pattern. Adds two lines, zero behavior change at the CLI surface.
**Effort:** hours

##### `ux:0d318f5c:flag-no-default-in-help` — flag no default in help

**Status:** not started
**Severity:** suggestion
**Cluster:** flag-conventions
**Evidence:** `internal/cli/root.go:251-252`
**Problem:** --log-format default in --help displays as empty because root.go L252 explicitly overwrites Lookup(flagLogFormat).DefValue = "" to hide the auto-switch behavior. Users running --help cannot see that 'text' is the default for TTY use; the help text describes the auto-switch in prose instead. This is an intentional hide of the field, but the consequence is that the help text is the only signal — a typo on the auto-switch logic could silently change behavior with no help-text trace.
**Fix:** Either restore the default to 'text' in --help so the auto-switch is documented as an override, or pin the override in a comment explaining that L252 is load-bearing for the TTY-vs-pipe contract. Currently it reads like a stylistic tweak rather than a contract anchor.
**Effort:** hours

##### `ux:0f076161:exit-taxonomy-not-published` — exit taxonomy not published

**Status:** not started
**Severity:** minor
**Cluster:** exit-codes — seam→`audit-errors`
**Evidence:** `internal/cli/destroy.go:204-227`
**Problem:** destroy returns errtypes.ConfigError (exit 2) for what are really --target/--only argument errors. Per the taxonomy in docs/cli/exit-codes.md, 64 (EX_USAGE) is the BSD code for command-line usage error and is what UsageError maps to. The destroy command does use UsageError for some checks (the index-out-of-range ones at L113-L128) but returns ConfigError for the --target-without-confirm-cluster check at L205 and for --dry-run incompatibility at L222. Same risk class (operator-supplied flags), different exit code.
**Fix:** Change ConfigError to UsageError at internal/cli/destroy.go:L205 and L222 (and similar patterns in deploy.go:L59-L62 for --metrics-allow-network without --metrics-addr). All three are flag-combination violations that should exit 64 (EX_USAGE), not 2 (config). Update docs/cli/exit-codes.md to clarify the boundary between ConfigError (file/schema problem) and UsageError (flag combo problem).
**Effort:** hours

##### `ux:073d24ed:flag-shortcut-collision` — flag shortcut collision

**Status:** not started
**Severity:** minor
**Cluster:** flag-conventions
**Evidence:** `internal/cli/deploy.go:46-51` + 2 more
**Problem:** deployCmd uses --output-file (no shorthand, per CLAUDE.md policy — kubeconfigCmd does the same). But deployOutputFile defaults to 'okdctl.yaml' while debugBundleOutput defaults to '' (interpreted as a generated filename) and kubeconfigOutput defaults to '-' (stdout). Three sibling commands that use the same flag give three different semantics for the empty/default state. Inconsistent UX across siblings sharing a flag.
**Fix:** Document a per-command default-value convention for --output-file in CLAUDE.md §flag-naming-convention: '-' for stdout-by-default (kubeconfig), '' for auto-generated (debug-bundle), and a literal default file for deploy. Currently a reader of okdctl deploy --help cannot tell whether '' would create or overwrite. Consider unifying on '-' = stdout for all three and a separate --auto-name flag for debug-bundle.
**Effort:** hours

##### `ux:b3356305:help-no-example` — help no example

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/ux-b3356305-readme-usage
**Severity:** minor
**Cluster:** help-text — seam→`audit-documentation`
**Evidence:** `README.md:77-83`
**Problem:** README.md §usage shows a four-line summary block listing only deploy/destroy/update-ingress/doctor/--version. It omits all major user-visible commands the generated docs/cli/okdctl.md does include: addon, cleanup, completion, config, debug-bundle, describe, kubeconfig, releases, status. A reader looking at the README will not discover okdctl status (the cluster-summary command), okdctl debug-bundle (the troubleshooting bundle), or okdctl kubeconfig (the export command).
**Fix:** Expand README.md §usage to mirror the SEE ALSO section of docs/cli/okdctl.md — list every top-level command with a one-line description, or link explicitly: 'See docs/cli/okdctl.md for the full reference.' Currently the link is at L85-L86 but the truncated list above it suggests those are the only commands.
**Effort:** hours


#### audit-observability

##### `obs:fde34e0c:err-stringified` — err stringified

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/obs-fde34e0c-err-attr
**Severity:** minor
**Cluster:** field-stability
**Evidence:** `internal/cluster/k8s.go:61-61`
**Problem:** `c.logger.Debug("ignoring $KUBECONFIG env value", "reason", err.Error())` stringifies err before the slog sink sees it. RedactHandler's redactAny dispatch operates on typed values — once err is collapsed to a string, the handler cannot inspect the chain for a `Redacted() any` implementation or for *url.URL userinfo. The validateKubeconfigEnv error here is unlikely to carry a credential today, but the pattern is the failure mode CLAUDE.md §credentials-and-secrets warns against.
**Fix:** Replace `"reason", err.Error()` with `"err", err`. The structured attr lets RedactHandler.redactAny() dispatch on the typed value, and an `errors.As` consumer downstream can still walk the chain. Net 0 LOC.
**Effort:** hours

##### `obs:97cb8adf:span-no-start-end` — span no start end

**Status:** not started
**Severity:** suggestion
**Cluster:** span-retry-boundary
**Evidence:** `internal/system/exec.go:117-163`
**Problem:** WaitFor's log messages `"waiting"` (L117, L162) and `"ready"` (L133, L156) lack the `<subsystem>:` prefix used everywhere else in the repo — every other log site uses `dns:`, `haproxy:`, `apache:`, `tools:`, etc. The first arg of WaitFor IS `prefix` and IS exposed as a structured attr, but the message itself loses the grep-anchor. Additionally, the attr key `"for"` is a Go-reserved word and an ambiguous identifier compared to canonical `target`/`desc`.
**Fix:** Move the prefix into the message via concatenation: `logger.Info(prefix+": waiting", "target", description)`. Rename the attr key from `"for"` to `"target"` or `"desc"`. The change is mechanical and keeps the structured attrs intact while restoring the grep contract.
**Effort:** hours

##### `obs:366b3f2d:level-error-not-user-visible` — level error not user visible

**Status:** not started
**Severity:** suggestion
**Cluster:** level-discipline
**Evidence:** `internal/distribution/orchestrator.go:184-188`
**Problem:** Orchestrator logs a fatal step at Error (L185) and then returns the error to the caller, which propagates up to cli/root.go::execute() and is logged a second time via `tui.Error("command failed", ...)`. The double-log is somewhat intentional (orchestrator wants per-step trace; root wants summary) but yields two Error lines for one event in the user-facing log stream.
**Fix:** Demote the fatal branch to Warn (`o.logger.Warn("step: failed (fatal)", ...)`) so the orchestrator's per-step trace stays one level below the cli-layer's Error. Or accept the double-log as deliberate audit-trail and document the exception in CLAUDE.md so future contributors do not normalise to Warn.
**Effort:** hours

##### `obs:632c9087:any-on-volatile-type` — any on volatile type

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/obs-632c9087-slice-attr
**Severity:** suggestion
**Cluster:** field-stability
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:103-108`
**Problem:** `p.Log.Info("update-ingress: discovered controllers", "count", len(controllers), "controllers", strings.Join(descriptions, ", "))` constructs a descriptions slice via fmt.Sprintf in a loop at L103-L106 and joins them into a single comma-separated string before logging. The structured attr `controllers` carries a stringified blob rather than a slice — a consumer that wants to filter on one controller name has to substring-match instead of using slog.Group/slice iteration.
**Fix:** Pass the slice directly: `slog.Any("controllers", descriptions)` (or attach a typed struct slice). The slog JSON handler renders the slice as a JSON array, keeping the per-controller name available to downstream filters. Note: `count` is then redundant — len() reproduces it at log-rendering time, so drop it from the attrs.
**Effort:** hours

##### `obs:bbc23e42:handler-no-tty-switch` — handler no tty switch

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/obs-bbc23e42-stdout-doc
**Severity:** suggestion
**Cluster:** handler-setup
**Evidence:** `internal/tui/logger.go:41-48`
**Problem:** tui/logger.go::init() builds stdoutLogger and stderrLogger via `charmlog.New(os.Stdout)` / `charmlog.New(os.Stderr)`, but only stderrLogger is wrapped through RedactHandler via buildStderrSlog(). stdoutLogger is used exclusively for non-slog UI output (json/text marshalled directly to stdout, kubeconfig contents), not for structured log records. The invariant is correct today but undocumented — a future contributor adding `slog.New(stdoutLogger)` for, e.g., a `--log-to-stdout` flag, would silently bypass RedactHandler.
**Fix:** Add a one-line invariant comment above stdoutLogger's init: `// stdoutLogger backs direct charmlog Print* output (json text, kubeconfig). It is NOT routed through slog.Default — the slog → RedactHandler discipline is enforced exclusively via stderrSlog. Adding a slog.Logger that targets stdout requires wrapping with logutil.NewRedactHandler.` Doc-only change; locks the invariant against accidental bypass.
**Effort:** hours


#### audit-modernization

##### `mod:366b3f2d:use-slices-clone` — use slices clone

**Status:** not started
**Severity:** minor
**Cluster:** slices-maps
**Evidence:** `internal/distribution/orchestrator.go:110-112` + 3 more
**Problem:** Hand-rolled defensive slice copy with make+copy where slices.Clone (Go 1.21) is the one-liner. Pattern occurs in four canonical Snapshot/Names-style accessors that all return a copy to protect internal state.
**Fix:** Replace `out := make([]T, len(x)); copy(out, x); return out` with `return slices.Clone(x)`. The slices package is already imported in three of the four sites; orchestrator.go needs the import added.
**Effort:** hours

##### `mod:b38ec9cc:use-strings-lines` — use strings lines

**Status:** not started
**Severity:** minor
**Cluster:** slices-maps
**Evidence:** `internal/distribution/okd/install/workers.go:91-97`
**Problem:** Materialises strings.Split(out, newline) into a slice when only iterating to count non-blank lines. Go 1.24's strings.Lines is the iterator form; the repo already uses it in eight other sites.
**Fix:** Replace `for _, line := range strings.Split(out, "\n")` with `for line := range strings.Lines(out)`. Matches the dominant pattern in internal/platform/platform.go:L85, internal/distribution/okd/install/flux.go:L36, internal/distribution/okd/setup/tools.go:L287. Note strings.Lines retains the trailing newline on each emitted line, so the TrimSpace check still works.
**Effort:** hours

##### `mod:6fc3d91e:use-strings-fieldsseq` — use strings fieldsseq

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/mod-6fc3d91e-fieldsseq
**Severity:** minor
**Cluster:** slices-maps
**Evidence:** `internal/platform/platform.go:122-129` + 1 more
**Problem:** Two iterate-once sites materialise strings.Fields(s) into a slice. Go 1.24's strings.FieldsSeq is the iterator form — no slice allocation, same loop body.
**Fix:** Replace `for _, x := range strings.Fields(s)` with `for x := range strings.FieldsSeq(s)` at internal/platform/platform.go:L122 and internal/runlock/runlock.go:L96.
**Effort:** hours

##### `mod:9d79b841:use-strings-cut` — use strings cut

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/mod-9d79b841-cut
**Severity:** suggestion
**Cluster:** slices-maps
**Evidence:** `internal/distribution/okd/setup/coreos.go:59-68` + 4 more
**Problem:** Five sites use strings.SplitN(s, sep, 2) + len-check + indexed access where strings.Cut (Go 1.18) returns (before, after, found) directly. The repo already uses strings.Cut in nine other locations.
**Fix:** Replace `parts := strings.SplitN(s, sep, 2); if len(parts) == 2 { ... parts[0] ... parts[1] }` with `before, after, ok := strings.Cut(s, sep); if ok { ... }`. Matches the dominant pattern in internal/platform/platform.go:L90, internal/cli/kubeconfig.go:L136, internal/system/fs.go:L83.
**Effort:** hours


#### audit-code-smells

##### `smell:0f076161:stringly-typed-enum` — stringly typed enum

**Status:** not started
**Severity:** minor
**Cluster:** magic-strings
**Evidence:** `internal/cli/destroy.go:109-130`
**Problem:** validateDestroyTargets re-derives role identity from a regex capture and switches on raw bootstrap/master/worker strings even though the canonical phase.NodeRole typed enum carries those exact literals. Adjacent code in the same file already uses destroyScope (phase-aligned typed values); the role compare drops back to bare strings.
**Fix:** Replace the bare-string switch with `switch phase.NodeRole(nodeType)` and `case phase.RoleBootstrap, phase.RoleMaster, phase.RoleWorker`. destroyTargetRE already constrains the captured value to the three role literals, so the conversion is lossless. Net change is three case-label updates; phase is already imported on file (L17).
**Effort:** hours

##### `smell:48688e63:bool-should-be-3state` — bool should be 3state

**Status:** not started
**Severity:** suggestion
**Cluster:** bool-should-be-enum
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:470-503`
**Problem:** probeVMEnumeration returns bool but the doc-comment names three distinct semantic outcomes (found in pvesh list, not in pvesh list, probe could not run treat as found). The boolean collapses two true cases that mean opposite things and forces every caller to read the doc to understand which branch they are in.
**Fix:** Introduce a vmEnumerationState int-iota with enumYes, enumNo, enumProbeSkipped constants. The single caller in Provision (L254-L263) currently uses `if vmidEnumerable` for log suppression; branching on the typed enum lets that code distinguish probe-confirmed-visible from probe-skipped-default-visible, which a future caller may need for retry decisions.
**Effort:** hours

##### `smell:820b96c9:stringly-typed-enum` — stringly typed enum

**Status:** not started
**Severity:** suggestion
**Cluster:** magic-strings
**Evidence:** `internal/addon/catalog/secretstore/providers.go:27-52`
**Problem:** Provider selection routes through bare-string constants (providerOnepassword, providerVault, providerBitwarden) keyed in map[string]provider with no typed enum. resolveProvider returns the resolved name as string, and the error path emits the unrecognised name from settings. A typed ProviderKind would make Settings.Provider, the providers map, and resolveProvider's second return value share one identity.
**Fix:** Define `type ProviderKind string` plus a ProviderKind field on Settings; key the providers map on ProviderKind; have resolveProvider return (provider, ProviderKind). The cleanup.Kind / bundleStatus / bundleCategory / VMState / NodeRole pattern in this repo is the counter-example. Three values today; second addon (after flux) where the same shape recurs.
**Effort:** hours

##### `smell:04f0e35f:abstraction-single-caller` — abstraction single caller

**Status:** not started
**Severity:** suggestion
**Cluster:** helper-package-no-value
**Evidence:** `internal/system/zeroize.go:1-9`
**Problem:** system.ZeroBytes is a 1-line wrapper around stdlib clear(b) with one production caller (internal/distribution/okd/setup/ignition.go::GenerateInstallConfig). The doc-comment positions it as the canonical complement to ProxmoxCredentials.Zeroize but adds no semantic gain over the stdlib builtin; defer clear(pullSecret) is shorter and self-evident.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `smell:cf43073b:enum-ad-hoc` — enum ad hoc

**Status:** not started
**Severity:** suggestion
**Cluster:** magic-strings
**Evidence:** `internal/config/types.go:1-32`
**Problem:** DistributionType and ProviderType are typed string enums with exactly one value each (DistributionOKD, ProviderProxmox). SupportedDistributions and SupportedProviders return one-element slices. The single-value enums look like premature abstraction in a strict reading but match the shape needed once a second distribution or provider lands.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `smell:262af6e4:abstraction-single-caller` — abstraction single caller

**Status:** not started
**Severity:** suggestion
**Cluster:** helper-package-no-value
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:24-56`
**Problem:** cleanup.Kind defines five values (Full, WorkOnly, WebOnly, HAProxyOnly, TerraformOnly) plus ValidKinds/KindStrings/IsValid/Validate helpers, but only Full and WorkOnly are referenced outside the package. WebOnly, HAProxyOnly, and TerraformOnly carry no callers; the switch in cleanupSteps covers them only to maintain the full enum shape.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `smell:6424733c:abstraction-single-caller` — abstraction single caller

**Status:** not started
**Severity:** suggestion
**Cluster:** helper-package-no-value
**Evidence:** `internal/cli/helpers.go:188-194`
**Problem:** tuiReporter is a 6-LOC factory whose doc says it exists so domain code can call a callback that captures the command ctx without taking an internal/tui dependency. The file already imports internal/tui at L25, and the only caller (executeFullDeployment, same file) immediately threads the returned closure into okd.WithProgressReporter(tuiReporter(ctx)). The doc rationale is false and the named abstraction adds no semantic gain over inlining.
**Fix:** Inline at the single call site: `provOpts = append(provOpts, okd.WithProgressReporter(func(desc string) func() { return tui.StartSpinner(ctx, desc) }))` — removes the misleading doc comment and ~5 LOC. Alternative: rewrite the doc to drop the false dependency-isolation claim and explain the real reason (named, testable indirection).
**Effort:** hours

##### `smell:c19ee328:magic-strings` — magic strings

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/smell-c19ee328-bin-consts
**Severity:** minor
**Cluster:** magic-strings
**Evidence:** `internal/distribution/okd/setup/steps.go:89-94`
**Problem:** setupBaseSteps lists the OKD tool trio as bare-string literals []string{openshift-install, oc, kubectl} even though openshiftInstallBin is already declared as a package-level constant in phase.go:L35 for the same binary. The other two names (oc, kubectl) are referenced as literal strings dozens of times across setup/install/postinstall/cleanup; a typo here equals a silent skip in the AlreadyDone guard.
**Fix:** Declare `const ocBin = "oc"` and `const kubectlBin = "kubectl"` alongside the existing openshiftInstallBin in setup/phase.go (or, since all three binaries cross package boundaries, hoist to internal/distribution/okd/phase). Replace the bare literals here and at the related sites. Net ~4 LOC added for the constants; removes pattern-wide silent-typo risk.
**Effort:** hours

##### `smell:efff8856:enum-ad-hoc` — enum ad hoc

**Status:** not started
**Severity:** suggestion
**Cluster:** magic-strings
**Evidence:** `internal/addon/catalog/flux/flux.go:267-278`
**Problem:** DefaultSettings sets SettingProvider to the literal flux. internal/tui/wizard/steps/defaults.go:L71 already exports DefaultGitOpsProvider = flux as a wizard default; the flux addon (catalog) and the wizard (steps/) each hardcode the same provider literal independently. Two unconnected sites spelling the same enum value is the canonical enum-ad-hoc shape.
**Fix:** Add `const ProviderID = "flux"` in flux/flux.go (the addon owns its identity), reuse from DefaultSettings, and have wizard/steps/defaults.go reference the same constant. Avoids the silent-drift class where the wizard default and the addon default diverge. Net 0-2 LOC.
**Effort:** hours


#### audit-dependencies

##### `dep:e8f33f61:maint-single-bus-go-proxmox` — maint single bus go proxmox

**Status:** not started
**Severity:** major
**Cluster:** maintenance-signal
**Evidence:** `go.mod:12-12`
**Problem:** github.com/luthermonson/go-proxmox v0.5.1 has bus factor 1 (luthermonson 191 commits vs next-best 20). v0.x semver allows breaking changes on minor bumps; sole Proxmox API path for the wizard. Re-confirm per CLAUDE.md §dependencies, do not propose ripping out.
**Fix:** Re-confirm only — do NOT propose removal. Track upstream releases; bump on each. Fallback documented: ~200 LOC REST-only rewrite under internal/proxmox using net/http + the documented Proxmox REST API. Treat any 3-month upstream-silence window as the trigger to start the rewrite.
**Effort:** hours

##### `dep:33ef32bf:yaml-prod-binary-engines` — yaml prod binary engines

**Status:** not started
**Severity:** minor
**Cluster:** duplicate-engine
**Evidence:** `go.mod:19-55`
**Problem:** Production binary ships two YAML engines: sigs.k8s.io/yaml v1.6.0 (direct) plus go.yaml.in/yaml/v2 v2.4.3 (transitive via k8s.io/apimachinery). Smaller than the previously documented quad — gopkg.in/yaml.v3 is now test-only and go.yaml.in/yaml/v3 ships only in cmd/okdctl-gen-docs (build-tool).
**Fix:** Re-confirm acceptance. Both engines are dictated by upstream (sigs.k8s.io/yaml needs go.yaml.in/yaml/v2 for the k8s schemas; sigs.k8s.io/yaml is the direct entry point). Update CLAUDE.md §dependencies to record the corrected two-engine prod baseline so the tripwire fires only when a fifth engine lands or the direct dep changes shape. Do not attempt consolidation.
**Effort:** hours

##### `dep:33ef32bf:gorilla-ws-version-floor` — gorilla ws version floor

**Status:** not started
**Severity:** minor
**Cluster:** justified-version-floor
**Evidence:** `go.mod:39-39`
**Problem:** gorilla/websocket v1.4.2 (from 2020) lags 1.5.3 latest by three minor releases. Pulled transitively via go-proxmox; okdctl does NOT reach it (wizard uses REST discovery only). Per CLAUDE.md §dependencies the version is upstream-locked, but the floor diverges enough that govulncheck silence is the only thing standing between us and a stale CVE surface.
**Fix:** Hold — no action. CLAUDE.md §dependencies pre-flags this: 'safe to keep until go-proxmox migrates to coder/websocket, at which point take the bump without local code changes.' File an upstream issue / open a tracking PR on go-proxmox advocating the migration so the freeze ends. Do not bump unilaterally — go.mod minimum-version semantics mean a local bump may not change what go-proxmox sees.
**Effort:** hours

##### `dep:33ef32bf:json-iterator-stale` — json iterator stale

**Status:** not started
**Severity:** suggestion
**Cluster:** maintenance-signal
**Evidence:** `go.mod:42-42`
**Problem:** json-iterator/go v1.1.12 (last release Sep 2021, last commit May 2024) is effectively in maintenance freeze. Pulled transitively via k8s.io/apimachinery + sigs.k8s.io/structured-merge-diff/v6 — okdctl does not import it directly. Upstream k8s is gradually migrating to encoding/json + cbor, but the dep persists in the binary.
**Fix:** No action — track k8s upstream's migration off json-iterator. Removal will come naturally on a k8s.io/api+apimachinery bump (likely k8s 1.32+ as the CBOR path matures). Do not propose a Go-side replace; the dep is k8s-mandated.
**Effort:** hours

##### `dep:33ef32bf:modern-go-concurrent-abandoned` — modern go concurrent abandoned

**Status:** not started
**Severity:** suggestion
**Cluster:** maintenance-signal
**Evidence:** `go.mod:47-48`
**Problem:** modern-go/concurrent (last commit Aug 2019, 6.5y stale) and modern-go/reflect2 (last release 2021, last commit Mar 2025) are abandoned-but-shipping. Both transitively required by json-iterator/go → k8s.io/apimachinery. License Apache-2.0 (clean). Pure tripwire: if upstream go-namespace ever lapses, k8s ecosystem breaks for everyone.
**Fix:** No action; can't dislodge transitively. Removal comes when json-iterator/go is removed (see dep:33ef32bf:json-iterator-stale). Record the tripwire so a future GitHub-namespace-vacate event triggers an immediate k8s.io bump.
**Effort:** hours

##### `dep:33ef32bf:claude-md-yaml-quad-outdated` — claude md yaml quad outdated

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/dep-33ef32bf-yaml-engines
**Severity:** suggestion
**Cluster:** duplicate-engine
**Evidence:** `CLAUDE.md:1-1`
**Problem:** CLAUDE.md §dependencies still documents 'Four YAML engines' but `go list -deps -test=false ./cmd/okdctl` shows only two shipping in the binary (sigs.k8s.io/yaml, go.yaml.in/yaml/v2). go.yaml.in/yaml/v3 is build-tool-only (cmd/okdctl-gen-docs via spf13/cobra/doc); gopkg.in/yaml.v3 is test-only. Documentation drifted from current code state.
**Fix:** Update CLAUDE.md §dependencies to reflect the two-engine production-binary baseline: 'sigs.k8s.io/yaml + go.yaml.in/yaml/v2 ship in cmd/okdctl. go.yaml.in/yaml/v3 is build-tool only (cmd/okdctl-gen-docs). gopkg.in/yaml.v3 is test-only.' Tripwire still applies: do not add a fifth engine.
**Effort:** hours

##### `dep:33ef32bf:claude-md-godotenv-stale` — claude md godotenv stale

**Status:** not started
**Severity:** suggestion
**Cluster:** maintenance-signal
**Evidence:** `CLAUDE.md:1-1`
**Problem:** CLAUDE.md §dependencies notes 'github.com/joho/godotenv ships its license file as LICENCE (British spelling) — a valid MIT license; SBOM scanners that grep for LICENSE will flag a false positive.' godotenv is no longer in go.mod or go.sum — the note is documenting a non-existent dep.
**Fix:** Remove the godotenv LICENCE note from CLAUDE.md §dependencies; it documents a dep no longer in the tree. Keep one line elsewhere (e.g. a deprecation log section) if the false-positive precedent is worth retaining for the next dep that ships its license under a non-LICENSE name.
**Effort:** hours

##### `dep:b803fcb7:tflint-pin-format` — tflint pin format

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/dep-b803fcb7-tflint-pin
**Severity:** suggestion
**Cluster:** pin-stability
**Evidence:** `.github/workflows/ci.yml:103-103`
**Problem:** tflint setup uses `tflint_version: "v0.62.0"` (with leading 'v') while every other tool in the workflow uses the canonical format without leading v (`version: v2.12.2`, `terraform_version: "1.10.3"`). Cosmetic inconsistency, not a correctness issue — terraform-linters/setup-tflint accepts both forms.
**Fix:** Normalize to `tflint_version: "0.62.0"` to match the in-repo pattern for terraform_version. Cosmetic only — both forms are accepted by setup-tflint.
**Effort:** hours

##### `dep:660d83a5:charm-log-vs-slog-policy` — charm log vs slog policy

**Status:** not started
**Severity:** suggestion
**Cluster:** duplicate-engine — seam→`audit-modernization`
**Evidence:** `internal/tui/logger.go:12-196`
**Problem:** charm.land/log/v2 is a third-party logger wrapping a slog.Handler facade, used purely for colored level styling on stderr. log/slog (stdlib since 1.21) plus a small lipgloss-styled handler could replicate this; the dep adds maintenance debt and a second logging engine alongside the stdlib slog default. CLAUDE.md does NOT pre-clear charm.land/log/v2 the way SKILL.md §5 pre-clears charm.land/bubbletea (which carries 'Charm libs — intentional UI stack').
**Fix:** Optional — replace charm.land/log/v2 with a hand-rolled slog.Handler that renders levels through lipgloss styles. ~50 LOC swap for the level-styling logic, removes one transitive subtree (go-logfmt, etc.). If the charm-ecosystem coherence is valued, keep as-is. SKIM ONLY: seam #10 says 'drop dep, stdlib covers it' = modernization-owned. Cross-listed here because the CHOICE of charm.land/log is a dependency-policy decision (charm coherence vs minimization), not a Go-version-driven migration. Defer to audit-modernization if it flags the same site.
**Effort:** hours

##### `dep:33ef32bf:diskfs-transitive-weight` — diskfs transitive weight

**Status:** not started
**Severity:** minor
**Cluster:** transitive-weight
**Evidence:** `go.mod:34-34`
**Problem:** github.com/diskfs/go-diskfs v1.9.3 ships in the production binary purely because go-proxmox imports it (likely for ISO9660 manipulation in the Proxmox iso upload flow). okdctl does not call go-diskfs directly, but the dep pulls anchore/go-lzo + pkg/xattr + ulikunitz/xz + pierrec/lz4/v4 + klauspost/compress into the binary — five compression libs for a code path okdctl never reaches.
**Fix:** Re-confirm — keep. Pulled by go-proxmox; the only ways to drop it are (a) the documented REST-only rewrite (see dep:e8f33f61:maint-single-bus-go-proxmox), (b) a go-proxmox upstream PR splitting the ISO9660 helper into a separate module. Document the transitive-weight cost (~5 compression libs, +diskfs surface area) as a tally toward the rewrite trigger so future-self has the receipt.
**Effort:** hours


#### audit-documentation


#### audit-tests



## Completed

Completed items live in [`docs/roadmap/completed-archive.md`](docs/roadmap/completed-archive.md). Grep there for the canonical "is dep X done?" lookup. The previous in-line pointer index (144 entries, mirroring archive contents) was removed on 2026-05-09 to keep `roadmap.md` focused on active work.

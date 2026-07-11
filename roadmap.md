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

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2s-steps
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

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2e-executor
**Severity:** suggestion
**Cluster:** io-handling — seam→`audit-api-design` — related: `sub:97cb8adf:no-cancel-func`
**Evidence:** `internal/distribution/okd/install/monitor.go:25-44`
**Problem:** defaultStartMonitorCmd reaches past the canonical executor.Executor to call osExec.CommandContext directly because the caller needs Start+Wait independence (the parent loop ticks at csrApprovalInterval while openshift-install runs for 30-60min). The env filter, cmd.Cancel SIGTERM, and WaitDelay are re-implemented inline; the embedded comment on L24 even acknowledges that this duplicates the canonical pattern. There is no Executor method that returns a done-channel for background subprocesses, so the duplication is structural rather than careless.
**Fix:** Add an Executor method along the lines of `StartStreamed(ctx, name, args...) (done <-chan error, err error)` that wires Start + a Wait-into-channel goroutine while sharing buildEnv / Cancel / WaitDelay from the existing run pattern. Then defaultStartMonitorCmd shrinks to a single call: `return p.Exec.StartStreamed(ctx, "openshift-install", "wait-for", "install-complete", "--dir", clusterDir, "--log-level=debug")`. Net LOC delta ~-10 at this site (+15 in executor) for a real interface gain. The new method is symmetric with RunStreamed.
**Effort:** hours

#### audit-state-and-recovery

##### `state:0f076161:destroy-no-cluster-confirm-without-yes` — destroy no cluster confirm without yes

**Status:** in review — PR #908
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

**Status:** in review — PR #907
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

**Status:** in review — PR #908
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

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2p-proxmox
**Severity:** suggestion
**Cluster:** cancellation-identity — seam→`audit-concurrency`
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:232-242`
**Problem:** On ctx-cancelled terraform apply: fmt.Errorf('terraform apply interrupted: %w', errors.Join(ctx.Err(), applyErr)). errors.Is(err, context.Canceled) walks correctly because errors.Join exposes Unwrap() []error. But the outer fmt.Errorf is the only object visible to errors.As, so callers cannot recover a typed *errtypes.ClusterError here (the non-cancel branch at L241 returns one). Mixed-shape return is a smell — callers must branch on errors.Is before errors.As to handle both shapes.
**Fix:** The bare wrap is intentional and load-bearing — internal/cli/root.go::signalExitCode walks the chain via errors.Is(err, context.Canceled) before exitCodeFor runs, mapping SIGINT→130. The pattern matches the install/monitor.go canon. Add an inline comment (matching install/monitor.go:L65-68) so reviewers do not wrap it in ClusterError and break the SIGINT→130 mapping.
**Effort:** hours

##### `err:b38ec9cc:lock-hint-exit-code-flip` — lock hint exit code flip

**Status:** in review — PR #907
**Severity:** major
**Cluster:** sentinel-vs-typed — seam→`audit-cli-ux`
**Evidence:** `internal/distribution/okd/install/workers.go:39-71` + 3 more
**Problem:** errors.Join(hint, wrapped) where hint is *errtypes.ConfigError and wrapped is *errtypes.ClusterError works because errors.Join's Unwrap() []error lets errors.As walk to both. But exitCodeFor walks the declaration order: ConfigError → 2, ClusterError → 4. errors.As returns on the first match. The hint matches ConfigError → exit 2; the ClusterError → exit 4 mapping is unreachable. terraform init failure → ClusterError → 4 normally; same failure under a stale lock → ConfigError → 2. Operators scripting against exit codes will see flaky behaviour.
**Fix:** Either (a) downgrade LockHint to return a *string* that the wrapped ClusterError appends to Msg (exit code stays uniform), or (b) restructure to wrap the hint message inside the ClusterError's Msg with the underlying err: `&errtypes.ClusterError{Msg: msg + '; ' + hintMsg, Err: err}` so only one typed error is in the chain. (c) Or accept the exit-2-for-locked-state mapping and document it in cli/root.go's exit-code table. Today the chain produces inconsistent exit codes silently.
**Effort:** hours

##### `err:f55b9c27:envfile-loadonce-no-sentinel` — envfile loadonce no sentinel

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2m-misc
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

**Status:** in review — PR #908
**Severity:** minor
**Cluster:** option-consistency
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:68-83`
**Problem:** cleanup.Options is the only phase Options struct without a NewOptions(cfg,projectRoot) constructor — setup, install, postinstall, destroy all expose NewOptions for the same purpose. The three call sites that build cleanup.Options (cli/cleanup.go:115, okd/okd.go:122, destroy/steps.go:131) all hand-assemble BaseOptions and Kind separately, duplicating phase.GetTerraformEnv(cfg) plumbing.
**Fix:** Add func NewOptions(cfg *config.Config, projectRoot string, kind Kind) Options to internal/distribution/okd/cleanup/cleanup.go that sets BaseOptions{ProjectRoot,WorkDir,TerraformEnv} and Kind, mirroring destroy.NewOptions. Caller code (internal/cli/cleanup.go:115, internal/distribution/okd/okd.go:122, internal/distribution/okd/destroy/steps.go:131) replaces the inline literal with the call. Caller-specific overrides (HTTPServerRoot, HAProxyConfig, VIP, BinDir) still set field-by-field after construction.
**Effort:** hours

##### `api:d6b325cb:pkg-types-direction` — pkg types direction

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2p-proxmox
**Severity:** minor
**Cluster:** package-boundary
**Evidence:** `internal/infrastructure/proxmox/types.go:1-49`
**Problem:** internal/infrastructure/proxmox imports internal/distribution/okd/phase for NodeRole, VMState, and the RemoteISOParams/PveshRun helpers. Per CLAUDE.md the dependency direction should be cli → distribution/okd → infrastructure/proxmox. The current shape inverts that on shared domain types: proxmox is parametrised by phase's NodeRole/VMState, and phase/pvesh.go contains Proxmox-specific subprocess plumbing (PveshRun, RemoteISOParams) that semantically belongs to the proxmox package. The package doc at vmstate.go:6 acknowledges the inversion.
**Fix:** Extract NodeRole/VMState/RemoteISOParams + the Proxmox SSH/pvesh subprocess primitives into a new internal/cluster/types or internal/infrastructure/proxmox/types-only sub-package that both phase and proxmox can import without forming a cycle. Today's compromise (cluster-domain enums in 'phase') works but every new shared concept widens the wrong-direction surface. Verify intent on the roadmap before refactor — multi-provider expansion would justify it, otherwise the cycle-break is the right call.
**Effort:** hours

##### `api:262af6e4:opt-execute-receiver-unused` — opt execute receiver unused

**Status:** in review — PR #908
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

**Status:** not started
**Severity:** suggestion
**Cluster:** option-consistency
**Evidence:** `internal/system/exec.go:89-102`
**Problem:** system.WaitForOptions is the only configuration-via-struct in the repo's runtime/operation surface — every other 'configure X at call time' surface either uses inline arguments or a struct-of-options *passed to a constructor* (terraform.PlanOptions, proxmox.ProvisionOptions). WaitFor's signature takes (ctx, prefix, description, check, opts) — the options struct is materialized once via DefaultWaitForOptions() + field assignment, then passed. Consider whether WaitFor should follow the functional-options pattern of the constructors instead.
**Fix:** Choice: (a) keep WaitForOptions as the canonical 'per-call config struct' (and document it as the deliberate choice for one-shot configurable operations); (b) rewrite to func WaitFor(ctx, prefix, desc, check, opts ...WaitForOption) for full symmetry with constructor options. Two callers exist today (kubectl.go OcPollOutputInterval, exec_test.go); the WaitForWithTimeout wrapper exists precisely because the struct shape is verbose. Functional options would simplify the wrapper away.
**Effort:** hours

##### `api:7b2829bb:opt-with-inherited-env-noarg` — opt with inherited env noarg

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2e-executor
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

**Status:** in review — PR #908
**Severity:** minor
**Cluster:** option-consistency
**Evidence:** `internal/distribution/okd/okd.go:181-222`
**Problem:** okd.DestroyOpts and destroy.Options carry nearly the same field set (AutoApprove, RemovePackages, KeepISOs, SkipTerraform, SkipCleanup, SkipFirewall, TerraformTargets). okd.Provisioner.Destroy unpacks DestroyOpts field-by-field into destroy.Options. The two structs differ by destroy.Options embedding phase.BaseOptions and adding CleanupKind/Parallelism. The duplication forces every new destroy flag to land in two places.
**Fix:** Pick one shape: (a) okd.Provisioner.Destroy takes destroy.Options directly (collapses DestroyOpts entirely); (b) DestroyOpts becomes the CLI-facing struct and the provisioner injects the BaseOptions+CleanupKind defaults internally before calling destroy.Phase.Execute. Option (a) is cleaner — the CLI builds destroy.Options once via destroy.NewOptions. The trade-off is exposing destroy.Options' embedded BaseOptions through the CLI surface; if BaseOptions internals shouldn't bleed into the CLI, prefer (b).
**Effort:** hours

##### `api:ae5b624c:iface-csr-approver-positive` — iface csr approver positive

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2c-cluster
**Severity:** suggestion
**Cluster:** interface-location
**Evidence:** `internal/distribution/okd/install/monitor.go:78-83`
**Problem:** install.csrApprover is a one-method interface (ApprovePendingCSRs) defined at the consumer side (install package) where cluster.Client is the real implementation. This is the correct shape per Go proverb 'Accept interfaces, return structs'. The interface is unexported (csrApprover), which is the right call for a single-package consumer. Logged as a positive counter-example for the api-design audit — every interface that abstracts a cluster operation should follow this shape.
**Fix:** No action — this is the canonical pattern. Document explicitly in cluster.Client's package doc that 'consumers may define their own narrow interfaces to test against; cluster.Client returns concrete types, never interfaces'. Future cluster-method additions (e.g. Client.GetNodes) should leave consumer-side interface definition to the consumer.
**Effort:** hours

#### audit-cli-ux

##### `ux:fd2125dd:concept-named-twice` — concept named twice

**Status:** in review — PR #896
**Severity:** minor
**Cluster:** verb-noun
**Evidence:** `internal/cli/addon.go:28-31` + 1 more
**Problem:** Parent group nouns are inconsistently singular vs plural across the command tree: 'addon list/install/uninstall/verify' (singular) but 'releases list/show' (plural). Internal references compound the drift — addon.go uses 'addon list/verify' in messages while releases.go uses 'releases list'. Pick one: kubectl/oc convention is singular (kubectl get pod, kubectl describe service).
**Fix:** Rename releases → release for consistency with addon. The fix breaks anyone scripting okdctl releases list, but okdctl is pre-1.0 (per README §status) and the CHANGELOG can document the rename. Alternative: rename addon → addons. Either way, document the convention in CLAUDE.md so new groups follow it.
**Effort:** hours

##### `ux:0f076161:exit-taxonomy-not-published` — exit taxonomy not published

**Status:** in review — PR #908
**Severity:** minor
**Cluster:** exit-codes — seam→`audit-errors`
**Evidence:** `internal/cli/destroy.go:204-227`
**Problem:** destroy returns errtypes.ConfigError (exit 2) for what are really --target/--only argument errors. Per the taxonomy in docs/cli/exit-codes.md, 64 (EX_USAGE) is the BSD code for command-line usage error and is what UsageError maps to. The destroy command does use UsageError for some checks (the index-out-of-range ones at L113-L128) but returns ConfigError for the --target-without-confirm-cluster check at L205 and for --dry-run incompatibility at L222. Same risk class (operator-supplied flags), different exit code.
**Fix:** Change ConfigError to UsageError at internal/cli/destroy.go:L205 and L222 (and similar patterns in deploy.go:L59-L62 for --metrics-allow-network without --metrics-addr). All three are flag-combination violations that should exit 64 (EX_USAGE), not 2 (config). Update docs/cli/exit-codes.md to clarify the boundary between ConfigError (file/schema problem) and UsageError (flag combo problem).
**Effort:** hours

##### `ux:073d24ed:flag-shortcut-collision` — flag shortcut collision

**Status:** in review — PR #896
**Severity:** minor
**Cluster:** flag-conventions
**Evidence:** `internal/cli/deploy.go:46-51` + 2 more
**Problem:** deployCmd uses --output-file (no shorthand, per CLAUDE.md policy — kubeconfigCmd does the same). But deployOutputFile defaults to 'okdctl.yaml' while debugBundleOutput defaults to '' (interpreted as a generated filename) and kubeconfigOutput defaults to '-' (stdout). Three sibling commands that use the same flag give three different semantics for the empty/default state. Inconsistent UX across siblings sharing a flag.
**Fix:** Document a per-command default-value convention for --output-file in CLAUDE.md §flag-naming-convention: '-' for stdout-by-default (kubeconfig), '' for auto-generated (debug-bundle), and a literal default file for deploy. Currently a reader of okdctl deploy --help cannot tell whether '' would create or overwrite. Consider unifying on '-' = stdout for all three and a separate --auto-name flag for debug-bundle.
**Effort:** hours

#### audit-observability

#### audit-modernization

##### `mod:b38ec9cc:use-strings-lines` — use strings lines

**Status:** in review — PR #907
**Severity:** minor
**Cluster:** slices-maps
**Evidence:** `internal/distribution/okd/install/workers.go:91-97`
**Problem:** Materialises strings.Split(out, newline) into a slice when only iterating to count non-blank lines. Go 1.24's strings.Lines is the iterator form; the repo already uses it in eight other sites.
**Fix:** Replace `for _, line := range strings.Split(out, "\n")` with `for line := range strings.Lines(out)`. Matches the dominant pattern in internal/platform/platform.go:L85, internal/distribution/okd/install/flux.go:L36, internal/distribution/okd/setup/tools.go:L287. Note strings.Lines retains the trailing newline on each emitted line, so the TrimSpace check still works.
**Effort:** hours

#### audit-code-smells

##### `smell:0f076161:stringly-typed-enum` — stringly typed enum

**Status:** in review — PR #908
**Severity:** minor
**Cluster:** magic-strings
**Evidence:** `internal/cli/destroy.go:109-130`
**Problem:** validateDestroyTargets re-derives role identity from a regex capture and switches on raw bootstrap/master/worker strings even though the canonical phase.NodeRole typed enum carries those exact literals. Adjacent code in the same file already uses destroyScope (phase-aligned typed values); the role compare drops back to bare strings.
**Fix:** Replace the bare-string switch with `switch phase.NodeRole(nodeType)` and `case phase.RoleBootstrap, phase.RoleMaster, phase.RoleWorker`. destroyTargetRE already constrains the captured value to the three role literals, so the conversion is lossless. Net change is three case-label updates; phase is already imported on file (L17).
**Effort:** hours

##### `smell:48688e63:bool-should-be-3state` — bool should be 3state

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2p-proxmox
**Severity:** suggestion
**Cluster:** bool-should-be-enum
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:470-503`
**Problem:** probeVMEnumeration returns bool but the doc-comment names three distinct semantic outcomes (found in pvesh list, not in pvesh list, probe could not run treat as found). The boolean collapses two true cases that mean opposite things and forces every caller to read the doc to understand which branch they are in.
**Fix:** Introduce a vmEnumerationState int-iota with enumYes, enumNo, enumProbeSkipped constants. The single caller in Provision (L254-L263) currently uses `if vmidEnumerable` for log suppression; branching on the typed enum lets that code distinguish probe-confirmed-visible from probe-skipped-default-visible, which a future caller may need for retry decisions.
**Effort:** hours

##### `smell:262af6e4:abstraction-single-caller` — abstraction single caller

**Status:** in review — PR #908
**Severity:** suggestion
**Cluster:** helper-package-no-value
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:24-56`
**Problem:** cleanup.Kind defines five values (Full, WorkOnly, WebOnly, HAProxyOnly, TerraformOnly) plus ValidKinds/KindStrings/IsValid/Validate helpers, but only Full and WorkOnly are referenced outside the package. WebOnly, HAProxyOnly, and TerraformOnly carry no callers; the switch in cleanupSteps covers them only to maintain the full enum shape.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

#### audit-dependencies

##### `dep:e8f33f61:maint-single-bus-go-proxmox` — maint single bus go proxmox

**Status:** in review — PR #895
**Severity:** major
**Cluster:** maintenance-signal
**Evidence:** `go.mod:12-12`
**Problem:** github.com/luthermonson/go-proxmox v0.5.1 has bus factor 1 (luthermonson 191 commits vs next-best 20). v0.x semver allows breaking changes on minor bumps; sole Proxmox API path for the wizard. Re-confirm per CLAUDE.md §dependencies, do not propose ripping out.
**Fix:** Re-confirm only — do NOT propose removal. Track upstream releases; bump on each. Fallback documented: ~200 LOC REST-only rewrite under internal/proxmox using net/http + the documented Proxmox REST API. Treat any 3-month upstream-silence window as the trigger to start the rewrite.
**Effort:** hours

##### `dep:33ef32bf:yaml-prod-binary-engines` — yaml prod binary engines

**Status:** in review — PR #895
**Severity:** minor
**Cluster:** duplicate-engine
**Evidence:** `go.mod:19-55`
**Problem:** Production binary ships two YAML engines: sigs.k8s.io/yaml v1.6.0 (direct) plus go.yaml.in/yaml/v2 v2.4.3 (transitive via k8s.io/apimachinery). Smaller than the previously documented quad — gopkg.in/yaml.v3 is now test-only and go.yaml.in/yaml/v3 ships only in cmd/okdctl-gen-docs (build-tool).
**Fix:** Re-confirm acceptance. Both engines are dictated by upstream (sigs.k8s.io/yaml needs go.yaml.in/yaml/v2 for the k8s schemas; sigs.k8s.io/yaml is the direct entry point). Update CLAUDE.md §dependencies to record the corrected two-engine prod baseline so the tripwire fires only when a fifth engine lands or the direct dep changes shape. Do not attempt consolidation.
**Effort:** hours

##### `dep:33ef32bf:gorilla-ws-version-floor` — gorilla ws version floor

**Status:** in review — PR #895
**Severity:** minor
**Cluster:** justified-version-floor
**Evidence:** `go.mod:39-39`
**Problem:** gorilla/websocket v1.4.2 (from 2020) lags 1.5.3 latest by three minor releases. Pulled transitively via go-proxmox; okdctl does NOT reach it (wizard uses REST discovery only). Per CLAUDE.md §dependencies the version is upstream-locked, but the floor diverges enough that govulncheck silence is the only thing standing between us and a stale CVE surface.
**Fix:** Hold — no action. CLAUDE.md §dependencies pre-flags this: 'safe to keep until go-proxmox migrates to coder/websocket, at which point take the bump without local code changes.' File an upstream issue / open a tracking PR on go-proxmox advocating the migration so the freeze ends. Do not bump unilaterally — go.mod minimum-version semantics mean a local bump may not change what go-proxmox sees.
**Effort:** hours

##### `dep:33ef32bf:json-iterator-stale` — json iterator stale

**Status:** in review — PR #895
**Severity:** suggestion
**Cluster:** maintenance-signal
**Evidence:** `go.mod:42-42`
**Problem:** json-iterator/go v1.1.12 (last release Sep 2021, last commit May 2024) is effectively in maintenance freeze. Pulled transitively via k8s.io/apimachinery + sigs.k8s.io/structured-merge-diff/v6 — okdctl does not import it directly. Upstream k8s is gradually migrating to encoding/json + cbor, but the dep persists in the binary.
**Fix:** No action — track k8s upstream's migration off json-iterator. Removal will come naturally on a k8s.io/api+apimachinery bump (likely k8s 1.32+ as the CBOR path matures). Do not propose a Go-side replace; the dep is k8s-mandated.
**Effort:** hours

##### `dep:33ef32bf:modern-go-concurrent-abandoned` — modern go concurrent abandoned

**Status:** in review — PR #895
**Severity:** suggestion
**Cluster:** maintenance-signal
**Evidence:** `go.mod:47-48`
**Problem:** modern-go/concurrent (last commit Aug 2019, 6.5y stale) and modern-go/reflect2 (last release 2021, last commit Mar 2025) are abandoned-but-shipping. Both transitively required by json-iterator/go → k8s.io/apimachinery. License Apache-2.0 (clean). Pure tripwire: if upstream go-namespace ever lapses, k8s ecosystem breaks for everyone.
**Fix:** No action; can't dislodge transitively. Removal comes when json-iterator/go is removed (see dep:33ef32bf:json-iterator-stale). Record the tripwire so a future GitHub-namespace-vacate event triggers an immediate k8s.io bump.
**Effort:** hours

##### `dep:33ef32bf:claude-md-godotenv-stale` — claude md godotenv stale

**Status:** in review — PR #895
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

**Status:** in review — PR #895
**Severity:** suggestion
**Cluster:** duplicate-engine — seam→`audit-modernization`
**Evidence:** `internal/tui/logger.go:12-196`
**Problem:** charm.land/log/v2 is a third-party logger wrapping a slog.Handler facade, used purely for colored level styling on stderr. log/slog (stdlib since 1.21) plus a small lipgloss-styled handler could replicate this; the dep adds maintenance debt and a second logging engine alongside the stdlib slog default. CLAUDE.md does NOT pre-clear charm.land/log/v2 the way SKILL.md §5 pre-clears charm.land/bubbletea (which carries 'Charm libs — intentional UI stack').
**Fix:** Optional — replace charm.land/log/v2 with a hand-rolled slog.Handler that renders levels through lipgloss styles. ~50 LOC swap for the level-styling logic, removes one transitive subtree (go-logfmt, etc.). If the charm-ecosystem coherence is valued, keep as-is. SKIM ONLY: seam #10 says 'drop dep, stdlib covers it' = modernization-owned. Cross-listed here because the CHOICE of charm.land/log is a dependency-policy decision (charm coherence vs minimization), not a Go-version-driven migration. Defer to audit-modernization if it flags the same site.
**Effort:** hours

##### `dep:33ef32bf:diskfs-transitive-weight` — diskfs transitive weight

**Status:** in review — PR #895
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

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2e-executor
**Severity:** suggestion
**Cluster:** timeout-cancel
**Evidence:** `internal/platform/packages.go:60-90` + 2 more
**Problem:** dnf/apt-get install/remove/update run via RunCaptured with no ctx deadline and stdout discarded: a wedged mirror or stale repo metadata hangs the deploy indefinitely with zero visible progress. Cancel wiring exists (cmd.Cancel SIGTERM + WaitDelay 30s, signal-watched root ctx), so Ctrl-C recovers — the gap is deadline + operator visibility only.
**Fix:** Wrap package-manager invocations in context.WithTimeout (generous, e.g. 15-20 min) mirroring ocExtractTimeout in release_extract.go, or stream output via the executor so a stall is at least visible. Risk: an aggressive timeout flakes slow mirrors — keep it generous.
**Effort:** hours

##### `sub:4c092fce:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2e-executor
**Severity:** minor
**Cluster:** io-handling — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/infrastructure/terraform/terraform.go:467-484`
**Problem:** Executor.Output unmarshals `terraform output -json` from the 200-line ring tail. Pretty-printed output maps beyond ~200 lines lose their head and fail the JSON parse. Fails loudly (invalid json error), but the failure mode is a latent capacity cliff unrelated to the actual terraform state.
**Fix:** Use a full-capture executor variant for `terraform output -json` (see sub:1e8ffb91). Current module output count fits in 200 lines, so this is a cliff, not an active break.
**Effort:** hours

##### `sub:19a715fd:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2e-executor
**Severity:** minor
**Cluster:** io-handling — seam→audit-security — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/addon/catalog/secretstore/secretstore.go:276-282`
**Problem:** readSecret returns `sops -d` plaintext from the ring tail. A decrypted secret longer than 200 lines (or a single line over the 64 KiB partial cap, which gets newline-split) is silently truncated/corrupted before being embedded in a cluster Secret manifest — no error, no truncation marker. Typical tokens are one short line, so likelihood is low, but the failure is silent credential corruption.
**Fix:** Decrypt through a full-capture executor variant (see sub:1e8ffb91) so secret material cannot be silently truncated; keep the stdout-not-argv channel for the plaintext.
**Effort:** hours

##### `sub:696d6b0e:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2e-executor
**Severity:** suggestion
**Cluster:** io-handling — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/distribution/okd/phase/iso_cleanup.go:138-159` + 1 more
**Problem:** listProxmoxVMIDs / vmConfigReferencesISO parse pvesh JSON, and RemoveFCOSISOFromProxmox parses find -print0 output, all from the ring tail. pvesh emits single-line JSON: past 64 KiB the partial-buffer split inserts newlines mid-token and breaks the parse. Every consumer fails closed (skip removal / treat as in-use), so impact is a skipped cleanup, not data loss.
**Fix:** When the full-capture executor variant lands (sub:1e8ffb91), route pvesh/find SSH reads through it; current fail-closed behavior makes this non-urgent.
**Effort:** hours

##### `sub:29293401:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2e-executor
**Severity:** suggestion
**Cluster:** io-handling — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/distribution/okd/setup/haproxy.go:186-200`
**Problem:** VerifyHAProxyPorts substring-scans `ss -tlnp` output from the ring tail; on a host with >200 listening sockets the head rows vanish and the check logs spurious 'may not be listening' warnings. Diagnostic-only path, warn-only outcome.
**Fix:** Narrow the query (`ss -tln sport = :6443` per port) or use the full-capture variant; warn-only today so opportunistic.
**Effort:** hours

#### audit-state-and-recovery

##### `state:0f076161:destroy-scoped-cleanup-unscoped` — destroy scoped cleanup unscoped

**Status:** in review — PR #908
**Severity:** blocker
**Cluster:** destroy-safety — related: state:0f076161:destroy-no-cluster-confirm-without-yes
**Evidence:** `internal/cli/destroy.go:282-290` + 2 more
**Problem:** A scoped destroy (--only=bootstrap/masters/workers/vms or --target) only scopes the terraform step. StepCleanupFiles (cleanup.Full: deletes okd-install incl. kubeconfig + kubeadmin-password, haproxy config, dnsmasq drop-in, terraform.tfvars), StepCleanupFirewall, and StepRemoveRemoteISO still run cluster-wide, so `okdctl destroy --only=workers` tears down the bastion DNS/LB plumbing and credential files of the still-running control plane.
**Fix:** When destroyTargets is non-empty (from --only or --target), default SkipCleanup=true, SkipFirewall=true, KeepISOs=true (or set CleanupKind="") in runDestroy, and say so in the flag help; full bastion teardown stays exclusive to the unscoped destroy. Alternatively reject the combination unless the skip flags are passed explicitly.
**Effort:** days

##### `state:15ba17da:destroy-orphans-custom-isos` — destroy orphans custom isos

**Status:** in review — PR #908
**Severity:** minor
**Cluster:** destroy-safety — related: state:0f076161:destroy-scoped-cleanup-unscoped
**Evidence:** `internal/distribution/okd/destroy/steps.go:93-115` + 2 more
**Problem:** StepRemoveRemoteISO only matches fedora-coreos-*.iso, but the VMs boot from the per-node custom ISOs setup uploads (bootstrap0.iso, master<N>.iso, worker<N>.iso — referenced as unmanaged cdrom file_ids in HCL, so terraform destroy never deletes them either). A full `okdctl destroy` therefore strands every multi-GB custom ISO on Proxmox storage indefinitely. Note the generated names carry no cluster prefix, so naive widening of the removal pattern could delete another cluster's ISOs on shared storage.
**Fix:** Extend the destroy ISO step to remove the exact node ISO names derived from cfg topology (bootstrap0.iso, master0..N.iso, worker0..N.iso) through the existing allowlist-validation layering; longer term, prefix generated ISO names with the cluster name so shared-storage collisions are impossible and removal stays name-exact.
**Effort:** hours

##### `state:4c092fce:snapshot-bak-retention-after-destroy` — snapshot bak retention after destroy

**Status:** in review — PR #908
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

**Status:** in review — PR #894
**Severity:** suggestion
**Cluster:** hcl-doc-hygiene
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/main.tf:9-10` + 3 more
**Problem:** The PROXMOX_VE_INSECURE env var is documented twice on adjacent comment lines with conflicting framing — one calls it '(optional, set to true to disable tls verification)', the next '(DEV ONLY ... never set in prod)'. A copy-paste artifact duplicated verbatim across four .tf files; the softer line undercuts the security warning.
**Fix:** Delete the softer 'optional, set to true' line in all four files (modules/.../main.tf, modules/.../variables.tf, environments/production/variables.tf, environments/production/versions.tf header), keeping only the DEV-ONLY warning so the security framing is unambiguous.
**Effort:** hours

##### `iac:e076e43c:token-in-argv` — token in argv

**Status:** in review — PR #894
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

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2m-misc
**Severity:** minor
**Cluster:** domain-vocabulary
**Evidence:** `internal/download/checksum.go:74-74` + 1 more
**Problem:** HTTP-status failures rendered as bare strings ('failed to fetch checksums: HTTP %d', 'github api returned status %d') while the same package defines HTTPStatusError specifically so isRetryable can fail-fast on 4xx and retry 5xx. setup/coreos.go already wraps download.HTTPStatusError cross-package. If FetchChecksum or the releases fetcher are ever placed under retryDownload (as Fetch is), 404s silently degrade from fail-fast to retry-everything.
**Fix:** Return &HTTPStatusError{Status: resp.StatusCode, Method: http.MethodGet, URL: checksumsURL} in FetchChecksum, and fmt.Errorf("github api: %w", &download.HTTPStatusError{...}) in releases/fetcher.go, matching the fetchToFile and setup/coreos.go idiom.
**Effort:** hours

##### `err:aa84670c:exit-mapping-nesting-precedence` — exit mapping nesting precedence

**Status:** in review — PR #896
**Severity:** suggestion
**Cluster:** typed-error-exit-mapping — seam→audit-cli-ux
**Evidence:** `internal/cli/root.go:212-251`
**Problem:** exitCodeFor checks the five category types in fixed order, and each errors.As walks the whole chain — so an inner ConfigError outranks an outer ClusterError regardless of nesting depth (ClusterError{Msg:'addon flux install failed', Err: …ConfigError} exits 2, not 4). The sentinel-over-category precedence is documented in-line; the category-over-category nesting precedence is not, in either the package doc or docs/cli/exit-codes.md.
**Fix:** Document the precedence ('sentinels outrank categories; among categories, Config > Network > Cluster > Auth > Usage wins anywhere in the chain — root-cause type, not outermost wrap, decides the code') in the exitCodeFor doc comment and docs/cli/exit-codes.md; alternatively switch to an outermost-wins single-Unwrap walk if wrap-level classification is the intended contract.
**Effort:** hours

#### audit-concurrency

#### audit-api-design

##### `api:2c4d8e6b:iface-in-producer` — iface in producer

**Status:** in review — PR #910
**Severity:** minor
**Cluster:** interface-location
**Evidence:** `internal/addon/addon.go:50-92` + 3 more
**Problem:** ConfigurableAddon and WizardProvider are implemented by both catalog addons (DefaultSettings/ValidateSettings/DecodeSettings/WizardFields) but no code anywhere type-asserts or consumes them. The intended consumer (wizard addons step) instead imports flux/secretstore concretely and hand-builds the same field catalog, so addon wizard fields, defaults, and validation now live in two places that can drift. Not scaffolding: the consumer exists and bypassed the contract, so the duplication cost is current, not future.
**Fix:** Either wire the wizard addons step to iterate addon.All() and type-assert WizardProvider/ConfigurableAddon to render fields generically (deleting the hand-built duplicates), or record a decision that the wizard owns its field layout and retire the unconsumed interfaces. Verify intent with the owner before either move (MEMORY.md scaffolding protocol).
**Effort:** hours

##### `api:7b2829bb:ring-tail-contract` — ring tail contract

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2e-executor
**Severity:** minor
**Cluster:** exported-surface — seam→audit-subprocess — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/executor/executor.go:25-31` + 2 more
**Problem:** Executor.Run's Result.Stdout/Stderr silently carry only the last 200 lines with no truncation signal and no full-capture sibling API; callers that parse stdout as JSON (8 sites per the wave-1 subprocess audit) consume the tail as if complete. The per-site misuse is owned by audit-subprocess; the API gap — no additive full-capture path — is the design defect here.
**Fix:** Add an additive full-capture API: either RunFull(ctx, name, args...) with a byte-capped full buffer, or a WithCaptureLimit/CaptureAll per-call option, plus a Result.Truncated bool so parse-callers can fail loudly. Migrate the 8 JSON-parsing call sites onto it; streaming/log paths keep the ring.
**Effort:** hours

##### `api:d6b325cb:pkg-import-cycle-adj` — pkg import cycle adj

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2p-proxmox
**Severity:** minor
**Cluster:** package-boundary
**Evidence:** `internal/infrastructure/proxmox/types.go:1-51` + 4 more
**Problem:** internal/infrastructure/proxmox imports internal/distribution/okd/phase for domain vocabulary (NodeRole, VMState) and remote pvesh helpers, inverting infra-under-distribution layering; pvesh.go's own comment concedes the helpers are stranded in phase/ only because moving them would cycle (proxmox already imports phase).
**Fix:** Extract NodeRole/VMState/Condition* plus the pvesh/SSH remote-op helpers (pvesh.go, ssh.go, iso_cleanup.go remote bits) into a neutral leaf package (e.g. internal/proxmoxops) imported by both phase and infrastructure/proxmox. Reverses the infra->distribution edge and dissolves the alias re-export block in types.go.
**Effort:** hours

##### `api:c287d5c0:pkg-facade-bypassed` — pkg facade bypassed

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2s-steps
**Severity:** minor
**Cluster:** package-boundary
**Evidence:** `internal/distribution/okd/okd.go:146-154` + 2 more
**Problem:** okd.Provisioner facade is asymmetric: Prepare/Configure build their phase Options internally, but Install requires cli to import okd/install and hand it install.NewOptions(cfg, projectRoot) verbatim. Separately, cli/deploy.go deployDryRunSteps hand-duplicates 32 step IDs AND display names already declared in the phases' StepDefs, importing setup/install/postinstall just for the constants - a drift-prone second source of truth.
**Fix:** (1) Have Provisioner.Install build install.NewOptions(cfg, p.projectRoot) internally, matching Configure; (2) add Provisioner.DeploySteps() returning ID+Name derived from the same xSteps() StepDef slices so the dry-run listing cannot drift. cli/deploy.go then drops its okd/setup, okd/install, okd/postinstall imports (postinstall.Result/UpdateIngressResult remain legitimately exposed).
**Effort:** hours

##### `api:5e892064:ctx-missing-on-io` — ctx missing on io

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2m-misc
**Severity:** minor
**Cluster:** ctx-first
**Evidence:** `internal/download/checksum.go:21-54` + 2 more
**Problem:** CalculateChecksum/ValidateChecksum stream entire files through sha256 with no ctx; on multi-GB CoreOS ISOs this runs tens of seconds inside Fetch's skip-check and post-download verification, so Ctrl-C cannot interrupt the hash even though every caller already holds a ctx.
**Fix:** Add ctx as first parameter (CalculateChecksum(ctx, path)) and copy via a ctx-checking reader (check ctx.Err() per 1-4 MiB chunk). Callers Fetch, canSkipDownload, verifyDownloadedFile, and ExtractTarGz already have ctx in scope.
**Effort:** hours

##### `api:d31d1b9d:pkg-facade-bypassed` — pkg facade bypassed

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2c-cluster
**Severity:** minor
**Cluster:** package-boundary — seam→audit-code-smells
**Evidence:** `internal/cli/status.go:388-404` + 1 more
**Problem:** cli/status constructs phase.NewBasePhase to reach BasePhase.OcOutput for oc queries, while internal/cluster.Client exists as the thin oc wrapper (with WithKubeconfig built for exactly this). CLAUDE.md scopes BasePhase.Oc* to phase code; cluster.Client's surface is too narrow (no exported output method), so cli reached for distribution internals instead.
**Fix:** Add an exported Output(ctx, args ...string) (string, error) to cluster.Client (it already wraps executor + KUBECONFIG injection) and switch cli status/describe to cluster.New(cluster.WithKubeconfig(kcPath)). BasePhase.Oc* stays untouched for phase code.
**Effort:** hours

##### `api:0934cf1b:iface-in-producer` — iface in producer

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2e-executor
**Severity:** suggestion
**Cluster:** interface-location
**Evidence:** `internal/platform/packages.go:18-56`
**Problem:** NewPackageManager returns the PackageManager interface rather than *Manager (violating 'accept interfaces, return structs'), and the interface methods take a per-call *slog.Logger (Remove even ignores it) while every other repo type injects the logger at construction.
**Fix:** Return *Manager from NewPackageManager and move the logger to a Manager field (constructor arg or option), dropping the per-call logger params. Keep the PackageManager interface only if a consumer-side fake needs it — then declare it consumer-side (setup/cleanup).
**Effort:** hours

##### `api:92553fff:export-no-caller` — export no caller

**Status:** not started
**Severity:** minor
**Cluster:** exported-surface
**Evidence:** `internal/cli/summary.go:77-277`
**Problem:** internal/cli exports six presentation helpers (DryRunStep, DryRunSummary, ValidationSummary, PostDeploySummary, InterruptSummary, UpdateIngressSummary) with zero callers outside the package; cli's external surface is only Execute/RootCmd/DeferWarn. Not future-CLI-verb shaped (they are called today, in-package).
**Fix:** Unexport the six summary symbols (mechanical rename within package cli). Keeps the package's public surface at the three symbols cmd/ actually consumes.
**Effort:** hours

##### `api:cfcdee2d:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** in review — PR #897
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

**Status:** in review — PR #897
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

**Status:** in review — PR #896
**Severity:** suggestion
**Cluster:** json-stability — related: sub:d31d1b9d:ring-truncated-stdout-parse
**Evidence:** `docs/cli/json-schema.md:56-67` + 2 more
**Problem:** `okdctl status --output=json` always emits nodes[].status (NodeStatusReady/NotReady, set unconditionally in runStatus) and the NodeStatus struct can additionally emit version, internal_ip, and conditions via omitempty, but the documented field table stops at name/role/ready. The stability promise ('field names are stable') covers fields consumers cannot discover from the doc.
**Fix:** Add nodes[].status to the field table (values: Ready|NotReady|Unknown) and note version/internal_ip/conditions as optional fields reserved for future population, or strip the unused omitempty fields from the CLI-facing projection.
**Effort:** hours

##### `ux:aa84670c:exit-code-undefined` — exit code undefined

**Status:** in review — PR #896
**Severity:** suggestion
**Cluster:** exit-codes
**Evidence:** `internal/cli/root.go:157-170`
**Problem:** signalLoop's second-strike path calls exit(130) unconditionally, so a double SIGTERM (e.g. systemd stop escalation) exits 130 (SIGINT code) instead of 143, contradicting the published taxonomy that maps SIGTERM to 143.
**Fix:** Capture the second signal value and exit 143 when it is syscall.SIGTERM, 130 otherwise. Preserve the documented close(sigCh)-after-signal.Stop ordering (con:aa84670c).
**Effort:** hours

##### `ux:e7db1220:flag-completion-inconsistent` — flag completion inconsistent

**Status:** in review — PR #896
**Severity:** suggestion
**Cluster:** flag-conventions
**Evidence:** `internal/cli/releases.go:77-83` + 2 more
**Problem:** Enum-valued flags get shell completion inconsistently: --channel (releases list) and --only (destroy) register RegisterFlagCompletionFunc, but the eight --output text|json flags plus --log-level and --log-format complete nothing despite having closed value sets validated at runtime.
**Fix:** Add a shared helper that registers cobra.FixedCompletions([]string{outputText, outputJSON}, NoFileComp) wherever flagOutput is bound, plus log-level (debug,info,warn,error) and log-format (text,json) on the root persistent flags.
**Effort:** hours

##### `ux:fd2125dd:verb-noun-inconsistent` — verb noun inconsistent

**Status:** in review — PR #896
**Severity:** suggestion
**Cluster:** verb-noun
**Evidence:** `internal/cli/addon.go:27-31` + 1 more
**Problem:** Sibling noun groups disagree on number: `okdctl addon list` (singular) vs `okdctl releases list` (plural). Same grammatical position, two conventions — users must remember which group pluralizes.
**Fix:** Both names are shipped; renaming is a breaking change bigger than the smell. If desired, add a `release` alias (cobra Aliases) to releasesCmd or `addons` alias to addonCmd and standardize in docs; otherwise record the choice and keep new noun groups singular.
**Effort:** hours

#### audit-observability

#### audit-modernization

##### `mod:48688e63:use-slices-containsfunc` — use slices containsfunc

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2p-proxmox
**Severity:** suggestion
**Cluster:** slices-maps
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:499-503`
**Problem:** Hand-rolled membership loop checking whether any enumerated VM carries vmidBase; slices.ContainsFunc (Go 1.21) collapses it to one expression.
**Fix:** return slices.ContainsFunc(vms, func(v vmIDProbe) bool { return v.VMID == vmidBase }) after naming the anonymous struct type — or keep the loop if naming the type costs more than it saves; behavior identical including the fall-through Info log when false.
**Effort:** hours

##### `mod:e3782ee7:use-errors-is` — use errors is

**Status:** not started
**Severity:** minor
**Cluster:** errors-and-deprecated-stdlib
**Evidence:** `internal/system/fs.go:91-210` + 16 more
**Problem:** 24 in-scope sites still use os.IsNotExist/os.IsExist, which the os package docs mark as predating errors.Is and which do not unwrap wrapped errors; newer repo code (internal/cli, internal/runlock) already uses errors.Is(err, os.ErrNotExist), and internal/errtypes/errtypes.go:35 explicitly documents that wrapped-sentinel matching relies on errors.Is.
**Fix:** Mechanical swap: os.IsNotExist(err) -> errors.Is(err, fs.ErrNotExist) (or os.ErrNotExist, matching internal/cli usage), os.IsExist(err) -> errors.Is(err, fs.ErrExist); add errors / io/fs imports as needed. One-for-one, no behavior change for unwrapped os errors, fixes latent mismatch for wrapped ones.
**Effort:** hours

#### audit-code-smells

##### `smell:4ded56d3:retry-scaffold-triplicated` — retry scaffold triplicated

**Status:** not started
**Severity:** minor
**Cluster:** dual-impl-same-job
**Evidence:** `internal/download/retry.go:84-112` + 2 more
**Problem:** Three packages hand-roll the identical retry scaffold around wait.ExponentialBackoffWithContext: same backoff constants (5s, factor 2, jitter 0.5, 3 steps, 5m cap), same lastErr-preservation tail so backoff exhaustion returns the real error instead of the wait sentinel, and three near-identical isRetryable classifiers. The copies even cross-reference each other in comments ("mirrors retryDownload"), confirming they are meant to be one thing.
**Fix:** Extract one helper, e.g. system.RetryBackoff(ctx, backoff wait.Backoff, retryable func(error) bool, fn func() error) (attempts int, err error) — internal/system already hosts the analogous WaitFor family and is imported by all three packages. Each caller keeps its own retryable classifier (download's HTTP-status logic, addon's exec.ErrNotFound case, proxmox's ConfigError/AuthError case). Risk medium: the three classifiers differ deliberately; only the backoff-loop shell is shared.
**Effort:** hours

##### `smell:4c092fce:pipeline-explicit-errors` — pipeline explicit errors

**Status:** in review — PR #907
**Severity:** minor
**Cluster:** arrow-anti
**Evidence:** `internal/infrastructure/terraform/terraform.go:212-227` + 8 more
**Problem:** Eight call sites across four packages repeat the same 5-line wrap dance: `if hint := tf.LockHint(); hint != nil { return errors.Join(hint, wrapped) } return wrapped`. The lock-hint enrichment is part of the terraform.Executor error contract but is hand-assembled by every caller, so new callers (and one existing destroy path: destroyInfrastructure's Destroy call at destroy/helpers.go:49-60 skips it) can forget the hint.
**Fix:** Add `func (t *Executor) WithLockHint(err error) error` to terraform.go: nil-in/nil-out, joins LockHint when present. Call sites become `return t.WithLockHint(&errtypes.ClusterError{...})`. Alternatively fold into Executor.run so every state-locking subcommand error carries the hint automatically.
**Effort:** hours

##### `smell:0139cb3f:magic-path-literal` — magic path literal

**Status:** not started
**Severity:** minor
**Cluster:** magic-strings
**Evidence:** `internal/distribution/okd/phase/paths.go:130-137` + 16 more
**Problem:** The terraform environment directory is assembled inline as filepath.Join(root, "infrastructure", "terraform", "environments", env) at 16 sites across 8 packages, even though phase/paths.go already hosts the sibling helpers GetTerraformEnv and ClusterConfigDir. Any layout change (or a typo in one segment) drifts per-site with no compile check.
**Fix:** Add `func TerraformEnvDir(projectRoot, env string) string` next to GetTerraformEnv in phase/paths.go (CLAUDE.md names phase/ as the home for cross-phase helpers) and replace the 16 Join sites. infrastructure/proxmox already imports phase, so no import-cycle risk. cli/debug_bundle.go's bare "production" fallback (L262) collapses into the same helper path or stays with a comment.
**Effort:** hours

##### `smell:6424733c:pipeline-explicit-errors` — pipeline explicit errors

**Status:** not started
**Severity:** suggestion
**Cluster:** arrow-anti
**Evidence:** `internal/cli/helpers.go:349-385` + 2 more
**Problem:** executeFullDeployment repeats a near-identical 8-line cancellation block after each of the three phase calls (errors.Is(context.Canceled) -> InterruptSummary -> phase-specific hint -> return) differing only in the hint string and the steps slice. Separately, the tfstate-recovery glob is pasted twice in the same file (hasProjectMarker and warnIfTfStateOnly).
**Fix:** Extract `func deployPhaseErr(w io.Writer, err error, steps []distribution.StepResult, runID, cancelHint string) error` collapsing the three blocks; extract `tfStateGlob(root string) []string` for the duplicated glob. Pure consolidation, no behavior change.
**Effort:** hours

##### `smell:451be4fa:magic-path-literal` — magic path literal

**Status:** not started
**Severity:** major
**Cluster:** magic-strings
**Evidence:** `internal/system/elevation.go:140-144` + 13 more
**Problem:** The workdir name "okd-install" is a bare string literal at 13 filepath.Join sites across cli and all five okd phase packages, and the same literal is independently duplicated inside the sudo chown-back security allowlist (isAllowedChownRoot). A rename at the Join sites silently stops matching the allowlist, disabling the chown-back guard without any compile error.
**Fix:** Declare a single exported constant in a leaf package both sides can import (internal/system, which phase/ and cli/ already import: `const WorkDirName = "okd-install"` plus optionally `func WorkDir(projectRoot string) string`). Replace all 13 Join literals and the elevation.go allowlist comparison with the constant. "infrastructure" in the same allowlist gets the same treatment.
**Effort:** hours

##### `smell:2c4d8e6b:interfaceany-lazy-exported` — interfaceany lazy exported

**Status:** in review — PR #910
**Severity:** suggestion
**Cluster:** interfaceany-lazy — seam→audit-api-design
**Evidence:** `internal/addon/addon.go:58-58` + 4 more
**Problem:** ConfigurableAddon.DecodeSettings returns (any, error), but grep shows zero generic consumers: the only callers are the implementing addons themselves, each immediately type-asserting its own concrete Settings (decoded.(Settings)) — an unchecked assertion that would panic if the method ever returned a different type. The any in the interface buys no polymorphism and costs a panic path per addon.
**Fix:** Either drop DecodeSettings from the interface (each addon keeps a package-private typed decode — nothing external calls it generically today), or keep the interface method but have each addon call an unexported typed decodeSettings internally so the any round-trip and unchecked assertions disappear. If a future generic consumer (wizard validation) is planned, document it on the interface; the interface-shape decision itself is audit-api-design territory.
**Effort:** hours

##### `smell:40d315ad:settings-stringified-numbers` — settings stringified numbers

**Status:** in review — PR #910
**Severity:** suggestion
**Cluster:** stringified-numbers
**Evidence:** `internal/addon/catalog/flux/flux.go:581-588` + 4 more
**Problem:** controller_timeout/git_sync_timeout defaults are stored as the strings "300"/"180" and re-parsed with strconv.Atoi at each use via getTimeout reading the raw settings map — bypassing the typed Settings struct that DecodeSettings exists to produce (the struct simply omits both fields). A malformed value silently falls back to the default instead of failing validation.
**Fix:** Add ControllerTimeout/GitSyncTimeout time.Duration fields to flux.Settings, parse once in DecodeSettings (returning an error on malformed input so ValidateSettings surfaces it), and have waitForControllers/waitForGitSync read the struct. Delete getTimeout. The map[string]string wire format stays — only the parse point moves.
**Effort:** hours

##### `smell:0f076161:enum-ad-hoc` — enum ad hoc

**Status:** in review — PR #908
**Severity:** suggestion
**Cluster:** magic-strings
**Evidence:** `internal/cli/destroy.go:109-130`
**Problem:** validateDestroyTargets switches on the raw regex capture with case "bootstrap" / "master" / "worker" string literals even though phase.NodeRole typed constants (RoleBootstrap/RoleMaster/RoleWorker) and ParseNodeRole exist exactly for this vocabulary, and destroy.go already imports phase. The same literals also appear baked into destroyTargetRE — acceptable there (regex alternation), but the switch should speak the typed vocabulary.
**Fix:** switch phase.NodeRole(m[1]) { case phase.RoleBootstrap: ... case phase.RoleMaster: ... case phase.RoleWorker: } — or run the capture through phase.ParseNodeRole, which exists as the canonical deserializer (its doc note "currently no caller" resolves itself).
**Effort:** hours

##### `smell:90fa855c:coreos-iso-glob-dup` — coreos iso glob dup

**Status:** not started
**Severity:** suggestion
**Cluster:** dual-impl-same-job — re-queued from PR #884's skipped-items section (reverted there by a stale-local-linter false gate)
**Evidence:** `internal/distribution/okd/setup/coreos.go:100-123`
**Problem:** The pattern-glob → slices.Max → logISOFound scan loop is duplicated byte-for-byte for isoDir (L100-110) and workISODir (L113-123); any change to the pattern list or newest-selection rule must be made twice or the two search paths silently diverge.
**Fix:** Extract a findNewestISO(dir string, patterns []string) (string, bool) helper and call it for both directories.
**Effort:** hours

#### audit-dependencies

##### `dep:b803fcb7:version-floor-unjustified` — version floor unjustified

**Status:** in review — PR #894
**Severity:** minor
**Cluster:** pin-stability
**Evidence:** `.github/workflows/ci.yml:56-56` + 2 more
**Problem:** Go-tool pins in CI and the Makefile are explicit versions (policy-compliant) but sit outside every renovate manager — the custom regex manager only matches annotated lines in .env/.sh/.yaml and the gomod manager only reads go.mod — so they rot silently: govulncheck pinned v1.1.4 vs latest v1.3.0 (the security gate itself is two minors stale), yamlfmt v0.14.0 vs v0.21.0, air v1.61.7 vs v1.65.3.
**Fix:** Add renovate annotations above each pin ('# renovate: datasource=go depName=golang.org/x/vuln') and extend customManagers.json5 managerFilePatterns to cover /\.yml$/ and /Makefile$/; or move govulncheck/yamlfmt to go.mod 'tool' directives (Go 1.24+) so the gomod manager tracks them; then bump govulncheck to v1.3.0, yamlfmt to v0.21.0, air to v1.65.3.
**Effort:** hours

##### `dep:33ef32bf:dup-log-engines` — dup log engines

**Status:** in review — PR #895
**Severity:** suggestion
**Cluster:** duplicate-engine
**Evidence:** `go.mod:11-11` + 3 more
**Problem:** Four log engines compile into the production binary: stdlib log/slog (canonical sink per CLAUDE.md), charm.land/log/v2 (direct, single call-site styled stderr formatter), and go-logr/logr + k8s.io/klog/v2 (pinned by k8s.io/apimachinery, both linked per go list -deps). CLAUDE.md carries a YAML-engine baseline tripwire but no equivalent log-engine baseline, so a fifth engine could land without a recorded justification.
**Fix:** Record a log-engine baseline in CLAUDE.md §dependencies mirroring the YAML tripwire: slog = canonical, charm.land/log/v2 = intentional UI formatter (1 file), logr/klog = k8s-pinned indirects; do not add a fifth without justification. No code change — charm libs are the intentional UI stack and klog/logr are upstream-locked.
**Effort:** hours

##### `dep:6ebdb617:dep-registry-drift` — dep registry drift

**Status:** in review — PR #895
**Severity:** minor
**Cluster:** maintenance-signal
**Evidence:** `CLAUDE.md:234-247` + 1 more
**Problem:** CLAUDE.md dependency registry has drifted from go.mod in three places: go-proxmox is documented at v0.5.x but go.mod pins v0.7.1; the joho/godotenv LICENCE-spelling entry remains but the dep is no longer in go.mod or go.sum; §tooling says 'Go 1.25 / toolchain 1.26.2' while go.mod declares go 1.26.0 / toolchain go1.26.4.
**Fix:** Update CLAUDE.md: bump go-proxmox registry row to v0.7.x, delete the godotenv entry (dep removed), and correct §tooling to 'Go 1.26 / toolchain 1.26.4' (or make it version-agnostic: 'go.mod go/toolchain directives are authoritative; don't downgrade').
**Effort:** hours

##### `dep:33ef32bf:version-floor-unjustified` — version floor unjustified

**Status:** in review — PR #895
**Severity:** minor
**Cluster:** justified-version-floor — related: dep:6ebdb617:dep-registry-drift
**Evidence:** `go.mod:39-39` + 1 more
**Problem:** Transitive version floors inherited from go-proxmox are years stale and both packages compile into the shipped binary: gorilla/websocket v1.4.2 (Mar 2020; latest v1.5.3, Jun 2024) and jinzhu/copier v0.3.4 (2021; latest v0.4.0). CLAUDE.md claims okdctl 'does not reach' websocket — true at the call-graph level, but `go list -deps ./cmd/okdctl` shows both modules linked into the release artifact.
**Fix:** go get github.com/gorilla/websocket@v1.5.3 github.com/jinzhu/copier@v0.4.0 && go mod tidy — okdctl calls neither API directly, so the floor lift is behavior-neutral; also sharpen the CLAUDE.md websocket entry to say 'linked but never called' rather than 'does not reach it'.
**Effort:** hours

##### `dep:b803fcb7:pin-action-trailer-imprecise` — pin action trailer imprecise

**Status:** in review — PR #894
**Severity:** minor
**Cluster:** pin-stability — related: dep:6ebdb617:dep-registry-drift
**Evidence:** `.github/workflows/ci.yml:15-19` + 6 more
**Problem:** All actions are SHA-pinned (policy-compliant) but several version trailers are major-only ('# v6', '# v9', '# v7', '# v4', '# v2') where CLAUDE.md prescribes 'uses: owner/action@<40-hex-sha> # vX.Y.Z' — a reviewer cannot tell which minor/patch a 40-hex digest corresponds to without resolving it. The same files contain the precise form as counter-examples (setup-tflint '# v6.2.2', cosign-installer '# v4.1.2', slsa generator '# v2.1.0').
**Fix:** Point renovate at precise tags so the trailer renders as vX.Y.Z (pin actions to the full release tag before digest-pinning, e.g. actions/checkout@<sha> # v6.0.2), or amend CLAUDE.md to permit major-tag trailers for renovate-managed digests — pick one so policy and practice agree.
**Effort:** hours

##### `dep:33ef32bf:transitive-heavy-narrow` — transitive heavy narrow

**Status:** in review — PR #895
**Severity:** suggestion
**Cluster:** transitive-weight — related: dep:33ef32bf:version-floor-unjustified
**Evidence:** `go.mod:12-12` + 1 more
**Problem:** go-proxmox v0.7.1 is imported in exactly one file (wizard REST discovery) yet links 5 extra modules into the release binary: diskfs/go-diskfs (full ISO9660/MBR filesystem stack), buger/goterm, jinzhu/copier, djherbis/times, gorilla/websocket. Known case per CLAUDE.md §dependencies (bus-factor 1, ~200 LOC REST-rewrite fallback); re-confirmed this run: upstream is active (v0.7.1 released 2026-06-02, 267 stars, Apache-2.0, 0 open issues) and the abandonment plan remains valid.
**Fix:** No action now — re-confirmation of the accepted trade. If upstream stalls or a v0.8 minor breaks the wizard, execute the documented fallback: ~200 LOC net/http REST client for the discovery endpoints, dropping 6 modules from the binary.
**Effort:** hours

#### audit-documentation

##### `doc:aa84670c:doc-comment-stale` — doc comment stale

**Status:** in review — PR #896
**Severity:** minor
**Cluster:** exported-doc — related: doc:b3356305:readme-flag-ghost, ux:073d24ed:concept-named-twice
**Evidence:** `internal/cli/root.go:33-38`
**Problem:** The cfgFile doc comment says it is "read by subcommand RunE handlers (deploy, destroy, update-ingress)". deploy never reads cfgFile (it uses deployOutputFile), while six undocumented commands do read it (addon, cleanup, config, debug-bundle, doctor, status). The stale reader list misleads at exactly the spot where the deploy/--config drift (ux:073d24ed) lives.
**Fix:** Drop the parenthetical command list (it rots on every new subcommand) or replace with "read by every config-consuming subcommand except deploy, which manages its own file via --output-file".
**Effort:** hours

##### `doc:acb745e5:stepbuilder-doc-self-referential` — stepbuilder doc self referential

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-doc — re-queued from PR #884's skipped-items section (reverted there by a stale-local-linter false gate)
**Evidence:** `internal/distribution/step.go:97-109`
**Problem:** NewStepBuilder's doc states the fatal-by-default contract and the constructor body restates it inline (`fatal: true, // default to fatal`) — a narrating echo of the doc one screen up, against the comment policy's echo/self-reference rules.
**Fix:** Keep the doc sentence; delete the inline `// default to fatal` echo.
**Effort:** hours

##### `doc:c3dc10bb:flux-gettimeout-doc-orphaned` — flux gettimeout doc orphaned

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-doc — re-queued from PR #884's skipped-items section (reverted there by a stale-local-linter false gate)
**Evidence:** `internal/addon/catalog/flux/flux.go:565-568`
**Problem:** getTimeout's doc comment ("reads a timeout setting (in seconds)…") is stranded directly above readKeyFile's own doc — the function it describes sits at L581 with no doc, so readKeyFile carries two doc blocks and godoc attributes the wrong one.
**Fix:** Move the two-line doc onto getTimeout at L581, or delete it if the signature is judged to carry the signal.
**Effort:** hours

#### audit-tests

##### `tst:b38ec9cc:destructive-happy-untested` — destructive happy untested

**Status:** in review — PR #907
**Severity:** major
**Cluster:** destructive-untested — seam→audit-state-and-recovery — related: state:0f076161:destroy-scoped-cleanup-unscoped
**Evidence:** `internal/distribution/okd/install/workers.go:22-76`
**Problem:** StartWorkerVMs runs a live terraform apply and nothing locks its two safety properties: the -target scoping to the worker VM resource (the in-code comment admits an unscoped apply would reconcile the full state) and the snapshot-before-apply ordering. workersAlreadyRunning's node-count parse is also untested.
**Fix:** Use the fake-terraform-binary harness from destroy/helpers_test.go: capture argv, assert -target=module.okd_cluster.proxmox_virtual_environment_vm.worker and -var start_workers_immediately=true are present, and that a state snapshot exists before apply runs. Table-test workersAlreadyRunning line counting (0 workers, exact count, cluster-unreachable→false,nil).
**Effort:** hours

##### `tst:40d315ad:destructive-happy-untested` — destructive happy untested

**Status:** in review — PR #910
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2s-steps
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2c-cluster
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2e-executor
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

- **Status:** in review — PR #893
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2w-wizard
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2w-wizard
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2w-wizard
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2p-proxmox
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2w-wizard
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

- **Status:** not started
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

- **Status:** in review — PR #908
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2w-wizard
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2w-wizard
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

- **Status:** in review — PR #896
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

- **Status:** in review — PR #896
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2w-wizard
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

- **Status:** not started
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2s-steps
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2m-misc
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2e-executor
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2p-proxmox
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

- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/w2m-misc
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

- **Status:** not started
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

- **Status:** in progress — worktree: wave3
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

- **Status:** not started
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

- **Status:** in review — PR #904
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

- **Status:** in progress — worktree: wave3
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** in progress — worktree: wave3
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

- **Status:** in review — PR #914
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** in progress — worktree: wave3
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

- **Status:** in review — PR #915
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

- **Status:** not started
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

## Completed

Completed items live in [`docs/roadmap/completed-archive.md`](docs/roadmap/completed-archive.md). Grep there for the canonical "is dep X done?" lookup. The previous in-line pointer index (144 entries, mirroring archive contents) was removed on 2026-05-09 to keep `roadmap.md` focused on active work.

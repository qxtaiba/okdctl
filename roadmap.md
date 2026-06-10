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

##### `ci-testgo-xcrypto-053-runner-death` — test-go deterministically kills the CI runner since x/crypto v0.53.0

**Status:** in review — PR #824
**Severity:** blocker
**Cluster:** ci-infrastructure
**Evidence:** runs 27179112763 (`46d11fa`, failed twice — original + 2026-06-10 re-run) and 27166818169 (`c651b3f`); last green `test-go` at `022b1ca`
**Problem:** `go test ./...` on ubuntu runners dies with "The runner has received a shutdown signal" (exit 143) at the same point in both failing commits and across a re-run: output green through `internal/addon/catalog/secretstore`, runner dead ~60 s later. Regression window is exactly the renovate `golang.org/x/crypto` v0.52.0→v0.53.0 bump. Local darwin `go test ./...` passes in full, so the trigger is in the linux-only test surface (`//go:build linux` files don't compile on darwin — see MEMORY.md GOOS-lint note). Signature (runner agent receiving SIGTERM, not the test process timing out) is consistent with memory exhaustion OOM-killing the runner service. Until fixed, every push to develop reports red CI regardless of code health.
**Fix:** Reproduce under linux (`GOOS`-honest env: container or `act`) with `go test -v` per-package to isolate the package/test that explodes after the x/crypto bump; check x/crypto v0.53.0 release notes for ssh-handshake/kex changes affecting `internal/sshpin` / cluster SSH tests; either fix the test or pin x/crypto back with a renovate ignore until upstream is understood. Verify by re-running ci on develop.
**Effort:** hours

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

**Status:** not started
**Severity:** suggestion
**Cluster:** tls-network
**Evidence:** `scripts/install.sh:113-119`
**Problem:** GitHub releases-API resolution at L113-L119 fetches the latest tag via TLS to api.github.com with no integrity check beyond the TLS handshake. A compromised api.github.com cert or attacker-controlled DNS resolving to a TLS-MITM proxy would let the resolver return any tag; the subsequent cosign verification of SHA256SUMS only verifies the file at $BASE_URL — but $BASE_URL is built from the attacker-influenced VERSION. Once VERSION is malicious, the trust chain follows. The cosign cert-identity-regexp does anchor artefacts to qxtaiba/okdctl, so the attacker must serve a real signed release.
**Fix:** Defense-in-depth: after VERSION is resolved, validate it matches the regex ^v[0-9]+\.[0-9]+\.[0-9]+ before constructing URLs. This won't stop a creative attacker but bounds the URL grammar. The cosign cert-identity-regexp at L156 is the real trust root.
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

##### `state:b38ec9cc:workers-targeted-apply-skips-other-drift` — workers targeted apply skips other drift

**Status:** not started
**Severity:** suggestion
**Cluster:** phase-idempotency
**Evidence:** `internal/distribution/okd/install/workers.go:46-76`
**Problem:** StartWorkerVMs runs `terraform apply -target=module.okd_cluster.proxmox_virtual_environment_vm.worker -var start_workers_immediately=true`. The targeted apply intentionally scopes to workers — but if a user hand-edited terraform.tfvars between setup and install (changing master CPU, network bridge), that drift is silently NOT applied. A subsequent un-scoped apply (e.g. via `okdctl deploy` re-run) suddenly applies all the drift at once.
**Fix:** No code change. The targeted apply is the right choice — un-scoped apply during worker-start would risk applying mid-cluster drift on master VMs. Document this trade-off in the function-level docstring (it's already a comment block, just lift to the function doc) so future readers don't mistake it for a forgotten target restriction.
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


#### audit-errors

##### `err:d7ce9d16:dns-bare-fmt-errorf-not-classified` — dns bare fmt errorf not classified

**Status:** in review — PR #807
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

##### `err:b38ec9cc:lock-hint-exit-code-flip` — lock hint exit code flip

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/err-b38ec9cc-lock-hint
**Severity:** major
**Cluster:** sentinel-vs-typed — seam→`audit-cli-ux`
**Evidence:** `internal/distribution/okd/install/workers.go:39-71` + 3 more
**Problem:** errors.Join(hint, wrapped) where hint is *errtypes.ConfigError and wrapped is *errtypes.ClusterError works because errors.Join's Unwrap() []error lets errors.As walk to both. But exitCodeFor walks the declaration order: ConfigError → 2, ClusterError → 4. errors.As returns on the first match. The hint matches ConfigError → exit 2; the ClusterError → exit 4 mapping is unreachable. terraform init failure → ClusterError → 4 normally; same failure under a stale lock → ConfigError → 2. Operators scripting against exit codes will see flaky behaviour.
**Fix:** Either (a) downgrade LockHint to return a *string* that the wrapped ClusterError appends to Msg (exit code stays uniform), or (b) restructure to wrap the hint message inside the ClusterError's Msg with the underlying err: `&errtypes.ClusterError{Msg: msg + '; ' + hintMsg, Err: err}` so only one typed error is in the chain. (c) Or accept the exit-2-for-locked-state mapping and document it in cli/root.go's exit-code table. Today the chain produces inconsistent exit codes silently.
**Effort:** hours

##### `err:f55b9c27:envfile-loadonce-no-sentinel` — envfile loadonce no sentinel

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/err-f55b9c27-envfile-sentinel
**Severity:** suggestion
**Cluster:** sentinel-vs-typed
**Evidence:** `internal/credentials/envfile.go:120-129`
**Problem:** LoadEnvFile returns a ConfigError when called twice with different paths. There is no way for the caller to distinguish 'already initialized, original error' from 'already initialized with a different path' without string-sniffing the Msg.
**Fix:** Add a package-local sentinel `var ErrEnvFileAlreadyLoaded = errors.New("env file already loaded with different path")` and use it as Err on the ConfigError. Callers can errors.Is to detect the double-init case without parsing Msg. Today there's exactly one caller chain (cli/helpers.go::handleCredentials, cli/destroy.go) so practical risk is zero.
**Effort:** hours

##### `err:366b3f2d:orchestrator-classifysteperr-canonical` — orchestrator classifysteperr canonical

**Status:** not started
**Severity:** suggestion
**Cluster:** sentinel-vs-typed
**Evidence:** `internal/distribution/orchestrator.go:115-133`
**Problem:** classifyStepErr is the load-bearing safety net: it correctly preserves cancellation identity and skips wrapping for already-typed errtypes. The smell isn't in this function but in its presence — it catches 14+ bare fmt.Errorf sites in dns/postinstall/setup/install packages. Those packages' contracts are silently orchestrator-dependent. Documented as the canonical example so reviewers know why surrounding findings are minor not major.
**Fix:** No change. Documented as the canonical fallback so future audits know it exists. If classifyStepErr is ever removed or moved, the 14 bare-fmt.Errorf sites above must each be hardened first.
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

**Status:** not started
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

##### `api:ae5b624c:iface-csr-approver-positive` — iface csr approver positive

**Status:** not started
**Severity:** suggestion
**Cluster:** interface-location
**Evidence:** `internal/distribution/okd/install/monitor.go:78-83`
**Problem:** install.csrApprover is a one-method interface (ApprovePendingCSRs) defined at the consumer side (install package) where cluster.Client is the real implementation. This is the correct shape per Go proverb 'Accept interfaces, return structs'. The interface is unexported (csrApprover), which is the right call for a single-package consumer. Logged as a positive counter-example for the api-design audit — every interface that abstracts a cluster operation should follow this shape.
**Fix:** No action — this is the canonical pattern. Document explicitly in cluster.Client's package doc that 'consumers may define their own narrow interfaces to test against; cluster.Client returns concrete types, never interfaces'. Future cluster-method additions (e.g. Client.GetNodes) should leave consumer-side interface definition to the consumer.
**Effort:** hours


#### audit-cli-ux

##### `ux:fd2125dd:concept-named-twice` — concept named twice

**Status:** not started
**Severity:** minor
**Cluster:** verb-noun
**Evidence:** `internal/cli/addon.go:28-31` + 1 more
**Problem:** Parent group nouns are inconsistently singular vs plural across the command tree: 'addon list/install/uninstall/verify' (singular) but 'releases list/show' (plural). Internal references compound the drift — addon.go uses 'addon list/verify' in messages while releases.go uses 'releases list'. Pick one: kubectl/oc convention is singular (kubectl get pod, kubectl describe service).
**Fix:** Rename releases → release for consistency with addon. The fix breaks anyone scripting okdctl releases list, but okdctl is pre-1.0 (per README §status) and the CHANGELOG can document the rename. Alternative: rename addon → addons. Either way, document the convention in CLAUDE.md so new groups follow it.
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


#### audit-observability

##### `obs:97cb8adf:span-no-start-end` — span no start end

**Status:** not started
**Severity:** suggestion
**Cluster:** span-retry-boundary
**Evidence:** `internal/system/exec.go:117-163`
**Problem:** WaitFor's log messages `"waiting"` (L117, L162) and `"ready"` (L133, L156) lack the `<subsystem>:` prefix used everywhere else in the repo — every other log site uses `dns:`, `haproxy:`, `apache:`, `tools:`, etc. The first arg of WaitFor IS `prefix` and IS exposed as a structured attr, but the message itself loses the grep-anchor. Additionally, the attr key `"for"` is a Go-reserved word and an ambiguous identifier compared to canonical `target`/`desc`.
**Fix:** Move the prefix into the message via concatenation: `logger.Info(prefix+": waiting", "target", description)`. Rename the attr key from `"for"` to `"target"` or `"desc"`. The change is mechanical and keeps the structured attrs intact while restoring the grep contract.
**Effort:** hours

#### audit-modernization

##### `mod:366b3f2d:use-slices-clone` — use slices clone

**Status:** in review — PR #803
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

##### `mod:9d79b841:use-strings-cut` — use strings cut

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/mod-9d79b841-strings-cut
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

##### `smell:262af6e4:abstraction-single-caller` — abstraction single caller

**Status:** not started
**Severity:** suggestion
**Cluster:** helper-package-no-value
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:24-56`
**Problem:** cleanup.Kind defines five values (Full, WorkOnly, WebOnly, HAProxyOnly, TerraformOnly) plus ValidKinds/KindStrings/IsValid/Validate helpers, but only Full and WorkOnly are referenced outside the package. WebOnly, HAProxyOnly, and TerraformOnly carry no callers; the switch in cleanupSteps covers them only to maintain the full enum shape.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `smell:c19ee328:magic-strings` — magic strings

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/smell-c19ee328-bin-consts
**Severity:** minor
**Cluster:** magic-strings
**Evidence:** `internal/distribution/okd/setup/steps.go:89-94`
**Problem:** setupBaseSteps lists the OKD tool trio as bare-string literals []string{openshift-install, oc, kubectl} even though openshiftInstallBin is already declared as a package-level constant in phase.go:L35 for the same binary. The other two names (oc, kubectl) are referenced as literal strings dozens of times across setup/install/postinstall/cleanup; a typo here equals a silent skip in the AlreadyDone guard.
**Fix:** Declare `const ocBin = "oc"` and `const kubectlBin = "kubectl"` alongside the existing openshiftInstallBin in setup/phase.go (or, since all three binaries cross package boundaries, hoist to internal/distribution/okd/phase). Replace the bare literals here and at the related sites. Net ~4 LOC added for the constants; removes pattern-wide silent-typo risk.
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



### Tier L — findings from 2026-06-10 /audit-all run

Filed by the orchestrator aggregation so `/roadmap-pickup` can fan them out when bandwidth opens. Each references the audit finding ID for diff tracking; when a finding recurs in a later run, its entry Status+Evidence updates here rather than being duplicated. Total: 141 findings (4 blocker, 33 major, 55 minor, 49 suggestion). Aggregated report: `.claude/audits/audit-all-2026-06-10.md`. Per-skill JSONL: `.claude/audits/audit-<skill>.jsonl`.

Recurring findings already tracked in earlier tiers (entries NOT duplicated here; status lives at the original entry):

- `state:0f076161:destroy-no-cluster-confirm-without-yes` — in progress (Tier K — worktree open)
- `state:fb54208a:postinstall-no-rollback-path` — deferred (earlier tier)
- `state:4f69fc9d:no-resume-checkpoint` — deferred (earlier tier)
- `api:4f69fc9d:iface-fragmented` — deferred (earlier tier)

#### audit-security

##### `sec:aa0f50f5:input-storage-name-unvalidated` — input storage name unvalidated

**Status:** in review — PR #813
**Severity:** major
**Cluster:** input-validation
**Evidence:** `internal/config/validators.go:255-281` + 1 more
**Problem:** validateProxmoxConfig validates Node and Storage against proxmoxNamePattern but leaves ISOStorage, DataStorage, Bridge, and CPUType entirely unvalidated. These flow verbatim into terraform.tfvars HCL string literals — buildISOStrings renders "%s:iso/%s%d.iso" — so a value containing a double quote breaks out of the HCL string and can inject arbitrary HCL into the generated tfvars.
**Fix:** In validateProxmoxConfig add proxmoxNamePattern checks for ISOStorage, DataStorage, and Bridge (mirroring the Storage check at validators.go:261-265) and constrain CPUType to a small allowlist or [A-Za-z0-9,_+.-]. Reject before the values reach buildTerraformVarsData / buildISOStrings.
**Effort:** hours

##### `sec:e3782ee7:symlink-escape` — symlink escape

**Status:** in review — PR #810
**Severity:** major
**Cluster:** file-toctou
**Evidence:** `internal/system/fs.go:122-170`
**Problem:** CopyFileMode (and CopyFile via it) open the destination with os.OpenFile(O_CREATE|O_WRONLY|O_TRUNC) plus a follow-up os.Chmod, neither using O_NOFOLLOW nor an Lstat symlink refusal — unlike the sibling AtomicWrite which guards both. Running as root under the sudo re-exec, a pre-planted symlink at a copy destination redirects the write (and the chmod) through the link.
**Fix:** Open dst with os.O_CREATE|os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW and add an os.Lstat symlink-refusal before the open, mirroring AtomicWrite (fs.go:203-215) and download.fetchToFile (download.go:169). Preserve mode-at-open-time; do NOT regress to Create+Chmod.
**Effort:** hours

##### `sec:19a715fd:symlink-escape` — symlink escape

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-19a715fd-nofollow
**Severity:** minor
**Cluster:** file-toctou
**Evidence:** `internal/addon/catalog/secretstore/secretstore.go:256-274`
**Problem:** readSecret stat()s then ReadFile()s a plaintext provider-secret file, both following symlinks; the permission gate uses os.Stat (not os.Lstat) and the read has no O_NOFOLLOW, unlike the readNoFollow / loadEnvFileOnce O_NOFOLLOW pattern used for every other secret read in the tree. During a sudo-re-exec deploy a symlink planted in the secrets dir redirects the read into an arbitrary root-readable file whose bytes are then base64'd into a Kubernetes Secret.
**Fix:** Read the file via an Lstat-symlink-refusal + os.OpenFile(O_RDONLY|syscall.O_NOFOLLOW) helper (mirror setup/ignition.go readNoFollow), and base the 0o077 perm gate on the Lstat result so the check and the read see the same inode.
**Effort:** hours

#### audit-subprocess

##### `sub:1e8ffb91:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** in review — PR #819
**Severity:** blocker
**Cluster:** io-handling — related: sub:d31d1b9d:ring-truncated-stdout-parse, sub:6b533f2d:ring-truncated-stdout-parse, sub:632c9087:ring-truncated-stdout-parse, sub:4c092fce:ring-truncated-stdout-parse
**Evidence:** `internal/distribution/okd/postinstall/verify.go:118-146`
**Problem:** VerifyClusterHealth machine-parses `oc get clusteroperators -o json` and `oc get nodes -o json` from Executor.Run's Result.Stdout, which is documented as a 200-line ring-buffer tail. ClusterOperator list JSON on any real OKD cluster runs thousands of lines (30+ operators with conditions/relatedObjects/managedFields), so the JSON head is always dropped and both parses fail on every live cluster.
**Fix:** Extend internal/executor with a full-capture variant (e.g. RunOutput with a byte cap, mirroring system.OutputCaptured's 4 MiB LimitReader) and route machine-parsed stdout through it; keep ring-tail Run for log/diagnostic capture. Do not bypass the canonical executor.
**Effort:** days

##### `sub:d31d1b9d:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** in review — PR #819
**Severity:** major
**Cluster:** io-handling — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/cli/status.go:155-187` + 1 more
**Problem:** `okdctl status` parses `oc get nodes -o json` and `oc get clusteroperators -o json` from OcOutput (ring-tail Run) and swallows the JSON parse error (`if jsonErr == nil` only-path). On a real cluster both payloads exceed 200 lines, so nodes silently come back empty and degraded silently stays 0 — status reports Unknown/Running for clusters it cannot actually see, with no diagnostic.
**Fix:** Route -o json reads through a full-capture executor variant (see sub:1e8ffb91) and surface jsonErr instead of discarding it (at minimum log at Warn so truncation is visible).
**Effort:** hours

##### `sub:6b533f2d:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** in review — PR #819
**Severity:** major
**Cluster:** io-handling — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/cluster/k8s_csrs.go:15-49`
**Problem:** PendingCSRs parses `oc get csr -o json` from the 200-line ring tail. Pretty-printed CSR objects (spec.request, conditions, managedFields) run 60-100 lines each, so 3+ pending CSRs truncate the JSON head and the parse fails exactly when approval matters most. MonitorInstallation only Warns on the final ApprovePendingCSRs failure, so worker serving-cert CSRs can be left unapproved while install reports success.
**Fix:** Use a full-capture executor variant for the JSON read, or fetch only names via a jsonpath/custom-columns query so output stays one line per CSR (bounded and ring-safe).
**Effort:** hours

##### `sub:e552bb7d:argv-unvalidated-token` — argv unvalidated token

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sub-e552bb7d-conn-validate
**Severity:** minor
**Cluster:** argv-construction
**Evidence:** `internal/netutil/iface.go:66-82` + 1 more
**Problem:** connectionForDevice returns a NetworkManager connection name parsed from nmcli output and RemoveSecondaryIP feeds it straight into `nmcli connection modify <conn> -ipv4.addresses ...` argv with no validation. nmcli property tokens are leading-dash-significant in this exact position (the call itself uses `-ipv4.addresses` for removal), so an odd connection name shaped like a property atom alters command semantics (CWE-88). The sibling code path in dns/dnsmasq.go validates the identical data with validateConnectionName; this one skips it.
**Fix:** Share dns/dnsmasq.go's validConnectionNameRegex check and apply it in connectionForDevice before returning the name; reject leading-dash names explicitly.
**Effort:** hours

##### `sub:632c9087:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** in review — PR #819
**Severity:** major
**Cluster:** io-handling — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:387-400`
**Problem:** discoverIngressControllers parses `oc get ingresscontroller -o json` from the ring tail and the per-item RawJSON it captures doubles as the rollback payload for attemptRollback. A list whose pretty-printed JSON exceeds 200 lines (one controller with managedFields is already borderline) fails discovery for the root-required, destructive update-ingress operation, and starves the rollback path of its source data.
**Fix:** Read the list via a full-capture executor variant (see sub:1e8ffb91); RawJSON rollback payloads must come from an untruncated stream.
**Effort:** hours

##### `sub:0934cf1b:no-timeout` — no timeout

**Status:** not started
**Severity:** suggestion
**Cluster:** timeout-cancel
**Evidence:** `internal/platform/packages.go:60-90` + 2 more
**Problem:** dnf/apt-get install/remove/update run via RunCaptured with no ctx deadline and stdout discarded: a wedged mirror or stale repo metadata hangs the deploy indefinitely with zero visible progress. Cancel wiring exists (cmd.Cancel SIGTERM + WaitDelay 30s, signal-watched root ctx), so Ctrl-C recovers — the gap is deadline + operator visibility only.
**Fix:** Wrap package-manager invocations in context.WithTimeout (generous, e.g. 15-20 min) mirroring ocExtractTimeout in release_extract.go, or stream output via the executor so a stall is at least visible. Risk: an aggressive timeout flakes slow mirrors — keep it generous.
**Effort:** hours

##### `sub:4c092fce:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** not started
**Severity:** minor
**Cluster:** io-handling — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/infrastructure/terraform/terraform.go:467-484`
**Problem:** Executor.Output unmarshals `terraform output -json` from the 200-line ring tail. Pretty-printed output maps beyond ~200 lines lose their head and fail the JSON parse. Fails loudly (invalid json error), but the failure mode is a latent capacity cliff unrelated to the actual terraform state.
**Fix:** Use a full-capture executor variant for `terraform output -json` (see sub:1e8ffb91). Current module output count fits in 200 lines, so this is a cliff, not an active break.
**Effort:** hours

##### `sub:19a715fd:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** not started
**Severity:** minor
**Cluster:** io-handling — seam→audit-security — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/addon/catalog/secretstore/secretstore.go:276-282`
**Problem:** readSecret returns `sops -d` plaintext from the ring tail. A decrypted secret longer than 200 lines (or a single line over the 64 KiB partial cap, which gets newline-split) is silently truncated/corrupted before being embedded in a cluster Secret manifest — no error, no truncation marker. Typical tokens are one short line, so likelihood is low, but the failure is silent credential corruption.
**Fix:** Decrypt through a full-capture executor variant (see sub:1e8ffb91) so secret material cannot be silently truncated; keep the stdout-not-argv channel for the plaintext.
**Effort:** hours

##### `sub:06f00bcb:exit-code-ignored` — exit code ignored

**Status:** in review — PR #802
**Severity:** minor
**Cluster:** io-handling
**Evidence:** `internal/distribution/okd/setup/apache.go:99-106`
**Problem:** a2enmod ssl / a2enconf ignition-ssl run via Exec.Run with only the transport error checked — Run returns err=nil for non-zero exits, so the intended Warn never fires when the command actually fails. The TLS vhost that serves credential-bearing ignition payloads depends on mod_ssl; failure surfaces only later as a confusing apache restart error.
**Fix:** Use RunChecked (typed *executor.ExitError on non-zero exit) so the Warn path actually fires, or inspect result.ExitCode explicitly if failure should stay non-fatal.
**Effort:** hours

##### `sub:696d6b0e:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** not started
**Severity:** suggestion
**Cluster:** io-handling — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/distribution/okd/phase/iso_cleanup.go:138-159` + 1 more
**Problem:** listProxmoxVMIDs / vmConfigReferencesISO parse pvesh JSON, and RemoveFCOSISOFromProxmox parses find -print0 output, all from the ring tail. pvesh emits single-line JSON: past 64 KiB the partial-buffer split inserts newlines mid-token and breaks the parse. Every consumer fails closed (skip removal / treat as in-use), so impact is a skipped cleanup, not data loss.
**Fix:** When the full-capture executor variant lands (sub:1e8ffb91), route pvesh/find SSH reads through it; current fail-closed behavior makes this non-urgent.
**Effort:** hours

##### `sub:29293401:ring-truncated-stdout-parse` — ring truncated stdout parse

**Status:** not started
**Severity:** suggestion
**Cluster:** io-handling — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/distribution/okd/setup/haproxy.go:186-200`
**Problem:** VerifyHAProxyPorts substring-scans `ss -tlnp` output from the ring tail; on a host with >200 listening sockets the head rows vanish and the check logs spurious 'may not be listening' warnings. Diagnostic-only path, warn-only outcome.
**Fix:** Narrow the query (`ss -tln sport = :6443` per port) or use the full-capture variant; warn-only today so opportunistic.
**Effort:** hours

#### audit-state-and-recovery

##### `state:0f076161:destroy-scoped-cleanup-unscoped` — destroy scoped cleanup unscoped

**Status:** not started
**Severity:** blocker
**Cluster:** destroy-safety — related: state:0f076161:destroy-no-cluster-confirm-without-yes
**Evidence:** `internal/cli/destroy.go:282-290` + 2 more
**Problem:** A scoped destroy (--only=bootstrap/masters/workers/vms or --target) only scopes the terraform step. StepCleanupFiles (cleanup.Full: deletes okd-install incl. kubeconfig + kubeadmin-password, haproxy config, dnsmasq drop-in, terraform.tfvars), StepCleanupFirewall, and StepRemoveRemoteISO still run cluster-wide, so `okdctl destroy --only=workers` tears down the bastion DNS/LB plumbing and credential files of the still-running control plane.
**Fix:** When destroyTargets is non-empty (from --only or --target), default SkipCleanup=true, SkipFirewall=true, KeepISOs=true (or set CleanupKind="") in runDestroy, and say so in the flag help; full bastion teardown stays exclusive to the unscoped destroy. Alternatively reject the combination unless the skip flags are passed explicitly.
**Effort:** days

##### `state:62cb8a95:corrupt-state-silent-destroy-noop` — corrupt state silent destroy noop

**Status:** in review — PR #812
**Severity:** major
**Cluster:** tf-state-atomicity — related: state:4c092fce:snapshot-bak-retention-after-destroy
**Evidence:** `internal/distribution/okd/destroy/helpers.go:27-30` + 1 more
**Problem:** HasState returns false for a corrupt/unparseable terraform.tfstate (warn only), and destroyInfrastructure treats false as 'already destroyed' and returns nil. A tfstate truncated by a crashed terraform run therefore makes destroy a silent no-op success: the summary says teardown completed, Full cleanup then removes tfvars/.terraform, and the orphaned VMs lose their recovery scaffolding — exactly the crash-mid-write scenario the snapshots exist for.
**Fix:** Split HasState into (hasResources bool, err error) or add a StateStatus() returning missing|empty|populated|corrupt. destroyInfrastructure returns a ClusterError on corrupt state naming the newest terraform.tfstate.*.bak snapshot and 'restore then re-run okdctl destroy'; only missing/empty states short-circuit to nil.
**Effort:** hours

##### `state:c287d5c0:prepare-wipes-live-cluster-artifacts` — prepare wipes live cluster artifacts

**Status:** in review — PR #820
**Severity:** blocker
**Cluster:** crash-recoverability — related: state:4f69fc9d:no-resume-checkpoint
**Evidence:** `internal/distribution/okd/okd.go:117-141` + 4 more
**Problem:** Provisioner.Prepare unconditionally wipes <projectRoot>/okd-install (cleanup.WorkOnly, PreserveConfig=false) before every deploy when the dir exists. Re-running `okdctl deploy` against an existing healthy cluster deletes cluster-config/auth/kubeadmin-password (the only plaintext copy) and the workdir kubeconfig with no confirmation, before any deploy step runs. It also makes every setup-step AlreadyDone sentinel dead code on the only call path, and a partial wipe failure is warn-and-continue, letting stale sentinels skip regeneration against a changed topology.
**Fix:** Before wiping, check whether the terraform env state has resources (tf.HasState) or cluster-config/auth exists; if so require explicit confirmation (or a --fresh flag) and always preserve cluster-config/auth (copy aside like SetupClusterAccess does for ~/.kube/config). Make pre-deploy cleanup failure fatal (or invalidate sentinels) so stale AlreadyDone sentinels cannot skip regeneration after a partial wipe.
**Effort:** days

##### `state:15ba17da:destroy-orphans-custom-isos` — destroy orphans custom isos

**Status:** not started
**Severity:** minor
**Cluster:** destroy-safety — related: state:0f076161:destroy-scoped-cleanup-unscoped
**Evidence:** `internal/distribution/okd/destroy/steps.go:93-115` + 2 more
**Problem:** StepRemoveRemoteISO only matches fedora-coreos-*.iso, but the VMs boot from the per-node custom ISOs setup uploads (bootstrap0.iso, master<N>.iso, worker<N>.iso — referenced as unmanaged cdrom file_ids in HCL, so terraform destroy never deletes them either). A full `okdctl destroy` therefore strands every multi-GB custom ISO on Proxmox storage indefinitely. Note the generated names carry no cluster prefix, so naive widening of the removal pattern could delete another cluster's ISOs on shared storage.
**Fix:** Extend the destroy ISO step to remove the exact node ISO names derived from cfg topology (bootstrap0.iso, master0..N.iso, worker0..N.iso) through the existing allowlist-validation layering; longer term, prefix generated ISO names with the cluster name so shared-storage collisions are impossible and removal stays name-exact.
**Effort:** hours

##### `state:262af6e4:cleanup-tfvars-proxy-precondition` — cleanup tfvars proxy precondition

**Status:** not started
**Severity:** minor
**Cluster:** phase-idempotency — related: state:368b892b:stale-bootstrap-sentinel-poisons-redeploy
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:235-261`
**Problem:** StepCleanupTerraform's AlreadyDone uses terraform.tfvars existence as a proxy for the whole artifact set. After a partial prior cleanup (tfvars removed, crash before .terraform/, lock file, plan files, or the post-destroy empty tfstate were handled), the re-run reports 'already done' and skips the remaining artifacts permanently — the precondition does not cover the step's full work product.
**Fix:** AlreadyDone should return true only when every entry in terraformFilesToRemove and the .terraform/ dir are absent (and, when PostDestroy, the empty tfstate is gone). The step is cheap; alternatively drop AlreadyDone and rely on idempotent SafeRemove.
**Effort:** hours

##### `state:368b892b:stale-bootstrap-sentinel-poisons-redeploy` — stale bootstrap sentinel poisons redeploy

**Status:** in review — PR #811
**Severity:** major
**Cluster:** phase-idempotency — related: state:fb54208a:postinstall-no-rollback-path
**Evidence:** `internal/distribution/okd/cleanup/infra.go:50-55` + 3 more
**Problem:** postinstall writes bootstrap-state.auto.tfvars.json ({"bootstrap_enabled": false}) into the terraform env dir, but cleanup's terraformFilesToRemove omits it and setup.GenerateTerraformVars never deletes it. Terraform loads *.auto.tfvars.json AFTER terraform.tfvars, so after destroy+cleanup (or a redeploy following a completed postinstall) the stale override beats the regenerated bootstrap_enabled=true: the next deploy creates no bootstrap VM and WaitForBootstrap fails after a 30-minute timeout with no diagnostic pointing at the file.
**Fix:** Add "bootstrap-state.auto.tfvars.json" to terraformFilesToRemove in cleanup/infra.go, and have setup.GenerateTerraformVars remove a stale sentinel when (re)rendering terraform.tfvars so a fresh deploy always starts with bootstrap_enabled=true.
**Effort:** hours

##### `state:6424733c:error-path-hint-always-destroy` — error path hint always destroy

**Status:** not started
**Severity:** minor
**Cluster:** crash-recoverability
**Evidence:** `internal/cli/helpers.go:349-385`
**Problem:** On a non-cancel Prepare failure executeFullDeployment prints 'run okdctl destroy to clean up resources' even though the prepare phase has applied nothing to Proxmox (terraform state empty). The cancel branch correctly distinguishes ('cancelled during prepare — terraform state is empty; run okdctl cleanup'). Operators who follow the error-path hint run a full destroy (sudo, terraform init, host-service teardown) when `okdctl cleanup` is the right verb.
**Fix:** Mirror the cancel-branch phase-aware hints on the error branch: prepare failure → 'okdctl cleanup'; install/configure failure → 'okdctl destroy'. The deploy-state marker already encodes the phase.
**Effort:** hours

##### `state:4c092fce:snapshot-bak-retention-after-destroy` — snapshot bak retention after destroy

**Status:** not started
**Severity:** suggestion
**Cluster:** tf-state-atomicity — seam→audit-security — related: state:62cb8a95:corrupt-state-silent-destroy-noop
**Evidence:** `internal/infrastructure/terraform/terraform.go:391-453` + 1 more
**Problem:** SnapshotState retains up to 5 terraform.tfstate.<ts>.bak files that nothing ever removes after teardown: Full cleanup deletes the empty live tfstate post-destroy but leaves the .bak snapshots (which contain the full pre-destroy resource state) in the env dir indefinitely. Recoverability-positive during destroy, but after a completed destroy+cleanup they are stale sensitive residue, and warnIfTfStateOnly's recovery-hint logic ignores them.
**Fix:** In the PostDestroy branch of StepCleanupTerraform, when the live tfstate is empty and removed, also remove terraform.tfstate.*.bak (or keep exactly the newest one and log its path as the rollback artefact, matching the CleanupPlans doc-comment philosophy).
**Effort:** hours

##### `state:881d089e:runlock-release-unlink-race` — runlock release unlink race

**Status:** not started
**Severity:** minor
**Cluster:** tf-state-atomicity
**Evidence:** `internal/runlock/runlock.go:110-119`
**Problem:** Lock.Release closes the fd then os.Remove(path). Unlinking a flock lockfile reintroduces the classic race: B opens the inode before A removes it, flocks the now-unlinked inode after A's close, while C creates a fresh file at the same path and flocks that — B and C hold 'the lock' concurrently on different inodes. Window is narrow and terraform's -lock-timeout is the authoritative backstop, but the unlink buys nothing except a tidy directory.
**Fix:** Drop the os.Remove and leave the (gitignored) zero-length .okdctl.lock in place — flock identity then always binds to a single stable inode. Optionally truncate the diagnostics on release instead.
**Effort:** hours

##### `state:08c49fc4:update-ingress-workdir-mispass` — update ingress workdir mispass

**Status:** in review — PR #814
**Severity:** major
**Cluster:** crash-recoverability
**Evidence:** `internal/cli/update_ingress.go:152-156` + 2 more
**Problem:** runUpdateIngress passes WorkDir: projectRoot, but UpdateIngressOptions.WorkDir is the okd-install workdir (parent of cluster-config/); the provisioner only substitutes <projectRoot>/okd-install when WorkDir is empty. RemoveHAProxy therefore looks for the kubeconfig CA at <projectRoot>/cluster-config/auth/kubeconfig, which never exists, so every `okdctl update-ingress` without --keep-haproxy fails the pre-flight AFTER the DNS swap and takes the rollback path (DNS reverted to bootstrap, haproxy restored) — the haproxy-removal cutover can never succeed from the CLI.
**Fix:** Pass WorkDir: "" (let okd.Provisioner.UpdateIngress default to filepath.Join(projectRoot, "okd-install")) or pass the joined path explicitly; add a regression test asserting RemoveHAProxy receives <workdir>/cluster-config.
**Effort:** hours

##### `state:4c092fce:destroy-direct-no-caller` — destroy direct no caller

**Status:** not started
**Severity:** suggestion
**Cluster:** destroy-safety
**Evidence:** `internal/infrastructure/terraform/terraform.go:343-363`
**Problem:** destroyDirect has no production caller — Destroy is always invoked with UsePlan=true. The doc comment declares it the retained 'emergency destroy' argv shape under regression coverage for a future opt-in caller. Scaffolding per MEMORY.md; recorded for the verify-intent ledger, not for deletion.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

#### audit-iac-and-shell

##### `iac:ef8f2924:count-exceeds-names` — count exceeds names

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/iac-ef8f2924-count-validate
**Severity:** minor
**Cluster:** hcl-robustness
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/variables.tf:148-245`
**Problem:** master_count validates up to 5 and worker_count up to 20, but the default master_names/worker_names lists each hold only 3 entries. main.tf derives masters/workers via slice(var.master_names, 0, var.master_count); when count exceeds the name-list length Terraform aborts with a cryptic 'slice end index out of range' instead of a clean validation message.
**Fix:** Add a validation tying counts to name-list length, e.g. on master_count: condition = var.master_count <= length(var.master_names) with a message telling the operator to extend master_names. Alternatively generate names ("${var.cluster_name}-master-${i}") instead of indexing a fixed list. Fails loudly with a clear message at plan time rather than an opaque slice error.
**Effort:** hours

##### `iac:ef8f2924:commented-out-vars` — commented out vars

**Status:** not started
**Severity:** suggestion
**Cluster:** hcl-doc-hygiene
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/variables.tf:252-265`
**Problem:** A 14-line block of commented-out variable declarations (bootstrap_kernel_args, master_kernel_args, worker_kernel_args) is left in the module. Commented-out code is rot — git history carries the prior intent; the dead block adds noise and invites accidental re-enablement against an interface the resources no longer wire up.
**Fix:** Delete lines 252-265. If kernel-args injection is a roadmap item, track it there rather than as commented HCL.
**Effort:** hours

##### `iac:bc37d034:lockfile-gitignored` — lockfile gitignored

**Status:** in review — PR #823
**Severity:** major
**Cluster:** hcl-provider-hygiene — seam→audit-dependencies
**Evidence:** `.gitignore:33-33`
**Problem:** The root .gitignore ignores .terraform.lock.hcl globally, so the provider dependency lock is never committed for the deployed root module (environments/production). Provider versions/hashes are re-resolved within the ~> 0.109.0 range on every init with no checksum pinning — the IaC analog of gitignoring go.sum. The only lock on disk (modules/proxmox-okd) is untracked cruft still pinning 0.108.0, stale after the 0.108->0.109 bump.
**Fix:** Narrow the ignore to runtime caches only (.terraform/ stays ignored) and force-commit the deployed root module's lock: `git add -f infrastructure/terraform/environments/production/.terraform.lock.hcl` after `terraform providers lock -platform=linux_amd64 -platform=linux_arm64`. Remove the stale untracked module lock or regenerate it at 0.109.0. HashiCorp explicitly recommends committing .terraform.lock.hcl.
**Effort:** hours

##### `iac:18a795d5:dup-insecure-comment` — dup insecure comment

**Status:** not started
**Severity:** suggestion
**Cluster:** hcl-doc-hygiene
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/main.tf:9-10` + 3 more
**Problem:** The PROXMOX_VE_INSECURE env var is documented twice on adjacent comment lines with conflicting framing — one calls it '(optional, set to true to disable tls verification)', the next '(DEV ONLY ... never set in prod)'. A copy-paste artifact duplicated verbatim across four .tf files; the softer line undercuts the security warning.
**Fix:** Delete the softer 'optional, set to true' line in all four files (modules/.../main.tf, modules/.../variables.tf, environments/production/variables.tf, environments/production/versions.tf header), keeping only the DEV-ONLY warning so the security framing is unambiguous.
**Effort:** hours

##### `iac:e076e43c:token-in-argv` — token in argv

**Status:** not started
**Severity:** suggestion
**Cluster:** install-sh-integrity
**Evidence:** `scripts/install.sh:109-117`
**Problem:** GITHUB_TOKEN is passed to curl as an -H 'Authorization: Bearer ...' argv element. During the brief curl exec it is visible in the process table (ps / /proc/PID/cmdline) to other local users. Low blast radius (the user's own token, short-lived, only when GITHUB_TOKEN is set), but argv is the wrong channel for a bearer secret.
**Fix:** Pass the header off-argv via a curl config file on stdin: `printf 'header = "Authorization: Bearer %s"\n' "$GITHUB_TOKEN" | curl_safe --config - ...`. Keeps the token out of the process table on shared/CI hosts.
**Effort:** hours

#### audit-errors

##### `err:4c092fce:ctx-err-unwrapped` — ctx err unwrapped

**Status:** in review — PR #805
**Severity:** major
**Cluster:** cancellation-identity — related: err:7b2829bb:ctx-err-unwrapped
**Evidence:** `internal/infrastructure/terraform/terraform.go:145-152` + 1 more
**Problem:** Executor.run() and Output() construct &ExecError{} directly from result.ExitCode, bypassing executor.NewExitError. os/exec prefers the process *ExitError over ctx.Err() (Cmd.Wait: 'If c.Process.Wait returned an error, prefer that'), so a SIGINT-cancelled terraform subcommand returns an ExecError with no context.Canceled in its chain — signalExitCode and cli/destroy.go's errors.Is(err, context.Canceled) both miss, mapping Ctrl-C to exit 2/4 instead of 130/143 on paths the orchestrator loop-top ctx check does not rescue (CleanupBootstrap apply, StartWorkerVMs, Output, dry-run init).
**Fix:** Replace both direct &ExecError{} constructions with executor.NewExitError(ctx, "terraform "+args[0], result.ExitCode, result.Stderr), the canonical ctx-aware constructor already used by RunChecked/RunStreamedChecked and cluster/k8s.go runCheck.
**Effort:** hours

##### `err:15ba17da:nonfatal-failures-exit-zero` — nonfatal failures exit zero

**Status:** in review — PR #809
**Severity:** major
**Cluster:** typed-error-exit-mapping — seam→audit-state-and-recovery — related: state:6424733c:error-path-hint-always-destroy, state:62cb8a95:corrupt-state-silent-destroy-noop
**Evidence:** `internal/distribution/okd/destroy/steps.go:171-192` + 1 more
**Problem:** Every destroy step is NonFatal and the summary step only logs tracked failures at Warn then returns nil, so orchestrator.Run returns nil after a failed terraform destroy: the CLI prints 'cluster destroyed' and exits 0 with live VMs still standing. The sibling cleanup package's summary step returns errors.Join(t.errs...) precisely so callers receive a joined error — destroyTracker keeps only []string labels and drops the error values.
**Fix:** Have destroyTracker retain []error alongside labels and make the summary step return &errtypes.ClusterError{Msg: "destroy finished with failed steps", Err: errors.Join(t.errs...)} when failures is non-empty (exit 4), mirroring cleanup/cleanup.go's cleanupSummaryStep. Keep the other steps NonFatal so continuation semantics are unchanged.
**Effort:** hours

##### `err:a4001485:vocab-gap-transient` — vocab gap transient

**Status:** not started
**Severity:** suggestion
**Cluster:** domain-vocabulary
**Evidence:** `internal/errtypes/errtypes.go:5-13` + 3 more
**Problem:** Three near-identical ad-hoc retryability classifiers (proxmox.initIsRetryable, addon.addonIsRetryable, download.isRetryable) encode the same transient-vs-permanent concept outside the errtypes vocabulary. The package doc records this as a deliberate deferral until a retry-aware consumer lands (roadmap err:9f8e7d6c); snapshot row keeps the gap visible — it persists this run.
**Fix:** When the roadmap consumer lands, add errtypes.TransientError{Msg, Err} (Unwrap-chaining) and collapse the three classifiers into errors.As-based checks; until then no action — the deferral is documented at the type-definition site.
**Effort:** hours

##### `err:5e892064:vocab-ad-hoc-synonym` — vocab ad hoc synonym

**Status:** not started
**Severity:** minor
**Cluster:** domain-vocabulary
**Evidence:** `internal/download/checksum.go:74-74` + 1 more
**Problem:** HTTP-status failures rendered as bare strings ('failed to fetch checksums: HTTP %d', 'github api returned status %d') while the same package defines HTTPStatusError specifically so isRetryable can fail-fast on 4xx and retry 5xx. setup/coreos.go already wraps download.HTTPStatusError cross-package. If FetchChecksum or the releases fetcher are ever placed under retryDownload (as Fetch is), 404s silently degrade from fail-fast to retry-everything.
**Fix:** Return &HTTPStatusError{Status: resp.StatusCode, Method: http.MethodGet, URL: checksumsURL} in FetchChecksum, and fmt.Errorf("github api: %w", &download.HTTPStatusError{...}) in releases/fetcher.go, matching the fetchToFile and setup/coreos.go idiom.
**Effort:** hours

##### `err:7b2829bb:ctx-err-unwrapped` — ctx err unwrapped

**Status:** in review — PR #806
**Severity:** major
**Cluster:** cancellation-identity — related: err:4c092fce:ctx-err-unwrapped
**Evidence:** `internal/executor/executor.go:349-374` + 3 more
**Problem:** RunInteractive returns cmd.Run()'s error raw. On ctx cancellation the SIGINT soft-cancel makes the child exit non-zero, so os/exec returns the process *exec.ExitError and discards ctx.Err() — context.Canceled never enters the chain. Consumers wrap it into ConfigError/NetworkError (destroy --dry-run PlanStreamed → exit 2; deploy --dry-run PlanOnly → exit 2; scp ISO upload → exit 3) instead of the documented 130.
**Fix:** In RunInteractive, after cmd.Run(): if err != nil { if ctxErr := ctx.Err(); ctxErr != nil { return ctxErr } } (mirrors NewExitError's precedence). Consider the same for the non-ExitError branch of run()/RunStreamed for symmetry.
**Effort:** hours

##### `err:40d315ad:vocab-ad-hoc-synonym` — vocab ad hoc synonym

**Status:** not started
**Severity:** minor
**Cluster:** domain-vocabulary
**Evidence:** `internal/addon/catalog/flux/flux.go:93-96` + 1 more
**Problem:** Settings-validation and unknown-provider failures — config-class errors — are returned as untyped fmt.Errorf ('flux: invalid settings: %w', 'secretstore: unknown provider %q') while sibling sites in the same files use errtypes.ConfigError ('helm is required', 'flux repository not configured'). Manager.installAndVerify wraps untyped errors in ClusterError, so the same misconfiguration class exits 4 or 2 depending on which constructor the addon author picked.
**Fix:** Return &errtypes.ConfigError{Msg: "flux: invalid settings", Err: err} (and the secretstore equivalents for invalid-settings/unknown-provider) so exitCodeFor's errors.As chain-walk maps them to exit 2 like their typed siblings.
**Effort:** hours

##### `err:d9f7733e:err-formats-cred` — err formats cred

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/err-d9f7733e-bundle-redact
**Severity:** minor
**Cluster:** redaction-in-error
**Evidence:** `internal/cli/debug_bundle.go:276-280` + 1 more
**Problem:** bundleTerraformState and bundleMustGather copy raw subprocess stderr (strings.TrimSpace(result.Stderr)) into the support-bundle manifest Message, bypassing both this file's safeMessage() Redacted()-dispatch helper and the ExitError.Error() RedactableStderr policy. The command's help text promises the bundle is 'safe to attach to a support ticket — credentials are redacted'; a credential-bearing terraform/oc diagnostic would land verbatim in the operator-shared artifact.
**Fix:** Route both Messages through fmt.Sprint(logutil.RedactableStderr(msg).Redacted()) — or construct an executor.ExitError and pass it to safeMessage — so stderr is truncated/redacted the same way every other stderr-bearing error is.
**Effort:** hours

##### `err:aa84670c:exit-mapping-nesting-precedence` — exit mapping nesting precedence

**Status:** not started
**Severity:** suggestion
**Cluster:** typed-error-exit-mapping — seam→audit-cli-ux
**Evidence:** `internal/cli/root.go:212-251`
**Problem:** exitCodeFor checks the five category types in fixed order, and each errors.As walks the whole chain — so an inner ConfigError outranks an outer ClusterError regardless of nesting depth (ClusterError{Msg:'addon flux install failed', Err: …ConfigError} exits 2, not 4). The sentinel-over-category precedence is documented in-line; the category-over-category nesting precedence is not, in either the package doc or docs/cli/exit-codes.md.
**Fix:** Document the precedence ('sentinels outrank categories; among categories, Config > Network > Cluster > Auth > Usage wins anywhere in the chain — root-cause type, not outermost wrap, decides the code') in the exitCodeFor doc comment and docs/cli/exit-codes.md; alternatively switch to an outermost-wins single-Unwrap walk if wrap-level classification is the intended contract.
**Effort:** hours

##### `err:ddf885f4:vocab-ad-hoc-synonym` — vocab ad hoc synonym

**Status:** in review — PR #798
**Severity:** minor
**Cluster:** domain-vocabulary
**Evidence:** `internal/addon/manager.go:167-170`
**Problem:** InstallOne returns the Resolve() dependency-resolution error bare (exit 1), while InstallAll wraps the identical failure in &errtypes.ConfigError{Msg: "addon dependency resolution failed"} (exit 2). Same concept, two classifications: 'okdctl addon install --all' and 'okdctl addon install <name>' exit differently for the same circular-dependency config mistake.
**Fix:** Wrap as in InstallAll: return &errtypes.ConfigError{Msg: "addon dependency resolution failed", Err: err}.
**Effort:** hours

#### audit-concurrency

##### `con:6424733c:go-no-wait` — go no wait

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/con-6424733c-metrics-bind
**Severity:** minor
**Cluster:** goroutine-lifetime
**Evidence:** `internal/cli/helpers.go:269-290`
**Problem:** startMetricsServer launches `go func() { errCh <- srv.ListenAndServe() }()` and immediately logs "metrics endpoint listening" without observing startup outcome. A bind failure (port in use, EPERM) sits in the buffered errCh until stop() drains it at deploy exit, so the operator sees a success log up front and the real error only after a multi-hour deploy — or never, if a phase error path logs it as a shutdown warning.
**Fix:** Bind synchronously, then serve in the goroutine: `ln, err := net.Listen("tcp", addr); if err != nil { return nil, nil, &errtypes.ConfigError{...} }; go func() { errCh <- srv.Serve(ln) }()`. Bind errors then surface before the deploy starts and the "listening" log becomes truthful. stop() drain logic is unchanged (Serve also returns ErrServerClosed on Shutdown).
**Effort:** hours

##### `con:ae5b624c:ticker-zero-interval-panic` — ticker zero interval panic

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/con-ae5b624c-ticker-guard
**Severity:** suggestion
**Cluster:** time-sleep-retry
**Evidence:** `internal/distribution/okd/install/monitor.go:117-118`
**Problem:** MonitorInstallation passes opts.CSRApprovalInterval straight to time.NewTicker, which panics on a non-positive duration. The only production constructor (install.NewOptions) always sets DefaultCSRApprovalInterval, but Options is an exported plain struct — any future caller building Options literally hits a runtime panic 30+ minutes into an install. The repo's own polling helper guards this case.
**Fix:** Mirror system.WaitFor's zero-guard: `interval := opts.CSRApprovalInterval; if interval <= 0 { interval = DefaultCSRApprovalInterval }` before NewTicker. 3 LOC, no behavior change for existing callers.
**Effort:** hours

#### audit-api-design

##### `api:2c4d8e6b:iface-in-producer` — iface in producer

**Status:** not started
**Severity:** minor
**Cluster:** interface-location
**Evidence:** `internal/addon/addon.go:50-92` + 3 more
**Problem:** ConfigurableAddon and WizardProvider are implemented by both catalog addons (DefaultSettings/ValidateSettings/DecodeSettings/WizardFields) but no code anywhere type-asserts or consumes them. The intended consumer (wizard addons step) instead imports flux/secretstore concretely and hand-builds the same field catalog, so addon wizard fields, defaults, and validation now live in two places that can drift. Not scaffolding: the consumer exists and bypassed the contract, so the duplication cost is current, not future.
**Fix:** Either wire the wizard addons step to iterate addon.All() and type-assert WizardProvider/ConfigurableAddon to render fields generically (deleting the hand-built duplicates), or record a decision that the wizard owns its field layout and retire the unconsumed interfaces. Verify intent with the owner before either move (MEMORY.md scaffolding protocol).
**Effort:** hours

##### `api:7b2829bb:ring-tail-contract` — ring tail contract

**Status:** not started
**Severity:** minor
**Cluster:** exported-surface — seam→audit-subprocess — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/executor/executor.go:25-31` + 2 more
**Problem:** Executor.Run's Result.Stdout/Stderr silently carry only the last 200 lines with no truncation signal and no full-capture sibling API; callers that parse stdout as JSON (8 sites per the wave-1 subprocess audit) consume the tail as if complete. The per-site misuse is owned by audit-subprocess; the API gap — no additive full-capture path — is the design defect here.
**Fix:** Add an additive full-capture API: either RunFull(ctx, name, args...) with a byte-capped full buffer, or a WithCaptureLimit/CaptureAll per-call option, plus a Result.Truncated bool so parse-callers can fail loudly. Migrate the 8 JSON-parsing call sites onto it; streaming/log paths keep the ring.
**Effort:** hours

##### `api:d6b325cb:pkg-import-cycle-adj` — pkg import cycle adj

**Status:** not started
**Severity:** minor
**Cluster:** package-boundary
**Evidence:** `internal/infrastructure/proxmox/types.go:1-51` + 4 more
**Problem:** internal/infrastructure/proxmox imports internal/distribution/okd/phase for domain vocabulary (NodeRole, VMState) and remote pvesh helpers, inverting infra-under-distribution layering; pvesh.go's own comment concedes the helpers are stranded in phase/ only because moving them would cycle (proxmox already imports phase).
**Fix:** Extract NodeRole/VMState/Condition* plus the pvesh/SSH remote-op helpers (pvesh.go, ssh.go, iso_cleanup.go remote bits) into a neutral leaf package (e.g. internal/proxmoxops) imported by both phase and infrastructure/proxmox. Reverses the infra->distribution edge and dissolves the alias re-export block in types.go.
**Effort:** hours

##### `api:c287d5c0:pkg-facade-bypassed` — pkg facade bypassed

**Status:** not started
**Severity:** minor
**Cluster:** package-boundary
**Evidence:** `internal/distribution/okd/okd.go:146-154` + 2 more
**Problem:** okd.Provisioner facade is asymmetric: Prepare/Configure build their phase Options internally, but Install requires cli to import okd/install and hand it install.NewOptions(cfg, projectRoot) verbatim. Separately, cli/deploy.go deployDryRunSteps hand-duplicates 32 step IDs AND display names already declared in the phases' StepDefs, importing setup/install/postinstall just for the constants - a drift-prone second source of truth.
**Fix:** (1) Have Provisioner.Install build install.NewOptions(cfg, p.projectRoot) internally, matching Configure; (2) add Provisioner.DeploySteps() returning ID+Name derived from the same xSteps() StepDef slices so the dry-run listing cannot drift. cli/deploy.go then drops its okd/setup, okd/install, okd/postinstall imports (postinstall.Result/UpdateIngressResult remain legitimately exposed).
**Effort:** hours

##### `api:5e892064:ctx-missing-on-io` — ctx missing on io

**Status:** not started
**Severity:** minor
**Cluster:** ctx-first
**Evidence:** `internal/download/checksum.go:21-54` + 2 more
**Problem:** CalculateChecksum/ValidateChecksum stream entire files through sha256 with no ctx; on multi-GB CoreOS ISOs this runs tens of seconds inside Fetch's skip-check and post-download verification, so Ctrl-C cannot interrupt the hash even though every caller already holds a ctx.
**Fix:** Add ctx as first parameter (CalculateChecksum(ctx, path)) and copy via a ctx-checking reader (check ctx.Err() per 1-4 MiB chunk). Callers Fetch, canSkipDownload, verifyDownloadedFile, and ExtractTarGz already have ctx in scope.
**Effort:** hours

##### `api:d31d1b9d:pkg-facade-bypassed` — pkg facade bypassed

**Status:** not started
**Severity:** minor
**Cluster:** package-boundary — seam→audit-code-smells
**Evidence:** `internal/cli/status.go:388-404` + 1 more
**Problem:** cli/status constructs phase.NewBasePhase to reach BasePhase.OcOutput for oc queries, while internal/cluster.Client exists as the thin oc wrapper (with WithKubeconfig built for exactly this). CLAUDE.md scopes BasePhase.Oc* to phase code; cluster.Client's surface is too narrow (no exported output method), so cli reached for distribution internals instead.
**Fix:** Add an exported Output(ctx, args ...string) (string, error) to cluster.Client (it already wraps executor + KUBECONFIG injection) and switch cli status/describe to cluster.New(cluster.WithKubeconfig(kcPath)). BasePhase.Oc* stays untouched for phase code.
**Effort:** hours

##### `api:ddf885f4:zero-value-unusable` — zero value unusable

**Status:** in review — PR #798
**Severity:** minor
**Cluster:** zero-value-usability
**Evidence:** `internal/addon/manager.go:48-57`
**Problem:** NewManager's doc claims nil-safety matching phase.NewBasePhase, but only the logger is defaulted: a Manager built without WithExecutor carries a nil *executor.Executor into Environment.Exec and nil-derefs deep inside the first addon Install. NewBasePhase, by contrast, materializes a default executor.
**Fix:** After applying options in NewManager, default m.exec = executor.New(executor.WithLogger(m.logger)) — the exact contract phase.NewBasePhase documents and that NewManager's own doc cites.
**Effort:** hours

##### `api:0934cf1b:iface-in-producer` — iface in producer

**Status:** not started
**Severity:** suggestion
**Cluster:** interface-location
**Evidence:** `internal/platform/packages.go:18-56`
**Problem:** NewPackageManager returns the PackageManager interface rather than *Manager (violating 'accept interfaces, return structs'), and the interface methods take a per-call *slog.Logger (Remove even ignores it) while every other repo type injects the logger at construction.
**Fix:** Return *Manager from NewPackageManager and move the logger to a Manager field (constructor arg or option), dropping the per-call logger params. Keep the PackageManager interface only if a consumer-side fake needs it — then declare it consumer-side (setup/cleanup).
**Effort:** hours

##### `api:297adb3e:opt-inconsistent` — opt inconsistent

**Status:** not started
**Severity:** suggestion
**Cluster:** option-consistency
**Evidence:** `internal/config/validation_types.go:131-142`
**Problem:** Config.Validate(opts ...ValidationOptions) is a variadic-struct hybrid that silently ignores opts[1:], and coexists with ValidateWithOptions(cfg, opts) — two public entry points for the same call, in a codebase that otherwise uses functional options for variadic configuration.
**Fix:** Make Validate() take no arguments (ScopeAll default) and keep ValidateWithOptions for scoped callers; or fold to a single Validate(opts ValidationOptions) — drop the silent opts[1:] discard either way.
**Effort:** hours

##### `api:484b40f0:zero-value-unusable` — zero value unusable

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/api-484b40f0-zero-recorder
**Severity:** suggestion
**Cluster:** zero-value-usability
**Evidence:** `internal/deploymetrics/metrics.go:20-34`
**Problem:** A zero-value Recorder compiles as a distribution.MetricsRecorder but panics in StepFinished (write to nil stepTotal map) the first time a step completes; only NewRecorder initialises the maps and nothing guards the zero value.
**Fix:** Lazy-init the maps inside StepFinished under the existing mutex (two nil checks), making the zero value usable; or document the NewRecorder-only contract on the type.
**Effort:** hours

##### `api:e45c2239:pkg-facade-bypassed` — pkg facade bypassed

**Status:** not started
**Severity:** minor
**Cluster:** package-boundary
**Evidence:** `cmd/okdctl/main.go:16-44` + 1 more
**Problem:** cmd/okdctl imports internal/distribution/okd/phase solely for PreflightBinDir; cli/doctor.go likewise for ResolveBinDir/DefaultBinDir. Bin-dir resolution depends only on config+system, so the binary entry point is coupled to OKD phase internals for a generic path helper.
**Fix:** Move ResolveBinDir/PreflightBinDir/BinDirOrDefault/envBinDir/validateAndClean (phase/paths.go L53-L111) into internal/config next to ValidateBinDir (their only validation dependency); re-export or update the four phase-internal call sites. cmd/ and cli/doctor then stop importing distribution internals.
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

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/httputil/httputil.go:18-26`
**Problem:** TimeoutDownload has no caller and is marked scaffolding for a future file-download caller — but the natural caller already exists: download.DefaultTimeout hand-rolls the identical 5-minute value instead of consuming the tier constant, so the scaffolding premise is stale.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:21dc1103:opt-inconsistent` — opt inconsistent

**Status:** not started
**Severity:** suggestion
**Cluster:** option-consistency
**Evidence:** `internal/download/download.go:40-57` + 1 more
**Problem:** Two functional-option families share package download with collision-driven, asymmetric prefixes: Fetch has WithFetchChecksum but unprefixed WithLogger/WithTimeout/WithDescription/WithOverwrite; Extract has WithExtractChecksum and WithExtractLogger but unprefixed WithStripComponents/WithCleanupArchive. Callers must memorize which member of each pair carries a prefix.
**Fix:** Pick one rule: prefix every Extract option (WithExtractStripComponents...) or split extraction into download/extract subpackage so both families use clean unprefixed names. Subpackage split is the cleaner long-term shape.
**Effort:** hours

##### `api:fde34e0c:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/cluster/k8s.go:51-72`
**Problem:** cluster.WithEnvFallback has zero call sites anywhere (production or test); it completes the Option family with an env-driven discovery mode documented for non-phase callers that have not landed.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:2be6306e:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/addon/registry.go:86-94`
**Problem:** addon.IsRegistered has no caller; its doc names the future 'okdctl addon validate' verb. The Registry type is also exported although all access flows through package-level functions over the unexported global.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:859eea6f:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/noderole.go:21-32`
**Problem:** ParseNodeRole has no caller; retained as the documented deserialization counterpart to NodeRole.String() for upcoming status-JSON/terraform-output parsing.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:3e02f6b8:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/vmstate.go:10-19`
**Problem:** Only StateRunning is consumed; StateStopped/Creating/Deleting/Unknown complete the pvesh status matrix for the future partial-cluster status path, per the in-code scaffolding marker.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:0fc0041d:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/condition.go:14-29`
**Problem:** ConditionTypeAvailable/Progressing and ConditionStatusUnknown are unreferenced; the const groups intentionally mirror the full Kubernetes condition matrix for the future status verb that surfaces non-Ready operator conditions.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:9ce5434c:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/phase/kubectl.go:44-53`
**Problem:** OcPollOutputInterval is exported solely as the test-injection seam for polling cadence; production code must use OcPollOutput per its doc. Exported-but-test-only surface, retained by the in-code scaffolding marker.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:48688e63:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** ctx-first
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:150-162`
**Problem:** Disconnect accepts a ctx it never uses, kept for signature symmetry with Connect (which now genuinely uses ctx via sshpin.Verify) and for a future network-bound teardown; documented in-code with the same tracking ID.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:de572c63:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/distribution/okd/dns/dnsmasq.go:49-64` + 17 more
**Problem:** Pattern-wide: the dns, cleanup, and setup phase packages export symmetric helper families with no callers outside their own package — dns service ops (Enable/Restart/ValidateDnsmasqConfig, ConfigureSystemResolver, IsNetworkManagerActive), cleanup subsystem funcs (WorkDirectory, WebServer, Dnsmasq, Packages, IgnitionCerts, GenerateSummary, ValidKinds...), setup builders (BuildLiveKargs, BuildDestKargs, ExtractNetworkConfig, EnsureIgnitionCert...). Symmetric-API shaped, so severity capped; alternative is a mechanical unexport sweep.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

##### `api:588ce79e:export-no-caller-scaffolding` — export no caller scaffolding

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-surface
**Evidence:** `internal/tui/colors.go:9-18`
**Problem:** ColorTheme/ThemeDefault/ThemeHighContrast are exported for a future 'okdctl theme' verb; today only the unexported setTheme consumes them via the HOMELAB_HIGH_CONTRAST env init. MEMORY.md records the user rejecting --color/lipgloss profile wiring, so the future-verb premise deserves an explicit owner check — argued here, not silently upgraded.
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.
**Effort:** hours

#### audit-cli-ux

##### `ux:073d24ed:concept-named-twice` — concept named twice

**Status:** in review — PR #821
**Severity:** major
**Cluster:** verb-noun — seam→audit-documentation
**Evidence:** `internal/cli/deploy.go:38-70` + 3 more
**Problem:** The 'which config file is this cluster' concept has two names: deploy selects its config via --output-file (stat/load/save at L67-70, L96, L126) and never reads the root --config flag, while destroy/status/cleanup/addon/config all key off --config. deploy's own Example advertises 'okdctl deploy --config my-cluster.yaml', which is silently ignored — deploy proceeds against okdctl.yaml.
**Fix:** Make deploy honor --config: when the root --config flag is Changed, use cfgFile as the load/save target and keep --output-file only as an explicit override for wizard output; alternatively drop the misleading `deploy --config` Example and document that deploy is keyed on --output-file. Keep credentials.EnvFilePath derivation in sync with whichever file is chosen.
**Effort:** hours

##### `ux:073d24ed:yes-flag-divergent-semantics` — yes flag divergent semantics

**Status:** in review — PR #821
**Severity:** major
**Cluster:** flag-conventions — related: state:0f076161:destroy-no-cluster-confirm-without-yes
**Evidence:** `internal/cli/deploy.go:48-97`
**Problem:** `okdctl deploy --yes` (help: "skip prompts, use defaults") saves the config file and exits without deploying anything, while --yes on destroy/cleanup/update-ingress/addon-uninstall executes the operation. There is no non-interactive deploy path at all; a scripted `deploy --yes` silently produces only okdctl.yaml.
**Fix:** Either (a) make `deploy --yes` proceed to runFullDeployment after saveConfig when the config validates (gated by the existing --confirm-cluster pattern for parity with destroy), or (b) minimally re-word the flag help and Long to "write configuration non-interactively; does not deploy" so the contract is explicit.
**Effort:** hours

##### `ux:073d24ed:exit-code-undefined` — exit code undefined

**Status:** in review — PR #821
**Severity:** minor
**Cluster:** exit-codes — seam→audit-errors
**Evidence:** `internal/cli/deploy.go:73-102` + 1 more
**Problem:** Several config-flavored failure paths return untyped fmt.Errorf and land on the generic exit 1 where the published taxonomy assigns code 2: invalid existing config in non-interactive mode (L74-75), wizard failure (L101), and debug-bundle output-path symlink/create refusals. Scripts branching on exit 2 per docs/cli/exit-codes.md miss these.
**Fix:** Wrap these returns in &errtypes.ConfigError{Msg: ..., Err: err} so exitCodeFor maps them to 2, matching the documented taxonomy. The per-type mapping validation itself belongs to audit-errors (seam #4).
**Effort:** hours

##### `ux:4583b75b:json-quiet-inconsistent` — json quiet inconsistent

**Status:** in review — PR #797
**Severity:** minor
**Cluster:** streams
**Evidence:** `internal/cli/config.go:63-66`
**Problem:** runConfigShow validates --output but is the only JSON-capable command that skips quietForJSON; `okdctl config show -o json 2>&1 | jq` sees info chatter that status/releases/doctor/version/addon suppress. Inconsistent stream contract across siblings.
**Fix:** Add quietForJSON(configShowOutput) after validateFormat, matching runStatus/runReleasesList/runDoctor/versionCmd.
**Effort:** hours

##### `ux:024a2c32:json-exit-contract-drift` — json exit contract drift

**Status:** not started
**Severity:** minor
**Cluster:** json-stability
**Evidence:** `docs/cli/json-schema.md:236-239` + 2 more
**Problem:** The Conventions section claims 'okdctl sets exit code 0 on JSON success even when the underlying state is degraded', but `addon verify --output=json` returns the joined ClusterError (exit 4) when any probe fails — its own Long says 'Exit code is non-zero if any probe fails' — and `doctor --output=json` exits 2 on failing checks. The blanket convention only holds for `status`.
**Fix:** Scope the Conventions bullet to `okdctl status` and explicitly list the exceptions (doctor exits 2 on fail, addon verify exits 4 on probe failure) so script authors get one truthful contract. Do not change the shipped exit behavior — both commands document their codes elsewhere.
**Effort:** hours

##### `ux:024a2c32:json-schema-undoc` — json schema undoc

**Status:** not started
**Severity:** suggestion
**Cluster:** json-stability — related: sub:d31d1b9d:ring-truncated-stdout-parse
**Evidence:** `docs/cli/json-schema.md:56-67` + 2 more
**Problem:** `okdctl status --output=json` always emits nodes[].status (NodeStatusReady/NotReady, set unconditionally in runStatus) and the NodeStatus struct can additionally emit version, internal_ip, and conditions via omitempty, but the documented field table stops at name/role/ready. The stability promise ('field names are stable') covers fields consumers cannot discover from the doc.
**Fix:** Add nodes[].status to the field table (values: Ready|NotReady|Unknown) and note version/internal_ip/conditions as optional fields reserved for future population, or strip the unused omitempty fields from the CLI-facing projection.
**Effort:** hours

##### `ux:aa84670c:exit-code-undefined` — exit code undefined

**Status:** not started
**Severity:** suggestion
**Cluster:** exit-codes
**Evidence:** `internal/cli/root.go:157-170`
**Problem:** signalLoop's second-strike path calls exit(130) unconditionally, so a double SIGTERM (e.g. systemd stop escalation) exits 130 (SIGINT code) instead of 143, contradicting the published taxonomy that maps SIGTERM to 143.
**Fix:** Capture the second signal value and exit 143 when it is syscall.SIGTERM, 130 otherwise. Preserve the documented close(sigCh)-after-signal.Stop ordering (con:aa84670c).
**Effort:** hours

##### `ux:e7db1220:flag-completion-inconsistent` — flag completion inconsistent

**Status:** not started
**Severity:** suggestion
**Cluster:** flag-conventions
**Evidence:** `internal/cli/releases.go:77-83` + 2 more
**Problem:** Enum-valued flags get shell completion inconsistently: --channel (releases list) and --only (destroy) register RegisterFlagCompletionFunc, but the eight --output text|json flags plus --log-level and --log-format complete nothing despite having closed value sets validated at runtime.
**Fix:** Add a shared helper that registers cobra.FixedCompletions([]string{outputText, outputJSON}, NoFileComp) wherever flagOutput is bound, plus log-level (debug,info,warn,error) and log-format (text,json) on the root persistent flags.
**Effort:** hours

##### `ux:daf5bee9:stdout-not-cobra-writer` — stdout not cobra writer

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/ux-daf5bee9-cobra-writer
**Severity:** suggestion
**Cluster:** streams
**Evidence:** `internal/cli/kubeconfig.go:41-70`
**Problem:** runKubeconfig is the only RunE that writes user data to os.Stdout directly (and discards its *cobra.Command parameter) instead of cmd.OutOrStdout(); every sibling routes data output through the cobra writer, which is what makes them testable and SetOut-respecting.
**Fix:** Accept cmd, write via cmd.OutOrStdout().Write(data). No behavior change in production (OutOrStdout defaults to os.Stdout).
**Effort:** hours

##### `ux:fd2125dd:verb-noun-inconsistent` — verb noun inconsistent

**Status:** not started
**Severity:** suggestion
**Cluster:** verb-noun
**Evidence:** `internal/cli/addon.go:27-31` + 1 more
**Problem:** Sibling noun groups disagree on number: `okdctl addon list` (singular) vs `okdctl releases list` (plural). Same grammatical position, two conventions — users must remember which group pluralizes.
**Fix:** Both names are shipped; renaming is a breaking change bigger than the smell. If desired, add a `release` alias (cobra Aliases) to releasesCmd or `addons` alias to addonCmd and standardize in docs; otherwise record the choice and keep new noun groups singular.
**Effort:** hours

##### `ux:e7db1220:help-long-missing` — help long missing

**Status:** not started
**Severity:** suggestion
**Cluster:** help-text
**Evidence:** `internal/cli/releases.go:40-47` + 2 more
**Problem:** A handful of leaf commands ship Short+Example but no Long: releases list (whose --channel stable-vs-all semantics live only in flag help), releases show, config show, describe node, describe addon. Siblings of equal weight (addon list, addon verify) carry Longs, so --help depth is uneven across the tree.
**Fix:** Add 1-3 sentence Longs to releases list (explain channel filtering and the disk cache), releases show, config show (note that text mode emits YAML), describe node/addon. Regenerate docs/cli via cmd/okdctl-gen-docs afterward.
**Effort:** hours

#### audit-observability

##### `obs:41a9d4eb:log-leaks-cred` — log leaks cred

**Status:** in review — PR #808
**Severity:** minor
**Cluster:** redaction-sink — seam→audit-errors — related: err:d9f7733e:err-formats-cred
**Evidence:** `internal/logutil/redact.go:126-139`
**Problem:** RedactableStderr.Redacted() bounds length (200-byte head + tail) but performs no content scrubbing, so a credential inside a short (≤400-byte) subprocess stderr — e.g. a provider auth diagnostic echoing a token — reaches the sink verbatim despite routing through the redaction middleware. The doc comment markets truncation as preventing credential dumps; it only prevents long ones.
**Fix:** Add a pattern scrub inside Redacted(): mask value tokens following key fragments from secretKeyFragments (password=, token:, Authorization:) in the retained head/tail before returning, keeping the truncation behavior intact.
**Effort:** hours

##### `obs:aa84670c:log-leaks-cred` — log leaks cred

**Status:** not started
**Severity:** suggestion
**Cluster:** redaction-sink
**Evidence:** `internal/cli/root.go:105-105`
**Problem:** Startup audit line joins the entire argv into a single string attr: tui.LF("argv", strings.Join(os.Args[1:], " ")). Key-based redaction cannot see inside a pre-joined string, so if a future flag ever carries a token/password the value is logged verbatim. No current okdctl flag accepts a secret (creds flow via env/.env file), so this is hardening, not a live leak.
**Fix:** Log argv through a Redacted()-implementing type (mirroring the ExitError.Command argv-redaction contract canaried in errtypes tests) or scrub key=value tokens whose key matches logutil.KeyIsSecret before joining.
**Effort:** hours

##### `obs:48688e63:key-inconsistent-casing` — key inconsistent casing

**Status:** not started
**Severity:** minor
**Cluster:** field-stability
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:220-293` + 1 more
**Problem:** The same concept (number of VMs in a terraform plan) is keyed "count" in Provision (L220) but "vm_count" in the dry-run plan preview (L293), so a jq/grep filter on one key silently misses the other.
**Fix:** Standardise on one key (suggest "vm_count") at both sites; "count" at L259 keys provisioned-VM totals and may stay if treated as a different concept, otherwise align it too.
**Effort:** hours

##### `obs:ed55ee90:key-inconsistent-casing` — key inconsistent casing

**Status:** in review — PR #801
**Severity:** minor
**Cluster:** field-stability
**Evidence:** `internal/distribution/okd/cleanup/summary.go:66-81`
**Problem:** The clean/dirty branches of the same summary metric use different keys: clean logs "files", 0 while the dirty branch logs "count", N (work dir, ignition, terraform). One metric, two key spellings, so a consumer cannot select the remaining-file count with a single key.
**Fix:** Use one key per metric in both branches, e.g. "remaining", summary.RemainingWorkFiles (0 on the clean branch); applies to all three clean/remaining pairs in printSummary.
**Effort:** hours

##### `obs:a6e38cc7:key-inconsistent-casing` — key inconsistent casing

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/obs-a6e38cc7-fp-key
**Severity:** minor
**Cluster:** field-stability
**Evidence:** `internal/sshpin/sshpin.go:80-82` + 1 more
**Problem:** The two fingerprint-pinning warn sites key the observed SSH host fingerprints differently: sshpin uses "observed", flux uses "observed_fingerprints". Same concept, same remediation flow, two key names.
**Fix:** Pick one key (suggest "observed_fingerprints" for self-description) and use it at both sites.
**Effort:** hours

##### `obs:40d315ad:level-warn-should-info` — level warn should info

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/obs-40d315ad-flux-level
**Severity:** minor
**Cluster:** level-discipline
**Evidence:** `internal/addon/catalog/flux/flux.go:124-126` + 2 more
**Problem:** Remediation/advice continuation lines are logged as separate Warn records after the real warning: flux logs three consecutive Warns where only the first ("git sync not ready") reports degradation — the "debug with: ..." and "will auto-reconcile" lines are guidance. Same shape at iso-upload and dns OnError handlers. Inflates Warn counts and pages on advice text.
**Fix:** Keep the first record at Warn and either fold the advice into structured attrs on it (e.g. "hint", "oc get gitrepository ...") or demote the follow-up lines to Info.
**Effort:** hours

##### `obs:21dc1103:level-error-not-user-visible` — level error not user visible

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/obs-21dc1103-dl-level
**Severity:** minor
**Cluster:** level-discipline — seam→audit-errors
**Evidence:** `internal/download/download.go:127-128` + 1 more
**Problem:** Log-and-return at Error: Fetch logs "download: giving up after retries" at Error then returns a NetworkError that cli/root.go logs again at Error ("command failed"), so one failure produces two ERROR lines. helpers.go:35 has the same shape for missing config (Error + ConfigError return + boundary Error).
**Fix:** Demote the inner record to Warn (it adds the attempts attr the boundary lacks) or enrich the returned error with attempts and drop the inner log; helpers.go:35 can demote to Warn since exitCodeFor already maps ErrConfigMissing to 66 and root logs the failure.
**Effort:** hours

#### audit-modernization

##### `mod:d9f7733e:use-builtins` — use builtins

**Status:** not started
**Severity:** suggestion
**Cluster:** any-interface-builtins
**Evidence:** `internal/cli/debug_bundle.go:361-365`
**Problem:** Hand-rolled clamp (assign, then if-greater reassign) where the min builtin (Go 1.21) collapses four lines to one.
**Fix:** cappedSize := min(actualSize, maxBundleFileBytes) — keep actualSize, it is still used for the truncation decision downstream.
**Effort:** hours

##### `mod:a3577f6c:use-slices-containsfunc` — use slices containsfunc

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/mod-a3577f6c-containsfunc
**Severity:** suggestion
**Cluster:** slices-maps
**Evidence:** `internal/distribution/okd/setup/cert.go:77-81`
**Problem:** Hand-rolled membership loop over cert.IPAddresses that also re-runs net.ParseIP(ip) on every iteration; slices.ContainsFunc (Go 1.21) with the parse hoisted expresses the SAN check in one line.
**Fix:** want := net.ParseIP(ip); if want != nil && slices.ContainsFunc(cert.IPAddresses, want.Equal) { return certRaw, keyRaw, true }. Keep net.IP throughout — crypto/x509.Certificate.IPAddresses is a []net.IP external contract, so no netip conversion.
**Effort:** hours

##### `mod:48688e63:use-slices-containsfunc` — use slices containsfunc

**Status:** not started
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

##### `mod:88fd3050:use-omitzero` — use omitzero

**Status:** in review — PR #800
**Severity:** minor
**Cluster:** encoding-omitzero
**Evidence:** `internal/config/cluster.go:23-66` + 2 more
**Problem:** Four struct-typed fields (Disks, Bootstrap, StaticIP, Bastion) carry json:",omitempty" tags that encoding/json ignores on non-pointer struct fields — the tag is a no-op and zero-valued sections are always serialized into okdctl.yaml. Go 1.24's omitzero tag does what these tags intend.
**Fix:** Replace omitempty with omitzero on the four struct-typed fields (encoding/json, Go 1.24; sigs.k8s.io/yaml marshals through encoding/json so the tag is honored). Loading is unaffected; saved YAML drops all-zero optional sections, matching the tag's stated intent. Verify any golden-file config tests after the change.
**Effort:** hours

##### `mod:696d6b0e:use-strings-splitseq` — use strings splitseq

**Status:** in review — PR #799
**Severity:** minor
**Cluster:** range-idioms
**Evidence:** `internal/distribution/okd/phase/iso_cleanup.go:212-212` + 1 more
**Problem:** Two range-over-strings.Split loops allocate the full []string when only iteration is needed; strings.SplitSeq (Go 1.24 range-over-func iterator) avoids the slice allocation and is the established repo idiom (10+ existing SplitSeq/Lines/FieldsSeq sites).
**Fix:** for _, seg := range strings.Split(v, ",") -> for seg := range strings.SplitSeq(v, ","); same for the \x00 split in parseNullDelimitedFileList. Drop the discarded index; loop bodies unchanged.
**Effort:** hours

##### `mod:632c9087:use-slices-sort` — use slices sort

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/mod-632c9087-slices-max
**Severity:** minor
**Cluster:** slices-maps
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:625-626`
**Problem:** sort.Strings(matches) followed by taking matches[len(matches)-1] sorts the whole slice only to read its maximum; slices.Max (Go 1.21) expresses the intent directly in O(n) and removes the last `sort` import in scope.
**Fix:** latest := slices.Max(matches) (len==0 already guarded just above); delete the sort import. Lexicographic ordering of the .backup.* suffixes is preserved exactly — slices.Max uses the same string ordering as sort.Strings.
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

**Status:** not started
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

##### `smell:073d24ed:stepdef-name-duplication` — stepdef name duplication

**Status:** not started
**Severity:** minor
**Cluster:** magic-strings
**Evidence:** `internal/cli/deploy.go:192-226`
**Problem:** deployDryRunSteps hardcodes all 32 step ID/Name pairs ("install system packages", "wait for bootstrap", ...) duplicating the StepDef.Name strings declared in setup/steps.go, install/steps.go, and postinstall/steps.go. Adding, removing, or renaming a step in a phase silently desynchronizes the --dry-run listing; nothing fails at compile or test time.
**Fix:** Make each phase export a static name table next to its StepID consts (e.g. `var StepNames = map[distribution.StepID]string{...}` populated from the same literals the StepDefs use, or hoist Name literals to consts referenced by both StepDef and the table). deployDryRunSteps then ranges the three tables in execution order. Risk is medium only because StepDef construction needs cfg/opts, so the listing must come from a side table, not from building real steps.
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

##### `smell:de572c63:magic-path-literal` — magic path literal

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/smell-de572c63-resolved-var
**Severity:** minor
**Cluster:** magic-strings
**Evidence:** `internal/distribution/okd/dns/dnsmasq.go:208-210`
**Problem:** ConfigureSystemResolver hardcodes "/etc/systemd/resolved.conf.d" + "/dnsmasq.conf" inline while the package already declares the test-injectable var resolvedConf (L30) holding the identical path for RestoreSystemResolver. The write path ignores the injection seam (tests overriding resolvedConf still target the real /etc path) and the two spellings can drift, leaving restore unable to find what configure wrote.
**Fix:** Derive both from the single var: confPath := resolvedConf; confDir := filepath.Dir(resolvedConf). Configure then honors the same test override Restore already uses.
**Effort:** hours

##### `smell:2c4d8e6b:interfaceany-lazy-exported` — interfaceany lazy exported

**Status:** not started
**Severity:** suggestion
**Cluster:** interfaceany-lazy — seam→audit-api-design
**Evidence:** `internal/addon/addon.go:58-58` + 4 more
**Problem:** ConfigurableAddon.DecodeSettings returns (any, error), but grep shows zero generic consumers: the only callers are the implementing addons themselves, each immediately type-asserting its own concrete Settings (decoded.(Settings)) — an unchecked assertion that would panic if the method ever returned a different type. The any in the interface buys no polymorphism and costs a panic path per addon.
**Fix:** Either drop DecodeSettings from the interface (each addon keeps a package-private typed decode — nothing external calls it generically today), or keep the interface method but have each addon call an unexported typed decodeSettings internally so the any round-trip and unchecked assertions disappear. If a future generic consumer (wizard validation) is planned, document it on the interface; the interface-shape decision itself is audit-api-design territory.
**Effort:** hours

##### `smell:40d315ad:settings-stringified-numbers` — settings stringified numbers

**Status:** not started
**Severity:** suggestion
**Cluster:** stringified-numbers
**Evidence:** `internal/addon/catalog/flux/flux.go:581-588` + 4 more
**Problem:** controller_timeout/git_sync_timeout defaults are stored as the strings "300"/"180" and re-parsed with strconv.Atoi at each use via getTimeout reading the raw settings map — bypassing the typed Settings struct that DecodeSettings exists to produce (the struct simply omits both fields). A malformed value silently falls back to the default instead of failing validation.
**Fix:** Add ControllerTimeout/GitSyncTimeout time.Duration fields to flux.Settings, parse once in DecodeSettings (returning an error on malformed input so ValidateSettings surfaces it), and have waitForControllers/waitForGitSync read the struct. Delete getTimeout. The map[string]string wire format stays — only the parse point moves.
**Effort:** hours

##### `smell:0f076161:enum-ad-hoc` — enum ad hoc

**Status:** not started
**Severity:** suggestion
**Cluster:** magic-strings
**Evidence:** `internal/cli/destroy.go:109-130`
**Problem:** validateDestroyTargets switches on the raw regex capture with case "bootstrap" / "master" / "worker" string literals even though phase.NodeRole typed constants (RoleBootstrap/RoleMaster/RoleWorker) and ParseNodeRole exist exactly for this vocabulary, and destroy.go already imports phase. The same literals also appear baked into destroyTargetRE — acceptable there (regex alternation), but the switch should speak the typed vocabulary.
**Fix:** switch phase.NodeRole(m[1]) { case phase.RoleBootstrap: ... case phase.RoleMaster: ... case phase.RoleWorker: } — or run the capture through phase.ParseNodeRole, which exists as the canonical deserializer (its doc note "currently no caller" resolves itself).
**Effort:** hours

##### `smell:91abd90c:magic-number-duplicated` — magic number duplicated

**Status:** in review — PR #804
**Severity:** suggestion
**Cluster:** magic-strings
**Evidence:** `internal/config/defaults.go:54-54` + 1 more
**Problem:** The default Proxmox VMID base 6000 is a bare literal in two packages: config/defaults.go seeds it into DefaultConfig, and proxmox.go's probeVMEnumeration re-hardcodes 6000 as the zero-value fallback. If the default ever changes in one place, the enumeration probe checks for the wrong VMID and the suppress-per-VM-logs heuristic silently inverts.
**Fix:** Export `const DefaultVMIDBase = 6000` in internal/config (next to the other Min*/Default* constants) and reference it from both sites.
**Effort:** hours

##### `smell:15ba17da:magic-label-sentinel` — magic label sentinel

**Status:** not started
**Severity:** suggestion
**Cluster:** magic-strings
**Evidence:** `internal/distribution/okd/destroy/steps.go:63-67` + 1 more
**Problem:** destroyTracker.terraformFailed gates ISO/firewall/cleanup-scope decisions on slices.Contains(t.failures, "terraform destroy") — a free string that must stay byte-identical to the label passed at track("terraform destroy") 23 lines away. A label reword breaks the downstream skip logic silently (ISO removal would run against live VMs).
**Fix:** const labelTerraformDestroy = "terraform destroy" used by both sites — or track failures by distribution.StepID (StepDestroyInfra), which is already a stable identifier per step.go's contract.
**Effort:** hours

##### `smell:6179837f:linter-config-bug` — linter config bug

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/smell-6179837f-goconst
**Severity:** suggestion
**Cluster:** linter-config — related: smell:451be4fa:magic-path-literal, smell:0139cb3f:magic-path-literal
**Evidence:** `.golangci.yml:50-51`
**Problem:** goconst is enabled with min-occurrences 3 / min-len 3, yet none of the heavy repeated literals in this repo fire ("flux-system" 16x in one file, "okd-install" 13x, the 4-segment terraform path 16x) because goconst's ignore-calls option defaults to true and virtually every occurrence is a function-call argument. The linter as configured can only catch repeats in assignments/comparisons, which this codebase rarely produces.
**Fix:** Evaluate `goconst: ignore-calls: false` in .golangci.yml. It would have flagged the magic-string findings in this report (smell:451be4fa, smell:0139cb3f, flux-system). Risk: expect noise on oc/helm argv literals ("-n", "get") — likely needs min-occurrences raised to 5+ or per-path exclusions to be tolerable; trial on a branch before adopting.
**Effort:** hours

#### audit-dependencies

##### `dep:2eef5feb:automerge-v0x-minor` — automerge v0x minor

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/dep-2eef5feb-automerge
**Severity:** minor
**Cluster:** pin-stability — related: dep:33ef32bf:transitive-heavy-narrow, dep:98cc639a:v0x-unregistered-provider
**Evidence:** `.github/renovate/autoMerge.json5:1-12`
**Problem:** Renovate automerges every minor/patch update across all managers ('matchManagers': ['*']), but CLAUDE.md itself states 'v0.x APIs may break on any minor bump' — go-proxmox (v0.7.x) and the bpg/proxmox terraform provider (0.109.x) receive breaking-capable bumps with zero human review (e.g. provider 0.108.0→0.109.0 landed as an automerged 'minor'). CI gates compile/lint/validate but cannot catch behavioral breakage in Proxmox API interactions.
**Fix:** Add a packageRule: { matchCurrentVersion: "<1.0.0", matchUpdateTypes: ["minor"], automerge: false } (or scope it to the gomod + terraform managers) so v0.x minor bumps require a human eyeball while v0.x patches and >=1.0 minors keep automerging.
**Effort:** hours

##### `dep:b803fcb7:version-floor-unjustified` — version floor unjustified

**Status:** not started
**Severity:** minor
**Cluster:** pin-stability
**Evidence:** `.github/workflows/ci.yml:56-56` + 2 more
**Problem:** Go-tool pins in CI and the Makefile are explicit versions (policy-compliant) but sit outside every renovate manager — the custom regex manager only matches annotated lines in .env/.sh/.yaml and the gomod manager only reads go.mod — so they rot silently: govulncheck pinned v1.1.4 vs latest v1.3.0 (the security gate itself is two minors stale), yamlfmt v0.14.0 vs v0.21.0, air v1.61.7 vs v1.65.3.
**Fix:** Add renovate annotations above each pin ('# renovate: datasource=go depName=golang.org/x/vuln') and extend customManagers.json5 managerFilePatterns to cover /\.yml$/ and /Makefile$/; or move govulncheck/yamlfmt to go.mod 'tool' directives (Go 1.24+) so the gomod manager tracks them; then bump govulncheck to v1.3.0, yamlfmt to v0.21.0, air to v1.65.3.
**Effort:** hours

##### `dep:33ef32bf:dup-log-engines` — dup log engines

**Status:** not started
**Severity:** suggestion
**Cluster:** duplicate-engine
**Evidence:** `go.mod:11-11` + 3 more
**Problem:** Four log engines compile into the production binary: stdlib log/slog (canonical sink per CLAUDE.md), charm.land/log/v2 (direct, single call-site styled stderr formatter), and go-logr/logr + k8s.io/klog/v2 (pinned by k8s.io/apimachinery, both linked per go list -deps). CLAUDE.md carries a YAML-engine baseline tripwire but no equivalent log-engine baseline, so a fifth engine could land without a recorded justification.
**Fix:** Record a log-engine baseline in CLAUDE.md §dependencies mirroring the YAML tripwire: slog = canonical, charm.land/log/v2 = intentional UI formatter (1 file), logr/klog = k8s-pinned indirects; do not add a fifth without justification. No code change — charm libs are the intentional UI stack and klog/logr are upstream-locked.
**Effort:** hours

##### `dep:98cc639a:v0x-unregistered-provider` — v0x unregistered provider

**Status:** in review — PR #823
**Severity:** suggestion
**Cluster:** pin-stability — seam→audit-iac-and-shell — related: iac:bc37d034:lockfile-gitignored, dep:2eef5feb:automerge-v0x-minor
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/versions.tf:6-11` + 1 more
**Problem:** The bpg/proxmox terraform provider is a v0.x dependency of the shipped IaC (~> 0.109.0) but is absent from CLAUDE.md's v0.x justification registry, which lists only Go modules (go-proxmox). With .terraform.lock.hcl gitignored repo-wide, the provider also has no committed hash pins, so the v0.x risk is doubly untracked.
**Fix:** Add a CLAUDE.md v0.x registry entry for bpg/proxmox (justification: only actively maintained Proxmox VE provider; abandonment plan: fall back to Telmate/proxmox or direct API via null_resource) and commit .terraform.lock.hcl per the iac finding so digests pin the provider.
**Effort:** hours

##### `dep:6ebdb617:dep-registry-drift` — dep registry drift

**Status:** not started
**Severity:** minor
**Cluster:** maintenance-signal
**Evidence:** `CLAUDE.md:234-247` + 1 more
**Problem:** CLAUDE.md dependency registry has drifted from go.mod in three places: go-proxmox is documented at v0.5.x but go.mod pins v0.7.1; the joho/godotenv LICENCE-spelling entry remains but the dep is no longer in go.mod or go.sum; §tooling says 'Go 1.25 / toolchain 1.26.2' while go.mod declares go 1.26.0 / toolchain go1.26.4.
**Fix:** Update CLAUDE.md: bump go-proxmox registry row to v0.7.x, delete the godotenv entry (dep removed), and correct §tooling to 'Go 1.26 / toolchain 1.26.4' (or make it version-agnostic: 'go.mod go/toolchain directives are authoritative; don't downgrade').
**Effort:** hours

##### `dep:33ef32bf:version-floor-unjustified` — version floor unjustified

**Status:** not started
**Severity:** minor
**Cluster:** justified-version-floor — related: dep:6ebdb617:dep-registry-drift
**Evidence:** `go.mod:39-39` + 1 more
**Problem:** Transitive version floors inherited from go-proxmox are years stale and both packages compile into the shipped binary: gorilla/websocket v1.4.2 (Mar 2020; latest v1.5.3, Jun 2024) and jinzhu/copier v0.3.4 (2021; latest v0.4.0). CLAUDE.md claims okdctl 'does not reach' websocket — true at the call-graph level, but `go list -deps ./cmd/okdctl` shows both modules linked into the release artifact.
**Fix:** go get github.com/gorilla/websocket@v1.5.3 github.com/jinzhu/copier@v0.4.0 && go mod tidy — okdctl calls neither API directly, so the floor lift is behavior-neutral; also sharpen the CLAUDE.md websocket entry to say 'linked but never called' rather than 'does not reach it'.
**Effort:** hours

##### `dep:b803fcb7:pin-action-trailer-imprecise` — pin action trailer imprecise

**Status:** not started
**Severity:** minor
**Cluster:** pin-stability — related: dep:6ebdb617:dep-registry-drift
**Evidence:** `.github/workflows/ci.yml:15-19` + 6 more
**Problem:** All actions are SHA-pinned (policy-compliant) but several version trailers are major-only ('# v6', '# v9', '# v7', '# v4', '# v2') where CLAUDE.md prescribes 'uses: owner/action@<40-hex-sha> # vX.Y.Z' — a reviewer cannot tell which minor/patch a 40-hex digest corresponds to without resolving it. The same files contain the precise form as counter-examples (setup-tflint '# v6.2.2', cosign-installer '# v4.1.2', slsa generator '# v2.1.0').
**Fix:** Point renovate at precise tags so the trailer renders as vX.Y.Z (pin actions to the full release tag before digest-pinning, e.g. actions/checkout@<sha> # v6.0.2), or amend CLAUDE.md to permit major-tag trailers for renovate-managed digests — pick one so policy and practice agree.
**Effort:** hours

##### `dep:33ef32bf:transitive-heavy-narrow` — transitive heavy narrow

**Status:** not started
**Severity:** suggestion
**Cluster:** transitive-weight — related: dep:33ef32bf:version-floor-unjustified
**Evidence:** `go.mod:12-12` + 1 more
**Problem:** go-proxmox v0.7.1 is imported in exactly one file (wizard REST discovery) yet links 5 extra modules into the release binary: diskfs/go-diskfs (full ISO9660/MBR filesystem stack), buger/goterm, jinzhu/copier, djherbis/times, gorilla/websocket. Known case per CLAUDE.md §dependencies (bus-factor 1, ~200 LOC REST-rewrite fallback); re-confirmed this run: upstream is active (v0.7.1 released 2026-06-02, 267 stars, Apache-2.0, 0 open issues) and the abandonment plan remains valid.
**Fix:** No action now — re-confirmation of the accepted trade. If upstream stalls or a v0.8 minor breaks the wizard, execute the documented fallback: ~200 LOC net/http REST client for the discovery endpoints, dropping 6 modules from the binary.
**Effort:** hours

#### audit-documentation

##### `doc:8f46b665:docs-struct-snippet-drift` — docs struct snippet drift

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/doc-8f46b665-phases
**Severity:** minor
**Cluster:** readme-drift
**Evidence:** `docs/architecture/phases.md:86-98` + 2 more
**Problem:** phases.md's BasePhase snippet omits the Reporter (logutil.ProgressReporter) field that the real struct carries, and both the StepDef snippet comment ("AlreadyDone ... optional") and the adding-a-step guide ("For resume safety, wire an AlreadyDone func") present AlreadyDone as advisory — BuildSteps panics when ReRunSafe==ReRunSafeNo and AlreadyDone is nil, so it is mandatory for every ReRunSafeNo step.
**Fix:** Add Reporter to the BasePhase snippet; change the AlreadyDone guidance to "required for ReRunSafeNo steps — BuildSteps panics without it" in both the L46-47 snippet comment and step 3 of the adding-a-step guide.
**Effort:** hours

##### `doc:b3356305:readme-flag-ghost` — readme flag ghost

**Status:** in review — PR #822
**Severity:** major
**Cluster:** readme-drift — seam→audit-cli-ux — related: ux:073d24ed:concept-named-twice, doc:073d24ed:readme-flag-ghost
**Evidence:** `README.md:97-99`
**Problem:** README tells users "--config other.yaml manages multiple clusters" in the deploy walkthrough, but `okdctl deploy` never reads the persistent --config flag — runDeploy reads only deployOutputFile (--output-file, default okdctl.yaml). A user following the README silently deploys against okdctl.yaml, not other.yaml.
**Fix:** Reword README L99 to `--output-file other.yaml` for deploy (and note --config selects the config for status/destroy/addon/etc.), or wait for the ux:073d24ed fix that makes deploy honor --config and then leave README as-is — coordinate with that finding before editing.
**Effort:** hours

##### `doc:073d24ed:readme-flag-ghost` — readme flag ghost

**Status:** in review — PR #821
**Severity:** major
**Cluster:** readme-drift — seam→audit-cli-ux — related: ux:073d24ed:concept-named-twice, doc:b3356305:readme-flag-ghost
**Evidence:** `internal/cli/deploy.go:38-41`
**Problem:** deployCmd's Example string shows `okdctl deploy --config my-cluster.yaml`, but deploy reads only --output-file. The example renders in `okdctl deploy --help` and is faithfully regenerated into docs/cli/okdctl_deploy.md:17, propagating the wrong flag into the entire CLI reference.
**Fix:** Drop or fix the `--config my-cluster.yaml` Example line (use --output-file), then `make docs` to regenerate docs/cli/okdctl_deploy.md. If ux:073d24ed instead wires deploy to read --config, regenerate docs after that change.
**Effort:** hours

##### `doc:b3356305:readme-default-drift` — readme default drift

**Status:** in review — PR #822
**Severity:** major
**Cluster:** readme-drift
**Evidence:** `README.md:176-178` + 2 more
**Problem:** README calls the credentials file ".env" three times, but credentials.EnvFilePath derives `okdctl.env` from the config name (okdctl.yaml → okdctl.env). The Uninstall section's `rm -rf ~/okd-install okdctl.yaml .env` therefore leaves okdctl.env — the file holding PROXMOX_VE_PASSWORD / PROXMOX_VE_API_TOKEN — on disk after a documented full uninstall.
**Fix:** Replace `.env` with `okdctl.env` in the Uninstall command (L177) and reword L97-98 / L128-130 to "an okdctl.env file next to the config (named after the config file)".
**Effort:** hours

##### `doc:b3356305:readme-behavior-drift` — readme behavior drift

**Status:** in review — PR #822
**Severity:** major
**Cluster:** readme-drift
**Evidence:** `README.md:234-252` + 2 more
**Problem:** README's "Ignition pull-secret exposure window" section and the bootstrap troubleshooting entry document ignition serving as plain HTTP on port 8080, but the code serves ignition exclusively over HTTPS on 443 with a pinned CA embedded in the node ISOs (BuildIgnitionURLForNode builds https:// URLs; DefaultIgnitionHTTPSPort=443; ConfigureApache writes a TLS vhost). The advertised mitigation ("firewalld rule scoping port 8080") targets a port the feature no longer uses; apache.go's own log strings still say "port 8080" while dialing :443.
**Fix:** Rewrite README L234-252 for the HTTPS model: TLS vhost on 443 bound to ignition_server_ip, CA pinned via `coreos-installer iso customize --ignition-ca`, residual risk = any host reaching the bridge IP on 443 still gets the files (TLS authenticates the server, not the client). Fix L159-162 to "ignition server IP, port 443, https". Also correct the two stale "port 8080" log strings in apache.go:71,75 to 443.
**Effort:** hours

##### `doc:b3356305:readme-dead-link` — readme dead link

**Status:** in review — PR #822
**Severity:** major
**Cluster:** readme-drift
**Evidence:** `README.md:30-32` + 2 more
**Problem:** The primary documented install path fetches raw.githubusercontent.com/qxtaiba/okdctl/main/scripts/install.sh, but the remote has no `main` branch (only develop; origin/HEAD → develop), so the curl 404s. The same URL is printed by the in-binary update notice (root.go:192) and in install.sh's own usage header.
**Fix:** Either create/maintain a `main` branch (or make it the default) so the pinned URL resolves, or switch all three occurrences (README.md:31, internal/cli/root.go:192, scripts/install.sh:10) to .../develop/scripts/install.sh or a release-tag URL. Pick one canonical URL; the update notice and README must match.
**Effort:** hours

##### `doc:1013f4e8:docs-behavior-drift` — docs behavior drift

**Status:** in review — PR #822
**Severity:** major
**Cluster:** readme-drift — seam→audit-cli-ux
**Evidence:** `docs/cli/exit-codes.md:25-27`
**Problem:** exit-codes.md claims "Invoking commands like `deploy` or `destroy` directly as root is rejected with code 5 ... use `sudo okdctl …` instead". The elevation policy is the exact inverse: euid=0 on a root-requiring command (deploy/destroy/cleanup/update-ingress) is ALLOWED (it is the re-exec'd privileged body); the code-5 rejection fires for non-root-requiring commands run under sudo (e.g. `sudo okdctl status`). The advice also contradicts README L71-73 ("Don't run okdctl as root").
**Fix:** Rewrite L25-27: "Commands that do not need root (status, config, kubeconfig, …) are rejected with code 5 when invoked under sudo/root; root-requiring commands (deploy, destroy, cleanup, update-ingress) self-elevate via an internal sudo re-exec — invoke them as a regular user."
**Effort:** hours

##### `doc:024a2c32:docs-schema-drift` — docs schema drift

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/doc-024a2c32-nodes-omitempty
**Severity:** minor
**Cluster:** readme-drift — seam→audit-cli-ux
**Evidence:** `docs/cli/json-schema.md:236-236` + 2 more
**Problem:** json-schema.md's stability contract says "`null` is never emitted — fields that are absent are omitted entirely", but okd.ClusterStatus.Nodes has no omitempty: when `oc get nodes` fails, runStatus leaves the slice nil and `okdctl status --output=json` emits "nodes": null. The status field table also omits nodes[].status (NodeStatusPhase), which is always populated in the output.
**Fix:** Honor the doc: add `,omitempty` to ClusterStatus.Nodes (or initialize nodes := []okd.NodeStatus{} in runStatus so it emits []), and add a `nodes[].status` row (Ready|NotReady) to the field table. Doc says unknown keys must be tolerated, so documenting status is additive.
**Effort:** hours

##### `doc:aa84670c:doc-comment-stale` — doc comment stale

**Status:** not started
**Severity:** minor
**Cluster:** exported-doc — related: doc:b3356305:readme-flag-ghost, ux:073d24ed:concept-named-twice
**Evidence:** `internal/cli/root.go:33-38`
**Problem:** The cfgFile doc comment says it is "read by subcommand RunE handlers (deploy, destroy, update-ingress)". deploy never reads cfgFile (it uses deployOutputFile), while six undocumented commands do read it (addon, cleanup, config, debug-bundle, doctor, status). The stale reader list misleads at exactly the spot where the deploy/--config drift (ux:073d24ed) lives.
**Fix:** Drop the parenthetical command list (it rots on every new subcommand) or replace with "read by every config-consuming subcommand except deploy, which manages its own file via --output-file".
**Effort:** hours

##### `doc:26a430ee:doc-comment-stale` — doc comment stale

**Status:** not started
**Severity:** suggestion
**Cluster:** exported-doc — related: doc:1013f4e8:docs-behavior-drift
**Evidence:** `internal/cli/elevation.go:78-84`
**Problem:** ensureRoot's policy doc illustrates the reject branch with "(e.g. `sudo okdctl wizard`)" — there is no `wizard` subcommand in the cobra tree (the wizard runs inside `deploy`). The example names a command that cannot be invoked.
**Fix:** Use a real non-root command in the example: `sudo okdctl status` or `sudo okdctl config show`.
**Effort:** hours

##### `doc:70b3bae2:docs-struct-snippet-drift` — docs struct snippet drift

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/doc-70b3bae2-wizard
**Severity:** suggestion
**Cluster:** readme-drift — related: doc:8f46b665:docs-struct-snippet-drift
**Evidence:** `docs/architecture/wizard.md:43-57`
**Problem:** wizard.md's FieldDefinition snippet presents itself as the full struct but omits the KVAsDelimitedString field (FieldTypeKeyValue serialization toggle) present in datadriven.go, so the doc's "smallest unit" contract is incomplete for key-value fields.
**Fix:** Add KVAsDelimitedString to the snippet or annotate the snippet as abridged ("core fields shown; see datadriven.go for the full set").
**Effort:** hours

##### `doc:73ad30ef:doc-comment-stale` — doc comment stale

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/doc-73ad30ef-warnonerror
**Severity:** suggestion
**Cluster:** exported-doc
**Evidence:** `internal/distribution/okd/phase/helpers.go:11-13`
**Problem:** WarnOnError's doc says "Use with StepBuilder.OnError()", pointing at the fluent-builder API that CLAUDE.md demotes to dynamic-source-only; all 8 call sites use the canonical StepDef.OnError field. The doc steers new code toward the non-canonical API.
**Fix:** Change the second sentence to "Use as a StepDef.OnError callback."
**Effort:** hours

#### audit-tests

##### `tst:aa0f50f5:trust-boundary-untested` — trust boundary untested

**Status:** in review — PR #813
**Severity:** major
**Cluster:** trust-boundary-untested — related: tst:c5e5c304:trust-boundary-untested
**Evidence:** `internal/config/validators.go:27-374`
**Problem:** Only 4 of ~25 validators have tests (isValidNetmask, ValidateProxmoxHost, validateHAMasters, ValidateTerraformEnv). The Validate() pipeline — validateRequired, validateEnums, validateNetworking, checkCIDROverlap, IsValidDNSLabel — is untested, yet it is the only gate between hand-edited YAML and cluster names interpolated into HCL tfvars, root-privileged file paths, and DNS records. Package coverage floor is 12%.
**Fix:** Extend validators_test.go: IsValidDNSLabel/ValidateClusterName with `../etc`, `a"b`, `A-UPPER`, 63/64-char boundary, leading digit/hyphen; ValidateCIDR with `10.0.0.0/40`, `::/129`; ValidateGatewayInCIDR out-of-range; ValidateSSHFingerprint missing-prefix/empty-b64; ValidateBinDir relative path; one end-to-end Validate() table asserting field-keyed errors. Raise the internal/config floor from 12 once landed.
**Effort:** days

##### `tst:48688e63:destructive-happy-untested` — destructive happy untested

**Status:** in review — PR #818
**Severity:** major
**Cluster:** destructive-untested — related: tst:48688e63:cred-zeroize-untested
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:304-366` + 3 more
**Problem:** The entire proxmox package (deploy path: Connect/Provision/retrieveProvisionResult/initIsRetryable/checkTerraformOutputs) has zero tests. retrieveProvisionResult's bootstrap/master/worker IP arithmetic decides which hosts later receive SSH and install operations; setup/nodes.go BuildNodeList duplicates the same offset logic, so divergence between the two is exactly the bug a table test would catch.
**Fix:** Table-test retrieveProvisionResult and BuildNodeList with identical configs and assert identical IP sequences (1+N masters, offset workers, CIDR-fit rejection). Add initIsRetryable cases (ConfigError/AuthError/ctx.Canceled permanent, exit errors retryable) and checkTerraformOutputs count-mismatch parse. Then add an internal/infrastructure/proxmox floor to .github/coverage-floors.conf.
**Effort:** days

##### `tst:632c9087:destructive-partial-untested` — destructive partial untested

**Status:** not started
**Severity:** major
**Cluster:** destructive-untested — related: sub:1e8ffb91:ring-truncated-stdout-parse
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:461-487` + 1 more
**Problem:** convertToLoadBalancer deletes the cluster's IngressController, waits for the router to terminate, then recreates it — with attemptRollback as the only recovery if the create fails. Tests cover only the pure JSON builders; the delete→wait→create→rollback orchestration and the rollback itself have no coverage. Same package where an untested parse path shipped broken (sub:1e8ffb91).
**Fix:** Drive convertToLoadBalancer through a fake oc binary (executor harness as in kubectl_test.go): assert delete argv targets only ic.Name in openshift-ingress-operator; on create failure assert attemptRollback issues a create with buildRollbackJSON output; on rollback failure assert the operator-facing error names both failures.
**Effort:** days

##### `tst:b38ec9cc:destructive-happy-untested` — destructive happy untested

**Status:** not started
**Severity:** major
**Cluster:** destructive-untested — seam→audit-state-and-recovery — related: state:0f076161:destroy-scoped-cleanup-unscoped
**Evidence:** `internal/distribution/okd/install/workers.go:22-76`
**Problem:** StartWorkerVMs runs a live terraform apply and nothing locks its two safety properties: the -target scoping to the worker VM resource (the in-code comment admits an unscoped apply would reconcile the full state) and the snapshot-before-apply ordering. workersAlreadyRunning's node-count parse is also untested.
**Fix:** Use the fake-terraform-binary harness from destroy/helpers_test.go: capture argv, assert -target=module.okd_cluster.proxmox_virtual_environment_vm.worker and -var start_workers_immediately=true are present, and that a state snapshot exists before apply runs. Table-test workersAlreadyRunning line counting (0 workers, exact count, cluster-unreachable→false,nil).
**Effort:** hours

##### `tst:98723e5d:cred-install-path-untested` — cred install path untested

**Status:** in review — PR #815
**Severity:** major
**Cluster:** cred-path-untested — related: sec:e3782ee7:symlink-escape, tst:e3782ee7:canonical-helper-untested
**Evidence:** `internal/distribution/okd/install/flux.go:49-105`
**Problem:** SetupClusterAccess installs the cluster-admin kubeconfig into the invoking user's ~/.kube/config under the sudo re-exec (CopyFileMode 0o600, timestamped backup, ChownToInvokingUser). Only the .bashrc helper is tested; the credential copy, backup-on-overwrite, and ownership restoration have no coverage.
**Fix:** Test SetupClusterAccess against a temp HOME (t.Setenv SUDO_USER to the current user as in fs_test.go TestExpandPath): fresh install creates 0o600 config; pre-existing config produces a .backup.* sibling at 0o600 with original bytes; ctx-cancelled early return leaves dest untouched. Mirror the perms-assertion shape of cli/kubeconfig_test.go TestMergeKubeconfig_Perms.
**Effort:** hours

##### `tst:2f70d7df:trust-boundary-untested` — trust boundary untested

**Status:** in review — PR #816
**Severity:** major
**Cluster:** trust-boundary-untested
**Evidence:** `internal/distribution/okd/setup/kargs.go:74-98`
**Problem:** BuildIgnitionURLForNode enforces that the ignition server IP is RFC1918/loopback/link-local — documented in-code as the invariant preventing pull-secret exposure on public interfaces — and has zero tests. kargs.go has no test file at all, so the karg string formats feeding coreos-installer are also unlocked.
**Fix:** Add kargs_test.go: table for BuildIgnitionURLForNode — accepts 10.x/192.168.x/127.0.0.1/169.254.x/fd00:: plus custom-port suffix behavior; rejects 8.8.8.8, 2001:db8::1, empty, hostname. Golden-string BuildLiveKargs/BuildDestKargs so the ip=...:none syntax can't silently regress.
**Effort:** hours

##### `tst:06f00bcb:cred-perms-policy-untested` — cred perms policy untested

**Status:** in review — PR #802
**Severity:** major
**Cluster:** cred-path-untested — related: tst:e3782ee7:canonical-helper-untested
**Evidence:** `internal/distribution/okd/setup/apache.go:158-183` + 1 more
**Problem:** DeployToWebServer copies pull-secret-bearing ignition files into the apache DocumentRoot at 0o640, and ensureIgnitionDir sets 0o750+chown so non-apache local users cannot read them. These perm/ownership policies are the credential-protection invariant and have no test; the doc-comment promise that kubeconfig/kubeadmin-password are never copied to the web root is also unlocked.
**Fix:** Test DeployToWebServer with a temp webRoot: ignition fixtures land at 0o640 inside ignition/ at 0o750; files absent from clusterDir are skipped without error; assert auth/kubeconfig and kubeadmin-password fixtures present in clusterDir are NOT copied (locks the doc-comment contract). ChownByName failure is warn-only — assert non-fatal.
**Effort:** hours

##### `tst:0934cf1b:destructive-happy-untested` — destructive happy untested

**Status:** not started
**Severity:** minor
**Cluster:** destructive-untested
**Evidence:** `internal/platform/packages.go:71-141`
**Problem:** Manager.Remove (root `dnf/apt-get remove -y` during destroy --remove-packages), IsInstalled (SubprocessError→ExitError unwrap chain plus dpkg 'ii ' postCheck), and AddRepo (writes /etc/apt/sources.list.d) have no tests. IsInstalled's error discrimination is the load-bearing filter deciding what gets removed.
**Fix:** Fake-binary harness (PATH shim as in cleanup/packages_test.go): IsInstalled maps exit-1 to (false,nil) and LookPath failure to error; dpkg postCheck distinguishes 'ii  pkg' from 'rc  pkg'; Remove issues remove -y only for installed packages and is a no-op when none are. AddRepo Debian branch: golden-test the list-file content via a temp listPath seam.
**Effort:** hours

##### `tst:262af6e4:destructive-partial-untested` — destructive partial untested

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-262af6e4-cleanup
**Severity:** major
**Cluster:** destructive-untested — seam→audit-state-and-recovery — related: state:0f076161:destroy-scoped-cleanup-unscoped, state:c287d5c0:prepare-wipes-live-cluster-artifacts
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:246-259`
**Problem:** The PostDestroy branch of the cleanup-terraform step is the only code path in the repo that deletes terraform.tfstate, gated by !tf.HasState(). Sibling cleanup paths carry explicit 'DATA LOSS' regression assertions, but no test covers either PostDestroy outcome: empty state removed, non-empty state (e.g. after a targeted destroy) preserved.
**Fix:** Extend cleanup_test.go with two PostDestroy=true cases reusing the existing envDir fixtures: (a) terraform.tfstate with resources → file survives (the 'DATA LOSS' assertion shape from TestExecute_FullKind_AllStepsRun:L255-L260); (b) empty-resources state → file removed. HasState is already unit-tested in terraform_test.go, so only the gate wiring needs locking.
**Effort:** hours

##### `tst:0139cb3f:trust-boundary-untested` — trust boundary untested

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-0139cb3f-paths
**Severity:** major
**Cluster:** trust-boundary-untested
**Evidence:** `internal/distribution/okd/phase/paths.go:57-111`
**Problem:** ResolveBinDir / PreflightBinDir / envBinDir / validateAndClean resolve the directory where setup installs root-owned binaries and cleanup later removes them, from OKDCTL_BIN_DIR (env, attacker-influenceable pre-sudo) and config. The doc admits '`..` traversal is not rejected'. None of the resolution chain is tested; env>config>default precedence and fall-through-on-invalid behavior are unlocked.
**Fix:** Add paths_test.go with t.Setenv: env wins over config wins over DefaultBinDir; relative/invalid env value falls through; `~/bin` expands via invoking user (SUDO_USER seam as in fs_test.go TestExpandPath); `/usr/local/bin/../../etc` locks the Clean result so a future traversal-rejection change is a visible diff. Cross-assert with cleanup's RefuseCriticalPath expectations.
**Effort:** hours

##### `tst:c5e5c304:trust-boundary-untested` — trust boundary untested

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-c5e5c304-tf-test
**Severity:** minor
**Cluster:** trust-boundary-untested — related: tst:aa0f50f5:trust-boundary-untested
**Evidence:** `internal/distribution/okd/setup/terraform.go:20-145`
**Problem:** buildTerraformVarsData and the format helpers (buildISOStrings/buildNodeNames/formatStringList/formatAdditionalNetworks, getDiskSizes/getBootstrapResources defaulting) render config into the terraform.tfvars that drives VM create/destroy. All pure, all untested — silent defaulting bugs (worker disk inheriting cp disk, bootstrap falling back to control-plane resources) would provision wrong-sized infrastructure.
**Fix:** Golden-test buildTerraformVarsData for a 3-master/2-worker config (names, ISO strings, counts, disk/bootstrap defaulting); formatAdditionalNetworks with/without VLAN tag; formatStringList empty→[]. Validator coverage for cluster name (tst:aa0f50f5) is the quote-injection gate; cross-link rather than duplicate.
**Effort:** hours

##### `tst:b5a79fda:trust-boundary-untested` — trust boundary untested

**Status:** in review — PR #817
**Severity:** minor
**Cluster:** trust-boundary-untested — seam→audit-state-and-recovery — related: state:c287d5c0:prepare-wipes-live-cluster-artifacts
**Evidence:** `internal/cli/deploystate.go:79-135`
**Problem:** readDeployState / announceDeployState parse a JSON marker on destroy entry and steer the operator ('prefer okdctl cleanup over destroy' vs 'running destroy'). Schema-version gating, cluster-name mismatch rejection, corrupt-JSON handling, and the stale-marker age hint are all untested.
**Fix:** Table-test readDeployState (missing file→nil,nil; corrupt JSON→error; v1 round-trip via writeDeployState; unknown schema→nil,nil) and announceDeployState cluster-mismatch ignore. Pure file IO against t.TempDir, no mocks.
**Effort:** hours

##### `tst:e3782ee7:canonical-helper-untested` — canonical helper untested

**Status:** in review — PR #810
**Severity:** blocker
**Cluster:** canonical-helper-untested — seam→audit-security — related: sec:e3782ee7:symlink-escape
**Evidence:** `internal/system/fs.go:122-170` + 3 more
**Problem:** CopyFileMode has no test for a symlink at dst: it opens with O_WRONLY|O_CREATE|O_TRUNC and no Lstat/O_NOFOLLOW guard, unlike AtomicWrite which has both the guard and a regression test. The helper writes kubeconfig backups, install-config.yaml (pull-secret), and ignition payloads as root.
**Fix:** Add a TestCopyFileMode subtest mirroring fs_test.go:L147-L174: create target, symlink dst→target, assert CopyFileMode either refuses (preferred, matching AtomicWrite) or document-and-lock current follow behavior. If the guard is added (per sec:e3782ee7), this test is its regression lock.
**Effort:** days

##### `tst:40d315ad:destructive-happy-untested` — destructive happy untested

**Status:** not started
**Severity:** minor
**Cluster:** destructive-untested
**Evidence:** `internal/addon/catalog/flux/flux.go:255-270` + 1 more
**Problem:** The addon uninstall paths — flux's `oc delete ns flux-system` and secretstore's per-secret `oc delete secret` + `oc delete secretstore` loop — have no tests, while the install-side builders in the same packages are well covered. Partial-failure semantics (one secret fails to delete) are unlocked.
**Fix:** Fake-oc harness: assert delete argv targets exactly the addon-owned namespace/secret names (no wildcard), and that a single failed secret delete continues/aggregates per the intended semantics rather than aborting the loop silently.
**Effort:** hours

##### `tst:48688e63:cred-zeroize-untested` — cred zeroize untested

**Status:** in review — PR #818
**Severity:** major
**Cluster:** cred-path-untested
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:91-100`
**Problem:** proxmox.Provider.ZeroizeEnv re-implements the KeyIsSecret-blank-then-clear logic instead of delegating like terraform.Executor and okd.Provisioner do, and has no test. executor.Executor.ZeroizeEnv (the tested sibling) shows the exact invariants to lock.
**Fix:** Add a table test mirroring internal/executor/executor_test.go:L158-L193 (secret keys blanked, slice nil after call, non-secret entries also cleared, idempotent second call). Alternatively refactor Provider to delegate to an executor so the existing test covers it.
**Effort:** hours

### Tier A — holistic review 2026-06-10

Captured from `holistic-review` run on 2026-06-10 (HEAD `46d11fa`). Items are
judgment-shaped (not audit atoms); each has a 1-3 sentence rationale inline.
Run focus: stripping AI-generated code smells. Headline: comment slop is
already gone (zero hits for narration/dividers/peacock/bare-TODO patterns);
what remains is structural — dead knobs, fabricated data, unreachable states,
and ceremony layers.

#### A1 — Collapse the step framework's triple representation

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

- **Status:** not started
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

## Completed

Completed items live in [`docs/roadmap/completed-archive.md`](docs/roadmap/completed-archive.md). Grep there for the canonical "is dep X done?" lookup. The previous in-line pointer index (144 entries, mirroring archive contents) was removed on 2026-05-09 to keep `roadmap.md` focused on active work.

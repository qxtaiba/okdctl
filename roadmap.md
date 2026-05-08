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

##### `sec:06f00bcb:auth-tree-in-webroot` — auth tree in webroot

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-06f00bcb-auth-tree-webroot  
**Severity:** blocker  
**Cluster:** file-toctou  
**Evidence:** `internal/distribution/okd/setup/apache.go:174-234`  
**Problem:** DeployToWebServer copies the openshift-install auth/ tree (kubeconfig + kubeadmin-password) into the apache DocumentRoot at webRoot/auth/, alongside the ignition files. The deployment relies entirely on the source mode bits (0o600) to keep apache from serving them — there is no Apache <Location /auth> deny rule. A future schema bump in openshift-install that emits a more permissive mode, an operator who runs `chmod -R go+r` to debug, or any DocumentRoot override that re-renders perms turns the kubeadmin password into a bastion-network HTTP fetch.  
**Fix:** Stop copying the auth/ tree into webRoot. The kubeconfig + kubeadmin-password are consumed by install/SetupClusterAccess (~/.kube/config) and the postinstall verifyKubeVIPAPIHealthBootstrap (kubeconfigPath in clusterDir). No phase NEEDS them under apache. If a future caller does, render a one-line drop-in `<Location /auth>Require all denied</Location>` snippet alongside the configureApachePort write so the deny rule is colocated with the document root.  
**Effort:** days

##### `sec:eb479d86:ssh-pin-bypass` — ssh pin bypass

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-eb479d86-ssh-pin-bypass  
**Severity:** blocker  
**Cluster:** tls-network  
**Evidence:** `internal/distribution/okd/setup/upload.go:78-91`  
**Problem:** uploadISOsViaSCP hardcodes StrictHostKeyChecking=accept-new and ignores the knownHostsPath that the caller obtained from sshpin.Verify. When proxmox.ssh_host_fingerprint is set, the rest of the system enforces strict host-key checking, but this scp call still trusts whatever key the network presents — defeating the pin on the very phase that uploads custom ISOs containing per-node ignition kargs.  
**Fix:** Thread knownHostsPath into uploadISOsViaSCP. When non-empty: pass `-o UserKnownHostsFile=<path>` and `-o StrictHostKeyChecking=yes`; only fall back to accept-new when the path is empty. Mirror the sshBaseArgs() shape used in phase/ssh.go so the policy matches the SSH path that ALREADY honors the pin.  
**Effort:** days

##### `sec:2f70d7df:ignition-over-http` — ignition over http

**Status:** not started  
**Severity:** major  
**Cluster:** tls-network  
**Evidence:** `internal/distribution/okd/setup/kargs.go:68-78`  
**Problem:** BuildIgnitionURLForNode hardcodes http:// for the ignition fetch URL embedded in every node's coreos.inst.ignition_url karg. The fetched ignition payload contains the cluster pull-secret, SSH authorized keys, and machine-config tokens. Anyone on the bastion network segment can passively recover those credentials by sniffing the FCOS first-boot HTTP request, and an active attacker can substitute a tampered ignition that ships a backdoor.  
**Fix:** Either: (a) document the threat model explicitly in the apache.go and kargs.go files (current state, network-isolated bastion is sole defense), and add a runtime check refusing to proceed when ignitionIP is on a non-RFC1918 network, OR (b) wire FCOS-served ignition via HTTPS with an apache-self-signed cert and the bastion CA pinned via coreos.inst.ignition_url + coreos.inst.cert. Option (a) is correct for okdctl's homelab posture; option (b) raises operator burden.  
**Effort:** hours

##### `sec:35abd54e:env-string-residue` — env string residue

**Status:** not started  
**Severity:** major  
**Cluster:** credentials  
**Evidence:** `internal/credentials/proxmox.go:151-172`  
**Problem:** ProxmoxCredentials.Env() returns []string with credential bytes converted to immutable Go strings (envProxmoxPassword+"="+string(c.Password)). The doc-comment correctly notes this and tells callers to defer Zeroize(). However the resulting strings live as long as any struct that stores cmd.Env (Executor.Env, Provider.env) and become heap garbage after the env slice is replaced. Zeroize() cleans the []byte source but cannot reach the strings. terraform.Executor.ZeroizeEnv() partially mitigates by overwriting the entry with empty before clear()ing — but executor.Executor has no equivalent.  
**Fix:** Add executor.Executor.ZeroizeEnv() mirroring terraform.Executor.ZeroizeEnv (terraform.go:L347-L364). Update internal/distribution/okd/install/* and internal/infrastructure/proxmox/proxmox.go to defer e.Exec.ZeroizeEnv() at end of phases that pass creds.Env(). Document the residual-string boundary in CLAUDE.md §credentials-and-secrets so future env-passing sites follow the same defer pattern.  
**Effort:** hours

##### `sec:40d315ad:flux-keyscan-tofu` — flux keyscan tofu

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-40d315ad-flux-keyscan-pin  
**Severity:** major  
**Cluster:** tls-network  
**Evidence:** `internal/addon/catalog/flux/flux.go:407-421`  
**Problem:** createDeployKeySecret runs `ssh-keyscan host` and bakes the result directly into the flux-system Secret as known_hosts. There is no fingerprint pin, no DNSSEC check, no fallback — whatever key the network returns at deploy time becomes the perpetual trust anchor for every flux git pull. An on-path attacker during deploy can substitute their host key and intercept every subsequent git operation.  
**Fix:** Add an `addons.flux.settings.git_host_fingerprint` setting (SHA256:...). When set, validate the keyscan output against it before writing the Secret (mirror sshpin.Verify's pattern). When unset, log a Warn naming the observed fingerprint so operators can pin it on the next deploy. The okdctl Proxmox SSH path already follows this exact pattern (sshpin.Verify) — use it as the counter-example.  
**Effort:** hours

##### `sec:5e892064:checksum-no-signature` — checksum no signature

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-5e892064-checksum-pin  
**Severity:** major  
**Cluster:** tls-network  
**Evidence:** `internal/download/checksum.go:59-111`  
**Problem:** FetchChecksum downloads a sha256sum.txt-style file over HTTPS and trusts whatever the server returns. There is no signature verification on the checksum file itself — an attacker who poisons the CDN serving the checksum AND the artifact controls both. okdctl install.sh does cosign-verify SHA256SUMS for okdctl's own artifacts, but the in-tree download path used by setup.installBinary (helm, sops checksums from get.helm.sh and getsops/sops releases) does not.  
**Fix:** Either: (a) compile-time-pin embedded SHA-256s for helm/sops at known versions (the yqChecksumsByArch pattern in tools.go:L40-43 already exists), removing the runtime FetchChecksum dependency entirely; (b) add a cosign verify-blob step against the checksums file's signature for vendors that publish one. Option (a) is the lower-risk fix and matches the existing yq pattern.  
**Effort:** hours

##### `sec:7b2829bb:executor-no-zeroize` — executor no zeroize

**Status:** not started  
**Severity:** major  
**Cluster:** credentials  
**Evidence:** `internal/executor/executor.go:38-92`  
**Problem:** Executor.Env is []string and there is no Zeroize/ZeroizeEnv method. Cred-bearing env entries (PROXMOX_VE_PASSWORD=..., PROXMOX_VE_API_TOKEN=...) are appended via WithEnv(creds.Env()) and live as immutable strings until the Executor is GC'd. terraform.Executor has the parallel ZeroizeEnv (terraform.go:L347-364) and proxmox.Provider.env is also never zeroized. Cross-references with sec:35abd54e (env-string-residue policy).  
**Fix:** Add executor.Executor.ZeroizeEnv() that walks e.Env, blanks any entry whose key matches secretKeyFragments (re-use logutil.KeyIsSecret), then clear()s and nils the slice. Wire it into the defer chain in createOKDProvisionerWithOpts (cli/helpers.go) alongside p.ZeroizeEnv(). Update CLAUDE.md to document the defer pattern.  
**Effort:** hours

##### `sec:9d79b841:coreos-stream-no-pin` — coreos stream no pin

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-9d79b841-coreos-pin  
**Severity:** major  
**Cluster:** tls-network  
**Evidence:** `internal/distribution/okd/setup/coreos.go:182-216`  
**Problem:** fetchCoreOSStream pulls fcos.json/scos.json from raw.githubusercontent.com and trusts the ISO URL + SHA256 it returns wholesale — only TLS to GitHub anchors authenticity. The ISO checksum verification (DownloadCoreOSISO) is then meaningless against an upstream poisoning of openshift/installer's release-4.X branch: the attacker rewrites both the location and the sha256 in the same JSON, and our fetch passes both checks.  
**Fix:** Pin the openshift/installer reference to a known-good commit SHA (not a branch) and embed the expected JSON sha256 at compile time, similar to bootstrapOCChecksum/bootstrapOCVersion in release_extract.go. Bumping the OKD minor in CI requires explicit re-pin. Alternatively, fetch from a release tag that is itself signed via cosign and verify the tag's sigstore bundle before parsing.  
**Effort:** hours

##### `sec:e3782ee7:atomicwrite-symlink-race` — atomicwrite symlink race

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-e3782ee7-atomicwrite-symlink  
**Severity:** major  
**Cluster:** file-toctou  
**Evidence:** `internal/system/fs.go:196-243`  
**Problem:** AtomicWrite creates the destination via openTempFile (O_CREATE|O_EXCL) then os.Rename. There is no os.Lstat/O_NOFOLLOW pre-check on path itself; if path pre-exists as a symlink, os.Rename atomically replaces the symlink (good) but the directory containing path can still be symlink-redirected by an attacker who controls a parent component. Critical writes (kubeconfig, .env, install-config.yaml) under the sudo re-exec model use AtomicWrite. Compare to envfile.go:L52-58 and runlock.go:L40-50 which both Lstat + O_NOFOLLOW on the path itself.  
**Fix:** In AtomicWrite, before the openTempFile call, run os.Lstat(path) and refuse if the result reports a symlink (mirroring credentials/envfile.go:WriteEnvFile L52-58). os.Rename's replace-target semantics already overwrite the link atomically once the temp file exists, but the explicit refusal makes the policy auditable and removes the TOCTOU window between EnsureDirForFile and Rename.  
**Effort:** hours

##### `sec:48688e63:proxmox-host-no-revalidate` — proxmox host no revalidate

**Status:** not started  
**Severity:** minor  
**Cluster:** input-validation  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:103-121`  
**Problem:** Provider.Connect captures cfg.Provider.Proxmox.Host into p.host without re-validating, then passes it to phase.ProxmoxBareHost which strips scheme/port. config.validateProxmoxConfig (validators.go:L228-277) does validate on load, but a config struct constructed by other paths (wizard, programmatic test) can bypass that validation. The result threads into pveshRun's argv and into SSHRunArgv at the executor seam — the per-call validateProxmoxName guard catches Node, but Host has no equivalent.  
**Fix:** In Provider.Connect, after p.host is captured, run config.ValidateProxmoxHost (already exported in validators.go) and return *errtypes.ConfigError on failure. Mirrors phase.validateProxmoxName at the pveshRun boundary.  
**Effort:** hours

##### `sec:a6e38cc7:keyscan-no-strict-baseline` — keyscan no strict baseline

**Status:** not started  
**Severity:** minor  
**Cluster:** input-validation  
**Evidence:** `internal/sshpin/sshpin.go:31-80`  
**Problem:** sshpin.Verify's empty-fingerprint branch is the documented fallback when proxmox.ssh_host_fingerprint is unset — but it logs a WARN and returns ('', nil), letting accept-new TOFU proceed. There is no operator-facing config gate (e.g. proxmox.require_pinned_fingerprint: true) that fails closed for security-sensitive deploys. A homelab without operator discipline silently runs every deploy on first-seen-key trust.  
**Fix:** Add provider.proxmox.require_pinned_fingerprint bool to ProxmoxConfig. When true and SSHHostFingerprint is empty, sshpin.Verify returns *errtypes.AuthError instead of the WARN+continue path. Default false to preserve current behaviour.  
**Effort:** hours

##### `sec:bdf5a873:cleanup-symlink-traversal` — cleanup symlink traversal

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-bdf5a873-cleanup-symlink  
**Severity:** minor  
**Cluster:** file-toctou  
**Evidence:** `internal/distribution/okd/cleanup/artifacts.go:33-60`  
**Problem:** SafeRemoveWithLogger calls os.RemoveAll(path) under sudo re-exec without lstat-checking if path is itself a symlink. The refuseCriticalPath allowlist guards /, /etc, etc., but a symlink at <workDir>/okd-install→/etc passes the check (Base is 'okd-install') and would direct the unlink at /etc — bounded because Go's os.RemoveAll does not follow symlinks during recursion, but the symlink itself is unlinked rather than what the operator intended.  
**Fix:** Add an Lstat check at the head of SafeRemoveWithLogger: if the entry is a symlink, log Warn and Refuse to RemoveAll, requiring the operator to remove the link manually. Mirrors the symlink refusal pattern in envfile.go:WriteEnvFile and runlock.go:Acquire.  
**Effort:** hours

##### `sec:c8b28673:tar-mode-trust` — tar mode trust

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-c8b28673-tar-mode-mask  
**Severity:** minor  
**Cluster:** file-toctou  
**Evidence:** `internal/download/extract.go:89-110`  
**Problem:** processTarEntry uses os.MkdirAll(targetPath, os.FileMode(header.Mode&0o755)) and OpenFile(...os.FileMode(header.Mode&0o777)) — the file mode is read from the tar header without a floor. A malicious tarball containing a 0o000 directory entry can prevent extraction of subsequent files inside it; a 0o4755 setuid file is masked off by &0o777, but 0o2xxx setgid bits ARE preserved. okdctl extracts release tarballs from quay.io/okd/scos-release (already-fetched, cosign-anchored), so reachability is bounded by upstream poisoning, but the helper is generic.  
**Fix:** Tighten the file mask from &0o777 to &0o755 for regular files and &0o755 for dirs. Setgid/sticky bits in archive entries have no legitimate use in the binary tarballs okdctl consumes; mask them off explicitly.  
**Effort:** hours

##### `sec:451be4fa:invokinguser-fallback-doc` — invokinguser fallback doc

**Status:** not started  
**Severity:** suggestion  
**Cluster:** privilege-escalation  
**Evidence:** `internal/system/elevation.go:24-31`  
**Problem:** InvokingUser falls back to user.Current() when SUDO_USER is unset OR names a user that no longer exists. Under the sudo re-exec model SUDO_USER should always be set; the fallback exists for non-sudo callers (wizard, dry-run). The risk: a privileged caller (root) of InvokingUserHomeDir gets /root, and if the result is then used as a chown target via ChownToInvokingUser the file is left root-owned. Today's call sites all gate on 'are we under sudo' implicitly — but the helper itself does not warn the caller.  
**Fix:** Document the fallback semantics in the godoc more explicitly: 'Returns the original sudo user; under direct root invocation returns the root user. Callers that need the original-not-root semantics MUST check os.Geteuid()==0 before calling.'  
**Effort:** hours

##### `sec:88fd3050:proxmox-username-no-redact` — proxmox username no redact

**Status:** not started  
**Severity:** suggestion  
**Cluster:** redaction  
**Evidence:** `internal/config/cluster.go:119-124`  
**Problem:** ProxmoxConfig.Username is json:- (correct — never serialised) and lives alongside Password (SecretBytes, redacted). But Username is plain string and the redactedCredentials view in credentials/proxmox.go DOES include Username — a slog %v on ProxmoxConfig that reaches a non-RedactHandler logger emits the username verbatim. Username is not a high-value credential on its own, but combined with a leaked password elsewhere it is a complete account.  
**Fix:** Either add 'username' to logutil.secretKeyFragments (changes redaction policy across the codebase — measure first), or add a Redacted() any method to ProxmoxConfig that omits Username from the safe view, mirroring credentials/proxmox.go:redactedCredentials.  
**Effort:** hours

##### `sec:8e65d574:updatecheck-no-sig` — updatecheck no sig

**Status:** not started  
**Severity:** suggestion  
**Cluster:** tls-network  
**Evidence:** `internal/version/updatecheck.go:89-116`  
**Problem:** BackgroundCheck fetches https://api.github.com/repos/qxtaiba/okdctl/releases/latest over HTTPS and parses TagName from the JSON. There is no signature verification on the response, only TLS to api.github.com. The returned tag is rendered to the user via printUpdateNotice — a poisoned response could trick users into an upgrade flow. Reachability requires GitHub MITM, which is unrealistic; the user-facing risk is bounded.  
**Fix:** Document in the function's godoc that the update notice is advisory; the user-facing copy already includes the version range. No code change needed.  
**Effort:** hours

##### `sec:8ea706f6:hashicorp-gpg-trust-doc` — hashicorp gpg trust doc

**Status:** not started  
**Severity:** suggestion  
**Cluster:** tls-network  
**Evidence:** `internal/distribution/okd/setup/tools.go:273-336`  
**Problem:** installHashiCorpDebianRepo fetches the GPG key from https://apt.releases.hashicorp.com/gpg via TLS and verifies the fingerprint against the embedded constant. The constant fingerprint pinning IS the trust anchor; flagging the absence of a unit test asserting expectedHashiCorpGPGFingerprint matches HashiCorp's published value (a typo in the constant would silently let any key through gpg's --import-options show-only output).  
**Fix:** Add a unit test that asserts expectedHashiCorpGPGFingerprint is the canonical HashiCorp release-signing-key fingerprint at the time of the test. Could be a string-equality check against a comment-in-code citation of the canonical source, or a network-gated test that fetches from apt.releases.hashicorp.com/gpg and re-derives the fingerprint via gpg.  
**Effort:** hours

##### `sec:bbc23e42:noplogger-no-redact` — noplogger no redact

**Status:** not started  
**Severity:** suggestion  
**Cluster:** redaction  
**Evidence:** `internal/logutil/logutil.go:1-28`  
**Problem:** NopLogger is the canonical no-op logger but is slog.New(slog.DiscardHandler) — it does NOT have RedactHandler installed. Production code paths that fall back to NopLogger (constructor's nil-logger argument) silently lose redaction even if the surrounding code thought it was protected. Today's call sites are bounded (test helpers, unconfigured-constructor fallback), but the pattern is brittle: a future logger.Info('...', 'password', secret) reaching NopLogger would silently bypass RedactHandler.  
**Fix:** Change NopLogger to slog.New(NewRedactHandler(slog.DiscardHandler)) so any future redirection still applies redaction. Effectively a no-op change today; ensures the NopLogger contract stays consistent with SimpleLogger's policy.  
**Effort:** hours

##### `sec:cfcdee2d:newinsecure-blast-radius` — newinsecure blast radius

**Status:** not started  
**Severity:** suggestion  
**Cluster:** tls-network  
**Evidence:** `internal/httputil/httputil.go:29-41`  
**Problem:** NewInsecure exists for the bootstrap-window VIP healthcheck (kube-vip cert re-issue) and is reachable only from postinstall.verifyKubeVIPAPIHealthBootstrap. The function is exported, so any future caller can opt into InsecureSkipVerify. Today's only caller is bounded by a fallback-after-x509.HostnameError pattern that is itself defended (haproxy_test.go:L97-158). The blast radius today is acceptable; flagging the export surface for future audit drift.  
**Fix:** Document NewInsecure's contract in the existing godoc more strongly: 'Adding a new caller MUST add a parallel test that asserts the secure path is preferred and the insecure fallback is reached only on a specific error class.' The haproxy_test pattern is the template.  
**Effort:** hours

##### `sec:e076e43c:install-sh-trust-doc` — install sh trust doc

**Status:** not started  
**Severity:** suggestion  
**Cluster:** file-toctou  
**Evidence:** `scripts/install.sh:162-170`  
**Problem:** install.sh extracts the goreleaser tarball with --no-same-owner --no-same-permissions --no-overwrite-dir. The --no-same-permissions flag drops file modes to umask, which on a typical user umask=022 yields 0o755 for the binary. The downstream `install -m 0755` step does set the final binary mode correctly, so the active risk is bounded — flagging for documentation rather than exploit.  
**Fix:** Extend the existing header comment block in install.sh to enumerate each defense layer (TLS to GitHub, cosign on SHA256SUMS, sha256 on archive, --no-same-permissions tar, install -m 0755 final write). No code change.  
**Effort:** hours

##### `sec:fde34e0c:k8s-kubeconfig-env-no-validate` — k8s kubeconfig env no validate

**Status:** not started  
**Severity:** suggestion  
**Cluster:** input-validation  
**Evidence:** `internal/cluster/k8s.go:55-64`  
**Problem:** WithEnvFallback() reads $KUBECONFIG from the process environment without validation. A user-controlled KUBECONFIG pointing at /dev/zero, /proc/self/environ, or a 100MB malicious yaml is consumed by every K8sClient constructed with this option. Production callers (install/postinstall) explicitly pass WithKubeconfig with a project-rooted path, so reachability is bounded — flagging as a hardening suggestion for tools or future callers.  
**Fix:** Add a simple sanity check inside WithEnvFallback: filepath.Clean the env value, refuse if it points outside $HOME or /etc, and Lstat-refuse symlinks. Match the resolveProjectRootOrDie pattern in cli/helpers.go.  
**Effort:** hours

#### audit-subprocess

##### `sub:48688e63:argv-concat` — argv concat

**Status:** not started  
**Severity:** major  
**Cluster:** argv-construction — seam→audit-security  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:378-380`  
**Problem:** probeVMEnumeration calls SSHRunArgv with '/nodes/'+p.node+'/qemu' built by string concat, bypassing pveshRun (the canonical helper that re-runs validateProxmoxName on the node atom — see pvesh.go:13). pvesh.go documents 'New pvesh callers go through pveshRun and inherit this guard automatically — do not interpolate names into ssh command strings without it' — this site is the policy violation.  
**Fix:** Replace the inline SSHRunArgv with a call to phase.pveshRun (or expose a public PveshRun helper if the proxmox package can't reach the unexported one). Pass through ctx + RemoteISOParams or equivalent. Net: route this probe through the same validation layer iso_cleanup uses.  
**Effort:** hours

##### `sub:7b2829bb:no-cancel-func` — no cancel func

**Status:** not started  
**Severity:** major  
**Cluster:** timeout-cancel — seam→audit-concurrency  
**Evidence:** `internal/executor/executor.go:310-329`  
**Problem:** RunInteractive calls exec.CommandContext but does not set cmd.Cancel + cmd.WaitDelay, so on ctx cancellation Go's default behaviour is SIGKILL the process. terraform.PlanStreamed flows through this path — a kill-9 mid-plan/apply leaves .terraform.tfstate.lock.info orphaned because terraform never receives SIGINT to release it gracefully. install/monitor.go::defaultStartMonitorCmd at L25-L33 is the canonical pattern this site should mirror.  
**Fix:** Mirror monitor.go: set cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGINT) } (terraform's documented soft-cancel signal) and cmd.WaitDelay = 30 * time.Second. Apply the same pair to Run/RunStreamed if they ever spawn a child that holds external locks.  
**Effort:** hours

##### `sub:0934cf1b:no-stderr-capture` — no stderr capture

**Status:** not started  
**Severity:** minor  
**Cluster:** io-handling  
**Evidence:** `internal/platform/packages.go:97-112`  
**Problem:** IsInstalled calls cmd.Output() which captures stdout but discards stderr; a non-ExitError failure (queryCmd missing from PATH, ctx cancellation, I/O fault) yields fmt.Errorf('%s query: %w', m.queryCmd, err) with no diagnostic context. Sibling Install/Remove use system.RunCaptured which preserves stderr inside *SubprocessError.  
**Fix:** Replace cmd.Output() with system.OutputCaptured(ctx, m.queryCmd, args...) and unwrap *SubprocessError when distinguishing ExitError vs not-found. Same gosec exemption (queryCmd is constructor-set) but stderr now flows.  
**Effort:** hours

##### `sub:a6e38cc7:no-stderr-capture` — no stderr capture

**Status:** not started  
**Severity:** minor  
**Cluster:** io-handling  
**Evidence:** `internal/sshpin/sshpin.go:41-47`  
**Problem:** runKeyscan calls cmd.Output() which returns stdout but lets stderr inherit the parent fd (TTY in interactive runs, /dev/null in piped runs). When ssh-keyscan -T 5 fails because the remote port 22 is closed, the diagnostic line lands on the operator's terminal but never reaches the returned err — the wrapper just becomes 'ssh-keyscan host: exit status 1'. Sibling system.OutputCaptured at exec.go:73 captures stderr into the typed error.  
**Fix:** Swap to system.OutputCaptured(ctx, "ssh-keyscan", "-T", "5", host) so stderr flows into a *SubprocessError and the structured log handler can redact + render it. Net: same return shape, better error.  
**Effort:** hours

##### `sub:e2343d2c:no-cmd-env` — no cmd env

**Status:** not started  
**Severity:** minor  
**Cluster:** io-handling  
**Evidence:** `internal/system/systemd.go:40-68` + 2 more  
**Problem:** systemd.go has three exec.CommandContext sites (ManageService ServiceStatus, IsServiceActive, IsServiceEnabled) that bypass the executor.FilterParentEnv allowlist used by sibling helper RunCaptured at exec.go:54. Under sudo re-exec these run as root, inheriting the unfiltered parent env (any GITHUB_TOKEN / GH_TOKEN / GIT_ASKPASS the operator's shell exported). Inconsistent with the env-isolation invariant the rest of the system package upholds.  
**Fix:** Either set cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist) at each site, or route through system.RunCaptured (drop the bare cmd.Run wrappers and let RunCaptured handle exit-code → bool conversion via errors.As against *SubprocessError).  
**Effort:** hours

##### `sub:eb479d86:argv-unclean-path` — argv unclean path

**Status:** not started  
**Severity:** minor  
**Cluster:** argv-construction — seam→audit-security  
**Evidence:** `internal/distribution/okd/setup/upload.go:21-35`  
**Problem:** remoteISO256 builds target = remotePath + '/' + filename and passes it to SSHRunArgv as a sha256sum positional arg. ssh.go:43-50 documents 'argv mode does NOT bypass the shell. Callers MUST validate every atom for shell metacharacters before calling.' filename is filepath.Base of an os.ReadDir entry under WorkDir/custom-isos so a hostile filesystem could plant 'foo;rm.iso' and the remote root shell would split it on ';'. The -- separator protects against flag injection, not shell metacharacters interpreted by the remote sshd login shell.  
**Fix:** Either (a) reject filenames containing shell metacharacters in collectISOFiles before adding them to isoFiles (cheapest), or (b) add a validateRemoteFilename helper similar to validateProxmoxName/validateISODir and call it before every SSHRunArgv interpolation. Counter-example: phase/iso_cleanup.go layers refuseUnsafeISOPath + shellSingleQuote.  
**Effort:** hours

#### audit-state-and-recovery

##### `state:15ba17da:destroy-iso-cleanup-before-tf` — destroy iso cleanup before tf

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/state-15ba17da-destroy-cleanup-order  
**Severity:** major  
**Cluster:** destroy-safety  
**Evidence:** `internal/distribution/okd/destroy/steps.go:74-167`  
**Problem:** Step ordering is StepDestroyInfra → StepRemoveRemoteISO → StepCleanupFiles → StepCleanupFirewall, but with NonFatal=true on every step the orchestrator continues even after `terraform destroy` fails. RemoveRemoteISO then runs against running VMs (because TF destroy didn't complete) — the safety check `anyVMReferencesISO` will skip ISO removal correctly, but file cleanup goes ahead and removes terraform.tfvars / cluster-config that an operator's retry would need. The terraformFailed() guard at L127 catches the cleanup-Kind downgrade but does NOT change the step ordering — irreversible host-file cleanup still runs after a TF failure that left VMs alive.  
**Fix:** When destroyTracker.terraformFailed() is true, also skip StepCleanupFirewall and the haproxy/dnsmasq subset of StepCleanupFiles (keep only the workdir cleanup). Today the WorkOnly downgrade is correct shape — extend it to skip the firewall step and the haproxy/dnsmasq cleanup helpers when VMs may still be alive. Surface a clearer summary message: 'destroy.terraform.failed = true; preserved haproxy/dnsmasq for retry'.  
**Effort:** hours

##### `state:48688e63:tf-state-no-backup` — tf state no backup

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/state-48688e63-tf-state-backup  
**Severity:** major  
**Cluster:** tf-state-atomicity  
**Evidence:** `internal/infrastructure/terraform/terraform.go:325-344`  
**Problem:** HasState reads terraform.tfstate but okdctl never snapshots it before invoking `terraform apply` or `terraform destroy`. terraform itself writes a `terraform.tfstate.backup` only across the apply boundary, leaving zero okdctl-managed history for the operator to roll back to after a corrupt state lands. The cleanup code's comment promises terraform.tfstate.backup as a rollback artefact, but on a fresh install there is none until the second apply.  
**Fix:** Add `Executor.SnapshotState(ctx)` that copies terraform.tfstate to terraform.tfstate.<UTC-timestamp>.bak via system.AtomicWrite immediately before every apply / destroy entrypoint (Provision, CleanupBootstrap, destroyInfrastructure, StartWorkerVMs). Retain N=5 most recent snapshots; surface the latest path in destroy / cleanup error messages so the operator can `cp` it back. Today HasState already exists at the right boundary — extend it.  
**Effort:** hours

##### `state:4c092fce:tf-state-no-lock` — tf state no lock

**Status:** not started  
**Severity:** major  
**Cluster:** tf-state-atomicity  
**Evidence:** `internal/infrastructure/terraform/terraform.go:118-174`  
**Problem:** runlock.Acquire serializes okdctl deploy/destroy/cleanup invocations on the same project root via flock, but Terraform itself enforces its own state lock via `.terraform.tfstate.lock.info` only inside terraform-the-binary. Two callers running on different hosts (NFS-mounted project root) bypass okdctl's flock (advisory on NFSv3) AND can race terraform if its lock-file write is non-atomic on that filesystem. The Executor never sets `-lock-timeout`, so a stale .terraform.tfstate.lock.info from a SIGKILL-ed prior run hangs the next apply for terraform's default 0s with an immediate fail rather than a wait+retry — operator hits the destroy.go:67 stateLockHint only on the destroy path, never on apply.  
**Fix:** Pass `-lock-timeout=120s` to every `terraform init/plan/apply/destroy` invocation in Executor.run / Init so a stale lock waits-then-fails with a clean diagnostic. Surface stateLockHint in Plan/Apply paths (today only destroyInfrastructure inspects it). Document the NFS bus-factor caveat in CLAUDE.md §architecture-notes alongside the existing flock advisory note in runlock.go.  
**Effort:** hours

##### `state:62cb8a95:state-version-warn-only` — state version warn only

**Status:** not started  
**Severity:** major  
**Cluster:** state-schema-evolution  
**Evidence:** `internal/distribution/okd/destroy/helpers.go:26-59`  
**Problem:** checkStateMajorVersion only runs on the destroy path. The deploy / install / postinstall paths invoke terraform.Init -> Apply against the same tfstate without ever validating its terraform_version. A user who upgraded terraform from 1.x to 2.x mid-deploy (or downgraded for hotfix) hits a silent state-format issue at apply time with terraform's own confusing error rather than okdctl's clear 'major mismatch' message. The deploy-time symmetry is missing.  
**Fix:** Move checkStateMajorVersion (and parseLockID/stateLockHint) into internal/infrastructure/terraform/state.go and call it once from Executor.Init when the state file already exists. Both deploy and destroy paths benefit; the destroy-only call site becomes a no-op symmetry trim. Keep the existing constants stateMajorMin/Max and the user-facing message verbatim — the test in helpers_test.go pins it.  
**Effort:** hours

##### `state:c19ee328:setup-installer-already-done-only-bootstrap` — setup installer already done only bootstrap

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/state-c19ee328-manifests-sentinel  
**Severity:** major  
**Cluster:** phase-idempotency  
**Evidence:** `internal/distribution/okd/setup/steps.go:109-201`  
**Problem:** StepGenerateConfig.AlreadyDone keys on `install-config.yaml.backup` — correct. StepGenerateManifests.AlreadyDone keys on `clusterDir/manifests` directory existing. StepGenerateIgnition keys on every .ign file. But manifests directory existing does NOT mean manifests are valid — `openshift-install create manifests` exits non-zero mid-write and leaves a partial manifests/ that the next run sees as 'already done'. Re-run skips manifest generation; subsequent ignition step then operates on partial manifests and produces malformed ignition. The AlreadyDone check is too coarse.  
**Fix:** Tighten the manifests-AlreadyDone to require BOTH the directory AND a sentinel file like `<clusterDir>/manifests/.complete` written via AtomicWrite by GenerateManifests on success. Pattern matches the install-config.yaml.backup sentinel already used by StepGenerateConfig. Same idea for StepGenerateIgnition: add a `.ignition.complete` sentinel after ValidateIgnitionFiles passes, instead of the implicit 'all .ign files >1024 bytes' check that runs every time.  
**Effort:** hours

##### `state:fb54208a:postinstall-bootstrap-cleanup-nonfatal` — postinstall bootstrap cleanup nonfatal

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/state-fb54208a-bootstrap-cleanup-fatal  
**Severity:** major  
**Cluster:** phase-idempotency  
**Evidence:** `internal/distribution/okd/postinstall/steps.go:43-58`  
**Problem:** StepCleanupBootstrap is NonFatal=true. If terraform apply -target=bootstrap fails (e.g. proxmox API auth glitch), postinstall continues and reports 'deployment complete' even though the bootstrap VM is still running, eating control-plane resources and risking etcd quorum confusion if it rebooted into the cluster. A subsequent `okdctl destroy` will catch it via terraform — but a user who 'just deploys' and doesn't destroy is left with a zombie bootstrap. Result.BootstrapCleaned tracks the success but isn't surfaced in the deploy-complete summary as a warning.  
**Fix:** Either elevate NonFatal=false (preferred — bootstrap cleanup failure is an operator-must-act event) OR keep NonFatal but surface result.BootstrapCleaned=false prominently in PostDeploySummary with a 'run okdctl destroy --only=bootstrap' hint. Today the post-deploy summary only shows the cleanup status if it succeeded.  
**Effort:** hours

##### `state:0f076161:destroy-target-no-precondition` — destroy target no precondition

**Status:** not started  
**Severity:** minor  
**Cluster:** destroy-safety  
**Evidence:** `internal/cli/destroy.go:80-162`  
**Problem:** --target / --only allowlist regex `module\.okd_cluster\.proxmox_virtual_environment_vm\.(bootstrap|master|worker)(\[\d+\])?` permits arbitrary index without verifying it's < cfg.Topology.{ControlPlane|Workers}.Count. An operator with a 3-master config who runs `okdctl destroy --target=module.okd_cluster.proxmox_virtual_environment_vm.master[7]` passes validation, hits terraform with a target that does not exist in state, and terraform happily destroys nothing — destroy reports success. Confusing for partial-failure recovery: operator believes index 7 was destroyed.  
**Fix:** After regex match, parse the index out of `[N]` and reject when N >= cfg.Topology.ControlPlane.Count for master, N >= cfg.Topology.Workers.Count for worker, or N != 0 for bootstrap. Test-coverage hook already exists in destroy_test.go. Catches typos like `master[10]` for a 3-master cluster.  
**Effort:** hours

##### `state:33579dd5:cleanup-haproxy-firewall-double` — cleanup haproxy firewall double

**Status:** not started  
**Severity:** minor  
**Cluster:** phase-idempotency  
**Evidence:** `internal/distribution/okd/cleanup/services.go:56-93`  
**Problem:** cleanup.HAProxy calls firewall.RemoveOKDRules at L71. The destroy phase ALSO has a separate StepCleanupFirewall that calls firewall.RemoveOKDRules. Both run in `okdctl destroy` (cleanup runs as a step inside destroy). RemoveOKDRules is allegedly idempotent, but on firewalld backends a second call against an already-removed rule logs a warning that propagates as a non-fatal failure in the destroy summary, falsely marking 'firewall' as a failed step. Two callers for the same operation across phases is a design smell.  
**Fix:** Remove the firewall.RemoveOKDRules call from cleanup.HAProxy; rely on StepCleanupFirewall to be the single removal site. Cleanup callers from contexts other than destroy (the cleanup CLI command) need StepCleanupFirewall too — already covered by cleanupSteps.Full path. No behavioral change for the cleanup-only command since it also runs StepCleanupFirewall via the cleanupSteps slice.  
**Effort:** hours

##### `state:368b892b:cleanup-tfstate-preserved-but-orphan` — cleanup tfstate preserved but orphan

**Status:** not started  
**Severity:** minor  
**Cluster:** state-schema-evolution  
**Evidence:** `internal/distribution/okd/cleanup/infra.go:47-72`  
**Problem:** terraformFilesToRemove deliberately excludes terraform.tfstate so destroy stays re-runnable. But after a successful `okdctl destroy` that destroys all resources, terraform.tfstate becomes an empty `{}` (technically `{"version":4,"resources":[]}`). Cleanup intentionally leaves it. The next deploy runs Setup which calls cleanup-WorkOnly via Provisioner.Prepare — but cleanup.WorkOnly does NOT include the terraformStep, so the orphan empty tfstate persists across cluster lifecycles. A second deploy with a different cluster name reuses that empty tfstate; tfstate.terraform_version may have been written by an older terraform and silently mismatch the new HCL.  
**Fix:** After a successful `terraform destroy` (HasState() returns false / tfstate has zero resources), have cleanup remove terraform.tfstate as well. Today the destroy code path doesn't tell cleanup 'destroy succeeded'; pipe a flag through DestroyOpts → cleanup.Options that gates an `if !HasState() then remove tfstate` step.  
**Effort:** hours

##### `state:48688e63:proxmox-no-retry-on-init-apply` — proxmox no retry on init apply

**Status:** not started  
**Severity:** minor  
**Cluster:** proxmox-api-idempotency  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:159-227`  
**Problem:** Provider.Provision delegates retry/backoff to the bpg/proxmox terraform provider via the doc-comment 'mutation invariant' — fair. But there is no okdctl-side retry around `terraform init` itself: if the Proxmox API is briefly unreachable while terraform tries to download the provider plugin or fetch its initial schema, init fails with a network error, the orchestrator marks StepDeployInfra failed (fatal — only StepDeployInfra is the only fatal in install), and the operator must re-run the entire deploy. There's no per-step retry seam.  
**Fix:** Wrap terraform.Executor.Init in a 3-attempt retry-with-jitter for transient errors (network EAI_AGAIN, 5xx-class executor.ExitError). Reuse internal/download's retryDownload pattern referenced in the proxmox.go doc-comment. Apply already retries via the provider; init is the gap.  
**Effort:** hours

##### `state:632c9087:update-ingress-no-rollback-on-dns` — update ingress no rollback on dns

**Status:** not started  
**Severity:** minor  
**Cluster:** crash-recoverability  
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:237-279`  
**Problem:** finalizeIngress deploys production DNS, then optionally removes HAProxy. If RemoveHAProxy fails AFTER DNS has been swapped from bootstrap → production, the cluster is left with DNS pointing at the new VIP/apps but HAProxy still listening on bastion — incoming requests bypass kube-vip routing. There's no rollback to bootstrap DNS on a failed haproxy removal. Operator's symptom is 'requests to *.apps.cluster intermittently route to wrong backend'.  
**Fix:** Sequence ops so DNS swap is the LAST destructive step (RemoveHAProxy first, then DNS). Or: on RemoveHAProxy failure, attempt rollback to bootstrap DNS via dns.DeployBootstrap and surface a strong error (not warn). RemoveHAProxy itself already verifies via VIP — the integration boundary is in finalizeIngress.  
**Effort:** hours

##### `state:6424733c:project-marker-stale` — project marker stale

**Status:** not started  
**Severity:** minor  
**Cluster:** crash-recoverability  
**Evidence:** `internal/cli/helpers.go:115-162`  
**Problem:** hasProjectMarker accepts EITHER okdctl.yaml, okdctl.env, or any terraform.tfstate under environments/. After a successful destroy that ran the cleanup phase, terraform.tfstate may still be present (cleanup intentionally preserves it for resumability), AND okdctl.yaml may have been removed via `rm`, AND okdctl.env may have been deleted by an operator paranoid about credentials. The hasProjectMarker check then succeeds purely on a leftover terraform.tfstate, allowing okdctl deploy to run with a default config it just generated, against state from a different cluster. The marker is OR-of-three; nothing checks consistency across them.  
**Fix:** When only terraform.tfstate is found (no okdctl.yaml, no okdctl.env), require an additional consistency check: parse the tfstate's outputs.cluster_name (if present) and warn the operator. OR demand okdctl.yaml as the primary marker and treat the tfstate as a secondary 'recovery hint' the resolveProjectRootOrDie message names explicitly.  
**Effort:** hours

##### `state:881d089e:lock-stale-host-different` — lock stale host different

**Status:** not started  
**Severity:** minor  
**Cluster:** tf-state-atomicity  
**Evidence:** `internal/runlock/runlock.go:32-77`  
**Problem:** runlock.Acquire writes PID/HOST/VERB/TIME into the lockfile but never validates the recorded HOST against os.Hostname() before flock contends. Two operators running okdctl on hosts foo and bar that share the project tree (NFS, syncthing) get a confusing 'another okdctl process holds the project lock' message naming a host the second operator cannot reach. Worse, on a cross-host stale lock (foo SIGKILL'd, bar tries deploy) the kernel-flock release does NOT propagate over NFSv3, so the second operator hangs forever with no actionable hint.  
**Fix:** In Acquire, after reading the conflicting body, parse HOST=<x> and compare to os.Hostname(). When different, append `; lock holder is on a different host (NFS-detected). On NFSv3 flock is advisory across hosts — verify with 'fuser .okdctl.lock' before deleting`. The package doc already calls out NFS pre-v4 — surface that to the operator at conflict time, not just in source comments.  
**Effort:** hours

##### `state:b38ec9cc:workers-targeted-apply-vars-not-snapshot` — workers targeted apply vars not snapshot

**Status:** not started  
**Severity:** minor  
**Cluster:** phase-idempotency  
**Evidence:** `internal/distribution/okd/install/workers.go:19-56`  
**Problem:** StartWorkerVMs runs `terraform apply -var start_workers_immediately=true -target=module.okd_cluster.proxmox_virtual_environment_vm.worker`. The override is at apply-time only; nothing writes `start_workers_immediately = true` back into terraform.tfvars, and the AlreadyDone check is missing. A re-run after a crash between bootstrap-complete and worker-start hits this step idempotent-by-terraform — fine — BUT a second `okdctl deploy` later with skip-bootstrap will re-execute this step against an already-running cluster, because there's no AlreadyDone hook. The orchestrator runs it; terraform diffs zero changes; safe but noisy. More important: a manual `terraform plan` from the workdir AFTER deploy completes still shows the workers diff as 'will set start_workers_immediately=true' because tfvars was never updated. The doc-comment acknowledges this — the gap remains.  
**Fix:** Either: (a) declare ReRunSafeNo with an AlreadyDone hook that checks `oc get nodes -l node-role.kubernetes.io/worker` count >= cfg.Topology.Workers.Count, OR (b) write the var back to tfvars via setup.GenerateTerraformVars's renderAndWrite path so manual `terraform plan` is no-op-clean. (a) is closer to existing phase patterns. The current ReRunSafeYes is correct but skips the cheap precondition check that would let `okdctl deploy` resume cleanly without re-driving terraform after install.  
**Effort:** hours

##### `state:b5a79fda:deploy-state-marker-stale` — deploy state marker stale

**Status:** not started  
**Severity:** minor  
**Cluster:** crash-recoverability  
**Evidence:** `internal/cli/deploystate.go:24-86`  
**Problem:** announceDeployState reads .okdctl-deploy-state.json on destroy entry and emits 'partial deploy detected — cancelled during X'. But there's no TTL or mtime check: a marker left from a successful deploy whose clearDeployMarker failed (errno EBUSY, RO mount, signal during cleanup) misleads the operator on a totally unrelated destroy weeks later. The deploy marker also doesn't survive cross-run identity (RunID is per-process, not per-cluster), so two operators on the same project can't tell which one's partial deploy left this marker.  
**Fix:** On readDeployState, also stat the file and emit the marker's age in the warning ('marker is 14 days old — likely stale'). Add a ClusterName field to deployState struct; on announceDeployState, when ds.ClusterName != cfg.Cluster.Name, emit 'marker is from a different cluster, ignoring'. Make markDeployPhase a fatal (not best-effort) on the first call so a write-failed marker can't accumulate.  
**Effort:** hours

##### `state:b804b2ec:bootstrap-cleanup-vars-not-snapshot` — bootstrap cleanup vars not snapshot

**Status:** not started  
**Severity:** minor  
**Cluster:** phase-idempotency  
**Evidence:** `internal/distribution/okd/postinstall/bootstrap.go:18-61`  
**Problem:** CleanupBootstrap apply-overrides bootstrap_enabled=false but never persists it. `okdctl destroy` later runs without that var and terraform reads bootstrap_enabled=true from tfvars, attempting to recreate the bootstrap VM which was already destroyed in postinstall — terraform reports a no-op because tfstate is already empty for that resource, BUT the destroy plan flags 'create' on bootstrap which is confusing in plan output. Same class of issue as workers.go but on a destroy-adjacent path.  
**Fix:** After successful CleanupBootstrap, rewrite terraform.tfvars with bootstrap_enabled=false (use system.AtomicWriteString and the same renderAndWrite scaffold setup uses). On a later destroy, plan output is clean. Alternative: route bootstrap_enabled through a generated `bootstrap-state.auto.tfvars.json` that takes precedence over the static tfvars and is updated atomically.  
**Effort:** hours

##### `state:c287d5c0:destroy-auto-approve-hardcoded` — destroy auto approve hardcoded

**Status:** not started  
**Severity:** minor  
**Cluster:** destroy-safety  
**Evidence:** `internal/distribution/okd/okd.go:219-231`  
**Problem:** Provisioner.Destroy hard-codes destroyOpts.AutoApprove=true regardless of cfg.Deployment.AutoApprove or any CLI flag. The CLI runDestroy path does its own confirm prompt, but anyone constructing a Provisioner programmatically (test harness, future MCP server) gets non-interactive terraform destroy with no upstream check. There is no programmatic DestroyOpts.AutoApprove field operators can set false.  
**Fix:** Add `AutoApprove bool` to DestroyOpts and propagate via `destroyOpts.AutoApprove = opts.AutoApprove`. CLI sets it true after passing the cluster-name confirmation; library callers can opt into interactive flow. Mirror the symmetric ProvisionOptions.AutoApprove already in place.  
**Effort:** hours

##### `state:eb479d86:upload-resume-not-supported` — upload resume not supported

**Status:** not started  
**Severity:** minor  
**Cluster:** phase-idempotency  
**Evidence:** `internal/distribution/okd/setup/upload.go:82-146`  
**Problem:** UploadCustomISOsToProxmox runs scp of every ISO whose remote sha256 differs from local in a single subprocess. If the operator Ctrl-C's mid-upload (or the network drops) after 3 of 4 ISOs landed, the partial 4th file remains on the Proxmox host with a corrupt content. The next AlreadyDone check (isoUploadAlreadyDone) hashes the remote — sees mismatch — and re-uploads ALL ISOs (not just the partial), because scp invocation is a single batch. Per-file scp would let resume only re-upload the corrupt tail. ISOs are 1-2 GiB; this matters on residential bastion uplinks.  
**Fix:** Iterate per-file in uploadISOsViaSCP; before each scp, run isoUploadNeeded specifically for that one file (skip already-matching). After SIGINT mid-batch, the next deploy resumes only the missing/corrupt tail. Keep the size logging summary; per-file logging adds <10 LOC.  
**Effort:** hours

##### `state:262af6e4:cleanup-no-resume-doc` — cleanup no resume doc

**Status:** not started  
**Severity:** suggestion  
**Cluster:** crash-recoverability  
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:1-21`  
**Problem:** Package doc says 'Cleanup is best-effort: a mid-run crash leaves workDir in a partially-removed state with no resume capability'. This is an acknowledged gap. The cleanupTracker accumulates errs into a joined error returned by StepCleanupSummary, but the `okdctl cleanup` command surfaces only the final error string — no per-step status the operator can use to decide which subsystems still need manual cleanup. After a SIGINT mid-cleanup, there's no diagnostic pointing at which steps succeeded.  
**Fix:** In runCleanup (cli/cleanup.go), after the orchestrator returns, log a per-step status table from orchestrator.Results() at Info level. On error, also include in the user-facing message: 'partial cleanup; rerun to retry; subsystems still active: <names from t.errs>'. The Summary struct in cleanup/summary.go already shows the inverse (what's left); the orchestrator results show what was attempted. Tying the two is a 5-line change in printSummary.  
**Effort:** hours

##### `state:f743eaa2:iso-build-fingerprint-not-fsynced` — iso build fingerprint not fsynced

**Status:** not started  
**Severity:** suggestion  
**Cluster:** crash-recoverability  
**Evidence:** `internal/distribution/okd/setup/iso.go:91-108`  
**Problem:** BuildCustomISOs writes the .fp-<node> fingerprint via system.AtomicWriteString AFTER buildNodeISO completes successfully — correct ordering. But if the kernel crashes between buildNodeISO returning success and AtomicWriteString completing the fsync, the .iso file exists but the fingerprint file is missing or stale. Next run sees stale fp and rebuilds the ISO unnecessarily — wasteful but safe. The actual gap is purely 'wasted rebuild on crash recovery'; flagging because the doc-comment promises fingerprint-based skip and a partial-write scenario silently loses that contract.  
**Fix:** Move the fingerprint write to occur as the LAST line of buildNodeISO (after coreos-installer succeeds, before the function returns). Same crash semantics; cleaner reasoning. Or document the wasted-rebuild-on-crash as the deliberate trade-off in the doc-comment.  
**Effort:** hours

#### audit-iac-and-shell

##### `iac:b803fcb7:tfsec-soft-fail` — tfsec soft fail

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/iac-b803fcb7-tfsec-fail-closed  
**Severity:** major  
**Cluster:** hcl-provider-hygiene  
**Evidence:** `.github/workflows/ci.yml:114-117`  
**Problem:** tfsec-action runs with soft_fail: true, so HCL security findings produce job logs but never fail scan-terraform. CLAUDE.md §Tooling states all CI checks must be green before merging — soft_fail makes tfsec un-gating, so the green check is cosmetic.  
**Fix:** Drop soft_fail: true (or set false). Add .tfsec/config.yml to suppress known-acceptable findings by rule code with documented justification — but the default must be gating.  
**Effort:** hours

##### `iac:e076e43c:cosign-optional-when-absent` — cosign optional when absent

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/iac-e076e43c-cosign-required  
**Severity:** major  
**Cluster:** install-sh-integrity — seam→audit-security  
**Evidence:** `scripts/install.sh:141-146`  
**Problem:** When cosign is not installed and INSECURE is unset, the script silently downgrades to sha256-only trust. SHA256SUMS is fetched from the same release origin as the binary, so a single compromised release host (or a defeated TLS chain) replaces both. CWE-494: download without integrity check. The INSECURE=1 branch at L60-L66 enforces opt-in when cosign IS present, but the cosign-absent path bypasses that gate.  
**Fix:** Either (a) require cosign as a hard prerequisite (mirror the sha256sum require at L49-L53 — die early with an install-cosign hint), or (b) print the cosign-absent message at WARN level via red() so the trust-degradation is visible. Option (a) matches helm/kind/terraform installer conventions.  
**Effort:** hours

##### `iac:18a795d5:depends-on-bootstrap-artificial` — depends on bootstrap artificial

**Status:** not started  
**Severity:** minor  
**Cluster:** hcl-destroy-ordering — seam→audit-state-and-recovery  
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/main.tf:243-243` + 1 more  
**Problem:** master declares depends_on = [bootstrap]; worker declares depends_on = [master]. Neither references the other's attributes; at the Proxmox API level the resources are independent. The OKD-installer-level dependency (masters need bootstrap to come up to fetch ignition) is not a Terraform-graph concern. Hashicorp guidance: depends_on only when the dep cannot be expressed via attribute reference.  
**Fix:** Drop both depends_on. Document destroy sequence in README or rely on okdctl's own two-phase destroy in internal/distribution/okd/destroy/. Verify with a dev destroy/apply cycle to rule out a Proxmox storage-release race.  
**Effort:** hours

##### `iac:18a795d5:worker-data-disk-no-prevent-destroy` — worker data disk no prevent destroy

**Status:** not started  
**Severity:** minor  
**Cluster:** hcl-destroy-ordering — seam→audit-state-and-recovery  
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/main.tf:328-339`  
**Problem:** Workers attach a 500 GiB Ceph data disk by default (worker_data_disk_size_gb = 500 in variables.tf:85) under a dynamic disk block. Variable description warns that lowering destroys the disk, but the worker resource has no prevent_destroy = true (unlike masters), and disk is not in lifecycle.ignore_changes — only network_device, startup, cdrom, boot_order, efi_disk are. terraform apply with worker_data_disk_size_gb = 0 silently destroys the data disk. The variable description warns; the IaC layer doesn't.  
**Fix:** Either (a) add prevent_destroy = true on the worker resource for production-mode (matching the master pattern + override.tf escape hatch documented at main.tf:245-254), or (b) extend lifecycle.ignore_changes to include disk for the data-disk attributes so subsequent applies do not size-down. Option (b) is narrower; option (a) is operator-grade.  
**Effort:** hours

##### `iac:b803fcb7:tflint-version-unpinned` — tflint version unpinned

**Status:** not started  
**Severity:** minor  
**Cluster:** hcl-provider-hygiene — seam→audit-dependencies  
**Evidence:** `.github/workflows/ci.yml:101-101`  
**Problem:** setup-tflint action is correctly SHA-pinned per CLAUDE.md §Pin stability, but with: omits tflint_version, so the action installs whichever tflint the runner defaults to. CLAUDE.md says tool installs from Go must be explicit versions — never @latest. Compare with terraform_version: 1.10.3 at L91 which IS patch-pinned.  
**Fix:** Add with: { tflint_version: "v0.55.0" } (or current pin) to both setup-tflint invocations. Rev together with ruleset bumps.  
**Effort:** hours

##### `iac:e076e43c:tar-zipslip-incomplete` — tar zipslip incomplete

**Status:** not started  
**Severity:** minor  
**Cluster:** install-sh-integrity  
**Evidence:** `scripts/install.sh:165-167`  
**Problem:** The defense-in-depth zip-slip check uses grep -qE '^(\.\.|/)' — only catches entries starting with .. or /. A tar entry like subdir/../../etc/passwd starts with subdir/ and silently passes. Goreleaser tarballs are flat and cosign+sha256 is the primary guard, so this is purely defense-in-depth — but as written it doesn't defend against the canonical tar zip-slip.  
**Fix:** Use the canonical anti-zip-slip regex: grep -qE '(^|/)\.\.(/|$)|^/' — catches .. as the first segment, any internal /../ segment, or any absolute path. Or rely on tar --no-anchored --exclude='*/../*' --exclude='/*' at extract time.  
**Effort:** hours

##### `iac:ef8f2924:bootstrap-default-shadows-coalesce` — bootstrap default shadows coalesce

**Status:** not started  
**Severity:** minor  
**Cluster:** hcl-credential-hygiene  
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/variables.tf:200-228`  
**Problem:** Three variables described as 'defaults to memory_mb / cpu_cores if not set' carry non-null defaults that defeat the coalesce-fallback in main.tf locals: bootstrap_memory_mb default 8192 (L203), worker_cpu_cores default 8 (L221), worker_memory_mb default 20480 (L227). coalesce(non_null, x) returns the first arg, so setting memory_mb=16384 does not raise these. Compare with master_cpu_cores / master_memory_mb / bootstrap_cpu_cores which correctly default to null.  
**Fix:** Set defaults to null to match the documented fallback semantics. If a per-role floor is intended (e.g., bootstrap >= 8 GiB), encode via validation or max(local.cluster_memory, 8192) rather than via a default that shadows operator input.  
**Effort:** hours

##### `iac:04b033b9:provider-insecure-not-pinned-false` — provider insecure not pinned false

**Status:** not started  
**Severity:** suggestion  
**Cluster:** hcl-credential-hygiene — seam→audit-security  
**Evidence:** `infrastructure/terraform/environments/production/versions.tf:1-10`  
**Problem:** Production env declares the bpg/proxmox provider via required_providers but has no provider "proxmox" {} block. The provider therefore consumes PROXMOX_VE_INSECURE from env. variables.tf:7 / main.tf:10 say DEV ONLY — but production HCL doesn't enforce that. An operator who exports PROXMOX_VE_INSECURE=true silently disables TLS verification with no audit trail in the .tf files.  
**Fix:** Add an explicit provider "proxmox" {} block in production env that pins insecure = false. Endpoint / username / password keep coming from PROXMOX_VE_* env vars. Dev environments (when added) leave insecure unset.  
**Effort:** hours

#### audit-errors

##### `err:48688e63:cancel-identity-lost-on-tf-apply` — cancel identity lost on tf apply

**Status:** not started  
**Severity:** major  
**Cluster:** cancellation-identity — seam→audit-concurrency  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:195-203`  
**Problem:** On terraform-apply cancellation, applyErr is the executor.ExitError from a SIGTERM'd subprocess — it does NOT carry context.Canceled in its chain. The wrap at L199 forwards applyErr only ('terraform apply interrupted: %w', applyErr); errors.Is(returned, context.Canceled) downstream returns false because the underlying chain is ExitError → exec.ExitError, no ctx identity. cli/root.go::signalExitCode (L183) gates the SIGINT→130 / SIGTERM→143 mapping on errors.Is(err, context.Canceled || DeadlineExceeded), so a Ctrl-C during terraform apply currently exits 1 instead of 130. install/monitor.go:68 has the canonical fix shape: 'fmt.Errorf(...: %w', ctx.Err())'.  
**Fix:** Wrap ctx.Err() instead of (or alongside) applyErr: 'return nil, fmt.Errorf("terraform apply interrupted: %w", ctx.Err())'. Or use errors.Join(ctx.Err(), applyErr) so both identities walk through. Net +0 LOC. Apply the same fix to PlanOnly's plan / Init paths if they grow ctx-cancel checks.  
**Effort:** hours

##### `err:7b2829bb:exit-error-no-redact` — exit error no redact

**Status:** not started  
**Severity:** major  
**Cluster:** redaction-in-error — seam→audit-observability — related: obs:6424733c:fmt-sprintf-message-pattern  
**Evidence:** `internal/executor/executor.go:187-198`  
**Problem:** executor.ExitError carries a Stderr string field but does NOT implement Redacted() any. The sibling type SubprocessError at internal/system/exec.go:42 does — the asymmetry means subprocess stderr surfaced through ExitError reaches slog/tui sinks unredacted. ExitError is the canonical typed exit error returned by RunChecked, RunStreamedChecked, K8sClient.runCheck, terraform.run, and OcOutput; every site that wraps an oc/kubectl/terraform/sops failure into the cli error chain forwards the raw subprocess stderr.  
**Fix:** Add Redacted() any returning {Command, ExitCode} (drop Stderr) so logutil.RedactHandler's interface{ Redacted() any } switch (internal/logutil/redact.go:113) catches ExitError when it lands as a slog attr. Mirror system.SubprocessError.Redacted at internal/system/exec.go:42-44. Net +4 LOC. terraform.ExecError aliases ExitError so it inherits automatically.  
**Effort:** hours

##### `err:fd2125dd:cli-bare-errors-skip-typed-mapping` — cli bare errors skip typed mapping

**Status:** not started  
**Severity:** major  
**Cluster:** sentinel-vs-typed — seam→audit-cli-ux  
**Evidence:** `internal/cli/addon.go:65-255` + 16 more  
**Problem:** CLI subcommand RunE returns plain fmt.Errorf for what are clearly typed-error categories: usage misconfig (--all + named addon mutually exclusive, unknown shell, invalid --channel/--output), missing config artifact (kubeconfig not found at ...; run okdctl deploy first), or executable-resolution failure. exitCodeFor (cli/root.go:L192-L231) maps errtypes.UsageError→64, ErrConfigMissing→66, but these sites never hit the map and fall to exit 1. The CLI's documented exit-code contract (cli/root.go:L4-L10, docs/cli/exit-codes.md) silently fails for all these surfaces.  
**Fix:** Wrap each in the right errtypes value: usage failures → &errtypes.UsageError{Msg: "..."} (exit 64); kubeconfig.go:49 → &errtypes.ConfigError{Msg: ..., Err: errtypes.ErrConfigMissing} (exit 66); addon.go:255 → &errtypes.ClusterError{...} (exit 4); elevation.go:104 → &errtypes.ConfigError{Msg: ..., Err: err} (exit 2). Mechanical, ~15 sites, ~+30 LOC.  
**Effort:** hours

##### `err:62cb8a95:state-lock-hint-drops-init-cause` — state lock hint drops init cause

**Status:** not started  
**Severity:** minor  
**Cluster:** wrapping — seam→audit-state-and-recovery  
**Evidence:** `internal/distribution/okd/destroy/helpers.go:124-129`  
**Problem:** When tf.Init fails AND a state lock file is present, the original Init error is silently discarded ('return hint'). The operator sees 'terraform state locked at ...' but loses the actual init failure context (which may itself be the cause of the orphan lock — e.g. a corrupted .terraform/ directory). errors.Join would carry both; today only the hint reaches the user.  
**Fix:** errors.Join the two: 'return errors.Join(hint, &errtypes.ClusterError{Msg: "terraform init failed", Err: err})'. Operator sees both: lock-hint AND the underlying Init failure. Net +1 LOC.  
**Effort:** hours

##### `err:6424733c:env-file-double-context` — env file double context

**Status:** not started  
**Severity:** minor  
**Cluster:** wrapping  
**Evidence:** `internal/cli/helpers.go:53-55` + 2 more  
**Problem:** loadEnvFile callers wrap with 'load env file <path>: %w' on top of an inner ConfigError/AuthError that already names the path ('failed to open env file <path>', '.env file has insecure permissions ...'). Result: 'load env file /home/x/okdctl.env: failed to open env file /home/x/okdctl.env: open /home/x/okdctl.env: ...'. errors.As traversal still finds the typed inner so exit code is preserved — the cost is purely operator-readability double-context. Three sites.  
**Fix:** Drop the outer wrap and return err directly: 'if err := credentials.LoadEnvFile(envPath); err != nil { return nil, err }'. The inner ConfigError/AuthError already names the path. Three sites: helpers.go:53-55, deploy.go:147, deploy.go:246. Net -6 LOC.  
**Effort:** hours

##### `err:881d089e:runlock-untyped-errors` — runlock untyped errors

**Status:** not started  
**Severity:** minor  
**Cluster:** sentinel-vs-typed — related: err:fd2125dd:cli-bare-errors-skip-typed-mapping  
**Evidence:** `internal/runlock/runlock.go:47-52`  
**Problem:** Acquire returns ConfigError for symlink-refusal (L42) and lock-conflict (L62), but bare fmt.Errorf for the lstat (L47) and OpenFile (L52) syscall failures. A read-permission fault on the project root falls to exit 1 instead of an AuthError (exit 5) or ConfigError (exit 2). The two paths share the same external failure shape (the operator's project root has the wrong permissions or doesn't exist) and should map consistently.  
**Fix:** Wrap each in &errtypes.ConfigError{Msg: fmt.Sprintf("runlock: lstat %s", path), Err: err} and similar for OpenFile. Consistent with the sibling cases at L42/L62. Net +6 LOC.  
**Effort:** hours

##### `err:c19ee328:phase-step-bare-fmt-errorf` — phase step bare fmt errorf

**Status:** not started  
**Severity:** minor  
**Cluster:** sentinel-vs-typed — seam→audit-cli-ux — related: err:fd2125dd:cli-bare-errors-skip-typed-mapping  
**Evidence:** `internal/distribution/okd/setup/steps.go:380-430` + 55 more  
**Problem:** Phase step bodies return plain fmt.Errorf('failed to X: %w', err) for what semantically map to ConfigError (manifest write/render), ClusterError (oc/terraform/install), or NetworkError (download/scp). The orchestrator forwards them as-is to cli, which falls to exit 1. addon.Manager wraps step bodies into ClusterError at internal/addon/manager.go:L123, so addon catalog code is shielded — but every direct phase step (setup/install/postinstall) returns naked errors.  
**Fix:** Pick a wrap site: either inside each step body (touching dozens of files), or in the orchestrator at the boundary where step.Execute returns (one place, retro-classifies via heuristic). The latter loses semantic precision; the former is correct. Pragmatic compromise: wrap at the phase-orchestrator entry (Phase.Execute or Provisioner.Install) so each phase has one ClusterError boundary. ~+10 LOC.  
**Effort:** hours

##### `err:fde34e0c:exit-error-no-ctx-identity` — exit error no ctx identity

**Status:** not started  
**Severity:** minor  
**Cluster:** cancellation-identity — seam→audit-concurrency — related: err:48688e63:cancel-identity-lost-on-tf-apply  
**Evidence:** `internal/cluster/k8s.go:108-127` + 6 more  
**Problem:** Every site that constructs an executor.ExitError on a non-zero exit code does so without consulting ctx.Err() — so a subprocess SIGTERM'd via cmd.Cancel that happens to exit non-zero produces an ExitError chain with no context.Canceled identity. Downstream errors.Is(err, context.Canceled) returns false at cli/root.go::signalExitCode, mapping the SIGINT to exit 4 (ClusterError) instead of 130. Pattern repeats at executor.go:L303,L355,L367, k8s.go:L120, phase/kubectl.go:L35, setup/release_extract.go:L128, setup/upload.go:L28. install/monitor.go has the canonical fix shape (check ctx.Err first).  
**Fix:** Centralise: at every ExitError construction site, prefer ctx.Err() when ctx is cancelled. Cleanest landing is a helper in executor: 'func newExitError(ctx context.Context, cmd string, code int, stderr string) error { if err := ctx.Err(); err != nil { return err }; return &ExitError{Command: cmd, ExitCode: code, Stderr: stderr} }'. 7 call sites use it. Net +6 LOC.  
**Effort:** hours

##### `err:5013fea6:auth-error-string-sniffing` — auth error string sniffing

**Status:** not started  
**Severity:** suggestion  
**Cluster:** string-sniffing  
**Evidence:** `internal/distribution/okd/setup/release_extract.go:92-141`  
**Problem:** isAuthError performs case-insensitive substring matching against an authMarkers list ('unauthorized','authentication','denied','forbidden','no basic auth','401','403') against subprocess stderr to classify ClusterError vs AuthError. The pattern is exactly what audit-errors string-sniffing rule catches: oc/registry stderr text is not a stable contract. Currently mitigated because exit code is the primary signal (L129) — string match is the secondary lift. Self-documented as best-effort with a roadmap link.  
**Fix:** Track upstream openshift/oc for a typed registry-auth error envelope. Until then, accept the documented best-effort. Optionally tighten: prefer to drop the broad 'authentication'/'denied' markers (high false-positive rate vs benign network errors) and rely solely on '401'/'403'/'unauthorized' (HTTP-status-aligned). Net -3 LOC.  
**Effort:** hours

##### `err:9f8e7d6c:errtypes-vocab-cert-pending` — errtypes vocab cert pending

**Status:** not started  
**Severity:** suggestion  
**Cluster:** domain-vocabulary — seam→audit-cli-ux  
**Evidence:** `internal/errtypes/errtypes.go:1-111`  
**Problem:** errtypes vocabulary has 5 concepts (Config/Network/Cluster/Auth/Usage) but no concept for 'transient' / 'recoverable' failures (a vip cert not yet rotated, a CSR pending approval, an oc operator still settling). Today these are forced into ClusterError, then cli maps to exit 4 and shell scripts cannot tell 'cluster permanently degraded' from 'wait and retry'. Suggestion-grade because no caller is forced to mis-classify today; the hole emerges when a retry-aware shell wrapper appears.  
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.  
**Effort:** hours

##### `err:ddf885f4:install-all-bare-ctx-err` — install all bare ctx err

**Status:** not started  
**Severity:** suggestion  
**Cluster:** cancellation-identity  
**Evidence:** `internal/addon/manager.go:85-113`  
**Problem:** InstallAll returns ctx.Err() directly (L86) without a typed wrap. cli/root.go::signalExitCode walks errors.Is(err, context.Canceled) and resolves to exit 130, so this works in practice. But the same loop at L110-L112 conditionally appends ctxErr to errs and joins — meaning two paths exist: bare-return (no other failures yet) versus joined (some failures already). The asymmetry is intentional but undocumented; a future refactor that 'simplifies' to always-bare or always-join would silently break the partial-failure aggregate.  
**Fix:** Add a one-line WHY comment at L85: '// bare ctx.Err so cli/root.go::signalExitCode resolves SIGINT→130 without a typed wrap; the L110 path joins because partial-failure errs already prevent the bare ctx return from running'. Net +1 LOC, +0 behavior change.  
**Effort:** hours

#### audit-concurrency

##### `con:0b188cab:retry-eats-non-retryable` — retry eats non retryable

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/con-0b188cab-retry-fast-fail  
**Severity:** major  
**Cluster:** time-sleep-retry  
**Evidence:** `internal/addon/helpers.go:23-41`  
**Problem:** RetryDefault retries every fn() error through the full backoff budget (3 steps, 5s base, factor 2, jitter 0.5, 5min cap). The //nolint:nilerr suppresses errcheck but documents the bug: a permanent failure (auth denied, oc binary missing, malformed addon manifest) consumes the full ~35s of backoff before surfacing. EnsureNamespace is one caller — a missing oc binary should fail fast, not after 3 retries.  
**Fix:** Mirror download/retry.go::isRetryable: add an Addon-level isRetryable(error) check (oc-binary-missing, ctx-canceled, ConfigError, AuthError → fail-fast). Return (false, fnErr) for non-retryable so wait aborts immediately; return (false, nil) only for transient (5xx, connection refused, transient executor.ExitError).  
**Effort:** hours

##### `con:06f00bcb:ctx-ignored-file-io` — ctx ignored file io

**Status:** not started  
**Severity:** minor  
**Cluster:** ctx-ignored  
**Evidence:** `internal/distribution/okd/setup/apache.go:48-95` + 1 more  
**Problem:** configureApachePort(_ context.Context, bindIP string) accepts ctx but never reads it. The body does substantial file I/O (system.CopyFile, os.ReadFile, bufio.Scanner of 1MiB buffer, AtomicWrite) that could hold the deploy ctx past a SIGINT. ensureIgnitionDir at L28 has the same shape. ConfigureApache (the caller) propagates a real ctx; dropping it inside the body breaks cancellation responsiveness for the whole apache-config step.  
**Fix:** Either rename the param to a real `ctx context.Context` and check `if err := ctx.Err(); err != nil { return }` at the top of each branch (cheap cancellation gate), or drop the ctx parameter from the signature entirely if cancellation responsiveness is genuinely a non-goal here. The `_ context.Context` placeholder is the worst of both worlds — looks like ctx threading without delivering it.  
**Effort:** hours

##### `con:8ea706f6:ctx-ignored-install-binary` — ctx ignored install binary

**Status:** not started  
**Severity:** minor  
**Cluster:** ctx-ignored  
**Evidence:** `internal/distribution/okd/setup/tools.go:240-253`  
**Problem:** installBinaryToPath(_ context.Context, srcPath, name string) accepts ctx but ignores it. The body calls system.CopyFile (privileged write to /usr/local/bin under sudo re-exec) plus system.MakeExecutable (chmod +x). Inside a long-running tools-install step, dropping ctx means a SIGINT after the privileged copy starts has no chance to abort before the next binary lands. Cheap fix: ctx.Err() at function top.  
**Fix:** Rename `_ context.Context` to `ctx context.Context` and add `if err := ctx.Err(); err != nil { return err }` at the top. Each `installBinaryToPath` call is bounded by a single cp+chmod, so this gate is sufficient — fancier ctx-aware copy isn't needed.  
**Effort:** hours

##### `con:ab9b764a:ctx-ignored-install-config` — ctx ignored install config

**Status:** not started  
**Severity:** minor  
**Cluster:** ctx-ignored  
**Evidence:** `internal/distribution/okd/setup/ignition.go:39-106` + 1 more  
**Problem:** GenerateInstallConfig(_ context.Context, ...) reads a pull-secret file, validates JSON, and renders+writes install-config.yaml without ever consulting ctx. InjectCompactClusterManifests at L157 has the same shape. Both run inside StepDef.Exec closures whose enclosing orchestrator threads a real ctx (orchestrator.go:L84 `o.executeStep(ctx, step)`). The cancellation gate must be inside the function to bound a SIGINT mid-deploy — a 5-line install-config render is fast, but ValidateIgnitionFiles (L196, also in this file) demonstrates the correct shape with ctx.Err() at the top.  
**Fix:** Add `if err := ctx.Err(); err != nil { return err }` at the top of each function's body, mirroring ValidateIgnitionFiles at L197 in the same file. The 1-line gate is cheap and gives the orchestrator the SIGINT responsiveness CLAUDE.md §concurrency expects without changing the function signature.  
**Effort:** hours

##### `con:15ba17da:tracker-mu-not-needed-yet` — tracker mu not needed yet

**Status:** not started  
**Severity:** suggestion  
**Cluster:** waitgroup-vs-errgroup  
**Evidence:** `internal/distribution/okd/destroy/steps.go:32-69`  
**Problem:** destroyTracker.mu sync.RWMutex guards two []string slices. Orchestrator.Run executes steps serially (orchestrator.go:L76-L98) — there is no concurrent caller of onError or skipWhen today. The mutex is forward-looking (consistent with internal/distribution/context.go:L13's documented forward-looking RWMutex), but unlike context.go this site has no comment naming the future parallel-step mode.  
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.  
**Effort:** hours

##### `con:181efc90:spinner-canonical` — spinner canonical

**Status:** not started  
**Severity:** suggestion  
**Cluster:** goroutine-lifetime  
**Evidence:** `internal/tui/spinner.go:19-56`  
**Problem:** StartSpinner is a canonical ticker-backed background worker with two stop signals (stopCh closed by sync.OnceFunc, plus ctx.Done()) and a `done` channel the returned stop function blocks on for ordered teardown. CLAUDE.md §concurrency lists this as a canonical pattern. Recording so audit-code-smells or audit-modernization doesn't propose simplifying away the dual-signal shape.  
**Fix:** No change. ticker.Stop is in defer; both stop signals are wired; sync.OnceFunc guards against double-close; done channel allows ordered teardown. Preserve as the canonical pattern.  
**Effort:** hours

##### `con:39c75e91:confirm-stdin-leak-bounded` — confirm stdin leak bounded

**Status:** not started  
**Severity:** suggestion  
**Cluster:** goroutine-lifetime  
**Evidence:** `internal/cli/confirm.go:31-58`  
**Problem:** promptForConfirmation spawns a stdin-reader goroutine that cannot be cancelled (Go has no portable way to interrupt an in-flight stdin read). On ctx cancel the function returns immediately while the goroutine remains blocked on Stdin.Read until the user hits enter or the process exits. The author documents the bounded leak at L21-L30. CLAUDE.md §concurrency explicitly permits this shape: 'fire-and-forget is acceptable when (a) the work is bounded by ctx and (b) the call site documents the leak bound.' inputCh has cap=1 so the eventual goroutine send never blocks.  
**Fix:** No change. The goroutine has a documented leak bound (process lifetime), the channel is buffered cap=1, and CLAUDE.md §concurrency permits this shape with documentation. Preserving as canonical 'cancel-blocking-stdin-read' pattern for any future caller.  
**Effort:** hours

##### `con:48688e63:disconnect-ctx-ignored` — disconnect ctx ignored

**Status:** not started  
**Severity:** suggestion  
**Cluster:** ctx-ignored  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:123-129`  
**Problem:** Provider.Disconnect(_ context.Context) accepts ctx for symmetry with future network-bound providers (documented at L123-L125). The doc-comment makes the intent explicit, but the body is two assignments — the ctx parameter is pure scaffolding for an interface that does not yet exist. Symmetric with Connect at L103 which takes ctx and uses it for SSH host-key verification.  
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.  
**Effort:** hours

##### `con:6424733c:metrics-stop-shutctx-background` — metrics stop shutctx background

**Status:** not started  
**Severity:** suggestion  
**Cluster:** ctx-todo  
**Evidence:** `internal/cli/helpers.go:240-256`  
**Problem:** startMetricsServer.stop closure builds a 5-second shutdown ctx via context.Background() rather than the parent ctx. The author documents the choice (L241-L242: 'by stop() time the parent ctx is already cancelled by SIGINT, and we need the 5s drain to complete'). This is correct on the SIGINT path but means a parent timeout deadline >5s is silently ignored. The justification comment satisfies CLAUDE.md §concurrency's 'context.Background()/TODO() needs justification' rule, so this is recorded as a documented deliberate divergence rather than a bug.  
**Fix:** Keep as-is. The justification comment matches CLAUDE.md §concurrency requirement that ctx.Background() needs a justification comment. This row exists so the next audit doesn't re-flag a documented deliberate choice as a bug.  
**Effort:** hours

##### `con:8e65d574:bgcheck-canonical-leak-bound` — bgcheck canonical leak bound

**Status:** not started  
**Severity:** suggestion  
**Cluster:** goroutine-lifetime  
**Evidence:** `internal/version/updatecheck.go:36-58`  
**Problem:** BackgroundCheck spawns a goroutine that does an HTTP GET with a 4s timeout and writes exactly one CheckResult into a buffered chan cap=1. The send never blocks (cap=1, single sender). The caller (cli/root.go::printUpdateNotice) waits at most 100ms before giving up. CLAUDE.md §concurrency explicitly cites this site as the canonical 'fire-and-forget with documented leak bound (httpTimeout=4s)' pattern.  
**Fix:** No change. ctx propagates through fetchLatest → http.NewRequestWithContext, the chan is buffered cap=1, leak bound is httpTimeout=4s, doc comment names the leak bound. Preserve as canonical pattern.  
**Effort:** hours

##### `con:97cb8adf:waitfor-check-no-ctx` — waitfor check no ctx

**Status:** not started  
**Severity:** suggestion  
**Cluster:** ctx-ignored  
**Evidence:** `internal/system/exec.go:107-167`  
**Problem:** WaitFor's `check func() bool` callback takes no ctx, so polling sites cannot propagate cancellation into their probe. Today every caller closes over an outer ctx (e.g. OcPollOutputInterval at phase/kubectl.go:L63 captures ctx in the closure for `p.Exec.Run(ctx,...)`), but the contract leaves the door open: a future caller that does CPU work or blocking syscalls in `check` has no way to honour ctx mid-probe. The select arm at L141 only fires between probe attempts, not during one.  
**Fix:** Change check signature to `check func(context.Context) bool`. Pass the outer ctx to each invocation. Update callers (BasePhase.OcPollOutputInterval is the main one) to thread ctx through. Today's only caller already captures ctx, so the rename is a 5-LOC patch with no behavioural change.  
**Effort:** hours

##### `con:aa84670c:signalloop-bounded-leak` — signalloop bounded leak

**Status:** not started  
**Severity:** suggestion  
**Cluster:** goroutine-lifetime  
**Evidence:** `internal/cli/root.go:99-152`  
**Problem:** signalLoop is a documented bounded-leak goroutine: on the happy path execute()'s defer signal.Stop + close(sigCh) fires, the receiver observes !ok and returns. On the SIGINT-twice path the goroutine calls os.Exit(130) directly. Preserving as canonical signal-watched-ctx pattern. Recently fixed (L137-L138 comments) to add the close(sigCh) after signal.Stop so the receiver returns on happy path rather than blocking until process exit.  
**Fix:** No change. The fix-up comment at L137-L138 was the result of the prior audit run (a `con` row likely flagged the bounded leak). Preserve as canonical signal-watched-ctx pattern.  
**Effort:** hours

##### `con:ae5b624c:monitor-cmd-cancel-pattern` — monitor cmd cancel pattern

**Status:** not started  
**Severity:** suggestion  
**Cluster:** goroutine-lifetime — seam→audit-subprocess — related: sub:7b2829bb:no-cancel-func  
**Evidence:** `internal/distribution/okd/install/monitor.go:24-43`  
**Problem:** defaultStartMonitorCmd is the canonical cmd.Cancel + cmd.WaitDelay pattern CLAUDE.md §concurrency points to. The goroutine at L38-L41 does `defer close(doneCh); doneCh <- cmd.Wait()` — clean, buffered chan cap=1, single sender, defer closes after send, no leak. Recorded as preservation of canonical helper so audit-modernization or audit-code-smells doesn't propose 'simplify' that breaks the pattern.  
**Fix:** No change. This is the canonical pattern. Audit-subprocess sub:7b2829bb:no-cancel-func explicitly cites this site as the pattern executor.RunInteractive should mirror; preserving it bit-for-bit is load-bearing.  
**Effort:** hours

#### audit-api-design

##### `api:7b2829bb:zeroize-asymmetry` — zeroize asymmetry

**Status:** not started  
**Severity:** major  
**Cluster:** exported-surface — seam→audit-security — related: sec:7b2829bb:executor-no-zeroize, sec:35abd54e:env-string-residue  
**Evidence:** `internal/executor/executor.go:38-92` + 2 more  
**Problem:** Sibling exec wrappers diverge on credential lifecycle: terraform.Executor.ZeroizeEnv (terraform.go:L346-L364) and okd.Provisioner.ZeroizeEnv (okd.go:L198-L216) both clear cred-bearing entries from their inner executor.Env, but executor.Executor itself has no ZeroizeEnv method — the canonical exec wrapper lacks the symmetric API its two consumers hand-roll. The duplicated bodies are byte-identical (PROXMOX_VE_PASSWORD/PROXMOX_VE_API_TOKEN allowlist) and parallel the well-formed Zeroize methods on credentials.ProxmoxCredentials.  
**Fix:** Add executor.Executor.ZeroizeEnv() that walks e.Env, blanks any entry whose key is in a secretKeyAllowlist (PROXMOX_VE_PASSWORD, PROXMOX_VE_API_TOKEN at minimum; optionally widen to logutil.RedactHandler's secret-key fragments), then clear()s and nils the slice. Replace the bodies in terraform.go:L352-L364 and okd.go:L204-L216 with calls to the new method (`t.exec.ZeroizeEnv()`, `p.executor.ZeroizeEnv()`). Net LOC: -16 (two body removals replaced by single producer-side method).  
**Effort:** hours

##### `api:25fa1be8:positional-logger` — positional logger

**Status:** not started  
**Severity:** minor  
**Cluster:** option-consistency  
**Evidence:** `internal/distribution/okd/firewall/firewall.go:83-246`  
**Problem:** Package-level firewall.{Configure, RemoveRules, ConfigureOKD, RemoveOKDRules, DetectBackend} all take *slog.Logger as the trailing positional parameter. Every other phase-helper package takes the logger via a constructor option (phase.WithLogger, addon.WithLogger, cluster.WithLogger, executor.WithLogger, terraform.WithLogger, proxmox.WithLogger). The mismatch forces 5 call sites in 4 packages to thread p.Log through the call rather than picking it up via the BasePhase. Five public functions in this package, all positional-logger.  
**Fix:** Promote firewall to a small struct with functional options: `func New(opts ...Option) *Firewall` returning a struct that holds the logger, then methods (*Firewall).Configure(ctx, ports, permanent), .RemoveRules(...), .ConfigureOKD(...). Either keep the package-level free functions for backward compat or convert call sites in destroy/steps.go, postinstall/haproxy.go, cleanup/services.go, setup/steps.go (4 sites) to construct via firewall.New(firewall.WithLogger(p.Log)). Aligns with phase.BasePhaseOption / addon.ManagerOption / executor.Option naming.  
**Effort:** hours

##### `api:262af6e4:dual-option-types` — dual option types

**Status:** not started  
**Severity:** minor  
**Cluster:** option-consistency  
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:85-118`  
**Problem:** cleanup.Phase has TWO option-pattern surfaces glued together: the canonical phase.BasePhaseOption used by New (line 103) and a second package-local cleanup.Option / cleanupConfig used by Execute (lines 89-95, 113). The Execute-time Option only carries a logger, which the BasePhase already has — so the second surface is pure noise. No sibling phase package (setup, install, postinstall, destroy) has a phase-local Option type beyond the inherited phase.BasePhaseOption.  
**Fix:** Drop cleanup.Option, cleanup.WithLogger, cleanup.cleanupConfig. Change Execute signature to `func (p *Phase) Execute(ctx context.Context, opts *Options) error` and have the body use p.Log (set by phase.WithLogger at construction). Update the single caller (okd.go:L131 — `cleanup.New(...).Execute(ctx, cleanupOpts, cleanup.WithLogger(p.logger))`) to pass the logger via cleanup.New(phase.WithLogger(p.logger)).  
**Effort:** hours

##### `api:4c092fce:terraform-mixed-shape` — terraform mixed shape

**Status:** not started  
**Severity:** minor  
**Cluster:** option-consistency — related: api:7b2829bb:exposed-fields-no-callers  
**Evidence:** `internal/infrastructure/terraform/terraform.go:32-59`  
**Problem:** terraform.Executor commits to functional options (WithLogger, WithVerbose, WithVarFile, WithEnv) yet exposes WorkDir, VarFile, Verbose as public fields. proxmox.go reads `p.terraformExec.WorkDir` (proxmox.go:L192) — the cross-package read encodes that the field IS part of the API, but the rest of the surface is shaped as if the struct were opaque. Pick one: either go full opaque (unexport, add WorkDir() method) or commit to options-as-public-struct (drop functional options, configure via field-set). The current half-half shape duplicates intent — VarFile/Verbose are both fields and With* options.  
**Fix:** Unexport WorkDir/VarFile/Verbose (rename workDir/varFile/verbose); add a WorkDir() string getter for the proxmox.go:L192 read site. Drop redundant Verbose-as-both — keep WithVerbose only. Tests inside the package access fields directly (no public API change). Aligns terraform.Executor with executor.Executor (also flagged at api:7b2829bb) and the rest of the New + functional options pattern.  
**Effort:** hours

##### `api:7b2829bb:exposed-fields-no-callers` — exposed fields no callers

**Status:** not started  
**Severity:** minor  
**Cluster:** exported-surface — related: api:7b2829bb:zeroize-asymmetry, api:d5915b0c:exec-env-direct-mutation  
**Evidence:** `internal/executor/executor.go:38-46`  
**Problem:** Executor.Stdout, Executor.Stderr, Executor.WorkDir, Executor.Verbose are exported but no external caller mutates or reads them. Stdout/Stderr default to os.Stdout/os.Stderr in New() and are set there only; WorkDir is set via WithWorkDir; Verbose has no WithVerbose option and is not read anywhere in this package or callers. The fields look like the prefix of an option-struct API but the package committed to functional-options. Either complete the option API (WithStdout/WithStderr/WithVerbose) or unexport the fields.  
**Fix:** Unexport Stdout, Stderr, WorkDir, Verbose (rename to stdout/stderr/workDir/verbose). Add executor.WithStdout(io.Writer), executor.WithStderr(io.Writer) for completeness. Verbose can be removed entirely (it is never read). RunStreamed reads e.Stdout/e.Stderr internally; that becomes e.stdout/e.stderr. No external code imports these names today, so the rename is local.  
**Effort:** hours

##### `api:d5915b0c:exec-env-direct-mutation` — exec env direct mutation

**Status:** not started  
**Severity:** minor  
**Cluster:** package-boundary — related: api:7b2829bb:zeroize-asymmetry  
**Evidence:** `internal/distribution/okd/install/phase.go:166-166`  
**Problem:** SetupKubeconfig appends to p.Exec.Env directly (`p.Exec.Env = append(p.Exec.Env, ...)`) — a public-field mutation that bypasses the executor.WithEnv functional-options API. The Executor exposes its Env field publicly (executor.go:L40) but every other call site uses WithEnv at construction; this is the only post-construction mutation in production code. It defeats any future invariant the Executor wants to enforce on Env (allowlist filtering, length cap, redaction on insert).  
**Fix:** Add executor.Executor.AppendEnv(kvs ...string) (or executor.Executor.SetEnvVar(key, value string)) and have SetupKubeconfig call it instead of append-on-public-field. Then unexport Executor.Env (renaming to env) — the only remaining external readers (terraform.WithEnv(p.Exec.Env), proxmox.WithEnv(p.Exec.Env)) become callers of e.Exec.SnapshotEnv() or a getter. Pairs with api:7b2829bb (ZeroizeEnv) — the same refactor closes both gaps.  
**Effort:** hours

##### `api:0139cb3f:bin-dir-fan-out` — bin dir fan out

**Status:** not started  
**Severity:** suggestion  
**Cluster:** exported-surface  
**Evidence:** `internal/distribution/okd/phase/paths.go:52-97`  
**Problem:** phase.{ResolveBinDir, PreflightBinDir, BinDirOrDefault} is a three-function surface where each consults a different input source. The doc-comment on BinDirOrDefault explicitly names this as scaffolding ('three-function bin-dir-resolution surface; each function consults a different input source'). Defense-in-depth justification is plausible but the API forces every caller to pick one of three nearly-indistinguishable verbs.  
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.  
**Effort:** hours

##### `api:48688e63:ctx-symmetry-no-network` — ctx symmetry no network

**Status:** not started  
**Severity:** suggestion  
**Cluster:** ctx-first  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:103-129`  
**Problem:** proxmox.Provider.Connect / Disconnect take ctx but the local-only implementation never uses it (sshpin.Verify uses ctx; the Disconnect body discards it via underscore). The doc-comment names this explicitly: 'ctx is accepted for symmetry with future network-bound providers; this implementation is local-only.' That IS the scaffolding rule from MEMORY.md §scaffolding — symmetric API for a future network-bound provider lifecycle. Flagging at suggestion to verify the intent is still live.  
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.  
**Effort:** hours

##### `api:c287d5c0:zeroize-no-callers-yet` — zeroize no callers yet

**Status:** not started  
**Severity:** suggestion  
**Cluster:** exported-surface — seam→audit-security — related: api:7b2829bb:zeroize-asymmetry, sec:7b2829bb:executor-no-zeroize  
**Evidence:** `internal/distribution/okd/okd.go:198-216`  
**Problem:** Provisioner.ZeroizeEnv is exported but its only caller is internal/cli/helpers.go (in defer chain). It mirrors terraform.Executor.ZeroizeEnv with byte-identical body. The api-design pressure: this is a credential-lifecycle method that belongs lower in the stack (executor.Executor, the field owner) — see api:7b2829bb. The current location is symmetric-with-terraform but redundant once executor exposes ZeroizeEnv.  
**Fix:** Verify intent (grep roadmap.md, ask owner) — do not delete; per MEMORY.md §scaffolding.  
**Effort:** hours

##### `api:ddf885f4:nil-logger-not-normalized` — nil logger not normalized

**Status:** not started  
**Severity:** suggestion  
**Cluster:** zero-value-usability  
**Evidence:** `internal/addon/manager.go:34-57`  
**Problem:** addon.WithLogger does not normalize nil to NopLogger — it sets m.logger = l verbatim. NewManager (L48-57) post-applies opts and only normalizes nil at construction-end. So the documented 'nil tolerated' contract works, but the option function itself isn't nil-safe in isolation: a future option-application order change could expose the nil to logger calls. cluster.WithLogger and terraform.WithLogger use logutil.OrNop inside the option function so nil-safety is option-local.  
**Fix:** In manager.go:L35-37, change `m.logger = l` to `m.logger = logutil.OrNop(l)`. Aligns with cluster.WithLogger and terraform.WithLogger. Drops the construction-end nil guard at NewManager:L53-55 (unnecessary once each option is nil-safe). Sweep phase.WithLogger (paths.go:L154) for the same pattern — it is also field-direct rather than OrNop-wrapped, but NewBasePhase normalizes at construction-end. Either pattern is OK if applied uniformly across the audited siblings.  
**Effort:** hours

##### `api:e2343d2c:unused-trailing-param` — unused trailing param

**Status:** not started  
**Severity:** suggestion  
**Cluster:** exported-surface  
**Evidence:** `internal/system/systemd.go:31-46`  
**Problem:** system.ManageService(ctx, action, serviceName, _ string) takes a fourth string parameter that is named `_` and has no documented purpose. The signature is exported so removing the param is a breaking change, but the param is genuinely unused (the function body never reads it). Either name and document it (does it carry a unit-file path? a target?) or remove it.  
**Fix:** Drop the unused parameter; sweep callers (RHS of `ManageService(ctx, ServiceStop, "haproxy", ...)` becomes 3-arg). If a future use case (description? target?) is anticipated, leave a one-line comment naming the intent and the issue/roadmap entry that drives the addition.  
**Effort:** hours

##### `api:fde34e0c:k8sclient-pkg-stutter` — k8sclient pkg stutter

**Status:** not started  
**Severity:** suggestion  
**Cluster:** exported-surface  
**Evidence:** `internal/cluster/k8s.go:20-88`  
**Problem:** cluster.K8sClient and cluster.NewK8sClient — `cluster.K8sClient` stutters (the package is named cluster; the typename should be `Client`). Go style guide §Packages explicitly flags this: bufio.Reader, not bufio.BufReader. Constructor NewK8sClient is the sole exported constructor in the package; no other Client variants justify the qualifier.  
**Fix:** Rename K8sClient → Client and NewK8sClient → New. cluster.Client / cluster.New(...) reads naturally and aligns with executor.Executor / executor.New, terraform.Executor / terraform.New, proxmox.Provider / proxmox.New. Five call sites (install/, postinstall/, cli/) plus one test file. revive's var-naming rule is enabled and would catch this in a fresh repo; suppression is implicit because the package was named first.  
**Effort:** hours

#### audit-cli-ux

##### `ux:e7db1220:releases-validate-flag-error-not-usageerror` — releases validate flag error not usageerror

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/ux-e7db1220-releases-usageerror  
**Severity:** major  
**Cluster:** exit-codes — seam→audit-errors — related: ux:fd2125dd:args-validator-not-usageerror  
**Evidence:** `internal/cli/releases.go:157-173` + 2 more  
**Problem:** validateChannel and validateFormat both return plain fmt.Errorf on rejected --channel / --output values. Reaches exitCodeFor → 1, not 64 (EX_USAGE). Same shape as the addon-install Args bug. These are flag-value-shape errors and should map to the documented usage-class code so scripted callers can branch consistently. Pattern: ≥3 sites (releases.go:L157, L166, plus addon Args:L62-L73). Umbrella per-pattern finding referencing the per-site addon finding.  
**Fix:** Wrap both validators' returns in &errtypes.UsageError{Msg: ..., Err: ...}. Single change in releases.go validates eight call sites at once. Updates the taxonomy alignment for --channel and --output across the read-only command surface.  
**Effort:** hours

##### `ux:fd2125dd:args-validator-not-usageerror` — args validator not usageerror

**Status:** not started  
**Severity:** major  
**Cluster:** exit-codes — seam→audit-errors  
**Evidence:** `internal/cli/addon.go:62-73`  
**Problem:** addonInstallCmd.Args returns plain fmt.Errorf for missing-or-conflicting positional args. The error reaches exitCodeFor without satisfying errors.As(*errtypes.UsageError), so the process exits 1 (generic) instead of 64 (EX_USAGE). The published taxonomy in docs/cli/exit-codes.md says 64 covers cobra flag-parse failures; positional-arg failures are the natural sibling and currently violate the taxonomy.  
**Fix:** Wrap each return in &errtypes.UsageError{Msg: ..., Err: ...} so exitCodeFor() resolves to 64 (EX_USAGE), matching the documented taxonomy and the SetFlagErrorFunc pattern in root.go:256-259. Apply the same pattern to any other Args validator that returns a raw fmt.Errorf for arg-shape violations.  
**Effort:** hours

##### `ux:024a2c32:json-schema-status-incomplete` — json schema status incomplete

**Status:** not started  
**Severity:** minor  
**Cluster:** json-stability — seam→audit-documentation  
**Evidence:** `docs/cli/json-schema.md:12-40`  
**Problem:** docs/cli/json-schema.md describes `okdctl status --output=json` with four top-level fields (api_reachable, nodes, degraded_operators, addons). The Go type emitted is okd.ClusterStatus (internal/distribution/okd/types.go) which carries six more documented fields: phase, version, api_server_url, console_url, conditions, message. Several are tagged `omitempty` and so may genuinely be absent on a healthy cluster, but `phase` is always emitted (it's a non-omitempty enum) and is missing from the schema doc.  
**Fix:** Update docs/cli/json-schema.md `okdctl status --output=json` to include `phase` (always present) and the omitempty-tagged fields (version, api_server_url, console_url, conditions, message) with a 'present when non-empty' note. Per seam #13 this is single-command drift, not pattern-wide; the markdown owns it (audit-documentation). Logged here only because it touches the published JSON contract this skill declares stable.  
**Effort:** hours

##### `ux:073d24ed:deploy-example-uses-config-not-output-file` — deploy example uses config not output file

**Status:** not started  
**Severity:** minor  
**Cluster:** flag-conventions — seam→audit-documentation  
**Evidence:** `internal/cli/deploy.go:38-40`  
**Problem:** deploy Example shows `okdctl deploy --yes --config my-cluster.yaml`. --config is the read-side persistent flag (root); --output-file is the deploy-specific write-side flag (defaults to okdctl.yaml). On --yes (non-interactive) deploy never enters the wizard and writes nothing through --output-file, so the example happens to work — but it conflates the two halves of the convention codified in CLAUDE.md §architecture (--output-file is the file destination). A user who copies this idiom thinks --config controls write destination.  
**Fix:** Either drop the --config flag from the second example (default config file is okdctl.yaml — implicit reuse) or split into two: one showing read (`okdctl deploy --config my-cluster.yaml`) and one showing write (`okdctl deploy --yes --output-file my-cluster.yaml`). Regenerate docs/cli/ via `make docs`.  
**Effort:** hours

##### `ux:4583b75b:config-describe-missing-long` — config describe missing long

**Status:** not started  
**Severity:** minor  
**Cluster:** help-text  
**Evidence:** `internal/cli/config.go:19-22` + 1 more  
**Problem:** configCmd is a parent verb (config show, config validate) and registers only a Short. The sibling parent groups in this repo (addonCmd L26-L30, releasesCmd L33-L37) all carry a Long that orients the user before they run a subcommand. describeCmd in status.go has the same omission. With no Long the help screen lists subcommands without context — fine for a power user, jarring for a first-timer.  
**Fix:** Add a one-sentence Long to both configCmd and describeCmd that names the subcommands and points at the typical entry point. Mirrors the pattern in addonCmd and releasesCmd. Regenerate docs/cli/ via `make docs`.  
**Effort:** hours

##### `ux:8d8faa80:completion-bypasses-outorstdout` — completion bypasses outorstdout

**Status:** not started  
**Severity:** minor  
**Cluster:** streams  
**Evidence:** `internal/cli/completion.go:36-47`  
**Problem:** runCompletion writes the generated shell script directly to os.Stdout via cmd.Root().GenBashCompletionV2(os.Stdout, true) instead of cmd.OutOrStdout(). The siblings (status, releases, addon list, addon verify, doctor) all funnel JSON/text through cmd.OutOrStdout(); completion is the lone exception. Tests cannot capture the output without process-level redirection, and the mismatch is a future trap when a maintainer adds output piping behaviour.  
**Fix:** Pass cmd.OutOrStdout() (or cmd.Root().OutOrStdout() since cobra reuses the root writer) to all three Gen*Completion calls. Test-side wins: a captured-output test can assert on the first ~20 bytes of each shell's preamble.  
**Effort:** hours

##### `ux:daf5bee9:kubeconfig-status-msg-bypasses-tui` — kubeconfig status msg bypasses tui

**Status:** not started  
**Severity:** minor  
**Cluster:** streams — seam→audit-observability  
**Evidence:** `internal/cli/kubeconfig.go:72-72`  
**Problem:** After AtomicWrite or merge succeeds, kubeconfig prints a free-form status line via fmt.Fprintf(os.Stderr, ...) instead of tui.Info. The repo's slog → RedactHandler chain is bypassed; future additions of attrs to that line silently lose redaction. Stream choice (stderr) is correct, but the sink should be tui so structured fields are honoured.  
**Fix:** Replace fmt.Fprintf(os.Stderr, ...) with tui.Info("kubeconfig written", tui.LF("path", kubeconfigOutput)) and tui.Info("kubeconfig merged", tui.LF("path", dest)). Matches CLAUDE.md §credentials-and-secrets directive that all log sinks pass through RedactHandler so future fields cannot leak.  
**Effort:** hours

##### `ux:fd2125dd:install-use-bracket-syntax` — install use bracket syntax

**Status:** not started  
**Severity:** minor  
**Cluster:** verb-noun  
**Evidence:** `internal/cli/addon.go:44-44`  
**Problem:** addonInstallCmd.Use is `install [name | --all]`. Cobra's Use field is a positional-arg signature, not a help-string; standard cobra renders flags via the Options block, not in the synopsis. The pipe-bar `|` is non-standard cobra and reads as if --all is itself a positional. The Args validator already enforces the mutual exclusion at runtime; the synopsis should be `install [name]` with --all surfacing in Long and Example.  
**Fix:** Change Use to `install [name]`. The Long block already explains the --all flow (L51-L61); the Example block already shows both shapes. Matches releases show `Use: "show <version>"` and describe node `Use: "node <name>"` — flags never appear in cobra Use across the repo.  
**Effort:** hours

##### `ux:08c49fc4:keep-haproxy-no-shorthand-asymmetry` — keep haproxy no shorthand asymmetry

**Status:** not started  
**Severity:** suggestion  
**Cluster:** flag-conventions  
**Evidence:** `internal/cli/update_ingress.go:45-47`  
**Problem:** update-ingress registers --yes/-y, --keep-haproxy, --dry-run. Across destructive siblings (deploy, destroy, cleanup) the trio --yes/--dry-run is universal but the boolean tail flag (--keep-isos, --skip-terraform, --keep-haproxy, --skip-must-gather) never carries a shorthand. Consistent enough as policy; flag this only as a design observation so the convention is explicit when new boolean tails are added.  
**Fix:** Codify the existing convention in CLAUDE.md §architecture-notes (Flag naming convention block): only the universal trio --yes/-y, --quiet/-q, --verbose/-v, --output/-o, --config/-c gets a shorthand. Per-command boolean tails stay long-form. No code change.  
**Effort:** hours

##### `ux:08ec0042:flag-output-name-collision-risk` — flag output name collision risk

**Status:** not started  
**Severity:** suggestion  
**Cluster:** flag-conventions  
**Evidence:** `internal/cli/flags.go:7-10`  
**Problem:** The constant `flagOutput = "output-file"` is used by deploy (--output-file = config dest) and debug-bundle (--output-file = bundle dest). Eight other commands register --output (not --output-file) for format selection via StringVarP. The repo convention codified in CLAUDE.md §architecture is correct, but a maintainer skimming `flagOutput` reads it as `--output` and will land on the wrong flag. The constant name aliases an existing flag-spelling; rename or split.  
**Fix:** Rename the constant to `flagOutputFile` (paired with `flagOutputFormat = "output"` if you want the format-side codified too). Reads correctly at registration sites and removes the conceptual collision with cobra's StringVarP "output" literal.  
**Effort:** hours

##### `ux:0d318f5c:log-format-tty-default-help-noise` — log format tty default help noise

**Status:** not started  
**Severity:** suggestion  
**Cluster:** streams  
**Evidence:** `internal/cli/logging.go:73-78`  
**Problem:** configureLogging auto-switches --log-format to json when stderr is piped, but cobra still renders `(default "text")` in --help output. The behaviour itself is sound (matches 12-factor CLI / clig.dev); the help string lies. The mitigating prose in the StringVar help text at root.go:L248 spells out the auto-switch, but the cobra-default paren remains the strongest signal a hurried reader sees.  
**Fix:** Two options: (a) leave as-is — the explanatory parenthetical above the cobra default already discloses the auto-switch; (b) suppress the cobra-default render with `Flag.DefValue = ""` after registration, replacing the truth-claim with a flag-help hint that names both behaviours. (a) is acceptable; (b) is more honest. No security or correctness impact either way.  
**Effort:** hours

##### `ux:8154ab0f:doctor-pull-secret-config-skew-warns` — doctor pull secret config skew warns

**Status:** not started  
**Severity:** suggestion  
**Cluster:** exit-codes — seam→audit-state-and-recovery  
**Evidence:** `internal/cli/doctor.go:428-476`  
**Problem:** checkPullSecret returns sevWarn when no config file exists ('no config yet at ...; run okdctl deploy') but sevFail when the config exists with an empty pull_secret. doctor's documented contract is `Exit 0 if no [fail] results`. A first-time user runs `okdctl doctor` before `deploy` and sees mostly green plus one warn for pull-secret. They run `deploy`, which then re-validates and rejects with exit 65 (EX_DATAERR) on the same field. The taxonomy holds, but doctor's preflight value drops: it should fail loudly when an essential value is unset, not pass-with-warning.  
**Fix:** Decision needed, not a code change yet. Either (a) keep the current shape (warn-then-fail) and document doctor as orientation-only, or (b) escalate the no-config branch to sevFail so doctor's exit code is honest. Option (b) breaks `okdctl doctor && okdctl deploy` chained-on-success scripts. Recommend (a) plus a doc note in docs/doctor-checks.md naming this as the intended split.  
**Effort:** hours

##### `ux:aa84670c:version-cmd-uses-run-not-rune` — version cmd uses run not rune

**Status:** not started  
**Severity:** suggestion  
**Cluster:** verb-noun  
**Evidence:** `internal/cli/root.go:236-243`  
**Problem:** versionCmd uses Run rather than RunE. Every other leaf command in the tree uses RunE so cobra propagates the error to ExecuteContext where exitCodeFor maps it. fmt.Fprintf return values are dropped (Run signature has no err). A failed write to a closed pipe (e.g. `okdctl version | head -c 0`) silently exits 0 instead of 141/EPIPE-shaped 1.  
**Fix:** Convert Run to RunE returning the fmt.Fprintf error. Aligns with deployCmd, destroyCmd, etc. Net effect: a broken-pipe write becomes a tracked error rather than a silent zero exit.  
**Effort:** hours

#### audit-observability

##### `obs:5013fea6:stderr-attr-leaks-subprocess-output` — stderr attr leaks subprocess output

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/obs-5013fea6-stderr-redact  
**Severity:** major  
**Cluster:** redaction-sink — seam→audit-errors — related: err:7b2829bb:exit-error-no-redact  
**Evidence:** `internal/distribution/okd/setup/release_extract.go:121-123` + 2 more  
**Problem:** Three log sites pass raw subprocess Stderr under the benign attr key 'stderr', which is not in logutil.secretKeyFragments (password/token/secret/api_key/apikey). The most exposed call is `oc adm release extract` whose stderr can carry --registry-config paths, partial auth diagnostics, or pull-secret excerpts on failure. RedactHandler cannot rewrite a generic 'stderr' key, and the value is a plain string — Redacted() is never consulted. This is the observability-side mirror of audit-errors err:7b2829bb (executor.ExitError lacking Redacted()): even when the error type is fixed, sites that pull `result.Stderr` out of *executor.Result and pass it as a separate slog attr bypass any type-level fix.  
**Fix:** Two options. (a) Stop logging raw subprocess stderr at the sink — let the typed error handle redaction; replace 'stderr', msg with 'err', execErr after err:7b2829bb adds Redacted() to executor.ExitError. (b) If a structured stderr attr is genuinely useful, route it through a redactable wrapper type (e.g. a new system.RedactableStderr string with Redacted() any returning a fixed-length head/tail). Net 0 LOC for option (a). Same fragment list lives in logutil/redact.go:23 — adding 'stderr' there would over-redact legitimate kubectl/terraform stderr that contains no secrets, so prefer not to widen secretKeyFragments.  
**Effort:** hours

##### `obs:97cb8adf:fmt-sprintf-message-pattern` — fmt sprintf message pattern

**Status:** not started  
**Severity:** minor  
**Cluster:** redaction-sink  
**Evidence:** `internal/system/exec.go:117-135`  
**Problem:** system.WaitFor builds the slog message via fmt.Sprintf and passes the rendered string as the message arg to logger.Info. Both 'prefix' and 'description' come from the caller and may grow to include cluster/path/host substrings — CLAUDE.md §credentials forbids X(fmt.Sprintf(...)) precisely because RedactHandler can only inspect attr keys/values, not pre-rendered message text. Today's call sites pass static prefix='ingress' and description='router-X termination'; tomorrow a description like fmt.Sprintf('VIP %s health', creds.Endpoint) silently leaks. The c07157e migration swept ~85 sites but missed this canonical helper.  
**Fix:** Move prefix and description to slog attrs and keep the message static. Replace `waitMsg := fmt.Sprintf("%s: waiting for %s...", prefix, description); logger.Info(waitMsg)` with `logger.Info("waiting", "for", description, "prefix", prefix)`. Same shape for readyMsg → `logger.Info("ready", "for", description, "prefix", prefix, "polls", polls, "elapsed", elapsed.Round(time.Second))`. Net -2 LOC. Consider also folding the L164 Debug similarly. Verify the tui formatter renders 'prefix' as a leading group.  
**Effort:** hours

##### `obs:beabab0c:phase-attr-missing-on-setup` — phase attr missing on setup

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/obs-beabab0c-setup-phase-attr  
**Severity:** minor  
**Cluster:** field-stability  
**Evidence:** `internal/distribution/okd/setup/phase.go:99-107`  
**Problem:** Setup Phase.New does not call `bp.Log = bp.Log.With("phase", "setup")` even though install/postinstall/destroy/cleanup all do. Every setup-phase log line therefore lacks the 'phase' attr while the other four phases emit it consistently — operators filtering JSON logs by phase=setup get an empty result. Field-stability regression: the same conceptual key is present in 4/5 phases.  
**Fix:** Add `bp.Log = bp.Log.With("phase", "setup")` between the NewBasePhase call (L100) and the platform detection at L101 — pattern matches install/phase.go:L91, postinstall/phase.go:L68, destroy/phase.go:L62, cleanup/cleanup.go:L105. Net +1 LOC.  
**Effort:** hours

##### `obs:eb479d86:sprintf-attr-value` — sprintf attr value

**Status:** not started  
**Severity:** minor  
**Cluster:** field-stability  
**Evidence:** `internal/distribution/okd/setup/upload.go:138-138`  
**Problem:** `p.Log.Info("iso: uploading", "count", ..., "size_mb", fmt.Sprintf("%.1f", totalSizeMB), ...)` pre-renders a float to a string before the slog handler sees it. The c07157e migration swept message-arg fmt.Sprintf but missed this attr-value form. JSON output emits `"size_mb": "123.4"` (string) instead of `"size_mb": 123.4` (number), which breaks downstream tooling that types the field as a number and forces consumers to re-parse.  
**Fix:** Drop the Sprintf wrapper and pass the float directly: `"size_mb", totalSizeMB`. Slog's JSON handler emits float64 with full precision; if the precision-1 rendering is load-bearing for the text formatter, push the rounding down by one level (compute roundedMB := math.Round(totalSizeMB*10)/10 and pass that float). Net 0 LOC.  
**Effort:** hours

##### `obs:19a715fd:instructional-logs-via-info` — instructional logs via info

**Status:** not started  
**Severity:** suggestion  
**Cluster:** level-discipline — seam→audit-cli-ux  
**Evidence:** `internal/addon/catalog/secretstore/secretstore.go:122-152`  
**Problem:** Two installPrereqCheck branches (1password, vault, bitwarden) emit setup instructions via env.Logger.Info as 7-line numbered procedures. Logs are an event stream, not a UX channel; six successive Info lines per missing-secret-file path read as if six events occurred. The instruction body also string-concatenates path components into the message ('echo -n YOUR_TOKEN > ' + dir/...) — same anti-pattern as the c07157e fmt.Sprintf migration in attr-vs-message form.  
**Fix:** Either (a) collapse to one Warn that points at docs/addons/secretstore.md and write the full procedure there, OR (b) move the procedure to a non-log writer (env.Out / cobra.OutOrStdout) so it does not pollute JSON-formatted logs. Existing 'echo -n YOUR_TOKEN > ' + path string concat should become 'cmd', filepath.Join(...) attrs at minimum. Net ~-12 LOC if doc-link path chosen.  
**Effort:** hours

##### `obs:48688e63:proxmox-probe-failure-as-info` — proxmox probe failure as info

**Status:** not started  
**Severity:** suggestion  
**Cluster:** level-discipline  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:382-397`  
**Problem:** probeVMEnumeration logs three failure-or-fallback paths at Info: 'pvesh probe skipped' (L382), 'pvesh probe payload unparseable' (L389), 'vm not yet enumerable, install phase will retry' (L397). The first two are best-effort fallbacks (the function intentionally treats unreachable/parse-fail as 'do not suppress per-VM logs') — those belong at Debug. The third is genuine retry-pending state and is correct at Info. Today the function is called once per provision so the spam is bounded; if it ever moves into a poll loop the chatter compounds.  
**Fix:** Drop both probe-fallback Info calls to Debug. L397 ('vm not yet enumerable, install phase will retry') stays at Info — it announces a deferred operation. Net 0 LOC.  
**Effort:** hours

##### `obs:632c9087:rollback-pair-not-spanned` — rollback pair not spanned

**Status:** not started  
**Severity:** suggestion  
**Cluster:** span-retry-boundary  
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:444-573`  
**Problem:** convertToLoadBalancer (L425+) is a destructive multi-step op (delete IC -> wait for router gone -> create replacement -> wait for new router up). The success path logs span boundaries ('converting ...', 'converted successfully'). The rollback path (attemptRollback at L560-L574) logs only the leaf success/failure outcomes — it does NOT log the entry into rollback. An operator following a JSON log stream sees 'failed to create replacement, attempting rollback' (L444) followed (much later) by 'rollback succeeded'. The intermediate rollback-build, rollback-apply moments are silent.  
**Fix:** Add `p.Log.Info("update-ingress: rollback: starting", "name", ic.Name)` at the top of attemptRollback (L560). The existing trailing logs serve as the end-of-span. Symmetrically, the convertToLoadBalancer L437 'waiting for router deployment to terminate' span has no end log on success — add one. Net +2 LOC.  
**Effort:** hours

##### `obs:660d83a5:setrunid-not-symmetric-with-stderrslog` — setrunid not symmetric with stderrslog

**Status:** not started  
**Severity:** suggestion  
**Cluster:** handler-setup  
**Evidence:** `internal/tui/logger.go:174-180`  
**Problem:** SetRunID rebinds stdoutLogger and stderrLogger to a new charmlog Logger via .With('run_id', id), then rebuilds stderrSlog via buildStderrSlog so the slog wrapper picks up the new charmlog binding. SimpleLogger() (L114, exposed publicly to terraform/proxmox/okd/addon constructors) wraps the *current* stderrLogger value at call time, so any *slog.Logger snapshotted into a Provider before SetRunID will permanently miss run_id. The only call site of SimpleLogger that runs before SetRunID is cli/root.go::Execute L78 (slog.SetDefault); subcommand RunE happens AFTER SetRunID at L87 so the deploy/destroy paths are fine. The risk window is the slog.Default() set in Execute — any third-party library or background goroutine that captures slog.Default() before SetRunID's L179 buildStderrSlog rebuild loses run_id permanently.  
**Fix:** Either (a) move the slog.SetDefault call inside SetRunID itself so the default logger always reflects the latest run_id, OR (b) restructure cli.Execute to call SetRunID before slog.SetDefault. Simplest is (a): in SetRunID, after stderrSlog.Store(buildStderrSlog()), call slog.SetDefault(slog.New(NewRedactHandler(...))) using the same wrapper. Net +2 LOC. Document the snapshot semantic in the SimpleLogger doc comment.  
**Effort:** hours

##### `obs:9d79b841:double-log-iso-download` — double log iso download

**Status:** not started  
**Severity:** suggestion  
**Cluster:** span-retry-boundary  
**Evidence:** `internal/distribution/okd/setup/coreos.go:277-278`  
**Problem:** Two consecutive Info records for one event: 'coreos: downloading iso' with version, then 'coreos: iso url' with url. Reads as one logical span ('I am starting download X from URL Y') split across two lines, doubling the line count for an event that has no intermediate state to surface. The download itself logs progress via download.WithLogger(p.Log) so this is purely the entry record.  
**Fix:** Fold to one record: `p.Log.Info("coreos: downloading iso", "version", info.Version, "url", info.ISOUrl)`. Net -1 LOC.  
**Effort:** hours

#### audit-modernization

##### `mod:15ba17da:use-slices-contains` — use slices contains

**Status:** not started  
**Severity:** minor  
**Cluster:** slices-maps — related: mod:262af6e4:use-slices-contains  
**Evidence:** `internal/distribution/okd/destroy/steps.go:60-69`  
**Problem:** destroyTracker.terraformFailed iterates t.failures looking for a literal string match — slices.Contains expresses the intent in one line. Identical pattern to the cleanup.Kind.IsValid case (mod:262af6e4); both reflect the same pre-1.21 idiom not yet migrated.  
**Fix:** Replace the inner loop with `return slices.Contains(t.failures, "terraform destroy")`. Keeps the lock take/defer wrapping unchanged. Net delete: ~5 LOC.  
**Effort:** hours

##### `mod:262af6e4:use-slices-contains` — use slices contains

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/mod-262af6e4-cleanup-slices-contains  
**Severity:** minor  
**Cluster:** slices-maps  
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:52-60`  
**Problem:** Kind.IsValid hand-rolls a contains loop over ValidKinds() — the textbook slices.Contains use case. The repo already uses slices.Contains for the equivalent check in internal/cli/elevation.go::requiresRoot (rootRequiredCmds) and internal/config/validators.go::isValidDistribution / isValidProvider, so this site is the inconsistent outlier.  
**Fix:** Collapse the body to `return slices.Contains(ValidKinds(), k)`. ValidKinds returns []Kind which slices.Contains supports natively (Kind is comparable). Net delete: ~6 LOC.  
**Effort:** hours

##### `mod:ddf885f4:use-slices-contains` — use slices contains

**Status:** not started  
**Severity:** minor  
**Cluster:** slices-maps — related: mod:262af6e4:use-slices-contains  
**Evidence:** `internal/addon/manager.go:267-275`  
**Problem:** Manager.dependsOn does a hand-rolled contains check on a.Info().Dependencies before recursing — slices.Contains expresses the early-return cleanly. The same package already imports slices for the rollback path (L183 slices.Backward), so the import is free.  
**Fix:** Hoist the equality short-circuit out: `if slices.Contains(a.Info().Dependencies, target) { return true }`. Then iterate only to recurse: `for _, dep := range a.Info().Dependencies { if m.dependsOn(dep, target, visited) { return true } }`. Approximate even LOC, clearer intent.  
**Effort:** hours

##### `mod:5e892064:strings-lines-checksum` — strings lines checksum

**Status:** not started  
**Severity:** suggestion  
**Cluster:** slices-maps  
**Evidence:** `internal/download/checksum.go:83-84`  
**Problem:** checksum.go reads the response body and immediately splits it on '\n' to walk lines. strings.Lines yields one line at a time without the slice allocation; on a long checksums file (multi-arch releases) this trims one allocation proportional to body size.  
**Fix:** Replace with `for line := range strings.Lines(string(body)) { line = strings.TrimSpace(line); ... }`. TrimSpace already handles the trailing newline that strings.Lines preserves on each yielded line. Net -1 LOC.  
**Effort:** hours

##### `mod:6b533f2d:slices-collect-projection` — slices collect projection

**Status:** not started  
**Severity:** suggestion  
**Cluster:** slices-maps  
**Evidence:** `internal/cluster/k8s_csrs.go:63-66`  
**Problem:** ApprovePendingCSRs builds a names slice via index assignment in a counted-style loop. The same projection shape recurs in update_ingress.go::handleHostNetworkConversion (L292-295) and cleanup.go::KindStrings (L43-50). The repo could land a tiny mapSlice helper in internal/sliceutil and migrate all three, OR keep the make+loop form (already idiomatic Go).  
**Fix:** Optional. The for-i form is already clear. If a third+fourth callsite appear, land a small `mapSlice[T,U](in []T, fn func(T) U) []U` helper. Don't land in isolation — there's no Go 1.x-specific reason to migrate.  
**Effort:** hours

##### `mod:8ea706f6:strings-lines-fingerprint` — strings lines fingerprint

**Status:** not started  
**Severity:** suggestion  
**Cluster:** slices-maps — related: mod:8ea706f6:strings-lines-version  
**Evidence:** `internal/distribution/okd/setup/tools.go:346-362`  
**Problem:** verifyHashiCorpGPGFingerprint uses `for _, line := range strings.Split(string(out), "\n")` to walk gpg --with-colons output. strings.Lines avoids the slice allocation and matches the repo norm. Same file already does this kind of split twice (L260 and here); both should land together.  
**Fix:** Change to `for line := range strings.Lines(string(out)) { line = strings.TrimRight(line, "\n"); ... }`. Fingerprint comparison is unaffected — it operates on the trimmed field-9, not on the whole line.  
**Effort:** hours

##### `mod:8ea706f6:strings-lines-version` — strings lines version

**Status:** not started  
**Severity:** suggestion  
**Cluster:** slices-maps  
**Evidence:** `internal/distribution/okd/setup/tools.go:260-264`  
**Problem:** getToolVersion calls `strings.Split(strings.TrimSpace(string(output)), "\n")` only to read the first line. With strings.Lines the read becomes a single-iteration loop with no allocation; the surrounding `if len(lines) > 0` guard goes away because the iterator yields zero times for an empty TrimSpace result.  
**Fix:** Replace with `for line := range strings.Lines(string(output)) { return strings.TrimSpace(line) }; return "unknown"`. No slice materialisation, ~3 LOC removed.  
**Effort:** hours

##### `mod:983f67f0:strings-lines-mixed` — strings lines mixed

**Status:** not started  
**Severity:** suggestion  
**Cluster:** slices-maps  
**Evidence:** `internal/tui/layouts.go:28-113`  
**Problem:** layouts.go is internally inconsistent: maxLineWidth (L30) uses `strings.Lines` (Go 1.24 iterator) while boxedSectionCore (L44) uses `strings.Split(content, "\n")` over the same content variable. The Split form materialises a slice; on tight terminal redraws the iterator form avoids the allocation and aligns with the repo's prevailing style (netutil/iface.go, dns.go, install/flux.go, platform/platform.go all use strings.Lines).  
**Fix:** Migrate L44 to the iterator: `for line := range strings.Lines(content) { ... }`. Or, if a slice is needed for the contentRows pre-size, `lines := slices.Collect(strings.Lines(content))`. Either way removes the lone Split site so the file uses one idiom for line iteration.  
**Effort:** hours

##### `mod:b8687976:slices-clip-dedup` — slices clip dedup

**Status:** not started  
**Severity:** suggestion  
**Cluster:** slices-maps  
**Evidence:** `internal/distribution/okd/releases/fetcher.go:101-113`  
**Problem:** deduplicateReleases hand-rolls a seen-set + filtered-result loop. With sortAndClassifySeries fully reordering downstream, a sort+slices.CompactFunc shape would dedup with smaller surface area. Flagging as low-confidence because preserving input order is the seen-set form's distinguishing property — only valid if downstream is provably order-insensitive.  
**Fix:** Optional. The seen-set form preserves input order, which slices.CompactFunc does not without a prior sort — so the swap is NOT semantics-preserving for callers that depend on iteration order downstream. Audit the callers (sortAndClassifySeries fully reorders so it appears safe). If yes: `slices.SortFunc(releases, ...); return slices.CompactFunc(releases, func(a, b githubRelease) bool { return a.TagName == b.TagName })`. Risk medium because of the order-dependency check.  
**Effort:** hours

##### `mod:c19ee328:slices-containsfunc-allexist` — slices containsfunc allexist

**Status:** not started  
**Severity:** suggestion  
**Cluster:** slices-maps  
**Evidence:** `internal/distribution/okd/setup/steps.go:85-93`  
**Problem:** AlreadyDone for the install-binaries step iterates [openshift-install, oc, kubectl] and returns false on the first missing entry. slices.ContainsFunc with negation expresses 'all-of' cleanly and matches the repo's any-of/all-of style elsewhere.  
**Fix:** Rewrite as `bins := []string{"openshift-install", "oc", "kubectl"}; missing := slices.ContainsFunc(bins, func(b string) bool { return !system.FileExists(filepath.Join(binDir, b)) }); return !missing, nil`. Same short-circuit semantics. Net -3 LOC.  
**Effort:** hours

##### `mod:c19ee328:slices-containsfunc-ignition` — slices containsfunc ignition

**Status:** not started  
**Severity:** suggestion  
**Cluster:** slices-maps — related: mod:c19ee328:slices-containsfunc-allexist  
**Evidence:** `internal/distribution/okd/setup/steps.go:186-192`  
**Problem:** AlreadyDone for StepGenerateIgnition repeats the same all-files-present pattern: iterate ignitionFilenames, return false on the first missing one. Twin to mod:c19ee328:slices-containsfunc-allexist; both should land together.  
**Fix:** `return !slices.ContainsFunc(ignitionFilenames, func(f string) bool { return !system.FileExists(filepath.Join(clusterDir, f)) }), nil`. Same semantics; matches the prevailing repo idiom.  
**Effort:** hours

##### `mod:c87d0b1f:log-fatal-in-cli-entrypoint` — log fatal in cli entrypoint

**Status:** not started  
**Severity:** suggestion  
**Cluster:** slog-over-3p  
**Evidence:** `cmd/okdctl-gen-docs/main.go:18-35`  
**Problem:** cmd/okdctl-gen-docs uses `log.Fatalf` from the standard `log` package. The repo standard is log/slog (CLAUDE.md notes the canonical helpers in internal/logutil; the SKILL flags 'log.Printf, fmt.Println as log' under slog-over-3p). Build-tagged `//go:build docs` so it ships only via `go run -tags docs`, not in the binary — but the inconsistency stands.  
**Fix:** For a tiny build-tagged tool the cost/benefit is borderline. Two options: (a) keep `log.Fatalf` and add a one-line WHY comment that this is the docs-only generator and slog plumbing is not desired; (b) switch to `fmt.Fprintln(os.Stderr, ...); os.Exit(1)` and drop the `log` import entirely. Do NOT pull in the project's slog setup just for this — overkill for a 35-LOC tool.  
**Effort:** hours

##### `mod:eb479d86:use-slices-containsfunc` — use slices containsfunc

**Status:** not started  
**Severity:** suggestion  
**Cluster:** slices-maps  
**Evidence:** `internal/distribution/okd/setup/upload.go:171-176`  
**Problem:** isoUploadAlreadyDone iterates isoFiles short-circuiting on the first 'needs upload' result. slices.ContainsFunc expresses the same any-true-then-stop semantics in one line and matches the repo's prevailing pattern (verify.go, status.go, executor.go all use ContainsFunc for any-of checks).  
**Fix:** Collapse to `if slices.ContainsFunc(isoFiles, func(f string) bool { return isoUploadNeeded(ctx, p.Exec, host, knownHostsPath, remotePath, f) }) { return false, nil }; return true, nil`. ContainsFunc preserves the short-circuit ordering of the SSH probes.  
**Effort:** hours

#### audit-code-smells

##### `smell:26a430ee:requires-root-annotation-key` — requires root annotation key

**Status:** not started  
**Severity:** minor  
**Cluster:** magic-strings  
**Evidence:** `internal/cli/elevation.go:51-51` + 2 more  
**Problem:** `annotationValueTrue` is centralized but the cobra annotation key `"requiresRoot"` is repeated as a bare string at 3 sites — the read site (elevation.go) and 2 write sites (addon.go). A typo on either side would silently disable the privilege gate, the very class the constant was introduced to prevent.  
**Fix:** Add `const annotationKeyRequiresRoot = "requiresRoot"` to elevation.go alongside annotationValueTrue. Replace the three string literals with the constant. Reasoning matches the existing flagDryRun / flagOutput pattern in cli/flags.go (a typo on either flag-set side returns zero value at runtime).  
**Effort:** hours

##### `smell:632c9087:ingress-strategy-default-shadow` — ingress strategy default shadow

**Status:** not started  
**Severity:** minor  
**Cluster:** magic-strings  
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:341-347`  
**Problem:** `IngressStrategy` is typed but only two values are declared (`HostNetwork`, `LoadBalancerService`). The OKD operator emits at least three more (`NodePortService`, `Private`, `Empty`) and discoverIngressControllers shoves any unknown spec.type back into the typed value. Today the comparison `ic.Strategy == strategyHostNetwork` works because of the empty/null branch, but a NodePort-strategy controller would silently bypass conversion AND bypass the LB branch — neither path matches.  
**Fix:** Either widen the enum (add NodePortService / Private constants) and have collectLBEntries / handleHostNetworkConversion explicitly route them, or narrow the typed assignment by parsing into IngressStrategy via a `parseIngressStrategy(string) (IngressStrategy, ok bool)` helper that returns false for anything outside the closed set. A controller with an unrecognised strategy should produce a typed warning, not silently flow through HostNetwork-branch logic.  
**Effort:** hours

##### `smell:b5a79fda:deploy-phase-stringly-typed` — deploy phase stringly typed

**Status:** not started  
**Severity:** minor  
**Cluster:** magic-strings  
**Evidence:** `internal/cli/deploystate.go:75-86` + 1 more  
**Problem:** The deploy-phase marker uses three bare strings ("prepare", "install", "configure") written by helpers.go and read by deploystate.go's switch. A producer/consumer string contract with no typed enum is exactly the shape the repo already replaced for cleanup.Kind, summary.stepDisplayStatus, debug_bundle.bundleStatus, and platform.Family.  
**Fix:** Introduce `type deployPhase string` with `phasePrepare/phaseInstall/phaseConfigure` constants alongside `deployState` in deploystate.go. Update markDeployPhase signature and the three call sites in helpers.go. JSON wire format is unchanged because the values are unchanged strings.  
**Effort:** hours

##### `smell:d6b325cb:duplicate-role-enum` — duplicate role enum

**Status:** not started  
**Severity:** minor  
**Cluster:** magic-strings — seam→audit-api-design  
**Evidence:** `internal/infrastructure/proxmox/types.go:42-49` + 1 more  
**Problem:** Two parallel typed enums name the same domain concept: `phase.NodeRole` (RoleBootstrap/RoleMaster/RoleWorker/RoleUnknown) and `proxmox.VMRole` (RoleBootstrap/RoleMaster/RoleWorker, no Unknown). They share the exact same string values verbatim. Each side has its own comment justifying the placement (avoiding an import cycle), yet phase already imports nothing from proxmox and proxmox already imports phase for VMState — the cycle excuse is one-directional.  
**Fix:** Drop `VMRole` and the three constants; type-alias if a name change is needed (`type VMRole = phase.NodeRole`). proxmox/types.go already imports phase for VMState — adding NodeRole costs no new import. Update VMStatus.Role accordingly. Risk medium because callers may switch-exhaustive over the proxmox values today.  
**Effort:** hours

##### `smell:e7db1220:duplicate-enum-label-fn` — duplicate enum label fn

**Status:** not started  
**Severity:** minor  
**Cluster:** helper-package-no-value  
**Evidence:** `internal/cli/releases.go:209-224` + 1 more  
**Problem:** `releaseTypeLabel` in cli/releases.go and `labelForReleaseType` in releases/types.go are identical switch tables on `ReleaseType`. The releases package already owns MarshalJSON/UnmarshalJSON for the type; one method (`func (t ReleaseType) String() string`) would let the cli caller and the marshaller share a single switch.  
**Fix:** Add `func (t ReleaseType) String() string` to releases/types.go containing the existing labelForReleaseType body, drop labelForReleaseType, change MarshalJSON to call String(), delete cli/releases.go releaseTypeLabel and call .String() from printVersionList / printVersionDetail.  
**Effort:** hours

##### `smell:fd2125dd:output-flag-magic-string` — output flag magic string

**Status:** not started  
**Severity:** minor  
**Cluster:** magic-strings  
**Evidence:** `internal/cli/addon.go:102-103` + 5 more  
**Problem:** The kubectl-style `--output`/`-o` format flag is registered with the bare string `"output"` (and shorthand `"o"`) at 7 call sites. cli/flags.go already holds the symmetric `flagOutput` constant for the file-destination flag (`--output-file`) with a doc comment naming the typo-guard rationale. The format flag has the same property and no constant.  
**Fix:** Add `flagOutputFormat = "output"` and `flagOutputFormatShort = "o"` to cli/flags.go. Replace all 7 `StringVarP(&X, "output", "o", outputText, ...)` sites. Doc comment can quote CLAUDE.md §architecture-notes (kubectl/oc convention) so the convention is explicit.  
**Effort:** hours

##### `smell:08ec0042:flags-package-not-canonical` — flags package not canonical

**Status:** not started  
**Severity:** suggestion  
**Cluster:** helper-package-no-value — related: smell:fd2125dd:output-flag-magic-string, smell:26a430ee:requires-root-annotation-key  
**Evidence:** `internal/cli/flags.go:1-10`  
**Problem:** cli/flags.go is a 10-line file holding two constants. The doc-comment correctly explains the typo-guard rationale, but the file holds only two flag names while the same package has at least 4 other repeated flag-name strings (`--output`, `-o`, `--config`, `requiresRoot`). The file is half-applied: it claims to be the canonical centralization point, but most of the package-level flag names live as bare strings in their registration sites.  
**Fix:** Either expand cli/flags.go to hold every shared flag-name constant (`flagOutputFormat`, `flagOutputFormatShort`, `flagConfig`, `flagLogLevel`, `flagLogFormat`, `flagLogFile`, `annotationRequiresRoot`) — see related smell:fd2125dd / smell:26a430ee — or inline the two existing constants back to their use sites and delete the file. The half-and-half state is the smell.  
**Effort:** hours

##### `smell:0f076161:destroy-only-stringly-typed` — destroy only stringly typed

**Status:** not started  
**Severity:** suggestion  
**Cluster:** magic-strings — seam→audit-cli-ux  
**Evidence:** `internal/cli/destroy.go:45-78`  
**Problem:** The `--only` flag accepts the bare strings "bootstrap"/"masters"/"workers"/"vms" via a switch. Compared to the parallel `--target` flag (which is regex-validated against a typed allowlist), `--only` reads as ad-hoc — there is no constant set, the help text duplicates the switch arms, and a future scope (e.g. "control-plane-only") would need updates in 3 places (the switch, the error message, and the help string).  
**Fix:** Define `type destroyScope string` with constants {scopeBootstrap, scopeMasters, scopeWorkers, scopeVMs} plus `validDestroyScopes()` so the help text, switch, and error message all read from one source. Cobra completion can return validDestroyScopes().  
**Effort:** hours

##### `smell:1d5afa08:release-type-unknown-default` — release type unknown default

**Status:** not started  
**Severity:** suggestion  
**Cluster:** magic-strings — seam→audit-errors  
**Evidence:** `internal/distribution/okd/releases/types.go:85-107`  
**Problem:** `UnmarshalJSON` for `ReleaseType` falls through to `*t = ReleaseTypeStable` on any unknown string. The label function returns "unknown" for the same out-of-range input on the marshal side. The asymmetry — produce "unknown" but consume by silent downgrade to "stable" — is a footgun: a feed entry with a typed-as-LTS-but-spelled-wrong field is round-tripped as Stable.  
**Fix:** Either reject unknown values with `return fmt.Errorf("unknown release type %q", s)` (caller's UnmarshalJSON branches on this) or add an explicit `ReleaseTypeUnknown` constant and round-trip through it. Today's implicit-stable default loses information on cache-format drift.  
**Effort:** hours

##### `smell:39c75e91:yes-no-magic-strings` — yes no magic strings

**Status:** not started  
**Severity:** suggestion  
**Cluster:** magic-strings  
**Evidence:** `internal/cli/confirm.go:60-62`  
**Problem:** `isConfirmResponse` encodes the Y/yes/y truthy set as three string literals inline. A second copy with a slightly different vocabulary lives in tui/wizard/datadriven.go (out of audit scope), but the smell at this site is that the canonical 'yes' parse for a destructive op is hand-rolled rather than going through `strconv.ParseBool` or a centralised helper.  
**Fix:** Replace the body with `strings.EqualFold(response, "y") || strings.EqualFold(response, "yes")` for case-insensitive match in one shot. Three literals -> two with no semantic loss; keeps in this package because the wizard parser is intentionally separate (looser vocabulary).  
**Effort:** hours

##### `smell:5013fea6:auth-error-string-sniff` — auth error string sniff

**Status:** not started  
**Severity:** suggestion  
**Cluster:** magic-strings — seam→audit-errors  
**Evidence:** `internal/distribution/okd/setup/release_extract.go:92-141`  
**Problem:** `isAuthError` classifies registry failures by lowercase substring search of the stderr against ["unauthorized","authentication","denied","forbidden","no basic auth","401","403"]. Error-string sniffing is the canonical un-idiomatic pattern: a registry that drifts wording silently downgrades a credential failure to a generic ClusterError. The repo already wraps in errtypes.AuthError elsewhere; the dispatch from oc stderr should look at the typed exit code first (it does — 1 / 125) and a structured marker second (oc emits JSON --output=json envelopes for newer versions).  
**Fix:** Document this site as the canonical 'string-sniff is acknowledged tech debt' boundary if no structured signal exists in oc, OR add `oc adm release extract --output=json` and parse the typed error envelope. If kept, deduplicate the lowercasing once outside the loop (already done) and add a TODO linking the upstream openshift/oc issue once filed.  
**Effort:** hours

##### `smell:62cb8a95:state-major-bounds-misnamed` — state major bounds misnamed

**Status:** not started  
**Severity:** suggestion  
**Cluster:** magic-strings  
**Evidence:** `internal/distribution/okd/destroy/helpers.go:26-59`  
**Problem:** `stateMajorMin, stateMajorMax = 1, 1` — two constants whose values are identical, named asymmetrically, and used in `if major < stateMajorMin || major > stateMajorMax`. The 'min' / 'max' framing implies a range; the body is in fact 'major must equal 1'. A future widening (terraform v2 support) would change *one* of the two and read like the bound got tighter, not looser. This is the bool-should-be-enum cluster's twin: two scalars where one suffices.  
**Fix:** Replace with `const requiredTerraformMajor = 1` and `if major != requiredTerraformMajor`. When v2 lands, change to a typed range struct then. The current spelling encodes a contract the body does not enforce — a 1.x match check.  
**Effort:** hours

##### `smell:6424733c:metrics-shutdown-timeout-magic` — metrics shutdown timeout magic

**Status:** not started  
**Severity:** suggestion  
**Cluster:** magic-strings  
**Evidence:** `internal/cli/helpers.go:228-256`  
**Problem:** `startMetricsServer` hardwires `5*time.Second`, `10*time.Second`, `60*time.Second` as ReadHeader/Read/Write/Idle/Shutdown timeouts. Per-timeout magic numbers in HTTP server construction are a frequent regression class — a future change wanting a tighter or looser default has to read the file to learn the implicit policy. The shutdown 5s in particular shows up as a comment hint, not a constant.  
**Fix:** Lift the four durations to package-level `const metricsReadHeaderTimeout = 5 * time.Second` etc. above startMetricsServer, with a single doc comment explaining 'Prometheus scrapers reconnect every interval; idle 60s leaves slack for slow scrapers'.  
**Effort:** hours

##### `smell:8154ab0f:doctor-severity-string-roundtrip` — doctor severity string roundtrip

**Status:** not started  
**Severity:** suggestion  
**Cluster:** magic-strings  
**Evidence:** `internal/cli/doctor.go:87-98`  
**Problem:** `severity` is a typed iota with three values; `sevString` switches on it and returns "ok"/"warn"/"fail"/"unknown". The three printable forms also exist as styled strings in `severityMarkers` ("[ok]"/"[warn]"/"[fail]") and as JSON values via sevString again. Three independent surface forms diverged from a common 3-state — risk is low but the JSON contract has zero test coverage of the unknown branch.  
**Fix:** Add `func (s severity) String() string` (with the existing body), drop sevString. severityMarkers can call .String() to derive the bracketed label. The 'unknown' branch is unreachable if severity is constructed via the iota constants — replace with a panic guard or document why it's reachable.  
**Effort:** hours

##### `smell:92553fff:summary-hardcoded-3state-fmt` — summary hardcoded 3state fmt

**Status:** not started  
**Severity:** suggestion  
**Cluster:** magic-strings  
**Evidence:** `internal/cli/summary.go:177-177` + 1 more  
**Problem:** `fmt.Sprintf("%-4s  %s", displayStatus(&s), d)` — the width 4 is the length of "fail" / "skip" rounded up; "%-4s" hardcodes the longest current value of stepDisplayStatus. Adding a fourth state ("warn") would silently cause column drift. Width should be max(len(stepStatus*)) computed once.  
**Fix:** Compute `const stepStatusColWidth = 4` at the top of summary.go alongside the stepDisplayStatus declarations, with a doc comment naming why 4 (= max len of current values). Replace both "%-4s" sites. Or use tabwriter as the rest of the package does for columnar output.  
**Effort:** hours

##### `smell:d31d1b9d:health-stringly-typed` — health stringly typed

**Status:** not started  
**Severity:** suggestion  
**Cluster:** magic-strings  
**Evidence:** `internal/cli/status.go:258-268` + 1 more  
**Problem:** Addon health is encoded as the bare strings "healthy" / "degraded" / "not enabled" at two sites. The same vocabulary already has a typed home — `okd.AddonStatus.Healthy` is a bool plus an `Error` string. The string literals should be a small typed enum (or a method on AddonStatus) so the printer cannot drift from the JSON output.  
**Fix:** Add `func (a AddonStatus) Label() string` to internal/distribution/okd/types.go returning healthy/degraded/not-enabled. Replace both call sites in status.go with a.Label() (the describe path also keeps appending err.Error()).  
**Effort:** hours

##### `smell:d9f7733e:bundle-category-magic-string-pair` — bundle category magic string pair

**Status:** not started  
**Severity:** suggestion  
**Cluster:** magic-strings  
**Evidence:** `internal/cli/debug_bundle.go:29-36`  
**Problem:** `categoryMustGather` / `categoryTerraformState` / `categoryDoctor` / `categoryConfig` / `categoryLogFile` / `categorySystemMeta` are typed `string` constants used as `manifestEntry.Name` slot keys. They're already centralized — the smell is the missing typed wrapper. Adjacent code wraps `bundleStatus` as a typed string with constants; `categoryX` should follow the same shape (`type bundleCategory string`) so a future entry that puts a category-typed value into manifestEntry.Name is compile-checked.  
**Fix:** Add `type bundleCategory string`, retag the 6 constants as `bundleCategory`, change manifestEntry.Name to `bundleCategory`. JSON wire format is unchanged because Go marshals string-typed values as strings.  
**Effort:** hours

##### `smell:daf5bee9:any-yaml-traversal` — any yaml traversal

**Status:** not started  
**Severity:** suggestion  
**Cluster:** interfaceany-lazy  
**Evidence:** `internal/cli/kubeconfig.go:141-175`  
**Problem:** `namedEntries` and `mergeNamedList` walk an unmarshalled kubeconfig as `map[string]any` of `[]any` of `map[string]any`. The kubeconfig schema is small and stable (clusters/contexts/users with .name); a typed shape would let the merge avoid four `, ok := ... .(...)` assertions per call. The `any`-soup makes a typo on the "name" key impossible to catch.  
**Fix:** Define a small private struct shape (`type namedItem struct { Name string `+"`"+`json:"name"`+"`"+`; raw map[string]any }`) and unmarshal both kubeconfigs into `struct { Clusters, Users, Contexts []namedItem }`. mergeNamedList becomes a typed loop. Risk medium because a typed model has to reproduce yaml round-trip fidelity for unknown fields — easy enough with raw json.RawMessage but worth a test.  
**Effort:** hours

#### audit-dependencies

##### `dep:33ef32bf:yaml-quad-engines` — yaml quad engines

**Status:** not started  
**Severity:** minor  
**Cluster:** duplicate-engine  
**Evidence:** `go.sum:125-153`  
**Problem:** go.sum still lists four YAML engines: sigs.k8s.io/yaml v1.6.0 (direct), go.yaml.in/yaml/v2 v2.4.3 (transitive), go.yaml.in/yaml/v3 v3.0.4 (transitive), and gopkg.in/yaml.v3 v3.0.1 (transitive via testify/check.v1). CLAUDE.md tripwire claims the count is 'down from four' to three, but the fourth (gopkg.in/yaml.v3) is still pulled via testify's gopkg.in/check.v1 dep tree.  
**Fix:** Either (a) update CLAUDE.md §dependencies tripwire text to read 'four YAML engines (one direct + three transitive)' so the running count matches reality, or (b) prune testify→gopkg.in/check.v1 by dropping testify if it is not actually used in okdctl tests (grep finds zero call sites). No code change either way; this is a doc/policy reconciliation.  
**Effort:** hours

##### `dep:b803fcb7:golangci-version-drift` — golangci version drift

**Status:** not started  
**Severity:** minor  
**Cluster:** pin-stability  
**Evidence:** `.github/workflows/ci.yml:19-21` + 1 more  
**Problem:** Makefile installs golangci-lint v2.12.1; ci.yml job lint-go pins golangci-lint-action with version v2.12.2. A developer running `make lint` locally executes a different linter than CI, so a clean local run can fail in CI on a rule that landed in v2.12.2 (or vice versa). The pre-commit lefthook hook does not run golangci-lint at all (only gofumpt + go vet), so the local feedback loop relies on `make lint`.  
**Fix:** Bump Makefile install line to @v2.12.2 (or whatever ci.yml currently pins). Better: extract the version into a file (e.g. .golangci-version) and have both Makefile and ci.yml read it, so future bumps are atomic. Lowest-risk fix is the literal sync.  
**Effort:** hours

##### `dep:3295df72:transitive-test-deps-from-proxmox` — transitive test deps from proxmox

**Status:** not started  
**Severity:** suggestion  
**Cluster:** transitive-weight  
**Evidence:** `go.sum:54-67`  
**Problem:** go.sum carries h2non/gock v1.2.0 + h2non/parth (test HTTP-mock deps), go-test/deep v1.1.1, gopkg.in/check.v1 — none of these are in okdctl's go.mod require block, none have call sites in okdctl's internal/ or cmd/. They land via go-proxmox's test-time deps and can't be pruned at the consumer side without dropping go-proxmox or applying a replace directive. Cosmetic; flagged for completeness.  
**Fix:** No action — Go's MVS pulls these because they appear in go-proxmox's test imports, even though okdctl doesn't compile or run those tests. They do not enter the release binary (test-only). Document as transitive-test-only acceptance.  
**Effort:** hours

##### `dep:33ef32bf:dup-log-engines-stack` — dup log engines stack

**Status:** not started  
**Severity:** suggestion  
**Cluster:** duplicate-engine  
**Evidence:** `go.mod:1-69`  
**Problem:** Four log engines coexist: stdlib log/slog (used by okdctl directly), charm.land/log/v2 (direct, intentional TUI stack — internal/tui/logger.go), github.com/go-logr/logr v1.4.3 (transitive via k8s.io/klog/v2), and k8s.io/klog/v2 v2.140.0 (transitive via apimachinery). Consolidation is not possible: charmlog is intentional UI stack (CLAUDE.md must-preserve in SKILL §5); klog/logr are baseline for k8s.io/* and never owned by okdctl. Document and accept; do not propose stripping.  
**Fix:** No action — record in CLAUDE.md (or this audit) that four log engines is the steady state for any project that consumes both Charm UI stack and k8s.io/* clients. Re-flag only if a NEW direct log dep enters go.mod or if charmlog gets replaced.  
**Effort:** hours

##### `dep:33ef32bf:exp-floor-stale-pseudoversion` — exp floor stale pseudoversion

**Status:** not started  
**Severity:** suggestion  
**Cluster:** justified-version-floor  
**Evidence:** `go.mod:57-57`  
**Problem:** golang.org/x/exp pinned at pseudo-version v0.0.0-20231006140011-7918f672742d (commit dated 2023-10-06, > 18 months old at audit date 2026-05-08). It is transitive-only (zero okdctl call sites). Floor is whatever Minimum Version Selection picked from a transitive bring-up; latest x/exp commits move regularly. Worth a `go get golang.org/x/exp@latest && go mod tidy` to refresh the floor, especially since x/exp routinely promotes APIs to stdlib.  
**Fix:** Run `go get golang.org/x/exp@latest && go mod tidy` and verify govulncheck + tests stay green. Or leave the floor as-is — MVS will lift it whenever a transitive consumer demands a newer commit. The dep adds zero direct surface to okdctl, so the lift is purely cosmetic.  
**Effort:** hours

##### `dep:33ef32bf:gorilla-websocket-stale` — gorilla websocket stale

**Status:** not started  
**Severity:** suggestion  
**Cluster:** maintenance-signal  
**Evidence:** `go.sum:61-62`  
**Problem:** gorilla/websocket v1.4.2 (released 2020-04, > 5 years old) is the transitive pin via go-proxmox. okdctl does not reach it (REST-only discovery, grep -r confirms zero call sites in internal/ and cmd/). Per CLAUDE.md §5a: keep until go-proxmox migrates to coder/websocket. Re-confirmation only.  
**Fix:** No action — the dep is transitive-only with no okdctl reachability. Track go-proxmox upstream releases (per CLAUDE.md, fallback plan is the REST-rewrite); when go-proxmox itself adopts coder/websocket, the bump auto-cleans. Re-confirm in the next dep audit.  
**Effort:** hours

##### `dep:33ef32bf:k8s-pseudoversion-floor` — k8s pseudoversion floor

**Status:** not started  
**Severity:** suggestion  
**Cluster:** justified-version-floor  
**Evidence:** `go.mod:64-66`  
**Problem:** k8s.io/kube-openapi v0.0.0-20260317180543-43fb72c5454a, k8s.io/utils v0.0.0-20260210185600-b8788abfbbc2, sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 are all pinned to commit-hash pseudoversions because k8s.io/* sub-modules ship without tagged releases. This is the upstream norm — apimachinery itself does this. Re-bump these three in lockstep when k8s.io/api is bumped, otherwise MVS can produce a mixed-vintage k8s tree. Note for future audits, no immediate action.  
**Fix:** On every k8s.io/api bump, run `go get k8s.io/kube-openapi@<matching-commit> k8s.io/utils@<matching-commit>` from the k8s.io/api go.sum to keep all sub-modules in lockstep. Document this in a release-prep checklist. No code change.  
**Effort:** hours

##### `dep:33ef32bf:proxmox-bus-factor-reconfirm` — proxmox bus factor reconfirm

**Status:** not started  
**Severity:** suggestion  
**Cluster:** maintenance-signal  
**Evidence:** `go.mod:12-12`  
**Problem:** luthermonson/go-proxmox v0.5.0: single maintainer (bus factor 1), v0.x API instability, sole Proxmox API client used in one 203-LOC file (internal/tui/wizard/steps/proxmox_discovery.go) calling client.Nodes/client.Node/Storages/Networks/GetContent. Pulls gorilla/websocket, magefile/mage, jinzhu/copier, buger/goterm, h2non/gock, h2non/parth — heavy transitive set for narrow REST-only usage. CLAUDE.md documents the ~200-LOC REST fallback and instructs not to rip out without the rewrite landing first.  
**Fix:** Re-confirm only — do NOT propose a swap this run. Keep the v0.5.0 pin; bump on each upstream release; track the documented ~200-LOC net/http rewrite in roadmap.md so the fallback exists when go-proxmox abandons. SKILL §5a explicitly forbids re-discovery and rip-out without the rewrite plan landing first.  
**Effort:** hours

##### `dep:33ef32bf:proxmox-version-drift` — proxmox version drift

**Status:** not started  
**Severity:** suggestion  
**Cluster:** maintenance-signal  
**Evidence:** `go.mod:12-12`  
**Problem:** CLAUDE.md §dependencies labels luthermonson/go-proxmox as 'v0.4.x — sole Proxmox discovery path. Bus-factor 1.' but go.mod has bumped to v0.5.0. The label and the constraint are both still accurate (still v0.x, still bus-factor 1, still single call site in proxmox_discovery.go), but the version sticker is stale. Re-confirmation per SKILL §5a, not novel.  
**Fix:** Edit CLAUDE.md §dependencies to read 'v0.5.x' (or 'current v0.x') so the policy doc tracks the actual pin. The abandonment plan (~200 LOC REST-only rewrite) and bus-factor-1 caveat both remain valid — only the version label is stale.  
**Effort:** hours

##### `dep:6ebdb617:claudemd-yaml-tripwire-stale` — claudemd yaml tripwire stale

**Status:** not started  
**Severity:** suggestion  
**Cluster:** duplicate-engine  
**Evidence:** `CLAUDE.md:196-201`  
**Problem:** CLAUDE.md §dependencies tripwire claims 'Count is down from four — gopkg.in/yaml.v3 was dropped from go.mod require.' That is technically accurate (it is no longer in the require block) but misleading: gopkg.in/yaml.v3 v3.0.1 still appears in go.sum via testify→gopkg.in/check.v1, so the operative engine count remains four. The tripwire fires only on a fourth direct add, masking the real duplicate.  
**Fix:** Edit CLAUDE.md §dependencies to either (a) say 'four YAML engines: sigs.k8s.io/yaml direct + go.yaml.in/yaml/v{2,3} via apimachinery + gopkg.in/yaml.v3 via testify/check.v1 — keep stable, do not add a fifth' or (b) actually drop testify by replacing test-only assertions with stdlib + go-cmp, then update the tripwire to say three.  
**Effort:** hours

#### audit-documentation

##### `doc:0139cb3f:pkg-doc-canonical-helpers` — pkg doc canonical helpers

**Status:** not started  
**Severity:** minor  
**Cluster:** package-doc  
**Evidence:** `internal/distribution/okd/phase/paths.go:1-1`  
**Problem:** Package phase is the canonical home for cross-phase helpers per CLAUDE.md §architecture-notes (BasePhase, OcResourceExists, OcPollOutput, etc.) but the package doc only says 'shared base types and path utilities'. Readers of godoc miss that this package is the canonical surface for new cross-phase helpers — and that adding helpers elsewhere is forbidden by CLAUDE.md. Surface the rule on the package clause.  
**Fix:** Expand to two-three sentences naming canonical helpers: 'Package phase hosts BasePhase plus the cross-phase helpers (OcResourceExists, OcPollOutput, NodeRole, ConditionStatus, VMState, SSHRunArgv) shared by setup, install, postinstall, destroy, and cleanup. New cross-phase helpers belong here per CLAUDE.md §architecture-notes — not in a specific phase package — to keep the import graph one-directional.'  
**Effort:** hours

##### `doc:4c092fce:pkg-doc-name-echo` — pkg doc name echo

**Status:** not started  
**Severity:** minor  
**Cluster:** package-doc  
**Evidence:** `internal/infrastructure/terraform/terraform.go:1-2`  
**Problem:** Package doc's second sentence is vacuous: 'It can be used by any infrastructure provider that uses Terraform.' That's not a contract — it's a tautology. Replace with what the package actually owns: subprocess shape (PATH lookup, env-allowlist via executor), state-file invariants (atomic writes via system.AtomicWrite), or call surface (Init/Plan/Apply/Destroy/Output).  
**Fix:** Replace the second sentence with a substantive description of the package's actual contract — e.g. 'Operations run via internal/executor with the default env allowlist; state files are written atomically via system.AtomicWrite. Provider packages drive it via Init / Plan / Apply / Destroy / Output.'  
**Effort:** hours

##### `doc:8aa632a6:pkg-doc-name-echo` — pkg doc name echo

**Status:** not started  
**Severity:** minor  
**Cluster:** package-doc  
**Evidence:** `internal/version/version.go:1-1`  
**Problem:** Package doc echoes the package name without adding signal: 'Package version provides version information for the okdctl CLI.' The package actually owns the build-time identity invariants (Version/GitCommit/BuildDate/GoVersion/Platform written exactly once before main, read race-free by BackgroundCheck) that already live in the file as a var-block doc — surface that contract on the package clause so godoc readers see the load-bearing constraint without reading the source.  
**Fix:** Expand the package doc to two sentences naming the build-time identity contract: 'Package version exposes the okdctl build identity (Version, GitCommit, BuildDate, GoVersion, Platform). All values are -ldflags-injected before main and read race-free at runtime; tests must save/restore via t.Cleanup to avoid leaking mutations across boundaries.'  
**Effort:** hours

##### `doc:35abd54e:pkg-doc-thin` — pkg doc thin

**Status:** not started  
**Severity:** suggestion  
**Cluster:** package-doc  
**Evidence:** `internal/credentials/proxmox.go:1-1`  
**Problem:** Package credentials carries the load-bearing []byte/Zeroize/Redacted-interface contract for credential lifecycle, but the package doc is one vacuous sentence: 'Package credentials provides credential management for infrastructure providers.' The doc should announce the type-level invariants (Password/APIToken []byte for in-memory wipe, Redacted() interface for slog scrubbing, defer Zeroize() lifecycle) so a godoc-reading caller does not have to scan ProxmoxCredentials's type doc to discover them.  
**Fix:** Expand to two-three sentences: 'Package credentials owns the Proxmox credential lifecycle. Password and APIToken are []byte (not string) so callers can defer Zeroize() to wipe them after use; ProxmoxCredentials.Redacted() satisfies logutil.RedactHandler so structured slog attrs never leak. See LoadEnvFile and GetProxmoxCredentials for the env-then-config resolution priority.'  
**Effort:** hours

##### `doc:beabab0c:pkg-doc-name-echo` — pkg doc name echo

**Status:** not started  
**Severity:** suggestion  
**Cluster:** package-doc  
**Evidence:** `internal/distribution/okd/setup/phase.go:1-1`  
**Problem:** Package doc echoes the package name with no added signal: 'Package setup provides the setup phase for OKD cluster provisioning.' Setup is the largest phase (~2.3K LOC of code) and owns rendering install configs, manifests, custom CoreOS ISOs, and configuring HAProxy/DNS/firewall on the bastion — a one-line name-echo gives readers no map.  
**Fix:** Expand to two-three sentences listing the phase's actual surface: 'Package setup runs the setup phase: install host packages and the tool trio (oc / openshift-install / terraform), render install configs and manifests, build custom CoreOS ISOs with embedded kargs, then configure HAProxy, DNS, and the bastion firewall. Steps are declared via setupBaseSteps / setupManifestSteps / setupWebSteps / setupInfraSteps and concatenated in setupSteps.'  
**Effort:** hours

##### `doc:d5915b0c:pkg-doc-name-echo` — pkg doc name echo

**Status:** not started  
**Severity:** suggestion  
**Cluster:** package-doc  
**Evidence:** `internal/distribution/okd/install/phase.go:1-1`  
**Problem:** Package doc echoes the package name with no added signal: 'Package install provides the install phase for OKD cluster provisioning.' The package owns the bootstrap-monitor + CSR-approval + cluster-operator-wait sequence with timeouts (DefaultBootstrapTimeout 30m, DefaultInstallTimeout 60m, DefaultCSRApprovalInterval 30s); the doc should surface the timeline, not the package name.  
**Fix:** Expand: 'Package install drives the install phase: terraform-up, bootstrap monitor, CSR approval, cluster-operator settle. Default timeouts are 30 m for bootstrap, 60 m for the cluster-operator wait, and a 30 s CSR-approval poll cadence; all are overridable via Deployment.* in the Config.'  
**Effort:** hours

#### audit-tests

##### `tst:bfdaf5e3:cred-bytes-type-untested` — cred bytes type untested

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-bfdaf5e3-cred-bytes  
**Severity:** blocker  
**Cluster:** cred-path-untested  
**Evidence:** `internal/config/secret.go:1-53`  
**Problem:** config.SecretBytes is the credential-bearing wrapper used for ProxmoxConfig.Password and ProxmoxConfig.APIToken (cluster.go:120-121). It exposes Set/Bytes/Zeroize/IsEmpty/String/Redacted, all of which are untested. The String()=='[redacted]' invariant and the Zeroize-overwrites-prior-Set-buffer invariant are both unguarded — a future refactor that drops Redacted() or returns Bytes() from String() would silently leak credentials through every fmt.Sprintf log site that takes a config value.  
**Fix:** Add internal/config/secret_test.go covering: (1) String() returns the literal '[redacted]' for any populated buffer, (2) Set replaces the prior buffer and Zeroizes the old backing array, (3) %v / %s / %+v fmt verbs all emit '[redacted]', (4) Redacted() returns '[redacted]', (5) IsEmpty toggles around Set/Zeroize, (6) Bytes() returns the live slice and a follow-up Zeroize wipes the bytes the caller still holds (caller-must-not-retain contract). Pattern-match credentials/proxmox_test.go::TestProxmoxCredentials_StringMasks for the fmt-verb sweep.  
**Effort:** days

##### `tst:40d315ad:destructive-happy-untested` — destructive happy untested

**Status:** not started  
**Severity:** major  
**Cluster:** destructive-untested  
**Evidence:** `internal/addon/catalog/flux/flux.go:237-251`  
**Problem:** Flux.Uninstall calls helm uninstall flux-instance + flux-operator and 'oc delete ns flux-system'. There is no test exercising Uninstall even though flux is the only addon registered today and Manager.installAndVerify rolls back via Uninstall on every install failure. Manager rollback is tested with stubs, but the real Flux.Uninstall body is unguarded. A regression that swaps 'oc delete ns flux-system' for 'oc delete ns' (typo loses the namespace arg) would propagate through manager rollback and delete every namespace.  
**Fix:** Add internal/addon/catalog/flux/flux_uninstall_test.go installing fake oc/helm via PATH (pattern-match cli/cleanup_test.go::installFakeOC), running Uninstall, and asserting: (1) the three commands fire in order, (2) each carries its expected argv ('helm uninstall flux-instance --namespace flux-system', 'oc delete ns flux-system'), (3) per-command failures are logged but do not abort the sequence (warnOnErr contract).  
**Effort:** hours

##### `tst:d7ce9d16:destructive-happy-untested` — destructive happy untested

**Status:** not started  
**Severity:** major  
**Cluster:** destructive-untested — seam→audit-state-and-recovery  
**Evidence:** `internal/distribution/okd/dns/dns.go:175-277`  
**Problem:** DeployBootstrap, DeployProduction, validateAndRestartDnsmasq, and the rollback-on-failure path that re-CopyFiles the .backup over /etc/dnsmasq.d/<name>.conf are all untested. validateAndRestartDnsmasq is the rollback-on-validation-failure shape that haproxy_rollback_test.go locks for the parallel haproxy path, but the dnsmasq twin has no test — a regression that silently drops the restore() call in the validation-fail branch leaves the cluster with broken DNS and no way to recover.  
**Fix:** Add internal/distribution/okd/dns/dns_destructive_test.go redirecting dnsmasqConfigDir to t.TempDir(), seeding a fixture, and exercising: (1) validateAndRestartDnsmasq happy path leaves config in place and removes .backup, (2) ValidateDnsmasqConfig failure restores from .backup, (3) RestartDnsmasq failure restores from .backup, (4) missing .backup is not a precondition. Inject ValidateDnsmasqConfig/RestartDnsmasq via package vars (or extract a small struct) so the test doesn't need a real dnsmasq binary. Pattern-match setup/haproxy_rollback_test.go::TestAttemptHAProxyRollback.  
**Effort:** days

##### `tst:de572c63:destructive-happy-untested` — destructive happy untested

**Status:** not started  
**Severity:** major  
**Cluster:** destructive-untested — seam→audit-state-and-recovery  
**Evidence:** `internal/distribution/okd/dns/dnsmasq.go:253-266`  
**Problem:** RestoreSystemResolver removes /etc/systemd/resolved.conf.d/dnsmasq.conf and restarts systemd-resolved. The /etc-touching write is gated by a hardcoded path const with no package-var indirection — there is no test seam, and consequently no test. A regression that loosens the FileExists guard would attempt RemoveAll on a path the cleanup never owned.  
**Fix:** Lift the resolvedConf const to a package-level var so a test can redirect it to t.TempDir(). Then add internal/distribution/okd/dns/restore_resolver_test.go covering: (1) missing drop-in is a no-op, (2) present drop-in is removed, (3) RemoveAll error is logged but not propagated. Mirror cleanup/services_test.go::TestDnsmasq_GlobLoopRemovesAllMatches for the var-redirection pattern.  
**Effort:** hours


## Completed

Completed items live in [`docs/roadmap/completed-archive.md`](docs/roadmap/completed-archive.md). Grep there for the canonical "is dep X done?" lookup. The previous in-line pointer index (144 entries, mirroring archive contents) was removed on 2026-05-09 to keep `roadmap.md` focused on active work.

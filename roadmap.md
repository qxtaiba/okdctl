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

##### `ux:e7db1220:format-vs-output-flag-name-drift` — format vs output flag name drift

**Status:** in review — PR #521  
**Severity:** suggestion  
**Cluster:** flag-conventions  
**Evidence:** `internal/cli/releases.go:76-82` + 1 more  
**Problem:** Format-selector flags are named `--format` on `releases list/show`, `status`, `describe node/addon`. Output-destination flags are `--output`/`-o` on `deploy`, `kubeconfig`, `debug-bundle`. kubectl/oc convention treats `-o/--output` as the format selector (`-o json`, `-o yaml`); okdctl splits the conventional `-o` namespace and surprises kubectl-fluent users who type `okdctl status -o json` and get a usage error.  
**Fix:** Option A (recommended pre-1.0): rename `--format` to `--output`/`-o` everywhere it acts as a format selector; rename the file-destination uses to `--output-file` so the `-o` shorthand is reserved kubectl-style. Option B (zero-break): add `--output`/`-o` as a hidden alias for `--format` on status/describe/releases; document one canonical name. Pick before 1.0; add the chosen convention to CLAUDE.md §architecture-notes.  
**Effort:** hours


#### audit-observability

##### `obs:6424733c:fmt-sprintf-message-pattern` — fmt sprintf message pattern

**Status:** in review — PR #522  
**Severity:** major  
**Cluster:** field-stability  
**Evidence:** `internal/cli/helpers.go:32-296` + 5 more  
**Problem:** Pervasive `tui.Info(fmt.Sprintf(...))` / `p.Log.Info(fmt.Sprintf(...))` pattern across ~50 sites in cli/ and distribution/okd/. Stringifying interpolated values into the message text bypasses RedactHandler structured-attr scrub (which only rewrites attr keys/values), kills slog-handler ability to filter by field, and prevents downstream JSON pipelines from extracting the dynamic value (path, cluster name, count, duration). CLAUDE.md is explicit: prefer structured attrs over fmt.Sprintf so the handler can inspect values.  
**Fix:** Mechanical sweep: every `tui.X(fmt.Sprintf("prefix: %s", v))` becomes `tui.X("prefix", tui.LF("key", v))`; every `p.Log.X(fmt.Sprintf("prefix: %s", v))` becomes `p.Log.X("prefix", "key", v)`. ~50 sites; one PR per package keeps churn reviewable. Roll-up message stays static; values move to attrs.  
**Effort:** hours

#### audit-modernization


#### audit-code-smells

#### audit-dependencies

#### audit-documentation

#### audit-tests

## Completed

Pointer index. Full bodies live in `docs/roadmap/completed-archive.md`. Bulk-archived on 2026-05-05 (140 entries).

- `E6` — done PR #124
- `F1` — done 2026-04-22 (commit 73912b5)
- `iac:b803fcb7b:tflint-recommended-findings` — done 2026-04-22 (commit f6abdb2)
- `api:0934cf1b:should-be-exported` — done PR #184
- `api:125729c4:opt-inconsistent-cfg-opts` — done PR #115
- `api:262af6e4:zero-value-usable-cleanup` — done PR #115
- `api:4c092fce:opt-inconsistent-terraform-ctors` — done PR #115
- `api:73ad30ef:export-no-caller-external-tool-binaries` — done PR #115
- `api:830d4653:export-no-caller-installed-lists` — done PR #115
- `api:ae5b624c:concrete-return-k8s` — done PR #115
- `api:beabab0c:mix-default-new-naming` — done PR #115
- `api:c287d5c0:withenv-order-coupling` — done PR #115
- `api:d7ce9d16:export-no-caller-dns-config-helpers` — done PR #115
- `api:dd75bdeb:export-no-caller` — done PR #181
- `api:ed55ee90:export-no-caller-generate-summary` — done PR #115
- `con:39c75e91:go-no-wait` — done acceptance note (CLAUDE.md §Concurrency)
- `con:484b40f0:lock-held-during-write` — done PR #109
- `con:6424733c:metrics-shutdown-bg-ctx` — done PR #202
- `con:8e65d574:go-no-wait` — done acceptance note (CLAUDE.md §Concurrency)
- `con:aa84670c:time-after-update-notice-ok` — done PR #179
- `con:ab9b764a:validate-ignition-only-checks-ctx-once` — done PR #183
- `con:ae5b624c:go-leak-on-error` — done acceptance note (CLAUDE.md §Concurrency)
- `con:bdf5a873:safe-remove-ignores-ctx` — done PR #175
- `con:e7db1220:releases-completion-bg-ctx` — done PR #172
- `con:f5d703ab:install-tools-to-system-no-ctx` — done PR #182
- `dep:33ef32bf:copyleft-audit-clean` — done acceptance note (CLAUDE.md §Dependencies)
- `dep:33ef32bf:go-yaml-in-fork-risk` — done acceptance note
- `dep:33ef32bf:golang-x-exp-stale` — done acceptance note (transitive upstream)
- `dep:33ef32bf:ultraviolet-pseudo-version` — done acceptance note (charm ecosystem convention)
- `dep:33ef32bf:yaml-quad-engines` — done acceptance note (CLAUDE.md §Dependencies)
- `dep:87db21a9:goreleaser-action-version-tag` — done acceptance note (cosign trust model)
- `dep:b803fcb7:workflow-pin-hygiene-clean` — done acceptance note (tripwire)
- `doc:0139cb3f:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:125729c4:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:297adb3e:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:2f70d7df:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:54654337:destroy-cli-ref-stale` — done docs commit d30866a
- `doc:588ce79e:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:632c9087:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:660d83a5:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:66cb1c69:addons-buildopaquesecret-sig` — done docs commit d30866a
- `doc:6fc3d91e:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:70b3bae2:wizard-registration-stale` — done docs commit d30866a
- `doc:983f67f0:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:a55b4592:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:a9ea115f:pkg-doc-too-long` — done PR #113
- `doc:aa0f50f5:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:ab9b764a:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:bc9ba9bc:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:beabab0c:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:c14fdd9d:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:cf43073b:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:d5915b0c:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:e2343d2c:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:e3782ee7:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:f99eddfa:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `doc:fb54208a:exported-doc-missing` — done covered by F1 (commit 73912b5)
- `err:40d315ad:wrap-tool-prereq-untyped` — done PR #105
- `err:48688e63:typed-err-fallthrough` — done PR #105
- `err:5013fea6:str-sniff-tool-msg` — done PR #205
- `err:7b2829bb:typed-no-error-iface` — done PR #105
- `err:9d79b841:fcos-stream-status-bare` — done PR #203
- `err:a55b4592:vocab-ad-hoc-config-perm` — done PR #204
- `err:aa84670c:ctx-err-check-on-ctx` — done PR #105
- `err:c287d5c0:vocab-ad-hoc-distribution-type` — done PR #153
- `err:ddf885f4:errors-join-ctx-lost` — done PR #105
- `err:f51f85bb:err-stringified-loses-type` — done PR #105
- `iac:b803fcb7:tflint-no-config` — done PR #114
- `iac:e076e43c:sh-posix-not-bash` — done PR #114
- `mod:0934cf1b:use-slices-concat` — done PR #107
- `mod:35abd54e:use-builtin-clear` — done PR #160
- `mod:6fc3d91e:use-strings-lines` — done PR #105
- `mod:7b2829bb:use-slices-containsfunc` — done PR #161
- `mod:983f67f0:use-builtin-max-innerwidth` — done PR #107
- `mod:983f67f0:use-builtin-max-padding` — done PR #107
- `mod:9d79b841:use-slices-max` — done PR #107
- `mod:d31d1b9d:use-map-index` — done PR #105
- `mod:f55b9c27:use-builtin-clear` — done PR #159
- `obs:00000002:inconsistent-domain-prefix-keys` — done PR #106
- `obs:15ba17da:err-stringified-into-label` — done PR #106
- `obs:19a715fd:level-warn-help-text` — done PR #106
- `obs:33579dd5:err-stringified-bypasses-handler` — done PR #176
- `obs:366b3f2d:span-no-start-end-per-step` — done PR #106
- `obs:366b3f2d:step-completed-info-on-failure` — done PR #180 (follow-up PR #191)
- `obs:48688e63:apply-failure-no-err-attr` — done PR #165
- `obs:48688e63:message-embedded-counts` — done PR #106
- `obs:6424733c:string-concat-err-error-in-tui` — done PR #155
- `obs:660d83a5:run-id-mutation-race` — done PR #188
- `obs:7b2829bb:executor-no-output-span` — done PR #106
- `obs:97cb8adf:waitfor-no-retry-count` — done PR #173
- `obs:9d79b841:duplicate-iso-exists-log` — done PR #106
- `obs:aa84670c:root-error-stringified` — done PR #106
- `obs:ae5b624c:monitor-retry-log-per-tick` — done PR #106
- `obs:c287d5c0:cleanup-warning-key-vague` — done PR #166
- `obs:ed55ee90:summary-keys-leading-whitespace` — done PR #189
- `sec:06f00bcb:ignition-dir-perms` — done PR #111
- `sec:19a715fd:secretstore-plaintext-disk` — done PR #111
- `sec:26a430ee:syscall-exec-env-leak` — done PR #111
- `sec:35abd54e:input-url-scheme-not-checked` — done PR #128
- `sec:98723e5d:bashrc-chown-leak` — done PR #123
- `sec:d66c3d7f:bashrc-no-nofollow` — done PR #111
- `smell:004ad79b:helper-pkg-thin-wrap` — done PR #110
- `smell:0934cf1b:query-match-mini-dsl` — done PR #110
- `smell:1d5afa08:enum-via-sscanf-int-parse` — done PR #110
- `smell:2f70d7df:magic-default-port` — done PR #162
- `smell:4f69fc9d:stepbuilder-build-no-callers` — done scaffolding retained per MEMORY.md
- `smell:696d6b0e:redundant-vmstatus-enum` — done PR #266
- `smell:830d4653:duplicate-os-fallback` — done PR #177
- `smell:8aa632a6:duplicate-platform-string` — done PR #163
- `smell:9d79b841:strconv-fallback-to-zero` — done PR #174
- `smell:c5e5c304:build-role-helper-near-duplicate` — done PR #110
- `smell:c5e5c304:named-return-unnecessary` — done PR #110
- `smell:d31d1b9d:role-string-instead-of-enum` — done PR #178
- `smell:daf5bee9:yaml-tree-walk-repeat-assertion` — done PR #110
- `state:48688e63:provision-leaves-tfplan` — done PR #109
- `state:4c092fce:no-concurrent-run-guard` — done PR #118
- `state:4c092fce:tf-state-backup-removed-on-success` — done PR #154
- `state:93957c53:cleanup-no-confirm-cluster` — done PR #109
- `state:b804b2ec:bootstrap-destroy-skip-tfvars-silent` — done 2026-04-26 — PR #149
- `sub:7b2829bb:unbounded-output-buffer` — done PR #119
- `sub:97cb8adf:no-cmd-env` — done 2026-04-26 — PR #147
- `sub:ae5b624c:bypass-canonical-executor` — done 2026-04-26 — PR #148
- `sub:e2343d2c:systemd-stderr-dropped` — done PR #112
- `tst:25fa1be8:no-test-validateport-attacker` — done PR #112
- `tst:98723e5d:no-test-add-kubeconfig-bashrc` — done PR #112
- `tst:ab9b764a:no-test-installconfig-perms` — done PR #112
- `ux:024a2c32:json-schema-doc-drift` — done docs commit d30866a — docs/cli/json-schema.md rewritten to match actual marshaled shapes; golden-test deferral tracked via audit-tests gap entry
- `ux:073d24ed:dry-run-yes-short-circuit` — done PR #108
- `ux:08c49fc4:remove-haproxy-no-x-bool-default-true` — done PRs #187 + #193
- `ux:0f076161:destroy-force-deprecated-but-still-default-binding` — done PRs #185 + #192
- `ux:54654337:readme-flag-drift` — done docs commit d30866a — `make docs` regenerated docs/cli/okdctl_destroy.md with the three skip-* flags
- `ux:6424733c:no-tty-prompt-returns-false-silently` — done PR #186
- `ux:8d8faa80:completion-powershell-on-linux-only-tool` — done PR #164
- `ux:8d8faa80:completion-use-bracket-optional` — done PR #108
- `ux:93957c53:cleanup-no-dry-run` — done PR #108
- `ux:aa84670c:exit-code-bsd-sysexits-partial` — done PR #108
- `ux:d31d1b9d:json-key-hyphenated` — done PR #105
- `ux:e45c2239:sig-not-handled-preflight` — done PR #108
- `ux:e7db1220:releases-show-no-completion` — done PR #108
- `ux:fd2125dd:addon-uninstall-no-confirm` — done PR #206


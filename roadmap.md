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

##### `sec:35abd54e:env-string-residue` — env string residue

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-35abd54e-env-string-residue  
**Severity:** major  
**Cluster:** credentials  
**Evidence:** `internal/credentials/proxmox.go:151-172`  
**Problem:** ProxmoxCredentials.Env() returns []string with credential bytes converted to immutable Go strings (envProxmoxPassword+"="+string(c.Password)). The doc-comment correctly notes this and tells callers to defer Zeroize(). However the resulting strings live as long as any struct that stores cmd.Env (Executor.Env, Provider.env) and become heap garbage after the env slice is replaced. Zeroize() cleans the []byte source but cannot reach the strings. terraform.Executor.ZeroizeEnv() partially mitigates by overwriting the entry with empty before clear()ing — but executor.Executor has no equivalent.  
**Fix:** Add executor.Executor.ZeroizeEnv() mirroring terraform.Executor.ZeroizeEnv (terraform.go:L347-L364). Update internal/distribution/okd/install/* and internal/infrastructure/proxmox/proxmox.go to defer e.Exec.ZeroizeEnv() at end of phases that pass creds.Env(). Document the residual-string boundary in CLAUDE.md §credentials-and-secrets so future env-passing sites follow the same defer pattern.  
**Effort:** hours

##### `sec:fde34e0c:k8s-kubeconfig-env-no-validate` — k8s kubeconfig env no validate

**Status:** in review — PR #628  
**Severity:** suggestion  
**Cluster:** input-validation  
**Evidence:** `internal/cluster/k8s.go:55-64`  
**Problem:** WithEnvFallback() reads $KUBECONFIG from the process environment without validation. A user-controlled KUBECONFIG pointing at /dev/zero, /proc/self/environ, or a 100MB malicious yaml is consumed by every K8sClient constructed with this option. Production callers (install/postinstall) explicitly pass WithKubeconfig with a project-rooted path, so reachability is bounded — flagging as a hardening suggestion for tools or future callers.  
**Fix:** Add a simple sanity check inside WithEnvFallback: filepath.Clean the env value, refuse if it points outside $HOME or /etc, and Lstat-refuse symlinks. Match the resolveProjectRootOrDie pattern in cli/helpers.go.  
**Effort:** hours

#### audit-subprocess

##### `sub:a6e38cc7:no-stderr-capture` — no stderr capture

**Status:** in review — PR #618  
**Severity:** minor  
**Cluster:** io-handling  
**Evidence:** `internal/sshpin/sshpin.go:41-47`  
**Problem:** runKeyscan calls cmd.Output() which returns stdout but lets stderr inherit the parent fd (TTY in interactive runs, /dev/null in piped runs). When ssh-keyscan -T 5 fails because the remote port 22 is closed, the diagnostic line lands on the operator's terminal but never reaches the returned err — the wrapper just becomes 'ssh-keyscan host: exit status 1'. Sibling system.OutputCaptured at exec.go:73 captures stderr into the typed error.  
**Fix:** Swap to system.OutputCaptured(ctx, "ssh-keyscan", "-T", "5", host) so stderr flows into a *SubprocessError and the structured log handler can redact + render it. Net: same return shape, better error.  
**Effort:** hours

##### `sub:e2343d2c:no-cmd-env` — no cmd env

**Status:** in review — PR #619  
**Severity:** minor  
**Cluster:** io-handling  
**Evidence:** `internal/system/systemd.go:40-68` + 2 more  
**Problem:** systemd.go has three exec.CommandContext sites (ManageService ServiceStatus, IsServiceActive, IsServiceEnabled) that bypass the executor.FilterParentEnv allowlist used by sibling helper RunCaptured at exec.go:54. Under sudo re-exec these run as root, inheriting the unfiltered parent env (any GITHUB_TOKEN / GH_TOKEN / GIT_ASKPASS the operator's shell exported). Inconsistent with the env-isolation invariant the rest of the system package upholds.  
**Fix:** Either set cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist) at each site, or route through system.RunCaptured (drop the bare cmd.Run wrappers and let RunCaptured handle exit-code → bool conversion via errors.As against *SubprocessError).  
**Effort:** hours

#### audit-state-and-recovery

##### `state:b5a79fda:deploy-state-marker-stale` — deploy state marker stale

**Status:** in review — PR #621  
**Severity:** minor  
**Cluster:** crash-recoverability  
**Evidence:** `internal/cli/deploystate.go:24-86`  
**Problem:** announceDeployState reads .okdctl-deploy-state.json on destroy entry and emits 'partial deploy detected — cancelled during X'. But there's no TTL or mtime check: a marker left from a successful deploy whose clearDeployMarker failed (errno EBUSY, RO mount, signal during cleanup) misleads the operator on a totally unrelated destroy weeks later. The deploy marker also doesn't survive cross-run identity (RunID is per-process, not per-cluster), so two operators on the same project can't tell which one's partial deploy left this marker.  
**Fix:** On readDeployState, also stat the file and emit the marker's age in the warning ('marker is 14 days old — likely stale'). Add a ClusterName field to deployState struct; on announceDeployState, when ds.ClusterName != cfg.Cluster.Name, emit 'marker is from a different cluster, ignoring'. Make markDeployPhase a fatal (not best-effort) on the first call so a write-failed marker can't accumulate.  
**Effort:** hours

#### audit-iac-and-shell

#### audit-errors

##### `err:48688e63:cancel-identity-lost-on-tf-apply` — cancel identity lost on tf apply

**Status:** in review — PR #615  
**Severity:** major  
**Cluster:** cancellation-identity — seam→audit-concurrency  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:195-203`  
**Problem:** On terraform-apply cancellation, applyErr is the executor.ExitError from a SIGTERM'd subprocess — it does NOT carry context.Canceled in its chain. The wrap at L199 forwards applyErr only ('terraform apply interrupted: %w', applyErr); errors.Is(returned, context.Canceled) downstream returns false because the underlying chain is ExitError → exec.ExitError, no ctx identity. cli/root.go::signalExitCode (L183) gates the SIGINT→130 / SIGTERM→143 mapping on errors.Is(err, context.Canceled || DeadlineExceeded), so a Ctrl-C during terraform apply currently exits 1 instead of 130. install/monitor.go:68 has the canonical fix shape: 'fmt.Errorf(...: %w', ctx.Err())'.  
**Fix:** Wrap ctx.Err() instead of (or alongside) applyErr: 'return nil, fmt.Errorf("terraform apply interrupted: %w", ctx.Err())'. Or use errors.Join(ctx.Err(), applyErr) so both identities walk through. Net +0 LOC. Apply the same fix to PlanOnly's plan / Init paths if they grow ctx-cancel checks.  
**Effort:** hours

##### `err:7b2829bb:exit-error-no-redact` — exit error no redact

**Status:** in review — PR #616  
**Severity:** major  
**Cluster:** redaction-in-error — seam→audit-observability — related: obs:6424733c:fmt-sprintf-message-pattern  
**Evidence:** `internal/executor/executor.go:187-198`  
**Problem:** executor.ExitError carries a Stderr string field but does NOT implement Redacted() any. The sibling type SubprocessError at internal/system/exec.go:42 does — the asymmetry means subprocess stderr surfaced through ExitError reaches slog/tui sinks unredacted. ExitError is the canonical typed exit error returned by RunChecked, RunStreamedChecked, K8sClient.runCheck, terraform.run, and OcOutput; every site that wraps an oc/kubectl/terraform/sops failure into the cli error chain forwards the raw subprocess stderr.  
**Fix:** Add Redacted() any returning {Command, ExitCode} (drop Stderr) so logutil.RedactHandler's interface{ Redacted() any } switch (internal/logutil/redact.go:113) catches ExitError when it lands as a slog attr. Mirror system.SubprocessError.Redacted at internal/system/exec.go:42-44. Net +4 LOC. terraform.ExecError aliases ExitError so it inherits automatically.  
**Effort:** hours

##### `err:fde34e0c:exit-error-no-ctx-identity` — exit error no ctx identity

**Status:** not started  
**Severity:** minor  
**Cluster:** cancellation-identity — seam→audit-concurrency — related: err:48688e63:cancel-identity-lost-on-tf-apply  
**Evidence:** `internal/cluster/k8s.go:108-127` + 6 more  
**Problem:** Every site that constructs an executor.ExitError on a non-zero exit code does so without consulting ctx.Err() — so a subprocess SIGTERM'd via cmd.Cancel that happens to exit non-zero produces an ExitError chain with no context.Canceled identity. Downstream errors.Is(err, context.Canceled) returns false at cli/root.go::signalExitCode, mapping the SIGINT to exit 4 (ClusterError) instead of 130. Pattern repeats at executor.go:L303,L355,L367, k8s.go:L120, phase/kubectl.go:L35, setup/release_extract.go:L128, setup/upload.go:L28. install/monitor.go has the canonical fix shape (check ctx.Err first).  
**Fix:** Centralise: at every ExitError construction site, prefer ctx.Err() when ctx is cancelled. Cleanest landing is a helper in executor: 'func newExitError(ctx context.Context, cmd string, code int, stderr string) error { if err := ctx.Err(); err != nil { return err }; return &ExitError{Command: cmd, ExitCode: code, Stderr: stderr} }'. 7 call sites use it. Net +6 LOC.  
**Effort:** hours

#### audit-concurrency

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

#### audit-cli-ux

##### `ux:fd2125dd:args-validator-not-usageerror` — args validator not usageerror

**Status:** in review — PR #617  
**Severity:** major  
**Cluster:** exit-codes — seam→audit-errors  
**Evidence:** `internal/cli/addon.go:62-73`  
**Problem:** addonInstallCmd.Args returns plain fmt.Errorf for missing-or-conflicting positional args. The error reaches exitCodeFor without satisfying errors.As(*errtypes.UsageError), so the process exits 1 (generic) instead of 64 (EX_USAGE). The published taxonomy in docs/cli/exit-codes.md says 64 covers cobra flag-parse failures; positional-arg failures are the natural sibling and currently violate the taxonomy.  
**Fix:** Wrap each return in &errtypes.UsageError{Msg: ..., Err: ...} so exitCodeFor() resolves to 64 (EX_USAGE), matching the documented taxonomy and the SetFlagErrorFunc pattern in root.go:256-259. Apply the same pattern to any other Args validator that returns a raw fmt.Errorf for arg-shape violations.  
**Effort:** hours

##### `ux:daf5bee9:kubeconfig-status-msg-bypasses-tui` — kubeconfig status msg bypasses tui

**Status:** in review — PR #624  
**Severity:** minor  
**Cluster:** streams — seam→audit-observability  
**Evidence:** `internal/cli/kubeconfig.go:72-72`  
**Problem:** After AtomicWrite or merge succeeds, kubeconfig prints a free-form status line via fmt.Fprintf(os.Stderr, ...) instead of tui.Info. The repo's slog → RedactHandler chain is bypassed; future additions of attrs to that line silently lose redaction. Stream choice (stderr) is correct, but the sink should be tui so structured fields are honoured.  
**Fix:** Replace fmt.Fprintf(os.Stderr, ...) with tui.Info("kubeconfig written", tui.LF("path", kubeconfigOutput)) and tui.Info("kubeconfig merged", tui.LF("path", dest)). Matches CLAUDE.md §credentials-and-secrets directive that all log sinks pass through RedactHandler so future fields cannot leak.  
**Effort:** hours

##### `ux:08ec0042:flag-output-name-collision-risk` — flag output name collision risk

**Status:** not started (already implemented in commit 08d3e35 — needs archive, not pickup)  
**Severity:** suggestion  
**Cluster:** flag-conventions  
**Evidence:** `internal/cli/flags.go:7-10`  
**Problem:** The constant `flagOutput = "output-file"` is used by deploy (--output-file = config dest) and debug-bundle (--output-file = bundle dest). Eight other commands register --output (not --output-file) for format selection via StringVarP. The repo convention codified in CLAUDE.md §architecture is correct, but a maintainer skimming `flagOutput` reads it as `--output` and will land on the wrong flag. The constant name aliases an existing flag-spelling; rename or split.  
**Fix:** Rename the constant to `flagOutputFile` (paired with `flagOutputFormat = "output"` if you want the format-side codified too). Reads correctly at registration sites and removes the conceptual collision with cobra's StringVarP "output" literal.  
**Effort:** hours

#### audit-observability

##### `obs:eb479d86:sprintf-attr-value` — sprintf attr value

**Status:** in review — PR #622  
**Severity:** minor  
**Cluster:** field-stability  
**Evidence:** `internal/distribution/okd/setup/upload.go:138-138`  
**Problem:** `p.Log.Info("iso: uploading", "count", ..., "size_mb", fmt.Sprintf("%.1f", totalSizeMB), ...)` pre-renders a float to a string before the slog handler sees it. The c07157e migration swept message-arg fmt.Sprintf but missed this attr-value form. JSON output emits `"size_mb": "123.4"` (string) instead of `"size_mb": 123.4` (number), which breaks downstream tooling that types the field as a number and forces consumers to re-parse.  
**Fix:** Drop the Sprintf wrapper and pass the float directly: `"size_mb", totalSizeMB`. Slog's JSON handler emits float64 with full precision; if the precision-1 rendering is load-bearing for the text formatter, push the rounding down by one level (compute roundedMB := math.Round(totalSizeMB*10)/10 and pass that float). Net 0 LOC.  
**Effort:** hours

#### audit-modernization

##### `mod:ddf885f4:use-slices-contains` — use slices contains

**Status:** not started (already implemented in commit 239959e — needs archive, not pickup)  
**Severity:** minor  
**Cluster:** slices-maps — related: mod:262af6e4:use-slices-contains  
**Evidence:** `internal/addon/manager.go:267-275`  
**Problem:** Manager.dependsOn does a hand-rolled contains check on a.Info().Dependencies before recursing — slices.Contains expresses the early-return cleanly. The same package already imports slices for the rollback path (L183 slices.Backward), so the import is free.  
**Fix:** Hoist the equality short-circuit out: `if slices.Contains(a.Info().Dependencies, target) { return true }`. Then iterate only to recurse: `for _, dep := range a.Info().Dependencies { if m.dependsOn(dep, target, visited) { return true } }`. Approximate even LOC, clearer intent.  
**Effort:** hours

##### `mod:6b533f2d:slices-collect-projection` — slices collect projection

**Status:** not started  
**Severity:** suggestion  
**Cluster:** slices-maps  
**Evidence:** `internal/cluster/k8s_csrs.go:63-66`  
**Problem:** ApprovePendingCSRs builds a names slice via index assignment in a counted-style loop. The same projection shape recurs in update_ingress.go::handleHostNetworkConversion (L292-295) and cleanup.go::KindStrings (L43-50). The repo could land a tiny mapSlice helper in internal/sliceutil and migrate all three, OR keep the make+loop form (already idiomatic Go).  
**Fix:** Optional. The for-i form is already clear. If a third+fourth callsite appear, land a small `mapSlice[T,U](in []T, fn func(T) U) []U` helper. Don't land in isolation — there's no Go 1.x-specific reason to migrate.  
**Effort:** hours

##### `mod:8ea706f6:strings-lines-version` — strings lines version

**Status:** in review — PR #623  
**Severity:** suggestion  
**Cluster:** slices-maps  
**Evidence:** `internal/distribution/okd/setup/tools.go:260-264`  
**Problem:** getToolVersion calls `strings.Split(strings.TrimSpace(string(output)), "\n")` only to read the first line. With strings.Lines the read becomes a single-iteration loop with no allocation; the surrounding `if len(lines) > 0` guard goes away because the iterator yields zero times for an empty TrimSpace result.  
**Fix:** Replace with `for line := range strings.Lines(string(output)) { return strings.TrimSpace(line) }; return "unknown"`. No slice materialisation, ~3 LOC removed.  
**Effort:** hours

##### `mod:c19ee328:slices-containsfunc-ignition` — slices containsfunc ignition

**Status:** not started (superseded by IgnitionSentinel sentinel-file approach — needs archive, not pickup)  
**Severity:** suggestion  
**Cluster:** slices-maps — related: mod:c19ee328:slices-containsfunc-allexist  
**Evidence:** `internal/distribution/okd/setup/steps.go:186-192`  
**Problem:** AlreadyDone for StepGenerateIgnition repeats the same all-files-present pattern: iterate ignitionFilenames, return false on the first missing one. Twin to mod:c19ee328:slices-containsfunc-allexist; both should land together.  
**Fix:** `return !slices.ContainsFunc(ignitionFilenames, func(f string) bool { return !system.FileExists(filepath.Join(clusterDir, f)) }), nil`. Same semantics; matches the prevailing repo idiom.  
**Effort:** hours

#### audit-code-smells

##### `smell:fd2125dd:output-flag-magic-string` — output flag magic string

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/smell-fd2125dd-output-flag-magic-string  
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

##### `smell:39c75e91:yes-no-magic-strings` — yes no magic strings

**Status:** not started (proposed EqualFold fix would break confirm_test.go YES/Yes→false assertions; needs maintainer decision: tighten tests or accept case-insensitive)  
**Severity:** suggestion  
**Cluster:** magic-strings  
**Evidence:** `internal/cli/confirm.go:60-62`  
**Problem:** `isConfirmResponse` encodes the Y/yes/y truthy set as three string literals inline. A second copy with a slightly different vocabulary lives in tui/wizard/datadriven.go (out of audit scope), but the smell at this site is that the canonical 'yes' parse for a destructive op is hand-rolled rather than going through `strconv.ParseBool` or a centralised helper.  
**Fix:** Replace the body with `strings.EqualFold(response, "y") || strings.EqualFold(response, "yes")` for case-insensitive match in one shot. Three literals -> two with no semantic loss; keeps in this package because the wizard parser is intentionally separate (looser vocabulary).  
**Effort:** hours

##### `smell:62cb8a95:state-major-bounds-misnamed` — state major bounds misnamed

**Status:** in review — PR #629  
**Severity:** suggestion  
**Cluster:** magic-strings  
**Evidence:** `internal/distribution/okd/destroy/helpers.go:26-59`  
**Problem:** `stateMajorMin, stateMajorMax = 1, 1` — two constants whose values are identical, named asymmetrically, and used in `if major < stateMajorMin || major > stateMajorMax`. The 'min' / 'max' framing implies a range; the body is in fact 'major must equal 1'. A future widening (terraform v2 support) would change *one* of the two and read like the bound got tighter, not looser. This is the bool-should-be-enum cluster's twin: two scalars where one suffices.  
**Fix:** Replace with `const requiredTerraformMajor = 1` and `if major != requiredTerraformMajor`. When v2 lands, change to a typed range struct then. The current spelling encodes a contract the body does not enforce — a 1.x match check.  
**Effort:** hours

##### `smell:6424733c:metrics-shutdown-timeout-magic` — metrics shutdown timeout magic

**Status:** in review — PR #620  
**Severity:** suggestion  
**Cluster:** magic-strings  
**Evidence:** `internal/cli/helpers.go:228-256`  
**Problem:** `startMetricsServer` hardwires `5*time.Second`, `10*time.Second`, `60*time.Second` as ReadHeader/Read/Write/Idle/Shutdown timeouts. Per-timeout magic numbers in HTTP server construction are a frequent regression class — a future change wanting a tighter or looser default has to read the file to learn the implicit policy. The shutdown 5s in particular shows up as a comment hint, not a constant.  
**Fix:** Lift the four durations to package-level `const metricsReadHeaderTimeout = 5 * time.Second` etc. above startMetricsServer, with a single doc comment explaining 'Prometheus scrapers reconnect every interval; idle 60s leaves slack for slow scrapers'.  
**Effort:** hours

#### audit-dependencies

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

##### `dep:6ebdb617:claudemd-yaml-tripwire-stale` — claudemd yaml tripwire stale

**Status:** not started  
**Severity:** suggestion  
**Cluster:** duplicate-engine  
**Evidence:** `CLAUDE.md:196-201`  
**Problem:** CLAUDE.md §dependencies tripwire claims 'Count is down from four — gopkg.in/yaml.v3 was dropped from go.mod require.' That is technically accurate (it is no longer in the require block) but misleading: gopkg.in/yaml.v3 v3.0.1 still appears in go.sum via testify→gopkg.in/check.v1, so the operative engine count remains four. The tripwire fires only on a fourth direct add, masking the real duplicate.  
**Fix:** Edit CLAUDE.md §dependencies to either (a) say 'four YAML engines: sigs.k8s.io/yaml direct + go.yaml.in/yaml/v{2,3} via apimachinery + gopkg.in/yaml.v3 via testify/check.v1 — keep stable, do not add a fifth' or (b) actually drop testify by replacing test-only assertions with stdlib + go-cmp, then update the tripwire to say three.  
**Effort:** hours

#### audit-documentation

##### `doc:4c092fce:pkg-doc-name-echo` — pkg doc name echo

**Status:** in review — PR #625  
**Severity:** minor  
**Cluster:** package-doc  
**Evidence:** `internal/infrastructure/terraform/terraform.go:1-2`  
**Problem:** Package doc's second sentence is vacuous: 'It can be used by any infrastructure provider that uses Terraform.' That's not a contract — it's a tautology. Replace with what the package actually owns: subprocess shape (PATH lookup, env-allowlist via executor), state-file invariants (atomic writes via system.AtomicWrite), or call surface (Init/Plan/Apply/Destroy/Output).  
**Fix:** Replace the second sentence with a substantive description of the package's actual contract — e.g. 'Operations run via internal/executor with the default env allowlist; state files are written atomically via system.AtomicWrite. Provider packages drive it via Init / Plan / Apply / Destroy / Output.'  
**Effort:** hours

##### `doc:35abd54e:pkg-doc-thin` — pkg doc thin

**Status:** in review — PR #626  
**Severity:** suggestion  
**Cluster:** package-doc  
**Evidence:** `internal/credentials/proxmox.go:1-1`  
**Problem:** Package credentials carries the load-bearing []byte/Zeroize/Redacted-interface contract for credential lifecycle, but the package doc is one vacuous sentence: 'Package credentials provides credential management for infrastructure providers.' The doc should announce the type-level invariants (Password/APIToken []byte for in-memory wipe, Redacted() interface for slog scrubbing, defer Zeroize() lifecycle) so a godoc-reading caller does not have to scan ProxmoxCredentials's type doc to discover them.  
**Fix:** Expand to two-three sentences: 'Package credentials owns the Proxmox credential lifecycle. Password and APIToken are []byte (not string) so callers can defer Zeroize() to wipe them after use; ProxmoxCredentials.Redacted() satisfies logutil.RedactHandler so structured slog attrs never leak. See LoadEnvFile and GetProxmoxCredentials for the env-then-config resolution priority.'  
**Effort:** hours

##### `doc:d5915b0c:pkg-doc-name-echo` — pkg doc name echo

**Status:** in review — PR #627  
**Severity:** suggestion  
**Cluster:** package-doc  
**Evidence:** `internal/distribution/okd/install/phase.go:1-1`  
**Problem:** Package doc echoes the package name with no added signal: 'Package install provides the install phase for OKD cluster provisioning.' The package owns the bootstrap-monitor + CSR-approval + cluster-operator-wait sequence with timeouts (DefaultBootstrapTimeout 30m, DefaultInstallTimeout 60m, DefaultCSRApprovalInterval 30s); the doc should surface the timeline, not the package name.  
**Fix:** Expand: 'Package install drives the install phase: terraform-up, bootstrap monitor, CSR approval, cluster-operator settle. Default timeouts are 30 m for bootstrap, 60 m for the cluster-operator wait, and a 30 s CSR-approval poll cadence; all are overridable via Deployment.* in the Config.'  
**Effort:** hours

#### audit-tests

## Completed

Completed items live in [`docs/roadmap/completed-archive.md`](docs/roadmap/completed-archive.md). Grep there for the canonical "is dep X done?" lookup. The previous in-line pointer index (144 entries, mirroring archive contents) was removed on 2026-05-09 to keep `roadmap.md` focused on active work.

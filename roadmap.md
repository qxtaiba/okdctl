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

##### `api:de572c63:ctx-not-first-write-dnsmasq` — ctx not first write dnsmasq

**Status:** deferred  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/dns/dnsmasq.go:54-92`  
**Problem:** WriteDnsmasqConfig now takes ctx and checks ctx.Err() at entry (progress from prior run), but still does not thread ctx into os.MkdirAll / system.WriteTempFile / system.CopyFile — the body advertises cancellation only via the entry-gate, not per-step. Either plumb ctx into the underlying helpers or select on ctx.Done between the steps.  
**Fix:** Add `select { case <-ctx.Done(): return ctx.Err(); default: }` between the mkdir / WriteTempFile / CopyFile steps so a mid-op cancellation is honored. Alternatively accept the entry-check as sufficient and add a one-line comment explaining why later operations are not gated.  
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

##### `iac:b803fcb7:ci-no-tflint-tfsec` — ci no tflint tfsec

**Status:** deferred  
**Severity:** minor  
**Evidence:** `.github/workflows/ci.yml:97-109`  
**Problem:** `validate-terraform` + `lint-terraform` jobs now run `terraform fmt`, `terraform validate`, and `tflint -f compact` — but no secret/policy scanner (tfsec, checkov, or trivy config). tflint catches terraform_* idiom issues; tfsec/checkov catch misconfigured provider secrets, missing `sensitive = true`, and public-exposure antipatterns that the HCL surface will grow into as the module adds network/firewall rules.  
**Fix:** Add a `tfsec` or `trivy config` step to the validate-terraform/lint-terraform job. tfsec has a maintained action `aquasecurity/tfsec-action@...`; `trivy config infrastructure/terraform` is a single call.  
**Effort:** hours

##### `iac:18a795d5:hcl-no-prevent-destroy-masters` — hcl no prevent destroy masters

**Status:** deferred  
**Severity:** suggestion  
**Evidence:** `infrastructure/terraform/modules/proxmox-okd/main.tf:140-255`  
**Problem:** Master VMs (OKD control plane carrying etcd quorum state) have no `lifecycle { prevent_destroy = true }` guard. A misconfigured `terraform apply` that perturbs a force-new attribute (e.g.  
**Fix:** Add `prevent_destroy = true` to the master VM resource's `lifecycle` block, gated by a variable (e.g. `var.allow_master_destroy`, default false) that `okdctl destroy` flips before running Terraform.  
**Effort:** hours

#### audit-modernization

#### audit-observability

##### `obs:0d318f5c:handler-no-tty-switch` — handler no tty switch

**Status:** deferred  
**Severity:** minor  
**Evidence:** `internal/cli/logging.go:35-67`  
**Problem:** configureLogging still does not auto-select JSON format when stderr is not a TTY. Operators piping `okdctl deploy 2>&1 | jq .` get charmlog text with ANSI escapes by default and must remember `--log-format json`.  
**Fix:** Route cobra's cmd into configureLogging so `cmd.Flags().Changed("log-format")` is available. If not set and !stderrIsTTY, default logFormat to "json".  
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

##### `sec:00000005:bootstrap-oc-no-integrity` — bootstrap oc no integrity

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/release_extract.go:24-76`  
**Problem:** bootstrapOC downloads oc.tar.gz from mirror.openshift.com with no checksum or cosign signature verification. The docstring admits 'no upstream checksum is published for this URL; post-extraction binary-exists verification is the integrity gate'.  
**Fix:** Either (a) pin bootstrapOCURL to a specific release tag and ship a baked-in sha256 in the okdctl binary (matches the 'explicit versions — never @latest' rule in CLAUDE.md §Dependencies), or (b) verify a cosign signature on the tarball if Red Hat publishes one for the client tarball set, or (c) fall through to `oc adm release extract` via the distribution-packaged `openshift-client` rpm/deb instead of curl-to-bash. Document the trust decision in CLAUDE.md §security-invariants.  
**Effort:** hours

##### `sec:00000006:debug-bundle-redact-partial` — debug bundle redact partial

**Status:** deferred  
**Severity:** minor  
**Evidence:** `internal/cli/config.go:65-79`  
**Problem:** redactConfig in cli/config.go only masks Provider.Proxmox.TokenID and leaves every other config field unchanged. Password and APIToken carry `json:"-"` so they never marshal into the bundle (correct today), but the function signature encourages a future 'add a field, forget to redact' regression.  
**Fix:** Walk the config via reflection and mask every string field whose struct-tag name matches the RedactHandler denylist (password, token, secret, api_key, apikey). Alternative: add an explicit `okdctl:"sensitive"` struct tag and have redactConfig honour it — future fields opt in by tagging.  
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

##### `tst:41a9d4eb:no-test-redact-handler` — no test redact handler

**Status:** deferred  
**Severity:** major  
**Evidence:** `internal/logutil/redact.go:30-123`  
**Problem:** RedactHandler is the canonical slog redaction middleware — CLAUDE.md §credentials-and-secrets explicitly calls it out as the mechanism "so credentials in structured attrs never reach the sink". Its direct unit tests are absent; coverage today is indirect via tui/logger_test.go.  
**Fix:** Add internal/logutil/redact_test.go with stdlib testing + bytes.Buffer + slog.NewTextHandler as the wrapped inner. Cases: (1) TestRedactAttr_SecretKeys — feed password/PASSWORD/api_token/bearer_token; assert all replaced with "[redacted]"; (2) TestRedactAttr_NonSecret — cluster/user (non-secret) pass through; (3) TestRedactAny_URL — *url.URL with User=url.UserPassword("u","p") → output has u@ but no :p@; (4) TestRedactAny_RedactedInterface — struct with Redacted() any returning "<masked>" → replaced; (5) TestWithAttrs_RedactsDerivedLogger — logger.With("password", "x").Info(...) → output has [redacted], never "x"; (6) TestWithGroup — group propagation preserves redaction; (7) TestGroupKind — nested slog.Group with a secret key inside is redacted.  
**Effort:** days

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


##### `sec:15ba17da:cred-no-zeroize` — cred no zeroize

**Status:** not started  
**Severity:** minor  
**Cluster:** credentials — related: sec:6424733c:cred-no-zeroize  
**Evidence:** `internal/distribution/okd/destroy/steps.go:24-133`  
**Problem:** Destroy cleanup uses opts.SkipFirewall / opts.SkipCleanup / opts.SkipTerraform flags wired from the CLI. The credential lifecycle on destroy: handleCredentials creates ProxmoxCredentials, defers creds.Zeroize, then plumbs creds.Env() into createOKDProvisionerWithOpts. Same Env() string-residue issue as the deploy path (sec:6424733c:cred-no-zeroize) — destroy holds the credential strings on the executor for its full duration. Less long-running than deploy (terraform destroy is faster), but the credential is held for the entire teardown sequence including ssh-based ISO removal.  
**Fix:** Companion fix to sec:6424733c:cred-no-zeroize. Once a ZeroizeEnv helper exists on the provisioner, destroy.go calls it in the same defer chain.  
**Effort:** hours

##### `sec:27088eab:input-kubeconfig-not-resolved` — input kubeconfig not resolved

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-27088eab-ssh-known-hosts  
**Severity:** minor  
**Cluster:** input-validation  
**Evidence:** `internal/distribution/okd/phase/ssh.go:29-41`  
**Problem:** SSHRun uses `-o StrictHostKeyChecking=accept-new` everywhere (uploads, ISO removal, custom commands), which is TOFU. There is no provision for a per-cluster known_hosts file and no enforcement that the Proxmox host fingerprint match an operator-pinned value. A first-deploy MITM permanently locks in an attacker's host key; the destroy path also relies on this same SSH transport and inherits the trust.  
**Fix:** Add an opt-in `proxmox.host_fingerprint` config field (sha256-of-pubkey form). When set, run `ssh-keyscan` once at first contact, validate the fingerprint matches the configured value, write to a per-project known_hosts file, and pass `-o StrictHostKeyChecking=yes -o UserKnownHostsFile=<path>` for every subsequent ssh/scp call. accept-new should be the explicit fallback only when the fingerprint is unset.  
**Effort:** hours

##### `sec:0d318f5c:cred-no-zeroize` — cred no zeroize

**Status:** not started  
**Severity:** suggestion  
**Cluster:** credentials  
**Evidence:** `internal/cli/logging.go:35-67`  
**Problem:** configureLogging is called by PersistentPreRunE — including for the deploy/destroy/cleanup/update-ingress paths under sudo re-exec. If --log-file is set and the file already exists, configureLogging opens it append-mode 0o600. Under sudo re-exec, the file existed before the re-exec (created by the unprivileged invocation) and is now opened by root. Subsequent log lines (which include redacted attrs but also raw env / path strings) get appended to a file the user owns. After the re-exec returns, the file is still user-owned — no chown back needed because root only appended. ...  
**Fix:** Already-hardened. Documenting as a counter-example reference: this file (cli/logging.go:25-32) is the canonical pattern other sites in this audit reference for O_NOFOLLOW + lstat. No action needed.  
**Effort:** hours


##### `sec:8ea706f6:cred-no-zeroize` — cred no zeroize

**Status:** not started  
**Severity:** suggestion  
**Cluster:** credentials  
**Evidence:** `internal/distribution/okd/setup/tools.go:227-248`  
**Problem:** installHashiCorpDebianRepo writes the GPG key to a temp file via system.WriteTempFile with mode 0o600 then runs `gpg --dearmor -o /usr/share/keyrings/...gpg`. The temp-file handler closes the file before gpg reads it (per WriteTempFile semantics), but the os.Remove on defer happens after gpg has succeeded. The dearmored output goes to /usr/share/keyrings (world-readable 0o644 by gpg's default) — not a defect since it's a public key, but the original tmp may briefly carry the armored key in /tmp where any local user could race it via inotify before defer cleanup. Minor — sam...  
**Fix:** Acceptable as-is — the GPG key is public. Document the cleanup contract for symmetric WriteTempFile usage.  
**Effort:** hours

##### `sec:8ea706f6:input-validation` — input validation

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-8ea706f6-rhel-repo-pin  
**Severity:** suggestion  
**Cluster:** input-validation  
**Evidence:** `internal/distribution/okd/setup/tools.go:129-133`  
**Problem:** installTerraform on RHEL: the repoURL `https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo` is hardcoded. dnf config-manager --add-repo trusts the repo file fetched at this URL — the file declares the gpgkey URL inside it. dnf signature-check then validates packages with that gpgkey. The chain is: HTTPS-trust-on-fetch → gpgkey-trust-on-fetch → package-signature. The first link is HTTPS-only; no signature on the .repo file itself.  
**Fix:** Embed the .repo file content in the binary and write it via WriteAsInvokingUser to /etc/yum.repos.d/hashicorp.repo with the gpgkey URL pinned to a HashiCorp-controlled HTTPS path. Removes the on-the-fly fetch step entirely. Same pattern as the deb-side installHashiCorpDebianRepo, just consistent across families.  
**Effort:** hours


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

##### `api:8e65d574:iface-in-producer` — iface in producer

**Status:** not started  
**Severity:** suggestion  
**Cluster:** interface-location  
**Evidence:** `internal/distribution/okd/install/monitor.go:52-56`  
**Problem:** csrApprover is defined consumer-side (good — Go idiom), but cluster.K8sClient (the producer) returns a concrete type that requires NewK8sClient(...) at the call site. The producer-side option struct WithCLI/WithKubeconfig/WithLogger is fine; the issue is that this consumer-side interface is unique to monitor.go yet ApprovePendingCSRs is exactly the kind of operation that other phases (postinstall verify, status) already use through the same K8sClient. If/when a second consumer needs the same shape, they'll re-declare it.  
**Fix:** Leave as-is for now (single consumer = correct Go idiom). Watch for a second consumer; promote to internal/cluster.CSRApprover only when a second site declares the same shape. Filing a tracking item is enough.  
**Effort:** hours

#### audit-cli-ux

#### audit-modernization

#### audit-code-smells

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


##### `dep:33ef32bf:transitive-narrow-uuid` — transitive narrow uuid

**Status:** not started  
**Severity:** suggestion  
**Cluster:** transitive-weight — seam→audit-modernization  
**Evidence:** `go.mod:12-12`  
**Problem:** `github.com/google/uuid v1.6.0` is a direct dep used in three files (`internal/cli/{deploy,destroy,debug_bundle}.go`) at three call sites — all `uuid.NewString()` for run-IDs / bundle-IDs. UUID v4 from `crypto/rand` is ~10 LOC stdlib. Flagging as suggestion only because the dep is small, BSD-3-clause, well-maintained, and the savings are marginal (one dep entry). Worth listing because it falls inside CLAUDE.md's `check whether stdlib covers it` policy.  
**Fix:** Optional: replace with `crypto/rand` + `fmt.Sprintf` UUIDv4 helper in `internal/system` (~15 LOC) and drop the dep. Risk is low because UUIDs are non-load-bearing here (run-ID telemetry only, not security tokens). Seam to audit-modernization. Lower priority than godotenv because google/uuid is a stable, widely-vendored dep with no maintenance signal issues.  
**Effort:** hours

#### audit-documentation

#### audit-tests

### Tier I — findings from 2026-05-05 /audit-all run

Filed by the orchestrator aggregation so `/roadmap-pickup` can fan them out when bandwidth opens. Each references the audit finding ID for diff tracking; when a finding recurs in a later run, its entry Status+Evidence updates here rather than being duplicated. Total: 171 findings (3 blocker, 32 major, 77 minor, 59 suggestion).


#### audit-security

##### `sec:06f00bcb:ignition-pullsecret-served-unauth` — ignition pullsecret served unauth

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-06f00bcb-ignition-unauth  
**Severity:** major  
**Cluster:** credentials  
**Evidence:** `internal/distribution/okd/setup/apache.go:175-188`  
**Problem:** bootstrap.ign / master.ign / worker.ign embed the OKD pull-secret JSON and are deployed to /var/www/html/ignition with 0o640 owned by apache, served by Apache on http://<bastion>:8080/ignition without authentication. Anyone on the machine network can fetch them during the bootstrap window (15-30 min) and harvest the pull secret. Apache is started with default vhost (binds 0.0.0.0:8080).  
**Fix:** Constrain Apache's ignition vhost to bind to the bastion's bridge IP (the IP FCOS nodes resolve from the kargs URL), not 0.0.0.0. Add iptables/firewalld INPUT rules limiting :8080 to machineCIDR. Optionally rotate to per-node ignition URLs via path tokens (`/ignition/<token>/master.ign`) created at apply time and dropped after bootstrap. Document the residual exposure window in README §security-considerations.  
**Effort:** hours

##### `sec:21dc1103:download-no-nofollow` — download no nofollow

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-21dc1103-download-nofollow  
**Severity:** major  
**Cluster:** file-toctou  
**Evidence:** `internal/download/download.go:140-148`  
**Problem:** Download.fetchToFile opens OutputPath with O_CREATE|O_WRONLY|O_TRUNC but no syscall.O_NOFOLLOW. Tools/setup callers pass tempdir paths or workdir paths; under the sudo re-exec model the open runs as root, so a pre-created symlink at OutputPath would be followed and binary content written through it. extract.go already uses O_NOFOLLOW on archive entries (good); this site is the asymmetric counterpart on the same package.  
**Fix:** Add syscall.O_NOFOLLOW to the OpenFile flags. If callers ever need to overwrite a real file at OutputPath, the symlink case should be a hard error — Lstat first or document the symlink rejection in Download's contract.  
**Effort:** hours

##### `sec:27088eab:ssh-strict-host-key-tofu` — ssh strict host key tofu

**Status:** not started  
**Severity:** minor  
**Cluster:** tls-network — seam→audit-subprocess — related: sec:eb479d86:scp-strict-host-key-tofu  
**Evidence:** `internal/distribution/okd/phase/ssh.go:31-57`  
**Problem:** SSHRun and SSHRunArgv install StrictHostKeyChecking=accept-new for every Proxmox-bastion ssh hop (pvesh, rm -f, sha256sum). Same TOFU concern as scp upload — paired with sec:eb479d86, this is the policy-level finding for the SSH side.  
**Fix:** Same fix as sec:eb479d86 — surface a per-cluster known_hosts pinning option. Replace accept-new with strict mode once a verified known_hosts line is established; the wizard could ssh-keyscan + display the fingerprint to the operator on first run for confirmation.  
**Effort:** hours

##### `sec:5013fea6:bootstrap-oc-no-signature` — bootstrap oc no signature

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-5013fea6-bootstrap-oc-pin  
**Severity:** minor  
**Cluster:** tls-network  
**Evidence:** `internal/distribution/okd/setup/release_extract.go:29-91`  
**Problem:** bootstrapOC fetches openshift-client from https://github.com/okd-project/okd-scos/releases/download/<bootstrapOCVersion>/<asset>.tar.gz and verifies via sha256sum.txt fetched from the same release URL. The sha256sum.txt has no signature (cosign or otherwise) — a release-asset compromise that swaps both the tarball and the sums file would not be caught. okdctl's own release pipeline ships cosign signatures (good); the bootstrap-oc dependency does not.  
**Fix:** Pin the SHA-256 of the bootstrap oc tarball at compile time (analogous to yqChecksumsByArch in setup/tools.go). Then sha256sum.txt becomes defense-in-depth, not the trust root.  
**Effort:** hours

##### `sec:8ea706f6:tools-tempdir-non-canonical` — tools tempdir non canonical

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-8ea706f6-tools-tempdir  
**Severity:** minor  
**Cluster:** file-toctou — related: sec:21dc1103:download-no-nofollow  
**Evidence:** `internal/distribution/okd/setup/tools.go:203-216`  
**Problem:** installBinary uses os.CreateTemp + immediate Close + later download.Download writing to that path. Bypasses the canonical system.WriteTempFile per CLAUDE.md §architecture-notes. Combined with sec:21dc1103 (download.Download lacks O_NOFOLLOW), the create-then-rewrite path runs as root with a temp file mode established by os.CreateTemp's default — fine today but fragile against future caller errors.  
**Fix:** Refactor to system.WriteTempFile so all binary downloads thread through the canonical helper. Pair with sec:21dc1103 (add O_NOFOLLOW in download.fetchToFile).  
**Effort:** hours

##### `sec:98723e5d:helm-set-cred-via-argv` — helm set cred via argv

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-98723e5d-helm-values-file  
**Severity:** minor  
**Cluster:** credentials  
**Evidence:** `internal/distribution/okd/install/flux.go:146-159`  
**Problem:** installInstance passes fs.Repository (operator-supplied) and other settings via `--set instance.sync.url=<url>` to helm. helm reads these as argv; --set values land in the helm-controller pod's process listing and are stored verbatim in the helm release secret. ValidateSettings forbids URLs with userinfo (good) but the URL itself may still encode tokens for non-https schemes (e.g. ssh://git+token@host). Defense is brittle.  
**Fix:** Render a Values YAML to a 0o600 temp file and pass `-f <values.yaml>` instead of multiple --set flags. Removes credential-shaped values from /proc/<pid>/cmdline and from helm release secrets. ValidateSettings can keep its current scheme/userinfo gate.  
**Effort:** hours

##### `sec:d9f7733e:debug-bundle-tar-readall` — debug bundle tar readall

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-d9f7733e-bundle-stream  
**Severity:** minor  
**Cluster:** file-toctou  
**Evidence:** `internal/cli/debug_bundle.go:301-329`  
**Problem:** tarDirInto walks srcDir and reads each regular file with io.ReadAll into memory. The os.Root protection (good) prevents symlink redirection out of srcDir, but unbounded ReadAll on must-gather output can OOM the process — must-gather routinely emits multi-GB dumps. The bundleMustGather caller has a 5-minute timeout but no size cap.  
**Fix:** Replace io.ReadAll with io.LimitReader-bounded streaming via tw.WriteHeader + io.Copy, or per-file size cap (e.g. 50MB). Also tag oversized files in the manifest so support engineers see why a file was truncated.  
**Effort:** hours

##### `sec:25fa1be8:firewall-haproxy-port-only-tcp` — firewall haproxy port only tcp

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/sec-25fa1be8-firewall-allowlist  
**Severity:** suggestion  
**Cluster:** input-validation  
**Evidence:** `internal/distribution/okd/firewall/firewall.go:52-66`  
**Problem:** haproxyPortNumbers is map[int]bool but HAProxyFrontendPorts() filters by both Number and Protocol == protoTCP. The rule is correct but the structure leaves a future-maintainer footgun: if anyone adds a UDP rule on those numbers (e.g. UDP 6443 for QUIC variants), HAProxyFrontendPorts() would silently include them only when their tcp filter passes. A pure allowlist of {Number, Protocol} pairs would be unambiguous.  
**Fix:** Define haproxyFrontends as []Port literal (number+protocol pairs) instead of map[int]bool keyed only by number. Removes the implicit tcp-only assumption.  
**Effort:** hours


#### audit-subprocess

##### `sub:25fa1be8:bypass-canonical-wrapper-ufw` — bypass canonical wrapper ufw

**Status:** not started  
**Severity:** minor  
**Cluster:** io-handling  
**Evidence:** `internal/distribution/okd/firewall/firewall.go:91-106`  
**Problem:** DetectBackend probes ufw with a raw exec.CommandContext + cmd.Output() and a manual *exec.ExitError unwrap, while every other shellout in the same file (Configure, modifyPort, etc.) goes through system.RunCaptured / system.OutputCaptured. The canonical wrappers already filter env through executor.DefaultEnvAllowlist, capture stderr into the returned error, and centralise diagnostics — bypassing them on this one path inherits the parent shell's env (including AWS_*/GH_TOKEN/GITHUB_TOKEN tokens that the allowlist drops).  
**Fix:** Replace with output, err := system.OutputCaptured(ctx, ufw, status); on err, log via the same debug fall-through (system.OutputCaptured already wraps stderr into the returned error). Same env-filter and stderr-into-err semantics as the rest of this file.  
**Effort:** hours

##### `sub:8ea706f6:coreutil-lsb-release` — coreutil lsb release

**Status:** not started  
**Severity:** minor  
**Cluster:** coreutils-shellout  
**Evidence:** `internal/distribution/okd/setup/tools.go:321-326`  
**Problem:** installHashiCorpDebianRepo shells out to lsb_release -cs to read the Debian codename, but /etc/os-release VERSION_CODENAME exposes the same value and is already parsed by internal/platform/platform.go (Detect). lsb_release is a Python script that ships separately on Debian/Ubuntu — relying on it adds a runtime dep and a fork+exec for data already in memory.  
**Fix:** Add a VERSION_CODENAME field to platform.Detect()'s output (it already parses /etc/os-release) or expose a small parseOSReleaseField(VERSION_CODENAME) helper, then call it here. Removes the lsb_release dep — older Debian/RHEL installations don't ship it by default.  
**Effort:** hours

#### audit-state-and-recovery

##### `state:4f69fc9d:rerunsafe-not-enforced` — rerunsafe not enforced

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/state-4f69fc9d-rerunsafe  
**Severity:** major  
**Cluster:** phase-idempotency  
**Evidence:** `internal/distribution/step.go:228-251`  
**Problem:** BuildSteps panics on ReRunSafeUnset but never propagates the value to builtStep, and Orchestrator.Run never queries it. ReRunSafeNo is decorative metadata — a step is still re-executed on a fresh run unless the StepDef ALSO supplies AlreadyDone. Out of 5 ReRunSafeNo steps in the codebase, only one (postinstall.StepCleanupBootstrap) provides a precondition guard.  
**Fix:** Either (a) make ReRunSafeNo + missing AlreadyDone fail BuildSteps so authors must wire a precondition, or (b) require a logger.Warn at orchestrator entry for ReRunSafeNo steps without AlreadyDone. Option (a) gives the contract teeth — every ReRunSafeNo step gets an AlreadyDone or the build panics. Cross-reference roadmap state:4f69fc9d (deferred).  
**Effort:** hours

##### `state:c19ee328:setup-iso-build-not-resumable` — setup iso build not resumable

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/state-c19ee328-iso-resumable  
**Severity:** major  
**Cluster:** crash-recoverability  
**Evidence:** `internal/distribution/okd/setup/steps.go:212-240`  
**Problem:** StepBuildISOs and StepUploadISOs are ReRunSafeNo but neither declares AlreadyDone. A SIGKILL mid-ISO-build leaves a partial fedora-coreos-master0.iso in workDir; mid-upload leaves a partial ISO on the Proxmox host. The next deploy re-runs the orchestrator from step 1; cleanup.WorkOnly removes custom-isos/ locally, but the partial remote upload is never sha256-verified before being referenced by the cdrom block in the bootstrap/master/worker resources.  
**Fix:** Add AlreadyDone to StepBuildISOs that verifies SHA256 of every expected output ISO matches the cached fedora-coreos image hash; on mismatch, treat as 'work not done' and re-build. For StepUploadISOs, add AlreadyDone that runs `pvesh get /nodes/<node>/storage/<storage>/content` filtered to iso/<expected-name>.iso and verifies the size matches the local ISO. Either pre-condition stops a half-uploaded artifact from being treated as ready.  
**Effort:** hours

#### audit-iac-and-shell

##### `iac:e076e43c:sh-doc-line-stale` — sh doc line stale

**Status:** not started  
**Severity:** suggestion  
**Cluster:** install-sh-integrity — related: iac:e076e43c:sh-bash-array-dash-incompat  
**Evidence:** `scripts/install.sh:21-21`  
**Problem:** The script's `Requires:` docstring lists `bash, curl, tar, sha256sum` — but does not mention that the script ALSO requires bash specifically (not sh) when invoked via `curl | sh`. Combined with the array-syntax issue (separate finding), users on Debian/Ubuntu reading only the README never learn the dependency until the script crashes.  
**Fix:** If the bash-array fix is option (a) `| bash`, this finding resolves automatically when the README and docstring update. If option (b) (POSIX-compatible auth), this finding becomes moot. Either way, no separate fix needed once `iac:e076e43c:sh-bash-array-dash-incompat` is addressed.  
**Effort:** hours

#### audit-errors

#### audit-concurrency

#### audit-api-design

##### `api:4f69fc9d:rerunsafe-declarative-only` — rerunsafe declarative only

**Status:** not started  
**Severity:** major  
**Cluster:** exported-surface — seam→audit-state-and-recovery  
**Evidence:** `internal/distribution/step.go:190-251`  
**Problem:** StepDef.ReRunSafe is enforced at declaration time (BuildSteps panics on ReRunSafeUnset) but never consulted at execution time — StepBuilder discards it and Orchestrator.Run never branches on it. The field is decorative metadata that the type system mandates without backing semantics, so a ReRunSafeNo step is treated identically to a ReRunSafeYes step on a partial-fail-and-resume.  
**Fix:** Either (a) propagate ReRunSafe into builtStep and have Orchestrator.executeStep skip ReRunSafeNo steps that have no AlreadyDone hook on a recovery rerun, or (b) demote the field to a doc-only label and replace the BuildSteps panic with a //nolint comment + roadmap entry. Today's setup phase has 6 ReRunSafeNo steps with no AlreadyDone — option (a) requires writing those checks; option (b) is honest about the current state.  
**Effort:** hours

##### `api:21dc1103:options-struct-vs-functional` — options struct vs functional

**Status:** not started  
**Severity:** minor  
**Cluster:** option-consistency  
**Evidence:** `internal/download/download.go:24-113` + 2 more  
**Problem:** download.Options, download.ExtractOptions, and cleanup.Options pass an optional *slog.Logger as a struct field with a getter (o.logger() / opts.getLogger()), while every sibling — executor.WithLogger, terraform.WithLogger, proxmox.WithLogger, addon.WithLogger, cluster.WithLogger, phase.WithLogger — uses functional options. The struct-field shape forces every Download/Extract/cleanup call site to allocate a struct-with-pointer and routes nil through a getter, while the option shape has logutil.OrNop applied once at construction.  
**Fix:** Either (a) commit the codebase to options-struct everywhere — uniform but loses at-construction nil-normalisation; or (b) commit to functional options — replace download.Download(ctx, *Options) with download.Fetch(ctx, url, dst string, opts ...Option). Recommend (b): three of the four call sites set ≤2 fields, so the per-call-site delta is small and matches the prevailing pattern. cleanup.Options stays a struct (its fields are required workdir/path data, not optional knobs) but Logger should move to a functional WithLogger.  
**Effort:** hours

##### `api:beabab0c:phase-new-positional-args` — phase new positional args

**Status:** not started  
**Severity:** minor  
**Cluster:** option-consistency — related: api:c287d5c0:public-fields-bypass-options  
**Evidence:** `internal/distribution/okd/setup/phase.go:102-109` + 4 more  
**Problem:** All five sibling phase constructors take positional (exec, logger, version) args, while their shared base phase.NewBasePhase and the parent okd.New use functional options (WithExecutor/WithLogger/WithVersion, ProvisionerOption). The split forces each phase.New to internally translate positional args into option calls and prevents callers from passing optional knobs (Recorder, Reporter) at construction — they end up writing exported fields directly (okd.go:L138, L147-L148, L156).  
**Fix:** Pick one shape for the okd phase family. Recommend functional options (matches NewBasePhase + okd.Provisioner): replace setup/install/postinstall/destroy/cleanup New(exec, logger, version) with New(version, ...PhaseOption) using WithExecutor/WithLogger/WithRecorder/WithReporter shared via phase package. okd.Provisioner.Prepare/Install/etc become single-line forwarders. Net delta ~+15 LOC of option types, -20 LOC of struct-field writes in okd.go.  
**Effort:** hours

#### audit-cli-ux

##### `ux:d31d1b9d:describe-format-unvalidated` — describe format unvalidated

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/ux-d31d1b9d-describe-format  
**Severity:** major  
**Cluster:** json-stability  
**Evidence:** `internal/cli/status.go:272-376` + 1 more  
**Problem:** `runDescribeNode` and `runDescribeAddon` accept `--format=json|text` but skip both `validateFormat(describeFormat)` and `quietForJSON(describeFormat)`. Result: `okdctl describe node x --format=foo` silently falls through to text mode (no error), and `okdctl describe node x --format=json` mixes `tui.Info` chatter from `loadConfig` / `resolveProjectRootOrDie` into the same fd as the JSON document — `2>&1 | jq` breaks. `runStatus` and `runReleasesList`/`Show` get this right; describe is the outlier.  
**Fix:** At the top of `runDescribeNode` and `runDescribeAddon` add `if err := validateFormat(describeFormat); err != nil { return err }` then `quietForJSON(describeFormat)`. Promote `describeFormat` to two separate vars (`describeNodeFormat`, `describeAddonFormat`) — sharing one package global is brittle. Add a table-driven test that exercises every `--format=foo` value (`text`, `json`, garbage) for both subcommands.  
**Effort:** hours

##### `ux:e7db1220:json-release-type-as-int` — json release type as int

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/ux-e7db1220-release-type-json  
**Severity:** major  
**Cluster:** json-stability  
**Evidence:** `internal/distribution/okd/releases/types.go:31-40` + 1 more  
**Problem:** `OKDVersion.Type` is `ReleaseType int` with no `MarshalJSON`, so `okdctl releases list --format=json` emits `"release_type": 0` (raw enum int). The published JSON schema in docs/cli/json-schema.md documents the field as the string `"stable"`/`"prerelease"`. Live output drifts from the documented contract.  
**Fix:** Add `MarshalJSON` to `releases.ReleaseType` returning the same labels as `releaseTypeLabel(t)` (`stable`, `latest-stable`, `preview`, `latest-preview`, `lts`). Or change the field type to `string` and store the label directly. Optional: add `UnmarshalJSON` for symmetry with the cache file. Add a snapshot test that asserts the JSON byte-for-byte against the documented schema.  
**Effort:** hours

##### `ux:8154ab0f:doctor-no-machine-format` — doctor no machine format

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/ux-8154ab0f-doctor-json  
**Severity:** minor  
**Cluster:** json-stability  
**Evidence:** `internal/cli/doctor.go:70-113` + 1 more  
**Problem:** `okdctl doctor` is the documented preflight that scripts and CI need to gate on, but it ships text-only — no `--format=json`. The published exit-code contract says `okdctl doctor` returns 2 on fail and 0 otherwise; consumers that want per-check granularity (CI dashboards, the `okdctl debug-bundle` collector itself) have to scrape ANSI-coloured tabwriter output. The same data already flows through internal `checkResult` structs that are JSON-shapeable.  
**Fix:** Add `--format=text|json` to `doctorCmd` (Linux side; the stub stays as-is since doctor is Linux-only). Promote `checkResult` and `checkItem` to JSON-tagged exported types (or local-to-cli copies); on `--format=json` emit `{"checks":[{"name":"host os","severity":"ok","detail":"..."},...],"failed":N,"warned":N}`. Document the schema in `docs/cli/json-schema.md`. Update `internal/cli/debug_bundle_doctor.go` to call `--format=json --log-format=json` so the bundle stores structured doctor output instead of ANSI text.  
**Effort:** hours

##### `ux:aa84670c:exit-code-package-doc-drift` — exit code package doc drift

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/ux-aa84670c-exit-code-doc  
**Severity:** minor  
**Cluster:** exit-codes — seam→audit-errors  
**Evidence:** `internal/cli/root.go:1-12`  
**Problem:** The package doc claims `invoked-as-root rejection=77 (EX_NOPERM, set in cmd/okdctl/main.go)` but `cmd/okdctl/main.go` never sets exit 77 — root rejection in `ensureRoot` (`elevation.go:88-92`) returns `errtypes.AuthError`, which `exitCodeFor` maps to 5. The user-facing `docs/cli/exit-codes.md` correctly documents 5; the canonical anchor comment in `root.go` is stale and is the file `exit-codes.md` points at as source of truth.  
**Fix:** Drop the `rejection=77 (EX_NOPERM, set in cmd/okdctl/main.go)` clause from the package doc on `internal/cli/root.go`. Replace with: `invoked-as-root rejection → 5 (AuthError, in elevation.go ensureRoot)`. Optionally promote 77/EX_NOPERM as a real exit code by routing the `elevReject` decision through a sentinel `errtypes.ErrInvokedAsRoot` and adding `errors.Is(err, errtypes.ErrInvokedAsRoot) → 77` to `exitCodeFor` — that matches BSD sysexits semantics and is what the comment originally intended.  
**Effort:** hours

##### `ux:aa84670c:version-printf-not-via-cmd-out` — version printf not via cmd out

**Status:** not started  
**Severity:** suggestion  
**Cluster:** streams  
**Evidence:** `internal/cli/root.go:208-215` + 3 more  
**Problem:** `versionCmd.Run` writes via `fmt.Printf(...)` directly to `os.Stdout`, ignoring `cmd.OutOrStdout()`. Same pattern across deploy/destroy/cleanup/helpers using package-global `fmt.Println`. This makes cobra-test `cmd.SetOut(buf); cmd.Execute()` impossible — every test that wants to assert command output has to swap `os.Stdout` globally (and most tests in this repo do exactly that). Cobra's idiomatic shape is `fmt.Fprintln(cmd.OutOrStdout(), ...)`.  
**Fix:** Migrate every `fmt.Println(X)` and `fmt.Printf(X...)` site inside a cobra `Run`/`RunE` to `fmt.Fprintln(cmd.OutOrStdout(), X)` / `fmt.Fprintf(cmd.OutOrStdout(), X...)`. Sites outside RunE (e.g. summary builders) take a writer argument. The few legitimate stderr writes (e.g. `kubeconfig.go:72,123`) already use `fmt.Fprintf(os.Stderr, ...)` and are correct.  
**Effort:** hours

##### `ux:d31d1b9d:describe-format-shared-global` — describe format shared global

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/ux-d31d1b9d-describe-format-split  
**Severity:** suggestion  
**Cluster:** flag-conventions  
**Evidence:** `internal/cli/status.go:62-67`  
**Problem:** `describeNodeCmd` and `describeAddonCmd` both write into the same package-level `describeFormat` string. cobra parses each subcommand independently, so this works in single-shot CLI usage. But: tests that run subcommands in sequence (`runDescribeNode` then `runDescribeAddon` in the same process) inherit stale state, and adding a third describe subcommand later will compound the problem. The other `--format` sites use per-command state (`statusFormat`, `releasesListFormat`, `releasesShowFormat`).  
**Fix:** Split `describeFormat` into `describeNodeFormat` and `describeAddonFormat` to match `releasesListFormat`/`releasesShowFormat` (releases.go:25-29). One ten-line change.  
**Effort:** hours

##### `ux:e7db1220:format-vs-output-flag-name-drift` — format vs output flag name drift

**Status:** not started  
**Severity:** suggestion  
**Cluster:** flag-conventions  
**Evidence:** `internal/cli/releases.go:76-82` + 1 more  
**Problem:** Format-selector flags are named `--format` on `releases list/show`, `status`, `describe node/addon`. Output-destination flags are `--output`/`-o` on `deploy`, `kubeconfig`, `debug-bundle`. kubectl/oc convention treats `-o/--output` as the format selector (`-o json`, `-o yaml`); okdctl splits the conventional `-o` namespace and surprises kubectl-fluent users who type `okdctl status -o json` and get a usage error.  
**Fix:** Option A (recommended pre-1.0): rename `--format` to `--output`/`-o` everywhere it acts as a format selector; rename the file-destination uses to `--output-file` so the `-o` shorthand is reserved kubectl-style. Option B (zero-break): add `--output`/`-o` as a hidden alias for `--format` on status/describe/releases; document one canonical name. Pick before 1.0; add the chosen convention to CLAUDE.md §architecture-notes.  
**Effort:** hours


#### audit-observability

##### `obs:41a9d4eb:redact-handler-struct-fields-passthrough` — redact handler struct fields passthrough

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/obs-41a9d4eb-redact-struct  
**Severity:** major  
**Cluster:** redaction-sink — seam→audit-errors  
**Evidence:** `internal/logutil/redact.go:91-112`  
**Problem:** redactAny only catches *url.URL/url.URL with userinfo and types implementing Redacted(); does NOT catch raw structs whose fields are credential-bearing (e.g. `slog.Any("creds", credentials.ProxmoxCredentials{Password: pwd})`). The CLAUDE.md guarantee — slog records pass through internal/logutil.RedactHandler ... attrs whose keys contain password/token/secret/api_key are rewritten to [redacted] — applies only when the credential is the direct value of a secret-keyed attr, not when it is a field inside a struct passed under a benign key like 'creds' or 'config'.  
**Fix:** Two complementary defences: (1) verify credentials.ProxmoxCredentials implements Redacted() any (audit-errors should already cover this); (2) add a type-switch case for known credential types in redactAny (`case *credentials.ProxmoxCredentials:` returning a redacted clone) as defence-in-depth — caller mistakes do not leak. Document the contract: 'credential types must implement Redacted()' in logutil/redact.go package doc.  
**Effort:** hours

##### `obs:6424733c:fmt-sprintf-message-pattern` — fmt sprintf message pattern

**Status:** not started  
**Severity:** major  
**Cluster:** field-stability  
**Evidence:** `internal/cli/helpers.go:32-296` + 5 more  
**Problem:** Pervasive `tui.Info(fmt.Sprintf(...))` / `p.Log.Info(fmt.Sprintf(...))` pattern across ~50 sites in cli/ and distribution/okd/. Stringifying interpolated values into the message text bypasses RedactHandler structured-attr scrub (which only rewrites attr keys/values), kills slog-handler ability to filter by field, and prevents downstream JSON pipelines from extracting the dynamic value (path, cluster name, count, duration). CLAUDE.md is explicit: prefer structured attrs over fmt.Sprintf so the handler can inspect values.  
**Fix:** Mechanical sweep: every `tui.X(fmt.Sprintf("prefix: %s", v))` becomes `tui.X("prefix", tui.LF("key", v))`; every `p.Log.X(fmt.Sprintf("prefix: %s", v))` becomes `p.Log.X("prefix", "key", v)`. ~50 sites; one PR per package keeps churn reviewable. Roll-up message stays static; values move to attrs.  
**Effort:** hours

##### `obs:25fa1be8:nil-logger-deref` — nil logger deref

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/obs-25fa1be8-firewall-nil-logger  
**Severity:** minor  
**Cluster:** handler-setup  
**Evidence:** `internal/distribution/okd/firewall/firewall.go:117-192`  
**Problem:** firewall.Configure / firewall.RemoveRules / openPort take a *slog.Logger but never normalise nil through logutil.OrNop. DetectBackend (L80) explicitly tolerates nil; the rest of the package does not. Today every caller passes non-nil p.Log so the latent panic is unreachable — but the *slog.Logger nil-tolerance contract diverges within the file (DetectBackend says nil-tolerant, everyone else assumes non-nil), inviting a future caller to honour the documented nil-tolerance and crash.  
**Fix:** At the top of Configure, RemoveRules, openPort, modifyPort: `logger = logutil.OrNop(logger)`. Or remove the 'logger may be nil' clause from DetectBackend doc and require non-nil everywhere. Pick one nil-policy and apply it package-wide.  
**Effort:** hours

##### `obs:33579dd5:refusing-critical-path-no-target` — refusing critical path no target

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/obs-33579dd5-path-attr  
**Severity:** minor  
**Cluster:** field-stability  
**Evidence:** `internal/distribution/okd/cleanup/services.go:155-176`  
**Problem:** services.Dnsmasq logs `cleanup: refusing critical path` three times in close succession (L155, L167, L176) without including which path was refused. The error message inside guardErr names the path, but a structured attr is missing — operators reading text logs see three identical lines with different `err` values; JSON pipelines cannot group by 'which file was rejected'.  
**Fix:** Add `"path", configPath` (or `"path", cfg`, `"path", backup`) at each of the three sites so the path is queryable separately from the error chain.  
**Effort:** hours

##### `obs:c19ee328:debug-fmt-sprintf-package-loop` — debug fmt sprintf package loop

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/obs-c19ee328-pkg-loop-slog  
**Severity:** minor  
**Cluster:** field-stability — related: obs:6424733c:fmt-sprintf-message-pattern  
**Evidence:** `internal/distribution/okd/setup/steps.go:312-323`  
**Problem:** installSystemPackages emits Debug per package via `p.Log.Debug(fmt.Sprintf("packages: %s not found", pkg))` and likewise for 'already installed'. Same fmt.Sprintf-into-message anti-pattern as obs:6424733c, but this one is in a tight per-element loop that runs at deploy time. Same fix pattern; called out separately because Debug-level traffic is the primary signal new operators turn on with --verbose, and structured attrs make `--log-format=json | jq` immediately useful.  
**Fix:** L312: `p.Log.Debug("packages: not found", "pkg", pkg)`. L314: `p.Log.Debug("packages: already installed", "pkg", pkg)`. L323: `p.Log.Info("packages: installing missing", "count", len(toInstall))`. Caller can then `jq 'select(.pkg=="haproxy")'` cleanly.  
**Effort:** hours

#### audit-modernization

##### `mod:5013fea6:use-slices-containsfunc` — use slices containsfunc

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/mod-5013fea6-slices-containsfunc  
**Severity:** suggestion  
**Cluster:** slices-maps  
**Evidence:** `internal/distribution/okd/setup/release_extract.go:139-147`  
**Problem:** isAuthError hand-rolls a contains-by-predicate loop over authMarkers. slices.ContainsFunc (Go 1.21) expresses the same intent in one expression and matches the pattern already in use elsewhere in this repo (status.go uses slices.ContainsFunc on cluster operator conditions).  
**Fix:** Replace body with: `lower := strings.ToLower(msg); return slices.ContainsFunc(authMarkers, func(m string) bool { return strings.Contains(lower, m) })`. Add `slices` to the existing import block (already includes strings, fmt, log/slog).  
**Effort:** hours


#### audit-code-smells

##### `smell:262af6e4:dual-cleanup-tracker` — dual cleanup tracker

**Status:** not started  
**Severity:** minor  
**Cluster:** helper-package-no-value  
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:67-137`  
**Problem:** Two parallel cleanup-tracking surfaces exist: the package-level `Execute` function in cleanup.go (which uses an `errs []error` accumulator and a switch over Kind) and `destroyTracker` in destroy/steps.go (which buffers failures+skipped labels for a printSummary step). Neither uses the canonical orchestrator `StepDef`/`BuildSteps` even though cleanup.Execute runs a fixed pipeline of named steps. The `cleanup.Phase.Execute` wrapper at line 71 does NOT actually use BuildSteps — it just calls the package-level Execute.  
**Fix:** Migrate cleanup.Execute to declare its steps as []distribution.StepDef and run via distribution.BuildSteps + Orchestrator. Reuses the destroyTracker pattern (failures + skipped labels) for the summary step instead of duplicating it.  
**Effort:** hours

##### `smell:262af6e4:pipeline-explicit-errors` — pipeline explicit errors

**Status:** not started  
**Severity:** minor  
**Cluster:** arrow-anti  
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:77-137`  
**Problem:** Execute() repeats the same `if err := X(...); err != nil { errs = append(errs, err) }` shape eight times for the Full cleanup case and once each for the *Only kinds. The cleanup pipeline is a fixed, declared list of named steps; iterating over a slice of (label, fn) pairs would shrink the body to ~10 LOC and make the kind→steps mapping data-driven.  
**Fix:** Replace the switch with a `cleanupStep` struct ({label, fn func() error}) and a kind→[]cleanupStep map. A single loop calls each fn and accumulates errs. Drops ~30 LOC and makes adding a new kind one map entry instead of a new switch arm.  
**Effort:** hours

##### `smell:262af6e4:pipeline-explicit-errors-cleanupkind` — pipeline explicit errors cleanupkind

**Status:** not started  
**Severity:** minor  
**Cluster:** magic-strings  
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:127-132`  
**Problem:** The cleanup-kind validation hardcodes the valid-values list as a literal string `(valid types: full, work-only, web-only, haproxy-only, terraform-only)` separate from the `Full` / `WorkOnly` / `WebOnly` / `HAProxyOnly` / `TerraformOnly` typed-enum constants. Add a kind to the type → forget to add it to the error message → silently misleading user-facing error.  
**Fix:** Add `var validKinds = []Kind{Full, WorkOnly, WebOnly, HAProxyOnly, TerraformOnly}` and `func (k Kind) IsValid() bool` plus a `KindStrings() []string` helper. Format the error via `strings.Join(KindStrings(), ", ")`. Self-maintaining when a new kind ships.  
**Effort:** hours

##### `smell:8ea706f6:abstraction-table-meta` — abstraction table meta

**Status:** not started  
**Severity:** minor  
**Cluster:** helper-package-no-value  
**Evidence:** `internal/distribution/okd/setup/tools.go:89-143`  
**Problem:** `binaryToolMeta` is a 25-LOC table indexed by `externalTool` whose only consumer is `installTool`. With three concrete entries (yq/helm/sops) the indirection adds a layer over what could be three explicit functions — `installYQ`, `installHelm`, `installSops` — sharing a common `installBinary(spec)` helper which already exists. The lookup table also creates a per-tool maintenance trap: forgetting to add an entry silently warns at runtime instead of failing at compile time.  
**Fix:** Replace the map with a `switch tool` in installTool that builds the binaryInstallSpec inline. Each case is ~6 LOC; total drops by ~10 LOC and missing-tool becomes a default-case panic instead of a runtime warn.  
**Effort:** hours

##### `smell:d31d1b9d:stringly-typed-enum` — stringly typed enum

**Status:** not started  
**Severity:** minor  
**Cluster:** magic-strings  
**Evidence:** `internal/cli/status.go:297-314`  
**Problem:** runDescribeNode renders node readiness as raw `"True"` / `"False"` literals (matching kube-style ConditionStatus output) but does so by ad-hoc if/else over the bool returned by isReady(), not by reusing phase.ConditionStatusTrue / phase.ConditionStatusFalse. The literal `"True"` and `"False"` here are the same enum spelled twice with no type backing the second site.  
**Fix:** Replace with `string(phase.ConditionStatusFalse)` / `string(phase.ConditionStatusTrue)` or factor a `boolToConditionStatus(bool) phase.ConditionStatus` helper. Keeps the one-line text path intact while ensuring future enum changes propagate.  
**Effort:** hours

##### `smell:262af6e4:enum-ad-hoc-cleanup-kind` — enum ad hoc cleanup kind

**Status:** not started  
**Severity:** suggestion  
**Cluster:** magic-strings  
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:21-32`  
**Problem:** Cleanup `Kind` is a typed enum with 5 valid values (Full, WorkOnly, WebOnly, HAProxyOnly, TerraformOnly) — but unlike NodeRole / VMRole / ClusterPhase the type does not expose a `Validate() error` or `String() string` method or a `SupportedKinds()` slice. Combined with the cleanup-kind error string finding, this leaves the validation as inline switch defaults that drift from the constants.  
**Fix:** Add `func (k Kind) Validate() error` and `func ValidKinds() []Kind` mirroring config.SupportedDistributions / config.SupportedProviders. Eliminates the hard-coded string list in the default branch of Execute.  
**Effort:** hours

#### audit-dependencies

##### `dep:33ef32bf:proxmox-bus-factor-1` — proxmox bus factor 1

**Status:** not started  
**Severity:** minor  
**Cluster:** maintenance-signal  
**Evidence:** `go.mod:13-13`  
**Problem:** github.com/luthermonson/go-proxmox v0.4.1 is the sole Proxmox discovery dep and exhibits bus-factor 1 (top contributor 159 commits, next 20). v0.x semver means any minor bump may break the API. Pre-known per CLAUDE.md §dependencies; re-confirmed: 28 commits in last 6 months, latest tag v0.4.1 dated 2026-04-03, license Apache-2.0, no archive marker. Abandonment plan (~200 LOC REST rewrite using net/http) still valid.  
**Fix:** No code change. Re-confirm CLAUDE.md §dependencies justification annually; bump on each upstream release; keep the documented REST-only fallback (~200 LOC) ready. Open an internal tracking issue if commit cadence drops below 4/quarter or top contributor goes inactive >6mo.  
**Effort:** hours

##### `dep:33ef32bf:dual-log-engines` — dual log engines

**Status:** not started  
**Severity:** suggestion  
**Cluster:** duplicate-engine  
**Evidence:** `go.mod:11-11` + 1 more  
**Problem:** Two logging engines: stdlib `log/slog` is the primary (44 source-file imports) and `charm.land/log/v2 v2.0.0` is the styled stderr formatter installed in internal/tui/logger.go. Intentional per CLAUDE.md (charm libs are the canonical TUI stack and listed under audit-dependencies §5 must-preserve). Re-confirming, not flagging.  
**Fix:** No action. Single charm.land/log/v2 site is the slog-handler implementation, not a parallel logging API. The TUI surface relies on charm styling; swapping it for a plain text/handler would regress UX. Document this as the intentional baseline if a future audit re-flags.  
**Effort:** hours

##### `dep:33ef32bf:exp-stale-transitive` — exp stale transitive

**Status:** not started  
**Severity:** suggestion  
**Cluster:** justified-version-floor  
**Evidence:** `go.mod:57-57`  
**Problem:** golang.org/x/exp v0.0.0-20231006140011 is an October 2023 pseudo-version pulled transitively via charm/k8s. golang.org/x/exp gets weekly pseudo-versions; the pinned timestamp is ~2.5 years stale. Not reached directly by okdctl (no source import). Floor is justified by transitive usage, but a `go mod tidy` after a charm/k8s bump should advance it.  
**Fix:** On the next k8s.io/apimachinery or charm.land/* bump run `go mod tidy` and re-check; the transitive pin will move. No targeted action needed.  
**Effort:** hours

##### `dep:33ef32bf:go-proxmox-transitive-weight` — go proxmox transitive weight

**Status:** not started  
**Severity:** suggestion  
**Cluster:** transitive-weight  
**Evidence:** `go.mod:13-13`  
**Problem:** go-proxmox v0.4.1 pulls 7 transitive deps okdctl never reaches: gorilla/websocket v1.4.2 (2020), buger/goterm (last commit 2023-02), jinzhu/copier, magefile/mage, diskfs/go-diskfs, djherbis/times, h2non/parth. okdctl's single call site (proxmox_discovery.go) only uses REST discovery. The hand-rolled REST fallback noted in CLAUDE.md would shed all 7. Documenting the cost shape; not proposing the swap mid-audit.  
**Fix:** Track go-proxmox upstream releases. When the bus-factor or breaking-change risk crosses the threshold documented in CLAUDE.md §dependencies, execute the ~200 LOC REST-only rewrite plan; that swap removes 7 transitive deps in one move. Until then, hold.  
**Effort:** hours

##### `dep:33ef32bf:gorilla-websocket-stale` — gorilla websocket stale

**Status:** not started  
**Severity:** suggestion  
**Cluster:** maintenance-signal  
**Evidence:** `go.mod:39-39`  
**Problem:** gorilla/websocket v1.4.2 is from 2020 and the latest upstream release is v1.5.3 (2024-06). Pulled transitively via go-proxmox, which pins it; okdctl never reaches it (REST-only Proxmox discovery, no shell/console websocket). Pre-known per CLAUDE.md §dependencies; documenting the version-floor delta for the next refresh of the dependency note.  
**Fix:** No local action — okdctl cannot pin a transitive without a `replace` directive, and replacing it would diverge from go-proxmox's tested set. Wait for go-proxmox to bump (or migrate to coder/websocket) and take the transitive bump for free. Re-flag only if a CVE lands that govulncheck flags as called.  
**Effort:** hours

##### `dep:33ef32bf:goterm-stale-transitive` — goterm stale transitive

**Status:** not started  
**Severity:** suggestion  
**Cluster:** maintenance-signal  
**Evidence:** `go.mod:24-24`  
**Problem:** buger/goterm v1.0.4 last commit 2023-02-25 (~3 years stale). Pulled transitively via go-proxmox for terminal helpers that okdctl never reaches. Not archived, but >18mo since last push triggers SKILL.md §7 maint-stale. Same disposition as gorilla/websocket: not reachable, leaves with the eventual go-proxmox swap.  
**Fix:** No action. Lives or dies with go-proxmox per CLAUDE.md fallback plan. Track via the dep audit footer so the next reviewer sees the staleness without re-deriving it.  
**Effort:** hours

#### audit-documentation

##### `doc:8f46b665:phases-add-step-missing-rerunsafe` — phases add step missing rerunsafe

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/doc-8f46b665-phases-arch  
**Severity:** major  
**Cluster:** readme-drift  
**Evidence:** `docs/architecture/phases.md:102-117`  
**Problem:** 'Adding a new step' instructions list NonFatal and SkipWhen but never mention ReRunSafe, which BuildSteps requires. A reader following these steps verbatim will hit 'BuildSteps: step <id> must declare ReRunSafe (ReRunSafeYes or ReRunSafeNo)' panic at first run.  
**Fix:** Add a step '0' or first-class step explaining ReRunSafe: every StepDef must set ReRunSafeYes (idempotent across re-runs — preferred default) or ReRunSafeNo (combined with AlreadyDone for resume safety). Note the panic in BuildSteps.  
**Effort:** hours

#### audit-tests

##### `tst:21dc1103:download-no-test` — download no test

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-21dc1103-download-tests  
**Severity:** blocker  
**Cluster:** canonical-helper-untested — related: sec:21dc1103:download-no-nofollow  
**Evidence:** `internal/download/download.go:79-168`  
**Problem:** Download/fetchToFile/canSkipDownload have no tests at all (only checksum.go and extract.go are covered). The function writes binaries that install.sh and setup.installBinaryToPath then chmod +x and copy into /usr/local/bin under sudo — a wrong code path here lands an attacker-controlled binary in the system PATH. The lack of a symlink-refusal test is also why sec:21dc1103 (no O_NOFOLLOW) survived multiple passes.  
**Fix:** Add internal/download/download_test.go: (1) httptest.Server-backed happy-path covering Download → file written at 0o600 with expected bytes; (2) checksum-mismatch retry → second attempt wins; (3) HTTP non-200 returns *HTTPStatusError; (4) ctx-cancel mid-download cleans the partial file (no .partial leftover); (5) symlink at OutputPath path is refused (locks the future O_NOFOLLOW guard from sec:21dc1103); (6) canSkipDownload returns true on existing file with matching checksum, false on size=0, false on checksum mismatch. Use httptest + t.TempDir; no third-party libs.  
**Effort:** days

##### `tst:39c75e91:promptconfirm-untested` — promptconfirm untested

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-39c75e91-promptconfirm  
**Severity:** major  
**Cluster:** destructive-untested  
**Evidence:** `internal/cli/confirm.go:21-62`  
**Problem:** promptForConfirmation is called by every destructive subcommand (deploy, destroy, cleanup, addon uninstall, update-ingress) but has zero direct test coverage. The function has three load-bearing branches that are unverified: (a) no-TTY refusal returning *errtypes.ConfigError, (b) ctx-cancel race vs the inputCh select, (c) isConfirmResponse only accepts y/Y/yes (not 'YES', not 'true'). A regression that loosened isConfirmResponse to strings.HasPrefix('y') would silently swallow ambiguous answers as confirms.  
**Fix:** Add tests using the existing testStdinReader hook: (1) testStdinReader=strings.NewReader('y\n') → (true, nil); (2) testStdinReader=strings.NewReader('yes\n') → (true, nil); (3) testStdinReader=strings.NewReader('n\n') → (false, nil); (4) testStdinReader=strings.NewReader('YES\n') → (false, nil) (locks case-sensitivity); (5) ctx pre-cancelled → (false, ctx.Err()); (6) testStdinReader=strings.NewReader('') → (false, nil) (EOF treated as not-confirmed). Skip the no-TTY branch since term.IsTerminal can't be stubbed without exposing a hook.  
**Effort:** hours

##### `tst:e3782ee7:make-executable-untested` — make executable untested

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-e3782ee7-makeexecutable  
**Severity:** major  
**Cluster:** canonical-helper-untested  
**Evidence:** `internal/system/fs.go:284-290`  
**Problem:** system.MakeExecutable has no test. It is the canonical 'chmod +x' helper called by setup/release_extract.go (sets +x on extracted oc binary), setup/tools.go (downloaded helper binaries), and setup/artifacts.go — all running as root under the sudo re-exec, all writing to paths that end up in PATH. A bug where mode|0o111 silently degrades to mode (e.g. an off-by-one bit-shift refactor) would land non-executable binaries that fail at runtime mid-deploy.  
**Fix:** Add to fs_test.go: (1) MakeExecutable on a 0o600 file → mode becomes 0o711 (owner exec preserved through bit-OR); (2) MakeExecutable on a 0o644 file → 0o755; (3) MakeExecutable on 0o600 in a t.TempDir() preserves contents (read body before/after); (4) MakeExecutable on a missing path → wrapped error containing the path. ~30 LOC, table-driven.  
**Effort:** hours

##### `tst:33579dd5:dnsmasq-config-path-untested` — dnsmasq config path untested

**Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/tst-33579dd5-dnsmasq-traversal  
**Severity:** minor  
**Cluster:** trust-boundary-untested  
**Evidence:** `internal/distribution/okd/cleanup/services.go:142-187`  
**Problem:** Dnsmasq() has TestDnsmasq_GlobLoopRemovesAllMatches but no test for the cluster-name-driven path: line 153 builds configPath via dns.DnsmasqConfigPath('okd-'+clusterName) and calls os.RemoveAll on it under refuseCriticalPath guard. ClusterName comes from cfg and is validated at config-load, but a hand-edited YAML with an attacker-shaped clusterName (e.g. '../../etc' or 'okd-..%2f..') is the threat the refuseCriticalPath guard exists to catch — and there is no test for that intersection.  
**Fix:** Add t.Run('clusterName containing path-traversal segments hits refuseCriticalPath') to services_test.go: monkey-patch dnsmasqConfPattern + dnsmasqBackupPattern (already done by sibling test) and pass clusterName='../../../../etc/okd-x'. Assert that no os.RemoveAll touched anything outside t.TempDir(). Use a t.TempDir() decoy with a sentinel file and check it survives.  
**Effort:** hours

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


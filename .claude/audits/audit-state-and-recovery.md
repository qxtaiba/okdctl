# audit-state-and-recovery — 2026-05-08

**Assumes green:** golangci-lint, govulncheck, CodeQL, shellcheck, tflint, go test ./...
**Scope:** `internal/distribution/okd/{setup,install,postinstall,destroy,cleanup}`, `internal/distribution/okd/phase/`, `internal/distribution/{step.go,orchestrator.go}`, `internal/runlock/`, `internal/system/atomic*` (fs.go), `internal/infrastructure/terraform/`, `internal/infrastructure/proxmox/`, `internal/cli/{destroy.go,deploy.go,cleanup.go,deploystate.go,helpers.go}`, `internal/config/loader.go`. Topics: tf-state atomicity / locking / backup / concurrent-safety, phase idempotency, mid-deploy crash recoverability, schema evolution, destroy safety, Proxmox API idempotency.
**Out of scope this run:** `internal/tui/wizard/**`, templates, `iso.go` (per AUDIT_CONVENTIONS §2 — but `setup/iso.go` is in scope as the build-fingerprint site is recovery-relevant), `_test.go` files.
**Seam co-owners:** `audit-security` owns credential hygiene during destroy (envvars passed to terraform, Zeroize lifecycle); this audit defers all credential-lifecycle findings to `audit-security`. Cross-references via `related` field — none in this run since no overlapping findings landed here.

## Executive summary

okdctl's state surface is mostly well-shaped: `system.AtomicWrite` is universally adopted (no `os.WriteFile`/`ioutil.WriteFile` outside tests on the production path), `runlock` uses kernel flock with O_NOFOLLOW symlink hardening, `StepDef` enforces `ReRunSafe` declaration, and the destroy path has a `--confirm-cluster` typo guard plus a `--dry-run` plan-only mode. The biggest cluster of findings is **phase-idempotency** (6 findings, 30%), driven by `AlreadyDone` checks that key on directory existence rather than commit-on-success sentinels — a partial `openshift-install create manifests` leaves a directory the next run silently skips. Second cluster is **crash-recoverability** (5 findings) where the deploy-state marker has no TTL and the cleanup phase has no resume diagnostic. Six findings are `major` (none are `blocker`) — the rubric ceiling is honored at 30%. The destroy step ordering after a TF failure (`state:15ba17da:destroy-iso-cleanup-before-tf`) and the missing canonical-helper coverage on the deploy path (`state:62cb8a95:state-version-warn-only`) are the highest-leverage fixes.

## Ranked table

| ID | Cluster | File:line | Severity | Confidence | LOC | Adjacent linter | Fix class |
|----|---------|-----------|----------|------------|-----|-----------------|-----------|
| state:48688e63:tf-state-no-backup | tf-state-atomicity | internal/infrastructure/terraform/terraform.go:325-344 | major | high | 20 | none | refactor |
| state:15ba17da:destroy-iso-cleanup-before-tf | destroy-safety | internal/distribution/okd/destroy/steps.go:74-167 | major | high | 50 | none | refactor |
| state:62cb8a95:state-version-warn-only | state-schema-evolution | internal/distribution/okd/destroy/helpers.go:26-59 | major | high | 34 | none | refactor |
| state:c19ee328:setup-installer-already-done-only-bootstrap | phase-idempotency | internal/distribution/okd/setup/steps.go:109-201 | major | medium | 93 | none | refactor |
| state:4c092fce:tf-state-no-lock | tf-state-atomicity | internal/infrastructure/terraform/terraform.go:118-174 | major | medium | 15 | none | refactor |
| state:fb54208a:postinstall-bootstrap-cleanup-nonfatal | phase-idempotency | internal/distribution/okd/postinstall/steps.go:43-58 | major | medium | 16 | none | refactor |
| state:eb479d86:upload-resume-not-supported | phase-idempotency | internal/distribution/okd/setup/upload.go:82-146 | minor | high | 65 | none | refactor |
| state:0f076161:destroy-target-no-precondition | destroy-safety | internal/cli/destroy.go:80-162 | minor | high | 50 | none | refactor |
| state:c287d5c0:destroy-auto-approve-hardcoded | destroy-safety | internal/distribution/okd/okd.go:219-231 | minor | high | 13 | none | refactor |
| state:33579dd5:cleanup-haproxy-firewall-double | phase-idempotency | internal/distribution/okd/cleanup/services.go:56-93 | minor | high | 38 | none | refactor |
| state:b38ec9cc:workers-targeted-apply-vars-not-snapshot | phase-idempotency | internal/distribution/okd/install/workers.go:19-56 | minor | medium | 38 | none | refactor |
| state:b804b2ec:bootstrap-cleanup-vars-not-snapshot | phase-idempotency | internal/distribution/okd/postinstall/bootstrap.go:18-61 | minor | medium | 44 | none | refactor |
| state:48688e63:proxmox-no-retry-on-init-apply | proxmox-api-idempotency | internal/infrastructure/proxmox/proxmox.go:159-227 | minor | medium | 68 | none | refactor |
| state:b5a79fda:deploy-state-marker-stale | crash-recoverability | internal/cli/deploystate.go:24-86 | minor | medium | 63 | none | refactor |
| state:632c9087:update-ingress-no-rollback-on-dns | crash-recoverability | internal/distribution/okd/postinstall/update_ingress.go:237-279 | minor | medium | 43 | none | refactor |
| state:368b892b:cleanup-tfstate-preserved-but-orphan | state-schema-evolution | internal/distribution/okd/cleanup/infra.go:47-72 | minor | medium | 26 | none | refactor |
| state:6424733c:project-marker-stale | crash-recoverability | internal/cli/helpers.go:115-162 | minor | medium | 48 | none | refactor |
| state:881d089e:lock-stale-host-different | tf-state-atomicity | internal/runlock/runlock.go:32-77 | minor | medium | 10 | none | refactor |
| state:262af6e4:cleanup-no-resume-doc | crash-recoverability | internal/distribution/okd/cleanup/cleanup.go:1-21 | suggestion | high | 21 | none | refactor |
| state:f743eaa2:iso-build-fingerprint-not-fsynced | crash-recoverability | internal/distribution/okd/setup/iso.go:91-108 | suggestion | high | 18 | none | refactor |

## Phase idempotency matrix

| Phase | Step | ReRunSafe | AlreadyDone? | Re-run safe after partial fail? |
|-------|------|-----------|--------------|---------------------------------|
| setup | install-packages | Yes | no | yes (executor.CommandExists per-pkg) |
| setup | install-tools | Yes | no | yes (idempotent) |
| setup | ensure-workdir | Yes | no | yes (mkdirall) |
| setup | download-tools | No | yes (binDir/3-bins) | yes |
| setup | generate-config | No | yes (.backup sentinel) | YES — sentinel after success |
| setup | generate-manifests | No | **yes (dir only)** | **PARTIAL — c19ee328** |
| setup | generate-kubevip-manifests | Yes | no | yes (atomic-write) |
| setup | inject-manifests | Yes | no | yes (CopyFile) |
| setup | compact-cluster | Yes | no | yes (atomic-write) |
| setup | generate-ignition | No | **yes (.ign size+JSON)** | **PARTIAL — c19ee328** |
| setup | install-apache | Yes | no | yes |
| setup | deploy-ignition | No | yes (bootstrap.ign exists) | yes (CopyFileMode atomic at file level) |
| setup | verify-webserver | Yes | no | yes (HTTP probe) |
| setup | build-isos | Yes | per-node fp file | yes — but f743eaa2 |
| setup | upload-isos | No | yes (sha256 per file) | partial — eb479d86 |
| setup | generate-tfvars | Yes | no | yes (atomic-write) |
| setup | configure-haproxy | Yes | no | yes (backup-then-replace + rollback) |
| setup | configure-firewall | Yes | no | yes (idempotent) |
| setup | configure-dns | Yes | no | yes (drop-in atomic) |
| install | deploy-infrastructure | Yes | no | yes (terraform-apply idempotent) |
| install | wait-bootstrap | Yes | no | yes (poll) |
| install | start-workers | Yes | no | yes — but b38ec9cc (no AlreadyDone, vars not persisted) |
| install | setup-kubeconfig | Yes | no | yes |
| install | validate-access | Yes | no | yes |
| install | monitor-install | Yes | no | yes |
| install | setup-access | Yes | no | yes (CopyFileMode + AtomicWrite) |
| postinstall | verify-health | Yes | no | yes |
| postinstall | cleanup-bootstrap | Yes | no | yes — but fb54208a (NonFatal, b804b2ec vars not persisted) |
| postinstall | verify-kubevip | Yes | no | yes |
| postinstall | deploy-production-dns | Yes | no | yes (atomic-write) |
| postinstall | install-addons | Yes | no | yes (helm upgrade --install / kubectl apply) |
| destroy | destroy-infrastructure | Yes | no | yes (TF idempotent on destroyed infra) |
| destroy | remove-remote-iso | Yes | no | yes (skip-if-VM-still-running) |
| destroy | cleanup-files | Yes | no | partial — 15ba17da (runs after TF fails) |
| destroy | cleanup-firewall | Yes | no | yes (RemoveOKDRules idempotent) — but 33579dd5 |
| cleanup | cleanup-workdir | No | yes (DirNotExists) | yes |
| cleanup | cleanup-webserver | Yes | no | yes (glob) |
| cleanup | cleanup-haproxy | No | yes (config absent + svc inactive) | yes |
| cleanup | cleanup-apache | Yes | no | yes |
| cleanup | cleanup-dnsmasq | No | yes (cluster conf absent) | yes |
| cleanup | cleanup-terraform | No | yes (tfvars absent) | yes |
| cleanup | cleanup-packages | Yes | no | yes |

**Verdict:** 36/41 steps clean; 5 carry idempotency or recoverability gaps tracked in findings.

## Crash-scenario table

For 8 representative crash points, what state the user is in + resume path:

| Crash point | User state after crash | Resume path | Gap |
|------|----|----|----|
| Setup before terraform.tfvars written | clusterDir partial; tfstate empty | re-run `okdctl deploy`; cleanup-WorkOnly clears workDir | clean |
| Mid-`openshift-install create manifests` | manifests/ dir exists with partial content | re-run `okdctl deploy` — **silently skips manifests step due to dir-exists AlreadyDone** | **c19ee328** |
| Bootstrap apply succeeded, ISO upload mid-batch interrupted | 3 of 4 ISOs on Proxmox, 1 corrupt | re-run `okdctl deploy` — **re-uploads all 4** (full batch) | eb479d86 |
| `terraform apply` cancelled mid-run | partial VMs created; tfstate populated | re-run `okdctl deploy` (TF apply idempotent) OR `okdctl destroy` | clean (with state:48688e63 caveat — no okdctl-managed state snapshot) |
| Bootstrap-complete reached, install monitor SIGINT | VMs up, kubeconfig present, install incomplete | re-run `okdctl install` (monitor re-attaches via kubeconfig) | clean (deploystate marker advises 'cancelled during install') |
| Postinstall: bootstrap cleanup TF apply -target=bootstrap fails | bootstrap VM still running, deploy reports complete | operator must run `okdctl destroy --only=bootstrap` manually | **fb54208a** (no warning surfaced) |
| Postinstall: production DNS deployed, RemoveHAProxy fails | DNS at production, haproxy still on bastion | manual fix needed; no rollback | **632c9087** |
| `okdctl destroy` cancelled between TF destroy and cleanup-files | tfstate empty, host configs partially removed | re-run `okdctl destroy` is safe (steps idempotent) | clean (modulo 15ba17da when TF itself FAILED rather than cancelled) |

## Destroy-safety checklist

| Check | Status | Notes |
|----|----|----|
| User confirmation required | ✓ | promptForConfirmation in confirm.go + --confirm-cluster typo guard |
| `--dry-run` flag exists | ✓ | runDestroyDryRun streams `terraform plan -destroy` |
| Master nodes have prevent_destroy | ✓ | `infrastructure/terraform/modules/proxmox-okd/main.tf:256` (operator must add override.tf to bypass) |
| `--target` allowlist | ✓ | regex destroyTargetRE; --confirm-cluster mandatory with --target |
| `--target` index bounds-check | ✗ | **state:0f076161** — regex permits master[7] for a 3-master cluster |
| Compute-before-network ordering | ✓ | terraform's resource graph handles per-resource; module destroy order respected |
| Partial-failure cleanup gates | ⚠ | terraformFailed→WorkOnly downgrade good; but firewall+haproxy still wiped — **state:15ba17da** |
| AutoApprove user-controllable | ✗ | **state:c287d5c0** — Provisioner.Destroy hard-codes true |
| State-version preflight | ⚠ | **state:62cb8a95** — only on destroy path; deploy missing |
| Resume after partial destroy | ✓ | --skip-terraform / --skip-cleanup / --skip-firewall flags |
| Lock-conflict diagnostic | ✓ | runlock body shows holder PID/HOST/VERB/TIME |
| Stale-lock force-unlock hint | ✓ | stateLockHint in destroy/helpers.go |
| Scoped destroy (`--only`) | ✓ | vms / workers / masters / bootstrap |
| ISO removal in-use guard | ✓ | iso_cleanup.go fail-closed `anyVMReferencesISO` |

## Findings

### state:48688e63:tf-state-no-backup
**Cluster:** tf-state-atomicity
**File + line range:** internal/infrastructure/terraform/terraform.go:325-344
**Current LOC touched:** 20
**Smell:** HasState reads terraform.tfstate but okdctl never snapshots it before invoking `terraform apply` or `terraform destroy`. terraform itself writes a `terraform.tfstate.backup` only across the apply boundary, leaving zero okdctl-managed history for the operator to roll back to after a corrupt state lands.
**Evidence:**
```go
// HasState reports whether the working directory contains a terraform.tfstate
// with at least one managed resource.
func (t *Executor) HasState() bool {
  stateFile := filepath.Join(t.WorkDir, "terraform.tfstate")
  // ... no copy / snapshot before any apply / destroy below this line ...
}
```
**Fix — preferred:** refactor — add `Executor.SnapshotState(ctx)` that copies tfstate to terraform.tfstate.<UTC-timestamp>.bak via system.AtomicWrite immediately before every apply/destroy entrypoint. Retain N=5 most recent snapshots; surface the latest path in destroy / cleanup error messages so the operator can `cp` it back.
**Rule source:** CLAUDE.md §architecture-notes (AtomicWrite + fsync on trust-boundary files); Twelve-factor app §disposability; repo-counter-example: internal/system/fs.go:L196-L243 (AtomicWrite + fsyncDir)
**Adjacent linter:** none — pure design audit
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** system.AtomicWrite contract; existing terraform.tfstate.backup preservation in cleanup
**Estimated net LOC delta:** +40
**Severity:** major
**Severity reason:** rubric §4/data-loss — corrupted tfstate with no okdctl-managed snapshot leaves operator unable to recover bootstrap+master VMID mapping after a crash mid-apply, blocking destroy.
**Risk (of applying fix):** low — snapshot is additive
**Confidence (in finding):** high — direct read of HasState + grep across all callers shows no snapshot site
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### state:15ba17da:destroy-iso-cleanup-before-tf
**Cluster:** destroy-safety
**File + line range:** internal/distribution/okd/destroy/steps.go:74-167
**Current LOC touched:** 50
**Smell:** Step ordering is StepDestroyInfra → StepRemoveRemoteISO → StepCleanupFiles → StepCleanupFirewall, but with NonFatal=true on every step the orchestrator continues even after `terraform destroy` fails. The terraformFailed() guard at L127 catches the cleanup-Kind downgrade but does NOT change the step ordering — irreversible host-file cleanup still runs after a TF failure that left VMs alive.
**Evidence:**
```go
// StepDestroyInfra: NonFatal: true
// StepCleanupFiles: NonFatal: true
// if t.terraformFailed() && kind == cleanup.Full {
//   kind = cleanup.WorkOnly  // <-- preserves tfvars but still removes haproxy/dnsmasq
// }
```
**Fix — preferred:** refactor — when destroyTracker.terraformFailed() is true, also skip StepCleanupFirewall and the haproxy/dnsmasq subset of StepCleanupFiles. Keep only the workdir cleanup. Today the WorkOnly downgrade is correct shape — extend it.
**Rule source:** CLAUDE.md §state-and-recovery (destroy ordering: compute before network before storage); repo-counter-example: internal/distribution/okd/destroy/steps.go:L127
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** NonFatal step semantics for TF-after-VMs-already-gone retry case; destroyTracker summary structure
**Estimated net LOC delta:** +25
**Severity:** major
**Severity reason:** rubric §4/data-loss — host service cleanup (haproxy/dnsmasq teardown) on a TF-destroy-failed run leaves the cluster up but unreachable from the bastion, blocking the user's recovery path.
**Risk (of applying fix):** medium — touching destroy ordering needs a tracker-state test
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** state:33579dd5 (firewall double-call)

### state:62cb8a95:state-version-warn-only
**Cluster:** state-schema-evolution
**File + line range:** internal/distribution/okd/destroy/helpers.go:26-59
**Current LOC touched:** 34
**Smell:** checkStateMajorVersion only runs on the destroy path. The deploy / install / postinstall paths invoke terraform.Init -> Apply against the same tfstate without ever validating its terraform_version. A user who upgraded terraform from 1.x to 2.x mid-deploy hits a silent state-format issue at apply time with terraform's own confusing error rather than okdctl's clear 'major mismatch' message.
**Evidence:**
```go
// checkStateMajorVersion reads stateFile, extracts .terraform_version, and
// returns *errtypes.ConfigError when the parsed major component falls outside
// [stateMajorMin, stateMajorMax].
// (only called from destroyInfrastructure; install/setup/postinstall never call it)
```
**Fix — preferred:** refactor — move checkStateMajorVersion (and parseLockID/stateLockHint) into internal/infrastructure/terraform/state.go and call it once from Executor.Init when the state file already exists. Both deploy and destroy paths benefit.
**Rule source:** CLAUDE.md §architecture-notes (canonical helpers belong in internal/distribution/okd/phase/ or internal/infrastructure/); repo-counter-example: internal/distribution/okd/destroy/helpers.go:L26-L59
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** checkStateMajorVersion error message text (helpers_test.go pins it); destroy.helpers_test stateLockHint Msg substring 'force-unlock'
**Estimated net LOC delta:** 0 (move, not add)
**Severity:** major
**Severity reason:** rubric §4/canonical-helper-on-critical-path — the canonical helper exists but is missing on the deploy/install path which is the more common entry.
**Risk (of applying fix):** low — pure motion
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### state:c19ee328:setup-installer-already-done-only-bootstrap
**Cluster:** phase-idempotency
**File + line range:** internal/distribution/okd/setup/steps.go:109-201
**Current LOC touched:** 93
**Smell:** StepGenerateManifests.AlreadyDone keys on `clusterDir/manifests` directory existing. But manifests directory existing does NOT mean manifests are valid — `openshift-install create manifests` exits non-zero mid-write and leaves a partial manifests/ that the next run sees as 'already done'. Re-run skips manifest generation; subsequent ignition step then operates on partial manifests and produces malformed ignition.
**Evidence:**
```go
AlreadyDone: func(_ context.Context) (bool, error) {
  return system.DirExists(filepath.Join(clusterDir, "manifests")), nil
},
```
**Fix — preferred:** refactor — tighten the manifests-AlreadyDone to require BOTH the directory AND a sentinel file like `<clusterDir>/manifests/.complete` written via AtomicWrite by GenerateManifests on success. Pattern matches the install-config.yaml.backup sentinel already used by StepGenerateConfig. Same idea for StepGenerateIgnition: add a `.ignition.complete` sentinel after ValidateIgnitionFiles passes.
**Rule source:** CLAUDE.md §architecture-notes (AlreadyDone check should commit-on-success); repo-counter-example: internal/distribution/okd/setup/steps.go:L115 StepGenerateConfig uses .backup sentinel correctly
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** openshift-install argv; ignitionFilenames list; existing .backup sentinel for install-config
**Estimated net LOC delta:** +15
**Severity:** major
**Severity reason:** rubric §4/data-loss-adjacent — partial manifests through to ignition produces a non-bootable cluster; failure mode silent until bootstrap timeout 30 min in.
**Risk (of applying fix):** medium — must coordinate sentinel write timing
**Confidence (in finding):** medium — depends on openshift-install's exact partial-state behavior; the gap is real either way
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### state:4c092fce:tf-state-no-lock
**Cluster:** tf-state-atomicity
**File + line range:** internal/infrastructure/terraform/terraform.go:118-174
**Current LOC touched:** 15
**Smell:** runlock.Acquire serializes okdctl invocations on the same project root via flock, but Terraform itself enforces its own state lock via `.terraform.tfstate.lock.info` only inside terraform-the-binary. Two callers running on different hosts (NFS-mounted project root) bypass okdctl's flock (advisory on NFSv3) AND can race terraform if its lock-file write is non-atomic on that filesystem. The Executor never sets `-lock-timeout`.
**Evidence:**
```go
func (t *Executor) Init(ctx context.Context) error {
  // ... no -lock-timeout flag set ...
}
func (t *Executor) run(ctx context.Context, args ...string) error {
  // ... no -lock-timeout flag set ...
}
```
**Fix — preferred:** refactor — pass `-lock-timeout=120s` to every terraform invocation in Executor.run / Init. Surface stateLockHint in Plan/Apply paths (today only destroyInfrastructure inspects it).
**Rule source:** CLAUDE.md §concurrency; Terraform best-practices: -lock-timeout; repo-counter-example: internal/distribution/okd/destroy/helpers.go:L67-L82
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** runlock.Acquire flock semantics; existing destroy stateLockHint user-facing message format
**Estimated net LOC delta:** +15
**Severity:** major
**Severity reason:** rubric §4/data-loss — concurrent terraform invocation on a shared filesystem (NFS, sshfs) can corrupt tfstate.
**Risk (of applying fix):** low — additive flag
**Confidence (in finding):** medium — depends on operator's deployment topology
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** state:881d089e

### state:fb54208a:postinstall-bootstrap-cleanup-nonfatal
**Cluster:** phase-idempotency
**File + line range:** internal/distribution/okd/postinstall/steps.go:43-58
**Current LOC touched:** 16
**Smell:** StepCleanupBootstrap is NonFatal=true. If terraform apply -target=bootstrap fails, postinstall continues and reports 'deployment complete' even though the bootstrap VM is still running, eating control-plane resources and risking etcd quorum confusion.
**Evidence:**
```go
{
  ID: StepCleanupBootstrap, ...
  NonFatal:  true,
  Exec: ...
  OnError: phase.WarnOnError(p.Log, "bootstrap: cleanup failed (non-critical)"),
}
```
**Fix — preferred:** refactor — elevate NonFatal=false (preferred) OR keep NonFatal but surface result.BootstrapCleaned=false prominently in PostDeploySummary with a 'run okdctl destroy --only=bootstrap' hint.
**Rule source:** CLAUDE.md §state-and-recovery (irreversible-op ordering); repo-counter-example: postinstall/steps.go:L26-L42 StepVerifyHealth is fatal — asymmetric
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** WarnOnError pattern; result.BootstrapCleaned semantics; ReRunSafeYes
**Estimated net LOC delta:** +5
**Severity:** major
**Severity reason:** rubric §4/data-loss-adjacent — zombie bootstrap can disrupt etcd quorum on member-count miscalculation.
**Risk (of applying fix):** low
**Confidence (in finding):** medium — depends on operator habit of running destroy after a noisy deploy
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** state:b804b2ec

### state:eb479d86:upload-resume-not-supported
**Cluster:** phase-idempotency
**File + line range:** internal/distribution/okd/setup/upload.go:82-146
**Current LOC touched:** 65
**Smell:** UploadCustomISOsToProxmox runs scp of every ISO whose remote sha256 differs from local in a single subprocess. Mid-upload SIGINT leaves a corrupt tail; the next AlreadyDone hashes the remote — sees mismatch — and re-uploads ALL ISOs (not just the partial), because scp invocation is a single batch.
**Evidence:**
```go
args := []string{"-o", "StrictHostKeyChecking=accept-new"}
args = append(args, isoFiles...)
args = append(args, fmt.Sprintf("%s@%s:%s/", user, host, remotePath))
// single scp call; partial-failure resumes by re-uploading every file, not just the tail
```
**Fix — preferred:** refactor — iterate per-file in uploadISOsViaSCP; before each scp, run isoUploadNeeded for that one file (skip already-matching).
**Rule source:** CLAUDE.md §state-and-recovery (mid-deploy crash + resume); repo-counter-example: upload.go:L40-L50 isoUploadNeeded already exists per-file
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** isoUploadNeeded sha256 comparison; SSHRunArgv path; scp StrictHostKeyChecking semantics
**Estimated net LOC delta:** +20
**Severity:** minor
**Risk (of applying fix):** low
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### state:0f076161:destroy-target-no-precondition
**Cluster:** destroy-safety
**File + line range:** internal/cli/destroy.go:80-162
**Current LOC touched:** 50
**Smell:** --target / --only allowlist regex permits arbitrary index without verifying it's < cfg.Topology.{ControlPlane|Workers}.Count. `okdctl destroy --target=...master[7]` for a 3-master cluster passes validation, hits terraform with a target that does not exist, and reports success.
**Evidence:**
```go
var destroyTargetRE = regexp.MustCompile(
  `^module\.okd_cluster\.proxmox_virtual_environment_vm\.(bootstrap|master|worker)(\[\d+\])?$`,
)
// validateDestroyTargets only checks regex; never compares index against cfg.Topology counts.
```
**Fix — preferred:** refactor — after regex match, parse the index out of `[N]` and reject when N >= cfg.Topology.ControlPlane.Count for master, etc.
**Rule source:** repo-counter-example: internal/cli/destroy.go:L45-L78 expandOnlyFlag already does the index math from cfg.Topology
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** destroyTargetRE allowlist; --confirm-cluster guard; expandOnlyFlag count semantics
**Estimated net LOC delta:** +15
**Severity:** minor
**Risk (of applying fix):** low
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### state:c287d5c0:destroy-auto-approve-hardcoded
**Cluster:** destroy-safety
**File + line range:** internal/distribution/okd/okd.go:219-231
**Current LOC touched:** 13
**Smell:** Provisioner.Destroy hard-codes destroyOpts.AutoApprove=true regardless of cfg.Deployment.AutoApprove or any CLI flag. Anyone constructing a Provisioner programmatically gets non-interactive terraform destroy with no upstream check.
**Evidence:**
```go
func (p *Provisioner) Destroy(ctx context.Context, cfg *config.Config, opts DestroyOpts) ([]distribution.StepResult, error) {
  destroyPhase := destroy.New(...)
  destroyOpts := destroy.NewOptions(cfg, p.projectRoot)
  destroyOpts.AutoApprove = true  // <-- always true
  // ... opts.AutoApprove never read ...
}
```
**Fix — preferred:** refactor — add `AutoApprove bool` to DestroyOpts and propagate via `destroyOpts.AutoApprove = opts.AutoApprove`. Mirror the symmetric ProvisionOptions.AutoApprove already in place.
**Rule source:** repo-counter-example: internal/infrastructure/proxmox/types.go ProvisionOptions.AutoApprove; CLAUDE.md §architecture-notes (option-struct symmetry)
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** existing CLI confirmation prompt + --confirm-cluster guard
**Estimated net LOC delta:** +3
**Severity:** minor
**Risk (of applying fix):** low
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### state:33579dd5:cleanup-haproxy-firewall-double
**Cluster:** phase-idempotency
**File + line range:** internal/distribution/okd/cleanup/services.go:56-93
**Current LOC touched:** 38
**Smell:** cleanup.HAProxy calls firewall.RemoveOKDRules at L71. The destroy phase ALSO has a separate StepCleanupFirewall that calls firewall.RemoveOKDRules. Both run in `okdctl destroy`; on firewalld a second call against an already-removed rule logs a warning that propagates as a non-fatal failure in the destroy summary.
**Evidence:**
```go
// cleanup/services.go:71
if err := firewall.RemoveOKDRules(ctx, true, logger); err != nil { ... }
// destroy/steps.go:160 (different step in same Run)
if err := firewall.RemoveOKDRules(ctx, true, p.Log); err != nil { ... }
```
**Fix — preferred:** refactor — remove the firewall.RemoveOKDRules call from cleanup.HAProxy; rely on StepCleanupFirewall as the single removal site.
**Rule source:** CLAUDE.md §architecture-notes (single canonical caller); Go proverb: clear is better than clever
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** firewall.RemoveOKDRules contract; haproxy stop-and-disable ordering
**Estimated net LOC delta:** -7
**Severity:** minor
**Risk (of applying fix):** low
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** state:15ba17da

### state:b38ec9cc:workers-targeted-apply-vars-not-snapshot
**Cluster:** phase-idempotency
**File + line range:** internal/distribution/okd/install/workers.go:19-56
**Current LOC touched:** 38
**Smell:** StartWorkerVMs runs `terraform apply -var start_workers_immediately=true -target=...worker`. The override is at apply-time only; nothing writes the value back into terraform.tfvars and the AlreadyDone check is missing. A manual `terraform plan` from the workdir AFTER deploy completes still shows the workers diff because tfvars was never updated.
**Evidence:**
```go
// terraform.tfvars is the deploy-time snapshot written by setup.GenerateTerraformVars
// and is not mutated here. start_workers_immediately defaults to false in that
// snapshot; this call overrides it at apply time via -var.
// (no AlreadyDone hook on the StepStartWorkers definition)
```
**Fix — preferred:** refactor — declare ReRunSafeNo with an AlreadyDone hook that checks `oc get nodes -l node-role.kubernetes.io/worker` count >= cfg.Topology.Workers.Count; OR write the var back to tfvars.
**Rule source:** CLAUDE.md §architecture-notes (StepDef.AlreadyDone for ReRunSafe steps); repo-counter-example: setup/steps.go:L131-L141
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** -target scope; existing comment about authoritative state in tfstate
**Estimated net LOC delta:** +10
**Severity:** minor
**Risk (of applying fix):** low
**Confidence (in finding):** medium
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** state:b804b2ec

### state:b804b2ec:bootstrap-cleanup-vars-not-snapshot
**Cluster:** phase-idempotency
**File + line range:** internal/distribution/okd/postinstall/bootstrap.go:18-61
**Current LOC touched:** 44
**Smell:** CleanupBootstrap apply-overrides bootstrap_enabled=false but never persists it. `okdctl destroy` later runs without that var; terraform reads bootstrap_enabled=true from tfvars and the destroy plan flags 'create' on bootstrap which is confusing in plan output.
**Evidence:**
```go
vars := map[string]string{"bootstrap_enabled": "false"}
targets := []string{"module.okd_cluster.proxmox_virtual_environment_vm.bootstrap"}
// ... applied via -var, never written back to terraform.tfvars ...
```
**Fix — preferred:** refactor — after successful CleanupBootstrap, rewrite terraform.tfvars with bootstrap_enabled=false. Alternative: route via a generated `bootstrap-state.auto.tfvars.json` that takes precedence over the static tfvars.
**Rule source:** repo-counter-example: setup/terraform.go:GenerateTerraformVars; CLAUDE.md §state-and-recovery
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** defer plan-file cleanup at L40-L43; -target scope
**Estimated net LOC delta:** +15
**Severity:** minor
**Risk (of applying fix):** low
**Confidence (in finding):** medium
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** state:b38ec9cc

### state:48688e63:proxmox-no-retry-on-init-apply
**Cluster:** proxmox-api-idempotency
**File + line range:** internal/infrastructure/proxmox/proxmox.go:159-227
**Current LOC touched:** 68
**Smell:** Provider.Provision delegates retry/backoff to the bpg/proxmox terraform provider. But there is no okdctl-side retry around `terraform init` itself: a transient TLS hiccup at init time fails the whole deploy with no per-step retry seam.
**Evidence:**
```go
if err := p.terraformExec.Init(ctx); err != nil {
  return nil, &errtypes.ClusterError{Msg: "terraform init failed", Err: err}
}
// no retry wrapper; transient TLS hiccup → restart the whole deploy
```
**Fix — preferred:** refactor — wrap terraform.Executor.Init in a 3-attempt retry-with-jitter for transient errors. Reuse internal/download's retryDownload pattern referenced in proxmox.go's doc-comment.
**Rule source:** CLAUDE.md §architecture-notes (mutation invariant comment in proxmox.go: state:48688e63); repo-counter-example: internal/download/retry.go retryDownload
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** Mutation invariant: all Proxmox mutations flow through terraform.Executor
**Estimated net LOC delta:** +15
**Severity:** minor
**Risk (of applying fix):** low
**Confidence (in finding):** medium
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### state:b5a79fda:deploy-state-marker-stale
**Cluster:** crash-recoverability
**File + line range:** internal/cli/deploystate.go:24-86
**Current LOC touched:** 63
**Smell:** announceDeployState reads .okdctl-deploy-state.json on destroy entry and emits 'partial deploy detected — cancelled during X'. But there's no TTL or mtime check; a marker left from a successful deploy whose clearDeployMarker failed misleads the operator on a totally unrelated destroy weeks later.
**Evidence:**
```go
func announceDeployState(path string) {
  ds, err := readDeployState(path)
  // ... no mtime check; no comparison against current cfg.Cluster.Name; no expiry ...
}
```
**Fix — preferred:** refactor — emit the marker's age in the warning; add ClusterName to deployState struct; on mismatch emit 'marker is from a different cluster, ignoring'. Make markDeployPhase fatal on first call.
**Rule source:** CLAUDE.md §state-and-recovery (mid-deploy crash + diagnostic pointing at resume point)
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** AtomicWrite usage; existing 'cancelled during prepare/install/configure' phase enum
**Estimated net LOC delta:** +20
**Severity:** minor
**Risk (of applying fix):** low
**Confidence (in finding):** medium
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### state:632c9087:update-ingress-no-rollback-on-dns
**Cluster:** crash-recoverability
**File + line range:** internal/distribution/okd/postinstall/update_ingress.go:237-279
**Current LOC touched:** 43
**Smell:** finalizeIngress deploys production DNS, then optionally removes HAProxy. If RemoveHAProxy fails AFTER DNS has been swapped from bootstrap → production, the cluster is left with DNS at production but HAProxy still listening on bastion — incoming requests bypass kube-vip. There's no rollback to bootstrap DNS on a failed haproxy removal.
**Evidence:**
```go
if err := p.deployProductionDNS(ctx, cfg, appsIP, vip, customDomains); err != nil {
  return nil, &errtypes.ClusterError{...}
}
// dns now production
if opts.RemoveHAProxy && hostNetworkCount == 0 {
  if err := p.RemoveHAProxy(ctx, vip, ...); err != nil {
    p.Log.Warn(...)  // <-- DNS already swapped; warn-only
  }
}
```
**Fix — preferred:** refactor — sequence ops so DNS swap is the LAST destructive step (RemoveHAProxy first, then DNS). Or: on RemoveHAProxy failure, attempt rollback to bootstrap DNS via dns.DeployBootstrap.
**Rule source:** CLAUDE.md §state-and-recovery (compute → network → storage destroy ordering applied to migrate ordering)
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** existing waitForServiceLB / VIP verification in RemoveHAProxy; user-facing 'haproxy removed' positive outcome
**Estimated net LOC delta:** +20
**Severity:** minor
**Risk (of applying fix):** medium — sequence change touches cutover semantics
**Confidence (in finding):** medium
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### state:368b892b:cleanup-tfstate-preserved-but-orphan
**Cluster:** state-schema-evolution
**File + line range:** internal/distribution/okd/cleanup/infra.go:47-72
**Current LOC touched:** 26
**Smell:** terraformFilesToRemove deliberately excludes terraform.tfstate. After a successful destroy that empties tfstate, cleanup leaves it. The next deploy / Provisioner.Prepare runs cleanup-WorkOnly which doesn't include the terraformStep — orphan empty tfstate persists and may carry an older terraform_version.
**Evidence:**
```go
// terraform.tfstate is intentionally absent: it must survive so
// that destroy can still run against existing infrastructure resources.
var terraformFilesToRemove = []string{
  "terraform.tfvars",
  "tfplan",
  "destroy.tfplan",
  ".terraform.lock.hcl",
}
```
**Fix — preferred:** refactor — after a successful `terraform destroy` (HasState() returns false / tfstate has zero resources), have cleanup remove terraform.tfstate as well. Pipe a flag through DestroyOpts → cleanup.Options.
**Rule source:** CLAUDE.md §state-and-recovery (state-file schema evolution + orphan)
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** existing 'preserve tfstate when there ARE still resources' invariant; cleanup.WorkOnly behaviour during deploy
**Estimated net LOC delta:** +12
**Severity:** minor
**Risk (of applying fix):** low
**Confidence (in finding):** medium
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** state:62cb8a95

### state:6424733c:project-marker-stale
**Cluster:** crash-recoverability
**File + line range:** internal/cli/helpers.go:115-162
**Current LOC touched:** 48
**Smell:** hasProjectMarker accepts EITHER okdctl.yaml, okdctl.env, or any terraform.tfstate. After a destroy + paranoid `rm okdctl.yaml okdctl.env`, hasProjectMarker still succeeds purely on a leftover tfstate. okdctl deploy then runs with a default config it just generated, against state from a different cluster.
**Evidence:**
```go
matches, _ := filepath.Glob(
  filepath.Join(root, "infrastructure", "terraform", "environments", "*", "terraform.tfstate"),
)
return len(matches) > 0
// terraform.tfstate alone is enough to mark this as an okdctl project — even a stale one from another cluster
```
**Fix — preferred:** refactor — when only terraform.tfstate is found (no okdctl.yaml, no okdctl.env), require an additional consistency check: parse the tfstate's outputs.cluster_name (if present) and warn the operator. OR demand okdctl.yaml as the primary marker.
**Rule source:** CLAUDE.md §state-and-recovery (state-file schema evolution + consistency)
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** OR-marker semantics for partial-deploy recovery; existing message format
**Estimated net LOC delta:** +15
**Severity:** minor
**Risk (of applying fix):** low
**Confidence (in finding):** medium
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** state:368b892b

### state:881d089e:lock-stale-host-different
**Cluster:** tf-state-atomicity
**File + line range:** internal/runlock/runlock.go:32-77
**Current LOC touched:** 10
**Smell:** runlock.Acquire writes PID/HOST/VERB/TIME but never validates the recorded HOST against os.Hostname(). On a cross-host stale lock (NFSv3, kernel-flock release does NOT propagate), the second operator hangs forever with no actionable hint.
**Evidence:**
```go
if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
  body, _ := os.ReadFile(path)
  // ... no host comparison; message lacks 'this looks stale, lock is on a different host' branch ...
}
```
**Fix — preferred:** refactor — parse HOST=<x> from conflicting body; when different from os.Hostname() append `; lock holder is on a different host (NFS-detected). On NFSv3 flock is advisory across hosts — verify with 'fuser .okdctl.lock' before deleting`.
**Rule source:** CLAUDE.md §concurrency; repo source-comment: runlock.go package doc names NFS pre-v4 caveat
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** flock-based release-on-fd-close semantics; O_NOFOLLOW symlink guard; PID/HOST/VERB body format
**Estimated net LOC delta:** +10
**Severity:** minor
**Risk (of applying fix):** low
**Confidence (in finding):** medium
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** state:4c092fce

### state:262af6e4:cleanup-no-resume-doc
**Cluster:** crash-recoverability
**File + line range:** internal/distribution/okd/cleanup/cleanup.go:1-21
**Current LOC touched:** 21
**Smell:** Package doc says 'mid-run crash leaves workDir in a partially-removed state with no resume capability'. The cleanupTracker accumulates errs into a joined error returned by StepCleanupSummary, but no per-step status table reaches the operator after a SIGINT.
**Evidence:**
```go
// Package cleanup provides utilities for removing OKD cluster artifacts.
// Cleanup is best-effort: a mid-run crash leaves workDir in a partially-removed
// state with no resume capability. Terraform state is removed last so destroy
// stays re-runnable as long as earlier steps have not corrupted it.
```
**Fix — preferred:** refactor — in runCleanup, after the orchestrator returns, log a per-step status table from orchestrator.Results() at Info level. On error, include 'subsystems still active: <names from t.errs>'.
**Rule source:** CLAUDE.md §state-and-recovery (resume diagnostic pointing at resume point); repo-counter-example: cleanup/summary.go printSummary already structurally correct
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** package doc invariant about terraform.tfstate-last ordering
**Estimated net LOC delta:** +10
**Severity:** suggestion
**Risk (of applying fix):** low
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### state:f743eaa2:iso-build-fingerprint-not-fsynced
**Cluster:** crash-recoverability
**File + line range:** internal/distribution/okd/setup/iso.go:91-108
**Current LOC touched:** 18
**Smell:** BuildCustomISOs writes the .fp-<node> fingerprint via system.AtomicWriteString AFTER buildNodeISO returns successfully — correct ordering. Crash between buildNodeISO returning and AtomicWriteString completing leaves the .iso on disk with a stale or missing fingerprint; next run rebuilds unnecessarily.
**Evidence:**
```go
if writeErr := system.AtomicWriteString(fpFile, fp, 0o644); writeErr != nil {
  p.Log.Warn("iso: failed to write build fingerprint", "node", node.Name, "err", writeErr)
  // ISO already on disk; on next run fingerprint will mismatch and rebuild
}
```
**Fix — preferred:** refactor — move the fingerprint write into buildNodeISO as the LAST line after coreos-installer succeeds. Or document the wasted-rebuild-on-crash as the deliberate trade-off.
**Rule source:** CLAUDE.md §architecture-notes (AtomicWrite + fsync)
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** buildNodeISO output path; AtomicWrite contract; coreos-installer args
**Estimated net LOC delta:** 0
**Severity:** suggestion
**Risk (of applying fix):** low
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

## Scaffolding items detected

None. All findings target either active code paths or canonical helpers; no `MEMORY.md §scaffolding` invocations needed.

## Linter-config-bug candidates

None — every finding has `adjacent_linter: "none"`. State-and-recovery findings are inherently judgment-heavy; no Go linter is shaped to catch "destroy ordering after a TF failure" or "AlreadyDone keys on directory existence not commit-on-success sentinel". This is the colonoscopy-mode skill's expected shape per SKILL.md §0.

## Skip list

No CLAUDE.md / MEMORY.md conflicts in this run. Possible-but-not-emitted candidates:

- **`internal/distribution/okd/destroy/steps.go::cleanupFilesSkipReason`** considered for a "string-typed status enum" smell but the helper is already a small read-only formatter and goconst/dupl would not flag it; deferred to `audit-code-smells`.
- **Bootstrap step's `bootstrap-destroy.tfplan` cleanup uses `defer system.SafeRemove`** which is correct. Considered flagging as "defer with error swallow" but the comment at L36-L38 names the invariant and the pattern matches `Executor.CleanupPlans`; not a finding.
- **`StartWorkerVMs` log says "no workers configured, skipping"** rather than declaring it via SkipWhen — considered a phase-idempotency finding but the orchestrator's SkipWhen is for static config-time checks; this is a runtime branch and the function-internal early return is idiomatic.

## Cluster verdicts

- **tf-state-atomicity (3 findings, 1 major / 2 minor):** AtomicWrite is universally adopted; flock-based runlock is solid. The two material gaps are no okdctl-managed state snapshot before each apply (state:48688e63 — major) and no `-lock-timeout` flag passed to terraform invocations (state:4c092fce — major). Both are surface-level fixes that don't need re-architecture.
- **state-schema-evolution (2 findings, 1 major / 1 minor):** The config YAML has a strict schemaVersion check (`SchemaVersionV1` = "v1") in loader.go:L48-L53 — correctly fails closed on mismatch. The Terraform state major-version preflight exists but only on the destroy path (state:62cb8a95 — major). Empty-tfstate orphan after destroy persists across cluster lifecycles (state:368b892b).
- **destroy-safety (3 findings, 1 major / 2 minor):** Confirmation, dry-run, scoped --only/--target, and prevent_destroy on masters are all in place. The hard problems are (a) host-file cleanup running after TF-destroy failed (state:15ba17da — major), (b) no per-target index validation (state:0f076161 — minor), and (c) Provisioner.Destroy hardcoding AutoApprove (state:c287d5c0 — minor).
- **phase-idempotency (6 findings, 2 major / 4 minor):** Largest cluster — driven by `AlreadyDone` checks that key on directory existence rather than commit-on-success sentinels (state:c19ee328 — major) and apply-time -var overrides not persisted to tfvars (state:b38ec9cc, state:b804b2ec). The bootstrap-cleanup-NonFatal=true issue (state:fb54208a — major) is the most user-visible.
- **crash-recoverability (5 findings, 0 major / 4 minor + 1 suggestion):** The deploy-state marker logic exists and surfaces phase-specific resume hints — solid design, but lacks TTL / cluster-name binding (state:b5a79fda) and produces no per-step diagnostic on cleanup failure (state:262af6e4).
- **proxmox-api-idempotency (1 finding, 0 major / 1 minor):** terraform provider owns 5xx/408/429 retry per the documented mutation invariant; the gap is around terraform-init itself (state:48688e63:proxmox-no-retry-on-init-apply). Single, narrow.

## Scope exceptions proposed

None. Files in scope per SKILL.md §3 fan-out were all read.

## Footer

**Total findings:** 20 (blocker: 0, major: 6, minor: 12, suggestion: 2)
**Scope coverage:** 34 / 34 in-scope files read in full. No fan-out sub-agents needed (single-skill audit, judgment-heavy).
**Seam deferrals:** 0 — no findings co-owned with audit-security; if a credential-during-destroy issue surfaces in a future run it would defer per `seams.md §5`.
**MEMORY.md status:** present and consulted (`feedback_audit_to_roadmap.md` confirms findings → roadmap pipeline).
**Anti-anchoring:** prior audit-state-and-recovery.jsonl was NOT read before audit derivation per AUDIT_CONVENTIONS §0.6.

To refresh linter-config-bugs.jsonl, run the aggregation command or `/audit-all`.

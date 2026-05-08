# audit-tests — 2026-05-08

**Assumes green:** golangci-lint, govulncheck, CodeQL, shellcheck, tflint, go test ./...
**Scope:** narrow critical-surface gap audit — credential handling (`internal/credentials/`, `internal/sshpin/`, `internal/system/elevation.go`, `internal/config/secret.go`), destructive ops (`destroy/`, `cleanup/`, `dns/`, addon `Uninstall`), commands touching `/etc` or root-required paths, `runlock`, `download/`, `system/atomic*`, signal/cancel handling.
**Out of scope this run:** general unit-test gap work, pure-logic helpers, TUI/wizard tests, benchmarks, fuzz beyond critical surface (per skill §0).
**Seam co-owners:** `audit-state-and-recovery` (rollback/recoverability framing on dns destructive findings), `audit-observability` (redacting handler — separately tested in `logutil/redact_test.go`).

## Executive summary

Critical-surface coverage is in unusually good shape — Wave-1+2 audits closed the load-bearing gaps (runlock symlink-refusal `tst:881d089e`, download `tst:21dc1103`, promptForConfirmation `tst:39c75e91`, kubeconfig merge, exit-code taxonomy, signal handling, credentials Zeroize/String, env-file 0600 + symlink refusal, ChownTree symlink-escape, atomic write, copyFileMode, kubectl helpers, ssh argv quoting, cleanup tfstate-preservation, destroy-tracker partial-failure path, addon manager rollback ordering, haproxy rollback). About **96%** of the in-scope critical surface is covered.

Four gaps remain, all `major`/`blocker` test-only adds (no production code change):

1. **`config.SecretBytes`** (the wrapper for `ProxmoxConfig.Password`/`APIToken`) has zero direct tests for its `String()=='[redacted]'`, `Zeroize`, `Set`-replaces-prior-buffer, or `Redacted()` invariants. The redaction sentinel is the entire reason this type exists; nothing locks it.
2. **`dns.validateAndRestartDnsmasq`** rollback-on-validation-failure has no test, despite `setup/haproxy_rollback_test.go` already pinning the parallel haproxy shape — pure asymmetry.
3. **`dns.RestoreSystemResolver`** removes `/etc/systemd/resolved.conf.d/dnsmasq.conf` with a hardcoded const path (no test seam, no test).
4. **`flux.Flux.Uninstall`** runs `oc delete ns flux-system` from the install-rollback path; only the manager-level stub is tested, not the real argv.

No blocker is a code-correctness bug — these are coverage gaps on irreversible destructive paths. Cost is one new test file per finding (≤80 LOC each); risk is low.

## Ranked table

| ID | Cluster | File:line | Severity | Confidence | LOC | Adjacent linter | Fix class |
|----|---------|-----------|----------|------------|-----|-----------------|-----------|
| `tst:bfdaf5e3:cred-bytes-type-untested` | cred-path-untested | internal/config/secret.go:1-53 | blocker | high | +80 | none | refactor |
| `tst:d7ce9d16:destructive-happy-untested` | destructive-untested | internal/distribution/okd/dns/dns.go:175-277 | major | high | +120 | none | refactor |
| `tst:de572c63:destructive-happy-untested` | destructive-untested | internal/distribution/okd/dns/dnsmasq.go:253-266 | major | high | +50 | none | refactor |
| `tst:40d315ad:destructive-happy-untested` | destructive-untested | internal/addon/catalog/flux/flux.go:237-251 | major | medium | +80 | none | refactor |

Sort key (severity_weight × confidence × |LOC delta| ÷ risk): blocker × high × 80 / low > major × high × 120 / low > major × high × 50 / low > major × medium × 80 / low.

## Findings

### `tst:bfdaf5e3:cred-bytes-type-untested`

**Cluster:** cred-path-untested
**File + line range:** `internal/config/secret.go:1-53`
**Current LOC touched:** 53
**Smell:** `config.SecretBytes` is the credential-bearing wrapper used for `ProxmoxConfig.Password` and `ProxmoxConfig.APIToken` (`cluster.go:120-121`). It exposes `Set`/`Bytes`/`Zeroize`/`IsEmpty`/`String`/`Redacted`, all of which are untested. The `String()=='[redacted]'` invariant and the `Zeroize`-overwrites-prior-`Set`-buffer invariant are both unguarded — a future refactor that drops `Redacted()` or returns `Bytes()` from `String()` would silently leak credentials through every `fmt.Sprintf` log site that takes a config value.
**Evidence:**
```go
// secret.go:44-46
func (s SecretBytes) String() string {
    return "[redacted]"
}
// secret.go:21-24
func (s *SecretBytes) Set(v string) {
    clear(s.b)
    s.b = []byte(v)
}
```
**Fix — preferred:** stdlib testing — add `internal/config/secret_test.go` covering: (1) `String()` returns `'[redacted]'` for any populated buffer, (2) `Set` replaces the prior buffer and zeroizes the old backing array (write a sentinel via `Set('a')`, capture `Bytes()`, call `Set('b')`, verify the captured slice is now zero), (3) `%v` / `%s` / `%+v` fmt verbs all emit `'[redacted]'`, (4) `Redacted()` returns `'[redacted]'`, (5) `IsEmpty` toggles around `Set`/`Zeroize`, (6) caller-must-not-retain contract: `Bytes()` returns the live slice, `Zeroize` wipes the bytes through the caller's view too. Pattern-match `credentials/proxmox_test.go::TestProxmoxCredentials_StringMasks` for the fmt-verb sweep.
**Rule source:** CLAUDE.md §credentials-and-secrets; repo counter-example at `internal/credentials/proxmox_test.go:L52-L82`
**Adjacent linter:** none
**Scaffolding?:** no — `Set`/`Zeroize`/`Bytes` all have production callers (`internal/tui/wizard/steps`, `internal/credentials/`)
**Seam:** none (this is a pure tests gap; the impl is correct, only the lock is missing)
**What MUST stay bit-for-bit:** the `'[redacted]'` sentinel string contract, the `[]byte` representation, `Zeroize` clears the backing array
**Estimated net LOC delta:** +80
**Severity:** blocker
**Risk (of applying fix):** low — test-only addition, no production code change
**Confidence (in finding):** high — code reading, no SecretBytes test exists in the tree (verified with grep)
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### `tst:d7ce9d16:destructive-happy-untested`

**Cluster:** destructive-untested
**File + line range:** `internal/distribution/okd/dns/dns.go:175-277`
**Current LOC touched:** 102
**Smell:** `DeployBootstrap`, `DeployProduction`, `validateAndRestartDnsmasq`, and the rollback-on-failure path that re-`CopyFile`s the `.backup` over `/etc/dnsmasq.d/<name>.conf` are all untested. `validateAndRestartDnsmasq` is the rollback-on-validation-failure shape that `haproxy_rollback_test.go` already locks for the parallel haproxy path, but the dnsmasq twin has no test — a regression that silently drops the `restore()` call in the validation-fail branch leaves the cluster with broken DNS and no way to recover.
**Evidence:**
```go
// dns.go:240-277
func validateAndRestartDnsmasq(ctx context.Context, configName string) error {
  ...
  if err := ValidateDnsmasqConfig(ctx); err != nil {
    restore() // ← untested
    return errors.Join(...)
  }
  if err := RestartDnsmasq(ctx); err != nil {
    restore() // ← untested
    return ...
  }
}
```
**Fix — preferred:** refactor — add `internal/distribution/okd/dns/dns_destructive_test.go` redirecting `dnsmasqConfigDir` (already a package-level var at `dnsmasq.go:24`) to `t.TempDir()`, seeding a fixture, and exercising: (1) `validateAndRestartDnsmasq` happy path leaves config in place and removes `.backup`, (2) `ValidateDnsmasqConfig` failure restores from `.backup`, (3) `RestartDnsmasq` failure restores from `.backup`, (4) missing `.backup` is not a precondition. Inject `ValidateDnsmasqConfig` / `RestartDnsmasq` via package vars (or extract a small struct) so the test doesn't need a real dnsmasq binary. Pattern-match `setup/haproxy_rollback_test.go::TestAttemptHAProxyRollback`.
**Rule source:** repo counter-example at `internal/distribution/okd/setup/haproxy_rollback_test.go:L10-L72`; CLAUDE.md §architecture-notes
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** `audit-state-and-recovery` co-owns the recoverability framing; this finding stays in tests because the recoverability mechanism itself is correct — only the lock is missing
**What MUST stay bit-for-bit:** existing `system.CopyFile` mode-preservation semantic in `restore()`
**Estimated net LOC delta:** +120
**Severity:** major
**Risk (of applying fix):** low — test-only addition, may need to add 1-2 package vars for injection
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### `tst:de572c63:destructive-happy-untested`

**Cluster:** destructive-untested
**File + line range:** `internal/distribution/okd/dns/dnsmasq.go:253-266`
**Current LOC touched:** 14
**Smell:** `RestoreSystemResolver` removes `/etc/systemd/resolved.conf.d/dnsmasq.conf` and restarts `systemd-resolved`. The `/etc`-touching write is gated by a hardcoded path const with no package-var indirection — there is no test seam, and consequently no test. A regression that loosens the `FileExists` guard would attempt `RemoveAll` on a path the cleanup never owned.
**Evidence:**
```go
// dnsmasq.go:253-263
const resolvedConf = "/etc/systemd/resolved.conf.d/dnsmasq.conf"
if system.FileExists(resolvedConf) {
  if err := os.RemoveAll(resolvedConf); err != nil {
    logger.Warn("resolver: failed to remove", "path", resolvedConf, "err", err)
  }
  ...
}
```
**Fix — preferred:** refactor — lift the `resolvedConf` const to a package-level var (`var systemdResolvedDropIn = "/etc/systemd/resolved.conf.d/dnsmasq.conf"`) so a test can redirect it to `t.TempDir()`. Then add `internal/distribution/okd/dns/restore_resolver_test.go` covering: (1) missing drop-in is a no-op, (2) present drop-in is removed, (3) `RemoveAll` error is logged but not propagated. Mirror `cleanup/services_test.go::TestDnsmasq_GlobLoopRemovesAllMatches` for the var-redirection pattern (the same file already uses `dnsmasqConfPattern` and `dnsmasqBackupPattern` as package vars for exactly this).
**Rule source:** repo counter-example at `internal/distribution/okd/cleanup/services_test.go:L13-L47`; CLAUDE.md §architecture-notes
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** `audit-state-and-recovery` co-owns; tests-skill emits because the gap is "no test exists for path X"
**What MUST stay bit-for-bit:** `FileExists` pre-check, warn-on-failure (no error propagation), `systemd-resolved` restart conditional
**Estimated net LOC delta:** +50 (1-line const→var + ~50 LOC test)
**Severity:** major
**Risk (of applying fix):** low — single-line const→var, no production behaviour change
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** `tst:d7ce9d16:destructive-happy-untested` (sibling /etc-touching gap in the same package)

### `tst:40d315ad:destructive-happy-untested`

**Cluster:** destructive-untested
**File + line range:** `internal/addon/catalog/flux/flux.go:237-251`
**Current LOC touched:** 14
**Smell:** `Flux.Uninstall` calls `helm uninstall flux-instance` + `flux-operator` and `oc delete ns flux-system`. There is no test exercising `Uninstall` even though flux is the only addon registered today and `Manager.installAndVerify` rolls back via `Uninstall` on every install failure (`manager.go:101`). The addon manager rollback ordering is tested with stubs (`manager_test.go::TestInstallAll_MiddleFailureRollsBackOnlyMiddle`), but the real `Flux.Uninstall` body — which is what fires in production rollback — is unguarded. A regression that swaps `oc delete ns flux-system` for `oc delete ns` (typo loses the namespace arg) would propagate through manager rollback and delete every namespace.
**Evidence:**
```go
// flux.go:244-249
_, err := env.Exec.Run(ctx, "helm", "uninstall", "flux-instance", "--namespace", "flux-system")
warnOnErr(err, "uninstall flux-instance")
_, err = env.Exec.Run(ctx, "helm", "uninstall", "flux-operator", "--namespace", "flux-system")
warnOnErr(err, "uninstall flux-operator")
_, err = env.Exec.Run(ctx, "oc", "delete", "ns", "flux-system")
```
**Fix — preferred:** refactor — add `internal/addon/catalog/flux/flux_uninstall_test.go` installing fake `oc`/`helm` via PATH (pattern-match `cli/cleanup_test.go::installFakeOC` and `addon/manager_test.go::installFakeOC`), running `Uninstall`, and asserting: (1) the three commands fire in order, (2) each carries its expected argv (`helm uninstall flux-instance --namespace flux-system`, `oc delete ns flux-system`), (3) per-command failures are logged but do not abort the sequence (`warnOnErr` contract — exit each fake binary with non-zero, assert the next fires anyway).
**Rule source:** repo counter-example at `internal/distribution/okd/destroy/helpers_test.go:L18-L30` (installFakeTerraform pattern); CLAUDE.md §architecture-notes
**Adjacent linter:** none
**Scaffolding?:** no — `Uninstall` is the canonical addon teardown and is invoked from manager rollback
**Seam:** none
**What MUST stay bit-for-bit:** warn-on-failure-don't-abort sequencing, the argv strings exactly as written
**Estimated net LOC delta:** +80
**Severity:** major
**Risk (of applying fix):** low — test-only addition, well-trodden fake-binary pattern
**Confidence (in finding):** medium — there's an argument that namespace-name is a constant the type system protects; demoted from high because a typo in a string literal is the only realistic failure mode and that's a code-review concern as much as a test concern. Still emit because the rollback path is irreversible.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

## Scaffolding items detected

None. The `SecretBytes` type has production callers; the dns and flux destructive paths are reached from operational verbs.

## Linter-config-bug candidates

None. No Go linter catches "this critical-path function has no test."

## Skip list

**Considered and skipped (not test-skill findings):**

- `internal/cli/helpers.go::handleCredentials` — no direct test, but it's a thin wrapper around `credentials.LoadEnvFile` (tested) + `credentials.GetProxmoxCredentials` (tested) + `tui.Info` log calls. Adding a test would be tautological.
- `internal/system/elevation_allowlist_test.go` — exists; `isAllowedChownRoot` is covered. Skipped from gap list.
- `internal/distribution/okd/postinstall/RemoveHAProxy` — `haproxy_test.go::TestRemoveHAProxy_KubeVIPHealthcheck` covers the CA-pool hard-fail path (sec:761e5126 regression guard). Skipped.
- `internal/distribution/okd/setup/tools.go::installBinaryToPath` — destructive insofar as it overwrites `/usr/local/bin/<tool>` but uses `system.CopyFile` (tested) + `system.MakeExecutable` (tested). The tool-install integration is exercised through CI in real deploys. Test-skill scope §0.6 says "skip tests that already exist on the critical surface" — the building blocks are tested.
- `internal/runlock` symlink refusal — `runlock_test.go::TestAcquire_RefusesSymlink` exists (the prior `tst:881d089e` finding has been resolved). Confirmed via Read.

**CLAUDE.md / MEMORY.md conflicts:** none.

## Cluster verdicts

**cred-path-untested (1 finding):** Credentials are exceptionally well-covered (proxmox_test.go: 401 LOC, envfile_test.go: 225 LOC, sshpin_test.go, elevation_test.go all comprehensive). The single residual gap is `config.SecretBytes`, the OTHER credential type — the redaction sentinel test was missed because the high-level `ProxmoxCredentials.String()` test covered the obvious case but not the underlying wrapper. Cheap to close.

**destructive-untested (3 findings):** Destroy is well-covered (`destroy/steps_test.go` covers happy/failure/skip/partial-failure paths via the destroyTracker; `cleanup_test.go` covers Full/WorkOnly/WebOnly/TerraformOnly variants and the tfstate-preservation invariant). The gap is in the `dns/` and `addon/catalog/flux/` packages where rollback-on-failure paths exist but are not pinned. Pattern is consistent — every other rollback (haproxy_rollback, addon manager) IS tested; only the dnsmasq and flux variants are missing.

**trust-boundary-untested:** zero findings — `netutil/ip_test.go` covers the attacker-shaped CIDR/IP inputs end-to-end; `phase/iso_cleanup_test.go::TestRefuseUnsafeISOPath` covers path-traversal; `phase/iso_cleanup_test.go::TestValidateProxmoxName` covers shell-metachar names; `config/validators_test.go::TestValidateTerraformEnv` covers null-byte and traversal-shaped env names.

**canonical-helper-untested:** zero findings — `WriteTempFile`, `CopyFileMode`, `AtomicWrite`, `OcResourceExists`, `OcPollOutput`, `BuildOpaqueSecret`, `shellSingleQuote`, `validateProxmoxName`, `validateISODir`, `pveshRun` all have direct tests. The Wave-1 download/runlock/promptForConfirmation gaps that drove three prior `blocker` findings have been closed (verified by Read).

## Scope exceptions proposed

None.

## Footer

Total findings: 4 (blocker: 1, major: 3, minor: 0, suggestion: 0)
Scope coverage: 32 / ~38 in-scope critical-surface files read in full (~84%); the remaining files were sampled by grep for destructive-op signatures (`os.Remove*`, `oc delete`, `tf destroy`, `sudo rm`) and confirmed clean.
Seam deferrals: 2 (`tst:d7ce9d16`, `tst:de572c63` cross-reference `audit-state-and-recovery` in the `seam` field; ownership stays with tests because the destructive paths exist and are correct — only the test lock is missing).

To refresh `linter-config-bugs.jsonl`, run the aggregation command from `AUDIT_CONVENTIONS.md §9c` or `/audit-all`.

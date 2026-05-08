# audit-subprocess — 2026-05-08

**Assumes green:** golangci-lint (gosec G204, noctx, errcheck, govet, staticcheck), govulncheck, CodeQL, shellcheck, tflint, go test ./...
**Scope:** every `exec.Command` / `exec.CommandContext` / `SSHRun` / `SSHRunArgv` site under `internal/**/*.go` and `cmd/**/*.go` (no production sites in `cmd/**`).
**Out of scope this run:** `internal/cli/debug_bundle_doctor.go` (`//go:build linux`, AUDIT_CONVENTIONS §2). `internal/cli/elevation.go::ensureRoot` uses `syscall.Exec`, not `exec.Command`, so it is outside the audit-subprocess rule catalog (mentioned in skip list for context). Test files are out of scope.
**Seam co-owners:** audit-security (shell-injection POLICY when ≥3 sites share a pattern; CWE-78 framing; sudo escalation pattern), audit-concurrency (cancellation-shape on long-running subprocesses).

## Executive summary

Subprocess hygiene in this tree is well-disciplined: every site outside the documented `RemoveFCOSISOFromProxmox` exemption already runs in argv mode, every site uses `CommandContext` (no `exec.Command` regressions), and the canonical `executor.Executor` pattern is used by terraform / openshift-install / scp orchestrators. Two `major` findings dominate this run: (1) `proxmox.go::probeVMEnumeration` bypasses the canonical `pveshRun` validation layer that the repo doc mandates for every pvesh interpolation, and (2) `executor.RunInteractive` lacks the `cmd.Cancel + WaitDelay` pair that `monitor.go` documents as canonical — a Ctrl-C mid-`terraform plan` SIGKILLs terraform and orphans the state lock. The remaining four findings are `minor`: three io-handling consistency gaps (one `no-cmd-env` cluster across three systemd sites, two `no-stderr-capture` sites that bypass `system.OutputCaptured`) and one `argv-unclean-path` on `upload.go::remoteISO256`, which interpolates an unvalidated filename atom into an SSHRunArgv command per the contract documented at `ssh.go:L43-50`.

## Ranked table

Sort key: severity_weight × confidence × |LOC delta| ÷ risk (blocker=4, major=3, minor=2, suggestion=1; high=3 / med=2 / low=1).

| ID | Cluster | File:line | Severity | Confidence | LOC | Adjacent linter | Fix class | ctx-ok | timeout |
|----|---------|-----------|----------|------------|-----|-----------------|-----------|--------|---------|
| sub:7b2829bb:no-cancel-func | timeout-cancel | internal/executor/executor.go:310-329 | major | high | +3 | none | refactor | ✓ | n/a (interactive) |
| sub:48688e63:argv-concat | argv-construction | internal/infrastructure/proxmox/proxmox.go:378-380 | major | high | -2 | none | refactor | ✓ | ssh-bounded |
| sub:eb479d86:argv-unclean-path | argv-construction | internal/distribution/okd/setup/upload.go:21-35 | minor | medium | +8 | none | refactor | ✓ | ssh-bounded |
| sub:e2343d2c:no-cmd-env | io-handling | internal/system/systemd.go:40-68 | minor | high | +3 | none | refactor | ✓ | ∞ (systemctl) |
| sub:a6e38cc7:no-stderr-capture | io-handling | internal/sshpin/sshpin.go:41-47 | minor | high | -1 | none | refactor | ✓ | -T 5 |
| sub:0934cf1b:no-stderr-capture | io-handling | internal/platform/packages.go:97-112 | minor | high | -1 | none | refactor | ✓ | ∞ (rpm/dpkg query) |

## Findings

---

**ID:** sub:7b2829bb:no-cancel-func
**Cluster:** timeout-cancel
**File + line range:** internal/executor/executor.go:L310-L329
**Current LOC touched:** 20
**Smell:** RunInteractive calls `exec.CommandContext` but does not set `cmd.Cancel + cmd.WaitDelay`, so on ctx cancellation Go's default behaviour is SIGKILL the process. terraform.PlanStreamed flows through this path — a kill-9 mid-plan/apply leaves `.terraform.tfstate.lock.info` orphaned because terraform never receives SIGINT to release it gracefully. `install/monitor.go::defaultStartMonitorCmd` at L25-L33 is the canonical pattern this site should mirror.
**Evidence:**
```go
func (e *Executor) RunInteractive(ctx context.Context, name string, args ...string) error {
    cmd := exec.CommandContext(ctx, name, args...)
    if e.WorkDir != "" { cmd.Dir = e.WorkDir }
    cmd.Env = e.buildEnv()
    cmd.Stdin = os.Stdin
    cmd.Stdout = e.Stdout
    cmd.Stderr = e.Stderr
    err := cmd.Run()
```
**Fix — preferred:** refactor — set `cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGINT) }` (terraform's documented soft-cancel) and `cmd.WaitDelay = 30 * time.Second`, mirroring `monitor.go`. Apply the same pair to Run/RunStreamed if they ever spawn a child holding external locks.
**Rule source:** CLAUDE.md §concurrency (graceful subprocess cancel pattern); repo-counter-example: internal/distribution/okd/install/monitor.go:L30-L33; Go stdlib doc os/exec#Cmd.Cancel.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-concurrency
**What MUST stay bit-for-bit:** existing argv/Env/Dir/Stdin wiring; the os.Stdin → cmd.Stdin pipe used for interactive prompts.
**Estimated net LOC delta:** +3
**Severity:** major (rubric §4/canonical-helper-on-critical-path)
**Risk (of applying fix):** low — additive; existing call sites get strictly better cancel semantics.
**Confidence (in finding):** high — the pattern is documented in CLAUDE.md and used live in monitor.go; the divergence is mechanical.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

---

**ID:** sub:48688e63:argv-concat
**Cluster:** argv-construction
**File + line range:** internal/infrastructure/proxmox/proxmox.go:L378-L380
**Current LOC touched:** 3
**Smell:** probeVMEnumeration calls `SSHRunArgv` with `"/nodes/"+p.node+"/qemu"` built by string concat, bypassing `pveshRun` (the canonical helper that re-runs `validateProxmoxName` on the node atom — see pvesh.go:L13). pvesh.go documents *"New pvesh callers go through pveshRun and inherit this guard automatically — do not interpolate names into ssh command strings without it"* — this site is the policy violation.
**Evidence:**
```go
result, err := phase.SSHRunArgv(ctx, p.sshExec, host, p.knownHostsPath,
    "pvesh", "get", "/nodes/"+p.node+"/qemu", "--output-format", "json",
)
```
**Fix — preferred:** refactor — replace the inline SSHRunArgv with a call to `phase.pveshRun` (or expose a public `PveshRun` helper if the proxmox package can't reach the unexported one). Net: route this probe through the same validation layer iso_cleanup uses.
**Rule source:** CLAUDE.md §architecture-notes (SSH shell policy); repo-counter-example: internal/distribution/okd/phase/pvesh.go:L11-L18; repo-counter-example: internal/distribution/okd/phase/iso_cleanup.go:L52-L69 (validateProxmoxName); CWE-78.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-security (security owns the policy framing if ≥3 sites share the pattern; this is the second site, so subprocess emits per-site)
**What MUST stay bit-for-bit:** phase.SSHRunArgv argv-mode contract; ctx propagation; existing logger semantics.
**Estimated net LOC delta:** -2
**Severity:** major (rubric §4/canonical-helper-on-critical-path)
**Risk (of applying fix):** low — pveshRun's argv shape is identical except for the validation prefix; extracting a public helper is mechanical.
**Confidence (in finding):** high — the contract is written down at pvesh.go:L11-L13.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

---

**ID:** sub:eb479d86:argv-unclean-path
**Cluster:** argv-construction
**File + line range:** internal/distribution/okd/setup/upload.go:L21-L35
**Current LOC touched:** 15
**Smell:** remoteISO256 builds `target = remotePath + "/" + filename` and passes it to `SSHRunArgv` as a sha256sum positional arg. ssh.go:L43-L50 documents *"argv mode does NOT bypass the shell. Callers MUST validate every atom for shell metacharacters before calling."* `filename` is `filepath.Base` of an `os.ReadDir` entry under `WorkDir/custom-isos` so a hostile filesystem could plant `foo;rm.iso` and the remote root shell would split it on `;`. The `--` separator protects against flag injection, not against shell metacharacters interpreted by the remote sshd login shell.
**Evidence:**
```go
target := remotePath + "/" + filename
result, err := phase.SSHRunArgv(ctx, exec, host, knownHostsPath, "sha256sum", "--", target)
```
**Fix — preferred:** refactor — either (a) reject filenames containing shell metacharacters in `collectISOFiles` before adding them to `isoFiles` (cheapest), or (b) add a `validateRemoteFilename` helper similar to `validateProxmoxName`/`validateISODir` and call it before every SSHRunArgv interpolation. Counter-example: phase/iso_cleanup.go layers `refuseUnsafeISOPath` + `shellSingleQuote`.
**Rule source:** repo-counter-example: internal/distribution/okd/phase/ssh.go:L43-L50 (SSHRunArgv contract); repo-counter-example: internal/distribution/okd/phase/iso_cleanup.go:L29-L41 (refuseUnsafeISOPath); CWE-78.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-security (umbrella "≥3 SSHRunArgv sites without atom-validation" candidate if more land — currently only this + proxmox.go:378)
**What MUST stay bit-for-bit:** phase.SSHRunArgv contract; `--` separator before target; ctx propagation.
**Estimated net LOC delta:** +8
**Severity:** minor — local FS is the trust boundary and the filenames originate from `collectISOFiles` (`.iso` extension filter); exploitability requires a hostile actor with write access to `WorkDir/custom-isos`, but the doc-stated invariant *"callers MUST validate"* is not honoured here.
**Risk (of applying fix):** low — adding a validator before a single SSHRunArgv is additive; same shape as existing iso_cleanup guards.
**Confidence (in finding):** medium — exploitability hinges on an external write to a tool-owned directory; the policy violation is unambiguous.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

---

**ID:** sub:e2343d2c:no-cmd-env
**Cluster:** io-handling
**File + line range:** internal/system/systemd.go:L40-L68 (3 sites; extras at L56-L57 and L67-L68)
**Current LOC touched:** 15
**Smell:** systemd.go has three `exec.CommandContext` sites (`ManageService` ServiceStatus, `IsServiceActive`, `IsServiceEnabled`) that bypass the `executor.FilterParentEnv` allowlist used by sibling helper `RunCaptured` at exec.go:L54. Under sudo re-exec these run as root, inheriting the unfiltered parent env (any GITHUB_TOKEN / GH_TOKEN / GIT_ASKPASS the operator's shell exported). Inconsistent with the env-isolation invariant the rest of the system package upholds.
**Evidence:**
```go
// systemd.go:40
cmd := exec.CommandContext(ctx, "systemctl", "is-active", serviceName)
return cmd.Run()
// systemd.go:56
cmd := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", serviceName)
return cmd.Run() == nil
```
**Fix — preferred:** refactor — set `cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist)` at each site, or route through `system.RunCaptured` and let it handle exit-code → bool conversion via `errors.As` against `*SubprocessError`. The `--quiet` flag means stderr is already silent; capturing isn't required.
**Rule source:** repo-counter-example: internal/system/exec.go:L52-L65 (RunCaptured filters env); repo-counter-example: internal/executor/executor.go:L93-L125 (DefaultEnvAllowlist); CLAUDE.md §credentials-and-secrets.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** non-Linux `runtime.GOOS != osLinux` short-circuit; ctx propagation.
**Estimated net LOC delta:** +3
**Severity:** minor — systemctl rarely consumes GH_TOKEN-style vars, so the user-harm path is narrow; the inconsistency with sibling RunCaptured is the real cost.
**Risk (of applying fix):** low — the env allowlist already includes everything systemd needs (`PATH`, `SYSTEMD_*` is not on the list and is not needed for unit-action paths).
**Confidence (in finding):** high — the divergence is mechanical and the canonical helper is one import away.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

---

**ID:** sub:a6e38cc7:no-stderr-capture
**Cluster:** io-handling
**File + line range:** internal/sshpin/sshpin.go:L41-L47
**Current LOC touched:** 7
**Smell:** runKeyscan calls `cmd.Output()` which returns stdout but lets stderr inherit the parent fd (TTY in interactive runs, /dev/null in piped runs). When `ssh-keyscan -T 5` fails because the remote port 22 is closed, the diagnostic line lands on the operator's terminal but never reaches the returned err — the wrapper just becomes `ssh-keyscan host: exit status 1`. Sibling `system.OutputCaptured` at exec.go:L73 captures stderr into the typed error.
**Evidence:**
```go
out, err := exec.CommandContext(ctx, "ssh-keyscan", "-T", "5", host).Output()
if err != nil {
    return "", err
}
return string(out), nil
```
**Fix — preferred:** refactor — swap to `system.OutputCaptured(ctx, "ssh-keyscan", "-T", "5", host)` so stderr flows into a `*SubprocessError` and the structured log handler can redact + render it. Net: same return shape, better error.
**Rule source:** repo-counter-example: internal/system/exec.go:L67-L86 (OutputCaptured); Uber Go §Error Wrapping.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** `ssh-keyscan -T 5` timeout flag; ctx propagation; the bare-hostname argv-positional shape (no `-H` so fingerprints are deterministic).
**Estimated net LOC delta:** -1
**Severity:** minor — degraded operator UX on connection failure; not a security or correctness bug.
**Risk (of applying fix):** low — `system.OutputCaptured` already returns `[]byte, error`.
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

---

**ID:** sub:0934cf1b:no-stderr-capture
**Cluster:** io-handling
**File + line range:** internal/platform/packages.go:L97-L112
**Current LOC touched:** 16
**Smell:** IsInstalled calls `cmd.Output()` which captures stdout but discards stderr; a non-ExitError failure (queryCmd missing from PATH, ctx cancellation, I/O fault) yields `fmt.Errorf("%s query: %w", m.queryCmd, err)` with no diagnostic context. Sibling Install/Remove use `system.RunCaptured` which preserves stderr inside `*SubprocessError`.
**Evidence:**
```go
cmd := exec.CommandContext(ctx, m.queryCmd, args...) //nolint:gosec ...
output, err := cmd.Output()
if err != nil {
    var exitErr *exec.ExitError
    if errors.As(err, &exitErr) { return false, nil }
    return false, fmt.Errorf("%s query: %w", m.queryCmd, err)
}
```
**Fix — preferred:** refactor — replace `cmd.Output()` with `system.OutputCaptured(ctx, m.queryCmd, args...)` and unwrap `*SubprocessError` when distinguishing ExitError vs not-found. Same gosec exemption (queryCmd is constructor-set) but stderr now flows.
**Rule source:** repo-counter-example: internal/system/exec.go:L67-L86 (OutputCaptured); repo-counter-example: internal/platform/packages.go:L60-L67 (Install routes through RunCaptured).
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the ExitError → (false, nil) branch; ctx propagation; the //nolint:gosec annotation reasoning.
**Estimated net LOC delta:** -1
**Severity:** minor — diagnostic-only quality issue; an `exec.ExitError` still maps to (false, nil) which is the dominant happy-path.
**Risk (of applying fix):** low
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

## Scaffolding items detected

None this run. The unused `destroyDirect` path in `terraform.go:L298-L318` is annotated as scaffolding ("retained as the canonical 'emergency destroy' path so the argv shape stays locked under regression coverage"); it is not a subprocess finding because the subprocess hygiene of that path is correct (routes through `executor.Executor.Run`).

## Linter-config-bug candidates

None this run. Every finding has `adjacent_linter: none` — gosec G204 is enabled but is satisfied at every site (literal cmd, or `//nolint:gosec` on the one constructor-set name in `packages.go:99`). The findings are about repo-canonical-helper hygiene that no off-the-shelf linter encodes.

## Skip list

- **`internal/distribution/okd/phase/ssh.go::SSHRun` (L33-L41).** Looks like `sh-c-variable` because it forwards a constructed cmd string to ssh which the remote shell evaluates. CLAUDE.md §architecture-notes pins this as the policy-of-record helper; ssh.go:L43-L50 + iso_cleanup.go:L209-L213 document the layering pattern (`validateXxx` + `shellSingleQuote`). Skipped per AUDIT_CONVENTIONS §17.
- **`internal/distribution/okd/phase/iso_cleanup.go::RemoveFCOSISOFromProxmox` (L214-L263).** Uses SSHRun with a constructed `findCmd`/`rm -f` string. CLAUDE.md explicitly names this function as the only allowed `sh -c` site, and it layers `validateISODir`, `refuseUnsafeISOPath`, and `shellSingleQuote`. Skipped per AUDIT_CONVENTIONS §17.
- **`internal/cli/elevation.go::ensureRoot` (L114).** Uses `syscall.Exec`, not `exec.Command*`, so it sits outside the audit-subprocess rule catalog. Mentioned for context: argv shape (`["sudo", "--", self, ...os.Args]`) is correct, env is filtered through the canonical allowlist, and the `//nolint:gosec` annotation is justified. The cobra annotation gating is in scope for audit-cli-ux / audit-security, not subprocess.
- **`internal/cli/debug_bundle_doctor.go::collectDoctorOutput` (L17-L28).** `//go:build linux` tag → out of scope per AUDIT_CONVENTIONS §2. The pattern (`os.Executable()` re-exec with literal argv, `CombinedOutput` via shared buffer for support-bundle text) is correct.
- **`internal/distribution/okd/install/monitor.go::defaultStartMonitorCmd` (L24-L43).** This is the *canonical reference implementation* (cmd.Cancel + WaitDelay) the rest of the audit cites — not a finding.

## Cluster verdicts

- **argv-construction.** Two sites flagged. proxmox.go:378 is the only `major` argv-concat — and it's a documented policy violation. upload.go:23 is a softer `minor` because the controlled FS surface narrows exploitability. The repo's argv-validation primitives (`validateProxmoxName`, `validateISODir`, `refuseUnsafeISOPath`, `shellSingleQuote`) are well-shaped; the gap is at the call-site discipline, not the toolkit.
- **shell-c.** No findings. The sole `sh -c`-style site is the documented `RemoveFCOSISOFromProxmox` exemption.
- **ctx-propagation.** No findings. Every reviewed site uses `exec.CommandContext` with a real ctx from the caller; no `context.TODO()` slippage, no `exec.Command` regressions.
- **timeout-cancel.** One `major` finding: executor.RunInteractive lacks the cmd.Cancel/WaitDelay pair canonically required for terraform/long-running children. Other long-running sites (openshift-install via monitor.go, terraform.run via Executor.Run with internal buffers, scp via RunInteractive — wait, scp is on RunInteractive too; same finding scope).
- **coreutils-shellout.** No findings. systemd is the only coreutils-adjacent shellout and there is no Go-stdlib substitute (sd-bus is not in stdlib). `ssh-keyscan` and `sha256sum` over ssh do not have a Go equivalent reachable from the orchestrator.
- **io-handling.** Three `minor` findings (one umbrella with two extras). The pattern is "use the canonical sibling helper in `system/exec.go` instead of bare `exec.CommandContext` + `Output()`/`Run()`."

## Scope exceptions proposed

None this run. The audit honoured the standard exclusions plus the `//go:build linux` exclusion in §2.

## Footer

Total findings: **6** (blocker: 0, major: 2, minor: 4, suggestion: 0).
Severity distribution sanity check: 33% major, 67% minor → well below the §4 40%-blocker abuse ceiling.
Scope coverage: **15 / 15 production .go files with subprocess sites read in full** (no sub-agent dispatch — total surface ~700 LOC fit a single read-pass). 1 file (`debug_bundle_doctor.go`) noted as out-of-scope build-tag-gated; 1 file (`cli/elevation.go`) noted as syscall.Exec, outside skill rule catalog. MEMORY.md present and consulted; no scaffolding-rule applications this run.
Seam deferrals: **2** (sub:48688e63 → audit-security, sub:eb479d86 → audit-security). audit-security may emit a single umbrella "argv interpolation without atom validation across multiple ssh sites" finding referencing both IDs once a third site lands; with two sites the per-site emit is canonical.
Validation: every JSONL row validated against `finding-schema.json` (id pattern, required fields, severity_reason on majors, adjacent_linter pattern, smell length 10-600). No drops.

To refresh `linter-config-bugs.jsonl`, run the aggregation command in AUDIT_CONVENTIONS §9c or `/audit-all`.

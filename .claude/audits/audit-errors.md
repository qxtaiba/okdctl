# audit-errors — 2026-05-08

**Assumes green:** golangci-lint (errcheck, errorlint, govet, staticcheck,
nilerr), govulncheck, CodeQL, shellcheck, tflint, go test ./...
**Scope:** every error-creation / wrapping / comparison site in
`internal/**/*.go` and `cmd/**/*.go` (excluding the global out-of-scope
list in AUDIT_CONVENTIONS.md §2). 179 in-scope `.go` files; 496 distinct
`fmt.Errorf|errors.New|errors.Is|errors.As|errors.Join|errors.Unwrap`
sites read in full.
**Out of scope this run:** `internal/tui/wizard/**` (per §2);
`_test.go` files; `iso.go` Linux-tagged tree.
**Seam co-owners:** audit-cli-ux (exit-code taxonomy — owns the table;
this audit owns each typed-err→exit-code mapping per seams.md §4),
audit-observability (slog sink redaction — owns the handler; this audit
owns the chain — seams.md §3), audit-concurrency (ctx.Done shape —
owns the loop; this audit owns identity preservation — seams.md §11).

## Executive summary

The `errtypes` vocabulary (5 concepts: ConfigError, NetworkError,
ClusterError, AuthError, UsageError + 3 sentinel `Err*` values) is
broadly correct, well-redacted at the type layer, and consistently
chained via `Unwrap` so `errors.Is`/`As` traversal works end-to-end.
Construction-site coverage is healthy (247 references across 30
distribution + cli + addon files). The findings cluster into three
seams: (1) `executor.ExitError` carries subprocess stderr without a
`Redacted()` projection where the sibling `system.SubprocessError`
already does — a credential-leak axis at the chain layer; (2)
cancellation identity is dropped on a handful of subprocess-cancel
paths (terraform apply, `executor.RunChecked` non-zero exit), so SIGINT
exits 1 instead of 130; (3) ~15 cli-RunE call sites and most phase
step bodies return bare `fmt.Errorf` instead of typing into `errtypes.*`,
silently bypassing the documented exit-code contract. No string-sniffing
on stdlib error text. No credential-bearing values in error fields.
No `err == sentinel` direct equality.

## Ranked table

| ID | Cluster | File:line | Sev | Conf | LOC | Adj. linter | Fix |
|----|---------|-----------|-----|------|-----|-------------|-----|
| err:7b2829bb:exit-error-no-redact | redaction-in-error | internal/executor/executor.go:187-198 | major | high | 4 | none | refactor |
| err:48688e63:cancel-identity-lost-on-tf-apply | cancellation-identity | internal/infrastructure/proxmox/proxmox.go:195-203 | major | high | 6 | none | refactor |
| err:fd2125dd:cli-bare-errors-skip-typed-mapping | sentinel-vs-typed | internal/cli/addon.go:65-255 (+12) | major | high | 8 | none | refactor |
| err:c19ee328:phase-step-bare-fmt-errorf | sentinel-vs-typed | internal/distribution/okd/setup/steps.go:380-430 (+50) | minor | med | 12 | none | refactor |
| err:fde34e0c:exit-error-no-ctx-identity | cancellation-identity | internal/cluster/k8s.go:108-127 (+2) | minor | med | 5 | none | refactor |
| err:6424733c:env-file-double-context | wrapping | internal/cli/helpers.go:53-55 | minor | high | 3 | none | refactor |
| err:881d089e:runlock-untyped-errors | sentinel-vs-typed | internal/runlock/runlock.go:47-52 | minor | high | 6 | none | refactor |
| err:62cb8a95:state-lock-hint-drops-init-cause | wrapping | internal/distribution/okd/destroy/helpers.go:124-129 | minor | high | 5 | none | refactor |
| err:5013fea6:auth-error-string-sniffing | string-sniffing | internal/distribution/okd/setup/release_extract.go:92-141 | sugg | high | 12 | none | refactor |
| err:ddf885f4:install-all-bare-ctx-err | cancellation-identity | internal/addon/manager.go:85-113 | sugg | med | 4 | none | refactor |
| err:9f8e7d6c:errtypes-vocab-cert-pending | domain-vocabulary | internal/errtypes/errtypes.go:1-111 | sugg | low | 0 | none | refactor (scaffolding) |

Sort: severity_weight × confidence × |LOC delta| ÷ risk
(blocker=4, major=3, minor=2, suggestion=1; high=3 / med=2 / low=1).

## Domain vocabulary table

| `errtypes` symbol | Type | Production callers | Verdict |
|-------------------|------|--------------------|---------|
| `ConfigError` | struct | 131 | healthy — used in 18 packages |
| `ClusterError` | struct | 81 | healthy — install/postinstall/destroy/addon |
| `NetworkError` | struct | 16 | healthy — download/upload/proxmox |
| `AuthError` | struct | 12 | healthy — credentials/elevation/loader |
| `UsageError` | struct | 2 | minimal — only cli/root SetFlagErrorFunc and tests; **err:fd2125dd** flags ~6 missing-wrap sites |
| `ErrConfigMissing` | sentinel | 3 | correct — wrapped through `ConfigError.Err` |
| `ErrPullSecretInvalid` | sentinel | 2 | correct — wrapped through `AuthError.Err` |
| `ErrSudoMissing` | sentinel | 2 | correct — wrapped via `errors.Join(err, ErrSudoMissing)` (cli/elevation.go:99) |

## Findings

### err:7b2829bb:exit-error-no-redact

**ID:** err:7b2829bb:exit-error-no-redact
**Cluster:** redaction-in-error
**File:** `internal/executor/executor.go:187-198`
**Current LOC touched:** 4
**Smell:** `executor.ExitError` carries `Stderr string` but does not
implement `Redacted() any`. Sibling type `system.SubprocessError` does.
Subprocess stderr from `oc`/`kubectl`/`terraform`/`sops` may carry
tokens, partial decrypted secrets, or endpoint URLs; when the chain
reaches a slog sink as a structured attr, `RedactHandler` cannot scrub
because no `Redacted()` projection exists.
**Evidence:**
```go
type ExitError struct {
    Command  string
    ExitCode int
    Stderr   string
}
func (e *ExitError) Error() string {
    return fmt.Sprintf("%s failed (exit %d): %s", e.Command, e.ExitCode, strings.TrimSpace(e.Stderr))
}
```
**Fix — preferred:** add `Redacted() any` returning `{Command, ExitCode}`
(drop `Stderr`). Mirror `system.SubprocessError.Redacted` shape.
`terraform.ExecError` aliases `executor.ExitError` so it inherits.
**Rule source:** CLAUDE.md §credentials-and-secrets; repo-counter-example
`internal/system/exec.go:L40-L44`; Uber §Error Types
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-observability (related: `obs:6424733c:fmt-sprintf-message-pattern`)
**What MUST stay bit-for-bit:** `Error()` text format (tests parse);
field shape (`errors.As` consumers).
**Estimated net LOC delta:** +4
**Severity:** major
**Severity reason:** rubric §4/credential-exposure
**Risk:** low — Redacted is purely additive; existing `errors.As` paths
unaffected.
**Confidence:** high
**CLAUDE.md / MEMORY.md conflict?:** no

### err:48688e63:cancel-identity-lost-on-tf-apply

**ID:** err:48688e63:cancel-identity-lost-on-tf-apply
**Cluster:** cancellation-identity
**File:** `internal/infrastructure/proxmox/proxmox.go:195-203`
**Smell:** On terraform-apply cancellation, `applyErr` is the
`executor.ExitError` from a SIGTERM'd subprocess; it carries no
`context.Canceled` identity. The wrap forwards `applyErr` only:
`fmt.Errorf("terraform apply interrupted: %w", applyErr)`. Downstream
`cli/root.go::signalExitCode` (L183) gates SIGINT→130 / SIGTERM→143
on `errors.Is(err, context.Canceled || DeadlineExceeded)` — false
here, so a Ctrl-C during terraform apply currently exits 1.
**Evidence:**
```go
applyErr := p.terraformExec.Apply(ctx, applyOpts)
if applyErr != nil {
    if errors.Is(ctx.Err(), context.Canceled) {
        return nil, fmt.Errorf("terraform apply interrupted: %w", applyErr)
    }
```
**Fix — preferred:** wrap `ctx.Err()` instead (or alongside) `applyErr`
via `errors.Join(ctx.Err(), applyErr)`. `install/monitor.go:L68` is the
canonical shape: `fmt.Errorf("...: %w", ctx.Err())`.
**Rule source:** CLAUDE.md §architecture-notes / concurrency canonical
patterns; repo-counter-example `internal/distribution/okd/install/monitor.go:L68`;
Uber §Error Wrapping
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-concurrency
**What MUST stay bit-for-bit:** the warn-log at L201; operator-facing
message tone.
**Estimated net LOC delta:** 0
**Severity:** major
**Severity reason:** rubric §4/un-idiomatic-pattern — concrete user
impact: SIGINT during the most common destructive step exits 1, not 130.
**Risk:** low — purely error-chain adjustment; no behavior change for
non-cancel paths.
**Confidence:** high
**CLAUDE.md / MEMORY.md conflict?:** no

### err:fd2125dd:cli-bare-errors-skip-typed-mapping

**ID:** err:fd2125dd:cli-bare-errors-skip-typed-mapping
**Cluster:** sentinel-vs-typed
**File:** `internal/cli/addon.go:65-255` plus 4 extras (`completion.go:45`,
`releases.go:124-171`, `kubeconfig.go:49`, `elevation.go:104`); 12 more
across the cli tree.
**Smell:** CLI subcommand `RunE` returns plain `fmt.Errorf` for what are
clearly typed-error categories: usage misconfig (`--all` + named addon
mutually exclusive, unknown shell, invalid `--channel`/`--output`),
missing config artifact (kubeconfig not found), or executable-resolution
failure. `exitCodeFor` (cli/root.go:L192-L231) maps these to specific
codes — but these sites fall to exit 1.
**Evidence:**
```go
addon.go:65:  return fmt.Errorf("--all and a named addon are mutually exclusive")
addon.go:255: return fmt.Errorf("%d addon(s) failed verification", failed)
completion.go:45: return fmt.Errorf("unknown shell %q", args[0])
kubeconfig.go:49: return fmt.Errorf("kubeconfig not found at %s; run `okdctl deploy` first", src)
releases.go:162: return fmt.Errorf("invalid --channel %q (want stable|all)", ch)
```
**Fix — preferred:** wrap each in the right errtypes value:
- usage failures → `&errtypes.UsageError{Msg: "..."}` (exit 64)
- `kubeconfig.go:49` → `&errtypes.ConfigError{Msg: ..., Err: errtypes.ErrConfigMissing}` (exit 66)
- `addon.go:255` → `&errtypes.ClusterError{...}` (exit 4)
- `elevation.go:104` → `&errtypes.ConfigError{Msg: ..., Err: err}` (exit 2)
**Rule source:** repo-counter-example `internal/cli/elevation.go:L97`
(canonical AuthError); `internal/cli/helpers.go:L41` (canonical
ConfigError + ErrConfigMissing); CLAUDE.md §architecture-notes (exit
code contract)
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-cli-ux (cli-ux owns the taxonomy table; this audit owns
the per-site mapping)
**What MUST stay bit-for-bit:** user-facing messages (don't reword);
existing `Err*` sentinel chains.
**Estimated net LOC delta:** +30 (~15 sites × 2 lines)
**Severity:** major
**Severity reason:** rubric §4/un-idiomatic-pattern — operator impact:
shell scripts cannot distinguish 'wrong flag' from 'cluster failed';
both return 1.
**Risk:** low — mechanical wrap; behavior preserved.
**Confidence:** high
**CLAUDE.md / MEMORY.md conflict?:** no

### err:c19ee328:phase-step-bare-fmt-errorf

**ID:** err:c19ee328:phase-step-bare-fmt-errorf
**Cluster:** sentinel-vs-typed
**File:** `internal/distribution/okd/setup/steps.go:380-430` plus
extras across `setup/tools.go`, `setup/iso.go`, `setup/apache.go`,
`addon/catalog/flux/flux.go`, `addon/helpers.go`. 50+ additional
occurrences.
**Smell:** Phase step bodies return plain `fmt.Errorf("failed to X: %w", err)`
for what semantically map to `ConfigError`/`ClusterError`/`NetworkError`.
Orchestrator forwards as-is to cli, which falls to exit 1.
`addon.Manager` wraps step bodies into `ClusterError` at `manager.go:L123`,
shielding addon catalog code — but every direct phase step (setup /
install / postinstall) returns naked errors.
**Evidence:**
```go
setup/steps.go:380: return fmt.Errorf("failed to ensure openshift manifests directory: %w", err)
setup/iso.go:180:   return "", fmt.Errorf("failed to marshal installer trigger ignition: %w", err)
setup/tools.go:209: return fmt.Errorf("failed to download %s: %w", spec.name, err)
flux.go:80:         return fmt.Errorf("flux: invalid settings: %w", err)
addon/helpers.go:61: return "", fmt.Errorf("marshal opaque secret %s/%s: %w", namespace, name, err)
```
**Fix — preferred:** wrap at the phase-orchestrator entry
(`Phase.Execute` / `Provisioner.Install`) so each phase has one
`ClusterError` boundary. Lossy on semantic precision but high
ROI; per-site wrap is the correct-but-expensive alternative.
**Rule source:** CLAUDE.md §architecture-notes (exit-code contract);
repo-counter-example `internal/addon/manager.go:L123`;
`internal/distribution/okd/postinstall/verify.go:L122-L143`
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-cli-ux (related: `err:fd2125dd:cli-bare-errors-skip-typed-mapping`)
**What MUST stay bit-for-bit:** `errors.Is`/`As` traversal — every
existing `%w` must stay; the wrap MUST sit at typed-error.Err.
**Estimated net LOC delta:** +10 (orchestrator-boundary approach)
**Severity:** minor
**Severity reason:** un-idiomatic pattern with bounded impact; orchestrator
phase failures are typically already user-fatal so 1 vs 4 doesn't change
retry decisions.
**Risk:** low
**Confidence:** medium
**CLAUDE.md / MEMORY.md conflict?:** no

### err:fde34e0c:exit-error-no-ctx-identity

**ID:** err:fde34e0c:exit-error-no-ctx-identity
**Cluster:** cancellation-identity
**File:** `internal/cluster/k8s.go:108-127` plus
`internal/executor/executor.go:297-367`,
`internal/distribution/okd/phase/kubectl.go:29-42`,
`internal/distribution/okd/setup/release_extract.go:121-133`,
`internal/distribution/okd/setup/upload.go:24-34`.
**Smell:** Every site that constructs an `executor.ExitError` on a
non-zero exit code does so without consulting `ctx.Err()`. A subprocess
SIGTERM'd via `cmd.Cancel` that exits non-zero produces an `ExitError`
chain with no `context.Canceled` identity. Downstream
`errors.Is(err, context.Canceled)` returns false; `signalExitCode` maps
the SIGINT to exit 4 (ClusterError) instead of 130. install/monitor.go
has the canonical fix shape (check `ctx.Err` first).
**Evidence:**
```go
// k8s.go:108-127
if result.ExitCode != 0 {
    return &executor.ExitError{Command: c.CLI + " " + subcommand(args), ExitCode: result.ExitCode, Stderr: stderr}
}
// executor.go:302: same shape
```
**Fix — preferred:** centralise via a helper
`func newExitError(ctx context.Context, cmd string, code int, stderr string) error`
that checks `ctx.Err()` first. 7 call sites adopt it.
**Rule source:** repo-counter-example
`internal/distribution/okd/install/monitor.go:L55-L69`;
Uber §Error Wrapping; Go proverb 'errors are values'
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-concurrency (related: `err:48688e63`)
**What MUST stay bit-for-bit:** `executor.ExitError` type; `Stderr`
field shape (callers `errors.As`).
**Estimated net LOC delta:** +6
**Severity:** minor
**Severity reason:** narrow timing window; most production cancellations
exit via `cmd.Wait` returning the ctx err naturally. Recorded because
err:48688e63 is the same root cause and a centralised fix covers both.
**Risk:** low
**Confidence:** medium
**CLAUDE.md / MEMORY.md conflict?:** no

### err:6424733c:env-file-double-context

**ID:** err:6424733c:env-file-double-context
**Cluster:** wrapping
**File:** `internal/cli/helpers.go:53-55` + `cli/deploy.go:147,246`
**Smell:** `LoadEnvFile` callers wrap with `'load env file <path>: %w'`
on top of an inner `ConfigError`/`AuthError` that already names the path.
Result: double-context like `"load env file /home/x/okdctl.env: failed
to open env file /home/x/okdctl.env: open /home/x/okdctl.env: permission
denied"`. `errors.As` traversal still finds the typed inner so exit code
is preserved — cost is purely operator-readability.
**Evidence:**
```go
helpers.go:53-55:
  if err := credentials.LoadEnvFile(envPath); err != nil {
      return nil, fmt.Errorf("load env file %s: %w", envPath, err)
  }
// inner ConfigError already says 'failed to open env file <envPath>: ...'
```
**Fix — preferred:** drop the outer wrap and return `err` directly.
Three sites.
**Rule source:** Uber §Error Wrapping (don't duplicate context);
repo-counter-example `internal/cli/deploy.go:L262-L264` (handleCredentials
returns inner err directly)
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** `errors.Is`/`As` traversal to inner
typed types.
**Estimated net LOC delta:** -6
**Severity:** minor
**Severity reason:** un-idiomatic-pattern; readability cost only.
**Risk:** low
**Confidence:** high
**CLAUDE.md / MEMORY.md conflict?:** no

### err:881d089e:runlock-untyped-errors

**ID:** err:881d089e:runlock-untyped-errors
**Cluster:** sentinel-vs-typed
**File:** `internal/runlock/runlock.go:47, 52`
**Smell:** `Acquire` returns `ConfigError` for symlink-refusal (L42) and
lock-conflict (L62), but bare `fmt.Errorf` for the lstat (L47) and
OpenFile (L52) syscall failures — falls to exit 1 instead of 2/5.
**Evidence:**
```go
} else if !errors.Is(err, os.ErrNotExist) {
    return nil, fmt.Errorf("runlock: lstat %s: %w", path, err)
}
f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
if err != nil {
    return nil, fmt.Errorf("runlock: open %s: %w", path, err)
}
```
**Fix — preferred:** wrap each in `&errtypes.ConfigError{Msg: ..., Err: err}`
to match L42/L62 siblings.
**Rule source:** repo-counter-example `internal/runlock/runlock.go:L42`
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none (related: `err:fd2125dd`)
**What MUST stay bit-for-bit:** `%w` wrapping for `os.ErrPermission`
callers.
**Estimated net LOC delta:** +6
**Severity:** minor
**Severity reason:** low blast-radius; rare disk-state condition.
**Risk:** low
**Confidence:** high
**CLAUDE.md / MEMORY.md conflict?:** no

### err:62cb8a95:state-lock-hint-drops-init-cause

**ID:** err:62cb8a95:state-lock-hint-drops-init-cause
**Cluster:** wrapping
**File:** `internal/distribution/okd/destroy/helpers.go:124-129`
**Smell:** When `tf.Init` fails AND a state lock file is present, the
original Init error is silently discarded (`return hint`). The operator
sees the hint message but loses the actual init failure context (which
may itself be the cause of the orphan lock — corrupted `.terraform/`).
**Evidence:**
```go
if err := tf.Init(ctx); err != nil {
    if hint := stateLockHint(terraformDir); hint != nil {
        return hint   // <-- err is dropped
    }
    return &errtypes.ClusterError{Msg: "terraform init failed", Err: err}
}
```
**Fix — preferred:** `return errors.Join(hint, &errtypes.ClusterError{Msg: "terraform init failed", Err: err})`
**Rule source:** Uber §Error Wrapping (preserve cause); repo-counter-example
`internal/distribution/okd/setup/haproxy.go:L155-L165`
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-state-and-recovery
**What MUST stay bit-for-bit:** hint message visibility (operator
actionable); `ClusterError` typing.
**Estimated net LOC delta:** +1
**Severity:** minor
**Severity reason:** observability degradation; hint already names the
right next action.
**Risk:** low
**Confidence:** high
**CLAUDE.md / MEMORY.md conflict?:** no

### err:5013fea6:auth-error-string-sniffing

**ID:** err:5013fea6:auth-error-string-sniffing
**Cluster:** string-sniffing
**File:** `internal/distribution/okd/setup/release_extract.go:92-141`
**Smell:** `isAuthError` substring-matches against an `authMarkers` list
(`unauthorized`, `authentication`, `denied`, `forbidden`, `no basic auth`,
`401`, `403`) on subprocess stderr to classify ClusterError vs AuthError.
The pattern is exactly what audit-errors string-sniffing rule catches.
Mitigated because exit code is the primary signal — string match is
secondary lift.
**Evidence:**
```go
var authMarkers = []string{"unauthorized", "authentication", "denied", "forbidden", "no basic auth", "401", "403"}
if (result.ExitCode == 1 || result.ExitCode == 125) && isAuthError(msg) {
    return &errtypes.AuthError{...}
}
```
**Fix — preferred:** track upstream `openshift/oc` for a typed
registry-auth error envelope; until then, accept the documented
best-effort. Optionally tighten markers to `401`/`403`/`unauthorized`
(HTTP-aligned, low FP rate).
**Rule source:** CLAUDE.md §architecture-notes; repo-self roadmap
err:5013fea6 (already tracked); Go proverb 'errors are values'
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** behavior on documented stderr substrings
(today's tests).
**Estimated net LOC delta:** -3
**Severity:** suggestion
**Severity reason:** acknowledged best-effort with roadmap entry.
**Risk:** low
**Confidence:** high
**CLAUDE.md / MEMORY.md conflict?:** no

### err:ddf885f4:install-all-bare-ctx-err

**ID:** err:ddf885f4:install-all-bare-ctx-err
**Cluster:** cancellation-identity
**File:** `internal/addon/manager.go:85-113`
**Smell:** Two ctx-err return paths exist: bare-return at L86 and joined
at L110-L113. The asymmetry is intentional but undocumented; a future
refactor that unifies to one shape would silently break the
partial-failure aggregate.
**Evidence:**
```go
L85:  if err := ctx.Err(); err != nil {
L86:      return err
L87:  }
...
L110: if ctxErr := ctx.Err(); ctxErr != nil && len(errs) > 0 {
L111:     errs = append(errs, ctxErr)
L112: }
L113: return errors.Join(errs...)
```
**Fix — preferred:** add a one-line WHY comment at L85 documenting why
the bare-ctx-return is load-bearing.
**Rule source:** CLAUDE.md §code-comments (Non-obvious WHY decisions);
repo-counter-example `internal/distribution/okd/install/monitor.go:L65-L67`
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** both ctx-err paths; the `errors.Join`.
**Estimated net LOC delta:** +1
**Severity:** suggestion
**Severity reason:** works today; documenting load-bearing shape.
**Risk:** low
**Confidence:** medium
**CLAUDE.md / MEMORY.md conflict?:** no

## Scaffolding items detected

### err:9f8e7d6c:errtypes-vocab-cert-pending

**Cluster:** domain-vocabulary
**File:** `internal/errtypes/errtypes.go`
**Why scaffolding:** symmetric-api candidate — adds a 6th typed error
(TransientError) that mirrors the existing 5 in shape, gated on a
concrete retry-aware caller appearing.
**Smell:** No `TransientError`/`RecoverableError` concept for
operator-degraded paths (kube-vip cert-pending, CSR-pending, oc
operator-still-settling). Today these are forced into `ClusterError`
→ exit 4, indistinguishable from 'cluster permanently degraded'.
**Suggested action:** verify intent against roadmap. If a retry-shell
wrapper or cron-driven verify run is on the near-term roadmap, add
`TransientError` with exit code 75 (EX_TEMPFAIL); otherwise leave the
vocabulary as-is.
**Severity:** suggestion (capped per §7).

## Linter-config-bug candidates

None this run. `errorlint` is enabled in `.golangci.yml` and is
catching the `%v` / `%s`-on-err class — every `fmt.Errorf` site this
audit examined uses `%w` correctly when wrapping. `nilerr` is enabled.
The audit-errors findings live in design space (which typed error,
where to wrap, identity preservation through subprocess cancellation),
which no linter can catch.

## Skip list

- **`err:26a430ee:sudo-missing-sentinel-not-wrapped`** (prior run):
  RESOLVED. `cli/elevation.go:99` now uses
  `errors.Join(err, errtypes.ErrSudoMissing)` so `errors.Is(err, ErrSudoMissing)`
  matches and exit code 71 is delivered. Removed from this run.
- `internal/tui/wizard/**` — out of scope per AUDIT_CONVENTIONS.md §2.
  Not flagging the wizard's `errors.New("key cannot be empty")` etc.
- `_test.go` files — out of scope.
- `validators.go` use of `fmt.Sprintf("invalid okd version: %v", err)`
  (line 319): the receiver is a `ValidationResult.AddError(field, message)`
  taking a string for human display — not a wrapped error chain. Pattern
  is correct for validation aggregation.

## Cluster verdicts

- **wrapping** — 290 `%w`-using sites; only 2 small smells (env-file
  double-context; state-lock hint dropping cause). `errors.Join` is
  used in 13 places, idiomatic. No double-`%w`-of-the-same-error
  patterns found.
- **string-sniffing** — exactly one site (`isAuthError` in
  release_extract.go); documented best-effort with a roadmap
  entry; exit-code primary signal. No string-sniffing on stdlib err
  text. Healthy.
- **sentinel-vs-typed** — vocabulary is consistent and broadly
  used (247 references). Two gaps: (a) cli RunE bare returns
  bypass the typed mapping (~15 sites, `err:fd2125dd`); (b) phase
  step bodies return naked `fmt.Errorf` (~50 sites,
  `err:c19ee328`). `runlock.go` is a small per-site instance of
  the same class.
- **redaction-in-error** — credential types
  (`ProxmoxCredentials`, `SecretBytes`) implement `Redacted()`
  correctly. `system.SubprocessError` redacts. `executor.ExitError`
  is the lone gap (`err:7b2829bb`) and it's the canonical type
  the rest of the codebase wraps around — a high-leverage fix.
- **domain-vocabulary** — 5 concepts cover the shipped use cases.
  Single forward-looking gap (`TransientError`) recorded as
  scaffolding-suggestion, not a current bug.
- **cancellation-identity** — `install/monitor.go` is the
  canonical reference; 2 sites diverge (`err:48688e63`,
  `err:fde34e0c`). `errors.Is(err, context.Canceled)` is used
  consistently — no `err == context.Canceled` direct equality.

## Scope exceptions proposed

None. The wizard exclusion (§2) is honoured; everything else
in scope was read in full.

## Footer

Total findings: 11 (blocker: 0, major: 3, minor: 5, suggestion: 3)
Major share: 27% (rubric §4 threshold: <40%)
Scope coverage: 179 / 179 in-scope `.go` files read directly or via
the patterns swept (`fmt.Errorf|errors.{New,Is,As,Join,Unwrap}` —
496 sites, every match line read).
Seam deferrals: 5 — `err:7b2829bb` (audit-observability),
`err:48688e63` (audit-concurrency), `err:fd2125dd` (audit-cli-ux),
`err:c19ee328` (audit-cli-ux), `err:fde34e0c` (audit-concurrency),
`err:62cb8a95` (audit-state-and-recovery), `err:9f8e7d6c`
(audit-cli-ux).

To refresh `.claude/audits/linter-config-bugs.jsonl`, run:
```
jq -c 'select(.adjacent_linter_enabled==true)' .claude/audits/*.jsonl > .claude/audits/linter-config-bugs.jsonl
```
or `/audit-all`.

# audit-observability — 2026-05-08

**Assumes green:** golangci-lint, govulncheck, CodeQL, shellcheck, tflint, go test ./...
**Scope:** every `slog.X` / `tui.X` / `*.Log.X` / `*.logger.X` call site in `internal/**/*.go` and `cmd/**/*.go`; the slog handler-setup chain (`internal/logutil/redact.go`, `internal/tui/logger.go`, `internal/cli/root.go`, `internal/cli/logging.go`).
**Out of scope this run:** `internal/tui/wizard/**`, `*_test.go` (per AUDIT_CONVENTIONS §2), templates.
**Seam co-owners:** audit-errors (own typed-error wrapping / Redacted() type-shape; this audit defers via `seam:audit-errors` on row obs:5013fea6 / obs:06f00bcb, cross-linked via `related: err:7b2829bb`); audit-cli-ux (UX channel for instructional output, obs:19a715fd); audit-tests (tests for the redact handler — already covered in `internal/logutil/redact_test.go`).

## Executive summary

Handler setup is healthy: `internal/logutil.RedactHandler` is installed via `tui.SimpleLogger()` and reaches `slog.SetDefault` in `cli/root.go:78`; TTY-vs-pipe switch lives in `cli/logging.go:73`; `--log-format=json` + `--quiet` are wired. The c07157e commit ("migrate fmt.Sprintf log messages to structured attrs") successfully removed every `tui.X(fmt.Sprintf(...))` and `*.Log.X(fmt.Sprintf(...))` direct invocation — grep returned zero hits for those patterns. Three migration misses remain: one `fmt.Sprintf` *attr value* (upload.go), and the `system.WaitFor` helper still pre-renders its own message (exec.go). The most material finding is **obs:5013fea6** — three sites pass raw subprocess `Stderr` under the benign attr key `"stderr"`, bypassing RedactHandler (which only matches password/token/secret/api_key/apikey fragments). This is the sink-side mirror of audit-errors `err:7b2829bb` (executor.ExitError lacks `Redacted()`); the seam rule (§3) puts the sink-bypass observation in observability. No blocker; one major; four minor; four suggestion.

## Ranked table

| ID | Cluster | File:line | Severity | Confidence | LOC | Adjacent linter | Fix class |
|----|---------|-----------|----------|------------|-----|-----------------|-----------|
| obs:5013fea6:stderr-attr-leaks-subprocess-output | redaction-sink | internal/distribution/okd/setup/release_extract.go:121-123 | major | medium | 3 | none | refactor |
| obs:97cb8adf:fmt-sprintf-message-pattern | redaction-sink | internal/system/exec.go:117-135 | minor | high | 3 | none | refactor |
| obs:eb479d86:sprintf-attr-value | field-stability | internal/distribution/okd/setup/upload.go:138 | minor | high | 1 | none | refactor |
| obs:beabab0c:phase-attr-missing-on-setup | field-stability | internal/distribution/okd/setup/phase.go:99-107 | minor | high | 2 | none | refactor |
| obs:48688e63:proxmox-probe-failure-as-info | level-discipline | internal/infrastructure/proxmox/proxmox.go:382-397 | suggestion | medium | 3 | none | refactor |
| obs:9d79b841:double-log-iso-download | span-retry-boundary | internal/distribution/okd/setup/coreos.go:277-278 | suggestion | high | 2 | none | refactor |
| obs:19a715fd:instructional-logs-via-info | level-discipline | internal/addon/catalog/secretstore/secretstore.go:122-152 | suggestion | high | 17 | none | refactor |
| obs:632c9087:rollback-pair-not-spanned | span-retry-boundary | internal/distribution/okd/postinstall/update_ingress.go:444-573 | suggestion | medium | 4 | none | refactor |
| obs:660d83a5:setrunid-not-symmetric-with-stderrslog | handler-setup | internal/tui/logger.go:174-180 | suggestion | medium | 7 | none | refactor |

Sort: severity_weight × confidence × |LOC delta| ÷ risk (blocker=4, major=3, minor=2, suggestion=1; high=3 / med=2 / low=1).

## Findings

### obs:5013fea6:stderr-attr-leaks-subprocess-output

**Cluster:** redaction-sink
**File + line range:** `internal/distribution/okd/setup/release_extract.go:121-123` (extras: `internal/distribution/okd/postinstall/update_ingress.go:569`, `internal/distribution/okd/setup/apache.go:111-113`)
**Smell:** Three log sites pass raw subprocess `Stderr` under the benign attr key `"stderr"`, which is not in `logutil.secretKeyFragments`. The release_extract case is the most exposed: `oc adm release extract` against a private registry can echo registry endpoint with userinfo, partial bearer tokens, or `pull-secret` diagnostic snippets into stderr. RedactHandler cannot rewrite a generic `stderr` key, and the value is a plain string — `Redacted()` is never consulted. Sink-side mirror of audit-errors `err:7b2829bb` (executor.ExitError lacking `Redacted()`).
**Evidence:**
```go
release_extract.go:L122-L123:
    msg := strings.TrimSpace(result.Stderr)
    p.Log.Error("tools: oc adm release extract failed", "ref", ref, "stderr", msg)
update_ingress.go:L569:
    p.Log.Warn("update-ingress: rollback create failed", "err", err, "stderr", result.Stderr)
apache.go:L112-L113:
    p.Log.Warn("apache: semanage port modify exited non-zero",
        "exit", r.ExitCode, "stderr", strings.TrimSpace(r.Stderr))
```
**Fix — preferred:** Stop logging raw subprocess stderr at the sink — let the typed error handle redaction; replace `"stderr", msg` with `"err", execErr` after err:7b2829bb adds `Redacted()` to `executor.ExitError`. If a structured stderr attr is genuinely useful, route it through a redactable wrapper type. Do **not** widen `secretKeyFragments` to include `stderr` — it would over-redact legitimate kubectl/terraform diagnostics.
**Rule source:** CLAUDE.md §credentials-and-secrets; seam-rule audit-errors vs audit-observability §3; repo counter-example `internal/system/exec.go:L40-L44 (SubprocessError.Redacted)`
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-errors
**Severity:** major
**Severity reason:** rubric §4/credential-exposure — registry stderr can carry userinfo or token fragments; RedactHandler does not match the "stderr" key.
**Risk:** low — option (a) reduces information; the typed error already wraps the same content.
**Confidence:** medium — credential exposure depends on subprocess stderr content, which is not deterministically auditable.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** `err:7b2829bb:exit-error-no-redact`

### obs:97cb8adf:fmt-sprintf-message-pattern

**Cluster:** redaction-sink
**File + line range:** `internal/system/exec.go:117-135`
**Smell:** `system.WaitFor` builds `waitMsg`/`readyMsg` via `fmt.Sprintf` and passes the rendered string as the slog message arg. CLAUDE.md §credentials forbids `X(fmt.Sprintf(...))` because RedactHandler can only inspect attr values, not pre-rendered text. The c07157e migration swept ~85 sites but missed this canonical helper.
**Evidence:**
```go
L117-L119:
    waitMsg := fmt.Sprintf("%s: waiting for %s...", prefix, description)
    readyMsg := fmt.Sprintf("%s: %s is ready", prefix, description)
    logger.Info(waitMsg)
```
**Fix — preferred:** static message + structured attrs: `logger.Info("waiting", "for", description, "prefix", prefix)` and `logger.Info("ready", "for", description, "prefix", prefix, "polls", polls, "elapsed", elapsed.Round(time.Second))`. Same shape for the L164 Debug. Net -2 LOC.
**Rule source:** CLAUDE.md §credentials-and-secrets; commit c07157e
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**Severity:** minor
**Severity reason:** present-day callers pass static prefix/description constants; the leak is theoretical until a future caller plumbs a credential-bearing string. Worth fixing because WaitFor is the canonical poll helper.
**Risk:** low
**Confidence:** high — pattern is verbatim what CLAUDE.md prohibits.
**CLAUDE.md / MEMORY.md conflict?:** no

### obs:eb479d86:sprintf-attr-value

**Cluster:** field-stability
**File + line range:** `internal/distribution/okd/setup/upload.go:138`
**Smell:** `fmt.Sprintf("%.1f", totalSizeMB)` pre-renders a float to a string before the slog handler sees it. JSON output emits `"size_mb": "123.4"` (string) instead of `"size_mb": 123.4` (number) — breaks downstream typed pipelines. Migration missed this attr-value form.
**Evidence:**
```go
L138: p.Log.Info("iso: uploading", "count", len(toUpload),
    "size_mb", fmt.Sprintf("%.1f", totalSizeMB),
    "user", user, "host", host, "path", remotePath)
```
**Fix — preferred:** Drop the Sprintf wrapper, pass the float directly: `"size_mb", totalSizeMB`. If precision-1 rendering is load-bearing for the text formatter, round before passing: `roundedMB := math.Round(totalSizeMB*10)/10`.
**Rule source:** CLAUDE.md §credentials-and-secrets (typed attrs); commit c07157e
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**Severity:** minor
**Severity reason:** field-stability regression — same key emits string-typed value vs float across the rest of the corpus.
**Risk:** low
**Confidence:** high
**CLAUDE.md / MEMORY.md conflict?:** no

### obs:beabab0c:phase-attr-missing-on-setup

**Cluster:** field-stability
**File + line range:** `internal/distribution/okd/setup/phase.go:99-107`
**Smell:** Setup `Phase.New` does not call `bp.Log = bp.Log.With("phase", "setup")` even though install/postinstall/destroy/cleanup all do. Operators filtering JSON logs by `phase=setup` get an empty result.
**Evidence:**
```go
setup/phase.go:L99-L107:
    func New(version string, opts ...phase.BasePhaseOption) *Phase {
        bp := phase.NewBasePhase(version, opts...)
        // <- missing: bp.Log = bp.Log.With("phase", "setup")
        detectedOS := platform.DetectOrDefault(bp.Log)
        return &Phase{ BasePhase: bp, ... }
    }
Counter-example install/phase.go:L91 — bp.Log = bp.Log.With("phase", "install")
```
**Fix — preferred:** Add `bp.Log = bp.Log.With("phase", "setup")` after `NewBasePhase`. Net +1 LOC. Pattern matches install/postinstall/destroy/cleanup.
**Rule source:** repo counter-examples — install/phase.go:L91, postinstall/phase.go:L68, destroy/phase.go:L62, cleanup/cleanup.go:L105
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**Severity:** minor
**Severity reason:** field-stability inconsistency across 4-of-5 sibling phase files.
**Risk:** low
**Confidence:** high
**CLAUDE.md / MEMORY.md conflict?:** no

### obs:48688e63:proxmox-probe-failure-as-info

**Cluster:** level-discipline
**File + line range:** `internal/infrastructure/proxmox/proxmox.go:382-397`
**Smell:** `probeVMEnumeration` logs three failure-or-fallback paths at Info. The first two are best-effort fallbacks (function intentionally returns `true` so callers do not suppress per-VM logs) — those belong at Debug. The third (vm not yet enumerable, retry) stays at Info.
**Evidence:**
```go
L381-L390:
    if err != nil {
        p.logger.Info("terraform: pvesh probe skipped", "err", err)
        return true
    }
    if err := json.Unmarshal([]byte(result.Stdout), &vms); err != nil {
        p.logger.Info("terraform: pvesh probe payload unparseable", "err", err)
        return true
    }
```
**Fix — preferred:** Drop both probe-fallback Info → Debug. L397 stays at Info. Net 0 LOC.
**Rule source:** repo counter-example `monitor.go:L155-L162`
**Severity:** suggestion
**Risk:** low
**Confidence:** medium

### obs:9d79b841:double-log-iso-download

**Cluster:** span-retry-boundary
**File + line range:** `internal/distribution/okd/setup/coreos.go:277-278`
**Smell:** Two consecutive Info records for one event. One logical span split across two lines.
**Evidence:**
```go
p.Log.Info("coreos: downloading iso", "version", info.Version)
p.Log.Info("coreos: iso url", "url", info.ISOUrl)
```
**Fix — preferred:** Fold to one record: `p.Log.Info("coreos: downloading iso", "version", info.Version, "url", info.ISOUrl)`. Net -1 LOC.
**Rule source:** repo counter-example `orchestrator.go:L149`
**Severity:** suggestion

### obs:19a715fd:instructional-logs-via-info

**Cluster:** level-discipline
**File + line range:** `internal/addon/catalog/secretstore/secretstore.go:122-152`
**Smell:** Three branches (1password, vault, bitwarden) emit setup instructions via `env.Logger.Info` as multi-line numbered procedures. Logs are an event stream, not a UX channel. Strings also concatenate path components into the message body.
**Fix — preferred:** Either (a) collapse to one Warn with `link` attr pointing at docs/addons/secretstore.md, or (b) write the procedure to `env.Out`/cobra stdout instead of the log channel.
**Severity:** suggestion
**Seam:** audit-cli-ux

### obs:632c9087:rollback-pair-not-spanned

**Cluster:** span-retry-boundary
**File + line range:** `internal/distribution/okd/postinstall/update_ingress.go:444-573`
**Smell:** `convertToLoadBalancer` rollback path lacks an explicit start record. A hung rollback shows nothing on the wire until cmd timeout.
**Fix — preferred:** Add `p.Log.Info("update-ingress: rollback: starting", "name", ic.Name)` at the top of `attemptRollback`. Net +2 LOC.
**Severity:** suggestion

### obs:660d83a5:setrunid-not-symmetric-with-stderrslog

**Cluster:** handler-setup
**File + line range:** `internal/tui/logger.go:174-180`
**Smell:** `SetRunID` rebinds stderrLogger and rebuilds stderrSlog, but `slog.SetDefault(tui.SimpleLogger())` runs at `cli/root.go:78` BEFORE `SetRunID` at L87. Any goroutine that captures `slog.Default()` before the SetRunID call permanently loses run_id. Today's only consumer (`version.BackgroundCheck`) spawns after SetRunID so it is fine; the risk surface is future package-init code or background goroutines.
**Fix — preferred:** Either (a) call `slog.SetDefault` inside `SetRunID` itself after the buildStderrSlog rebuild, or (b) reorder cli.Execute to call SetRunID before slog.SetDefault. Net +2 LOC.
**Severity:** suggestion

## Scaffolding items detected

None. Every flagged log site has live callers.

## Linter-config-bug candidates

None — no Go linter covers slog discipline. All findings are pure human-review.

## Skip list

- **executor.ExitError redaction (system-wide pattern).** Audit-errors emits this finding (`err:7b2829bb`) and labels its seam `audit-observability`. By the seam rule (§3), errors owns the type-shape; observability owns only the sink — captured in the cross-linked `obs:5013fea6` row above. Not double-counted.
- **`internal/cli/firewall/firewall.go` nil logger asymmetry** (prior `obs:25fa1be8`): no longer reachable — all four call sites now pass `p.Log`. Today's exposure is design-doc consistency, not a panic path. Removed from current run.
- **`obs:6424733c:fmt-sprintf-message-pattern` (prior umbrella).** Resolved by commit c07157e. The two surviving fmt.Sprintf-in-log sites (`obs:97cb8adf` for the message pattern in WaitFor, `obs:eb479d86` for the attr-value in upload.go) are emitted as narrow successors.
- **`obs:366b3f2d:step-key-id-vs-name-collision` (prior orchestrator finding).** Re-checked `internal/distribution/orchestrator.go` — every `"step"` attr now consistently passes `step.ID()`. The skipped-step branches (L90, L142) carry `step.ID()` and `step.Name()` separately. Resolved.

## Cluster verdicts

- **handler-setup**: healthy. RedactHandler installed at the right boundary, tied through SimpleLogger, default logger set early in `cli.Execute`. The only remaining wrinkle is the SetRunID ordering (obs:660d83a5).
- **field-stability**: 99% clean — keys are stable lowercase snake_case (err: 102 occurrences, file: 23, path: 36, count: 17, duration: 12). Two narrow misses: `phase` attr absent on setup phase (obs:beabab0c); `size_mb` typed string instead of float (obs:eb479d86). The prior `step` key collision (obs:366b3f2d) is fully resolved.
- **redaction-sink**: one structural gap remaining — the `stderr` attr key (obs:5013fea6) is the single most exposed surface today. The fmt.Sprintf message pattern in `system.WaitFor` (obs:97cb8adf) is currently theoretical but lives in a canonical helper.
- **level-discipline**: largely well-tuned. `monitor.go` log-once pattern (CSR loop) is the canonical reference and is preserved. Two narrow over-Info sites in proxmox probe (obs:48688e63) and secretstore instructions (obs:19a715fd).
- **log-once / span-retry-boundary**: orchestrator and monitor both implement clean pairs. Two minor misses: doubled coreos download record (obs:9d79b841), missing rollback span entry (obs:632c9087).

## Scope exceptions proposed

None requested. Wizard code (`internal/tui/wizard/**`) remains out of scope per AUDIT_CONVENTIONS §2.

## Footer

Total findings: 9 (blocker: 0, major: 1, minor: 3, suggestion: 5).
Scope coverage: 46 production .go files use a logging API; sweep covered every grep-match against `slog.X`, `tui.X`, `*.Log.X`, `*.logger.X`, `slog.X` direct (514 + 281 = ~795 raw match lines, deduplicated to ~340 distinct call sites). Read-in-full: `internal/logutil/logutil.go`, `internal/logutil/redact.go`, `internal/logutil/redact_test.go`, `internal/tui/logger.go`, `internal/cli/root.go`, `internal/cli/logging.go`, `internal/cli/helpers.go`, `cmd/okdctl/main.go`, `internal/distribution/orchestrator.go`, `internal/distribution/okd/install/monitor.go`, `internal/system/exec.go` (WaitFor), `internal/distribution/okd/postinstall/update_ingress.go`, `internal/distribution/okd/setup/upload.go`, `internal/distribution/okd/setup/coreos.go`, `internal/distribution/okd/setup/release_extract.go`, `internal/distribution/okd/setup/apache.go`, `internal/distribution/okd/setup/phase.go`, `internal/distribution/okd/install/phase.go`, `internal/distribution/okd/install/flux.go`, `internal/distribution/okd/destroy/helpers.go`, `internal/distribution/okd/cleanup/cleanup.go`, `internal/infrastructure/proxmox/proxmox.go`, `internal/infrastructure/terraform/terraform.go`, `internal/version/updatecheck.go`, `internal/runlock/runlock.go`, `internal/sshpin/sshpin.go`, `internal/executor/executor.go`, `internal/download/download.go`, `internal/addon/catalog/secretstore/secretstore.go`, `internal/addon/manager.go`, `internal/addon/helpers.go`. JSONL row count: 9 / 9 schema-validated. Seam deferrals: 2 (obs:5013fea6 / obs:06f00bcb fold into obs:5013fea6 cross-linked to err:7b2829bb).

To refresh `linter-config-bugs.jsonl`, run the aggregation command or `/audit-all`.

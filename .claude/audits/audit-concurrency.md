# audit-concurrency — 2026-05-08

**Assumes green:** golangci-lint (govet, staticcheck, ineffassign, unused, gosec G-set, errorlint, nilerr, noctx, errcheck), govulncheck, `go test -race`, CodeQL, shellcheck.
**Scope:** every goroutine launch, channel close, ticker, `time.Sleep`, ctx propagation site in `internal/**/*.go` and `cmd/**/*.go`.
**Out of scope this run:** `internal/tui/wizard/**` (per AUDIT_CONVENTIONS §2), `_test.go`, `//go:build linux`-tagged files (`internal/cli/doctor.go`, `debug_bundle_doctor.go`), `internal/distribution/okd/setup/iso.go`, templates.
**Seam co-owners:** audit-subprocess (cmd.Cancel/WaitDelay shape — see `con:ae5b624c` related to `sub:7b2829bb`).

## Executive summary

Concurrency posture in this repo is unusually disciplined for a CLI of this size. Every non-test `go`-spawn site has either a documented stop signal (spinner, monitor, signal-loop, metrics-server) or a documented bounded leak (`promptForConfirmation` stdin reader, `BackgroundCheck` HTTP fetcher) — both shapes CLAUDE.md §concurrency explicitly endorses. Channels are buffered cap=1 with single senders; tickers are `defer ticker.Stop()`'d at every site. The only `major` finding is `addon/helpers.go::RetryDefault`, which retries every error through the full backoff budget (~35s) including permanent failures — the `//nolint:nilerr` comment acknowledges the gap and `download/retry.go::isRetryable` already demonstrates the canonical fix in this repo. The remaining findings are `_ context.Context` parameter holes in `internal/distribution/okd/setup/{apache.go,ignition.go,tools.go}` — minor cancellation-responsiveness regressions inside file-I/O steps that have a 1-line ctx.Err() fix in the same file as a counter-example.

## Ranked table

| ID | Cluster | File:line | Severity | Confidence | LOC | Adjacent linter | Fix class |
|----|---------|-----------|----------|------------|-----|-----------------|-----------|
| con:0b188cab:retry-eats-non-retryable | time-sleep-retry | internal/addon/helpers.go:23-41 | major | high | 19 | none | refactor |
| con:06f00bcb:ctx-ignored-file-io | ctx-ignored | internal/distribution/okd/setup/apache.go:48-95 | minor | medium | 48 | none | refactor |
| con:ab9b764a:ctx-ignored-install-config | ctx-ignored | internal/distribution/okd/setup/ignition.go:39-106 | minor | medium | 68 | none | refactor |
| con:8ea706f6:ctx-ignored-install-binary | ctx-ignored | internal/distribution/okd/setup/tools.go:240-253 | minor | medium | 14 | none | refactor |
| con:97cb8adf:waitfor-check-no-ctx | ctx-ignored | internal/system/exec.go:107-167 | suggestion | medium | 61 | none | refactor |
| con:6424733c:metrics-stop-shutctx-background | ctx-todo | internal/cli/helpers.go:240-256 | suggestion | high | 17 | none | refactor |
| con:15ba17da:tracker-mu-not-needed-yet | waitgroup-vs-errgroup | internal/distribution/okd/destroy/steps.go:32-69 | suggestion | high | 38 | none | refactor |
| con:48688e63:disconnect-ctx-ignored | ctx-ignored | internal/infrastructure/proxmox/proxmox.go:123-129 | suggestion | high | 7 | none | refactor |
| con:ae5b624c:monitor-cmd-cancel-pattern | goroutine-lifetime | internal/distribution/okd/install/monitor.go:24-43 | suggestion | high | 20 | none | policy |
| con:39c75e91:confirm-stdin-leak-bounded | goroutine-lifetime | internal/cli/confirm.go:31-58 | suggestion | high | 28 | none | policy |
| con:181efc90:spinner-canonical | goroutine-lifetime | internal/tui/spinner.go:19-56 | suggestion | high | 38 | none | policy |
| con:aa84670c:signalloop-bounded-leak | goroutine-lifetime | internal/cli/root.go:99-152 | suggestion | high | 54 | none | policy |
| con:8e65d574:bgcheck-canonical-leak-bound | goroutine-lifetime | internal/version/updatecheck.go:36-58 | suggestion | high | 23 | none | policy |

## Goroutine inventory

Every non-test `\bgo func` / `\bgo [A-Z]` spawn in scope:

| Site | Owner of shutdown | Verdict |
|------|-------------------|---------|
| `internal/tui/spinner.go:28` | dual: `stopCh` (sync.OnceFunc-closed) + `ctx.Done()`; caller blocks on `done` channel | clean — canonical pattern |
| `internal/distribution/okd/install/monitor.go:38` | `cmd.Cancel` + `cmd.WaitDelay` (Go 1.20 stdlib); buffered `doneCh` cap=1, single sender, `defer close` | clean — canonical pattern |
| `internal/cli/confirm.go:42` | uncancellable stdin read — documented bounded leak (process lifetime); `inputCh` cap=1 so eventual send never blocks | clean — documented leak bound |
| `internal/cli/helpers.go:238` | `srv.Shutdown` from stop closure; `errCh` cap=1, single send; `BaseContext` propagates parent ctx to in-flight scrapes | clean |
| `internal/version/updatecheck.go:53` | `ctx` propagates through `fetchLatest → http.NewRequestWithContext`; `ch` cap=1, single send; leak bound = `httpTimeout` (4s) | clean — canonical pattern |
| `internal/cli/root.go:113` (`signalLoop`) | `signal.Stop(sigCh) + close(sigCh)` in `execute`'s defer; receiver observes `!ok` and returns | clean — canonical pattern |

No `go-unowned`, `go-leak-on-error`, or `go-no-wait` findings.

## Channel-close inventory

| Site | Closer | Verdict |
|------|--------|---------|
| `tui/spinner.go:26` `close(stopCh)` | wrapped in `sync.OnceFunc` so double-close panic is impossible; closed by caller (sender side) | clean |
| `tui/spinner.go:29` `close(done)` | `defer` inside the goroutine; closed by sender | clean |
| `install/monitor.go:39` `close(doneCh)` | `defer` inside the goroutine after sending `cmd.Wait()` result; sender closes | clean |
| `cli/root.go:106` `close(sigCh)` | closed in `execute`'s defer after `signal.Stop`; sender (the `signal` package) is detached first so no send-on-closed possible | clean |

No `close-by-receiver`, `nil-chan-close`, or `send-on-closed` findings.

## Ticker / timer / sleep inventory

| Site | Stop discipline |
|------|-----------------|
| `tui/spinner.go:30` `time.NewTicker(120ms)` | `defer ticker.Stop()` |
| `install/monitor.go:116` `time.NewTicker(opts.CSRApprovalInterval)` | `defer ticker.Stop()` |
| `cli/root.go:159` `time.NewTimer(100ms)` | `defer t.Stop()` |
| `system/exec.go:121` `time.NewTicker(opts.Interval)` | `defer ticker.Stop()` |
| `system/exec.go:126` `time.NewTimer(opts.Timeout)` | `defer timer.Stop()` (only when `opts.Timeout > 0`) |

No `time.Sleep` calls in retry loops anywhere in production code (CLAUDE.md §concurrency policy honoured). No `time.After` in long-lived selects (no `time-after-leak` findings).

## Findings

### con:0b188cab:retry-eats-non-retryable

**Cluster:** time-sleep-retry
**File:** internal/addon/helpers.go:23-41
**LOC touched:** 19
**Smell:** RetryDefault retries every `fn() error` through the full backoff budget (3 steps, 5s base, factor 2, jitter 0.5, 5min cap). The `//nolint:nilerr` suppresses errcheck but documents the bug: a permanent failure (auth denied, oc binary missing, malformed addon manifest) consumes the full ~35s of backoff before surfacing. `EnsureNamespace` is one caller — a missing `oc` binary should fail fast, not after 3 retries.
**Evidence:**
```go
return wait.ExponentialBackoffWithContext(ctx, wait.Backoff{...}, func(_ context.Context) (bool, error) {
    // Returning (false, nil) on error asks wait to retry; returning
    // the error would abort the retry loop. Non-retryable errors
    // aren't distinguished today — all fn failures are retried
    // through the full backoff budget...
    if err := fn(); err != nil {
        return false, nil //nolint:nilerr // intentional: retry on any error
    }
    return true, nil
})
```
**Fix — preferred:** Mirror `download/retry.go::isRetryable`: add an Addon-level `isRetryable(error)` check (oc-binary-missing, ctx-canceled, `ConfigError`, `AuthError` → fail-fast). Return `(false, fnErr)` for non-retryable so wait aborts immediately; return `(false, nil)` only for transient (5xx, connection refused, transient `executor.ExitError`).
**Rule source:** CLAUDE.md §concurrency (no `time.Sleep` in retry loops; retry honours cancellation); repo-counter-example: `internal/download/retry.go:L58-L78` (isRetryable taxonomy); Uber §Goroutine Lifetime.
**Adjacent linter:** none
**Severity:** major (rubric §4/un-idiomatic-pattern-bitten-repo — `download/retry.go::isRetryable` already corrected this anti-pattern; `RetryDefault` re-introduces it on the addon path)
**Risk (of applying fix):** low — isolated helper, all callers go through it.
**Confidence:** high — author's `//nolint` comment names the gap.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

---

### con:06f00bcb:ctx-ignored-file-io

**Cluster:** ctx-ignored
**File:** internal/distribution/okd/setup/apache.go:48-95 (also L28-L46)
**LOC touched:** 48
**Smell:** `configureApachePort(_ context.Context, bindIP string)` accepts ctx but never reads it. The body does substantial file I/O (`system.CopyFile`, `os.ReadFile`, `bufio.Scanner` of 1 MiB buffer, `AtomicWrite`) that could hold the deploy ctx past a SIGINT. `ensureIgnitionDir` at L28 has the same shape. `ConfigureApache` (the caller) propagates a real ctx; dropping it inside the body breaks cancellation responsiveness for the whole apache-config step.
**Evidence:**
```go
func (p *Phase) configureApachePort(_ context.Context, bindIP string) {
    httpdConf := p.OS.ApacheConfigPath()
    if !system.FileExists(httpdConf) { return }
    // ... os.ReadFile + bufio.Scanner + AtomicWrite, ctx unused
}
```
**Fix — preferred:** Either rename the param to a real `ctx context.Context` and check `if err := ctx.Err(); err != nil { return }` at the top of each branch, or drop the ctx parameter from the signature entirely. The `_ context.Context` placeholder is the worst of both worlds.
**Rule source:** CLAUDE.md §concurrency; Go proverb: Don't ignore the ctx; Uber §Context; repo-counter-example: `internal/distribution/okd/setup/ignition.go:L130` (ctx.Err() check inside file-I/O loop).
**Adjacent linter:** none
**Severity:** minor
**Risk:** low — pure additive ctx.Err() gate.
**Confidence:** medium — file I/O is fast in practice; cancellation responsiveness is the cost.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** con:ab9b764a, con:8ea706f6 (same pattern in same package)

---

### con:ab9b764a:ctx-ignored-install-config

**Cluster:** ctx-ignored
**File:** internal/distribution/okd/setup/ignition.go:39-106 (also L157-L177)
**LOC touched:** 68
**Smell:** `GenerateInstallConfig(_ context.Context, ...)` reads a pull-secret file, validates JSON, and renders+writes `install-config.yaml` without ever consulting ctx. `InjectCompactClusterManifests` at L157 has the same shape. Both run inside `StepDef.Exec` closures whose enclosing orchestrator threads a real ctx (`orchestrator.go:L84` `o.executeStep(ctx, step)`). The cancellation gate must be inside the function to bound a SIGINT mid-deploy — `ValidateIgnitionFiles` (L196, also in this file) demonstrates the correct shape with `ctx.Err()` at the top.
**Evidence:**
```go
func (p *Phase) GenerateInstallConfig(_ context.Context, cfg *config.Config, outputDir string) error {
    if err := system.EnsureDir(outputDir); err != nil { ... }
    pullSecret, err := os.ReadFile(cfg.Files.PullSecret)
    // ... no ctx.Err() gate; ctx parameter dropped
```
**Fix — preferred:** Add `if err := ctx.Err(); err != nil { return err }` at the top of each function's body, mirroring `ValidateIgnitionFiles` at L197 in the same file.
**Rule source:** CLAUDE.md §concurrency; repo-counter-example: `internal/distribution/okd/setup/ignition.go:L197-L205`.
**Adjacent linter:** none
**Severity:** minor
**Risk:** low
**Confidence:** medium
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** con:06f00bcb, con:8ea706f6

---

### con:8ea706f6:ctx-ignored-install-binary

**Cluster:** ctx-ignored
**File:** internal/distribution/okd/setup/tools.go:240-253
**LOC touched:** 14
**Smell:** `installBinaryToPath(_ context.Context, srcPath, name string)` accepts ctx but ignores it. The body calls `system.CopyFile` (privileged write to `/usr/local/bin` under sudo re-exec) plus `system.MakeExecutable` (chmod +x). Inside a long-running tools-install step, dropping ctx means a SIGINT after the privileged copy starts has no chance to abort before the next binary lands.
**Evidence:**
```go
func (p *Phase) installBinaryToPath(_ context.Context, srcPath, name string) error {
    binDir := phase.BinDirOrDefault(p.BinDir)
    destPath := filepath.Join(binDir, name)
    if err := system.CopyFile(srcPath, destPath); err != nil { ... }
    if err := system.MakeExecutable(destPath); err != nil { ... }
    return nil
}
```
**Fix — preferred:** Rename `_ context.Context` to `ctx context.Context` and add `if err := ctx.Err(); err != nil { return err }` at the top.
**Rule source:** CLAUDE.md §concurrency; Uber §Context.
**Adjacent linter:** none
**Severity:** minor
**Risk:** low
**Confidence:** medium
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** con:06f00bcb, con:ab9b764a

---

### con:97cb8adf:waitfor-check-no-ctx

**Cluster:** ctx-ignored
**File:** internal/system/exec.go:107-167
**LOC touched:** 61
**Smell:** `WaitFor`'s `check func() bool` callback takes no ctx, so polling sites cannot propagate cancellation into their probe. Today every caller closes over an outer ctx (e.g. `OcPollOutputInterval` at `phase/kubectl.go:L63` captures ctx in the closure for `p.Exec.Run(ctx,...)`), but the contract leaves the door open: a future caller that does CPU work or blocking syscalls in `check` has no way to honour ctx mid-probe.
**Evidence:**
```go
for {
    select {
    case <-ctx.Done():
        return fmt.Errorf("waiting for %s %s: %w", prefix, description, ctx.Err())
    case <-ticker.C:
        if check() {  // ← check takes no ctx
            return nil
        }
```
**Fix — preferred:** Change check signature to `check func(context.Context) bool`. Pass the outer ctx to each invocation. Today's only caller already captures ctx, so the rename is a 5-LOC patch with no behavioural change.
**Rule source:** CLAUDE.md §concurrency (canonical patterns: signal-watched ctx); repo-counter-example: `internal/distribution/okd/install/monitor.go:L116-L184`.
**Adjacent linter:** none
**Severity:** suggestion
**Risk:** medium — touches the canonical `BasePhase.OcPollOutput` chain CLAUDE.md §architecture-notes pins as canonical; refactor must preserve the test-injection seam at `OcPollOutputInterval`.
**Confidence:** medium
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

## Scaffolding items detected

### con:48688e63:disconnect-ctx-ignored

`Provider.Disconnect(_ context.Context)` accepts ctx for symmetry with future network-bound providers (documented at L123-L125). The doc-comment makes the symmetry-with-Connect intent explicit. Symmetric with `Connect` at L103 which takes ctx and uses it for SSH host-key verification. `scaffolding=true scaffolding_reason=symmetric-api`. **Fix:** none — keep as recorded scaffolding so the next sweep doesn't re-flag.

### con:15ba17da:tracker-mu-not-needed-yet

`destroyTracker.mu` sync.RWMutex guards two `[]string` slices. `Orchestrator.Run` executes steps serially today — there is no concurrent caller of `onError` or `skipWhen`. Forward-looking, consistent with `internal/distribution/context.go:L13`'s documented forward-looking RWMutex; unlike `context.go` this site has no comment naming the future parallel-step mode. `scaffolding=true scaffolding_reason=symmetric-api`. **Fix:** add a clarifying comment matching `context.go:L11-L21`'s pattern, or leave as-is.

## Linter-config-bug candidates

None this run. Every finding has `adjacent_linter: none` — no Go static analyser today catches "ctx parameter is silently dropped via `_ context.Context`" or "retry loop swallows non-retryable errors via `nilerr`-style return". `noctx` covers `http.NewRequest` without ctx but not these patterns.

## Skip list

The following appeared like findings but are CLAUDE.md-conformant deliberate choices (recorded as `suggestion` rows so the next sweep doesn't re-flag):

- `internal/cli/helpers.go:240-256` startMetricsServer.stop uses `context.Background()` — documented per CLAUDE.md §concurrency (`con:6424733c`).
- `internal/distribution/okd/install/monitor.go:24-43` canonical cmd.Cancel + WaitDelay pattern — preserve bit-for-bit (`con:ae5b624c`).
- `internal/cli/confirm.go:31-58` documented bounded-leak stdin goroutine — preserve (`con:39c75e91`).
- `internal/tui/spinner.go:19-56` canonical ticker-backed background worker — preserve (`con:181efc90`).
- `internal/cli/root.go:99-152` canonical signal-watched-ctx pattern — preserve (`con:aa84670c`).
- `internal/version/updatecheck.go:36-58` canonical fire-and-forget with documented leak bound — preserve (`con:8e65d574`).
- `internal/distribution/context.go:11-43` `PhaseContext[T]` RWMutex with explicit forward-looking comment naming roadmap items — CLAUDE.md MEMORY.md §scaffolding allows.
- `internal/addon/registry.go:18-22` Registry RWMutex protects map+order slice — correct, init() registration is serialized by Go runtime, lookups are concurrent.
- `internal/deploymetrics/metrics.go:20-26` Recorder mu sync.Mutex — correct (HTTP scrape goroutine vs orchestrator step goroutine).
- `internal/credentials/envfile.go:24-27` `loadOnce sync.Once` for `LoadEnvFile` — correct, env mutation footgun explicitly guarded.

## Cluster verdicts

- **ctx-ignored:** 5 findings, three of them in `setup/{apache.go,ignition.go,tools.go}` are real minor cancellation regressions; `WaitFor.check` is a contract-shape suggestion; `Provider.Disconnect` is documented scaffolding. No `ctx-stored` (struct fields). Cluster verdict: clean except for the three setup-package sites.
- **ctx-todo:** 1 finding — `startMetricsServer.stop` uses `context.Background()` with a justification comment (CLAUDE.md §concurrency requirement satisfied). Cluster verdict: clean.
- **goroutine-lifetime:** 5 findings, all preservation rows for canonical patterns. Every non-test go-spawn has a documented stop signal or leak bound. Cluster verdict: clean.
- **channel-close:** 0 findings. Every `close()` site is in the goroutine inventory above; all closed by sender, all `sync.OnceFunc`-guarded against double-close where applicable.
- **time-sleep-retry:** 1 finding — `RetryDefault` retries non-retryable errors. No `time.Sleep` retry loops anywhere; all retry uses `wait.ExponentialBackoffWithContext` (ctx-aware). All tickers `defer ticker.Stop()`'d; no `time.After` in long-lived selects.
- **waitgroup-vs-errgroup:** 1 finding — `destroyTracker.mu` (forward-looking scaffolding). No `sync.WaitGroup` anywhere in scope; the codebase has standardized on serial step execution + buffered channel + ctx-cancel patterns. No mutex-protected single-bool/int (no `mutex-should-be-atomic`). No `sync.Map` (no `syncmap-misuse`). Cluster verdict: clean.

## Scope exceptions proposed

None.

## Footer

Total findings: 13 (blocker: 0, major: 1, minor: 3, suggestion: 9).
Scope coverage: every in-scope file containing `\bgo func`, `sync.`, `time.NewTicker`, `time.NewTimer`, `time.Sleep`, `time.After`, `_ context.Context`, `context.Background`, `context.TODO`, or `wait.ExponentialBackoff` was read in full (~25 files). No sub-agent dispatch this run — corpus was small enough to read directly.
Seam deferrals: 1 — `con:ae5b624c` cross-references `sub:7b2829bb:no-cancel-func` (audit-subprocess owns the per-site finding for `executor.RunInteractive`; this audit owns the canonical-pattern-preservation row at `monitor.go`).

To refresh `linter-config-bugs.jsonl`, run the aggregation command or `/audit-all`. (None expected from this run — all findings are `adjacent_linter: none`.)

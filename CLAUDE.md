# Repo conventions for AI contributors

This file is the durable instruction set for Claude, GPT-based tools, and any
other agent that edits this repository. The rules here are authoritative —
they override defaults baked into the assistant. If a section is missing,
fall back to standard Go community conventions (gofmt, effective-go, the
golangci-lint config in `.golangci.yml`).

## Commit messages

- **Format**: `type(scope): description` (conventional commits)
- **Style**: lowercase, imperative, short (under 70 chars for the subject line)
- **Types in use**: `refactor`, `fix`, `feat`, `chore`, `ci`, `docs`, `test`
- **No trailers.** Do not add `Co-Authored-By: Claude <...>` or similar. Do not
  reference AI, LLMs, or any tool branding in commit bodies.
- **Body** (optional): describe the *why* and any non-obvious trade-offs, not
  the what (the diff already shows that).

See `git log --oneline` for existing examples.

## Code comments

This repo targets ~3% comment density. Go's naming conventions and the type
system already carry most of the signal; comments should add what the code
can't say for itself.

### Write a comment when (and only when)

1. **Package doc**. Every package has a `// Package X ...` block above the
   package clause, one to three sentences.
2. **Exported API with non-obvious behavior**: ordering constraints, caller
   contracts, failure modes, or mutation semantics that aren't evident from
   the signature.
3. **Non-obvious WHY decisions**: a workaround, a security-relevant choice, a
   performance decision that beats the "obvious" version, a ctx/cancellation
   subtlety, a lint suppression.
4. **Workarounds with context**: link the upstream bug or issue when you're
   working around it. Example: `// charmbracelet/bubbles#812 — TextStyle
   breaks width calc`.
5. **Subtle invariants and footguns**: "caller must hold mu", "not safe after
   Close()", "never log raw password — scrub via X".

### Do NOT write a comment when

1. **The name already says it.** Bad:
   ```go
   Key string      // unique key
   Label string    // display label
   Required bool   // whether the field is required
   ```
   These add zero signal. Delete.

2. **The function doc just echoes the signature.** Bad:
   ```go
   // GetFoo returns foo.
   func (x *X) GetFoo() Foo { return x.foo }
   ```
   Either add semantic info or delete the comment.

3. **You're narrating your own refactor.** Bad:
   ```go
   // Extracted from X because the body was 40 lines.
   // Split into 4 sub-methods to stay under funlen.
   ```
   The commit message already carries this. Delete.

4. **Self-referential peacock talk.** Bad:
   ```go
   // This is the canonical "does X exist?" check.
   // Use this function for ...
   // This helper function ...
   ```
   Replace with a plain one-line description or delete.

5. **Section dividers.** Bad:
   ```go
   // --- Auth ---
   // === helpers ===
   ```
   Use file splits or function names. Delete dividers.

6. **Inline comments that narrate the next line.** Bad:
   ```go
   // Create the step builder.
   b := distribution.NewStepBuilder(id, name)
   ```
   If the code isn't self-evident, rename variables.

7. **TODO / FIXME without context.** If you must leave one, name the person
   and link an issue: `// TODO(@handle): see owner/repo#123`. Bare `// TODO`
   is rot.

### Comment format

- Use `//` line comments, not `/* ... */` blocks, except for build tags or
  embed directives.
- Start doc comments with the identifier name: `// Foo does X.`
- Keep doc comments to 1–3 sentences unless there's a real invariant to
  explain. If you're writing a paragraph, ask whether the function is too
  complex or the doc is over-explaining.
- Wrap lines at ~80 columns for readability, ~100 max.
- No trailing periods on field-level inline comments (`Foo int // count`), do
  use them in full-sentence doc comments.

### When in doubt

Delete the comment. If the code breaks later, the commit history and the
type system will catch it. If a reviewer asks "why?", that's the signal to
write the comment — then it carries real information.

## Architecture notes

- `internal/distribution/okd/phase/` holds shared helpers used by all OKD
  phases (`setup`, `install`, `postinstall`, `destroy`, `cleanup`). New
  cross-phase helpers belong here, not in a specific phase package.
- `internal/distribution.StepDef` + `BuildSteps` is the canonical way to
  declare phase steps. Don't introduce new per-step builder functions;
  add entries to the phase's existing `xSteps()` method.
- `internal/logutil.NopLogger` is the canonical no-op logger. Don't
  write `slog.New(slog.DiscardHandler)` inline.
- `internal/system.WriteTempFile` handles temp-file-with-callback.
  Don't hand-roll `os.CreateTemp` + chmod + defer cleanup.
- `BasePhase.OcResourceExists` and `BasePhase.OcPollOutput` are the canonical
  kubectl/oc helpers for phase code. Extend `phase/kubectl.go` rather than
  writing local wrappers. Internally every `BasePhase.Oc*` method delegates
  to `cluster.Client` (via `cluster.WithExecutor`, sharing the phase's own
  executor) for the actual invocation and transport-error formatting —
  `internal/cluster` is the low-level oc/kubectl shell-out layer for phase
  code and non-phase CLI callers (`cli/status.go`,
  `internal/distribution/okd/install/monitor.go`). Known exceptions:
  stdin-fed `oc create -f -` in `postinstall/update_ingress.go` (apply +
  rollback; Client has no stdin primitive), the streamed
  `oc adm release extract` in `setup/release_extract.go` (Client has no
  streaming primitive), and `oc adm must-gather` in `cli/debug_bundle.go`
  (runs the PATH-resolved oc through a purpose-built one-off executor;
  deliberate non-migration). The addon layer also still shells out via its
  own `Environment.Exec` — migrating any of these is a separate decision.
  Add new oc primitives to `cluster.Client`, not a second copy in phase
  code.
- `addon.BuildOpaqueSecret` is the canonical k8s Opaque Secret manifest
  builder for addons.
- SSH shell policy: new SSH operations MUST use `SSHRunArgv` (argv-mode).
  `SSHRun` (sh -c with a string argument) is used only in
  `phase/iso_cleanup.go::RemoveFCOSISOFromProxmox` — see that function for
  the required `validateXxx` + `shellSingleQuote` layering pattern.
- Flag naming convention: `--output`/`-o` selects the output format (values:
  `text`, `json`), mirroring kubectl/oc convention. `--output-file` (no
  shorthand) writes data to a file path. Never register `-o` as a shorthand
  for a file-destination flag; reserve it exclusively for format selection.
  Shorthand allowlist: only `--yes`/`-y`, `--quiet`/`-q`, `--verbose`/`-v`,
  `--output`/`-o`, and `--config`/`-c` carry single-letter shorthands.
  Per-command boolean tail flags (`--keep-haproxy`, `--keep-isos`,
  `--skip-terraform`, `--skip-must-gather`, `--dry-run`, etc.) are
  intentionally long-form only; do not add a shorthand to new ones.
- `internal/runlock` serialises concurrent okdctl invocations via flock, which
  is advisory on NFSv3 and bypassed entirely across hosts; never rely on it as
  a cross-host correctness guarantee. Terraform's own state lock is the
  authoritative guard: every state-locking subcommand (plan, apply, destroy)
  passes `-lock-timeout=120s` so a stale lock from a SIGKILL-ed prior run
  waits and fails with a clean diagnostic rather than failing immediately.

## Tooling

- `go.mod` targets Go 1.25 / toolchain 1.26.2. Don't downgrade.
- `.golangci.yml` is authoritative for lint config. Key thresholds:
  `funlen.lines: 120`, `gocyclo.min-complexity: 30`, `dupl.threshold: 100`.
- CI runs `lint-go`, `test-go`, `build-go`, `security` (govulncheck),
  `lint-yaml`, `validate-terraform`. All must be green before merging to
  `develop`.
- Coverage floors live in `.github/coverage-floors.conf` (key=value,
  one package per line, `*` is the default). Raise a package's floor
  when its tests land; the `test-go` CI job will fail on regression.
- Pre-commit hooks run via `lefthook`.

## Concurrency

- **Every goroutine needs a stop signal or a documented leak bound.**
  Fire-and-forget is acceptable only when (a) the work is bounded by
  `ctx` and (b) the call site documents the leak bound. See
  `internal/version/updatecheck.go` (100ms wait at caller) for the
  canonical example.
- **Prefer `errgroup` over `sync.WaitGroup`** when errors must bubble up.
  `sync.WaitGroup` + a shared error variable is a footgun.
- **No `time.Sleep` in retry loops.** Use `select { case <-ctx.Done(): ... case <-time.After(d): ... }`
  or `wait.ExponentialBackoffWithContext`. A bare `time.Sleep` ignores
  cancellation and stretches shutdown time unbounded.
- **`context.Background()` / `context.TODO()` in production code needs
  a justification comment** — almost every call site should be
  receiving a ctx from its caller. Root-level exceptions live in
  `internal/cli/root.go::execute()` (signal-watched context creation)
  and test-only helpers.
- **Canonical patterns:**
  - graceful subprocess cancel: `internal/distribution/okd/install/monitor.go`
    (`cmd.Cancel` SIGTERM + `cmd.WaitDelay` 30 s, Go 1.20+).
  - Ticker-backed background worker: `internal/tui/spinner.go`.
  - Signal-watched cancellation: `internal/cli/root.go::execute()` —
    `defer close(sigCh)` after `signal.Stop` so the receiver returns
    cleanly on the happy path.
  - Poll-loop log-once: `internal/distribution/okd/install/monitor.go`
    (CSR approval loop, L122–L161) — first occurrence of an error
    string logs at Warn and sets `lastCSRWarnMsg`; identical repeats
    demote to Debug; a clean tick resets the gate. Follow this shape
    in every new poll/retry loop.

## Credentials and secrets

- Never commit secrets. `.env` files live alongside `configs/*.yaml` and are
  gitignored.
- Password fields in `credentials.ProxmoxCredentials` use `[]byte` so callers
  can `Zeroize` them after use.
- Error messages must not leak raw credentials. See `InputField.Validate`
  for the password scrubbing pattern.
- slog records pass through `internal/logutil.RedactHandler` (installed by
  `tui.SimpleLogger`). Attrs whose keys contain password/token/secret/api_key
  are rewritten to `[redacted]`; `*url.URL` userinfo is stripped; types that
  implement `Redacted() any` control their own output. Prefer structured
  attrs (`logger.Warn("…", "err", err)`) over `fmt.Sprintf(…%v…)` so the
  handler can inspect values.
- **Never** call `tui.Info(fmt.Sprintf(...))`, `tui.Warn(fmt.Sprintf(...))`,
  `p.Log.Info(fmt.Sprintf(...))`, or any `X(fmt.Sprintf(...))` log form.
  `RedactHandler` scrubs attr keys/values; it cannot inspect a pre-rendered
  string, so a future field that interpolates a credential into the message
  silently leaks. Use the structured form: `tui.Info("using credentials",
  tui.LF("source", creds.Source))` or `p.Log.Info("…", "key", val)`.
- **ZeroizeEnv defer pattern.** Any type that stores credential-bearing env
  entries (`[]string` of `KEY=value` pairs) must expose a `ZeroizeEnv()` method
  that blanks secret-keyed entries via `logutil.KeyIsSecret`, then `clear()`s
  and nils the slice. Call it with `defer x.ZeroizeEnv()` immediately after the
  object is constructed and before any subprocess operation, bounding the
  plaintext lifetime to the enclosing function scope. This includes
  short-lived locals: a `terraform.Executor` built with
  `terraform.WithEnv(creds env)` must also receive `defer tf.ZeroizeEnv()`
  immediately after construction so error paths and early returns do not
  leave credential strings reachable until GC.
  **Reviewer checklist:** any PR that adds a new `creds.Env()` call site
  must also add that site to the known-call-sites registry in the
  `ProxmoxCredentials.Env()` doc comment (`internal/credentials/proxmox.go`)
  and verify `defer ZeroizeEnv()` is present on the object constructed with
  that env slice.

## Dependencies

- **License policy.** Every direct and transitive dep must ship under a
  permissive license (MIT / Apache-2.0 / BSD-{2,3}). No GPL / AGPL / LGPL
  / custom / missing licenses — the shipped static binary plus apt/rpm/brew
  packaging would be blocked. New deps touch `go.mod` ⇒ check the upstream
  LICENSE.
- **v0.x deps need a justification and an abandonment plan.** v0.x APIs
  may break on any minor bump. Today's entries:
  - `github.com/luthermonson/go-proxmox` v0.5.x — sole Proxmox discovery
    path (`internal/tui/wizard/steps/proxmox_discovery.go`). Bus-factor 1.
    Fallback: ~200 LOC REST-only rewrite using `net/http` + the documented
    Proxmox API. Track upstream releases; bump on each.
  - `registry.terraform.io/bpg/proxmox` ~> 0.109.0 — sole actively
    maintained Proxmox VE Terraform provider; hash-pinned at 0.109.0 in
    `infrastructure/terraform/environments/production/.terraform.lock.hcl`
    (linux_amd64 + linux_arm64 hashes committed). Fallback: migrate to
    `Telmate/proxmox` or replace with direct REST calls via
    `null_resource`. Track upstream releases; bump on each.
- **Maintained but upstream-locked deps.** `gorilla/websocket` is pulled
  transitively via `go-proxmox`. **okdctl does not reach it** — the wizard
  uses REST discovery only, not shell/console websockets. Safe to keep
  until go-proxmox migrates to `coder/websocket`, at which point take the
  bump without local code changes.
- **`github.com/joho/godotenv` ships its license file as `LICENCE`
  (British spelling) — a valid MIT license; SBOM scanners that grep for
  `LICENSE` will flag a false positive.**
- **Removed transitive-weight deps.** `schollz/progressbar/v3` was dropped
  in favour of a ~30 LOC hand-rolled byte-progress writer in
  `internal/download/progress.go`. The writer reuses
  `tui.ProgressBarsEnabled()` and `golang.org/x/term` for TTY gating.
  Re-introducing a third-party progressbar needs a second call site or a
  concrete feature (cross-file aggregate, ETA, coloured segments) that
  the hand-roll can't serve.
- **Pin stability.** GitHub Actions must be SHA-pinned with a version
  trailer (`uses: owner/action@<40-hex-sha> # vX.Y.Z`). Tool installs
  from Go must be explicit versions — never `@latest`. Terraform versions
  in CI are patch-pinned (`terraform_version: "1.10.3"`, not `"1.10"`).
- **YAML engine baseline (tripwire).** Two engines ship in the
  `cmd/okdctl` production binary: `sigs.k8s.io/yaml` v1.6.0 (direct,
  required by k8s.io/api) and `go.yaml.in/yaml/v2` v2.4.3 (indirect,
  pinned by k8s.io/apimachinery). `go.yaml.in/yaml/v3` v3.0.4 is
  indirect in go.mod (also pinned by k8s.io/apimachinery) but only
  reachable under `-tags docs` via `cmd/okdctl-gen-docs`.
  `gopkg.in/yaml.v3` v3.0.1 is go.sum-only (absent from go.mod),
  pulled transitively. Do not add a fifth engine without a recorded
  justification here.
- **Before adding a dep,** check whether Go 1.25 stdlib covers it
  (`slices`, `maps`, `net/netip`, `log/slog`, `sync.OnceFunc`, etc.).

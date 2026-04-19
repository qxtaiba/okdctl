# Tier B — decisions gate (2026-04-18)

Drafted by the fix-through session; **awaiting sign-off before execution**.
Each block is one decision. Say "approve all", "approve X,Y,Z", or annotate
rejections. Anything not approved stays on this page; anything approved
executes in the next session.

Execution shape: once approved, Tier B items fan out via `/roadmap-pickup`
in isolated worktrees — each decision becomes one roadmap item with its
finding ID in `related:`.

---

## B1 · Rename `--output` on releases to disambiguate (breaking)

- **Finding:** `ux:e7db1220:flag-name-collision-output`
- **Problem:** `--output` means "file path" on `deploy` / `kubeconfig` but
  "format (text|json)" on `releases list|show`. Same flag name, different
  semantics — muscle memory lies.
- **Proposed fix:** Pick one; rename the other.
  - **Option A:** rename `releases` flag to `--format` / `-F`. Keeps
    `deploy`/`kubeconfig` muscle memory for file-path output.
  - **Option B:** adopt kubectl convention (`-o` = format); rename
    `deploy`/`kubeconfig`'s `--output` to `--output-file` / `-O`.
- **Blast radius:** pre-1.0; breaks any user scripts invoking `okdctl
  releases list --output=json`. Mitigate with `MarkHidden` alias for one
  release.
- **Recommendation:** Option A. Single command changes, kubectl convention
  is respected elsewhere in the tree, the rename is shorter.
- **Est effort:** hours.

## B2 · Drop `--non-interactive` alias on deploy

- **Finding:** `ux:073d24ed:deploy-yes-aliases`
- **Problem:** `deploy` exposes both `--non-interactive` and `--yes`; help
  text admits one is an alias. `cleanup` / `update-ingress` use only
  `--yes`.
- **Proposed fix:** drop `--non-interactive`, keep `--yes/-y` as primary.
- **Blast radius:** removes a CLI flag. Mitigate with `MarkHidden` alias
  for one release, then delete.
- **Recommendation:** approve.
- **Est effort:** hours.

## B3 · Add `version` subcommand (additive)

- **Finding:** `ux:aa84670c:version-subcommand-missing` + README drift
- **Problem:** Only `--version` flag exists. README documented `okdctl
  version` (I stripped that in Task #8, but the subcommand is standard —
  kubectl/docker/gh all expose both).
- **Proposed fix:** add `versionCmd` RunE that prints the same template
  as `--version`. Additive.
- **Blast radius:** none — new subcommand. `--version` unchanged.
- **Recommendation:** approve.
- **Est effort:** hours.

## B4 · Split SIGINT (130) vs SIGTERM (143) exit codes

- **Finding:** `ux:0f076161:signal-sigterm-collapsed`
- **Problem:** `signal.NotifyContext` watches both; both collapse to 130.
  POSIX: 128+N where SIGINT=2 (130) and SIGTERM=15 (143). Scripts can't
  distinguish operator Ctrl-C from orchestrator kill.
- **Proposed fix:** dedicated `chan os.Signal` separate from the
  NotifyContext; return 130 on Interrupt, 143 on SIGTERM. Update
  `root.go:81-96`.
- **Blast radius:** any script that currently treats 130 as "we gave up
  for any signal reason". Survey: CI, cron? Unknown.
- **Recommendation:** approve — conforms to POSIX, breaking no
  documented contract.
- **Est effort:** hours.

## B5 · Set `rootCmd.SilenceUsage = true`

- **Finding:** `ux:aa84670c:silence-usage-missing`
- **Problem:** cobra prints 20-line usage block ahead of any RunE error;
  the error message gets buried. kubectl/docker/gh all set this.
- **Proposed fix:** `rootCmd.SilenceUsage = true; rootCmd.SilenceErrors
  = true` in `cli/root.go init()`. `tui.Error` already renders errors.
- **Blast radius:** none — cobra still prints usage on arg-parse errors.
- **Recommendation:** approve.
- **Est effort:** minutes.

## B6 · Add `--quiet/-q` and `--verbose/-v` short flags

- **Finding:** `ux:aa84670c:quiet-verbose-missing`
- **Problem:** Only `--log-level=level` exists. `-v`/`-q` are muscle
  memory for kubectl/docker/git.
- **Proposed fix:** persistent flags on `rootCmd` aliased to
  `--log-level=debug` / `--log-level=error`; keep `--log-level` explicit.
- **Blast radius:** none — additive.
- **Recommendation:** approve.
- **Est effort:** hours.

---

## B7 · Introduce `executor.ExitError` type

- **Finding:** `err:7b2829bb:err-stringified-loses-type`
- **Problem:** `executor.RunChecked` / `k8s.run` / `k8s_csrs.PendingCSRs`
  strip `*exec.ExitError` via bare `fmt.Errorf`. Callers can't
  `errors.As` for exit code; ctx sentinels don't propagate.
- **Proposed fix:** mirror `terraform.ExecError` at
  `internal/infrastructure/terraform/terraform.go:22-33`. Add
  `executor.ExitError{Command, ExitCode, Stderr}` implementing `Unwrap`
  + `Error`. Rewrite the three call sites.
- **Blast radius:** error-type change; callers already stringify via
  `.Error()`, no behavior shift. New `errors.As` path unlocks cleaner
  retry logic downstream (addon manager, update-ingress rollback).
- **Recommendation:** approve.
- **Est effort:** hours (new type + 3 migration sites).

## B8 · Audit 5 typed-err fallthrough sites

- **Finding:** `err:48688e63:typed-err-fallthrough`, `err:d5915b0c`,
  `err:98723e5d`, `err:1e8ffb91`, `err:6b533f2d` (5 sites)
- **Problem:** `proxmox.Connect`, `install.Phase.SetupKubeconfig`,
  `install.flux`, `postinstall.verify`, `cluster.k8s_csrs` return bare
  `fmt.Errorf` where the documented exit-code contract wants a typed
  error (exit 2/3/4/5 per err type). Fallthrough to exit 1.
- **Proposed fix:** wrap with `&errtypes.ConfigError{Msg, Err}` /
  `ClusterError` / `NetworkError` as appropriate. 5 small edits.
- **Blast radius:** observable exit code shift on those specific
  failure paths. Scripts relying on "exit 1 means it failed" still
  work; scripts that branch on `==2 for config error` will now fire
  correctly.
- **Recommendation:** approve.
- **Est effort:** hours.

---

## B9 · Fix orchestrator fatal-step early-return on destroy

- **Finding:** `state:366b3f2d:fatal-step-blocks-cleanup`
- **Problem:** `StepDestroyInfra` is fatal; orchestrator short-circuits
  on fatal failure. A failed `terraform destroy` aborts the pipeline
  before `remove-remote-iso`, `cleanup-files`, `cleanup-firewall`,
  `print-summary` run — half-destroyed cluster. Log text "file cleanup
  will be skipped unless --force is used" is misleading because
  `Options.Force` is always true and the orchestrator ignores it.
- **Proposed fix:** Either (a) mark `StepDestroyInfra` NonFatal when
  `Options.Force=true` and surface TF failure through phase context,
  or (b) add a continue-on-fatal interface. Strip the misleading
  `--force`-related log text.
- **Blast radius:** changes destroy semantics — partial-failure runs
  ALL steps now instead of stopping at the first fatal. Verify against
  `destroy/steps.go:37-43` comment intent.
- **Recommendation:** approve option (a); simpler path.
- **Est effort:** days.

## B10 · Remove `destroyWithPlan` silent fallback to `destroyDirect`

- **Finding:** `state:4c092fce:destroy-plan-fallback-unsafe`
- **Problem:** On `terraform plan -destroy` failure, `destroyWithPlan`
  silently degrades to `destroyDirect` (auto-approved `terraform
  destroy`), subverting the documented "safer plan-then-apply approach"
  comment.
- **Proposed fix:** return the plan error; operator re-runs with explicit
  `--force` or `--dry-run` as a diagnostic path.
- **Blast radius:** destroy fails harder where it used to silently
  proceed. That's the point — plan failures usually signal real problems.
- **Recommendation:** approve.
- **Est effort:** hours.

## B11 · Add `--dry-run` flag on destroy

- **Finding:** `state:0f076161:no-dry-run-flag` + `ux:0f076161`
- **Problem:** destroy has only y/N prompt; no `--dry-run` / `--plan`
  that would show the terraform destroy plan without executing.
- **Proposed fix:** thread a `--dry-run` mode that skips re-exec-as-root,
  prints the TF plan, exits 0 (or 2 per U4 exit-code dispatch).
- **Blast radius:** new flag, additive.
- **Recommendation:** approve.
- **Est effort:** days.

## B12 · Add `schemaVersion` to `okdctl.yaml` + switch to `UnmarshalStrict`

- **Finding:** `state:a55b4592:no-schema-version`,
  `state:cf43073b:unknown-yaml-fields-silent`
- **Problem:** `okdctl.yaml` has no version marker; loader uses
  non-strict `yaml.Unmarshal` — unknown fields silently accepted, typos
  silently default. Operator upgrading okdctl and running destroy on a
  stale config gets surprising behavior with no diagnostic.
- **Proposed fix:** add `schemaVersion: "v1"` top-level field; loader
  errors on missing/unknown version; switch to `yaml.UnmarshalStrict`;
  introduce a migration registry for future schema bumps.
- **Blast radius:** existing configs without `schemaVersion` break on
  load. Mitigate with a one-release grace period (default to "v1" if
  absent, warn loudly).
- **Recommendation:** approve.
- **Est effort:** days.

---

## B13 · Install cosign signature verification in `install.sh`

- **Finding:** `iac:e076e43c:sh-no-signature-verify` (only major from
  audit-iac-and-shell)
- **Problem:** `install.sh` fetches SHA256SUMS over HTTPS and checks
  archive integrity, but never verifies cosign's signature on
  SHA256SUMS itself — despite goreleaser publishing
  `SHA256SUMS.sig` / `SHA256SUMS.pem` on every release. An attacker who
  compromises release-asset upload (or MITMs with a compromised CA) can
  swap both.
- **Proposed fix:** if `cosign` on PATH, fetch `SHA256SUMS.sig` +
  `SHA256SUMS.pem`, run `cosign verify-blob --certificate=...
  --signature=... SHA256SUMS` before trusting. When absent, warn loudly.
  README's release-verification section already documents the manual
  version — match.
- **Blast radius:** install.sh adds a conditional dep (`cosign`). Wrong
  flag values could fail-open. Test on Linux+arm64+amd64.
- **Recommendation:** approve.
- **Est effort:** days (including test matrix).

## B14 · Plan HTTP → HTTPS for ignition pullSecret delivery

- **Finding:** `sec:00000001:http-ignition-pullsecret`
- **Problem:** OKD pullSecret is served in cleartext over HTTP on
  machineCIDR during install (`setup/phase.go:44-52`). Any co-tenant on
  the bastion network can sniff it.
- **Proposed fix:** spec a self-signed TLS option for the bastion
  httpd; document trust-store propagation to FCOS ignition path.
- **Blast radius:** large — changes bootstrap networking. Requires
  design doc + testing against all three FCOS release channels.
- **Recommendation:** **defer** to a roadmap design-doc item; too large
  for a bundled Tier B PR. Filed as candidate for a separate sprint.
- **Est effort:** weeks.

## B15 · Loud stderr warning when `INSECURE=1` is set

- **Finding:** `iac:e076e43c:sh-insecure-bypass`
- **Problem:** `INSECURE=1` silently skips SHA256 verification; only the
  header comment mentions this. Operators who set it via docs
  copy-paste miss the cost.
- **Proposed fix:** at the skipped block entry, emit
  `red "WARNING: INSECURE=1 set — SHA256 verification SKIPPED."` to
  stderr; optionally require a longer sentinel
  (`INSECURE_ACKNOWLEDGE_I_KNOW_WHAT_I_AM_DOING=1`).
- **Blast radius:** none — additive diagnostic.
- **Recommendation:** approve (short sentinel version; long one is
  clever-ceremony).
- **Est effort:** minutes.

---

## B16 · Install slog redacting handler middleware

- **Finding:** `sec:00000003:no-redact-handler`,
  `obs:bbc23e42:no-redact-handler-installed`,
  `err:a4001485:err-fmt-v-on-inner` (cross-skill anchor)
- **Problem:** No handler middleware inspects attrs to redact; a future
  caller that logs a `ProxmoxCredentials` leaks. Task #9's structured-
  error sweep unblocked this — errors now land as `"err", err` not
  embedded in message strings, so a handler CAN inspect them.
- **Proposed fix:** add `internal/logutil/redact` — a wrapping
  `slog.Handler` that walks attrs and rewrites values of types
  `ProxmoxCredentials`, `[]byte` keyed `password|token|secret|api`,
  `*url.URL` with userinfo. Install by default in
  `tui.SimpleLogger`; provide `Disable()` for test harnesses.
- **Blast radius:** all log output goes through the new middleware.
  Negligible overhead; must preserve existing JSON output shape.
- **Recommendation:** approve. This is the load-bearing
  observability fix the audit has been pointing at.
- **Est effort:** days.

---

## B17 · Split `addon.NewManager` / `phase.NewBasePhase` to variadic options

- **Finding:** `api:c287d5c0:opt-inconsistent-manager`,
  `api:ddf885f4:opt-inconsistent-addon-manager`
- **Problem:** Positional-arg constructors break every caller on each
  added field. Sibling packages (okd/executor/proxmox/terraform/cluster)
  use variadic `...Option`.
- **Proposed fix:** migrate `addon.NewManager` and `phase.NewBasePhase`
  to variadic options; add `WithExecutor`, `WithLogger`, `WithProjectRoot`.
- **Blast radius:** every caller of these two constructors (~6 total).
  Mechanical.
- **Recommendation:** approve.
- **Est effort:** hours.

## B18 · Delete duplicated OKD resource minimums

- **Finding:** `api:c287d5c0:dup-min-constants`
- **Problem:** `okd.go` and `config/validation_types.go` each define
  `MinControlPlaneMemoryMB`/`CPUs`/`DiskGB`. They currently agree
  (8192/4/50); `cfg.Validate()` during load makes `Provisioner.Validate`
  redundant.
- **Proposed fix:** delete the `okd`-side consts + the 3 `Validate`
  resource checks; rely on `config.ValidateOKDConfig`.
- **Blast radius:** internal; the 8192/4/50 values stay enforced at
  config-load time.
- **Recommendation:** approve.
- **Est effort:** hours.

---

## Not in Tier B (scoped out of this session's plan)

- **`ux:660d83a5:stderr-stdout-mixed`** (56 sites redirecting
  `tui.Info`/`tui.Warn` from stdout to stderr) — larger than a sign-off
  bundle; needs its own plan + per-site verification for scripted
  callers. File as separate roadmap item.
- **Cleanup of 21 remaining multi-arg `fmt.Sprintf` log sites** — the
  Task #9 sweep covered single-arg `%v`-err patterns. Multi-arg (e.g.
  `"foo %s: %v", name, err`) need per-site judgment on which attrs get
  names.
- **Scoped destroy flag (`--scope=infra|files|firewall`)**
  (`state:15ba17da:no-scoped-destroy`) — UX design decision;
  deferrable.
- **Doc additions for the ~50 `revive.exported` sites outside the audit
  scope** — out-of-scope per the audit's canonical-API focus.

---

## Sign-off

Copy this line, strike through rejections, send back:

```
B1:A · B2:Y · B3:Y · B4:Y · B5:Y · B6:Y · B7:Y · B8:Y · B9:Y · B10:Y · B11:Y · B12:Y · B13:Y · B14:defer · B15:Y · B16:Y · B17:Y · B18:Y
```

Once confirmed, the next session runs `/roadmap-pickup` and dispatches
each approved item into a worktree.

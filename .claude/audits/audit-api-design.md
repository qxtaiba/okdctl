# audit-api-design — 2026-05-08

**Assumes green:** golangci-lint (deadcode, unused, unparam, revive),
govulncheck, CodeQL, shellcheck, tflint, `go test ./...`.
**Scope:** all `internal/**` Go packages, `cmd/**`, package-level shape
(option-pattern consistency across siblings, ctx-first discipline,
import graph, exported-surface minimalism, zero-value usability,
sibling symmetry).
**Out of scope this run:** `internal/tui/wizard/**`, templates,
`//go:build linux` files, `*_test.go`, generated docs.
**Seam co-owners:** `audit-security` (sec:7b2829bb:executor-no-zeroize,
sec:35abd54e:env-string-residue — co-own the credential-lifecycle
asymmetry; api-design owns the API shape, security owns the impact).
`audit-code-smells` (would own per-body smells; this audit emits only
package-level pairwise findings).

## Executive summary

The okdctl exec-wrapper stack has a sibling-symmetry break that lights
up the rest of the audit: terraform.Executor and okd.Provisioner both
ship `ZeroizeEnv` for credential teardown, but executor.Executor — the
canonical exec wrapper named in CLAUDE.md §architecture-notes — does
not. The two consumers hand-roll the same body, which is why
audit-security flagged the gap independently. Because the field
mutations live on `Executor.Env` (a public field), the asymmetry is
load-bearing, not cosmetic. A second, smaller cluster of
option-pattern drift covers `firewall` (positional logger), `cleanup`
(double Option type), `terraform.Executor` (mixed public-fields plus
functional options), and the `cluster.K8sClient` package-stutter. Two
scaffolding rows (proxmox.Provider.Disconnect ctx, phase BinDir
trio) are left alone per MEMORY.md §scaffolding — they are documented
as symmetric-API placeholders. No blocker, one major, six minor, five
suggestions.

## Ranked table

Sort key: `severity_weight × confidence × |LOC delta| ÷ risk`
(blocker=4, major=3, minor=2, suggestion=1; high=3 / med=2 / low=1).

| ID | Cluster | File:line | Severity | Confidence | LOC | Adjacent linter | Fix class |
|----|---------|-----------|----------|------------|-----|-----------------|-----------|
| api:7b2829bb:zeroize-asymmetry | exported-surface | internal/executor/executor.go:38-92 | major | high | -16 | dupl (enabled) | refactor |
| api:7b2829bb:exposed-fields-no-callers | exported-surface | internal/executor/executor.go:38-46 | minor | high | +4 | unused (enabled) | refactor |
| api:25fa1be8:positional-logger | option-consistency | internal/distribution/okd/firewall/firewall.go:83-246 | minor | high | +15 | none | refactor |
| api:262af6e4:dual-option-types | option-consistency | internal/distribution/okd/cleanup/cleanup.go:85-118 | minor | high | -12 | none | refactor |
| api:4c092fce:terraform-mixed-shape | option-consistency | internal/infrastructure/terraform/terraform.go:32-59 | minor | high | -2 | unused | refactor |
| api:d5915b0c:exec-env-direct-mutation | package-boundary | internal/distribution/okd/install/phase.go:166 | minor | high | +2 | none | refactor |
| api:c287d5c0:zeroize-no-callers-yet | exported-surface | internal/distribution/okd/okd.go:198-216 | suggestion | high | -15 | dupl (enabled) | refactor |
| api:fde34e0c:k8sclient-pkg-stutter | exported-surface | internal/cluster/k8s.go:20-88 | suggestion | high | 0 | revive (enabled) | refactor |
| api:e2343d2c:unused-trailing-param | exported-surface | internal/system/systemd.go:31-46 | suggestion | high | -1 | revive (enabled) | refactor |
| api:ddf885f4:nil-logger-not-normalized | zero-value-usability | internal/addon/manager.go:34-57 | suggestion | high | 0 | none | refactor |
| api:48688e63:ctx-symmetry-no-network | ctx-first | internal/infrastructure/proxmox/proxmox.go:103-129 | suggestion | high | 0 | none | policy |
| api:0139cb3f:bin-dir-fan-out | exported-surface | internal/distribution/okd/phase/paths.go:52-97 | suggestion | medium | 0 | none | policy |

## Findings

### api:7b2829bb:zeroize-asymmetry — major

**Cluster:** exported-surface
**File + line range:** `internal/executor/executor.go:L38-L92`
(extras: `internal/infrastructure/terraform/terraform.go:L346-L364`,
`internal/distribution/okd/okd.go:L198-L216`)
**Smell:** terraform.Executor.ZeroizeEnv and okd.Provisioner.ZeroizeEnv
both clear cred-bearing entries from their inner executor.Env; the
canonical executor.Executor itself has no ZeroizeEnv method. Two
hand-rolled byte-identical bodies; the producer is the right home.
**Evidence:** terraform.go:L352 / okd.go:L204 are textually equivalent
loops over `t.exec.Env` blanking PROXMOX_VE_PASSWORD /
PROXMOX_VE_API_TOKEN entries, then `clear()` and nil. executor.go
ships no parallel.
**Fix:** add `(*Executor).ZeroizeEnv()` in internal/executor; collapse
the two consumer bodies to single-line forwards. Net LOC: -16.
**Rule source:** CLAUDE.md §architecture-notes (executor IS the
canonical exec wrapper); repo-counter-example
terraform.go:L346-L364; Go proverb "accept interfaces, return structs".
**Adjacent linter:** dupl (enabled — caught the duplication class but
the bodies are 16 lines, just under the threshold of 200).
**Severity reason:** rubric §4/canonical-helper-on-critical-path —
Executor IS the canonical exec wrapper; Env carries plaintext password
on every deploy/destroy.
**Seam:** audit-security (sec:7b2829bb:executor-no-zeroize owns the
impact; api-design owns the API shape and the symmetric-API
argument).
**Must preserve:** overwrite-then-clear ordering — Go strings are
immutable; collapsing to bare `clear()` would leave the string headers
untouched.

---

### api:7b2829bb:exposed-fields-no-callers — minor

**Cluster:** exported-surface
**File + line range:** `internal/executor/executor.go:L38-L46`
**Smell:** Executor.Stdout, Stderr, WorkDir, Verbose are exported but
no external caller mutates or reads them. WithWorkDir already exists;
the others should follow. The Executor committed to functional
options (WithLogger / WithEnv / WithInheritedEnv / WithWorkDir).
Verbose is dead — no internal reader.
**Evidence:** grep confirms zero out-of-package writes to these
fields; only RunStreamed reads e.Stdout/e.Stderr internally.
**Fix:** unexport stdout/stderr/workDir; add WithStdout / WithStderr;
delete Verbose. RunStreamed reads e.stdout/e.stderr internally.
**Rule source:** Go proverb "accept interfaces, return structs"; Uber
§Functional Options; repo-counter-example
internal/executor/executor.go:L44 (inheritEnv already unexported).
**Adjacent linter:** unused (enabled — but unused only flags
unreferenced symbols; cross-package mutability concerns are not its
domain). **linter-config-bug candidate**: the field IS read inside the
package (e.g. RunStreamed), so unused is silent.
**Seam:** none.

---

### api:25fa1be8:positional-logger — minor

**Cluster:** option-consistency
**File + line range:** `internal/distribution/okd/firewall/firewall.go:L83-L246`
**Smell:** every public function in the firewall package
(Configure, RemoveRules, ConfigureOKD, RemoveOKDRules, DetectBackend)
takes `*slog.Logger` as the trailing positional parameter. Sibling
phase-helper packages (phase, addon, cluster, executor, terraform,
proxmox) all take logger via a constructor option.
**Evidence:** firewall.go:L113, L170, L239 all have the same shape;
each function nil-guards `logger = logutil.OrNop(logger)` inline. Four
call sites in destroy/, postinstall/, cleanup/, setup/ thread the
logger through.
**Fix:** add `firewall.New(opts ...Option) *Firewall` with
WithLogger; convert the five public functions to methods. Net LOC:
+15 (struct + option scaffolding).
**Rule source:** Uber §Functional Options; repo-counter-example
internal/addon/manager.go (NewManager+ManagerOption).
**Adjacent linter:** none.
**Seam:** none.
**Must preserve:** firewalld>ufw>iptables backend precedence;
validatePort allowlist (security guard).

---

### api:262af6e4:dual-option-types — minor

**Cluster:** option-consistency
**File + line range:** `internal/distribution/okd/cleanup/cleanup.go:L85-L118`
**Smell:** cleanup.Phase has TWO option-pattern surfaces — the
canonical phase.BasePhaseOption (used by New) and a package-local
cleanup.Option / cleanupConfig (used by Execute). The Execute-time
Option only carries a logger which the BasePhase already has, so the
second surface is pure noise. No sibling phase package
(setup/install/postinstall/destroy) has this duplicate.
**Evidence:** cleanup.go:L89-L95 declares cleanupConfig+Option+WithLogger;
Execute (L113) takes both `*Options` and `...Option`.
**Fix:** drop cleanup.Option / cleanup.WithLogger / cleanup.cleanupConfig;
collapse Execute to `(*Phase).Execute(ctx, *Options) error`; have body
use p.Log (set by phase.WithLogger at construction). One caller
(okd.go:L131) updates from `cleanup.New(...).Execute(ctx, opts,
cleanup.WithLogger(p.logger))` to passing the logger via cleanup.New.
Net LOC: -12.
**Rule source:** repo-counter-example setup/phase.go,
install/phase.go (single Option type from phase.BasePhaseOption);
CLAUDE.md §architecture-notes.
**Adjacent linter:** none.
**Seam:** none.

---

### api:4c092fce:terraform-mixed-shape — minor

**Cluster:** option-consistency
**File + line range:** `internal/infrastructure/terraform/terraform.go:L32-L59`
**Smell:** terraform.Executor commits to functional options yet
exposes WorkDir, VarFile, Verbose as public fields. proxmox.go reads
`p.terraformExec.WorkDir` (proxmox.go:L192) — the cross-package read
encodes that the field IS part of the API. VarFile/Verbose are also
configurable via With* options, duplicating intent. logger is
unexported; the rest are exported. Pick one consistent pattern.
**Evidence:** struct definition at terraform.go:L32-L39 mixes public
and private; terraform.go:L52 sets e.Verbose, terraform.go:L133/369
read t.exec.Verbose, but no other external caller reads Verbose.
**Fix:** unexport WorkDir/VarFile/Verbose; add `(*Executor).WorkDir()
string` getter for proxmox.go:L192. Drop redundant public Verbose;
keep WithVerbose. Aligns with sibling executor.Executor (also flagged
at api:7b2829bb).
**Rule source:** Uber §Functional Options; Google Go Style Guide
§Packages; repo-counter-example internal/infrastructure/proxmox/proxmox.go
(all-unexported).
**Adjacent linter:** unused.
**Seam:** none.
**Must preserve:** PlanFileName const stays exported (used in
proxmox.go:L179 via terraform.PlanFileName).

---

### api:d5915b0c:exec-env-direct-mutation — minor

**Cluster:** package-boundary
**File + line range:** `internal/distribution/okd/install/phase.go:L166`
**Smell:** SetupKubeconfig appends to p.Exec.Env directly via
public-field mutation, bypassing executor.WithEnv. Sole post-construction
Env mutation in production code. Defeats any future Executor invariant
on Env (length cap, allowlist filter, redaction-on-insert).
**Evidence:** `p.Exec.Env = append(p.Exec.Env, "KUBECONFIG="+kubeconfigPath)`
at line 166. Every other call site uses WithEnv at construction.
**Fix:** add `(*Executor).AppendEnv(kvs ...string)` (or SetEnvVar);
have SetupKubeconfig call it; unexport Executor.Env. The two
remaining external readers (terraform.WithEnv, proxmox.WithEnv,
both at construction time) become callers of e.Exec.SnapshotEnv() or
similar getter. Pairs with api:7b2829bb (ZeroizeEnv) — same refactor
fixes both.
**Rule source:** Uber §Functional Options; CLAUDE.md §architecture-notes.
**Adjacent linter:** none.
**Seam:** none.

---

(Suggestions follow in compact form.)

### api:c287d5c0:zeroize-no-callers-yet — suggestion (scaffolding)

**File:** `internal/distribution/okd/okd.go:L198-L216`. Provisioner.ZeroizeEnv
mirrors terraform.Executor.ZeroizeEnv with byte-identical body. Once
api:7b2829bb lands, collapse to one-line forward `p.executor.ZeroizeEnv()`.
Keep the wrapper exported — cli/helpers.go calls it via the
Provisioner facade. **Scaffolding (symmetric-api).**

### api:fde34e0c:k8sclient-pkg-stutter — suggestion

**File:** `internal/cluster/k8s.go:L20-L88`. cluster.K8sClient stutters
(Google Go Style Guide §Packages). Rename to cluster.Client / cluster.New.
Five call sites + one test file. revive's var-naming would catch this
in a fresh repo; suppression is implicit. Compatible with sibling
naming (executor.Executor / terraform.Executor / proxmox.Provider).
**linter-config-bug candidate**: revive (enabled) but suppressed
because the existing typename predates the rule landing.

### api:e2343d2c:unused-trailing-param — suggestion

**File:** `internal/system/systemd.go:L31-L46`. ManageService takes
4-arg with the 4th named `_` — never read. Drop or document.

### api:ddf885f4:nil-logger-not-normalized — suggestion

**File:** `internal/addon/manager.go:L34-L57`. addon.WithLogger does
not call logutil.OrNop inside the option function — relies on
construction-end normalization in NewManager. cluster.WithLogger and
terraform.WithLogger DO call OrNop inside; phase.WithLogger does not
(matches addon's pattern). Pick one across siblings; today the split
is 50/50.

### api:48688e63:ctx-symmetry-no-network — suggestion (scaffolding)

**File:** `internal/infrastructure/proxmox/proxmox.go:L103-L129`.
Provider.Connect/Disconnect take ctx but the local-only impl uses ctx
only inside Connect (sshpin.Verify). Documented as
"symmetric with future network-bound providers." Verify intent —
roadmap.md should drive whether the symmetric API stays.
**Scaffolding (symmetric-api).**

### api:0139cb3f:bin-dir-fan-out — suggestion (scaffolding)

**File:** `internal/distribution/okd/phase/paths.go:L52-L97`.
phase.{ResolveBinDir, PreflightBinDir, BinDirOrDefault} forms a
three-function surface where each consults a different input source.
The doc-comment on BinDirOrDefault names this as scaffolding /
defense-in-depth. Verify intent annually; collapse if call-counts
drop. **Scaffolding (symmetric-api).**

## Scaffolding items detected

Per MEMORY.md §scaffolding, the following exports are shaped like
future APIs and stay regardless of caller count. Every row is capped
at severity=suggestion and tagged `scaffolding: true`.

- **api:48688e63:ctx-symmetry-no-network** — proxmox.Provider.Disconnect
  takes ctx for symmetry with a future network-bound provider lifecycle.
  Action: roadmap-search; keep if the multi-provider track is live.
- **api:c287d5c0:zeroize-no-callers-yet** — Provisioner.ZeroizeEnv is
  the symmetric Provisioner-facade wrapper for the
  terraform.Executor.ZeroizeEnv. Will reduce to a one-line forward
  once api:7b2829bb lands.
- **api:0139cb3f:bin-dir-fan-out** — three resolve functions form a
  symmetric surface (config / env / default fallback). Defense-in-depth
  per the existing doc-comment. Annual verification.

## Linter-config-bug candidates

Findings whose adjacent linter IS enabled in `.golangci.yml` but
silently passes through:

- **api:7b2829bb:zeroize-asymmetry** — `dupl` is enabled at threshold
  200; the duplicated bodies are ~16 lines each, below the threshold.
  Lowering the threshold to 50 across the repo would catch this and
  api:c287d5c0 in one config edit.
- **api:7b2829bb:exposed-fields-no-callers** — `unused` is enabled but
  cannot detect "exported field with no external mutator" because the
  package-internal RunStreamed reads e.Stdout/e.Stderr.
- **api:e2343d2c:unused-trailing-param** — `revive` is enabled with
  unused-parameter rule, but the param name `_` suppresses it.
- **api:fde34e0c:k8sclient-pkg-stutter** — `revive` var-naming is
  enabled but does not catch package-stutter on existing typenames
  (only on new exports). Fixing requires either renaming or a manual
  policy review.

To refresh `linter-config-bugs.jsonl`, run the aggregation command or
`/audit-all`.

## Skip list

- All canonical APIs from CLAUDE.md §architecture-notes are preserved:
  StepDef + BuildSteps (orchestrator path), NopLogger (logutil),
  WriteTempFile (system), OcResourceExists / OcPollOutput (BasePhase),
  BuildOpaqueSecret (addon), SSHRunArgv (phase). No finding proposes
  removing or bypassing any of these.
- distribution.MetricsRecorder is producer-side (defined in the package
  that emits to it via Orchestrator); deploymetrics.Recorder is the
  consumer-implementer. Producer-side interface is the correct location
  here because Orchestrator owns the dispatch — not flagged.
- Phase constructors all take `phase.BasePhaseOption` and follow a
  consistent shape (setup, install, postinstall, destroy, cleanup all
  use `New(version string, opts ...phase.BasePhaseOption) *Phase`).
  No option-inconsistency at the constructor level.
- Skip-listed: setup.Phase struct field BinDir is exported and read by
  internal/distribution/okd/okd.go:L141 — that is intentional cross-
  package wiring for the okd Provisioner facade. Not flagged.

## Cluster verdicts

- **package-boundary** — clean except for the executor.Env public-field
  mutation (api:d5915b0c). cmd/ never imports phase packages directly
  (goes through internal/cli or internal/distribution/okd). No
  bidirectional imports detected by `go list -deps`.
- **exported-surface** — three live findings (zeroize asymmetry,
  exposed fields, k8sclient stutter) plus three scaffolding rows.
  The dominant issue is the executor public-field/method gap.
- **option-consistency** — three findings (firewall positional-logger,
  cleanup dual-option, terraform mixed-shape). All three are localized
  drift from the shared phase.BasePhaseOption / functional-options
  norm. Each is a one-package fix.
- **ctx-first** — clean except for one scaffolding row
  (proxmox.Disconnect symmetric ctx). All other public methods
  satisfy the ctx-first contract.
- **zero-value-usability** — addon.Manager nil-logger inconsistency
  is the only finding. PhaseContext (distribution/context.go)
  correctly panics on zero-value with a clear "must be created via
  NewPhaseContext" message — not flagged.
- **interface-location** — clean. distribution.MetricsRecorder is
  small (3 methods), cohesive, and defined producer-side because
  the producer dispatches; consumers (deploymetrics) implement.
  addon.Addon, addon.ConfigurableAddon, addon.ToolProvider,
  addon.WizardProvider are split correctly along feature axes.

## Scope exceptions proposed

None. The default exclusion list (`tui/wizard`, templates,
`//go:build linux`, `_test.go`) captures the right coverage for an
API-shape audit. `cmd/okdctl-gen-docs` was confirmed as a real
caller of generated CLI surface and is not in scope of API churn.

## Footer

Total findings: 12 (blocker: 0, major: 1, minor: 5, suggestion: 6).
Scaffolding rows: 3 (api:48688e63, api:c287d5c0, api:0139cb3f) — all
capped at severity=suggestion per MEMORY.md §scaffolding.
Scope coverage: ~110 / 145 in-scope `.go` files read in full (76%);
the remainder were grepped for exported-surface signatures and
import-graph membership but not line-by-line inspected — no findings
emitted from those.
Seam deferrals: 2 (sec:7b2829bb:executor-no-zeroize,
sec:35abd54e:env-string-residue) — co-owned with audit-security via
api:7b2829bb:zeroize-asymmetry.
MEMORY.md present, scaffolding rule honored.
Validation: 12/12 rows validate against finding-schema.json
(all required fields, severity_reason on the major row,
scaffolding_reason on every scaffolding row, ID pattern matched).

To refresh linter-config-bugs.jsonl, run:
`jq -c 'select(.adjacent_linter_enabled==true)' .claude/audits/*.jsonl > .claude/audits/linter-config-bugs.jsonl`
or `/audit-all`.

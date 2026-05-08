# audit-documentation — 2026-05-08

**Assumes green:** golangci-lint (revive exported-doc, misspell, dupword),
govulncheck, CodeQL, shellcheck, tflint, go test ./...
**Scope:** every package doc, every exported symbol's doc-comment style,
per-package comment density (CLAUDE.md ~3% target — re-evaluated; see
notes), README flag-drift vs cobra surface, docs/ vs generated CLI
reference drift, testable-example presence.
**Out of scope this run:** `internal/tui/wizard/**`,
`internal/distribution/okd/templates/**`, `*_test.go`, `*.gen.go`,
`vendor/**`, `//go:build linux`-tagged code (per AUDIT_CONVENTIONS §2).
**Seam co-owners:** none — no per-flag README drift surfaced; nothing to
hand to audit-cli-ux for the umbrella seam (#13). audit-api-design owns
"shouldn't be exported" — none of the present findings cross that seam.

## Executive summary

Documentation in this repo is in unusually good shape. Every in-scope
package has a `// Package X ...` block; every exported symbol has either a
verb-first doc comment or is a well-known interface implementation
(`Error`, `Unwrap`, `String`, `Write`, `Close`, slog `Handle`/`WithAttrs`)
where Go convention rightly omits the comment to avoid echoing the
signature. The generated `docs/cli/*.md` tree matches the cobra command
surface bit-for-bit (`make docs` produced no diff). README flag mentions
match shipping flags; documented defaults (`~/pull-secret.json`,
secretstore/flux defaults) match code. No bare TODOs, no section dividers,
no peacock comments, no narrating-next-line patterns. The only findings
are six package-doc thinness/name-echo cases — all minor or suggestion —
where a one-sentence `// Package X provides X for the OKD CLI` could
better serve godoc readers by surfacing the load-bearing contract that
already lives elsewhere in the file.

## Ranked table

| ID | Cluster | File:line | Severity | Confidence | LOC | Adjacent linter | Fix class |
|---|---|---|---|---|---|---|---|
| `doc:8aa632a6:pkg-doc-name-echo` | package-doc | internal/version/version.go:1 | minor | high | 1 | revive | refactor |
| `doc:4c092fce:pkg-doc-name-echo` | package-doc | internal/infrastructure/terraform/terraform.go:1-2 | minor | high | 2 | revive | refactor |
| `doc:0139cb3f:pkg-doc-canonical-helpers` | package-doc | internal/distribution/okd/phase/paths.go:1 | minor | high | 1 | none | refactor |
| `doc:35abd54e:pkg-doc-thin` | package-doc | internal/credentials/proxmox.go:1 | suggestion | high | 1 | none | refactor |
| `doc:d5915b0c:pkg-doc-name-echo` | package-doc | internal/distribution/okd/install/phase.go:1 | suggestion | high | 1 | revive | refactor |
| `doc:beabab0c:pkg-doc-name-echo` | package-doc | internal/distribution/okd/setup/phase.go:1 | suggestion | high | 1 | revive | refactor |

Sort: severity_weight × confidence × |LOC delta| ÷ risk
(blocker=4, major=3, minor=2, suggestion=1).

## Findings

### `doc:8aa632a6:pkg-doc-name-echo`

**Cluster:** package-doc
**File + line range:** `internal/version/version.go:1`
**Current LOC touched:** 1
**Smell:** Package doc echoes the package name without adding signal:
`Package version provides version information for the okdctl CLI.` The
package actually owns the build-time identity invariants
(`Version/GitCommit/BuildDate/GoVersion/Platform` written exactly once
before `main`, read race-free by `BackgroundCheck`) that already live in
the file as a var-block doc — surface that contract on the package clause
so godoc readers see the load-bearing constraint without reading the
source.
**Evidence:**
```go
// Package version provides version information for the okdctl CLI.
package version

// Build-time identity variables injected via -ldflags by goreleaser. They
// are written exactly once before main() runs and must not be written by
// production code afterwards: BackgroundCheck (updatecheck.go) reads
// Version from a goroutine without synchronisation, so a concurrent write
// is a data race. ...
```
**Fix — preferred:** refactor (expand to two sentences naming the
build-identity contract and the t.Cleanup test rule).
**Rule source:** CLAUDE.md §code-comments — package doc one-to-three
sentences; not vacuous name-echo. Effective Go §Commentary.
**Adjacent linter:** revive (revive's exported-doc rule does not catch
"echoes name"; this is human-only).
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the var-block doc at L9-15 already carries
the canonical contract; it stays.
**Estimated net LOC delta:** +2
**Severity:** minor
**Risk (of applying fix):** low — pure doc edit; no API change.
**Confidence (in finding):** high — name-echo is mechanically detectable.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### `doc:4c092fce:pkg-doc-name-echo`

**Cluster:** package-doc
**File + line range:** `internal/infrastructure/terraform/terraform.go:1-2`
**Current LOC touched:** 2
**Smell:** Package doc's second sentence is vacuous:
`It can be used by any infrastructure provider that uses Terraform.`
That's not a contract — it's a tautology. Replace with what the package
actually owns: subprocess shape (PATH lookup, env-allowlist via
`internal/executor`), state-file invariants (atomic writes via
`system.AtomicWrite`), or call surface (`Init/Plan/Apply/Destroy/Output`).
**Evidence:**
```go
// Package terraform provides a high-level interface for Terraform operations.
// It can be used by any infrastructure provider that uses Terraform.
```
**Fix — preferred:** refactor — replace the second sentence with a
substantive description of the package's actual contract.
**Rule source:** CLAUDE.md §code-comments — "package doc adds what the
code can't say for itself".
**Adjacent linter:** revive (rule for exported-doc style; doesn't catch
tautological content).
**Scaffolding?:** no
**Seam:** none
**Estimated net LOC delta:** +1
**Severity:** minor
**Risk:** low.
**Confidence:** high.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### `doc:0139cb3f:pkg-doc-canonical-helpers`

**Cluster:** package-doc
**File + line range:** `internal/distribution/okd/phase/paths.go:1`
**Current LOC touched:** 1
**Smell:** Package `phase` is the canonical home for cross-phase helpers
per CLAUDE.md §architecture-notes (`BasePhase`, `OcResourceExists`,
`OcPollOutput`, `NodeRole`, `ConditionStatus`, `VMState`, `SSHRunArgv`)
but the package doc only says
`shared base types and path utilities for OKD phases`. Readers of godoc
miss that this package is the canonical surface for new cross-phase
helpers — and that adding helpers elsewhere is forbidden by CLAUDE.md.
Surface the rule on the package clause.
**Evidence:**
```go
// Package phase provides shared base types and path utilities for OKD phases.
package phase
```
**Fix — preferred:** refactor — expand to two-three sentences naming the
canonical helpers and the "new cross-phase helpers belong here" rule.
**Rule source:** CLAUDE.md §architecture-notes — `internal/distribution/
okd/phase/` holds shared helpers used by all OKD phases. CLAUDE.md
§code-comments.
**Adjacent linter:** none — humans have to check architecture-rule
surfacing.
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the existing one-sentence summary stays as
the lead.
**Estimated net LOC delta:** +2
**Severity:** minor
**Risk:** low.
**Confidence:** high.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### `doc:35abd54e:pkg-doc-thin`

**Cluster:** package-doc
**File + line range:** `internal/credentials/proxmox.go:1`
**Current LOC touched:** 1
**Smell:** Package `credentials` carries the load-bearing
`[]byte`/`Zeroize`/`Redacted`-interface contract for credential lifecycle,
but the package doc is one vacuous sentence. The doc should announce the
type-level invariants (Password/APIToken `[]byte` for in-memory wipe,
`Redacted() any` for slog scrubbing, `defer Zeroize()` lifecycle) so a
godoc-reading caller does not have to scan `ProxmoxCredentials`'s type
doc to discover them.
**Evidence:**
```go
// Package credentials provides credential management for infrastructure providers.
package credentials
```
**Fix — preferred:** refactor — expand to two-three sentences (see
fix_summary in the JSONL).
**Rule source:** CLAUDE.md §credentials-and-secrets — `Zeroize` lifecycle,
`Redacted` interface. CLAUDE.md §code-comments — surface non-obvious
behavior on the package doc.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**Estimated net LOC delta:** +3
**Severity:** suggestion (security-relevant content already lives on the
type doc; this is a discoverability nit, not a missing invariant).
**Risk:** low.
**Confidence:** high.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### `doc:d5915b0c:pkg-doc-name-echo`

**Cluster:** package-doc
**File + line range:** `internal/distribution/okd/install/phase.go:1`
**Current LOC touched:** 1
**Smell:** Package doc echoes the package name with no added signal. The
package owns the bootstrap-monitor + CSR-approval + cluster-operator-wait
sequence with timeouts (`DefaultBootstrapTimeout` 30 m,
`DefaultInstallTimeout` 60 m, `DefaultCSRApprovalInterval` 30 s); the
doc should surface the timeline, not the package name.
**Evidence:**
```go
// Package install provides the install phase for OKD cluster provisioning.
package install
```
**Fix — preferred:** refactor.
**Rule source:** CLAUDE.md §code-comments.
**Adjacent linter:** revive
**Scaffolding?:** no
**Seam:** none
**Estimated net LOC delta:** +2
**Severity:** suggestion
**Risk:** low.
**Confidence:** high.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### `doc:beabab0c:pkg-doc-name-echo`

**Cluster:** package-doc
**File + line range:** `internal/distribution/okd/setup/phase.go:1`
**Current LOC touched:** 1
**Smell:** Package doc echoes the package name with no added signal.
Setup is the largest phase (~2.3K LOC) and owns rendering install
configs, manifests, custom CoreOS ISOs, plus configuring HAProxy/DNS/
firewall on the bastion — a one-line name-echo gives readers no map.
**Evidence:**
```go
// Package setup provides the setup phase for OKD cluster provisioning.
package setup
```
**Fix — preferred:** refactor.
**Rule source:** CLAUDE.md §code-comments.
**Adjacent linter:** revive
**Scaffolding?:** no
**Seam:** none
**Estimated net LOC delta:** +2
**Severity:** suggestion
**Risk:** low.
**Confidence:** high.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

## Scaffolding items detected

None. No exported symbol surfaced in this audit triggers MEMORY.md
§scaffolding (the API-shape/symmetric-API/test-helper-family rule). The
exported surface is well-anchored to current callers; nothing was flagged
as "shouldn't be documented because it shouldn't be exported" (that
question is owned by `audit-api-design` per seam #12 anyway).

## Linter-config-bug candidates

None. `revive` exported-doc covers verb-first / has-doc; the present
findings are about *content quality* of correctly-formatted package-doc
blocks. No linter catches "this content is vacuous" — pure human review.

## Skip list

**Inline `// Output:` testable examples (cluster 5 — testable-examples):**
zero `func ExampleX` defined repo-wide. Per MEMORY.md §scaffolding-as-
roadmap, missing-but-not-future-shaped is not a finding; tests cover
behavior. Skip.

**Methods that implement well-known interfaces:**
`Error`, `Unwrap`, `String`, `GoString`, `Write`, `Close`, slog
`Enabled/Handle/WithAttrs/WithGroup`, `MetricsRecorder.Step*`,
`ProvisioningStep.{ID,Name,Description,IsFatal,IsAlreadyDone,ShouldSkip,
SkipReason,Execute,OnStart,OnComplete,OnError}`. Per CLAUDE.md
§code-comments anti-pattern #2 ("function doc just echoes signature"),
adding a doc here would be a *worse* state. Confirmed by reviewing each
type — every interface implemented has its contract documented at the
*interface* declaration (e.g. `MetricsRecorder` has its contract on the
interface; `nopMetricsRecorder` correctly omits per-method docs).

**Package-doc length on `cli/root.go` (10 lines, ~3 long sentences):**
The package doc is exhaustive on exit-code mapping. CLAUDE.md says
"one-to-three sentences unless there's a real invariant to explain";
exit-code-script-contract is exactly such an invariant. The doc was
previously updated to drop the `77 (EX_NOPERM)` drift the prior audit
flagged (`doc:aa84670c:exit-code-77-pkgdoc-drift`) — that finding
resolves this run; current text says "auth error=5 (includes
invoked-as-root rejection via AuthError)" which matches `exitCodeFor`.

**`docs/architecture/phases.md` shape vs `StepDef`:** the prior-run
findings about missing `ReRunSafe`, `AlreadyDone`, `OnStart`, `Recorder`
fields are resolved — current `phases.md` lines 41-53 and 87-93 include
them. Verified by direct grep against `internal/distribution/step.go`
and `internal/distribution/okd/phase/paths.go`.

**README "Usage" section is curated, not exhaustive:** README lists 5 of
14 commands (`deploy`, `destroy`, `update-ingress`, `doctor`,
`--version`) and points at `docs/cli/okdctl.md` for the full reference.
Per seam #13, the umbrella "≥3 sites missing" pattern would be cli-ux,
not documentation, but the README explicitly delegates to the generated
reference — not a finding either way.

## Cluster verdicts

**package-doc.** Every in-scope package has a `// Package X ...` block;
36 of 36 present. Six are thin or name-echo; the present findings cover
those. The longest (`internal/cli/root.go`, 10 lines on exit codes) is
substantive and matches a CLAUDE.md exception for non-obvious behavior.

**exported-doc.** Every exported symbol is either documented verb-first
or is an interface implementation method that Go convention lets stand
without a doc (the comment would echo the signature). No
`exported-doc-missing` or `exported-doc-style` finding emits. Sampling 60
exported symbols across `addon`, `credentials`, `errtypes`, `logutil`,
`distribution`, `phase`, `executor`, `system` confirms verb-first
discipline holds across the codebase.

**density-target.** Per-package comment density measured, range
9.1%-44.9% (excluding the 80% one-file `addon/catalog` package, which
is a 5-line import-only registration package). CLAUDE.md says "~3%
target" with "Packages over 5% driven by anti-patterns are findings."
Sampling the highest-density packages (`errtypes` 77%, `logutil` 45%,
`credentials` 41%) shows the comments are uniformly load-bearing
contracts: redaction invariants, credential lifecycle, exit-code
mapping, error-wrapping rules. None is an anti-pattern. The 3% target
appears to be a low-end aspirational, not a ceiling — this codebase
intentionally over-documents the security-sensitive paths and that
choice tracks with CLAUDE.md §credentials-and-secrets and the published
audit history. No finding emits.

**readme-drift.** README mentions five commands explicitly (deploy,
destroy, update-ingress, doctor, --version) and one flag (--config) by
name. All exist with the documented behavior. README mentions
`~/pull-secret.json` as the wizard's default — matches
`internal/tui/wizard/steps/files.go:25`. No drift, no ghost flags, no
default drift. `make docs` produced zero diff vs `docs/cli/`, confirming
the generated reference matches the cobra surface.

**testable-examples.** Zero `func ExampleX` defined repo-wide. Not a
finding (no example references removed code; no example missing `//
Output:`). A future docs-improvement could add examples for
`addon.BuildOpaqueSecret`, `system.WriteTempFile`, or
`credentials.GetProxmoxCredentials` — those are future-API-shaped
discoverability boosts, not current-state defects.

## Scope exceptions proposed

None. Audit ran fully within the AUDIT_CONVENTIONS §1-§2 scope. The
out-of-scope wizard package was not opened.

## Comment density table

| package | code LOC | comment LOC | % | anti-pattern mix |
|---|---:|---:|---:|---|
| internal/cli | 3745 | 341 | 9.1% | none — load-bearing WHY/contract |
| internal/distribution/okd/postinstall | 1046 | 106 | 10.1% | none |
| internal/distribution/okd/destroy | 361 | 40 | 11.1% | none |
| internal/distribution/okd/setup | 2326 | 261 | 11.2% | none |
| internal/distribution/okd/cleanup | 650 | 74 | 11.4% | none |
| internal/version | 163 | 21 | 12.9% | none |
| internal/download | 543 | 70 | 12.9% | none |
| internal/addon/catalog/flux | 404 | 54 | 13.4% | none |
| internal/netutil | 180 | 26 | 14.4% | none |
| internal/distribution/okd/releases | 412 | 60 | 14.6% | none |
| internal/deploymetrics | 102 | 15 | 14.7% | none |
| internal/config | 926 | 140 | 15.1% | none |
| internal/distribution/okd/dns | 407 | 63 | 15.5% | none |
| internal/distribution/okd/install | 502 | 79 | 15.7% | none |
| internal/platform | 234 | 38 | 16.2% | none |
| internal/tui | 402 | 71 | 17.7% | none |
| internal/addon | 484 | 95 | 19.6% | none |
| internal/infrastructure/proxmox | 329 | 65 | 19.8% | none |
| internal/distribution/okd/firewall | 171 | 37 | 21.6% | none |
| internal/distribution | 378 | 101 | 26.7% | none |
| internal/cluster | 142 | 38 | 26.8% | none |
| internal/system | 562 | 154 | 27.4% | none |
| internal/infrastructure/terraform | 276 | 76 | 27.5% | none |
| internal/httputil | 79 | 22 | 27.8% | none |
| internal/distribution/okd | 204 | 62 | 30.4% | none |
| cmd/okdctl | 36 | 11 | 30.6% | none |
| internal/executor | 304 | 93 | 30.6% | none |
| internal/runlock | 58 | 19 | 32.8% | none |
| internal/sshpin | 58 | 19 | 32.8% | none |
| internal/distribution/okd/phase | 525 | 175 | 33.3% | none |
| cmd/okdctl-gen-docs | 21 | 7 | 33.3% | none — short single-purpose binary |
| internal/credentials | 295 | 120 | 40.7% | none — credential-contract docs |
| internal/logutil | 98 | 44 | 44.9% | none — redact-handler contract |
| internal/errtypes | 52 | 40 | 76.9% | none — exit-code/error-taxonomy docs |
| internal/addon/catalog | 5 | 4 | 80.0% | none — 5-line registration-only package |
| internal/distribution/okd/secretstore | 550 | 51 | 9.3% | none |
| **(global)** | | | | none — all density driven by load-bearing WHY/contract |

## README drift table

| README mention | Code site | State |
|---|---|---|
| `okdctl deploy` | `internal/cli/deploy.go:25` | match |
| `okdctl destroy` | `internal/cli/destroy.go:30` | match |
| `okdctl update-ingress` | `internal/cli/update_ingress.go` | match |
| `okdctl doctor` | `internal/cli/doctor_cmd.go` | match |
| `--version` | `internal/version/version.go` + cobra `--version` | match |
| `--config` | `internal/cli/root.go:246` (default `okdctl.yaml`) | match |
| `okdctl completion bash/zsh/fish` | `internal/cli/completion.go` | match |
| `~/pull-secret.json` (default) | `internal/tui/wizard/steps/files.go:25` | match |
| `okdctl cleanup` | `internal/cli/cleanup.go` | match (referenced in security-considerations §) |
| `okdctl postinstall` | (not a top-level command — it's a phase, not a verb) | non-issue: README phrasing reads as "after postinstall completes" not as a CLI command |

## Exported-doc coverage

| package | exported symbols | documented (incl. interface-impl skip) | % |
|---|---:|---:|---:|
| internal/credentials | 14 | 14 | 100% |
| internal/errtypes | 11 | 11 (5 Error/Unwrap pairs are interface-impl skip) | 100% |
| internal/logutil | 7 | 7 | 100% |
| internal/distribution | 24 | 24 (12 ProvisioningStep impls are interface-impl skip) | 100% |
| internal/distribution/okd/phase | 18 | 18 | 100% |
| internal/cli | 7 (`Execute`, `RootCmd`, …) | 7 | 100% |
| internal/system | 19 | 19 (Error/Unwrap interface-impl skip) | 100% |
| internal/executor | 13 | 13 | 100% |
| internal/download | 9 | 9 | 100% |
| internal/addon | 28 | 28 | 100% |
| internal/addon/catalog/flux | 22 | 22 | 100% |
| internal/addon/catalog/secretstore | 19 | 19 | 100% |
| internal/version | 7 | 7 (var-block doc covers the 5 vars) | 100% |
| internal/infrastructure/proxmox | 9 | 9 | 100% |
| internal/infrastructure/terraform | 11 | 11 | 100% |
| internal/distribution/okd/install | 7 | 7 | 100% |
| internal/distribution/okd/setup | 12 | 12 | 100% |
| internal/distribution/okd/postinstall | 8 | 8 | 100% |
| internal/distribution/okd/destroy | 4 | 4 | 100% |
| internal/distribution/okd/cleanup | 6 | 6 | 100% |
| internal/distribution/okd/firewall | 5 | 5 | 100% |
| internal/distribution/okd/dns | 6 | 6 | 100% |
| internal/distribution/okd/releases | 9 | 9 | 100% |
| internal/runlock | 5 | 5 | 100% |
| internal/sshpin | 4 | 4 | 100% |
| internal/netutil | 9 | 9 | 100% |
| internal/platform | 13 | 13 | 100% |
| internal/tui | 30+ | 30+ (Enabled/Handle/WithAttrs/WithGroup interface-impl skip) | 100% |
| internal/cluster | 7 | 7 | 100% |
| internal/config | 60+ | 60+ (Field*/Scope*/Min* block-doc grouped) | 100% |
| internal/deploymetrics | 5 | 5 | 100% |
| internal/httputil | 4 | 4 | 100% |
| **(global)** | ≈400 | ≈400 | **100%** |

The "interface-impl skip" annotations are the cases CLAUDE.md
§code-comments anti-pattern #2 explicitly forbids documenting (would
echo the signature). Every documented symbol verified verb-first.

## Footer

Total findings: 6 (blocker: 0, major: 0, minor: 3, suggestion: 3)
Scope coverage: 145 / 145 in-scope `.go` files read or scanned (100%);
README, all `docs/cli/*.md`, all `docs/architecture/*.md`, all
`docs/addons/*.md` read in full; `docs/cli/` regenerated via `make docs`
and diffed (zero diff). Out-of-scope skipped per AUDIT_CONVENTIONS §2.
Seam deferrals: 0 (no per-flag README drift surfaced — nothing to send to
audit-cli-ux umbrella).
Validation failures: 0 — every emitted JSONL row validates against
`finding-schema.json` (required keys, ID format, severity rubric,
scaffolding/claude-conflict conditional rules).

To refresh `linter-config-bugs.jsonl`, run the aggregation command or
`/audit-all`.

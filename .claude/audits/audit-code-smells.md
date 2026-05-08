# audit-code-smells — 2026-05-08

**Assumes green:** golangci-lint, govulncheck, CodeQL, shellcheck, tflint, go test ./...
**Scope:** internal/**, cmd/**, .github/workflows/*.yml — all in-scope Go files read in full or via grep + targeted reads.
**Out of scope this run:** internal/tui/wizard/**, internal/distribution/okd/templates/**, internal/distribution/okd/setup/iso.go, vendor/**, *.gen.go, //go:build linux test files.
**Seam co-owners:** audit-errors (release_extract auth-string sniffing, release-type unmarshal silent-downgrade), audit-api-design (duplicate role enum across phase / proxmox), audit-cli-ux (`--only` stringly-typed scope flag).

## Executive summary

This is the catch-all sweep — specialized audits already own arrow-pyramids (concurrency), exit-code mapping (cli-ux + errors), and stdlib migration (modernization). What remains is the magic-strings cluster: 13 of 18 findings are stringly-typed enums, ad-hoc string contracts, or duplicated label switches the repo has no single typed home for. Severity skews suggestion / minor — none are correctness bugs but several are lopsided patterns the repo has half-converted (cli/flags.go centralizes 2 of 6 shared flag names; debug_bundle wraps `bundleStatus` but not `categoryX`; releases marshals through one switch but the cli prints from a duplicate copy of it). The two findings to fix first (smell:e7db1220 duplicate-enum-label-fn and smell:fd2125dd output-flag-magic-string) align directly with established repo counter-examples and are zero-risk refactors. The most subtle is smell:632c9087 — the IngressStrategy enum is open-ended on the producer side and closed on the consumer side, so a NodePort-strategy controller routes through HostNetwork-branch logic silently.

## Ranked table

Sort weight: severity_weight × confidence × |LOC delta| ÷ risk (blocker=4 / major=3 / minor=2 / suggestion=1; high=3 / medium=2 / low=1).

| ID | Cluster | File:line | Severity | Confidence | LOC | Adjacent linter | Fix class |
|---|---|---|---|---|---|---|---|
| smell:e7db1220:duplicate-enum-label-fn | helper-package-no-value | internal/cli/releases.go:209-224 | minor | high | 32 | dupl (enabled) | refactor |
| smell:fd2125dd:output-flag-magic-string | magic-strings | internal/cli/addon.go:102-103 (+5 sites) | minor | high | 7 | goconst (enabled) | refactor |
| smell:b5a79fda:deploy-phase-stringly-typed | magic-strings | internal/cli/deploystate.go:75-86 | minor | high | 14 | goconst (enabled) | refactor |
| smell:26a430ee:requires-root-annotation-key | magic-strings | internal/cli/elevation.go:51 | minor | high | 3 | goconst (enabled) | refactor |
| smell:0f076161:destroy-only-stringly-typed | magic-strings | internal/cli/destroy.go:45-78 | suggestion | medium | 35 | goconst (enabled) | refactor |
| smell:d6b325cb:duplicate-role-enum | magic-strings | internal/infrastructure/proxmox/types.go:42-49 | minor | high | 40 | none | refactor |
| smell:632c9087:ingress-strategy-default-shadow | magic-strings | internal/distribution/okd/postinstall/update_ingress.go:341-347 | minor | medium | 7 | exhaustive (off) | refactor |
| smell:1d5afa08:release-type-unknown-default | magic-strings | internal/distribution/okd/releases/types.go:85-107 | suggestion | high | 23 | none | refactor |
| smell:62cb8a95:state-major-bounds-misnamed | magic-strings | internal/distribution/okd/destroy/helpers.go:26-59 | suggestion | high | 34 | none | refactor |
| smell:08ec0042:flags-package-not-canonical | helper-package-no-value | internal/cli/flags.go:1-10 | suggestion | medium | 10 | none | refactor |
| smell:d31d1b9d:health-stringly-typed | magic-strings | internal/cli/status.go:258-268 | suggestion | medium | 20 | goconst (enabled) | refactor |
| smell:d9f7733e:bundle-category-magic-string-pair | magic-strings | internal/cli/debug_bundle.go:29-36 | suggestion | high | 8 | none | refactor |
| smell:8154ab0f:doctor-severity-string-roundtrip | magic-strings | internal/cli/doctor.go:87-98 | suggestion | medium | 12 | none | refactor |
| smell:6424733c:metrics-shutdown-timeout-magic | magic-strings | internal/cli/helpers.go:228-256 | suggestion | medium | 12 | none | refactor |
| smell:5013fea6:auth-error-string-sniff | magic-strings | internal/distribution/okd/setup/release_extract.go:92-141 | suggestion | medium | 50 | none | refactor |
| smell:daf5bee9:any-yaml-traversal | interfaceany-lazy | internal/cli/kubeconfig.go:141-175 | suggestion | medium | 36 | none | refactor |
| smell:92553fff:summary-hardcoded-3state-fmt | magic-strings | internal/cli/summary.go:177 | suggestion | high | 4 | none | refactor |
| smell:39c75e91:yes-no-magic-strings | magic-strings | internal/cli/confirm.go:60-62 | suggestion | medium | 3 | none | refactor |

## Findings

### smell:e7db1220:duplicate-enum-label-fn

**Cluster:** helper-package-no-value
**File + line range:** internal/cli/releases.go:L209-L224 (+ internal/distribution/okd/releases/types.go:L62-L77)
**Current LOC touched:** 32
**Smell:** Two identical switch tables on `ReleaseType` — the cli-side `releaseTypeLabel` and the package-internal `labelForReleaseType` — exist because the package never exposed `String()`. MarshalJSON already calls `labelForReleaseType`; collapsing both into `ReleaseType.String()` would let the cli printer use the same code path the marshaller does.
**Evidence:**
```go
// cli/releases.go:209
func releaseTypeLabel(t releases.ReleaseType) string {
    switch t {
    case releases.ReleaseTypeStable: return "stable"
    ...

// releases/types.go:62
func labelForReleaseType(t ReleaseType) string {
    switch t {
    case ReleaseTypeStable: return "stable"
    ...
```
**Fix — preferred:** refactor — add `func (t ReleaseType) String() string` containing the existing labelForReleaseType body, drop labelForReleaseType, change MarshalJSON to call String(), delete cli/releases.go releaseTypeLabel and call .String() at both print sites.
**Rule source:** Go proverb (clear is better than clever); Uber §Stringly Typed; repo-counter-example: internal/distribution/okd/phase/noderole.go:L34 (NodeRole.String).
**Adjacent linter:** dupl (enabled — threshold 200; this duplicate is too small to fire)
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** MarshalJSON's "release_type":"stable" wire format — `String()` keeps the same return values.
**Estimated net LOC delta:** -13
**Severity:** minor
**Risk (of applying fix):** low — pure substitution, single-package internal symbol drop.
**Confidence (in finding):** high — diff was verified character-by-character.
**CLAUDE.md / MEMORY.md conflict?:** no

### smell:fd2125dd:output-flag-magic-string

**Cluster:** magic-strings
**File + line range:** internal/cli/addon.go:L102-L103 (+ internal/cli/doctor_cmd.go:L39, internal/cli/releases.go:L78-L80, internal/cli/status.go:L68-L70; 2 more not shown)
**Current LOC touched:** 7 across 7 sites
**Smell:** The kubectl-style `--output`/`-o` format flag is registered with the bare string `"output"` (and shorthand `"o"`) at 7 call sites. cli/flags.go already holds the symmetric `flagOutput` constant for the file-destination flag (`--output-file`) with a doc comment naming the typo-guard rationale. The format flag has the same property and no constant.
**Evidence:**
```go
addonListCmd.Flags().StringVarP(&addonListOutput, "output", "o", outputText, ...)
addonVerifyCmd.Flags().StringVarP(&addonVerifyOutput, "output", "o", outputText, ...)
doctorCmd.Flags().StringVarP(&doctorOutput, "output", "o", outputText, ...)
releasesListCmd.Flags().StringVarP(&releasesListOutput, "output", "o", outputText, ...)
statusCmd.Flags().StringVarP(&statusOutput, "output", "o", outputText, ...)
```
**Fix — preferred:** refactor — add `flagOutputFormat = "output"` and `flagOutputFormatShort = "o"` to cli/flags.go, replace 7 sites.
**Rule source:** repo-counter-example: internal/cli/flags.go:L7-L10; CLAUDE.md §architecture-notes.
**Adjacent linter:** goconst (enabled — min-occurrences 3; should fire here)
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** every CLI surface contract (`okdctl X --output` / `-o` / values "text"|"json").
**Estimated net LOC delta:** +2
**Severity:** minor
**Severity reason:** rubric §4/un-idiomatic-pattern (CLAUDE.md §architecture-notes explicitly names the kubectl-style `--output` convention — repeating the literal at 7 sites under-codifies it).
**Risk (of applying fix):** low.
**Confidence (in finding):** high — grep verified the 7 sites.
**CLAUDE.md / MEMORY.md conflict?:** no — the fix would *strengthen* CLAUDE.md adherence.

### smell:b5a79fda:deploy-phase-stringly-typed

**Cluster:** magic-strings
**File + line range:** internal/cli/deploystate.go:L75-L86 (+ internal/cli/helpers.go:L315, L327, L341)
**Current LOC touched:** 14
**Smell:** The deploy-phase marker uses three bare strings (`"prepare"`, `"install"`, `"configure"`) written by helpers.go and read by deploystate.go's switch. Producer/consumer string contract with no typed enum; the repo has already replaced this exact shape for cleanup.Kind, summary.stepDisplayStatus, debug_bundle.bundleStatus, platform.Family.
**Evidence:**
```go
// helpers.go
markDeployPhase(markerPath, "prepare", runID)
markDeployPhase(markerPath, "install", runID)
markDeployPhase(markerPath, "configure", runID)

// deploystate.go
switch ds.Phase {
case "prepare": ...
case "install", "configure": ...
}
```
**Fix — preferred:** refactor — `type deployPhase string` with constants alongside `deployState` in deploystate.go; update markDeployPhase signature and the three call sites. JSON wire format unchanged.
**Rule source:** Uber §Stringly Typed; repo-counter-examples: cleanup.Kind, stepDisplayStatus, bundleStatus.
**Adjacent linter:** goconst (enabled — should fire on the helpers.go writers, possibly suppressed)
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the `.okdctl-deploy-state.json` on-disk format (Phase remains a JSON-string).
**Estimated net LOC delta:** +4
**Severity:** minor
**Severity reason:** rubric §4/un-idiomatic-pattern (a typo on either producer or consumer side silently mismatches and skips the phase-specific destroy hint).
**Risk (of applying fix):** low.
**Confidence (in finding):** high.
**CLAUDE.md / MEMORY.md conflict?:** no.

### smell:26a430ee:requires-root-annotation-key

**Cluster:** magic-strings
**File + line range:** internal/cli/elevation.go:L51 (+ internal/cli/addon.go:L47, L82)
**Current LOC touched:** 3
**Smell:** `annotationValueTrue` is centralized but the cobra annotation key `"requiresRoot"` is repeated as a bare string at 3 sites. A typo silently disables the privilege gate.
**Evidence:**
```go
// elevation.go:51
if cmd.Annotations["requiresRoot"] == annotationValueTrue {

// addon.go:47, 82
Annotations: map[string]string{"requiresRoot": annotationValueTrue},
```
**Fix — preferred:** refactor — add `const annotationKeyRequiresRoot = "requiresRoot"` alongside annotationValueTrue.
**Rule source:** repo-counter-example: cli/flags.go:L7-L10 (same typo-guard rationale spelled out in the comment).
**Adjacent linter:** goconst (enabled — 3 occurrences exactly hits min-occurrences=3).
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the privilege-gate matching contract.
**Estimated net LOC delta:** +1
**Severity:** minor
**Severity reason:** rubric §4/un-idiomatic-pattern (typo silently disables a privilege gate; the comment on annotationValueTrue already names typo-guard as the rationale).
**Risk (of applying fix):** low.
**Confidence (in finding):** high.
**CLAUDE.md / MEMORY.md conflict?:** no.

### smell:0f076161:destroy-only-stringly-typed

**Cluster:** magic-strings
**File + line range:** internal/cli/destroy.go:L45-L78
**Current LOC touched:** 35
**Smell:** `--only` accepts the bare strings "bootstrap"/"masters"/"workers"/"vms" via a switch with help text and an error message duplicating the arms. cleanup.Kind in a sibling package follows the typed-enum + ValidKinds pattern; --only does not.
**Evidence:**
```go
switch only {
case "bootstrap": targets = []string{prefix + "bootstrap[0]"}
case "masters": ...
case "workers": ...
case "vms": ...
default:
    return nil, &errtypes.ConfigError{Msg: fmt.Sprintf(
        "--only %q is not valid; choose one of: vms, workers, masters, bootstrap", only)}
}
```
**Fix — preferred:** refactor — `type destroyScope string` + scopeBootstrap/scopeMasters/scopeWorkers/scopeVMs + validDestroyScopes() shared by switch, error message, help, completion.
**Rule source:** repo-counter-example: cleanup/cleanup.go:L36-L50 (ValidKinds + KindStrings + Kind.Validate).
**Adjacent linter:** goconst (enabled).
**Scaffolding?:** no
**Seam:** audit-cli-ux (taxonomy of --only values is a UX surface; this audit catalogs the body shape).
**What MUST stay bit-for-bit:** the on-CLI surface (`--only=vms` etc).
**Estimated net LOC delta:** +4
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** medium — the fix's *value* is medium because there are only 4 arms.
**CLAUDE.md / MEMORY.md conflict?:** no.

### smell:d6b325cb:duplicate-role-enum

**Cluster:** magic-strings
**File + line range:** internal/infrastructure/proxmox/types.go:L42-L49 (+ internal/distribution/okd/phase/noderole.go:L10-L34)
**Current LOC touched:** 40
**Smell:** Two parallel typed enums name the same domain concept: `phase.NodeRole` and `proxmox.VMRole`. Same string values verbatim. Each comment justifies its placement; the proxmox justification ("avoid import cycle") is one-directional — proxmox already imports phase for VMState.
**Evidence:**
```go
// phase/noderole.go
const (
    RoleBootstrap NodeRole = "bootstrap"
    RoleMaster    NodeRole = "master"
    ...

// infrastructure/proxmox/types.go
const (
    RoleBootstrap VMRole = "bootstrap"
    RoleMaster    VMRole = "master"
    ...
```
**Fix — preferred:** refactor — `type VMRole = phase.NodeRole` re-export (or drop VMRole, retype VMStatus.Role to phase.NodeRole and update 3 callers in proxmox/proxmox.go).
**Rule source:** Uber §Enums (single source of truth per domain concept); the existing phase/vmstate.go header comment names this exact pattern as solved.
**Adjacent linter:** none — `dupl` won't fire across packages.
**Scaffolding?:** no
**Seam:** audit-api-design — pairwise/package-level shape question is api-design's; the duplicate-enum *body* is what this audit catches. Cross-link.
**What MUST stay bit-for-bit:** the Terraform tfvars role strings ("bootstrap"/"master"/"worker") consumed by the HCL templates; both enums already share them.
**Estimated net LOC delta:** -9
**Severity:** minor
**Severity reason:** rubric §4/un-idiomatic-pattern (two typed enums for the same concept; proxmox already imports phase for VMState so the import-cycle justification on each side is asymmetric).
**Risk (of applying fix):** medium — callers may rely on switch-exhaustive over the proxmox values.
**Confidence (in finding):** high.
**CLAUDE.md / MEMORY.md conflict?:** no.

### smell:632c9087:ingress-strategy-default-shadow

**Cluster:** magic-strings
**File + line range:** internal/distribution/okd/postinstall/update_ingress.go:L341-L347
**Current LOC touched:** 7
**Smell:** `IngressStrategy` is typed but only two values (HostNetwork, LoadBalancerService) are declared. discoverIngressControllers shoves any unknown spec.type into the typed value. A NodePortService or Private controller would silently flow through HostNetwork-branch logic — neither path matches the actual K8s strategy.
**Evidence:**
```go
const (
    strategyHostNetwork  IngressStrategy = "HostNetwork"
    strategyLoadBalancer IngressStrategy = "LoadBalancerService"
)
// discoverIngressControllers:
strategy := strategyHostNetwork
if item.Spec.EndpointPublishingStrategy != nil && item.Spec.EndpointPublishingStrategy.Type != "" {
    strategy = item.Spec.EndpointPublishingStrategy.Type  // unknown strings flow through
}
```
**Fix — preferred:** refactor — either widen the enum (NodePortService / Private constants + explicit routing) or narrow with `parseIngressStrategy(string) (IngressStrategy, ok bool)` and warn-skip unrecognised strategies.
**Rule source:** repo-counter-example: phase/noderole.go:L25-L32 (ParseNodeRole rejects unknown).
**Adjacent linter:** exhaustive (off) — switching it on would catch this.
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the current routing of the two declared strategies.
**Estimated net LOC delta:** +12
**Severity:** minor
**Severity reason:** rubric §4/un-idiomatic-pattern (the typed enum is open-ended on producer side and closed on consumer side; a NodePort controller silently routes through HostNetwork branch logic).
**Risk (of applying fix):** low.
**Confidence (in finding):** medium — depends on whether NodePort/Private deployments are out of scope for okdctl by policy.
**CLAUDE.md / MEMORY.md conflict?:** no.

### smell:1d5afa08:release-type-unknown-default

**Cluster:** magic-strings
**File + line range:** internal/distribution/okd/releases/types.go:L85-L107
**Current LOC touched:** 23
**Smell:** UnmarshalJSON falls through to ReleaseTypeStable on unknown input; labelForReleaseType returns "unknown" for the same out-of-range input. Marshal-side and unmarshal-side disagree on the meaning of unknown.
**Evidence:**
```go
func (t *ReleaseType) UnmarshalJSON(data []byte) error {
    ...
    case "lts": *t = ReleaseTypeLTS
    default: *t = ReleaseTypeStable  // silent downgrade
}

func labelForReleaseType(t ReleaseType) string {
    ...
    default: return "unknown"
}
```
**Fix — preferred:** refactor — either reject unknown values (`return fmt.Errorf("unknown release type %q", s)`) or add explicit ReleaseTypeUnknown constant for round-trip.
**Rule source:** Uber §Errors ("don't silently downgrade"); Go proverb ("errors are values").
**Adjacent linter:** none.
**Scaffolding?:** no
**Seam:** audit-errors — error-creation discipline at the wrap boundary.
**What MUST stay bit-for-bit:** the cache-format compat for older entries.
**Estimated net LOC delta:** 0
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** high.
**CLAUDE.md / MEMORY.md conflict?:** no.

### smell:62cb8a95:state-major-bounds-misnamed

**Cluster:** magic-strings
**File + line range:** internal/distribution/okd/destroy/helpers.go:L26-L59
**Current LOC touched:** 34
**Smell:** `const stateMajorMin, stateMajorMax = 1, 1` — two constants whose values are identical, named asymmetrically, and used in a range check. The min/max framing implies range; the body enforces equality.
**Evidence:**
```go
const stateMajorMin, stateMajorMax = 1, 1
...
if major < stateMajorMin || major > stateMajorMax {
```
**Fix — preferred:** refactor — `const requiredTerraformMajor = 1` and `if major != requiredTerraformMajor`. When v2 lands, switch to a typed range struct.
**Rule source:** Go proverb (clear is better than clever); Uber §Local Variable Declarations.
**Adjacent linter:** none.
**Scaffolding?:** no
**Seam:** none.
**What MUST stay bit-for-bit:** the operator-facing error message (which already cites a single major number).
**Estimated net LOC delta:** -1
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** high.
**CLAUDE.md / MEMORY.md conflict?:** no.

### smell:08ec0042:flags-package-not-canonical

**Cluster:** helper-package-no-value
**File + line range:** internal/cli/flags.go:L1-L10
**Current LOC touched:** 10
**Smell:** cli/flags.go is a 10-line file holding two constants. The doc-comment correctly explains the typo-guard rationale, but the file holds only two flag names while the same package has at least 4 other repeated flag-name strings (`--output`, `-o`, `--config`, `requiresRoot`). The half-and-half state is the smell.
**Evidence:**
```go
const (
    flagDryRun = "dry-run"
    flagOutput = "output-file"
)
```
**Fix — preferred:** refactor — expand cli/flags.go to hold every shared flag-name constant; or inline the existing two and delete the file.
**Rule source:** repo-counter-example: cli/flags.go's own doc comment names the typo-guard property.
**Adjacent linter:** none.
**Scaffolding?:** no — flags.go isn't a future-CLI-verb shape.
**Seam:** none
**Related:** smell:fd2125dd, smell:26a430ee.
**What MUST stay bit-for-bit:** the `--dry-run` / `--output-file` CLI surface.
**Estimated net LOC delta:** +15 (if expanded).
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** medium — the right outcome depends on whether the package wants to grow into a flag-table file.
**CLAUDE.md / MEMORY.md conflict?:** no.

### smell:d31d1b9d:health-stringly-typed

**Cluster:** magic-strings
**File + line range:** internal/cli/status.go:L258-L268 (+ L352-L365)
**Current LOC touched:** 20
**Smell:** Addon health is encoded as bare "healthy"/"degraded"/"not enabled" at two sites. The same vocabulary is already typed via `okd.AddonStatus.Healthy`.
**Evidence:**
```go
addonHealth := "healthy"
if !a.Healthy { addonHealth = "degraded" }
sb.kv(a.Name, addonHealth)

// runDescribeAddon, status.go:355
if r.Err == nil { health = "healthy" } else { health = "degraded: " + r.Err.Error() }
```
**Fix — preferred:** refactor — add `func (a AddonStatus) Label() string` to internal/distribution/okd/types.go, replace both call sites with a.Label().
**Rule source:** Uber §Stringly Typed.
**Adjacent linter:** goconst (enabled).
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the operator-visible status output strings.
**Estimated net LOC delta:** -3
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** medium.
**CLAUDE.md / MEMORY.md conflict?:** no.

### smell:d9f7733e:bundle-category-magic-string-pair

**Cluster:** magic-strings
**File + line range:** internal/cli/debug_bundle.go:L29-L36
**Current LOC touched:** 8
**Smell:** Six `categoryX` constants are typed `string` and used as `manifestEntry.Name` slot keys. Adjacent `bundleStatus` is typed; `categoryX` should mirror that shape so a future entry is compile-checked.
**Evidence:**
```go
const (
    categoryMustGather     = "must-gather"
    categoryTerraformState = "terraform-state"
    categoryDoctor         = "doctor"
    ...
)

type manifestEntry struct {
    Name    string       `json:"name"`
    Status  bundleStatus `json:"status"`
}
```
**Fix — preferred:** refactor — `type bundleCategory string`, retag the 6 constants, change manifestEntry.Name to bundleCategory. JSON wire format unchanged.
**Rule source:** repo-counter-example: cli/debug_bundle.go:L88-L94 (bundleStatus already follows this shape).
**Adjacent linter:** none.
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the manifest.yaml wire format inside the bundle.
**Estimated net LOC delta:** +1
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** high.
**CLAUDE.md / MEMORY.md conflict?:** no.

### smell:8154ab0f:doctor-severity-string-roundtrip

**Cluster:** magic-strings
**File + line range:** internal/cli/doctor.go:L87-L98
**Current LOC touched:** 12
**Smell:** `severity` is a typed iota with 3 values; `sevString` switches on it and returns "ok"/"warn"/"fail"/"unknown". Stringer interface unused.
**Evidence:**
```go
func sevString(s severity) string {
    switch s {
    case sevPass: return "ok"
    case sevWarn: return "warn"
    case sevFail: return "fail"
    default: return "unknown"
    }
}
```
**Fix — preferred:** refactor — add `func (s severity) String() string`, drop sevString, severityMarkers can derive bracketed label by .String() composition.
**Rule source:** Uber §Strings.
**Adjacent linter:** none.
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the doctor JSON output's "severity" values.
**Estimated net LOC delta:** -2
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** medium — Stringer is mostly cosmetic here.
**CLAUDE.md / MEMORY.md conflict?:** no.

### smell:6424733c:metrics-shutdown-timeout-magic

**Cluster:** magic-strings
**File + line range:** internal/cli/helpers.go:L228-L256
**Current LOC touched:** 12
**Smell:** Four hardcoded durations (5s, 10s, 60s, 5s) appear inline in HTTP server construction and shutdown. Per-timeout magic numbers are a frequent regression class.
**Evidence:**
```go
srv := &http.Server{
    ReadHeaderTimeout: 5 * time.Second,
    ReadTimeout:       10 * time.Second,
    WriteTimeout:      10 * time.Second,
    IdleTimeout:       60 * time.Second,
}
...
shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
```
**Fix — preferred:** refactor — lift to package-level constants `metricsReadHeaderTimeout`, `metricsReadTimeout`, `metricsIdleTimeout`, `metricsShutdownTimeout`.
**Rule source:** Uber §Constants Above Magic Numbers.
**Adjacent linter:** none.
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** behavior — same durations.
**Estimated net LOC delta:** +4
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** medium.
**CLAUDE.md / MEMORY.md conflict?:** no.

### smell:5013fea6:auth-error-string-sniff

**Cluster:** magic-strings
**File + line range:** internal/distribution/okd/setup/release_extract.go:L92-L141
**Current LOC touched:** 50
**Smell:** `isAuthError` classifies registry failures by lowercase substring search of `oc` stderr. Error-string sniffing is the canonical un-idiomatic pattern — drift in the registry's error envelope silently downgrades a credential failure to a generic ClusterError.
**Evidence:**
```go
var authMarkers = []string{
    "unauthorized", "authentication", "denied",
    "forbidden", "no basic auth", "401", "403",
}
...
if (result.ExitCode == 1 || result.ExitCode == 125) && isAuthError(msg) {
    return &errtypes.AuthError{...}
}
```
**Fix — preferred:** refactor — try `oc adm release extract --output=json` and parse the typed envelope; if no structured signal is available, document this site as acknowledged tech debt with a TODO linking the upstream openshift/oc issue once filed.
**Rule source:** Uber §Errors ("do not match against an error's text"); Go proverb (clear is better than clever); CLAUDE.md §concurrency notes (the install-monitor CSR loop uses similar string-tracking but only for log dedup, not control flow).
**Adjacent linter:** none.
**Scaffolding?:** no
**Seam:** audit-errors — wrap-chain identity question is errors' territory; the body's string-sniffing pattern is what this audit catches.
**What MUST stay bit-for-bit:** the AuthError → exit code 5 mapping.
**Estimated net LOC delta:** 0 (when documented) / +20 (when restructured).
**Severity:** suggestion
**Risk (of applying fix):** medium — restructuring may regress on real-world registry stderr that currently passes.
**Confidence (in finding):** medium — ack'd by repo via the existing comment "Best-effort — a registry whose error envelope drifts from these patterns will fall through".
**CLAUDE.md / MEMORY.md conflict?:** no.

### smell:daf5bee9:any-yaml-traversal

**Cluster:** interfaceany-lazy
**File + line range:** internal/cli/kubeconfig.go:L141-L175
**Current LOC touched:** 36
**Smell:** `namedEntries` and `mergeNamedList` walk an unmarshalled kubeconfig as nested `any`. The kubeconfig schema is small and stable; a typed shape would let the merge avoid four type assertions per call.
**Evidence:**
```go
func namedEntries(v any) map[string]any {
    items, _ := v.([]any)
    result := make(map[string]any, len(items))
    for _, item := range items {
        if m, ok := item.(map[string]any); ok {
            if name, ok := m["name"].(string); ok {
                result[name] = item
            }
        }
    }
    return result
}
```
**Fix — preferred:** refactor — define a private struct shape carrying Name + raw json.RawMessage, unmarshal both kubeconfigs into typed envelopes.
**Rule source:** Go proverb ("interface{} says nothing"); Uber §Function Names (verbs return typed).
**Adjacent linter:** none.
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the round-trip yaml of unknown fields (extra keys must survive).
**Estimated net LOC delta:** +10
**Severity:** suggestion
**Risk (of applying fix):** medium — yaml round-trip fidelity tests recommended.
**Confidence (in finding):** medium.
**CLAUDE.md / MEMORY.md conflict?:** no.

### smell:92553fff:summary-hardcoded-3state-fmt

**Cluster:** magic-strings
**File + line range:** internal/cli/summary.go:L177 (+ L218)
**Current LOC touched:** 4
**Smell:** `fmt.Sprintf("%-4s  %s", displayStatus(&s), d)` — the width 4 is the longest current value of stepDisplayStatus. A fourth state would silently cause column drift.
**Evidence:**
```go
sb.kv(string(s.StepID), fmt.Sprintf("%-4s  %s", displayStatus(&s), d))
```
**Fix — preferred:** refactor — `const stepStatusColWidth = 4` doc-commented in summary.go, or use tabwriter (used elsewhere in the package).
**Rule source:** Go proverb (clear is better than clever).
**Adjacent linter:** none.
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** column alignment of the deploy-summary table.
**Estimated net LOC delta:** +2
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** high.
**CLAUDE.md / MEMORY.md conflict?:** no.

### smell:39c75e91:yes-no-magic-strings

**Cluster:** magic-strings
**File + line range:** internal/cli/confirm.go:L60-L62
**Current LOC touched:** 3
**Smell:** `isConfirmResponse` encodes the Y/yes/y truthy set with three string literals. Case-folded match would be one literal.
**Evidence:**
```go
func isConfirmResponse(response string) bool {
    return response == "y" || response == "Y" || response == "yes"
}
```
**Fix — preferred:** refactor — `strings.EqualFold(response, "y") || strings.EqualFold(response, "yes")`.
**Rule source:** Uber §Stringly Typed; Go proverb (clear is better than clever).
**Adjacent linter:** none.
**Scaffolding?:** no
**Seam:** none — the parallel parser in tui/wizard/datadriven.go is out of audit scope per global exclusions.
**What MUST stay bit-for-bit:** the y/yes affirmative semantics.
**Estimated net LOC delta:** -1
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** medium.
**CLAUDE.md / MEMORY.md conflict?:** no.

## Scaffolding items detected

None this run. Two prior-run scaffolding rows (smell:262af6e4 cleanup pipeline, smell:c4182b1c PhaseContext, smell:2be6306e IsRegistered) were not re-flagged in this catch-all sweep — the cleanup pipeline already moved to StepDef + BuildSteps in subsequent commits, and PhaseContext/IsRegistered are explicitly documented as scaffolding in their package comments.

## Linter-config-bug candidates

These rows have `adjacent_linter_enabled: true` but the linter did not fire on the listed sites:

- **smell:e7db1220** — dupl threshold 200 misses a 16-line cross-file duplicate. Consider lowering for the cli package only.
- **smell:fd2125dd**, **smell:b5a79fda**, **smell:26a430ee**, **smell:0f076161**, **smell:d31d1b9d** — goconst.min-occurrences is 3, which should fire on most of these. The likely cause is that goconst counts string-typed *literals*, not flag-key strings whose registration site is the source of truth. Verify by running `golangci-lint run --enable goconst --disable-all` once a constant is introduced.

To refresh linter-config-bugs.jsonl, run the aggregation command or `/audit-all`.

## Skip list

Findings considered and dropped:

- **arrow-pyramid in addon Manager.InstallAll / InstallOne** — owned by `audit-concurrency` (errgroup vs WaitGroup convention).
- **string-sniff on err.Error() in install/monitor.go CSR loop** — log dedup, not control flow; CLAUDE.md §concurrency calls this out as the canonical poll-loop log-once shape.
- **`any` in slog group walks (logutil/redact.go)** — slog's interface contract requires it; canonical helper.
- **PhaseContext genericity** — explicit scaffolding comment names symmetric-API intent (CLAUDE.md §architecture-notes "BuildSteps + StepDef").
- **Helper packages internal/runlock, internal/sshpin, internal/cluster, internal/netutil, internal/httputil** — each has narrow scope, real use, and no dual-library overlap. None qualify as helper-package-no-value.

## Cluster verdicts

- **magic-strings** (13 findings) — the dominant cluster. The repo has a strong pattern (typed enums in phase/, cleanup/, summary, debug_bundle, config) but the cli package has many half-applied conversions. Two cli-flags fixes + the deploy-phase fix would bring the cli package into alignment.
- **helper-package-no-value** (2) — neither finding is severe; cli/flags.go is half-applied and the duplicate-enum-label is a 13-LOC cleanup with a method.
- **interfaceany-lazy** (1) — kubeconfig merge is the only `any`-soup site of consequence.
- **stringified-numbers, bool-should-be-3state, multi-bool-exclusive, arrow-pyramid, helper-pkg-thin-wrap** — no rule-fitting findings. arrow patterns and bool fields in DestroyOpts / DeploymentConfig are independent flags; not mutually exclusive.

## Smell density by package (findings per 100 LOC, in-scope code only)

| Package | LOC | Findings | Density |
|---|---|---|---|
| internal/cli | 4659 | 12 | 0.26 |
| internal/distribution/okd/postinstall | ~700 | 1 | 0.14 |
| internal/distribution/okd/setup | ~1800 | 1 | 0.06 |
| internal/distribution/okd/destroy | ~250 | 1 | 0.40 |
| internal/distribution/okd/releases | ~250 | 1 | 0.40 |
| internal/infrastructure/proxmox | ~600 | 1 | 0.17 |

Verdict: cli/ has the highest absolute count and density that warrants attention; the other elevated-density packages have small denominators so the ratios are noisy.

## Scope exceptions proposed

None. The audit respected the global out-of-scope list (tui/wizard/, templates/, iso.go, linux-build-tagged code, vendor/).

## Footer

Total findings: 18 (blocker: 0, major: 0, minor: 7, suggestion: 11)
Scope coverage: in-scope `internal/**` packages walked package-by-package; the cli/* and distribution/okd/{postinstall,destroy,releases,setup,phase}/* tree was read in full or via grep + targeted reads; out-of-scope dirs (tui/wizard/, templates/, iso.go, *_test.go, linux-build-tagged) were not opened.
Seam deferrals: 4 (smell:5013fea6 → audit-errors; smell:1d5afa08 → audit-errors; smell:d6b325cb → audit-api-design; smell:0f076161 → audit-cli-ux).
MEMORY.md: absent for this operator (none required as per AUDIT_CONVENTIONS §0.2).

To refresh linter-config-bugs.jsonl, run the aggregation command or `/audit-all`.

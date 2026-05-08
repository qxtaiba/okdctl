# audit-dependencies — 2026-05-08

**Assumes green:** golangci-lint, govulncheck, CodeQL, shellcheck, tflint, go test ./...
**Scope:** every direct + indirect entry in `go.mod` / `go.sum`; every action pin in `.github/workflows/*.yml`; tool installs in `Makefile` + CI workflows; license / maintenance / pin-stability / duplicate-engine / transitive-weight / justified-version-floor.
**Out of scope this run:** in-binary CVE matching (handled by govulncheck job), go-version migration suggestions (seam → audit-modernization).
**Seam co-owners:** `audit-modernization` for "drop dep in favor of stdlib" findings (none emitted this run — see seams.md §10).

## Executive summary

Tree is clean. govulncheck CI job is green; all direct deps ship under permissive licenses (MIT / Apache-2.0 / BSD); every GitHub Action is SHA-pinned with a `# vX.Y.Z` trailer; every `go install` and `goreleaser`/`tflint`/`terraform` invocation is pinned to an explicit version. Two minor findings: (1) the YAML-engine tripwire in CLAUDE.md is stale — `gopkg.in/yaml.v3` is back in `go.sum` via testify→check.v1, so the engine count is four not three; (2) `make lint` runs golangci-lint v2.12.1 while CI runs v2.12.2. Eight suggestion-level rows re-confirm pre-known cases (go-proxmox bus-factor, gorilla/websocket transitive-only, k8s.io/* pseudoversion floor, four-log-engine stack from Charm + k8s) and document the `golang.org/x/exp` 2023 floor that MVS hasn't lifted. No blockers, no majors.

## Ranked table

| ID | Cluster | File:line | Severity | Confidence | LOC | Adjacent linter | Fix class |
|----|---------|-----------|----------|------------|-----|-----------------|-----------|
| dep:33ef32bf:yaml-quad-engines | duplicate-engine | go.sum:125-153 | minor | high | 0 | none | policy |
| dep:b803fcb7:golangci-version-drift | pin-stability | .github/workflows/ci.yml:19-21 | minor | high | 1 | none | config |
| dep:6ebdb617:claudemd-yaml-tripwire-stale | duplicate-engine | CLAUDE.md:196-201 | suggestion | high | 6 | none | policy |
| dep:33ef32bf:proxmox-version-drift | maintenance-signal | go.mod:12 | suggestion | high | 2 | none | policy |
| dep:33ef32bf:proxmox-bus-factor-reconfirm | maintenance-signal | go.mod:12 | suggestion | high | 0 | none | library-swap |
| dep:33ef32bf:gorilla-websocket-stale | maintenance-signal | go.sum:61-62 | suggestion | high | 0 | none | policy |
| dep:33ef32bf:dup-log-engines-stack | duplicate-engine | go.mod:1-69 | suggestion | high | 0 | none | policy |
| dep:33ef32bf:k8s-pseudoversion-floor | justified-version-floor | go.mod:64-66 | suggestion | high | 0 | none | policy |
| dep:33ef32bf:exp-floor-stale-pseudoversion | justified-version-floor | go.mod:57 | suggestion | medium | 1 | none | config |
| dep:3295df72:transitive-test-deps-from-proxmox | transitive-weight | go.sum:54-67 | suggestion | high | 0 | none | policy |

## Direct dep scorecard

| name | version | license | usage (in repo) | notes |
|------|---------|---------|-----------------|-------|
| charm.land/bubbles/v2 | v2.1.0 | MIT | TUI wizard (≥20 sites) | intentional UI stack |
| charm.land/bubbletea/v2 | v2.0.6 | MIT | TUI run loop | intentional UI stack |
| charm.land/lipgloss/v2 | v2.0.3 | MIT | TUI styling | intentional UI stack |
| charm.land/log/v2 | v2.0.0 | MIT | internal/tui/logger.go | intentional UI stack |
| github.com/luthermonson/go-proxmox | v0.5.0 | Apache-2.0 | proxmox_discovery.go (sole) | v0.x, bus-factor 1; abandonment plan documented |
| github.com/spf13/cobra | v1.10.2 | Apache-2.0 | internal/cli/* | canonical CLI library |
| golang.org/x/crypto | v0.50.0 | BSD-3-Clause | sshpin | well-maintained Go subrepo |
| golang.org/x/mod | v0.35.0 | BSD-3-Clause | releases/fetcher, version/updatecheck | semver parsing |
| golang.org/x/term | v0.43.0 | BSD-3-Clause | tui/wizard, cli/logging, download/progress | TTY detection |
| k8s.io/api | v0.36.0 | Apache-2.0 | addon/helpers, cli/status | k8s-tracking; release decision to bump |
| k8s.io/apimachinery | v0.36.0 | Apache-2.0 | addon, download/retry | k8s-tracking; bump in lockstep |
| sigs.k8s.io/yaml | v1.6.0 | Apache-2.0 + MIT | 12 sites (config, addon, cli) | canonical YAML for k8s manifests |

All licenses permissive. No copyleft anywhere in the direct dep set.

## Duplicate-engine summary

| category | engines in tree | site count |
|----------|-----------------|------------|
| YAML | sigs.k8s.io/yaml (direct), go.yaml.in/yaml/v2, go.yaml.in/yaml/v3, gopkg.in/yaml.v3 | 4 |
| log | log/slog (stdlib), charm.land/log/v2 (direct), go-logr/logr (transitive), k8s.io/klog/v2 (transitive) | 4 |
| HTTP | net/http (stdlib) only | 1 |
| websocket | gorilla/websocket (transitive, unreached) | 0 reached |

YAML count is four, not three. Both YAML and log dup-counts are structural (k8s tree + Charm tree); neither is collapsible without dropping a direct dep family. Document and accept.

## Pin audit (workflows + tooling)

| Surface | Pinning |
|---------|---------|
| `.github/workflows/ci.yml` actions (10 SHAs) | all pinned by SHA + `# vX.Y.Z` trailer |
| `.github/workflows/release.yml` actions | all pinned by SHA + trailer (incl. cosign-installer, sbom-action, slsa-github-generator) |
| `.github/workflows/release-prep.yml` | all SHA-pinned |
| `.github/workflows/codeql.yml` | all SHA-pinned |
| `.github/workflows/labeler.yaml` / `label-sync.yaml` | all SHA-pinned |
| `terraform_version` (ci.yml:91) | "1.10.3" — patch-pinned |
| `golangci-lint-action` version arg (ci.yml:21) | v2.12.2 |
| `goreleaser-action` version arg (release.yml:28, release-prep.yml:31) | v2.15.2 |
| `go install golang.org/x/vuln/cmd/govulncheck` (ci.yml:56) | @v1.1.4 |
| `go install github.com/google/yamlfmt/cmd/yamlfmt` (ci.yml:82) | @v0.14.0 |
| Makefile `go install golangci-lint` (line 80) | @v2.12.1 — DRIFT vs CI v2.12.2 |
| Makefile `go install air` (line 60) | @v1.61.7 |

One drift: golangci-lint local (v2.12.1) vs CI (v2.12.2). All other surfaces clean.

## Findings

**ID:** dep:33ef32bf:yaml-quad-engines
**Cluster:** duplicate-engine
**File + line range:** go.sum:125-153
**Smell:** go.sum still lists four YAML engines: `sigs.k8s.io/yaml v1.6.0` (direct), `go.yaml.in/yaml/v2 v2.4.3` (transitive), `go.yaml.in/yaml/v3 v3.0.4` (transitive), and `gopkg.in/yaml.v3 v3.0.1` (transitive via testify→check.v1). CLAUDE.md tripwire claims the count is "down from four" to three.
**Evidence:**
```
go.sum:125 go.yaml.in/yaml/v2 v2.4.3
go.sum:127 go.yaml.in/yaml/v3 v3.0.4
go.sum:152 gopkg.in/yaml.v3 v3.0.1
go.sum:170 sigs.k8s.io/yaml v1.6.0
```
**Fix — preferred:** policy. Update CLAUDE.md tripwire to "four YAML engines" or actually drop testify (zero call sites in okdctl) so the count truly becomes three.
**Rule source:** CLAUDE.md §dependencies; SKILL.md §5a known cases.
**Adjacent linter:** none
**Scaffolding?:** no
**Severity:** minor — re-confirmation of an already-flagged case per §5a, but the tripwire is functionally broken (it documents a count that doesn't match reality, so a future fifth engine would slip past).
**Risk:** low
**Confidence:** high

---

**ID:** dep:b803fcb7:golangci-version-drift
**Cluster:** pin-stability
**File + line range:** `.github/workflows/ci.yml:19-21` + `Makefile:80`
**Smell:** Makefile installs golangci-lint v2.12.1; ci.yml runs v2.12.2. Local `make lint` and CI lint-go run different linter versions, opening a "passes locally, fails in CI" (or vice versa) wedge. The lefthook pre-commit doesn't run golangci-lint (only gofumpt + go vet), so the local feedback loop relies on `make lint`.
**Evidence:**
```
Makefile:80  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.1
.github/workflows/ci.yml:21      version: v2.12.2
```
**Fix — preferred:** config. Sync the Makefile pin to v2.12.2; better, extract a single `GOLANGCI_LINT_VERSION` source consumed by both.
**Rule source:** CLAUDE.md §dependencies — "Tool installs from Go must be explicit versions — never @latest"; CLAUDE.md §tooling.
**Adjacent linter:** none
**Scaffolding?:** no
**Severity:** minor
**Risk:** low
**Confidence:** high

---

**ID:** dep:6ebdb617:claudemd-yaml-tripwire-stale
**Cluster:** duplicate-engine
**File + line range:** `CLAUDE.md:196-201`
**Smell:** Tripwire text says "Count is down from four — gopkg.in/yaml.v3 was dropped from go.mod require." Technically true (not in require block) but the engine is still in go.sum via testify; the tripwire fires only on a fourth direct add and won't catch the actual fourth engine that's already there.
**Fix — preferred:** policy. Edit the comment to match reality, or actually drop testify.
**Rule source:** CLAUDE.md §dependencies; counter-example: go.sum:152.
**Severity:** suggestion
**Confidence:** high
**Risk:** low

---

**ID:** dep:33ef32bf:proxmox-version-drift
**Cluster:** maintenance-signal
**File + line range:** `go.mod:12`
**Smell:** CLAUDE.md says "v0.4.x" but go.mod has v0.5.0. Bus-factor and abandonment-plan claims still apply; only the version sticker is stale.
**Fix — preferred:** policy. Update CLAUDE.md to "v0.5.x".
**Rule source:** CLAUDE.md §dependencies; SKILL.md §5a.
**Severity:** suggestion (re-confirmation, not novel)
**Confidence:** high
**Risk:** low

---

**ID:** dep:33ef32bf:proxmox-bus-factor-reconfirm
**Cluster:** maintenance-signal
**File + line range:** `go.mod:12` + `internal/tui/wizard/steps/proxmox_discovery.go:11-176`
**Smell:** v0.5.0, single maintainer, single 203-LOC call site (five method calls: `client.Nodes`, `client.Node`, `Storages`, `Networks`, `GetContent`). Pulls heavy transitive set (websocket, mage, copier, goterm, gock, parth) for narrow REST usage. Re-confirmation per §5a — abandonment plan documented; do not propose rip-out.
**Rule source:** CLAUDE.md §dependencies; SKILL.md §5a + §5.
**Severity:** suggestion
**Confidence:** high
**Risk:** high (a swap risks breaking discovery; deferred to roadmap)

---

**ID:** dep:33ef32bf:gorilla-websocket-stale
**Cluster:** maintenance-signal
**File + line range:** `go.sum:61-62`
**Smell:** v1.4.2 (2020-04, > 5 years old); transitive via go-proxmox; zero okdctl call sites. CLAUDE.md §5a says: keep until go-proxmox migrates. Re-confirmation.
**Rule source:** CLAUDE.md §dependencies; SKILL.md §5a.
**Severity:** suggestion
**Confidence:** high
**Risk:** low

---

**ID:** dep:33ef32bf:dup-log-engines-stack
**Cluster:** duplicate-engine
**File + line range:** `go.mod:1-69`
**Smell:** Four log engines (slog, charmlog, logr, klog) coexist; consolidation impossible because charmlog is intentional UI stack and logr/klog are k8s baseline. Steady state.
**Rule source:** SKILL.md §5 must-preserve; CLAUDE.md §dependencies.
**Severity:** suggestion
**Confidence:** high
**Risk:** high (any consolidation ships a UX/k8s breakage)

---

**ID:** dep:33ef32bf:k8s-pseudoversion-floor
**Cluster:** justified-version-floor
**File + line range:** `go.mod:64-66`
**Smell:** k8s.io/kube-openapi, k8s.io/utils, sigs.k8s.io/json all pinned to commit-hash pseudoversions (upstream norm). Bump in lockstep with k8s.io/api or risk mixed-vintage trees.
**Fix — preferred:** policy. Add a release-prep checklist note: bump kube-openapi/utils/json from k8s.io/api's go.sum on each k8s bump.
**Rule source:** CLAUDE.md §dependencies; SKILL.md §5 must-preserve.
**Severity:** suggestion
**Confidence:** high
**Risk:** low

---

**ID:** dep:33ef32bf:exp-floor-stale-pseudoversion
**Cluster:** justified-version-floor
**File + line range:** `go.mod:57`
**Smell:** golang.org/x/exp pinned to v0.0.0-20231006140011 (2023-10), > 18 months old; transitive-only. MVS hasn't lifted because no consumer demands a newer commit.
**Fix — preferred:** config. `go get golang.org/x/exp@latest && go mod tidy`. Optional — purely cosmetic since no okdctl code reaches x/exp.
**Rule source:** CLAUDE.md §dependencies; SKILL.md §6.
**Severity:** suggestion
**Confidence:** medium (no reachability so blast radius is zero)
**Risk:** low

---

**ID:** dep:3295df72:transitive-test-deps-from-proxmox
**Cluster:** transitive-weight
**File + line range:** `go.sum:54-67` (go-test/deep, h2non/gock, h2non/parth) + `go.sum:147-149` (gopkg.in/check.v1)
**Smell:** Test-only deps from go-proxmox's test imports; not in okdctl's require block; zero okdctl call sites; do not enter the release binary.
**Fix — preferred:** policy. Document as transitive-test-only acceptance.
**Severity:** suggestion
**Confidence:** high
**Risk:** low

## Scaffolding items detected

None. Scaffolding rule (MEMORY.md §scaffolding) does not apply to dep audit findings.

## Linter-config-bug candidates

None. No dep-audit finding has an enabled linter that should have caught it.

## Skip list

- **Cobra v1.10.2** — direct dep, license Apache-2.0, well-maintained; nothing to flag.
- **golang.org/x/{crypto,mod,term,net,sync,sys,text}** — all on current Go subrepo cadence (v0.x but Google-maintained). Skip.
- **k8s.io/{api,apimachinery}** — must-preserve per SKILL §5; release-decision to bump.
- **All charm.land/* packages** — must-preserve per SKILL §5 (intentional UI stack); never propose swapping for cobra builtins.
- **fxamacker/cbor v2.9.0, modern-go/{concurrent,reflect2}, jinzhu/copier, x448/float16, ulikunitz/xz, klauspost/compress** — all transitive via go-proxmox / k8s. Out of scope at the consumer level; bump arrives via dep updates.

## Cluster verdicts

- **license-compat** — clean. All direct deps permissive (MIT / Apache-2.0 / BSD). The `joho/godotenv` LICENCE-spelling note in CLAUDE.md is moot here because godotenv is not in go.mod.
- **maintenance-signal** — go-proxmox v0.5.0 / gorilla/websocket v1.4.2 are both pre-flagged in CLAUDE.md §5a; re-confirmed without change. CLAUDE.md version sticker on go-proxmox is stale (says v0.4.x).
- **pin-stability** — all GitHub Actions SHA-pinned with version trailer; all go-tool installs are explicit versions; all `setup-terraform` / `setup-tflint` / `goreleaser` use explicit versions. ONE drift: `make lint` v2.12.1 vs CI v2.12.2.
- **duplicate-engine** — YAML count is four, not three; the tripwire is functionally broken. log count is four (steady state — Charm + k8s). HTTP is single-engine (stdlib). websocket is unreached.
- **transitive-weight** — go-proxmox is the heaviest direct dep for the narrowest okdctl footprint (203 LOC, REST-only). Documented fallback exists; do not act this run.
- **justified-version-floor** — golang.org/x/exp 2023 floor is cosmetically old; k8s.io/* pseudoversions tied to k8s.io/api bump cadence.

## Scope exceptions proposed

None — all in-scope files were read in full or grepped exhaustively.

## Footer

Total findings: 10 (blocker: 0, major: 0, minor: 2, suggestion: 8)
Scope coverage: go.mod (1/1), go.sum (1/1, 171 lines), 6/6 workflow files, Makefile, lefthook.yml, install.sh, .goreleaser.yaml, .golangci.yml, .github/coverage-floors.conf, .github/renovate.json5, CLAUDE.md §dependencies. 100% direct in-full; transitive set scanned via grep + go.sum line analysis (no per-transitive websearch — score uses CLAUDE.md and §5a known-cases as ground truth).
Seam deferrals: 0 (no findings whose fix is "delete the import line in favor of stdlib" — that would be `audit-modernization`).

To refresh `linter-config-bugs.jsonl`, run `jq -c 'select(.adjacent_linter_enabled==true)' .claude/audits/*.jsonl > .claude/audits/linter-config-bugs.jsonl` or invoke `/audit-all`.

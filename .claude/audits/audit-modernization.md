# audit-modernization — 2026-05-08

**Assumes green:** golangci-lint (errcheck, govet, staticcheck, ineffassign, unused, gocritic, revive, unparam, nilerr, noctx, gosec), govulncheck, shellcheck, tflint, `go test ./...`.

**Scope:** `internal/**/*.go` and `cmd/**/*.go` (excluding wizard, templates, `setup/iso.go`, `*_test.go`, and `//go:build linux` files per AUDIT_CONVENTIONS §2). go.mod toolchain is Go 1.26.0 / 1.26.3, so every Go 1.21–1.25 stdlib feature is fair game.

**Out of scope this run:** wizard packages (`internal/tui/wizard/**`), `internal/distribution/okd/templates/**`, `internal/distribution/okd/setup/iso.go`, `_test.go` files, `//go:build linux` files (`internal/cli/doctor.go`, `internal/cli/debug_bundle_doctor.go`). `doctor.go` referenced once for cross-pattern context but not landed as in-scope finding (mod:8154ab0f de-duplicated against the in-scope twin in `setup/steps.go`).

**Seam co-owners:** none active this run. Modernization owns Go 1.22+ stdlib moves (slices, maps, errors.Join, log/slog, net/netip, range-over-int, range-over-func, min/max/clear, any over interface{}, strings.Lines). Where a finding could equally land in code-smells (e.g. `map[string]bool` vs `map[string]struct{}`), I deferred per seam #7 since the fix is not stdlib-version-keyed.

## Executive summary

okdctl is already modernization-mature. There is **no `interface{}`** in the audited tree, **no `ioutil`** anywhere, **no `golang.org/x/exp` imports**, **no `net.IP` / `net.ParseIP` / `net.ParseCIDR`** (every IP/CIDR codepath uses `net/netip` already), and **no counted `for i := 0; i < N; i++` loops** outside of `_test.go` and the wizard. `slices`, `maps`, `errors.Join`, `errors.Is`, `cmp.Or`, `clear`, `min`/`max`, `slices.SortFunc`/`Sorted`/`Concat`/`Backward`/`ContainsFunc`/`DeleteFunc`/`IndexFunc`/`Max`, `sync.OnceValue`/`OnceValues`/`OnceFunc`, `strings.Lines`, range-over-int, and `errors.Join` are all in active production use.

The 13 findings here are mostly **suggestions** (style consistency) plus 3 **minor** sites where a hand-rolled `slices.Contains` loop survived in code that pre-dates the migration wave — the rest of the repo already uses `slices.Contains` for the same pattern (`elevation.go::requiresRoot`, `validators.go::isValidDistribution`). Land all three in one PR; total LOC delta is roughly -25 LOC across ~10 sites.

The strongest cluster is **`strings.Lines` consistency**: the repo has 8+ sites using the Go 1.24 iterator and 4 lingering `strings.Split(x, "\n")` sites. A single-PR sweep brings the file count to one idiom and saves a handful of allocations on hot paths (checksum.go reads response bodies, tools.go runs every preflight). No `blocker` or `major` findings — release is not gated by modernization debt.

## Ranked table

Sorted by severity_weight × confidence × |LOC delta| ÷ risk (blocker=4, major=3, minor=2, suggestion=1; high=3 / med=2 / low=1).

| ID | Cluster | File:line | Severity | Confidence | LOC | Adjacent linter | Fix class |
|---|---|---|---|---|---|---|---|
| mod:262af6e4:use-slices-contains | slices-maps | cleanup/cleanup.go:L52-L60 | minor | high | -6 | gocritic (enabled) | stdlib |
| mod:15ba17da:use-slices-contains | slices-maps | destroy/steps.go:L60-L69 | minor | high | -5 | gocritic (enabled) | stdlib |
| mod:ddf885f4:use-slices-contains | slices-maps | addon/manager.go:L267-L275 | minor | high | 0 | gocritic (enabled) | stdlib |
| mod:c19ee328:slices-containsfunc-allexist | slices-maps | setup/steps.go:L85-L93 | suggestion | high | -3 | gocritic (enabled) | stdlib |
| mod:c19ee328:slices-containsfunc-ignition | slices-maps | setup/steps.go:L186-L192 | suggestion | high | -3 | gocritic (enabled) | stdlib |
| mod:eb479d86:use-slices-containsfunc | slices-maps | setup/upload.go:L171-L176 | suggestion | high | -3 | gocritic (enabled) | stdlib |
| mod:8ea706f6:strings-lines-version | slices-maps | setup/tools.go:L260-L264 | suggestion | high | -3 | none | stdlib |
| mod:8ea706f6:strings-lines-fingerprint | slices-maps | setup/tools.go:L346-L362 | suggestion | high | 0 | none | stdlib |
| mod:5e892064:strings-lines-checksum | slices-maps | download/checksum.go:L83-L84 | suggestion | high | -1 | none | stdlib |
| mod:983f67f0:strings-lines-mixed | slices-maps | tui/layouts.go:L28-L113 | suggestion | high | 0 | none | stdlib |
| mod:6b533f2d:slices-collect-projection | slices-maps | cluster/k8s_csrs.go:L63-L66 | suggestion | medium | -2 | none | refactor |
| mod:b8687976:slices-clip-dedup | slices-maps | releases/fetcher.go:L101-L113 | suggestion | low | -7 | none | stdlib |
| mod:c87d0b1f:log-fatal-in-cli-entrypoint | slog-over-3p | cmd/okdctl-gen-docs/main.go:L18-L35 | suggestion | medium | +2 | none | refactor |

## Findings

### mod:262af6e4:use-slices-contains

**ID:** mod:262af6e4:use-slices-contains
**Cluster:** slices-maps
**File:** internal/distribution/okd/cleanup/cleanup.go:L52-L60
**Current LOC touched:** 9
**Smell:** Kind.IsValid hand-rolls a contains loop over ValidKinds() — the textbook `slices.Contains` use case. Twin to mod:15ba17da and mod:ddf885f4. Already-migrated equivalents: `internal/cli/elevation.go:L54-L58` (rootRequiredCmds) and `internal/config/validators.go:L409-L415` (isValidDistribution / isValidProvider).
**Evidence:**
```go
func (k Kind) IsValid() bool {
	for _, v := range ValidKinds() {
		if k == v {
			return true
		}
	}
	return false
}
```
**Fix — preferred:** `return slices.Contains(ValidKinds(), k)`. Kind is comparable; ValidKinds returns []Kind. Net -6 LOC.
**Rule source:** Go 1.21 stdlib: slices.Contains. Repo counter-examples: `internal/cli/elevation.go:L54-L58`, `internal/config/validators.go:L409-L415`.
**Adjacent linter:** gocritic (enabled — see Linter-config-bug section).
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** Kind comparison semantics (kind-level equality, not string-level).
**Estimated net LOC delta:** -6
**Severity:** minor
**Risk (of applying fix):** low — pure local rewrite, full test coverage in cleanup_test.go.
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** mod:15ba17da:use-slices-contains, mod:ddf885f4:use-slices-contains.

### mod:15ba17da:use-slices-contains

**ID:** mod:15ba17da:use-slices-contains
**Cluster:** slices-maps
**File:** internal/distribution/okd/destroy/steps.go:L60-L69
**Current LOC touched:** 10
**Smell:** destroyTracker.terraformFailed iterates t.failures looking for a literal string match. `slices.Contains(t.failures, "terraform destroy")` is one line.
**Evidence:**
```go
func (t *destroyTracker) terraformFailed() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, f := range t.failures {
		if f == "terraform destroy" {
			return true
		}
	}
	return false
}
```
**Fix — preferred:** Replace inner loop with `return slices.Contains(t.failures, "terraform destroy")`. Lock take/defer wrapping unchanged.
**Rule source:** Go 1.21 stdlib: slices.Contains. Repo counter-example: `internal/cli/elevation.go:L54-L58`.
**Adjacent linter:** gocritic (enabled).
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** RWMutex acquire/defer lifecycle.
**Estimated net LOC delta:** -5
**Severity:** minor
**Risk (of applying fix):** low.
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** mod:262af6e4:use-slices-contains.

### mod:ddf885f4:use-slices-contains

**ID:** mod:ddf885f4:use-slices-contains
**Cluster:** slices-maps
**File:** internal/addon/manager.go:L267-L275
**Current LOC touched:** 9
**Smell:** Manager.dependsOn does a hand-rolled equality check before recursing. Hoisting the equality short-circuit out of the loop separates the "is this the target?" question from the "do any of these transitively depend?" question. Package already imports `slices` (L8) for `slices.Backward`, so the import is free.
**Evidence:**
```go
for _, dep := range a.Info().Dependencies {
	if dep == target {
		return true
	}
	if m.dependsOn(dep, target, visited) {
		return true
	}
}
```
**Fix — preferred:** `if slices.Contains(a.Info().Dependencies, target) { return true }; for _, dep := range a.Info().Dependencies { if m.dependsOn(dep, target, visited) { return true } }`. Two passes, both short-circuit; the equality pass is O(N) cache-friendly and likely faster on small dep lists than the interleaved version.
**Rule source:** Go 1.21 stdlib: slices.Contains. Repo counter-example: `internal/cli/elevation.go:L54-L58`.
**Adjacent linter:** gocritic (enabled).
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the visited-map cycle guard semantics; the visit ordering inside `dependsOn` is observable through the recursion contract.
**Estimated net LOC delta:** 0
**Severity:** minor
**Risk (of applying fix):** low.
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** mod:262af6e4:use-slices-contains.

### mod:eb479d86:use-slices-containsfunc

**ID:** mod:eb479d86:use-slices-containsfunc
**Cluster:** slices-maps
**File:** internal/distribution/okd/setup/upload.go:L171-L176
**Current LOC touched:** 6
**Smell:** isoUploadAlreadyDone iterates isoFiles short-circuiting on the first 'needs upload' result. `slices.ContainsFunc` expresses any-true-then-stop in one line and matches the repo's prevailing pattern (verify.go, status.go, executor.go).
**Evidence:**
```go
for _, f := range isoFiles {
	if isoUploadNeeded(ctx, p.Exec, host, knownHostsPath, remotePath, f) {
		return false, nil
	}
}
return true, nil
```
**Fix — preferred:** `if slices.ContainsFunc(isoFiles, func(f string) bool { return isoUploadNeeded(ctx, p.Exec, host, knownHostsPath, remotePath, f) }) { return false, nil }; return true, nil`.
**Rule source:** Go 1.21 stdlib: slices.ContainsFunc. Repo counter-examples: `internal/distribution/okd/postinstall/verify.go:L54`, `internal/cli/status.go:L111`.
**Adjacent linter:** gocritic (enabled).
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** short-circuit ordering of the SSH probes (matters because each call may incur network latency).
**Estimated net LOC delta:** -3
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no

### mod:983f67f0:strings-lines-mixed

**ID:** mod:983f67f0:strings-lines-mixed
**Cluster:** slices-maps
**File:** internal/tui/layouts.go:L28-L113 (two adjacent sites in the same function flow)
**Current LOC touched:** 4
**Smell:** Intra-file inconsistency. `maxLineWidth` (L30) uses `strings.Lines` (Go 1.24); `boxedSectionCore` (L44) uses `strings.Split(content, "\n")` over the same content variable. The Split form materialises a slice; on tight terminal redraws the iterator avoids the allocation.
**Evidence:**
```go
// L28-34 (iterator)
func maxLineWidth(content string) int {
	var m int
	for line := range strings.Lines(content) {
		m = max(m, lipgloss.Width(line))
	}
	return m
}
// L44 (split — same content var)
	lines := strings.Split(content, "\n")
```
**Fix — preferred:** Migrate L44 to `for line := range strings.Lines(content) { ... }`. Or, if a slice is needed for the contentRows pre-size, `lines := slices.Collect(strings.Lines(content))`.
**Rule source:** Go 1.24 stdlib: strings.Lines. Counter-examples: `internal/netutil/iface.go:L72`, `internal/distribution/okd/dns/dns.go:L153`, `internal/distribution/okd/install/flux.go:L36`, `internal/platform/platform.go:L85`.
**Adjacent linter:** none — no Go linter catches "mixed line-iteration idioms across the same file".
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the box rendering output (each line padded to innerWidth).
**Estimated net LOC delta:** 0
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no

### mod:8ea706f6:strings-lines-version

**ID:** mod:8ea706f6:strings-lines-version
**Cluster:** slices-maps
**File:** internal/distribution/okd/setup/tools.go:L260-L264
**Current LOC touched:** 5
**Smell:** `strings.Split(strings.TrimSpace(string(output)), "\n")` only to read the first line. With strings.Lines the read becomes a single-iteration loop with no allocation; the surrounding `if len(lines) > 0` guard goes away.
**Evidence:**
```go
lines := strings.Split(strings.TrimSpace(string(output)), "\n")
if len(lines) > 0 {
	return lines[0]
}
return "unknown"
```
**Fix — preferred:** `for line := range strings.Lines(string(output)) { return strings.TrimSpace(line) }; return "unknown"`. The TrimSpace on the loop variable handles the trailing newline strings.Lines preserves.
**Rule source:** Go 1.24 stdlib: strings.Lines. Counter-example: `internal/distribution/okd/install/flux.go:L36`.
**Adjacent linter:** none.
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** "unknown" sentinel on empty output.
**Estimated net LOC delta:** -3
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no

### mod:8ea706f6:strings-lines-fingerprint

**ID:** mod:8ea706f6:strings-lines-fingerprint
**Cluster:** slices-maps
**File:** internal/distribution/okd/setup/tools.go:L346-L362
**Current LOC touched:** 4
**Smell:** verifyHashiCorpGPGFingerprint uses `for _, line := range strings.Split(string(out), "\n")` to walk gpg --with-colons output. Same file already does this kind of split twice (L260 and here); both should land together.
**Evidence:**
```go
for _, line := range strings.Split(string(out), "\n") {
	if !strings.HasPrefix(line, "fpr:") {
		continue
	}
```
**Fix — preferred:** `for line := range strings.Lines(string(out)) { line = strings.TrimRight(line, "\n"); ... }`. Fingerprint comparison operates on a trimmed sub-field, unaffected by the per-line trailing newline.
**Rule source:** Go 1.24 stdlib: strings.Lines. Counter-example: `internal/netutil/iface.go:L72-L78` (same TrimRight idiom).
**Adjacent linter:** none.
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the canonical HashiCorp fingerprint match (security gate). The change is purely the line-splitter, not the comparison.
**Estimated net LOC delta:** 0
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** mod:8ea706f6:strings-lines-version.

### mod:5e892064:strings-lines-checksum

**ID:** mod:5e892064:strings-lines-checksum
**Cluster:** slices-maps
**File:** internal/download/checksum.go:L83-L84
**Current LOC touched:** 2
**Smell:** `strings.Split(string(body), "\n")` materialises a slice over an HTTP-response body. strings.Lines yields one line at a time with no slice allocation; on multi-arch checksum files the saving is proportional to body size.
**Evidence:**
```go
lines := strings.Split(string(body), "\n")
for _, line := range lines {
	line = strings.TrimSpace(line)
```
**Fix — preferred:** `for line := range strings.Lines(string(body)) { line = strings.TrimSpace(line); ... }`. TrimSpace already handles the per-line trailing newline.
**Rule source:** Go 1.24 stdlib: strings.Lines. Counter-example: `internal/distribution/okd/dns/dns.go:L153`.
**Adjacent linter:** none.
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the per-line filtering (skip-empty, skip-`#`-comment) and the checksum-format validation logic.
**Estimated net LOC delta:** -1
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no

### mod:c19ee328:slices-containsfunc-allexist

**ID:** mod:c19ee328:slices-containsfunc-allexist
**Cluster:** slices-maps
**File:** internal/distribution/okd/setup/steps.go:L85-L93
**Current LOC touched:** 9
**Smell:** AlreadyDone for the install-binaries step iterates `[openshift-install, oc, kubectl]` and returns false on the first missing entry. slices.ContainsFunc with negation expresses 'all-of' cleanly.
**Evidence:**
```go
AlreadyDone: func(_ context.Context) (bool, error) {
	binDir := phase.BinDirOrDefault(p.BinDir)
	for _, bin := range []string{"openshift-install", "oc", "kubectl"} {
		if !system.FileExists(filepath.Join(binDir, bin)) {
			return false, nil
		}
	}
	return true, nil
},
```
**Fix — preferred:** `bins := []string{"openshift-install", "oc", "kubectl"}; missing := slices.ContainsFunc(bins, func(b string) bool { return !system.FileExists(filepath.Join(binDir, b)) }); return !missing, nil`. Same short-circuit semantics.
**Rule source:** Go 1.21 stdlib: slices.ContainsFunc. Counter-example: `internal/cli/status.go:L189`.
**Adjacent linter:** gocritic (enabled).
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the AlreadyDone contract — false on the first missing file.
**Estimated net LOC delta:** -3
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no

### mod:c19ee328:slices-containsfunc-ignition

**ID:** mod:c19ee328:slices-containsfunc-ignition
**Cluster:** slices-maps
**File:** internal/distribution/okd/setup/steps.go:L186-L192
**Current LOC touched:** 7
**Smell:** AlreadyDone for StepGenerateIgnition repeats the same all-files-present pattern. Twin to mod:c19ee328:slices-containsfunc-allexist.
**Evidence:**
```go
AlreadyDone: func(_ context.Context) (bool, error) {
	for _, f := range ignitionFilenames {
		if !system.FileExists(filepath.Join(clusterDir, f)) {
			return false, nil
		}
	}
	return true, nil
},
```
**Fix — preferred:** `return !slices.ContainsFunc(ignitionFilenames, func(f string) bool { return !system.FileExists(filepath.Join(clusterDir, f)) }), nil`.
**Rule source:** Go 1.21 stdlib: slices.ContainsFunc.
**Adjacent linter:** gocritic (enabled).
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** AlreadyDone contract.
**Estimated net LOC delta:** -3
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** mod:c19ee328:slices-containsfunc-allexist.

### mod:6b533f2d:slices-collect-projection

**ID:** mod:6b533f2d:slices-collect-projection
**Cluster:** slices-maps
**File:** internal/cluster/k8s_csrs.go:L63-L66
**Current LOC touched:** 4
**Smell:** ApprovePendingCSRs builds a names slice via index assignment. The same projection shape recurs in `update_ingress.go:L292-L295` (handleHostNetworkConversion) and `cleanup.go:L43-L50` (KindStrings). The for-i form is already idiomatic Go; flagging only as a candidate for consolidation if a fourth callsite appears.
**Evidence:**
```go
names := make([]string, len(csrs))
for i, csr := range csrs {
	names[i] = csr.Name
}
```
**Fix — preferred:** Optional. Land a small `mapSlice[T,U](in []T, fn func(T) U) []U` helper in internal/sliceutil and migrate all three sites; OR keep as-is. Don't land in isolation — there's no Go-1.x-specific reason to migrate a projection that's already O(N) and clear.
**Rule source:** Go 1.23 stdlib: iter.Seq. Repo parallel-pattern: `internal/distribution/okd/cleanup/cleanup.go:L43-L50`, `internal/distribution/okd/postinstall/update_ingress.go:L292-L295`.
**Adjacent linter:** none.
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** order-preserving projection.
**Estimated net LOC delta:** -2 per site (-6 if all three migrate via helper).
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** medium — borderline call between "modernize" and "keep idiomatic".
**CLAUDE.md / MEMORY.md conflict?:** no

### mod:b8687976:slices-clip-dedup

**ID:** mod:b8687976:slices-clip-dedup
**Cluster:** slices-maps
**File:** internal/distribution/okd/releases/fetcher.go:L101-L113
**Current LOC touched:** 13
**Smell:** deduplicateReleases hand-rolls a seen-set + filtered-result loop. `sortAndClassifySeries` fully reorders downstream so input order is not consumed; a sort+slices.CompactFunc shape would dedup with smaller surface.
**Evidence:**
```go
func (f *OKDVersionFetcher) deduplicateReleases(releases []githubRelease) []githubRelease {
	seen := make(map[string]bool)
	result := make([]githubRelease, 0, len(releases))

	for _, rel := range releases {
		if !seen[rel.TagName] {
			seen[rel.TagName] = true
			result = append(result, rel)
		}
	}
	return result
}
```
**Fix — preferred:** Optional. The seen-set form preserves input order, which slices.CompactFunc does not without a prior sort — so the swap is NOT semantics-preserving for callers that depend on iteration order downstream. AUDIT THE CALLERS first (sortAndClassifySeries fully reorders so it appears safe). If yes: `slices.SortFunc(releases, ...); return slices.CompactFunc(releases, func(a, b githubRelease) bool { return a.TagName == b.TagName })`. Net -7 LOC.
**Rule source:** Go 1.21 stdlib: slices.CompactFunc.
**Adjacent linter:** none.
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** unique-by-TagName output.
**Estimated net LOC delta:** -7
**Severity:** suggestion
**Risk (of applying fix):** medium — order-dependency check required before landing.
**Confidence (in finding):** low — depends on a downstream-order audit the auditor cannot complete from the call site alone.
**CLAUDE.md / MEMORY.md conflict?:** no

### mod:c87d0b1f:log-fatal-in-cli-entrypoint

**ID:** mod:c87d0b1f:log-fatal-in-cli-entrypoint
**Cluster:** slog-over-3p
**File:** cmd/okdctl-gen-docs/main.go:L18-L35
**Current LOC touched:** 18
**Smell:** Uses `log.Fatalf` from the standard `log` package. The repo standard is log/slog; the SKILL flags 'log.Printf, fmt.Println as log' under slog-over-3p. Build-tagged `//go:build docs` so it ships only via `go run -tags docs`, not in the binary — so the bar for "must land" is lower than for production code.
**Evidence:**
```go
if err := os.MkdirAll(*outDir, 0o755); err != nil {
	log.Fatalf("create output dir: %v", err)
}
...
if err := doc.GenMarkdownTree(root, *outDir); err != nil {
	log.Fatalf("generate docs: %v", err)
}
```
**Fix — preferred:** Two options:
- (a) Keep `log.Fatalf`; add a one-line comment that this is a build-tagged docs generator, no slog plumbing desired. Self-documenting exception.
- (b) Switch to `fmt.Fprintln(os.Stderr, "create output dir:", err); os.Exit(1)`; drop the `log` import entirely. No new dependency.

DO NOT pull the project's slog setup into this 35-LOC tool — overkill.
**Rule source:** audit-modernization SKILL.md §rule-catalog: use-slog. CLAUDE.md §architecture-notes (canonical helpers).
**Adjacent linter:** none.
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** non-zero exit on failure (so CI's go-run-tags-docs gate still fires red).
**Estimated net LOC delta:** +2 (add WHY comment) or 0 (option b).
**Severity:** suggestion
**Risk (of applying fix):** low.
**Confidence (in finding):** medium — judgment call on a tiny build-tagged tool.
**CLAUDE.md / MEMORY.md conflict?:** no

## Scaffolding items detected

None. No exported symbols in this audit hit MEMORY.md §scaffolding criteria — every flagged site is a current-call-site migration.

## Linter-config-bug candidates

`gocritic` (enabled in `.golangci.yml`, tags: diagnostic/performance/style) ought to fire on the `slices.Contains`-shaped findings. It includes the `rangeValCopy`, `sloppyLen`, and similar checks but **does NOT flag hand-rolled contains loops**. The relevant gocritic check is `forSwitch` / `paramTypeCombine` / `appendCombine` — none cover this pattern. Modernization linting for slices.Contains specifically is what the proposed-but-not-enabled `intrange`/`copyloopvar`/`gocheckcompilerdirectives` set covers in newer golangci-lint releases; closer still is the `staticcheck:S1001-S1041` series, but `S1041` covers map-merge not slice-contains. **No actionable linter config bug.** Recommend the maintainer revisit when golangci-lint ships an explicit "use slices.Contains" check.

To refresh `linter-config-bugs.jsonl`, run the aggregation command or `/audit-all`.

## Skip list

- **`map[string]bool` vs `map[string]struct{}`** (sites: `releases/fetcher.go:L102`, `update_ingress.go:L290`, `setup/coreos.go:L28`, `setup/tools.go:L58`, `addon/manager.go:L80,L250,L280`). Fix is a memory-style consolidation, not a stdlib migration. **Seam:** belongs in `audit-code-smells` per seams.md §7. Not emitted here.
- **`time.Sleep` in retry loops.** None found in production code (sites in `_test.go` only — out of scope for this audit per AUDIT_CONVENTIONS §2).
- **`net.IP`/`net.ParseCIDR`.** None found. Every IP/CIDR codepath uses `net/netip`.
- **`golang.org/x/exp/*` imports.** None.
- **`io/ioutil`.** None.
- **`interface{}`.** None — fully migrated to `any`.
- **counted `for i := 0; ...` loops.** None outside `_test.go`/`wizard/**`.
- **`testing/synctest`.** Inspected `internal/cluster/k8s_csrs_test.go`, `internal/version/updatecheck_test.go`, and the `_test.go` files in monitor/install/postinstall — they use real time. Out of scope per §2 (_test.go files); a future `audit-tests` pass owns time-test modernization.
- **`internal/cli/doctor.go::checkBinaries` apacheFound loop.** Same pattern as `setup/steps.go::AlreadyDone`, but doctor.go is `//go:build linux` (out of scope per §2). Not landed; if maintainer chooses to relax the linux-only exclusion later, it can be migrated alongside the in-scope twin.

## Cluster verdicts

**net-to-netip:** clean. Zero `net.IP` / `net.ParseIP` / `net.ParseCIDR` outside vendor. Every netutil function uses netip.Addr/Prefix idioms.

**slices-maps:** the longest tail. ~10 lingering hand-rolled loops where the rest of the repo has already migrated to slices.Contains/ContainsFunc/Sorted. None of them are correctness risks; landing them tightens cross-package style consistency.

**errors-and-deprecated-stdlib:** clean. errors.Is/As/Join used wherever appropriate; haproxy.go and cleanup.go use errors.Join idiomatically. No `ioutil`. No `golang.org/x/exp`.

**slog-over-3p:** clean except the docs-generator (build-tagged). Production logging uses log/slog with a redacting handler.

**range-idioms:** clean. range-over-int used in 18+ sites (config.go reflect loop, dns.go node loops, destroy.go target loops, proxmox.go IP-derivation loops, executor/ringbuf.go, iso_cleanup.go, setup/nodes.go, etc.).

**any-interface-builtins:** clean. `any` used uniformly. `clear()`, `min()`, `max()` used in TUI layout, secret zeroize, doctor severity rollup, download progress. `sync.OnceFunc` / `OnceValue` / `OnceValues` all present where appropriate.

## Scope exceptions proposed

None requested. All in-scope findings stay in scope; the linux-only twin (`doctor.go::apacheFound`) is noted for future awareness only.

## Footer

Total findings: 13 (blocker: 0, major: 0, minor: 3, suggestion: 10)
Scope coverage: ~140 of ~140 in-scope `.go` files reviewed (single-pass; no sub-agent dispatch needed for ~10k LOC under tight scope).
Seam deferrals: 1 cluster (`map[string]bool` set idiom → audit-code-smells).
Validation failures on emit: 0.

To refresh `linter-config-bugs.jsonl`, run the aggregation command or `/audit-all`.

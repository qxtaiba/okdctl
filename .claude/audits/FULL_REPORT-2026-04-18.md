# okdctl full audit — 2026-04-18

## Executive summary

215 findings across 14 audits, 9 blockers, 54 majors, 93 minors, 59 suggestions. **All 9 blockers live in `audit-tests` — untested canonical helpers on credential, destructive, or trust-boundary paths** (Zeroize, AtomicWrite, CopyFileMode, WriteTempFile, WriteEnvFile/LoadEnvFile perms, refuseCriticalPath, zip-slip guard, credential-String mask). Zero blockers elsewhere: the structural invariants CLAUDE.md documents (credential `[]byte`+Zeroize, AtomicWrite+fsync, argv-slice-only, critical-path allowlist) hold up under the sweep.

The biggest cross-cutting clusters by impact: (1) the absent slog redacting middleware (security + observability + errors all cross-link on this), (2) ~86 `fmt.Sprintf("...: %v", err)` sites that will blunt any future redact handler, (3) destroy-path orchestrator short-circuit that strands firewall/ISO/haproxy/dnsmasq cleanup when terraform destroy fails, (4) 56 `tui.Info`/`tui.Warn` sites writing status messages to stdout (pipe contamination), and (5) README/architecture-doc drift naming `okdctl version` + `--skip-*` flags that do not exist. 21 findings are **linter-config-bug candidates** — `.golangci.yml` has the rule enabled but tuned to miss. 8 findings are scaffolding (suggestion-capped per MEMORY.md §7). First run — nothing to diff against.

## Per-skill coverage

| Skill | Findings | Blocker | Major | Minor | Suggestion | New | Resolved |
|-------|---------:|--------:|------:|------:|-----------:|----:|---------:|
| audit-security           | 22 | 0 | 9 | 10 | 3 | 22 | 0 |
| audit-subprocess         | 10 | 0 | 3 | 7  | 0 | 10 | 0 |
| audit-state-and-recovery | 13 | 0 | 5 | 7  | 1 | 13 | 0 |
| audit-iac-and-shell      | 9  | 0 | 1 | 5  | 3 | 9  | 0 |
| audit-errors             | 14 | 0 | 4 | 6  | 4 | 14 | 0 |
| audit-concurrency        | 6  | 0 | 0 | 0  | 6 | 6  | 0 |
| audit-api-design         | 20 | 0 | 0 | 9  | 11 | 20 | 0 |
| audit-cli-ux             | 18 | 0 | 2 | 12 | 4 | 18 | 0 |
| audit-observability      | 15 | 0 | 3 | 10 | 2 | 15 | 0 |
| audit-modernization      | 5  | 0 | 0 | 2  | 3 | 5  | 0 |
| audit-code-smells        | 16 | 0 | 0 | 7  | 9 | 16 | 0 |
| audit-dependencies       | 13 | 0 | 1 | 5  | 7 | 13 | 0 |
| audit-documentation      | 15 | 0 | 3 | 8  | 4 | 15 | 0 |
| audit-tests              | 39 | 9 | 23 | 7 | 0 | 39 | 0 |
| **Total**                | **215** | **9** | **54** | **93** | **59** | **215** | **0** |

## Top 20 findings (cross-audit, ranked)

Ranked by `severity_weight × confidence × |net_loc_delta|+1 ÷ risk`. Scaffolding and CLAUDE.md-conflicts excluded.

| Rank | ID | Skill | File:line | Severity/Conf | Fix | Score |
|-----:|----|-------|-----------|---------------|-----|------:|
| 1  | tst:c8b28673:no-test-zipslip-guard | tests | download/extract.go:47 | blocker/high | refactor | 1086 |
| 2  | tst:15ba17da:no-test-destroy-orchestration | tests | destroy/steps.go:23 | major/high | refactor | 679.5 |
| 3  | tst:29293401:no-test-haproxy-rollback | tests | setup/haproxy.go:81 | major/medium | refactor | 486 |
| 4  | tst:62cb8a95:no-test-destroyinfrastructure | tests | destroy/helpers.go:12 | major/medium | refactor | 486 |
| 5  | tst:97cb8adf:test-missing-synctest-waitfor | tests | system/exec.go:28 | major/high | refactor | 423 |
| 6  | tst:e3782ee7:no-test-copyfilemode | tests | system/fs.go:89 | blocker/high | refactor | 404 |
| 7  | tst:35abd54e:no-test-get-proxmox-credentials | tests | credentials/proxmox.go:159 | major/high | refactor | 393 |
| 8  | tst:e3782ee7:no-test-atomicwrite-fsync | tests | system/fs.go:164 | blocker/high | refactor | 364 |
| 9  | tst:ae5b624c:test-missing-synctest-monitor | tests | install/monitor.go:43 | major/medium | refactor | 363 |
| 10 | tst:262af6e4:no-test-cleanup-dispatch | tests | cleanup/cleanup.go:44 | major/high | refactor | 333 |
| 11 | tst:5e892064:no-test-fetchchecksum-parser | tests | download/checksum.go:53 | major/high | refactor | 288 |
| 12 | state:0f076161:no-dry-run-flag | state | cli/destroy.go:21 | major/high | refactor | 274.5 |
| 13 | tst:9ce5434c:no-test-ocresourceexists | tests | phase/kubectl.go:15 | major/high | refactor | 274.5 |
| 14 | tst:f55b9c27:no-test-loadenvfile-perm-refusal | tests | credentials/envfile.go:92 | blocker/high | refactor | 244 |
| 15 | tst:bdf5a873:no-test-saferemove-with-logger | tests | cleanup/artifacts.go:33 | major/high | refactor | 243 |
| 16 | tst:9ce5434c:no-test-ocpolloutput | tests | phase/kubectl.go:28 | major/medium | refactor | 243 |
| 17 | tst:e3782ee7:no-test-writetempfile-cleanup | tests | system/fs.go:42 | blocker/high | refactor | 224 |
| 18 | tst:f55b9c27:no-test-writeenvfile-perms | tests | credentials/envfile.go:42 | blocker/high | refactor | 204 |
| 19 | state:a55b4592:no-schema-version | state | config/loader.go:16 | major/high | refactor | 184.5 |
| 20 | sub:7b2829bb:unbounded-output-buffer | subprocess | executor/executor.go:96 | major/high | refactor | 184.5 |

**Observation:** 14 of top 20 are `audit-tests`. Scoring reflects LOC delta (test files are larger than fix diffs), so the rank is weighted toward "adding a test file" work. Filtering `audit-tests` out surfaces `state:0f076161:no-dry-run-flag`, `state:a55b4592:no-schema-version`, `sub:7b2829bb:unbounded-output-buffer`, `sec:00000003:no-redact-handler`, `ux:b3356305:readme-flag-drift` as the top-ranked non-test findings.

## Ship-blockers (blocker severity, high confidence, low risk)

All 9 live in `audit-tests`. Each cites a specific CLAUDE.md or AUDIT_CONVENTIONS §13 invariant:

1. **tst:35abd54e:no-test-credential-zeroize** — `internal/credentials/proxmox.go:69`. Zeroize wipes `[]byte` password + APIToken; a regression that leaves bytes un-wiped silently breaks the CLAUDE.md §credentials-and-secrets contract.
2. **tst:35abd54e:no-test-credentials-string-mask** — `internal/credentials/proxmox.go:85`. `String()`/`GoString()` exist to mask creds from stray `fmt.Printf`. Regression leaks creds via any accidental `%v`.
3. **tst:f55b9c27:no-test-writeenvfile-perms** — `internal/credentials/envfile.go:42`. Writes .env at 0o600 via AtomicWrite; regression to 0o644 exposes the password file.
4. **tst:f55b9c27:no-test-loadenvfile-perm-refusal** — `internal/credentials/envfile.go:92`. Refuses to load .env with `perm & 0o077 != 0`; regression silently accepts insecure files.
5. **tst:e3782ee7:no-test-atomicwrite-fsync** — `internal/system/fs.go:164`. Canonical trust-boundary file write; covers kubeconfigs, .env, hosts fragments. Losing fsync means power-loss-during-write scenarios go un-noticed.
6. **tst:e3782ee7:no-test-copyfilemode** — `internal/system/fs.go:89`. Canonical cred-file copy; regression loses source mode.
7. **tst:e3782ee7:no-test-writetempfile-cleanup** — `internal/system/fs.go:42`. Canonical temp-file-with-callback; regression leaves cred temp files behind.
8. **tst:bdf5a873:no-test-refusecriticalpath** — `internal/distribution/okd/cleanup/artifacts.go:14`. Defense-in-depth against `rm -rf /etc` on cleanup path.
9. **tst:c8b28673:no-test-zipslip-guard** — `internal/download/extract.go:47`. CWE-22/CWE-59 guard against tar-archive escape. Three distinct guards in one function, all untested.

**Triage direction:** every fix is clone-and-adapt from the existing `iso_cleanup_test.go` template. Rough estimate: 4 focused PRs (credentials / system.fs / cleanup / download) add ~250 LOC of test code and close all 9 blockers.

## Findings by skill — artifact pointers

Each skill's full report was returned at dispatch time; below is the artifact index. Re-dispatch the skill (`/audit-<name>`) to get the full per-finding markdown, or read the JSONL row directly.

- `.claude/audits/audit-security.jsonl` — 22 findings (0/9/10/3). Clusters: credentials(6), file-toctou(7), tls-network(3), redaction(2), input-validation(3), shell-injection-clean(1). Biggest headline: HTTP-over-LAN for ignition delivery embedding pullSecret; checksum fail-open; no slog redacting middleware.
- `.claude/audits/audit-subprocess.jsonl` — 10 findings (0/3/7/0). Clusters: io-handling(10). Biggest: canonical `Executor` buffers stdout/stderr unbounded + inherits full parent env (credential-carrier risk).
- `.claude/audits/audit-state-and-recovery.jsonl` — 13 findings (0/5/7/1). Biggest: fatal-step early-return strands four cleanup steps on TF-destroy failure; plan-failure silent fallback to direct destroy; no schemaVersion on okdctl.yaml.
- `.claude/audits/audit-iac-and-shell.jsonl` — 9 findings (0/1/5/3). Biggest: `install.sh` consumes goreleaser's SHA256SUMS without verifying the cosign signature goreleaser already publishes.
- `.claude/audits/audit-errors.jsonl` — 14 findings (0/4/6/4). Biggest: errtypes `Error()` uses `%v` on inner err (latent cred exposure); executor strips `*exec.ExitError`; bare `fmt.Errorf` where typed errors are called for.
- `.claude/audits/audit-concurrency.jsonl` — 6 findings (0/0/0/6). All suggestion-severity — concurrency surface is unusually clean (4 goroutines total, all documented). Main ask: CLAUDE.md §Concurrency anchor.
- `.claude/audits/audit-api-design.jsonl` — 20 findings (0/0/9/11). Biggest: constructor shapes diverge (variadic vs positional vs builder vs Default*); `WithEnv` option has order-dependence; duplicated OKD resource minimums.
- `.claude/audits/audit-cli-ux.jsonl` — 18 findings (0/2/12/4). Biggest: `tui.Info`/`tui.Warn` to stdout across 56 sites; README flag drift; exit-code taxonomy half-published (SIGTERM collapses to 130).
- `.claude/audits/audit-observability.jsonl` — 15 findings (0/3/10/2). Biggest: no redacting handler installed; ~86 `fmt.Sprintf("...: %v", err)` sites vs structured `"err", err`; no per-step span boundaries at orchestrator.
- `.claude/audits/audit-modernization.jsonl` — 5 findings (0/0/2/3). Repo already 1.25-idiomatic (netip done, slices/errors.Join used). Residual: `sync.Once` → `sync.OnceFunc`, `maps.Keys+sort` → `slices.Sorted(maps.Keys)`, one hand-rolled `slices.Contains`, one `slices.Backward`.
- `.claude/audits/audit-code-smells.jsonl` — 16 findings (0/0/7/9). Biggest: 26 `if logger != nil` guards bypassing `logutil.NopLogger`; parallel role enums (NodeRole/VMRole); platform-family strings hardcoded.
- `.claude/audits/audit-dependencies.jsonl` — 13 findings (0/1/5/7). Supply chain clean: all 36 deps permissive-licensed, actions SHA-pinned, SLSA3 provenance. One major: `go-proxmox v0.x` on critical discovery path, single maintainer.
- `.claude/audits/audit-documentation.jsonl` — 15 findings (0/3/8/4). Biggest: README drift (ghost flags); `docs/architecture/phases.md` BasePhase fields wrong; `docs/architecture/addons.md` Addon interface won't compile; canonical `Step*`/`Orchestrator`/`BasePhase` APIs under-documented.
- `.claude/audits/audit-tests.jsonl` — 39 findings (9/23/7/0). See ship-blockers above. Critical-path coverage ~14% (6/43 functions); single existing test file is the canonical template.

## Seams resolved

**65 cross-audit seam pointers** recorded via `seam:` field. All map to valid audits in `seams.md`. **Zero unmapped seams** — no new entries needed. Notable clusters:

- **Redaction (seam #3)** — 7 cross-links between security (policy), errors (chain), observability (sink), tests (missing coverage). The absence of a slog redacting middleware is the coherent story across 4 audits.
- **Destroy safety (seam #5)** — 6 cross-links between state-and-recovery (orchestration), security (credential hygiene during destroy), ux (dry-run flag), tests (no coverage).
- **Executor buffering (seams #1+observability)** — 3 cross-links between subprocess (per-site), observability (streaming discipline), errors (exit-err type preservation).
- **Exit codes (seam #4)** — 7 cross-links between cli-ux (taxonomy) and errors (mapping). cli-ux published the taxonomy table; errors tagged 5 typed-err fallthrough sites.
- **Exported surface doc (seam #12)** — 4 cross-links between api-design ("shouldn't be exported") and documentation ("missing doc").

## Scaffolding items (not ranked) — 8 total

Per MEMORY.md §7, these are capped at `suggestion` severity with fix class "verify intent against roadmap.md", NOT delete.

| ID | Reason | Location |
|----|--------|----------|
| api:25fa1be8:export-no-caller-configure | symmetric-api | firewall.Configure / RemoveRules pair |
| api:66f217c9:export-no-caller-getlatestforminor | symmetric-api | OKDVersionFetcher (GetLatestStable sibling used) |
| api:1d5afa08:export-no-caller-shortversion | symmetric-api | OKDVersion (DisplayName/Major/Minor used) |
| api:98723e5d:export-no-caller-validateclusteraccess | future-cli-verb | install.Phase.ValidateClusterAccess / SetupClusterAccess / SetupKubeconfig |
| smell:2c4d8e6b:unused-metadata-field | symmetric-api | AddonInfo.Category populated but unread |
| smell:2be6306e:scaffolding-registry-api | symmetric-api | addon.IsRegistered (sibling of Register/Get/All/Enabled) |
| err:d6b325cb:vocab-ad-hoc-sentinel | symmetric-api | proxmox.ErrNotConnected / ErrTerraformNotConfigured |
| err:a4001485:vocab-gap-cert-pending | symmetric-api | errtypes — Recoverable/CertPending named in coordinator memo |

## Linter-config-bug candidates — 21 total

Findings where the adjacent linter IS enabled in `.golangci.yml` but did not fire on this site. These become one `.golangci.yml` tuning PR that resolves N findings across multiple audits. Full list in `.claude/audits/linter-config-bugs.jsonl`.

**By linter:**
- **`unused` (7)** — Go's `unused` linter misses struct-field write-only + scaffolding symbols. 4 api-design + 3 code-smells findings cite it.
- **`goconst` (4)** — `min-occurrences: 3` is too loose for cross-file duplicated magic strings ("rhel", "host", "bootstrap").
- **`revive` (4)** — `exported` / `package-comments` / `context-as-argument` rules configured but quiet on some sites; 2 api-design + 2 documentation findings.
- **`dupl`, `gocritic`, `gosec:G204`, `gosec:G402`, `errcheck` (1 each)** — each a single-site mis-fire.

**Recommendation:** one PR that tightens `.golangci.yml` (lower `goconst.min-occurrences`, enable `unused` struct-field-write-only check via a ruleguard rule, enable `errorlint`, verify `revive` rule severities). Catches ~21 findings without touching code.

## Artifact index

- `.claude/audits/audit-security.jsonl` — 22 findings
- `.claude/audits/audit-subprocess.jsonl` — 10 findings
- `.claude/audits/audit-state-and-recovery.jsonl` — 13 findings
- `.claude/audits/audit-iac-and-shell.jsonl` — 9 findings
- `.claude/audits/audit-errors.jsonl` — 14 findings
- `.claude/audits/audit-concurrency.jsonl` — 6 findings
- `.claude/audits/audit-api-design.jsonl` — 20 findings
- `.claude/audits/audit-cli-ux.jsonl` — 18 findings
- `.claude/audits/audit-observability.jsonl` — 15 findings
- `.claude/audits/audit-modernization.jsonl` — 5 findings
- `.claude/audits/audit-code-smells.jsonl` — 16 findings
- `.claude/audits/audit-dependencies.jsonl` — 13 findings
- `.claude/audits/audit-documentation.jsonl` — 15 findings
- `.claude/audits/audit-tests.jsonl` — 39 findings
- `.claude/audits/linter-config-bugs.jsonl` — 21 findings (derived; `.golangci.yml` enabled but missed)
- `.claude/audits/history/` — prior snapshots (empty; first run)

## Footer

- Total findings across all audits: **215** (blocker 9, major 54, minor 93, suggestion 59)
- Skills that failed to run: 0
- Schema validation failures dropped: 0
- Duplicate IDs dropped: 0
- Seam deferrals recorded: 65 (all mapped; 0 unmapped)
- Scaffolding items kept: 8
- Linter-config-bug candidates: 21
- Wall time: ~35 minutes (3 waves of parallel dispatch)
- Run date: 2026-04-18 (UTC)
- Coverage: 100% of in-scope Go files per each skill's self-report; no sub-agent dispatch by most skills (each codebase slice under the 500-LOC threshold)

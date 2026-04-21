# okdctl full audit — 2026-04-20

## Executive summary

201 findings across 14 audits: **12 blocker, 47 major, 76 minor, 66 suggestion**. Every blocker is in `audit-tests`, all on untested critical-surface code (credential handling, destructive ops, canonical helpers). Net progress since 2026-04-18: **75 prior findings resolved, 140 recurring, 61 new**. Biggest wins: `audit-documentation` (15/15 resolved — all package-doc and exported-doc coverage closed), `audit-dependencies` (8 resolved — Tier-B deps work landed), `audit-state-and-recovery` (6 resolved — destroy now NonFatal, `--dry-run` shipped, schemaVersion enforced), `audit-observability` (7 resolved — RedactHandler shipped, pervasive `%v err` cleaned). Biggest cluster: `audit-tests` with 48 findings and zero resolved — coverage has not moved since prior run. Ship-blocker count (blocker + high-confidence + low-risk): **11**, all concentrated in credentials, canonical helpers, and destructive-path test coverage.

## Per-skill coverage

| Skill | Findings | blocker | major | minor | suggestion | New | Recurring | Resolved |
|-------|---------:|--------:|------:|------:|-----------:|----:|----------:|---------:|
| audit-api-design | 21 | 0 | 0 | 10 | 11 | 9 | 12 | 8 |
| audit-cli-ux | 11 | 0 | 0 | 5 | 6 | 5 | 6 | 12 |
| audit-code-smells | 14 | 0 | 0 | 5 | 9 | 4 | 10 | 6 |
| audit-concurrency | 7 | 0 | 0 | 0 | 7 | 1 | 6 | 0 |
| audit-dependencies | 8 | 0 | 0 | 3 | 5 | 3 | 5 | 8 |
| audit-documentation | 2 | 0 | 0 | 2 | 0 | 2 | 0 | 15 |
| audit-errors | 14 | 0 | 2 | 6 | 6 | 4 | 10 | 4 |
| audit-iac-and-shell | 10 | 0 | 0 | 4 | 6 | 3 | 7 | 2 |
| audit-modernization | 12 | 0 | 0 | 2 | 10 | 12 | 0 | 5 |
| audit-observability | 13 | 0 | 2 | 10 | 1 | 5 | 8 | 7 |
| audit-security | 23 | 0 | 8 | 11 | 4 | 3 | 20 | 2 |
| audit-state-and-recovery | 7 | 0 | 2 | 4 | 1 | 0 | 7 | 6 |
| audit-subprocess | 11 | 0 | 4 | 7 | 0 | 1 | 10 | 0 |
| audit-tests | 48 | 12 | 29 | 7 | 0 | 9 | 39 | 0 |
| **TOTAL** | **201** | **12** | **47** | **76** | **66** | **61** | **140** | **75** |

## Top 20 findings (cross-audit, ranked)

Rank = `severity_weight × confidence × |loc_delta+1| ÷ risk_numeric` (blocker=4, major=3, minor=2, suggestion=1; high-confidence=3 / med=2 / low=1; low-risk=1 / med=2 / high=3 so easier-to-apply ranks higher).

| Rank | ID | Skill | File:line | Severity | Confidence | Fix |
|---:|---|---|---|---|---|---|
| 1 | `tst:97cb8adf:test-missing-synctest-waitfor` | tests | `internal/system/exec.go:28` | major | high | refactor |
| 2 | `tst:e3782ee7:no-test-copyfilemode` | tests | `internal/system/fs.go:89` | blocker | high | refactor |
| 3 | `tst:35abd54e:no-test-get-proxmox-credentials` | tests | `internal/credentials/proxmox.go:159` | major | high | refactor |
| 4 | `tst:e3782ee7:no-test-atomicwrite-fsync` | tests | `internal/system/fs.go:164` | blocker | high | refactor |
| 5 | `tst:daf5bee9:no-test-kubeconfig-merge-noclobber` | tests | `internal/cli/kubeconfig.go:77` | blocker | high | refactor |
| 6 | `tst:c8b28673:no-test-zipslip-guard` | tests | `internal/download/extract.go:47` | blocker | high | refactor |
| 7 | `tst:262af6e4:no-test-cleanup-dispatch` | tests | `internal/distribution/okd/cleanup/cleanup.go:44` | major | high | refactor |
| 8 | `tst:5e892064:no-test-fetchchecksum-parser` | tests | `internal/download/checksum.go:53` | major | high | refactor |
| 9 | `tst:761e5126:no-test-removehaproxy` | tests | `internal/distribution/okd/postinstall/haproxy.go:23` | major | high | refactor |
| 10 | `tst:f55b9c27:no-test-loadenvfile-perm-refusal` | tests | `internal/credentials/envfile.go:92` | blocker | high | refactor |
| 11 | `tst:bdf5a873:no-test-saferemove-with-logger` | tests | `internal/distribution/okd/cleanup/artifacts.go:33` | major | high | refactor |
| 12 | `tst:15ba17da:no-test-destroy-orchestration` | tests | `internal/distribution/okd/destroy/steps.go:23` | major | high | refactor |
| 13 | `tst:e3782ee7:no-test-writetempfile-cleanup` | tests | `internal/system/fs.go:42` | blocker | high | refactor |
| 14 | `tst:4583b75b:no-test-redactconfig` | tests | `internal/cli/config.go:67` | blocker | high | refactor |
| 15 | `tst:368b892b:no-test-preserve-tfstate` | tests | `internal/distribution/okd/cleanup/infra.go:47` | blocker | high | refactor |
| 16 | `tst:f55b9c27:no-test-writeenvfile-perms` | tests | `internal/credentials/envfile.go:42` | blocker | high | refactor |
| 17 | `tst:bdf5a873:no-test-refusecriticalpath` | tests | `internal/distribution/okd/cleanup/artifacts.go:14` | blocker | high | refactor |
| 18 | `tst:451be4fa:no-test-chowntree-lchown` | tests | `internal/system/elevation.go:111` | major | high | refactor |
| 19 | `tst:35abd54e:no-test-credentials-env` | tests | `internal/credentials/proxmox.go:104` | major | high | refactor |
| 20 | `tst:35abd54e:no-test-credential-zeroize` | tests | `internal/credentials/proxmox.go:69` | blocker | high | refactor |

**Note:** The top-20 is dominated by `audit-tests` because untested functions carry high LOC factor (the function body itself is the LOC delta — the "fix" is writing that many lines of test). This reflects roadmap.md §5's deferred-tests stance: tests are not priority, but the critical-surface gap is genuinely the biggest open item when surfaced by the ranking formula. See the "Top 15 non-tests" table below for diversity.

## Top 15 non-tests findings (for cross-skill visibility)

| ID | Skill | File:line | Severity | Confidence | Fix |
|---|---|---|---|---|---|
| `con:97cb8adf:synctest-opportunity` | concurrency | `internal/system/exec.go:28` | suggestion | high | refactor |
| `ux:e7db1220:json-schema-undoc` | cli-ux | `internal/cli/releases.go:84` | minor | high | refactor |
| `ux:d31d1b9d:describe-format-missing` | cli-ux | `internal/cli/status.go:31` | minor | high | refactor |
| `state:15ba17da:no-scoped-destroy` | state-and-recovery | `internal/distribution/okd/destroy/steps.go:24` | minor | high | refactor |
| `sub:7b2829bb:unbounded-output-buffer` | subprocess | `internal/executor/executor.go:116` | major | high | refactor |
| `ux:073d24ed:help-no-example` | cli-ux | `internal/cli/deploy.go:29` | suggestion | high | refactor |
| `ux:fd2125dd:addon-args-no-completion` | cli-ux | `internal/cli/addon.go:31` | suggestion | high | refactor |
| `state:4c092fce:no-concurrent-run-guard` | state-and-recovery | `internal/infrastructure/terraform/terraform.go:119` | major | medium | refactor |
| `obs:660d83a5:tui-bypasses-redact-handler` | observability | `internal/tui/logger.go:67` | major | high | refactor |
| `sub:7b2829bb:no-cmd-env` | subprocess | `internal/executor/executor.go:112` | major | high | refactor |
| `smell:92553fff:status-stringly-typed-enum` | code-smells | `internal/cli/summary.go:155` | minor | high | refactor |
| `sub:de572c63:stderr-dropped-nmcli` | subprocess | `internal/distribution/okd/dns/dnsmasq.go:155` | minor | high | refactor |
| `state:fb54208a:postinstall-no-rollback-path` | state-and-recovery | `internal/distribution/okd/postinstall/steps.go:42` | minor | medium | refactor |
| `con:ae5b624c:synctest-opportunity` | concurrency | `internal/distribution/okd/install/monitor.go:43` | suggestion | medium | refactor |
| `state:15ba17da:misleading-teardown-summary` | state-and-recovery | `internal/distribution/okd/destroy/steps.go:105` | major | high | refactor |

## Ship-blockers (blocker + high-confidence + low-risk)

11 findings gate the next release. All are untested critical-surface paths:

- `tst:35abd54e:no-test-credential-zeroize` — `internal/credentials/proxmox.go:69` — cred-path-untested
- `tst:35abd54e:no-test-credentials-string-mask` — `internal/credentials/proxmox.go:85` — cred-path-untested
- `tst:f55b9c27:no-test-writeenvfile-perms` — `internal/credentials/envfile.go:42` — cred-path-untested
- `tst:f55b9c27:no-test-loadenvfile-perm-refusal` — `internal/credentials/envfile.go:92` — cred-path-untested
- `tst:e3782ee7:no-test-atomicwrite-fsync` — `internal/system/fs.go:164` — canonical-helper-untested
- `tst:e3782ee7:no-test-copyfilemode` — `internal/system/fs.go:89` — canonical-helper-untested
- `tst:e3782ee7:no-test-writetempfile-cleanup` — `internal/system/fs.go:42` — canonical-helper-untested
- `tst:bdf5a873:no-test-refusecriticalpath` — `internal/distribution/okd/cleanup/artifacts.go:14` — destructive-untested
- `tst:4583b75b:no-test-redactconfig` — `internal/cli/config.go:67` — cred-path-untested
- `tst:368b892b:no-test-preserve-tfstate` — `internal/distribution/okd/cleanup/infra.go:47` — destructive-untested
- `tst:daf5bee9:no-test-kubeconfig-merge-noclobber` — `internal/cli/kubeconfig.go:77` — cred-path-untested

**One additional blocker** (`tst:c8b28673:no-test-zipslip-guard`) is `blocker/high/medium` — still critical but falls outside the strict "low-risk-to-apply" gate because the release-tarball test harness needs a bit more infrastructure.

**Per roadmap §5** ("tests are not priority"), these blockers are a policy-deferred cluster — the severity reflects the rubric anchor (credentials, canonical helpers, destructive ops), not a demand that they ship this release. The recommendation is to either (a) treat §5 as an explicit ship-blocker override in CLAUDE.md, or (b) nominate a subset (credentials + destroy + CopyFileMode) for a focused test-hardening roadmap item.

## Findings by skill

| Skill | Report (JSONL) | Headline |
|---|---|---|
| [audit-security](audit-security.jsonl) | 23 rows | 8 majors across credentials (5), TLS (4), file-TOCTOU (6), privilege escalation (2), input validation (3); new: bootstrap-oc integrity gap, debug-bundle redact regression, metrics-unauth-listener |
| [audit-subprocess](audit-subprocess.jsonl) | 11 rows | 4 majors clustered on `internal/executor`: unbounded buffer + full-env inherit + terraform-buffered-execution + new iso-cleanup node-shell-injection |
| [audit-state-and-recovery](audit-state-and-recovery.jsonl) | 7 rows | destroy NonFatal landed; concurrent-run lock still missing; teardown-summary promoted to major because it now fires on partial-failure paths |
| [audit-iac-and-shell](audit-iac-and-shell.jsonl) | 10 rows | cosign verification + INSECURE=1 warning shipped; remaining polish on install.sh (pipefail, grep -F, cosign stderr) + missing shellcheck/tflint in CI |
| [audit-errors](audit-errors.jsonl) | 14 rows | 2 majors: errtypes still `%v`s inner err (bypasses RedactHandler on string path); monitor.go:34 ClusterError wraps without Err (breaks `errors.Is(err, context.DeadlineExceeded)`) |
| [audit-concurrency](audit-concurrency.jsonl) | 7 rows | all suggestions; one new: root.go signal-watcher goroutine has process-lifetime leak (defer close(sigCh) would terminate it cleanly) |
| [audit-api-design](audit-api-design.jsonl) | 21 rows | addon.Manager + phase.BasePhase functional-options migration landed; remaining: 4 ctx-missing-on-io gaps, 1 package-reach-through (destroy→setup), 4 scaffolding exports |
| [audit-cli-ux](audit-cli-ux.jsonl) | 11 rows | exit-code taxonomy published; stream discipline + SIGINT/SIGTERM split done; remaining: destroy --force vs siblings --yes, JSON schema undoc, describe missing --format |
| [audit-observability](audit-observability.jsonl) | 13 rows | 2 majors: `tui.Debug/Info/Warn/Error` and `--log-file` bypass RedactHandler (only SimpleLogger wraps it); one fix resolves both |
| [audit-modernization](audit-modernization.jsonl) | 12 rows | 12 new — all stdlib migration opportunities (min/max builtins, slices.ContainsFunc/IndexFunc/Concat, cmp.Or, strings.Lines); net LOC delta ≈ −59 |
| [audit-code-smells](audit-code-smells.jsonl) | 14 rows | 6 resolved (nil-logger sprawl → `logutil.OrNop`); remaining: parallel-enum-duplicate, duplicated ExitError/ExecError, stringly-typed status |
| [audit-dependencies](audit-dependencies.jsonl) | 8 rows | Tier-B deps work landed; one new: `.goreleaser.yaml` SBOM scoped to archive artifacts only — .deb/.rpm get no SBOM |
| [audit-documentation](audit-documentation.jsonl) | 2 rows | 15 prior all resolved; 2 residual: duplicate `// Package distribution` block in `context.go`, FieldType enum drift in wizard.md |
| [audit-tests](audit-tests.jsonl) | 48 rows | zero progress since 2026-04-18 (no new tests covering flagged surface); 9 new blockers/majors catch Wave-1 gaps (redactConfig, kubeconfig-noclobber, tfstate-preservation, Lchown contract) |

## Seams resolved

30 findings carry a non-`none` `seam` field. All referenced audits are registered in `seams.md`. Seams that fall outside the current `seams.md` table (flagged here for the next `seams.md` update, not as errors):

- `api-design → audit-tests` (concrete-return-k8s motivating a test) — new entry: api-design concrete-return vs tests
- `api-design → audit-documentation` (3 findings where "should be unexported OR documented") — seam #12 partially covers but reverse direction
- `cli-ux → audit-state-and-recovery` (dangerous-op-confirm-cluster-name typo guard) — new entry
- `concurrency → audit-tests` (2 synctest-opportunity rows) — new entry (synctest landed in Go 1.25, tests-owned for test-file changes)
- `observability → audit-cli-ux` (handler-no-tty-switch) — new entry
- `observability → audit-errors` (root-error-stringified) — new entry (though close to seam #3)

These are emitted by the audit most natural to the fix per the §6 default rule; when `seams.md` is next edited, add the above.

## Scaffolding items (not ranked)

7 findings flagged with `scaffolding=true`. Per MEMORY.md §scaffolding, severity is capped at `suggestion` and the fix is "verify intent" (grep roadmap.md, ask the owner), NOT "delete":

- `api:25fa1be8:export-no-caller-configure` — symmetric-api: Configure/RemoveRules/ConfigureOKD/RemoveOKDRules quartet in `firewall/firewall.go:97`
- `api:66f217c9:export-no-caller-getlatestforminor` — symmetric-api: FetchVersions/GetLatestStable/GetLatestForMinor trio in `releases/okd.go:45`
- `api:98723e5d:export-no-caller-validateclusteraccess` — future-cli-verb: shaped for `okdctl cluster access verify` in `install/flux.go:15`
- `api:830d4653:export-no-caller-installed-lists` — future-cli-verb: shaped for `okdctl cleanup preview` in `cleanup/packages.go:34`
- `smell:2be6306e:scaffolding-registry-api` — symmetric-api: `IsRegistered` alongside Register/Get/All/Names/Enabled in `addon/registry.go:86`
- `err:d6b325cb:vocab-ad-hoc-sentinel` — symmetric-api: `ErrNotConnected`/`ErrTerraformNotConfigured` in `proxmox/types.go:5`
- `err:a4001485:vocab-gap-cert-pending` — symmetric-api: errtypes gap for `RecoverableError`/`CertPendingError` in `errtypes/errtypes.go:1`

## Linter-config-bug candidates

22 findings in `.claude/audits/linter-config-bugs.jsonl` (aggregated via `jq -c 'select(.adjacent_linter_enabled==true)' .claude/audits/audit-*.jsonl`). Breakdown by linter:

| Linter | Rows | Notes |
|---|---:|---|
| `unused` | 8 | Exported scaffolding symbols that `unused` won't fire on because they are exported (treated as public API) |
| `revive` | 5 | Mostly `exported` / `context-as-argument` rules with the finding in a non-revivable context |
| `gocritic` | 4 | `min`/`max` clamp candidates and `ifElseChain` that current gocritic tag selection doesn't surface |
| `dupl` | 2 | Small semantic duplicates under the 200-line threshold |
| `gosec:G204` | 1 | Canonical-executor wrapper bypasses per-site annotation |
| `gosec:G402` | 1 | TLS NewInsecure VIP probe (intentional, per security finding) |
| `errcheck` | 1 | `_, _ = p.Exec.Run(...)` escape-hatch on semanage |

Most of these are not actually `.golangci.yml` tuning opportunities — they are "the linter's rule doesn't cover this pattern" gaps. The two actionable config tweaks: (1) lower `dupl` threshold or add a specific `dupl`-small rule; (2) expand `gocritic` `enabled-checkers` to explicitly surface `ifElseChain` / clamp-via-max.

## Artifact index

| Artifact | Rows | Purpose |
|---|---:|---|
| `.claude/audits/audit-security.jsonl` | 23 | credentials, TLS, file-TOCTOU, privilege-escalation |
| `.claude/audits/audit-subprocess.jsonl` | 11 | exec.Command argv + io hygiene |
| `.claude/audits/audit-state-and-recovery.jsonl` | 7 | destroy, resume, idempotency |
| `.claude/audits/audit-iac-and-shell.jsonl` | 10 | install.sh + Terraform HCL |
| `.claude/audits/audit-errors.jsonl` | 14 | wrapping, typed errors, redaction chain |
| `.claude/audits/audit-concurrency.jsonl` | 7 | goroutine lifetime, ctx cancellation |
| `.claude/audits/audit-api-design.jsonl` | 21 | package boundaries, exported surface, option consistency |
| `.claude/audits/audit-cli-ux.jsonl` | 11 | verb-noun, exit codes, help, --json |
| `.claude/audits/audit-observability.jsonl` | 13 | slog, redaction sink, log-once, spans |
| `.claude/audits/audit-modernization.jsonl` | 12 | Go 1.21-1.25 stdlib migrations |
| `.claude/audits/audit-code-smells.jsonl` | 14 | catch-all idioms (magic-strings, premature abstraction) |
| `.claude/audits/audit-dependencies.jsonl` | 8 | license, maintenance, pin hygiene |
| `.claude/audits/audit-documentation.jsonl` | 2 | package-doc, exported-doc, README drift |
| `.claude/audits/audit-tests.jsonl` | 48 | critical-path test gaps |
| **`.claude/audits/linter-config-bugs.jsonl`** | **22** | **derived: findings whose adjacent linter is already enabled** |
| `.claude/audits/history/*-2026-04-20.jsonl` | 14 files | prior snapshot, idempotent by date |

## Footer

- **Total findings:** 201 (blocker 12, major 47, minor 76, suggestion 66)
- **Skills that failed to run:** 0 (all 14 returned)
- **Schema validation failures dropped:** 0 (every row validates)
- **Duplicate IDs dropped:** 0
- **Seam deferrals (findings referencing other audits):** 30
- **Unmapped seams flagged for seams.md update:** 6 (see above — api-design↔tests, api-design↔doc, cli-ux↔state, concurrency↔tests, observability↔cli-ux, observability↔errors)
- **Scaffolding items (not ranked):** 7
- **Linter-config-bug candidates:** 22
- **Prior-run delta:** 61 new / 140 recurring / 75 resolved
- **Wall time (dispatch → aggregation):** ~20 minutes across 3 parallel waves

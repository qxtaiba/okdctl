# Completed roadmap items — archive

This file is the historical record of completed roadmap items, archived
out of `roadmap.md` to keep the active file readable and to keep
`/roadmap-pickup` sessions from ingesting ~1,000 lines of postmortem prose
on every run.

The active `roadmap.md` keeps a short pointer in its `## Completed`
section. The Appendix ledger at the bottom of `roadmap.md` is the
canonical "is dep X done?" lookup; this file is for incident review,
postmortem search, and pattern reuse.

## Provenance

- Archived from `roadmap.md` on 2026-04-26.
- New completions append here directly. When a PR merges (or an item is
  closed without code), add the entry below in the same shape as the
  existing entries — close date, PR / merge commit, terse evidence,
  postmortem lesson when one exists.

## Entries

Items that have reached `done` status, ordered by close date. New
entries land here when a PR merges, or when an item is closed without
code (audit error, done-by-prior-work). Keep the explanation terse
but link evidence.

- **`sec:cfcdee2d:tls-no-redirect-cap`** — done 2026-04-26 — PR #144,
  merge commit `459a9a8`. Tier H minor (tls-network). All three
  `httputil` factories (`New`, `NewInsecure`, `NewWithCA`) returned
  `&http.Client{}` with no `CheckRedirect`, so Go's stdlib default
  allowed 10 hops and only stripped `Authorization` for headers it
  managed internally — a header set explicitly via `req.Header.Set`
  survived cross-host redirects. Added a shared unexported
  `capRedirects` referenced from all three factories: caps at 5 hops
  via `len(via) >= 5`, refuses cross-host redirects that carry
  `Authorization`. Added `TestCapRedirects` (six sub-cases covering
  same-host below cap, cross-host without auth, cross-host with auth
  refusal, cap boundary, four redirects below cap, cap-precedes-cross-host).
  No call site sets `Authorization` today, so the policy is
  backward-compatible and forward-protective. **Postmortem lesson:**
  `.github/scripts/coverage-check.sh` averages per-function coverage
  percents (not statement-weighted), so the 95% floor on `internal/httputil`
  is a *function-shape* invariant — every exported func must be near
  100% covered, otherwise even one 80%-covered function (here
  `KubeconfigCAPool`) drags the average. Adding the well-tested
  `capRedirects` *raised* the average from 95.0% → 96.0% rather than
  lowering it; without the new test the cap would have held but the
  margin would be zero. Read the floor script before assuming
  coverage tradeoffs.

- **`sec:881d089e:input-path-not-prefix-checked`** — done 2026-04-26
  — PR #145, merge commit `e161c6b`. Tier H minor (file-toctou).
  `runlock.Acquire` opened `<projectRoot>/.okdctl.lock` with
  `O_RDWR|O_CREATE|0o600` and no `O_NOFOLLOW`; under the deploy/destroy
  sudo re-exec model the open runs as root while `projectRoot` is
  user-writable, so a planted symlink at the lock path would redirect
  the root-authored `PID=…/VERB=…/TIME=…` write. Mirrored the
  canonical pattern at `cli/logging.go:24-32`: `os.Lstat` first and
  refuse a symlink with a `*errtypes.ConfigError`, otherwise add
  `syscall.O_NOFOLLOW` to the existing `O_RDWR|O_CREATE` flags so a
  symlink swapped in between lstat and open still loses the race. Kept
  `O_RDWR|O_CREATE` rather than copying logging.go's `O_WRONLY|O_APPEND`
  — the lock file uses RDWR for diagnostic read-back; the log file
  uses APPEND for tailing. **Postmortem lesson:** when reusing a
  security pattern from a sibling call site, copy the *structural
  shape* (lstat-refuse-symlink → NOFOLLOW open) but keep the
  call-site-specific flags. A shared helper here would have hidden
  the meaningful flag-set difference behind an option struct.

- **`sec:29293401:toctou-chmod`** — done 2026-04-26 — PR #146, merge
  commit `9398863`. Tier H minor (file-toctou). The HAProxy rollback
  closure paired `system.CopyFile(backup, configPath)` with
  `os.Chmod(configPath, 0o644)`. `os.Chmod` follows symlinks on Linux,
  and on the privileged `/etc/haproxy/haproxy.cfg` path the chmod
  could redirect via a planted symlink between the copy and the
  chmod. Replaced the pair with a single
  `system.CopyFileMode(backup, configPath, 0o644)` call: mode is set
  at open time in one syscall, removing both the redundant chmod and
  the symlink-follow window. The `chmodErr` warn-only branch is gone
  too. **Postmortem lesson:** the paired finding `sec:de572c63`
  (dnsmasq) chose a different resolution for the same TOCTOU shape —
  it dropped the chmod entirely (`CopyFile` only) to preserve any
  operator hardening on the dnsmasq drop-in. Two valid resolutions
  for the same hazard at different sites: dnsmasq trusts operator
  intent, haproxy enforces 0o644 because that's the audit baseline
  for haproxy.cfg. The roadmap's per-item Fix is the authority on
  which resolution applies where; don't assume a fix recipe transfers
  unchanged across audit-paired entries.

- **`sec:5013fea6:dl-no-checksum`** — done 2026-04-26 — PR #134, merge
  commit `d179101`. Tier H major (tls-network). Replaced unchecked
  `mirror.openshift.com/.../latest/linux/oc.tar.gz` fetch with a
  SHA-256-verified download from a pinned okd-scos GitHub release.
  Introduced `bootstrapOCVersion = "4.18.0-okd-scos.8"` as a
  compile-time constant in `release_extract.go`, decoupled from
  `cfg.Cluster.Version`: bootstrap-oc only runs `oc adm release
  extract` once before the cluster oc swaps in, so coupling them was
  unnecessary and would have broken against the project default
  version (`4.18.0-okd-scos.10` has no GitHub release — verified 404
  live during planning). `bootstrapOC` derives the asset name and
  `sha256sum.txt` URL from the pinned constant, fetches the digest via
  `download.FetchChecksum`, and passes it to
  `download.Options.ExpectedChecksum`. Failure is fail-closed (no
  fallback to unverified bytes). **Postmortem lesson:** the first
  planner draft tied the bootstrap-oc URL to `cfg.Cluster.Version`
  because that was the most "obvious" version source — but bootstrap-oc
  and cluster-oc serve different roles. Always interrogate "does this
  knob actually need to track that knob?" before adding the wiring.

- **`sec:8ea706f6:dl-helm-sops-no-checksum`** — done 2026-04-26 — PR
  #133, merge commit `0e120c5`. Tier H major (tls-network). helm and
  sops binary downloads landed in `/usr/local/bin` without integrity
  verification despite version-pinning. Added `checksumURLTemplate` +
  `checksumFilenameTemplate` to `binaryToolMeta` for `toolHelm` and
  `toolSops`; `installTool` resolves the per-arch URLs alongside the
  existing `urlTemplate`; `installBinary` calls
  `download.FetchChecksum` before `download.Download` and passes the
  digest as `ExpectedChecksum`. Helm uses
  `https://get.helm.sh/helm-vX.Y.Z-linux-{arch}.tar.gz.sha256sum`; sops
  uses `https://github.com/getsops/sops/releases/download/vX.Y.Z/sops-vX.Y.Z.checksums.txt`.
  `toolYQ` is intentionally untouched (separate roadmap item); the
  `checksumURL == ""` gate keeps the non-pinned path a clean no-op.
  Fail-closed on upstream sums fetch error. **Postmortem lesson:** the
  canonical `download.FetchChecksum` already existed in the repo with
  full test coverage but had zero production callers — scaffolding
  waiting for a use case. When closing a security gap, audit the
  existing helper surface before introducing new abstractions; the
  whole change was 47 lines because the right helper was already
  there.

- **`sec:ab9b764a:cred-as-string`** — done 2026-04-26 — PR #132, merge
  commits `44bf6d1` (initial) and `28c901a` (review delta). Tier H
  major (credentials). `GenerateInstallConfig` held the pull-secret
  bytes on the heap until GC because the `os.ReadFile` buffer flowed
  through `string(...)` before `TrimSpace`, materialising an immutable
  copy nothing could zeroize. Two-commit landing: first wired
  `bytes.TrimSpace` directly on the buffer and added a success-path
  zero loop. Reviewer flagged that error-path returns leaked, the loop
  duplicated `credentials.ProxmoxCredentials.Zeroize`'s pattern, and
  the new wipe was untested. Delta commit hoisted the zero loop into
  `internal/system.ZeroBytes` (with focused unit test), switched the
  call site to `defer system.ZeroBytes(pullSecret)` placed immediately
  after `os.ReadFile` (so every return path wipes), and added a
  doc-block on the `.backup` write explaining the on-disk lifecycle
  (rollback artifact gated by 0o600; full removal post-manifest is a
  separate item). **Postmortem lesson:** for buffer-lifetime
  invariants, `defer` is the canonical Go idiom — a success-path-only
  wipe is a footgun the reviewer caught immediately. Self-merge note:
  the reviewer for the delta couldn't be dispatched (subagent quota);
  the merge proceeded under user-instructed override per memory
  `feedback_merge_authority.md`.

- **`mod:bb81a5b0:use-range-int`** — done 2026-04-26 — PR #140, merge
  commit `3811fd4`. Tier H suggestion. Replaced the sole remaining
  classic counted `for i := 0; i < r.max; i++` loop in
  `internal/executor/ringbuf.go:58` with Go 1.22 range-over-int
  (`for i := range r.max`). One-line change; `r.max` is `int` set
  from `newRingWriter(constMaxLines=200)`, so range semantics match
  exactly. Aligns ringbuf with the form already adopted in
  `internal/distribution/okd/phase/iso_cleanup.go` and
  `internal/distribution/okd/dns/dns.go`.

- **`ux:b3356305:readme-flag-drift-deploy-options`** — done 2026-04-26
  — PR #141, merge commit `2518d98`. Tier H minor. README Usage block
  listed 5 of the 14 top-level commands; readers formed a wrong mental
  model (`addon`, `kubeconfig`, `status`, `releases`, `debug-bundle`
  hidden until they read `--help`). Added a single Reference line
  below the Usage block pointing to `docs/cli/okdctl.md` — the
  cobra-generated reference that already lists all 14. Chose the link
  over expanding the block: lower-maintenance, never drifts when new
  verbs land.

- **`smell:92553fff:sprintf-d-instead-of-itoa`** — done 2026-04-26 —
  PR #143, merge commit `24f5f11`. Tier H suggestion. Replaced
  `fmt.Sprintf("%d", x)` with `strconv.Itoa(x)` at six sites across
  three files: `internal/cli/status.go` (4 sites: masters / workers /
  total / degraded), `internal/cli/doctor.go` (1 site: busy port in
  preflight), `internal/distribution/okd/firewall/firewall.go` (1
  site: iptables `--dport`). All operands verified plain `int` —
  type-safe substitution at every site. Note: audit Evidence said
  `setup/firewall.go` but the actual path is
  `internal/distribution/okd/firewall/firewall.go`; the file moved
  between audit and now. Three additional candidates in
  `internal/tui/wizard/...` (`model_view.go:157`,
  `steps/review.go:230,409`) left for a follow-on sweep.

- **`sec:d9f7733e:input-path-not-prefix-checked`** — done 2026-04-26 —
  PR #138. Tier H minor (file-toctou). `runDebugBundle` opened the
  bundle output path with `O_CREATE|O_WRONLY|O_TRUNC|0o600` and no
  `O_NOFOLLOW`, so a planted symlink at `outPath` would redirect the
  bundle write. Mirrored the canonical pattern in
  `internal/cli/logging.go::openLogFile` (lines 19-33): `os.Lstat`
  refuses a symlink up front, then `os.OpenFile` adds
  `syscall.O_NOFOLLOW` to close the TOCTOU window. Flag delta from
  openLogFile is `O_TRUNC` (fresh bundle file) instead of `O_APPEND`
  (log file). Added `errors` and `syscall` to the import block.
  **Postmortem lesson:** when a canonical pattern already exists for
  the same hazard at a sibling call site, mirroring beats extracting —
  the two sites have different flag combinations and error message
  prefixes, so a shared helper would have hidden the meaningful delta
  behind an option struct.

- **`sec:de572c63:toctou-chmod`** — done 2026-04-26 — PR #139. Tier H
  minor (file-toctou). `validateAndRestartDnsmasq`'s restore closure
  did `system.CopyFile(backup, configPath)` then
  `os.Chmod(configPath, 0o644)`. The chmod was redundant — `CopyFile`
  delegates to `CopyFileMode` which sets the mode at file-open time —
  and `os.Chmod` follows symlinks on Linux, so a planted symlink at
  `/etc/dnsmasq.d/<name>` would have had its target's mode changed
  instead of the configured path. Roadmap evidence pointed at
  `dnsmasq.go` but the function lives in `dns.go:218-224`; corrected
  in commit. Considered `system.CopyFileMode(b, c, 0o644)` to force
  0o644 atomically — rejected because it would silently overwrite an
  operator's hardening (e.g., a 0o600 dnsmasq config). **Postmortem
  lesson:** dropping a redundant op is safer than replacing it. The
  follow-up chmod read like a guard rail but actually opened a
  symlink-redirect hole that the canonical helper had already closed.


- **`err:ae5b624c:ctx-timeout-loses-cluster-identity`** — done
  2026-04-26 — PR #137, merge commit `78e7fb5`. Tier H major.
  Wrapped both `MonitorInstallation` `DeadlineExceeded` paths (the
  `installDone` branch and the `ctx.Done` reap fallthrough) in
  `*errtypes.ClusterError{Msg, Err: ctx.Err()}`, mirroring
  `WaitForBootstrap`'s pattern in the same file. The Canceled
  branches stay bare `fmt.Errorf` so a SIGINT still resolves cleanly.
  `ctx.Err()` rides through `ClusterError.Unwrap`, so any caller's
  `errors.Is(err, context.DeadlineExceeded)` still matches.
  **Caveat / follow-up needed:** the audit's stated user-visible
  goal (exit 4 instead of 130 on install-budget exhaustion) is not
  yet achieved — `internal/cli/root.go:111` maps
  `errors.Is(err, context.DeadlineExceeded)` → 130 unconditionally,
  without checking `caughtSig`. This PR aligns the error type;
  gating the exit-code mapping on `caughtSig` is the missing
  follow-up worth filing as a separate roadmap item.
  **Postmortem lesson:** when a planner finds the audit's premise
  about caller behavior is wrong, document the partial-fix scope in
  the PR body rather than expanding into the caller — keeps PR
  scope tight and surfaces the follow-up cleanly.

- **`state:62cb8a95:destroy-init-without-state`** — done 2026-04-26
  — PR #136, merge commit `9b77906`. Tier H minor. Added
  `stateLockHint(dir string) error` — stats
  `<dir>/.terraform.tfstate.lock.info` and returns a typed
  `*errtypes.ConfigError` naming the lock path and dir when present.
  Called from `destroyInfrastructure` after `tf.Init` failure, so a
  stale-lock signal becomes an actionable message instead of a
  generic `ClusterError`. Never auto-unlocks. Two unit tests cover
  both branches.
  **Deviation from audit:** the suggested message referenced an
  okdctl `--force-unlock` flag that does not exist; the
  implementation references only the real `terraform force-unlock`
  CLI command. If a future okdctl-side flag lands, the message can
  incorporate it.

- **`state:368b892b:cleanup-tfstate-explicit-only-implicit`** —
  done 2026-04-26 — PR #135, merge commit `5a417c5`. Tier H minor.
  Hoisted the local `filesToRemove` slice in `cleanupTerraformEnv`
  to a package-level unexported `terraformFilesToRemove` var. Added
  `TestTerraformFilesToRemove_DoesNotIncludeTfstate` whose name and
  `t.Fatal` message both spell out the destroy-recoverability
  invariant — a future edit that adds `terraform.tfstate` to the
  slice now fails at test time instead of silently breaking destroy.
  Var stays unexported per MEMORY.md §scaffolding (test lives in
  same package). When `state:4c092fce:tf-state-backup-removed-on-success`
  lands, the assertion can extend to cover `.backup`.

- **`state:b38ec9cc:install-workers-targets-omitted`** — done
  2026-04-26 — PR #131, merge commit `d5b0eb9`. Tier H minor. Added
  `Targets: ["module.okd_cluster.proxmox_virtual_environment_vm.worker"]`
  to `StartWorkerVMs`'s `ApplyOptions`, mirroring the precaution
  already in `postinstall/bootstrap.go`. A stray hand-edit elsewhere
  in tfvars no longer rides along with the worker-start apply.
  Terraform's count-resource targeting expands an unindexed
  reference to all instances, so all workers still start. Four-line
  change with a comment block explaining the WHY.

- **`sub:0934cf1b:duplicate-runcaptured`** — done 2026-04-26 —
  PR #130, merge commit `05e97d1`. Tier H minor. The private
  `platform.runCaptured` was a verbatim structural duplicate of
  `system.RunCaptured`. Deleted it; routed the four call sites
  (`Install`, `Remove`, two `AddRepo` paths) through the canonical
  `system.RunCaptured`, matching every other caller in the repo.
  Trade-off: error prefix narrows from `dnf install: …` to
  `dnf: …` — the operation context is preserved via the surrounding
  `fmt.Errorf` chain at each caller; no other `system.RunCaptured`
  caller in the repo embeds args[0] either.
  **Postmortem lesson:** the planner initially proposed dropping
  the `bytes` import along with `runCaptured`, but `bytes.Contains`
  was still in use by the Debian `postCheck`. A pre-apply `grep`
  for every import-removal candidate would have caught this — caught
  during application instead. Add to the import-removal checklist.

- **`iac:e076e43c:curl-no-timeout`** — done 2026-04-26 — PR #129
  (bundled with `curl-no-tls-pin` and `gh-api-unauth-rate-limit`),
  merge commit `d9af1bf`. Tier H minor. Added `curl_safe()` wrapper
  in `scripts/install.sh` that pins
  `--connect-timeout 10 --max-time 120 --retry 2 --retry-connrefused`.
  The four asset downloads (archive + SHA256SUMS + sig + pem) now
  route through it; the GitHub API release-tag lookup keeps inline
  flags so it can carry a shorter 30s budget alongside the optional
  bearer header. Closes the indefinite-stall window on hung CDN or
  sigstore endpoints.

- **`iac:e076e43c:curl-no-tls-pin`** — done 2026-04-26 — PR #129
  (bundled with `curl-no-timeout` and `gh-api-unauth-rate-limit`),
  merge commit `d9af1bf`. Tier H suggestion. `curl_safe()` includes
  `--proto '=https'` and `--tlsv1.2`; the inline API call carries
  the same flags directly. All five active curl call sites in
  `scripts/install.sh` now enforce HTTPS-only and TLS 1.2 floor.
  Defense-in-depth on a sudo-tier installer.

- **`iac:e076e43c:gh-api-unauth-rate-limit`** — done 2026-04-26 —
  PR #129 (bundled with `curl-no-timeout` and `curl-no-tls-pin`),
  merge commit `d9af1bf`. Tier H suggestion. When `GITHUB_TOKEN`
  is set in the environment, the release-tag lookup now sends
  `Authorization: Bearer $GITHUB_TOKEN` (via a bash array so the
  header argument is only added when the variable is non-empty,
  shellcheck-safe). Lifts the GitHub API rate cap from 60 to 5 000
  req/hr/IP — relevant on shared CI runners. Improved `die` message
  hints at `VERSION=` pinning or `GITHUB_TOKEN` on lookup failure.
  `GITHUB_TOKEN` is documented in the script header comment.

- **`sec:35abd54e:input-url-scheme-not-checked`** — done 2026-04-26 —
  PR #128, merge commit `b1bf4e4`. Tier H major. Added
  `ProxmoxConfig.InsecureHTTP bool` (json `insecure_http,omitempty`)
  mirroring the existing `Insecure` TLS-skip flag, plus
  `FieldProxmoxInsecureHTTP` constant. `validateProxmoxConfig`
  (ScopeProvider, in ScopeAll, runs before any credential is built)
  refuses an `http://` Proxmox host unless the flag is set, with an
  error pointing at the YAML path: *"set provider.proxmox.insecure_http:
  true to opt in"*. The schemeless-host happy path is unchanged;
  `GetProxmoxCredentials:187` still adds `https://` to bare hostnames.
  Added a four-case matrix test (schemeless / bare hostname / http
  rejected / http accepted with flag) in
  `internal/credentials/proxmox_test.go`. Linter side-effects: the new
  third branch turned the `if/else` into a chain that gocritic preferred
  as a `switch`, and gofumpt re-aligned the struct tag column — both
  folded into the same commit. **Postmortem lesson:** the field-name
  choice (`InsecureHTTP` mirroring `Insecure`) was good pattern-matching
  — reviewer flagged it as the right shape on first round. Auditing for
  "do we have an existing analogue field" before naming a new knob saves
  bikeshedding and keeps the schema discoverable.

- **E6 — kube-vip probe TLS uses cluster CA after install** — done
  2026-04-26 — PR #124, merge commit `c421069`. Audit
  `sec:cfcdee2d:tls-insecure-vip-probe`. Added
  `httputil.NewWithCA(pool, timeout)` (RootCAs + MinVersion=TLS 1.2)
  and `httputil.KubeconfigCAPool(path)` (base64-decode
  `clusters[0].cluster.certificate-authority-data` into
  `*x509.CertPool` via stdlib `crypto/x509` + `sigs.k8s.io/yaml` —
  already in go.mod, no new dep). Both production callers
  (`postinstall/verify.go::verifyKubeVIPAPIHealth`,
  `postinstall/haproxy.go::RemoveHAProxy`) now load the CA from
  `<clusterDir>/auth/kubeconfig` and verify TLS against it.
  `NewInsecure` survives only as a fallback when `KubeconfigCAPool`
  errors — i.e., the genuine pre-install / VIP-not-yet-in-SANs window.
  Plumbing: added `clusterDir` parameter to both probe functions,
  `UpdateIngressOptions.WorkDir` field for the update-ingress flow, and
  a default-fill in `Provisioner.UpdateIngress`
  (`filepath.Join(p.projectRoot, "okd-install")`) so existing CLI call
  sites stay unchanged. Tests: `TestNewWithCA` checks RootCAs identity
  and MinVersion; `TestKubeconfigCAPool` synthesises a real ECDSA
  self-signed CA, encodes it as base64 PEM into a YAML kubeconfig, and
  asserts pool extraction plus error paths (missing file, no clusters).
  E4 first-attempt postmortem above (`-H` salt non-determinism) doesn't
  apply here — kubeconfig parse is deterministic. **Postmortem lesson:**
  the existing `// TLS verification is skipped because the VIP is not
  yet in cert SANs` comment was true at first-install time and stale by
  post-install time — comments that name a phase-specific invariant rot
  when the function gets reused across phases. The fix moves the
  invariant note to `NewInsecure` itself (where it is permanent) rather
  than the caller (where it isn't).

- **`sec:98723e5d:bashrc-chown-leak`** — done 2026-04-26 — PR #123,
  merge commit `2eb7396`. Tier H major. `addKubeconfigToBashrc` had two
  write paths: when `.bashrc` did not yet exist, `AtomicWriteString`
  was followed by `ChownToInvokingUser`; when `.bashrc` existed and was
  being appended, the chown was missing. Under the sudo re-exec model,
  `os.CreateTemp` + `os.Rename` produces a root-owned file, so every
  deploy after the first silently chown'd the user's `.bashrc` to root
  (the user could no longer edit their own bashrc without sudo).
  Two-line fix at `internal/distribution/okd/install/flux.go:139`:
  propagate the AtomicWrite error, then call `ChownToInvokingUser`
  unconditionally on the success path. `ChownToInvokingUser` is a
  no-op when not under sudo (`internal/system/elevation.go:50-53`), so
  the unconditional call is safe in non-sudo invocations. Audit of every
  other `AtomicWrite*` call site confirmed this was the only user-home
  file missing the chown — `version/updatecheck.go:151`,
  `credentials/envfile.go:78`, `config/loader.go:59`,
  `cli/kubeconfig.go:69+120` all run pre-elevation; the system-config
  sites in `setup/*` are intentionally root-owned; `WriteAsInvokingUser`
  already chains chown internally. **Postmortem lesson:** the
  new-file branch and the update-file branch had structurally different
  chown handling — easy to miss because tests ran the new-file branch
  and the update-file branch only fires on a re-deploy. Fix patterns
  that must apply to *every* write path want a single chained helper
  (which `WriteAsInvokingUser` already is for the simple case), not
  duplicated per-branch chown calls.

- **`tst:d9f7733e:debug-bundle-tar-no-test`** — done 2026-04-26 — PR #125,
  merge commit `7689169`. Tier H blocker. Added `internal/cli/debug_bundle_test.go`
  covering bundleConfig credential redaction (Proxmox `TokenID` absent, `***`
  present), bundleLogFile content, tarDirInto symlink-escape outcome (dual:
  `os.Root` blocks OR WalkDir skips dir symlink as non-regular), and the
  skip-must-gather manifest entry. Source-side: extracted inline section list
  into `collectSections` so the skip path is reachable without cobra.
  Caveat: the symlink test does not exercise `os.Root.Open` because
  WalkDir skips dir symlinks before that call — a follow-up could add a file
  symlink variant to actually trip the OpenRoot guard.
- **`tst:0f076161:destroy-confirm-cluster-untested`** — done 2026-04-26 —
  PR #126, merge commit `966a694`. Tier H blocker. Hoisted the `destroy.go`
  inline typo-guard at the old lines 82-94 into unexported
  `validateConfirmCluster(force bool, confirm, name string) error` so a
  cobra-free table test can exercise the four states (force-off short-circuit,
  empty confirm, mismatched confirm, correct confirm). Error messages
  preserved bit-for-bit so the related cleanup mirror item
  `tst:93957c53:cleanup-confirm-cluster-untested` can later hoist both sites
  into a shared `cli/confirm.go` without behaviour change.
- **`tst:41a9d4eb:redact-handler-no-test`** — done 2026-04-26 — PR #127,
  merge commit `7dfa0dc`. Tier H blocker (seam→`audit-observability`).
  Added `internal/logutil/redact_test.go` with eight tests covering the
  full RedactHandler contract: secret-key redaction, case-insensitive key
  matching, `slog.Group` recursion, `*url.URL` userinfo stripping
  (preserving username), nil `*url.URL` passthrough, the
  `interface{ Redacted() any }` opt-out, `WithAttrs` and `WithGroup`
  propagation. Source unchanged. Caveat surfaced during implementation:
  slog's JSON handler reflection-walks `*url.URL` struct fields rather
  than calling `(*url.URL).String()`, so cases (4)/(5) call the
  package-private `redactAttr` directly to test the handler's actual
  output value rather than the JSON renderer's interpretation.

- **M40 — Air-gap workstream removed** — done 2026-04-20. The entire
  L15 air-gap workstream (M21–M27, M33) was ripped out in one pass.
  Deleted: `internal/fetchplan/`, `internal/addon/mirror/`,
  `internal/cli/airgap.go` + test + `testdata/airgap/`,
  `internal/cli/doctor_airgap.go` + test, the `--airgap` flag on
  deploy/destroy/doctor, `OKDCTL_AIRGAP` / `OKDCTL_MIRROR_BASE` /
  `OKDCTL_SCOS_*` / `OKDCTL_BOOTSTRAP_OC_URL` / `OKDCTL_UPDATE_CHECK_URL`
  env vars, `Deployment.MirrorBase` + `Deployment.ToolVersions` config
  fields, `okd.WithAirgap`, `addon.MirrorableAddon` / `MirrorSpec` /
  `ChartRef`, `addon.Environment.Resolver`, `flux.resolveChartRef` +
  `flux.MirrorArtifacts`, `secretstore.MirrorArtifacts`,
  `addon.WithResolver`, `fetchplan.PickResolver` / `IsAirgap` /
  `ResolveMirrorBase`, both resolver chains, the 9 Plan builders, the
  9 Purpose constants, the 4 per-fetch Env*URL constants, the two
  scoping docs under `docs/superpowers/plans/`, the airgap doctor
  checks section in `docs/doctor-checks.md`. Kept: M22's
  `oc adm release extract --tools` path (it's the current OKD-binary
  fetch mechanism; the release-image URL is now a hardcoded constant,
  no resolver wrap) and M23's direct scos.json/fcos.json fetch (same
  deal — URLs inlined). Tool URLs (helm, sops, yq) moved from fetchplan
  overrides back to hardcoded template constants in
  `setup/tools.go`. `version.BackgroundCheck` signature reverted to
  `(ctx) <-chan CheckResult`; direct `api.github.com` call. M29 (GitHub
  Attestations) stays — it was an adjacent supply-chain item, not an
  L15 item. **Rationale:** after shipping M21–M27 + M33 and writing
  the M34 architectural review (PR #102), the conclusion was that
  okdctl should not own mirror configuration. Operators doing air-gap
  installs are sophisticated users who already own their staging
  pipeline (`oc-mirror`, `skopeo`, Harbor, zot, Hauler, whatever);
  okdctl's `MirrorBase` + rewrite-table + `airgap plan` emission was
  paternalism the user base didn't need. The `Plan`/`Resolver`
  abstraction was elegant but earned its keep only to support the
  rewrite logic being deleted; without air-gap it was empty ceremony.
  Net deletion: ~1,800 LOC of production code + 4 goldens + 2 scoping
  docs + 2 CLI reference entries. Build and test suite green after the
  sweep. **Postmortem lesson:** L15 (scoping doc, 2026-04-19) locked
  an architecture the product didn't want. The failure mode was
  premature commitment: a well-researched scoping doc became
  load-bearing for M21–M27 implementation weeks before any operator
  had deployed the feature. Next time: ship a one-knob MVP (a single
  per-purpose URL override, wizard-configurable) before designing a
  resolver chain. Zero operator feedback is a strong signal to keep
  the abstraction surface tiny.

- **M29 — GitHub Artifact Attestations for release binaries** — done
  PR #94, merged 2026-04-20. New `actions/attest-build-provenance@v4.1.0`
  step in `.github/workflows/release.yml` after goreleaser (SHA-pinned
  `a2bbfa25...`); permissions were pre-provisioned at lines 7-10 so no
  scope change. `subject-path` covers all four shipped artifact globs
  (`dist/okdctl_*.tar.gz`, `*.deb`, `*.rpm`, `SHA256SUMS`); SBOMs
  intentionally excluded per acceptance. Additive to the existing
  cosign + SLSA flows; `install.sh` untouched. README gains a one-line
  `gh attestation verify <file> --repo qxtaiba/okdctl` snippet next to
  the existing cosign block. Tagged release will publish attestations
  at `https://github.com/qxtaiba/okdctl/attestations/<n>`.
- **U1 — Cleanup phase leaves FCOS ISO cache** — closed 2026-04-18 as
  audit error; no code change required. Local `downloads/` cache is
  already removed by `internal/distribution/okd/cleanup/artifacts.go:84,92`
  in both `preserveConfig` branches. The real gap — remote Proxmox
  ISO cleanup — is now tracked as **U1b**.
- **U5 — `cmd.Run()` without `CommandContext` in two sites** — closed
  2026-04-18 as done-by-prior-work. Both sites already use
  `exec.CommandContext(ctx, ...)`: `internal/distribution/okd/setup/tools.go`
  builds every cmd with `CommandContext` across lines 112/193/210/223/227/248,
  and `internal/system/elevation.go:142` constructs its cmd via
  `exec.CommandContext(ctx, "sudo", "-n", "true")` — the `cmd.Run()` at
  line 146 is the method call on that ctx-aware cmd (the audit misread
  it). Fix landed as a side-effect of commit `65d8fce refactor(platform):
  thread ctx through PackageManager and tool version lookup`
  (elevation-refactor-and-hardening plan).
- **U3 — HTTP downloads have no retry** — done PR #49, merged 2026-04-18.
  `internal/download` now retries 5xx, 408, 429, and transport errors with
  exponential backoff (5s base, factor 2, jitter 0.5, 3 steps, 5-minute
  cap). 4xx and context cancellation fail fast. Retry helper kept local
  to `internal/download/retry.go` rather than importing `addon` so the
  package layering stays low-level → nothing. Attempt count and last
  error are logged on exhaustion.
- **U6 — `BuildOpaqueSecret` panics on YAML marshal error** — done PR #50,
  merged 2026-04-18. `addon.BuildOpaqueSecret` now returns
  `(string, error)`; secretstore and flux callers propagate. The two
  remaining `panic(err)` sites in `internal/addon/catalog/*` init paths
  are `addon.Register` duplicate-name guards — distinct from the
  YAML-marshal panic U6 addressed. Pre-existing doc drift in
  `docs/architecture/addons.md:109` (arg order shown reversed) left for
  a follow-up.
- **N2 — Wire `okdctl releases list/show`** — done PR #51, merged
  2026-04-18. New `internal/cli/releases.go` wires
  `releases.OKDVersionFetcher` to two subcommands. `list --channel
  stable|all --output text|json` uses `text/tabwriter` so alignment holds
  for long tags like `4.21.0-okd-scos.10`. `show <version>` matches by
  `Version` or `Tag` and prints via `tui.DottedKeyValueFull`. Both honour
  the fetcher's existing disk cache; neither is added to
  `rootRequiredCmds` (read-only commands).
- **N5 — `okdctl kubeconfig`** — done PR #55, merged 2026-04-18. New
  `internal/cli/kubeconfig.go` prints the post-install cluster kubeconfig
  to stdout (default), writes it to a file via `--output`, or merges it
  into the first `$KUBECONFIG` path (falling back to `~/.kube/config`)
  via `--merge`. Structural YAML merge through `sigs.k8s.io/yaml`
  deduplicates `clusters`/`users`/`contexts` by `.name`; `current-context`
  is only adopted when the destination has none. Read-only — not added
  to `rootRequiredCmds`.
- **N17 — Wizard review renders all fields** — done PR #56, merged
  2026-04-18. `internal/tui/wizard/steps/review.go` now renders
  `HostPrefix` and `StaticIP.DNS`, and displays the API VIP with an
  `(auto)` suffix when the user left it blank but static IPs are
  configured — derived via `netutil.DeriveVIPFromStaticIP` at render
  time so the review doesn't silently omit the effective value.
- **N18 — Gateway-in-CIDR wizard validation** — done PR #57, merged
  2026-04-18. New exported `config.ValidateGatewayInCIDR(gateway, cidr)`
  in `internal/config/validators.go`, invoked from
  `NetworkingStepDefinition.Validate` so users cannot advance past the
  networking wizard page when the gateway falls outside the machine
  CIDR. Defers to per-field validators when either input is empty or
  malformed so the user doesn't see two errors for one bad field.
- **N4 — `okdctl cleanup` standalone** — done PR #58, merged 2026-04-18.
  New `internal/cli/cleanup.go` runs the full cleanup phase
  (`cleanup.Execute` with `Kind=Full`) against a config without a
  destroy flow, reusing `phase.ResolveClusterVIP`, `phase.BaseOptions`,
  `phase.GetTerraformEnv`, and `phase.DefaultHAProxyConfigPath`. Named
  `cleanup` so the existing `rootRequiredCmds` entry at
  `internal/cli/elevation.go:23` drives the sudo re-exec gate; no
  elevation edits needed. `--yes`/`-y` skips the confirmation prompt.
- **N6 — `okdctl config show`** — done PR #59, merged 2026-04-18.
  New `internal/cli/config.go` adds a `config` parent with a `show`
  subcommand that prints the resolved YAML to stdout with Proxmox
  `TokenID` redacted to `***`. `Username`/`Password`/`APIToken` on
  `ProxmoxConfig` already carry `json:"-"` so they never marshal —
  `redactConfig` only needs to shallow-copy and scrub `TokenID`. Read-only,
  not added to `rootRequiredCmds`.
- **N7 — `okdctl completion`** — done PR #60, merged 2026-04-18. New
  `internal/cli/completion.go` exposes `okdctl completion
  <bash|zsh|fish|powershell>` via cobra's built-in generators
  (`GenBashCompletionV2`, `GenZshCompletion`, `GenFishCompletion`,
  `GenPowerShellCompletionWithDesc`). Cobra's auto-registered bare
  `completion` command is suppressed via
  `rootCmd.CompletionOptions.DisableDefaultCmd = true` in favour of one
  with activation docs. README install section gains per-shell
  one-liners.
- **N9 — `--log-level`/`--log-format`/`--log-file` flags** — done PR #65,
  merged 2026-04-18. Three persistent flags on `rootCmd` wire through a
  new `configureLogging` helper called from `PersistentPreRunE`, which
  calls `tui.ConfigureLoggers` to mutate `stdoutLogger`/`stderrLogger`
  in place (charmlog's `SetLevel`/`SetFormatter`/`SetOutput`).
  `--log-format=json` uses `charmlog.JSONFormatter` (NDJSON).
  `--log-file` opens with `O_CREATE|O_WRONLY|O_APPEND` mode 0600 and
  duplicates both streams via `io.MultiWriter`. `tui.ProgressBarsEnabled()`
  is the new predicate consulted by `internal/download/download.go`,
  gated on stderr TTY (where `progressbar.DefaultBytes` renders) AND
  non-JSON format. A follow-up fix commit corrected an initial version
  that probed stdout TTY — the gate was wrong because the bar writes
  to stderr.
- **N15 — Wizard collects `Deployment` fields** — done PR #66, merged
  2026-04-18. New "deployment options" section in the advanced step of
  `internal/tui/wizard/steps/advanced.go` captures `Debug`,
  `SkipDepsCheck`, `TerraformEnv`, and `AutoApprove`. Bool fields use
  the existing `FieldTypeSelect` + `valYes`/`valNo` + `wizard.SetBool`
  pattern; `TerraformEnv` is a text input validated by new exported
  `config.ValidateTerraformEnv` (accepts empty or a terraform-workspace
  identifier `^[A-Za-z_][A-Za-z0-9_-]*$`). Review step renders each
  with `skip` gates so the all-defaults common case stays clean. Blank
  `TerraformEnv` is semantically correct for "no override" — the
  `GetTerraformEnv` helper in `phase/paths.go` already supplies the
  `production` fallback at runtime.
- **N20 — Doctor-check reference doc** — done PR #67, merged 2026-04-18.
  New `docs/doctor-checks.md` documents all 9 preflight checks (`host
  os`, `root check`, `path`, `tools and packages`, `sudo`, `ssh public
  key`, `pull secret`, `disk space`, `host ports`) with what each
  checks, the exact fail/warn strings extracted from
  `internal/cli/doctor.go`, and concrete fix commands. The doctor
  command's `Long` help gains a one-line pointer to the doc. No check
  implementations were touched. The roadmap cited `doctor.go:16-24`
  for the check list but the actual registry is at `:98-108` — stale
  line reference, valid item.
- **M4 — OKD release URL override** — done PR #61, merged 2026-04-18.
  New `Deployment.OKDReleaseBaseURL` YAML field and
  `OKDCTL_OKD_RELEASE_URL` env var override the hardcoded GitHub
  release URL. Resolution order in `setup.ResolveReleaseBaseURL`:
  env > config > default
  `https://github.com/okd-project/okd/releases/download`.
  `strings.TrimRight` normalizes trailing slashes. Mirrors that
  preserve upstream path layout (`<base>/<version>/<filename>`) work
  out of the box; full URL-template overrides for non-standard mirror
  shapes are scoped to M5.
- **N23 — HTTP error surface** — done PR #62, merged 2026-04-18.
  `httpStatusError` now carries `Method` and a ≤256-byte `Body`
  excerpt; `Error()` emits `HTTP <status> <method> <url>: <body>`.
  `fetchToFile` reads up to 256 bytes via `io.LimitReader` before
  returning the error. `bodySnippet` strips non-printable bytes
  (defense against terminal escape sequences in an error body) and
  trims whitespace; no credential scrubbing — an earlier bare-token
  heuristic was dropped because it caught only one narrow case while
  missing JSON/JWT/prose embeddings, and logs are not shipped today.
  A key-based scrubber (`token=`, `password=`, etc.) is the right
  follow-up when M2 (debug-bundle) or `--log-file` persistence lands.
  Retry behaviour unchanged — `isRetryable` still switches on
  `httpErr.Status`.
- **N8 — Step timing + per-step deploy summary** — done PR #64,
  merged 2026-04-18. `StepResult` gains `StartedAt time.Time` and
  `Duration time.Duration`. `Orchestrator.executeStep` captures
  `time.Now()` before the skip/execute branch and sets both fields
  on all three return paths (skip, fail, success); new `Results()`
  getter returns a copy of the slice under `RLock`. Each phase's
  `Execute` returns `[]distribution.StepResult` alongside existing
  returns; `executeFullDeployment` concatenates across setup +
  install + postinstall and passes the slice to `PostDeploySummary`,
  which renders a "steps" section (step id | ok/skip/fail | duration)
  with a total row. Destroy not instrumented — it does not flow
  through the post-deploy summary. Unblocks L5 (Prometheus metrics),
  N10 (Ctrl-C partial-progress summary), and M1 (`okdctl status`).
- **N11 — `--yes`/`--force` parity on deploy and update-ingress** — done
  PR #68, merged 2026-04-19. `internal/cli/deploy.go` gains `--yes`/`-y`
  that sets `deployNonInteractive = true` at `runDeploy` entry so the
  flag surface matches destroy (`--force`/`-y`) and update-ingress
  (`--yes`/`-y`). `internal/cli/confirm.go:promptForConfirmation`
  short-circuits to `(false, nil)` when
  `term.IsTerminal(os.Stdin.Fd())` is false, preventing the prompt
  goroutine from dead-locking in CI or piped invocations. All three
  existing call sites (destroy, update-ingress main, update-ingress
  HostNetwork-conversion) inherit the fix through the shared helper.
  `//nolint:gosec // G115` matches the existing
  `internal/tui/wizard/model.go:163` suppression for the uintptr→int
  cast.
- **L13 — Auto-update version check on startup** — done PR #69, merged
  2026-04-19. New `internal/version/updatecheck.go` with
  `BackgroundCheck(ctx)` fires a goroutine that queries
  `/repos/qxtaiba/okdctl/releases/latest` under a 4s
  `context.WithTimeout`; results are cached 24h under
  `$UserCacheDir/okdctl/update-check.json` (atomic tmp + `os.Rename`,
  mode 0600). `OKDCTL_NO_UPDATE_CHECK=1` short-circuits before the
  goroutine starts. `internal/cli/root.go:execute()` fires the check
  before `rootCmd.ExecuteContext` and drains the buffered channel via
  a `select` with `time.After(100ms)` after the command returns —
  only on exit 0, so error paths stay clean. Non-2xx responses,
  non-semver current version, and cache I/O errors all fail silently
  via `slog.Debug`.
- **M13 — Typed error hierarchy** — done PR #70, merged 2026-04-18.
  New `internal/errtypes` package defines `ConfigError`, `NetworkError`,
  `ClusterError`, and `AuthError` as concrete pointer-receiver structs
  (`Msg string`, `Err error`) with `Error()` and `Unwrap()` so
  `errors.As`/`errors.Is` both work across the chain.
  `WrapValidation(*config.ValidationResult) error` bridges the existing
  validation output into `*ConfigError` without touching the
  `ValidationResult` API. Five exemplar sites wrapped in this PR
  (`loadConfig`, `Download`, `ValidateClusterAccess`, `ensureRoot`,
  envfile insecure-perms check). Broader migration across remaining
  phase/addon error returns is tracked as **M13b** so U4's dispatch
  eventually covers every error path.
- **U4 — Exit codes collapse everything to 0/1/130** — done PR #70,
  merged 2026-04-18. `internal/cli/root.go:execute()` now dispatches
  exit codes via `errors.As` against the `internal/errtypes` types:
  `ConfigError`=2, `NetworkError`=3, `ClusterError`=4, `AuthError`=5,
  other=1. SIGINT/SIGTERM still return 130 via the existing
  `ctx.Err()` guard. The exit-code table is documented in the
  `internal/cli` package doc (authoritative for operators and CI
  scripts); the `internal/errtypes` package doc mirrors the same
  mapping from the type-producer side. Coverage is partial until
  M13b lands — sites not yet wrapped fall through to exit 1.
- **N19 — Addon-specific docs in `docs/addons/`** — done PR #71,
  merged 2026-04-18. New `docs/addons/flux.md` and
  `docs/addons/secretstore.md` cover purpose, when-to-use, defaults,
  configuration, common failure modes, and uninstall behaviour. All
  defaults are quoted from `internal/addon/catalog/{flux,secretstore}/`
  with source line refs. The flux doc explicitly distinguishes the
  warn-on-query-fail vs fatal-zero-replicas paths in
  `Verify` (an earlier draft conflated them). README's addon-system
  sentence gains two per-addon links; no new addon section was created
  since one did not exist.
- **M17 — Architecture diagrams (Mermaid)** — done PR #72, merged
  2026-04-18. Inline Mermaid `flowchart` blocks added to the three
  existing `docs/architecture/*.md`: `phases.md` shows
  setup→install→postinstall plus the inverse destroy→cleanup path;
  `wizard.md` shows all 11 wizard steps with explicit conditional
  routing for `node-placement` (Proxmox-only) and `files` (OKD-only)
  via separate "step or skip" branches; `addons.md` mirrors the
  `Manager.InstallAll` loop including dep-failed skip and rollback on
  install/verify error. No new docs created.
- **N1 — Wire `okdctl addon list/install/uninstall/verify`** — done
  PR #73, merged 2026-04-18. New `internal/cli/addon.go` registers
  four subcommands. `list` enumerates `addon.All()` + `cfg.Addons` and
  prints a tabwriter table named `CONFIG-ENABLED` (with a footer
  pointing at `verify` for live cluster state — the column is config
  truth, not cluster truth). `install [name]` and `install --all` map
  to `Manager.InstallOne` / `InstallAll`; the `Long` text documents
  the asymmetric rollback (single = all-or-nothing, `--all` =
  per-addon continuation). `uninstall <name>` surfaces
  `Manager.Uninstall`'s dependency-block error verbatim. `verify`
  consumes a new `Manager.VerifyAll` shape — `([]VerifyResult, error)`
  — so the CLI can render NAME/STATUS rows without losing per-addon
  detail; aggregated error is replaced by a sentinel
  `N addon(s) failed verification` so cobra doesn't double-print.
  Elevation gate refactored: instead of adding `"addon"` to
  `rootRequiredCmds` (which would force sudo on `list`/`verify`), the
  mutating leaves carry `Annotations["requiresRoot"] = "true"`, and
  `requiresRoot` checks the leaf annotation before walking ancestors.
  Other commands (`deploy`, `destroy`, `cleanup`, `update-ingress`)
  unchanged.
- **N3 — `okdctl config validate` standalone** — done PR #74, merged
  2026-04-18. New `configValidateCmd` under the existing `config` parent
  (`internal/cli/config.go`) loads the config, prints
  `ValidationSummary(result)` (the same renderer the deploy flow uses),
  and returns `errtypes.WrapValidation(result)` — nil on success (exit 0),
  `*ConfigError` on failure (exit 2 via `root.exitCodeFor`). No new flag
  needed; the persistent `--config`/`-c` on rootCmd is inherited.
- **M5 — Tool binary versions / URLs overridable** — done PR #75, merged
  2026-04-18. New `Deployment.ToolVersions` YAML map
  (`ToolVersionOverride{Version, URLTemplate}`) plus env vars
  `OKDCTL_{HELM,SOPS,YQ}_{VERSION,URL}`. `ResolveToolURL` and
  `ResolveToolVersion` in
  `internal/distribution/okd/setup/artifacts.go` mirror M4's
  env > config > default resolution. URL templates use named `{version}`
  and `{arch}` placeholders substituted via `strings.NewReplacer` — safe
  with zero, one, or both placeholders. An earlier draft used
  `fmt.Sprintf`; it emitted `%!(EXTRA …)` on verbatim-URL overrides and
  was dropped in the review-round refactor. yq keeps the
  `/releases/latest/download/` GitHub redirect (the only path that
  resolves without a concrete tag), so its `Version` override is a silent
  no-op unless the operator also supplies a URLTemplate containing
  `{version}`. Scope is intentionally narrow: terraform, OS packages, and
  FCOS still hit upstream repos.
- **M16 — Autogenerated CLI reference** — done PR #76, merged 2026-04-18.
  New `cmd/okdctl-gen-docs` binary generates 18 Markdown files under
  `docs/cli/` via `doc.GenMarkdownTree`. Exported `cli.RootCmd()` in
  `internal/cli/docs.go` surfaces the package-private root tree for
  offline tooling. `DisableAutoGenTag = true` suppresses cobra's
  date-stamped footer so regeneration is deterministic — without it the
  drift check is a false-positive factory. Makefile `docs`/`docs-check`
  targets and the new CI `docs-go` job fail on drift, checking both
  tracked-file diff and `git ls-files --others` (so new subcommands can't
  land without updating docs). README gains a release-checklist section.
  Doctor command metadata split out of `doctor.go` (still Linux-only,
  untouched) into `doctor_cmd.go` (shared, no build tag) and
  `doctor_stub.go` (`!linux`) so the cobra tree is platform-consistent
  for doc gen — preserving the "doctor is Linux-only at runtime"
  invariant while fixing a macOS/Linux drift that would have made the
  drift check unresolvable.
- **N10 — Ctrl-C partial-progress summary + resume hint** — done PR #77,
  merged 2026-04-19. New `InterruptSummary` in `internal/cli/summary.go`
  reuses the N8 `StepResult` plumbing to render a partial-progress box
  plus "resume with okdctl deploy/destroy" hint. `executeFullDeployment`
  (helpers.go) and `runDestroy` (destroy.go) detect `errors.Is(err,
  context.Canceled)` and print the box before returning the bare
  cancellation error so root.go's `ctx.Err() != nil → exit 130` dispatch
  still fires. `destroy.Phase.Execute` and `okd.Provisioner.Destroy`
  widened to return `([]distribution.StepResult, error)` so destroy has
  the same step data the deploy summary already used.
- **N25 — Progress bars for long-running operations** — done PR #78,
  merged 2026-04-19. New `tui.StartSpinner(ctx, desc) func()` in
  `internal/tui/spinner.go` renders a stderr spinner gated on
  `tui.ProgressBarsEnabled()` (the N9 predicate). Terraform apply
  (`internal/infrastructure/proxmox/proxmox.go`), bootstrap wait, and
  install monitor (`internal/distribution/okd/install/monitor.go`) wrap
  their long-running call with start/stop. No new third-party dep —
  stdlib goroutine + 120ms ticker; `sync.Once` guards the stop closure;
  `ctx.Done()` is one of the select cases so the spinner exits on
  cancel. Determinate progress wasn't viable: terraform buffers output
  through `executor.Run`, and openshift-install emits unparseable
  log lines.
- **N16 — Wizard collects `FCOSIso` / `TokenID` / `AdditionalNetworks`** —
  done PR #79, merged 2026-04-19. Three new fields wired into the
  wizard: `token_id` text field in the Proxmox credentials section;
  `fcos_iso` storage-ref `FieldTypeSelect` populated by walking
  ISO-capable storage pools and listing volids with `.iso` suffix
  (proxmox_discovery.go uses go-proxmox `Storage(...).GetContent(...)`);
  `additional_networks` `FieldTypeMultiSelect` over discovered bridges,
  with a new `MultiSelectField` component (j/k cursor, space toggle).
  `parseAdditionalNetworks` is a bridge-keyed merge — hand-authored
  `Model` and `VLANTag` survive a wizard re-run (caught in code review).
  Discovery error path distinguishes "token-only credentials" from
  generic missing-credentials so users understand wizard discovery
  still requires password auth (token id is saved for deploy use with
  `PROXMOX_VE_API_TOKEN_SECRET`).
- **D1 — document go-proxmox v0.x abandonment plan** — closed 2026-04-19
  as done-by-prior-work. `CLAUDE.md:154-182` already contains a complete
  `## Dependencies` section covering the permissive-license rule
  (MIT/Apache-2.0/BSD only), the v0.x justification format with the
  go-proxmox v0.4.x entry and its ~200-LOC REST-only fallback, the
  GitHub-Actions SHA-pin expectation with version-trailer format, and
  the stdlib-first rule. Landed in commit `d69c36d refactor(repo):
  resolve 2026-04-18 audit findings (tiers a+b)`.
- **D3 — pin tool-install @latest references** — closed 2026-04-19 as
  done-by-prior-work. `Makefile:60` pins `air@v1.61.7`, `Makefile:80`
  pins `golangci-lint@v2.11.4`, `ci.yml:54` pins `govulncheck@v1.1.4`,
  `ci.yml:80` pins `yamlfmt@v0.14.0`. Zero `@latest` left in tool-install
  sites. Landed in commit `d69c36d`.
- **D4 — tighten terraform version floor in CI** — closed 2026-04-19 as
  done-by-prior-work. `ci.yml:89` is `terraform_version: "1.10.3"` (was
  `"1.10"`). Landed in commit `d69c36d`.
- **D5 — plan gorilla/websocket removal path** — closed 2026-04-19 as
  done-by-prior-work. `CLAUDE.md:167-171` documents non-reachability
  (okdctl's Go source contains zero `websocket` references outside
  `go.mod`/`go.sum`) and records the tracking signal for the
  go-proxmox → `coder/websocket` upstream bump so the transitive
  update lands without local code changes. Audit ledger
  `.claude/audits/resolved-2026-04-18.jsonl:75` records this as
  resolved under `task-21-D5`. Landed in commit `d69c36d`.
- **M14 — Correlation ID per deploy run** — done PR #82, merged
  2026-04-19. `uuid.NewString()` is minted at the top of `runDeploy`
  and `runDestroy` (before credential/config/wizard log lines) and
  pinned on the package-level charmlog loggers via new
  `tui.SetRunID`. Subsequent `tui.X` calls and every slog record from
  the provisioner's `SimpleLogger()` snapshot carry `run_id`
  automatically. `tui.RunID()` reads the pinned value back for the
  summary renderer; `PostDeploySummary` and `InterruptSummary` gained
  a `runID string` parameter and render it via `sb.kv("run_id",
  runID)`. `github.com/google/uuid` promoted from transitive (via
  go-proxmox) to direct require.
- **M13b — Complete errtypes migration across phase code** — done PR
  #83, merged 2026-04-19. Wraps every exported phase/addon boundary
  in `internal/distribution/okd/{setup,install,postinstall,destroy,
  cleanup}`, `internal/addon/manager.go`, and
  `internal/credentials/envfile.go` with the appropriate `errtypes.*`
  type. ~30 files touched, ~100 wrapping sites. `ctx.Err()` paths in
  `install/monitor.go` left as raw `fmt.Errorf` so
  `errors.Is(err, context.Canceled)` still resolves and root.go's
  exit-130 dispatch stays intact. U4's `errors.As` now routes the
  full failure surface to exit codes 2–5 instead of falling through
  to 1. Cleanup destroy paths are NonFatal steps so those wraps are
  belt-and-braces; every other site is a Fatal boundary. Sweep took
  three review rounds — gap narrowed from ~30 sites (round 1) → 14
  (round 2) → 9 (round 3), all addressed inline.
- **M12 — Generalize SecretStore beyond 1Password** — done PR #81,
  merged 2026-04-19. New package-private `provider` interface in
  `internal/addon/catalog/secretstore/providers.go` with three impls:
  `onepassword` (default, preserves existing behavior and file
  names), `vault` (full — `vault-token.txt` + ESO SecretStore CRD
  with token auth), `bitwarden` (full — Bitwarden Secrets Manager /
  Vaultwarden-compatible, requires an in-cluster
  `bitwarden-sdk-server` sidecar not provisioned by this addon).
  Install now applies both the provider's auth Secrets AND an ESO
  `SecretStore` CRD named `okdctl-secretstore` (previously only
  Opaque Secrets). `ValidateSettings` dispatches to the provider's
  validator so misconfig surfaces before `oc apply`.
  `onepassword_vaults` setting exposes the 1P vault map as CSV
  (`"homelab=1,shared=2"`) with default `"homelab=1"`; a structured
  key-value editor is tracked as N26. Design investigation returned
  M19 (typed decoder) and M20 (grouped wizard fields) as the
  follow-on items.
- **M19 — Typed addon settings via per-addon decoder method** — done
  PR #89, merged 2026-04-19. `ConfigurableAddon` grows
  `DecodeSettings(map[string]string) (any, error)`; `flux.Settings`
  and `secretstore.Settings` are the per-addon typed structs.
  `secretstore.Settings` carries three provider sub-structs
  (`OnePasswordSettings`, `VaultSettings`, `BitwardenSettings`);
  `DecodeSettings` populates only the sub-struct matching the active
  `Provider`, so `s.Bitwarden.OrganizationID == ""` is structurally
  scoped to the bitwarden provider — no more string-prefix matching.
  `Install` and `ValidateSettings` on both addons call `DecodeSettings`
  once at entry and operate on typed fields. Linter required two
  renames: `flux.FluxSettings` → `flux.Settings` and
  `secretstore.SecretStoreSettings` → `secretstore.Settings` (revive
  stutter); provider names (`onepassword`/`vault`/`bitwarden`) got
  package-private constants (goconst). Design choice B (added to
  `ConfigurableAddon` directly) over design choice A (sub-interface
  `TypedConfigurableAddon`) — repo has only two addon implementers,
  both in-tree, no external type assertions to break.
- **M20 — Grouped wizard fields for structured addon settings** — done
  PR #89, merged 2026-04-19. `addon.WizardField` grows an optional
  `Group string`. Secretstore's `WizardFields()` annotates each field
  with its provider group and surfaces 10 provider-specific settings
  that were previously absent from the hardcoded wizard. The
  `AddonsStepDefinition` at `internal/tui/wizard/steps/addons.go`
  splits the single "1password secret store" section into four
  sections — common (`enabled`, `provider` dropdown, `secrets_dir`),
  onepassword, vault, bitwarden — each with a group-level title.
  Approach A (static `SectionDefinition` entries) chosen over approach
  B (dynamic renderer walking `WizardProvider`) — every other wizard
  step is hand-authored; a dynamic renderer just for secretstore would
  create asymmetry. Optional per-group hiding based on the selected
  provider is deferred: `DataDrivenStep` has no per-section
  `ShouldShow` and plumbing one exceeds M20 scope. Group headers alone
  materially improve UX over the previous flat 2-field view. Flux
  unchanged.
- **M2 — `okdctl debug-bundle`** — done PR #90, merged 2026-04-19.
  New `internal/cli/debug_bundle.go` collects redacted config
  (via N6's `redactConfig`), the `--log-file` from N9, `oc adm
  must-gather` output, `terraform state list` (raw `terraform.tfstate`
  excluded — it carries Proxmox credentials), `okdctl doctor` output,
  and runtime/version metadata into a gzip tarball with a top-level
  `manifest.yaml`. Each section returns a `manifestEntry` instead of
  fatally erroring, so a partial bundle is still useful in the exact
  scenario (broken cluster) where bundles are needed most.
  Doctor collection is build-tag-split (`debug_bundle_doctor.go` /
  `debug_bundle_doctor_stub.go`), mirroring the `doctor_cmd.go` /
  `doctor_stub.go` pattern from M16. Must-gather is bounded by a
  5-minute context timeout and a `--skip-must-gather` flag; output
  is archived through `os.OpenRoot`-scoped reads so symlinks cannot
  redirect reads outside the temp dir (TOCTOU-safe). Bundle
  correlation id minted via `uuid.NewString()`; `github.com/google/uuid`
  promoted from indirect to direct in `go.mod` (M14 had left this
  drift). Not added to `rootRequiredCmds` — read-only collection.
  Review round 1 caught three issues: double `loadConfig` print,
  tar/gzip not deferred (truncation risk on mid-run failure), and
  the go.mod tidy drift; round 2 PASSed.
- **L14 — Coverage thresholds + codecov in CI** — done PR #85, merged
  2026-04-19. New `.github/scripts/coverage-check.sh` reads `coverage.out`
  and enforces per-package floors from `.github/coverage-floors.conf`
  (key=value, `*` is default, `total` gates the aggregate). All floors
  start at 0 so the scaffolding passes vacuously today; N12/N13 tighten
  specific packages one line at a time when tests land. Self-contained
  shell check chosen over codecov SaaS — no token, no third-party
  dashboard, acceptance's "or equivalent" allows the substitution.
- **D2 — evaluate progressbar swap for bubbles/progress** — done PR #86,
  merged 2026-04-19. Dropped `schollz/progressbar/v3` in favour of a
  ~60 LOC hand-rolled `io.WriteCloser` in
  `internal/download/progress.go` that reuses `tui.ProgressBarsEnabled()`
  and `golang.org/x/term` for width. Cleanup removes
  `mitchellh/colorstring` (2019-stale) and `chengxilo/virtualterm` from
  the transitive graph. `bubbles/v2/progress` swap was re-rejected as
  still-strictly-heavier (requires bubbletea Program); CLAUDE.md §Deps
  note updated from "Kept" to "Removed".
- **M1 — `okdctl status` / `describe`** — done PR #84, merged 2026-04-19.
  New `internal/cli/status.go` wires three subcommands. `status` prints
  API reachability (`oc get --raw /healthz`), node counts by role
  (master/worker from `node-role.kubernetes.io/*` labels via
  `oc get nodes -o json`), cluster-operator degraded count
  (`oc get clusteroperators --no-headers`), and addon VerifyAll results.
  `describe node <name>` and `describe addon <name>` drill into a single
  resource with tabwriter output. Reuses `phase.BasePhase` + executor
  via a new `OcOutput` one-shot helper added next to `OcPollOutput` in
  `phase/kubectl.go` (polling helper was the wrong shape for a one-shot
  describe). Read-only — not added to `rootRequiredCmds`.
- **M3 — `--dry-run` / `--plan` mode** — done PR #87, merged 2026-04-19.
  New `--dry-run` flag on `deploy`, `destroy`, and `update-ingress`.
  Re-exec-as-root gate in `cli/elevation.go` already probed
  `cmd.Flags().GetBool("dry-run")` — adding the flag on the three
  commands activates the bypass. New `terraform.Executor.PlanStreamed`
  wires terraform stdout/stderr directly to the terminal via
  `executor.RunInteractive`; new `proxmox.Provider.PlanOnly` does
  Init + PlanStreamed. Deploy dry-run renders a 31-entry step listing
  through new `DryRunSummary`. Plan failures wrap as
  `*errtypes.ConfigError` → exit 2 via the U4 taxonomy.
- **L5 — Prometheus metrics endpoint during deploy** — done PR #87,
  merged 2026-04-19. New `--metrics-addr :9090` on deploy starts an
  HTTP server serving `/metrics` in Prometheus text format for the
  lifetime of the run; disabled when empty. Four metric families —
  `okdctl_deploy_step_total` (counter, success/failure labels),
  `okdctl_deploy_step_duration_seconds` (histogram, 12 buckets),
  `okdctl_deploy_current_step` (gauge, set by StepStarted/StepFinished
  on the new `MetricsRecorder` interface), `okdctl_deploy_duration_seconds`
  (gauge). Hand-rolled text renderer in `internal/deploymetrics/`
  (no `prometheus/client_golang` dep — the four metrics don't justify
  ~15 transitive packages). Orchestrator `MetricsRecorder` interface
  with a no-op default means existing callers are unaffected;
  `BasePhase.Recorder` propagates through setup/install/postinstall.
- **N26 — TUI key-value map editor component** — done PR #92, merged
  2026-04-19. New `components.KeyValueField` in
  `internal/tui/wizard/components/key_value_field.go` renders a focused
  (key, value) table mirroring the `MultiSelectField` shape — `j/k`
  moves rows, `h/l` switches column, `a` adds, `d` deletes, `ctrl+e`
  toggles edit mode (the host `DataDrivenStep` consumes `enter`/`tab`/
  `shift+tab` for inter-field navigation, same constraint documented
  on `MultiSelectField`). `FieldDefinition` gains `Type:
  FieldTypeKeyValue` and `KVAsDelimitedString bool` — true = CSV
  `"k1=v1,k2=v2"`, false = YAML-map `"k1: v1\nk2: v2"`. Secretstore
  wizard's `secretstore_op_vaults` retrofit as the first consumer in
  CSV mode; `Default: "homelab=1"` round-trips unchanged so existing
  YAMLs keep working. Review round 1 flagged 11 findings (empty-key
  pair emission in `Value()`, sentinel-vs-dynamic error, redundant doc
  comments, dead `defaultValue` field, file-name snake_case, host-step
  key-consumption type doc, one-frame width drift in `addRow`,
  delimiter-round-trip doc on `Value`/`SetValue`, blink-cmd plumbing
  through `syncInputFocus`/`Focus`/`toggleEditMode`, 73→60-char commit
  subject) — all addressed in round 2. Develop merged 7 items
  (M19/M20/M2/M1/M3/L5/L14/D2) during this session; rebase caught the
  drift and moved the retrofit target onto M20's grouped
  `secretstore_op_vaults` field in the onepassword section rather than
  adding a duplicate-binding field in the earlier "common" layout.
- **U2 — Wizard never sets `Provider.Type`** — done PR #52, merged
  2026-04-18. `ProxmoxStepDefinition` replaced its `ShouldShow`
  catch-22 (hidden when `Provider.Type` was unset → step never ran →
  type stayed unset) with an `Apply` hook that assigns
  `config.ProviderProxmox`. Mirrors `DistributionStep.Apply` at
  `internal/tui/wizard/steps/distribution.go:236` and the Apply hooks
  already in `networking.go` / `files.go` / `node_placement.go`.
  Single-distribution assumption is encoded in the hook body; revisit
  if L1–L3 ever move out of Skipped.
- **U1b — Clean remote Proxmox FCOS ISO on destroy** — done PR #54,
  merged 2026-04-18. New `StepRemoveRemoteISO` in the destroy phase
  SSHs to the Proxmox host, enumerates `<isoDir>/fedora-coreos-*.iso`
  via `find -print0`, checks the running-VM set via `pvesh` per-vmid
  config queries, and removes the file only if no running VM
  references it. Shared SSH plumbing extracted to new `phase/ssh.go`
  (`ProxmoxBareHost`, `SSHRun`); `setup/upload.go` now uses the same
  helper. Safety layers: `validateISODir` rejects shell
  metacharacters / whitespace / quotes; `refuseUnsafeISOPath`
  restricts filenames to `<isoDir>/fedora-coreos-*.iso`; paths are
  single-quoted before shell interpolation. VM-reference scan walks
  device fields (`ide*`, `sata*`, `scsi*`, `virtio*`, `boot`,
  `bootdisk`) with `file=`-prefix strip and suffix match; fails
  closed on any pvesh error. New `--keep-isos` flag on `destroy`
  preserves the ISO for users chaining destroy → re-deploy. Four
  review rounds resolved 15+ findings (helper duplication, `rm`/
  `find` injection, wrong pvesh endpoint, fail-open parse, comment
  density).
- **N14 — `go vet ./...` in CI** — done PR #53, merged 2026-04-18.
  New `vet-go` job in `.github/workflows/ci.yml` mirrors
  `lint-go` / `build-go` (same pinned action SHAs, `go-version-file:
  go.mod`, `ubuntu-latest`). Closes the gap where `make vet` existed
  locally but CI never invoked it.
- **L15 — Air-gap feasibility + scoping doc** — superseded by **M40**
  (2026-04-20). L15 locked a `FetchPlan` + `Resolver` + `oc-mirror`-wrapper
  architecture that was subsequently ripped out. The scoping doc is
  deleted alongside the implementation. Retained here for roadmap
  archaeology; see M40 for the postmortem.
- **M6 — `DefaultBinDir` configurable (rootless support)** — done PR
  #93, merged 2026-04-20. New `DeploymentConfig.BinDir` YAML field and
  `OKDCTL_BIN_DIR` env var override the hardcoded `/usr/local/bin`,
  resolved via a new `phase.ResolveBinDir(cfg)` helper
  (env > config > default, mirroring M4's `ResolveReleaseBaseURL`
  pattern). `system.ExpandPath` is applied before validation so
  `~/bin` matches pull_secret / ssh_public_key ergonomics. Setup
  install sites (`InstallToolsToSystem`, `installBinaryToPath`) and
  the cleanup binary-removal path thread the resolved value through
  `okd.go:Prepare`, `cli/cleanup.go`, and `destroy/steps.go`; a shared
  `phase.BinDirOrDefault` replaces three copies of the zero-value
  fallback. `phase.PreflightBinDir` encapsulates the env-only
  resolution `main.preflight` uses (config isn't parsed at startup)
  so doctor's renamed `bin dir on path` check can compare against
  exactly what preflight chose. New doctor `bin dir` check probes
  existence and writability separately (stat errors reported with
  raw error); user-configured fail text makes the sudo re-exec
  semantics explicit (binaries are root-owned; chown to manage
  later). `resolveBinDirForDoctor` memoises the config load via
  `sync.OnceValue` and surfaces load failures via a detail suffix
  plus pass→warn demotion so a malformed YAML never reads as green.
  Elevation is **not** rewired: acceptance bullet 2 is satisfied
  machinery-only (ResolveBinDir / IsDirWritable / checkBinDir all
  ship) — a blanket re-exec skip would bypass sudo for
  deploy/destroy/cleanup/update-ingress, which also write to
  `/etc/haproxy`, `/etc/dnsmasq.d`, `/var/www/html` and run `dnf`.
  Future standalone `okdctl install-tools` subcommand is the
  correct home for the scoped skip. Seven review rounds resolved
  the full surface: round 3 (destroy cleanup.Options missing
  BinDir, setup zero-value), round 4 (`ResolveBinDir` bypassed
  `ValidateBinDir`, path-vs-default equality fragility,
  preflight/doctor contradiction when env set), round 5
  (missing-dir vs not-writable distinction, `checkPath` warn-gate
  on post-validation env, docs parametric fix snippet, tilde
  expansion, `PreflightBinDir` vs `checkPath` consistency), round
  6 (malformed-config demote, stat error branch, comment density),
  final PASS on round 7.

- **`sub:97cb8adf:no-cmd-env`** — done 2026-04-26 — PR #147, merge
  commit `f7091b3`. Tier H major (io-handling, seam→audit-security).
  `system.RunCaptured` built `exec.CommandContext` without setting
  `cmd.Env`, so the os/exec nil-Env contract forwarded the full parent
  environment to ~19 firewall/dnsmasq/packages/tools/systemd callsites.
  The canonical `executor.Executor` already filters env through
  `executor.DefaultEnvAllowlist` via `executor.FilterParentEnv`, and
  `cli/elevation.go` reuses the same exported helpers for the sudo
  re-exec path — `RunCaptured` was the lone second-tier hole. Fix is
  one line: `cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist)`
  immediately after `exec.CommandContext`. Signature unchanged; all
  callsites compile unaltered. Added `TestRunCaptured_EnvFiltered`
  guard: plant `OKDCTL_SECRET_CANARY` via `t.Setenv`, assert child
  `sh -c '[ -z "$OKDCTL_SECRET_CANARY" ] || exit 42'` returns 0 — the
  canary must not reach the child. Verified import-cycle safety
  (executor → system was already not imported; only `logutil` was
  shared). Reviewer PASS first round.

- **`sub:ae5b624c:bypass-canonical-executor`** — done 2026-04-26 —
  PR #148, merge commit `f9808f6`. Tier H minor (io-handling).
  `WaitForBootstrap` shelled out to `openshift-install` via raw
  `osExec.CommandContext` with `cmd.Stdout = os.Stdout` /
  `cmd.Stderr = os.Stderr`, bypassing three Executor amenities: env
  allowlist (via `e.buildEnv()`), the ring-buffered stderr tail that
  feeds `*ExitError.Stderr`, and the structured `exec: started` /
  `exec: completed` debug logs. Replaced with a single
  `p.Exec.RunStreamedChecked(ctx, "openshift-install", "wait-for",
  "bootstrap-complete", "--dir", clusterDir, "--log-level=debug")`.
  `RunStreamed` wires `cmd.Stdout = io.MultiWriter(e.Stdout, ringWriter)`
  / `cmd.Stderr = io.MultiWriter(e.Stderr, ringWriter)`, so live TTY
  streaming is unchanged for the user; the spinner lifecycle is
  preserved. Cancellation identity preserved through the swap because
  `RunStreamed`'s non-`*exec.ExitError` branch returns the wrapped
  ctx error verbatim, so `errors.Is(ctx.Err(), context.DeadlineExceeded)`
  and `context.Canceled` still fire on the receiving branches.
  `MonitorInstallation` deliberately untouched — its CSR-tick loop
  needs explicit `cmd.Process` retention for the `killInstall`+`reapTimer`
  pattern, and a separate roadmap item `sub:ae5b624c:no-cmd-env-install`
  covers it. `os` and `osExec "os/exec"` imports retained because
  `MonitorInstallation` still uses both. Reviewer PASS first round.
  6-line deletion, 1-line addition.

- **`state:b804b2ec:bootstrap-destroy-skip-tfvars-silent`** — done
  2026-04-26 — PR #149, merge commit `52c848b`. Tier H major
  (crash-recoverability). `CleanupBootstrap` returned `nil` when
  `terraform.tfvars` was missing, with only a Warn — and the caller
  in `postinstall/steps.go` unconditionally flipped
  `pctx.BootstrapCleaned = true` on `nil`. Result: if a previous
  cleanup wiped tfvars but the bootstrap VM was still running, the
  postinstall summary lied about the teardown. Adopted fix path (a)
  from the roadmap: a package-local exported sentinel
  `var ErrBootstrapTfvarsNotFound = errors.New("bootstrap cleanup
  skipped: terraform.tfvars not found")` returned bare (not wrapped)
  from `CleanupBootstrap` on the missing-tfvars branch, plus an
  `errors.Is` guard in the step's `Exec` body that returns `nil`
  without calling `pctx.Update` — `BootstrapCleaned` stays false, no
  spurious failure surfaced to the user, the existing
  `OnError: phase.WarnOnError` hook does not fire (Exec returns nil).
  Real terraform errors keep the `errtypes.ClusterError` wrap. No
  canonical `errtypes.RecoverableSkipped` exists yet (verified by
  reading `internal/errtypes/errtypes.go`); the nearest prior art is
  `cleanup.ErrKindNotSet` which uses the same package-local sentinel
  shape. Single caller of `CleanupBootstrap` (`postinstall/steps.go`),
  so no other contract broken. Reviewer PASS first round.

- **`sec:f55b9c27:input-path-not-prefix-checked`** — done 2026-04-26 —
  PR #151, merge commit `a8056b7`. Tier H minor (input-validation).
  `WriteEnvFile` passed the destination path straight to
  `system.AtomicWrite` with no `lstat` check; an attacker who could
  plant a symlink at the .env path before the rename could redirect
  the credential bytes (mode 0o600) to an attacker-chosen target.
  Mirrored the canonical `openLogFile` shape at `cli/logging.go:25-32`:
  `os.Lstat` first, refuse with `*errtypes.AuthError` if
  `info.Mode()&os.ModeSymlink != 0`; non-`NotExist` lstat errors also
  short-circuit (writing credentials to an unstat-able path is unsafe);
  `NotExist` falls through normally for the first-write case. New
  `TestWriteEnvFile_SymlinkRefused` locks the regression in (skips
  under root, mirroring the existing `TestLoadEnvFile_PermRefusal`
  guard at `envfile_test.go:131-132`). **Scope-down recorded in PR
  body:** the Fix's second clause (path-traversal validation in
  `EnvFilePath` to reject `--output=../../etc/...`) was deferred — it
  is a cobra-flag-validation concern symmetric across `saveConfig`,
  not a credentials-write concern. **Postmortem lesson:** when a
  Fix bundles a load-bearing security clause with an architectural
  one, scope the PR to the load-bearing clause and surface the
  deferred clause in the PR body. Splitting prevents the architectural
  decision from blocking the urgent fix; the deferred clause becomes a
  fresh roadmap item naturally on the next audit. Reviewer PASS first
  round.

- **`sec:35abd54e:cred-struct-bare-format`** — done 2026-04-26 —
  PR #150, merge commit `c28a3bd`. Tier H minor (credentials).
  `ProxmoxCredentials.String` hand-rolled a `fmt.Sprintf` masking only
  the four fields it remembered to list; a future secret field added
  to the struct (`ClientSecret`, `RefreshToken`, etc.) would have
  leaked through `%v` / `%s` with no compile-time signal. Added a
  private `redactedCredentials` struct holding only the six safe
  fields (`Endpoint`, `Username`, `Insecure`, `Source`,
  `EndpointFromConfig`, `ConfigCredentialsOverridden`) and implemented
  `Redacted() any` returning a populated value of it. The
  `logutil.redactAny` path at `logutil/redact.go:107` already
  type-switches on `interface{ Redacted() any }`, so any slog record
  carrying a `*ProxmoxCredentials` now hits the structural-redact arm
  instead of relying on the `String()` format string. `String()` /
  `GoString()` delegate to `Redacted()` via
  `fmt.Sprintf("%+v", c.Redacted())` so `%v`, `%s`, `%+v`, `%#v` all
  share one safe-field whitelist. Existing
  `TestProxmoxCredentials_StringMasks` still passes (the new `%+v`
  output still contains the non-secret fields it asserts on, and
  never contains the secret bytes). New
  `TestProxmoxCredentials_Redacted` asserts the type shape, every
  preserved field, and nil-receiver safety. **Postmortem lesson:**
  when the redactor protocol is an *interface* (`Redacted() any`),
  the type that owns the secret should *implement* it rather than
  every call site wrapping. Format-string discipline is per-call and
  fragile under refactoring; interface satisfaction is per-type and
  forces the developer to consciously add a new field to the safe
  whitelist. Reviewer PASS first round.

- **`sec:1e8ffb91:tls-insecure-vip-name`** — done 2026-04-26 —
  PR #152, merge commit `b501017`. Tier H suggestion (tls-network).
  `verifyKubeVIPAPIHealth` falls back to `httputil.NewInsecure` when
  the kubeconfig CA is unavailable because the VIP is not yet in the
  apiserver certificate SANs during the bootstrap-to-kube-vip
  transition. The function name did not encode that temporal
  precondition; a future caller at any other phase would inherit the
  TLS skip silently. Pure rename to `verifyKubeVIPAPIHealthBootstrap`
  with a tightened doc comment that names the bootstrap-phase
  contract and explicitly tells later-phase callers to use a verified
  client instead. Picked the rename option over extracting a
  `httputil.NewBootstrapInsecure` factory: the helper itself is
  correctly named for what it does (skip TLS), the misuse risk lives
  at the caller, and the function is unexported with a single
  intra-file caller — zero blast radius. The companion finding
  `sec:761e5126:tls-insecure-skip` for `haproxy.go` is intentionally
  separate and untouched. **Postmortem lesson:** when a function's
  safety contract is *temporal* (only safe during a specific phase),
  encode the precondition in the function name. A reviewer scanning
  call sites should see the constraint without having to read the
  doc.

- **`err:aa84670c:deadline-exit-code-not-gated-on-signal`** — done
  2026-04-26 — PR #156, merge commits `6aea3e8` (gate) + `9c825d0`
  (extract+test). Tier H major (cancellation-identity). Follow-up to
  `err:ae5b624c` (PR #137): that earlier fix wrapped
  MonitorInstallation's two timeout paths in
  `*errtypes.ClusterError{Err: ctx.Err()}` so error consumers saw a
  consistent shape. But `root.go::execute()` short-circuited on
  `errors.Is(err, context.Canceled || DeadlineExceeded)` BEFORE
  calling `exitCodeFor`, and `errors.Is` walks through
  `ClusterError.Unwrap()` — so a 60-minute install-budget exhaustion
  exited 130 (signal) instead of 4 (cluster) even though no signal
  was caught. Fix gated the short-circuit on `caughtSig.Load() != nil`,
  letting ClusterError-wrapped sentinels fall through to `exitCodeFor`'s
  typed-error mapping. Round-1 reviewer FAIL caught that the initial
  test only asserted `atomic.Value` stdlib semantics, not the actual
  130/143 branches; round-2 delta extracted the predicate into
  `signalExitCode(*atomic.Value, error) (int, bool)` and added a
  4-row table-driven test covering every caughtSig×err combination.
  **Postmortem lesson 1:** an `errors.Is` early-return on the
  signal-meaning sentinels is only safe when the signal-meaning
  pathway is *also* present — gate the sentinel match on the
  out-of-band signal channel, not just the error value, otherwise
  any wrapping of `ctx.Err()` becomes a silent exit-code reclassifier.
  **Postmortem lesson 2:** when a unit test asserts predicate
  semantics, prove it by *calling the predicate*. The first test
  exercised `atomic.Value` (stdlib) instead of the gate logic,
  which is exactly what the reviewer caught — extracting the
  predicate function made it trivially testable. Reviewer PASS
  second round.

- **`tst:93957c53:cleanup-confirm-cluster-untested`** — done
  2026-04-26 — PR #157, merge commit `60bc7bb`. Tier H major
  (destructive-untested). `runCleanup` and `runDestroy` carried
  nearly-verbatim `--yes` / `--confirm-cluster` typo guards with the
  same blast radius (services stopped, terraform state files removed,
  /usr/local/bin binaries deleted). The `cleanup.go` site had no
  tests; `destroy.go` had a `validateConfirmCluster` extraction and a
  test, but a divergence between the two sites in a future refactor
  would silently let scripted cleanups proceed against the wrong
  cluster. Fix hoisted the guard into
  `cli/confirm.go::confirmClusterMatches(force, confirm, name, verb)`,
  parameterising the verb so the existing operator-facing strings
  ("scripted cleanups", "refusing destroy", etc.) reproduce verbatim
  for both call sites. Deleted `validateConfirmCluster` from
  `destroy.go` and migrated its test to a new `confirm_test.go` with
  6 table rows covering the four logical branches across both verbs.
  Error type (`*errtypes.ConfigError`) preserved at both sites.
  **Postmortem lesson:** when two destructive sites duplicate the
  same guard, the test that anchors one site does not protect the
  other — extract the guard so a single unit-test row catches both.
  Verb-string parameterisation works cleanly when the existing
  messages already follow `<verb>s` plural / `<verb>` singular
  conventions; future verbs with irregular pluralisation would need
  to pass the pluralised form explicitly. Reviewer PASS first round.

- **`sec:8ea706f6:dl-yq-unpinned-no-checksum`** — done 2026-04-26 —
  PR #158, merge commit `60a1fec`. Tier H major (tls-network).
  `yqURLTemplate` used GitHub's `/releases/latest/download/`
  redirect with no checksum and no version pin, while helm and sops
  in the same file were both pinned and checksum-verified. A
  compromised yq release tag (or upstream-account compromise) would
  silently land in BinDir = /usr/local/bin and sit on PATH for every
  subsequent okdctl invocation and the operator's interactive shell.
  Fix pinned `yqVersion = "v4.45.1"` and embedded per-arch SHA-256
  constants (amd64, arm64) sourced from yq's official `checksums`
  release asset, wired through new `binaryToolMeta.checksumsByArch` →
  `binaryInstallSpec.embeddedChecksum` → `installBinary` switch.
  Helm/sops paths preserved unchanged via the first switch case.
  **Design note:** the obvious "fetch the checksums file at install
  time" approach (matching helm/sops) does not work for yq —
  `download.FetchChecksum` expects a two-column `<hash>  <filename>`
  format, but yq publishes a 31-column multi-algorithm matrix where
  SHA-256 sits in column 19. The choice was: extend the checksum
  parser to handle yq's matrix (heavier, more brittle, only one
  consumer) or embed the SHA-256 as a constant tied to `yqVersion`.
  Embedding chosen with a maintenance comment on the version
  constant. **Postmortem lesson:** "fetch a vendor-published
  checksums file" assumes the vendor's file is parseable by the
  shared helper. When it isn't, embedding the constant — with a
  clear maintenance pointer to the source URL — is a smaller and
  more auditable trust anchor than carrying a per-vendor parser.
  Reviewer PASS first round; flagged a defense-in-depth note that
  `checksumsByArch[unmapped-arch]` returns "" and skips verification
  — acceptable today since `platform.DownloadArch()` is constrained
  to amd64/arm64, but a future arch addition needs a corresponding
  map entry or the verification silently regresses.

- **`err:c287d5c0:vocab-ad-hoc-distribution-type`** — done 2026-04-26
  — PR #153, merge commit `eca0949`. Tier H minor (domain-vocabulary).
  `Provisioner.Validate` returned a bare `fmt.Errorf("invalid
  distribution type: …")` for a config-shape error, so
  `cli/root.go::exitCodeFor` fell through to exit 1 instead of the
  documented "config error → 2" contract from the `errtypes` package
  doc. Wrapped the error in `&errtypes.ConfigError{Msg:
  fmt.Sprintf(…)}`; the `%w` chain through the helpers.go re-wrap
  preserves identity for `errors.As`, so `exitCodeFor`'s
  `*errtypes.ConfigError` arm now matches and returns 2.
  **Postmortem lesson:** every error a CLI verb can return is a
  contract about an exit code. Bare `fmt.Errorf` at a typed-error
  boundary silently violates that contract — pre-merge, grep
  `fmt.Errorf` near `cli/root.go`'s exit-code-mapped errtypes and
  ask whether the surface error is one of the typed sentinels.
  Reviewer PASS first round.

- **`state:4c092fce:tf-state-backup-removed-on-success`** — done
  2026-04-26 — PR #154, merge commit `1105be4`. Tier H major
  (tf-state-atomicity). `Executor.Cleanup()` unconditionally deleted
  `terraform.tfstate.backup` alongside `tfplan` and `destroy.tfplan`
  after every successful apply or destroy, sweeping away the
  operator's only built-in rollback artefact if the live tfstate was
  later corrupted. Renamed the method to `CleanupPlans()`, dropped
  `.backup` from its file list, updated both call sites
  (`destroy/helpers.go`, `proxmox/proxmox.go`) and the existing
  `TestExecutor_Cleanup_PreservesTFState` assertion to expect
  `.backup` to survive. The Fix bullet allowed two shapes —
  CleanupPlans+CleanupBackup pair, OR keep `.backup` until the next
  successful run. Picked the latter (no `CleanupBackup()` method
  added) because an exported method with zero callers is dead-API
  per CLAUDE.md "don't introduce abstractions beyond what the task
  requires." The okdctl cleanup phase
  (`internal/distribution/okd/cleanup`) still removes `.backup` via
  its own `terraformFilesToRemove` list — that is operator-triggered
  and intentional. Reviewer round-1 FAIL was a false positive from
  grepping the main checkout (which still showed un-renamed callers
  on develop, not in the worktree); re-dispatched with diff-only
  scope → PASS. **Postmortem lesson 1:** when a roadmap Fix bullet
  proposes a method-pair (`X` + `Y`) but only one half has a real
  caller, the unused half is dead-API; implement the behavioral
  shape, not the literal proposal. **Postmortem lesson 2:** when
  briefing a code-reviewer agent about a worktree branch, scope it
  to the diff text, not to grep against the on-disk checkout — the
  on-disk state is the pre-merge develop branch and will produce
  false positives on rename-style refactors.

- **`obs:6424733c:string-concat-err-error-in-tui`** — done
  2026-04-26 — PR #155, merge commit `2b7d5b5`. Tier H minor
  (field-stability). Two `tui.Warn` calls in `cmd/okdctl/main.go`'s
  `preflight()` built their messages via `+`-string concatenation,
  collapsing the err's chain into the message body before
  `logutil.RedactHandler` saw it. Replaced both with structured
  `tui.LF("k", v)` attrs (`OKDCTL_BIN_DIR ignored` carries `value`
  + `err`; `failed to prepend bin dir to PATH` carries `bin_dir` +
  `err`). The second site now passes the `err` value directly
  rather than `err.Error()`, preserving type info for the
  `Redacted() any` interface path. **Postmortem lesson:** the
  message-vs-attr split is the redaction boundary. A `+ err.Error()`
  on a tui.Warn is structurally indistinguishable from a
  `fmt.Sprintf("…%v", err)`, and both sit upstream of
  `logutil.RedactHandler`. Codify the discipline as a `forbidigo`
  rule when the audit-observability finding is picked up.
  Reviewer PASS first round.

- **`mod:f55b9c27:use-builtin-clear`** — done 2026-04-26 — PR #159,
  merge commit `d141cd5`. Tier H minor (any-interface-builtins).
  `WriteEnvFile` zeroed the bytes.Buffer's backing store via a
  three-line `for i := range data { data[i] = 0 }` loop after the
  atomic write to disk. Swapped for the Go 1.21 `clear(data)`
  builtin: identical semantics on `[]byte`, signals "this is a
  wipe" at the call site (load-bearing on a credential-handling
  path), and preserves the existing two-line WHY comment unchanged.
  Net delta: -2 lines. Reviewer PASS first round.

- **`mod:35abd54e:use-builtin-clear`** — done 2026-04-26 — PR #160,
  merge commit `24a4c1f`. Tier H minor (any-interface-builtins).
  `ProxmoxCredentials.Zeroize` hand-rolled two
  `for i := range slice { slice[i] = 0 }` loops between the
  `c.Password = nil` / `c.APIToken = nil` assignments. Replaced
  each loop with `clear(c.Password)` / `clear(c.APIToken)`; the
  nil-assignments stay so the backing array can be GC'd. Net
  delta: -4 lines. Reviewer PASS first round.

- **`mod:7b2829bb:use-slices-containsfunc`** — done 2026-04-26 —
  PR #161, merge commit `6f8647c`. Tier H suggestion (slices-maps).
  `EnvAllowlist.allows` walked `a.Prefixes` with a hand-rolled
  `for/strings.HasPrefix` loop where `slices.ContainsFunc` is the
  canonical shape; the codebase already adopts it at
  `internal/cli/status.go` and `internal/logutil/redact.go`.
  Replaced the body with
  `return a.Exact[key] || slices.ContainsFunc(a.Prefixes, func(p string) bool { return strings.HasPrefix(key, p) })`
  and added `"slices"` to imports. Exact-then-prefix short-circuit
  preserved via `||`. Net delta: -7 LOC. Reviewer PASS first round.

- **`smell:2f70d7df:magic-default-port`** — done 2026-04-26 —
  PR #162, merge commit `6457aa4`. Tier H minor (magic-strings).
  `BuildIgnitionURLForNode` fell back to the literal `8080` when
  `cfg.HTTPServer.Port` was unset, duplicating the canonical
  `DefaultIgnitionPort = 8080` constant declared one file over in
  the same `package setup`'s `phase.go`. Replaced the literal with
  the named constant — no import needed (same package). One-line
  change, no behavioral diff. Reviewer PASS first round.

- **`smell:8aa632a6:duplicate-platform-string`** — done 2026-04-26
  — PR #163, merge commit `7ec51d0`. Tier H suggestion
  (helper-package-no-value). `debug_bundle.go` constructed the
  manifest's `Platform` via inline
  `fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)` even though
  `version.Platform` (computed once at package init from the same
  expression) was already imported. Replaced with
  `Platform: version.Platform`. The `runtime` import stays —
  `bundleSystemMeta` (lines 335-337) still reads `GOOS`, `GOARCH`,
  and `NumCPU()` directly. One-line change, byte-identical output.
  Reviewer PASS first round.

- **`ux:8d8faa80:completion-powershell-on-linux-only-tool`** —
  done 2026-04-26 — PR #164, merge commit `4600c24`. Tier H
  suggestion (verb-noun). `completionCmd` advertised `powershell`
  as a valid arg in `Use`, `ValidArgs`, the `Long` description,
  and the `runCompletion` switch — but okdctl is Linux-only
  (CLAUDE.md, README), so the powershell branch was dishonest help
  text. Dropped powershell from all four sites in `completion.go`,
  removed the `okdctl completion powershell | Out-String |
  Invoke-Expression` hint from README, and regenerated
  `docs/cli/okdctl_completion.md` via `make docs` (CI's
  `git diff --quiet docs/cli/` check stayed green).
  **Postmortem lesson:** when a roadmap fix touches a generated
  CLI doc, the planner's "either direct edit or `make docs`" caveat
  resolved cleanly here — `make docs` produced output identical to
  what hand-edits would have. The dual-path plan is overcautious for
  cobra-generated refs; future doc-touching items can drop the
  direct-edit fallback and trust `make docs`. Reviewer PASS first
  round.

- **`obs:48688e63:apply-failure-no-err-attr`** — done 2026-04-26 —
  PR #165, merge commit `4358b7e`. Tier H minor (field-stability).
  `Provider.Provision`'s terraform-apply-failed Warn carried only
  the user-facing recovery hint, not `applyErr` itself, so slog
  records had no `err` field for log-aggregation filtering. Added
  `"err", applyErr` as a structured attr alongside the existing
  message; the `fmt.Errorf("terraform apply failed: %w", applyErr)`
  wrap on the next line is unchanged so callers still receive the
  typed error. `applyErr` is a typed Go error with no credential
  content; `RedactHandler` only scrubs password/token/secret/api_key
  keys, so `"err"` passes through unmodified. Reviewer PASS first
  round.

- **`obs:c287d5c0:cleanup-warning-key-vague`** — done 2026-04-26 —
  PR #166, merge commit `48bde66`. Tier H suggestion
  (field-stability). `Provisioner.Prepare`'s post-cleanup Warn
  read `p.logger.Warn("cleanup warning", "err", err)` — the
  unkeyed message broke the cleanup package's `cleanup: <verb>
  <object>` convention and gave structured consumers no field
  identifying which operation failed. Replaced with
  `p.logger.Warn("cleanup: pre-deploy artifact removal incomplete", "phase", "prepare", "err", err)`,
  matching the orchestrator's `step` key style. One-line change.
  Reviewer PASS first round.

- **`con:e7db1220:releases-completion-bg-ctx`** — done 2026-04-26
  — PR #172, merge commit `0ddb998`. Tier H suggestion (ctx-todo).
  `releasesShowCmd.ValidArgsFunction` discarded its `*cobra.Command`
  parameter and called `fetcher.FetchVersions(context.Background())`,
  so a hung GitHub fetch during tab completion blocked the user's
  shell until the http client's own 4-second timeout expired —
  Ctrl-C had no effect. Renamed the closure's first parameter to
  `cmd` and replaced `context.Background()` with `cmd.Context()`,
  which is the signal-watched ctx installed by
  `internal/cli/root.go::execute()`. **Postmortem lesson:**
  the planner's first-pass YAML proposed deleting the
  `"context"` import alongside the closure change. Caught at
  apply time because `fetchFlatVersions(ctx context.Context)` at
  line 131 still needs the import. Reviewers can't see the full
  file the planner is reasoning about, so a "remove unused
  import" delta from the planner needs the orchestrator to verify
  every site, not just the one being edited. Reviewer PASS first
  round.

- **`obs:97cb8adf:waitfor-no-retry-count`** — done 2026-04-26 —
  PR #173, merge commit `b537ca7`. Tier H suggestion
  (span-retry-boundary). `WaitFor`'s ready/timeout span carried
  no iteration count — operators tailing structured logs saw
  "X is ready" / "timeout waiting for X" but not how many polls
  had fired. Added a `polls` counter declared before the
  pre-loop check, incremented at the top of the ticker case
  before `check()`, and surfaced as a `"polls"` attr on both
  `logger.Info(readyMsg, …)` sites plus an embedded `(%d polls)`
  in the timeout `fmt.Errorf`. The timeout uses string embedding
  rather than a structured attr because the timeout site returns
  an error, not a log call — `RedactHandler` operates on slog
  records, not error chains. Reviewer PASS first round.

- **`smell:9d79b841:strconv-fallback-to-zero`** — done
  2026-04-26 — PR #174, merge commit `570e6c8`. Tier H minor
  (stringified-numbers). `parseOKDMinor` discarded `fmt.Sscanf`'s
  err and treated minor==0 as a successful parse, so a malformed
  version string like `"4.x.0"` silently fell through to a fetch
  against `release-4.0/data/data/coreos/fcos.json` which 404s.
  Changed the signature to `(int, bool)` keyed off Sscanf's `n
  == 2` discriminant, and `DetectCoreOSVersion` now returns a
  typed `errtypes.ConfigError` early when ok is false. Test
  table updated for the new signature; new
  `TestDetectCoreOSVersion_malformedVersion` covers
  "not-a-version", "", and "x.y.0". **Postmortem lesson:** when
  `fmt.Sscanf`'s docstring promises "returns the number of
  items successfully parsed", treating that count as the
  validity discriminant is more honest than discarding the err
  and the result and hoping the zero-value happens to be
  invalid. Reviewer PASS first round.

- **`con:bdf5a873:safe-remove-ignores-ctx`** — done 2026-04-26
  — PR #175, merge commit `dda6440`. Tier H minor
  (ctx-ignored). `SafeRemoveWithLogger` accepted `_
  context.Context` and discarded it, so an `os.RemoveAll` against
  a hung NFS or stuck FUSE mount during destroy could stall
  indefinitely while the destroy ctx had a meaningful cancel
  point. Picked option (b) from the roadmap: rename `_` to
  `ctx`, add `if err := ctx.Err(); err != nil { return err }`
  immediately before the `os.RemoveAll`. Three lines added; no
  caller signature change because the parameter was already
  there (just discarded). The `os.Stat` upstream of the guard
  can also stall on a hung mount — out of scope for this fix
  per the roadmap, but flagged as a follow-up if the
  destroy-stall recurs. Reviewer PASS first round.

- **`obs:33579dd5:err-stringified-bypasses-handler`** — done
  2026-04-26 — PR #176, merge commit `bf1602e`. Tier H minor
  (field-stability). Four cleanup sites — `services.go:147`,
  `services.go:160`, `services.go:170`, `packages.go:79` —
  passed `guardErr.Error()` as the slog message rather than as
  an `"err"` attr. `RedactHandler` walks attr values for
  `Redacted() any` interface implementations and credential-key
  scrubbing; a flat message string is opaque to it. Replaced
  all four with `logger.Warn("cleanup: refusing critical
  path", "err", guardErr)` matching the canonical idiom at
  `artifacts.go:36` and `services.go:139`. **Postmortem
  lesson:** the canonical idiom search (in this case
  `git grep -n 'logger.Warn("cleanup'`) was load-bearing —
  finding the existing-idiom site in the same package gave
  the planner the exact message string and attr key shape, so
  the four edits collapsed to byte-identical patterns instead
  of four ad-hoc rewrites. Reviewer PASS first round.

- **`smell:830d4653:duplicate-os-fallback`** — done 2026-04-27
  — PR #177, merge commit `1147759`. Tier H minor
  (helper-package-no-value). The platform.Detect()
  warn-and-fallback pattern was duplicated in two phases:
  `cleanup/packages.go::detectOS` (called from
  `detectPackageManager` and `Apache`) and an inline block in
  `setup/phase.go::New`. Both warned via `logutil.OrNop(logger)`
  and fell back to `OS{Family: FamilyRHEL, ID: "unknown"}`.
  Hoisted into `platform.DetectOrDefault(*slog.Logger) OS` —
  the platform package gained an internal `logutil` import (no
  cycle: logutil only depends on stdlib). Removed the cleanup
  detectOS wrapper, migrated its three call sites, replaced the
  setup inline block, and dropped logutil from setup/phase.go's
  imports (no remaining use after the inline block went away).
  Unified warn message to "platform: detect failed; defaulting
  to rhel" (cleanup's prior phrasing). **Postmortem lesson:**
  when a planner removes a function, audit every import the
  function's package owns — logutil stays needed in
  cleanup/packages.go because `Packages()` still calls
  `logutil.OrNop`, but it goes away from setup/phase.go because
  the inline block was its only user. The Edit tool would not
  catch an unused-import bug; only `go build` does.
  Reviewer PASS first round.

- **`smell:d31d1b9d:role-string-instead-of-enum`** — done
  2026-04-27 — PR #178, merge commit `a8f0b40`. Tier H minor
  (magic-strings). `statusNode.role()` returned bare strings
  ("master" / "worker" / "unknown") and the
  `printClusterStatus` switch compared against the same
  literals — one rename upstream away from a silent drift.
  Changed `role()`'s return type to `phase.NodeRole`, added
  `phase.RoleUnknown` to the const block as a display-path
  sentinel, typed `nodeStatusEntry.Role` as `phase.NodeRole`,
  and made the switch arms `case phase.RoleMaster:` / `case
  phase.RoleWorker:`. JSON wire format preserved because
  `phase.NodeRole` is `type NodeRole string` with no custom
  marshaler — the underlying string serialises unchanged.
  `ParseNodeRole` deliberately still rejects "unknown" since it
  is not a value openshift-install ever produces; only the
  display path uses the sentinel. **Postmortem lesson:**
  enum-widening can stay scoped to the consuming path —
  parsers and emitters do not need to share the same accept
  set. If a planner proposes adding a sentinel to the enum,
  ask whether it should also flow into the parser; here the
  answer was no, and the asymmetry is load-bearing.
  Reviewer PASS first round.

- **`con:aa84670c:time-after-update-notice-ok`** — done
  2026-04-27 — PR #179, merge commit `3680eb0`. Tier H
  suggestion (time-sleep-retry). `printUpdateNotice` used a
  bare `<-time.After(100 * time.Millisecond)` in a select.
  Bare time.After leaks the underlying Timer until it fires;
  for this site (called once at process exit, 100ms cap) the
  cost is zero, but the pattern violates CLAUDE.md
  §Concurrency's canonical reapTimer reference. Replaced with
  `t := time.NewTimer(100 * time.Millisecond); defer t.Stop()`
  and switched the case to `<-t.C`. Two-line addition; no
  behavioral diff. **Note:** chose `defer t.Stop()` over the
  reapTimer's explicit `Stop()` on the win path because
  `printUpdateNotice` returns immediately after the select
  resolves, so defer fires as expected — reapTimer uses
  explicit Stop because it sits inside a for-loop where
  defer would not run per iteration. Reviewer PASS first round.

- **`obs:366b3f2d:step-completed-info-on-failure`** — done
  2026-04-27 — PR #180 (follow-up PR #191), merge commits
  `081d112` and `9f6bd22`. Tier H minor (level-discipline).
  `Orchestrator.executeStep` logged `step: completed` at Info
  for both success AND failure paths, so consumers using
  `--log-level=warn` filters silently missed every step
  failure. Branched the failure path on `step.IsFatal()`:
  fatal → Error, non-fatal → Warn, both with `"err", err`
  attrs so log-aggregation can filter by error class. Dropped
  the redundant `"success", false` attr from the failure path
  because the level itself now carries that signal. **Follow-up
  PR #191** completed the symmetry by renaming the success
  path message from `"step: completed"` to `"step: succeeded"`
  and dropping its `"success", true` attr — the planner
  initially preserved the old message "for backward
  compatibility for any log parsers keyed on the string,"
  which the user (correctly) rejected as unnecessary
  back-compat. **Postmortem lesson:** "preserve the old string
  for back-compat" is a reflex that needs a real consumer to
  justify it. okdctl has no published log-string contract, so
  the symmetry win (succeeded/failed naming pair) wins
  outright. Surface back-compat decisions in caveats so the
  user can reject them before merge. Reviewer PASS first round
  on both PRs.

- **`api:dd75bdeb:export-no-caller`** — done 2026-04-27 —
  PR #181, merge commit `2f47170`. Tier H minor
  (exported-surface). `PostInstallContext` was exported with a
  `//nolint:revive` directive that suppressed the stutter
  lint, but no caller outside the postinstall package
  referenced the type by name — it only flowed through
  `distribution.PhaseContext[T any]` as a type parameter, and
  generics happily hold unexported types. Lowercased to
  `postInstallContext`, dropped the nolint directive (and the
  doc comment that was solely a stutter-rename justification —
  no semantic content), updated the constructor call in
  `phase.go` and four `pctx.Update(func(c *...) { ... })`
  closures in `steps.go`. **Postmortem lesson:** a
  `//nolint:revive` whose justification text is "established
  internal API; rename deferred" is a deferred TODO, not a
  load-bearing suppression. Internal packages have no
  external contract — the rename should land the moment the
  audit notices it. Reviewer PASS first round.

- **`con:f5d703ab:install-tools-to-system-no-ctx`** — done
  2026-04-27 — PR #182, merge commit `d61bb63`. Tier H minor
  (ctx-ignored). `InstallToolsToSystem` accepted `_ context.Context`
  and looped over three multi-hundred-MB binaries calling
  `system.CopyFile` + `system.MakeExecutable`; a deploy ctx cancel
  was silently dropped. Renamed `_ → ctx` and added `if err :=
  ctx.Err(); err != nil { return err }` at the top of the loop body.
  `system.CopyFile` itself still has no ctx — mid-copy cancellation
  is bounded only by binary count, not file size. Filed as a known
  caveat for a later `system.CopyFile` ctx-aware refactor. Reviewer
  PASS first round.

- **`con:ab9b764a:validate-ignition-only-checks-ctx-once`** — done
  2026-04-27 — PR #183, merge commit `834afd0`. Tier H minor
  (ctx-ignored). `ValidateIgnitionFiles` checked `ctx.Err()` at
  function entry then iterated three files with `os.Stat` +
  `os.ReadFile` + `json.Unmarshal`; mid-loop cancellation was
  ignored. Inserted the canonical guard at the top of the loop
  body (3 LOC). Three-file loop bound + small (<1 MiB) file size
  means the practical leak was short, but the pattern was the same
  shape CLAUDE.md §concurrency calls out for retry sleeps. Reviewer
  PASS first round.

- **`api:0934cf1b:should-be-exported`** — done 2026-04-27 — PR #184,
  merge commit `935efb2`. Tier H minor (exported-surface;
  helper-package-no-value). `platform.runCaptured` had already been
  removed in a prior commit; only `dpkgArch` (single caller, captures
  stdout via `exec.CommandContext.Output()`) remained. `system.RunCaptured`
  cannot substitute because it discards stdout, so inlined the
  two-line `dpkg --print-architecture` body directly into `AddRepo`
  and deleted the helper. **Postmortem lesson:** roadmap evidence
  drifts within hours during multi-agent runs — the planner's first
  inspection found the audit's primary target (`runCaptured`) already
  gone. Audit findings should grep current state before committing
  to a plan; the salvageable subset was preserved by adapting the
  fix to the surviving helper. Reviewer PASS first round.

- **`ux:6424733c:no-tty-prompt-returns-false-silently`** — done
  2026-04-27 — PR #186, merge commit `e41f86f`. Tier H minor
  (signals). `promptForConfirmation` returned `(false, nil)` when
  stdin was not a TTY, indistinguishable from a user typing 'n'.
  CI scripts piping to a destructive command silently aborted with
  "cancelled" instead of failing fast. Returns
  `&errtypes.ConfigError{Msg: "no TTY and --yes not set; refusing
  destructive op"}` on the no-TTY+no-yes path; existing callers
  (destroy/cleanup/update-ingress) already check `if err != nil`
  before consulting `confirmed`, so the typed error propagates to
  exit code 2 via `cli/root.go`'s `exitCodeFor`. The inner
  `ConfirmConversion` closure (`func([]string) bool`, cannot
  propagate errors) surfaces the message via `tui.Warn` before
  returning false. Reviewer PASS first round.

- **`ux:0f076161:destroy-force-deprecated-but-still-default-binding`**
  — done 2026-04-27 — PRs #185 + #192, merge commits `a2ecca4` +
  `8bfd698`. Tier H minor (flag-conventions). `--yes` and `--force`
  were both bound to a single `*bool` (`destroyForce`); cobra's
  last-write-wins meant `--yes=false --force=true` and
  `--yes=true --force=false` both yielded `true`. PR #185 split the
  binding (separate `destroyYes` var, `effective := destroyYes ||
  destroyForce`); PR #192 followed the maintainer's "no back-compat"
  call and dropped `--force` entirely (one minor cycle compressed
  to zero — okdctl is pre-1.0 and has no shipped users requiring
  the alias). **Postmortem lesson:** the planner's instinct to
  preserve a deprecated alias for a release cycle is correct for
  shipping software but wrong for pre-1.0 internal tools — surface
  the pre-1.0 status at planner-prompt time so the default plan
  drops shims rather than adding them. Reviewer PASS first round.

- **`ux:08c49fc4:remove-haproxy-no-x-bool-default-true`** — done
  2026-04-27 — PRs #187 + #193, merge commits `d16ff51` + `f90a8f0`.
  Tier H minor (flag-conventions). `--remove-haproxy=true` default
  meant the only opt-out was `--remove-haproxy=false` — a
  no-X-style boolean masquerading as a positive. PR #187 added
  `--keep-haproxy` (default false) as the canonical opt-out and
  kept `--remove-haproxy` as a `MarkDeprecated` alias with
  `effectiveRemoveHAProxy := updateIngressRemoveHAProxy &&
  !updateIngressKeepHAProxy`; PR #193 dropped the deprecated alias
  entirely per the same maintainer call as `ux:0f076161`. Final
  state: `--keep-haproxy` is the only flag, `!updateIngressKeepHAProxy`
  drives the dry-run print, the warn message, and
  `RemoveHAProxy` postinstall option. Same pre-1.0 lesson as the
  destroy-force pair. Reviewer PASS first round.

- **`obs:660d83a5:run-id-mutation-race`** — done 2026-04-27 —
  PR #188, merge commit `f658dc6`. Tier H minor (handler-setup,
  seam→audit-concurrency). `SetRunID` rebound package-level
  `stdoutLogger`, `stderrLogger`, `stderrSlog`, and `runID` with
  plain `=` operators. Readers (tui.Debug/Info/Warn/Error,
  buildStderrSlog, SimpleLogger, ConfigureLoggers) had no
  happens-before edge against the writer. Wrapped all four vars in
  `sync/atomic.Pointer[T]`, moved initial values to package
  `init()`, swapped reads to `.Load()` and writes to `.Store()`.
  ConfigureLoggers' in-place SetLevel/SetFormatter/SetOutput calls
  on the loaded `*charmlog.Logger` are still covered by charmlog's
  own internal mu — no further synchronization required.
  `logger_test.go` had three direct assignments (sed-replaced to
  `.Store(...)`). RunID() guards against nil Load result with `""`
  fallback. Race-detector tests pass. Reviewer PASS first round.

- **`obs:ed55ee90:summary-keys-leading-whitespace`** — done
  2026-04-27 — PR #189, merge commit `b18d55e`. Tier H suggestion
  (field-stability). Six summary log lines in `printSummary` used
  leading whitespace inside the message string ("  work directory:
  clean") for visual indentation; under JSON formatter the indent
  becomes part of the `msg` field, breaking parser keys. Rewrote
  all six to terse `cleanup: <verb> <object>` messages with
  structured `files`/`count`/`size` attrs (matching the rest of
  the cleanup package's idiom). The "(0 files)" parenthetical was
  promoted to a `files` attr so JSON consumers gain a queryable
  field. **Postmortem note:** TTY visual output flattens (no more
  two-space indent under section header); the styled charmlog
  formatter doesn't re-indent. If indent is wanted later, push
  it to the formatter, not the message string. Reviewer PASS
  first round.

- **`con:6424733c:metrics-shutdown-bg-ctx`** — done 2026-04-27 —
  PR #202, merge commit `481f2b8`. Tier H suggestion (ctx-todo).
  `startMetricsServer`'s stop closure builds `shutCtx` with
  `context.Background()` rather than the caller's ctx. The
  Background choice is correct — by the time `stop()` runs, the
  parent ctx is already cancelled by SIGINT and we need the 5s
  graceful drain to complete — but CLAUDE.md §concurrency
  requires every production `context.Background()` to carry a
  justification comment. Added a 2-line WHY comment above the
  `WithTimeout(Background(), ...)` call, mirroring monitor.go's
  reapTimer style. Pure documentation; zero behavior change.
  Reviewer PASS first round.

- **`err:9d79b841:fcos-stream-status-bare`** — done 2026-04-27 —
  PR #203, merge commit `f0a47fb`. Tier H suggestion (wrapping).
  `fetchCoreOSStream` returned a bare
  `fmt.Errorf("coreos stream: HTTP %d", ...)` for non-200 — opaque
  to `errors.As`, drifting from the rest of the download layer
  which already had the typed `httpStatusError` for this exact
  shape. Renamed `httpStatusError` → `HTTPStatusError` (pure
  capitalization; no field/behavior change), updated download.go's
  constructor + retry.go's `errors.As` var, and rewrote the coreos
  fetch to `fmt.Errorf("coreos stream: %w", &download.HTTPStatusError{...})`.
  `%w` wrap means the chain unwraps through the surrounding
  `errtypes.ClusterError` so callers can `errors.As` end-to-end.
  Body field stays empty on the coreos path (body only read on
  200 for JSON parsing); could be enriched later. Reviewer PASS
  first round.

- **`err:a55b4592:vocab-ad-hoc-config-perm`** — done 2026-04-27 —
  PR #204, merge commit `af31d2c`. Tier H minor
  (domain-vocabulary). `Loader.LoadFile` returned bare `fmt.Errorf`
  for stat/perm/read/parse failures while
  `internal/credentials/envfile.go` already returned
  `*errtypes.AuthError{Err: os.ErrPermission}` for the analogous
  perm check. Two security-critical perm checks shipped two
  different error shapes — one mapped to exit 1, one to exit 5.
  Typed insecure-perm as `AuthError(ErrPermission)`; stat/read/
  parse as `ConfigError(err)`; schema-version drift as `ConfigError`
  with no inner sentinel. **Side effect:** removing
  `errtypes.WrapValidation` was required to break the new
  `config→errtypes` import edge — `WrapValidation` referenced
  `*config.ValidationResult`, so adding `errtypes` to loader.go's
  imports would have created a `config↔errtypes` cycle. Inlined
  WrapValidation's sole call site (`cli/config.go:47`) into a
  3-line `if !result.IsValid() { return ConfigError{Msg:
  result.Error()} }` block. **Postmortem lesson:** when one
  package's helper grows to import another package's type, that
  helper has chosen sides — moving it back across the import edge
  is cheaper than deferring the cycle. Reviewer PASS first round.

- **`err:5013fea6:str-sniff-tool-msg`** — done 2026-04-27 —
  PR #205, merge commit `1ea7374`. Tier H minor (string-sniffing).
  `isAuthError(stderr)` was the SOLE gate for AuthError vs
  ClusterError on `oc adm release extract` failures. oc's
  auth-failure wording drifts across minor versions, so a future
  oc could fall through silently OR a non-auth failure whose
  stderr happens to mention "401" could misfire. Made exit code
  the **primary** signal: `errors.As(runErr, &exec.ExitError{})`
  + `ExitCode() ∈ {1, 125}` AND `isAuthError(msg)` for AuthError;
  everything else → ClusterError. The TODO references
  `err:5013fea6` and notes the heuristic exit-code set; widen if
  upstream oc changes its exit-code contract. Note: the command
  runs through bare `os/exec.CommandContext`, not
  `executor.Executor`, so the asserted type is `*exec.ExitError`.
  If a future PR migrates this call site to `executor.Executor`,
  the type assertion needs updating. Reviewer PASS first round.

- **`ux:fd2125dd:addon-uninstall-no-confirm`** — done 2026-04-27
  — PR #206, merge commit `9a6ab6d`. Tier H major (verb-noun).
  `okdctl addon uninstall` deletes manifests, namespaces, and
  secrets but had no confirmation gate while sibling destructive
  verbs (destroy, cleanup, update-ingress) all gate on a TTY
  prompt or `--yes`+`--confirm-cluster` pair. Mirrored destroy.go's
  two-phase guard: `confirmClusterMatches(yes, confirm,
  cfg.Cluster.Name, "uninstall")` errors when `--yes` lacks a
  matching `--confirm-cluster`; otherwise `promptForConfirmation`
  reads y/N from TTY. `tui.Warn` summarizes the addon+cluster
  before the gate. Non-TTY without `--yes` returns nil
  (cancelled) per existing destroy/cleanup behavior — the
  systemic non-TTY refusal lands via `ux:6424733c` (separate
  in-flight PR), so all four destructive verbs harden in one
  shot. Reviewer PASS first round.

- **`iac:18a795d5:network-device-ignored`** — done 2026-04-27 —
  PR #209, merge commit `3bcf7c5`. Tier H suggestion
  (hcl-destroy-ordering). All three VM resources
  (bootstrap/master/worker) had `network_device,` in
  `ignore_changes` with no rationale, so an auditor or future
  Terraform upgrade reviewer could not tell the entry from stale
  scaffolding. Added a 2-line `#`-prefixed comment above each
  `network_device,` matching the existing `efi_disk` rationale
  shape, naming the upstream cause (bpg/terraform-provider-proxmox
  emits spurious diffs when static + dynamic network_device
  coexist) and the consequence. **Postmortem lesson:** when a
  workaround sits in a lifecycle/ignore_changes block, attach the
  upstream-bug rationale at the call site — without it, future
  cleanup passes will treat the entry as cruft. Reviewer PASS
  first round.

- **`smell:d9f7733e:stringly-typed-status-enum`** — done
  2026-04-27 — PR #210, merge commit `4e97662`. Tier H minor
  (magic-strings). `manifestEntry.Status` was free-form `string`;
  the codebase only ever assigned `"ok"`, `"skipped"`, `"failed"`
  but a typo at any of 24 sites silently shipped a malformed
  manifest.yaml — the support engineer's first read on a debug
  bundle. Introduced `type bundleStatus string` + three constants
  (`bundleStatusOK`/`Skipped`/`Failed`), retyped the field, and
  swept all 24 assignments. Mirrors the existing
  `stepDisplayStatus` shape in `cli/summary.go`. Wire format
  byte-identical because Go marshals named string types as plain
  strings; existing tests compare typed field against untyped
  string literals (Go-spec-compliant). **Postmortem lesson:**
  named-string enums cost zero in Go (no runtime overhead, no
  wire-format change, untyped literal compares still work), so
  the bar for promoting a `string` field to a named type is low
  whenever the value space is fixed. Reviewer PASS first round.

- **`api:2c4d8e6b:should-be-exported`** — done 2026-04-27 —
  PR #211, merge commit `9837584`. Tier H minor
  (exported-surface). `addon.AddonInfo` carried a
  `//nolint:revive` "stutter is established public API" suppression
  but the package lives under `internal/`, where rename has no
  external-API cost. Renamed to `addon.Metadata` (avoids the
  `Info() Info` method/type collision that `addon.Info` would
  produce) and dropped the nolint. Three files touched, no public
  surface affected. **Postmortem lesson:** //nolint suppressions
  on internal/ types should be reviewed against the actual import
  graph — internal/ has no consumers outside the module, so the
  "breaking change" framing is wrong. The right mental model is:
  treat internal/ stutter the same as any other package-private
  rename. Reviewer PASS first round.

- **`smell:1e8ffb91:repeated-port-literal`** — done 2026-04-27 —
  PR #207, merge commit `fa313e6`. Tier H minor (magic-strings).
  Port 6443 (kube-apiserver) was repeated as a raw literal across
  postinstall verify/haproxy/steps, setup haproxy, and firewall
  package — 9 sites total (the roadmap evidence count of 7 was
  off; grep found 9). Added `const KubeAPIPort = 6443` to
  `phase/paths.go` (canonical phase-level constant location) and
  swept all 9 sites; firewall picked up a new `phase` import (no
  cycle: phase does not import firewall). **Postmortem lesson:**
  introducing a constant where a literal repeats across ≥3 call
  sites pays for itself even if the value never changes — it
  documents the *role* of the number, which `6443` does not. The
  audit-claimed count was an undercount; always verify with
  `grep -rn` rather than trusting the roadmap evidence list.
  Reviewer PASS first round.

- **`ux:e7db1220:json-flag-shorthand-collision-risk`** — done
  2026-04-27 — PR #212, merge commit `2c52d65`. Tier H minor
  (flag-conventions). `releases list/show`, `status`, and
  `describe node/addon` all bound `--format` to a `-F` shorthand,
  which collided with the kubectl/docker/gh `-o` muscle memory
  and confused the existing `-o output-path` semantics on
  deploy/debug-bundle/kubeconfig. Dropped `-F` (lower blast
  radius than re-purposing `-o`) on all 5 sites; regenerated
  docs/cli/ via `make docs`; updated the `-F json` prose mention
  in `docs/cli/json-schema.md`. **Postmortem lesson:**
  flag-shorthand decisions should anchor on the ecosystem's
  prevailing convention (kubectl/docker/gh `-o`) before inventing
  local shorthand; `-F` was unique-but-confusing. Reviewer PASS
  first round; PR #208 was opened against the wrong branch by
  mistake during the parallel push and closed immediately.

- **`smell:c19ee328:duplicate-iface-default`** — done 2026-04-27
  — PR #195, merge commit `3953e7c`. Tier H minor (magic-strings).
  `"ens18"` literal duplicated between
  `internal/distribution/okd/setup/kargs.go:61`
  (ExtractNetworkConfig fallback) and
  `internal/distribution/okd/setup/steps.go:318`
  (generateKubeVIPManifests fallback). Hoisted to
  `netutil.DefaultProxmoxIface` with a doc comment naming the
  NM-interlock invariant — changing it requires updating the
  NetworkManager connection name in tandem. Wizard package
  (`internal/tui/wizard/steps/defaults.go:44-45` and
  `internal/tui/wizard/steps/networking.go:117`) carries its own
  `DefaultInterface` and a third literal — explicitly out of
  scope per the audit Evidence which named only the two setup
  sites. Reviewer first round FAILED on wrong-worktree audit
  (the reviewer read paths from a sibling worktree); re-dispatch
  with explicit absolute path and out-of-scope clarification
  passed. Doc-comment refinement during re-dispatch preserved
  the bastion-IP-for-DNS signal that the original "package-level
  defaults when unset" wording lost. **Postmortem lesson:**
  reviewer prompts must pin an absolute worktree path or the
  agent picks a worktree at random when verifying against on-disk
  state — the skill template should hard-require this for every
  reviewer dispatch.

- **`smell:c19ee328:duplicate-netmask-default`** — done 2026-04-27
  — PR #195, merge commit `3953e7c`. Tier H minor (magic-strings).
  `"255.255.255.0"` literal duplicated between
  `internal/config/defaults.go:58` (DefaultConfig StaticIP) and
  `internal/distribution/okd/setup/kargs.go:51`
  (ExtractNetworkConfig fallback). Hoisted to
  `netutil.DefaultNetmask` with a doc comment naming the
  homelab-/24 / Proxmox-default-bridge rationale. Audit
  suggested removing the kargs.go fallback ("DefaultConfig
  populates it before save"); kept it because
  ExtractNetworkConfig accepts any `*config.Config` including
  user-edited YAML where `StaticIP.Netmask` may be empty —
  defence-in-depth at the consumer is the right shape.
  Two-commit PR (netmask first, iface second) so build is clean
  at every commit boundary. **Postmortem lesson:** an "and
  remove the redundant fallback" suggestion in audit text is
  worth challenging — the audit can't see every caller, so
  consumer-side defaults often remain load-bearing.

- **`obs:8154ab0f:doctor-error-not-blocker`** — done 2026-04-27
  — PR #197, merge commit `ec6ec03`. Tier H suggestion
  (level-discipline, seam→audit-cli-ux). `runDoctor` recap line
  used `tui.Error(fmt.Sprintf(...))` to embed failing/warning
  counts in the message string, collapsing them past
  `logutil.RedactHandler` and bypassing the structured-attr
  path the rest of the codebase uses. Replaced with
  `tui.Error("doctor: failing checks block deploy",
  tui.LF("failing", fails), tui.LF("warnings", warns))`. Error
  level retained because deploy is genuinely blocked; the
  per-check loop above (lines 91-100) carries the actionable
  "fix it" guidance that the recap deliberately drops. Sibling
  Warn at line 107 uses the same fmt.Sprintf pattern but is
  outside this item's scope (Warn level is correct for warnings
  recap). Reviewer PASS first round.

- **`ux:e45c2239:preflight-tui-error-uses-exit-1`** — done
  2026-04-27 — PR #198, merge commit `d68b8f1`. Tier H
  suggestion (exit-codes). `cmd/okdctl/main.go::preflight()`
  exited 1 (reserved for "other error") when invoked as root,
  conflating the root-rejection path with generic command
  failure for wrapper scripts. Changed to `os.Exit(77)`
  (EX_NOPERM from BSD sysexits.h) with an inline WHY comment
  anchoring the literal. cli package-doc taxonomy in
  `internal/cli/root.go` updated to record code 77. The audit
  also said "adjust docs/cli/exit-codes.md" — that file does
  not exist; two larger items (`ux:aa84670c:exit-taxonomy-
  doc-only-in-package-doc`, `ux:aa84670c:exit-code-66-65-78-
  unmapped`) own its creation. Updating the in-code taxonomy
  comment is the minimal non-overlapping action; when the
  aa84670c items create the docs file they will pick up code 77
  from this comment. Verified no collision with `exitCodeFor`
  (returns 0/1/2/3/4/5/64/130/143). Reviewer PASS first round.

- **`err:d5915b0c:naked-ctx-err-return`** — done 2026-04-27
  — PR #199, merge commit `9e52552`. Tier H suggestion
  (cancellation-identity). `SetupKubeconfig` at
  `internal/distribution/okd/install/phase.go:158` returned bare
  `ctx.Err()` on cancellation — correct behaviourally (cli
  signal-exit-code mapping at root.go:149 uses `errors.Is(err,
  context.Canceled)` which still routes to 130) but
  cosmetically inconsistent with the in-tree wrap pattern at
  `system/exec.go::WaitFor`. Wrapped as
  `fmt.Errorf("setup kubeconfig: %w", err)`, matching the
  StepID string at `install/steps.go:62`. `%w` preserves
  `errors.Is` identity through Unwrap so the signal-exit-code
  mapping continues to route correctly. No tests assert on the
  bare error string. Reviewer PASS first round.

- **`sub:e552bb7d:iface-output-discards-stderr`** — done
  2026-04-27 — PR #200, merge commit `ece7685`. Tier H
  suggestion (io-handling). Three `exec.CommandContext(...).Output()`
  sites in `internal/netutil/iface.go` (RemoveSecondaryIP,
  GetDefaultInterface, connectionForDevice) discarded
  stderr-not-fall-through-to-ExitError on failure, so ip and
  nmcli diagnostics ("Cannot find device", "NetworkManager not
  running") were lost in the wrapped errors. Picked option (a)
  from the audit menu — extracted `system.OutputCaptured`
  symmetric with existing `RunCaptured` (same env-allowlist
  filtering via `executor.DefaultEnvAllowlist`, same
  `bin: %w: stderr` error format) — over option (b) errors.As
  at call sites (which would scatter three identical exitErr
  blocks) and option (c) cmd.Output's implicit stderr fallback
  (which only works when cmd.Stderr is unset and is reachable
  only via errors.As anyway). Behaviour change: env is now
  filtered (the original raw `exec.CommandContext` calls did
  not filter), but `DBUS_SESSION_BUS_ADDRESS` is in
  `DefaultEnvAllowlist` so nmcli's D-Bus path remains intact.
  Drops `os/exec` import from iface.go. Reviewer PASS first
  round.

- **`bug:elevation-preflight-deadlock`** — done 2026-05-03 —
  PR #217, merge commit `12f3147`. Tier 0 blocker. main.preflight()
  exited 77 on euid=0, but ensureRoot() syscall.Execs sudo and the
  re-exec'd okdctl re-entered main() at euid=0 — every privileged
  subcommand (deploy/destroy/cleanup/update-ingress) was broken
  end-to-end since elevation shipped (f00f08a). Moved the policy
  into ensureRoot via a testable elevationDecision helper, with a
  unit-test matrix covering euid×requiresRoot×dry-run. Reviewer
  PASS first round.

- **`tst:c8b28673:extract-tar-strip-symlink-resolved-untested`** —
  done 2026-05-03 — PR #218, merge commit `d88dbda`. Tier H major.
  Extended TestExtractTarGz_ZipSlipRejected with a TypeSymlink-then-
  TypeReg case verifying ExtractTarGz catches the symlink-then-write
  attack and no file lands at the attacker-controlled escape-dir
  outside dest. Reviewer PASS first round.

- **`sec:696d6b0e:shellinj-pattern`** — done 2026-05-03 — PR #219,
  merge commits `45f524d` + `7fecf29` (test followup). Tier H major.
  Three pvesh-over-ssh sites in iso_cleanup.go used per-call
  validateProxmoxName guards; a fourth call without the guard would
  silently re-open the gap. Introduced SSHRunArgv (helper) and
  pveshRun that validates p.Node once at the boundary. Doc comments
  explicitly note ssh argv mode does NOT bypass the remote shell —
  callers MUST validate atoms. setup/upload.go scp path is out of
  scope. First reviewer FAIL on stale doc comments (claimed execvp
  semantics ssh doesn't have); fixed. Coverage CI failed initially —
  added pveshRun helper tests to bring phase pkg back above the 35%
  floor.

- **`tst:1e8ffb91:parse-node-readiness-no-test`** — done 2026-05-03 —
  PR #220, merge commit `295f944`. Tier H major. parseNodeReadiness
  replaced a buggy strings.Contains parser that misclassified
  "SchedulingDisabled Ready". Five static-JSON cases: all-ready,
  multi-condition Ready=True (regression case), NotReady, malformed
  JSON, empty list. Reviewer PASS first round.

- **`err:97cb8adf:waitfor-timeout-loses-cluster-identity`** — done
  2026-05-03 — PR #221, merge commit `d941906`. Tier H major.
  WaitFor wrapped context.DeadlineExceeded in bare fmt.Errorf, so
  kube-vip / api-via-vip / svc-LB poll timeouts exited 130 (signal)
  instead of 4 (cluster) at cli/root.go's exitCodeFor. Wrapped into
  errtypes.ClusterError{Err: context.DeadlineExceeded} so both
  errors.Is and errors.As resolve. Reviewer PASS first round.

- **`obs:0934cf1b:sprintf-bypasses-redact-handler`** — done
  2026-05-03 — PR #222, merge commit `8051e52`. Tier H major.
  CLAUDE.md §credentials-and-secrets requires structured slog attrs
  so RedactHandler can inspect values before they collapse into the
  message string. Converted 13 sites in non-parallel-owned packages
  (platform/, cleanup/, dns/, download/, terraform/). First reviewer
  FAIL on (1) `strings.Join` collapsing the slice attr value and
  (2) missing caveats listing. Fixed: pass packages as []string
  directly, commit body now enumerates parallel-owned packages
  deferred to follow-up.

- **`ux:aa84670c:exit-code-66-65-78-unmapped`** — done 2026-05-03 —
  PR #223, merge commit `53be1ba`. Tier H major. exitCodeFor only
  branched on four typed errors (ConfigError=2/NetworkError=3/
  ClusterError=4/AuthError=5) and fell through to 1; BSD sysexits
  slots were not mapped. Added ErrConfigMissing (66 EX_NOINPUT),
  ErrPullSecretInvalid (65 EX_DATAERR), ErrSudoMissing (71 EX_OSERR)
  sentinels and wired exitCodeFor via errors.Is BEFORE errors.As so
  the specific BSD code beats the broad category. loadConfig also
  picks up errors.Is(err, os.ErrNotExist) — fixes a dead branch
  where LoadFile's *ConfigError wrapping made os.IsNotExist never
  match. ErrSudoMissing scaffolded for the elevation worktree to
  wire at elevation.go:60. docs/cli/exit-codes.md publishes the
  taxonomy. Reviewer PASS first round.

- **`ux:d31d1b9d:status-example-mismatches-schema`** — done
  2026-05-03 — PR #224, merge commit `294b097`. Tier H major.
  `okdctl status` Example showed `jq .ready_nodes` but the JSON
  schema has nodes[] with per-node ready booleans, so the example
  silently emitted null. Replaced with `jq '.nodes'` and
  `jq '[.nodes[] | select(.ready)] | length'` — both resolve against
  the actual schema. Reviewer PASS first round.

- **`tst:761e5126:remove-haproxy-no-test`** — done 2026-05-03 —
  PR #225, merge commit `d92086b`. Tier H major. RemoveHAProxy
  embedded three production literals (config path, health port, VIP
  timeout) blocking unit testing without root. Promoted to
  package-level vars (haproxyConfigPath, haproxyHealthPort,
  haproxyVIPTimeout) and added an empty-vip baseline test exercising
  the seam. Fuller VIP/hostname verify-block coverage (httptest +
  PATH-shadowed systemctl/oc fakes) deferred to a follow-up — the
  seam is wired and exercised. Reviewer PASS on scope-narrowed
  delivery.

- **`state:0f076161:destroy-no-scoped-only`** — done 2026-05-03 —
  PR #226, merge commit `2ba1856`. Tier H minor. Repeatable
  --target threads through okd.DestroyOpts → destroy.Options →
  terraform.DestroyOptions{Targets} (and PlanOptions in dry-run).
  Validated against an anchored regex matching `module.okd_cluster.
  proxmox_virtual_environment_vm.{bootstrap|master|worker}[<n>]`.
  --confirm-cluster required whenever --target is set. First
  reviewer FAIL on dry-run path skipping validation/threading and
  missing test for the runtime guard; fixed by moving validation
  before the dry-run branch and threading destroyTargets into
  PlanStreamed.

- **`sec:451be4fa:sudo-cp-no-p`** — done 2026-05-03 — PR #227,
  merge commit `b3ffa6f`. Tier H minor. filepath.WalkDir followed
  directory-component symlinks; a symlink under the workdir on a
  partial-failure resume could redirect Lchown to attacker-chosen
  paths. Open the root via os.OpenRoot and walk via
  fs.WalkDir(osRoot.FS(), ".", ...) so symlinks that escape the
  root return an error rather than silently traversing the target.
  Mirrors debug_bundle.go's tarDirInto pattern. Reviewer PASS first
  round.

- **`state:881d089e:runlock-stale-pid-no-recovery`** — done
  2026-05-03 — PR #228, merge commit `30b26d1`. Tier H minor. Lock
  body carried only PID/VERB/TIME — a shared NFS mount running
  flock from multiple hosts produced misleading conflict
  diagnostics. Append HOST= so cross-host conflicts are diagnosable.
  Package doc gets a one-sentence note about flock pre-NFSv4
  advisory-only semantics. Optional `okdctl unlock` verb deferred.
  Reviewer PASS first round.

- **`iac:18a795d5:dynamic-disk-no-precondition`** — done 2026-05-03
  — landed via in-review chore commit `e50d600` (data-disk-floor
  changes were inadvertently picked up when committing roadmap
  status updates; PR #229 closed as duplicate). Tier H minor. The
  dynamic "disk" block for the Ceph data disk on master/worker VMs
  was gated on `> 0`, so a typo zeroing master_data_disk_size_gb or
  worker_data_disk_size_gb in a re-apply silently stripped the disk.
  Added var.minimum_data_disk_size_gb (default 0) and use
  `>= floor && > 0` so operators can opt into a belt-and-suspenders
  refusal by setting the floor to 1. Behavior unchanged at the
  default. PROCESS NOTE: this is the second time stray
  worktree-tree changes have leaked into a chore(roadmap) commit
  — re-investigate the staging hygiene before the next batched run.

- **`iac:e076e43c:insecure-skips-cosign`** — done 2026-05-03 —
  PR #230, merge commit `161e4d7`. Tier H minor. The cosign
  verify-blob branch was nested inside the SHA256-gated block, so
  INSECURE=1 silently dropped both layers — even though cosign is
  independent of sha256sum. Probe COSIGN_CMD upfront and run
  verify-blob whenever cosign is present; INSECURE now only skips
  SHA256 verification. Warning text adapts to whether cosign is
  available. Reviewer PASS first round.

- **`tst:25fa1be8:firewall-haproxy-frontend-ports-no-test`** — done
  2026-05-03 — PR #231, merge commit `3e9f604`. Tier H minor.
  HAProxyFrontendPorts derives a subset of OKDRequiredPorts (TCP
  6443/22623/80/443) for postinstall.RemoveHAProxy. Added
  TestHAProxyFrontendPorts asserting the four expected TCP numbers,
  no DNS udp/53, length matches haproxyPortNumbers cardinality.
  Reviewer PASS first round.

- **`con:ab9b764a:inject-custom-manifests-no-ctx`** — done
  2026-05-03 — PR #233, merge commit `f593cbf`. Tier H suggestion.
  setup.InjectCustomManifests took `_ context.Context` and looped
  copying user-supplied YAML files via system.CopyFile; an unbounded
  number of files meant a cancelled deploy still ran to completion.
  Renamed the parameter to `ctx` and added `if err := ctx.Err(); err
  != nil { return count, err }` at the top of the loop body. Mirrors
  the ValidateIgnitionFiles pattern. Reviewer PASS first round.

- **`err:d31d1b9d:vocab-ad-hoc-unknown-addon`** — done 2026-05-03 —
  PR #234, merge commit `239bb5d`. Tier H minor. cli/status.go's
  runDescribeAddon returned a bare `fmt.Errorf("addon %q not
  registered…")` while the same condition in addon/manager.go:152
  returned `&errtypes.ConfigError{…}`. cli/root.go's exitCodeFor
  maps ConfigError → 2 and bare → 1, so the same user error exited
  with different codes depending on entry point. Wrapped the bare
  fmt.Errorf in `&errtypes.ConfigError{Msg: fmt.Sprintf(…)}` so
  exit 2 is consistent. Reviewer PASS first round.

- **`err:6424733c:wrap-double-context-deployment`** — done
  2026-05-03 — PR #236, merge commit `e1d71c4`. Tier H minor.
  executeFullDeployment in cli/helpers.go wrapped each phase error
  with `fmt.Errorf("deployment failed: %w", err)`. Inner errors
  were already typed errtypes.ClusterError / NetworkError carrying
  their own "failed to X" context, producing triple-layered surface
  messages. Dropped the outer wrap at three sites in helpers.go
  plus one at cli/deploy.go:120 (ActionDeploy wizard branch). The
  preceding `tui.Info("run 'okdctl destroy'…")` hint is preserved.
  errors.As walking — and therefore exit-code mapping — still
  works because the inner typed errors flow through unchanged.
  Reviewer PASS first round.

- **`err:a4001485:errtype-msg-vs-error-asymmetry`** — done
  2026-05-03 — merge commits `288be52` (initial doc + AST scan) and
  `9905893` (round-2 doc tightening). Tier H minor. All four
  errtypes carried both Msg and Err but `Error()` returned only
  Msg. Path leakage at the Msg level bypassed RedactHandler. Added
  doc comments on each errtype declaring "Msg must never include
  credentials" plus internal/errtypes/errtypes_credleak_test.go
  using go/parser+go/ast to walk every non-test .go under internal/
  and fail if errtypes.X{Msg: fmt.Sprintf(...)} interpolates a
  credential-bearing fragment. Reviewer FAIL first round flagged
  doc-vs-scanner contract drift (doc said "passwords, tokens,
  secrets" but scanner only matched password/api_key/apikey/passwd
  to avoid false positives on benign descriptive words like "pull
  secret"); follow-up commit aligned the doc to what the scanner
  actually enforces. Reviewer PASS round 2. Known limitations:
  scanner only catches Sprintf, not string concat (documented as
  Path A's intent).

- **`mod:377f1dcd:use-synctest`** — done 2026-05-03 — PR #238,
  merge commit `bbd6c50`. Tier H suggestion. TestOcPollOutput in
  phase/kubectl_test.go used real wall-clock budgets (30s, 500ms,
  5s timeouts). Wrapped each of three t.Run bodies in
  `synctest.Test(t, ...)` (Go 1.25 stdlib). The fake-oc subprocess
  exits in microseconds so it does not stall the bubble. Reviewer
  PASS first round.

- **`tst:368b892b:cleanup-tfstate-explicit-only-no-implicit-test`**
  — done 2026-05-03 — PR #239, merge commit `516ba33`. Tier H
  major. cleanup.Terraform iterates ReadDir when terraformEnv == ""
  and cleans every env dir. The tfstate-preservation invariant was
  only tested on the single-env path. Added
  TestTerraform_AllEnvs_PreservesEachState seeding two env dirs
  (production + staging) with terraform.tfstate + tfplan +
  .terraform.lock.hcl in each; calls Terraform with empty env name;
  asserts every tfstate survives byte-for-byte while every artefact
  is removed. Reviewer PASS first round.

- **`iac:18a795d5:master-no-prevent-destroy`** — done 2026-05-03 —
  merge commits `9b649f4` (variable + main.tf comment) and
  `723b5c8` (CI fix: tflint-ignore for unused-declaration). Tier H
  suggestion. Master VMs run etcd quorum but the
  proxmox_virtual_environment_vm.master resource had no
  `lifecycle { prevent_destroy = true }`. Terraform requires
  prevent_destroy to be a literal boolean, so a runtime toggle via
  variable is impossible. Added documentation-only
  `var.protect_masters` (default false) signalling operator intent,
  plus an HCL comment block above the master lifecycle showing the
  override-module pattern operators should adopt for production
  (`override.tf` with the literal lifecycle override). The
  authoritative destroy-safety guard still lives in the okdctl Go
  layer. Round 1 PASS; CI follow-up needed `# tflint-ignore:
  terraform_unused_declarations` directive.

- **`tst:830d4653:packages-binary-removal-untested`** — done
  2026-05-03 — PR #243, merge commit `ac868f2`. Tier H major.
  cleanup.Packages walked InstalledBinaries() and os.RemoveAll'd
  each filepath.Join(binDir, b); the per-binary refuseCriticalPath
  guard operated on the joined path (e.g. "/openshift-install"
  for binDir="/") which isn't in criticalPaths, so the guard
  never fired for dangerous binDir values. Added a top-of-function
  refuseCriticalPath(binDir) check returning *errtypes.ClusterError
  and added "/usr/local" to criticalPaths. New
  packages_test.go::TestPackages_RemovesScopedBinariesOnly seeds
  every InstalledBinaries() name plus an unrelated file in
  t.TempDir; fakes rpm/dnf/dpkg/apt-get on PATH; asserts each
  named binary is gone and unrelated survives. Companion test
  TestPackages_RefusesCriticalBinDir asserts "/" and "/usr/local"
  reject. Reviewer PASS first round.

- **`tst:d7ce9d16:dns-package-no-tests`** — done 2026-05-03 —
  PR #244, merge commit `b9bcfc5`. Tier H major. The dns/ package
  writes to /etc/dnsmasq.d/* under sudo and had zero tests.
  Extracted dnsmasqConfigDir from a const to a package var so
  tests can override. Added dnsmasq_test.go covering
  validateConfigName (accepts okd-prod, rejects empty/dots/path-
  traversal/special chars/leading-hyphen/length-65),
  DnsmasqConfigPath (rejects "../etc/passwd" and ""), and
  configName ("prod" → "okd-prod"). The integration restore-path
  test from the planner's plan was descoped — pure-function
  coverage was deemed sufficient for this round. Reviewer PASS
  first round (acknowledged the descope).

- **`tst:62cb8a95:destroy-infrastructure-tf-failure-untested`** —
  done 2026-05-03 — PR #245, merge commit `2dff510`. Tier H
  major. destroyInfrastructure had tests for missing-env-dir and
  empty-state, but not the path where tf.Init succeeds and
  tf.Destroy fails. Added installFakeTerraform helper writing a
  shell stub that exits 0 on init and 1 on all other subcommands;
  seedTerraformEnvDir pre-creates the .terraform short-circuit
  artefacts so Init never spawns a real subprocess. Test asserts
  the returned err is *errtypes.ClusterError unwrapping to
  *executor.ExitError (terraform.ExecError alias) with non-zero
  ExitCode. Reviewer PASS first round.

- **`tst:15ba17da:destroy-steps-failure-tracker-untested`** — done
  2026-05-03 — PR #246, merge commit `2fdedc2`. Tier H major.
  destroySteps's failures slice + track() closure is the
  post-mortem signal StepPrintSummary uses to distinguish
  "completed" from "finished with non-fatal failures" — a
  regression on the OnError wiring would silently restore the
  misleading-success bug. Added steps_test.go with a captureHandler
  slog.Handler implementation collecting records; three tests cover
  success path (Info "completed"), full failure path (drives all
  four StepDef.OnError, asserts Warn message + steps attr contains
  every expected label), and partial failure (only two OnError
  fired, asserts steps attr includes those two and excludes the
  others). Pure functional, no subprocess. Reviewer PASS first
  round.

- **`tst:632c9087:build-lb-ingress-controller-no-test`** — done
  2026-05-03 — PR #247, merge commit `fb8b470`. Tier H major.
  buildLBIngressController and buildRollbackJSON in
  postinstall/update_ingress.go drive the destructive
  HostNetwork→LoadBalancerService conversion and had zero tests.
  Added update_ingress_test.go with table-driven cases:
  TestBuildLBIngressController_PreservesFields covers all six
  optional fields (Replicas, DefaultCertificate, RouteSelector,
  RouteAdmission, NodePlacement) present and absent;
  TestBuildLBIngressController_TypeIsLoadBalancerService asserts
  the Type field is overridden from HostNetwork;
  TestBuildRollbackJSON_StripsServerFields verifies
  creationTimestamp, generation, resourceVersion, selfLink,
  managedFields, and top-level "status" are stripped. Stdlib
  encoding/json only. Reviewer PASS first round.

- **`tst:40d315ad:flux-deploy-key-secret-no-test`** — done
  2026-05-03 — PR #250, merge commit `6ddf96e`. Tier H major.
  flux.buildFluxDeployKeySecret reads a private key and constructs
  a Kubernetes Opaque Secret; flux.gitHost parses operator-supplied
  repo URLs. Both unexported, both untested. Added flux_test.go
  with TestBuildFluxDeployKeySecret covering all-fields-present
  (asserts identity, identity.pub, known_hosts in the data map and
  that plaintext does NOT appear in the YAML — base64-encoded
  only), empty-publicKey-omits-identity.pub, and namespace/name
  metadata correctness. TestGitHost table covers ssh://, https://,
  scp-style (github.com and self-hosted), ssh-with-port, and
  rejects empty/whitespace-only inputs. sigs.k8s.io/yaml round-
  trip. Reviewer PASS first round.

- **`con:8e65d574:update-check-bounded-leak-doc`** — done
  2026-05-03 — PR #235, merge commit `0ccc208`. Tier H suggestion.
  version.BackgroundCheck is the canonical fire-and-forget pattern
  CLAUDE.md §concurrency points to; the leak bound (httpTimeout =
  4s) was implicit. Extended the function's doc comment to record
  the contract: if the caller's 100ms select expires, the buffered
  send in the goroutine never blocks; the goroutine reaps within
  4s. No code change. Reviewer PASS first round.

- **`tst:262af6e4:cleanup-execute-full-kind-untested`** — done
  2026-05-03 — PR #241, merge commit `a0d6cce`. Tier H major.
  cleanup.Execute(Full) chains seven destructive helpers but
  tests only covered the partial-kind paths. Added three tests:
  TestExecute_FullKind_AllStepsRun (happy path: tfstate preserved,
  workDir + ignition removed), TestExecute_FullKind_AggregatesErrors
  (planted a regular file at the terraform environments path so
  ReadDir returns ENOTDIR; asserts *errtypes.ConfigError in the
  joined error and that prior steps still ran),
  TestExecute_FullKind_RemovePackagesGating (PATH-injected fake
  dnf+rpm+dpkg+apt-get assert package removal fires only when
  RemovePackages=true). CI required two follow-ups: (a) the initial
  fakes were dnf+rpm only, but Linux CI runs Debian → switched to
  family-agnostic apt-get/dpkg + dnf/rpm logging to a single
  `pkg.called` file; (b) the dpkg fake initially exited 0 with
  empty stdout but the platform postCheck requires `ii  <pkg>` in
  output to consider a package installed — fake now emits one
  `ii  <pkg> 1.0 amd64 fake` line per non-flag arg. Reviewer PASS
  first round; CI debugging took two rounds.

- **`tst:f55b9c27:write-env-file-zeroize-buf-untested`** — done
  2026-05-03 — PR #248, merge commit `1c67be9`. Tier H major.
  credentials.WriteEnvFile builds the .env body in a bytes.Buffer,
  AtomicWrites, then `clear(data)` to zero the backing store.
  Without a test asserting the wipe fires, a regression that
  moves the clear before the write or replaces it with `_ = data`
  would silently leak hot credential bytes on the heap. Extracted
  buildEnvFileBody(*ProxmoxCredentials) []byte so the test can
  hold a reference to the same backing slice WriteEnvFile uses.
  TestWriteEnvFile_BufferZeroedAfterWrite verifies the helper
  embeds the password as expected, then `clear`s and confirms every
  byte is zero. A second sub-test confirms WriteEnvFile writes the
  password to disk and the test's independently-allocated pre-call
  slice is NOT zeroed (i.e. WriteEnvFile clears its own allocation,
  not aliased external state). CI flake on the rebase: an unrelated
  test in another package hung for 60s — recovered by re-pushing
  the rebased branch. Reviewer PASS first round.

- **`tst:33579dd5:safe-remove-with-logger-error-paths-untested`**
  — done 2026-05-03 — merge commits `c01be98` (initial extract +
  test) and `01f50d3` (CI fix: gofumpt-format the var block
  introduced by the extract). Tier H major. cleanup.Dnsmasq's
  hard-coded `/etc/dnsmasq.d/okd-*.conf` and `*.backup` glob
  patterns prevented testing the glob-and-remove loop. Extracted
  both into package vars (`dnsmasqConfPattern`,
  `dnsmasqBackupPattern`) so tests can redirect to t.TempDir().
  Added services_test.go::TestDnsmasq_GlobLoopRemovesAllMatches
  seeding 3 conf + 2 backup files, overriding the vars (with
  t.Cleanup restore), and asserting each is removed. The bonus
  symlink-into-critical-path assertion was descoped per the item's
  "Bonus" wording. Reviewer PASS first round.

- **`tst:daf5bee9:merge-kubeconfig-secret-survival-untested`** —
  done 2026-05-03 — PR #249, merge commit `ca554e9`. Tier H major.
  cli/kubeconfig.go's mergeKubeconfig writes via
  system.AtomicWrite(0o600) but the existing test only covered
  mergeNamedList. A regression to 0o644 would silently widen perms
  on a file that may carry the user's bearer token. Added
  TestMergeKubeconfig_Perms that seeds a t.TempDir kubeconfig with
  `token: real-token`, sets KUBECONFIG to that path (so
  mergeTargetPath resolves to it without touching real
  ~/.kube/config), calls mergeKubeconfig with a different-name
  src user, then asserts: dest mode 0o600, original `real-token`
  preserved, src `new-token` appended, no `.tmp-*` artefacts left
  by AtomicWrite. Reviewer PASS first round.

- **`tst:ae5b624c:monitor-installation-no-test`** — done
  2026-05-03 — merge commits `dd9b73f` (initial monitor_test.go)
  and `4f1eb0e` (CI fix: `exec sleep` + redirect stdio to avoid
  orphan-pipe stall). Tier H major. install.MonitorInstallation
  is the longest privileged loop in the binary (60-min
  openshift-install wait, ticker-driven CSR approval, sync.OnceFunc
  kill, reapTimer ctx-cancel reap) — zero tests. Added
  monitor_test.go with three cases using a fakeApprover (atomic
  counter for ApprovePendingCSRs invocations) and a PATH-injected
  `openshift-install` shell stub keyed off OC_FAKE_MODE: success
  (asserts final ApprovePendingCSRs >= 1), DeadlineExceeded (20ms
  inner timeout against `sleep 300`; asserts errors.Is to
  DeadlineExceeded + errors.As to *errtypes.ClusterError),
  ctx.Cancel mid-loop (parent cancelled after 20ms; asserts
  errors.Is to context.Canceled, no ClusterError wrap). Round 1
  PASS, round 2 fixed CI hang: the original `sleep 300` shell
  child became orphaned after SIGKILL on the wrapper script and
  held stdout/stderr pipes, stalling `go test` shutdown 60s. Fix
  was `exec sleep 300 < /dev/null > /dev/null 2>&1` so the script
  process is replaced by sleep directly.

- **`tst:ddf885f4:manager-rollback-untested`** — done 2026-05-03 —
  PR #252, content landed via leak commit `7e12308` (PROCESS NOTE:
  third occurrence of stray-worktree-content leakage into a
  chore(roadmap) commit; PR #252 then merged as a no-op since the
  test code was already on develop). Tier H major.
  Manager.InstallAll rolls back ONE failed addon in isolation;
  Manager.InstallOne does ALL-OR-NOTHING reverse rollback. Neither
  rollback path had a test. Added manager_test.go with a stubAddon
  type using sync/atomic.Int32 spy counters for Install/Verify/
  Uninstall calls, plus registerStubs/registerStubs-cleanup to
  manipulate the global registry per-test. Four tests:
  middle-failure-rollback-only-middle (InstallAll independents),
  dependency-failure-skips-dependent (A→B chain), all-or-nothing-
  reverse-rollback (InstallOne A→B→C with C failing fires B then A
  uninstall), ctx-cancel-stops-install. Fake `oc` on PATH bypasses
  the executor.CommandExists guard. Reviewer PASS first round.

- **`smell:696d6b0e:redundant-vmstatus-enum`** — done 2026-05-03 —
  PR #266, merge commit `cb35802`. Tier G suggestion (magic-strings).
  `phase/iso_cleanup.go` declared a private `type vmStatus string;
  const vmStatusRunning vmStatus = "running"` parallel to
  `infrastructure/proxmox/types.go`'s `type VMState string;
  StateRunning VMState = "running"` — same Proxmox wire-protocol
  value, two definitions. Picked option (a) from the roadmap fix
  notes: created `phase/vmstate.go` with the canonical type +
  five constants (Running/Stopped/Creating/Deleting/Unknown) and
  reduced `proxmox/types.go` to `type VMState = phase.VMState` plus
  re-exported constants, mirroring the existing
  `type VMRole = phase.NodeRole` pattern. iso_cleanup.go now
  unmarshals JSON into `VMState` directly and compares against
  `StateRunning`. proxmox already imported phase for NodeRole, so
  no new edges in the import graph. Reviewer pre-merge: all 14 CI
  checks green.

  Bundled bonus extractions in the same PR (each was a duplicated
  string-literal surface that a typo on either side would have
  silently broken at runtime — Go has no compile-time check across
  string keys):
  - **wizard field-key prefixes** — `node_placement.go` had `"master"`
    / `"worker"` repeated across `nodePlacementSection` constructor
    args and the Apply read-back loop. Extracted `fieldPrefixMaster`
    / `fieldPrefixWorker` package-level consts.
  - **openshift subdir + binary** — `"openshift"` (3 sites in
    setup/steps.go + setup/ignition.go x2) and `"openshift-install"`
    (2 sites in setup/ignition.go) extracted to `openshiftSubdir` /
    `openshiftInstallBin` consts in setup/phase.go.
  - **PROXMOX\_VE\_\* env-var names** — five names duplicated
    between `credentials/envfile.go` (writes the names as literals
    when serialising a .env file) and `credentials/proxmox.go`
    (reads them back via `os.Getenv`). Extracted to
    `envProxmox{Endpoint,Username,Password,APIToken,Insecure}` in
    proxmox.go. Password and APIToken constants carry
    `//nolint:gosec // G101: env-var name, not a credential value`
    because gosec keys off the substring "PASSWORD"/"TOKEN" in any
    string literal regardless of whether it's a name or a value.
  - **cli flag names** — `"dry-run"` (4 registration sites + 1
    read-back via `cmd.Flags().GetBool` in `elevation.go`) and
    `"output"` (3 sites). Extracted `flagDryRun` / `flagOutput`
    in new `internal/cli/flags.go`. The dry-run drift was the
    highest-value catch in the sweep — a typo in the registration
    or in the elevation gate's GetBool would have silently routed
    a `--dry-run` invocation through the privileged code path.
  - **dup creds-warning chain** — the mixed-source
    "PROXMOX\_VE\_ENDPOINT not set; endpoint falling back to
    config file" warning (plus the override + provenance lines)
    was duplicated byte-for-byte in `cli/deploy.go` and
    `cli/helpers.go`. Lifted into `reportCredentialProvenance` in
    helpers.go; both call sites now invoke the helper.

  CI lint round-trip caveat: local golangci-lint v2.11.4 didn't
  flag the alignment quirk that v2.12.1 (CI) caught — gofumpt
  wanted two-space alignment between the `envProxmoxPassword` and
  `envProxmoxAPIToken` lines because the trailing nolint comments
  needed to align across the two consecutive lines. Fixed in a
  follow-up commit (`be71617`); no behaviour change. Lesson for
  future agents: when a CI lint version differs from local, expect
  formatting-class issues to surface in CI, not local. Make the
  Makefile pin the version to v2.12.1 to close this gap (out of
  scope for this PR).

- **`state:262af6e4:cleanup-tfstate-removal-window`** — done 2026-05-03
  — PR #253, merge commit `1ec954e`. Tier H suggestion
  (tf-state-atomicity). Documentation-only resolution per the item's
  Fix bullet ("Out-of-scope for a release fix; document the failure
  mode in the package doc as a known limitation"). Expanded the
  cleanup package doc at `internal/distribution/okd/cleanup/cleanup.go:1`
  with a paragraph naming the best-effort failure mode, the
  Terraform-removed-last invariant that keeps destroy re-runnable,
  and a `.cleanup-plan.json` checkpoint as future-work pointer.
  Round-2 fix: original draft asserted "services → files →
  terraform-cache" but the actual code orders WorkDirectory (files)
  → service shutdowns → Terraform; the misleading order claim was
  removed in commit `a620e27` after reviewer caught it.

- **`dep:33ef32bf:godotenv-license-filename`** — done 2026-05-03 —
  PR #254, merge commit `8003eed`. Tier H suggestion (license-compat).
  `github.com/joho/godotenv@v1.5.1` ships its MIT license under the
  British-English filename `LICENCE`; naïve SBOM scanners that grep
  for `LICENSE` flag a false positive. Added a one-line note in
  `CLAUDE.md §Dependencies` documenting the spelling so future
  scanner work knows the false positive is intentional. No code
  change. Reviewer PASS first round.

- **`tst:40d315ad:git-host-no-test`** — done 2026-05-03 — PR #255,
  merge commit `248a960`. Tier H minor (trust-boundary-untested).
  Added 5 missing edge-case rows to the existing TestGitHost table
  in `internal/addon/catalog/flux/flux_test.go`: ssh+port
  (`ssh://git@host:2222/o/r` → `host`), IPv6+port
  (`ssh://git@[2001:db8::1]:2222/o/r` → `2001:db8::1`,
  bracket-stripped per Go's url.URL.Hostname()), no-host scp,
  malformed-scheme (`://nope`), scheme-only (`http://`). Reviewer
  PASS first round.

- **`tst:35abd54e:env-method-zeroize-survives-no-explicit-test`** —
  done 2026-05-03 — PR #256, merge commit `532fa4a`. Tier H minor
  (cred-path-untested). Existing TestProxmoxCredentials_Env covered
  password backing not shared with the env string but had no parallel
  case for APIToken — a future refactor that swaps `string(c.APIToken)`
  for a zero-copy cast would silently break Zeroize. Added an
  `api_token_backing_not_shared_with_env_string` subtest that wipes
  the underlying `[]byte` after Env() and asserts the env entry
  preserves the original literal. Removed the now-redundant inline
  `string(pw) copies` comment per CLAUDE.md "don't narrate next
  line" — subtest name carries the contract. Reviewer PASS first
  round.

- **`smell:9ce5434c:single-caller-poll-wrapper`** — done 2026-05-03
  — PR #257, merge commit `935ba90`. Tier H suggestion
  (helper-package-no-value, scaffolding). Per MEMORY.md
  feedback_scaffolding the symmetric OcPollOutput / OcPollOutputInterval
  pair stays. Replaced the existing weaker doc on
  `internal/distribution/okd/phase/kubectl.go:51` to explicitly tag
  OcPollOutputInterval as the test-injection seam used by
  `phase/kubectl_test.go`, named the production rule (callers MUST
  use OcPollOutput which fixes interval=0), and noted the
  rename/delete coupling. Reviewer PASS first round.

- **`tst:9ce5434c:oc-output-typed-exit-error-untested`** — done
  2026-05-03 — PR #258, merge commit `4deecc7`. Tier H minor
  (canonical-helper-untested). OcOutput is the third canonical Oc*
  helper; tests covered OcResourceExists and OcPollOutput but not
  OcOutput's typed `*executor.ExitError` return. Added TestOcOutput
  with three subtests reusing installFakeOC: OC_FAKE_MODE=exists →
  trimmed stdout; OC_FAKE_MODE=error → errors.As to
  `*executor.ExitError`, ExitCode==1, Stderr contains
  "cluster unreachable"; ctx-cancel → propagates context.Canceled
  (not an ExitError). Reviewer PASS first round.

- **`sec:696d6b0e:input-url-scheme-not-checked`** — done 2026-05-03
  — PR #259, merge commit `812e7da`. Tier H minor (input-validation).
  pveshRun + SSHRunArgv (post-`d92086b`) already centralise the
  validateProxmoxName guard at the boundary between operator config
  and remote shell — the structural fix had landed; the gap was
  test coverage. Added TestValidateProxmoxName_RejectsBadNode and
  TestValidateProxmoxName_AcceptsValidNames to
  `internal/distribution/okd/phase/pvesh_test.go` covering the
  exact dangerous-character classes the item enumerates: empty,
  `.`, `..`, `/`, `node/name`, `node;name`, whitespace, backtick,
  `$()`, pipe, ampersand. Reviewer PASS first round.

- **`sec:7b2829bb:cred-env-leak-to-child`** — done 2026-05-03 —
  PR #260, merge commit `8a92bb8`. Tier H minor (credentials).
  DefaultEnvAllowlist's broad `GIT_`, `GITHUB_`, `GH_` prefixes
  forwarded GITHUB_TOKEN, GH_TOKEN, GIT_ASKPASS to every subprocess
  (`oc`, `helm`, `terraform`, `dnf`, `apt-get`) despite zero
  in-tree consumers. Verified by grep that no production code
  reads those vars. Replaced the prefixes with two exact-match
  keys actually needed: GIT_SSH_COMMAND (path-override) and
  GIT_TERMINAL_PROMPT (suppress git interactive prompts in CI).
  Updated TestAllowlist_ExactAndPrefix table: GITHUB_TOKEN/GH_TOKEN
  flipped to false. Reviewer PASS first round.

- **`sec:8ea706f6:input-path-not-prefix-checked`** — done 2026-05-03
  — PR #261, merge commit `c526dc9`. Tier H minor (file-toctou).
  installBinary used predictable `os.TempDir()/<name>-download`
  and `<name>-extract` paths — TOCTOU-vulnerable before the
  `system.CopyFile` to `/usr/local/bin` under sudo. Replaced both
  with `os.CreateTemp(os.TempDir(), <name>-download-*)` (handle
  closed before download, kept defer Remove, added explicit
  Remove on download-error path) and `os.MkdirTemp(os.TempDir(),
  <name>-extract-*)`. system.WriteTempFile was not the right fit
  because download.Download owns the OutputPath open/write itself.
  Reviewer PASS first round.

- **`sub:de572c63:nmcli-output-discards-stderr`** — done 2026-05-03
  — PR #262, merge commit `3e519ee`. Tier H suggestion
  (io-handling). `getActiveConnection` ran nmcli via `cmd.Output()`
  which discards stderr, so a `Error: NetworkManager is not running`
  diagnostic vanished into a bare exit-status error. Switched to
  `system.OutputCaptured` (the canonical helper, already used by
  `internal/netutil/iface.go` for the analogous nmcli call) so
  stderr lands in the wrapped error message. Added
  TestGetActiveConnectionStderr installing a fake nmcli that exits
  10 with the diagnostic on stderr and asserting the message
  reaches the error string. Reviewer PASS first round.

- **`tst:98bcb208:collect-doctor-output-no-test`** — done 2026-05-03
  — PR #263, merge commit `7e55dfe`. Tier H suggestion
  (canonical-helper-untested). collectDoctorOutput re-execs the
  binary as `doctor` and intentionally ignores `cmd.Run` error
  (failing preflight should still reach the debug bundle). Added
  linux-gated debug_bundle_doctor_test.go using the TestMain
  subprocess-hijack pattern: when `TEST_DOCTOR_SUBPROCESS` env is
  set the test binary acts as the fake doctor command (prints the
  env value, exits 1) before `m.Run()` parses test flags.
  TestCollectDoctorOutputBuffersOnFail proves the buffer survives
  non-zero exit; TestCollectDoctorOutputEmptyIsNonNil proves no
  panic on empty output. The os.Executable error path is not
  covered (no seam for it; out of Acceptance scope). Reviewer
  PASS first round.

- **`dep:33ef32bf:dup-yaml-engines`** — done 2026-05-03 — PR #264,
  merge commit `5137de2`. Tier H suggestion (duplicate-engine).
  cmd/okdctl-gen-docs's cobra/doc import transitively pulled
  `go.yaml.in/yaml/v3` into the release binary's linker graph
  (sigs.k8s.io/yaml's runtime parser is yaml/v2, not v3 — v3 was
  pure tax). Added `//go:build docs` to
  `cmd/okdctl-gen-docs/main.go` (only file in the package),
  switched Makefile `docs`/`docs-check` targets to invoke with
  `-tags docs`, updated CI `docs-go` job to do the same. Default
  `make build` / `go build ./...` no longer drags yaml/v3 through
  the linker; `go run -tags docs ./cmd/okdctl-gen-docs` still works.
  Reviewer PASS first round.

- **`sec:761e5126:tls-insecure-skip`** — done 2026-05-03 — PR #265,
  merge commit `182733c`. Tier H minor (tls-network). The production
  fix landed earlier in commit `c421069`
  (`fix(httputil): pin kube-vip TLS to cluster CA after install`) —
  RemoveHAProxy now uses httputil.KubeconfigCAPool +
  httputil.NewWithCA with a NewInsecure fallback when the CA is
  unavailable. This PR closed the roadmap item by adding the
  regression test the original fix shipped without. New
  TestRemoveHAProxy_KubeVIPHealthcheck builds a synthetic kubeconfig
  from a httptest.NewTLSServer cert (testcert SANs include 127.0.0.1)
  and asserts the VIP healthz check passes (advancing to the oc
  hostname check → ClusterError); a NetworkError from the VIP
  check would prove TLS was skipped. Subtest
  insecure_fallback_when_kubeconfig_absent verifies the fallback
  path is non-fatal. Reviewer PASS first round.

- **`state:15ba17da:destroy-summary-misleading-on-skip`** — done
  2026-05-03 — PR #277, merge commit `ae57e2c`. Tier H suggestion
  (crash-recoverability). StepPrintSummary classified the destroy
  as 'completed' iff len(failures)==0 — but Skipped steps don't
  append to failures, so SkipTerraform=true reported false success
  ("cluster teardown completed" even though terraform — the only
  infra-touching step — was skipped). Added a `skipped []string`
  slice alongside `failures`, plus a `trackSkip(label, fn)` helper
  that wraps each SkipWhen predicate so the label gets recorded
  when the predicate fires. StepPrintSummary now switches on three
  variants: failures>0 → Warn with `steps` attr; skipped>0 →
  Info with `skipped` attr; both empty → bare Info. Removed the
  now-misleading "re-run okdctl destroy to retry" hint from the
  failure path. New TestDestroySteps_SkipPath asserts all four
  skippable steps populate the slice and Info+skipped attr fires.
  Reviewer PASS first round.

- **`smell:073d24ed:duplicate-step-id-table`** — done 2026-05-03 —
  PR #279, merge commit `707c4dc`. Tier H minor (magic-strings).
  deployDryRunSteps hand-rolled 31 raw step ID strings while the
  canonical `StepID` constants already exist in setup/install/
  postinstall — silent-drift hazard whenever a phase renames a
  step. Replaced every literal with `string(setup.StepXxx)` /
  `string(install.StepXxx)` / `string(postinstall.StepXxx)` so a
  rename now produces a compile error. Added new deploy_test.go
  with TestDeployDryRunSteps_IDs as a compile-time guarantee that
  the dry-run list stays in sync. Round-2 fix: branch was rebased
  twice onto current develop because parallel session activity
  kept landing in-review status updates that produced noisy
  roadmap.md hunks; the second rebase + force-push cleared the
  diff. Reviewer PASS on round 2.

- **`sec:5013fea6:cred-env-leak-to-child`** — done 2026-05-03 —
  PR #282, merge commit `01fdb02`. Tier H minor (credentials,
  seam→audit-subprocess). extractReleaseImage used raw
  exec.CommandContext, bypassing Executor.buildEnv's allowlist —
  the child inherited the FULL parent env (KUBE_TOKEN, AWS_*,
  etc.) for the duration of the long-running release extract.
  Switched to `p.Exec.RunStreamed` so DefaultEnvAllowlist filters
  the env (KUBE/OC_/PROXMOX prefixes already cover legitimate
  registry-auth needs). result.Stderr's ring-buffered tail
  replaced the unbounded strings.Builder accumulator (also
  resolves sub:5013fea6:unbounded-stderr-builder at this call
  site). Constructed *executor.ExitError from the Result so
  errors.As callers retain ExitCode visibility. Dropped now-unused
  `errors` and `os/exec` imports. Reviewer PASS first round (with
  a non-blocking note about stale-base roadmap.md noise that the
  later rebase-merge resolved).

- **`doc:dd75bdeb:exported-doc-missing-type`** — done 2026-05-03 —
  closed without code (resolved by prior work). Tier H minor
  (exported-doc, seam→audit-api-design). The finding flagged the
  exported `PostInstallContext` struct in
  `internal/distribution/okd/postinstall/context.go` as needing a
  type doc — but PR #181 (api:dd75bdeb:export-no-caller) had
  already lowercased the type to `postInstallContext`, removing
  the export entirely. Verified by `grep` — only the lowercase
  symbol exists; CLAUDE.md §code-comments rule 2 (which requires
  docs on exported API) no longer applies. The roadmap-pickup
  planner caught the obsolescence and surfaced it via
  `unresolved_questions:`. No code change. Lesson: when a
  dependent item lands first, follow-up audit findings can be
  silently invalidated — a roadmap-pickup planner check on
  related-id status before applying a plan would have caught this
  earlier in the session.

- **`con:98723e5d:monitor-installation-no-test`** — done 2026-05-03
  — PR #283, merge commit `97624b5`. Tier H suggestion
  (time-sleep-retry, seam→audit-tests). MonitorInstallation is the
  most concurrency-dense function in the codebase (Wait-reaper
  goroutine, sync.OnceFunc kill, CSR-approval ticker, reapTimer
  with deadline, three-way select) but had zero tests; the existing
  TestMonitorInstallation_CtxCanceled also violated CLAUDE.md by
  using a bare `time.Sleep(20ms)` to delay cancellation. Extracted
  `defaultStartMonitorCmd` to monitor.go and added a
  `startMonitorCmd func(...) (done <-chan error, kill func(), err
  error)` field on Phase as a test-injection seam (production path
  unchanged when nil). Fixed CtxCanceled to cancel before entering
  MonitorInstallation. Added three synctest.Test cases:
  TickerApproveCSRs (advance 2s past 1s interval, assert
  approver.calls >= 1), CtxCancelReapsGracefully (cancel + advance
  5s + send done; assert context.Canceled returned and kill
  invocations == 1, no abandon-log), ReapTimeout (cancel + advance
  31s without firing done; assert kill invocations == 1 and
  abandon-log message captured). Round-1 reviewer FAILed on
  gocritic unnamedResult lint and missing kill-invocation
  assertion; both fixed in dc9be5b (named returns
  `(done <-chan error, kill func(), err error)` + atomic.Int32
  killed counter). Final merge required a manual rebase against
  develop's `d7260a6 fix(install): filter env on openshift-install
  monitor` which had landed independently and added
  `cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist)`
  directly to MonitorInstallation; conflict resolved by porting
  the env filter into defaultStartMonitorCmd so the seam preserves
  the env-allowlist behaviour. Lesson: when a long-running PR
  touches a hot file (monitor.go), expect parallel agents to land
  independent fixes there — rebase early and rebase often.

- **`state:48688e63:proxmox-no-retry-layer`** — done 2026-05-03 —
  PR #267, merge commit `73cb954`. Tier H suggestion
  (proxmox-api-idempotency). Doc-only addition: a six-line block on
  `proxmox.Provider` codifying the must-route-through-terraform
  invariant and naming `internal/download`'s retryDownload/
  isRetryable as the future-status-read path. Closes the gap where
  a future status-query patch could land an unprotected 5xx/429
  HTTP call.

- **`sec:8ea706f6:dl-no-checksum`** — done 2026-05-03 — PR #268,
  merge commit `0a1b06c`. Tier H major (tls-network). Hard-codes
  HashiCorp's published GPG fingerprint
  `AA16FCBCA621E70139936A4C798AEC654FA7E1A1` and verifies the
  fetched armored key via `gpg --with-fingerprint --with-colons
  --import-options show-only --import` before dearmoring to
  /usr/share/keyrings. Closes the MITM-during-deploy →
  permanent-apt-trust-root attack. Considered embedding the
  dearmored key bytes via go:embed; rejected because shipping a
  vendored copy of an upstream key okdctl can't independently
  validate is its own supply-chain risk.

- **`sec:40d315ad:cred-flux-deploykey-as-string`** — done
  2026-05-03 — PR #269, merge commit `c6c17fd`. Tier H minor
  (credentials). `buildFluxDeployKeySecret` now takes `[]byte` for
  privateKey/publicKey/knownHosts; eliminates the round-trip
  through Go strings. Added `clear(privateKey)` after
  `RunWithStdinChecked` returns so the caller's local buffer
  releases immediately. Caveat: yaml.Marshal still creates
  intermediate base64-string copies inside sigs.k8s.io/yaml;
  perfect zeroization is unreachable through the third-party
  library, but our local buffer is reclaimable.

- **`sub:ae5b624c:no-cmd-env-install`** — done 2026-05-03 —
  PR #270, merge commit `d7260a6`. Tier H minor (io-handling).
  MonitorInstallation built `installCmd` with raw
  exec.CommandContext and inherited the full parent env unfiltered;
  bypassed cli/elevation.go's allowlist when sudo re-exec was
  bypassed (test harnesses, already-root runs). Set
  `installCmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist)`
  before Start so AWS_*/GCP_*/AZURE_* and similar provider env
  can't reach openshift-install. Wait/reaper concurrency structure
  unchanged.

- **`sub:25fa1be8:ufw-output-discards-stderr`** — done 2026-05-03
  — PR #271, merge commit `69759fc`. Tier H suggestion
  (io-handling). `DetectBackend` previously fell through silently
  from ufw to iptables when `ufw status` failed. Threaded the
  package logger into `DetectBackend` (both internal callers
  Configure/RemoveRules updated), extracted stderr via
  `errors.As(*exec.ExitError)`, and emit a Debug-level record with
  err+stderr+backend keys so doctor and debug-bundle reflect why
  ufw was skipped. Probe-style fall-through preserved.

- **`err:b804b2ec:bootstrap-skip-tfvars-nil-as-success`** — done
  2026-05-03 — PR #272, merge commit `23fc1e4`. Tier H suggestion
  (sentinel-vs-typed). Removed the `ErrBootstrapTfvarsNotFound`
  sentinel and the in-body Warn+nil branch from `CleanupBootstrap`.
  Wired the tfvars-existence check into `StepDef.SkipWhen` so the
  orchestrator now records `Skipped=true` with a SkipReason rather
  than fake-success — distinguishing genuine "skipped because not
  applicable" from "successfully cleaned" in StepResult. Mirrors
  `internal/distribution/okd/destroy/steps.go:43-44`.

- **`ux:aa84670c:flag-error-os-exit-bypasses-defers`** — done
  2026-05-03 — PR #273, merge commit `8bffca0`. Tier H minor
  (exit-codes). `SetFlagErrorFunc` previously called `os.Exit(64)`
  directly, bypassing `Execute`'s deferred `logFileCloser.Close()`
  — any partial log written to `--log-file` was never flushed.
  Added `errtypes.UsageError` (Msg+Err with Error/Unwrap shape
  matching the existing four typed errors), mapped it to 64
  (EX_USAGE per BSD sysexits.h) in `exitCodeFor`, and replaced
  the os.Exit body with `return &errtypes.UsageError{Msg: err.Error(),
  Err: err}`. Updated the credleak AST scan, errtypes_test.go,
  and root_test.go for the new type. Reviewer note (non-blocking):
  flow now logs the flag-error twice — once in SetFlagErrorFunc
  body, once via execute()'s "command failed" handler; cosmetic
  follow-up.

- **`ux:d31d1b9d:status-degraded-operators-parsing-fragile`** —
  done 2026-05-03 — PR #274, merge commit `8ed0445`. Tier H minor
  (json-stability). `runStatus` parsed
  `oc get clusteroperators --no-headers` text by field index;
  column order is not stable across oc versions. Switched to
  `-o json` + `json.Unmarshal` into a `statusClusterOperatorList`
  struct, filtered via `slices.ContainsFunc` for type=Degraded
  status=True using the existing `phase.ConditionTypeDegraded` /
  `phase.ConditionStatusTrue` constants (also pays down their
  scaffolding lifetime). Mirrors the existing node-parsing shape.

- **`ux:fd2125dd:addon-list-config-enabled-column-cryptic`** —
  done 2026-05-03 — PR #275, merge commit `e3e7e0a`. Tier H
  suggestion (help-text). Added a `Long` description on
  `addonListCmd` with a `See also: addon verify` cross-link,
  renamed the table column from `CONFIG-ENABLED` to `IN-CONFIG`,
  updated the footnote to match. Required a follow-up commit
  `ac662d5` to regenerate `docs/cli/okdctl_addon_list.md` after
  the initial commit failed CI's docs-go drift check (caveat the
  planner had flagged).

- **`ux:8154ab0f:doctor-exits-1-on-fails-no-typed-error`** — done
  2026-05-03 — PR #276, merge commit `a9bd185`. Tier H suggestion
  (exit-codes). `runDoctor` returned a bare `fmt.Errorf` which
  exitCodeFor mapped to 1 (unclassified); broke the documented
  exit-code taxonomy. Wrapped with `&errtypes.ConfigError{Msg:
  "preflight checks failed"}` so doctor exits 2. Updated
  `doctor_cmd.go` Long ("1 otherwise" → "2 (configuration error)
  otherwise"), `docs/cli/exit-codes.md`, and regenerated
  `docs/cli/okdctl_doctor.md` in follow-up commit `37658f7` after
  initial push failed CI's docs-go drift check.

- **`ux:daf5bee9:kubeconfig-merge-no-y-flag-no-prompt`** — done
  2026-05-03 — PR #278, merge commit `948099d`. Tier H suggestion
  (flag-conventions). Single-line update to the `--merge` flag
  help: documents the non-destructive merge semantic
  ("(non-destructive: existing entries preserved)") so operators
  don't conflate `--merge` with destroy/cleanup which prompt.
  Required a rebase against develop's `5137de2 chore(deps): gate
  okdctl-gen-docs behind docs build tag` (resolved a kubeconfig.go
  conflict where develop introduced a `flagOutput` constant), plus
  a regen with `go run -tags docs ./cmd/okdctl-gen-docs`.

- **`ux:e7db1220:json-output-not-suppressed-by-quiet`** — done
  2026-05-03 — PR #280, merge commit `0d1e8b3`. Tier H suggestion
  (streams). `runReleasesList`/`runReleasesShow`/`runStatus` emit
  JSON to stdout while `tui.Info` chatter still flowed to stderr,
  breaking `2>&1 | jq` pipelines. Added `tui.SuppressInfo()`
  (raises stderr logger to ErrorLevel via
  `stderrLogger.Load().SetLevel`) and a `quietForJSON(format)`
  helper in cli/logging.go that calls SuppressInfo when
  `format==outputJSON && !logVerbose`. Wired into the three RunE
  funcs immediately after format validation. Reviewer note
  (non-blocking): no test added for the predicate; risk is low
  because the helper is one-line, but worth a follow-up.

- **`tst:de572c63:validate-config-name-no-test`** — done
  2026-05-03 — PR #281, merge commit `da36eae`. Tier H major
  (trust-boundary-untested). Extended `TestValidateConfigName`
  with four cases the existing table missed: single-char accept
  ("a"), two-char accept ("a1"), unicode reject ("é"), null-byte
  reject ("a\x00b"). Locks the regex enforcement against a future
  refactor that drifts the first-char anchor or the byte-class.
  Pure function, no fixtures.

- **`tst:696d6b0e:remove-fcos-iso-from-proxmox-no-test`** — done
  2026-05-03 — PR #284, merge commit `d48dbf3`. Tier H major
  (destructive-untested). Added three integration-level tests for
  `RemoveFCOSISOFromProxmox` using a fake `ssh` script in PATH
  (installFakeOC pattern from kubectl_test.go). The script
  switches on $6 to dispatch SSHRun vs SSHRunArgv calls, returns
  canned pvesh JSON, and writes to a counter file when `rm` is
  invoked. Cases: (1) ISO with no VM reference → counter==1;
  (2) ISO referenced by a running VM → counter file absent (skip
  path); (3) refuseUnsafeISOPath rejects /etc/passwd before rm
  fires. Used `executor.WithInheritedEnv()` to pass SSH_FAKE_MODE /
  SSH_RM_COUNTER through (neither is on DefaultEnvAllowlist).

- **`tst:0b188cab:retry-default-cancel-untested`** — done
  2026-05-03 — PR #285, merge commit `94040b2`. Tier H minor
  (canonical-helper-untested). Added three synctest-based tests
  for `addon.RetryDefault`: success on attempt N (asserts
  counter==3 with fn returning nil on call 3); all-failures
  returns `wait.Interrupted` with counter==DefaultRetryCount;
  ctx-cancel mid-sleep (goroutine cancels after
  DefaultRetryBackoff/2 fake) returns context.Canceled. Used
  Go 1.26 `testing/synctest` so the 5s default backoff doesn't
  extend test runtime. wait.Interrupted is the canonical check
  per upstream error.go:52-61 (deprecates direct ErrWaitTimeout
  comparison).

- **`tst:26a430ee:requires-root-dryrun-escape-untested`** — done
  2026-05-03 — PR #286, merge commit `f38e684`. Tier H minor
  (canonical-helper-untested). Added `TestRequiresRoot` covering
  all four canonical paths: destroy+dry-run=true → false (escape
  via flag), destroy+dry-run=false → true (membership wins on
  flag-present-but-false), deploy without dry-run flag → true
  (membership wins via GetBool err path), status (not in
  rootRequiredCmds) → false. Reused existing `newCmd` /
  `newDryRunCmd` helpers in elevation_test.go.

- **`tst:bdf5a873:work-directory-preserve-config-untested`** —
  done 2026-05-03 — PR #287, merge commit `4c20cde`. Tier H minor
  (destructive-untested). Added
  `TestWorkDirectory_PreservesConfigYaml` seeding workDir with
  okdctl.yaml + the four removable sub-trees (tmp, downloads,
  installer, custom-isos), calling WorkDirectory with
  preserveConfig=true, and asserting the config persists at the
  root while every sub-tree is gone. Catches a regression where
  preserveConfig is silently inverted and silently destroys
  operator-edited okdctl.yaml during partial cleanup.

- **`tst:e3782ee7:safe-remove-no-test`** — done 2026-05-03 —
  PR #288, merge commit `4809a90`. Tier H minor
  (canonical-helper-untested). Added `TestSafeRemove` with four
  subtests: missing path → nil (no-op); regular file → removed;
  directory tree → removed recursively; symlink → link removed
  but target preserved (RemoveAll does not follow symlinks into
  the target). Documents the symlink boundary so a future caller
  doesn't assume target preservation through the helper.

- **`tst:9d79b841:logged-iso-once-untested`** — done 2026-05-03 —
  PR #289, merge commit `4775152`. Tier H suggestion
  (canonical-helper-untested). Added `TestLogISOFound` using a
  custom `slog.Handler` as a record counter. Locks the basename
  dedup contract: two calls with the same basename emit one
  record; adding a new basename emits a second; a third call with
  the same basename from a different directory is suppressed
  (basename key, not full-path key). Catches the regression where
  the dedup key drifts from basename to full path.

- **`tst:08c49fc4:update-ingress-confirm-callback-untested`** —
  done 2026-05-03 — PR #290, merge commit `d3a23f6`. Tier H minor
  (destructive-untested). Refactored the inline ConfirmConversion
  closure in `runUpdateIngress` into `buildConvertConfirm(ctx, yes)`
  at file scope so it can be unit-tested. Added a `testStdinReader`
  package var to confirm.go — when non-nil, replaces os.Stdin and
  bypasses the TTY guard; production code never touches it. Tests
  cover yes=true (returns true regardless of input) and yes=false
  with y/n/EOF inputs via the stdin reader seam. Tests must run
  sequentially because the seam is package-global; no t.Parallel().

- **`sub:5013fea6:unbounded-stderr-builder`** — done 2026-05-04 —
  merge commit `01fdb02`. Tier H minor (io-handling). Discovered
  during /roadmap-pickup that the unbounded `strings.Builder` had
  already been replaced by routing through `p.Exec.RunStreamed`,
  which attaches `io.MultiWriter(e.Stderr, ringWriter)` capped at
  200 lines (`internal/executor/executor.go:266-269`). Stderr is
  now ring-bounded for both live streaming and the post-run
  result.Stderr tail used for auth-error sniffing. RunStreamed
  preferred over RunStreamedChecked because the auth-marker scan
  in `setup/release_extract.go:130-133` reads result.Stderr
  directly; RunStreamedChecked would fold non-zero exit into an
  *ExitError needing errors.As extraction.

- **`tst:de572c63:dnsmasq-config-path-no-test`** — done 2026-05-04 —
  merge commit `b9bcfc5`. Tier H minor (trust-boundary-untested).
  Discovered during /roadmap-pickup that `TestDnsmasqConfigPath`
  was already in `internal/distribution/okd/dns/dnsmasq_test.go`
  with all three required cases: clean `okd-prod` returns the
  canonical `/etc/dnsmasq.d/okd-prod.conf` path; `../etc/passwd`
  returns an error not a path (validateConfigName regex rejects
  the slash before filepath.Join is reached); empty name returns
  an error. Test uses `package dns` (white-box) to read
  unexported `dnsmasqConfigDir` directly so the asserted path
  stays in sync with phase.DefaultDNSMasqConfigDir.

- **`ux:073d24ed:metrics-addr-no-bind-tty-gating`** — done 2026-05-04
  — PR #303, merge commit `045d5c6`. Tier H suggestion
  (flag-conventions). The `--metrics-addr` flag help said `e.g. :9090`
  but `helpers.go:147` silently rewrites bare `:port` → `127.0.0.1:port`
  for safety. Updated the help string to document both forms (loopback
  default vs `0.0.0.0:port` wildcard) and regenerated the cobra-driven
  reference docs to keep the docs-drift CI check green.

- **`ux:024a2c32:json-schema-display-name-hyphen-inconsistent`** —
  done 2026-05-04 — PR #300, merge commit `889ecc5`. Tier H suggestion
  (json-stability). Encoder at `cli/status.go:358` already emitted
  snake_case `display_name` for the JSON output; only the docs were
  stale (`docs/cli/json-schema.md:107-121` still showed kebab-case
  `display-name` with a "historical reasons" disclaimer). Updated the
  example, removed the disclaimer, added a CHANGELOG entry. No code
  change needed.

- **`tst:696d6b0e:validate-proxmox-name-no-test`** — done 2026-05-04
  — PR #292, merge commit `5c78944`. Tier H major
  (trust-boundary-untested). Added `TestValidateProxmoxName` in
  `phase/iso_cleanup_test.go` covering the byte-by-byte allowlist:
  accept set `{"pve","pve-1","node_a","PVE0","1pve"}` and reject set
  covering empty, dot, slash, semicolon, backtick, dollar, space,
  unicode, null byte. Note: `1pve` (leading digit) is in the accept
  list because the current impl allows digits in any position; the
  test locks current behavior, not future-tightening intent.

- **`tst:27088eab:ssh-run-no-test`** — done 2026-05-04 — PR #302,
  merge commit `41c41ce`. Tier H minor (canonical-helper-untested).
  Added `TestSSHRun` and `TestSSHRunArgv` plus `installFakeSSHEcho`
  helper (named to avoid colliding with the existing mode-switching
  `installFakeSSH` in `remove_fcos_iso_test.go`). Both tests assert
  the canonical flag set (`-o StrictHostKeyChecking=accept-new`,
  `-o BatchMode=yes`) and `root@<host>` survive the executor argv
  unchanged.

- **`tst:b804b2ec:cleanup-bootstrap-plan-file-leak-untested`** —
  done 2026-05-04 — PR #301, merge commit `0b6bd3d`. Tier H minor
  (destructive-untested). Added `bootstrap_test.go` covering the
  three exit paths of `CleanupBootstrap` (success, plan-fail,
  apply-fail) with a fake terraform binary keyed on `TF_FAKE_MODE`.
  Each test pre-creates `bootstrap-destroy.tfplan` and asserts
  `os.IsNotExist` after the call — pinning the deferred `SafeRemove`
  invariant the doc comment names as a regression risk.

- **`sec:d7ce9d16:input-validation`** — done 2026-05-04 — PR #298,
  merge commit `92b93a3`. Tier H suggestion (input-validation).
  Added `validateConnectionName` to `dns/dnsmasq.go` rejecting
  `;\n\r\x00`$<>|&` while permitting spaces (NetworkManager allows
  them). Validation lives in `getActiveConnection` so both
  `ConfigureSystemResolver` and `RestoreSystemResolver` inherit
  the guard at one chokepoint. Defense-in-depth against poisoned
  nmcli output; today's argv invocations are safe but a future
  shell-style call would be exposed.

- **`sec:8ea706f6:dl-hashicorp-gpg-overwrite`** — done 2026-05-04
  — PR #297, merge commit `fdd4ac4`. Tier H suggestion
  (tls-network). `installHashiCorpDebianRepo` now stats the target
  keyring path and re-runs `verifyHashiCorpGPGFingerprint` against
  the on-disk binary keyring (`gpg --import-options show-only`
  accepts both armored and binary). Mismatch returns
  `*errtypes.ConfigError` (exit 2) instructing the operator to
  remove the file. Match short-circuits the dearmor entirely,
  making re-deploys idempotent.

- **`sec:f55b9c27:err-type-carries-cred`** — done 2026-05-04 —
  PR #296, merge commit `f86f5f6`. Tier H suggestion (redaction —
  seam→audit-errors). Added `Path string` field to
  `errtypes.AuthError`; `Error()` appends `(path: <Path>)` only
  when set so existing callers' output stays stable when Path is
  zero. `loadEnvFileOnce` populates `Path: path` instead of
  embedding it in `Msg` via `fmt.Sprintf`. Other AuthError
  construction sites (loader.go, ignition.go, release_extract.go,
  elevation.go, WriteEnvFile) leave Path zero — staged migration
  out of scope here.

- **`sec:1e8ffb91:input-validation`** (clusteroperator-json) —
  done 2026-05-04 — PR #299, merge commit `29dd0c9`. Tier H
  suggestion (input-validation). Replaced the brittle
  `oc get clusteroperators --no-headers` positional fields[4]
  parse with `-o json` + structured walk over `status.conditions`
  for type=Degraded status=True. New `clusterOperatorList` struct
  + `parseOperatorDegradation` mirror the existing
  `nodeList`/`parseNodeReadiness` pattern. Six-case test covers
  no-degraded, one, multiple, missing-condition, malformed-json,
  empty list.

- **`sec:40d315ad:cred-flux-helm-set-leak`** — done 2026-05-04 —
  PR #294, merge commit `13c94bd`. Tier H minor (credentials).
  `flux.ValidateSettings` now rejects http/https URLs containing
  userinfo (`https://user:token@host`) — these would land in
  helm `--set` argv visible via `/proc/<pid>/cmdline`. Restricted
  to http/https schemes so legitimate `ssh://git@host/` SSH
  usernames still work; scp-style `git@host:path` bypasses
  url.Parse and is also accepted. Wizard help text and doc
  comment direct users toward SSH deploy-key auth.

- **`err:45cf4e29:wrap-double-context-typed`** — done 2026-05-04
  — PR #295, merge commit `4be25ef`. Tier H minor (wrapping).
  Six step closures across `install/postinstall/setup/destroy
  steps.go` were re-wrapping inner typed errors with
  `&errtypes.ClusterError{...}` (or `NetworkError`), silently
  reclassifying inner types and drifting exit codes. Each closure
  now returns the inner error directly so `errors.As` in
  `exitCodeFor` walks the chain and the inner type's exit code
  surfaces (ConfigError → 2, NetworkError → 3, ClusterError → 4,
  AuthError → 5). The biggest concrete win: AuthError from
  `extractReleaseImage` now correctly surfaces as exit 5 instead
  of buried under NetworkError → exit 3.

- **`state:fb54208a:postinstall-no-rollback-path`** — done
  2026-05-04 — PR #293, merge commit `85516cc`. Tier H major
  (crash-recoverability). Added `dns.IsBootstrapDNS(cfg)` that
  reads `/etc/dnsmasq.d/okd-<name>.conf` and detects when api.*
  still resolves to the bastion IP rather than the kube-VIP,
  using **line-exact matching** rather than substring
  (substring false-positives `10.0.0.1` against `10.0.0.10` —
  caught by the new test before merge). `UpdateIngress` logs a
  warning when bootstrap-pointed DNS is detected and surfaces a
  `DNSReconciled` flag in the result + summary line. The actual
  production-DNS deploy already happens unconditionally inside
  `finalizeIngress`; this commit makes the recovery path
  discoverable rather than silent.

- **`sec:6424733c:cred-as-string`** — done 2026-05-04 — PR #291,
  merge commit `8e0be7c`. Tier H major (credentials). New
  `config.SecretBytes` type owns a `[]byte` with
  `Set/Zeroize/Bytes/IsEmpty/String/Redacted` so credentials
  can be wiped after use rather than lingering as immutable Go
  strings until GC. `ProxmoxConfig.Password` and `APIToken`
  changed from `string` to `SecretBytes`; wizard, deploy,
  destroy, and tests migrated. `clearConfigCredentials` now
  calls `Zeroize()` on the wrapper. **Residual leak boundary:**
  the wizard input pipeline still funnels the captured password
  through a Go string at `bubbles.textinput.Value()`, and
  `proxmox_discovery.go:77` converts back to string for the
  third-party `go-proxmox.Credentials.Password string` field.
  These are inherent to the upstream library boundaries and
  bounded in lifetime (10s discovery timeout, single wizard
  pass); follow-on work would require forking those libraries
  or refactoring the wizard component model. GitGuardian
  initially flagged the migrated test fixture strings (existing
  `cfg-token` / `cfg-pw` values now adjacent to a `Set()`
  setter); renamed to `EXAMPLE-` prefixed values to satisfy the
  scanner.


- **`api:d6b325cb:pkg-sibling-reach-through`** — done 2026-05-04
  — PR #304, merge commit `95cf90c`. Tier G major
  (package-boundary). `internal/infrastructure/proxmox/types.go`
  was importing OKD-specific `internal/distribution/okd/phase` to
  alias `VMRole = phase.NodeRole` and re-export Role/State
  constants — inverted the directional invariant
  (distribution depends on infrastructure, not the reverse).
  Replaced both aliases with independent string-typed local
  declarations carrying the same wire values. No translator
  needed because no call site outside the proxmox package
  consumes the types — `proxmox.go` constructs `VMStatus`
  literals entirely from its own constants.

- **`api:262af6e4:opt-inconsistent`** + **`err:262af6e4:sentinel-double-wrapped`**
  — done 2026-05-04 — PR #305, merge commit `3dededd`. Tier G
  minor (option-consistency / sentinel-vs-typed). Cleanup was
  the only OKD phase exposed as a package-level function rather
  than the `New(...)` + `Phase.Execute(ctx, *Options)` shape
  used by setup/install/postinstall/destroy. Added
  `cleanup.New(exec, logger, version) *Phase` mirroring
  `destroy/phase.go::New` exactly; updated Provisioner-holding
  callers (`okd.go::Prepare`, `destroy/steps.go`) to use it.
  Kept the package-level `Execute` for the bare-CLI use case
  (`cli/cleanup.go` doesn't have a Provisioner instance).
  Dropped `ErrKindNotSet` sentinel — zero `errors.Is` callers
  in repo; replaced its sole use site with a bare
  `&errtypes.ConfigError{Msg}`. `errors` import preserved via
  `errors.Join`.

- **`api:35abd54e:export-no-caller-scaffolding`** + **`doc:35abd54e:doc-claim-vs-impl-drift`**
  — done 2026-05-04 — PR #306, merge commit `5ed7cd7`. Tier G
  suggestion (exported-surface / exported-doc). The Source enum
  doc claimed `SourceConfig` was a reachable state, but
  `GetProxmoxCredentials` never sets it (per the L250 comment
  removing config-file fallback). Dropped `SourceConfig`
  constant + its `String()` arm; simplified the switch into an
  if/else. The doc commitment ("callers SHOULD warn on
  EndpointFromConfig / ConfigCredentialsOverridden") was
  already wired by `cli/helpers.go::reportCredentialProvenance`
  — no new warning code needed.

- **`smell:7f86cbe2:any-return-second-value`** — done 2026-05-04
  — PR #307, merge commit `fd2fd32` (with delta commits
  `5f1c0a1` and `fc7fe6f`). Tier G suggestion (interfaceany-lazy).
  Replaced `func() (wizard.WizardStep, any)` factory return type
  with `func() (wizard.WizardStep, wizard.StepState)` where
  `StepState` is a marker interface (`interface{ IsWizardStepState() }`).
  `*DataDrivenStep`, `*NodePlacementStep`, `*ResourcesStepState`
  each gained the no-op marker. Cross-package interface
  satisfaction required the marker method be exported (initial
  unexported `wizardStateMarker` failed compile). Reviewer
  delta caught two issues: (a) echo-signature comments with stale
  identifier names — deleted; then (b) revive's `exported` rule
  required comments back, so re-added one-line WHY comments.
  Lesson: marker methods on exported types still need doc
  comments per revive.

- **`smell:125729c4:unused-public-field-force`** — done
  2026-05-04 — PR #308, merge commit `b107bf5`. Tier G
  suggestion (helper-package-no-value). `destroy.Options.Force`
  was assigned `true` at `okd.go:182` but never read anywhere.
  AutoApprove already covered the skip-prompt axis; investigation
  found no roadmap entry planning a future `--force` destroy verb
  (ux:0f076161 was already done covering `--force` flag
  deprecation). Dropped the field, the assignment site, and the
  stale `Execute` doc-comment that referenced `opts.Force`.

- **`ux:aa84670c:exit-taxonomy-doc-only-in-package-doc`** —
  done 2026-05-04 — PR #309, merge commit `666bbab`. Tier G
  minor (exit-codes). Expanded `docs/cli/exit-codes.md` from a
  22-line stub to a full reference page (intro, table, examples,
  source anchor); linked from README. Reviewer flagged a stale
  `77 | EX_NOPERM | invoked as root user` row faithfully copied
  from `internal/cli/root.go:8-9`'s package doc — but `ensureRoot`
  returns `errtypes.AuthError` mapped to exit 5, not `os.Exit(77)`.
  Dropped the bogus row; added a clarifying paragraph that root
  rejection lands at code 5. The pre-existing `root.go:8-9`
  package-doc inaccuracy remains and should be a follow-up item.

- **`con:48688e63:proxmox-connect-discards-ctx`** — done
  2026-05-04 — PR #310, merge commit `ffc52a2`. Tier G
  suggestion (ctx-ignored). `Provider.Connect`/`Disconnect`
  accept `_ context.Context` for interface symmetry but never
  use it (no I/O happens until Provision; auth flows through
  terraform via env vars). Added one-sentence WHY comments to
  both functions: "ctx is accepted for symmetry with future
  network-bound providers; this implementation is local-only."
  Per CLAUDE.md §concurrency, unused ctx params need a
  justification comment.

- **`tst:79e2cbc4:resolver-circular-deps-untested`** — done
  2026-05-04 — PR #311, merge commit `9b843ca`. Tier H major
  (destructive-untested). Resolver implements Kahn's topological
  sort over addon dependencies; no test covered circular
  detection, priority ordering, or missing-dependency paths.
  Added `internal/addon/resolver_test.go` with a `fakeAddon`
  fixture and 5 cases covering name-sort tiebreak, priority
  tiebreak, A→B→C chain ordering, missing-dep error string, and
  circular A↔B error message. Initial planner draft used a
  placeholder `interface{ Done() <-chan struct{} }` for the
  Install/Verify/Uninstall stubs; the corrected version uses
  `context.Context` matching the real Addon interface.

- **`tst:451be4fa:chown-tree-error-aggregation-untested`** —
  done 2026-05-04 — PR #312, merge commit `c5463dd`. Tier H
  major (canonical-helper-untested). `ChownTreeToInvokingUser`
  uses `errors.Join` to aggregate per-entry chown failures and
  continues the walk on individual EPERM. The existing test
  only covered the no-SUDO_UID short-circuit. Added two
  sub-tests: (1) no-op same-uid case asserting nil for a tree
  with a regular file + dangling symlink; (2) unprivileged
  uid 65534 case asserting `errors.Join` wraps ≥3 sub-errors
  (proves the walk visited "." + 2 files rather than aborting
  at first EPERM). Test branches on `os.Getuid()==0` to handle
  root CI runners.

- **`tst:97cb8adf:run-captured-no-test`** — done 2026-05-04
  — PR #313, merge commit `f6aeb95`. Tier H major
  (canonical-helper-untested). `system.RunCaptured` is the
  canonical "run a command, surface stderr in the err" helper
  (15+ call sites in firewall/dnsmasq/netutil); the
  stderr-into-err shape is load-bearing for downstream
  `errors.As` consumers. Added 4 cases via inline `sh -c`
  invocations: exit 0 → nil; exit 1 with stderr → wrapped +
  errors.As to `*exec.ExitError`; exit 1 empty stderr →
  bin-name only; pre-cancelled ctx → `errors.Is(err, context.Canceled)`.

- **`tst:73ad30ef:resolve-cluster-vip-no-test`** — done
  2026-05-04 — PR #314, merge commit `ee90cd2`. Tier H minor
  (canonical-helper-untested). `phase.ResolveClusterVIP` is the
  5-call-site wrapper around `netutil.ResolveVIP` with a fixed
  `"failed to resolve VIP"` prefix. Added `helpers_test.go`
  with 3 sub-tests: explicit VIP wins; static-IP-derived VIP
  uses `DefaultVIPLastOctet=10` (so `192.168.1.50` → `192.168.1.10`);
  malformed VIP wraps with the canonical prefix.

- **`tst:5e892064:download-checksum-fetch-paths`** — done
  2026-05-04 — PR #315, merge commit `ba12a5a`. Tier H minor
  (trust-boundary-untested). `verifyDownloadedFile` removes the
  artifact on checksum mismatch — caller-side path was untested.
  Added `TestVerifyDownloadedFile` with 3 cases: empty expected
  → nil + file untouched; matching expected → nil + file intact;
  mismatching expected → err + file gone. Uses canonical
  `logutil.NopLogger` rather than inline
  `slog.New(slog.DiscardHandler)`.

- **`tst:7b2829bb:run-streamed-checked-no-test`** — done
  2026-05-04 — PR #316, merge commit `cd0effa`. Tier H minor
  (canonical-helper-untested). `RunStreamedChecked` is the
  canonical "stream stdout+stderr live AND return ExitError on
  non-zero" helper used by `terraform.Plan/Apply/Destroy`. Added
  `TestRunStreamedChecked` with 3 sub-tests via `sh -c`: zero
  exit streams + captures; non-zero returns `*ExitError` with
  ring-buffered stderr tail; ctx cancel returns context error
  unwrapped (NOT `*ExitError`). Uses
  `New(WithInheritedEnv())` so the test's PATH propagates.

- **`tst:4c092fce:terraform-build-var-args-untested`** — done
  2026-05-04 — PR #317, merge commit `0615fb4`. Tier H minor
  (canonical-helper-untested). `buildVarArgs` sorts vars
  alphabetically before composing `-var k=v` — terraform plan
  reproducibility depends on this. Added two tests: (1)
  deterministic-order assertion against `{"z":"3","a":"1","m":"2"}`
  → `["-var","a=1","-var","m=2","-var","z=3"]`; (2) missing-var-file
  asserts no `-var-file=` token in result + Warn was logged.
  Tests use a tiny `captureHandler` slog.Handler stub for the
  Warn-capture assertion (canonical NopLogger insufficient
  here).

- **`tst:e552bb7d:remove-secondary-ip-no-test`** — done
  2026-05-04 — PR #318, merge commit `96ecee8`. Tier H minor
  (destructive-untested). `RemoveSecondaryIP` short-circuits to
  nil when the IP is absent (avoids needless `nmcli device
  reapply`). Added `iface_test.go` with `installFakeIPNmcli`
  helper that writes POSIX shell scripts named `ip` and `nmcli`
  into a `t.TempDir`, prepends to PATH (PATH is in
  `executor.DefaultEnvAllowlist.Exact`). nmcli log path
  embedded in the script body to bypass env-allowlist
  filtering. 5 sub-tests: ip absent (0 nmcli calls); ip present
  (3 nmcli calls: show + modify + reapply); ip exits 1 (wrapped
  error with "failed to check IP presence"); empty-arg
  validation × 2.

- **`tst:881d089e:runlock-write-failure-untested`** — done
  2026-05-04 — PR #319, merge commit `54d774b`. Tier H minor
  (canonical-helper-untested). Acquire writes
  `PID=%d VERB=%s TIME=%s HOST=%s\n` after successful flock;
  prior tests asserted only HOST=. Extended TestAcquireAndRelease
  to read the lock file post-acquire and assert PID=, VERB=deploy,
  TIME= are present. Added TestConflictMessageContainsPID
  asserting the conflict `*errtypes.ConfigError.Msg` contains
  the prior holder's PID (proves the lock body was actually
  read into the conflict message — guards against a regression
  where truncate-then-write fails silently).

- **`tst:e3782ee7:expand-path-no-test`** — done 2026-05-04
  — PR #320, merge commit `1d2dc40`. Tier H minor
  (canonical-helper-untested). `ExpandPath` resolves `~/foo` via
  `InvokingUserHomeDir` (which uses `user.Lookup`, not
  `$HOME`). Added 5 sub-tests: SUDO_USER=current → `~/x`
  expands to `<homedir>/x`; bare `~` → unchanged; `~user/foo`
  → unchanged (only `~/` prefix expands); absolute → unchanged;
  relative → unchanged. Lesson: don't try to redirect ExpandPath
  via `t.Setenv("HOME", ...)` — `user.Lookup` reads
  `/etc/passwd`, so the test depends on a valid passwd entry
  for the current user (always true in normal CI).

- **`tst:eb479d86:upload-iso-via-scp-no-test`** — done
  2026-05-04 — PR #321, merge commit `fab356e`. Tier H minor
  (trust-boundary-untested). `uploadISOsViaSCP` composes scp
  argv from `[]isoFiles + user/host/remotePath`; argv passes
  through `os/exec` (no shell interpolation), but the lack of
  test left a future `sh -c` retry refactor free to silently
  introduce CWE-78. Added `installFakeSCP` helper that writes a
  POSIX shell script printing each `$@` arg on its own line.
  Two tests: (1) argv-shape asserts `-o
  StrictHostKeyChecking=accept-new` pair, ISO paths as discrete
  argv entries, destination `user@host:path/` with trailing
  slash; (2) filename-with-spaces survives as one argv entry.
  Used canonical `logutil.NopLogger`. No production code change
  — `uploadISOsViaSCP` already accepted an Executor.

- **`tst:f51f85bb:cidr-to-netmask-edge-no-test`** — done
  2026-05-04 — PR #322, merge commit `f60208f`. Tier H
  suggestion (trust-boundary-untested). `CIDRToNetmask` was
  well-tested for typical CIDRs (/0 /8 /12 /24 /32) but missing
  the off-by-one boundaries downstream HAProxy/dnsmasq template
  substitution depends on. Added 3 table rows: `/1` →
  `128.0.0.0`; `/31` → `255.255.255.254`; `/30` →
  `255.255.255.252`. All three values verified by hand against
  `^uint32(0) << (32 - bits)` arithmetic.

- **`sec:6424733c:cred-no-zeroize`** — done 2026-05-04 — PR
  #323, merge commit `d1d88d8`. Tier H major (credentials).
  `creds.Env()` materialised `PROXMOX_VE_PASSWORD`/`API_TOKEN`
  as immutable Go strings into `executor.Env`; the `deploy.go`
  defer of `creds.Zeroize()` cleared the source `[]byte` but
  could not reach the residual strings — plaintext credentials
  lived for the full 30-60 min Prepare → Install → Configure
  run. Added `Provisioner.ZeroizeEnv()`
  (`internal/distribution/okd/okd.go:178-194`) which walks
  `executor.Env`, blanks the credential-key entries, then
  `clear()`s and nils the slice; wired via
  `defer p.ZeroizeEnv()` after provisioner construction in
  `executeFullDeployment`
  (`internal/cli/helpers.go:198`). Lesson: `clear([]string)`
  zeros string headers but not the backing bytes — best-effort
  without `unsafe.Pointer`. Acceptable because the strings
  become unreachable for the next GC; a true byte-level wipe
  would need `unsafe` and there is currently no policy for
  introducing it.

- **`sec:696d6b0e:input-path-not-prefix-checked`** — done
  2026-05-04 — PR #324, merge commit `68753a0`. Tier H
  suggestion (file-toctou). `vmDevicesReferenceISO` matched
  `HasSuffix(seg, isoBase)` against device-mapping segments, so
  two ISOs sharing a basename across non-default Proxmox
  storages would alias. Pass `"iso/"+filepath.Base(f)` as the
  token from `RemoveFCOSISOFromProxmox`
  (`internal/distribution/okd/phase/iso_cleanup.go:232`) so the
  suffix anchors at the content-type boundary. Tests at
  `iso_cleanup_test.go:131,134,195,211` updated to mirror the
  new token form; the `.old`-suffix regression remains covered.
  Defense-in-depth only — default `local:iso/<name>` storage
  layout was unaffected. Lesson: helper parameters are still
  named `isoBase` for minimum-diff; rename to `isoToken` is a
  follow-up nit, not blocking.

- **`sec:6424733c:input-validation`** — done 2026-05-04 — PR
  #325, merge commit `21779e0`. Tier H suggestion
  (input-validation). `startMetricsServer` rewrote bare
  `:port` to `127.0.0.1:port` but accepted explicit
  `0.0.0.0:port` / `[::]:port` silently — the metrics endpoint
  is unauthenticated. Added `--metrics-allow-network` cobra
  Bool flag (`internal/cli/deploy.go:49`) and gated wildcard
  binds behind it via `net.SplitHostPort` +
  `netip.IsUnspecified()` in `startMetricsServer`
  (`internal/cli/helpers.go:152-160`); returns
  `errtypes.ConfigError` for the disallowed case. Picked up
  `con:6424733c:metrics-shutdown-bg-ctx` (missing
  `context.Background()` justification comment) as a two-for-one
  ratchet. Lesson: `make docs` regen is mandatory when adding a
  cobra flag — first push failed CI on `docs-go` drift in
  `docs/cli/okdctl_deploy.md`.

- **`sec:6424733c:input-path-not-prefix-checked`** — done
  2026-05-04 — PR #326, merge commit `860f8d6`. Tier H minor
  (input-validation). `resolveProjectRoot` swallowed every
  `EvalSymlinks` failure with `//nolint:nilerr`, returning the
  unresolved abs path to `runlock.Acquire`, every cleanup
  helper, and `ChownTreeToInvokingUser`. A symlink in cwd that
  produced a non-`ENOENT` `EvalSymlinks` error gave attacker-
  influenced paths to root-elevated code. Now only
  `os.ErrNotExist` falls back to abs (the documented
  macOS-temp-dir benign case); other errors propagate
  (`internal/cli/helpers.go:92-97`).
  `resolveProjectRootOrDie` additionally stats
  `filepath.Join(root, filepath.Base(cfgFile))` so a symlink
  resolving outside the project triggers
  `&errtypes.ConfigError{Err: errtypes.ErrConfigMissing}` before
  any sudo-elevated mutation runs
  (`internal/cli/helpers.go:111-130`). Lesson:
  `filepath.Base(cfgFile)` keeps the marker check inside `root`
  even when `--config` is an absolute path, because Go's
  `filepath.Join("/root", "/abs/file")` collapses to the
  absolute argument.

- **`sec:1e8ffb91:tls-insecure-permanent-skip`** — done
  2026-05-04 — PR #327, merge commit `7048903`. Tier H
  suggestion (tls-network). `verifyKubeVIPAPIHealthBootstrap`
  always built an `InsecureSkipVerify` client when the
  kubeconfig CA was unavailable; even after the apiserver
  re-issued its cert with the VIP in SANs, a continuous-
  monitoring caller would silently skip verification. Now the
  function attempts CA-verified first and falls back to
  insecure only on `errors.As(err, &x509.HostnameError{})` —
  the expected transient during the kube-vip cert re-issue
  window. Other TLS errors (expired, unknown CA) propagate
  without silent downgrade
  (`internal/distribution/okd/postinstall/verify.go:229-291`).
  Lesson: golangci-lint's `nolintlint` rule fires on unused
  `nolint:gosec` directives. `httputil.NewInsecure` is a
  wrapper, so gosec G402 doesn't trigger on the call site by
  name — the directives were nullops and had to be removed.

- **`state:15ba17da:destroy-no-precondition-resume`** — done
  2026-05-04 — PR #328, merge commit `e3e9a0b`. Tier H minor
  (phase-idempotency). `destroySteps()` had no auto-skip when
  cleanup targets were already absent — `StepCleanupFiles`
  blindly invoked `cleanup.Execute` on a missing
  `opts.WorkDir`, `StepCleanupFirewall` blindly ran
  `RemoveOKDRules` with no backend installed; both produced
  Success-with-warning instead of Skipped. Extended `SkipWhen`
  predicates inline:
  `system.DirExists(opts.WorkDir)` for files,
  `firewall.DetectBackend(...) == firewall.None` for firewall
  (`internal/distribution/okd/destroy/steps.go:86,118-121`).
  `cleanupFilesSkipReason` gained a "work directory absent"
  branch. Did NOT introduce the panic-on-missing `ReRunSafe`
  field — that is `state:4f69fc9d`'s territory and out of
  scope. Lesson: gofumpt rejects long single-line predicates
  inside a struct literal; CI flagged after the first push and
  the formatter wrapped the lambda body across two lines.

- **`err:d6b325cb:sentinel-not-matched`** — done 2026-05-04 —
  PR #329, merge commit `6663b81`. Tier H suggestion
  (sentinel-vs-typed). `proxmox.ErrNotConnected` and
  `ErrTerraformNotConfigured` were exported sentinels but every
  call site in `Provision` / `PlanOnly` returned them BARE;
  `cli/root.go::exitCodeFor` cannot match a bare sentinel via
  `errors.As`, so user-fixable config errors fell through to
  exit 1 instead of the documented exit 2. Wrapped each return
  in `&errtypes.ConfigError{Msg, Err: <sentinel>}`
  (`internal/infrastructure/proxmox/proxmox.go:133,141,201,209`);
  sentinels remain exported so `errors.Is(err, ErrNotConnected)`
  still matches via `Unwrap`. Lesson: the `feedback_scaffolding`
  rule applies — keep API-shaped exports; the fix wraps callers,
  it does not delete the symbol.

- **`sec:e3782ee7:toctou-chmod`** — done 2026-05-04 — PR #330,
  merge commit `fb33ee2`. Tier H suggestion (file-toctou).
  `WriteTempFile` called `os.CreateTemp` (kernel default
  0o600) then `f.Chmod(mode)` — a microscopic create-then-
  chmod window inconsistent with the helper's own doc comment
  promising mode-at-open semantics. Switched to `os.OpenFile`
  + `O_RDWR|O_CREATE|O_EXCL` with the caller's mode set at
  open time, replicating `os.CreateTemp`'s `*` substitution
  and 10000-iteration collision retry
  (`internal/system/fs.go:46-91`). Lesson: gosec G404 flags
  `math/rand.Uint32()` for tempfile naming even though
  `O_EXCL` provides the actual collision safety. Switched to
  `crypto/rand.Read` + `binary.BigEndian.Uint32` to satisfy
  the linter — not a security improvement, just a lint one.

- **`dep:33ef32bf:transitive-narrow-godotenv`** — done
  2026-05-04 — PR #331, merge commit `ce41e2f`. Tier H
  suggestion (transitive-weight). `github.com/joho/godotenv
  v1.5.1` was a single-call-site direct dep used only by
  `loadEnvFileOnce`. Replaced with a ~30-LOC `bufio.Scanner`
  parser
  (`internal/credentials/envfile.go:158-185`) that handles
  `key=value`, blank lines, `#` comments, and surrounding-quote
  stripping; preserves the no-overwrite contract via
  `os.LookupEnv` before `os.Setenv`. The new parser accepts
  `io.Reader` so tests don't mutate process env. Drops the dep
  from `go.mod`/`go.sum`. Lesson: golangci-lint's `nolintlint`
  rejected `//nolint:errcheck` on `defer f.Close()` for the
  read-only file; repo's `errcheck` config presumably already
  allowlists `(*os.File).Close`, so the suppression was
  redundant and had to be removed.

- **`state:c19ee328:setup-no-precondition-for-iso-rebuild`** —
  done 2026-05-04 — PR #332, merge commit `bc866ef`. Tier H
  suggestion (phase-idempotency). On partial-fail-and-resume,
  `StepBuildISOs` rebuilt every node ISO from scratch (~5 min)
  and `StepUploadISOs` re-scp'd multi-GB even when nothing
  changed. Added `nodeISOFingerprint`
  (`internal/distribution/okd/setup/iso.go:21-32`) hashing the
  (live kargs, dest kargs, sshKey, base ISO path) tuple per
  node; persisted via `system.AtomicWriteString` to
  `<isoDir>/.fp-<name>`. Skip path: output ISO exists AND
  fingerprint matches. Upload path filters via new
  `isoUploadNeeded` comparing local sha256 to remote
  `sha256sum` over SSH
  (`internal/distribution/okd/setup/upload.go:55-93`); fail-
  open on any error to preserve correctness. Lesson: per-node
  `.fp-<name>` files were a beneficial deviation from the
  contract's single combined `.iso-build-fingerprint` —
  granularity matches the per-node build loop and skips one
  node at a time on partial resume.

- **`sec:8ea706f6:cred-env-leak-to-child`** — done 2026-05-04
  — PR #333, merge commit `d99fd0e`. Tier H suggestion
  (credentials → seam→audit-subprocess). `getToolVersion` at
  `internal/distribution/okd/setup/tools.go:259-270` used raw
  `exec.CommandContext` with `cmd.Env` left nil — Go's
  `os/exec` interprets `nil` as "inherit `os.Environ()`", so
  every exported credential flowed into
  `terraform/oc/openshift-install --version` probes. Routed
  through canonical `system.OutputCaptured`, which sets
  `cmd.Env = executor.FilterParentEnv(executor.DefaultEnvAllowlist)`
  so child processes only see `PATH` and the documented
  allowlist. Lesson: a sibling raw `exec.CommandContext` for
  `lsb_release` lives at line 322 — outside the cited evidence,
  deliberately left alone for a follow-up audit-positive sweep.

- **`ux:d9f7733e:debug-bundle-skip-must-gather-no-quiet-suppress`**
  — done 2026-05-04 — PR #335, merge commit `6208372`. Tier H
  suggestion (streams). `debug-bundle`'s Long help text named
  `-o` and `--log-file` but said nothing about progress logs going
  to stderr or about the global `--quiet` flag suppressing them.
  Added one paragraph to `internal/cli/debug_bundle.go:53-57`
  pointing users at `--quiet` for clean script/CI output and
  regenerated `docs/cli/okdctl_debug-bundle.md` to match. No code
  logic change. Lesson: any Long-text edit needs `make docs` in
  the same commit — CI's docs-drift gate fired on the first push
  because the regenerated reference was missing; round-trip cost
  one force-push. Fold `make docs` into the local pre-commit when
  Long/Short text changes.

- **`api:a7f4383d:export-no-caller-scaffolding`** — done 2026-05-04
  — PR #336, merge commit `00b5cfc`. Tier H suggestion
  (exported-surface; scaffolding — verify intent only). The
  `okd.ClusterStatus` / `NodeStatus` / `Condition` / `ClusterPhase`
  exports plus six `PhaseXxx` constants in `internal/distribution/okd/types.go`
  had zero callers; `internal/cli/status.go` was carrying parallel
  `clusterStatus` / `nodeStatusEntry` / `addonStatusEntry` types.
  Migrated `runStatus` to build `okd.ClusterStatus`, deleted the
  three local types, switched `printClusterStatus` to a pointer
  receiver to satisfy gocritic's `hugeParam` (the type is now 168
  bytes), added JSON tags + three additive fields (`APIReachable`,
  `DegradedOperators`, `Addons`) plus a new `AddonStatus` helper
  struct so the documented JSON keys are preserved. `phase` and
  per-node `status` are additive keys allowed by `docs/cli/json-schema.md`'s
  evolution policy. Three of six `PhaseXxx` constants
  (`PhaseRunning`, `PhaseDegraded`, `PhaseUnknown`) are now wired
  to runtime; the other three remain as scaffolding for future
  deploy-state surfaces (per MEMORY.md `feedback_scaffolding`).
  Lesson: gocritic's `hugeParam` threshold (~80 bytes) bites when
  scaffolding types accrete fields — switch to pointer receivers
  proactively when the struct grows past a handful of fields. Also:
  shadow-risk variable name `status` in the addon-render loop was
  renamed `addonHealth` to avoid the appearance of shadowing the
  new `Status` field on `okd.NodeStatus`.

- **`api:48688e63:pkg-facade-bypassed`** — done 2026-05-04 — PR
  #338, merge commit `c8848ca`. Tier H minor (package-boundary).
  Domain logic in `internal/infrastructure/proxmox` and
  `internal/distribution/okd/install/monitor.go` imported
  `internal/tui` to call `tui.StartSpinner`, leaking UI/presentation
  concerns into provisioning logic and preventing reuse from a
  non-TTY caller. Added `logutil.ProgressReporter` (callback type)
  + `logutil.NopProgressReporter` (default no-op); threaded it
  through `proxmox.Provider` (new `WithProgressReporter` option),
  `install.Phase` (new exported `Reporter` field, set from
  `okd.Install` alongside `Recorder`), and `okd.Provisioner`
  (new `WithProgressReporter` option). CLI helpers added
  `tuiReporter(ctx)` wrapping `tui.StartSpinner` and registered it
  via `okd.WithProgressReporter` at the
  `executeFullDeployment` provisioner-construction site
  (`internal/cli/helpers.go:244`). Removed the `internal/tui`
  import from both target packages. Two test struct-literals updated
  to set `Reporter: logutil.NopProgressReporter` to avoid
  nil-func panic. Lesson: for symmetry with the existing exported
  `Recorder` field on `install.Phase`, keep `Reporter` as an
  exported field assigned from the parent `Install()` call rather
  than introducing a one-off functional option.

- **`state:4f69fc9d:no-resume-checkpoint`** — done 2026-05-04 — PR
  #339, merge commit `4db1375`. Tier H minor (phase-idempotency).
  `StepDef` had no idempotency declaration; `Orchestrator.Run`
  iterated every step from index 0 on each invocation, so a
  mid-phase crash re-ran slow generators (ISO build, ignition gen,
  manifest gen) on every retry. Added typed enum `ReRunSafety`
  (`Unset`/`Yes`/`No`) plus a new `AlreadyDone func(ctx) (bool, error)`
  field to `StepDef`; `BuildSteps` panics on `ReRunSafeUnset`
  (zero value) so every literal must commit. All 36 existing
  `StepDef` literals across `setup/install/postinstall/destroy`
  declare `ReRunSafe` explicitly (setup 19, install 7, postinstall
  5, destroy 5). New `AlreadyDoneChecker` interface
  (`internal/distribution/step.go:55-57`); `Orchestrator.executeStep`
  consults it after `SkipWhen` and before `Exec`, recording Skipped
  with reason "already done" on a true return. `destroyTracker`
  factored out of `destroySteps()` to clear the `funlen 120`
  threshold after the new `ReRunSafe` lines pushed the function
  over. Stretch run-state.json persistence is deferred — the panic
  lever and `AlreadyDone` hook are the necessary scaffolding;
  durable checkpoint can land in a follow-up. Lesson: typed enum
  with zero-value-as-sentinel beats `*bool` for required-field
  semantics — callers write `ReRunSafe: ReRunSafeYes` inline,
  matching the existing `NonFatal: true` ergonomics in every
  `StepDef`, while the zero value remains the panic trigger.

## Bulk archive 2026-05-05

Bulk-moved 140 done entries from `roadmap.md` to keep the active file readable. Each entry preserves its original heading level (`####` for theme items, `#####` for audit-tier items) and full body (status, severity, cluster, evidence, problem, fix, effort, plus any postmortem prose).

#### F1 — Restore exported-symbol docs stripped by comment-hygiene

**Status:** done — 2026-04-22 (commit 73912b5)
**Audit:** 21 findings with prefix `doc:*:exported-doc-missing`
**Evidence:** `golangci-lint run` reports ~50 revive `exported`
violations across 16 files after the 2026-04-21 hygiene pass.
**Problem:** The `/comment-hygiene` skill prunes comments when the
`WHY` isn't clearly articulated. When applied repo-wide it stripped
contract docs from exported symbols the revive `exported` rule
requires a comment on. The stripped symbols include canonical types
(`install.Phase`, `postinstall.Phase`, `setup.Phase`), validator
helpers (`IsValidIP`, `ValidateClusterName`, etc.), logger entry
points (`tui.Info`/`Warn`/`Error`), and security-adjacent helpers
(`system.FileExists`, `AtomicWriteString`).
**Scope:** Restore concise verb-first doc comments on each flagged
symbol. Per-file fix actions with canonical restore phrasing are
captured in each finding's `fix_summary` field in the JSONL artifact.
Group-header comments on `const` blocks cover multiple sites at once.
Net LOC delta: ≈+110.
**Effort:** hours (mechanical). The hygiene-pass skill should also
gain a "skip exported-symbol docs" carve-out before being re-run
against the whole repo.


#### F2 — Trim 3 over-long package docs

**Status:** done — PR #113
**Audit:** `doc:a9ea115f:pkg-doc-too-long`,
`doc:a4001485:pkg-doc-too-long`,
`doc:48688e63:pkg-doc-too-long`
**Evidence:** `internal/addon/catalog/catalog.go:1-15`,
`internal/errtypes/errtypes.go:1-8`,
`internal/infrastructure/proxmox/proxmox.go:1-13`.
**Problem:** Three package doc blocks run past the CLAUDE.md 1-3
sentence ceiling — catalog carries a 5-step "adding an addon"
walkthrough, errtypes embeds a credential-redaction invariant that
belongs on the type not the package, proxmox carries a lifecycle
example block.
**Scope:** Trim each to 1-3 sentences. Move the catalog walkthrough
to `docs/addons/` (directory already exists). Move errtypes
redaction invariant to a type-level comment. Move proxmox lifecycle
example to method docs on `Connect`/`Provision`/`Disconnect`.
**Effort:** one sitting.


#### F3 — Regenerate docs/cli/okdctl_destroy.md

**Status:** done — docs commit d30866a
**Audit:** `doc:54654337:destroy-cli-ref-stale`
**Evidence:** `docs/cli/okdctl_destroy.md:26-32`.
**Problem:** The generated CLI reference is missing the three
resume-after-partial-destroy flags added in afa579b
(`--skip-terraform`, `--skip-cleanup`, `--skip-firewall`). README
already prescribes `make docs` before tagging; this is the output of
skipping that step when landing afa579b.
**Scope:** `make docs` + commit. No code changes.
**Effort:** minutes.


#### F4 — Fix BuildOpaqueSecret arg-order doc drift

**Status:** done — docs commit d30866a
**Audit:** `doc:66cb1c69:addons-buildopaquesecret-sig`
**Evidence:** `docs/architecture/addons.md:141-142` vs
`internal/addon/helpers.go:46`.
**Problem:** Architecture doc shows
`BuildOpaqueSecret(name, namespace, data)` but the canonical helper
signature is `(namespace, name, data)`. A new addon author following
the doc constructs a Secret in the wrong namespace.
**Scope:** One-line doc edit. Do NOT flip the Go signature —
CLAUDE.md §architecture-notes names BuildOpaqueSecret as canonical;
fix the doc, not the API.
**Effort:** minutes.


#### F5 — Fix wizard-registration doc stale path

**Status:** done — docs commit d30866a
**Audit:** `doc:70b3bae2:wizard-registration-stale`
**Evidence:** `docs/architecture/wizard.md:38-39`.
**Problem:** Doc tells step authors to register in
`internal/tui/wizard/wizard.go` — file doesn't exist; real site is
`StepBuilder.Register` in `internal/tui/wizard/builder.go:24`.
**Scope:** Swap the filename in the doc. No code changes.
**Effort:** minutes.


#### E1 — Concurrent-run lock with stale-PID detection

**Status:** done — PR #118
**Audit:** `state:4c092fce:no-concurrent-run-guard`
**Evidence:** `internal/infrastructure/terraform/terraform.go:119`
**Problem:** Two concurrent `okdctl deploy` or `okdctl destroy` runs in
the same project root both target
`infrastructure/terraform/environments/<env>/terraform.tfstate` with no
mutual exclusion. Terraform's own state lock fires only per-operation,
so racing `okdctl apply` → `tf plan` → `tf apply` against a sibling
`okdctl destroy` yields interleaved applies and a corrupted state.
**Scope:** Add a process-level advisory lock under
`<projectRoot>/.okdctl.lock` taken in `cli/deploy.go` and
`cli/destroy.go` before the phase orchestrator runs. Must detect stale
locks (prior run died via SIGKILL) without racing a live sibling; the
usual PID-file + `kill -0` trick has a reuse window. Consider
`flock(LOCK_EX|LOCK_NB)` with the PID written into the file for
human diagnostics. Must unlock on normal exit and on ctx cancel.
**Effort:** days. Why it's filed here not done inline: stale-detection
design has to be thought through (PID reuse, cross-host NFS homes, the
sudo-re-exec crossing).


#### E2 — Ring-buffered / streamed executor output

**Status:** done — PR #119
**Audit:** `sub:7b2829bb:unbounded-output-buffer`,
`sub:4c092fce:terraform-buffered-through-executor`
**Evidence:** `internal/executor/executor.go:116`,
`internal/infrastructure/terraform/terraform.go:148`
**Problem:** `executor.Executor.Run` buffers full stdout+stderr into
`bytes.Buffer` with no cap. `terraform apply` on a cluster with many
VMs, or `openshift-install` on a long bootstrap, materializes tens of
MB in RAM before returning. Plus the user sees nothing until it
completes.
**Scope:** Ring-buffered trail (keep only the last N lines) for error
messages plus a streaming variant (`RunStreamed`) that pipes live to
stdout/stderr. Already have `PlanStreamed` and `ApplyStreamed` on the
terraform wrapper for progress visibility; generalize the pattern at
the executor layer. Keep `RunChecked` semantics for short-output
callers.
**Effort:** days.


#### E6 — kube-vip probe TLS: use cluster CA once available

**Status:** done — PR #124 (moved to Completed)


##### `api:262af6e4:zero-value-usable-cleanup` — zero value usable cleanup

**Status:** done — PR #115  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/cleanup/cleanup.go:50-107`  
**Problem:** cleanup.Execute takes *Options whose zero Kind yields a bare '*errtypes.ConfigError{Msg: "unknown cleanup type: ..."}' with no sentinel callers can match. An Options{} zero-value (no Logger, no Kind) also silently defaults to NopLogger.  
**Fix:** Add an exported `var ErrKindNotSet = errors.New(...)` sentinel and have Execute return it wrapped when opts.Kind == ''. Alternatively, switch to `NewOptions(kind Kind) *Options` so the required field is a constructor parameter, mirroring destroy.NewOptions / install.NewOptions.  
**Effort:** hours


##### `api:125729c4:opt-inconsistent-cfg-opts` — opt inconsistent cfg opts

**Status:** done — PR #115  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/destroy/phase.go:40-50`  
**Problem:** Phase NewOptions factory shapes still diverge across siblings. setup.DefaultOptions(projectRoot) takes ONLY projectRoot; install.NewOptions, postinstall.NewOptions, destroy.NewOptions take (cfg, projectRoot).  
**Fix:** Rename setup.DefaultOptions to setup.NewOptions(cfg, projectRoot) and fold any cfg-driven defaults into it. Matches the (cfg, projectRoot) signature the other three phase packages share.  
**Effort:** hours


##### `api:c287d5c0:withenv-order-coupling` — withenv order coupling

**Status:** done — PR #115  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/okd.go:61-98`  
**Problem:** okd.WithEnv still encodes an order-dependency contract in New: WithEnv may construct the executor before WithLogger runs, and New compensates after the loop by re-applying WithLogger to the now-existing executor. Option functions should be commutative; any future lazy-building option (e.g.  
**Fix:** Defer executor construction until after the option loop: WithEnv stores pendingEnv []string on *Provisioner, then New builds p.executor = executor.New(executor.WithLogger(p.logger), executor.WithEnv(p.pendingEnv)) once. Options stay pure setters; the constructor owns ordering.  
**Effort:** hours


##### `api:4c092fce:opt-inconsistent-terraform-ctors` — opt inconsistent terraform ctors

**Status:** done — PR #115  
**Severity:** minor  
**Evidence:** `internal/infrastructure/terraform/terraform.go:109-136`  
**Problem:** terraform package still exports two constructors — New(workDir, opts...) and NewWithVarFile(workDir, varFile, opts...) — that differ only in one default. The second is a thin wrapper solely to preset VarFile.  
**Fix:** Add `func WithVarFile(path string) Option` that sets e.VarFile. Delete NewWithVarFile.  
**Effort:** hours


##### `api:830d4653:export-no-caller-installed-lists` — export no caller installed lists (scaffolding — verify intent only)

**Status:** done — PR #115  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/cleanup/packages.go:34-53`  
**Problem:** cleanup.InstalledPackages and cleanup.InstalledBinaries are exported but their only callers are the package-private Packages() function at line 66 and 72. No external caller in-tree.  
**Fix:** Verify intent — if a preview/plan CLI verb is planned, keep and document. Otherwise unexport (installedPackages, installedBinaries).  
**Effort:** hours


##### `api:ed55ee90:export-no-caller-generate-summary` — export no caller generate summary

**Status:** done — PR #115  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/cleanup/summary.go:11-57`  
**Problem:** cleanup.GenerateSummary and cleanup.Summary struct are exported but the only caller is the package-private printSummary(). No external caller.  
**Fix:** Either unexport (GenerateSummary -> generateSummary, Summary -> summary) if no CLI surface planned, or keep both exported and add a one-line doc-comment pointing to the intended caller (e.g. 'used by okdctl cleanup status; see roadmap.md').  
**Effort:** hours


##### `api:d7ce9d16:export-no-caller-dns-config-helpers` — export no caller dns config helpers

**Status:** done — PR #115  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/dns/dns.go:23-128`  
**Problem:** dns.BuildConfigData, dns.ConfigName, and dns.WriteDnsmasqConfig remain exported with callers only inside the dns package. dns.GenerateBootstrapConfig has a single external caller in setup/steps.go:368.  
**Fix:** Unexport: buildConfigData, configName, writeDnsmasqConfig. Keep GenerateBootstrapConfig, DeployBootstrap, DeployProduction, Setup, RestoreSystemResolver as the package's external API.  
**Effort:** hours


##### `api:ae5b624c:concrete-return-k8s` — concrete return k8s

**Status:** done — PR #115  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/install/monitor.go:54-63`  
**Problem:** K8sClient is used in monitor.go only for ApprovePendingCSRs. Rather than accepting a concrete *cluster.K8sClient in MonitorInstallation, the caller could define a tiny consumer-side interface `type csrApprover interface { ApprovePendingCSRs(context.Context) (int, error) }` at the install package.  
**Fix:** Inside install package define `type csrApprover interface { ApprovePendingCSRs(ctx context.Context) (int, error) }`. Accept it as a parameter to MonitorInstallation, defaulting to NewK8sClient(...).  
**Effort:** hours


##### `api:73ad30ef:export-no-caller-external-tool-binaries` — export no caller external tool binaries

**Status:** done — PR #115  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/phase/paths.go:96-105`  
**Problem:** phase.ExternalToolBinaries has one in-tree caller (cleanup/packages.go:52). Exported for the sole purpose of avoiding a setup→cleanup import.  
**Fix:** Move ExternalToolBinaries to a new phase/tools.go with a one-line doc clarifying 'these binaries are installed by setup; cleanup removes them.' No callsite change.  
**Effort:** hours


##### `api:beabab0c:mix-default-new-naming` — mix default new naming

**Status:** done — PR #115  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/setup/phase.go:34-42`  
**Problem:** setup.DefaultOptions continues the Default* naming pattern common for 'zero-arg constructor of a defaulted options struct'. Other phase packages use NewOptions(cfg, projectRoot).  
**Fix:** Rename setup.DefaultOptions -> setup.NewOptions; accept (cfg, projectRoot) and fold any cfg-driven defaults inside. Single call site in okd.go updates.  
**Effort:** hours


##### `ux:024a2c32:json-schema-doc-drift` — json schema doc drift

**Status:** done — docs commit d30866a — docs/cli/json-schema.md rewritten to match actual marshaled shapes; golden-test deferral tracked via audit-tests gap entry  
**Severity:** major  
**Evidence:** `docs/cli/json-schema.md:12-67`  
**Problem:** docs/cli/json-schema.md documents field shapes that do not match what the code emits. `okdctl status --format=json` is documented with cluster_name/version/ready_nodes/total_nodes but emits api_reachable/nodes/degraded_operators/addons.  
**Fix:** Option (a, preferred): update docs/cli/json-schema.md to match code — for status enumerate api_reachable, nodes[{name,role,ready}], degraded_operators, addons[{name,healthy,error?}]; for releases list enumerate the flat OKDVersion array including release_type (int, document the 0-4 encoding or switch code to emit the string label). Add fixture-based golden tests (status_test.go, releases_test.go) that compare marshaled output against the doc so future drift fails CI.  
**Effort:** hours


##### `ux:54654337:readme-flag-drift` — readme flag drift

**Status:** done — docs commit d30866a — `make docs` regenerated docs/cli/okdctl_destroy.md with the three skip-* flags  
**Severity:** minor  
**Evidence:** `docs/cli/okdctl_destroy.md:26-33`  
**Problem:** Generated CLI reference for `okdctl destroy` is stale: commit afsd79b added --skip-terraform, --skip-cleanup, --skip-firewall to destroy.go, but docs/cli/okdctl_destroy.md still lists only --confirm-cluster, --dry-run, -h/--help, --keep-isos, -y/--yes. CI's docs-drift check (.github/workflows/ci.yml: `git diff --quiet docs/cli/`) would fail on this state.  
**Fix:** Run `make docs` (or `go run ./cmd/okdctl-gen-docs`) and commit docs/cli/. The regenerator is already in CI (.github/workflows/ci.yml:65); the drift is pre-commit residue from the afsd79b work.  
**Effort:** hours


##### `ux:073d24ed:dry-run-yes-short-circuit` — dry run yes short circuit

**Status:** done — PR #108  
**Severity:** minor  
**Evidence:** `internal/cli/deploy.go:78-80`  
**Problem:** runDeploy checks deployYes before deployDryRun and returns after saving the config, so `okdctl deploy --yes --dry-run` silently skips the dry-run preview the user asked for. --yes is documented as 'skip prompts, use defaults' and --dry-run as 'preview terraform plan and step listing without deploying' — the combination should still preview, not no-op into a config save.  
**Fix:** Reorder the guard: if deployDryRun { return runDeployDryRun(ctx, cfg) } BEFORE the deployYes short-circuit, or gate the --yes fast-path on !deployDryRun. Matches runDestroy (destroy.go:71-73) which checks destroyDryRun first.  
**Effort:** hours


##### `ux:d31d1b9d:json-key-hyphenated` — json key hyphenated

**Status:** done — PR #105  
**Severity:** minor  
**Evidence:** `internal/cli/status.go:338-353`  
**Problem:** runDescribeAddon emits JSON with a hyphen-cased key `display-name` while every other field in the same payload and every other JSON endpoint uses snake_case (api_reachable, ready_nodes, degraded_operators, release_date, release_type). jq consumers have to quote the field: `jq '."display-name"'`, which is a pain-point.  
**Fix:** Rename the JSON key to display_name in the lines slice when describeFormat == outputJSON. Text mode can keep the hyphenated label (it is human-facing and reads as a single phrase).  
**Effort:** hours


##### `ux:e45c2239:sig-not-handled-preflight` — sig not handled preflight

**Status:** done — PR #108  
**Severity:** suggestion  
**Evidence:** `cmd/okdctl/main.go:20-23`  
**Problem:** main() calls preflight() before cli.Execute(); signal.Notify setup lives inside internal/cli/root.go:execute(). If the user hits Ctrl-C during preflight's euid check, OKDCTL_BIN_DIR validation, or PATH mutation, the process dies with SIGINT default (no partial summary, undocumented behavior).  
**Fix:** Either (a) accept current behavior (document in main's package comment: 'preflight runs before signal setup; it is small enough that interruption is racy but not harmful') or (b) move signal.Notify earlier into main() and pass ctx through preflight. Only pay for (b) if preflight grows (e.g.  
**Effort:** hours


##### `ux:93957c53:cleanup-no-dry-run` — cleanup no dry run

**Status:** done — PR #108  
**Severity:** suggestion  
**Evidence:** `internal/cli/cleanup.go:18-34`  
**Problem:** cleanupCmd has no --dry-run flag while its destructive siblings (deploy, destroy, update-ingress) all do. cleanup removes packages, dnsmasq/haproxy configs, ignition files, and terraform state — destructive enough that a preview flag has the same value proposition.  
**Fix:** Add cleanupDryRun bool and --dry-run flag. Branch to a runCleanupDryRun before the confirmation prompt; enumerate what would be removed (work directory path, haproxy config block, dnsmasq drop-in path, packages to uninstall) via the existing cleanup.Options struct — do not call cleanup.Execute.  
**Effort:** hours


##### `ux:8d8faa80:completion-use-bracket-optional` — completion use bracket optional

**Status:** done — PR #108  
**Severity:** suggestion  
**Evidence:** `internal/cli/completion.go:11-11`  
**Problem:** completionCmd.Use is 'completion [bash|zsh|fish|powershell]' — square brackets per man(1) convention mean optional, but cobra.ExactArgs(1) rejects zero-arg. The shell token is required; Use should render `<bash|zsh|fish|powershell>`.  
**Fix:** Change Use to 'completion <bash|zsh|fish|powershell>'. Same pattern as internal/cli/addon.go:67 ('uninstall <name>') and releases.go:53 ('show <version>').  
**Effort:** hours


##### `ux:e7db1220:releases-show-no-completion` — releases show no completion

**Status:** done — PR #108  
**Severity:** suggestion  
**Evidence:** `internal/cli/releases.go:52-59`  
**Problem:** addon install/uninstall and describe-addon gained ValidArgsFunction for tab-completion; releasesShowCmd still has none. Tab-completing `okdctl releases show <TAB>` does filesystem completion instead of version suggestions.  
**Fix:** Add ValidArgsFunction that reads the disk cache (releases.NewOKDVersionFetcher has a cache-backed path) and returns Versions + ShellCompDirectiveNoFileComp. Fall through to ShellCompDirectiveError when the cache is empty rather than fetching on tab — keeps completion latency under the 1s shell threshold.  
**Effort:** hours


##### `ux:aa84670c:exit-code-bsd-sysexits-partial` — exit code bsd sysexits partial

**Status:** done — PR #108  
**Severity:** suggestion  
**Evidence:** `internal/cli/root.go:144-162`  
**Problem:** exitCodeFor maps ConfigError=2 (not EX_DATAERR=65 or EX_CONFIG=78), NetworkError=3 (not EX_UNAVAILABLE=69), ClusterError=4 (not EX_UNAVAILABLE=69), AuthError=5 (not EX_NOPERM=77). The taxonomy IS published at the package doc (root.go:1-8).  
**Fix:** Keep the current mapping for backward compatibility (scripts may pin 2/3/4/5); add a regression test in root_test.go asserting each typed error reaches the right code, and --version / help exit 0. Optionally introduce --exit-code-mode={compat|sysexits} for opt-in BSD mapping.  
**Effort:** hours



##### `smell:daf5bee9:yaml-tree-walk-repeat-assertion` — yaml tree walk repeat assertion

**Status:** done — PR #110  
**Severity:** suggestion  
**Evidence:** `internal/cli/kubeconfig.go:141-168`  
**Problem:** mergeNamedList has four nested type-assertion chains to walk a generic YAML tree (any → []any → map[string]any → map[string]any['name'] → string). Function works and the any is load-bearing (YAML unmarshal targets `any` for open schemas), but the walk is tightly coupled to one semantic (merge-by-name) so the any-ness doesn't buy reuse.  
**Fix:** Either (a) declare a minimal kubeconfig schema (ClustersList, UsersList, ContextsList) and yaml-unmarshal into typed slices, then merge; or (b) extract a `namedEntries(v any) map[string]any` helper so the tree walk lives in one place. (a) is the clean fix but adds types the package doesn't need elsewhere; (b) preserves the any-based approach but shrinks the walk to one site.  
**Effort:** hours


##### `smell:004ad79b:helper-pkg-thin-wrap` — helper pkg thin wrap

**Status:** done — PR #110  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/packages/packages.go:1-42`  
**Problem:** Package `packages` wraps `platform.PackageManager.Install`/`Remove` with an extra logger.Info() envelope and a single `fmt.Errorf` rewrap. Three call sites consume it (setup/steps.go:305, cleanup/packages.go:67, cleanup/services.go:41).  
**Fix:** Inline the two functions at their call sites (setup/steps.go:305, cleanup/packages.go:67, cleanup/services.go:41) and delete internal/distribution/okd/packages. The logger.Info lines are already repeated in the callers' surrounding context.  
**Effort:** hours


##### `smell:1d5afa08:enum-via-sscanf-int-parse` — enum via sscanf int parse

**Status:** done — PR #110  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/releases/types.go:59-94`  
**Problem:** OKDVersion.Major() and OKDVersion.Minor() parse the Version string via fmt.Sscanf on every call. ShortVersion calls both (two parses per call), and callers invoke the methods inside filter loops (fetcher.go parseReleases and sortAndClassifySeries, cli/releases.go), so a list of ~40 releases runs dozens of Sscanf parses per display even though the version string is immutable per OKDVersion.  
**Fix:** Parse once at unmarshal time (or memoize). Either (a) add unexported `major, minor int` fields and populate them in the fetcher's parseVersionTag flow (fetcher.go:241 already runs Sscanf — fold the result into the struct), or (b) use `strings.Cut(v.Version, ".")` + strconv.Atoi, which is faster and avoids the fmt machinery.  
**Effort:** hours


##### `smell:c5e5c304:build-role-helper-near-duplicate` — build role helper near duplicate

**Status:** done — PR #110  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/setup/terraform.go:20-34`  
**Problem:** buildISOStrings and buildNodeNames in setup/terraform.go are structurally identical: allocate []string of length count, loop `for i := range count`, format `"%s:iso/%s%d.iso"` vs `"%s-%s%d"`. Both take (isoStorage/clusterName, phase.NodeRole, count).  
**Fix:** Introduce `buildQuotedRoleList(format string, prefix string, role phase.NodeRole, count int) []string` that takes a format string with two %s + one %d and renders count elements. Both sites collapse to one-liners.  
**Effort:** hours


##### `smell:c5e5c304:named-return-unnecessary` — named return unnecessary

**Status:** done — PR #110  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/setup/terraform.go:36-48`  
**Problem:** getDiskSizes returns `(cpDisk, workerDisk, workerDataDisk, masterDataDisk int)` — four unnamed integers with no semantic ordering. The named returns document the positional identity, but the real signal a caller needs is 'which is which'.  
**Fix:** Replace the 4-int tuple with a `type diskSizes struct { cpOS, workerOS, workerData, masterData int }` and return the struct. The single caller (buildTerraformVarsData) then assigns by field name, not by position.  
**Effort:** hours


##### `smell:4f69fc9d:stepbuilder-build-no-callers` — stepbuilder build no callers (scaffolding — verify intent only)

**Status:** done — scaffolding retained per MEMORY.md  
**Severity:** suggestion  
**Evidence:** `internal/distribution/step.go:155-173`  
**Problem:** distribution.StepBuilder.Build() has no external callers; every production path goes through BuildSteps → MustBuild, and MustBuild is Build's only caller. Build's stated value is 'returns an error only when b is nil' but NewStepBuilder is the only way to get a *StepBuilder and it never returns nil, so the error is unreachable.  
**Fix:** Keep. Build/MustBuild is the canonical pair for fluent builders in Go (errors.New + errors.Must, template.New + template.Must, etc.), and CLAUDE.md §architecture-notes names StepDef + BuildSteps as canonical.  
**Effort:** hours


##### `smell:0934cf1b:query-match-mini-dsl` — query match mini dsl

**Status:** done — PR #110  
**Severity:** suggestion  
**Evidence:** `internal/platform/packages.go:100-115`  
**Problem:** Manager.IsInstalled uses a bespoke `queryMatch` string substring to distinguish "installed" from "purged" on dpkg output. The logic is correct but the two-branch design (empty → exit-code-only, non-empty → substring match) is a mini-DSL inside a single method.  
**Fix:** Replace the `queryMatch` string field with `postCheck func(stdout []byte, pkg string) bool` on Manager. Set it to a no-op in the RHEL constructor and a dpkg-ii-prefix check in the Debian constructor.  
**Effort:** hours



##### `con:39c75e91:go-no-wait` — go no wait

**Status:** done — acceptance note (CLAUDE.md §Concurrency)  
**Severity:** suggestion  
**Evidence:** `internal/cli/confirm.go:22-45`  
**Problem:** promptForConfirmation spawns a reader goroutine that blocks on bufio.Reader.ReadString, races against ctx.Done, and on ctx cancel the goroutine remains blocked on Stdin.Read until the user presses enter or the process exits. Thoroughly documented in the function header; the capacity-1 inputCh means the goroutine's eventual send never deadlocks — but this is still an unowned goroutine whose lifetime is bounded only by the parent process.  
**Fix:** Go 1.25 has no portable cross-platform stdin cancellation; the current design is the least-bad option for a CLI prompt. CLAUDE.md §Concurrency already names "documented leak bound" as an accepted exception — this site satisfies it.  
**Effort:** hours


##### `con:484b40f0:lock-held-during-write` — lock held during write

**Status:** done — PR #109  
**Severity:** suggestion  
**Evidence:** `internal/deploymetrics/metrics.go:75-84`  
**Problem:** Handler holds r.mu.Lock() across fmt.Fprint(w, b.String()) — writing to an http.ResponseWriter under the mutex. A slow Prometheus scraper or stalled network connection blocks every StepStarted/StepFinished call in the deploy path until the write completes, coupling scrape latency to deploy latency.  
**Fix:** Build the rendered metrics string under the lock, release the lock, then write to w: r.mu.Lock(); var b strings.Builder; r.writeMetrics(&b); out := b.String(); r.mu.Unlock(); fmt.Fprint(w, out). The renderer writes to a local Builder so it can't race; the net write happens outside the critical section.  
**Effort:** hours


##### `con:ae5b624c:go-leak-on-error` — go leak on error

**Status:** done — acceptance note (CLAUDE.md §Concurrency)  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/install/monitor.go:65-150`  
**Problem:** MonitorInstallation spawns a goroutine holding installCmd.Wait(). On ctx cancel the function calls killInstall, waits up to 30s via reapTimer for Wait() to return, then abandons — leaving the goroutine still blocked on the (now-killed) process's Wait() until the OS reaps it.  
**Fix:** Pattern is sound and CLAUDE.md §Concurrency now names it as the canonical cmd.Wait reap-with-deadline example. Optional improvement: promote the reap-with-deadline shape to a shared helper in internal/distribution/okd/phase/ when a second caller (e.g.  
**Effort:** hours


##### `con:8e65d574:go-no-wait` — go no wait

**Status:** done — acceptance note (CLAUDE.md §Concurrency)  
**Severity:** suggestion  
**Evidence:** `internal/version/updatecheck.go:40-53`  
**Problem:** BackgroundCheck spawns a fire-and-forget goroutine that runs runCheck(ctx); printUpdateNotice in cli/root.go waits at most 100ms before returning, so on the happy path the goroutine races to completion and on the slow path it leaks until the process exits. CLAUDE.md §Concurrency now names this as the canonical fire-and-forget example, so the pattern is fully grounded — kept as a long-term advisory that future cross-references should pin line numbers rather than re-raise.  
**Fix:** No code change required — CLAUDE.md §Concurrency now pins this site as the canonical example. Optional future improvement: expose a Done() <-chan struct{} so an integration test can synchronously wait on BackgroundCheck to finish (currently testable only via cache-populate workarounds).  
**Effort:** hours



##### `dep:33ef32bf:yaml-quad-engines` — yaml quad engines

**Status:** done — acceptance note (CLAUDE.md §Dependencies)  
**Severity:** minor  
**Evidence:** `go.mod:20-60`  
**Problem:** Four YAML engines in the tree: sigs.k8s.io/yaml (direct), go.yaml.in/yaml/v2 (via k8s), go.yaml.in/yaml/v3 (via cobra/doc + kube-openapi), gopkg.in/yaml.v3 (via go-proxmox + testify + charm/log). Binary ships N engines even though only sigs.k8s.io/yaml is directly imported.  
**Fix:** Document the split in CLAUDE.md §Dependencies: sigs.k8s.io/yaml is REQUIRED for k8s Secret marshaling (JSON-tag respect); the three transitive engines are pulled by upstream deps we can't control (k8s, cobra, testify). No action on the code; action is on documentation so future PRs don't accidentally try to 'consolidate' and break the k8s addon path.  
**Effort:** hours


##### `dep:33ef32bf:ultraviolet-pseudo-version` — ultraviolet pseudo version

**Status:** done — acceptance note (charm ecosystem convention)  
**Severity:** minor  
**Evidence:** `go.mod:27-27`  
**Problem:** github.com/charmbracelet/ultraviolet is pinned to a pseudo-version (commit SHA, not a tagged release) — the project has never cut a tag. Pulled at three different pseudo-versions by charm.land/bubbles, lipgloss+log, and bubbletea; MVS picks the newest.  
**Fix:** Acceptance note. Charm ecosystem convention is that ultraviolet (the internal renderer) is not publicly-tagged.  
**Effort:** hours


##### `dep:b803fcb7:workflow-pin-hygiene-clean` — workflow pin hygiene clean

**Status:** done — acceptance note (tripwire)  
**Severity:** suggestion  
**Evidence:** `.github/workflows/ci.yml:1-119`  
**Problem:** Pin hygiene audit: every GitHub Action in .github/workflows/ is pinned by full 40-char SHA with the version tag in a trailing comment (actions/checkout, setup-go, golangci-lint-action, codeql-action, goreleaser-action, cosign-installer, sbom-action, setup-terraform, label-sync, labeler, slsa-github-generator, shellcheck, setup-tflint, attest-build-provenance). Go-install tools pinned by exact version (govulncheck v1.1.4, yamlfmt v0.14.0, terraform 1.10.3, golangci-lint v2.11.4).  
**Fix:** Optional tripwire: add a CI guard that fails if any workflow introduces a non-SHA action ref. Example — a lightweight rule in a new lint job running a regex over .github/workflows/ that flags `uses: org/name@tag` (where tag is not 40 hex).  
**Effort:** hours


##### `dep:87db21a9:goreleaser-action-version-tag` — goreleaser action version tag

**Status:** done — acceptance note (cosign trust model)  
**Severity:** suggestion  
**Evidence:** `.github/workflows/release.yml:25-29`  
**Problem:** goreleaser-action is SHA-pinned (good), but the version parameter it resolves IS a tag, not a SHA — version: v2.15.2 in both release.yml and release-prep.yml. This is the goreleaser CLI binary version, not the GH Action.  
**Fix:** Minor tightening: if goreleaser publishes binary SHA256s (it does, as part of its own release process), add a post-install `sha256sum goreleaser` step and match against a pinned hash. Alternatively, accept the v2.15.2 tag trust model — goreleaser signs its own releases with cosign.  
**Effort:** hours


##### `dep:33ef32bf:copyleft-audit-clean` — copyleft audit clean

**Status:** done — acceptance note (CLAUDE.md §Dependencies)  
**Severity:** suggestion  
**Evidence:** `go.mod:1-72`  
**Problem:** License compatibility audit: NO copyleft (GPL/AGPL/LGPL) or custom/unclear licenses in the transitive dep tree. All direct and indirect deps carry permissive licenses (MIT / Apache-2.0 / BSD-3).  
**Fix:** CLAUDE.md §Dependencies already codifies the MIT/Apache/BSD-only policy as of 2026-04-19. This row stays as a tripwire reference so future PR reviewers see the baseline; no code change needed.  
**Effort:** hours


##### `dep:33ef32bf:go-yaml-in-fork-risk` — go yaml in fork risk

**Status:** done — acceptance note  
**Severity:** suggestion  
**Evidence:** `go.mod:58-59`  
**Problem:** go.yaml.in/yaml/v2 and go.yaml.in/yaml/v3 are a vanity-domain fork of the original gopkg.in/yaml.v{2,3}. The domain (go.yaml.in) is a 2024+ rehosting that the k8s/cobra ecosystems migrated to after gopkg.in archived yaml.v2.  
**Fix:** Acceptance note only. The go.yaml.in move is the same maintainer collective as gopkg.in (kubernetes-sigs).  
**Effort:** hours


##### `dep:33ef32bf:golang-x-exp-stale` — golang x exp stale

**Status:** done — acceptance note (transitive upstream)  
**Severity:** suggestion  
**Evidence:** `go.mod:60-60`  
**Problem:** golang.org/x/exp pinned at v0.0.0-20231006140011 (Oct 2023) — almost 2.5 years old. Pulled transitively by charm.land/log/v2, which only imports golang.org/x/exp/slog (a BACKPORT of log/slog that the stdlib now provides since Go 1.21 — and this repo targets 1.25 per go.mod).  
**Fix:** File upstream issue at github.com/charmbracelet/log requesting a drop of the x/exp/slog import in favor of stdlib log/slog. Until that lands, the stale pin persists.  
**Effort:** hours



##### `doc:a55b4592:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/config/loader.go:15-15`  
**Problem:** NewLoader (line 15) lost its doc. Zero-arg constructor returning a pointer — the trivial signature masks the fact that the Loader has lifecycle state (caching, YAML defaults).  
**Fix:** Restore: '// NewLoader returns a Loader suitable for reading okdctl YAML configs. Loaders cache parsed schemas; reuse one per process to avoid re-parsing defaults.'  
**Effort:** hours


##### `doc:cf43073b:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/config/types.go:7-23`  
**Problem:** 4 exported symbols missing docs: DistributionOKD const (7), ProviderProxmox const (14), SupportedDistributions func (17), SupportedProviders func (23). The const values encode the supported-distributions/providers whitelist — semantic meaning is implicit.  
**Fix:** Restore group docs on each const block and one-line docs on SupportedDistributions / SupportedProviders. Example: '// Distributions okdctl can deploy.' before the DistributionOKD block covers revive's exported rule for the whole group.  
**Effort:** hours


##### `doc:297adb3e:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/config/validation_types.go:41-132`  
**Problem:** 5 exported validation types/methods missing docs: ValidationResult.IsValid (41), ValidationResult.AddError (45), ScopeRequired const (66), ValidationScope.HasScope (81), ValidateWithOptions (132). ScopeRequired is a bitflag const — semantic meaning is NOT evident from the name.  
**Fix:** Restore one-line contract docs. For bitflag consts, use a group header: '// Validation scope flags.' above the const block covers the whole group per revive's exported rule.  
**Effort:** hours


##### `doc:aa0f50f5:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/config/validators.go:369-481`  
**Problem:** 6 exported validators at lines 369 (IsValidIP), 374 (IsValidCIDR), 421 (ValidateClusterName), 431 (ValidateDomain), 457 (ValidateIP), 481 (ValidateCIDR) lack doc comments. Is- prefixed returns bool, Validate- prefixed returns error — the naming encodes the behavior but CLAUDE.md §code-comments item 2 still requires a contract doc on exported helpers to clarify failure modes.  
**Fix:** Add one-line verb-first docs. Example: '// IsValidIP reports whether s parses as an IPv4 or IPv6 literal.' '// ValidateClusterName returns a descriptive error if value violates the DNS-1123 cluster-name grammar.'  
**Effort:** hours


##### `doc:125729c4:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/destroy/phase.go:56-56`  
**Problem:** The New constructor (line 56) on destroy.Phase lost its doc in the hygiene pass. revive:exported will fail CI.  
**Fix:** Restore: '// New constructs a destroy.Phase bound to cfg. The Phase is safe to call multiple times — each step idempotently skips if its resource is absent.'  
**Effort:** hours


##### `doc:d5915b0c:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/install/phase.go:80-84`  
**Problem:** 2 exported symbols missing docs: Phase type (80), New func (84). Phase is part of the canonical per-phase pattern per CLAUDE.md §architecture-notes.  
**Fix:** Restore one-line docs mirroring the surviving cleanup/destroy phase patterns. Example: '// Phase drives the install flow: openshift-install wrapper, bootstrap monitor, cluster-up poll.'  
**Effort:** hours


##### `doc:0139cb3f:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/phase/paths.go:70-132`  
**Problem:** 2 exported symbols missing docs: BinDirOrDefault func (70), BasePhaseOption type (132). BasePhase helpers are canonical per CLAUDE.md §architecture-notes — these are the shared cross-phase APIs.  
**Fix:** Restore docs. Example: '// BinDirOrDefault returns s when non-empty, else the default bin dir (from ResolveBinDir).' '// BasePhaseOption configures a BasePhase at construction time.'  
**Effort:** hours


##### `doc:f99eddfa:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/postinstall/phase.go:26-60`  
**Problem:** 5 exported postinstall symbols missing docs: Options type (26), NewOptions func (35), Result type (48), Phase type (56), New func (60). Phase is the canonical per-phase type pattern — CLAUDE.md §architecture-notes explicitly names this.  
**Fix:** Restore one-line docs. Mirror the surviving setup/phase.go or install/phase.go pattern.  
**Effort:** hours


##### `doc:fb54208a:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/postinstall/steps.go:15-15`  
**Problem:** StepVerifyHealth const (line 15) lost its group-header doc. The const is a StepID — part of the canonical distribution.StepID enum used across the phase-step orchestration.  
**Fix:** Restore a group-level doc on the const block: '// Postinstall StepIDs. These identify the steps in Phase.Run order and appear in distribution.Orchestrator events.'  
**Effort:** hours


##### `doc:632c9087:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:18-39`  
**Problem:** 3 exported symbols missing docs: DefaultIngressLBTimeout const (18), IngressEntry type (31), UpdateIngressResult type (39). DefaultIngressLBTimeout encodes a 10-minute operational value — semantic meaning (why 10 not 5) is not evident from the name.  
**Fix:** Restore docs. For DefaultIngressLBTimeout include the rationale inline: '// DefaultIngressLBTimeout caps how long update-ingress waits for the ingress LB service to report a ready external IP.  
**Effort:** hours


##### `doc:ab9b764a:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/ignition.go:85-85`  
**Problem:** Phase.GenerateManifests (line 85) lost its doc. Manifest generation is an externally-visible step — callers need to know the failure mode.  
**Fix:** Restore: '// GenerateManifests invokes openshift-install to expand install-config.yaml into the full manifest set. Returns wrapped ConfigError for validation failures and wrapped ExecError for binary failures.'  
**Effort:** hours


##### `doc:2f70d7df:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/kargs.go:41-41`  
**Problem:** ExtractNetworkConfig (line 41) lost its doc. The name suggests extraction but the semantics (from which input?  
**Fix:** Restore: '// ExtractNetworkConfig parses the Ignition JSON and returns the first storage-files entry matching the NetworkManager connection path. Returns a typed ConfigError for malformed JSON.'  
**Effort:** hours


##### `doc:beabab0c:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/phase.go:19-111`  
**Problem:** 8 exported symbols in setup.Phase carry no doc comment after the 2026-04-21 comment-hygiene pass: DefaultIgnitionPort const (19), Options type (23), DefaultOptions func (34), CoreOSInfo type (57), NodeInfo type (64), Phase type (71), Phase.Execute method (94), Phase.PrintSetupCompletionSummary method (111). revive:exported enabled in .golangci.yml will fail CI on next push.  
**Fix:** Restore concise verb-first doc comments on each of the 8 sites. For the Phase type, lead with 'Phase drives the setup phase of an OKD install — artifact download, config generation, ignition upload.' Mirror existing docs in sibling install/destroy packages for consistency.  
**Effort:** hours


##### `doc:6fc3d91e:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/platform/platform.go:18-18`  
**Problem:** FamilyRHEL const (line 18) lost its group-header doc. Part of the const block defining platform family identifiers.  
**Fix:** Restore a group-level comment on the const block: '// Platform OS-family identifiers and supported arch literals.' covers revive's exported-block requirement.  
**Effort:** hours


##### `doc:e3782ee7:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/system/fs.go:14-235`  
**Problem:** 4 exported filesystem helpers missing docs: FileExists (14), DirExists (22), EnsureDirForFile (35), AtomicWriteString (235). AtomicWriteString is a wrapper around the canonical AtomicWrite — the wrapper's TOCTOU/fsync semantics should inherit from the underlying contract.  
**Fix:** Restore docs. Example: '// FileExists reports whether path refers to an existing regular file (returns false for directories).' '// AtomicWriteString is a string-typed convenience wrapper around AtomicWrite; the fsync + rename invariants are the same.'  
**Effort:** hours


##### `doc:e2343d2c:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/system/systemd.go:17-17`  
**Problem:** ServiceEnable const (line 17) lost its group-header doc. Part of the ServiceAction enum driving systemctl operations.  
**Fix:** Restore a group-level comment: '// Actions passable to SystemdCtl. Each value maps to a systemctl subcommand.'  
**Effort:** hours


##### `doc:c14fdd9d:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/tui/base_styles.go:9-9`  
**Problem:** TitleStyle var (line 9) lost its doc. Part of the base-styles palette; caller code in wizard steps imports it.  
**Fix:** Restore: '// TitleStyle is the bold Blue400 header style used at the top of each TUI step.'  
**Effort:** hours


##### `doc:588ce79e:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/tui/colors.go:13-18`  
**Problem:** 2 exported color symbols missing docs: ThemeDefault const (13), ColorPurple600 var (18). These are the theme system's public palette — downstream TUI components import them.  
**Fix:** Add group header on ThemeDefault's const block: '// Built-in color themes.' Add: '// ColorPurple600 is the purple-600 palette color used by the default theme.'  
**Effort:** hours


##### `doc:983f67f0:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/tui/layouts.go:11-11`  
**Problem:** DefaultBoxWidth const (line 11) lost its doc. A layout-constant encoding a semantic choice (78 columns?  
**Fix:** Restore: '// DefaultBoxWidth is the content width used by BoxedSection; 78 leaves room for the 1-col lipgloss border on each side in an 80-col terminal.'  
**Effort:** hours


##### `doc:660d83a5:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/tui/logger.go:22-161`  
**Problem:** 5 exported logger helpers missing docs: LF (line 22), Info (68), Warn (70), Error (72), RunID (161). Info/Warn/Error in particular encode contract invariants (stderr redirect + RedactHandler wiring per the kept comments elsewhere in this file).  
**Fix:** Restore one-line docs that reference the stderr + RedactHandler invariant already kept on stderrSlog. Example: '// Info logs at INFO through the redact-handling stderr slog.  
**Effort:** hours


##### `doc:bc9ba9bc:exported-doc-missing` — exported doc missing

**Status:** done — covered by F1 (commit 73912b5)  
**Severity:** major  
**Evidence:** `internal/tui/rendering.go:3-11`  
**Problem:** 3 exported rendering helpers missing docs: SubsectionLabel (3), CompletionSuccess (7), CompletionError (11). User-facing TUI output helpers — caller code needs to know formatting guarantees.  
**Fix:** Restore one-line docs. Example: '// CompletionSuccess formats msg with the green-check success prefix and the configured base style.'  
**Effort:** hours



##### `err:48688e63:typed-err-fallthrough` — typed err fallthrough

**Status:** done — PR #105  
**Severity:** minor  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:181-237`  
**Problem:** Provider.Provision and Provider.retrieveProvisionResult still raise bare fmt.Errorf for config-class / cluster-runtime failures ('no VMs provisioned; check config', 'static IP start address is required for OKD deployments'). The prior sweep fixed Connect (line 85, 88 now use ConfigError); these two adjacent sites were missed.  
**Fix:** Line 182: wrap with &errtypes.ClusterError{Msg: "terraform apply succeeded but no VMs were provisioned; check config"} (cluster-runtime failure, exit 4). Line 237: wrap with &errtypes.ConfigError{Msg: "static IP start address is required for OKD deployments"} (config, exit 2).  
**Effort:** hours


##### `err:40d315ad:wrap-tool-prereq-untyped` — wrap tool prereq untyped

**Status:** done — PR #105  
**Severity:** suggestion  
**Evidence:** `internal/addon/catalog/flux/flux.go:72-72`  
**Problem:** Flux.Install returns a bare fmt.Errorf('helm is required to install Flux') when helm is missing. The message is user-friendly but the error carries no chain or type — it's a tool-prerequisite failure that semantically matches ConfigError (missing external dep is a configuration/environment issue, exit 2).  
**Fix:** For tool-prerequisite errors (line 72) and config errors (:148, :332, :391), wrap with &errtypes.ConfigError{Msg: ...} so the underlying chain carries correct classification. addon/manager.go:installAndVerify should then errors.As for ConfigError before wrapping as ClusterError, preserving the 'tool missing' vs 'install failed' distinction at the outer boundary.  
**Effort:** hours


##### `err:ddf885f4:errors-join-ctx-lost` — errors join ctx lost

**Status:** done — PR #105  
**Severity:** suggestion  
**Evidence:** `internal/addon/manager.go:83-111`  
**Problem:** InstallAll aggregates failures via errors.Join(errs...) after wrapping each with ClusterError at installAndVerify:120. Good pattern.  
**Fix:** After the for-loop, add `if err := ctx.Err(); err != nil && len(errs) > 0 { errs = append(errs, err) }` so the joined error includes the ctx sentinel when cancellation contributed. Alternatively, make installAndVerify itself wrap ctx.Err via %w when Install/Verify return a ctx-related error.  
**Effort:** hours


##### `err:aa84670c:ctx-err-check-on-ctx` — ctx err check on ctx

**Status:** done — PR #105  
**Severity:** suggestion  
**Evidence:** `internal/cli/root.go:110-116`  
**Problem:** execute() still checks `if ctx.Err() != nil` to decide whether to return 130 (SIGINT) or 143 (SIGTERM). This works today because the hand-rolled signal handler always cancels the ctx before ExecuteContext returns, but it's fragile: a future subcommand that returns context.Canceled WITHOUT the parent ctx being canceled hits exitCodeFor instead of the 130/143 branch.  
**Fix:** Change to `if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) { /* ... SIGTERM/SIGINT branch ...  
**Effort:** hours


##### `err:7b2829bb:typed-no-error-iface` — typed no error iface

**Status:** done — PR #105  
**Severity:** suggestion  
**Evidence:** `internal/executor/executor.go:184-199`  
**Problem:** executor.ExitError doc still claims 'errors.Is to compare against Unwrap chain values' but the type has no Unwrap() method and no Err field. The claim is aspirational — there is nothing in the chain to traverse.  
**Fix:** Option A (recommended, -1 LOC): remove 'errors.Is to compare against Unwrap chain values' from the doc comment — the type doesn't currently chain. Option B (+5 LOC): add `Err error` field, populate from executor.run's `err` var when cmd.Run fails, implement Unwrap().  
**Effort:** hours


##### `err:f51f85bb:err-stringified-loses-type` — err stringified loses type

**Status:** done — PR #105  
**Severity:** suggestion  
**Evidence:** `internal/netutil/ip.go:43-46`  
**Problem:** Four sites still use `if err != nil || !X.Is4()` and return a synthetic fmt.Errorf that drops the netip.ParseAddr / netip.ParsePrefix error entirely. Debugging 'invalid IPv4 address: 192.168' gives no hint whether netip rejected the format, the IP-version check rejected it, or a whitespace issue.  
**Fix:** Split the conditional so parse-err and is4-check produce different messages, or wrap on the err-present case: `if err != nil { return fmt.Errorf("invalid IPv4 address %q: %w", startIP, err) } if !addr.Is4() { return fmt.Errorf("IPv6 not supported: %q", startIP) }`. Matches the 'invalid CIDR %q: %w' pattern at internal/netutil/ip.go:18.  
**Effort:** hours



##### `iac:b803fcb7:tflint-no-config` — tflint no config

**Status:** done — PR #114  
**Severity:** suggestion  
**Evidence:** `.github/workflows/ci.yml:102-109`  
**Problem:** CI runs `tflint --init && tflint -f compact` with no `.tflint.hcl` config file in either module or environment directory. Without a config, tflint loads only the default language ruleset — the `terraform-linters/tflint-ruleset-terraform` plugin (recommended preset: module_pinned_source, required_providers, required_version, naming conventions, unused_declarations) is therefore silent.  
**Fix:** Add `infrastructure/terraform/.tflint.hcl` with `plugin "terraform" { enabled = true, preset = "recommended" }`. Point CI to it via `--config=$GITHUB_WORKSPACE/infrastructure/terraform/.tflint.hcl` (shared across module + env) or per-directory symlinks.  
**Effort:** hours


##### `iac:b803fcb7b:tflint-recommended-findings` — tflint recommended findings block CI

**Status:** done — 2026-04-22 (commit f6abdb2) — wired numa through to VM cpu blocks, deleted deprecated `data_disk_size_gb`  
**Severity:** minor  
**Evidence:** `.github/workflows/ci.yml:102-109`, `infrastructure/terraform/modules/proxmox-okd/`, `infrastructure/terraform/environments/production/`  
**Problem:** Once PR #114 pointed tflint at the `recommended` preset, the `lint-terraform` CI job started failing with real findings on the existing HCL tree (exit code 2). The findings were not enumerated during this session because tflint isn't available locally; CI's log has the specifics.  
**Fix:** Run `tflint --init && tflint --config=infrastructure/terraform/.tflint.hcl` locally against both module and environment directories, fix each reported issue (module_pinned_source, required_providers, required_version, naming, unused_declarations are the usual suspects). Alternatively, narrow the preset from `recommended` to a curated rule list if some findings are intentional (e.g., module paths pinned via branch rather than tag).  
**Effort:** hours


##### `iac:e076e43c:sh-posix-not-bash` — sh posix not bash

**Status:** done — PR #114  
**Severity:** suggestion  
**Evidence:** `scripts/install.sh:1-1`  
**Problem:** Shebang `#!/bin/sh` constrains the script to POSIX sh (dash on Debian/Ubuntu, ash on Alpine), which prevents unconditional `set -o pipefail`, `[[ ]]`, and other bash conveniences. Script now mitigates with a conditional `(set -o pipefail 2>/dev/null) && set -o pipefail` probe, but future contributors may still introduce bashisms that break silently under dash/ash.  
**Fix:** Either (a) switch shebang to `#!/usr/bin/env bash` and drop the pipefail probe — bash is available on every supported install target (Debian/Ubuntu/Fedora/RHEL), Alpine is not a target platform per `uname -s` gate; or (b) keep `/bin/sh` and document in a one-line comment above the shebang that POSIX-only constructs are required, so future contributors don't accidentally introduce bashisms. The current hybrid works but sits on a portability knife-edge.  
**Effort:** hours



##### `mod:d31d1b9d:use-map-index` — use map index

**Status:** done — PR #105  
**Severity:** minor  
**Evidence:** `internal/cli/status.go:97-107`  
**Problem:** `statusNode.role()` iterates every key of `Labels` to check for two specific well-known strings. This is a map lookup dressed as a scan — O(n) in label count when a direct `if _, ok := Labels["node-role.kubernetes.io/master"]; ok` is O(1) and reads straight.  
**Fix:** Replace with direct map index: `if _, ok := n.Metadata.Labels["node-role.kubernetes.io/master"]; ok { return "master" }; if _, ok := n.Metadata.Labels["node-role.kubernetes.io/worker"]; ok { return "worker" }; return "unknown"`. No imports needed.  
**Effort:** hours


##### `mod:6fc3d91e:use-strings-lines` — use strings lines

**Status:** done — PR #105  
**Severity:** suggestion  
**Evidence:** `internal/cli/status.go:171-171`  
**Problem:** `for _, line := range strings.Split(strings.TrimSpace(coRaw), "\n")` materializes the split slice only to walk it. Go 1.24's `strings.Lines` iterator skips the allocation.  
**Fix:** Replace `for _, line := range strings.Split(s, "\n")` with `for line := range strings.Lines(s)`. Go 1.24 stdlib, no import change.  
**Effort:** hours


##### `mod:9d79b841:use-slices-max` — use slices max

**Status:** done — PR #107  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/setup/coreos.go:70-74`  
**Problem:** One of two near-identical blocks in `findOrDownloadFCOSISO` still does `slices.Sort(matches); matches[len(matches)-1]` to fetch the lexicographically-largest filename. The sibling block at lines 57-61 was already rewritten to `slices.Max(matches)`; this one (lines 70-74) was left behind.  
**Fix:** Replace `slices.Sort(matches); isoPath := matches[len(matches)-1]` with `isoPath := slices.Max(matches)`. Go 1.21+.  
**Effort:** hours


##### `mod:0934cf1b:use-slices-concat` — use slices concat

**Status:** done — PR #107  
**Severity:** suggestion  
**Evidence:** `internal/platform/packages.go:101-101`  
**Problem:** `append(append([]string{}, m.queryArgs...), pkg)` nests two `append`s to clone-then-extend a slice. Go 1.22's `slices.Concat` expresses the same intent in one call and also matches the repo's existing `slices.Concat(a, b)` style (see internal/cli/helpers.go:213, internal/distribution/okd/dns/dnsmasq.go:155).  
**Fix:** Replace with `args := slices.Concat(m.queryArgs, []string{pkg})`. Requires `slices` import (not currently imported in this file — the sibling InstallPackages uses a plain `append([]string{...}, installed...)` which is already idiomatic).  
**Effort:** hours


##### `mod:983f67f0:use-builtin-max-innerwidth` — use builtin max innerwidth

**Status:** done — PR #107  
**Severity:** suggestion  
**Evidence:** `internal/tui/layouts.go:54-60`  
**Problem:** Two sequential `if X > innerWidth { innerWidth = X }` blocks compute a running max over two candidates. Go 1.21's `max` builtin collapses both into `innerWidth = max(innerWidth-ContentPadding+maxContentWidth+ContentPadding, minWidthForTitle)` — or more readably, `innerWidth = max(innerWidth, maxContentWidth+ContentPadding, minWidthForTitle)`.  
**Fix:** Replace with `innerWidth := max(width-ContentPadding, maxContentWidth+ContentPadding, minWidthForTitle)`. `max` takes variadic ordered args in Go 1.21+.  
**Effort:** hours


##### `mod:983f67f0:use-builtin-max-padding` — use builtin max padding

**Status:** done — PR #107  
**Severity:** suggestion  
**Evidence:** `internal/tui/layouts.go:100-104`  
**Problem:** `padding := innerWidth - lineWidth; if padding < 0 { padding = 0 }` is a hand-rolled `max(padding, 0)` — the exact floor `max` was added (Go 1.21) to express. This pattern has been flagged and fixed in at least three sibling files already; layouts.go is the last holdout in the tui package.  
**Fix:** Replace with `padding := max(innerWidth-lineWidth, 0)`. Drops 3 lines to 0.  
**Effort:** hours



##### `obs:19a715fd:level-warn-help-text` — level warn help text

**Status:** done — PR #106  
**Severity:** minor  
**Evidence:** `internal/addon/catalog/secretstore/secretstore.go:122-152`  
**Problem:** secretstore.installPrereqCheck still logs multi-line HOW-TO guides (onepassword: 6 Warn lines, vault: 3 Warn lines, bitwarden: 3 Warn lines) via env.Logger.Warn when credential files are missing. Warn is for recoverable degradation; this is user education.  
**Fix:** Emit the first line ('no secret files found ... skipping') at Warn as the actual advisory.  
**Effort:** hours


##### `obs:15ba17da:err-stringified-into-label` — err stringified into label

**Status:** done — PR #106  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/destroy/steps.go:32-37`  
**Problem:** destroy.steps.go builds its per-step OnError callback as `phase.WarnOnError(p.Log, label+": "+err.Error())(err)`, which concatenates err.Error() into the Warn message AND passes err again as the structured attr (WarnOnError body: `logger.Warn(msg, "err", err)`). Result: the error text appears twice — once inlined into `msg` (bypassing RedactHandler's attr-walk) and once as the structured `err` attr.  
**Fix:** Drop the `+ ": " + err.Error()` concatenation: `phase.WarnOnError(p.Log, label)(err)`. WarnOnError already emits `logger.Warn(label, "err", err)`, which gives structured consumers the label and a redaction-eligible err attr.  
**Effort:** hours


##### `obs:00000002:inconsistent-domain-prefix-keys` — inconsistent domain prefix keys

**Status:** done — PR #106  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/postinstall/update_ingress.go:140-285`  
**Problem:** The codebase still leans on the `prefix: message` convention ('update-ingress:', 'haproxy:', 'kubevip:', 'cluster:', 'cleanup:', 'terraform:', 'coreos:', 'iso:', 'csr:', 'addons:') in message bodies, but no call-site pins a structured `component` or `phase` attr via logger.With(). Only `run_id` is propagated (tui.SetRunID).  
**Fix:** At each phase constructor (setup/install/postinstall/destroy/cleanup) wrap p.Log via logger.With("phase", "install"); at sub-component boundaries (haproxy, dns, kubevip, terraform, iso, packages, cleanup.services, cleanup.packages, addon.manager) narrow with logger.With("component", "haproxy"). Retain the human prefix in the message for TTY readability — the attr is additive.  
**Effort:** hours


##### `obs:9d79b841:duplicate-iso-exists-log` — duplicate iso exists log

**Status:** done — PR #106  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/setup/coreos.go:59-265`  
**Problem:** coreos.go logs `coreos: found existing iso at X` (L59, L73) and `coreos: iso already exists at X` (L201, L265) in four distinct sites. A single setup run can fire more than one because the lookups happen across different layers (local iso dir, work-dir cache, download destination, upload destination), producing near-identical Info lines.  
**Fix:** Consolidate the four sites into one helper (`logISOFound(path)`) that emits the Info line once per iso-path in a run — keep a set of already-logged paths keyed off filepath.Base. If the two message variants (found vs already-exists) carry distinct operator semantics, rename them so the distinction is legible.  
**Effort:** hours


##### `obs:366b3f2d:span-no-start-end-per-step` — span no start end per step

**Status:** done — PR #106  
**Severity:** minor  
**Evidence:** `internal/distribution/orchestrator.go:113-154`  
**Problem:** Orchestrator.executeStep still does not emit a structured start/finish log pair per step. Skipping is logged (L90) but success/duration is not — Duration is captured in StepResult but never reaches the logger.  
**Fix:** In executeStep, before step.OnStart() log `o.logger.Info("step started", "step", step.ID(), "name", step.Name())`. After Execute (both success and error branches) log `o.logger.Info("step completed", "step", step.ID(), "duration", time.Since(startedAt), "success", err == nil)`.  
**Effort:** hours


##### `obs:7b2829bb:executor-no-output-span` — executor no output span

**Status:** done — PR #106  
**Severity:** minor  
**Evidence:** `internal/executor/executor.go:213-273`  
**Problem:** executor.run and RunInteractive still only log `+ <name> <args>` at Debug when Verbose is true — nothing bookends the call in the structured stream. For a 15-minute terraform apply or oc poll the JSON sink sees nothing until completion.  
**Fix:** At start of executor.run log `e.logger.Debug("exec started", "cmd", name, "argc", len(args))` (omit argv itself — terraform invocation argv can contain a credential substitution in rare configs). After cmd.Run log `e.logger.Debug("exec completed", "cmd", name, "exit", result.ExitCode, "duration", result.Duration)`.  
**Effort:** hours


##### `obs:48688e63:message-embedded-counts` — message embedded counts

**Status:** done — PR #106  
**Severity:** minor  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:217-217`  
**Problem:** The prior audit flagged three terraform count-in-message lines in proxmox.go; L158 and L185 are now structured (`"count", n`) but L217 remains `fmt.Sprintf("terraform: plan will preview %d virtual machines", totalNodes)`. The pattern also spreads to cleanup/summary.go L73 / L80 / L94 and addon/manager.go L78 / L101 / L117 / L125 / L181 — numeric counts wedged into the message string that JSON consumers cannot index by `count`.  
**Fix:** Replace the remaining `fmt.Sprintf(...%d...)` log sites with structured attrs: proxmox.go:L217 → `p.logger.Info("terraform: plan preview", "vm_count", totalNodes)`; cleanup/summary.go:L73/L80/L94 → match L67's shape `logger.Info("cleanup: ignition files", "count", n)`; addon/manager.go:L78/L101/L117/L125/L181 → `"addons: installing", "count", len(ordered)` etc. Keep human prefix in the message for TTY readability.  
**Effort:** hours


##### `obs:aa84670c:root-error-stringified` — root error stringified

**Status:** done — PR #106  
**Severity:** suggestion  
**Evidence:** `internal/cli/root.go:187-187`  
**Problem:** The ctx-done-miss branch at L120 was migrated to structured form `tui.Error("command failed", tui.LF("err", err))` — prior audit's core case. The SetFlagErrorFunc handler at L187 still stringifies: `tui.Error(err.Error())`.  
**Fix:** Replace `tui.Error(err.Error())` with `tui.Error("flag error", tui.LF("err", err))`. Keep the exit-code logic unchanged.  
**Effort:** hours


##### `obs:ae5b624c:monitor-retry-log-per-tick` — monitor retry log per tick

**Status:** done — PR #106  
**Severity:** suggestion  
**Evidence:** `internal/distribution/okd/install/monitor.go:119-127`  
**Problem:** MonitorInstallation's CSR approval tick runs every 30s for up to 60 minutes. On each tick: on error it Warns structured, on approved>0 it Infos structured.  
**Fix:** Optional: de-dup identical consecutive Warns via a `lastWarnErrMsg` tracker, downgrading repeats to Debug after the first. Keep `approved>0` Info as-is (state transition).  
**Effort:** hours



##### `sec:06f00bcb:ignition-dir-perms` — ignition dir perms

**Status:** done — PR #111  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/apache.go:28-45`  
**Problem:** ensureIgnitionDir creates /var/www/html/ignition at 0o755 and then explicitly re-chmods to 0o755 if pre-existing. The ignition files inside (bootstrap.ign, master.ign, worker.ign) carry the pullSecret.  
**Fix:** Tighten the ignition directory and file perms: dir → 0o750 owner apache:apache; files → 0o640 via CopyFileMode with 0o640. Apache serves them fine under its own uid; local non-apache users can no longer grep out the pullSecret.  
**Effort:** hours


##### `sec:19a715fd:secretstore-plaintext-disk` — secretstore plaintext disk

**Status:** done — PR #111  
**Severity:** minor  
**Evidence:** `internal/addon/catalog/secretstore/secretstore.go:253-278`  
**Problem:** The secretstore addon reads 1password-credentials.json and 1password-token.txt (plus the vault/bitwarden equivalents) from automation/config/secrets/ and applies them as Kubernetes secrets. The code neither checks nor enforces restrictive file permissions on these on-disk credential files: a user who followed the setup instructions with `echo -n 'TOKEN' > file` gets default umask (often 0o644).  
**Fix:** Before os.ReadFile(path), Stat the path and reject any file whose perm bits exceed 0o600 unless it's sops-encrypted. Mirror the pattern used in internal/credentials/envfile.go loadEnvFileOnce.  
**Effort:** hours


##### `sec:26a430ee:syscall-exec-env-leak` — syscall exec env leak

**Status:** done — PR #111  
**Severity:** minor  
**Evidence:** `internal/cli/elevation.go:54-77`  
**Problem:** ensureRoot re-execs via syscall.Exec(sudoPath, args, os.Environ()). The full inherited environment is handed to sudo → the new okdctl process.  
**Fix:** Filter os.Environ() before passing to syscall.Exec: keep PATH, HOME, USER, LANG, LC_*, SUDO_*, PROXMOX_VE_* (needed downstream), OKDCTL_*, KUBECONFIG, and reject everything else. The downstream Executor now applies a similar allowlist (internal/executor/executor.go:85-121), so this layer is additive defense-in-depth — but the sudo boundary is the highest-value place to enforce.  
**Effort:** hours


##### `sec:d66c3d7f:bashrc-no-nofollow` — bashrc no nofollow

**Status:** done — PR #111  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/install/flux.go:93-143`  
**Problem:** addKubeconfigToBashrc opens ~/.bashrc with os.OpenFile(O_APPEND|O_WRONLY, 0o644) under the sudo re-exec (running as root, HOME resolved via InvokingUserHomeDir). No lstat + O_NOFOLLOW guard.  
**Fix:** Mirror the logging.go fix: lstat the path first and refuse if it is a symlink; then open with syscall.O_NOFOLLOW so any concurrent symlink-plant races fail. Alternatively, use system.AtomicWrite to read-modify-rewrite instead of O_APPEND on a user-owned file — AtomicWrite already has the fsync/rename guarantees this site could benefit from.  
**Effort:** hours


##### `state:93957c53:cleanup-no-confirm-cluster` — cleanup no confirm cluster

**Status:** done — PR #109  
**Severity:** major  
**Evidence:** `internal/cli/cleanup.go:37-103`  
**Problem:** `okdctl cleanup` has only `--yes` with no typo-guard against the wrong config. Unlike `okdctl destroy` which requires `--confirm-cluster=<name>` when `--yes` is passed, cleanup stops and uninstalls haproxy/dnsmasq/apache, drops the VIP secondary IP, wipes terraform.tfstate.backup + .terraform.lock.hcl + bin-dir binaries (coreos-installer/terraform/oc/kubectl) without asserting the cluster name in scripted invocations.  
**Fix:** Mirror the destroy guards: add `--confirm-cluster` (required with `--yes`, must match cfg.Cluster.Name) and `--dry-run` (prints the list of services that would be stopped and files that would be removed without mutating). Promote the destroy.go guard block into a shared helper (e.g.  
**Effort:** hours


##### `state:48688e63:provision-leaves-tfplan` — provision leaves tfplan

**Status:** done — PR #109  
**Severity:** minor  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:149-174`  
**Problem:** `Provider.Provision` writes `<workDir>/tfplan` via Plan then applies it, but never sweeps the plan file on success or failure — only `destroyInfrastructure` calls `tf.Cleanup()`. After a successful deploy the operator is left with a stale `tfplan` that no longer matches state; after a failed apply it's doubly-stale.  
**Fix:** Add `defer func() { _ = p.terraformExec.Cleanup() }()` immediately after the Plan call succeeds in `Provider.Provision`, matching the bootstrap.go pattern. Plan file removal on failure also helps operators inspecting `<workDir>` after a failed apply because nothing stale is left.  
**Effort:** hours


##### `sub:e2343d2c:systemd-stderr-dropped` — systemd stderr dropped

**Status:** done — PR #112  
**Severity:** minor  
**Evidence:** `internal/system/systemd.go:36-43`  
**Problem:** ManageService runs systemctl enable/disable/start/stop/restart/reload via exec.CommandContext(...).Run() with both stdout and stderr left nil. On failure the caller gets a bare *exec.ExitError with no systemctl diagnostic ('Failed to enable unit: Unit file haproxy.service does not exist' / 'Job for X failed because the control process exited').  
**Fix:** Route the default (state-changing) branch through system.RunCaptured so the returned error carries systemctl's stderr diagnostic. The is-active/is-enabled probe branches can stay on bare .Run() — exit code alone is the signal and --quiet already suppresses stderr noise.  
**Effort:** hours



##### `tst:25fa1be8:no-test-validateport-attacker` — no test validateport attacker

**Status:** done — PR #112  
**Severity:** major  
**Evidence:** `internal/distribution/okd/firewall/firewall.go:124-140`  
**Problem:** validatePort is the explicit defense-in-depth guard preventing Port.Protocol from flowing unchecked into fmt.Sprintf("%d/%s", ...) and onward into firewall-cmd / ufw / iptables argv. The doc comment explicitly warns: "keeping the guard here prevents a future caller from sneaking an unvalidated protocol string into the rendered rule".  
**Fix:** Add internal/distribution/okd/firewall/firewall_test.go::TestValidatePort — table-driven: valid [{6443,tcp}, {53,udp}] → nil; invalid port number [0, -1, 65536, 99999] → "invalid port number"; invalid protocol ["", "TCP", "sctp", "tcp/ip", "tcp; rm", "icmp"] → "invalid protocol". Twenty lines.  
**Effort:** hours


##### `tst:ab9b764a:no-test-installconfig-perms` — no test installconfig perms

**Status:** done — PR #112  
**Severity:** major  
**Evidence:** `internal/distribution/okd/setup/ignition.go:34-83`  
**Problem:** GenerateInstallConfig reads the pull-secret and writes install-config.yaml (containing the raw pull-secret JSON) at mode 0o600 via AtomicWriteString, then duplicates it to install-config.yaml.backup via CopyFileMode at 0o600. Both perm values are critical — the pull-secret is a Red Hat registry credential.  
**Fix:** Add setup/ignition_test.go::TestGenerateInstallConfig_Perms — build a minimal cfg with cfg.Files.PullSecret + cfg.Files.SSHPublicKey pointing at tmp files; call GenerateInstallConfig with outputDir=t.TempDir; stat both install-config.yaml and install-config.yaml.backup; assert os.FileMode.Perm() == 0o600 for each. Also TestGenerateInstallConfig_PullSecretReadFail asserts *errtypes.AuthError on missing pull-secret.  
**Effort:** hours


##### `tst:98723e5d:no-test-add-kubeconfig-bashrc` — no test add kubeconfig bashrc

**Status:** done — PR #112  
**Severity:** minor  
**Evidence:** `internal/distribution/okd/install/flux.go:93-143`  
**Problem:** addKubeconfigToBashrc appends `export KUBECONFIG=<path>` to the invoking user's ~/.bashrc. It preserves the existing file mode explicitly (doc: "appending an export line can't silently relax stricter perms the user may have set") and is idempotent (skips if `export KUBECONFIG=` already present).  
**Fix:** Extend the flux_test.go with: (1) TestAddKubeconfigToBashrc_Idempotent — pre-populate bashrc with `export KUBECONFIG=/old`, call addKubeconfigToBashrc, assert file content is byte-identical (the idempotency short-circuit); (2) TestAddKubeconfigToBashrc_PreservesMode — create bashrc at 0o600, call, stat, assert perm still 0o600; (3) TestAddKubeconfigToBashrc_CreatesIfMissing — no prior bashrc, call, assert it exists at 0o644 with the export line.  
**Effort:** hours


##### `sec:35abd54e:input-url-scheme-not-checked` — input url scheme not checked

**Status:** done — PR #128 (moved to Completed)


##### `sec:98723e5d:bashrc-chown-leak` — bashrc chown leak

**Status:** done — PR #123 (moved to Completed)


##### `sub:97cb8adf:no-cmd-env` — no cmd env

**Status:** done — 2026-04-26 — PR #147 (moved to Completed)


##### `sub:ae5b624c:bypass-canonical-executor` — bypass canonical executor

**Status:** done — 2026-04-26 — PR #148 (moved to Completed)


##### `state:b804b2ec:bootstrap-destroy-skip-tfvars-silent` — bootstrap destroy skip tfvars silent

**Status:** done — 2026-04-26 — PR #149 (moved to Completed)


##### `state:4c092fce:tf-state-backup-removed-on-success` — tf state backup removed on success

**Status:** done — PR #154 (moved to Completed)  
**Severity:** major  
**Cluster:** tf-state-atomicity  
**Evidence:** `internal/infrastructure/terraform/terraform.go:314-328`  
**Problem:** Executor.Cleanup() unconditionally deletes terraform.tfstate.backup along with tfplan/destroy.tfplan after a successful destroy. The .backup file is the operator's only built-in rollback artefact if the live tfstate is later corrupted; sweeping it on success leaves the workdir in a state where a subsequent stale-state recovery has to be reconstructed from Proxmox by hand.  
**Fix:** Split Cleanup() into two methods: CleanupPlans() removes only tfplan + destroy.tfplan, CleanupBackup() removes terraform.tfstate.backup. Call CleanupPlans() at the existing site (destroy/helpers.go:46, proxmox/proxmox.go:147). Never call CleanupBackup() — let the operator decide. Or: keep .backup until the *next* successful run, mirroring git's reflog policy.  
**Effort:** hours



##### `err:a55b4592:vocab-ad-hoc-config-perm` — vocab ad hoc config perm

**Status:** done — PR #204 (moved to Completed)  
**Severity:** minor  
**Cluster:** domain-vocabulary  
**Evidence:** `internal/config/loader.go:22-47`  
**Problem:** Loader.LoadFile wraps insecure-perm and parse failures with bare fmt.Errorf("...%w", err) instead of typing them as errtypes.ConfigError. The exact same security check in internal/credentials/envfile.go:122-128 returns &errtypes.AuthError{Err: os.ErrPermission}. Two security-critical perm checks; two different error shapes; one mapped to exit 2 (or 5), one to exit 1.  
**Fix:** Wrap insecure-perm in &errtypes.AuthError{Msg, Err: os.ErrPermission} (matches envfile.go:124-127); wrap parse/read in &errtypes.ConfigError{Msg, Err: err}. Both preserve %w identity through Unwrap so callers can still errors.Is(err, os.ErrPermission).  
**Effort:** hours


##### `err:c287d5c0:vocab-ad-hoc-distribution-type` — vocab ad hoc distribution type

**Status:** done — PR #153 (moved to Completed)  
**Severity:** minor  
**Cluster:** domain-vocabulary  
**Evidence:** `internal/distribution/okd/okd.go:99-103`  
**Problem:** Provisioner.Validate returns bare fmt.Errorf("invalid distribution type: ...") for a config-shape error. cli/root.go's exitCodeFor maps this to 1; the documented "config error → 2" contract (errtypes.go package doc) is broken. validation.WrapValidation exists for this exact pattern (config/cluster.go), but Provisioner.Validate doesn't use it.  
**Fix:** return &errtypes.ConfigError{Msg: fmt.Sprintf("invalid distribution type: expected okd, got %s", cfg.Distribution.Type)}.  
**Effort:** hours


##### `err:5013fea6:str-sniff-tool-msg` — str sniff tool msg

**Status:** done — PR #205 (moved to Completed)  
**Severity:** minor  
**Cluster:** string-sniffing  
**Evidence:** `internal/distribution/okd/setup/release_extract.go:81-127`  
**Problem:** isAuthError() does strings.Contains(lower, marker) over a fixed list against `oc adm release extract` stderr to choose between AuthError (exit 5) and ClusterError (exit 4). The exact wording of these markers ("unauthorized", "401", "no basic auth") is upstream-tooling output; oc has changed wording across minors. The author flagged this as best-effort in the comment, but the user-visible exit-code branches on it.  
**Fix:** Two-step: (1) parse the executor.ExitError exit code — non-zero exit codes 1 and 125 are typical for auth failure on most container runtimes; (2) keep the string match as a secondary heuristic but downgrade unmatched-fail to ClusterError. The single-source-of-truth is the exit code; stderr-text is the fallback. Document the risk in a TODO with a roadmap link if the exit-code path needs upstream investigation.  
**Effort:** hours


##### `err:9d79b841:fcos-stream-status-bare` — fcos stream status bare

**Status:** done — PR #203 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** wrapping  
**Evidence:** `internal/distribution/okd/setup/coreos.go:150-172`  
**Problem:** fetchCoreOSStream returns bare fmt.Errorf("coreos stream: HTTP %d", resp.StatusCode) for non-200 responses. The download package has a typed httpStatusError (download/retry.go:17-32) for exactly this pattern, used by isRetryable to drive retry behavior. Coreos stream HTTP failures here are NOT retryable via the same path, and a future caller wanting to errors.As(err, &httpStatusError{}) on the coreos endpoint can't.  
**Fix:** Reuse internal/download.httpStatusError (or export it as download.HTTPStatusError) so the coreos stream fetch surfaces the same shape as the rest of the download retry layer. Bonus: makes isRetryable's logic shareable.  
**Effort:** hours



##### `con:bdf5a873:safe-remove-ignores-ctx` — safe remove ignores ctx

**Status:** done — PR #175 (moved to Completed)  
**Severity:** minor  
**Cluster:** ctx-ignored  
**Evidence:** `internal/distribution/okd/cleanup/artifacts.go:33-57`  
**Problem:** SafeRemoveWithLogger accepts ctx but discards it (`_ context.Context`). The body issues os.Stat / os.RemoveAll which can stall indefinitely on a slow / hung NFS or stuck mount during destroy — the long-running operation a destroy ctx is supposed to cancel. Either thread ctx through (Go 1.25 has no ctx-aware RemoveAll, so the canonical fix is to wrap the call in WaitFor + a ctx-cancel race) or drop the parameter so callers don't expect cancellation honoured.  
**Fix:** Either: (a) drop the ctx parameter (the four call sites in cleanup.go are sequential and don't supply cancellation expectations), or (b) before each os.RemoveAll, do `if err := ctx.Err(); err != nil { return err }`. (a) is honest about what the function does; (b) costs ~3 lines and gives a destroy ctx a meaningful cancel point.  
**Effort:** hours


##### `con:f5d703ab:install-tools-to-system-no-ctx` — install tools to system no ctx

**Status:** done — PR #182 (moved to Completed)


##### `con:ab9b764a:validate-ignition-only-checks-ctx-once` — validate ignition only checks ctx once

**Status:** done — PR #183 (moved to Completed)


##### `con:6424733c:metrics-shutdown-bg-ctx` — metrics shutdown bg ctx

**Status:** done — PR #202 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** ctx-todo  
**Evidence:** `internal/cli/helpers.go:140-159`  
**Problem:** startMetricsServer's stop closure builds `shutCtx` with `context.Background()` even though the caller (executeFullDeployment) has a ctx in hand. The ctx-from-caller would be cancelled by SIGINT first, which is exactly what we don't want here (we want the 5-second graceful drain to run after parent ctx cancel). So Background is *correct* — but there is no comment justifying it, which CLAUDE.md §concurrency requires ("`context.Background()` / `context.TODO()` in production code needs a justification comment").  
**Fix:** Add a one-line comment above `context.WithTimeout(context.Background(), ...)` explaining: "Use Background, not the caller's ctx, so the 5s graceful drain runs even after SIGINT cancelled the parent." Same wording as monitor.go's reapTimer comment.  
**Effort:** hours


##### `con:e7db1220:releases-completion-bg-ctx` — releases completion bg ctx

**Status:** done — PR #172 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** ctx-todo  
**Evidence:** `internal/cli/releases.go:58-71`  
**Problem:** The shell-completion ValidArgsFunction passes context.Background() to fetcher.FetchVersions. cobra exposes a Context-bearing variant via the (cmd, args, toComplete) signature — completion can get cmd.Context() which honours the user's Ctrl-C. Today, a hung GitHub call during tab-completion blocks the shell until the underlying http client's own timeout fires.  
**Fix:** Change the closure to `func(cmd *cobra.Command, _ []string, _ string)` and call `fetcher.FetchVersions(cmd.Context())`. Adds proper cancellation if the user Ctrl-C's during completion.  
**Effort:** hours


##### `con:aa84670c:time-after-update-notice-ok` — time after update notice ok

**Status:** done — PR #179  
**Severity:** suggestion  
**Cluster:** time-sleep-retry  
**Evidence:** `internal/cli/root.go:128-134`  
**Problem:** printUpdateNotice uses `case <-time.After(100 * time.Millisecond)` in a select. Bare time.After leaks the underlying Timer until it fires (per CLAUDE.md, the canonical alternative is time.NewTimer + Stop on the win path). Here the cap is 100ms and the function is called once at process exit, so the 'leak' has zero observable cost — but the pattern is the one the doc calls out.  
**Fix:** Replace with: `t := time.NewTimer(100 * time.Millisecond); defer t.Stop(); select { case result = <-ch: case <-t.C: return }`. Two extra lines for pattern consistency with internal/distribution/okd/install/monitor.go's reapTimer.  
**Effort:** hours


##### `api:dd75bdeb:export-no-caller` — export no caller

**Status:** done — PR #181  
**Severity:** minor  
**Cluster:** exported-surface  
**Evidence:** `internal/distribution/okd/postinstall/context.go:4-10`  
**Problem:** PostInstallContext is exported (with a //nolint:revive stutter suppression) but is only ever consumed inside the postinstall package as the type parameter of distribution.PhaseContext[PostInstallContext]. No external caller references the type by name. The nolint comment itself names it 'established internal API; rename deferred' — the established-internal status is the giveaway: if it's internal-only it should be lowercased and the stutter goes away naturally.  
**Fix:** Lowercase to `postInstallContext`; the type only flows through distribution.PhaseContext[T any] which is generic over any T. The nolint:revive comment can be deleted in the same patch. distribution.PhaseContext can hold an unexported type just fine — generics don't care.  
**Effort:** hours



##### `api:0934cf1b:should-be-exported` — should be exported

**Status:** done — PR #184 (moved to Completed)


##### `ux:fd2125dd:addon-uninstall-no-confirm` — addon uninstall no confirm

**Status:** done — PR #206 (moved to Completed)  
**Severity:** major  
**Cluster:** verb-noun  
**Evidence:** `internal/cli/addon.go:66-80`  
**Problem:** `okdctl addon uninstall <name>` is a destructive op (deletes manifests, namespaces, secrets) but has no `--yes` flag and no confirmation prompt. Sibling destructive verbs (destroy, cleanup, update-ingress) all gate on either a TTY prompt or a `--yes`+`--confirm-cluster` pair. addon uninstall is a hole.  
**Fix:** Add `--yes` boolean (default false) and `--confirm-cluster` (typo guard, same pattern as destroy.go L82-94). When non-TTY without --yes, refuse. When --yes, require --confirm-cluster matches cfg.Cluster.Name. Copy promptForConfirmation pattern.  
**Effort:** hours


##### `ux:6424733c:no-tty-prompt-returns-false-silently` — no tty prompt returns false silently

**Status:** done — PR #186 (moved to Completed)


##### `ux:0f076161:destroy-force-deprecated-but-still-default-binding` — destroy force deprecated but still default binding

**Status:** done — PRs #185 + #192 (moved to Completed)


##### `ux:08c49fc4:remove-haproxy-no-x-bool-default-true` — remove haproxy no x bool default true

**Status:** done — PRs #187 + #193 (moved to Completed)


##### `ux:8d8faa80:completion-powershell-on-linux-only-tool` — completion powershell on linux only tool

**Status:** done — PR #164 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** verb-noun  
**Evidence:** `internal/cli/completion.go:11-31`  
**Problem:** completionCmd advertises `powershell` as a valid arg, but okdctl is Linux-only (README L24-L26, MEMORY.md). The shell completion is generated correctly by cobra but operators on Windows literally can't run okdctl, so listing powershell at all is dishonest help text.  
**Fix:** Drop `powershell` from Use and ValidArgs. Update Long to 3 shells. Drop the powershell hint from README L70-72. (User memory says: skip Windows-compat suggestions; this is the inverse direction — *removing* a Windows-flavored thing on a Linux-only tool, which aligns with the memory note.)  
**Effort:** hours


##### `obs:6424733c:string-concat-err-error-in-tui` — string concat err error in tui

**Status:** done — PR #155 (moved to Completed)  
**Severity:** minor  
**Cluster:** field-stability  
**Evidence:** `cmd/okdctl/main.go:52-52`  
**Problem:** `tui.Warn("failed to prepend " + binDir + " to PATH: " + err.Error())` builds the full message via string concatenation, so the err's chain reaches the sink as a flat string. RedactHandler scrubs structured attr values, not the message body — any future error wrapping a cred would slip through this site.  
**Fix:** Replace with `tui.Warn("OKDCTL_BIN_DIR ignored", tui.LF("value", v), tui.LF("err", detail))` and `tui.Warn("failed to prepend bin dir to PATH", tui.LF("bin_dir", binDir), tui.LF("err", err))`. Two-line edit each.  
**Effort:** hours


##### `obs:33579dd5:err-stringified-bypasses-handler` — err stringified bypasses handler

**Status:** done — PR #176 (moved to Completed)  
**Severity:** minor  
**Cluster:** field-stability — seam→audit-errors; related: err:a4001485:errtype-msg-vs-error-asymmetry  
**Evidence:** `internal/distribution/okd/cleanup/services.go:147-147`  
**Problem:** Four cleanup sites pass `guardErr.Error()` as the slog message rather than as a structured `err` attr. The handler sees only a free-form string, loses error-chain identity, and can't apply the cred/userinfo/Redacted() sweep that fires on attr values.  
**Fix:** Rewrite as `logger.Warn("cleanup: refusing critical path", "err", guardErr)` (matches the existing idiom at services.go:139 and artifacts.go:36). One mechanical edit per site.  
**Effort:** hours


##### `obs:366b3f2d:step-completed-info-on-failure` — step completed info on failure

**Status:** done — PR #180 (follow-up PR #191)  
**Severity:** minor  
**Cluster:** level-discipline — seam→audit-cli-ux  
**Evidence:** `internal/distribution/orchestrator.go:142-142`  
**Problem:** `step: completed` is logged at Info level whether the step succeeded or failed. A failed fatal step is a user-visible failure — it should surface at Warn or Error so log-level filtering at the sink (e.g. `--log-level=warn`) reaches it.  
**Fix:** Branch on result: `if !result.Success { o.logger.Warn("step: failed", ...) } else { o.logger.Info("step: completed", ...) }`. If `step.IsFatal()`, escalate to Error. Keeps the structured shape; flips the level so log-filter consumers can grep.  
**Effort:** hours


##### `obs:48688e63:apply-failure-no-err-attr` — apply failure no err attr

**Status:** done — PR #165 (moved to Completed)  
**Severity:** minor  
**Cluster:** field-stability  
**Evidence:** `internal/infrastructure/proxmox/proxmox.go:164-164`  
**Problem:** On terraform apply failure, the warn log carries the user-facing recovery hint ("run 'okdctl destroy'") but does NOT carry the apply error itself as an attr. The error then surfaces only via `fmt.Errorf(... %w ...)` two lines later — the slog record has no `err` field, so log-aggregation can't filter by error class.  
**Fix:** `p.logger.Warn("terraform: apply failed; run 'okdctl destroy' to clean up partial infrastructure", "err", applyErr)`. Single-attr addition; recovery hint stays in the message so TTY users still see it.  
**Effort:** hours


##### `obs:660d83a5:run-id-mutation-race` — run id mutation race

**Status:** done — PR #188 (moved to Completed)


##### `obs:ed55ee90:summary-keys-leading-whitespace` — summary keys leading whitespace

**Status:** done — PR #189 (moved to Completed)


##### `obs:c287d5c0:cleanup-warning-key-vague` — cleanup warning key vague

**Status:** done — PR #166 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** field-stability  
**Evidence:** `internal/distribution/okd/okd.go:122-122`  
**Problem:** `logger.Warn("cleanup warning", "err", err)` uses an unkeyed message that doesn't identify which cleanup operation failed (the parent context is `Prepare → cleanup.Execute`). Compare to the rest of the cleanup package (`cleanup: failed to stop service`, `cleanup: failed to remove package`) — this one breaks the established `cleanup: <verb> <object>` key convention.  
**Fix:** `p.logger.Warn("cleanup: pre-deploy artifact removal incomplete", "phase", "prepare", "err", err)`. Adds a `phase` attr matching orchestrator's `step` key style.  
**Effort:** hours


##### `obs:97cb8adf:waitfor-no-retry-count` — waitfor no retry count

**Status:** done — PR #173 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** span-retry-boundary  
**Evidence:** `internal/system/exec.go:54-108`  
**Problem:** `WaitFor` logs `prefix: waiting` with `for` and `elapsed` on every poll iteration at Debug, but the start log ("waiting for X") and the ready log ("X is ready") form the span; the iteration count never appears in either bookend. Operators tailing logs can't tell from the structured record how many polls fired before ready.  
**Fix:** Track iteration count in the loop and append `"polls", i, "elapsed", elapsed` to the readyMsg at L81/L100 and to the timeout error at L96. Three structured attrs on existing log calls; no new sites.  
**Effort:** hours


##### `mod:f55b9c27:use-builtin-clear` — use builtin clear

**Status:** done — PR #159 (moved to Completed)  
**Severity:** minor  
**Cluster:** any-interface-builtins  
**Evidence:** `internal/credentials/envfile.go:81-83`  
**Problem:** `WriteEnvFile` hand-rolls a `for i := range data { data[i] = 0 }` to zero the buffer's backing store after the credential bytes have been flushed to disk. Go 1.21's `clear` builtin expresses the same operation in one call and reads as "wipe", which matters on a credential-handling path where the loop's intent is load-bearing.  
**Fix:** Replace the three-line loop with `clear(data)`. Identical semantics on `[]byte`; preserves the `// Zero the buffer's backing store` comment as the WHY explanation. Keep the comment — `clear(data)` reads as the operation, the comment still adds the security context.  
**Effort:** hours


##### `mod:35abd54e:use-builtin-clear` — use builtin clear

**Status:** done — PR #160 (moved to Completed)  
**Severity:** minor  
**Cluster:** any-interface-builtins  
**Evidence:** `internal/credentials/proxmox.go:82-89`  
**Problem:** `Zeroize` hand-rolls two `for i := range slice { slice[i] = 0 }` zero-fill loops where the Go 1.21 `clear` builtin does exactly this — and signals intent at the call site ("this is a wipe, not a fill"). The current loop is correct but verbose, and a maintainer scanning for credential-handling sites benefits from the named operation.  
**Fix:** Replace each loop with `clear(c.Password)` / `clear(c.APIToken)`. Go spec guarantees `clear` on a `[]T` sets every element to T's zero value, which for `[]byte` is the same byte-by-byte zeroize. Keep the subsequent `c.Password = nil` / `c.APIToken = nil` assignments unchanged — they release the backing array.  
**Effort:** hours


##### `mod:7b2829bb:use-slices-containsfunc` — use slices containsfunc

**Status:** done — PR #161 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** slices-maps  
**Evidence:** `internal/executor/executor.go:132-142`  
**Problem:** `EnvAllowlist.allows` hand-rolls `for _, p := range a.Prefixes { if strings.HasPrefix(key, p) { return true } } return false` — the canonical shape of `slices.ContainsFunc`. Go 1.21 stdlib captures exactly this pattern. The codebase already adopts it elsewhere (`internal/cli/status.go:92`, `internal/logutil/redact.go:120`), so the swap aligns with house style.  
**Fix:** Replace the loop body with `return a.Exact[key] || slices.ContainsFunc(a.Prefixes, func(p string) bool { return strings.HasPrefix(key, p) })`. Add `"slices"` to imports. Net result: -7 LOC, same behavior, matches in-tree usage.  
**Effort:** hours


##### `smell:d31d1b9d:role-string-instead-of-enum` — role string instead of enum

**Status:** done — PR #178  
**Severity:** minor  
**Cluster:** magic-strings  
**Evidence:** `internal/cli/status.go:97-222`  
**Problem:** statusNode.role() returns the bare strings "master"/"worker"/"unknown" and printClusterStatus switch-cases on the same string literals, ignoring the typed phase.NodeRole / phase.RoleMaster / phase.RoleWorker enum that already exists in internal/distribution/okd/phase/noderole.go. The two sites are one rename away from drifting (e.g. someone introduces 'control-plane' upstream) — a typed enum would catch it at compile time.  
**Fix:** Change role()'s return type to phase.NodeRole and return phase.RoleMaster / phase.RoleWorker. Add a phase.RoleUnknown constant or return ("", false). The status counter switch then becomes `case phase.RoleMaster: ... case phase.RoleWorker:` which the compiler enforces against the canonical enum.  
**Effort:** hours


##### `smell:830d4653:duplicate-os-fallback` — duplicate os fallback

**Status:** done — PR #177  
**Severity:** minor  
**Cluster:** helper-package-no-value  
**Evidence:** `internal/distribution/okd/cleanup/packages.go:17-25`  
**Problem:** detectOS in cleanup/packages.go and the inline equivalent in setup/phase.go::New both call platform.Detect(), Warn on error, and fall back to platform.OS{Family: FamilyRHEL, ID: "unknown"}. The exact fallback literal is duplicated in two phases. A canonical platform.DetectOrDefault helper is one-of-each.  
**Fix:** Add `func DetectOrDefault(logger *slog.Logger) OS` to internal/platform/platform.go encapsulating the warn+fallback. Replace the cleanup detectOS function and the setup inline block with a call. Net: -8 LOC, single source of truth for the 'platform-detect failed' decision.  
**Effort:** hours


##### `smell:9d79b841:strconv-fallback-to-zero` — strconv fallback to zero

**Status:** done — PR #174 (moved to Completed)  
**Severity:** minor  
**Cluster:** stringified-numbers  
**Evidence:** `internal/distribution/okd/setup/coreos.go:134-138`  
**Problem:** parseOKDMinor uses fmt.Sscanf with %d and discards the err — a malformed version like '"4.x.0"' resolves to minor=0 and the caller proceeds to fetch fcos.json (the legacy file). Silent fallback through a parse failure rather than typed minor.Parse(...) (uint, ok). Compounded by the doc-comment claiming 'parses to minor 0' as if that were a feature.  
**Fix:** Either (a) return (int, bool) so the caller can refuse the request with a typed errtypes.ConfigError, or (b) at minimum reject minor==0 inside DetectCoreOSVersion before formatting the URL. Today an unparseable version sends okdctl to https://raw.githubusercontent.com/openshift/installer/release-4.0/... which 404s; better to fail-fast at parse time.  
**Effort:** hours


##### `smell:2f70d7df:magic-default-port` — magic default port

**Status:** done — PR #162 (moved to Completed)  
**Severity:** minor  
**Cluster:** magic-strings  
**Evidence:** `internal/distribution/okd/setup/kargs.go:73-73`  
**Problem:** BuildIgnitionURLForNode falls back to the literal 8080 when cfg.HTTPServer.Port is unset, but the canonical DefaultIgnitionPort = 8080 constant lives one file over in setup/phase.go. Two sources of truth — bump the constant and this fallback drifts silently.  
**Fix:** Replace `ignitionPort = 8080` with `ignitionPort = DefaultIgnitionPort`. Both files are in package setup so no import is needed.  
**Effort:** hours


##### `smell:8aa632a6:duplicate-platform-string` — duplicate platform string

**Status:** done — PR #163 (moved to Completed)  
**Severity:** suggestion  
**Cluster:** helper-package-no-value  
**Evidence:** `internal/cli/debug_bundle.go:144-144`  
**Problem:** debug_bundle.go:144 rebuilds `fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)` even though version.Platform — a package-level var holding the exact same expression — is already imported at line 24. Two byte-identical builds of the same string from the same inputs.  
**Fix:** Replace `Platform: fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)` with `Platform: version.Platform` and remove the unused `runtime` import (debug_bundle.go:12) if no other code references it.  
**Effort:** hours


##### `smell:696d6b0e:redundant-vmstatus-enum` — redundant vmstatus enum

**Status:** done — PR #266  
**Severity:** suggestion  
**Cluster:** magic-strings — related: api:262af6e4:opt-inconsistent  
**Evidence:** `internal/distribution/okd/phase/iso_cleanup.go:81-105`  
**Problem:** phase/iso_cleanup.go declares `type vmStatus string; const vmStatusRunning vmStatus = "running"` while internal/infrastructure/proxmox/types.go already exposes `type VMState string; StateRunning VMState = "running"` for the same Proxmox concept. Two parallel enums for the same wire-protocol value. The phase-package shape is private (single value, single use site), but it is still a logical duplicate of the infra enum.  
**Fix:** Either (a) move VMState into a shared package (e.g. phase/) so iso_cleanup.go and proxmox/proxmox.go both reference proxmox.StateRunning, or (b) accept that iso_cleanup parses pvesh JSON (a Proxmox-specific surface) and document the duplication intentionally with a // matches proxmox.StateRunning comment. The current state has neither share-of-truth nor a written reason for the split.  
**Effort:** hours



- **`tst:881d089e:runlock-symlink-untested`** — done 2026-05-05 — PR #341,
  merge commit `0532dfd`. Tier I blocker (canonical-helper-untested).
  Added `TestAcquire_RefusesSymlink` mirroring `cli/logging_test.go:TestOpenLogFileRefusesSymlink`. Plants a dangling symlink at `<dir>/.okdctl.lock`, calls `runlock.Acquire`, asserts `*errtypes.ConfigError` whose `Msg` contains `"symlink"`. Skips on `os.Geteuid()==0` since root bypasses the DAC restrictions. **Postmortem lesson:** dangling symlinks beat symlinks-to-existing-files in security tests — minimal surface, no filesystem state to mask regressions.

- **`sec:1e8ffb91:tls-fallback-skip-verify`** — done 2026-05-05 — PR #342,
  merge commit `29a3ad2`. Tier I major (tls-network).
  `verifyKubeVIPAPIHealthBootstrap` previously fell back to `InsecureSkipVerify` when `KubeconfigCAPool` failed, even outside the documented `x509.HostnameError` window — an attacker with bastion-network access could spoof healthy responses. Made CA-pool retrieval a hard error returning `*errtypes.ClusterError`. **Postmortem lesson:** defense-in-depth fallbacks on credential-bearing probes are an anti-pattern when the probe outcome gates destructive state changes.

- **`state:368b892b:tf-state-backup-still-cleaned`** — done 2026-05-05 — PR #344,
  merge commit `3001b8b`. Tier I major (tf-state-atomicity).
  `terraformFilesToRemove` was wiping `terraform.tfstate.backup` on every destroy. Removed entry and added positive assertion that backup survives. **Postmortem lesson:** distinguish user state (preserve), generated artefacts (delete), rollback metadata (preserve unless opt-in removed) — the third bucket is easy to miss in a denylist.

- **`sub:eb479d86:argv-concat-into-ssh-cmd`** — done 2026-05-05 — PR #345,
  merge commit `3f8dfae`. Tier I major (argv-construction).
  `remoteISO256` was building the remote ssh command via raw string concat. Switched to `phase.SSHRunArgv(ctx, exec, host, "sha256sum", "--", target)`. **Postmortem lesson:** `SSHRunArgv` is not a silver bullet — its doc warns ssh joins atoms with spaces remotely. Filename-from-config sites should pair argv mode with an upstream allowlist.

- **`state:0f076161:dry-run-no-runlock`** — done 2026-05-05 — PR #346,
  merge commit `e50210b`. Tier I major (tf-state-atomicity).
  `runDestroyDryRun` and `runDeployDryRun` ran `terraform init`+`plan` against the shared `.terraform/` directory without holding the project runlock. Added `runlock.Acquire(projectRoot, "<verb> --dry-run")` after `resolveProjectRootOrDie`. **Postmortem lesson:** dry-run paths share on-disk state with non-dry-run paths and need the same concurrency guards.

- **`err:26a430ee:sudo-missing-sentinel-not-wrapped`** — done 2026-05-05 — PR #347,
  merge commit `62ff3c7`. Tier I major (sentinel-vs-typed).
  `ensureRoot` returned `&errtypes.AuthError{Err: lookPathErr}` so `errors.Is(err, errtypes.ErrSudoMissing)` was false at runtime — `exitCodeFor` returned 5 instead of 71 (sysexits EX_OSERR). Wrapped via `errors.Join(err, errtypes.ErrSudoMissing)`. **Postmortem lesson:** every production site that should map to a sentinel exit code must include the sentinel in its `Err` chain — the standalone `exitCodeFor` test doesn't catch the integration gap.

- **`iac:b803fcb7:sh-shellcheck-scope-gap`** — done 2026-05-05 — PR #348,
  merge commit `a1b9588`. Tier I major (install-sh-fail-closed).
  lint-shell job restricted shellcheck `scandir` to `./scripts`, leaving `.github/scripts/coverage-check.sh` (82-line bash) unlinted. Dropped the `scandir` line. **Postmortem lesson:** every shell script in CI is a candidate for shellcheck — restricting scope to a single directory is fragile when scripts grow elsewhere.

- **`ux:aa84670c:update-notice-on-stdout`** — done 2026-05-05 — PR #349,
  merge commit `36658cd`. Tier I major (streams).
  `printUpdateNotice` wrote the update banner via `fmt.Println` to stdout, corrupting JSON streams for `okdctl status --format=json | jq` consumers. Routed via `fmt.Fprintln(os.Stderr, ...)` and gated on `!logQuiet && logFormat != "json"`. **Postmortem lesson:** any stdout write outside the subcommand's declared output is a JSON-pipeline footgun.

- **`sec:40d315ad:argv-injection-leading-dash-host`** — done 2026-05-05 — PR #350,
  merge commit `63cb5c2`. Tier I major (shell-injection (CWE-88)).
  Flux's `gitHost` parsed scp-style URLs (`git@-arg:path`) and returned `-arg`, which `ssh-keyscan` consumed as a flag. Added a guard in both scp and url-parse branches rejecting hosts beginning with `-` or empty, plus an `else if _, hostErr := gitHost(...)` in `ValidateSettings`. **Postmortem lesson:** the choke point for argv-injection is at the data shape level, not at the call site.

- **`state:de572c63:dnsmasq-write-non-atomic`** — done 2026-05-05 — PR #351,
  merge commit `52e0e92`. Tier I major (crash-recoverability).
  `writeDnsmasqConfig` used `WriteTempFile` + `CopyFile` (not atomic). Replaced with `system.AtomicWriteString` (temp+fsync+rename+parent-fsync). **Postmortem lesson:** `WriteTempFile` is for callbacks needing a writeable handle; for pure string content the canonical helper is `AtomicWriteString`.

- **`state:15ba17da:partial-destroy-still-cleans`** — done 2026-05-05 — PR #353,
  merge commit `85232a5`. Tier I major (destroy-safety).
  When `terraform destroy` failed mid-run, the orchestrator continued to `StepCleanupFiles` which wiped tfvars and `.terraform/`, breaking retry. Added `destroyTracker.terraformFailed()` and downgrade `Full → WorkOnly` when terraform destroy failed. **Postmortem lesson:** `NonFatal: true` step semantics need to inform downstream cleanup decisions — blindly running cleanup after a failed destroy makes the failure permanent.

- **`err:48688e63:terraform-bare-fmt-errorf`** — done 2026-05-05 — PR #354,
  merge commit `442479a`. Tier I major (domain-vocabulary).
  `infrastructure/proxmox/proxmox.go` wrapped terraform init/plan/apply errors with bare `fmt.Errorf`, mapping to exit 1 instead of exit 4. Replaced 5 sites with `errtypes.ClusterError` and 3 with `errtypes.ConfigError`. The cancellation branch stays bare (signalExitCode runs first). **Postmortem lesson:** the exit-code taxonomy is only as consistent as the laziness of error wrapping in fan-out call sites.

- **`obs:366b3f2d:step-key-id-vs-name-collision`** — done 2026-05-05 — PR #355,
  merge commit `938ab87`. Tier I major (field-stability).
  `orchestrator.go` had two log sites emitting `step.Name()` under the `"step"` key while five sibling sites used `step.ID()`, breaking JSON pipelines. Standardised on `step.ID()` under `"step"` and added `"name", step.Name()` for human display. **Postmortem lesson:** structured-log key stability is contractual for downstream consumers.

- **`sec:761e5126:haproxy-vip-tls-skip`** — done 2026-05-05 — PR #356,
  merge commit `620bead`. Tier I major (tls-network).
  `RemoveHAProxy` fell back to `InsecureSkipVerify` when `KubeconfigCAPool` failed on a probe whose outcome gates HAProxy teardown. Replaced with hard `*errtypes.ClusterError`; renamed test from `insecure_fallback_when_kubeconfig_absent` to `hard_errors_when_kubeconfig_absent` and tightened the assertion via `Msg` substring match after initial reviewer rejected the diff because the test would have silently passed even if the fix regressed. **Postmortem lesson:** when changing behaviour, the test name and assertions must change too.

- **`doc:66cb1c69:addons-info-renamed-type`** — done 2026-05-05 — PR #357,
  merge commit `8e88e66`. Tier I major (readme-drift).
  `docs/architecture/addons.md` showed `Info() AddonInfo` but the type was renamed to `Metadata`. Updated the snippet. **Postmortem lesson:** doc snippets that mirror an unexported type should be regenerated whenever the type changes.

- **`doc:66cb1c69:addons-configurable-missing-decode`** — done 2026-05-05 — PR #357,
  merge commit `8e88e66`. Tier I minor (readme-drift).
  `docs/architecture/addons.md` documented `ConfigurableAddon` without `DecodeSettings`. Added the third method. **Postmortem lesson:** symmetric triplets must be documented as a triplet.

- **`doc:66cb1c69:addons-environment-fields-stale`** — done 2026-05-05 — PR #357,
  merge commit `8e88e66`. Tier I minor (readme-drift).
  `docs/architecture/addons.md` claimed `Environment` carried `kubeconfig` (does not). Reworded the prose to list `AddonConfig`, `Exec`, `Logger`, `ProjectRoot`. **Postmortem lesson:** struct field lists in prose drift the fastest of all doc content.

- **`tst:451be4fa:writeasinvokinguser-chown-fail`** — done 2026-05-05 — PR #358,
  merge commit `2fd5448`. Tier I major (canonical-helper-untested).
  `WriteAsInvokingUser`'s `AtomicWrite-succeeds-but-Chown-fails` branch was untested; deploy crashes mid-flight could leave a kubeconfig root-owned. Added a sub-test setting `SUDO_UID=65534` and asserting the file persists. **Postmortem lesson:** test the failure shapes that matter operationally, not just the happy path.

- **`sec:632c9087:ingress-rollback-server-fields`** — done 2026-05-05 — PR #359,
  merge commit `344e461`. Tier I suggestion (input-validation).
  `buildRollbackJSON` used a denylist of server-managed metadata fields; future kube versions adding new fields would silently round-trip. Switched to allowlist (`name`, `namespace`, `labels`, `annotations`, `ownerReferences`, `finalizers`). **Postmortem lesson:** allowlist over denylist for security-sensitive paths.

- **`sec:6424733c:metrics-server-no-readtimeout`** — done 2026-05-05 — PR #360,
  merge commit `20ede42`. Tier I minor (tls-network).
  `startMetricsServer` set only `ReadHeaderTimeout: 5s`, leaving the listener vulnerable to slow-body attacks especially when bound to network. Added `ReadTimeout: 10s`, `WriteTimeout: 10s`, `IdleTimeout: 60s`. **Postmortem lesson:** Go's `http.Server` zero-values are too generous for any production listener.

- **`obs:8e65d574:slog-default-package-coupling`** — done 2026-05-05 — PR #361,
  merge commit `29250f4`. Tier I minor (handler-setup).
  `version.runCheck` calls package-level `slog.Debug` routing through `slog.Default()`. Future callers bypassing `cli.Execute` would silently bypass `RedactHandler`. Documented the implicit coupling on `BackgroundCheck`'s doc comment. **Postmortem lesson:** package-level `slog.Default()` calls are a foot-gun; document at minimum so future refactors recognise the invariant.

- **`ux:0d318f5c:quiet-suppresses-only-info`** — done 2026-05-05 — PR #362,
  merge commit `7b295eb`. Tier I minor (streams).
  `--log-format=json` did not call `tui.SuppressInfo`, so JSON log streams interleaved `tui.Info` chatter with structured slog records. `configureLogging` now calls `tui.SuppressInfo` when `logFormat == "json" && !logVerbose`. **Postmortem lesson:** the `tui.Info`/`slog` split is two output channels through stderr; JSON consumers need both gated.

- **`obs:b38ec9cc:per-record-string-format-vs-attrs`** — done 2026-05-05 — PR #365,
  merge commit `27487de`. Tier I minor (field-stability).
  `install/workers.go:20` interpolated `cfg.Topology.Workers.Count` into the message string via `fmt.Sprintf`. Replaced with `p.Log.Info("workers: starting", "count", N)`. **Postmortem lesson:** the broader `fmt.Sprintf`-into-message anti-pattern is tracked under `obs:6424733c:fmt-sprintf-message-pattern`.

- **`iac:e076e43c:sh-bash-array-dash-incompat`** — done 2026-05-05 — PR #340,
  merge commit `434b118`. Tier I blocker (install-sh-fail-closed).
  `install.sh` declared bash arrays (`_gh_auth_header=()`/`${_gh_auth_header[@]}`) but the README and docstring directed users to pipe into `sh`. On Debian/Ubuntu (`/bin/sh` = dash) the script syntax-errors at the array declaration before any download starts. Updated README and the install.sh docstring to pipe to `bash` instead of `sh`, matching the helm.sh / k3s.io pattern. **Postmortem lesson:** the shebang `#!/usr/bin/env bash` is ignored when the script is read via stdin by `sh`; documentation must specify the interpreter explicitly.

- **`state:29293401:haproxy-install-non-atomic`** — done 2026-05-05 — PR #343,
  merge commit `3280e55`. Tier I major (crash-recoverability).
  `installHAProxyConfig` and the rollback restore path used `system.CopyFile`/`CopyFileMode` (not atomic). A SIGKILL between truncate and io.Copy completion left the live `haproxy.cfg` empty and broke the bastion's kube-api fronting. Replaced both sites with `os.ReadFile` + `system.AtomicWriteString`. The backup snapshot at line 111 (not on the trust-boundary path) is unchanged. **Postmortem lesson:** any write to a file that a service re-reads on restart must be atomic — the truncate window is the failure mode that costs the bastion.

- **`state:18a795d5:masters-no-prevent-destroy`** — done 2026-05-05 — PR #352,
  merge commit `a4cc0fe`. Tier I major (destroy-safety).
  Master VM resource had no `lifecycle { prevent_destroy = true }`. A rogue `--target=module.okd_cluster.proxmox_virtual_environment_vm.master[N]` passed `destroyTargetRE` and could destroy an etcd-quorum member with zero terraform-side guard. Added the lifecycle block; documented the override.tf escape hatch in the `destroy --help` long text. **Postmortem lesson:** `prevent_destroy` is a literal-only field in HCL; defense-in-depth via terraform requires hardcoding plus a documented opt-out path.

- **`sec:c8b28673:archive-mkdirall-mode`** — done 2026-05-05 — PR #363,
  merge commit `8946c52`. Tier I minor (file-toctou).
  `processTarEntry` called `os.MkdirAll` with the archive-supplied mode (`header.Mode & 0o777`). A malicious archive declaring 0o777 on a directory created it world-writable, opening a TOCTOU window between MkdirAll and the subsequent file extraction. Masked against 0o755. **Postmortem lesson:** archive-supplied modes need a max-bound mask before any privileged write — `0o755` for directories matches `tar --no-same-permissions`.

- **`sec:9d79b841:coreos-stream-no-pin`** — done 2026-05-05 — PR #364,
  merge commit `11b675c`. Tier I minor (tls-network).
  `fetchCoreOSStream` GETs `fcos.json`/`scos.json` from `raw.githubusercontent.com` over HTTPS with no out-of-band signature or commit pin — GitHub's TLS cert is the sole trust anchor. The ISO artefact itself is sha256-verified at download, but the JSON document supplying the checksum is not. Added a doc-comment on `fetchCoreOSStream` documenting the trust model so future maintainers don't try to add their own pinning without coordinating. README §security-considerations was skipped to avoid conflict with in-progress sec:06f00bcb. **Postmortem lesson:** when a fix is documentation-only because the right code change requires cross-cutting work, name the gap in the code comment so a future PR has a clear seam.

- **`obs:ddf885f4:install-failure-drops-addon-attr`** — done 2026-05-05 — PR #366,
  merge commit `114212d`. Tier I minor (field-stability).
  `Manager.InstallAll`'s failure-log at line 98 dropped the `addon` attr that every neighbouring site emits. When two addons fail in the same run, the resulting log records were indistinguishable. Added `"addon", info.DisplayName`. **Postmortem lesson:** every record inside a per-element loop should carry the element identifier as a structured attr — without it, JSON pipelines can't group by element.

- **`doc:23d812fa:flux-doc-missing-provider-setting`** — done 2026-05-05 — PR #369, merge commit `68ae944`. Tier H minor (readme-drift). `docs/addons/flux.md` default-settings table omitted the `provider` key (default `"flux"`) returned by `Flux.DefaultSettings`. Added a `provider | flux | toolkit identifier; only `flux` is currently shipped` row at the top of the table. **Postmortem lesson:** when a Go function returns a settings map, the doc table needs a row per key, not just the ones with non-trivial defaults — operators read the table to discover overridable knobs.

- **`doc:3229e1a6:doctor-checks-exit-code-drift`** — done 2026-05-05 — PR #370, merge commit `68ffcf8`. Tier H minor (readme-drift). `docs/doctor-checks.md` claimed doctor exits 1 on fail; actual code returns `*errtypes.ConfigError` mapped to 2 by `exitCodeFor`. Companion doc `docs/cli/okdctl_doctor.md` already correctly said 2. Updated this doc to match. **Postmortem lesson:** when two docs describe the same surface, drift catches one but not the other — a CI grep that compares exit-code claims across `docs/` would close the loop.

- **`doc:b3356305:readme-minimal-yaml-drift`** — done 2026-05-05 — PR #371, merge commit `7f2970e`. Tier H minor (readme-drift). README claimed `minimal.yaml` had 3 control-plane nodes; actual `configs/examples/minimal.yaml` has `control_plane.count: 1` (single-node). Fixed the bullet to "1 control-plane node, 0 workers (single-node cluster)". **Postmortem lesson:** README descriptions of YAML examples drift the moment the YAML changes — schema + example consistency needs a generator or test, not prose.

- **`doc:262af6e4:cleanup-pkgdoc-over-3-sentences`** — done 2026-05-05 — PR #372, merge commit `fa38f4f`. Tier H suggestion (package-doc). Cleanup package doc had 6 sentences including an aspirational two-pass-checkpoint design note. CLAUDE.md targets 1–3 sentences. Trimmed to 3: what the package does, best-effort partial-fail behavior, terraform-state-removed-last invariant. **Postmortem lesson:** aspirational design notes belong in roadmap entries, not package docs — they rot in place as the design either lands or fades.

- **`doc:88fd3050:cluster-pkgdoc-over-3-sentences`** — done 2026-05-05 — PR #373, merge commit `d77da0e`. Tier H suggestion (package-doc). Config package doc spanned 5 sentences. Tightened to 3: what the package defines, the YAML serialization library, the json-tag-only invariant. Dropped the redundant schema-enumeration parenthetical. **Postmortem lesson:** when a doc lists every field the type already exports, the doc is duplicating type-system signal — drop the enumeration, keep the invariant.

- **`doc:a4001485:errtypes-pkgdoc-over-3-sentences`** — done 2026-05-05 — PR #374, merge commit `3c1ed46`. Tier H suggestion (package-doc). Tightened from 5 to 2 sentences: package purpose + exitCodeFor cross-reference with the broad code mapping (now also includes UsageError=64). Per-sentinel var docs already carry their specific BSD sysexits codes. **Postmortem lesson:** when per-symbol docs already carry the specifics, the package doc should carry only the cross-reference and the broad invariant — anything else is duplication.

- **`err:1e8ffb91:double-context-wrap-kubevip`** — done 2026-05-05 — PR #375, merge commit `60ec542`. Tier H minor (wrapping). `waitForKubeVIPDaemonSet` wrapped its error with the same "kube-vip daemonset not ready" prefix that `VerifyKubeVIP`'s outer `*errtypes.ClusterError` already supplied, producing a double-prefix. Replaced inner `fmt.Errorf` with `return err`. The two sibling sites (vip ping at L217, api health at L178) carry distinct messages and were intentionally left unchanged. **Postmortem lesson:** "wrap once, own the prefix at the outer boundary" — when both inner and outer wrap the same string, the inner is the one to drop because the outer carries the typed error category.

- **`err:6424733c:metrics-addr-loses-chain`** — done 2026-05-05 — PR #376, merge commit `ef35f3a`. Tier H minor (wrapping). `startMetricsServer` formatted the `net.SplitHostPort` error into `Msg` via `%s`, breaking `errors.Is/As` against the underlying `*net.AddrError`. Moved `err` into the `Err` field of `*errtypes.ConfigError`; `Error()` is unchanged because `ConfigError.Error()` formats both. **Postmortem lesson:** typed errors with a `Msg` and an `Err` field exist precisely so the chain is preserved — `%s err` interpolation turns an unwrappable error into a string.

- **`err:761e5126:network-vs-cluster-inconsistent`** — done 2026-05-05 — PR #377, merge commit `ede29aa`. Tier H minor (domain-vocabulary). Two adjacent post-haproxy-removal probes returned different typed errors for the same conceptual failure: line 90 used `NetworkError` (exit 3); line 105 used `ClusterError` (exit 4). Aligned line 90 to `ClusterError`; both teardown probes now share exit code 4. **Postmortem lesson:** when the failure surface is "cluster-API unreachable", `ClusterError` is the right type — `NetworkError` is for raw layer-3/4 failures.

- **`err:97cb8adf:stderr-tail-in-error-text`** — done 2026-05-05 — PR #378, merge commit `9b36ec5`. Tier H minor (redaction-in-error). `RunCaptured` and `OutputCaptured` folded subprocess stderr into the error string via `%s`, bypassing `logutil.RedactHandler`. Introduced `system.SubprocessError{Bin, Err, StderrTail}` whose `Error()` preserves the existing format for callers but `Redacted() any` omits `StderrTail` so stderr never reaches structured slog sinks. **Postmortem lesson:** the `Redacted() any` interface is the seam where credentials leak into logs — any error type that carries free-form subprocess output needs to implement it.

- **`err:f55b9c27:envfile-parsedotenv-leaks-line`** — done 2026-05-05 — PR #379, merge commit `a869cef`. Tier H suggestion (redaction-in-error). `parseDotEnv` returned `fmt.Errorf("malformed line (no '='): %q", line)` — only fired today on lines without `=`, so no current secret leak, but a future change widening accepted line shapes (continuation lines, single-quoted keys) would silently leak credentials. Replaced with `fmt.Errorf("malformed line %d (no '=')", lineNum)`. **Postmortem lesson:** a parser's error-on-bad-input must report position-only, never the raw input — even a "safe today" branch is a foot-gun for the next maintainer who widens the input class.

- **`err:a55b4592:loader-save-no-typed-wrap`** — done 2026-05-05 — PR #380, merge commit `da3165b`. Tier H suggestion (domain-vocabulary). `Loader.Save` returned raw `yaml.Marshal` and `AtomicWrite` errors while the five `Loader.LoadFile` sites consistently wrapped in `*errtypes.ConfigError`. Asymmetry meant Save failures would map to exit 1 instead of exit 2. Wrapped both error paths to match the LoadFile siblings. **Postmortem lesson:** symmetric API methods (Load/Save) on the same type should wrap errors with the same typed wrapper — exit-code drift inside one struct is invisible at the call site.

- **`mod:04f0e35f:use-builtins`** — done 2026-05-05 — PR #381, merge commit `83390c7`. Tier H suggestion (any-interface-builtins). Replaced the explicit zero loop in `system.ZeroBytes` with `clear(b)` (Go 1.21 builtin). Both forms compile to `runtime.memclrNoHeapPointers` — security semantics identical, intent clearer. The rest of the credentials path already uses `clear()`. **Postmortem lesson:** when stdlib gains a builtin that expresses the exact intent of a hand-rolled loop, migrate the loop — the builtin documents intent more precisely than the loop body.

- **`smell:6fc3d91e:stringly-typed-enum`** — done 2026-05-05 — PR #382, merge commits `337cc28` (introduce) + `c85daef` (gofumpt fix). Tier H minor (magic-strings). `OS.Family` was an untyped `string` while every comparable concept (NodeRole, VMRole, VMState, ClusterPhase) used a typed enum. Introduced `type Family string`, retyped the constants and fields. Caller comparisons against named constants typecheck unchanged. The gofumpt follow-up split the const block because mixing untyped `archARM64` with typed `Family` constants confuses the formatter. **Postmortem lesson:** a typed-string enum needs its own const block in Go 1.25+ — gofumpt rejects mixed-type const blocks where some entries declare a type and others don't.

- **`smell:e7db1220:abstraction-single-caller`** — done 2026-05-05 — PR #383, merge commit `2aef815`. Tier H suggestion (helper-package-no-value). `filterStable` was a 6-LOC helper used exactly once in `runReleasesList`. Inlined as a single `slices.DeleteFunc` call (slices already imported). **Postmortem lesson:** Go 1.21+ stdlib `slices` package makes most one-liner helper functions redundant — the right test is "is the body shorter than `slices.X(predicate)`?".

- **`dep:b803fcb7:golangci-lint-version-drift`** — done 2026-05-05 — PR #384, merge commit `4e214c8`. Tier H minor (pin-stability). CI pinned golangci-lint to v2.12.1; Makefile pinned v2.11.4 — local `make lint` ran a different rule set than CI. Bumped Makefile pin to match. The single-source-of-truth refactor (shared variable / .tool-versions) is deliberately deferred. **Postmortem lesson:** any tool pinned in two places will drift — the second pin is the regression waiting to happen.

- **`api:881d089e:lock-method-on-zero-value`** — done 2026-05-05 — PR #385, merge commit `9308175`. Tier H suggestion (zero-value-usability). `runlock.Lock.Release` panicked with a nil-pointer deref on a nil receiver or zero-value `Lock{}`. Added a guard at the top making both paths a safe no-op. Updated the doc comment. **Postmortem lesson:** any method on a pointer receiver to an unexported-field type should guard `nil` and the zero value — Go's "zero value usable" idiom requires the API to handle them.

- **`con:15ba17da:detect-backend-ctx-bg`** — done 2026-05-05 — PR #387, merge commit `8b4ecc1`. Tier H minor (ctx-todo). `StepCleanupFirewall.SkipWhen` called `firewall.DetectBackend(context.Background(), p.Log)`, which spawns `systemctl`/`ufw` subprocesses — a wedged-systemd probe under `Background` would block Ctrl-C indefinitely and wedge the orchestrator's serial step loop. Threaded `ctx` through `destroySteps(ctx, ...)` to the SkipWhen closure; updated 4 test callsites. **Postmortem lesson:** `context.Background()` inside a `SkipWhen` closure that spawns subprocesses is a wedge-on-cancel hazard — every closure spawning external processes must capture an outer ctx.

- **`doc:aa84670c:exit-code-77-pkgdoc-drift`** — done 2026-05-05 — PR #368, merge commit `b41600a`. Tier H major (package-doc). `internal/cli/root.go` package doc claimed invoked-as-root rejection exits 77 (EX_NOPERM); actual code returns `*errtypes.AuthError` mapped to 5 by `exitCodeFor` — matching `docs/cli/exit-codes.md`. Removed the false claim and added a parenthetical to the existing auth-error=5 entry noting it covers invoked-as-root rejection. **Postmortem lesson:** when a package doc and a user-facing doc both describe exit codes, keep the user-facing doc canonical and have the package doc cross-reference it instead of duplicating the table.

- **`obs:0d318f5c:no-tty-format-default-mismatch`** — done 2026-05-05 — PR #386, merge commits `ea26b29` (feat) + `24173ec` (regen docs) + `70ae032` (goconst fix). Tier H minor (handler-setup). `configureLogging` defaulted `--log-format=text` regardless of TTY state, bleeding ANSI text into pipes. Now `configureLogging(cmd *cobra.Command)` calls `cmd.Root().PersistentFlags().Changed("log-format")` and switches to `outputJSON` when the flag was not explicitly set AND stderr is not a TTY. Help text updated. Required two follow-up commits: `make docs` regeneration (cobra help-text drift catches this in CI's `docs-go` check) and a `goconst` lint fix to reuse the existing `outputJSON` const instead of the literal `"json"`. **Postmortem lesson:** changing cobra flag help text triggers `docs-go` CI; introducing a literal that shadows an existing const triggers `goconst` — both are mechanical lints worth running locally before push.


- **`sec:35abd54e:env-string-immutable-residue`** — done 2026-05-06 — PR #388, merge commit `02d7fbb`. Tier I minor (credentials). `ProxmoxCredentials.Env()` converts `[]byte` Password/APIToken to immutable Go strings the receiving struct retains for the caller's full function frame — but the original doc said "pass directly to cmd.Run" which understated the actual residue window. Tightened the doc to spell out which structs hold the strings (`Executor.Env`, `Provider.env`, `Provisioner.pendingEnv`) and the three caller obligations (pass directly to WithEnv, defer Zeroize in the same frame, never escape to a goroutine/cache). **Postmortem lesson:** When the Go heap immutability of `string` makes a redaction guarantee impossible, the doc must name every struct that retains the string so future refactor reviewers can see the leak window.

- **`sec:eb479d86:scp-strict-host-key-tofu`** — done 2026-05-06 — PR #389, merge commit `dfa87d3`. Tier I minor (tls-network). `uploadISOsViaSCP` in `internal/distribution/okd/setup/upload.go:77-86` uses `StrictHostKeyChecking=accept-new` — a TOFU window where a first-run MITM permanently pins an attacker key for every subsequent SSH/SCP call. The accept-new behaviour itself stays (the per-cluster `proxmox.host_fingerprint` config field is owned by sec:27088eab, in flight). This PR landed the README §security-considerations entry naming the risk and the out-of-band verification path (Proxmox UI → Node → Shell → `ssh-keygen -lf`). **Postmortem lesson:** When a security mitigation is split across two PRs by ownership, the doc-only PR should land first so the risk is named to operators before the fix lands.

- **`sec:696d6b0e:iso-cleanup-shellquote-policy`** — done 2026-05-06 — PR #390, merge commit `2363b8f`. Tier I suggestion (shell-injection). `RemoveFCOSISOFromProxmox` (`internal/distribution/okd/phase/iso_cleanup.go:205-253`) is the only `sh -c <variable>` shellout in the repo, guarded by `validateISODir` + `refuseUnsafeISOPath` + `shellSingleQuote`. Recorded the policy as a function-level doc comment and cross-linked it from CLAUDE.md §Architecture notes so a future SSH addition does not silently grow a second sh-c usage without the same guards. **Postmortem lesson:** Sole-instance security-sensitive patterns need both a code-side comment and a CLAUDE.md cross-link — the former survives refactors, the latter survives full rewrites.

- **`sec:8e65d574:updatecheck-cache-no-perm-check`** — done 2026-05-06 — PR #391, merge commit `2cf1010`. Tier I suggestion (credentials). `loadCache` in `internal/version/updatecheck.go:118-138` read `~/.cache/okdctl/update-check.json` without checking perms. Added an `os.Stat` + `mode&0o022` check before `os.ReadFile`; group/world-writable cache files are logged at Warn and ignored. The cache currently holds only public release tags — the gate exists for the future field that drives behaviour (auto-update target). **Postmortem lesson:** Mirror the loadEnvFileOnce pattern when reading any user-controllable file under `~/.cache/` — the threat is poisoning, not disclosure, so a `0o022` mask is the right gate.

- **`sec:bdf5a873:secretbytes-string-immutable-leak`** — done 2026-05-06 — PR #392, merge commit `add50a1`. Tier I suggestion (credentials). `SecretBytes.Set` in `internal/config/secret.go:16-19` copies a string into an owned `[]byte`, but the argument string itself stays heap-resident until GC — a known footgun the original comment already named. Audited the tree: one production caller (`internal/tui/wizard/steps/proxmox.go:68`, password ConfigSet callback). Recorded the boundary contract on the Set doc so future reviewers can grep `.Set(` and see whether a new caller is in policy. **Postmortem lesson:** When the type system can't enforce a single-caller contract, a doc-of-record naming the authorised caller package by import path is the cheapest grep-auditable substitute.

- **`sec:cfcdee2d:newinsecure-policy-umbrella`** — done 2026-05-06 — PR #393, merge commit `3e9a8a5`. Tier I suggestion (tls-network). `httputil.NewInsecure` is a TLS-skip constructor used during the bootstrap window before kubeconfig CA is available. Today's only caller is `internal/distribution/okd/postinstall/verify.go:265`. Added `internal/httputil/httputil_newinsecure_policy_test.go` — a static-analysis test that walks `internal/`, parses every non-test `.go` file with `go/parser`, and fails CI when a file outside `internal/distribution/okd/postinstall/` both imports httputil and calls `httputil.NewInsecure`. Mirrors the `errtypes_credleak_test.go` pattern. **Postmortem lesson:** Static AST tests are the right shape for "no caller outside this directory may use this symbol" policies — stronger than a build tag, no runtime cost, fails CI on the offending PR.

- **`sec:de572c63:nm-active-conn-shellable-name`** — done 2026-05-06 — PR #394, merge commit `ff99be9`. Tier I suggestion (input-validation). `validateConnectionName` in `internal/distribution/okd/dns/dnsmasq.go:108-119` previously denylisted `;\n\r\x00\`$<>|&` — wrong shape per CLAUDE.md §security-invariants because new shell metacharacters slip through. Replaced with a precompiled allowlist `^[A-Za-z0-9 ._/:-]{1,128}$` covering NetworkManager's realistic name space (alphanumerics, space, dot, underscore, slash, colon for interface aliases like `br0:1`, hyphen). New table-driven tests cover length-128 boundary, slash, length-129 reject, and `; rm -rf /` injection. **Postmortem lesson:** Allowlists are the only future-proof shape for shell-context input validation — denylists silently widen as new metacharacters land.

- **`sub:0934cf1b:coreutil-dpkg-arch`** — done 2026-05-06 — PR #395, merge commit `f647b0a`. Tier I minor (coreutils-shellout). `AddRepo` in `internal/platform/packages.go:124-128` shelled out to `dpkg --print-architecture` to derive the apt sources.list `arch=` token. The same value comes from `platform.DownloadArch()` (returns `runtime.GOARCH`) which already resolves amd64/arm64 the way Debian dpkg does. Removed one fork+exec per AddRepo and the runtime dep on dpkg being on PATH. **Postmortem lesson:** Before reaching for a coreutils shellout, check whether stdlib (`runtime.GOARCH`, `os.Hostname`, `os.UserCacheDir`, etc.) covers it — the Go answer is usually present and faster.

- **`sub:bb81a5b0:unbounded-partial-line`** — done 2026-05-06 — PR #396, merge commit `cbe8a08`. Tier I minor (io-handling). `ringWriter.partial` in `internal/executor/ringbuf.go:14-42` capped completed lines at `constMaxLines=200` but the partial-line buffer (no trailing newline yet) grew without bound. A subprocess streaming a multi-MB JSON object without newlines (or a binary streaming progress bytes) would balloon retained memory. Added `maxPartial = 64*1024` and a flush loop that pushes overflow into the ring as synthetic lines so total memory stays within `maxPartial * (constMaxLines+1)` ≈ 12.8 MiB. **Postmortem lesson:** Any streaming buffer that's bounded on one dimension (lines, here) needs to be bounded on every dimension — a malicious or misbehaving producer always finds the unbounded one.

- **`state:0f076161:dry-run-skip-flags-silent-noop`** — done 2026-05-06 — PR #397, merge commit `5629193`. Tier I minor (destroy-safety). `runDestroy` in `internal/cli/destroy.go:67-102` documented `--skip-terraform`/`--skip-cleanup`/`--skip-firewall` as "no-op with --dry-run" but the dry-run path silently dropped them. An operator running `destroy --dry-run --skip-terraform` had no signal the flag was discarded. Added a guard that returns `*errtypes.ConfigError` (exit 2) naming every offending flag (joined with `, `) when `destroyDryRun` is true and any skip-* is set. **Postmortem lesson:** "Silently ignore" is never the right behaviour for a CLI flag — fail-fast with a typed error so the operator sees the misuse instead of trusting the wrong outcome.

- **`state:62cb8a95:destroy-hasstate-empty-state`** — done 2026-05-06 — PR #398, merge commit `5a7d7d1`. Tier I minor (tf-state-atomicity). `Executor.HasState()` in `internal/infrastructure/terraform/terraform.go:313-315` was a `system.FileExists` check, so any file (empty `{}`, partial JSON from a crashed mid-write, garbage) returned true and sent destroy down a guaranteed-to-fail tf init/plan path. Now parses the JSON and requires `len(.resources) > 0`. Parse failures log a Warn naming the path so the operator sees corruption instead of a confusing tf error after init succeeds. Updated `seedTerraformEnvDir` test fixture to write populated state since the empty-state path is covered separately. **Postmortem lesson:** When a probe like `HasState()` is the gate for a destructive workflow, the probe must verify the actual property the workflow needs — file existence is rarely sufficient.

- **`state:b804b2ec:bootstrap-cleanup-tfvars-precondition`** — done 2026-05-06 — PR #399, merge commit `43a2b3d`. Tier I minor (phase-idempotency). `StepCleanupBootstrap` in `internal/distribution/okd/postinstall/steps.go:42-108` was `ReRunSafeNo` with a tfvars-existence SkipWhen. The guard checked file existence, not bootstrap_enabled state — a re-run with bootstrap_enabled already false planned a no-op and silently "succeeded". Flipped to `ReRunSafeYes` (matches reality: tf -target apply with destroy diff is genuinely idempotent) and dropped the misleading guard; tf init fails loudly if tfvars are missing. Cleaned up the now-unused `filepath` and `system` imports. **Postmortem lesson:** `ReRunSafeNo` belongs to steps that genuinely cannot be re-run safely — when the underlying tool already handles re-runs idempotently, claiming otherwise just adds a useless gate.

- **`state:fb54208a:postinstall-mutates-skipped-cleanup`** — done 2026-05-06 — PR #400, merge commit `8f4246f`. Tier G minor (crash-recoverability). `update-ingress` in `internal/distribution/okd/postinstall/update_ingress.go:84-91` returned a hard error when `discoverIngressControllers` failed or returned zero controllers — even when DNS was still bootstrap-pointed and the cluster API had become unreachable mid-postinstall. Added a `reconcileBootstrapDNSOnly` shortcut: when bootstrap DNS is detected and discovery fails or returns zero controllers, deploy production DNS without querying the cluster (api.* → vip, *.apps → bastion). Also wired `WorkDir: projectRoot` through `UpdateIngressOptions` from `internal/cli/update_ingress.go:127-130` — RemoveHAProxy was using a relative path for the kubeconfig CA cert. **Postmortem lesson:** When postinstall mutates external state (DNS, VMs) but skips a downstream step on a verify failure, the downstream verb (`update-ingress` here) must have a re-entry path that completes the half-finished migration without re-running the cluster-touching parts.

- **`state:48688e63:no-state-version-check`** — done 2026-05-06 — PR #401, merge commit `1b8354d`. Tier I suggestion (state-schema-evolution). `destroyInfrastructure` in `internal/distribution/okd/destroy/helpers.go` ran tf init/plan against any tfstate present, even when written by a future terraform major. Added `checkStateMajorVersion` preflight: parse `.terraform_version`, refuse with `*errtypes.ConfigError` when major falls outside [1, 1] window (matches `infrastructure/terraform/environments/production/versions.tf` `>= 1.10, < 2.0`). Parse failures are non-fatal (logged Warn, returns nil with `//nolint:nilerr`) so terraform's own error path still fires. **Postmortem lesson:** Tools that pin a major-version range in versions.tf should also pin it in code at every call site that reads tfstate — terraform itself errors, but "errors after init succeeds" is worse UX than "errors before init starts".

- **`state:b38ec9cc:install-workers-tfvars-mutation`** — done 2026-05-06 — PR #402, merge commit `49df855`. Tier I suggestion (phase-idempotency). `StartWorkerVMs` in `internal/distribution/okd/install/workers.go:14-51` overrides `start_workers_immediately` via `-var` at apply time; the on-disk tfvars stays at the deploy-time snapshot value. A manual `terraform plan` from the workdir would diff against the in-memory override, confusing operators reading state-as-source-of-truth. Added a doc comment naming the convention: tfvars is the deploy-time snapshot, tfstate is the source of truth, the diff in `terraform plan` is expected. **Postmortem lesson:** When a tool override-via-`-var` instead of editing tfvars, the divergence is invisible to the operator unless the doc names it — the diff they see in `terraform plan` is the question, the doc is the answer.

- **`iac:e076e43c:sh-insecure-fail-open`** — done 2026-05-06 — PR #403, merge commit `e1336a2`. Tier I major (install-sh-integrity). `scripts/install.sh` previously let `INSECURE=1` skip BOTH cosign and sha256 when sha256sum was missing — a single env var bypassed every integrity guard. First fix landed: refuse to install when sha256sum is missing (it ships with coreutils on every supported distro), restrict `INSECURE=1` to bypassing cosign only, and download SHA256SUMS unconditionally. Round-1 reviewer caught that the cosign block had no `$INSECURE` gate; round-2 added `[ -z "$INSECURE" ]` and an explicit "cosign skipped" info line. **Postmortem lesson:** When changing a security-flag's semantics, audit every reference to the flag — the warning-text update was easy to spot, the missing gate on the cosign block was the actual change.

- **`iac:90de5406:hcl-prod-defaults-baked`** — done 2026-05-06 — PR #404, merge commit `2360e5f`. Tier I minor (hcl-credential-hygiene). `infrastructure/terraform/environments/production/variables.tf` shipped operator-specific defaults (`target_node = "pve01"`, `cluster_name = "grappleberry"`, `vmid_base = 7000`). A bare `terraform apply` without overrides silently deployed to the original lab. Dropped the `default` attribute on those three; terraform now fails fast with a missing-required error. Shape vars (`cpu_cores`, `memory_mb`) keep their defaults. **Postmortem lesson:** Terraform `default` values that are operator-specific are an anti-pattern — they convert a missing-config error into a silent wrong-target deploy.

- **`api:c287d5c0:public-fields-bypass-options`** — done 2026-05-06 — PR #405, merge commit `1ae2755`. Tier I minor (option-consistency). `phase.WithRecorder` and `install.Phase.Reporter` both exist as scaffolding for the api:beabab0c migration that will thread Recorder/Reporter through `phase.New` as functional options. Today no caller threads `WithRecorder`; `okd.Provisioner` writes the exported field directly. Doc-only PR: extended the doc comments on both to cite api:beabab0c as the migration owner so the next audit recognises intent. **Postmortem lesson:** When scaffolding awaits a follow-up refactor, the cheapest annotation is a doc comment naming the owning roadmap item — without it, the next audit re-flags the same surface.

- **`api:fde34e0c:k8sclient-construction-side-effect`** — done 2026-05-06 — PR #406, merge commit `3e2498d`. Tier I minor (zero-value-usability). `NewK8sClient` in `internal/cluster/k8s.go:52-81` read `os.Getenv(KUBECONFIG)` and probed PATH for `oc` inside the constructor body, so zero-value/test construction silently inherited env state. Moved both side effects into a new `WithEnvFallback()` option callers explicitly opt into. Today's sole production caller (`internal/distribution/okd/install/monitor.go:92-96`) already passes `WithCLI`/`WithKubeconfig` explicitly, so behaviour is unchanged. **Postmortem lesson:** Constructor side effects break test reproducibility and zero-value usability — push them out into named options so the call site documents what env state it intentionally consumes.

- **`api:8aa632a6:version-globals-mutable`** — done 2026-05-06 — PR #407, merge commit `27ab05c`. Tier I suggestion (zero-value-usability). `internal/version/version.go:9-16` exports five mutable package-level vars (`Version`, `GitCommit`, `BuildDate`, `GoVersion`, `Platform`) injected via `-ldflags`. `BackgroundCheck` reads `Version` from a goroutine without sync — a runtime write would race. Recorded the build-time-only write contract on the var block including the test-cleanup discipline (save/swap/restore in `t.Cleanup`). **Postmortem lesson:** When a Go binary uses ldflags-injected globals, the doc must explicitly forbid runtime writes — "set at build time" is too easy to misread as "writable".

- **`api:c4182b1c:phasecontext-no-reentry-footgun`** — done 2026-05-06 — PR #408, merge commit `bc9f902`. Tier I suggestion (zero-value-usability). `PhaseContext[T]` in `internal/distribution/context.go:1-33` had a no-reentrancy invariant buried in the type doc — caller closures reaching `Get`/`Update` recursively would deadlock. Added a sentinel `initialized bool` set in `NewPhaseContext`; `Get` and `Update` panic on the zero value. Moved the no-reentrancy warning from the type doc to the `Update` method doc where callers will read it. **Postmortem lesson:** When the type system can't enforce a constructor-only invariant, a sentinel-field panic on the zero value is the cheapest runtime guard — better than a doc comment alone.

- **`ux:fd2125dd:addon-list-no-json-format`** — done 2026-05-06 — PR #409, merge commit `afcf0f3`. Tier I minor (json-stability). `addon list` and `addon verify` were the only read-only inspection commands without `--format=json` — sibling commands `releases list/show`, `status`, `describe node/addon` all had it. Added per-command format vars (`addonListFormat`, `addonVerifyFormat`), routed through `validateFormat` + `quietForJSON`. JSON shapes documented in `docs/cli/json-schema.md`. Result slices use `make([]T, 0, n)` so empty input emits `[]` not `null`. Required a follow-up commit to regenerate `docs/cli/okdctl_addon_*.md` after the cobra help-text drift. **Postmortem lesson:** Adding a new cobra flag to a command always triggers `docs-go` CI — `make docs` belongs in the same commit as the flag registration.

- **`ux:073d24ed:metrics-allow-network-unguarded`** — done 2026-05-06 — PR #410, merge commit `ea66650`. Tier I suggestion (flag-conventions). `--metrics-allow-network` had no effect without `--metrics-addr`, so an operator copy-pasting from an issue thread without realising both were required got silently-disabled metrics. The planner suggested `MarkFlagsRequiredTogether` but that would also reject `--metrics-addr` alone (the common case). Implemented the one-way guard: top of `runDeploy` returns `*errtypes.ConfigError` when `deployMetricsAllowNetwork && deployMetricsAddr == ""`. **Postmortem lesson:** `MarkFlagsRequiredTogether` is symmetric — when the dependency is one-way ("A requires B" but "B alone is fine"), the planner shouldn't reach for it.

- **`smell:9d79b841:bool-should-be-3state`** — done 2026-05-06 — PR #411, merge commit `f70abfb`. Tier I minor (bool-should-be-enum). `findOrDownloadFCOSISO` in `internal/distribution/okd/setup/coreos.go:37-100` encoded three states (unconfigured, resolved, missing) in control flow — a missing operator-pinned ISO silently fell through to the glob loop and a fresh download. Lifted spec resolution into a pure `resolveConfiguredISO` helper that returns an `isoResolution` enum (Empty/Resolved/Missing); caller now refuses to fall through on Missing with `*errtypes.ConfigError`. The `local:iso` (no filename) edge case still falls through to glob. **Postmortem lesson:** When control flow encodes more than two states, the variable should be an enum — the silent-fallthrough bug is invisible until the third state actually fires in production.

- **`tst:368b892b:cleanup-tf-partial-untested`** — done 2026-05-06 — PR #412, merge commit `b2cd471`. Tier I minor (destructive-untested). `cleanupTerraformEnv` in `internal/distribution/okd/cleanup/infra.go:58-73` discards every per-file remove error so a permission denial on `.terraform/providers` never aborts the loop — an invariant with no test. Added `TestCleanupTerraformEnv_PartialFailureDoesNotAbort` that chmods `.terraform` to 0o500, asserts (a) function returns nil, (b) tfvars still removed, (c) tfstate survives unchanged. Skips on root since chmod cannot produce EPERM there. **Postmortem lesson:** Loops that swallow per-iteration errors must have a test that exercises the swallow — without it, a future maintainer who "fixes" the silent-failure path breaks the recovery contract.

- **`doc:23d812fa:flux-doc-stale-line-numbers`** — done 2026-05-08 — PR #416, merge commit `ecbab03`. Tier I suggestion (readme-drift). `docs/addons/flux.md` cited `flux.go:223-231` for `DefaultSettings` and `flux.go:22-25` for the duration constants — both off by 7 lines after a refactor. Replaced line numbers with symbol references (`Flux.DefaultSettings`, `defaultControllerTimeout`, `defaultGitRepoSyncTimeout`) so anchors don't rot on every refactor. **Postmortem lesson:** Doc anchors should cite by symbol, not line number — line-number anchors are an audit-replenishment loop.

- **`doc:b3356305:readme-sbom-filename-drift`** — done 2026-05-08 — PR #417, merge commit `687a693`. Tier I minor (readme-drift). README's "Verifying a release" section claimed a single `okdctl.sbom.json` artifact, but goreleaser produces per-archive `okdctl_<version>_<os>_<arch>.sbom.json` and per-package `okdctl_<version>_<os>_<arch>.pkg.sbom.json`. Verifiers grepping for the bare name found nothing. Replaced with the actual templates mirroring the existing tar.gz line. **Postmortem lesson:** When README cites release-artifact filenames, verify against `goreleaser.yml`'s `name_template` — packaging-pipeline output is the source of truth, not the operator's mental model.

- **`smell:2be6306e:abstraction-single-caller`** — done 2026-05-08 — PR #418, merge commit `8d17082`. Tier I suggestion (helper-package-no-value). `addon.IsRegistered` in `internal/addon/registry.go:86-92` had no callers but was symmetric with Get/Names/All. Per MEMORY.md §scaffolding, doc-only PR: extended the doc comment to name the future `okdctl addon validate` verb and wizard pre-checks as the call sites the symmetric API exists for. **Postmortem lesson:** When verifying scaffolding intent under MEMORY.md §scaffolding, the doc comment must name a concrete future caller path — "kept for future use" without naming the verb invites the next audit to re-flag the same symbol.

- **`smell:dd75bdeb:multi-bool-not-exclusive`** — done 2026-05-08 — PR #419, merge commit `9514497`. Tier I suggestion (bool-should-be-enum). `postInstallContext` in `internal/distribution/okd/postinstall/context.go:1-10` carries three orthogonal stage-completion bools (`KubeVIPVerified`, `BootstrapCleaned`, `DNSDeployed`). The audit flagged the "multi-bool" shape as enum-shaped, but they are genuinely independent (multiple may be true simultaneously). Doc-only fix: added a one-line invariant naming them as parallel-by-design and forbidding a future fold into a single phase enum (which would lose parallel-progress reporting in the deploy summary). **Postmortem lesson:** Multi-bool flag groups need an explicit "do not collapse" doc when they're orthogonal-by-design — without it, the next "clean up to enum" refactor destroys the parallel-state model.

- **`api:859eea6f:export-no-caller-scaffolding`** — done 2026-05-08 — PR #421, merge commit `5ba4913`. Tier I suggestion (exported-surface). `phase.ParseNodeRole` in `internal/distribution/okd/phase/noderole.go:22-29` is the symmetric counterpart to `NodeRole.String()` with no current caller. Doc comment names it as scaffolding for upcoming JSON-deserialization paths (status output, terraform output, persisted state). Per MEMORY.md §scaffolding: do not delete; six-month tripwire — if no JSON-deserialization caller lands by 2026-11-08, demote to test-only. **Postmortem lesson:** Symmetric String/Parse pairs are scaffolding worth preserving — but the doc must include a tripwire date so audits stop accumulating "still no caller" annotations.

- **`api:0139cb3f:export-no-caller-scaffolding`** — done 2026-05-08 — PR #422 closed (no merge), covered by PR #405's WithRecorder doc comment. Tier I suggestion (exported-surface). `phase.WithRecorder` in `internal/distribution/okd/phase/paths.go:150-152` was already documented by PR #405 (`api:c287d5c0:public-fields-bypass-options`, archived 2026-05-06) — that earlier PR named `api:beabab0c` as the migration owner that will thread Recorder/Reporter through `phase.New`. PR #422's rebase-on-develop produced an empty diff against the existing comment; closed as redundant rather than re-asserting the same scaffolding intent. **Postmortem lesson:** When two roadmap items cover the same symbol, the second item's PR should rebase early and check for emptiness — a no-op rebase is the canonical signal to close-as-redundant rather than synthesise filler text.

- **`api:7b2829bb:executor-with-inherited-env-no-callers`** — done 2026-05-08 — PR #423, merge commit `19c7b96`. Tier I suggestion (exported-surface). `executor.WithInheritedEnv` in `internal/executor/executor.go:69-76` had no production caller (only test files exercise it) but exists as the documented escape hatch for tools consuming non-allowlisted variables. Doc-only fix: appended a single sentence naming WithInheritedEnv and WithEnv as the symmetric inherit-vs-filter option pair. Round-1 reviewer correctly rejected an earlier draft that included "currently no production caller exists" — that wording dates poorly and the reviewer caught it before merge. **Postmortem lesson:** Scaffolding-intent doc comments must avoid maintenance-status claims ("currently no caller") that age — name the function's role in the option pair instead, which survives the eventual first caller.

- **`api:a55b4592:loader-stateless-struct`** — done 2026-05-08 — PR #424, merge commit `af8ddc5`. Tier I suggestion (exported-surface). `config.Loader` in `internal/config/loader.go:13-64` is a stateless struct (zero fields). Six call sites do `config.NewLoader(); loader.LoadFile(p)` — the no-state equivalent of two package-level functions. Doc-only fix: added a non-obvious WHY comment naming a future stateful direction (caching, decryption keyring) per MEMORY.md §scaffolding so the next audit recognises intent rather than suggesting collapse to package-level functions. **Postmortem lesson:** Stateless structs surviving as constructor-call patterns are scaffolding for stateful follow-ups — a one-line comment naming the future stateful field (caching, keyring) prevents the next refactor from destroying the constructor surface.

- **`tst:4c092fce:terraform-destroy-direct-untested`** — done 2026-05-08 — PR #425, merge commit `63134c4`. Tier I suggestion (destructive-untested). `Executor.destroyDirect` in `internal/infrastructure/terraform/terraform.go:260-311` is unreachable today (every `okdctl destroy` site passes `UsePlan=true`) but retained as the canonical emergency-destroy path (no plan, immediate apply). Doc-only fix: added a comment naming the scaffolding intent and the argv-shape invariant the function locks (parallelism handling, target injection). The roadmap asked for an argv-shape lock test; deferred to a future PR to keep this scoped to verify-intent. **Postmortem lesson:** When an unused public function is genuinely scaffolding (clear future-caller story), the doc comment is sufficient — but the deferred test should be filed as its own roadmap item rather than left implicit.

- **`obs:1e8ffb91:degraded-operator-loop-could-aggregate`** — done 2026-05-08 — PR #426, merge commit `627f302`. Tier I minor (log-once). `verify.go` in `internal/distribution/okd/postinstall/verify.go:131-137` emitted one Warn per degraded operator plus a summary Warn — on a 30-operator cluster that is 31 Warn lines per check tick. Replaced with a single structured warn carrying `count` and `names` attrs; alert pipelines now group by event identity, TTY readers see one line, JSON pipelines can `jq '.names'` directly. **Postmortem lesson:** Per-element Warn loops are alert-pipeline noise — a single structured Warn carrying the slice attribute is always cheaper for both human readers and downstream consumers.

- **`sec:29293401:predictable-tmp-pid`** — done 2026-05-08 — PR #427, merge commit `4279950`. Tier I minor (file-toctou). `internal/distribution/okd/setup/haproxy.go:101-105` built the temp-config path from `os.TempDir()` + `os.Getpid()` — predictable and bypassing the canonical `system.WriteTempFile` per CLAUDE.md §architecture-notes. The /tmp sticky bit + sudo re-exec made the predictable name benign in practice, but the inconsistency invited a future copy-paste regression. Replaced with `system.WriteTempFile("haproxy-*.cfg", 0o644, writer)`. **Postmortem lesson:** Predictable temp names that are "currently safe" still erode the canonical-helper invariant — every `os.TempDir()` + `os.Getpid()` site should be migrated even when the immediate threat model says no exploit exists.

- **`iac:90de5406:hcl-prod-no-validation`** — done 2026-05-08 — PR #428, merge commit `7e29a55`. Tier I suggestion (hcl-credential-hygiene). `infrastructure/terraform/environments/production/variables.tf` redeclared every module variable but dropped the `validation` blocks (cpu_cores 2-32, memory_mb >= 8192, master_count odd 1-5, vmid_base 100-9000). Two specs that could drift, with the env spec silently weaker. Implemented option (a): deleted 21 redundant passthrough vars whose defaults matched the module exactly, kept only env-specific overrides (target_node, isos, cluster_name, vmid_base, memory_mb, bootstrap_memory_mb, worker_cpu/memory, vm_tags) including the four `null`-overrides that intentionally inherit module defaults. Module is now the single source of truth for both defaults and validation. **Postmortem lesson:** Env-level passthrough vars are a duplicated-spec liability — when the env's only role is to override a subset, drop the rest and let the module own validation.

- **`con:98723e5d:setupclusteraccess-ctx-ignored`** — done 2026-05-08 — PR #429, merge commit `60fa195`. Tier I suggestion (ctx-ignored). `Phase.SetupClusterAccess` in `internal/distribution/okd/install/flux.go:50-91` accepted ctx but discarded it via `_ context.Context` — a Ctrl-C between MonitorInstallation returning and the kubeconfig install would not interrupt bashrc/copyfile/chown work. Renamed param to `ctx context.Context` and added `if err := ctx.Err(); err != nil { return err }` between the four logical sub-steps, matching the pattern in `SetupKubeconfig` at `flux.go:166-169`. **Postmortem lesson:** A `ctx` param discarded via `_` is always a regression vector — the function's caller may rely on cancellation propagation that the body silently breaks; either honour ctx or remove the param.

- **`ux:aa84670c:no-second-ctrl-c-escape`** — done 2026-05-08 — PR #430, merge commit `d6ca9fe`. Tier I minor (signals). `internal/cli/root.go:86-109` read exactly one signal then returned — a second SIGINT during graceful shutdown (terraform destroy hung on Proxmox API drop, must-gather mid-archive) had no escape, forcing `kill -9`. Replaced single-receive with a `signalLoop` helper: first signal stores+cancels (existing behaviour) and prints "shutdown in progress; press ctrl-c again to force quit"; subsequent signals call `os.Exit(130)` directly bypassing deferred logFileCloser.Close(). Tested via injectable `exit func(int)` so the second-signal path is asserted without actually exiting the test process. **Postmortem lesson:** Two-strikes signal handling is a CLI ergonomics standard (kubectl, docker, gh) — the bypassed-cleanup tradeoff is acceptable because the user has explicitly asked for a hard kill.

- **`smell:d6b325cb:enum-ad-hoc`** — done 2026-05-08 — PR #431, merge commit `371ba6f`. Tier I minor (magic-strings). `VMState` was defined twice with the same five string literals — once in `internal/infrastructure/proxmox/types.go:47-58` and once in `internal/distribution/okd/phase/vmstate.go`. The phase/vmstate.go header comment had already designated phase as the canonical owner ("so iso_cleanup and proxmox can share a single source of truth without an import cycle") but proxmox had not adopted it. Implemented option (b): removed the proxmox copy, retyped `VMStatus.Status` to `phase.VMState`, updated three callers in proxmox.go to `phase.StateRunning`. No import cycle (phase has no proxmox dep). Round-1 lint failed on a stray trailing newline; round-2 fixed and merged. **Postmortem lesson:** When a header comment names canonical ownership but callers haven't adopted, the divergence persists silently — the audit-flag-then-migrate cycle is the only forcing function.

- **`sec:f55b9c27:envfile-once-cross-config`** — done 2026-05-08 — PR #432, merge commit `b5fe27e`. Tier I suggestion (credentials). `LoadEnvFile` in `internal/credentials/envfile.go:116-125` is sync.Once-guarded — once loaded with one path, subsequent calls with a different path return an error. The deploy at `cli/helpers.go:48` swallowed that error to a `tui.Warn` log line; deploy then proceeded silently and only failed later when GetProxmoxCredentials returned `IsValid()==false`. A misconfigured deploy could therefore proceed without the env credentials it expected. Changed `handleCredentials` to return `(creds, err)`; updated three callers (deploy.go runDeploy, runDeployDryRun, runFullDeployment; destroy.go runDestroy + runDestroyDryRun) to fail-fast. Tightened `LoadEnvFile` docstring to state the contract: "A non-nil return always means credentials were not loaded. Callers must treat any error as fatal." **Postmortem lesson:** sync.Once-guarded loaders that return errors on cross-call mismatch must propagate the error to the caller, not log-and-continue — the caller's downstream "credentials missing?" check fires later, with worse diagnostics.

- **`state:08c49fc4:update-ingress-no-dryrun-state-probe`** — done 2026-05-08 — PR #433, merge commit `4f7a0bf`. Tier I suggestion (destroy-safety). `runUpdateIngressDryRun` in `internal/cli/update_ingress.go:48-61` printed the would-do list without consulting on-disk dnsmasq state or haproxy service state — after a successful cutover, re-running with `--dry-run` claimed it would re-deploy production DNS even though the live run would be a no-op. Wired `dns.IsBootstrapDNS(cfg)` and `system.IsServiceActive(ctx, "haproxy")` probes; each affected step now labels itself `(no-op: ...)` when the live run would not mutate. Function signature gained ctx, err propagation includes the dnsmasq probe failure. **Postmortem lesson:** Dry-run preview text that doesn't probe live state is worse than no preview — operators trust it as an oracle, then act on a lie.

- **`state:15ba17da:destroy-no-only-scope`** — done 2026-05-08 — PR #434, merge commit `198a436`. Tier I minor (destroy-safety). `okdctl destroy` was all-or-nothing at the cluster level. The CLI accepted `--target=module.okd_cluster.proxmox_virtual_environment_vm.{...}[N]` (validated against `destroyTargetRE`) but offered no higher-level scoping like "destroy only workers" — operators wanting to recycle a worker pool without touching masters had to hand-craft N --target args. Added `--only={vms,workers,masters,bootstrap}` that expands into the canonical destroyTargetRE-matching target list using `cfg.Topology.{ControlPlane,Workers}.Count`. `--only` and `--target` are mutually exclusive via cobra `MarkFlagsMutuallyExclusive`; the existing `--confirm-cluster` guard fires on either flag. Tests cover all four values, invalid value, zero-count edge. Required a follow-up `make docs` regeneration commit for the new flag. **Postmortem lesson:** When adding a cobra flag, `make docs` is part of the same commit — the docs-go CI check catches drift on every PR but the local pre-commit doesn't, so plan it in.

- **`api:0fc0041d:export-no-caller-scaffolding`** — done 2026-05-08 — PR #420, merge commit `cc1293d`. Tier I suggestion (exported-surface). `internal/distribution/okd/phase/condition.go:10-35` declared the symmetric Kubernetes condition matrix (ConditionTypeReady/Available/Progressing/Degraded × ConditionStatusTrue/False/Unknown plus NodeStatusReady/NotReady/Unknown), but only ConditionTypeReady + ConditionStatusTrue had in-scope callers (cli/status.go, postinstall/verify.go). Per MEMORY.md §scaffolding the matrix is preserved as a unit so a future operator-degraded status verb can surface non-Ready conditions without re-introducing the missing constants under different names. Doc-only fix: a single block-level comment above the const declaration naming the matrix shape (4×3 + 3 node states) and the future status-verb caller. Round-1 reviewer accepted; CI runner backlog held the merge for ~30 min. **Postmortem lesson:** Scaffolding-shaped enum matrices need a single block-level comment that names every dimension — partial-trim refactors are most likely when the unused half of the matrix isn't visibly tied to the used half.

- **`sec:e076e43c:install-sh-insecure-flag-trust`** — done 2026-05-08 — PR #439, merge commit `f49ca67`. Tier I minor (tls-network). `scripts/install.sh:60-64` honoured `INSECURE=1` to skip cosign verification regardless of whether cosign was installed; with no cosign present, sha256 became self-referential against an unverified `SHA256SUMS`, leaving zero chain-of-trust on the curl|sh distribution path. Implemented option (b): the INSECURE block now `die`s when `COSIGN_CMD` is empty, preserving the emergency escape hatch when cosign is installed but refusing to install when no alternative trust anchor exists. Header docstring updated to reflect the new contract. **Postmortem lesson:** Trust-boundary downloads should refuse to disable verification unless an alternative anchor is verifiable — never just warn.

- **`sec:e3782ee7:atomicwrite-create-then-chmod`** — done 2026-05-08 — PR #441, merge commit `e83e252`. Tier I suggestion (file-toctou). `AtomicWrite` in `internal/system/fs.go:196-247` used `os.CreateTemp` (mode 0o600 always) then `os.Chmod` to widen to the caller-requested perm, leaving a brief window where the perm differed from intent. Replaced with the existing `openTempFile` helper which applies mode at open time via `O_RDWR|O_CREATE|O_EXCL|perm` in a single syscall. Round-1 reviewer caught a stale docstring still referencing the removed chmod step; round-2 fix updated it. **Postmortem lesson:** When swapping a multi-step operation for a single syscall, audit every doc comment that names the steps — stale narration is a reviewer-magnet bug.

- **`sec:0d318f5c:logfile-mode-fixed`** — done 2026-05-08 — PR #442, merge commit `c95a806`. Tier I suggestion (credentials). `openLogFile` in `internal/cli/logging.go:24-33` already had Lstat + O_NOFOLLOW + O_APPEND + 0o600, but the privilege contract (`--log-file` is operator-chosen and opened as root post-sudo-re-exec) was undocumented — readers couldn't tell whether the existing guard was sufficient or whether path-restriction hardening was pending. Doc-only fix: extended the doc-comment to name the contract and identify O_APPEND + 0o600 as the bounded-risk guarantees. **Postmortem lesson:** When the threat model is bounded by mode/flag choices, document why those bounds suffice — readers otherwise re-relitigate the audit on every audit cycle.

- **`state:62cb8a95:helper-stale-lock-message`** — done 2026-05-08 — PR #443, merge commit `e57fe2b`. Tier I suggestion (tf-state-atomicity). `stateLockHint` in `internal/distribution/okd/destroy/helpers.go:19-30` told operators to run `terraform force-unlock <id>` but the `<id>` was a literal placeholder — operators had to read the lock file manually. Added `parseLockID` (encoding/json + os.ReadFile) that extracts the .ID field from `.terraform.tfstate.lock.info`; falls back to the placeholder on parse failure. Two new tests cover the success and corrupt-file paths. Mirrors the parse-on-best-effort pattern at `checkStateMajorVersion`. **Postmortem lesson:** Diagnostics that say "find <X> and run command" should pre-fetch <X> when it's in a documented stable shape; manual indirection is operator-hostile.

- **`state:262af6e4:cleanup-best-effort-no-resume`** — done 2026-05-08 — PR #444, merge commit `4178edb`. Tier I minor (crash-recoverability). `cleanup.Execute` in `internal/distribution/okd/cleanup/cleanup.go` was a flat switch over Kind that called subsystem helpers serially; a SIGKILL mid-run left subsequent subsystems uncleaned with no resume signal. Lifted onto `distribution.StepDef` + `BuildSteps` with per-subsystem `AlreadyDone` predicates (workdir/haproxy/dnsmasq/terraform check sentinel files; webserver/apache/packages stay ReRunSafeYes). A `cleanupTracker` mirrors destroyTracker to surface NonFatal step errors through a final summary step that calls `errors.Join`. Round-1 reviewer caught a `nilerr` lint violation in dnsmasqStep.AlreadyDone; round-2 fix propagates the error so the orchestrator logs and re-executes. **Postmortem lesson:** Best-effort cleanup pipelines naturally migrate to StepDef + AlreadyDone — but every closure that calls a fallible helper must propagate the error, not swallow it; reviewers must scan AlreadyDone closures specifically.

- **`sub:ae5b624c:no-graceful-cancel`** — done 2026-05-08 — PR #445, merge commit `09ef828`. Tier I minor (timeout-cancel). `defaultStartMonitorCmd` in `internal/distribution/okd/install/monitor.go:23-45` used `osExec.CommandContext` (defaults to SIGKILL on cancel) plus a hand-rolled `sync.OnceFunc` kill closure and a 30 s `reapTimer`; on install timeout, openshift-install was SIGKILL'd before flushing diagnostics. Replaced with Go 1.20 `cmd.Cancel = SIGTERM` + `cmd.WaitDelay = 30s`, which the runtime coordinates natively. Dropped the kill closure, the reapTimer block, and the now-stale `sync` import. CLAUDE.md §Concurrency canonical-pattern entry updated to the new shape. Two synctest tests collapsed to context-cancellation assertions. Sibling items (con:ae5b624c, err:ae5b624c, obs:ae5b624c) intentionally left scoped out. **Postmortem lesson:** Hand-rolled cancel-then-wait-then-kill is always reaching for cmd.Cancel + cmd.WaitDelay; when the stdlib offers the pattern, swap and update the CLAUDE.md anchor in the same PR.

- **`ux:fd2125dd:addon-uninstall-stdout-msg`** — done 2026-05-08 — PR #446, merge commit `20bcd1a`. Tier I suggestion (streams). `runAddonUninstall` in `internal/cli/addon.go:200` wrote `addon X uninstalled` to stdout via `fmt.Fprintf(cmd.OutOrStdout())`; siblings `destroy`, `cleanup`, and `update-ingress` route post-action confirmation through `tui.Info` (stderr). One-line replacement aligns the CLI surface; `tui.Info(fmt.Sprintf(...))` matches the exact sibling shape rather than reaching for structured fields (which is the broader `obs:6424733c:fmt-sprintf-message-pattern` item's scope). **Postmortem lesson:** "Match sibling shape" beats "switch to structured" when one item touches one site — the broader migration belongs to the umbrella audit-observability item, not to the alignment fix.

- **`smell:c4182b1c:abstraction-single-caller`** — done 2026-05-08 — PR #447, merge commit `bf7b64e`. Tier I suggestion (helper-package-no-value). `PhaseContext[T]` in `internal/distribution/context.go:1-33` had only one consumer (postinstall) and an RWMutex protecting against concurrency that doesn't exist today. Per MEMORY.md §scaffolding the type stays; doc-only fix extends the type doc to name the symmetric-API intent (resume-checkpoint work in `state:4f69fc9d`/`state:262af6e4`) and justifies the RWMutex for the future parallel-step case. **Postmortem lesson:** Scaffolding-intent doc comments must cite specific roadmap items as future consumers — "kept for future use" without naming the verb invites the next audit to re-flag.

- **`doc:8f46b665:phases-stepdef-missing-fields`** — done 2026-05-08 — PR #449, merge commit `e736762`. Tier F major (readme-drift). `docs/architecture/phases.md:40-51` documented StepDef without `ReRunSafe` (mandatory; BuildSteps panics on `ReRunSafeUnset`), `AlreadyDone`, or `OnStart` — readers cloning the doc shape would hit `BuildSteps: must declare ReRunSafe` panic at first run. Replaced the doc's StepDef code block with a faithful mirror of `internal/distribution/step.go:211-223` (canonical field order, three new fields, sparse inline comments) plus a paragraph naming the panic invariant. **Postmortem lesson:** When a struct gains a mandatory zero-value-rejecting field, the architecture doc is part of the same change — drift is silent until a contributor follows the doc verbatim.

- **`doc:8f46b665:phases-basephase-missing-recorder`** — done 2026-05-08 — PR #449, merge commit `e736762`. Tier F minor (readme-drift). Same file as `phases-stepdef-missing-fields`; `docs/architecture/phases.md:79-85` documented BasePhase without the `Recorder` field (`distribution.MetricsRecorder`), which is load-bearing for the deploymetrics path. Bundled into the same PR; the new BasePhase block adds Recorder with a one-line comment naming the per-step + overall observation-sink role and the nil → nopMetricsRecorder default via WithRecorder. **Postmortem lesson:** Doc-drift items targeting the same file should be bundled — separate PRs would have churned the same code block twice.

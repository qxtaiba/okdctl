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

- **`sec:fde34e0c:k8sclient-env-direct-write`** — done 2026-05-08 — PR #440, merge commit `20a8406`. Tier I suggestion (credentials). `NewK8sClient` in `internal/cluster/k8s.go:71-81` set `cmdRunner.Env = []string{"KUBECONFIG=..."}` directly after `executor.New(WithLogger)`, bypassing the canonical `executor.WithEnv` option-funnel. Today's runtime behaviour is identical (KUBECONFIG already passes the `KUBE` prefix in `DefaultEnvAllowlist`), but the post-construction field write would compose strangely with any future option that also touches `Env`. Replaced with `executor.New(executor.WithLogger(...), executor.WithEnv([]string{...}))` built from a conditional options slice. CI required two cancel-and-rerun cycles before test-go ran on a clean runner — multiple stuck/hanging GitHub-hosted runners on the same workflow ID kept timing out at the test step. **Postmortem lesson:** When CI test-go has been "pending" for 30+ min, the runner is stuck — `gh run cancel` then `gh run rerun --failed` fixes it without rebasing, and the cycle may need to repeat.

- **`sec:0f076161:cred-no-zeroize`** — done 2026-05-08 — PR #450, merge commit `fcd4b52`. Tier H minor (credentials). `runDestroyDryRun` in `internal/cli/destroy.go:282-285` appended `creds.Env()` to `terraform.WithEnv` and constructed a `terraform.Executor` directly — no Provisioner, so the existing `Provisioner.ZeroizeEnv()` (sec:6424733c) couldn't reach it. The credential strings in `tf.exec.Env` outlived the deferred `creds.Zeroize()` because Go strings are immutable copies of the source `[]byte`. Added `terraform.Executor.ZeroizeEnv()` mirroring the deploy-side helper exactly (same `PROXMOX_VE_PASSWORD`/`PROXMOX_VE_API_TOKEN` keys, same blank-then-`clear()`-then-nil sequence, same nil guard); `defer tf.ZeroizeEnv()` in `runDestroyDryRun` bounds credential lifetime on both error and success paths. **Postmortem lesson:** Symmetric helpers belong on whichever component holds the data — when the deploy path uses Provisioner and the dry-run path uses raw Executor, both need the same zeroize method; a single helper on the credentials package would have forced callers to remember to call it, while a method on the Env-holder makes the responsibility self-evident.

- **`iac:e076e43c:sh-curl-bypass-wrapper`** — done 2026-05-08 — PR #451, merge commit `43b9f00`. Tier I minor (install-sh-fail-closed). `scripts/install.sh:91` inlined `curl -sSfL --proto '=https' --tlsv1.2 --connect-timeout 10 --max-time 30` at the latest-release resolution call instead of using the centralized `curl_safe` wrapper, silently dropping `--retry 2 --retry-connrefused` at exactly the call site most likely to hit transient GitHub API failures. Replaced with `curl_safe -sSfL --max-time 30`; the `--max-time 30` still overrides the wrapper's `--max-time 120` (curl uses last value). Bundled with `iac:e076e43c:sh-tar-no-confinement` since both touch install.sh and PR #439's hunks (lines 12-19, 58-67, 117-122) were disjoint from these regions (lines 91, 162-168). **Postmortem lesson:** When a wrapper centralizes hardened defaults, every call site must use it or document why not — partial-flag-inlining is the worst of both worlds, claiming the appearance of hardening while shedding the retry semantics.

- **`iac:e076e43c:sh-tar-no-confinement`** — done 2026-05-08 — PR #451, merge commit `43b9f00`. Tier I suggestion (install-sh-integrity). `scripts/install.sh:162` extracted the release tarball with `--no-same-owner --no-same-permissions` but lacked `--no-overwrite-dir` and ran without inspecting archive contents. The cosign + sha256 chain is the primary guard, but a malformed or tampered tarball with `..`-prefixed entries could in theory escape `$TMP` on GNU tar versions that don't normalize parent traversals. Added a `tar -tzf "$ARCHIVE_NAME" | grep -qE '^(\.\.|/)' && die` guard before the extraction (under `set -e` the LHS of `&&` is in tested context, so a no-match `grep -qE` doesn't abort the script — verified via shellcheck and bash semantics) plus `--no-overwrite-dir` on the extraction line. **Postmortem lesson:** For curl|sh distribution paths, defense-in-depth on archive extraction costs ~3 lines and catches malformed tarballs that slip past the integrity chain; the audit's "Goreleaser tarballs are flat today" note is exactly the kind of premise that erodes silently when a future Goreleaser bump or release-process change lands.

- **`state:6424733c:projectroot-marker-restrictive`** — done 2026-05-08 — PR #452, merge commit `0c32a4b`. Tier H minor (crash-recoverability). `resolveProjectRootOrDie` in `internal/cli/helpers.go:110-134` rejected any directory whose `Base(cfgFile)` was missing — defense-in-depth against symlink-redirected sudo cleanups, but a partial-failed deploy that wipes `okdctl.yaml` (cleanup.WorkDirectory removes the cluster-config dir first in Full kind) left destroy/cleanup unable to recognise its own project despite live VMs and tfstate. Added `hasProjectMarker(root)` checking `okdctl.yaml`, `okdctl.env`, and `infrastructure/terraform/environments/*/terraform.tfstate` via stat + filepath.Glob; any present marker accepts the root. The symlink-redirect defence is preserved because all three files are exclusively written by okdctl inside a project. Four-case `helpers_test.go` covers each marker plus the empty-dir rejection. **Postmortem lesson:** Defensive single-marker checks become operator-hostile after partial failures destroy the marker — broaden the marker set to any okdctl-authored file inside the project root; the security argument survives because the candidate set is still okdctl-only.

- **`state:48688e63:provision-no-output-readback`** — done 2026-05-08 — PR #453, merge commit `8cffe66`. Tier H minor (proxmox-api-idempotency). `retrieveProvisionResult` in `internal/infrastructure/proxmox/proxmox.go:237-299` derived every VM IP from `cfg.Networking.StaticIP.Start` by arithmetic — no consultation of terraform state, so a network-ordering bug or a parallel-apply race producing a different IP went undetected and downstream `WaitForBootstrap` polled a phantom IP for 30 minutes with no diagnostic. Added `terraform.Executor.Output(ctx)` returning the parsed `terraform output -json` map; `Provider.checkTerraformOutputs` cross-checks the `vm_ids` envelope against `cfg.Topology` counts and Warns on missing key, unparseable payload, or count drift. Full IP comparison was deferred via in-code doc comment because `outputs.tf` today exposes only `vm_ids` (not `control_plane_ips`/`worker_ips` as the audit-evidence text claimed); HCL changes for IP outputs left as a follow-up. **Postmortem lesson:** When the audit-evidence references outputs that don't exist, do not synthesise them — implement the diluted version, document the deferral inline next to the function, and surface as a follow-up roadmap item rather than silently shipping a half-implemented spec.

- **`state:48688e63:proxmox-no-eventual-consistency`** — done 2026-05-08 — PR #453, merge commit `8cffe66`. Tier H suggestion (proxmox-api-idempotency). Same file as `provision-no-output-readback`; `Provider.Provision` returned from `terraform.Apply` and immediately logged "vm provisioned" per VM with no readiness probe against Proxmox — VMs may exist in tfstate before the API enumerates them. Added `Provider.probeVMEnumeration(ctx, cfg) bool` that pvesh-GETs `/nodes/<node>/qemu` over SSH (gated on a new `WithSSHExec` option), parses the VMID list, and returns `false` only when the probe ran and parsed but `vmidBase` was absent. Provision gates the per-VM "vm provisioned" Info loop on the bool, so operators no longer see contradictory "not yet enumerable" + "vm provisioned" lines back-to-back. The `WithSSHExec` option ships dormant — no caller wires it today — accepted under MEMORY.md `feedback_scaffolding` with a follow-up to wire from `install.DeployInfrastructure`. Reviewer round 1 caught the contradictory-log bug and three silent-error swallows; round 2 fixed all four. **Postmortem lesson:** "Instead of" language in the Fix is a hard constraint, not a suggestion — gate the existing log on the new probe's outcome, do not just add the new log next to the old one and call it done.

- **`iac:18a795d5:hcl-tls-skip-doc-no-warning`** — done 2026-05-08 — PR #454, merge commit `4a6377e`. Tier H suggestion (hcl-credential-hygiene). `infrastructure/terraform/modules/proxmox-okd/main.tf:9`, `modules/proxmox-okd/variables.tf:6`, and `environments/production/variables.tf:6` documented `PROXMOX_VE_INSECURE` without flagging it as a development-only knob — three duplicated comment blocks normalised TLS-disable as a routine operator action. Appended a one-line warning to each (DEV ONLY: disables TLS verification — never set in prod; use a CA-signed cert or add the proxmox CA to your trust store). The append (rather than replace) leaves the original variable description intact and matches the Fix wording verbatim; the duplicate `# - PROXMOX_VE_INSECURE` lines are intentional per spec. **Postmortem lesson:** When a security-relevant env var is documented in three places, the warning belongs in all three — partial coverage trains operators to ignore the warning when they encounter the unwarned copy first.

- **`err:08c49fc4:tui-warn-stringifies-err`** — done 2026-05-08 — PR #455, merge commit `c206be1`. Tier I suggestion (redaction-in-error). `tui.Warn("skipping HostNetwork conversion: " + err.Error())` at `internal/cli/update_ingress.go:93` concatenated `err.Error()` into the message text, bypassing `logutil.RedactHandler` (which only inspects structured slog attrs and `*url.URL`/`Redacted()` values). Today's err chain carries only `ctx.Err` and a TTY-config error so the leak is theoretical, but the surface is exposed to refactor drift. Replaced with `tui.Warn("skipping HostNetwork conversion", tui.LF("err", err))` — the structured-attr form the redact handler can scrub. Bundled with two sibling items in the same file via one PR. **Postmortem lesson:** `tui.X("...: " + err.Error())` is the canonical RedactHandler bypass — every site is a one-line fix to `tui.LF("err", err)`; reviewers should mechanically grep for the antipattern.

- **`obs:08c49fc4:hostnetwork-ic-message-text`** — done 2026-05-08 — PR #455, merge commit `c206be1`. Tier I minor (field-stability). Same line as `err:08c49fc4` (`internal/cli/update_ingress.go:93`); two roadmap items pointed at one fix because audit-errors and audit-observability flagged the same string-melt independently. The single edit satisfies both. **Postmortem lesson:** When two audit dimensions surface the same line, the dependent-batch worktree is the right shape — one fix, one PR, both items archived together; avoid splitting into parallel PRs that would race on the same commit.

- **`ux:08c49fc4:dryrun-mixed-streams`** — done 2026-05-08 — PR #455, merge commit `c206be1`. Tier I minor (streams). `runUpdateIngressDryRun` at `internal/cli/update_ingress.go:50-79` mixed streams: opening/closing lines via `tui.Info` (stderr) but bulleted "would: ..." previews via `fmt.Println` (stdout). Sibling dry-run helpers (`runCleanupDryRun`, `runDeployDryRun`) use `tui.Info` exclusively. Replaced six `fmt.Println("  would: ...")` calls with `tui.Info("would: ...")` — every dry-run line now lands on stderr, `> log 2>&1` sees one ordered stream, `--quiet` suppresses preview consistently. Also corrected an unrelated stray `)` typo in one of the would-lines that the diff happened to touch. **Postmortem lesson:** Dry-run helpers benefit from a static checklist — every `Println` inside a `*DryRun` function should be `tui.Info` for stream consistency; fold this into `audit-cli-ux` for the next sweep.

- **`api:0139cb3f:bindirordefault-symmetric-helper`** — done 2026-05-08 — PR #456, merge commit `2d4cac0`. Tier I suggestion (exported-surface). `phase.BinDirOrDefault` at `internal/distribution/okd/phase/paths.go:73-79` is part of a three-function bin-dir-resolution surface (`PreflightBinDir` + `ResolveBinDir` + `BinDirOrDefault`); call sites in setup/cleanup invoke it as defense-in-depth on a field already populated by `ResolveBinDir` at construction. Per MEMORY.md §scaffolding the symmetric trio stays. Doc-only fix: a six-line `Scaffolding:` comment above the function names the trio explicitly, identifies the defense-in-depth call-site usage, and records that the explicit fallback documents zero-value safety auditable at each call site. CI required a stuck-runner cancel-and-rerun cycle before test-go ran on a clean runner. **Postmortem lesson:** Symmetric N-function surfaces need a doc comment naming all N members on whichever function is the smallest (audit pressure first lands there) — readers tracing the trio shouldn't have to grep every sibling to see the pattern.

- **`obs:ae5b624c:per-tick-csr-warn-already-deduped`** — done 2026-05-08 — PR #457, merge commit `d0555b1`. Tier I suggestion (log-once). `monitor.go:122-161` already implements the canonical poll-loop log-once idiom (first occurrence at Warn + sets `lastCSRWarnMsg`, identical repeats demote to Debug, clean tick resets the gate). Flagged as a positive counter-example so future audits don't accidentally regress it. Doc-only fix: appended a fourth bullet to the §Concurrency canonical-patterns list in CLAUDE.md naming the idiom and pointing at the file/line range. Bundled with `dep:33ef32bf:yaml-triple-engines` in one CLAUDE.md PR. **Postmortem lesson:** Positive-baseline audit findings still need a CLAUDE.md anchor — without it, the next audit re-discovers the pattern and re-flags it as "should we add log-once?" instead of recognising it as already-canonical.

- **`dep:33ef32bf:yaml-triple-engines`** — done 2026-05-08 — PR #457, merge commit `d0555b1`. Tier I suggestion (duplicate-engine). Three YAML engines remain in the dep tree: `sigs.k8s.io/yaml v1.6.0` (direct, required by k8s.io/api) + `go.yaml.in/yaml/v{2,3}` (transitive, pinned by k8s.io/apimachinery) — down from four after `gopkg.in/yaml.v3` dropped from `go.mod` require. Doc-only fix: appended a §Dependencies tripwire bullet in CLAUDE.md recording the count with the delta-from-four annotation, instructing reviewers not to add a fourth without recorded justification. **Postmortem lesson:** Tripwire bullets in CLAUDE.md must include the delta-from-prior-count; "three YAML engines" by itself doesn't tell a future auditor whether the trend is up or down, but "down from four" makes the next audit cheap.

- **`mod:1e8ffb91:use-slices-containsfunc`** — done 2026-05-08 — PR #458, merge commit `75ca318`. Tier I suggestion (slices-maps). `parseOperatorDegradation` at `internal/distribution/okd/postinstall/verify.go:50-57` used a nested for-range with explicit break — exactly `slices.ContainsFunc`. Bundled with `mod:1e8ffb91:use-slices-containsfunc-readiness` so a single `slices` import covered both rewrites. The named-type extraction (lifting the anonymous Conditions struct into `verifyCondition`) was needed because Go forbids anonymous-struct-literal expressions as closure parameter types in idiomatic form. Reviewer round 1 flagged duplicate `clusterOperatorCondition` + `nodeCondition` types; round 2 collapsed to one shared `verifyCondition` mirroring the `statusCondition` pattern in `internal/cli/status.go`. **Postmortem lesson:** When two list types share an identical condition struct, the modernization is also an opportunity to collapse the duplication — extract one named type, not two; the sibling file's pattern is the canonical name.

- **`mod:1e8ffb91:use-slices-containsfunc-readiness`** — done 2026-05-08 — PR #458, merge commit `75ca318`. Tier I suggestion (slices-maps). Sibling rewrite at `internal/distribution/okd/postinstall/verify.go:89-97` (parseNodeReadiness) using the same shared `verifyCondition` and `slices.ContainsFunc` pattern. Bundled into one PR with `use-slices-containsfunc` so the import-add was paid once. **Postmortem lesson:** Paired-rewrite items targeting the same file are always one PR — separating them would have forced two import diffs and two reviewer cycles.

- **`doc:b3356305:readme-production-yaml-worker-drift`** — done 2026-05-08 — PR #459, merge commit `06d73dc`. Tier I minor (readme-drift). `README.md:116` claimed `production.yaml — 3 control-plane, 3 worker layout` but `configs/examples/production.yaml:30` has `workers.count: 5`. One-line README fix to "5 worker layout" (the YAML is the intended source of truth). **Postmortem lesson:** README example summaries are statically derivable from the config files — a future CI check that diffs README claims against actual `count` fields would catch this drift class permanently; until then, every count change to an example config needs a paired README update.

- **`state:15ba17da:nofatal-tracker-sync-todo`** — done 2026-05-08 — PR #460, merge commit `db645de`. Tier I suggestion (phase-idempotency). `destroyTracker` in `internal/distribution/okd/destroy/steps.go:30-54` buffered failure labels behind a comment hedging "Safe without a mutex because Orchestrator.Run iterates steps serially; add sync.Mutex if step parallelism ever lands" — a bare TODO in violation of CLAUDE.md §code-comments. Branch (a) chosen over branch (b): embedded `sync.RWMutex` (5 LOC), locked the two write sites and snapshot-copied at the two read sites. The hedging comment is gone; a one-line invariant `// guards failures and skipped` replaces it. `go test -race` clean on `internal/distribution/okd/destroy/...`. **Postmortem lesson:** Hedge comments naming a future condition ("if X ever lands") are bare TODOs by another name — fix the underlying concern in the same PR rather than carrying the comment as a future-author handoff that never gets handed off.

- **`con:ae5b624c:reap-reimplements-cmd-cancel`** — done 2026-05-08 — no new PR (refactor already on develop via prior commit). Tier I minor (goroutine-lifetime). `internal/distribution/okd/install/monitor.go::defaultStartMonitorCmd` already sets `cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }` (L32) and `cmd.WaitDelay = 30 * time.Second` (L33); the hand-rolled `sync.OnceFunc(Process.Kill)` closure and 30 s `reapTimer` race are absent — `sync` is not imported. CLAUDE.md §Concurrency already names this file as the canonical Go 1.20+ `cmd.Cancel`/`cmd.WaitDelay` example, and that assertion now matches the code. The remaining `case <-ctx.Done()` branch in `MonitorInstallation` is intentional: stub-injected tests (`TestMonitorInstallation_ReapTimeout`, `TestMonitorInstallation_CtxCancelReapsGracefully`) never close their `installDone` channel, so the function must bail via `ctx.Done` for those tests not to hang. **Postmortem lesson:** When a roadmap item is captured against an aspirational doc claim that has since become real, archive-sweep the item instead of opening an empty PR — check the file before claiming the worktree.

- **`sec:6424733c:cred-in-log`** — done 2026-05-08 — PR #462, merge commit `cc69829`. Tier H suggestion (redaction). `internal/cli/helpers.go:56-63` logged `tui.Info(fmt.Sprintf("using credentials from %s", creds.Source))` — the `fmt.Sprintf` interpolation pre-renders the message before `logutil.RedactHandler` sees the slog record, so a future field that interpolates a credential into the message text would silently leak past the handler. Codified the umbrella ban in CLAUDE.md §Credentials and secrets: explicit prohibition on `tui.Info/Warn/Error(fmt.Sprintf(...))`, `p.Log.X(fmt.Sprintf(...))`, and the structured-attrs alternative. Per-site cleanup is the deferred companion item `obs:6424733c:fmt-sprintf-message-pattern` (~50 sites); landing the lint rule alongside the policy without the sweep would have failed CI on every existing site. **Postmortem lesson:** Umbrella-policy items that imply a lint rule must be paired with the per-site sweep in the same release window — codify-only is the right interim step when the sweep is too broad to bundle.

- **`state:6424733c:cancel-mid-deploy-no-state-marker`** — done 2026-05-08 — PR #463, merge commit `db7fad2`. Tier H minor (crash-recoverability). `executeFullDeployment` in `internal/cli/helpers.go:240-332` cancelled into a generic "run 'okdctl destroy' to clean up" hint at all three phase boundaries (Prepare/Install/Configure), even when terraform state was empty (cancel-during-prepare → `okdctl cleanup` would be the right move; destroy fails because the cluster-config dir is wiped first). Added a tiny JSON marker `.okdctl-deploy-state.json` written under workDir on each phase entry via `system.AtomicWrite` and read back by `runDestroy`; cancellation hints now distinguish "cancelled during prepare — terraform state empty; run 'okdctl cleanup'" from "install/configure — terraform populated; run 'okdctl destroy'". Marker write failures are non-fatal warns; marker is removed on clean completion. Factored `markDeployPhase` and `announceDeployState` helpers in `deploystate.go` to keep `executeFullDeployment`/`runDestroy` under funlen caps. **Postmortem lesson:** Lint caps (funlen 65 statements / 120 lines) catch when a defensive feature padded a critical-path function — extracting the per-phase write and the destroy-side read into helpers is cleaner anyway, and the cap forces the extraction.

- **`err:ae5b624c:install-cancelled-no-typed-wrap`** — done 2026-05-08 — PR #464, merge commit `6fc427d`. Tier H suggestion (cancellation-identity). Three `fmt.Errorf("… cancelled: %w", ctx.Err())` sites in `internal/distribution/okd/install/monitor.go` (WaitForBootstrap L65, MonitorInstallation L133 and L167) used bare wraps where every other branch returned `*errtypes.ClusterError`, looking like missing typed errors. Behaviour was already correct because `cli/root.go::signalExitCode` walks the chain via `errors.Is(err, context.Canceled)` BEFORE `exitCodeFor` runs and maps SIGINT→130 / SIGTERM→143 directly. Took the lower-risk preferred path: added a 3-line WHY comment at each site naming the signalExitCode shortcut. No behaviour change; the comments document the intentional asymmetry so future readers don't "fix" it by converting to typed and breaking nothing. **Postmortem lesson:** When the audit recommendation is "either A or B, B preferred", the postmortem should record why B was picked — here, lower risk and zero behaviour change.

- **`con:6424733c:metrics-server-no-basecontext`** — done 2026-05-08 — PR #465, merge commit `8f79b55`. Tier H minor (goroutine-lifetime). `startMetricsServer` in `internal/cli/helpers.go:195-238` launched `go func() { _ = srv.ListenAndServe() }()` with no `BaseContext` and discarded the err. Two bugs: (1) in-flight `/metrics` scrape connections couldn't inherit deploy-cancel; (2) a bind failure (port in use, perm denied) showed up only as a missing `/metrics` endpoint. Wired `srv.BaseContext = func(net.Listener) context.Context { return ctx }`, captured `ListenAndServe`'s err to a buffered `errCh` (cap=1, leak-bounded — the goroutine sends exactly once and never blocks even if `stop()` is never called), and changed `stop()` to return `error`, mapping `http.ErrServerClosed` to nil. Caller's `defer` warn-logs any other err. Metrics is auxiliary so a bind failure is a warn, not a fatal. **Postmortem lesson:** A buffered cap=1 chan is the canonical leak-bound for fire-and-forget goroutines that send exactly one value; the comment naming the cap choice and the leak-bound is load-bearing per CLAUDE.md §Concurrency.

- **`api:262af6e4:cleanup-double-execute`** — done 2026-05-08 — PR #467, merge commit `51d2f71`. Tier H minor (exported-surface). `internal/distribution/okd/cleanup/cleanup.go` shipped two Execute entry points: `(p *Phase) Execute(ctx, *Options)` (a one-line forwarder) and a package-level `Execute(ctx, *Options)`. Three callers were already on the method form (`okd.go:L132`, `destroy/steps.go:L117`, tests) but `cli/cleanup.go:L133` consumed the package-level form directly. Picked option (b) per the recommendation: kept `Phase.Execute` as the canonical exported surface for sibling-shape symmetry with setup/install/postinstall/destroy, and lowered the package-level form to private `execute`. Updated `cli/cleanup.go` to use `cleanup.New(exec, opts.Logger, version.Version).Execute(...)` (post-review followup pass switched the placeholder `nil/""` args to a real executor and `version.Version` so the BasePhase fields are honestly populated). Tests in package cleanup call `execute(...)` directly. **Postmortem lesson:** Don't ship `cleanup.New(nil, ..., "")` as a "Phase.Execute only forwards opts so this is safe today" call — the next Phase method that consumes BasePhase will panic; pass real values from the start so the signature is the contract, not the runtime accident.

- **`ux:e7db1220:releases-list-omitted-vs-flat`** — done 2026-05-08 — PR #469, merge commit `ee5a7eb`. Tier H minor (json-stability). `fetchFlatVersions` in `internal/cli/releases.go:139` declared `var out []releases.OKDVersion` — a nil slice — so when the upstream feed returned zero stable entries the JSON encoder emitted literal `null\n` instead of `[]\n`, contradicting `docs/cli/json-schema.md`'s "null is never emitted" contract. One-character fix: `out := make([]releases.OKDVersion, 0, len(series))` — non-nil empty slice encodes as `[]`. New `internal/cli/releases_test.go` exercises the JSON-encoder path with both `[]T{}` and `make([]T, 0)` slices, asserting `[]` is the result and `null` never appears. The fetcher-injection seam doesn't exist on the production path so the test targets `writeJSON` directly — sufficient guard for the encoder contract; an end-to-end test on `fetchFlatVersions` would require introducing a fetcher seam, out of scope. **Postmortem lesson:** `var x []T` vs `make([]T, 0)` is one of the few Go zero-value choices that has user-facing consequences; for JSON-emitting paths, always use `make` to lock the array-not-null contract.

- **`ux:b3356305:readme-14-commands-claim`** — done 2026-05-08 — PR #470, merge commit `4a46fa0`. Tier H suggestion (readme-drift). `README.md:85` claimed "Full command reference (all 14 commands)" — fragile because the count silently drifts every time a subcommand lands or is removed. One-line fix: dropped the parenthetical to "Full command reference: [`docs/cli/okdctl.md`](docs/cli/okdctl.md)." Self-maintaining. Recommended CI-check option (diff `find docs/cli -name 'okdctl_*.md' | wc -l` against the README) was rejected as out-of-scope — the simpler-fix path was preferred and lands a real fix today rather than a follow-up CI item. **Postmortem lesson:** README claims with hard-coded counts are anti-patterns; remove the count and link to the auto-generated source of truth.

- **`obs:aa84670c:run-id-not-on-pre-runid-records`** — done 2026-05-08 — PR #471, merge commit `0d4f47b`. Tier H minor (span-retry-boundary). `cli/root.go::execute()` set `run_id` only inside per-subcommand RunE handlers (deploy.go:L55, destroy.go:L139), so preflight, configureLogging errors, and BackgroundCheck records emitted before `tui.SetRunID` ran — JSON pipelines couldn't correlate them to an invocation. Generated `uuid.NewString()` as the FIRST statement in `execute()` and immediately pinned via `tui.SetRunID`, then emitted `tui.Info("okdctl: started", tui.LF("argv", strings.Join(os.Args[1:], " ")))` and a deferred `tui.Info("okdctl: finished", tui.LF("duration", ...), tui.LF("exit_code", code))` so every invocation has a one-line span boundary on stderr. Function signature became `func execute() (code int)` (named return) so the deferred `finished` record sees the final exit code; the `signalExitCode` short-variable was renamed to `sigCode` to avoid shadowing the named return. Post-review pass dropped the per-command `tui.SetRunID(uuid.NewString())` calls in deploy.go/destroy.go that overwrote the span-level ID; lint-go nakedret fix replaced the bare `return` after `code = sigCode` with `return sigCode` (named-return mechanics still set `code` correctly). **Caveat:** `cmd/okdctl/main.go::preflight()` still fires before `execute()`, so its records carry no run_id; closing that gap is a future architectural item. **Postmortem lesson:** Lint catches what reviewers miss — funlen and nakedret both fired on this and #463 in the same session; let the lint be the third reviewer rather than fighting it post-push.

- **`ux:aa84670c:exit-code-package-doc-drift`** — done 2026-05-08 — no new PR (fix already on develop via companion item `doc:aa84670c:exit-code-77-pkgdoc-drift`, PR #368, commit `b41600a`). Tier H minor (package-doc). The stale `rejection=77 (EX_NOPERM, set in cmd/okdctl/main.go)` clause is absent from `internal/cli/root.go:1-10`; the package doc now reads `auth error=5 ... (includes invoked-as-root rejection via AuthError)`, aligned with `internal/cli/elevation.go::ensureRoot` returning `*errtypes.AuthError` and `exitCodeFor` mapping it to 5. `docs/cli/exit-codes.md:25-27` already documents code 5 for root invocation. `EX_NOPERM`/`=77` literals appear only in archival prose. PR #468 was opened with the archive sweep but auto-closed empty after rebasing onto a develop that already had the entry via the parallel sec/state/err sweep commit. **Postmortem lesson:** When a 2026-05-05 audit captures a finding the 2026-04-21 audit already filed under a sibling ID (`doc:aa84670c` vs `ux:aa84670c`), grep the completed-archive before claiming the worktree — saves a wasted plan/review cycle.

- **`tst:21dc1103:download-no-test`** — done 2026-05-08 — PR #477, merge commit `b414686`. Tier I blocker (canonical-helper-untested). `internal/download/download.go:79-168` had zero coverage on Download/fetchToFile/canSkipDownload — release tarballs flow through this surface and into `/usr/local/bin` under sudo, so a regression on the write path lands attacker-controlled binaries. Added `internal/download/download_test.go` covering happy path (0o600 perms), HTTP non-200 → `*HTTPStatusError`, ctx-cancel partial-file cleanup, retryDownload retriable-error retry, canSkipDownload branch matrix, and a symlink-at-OutputPath test that locks the future O_NOFOLLOW guard from `sec:21dc1103`. The `testing.Short()` gate on the retry test was removed so CI exercises the retry path; first-failure backoff is ~2.5-7.5 s. Round-1 reviewer caught the missing symlink test, mis-named retry test (roadmap framing of "checksum-mismatch retry" doesn't match `verifyDownloadedFile` semantics — retry is transport-tier only), and the `-short` gate. Round-2 fix added all three. CI then failed on the symlink test because `canSkipDownload` short-circuits when `ExpectedChecksum == ""` (`os.Stat` follows the symlink to its non-empty target); a third commit added `Overwrite: true` on the test opts to bypass canSkipDownload. **Postmortem lesson:** When a test's contract is "the write path", set Overwrite or ExpectedChecksum so canSkipDownload does not short-circuit before fetchToFile runs — otherwise the test asserts on state the function never modified, and local-pass-CI-fail is the symptom.

- **`tst:33579dd5:dnsmasq-config-path-untested`** — done 2026-05-08 — PR #478, merge commit `1e0ba74`. Tier H minor (trust-boundary-untested). `internal/distribution/okd/cleanup/services.go:142-187` had no test for the cluster-name-driven path through `Dnsmasq()` — clusterName flows into `dns.DnsmasqConfigPath` then `os.RemoveAll` under a refuseCriticalPath guard. Round-1 used a sentinel-in-tempdir assertion that the reviewer flagged as vacuous: the resolved traversal path `/etc/okd-x.conf` could never reach an unrelated tempdir, so the test passed regardless of whether `validateConfigName` was present. Round-2 replaced it with a direct `dns.DnsmasqConfigPath("okd-../../../../etc/okd-x")` rejection assertion (the regex layer is the actual contract), plus a no-error-from-Dnsmasq assertion confirming the warn-and-skip path. Renamed to `…AtRegex` since `refuseCriticalPath` is unreachable on this input class. **Postmortem lesson:** When a roadmap item names guard X but the test must pass through layer Y to reach X, the test name and assertions belong to layer Y — sentinel-in-tempdir designs are vacuous if the threat path can't logically reach the sentinel.

- **`sec:8ea706f6:input-validation`** — done 2026-05-08 — PR #476, merge commit `3491198`. Tier H suggestion (input-validation). `installTerraform` on RHEL at `internal/distribution/okd/setup/tools.go:129-133` shelled to `dnf config-manager --add-repo https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo` — the `.repo` file's `gpgkey=` URL is trusted at fetch time over HTTPS-only. Replaced with a `//go:embed hashicorp.repo` build-time-pinned descriptor written via `system.AtomicWrite` to `/etc/yum.repos.d/hashicorp.repo` (root-owned 0o644). Round-1 used `system.WriteAsInvokingUser`, which the reviewer correctly flagged as a security blocker: under sudo re-exec it chowns to `SUDO_UID:SUDO_GID`, letting a non-root user later edit `gpgkey=` without sudo and poison subsequent `dnf install` operations. Round-2 swapped to AtomicWrite. **Postmortem lesson:** Roadmap fix-text occasionally names the wrong canonical helper — `WriteAsInvokingUser` is for files the operator should own; system trust roots like `/etc/yum.repos.d/*.repo` must stay root-owned. Match the deb-side pattern (`CopyFile` from a temp), not the recommended helper.

- **`mod:5013fea6:use-slices-containsfunc`** — done 2026-05-08 — PR #474, merge commit `ff57651`. Tier H suggestion (slices-maps). `isAuthError` at `internal/distribution/okd/setup/release_extract.go:139-147` hand-rolled a contains-by-predicate loop over `authMarkers`. Replaced with `slices.ContainsFunc(authMarkers, func(m string) bool { return strings.Contains(lower, m) })`. Pure refactor; same lowercase normalisation, same predicate, same slice. Reviewer cleared in one round. **Postmortem lesson:** None — straight modernization; no new pattern to record.

- **`obs:c19ee328:debug-fmt-sprintf-package-loop`** — done 2026-05-08 — PR #472, merge commit `caaa4ab`. Tier I minor (field-stability). `installSystemPackages` at `internal/distribution/okd/setup/steps.go:312-323` used `p.Log.Debug(fmt.Sprintf("packages: %s not found", pkg))` and two siblings — interpolating values into the message string bypasses RedactHandler's structured-attr scrub and breaks `--log-format=json | jq` extraction. Replaced with the structured shape: `("packages: not found", "pkg", pkg)`, `("packages: already installed", "pkg", pkg)`, `("packages: installing missing", "count", len(toInstall))`. The `fmt` import is retained because other call sites in the file still use it. Reviewer cleared in one round. **Postmortem lesson:** None — straight modernization; the broader `obs:6424733c:fmt-sprintf-message-pattern` umbrella will sweep the remaining ~50 sites.

- **`ux:d31d1b9d:describe-format-shared-global`** — done 2026-05-08 — PR #473, merge commit `37144da`. Tier H suggestion (flag-conventions). `describeNodeCmd` and `describeAddonCmd` at `internal/cli/status.go:62-67` both bound `--format` to a single package-level `describeFormat` string, leaking state across subcommand sequences in tests. Split into per-command vars `describeNodeFormat` and `describeAddonFormat` mirroring the `releasesListFormat`/`releasesShowFormat` pattern at `internal/cli/releases.go:25-29`. Eight-line change covering the declaration, two flag bindings, and two read sites. Reviewer cleared in one round. **Postmortem lesson:** None — symmetric pattern restoration.

- **`obs:25fa1be8:nil-logger-deref`** — done 2026-05-08 — PR #475, merge commit `39298db`. Tier I minor (handler-setup). `firewall.Configure`, `firewall.RemoveRules`, and `firewall.openPort` at `internal/distribution/okd/firewall/firewall.go:117-192` accepted `*slog.Logger` and dereferenced unconditionally, while `DetectBackend` documented nil tolerance — a future caller honouring the documented contract would crash. Wrapped each entry-point with `logger = logutil.OrNop(logger)` so the package presents one nil-policy. `modifyPort` takes no logger and is intentionally excluded. CI test-go was cancelled on the first run by a stuck runner ("operation was canceled" before tests ran); rerun was clean. **Postmortem lesson:** When CI `test-go` cancels with "operation was canceled" before tests actually start, the runner is stuck — `gh run rerun --failed` typically fixes it without code changes (matches the `sec:fde34e0c:k8sclient-env-direct-write` postmortem).

- **`sec:15ba17da:cred-no-zeroize`** — done 2026-05-08 — PR #479, merge commit `4aa0b65`. Tier H minor (credentials). Companion to sec:6424733c. The destroy path constructed the Provisioner via `createOKDProvisionerWithOpts(cfg, creds, projectRoot)` and deferred `creds.Zeroize()` but never called `Provisioner.ZeroizeEnv()`, leaving credential strings on the executor's `Env []string` for the full teardown sequence (terraform destroy + ssh-based ISO removal + cleanup). Added one line: `defer p.ZeroizeEnv()` immediately after `createOKDProvisionerWithOpts` in `internal/cli/destroy.go::runDestroy` (line 228 area), mirroring the deploy path at `helpers.go:301`. `runDestroyDryRun` already had `defer tf.ZeroizeEnv()` so destroy paths are now symmetric. **Postmortem lesson:** None — symmetric companion fix; the original sec:6424733c work landed `Provisioner.ZeroizeEnv()` but only the deploy path's defer was added at the time.

- **`api:de572c63:ctx-not-first-write-dnsmasq`** — done 2026-05-08 — PR #480, merge commit `c41005c`. Tier G suggestion (context-propagation). `writeDnsmasqConfig` and `ConfigureSystemResolver` in `internal/distribution/okd/dns/dnsmasq.go` advertised cancellation only via the entry-time `ctx.Err()` check; mid-operation cancellation between `os.MkdirAll` / `system.WriteTempFile` / `system.CopyFile` was silently ignored until the next blocking I/O happened to fail. Added per-step `select { case <-ctx.Done(): return ctx.Err(); default: }` between the backup CopyFile and AtomicWriteString in `writeDnsmasqConfig`, and between mkdir/WriteTempFile/CopyFile in `ConfigureSystemResolver`'s systemd-resolved branch. Pure additive change; no helper signatures touched. **Postmortem lesson:** The first-option fix in audit findings ("plumb ctx into helpers") is a deeper refactor than the second option ("select-on-ctx between steps") — the second is faster to land and equally correct.

- **`api:761e5126:export-no-caller-removehaproxy`** — done 2026-05-08 — PR #481, merge commit `9e8883d`. Tier G suggestion (scaffolding). `RemoveHAProxy` at `internal/distribution/okd/postinstall/haproxy.go:31` was exported but had only one in-tree caller (`finalizeIngress` in update_ingress.go); per MEMORY policy, scaffolding-shaped exports stay. Added two doc-comment lines naming the only in-tree caller and the reserved `okdctl haproxy` subcommand space. Bundled with `tst:761e5126`. **Postmortem lesson:** None — doc-only fix preserving API-shape per scaffolding policy.

- **`tst:761e5126:no-test-removehaproxy`** — done 2026-05-08 — PR #481, merge commit `9e8883d`. Tier G major (destructive-untested). `RemoveHAProxy` at `internal/distribution/okd/postinstall/haproxy.go` had no test coverage on the destructive code path. Added `TestRemoveHAProxy_HappyPath_ConfigFileRemoved` (writes a stub at `haproxyConfigPath`, asserts `os.IsNotExist` after the call) and `TestRemoveHAProxy_ConfigRemoveAllError_DoesNotAbort` (creates the file in a 0o555 parent so RemoveAll fails permission-denied; asserts the function still returns nil — confirms the Warn-and-continue branch). The latter skips when running as root because root bypasses 0o555 perms. Cases (b) and (d) from the roadmap fix were already covered by pre-existing `TestRemoveHAProxy_EmptyVIPSkipsVerify` and `TestRemoveHAProxy_KubeVIPHealthcheck`. **Postmortem lesson:** Permission-tied resilience tests (chmod 0o555 → expect failure) are no-ops when the test runs as root; gate with `os.Getuid() == 0` skip so the test self-documents the gap rather than silently passing.

- **`iac:b803fcb7:ci-no-tflint-tfsec`** — done 2026-05-08 — PR #482, merge commit `17c4c45`. Tier G minor (ci-coverage). `validate-terraform` + `lint-terraform` jobs covered fmt/validate/tflint but not secret/policy scanning. Added a sibling `scan-terraform` job running `aquasecurity/tfsec-action@b466648d6e39e7c75324f25d83891162a721f2d6 # v1.0.3` (SHA-pinned with version trailer per CLAUDE.md §dependencies) against `infrastructure/terraform/` with `soft_fail: true` so findings surface in PR logs without blocking merges initially. Operators flip to hard-fail once the team has triaged any baseline output. **Postmortem lesson:** None — straight CI hardening.

- **`iac:18a795d5:hcl-no-prevent-destroy-masters`** — done 2026-05-08 — PR #483, merge commit `fcb507e`. Tier G suggestion (hcl-safety). `prevent_destroy = true` was already pinned on the master VM resource in `infrastructure/terraform/modules/proxmox-okd/main.tf:249`, but the surrounding comment said "place an override.tf in this module directory that removes or overrides this lifecycle block" — Terraform IGNORES override files inside modules; only root-module overrides take effect (hashicorp/terraform#3116 also forbids variable-gating prevent_destroy). Replaced the comment with two correct procedures: `terraform state rm 'module.okd.proxmox_virtual_environment_vm.master[N]'` (per-resource) and a root-environment override file (`environments/production/override.tf`, gitignored) that flips `prevent_destroy = false`. Added `environments/production/.gitignore` covering `override.tf` and `*_override.tf` so a temporary override is never accidentally committed. variables.tf comment now points to main.tf for the canonical procedure. **Postmortem lesson:** Document override procedures by what Terraform actually accepts — module-level override files are silently ignored; only root-module overrides take effect.

- **`sec:00000006:debug-bundle-redact-partial`** — done 2026-05-08 — PR #484, merge commit `65742f3`. Tier G minor (redaction-completeness). `redactConfig` in `internal/cli/config.go:74-84` only masked `Provider.Proxmox.TokenID`, leaving every other config field unchanged — the function shape encouraged a future "add a string field, forget to redact" regression. Replaced with a reflection walker that recursively masks every exported string field whose JSON-tag name contains a denylist fragment (password/token/secret/api_key/apikey). Exported `logutil.KeyIsSecret` (renamed from `keyIsSecret`) so both sites share the single `secretKeyFragments` list. Tests cover non-sensitive pass-through (Host), nested struct masking via the pointer-clone-then-recurse path (Provider.Proxmox.TokenID), and nil-pointer no-panic. Round-1 lint flagged govet's `inline: Constant reflect.Ptr should be inlined`; round-2 commit replaced both `reflect.Ptr` cases with `reflect.Pointer` (the Go 1.18+ canonical name). **Postmortem lesson:** govet's `inline` rule wants the canonical kind name `reflect.Pointer`, not the legacy alias `reflect.Ptr` — applies anywhere this codebase reflects on Kind.

- **`sec:451be4fa:chowntree-symlink-audit`** — done 2026-05-08 — PR #485, merge commit `04c5108`. Tier G minor (privilege). `ChownTreeToInvokingUser` at `internal/system/elevation.go:108` accepted any string and recursively chowned the tree under the invoking user — the docstring required callers to pass paths whose subtree okdctl created in this process, but no runtime guard. Added `isAllowedChownRoot(absPath, homeDir, tmpDir)` allowing Base==okd-install/infrastructure (the only trees okdctl creates as root), subpaths of the invoking user's home, or subpaths of os.TempDir(). Disallowed paths return `*errtypes.AuthError`. Symlinks resolved via `canonicalizePath` (filepath.EvalSymlinks) so /tmp -> /private/tmp on macOS does not split the predicate. Round-1 lint flagged `revive: redefines-builtin-id` because the EvalSymlinks return value was named `real`; round-2 renamed to `resolved`. **Postmortem lesson:** Go's `real` builtin is rarely used in this codebase but revive flags shadowing — avoid `real`/`imag`/`new`/`make`/`copy`/`len`/`cap`/`close`/`delete` as variable names even where they read more naturally.

- **`tst:451be4fa:no-test-writeasinvoking`** — done 2026-05-08 — PR #485, merge commit `04c5108`. Tier G minor (canonical-helper-untested). Bundled with sec:451be4fa. `WriteAsInvokingUser` at `internal/system/elevation.go:82-98` had a subtle parentExisted invariant — chown the parent dir iff AtomicWrite created it, not when the user pre-created with different ownership. Extracted the `os.Stat(parentDir)` probe as a `statFn` package var so a test can drive both branches without root. `TestWriteAsInvokingUser_ParentExistedFlag` table-tests both paths via `statFn` swap. The chown-back is not mocked — actual root-required behaviour stays acknowledged-but-untested per the roadmap's "minimal value" guidance. **Postmortem lesson:** None — minimal seam test landing alongside the security guard.

- **`tst:daf5bee9:no-test-kubeconfig-merge-full`** — done 2026-05-08 — PR #486, merge commit `d69dfcc`. Tier G blocker (canonical-helper-untested). `mergeKubeconfig` at `internal/cli/kubeconfig.go:80-125` had `mergeNamedList` covered but the full-pipeline invariants (current-context preservation when dest has one; AtomicWrite at 0o600) were untested end-to-end. Added `TestMergeKubeconfig_PreservesCurrentContext` (dest current-context=prod with one cluster, src current-context=okd-test with clusters [okd-test, dev], result preserves "prod" and merges all three cluster names) and `TestMergeKubeconfig_EmptyDestTakesSrcCurrentContext` (no dest file → result adopts src's current-context). Both assert dest mode is 0o600. Tests use stdlib `testing` + `sigs.k8s.io/yaml` for typed parse-back; `t.Setenv("KUBECONFIG", tmp)` redirects `mergeTargetPath`. **Postmortem lesson:** None — load-bearing blocker test landed on first review.

- **`tst:6b533f2d:no-test-approve-pending-csrs`** — done 2026-05-08 — PR #487, merge commit `c398f57`. Tier G major (canonical-helper-untested). `ApprovePendingCSRs` at `internal/cluster/k8s_csrs.go:53-74` drives MonitorInstallation's CSR-approval loop with no test coverage. Added `internal/cluster/k8s_csrs_test.go` using the PATH-shadow fake-oc pattern from `internal/distribution/okd/phase/kubectl_test.go`: write a POSIX-sh `oc` into `t.TempDir()`, prepend to PATH, branch on `$1` for `get`/`adm` cases driven by `OC_*` env vars (`OC_CSR_JSON`, `OC_GET_EXIT`, `OC_ARGV_FILE`, `OC_APPROVE_EXIT`) which pass the executor allowlist via the `OC_` prefix entry. Four cases: empty list → (0,nil) and no approve call, batched single argv with all 3 CSR names in one line, PendingCSRs error propagates as `*errtypes.ClusterError`, approve exit non-zero wraps with "failed to approve CSRs" prefix. **Postmortem lesson:** Shell `${VAR:-default}` parameter expansion mis-parses when `default` contains `}` — saw `{"items":[]}}` (extra `}`) on the first attempt. Hoisted the default value to its own variable: `default_csr='{"items":[]}'; printf '%s' "${OC_CSR_JSON:-$default_csr}"`. Worth knowing for future test-shim work that interpolates JSON literals.

- **`tst:830d4653:no-test-packages-cleanup-guard`** — done 2026-05-08 — PR #488, merge commit `7d50f8e`. Tier G major (destructive-untested). `Packages` at `internal/distribution/okd/cleanup/packages.go:47-87` chains `BinDirOrDefault` → `refuseCriticalPath` → `os.RemoveAll` — the per-iter and top-level guards are the only thing stopping `OKDCTL_BIN_DIR=/etc` from walking RemoveAll into /etc/yq. Strengthened the existing `TestPackages_RefusesCriticalBinDir` to assert `*errtypes.ClusterError` via `errors.As` (the type was unverified before — only non-nil), and added `TestPackages_MissingBinariesNoError` covering the empty-binDir case. Happy path `TestPackages_RemovesScopedBinariesOnly` was already present (the roadmap-named TestPackages_HappyPath maps to it). All three cases use `installFakePkgTools` which puts fake `rpm`/`dnf`/`dpkg`/`apt-get` exit-0 scripts onto PATH so the test runs without root. **Postmortem lesson:** None — straight test additions matching an existing harness pattern.

- **`tst:29293401:no-test-haproxy-rollback`** — done 2026-05-08 — PR #489, merge commit `df568a9`. Tier G major (canonical-helper-untested). `ConfigureHAProxy` at `internal/distribution/okd/setup/haproxy.go:87-146` had a rollback closure with hard-coded `system.AtomicWriteString` and `system.ManageService` calls — untestable without real /etc/haproxy and systemd. Extracted as package-local `attemptHAProxyRollback(cause error, cfgPath, backupPath string, writeFn func(string, string, os.FileMode) error, restartFn func() error) error` with table-driven tests covering write-fails (cause + write joined, restart sentinel never called), restart-fails (cause + restart joined), and happy rollback (cause returned). Helper preserves `errors.Join` semantics. Drive-by: replaced `p.Log.Warn(fmt.Sprintf("haproxy: %s, restoring from backup", reason))` with structured `Warn("haproxy: restoring from backup", "reason", reason)` to satisfy CLAUDE.md's ban on `fmt.Sprintf` into log messages. **Postmortem lesson:** When extracting a closure for testability, also fix any CLAUDE.md log-pattern violations in the closure body — same diff covers both at lower review cost than two PRs.

- **`tst:632c9087:no-test-buildlb-ingresscontroller`** — done 2026-05-08 — PR #490, merge commit `b847b24`. Tier G major (canonical-helper-untested). `convertToLoadBalancer` in `internal/distribution/okd/postinstall/update_ingress.go` is destructive (oc delete + oc create with rollback). The pure JSON transforms `buildLBIngressController` (must preserve six optional spec fields while swapping strategy to LoadBalancerService) and `buildRollbackJSON` (must strip server-managed metadata) had no tests. Added `TestBuildLBIngressController_PreservesSpecFields` (RawJSON with all six fields populated; output round-trips intact and strategy.type == LoadBalancerService) and `TestBuildLBIngressController_EmptyNamespaceDefaults` (empty namespace defaults to openshift-ingress-operator). Strengthened the existing `TestBuildRollbackJSON_StripsServerFields` to include `uid` in the strip list (was missing) and assert spec/name/namespace survive. **Postmortem lesson:** None — pure-function tests using stdlib `encoding/json`.

- **`smell:262af6e4:enum-ad-hoc-cleanup-kind`** — done 2026-05-08 — PR #491, merge commit `5c6f488`. Tier I suggestion (magic-strings). `Kind` at `internal/distribution/okd/cleanup/cleanup.go:23-34` had no IsValid/Validate/ValidKinds/KindStrings — the inline check at `execute()` listed all five constants by literal string, drifting from the typed enum. Added the four canonical helpers mirroring `config.SupportedDistributions`: `ValidKinds() []Kind`, `KindStrings() []string`, `(k Kind) IsValid() bool`, `(k Kind) Validate() error` (returns `*errtypes.ConfigError` with message built via `strings.Join(KindStrings(), ", ")`). `execute()` now calls `opts.Kind.Validate()` instead of the inline disjunction. **Postmortem lesson:** None — symmetric pattern restoration.

- **`smell:262af6e4:pipeline-explicit-errors-cleanupkind`** — done 2026-05-08 — PR #491, merge commit `5c6f488`. Tier I minor (magic-strings). The cleanup-kind validation hardcoded the valid-values list as a literal string `(valid types: full, work-only, web-only, haproxy-only, terraform-only)` separate from the typed-enum constants. Resolved alongside `enum-ad-hoc-cleanup-kind` — the new `KindStrings()` helper supplies the canonical list, and `(k Kind) Validate()` formats the error via `strings.Join(KindStrings(), ", ")`. Adding a new Kind now needs only an entry in `ValidKinds()`. **Postmortem lesson:** None — the four bundled smell:262af6e4 entries fall naturally into one PR; the helper landed in the bundling commit closes both this entry and enum-ad-hoc-cleanup-kind.

- **`smell:262af6e4:pipeline-explicit-errors`** — done 2026-05-08 — PR #491 (already on develop pre-PR; documented in the bundling commit). Tier I minor (arrow-anti). The original entry called out `Execute()` repeating the `if err := X(...); err != nil { errs = append(errs, err) }` shape eight times; that pattern was ALREADY refactored away on develop before the PR landed — `cleanup.Execute` now delegates to a private `execute()` that uses `distribution.NewOrchestrator(distribution.BuildSteps(defs)...)` and a `cleanupTracker`. The roadmap entry was stale; archiving on PR #491 alongside the related enum/validate work. **Postmortem lesson:** Audit-fan-out tier items can outlive their fix when a sibling PR closes the underlying smell without updating the roadmap entry. Periodic grep-against-evidence sweeps catch this — recommended next-quarter follow-up.

- **`smell:262af6e4:dual-cleanup-tracker`** — done 2026-05-08 — PR #491 (already on develop pre-PR; documented in the bundling commit). Tier I minor (helper-package-no-value). The original entry called out two parallel cleanup-tracking surfaces (`cleanup.Execute`'s `errs []error` accumulator + `destroyTracker` in destroy/steps.go); that migration was ALREADY landed on develop before the PR — `cleanup.Execute` delegates to `execute()` which uses `distribution.NewOrchestrator(distribution.BuildSteps(...))` with a `cleanupTracker` shape-identical to `destroyTracker`. The roadmap entry was stale; archiving on PR #491 alongside the related enum/validate work. **Postmortem lesson:** Same as `pipeline-explicit-errors` — stale audit entries vs already-landed fix; the planner's evidence check ("is this still true?") caught it before code was written, exactly the value of the read-only planner separation.

- **`obs:0d318f5c:handler-no-tty-switch`** — done 2026-05-05 — already on develop in PR #386, merge commit `ea26b29` (feat(cli): default log-format to json on non-tty stderr). Tier I minor (handler-setup). Discovered during planner-phase of /roadmap-pickup as a stale duplicate of `obs:0d318f5c:no-tty-format-default-mismatch` (already archived) — the same finding under a different sub-id from a later audit run. `internal/cli/logging.go:65-75` already sets `logFormat = outputJSON` when `--log-format` is unset and stderr is not a TTY, exactly matching the Fix description. **Postmortem lesson:** Two audit runs can capture the same finding under different sub-ids (`:handler-no-tty-switch` vs `:no-tty-format-default-mismatch`); when grepping the archive for "is X already done?" search the audit ID prefix (`obs:0d318f5c:`) not just the full sub-id.

- **`sec:f55b9c27:cred-string-copy-envfile`** — done 2026-05-08 — already on develop in commits `4024257` (refactor) + `d141cd5` (clear builtin) + `1c67be9` (zeroize test). Tier H major (credentials). Discovered during planner-phase. `WriteEnvFile` at `internal/credentials/envfile.go:48-72` already builds the file content via `bytes.Buffer.Write(creds.Password)` / `bytes.Buffer.Write(creds.APIToken)` (not `string(...)` concatenation), passes the buffer's bytes to `system.AtomicWrite`, then calls `clear(data)` (Go 1.21 builtin) to wipe the backing slice — credential bytes never leave the wipeable path. Test `TestWriteEnvFile_BufferZeroedAfterWrite` already locks the invariant. **Postmortem lesson:** Same shape as obs:0d318f5c — the audit's roadmap entry survived the underlying fix; archive sweeps catch this. Grep evidence (`bytes.Buffer.Write` + `clear(`) before claiming a worktree to avoid wasted plan/review cycles.

- **`sec:0d318f5c:cred-no-zeroize`** — done 2026-05-08 — acceptance note. Tier H suggestion (credentials). `configureLogging` at `internal/cli/logging.go:25-67` already opens `--log-file` with `O_CREATE|O_NOFOLLOW` after an `lstat` rejection of non-files — the audit reflagged it specifically as a **counter-example reference** that other sites should mirror, with explicit Fix text "Already-hardened. Documenting as a counter-example reference … No action needed." No code change required. **Postmortem lesson:** Audit entries can be "audit-positive" pointers naming a canonical pattern; their roadmap presence is for discoverability, not a defect to fix. Treat the explicit "No action needed" Fix line as the disposition.

- **`sec:8ea706f6:cred-no-zeroize`** — done 2026-05-08 — acceptance note. Tier H suggestion (credentials). `installHashiCorpDebianRepo` at `internal/distribution/okd/setup/tools.go:227-248` writes the GPG armored key to a 0o600 temp via `system.WriteTempFile`, runs `gpg --dearmor -o /usr/share/keyrings/...gpg`, then defers `os.Remove`. The dearmored output is a public key (world-readable by design); the brief armored-temp window is on a public artifact, no secret leak. Audit Fix: "Acceptable as-is — the GPG key is public." No code change required. **Postmortem lesson:** `system.WriteTempFile`'s callback-then-cleanup is the canonical helper for short-lived temp materials; secrecy of the contents drives whether the cleanup window matters, not the helper choice.

- **`api:8e65d574:iface-in-producer`** — done 2026-05-08 — acceptance note. Tier H suggestion (interface-location). `csrApprover` at `internal/distribution/okd/install/monitor.go:52-56` is consumer-side defined (correct Go idiom for a single consumer). `cluster.K8sClient.ApprovePendingCSRs` is the producer concrete; the option-struct shape (`WithCLI`/`WithKubeconfig`/`WithLogger`) is fine. Audit Fix: "Leave as-is for now (single consumer = correct Go idiom). Watch for a second consumer; promote to internal/cluster.CSRApprover only when a second site declares the same shape. Filing a tracking item is enough." No code change required. **Postmortem lesson:** Consumer-side interface declaration is canonical Go; promote to a producer-side interface only when ≥2 consumers materialise the same shape, not as a speculative refactor.

- **`dep:33ef32bf:atotto-clipboard-stale`** — done 2026-05-08 — acceptance note. Tier H minor (maintenance-signal). `github.com/atotto/clipboard v0.1.4` (2021-02-24, BSD-3-Clause, bus-factor 1) at `go.mod:24` is pulled transitively via `charm.land/bubbles/v2/textinput` for paste-into-input support. okdctl has no direct usage — abandonment is bound to the upstream charm-libs migration per CLAUDE.md §dependencies must-preserve charm libs policy. Audit Fix: "No action … Re-evaluate if charm.land/bubbles releases a version that drops the clipboard dep." No code change required. **Postmortem lesson:** When a transitive dep sits under a must-preserve upstream, the abandonment trigger is upstream's choice — track via the charm-libs pin policy, don't try to `replace` directive around it.

- **`dep:33ef32bf:gorilla-websocket-stale`** (Tier H 2026-04-25) — done 2026-05-08 — acceptance note. Tier H minor (maintenance-signal). `github.com/gorilla/websocket v1.4.2` (2020-03-19) at `go.mod:41` is pulled transitively via `github.com/luthermonson/go-proxmox` for shell/console websocket support. okdctl never reaches it — wizard uses REST-only Proxmox discovery per CLAUDE.md §dependencies. Audit Fix: "Safe to keep until go-proxmox migrates to coder/websocket, at which point take the bump without local code changes." No code change required. **Postmortem lesson:** Transitive deps with a documented "wait for upstream swap" plan stay until the swap; documenting the policy in CLAUDE.md is the durable artifact, the per-audit re-flag is just a re-confirmation.

- **`dep:33ef32bf:proxmox-v0x-bus-factor`** — done 2026-05-08 — acceptance note. Tier H minor (maintenance-signal). `github.com/luthermonson/go-proxmox v0.4.1` at `go.mod:14` is the sole Proxmox VE discovery dep (single call site at `internal/tui/wizard/steps/proxmox_discovery.go`). Bus-factor 1; v0.x semver. CLAUDE.md §dependencies names this as the canonical v0.x exposure with a documented ~200 LOC REST-only fallback. Audit Fix: "No action this run. Track go-proxmox releases each audit cycle … When a v1.0 lands, evaluate the bump. If go-proxmox is abandoned for >12 months, execute the CLAUDE.md fallback." No code change required. **Postmortem lesson:** v0.x deps need both a written abandonment plan AND an active upstream-cadence check; this dep has both (latest v0.4.1 dated 2026-04-03, fallback documented).

- **`dep:33ef32bf:xo-terminfo-untagged`** — done 2026-05-08 — acceptance note. Tier H minor (maintenance-signal). `github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e` at `go.mod:57` is an untagged 2022 pseudo-version (3+ years stale, MIT) pulled transitively via `charm.land/lipgloss/v2 → github.com/charmbracelet/colorprofile`. Same disposition as `atotto-clipboard-stale`: charm-libs control the choice. Audit Fix: "No action. Same policy as atotto-clipboard-stale." No code change required. **Postmortem lesson:** Untagged pseudo-versions under must-preserve upstream owners are not actionable from this repo; document the constraint in CLAUDE.md once and re-confirm via audit cadence.

- **`dep:b803fcb7:tflint-action-no-version-trailer`** — done 2026-05-08 — acceptance note. Tier H suggestion (pin-stability). `terraform-linters/setup-tflint@b480b8fcdaa6f2c577f8e4fa799e89e756bb7c93 # v6.2.2` at `.github/workflows/ci.yml:101` already matches the canonical `uses: owner/action@<40-hex-sha> # vX.Y.Z` convention from CLAUDE.md §dependencies. The audit re-validated the pin and found no defect. Audit Fix: "No action. The pin matches the SHA + version-trailer convention. This row exists so future audit runs can trace the pin-audit decision." No code change required. **Postmortem lesson:** Audit-positive rows on pin-stability findings serve as the audit history's traceability; resist "fixing" what already matches.

- **`dep:33ef32bf:dup-log-engines`** — done 2026-05-08 — acceptance note. Tier H suggestion (duplicate-engine — seam→audit-modernization). Same finding as the Tier I 2026-05-05 echo `dep:33ef32bf:dual-log-engines` (archived in this batch). Four log engines link in: stdlib `log/slog` (canonical, 44 imports), `charm.land/log/v2 v2.0.0` (single site at `internal/tui/logger.go`, the slog-handler implementation, not a parallel API), and `k8s.io/klog/v2` + `go-logr/logr` (transitive via k8s.io/api). The "Optional" charm.land swap conflicts with CLAUDE.md §dependencies must-preserve charm libs policy and the documented intent that bubbletea TUI integration relies on the styled output. No action required. **Postmortem lesson:** "Optional" swap-recommendations from audits should be cross-checked against CLAUDE.md must-preserve clauses before scheduling — the must-preserve policy supersedes audit ergonomics suggestions.

- **`iac:e076e43c:sh-doc-line-stale`** — done 2026-05-08 — auto-resolved by PR #340 (merge `434b118`). Tier I suggestion (install-sh-integrity — related: `iac:e076e43c:sh-bash-array-dash-incompat`). The audit Fix said: "no separate fix needed once `iac:e076e43c:sh-bash-array-dash-incompat` is addressed." That sibling finding completed on 2026-05-05 (PR #340) — both the docstring `Requires:` line at `scripts/install.sh:21` and the Usage example at `scripts/install.sh:10` already pipe to `bash` explicitly, matching the helm.sh/k3s.io pattern. No code change required. **Postmortem lesson:** Dependent doc-only findings get auto-archived when the canonical sibling fix lands; the planner-phase grep against current evidence catches these stale entries before any worktree gets claimed.

- **`dep:33ef32bf:proxmox-bus-factor-1`** — done 2026-05-08 — acceptance note. Tier I minor (maintenance-signal). Re-confirmation of the Tier H 2026-04-25 entry `dep:33ef32bf:proxmox-v0x-bus-factor` (archived in this batch). `github.com/luthermonson/go-proxmox v0.4.1` at `go.mod:13` re-validated: 28 commits in last 6 months, latest tag 2026-04-03, Apache-2.0, no archive marker. CLAUDE.md §dependencies abandonment plan still valid. Audit Fix: "No code change. Re-confirm CLAUDE.md §dependencies justification annually." No code change required. **Postmortem lesson:** Same finding under different audit-run sub-ids batches naturally — Tier H + Tier I echoes archive together when the disposition is identical.

- **`dep:33ef32bf:dual-log-engines`** — done 2026-05-08 — acceptance note. Tier I suggestion (duplicate-engine). Re-confirmation of the Tier H 2026-04-25 entry `dep:33ef32bf:dup-log-engines`. Two engines: stdlib `log/slog` primary; `charm.land/log/v2 v2.0.0` as the single styled-stderr slog-handler implementation in `internal/tui/logger.go`. Intentional per CLAUDE.md (charm libs canonical TUI stack). Audit Fix: "No action. Single charm.land/log/v2 site is the slog-handler implementation, not a parallel logging API … Document this as the intentional baseline if a future audit re-flags." No code change required. **Postmortem lesson:** Where a single styling library implements the slog.Handler interface, it is not a "second engine" — the engine count grep should be filtered by API surface, not import count.

- **`dep:33ef32bf:exp-stale-transitive`** — done 2026-05-08 — acceptance note. Tier I suggestion (justified-version-floor). `golang.org/x/exp v0.0.0-20231006140011` at `go.mod:57` is an October 2023 pseudo-version pulled transitively via charm/k8s; not directly imported by okdctl. Floor advances mechanically on the next charm/k8s bump via `go mod tidy`. Audit Fix: "On the next k8s.io/apimachinery or charm.land/* bump run `go mod tidy` and re-check; the transitive pin will move. No targeted action needed." No code change required. **Postmortem lesson:** Transitive `x/exp` floors don't require dedicated PRs — they ratchet forward with sibling-dep bumps. Don't open a tidy-only PR just for this.

- **`dep:33ef32bf:go-proxmox-transitive-weight`** — done 2026-05-08 — acceptance note. Tier I suggestion (transitive-weight). `go-proxmox v0.4.1` at `go.mod:13` drags 7 transitive deps (gorilla/websocket, buger/goterm, jinzhu/copier, magefile/mage, diskfs/go-diskfs, djherbis/times, h2non/parth) okdctl never reaches via the REST-only `proxmox_discovery.go` call site. The hand-rolled REST fallback in CLAUDE.md §dependencies (~200 LOC) would shed all 7 in one move. Audit Fix: "Track go-proxmox upstream releases. When the bus-factor or breaking-change risk crosses the threshold documented in CLAUDE.md §dependencies, execute the ~200 LOC REST-only rewrite plan; that swap removes 7 transitive deps in one move. Until then, hold." No code change required. **Postmortem lesson:** Transitive-weight findings on must-preserve-or-fallback deps stay open as cost-shape documentation; the swap is a single coordinated decision, not seven separate evictions.

- **`dep:33ef32bf:gorilla-websocket-stale`** (Tier I 2026-05-05) — done 2026-05-08 — acceptance note. Tier I suggestion (maintenance-signal). Re-confirmation of the Tier H 2026-04-25 entry of the same name (archived in this batch). Upstream is at v1.5.3 (2024-06); okdctl pins v1.4.2 transitively via go-proxmox and never reaches it. A `replace` directive would diverge from go-proxmox's tested set without local benefit. Audit Fix: "No local action … Wait for go-proxmox to bump (or migrate to coder/websocket) and take the transitive bump for free." No code change required. **Postmortem lesson:** `replace` directives on unreached transitives introduce risk without payoff — always wait for the direct-dep owner to upgrade.

- **`dep:33ef32bf:goterm-stale-transitive`** — done 2026-05-08 — acceptance note. Tier I suggestion (maintenance-signal). `buger/goterm v1.0.4` (last commit 2023-02-25, ~3 years stale) at `go.mod:24` is pulled transitively via `go-proxmox` for terminal helpers okdctl never reaches. Not archived upstream but >18mo since last push triggers maint-stale. Lives or dies with the go-proxmox swap per CLAUDE.md §dependencies fallback plan. Audit Fix: "No action. Lives or dies with go-proxmox per CLAUDE.md fallback plan." No code change required. **Postmortem lesson:** Stale transitives that are not reached by the binary are documentation-only entries; they don't justify a PR until the parent dep gets swapped.

- **`sec:06f00bcb:ignition-pullsecret-served-unauth`** — done 2026-05-08 — PR #494, merge commit `d2752a8`. Tier I major (credentials). Apache served `bootstrap.ign`/`master.ign`/`worker.ign` (which embed the OKD pullSecret) on `0.0.0.0:8080` during the 15-30 minute bootstrap window. Threaded `cfg.HTTPServer.IgnitionServerIP` (the bridge IP FCOS nodes already resolve from kargs) through `configureApachePort` and `verifyApacheListening` in `internal/distribution/okd/setup/apache.go`; non-empty bindIP rewrites `Listen 80` to `Listen <ip>:8080`. Empty-bindIP fallback preserves the legacy `Listen 8080` so existing configs without the field don't break. Documented the residual exposure window in README §Security considerations alongside the SSH TOFU note. Firewall source-restriction rules and per-node ignition tokens are explicit follow-ups (require touching `OKDRequiredPorts` + destroy/cleanup). **Postmortem lesson:** When threading a config-derived parameter through nested helpers, keep the empty-value fallback explicit — it's the difference between a security improvement and a deploy regression on configs missing the field.

- **`sec:21dc1103:download-no-nofollow`** — done 2026-05-08 — PR #495, merge commit `226ca05`. Tier I major (file-toctou). `internal/download/download.go:144` opened `OutputPath` with `O_CREATE|O_WRONLY|O_TRUNC` but no `O_NOFOLLOW`; under the sudo re-exec model a pre-created symlink would be silently followed, writing through to an attacker-chosen path. Added `syscall.O_NOFOLLOW` to the flag set; symlink targets now return `ELOOP` wrapped as `*errtypes.NetworkError`. Flipped the existing scaffolded `TestDownload_SymlinkAtOutputPath` (which previously documented the unsafe behavior) to assert non-nil error and unchanged target. Pattern matches the existing `extract.go:93` site, keeping the package self-consistent. **Postmortem lesson:** Scaffolded "documents current behavior; flip when fix lands" tests are useful — they pre-encode the regression contract and a planner just inverts the assertion. Look for them when picking up fix-it work.

- **`state:c19ee328:setup-iso-build-not-resumable`** — done 2026-05-08 — PR #496, merge commit `ea835a3`. Tier I major (crash-recoverability). `StepUploadISOs` (and originally `StepBuildISOs`) declared `ReRunSafeNo` but had no `AlreadyDone` hook — a SIGKILL mid-upload silently re-uploaded; mid-build had cosmetic re-run cost. Added `isoUploadAlreadyDone` in `internal/distribution/okd/setup/upload.go` reusing the existing `isoUploadNeeded` SSH-sha256 predicate per ISO; orchestrator skips upload when every local ISO has an identical remote sha. **First-attempt regression caught by reviewer:** an outer `isoBuildAlreadyDone` short-circuited before `BuildCustomISOs`'s per-node fingerprint loop could detect config changes (sshKey/kargs) — would have shipped stale ISOs after a config edit. Removed; the per-node `.fp-<node>` fingerprint loop already provides correct resume + change-detection. Drive-by lint fix: `//nolint:nilerr` on the conservative error-or-empty branch (intentional swallow makes Exec run and surface the real failure). **Postmortem lesson:** Outer `AlreadyDone` is dangerous when the inner Exec body has its own per-element correctness check — adding both creates a short-circuit that bypasses the inner check. Prefer one or the other, not both. The reviewer's independence (no access to the planner's "fp loop already covers this" reasoning) is what surfaced this.

- **`ux:d31d1b9d:describe-format-unvalidated`** — done 2026-05-08 — PR #497, merge commit `91ef42d`. Tier I major (json-stability). `runDescribeNode` and `runDescribeAddon` in `internal/cli/status.go` accepted `--format=json|text` but skipped both `validateFormat` and `quietForJSON` — `--format=foo` silently fell through to text, and `--format=json` mixed `tui.Info` chatter from `loadConfig`/`resolveProjectRootOrDie` into stdout, breaking `2>&1 | jq`. Added the two-line guard at function entry, matching the `runStatus`/`runReleasesList` reference shape. The format vars (`describeNodeFormat`/`describeAddonFormat`) were already split — only the entry guard was missing. New `internal/cli/status_test.go` table-drives `text`/`json`/`foo`/`""`/case-mismatch through `validateFormat`. **Postmortem lesson:** Sibling subcommands diverging from a canonical pattern is a recurring drift class — when adding a new format-aware subcommand, check that the canonical helper is the first call, not buried after I/O. The drift happened because the format vars were already split, making the missing guard easy to overlook.

- **`ux:e7db1220:json-release-type-as-int`** — done 2026-05-08 — PR #498, merge commit `9b5fa15`. Tier I major (json-stability). `OKDVersion.Type` is `ReleaseType int`; without `MarshalJSON` it serialized as `"release_type": 0` against a documented schema string vocabulary (`stable`/`latest-stable`/`preview`/`latest-preview`/`lts`). Added `MarshalJSON` (delegating to a package-private `labelForReleaseType`) and `UnmarshalJSON` for cache round-trip; old integer-encoded caches gracefully invalidate to a refresh on first load. Snapshot tests in `internal/distribution/okd/releases/types_test.go` lock per-variant marshal output, per-variant unmarshal, full round-trip, and `OKDVersion`-level shape (`"release_type":"latest-stable"`). Duplication with `cli/releases.go::releaseTypeLabel` noted as minor smell — could collapse later. **Postmortem lesson:** Int-enum JSON marshaling is a recurring schema-drift pitfall — when you ship `--format=json`, every typed enum field needs an explicit `MarshalJSON` or string-typed retype, or the schema doc lies on first emit.

- **`obs:41a9d4eb:redact-handler-struct-fields-passthrough`** — done 2026-05-08 — PR #499, merge commit `201b0e0`. Tier I major (redaction-sink). `RedactHandler.redactAny` only matched `*url.URL` userinfo and types implementing `Redacted() any`; raw structs with credential fields under benign keys (e.g. `slog.Any("creds", &credentials.ProxmoxCredentials{...})`) leaked through. The `Redacted()` interface dispatch already worked correctly — `ProxmoxCredentials` already implemented it. The fix was documentation: extended the `RedactHandler` package doc to state the contract ("credential types MUST implement `Redacted() any`; key-based redaction alone cannot protect them"). Added `TestRedactHandler_StructFieldsStrippedViaBenignKey` locking the dispatch path with a `credStub` that has a `Password` field, plus a nil-receiver test. Type-switch defense-in-depth in `redactAny` was rejected — would invert the `logutil → credentials` import direction and create a coupling that prevents `credentials` from ever importing logutil. **Postmortem lesson:** Sometimes the security fix is a doc + test, not code — when the runtime dispatch already handles the case, the gap is the missing contract documentation that lets callers reason about what's protected. Skip code changes that would create import-direction cycles; document the invariant on the producer's package doc instead.

- **`doc:8f46b665:phases-add-step-missing-rerunsafe`** — done 2026-05-08 — PR #500, merge commit `6b1ed33`. Tier I major (readme-drift). `docs/architecture/phases.md` "Adding a new step" section enumerated `NonFatal` and `SkipWhen` but never mentioned `ReRunSafe` — a reader following the doc verbatim would hit `BuildSteps`'s panic ("must declare ReRunSafe") on first run. Inserted a numbered step covering both `ReRunSafeYes` (idempotent default) and `ReRunSafeNo` paired with `AlreadyDone`. Added a one-line callout in the BuildSteps Orchestration paragraph quoting the panic message verbatim so it's discoverable from both doc sides. **Postmortem lesson:** Required-field panics are the doc's contract enforcement — when adding a required field to a struct that has a doc-described initialization pattern, the doc PR must land in the same release as the validation PR or the next reader hits the panic.

- **`tst:39c75e91:promptconfirm-untested`** — done 2026-05-08 — PR #501, merge commit `cea5e6e`. Tier I major (destructive-untested). `promptForConfirmation` in `internal/cli/confirm.go` is the y/N prompt every destructive subcommand (deploy/destroy/cleanup/addon uninstall/update-ingress) calls but had zero direct test coverage. Added `TestPromptForConfirmation` table-driven over 5 cases (y, yes, n, YES, EOF) using the existing `testStdinReader` hook; `TestPromptForConfirmation_CtxCancel` uses `io.Pipe` with an unwritten reader so the goroutine stays blocked, guaranteeing pre-cancelled `ctx.Done` wins the select deterministically; `TestIsConfirmResponse` table locks case-sensitivity (y/Y/yes confirm; YES/Yes/true do NOT). The no-TTY branch is intentionally untested — `term.IsTerminal` would need a hook the production code doesn't expose. **Postmortem lesson:** When testing a `select { stdinCh, ctx.Done }` race, an unwritten `io.Pipe` reader is the cleanest way to deterministically force ctx.Done to win — much less flaky than relying on goroutine scheduling under contention.

- **`tst:e3782ee7:make-executable-untested`** — done 2026-05-08 — PR #502, merge commit `f17dba1`. Tier I major (canonical-helper-untested). `system.MakeExecutable` (the canonical chmod+x helper for downloaded oc and helper binaries running as root under sudo re-exec) had no test. An off-by-one bit-shift refactor would silently land non-executable binaries that fail mid-deploy. Added `TestMakeExecutable` covering 0o600→0o711, 0o644→0o755, content preservation, and missing-path error containing the path. Mode pinning via explicit `os.Chmod` after `os.WriteFile` sidesteps process umask interference. **Postmortem lesson:** When testing file-mode operations on macOS/Linux, always pin the initial mode with `os.Chmod` after `os.WriteFile` — process umask varies across CI runners and changes the implicit mode silently, masking real bugs.

- **`sec:5013fea6:bootstrap-oc-no-signature`** — done 2026-05-08 — PR #503, merge commit `3c29899`. Tier I minor (tls-network). `bootstrapOC` in `internal/distribution/okd/setup/release_extract.go` fetched the expected SHA-256 from `sha256sum.txt` on the same release URL as the tarball — a paired-asset compromise (both files swapped) would pass verification. Pinned `bootstrapOCChecksum = "00c15ce8...e207b7f3"` at compile time (mirrors `yqChecksumsByArch` in `tools.go`); removed the runtime `download.FetchChecksum` call entirely. The compile-time const is now the sole trust anchor; `sha256sum.txt` is no longer fetched. Bumping `bootstrapOCVersion` requires updating `bootstrapOCChecksum` from the new release's `sha256sum.txt` before commit — the comment on the const records the invariant. **Postmortem lesson:** "Pin the version, fetch the checksum from the same URL" is a common but flawed pattern — the trust root must be local-binary or independently signed. Compile-time pinning is the cheapest fix; the abandonment trigger is upstream publishing cosign signatures (then we can verify rather than pin).

- **`sec:8ea706f6:tools-tempdir-non-canonical`** — done 2026-05-08 — PR #504, merge commit `93612f3`. Tier I minor (file-toctou). `installBinary` in `internal/distribution/okd/setup/tools.go` used `os.CreateTemp` + immediate `Close` + later `download.Download` writing to that path — the canonical `system.WriteTempFile` per CLAUDE.md §architecture-notes was bypassed. Refactored to thread `download.Download` through `system.WriteTempFile`'s callback (passing `f.Name()` as `OutputPath`); the create-then-rewrite window closes. The `download.go` package was deliberately not modified — `sec:21dc1103` (PR #495) owned the parallel O_NOFOLLOW change there; the two land cleanly composed. Drive-by replaced `p.Log.Info(fmt.Sprintf("tools: installing %s", spec.name))` with structured `Info("tools: installing", "tool", spec.name)` per CLAUDE.md's redact-handler policy. **Postmortem lesson:** When two parallel items touch related but distinct files, the deliberate "do NOT modify file X" instruction in the planner brief is load-bearing — without it, both planners might converge on the same site and create a merge-conflict-on-develop race.

- **`sec:98723e5d:helm-set-cred-via-argv`** — done 2026-05-08 — PR #505, merge commit `011b691`. Tier I minor (credentials). `installInstance` in `internal/addon/catalog/flux/flux.go` passed `fs.Repository` and other settings via `--set instance.sync.url=<url>` to helm; `--set` values land in `/proc/<pid>/cmdline` and verbatim in the helm release Secret. `ValidateSettings` rejects `https://` URLs with userinfo but `ssh://git+token@host` schemes can still encode tokens. Added `buildInstanceValues` that marshals nested `map[string]any` via `sigs.k8s.io/yaml` (already a direct dep), wrote a 0o600 temp file via `system.WriteTempFile`, and replaced the four `--set` flags with `-f valuesPath`. `defer os.Remove(valuesPath)` cleans up after `helm install/upgrade` returns. `TestBuildInstanceValues` locks the YAML shape (`instance.sync.{url,ref,path}`, `instance.cluster.type`). **Postmortem lesson:** Any helm value containing user-supplied URLs/paths/secrets should go via `-f` not `--set` — argv-mode exposes them in `/proc/<pid>/cmdline` and helm release Secrets, both of which are observable by other processes / cluster-readers.

- **`sec:d9f7733e:debug-bundle-tar-readall`** — done 2026-05-08 — PR #506, merge commit `ca771af`. Tier I minor (file-toctou). `tarDirInto` in `internal/cli/debug_bundle.go` walked srcDir and `io.ReadAll`-ed each file into memory before writing to the tar; must-gather output can be multi-GB and OOM the process. Refactored to stream via `tw.WriteHeader + io.Copy(tw, io.LimitReader(f, maxBundleFileBytes))`; capped per-file at 50 MB; truncated paths bubble back as `[]string` so `bundleMustGather` records them in the manifest entry message. Signature changed from `addFile(string, []byte) → addStream(*tar.Header, io.Reader)`; existing `TestTarDirIntoRejectsSymlinkEscape` migrated to `stubAddStream`; new `TestTarDirIntoTruncatesOversizedFile` asserts cap behavior. **CI fix iteration:** First push made `maxBundleFileBytes` a `const`; the test materialized 50 MB+1024 of zeros into a `bytes.Buffer` via tar, timing out CI's test-go job at ~67s. Converted to `var` and override to 1 KB in the test via `t.Cleanup`. **Postmortem lesson:** Threshold constants that are referenced in performance-sensitive test paths should be `var`, not `const` — letting tests lower them via `t.Cleanup` keeps CI fast without exposing tuning to production callers.

- **`sub:25fa1be8:bypass-canonical-wrapper-ufw`** — done 2026-05-08 — PR #507, merge commit `3079e7b`. Tier I minor (io-handling). `DetectBackend`'s ufw probe in `internal/distribution/okd/firewall/firewall.go:91-106` was the only site in the file using raw `exec.CommandContext + cmd.Output()` instead of the canonical `system.OutputCaptured` — inheriting the parent shell's full env (including credential-bearing AWS_*/GH_TOKEN/GITHUB_TOKEN that the executor allowlist drops) and requiring manual `*exec.ExitError` unwrap for stderr. Replaced with `system.OutputCaptured(ctx, "ufw", "status")`; `SubprocessError.Error()` already embeds the stderr tail so the debug fall-through log loses no diagnostic value. `errors` import dropped (no longer needed). Behavior preserved: non-nil err → fall through to next backend. **Postmortem lesson:** When grepping for "all shellouts use the canonical wrapper", check the file end-to-end for `exec.CommandContext`/`exec.Command` — one outlier site is enough to leak credentials, and outliers tend to be older code paths predating the canonical helper.

- **`ux:8154ab0f:doctor-no-machine-format`** — done 2026-05-08 — PR #508, merge commit `d1eb0d9`. Tier I minor (json-stability). `okdctl doctor` shipped text-only — CI dashboards and the `debug-bundle` collector had to scrape ANSI-coloured tabwriter output. Added `--format=text|json` to `doctorCmd`; `runDoctor` calls `validateFormat` + `quietForJSON` and on the JSON path emits `{"checks":[{"name":"...","severity":"ok|warn|fail","detail":"..."}],"failed":N,"warned":N}` via `writeJSON`. Multi-item checks (e.g. tools-and-packages) expand to `<check>/<item>` notation. `debug_bundle_doctor.go` now invokes `doctor --format json --log-format json --log-level warn`; the bundle stores `doctor.json` instead of `doctor.txt`. Schema documented in `docs/cli/json-schema.md`. **CI fix iteration:** First push missed regenerating the autogenerated `docs/cli/okdctl_doctor.md`; reviewer caught it (would fail `make docs-check`). Ran `make docs` and committed the regenerated reference. **Postmortem lesson:** Cobra metadata changes (`Long`, `Example`, new flag) propagate to autogenerated CLI reference pages — `make docs` is mandatory after any change to a cobra command's user-visible surface, even when the change is "just a flag".

- **`obs:33579dd5:refusing-critical-path-no-target`** — done 2026-05-08 — PR #509, merge commit `263e357`. Tier I minor (field-stability). `services.Dnsmasq` in `internal/distribution/okd/cleanup/services.go` logged `"cleanup: refusing critical path"` three times in close succession (L155/L167/L176) with only `"err"` as a structured attr — the rejected path was encoded in the err chain but not queryable as a discrete field. JSON pipelines couldn't group by which file triggered the refusal. Added `"path", <var>` at each site (configPath, cfg, backup respectively, matching the in-scope variable). No other behavior changes. **Postmortem lesson:** When the same static log message fires from multiple sites with only differing context, the discriminating context must be a structured attr — encoding it only in the err chain forces JSON pipeline consumers into substring matching, which is brittle and discards the typed information.

- **`state:4f69fc9d:rerunsafe-not-enforced`** — done 2026-05-08 — PR #512, merge commit `d3d172b`. Tier I major (phase-idempotency). `internal/distribution/step.go:228-251` BuildSteps panicked on `ReRunSafeUnset` but never propagated the value into builtStep, and Orchestrator.Run never branched on it — `ReRunSafeNo` was decorative metadata unless the StepDef also supplied `AlreadyDone`. Made BuildSteps panic when `ReRunSafeNo + AlreadyDone == nil`, mirroring the existing unset panic shape. Five setup steps (download-tools, generate-config, generate-manifests, generate-ignition, deploy-ignition) gained AlreadyDone hooks based on observable local file state; four steps (StepBuildISOs, StepDeployInfra, StepInstallAddons, StepDestroyInfra) flipped to `ReRunSafeYes` because their exec bodies are already internally idempotent. **Lint follow-up commit `bf84bff`:** the new `bootstrap.ign` reference at steps.go:225 pushed the literal's count from 2 to 4, tripping `goconst`'s 3-occurrence threshold; extracted `ignitionFilenames` package var (already had master/worker/bootstrap shape elsewhere) and reused it. **Postmortem lesson:** When adding a new occurrence of any string literal that already appears in the package, run `golangci-lint run ./...` from a Linux runner before pushing — darwin's lint config can pass while Linux's `goconst` (and `//go:build linux`-gated rules) trip on the extra reference.

- **`api:4f69fc9d:rerunsafe-declarative-only`** — done 2026-05-08 — PR #512, merge commit `d3d172b`. Tier I major (exported-surface). Paired with `state:4f69fc9d` above. `StepDef.ReRunSafe` was enforced at declaration time but never consulted at execution time — StepBuilder discarded it and Orchestrator.Run never branched on it. Added `reRunSafe ReRunSafety` to `StepBuilder` plus a `SetReRunSafe` setter; BuildSteps wires `SetReRunSafe(d.ReRunSafe)` into every builder; `Build()` copies into `builtStep.reRunSafe`; builtStep exposes `ReRunSafe() ReRunSafety` so a future Orchestrator change can branch on it without re-plumbing. New tests `TestBuildSteps_PanicsOnReRunSafeNoWithoutAlreadyDone` and `TestBuildSteps_AcceptsReRunSafeNoWithAlreadyDone`. **Postmortem lesson:** "Decorative metadata enforced only at declaration time" is a recurring anti-pattern — once a field is required at construction, it should also be load-bearing at runtime, otherwise authors learn it's optional in spirit and start working around the panic. Pair every required-field validation with a runtime read.

- **`sub:8ea706f6:coreutil-lsb-release`** — done 2026-05-08 — PR #513, merge commit `e0fd7af`. Tier I minor (coreutils-shellout). `installHashiCorpDebianRepo` in `internal/distribution/okd/setup/tools.go:321-326` forked `lsb_release -cs` to read the Debian codename, but `/etc/os-release` `VERSION_CODENAME` exposes the same value and is already parsed by `internal/platform/platform.go::Detect`. Added `Codename string` to `platform.OS` populated from `fields["VERSION_CODENAME"]`; `installTerraform` now passes `p.OS.Codename` to `installHashiCorpDebianRepo` which gates with an explicit empty-codename error. Dropped `os/exec` import from tools.go. Removes the runtime dep on `lsb_release` (a Python script that ships separately on Debian/Ubuntu and is absent on minimal installations). **Postmortem lesson:** `lsb_release`-based shellouts are a recurring footgun — the program is a Python script and is not present on minimal/container distributions. Always read /etc/os-release directly when you need codename/version data.

- **`smell:8ea706f6:abstraction-table-meta`** — done 2026-05-08 — PR #513, merge commit `e0fd7af`. Tier I minor (helper-package-no-value). `binaryToolMeta` in `internal/distribution/okd/setup/tools.go:89-143` was a 25-LOC `map[externalTool]struct{...}` indexed by `externalTool` whose only consumer was `installTool` with three concrete entries (yq/helm/sops). Replaced with a `switch tool` in installTool that builds the `binaryInstallSpec` inline per case (~6 LOC each); the unknown-tool fallthrough returns an error instead of the prior silent warn-and-skip. Total LOC decreases by ~10. The `fmt.Sprintf("tools: %s already installed", tool)` log call also migrated to structured attrs (`"tool", string(tool)`) per CLAUDE.md's no-`fmt.Sprintf`-in-log policy. **Postmortem lesson:** When a lookup table has only three entries and one consumer, the indirection costs more than it pays — a switch with explicit cases reads better and turns "unknown key" from a runtime warn into a returned error. Map-indexed dispatch is for variable-length plugin-style sets, not finite enums with three members.

- **`sec:25fa1be8:firewall-haproxy-port-only-tcp`** — done 2026-05-08 — PR #514, merge commit `46608ba`. Tier I suggestion (input-validation). `haproxyPortNumbers` in `internal/distribution/okd/firewall/firewall.go:52-66` was `map[int]bool` keyed only by port number; `HAProxyFrontendPorts()` filtered `OKDRequiredPorts` by both Number and `Protocol == protoTCP`. The structure was a future-maintainer footgun — a UDP rule on the same number would silently pass the number filter. Replaced with `haproxyFrontends []Port` carrying explicit `{Number, Protocol: protoTCP, Description}` for KubeAPIPort/22623/80/443; `HAProxyFrontendPorts()` returns a defensive copy of the static slice (no longer iterates OKDRequiredPorts). New test sub-assertion verifies callers cannot mutate the package var via the returned slice. **Postmortem lesson:** When a filter relies on multiple fields but only some are encoded in the data structure, the filter logic carries the rest as implicit knowledge — making the structure carry all the discriminating fields explicitly removes the silent-passthrough class of bugs.

- **`api:21dc1103:options-struct-vs-functional`** — done 2026-05-08 — PR #515, merge commit `2f87e24`. Tier I minor (option-consistency). `download.Options`, `download.ExtractOptions`, and `cleanup.Options` passed an optional `*slog.Logger` as a struct field with a per-call `getLogger()` shim, while every sibling package (`executor`, `terraform`, `proxmox`, `addon`, `cluster`, `phase`) used functional options. Renamed `download.Download(ctx, *Options) error` → `download.Fetch(ctx, url, dst string, opts ...Option) error` with `WithChecksum`/`WithDescription`/`WithTimeout`/`WithOverwrite`/`WithLogger`; `ExtractTarGz(ctx, ExtractOptions)` → `ExtractTarGz(ctx, archivePath, destDir string, opts ...ExtractOption)`. `cleanup.Options` shed Logger; `cleanup.Execute` gained `...Option` with `cleanup.WithLogger`. `logutil.OrNop` now applies once at construction inside `WithLogger` instead of on every log call. All seven setup/cleanup/cli call sites migrated; same-package canSkipDownload tests use the new lowercase `dlConfig`/`extractConfig` fields. **Postmortem lesson:** Inconsistent option shapes across sibling packages erode reviewer attention — when the audit-api-design audit flags this as "option-consistency", the cheapest fix is to migrate the outliers to match the prevailing shape, not to debate which shape is "best". Functional options were already the majority pattern.

- **`smell:d31d1b9d:stringly-typed-enum`** — done 2026-05-08 — PR #516, merge commit `8bdd136`. Tier I minor (magic-strings). `runDescribeNode` in `internal/cli/status.go:297-314` rendered node readiness as raw `"True"`/`"False"` literals via an ad-hoc if/else over `n.isReady()`, even though `phase.ConditionStatusTrue`/`ConditionStatusFalse` already model the same enum and the file already imports `phase`. Replaced with `string(phase.ConditionStatusFalse)` / `string(phase.ConditionStatusTrue)` inline (no helper — lighter touch); output is byte-for-byte identical. New `TestConditionStatusLiterals` guards against future enum drift by asserting both constants stringify to "True"/"False". **Postmortem lesson:** Stringly-typed-enum sites flagged by audit-code-smells are usually one-line fixes when the enum already exists in scope; the cost is the test that locks the constant value, not the substitution.

- **`dep:33ef32bf:transitive-narrow-uuid`** — done 2026-05-08 — PR #517, merge commit `eb6e8f5`. Tier I suggestion (transitive-weight). `github.com/google/uuid v1.6.0` was a direct dep used at two call sites (debug_bundle.go bundleID, root.go run-ID telemetry) — UUID v4 from `crypto/rand` is ~15 LOC stdlib. Added `system.NewUUIDv4() string` using `crypto/rand` + `encoding/hex` to format a canonical 36-char dashed v4 with version bits (b[6]=0x40) and variant bits (b[8]=0x80) per RFC 4122 §4.4; panics on `crypto/rand` failure to mirror google/uuid.NewString's contract (no signature churn at the two call sites). New `TestNewUUIDv4` validates the format regex and uniqueness across consecutive calls. google/uuid stays in go.sum as a transitive via go-proxmox/diskfs. Also bumped some indirect deps (go-test/deep 1.0.8→1.1.1, ulikunitz/xz 0.5.11→0.5.15, klauspost/compress) as side-effects of `go mod tidy`. **Postmortem lesson:** Before adding a new direct dep, check whether stdlib already covers the use case at <20 LOC — `crypto/rand` + bit-twiddling is shorter than the import-and-version-pin hygiene cost of a third-party UUID library when uniqueness is the only requirement (no namespace UUIDs, no v3/v5).

- **`sec:27088eab:ssh-strict-host-key-tofu`** — done 2026-05-08 — PR #518, merge commit `7470b4f`. Tier I minor (tls-network). `SSHRun` and `SSHRunArgv` in `internal/distribution/okd/phase/ssh.go:31-57` installed `StrictHostKeyChecking=accept-new` for every Proxmox-bastion ssh hop — a first-deploy MITM permanently locked in an attacker's host key. Added `proxmox.ssh_host_fingerprint` config field accepting standard `SHA256:<base64>` values from `ssh-keygen -lf` or the Proxmox UI. New `internal/sshpin` package runs `ssh-keyscan` (NO `-H`), parses each key with `ssh.ParseAuthorizedKey`, and compares `ssh.FingerprintSHA256(key)` against the configured pin — explicitly avoiding the whole-stdout SHA256 approach two prior PRs (#117 closed 2026-04-22, #142 closed 2026-04-26) found non-deterministic due to the random salt under `-H`. On match, sshpin writes a temp known_hosts file and SSHRun/SSHRunArgv switch to `StrictHostKeyChecking=yes` against it. When unset, observed fingerprints log at WARN and accept-new TOFU still applies. All five SSH call sites (pvesh, iso_cleanup, upload, proxmox.Provider) thread the resolved known_hosts path. The Tier H sibling `sec:27088eab:input-kubeconfig-not-resolved` (whose body text was identical to this finding's despite the misleading label) closes transitively via the same fix. **Postmortem lesson:** Per-key `ssh.FingerprintSHA256` over parsed `ssh.PublicKey` values is the only correct approach to host-key pinning — hashing the raw `ssh-keyscan` stdout is fundamentally broken because of `-H`'s random salt and banner-line ordering. The first two attempts both failed on this; pinning the `golang.org/x/crypto/ssh` package's parser as the trust boundary is what makes the third attempt sound.

- **`api:beabab0c:phase-new-positional-args`** — done 2026-05-08 — PR #519, merge commit `3e8c4fb`. Tier I minor (option-consistency). All five sibling phase constructors (setup/install/postinstall/destroy/cleanup) took `New(exec, logger, version)` while their shared base `phase.NewBasePhase` and the parent `okd.New` already used functional options. The split forced okd.go to construct positionally then immediately write exported fields — `setupPhase.Recorder`, `installPhase.Recorder`/`Reporter`, `postPhase.Recorder` (L138, L147-L148, L156). Migrated each `New` to `(version string, opts ...phase.BasePhaseOption)`; Reporter moved from `install.Phase` into `BasePhase` via a new `WithReporter(logutil.ProgressReporter)`; added `WithVersion` as scaffolding. Version stays positional — empty would silently corrupt provenance tags and 10+ test sites already pass it positionally to NewBasePhase. okd.go's post-construction field writes collapse into option calls. Test sites (haproxy_test, monitor_test, flux_test) updated to the option shape. **Postmortem lesson:** The "positional args mid-package, options elsewhere" split is the API-design anti-pattern this audit catches — once two sibling constructors disagree, every consumer learns to translate, and the translation layer becomes the bug surface. Pick one shape per logical family and migrate the outliers.

- **`sec:27088eab:input-kubeconfig-not-resolved`** — done 2026-05-08 — PR #518 (transitive). Tier H minor (input-validation). The Tier H entry's body text described the exact same accept-new TOFU surface in `internal/distribution/okd/phase/ssh.go` as the Tier I `ssh-strict-host-key-tofu` entry — a duplicate finding from a different audit run with a misleading label ("input-kubeconfig-not-resolved" was a stale slot name unrelated to the actual problem). PR #518's `sshpin` fix closes it without separate work. **Postmortem lesson:** When the same audit ID prefix appears in two tiers, compare the *body* text not just the labels — different label slugs can hide identical findings, and resolving one PR can transitively close the other. Mark the duplicate as done at the same time, not after a separate pass.

- **`ux:aa84670c:version-printf-not-via-cmd-out`** — done 2026-05-08 — PR #520, merge commit `805e839`. Tier I suggestion (streams). `versionCmd.Run` and other cobra Run/RunE handlers wrote via `fmt.Printf`/`fmt.Println` directly to `os.Stdout`, ignoring `cmd.OutOrStdout()` — making cobra-test `cmd.SetOut(buf); cmd.Execute()` impossible (existing tests had to swap `os.Stdout` globally). Migrated every `fmt.Println(X)`/`fmt.Printf(X, ...)` inside a cobra RunE/Run to `fmt.Fprintln(cmd.OutOrStdout(), X)`/`fmt.Fprintf(cmd.OutOrStdout(), X, ...)`. Helper functions that print summaries (`validateConfig`, `executeFullDeployment`, `runDeployDryRun`, `runFullDeployment`, `saveConfig`, `showExitSummary`, `printResult`) gained an `io.Writer` parameter; runDeploy/runDoctor capture `cmd.OutOrStdout()` once at the top of the handler and pass it through. `fmt.Fprintf(os.Stderr, ...)` writes (kubeconfig.go:72,123) left as-is — explicit stderr is correct. **Postmortem lesson:** `cmd.OutOrStdout()` discipline is the only way to make cobra commands testable without `os.Stdout`-swap monkey-patching — when a helper needs to print summary text, prefer threading `io.Writer` over reaching for `os.Stdout`, even if every current caller would pass `os.Stdout` anyway.

- **`ux:e7db1220:format-vs-output-flag-name-drift`** — done 2026-05-08 — PR #521, merge commit `51df2ad`. Tier I suggestion (flag-conventions). okdctl split the kubectl/oc `-o`/`--output` namespace: format-selector flags were `--format` on `releases list/show`, `status`, `describe node/addon`, `doctor`, `addon list/verify`; file-destination flags were `--output`/`-o` on `deploy`, `kubeconfig`, `debug-bundle`. A user typing `okdctl status -o json` got a usage error because `-o` was reserved for the file-destination namespace on the wrong commands. Took Option A (pre-1.0 break): renamed all 8 format-selector flags to `--output`/`-o`; renamed all 3 file-destination flags to `--output-file` (no shorthand). Variable names follow: `<x>Format` → `<x>Output`. Convention recorded in CLAUDE.md §Architecture-notes. `make docs` regenerated 11 autogenerated CLI reference pages. `internal/cli/debug_bundle_doctor.go` updated to invoke `doctor --output json`. **Postmortem lesson:** When a CLI namespace conflict between conventions exists pre-1.0, take the breaking rename rather than adding a hidden alias — aliases proliferate, the canonical name becomes ambiguous, and operator muscle memory drifts. The pre-1.0 break costs one PR; the alias would cost permanent doc complexity.

- **`obs:6424733c:fmt-sprintf-message-pattern`** — done 2026-05-08 — PR #522, merge commit `c07157e`. Tier I major (field-stability). 85 `tui.X(fmt.Sprintf(...))` and `<logger>.X(fmt.Sprintf(...))` log sites across `internal/cli/`, `internal/distribution/okd/`, `internal/addon/`, `internal/download/`, and `internal/distribution/okd/firewall/` interpolated values into the message string, bypassing `RedactHandler` (which only scrubs attr keys/values, not pre-rendered messages). Migrated every site to the structured-attr form: static lowercase message + typed `tui.LF("key", val)` or `"key", val` attrs. Three files (`postinstall/bootstrap.go`, `postinstall/steps.go`, `cli/cleanup.go`) dropped the `fmt` import after the last log-call `fmt.Sprintf` disappeared. fmt.Errorf wraps, cobra command output, env-var construction, and URL/path formatting were intentionally left unchanged — only log-record messages migrated. **Rebase fix during merge:** `cli/helpers.go:39` had a textual conflict where W-K (`refactor/ux-e7db1220-output-flag`) renamed `--output` to `--output-file` in the same line W-L was migrating from `fmt.Sprintf` to structured attrs; resolved by combining both: `tui.Info("run 'okdctl deploy --output-file <file>' to create it", tui.LF("file", configFile))`. **Postmortem lesson:** When two parallel sweeps target the same line (one renaming a flag, one converting to structured attrs), the rebase merge is mechanical — keep both signature changes and combine the static-message text. Plan parallel sweeps with explicit "do NOT touch line X" instructions when the surface overlaps to avoid the rebase round-trip.

- **`mod:262af6e4:use-slices-contains`** — done 2026-05-09 — already on develop in commit `239959e` (refactor: prefer slices.Contains over hand-rolled loops). Tier J minor (slices-maps). The Tier J audit (2026-05-08) flagged `internal/distribution/okd/cleanup/cleanup.go::Kind.IsValid` for hand-rolling a contains loop over `ValidKinds()`. Discovered during planner-phase: commit `239959e` had already collapsed the body to `slices.Contains(ValidKinds(), k)` — the audit was generated against a pre-`239959e` snapshot. No code change required; archive on inspection. **Postmortem lesson:** Audit findings can lag develop by a commit when the audit run preceded a sweeping refactor; the planner's read-only "verify the evidence still applies" step caught it before any worktree was claimed. Always grep the cited file:line first.

- **`obs:beabab0c:phase-attr-missing-on-setup`** — done 2026-05-09 — PR #524, merge commit `a1af032`. Tier J minor (field-stability). `internal/distribution/okd/setup/phase.go::New` was the only of five sibling phases (install/postinstall/destroy/cleanup/setup) not enriching `bp.Log` with `("phase", "setup")` after `NewBasePhase`. JSON log filtering on `phase=setup` returned an empty result while the other four phases emitted the attr consistently. One-line fix between `NewBasePhase` and the platform-detection call, matching install/phase.go:L91 verbatim. **Postmortem lesson:** Field-stability findings on log attrs are the kind of drift that's invisible in code review but obvious to operators filtering JSON streams; sibling-phase comparisons (4/5 emitting one attr, 1/5 not) are the cheapest detection.

- **`sec:c8b28673:tar-mode-trust`** — done 2026-05-09 — PR #525, merge commit `367ef99`. Tier J minor (file-toctou). `internal/download/extract.go:110` extracted regular files with `os.FileMode(header.Mode & 0o777)` — preserving setuid (0o4xxx; bit-9), setgid (0o2xxx), and sticky (0o1xxx) bits from any tar header. Tightened the file mask to `& 0o755` matching the directory branch on L89. okdctl extracts cosign-anchored release tarballs from `quay.io/okd/scos-release`, so reachability is bounded by upstream poisoning, but the helper is generic and the previous mask preserved 0o2xxx bits for any future caller. New `TestExtractTarGz_ModeMask` covers 0o2755 + 0o4755 + 0o755 + 0o644 entries via subset-check (`got &^ wantMode != 0`) so process umask doesn't flake the assertion across runners. **Postmortem lesson:** When testing file-mode operations, assert "no bits above wantMode are set" rather than strict equality — process umask reduces but doesn't add bits, so the subset-check is the correct invariant.

- **`iac:b803fcb7:tfsec-soft-fail`** — done 2026-05-09 — PR #526, merge commit `5671fa1`. Tier J major (hcl-provider-hygiene). `.github/workflows/ci.yml:114-117` ran `aquasecurity/tfsec-action` with `soft_fail: true`, rendering scan-terraform a cosmetic check — findings logged but the job always exited 0, violating CLAUDE.md §Tooling "all CI checks must be green before merging". Dropped `soft_fail` and seeded `infrastructure/terraform/.tfsec/config.yml` with a fail-closed policy header documenting the suppression-by-rule-code template. **Two CI iterations during merge:** initial commit had a comment-only YAML body, which yamlfmt v0.14.0 (CI's pinned version) rejected for missing the `---` document-start marker; second push added `---`; third push added the trailing two blank lines yamlfmt expected after the marker. Local yamlfmt v0.20.0 was lenient where CI's v0.14.0 was strict. **Postmortem lesson:** When local-vs-CI tooling versions differ, the lenient local version masks formatter expectations; either pin local to match CI or run the actual CI command in dry-run before pushing. The `.yamlfmt` config (`include_document_start: true`) was the spec all along — the local pass was the false signal.

- **`iac:e076e43c:cosign-optional-when-absent`** — done 2026-05-09 — PR #527, merge commit `9723e7c`. Tier J major (install-sh-integrity, seam→audit-security). `scripts/install.sh:141-146` silently downgraded to sha256-only trust when cosign was absent and `INSECURE` was unset — a single CDN compromise of both the binary and `SHA256SUMS` would bypass verification (CWE-494). Restructured the gate so cosign is now a hard prerequisite mirroring the `sha256sum` require at L49-53; `INSECURE=1` is the explicit operator opt-out, with a loud `red()` warning naming both the trust degradation and the cosign install URL. The download-time else branch (L141-146) for the silent-downgrade case becomes unreachable. **Postmortem lesson:** "Optional cosign" is a Trojan horse for trust assumptions — the silent-fallback path is exactly where the supply-chain attacker lives. Either require the verification tool or require an explicit env var to opt out; never both-implicit.

- **`ux:e7db1220:releases-validate-flag-error-not-usageerror`** — done 2026-05-09 — PR #528, merge commit `f81e9b1`. Tier J major (exit-codes, seam→audit-errors). `internal/cli/releases.go::validateChannel` and `validateFormat` returned plain `fmt.Errorf` for invalid `--channel`/`--output` values — they reached `exitCodeFor` without satisfying `errors.As(*errtypes.UsageError)` and fell to exit 1 instead of 64 (EX_USAGE) per the documented exit-code taxonomy. Wrapped both validator returns in `&errtypes.UsageError{Msg: fmt.Sprintf(...)}` and added 4 tests asserting `errors.As` resolves correctly for both validators. Distinct from the already-archived `ux:e7db1220:format-vs-output-flag-name-drift` (PR #521) — that finding renamed the flag namespace; this one fixes the typed-error wrapping for the same flag. **Postmortem lesson:** Audit IDs reuse the same prefix across runs when the underlying audit class is the same surface (`ux:e7db1220` for the releases command) but the sub-id differentiates the specific defect. Don't assume "this prefix already merged" without comparing sub-ids.

- **`sec:bdf5a873:cleanup-symlink-traversal`** — done 2026-05-09 — PR #529, merge commit `08e0f8a`. Tier J minor (file-toctou). `internal/distribution/okd/cleanup/artifacts.go::SafeRemoveWithLogger` ran `os.Stat` (which follows symlinks) then `os.RemoveAll` — a symlink at `<workDir>/okd-install -> /etc` passed the `refuseCriticalPath` allowlist check (Base is `okd-install`) and would unlink the link itself, even though Go's `os.RemoveAll` doesn't follow into the pointed-at tree. Added an `os.Lstat` + `ModeSymlink` refusal head matching the patterns in `credentials/envfile.go::WriteEnvFile` (L52-58) and `runlock/runlock.go::Acquire` (L40-50). New "refuses symlink target and link survives" subtest asserts the link is preserved after the call returns an error. **Postmortem lesson:** `os.Stat` vs `os.Lstat` is the canonical TOCTOU distinction — when the caller's intent is "operate on the literal path, not what it points to", Lstat is the right primitive. Three okdctl sites now share the same Lstat-refuse pattern; the doc-comment on each links to the others for reviewer-discoverability.

- **`sec:e3782ee7:atomicwrite-symlink-race`** — done 2026-05-09 — PR #530, merge commit `493c505`. Tier J major (file-toctou). `internal/system/fs.go::AtomicWrite` opened a temp file and renamed it without any symlink check on the destination — under sudo re-exec, an attacker-planted symlink at the destination path could redirect a kubeconfig/.env/install-config.yaml write through to a different location. `os.Rename`'s replace-target semantics close the inner-file race, but the directory-component race remained. Added an `os.Lstat`-then-refuse head returning `&errtypes.AuthError{Err: os.ErrPermission}`, mirroring `credentials/envfile.go::WriteEnvFile`. New `TestAtomicWrite_RefusesSymlinkTarget` asserts both the typed error and that the symlink target was not overwritten. **Postmortem lesson:** When a low-level primitive (AtomicWrite, RemoveAll, OpenFile) protects all higher-level wrappers automatically, the right place to add the symlink check is the primitive itself — every wrapper inherits the protection without per-caller changes. AtomicWriteString automatically benefits from this.

- **`sec:eb479d86:ssh-pin-bypass`** — done 2026-05-09 — PR #531, merge commit `10d35ef`. **Tier J blocker** (tls-network). `internal/distribution/okd/setup/upload.go::uploadISOsViaSCP` hard-coded `StrictHostKeyChecking=accept-new` and ignored the `knownHostsPath` that `UploadCustomISOsToProxmox` already obtained from `sshpin.Verify` — defeating the host-key pin on the very phase that uploads custom ISOs containing per-node ignition kargs. Threaded `knownHostsPath` into `uploadISOsViaSCP` and branched on it: non-empty → `UserKnownHostsFile=<path>` + `StrictHostKeyChecking=yes` + `BatchMode=yes` (mirroring `phase/ssh.go::sshBaseArgs` exactly); empty → `StrictHostKeyChecking=accept-new` + `BatchMode=yes` (preserves TOFU for unconfigured operators). Test split into pinned-vs-unpinned subtests; existing `spaceInFilename` test updated to pass empty-string `knownHostsPath`. **Postmortem lesson:** When a security helper exists in the codebase (`sshpin.Verify`), every related operation must thread its output — not every call site, every operation. The grep should be "where do we call out to ssh/scp" not "where do we read the fingerprint config"; the latter only finds the current callers.

- **`sec:06f00bcb:auth-tree-in-webroot`** — done 2026-05-09 — PR #532, merge commit `5d04ee8`. **Tier J blocker** (file-toctou). `internal/distribution/okd/setup/apache.go::DeployToWebServer` copied the openshift-install `auth/` tree (kubeconfig + kubeadmin-password) into the apache DocumentRoot at `webRoot/auth/`, alongside the ignition files. The deployment relied entirely on source mode bits (0o600) to keep apache from serving them — a future schema bump in openshift-install with more permissive modes, an operator running `chmod -R go+r` to debug, or any DocumentRoot override re-rendering perms would turn the kubeadmin password into a bastion-network HTTP fetch. Removed the auth copy and the `copyAuthTree` helper entirely. Verified by grep that `install.SetupClusterAccess` (`install/flux.go:72`) and `postinstall.verifyKubeVIPAPIHealthBootstrap` (`postinstall/verify.go:226`) both read kubeconfig from `clusterDir/auth/kubeconfig` directly — no caller needed the tree under apache. **Postmortem lesson:** Defense-in-depth on file modes is the wrong primitive when the root cause is "secrets in DocumentRoot"; the only safe fix is "secrets never enter DocumentRoot". Followed the planner's caveat: a `<Location /auth>Require all denied</Location>` Apache deny rule was deliberately not added (no caller needs the tree), keeping the surface minimal.

- **`con:0b188cab:retry-eats-non-retryable`** — done 2026-05-09 — PR #533, merge commit `e2cc13c`. Tier J major (time-sleep-retry). `internal/addon/helpers.go::RetryDefault` swallowed every `fn() error` and consumed the full ~35s backoff budget on permanent failures (auth denied, missing oc binary, malformed addon manifest) — the `//nolint:nilerr` directive documented the behaviour as intentional but it was a defect. Added `addonIsRetryable(error) bool` mirroring `internal/download/retry.go::isRetryable`: `context.Canceled`, `context.DeadlineExceeded`, `exec.ErrNotFound`, typed `*errtypes.ConfigError`, and `*errtypes.AuthError` abort immediately; everything else (transient executor.ExitError, connection errors) still retries through the full budget. New `Test_addonIsRetryable` table covers 11 cases including wrapped forms. **CI nit during merge:** staticcheck S1008 flagged the trailing `if errors.As(err, &authErr) { return false }; return true` block; collapsed to `return !errors.As(err, &authErr)` in a follow-up commit. **Postmortem lesson:** When you write a guard chain that ends with a yes/no return on the last predicate, staticcheck S1008 is a free signal — the trailing `if` is always reducible to a single negation; CI catches this pattern automatically once the lint chain hits it.

- **`state:fb54208a:postinstall-bootstrap-cleanup-nonfatal`** — done 2026-05-09 — PR #534, merge commit `2e83553`. Tier J major (phase-idempotency). `internal/distribution/okd/postinstall/steps.go::StepCleanupBootstrap` was `NonFatal: true` with a `phase.WarnOnError` callback — when `terraform apply -target=bootstrap` failed (e.g. proxmox API auth glitch), postinstall continued and reported "deployment complete" while the bootstrap VM was still alive, eating control-plane resources and risking etcd quorum confusion. Removed `NonFatal: true` (default-fatal) and dropped the now-misleading "non-critical" `OnError` callback — Orchestrator.Run propagates the error, p.Configure returns it, and `executeFullDeployment` prints the actionable failure message. The other three postinstall steps (`StepVerifyKubeVIP`, `StepDeployProductionDNS`, `StepInstallAddons`) remain non-fatal as designed. **Postmortem lesson:** `NonFatal: true` is a strong claim — "this step's failure does not invalidate the deploy". Bootstrap teardown failures violate that claim because the residual resource (live bootstrap VM) materially affects every subsequent operation. The right shape is fatal-with-clear-error-message, not non-fatal-with-summary-warn.

- **`tst:bfdaf5e3:cred-bytes-type-untested`** — done 2026-05-09 — PR #535, merge commit `85974e1`. **Tier J blocker** (cred-path-untested). `internal/config/secret.go::SecretBytes` is the credential wrapper for `ProxmoxConfig.Password` and `APIToken`; its `String()=="[redacted]"` invariant and the `Set` zeroize-prior-buffer behavior were both unguarded — a future refactor that dropped `Redacted()` or returned `Bytes()` from `String()` would silently leak credentials through every `fmt.Sprintf` log site that takes a config value. Added `internal/config/secret_test.go` with 6 tests covering: `String` redacts on populated and zero buffers; `Set` zeroes the prior backing array (verified by capturing the old slice header before re-Set); `%s/%v/%+v` fmt verbs all redact; `Redacted()` returns `[redacted]`; `IsEmpty` toggles around Set/Zeroize; `Bytes()` alias is wiped by `Zeroize` (caller-must-not-retain contract). Coverage floor for `internal/config` raised 10 → 12 (measured 13.9%, margin below). **Postmortem lesson:** Coverage floors raised aggressively (e.g. 10 → 90) fail CI on first push because the package's overall coverage is still dominated by uncovered sibling files (cluster.go, validators.go); raise floors incrementally, just below the new measured value, so future regressions are caught without false-positive blocking.

- **`state:15ba17da:destroy-iso-cleanup-before-tf`** — done 2026-05-09 — PR #536, merge commit `e80d7aa`. Tier J major (destroy-safety). `internal/distribution/okd/destroy/steps.go::StepCleanupFirewall.SkipWhen` gated only on `opts.SkipFirewall` and `firewall.DetectBackend == None`. After a `terraform destroy` failure that left VMs alive, firewall rules were still removed — severing access to the live VMs. Added `t.terraformFailed()` to the SkipWhen disjunction and broadened the SkipReason to "firewall cleanup disabled, terraform owns live vms, or no active backend". The existing `WorkOnly` cleanup-Kind downgrade (steps.go:L122-126) already preserves haproxy/dnsmasq paths in StepCleanupFiles when terraformFailed() is true — only the firewall step needed the additional guard. The trackSkip("firewall", ...) recording captures "firewall" in the summary's `skipped` attr, so operators see the preservation in the structured log. **First attempt regression:** initial commit also added a standalone `Warn("preserved haproxy/dnsmasq/firewall for retry")` after the StepPrintSummary switch — broke `TestDestroySteps_FailurePath` because the test's last-record assertion captured my new Warn instead of the existing failures Warn. Removed the standalone Warn; the structured `skipped` attr already conveys the preservation. **Postmortem lesson:** When extending an existing summary with a new conditional log, check whether existing tests assert on the ordering or last-record of records — adding to the tail can displace the previously-last record and break the assertion. Prefer using existing structured attrs (skipped, failures) over adding new top-level Warn records when the data is already in the orchestrator's tracker.

- **`obs:5013fea6:stderr-attr-leaks-subprocess-output`** — done 2026-05-09 — PR #537, merge commit `67511ac`. Tier J major (redaction-sink, seam→audit-errors). Three log sites passed raw subprocess Stderr under the benign attr key `"stderr"` — RedactHandler's secretKeyFragments don't match `stderr` and the value was a plain string so the `Redacted() any` switch in `redactAny` was never reached. The most exposed call (`oc adm release extract` at `internal/distribution/okd/setup/release_extract.go:123`) can carry `--registry-config` paths or partial pull-secret excerpts on auth failure; the other two sites are `apache.go:113` (semanage) and `update_ingress.go:569` (oc rollback). Added `logutil.RedactableStderr` (named string with `Redacted() any` returning the first 200 + last 200 chars when text exceeds 400) and routed all three sites through it. Tests assert the truncation marker appears and the secret-shaped middle section never reaches the JSON sink. **Postmortem lesson:** Type-driven redaction (Redacted() any) is the right primitive when the attr key would otherwise be flagged as benign — extending secretKeyFragments to include "stderr" would over-redact legitimate kubectl/terraform diagnostics where the value isn't sensitive. The named-string-with-method pattern lets each call site opt in.

- **`state:c19ee328:setup-installer-already-done-only-bootstrap`** — done 2026-05-09 — PR #538, merge commit `b4e811d`. Tier J major (phase-idempotency). `StepGenerateManifests.AlreadyDone` keyed on the `manifests/` directory existing — but `openshift-install create manifests` can exit non-zero mid-write, leaving a partial directory the next run sees as "already done", skipping regeneration and producing malformed ignition. `StepGenerateIgnition`'s per-`.ign`-file check had the same shape. Added `ManifestsSentinel` (`<clusterDir>/manifests/.complete`) and `IgnitionSentinel` (`<clusterDir>/.ignition.complete`), both written via `system.AtomicWrite` after the success branch in `GenerateManifests` and `GenerateIgnitionConfigs`. AlreadyDone now keys on the sentinel, mirroring the `install-config.yaml.backup` pattern from `StepGenerateConfig`. **Postmortem lesson:** Directory-existence as an "already done" signal is unsafe whenever the directory is populated incrementally — the failure mode of partial population looks identical to completion. The sentinel-after-success pattern is the canonical fix; pair it with `system.AtomicWrite` so the sentinel itself can't land in a partial state.

- **`sec:5e892064:checksum-no-signature`** — done 2026-05-09 — PR #539, merge commit `7c3e4b7`. Tier J major (tls-network). `internal/download/checksum.go::FetchChecksum` downloaded the SHA-256 file from the same origin as the artifact — a single CDN compromise that swaps both files passed verification (CWE-494). Mirrored the existing `yqChecksumsByArch` pattern: added `helmChecksumsByArch` and `sopsChecksumsByArch` maps with SHA-256 values fetched at pin-update time (verified live against `get.helm.sh` and `github.com/getsops/sops/releases/download` before commit), switched `toolHelm`/`toolSops` to `embeddedChecksum` so the runtime `FetchChecksum` call is no longer reachable for these two tools. **CI nit:** initial commit triggered goconst (`amd64`/`arm64` literal had 3 occurrences across the three checksum maps); follow-up commit extracted `archAMD64`/`archARM64` constants. **Postmortem lesson:** When extracting a constant to satisfy goconst, name it for the domain concept (e.g. `archAMD64`) not the literal value — a typo on either side becomes a compile error rather than a silent miss, which is the same property goconst is trying to enforce.

- **`sec:40d315ad:flux-keyscan-tofu`** — done 2026-05-09 — PR #540, merge commit `7eac12f`. Tier J major (tls-network). `flux.go::createDeployKeySecret` ran `ssh-keyscan` and baked the result directly into the flux-system Secret as `known_hosts` — no fingerprint pin, no fallback. An on-path attacker during deploy substituted their host key and became the perpetual trust anchor for every flux git pull. Added `addons.flux.settings.git_host_fingerprint` setting and `verifyKeyscanFingerprint(keyscanOut, host, expected, log)` helper using `golang.org/x/crypto/ssh` (already in go.mod via other paths). When set, the helper parses each authorized_keys-format line and matches `ssh.FingerprintSHA256(key)` against the configured pin; mismatch returns an error naming both expected and observed values. When unset, logs a Warn naming the observed fingerprints so the operator can pin on the next deploy. The local helper avoids inverting the addon→sshpin import direction; sshpin's parseAndMatch is unexported and its only public entry point Verify re-runs its own keyscan, which would bypass `env.Exec`. **Postmortem lesson:** Cross-package helper extraction is tempting but pays for itself only when the same shape exists in 2+ consumer packages; until then, a private helper that documents the alternative-considered keeps the import graph clean.

- **`state:48688e63:tf-state-no-backup`** — done 2026-05-09 — PR #541, merge commit `6b1293f`. Tier J major (tf-state-atomicity). okdctl never snapshotted `terraform.tfstate` before apply/destroy — terraform's own `.tfstate.backup` is only written across the apply boundary, leaving zero operator-managed history for rollback after a corrupt state lands. The cleanup code's docstring already promised a backup artefact that didn't exist on first deploy. Added `Executor.SnapshotState(ctx) (string, error)` to `internal/infrastructure/terraform/terraform.go` — atomic-write copy to `terraform.tfstate.<UTC-RFC3339-with-dashes>.bak` via `system.AtomicWrite`, retains the 5 most recent snapshots via `pruneSnapshots()` which leans on `os.ReadDir`'s lexicographic sort (filename embeds UTC timestamp, so lex == chronological). Threaded `snapPath, snapErr := tf.SnapshotState(ctx)` into all four destructive entrypoints: `proxmox.Provider.Provision`, `install.StartWorkerVMs`, `postinstall.CleanupBootstrap`, `destroy.destroyInfrastructure`. Each formats the snapshot path into the failure-message `ClusterError` so the operator can `cp` it back. **Postmortem lesson:** Atomic-write-then-rotate is the right primitive for "keep N most recent" file collections; sortable filenames (UTC timestamp embedded) avoid the need for a separate sort step in the rotation logic.

- **`sec:9d79b841:coreos-stream-no-pin`** — done 2026-05-09 — PR #542, merge commit `7f24df4`. Tier J major (tls-network). `fetchCoreOSStream` pulled `fcos.json`/`scos.json` from openshift/installer's `release-4.X` branch (mutable) and trusted whatever ISO URL + sha256 the JSON returned. An attacker who pushed to that branch could rewrite both fields atomically — `DownloadCoreOSISO`'s checksum check would pass against the attacker-supplied sha256. Replaced with `streamPins` map keyed on OKD minor: each entry pins a commit SHA + SHA-256 of the JSON file at that commit. `fetchCoreOSStream` now verifies the response body sha256 against the pin before parsing. Range covers **4.10-4.23** (14 minors) — `streamFileForMinor` already routes pre-19 to `fcos.json` and 19+ to `scos.json`; pre-4.10 minors don't ship `data/data/coreos/fcos.json` at all (HTTP 404), so the parser path never worked there. **Auto-update workflow:** `scripts/update-coreos-pins.sh` re-fetches each minor's tip + JSON sha256, idempotently rewrites the streamPins map; `.github/workflows/update-coreos-pins.yml` runs it Monday 13:00 UTC and opens a PR when any pin drifts. Goconst suppression on the var (4.15-4.18 share an fcos.json sha256 by design; the script can't dedupe without losing per-minor drift visibility). **Postmortem lessons:** (1) pin-the-commit-SHA-and-content-hash is the right shape for a "data-fetch dep that lives outside Go modules"; the maintenance tax is real (~30 sec per minor per OKD release) but the auto-update workflow turns it into a reviewable PR rather than a manual chore. (2) Question the scope assumption — initial pin covered 4.15-4.21 based on a comment in the file; user pushback caught that the realistic supported range is wider, and once the auto-update script existed, extending to 14 minors was a one-line change. (3) `nolint:goconst,nolintlint` is the cleanest dual-suppression when the local lint version is more permissive than CI's — avoids the "fix that creates a new lint" loop.

- **`sec:2f70d7df:ignition-over-http`** — done 2026-05-09 — PR #544, merge commit `7d740de`. Tier J major (tls-network). HTTP ignition fetch URL embedded the cluster pull-secret + SSH keys + machine-config tokens; any sniffer on the bastion VLAN could harvest creds, and an active attacker could substitute a backdoored ignition. Pursued option (a) from the roadmap: tightened threat-model docstrings on `internal/distribution/okd/setup/kargs.go::BuildIgnitionURLForNode` and `setup/apache.go::ConfigureApache` naming the payload contents and VLAN dependency, and added an `net/netip`-based runtime check rejecting non-RFC1918/loopback/link-local ignition IPs (returns `*errtypes.ConfigError`). Both call sites in `iso.go` updated to propagate the error. **Postmortem lesson:** When a fix has two valid options (operator-burden HTTPS rewrite vs. config-time invariant + threat-model doc), pick the one that doesn't change the operator workflow — the runtime guard catches the misconfig everyone actually makes and leaves the door open for a future HTTPS upgrade without invalidating the threat-model docs.

- **`sec:7b2829bb:executor-no-zeroize`** — done 2026-05-09 — PR #545, merge commit `34e72ad`. Tier J major (credentials). `internal/executor/executor.go::Executor.Env` was `[]string` of cred-bearing entries (`PROXMOX_VE_PASSWORD=…`) with no zeroize method — strings persisted as immutable heap objects until GC. Added `executor.Executor.ZeroizeEnv()` using `logutil.KeyIsSecret` (broader than the hardcoded allowlist that `terraform.Executor.ZeroizeEnv` used), made `okd.Provisioner.ZeroizeEnv` delegate to the executor, and wired `defer prov.ZeroizeEnv()` into both direct `proxmox.New` call sites (`cli/deploy.go::runDeployDryRun` and `install/phase.go`) — the latter was missed in round 1 and added as a follow-up commit (`0186b2c`) after the reviewer flagged the gap. CLAUDE.md §credentials gained a 'ZeroizeEnv defer pattern' bullet. **Postmortem lesson:** When the Problem statement names multiple distinct fields (`executor.Env`, `terraform.Executor.Env`, `proxmox.Provider.env`), the planner brief must re-quote them verbatim and verify each one is addressed in the changes list — paraphrasing the Problem to 'add ZeroizeEnv' silently lost the proxmox.Provider field on the first cut.

- **`sub:48688e63:argv-concat`** — done 2026-05-09 — PR #546, merge commit `1515ec6`. Tier J major (argv-construction). `internal/infrastructure/proxmox/proxmox.go::probeVMEnumeration` called `phase.SSHRunArgv` with `'/nodes/'+p.node+'/qemu'` built by string concat, bypassing `pveshRun`'s `validateProxmoxName` guard. Exposed `phase.PveshRun(ctx, *RemoteISOParams, subcommand, path) (string, error)` as the canonical wrapper around the unexported `pveshRun`, and rewrote `probeVMEnumeration` to call it through `RemoteISOParams`. The validation guard is now inherited automatically. **Postmortem lesson:** When a private helper enforces an invariant (`validateProxmoxName` at `pveshRun`'s boundary), exporting a thin wrapper is the right pattern for cross-package callers — it keeps the validation single-sourced without forcing every caller to reproduce the check.

- **`sub:eb479d86:argv-unclean-path`** — done 2026-05-09 — PR #547, merge commit `66e0398`. Tier J minor (argv-construction). `internal/distribution/okd/setup/upload.go::remoteISO256` passed a filename (filepath.Base of an os.ReadDir entry) to `phase.SSHRunArgv` as a sha256sum positional arg, but argv-mode does NOT bypass the remote login shell — a hostile filesystem entry like `foo;rm.iso` would split on `;`. Added `phase.ValidateRemoteFilename(name string) error` mirroring the existing `validateProxmoxName`/`validateISODir` shape (rejects empty, `..`, path separators, and `;|&$\`*?[]<>~ \t\n\r'` shell metas), called it at the top of `remoteISO256` before any SSHRunArgv interpolation. New `TestValidateRemoteFilename` table covers ~15 reject cases and 4 accept cases. **Postmortem lesson:** SSHRunArgv's argv-mode protects against flag injection (`--`) but not against shell-metacharacter splitting at the remote sshd login shell — every filename atom interpolated into a remote argv needs explicit validation, mirroring the existing iso_cleanup pattern.

- **`state:4c092fce:tf-state-no-lock`** — done 2026-05-09 — PR #548, merge commit `e946eeb`. Tier J major (tf-state-atomicity). okdctl's terraform Executor never set `-lock-timeout`, so a stale `.terraform.tfstate.lock.info` from a SIGKILL'd prior run hung the next apply at terraform's default 0s with an immediate fail rather than a wait+retry — and `stateLockHint` only fired on the destroy path. Added `defaultLockTimeout = "120s"` constant and threaded it as `-lock-timeout=120s` into Plan/PlanStreamed/Apply/destroyDirect (init excluded — terraform rejects the flag there). Promoted `stateLockHint`+`parseLockID` from `destroy/helpers.go` to `terraform.Executor.LockHint()` so install/postinstall/cli paths can surface it via `errors.Join(hint, &errtypes.ClusterError{...})`. Wired Init/Plan/Apply failure paths in `destroy/helpers.go`, `postinstall/bootstrap.go`, `install/workers.go`, and `cli/destroy.go`. CLAUDE.md §architecture-notes gained an NFS bus-factor caveat. **Postmortem lesson:** When promoting a private helper to an exported method, the existing tests pattern `&terraform.Executor{WorkDir: dir}` (struct literal with only required fields) lets test rewrites stay one-line — but only because the method touches only WorkDir; future changes that add field accesses must update the test fixtures too.

- **`state:0f076161:destroy-target-no-precondition`** — done 2026-05-09 — PR #549, merge commit `334d817`. Tier J minor (destroy-safety). `internal/cli/destroy.go::validateDestroyTargets` regex-matched the `--target` allowlist but never bounds-checked the `[N]` index against `cfg.Topology.{ControlPlane,Workers}.Count`. An operator typo like `master[7]` on a 3-master cluster passed validation, hit terraform with a non-existent target, and destroy reported success while destroying nothing. Switched to `FindStringSubmatch`, parsed the bracket index with `strconv.Atoi`, and rejected `bootstrap[idx!=0]` / `master[idx>=ControlPlane.Count]` / `worker[idx>=Workers.Count]` as `*errtypes.UsageError` (exit 64). Added `TestValidateDestroyTargets_Bounds` covering OOB and in-range cases. **Postmortem lesson:** Allowlist regex on a flag value catches the shape but not the semantics — when the value indexes into a config-defined collection, the bounds check belongs alongside the regex, not deferred to the downstream tool.

- **`state:33579dd5:cleanup-haproxy-firewall-double`** — done 2026-05-09 — PR #550, merge commit `5484432`. Tier J minor (phase-idempotency). `internal/distribution/okd/cleanup/services.go::HAProxy` called `firewall.RemoveOKDRules`, but the destroy phase's `StepCleanupFirewall` already calls it — both ran during `okdctl destroy`, and the second call against an already-removed rule logged a warning that propagated as a non-fatal failure in the destroy summary, falsely marking 'firewall' as a failed step. Removed the duplicate call from `cleanup.HAProxy` (and the now-unused `firewall` import); `StepCleanupFirewall` is the single removal site. **Postmortem lesson:** When two phases each call a 'best-effort' helper that's documented as idempotent, the failure modes can still differ across backends (firewalld here) — single-source the call to one canonical step rather than relying on the helper's idempotency contract holding across all platforms.

- **`state:632c9087:update-ingress-no-rollback-on-dns`** — done 2026-05-09 — PR #551, merge commit `9426e39`. Tier J minor (crash-recoverability). `internal/distribution/okd/postinstall/update_ingress.go::finalizeIngress` deployed production DNS and then optionally removed HAProxy. If `RemoveHAProxy` failed AFTER the DNS swap, the cluster was left with DNS pointing at the new VIP/apps but HAProxy still listening on bastion — incoming requests bypassed kube-vip routing. Re-sequencing was unsafe (RemoveHAProxy's hostname check resolves api.* via dnsmasq, requires DNS to already point at VIP), so implemented option 2: on RemoveHAProxy failure, call `dns.DeployBootstrap(ctx, cfg)` to roll DNS back, then return a typed `*errtypes.ClusterError`. Added a 3-line WHY comment naming the hostname-resolution ordering constraint. **Postmortem lesson:** When two destructive ops have a circular dependency (each requires the other's old state to verify), the rollback path is the only safe shape — re-sequencing gives an unverifiable intermediate state.

- **`state:c287d5c0:destroy-auto-approve-hardcoded`** — done 2026-05-09 — PR #552, merge commit `4b78042`. Tier J minor (destroy-safety). `internal/distribution/okd/okd.go::Provisioner.Destroy` hard-coded `destroyOpts.AutoApprove = true` regardless of `cfg.Deployment.AutoApprove` or any CLI flag — anyone constructing a Provisioner programmatically (test harness, future MCP server) got non-interactive terraform destroy with no upstream check. Added `AutoApprove bool` to `DestroyOpts` (mirroring `ProvisionOptions.AutoApprove`) and replaced the hardcoded `=true` with `=opts.AutoApprove`. CLI's `runDestroy` now sets `AutoApprove: true` only after the cluster-name confirmation gate passes; library callers default to false. **Postmortem lesson:** Hardcoding a destructive flag downstream of the user-confirmation prompt makes the prompt look load-bearing when it isn't — the right shape is to plumb the decision through the options struct so library callers cannot bypass the check.

- **`iac:b803fcb7:tflint-version-unpinned`** — done 2026-05-09 — PR #553, merge commit `a4ebbd7`. Tier J minor (hcl-provider-hygiene). `.github/workflows/ci.yml` ran `setup-tflint` SHA-pinned but with no `tflint_version` — the action installed whatever tflint the runner defaulted to, violating CLAUDE.md §Pin stability ('tool installs must be explicit versions, never @latest'). Added `with: { tflint_version: "v0.62.0" }` (latest stable as of 2026-05-09 per GitHub releases API) to the single setup-tflint invocation. **Postmortem lesson:** When a roadmap Fix says 'pin to vX.Y.Z (or current pin)', verify against the upstream release feed at edit time — the suggested version may be older than what's available, and pinning to the latest stable amortizes the next bump.

- **`iac:e076e43c:tar-zipslip-incomplete`** — done 2026-05-09 — PR #554, merge commit `e05d0b1`. Tier J minor (install-sh-integrity). `scripts/install.sh` defense-in-depth zip-slip check used `grep -qE '^(\.\.|/)' ` — only catches entries starting with `..` or `/`. A canonical zip-slip entry like `subdir/../../etc/passwd` starts with `subdir/` and silently passed. Replaced with the OWASP-canonical regex `(^|/)\.\.(/|$)|^/` which catches `..` as the first segment, any internal `/../` segment, or any absolute path. **Postmortem lesson:** Anchored regex (`^pattern`) is a footgun for path-traversal detection — the attack vector is internal `/../` segments, not leading ones. Use the canonical OWASP pattern when implementing defense-in-depth checks; reinventing it is how the bypass class survives.

- **`iac:ef8f2924:bootstrap-default-shadows-coalesce`** — done 2026-05-09 — PR #555, merge commit `07b833c`. Tier J minor (hcl-credential-hygiene). `infrastructure/terraform/modules/proxmox-okd/variables.tf` declared three per-role override variables (`bootstrap_memory_mb=8192`, `worker_cpu_cores=8`, `worker_memory_mb=20480`) with non-null defaults, defeating the `coalesce(non_null, x)` fallback in `main.tf` locals — setting cluster-wide `memory_mb=16384` did not raise these. Set defaults to null to match the documented fallback semantics, mirroring the correctly-shaped peers (`master_cpu_cores`, `master_memory_mb`, `bootstrap_cpu_cores`). The cluster-wide `memory_mb` validation block (>= 8192) still enforces the operator-grade floor. **Postmortem lesson:** A non-null default on a variable described as 'defaults to X if not set' is a contradiction — the variable is ALREADY set (to that default), so the coalesce never sees null. The tell is the variable description; when it promises 'defaults to', the actual default must be null.

- **`err:62cb8a95:state-lock-hint-drops-init-cause`** — done 2026-05-09 — PR #556 closed (superseded). Tier J minor (wrapping). PR #556 was opened with the standalone fix (`return errors.Join(hint, &errtypes.ClusterError{...})` in `destroy/helpers.go`) but state:4c092fce (PR #548) landed the same join semantics one merge earlier as part of promoting `stateLockHint` to `terraform.Executor.LockHint()` — the helper relocation moved the call site, made the standalone PR conflict with develop, and the join semantics were already in place at the new location. Closed PR #556 as superseded; the fix intent is fully captured in #548 (commit `e946eeb`). **Postmortem lesson:** When two roadmap items target the same call site with overlapping fixes, batch them into one PR — running them as parallel worktrees produces a redundant follow-up that conflicts with the broader fix, and the second PR adds zero value.

- **`err:881d089e:runlock-untyped-errors`** — done 2026-05-09 — PR #557, merge commit `91fe8e7`. Tier J minor (sentinel-vs-typed). `internal/runlock/runlock.go::Acquire` returned `*errtypes.ConfigError` for symlink-refusal and lock-conflict but bare `fmt.Errorf` for the lstat and OpenFile syscall failures — a read-permission fault on the project root fell to exit 1 instead of the documented exit 2 (ConfigError). Wrapped both in `&errtypes.ConfigError{Msg: fmt.Sprintf("runlock: lstat %s", path), Err: err}` and similar for OpenFile, mirroring the sibling cases. **Postmortem lesson:** Mixed-typed-and-bare error returns in the same function are a smell — the typed paths set the documented exit code, the bare paths silently fall to exit 1. Sweep the function as a unit when the typed taxonomy lands.

- **`con:ab9b764a:ctx-ignored-install-config`** — done 2026-05-09 — PR #558, merge commit `9425632`. Tier J minor (ctx-ignored). `internal/distribution/okd/setup/ignition.go::GenerateInstallConfig` and `InjectCompactClusterManifests` accepted `_ context.Context` and never consulted ctx. Both run inside StepDef.Exec closures whose orchestrator threads a real ctx — dropping it inside the body breaks SIGINT responsiveness. Renamed `_` to `ctx` and added `if err := ctx.Err(); err != nil { return err }` at the top of each body, mirroring `ValidateIgnitionFiles` in the same file. **Postmortem lesson:** `_ context.Context` in a function signature is the worst of both worlds — looks like ctx threading without delivering it. Either drop the param entirely or honor it; the placeholder is rot.

- **`con:97cb8adf:waitfor-check-no-ctx`** — done 2026-05-09 — PR #559, merge commit `db2ed2f`. Tier J suggestion (ctx-ignored). `internal/system/exec.go::WaitFor`'s `check func() bool` callback took no ctx, so polling sites couldn't propagate cancellation into their probe. Changed signature to `check func(context.Context) bool` and threaded the outer ctx into both `check(ctx)` invocations (initial probe + ticker loop). `WaitForWithTimeout` updated in parallel. Updated 7 callers (kubectl.go, haproxy.go, update_ingress.go, verify.go, flux.go) plus 6 test callers — all closed over the outer ctx already so the rewrite was mechanical. **Postmortem lesson:** Signature changes that add a positional param can be safely batched across all call sites in one commit when each caller already has the value in scope; a perl one-liner over the call sites turns a 7-file rewrite into a 30-second sweep.

- **`obs:660d83a5:setrunid-not-symmetric-with-stderrslog`** — done 2026-05-09 — PR #560, merge commit `1c31b0b`. Tier J suggestion (handler-setup). `internal/tui/logger.go::SetRunID` rebuilt `stderrSlog` after pinning run_id but never re-bound `slog.Default()` — any third-party library that captured `slog.Default()` before SetRunID's rebuild permanently missed run_id. Pursued option (a) from the roadmap: added `slog.SetDefault(stderrSlog.Load())` immediately after `stderrSlog.Store(buildStderrSlog())` inside SetRunID, with a one-line WHY comment. Updated `SimpleLogger` doc to call out the snapshot semantic for callers. **Postmortem lesson:** When a helper rebuilds a global, every consumer that captured the global before the rebuild is stale — the symmetric fix is to re-publish the global at the rebuild site, not to retrofit each consumer.

- **`obs:9d79b841:double-log-iso-download`** — done 2026-05-09 — PR #561, merge commit `e61d2e5`. Tier J suggestion (span-retry-boundary). Two consecutive `p.Log.Info` records for one event in `internal/distribution/okd/setup/coreos.go:277-278` ('coreos: downloading iso' with version, then 'coreos: iso url' with url) read as one logical span split across two lines. Folded to a single record: `p.Log.Info("coreos: downloading iso", "version", info.Version, "url", info.ISOUrl)`. Net -1 LOC. **Postmortem lesson:** When two consecutive log records have no intermediate state to surface, fold them — one structured record with multiple attrs is the canonical form, two records is signal that the second was tacked on later.

- **`mod:5e892064:strings-lines-checksum`** — done 2026-05-09 — PR #562, merge commit `6541d04`. Tier J suggestion (slices-maps). `internal/download/checksum.go` walked the body via `strings.Split(string(body), "\n")` then ranged the slice. Migrated to `for line := range strings.Lines(string(body))` — same semantics because the immediate `strings.TrimSpace(line)` strips the trailing `\n` that `strings.Lines` preserves, but no upfront slice allocation. **Postmortem lesson:** `strings.Lines` is safe as a `Split("\n")` drop-in only when the loop body does something to the line that strips the trailing `\n` — TrimSpace works because the empty-string check catches both pre- and post-newline forms; otherwise use `strings.SplitSeq` (see mod:983f67f0).

- **`mod:983f67f0:strings-lines-mixed`** — done 2026-05-09 — PR #563, merge commit `dceea3a`. Tier J suggestion (slices-maps). `internal/tui/layouts.go::boxedSectionCore` used `strings.Split` while `maxLineWidth` already used `strings.Lines` — internal inconsistency. First-cut delta migrated to `strings.Lines` (commit `dceea3a`) but reviewer caught a real correctness regression: `strings.Lines` retains the trailing `\n` per yielded line, which embedded between content and the right border `│` would have shipped a visibly broken box. Round 2 (commit `34db690`) switched both sites to `strings.SplitSeq(content, "\n")` — same Go 1.24 iterator allocation savings, but `Split` semantics (no terminator inclusion, preserves trailing empty element on `\n`-terminated input). **Postmortem lesson:** `strings.Lines` and `strings.SplitSeq` look interchangeable but differ on terminator handling — `Lines` keeps `\n`, `SplitSeq` strips on the boundary. Use `SplitSeq` whenever the line is going into rendered output without intermediate trimming; `Lines` only when the loop body trims or scans byte-by-byte.

- **`mod:c87d0b1f:log-fatal-in-cli-entrypoint`** — done 2026-05-09 — PR #564, merge commit `c2e7790`. Tier J suggestion (slog-over-3p). `cmd/okdctl-gen-docs/main.go` (build-tagged `//go:build docs`) used `log.Fatalf` from stdlib — repo runtime uses `tui.X` + `slog`. First-cut implemented option (b) from the roadmap (switch to `fmt.Fprintf(os.Stderr, ...) + os.Exit(1)`, commit `c2e7790`) but the user flagged this as a third pattern conflicting with the canonical standardization (active roadmap item `ux:daf5bee9` flags the same `fmt.Fprintf(os.Stderr, ...)` antipattern in `cli/kubeconfig.go`). Reverted to option (a) in commit `d4e07a8`: kept `log.Fatalf` and added a 3-line WHY comment naming why slog plumbing is not desired (docs-only generator, never linked into the okdctl binary, slog/tui import graph would be dead weight). **Postmortem lesson:** When a roadmap Fix presents two options and one of them introduces a third pattern that conflicts with active standardization elsewhere, surface the conflict to the user before picking — the planner's local cost/benefit can miss the global consistency cost.

- **`smell:26a430ee:requires-root-annotation-key`** — done 2026-05-09 — no PR (already implemented in develop). Tier J minor (magic-strings). Already implemented in develop at the time of this session's pickup: `internal/cli/flags.go:29` defined `const annotationKeyRequiresRoot = "requiresRoot"` and all three consumer sites (`elevation.go:51`, `addon.go:47`, `addon.go:82`) used the constant. The roadmap-implementer planner correctly returned `unresolved_questions` flagging the no-op rather than inventing a delta. Closed as `deferred-done` (no PR opened) — archive presence is the canonical record. **Postmortem lesson:** When a roadmap item's claimed defect is already fixed in develop, the planner's correct response is `unresolved_questions`, not a no-op delta — the no-op delta would have wasted a worktree, a PR, and a review round on a change that doesn't change anything. Trust the planner's read.

- **`smell:e7db1220:duplicate-enum-label-fn`** — done 2026-05-09 — PR #565, merge commit `7c51cd9`. Tier J minor (helper-package-no-value). `releases.LabelForReleaseType` (in `internal/distribution/okd/releases/types.go`) and the cli call sites had two paths to the same switch table on `ReleaseType`. Added `func (t ReleaseType) String() string` (idiomatic `fmt.Stringer`) with the existing label body, dropped `LabelForReleaseType`, updated `MarshalJSON` to call `t.String()`, and switched both `cli/releases.go` call sites (`printVersionList` L186, `printVersionDetail` L200) from `releases.LabelForReleaseType(v.Type)` to `v.Type.String()`. **Postmortem lesson:** When a free function on a type-aliased value duplicates a switch table, promoting it to a `String()` method gives both the dedupe AND the `fmt.Stringer` interface for free — every caller that uses `%s` / `%v` on the value now formats correctly without explicit calls.

- **`doc:0139cb3f:pkg-doc-canonical-helpers`** — done 2026-05-09 — PR #566, merge commit `d9063b8`. Tier J minor (package-doc). `internal/distribution/okd/phase/paths.go` package doc said only 'shared base types and path utilities' — readers of godoc missed that this package is the canonical home for cross-phase helpers (CLAUDE.md §architecture-notes forbids adding helpers elsewhere). Expanded to a 5-line block naming `BasePhase`, `OcResourceExists`, `OcPollOutput`, `NodeRole`, `ConditionStatus`, `VMState`, `SSHRunArgv` and stating the import-graph rule. All six helpers verified to exist in the package before inclusion. **Postmortem lesson:** Package docs that echo the package name are silent on the load-bearing rule — when the architecture forbids spreading helpers across packages, the package doc on the canonical home is the right place to surface that rule, since godoc readers see it before they grep.

- **`doc:8aa632a6:pkg-doc-name-echo`** — done 2026-05-09 — PR #567, merge commit `45193d5`. Tier J minor (package-doc). `internal/version/version.go` package doc was a generic name echo. Expanded to two sentences naming the build-time identity contract (Version, GitCommit, BuildDate, GoVersion, Platform — all `-ldflags`-injected before main, read race-free at runtime, save/restore via t.Cleanup in tests). Round 1 shipped as a single ~190-char line; reviewer correctly flagged the CLAUDE.md ~100-char ceiling violation — round 2 (commit `97c34f3`) wrapped to four lines ≤80 cols. **Postmortem lesson:** When the roadmap Fix paragraph is provided as a single sentence run, ask whether it fits the file's wrap policy before echoing it verbatim — line-length lints catch this on lint pass, but reviewers should also catch it before push.
- **`sub:0934cf1b:no-stderr-capture`** — done 2026-05-09 — PR #568, merge commit `3f64cbc`. Tier J minor (io-handling). `internal/platform/packages.go::Manager.IsInstalled` (L97-L112) called `cmd.Output()` directly, capturing stdout but discarding stderr — sibling `Install`/`Remove` already routed through `system.RunCaptured` which preserves stderr inside `*system.SubprocessError`. Replaced with `system.OutputCaptured`; unwrapped `*system.SubprocessError` then inner `*exec.ExitError` to preserve the `(false, nil)` semantic on non-zero exits while letting ctx cancellation, LookPath misses, and I/O faults propagate with stderr context attached. **Postmortem lesson:** When two siblings in the same wrapper diverge on subprocess capture, the divergent one should adopt the canonical helper rather than each carrying its own stderr-discarding shim — symmetric helpers reduce future regression surface.

- **`obs:97cb8adf:fmt-sprintf-message-pattern`** — done 2026-05-09 — PR #569, merge commit `ff4972c`. Tier J minor (redaction-sink). `internal/system/exec.go::WaitFor` built slog messages via `fmt.Sprintf("%s: waiting for %s...", prefix, description)` then passed the rendered string as the message arg — `RedactHandler` cannot inspect pre-rendered text per CLAUDE.md §credentials. Folded both `Info` sites and the L164 `Debug` to static lowercase messages (`"waiting"`, `"ready"`) with structured `prefix`/`for`/`polls`/`elapsed` attrs. The c07157e migration swept ~85 sites but missed this canonical helper. **Postmortem lesson:** A migration that's complete-by-call-count can still miss a canonical helper that has only one or two call sites — re-grep the forbidden pattern in the helper layer specifically before declaring such migrations done.

- **`mod:b8687976:slices-clip-dedup`** — done 2026-05-09 — PR #570, merge commit `42f3bbd`. Tier J suggestion (slices-maps). `internal/distribution/okd/releases/fetcher.go::deduplicateReleases` hand-rolled a seen-set + filtered-result loop. Audited downstream (`parseReleases` buckets via `map` erasing order, `sortAndClassifySeries` fully reorders by semver+date) — confirmed order-insensitive — then swapped to `slices.SortFunc(TagName) + slices.CompactFunc(TagName equality)`. Added 4 table-driven tests in new `fetcher_test.go`. **Postmortem lesson:** `slices.CompactFunc` requires sort precondition — every "dedup with `CompactFunc`" plan must include the order-dependency audit on every caller before the swap, not just the immediate consumer.

- **`smell:1d5afa08:release-type-unknown-default`** — done 2026-05-09 — PR #571, merge commit `a168480`. Tier J suggestion (magic-strings). `internal/distribution/okd/releases/types.go::UnmarshalJSON` for `ReleaseType` silently coerced any unknown string to `ReleaseTypeStable` while the matching label function emitted "unknown" on the marshal side — round-trip asymmetry that lost information on cache-format drift. Replaced with `fmt.Errorf("unknown release type %q", s)`; the `loadFromDiskCache` path already returns nil on `json.Unmarshal` error, so a drifted cache now triggers a fresh network refetch instead of silent miscategorisation. Added `TestReleaseTypeUnmarshalJSONUnknown`. **Postmortem lesson:** Round-trip asymmetry between marshal ("unknown") and unmarshal (silent stable fallback) is a footgun even when the unmarshal path looks defensible — make the asymmetry an error.

- **`tst:40d315ad:destructive-happy-untested`** — done 2026-05-09 — PR #572, merge commit `aa147b5`. Tier J major (destructive-untested). `internal/addon/catalog/flux/flux.go::Uninstall` invoked `helm uninstall` (×2) + `oc delete ns flux-system` with no test coverage — a regression that dropped the namespace arg from `oc delete ns` would propagate through `Manager.installAndVerify` rollback and delete every namespace. Added `flux_uninstall_test.go` with POSIX-shell fake `helm`/`oc` on PATH; happy-path asserts exact ordered argv via `ARGV_LOG`; failure-path uses empty PATH (since `Exec.Run` swallows non-zero exit codes — only `LookPath` failure surfaces a non-nil error to the `warnOnErr` closure). **Postmortem lesson:** `Exec.Run` (vs `RunChecked`) swallows exit-code failures — the failure-path test must trigger an actual error class (`LookPath` ENOENT here), not just a non-zero exit, otherwise the test "passes" while exercising nothing.

- **`smell:d9f7733e:bundle-category-magic-string-pair`** — done 2026-05-09 — PR #573, merge commit `c08c239`. Tier J suggestion (magic-strings). The 6 `categoryX` constants in `internal/cli/debug_bundle.go` were typed `string`, used as `manifestEntry.Name` slot keys with no compile-time guard. Adjacent `bundleStatus` already used the typed-string-with-constants pattern. Added `type bundleCategory string`, retagged the 6 constants, changed `manifestEntry.Name` to `bundleCategory`. `doctor_cmd.go` cobra fields (`Use`, `Example`) coerce via `string(categoryDoctor)` since cobra requires `string`. JSON wire format unchanged. **Postmortem lesson:** When a sibling in the same file already uses a typed-string-constants pattern, replicate it for any new family — the `bundleStatus`/`bundleCategory` symmetry is now the local norm.

- **`ux:024a2c32:json-schema-status-incomplete`** — done 2026-05-09 — PR #574, merge commit `975bf0c`. Tier J minor (json-stability). `docs/cli/json-schema.md` documented only 4 of 10 fields on `okdctl status --output=json`; `phase` (always present) and 5 omitempty fields (`version`, `api_server_url`, `console_url`, `conditions`, `message`) were absent. Verified each tag against `internal/distribution/okd/types.go::ClusterStatus`. Added all 6 fields with accurate omitempty notes plus `conditions[].type/status/reason/message` sub-rows following the existing `nodes[]` dotted pattern. Corrected `addons[]` to "present when non-empty" since its tag is `omitempty`. **Postmortem lesson:** Schema docs that describe the human-friendly subset silently teach consumers that the omitted fields don't exist — the doc must enumerate every json tag, not just the ones common in healthy-cluster snapshots.

- **`smell:d31d1b9d:health-stringly-typed`** — done 2026-05-09 — PR #575, merge commit `53c4226`. Tier J suggestion (magic-strings). Two sites in `internal/cli/status.go` printed addon health by branching on `Healthy` directly and emitting "healthy"/"degraded"/"not enabled" string literals — drift risk from the JSON `Healthy bool`. Added `func (a okd.AddonStatus) Label() string` returning the tri-state mapping ("healthy" if `Healthy`; "degraded" if `!Healthy && Error != ""`; "not enabled" for zero-value). Both call sites delegate to `Label()`; describe path keeps the `": " + Error` suffix when an error is present, preserving the original `"degraded: <err>"` composite string verbatim. **Postmortem lesson:** Tri-state print labels driven by a (bool, string) struct deserve a method on the type so the two halves can't drift — and the zero-value mapping is the natural "not present" fallback when no struct exists for the lookup key.

- **`smell:0f076161:destroy-only-stringly-typed`** — done 2026-05-09 — PR #576, merge commit `e5192a9`. Tier J suggestion (magic-strings). `internal/cli/destroy.go --only` flag accepted bare strings via a switch with the valid set duplicated across 3 sites (switch arms, error message, help text). Added `type destroyScope string` + 4 named constants + `validDestroyScopes() []string` helper; switch uses typed cases; help string and error message both derive from the helper; `RegisterFlagCompletionFunc("only", ...)` wires shell completion to the helper. CLI wire values preserved verbatim. **Postmortem lesson:** Once a flag's valid set is duplicated across three places (switch, error message, help text), every future addition is a 3-touch invitation to drift — funnel through one helper and the cobra completion comes free.

- **`smell:8154ab0f:doctor-severity-string-roundtrip`** — done 2026-05-09 — PR #577, merge commit `8a85d9a`. Tier J suggestion (magic-strings). `internal/cli/doctor.go` had three independent surface forms for the same 3-state `severity` iota: `sevString` returning "ok/warn/fail/unknown", `severityMarkers` hardcoding "[ok]/[warn]/[fail]" rawLabels, and JSON output via `sevString`. Promoted `sevString` to `func (s severity) String() string` (idiomatic `fmt.Stringer`); both call sites use `.String()`; `severityMarkers` derives `rawLabel = "[" + sev.String() + "]"` once before the switch. JSON output strings unchanged. The "unknown" default branch is unreachable with current iota usage but kept with a one-line WHY comment to guard future severity additions. **Postmortem lesson:** Multiple surface forms of the same enum (string, bracketed string, JSON value) only need one source of truth — `fmt.Stringer` is the right shape and gives `%s`/`%v` formatters for free.

- **`ux:8d8faa80:completion-bypasses-outorstdout`** — done 2026-05-09 — PR #578, merge commit `8499825`. Tier J minor (streams). `internal/cli/completion.go::runCompletion` wrote shell scripts to `os.Stdout` directly instead of `cmd.OutOrStdout()` — every other leaf command (status, releases, addon list, addon verify, doctor) routed through cobra's writer. Switched all 3 `Gen*Completion` calls (bash/zsh/fish; PowerShell already removed) to `cmd.OutOrStdout()`; dropped now-unused `"os"` import; added `completion_test.go` covering writer wiring (captured-buffer assertion of shell preamble bytes) plus the unknown-shell error path. **Postmortem lesson:** A leaf cobra command that writes to `os.Stdout` directly is invisible to test capture and to any future caller that wants to redirect output — `cmd.OutOrStdout()` is the universal sink across the tree, no exceptions.

- **`ux:aa84670c:version-cmd-uses-run-not-rune`** — done 2026-05-09 — PR #579, merge commit `1f3fe6d`. Tier J suggestion (verb-noun). `internal/cli/root.go::versionCmd` used `Run` instead of `RunE`, dropping `fmt.Fprintf`'s return value — a failed write to a closed pipe (`okdctl version | head -c 0`) silently exited 0 instead of propagating to `exitCodeFor`. Converted to `RunE` returning the `fmt.Fprintf` error; cobra ignores `Run` when `RunE` is set, so the swap is purely additive. **Postmortem lesson:** Every leaf cobra command in this tree should use `RunE` so the documented exit-code taxonomy applies uniformly — a single `Run` outlier is a silent-success surface waiting to happen.

- **`ux:073d24ed:deploy-example-uses-config-not-output-file`** — done 2026-05-09 — PR #580, merge commit `c95b4ec`. Tier J minor (flag-conventions). `internal/cli/deploy.go` Example block conflated `--config` (read-side persistent flag) with `--output-file` (deploy's write-side flag) by showing `okdctl deploy --yes --config my-cluster.yaml`. The example happened to work on `--yes` because no wizard write occurs, but a user copying it would think `--config` controls write destination. Split into two pedagogical lines: read (`--config`) and write (`--output-file`); regenerated `docs/cli/okdctl_deploy.md` via `make docs`. **Postmortem lesson:** A working example that conflates two flags into one happy-path invocation teaches incorrect mental models — split read and write semantics into distinct examples even when both run cleanly.

- **`obs:632c9087:rollback-pair-not-spanned`** — done 2026-05-09 — PR #581, merge commit `a036008`. Tier J suggestion (span-retry-boundary). `internal/distribution/okd/postinstall/update_ingress.go::convertToLoadBalancer` had open span boundaries: the `waitForRouterGone` "waiting…" log had no success-side close, and `attemptRollback` had no entry log (only its 3 leaf exit logs). Operators following the JSON log stream saw "failed to create replacement, attempting rollback" then silence until the leaf landed. Added 2 lowercase structured log calls — "router terminated" after the wait succeeds and "rollback: starting" at the top of `attemptRollback` — both with `"name", ic.Name`. Net +2 LOC. **Postmortem lesson:** Open span boundaries in destructive multi-step ops force operators to guess intermediate state from silence — close every span-open with a span-close log on the success path, even when the next event would already imply success.

- **`con:06f00bcb:ctx-ignored-file-io`** — done 2026-05-09 — PR #582, merge commit `220d341`. Tier J minor (ctx-ignored). `internal/distribution/okd/setup/apache.go::ensureIgnitionDir` and `configureApachePort` accepted `_ context.Context` but never read it — substantial file I/O (CopyFile, 1MiB-buffered Scanner, AtomicWrite) ran past SIGINT. CLAUDE.md §concurrency calls `_ context.Context` "the worst pattern". Renamed each `_` to `ctx` and added `if err := ctx.Err(); err != nil { return ... }` at the top of each function body before any syscall. **Postmortem lesson:** `_ context.Context` is the worst-of-both-worlds shape — looks like ctx threading without delivering it. Either accept and gate or drop the parameter entirely.

- **`mod:c19ee328:slices-containsfunc-allexist`** — done 2026-05-09 — PR #583, merge commit `46cf24a`. Tier J suggestion (slices-maps). `internal/distribution/okd/setup/steps.go::StepDownloadTools.AlreadyDone` iterated 3 binary names with a hand-rolled "all-of" loop. Replaced with `slices.ContainsFunc + negation` — same short-circuit semantics, -3 LOC, matches the repo's prevailing any-of/all-of style. Added `"slices"` import. The twin `StepGenerateIgnition.AlreadyDone` (L186-192) is a single-file existence check, not a loop, and was intentionally untouched (separate roadmap item if revisited). **Postmortem lesson:** When two AlreadyDone helpers in the same file have superficially similar shapes, claim one item per shape — don't rewrite a single-file check as if it were a loop.

- **`iac:04b033b9:provider-insecure-not-pinned-false`** — done 2026-05-09 — PR #584, merge commit `3ade6b3`. Tier J suggestion (hcl-credential-hygiene). `infrastructure/terraform/environments/production/versions.tf` declared bpg/proxmox provider via `required_providers` but had no `provider "proxmox" {}` block — production silently consumed `PROXMOX_VE_INSECURE` from env. Added explicit `provider "proxmox" { insecure = false }` block. Endpoint, username, password remain env-driven (intentional). **Postmortem lesson:** A required_providers without an explicit provider block hands TLS posture to the operator's shell environment — pin the security-sensitive attributes in HCL even when other attributes flow through env vars.

- **`dep:b803fcb7:golangci-version-drift`** — done 2026-05-09 — PR #585, merge commit `bbc8c81`. Tier J minor (pin-stability). `Makefile` installed `golangci-lint@v2.12.1`; `ci.yml` lint-go pinned `v2.12.2` — local `make lint` ran a different linter than CI. Bumped Makefile to v2.12.2 (literal sync). The Makefile's `which golangci-lint` guard means existing local installs continue with whichever version is on PATH until reinstalled — a pre-existing invariant, separate concern from this fix. **Postmortem lesson:** Pin-version drift between Makefile install lines and CI action versions is invisible until a lint rule lands in one but not the other — keep them in lockstep, or extract to a single source-of-truth file.

- **`sec:bbc23e42:noplogger-no-redact`** — done 2026-05-09 — PR #586, merge commit `f162eb8`. Tier J suggestion (redaction). `internal/logutil.NopLogger` was `slog.New(slog.DiscardHandler)` — no `RedactHandler` in the chain. DiscardHandler discards everything anyway, but the brittleness was forward-looking: a future `logger.Info("...", "password", secret)` reaching `NopLogger` would silently bypass `RedactHandler`. Wrapped the chain: `slog.New(NewRedactHandler(slog.DiscardHandler))`. Effectively a no-op today; updated the doc comment to name the invariant. **Postmortem lesson:** Forward-looking redaction wraps cost zero at runtime (RedactHandler→DiscardHandler is two function calls per record both of which short-circuit) but eliminate a whole class of "future caller bypasses redaction" regressions.

- **`iac:18a795d5:depends-on-bootstrap-artificial`** — done 2026-05-09 — PR #587, merge commit `4c6750e`. Tier J minor (hcl-destroy-ordering). `infrastructure/terraform/modules/proxmox-okd/main.tf` master declared `depends_on = [bootstrap]` and worker declared `depends_on = [master]`, but neither block referenced the dependency's attributes. The OKD-installer-level dependency (masters need bootstrap to fetch ignition) is application-layer, not Terraform-graph; okdctl's two-phase destroy in `internal/distribution/okd/destroy/` already owns sequencing. Dropped both `depends_on`. `terraform fmt` and `terraform validate` pass. **Postmortem lesson:** HashiCorp guidance is unambiguous — `depends_on` only when the dep cannot be expressed via attribute reference; encoding application-layer ordering in the Terraform graph adds artificial serialisation and confusing destroy plans without buying anything.

- **`tst:d7ce9d16:destructive-happy-untested`** — done 2026-05-09 — PR #588, merge commit `1dcb6ee`. Tier J major (destructive-untested). `internal/distribution/okd/dns/dns.go::validateAndRestartDnsmasq` is the rollback-on-validation-failure twin of the haproxy path that has `haproxy_rollback_test.go` coverage; the dnsmasq twin had none — a regression silently dropping `restore()` would leave the cluster with broken DNS. Added 2 package-level fn vars (`validateDnsmasqConfigFn`, `restartDnsmasqFn`) as the test seam (matches the existing `dnsmasqConfigDir` var-override pattern); routed `validateAndRestartDnsmasq` through them; new `dns_destructive_test.go` covers all 4 cases — happy_path_removes_backup, validate_failure_restores_backup, restart_failure_restores_backup, missing_backup_not_precondition. Test never touches real `/etc/dnsmasq.d/` or real `dnsmasq` binary. **Postmortem lesson:** When two destructive paths use the same rollback-on-validation-failure shape, every coverage gap on one is a guaranteed regression surface — `haproxy_rollback_test` set the contract; the dnsmasq twin had to follow.

- **`mod:15ba17da:use-slices-contains`** — done 2026-05-09 — no PR (already implemented in develop). Tier J minor (slices-maps). The planner's read of `internal/distribution/okd/destroy/steps.go::destroyTracker.terraformFailed` (L60-65) found `slices.Contains` already in place at L64 with `slices` imported at L6 — the audit Evidence describing a hand-rolled for-range loop reflected pre-migration state. The planner correctly returned `unresolved_questions` rather than inventing a no-op delta. Closed as `deferred-done` (no PR opened); archive presence is the canonical record. **Postmortem lesson:** Same shape as `smell:26a430ee` from the prior session — when the audit Evidence describes pre-migration state but the code has already been migrated, the planner's correct response is `unresolved_questions`, not a no-op delta. Trust the planner's read; verify and archive.

- **`sec:451be4fa:invokinguser-fallback-doc`** — done 2026-05-19 — PR #591, merge commit `cb7d56b`. Tier J suggestion (privilege-escalation). `internal/system/elevation.go::InvokingUser` falls back to `user.Current()` when `SUDO_USER` is unset; under direct root that returns root, and a downstream `InvokingUserHomeDir` → `ChownToInvokingUser` would leave files root-owned. No call site reaches the fallback under direct-root today (the `ChownTo*` family gates on `SUDO_UID`/`SUDO_GID` separately), so this was a preventive godoc-only change on both `InvokingUser` and `InvokingUserHomeDir`. **Postmortem lesson:** When a helper has a silent fallback that only bites a specific caller class, document the contract on the helper *and* the most likely unsafe consumer — the guard belongs where the reader lands, not just where the bug originates.

- **`sec:88fd3050:proxmox-username-no-redact`** — done 2026-05-19 — PR #592, merge commit `adb7351`. Tier J suggestion (redaction). `config.ProxmoxConfig.Username` is `json:"-"` but a plain string; a `slog.Any` on `*ProxmoxConfig` reaching a non-RedactHandler sink would emit it verbatim. Added a `Redacted() any` method returning a `redactedProxmoxConfig` projection that omits Username/Password/APIToken, mirroring `credentials/proxmox.go:redactedCredentials`; deliberately did NOT widen `logutil.secretKeyFragments` (cross-codebase blast radius). Required a follow-up fix commit (`adb7351`) — the first cut placed the method before the type and tripped `gocritic typeDefFirst`. **Postmortem lesson:** `gocritic typeDefFirst` is enforced — methods (and the helper structs they return) belong *after* the receiver type definition; darwin `make lint` skips it on `//go:build linux` files but CI runs `GOOS=linux`, so reproduce lint with the Linux GOOS before pushing (MEMORY.md `feedback_goos_lint`).

- **`sec:8e65d574:updatecheck-no-sig`** — done 2026-05-19 — PR #593, merge commit `9a913a8`. Tier J suggestion (tls-network). `version.BackgroundCheck` fetches the GitHub Releases API over TLS with no signature on the response body. Reachability requires a GitHub MITM; risk is bounded. Godoc-only change documenting that the response is unsigned, TLS to api.github.com is the sole trust anchor, and the notice is advisory — verify the binary via cosign/checksums on upgrade. **Postmortem lesson:** Bounded-risk network-trust findings resolve as a documented trust decision, not code — the value is making the implicit trust anchor explicit for the next auditor.

- **`sec:cfcdee2d:newinsecure-blast-radius`** — done 2026-05-19 — PR #594, merge commit `9dc2924`. Tier J suggestion (tls-network). `httputil.NewInsecure` is exported and reachable by any future caller; today's sole caller (bootstrap kube-vip healthcheck) is bounded by a fallback-after-`x509.HostnameError` pattern. Strengthened the godoc to state the contract (secure path first, insecure only on `x509.HostnameError`) and require every new caller to add a parallel test, citing `postinstall/haproxy_test.go:97-158` as the template; the existing `TestNewInsecureCallerPolicy` AST-walk already enforces caller scope. **Postmortem lesson:** For an intentionally-dangerous exported function, the godoc must carry the test contract for future callers — the policy test enforces *who* may call; the doc tells them *what they must prove*.

- **`sec:e076e43c:install-sh-trust-doc`** — done 2026-05-19 — PR #595, merge commit `bbafb46`. Tier J suggestion (file-toctou). `scripts/install.sh` extracts with `--no-same-permissions` (drops to umask) but the downstream `install -m 0755` sets the final mode correctly, so active risk is bounded. Comment-only: expanded the header into a 5-layer supply-chain trust ledger (TLS → cosign on SHA256SUMS → sha256 on archive → `--no-same-permissions` tar → `install -m 0755`). **Postmortem lesson:** A defense-in-depth shell script accumulates layers whose individual purpose is non-obvious; an enumerated trust-ledger comment is the WHY that stops a future maintainer from "simplifying" away a redundant-looking layer.

- **`state:62cb8a95:state-version-warn-only`** — done 2026-05-19 — PR #596, merge commit `a790d73`. Tier J major (state-schema-evolution). `checkStateMajorVersion` only ran on the destroy path; deploy/install/postinstall hit `terraform.Init` → `Apply` with no `terraform_version` preflight. Moved the function + constants from `destroy/helpers.go` into new `internal/infrastructure/terraform/state.go` and call it once at the top of `Executor.Init`, so all three paths get the preflight without per-call-site wiring; user-facing error message kept verbatim (helpers_test.go pins it); destroy-side call removed. **Postmortem lesson:** When a guard belongs on a shared chokepoint but lives on one caller, lift it to the chokepoint (`Executor.Init`) rather than copy it — symmetry-by-duplication is the smell the move eliminates.

- **`state:368b892b:cleanup-tfstate-preserved-but-orphan`** — done 2026-05-19 — PR #597, merge commit `4800fb0`. Tier J minor (state-schema-evolution). `cleanup` deliberately preserves `terraform.tfstate` for destroy re-runnability, but after a successful destroy the empty state orphans across cluster lifecycles and a stale `terraform_version` could silently mismatch new HCL. Added `PostDestroy bool` to `cleanup.Options`; the terraform cleanup step now removes `terraform.tfstate` via `SafeRemoveWithLogger` when `PostDestroy && !tf.HasState()`; `destroy/steps.go` sets `PostDestroy: !t.terraformFailed()`. Prepare flow uses `WorkOnly` which never reaches the terraform step, so the default-false is correct there. **Postmortem lesson:** A "deliberately preserved for resumability" file still needs an explicit terminal-state cleanup; the fix is a single signal flag threaded from the one caller that knows the operation succeeded, not a behavioural change to the preservation rule.

- **`state:881d089e:lock-stale-host-different`** — done 2026-05-19 — PR #598, merge commit `50a828d`. Tier J minor (tf-state-atomicity). `runlock.Acquire` never compared the lockfile's recorded `HOST=` against `os.Hostname()`; on a shared project tree (NFS/syncthing) a cross-host stale lock hangs the second operator with no actionable hint. Added `crossHostHint` parsing `HOST=` via `strings.CutPrefix`; on mismatch the conflict error appends the NFSv3 advisory (`fuser .okdctl.lock` before deleting). **Postmortem lesson:** The package doc already warned about NFSv3 flock semantics in source comments — surfacing that warning *at conflict time in the operator-facing error* is where it actually prevents the hang.

- **`state:b38ec9cc:workers-targeted-apply-vars-not-snapshot`** — done 2026-05-19 — PR #599, merge commit `5d27e72`. Tier J minor (phase-idempotency). `StepStartWorkers` was `ReRunSafeYes` with no `AlreadyDone` hook, so a re-`deploy` re-drives a terraform targeted-apply against an already-running cluster. Added `workersAlreadyRunning` that counts `oc get nodes -l node-role.kubernetes.io/worker`; the `AlreadyDone` hook self-skips when count ≥ `cfg.Topology.Workers.Count`. Cluster-unreachable returns `(false, nil)` so terraform runs as the safe fallback — annotated `//nolint:nilerr` (follow-up commit `5d27e72`) because the swallow is intentional. **Postmortem lesson:** `nilerr` fires on deliberate error-swallowing fallbacks; the correct response is a `//nolint:nilerr` with a WHY (not restructuring), but it must be reproduced with `GOOS=linux golangci-lint` before push or it round-trips through CI.

- **`state:b804b2ec:bootstrap-cleanup-vars-not-snapshot`** — done 2026-05-19 — PR #600, merge commit `9782983`. Tier J minor (phase-idempotency). `CleanupBootstrap` apply-overrode `bootstrap_enabled=false` but never persisted it, so a later `destroy` reads `bootstrap_enabled=true` from `terraform.tfvars` and shows a confusing recreate-bootstrap diff. After a successful CleanupBootstrap we now atomically write `bootstrap-state.auto.tfvars.json` (`{"bootstrap_enabled": false}`) into the env dir; terraform auto-loads `*.auto.tfvars.json` so subsequent plan/destroy is clean without mutating the user-authored tfvars. **Postmortem lesson:** Persisting an apply-time override belongs in a generated `*.auto.tfvars.json`, never by rewriting the operator's hand-authored `terraform.tfvars` — the auto-file is precedence-correct and leaves user intent untouched.

- **`state:f743eaa2:iso-build-fingerprint-not-fsynced`** — done 2026-05-19 — PR #601, merge commit `c9cbe3a`. Tier J suggestion (crash-recoverability). The `.fp-<node>` fingerprint was written by the `BuildCustomISOs` caller *after* `buildNodeISO` returned; a crash in that window leaves the ISO without its fingerprint and forces a wasteful rebuild next run. Moved the `system.AtomicWriteString` fingerprint write to the last line of `buildNodeISO` (after coreos-installer succeeds) by threading `fp`/`fpFile` params in. **Postmortem lesson:** A "write the success marker after the work" sequence must keep the marker write *inside* the function that did the work — co-location, not a caller-side afterthought, is what makes the crash reasoning sound.

- **`iac:18a795d5:worker-data-disk-no-prevent-destroy`** — done 2026-05-19 — PR #602, merge commit `710e2b4`. Tier J minor (hcl-destroy-ordering). The worker VM resource had no `prevent_destroy` and `disk` was absent from `lifecycle.ignore_changes`, so `terraform apply` with `worker_data_disk_size_gb = 0` silently destroys the 500 GiB Ceph data disk; masters are guarded by `prevent_destroy = true`. Took the narrower Option (b): added `disk` to the worker `lifecycle.ignore_changes` (3 identical blocks in main.tf — anchored the edit on the worker precondition to disambiguate). **Postmortem lesson:** `prevent_destroy` blocks *all* destroy (including legitimate worker removal); `ignore_changes` on `disk` is the surgical guard that freezes topology without blocking teardown — pick the narrowest lifecycle lever that closes the footgun.

- **`ux:4583b75b:config-describe-missing-long`** — done 2026-05-19 — PR #603, merge commit `0e1aab2`. Tier J minor (help-text). `configCmd` and `describeCmd` (parent verbs) registered only `Short`; sibling parents `addonCmd`/`releasesCmd` carry a `Long`. Added one-sentence `Long` to both naming the subcommands + entry point and regenerated `docs/cli/` via `make docs`. **Postmortem lesson:** cobra parent-verb help is a first-timer's map; match the established `Long` pattern of sibling parents and always regenerate `docs/cli/` in the same commit so the generated reference doesn't drift.

- **`obs:19a715fd:instructional-logs-via-info`** — done 2026-05-19 — PR #605, merge commit `97713d8`. Tier J suggestion (level-discipline). `secretstore.installPrereqCheck` emitted 7-line numbered setup procedures via `Logger.Info` (logs-as-UX) with string-concatenated paths in the message. Collapsed each of the three provider branches to a single `Warn` carrying a structured `docs` attr pointing at `docs/addons/secretstore.md`; net ~-12 LOC, no concatenated paths in messages. **Postmortem lesson:** Logs are an event stream, not a UX channel — multi-line instructional procedures belong in docs; the log emits one structured Warn with a doc pointer so JSON sinks stay clean.

- **`smell:632c9087:ingress-strategy-default-shadow`** — done 2026-05-19 — PR #606, merge commit `ad6bc7e`. Tier J minor (magic-strings). `IngressStrategy` had only two constants but `discoverIngressControllers` cast any API string verbatim, so a `NodePortService` controller silently flowed through HostNetwork-branch logic *and* missed the LB branch. Added `parseIngressStrategy(string) (IngressStrategy, bool)` returning `ok=false` outside the closed set; unknown strategies now emit a typed Warn and skip the controller instead of mis-routing. **Postmortem lesson:** A stringly-typed enum that absorbs unknown API values is a silent-misroute primitive — a closed-set parser with an explicit ok-false + warn-and-skip is the idiomatic narrowing.

- **`smell:92553fff:summary-hardcoded-3state-fmt`** — done 2026-05-19 — PR #607, merge commit `91d5267`. Tier J suggestion (magic-strings). `fmt.Sprintf("%-4s …")` hardcoded the max length of the current `stepDisplayStatus` values at two sites; a future "warn" state would silently misalign columns. Added `const stepStatusColWidth = 4` next to the status constants with a WHY comment and switched both sites to `%-*s` runtime-width injection. **Postmortem lesson:** A format width that encodes `max(len(enum values))` must be a named constant adjacent to the enum so adding a value forces a one-line update instead of a silent column drift.

- **`doc:beabab0c:pkg-doc-name-echo`** — done 2026-05-19 — PR #608, merge commit `0397ceb`. Tier J suggestion (package-doc). `setup` package doc echoed the package name for a ~2.3K-LOC phase. Replaced with a two-sentence description listing the actual surface (host packages + tool trio, install-config/manifests, custom CoreOS ISOs, HAProxy/dnsmasq/firewall) and naming the four step-group methods, verified against `steps.go`. **Postmortem lesson:** A name-echo package doc on the largest phase gives readers no map; the doc should list the concrete surface and the step-group entry points so a godoc reader can navigate without opening the file.

- **`tst:de572c63:destructive-happy-untested`** — done 2026-05-19 — PR #609, merge commit `2a3e5af`. Tier J major (destructive-untested). `dns.RestoreSystemResolver` removes `/etc/systemd/resolved.conf.d/dnsmasq.conf` gated only by a hardcoded `const` with no test seam. Lifted `resolvedConf` to a package var and `os.RemoveAll` to a `removeAllFn` var (matching the existing `dnsmasqConfigDir`/`validateDnsmasqConfigFn` pattern); added `restore_resolver_test.go` covering missing-dropin no-op, present-dropin removed, and RemoveAll-error logged-not-propagated. Tests never touch real `/etc`. **Postmortem lesson:** Same shape as `tst:d7ce9d16` — an `/etc`-touching destructive helper needs a package-var test seam; the var-override pattern was already established in the package, so the test had to follow it.

- **`con:8ea706f6:ctx-ignored-install-binary`** — done 2026-05-19 — PR #610, merge commit `60792cc`. Tier J minor (ctx-ignored). `installBinaryToPath(_ context.Context, …)` discarded ctx while doing a privileged copy+chmod into `/usr/local/bin` under sudo re-exec; a SIGINT mid-tools-loop couldn't abort before the next binary. Renamed `_` → `ctx` and added `if err := ctx.Err(); err != nil { return err }` at function entry; the sole call site already passes a live ctx. **Postmortem lesson:** Each invocation is bounded by a single cp+chmod, so a top-of-function `ctx.Err()` gate is sufficient — ctx-aware copy helpers would be over-engineering for a bounded operation.

- **`err:ddf885f4:install-all-bare-ctx-err`** — done 2026-05-19 — PR #611, merge commit `40c5c33`. Tier J suggestion (cancellation-identity). `addon.Manager.InstallAll` returns bare `ctx.Err()` at L85 (so `cli/root.go::signalExitCode` resolves SIGINT→130 without a typed wrap) but joins `ctxErr` into `errs` at L110 when partial failures exist; the asymmetry is intentional but was undocumented and a "simplification" could break either the exit-130 path or the partial-failure aggregate. Added a 3-line WHY comment naming both directions. **Postmortem lesson:** An intentional control-flow asymmetry with an exit-code contract is exactly a CLAUDE.md §code-comments rule-3 WHY comment — undocumented, it is a refactor landmine.

- **`ux:0d318f5c:log-format-tty-default-help-noise`** — done 2026-05-19 — PR #604, merge commit `2349379`. Tier J suggestion (streams). `configureLogging` auto-switches `--log-format` to json when stderr is piped, but cobra still rendered `(default "text")` in `--help`, so the help string lied. Took Option (b): replaced the help text with one naming both behaviours (`text (TTY default) | json (auto-selected when stderr is piped)`) and suppressed the cobra default render via `rootCmd.PersistentFlags().Lookup("log-format").DefValue = ""` after registration; the bound variable still defaults to `text` at parse time. Regenerated `docs/cli/` (the persistent-flag help propagates to all 25 command docs). The first two CI attempts hit a 46-minute `test-go` timeout (flaky infra, not a real failure — full suite passed locally); a third re-run after a clean rebase onto the post-merge develop went green. **Postmortem lesson:** `Flag.DefValue = ""` post-registration is the idiomatic cobra way to suppress a now-dishonest default without changing parse behaviour; and a 46-minute `test-go` duration with step `conclusion: null` is a CI timeout signature, not a test failure — re-run rather than chase a phantom regression.

- **`sub:7b2829bb:no-cancel-func`** — done 2026-05-19 — PR #630, merge commit `7438cd7`. Tier J major (timeout-cancel). `executor.RunInteractive` (and the shared `run` body + `RunStreamed`) used bare `exec.CommandContext` with no `cmd.Cancel`/`cmd.WaitDelay`, so ctx cancellation SIGKILLed the child; `terraform apply`/`destroy` route through `executor.run`, so a Ctrl-C orphaned `.terraform.tfstate.lock.info`. Set `cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGINT) }` + `cmd.WaitDelay = 30s` on all three `exec.CommandContext` sites (executor.go:227,275,330). First commit covered only RunInteractive; independent review FAILed it because the terraform mutation path is `run`, not RunInteractive — the fix was extended in a delta. **Postmortem lesson:** the roadmap Problem named `terraform.PlanStreamed`→RunInteractive, but the higher-blast-radius apply/destroy path is `executor.run`; when an Acceptance says "apply the same pair to Run/RunStreamed if they hold external locks", verify the *whole* call graph before scoping — a reviewer caught the half-fix.
- **`sec:48688e63:proxmox-host-no-revalidate`** — done 2026-05-19 — PR #631, merge commit `b05c9b7`. Tier J minor (input-validation). `proxmox.Provider.Connect` captured `cfg.Provider.Proxmox.Host` into `p.host` with no revalidation; wizard/programmatic Config construction bypasses `config.validateProxmoxConfig`, and `p.host` threads into pvesh argv + SSHRunArgv. Added `config.ValidateProxmoxHost(p.host)` immediately after capture (proxmox.go:128), returning `*errtypes.ConfigError` on failure. **Postmortem lesson:** load-path validation is not a substitute for seam validation when other constructors (wizard, tests) can build the struct directly — mirror the existing `validateProxmoxName`-at-pveshRun-boundary pattern at every place a user-controlled atom enters a subprocess argv.
- **`state:6424733c:project-marker-stale`** — done 2026-05-19 — PR #632, merge commit `bce2efa`. Tier J minor (crash-recoverability). `hasProjectMarker` is OR-of-three (okdctl.yaml | okdctl.env | terraform.tfstate); a lone leftover tfstate after destroy+cleanup made the check pass, so `deploy` could run a fresh default config against a different cluster's state. Took the lower-risk option: kept the boolean marker semantic (callers/tests unchanged) and added `warnIfTfStateOnly` in `resolveProjectRootOrDie` (helpers.go) emitting a structured `tui.Warn`+`tui.Info` naming tfstate as a recovery hint; +3 tests. **Postmortem lesson:** when a marker is intentionally permissive for resumability, don't tighten the predicate (breaks destroy/cleanup resume) — surface the ambiguous case with a structured operator warning and leave the boolean contract intact.
- **`api:ddf885f4:nil-logger-not-normalized`** — done 2026-05-19 — PR #633, merge commit `c392120`. Tier J suggestion (zero-value-usability). `addon.WithLogger` and `phase.WithLogger` set the logger field verbatim; nil-safety depended solely on a construction-end guard. Wrapped both with `logutil.OrNop(l)` inside the option (manager.go:35, paths.go:159), aligning with cluster/terraform `WithLogger`. The roadmap suggested dropping the construction-end guard "once each option is nil-safe"; that is wrong — `WithLogger` is optional, so a no-option `NewManager`/`NewBasePhase` still needs the guard for the nil zero-value. Independent reviewer confirmed keeping the guards. **Postmortem lesson:** a roadmap "drop the now-redundant guard" hint can be unsound when the option is optional; option-local nil-safety and the construction-end guard cover *different* paths (explicit-nil vs never-called) — keep both.
- **`api:25fa1be8:positional-logger`** — done 2026-05-19 — PR #634, merge commit `516d573`. Tier J minor (option-consistency). `firewall.{DetectBackend,Configure,RemoveRules,ConfigureOKD,RemoveOKDRules}` took `*slog.Logger` as a trailing positional arg, diverging from every sibling helper. Promoted to a `Firewall` struct with `Option`/`WithLogger`(logutil.OrNop)/`New`(NopLogger default), converted all five to methods, and updated every call site (destroy/steps.go hoists one `fw`; postinstall/haproxy.go + setup/steps.go inline). No dead free functions. The roadmap's call-site list named `cleanup/services.go`, but grep proved that file never calls firewall — the list was speculative. **Postmortem lesson:** verify the roadmap's enumerated call sites with grep before trusting them; here one listed site was wrong and trusting it blindly would have produced a phantom edit or a confused diff.
- **`ux:08c49fc4:keep-haproxy-no-shorthand-asymmetry`** — done 2026-05-19 — PR #635, merge commit `7f3f1b1`. Tier J suggestion (flag-conventions). The CLAUDE.md Flag naming convention block only covered `--output`/`-o`; the shorthand allowlist was implicit. Appended the codified allowlist (`-y -q -v -o -c`) and the long-form-only rule for per-command boolean tails. The roadmap loosely called these a "trio" and implied `--dry-run` had a shorthand; verifying registrations in `internal/cli` showed `--dry-run` is `BoolVar` (no shorthand), so the doc correctly lists it as long-form-only. **Postmortem lesson:** when codifying an existing convention, verify it against the code rather than the roadmap's prose — the prose mis-described `--dry-run`, and a docs PR that enshrined the wrong claim would have been worse than no doc.
- **`api:262af6e4:dual-option-types`** — done 2026-05-19 — PR #636, merge commit `334f3bf`. Tier J minor (option-consistency). `cleanup.Phase` carried two option surfaces: the canonical `phase.BasePhaseOption` (via `New`) and a redundant package-local `cleanup.Option`/`cleanupConfig`/`WithLogger` (via `Execute`) that only re-passed a logger `p.Log` already held. Dropped the local surface; `Execute(ctx, opts)` now uses `p.Log`; removed the `logger.With("phase","cleanup")` double-tag in `execute` (New already tags at construction); dropped the now-unused `logutil` import. The roadmap Fix said "the single caller (okd.go)" but grep found three (okd.go, destroy/steps.go, cli/cleanup.go) — all updated or it would not compile. **Postmortem lesson:** never trust a roadmap's caller count; grep the symbol — a "single caller" Fix that is actually three would have produced a non-compiling half-edit.
- **`state:262af6e4:cleanup-no-resume-doc`** — done 2026-05-19 — PR #636, merge commit `36724af`. Tier J suggestion (crash-recoverability). `cleanupTracker` joined step errors but `okdctl cleanup` surfaced only the final string — no per-subsystem diagnostic after a SIGINT mid-cleanup. `distribution.Orchestrator` exposes no `Results()`, so the tracker now records a subsystem name per failed step (`onError(name)`, `failedNames()`); `printSummary` warns `cleanup: partial cleanup; rerun to retry; subsystems still active` with a structured `subsystems` attr, and `runCleanup` emits a terminal-level `tui.Warn`. Sequenced on top of api:262af6e4 in one worktree (shared files); the original plan was re-planned against post-api code because api rewrote the exact `Execute` call site and signature. **Postmortem lesson:** when two picked items share files, sequencing beats parallel — and the second item's plan MUST be regenerated against the first item's committed code, not the stale base, or its `old_string`s will not match.
- **`smell:d6b325cb:duplicate-role-enum`** — done 2026-05-19 — PR #637, merge commit `a4a81a8`. Tier J minor (magic-strings). `proxmox.VMRole` (string + 3 literal consts) duplicated `phase.NodeRole` verbatim; `phase` was already imported for `VMState`. Replaced with `type VMRole = phase.NodeRole` and consts re-exporting `phase.Role*`; `VMStatus.Role` stays assignable. The planner's `new_string` dropped the const-block doc comment, tripping revive `exported: ... should have comment`; added a one-line block comment back before commit. **Postmortem lesson:** a type-collapse refactor that removes an exported declaration's doc comment will fail `revive` even when behaviour is correct — keep (or restore) a block doc comment on exported const groups; scoped lint caught it pre-commit.
- **`err:5013fea6:auth-error-string-sniffing`** — done 2026-05-19 — PR #638, merge commit `cecd9fa`. Tier J suggestion (string-sniffing). `isAuthError` substring-matched oc/registry stderr against a 7-marker list to split AuthError vs ClusterError (exit code is the primary signal; string match secondary). Dropped the over-broad `authentication`/`denied` markers (high false-positive vs benign network errors), keeping the HTTP-status-aligned five (`unauthorized`,`forbidden`,`no basic auth`,`401`,`403`); reworded the best-effort doc comment to stay accurate. No test asserted on the removed markers. **Postmortem lesson:** tightening a heuristic marker set is safe only after grepping tests for assertions on the removed tokens — verified none existed before trimming, so the errtypes contract stayed intact.
- **`dep:33ef32bf:proxmox-version-drift`** — done 2026-05-19 — PR #639, merge commit `a6d7194`. Tier J suggestion (maintenance-signal). CLAUDE.md §dependencies labelled `go-proxmox` `v0.4.x` while `go.mod` pins `v0.5.1`. One-line doc edit `v0.4.x`→`v0.5.x` (covers v0.5.1); bus-factor-1 caveat, call-site ref, and ~200-LOC REST fallback preserved verbatim. The roadmap Problem guessed `v0.5.0`; the planner read go.mod and used the actual `v0.5.1`→`v0.5.x`. **Postmortem lesson:** for a "track the actual pin" doc fix, read the pin from go.mod rather than the roadmap's remembered value — the roadmap said v0.5.0, reality was v0.5.1.
- **`ux:8154ab0f:doctor-pull-secret-config-skew-warns`** — done 2026-05-19 — PR #640, merge commit `02c0090`. Tier J suggestion (exit-codes). `checkPullSecret` warns when no config exists but fails when config exists with empty `pull_secret`; a first-time user passes `doctor` (exit 0) then `deploy` rejects with exit 65 on the same field. Took Option (a): no code change; added a "Warn vs fail split (by design)" note to `docs/doctor-checks.md` naming doctor as orientation-only and deploy as the enforcing gate. Reviewer independently verified all five factual claims against doctor.go/ignition.go/root.go. **Postmortem lesson:** a documented intentional split is the right resolution when escalating to fail would break `okdctl doctor && okdctl deploy` chained-on-success scripts — the doc records the contract instead of silently changing exit behaviour.
- **`dep:33ef32bf:exp-floor-stale-pseudoversion`** — done 2026-05-19 — PR #641, merge commit `579d16e`. Tier J suggestion (justified-version-floor). `golang.org/x/exp` pinned at a 2023-10-06 pseudo-version, transitive-only, zero okdctl call sites. `go get golang.org/x/exp@latest && go mod tidy` lifted the floor to `v0.0.0-20260508232706-...` and pruned ~17 stale go.sum lines; the `go`/`toolchain` directives were untouched (already `go 1.26.0` on develop, independent of this change — no downgrade). govulncheck is not installed locally; relied on the CI `security` job (passed). **Postmortem lesson:** a transitive-only floor refresh is a safe no-risk hygiene change, but the go-directive must be diffed explicitly after `go mod tidy` to prove no silent toolchain bump/downgrade slipped in — confirmed only the x/exp line moved.
- **`state:48688e63:proxmox-no-retry-on-init-apply`** — done 2026-05-19 — PR #656, merge commit `48508bc`. Tier J minor (proxmox-api-idempotency). `Provider.Provision`/`PlanOnly` called `p.terraformExec.Init(ctx)` bare; a transient network blip during provider-plugin download failed `StepDeployInfra` (the only fatal install step) and forced a full deploy re-run. Added `initWithRetry` (proxmox.go) — 3-attempt `wait.ExponentialBackoffWithContext` (5s base, ×2, jitter 0.5, 5m cap) gated by `initIsRetryable` (fast-fails config/auth/ctx-cancel, retries everything else); both call sites swapped. First independent review FAILed: the initial impl returned `wait.ErrWaitTimeout` on retryable-exhaustion, discarding the real terraform error, diverging from `internal/download/retry.go`; a delta added `lastErr` capture + the identical post-loop substitution, re-reviewed PASS. **Postmortem lesson:** `wait.ExponentialBackoffWithContext` swallows the closure's error on step-exhaustion — you must capture `lastErr` and substitute it (the `internal/download/retry.go` shape, gated on not-ctx-cancelled) or the operator sees a useless "timed out waiting for the condition"; an independent reviewer caught the half-fix that "reuse the download pattern" was supposed to prevent.
- **`state:eb479d86:upload-resume-not-supported`** — done 2026-05-19 — PR #657, merge commit `e3faad7`. Tier J minor (phase-idempotency). `uploadISOsViaSCP` scp'd every changed ISO in one subprocess; a SIGINT/network-drop mid-batch corrupted the partial file and the next run re-uploaded ALL ISOs (1–2 GiB each) because scp was a single batch invocation. Rewrote to per-file scp with a per-file `isoUploadNeeded` skip and a between-files `ctx.Err()` check; `args := append(slices.Clone(baseArgs), f, dest)` avoids the loop-append backing-array aliasing footgun; argv-mode preserved (no `sh -c` regression); the aggregate size summary stayed untouched in the caller `UploadCustomISOsToProxmox`. Reviewed PASS round 1. **Postmortem lesson:** batching a resumable transfer into one subprocess defeats idempotency — per-item invocation plus a per-item already-done probe is what makes Ctrl-C resumable; keep the rewrite scoped to the transfer loop so upstream summary/telemetry need no change.
- **`sec:a6e38cc7:keyscan-no-strict-baseline`** — done 2026-05-19 — PR #655, merge commit `bf2a074`. Tier J minor (input-validation). `sshpin.Verify`'s empty-fingerprint branch logged WARN and returned `("", nil)`, letting accept-new TOFU proceed with no fail-closed gate for security-sensitive deploys. Added `provider.proxmox.require_pinned_fingerprint` (bool, `omitempty`, default false) to `ProxmoxConfig` + `FieldProxmoxRequirePinnedFingerprint` + the redacted projection; `Verify`/`parseAndMatch` gained a `requirePinned` param returning `*errtypes.AuthError` when set and the fingerprint is empty; threaded through all four call sites; the proxmox `Connect` guard widened to `SSHHostFingerprint != "" || RequirePinnedFingerprint` so the gate is reachable. Reviewed PASS round 1. The branch had to be rebased twice via `git rebase --onto origin/develop <claim-sha>`: it was cut from a local `develop` carrying an unpushed roadmap-claim commit, which polluted the PR and broke `gh pr merge --rebase` once a sibling PR (#656) landed the identical claim content on `origin/develop`. **Postmortem lesson:** never cut feature worktrees from a local `develop` that holds the unpushed `chore(roadmap): claim` commit — keep the claim strictly on the pushed develop or branch from `origin/develop`; `git rebase --onto origin/develop <claim-sha> <branch>` cleanly strips a redundant base commit when this happens. Sibling item `sec:8ea706f6` (scoped "add a test") surfaced that the GPG trust-anchor constant is wrong (`AA16FCBC…` vs canonical `798AEC65…`); it was blocked for a maintainer decision rather than auto-corrected, since rewriting a trust anchor exceeds a test-only item's scope.

- **`api:7b2829bb:zeroize-asymmetry`** — done 2026-05-19 — PR #642, merge commit `fa5b58b6`. Tier J major (exported-surface). terraform.Executor.ZeroizeEnv hand-rolled a byte-identical credential-clearing body. The read-only planner found executor.Executor.ZeroizeEnv AND okd.Provisioner's delegation had ALREADY landed on develop; only terraform.go remained. Replaced its body with a nil-guarded t.exec.ZeroizeEnv() delegation (logutil.KeyIsSecret is a strict superset of the old 2-key allowlist, so plaintext lifetime is bounded at least as tightly). **Postmortem lesson:** A roadmap Fix that says "add X and replace two call sites" may be partially done already — the planner must read current code; here 2 of 3 sub-tasks were pre-landed and forcing all three would have been a redundant or conflicting diff.

- **`obs:48688e63:proxmox-probe-failure-as-info`** — done 2026-05-19 — PR #643, merge commit `7ac3cd6c`. Tier J suggestion (level-discipline). probeVMEnumeration logged three paths at Info; the two best-effort fallbacks (pvesh probe skipped / payload unparseable) belonged at Debug. Demoted exactly those two; the genuine retry-pending line stayed Info. Message strings and structured attrs unchanged, net 0 LOC. **Postmortem lesson:** Level-discipline fixes must keep message/attr text byte-identical — only the level method changes — so log-scrapers keyed on the message keep working.

- **`ux:fd2125dd:install-use-bracket-syntax`** — done 2026-05-19 — PR #644, merge commit `2d97b0fa`. Tier J minor (verb-noun). addonInstallCmd.Use was `install [name | --all]`; cobra Use is a positional-arg signature, so the flag/pipe mis-rendered. Changed to `install [name]`. CI `docs-go` then failed on CLI-reference drift; regenerated docs/cli/okdctl_addon_install.md via the docs generator and committed it in the same PR. **Postmortem lesson:** Any change to a cobra Use/Short/Long/flag must regenerate docs/cli/ in the SAME commit — the docs-go drift gate fails the PR otherwise; this is now a reflex for CLI-surface edits.

- **`mod:8ea706f6:strings-lines-fingerprint`** — done 2026-05-19 — PR #645, merge commit `702a0e1e`. Tier J suggestion (slices-maps). verifyHashiCorpGPGFingerprint walked gpg --with-colons output via strings.Split(out,"\n"). Switched to strings.Lines with a strings.TrimRight(line,"\n") applied BEFORE the HasPrefix/Split field extraction, so field-9 fingerprint comparison is byte-identical (strings.Lines yields the trailing newline; the old Split's spurious final empty element was already a no-op). **Postmortem lesson:** strings.Lines is not a drop-in for Split(s,"\n"): it retains the trailing newline and omits the final empty element — TrimRight before any field parse is mandatory for semantic equivalence.

- **`con:15ba17da:tracker-mu-not-needed-yet`** — done 2026-05-19 — PR #647, merge commit `672375b5`. Tier J suggestion (waitgroup-vs-errgroup). destroyTracker.mu is a forward-looking RWMutex (Orchestrator.Run is serial today) but, unlike the sibling distribution.PhaseContext mutex, carried no comment naming the future parallel-step intent. Added one WHY comment mirroring context.go; the mutex was preserved per MEMORY.md §scaffolding (do not delete forward-looking scaffolding). **Postmortem lesson:** Scaffolding flagged by an audit is resolved by documenting the intent (one WHY comment matching the canonical sibling), not by deleting it — the maintainer policy is keep-and-explain.

- **`err:9f8e7d6c:errtypes-vocab-cert-pending`** — done 2026-05-19 — PR #648, merge commit `5598908`. Tier J suggestion (domain-vocabulary). errtypes has no transient/recoverable concept; transient failures map to ClusterError(exit 4). Added a package-doc paragraph recording that the omission is intentional and deferred until a retry-aware consumer exists — NO speculative TransientError type was added (premature API). **Postmortem lesson:** A "missing concept" audit item on a taxonomy is closed by a deferral-rationale doc comment, not by speculatively adding the type — add the type when the first real consumer lands, not before.

- **`dep:33ef32bf:yaml-quad-engines`** — done 2026-05-19 — PR #649, merge commit `3243cdd1`. Tier J minor (duplicate-engine). CLAUDE.md YAML tripwire claimed "three / down from four" while go.sum has four engines. Rewrote it to state four (sigs.k8s.io/yaml direct; go.yaml.in/yaml/v{2,3} //indirect; gopkg.in/yaml.v3 purely transitive via testify→check.v1) and "do not add a fifth". Independent review round-1 FAILed: the first draft claimed sigs.k8s.io/yaml was //indirect (it is a direct require); a delta corrected the direct/indirect attribution, re-reviewed PASS. **Postmortem lesson:** Doc claims about go.mod direct-vs-indirect must be verified against go.mod itself — an independent reviewer caught a self-contradictory sentence ("direct" then "//indirect" for the same module).

- **`err:6424733c:env-file-double-context`** — done 2026-05-19 — PR #650, merge commit `4ea97985`. Tier J minor (wrapping). Three CLI sites wrapped credentials.LoadEnvFile errors with an outer "load env file <path>: %w" on top of an inner ConfigError/AuthError that already names the path — doubled operator context. Dropped the outer wrap at helpers.go + deploy.go x2; the inner typed error still resolves via errors.As so exit-code mapping is preserved; fmt stays imported (used elsewhere). **Postmortem lesson:** Removing a redundant wrap is safe only after confirming the inner error both embeds the path AND is still reachable by errors.As for the exit-code contract — both were verified before dropping.

- **`api:0139cb3f:bin-dir-fan-out`** — done 2026-05-19 — PR #651, merge commit `8306708f`. Tier J suggestion (exported-surface). The phase.{ResolveBinDir,PreflightBinDir,BinDirOrDefault} three-function surface is intentional scaffolding. Anchored the rationale + roadmap id on BinDirOrDefault and added concise cross-references on the other two; no function deleted/merged (MEMORY.md §scaffolding). **Postmortem lesson:** A multi-function scaffolding surface is best documented with ONE anchor comment carrying the rationale + roadmap id and short pointers from the siblings — keeps comment density low while making the next audit no-op.

- **`api:e2343d2c:unused-trailing-param`** — done 2026-05-19 — PR #652, merge commit `9fe9867d`. Tier J suggestion (exported-surface). system.ManageService(ctx,action,serviceName,_ string) had a provably-unused 4th param. Dropped it and swept all 12 call sites (postinstall/haproxy, setup/haproxy, setup/apache, cleanup/services, dns/dnsmasq) to 3-arg; no _test.go referenced it. systemd.go uses runtime.GOOS (no build tag) so darwin build/vet covered it. **Postmortem lesson:** A signature-narrowing sweep needs a grep-proven complete call-site list before the edit — confirmed zero remaining 4-arg calls and zero test references prior to committing.

- **`api:fde34e0c:k8sclient-pkg-stutter`** — done 2026-05-19 — PR #653, merge commit `f756853c`. Tier J suggestion (exported-surface). Renamed cluster.K8sClient→Client and NewK8sClient→New across the package, receivers, doc comments, the single external caller (install/monitor.go) and tests. The roadmap claimed "five call sites"; grep showed exactly one external production consumer — scope was set from the grep, not the roadmap. **Postmortem lesson:** Trust grep over the roadmap's remembered call-site count: the stutter-rename's real blast radius was 1 external site, not the 5 the entry guessed — verifying first kept the parallel-batch isolation valid.

- **`smell:daf5bee9:any-yaml-traversal`** — done 2026-05-19 — PR #654, merge commit `5295cd54`. Tier J suggestion (interfaceany-lazy). namedEntries/mergeNamedList walked the kubeconfig as map[string]any of []any with fragile assertions. Replaced with a json.RawMessage-backed typed model (toKubeEntries/fromKubeEntries); sigs.k8s.io/yaml is itself a JSON codec so the round-trip adds zero loss. Added TestMergeKubeconfig_PreservesUnknownFields proving an unknown extension key survives an end-to-end merge; no-clobber semantics preserved. **Postmortem lesson:** A typed remodel of a passthrough merge is only safe with an explicit round-trip-fidelity test on an unknown field — the json.RawMessage shape is lossless precisely because sigs.k8s.io/yaml marshals through encoding/json.

- **`smell:5013fea6:auth-error-string-sniff`** — done 2026-05-19 — PR #646, merge commit `6d7ebf68`. Tier J suggestion (magic-strings). isAuthError's acknowledged-tech-debt boundary was already documented by an earlier merged commit (cecd9fa); the only remaining defect was a stale roadmap-id cross-reference in the comment (err:5013fea6 → smell:5013fea6). One-character correction so the boundary comment is grep-accurate. CI test-go was flaky twice on this comment-only change (passed clean on rerun). **Postmortem lesson:** When a prior commit already satisfied the substance of an item, the residual fix can be a single grep-accuracy correction — and a comment-only change failing test-go is a CI-infra flake signature, not a regression: rerun, do not chase.

- **`mod:eb479d86:use-slices-containsfunc`** — done 2026-05-19 — PR — (no PR), merge commit `develop`. Tier J suggestion (slices-maps). Picked and claimed during /roadmap-pickup, but the read-only planner found it ALREADY implemented on develop (internal/distribution/okd/setup/upload.go:190-194 already uses slices.ContainsFunc with the exact prescribed closure). No worktree was mutated and no PR was opened; the item is resolved by prior unrelated work. **Postmortem lesson:** The plan-then-execute split paid for itself: a read-only planner detected an already-done item before any branch/PR was created, turning a wasted PR into a zero-cost archive — a direct-write agent would have produced an empty/thrashing diff.

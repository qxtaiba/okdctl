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


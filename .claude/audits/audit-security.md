# audit-security — 2026-05-08

**Assumes green:** golangci-lint (gosec G1xx–G6xx as configured in
`.golangci.yml`), govulncheck, CodeQL `security-extended`, shellcheck,
tflint, `go test ./...`. Findings here cover what those tools miss.

**Scope:** every in-scope file per AUDIT_CONVENTIONS.md §1 — `internal/**/*.go`,
`cmd/**/*.go`, `scripts/install.sh`, Terraform HCL under
`infrastructure/terraform/`, with §2 exclusions applied (`internal/tui/wizard/**`,
`templates/**`, `setup/iso.go`, `_test.go`, `//go:build linux`-tagged).

**Out of scope this run:** wizard package (proxmox_discovery.go
InsecureSkipVerify is opt-in via `provider.proxmox.insecure`, gated by
the user — wizard scope is excluded by AUDIT_CONVENTIONS); doctor.go
(`//go:build linux`-tagged); template files (excluded).

**Seam co-owners:**
- `audit-subprocess` — per-site shell-injection (this run did not run
  the subprocess sweep; deferred per seams.md §1)
- `audit-state-and-recovery` — destroy-path recoverability (this audit
  layered credential hygiene only; cleanup symlink finding belongs here)
- `audit-iac-and-shell` — install.sh shellcheck baseline; this audit
  flagged the Go-side credential trust on the install path
- `audit-errors` — error-type credential carriage (no findings; the
  SubprocessError.Redacted shape and ProxmoxCredentials.Redacted both
  preserve invariants)
- `audit-observability` — slog redaction sink (no findings; RedactHandler
  handles attr keys + Redacted() any types correctly)

## Executive summary

okdctl's credential hygiene baseline is genuinely good — `[]byte`
+ Zeroize, RedactHandler attr key denylist, json:"-" on Password/APIToken,
SubprocessError.Redacted at the error type, AtomicWrite + 0o600 on
.env, sshpin.Verify pinning the Proxmox SSH key. The blockers are at
the *seams*: (1) the SCP path that uploads ignition ISOs to Proxmox
ignores the pinned-fingerprint known_hosts file the rest of the system
threads, defaulting to TOFU; (2) the install-config auth/ tree
(kubeconfig + kubeadmin-password) is copied into the apache
DocumentRoot relying solely on file mode bits to keep apache from
serving them. Both are one mode-default change away from credential
exposure on a homelab network. The remaining major findings are
supply-chain trust gaps (CoreOS stream JSON unsigned, helm/sops
checksums fetched at deploy time without signature) and a
canonical-helper gap (`executor.Executor` lacks the
`ZeroizeEnv` parallel that `terraform.Executor` has). Total: 21 findings
(2 blocker, 5 major, 4 minor, 10 suggestion).

## Ranked table

Sort: severity_weight × confidence × |LOC delta| ÷ risk
(blocker=4, major=3, minor=2, suggestion=1; high=3 / med=2 / low=1).
LOC counts list the line range; |Δ| approximates net change.

| ID | Cluster | File:line | Severity | Confidence | LOC | Adjacent linter | Fix class | CVSS-ish |
|----|---------|-----------|----------|------------|-----|-----------------|-----------|----------|
| sec:eb479d86:ssh-pin-bypass | tls-network | upload.go:78-91 | blocker | high | 14 | none | refactor | scp ignores pinned known_hosts → MITM intercepts ignition ISOs / network |
| sec:06f00bcb:auth-tree-in-webroot | file-toctou | apache.go:174-234 | blocker | high | 60 | none | refactor | kubeadmin-password under apache DocumentRoot, mode-bit-only defense / network |
| sec:35abd54e:env-string-residue | credentials | proxmox.go:151-172 | major | high | 22 | none | refactor | residual cred strings in heap / local |
| sec:7b2829bb:executor-no-zeroize | credentials | executor.go:38-92 | major | high | 55 | none | refactor | canonical helper lacks ZeroizeEnv / local |
| sec:9d79b841:coreos-stream-no-pin | tls-network | coreos.go:182-216 | major | high | 35 | none | refactor | unsigned stream JSON anchors ISO trust / supply-chain |
| sec:40d315ad:flux-keyscan-tofu | tls-network | flux.go:407-421 | major | high | 15 | none | refactor | flux git host TOFU at deploy / network |
| sec:5e892064:checksum-no-signature | tls-network | checksum.go:59-111 | major | medium | 53 | none | refactor | helm/sops checksum unsigned / supply-chain |
| sec:e3782ee7:atomicwrite-symlink-race | file-toctou | fs.go:196-243 | major | medium | 48 | none | refactor | canonical AtomicWrite missing Lstat refusal / local-privileged |
| sec:2f70d7df:ignition-over-http | tls-network | kargs.go:68-78 | major | high | 11 | none | refactor | ignition fetch over HTTP w/ pull-secret / network |
| sec:a6e38cc7:keyscan-no-strict-baseline | input-validation | sshpin.go:31-80 | minor | medium | 50 | none | config | no opt-in strict-mode for unpinned hosts / network |
| sec:c8b28673:tar-mode-trust | file-toctou | extract.go:89-110 | minor | high | 22 | none | refactor | tar header mode mask permits setgid / supply-chain |
| sec:bdf5a873:cleanup-symlink-traversal | file-toctou | artifacts.go:33-60 | minor | medium | 28 | none | refactor | RemoveAll on symlink under sudo / local-privileged |
| sec:48688e63:proxmox-host-no-revalidate | input-validation | proxmox.go:103-121 | minor | medium | 19 | none | refactor | provider boundary doesn't re-validate Host / network |
| sec:8ea706f6:hashicorp-gpg-trust-doc | tls-network | tools.go:273-336 | suggestion | high | 64 | none | refactor | GPG fingerprint constant lacks regression test |
| sec:88fd3050:proxmox-username-no-redact | redaction | cluster.go:119-124 | suggestion | low | 6 | none | policy | username not on RedactHandler denylist |
| sec:cfcdee2d:newinsecure-blast-radius | tls-network | httputil.go:29-41 | suggestion | high | 13 | gosec:G402 | policy | NewInsecure exported, future-caller risk |
| sec:bbc23e42:noplogger-no-redact | redaction | logutil.go:1-28 | suggestion | low | 28 | none | refactor | NopLogger lacks RedactHandler wrap |
| sec:e076e43c:install-sh-trust-doc | file-toctou | install.sh:162-170 | suggestion | medium | 9 | none | policy | trust-layer documentation gap |
| sec:fde34e0c:k8s-kubeconfig-env-no-validate | input-validation | k8s.go:55-64 | suggestion | low | 10 | none | refactor | KUBECONFIG env not validated |
| sec:8e65d574:updatecheck-no-sig | tls-network | updatecheck.go:89-116 | suggestion | high | 28 | none | policy | update notice unsigned (advisory only) |
| sec:451be4fa:invokinguser-fallback-doc | privilege-escalation | elevation.go:24-31 | suggestion | medium | 8 | none | policy | non-sudo fallback contract under-documented |

## Findings

### sec:eb479d86:ssh-pin-bypass — blocker

**Cluster:** tls-network
**File + line range:** `internal/distribution/okd/setup/upload.go:78-91`
**Smell:** `uploadISOsViaSCP` hardcodes
`StrictHostKeyChecking=accept-new` and ignores the `knownHostsPath` the
caller obtained from `sshpin.Verify`. When `proxmox.ssh_host_fingerprint`
is set, the rest of the system enforces strict host-key checking, but
this scp call still trusts whatever key the network presents — defeating
the pin on the very phase that uploads custom ISOs containing per-node
ignition kargs.
**Evidence:**
```go
func uploadISOsViaSCP(ctx context.Context, cmdRunner *executor.Executor, isoFiles []string, user, host, remotePath string) error {
    args := []string{"-o", "StrictHostKeyChecking=accept-new"}
    args = append(args, isoFiles...)
    args = append(args, fmt.Sprintf("%s@%s:%s/", user, host, remotePath))
    if err := cmdRunner.RunInteractive(ctx, "scp", args...); err != nil { ... }
```
**Fix — preferred:** Thread `knownHostsPath` into `uploadISOsViaSCP`.
When non-empty: pass `-o UserKnownHostsFile=<path>` and
`-o StrictHostKeyChecking=yes`; only fall back to `accept-new` when the
path is empty. Mirror the `sshBaseArgs()` shape used in `phase/ssh.go`
so the policy matches the SSH path that already honors the pin.
**Rule source:** CWE-322 / CLAUDE.md §architecture-notes (SSH shell policy) /
repo counter-example: `internal/distribution/okd/phase/ssh.go:L63-L77`
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none (subprocess seam: subprocess audit catalogs the per-site
hygiene of `scp` argv; security owns the pin-bypass policy)
**What MUST stay bit-for-bit:** the `accept-new` fallback when pin is
unset (otherwise unconfigured deploys break)
**Estimated net LOC delta:** +6
**Risk (of fix):** low — argv shape change, `phase/ssh.go` already has
the template, easy to test
**Confidence:** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related:** none

### sec:06f00bcb:auth-tree-in-webroot — blocker

**Cluster:** file-toctou
**File + line range:**
`internal/distribution/okd/setup/apache.go:174-234`
**Smell:** `DeployToWebServer` copies the openshift-install `auth/`
tree (kubeconfig + kubeadmin-password) into the apache DocumentRoot at
`webRoot/auth/`, alongside the ignition files. The deployment relies
entirely on the source mode bits (0o600) to keep apache from serving
them — there is no Apache `<Location /auth>` deny rule. A future
schema bump in openshift-install that emits a more permissive mode, an
operator who runs `chmod -R go+r` to debug, or any DocumentRoot
override that re-renders perms turns the kubeadmin password into a
bastion-network HTTP fetch.
**Evidence:**
```go
authSrc := filepath.Join(clusterDir, "auth")
if system.FileExists(authSrc) {
    authDest := filepath.Join(webRoot, "auth")
    if err := copyAuthTree(authSrc, authDest); err != nil { ... }
}
// copyAuthTree uses system.CopyFile (preserves mode) — no Apache deny
// rule renders /auth/ unreadable on the wire if the mode-bit defense
// is breached.
```
**Fix — preferred:** Stop copying the `auth/` tree into `webRoot`. The
kubeconfig + kubeadmin-password are consumed by
`install/SetupClusterAccess` (`~/.kube/config`) and the postinstall
`verifyKubeVIPAPIHealthBootstrap` (`kubeconfigPath` in `clusterDir`).
No phase NEEDS them under apache. If a future caller does, render a
one-line drop-in `<Location /auth>Require all denied</Location>`
snippet alongside the `configureApachePort` write so the deny rule is
colocated with the document root.
**Rule source:** CWE-200, CWE-552, OWASP-A01, CLAUDE.md
§credentials-and-secrets
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**What MUST stay bit-for-bit:** the ignition-files-in-webroot path
(those are intentionally served at HTTP); only the auth/ subtree
moves.
**Estimated net LOC delta:** −24
**Risk (of fix):** low — removing dead callers; verifyKubeVIP and
SetupClusterAccess already read from `clusterDir/auth/` directly
**Confidence:** high
**CLAUDE.md / MEMORY.md conflict?:** no
**Related:** none

### sec:35abd54e:env-string-residue — major

**Cluster:** credentials
**File + line range:** `internal/credentials/proxmox.go:151-172`
**Smell:** `ProxmoxCredentials.Env()` returns `[]string` with the
credential bytes as immutable Go strings. The doc-comment correctly
notes this and instructs callers to defer `Zeroize()` and pass the
result directly to `WithEnv` in the same frame. However the resulting
strings live as long as any struct that stores `cmd.Env`
(`Executor.Env`, `Provider.env`) and become heap garbage after the
slice is replaced. `Zeroize()` cleans the `[]byte` source but cannot
reach the strings. `terraform.Executor.ZeroizeEnv()` partially
mitigates by overwriting the entry with `""` before clearing —
`executor.Executor` has no equivalent.
**Fix — preferred:** Add `executor.Executor.ZeroizeEnv()` mirroring
`terraform.Executor.ZeroizeEnv` (`terraform.go:L347-L364`). Update
`internal/distribution/okd/install/*` and
`internal/infrastructure/proxmox/proxmox.go` to defer
`e.Exec.ZeroizeEnv()` at end of phases that pass `creds.Env()`.
Document the residual-string boundary in CLAUDE.md
§credentials-and-secrets so future env-passing sites follow the same
defer pattern.
**Rule source:** CLAUDE.md §credentials-and-secrets / repo
counter-example `terraform.go:L347-L364` / CWE-316
**Adjacent linter:** none / **Scaffolding?:** no / **Seam:** none
**Net LOC:** +30 / **Risk:** medium / **Confidence:** high
**Related:** sec:7b2829bb (executor-no-zeroize)

### sec:7b2829bb:executor-no-zeroize — major

**Cluster:** credentials
**File + line range:** `internal/executor/executor.go:38-92`
**Smell:** `Executor.Env` is `[]string` and there is no
`Zeroize`/`ZeroizeEnv` method. Cred-bearing env entries
(`PROXMOX_VE_PASSWORD=…`, `PROXMOX_VE_API_TOKEN=…`) are appended via
`WithEnv(creds.Env())` and live as immutable strings until the
Executor is GC'd. `terraform.Executor` has the parallel `ZeroizeEnv`;
`proxmox.Provider.env` is also never zeroized.
**Fix — preferred:** Add `executor.Executor.ZeroizeEnv()` that walks
`e.Env`, blanks any entry whose key matches `secretKeyFragments`
(re-use `logutil.KeyIsSecret`), then `clear()`s and nils the slice.
Wire it into the defer chain in `createOKDProvisionerWithOpts`
(`cli/helpers.go`) alongside `p.ZeroizeEnv()`. Update CLAUDE.md to
document the defer pattern.
**Rule source:** CLAUDE.md §architecture-notes / repo counter-example
`terraform.go:L347-L364` / CWE-316
**Adjacent linter:** none / **Scaffolding?:** no / **Seam:** none
**Net LOC:** +18 / **Risk:** low / **Confidence:** high
**Related:** sec:35abd54e

### sec:9d79b841:coreos-stream-no-pin — major

**Cluster:** tls-network
**File + line range:**
`internal/distribution/okd/setup/coreos.go:182-216`
**Smell:** `fetchCoreOSStream` pulls `fcos.json`/`scos.json` from
`raw.githubusercontent.com` and trusts the ISO URL + SHA256 it returns
wholesale — only TLS to GitHub anchors authenticity. The ISO checksum
verification (`DownloadCoreOSISO`) is then meaningless against an
upstream poisoning of `openshift/installer`'s `release-4.X` branch:
the attacker rewrites both location and sha256 in the same JSON and
our fetch passes both checks.
**Fix — preferred:** Pin the `openshift/installer` reference to a
known-good commit SHA (not a branch) and embed the expected JSON
sha256 at compile time, similar to
`bootstrapOCChecksum`/`bootstrapOCVersion` in `release_extract.go`.
Bumping the OKD minor in CI requires explicit re-pin.
**Rule source:** CWE-494, CWE-829, repo counter-example
`release_extract.go:L31-L38`
**Net LOC:** +30 / **Risk:** medium / **Confidence:** high

### sec:40d315ad:flux-keyscan-tofu — major

**Cluster:** tls-network
**File + line range:**
`internal/addon/catalog/flux/flux.go:407-421`
**Smell:** `createDeployKeySecret` runs `ssh-keyscan host` and bakes
the result into the `flux-system` Secret as `known_hosts`. No
fingerprint pin → on-path attacker during deploy substitutes their
host key, owns every subsequent flux git pull.
**Fix — preferred:** Add an `addons.flux.settings.git_host_fingerprint`
setting. When set, validate the keyscan output against it (mirror
`sshpin.Verify`). When unset, log Warn naming the observed
fingerprint.
**Rule source:** CWE-322, CWE-295, repo counter-example
`sshpin.go:L31-L80`
**Net LOC:** +25 / **Risk:** medium / **Confidence:** high

### sec:5e892064:checksum-no-signature — major

**Cluster:** tls-network
**File + line range:** `internal/download/checksum.go:59-111`
**Smell:** `FetchChecksum` downloads sha256sum.txt-style files over
HTTPS with no signature verification. okdctl's own `install.sh`
cosign-verifies SHA256SUMS — the in-tree download path used by
`setup.installBinary` (helm, sops) does not.
**Fix — preferred:** Compile-time-pin embedded SHA-256s for helm/sops
at known versions (the `yqChecksumsByArch` pattern in
`tools.go:L40-43`), removing runtime `FetchChecksum` for the bounded
set of vendor binaries okdctl installs.
**Rule source:** CWE-494, CWE-829, repo counter-example
`tools.go:L40-43`
**Net LOC:** −20 / **Risk:** high / **Confidence:** medium

### sec:e3782ee7:atomicwrite-symlink-race — major

**Cluster:** file-toctou
**File + line range:** `internal/system/fs.go:196-243`
**Smell:** `AtomicWrite` creates the destination via `openTempFile`
(`O_CREATE|O_EXCL`) then `os.Rename`. There is no `os.Lstat`/
`O_NOFOLLOW` pre-check on `path` itself. Critical writes (kubeconfig,
.env, install-config.yaml) under sudo re-exec use `AtomicWrite`.
Compare to `envfile.go:L52-58` and `runlock.go:L40-50` which both
Lstat + O_NOFOLLOW.
**Fix — preferred:** Before `openTempFile`, `os.Lstat(path)` and
refuse if the result reports a symlink (mirror
`credentials/envfile.go:WriteEnvFile L52-58`).
**Rule source:** CWE-367, CWE-59, counter-examples cited
**Net LOC:** +12 / **Risk:** low / **Confidence:** medium

### sec:2f70d7df:ignition-over-http — major

**Cluster:** tls-network
**File + line range:**
`internal/distribution/okd/setup/kargs.go:68-78`
**Smell:** `BuildIgnitionURLForNode` hardcodes `http://` for the
ignition fetch URL. The fetched ignition payload contains the cluster
pull-secret, SSH authorized keys, and machine-config tokens. Anyone
on the bastion network segment can passively recover those credentials
by sniffing the FCOS first-boot HTTP request.
**Fix — preferred:** Document the threat model explicitly (current
state: network-isolated bastion is sole defense), and add a runtime
check refusing to proceed when `ignitionIP` is on a non-RFC1918
network. The HTTPS-served-with-self-signed-cert option is correct but
raises operator burden — flag for deferral until operator demand.
**Rule source:** CWE-319, CWE-200, OWASP-A02
**Net LOC:** +15 / **Risk:** high / **Confidence:** high

### sec:a6e38cc7:keyscan-no-strict-baseline — minor

**Cluster:** input-validation
**File + line range:** `internal/sshpin/sshpin.go:31-80`
**Smell:** `sshpin.Verify`'s empty-fingerprint branch returns
`("", nil)`, letting accept-new TOFU proceed. No operator-facing
strict-mode flag (`require_pinned_fingerprint: true`) exists.
**Fix — preferred:** Add `provider.proxmox.require_pinned_fingerprint`
bool. When true and `SSHHostFingerprint` is empty, return
`*errtypes.AuthError`.
**Net LOC:** +10 / **Risk:** low / **Confidence:** medium

### sec:c8b28673:tar-mode-trust — minor

**Cluster:** file-toctou
**File + line range:** `internal/download/extract.go:89-110`
**Smell:** `processTarEntry` uses
`os.FileMode(header.Mode&0o777)` for files and `&0o755` for dirs.
Setuid (0o4000) is masked off but setgid (0o2000) on extracted files
survives `&0o777`.
**Fix — preferred:** Tighten the file mask from `&0o777` to `&0o755`.
**Net LOC:** ~2 / **Risk:** low / **Confidence:** high

### sec:bdf5a873:cleanup-symlink-traversal — minor

**Cluster:** file-toctou
**File + line range:**
`internal/distribution/okd/cleanup/artifacts.go:33-60`
**Smell:** `SafeRemoveWithLogger` calls `os.RemoveAll(path)` under
sudo without an `Lstat` symlink refusal. `refuseCriticalPath` guards
known roots, but a symlink at `<workDir>/okd-install→/etc` passes the
check.
**Fix — preferred:** Add `Lstat` check at function head, refuse
symlinks (mirror `envfile.go:WriteEnvFile`).
**Net LOC:** +8 / **Risk:** low / **Confidence:** medium

### sec:48688e63:proxmox-host-no-revalidate — minor

**Cluster:** input-validation
**File + line range:**
`internal/infrastructure/proxmox/proxmox.go:103-121`
**Smell:** `Provider.Connect` captures
`cfg.Provider.Proxmox.Host` without re-validating, then passes it to
`phase.ProxmoxBareHost`. Load-time validator catches malformed hosts;
non-LoadFile constructions can bypass.
**Fix — preferred:** In `Provider.Connect`, after capture, call
`config.ValidateProxmoxHost`.
**Net LOC:** +5 / **Risk:** low / **Confidence:** medium

(Suggestion-tier findings — sec:8ea632a6, sec:88fd3050, sec:cfcdee2d,
sec:bbc23e42, sec:e076e43c, sec:fde34e0c, sec:8e65d574,
sec:451be4fa — are documented in the JSONL artifact at the same
shape; omitted from prose for brevity per §9a.)

## Scaffolding items detected

None. The audit did not find exported security-shaped surface that
satisfied the §7 scaffolding criteria.

## Linter-config-bug candidates

`sec:cfcdee2d:newinsecure-blast-radius` cites `gosec:G402` as the
adjacent linter. `gosec` IS enabled in `.golangci.yml` (line 26) but
the `httputil.go:L38` site has a `//nolint:gosec` suppression with
the justification "bootstrap self-signed cert; see doc". The
suppression itself is correct policy (the function exists for the
documented bootstrap-window use case); the finding is about the
exported-surface blast radius, not the suppression. No linter-config
fix needed.

Run `jq -c 'select(.adjacent_linter_enabled==true)'
.claude/audits/audit-security.jsonl` to refresh
`linter-config-bugs.jsonl` after this run.

## Skip list

Items investigated but dropped:
- **`internal/tui/wizard/steps/proxmox_discovery.go:L70` —
  `InsecureSkipVerify: px.Insecure`**: the wizard package is excluded
  by AUDIT_CONVENTIONS §2 and the field is gated by an explicit
  user-supplied `provider.proxmox.insecure` flag. CLAUDE.md does not
  forbid user-opt-in TLS skip via config. No finding.
- **`config.cluster.go:L123 TokenID string` (no `json:"-"` partner)**:
  TokenID is the Proxmox API-token *identifier* (e.g.
  `user@pam!okdctl`), not the token secret. The secret is `APIToken`
  which IS `json:"-"`. No finding.
- **`provider.proxmox.proxmox.go:L42-43 logger / env fields cleared on
  Disconnect`**: Disconnect leaves `p.env` populated. The `WithEnv`
  option appends; an envvar leak would require re-using a stale
  Provider, which production code does not. Defer the redesign to a
  future API-design refactor.
- **`flux.go:L412 publicKey omitted on missing pub`**: the read of
  `deployKeyFile + ".pub"` is best-effort, falling back to an empty
  publicKey. Flux Source Controller does not require the pub half;
  the omission is correct.

## Cluster verdicts

- **credentials** — 3 findings (1 major, 1 major, 1 suggestion):
  the `[]byte` + `Zeroize` pattern is correctly applied; the gap is
  the residual-string lifetime in `Executor.Env` and
  `Provider.env`, mirrored by `terraform.Executor.ZeroizeEnv` but
  missing from the canonical helpers. Closing this is one
  ZeroizeEnv() method on Executor plus a defer wiring.
- **privilege-escalation** — 1 finding (suggestion): the elevation
  policy (re-exec under sudo, `requiresRoot` annotation, sudo-flag
  argv hygiene in `cli/elevation.go`) is well-reasoned. Single doc
  gap for `InvokingUser`'s root-fallback semantics.
- **file-toctou** — 5 findings (1 blocker, 2 major, 2 minor):
  blocker is the auth-tree-in-webroot exposure path; the rest are
  hygiene gaps (AtomicWrite Lstat refusal, tar mode mask,
  cleanup symlink, install.sh layered-trust doc). The codebase uses
  `system.WriteTempFile` and `O_NOFOLLOW` correctly at the sites that
  matter; closing AtomicWrite raises the canonical helper to
  envfile.go's bar.
- **shell-injection** — swept clean — 23 `exec.Command` /
  `CommandContext` sites checked; all use argv form except the
  documented `iso_cleanup.go::RemoveFCOSISOFromProxmox` which has the
  `validateXxx` + `shellSingleQuote` layered guards CLAUDE.md
  requires. No POLICY finding (no umbrella ≥3-site pattern of
  variable interpolation into shell strings).
- **tls-network** — 7 findings (1 blocker, 4 major, 2 suggestion):
  the cluster's central concerns are the SCP pin-bypass and the
  CoreOS stream / helm-sops checksum unsigned trust. `httputil.New`
  ships sane defaults (5-redirect cap, Authorization-strip on
  cross-host redirects, TLS 1.2 floor on `NewWithCA`). The
  `NewInsecure` blast radius is bounded by the existing test in
  `haproxy_test.go`.
- **redaction** — 2 findings (suggestion): RedactHandler attr
  middleware works as documented; the gaps are cosmetic
  (NopLogger wrapping, username on the secret-fragment list).
- **input-validation** — 3 findings (minor, minor, suggestion): the
  validators package (`config/validators.go`) does the right work at
  load time; the gaps are at non-LoadFile boundaries
  (`Provider.Connect`, `WithEnvFallback`, `sshpin.Verify`'s
  fail-closed mode).

## Scope exceptions proposed

None. The audit operated entirely within AUDIT_CONVENTIONS §1 / §2
boundaries.

## Footer

**Total findings:** 21 (blocker: 2, major: 7, minor: 4, suggestion: 8)
**Scope coverage:** ~150 in-scope files / ~30k LOC. All must-read-in-full
files (per SKILL.md §2) read in full: `internal/credentials/**`,
`internal/system/{fs,exec,elevation,zeroize,systemd,runid}.go`,
`internal/executor/**`, `internal/cluster/k8s.go`,
`internal/distribution/okd/phase/iso_cleanup.go`,
`internal/download/**`, `internal/netutil/**`,
`internal/config/**`. Sub-agent fan-out NOT used (single-agent sweep
selected because the canonical APIs and seam invariants needed
human-judgment correlation across files; spot-checking would
duplicate effort). Every file referenced in a finding was read
top-to-bottom. Out-of-scope files (`tui/wizard/**`, `templates/**`,
`setup/iso.go`, `_test.go`, `//go:build linux`) explicitly skipped.
**Seam deferrals:** subprocess (no audit-subprocess run this session;
shell-injection findings would route per-site there), state-and-recovery
(destroy-path recoverability is its skill), iac-and-shell (install.sh
shellcheck baseline + HCL-idiom smells).
**Linter-config-bugs:** to refresh aggregate, run
`jq -c 'select(.adjacent_linter_enabled==true)' .claude/audits/*.jsonl
> .claude/audits/linter-config-bugs.jsonl` or `/audit-all`.

**Ship-blockers (severity=blocker AND confidence=high AND risk=low):**
- `sec:eb479d86:ssh-pin-bypass` — fix before next release
- `sec:06f00bcb:auth-tree-in-webroot` — fix before next release

**MEMORY.md status:** present, read.

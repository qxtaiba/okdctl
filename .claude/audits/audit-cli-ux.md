# audit-cli-ux — 2026-05-08

**Assumes green:** golangci-lint, govulncheck, CodeQL, shellcheck, tflint, go test ./...
**Scope:** `internal/cli/**/*.go`, `cmd/okdctl/main.go`, `cmd/okdctl-gen-docs/main.go`, `README.md`, `docs/cli/**`. Read every file in full.
**Out of scope this run:** wizard internals (`internal/tui/wizard/**`), addon/distribution domain code (covered by other audits).
**Seam co-owners:** audit-errors (typed-err → exit-code mapping per seam #4), audit-documentation (per-site README/CLI-doc drift per seam #13), audit-observability (log-sink choice per seam #3).

## Executive summary

Cobra surface is small (16 commands, two-level deepest), well-organized, and the exit-code taxonomy is published in `docs/cli/exit-codes.md` with a verified anchor in `internal/cli/root.go`. Two related-but-separate findings fail the published taxonomy: `addonInstallCmd.Args` and the `validateChannel`/`validateFormat` helpers all return raw `fmt.Errorf` for usage-class violations, so rejected positional args and rejected `--output`/`--channel` values surface as exit 1 instead of 64 (EX_USAGE). The kubeconfig command still reaches for `fmt.Fprintf(os.Stderr, ...)` instead of the structured `tui.Info` sink — single-site exception in an otherwise-disciplined RedactHandler regime. Help-text cleanups (`configCmd` and `describeCmd` lacking `Long`, `addonInstallCmd.Use` carrying flag-syntax inside the cobra `Use` field) are minor but keep the generated docs (`docs/cli/`) honest. JSON contract for `okdctl status --output=json` is incomplete — six emitted fields including the always-present `phase` are missing from the published schema.

## Ranked table

Sort: severity_weight × confidence × |LOC delta| ÷ risk (blocker=4, major=3, minor=2, suggestion=1; high=3 / med=2 / low=1).

| ID | Cluster | File:line | Severity | Confidence | LOC | Adjacent linter | Fix class |
|---|---|---|---|---|---|---|---|
| ux:e7db1220:releases-validate-flag-error-not-usageerror | exit-codes | internal/cli/releases.go:157-173 | major | high | 4 | none | refactor |
| ux:fd2125dd:args-validator-not-usageerror | exit-codes | internal/cli/addon.go:62-73 | major | high | 4 | none | refactor |
| ux:024a2c32:json-schema-status-incomplete | json-stability | docs/cli/json-schema.md:12-40 | minor | high | 12 | none | refactor |
| ux:4583b75b:config-describe-missing-long | help-text | internal/cli/config.go:19-22 | minor | high | 4 | none | refactor |
| ux:daf5bee9:kubeconfig-status-msg-bypasses-tui | streams | internal/cli/kubeconfig.go:72 | minor | high | 0 | none | refactor |
| ux:8d8faa80:completion-bypasses-outorstdout | streams | internal/cli/completion.go:36-47 | minor | high | 0 | none | refactor |
| ux:fd2125dd:install-use-bracket-syntax | verb-noun | internal/cli/addon.go:44 | minor | high | 0 | none | refactor |
| ux:073d24ed:deploy-example-uses-config-not-output-file | flag-conventions | internal/cli/deploy.go:38-40 | minor | medium | 0 | none | refactor |
| ux:0d318f5c:log-format-tty-default-help-noise | streams | internal/cli/logging.go:73-78 | suggestion | high | 0 | none | refactor |
| ux:08ec0042:flag-output-name-collision-risk | flag-conventions | internal/cli/flags.go:7-10 | suggestion | high | 1 | none | refactor |
| ux:aa84670c:version-cmd-uses-run-not-rune | verb-noun | internal/cli/root.go:236-243 | suggestion | high | 1 | errcheck | refactor |
| ux:08c49fc4:keep-haproxy-no-shorthand-asymmetry | flag-conventions | internal/cli/update_ingress.go:45-47 | suggestion | high | 0 | none | policy |
| ux:8154ab0f:doctor-pull-secret-config-skew-warns | exit-codes | internal/cli/doctor.go:428-476 | suggestion | medium | 0 | none | policy |

## Exit code taxonomy (current state, verified)

| Code | Source                     | Trigger                                         | Cited in                                   |
|------|----------------------------|-------------------------------------------------|--------------------------------------------|
| 0    | success                    | `RunE` returns nil                              | root.go:131                                |
| 1    | generic                    | unclassified errors, fmt.Errorf, addon-Args bug | root.go:230 (default arm of `exitCodeFor`) |
| 2    | ConfigError                | `errors.As(err, *errtypes.ConfigError)`         | root.go:208-211                            |
| 3    | NetworkError               | `errors.As(err, *errtypes.NetworkError)`        | root.go:212-214                            |
| 4    | ClusterError               | `errors.As(err, *errtypes.ClusterError)`        | root.go:216-218                            |
| 5    | AuthError                  | `errors.As(err, *errtypes.AuthError)`           | root.go:220-222                            |
| 64   | EX_USAGE                   | flag-parse via SetFlagErrorFunc → UsageError    | root.go:224-228, root.go:256-259           |
| 65   | EX_DATAERR                 | `errors.Is(err, errtypes.ErrPullSecretInvalid)` | root.go:202-204                            |
| 66   | EX_NOINPUT                 | `errors.Is(err, errtypes.ErrConfigMissing)`     | root.go:199-201                            |
| 71   | EX_OSERR                   | `errors.Is(err, errtypes.ErrSudoMissing)`       | root.go:205-207                            |
| 130  | SIGINT                     | `signalExitCode` resolves to 130                | root.go:179-190                            |
| 143  | SIGTERM                    | `signalExitCode` resolves to 143                | root.go:179-190                            |

The taxonomy is published in `docs/cli/exit-codes.md`. Two findings (`ux:fd2125dd:args-validator-not-usageerror`, `ux:e7db1220:releases-validate-flag-error-not-usageerror`) close holes between the implementation and the published table.

## Command tree (verb/noun layout)

```
okdctl
├── deploy                         (root-required; --output-file convention)
├── destroy                        (root-required)
├── cleanup                        (root-required)
├── update-ingress                 (root-required)
├── doctor                         (linux-only; --output=json schema published)
├── version                        (Run, not RunE — finding)
├── completion <bash|zsh|fish>     (writes os.Stdout direct — finding)
├── kubeconfig                     (--output-file; mid-success uses fmt.Fprintf — finding)
├── debug-bundle                   (--output-file; complete)
├── status                         (--output=json schema partial — finding)
├── config (parent, no Long — finding)
│   ├── show
│   └── validate
├── describe (parent, no Long — finding)
│   ├── node    <name>
│   └── addon   <name>
├── addon (parent)
│   ├── list                       (--output=json)
│   ├── install [name | --all]     (Use field non-cobra-shape — finding)
│   ├── uninstall <name>
│   └── verify                     (--output=json)
└── releases (parent)
    ├── list                       (--output=json)
    └── show <version>             (--output=json)
```

## JSON schema presence — one boolean per command

| Command                 | --json | Schema documented |
|-------------------------|--------|--------------------|
| status                  | yes    | partial (missing `phase`, `version`, `api_server_url`, `console_url`, `conditions`, `message` — finding ux:024a2c32) |
| addon list              | yes    | yes               |
| addon verify            | yes    | yes               |
| describe node           | yes    | yes               |
| describe addon          | yes    | yes               |
| releases list           | yes    | yes               |
| releases show           | yes    | yes               |
| doctor                  | yes    | yes               |
| deploy                  | no     | n/a               |
| destroy                 | no     | n/a               |
| cleanup                 | no     | n/a               |
| update-ingress          | no     | n/a               |
| kubeconfig              | no     | n/a               |
| debug-bundle            | no     | n/a               |
| version                 | no     | n/a               |
| completion              | no     | n/a               |
| config show / validate  | no     | n/a               |

## Findings

### ux:e7db1220:releases-validate-flag-error-not-usageerror

**ID:** ux:e7db1220:releases-validate-flag-error-not-usageerror
**Cluster:** exit-codes
**File + line range:** internal/cli/releases.go:157-173 (also: internal/cli/addon.go:62-73, internal/cli/releases.go:166-173)
**Current LOC touched:** 17
**Smell:** `validateChannel` and `validateFormat` both return plain `fmt.Errorf` on rejected `--channel` / `--output` values, so the error reaches `exitCodeFor` → 1 instead of 64 (EX_USAGE). Pattern-wide: eight subcommands (releases list/show, status, doctor, addon list/verify, describe node/addon) call `validateFormat`, plus `releases list` calls `validateChannel`.
**Evidence:**
```go
func validateChannel(ch string) error {
    switch ch { case channelStable, channelAll: return nil
    default: return fmt.Errorf("invalid --channel %q (want stable|all)", ch)
    }
}
func validateFormat(format string) error { ... return fmt.Errorf("invalid --output %q (want text|json)", format) }
```
**Fix — preferred:** refactor — wrap each return in `&errtypes.UsageError{Msg: ..., Err: ...}`.
**Rule source:** `repo-counter-example: internal/cli/root.go:L256-L259` (SetFlagErrorFunc → UsageError); `docs/cli/exit-codes.md L14` (64 EX_USAGE); BSD sysexits.h.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-errors (errors owns the typed-err → code mapping)
**What MUST stay bit-for-bit:** the published 64=EX_USAGE contract in docs/cli/exit-codes.md; the SetFlagErrorFunc wrap pattern at root.go:256-259.
**Estimated net LOC delta:** +4
**Severity:** major
**Severity reason:** published-taxonomy violation pattern-wide.
**Risk (of applying fix):** low — single function, single test.
**Confidence (in finding):** high — verified by reading exitCodeFor.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** ux:fd2125dd:args-validator-not-usageerror

### ux:fd2125dd:args-validator-not-usageerror

**ID:** ux:fd2125dd:args-validator-not-usageerror
**Cluster:** exit-codes
**File + line range:** internal/cli/addon.go:62-73
**Current LOC touched:** 12
**Smell:** `addonInstallCmd.Args` returns plain `fmt.Errorf` for missing-or-conflicting positional args. Reaches `exitCodeFor` → 1 instead of 64 (EX_USAGE).
**Evidence:**
```go
Args: func(_ *cobra.Command, args []string) error {
    if addonInstallAll {
        if len(args) != 0 { return fmt.Errorf("--all and a named addon are mutually exclusive") }
        return nil
    }
    if len(args) != 1 { return fmt.Errorf("expected exactly one addon name, or use --all") }
    return nil
},
```
**Fix — preferred:** refactor — wrap each return in `&errtypes.UsageError{Msg: ..., Err: ...}`.
**Rule source:** `repo-counter-example: internal/cli/root.go:L256-L259`; `docs/cli/exit-codes.md L14`; BSD sysexits.h.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-errors
**What MUST stay bit-for-bit:** `docs/cli/exit-codes.md` 64=EX_USAGE contract.
**Estimated net LOC delta:** +4
**Severity:** major
**Severity reason:** published-taxonomy violation; user scripts branching on `rc==64` silently miss arg-shape errors.
**Risk:** low — pure error-type wrap.
**Confidence:** high.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** ux:e7db1220:releases-validate-flag-error-not-usageerror

### ux:024a2c32:json-schema-status-incomplete

**ID:** ux:024a2c32:json-schema-status-incomplete
**Cluster:** json-stability
**File + line range:** docs/cli/json-schema.md:12-40
**Current LOC touched:** 29
**Smell:** Documented schema for `okdctl status --output=json` lists four fields. Emitted Go type `okd.ClusterStatus` carries six more (`phase`, `version`, `api_server_url`, `console_url`, `conditions`, `message`); `phase` is non-omitempty and always present.
**Evidence:**
```go
// internal/distribution/okd/types.go
type ClusterStatus struct {
    Phase             ClusterPhase  `json:"phase"`
    APIReachable      bool          `json:"api_reachable"`
    Version           string        `json:"version,omitempty"`
    APIServerURL      string        `json:"api_server_url,omitempty"`
    ConsoleURL        string        `json:"console_url,omitempty"`
    Nodes             []NodeStatus  `json:"nodes"`
    DegradedOperators int           `json:"degraded_operators"`
    Conditions        []Condition   `json:"conditions,omitempty"`
    Addons            []AddonStatus `json:"addons,omitempty"`
    Message           string        `json:"message,omitempty"`
}
```
**Fix — preferred:** refactor — extend the markdown table to cover all ten fields and tag the omitempty subset.
**Rule source:** `docs/cli/json-schema.md L9-L11` (stability statement); `internal/distribution/okd/types.go: ClusterStatus`; seams.md §13.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-documentation (per-site)
**What MUST stay bit-for-bit:** existing field names and types — the doc already promises stability across patch and minor releases.
**Estimated net LOC delta:** +12 docs lines
**Severity:** minor
**Risk:** low — doc-only change.
**Confidence:** high.
**CLAUDE.md / MEMORY.md conflict?:** no
**Related findings:** none

### ux:4583b75b:config-describe-missing-long

**ID:** ux:4583b75b:config-describe-missing-long
**Cluster:** help-text
**File + line range:** internal/cli/config.go:19-22 (also: internal/cli/status.go:38-41)
**Current LOC touched:** 4
**Smell:** `configCmd` and `describeCmd` are parent verbs with subcommands but register only `Short`. Sibling parents `addonCmd` and `releasesCmd` carry a `Long` that orients first-time users.
**Evidence:**
```go
// internal/cli/config.go
var configCmd = &cobra.Command{
    Use:   cfgVerb,
    Short: "Inspect okdctl configuration",
}
// internal/cli/status.go
var describeCmd = &cobra.Command{
    Use:   "describe",
    Short: "Drill into a specific node or addon",
}
```
**Fix — preferred:** refactor — add a one-sentence `Long` to both. Regenerate `docs/cli/` via `make docs`.
**Rule source:** `repo-counter-example: internal/cli/addon.go:L26-L30`; `repo-counter-example: internal/cli/releases.go:L33-L37`; CLAUDE.md §code-comments.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**Estimated net LOC delta:** +4
**Severity:** minor
**Risk:** low.
**Confidence:** high.
**CLAUDE.md conflict?:** no — doc does not echo signature; describes the parent group's purpose.

### ux:daf5bee9:kubeconfig-status-msg-bypasses-tui

**ID:** ux:daf5bee9:kubeconfig-status-msg-bypasses-tui
**Cluster:** streams
**File + line range:** internal/cli/kubeconfig.go:72 (and L123 for the merge path)
**Current LOC touched:** 1
**Smell:** Post-success status line uses `fmt.Fprintf(os.Stderr, ...)` instead of `tui.Info(...)`. Stream choice (stderr) is correct; the slog/RedactHandler chain is bypassed so future structured fields silently lose redaction.
**Evidence:**
```go
if err := system.AtomicWrite(kubeconfigOutput, data, 0o600); err != nil { ... }
fmt.Fprintf(os.Stderr, "kubeconfig written to %s\n", kubeconfigOutput)
```
**Fix — preferred:** refactor — `tui.Info("kubeconfig written", tui.LF("path", kubeconfigOutput))` and the same shape for the merge path at L123.
**Rule source:** `CLAUDE.md §credentials-and-secrets` (RedactHandler chain); `repo-counter-example: internal/cli/destroy.go:L184`; `repo-counter-example: internal/cli/cleanup.go:L131`.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-observability
**Estimated net LOC delta:** 0
**Severity:** minor
**Risk:** low — `tui.Info` exists and is the canonical sink.
**Confidence:** high.
**CLAUDE.md conflict?:** no — the rule explicitly says use structured attrs.

### ux:8d8faa80:completion-bypasses-outorstdout

**ID:** ux:8d8faa80:completion-bypasses-outorstdout
**Cluster:** streams
**File + line range:** internal/cli/completion.go:36-47
**Current LOC touched:** 12
**Smell:** Completion writes generated shells directly to `os.Stdout` rather than `cmd.OutOrStdout()`. Sibling commands consistently use `cmd.OutOrStdout()`. Tests must redirect at the process level instead of binding the cobra writer.
**Evidence:**
```go
case "bash":
    return cmd.Root().GenBashCompletionV2(os.Stdout, true)
case "zsh":
    return cmd.Root().GenZshCompletion(os.Stdout)
case "fish":
    return cmd.Root().GenFishCompletion(os.Stdout, true)
```
**Fix — preferred:** refactor — pass `cmd.OutOrStdout()`.
**Rule source:** repo-counter-example: `internal/cli/status.go:L217`; `internal/cli/releases.go:L106`; cobra `Command.OutOrStdout` docs.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**Estimated net LOC delta:** 0
**Severity:** minor
**Risk:** low.
**Confidence:** high.

### ux:fd2125dd:install-use-bracket-syntax

**ID:** ux:fd2125dd:install-use-bracket-syntax
**Cluster:** verb-noun
**File + line range:** internal/cli/addon.go:44
**Current LOC touched:** 1
**Smell:** `addonInstallCmd.Use = "install [name | --all]"` puts a flag inside the cobra `Use` field. Cobra renders flags through Options; the pipe-syntax reads as if `--all` is positional. Sibling commands use clean positional shapes (`Use: "show <version>"`, `Use: "node <name>"`).
**Evidence:**
```go
var addonInstallCmd = &cobra.Command{
    Use:         "install [name | --all]",
    Short:       "Install one addon (or all enabled addons with --all)",
    Example:     "  okdctl addon install flux\n  okdctl addon install --all",
    ...
}
```
**Fix — preferred:** refactor — `Use: "install [name]"`. Long and Example already explain the `--all` shape (L51-L61, L46-L47).
**Rule source:** repo-counter-example: `internal/cli/releases.go:L53`; `internal/cli/status.go:L44`.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**Estimated net LOC delta:** 0
**Severity:** minor
**Risk:** low — Use string is cosmetic; behaviour unchanged.
**Confidence:** high.

### ux:073d24ed:deploy-example-uses-config-not-output-file

**ID:** ux:073d24ed:deploy-example-uses-config-not-output-file
**Cluster:** flag-conventions
**File + line range:** internal/cli/deploy.go:38-40
**Current LOC touched:** 3
**Smell:** Deploy example shows `okdctl deploy --yes --config my-cluster.yaml`. `--config` is the read-side persistent flag (root); `--output-file` is the deploy-specific write-side flag. Under `--yes` the wizard never runs and write-side never fires, so the example happens to work — but it conflates the two halves of the convention codified in CLAUDE.md §architecture.
**Evidence:**
```go
Example: `  okdctl deploy
  okdctl deploy --yes --config my-cluster.yaml
  okdctl deploy --dry-run`,
```
**Fix — preferred:** refactor — drop `--config` from the second example, or split into one read example and one write example (`okdctl deploy --yes --output-file my-cluster.yaml`). Regenerate `docs/cli/` via `make docs`.
**Rule source:** CLAUDE.md §architecture (flag-naming convention block); repo-counter-example: `internal/cli/deploy.go:L45`.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-documentation
**Estimated net LOC delta:** 0
**Severity:** minor
**Risk:** low.
**Confidence:** medium — example works as written; this is naming-clarity not behaviour.

### ux:0d318f5c:log-format-tty-default-help-noise

**ID:** ux:0d318f5c:log-format-tty-default-help-noise
**Cluster:** streams
**File + line range:** internal/cli/logging.go:73-78
**Current LOC touched:** 6
**Smell:** Auto-switch to `--log-format=json` when stderr is piped is correct (12-factor / clig.dev) but cobra still renders `(default "text")` in `--help`. The mitigating prose at `root.go:L248` discloses the auto-switch in parentheses, so the contract is documented; the cobra-default paren just contradicts it.
**Evidence:**
```go
// internal/cli/logging.go:L73-L75
if !cmd.Root().PersistentFlags().Changed("log-format") && !stderrIsTTY {
    logFormat = outputJSON
}
// internal/cli/root.go:L248
rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text",
    "log output format (text, json); defaults to json when stderr is not a TTY ...")
```
**Fix — preferred:** refactor or accept — either suppress the cobra-rendered default (`Flag.DefValue = ""`) or accept the explanatory paren above as sufficient. No security impact.
**Rule source:** clig.dev §6 (be honest about side-effects); 12-factor CLI §III.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**Estimated net LOC delta:** 0
**Severity:** suggestion
**Risk:** low.
**Confidence:** high.

### ux:08ec0042:flag-output-name-collision-risk

**ID:** ux:08ec0042:flag-output-name-collision-risk
**Cluster:** flag-conventions
**File + line range:** internal/cli/flags.go:7-10
**Current LOC touched:** 4
**Smell:** Constant `flagOutput = "output-file"` collides with the `--output` flag spelling used by eight other commands. A maintainer reading `flagOutput` in a deploy/debug-bundle registration silently mis-reads it as `--output`.
**Evidence:**
```go
const (
    flagDryRun = "dry-run"
    flagOutput = "output-file"
)
// vs status.go:L68: StringVarP(..., "output", "o", outputText, ...)
```
**Fix — preferred:** refactor — rename to `flagOutputFile` (and optionally codify `flagOutputFormat = "output"`).
**Rule source:** CLAUDE.md §architecture (flag-naming convention block); Go proverb (clear is better than clever).
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**Estimated net LOC delta:** +1 (or 0 — ident-rename only)
**Severity:** suggestion
**Risk:** low.
**Confidence:** high.

### ux:aa84670c:version-cmd-uses-run-not-rune

**ID:** ux:aa84670c:version-cmd-uses-run-not-rune
**Cluster:** verb-noun
**File + line range:** internal/cli/root.go:236-243
**Current LOC touched:** 8
**Smell:** `versionCmd` uses `Run` rather than `RunE`. Every other leaf command in the tree uses `RunE`. `fmt.Fprintf` errors are silently dropped — `okdctl version | head -c 0` exits 0 instead of emitting an EPIPE-shaped error.
**Evidence:**
```go
var versionCmd = &cobra.Command{
    Use:   "version",
    Short: "Print version, git commit, build date",
    Run: func(cmd *cobra.Command, _ []string) {
        fmt.Fprintf(cmd.OutOrStdout(), "okdctl %s\n...", ...)
    },
}
```
**Fix — preferred:** refactor — convert `Run` to `RunE`, return the `fmt.Fprintf` error.
**Rule source:** repo-counter-example: every other leaf command; cobra docs.
**Adjacent linter:** errcheck (already enabled; would have flagged the dropped error).
**Scaffolding?:** no
**Seam:** none
**Estimated net LOC delta:** +1
**Severity:** suggestion
**Risk:** low.
**Confidence:** high.

### ux:08c49fc4:keep-haproxy-no-shorthand-asymmetry

**ID:** ux:08c49fc4:keep-haproxy-no-shorthand-asymmetry
**Cluster:** flag-conventions
**File + line range:** internal/cli/update_ingress.go:45-47
**Current LOC touched:** 3
**Smell:** Boolean tail flags (`--keep-haproxy`, `--keep-isos`, `--skip-must-gather`, `--skip-terraform`, `--skip-cleanup`, `--skip-firewall`) consistently lack a single-letter shorthand. The existing convention is sound (only the universal trio gets shorthands); flag is just an observation that the convention is not yet codified.
**Evidence:**
```go
updateIngressCmd.Flags().BoolVarP(&updateIngressYes, "yes", "y", false, ...)
updateIngressCmd.Flags().BoolVar(&updateIngressKeepHAProxy, "keep-haproxy", false, ...)
```
**Fix — preferred:** policy — add a one-line note in CLAUDE.md §architecture pinning the shorthand allowlist (`--yes/-y`, `--quiet/-q`, `--verbose/-v`, `--output/-o`, `--config/-c`).
**Rule source:** repo-counter-example: every Bool flag in destroy.go and cleanup.go.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** none
**Estimated net LOC delta:** 0 code, +3 doc
**Severity:** suggestion
**Risk:** low.
**Confidence:** high.

### ux:8154ab0f:doctor-pull-secret-config-skew-warns

**ID:** ux:8154ab0f:doctor-pull-secret-config-skew-warns
**Cluster:** exit-codes
**File + line range:** internal/cli/doctor.go:428-476
**Current LOC touched:** 49
**Smell:** `checkPullSecret` returns `sevWarn` when no config exists (first-time user) but `sevFail` when the config exists with empty pull-secret. doctor's contract is "exit 0 unless [fail]". A first-time user sees mostly green plus a warn for pull-secret, runs deploy, and gets rejected with exit 65 (EX_DATAERR) on the same field. Taxonomy holds; doctor's preflight value drops.
**Evidence:**
```go
if _, err := os.Stat(configPath); err != nil {
    if os.IsNotExist(err) {
        return checkResult{
            sev:    sevWarn,
            detail: "no config yet at " + configPath + "; run 'okdctl deploy' to set the pull secret path in the wizard",
        }
    }
}
```
**Fix — preferred:** policy — keep current shape (warn-then-fail) and document doctor as orientation-only in `docs/doctor-checks.md`. Promoting to `sevFail` would break `okdctl doctor && okdctl deploy`-style chains.
**Rule source:** docs/cli/exit-codes.md L19-L23; internal/cli/doctor.go:L181-L188.
**Adjacent linter:** none
**Scaffolding?:** no
**Seam:** audit-state-and-recovery
**Estimated net LOC delta:** 0 code, ~5 doc
**Severity:** suggestion
**Risk:** medium — naive escalation breaks chained scripts.
**Confidence:** medium.

## Scaffolding items detected

None this run — every flagged symbol has a current call site. The unused-export-as-future-API exemption in MEMORY.md §scaffolding does not fire.

## Linter-config-bug candidates

`ux:aa84670c:version-cmd-uses-run-not-rune` lists `errcheck` as adjacent. `errcheck` is enabled in `.golangci.yml`; it should have flagged the dropped `fmt.Fprintf` error, but the `Run` signature has no return value so the dropped error is structurally invisible. Not a `.golangci.yml` config bug — it is a cobra-API-shape bug. Excluded from the linter-config aggregate.

## Skip list

- **Root command `Long:` marketing language** ("delightful CLI tool", "beautiful TUI"). MEMORY.md `feedback_lowercase_logs` notes cobra help text is out of scope. Skip.
- **`okdctl deploy` Example colour bullets** (root `Long:` uses `•`). User-facing TUI text; not a slog message. Skip.
- **`--color`/`--no-color` flag absence**. MEMORY.md `feedback_color_flag` records the maintainer rejected this as "stupid"; the existing `NO_COLOR` env honour at `logging.go:69` is sufficient. Skip.
- **`os.Exit(130)` on second SIGINT bypassing `logFileCloser.Close()`** at `root.go:151`. Justified in a comment block ("the user has explicitly asked for a hard kill"). Skip.

## Cluster verdicts

- **verb-noun** — clean. Verbs are imperative, nouns are singular except `releases` (collection-noun, kubectl-style). One stylistic finding (`addon install` Use field) and one error-handling finding (`version` uses Run, not RunE).
- **flag-conventions** — kebab-case throughout, no `--no-X` antipattern. Three findings, all minor: a constant naming risk, a documented but slightly mismatched example, and a pure-policy nit about shorthand allowlist.
- **exit-codes** — taxonomy is published and verified, but the implementation has two pattern-wide gaps where `fmt.Errorf` for usage-class violations bypasses the typed-error-to-code mapping. Both fixes are 4-LOC wraps. Doctor's pull-secret skew is a separate UX-vs-script tension worth deciding on.
- **help-text** — every leaf command has Short. Two parents (`config`, `describe`) lack Long. Examples present everywhere except parent groups (acceptable) and `version` (acceptable convention).
- **signals** — solid. Signal-loop, two-strike, and `cmd.Cancel`/`cmd.WaitDelay` plumbing are documented in CLAUDE.md §concurrency and verified against the code.
- **streams** — disciplined. tui sink for logs, cmd.OutOrStdout for data. Two single-site exceptions (kubeconfig, completion). The auto-switch to JSON when stderr is piped is the right shape but creates a help-text honesty nit.
- **json-stability** — schemas are documented, snake_case is consistent, no secret leakage observed (config.go redactConfig walks all string fields). One incomplete-schema finding (`status`).

## Scope exceptions proposed

None. Sweep covered every file in `internal/cli/**/*.go`, both files in `cmd/**/*.go`, README, and every file under `docs/cli/`.

## Footer

Total findings: 13 (blocker: 0, major: 2, minor: 6, suggestion: 5)
Scope coverage: 23 / 23 files read in full (100%; no sub-agent dispatch needed — surface fits in one pass).
Seam deferrals: 4 — ux:fd2125dd:args-validator-not-usageerror → audit-errors; ux:e7db1220:releases-validate-flag-error-not-usageerror → audit-errors; ux:024a2c32:json-schema-status-incomplete → audit-documentation; ux:073d24ed:deploy-example-uses-config-not-output-file → audit-documentation; ux:daf5bee9:kubeconfig-status-msg-bypasses-tui → audit-observability; ux:8154ab0f:doctor-pull-secret-config-skew-warns → audit-state-and-recovery.
JSONL validation: 13/13 rows pass `finding-schema.json` (required fields, ID pattern, severity_reason on major findings, additionalProperties=false).
Schema-noted MEMORY.md absences: none — local MEMORY.md was read.
To refresh `linter-config-bugs.jsonl`, run the aggregation command or `/audit-all`.

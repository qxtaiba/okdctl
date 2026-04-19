# okdctl roadmap

Captured 2026-04-18 from a full-repo audit (12 parallel Explore subagents) and a
follow-up triage interview. Items are agent-actionable: each includes concrete
file references, acceptance criteria, and dependencies.

## How to use this file

- Items are grouped by schedule band (Sprint 1 / Addon-refactor workstream /
  Deferred / Skipped), then by theme inside each band.
- Every item has a stable ID (`U1`, `N5`, `M12`, `L5`) so PRs, commits, and
  issues can cross-reference it.
- "Acceptance" states what makes the item done — use it as a PR checklist.
- "Depends on" lists prerequisite items by ID.
- Effort estimates follow audit scale: `hours` / `days` / `weeks`.
- Evidence refs use `path:line` format pointing at the repo state on
  2026-04-18 (post–elevation-refactor, post-yaml-migration, post-netip).

### Status lifecycle

Every actionable item carries a `**Status:**` field. Sessions updating an
item **must** edit this field so other sessions know the item is claimed.

| Value | Meaning |
|---|---|
| `not started` | Free to pick up. |
| `in progress — branch: <name>` | A session is actively working on it. |
| `in progress — worktree: <path>` | A parallel agent owns an isolated worktree. |
| `in review — PR #<n>` | Implementation done, awaiting merge. |
| `done — PR #<n>` | Merged. After merge, move the item entry under a `## Completed` section at the bottom of this file and record the merge date. |
| `blocked — waiting on <ID>` | Dependency not yet in `done`; skip for now. |
| `deferred` | Scheduled for a later quarter; do not pick up. |

To find eligible work: grep for `**Status:** not started`, then filter by
`Depends on:` being `none` or only IDs already in `done`.

## Product philosophy (captured in triage)

These are durable constraints the roadmap assumes. They supersede prior
ambitions found in the code.

1. **Single provider: Proxmox.** Multi-provider support (libvirt / vSphere /
   AWS / Equinix / bare-metal / Vagrant) and multi-distribution (RKE2,
   vanilla k8s) are explicitly **skipped**. No provider interface will be
   extracted. The `ProviderType` enum stays single-variant; `ProviderConfig`
   stays a Proxmox-shaped struct.
2. **Linux-only deploy.** macOS/Windows builds are by design removed; wizard
   is cross-platform but deploy targets Linux. `okdctl doctor` stays
   Linux-gated. Documented further, not relaxed.
3. **Curated + alternatives addon catalog with per-category wizard
   dropdowns.** The catalog will grow via a *category model*: each addon
   slots into a category (ingress, load-balancer, GitOps, certs, monitoring,
   storage, backup, policy, service-mesh) and the wizard renders a
   per-category dropdown. Every category includes a "None / BYO" option.
   The specific addons that populate each category are a **later
   conversation** — this roadmap only covers the refactor + scaffolding.
4. **GitOps is not default.** Flux is not auto-installed. Users explicitly
   opt into either Flux or ArgoCD per cluster. Existing Flux addon stays
   available as a category option once the refactor lands.
5. **Tests are not priority.** Zero tests exist today. Unit tests for
   `netutil` and `config/validators` and any integration harness are
   explicitly deferred to quarter+ despite high ROI; the CI test-go gate
   passes vacuously for now.

## Sprint 1 — active work

**Scope warning:** Sprint 1 currently contains ~40 items selected during
triage. This is realistically 3–6 months of work at one engineer. Whoever
picks this up should further sequence using the themes below. Recommended
intra-sprint order: Theme A (urgent correctness) → Theme B (CLI surface) →
Theme D (wizard) → Theme F (errors/types) → remaining themes in parallel.

### Theme A — urgent correctness bugs

#### U2 — Wizard never sets `Provider.Type`
- **Status:** in review — PR #52
- **Category:** urgent-bugfix
- **State:** not started
- **Effort:** days
- **Impact:** large
- **Evidence:** `internal/config/validators.go` requires `cfg.Provider.Type`;
  `internal/tui/wizard/steps/proxmox.go` reads it via `ShouldShow` but no
  step writes it. Wizard-only users produce YAML that fails validation.
- **Acceptance:** wizard completing produces a YAML where
  `cfg.Provider.Type == config.ProviderProxmox` is set; validation passes
  end-to-end without manual YAML edit.
- **Depends on:** none.

#### U1b — Clean remote Proxmox FCOS ISO on destroy
- **Status:** in review — PR #54
- **Category:** urgent-bugfix
- **State:** not started
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/distribution/okd/setup/coreos.go:19` defines
  `DefaultProxmoxISODir = "/var/lib/vz/template/iso"`. ISOs are uploaded
  via SSH at `internal/distribution/okd/setup/upload.go:86` but never
  removed during `destroy` or `cleanup`. Each deploy leaves
  500 MB–2 GB on the Proxmox host. Supersedes the closed U1 (the
  audit conflated the local `downloads/` cache — which is already
  cleaned by `internal/distribution/okd/cleanup/artifacts.go:84,92` —
  with the remote Proxmox ISO dir, which is the real gap).
- **Acceptance:** destroy (and `okdctl cleanup` once N4 lands) removes
  `fedora-coreos-*.iso` from `DefaultProxmoxISODir` on the Proxmox host,
  reusing the SSH plumbing from `upload.go`. Before removal: check
  `pvesh get /nodes/<node>/qemu` (or the equivalent
  `luthermonson/go-proxmox` call) to confirm no running VM references
  the ISO; skip with a warning if one does. New `--keep-isos` flag on
  `destroy` preserves the ISO for users chaining deploys. Honours
  `refuseCriticalPath` semantics — the removal path must pass through
  `SafeRemoveWithLogger` or an SSH-equivalent safety check before
  the rm runs.
- **Depends on:** none. (N4 would let `okdctl cleanup` invoke the same
  logic standalone; not a blocker.)

### Theme B — CLI subcommand expansion

Motto for this theme: "finish the last mile of the manager/fetcher
scaffolding the internal code already holds."

#### M1 — `okdctl status` / `describe`
- **Status:** in review — PR #84
- **Category:** feature-gap
- **State:** not started
- **Effort:** days
- **Impact:** large
- **Acceptance:** `okdctl status` prints a post-deploy summary: API
  reachability, node count by role, operator health, addon status.
  `okdctl describe` drills into a specific node / addon. Uses existing
  `OcPollOutput` helpers in `phase/kubectl.go`.
- **Depends on:** N8 (step-timing summary shape to reuse), N5
  (kubeconfig access pattern).

#### M2 — `okdctl debug-bundle`
- **Status:** in review — PR #90
- **Category:** feature-gap
- **State:** not started
- **Effort:** week
- **Impact:** large
- **Acceptance:** `okdctl debug-bundle [--output bundle.tgz]` collects:
  redacted config, recent log file (see N9), `oc adm must-gather`
  output, terraform state summary, `okdctl doctor` results, system
  metadata. Output is a tarball safe to attach to a support ticket.
- **Depends on:** N9 (log-file flag so logs persist), M14 (correlation
  ID for cross-referencing).

### Theme C — observability, error messages, logging

#### L5 — Prometheus metrics endpoint during deploy
- **Status:** in review — PR #87
- **Category:** feature-gap
- **State:** not started
- **Effort:** week
- **Impact:** medium
- **Acceptance:** `okdctl deploy --metrics-addr :9090` starts a Prometheus
  HTTP endpoint for the duration of the run. Metrics emitted: per-step
  counter + histogram, current-step gauge, deploy total duration.
  Disabled by default.
- **Depends on:** N8 (step timing must record durations first).

### Theme D — wizard coverage

#### N26 — TUI key-value map editor component
- **Status:** not started
- **Category:** feature-gap / TUI
- **State:** design needed
- **Effort:** days
- **Impact:** small (enables future addon features)
- **Evidence:** M12 (PR #81) secretstore has `onepassword_vaults:
  "homelab=1,shared=2"` (CSV in a string) because the wizard has no
  native map-editor widget. Future addons with structured sub-maps
  (vault mount paths, custom headers, label selectors) will either
  re-invent CSV-in-string or have to edit YAML by hand.
- **Acceptance:** new TUI component (analogous to `InputField` /
  `InputGroup` / the `MultiSelectField` added for N16) renders a
  focused table of (key, value) rows with add/delete/edit navigation.
  Integrates with `FieldDefinition` via a new `FieldTypeMapEditor` or
  `FieldTypeKeyValue` type. Stores as a YAML map or as a delimited
  string (configurable per field). Retrofit `onepassword_vaults` in
  the secretstore wizard as the first consumer.
- **Depends on:** none. (Nice-to-have foundation; does not block
  M19/M20 but pairs naturally with M20.)

### Theme E — config ergonomics, air-gap, rootless

#### M3 — `--dry-run` / `--plan` mode
- **Status:** in review — PR #87
- **Category:** feature-gap
- **State:** not started
- **Effort:** days
- **Impact:** large
- **Acceptance:** `--dry-run` on deploy/destroy/update-ingress skips
  the re-exec-as-root gate, prints the terraform plan output, and lists
  every mutating step it *would* execute. No real mutations. Exit 0
  on plan success, 2 on plan failure.
- **Depends on:** U4 (exit code 2 for plan failure).

#### M6 — `DefaultBinDir` configurable (rootless support)
- **Status:** not started
- **Category:** feature-gap
- **State:** not started
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/distribution/okd/phase/paths.go:24` hardcodes
  `/usr/local/bin`.
- **Acceptance:** config field + env override for bin dir. Elevation
  logic skips re-exec when installing to a user-local dir. Doctor check
  validates the chosen dir is writable by the invoking user.
- **Depends on:** U2 (wizard should then collect this field).

### Theme F — error types, exit codes, correctness

#### M19 — Typed addon settings via per-addon decoder method
- **Status:** in review — PR #89
- **Category:** feature-gap / refactor
- **State:** design needed (Option A from the M12 design investigation)
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/addon/addon.go:49-56` defines
  `ConfigurableAddon`; `internal/config/cluster.go:89-92` holds flat
  `AddonConfig.Settings map[string]string`; M12 (PR #81) now carries
  ~13 flat keys across three providers (onepassword, vault, bitwarden)
  with provider-specific meaning. `ValidateSettings` can't distinguish
  provider context without string-prefix matching; CSV-in-string
  encoding (`onepassword_vaults: "homelab=1,shared=2"`) is a workaround
  for lack of structured types.
- **Acceptance:** optional `DecodeSettings(settings map[string]string)
  (any, error)` method added to `ConfigurableAddon`. Each addon catalog
  package defines its own settings struct(s) (e.g., `FluxSettings`,
  `SecretStoreSettings` with provider-specific sub-structs). Each
  addon's `ValidateSettings` and `Install` use the typed struct
  internally. YAML shape stays flat — no config migration required.
  Exemplar: secretstore multi-provider case validates
  `bitwarden_organization_id` required when `provider=bitwarden` at
  the struct field level, not via string matching.
- **Depends on:** none. (Follow-on refinement of M12; could land before
  or after R1.)

#### M20 — Grouped wizard fields for structured addon settings
- **Status:** in review — PR #89
- **Category:** feature-gap / UX
- **State:** design needed (Option C from the M12 design investigation)
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/addon/addon.go:73-86` `WizardField` has no
  `Group` field. `internal/tui/wizard/steps/addons.go:74-140` renders
  all addon fields flat. M12 (PR #81) secretstore has 8 wizard fields
  spanning three providers with no visual grouping — users see all of
  them regardless of which provider they picked.
- **Acceptance:** optional `Group string` field added to
  `addon.WizardField`. Wizard renderer buckets fields by group,
  renders group headers, and (optionally) hides groups that don't
  match the currently-selected provider. Secretstore wizard shows a
  "provider" dropdown, then the matching provider's field group below
  it. Addons with no `Group` annotations render in the existing flat
  path (flux, etc. unchanged). No YAML changes.
- **Depends on:** M19 (loose dependency — structured decoders make
  group selection deterministic; can ship with manual Group
  annotations in WizardFields if M19 slips).

### Theme G — CI, tooling, distribution

#### N14 — Add `go vet ./...` to CI
- **Status:** in review — PR #53
- **Category:** infra
- **State:** not started
- **Effort:** hours
- **Impact:** small
- **Evidence:** `.github/workflows/ci.yml` runs golangci-lint and
  `go test` but no explicit vet step. `Makefile:make vet` exists.
- **Acceptance:** new job or step in CI runs `go vet ./...` and fails
  the pipeline on non-zero exit. Documented in CONTRIBUTING if it
  lands.
- **Depends on:** none.

#### L14 — Coverage thresholds + codecov in CI
- **Status:** in review — PR #85
- **Category:** infra
- **State:** not started
- **Effort:** days
- **Impact:** medium
- **Evidence:** CI already generates `coverage.out`
  (`.github/workflows/ci.yml:37`) but enforces nothing. With zero tests
  the threshold would be 0% — set a floor that rises as N12/N13 land.
- **Acceptance:** codecov or equivalent configured. Per-package
  minimums documented. PR check fails on coverage regression.
- **Depends on:** N12 / N13 to land in some form first (they are
  deferred — so this ships as scaffolding with 0% floor, ready to
  tighten).

## Addon category refactor — dedicated workstream

This is **design-doc-first**. No code lands until the plan is reviewed.

### R1 — Addon category model + design doc

- **Status:** not started
- **Category:** feature-gap / half-done
- **State:** design needed
- **Effort:** weeks
- **Impact:** large
- **Scope:**
  - Design document in `docs/superpowers/plans/YYYY-MM-DD-addon-category-model.md`
    covering:
    - Category enum (ingress, load-balancer, gitops, cert, monitoring,
      storage, backup, policy, service-mesh, secrets).
    - Extension to the `Addon` interface at `internal/addon/addon.go:13-19`
      to require a `Category()` method.
    - Per-category "None / BYO" option as a first-class zero value.
    - Wizard UX: per-category dropdown with "None" / "recommended" /
      "alternative 1" / "alternative 2" / "BYO (pin Helm chart)".
    - Migration path for existing `flux` and `secretstore` addons.
    - BYO contract: what settings a user must provide to install an
      arbitrary Helm chart through the category slot.
    - CLI: `okdctl addon categories` lists categories; `okdctl addon list
      --category ingress` filters.
- **Acceptance:** design doc reviewed and approved before any code.
  Once approved, refactor lands in phases: (a) interface + `Category()`
  method, (b) wizard dropdown rewrite, (c) migrate existing addons, (d)
  unblock R2.
- **Depends on:** none. Gates R2 and all specific-addon work.

### R2 — Specific addons (deferred conversation)

- **Status:** deferred (menu open; no item scheduled)

## Air-gap workstream — design-doc-first

Full air-gap support is not a single PR; it is a cross-cutting question
about every upstream okdctl currently reaches. M4 (`OKDReleaseBaseURL`)
and M5 (`ToolVersions` for helm/sops/yq) redirect four HTTP fetches but
leave terraform, OS package managers, FCOS media, and addon Helm charts
pointing at public networks. Before shipping more redirects piecemeal we
want a scoping doc that maps the complete surface and decides which
scenarios we actually support.

### L15 — Air-gap feasibility + scoping doc

- **Status:** not started
- **Category:** feature-gap / design-needed
- **State:** scoping needed
- **Effort:** weeks (scoping); full implementation is quarters
- **Impact:** large
- **Scope:**
  - Design document in `docs/superpowers/plans/YYYY-MM-DD-airgap-scoping.md`
    covering:
    - Complete inventory of external HTTP/package fetches: OKD release
      binaries, FCOS ISO, helm/sops/yq, terraform (HashiCorp apt/dnf),
      OS packages (apt/dnf distro mirrors), addon Helm chart pulls,
      container image references baked into addon manifests, the GitHub
      update check, any runtime `go install` (e.g. yamlfmt in CI) —
      audit-grade with file:line refs.
    - For each fetch: today's source, whether it is already redirectable
      (M4/M5), and what would be required to redirect it (config field,
      env var, install-script change, addon contract change).
    - Target user scenarios: "fully air-gapped homelab with a mirror
      registry"; "intermittently-connected with a caching proxy";
      "connected but wants a private mirror for speed/compliance" — pick
      which to support, explicitly defer the rest.
    - Mirror contract: what URL shape / directory layout a mirror must
      expose to satisfy okdctl. Prefer a layout that maps 1:1 to
      upstream paths so users can `rsync` rather than rewrite.
    - Addon implications: does an air-gap mode require every addon to
      declare a mirror-friendly chart repo? If so, an extension to the
      `Addon` interface may land alongside R1.
    - Verification strategy: how do we smoke-test air-gap in CI without
      a real mirror?
- **Acceptance:** design doc reviewed and approved before any code. Once
  approved, implementation lands as a series of roadmap items (one per
  unblocked fetch), not one mega-PR. M4 and M5 are referenced as
  precedent for the env > config > default resolution pattern.
- **Depends on:** none. M4 and M5 are context but not prerequisites.



When the refactor lands, these are the candidates captured during
triage. **None of these are scheduled yet.** They are the menu for a
later decision.

- **Ingress:** ingress-nginx, Envoy Gateway. (OKD built-in router is the
  "None" option.)
- **Load-balancer:** MetalLB, kube-vip.
- **GitOps:** Flux (existing), Argo CD. Neither is a default; users
  opt in.
- **Certs:** cert-manager.
- **Monitoring:** kube-prometheus-stack, Loki, "use OKD built-in" as an
  option.
- **Storage:** Rook-Ceph.
- **Backup:** Velero. VolSync considered (user familiarity) but not
  decided.
- **Policy:** Kyverno, OPA Gatekeeper — both flagged for possible future
  support but not scoped here.
- **Service mesh:** skipped (user BYO).

## Deferred — revisit next quarter

These made the audit but are not scheduled now. Re-evaluate on
2026-07-01 or when a concrete use case surfaces.

| ID | Item | Why deferred |
|---|---|---|
| N12 | Unit tests for `internal/netutil` | User priority: not testing now |
| N13 | Unit tests for `internal/config/validators.go` | Same |
| N21 | `CONTRIBUTING.md` | Pre-1.0; CLAUDE.md covers AI agents |
| N22 | Troubleshooting / FAQ doc | README section is sufficient today |
| M15 | Integration test harness (wizard end-to-end) | Consistent with N12/N13 |
| M18 | Homebrew tap | Post-1.0 distribution item |
| L4 | `okdctl upgrade` (in-place OKD upgrade) | Strategic; needs design |
| L6 | OpenTelemetry tracing hooks | N8 step-timing covers 80% of value |

### Tier D — dependency items from 2026-04-18 audit

Filed as roadmap items so `/roadmap-pickup` can fan them out when
bandwidth opens. Each references the audit finding ID for diff tracking.

#### D2 — evaluate progressbar swap for bubbles/progress
- **Status:** in review — PR #86
- **Category:** deps (transitive-weight)
- **Effort:** days
- **Impact:** small (dep-count reduction)
- **Finding:** `dep:33ef32bf:schollz-progressbar-transitive-weight`
- **Evidence:** `go.mod:14`; single call site `internal/download/download.go:16`.
  Pulls `mitchellh/colorstring` (2019-stale) as a transitive.
- **Acceptance:** evaluate `charm.land/bubbles/v2/progress` as replacement
  (already in tree); or hand-roll ~30 LOC. TTY/SIGWINCH/pipe-detection
  paths are battle-tested — plan first, then swap.
- **Depends on:** none

## Explicitly skipped

Do not revisit these without a strong new signal.

| ID | Item | Rationale |
|---|---|---|
| L1 | libvirt / KVM provider | Proxmox-only is a product constraint |
| L2 | vSphere / AWS / Equinix / bare-metal / Vagrant | Same |
| L3 | Multi-distribution (RKE2, vanilla k8s) | Same |
| M7 | Provider interface extraction | No longer needed given L1–L3 skipped |
| L12 | Container image distribution (ghcr.io/...) | Deploy model is host-shell; container fit is poor |

Also skipped by design (documented in audit, not user-driven):

- macOS / Windows build targets (goreleaser ships linux/amd64 + linux/arm64 only).
- `okdctl doctor` on non-Linux (Linux-only build tag is intentional).
- Bootstrap as a separate wizard section (wizard intentionally mirrors
  control-plane specs via `internal/tui/wizard/steps/resources.go:56`).
- Manual addon registration via blank-import (pattern is documented in
  `docs/architecture/addons.md:50-58`; scale does not yet justify codegen).
- Terraform state file preservation during cleanup — deliberate in
  `internal/distribution/okd/cleanup/infra.go:52-61`.
- Hardcoded k8s ports (6443, 22623) — OKD spec-level; configurability
  would break installer assumptions.

## Completed

Items that have reached `done` status, ordered by close date. New
entries land here when a PR merges, or when an item is closed without
code (audit error, done-by-prior-work). Keep the explanation terse
but link evidence.

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
  FCOS still hit upstream repos — the broader air-gap story is now
  tracked as L15.
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

## Appendix — full item ledger

| ID | Item | Disposition |
|---|---|---|
| U1 | Cleanup leaves FCOS ISO cache | **Done** (audit error — see Completed) |
| U1b | Clean remote Proxmox FCOS ISO on destroy | Sprint 1 |
| U2 | Wizard never sets `Provider.Type` | Sprint 1 |
| U3 | HTTP downloads no retry | Sprint 1 |
| U4 | Exit codes only 0/1/130 | **Done** (PR #70) |
| U5 | `cmd.Run()` without ctx (2 sites) | **Done** (landed in 65d8fce — see Completed) |
| U6 | `BuildOpaqueSecret` panic | Sprint 1 |
| N1 | `okdctl addon list/install/uninstall/verify` | **Done** (PR #73) |
| N2 | `okdctl releases list/show` | Sprint 1 |
| N3 | `okdctl config validate` standalone | **Done** (PR #74) |
| N4 | `okdctl cleanup` standalone | **Done** (PR #58) |
| N5 | `okdctl kubeconfig` | **Done** (PR #55) |
| N6 | `okdctl config show` | **Done** (PR #59) |
| N7 | `okdctl completion` | **Done** (PR #60) |
| N8 | Step timing + deploy summary | **Done** (PR #64) |
| N9 | `--log-level/--log-format/--log-file` flags | **Done** (PR #65) |
| N10 | Ctrl-C partial-progress summary | **Done** (PR #77) |
| N11 | `--yes` parity on deploy/update-ingress | **Done** (PR #68) |
| N12 | Unit tests for `netutil` | Deferred |
| N13 | Unit tests for `config/validators` | Deferred |
| N14 | `go vet` in CI | Sprint 1 |
| N15 | Wizard: Deployment fields | **Done** (PR #66) |
| N16 | Wizard: FCOSIso/TokenID/AdditionalNetworks | **Done** (PR #79) |
| N17 | Wizard review completeness | **Done** (PR #56) |
| N18 | Gateway-in-CIDR validator | **Done** (PR #57) |
| N19 | Addon-specific docs | **Done** (PR #71) |
| N20 | Doctor-check reference doc | **Done** (PR #67) |
| N21 | `CONTRIBUTING.md` | Deferred |
| N22 | Troubleshooting / FAQ | Deferred |
| N23 | HTTP error context | **Done** (PR #62) |
| N25 | Progress bars for long ops | **Done** (PR #78) |
| N26 | TUI key-value map editor component | Sprint 1 |
| M1 | `okdctl status` / `describe` | Sprint 1 |
| M2 | `okdctl debug-bundle` | Sprint 1 |
| M3 | `--dry-run` / `--plan` mode | Sprint 1 |
| M4 | OKD release URL override | **Done** (PR #61) |
| M5 | Tool binary versions override | **Done** (PR #75) |
| M6 | `DefaultBinDir` rootless | Sprint 1 |
| M7 | Provider interface extraction | **Skipped** |
| M8 | MetalLB addon | Deferred (under R2) |
| M9 | cert-manager addon | Deferred (under R2) |
| M10 | ingress-nginx addon | Deferred (under R2) |
| M11 | kube-prometheus-stack addon | Deferred (under R2) |
| M12 | SecretStore multi-provider | **Done** (PR #81) |
| M13 | Typed error hierarchy | **Done** (PR #70) |
| M13b | Complete errtypes migration across phase code | **Done** (PR #83) |
| M14 | Correlation ID | **Done** (PR #82) |
| M15 | Integration test harness | Deferred |
| M16 | Autogenerated CLI reference | **Done** (PR #76) |
| M17 | Architecture diagrams | **Done** (PR #72) |
| M18 | Homebrew tap | Deferred |
| M19 | Typed addon settings (decoder method) | Sprint 1 |
| M20 | Grouped wizard fields for addons | Sprint 1 |
| L1 | libvirt/KVM provider | **Skipped** |
| L2 | vSphere/AWS/Equinix/bare-metal/Vagrant | **Skipped** |
| L3 | Multi-distribution (RKE2, vanilla) | **Skipped** |
| L4 | `okdctl upgrade` | Deferred |
| L5 | Prometheus metrics endpoint during deploy | Sprint 1 |
| L6 | OpenTelemetry tracing | Deferred |
| L7 | Storage addons (Rook-Ceph) | Deferred (under R2) |
| L8 | Backup addons (Velero / VolSync) | Deferred (under R2) |
| L9 | Policy addons (Kyverno / Gatekeeper) | Deferred (under R2) |
| L10 | Argo CD addon | Deferred (under R2) |
| L11 | Service mesh addons | Deferred (under R2) |
| L12 | Container image distribution | **Skipped** |
| L13 | Auto-update version check | **Done** (PR #69) |
| L14 | Coverage thresholds + codecov | Sprint 1 |
| L15 | Air-gap feasibility + scoping doc | Workstream (design-doc-first) |
| R1 | Addon category model + design doc | Workstream (design-doc-first) |
| R2 | Specific addons after R1 | Deferred conversation |

## Scaffolding — verify intent (no code change)

Captured from the 2026-04-18 full audit. Per MEMORY.md §scaffolding, these
are exported-but-unreferenced symbols that MAY be future-API-shaped. Do
NOT delete without first confirming against the roadmap (either with a
code owner or by resolving the symmetric-sibling it pairs with). Each
entry lists the stable finding ID so subsequent audit runs mark it
resolved when the symmetric caller lands.

| Finding ID | Location | Why kept | Symmetric sibling |
|---|---|---|---|
| `api:25fa1be8:export-no-caller-configure` | `internal/distribution/okd/firewall/firewall.go:88` | `Configure` has no caller; `RemoveRules` (sibling) has one. Shaped for a future "configure an arbitrary port set" verb. | `RemoveRules` |
| `api:66f217c9:export-no-caller-getlatestforminor` | `internal/distribution/okd/releases/okd.go:54` | `GetLatestForMinor` / `GetLatestStable` unreferenced; `FetchVersions` is used. Shaped for `okdctl releases latest [--minor N.M]`. | `FetchVersions` |
| `api:1d5afa08:export-no-caller-shortversion` | `internal/distribution/okd/releases/types.go:76` | `ShortVersion` unreferenced; `DisplayName`, `Major`, `Minor` are used. | `DisplayName` |
| `api:98723e5d:export-no-caller-validateclusteraccess` | `internal/distribution/okd/install/flux.go:15` | `ValidateClusterAccess` / `SetupClusterAccess` / `SetupKubeconfig` only called in-package; Phase itself is exported one hop away from the CLI. | `Phase` |
| `smell:2c4d8e6b:unused-metadata-field` | `internal/addon/addon.go:24` | `AddonInfo.Category` populated ("gitops", "secrets") but unread. Shaped for a category-grouped addon listing. | — (see R1 Addon category model) |
| `smell:2be6306e:scaffolding-registry-api` | `internal/addon/registry.go:78` | `addon.IsRegistered` unreferenced but is the symmetric sibling of `Register`/`Get`/`All`/`Enabled`/`Names`. | `Register` |
| `err:d6b325cb:vocab-ad-hoc-sentinel` | `internal/infrastructure/proxmox/types.go:5` | `ErrNotConnected`, `ErrTerraformNotConfigured` exist with no `errors.Is` callers but pair symmetrically with a future `Connected()` probe. | `Connect` |
| `err:a4001485:vocab-gap-cert-pending` | `internal/errtypes/errtypes.go:5` | errtypes vocabulary covers Config/Network/Cluster/Auth but has no typed error for `Recoverable` or `CertPending` — both concrete error states named in the coordinator memo. | `ConfigError`, `ClusterError` |

**Review cadence:** re-check on every audit sweep. If a symmetric
sibling never materialises for ≥6 months and no roadmap item targets
the symbol, downgrade to a delete candidate then.

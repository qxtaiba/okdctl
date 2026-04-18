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

#### U4 — Exit codes collapse everything to 0/1/130
- **Status:** in progress — branch: fix/u4-m13-exit-codes-typed-errors
- **Category:** urgent-bugfix
- **State:** not started
- **Effort:** hours (plus M13 for the typed-error plumbing)
- **Impact:** large
- **Evidence:** `internal/cli/root.go:55,64,67` exits 0, 1, or 130. CI
  cannot distinguish config error vs network error vs cluster failure.
- **Acceptance:** documented exit-code table (config=2, network=3,
  cluster=4, auth=5, other=1). `main()` switches on typed error from M13.
- **Depends on:** M13 (typed error hierarchy) lands first or in parallel.

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

#### N1 — Wire `okdctl addon list/install/uninstall/verify`
- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/n1-addon-cli
- **Category:** half-done
- **State:** scaffolding exists
- **Effort:** days
- **Impact:** large
- **Evidence:** `internal/addon/manager.go:32` (`InstallAll`), `:116`
  (`InstallOne`), `:166` (`VerifyAll`), `:184` (`Uninstall`) — lifecycle
  complete. Only `InstallAll` is called today (from
  `internal/distribution/okd/postinstall/steps.go:99`).
- **Acceptance:** four subcommands under `okdctl addon`:
  - `list` prints catalog with registered / installed state.
  - `install <name>` calls `Manager.InstallOne`.
  - `uninstall <name>` calls `Manager.Uninstall` with dependency guard
    messaging.
  - `verify` calls `Manager.VerifyAll` and reports per-addon health.
  - `--all` on `install` calls `InstallAll`.
  - Help text documents the asymmetric rollback semantics between
    `install` (all-or-nothing) and `install --all` (per-addon continue).
  - `addon` is added to `rootRequiredCmds` if any subcommand mutates
    cluster state.
- **Depends on:** addon category refactor (R1) should land first if it
  lands before this item; otherwise this ships flat and migrates later.

#### N3 — `okdctl config validate` standalone
- **Status:** not started
- **Category:** half-done
- **State:** scaffolding exists
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `config.Validate()` is called from
  `internal/cli/helpers.go:62`; no standalone cobra command.
- **Acceptance:** `okdctl config validate [--config path.yaml]` returns
  exit 0 on success, exit 2 on validation error. Prints the same
  `ValidationResult` summary the deploy flow uses.
- **Depends on:** U4 (so validation failure maps to exit code 2).

#### M1 — `okdctl status` / `describe`
- **Status:** not started
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
- **Status:** not started
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

#### N8 — Step timing + per-step deploy summary
- **Status:** in review — PR #64
- **Category:** feature-gap
- **State:** scaffolding exists
- **Effort:** hours
- **Impact:** large
- **Evidence:** `internal/distribution/step.go:10-16` `StepResult` lacks
  `Duration`; `internal/distribution/orchestrator.go:62-88` has
  `OnStart`/`OnComplete` callbacks that are never used for timing.
  `internal/cli/summary.go:70-135` has no duration section.
- **Acceptance:** `StepResult` gains `Duration time.Duration` and
  `StartedAt time.Time`. Orchestrator records both. Post-deploy summary
  prints a per-step table (`step | result | duration`) plus a total.
- **Depends on:** none.

#### N10 — Ctrl-C partial-progress summary + resume hint
- **Status:** not started
- **Category:** polish
- **State:** scaffolding exists
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/cli/root.go:59` catches SIGINT → exit 130 with no
  guidance. Destroy flow already prints recovery hints on partial
  failure — SIGINT doesn't.
- **Acceptance:** SIGINT during deploy/destroy prints "partial progress:
  <steps done>; resume with <recommended next command>". Exit code
  stays 130.
- **Depends on:** N8 (to know which steps completed).

#### N11 — `--yes` / `--force` parity on `deploy` and `update-ingress`
- **Status:** in review — PR #68
- **Category:** feature-gap
- **State:** half-done
- **Effort:** hours
- **Impact:** medium
- **Evidence:** destroy has `--force` at `internal/cli/flags.go:25`. Deploy
  + update-ingress prompt interactively with no non-TTY fallback.
- **Acceptance:** both commands accept `--yes` / `-y`; when the stdin is
  not a TTY, prompts return their default answer rather than blocking.
- **Depends on:** none.

#### N23 — HTTP error surface: include URL + response snippet
- **Status:** in review — PR #62
- **Category:** polish
- **State:** scaffolding exists
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/download/download.go:112` returns bare status
  code; `internal/tui/wizard/steps/proxmox_discovery.go:156-164` shows
  the preferred pattern (net-error-to-actionable-message).
- **Acceptance:** download errors include method, URL, status code, and a
  ≤256-byte response-body excerpt. Body is redacted if it looks like a
  credential (matches the password redaction pattern in
  `InputField.Validate`).
- **Depends on:** U3 (retry wraps the eventual failure).

#### N25 — Progress bars for long-running operations
- **Status:** not started
- **Category:** polish
- **State:** scaffolding exists
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `schollz/progressbar` is imported only at
  `internal/download/download.go:16`. Terraform apply, bootstrap wait,
  install monitor have no progress indication; users see 15+ minutes of
  silence.
- **Acceptance:** terraform apply, bootstrap wait, install monitor each
  emit a live progress line (determinate where possible, spinner
  otherwise). Respects TTY detection (disabled when `--log-format=json`
  or stdout is piped).
- **Depends on:** N9 (TTY detection hook).

#### M14 — Correlation ID per deploy run
- **Status:** not started
- **Category:** feature-gap
- **State:** not started
- **Effort:** hours
- **Impact:** medium
- **Acceptance:** every log line carries a `run_id=<uuid>` field. ID is
  generated once per `okdctl deploy` / `destroy` invocation and stored
  on the `Orchestrator` context. Printed in the post-deploy summary.
- **Depends on:** N9 (structured log format) — but ID can be added
  regardless and wired through both console and file outputs.

#### L5 — Prometheus metrics endpoint during deploy
- **Status:** not started
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

#### N16 — Wizard collects `FCOSIso` / `TokenID` / `AdditionalNetworks`
- **Status:** not started
- **Category:** half-done
- **State:** partially done
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/config/cluster.go:95,105,108` defined but no
  wizard step collects them. Bridge discovery exists
  (`node_placement.go:87-135`); additional networks discovery does not.
- **Acceptance:** wizard Proxmox step collects `TokenID` (optional, text
  input). `FCOSIso` collected via storage-ref picker similar to existing
  ISO discovery. `AdditionalNetworks` collected as multi-select over
  discovered bridges.
- **Depends on:** none.

### Theme E — config ergonomics, air-gap, rootless

#### M3 — `--dry-run` / `--plan` mode
- **Status:** not started
- **Category:** feature-gap
- **State:** not started
- **Effort:** days
- **Impact:** large
- **Acceptance:** `--dry-run` on deploy/destroy/update-ingress skips
  the re-exec-as-root gate, prints the terraform plan output, and lists
  every mutating step it *would* execute. No real mutations. Exit 0
  on plan success, 2 on plan failure.
- **Depends on:** U4 (exit code 2 for plan failure).

#### M4 — OKD release URL override
- **Status:** in review — PR #61
- **Category:** feature-gap
- **State:** not started
- **Effort:** hours
- **Impact:** large
- **Evidence:** `internal/distribution/okd/setup/artifacts.go:19`
  hardcodes `https://github.com/okd-project/okd/releases/download/%s`.
- **Acceptance:** new config field `Deployment.OKDReleaseBaseURL` (or
  env var `OKDCTL_OKD_RELEASE_URL`) overrides the base URL. Air-gapped
  users point to a mirror. Default unchanged.
- **Depends on:** none.

#### M5 — Tool binary versions / URLs overridable
- **Status:** not started
- **Category:** feature-gap
- **State:** not started
- **Effort:** hours
- **Impact:** medium
- **Evidence:** `internal/distribution/okd/setup/tools.go:74,79` hardcodes
  helm v3.17.3, sops v3.9.4, yq latest.
- **Acceptance:** config block `Deployment.ToolVersions` with per-tool
  version + URL-template override. Defaults unchanged.
- **Depends on:** M4 (share the override pattern).

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

#### M13 — Typed error hierarchy
- **Status:** in progress — branch: fix/u4-m13-exit-codes-typed-errors
- **Category:** feature-gap
- **State:** not started
- **Effort:** days
- **Impact:** medium
- **Acceptance:** new package `internal/errtypes/` (or extension of
  `internal/config.ValidationError`) defines `ConfigError`,
  `NetworkError`, `ClusterError`, `AuthError`. Phase code wraps
  returned errors with the right type. `internal/cli/root.go` uses
  `errors.As` to pick exit code. Documented in CONTRIBUTING when that
  lands.
- **Depends on:** U4 (pairs with exit-code work).

#### M12 — Generalize SecretStore beyond 1Password
- **Status:** not started
- **Category:** half-done
- **State:** partially done
- **Effort:** days
- **Impact:** medium
- **Evidence:** `internal/addon/catalog/secretstore/secretstore.go:20-24`
  has 1P-specific paths hardcoded; `:154` `ValidateSettings` is a no-op.
- **Acceptance:** secretstore addon exposes provider selection
  (`onepassword`, `vault`, `aws-sm`, etc.) via settings; ESO CRDs
  generated per provider. Existing 1Password users keep working with
  default. `ValidateSettings` validates provider-specific fields.
- **Depends on:** addon category refactor (R1) if it lands first; can
  ship flat otherwise.

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

#### L13 — Auto-update version check on startup
- **Status:** in review — PR #69
- **Category:** polish
- **State:** not started
- **Effort:** days
- **Impact:** small
- **Acceptance:** on startup, background-check GitHub releases/latest
  (with cache, timeout, opt-out via `OKDCTL_NO_UPDATE_CHECK=1`). If a
  newer version exists, print one-line notice after the main command
  output. Never blocks the main flow.
- **Depends on:** none.

#### L14 — Coverage thresholds + codecov in CI
- **Status:** not started
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

### Theme H — docs

#### N19 — Addon-specific docs in `docs/addons/`
- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/n19-addon-docs
- **Category:** docs
- **State:** not started
- **Effort:** hours
- **Impact:** medium
- **Acceptance:** `docs/addons/flux.md` + `docs/addons/secretstore.md`
  cover: purpose, when to use, default settings, common failure modes,
  uninstall behaviour. Linked from README addon section.
- **Depends on:** none.

#### M16 — Autogenerated CLI reference
- **Status:** not started
- **Category:** docs
- **State:** not started
- **Effort:** hours
- **Impact:** small
- **Acceptance:** `docs/cli/okdctl_*.md` generated via `cobra doc`.
  New CI step runs generation and fails the pipeline on drift. Added
  to release checklist.
- **Depends on:** N7 (so completion command is covered).

#### M17 — Architecture diagrams (Mermaid)
- **Status:** in progress — worktree: /Users/qalnuaimy/Desktop/okdctl/.worktrees/m17-arch-diagrams
- **Category:** docs
- **State:** not started
- **Effort:** hours
- **Impact:** medium
- **Acceptance:** Mermaid diagrams for: phase flow (setup →
  install → postinstall), wizard flow, addon lifecycle. Embedded in
  existing `docs/architecture/*.md`.
- **Depends on:** none.

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

## Appendix — full item ledger

| ID | Item | Disposition |
|---|---|---|
| U1 | Cleanup leaves FCOS ISO cache | **Done** (audit error — see Completed) |
| U1b | Clean remote Proxmox FCOS ISO on destroy | Sprint 1 |
| U2 | Wizard never sets `Provider.Type` | Sprint 1 |
| U3 | HTTP downloads no retry | Sprint 1 |
| U4 | Exit codes only 0/1/130 | Sprint 1 |
| U5 | `cmd.Run()` without ctx (2 sites) | **Done** (landed in 65d8fce — see Completed) |
| U6 | `BuildOpaqueSecret` panic | Sprint 1 |
| N1 | `okdctl addon list/install/uninstall/verify` | Sprint 1 |
| N2 | `okdctl releases list/show` | Sprint 1 |
| N3 | `okdctl config validate` standalone | Sprint 1 |
| N4 | `okdctl cleanup` standalone | **Done** (PR #58) |
| N5 | `okdctl kubeconfig` | **Done** (PR #55) |
| N6 | `okdctl config show` | **Done** (PR #59) |
| N7 | `okdctl completion` | **Done** (PR #60) |
| N8 | Step timing + deploy summary | Sprint 1 |
| N9 | `--log-level/--log-format/--log-file` flags | **Done** (PR #65) |
| N10 | Ctrl-C partial-progress summary | Sprint 1 |
| N11 | `--yes` parity on deploy/update-ingress | Sprint 1 |
| N12 | Unit tests for `netutil` | Deferred |
| N13 | Unit tests for `config/validators` | Deferred |
| N14 | `go vet` in CI | Sprint 1 |
| N15 | Wizard: Deployment fields | **Done** (PR #66) |
| N16 | Wizard: FCOSIso/TokenID/AdditionalNetworks | Sprint 1 |
| N17 | Wizard review completeness | **Done** (PR #56) |
| N18 | Gateway-in-CIDR validator | **Done** (PR #57) |
| N19 | Addon-specific docs | Sprint 1 |
| N20 | Doctor-check reference doc | **Done** (PR #67) |
| N21 | `CONTRIBUTING.md` | Deferred |
| N22 | Troubleshooting / FAQ | Deferred |
| N23 | HTTP error context | Sprint 1 |
| N25 | Progress bars for long ops | Sprint 1 |
| M1 | `okdctl status` / `describe` | Sprint 1 |
| M2 | `okdctl debug-bundle` | Sprint 1 |
| M3 | `--dry-run` / `--plan` mode | Sprint 1 |
| M4 | OKD release URL override | Sprint 1 |
| M5 | Tool binary versions override | Sprint 1 |
| M6 | `DefaultBinDir` rootless | Sprint 1 |
| M7 | Provider interface extraction | **Skipped** |
| M8 | MetalLB addon | Deferred (under R2) |
| M9 | cert-manager addon | Deferred (under R2) |
| M10 | ingress-nginx addon | Deferred (under R2) |
| M11 | kube-prometheus-stack addon | Deferred (under R2) |
| M12 | SecretStore multi-provider | Sprint 1 |
| M13 | Typed error hierarchy | Sprint 1 |
| M14 | Correlation ID | Sprint 1 |
| M15 | Integration test harness | Deferred |
| M16 | Autogenerated CLI reference | Sprint 1 |
| M17 | Architecture diagrams | Sprint 1 |
| M18 | Homebrew tap | Deferred |
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
| L13 | Auto-update version check | Sprint 1 |
| L14 | Coverage thresholds + codecov | Sprint 1 |
| R1 | Addon category model + design doc | Workstream (design-doc-first) |
| R2 | Specific addons after R1 | Deferred conversation |

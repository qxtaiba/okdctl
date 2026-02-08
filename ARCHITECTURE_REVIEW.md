# Architecture Review — okd-proxmox-cli

**Date:** 2026-02-08
**Scope:** Structural review of the entire Go codebase (~20,400 lines, 119 files, 35 packages)
**Goal:** Identify concrete simplifications — dead code, package consolidation, dependency graph improvements

---

## 1. Codebase Overview

```
cmd/openshitctl/          28 lines   CLI entry point
internal/
  addon/                 572 lines   Plugin system (interface, registry, resolver, outputs)
    catalog/               8 lines   Init-based addon registration (blank imports)
      flux/              328 lines   Flux GitOps addon
      secretstore/       250 lines   External Secrets addon
  cli/                 1,251 lines   Cobra commands (deploy, destroy, config, wizard)
  cluster/               209 lines   K8s client for CSR approval
  config/              1,429 lines   Config types, loading, validation, defaults
  credentials/           243 lines   Proxmox credential resolution
  deployment/            327 lines   Deployment orchestration (executor, interrupt, workflow)
  distribution/          371 lines   Step-based orchestrator, PhaseContext
    okd/                 271 lines   OKD provisioner
      setup/           2,395 lines   Infrastructure provisioning (terraform, ISO, DNS)
      install/           612 lines   Bootstrap monitoring, CSR approval, worker join
      postinstall/       572 lines   Ingress, addons, HAProxy teardown, verification
      cleanup/           626 lines   Service/package/artifact cleanup
      destroy/           271 lines   Cluster destruction
      releases/          615 lines   OKD release fetching and caching
      dns/               199 lines   DNS record management
      paths/             103 lines   Shared path utils and BasePhase struct
      templates/         242 lines   Embedded Go templates (+ 12 .tmpl files)
  executor/              259 lines   Command execution wrapper
  infrastructure/
    proxmox/             312 lines   Proxmox VE API client
    terraform/           381 lines   Terraform CLI wrapper
  logging/                39 lines   Logger interface + NoopLogger
  tui/                   663 lines   TUI rendering, colors, styles, icons
    wizard/            2,326 lines   Bubbletea interactive wizard
      components/      1,071 lines   Input, selector, dropdown components
      steps/           2,212 lines   13 wizard step implementations
  utils/                  54 lines   WrapError helpers + global logger
    download/            549 lines   HTTP download with checksum verification
    netutil/             231 lines   IP/CIDR utilities
    semver/              156 lines   Semantic version parsing (UNUSED)
    system/            1,205 lines   Exec, systemd, dnsmasq, firewall, network, HTTP
pkg/version/              46 lines   Build-time version info
```

---

## 2. Dependency Graph

```
cmd/openshitctl
 └─ cli ─────────────────────────────────────────────────────────┐
     ├─ addon                                                    │
     ├─ config ──── utils, utils/netutil, utils/system           │
     ├─ credentials (leaf: no internal imports)                   │
     ├─ deployment ─── distribution, distribution/okd,           │
     │                 distribution/okd/install,                  │
     │                 distribution/okd/postinstall               │
     ├─ executor ──── logging, utils                             │
     ├─ tui ──── logging                                         │
     │   └─ wizard ──── config, tui, wizard/components           │
     │       └─ steps ──── config, releases, tui, wizard,        │
     │                     wizard/components, utils/system        │
     └─ utils/netutil                                            │
                                                                 │
 distribution/okd ───────────────────────────────────────────────┘
     ├─ setup ──── addon, config, distribution, dns, paths,
     │             templates, executor, logging, utils,
     │             utils/download, utils/netutil, utils/system
     ├─ install ── cluster, config, distribution, paths,
     │             executor, infrastructure/proxmox,
     │             infrastructure/terraform, logging, utils,
     │             utils/system
     ├─ postinstall ── addon, config, distribution, dns, paths,
     │                 executor, logging, utils, utils/netutil,
     │                 utils/system
     ├─ cleanup ── paths, setup(partial), logging, utils/system
     ├─ destroy ── config, distribution, cleanup, paths,
     │             executor, infrastructure/terraform, logging,
     │             utils, utils/system
     └─ releases ── utils, utils/system

 Leaf packages (zero internal imports):
   credentials, logging, utils/netutil, utils/semver
```

**Verdict:** No circular dependencies. The graph is acyclic and well-layered. The main concern is `deployment/executor.go` importing OKD-specific sub-packages directly (see Tier 3).

---

## 3. Findings

### 3.1 Dead Code

#### 3.1.1 Entire dead package: `internal/utils/semver/` (156 lines)

**Evidence:** Zero files import this package. Grep for `semver\.`, `"semver"`, and the full import path all return zero results outside the package itself.

**Files:**
- `internal/utils/semver/semver.go` — `Version` struct, `Parse()`, `Compare()`, `ExtractVersion()`, `AtLeast()`, `AtLeastString()`

This package was likely written for future use or was replaced by inline version checks. It is completely disconnected from the codebase.

---

#### 3.1.2 Dead executor methods (60 lines)

**File:** `internal/executor/executor.go`

| Function | Lines | Callers |
|----------|-------|---------|
| `ValidateExecResult()` | 180–198 | Only called by the dead sudo helpers below |
| `SudoCopy()` | 205–208 | 0 |
| `SudoSystemctl()` | 211–214 | 0 |
| `RunSudoInteractive()` | 218–239 | 0 |

These convenience wrappers were never adopted. The codebase uses `Run()` and `RunWithOutput()` with explicit `"sudo"` args instead.

---

#### 3.1.3 Dead interface: `CommandExecutor` (20 lines)

**File:** `internal/executor/executor_interface.go`

Defines a `CommandExecutor` interface with a compile-time assertion (`var _ CommandExecutor = (*Executor)(nil)`) but zero code ever accepts this interface as a parameter. No test mocks, no dependency injection. The concrete `*Executor` type is used everywhere.

---

#### 3.1.4 Dead function: `MustNewStepBuilder()` (13 lines)

**File:** `internal/distribution/step.go:90-102`

Defined but never called. All call sites use `NewStepBuilder(...).MustBuild()` instead.

---

### 3.2 Duplicated Code

#### 3.2.1 Path constants defined twice

**Location A:** `internal/distribution/okd/constants.go:3-5`
```go
const (
    DefaultHAProxyConfigPath = "/etc/haproxy/haproxy.cfg"
    DefaultHTTPServerRoot    = "/var/www/html"
)
```

**Location B:** `internal/distribution/okd/paths/paths.go:31-34`
```go
const (
    DefaultHAProxyConfigPath = "/etc/haproxy/haproxy.cfg"
    DefaultHTTPServerRoot    = "/var/www/html"
)
```

The `cleanup/` package re-exports from `paths`:
```go
// cleanup/cleanup.go:27-30
const DefaultHAProxyConfigPath = paths.DefaultHAProxyConfigPath
const DefaultHTTPServerRoot = paths.DefaultHTTPServerRoot
```

The `constants.go` copies in the `okd` package are never imported — all consumers use `paths.DefaultHAProxyConfigPath` or the `cleanup` re-exports. The `constants.go` file's two path constants are dead duplicates.

#### 3.2.2 Cleanup re-exports add indirection

**File:** `internal/distribution/okd/cleanup/cleanup.go:27-30`

The `cleanup` package re-exports `paths.DefaultHAProxyConfigPath` and `paths.DefaultHTTPServerRoot` as package-level constants. The sole consumer is `destroy/steps.go`. This chain (`paths` → `cleanup` → `destroy`) could be flattened to `paths` → `destroy`.

---

### 3.3 Redundant Abstractions

#### 3.3.1 `BasePhase` logging wrappers

**File:** `internal/distribution/okd/paths/paths.go:86-103`

```go
func (b *BasePhase) LogInfo(msg string)  { b.Log.Info(msg) }
func (b *BasePhase) LogWarn(msg string)  { b.Log.Warn(msg) }
func (b *BasePhase) LogError(msg string) { b.Log.Error(msg) }
func (b *BasePhase) LogDebug(msg string) { b.Log.Debug(msg) }
```

These four methods add zero logic — they are trivial one-line delegates to the public `Log` field. Yet they are called across 21 files. Since `BasePhase.Log` is an exported field of type `logging.Logger`, callers can use `b.Log.Info(msg)` directly. The wrappers and the `Executor()` / `Logger()` getters add no value when the fields are public.

#### 3.3.2 Global logger pattern in `utils/`

**File:** `internal/utils/logger.go` (32 lines)

Provides `SetLogger()` / `GetLogger()` with mutex protection. `SetLogger` is called once in `cli/root.go`. `GetLogger` is called in 7 files under `utils/system/` and `utils/download/`.

This pattern conflicts with the explicit logger injection used everywhere else in the codebase (via `logging.Logger` parameters, `BasePhase.Log`, etc.). The `utils/system` and `utils/download` packages are the only ones using this global logger pattern.

---

### 3.4 Package Consolidation Candidates

#### 3.4.1 `internal/distribution/okd/constants.go` → merge into `okd.go`

This 13-line file defines 5 constants: 2 are dead duplicates (path constants, see 3.2.1) and 3 are resource minimums (`MinControlPlaneMemoryMB`, `MinControlPlaneCPUs`, `MinControlPlaneDiskGB`). The 3 resource constants should move to `okd.go` or `paths/paths.go`, and the file should be deleted.

#### 3.4.2 `internal/logging/` → merge into `internal/utils/`

The `logging` package is 39 lines: one interface (`Logger`) and one implementation (`NoopLogger`). The `utils` package already imports `logging`. Every package that imports `logging` also imports `utils` (or a `utils/*` sub-package). Merging would reduce a package boundary without losing clarity.

**Counter-argument:** Keeping `logging` separate is idiomatic Go for a foundational interface. This is a judgment call — the savings are small (one fewer import path) and the current structure is clean. Listed as medium effort because all import paths would need updating.

---

### 3.5 Structural Observations (No Action Needed)

These patterns look like they could be issues but are actually justified:

| Pattern | Justification |
|---------|---------------|
| `pkg/version/` (46 lines) | Version vars set by `-ldflags` at build time; standard Go CLI pattern |
| `addon/catalog/catalog.go` (8 lines of blank imports) | Standard Go init-based plugin registration |
| `config/provider_adapter.go` (46 lines of getters) | Avoids circular import between `config` and `credentials` |
| `distribution/okd/paths/` as separate package | Prevents circular imports between phase packages |
| `deployment/doc.go` (11 lines) | Go package documentation convention |
| `cluster/interfaces.go` (13 lines, just `CSR` struct) | Small but used; name is misleading (no interfaces) |

---

### 3.6 Naming Inconsistencies

| Issue | Location | Suggestion |
|-------|----------|------------|
| `cluster/interfaces.go` contains only a struct (`CSR`), no interfaces | `internal/cluster/interfaces.go` | Rename to `types.go` |
| `config/validator.go` vs `config/validators.go` — confusingly similar names | `internal/config/` | Rename `validator.go` → `validation_types.go` (it holds types/constants, not validation logic) |

---

## 4. Recommendations

### Tier 1 — High impact / low effort

These are safe deletions and consolidations. Each can be done in an isolated commit.

| # | Action | Lines saved | Files affected |
|---|--------|-------------|----------------|
| 1 | **Delete `internal/utils/semver/`** — entire package is unused | 156 | 1 file deleted |
| 2 | **Delete dead executor code** — remove `ValidateExecResult`, `SudoCopy`, `SudoSystemctl`, `RunSudoInteractive` from `executor.go` | 60 | 1 file |
| 3 | **Delete `internal/executor/executor_interface.go`** — unused interface | 20 | 1 file deleted |
| 4 | **Delete `MustNewStepBuilder()`** from `internal/distribution/step.go` | 13 | 1 file |
| 5 | **Delete `internal/distribution/okd/constants.go`** — move 3 resource constants (`MinControlPlane*`) into `internal/distribution/okd/okd.go`; the 2 path constants are dead duplicates | 13 (net ~0 after moving 3 constants) | 2 files |
| 6 | **Remove `BasePhase` logging wrappers** — delete `LogInfo`, `LogWarn`, `LogError`, `LogDebug` methods from `paths/paths.go`; replace `b.LogInfo(msg)` → `b.Log.Info(msg)` across 21 files | 16 (definition) | 22 files |
| 7 | **Rename `cluster/interfaces.go` → `cluster/types.go`** | 0 | 1 file renamed |

**Estimated total:** ~265 lines removed, 3 files deleted, 1 renamed, ~25 files touched

---

### Tier 2 — Medium effort

| # | Action | Rationale |
|---|--------|-----------|
| 8 | **Flatten cleanup re-exports** — have `destroy/steps.go` import `paths` directly instead of re-exporting through `cleanup` | Removes unnecessary indirection; delete the 2 `const` aliases in `cleanup/cleanup.go` |
| 9 | **Remove global logger from `utils/`** — replace `utils.GetLogger()` calls in `utils/system/` (5 files) and `utils/download/` (3 files) with explicit `logging.Logger` parameters | Aligns with the explicit-injection pattern used everywhere else; removes hidden state |
| 10 | **Rename `config/validator.go` → `config/validation_types.go`** | Disambiguates from `validators.go` which holds actual validation logic |

---

### Tier 3 — Large effort (consider, don't rush)

| # | Action | Rationale |
|---|--------|-----------|
| 11 | **Decouple `deployment/` from OKD-specific imports** | `deployment/executor.go` imports `distribution/okd`, `distribution/okd/install`, and `distribution/okd/postinstall` directly. If a second distribution is ever added, this package should go through an abstract provisioner interface. Not worth doing until that need arises. |
| 12 | **Merge `internal/logging/` into `internal/utils/`** | Would remove one package boundary. Every consumer of `logging` already imports `utils`. However, the current separation is idiomatic Go and clean. Only do this if import path reduction is a priority. |

---

## 5. What's Working Well

The codebase has strong architectural properties that should be preserved:

- **Zero circular dependencies** — the import graph is acyclic and flows cleanly downward
- **Clean phase isolation** — setup/install/postinstall/cleanup/destroy are well-separated with `BasePhase` providing shared infrastructure
- **Addon system** — init-based registration, dependency resolution via Kahn's algorithm, and cross-addon output sharing are well-designed
- **Config validation** — centralized in `internal/config/` with a clear types/validators split
- **TUI separation** — UI logic is entirely contained in `internal/tui/` with no business logic leakage
- **Credential handling** — the adapter pattern in `config/provider_adapter.go` cleanly avoids circular imports while keeping credentials out of config YAML

# Addon category model — design plan

**Goal:** Replace the free-form `Metadata.Category` string with a typed enum,
extend the `Addon` interface to require a `Category()` method, introduce a
per-category wizard dropdown (including a first-class "None / BYO" slot), extend
the config schema for BYO Helm chart pinning, and add `okdctl addon categories`
and `okdctl addon list --category` CLI verbs. No code lands before this document
is approved.

**Status:** draft — under review.

**Affects:** `internal/addon/`, `internal/tui/wizard/steps/addons.go`,
`internal/cli/addon.go`, `internal/config/cluster.go`,
`internal/addon/catalog/flux/`, `internal/addon/catalog/secretstore/`.

---

## 1. Background and goals

okdctl targets a single infrastructure provider (Proxmox) and a curated addon
catalog. The catalog is expected to grow from two addons (flux, secretstore) to
a dozen or more covering ingress, load-balancing, cert management, monitoring,
storage, backup, policy, service mesh, and secrets management.

Two problems make growth costly today:

1. **No enforced categorisation.** `Metadata.Category` is a free-form string
   (`internal/addon/addon.go:31`). Any addon can write any string; nothing
   enforces a controlled vocabulary or prevents two addons from claiming the same
   slot within a category.

2. **Hard-coded wizard sections.** `AddonsStepDefinition` in
   `internal/tui/wizard/steps/addons.go:69` lists sections for flux and
   secretstore directly. Adding a new addon means manually extending this struct.
   The UX does not express "pick one of these for this category" semantics —
   the user sees flat form fields for every addon regardless of whether they
   want that category at all.

The goal of this model is to:

- Define a `Category` type with exactly 10 values.
- Require every addon to declare which category it belongs to via a method on
  the `Addon` interface.
- Guarantee at most one active addon per category cluster-wide (the category
  slot). A category may be set to "None" (nothing installed) or "BYO" (user
  supplies a Helm chart reference).
- Replace the flat addons wizard step with per-category dropdowns that list
  curated options, a BYO option, and a None option.
- Surface categories in the CLI.

Non-goals for this document: migrating specific addons beyond flux and
secretstore; implementing new addons from the R2 menu; multi-provider support.

---

## 2. Status quo

### 2.1 Addon interface

```go
// internal/addon/addon.go:16-22
type Addon interface {
    Info() Metadata
    Install(ctx context.Context, env *Environment) error
    Verify(ctx context.Context, env *Environment) error
    Uninstall(ctx context.Context, env *Environment) error
}
```

`Metadata` at line 27 carries:

```go
type Metadata struct {
    Name           string
    DisplayName    string
    Description    string
    Category       string   // free-form; "gitops" or "secrets" today
    Dependencies   []string
    Priority       int
    DefaultEnabled bool
}
```

### 2.2 Registry

`internal/addon/registry.go` provides `Register`, `Get`, `All`, `Enabled`,
`Names`, `IsRegistered`. There is no `ByCategory` or `EnabledByCategory` query.
The registry iterates in insertion order.

### 2.3 Config schema

`internal/config/cluster.go:88-92`:

```go
type AddonConfig struct {
    Enabled  bool              `json:"enabled"`
    Settings map[string]string `json:"settings,omitempty"`
}
```

`Config.Addons` is `map[string]AddonConfig` keyed by addon name. There is no
representation of "which addon is active for category X" or BYO chart pinning.

### 2.4 Wizard

`AddonsStepDefinition` hard-codes two sections (gitops/flux and secretstore)
as flat `SectionDefinition` slices. Each field has an explicit `ConfigSet` /
`ConfigGet` closure wired to a specific addon name and settings key. A third
addon needs a third manually added section.

### 2.5 CLI

`okdctl addon list` prints all registered addons in insertion order, no
filtering. There is no `categories` subcommand.

### 2.6 Gaps

| Gap | Location | Impact |
|-----|----------|--------|
| Free-form Category string | `addon.go:31` | No enforcement; typos silently pass |
| No category-level wizard UX | `steps/addons.go` | Can't express "pick one for ingress" |
| No None/BYO slot | `config/cluster.go` | User cannot opt out of a category cleanly |
| No BYO Helm chart fields | `config/cluster.go` | BYO not representable in config |
| No CLI category surface | `cli/addon.go` | Discoverability gap |

---

## 3. Category enum

Ten categories cover the planned catalog. The enum is defined in
`internal/addon/addon.go` (same file as the interface, no new file needed).

```go
// Category identifies the functional role an addon fills in the cluster.
// Exactly one addon (or None) may occupy a category slot at a time.
type Category int

const (
    CategoryNone        Category = iota // zero value — no addon selected
    CategoryIngress                     // HTTP/HTTPS ingress controller; "None" means use OKD built-in router
    CategoryLoadBalancer                // bare-metal load balancer (L2/L3)
    CategoryGitOps
    CategoryCert
    CategoryMonitoring
    CategoryStorage
    CategoryBackup
    CategoryPolicy
    CategoryServiceMesh                 // service-to-service proxy mesh
    CategorySecrets
)
```

Rationale for names:
- `CategoryIngress` — the ingress controller slot. OKD ships a built-in
  OpenShift router; the "None" option for this category means "use the OKD
  router as-is". An explicit ingress-nginx or Envoy Gateway addon occupies
  the slot when selected.
- `CategoryLoadBalancer` — bare-metal LB (MetalLB, kube-vip). No built-in
  equivalent on Proxmox; None is a valid first-class choice.
- `CategoryGitOps` — Flux or Argo CD. GitOps is not the default; None means
  the cluster is managed imperatively.
- `CategoryCert` — cert-manager. None means TLS is managed externally.
- `CategoryMonitoring` — kube-prometheus-stack, Loki, or "use OKD built-in".
  The OKD built-in monitoring stack is the None/default.
- `CategoryStorage` — Rook-Ceph (Proxmox-only use case today). None means
  no in-cluster storage provisioner; use Proxmox NFS directly.
- `CategoryBackup` — Velero. None means no automated backup.
- `CategoryPolicy` — Kyverno or OPA Gatekeeper. None means no policy engine.
- `CategoryServiceMesh` — user BYO only; no curated option today. None is
  the only non-BYO choice.
- `CategorySecrets` — External Secrets Operator bootstrap (secretstore addon).
  None means secrets are managed out-of-band.

A `String()` method returns the lowercase hyphenated name for logging and CLI
display:

```go
func (c Category) String() string {
    switch c {
    case CategoryIngress:
        return "ingress"
    case CategoryLoadBalancer:
        return "load-balancer"
    case CategoryGitOps:
        return "gitops"
    case CategoryCert:
        return "cert"
    case CategoryMonitoring:
        return "monitoring"
    case CategoryStorage:
        return "storage"
    case CategoryBackup:
        return "backup"
    case CategoryPolicy:
        return "policy"
    case CategoryServiceMesh:
        return "service-mesh"
    case CategorySecrets:
        return "secrets"
    default:
        return "none"
    }
}
```

A package-level `ParseCategory(s string) (Category, bool)` converts a
user-supplied string (from CLI flag or YAML) to the typed value. It accepts
the exact strings from `String()` and returns `false` for unknown values.

---

## 4. Addon interface extension

The `Addon` interface gains one method:

```go
type Addon interface {
    Info() Metadata
    Category() Category

    Install(ctx context.Context, env *Environment) error
    Verify(ctx context.Context, env *Environment) error
    Uninstall(ctx context.Context, env *Environment) error
}
```

`Category()` returns the category this addon fills. The return value must be
one of the typed constants above; `CategoryNone` is not a valid return value
for a real addon (it signals "no addon selected" in the config layer, not an
addon's own identity).

The `Metadata.Category string` field is removed from `Metadata`. Its
informational role is replaced by the typed method. Callers that previously
read `a.Info().Category` switch to `a.Category().String()`.

### Why a method rather than keeping it in Metadata

Keeping it in `Metadata` (a struct return value) would allow an addon to
return a different value across calls, and would not be checkable by the
compiler as part of the interface contract. A method makes the compiler enforce
the contract on every implementation.

### Registry extension

Two new functions are added to `internal/addon/registry.go`:

```go
// ByCategory returns all registered addons whose Category() equals c,
// in insertion order.
func ByCategory(c Category) []Addon

// EnabledByCategory returns the subset of ByCategory(c) that is enabled
// in cfg. At most one addon should be enabled per category in normal use;
// a multi-element return is treated as a configuration error by the wizard.
func EnabledByCategory(cfg *config.Config, c Category) []Addon
```

---

## 5. None / BYO as first-class values

### None

"None" for a category means the user deliberately opts out of installing any
addon in that slot. It is the zero value of the config representation and
requires no special sentinel string.

In `Config.Addons` (the existing `map[string]AddonConfig`), a category with no
entry and no BYO chart reference means "None". The wizard writes nothing for
that category when the user leaves the dropdown at "None".

In code, `EnabledByCategory(cfg, CategoryIngress)` returning an empty slice
means the ingress category is None.

### BYO

"BYO (bring your own)" means the user supplies a Helm chart reference and
values. The BYO Helm chart is installed as if it were a built-in addon but
okdctl does not know its internals (no Verify logic, no structured settings).

BYO is represented as a special addon name sentinel in the config map:
`"byo-<category>"`. For example, the BYO ingress slot is keyed as
`"byo-ingress"` in `Config.Addons`. The hyphen separator is lex-safe in
both YAML and JSON keys; no quoting is required. This lets the existing
map structure carry BYO configuration without schema changes to the map
key type.

`AddonConfig` gains three new fields for BYO chart pinning:

```go
type AddonConfig struct {
    Enabled  bool              `json:"enabled"`
    Settings map[string]string `json:"settings,omitempty"`

    // BYO chart fields — only meaningful when the key is "byo-<category>".
    Chart        string            `json:"chart,omitempty"`         // e.g. "ingress-nginx/ingress-nginx"
    ChartRepo    string            `json:"chart_repo,omitempty"`    // e.g. "https://kubernetes.github.io/ingress-nginx"
    ChartVersion string            `json:"chart_version,omitempty"` // pinned semver, e.g. "4.10.1"
    Values       map[string]string `json:"values,omitempty"`        // inline Helm --set overrides
    ValuesFile   string            `json:"values_file,omitempty"`   // path relative to project root
}
```

The BYO installer is a new built-in addon implementation registered as
`byo-<category>` at startup (one registration per category with a BYO
option). Its `Install` method calls `helm upgrade --install` with the fields
above. Its `Verify` method checks that the release exists via `helm status`.
Its `Category()` returns the appropriate category.

Because service-mesh has no curated option, its BYO path is the only
non-None choice in that category.

### Zero value reads cleanly

In code, checking "is ingress None?":

```go
if len(addon.EnabledByCategory(cfg, addon.CategoryIngress)) == 0 {
    // ingress category is None or BYO-disabled
}
```

In YAML, a cluster with no ingress addon has no `byo-ingress` or curated
ingress key under `addons:`, which is the natural zero value.

---

## 6. Wizard UX

### Current shape

The addons step (`steps/addons.go`) is one `StepDefinition` with flat
`SectionDefinition` slices. Each section is an addon. Fields within a section
are addon-specific settings inputs.

### Proposed shape

The addons step becomes a sequence of **category screens**, one per category
that has at least one curated option or a BYO path. Each screen has:

1. A **selector dropdown** (`FieldTypeSelect`) labelled after the category.
   Options: `"none"`, then the curated addon names (display names), then
   `"byo (helm chart)"`.
2. **Settings fields** for the selected curated addon, shown only when that
   addon is selected (conditional rendering controlled by the existing
   `DataDrivenStep.WithExtraContentFunc` pattern or a new show-if predicate
   on `FieldDefinition`).
3. **BYO fields** (chart, repo, version, values, values-file) shown only when
   the user selects "byo".

The category order in the wizard mirrors the enum declaration order:
ingress → load-balancer → gitops → cert → monitoring → storage → backup →
policy → service-mesh → secrets.

### Interaction flow (per category)

```
┌─────────────────────────────────────────────────────┐
│  gitops                                             │
│  ─────────────────────────────────────────────────  │
│  controller  [none          ▼]                      │
│              [none              ]                   │
│              [flux gitops       ]  ← curated        │
│              [byo (helm chart)  ]  ← BYO slot       │
└─────────────────────────────────────────────────────┘

When "flux gitops" is selected, the screen expands:

┌─────────────────────────────────────────────────────┐
│  gitops                                             │
│  ─────────────────────────────────────────────────  │
│  controller    [flux gitops   ▼]                    │
│  repository    [ssh://...        ]  ← flux field    │
│  branch        [main             ]                  │
│  path          [kubernetes/...   ]                  │
└─────────────────────────────────────────────────────┘

When "byo (helm chart)" is selected:

┌─────────────────────────────────────────────────────┐
│  gitops                                             │
│  ─────────────────────────────────────────────────  │
│  controller    [byo (helm chart) ▼]                 │
│  chart         [myorg/argo-cd     ]                 │
│  chart repo    [https://...       ]                 │
│  version       [5.51.4            ]                 │
│  values file   [                  ]  (optional)     │
└─────────────────────────────────────────────────────┘
```

### Implementation note

The `FieldDefinition` type (`internal/tui/wizard/datadriven.go:44`) already
carries a `Type` and `Options` field. A new optional `ShowIf func(step
*DataDrivenStep) bool` predicate on `FieldDefinition` (added in phase (b))
will control conditional field visibility without replacing the existing
`WithExtraContentFunc` machinery. The current `renderAddonWarnings` pattern
(steps/addons.go:261) transfers to per-category warning renderers.

Config writes from the wizard selector: when the user picks a curated addon
name, the wizard sets `cfg.Addons[addonName].Enabled = true` and clears any
sibling addon's `Enabled` for the same category. When the user picks "byo",
the wizard sets `cfg.Addons["byo-<category>"].Enabled = true` and populates
the BYO chart fields. When the user picks "none", any existing enabled addon
for that category is set to `Enabled = false`.

---

## 7. Config schema additions

### 7.1 `okdctl.yaml` before

```yaml
addons:
  flux:
    enabled: true
    settings:
      repository: ssh://git@github.com/org/repo.git
      branch: main
      path: kubernetes/clusters/production
  secretstore:
    enabled: false
    settings:
      provider: onepassword
```

### 7.2 `okdctl.yaml` after (curated addon selected)

```yaml
addons:
  flux:
    enabled: true
    settings:
      repository: ssh://git@github.com/org/repo.git
      branch: main
      path: kubernetes/clusters/production
  secretstore:
    enabled: false
    settings:
      provider: onepassword
  # New addons land here when user selects them in the wizard.
  # Categories with no selection have no entry (None is the absence of a key).
```

### 7.3 `okdctl.yaml` after (BYO ingress example)

```yaml
addons:
  byo-ingress:
    enabled: true
    chart: ingress-nginx/ingress-nginx
    chart_repo: https://kubernetes.github.io/ingress-nginx
    chart_version: "4.10.1"
    values:
      controller.replicaCount: "2"
    values_file: ""
```

The key `byo-ingress` uses a hyphen separator, which is lex-safe for
hand-editing and requires no quoting in YAML or JSON. `Config.Addons` is
`map[string]AddonConfig`; no special serialization handling is needed.

### 7.4 Back-compat

Existing `okdctl.yaml` files that have `flux:` or `secretstore:` keys
continue to load without change. The new Category-aware code reads these keys
the same way `Enabled(cfg)` does today. No migration of existing YAML is
required for the config schema itself.

---

## 8. Migration plan for existing addons

### 8.1 `flux` addon

Current state:
- `Metadata.Category = "gitops"` (`flux.go:63`).
- Implements `ConfigurableAddon`, `ToolProvider`, `WizardProvider`.

Migration (phase c):
- Remove `Category: "gitops"` from `Metadata` struct literal.
- Add `func (f *Flux) Category() addon.Category { return addon.CategoryGitOps }`.
- No change to settings keys, install/verify/uninstall logic, or config
  serialization. Existing YAML with `addons.flux.enabled: true` continues
  to work.
- The wizard section for flux is driven by the per-category dropdown; the
  existing `WizardFields()` return value is reused as the settings fields
  shown when "flux gitops" is selected.

### 8.2 `secretstore` addon

Current state:
- `Metadata.Category = "secrets"` (`secretstore.go:56`).
- Implements `ConfigurableAddon`, `ToolProvider`, `WizardProvider`.

Migration (phase c):
- Remove `Category: "secrets"` from `Metadata` struct literal.
- Add `func (s *SecretStore) Category() addon.Category { return addon.CategorySecrets }`.
- No change to settings keys, install/verify/uninstall logic, or config
  serialization. Existing YAML continues to work.
- Provider sub-sections (onepassword, vault, bitwarden) remain as
  conditional fields within the secrets category screen.

### 8.3 Compatibility guarantee

Both addons have `DefaultEnabled: false` and are disabled by default.
No user-facing behaviour changes in phase (c). An existing config with
`addons.flux.enabled: true` continues to install flux exactly as today.
The only observable change is that `okdctl addon list --category gitops`
now returns flux.

---

## 9. BYO contract

A user who selects "byo (helm chart)" for a category slot must provide:

| Field | Required | Description |
|-------|----------|-------------|
| `chart` | yes | Helm chart reference: `repo-alias/chart-name` or OCI URL `oci://...` |
| `chart_repo` | yes (unless OCI) | Helm repo URL, added via `helm repo add` before install |
| `chart_version` | yes | Pinned semver. No `latest` or floating ranges. |
| `values` | no | Inline `--set key=value` overrides as a map |
| `values_file` | no | Path to a values YAML file, relative to project root |

The BYO addon installer (registered as `"byo-<category>"`) executes:

```
if strings.HasPrefix(chart, "oci://") {
    # OCI chart: no repo registration step; helm pulls directly from the
    # OCI registry URL supplied in chart.
    helm upgrade --install byo-<category> <chart> \
        --namespace byo-<category> \
        --create-namespace \
        --version <chart_version> \
        [--set key=value ...]
        [-f <values_file>]
        --wait
} else {
    # Standard Helm repo chart: register the repo, then install.
    helm repo add byo-<category> <chart_repo>
    helm repo update byo-<category>
    helm upgrade --install byo-<category> <chart> \
        --namespace byo-<category> \
        --create-namespace \
        --version <chart_version> \
        [--set key=value ...]
        [-f <values_file>]
        --wait
}
```

For OCI charts (`chart` begins with `oci://`), `chart_repo` is ignored and
must be left empty; `ValidateSettings` returns an error if both `chart_repo`
and an OCI `chart` are supplied together.

The BYO installer implements only `Install` and `Uninstall`. `Verify` checks
that `helm status byo-<category>` exits 0 and the release is in `deployed`
state.

**Constraints:**

- `chart_version` is required (no floating). This is enforced by
  `ValidateSettings` on the BYO addon, which returns an error if the field
  is empty or contains a wildcard. The policy matches the pin-stability
  convention in CLAUDE.md.
- `values_file`, if set, must be a relative path within the project root.
  The BYO installer resolves it via `env.ProjectRoot`. Absolute paths are
  rejected by `ValidateSettings`.
- Helm is the only BYO install mechanism. Raw manifests or Kustomize paths
  are out of scope for this design.
- BYO addons have no `WizardFields()` — their four fields are wired directly
  into the category screen by the wizard step builder.
- The `Dependencies` field of BYO addon metadata is always nil. Cross-category
  dependencies for BYO charts are the user's responsibility.

---

## 10. CLI surface

### 10.1 `okdctl addon categories`

New subcommand. Lists the 10 categories, the curated addon names in each,
and whether any is currently enabled in the loaded config.

Output format (tabwriter, same style as `addon list`):

```
CATEGORY         CURATED                  ACTIVE
ingress          -                         none
load-balancer    -                         none
gitops           flux                      none
cert             -                         none
monitoring       -                         none
storage          -                         none
backup           -                         none
policy           -                         none
service-mesh     -                         none
secrets          secretstore               none
```

`CURATED` lists curated addon display names for that category (dash if none).
`ACTIVE` shows the enabled addon's name, "byo" for a BYO slot, or "none".

Implementation: iterate the 10 `Category` constants in declaration order;
for each call `ByCategory(c)` to get curated addons, then
`EnabledByCategory(cfg, c)` to determine active state.

Wire-up: new `addonCategoriesCmd` in `internal/cli/addon.go`, added to
`addonCmd.AddCommand` in `init()`.

### 10.2 `okdctl addon list --category <name>`

New flag on the existing `addonListCmd`. When `--category` is set,
`printAddonList` filters the full registry to only addons whose
`Category().String()` matches the flag value. Unknown category names return
an error listing valid values.

```
$ okdctl addon list --category gitops
NAME   DISPLAY-NAME   DEPS   IN-CONFIG
flux   Flux GitOps    -      yes
```

The `--category` flag accepts the exact strings from `Category.String()`:
`ingress`, `load-balancer`, `gitops`, `cert`, `monitoring`, `storage`,
`backup`, `policy`, `service-mesh`, `secrets`.

---

## 11. Phased rollout

Phase dependencies are listed explicitly. Each phase is a separate PR.

### Phase (a) — interface + Category() method

**Scope:**
- Add `Category` type, constants, `String()`, and `ParseCategory` to
  `internal/addon/addon.go`.
- Remove `Category string` from `Metadata`.
- Add `Category() Category` to the `Addon` interface.
- Add `ByCategory` and `EnabledByCategory` to `internal/addon/registry.go`.
- Update `flux` and `secretstore` to implement `Category()` and remove
  `Metadata.Category` string.
- Update `printAddonList` to print `a.Category().String()` in the CATEGORY
  column.

**Verification:** `make build`, `make lint`, `make vet`. No existing tests
break (the registry tests in `internal/addon/manager_test.go` and
`resolver_test.go` use test stubs — those stubs must also implement
`Category()`).

**Does not include:** wizard changes, config schema changes, new CLI verbs.

**Risk:** interface change forces all test stubs to add `Category()`. This is
the highest-friction change in the plan; doing it first keeps later phases
clean.

### Phase (b) — wizard dropdown rewrite

**Scope:**
- Add `ShowIf func(*DataDrivenStep) bool` to `wizard.FieldDefinition` in
  `internal/tui/wizard/datadriven.go`. Fields with a non-nil `ShowIf` are
  hidden unless the predicate returns true.
- Rewrite `AddonsStepDefinition` in `steps/addons.go` to use per-category
  sections. Each section has one selector field and conditional settings
  fields driven by `ShowIf`.
- Add BYO fields to each category section.

**Depends on:** phase (a) (ByCategory available for building option lists).

**Does not include:** config schema changes (BYO fields in AddonConfig are
added in this phase since the wizard needs to write them).

**Risk:** medium. The `DataDrivenStep` rendering path (`datadriven.go`) must
correctly handle `ShowIf` predicates without breaking existing steps. The
`WithExtraContentFunc` pattern used by `renderAddonWarnings` transfers to
per-category warning functions.

### Phase (c) — migrate existing addons

**Scope:**
- `flux`: add `Category()`, remove `Metadata.Category`, confirm no settings
  or install logic changes.
- `secretstore`: same.
- Update all call sites that read `a.Info().Category` or `info.Category`:
  - `internal/cli/status.go:360` — replace `info.Category` with
    `a.Category().String()` in the tabwriter row for the describe command.
  - Confirm no further call sites remain by running
    `grep -r 'Info()\.Category\|info\.Category' ./internal` before opening
    the PR; the grep must return zero results from outside the addon
    packages.

**Depends on:** phase (a) (interface exists). Can be done concurrently with
phase (b) but must land before phase (d).

**Risk:** low. These are additive changes to two files.

### Phase (d) — CLI verbs + unblock R2

**Scope:**
- Add `okdctl addon categories` subcommand.
- Add `--category` flag to `okdctl addon list`.
- Add `ParseCategory` validation to the flag handler.

**Depends on:** phases (a) and (c) (Category type and ByCategory available).

**Risk:** low. Pure CLI surface addition.

---

## 12. Open questions / non-goals

### Open questions

1. **Category uniqueness enforcement.** Should the `Manager` or `Registry`
   actively block registering two curated addons with the same `Category()`
   return value? Today, enforcement is by convention. The design above relies
   on `EnabledByCategory` returning at most one result; a multi-result return
   is treated as a config error by the wizard. Reviewers should decide whether
   a hard `Register` error is preferable for build-time safety.

2. **BYO installer namespace.** The plan uses `byo-<category>` as the
   namespace for BYO Helm releases. Reviewers should confirm this is
   acceptable or propose a user-configurable namespace field.

3. **`okdctl addon list` CATEGORY column.** The existing `list` output has
   four columns: NAME, DISPLAY-NAME, DEPS, IN-CONFIG. Adding a CATEGORY
   column changes the table shape. This is a minor UX break for any scripts
   parsing the output. Confirm whether this is acceptable before phase (a)
   ships.

4. **Monitoring "use OKD built-in" option.** OKD ships a monitoring stack
   out of the box. The "None" option for CategoryMonitoring should be
   documented as "use OKD built-in" to avoid user confusion. This is a label
   decision, not a code decision.

5. **`ShowIf` predicate vs. multi-step.** The wizard rewrite in phase (b)
   uses a `ShowIf` predicate on `FieldDefinition` for conditional rendering.
   An alternative is one `DataDrivenStep` per category (10 steps). Multiple
   steps is more isolated but increases step count significantly and requires
   step-skipping logic when a category is "none". Reviewers should confirm
   the single-step-with-ShowIf approach.

6. **PolicyEngine: Kyverno vs. OPA Gatekeeper.** Both are in the R2 menu.
   The category model supports listing both as curated options within
   CategoryPolicy, but they are mutually exclusive. The "at most one active
   per category" invariant covers this automatically.

### Non-goals

- Multi-provider support. Proxmox is the only provider; no provider-specific
  category filtering.
- Windows or macOS compatibility. Linux-only deploy; okdctl runs on the
  bastion.
- Auto-discovery of BYO addons from Helm repos. The BYO contract requires
  explicit chart, repo, and version fields.
- Category hierarchies or subcategories.
- Addon marketplace or remote registry.

---

## 13. References

| Resource | Path |
|----------|------|
| Addon interface | `internal/addon/addon.go:13-22` |
| Metadata struct | `internal/addon/addon.go:27-35` |
| Global registry | `internal/addon/registry.go` |
| Manager / lifecycle | `internal/addon/manager.go` |
| Flux addon | `internal/addon/catalog/flux/flux.go` |
| SecretStore addon | `internal/addon/catalog/secretstore/secretstore.go` |
| Addons wizard step | `internal/tui/wizard/steps/addons.go` |
| FieldType enum | `internal/tui/wizard/datadriven.go:17-28` |
| CLI addon commands | `internal/cli/addon.go` |
| AddonConfig struct | `internal/config/cluster.go:88-92` |
| Config.Addons map | `internal/config/cluster.go:23` |
| R2 addon menu | `roadmap.md` (lines 123-144) |

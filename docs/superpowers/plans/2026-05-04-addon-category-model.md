# Addon category model — design doc

**Date:** 2026-05-04
**Implements:** R1 phase 0 (design only; phases (a)-(d) are out of scope for this PR)
**Supersedes:** nothing — this is a net-new model layered over the existing addon system

---

## Why this design exists

The audit finding (roadmap.md §"Product philosophy", point 3) calls for a
curated-plus-alternatives catalog with per-category wizard dropdowns and a
"None / BYO" first-class zero value in every slot. Today the catalog
hard-codes two addons (flux, secretstore) and the wizard in
`internal/tui/wizard/steps/addons.go` is a flat list of yes/no toggles.
Adding a third addon requires editing both the catalog blank-import and
the wizard step definition. There is no concept of categories, slots,
recommended vs. alternative picks, or user-supplied Helm charts.

The immediate pain points this design addresses:

- Flux is not opt-in by default in the product philosophy, but the wizard
  treats it as a named toggle with no "None" state visible above the fold.
  Users who want no GitOps tooling must actively set flux_enabled=no and
  understand that flux is the only option.
- secretstore has no peer alternative in the secrets category; adding one
  today would require hardcoding it alongside flux in addons.go.
- The `Metadata.Category` string field on line 31 of
  `internal/addon/addon.go` is populated by both existing addons
  ("gitops", "secrets") but is never read by the registry, manager,
  wizard, or CLI. It is dead weight until this design formalises it.

This design establishes the *shape* of the category model — enum,
interface extension, config schema, wizard UX, BYO contract, CLI surface,
and migration path — so that phases (a)–(d) can land without renegotiating
fundamentals mid-flight.

---

## Category enum

Ten categories cover the addon surface the product philosophy names plus
`secrets` already present in the codebase.

```go
// Category is the functional slot an addon occupies in the catalog.
// Values are load-bearing YAML keys and CLI filter tokens — change carefully.
type Category string

const (
    CategoryIngress     Category = "ingress"
    CategoryLoadBalancer Category = "load-balancer"
    CategoryGitOps      Category = "gitops"
    CategoryCert        Category = "cert"
    CategoryMonitoring  Category = "monitoring"
    CategoryStorage     Category = "storage"
    CategoryBackup      Category = "backup"
    CategoryPolicy      Category = "policy"
    CategoryServiceMesh Category = "service-mesh"
    CategorySecrets     Category = "secrets"

    // CategoryNone is the explicit "no addon for this category" zero value.
    // An addon must never return CategoryNone from Category().
    CategoryNone Category = ""
)
```

The typed-string variant is chosen over `iota`-based int for the same
reasons `phase.NodeRole` uses it (internal/distribution/okd/phase/noderole.go):
string literals survive YAML round-trips without a custom marshaller, CLI
filter tokens are human-readable without a lookup table, and the constant
values are self-documenting in log output.

`CategoryNone` is the config-level sentinel meaning "user has explicitly
chosen no addon for this category." It must never be returned by a
registered addon's `Category()` method — the registry will reject it.

### Per-category definitions and current catalog coverage

| Category | Definition | Examples in the wild | okdctl today |
|---|---|---|---|
| `ingress` | Kubernetes Ingress controller | ingress-nginx, Envoy Gateway, Traefik | none |
| `load-balancer` | L4 LoadBalancer service implementation | MetalLB, kube-vip | none |
| `gitops` | Continuous delivery / cluster reconciliation | Flux, ArgoCD | flux ✓ |
| `cert` | TLS certificate issuance and rotation | cert-manager, trust-manager | none |
| `monitoring` | Metrics, alerting, dashboards | kube-prometheus-stack, VictoriaMetrics | none |
| `storage` | Persistent volume provisioners | Longhorn, OpenEBS, Rook-Ceph | none |
| `backup` | Cluster and volume backup | Velero, Kasten K10 | none |
| `policy` | Admission control and policy enforcement | Kyverno, OPA Gatekeeper | none |
| `service-mesh` | mTLS, traffic management, observability | Istio, Linkerd | none |
| `secrets` | External secret synchronisation | External Secrets Operator | secretstore ✓ |

R2 and later items will populate the non-flux/secretstore categories with
concrete addon implementations. This design doc covers the model only.

---

## Addon interface extension

### Current interface (`internal/addon/addon.go:16-22`)

```go
type Addon interface {
    Info() Metadata
    Install(ctx context.Context, env *Environment) error
    Verify(ctx context.Context, env *Environment) error
    Uninstall(ctx context.Context, env *Environment) error
}
```

`Metadata` (lines 27-35) already carries `Category string` as a
free-form field. It is populated by both existing addons but is never
read anywhere in the registry, manager, or wizard.

### New interface (phase (a) target)

```go
// Addon is the contract every pluggable cluster feature must satisfy.
// Implementations self-register via init() and are installed in dependency
// order by Manager.
type Addon interface {
    Info() Metadata

    // Category returns the functional slot this addon occupies.
    // Must not return CategoryNone — the registry rejects registrations
    // that do.
    Category() Category

    Install(ctx context.Context, env *Environment) error
    Verify(ctx context.Context, env *Environment) error
    Uninstall(ctx context.Context, env *Environment) error
}
```

`Metadata.Category string` is removed at the same time: the string field
is replaced by the typed `Category()` method so the compiler enforces it.
`Metadata.DisplayName` and `Metadata.Description` stay — they drive the
wizard labels and `okdctl addon list` output.

Callers that today read `addon.Info().Category` switch to `addon.Category()`.
The registry's `Register` function gains a guard:

```go
if a.Category() == CategoryNone {
    return fmt.Errorf("addon %q must return a non-empty Category()", a.Info().Name)
}
```

### Slot ranking within a category

Each addon also declares its position within a category via a new
`Metadata.Slot` field:

```go
type Slot int

const (
    SlotRecommended Slot = iota  // the curated default for this category
    SlotAlternative1             // first alternative
    SlotAlternative2             // second alternative
    // SlotBYO is not a registered addon — it is a wizard-only sentinel
    // that triggers the BYO Helm config path.
)
```

`Slot` uses iota-int because the values drive wizard display ordering and
are never serialised to YAML or CLI tokens. Slot is a `Metadata` field;
it is not part of the `Addon` interface because it is static metadata.

With this model the wizard can enumerate `addon.AllByCategory(cat)` and
render them in slot order, then append the BYO sentinel.

---

## Per-category "None / BYO" zero value

### None

`CategoryNone Category = ""` is the zero value of the `Category` type.
At the config level it manifests as:

```yaml
addons:
  categories:
    ingress: none
    gitops: flux
    secrets: secretstore
```

`none` (or the empty string) means the user has explicitly chosen no
addon for the category. The deploy phase skips the category entirely.
It is distinct from the key being absent (absent → use default, which for
all categories is `none` until the user makes an explicit choice).

`okdctl addon list` shows `(none)` in the selected column for categories
where the value is absent or `none`. The wizard pre-selects `None` for
every category where no choice has been made.

### BYO (Bring Your Own Helm chart)

BYO is not a registered `Addon` implementation. It is a wizard-visible
sentinel that causes the selected slot for a category to be stored as the
string `"byo"` under `addons.categories.<cat>`. The deploy phase
detects `"byo"` and dispatches to a generic Helm installer that reads
the BYO block from config (see "BYO contract" section below).

This keeps the `Addon` interface clean: no registered addon ever returns
`"byo"` from `Category()`. The BYO path is a fallback executed by the
category manager, not by an `Addon` implementation.

---

## Wizard UX

### Current state

`internal/tui/wizard/steps/addons.go:69-250` defines `AddonsStepDefinition`
as a flat `wizard.StepDefinition` with sections named after addons and
their provider sub-groups. Every section is always visible regardless of
whether the addon is enabled. Adding a new addon means extending this file
by hand.

### Target UX (phase (b))

Each category gets a primary `FieldTypeSelect` dropdown at the top of
its section. The options are built at wizard-init time from the registry:

```
ingress:
  [ None (no ingress controller)          ]  ← default
  [ ingress-nginx (recommended)            ]
  [ Envoy Gateway (alternative)            ]
  [ BYO — provide Helm chart details       ]

gitops:
  [ None (no GitOps)                       ]  ← default for new clusters
  [ Flux (recommended)                     ]
  [ ArgoCD (alternative)                   ]
  [ BYO — provide Helm chart details       ]

secrets:
  [ External Secrets Operator (recommended)]  ← secretstore is the only option today
  [ None (no external secrets sync)        ]
  [ BYO — provide Helm chart details       ]
```

Option labels follow the pattern `<DisplayName> (<slot label>)` for
registered addons, `None (<one-line definition>)` for the none sentinel,
and `BYO — provide Helm chart details` for the BYO path.

When the user selects a registered addon, the section below the dropdown
expands to show that addon's `WizardFields()` (same mechanism as today's
provider sub-groups in the secretstore section). When the user selects BYO,
a fixed set of BYO fields appears (chart, version, repo, values — see BYO
contract). When the user selects None, no extra fields appear.

This dynamic expansion is implemented by `DataDrivenStep`'s existing
conditional field support (the `KVAsDelimitedString` / `Group` mechanism
in `internal/tui/wizard/datadriven.go`). The category dropdown's value is
the config key; fields whose `Group` matches the selected addon name are
shown.

### Non-interactive flow (`okdctl deploy --yes`)

The `--yes` flag bypasses the wizard. In non-interactive mode, category
selections are read directly from the config YAML. A cluster config with:

```yaml
addons:
  categories:
    gitops: flux
```

causes the `flux` addon to be installed. A cluster config with no
`addons.categories` block installs nothing (all categories default to
`none`). The BYO path works identically: if `addons.categories.ingress`
is `byo`, the deploy phase reads `addons.byo.ingress` for the Helm block.

---

## Migration of existing addons

### flux → gitops, SlotRecommended

Today flux's `Info()` returns `Category: "gitops"` as a string. After
phase (a) lands, flux implements `Category() Category { return CategoryGitOps }`
and its `Metadata.Slot` is set to `SlotRecommended`.

The existing config schema uses:

```yaml
addons:
  flux:
    enabled: true
    settings:
      repository: ssh://git@github.com/org/repo.git
      branch: main
      path: kubernetes/clusters/production
```

The new schema uses:

```yaml
addons:
  categories:
    gitops: flux
  flux:
    settings:
      repository: ssh://git@github.com/org/repo.git
      branch: main
      path: kubernetes/clusters/production
```

Note that `addons.flux.enabled` disappears; enablement is derived from
`addons.categories.gitops == "flux"`. The settings sub-block remains
unchanged in structure — only its `enabled` sibling is removed.

The in-memory loader migration (see "Migration path for existing users"
below) handles the translation transparently for one quarter. Flux's
install logic does not change; only the registration surface changes.

The product philosophy (roadmap.md §3 and §4) mandates that GitOps is
not default. New cluster configs produced by the wizard after phase (b)
will have `addons.categories.gitops: none` unless the user explicitly
selects Flux or another GitOps addon. Existing clusters that have
`addons.flux.enabled: true` retain their behaviour via the loader
migration until the legacy schema is dropped.

The deploy-without-flux scenario is explicit: if `addons.categories.gitops`
is `none` (or absent), the GitOps category is skipped entirely, no
`flux-system` namespace is created, and no Helm release is attempted.
The flux addon's `Install()` function is never called.

### secretstore → secrets, SlotRecommended

secretstore's `Info()` already returns `Category: "secrets"`. After phase
(a) it implements `Category() Category { return CategorySecrets }` with
`Metadata.Slot = SlotRecommended`.

The config migration for secretstore is identical in shape to flux:
`addons.secretstore.enabled` is replaced by
`addons.categories.secrets: secretstore`. Settings remain in
`addons.secretstore.settings`. The legacy loader migration handles both
addons in a single pass.

---

## BYO contract

BYO allows a user to install an arbitrary Helm chart through a category
slot. The user provides:

```yaml
addons:
  categories:
    ingress: byo
  byo:
    ingress:
      chart: ingress-nginx/ingress-nginx
      version: "4.10.1"
      repo: https://kubernetes.github.io/ingress-nginx
      repo_checksum: sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
      namespace: ingress-nginx
      values: automation/config/ingress-values.yaml
```

| Field | Required | Validation |
|---|---|---|
| `chart` | yes | non-empty; `<repo-alias>/<chart-name>` or OCI `oci://...` form |
| `version` | yes | non-empty semver or semver-prefix (Helm accepts `4.x`) |
| `repo` | yes if chart is not OCI | must be HTTPS; rejected if HTTP or empty for non-OCI charts |
| `repo_checksum` | recommended | if present, `helm repo add` output is compared; warn-only if absent |
| `namespace` | no | defaults to `<category-name>` (e.g., `ingress`) |
| `values` | no | if present, must be a path relative to the project root; rejected if the path escapes the project root (no `../`) |

Validation rules:
- Refuse `chart` empty.
- Refuse `repo` not starting with `https://` for non-OCI charts. This
  blocks MITM-able plain-HTTP repos.
- Refuse `values` path that contains `..` or is absolute.
- Refuse `version` empty.
- Warn (do not error) if `repo_checksum` is absent — advanced users may
  deliberately omit it for private registries.

OCI charts (Helm v3.7+, `oci://` scheme) are supported. When `chart`
starts with `oci://`, `repo` is ignored and `helm upgrade --install` is
invoked with the full OCI reference as the chart argument. The
`repo_checksum` field has no effect for OCI charts (Helm does not expose
a digest-verification surface at `helm upgrade` time in v3; this may
change in Helm v4).

Dependency handling: `--dependency-update` is passed by default so BYO
charts with subcharts work out of the box. Users who want to skip
dependency resolution can add `no_dep_update: true` to the BYO block;
this maps to omitting `--dependency-update` and is useful for air-gapped
installs where sub-chart repos may be unreachable.

The BYO installer does not support `helm diff`, rollback hooks, or
post-install verification beyond `helm upgrade --wait`. The `Verify()`
step for a BYO-installed category is a no-op (success) — the user is
responsible for verifying their own chart.

---

## CLI surface

### `okdctl addon categories`

Lists all 10 categories with their one-line definitions and, for clusters
where a config is loaded, the currently selected addon (or `none`).

```
$ okdctl addon categories
CATEGORY       DEFINITION                                          SELECTED
ingress        Kubernetes Ingress controller                       none
load-balancer  L4 LoadBalancer service implementation              none
gitops         Continuous delivery / cluster reconciliation        flux
cert           TLS certificate issuance and rotation               none
monitoring     Metrics, alerting, dashboards                       none
storage        Persistent volume provisioners                      none
backup         Cluster and volume backup                           none
policy         Admission control and policy enforcement            none
service-mesh   mTLS, traffic management, observability             none
secrets        External secret synchronisation                     secretstore
```

When no config file is present the `SELECTED` column shows `none` for all
categories.

Implementation target: `internal/cli/addon.go` — a new `categoriesCmd`
cobra command registered under the existing `addonCmd`.

### `okdctl addon list --category <name>`

The existing `okdctl addon list` output is filtered to show only addons
in the requested category. `--category` is repeatable. Invalid category
names print the valid list and exit non-zero.

```
$ okdctl addon list --category gitops
NAME   DISPLAY NAME    CATEGORY  SLOT         STATUS
flux   Flux GitOps     gitops    recommended  installed
```

Implementation target: `internal/cli/addon.go` — extend the existing
`listCmd` with a `--category` flag.

### `okdctl addon set-category` — deferred

Mutating the category selection from the CLI (`okdctl addon set-category
ingress=ingress-nginx`) is a follow-up item. For this design, category
selections are made either via the wizard or by directly editing
`okdctl.yaml`. The friction is intentional: a category change has
install/uninstall implications that the CLI cannot safely handle without
a confirmation flow, which can be designed separately.

---

## Migration path for existing users

Users on the current `addons.flux.enabled: true` / `addons.secretstore.enabled: true`
schema need a transparent migration path.

### In-memory loader migration in `internal/config/loader.go`

After `LoadFile` succeeds and schema version is validated, a
`migrateAddonSchema` function runs before the config is returned. It
detects the legacy schema by checking whether any addon key in
`config.Addons` has `Enabled == true` and whether
`config.Addons["categories"]` is absent (the new schema always has it):

```go
// migrateAddonSchema rewrites the legacy addons.X.enabled=true schema
// to the category-slot representation in memory. It does not write to
// disk — the user is responsible for persisting the new format.
func migrateAddonSchema(cfg *Config, logger *slog.Logger) {
    if _, hasCats := cfg.Addons["categories"]; hasCats {
        return // already in new format
    }
    legacySlots := map[string]string{
        "flux":        "gitops",
        "secretstore": "secrets",
    }
    for addonName, cat := range legacySlots {
        ac, ok := cfg.Addons[addonName]
        if !ok || !ac.Enabled {
            continue
        }
        if cfg.CategorySelections == nil {
            cfg.CategorySelections = make(map[Category]string)
        }
        cfg.CategorySelections[Category(cat)] = addonName
        ac.Enabled = false // strip the legacy flag; selection is now via category
        cfg.Addons[addonName] = ac
        logger.Warn("config: legacy addons."+addonName+".enabled detected; using category-slot schema instead — update okdctl.yaml: set addons.categories."+cat+": "+addonName)
    }
}
```

The `tui.Warn` is surfaced once per load. The log line points at the new
key. The migration is in-memory only: the user must run `okdctl config
migrate` (a follow-up CLI item) or hand-edit their YAML to persist the
new format.

### Deprecation timeline

- Phase (c) landing: legacy schema detected → warn, continue working.
- One quarter after phase (c) merges: the `migrateAddonSchema` function
  is replaced by a hard error ("legacy addons.X.enabled schema is no
  longer supported; run `okdctl config migrate`").
- The error path is gated on a date constant so the exact quarter boundary
  can be adjusted in one line.

### New config schema shape

`config.Config` gains a `CategorySelections` field:

```go
type Config struct {
    // ...existing fields...
    CategorySelections map[Category]string `json:"addons_categories,omitempty"`
}
```

The JSON key is `addons_categories` rather than nesting under `addons`
to keep the YAML diff minimal and avoid colliding with the existing
`addons` map key. The wizard and CLI write to this field; `Loader.Save`
serialises it normally.

A `BYOConfigs map[Category]BYOConfig` field is added alongside it:

```go
type BYOConfig struct {
    Chart        string `json:"chart"`
    Version      string `json:"version"`
    Repo         string `json:"repo,omitempty"`
    RepoChecksum string `json:"repo_checksum,omitempty"`
    Namespace    string `json:"namespace,omitempty"`
    Values       string `json:"values,omitempty"`
    NoDepUpdate  bool   `json:"no_dep_update,omitempty"`
}
```

For the on-disk schema, the YAML representation of a cluster that has
flux and a BYO ingress looks like:

```yaml
schemaVersion: v1
cluster:
  name: homelab
  domain: example.com
# ...
addons_categories:
  gitops: flux
  ingress: byo
addons_byo:
  ingress:
    chart: ingress-nginx/ingress-nginx
    version: "4.10.1"
    repo: https://kubernetes.github.io/ingress-nginx
    namespace: ingress-nginx
    values: automation/config/ingress-values.yaml
addons:
  flux:
    settings:
      repository: ssh://git@github.com/org/repo.git
      branch: main
      path: kubernetes/clusters/production
```

---

## Out of scope for this PR

The following four code phases are gated on this design doc landing.
Each is a separate PR.

**(a) Interface + Category() method** — target files:
`internal/addon/addon.go` (add `Category() Category`, remove
`Metadata.Category string`), `internal/addon/registry.go` (guard in
`Register`), `internal/addon/catalog/flux/flux.go` (implement
`Category()`), `internal/addon/catalog/secretstore/secretstore.go`
(implement `Category()`). Estimated effort: 1-2 hours. Dependency: this
design doc approved.

**(b) Wizard dropdown rewrite** — target file:
`internal/tui/wizard/steps/addons.go`. Replace flat section list with
per-category dropdown sections populated from the registry. Estimated
effort: 1-2 days. Dependency: phase (a) merged.

**(c) Migrate existing addons + loader** — target files:
`internal/config/cluster.go` (add `CategorySelections`, `BYOConfigs`),
`internal/config/loader.go` (add `migrateAddonSchema`), existing
addon Info() fields. Estimated effort: 1 day. Dependency: phase (a).

**(d) Unblock R2** — phase (c) merged; the category model is live;
R2-specific addon picks (ingress-nginx, cert-manager, etc.) can be
registered as new catalog packages. Dependency: phase (c).

---

## Open questions for the maintainer

1. **`addons_categories` vs. nesting under `addons`**: the design uses a
   top-level `addons_categories` JSON key to avoid colliding with the
   existing `addons map[string]AddonConfig`. An alternative is to reserve
   the string key `"categories"` inside the `addons` map and store a
   special struct there. That would keep a single `addons:` block in YAML
   but requires `AddonConfig` to be an `any` or an interface type, which
   complicates strict unmarshalling. The `addons_categories` top-level key
   is simpler; pick one before phase (c) lands.

2. **Slot label text in the wizard**: the design uses `(recommended)` and
   `(alternative)` as suffixes. Should alternatives be labelled by
   popularity rank (alternative 1, alternative 2) or alphabetically? The
   rank approach implies a curation judgment call per category. Alphabetical
   is neutral but less opinionated. Pick before phase (b) lands.

3. **OCI chart BYO support**: the design includes OCI chart support
   (`oci://` prefix). Helm v3.7+ supports it. If the homelab target
   clusters run OKD versions that ship Helm < 3.7 (via `helm` binary on
   the bastion), OCI support may not be available. Confirm the minimum
   Helm version assumption before phase (c) adds the OCI validation path.

4. **`repo_checksum` semantics**: the design makes `repo_checksum`
   warn-only when absent. An alternative is to require it for all
   non-OCI BYO charts (strict supply-chain posture) or to omit the field
   entirely and rely on Helm's built-in chart digest verification (if
   the repo supports it). Decide before phase (c) lands.

5. **`okdctl config migrate` CLI command**: the deprecation path mentions
   a `okdctl config migrate` command to persist the in-memory schema
   rewrite to disk. This command is not designed here — it is a follow-up.
   Decide whether the one-quarter deprecation window is sufficient without
   an automated migration command, or whether the command should land in
   phase (c).

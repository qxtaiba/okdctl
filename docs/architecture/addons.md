# The addon system

The addons are optional components the user can opt into per-cluster:
things like Flux for GitOps, cert-manager for TLS, an external secret
store, or a storage class. They're decoupled from the core phase model
in that the phases deploy OKD itself and stop, while the addons install
on top of a live cluster.

## Goals

1. Zero cost for users who don't want them: with no addons enabled, you
   pay no install time and no cluster resources.
2. Discoverable in the wizard, which lists all registered addons and
   lets the user pick which to install during the post-install phase.
3. Pluggable without modifying core code: adding a new addon means
   dropping a file in the catalog, with no edits to the wizard, phase,
   or manager.
4. Dependency-aware: an addon can declare that it depends on another
   addon (say, cert-manager on trust-manager) and the manager resolves
   the order.

## The Addon interface

```go
type Addon interface {
    Info() Metadata
    Install(ctx context.Context, env *Environment) error
    Verify(ctx context.Context, env *Environment) error
    Uninstall(ctx context.Context, env *Environment) error
}
```

The addons that take user-tunable settings opt into the
`ConfigurableAddon` sub-interface:

```go
type ConfigurableAddon interface {
    Addon
    DefaultSettings() map[string]string
    ValidateSettings(settings map[string]string) []string
}
```

`Info` is static metadata (name, display name, dependencies, priority);
`Install`, `Verify`, and `Uninstall` do the actual work against a live
cluster via the `Environment` (which carries `AddonConfig`, an executor, a
logger, and the project root). `DefaultSettings` and `ValidateSettings`
describe an addon's settings map. The manager calls `ValidateSettings`
before `Install` and aborts the install on any errors; `DefaultSettings`
has no polymorphic caller today. That leaves typed decoding to each addon:
an unexported `decodeSettings` its own `Install` calls directly, with no
`any` round-trip on the interface.

## Registration via init()

Every addon package has an `init()` function that calls `addon.Register`:

```go
func init() {
    if err := addon.Register(&fluxAddon{}); err != nil {
        panic(err)  // init() cannot propagate
    }
}
```

The binary imports the catalog root for side effects:

```go
import _ "github.com/qxtaiba/okdctl/internal/addon/catalog"
```

which in turn imports each addon package for its side effects. This is
why every addon automatically becomes visible to the wizard without any
explicit wiring: the import graph is the registration.

**Caveat:** because registration happens via `init()`, the order is
non-deterministic. In particular, addons must not depend on other addons
being *registered* (they can depend on other addons being *installed*,
via `Info().Dependencies`).

## The catalog

`internal/addon/catalog/` is the top-level package that blank-imports
every supported addon. Adding a new addon means:

1. Create a package under `internal/addon/catalog/<name>/`
2. Implement the `Addon` interface
3. Add an `init()` that calls `addon.Register`
4. Blank-import your package in `internal/addon/catalog/catalog.go`

Nothing else. The wizard, the manager, and the phase all discover your
addon automatically via the registry.

## Lifecycle

The post-install phase iterates enabled addons and calls their `Install`
in dependency order, then `Verify`. On failure, `InstallAll` has
per-addon rollback: if addon C fails to install or verify, the manager
attempts to uninstall C itself (under a bounded, cancellation-detached
context), records the error, skips any addon that depends on C, and
returns the aggregated error at the end. Addons that already installed
successfully stay installed.

Addon lifecycle (per addon, in dependency order):

```mermaid
flowchart TD
    A([InstallAll]) --> B{any enabled?}
    B -->|no| Z([skip])
    B -->|yes| C[Resolve dependency order]
    C --> D{next addon}
    D -->|dep failed| SK[skip — log warning]
    SK --> D
    D -->|all done| OK([return errors])
    D -->|proceed| E[Install]
    E -->|error| R[Uninstall rollback]
    R --> ERR[record error]
    ERR --> D
    E -->|ok| F[Verify]
    F -->|error| R
    F -->|ok| G[log installed]
    G --> D
```

## Wizard field layout lives in the wizard package

The addons that need user input (e.g., Flux wants a Git repository URL)
do not declare their own field list. The wizard's addons step
(`internal/tui/wizard/steps/addons.go`) hand-builds a `DataDrivenStep` with
its own field definitions, importing each addon package only for its
`SettingXxx` constants. This is deliberate: the wizard needs per-field
input type (select vs free text vs key-value), option lists, grouped
sections with notes, and cross-field warnings computed from filesystem and
tool checks — richness a flat addon-owned field list can't express without
duplicating the wizard's own step-definition model inside every addon
package. The field values are persisted into the addon's settings map
(`config.Addons["flux"].Settings`) and surfaced via `AddonConfig` to
`ValidateSettings` and `Install`.

## EnsureNamespace and other shared helpers

Most addons install into their own namespace (`flux-system`,
`cert-manager`, etc.). The `addon.EnsureNamespace(ctx, env, name)` helper
creates the namespace idempotently. Similar helpers live in
`internal/addon/helpers.go`:

- `addon.BuildOpaqueSecret(namespace, name, data)` — canonical k8s
  Opaque Secret manifest builder for addons
- `addon.EnsureNamespace(ctx, env, name)` — idempotent namespace
  creation with retries

New cross-addon helpers belong in `helpers.go`. Do not write per-addon
copies of namespace creation or secret construction logic.

## Why not Helm

Helm would let users install any chart, but:

- Most homelab users don't want to think about Helm values files
- Helm state lives in the cluster (as secrets), which makes "what's
  installed?" a k8s query rather than a config inspection
- Some addons need host-side work (e.g., adding a DNS record for cert
  challenges) that Helm can't do
- The addon pattern here gives us a typed, validated interface for
  addons that can reach both host *and* cluster

Individual addons may internally use `helm install` (via the executor)
to deploy their workloads — that's fine. What we're avoiding is making
*Helm itself* the addon mechanism.

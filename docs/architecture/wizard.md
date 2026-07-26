# The wizard

The wizard is the interactive TUI that walks the user through configuring
a cluster. It runs the first time `okdctl deploy` is invoked without
an existing `okdctl.yaml`, and can be re-run on demand to edit an
existing config.

## Data-driven, not code-driven

The wizard is built on a **data-driven** model: each step is declared as
a `StepDefinition` value with sections, fields, and validators. The
wizard runtime (`internal/tui/wizard/datadriven.go`) renders any
`StepDefinition` without needing custom code per step.

```go
type StepDefinition struct {
    ID           StepID
    Title        string
    DisplayTitle string
    Description  string
    Sections     []SectionDefinition

    Validate     func(values map[string]string) error
    Apply        func(step *DataDrivenStep, cfg *config.Config) error
    ShouldShow   func(*config.Config) bool
    ExtraContent func(values map[string]string, width int) string
}
```

The wizard package lives under `internal/tui/wizard/`:

- `datadriven.go` — the runtime that renders any `StepDefinition`
- `steps/` — the step definitions themselves, one file per step (welcome,
  distribution, proxmox, basics, node_placement, networking, resources,
  addons, files, advanced, review)

Each step's file declares a single `StepDefinition` literal plus its
helper validators. Adding a new step takes three additive edits: create
the file under `steps/`, add a `StepType` constant and a `DefaultConfig()`
entry in `internal/tui/wizard/config.go`, and register a factory in
`defaultStepRegistrations` in `internal/cli/wizard_setup.go`.

## FieldDefinition: the smallest unit

```go
type FieldDefinition struct {
    Key      string
    Label    string
    Default  string
    Help     string
    Type     FieldType   // see FieldType constants in datadriven.go
    Options  []string    // populated only when Type == FieldTypeSelect
    Required bool
    Validate func(string) error

    ConfigSet ConfigSetter  // how to push this field into *config.Config
    ConfigGet ConfigGetter  // how to read this field from *config.Config
}
```

`ConfigSet` / `ConfigGet` are the two-way bridge between the wizard's
in-memory map of `map[string]string` values and the typed `*config.Config`
struct. The `SetString`, `SetInt`, `SetBool`, `GetString`, `GetInt`
helpers in `datadriven.go` wrap common type conversions so field
definitions stay terse:

```go
{
    Key:      "cluster.name",
    Label:    "Cluster name",
    Required: true,
    Validate: ValidateClusterName,
    ConfigSet: wizard.SetString(func(c *config.Config, v string) {
        c.Cluster.Name = v
    }),
    ConfigGet: wizard.GetString(func(c *config.Config) string {
        return c.Cluster.Name
    }),
},
```

## Why data-driven

The payoff is consistency and testability. Every step gets the same
header, navigation, validation behavior, help text placement, and
keyboard shortcuts, because one runtime renders them all; hand-written
bubbletea models would drift apart step by step. The step definitions
are also pure data with pure validator functions, so you can unit-test
a validator or snapshot-test a step's rendered output without spinning
up a TUI.

## The escape hatch: ExtraContent and WithExtraContentFunc

Some steps need to render content that isn't a form field: a preview of
what will be created, a live Proxmox discovery result, a warning banner.
These steps use `ExtraContent` in the step definition or
`WithExtraContentFunc` on the step instance to render arbitrary
lipgloss output below the form.

This is the only point where a step can have custom rendering. If you
find yourself wanting more than `ExtraContent` allows, the right
answer is usually "your step is too complex — split it" rather than
"the wizard needs a new escape hatch."

## Validation lifecycle

1. **Per-field validation** runs on every keystroke (live feedback) and
   blocks advancing if any required field is empty or any validator
   returns an error
2. **Per-step validation** (`StepDefinition.Validate`) runs when the
   user presses enter to advance; it gets all the step's field values
   and can enforce cross-field invariants
3. **Apply** runs only after validation passes; it writes the values
   into `*config.Config`
4. **Whole-config validation** (`config.Config.Validate`) runs at the
   end of the wizard, before `deploy` actually starts; it's the same
   validator run by `loadConfig` in `internal/cli/helpers.go`

If the user hits escape, the state is not discarded. It stays in the
step's field values so they can come back and tweak.

The step flow is shown below. The steps with `ShouldShow` predicates
appear twice, once as the active step and once as a `(skipped)` bypass,
to make the condition explicit:

```mermaid
flowchart TD
    welcome --> distribution
    distribution --> proxmox
    proxmox --> basics
    basics -->|"provider == proxmox"| nodePlacement[node-placement]
    basics -->|"provider != proxmox"| networking
    nodePlacement --> networking
    networking --> resources
    resources --> addons
    addons -->|"distribution == okd"| filesStep[files]
    addons -->|"distribution != okd"| advanced
    filesStep --> advanced
    advanced --> review
    review --> E([complete])
```

## Why not huh, survey, or promptui

We looked at the Charmbracelet `huh` library and decided to skip it.
`huh` is great for one-shot forms but fights the multi-section, back-
and-forth, live-preview wizard structure we needed. The data-driven
model gives us what `huh` would have given us (consistent rendering,
reusable validators) plus per-step custom extra content, which `huh`
doesn't support.

The wizard is built directly on `bubbletea`, `bubbles`, and `lipgloss`.

# The wizard

The wizard is the interactive TUI that walks the user through configuring
a cluster. It runs the first time `openshitctl deploy` is invoked without
an existing `openshitctl.yaml`, and can be re-run on demand to edit an
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
- `steps/` — the step definitions themselves (basics, topology, networking,
  proxmox, addons, review)

Each step's file declares a single `StepDefinition` literal plus its
helper validators. Adding a new step is mostly additive: create the
file, register the step in the wizard assembly in
`internal/tui/wizard/wizard.go`, and you're done.

## FieldDefinition: the smallest unit

```go
type FieldDefinition struct {
    Key      string
    Label    string
    Default  string
    Help     string
    Type     FieldType   // Text, Password, Number, Bool, Select
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

## Why data-driven?

Two reasons.

**Consistency.** Every step looks the same: the same header, the same
navigation, the same validation behavior, the same help text placement,
the same keyboard shortcuts. If we hand-wrote each step as a bespoke
bubbletea model, wizard UX would drift between steps and bugs would
proliferate.

**Testability.** Step definitions are pure data. Their validators are
pure functions. You can unit-test a field's validator without spinning
up a TUI. You can snapshot-test the wizard's rendered output for a given
set of field values. Neither is practical if the steps are imperative
bubbletea code.

## The escape hatch: ExtraContent and WithExtraContentFunc

Some steps need to render content that isn't a form field — a preview of
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

If the user hits escape, state is **not** discarded — it's preserved
in the step's field values so they can come back and tweak.

## Why not huh / survey / promptui?

We looked at the Charmbracelet `huh` library and decided to skip it.
`huh` is great for one-shot forms but fights the multi-section, back-
and-forth, live-preview wizard structure we needed. The data-driven
model gives us what `huh` would have given us (consistent rendering,
reusable validators) plus the ability to render custom extra content
per step — which `huh` doesn't support.

The wizard is built directly on `bubbletea`, `bubbles`, and `lipgloss`.

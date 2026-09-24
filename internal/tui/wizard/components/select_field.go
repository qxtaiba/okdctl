package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// SelectField is a dropdown-style field that cycles options with left/right
// keys, rendered in the shared field box.
type SelectField struct {
	Label   string
	Help    string
	Note    string
	Options []string

	selected  int
	focused   bool
	width     int
	boxWidth  int
	isDefault bool
}

// NewSelectField builds a SelectField with the given label and option list.
func NewSelectField(label string, options []string) *SelectField {
	return &SelectField{
		Label:   label,
		Options: options,
	}
}

// Value returns the currently selected option, or "" when none.
func (f *SelectField) Value() string {
	if f.selected >= 0 && f.selected < len(f.Options) {
		return f.Options[f.selected]
	}
	return ""
}

// SetValue selects the first option equal to value and marks the field as
// user-modified. Unknown values are silently ignored.
func (f *SelectField) SetValue(value string) {
	for i, opt := range f.Options {
		if opt == value {
			f.selected = i
			f.isDefault = false
			return
		}
	}
}

// SetDefault sets the starting selection and marks the field as unchanged.
func (f *SelectField) SetDefault(value string) {
	f.isDefault = true
	for i, opt := range f.Options {
		if opt == value {
			f.selected = i
			return
		}
	}
}

// IsBoolean reports whether Options is exactly {"yes", "no"} in either order.
func (f *SelectField) IsBoolean() bool {
	if len(f.Options) != 2 {
		return false
	}
	a, b := f.Options[0], f.Options[1]
	return (a == "yes" && b == "no") || (a == "no" && b == "yes")
}

// Focus gives the field focus so arrow keys cycle options.
func (f *SelectField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

// Blur removes focus from the field.
func (f *SelectField) Blur() {
	f.focused = false
}

// SetWidth records the width available to the field's box, help, and note
// rows.
func (f *SelectField) SetWidth(width int) {
	f.width = width
}

// SetBoxWidth sets the field's explicit box width, overriding the width
// SelectField would otherwise compute from its options; the box still
// clamps to min(boxWidth, the width from SetWidth).
func (f *SelectField) SetBoxWidth(outer int) {
	f.boxWidth = outer
}

// Check always returns nil because selection is constrained to Options.
func (f *SelectField) Check() error {
	return nil
}

// Validate always returns nil because selection is constrained to Options.
func (f *SelectField) Validate() error {
	return f.Check()
}

// Update handles left/right and h/l key presses to cycle through Options,
// clearing the default tag only when the selected index actually moves to a
// different option — a single-option field's arrows are inert and keep it.
func (f *SelectField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused || len(f.Options) == 0 {
		return f, nil
	}

	if msg, ok := msg.(tea.KeyPressMsg); ok {
		prev := f.selected
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			f.selected--
			if f.selected < 0 {
				f.selected = len(f.Options) - 1
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			f.selected++
			if f.selected >= len(f.Options) {
				f.selected = 0
			}
		}
		if f.selected != prev {
			f.isDefault = false
		}
	}

	return f, nil
}

// View renders the field's label, a bordered box (a radio pair for boolean
// fields, cycle arrows otherwise), and — depending on state — a default
// tag beside the box, a help row (focused only), and a Note row.
func (f *SelectField) View() string {
	label := labelStyle.Render(f.Label)

	content := f.arrowContent()
	if f.IsBoolean() {
		content = f.booleanContent()
	}

	outer := min(f.nominalBoxWidth(), f.width)
	box := fieldBox(content, outer, f.focused, false)
	if f.isDefault {
		box = lipgloss.JoinHorizontal(lipgloss.Center, box, " "+tagStyle.Render("default"))
	}

	out := label + "\n" + box
	if f.focused && f.Help != "" {
		out += "\n" + helpStyle.Width(f.width).Render(f.Help)
	}
	if f.Note != "" {
		out += "\n" + f.Note
	}
	return out
}

// nominalBoxWidth returns the field's box width before clamping to the
// available width: an explicit SetBoxWidth value, else 16 for a boolean
// field, else the widest option plus room for arrows, padding, and border.
func (f *SelectField) nominalBoxWidth() int {
	if f.boxWidth > 0 {
		return f.boxWidth
	}
	if f.IsBoolean() {
		return 16
	}
	widest := 0
	for _, opt := range f.Options {
		widest = max(widest, lipgloss.Width(opt))
	}
	return max(widest+8, 12)
}

// arrowContent renders the current value (a dim "none" when it's blank, so
// the field never shows an empty gap between the arrows) flanked by cycle
// arrows, shown even while blurred; a lone option has nothing to cycle to,
// so it renders bare rather than implying an interaction that doesn't exist.
func (f *SelectField) arrowContent() string {
	if len(f.Options) < 2 {
		return f.Value()
	}
	arrow := lipgloss.NewStyle().Foreground(tui.ColorPrimary)
	value := f.Value()
	if value == "" {
		value = tagStyle.Render("none")
	}
	return arrow.Render("◂") + " " + value + " " + arrow.Render("▸")
}

// booleanContent renders both options as a radio pair, lighting the
// selected side in ColorPrimary and dimming the other to Slate500.
func (f *SelectField) booleanContent() string {
	active := lipgloss.NewStyle().Foreground(tui.ColorPrimary)
	inactive := lipgloss.NewStyle().Foreground(tui.ColorSlate500)

	parts := make([]string, len(f.Options))
	for i, opt := range f.Options {
		if i == f.selected {
			parts[i] = active.Render(tui.IconActive + " " + opt)
		} else {
			parts[i] = inactive.Render(tui.IconPending + " " + opt)
		}
	}
	return strings.Join(parts, "  ")
}

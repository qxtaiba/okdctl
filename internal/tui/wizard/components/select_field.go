package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// SelectField cycles options with left/right in the shared field box.
type SelectField struct {
	Label          string
	Help           string
	Note           string
	Options        []string
	displayOptions []string

	selected   int
	injected   int // index of the option SetValue injected for an off-list value, or -1
	focused    bool
	width      int
	boxWidth   int
	isDefault  bool
	hasDefault bool // set by SetDefault, never cleared — see boxOuterWidth
}

// NewSelectField builds a SelectField over options.
func NewSelectField(label string, options []string) *SelectField {
	return &SelectField{
		Label:    label,
		Options:  options,
		injected: -1,
	}
}

// Value returns the selected option, or "" when none.
func (f *SelectField) Value() string {
	if f.selected >= 0 && f.selected < len(f.Options) {
		return f.Options[f.selected]
	}
	return ""
}

// SetDisplayOptions annotates the options in the view without changing
// their values.
func (f *SelectField) SetDisplayOptions(options []string) {
	if len(options) != len(f.Options) {
		f.displayOptions = nil
		return
	}
	f.displayOptions = append(f.displayOptions[:0], options...)
}

// SetValue selects value, injecting a non-empty value outside Options as a
// "current" option so a loaded config round-trips instead of being coerced.
func (f *SelectField) SetValue(value string) {
	for i, opt := range f.Options {
		if opt == value {
			f.selected = i
			f.isDefault = false
			return
		}
	}
	if value == "" {
		return
	}
	if f.injected < 0 {
		f.Options = append(f.Options, "")
		f.injected = len(f.Options) - 1
	}
	f.Options[f.injected] = value
	f.selected = f.injected
	f.isDefault = false
}

// SetDefault selects value and tags it default when Options offer it;
// hasDefault never clears, so the box keeps the tag's room.
func (f *SelectField) SetDefault(value string) {
	f.hasDefault = true
	for i, opt := range f.Options {
		if opt == value {
			f.selected = i
			f.isDefault = true
			return
		}
	}
}

// IsDefault reports whether the selection is still the untouched default.
func (f *SelectField) IsDefault() bool {
	return f.isDefault
}

// HasDefaultTag reports whether the field has ever carried a default.
func (f *SelectField) HasDefaultTag() bool {
	return f.hasDefault
}

// IsBoolean reports whether Options is exactly yes/no in either order.
func (f *SelectField) IsBoolean() bool {
	if len(f.Options) != 2 {
		return false
	}
	a, b := f.Options[0], f.Options[1]
	return (a == "yes" && b == "no") || (a == "no" && b == "yes")
}

// Focus gives the field focus.
func (f *SelectField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

// Blur removes focus.
func (f *SelectField) Blur() {
	f.focused = false
}

// SetWidth records the available width.
func (f *SelectField) SetWidth(width int) {
	f.width = width
}

// SetBoxWidth overrides the width computed from the options; the box still
// clamps to the SetWidth width.
func (f *SelectField) SetBoxWidth(outer int) {
	f.boxWidth = outer
}

// Check always returns nil; selection is constrained to Options.
func (f *SelectField) Check() error {
	return nil
}

// Validate always returns nil; selection is constrained to Options.
func (f *SelectField) Validate() error {
	return f.Check()
}

// Update cycles Options on left/right and h/l, dropping the default tag
// only when the selection moves.
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

// View renders the label, the box (a radio pair for booleans) and the
// default, help and note rows.
func (f *SelectField) View() string {
	label := labelStyle.Render(f.Label)

	content := f.arrowContent()
	if f.IsBoolean() {
		content = f.booleanContent()
	}

	box := fieldBox(content, f.boxOuterWidth(), f.focused, false, false)
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

// nominalBoxWidth is the explicit SetBoxWidth, else 18 for a boolean, else
// the widest option plus chrome.
func (f *SelectField) nominalBoxWidth() int {
	if f.boxWidth > 0 {
		return f.boxWidth
	}
	if f.IsBoolean() {
		return 18
	}
	widest := 0
	for i, opt := range f.Options {
		if len(f.displayOptions) == len(f.Options) {
			opt = f.displayOptions[i]
		}
		w := lipgloss.Width(opt)
		if i == f.injected {
			w += lipgloss.Width(" current")
		}
		widest = max(widest, w)
	}
	return max(widest+8, 14)
}

// boxOuterWidth reserves defaultTagReserve once the field has ever carried
// a default, so the box never resizes when the tag drops.
func (f *SelectField) boxOuterWidth() int {
	avail := f.width
	if f.hasDefault {
		avail -= defaultTagReserve
	}
	return min(f.nominalBoxWidth(), max(avail, 0))
}

// arrowContent renders the value between cycle arrows, bare for a lone
// option and tagged "current" for an injected off-list value.
func (f *SelectField) arrowContent() string {
	value := f.Value()
	if f.selected >= 0 && f.selected < len(f.displayOptions) && len(f.displayOptions) == len(f.Options) {
		value = f.displayOptions[f.selected]
	}
	if f.selected == f.injected && f.injected >= 0 {
		value += " " + tagStyle.Render("current")
	}
	if len(f.Options) < 2 {
		return value
	}
	arrow := lipgloss.NewStyle().Foreground(tui.ColorPrimary())
	if value == "" {
		value = tagStyle.Render("none")
	}
	return arrow.Render(tui.IconCaretLeft) + " " + value + " " + arrow.Render(tui.IconCaretRight)
}

func (f *SelectField) booleanContent() string {
	active := lipgloss.NewStyle().Foreground(tui.ColorPrimary())
	inactive := lipgloss.NewStyle().Foreground(tui.ColorTextFaint())

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

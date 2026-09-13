// Package components provides reusable bubbletea widgets (text inputs,
// selectors, dropdowns) used by wizard steps to collect user configuration.
package components

import (
	"errors"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// FormField is the interface all form field types must implement to be
// usable in an InputGroup.
type FormField interface {
	Value() string
	SetValue(value string)
	Focus() tea.Cmd
	Blur()
	SetWidth(width int)
	Validate() error
	Update(msg tea.Msg) (FormField, tea.Cmd)
	View() string
}

// InputField is a single text input FormField. Password fields mask input
// in View and scrub the raw value out of validator error messages.
type InputField struct {
	Label       string
	Placeholder string
	Help        string
	Note        string
	Required    bool
	Password    bool
	Validator   func(string) error

	input     textinput.Model
	focused   bool
	width     int
	boxWidth  int
	isDefault bool
	savedPos  int
	err       error
}

// NewInputField builds a plain-text InputField from label and placeholder.
func NewInputField(label, placeholder string) *InputField {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = placeholder
	ti.CharLimit = 256
	ti.SetStyles(fieldInputStyles())

	return &InputField{
		Label:       label,
		Placeholder: placeholder,
		input:       ti,
		boxWidth:    32,
	}
}

// fieldInputStyles builds the textinput color scheme shared by every
// InputField: dim placeholder, brand-purple cursor, slate blurred text.
func fieldInputStyles() textinput.Styles {
	return textinput.Styles{
		Focused: textinput.StyleState{
			Text:        lipgloss.NewStyle().Foreground(tui.ColorText),
			Placeholder: lipgloss.NewStyle().Foreground(tui.ColorSlate600),
		},
		Blurred: textinput.StyleState{
			Text:        lipgloss.NewStyle().Foreground(tui.ColorSlate300),
			Placeholder: lipgloss.NewStyle().Foreground(tui.ColorSlate600),
		},
		Cursor: textinput.CursorStyle{
			Color: tui.ColorPrimary,
			Shape: tea.CursorBlock,
			Blink: true,
		},
	}
}

// NewPasswordField builds an InputField that masks input with echo chars and
// scrubs the raw value from validator error messages.
func NewPasswordField(label, placeholder string) *InputField {
	f := NewInputField(label, placeholder)
	f.Password = true
	f.input.EchoMode = textinput.EchoPassword
	f.input.EchoCharacter = '•'
	return f
}

// Value returns the current text of the field.
func (f *InputField) Value() string {
	return f.input.Value()
}

// SetValue replaces the field value.
func (f *InputField) SetValue(value string) {
	f.input.SetValue(value)
}

// Focus gives the field focus, restores the cursor to where Blur last left
// it, and returns the textinput blink command.
func (f *InputField) Focus() tea.Cmd {
	f.focused = true
	f.input.SetCursor(f.savedPos)
	return f.input.Focus()
}

// Blur removes focus, scrolls a long value back to its head so it reads
// from the start while unfocused, and runs one validation pass so error
// state is current when the field is rendered next.
func (f *InputField) Blur() {
	f.focused = false
	f.savedPos = f.input.Position()
	f.input.SetCursor(0)
	f.input.Blur()
	_ = f.Validate()
}

// SetWidth records the width available to the field's label and help/error
// rows, then reapplies the box so the textinput's scroll window matches.
func (f *InputField) SetWidth(width int) {
	f.width = width
	f.applyBox()
}

// SetBoxWidth sets the field's nominal box width; the box actually renders
// at min(outer, the width from SetWidth), so a narrow terminal still clamps
// it.
func (f *InputField) SetBoxWidth(outer int) {
	f.boxWidth = outer
	f.applyBox()
}

// SetPlaceholder replaces the dim hint text shown inside an empty box.
func (f *InputField) SetPlaceholder(p string) {
	f.Placeholder = p
	f.input.Placeholder = p
}

// applyBox resizes the textinput to the current box's inner width (border 2
// + padding 2 + cursor cell 1) and recomputes its scroll window — bubbles'
// SetWidth alone leaves a stale window after a resize.
func (f *InputField) applyBox() {
	w := min(f.boxWidth, f.width)
	f.input.SetWidth(w - 5)
	f.input.SetCursor(f.input.Position())
}

// Validate runs the Required check and Validator; for password fields the
// raw value is scrubbed from error messages so secrets can't leak.
func (f *InputField) Validate() error {
	if f.Required && strings.TrimSpace(f.input.Value()) == "" {
		f.err = errRequired
		return f.err
	}
	if f.Validator != nil {
		value := f.input.Value()
		f.err = f.Validator(value)
		// Wraps a validator's error for password fields so its message can't
		// leak the raw value, while preserving Unwrap().
		if f.Password && f.err != nil && value != "" {
			var msg string
			// Short values (e.g. "a") would mangle unrelated chars via
			// ReplaceAll; fall back to a generic message instead.
			if len(value) >= 4 {
				msg = strings.ReplaceAll(f.err.Error(), value, "***")
			} else {
				msg = "invalid password"
			}
			f.err = &scrubbedError{msg: msg, inner: f.err}
		}
		return f.err
	}
	f.err = nil
	return nil
}

// Update forwards msg to the underlying textinput, clearing any stale
// validation error on keypress.
func (f *InputField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused {
		return f, nil
	}

	if _, ok := msg.(tea.KeyPressMsg); ok {
		f.err = nil
	}

	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)

	return f, cmd
}

// View renders the label, the boxed input (masked and error-scrubbed for
// password fields), and — depending on state — an error row, a help row
// (focused only), and a Note row.
func (f *InputField) View() string {
	// Never render f.input.Value() directly when Password is true; rely on
	// EchoMode, and scrub raw value on every text path below.
	label := labelStyle.Render(f.Label)

	content := f.input.View()
	if f.input.Value() == "" && f.Placeholder == "" {
		content = tagStyle.Render("·")
	}

	box := fieldBox(content, min(f.boxWidth, f.width), f.focused, f.err != nil)
	if f.isDefault {
		box = lipgloss.JoinHorizontal(lipgloss.Center, box, " "+tagStyle.Render("default"))
	}

	out := label + "\n" + box
	switch {
	case f.err != nil:
		out += "\n" + errStyle.Width(f.width).Render(tui.IconError+" "+f.scrubbed(f.err.Error()))
	case f.focused && f.Help != "":
		out += "\n" + helpStyle.Width(f.width).Render(f.Help)
	}
	if f.Note != "" {
		out += "\n" + f.Note
	}
	return out
}

// scrubbed redacts the raw value out of msg for password fields so an
// interpolating validator error can't leak it.
func (f *InputField) scrubbed(msg string) string {
	if f.Password {
		if v := f.input.Value(); v != "" {
			msg = strings.ReplaceAll(msg, v, "<redacted>")
		}
	}
	return msg
}

var errRequired = errors.New("this field is required")

// scrubbedError wraps a validator error, rewriting the visible message while keeping Unwrap().
type scrubbedError struct {
	msg   string
	inner error
}

func (e *scrubbedError) Error() string { return e.msg }
func (e *scrubbedError) Unwrap() error { return e.inner }

// InputGroup is an ordered collection of FormFields sharing one focus
// cursor; handles tab/shift-tab traversal and aggregate validation.
type InputGroup struct {
	fields []FormField

	focusIndex int
	focused    bool
	width      int
}

// NewInputGroup returns a group containing the given fields.
func NewInputGroup(fields ...FormField) *InputGroup {
	return &InputGroup{
		fields:     fields,
		focusIndex: 0,
	}
}

// Fields returns the group's fields in insertion order.
func (g *InputGroup) Fields() []FormField {
	return g.fields
}

// Field returns the field at index, or nil when out of range.
func (g *InputGroup) Field(index int) FormField {
	if index >= 0 && index < len(g.fields) {
		return g.fields[index]
	}
	return nil
}

// FocusIndex returns the index of the currently focused field.
func (g *InputGroup) FocusIndex() int {
	return g.focusIndex
}

// SetFocusIndex moves focus to index, ignoring out-of-range values.
func (g *InputGroup) SetFocusIndex(index int) {
	if index >= 0 && index < len(g.fields) {
		g.focusIndex = index
		g.updateFocus()
	}
}

// Focus focuses the group and the currently selected field.
func (g *InputGroup) Focus() tea.Cmd {
	g.focused = true
	return g.updateFocus()
}

// Blur blurs the group and every contained field.
func (g *InputGroup) Blur() {
	g.focused = false
	for _, f := range g.fields {
		f.Blur()
	}
}

// SetWidth resizes the group and propagates the width to each field.
func (g *InputGroup) SetWidth(width int) {
	g.width = width
	for _, f := range g.fields {
		f.SetWidth(width)
	}
}

func (g *InputGroup) updateFocus() tea.Cmd {
	var cmd tea.Cmd
	for i, f := range g.fields {
		if i == g.focusIndex && g.focused {
			cmd = f.Focus()
		} else {
			f.Blur()
		}
	}
	return cmd
}

// Next moves focus to the next field, wrapping to the first.
func (g *InputGroup) Next() tea.Cmd {
	g.focusIndex++
	if g.focusIndex >= len(g.fields) {
		g.focusIndex = 0
	}
	return g.updateFocus()
}

// Previous moves focus to the previous field, wrapping to the last.
func (g *InputGroup) Previous() tea.Cmd {
	g.focusIndex--
	if g.focusIndex < 0 {
		g.focusIndex = len(g.fields) - 1
	}
	return g.updateFocus()
}

// Validate returns the collected errors from each field's Validate.
func (g *InputGroup) Validate() []error {
	var errs []error
	for _, f := range g.fields {
		if err := f.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// Update handles group-level navigation keys (tab, shift-tab) and forwards
// everything else to the focused field.
func (g *InputGroup) Update(msg tea.Msg) (*InputGroup, tea.Cmd) {
	if !g.focused || len(g.fields) == 0 {
		return g, nil
	}

	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("tab", "down"))):
			cmd := g.Next()
			return g, cmd
		case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab", "up"))):
			cmd := g.Previous()
			return g, cmd
		}
	}

	var cmd tea.Cmd
	g.fields[g.focusIndex], cmd = g.fields[g.focusIndex].Update(msg)
	return g, cmd
}

// FieldViews renders each field in order; View is exactly these joined by a
// blank row, so callers may index into the group's rendering field by field.
func (g *InputGroup) FieldViews() []string {
	views := make([]string, len(g.fields))
	for i, f := range g.fields {
		views[i] = f.View()
	}
	return views
}

// View renders the group's fields separated by blank lines.
func (g *InputGroup) View() string {
	return strings.Join(g.FieldViews(), "\n\n")
}

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
	Required    bool
	Password    bool
	Validator   func(string) error

	input           textinput.Model
	focused         bool
	width           int
	err             error
	validated       bool
	validationError error
}

// NewInputField builds a plain-text InputField from label and placeholder.
func NewInputField(label, placeholder string) *InputField {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 256
	ti.SetWidth(40)

	return &InputField{
		Label:       label,
		Placeholder: placeholder,
		input:       ti,
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
	f.validated = false
}

// Focus gives the field focus and returns the textinput blink command.
func (f *InputField) Focus() tea.Cmd {
	f.focused = true
	return f.input.Focus()
}

// Blur removes focus and runs one validation pass so error state is current
// when the field is rendered next.
func (f *InputField) Blur() {
	wasFocused := f.focused
	f.focused = false
	f.input.Blur()
	if wasFocused {
		_ = f.Validate()
	}
}

// SetWidth resizes the input field, reserving border and padding space.
func (f *InputField) SetWidth(width int) {
	f.width = width
	inputWidth := max(width-4, 20) // border (2) + padding (2)
	f.input.SetWidth(inputWidth)
}

// Validate runs the Required check and Validator; for password fields the
// raw value is scrubbed from error messages so secrets can't leak.
func (f *InputField) Validate() error { f.err = f.validateValue(); return f.err }

// Valid reports cached validity without revealing untouched field errors.
func (f *InputField) Valid() bool { return f.validateValue() == nil }

func (f *InputField) validateValue() error {
	if f.validated {
		return f.validationError
	}
	f.validated = true
	if !f.Required && f.input.Value() == "" {
		f.validationError = nil
		return nil
	}
	if f.Required && strings.TrimSpace(f.input.Value()) == "" {
		f.validationError = errRequired
		return f.validationError
	}
	if f.Validator != nil {
		value := f.input.Value()
		f.validationError = f.Validator(value)
		// Wraps a validator's error for password fields so its message can't
		// leak the raw value, while preserving Unwrap().
		if f.Password && f.validationError != nil && value != "" {
			var msg string
			// Short values (e.g. "a") would mangle unrelated chars via
			// ReplaceAll; fall back to a generic message instead.
			if len(value) >= 4 {
				msg = strings.ReplaceAll(f.validationError.Error(), value, "***")
			} else {
				msg = "invalid password"
			}
			f.validationError = &scrubbedError{msg: msg, inner: f.validationError}
		}
		return f.validationError
	}
	f.validationError = nil
	return nil
}

// Update invalidates cached validation when the input value changes.
func (f *InputField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused {
		return f, nil
	}

	before := f.input.Value()
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	if f.input.Value() != before {
		f.err = nil
		f.validated = false
	}

	return f, cmd
}

// View renders the field: label, input box, and any validation error.
// Password fields mask the value and scrub it from error text.
func (f *InputField) View() string {
	// Never render f.input.Value() directly when Password is true; rely on
	// EchoMode, and scrub raw value on every text path below.
	labelStyle := lipgloss.NewStyle().
		Foreground(tui.ColorText)

	hintStyle := lipgloss.NewStyle().
		Foreground(tui.ColorTextDim)

	labelText := strings.ToLower(f.Label)
	if f.focused {
		labelText = "> " + labelText
	}
	labelLine := labelStyle.Render(labelText)
	if f.focused && f.Help != "" {
		labelLine += " " + hintStyle.Render("("+f.Help+")")
	}

	contentWidth := f.input.Width()
	if contentWidth < 20 {
		contentWidth = 40
	}

	borderColor := tui.ColorBorder
	switch {
	case f.focused:
		borderColor = tui.ColorPrimary
	case f.err != nil:
		borderColor = tui.ColorError
	}
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(contentWidth)

	input := inputStyle.Render(f.input.View())

	result := labelLine + "\n" + input
	if !f.focused {
		compact := f.input
		compact.Prompt = ""
		compact.SetWidth(max(1, f.width-lipgloss.Width(labelLine)-2))
		result = labelLine + ": " + compact.View()
	}

	if f.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(tui.ColorError)
		errText := f.err.Error()
		// Scrub the raw value from password-field error messages so an
		// interpolating validator can't leak it.
		if f.Password {
			if v := f.input.Value(); v != "" {
				errText = strings.ReplaceAll(errText, v, "<redacted>")
			}
		}
		result += "\n" + errStyle.Render(tui.IconError+" "+errText)
	}

	return result
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

// View renders the group's fields separated by blank lines.
func (g *InputGroup) View() string {
	var lines []string

	for i, f := range g.fields {
		lines = append(lines, g.renderField(f))
		if i < len(g.fields)-1 {
			lines = append(lines, "")
		}
	}

	return strings.Join(lines, "\n")
}

func (g *InputGroup) renderField(f FormField) string {
	if g.width <= 0 {
		return f.View()
	}
	return lipgloss.NewStyle().MaxWidth(g.width).Render(f.View())
}

// FocusBounds returns the focused control's rendered line interval.
func (g *InputGroup) FocusBounds() (top, bottom int) {
	for i, field := range g.fields {
		height := lipgloss.Height(g.renderField(field))
		if i == g.focusIndex {
			return top, top + height
		}
		top += height + 1
	}
	return top, top
}

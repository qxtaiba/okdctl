// Package components provides the form widgets wizard steps collect input
// with.
package components

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// errPasteRejected never echoes the rejected content, which may be a secret.
var errPasteRejected = errors.New("paste rejected — contains a newline, carriage return, or control character; paste a single-line value")

// pasteCorrupting reports a newline, carriage return or C0 byte that
// textinput would silently collapse or drop.
func pasteCorrupting(content string) bool {
	for _, r := range content {
		if r == '\n' || r == '\r' || r < 0x20 {
			return true
		}
	}
	return false
}

// FormField is the contract every InputGroup member satisfies.
type FormField interface {
	Value() string
	SetValue(value string)
	Focus() tea.Cmd
	Blur()
	SetWidth(width int)
	Check() error    // pure: reports validity without recording it for View
	Validate() error // Check, then records the result for View to render
	Update(msg tea.Msg) (FormField, tea.Cmd)
	View() string
}

// TextInputField is implemented by fields that take a raw keystroke as typed
// text while focused, so "?" types instead of opening the help overlay.
type TextInputField interface {
	ConsumesTextInput() bool
}

// EnterConsumer is implemented by fields that use enter inside their own
// flow; the form routes enter to them instead of submitting.
type EnterConsumer interface {
	ConsumesEnter() bool
}

// FieldErrorSetter is implemented by fields that render an externally
// supplied error inline.
type FieldErrorSetter interface {
	SetError(err error)
}

// InputField is a single text input. Password fields mask input and scrub
// the raw value out of validator errors.
type InputField struct {
	Label       string
	Placeholder string
	Help        string
	Note        string
	Required    bool
	Password    bool
	Validator   func(string) error

	// Disabled dims the field to signal its value has no effect right now; it
	// never blocks editing
	Disabled bool

	input        textinput.Model
	focused      bool
	width        int
	boxWidth     int
	isDefault    bool
	hasDefault   bool // set by SetDefault, never cleared — see boxOuterWidth
	touched      bool
	savedPos     int
	err          error
	focusValue   string
	focusDefault bool

	// validCache backs Valid() until the value changes
	validCache bool
	validated  bool
}

// NewInputField builds a plain-text InputField.
func NewInputField(label, placeholder string) *InputField {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = placeholder
	ti.CharLimit = 256
	ti.SetStyles(fieldInputStyles(false, false))

	return &InputField{
		Label:       label,
		Placeholder: placeholder,
		input:       ti,
		boxWidth:    40,
	}
}

// fieldInputStyles dims an unmodified default to the faint tier and
// italicises the placeholder so an empty box never reads as filled; under
// NO_COLOR the " default" tag is the only disambiguator.
func fieldInputStyles(isDefault, disabled bool) textinput.Styles {
	focusedText := lipgloss.NewStyle().Foreground(tui.ColorText())
	blurredText := lipgloss.NewStyle().Foreground(tui.ColorTextSoft())
	switch {
	case disabled:
		focusedText = lipgloss.NewStyle().Foreground(tui.ColorSubtle())
		blurredText = lipgloss.NewStyle().Foreground(tui.ColorSubtle())
	case isDefault:
		focusedText = lipgloss.NewStyle().Foreground(tui.ColorTextFaint())
		blurredText = lipgloss.NewStyle().Foreground(tui.ColorTextFaint())
	}
	placeholder := lipgloss.NewStyle().Foreground(tui.ColorSubtle()).Italic(true)
	return textinput.Styles{
		Focused: textinput.StyleState{
			Text:        focusedText,
			Placeholder: placeholder,
		},
		Blurred: textinput.StyleState{
			Text:        blurredText,
			Placeholder: placeholder,
		},
		// Blink is off: textinput refreshes the cursor cell's color only on the
		// placeholder path, so a blinking cursor renders one character unstyled.
		Cursor: textinput.CursorStyle{
			Color: tui.ColorPrimary(),
			Shape: tea.CursorBlock,
			Blink: false,
		},
	}
}

// NewPasswordField builds an InputField that masks input and scrubs the raw
// value from validator errors.
func NewPasswordField(label, placeholder string) *InputField {
	f := NewInputField(label, placeholder)
	f.Password = true
	f.input.EchoMode = textinput.EchoPassword
	f.input.EchoCharacter = '•'
	return f
}

// Value returns the current text.
func (f *InputField) Value() string {
	return f.input.Value()
}

// ConsumesTextInput reports true while focused.
func (f *InputField) ConsumesTextInput() bool {
	return f.focused
}

// SetValue replaces the value and clears the default tag.
func (f *InputField) SetValue(value string) {
	f.input.SetValue(value)
	f.isDefault = false
	f.validated = false
}

// SetDefault sets v as the unmodified default View renders dim with a tag
// until it changes; hasDefault never clears, so the box keeps the tag's room.
func (f *InputField) SetDefault(v string) {
	f.input.SetValue(v)
	f.isDefault = true
	f.hasDefault = true
}

// IsDefault reports whether the value is still its unmodified default.
func (f *InputField) IsDefault() bool {
	return f.isDefault
}

// HasDefaultTag reports whether the field has ever carried a default, which
// pairing uses to reserve the tag's room symmetrically.
func (f *InputField) HasDefaultTag() bool {
	return f.hasDefault
}

// Focus marks the field touched, restores the cursor Blur saved and returns
// the textinput blink command.
func (f *InputField) Focus() tea.Cmd {
	f.focused = true
	f.touched = true
	f.focusValue = f.input.Value()
	f.focusDefault = f.isDefault
	f.input.SetCursor(f.savedPos)
	return f.input.Focus()
}

// Blur scrolls the value back to its head and validates a touched field so
// its error state is current on the next render; the cursor save is guarded
// on a real focus->blur transition because InputGroup re-blurs blurred
// fields.
func (f *InputField) Blur() {
	if f.focused {
		f.savedPos = f.input.Position()
	}
	f.focused = false
	f.input.SetCursor(0)
	f.input.Blur()
	if f.touched {
		_ = f.Validate()
	}
}

// SetWidth records the available width and reapplies the box.
func (f *InputField) SetWidth(width int) {
	f.width = width
	f.applyBox()
}

// SetBoxWidth sets the nominal box width; the box renders at min(outer, the
// SetWidth width).
func (f *InputField) SetBoxWidth(outer int) {
	f.boxWidth = outer
	f.applyBox()
}

// SetPlaceholder replaces the hint shown inside an empty box.
func (f *InputField) SetPlaceholder(p string) {
	f.Placeholder = p
	f.input.Placeholder = p
}

// applyBox resizes the textinput and recomputes its scroll window, which
// bubbles' SetWidth alone leaves stale.
func (f *InputField) applyBox() {
	w := f.boxOuterWidth()
	f.input.SetWidth(w - 5)
	f.input.SetCursor(f.input.Position())
}

// boxOuterWidth reserves defaultTagReserve once the field has ever carried a
// default, so the box never resizes under the cursor when the tag drops.
func (f *InputField) boxOuterWidth() int {
	avail := f.width
	if f.hasDefault {
		avail -= defaultTagReserve
	}
	return min(f.boxWidth, max(avail, 0))
}

// Check reports validity without recording it, scrubbing a password's raw
// value from the message.
func (f *InputField) Check() error {
	if f.Required && strings.TrimSpace(f.input.Value()) == "" {
		label := f.Label
		if label == "" {
			label = "this field"
		}
		return fmt.Errorf("%s is required — enter a value", label)
	}
	if f.Validator == nil {
		return nil
	}
	value := f.input.Value()
	err := f.Validator(value)
	if f.Password && err != nil && value != "" {
		var msg string
		// ReplaceAll on a short value would mangle unrelated characters.
		if len(value) >= 4 {
			msg = strings.ReplaceAll(err.Error(), value, "***")
		} else {
			msg = "invalid password"
		}
		err = &scrubbedError{msg: msg, inner: err}
	}
	return err
}

// Validate runs Check, records the result for View and marks the field
// touched.
func (f *InputField) Validate() error {
	f.touched = true
	f.err = f.Check()
	return f.err
}

// Valid reports whether the value passes Check without touching the field,
// cached until the value changes.
func (f *InputField) Valid() bool {
	if !f.validated {
		f.validCache = f.Check() == nil
		f.validated = true
	}
	return f.validCache
}

// SetError marks the field invalid with err until the next edit.
func (f *InputField) SetError(err error) {
	f.err = err
	f.touched = true
}

// Update forwards msg to the textinput; the first genuine edit over an
// unmodified default replaces the text instead of appending to it.
func (f *InputField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused {
		return f, nil
	}

	// Only a value change invalidates the Valid() cache.
	before := f.input.Value()
	defer func() {
		if f.input.Value() != before {
			f.validated = false
		}
	}()

	switch k := msg.(type) {
	case tea.KeyPressMsg:
		if key.Matches(k, key.NewBinding(key.WithKeys("ctrl+z"))) {
			f.input.SetValue(f.focusValue)
			f.isDefault = f.focusDefault
			f.err = nil
			return f, nil
		}
		f.err = nil
		if f.isDefault && (k.Text != "" || k.Code == tea.KeyBackspace || k.Code == tea.KeyDelete) {
			f.isDefault = false
			f.input.SetValue("")
		}
	case tea.PasteMsg:
		if pasteCorrupting(k.Content) {
			f.err = errPasteRejected
			return f, nil
		}
		f.err = nil
		if f.isDefault {
			f.isDefault = false
			f.input.SetValue("")
		}
	}

	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)

	return f, cmd
}

// View renders the label, the boxed input and the error, help and note rows.
func (f *InputField) View() string {
	// Never render f.input.Value() directly for a password field.
	label := labelStyle.Render(f.Label)
	if f.Disabled {
		label = helpStyle.Render(f.Label)
	}

	f.input.SetStyles(fieldInputStyles(f.isDefault, f.Disabled))
	content := f.input.View()
	if !f.focused {
		switch {
		case f.input.Value() != "":
			content = f.blurredValueView()
		case f.Placeholder == "":
			content = tagStyle.Render("·")
		}
	}

	box := fieldBox(content, f.boxOuterWidth(), f.focused, f.err != nil, f.Disabled)
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

// blurredValueView bypasses the textinput's cursor path: the blurred cursor
// cell renders through a TextStyle textinput never sets, so one character
// would read bright.
func (f *InputField) blurredValueView() string {
	v := f.input.Value()
	if f.Password {
		v = strings.Repeat(string(f.input.EchoCharacter), lipgloss.Width(v))
	}
	style := fieldInputStyles(f.isDefault, f.Disabled).Blurred.Text
	return style.Render(tui.Truncate(v, max(f.boxOuterWidth()-4, 1)))
}

// scrubbed redacts the raw value out of msg for password fields.
func (f *InputField) scrubbed(msg string) string {
	if f.Password {
		if v := f.input.Value(); v != "" {
			msg = strings.ReplaceAll(msg, v, "<redacted>")
		}
	}
	return msg
}

// scrubbedError rewrites the visible message while keeping Unwrap().
type scrubbedError struct {
	msg   string
	inner error
}

func (e *scrubbedError) Error() string { return e.msg }
func (e *scrubbedError) Unwrap() error { return e.inner }

// InputGroup is an ordered set of FormFields sharing one focus cursor.
type InputGroup struct {
	fields []FormField

	focusIndex int
	focused    bool
	width      int
}

// NewInputGroup returns a group containing fields.
func NewInputGroup(fields ...FormField) *InputGroup {
	return &InputGroup{
		fields:     fields,
		focusIndex: 0,
	}
}

// Fields returns the group's fields in order.
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

// FocusIndex returns the focused field's index.
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

// Focus focuses the group and its selected field.
func (g *InputGroup) Focus() tea.Cmd {
	g.focused = true
	return g.updateFocus()
}

// Blur blurs the group and every field.
func (g *InputGroup) Blur() {
	g.focused = false
	for _, f := range g.fields {
		f.Blur()
	}
}

// SetWidth propagates width to each field.
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

// Next moves focus to the next field, wrapping.
func (g *InputGroup) Next() tea.Cmd {
	g.focusIndex++
	if g.focusIndex >= len(g.fields) {
		g.focusIndex = 0
	}
	return g.updateFocus()
}

// Previous moves focus to the previous field, wrapping.
func (g *InputGroup) Previous() tea.Cmd {
	g.focusIndex--
	if g.focusIndex < 0 {
		g.focusIndex = len(g.fields) - 1
	}
	return g.updateFocus()
}

// Validate returns every field's Validate error.
func (g *InputGroup) Validate() []error {
	var errs []error
	for _, f := range g.fields {
		if err := f.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// TouchAll validates every field so a forced submission paints each one's
// real state.
func (g *InputGroup) TouchAll() {
	for _, f := range g.fields {
		_ = f.Validate()
	}
}

// Update handles tab navigation and forwards the rest to the focused field.
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

// FieldViews renders each field in order; View is these joined by a blank
// row.
func (g *InputGroup) FieldViews() []string {
	views := make([]string, len(g.fields))
	for i, f := range g.fields {
		views[i] = f.View()
	}
	return views
}

// View renders the fields separated by blank lines.
func (g *InputGroup) View() string {
	return strings.Join(g.FieldViews(), "\n\n")
}

// Package components provides reusable bubbletea widgets (text inputs,
// selectors, dropdowns) used by wizard steps to collect user configuration.
package components

import (
	"fmt"
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
	Check() error    // pure: reports validity without recording it for View
	Validate() error // Check, then records the result for View to render
	Update(msg tea.Msg) (FormField, tea.Cmd)
	View() string
}

// TextInputField is implemented by form fields that consume a raw
// keystroke as literal typed text while focused, rather than interpreting
// it as a navigation or toggle command — the wizard uses it to decide
// whether "?" opens the help overlay or types the character.
type TextInputField interface {
	ConsumesTextInput() bool
}

// LabeledField is implemented by every concrete FormField, letting a caller
// describe the currently focused field without a type switch over each
// field kind — the wide-terminal context pane's focused-field echo uses it.
type LabeledField interface {
	FieldLabel() string
	FieldHelp() string
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

	// Disabled dims the label, box, and value to signal the field's current
	// value has no effect right now (e.g. a drain timeout while skip-drain
	// is selected) — a Tesler-law affordance so the operator doesn't have to
	// submit the form to learn an edit was ignored. It does not block focus
	// or editing: the value still round-trips normally if the condition
	// that disabled it changes back.
	Disabled bool

	input      textinput.Model
	focused    bool
	width      int
	boxWidth   int
	isDefault  bool
	hasDefault bool // set by SetDefault, never cleared — see boxOuterWidth
	touched    bool
	savedPos   int
	err        error
}

// NewInputField builds a plain-text InputField from label and placeholder.
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

// fieldInputStyles builds the textinput color scheme shared by every
// InputField: brand-purple cursor, dim italic placeholder, and — while
// isDefault — Slate500 text so an unmodified default reads as dimmer than a
// typed value. Placeholder is italicized on top of its own dimmer Slate600
// so an empty box's hint text never reads as an already-filled default at a
// glance (NO_COLOR strips both the color and the italic, leaving the
// " default" tag as the only disambiguator there — see SetDefault). disabled
// wins over isDefault, dimming further to Slate600 — see InputField.Disabled.
func fieldInputStyles(isDefault, disabled bool) textinput.Styles {
	focusedText := lipgloss.NewStyle().Foreground(tui.ColorText)
	blurredText := lipgloss.NewStyle().Foreground(tui.ColorSlate300)
	switch {
	case disabled:
		focusedText = lipgloss.NewStyle().Foreground(tui.ColorSlate600)
		blurredText = lipgloss.NewStyle().Foreground(tui.ColorSlate600)
	case isDefault:
		focusedText = lipgloss.NewStyle().Foreground(tui.ColorSlate500)
		blurredText = lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	}
	placeholder := lipgloss.NewStyle().Foreground(tui.ColorSlate600).Italic(true)
	return textinput.Styles{
		Focused: textinput.StyleState{
			Text:        focusedText,
			Placeholder: placeholder,
		},
		Blurred: textinput.StyleState{
			Text:        blurredText,
			Placeholder: placeholder,
		},
		// Blink is off: bubbles' textinput only refreshes its cursor-cell
		// glyph color on the placeholder/suggestion paths, so a blinking
		// cursor over real (typed or default) text intermittently renders
		// that one character unstyled instead of matching its neighbors —
		// a static reverse-video block sidesteps the glitch entirely.
		Cursor: textinput.CursorStyle{
			Color: tui.ColorPrimary,
			Shape: tea.CursorBlock,
			Blink: false,
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

// FieldLabel returns the field's label.
func (f *InputField) FieldLabel() string { return f.Label }

// FieldHelp returns the field's help text.
func (f *InputField) FieldHelp() string { return f.Help }

// ConsumesTextInput always reports true while focused: a focused InputField
// forwards every printable keystroke — including "?" — into the textinput.
func (f *InputField) ConsumesTextInput() bool {
	return f.focused
}

// SetValue replaces the field value and clears the default tag, since the
// value is now an explicit one rather than an unmodified default.
func (f *InputField) SetValue(value string) {
	f.input.SetValue(value)
	f.isDefault = false
}

// SetDefault sets the field's value to v and marks it as an unmodified
// default, which View renders dim with a "default" tag until the value
// changes. hasDefault latches permanently — unlike isDefault, it never
// clears — so the box keeps reserving the tag's room for the field's whole
// life; see boxOuterWidth.
func (f *InputField) SetDefault(v string) {
	f.input.SetValue(v)
	f.isDefault = true
	f.hasDefault = true
}

// IsDefault reports whether the field's value is still its unmodified default.
func (f *InputField) IsDefault() bool {
	return f.isDefault
}

// Focus gives the field focus, marks it touched so a later Blur will
// validate it, restores the cursor to where Blur last left it, and returns
// the textinput blink command.
func (f *InputField) Focus() tea.Cmd {
	f.focused = true
	f.touched = true
	f.input.SetCursor(f.savedPos)
	return f.input.Focus()
}

// Blur removes focus, scrolls a long value back to its head so it reads
// from the start while unfocused, and — only for a field that has actually
// held focus (touched), so the wizard never manufactures an error for a
// field nobody has visited via InputGroup's blur-everyone-else navigation —
// runs one validation pass so error state is current when the field is
// rendered next; the position save is guarded on an actual focus->blur
// transition so InputGroup.updateFocus's redundant re-blur of an
// already-blurred field can't collapse it to 0.
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
	w := f.boxOuterWidth()
	f.input.SetWidth(w - 5)
	f.input.SetCursor(f.input.Position())
}

// boxOuterWidth returns the box's render width: min(f.boxWidth, the width
// from SetWidth), minus defaultTagReserve once the field has ever carried a
// default value (hasDefault, not isDefault) — so the box reserves the
// "default" tag's room for its whole life and never resizes out from under
// the cursor when the user's first edit drops the tag.
func (f *InputField) boxOuterWidth() int {
	avail := f.width
	if f.hasDefault {
		avail -= defaultTagReserve
	}
	return min(f.boxWidth, max(avail, 0))
}

// Check runs the Required check and Validator without recording the
// result, so a caller probing validity (e.g. a section-complete indicator)
// can't paint error state onto a field the user hasn't touched; for
// password fields the raw value is scrubbed from error messages so
// secrets can't leak.
func (f *InputField) Check() error {
	if f.Required && strings.TrimSpace(f.input.Value()) == "" {
		// f.Label is lowercase everywhere it's set (e.g. "cluster name",
		// "pull secret"), matching the "verb noun" grammar every other error
		// in the wizard uses; "this field" covers the rare case a Required
		// field has no label rather than reading as "" is required.
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
	// Wraps a validator's error for password fields so its message can't
	// leak the raw value, while preserving Unwrap().
	if f.Password && err != nil && value != "" {
		var msg string
		// Short values (e.g. "a") would mangle unrelated chars via
		// ReplaceAll; fall back to a generic message instead.
		if len(value) >= 4 {
			msg = strings.ReplaceAll(err.Error(), value, "***")
		} else {
			msg = "invalid password"
		}
		err = &scrubbedError{msg: msg, inner: err}
	}
	return err
}

// Validate runs Check, records the result as the field's current error for
// View to render, and marks the field touched — so InputGroup.TouchAll can
// force every field to a current, visible validation state through this
// same interface method.
func (f *InputField) Validate() error {
	f.touched = true
	f.err = f.Check()
	return f.err
}

// Update forwards msg to the underlying textinput, clearing any stale
// validation error on keypress; a genuine edit (typing over, backspace, or
// delete) while the value is still an unmodified default clears both the
// default tag and the text itself, so the first keystroke replaces the
// default instead of appending to it, while pure cursor movement leaves the
// default and its tag untouched.
func (f *InputField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused {
		return f, nil
	}

	if k, ok := msg.(tea.KeyPressMsg); ok {
		f.err = nil
		if f.isDefault && (k.Text != "" || k.Code == tea.KeyBackspace || k.Code == tea.KeyDelete) {
			f.isDefault = false
			f.input.SetValue("")
		}
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

// blurredValueView renders a blurred non-empty value directly in the field's
// blurred text style, bypassing the textinput's cursor path: Blur parks the
// cursor on the value's first character, and the blurred cursor cell renders
// through the cursor's own TextStyle — which the textinput never sets on the
// value path — so that one character would read bright while the rest of the
// value stays dim (the same bug family as the Blink:false fix in
// fieldInputStyles).
func (f *InputField) blurredValueView() string {
	v := f.input.Value()
	if f.Password {
		v = strings.Repeat(string(f.input.EchoCharacter), lipgloss.Width(v))
	}
	style := fieldInputStyles(f.isDefault, f.Disabled).Blurred.Text
	return style.Render(tui.Truncate(v, max(f.boxOuterWidth()-4, 1)))
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

// TouchAll runs Validate on every field, marking each one touched and
// recording its current error, so a forced submission attempt (enter)
// shows every invalid field's real state at once rather than only the
// ones the user happened to visit.
func (g *InputGroup) TouchAll() {
	for _, f := range g.fields {
		_ = f.Validate()
	}
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

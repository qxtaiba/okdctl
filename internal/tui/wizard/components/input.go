package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
)

type InputField struct {
	Label       string
	Placeholder string
	Help        string
	Required    bool
	Password    bool
	Validator   func(string) error

	input        textinput.Model
	focused      bool
	width        int
	err          error
	isDefault    bool   // true if value is the original default (not user-modified)
	defaultValue string // stores the original default value
}

func NewInputField(label, placeholder string) *InputField {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 256
	ti.Width = 40

	return &InputField{
		Label:       label,
		Placeholder: placeholder,
		input:       ti,
	}
}

func NewPasswordField(label, placeholder string) *InputField {
	f := NewInputField(label, placeholder)
	f.Password = true
	f.input.EchoMode = textinput.EchoPassword
	f.input.EchoCharacter = '•'
	return f
}

func (f *InputField) Value() string {
	return f.input.Value()
}

func (f *InputField) SetValue(value string) {
	f.input.SetValue(value)
	f.isDefault = false
	f.updateTextStyle()
}

func (f *InputField) SetDefault(value string) {
	f.input.SetValue(value)
	f.defaultValue = value
	f.isDefault = true
	f.updateTextStyle()
}

func (f *InputField) IsDefault() bool {
	return f.isDefault
}

// updateTextStyle is intentionally a no-op. Setting TextStyle on textinput.Model
// causes ANSI escape codes that break width calculations and multi-line rendering.
// See charmbracelet/bubbles issues #812, #779.
func (f *InputField) updateTextStyle() {}

func (f *InputField) Focus() tea.Cmd {
	f.focused = true
	return f.input.Focus()
}

func (f *InputField) Blur() {
	f.focused = false
	f.input.Blur()
	_ = f.Validate()
}

func (f *InputField) IsFocused() bool {
	return f.focused
}

func (f *InputField) SetWidth(width int) {
	f.width = width
	inputWidth := width - 4 // border (2) + padding (2)
	if inputWidth < 20 {
		inputWidth = 20
	}
	f.input.Width = inputWidth
}

func (f *InputField) Validate() error {
	if f.Required && strings.TrimSpace(f.input.Value()) == "" {
		f.err = errRequired
		return f.err
	}
	if f.Validator != nil {
		f.err = f.Validator(f.input.Value())
		return f.err
	}
	f.err = nil
	return nil
}

func (f *InputField) Error() error {
	return f.err
}

func (f *InputField) Update(msg tea.Msg) (*InputField, tea.Cmd) {
	if !f.focused {
		return f, nil
	}

	if _, ok := msg.(tea.KeyMsg); ok {
		f.err = nil
	}

	oldValue := f.input.Value()

	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)

	if f.input.Value() != oldValue {
		f.isDefault = false
		f.updateTextStyle()
	}

	return f, cmd
}

func (f *InputField) View() string {
	labelStyle := lipgloss.NewStyle().
		Foreground(tui.ColorSlate300)

	hintStyle := lipgloss.NewStyle().
		Foreground(tui.ColorSlate500)

	labelText := strings.ToLower(f.Label)
	labelLine := labelStyle.Render(labelText)
	if f.Help != "" {
		labelLine += " " + hintStyle.Render("("+strings.ToLower(f.Help)+")")
	}

	if f.isDefault && f.input.Value() != "" {
		defaultIndicator := lipgloss.NewStyle().
			Foreground(tui.ColorSlate500).
			Italic(true).
			Render(" (default)")
		labelLine += defaultIndicator
	}

	contentWidth := f.input.Width
	if contentWidth < 20 {
		contentWidth = 40
	}

	var inputStyle lipgloss.Style
	if f.focused {
		inputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorPrimary).
			Padding(0, 1).
			Width(contentWidth)
	} else if f.err != nil {
		inputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorError).
			Padding(0, 1).
			Width(contentWidth)
	} else {
		inputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorSlate600).
			Padding(0, 1).
			Width(contentWidth)
	}

	input := inputStyle.Render(f.input.View())

	result := labelLine + "\n" + input

	if f.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(tui.ColorError)
		result += "\n" + errStyle.Render("✖ "+strings.ToLower(f.err.Error()))
	}

	return result
}

type requiredError struct{}

func (e requiredError) Error() string { return "this field is required" }

var errRequired = requiredError{}

type InputGroup struct {
	Title  string
	fields []*InputField

	focusIndex int
	focused    bool
	width      int
}

func NewInputGroup(title string, fields ...*InputField) *InputGroup {
	return &InputGroup{
		Title:      title,
		fields:     fields,
		focusIndex: 0,
	}
}

func (g *InputGroup) AddField(field *InputField) {
	g.fields = append(g.fields, field)
}

func (g *InputGroup) Fields() []*InputField {
	return g.fields
}

func (g *InputGroup) Field(index int) *InputField {
	if index >= 0 && index < len(g.fields) {
		return g.fields[index]
	}
	return nil
}

func (g *InputGroup) FieldByLabel(label string) *InputField {
	for _, f := range g.fields {
		if f.Label == label {
			return f
		}
	}
	return nil
}

func (g *InputGroup) FocusIndex() int {
	return g.focusIndex
}

func (g *InputGroup) SetFocusIndex(index int) {
	if index >= 0 && index < len(g.fields) {
		g.focusIndex = index
		g.updateFocus()
	}
}

func (g *InputGroup) Focus() tea.Cmd {
	g.focused = true
	return g.updateFocus()
}

func (g *InputGroup) Blur() {
	g.focused = false
	for _, f := range g.fields {
		f.Blur()
	}
}

func (g *InputGroup) IsFocused() bool {
	return g.focused
}

func (g *InputGroup) SetWidth(width int) {
	g.width = width
	for _, f := range g.fields {
		f.SetWidth(width) // ViewCompact has no padding
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

func (g *InputGroup) Next() tea.Cmd {
	g.focusIndex++
	if g.focusIndex >= len(g.fields) {
		g.focusIndex = 0
	}
	return g.updateFocus()
}

func (g *InputGroup) Previous() tea.Cmd {
	g.focusIndex--
	if g.focusIndex < 0 {
		g.focusIndex = len(g.fields) - 1
	}
	return g.updateFocus()
}

func (g *InputGroup) Validate() []error {
	var errors []error
	for _, f := range g.fields {
		if err := f.Validate(); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

func (g *InputGroup) IsValid() bool {
	for _, f := range g.fields {
		if err := f.Validate(); err != nil {
			return false
		}
	}
	return true
}

func (g *InputGroup) Update(msg tea.Msg) (*InputGroup, tea.Cmd) {
	if !g.focused || len(g.fields) == 0 {
		return g, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("tab", "down"))):
			return g, g.Next()
		case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab", "up"))):
			return g, g.Previous()
		}
	}

	var cmd tea.Cmd
	g.fields[g.focusIndex], cmd = g.fields[g.focusIndex].Update(msg)
	return g, cmd
}

func (g *InputGroup) View() string {
	var lines []string

	if g.Title != "" {
		titleStyle := lipgloss.NewStyle().
			Foreground(tui.ColorSlate300).
			Bold(true).
			MarginBottom(1)
		lines = append(lines, titleStyle.Render(g.Title))
	}

	for _, f := range g.fields {
		lines = append(lines, f.View())
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")

	if g.Title != "" {
		groupStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorSlate700).
			Padding(1, 2)
		return groupStyle.Render(content)
	}

	return content
}

func (g *InputGroup) ViewCompact(title string) string {
	var lines []string

	if title != "" {
		titleStyle := lipgloss.NewStyle().
			Foreground(tui.ColorCyan500).
			Bold(true)
		lines = append(lines, titleStyle.Render(title))
	}

	for i, f := range g.fields {
		lines = append(lines, f.View())
		if i < len(g.fields)-1 {
			lines = append(lines, "")
		}
	}

	return strings.Join(lines, "\n")
}

func (g *InputGroup) GetValues() map[string]string {
	values := make(map[string]string)
	for _, f := range g.fields {
		values[f.Label] = f.Value()
	}
	return values
}

func (g *InputGroup) SetValues(values map[string]string) {
	for label, value := range values {
		if f := g.FieldByLabel(label); f != nil {
			f.SetValue(value)
		}
	}
}

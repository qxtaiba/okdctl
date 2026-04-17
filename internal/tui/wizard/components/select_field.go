package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// SelectField is a dropdown-style form field that cycles through predefined
// options using left/right keys. It renders with the same bordered box style
// as InputField for visual consistency.
type SelectField struct {
	Label   string
	Help    string
	Options []string

	selected     int
	focused      bool
	width        int
	isDefault    bool
	defaultValue string
}

func NewSelectField(label string, options []string) *SelectField {
	return &SelectField{
		Label:   label,
		Options: options,
	}
}

func (f *SelectField) Value() string {
	if f.selected >= 0 && f.selected < len(f.Options) {
		return f.Options[f.selected]
	}
	return ""
}

func (f *SelectField) SetValue(value string) {
	for i, opt := range f.Options {
		if opt == value {
			f.selected = i
			f.isDefault = false
			return
		}
	}
}

func (f *SelectField) SetDefault(value string) {
	f.defaultValue = value
	f.isDefault = true
	for i, opt := range f.Options {
		if opt == value {
			f.selected = i
			return
		}
	}
}

func (f *SelectField) IsDefault() bool {
	return f.isDefault
}

func (f *SelectField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

func (f *SelectField) Blur() {
	f.focused = false
}

func (f *SelectField) IsFocused() bool {
	return f.focused
}

func (f *SelectField) SetWidth(width int) {
	f.width = width
}

func (f *SelectField) Validate() error {
	return nil // always valid — constrained to options
}

func (f *SelectField) Error() error {
	return nil
}

func (f *SelectField) SetOptions(options []string) {
	current := f.Value()
	f.Options = options
	// Preserve selection by name
	f.selected = 0
	for i, opt := range options {
		if opt == current {
			f.selected = i
			return
		}
	}
}

func (f *SelectField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused || len(f.Options) == 0 {
		return f, nil
	}

	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			f.selected--
			if f.selected < 0 {
				f.selected = len(f.Options) - 1
			}
			f.isDefault = false
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			f.selected++
			if f.selected >= len(f.Options) {
				f.selected = 0
			}
			f.isDefault = false
		}
	}

	return f, nil
}

func (f *SelectField) View() string {
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate300)
	hintStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)

	// Label line
	labelText := strings.ToLower(f.Label)
	labelLine := labelStyle.Render(labelText)
	if f.Help != "" {
		labelLine += " " + hintStyle.Render("("+strings.ToLower(f.Help)+")")
	}
	if f.isDefault && f.Value() != "" {
		defaultIndicator := lipgloss.NewStyle().
			Foreground(tui.ColorSlate500).
			Italic(true).
			Render(" (default)")
		labelLine += defaultIndicator
	}

	// Content width
	contentWidth := f.width - 4
	if contentWidth < 20 {
		contentWidth = 40
	}

	// Border style
	var boxStyle lipgloss.Style
	if f.focused {
		boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorPrimary).
			Padding(0, 1).
			Width(contentWidth)
	} else {
		boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorSlate600).
			Padding(0, 1).
			Width(contentWidth)
	}

	// Content inside box
	val := f.Value()
	var content string
	if f.focused && len(f.Options) > 1 {
		content = "◂ " + val + " ▸"
	} else {
		content = "> " + val
	}

	return labelLine + "\n" + boxStyle.Render(content)
}

// Package tui provides shared terminal UI primitives (lipgloss styles,
// color themes, icons, layouts, and print helpers) used by the CLI output
// and the bubbletea wizard.
package tui

import "charm.land/lipgloss/v2"

// Base text styles used across TUI output; rebuildStyles assigns them from
// the current colour vars since the vars below initialize before colors.go's
// init runs setTheme.
var (
	TitleStyle      lipgloss.Style
	TextStyle       lipgloss.Style
	MutedStyle      lipgloss.Style
	DimStyle        lipgloss.Style
	CodeInlineStyle lipgloss.Style
	SuccessStyle    lipgloss.Style
	ErrorStyle      lipgloss.Style
	WarningStyle    lipgloss.Style
	HighlightStyle  lipgloss.Style
	SpinnerStyle    lipgloss.Style
)

// rebuildStyles assigns TitleStyle through SpinnerStyle from the current
// colour vars; call after setTheme or SetDarkBackground changes them.
func rebuildStyles() {
	TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	TextStyle = lipgloss.NewStyle()
	MutedStyle = lipgloss.NewStyle().Foreground(ColorSlate500)
	DimStyle = lipgloss.NewStyle().Foreground(ColorTextDim)
	CodeInlineStyle = lipgloss.NewStyle().Foreground(ColorCyan400)
	SuccessStyle = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	ErrorStyle = lipgloss.NewStyle().Foreground(ColorError).Bold(true)
	WarningStyle = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
	HighlightStyle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	SpinnerStyle = lipgloss.NewStyle().Foreground(ColorCyan500).Bold(true)
}

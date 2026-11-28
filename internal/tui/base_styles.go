// Package tui provides the shared terminal UI primitives (styles, themes,
// icons, layouts) behind the CLI output and the wizard.
package tui

import "charm.land/lipgloss/v2"

// Base text styles, rebuilt from the active Theme by rebuildStyles.
var (
	TitleStyle      lipgloss.Style
	TextStyle       lipgloss.Style
	MutedStyle      lipgloss.Style
	DimStyle        lipgloss.Style
	CodeInlineStyle lipgloss.Style
	SuccessStyle    lipgloss.Style
	ErrorStyle      lipgloss.Style
	HighlightStyle  lipgloss.Style
)

func rebuildStyles() {
	TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary())
	TextStyle = lipgloss.NewStyle()
	MutedStyle = lipgloss.NewStyle().Foreground(ColorTextFaint())
	DimStyle = lipgloss.NewStyle().Foreground(ColorTextDim())
	CodeInlineStyle = lipgloss.NewStyle().Foreground(ColorCode())
	SuccessStyle = lipgloss.NewStyle().Foreground(ColorSuccess()).Bold(true)
	ErrorStyle = lipgloss.NewStyle().Foreground(ColorError()).Bold(true)
	HighlightStyle = lipgloss.NewStyle().Foreground(ColorPrimary()).Bold(true)
}

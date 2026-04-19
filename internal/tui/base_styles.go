// Package tui provides shared terminal UI primitives (lipgloss styles,
// color themes, icons, layouts, and print helpers) used by the CLI output
// and the bubbletea wizard.
package tui

import "charm.land/lipgloss/v2"

// Shared lipgloss styles used across the CLI output and wizard. Colors are
// resolved from the active ColorTheme; see colors.go.
var (
	TitleStyle      = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	TextStyle       = lipgloss.NewStyle().Foreground(ColorText)
	MutedStyle      = lipgloss.NewStyle().Foreground(ColorSlate500)
	DimStyle        = lipgloss.NewStyle().Foreground(ColorTextDim)
	CodeInlineStyle = lipgloss.NewStyle().Foreground(ColorCyan400)
	SuccessStyle    = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	ErrorStyle      = lipgloss.NewStyle().Foreground(ColorError).Bold(true)
	WarningStyle    = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
	HighlightStyle  = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
)

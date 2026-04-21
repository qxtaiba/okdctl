// Package tui provides shared terminal UI primitives (lipgloss styles,
// color themes, icons, layouts, and print helpers) used by the CLI output
// and the bubbletea wizard.
package tui

import "charm.land/lipgloss/v2"

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

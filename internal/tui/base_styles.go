package tui

import "github.com/charmbracelet/lipgloss"

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

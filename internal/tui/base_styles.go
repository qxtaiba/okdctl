package tui

import "github.com/charmbracelet/lipgloss"

// ═══════════════════════════════════════════════════════════════════════════════
// BASE STYLES
// ═══════════════════════════════════════════════════════════════════════════════

var (
	// TitleStyle is used for section titles and headings.
	TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)

	// TextStyle is the default style for body text.
	TextStyle = lipgloss.NewStyle().Foreground(ColorText)

	// MutedStyle is used for secondary or less important text.
	MutedStyle = lipgloss.NewStyle().Foreground(ColorSlate500)

	// DimStyle is used for dimmed or disabled text.
	DimStyle = lipgloss.NewStyle().Foreground(ColorTextDim)

	// CodeInlineStyle is used for inline code snippets.
	CodeInlineStyle = lipgloss.NewStyle().Foreground(ColorCyan400)

	// SuccessStyle is used for success messages and indicators.
	SuccessStyle = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)

	// ErrorStyle is used for error messages and indicators.
	ErrorStyle = lipgloss.NewStyle().Foreground(ColorError).Bold(true)

	// WarningStyle is used for warning messages and indicators.
	WarningStyle = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)

	// HighlightStyle is used for highlighted or emphasized content.
	HighlightStyle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
)

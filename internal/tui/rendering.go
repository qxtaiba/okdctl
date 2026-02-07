// Package tui provides terminal user interface components and styling
// for the openshitctl CLI wizard and deployment progress display.
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ═══════════════════════════════════════════════════════════════════════════════
// LOG PREFIXES - Rich-style bracketed log messages
// ═══════════════════════════════════════════════════════════════════════════════

// LogInfo renders a bracketed [INFO] prefix in cyan with message
func LogInfo(msg string) string {
	prefix := lipgloss.NewStyle().Foreground(ColorCyan500).Bold(true).Render("[INFO]")
	return prefix + " " + TextStyle.Render(msg)
}

// LogWarn renders a bracketed [WARN] prefix in amber/yellow with message
func LogWarn(msg string) string {
	prefix := lipgloss.NewStyle().Foreground(ColorWarning).Bold(true).Render("[WARN]")
	return prefix + " " + TextStyle.Render(msg)
}

// LogError renders a bracketed [ERROR] prefix in red with message
func LogError(msg string) string {
	prefix := lipgloss.NewStyle().Foreground(ColorError).Bold(true).Render("[ERROR]")
	return prefix + " " + TextStyle.Render(msg)
}

// LogDebug renders a bracketed [DEBUG] prefix in muted gray with message
func LogDebug(msg string) string {
	prefix := lipgloss.NewStyle().Foreground(ColorSlate500).Render("[DEBUG]")
	return prefix + " " + MutedStyle.Render(msg)
}

// ═══════════════════════════════════════════════════════════════════════════════
// SUBSECTION AND RESOURCE FORMATTING
// ═══════════════════════════════════════════════════════════════════════════════

// SubsectionLabel creates an inline subsection title in uppercase.
func SubsectionLabel(title string) string {
	return lipgloss.NewStyle().
		Foreground(ColorCyan500).
		Bold(true).
		Render(strings.ToUpper(title))
}

// ResourceLine formats a resource action line for deployment plans.
func ResourceLine(action, name, details string) string {
	var actionStyle lipgloss.Style
	switch action {
	case "+", "add", "create":
		actionStyle = SuccessStyle
		action = "+"
	case "-", "remove", "delete", "destroy":
		actionStyle = ErrorStyle
		action = "-"
	case "~", "change", "modify", "update":
		actionStyle = WarningStyle
		action = "~"
	default:
		actionStyle = MutedStyle
	}

	nameStyle := lipgloss.NewStyle().Foreground(ColorText)
	detailStyle := MutedStyle

	return actionStyle.Render(action) + " " +
		nameStyle.Render(name) + " " +
		detailStyle.Render(details)
}

// ═══════════════════════════════════════════════════════════════════════════════
// COMPLETION MESSAGES
// ═══════════════════════════════════════════════════════════════════════════════

// CompletionSuccess renders a final success message with checkmark icon.
func CompletionSuccess(msg string) string {
	return SuccessStyle.Render(IconSuccess) + " " + TextStyle.Render(msg)
}

// CompletionError renders a final error message with X icon.
func CompletionError(msg string) string {
	return ErrorStyle.Render(IconError) + " " + TextStyle.Render(msg)
}

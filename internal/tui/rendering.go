package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func LogInfo(msg string) string {
	prefix := lipgloss.NewStyle().Foreground(ColorCyan500).Bold(true).Render("[INFO]")
	return prefix + " " + TextStyle.Render(msg)
}

func LogWarn(msg string) string {
	prefix := lipgloss.NewStyle().Foreground(ColorWarning).Bold(true).Render("[WARN]")
	return prefix + " " + TextStyle.Render(msg)
}

func LogError(msg string) string {
	prefix := lipgloss.NewStyle().Foreground(ColorError).Bold(true).Render("[ERROR]")
	return prefix + " " + TextStyle.Render(msg)
}

func LogDebug(msg string) string {
	prefix := lipgloss.NewStyle().Foreground(ColorSlate500).Render("[DEBUG]")
	return prefix + " " + MutedStyle.Render(msg)
}

func SubsectionLabel(title string) string {
	return lipgloss.NewStyle().
		Foreground(ColorCyan500).
		Bold(true).
		Render(strings.ToUpper(title))
}

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

func CompletionSuccess(msg string) string {
	return SuccessStyle.Render(IconSuccess) + " " + TextStyle.Render(msg)
}

func CompletionError(msg string) string {
	return ErrorStyle.Render(IconError) + " " + TextStyle.Render(msg)
}

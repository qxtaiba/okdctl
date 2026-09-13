package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Card renders body inside a rounded box exactly width columns wide, with
// title spliced into the top border ("╭─ title ─…╮") and required to fit
// within width-6 columns or the exact-width guarantee breaks; body is
// padded to fit but never rewrapped, so the caller must pre-wrap each line
// to width-2 columns; and unlike BoxedSectionCompact, Card doesn't
// upper-case the title or run its output through Downsample, since it's
// meant for the wizard's live viewport rather than the CLI's static boxes.
func Card(title, body string, width int, accent color.Color) string {
	border := lipgloss.NewStyle().Foreground(accent)
	titleStyle := lipgloss.NewStyle().Foreground(accent).Bold(true)

	inner := max(width-2, 0)
	dashes := max(inner-lipgloss.Width(title)-4, 0)
	top := border.Render("╭─ ") + titleStyle.Render(title) +
		border.Render(" ─"+strings.Repeat("─", dashes)+"╮")

	rows := make([]string, 0, strings.Count(body, "\n")+3)
	rows = append(rows, top)
	for line := range strings.SplitSeq(body, "\n") {
		pad := max(inner-lipgloss.Width(line), 0)
		rows = append(rows, border.Render("│")+line+strings.Repeat(" ", pad)+border.Render("│"))
	}
	rows = append(rows, border.Render("╰"+strings.Repeat("─", inner)+"╯"))

	return strings.Join(rows, "\n")
}

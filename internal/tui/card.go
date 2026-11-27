package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Card renders body in a rounded box exactly width columns wide with title
// spliced into the top border; title must fit in width-6 columns, and body
// lines are padded or truncated, never rewrapped. Unlike BoxedSectionCompact
// it neither upper-cases the title nor Downsamples, since it targets the
// wizard's live viewport.
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
		line = Truncate(line, inner)
		pad := max(inner-lipgloss.Width(line), 0)
		rows = append(rows, border.Render("│")+line+strings.Repeat(" ", pad)+border.Render("│"))
	}
	rows = append(rows, border.Render("╰"+strings.Repeat("─", inner)+"╯"))

	return strings.Join(rows, "\n")
}

package wizard

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// Outer container and header/footer frame styles.
var (
	OuterContainerStyle = lipgloss.NewStyle().
				Padding(1, 2)

	WizardBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(tui.ColorSlate600)

	HeaderStyle = lipgloss.NewStyle().
			Padding(0, 1).
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(tui.ColorSlate700)

	FooterStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(tui.ColorSlate500)
)

// Header element styles (logo, tagline, step indicator).
var (
	LogoStyle = lipgloss.NewStyle().
			Foreground(tui.ColorPrimary).
			Bold(true)

	TaglineStyle = lipgloss.NewStyle().
			Foreground(tui.ColorSlate400).
			Italic(true)

	StepIndicatorStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSlate500)

	StepIndicatorCurrentStyle = lipgloss.NewStyle().
					Foreground(tui.ColorPrimary).
					Bold(true)
)

// Help-ribbon styles for footer key/text/separator rendering.
var (
	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(tui.ColorSlate300).
			Bold(true)

	HelpTextStyle = lipgloss.NewStyle().
			Foreground(tui.ColorSlate500)

	HelpSeparatorStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSlate700)
)

// Step progress-dot styles (completed / current / pending).
var (
	StepDotCompletedStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSuccess)

	StepDotCurrentStyle = lipgloss.NewStyle().
				Foreground(tui.ColorPrimary)

	StepDotPendingStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSlate600)
)

// RenderStepProgress renders the step dots: 1..current-1 completed, current active, rest pending.
func RenderStepProgress(current, total int) string {
	var parts []string
	for i := range total {
		switch {
		case i < current-1:
			parts = append(parts, StepDotCompletedStyle.Render(tui.IconActive))
		case i == current-1:
			parts = append(parts, StepDotCurrentStyle.Render(tui.IconActive))
		default:
			parts = append(parts, StepDotPendingStyle.Render(tui.IconPending))
		}
	}
	connector := StepDotPendingStyle.Render("─")
	return strings.Join(parts, connector)
}

// RenderHelpRibbon joins items as "key desc • key desc" within width,
// dropping whole items and appending "…" once the remainder no longer fits;
// the result never wraps.
func RenderHelpRibbon(items []KeyBinding, width int) string {
	sep := HelpSeparatorStyle.Render(" • ")
	more := HelpSeparatorStyle.Render(" …")

	var b strings.Builder
	used := 0
	for i, it := range items {
		piece := HelpKeyStyle.Render(it.Key) + " " + HelpTextStyle.Render(it.Help)
		need := lipgloss.Width(piece)
		if i > 0 {
			need += lipgloss.Width(sep)
		}
		reserve := 0
		if i < len(items)-1 {
			reserve = lipgloss.Width(more)
		}
		if used+need+reserve > width {
			b.WriteString(more)
			break
		}
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(piece)
		used += need
	}
	return lipgloss.NewStyle().MaxWidth(width).Inline(true).Render(b.String())
}

package wizard

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

var (
	OuterContainerStyle = lipgloss.NewStyle().
				Padding(1, 2)

	WizardBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderTop(true).
				BorderRight(true).
				BorderBottom(true).
				BorderLeft(true).
				BorderForeground(tui.ColorSlate600)

	HeaderStyle = lipgloss.NewStyle().
			Padding(0, 1).
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(tui.ColorSlate700)

	FooterStyle = lipgloss.NewStyle().
			Padding(0, 2).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(tui.ColorSlate700).
			Foreground(tui.ColorSlate500)
)

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

var (
	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(tui.ColorSlate900).
			Background(tui.ColorSlate300).
			Padding(0, 1).
			Bold(true)

	HelpTextStyle = lipgloss.NewStyle().
			Foreground(tui.ColorSlate500)

	HelpSeparatorStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSlate700)
)

var (
	StepDotCompletedStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSuccess)

	StepDotCurrentStyle = lipgloss.NewStyle().
				Foreground(tui.ColorPrimary)

	StepDotPendingStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSlate600)
)

func RenderStepProgress(current, total int) string {
	var parts []string
	for i := range total {
		switch {
		case i < current-1:
			parts = append(parts, StepDotCompletedStyle.Render("●"))
		case i == current-1:
			parts = append(parts, StepDotCurrentStyle.Render("●"))
		default:
			parts = append(parts, StepDotPendingStyle.Render("○"))
		}
	}
	connector := StepDotPendingStyle.Render("─")
	return strings.Join(parts, connector)
}

func RenderHelpItem(key, description string) string {
	return HelpKeyStyle.Render(key) + " " + HelpTextStyle.Render(description)
}

func RenderHelpBar(items []KeyBinding) string {
	var parts []string
	separator := HelpSeparatorStyle.Render("   ")

	for _, item := range items {
		parts = append(parts, RenderHelpItem(item.Key, item.Help))
	}

	return strings.Join(parts, separator)
}

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

// Help-bar styles for footer key/text/separator rendering.
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

// Step progress-dot styles (completed / current / pending).
var (
	StepDotCompletedStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSuccess)

	StepDotCurrentStyle = lipgloss.NewStyle().
				Foreground(tui.ColorPrimary)

	StepDotPendingStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSlate600)
)

// RenderStepProgress returns the wizard header's step dot indicator, with
// dot 1..current-1 styled as completed, current as active, and the rest
// pending.
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

// RenderHelpItem renders a single key/description pair for the help bar.
func RenderHelpItem(key, description string) string {
	return HelpKeyStyle.Render(key) + " " + HelpTextStyle.Render(description)
}

// RenderHelpBar joins multiple RenderHelpItem outputs with separators.
func RenderHelpBar(items []KeyBinding) string {
	var parts []string
	separator := HelpSeparatorStyle.Render("   ")

	for _, item := range items {
		parts = append(parts, RenderHelpItem(item.Key, item.Help))
	}

	return strings.Join(parts, separator)
}

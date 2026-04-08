package wizard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
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

	ContentStyle = lipgloss.NewStyle().
			Padding(1, 2)

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
	StepTitleStyle = lipgloss.NewStyle().
			Foreground(tui.ColorText).
			Bold(true).
			MarginBottom(1)

	StepDescriptionStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSlate400).
				MarginBottom(1)

	SectionHeaderStyle = lipgloss.NewStyle().
				Foreground(tui.ColorCyan500).
				Bold(true)
)

var (
	OptionSelectedStyle = lipgloss.NewStyle().
				Foreground(tui.ColorPrimary).
				Bold(true)

	OptionUnselectedStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSlate600)

	OptionTitleSelectedStyle = lipgloss.NewStyle().
					Foreground(tui.ColorText).
					Bold(true)

	OptionTitleStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSlate300)

	OptionDescriptionStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSlate500).
				PaddingLeft(4)

	OptionRequirementsStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSlate600).
				Italic(true).
				PaddingLeft(4)

	RecommendedBadgeStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSuccess).
				Italic(true)

	VerticalLineStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSlate700)
)

var (
	InputLabelStyle = lipgloss.NewStyle().
			Foreground(tui.ColorSlate300).
			Width(20)

	InputFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(tui.ColorPrimary).
				Padding(0, 1)

	InputBlurredStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(tui.ColorSlate600).
				Padding(0, 1)

	InputHintStyle = lipgloss.NewStyle().
			Foreground(tui.ColorSlate600).
			Italic(true)

	InputErrorStyle = lipgloss.NewStyle().
			Foreground(tui.ColorError)

	InputGroupStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorSlate700).
			Padding(1, 2).
			MarginBottom(1)

	InputGroupTitleStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSlate300).
				Bold(true).
				MarginBottom(1)
)

var (
	ContextPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(tui.ColorSlate700).
				Padding(1, 2)

	ContextTitleStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSlate400).
				Bold(true).
				MarginBottom(1)

	ContextLabelStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSlate500)

	ContextValueStyle = lipgloss.NewStyle().
				Foreground(tui.ColorText)

	ContextHighlightStyle = lipgloss.NewStyle().
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
		if i < current-1 {
			parts = append(parts, StepDotCompletedStyle.Render("●"))
		} else if i == current-1 {
			parts = append(parts, StepDotCurrentStyle.Render("●"))
		} else {
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

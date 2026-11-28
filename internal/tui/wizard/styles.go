package wizard

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// OuterContainerStyle carries no theme role, so it alone may build at init;
// every themed style rebinds on the background flip.
var OuterContainerStyle = lipgloss.NewStyle().
	Padding(1, 2)

// Frame, header, help-ribbon and step-dot styles, assigned by
// rebuildWizardStyles.
var (
	WizardBorderStyle lipgloss.Style
	HeaderStyle       lipgloss.Style
	FooterStyle       lipgloss.Style

	LogoStyle                 lipgloss.Style
	TaglineStyle              lipgloss.Style
	StepIndicatorStyle        lipgloss.Style
	StepIndicatorCurrentStyle lipgloss.Style

	HelpKeyStyle       lipgloss.Style
	HelpTextStyle      lipgloss.Style
	HelpSeparatorStyle lipgloss.Style

	StepDotCompletedStyle lipgloss.Style
	StepDotCurrentStyle   lipgloss.Style
	StepDotPendingStyle   lipgloss.Style
)

// rebuildWizardStyles assigns every themed wizard style from the active
// Theme; call at init and after every theme swap.
func rebuildWizardStyles() {
	WizardBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorPrimaryDim())

	HeaderStyle = lipgloss.NewStyle().
		Padding(0, 1).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(tui.ColorPrimaryDim())

	FooterStyle = lipgloss.NewStyle().
		Padding(0, 2).
		Foreground(tui.ColorTextFaint())

	LogoStyle = lipgloss.NewStyle().
		Foreground(tui.ColorPrimary()).
		Bold(true)

	TaglineStyle = lipgloss.NewStyle().
		Foreground(tui.ColorTextDim()).
		Italic(true)

	StepIndicatorStyle = lipgloss.NewStyle().
		Foreground(tui.ColorTextFaint())

	StepIndicatorCurrentStyle = lipgloss.NewStyle().
		Foreground(tui.ColorPrimary()).
		Bold(true)

	HelpKeyStyle = lipgloss.NewStyle().
		Foreground(tui.ColorTextSoft()).
		Bold(true)

	HelpTextStyle = lipgloss.NewStyle().
		Foreground(tui.ColorTextFaint())

	HelpSeparatorStyle = lipgloss.NewStyle().
		Foreground(tui.ColorRule())

	StepDotCompletedStyle = lipgloss.NewStyle().
		Foreground(tui.ColorSuccess())

	StepDotCurrentStyle = lipgloss.NewStyle().
		Foreground(tui.ColorPrimary())

	StepDotPendingStyle = lipgloss.NewStyle().
		Foreground(tui.ColorSubtle())

	stageLabelStyle = lipgloss.NewStyle().
		Foreground(tui.ColorTextFaint())

	stageLabelCurrentStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(tui.ColorText())

	stageSeparatorStyle = lipgloss.NewStyle().
		Foreground(tui.ColorSubtle())

	rebuildFormViewStyles()
}

func init() {
	rebuildWizardStyles()
}

// RenderStepProgress renders ✓/●/○ step dots, so state never rides on color
// alone.
func RenderStepProgress(current, total int) string {
	var parts []string
	for i := range total {
		switch {
		case i < current-1:
			parts = append(parts, StepDotCompletedStyle.Render(tui.IconSuccess))
		case i == current-1:
			parts = append(parts, StepDotCurrentStyle.Render(tui.IconActive))
		default:
			parts = append(parts, StepDotPendingStyle.Render(tui.IconPending))
		}
	}
	connector := StepDotPendingStyle.Render("─")
	return strings.Join(parts, connector)
}

// helpEssentialKeys are reserved before anything else: ctrl+c and "?" are
// the wizard's only guaranteed escape hatches, so truncation eats the other
// items first.
var helpEssentialKeys = map[string]bool{
	HelpCtrlC:    true,
	HelpQuestion: true,
}

// RenderHelpRibbon joins items as "key desc • key desc" within width,
// dropping non-essential items front to back and appending "…"; the result
// never wraps.
func RenderHelpRibbon(items []KeyBinding, width int) string {
	sep := HelpSeparatorStyle.Render(" • ")
	more := HelpSeparatorStyle.Render(" …")

	var essential, rest []KeyBinding
	for _, it := range items {
		if helpEssentialKeys[it.Key] {
			essential = append(essential, it)
		} else {
			rest = append(rest, it)
		}
	}

	essentialStr := joinHelpPieces(essential, sep)
	essentialWidth := lipgloss.Width(essentialStr)

	// A step's PinnedFooter can leave the ribbon a handful of columns; degrade
	// honestly rather than clip mid-word.
	if essentialWidth > width {
		return renderEssentialsDegraded(essential, width)
	}

	budget := width - essentialWidth
	if len(essential) > 0 && len(rest) > 0 {
		budget -= lipgloss.Width(sep)
	}

	var b strings.Builder
	used := 0
	dropped := false
	for i, it := range rest {
		piece := HelpKeyStyle.Render(it.Key) + " " + HelpTextStyle.Render(it.Help)
		need := lipgloss.Width(piece)
		if i > 0 {
			need += lipgloss.Width(sep)
		}
		reserve := 0
		if i < len(rest)-1 {
			reserve = lipgloss.Width(more)
		}
		if used+need+reserve > budget {
			dropped = true
			break
		}
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(piece)
		used += need
	}

	out := b.String()
	// Only mark the drop when something from rest rendered before it.
	if dropped && used > 0 {
		out += more
	}
	if essentialStr != "" {
		if out != "" {
			out += sep
		}
		out += essentialStr
	}

	return lipgloss.NewStyle().MaxWidth(width).Inline(true).Render(out)
}

const helpRibbonFloorWidth = 6

// renderEssentialsDegraded renders essential when it does not fit width:
// the "…" marker alone below helpRibbonFloorWidth, else plain truncated text.
func renderEssentialsDegraded(essential []KeyBinding, width int) string {
	if width < helpRibbonFloorWidth {
		return lipgloss.NewStyle().MaxWidth(width).Inline(true).Render(HelpSeparatorStyle.Render("…"))
	}

	var plain strings.Builder
	for i, it := range essential {
		if i > 0 {
			plain.WriteString(" • ")
		}
		plain.WriteString(it.Key + " " + it.Help)
	}

	return HelpTextStyle.Render(tui.Truncate(plain.String(), width))
}

func joinHelpPieces(items []KeyBinding, sep string) string {
	var b strings.Builder
	for i, it := range items {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(HelpKeyStyle.Render(it.Key) + " " + HelpTextStyle.Render(it.Help))
	}
	return b.String()
}

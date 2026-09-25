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

	// HeaderStyle and FooterStyle are assigned by rebuildWizardStyles since
	// they capture ColorSlate700/ColorSlate500, tiers SetDarkBackground rebinds.
	HeaderStyle lipgloss.Style
	FooterStyle lipgloss.Style
)

// Header element styles (logo, tagline, step indicator).
var (
	LogoStyle = lipgloss.NewStyle().
			Foreground(tui.ColorPrimary).
			Bold(true)

	TaglineStyle = lipgloss.NewStyle().
			Foreground(tui.ColorSlate400).
			Italic(true)

	// StepIndicatorStyle is assigned by rebuildWizardStyles (ColorSlate500).
	StepIndicatorStyle lipgloss.Style

	StepIndicatorCurrentStyle = lipgloss.NewStyle().
					Foreground(tui.ColorPrimary).
					Bold(true)
)

// Help-ribbon styles for footer key/text/separator rendering.
var (
	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(tui.ColorSlate300).
			Bold(true)

	// HelpTextStyle and HelpSeparatorStyle are assigned by rebuildWizardStyles
	// (ColorSlate500, ColorSlate700).
	HelpTextStyle      lipgloss.Style
	HelpSeparatorStyle lipgloss.Style
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

// rebuildWizardStyles assigns HeaderStyle, FooterStyle, StepIndicatorStyle,
// HelpTextStyle, and HelpSeparatorStyle from the current tui.ColorSlate500/700
// values; call at init and whenever tui.SetDarkBackground rebinds those tiers.
func rebuildWizardStyles() {
	HeaderStyle = lipgloss.NewStyle().
		Padding(0, 1).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(tui.ColorSlate700)

	FooterStyle = lipgloss.NewStyle().
		Padding(0, 2).
		Foreground(tui.ColorSlate500)

	StepIndicatorStyle = lipgloss.NewStyle().
		Foreground(tui.ColorSlate500)

	HelpTextStyle = lipgloss.NewStyle().
		Foreground(tui.ColorSlate500)

	HelpSeparatorStyle = lipgloss.NewStyle().
		Foreground(tui.ColorSlate700)
}

func init() {
	rebuildWizardStyles()
}

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

// helpEssentialKeys marks the ribbon entries RenderHelpRibbon reserves
// before doing anything else: the quit binding and the help-overlay hint
// are the wizard's only guaranteed escape hatches (ctrl+c always quits,
// "?" always lists everything else, including whatever the ribbon itself
// had no room for), so truncation eats into the other items first —
// wherever these two appear in items, they always render, trailing in that
// relative order.
var helpEssentialKeys = map[string]bool{
	HelpCtrlC:    true,
	HelpQuestion: true,
}

// RenderHelpRibbon joins items as "key desc • key desc" within width,
// reserving helpEssentialKeys first so truncation only ever drops the rest,
// front to back, and appending "…" once they still don't all fit; the
// result never wraps.
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

	// The essentials alone might not fit width at all — a step's own
	// PinnedFooter can claim nearly the whole row (e.g. preview's action
	// selector with several blockers), leaving the ribbon only a handful
	// of columns. Degrade honestly instead of falling through to the
	// bare MaxWidth clip below, which would cut mid-word with no
	// indication anything was cut ("ctrl+c quit" -> "ctr").
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
	// Only mark the drop when something from rest actually rendered before
	// it: with nothing to append "more" after, the marker would only eat
	// into the width reserved for the essential items themselves.
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

// helpRibbonFloorWidth is the point below which a truncated essential list
// still has no room for any of its own text — only the ellipsis marker fits.
const helpRibbonFloorWidth = 6

// renderEssentialsDegraded renders essential when it doesn't fit width at
// all: below helpRibbonFloorWidth, just the "…" marker; otherwise the
// essentials as plain text, rune-truncated with a trailing "…" via
// tui.Truncate — never the bare, unmarked clip lipgloss's own MaxWidth
// would produce on ANSI-styled text.
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

// joinHelpPieces renders items as "key desc • key desc" with no width
// limit — the caller has already reserved the room these need.
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

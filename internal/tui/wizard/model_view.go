package wizard

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// View implements tea.Model; it renders header, viewport, scroll
// indicator, optional error banner, and footer into a bordered box.
func (m *Model) View() tea.View {
	v := tea.View{AltScreen: true}

	if m.quitting {
		return v
	}

	if !m.ready {
		v.Content = "\n  Initializing..."
		return v
	}

	var content strings.Builder

	content.WriteString(m.renderHeader())
	content.WriteString("\n")
	content.WriteString(m.viewport.View())
	content.WriteString("\n")

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().
			Foreground(tui.ColorError).
			Bold(true).
			Padding(0, 1)
		content.WriteString(errorStyle.Render("✖ " + m.err.Error()))
		content.WriteString("\n")
	}

	// renderFooter prepends the scroll indicator to the help bar so the
	// scroll-indicator line doubles as the footer's top divider — one row
	// of vertical real estate instead of two.
	content.WriteString(m.renderFooter())

	// In lipgloss v2, Style.Width(N) sets OUTER width — the border is
	// counted INSIDE N, so the content area is N - 2. Pass contentWidth + 2
	// as the .Width() argument so the inner content area equals contentWidth
	// (what every render*() helper sizes itself to). Without this offset,
	// every full-width line wraps by 2 chars and the wraps push the bottom
	// of the box off the visible terminal.
	innerWidth := m.contentWidth()
	if innerWidth < minWidth {
		innerWidth = minWidth
	}

	bordered := WizardBorderStyle.
		Width(innerWidth + wizardBorderHorizontal).
		Render(content.String())

	v.Content = OuterContainerStyle.Render(bordered)
	return v
}

// contentWidth is the inner content area inside the wizard's border —
// what header/viewport/scrollIndicator/footer must size themselves to.
// It accounts for outerHorizontalPadding (4) and wizardBorderHorizontal (2).
// View() passes contentWidth + wizardBorderHorizontal to WizardBorderStyle's
// .Width() because in lipgloss v2 that argument is the OUTER width (border
// inclusive); both sides agree on what fits.
func (m *Model) contentWidth() int {
	width := m.width - outerHorizontalPadding - wizardBorderHorizontal
	if width < 60 {
		width = 60
	}
	return width
}

func (m *Model) contentDimensions() (width, height int) {
	width = m.contentWidth()
	height = m.height - fixedLayoutOverhead
	if height < 10 {
		height = 10
	}
	return width, height
}

func (m *Model) viewportDimensions() (width, height int) {
	contentWidth := m.contentWidth()

	viewportHeight := m.height - fixedLayoutOverhead

	if viewportHeight < 5 {
		viewportHeight = 5
	}

	return contentWidth, viewportHeight
}

func (m *Model) syncViewportContent() {
	if len(m.steps) == 0 || m.currentStep < 0 || m.currentStep >= len(m.steps) {
		m.viewport.SetContent("no steps configured")
		return
	}

	step := m.steps[m.currentStep]
	contentWidth := m.contentWidth()

	innerWidth := contentWidth - 4
	if innerWidth < 40 {
		innerWidth = 40
	}

	var content strings.Builder

	if d, ok := step.(DescribedStep); ok {
		if displayTitle := d.DisplayTitle(); displayTitle != "" {
			content.WriteString(m.renderStepTitle(displayTitle))
			content.WriteString("\n\n")
		}
	}

	stepContent := step.View(innerWidth, 1000)

	if c, ok := step.(centerable); ok && c.IsCentered() {
		viewportHeight := m.viewport.Height()
		contentWidth := lipgloss.Width(stepContent)
		contentHeight := lipgloss.Height(stepContent)

		leftPadding := (innerWidth - contentWidth) / 2
		topPadding := (viewportHeight - contentHeight) / 2
		if leftPadding < 0 {
			leftPadding = 0
		}
		if topPadding < 0 {
			topPadding = 0
		}

		stepContent = lipgloss.NewStyle().
			PaddingLeft(leftPadding).
			PaddingTop(topPadding).
			Render(stepContent)
	}

	content.WriteString(stepContent)

	paddingStyle := lipgloss.NewStyle().
		PaddingLeft(2).
		PaddingRight(2).
		Width(contentWidth)
	paddedContent := paddingStyle.Render(content.String())
	m.viewport.SetContent(paddedContent)
}

func (m *Model) renderHeader() string {
	width := m.contentWidth()

	brand := LogoStyle.Render("O P E N S H I T")
	tagline := TaglineStyle.Render("okd over proxmox, the easy way")

	visibleSteps := m.countVisibleSteps()
	currentVisible := m.currentVisibleStepIndex() + 1
	progressDots := RenderStepProgress(currentVisible, visibleSteps)
	stepIndicator := progressDots + " " +
		StepIndicatorStyle.Render("step ") +
		StepIndicatorCurrentStyle.Render(fmt.Sprintf("%d", currentVisible)) +
		StepIndicatorStyle.Render(fmt.Sprintf(" of %d", visibleSteps))

	taglineWidth := lipgloss.Width(tagline)
	indicatorWidth := lipgloss.Width(stepIndicator)
	spacing := width - taglineWidth - indicatorWidth - 2
	if spacing < 1 {
		spacing = 1
	}

	header := brand + "\n" + tagline + strings.Repeat(" ", spacing) + stepIndicator

	return HeaderStyle.Render(header)
}

func (m *Model) renderFooter() string {
	width := m.contentWidth()

	bindings := defaultKeyBindings()
	if len(m.steps) > 0 && m.currentStep >= 0 && m.currentStep < len(m.steps) {
		if h, ok := m.steps[m.currentStep].(HelpProvider); ok {
			bindings = h.ShortHelp()
		}
	}

	helpBar := RenderHelpBar(bindings)
	helpBarRendered := FooterStyle.Width(width).Render(helpBar)

	return m.renderScrollIndicator() + "\n" + helpBarRendered
}

func defaultKeyBindings() []KeyBinding {
	return []KeyBinding{
		{Key: "↑↓", Help: helpNavigate},
		{Key: helpEnter, Help: helpConfirm},
		{Key: helpEsc, Help: helpBack},
		{Key: helpCtrlC, Help: helpQuit},
	}
}

func (m *Model) renderStepTitle(title string) string {
	titleStyle := lipgloss.NewStyle().
		Foreground(tui.ColorText).
		Bold(true)
	return titleStyle.Render(title)
}

func (m *Model) renderScrollIndicator() string {
	width := m.contentWidth()
	lineStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate700)

	contextBadge := m.renderContextBadge()
	var badgeStyled string
	badgeWidth := 0
	if contextBadge != "" {
		badgeStyled = lipgloss.NewStyle().
			Foreground(tui.ColorSuccess).
			Bold(true).
			Render(" ▸ " + contextBadge + " ")
		badgeWidth = lipgloss.Width(badgeStyled)
	}

	if m.viewport.TotalLineCount() <= m.viewport.Height() {
		lineWidth := width - badgeWidth
		if lineWidth < 10 {
			lineWidth = 10
		}
		return lineStyle.Render(strings.Repeat("─", lineWidth)) + badgeStyled
	}

	scrollPercent := m.viewport.ScrollPercent()
	atTop := m.viewport.YOffset() == 0
	atBottom := scrollPercent >= 1.0

	arrowStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	dimArrowStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate600)
	textStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate400)

	var arrows string
	switch {
	case atTop:
		arrows = dimArrowStyle.Render("↑") + " " + arrowStyle.Render("↓")
	case atBottom:
		arrows = arrowStyle.Render("↑") + " " + dimArrowStyle.Render("↓")
	default:
		arrows = arrowStyle.Render("↑") + " " + arrowStyle.Render("↓")
	}

	var message string
	switch {
	case atTop:
		message = "scroll down for more"
	case atBottom:
		message = "scroll up for more"
	default:
		message = fmt.Sprintf("%.0f%% • scroll for more", scrollPercent*100)
	}

	indicator := arrows + "  " + textStyle.Render(message)
	indicatorWidth := lipgloss.Width(indicator)

	leftWidth := (width-indicatorWidth)/2 - 1                         // -1 for space before indicator
	rightWidth := width - leftWidth - indicatorWidth - badgeWidth - 2 // -2 for spaces around indicator
	if leftWidth < 3 {
		leftWidth = 3
	}
	if rightWidth < 3 {
		rightWidth = 3
	}

	leftLine := lineStyle.Render(strings.Repeat("─", leftWidth))
	rightLine := lineStyle.Render(strings.Repeat("─", rightWidth))

	return leftLine + " " + indicator + " " + rightLine + badgeStyled
}

func (m *Model) renderContextBadge() string {
	var parts []string

	if m.config.Distribution.Type != "" {
		parts = append(parts, string(m.config.Distribution.Type))
		if m.config.Distribution.Version != "" {
			parts[0] += " " + m.config.Distribution.Version
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, " → ")
}

func (m *Model) countVisibleSteps() int {
	count := 0
	for _, step := range m.steps {
		if stepShouldShow(step, m.config) {
			count++
		}
	}
	return count
}

func (m *Model) currentVisibleStepIndex() int {
	index := 0
	for i := 0; i < m.currentStep && i < len(m.steps); i++ {
		if stepShouldShow(m.steps[i], m.config) {
			index++
		}
	}
	return index
}

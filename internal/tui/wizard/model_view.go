package wizard

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

const tooSmallNotice = "okdctl needs at least 60×20 — resize the terminal"

// tooSmall reports whether the terminal is below the floor the wizard chrome needs to render.
func (m *Model) tooSmall() bool {
	return m.width < minTerminalWidth || m.height < minTerminalHeight
}

// View implements tea.Model, rendering header, viewport, status row, and
// footer into a bordered box drawn at exactly the terminal width.
func (m *Model) View() tea.View {
	v := tea.View{AltScreen: true}

	if m.quitting {
		return v
	}

	if !m.ready {
		v.Content = "\n  Initializing..."
		return v
	}

	if m.tooSmall() {
		v.Content = "\n  " + lipgloss.NewStyle().Foreground(tui.ColorTextDim).Render(tooSmallNotice)
		return v
	}

	var content strings.Builder

	content.WriteString(m.renderHeader())
	content.WriteString("\n")
	content.WriteString(m.viewport.View())
	content.WriteString("\n")
	content.WriteString(m.statusRow())
	content.WriteString("\n")
	content.WriteString(m.renderFooter())

	// lipgloss v2 Width(N) counts the border inside N — pass contentWidth+2 or
	// full-width lines clip by 2 chars.
	bordered := WizardBorderStyle.
		Width(m.contentWidth() + wizardBorderHorizontal).
		Render(content.String())

	v.Content = OuterContainerStyle.Render(bordered)
	return v
}

// contentWidth is the inner content area every header/viewport/footer helper sizes itself to.
func (m *Model) contentWidth() int {
	width := m.width - outerHorizontalPadding - wizardBorderHorizontal
	if width < minTerminalWidth-6 {
		width = minTerminalWidth - 6
	}
	return width
}

// statusRow is always exactly one row so the frame never grows; blank when clear.
func (m *Model) statusRow() string {
	width := m.contentWidth()
	if m.err == nil {
		return ""
	}
	// Padding is inert under Inline (lipgloss v2 skips it), so the 2-space
	// inset — matching the body's viewport inset — is prepended literally.
	style := lipgloss.NewStyle().Foreground(tui.ColorError).Inline(true).MaxWidth(width - 2)
	return "  " + style.Render(tui.IconError+" "+m.err.Error())
}

func (m *Model) contentDimensions() (width, height int) {
	width = m.contentWidth()
	height = m.height - fixedLayoutOverhead
	if height < 1 {
		height = 1
	}
	return width, height
}

func (m *Model) viewportDimensions() (width, height int) {
	contentWidth := m.contentWidth()

	viewportHeight := m.height - fixedLayoutOverhead
	if viewportHeight < 1 {
		viewportHeight = 1
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

// renderHeader draws the two-row header: brand (+ tagline on the first
// visible step) on row one, the step's display title and the progress
// trail on row two.
func (m *Model) renderHeader() string {
	width := m.contentWidth()

	brand := LogoStyle.Render("O K D C T L")
	if m.currentVisibleStepIndex() == 0 && m.chrome.Tagline != "" {
		brand += "  " + TaglineStyle.Render(m.chrome.Tagline)
	}

	right := m.renderTrail()
	rightW := lipgloss.Width(right)

	titleWidth := max(width-2-rightW-2, 8)
	title := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorText).Inline(true).
		Render(truncateTitle(m.headerTitle(), titleWidth))

	gap := max(width-2-lipgloss.Width(title)-rightW, 1)
	row2 := title + strings.Repeat(" ", gap) + right

	return HeaderStyle.Width(width).Render(brand + "\n" + row2)
}

// renderTrail renders the header's right-hand progress indicator: the
// chrome's Trail hook if set, otherwise the default dot ribbon.
func (m *Model) renderTrail() string {
	p := m.progressInfo()
	if m.chrome.Trail != nil {
		return m.chrome.Trail(p)
	}
	return RenderStepProgress(p.Current, p.Total) + "  " +
		StepIndicatorStyle.Render("step ") +
		StepIndicatorCurrentStyle.Render(strconv.Itoa(p.Current)) +
		StepIndicatorStyle.Render(" of "+strconv.Itoa(p.Total))
}

// headerTitle returns the current step's DisplayTitle(), falling back to
// its Title() when DisplayTitle is unimplemented or empty.
func (m *Model) headerTitle() string {
	if len(m.steps) == 0 || m.currentStep < 0 || m.currentStep >= len(m.steps) {
		return ""
	}
	step := m.steps[m.currentStep]
	if d, ok := step.(displayTitler); ok {
		if t := d.DisplayTitle(); t != "" {
			return t
		}
	}
	return step.Title()
}

// progressInfo reports the current step's position among visible steps for
// FlowChrome.Trail hooks.
func (m *Model) progressInfo() ProgressInfo {
	titles := make([]string, 0, len(m.steps))
	var currentID StepID
	for i, step := range m.steps {
		if !stepShouldShow(step, m.config) {
			continue
		}
		titles = append(titles, step.Title())
		if i == m.currentStep {
			currentID = step.ID()
		}
	}
	return ProgressInfo{
		Current:   m.currentVisibleStepIndex() + 1,
		Total:     m.countVisibleSteps(),
		CurrentID: currentID,
		Titles:    titles,
	}
}

// truncateTitle rune-safely clips s to fit within maxWidth visible columns,
// appending "…" when it clips — lipgloss's own MaxWidth truncates silently.
func truncateTitle(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for i := len(runes) - 1; i > 0; i-- {
		candidate := string(runes[:i]) + "…"
		if lipgloss.Width(candidate) <= maxWidth {
			return candidate
		}
	}
	return "…"
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
		{Key: "↑↓", Help: HelpNavigate},
		{Key: HelpEnter, Help: HelpConfirm},
		{Key: HelpEsc, Help: HelpBack},
		{Key: HelpCtrlC, Help: HelpQuit},
	}
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
	if m.chrome.Badge == nil {
		return ""
	}
	return m.chrome.Badge(m.config)
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

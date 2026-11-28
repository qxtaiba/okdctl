package wizard

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

const tooSmallNotice = "okdctl needs at least 60×20 — resize the terminal"

func (m *Model) tooSmall() bool {
	return m.width < minTerminalWidth || m.height < minTerminalHeight
}

// tooSmallContent clamps the resize notice to width×height so the one
// message about a too-small terminal never overflows it.
func tooSmallContent(width, height int) string {
	width, height = max(width, 1), max(height, 1)
	style := lipgloss.NewStyle().Foreground(tui.ColorTextDim())
	if height == 1 {
		return style.Render(tui.Truncate(tooSmallNotice, width))
	}
	lines := tui.WrapLines(tooSmallNotice, max(width-2, 1))
	if maxLines := height - 1; len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for i, l := range lines {
		lines[i] = tui.Truncate(l, width-2)
	}
	return style.Render("\n  " + strings.Join(lines, "\n  "))
}

// View renders header, viewport, status row and footer into a bordered box
// exactly the terminal width.
func (m *Model) View() tea.View {
	// ReportFocus is away mode's wire.
	v := tea.View{AltScreen: true, ReportFocus: true}
	v.WindowTitle = "okdctl · " + m.headerTitle()

	if m.quitting {
		return v
	}

	if !m.ready {
		v.Content = "\n  Initializing..."
		return v
	}

	if m.tooSmall() {
		v.Content = tooSmallContent(m.width, m.height)
		return v
	}

	body := m.viewport.View()
	if m.helpOpen {
		body = m.renderHelpOverlay()
	}

	var content strings.Builder

	if header := m.renderHeader(); header != "" {
		content.WriteString(header)
		content.WriteString("\n")
	}
	content.WriteString(body)
	content.WriteString("\n")
	content.WriteString(m.statusRow())
	content.WriteString("\n")
	content.WriteString(m.renderFooter())

	// lipgloss v2 Width(N) counts the border inside N.
	bordered := WizardBorderStyle.
		Width(m.contentWidth() + wizardBorderHorizontal).
		Render(content.String())

	// Width(m.width) restores the outer padding so every row spans the terminal.
	v.Content = OuterContainerStyle.Width(m.width).Render(bordered)
	return v
}

// contentWidth is the frame's inner width: the terminal minus outer padding
// and border, whatever the tier.
func (m *Model) contentWidth() int {
	width := m.width - outerHorizontalPadding - wizardBorderHorizontal
	if width < minTerminalWidth-6 {
		width = minTerminalWidth - 6
	}
	return width
}

// bodyWidth is the width a step renders at: the whole frame for a
// frameWidthOwner, the capped form measure otherwise.
func (m *Model) bodyWidth() int {
	if s, ok := m.CurrentStep().(frameWidthOwner); ok && s.OwnsFrameWidth() {
		return m.contentWidth()
	}
	width := m.contentWidth()
	if width > singleFormMaxWidth {
		width = singleFormMaxWidth
	}
	return width
}

// statusRow is always exactly one row so the frame never grows.
func (m *Model) statusRow() string {
	width := m.contentWidth()
	if m.discardPending {
		style := lipgloss.NewStyle().Foreground(tui.ColorWarning()).Inline(true)
		return "  " + style.Render(truncateTitle(tui.IconWarning+" "+discardPrompt, width-2))
	}
	if m.err == nil {
		return ""
	}
	// Padding is inert under Inline, so the inset is literal; truncateTitle,
	// not MaxWidth, so the "— fix" suffix is never silently clipped.
	style := lipgloss.NewStyle().Foreground(tui.ColorError()).Inline(true)
	return "  " + style.Render(truncateTitle(tui.IconError+" "+m.err.Error(), width-2))
}

func (m *Model) contentDimensions() (width, height int) {
	width = m.bodyWidth()
	height = m.height - m.layoutOverhead()
	if height < 1 {
		height = 1
	}
	return width, height
}

func (m *Model) viewportDimensions() (width, height int) {
	contentWidth := m.bodyWidth()

	viewportHeight := m.height - m.layoutOverhead()
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	viewportHeight = max(1, viewportHeight)

	return contentWidth, viewportHeight
}

func (m *Model) syncViewportContent() {
	if m.ready {
		_, height := m.viewportDimensions()
		m.viewport.SetHeight(height)
	}
	if len(m.steps) == 0 || m.currentStep < 0 || m.currentStep >= len(m.steps) {
		m.viewport.SetContent("no steps configured")
		return
	}

	step := m.steps[m.currentStep]
	contentWidth := m.bodyWidth()

	innerWidth := contentWidth - 4
	if innerWidth < 40 {
		innerWidth = 40
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

	padded, rows := padContent(stepContent, contentWidth)
	m.contentRows = rows
	m.viewport.SetContent(padded)
}

// padContent insets each step line into a width-wide column and records the
// viewport row each starts on, since an over-wide line wraps into several
// rows.
func padContent(content string, width int) (padded string, rows []int) {
	style := lipgloss.NewStyle().
		PaddingLeft(2).
		PaddingRight(2).
		Width(width)

	lines := strings.Split(content, "\n")
	rows = make([]int, 0, len(lines))

	out := make([]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, len(out))
		out = append(out, strings.Split(style.Render(line), "\n")...)
	}
	rows = append(rows, len(out))

	return strings.Join(out, "\n"), rows
}

// renderHeader draws the brand row and the title-plus-trail row.
func (m *Model) renderHeader() string {
	if m.headerRows() == 0 {
		return ""
	}

	width := m.contentWidth()

	brand := LogoStyle.Render("O K D C T L")
	if m.currentVisibleStepIndex() == 0 && m.chrome.Tagline != "" {
		brand += "  " + TaglineStyle.Render(m.chrome.Tagline)
	}

	right := m.renderTrail()
	rightW := lipgloss.Width(right)

	titleWidth := max(width-2-rightW-2, 8)
	title := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorText()).Inline(true).
		Render(truncateTitle(m.headerTitle(), titleWidth))

	gap := max(width-2-lipgloss.Width(title)-rightW, 1)
	row2 := title + strings.Repeat(" ", gap) + right

	return HeaderStyle.Width(width).Render(brand + "\n" + row2)
}

// headerRows is zero on a step that renders the wordmark itself.
func (m *Model) headerRows() int {
	if h, ok := m.CurrentStep().(heroRenderer); ok && h.RendersHero() {
		return 0
	}
	return headerHeight
}

// layoutOverhead is fixedLayoutOverhead less the header rows a hero step
// drops, which become viewport.
func (m *Model) layoutOverhead() int {
	return fixedLayoutOverhead - (headerHeight - m.headerRows())
}

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

// headerTitle returns DisplayTitle(), falling back to Title().
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

func (m *Model) renderStepTitle(title string) string {
	return lipgloss.NewStyle().Foreground(tui.ColorText()).Bold(true).Render(title)
}

// progressInfo reports the step's position among visible steps.
func (m *Model) progressInfo() ProgressInfo {
	visibleIDs := make([]StepID, 0, len(m.steps))
	var currentID StepID
	for i, step := range m.steps {
		if !stepShouldShow(step, m.config) {
			continue
		}
		visibleIDs = append(visibleIDs, step.ID())
		if i == m.currentStep {
			currentID = step.ID()
		}
	}
	return ProgressInfo{
		Current:    m.currentVisibleStepIndex() + 1,
		Total:      m.countVisibleSteps(),
		CurrentID:  currentID,
		VisibleIDs: visibleIDs,
	}
}

// truncateTitle clips s to maxWidth with "…"; lipgloss's MaxWidth truncates
// silently.
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

// renderFooter draws the scroll-indicator rule and the help row.
func (m *Model) renderFooter() string {
	return m.renderFooterRule() + "\n" + m.renderHelpRow()
}

func defaultKeyBindings() []KeyBinding {
	return []KeyBinding{
		{Key: "↑↓", Help: HelpNavigate},
		{Key: HelpEnter, Help: HelpConfirm},
		{Key: HelpEsc, Help: HelpBack},
		{Key: HelpCtrlC, Help: HelpQuit},
	}
}

// footerBindings returns the step's ShortHelp (or the defaults), a pgup/pgdn
// hint when the viewport overflows, and the always-visible "? help" hint.
func (m *Model) footerBindings() []KeyBinding {
	bindings := defaultKeyBindings()
	if len(m.steps) > 0 && m.currentStep >= 0 && m.currentStep < len(m.steps) {
		if h, ok := m.steps[m.currentStep].(HelpProvider); ok {
			bindings = h.ShortHelp()
		}
	}
	if m.viewport.TotalLineCount() > m.viewport.Height() {
		bindings = append(bindings, KeyBinding{Key: "pgup/pgdn", Help: "scroll"})
	}
	return append(bindings, KeyBinding{Key: HelpQuestion, Help: HelpOverlay})
}

// renderHelpOverlay renders the untruncated bindings, the step's
// OverlayHelpProvider extras and the vim/jump vocabulary over the body.
func (m *Model) renderHelpOverlay() string {
	width, height := m.viewportDimensions()

	bindings := m.footerBindings()
	hints := make([]components.KeyHint, len(bindings), len(bindings)+8)
	for i, b := range bindings {
		hints[i] = components.KeyHint{Key: b.Key, Help: b.Help}
	}
	if len(m.steps) > 0 && m.currentStep >= 0 && m.currentStep < len(m.steps) {
		if h, ok := m.steps[m.currentStep].(OverlayHelpProvider); ok {
			for _, b := range h.OverlayHelp() {
				hints = append(hints, components.KeyHint{Key: b.Key, Help: b.Help})
			}
		}
	}
	hints = append(hints,
		components.KeyHint{Key: "ctrl+d/u", Help: "half page"},
		components.KeyHint{Key: "gg/G", Help: "top/bottom"},
	)

	return components.RenderHelpOverlay(hints, width, height)
}

// renderHelpRow draws a step's PinnedFooter at the left with the ribbon
// right-aligned in the remaining width.
func (m *Model) renderHelpRow() string {
	width := m.contentWidth()
	innerWidth := width - 4 // FooterStyle Padding(0, 2) on both sides
	bindings := m.footerBindings()

	var left string
	if len(m.steps) > 0 && m.currentStep >= 0 && m.currentStep < len(m.steps) {
		if pf, ok := m.steps[m.currentStep].(PinnedFooter); ok {
			left = pf.PinnedFooter(innerWidth)
		}
	}

	if left == "" {
		return FooterStyle.Width(width).Render(RenderHelpRibbon(bindings, innerWidth))
	}

	leftWidth := lipgloss.Width(left)
	ribbonWidth := max(innerWidth-leftWidth-2, 0)
	ribbon := RenderHelpRibbon(bindings, ribbonWidth)
	row := left + lipgloss.PlaceHorizontal(innerWidth-leftWidth, lipgloss.Right, ribbon)
	return FooterStyle.Width(width).Render(row)
}

// renderFooterRule draws the "─" rule with the scroll indicator centred and
// the context badge at the right, degrading badge then indicator so the row
// never exceeds the content width.
func (m *Model) renderFooterRule() string {
	width := m.contentWidth()
	lineStyle := lipgloss.NewStyle().Foreground(tui.ColorRule())

	contextBadge := m.renderContextBadge()
	var badgeStyled string
	badgeWidth := 0
	if contextBadge != "" {
		badgeStyled = lipgloss.NewStyle().
			Foreground(tui.ColorSuccess()).
			Bold(true).Inline(true).
			Render(tui.Truncate(" "+tui.IconCaretRight+" "+contextBadge+" ", width/2))
		badgeWidth = lipgloss.Width(badgeStyled)
		if badgeWidth > width {
			badgeStyled, badgeWidth = "", 0
		}
	}

	avail := width - badgeWidth

	if arrows, message, scrollable := m.scrollIndicator(); scrollable {
		for _, ind := range []string{arrows + "  " + message, arrows} {
			if lipgloss.Width(ind)+centreInRuleReserve <= avail {
				return m.centreInRule(ind, avail) + badgeStyled
			}
		}
	}
	return lineStyle.Render(strings.Repeat("─", max(avail, 0))) + badgeStyled
}

// centreInRuleReserve is the rule floor plus one space each side.
const centreInRuleReserve = 8

// centreInRule centres ind within avail columns of rule; the caller must
// ensure ind plus centreInRuleReserve fits avail.
func (m *Model) centreInRule(ind string, avail int) string {
	lineStyle := lipgloss.NewStyle().Foreground(tui.ColorRule())
	indWidth := lipgloss.Width(ind)

	left := max((avail-indWidth-2)/2, 3)
	right := max(avail-left-indWidth-2, 3)

	return lineStyle.Render(strings.Repeat("─", left)) + " " + ind + " " + lineStyle.Render(strings.Repeat("─", right))
}

// scrollIndicator returns the footer's arrows and message separately so a
// tight fit can drop the message alone; scrollable is false when the
// content fits.
func (m *Model) scrollIndicator() (arrows, message string, scrollable bool) {
	if m.viewport.TotalLineCount() <= m.viewport.Height() {
		return "", "", false
	}

	scrollPercent := m.viewport.ScrollPercent()
	atTop := m.viewport.YOffset() == 0
	atBottom := scrollPercent >= 1.0

	arrowStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true)
	dimArrowStyle := lipgloss.NewStyle().Foreground(tui.ColorSubtle())
	textStyle := lipgloss.NewStyle().Foreground(tui.ColorTextDim())

	switch {
	case atTop:
		arrows = dimArrowStyle.Render("↑") + " " + arrowStyle.Render("↓")
	case atBottom:
		arrows = arrowStyle.Render("↑") + " " + dimArrowStyle.Render("↓")
	default:
		arrows = arrowStyle.Render("↑") + " " + arrowStyle.Render("↓")
	}

	switch {
	case atTop:
		message = "scroll down for more"
	case atBottom:
		message = "scroll up for more"
	default:
		message = fmt.Sprintf("%.0f%% • scroll for more", scrollPercent*100)
	}

	return arrows, textStyle.Render(message), true
}

func (m *Model) renderContextBadge() string {
	if m.chrome.Badge == nil {
		return ""
	}
	if b, ok := m.CurrentStep().(BadgeSuppressor); ok && b.SuppressesBadge() {
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

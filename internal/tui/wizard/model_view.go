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

// tooSmall reports whether the terminal is below the floor the wizard chrome needs to render.
func (m *Model) tooSmall() bool {
	return m.width < minTerminalWidth || m.height < minTerminalHeight
}

// View implements tea.Model, rendering header, viewport, status row, and
// footer into a bordered box drawn at exactly the terminal width.
func (m *Model) View() tea.View {
	v := tea.View{AltScreen: true}
	v.WindowTitle = "okdctl · " + m.headerTitle()

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

	body := m.viewport.View()
	if m.splitLayout() {
		body = m.composeWideBody(body)
	}
	// The overlay is a modal moment: it replaces the whole body region —
	// on a split tier it centers over the full content width rather than
	// sitting in the form column beside a still-rendered pane.
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

	// lipgloss v2 Width(N) counts the border inside N — pass contentWidth+2 or
	// full-width lines clip by 2 chars.
	bordered := WizardBorderStyle.
		Width(m.contentWidth() + wizardBorderHorizontal).
		Render(content.String())

	// Width(m.width) restores the outer padding around the frame so every
	// row spans the terminal exactly, the way AltScreen repaints expect.
	v.Content = OuterContainerStyle.Width(m.width).Render(bordered)
	return v
}

// contentWidth is the inner content area every header/status/footer helper
// sizes itself to: the terminal width minus the outer padding and the frame
// border — the frame always spans the terminal, whatever the tier; the
// readable measure is bodyWidth's business, inside the frame.
func (m *Model) contentWidth() int {
	width := m.width - outerHorizontalPadding - wizardBorderHorizontal
	if width < minTerminalWidth-6 {
		width = minTerminalWidth - 6
	}
	return width
}

// splitLayout reports whether the terminal is both wide enough and tall
// enough to split the frame into a form column and a context pane — width
// alone isn't sufficient: below splitMinHeight's floor the pane's own PROGRESS
// section wouldn't have room to render without truncating, so the frame
// falls back to the capped single-column tier instead of splitting into
// something unusably short. A splitSuppressor step declines the split at any
// size.
func (m *Model) splitLayout() bool {
	if s, ok := m.CurrentStep().(splitSuppressor); ok && s.SuppressesSplit() {
		return false
	}
	return SplitsFrame(m.width, m.height, m.countVisibleSteps())
}

// SplitsFrame reports whether a width×height terminal gives a stepCount-step
// flow a right-hand pane. Exported for the steps that render one thing beside a
// pane and another without it, so they ask the frame's own gate instead of
// re-deriving it from a body width the caps have already flattened.
func SplitsFrame(width, height, stepCount int) bool {
	return width >= wideSplitWidth && height >= splitMinHeight(stepCount)
}

// formPaneWidths returns the split layout's form column width (capped at
// formMaxWidth) and context pane width — the entire remainder after the rule
// column, floored at paneMinWidth; the pane wraps its content to whatever
// width the terminal hands it rather than idling surplus as margin.
func (m *Model) formPaneWidths() (form, pane int) {
	form = formMaxWidth
	pane = m.contentWidth() - form - paneRuleWidth
	if pane < paneMinWidth {
		pane = paneMinWidth
	}
	return form, pane
}

// bodyWidth is the width a step's own content renders at: the form column
// once the layout splits, the whole frame for a step that owns the full
// width (splitSuppressor), and the capped single-column measure otherwise.
func (m *Model) bodyWidth() int {
	if m.splitLayout() {
		form, _ := m.formPaneWidths()
		return form
	}
	if s, ok := m.CurrentStep().(splitSuppressor); ok && s.SuppressesSplit() {
		return m.contentWidth()
	}
	width := m.contentWidth()
	if width > singleFormMaxWidth {
		width = singleFormMaxWidth
	}
	return width
}

// composeWideBody joins the step's rendered form with a dim 1-column rule
// and the context pane, reaching exactly contentWidth columns total.
func (m *Model) composeWideBody(form string) string {
	_, paneWidth := m.formPaneWidths()
	height := m.viewport.Height()

	rule := renderPaneRule(height)
	pane := lipgloss.NewStyle().Width(paneWidth).Height(height).Render(m.paneBody(paneWidth, height))

	return lipgloss.JoinHorizontal(lipgloss.Top, form, rule, pane)
}

// paneBody renders the split layout's right pane: the active step's own content
// when it fills the pane itself, the context pane otherwise. An empty string
// from a paneRenderer falls back too, so a step with nothing to show yet keeps
// the pane useful rather than blank.
func (m *Model) paneBody(width, height int) string {
	if p, ok := m.CurrentStep().(paneRenderer); ok {
		if content := p.PaneContent(width, height); content != "" {
			return content
		}
	}
	return m.renderContextPane(width, height)
}

// renderPaneRule draws the dim vertical divider between the form and the
// context pane, height rows tall.
func renderPaneRule(height int) string {
	if height < 1 {
		height = 1
	}
	style := lipgloss.NewStyle().Foreground(tui.ColorSlate700)
	return style.Render(strings.Repeat("│\n", height-1) + "│")
}

// statusRow is always exactly one row so the frame never grows; blank when clear.
func (m *Model) statusRow() string {
	width := m.contentWidth()
	if m.err == nil {
		return ""
	}
	// Padding is inert under Inline (lipgloss v2 skips it), so the 2-space
	// inset — matching the body's viewport inset — is prepended literally.
	// truncateTitle, not MaxWidth: a silent clip amputates exactly the
	// "— fix" suffix every error carries.
	style := lipgloss.NewStyle().Foreground(tui.ColorError).Inline(true)
	return "  " + style.Render(truncateTitle(tui.IconError+" "+m.err.Error(), width-2))
}

// contentDimensions sizes a ResizableStep: bodyWidth (the form column, not
// the pane) by the same fixed-overhead height the viewport uses.
func (m *Model) contentDimensions() (width, height int) {
	width = m.bodyWidth()
	height = m.height - m.layoutOverhead()
	if height < 1 {
		height = 1
	}
	return width, height
}

// viewportDimensions sizes the scrollable viewport itself to bodyWidth — the
// form column alone, since the context pane beside it isn't scrollable.
func (m *Model) viewportDimensions() (width, height int) {
	contentWidth := m.bodyWidth()

	viewportHeight := m.height - m.layoutOverhead()
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

// padContent insets each step line into a width-wide content column and
// records the viewport row each one starts on: the column width wraps an
// over-wide line into several rows, which would otherwise desynchronise a
// step's LineSpan indices from the viewport's. Rendering line by line matches
// rendering the whole block.
func padContent(content string, width int) (padded string, rows []int) {
	style := lipgloss.NewStyle().
		PaddingLeft(2).
		PaddingRight(2).
		Width(width)

	lines := strings.Split(content, "\n")
	rows = make([]int, len(lines)+1)

	out := make([]string, 0, len(lines))
	for i, line := range lines {
		rows[i] = len(out)
		out = append(out, strings.Split(style.Render(line), "\n")...)
	}
	rows[len(lines)] = len(out)

	return strings.Join(out, "\n"), rows
}

// renderHeader draws the two-row header: brand (+ tagline on the first
// visible step) on row one, the step's display title and the progress
// trail on row two.
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
	title := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorText).Inline(true).
		Render(truncateTitle(m.headerTitle(), titleWidth))

	gap := max(width-2-lipgloss.Width(title)-rightW, 1)
	row2 := title + strings.Repeat(" ", gap) + right

	return HeaderStyle.Width(width).Render(brand + "\n" + row2)
}

// headerRows is the number of rows renderHeader draws: none at all on a step
// that renders the wordmark itself, the full three otherwise.
func (m *Model) headerRows() int {
	if h, ok := m.CurrentStep().(heroRenderer); ok && h.RendersHero() {
		return 0
	}
	return headerHeight
}

// layoutOverhead is the frame's non-body row budget for the active step:
// fixedLayoutOverhead, less whatever header rows a hero step drops, so the
// reclaimed rows become viewport instead of blank frame.
func (m *Model) layoutOverhead() int {
	return fixedLayoutOverhead - (headerHeight - m.headerRows())
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
	visibleIDs := make([]StepID, 0, len(m.steps))
	var currentID StepID
	for i, step := range m.steps {
		if !stepShouldShow(step, m.config) {
			continue
		}
		titles = append(titles, step.Title())
		visibleIDs = append(visibleIDs, step.ID())
		if i == m.currentStep {
			currentID = step.ID()
		}
	}
	return ProgressInfo{
		Current:    m.currentVisibleStepIndex() + 1,
		Total:      m.countVisibleSteps(),
		CurrentID:  currentID,
		Titles:     titles,
		VisibleIDs: visibleIDs,
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

// renderFooter draws the two-row footer: the scroll-indicator rule on top,
// the help ribbon (or a step's PinnedFooter row) beneath it.
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

// footerBindings returns the current step's ShortHelp() bindings (falling
// back to defaultKeyBindings when the step has none), plus a pgup/pgdn hint
// whenever the viewport content overflows its height, plus a trailing "?
// help" hint — always visible, since it's the wizard's guaranteed discovery
// path for whatever else the footer ribbon has no room to show.
func (m *Model) footerBindings() []KeyBinding {
	bindings := defaultKeyBindings()
	if len(m.steps) > 0 && m.currentStep >= 0 && m.currentStep < len(m.steps) {
		if h, ok := m.steps[m.currentStep].(HelpProvider); ok {
			bindings = h.ShortHelp()
		}
	}
	// A step paging its own log region owns pgup/pgdn (and says so in its
	// ShortHelp); advertising a viewport scroll beside it would be a lie.
	if m.viewport.TotalLineCount() > m.viewport.Height() && !m.currentStepConsumesPaging() {
		bindings = append(bindings, KeyBinding{Key: "pgup/pgdn", Help: "scroll"})
	}
	bindings = m.withHubEscape(bindings)
	return append(bindings, KeyBinding{Key: HelpQuestion, Help: HelpOverlay})
}

// withHubEscape advertises the esc round-trip on a swapped-in flow's first
// screen, where esc leaves the sub-flow instead of stepping back within it.
// Steps that already bind esc themselves are left alone — theirs says where it
// goes, and two esc entries in one ribbon would read as a contradiction.
func (m *Model) withHubEscape(bindings []KeyBinding) []KeyBinding {
	if m.suspended == nil || m.currentStep != 0 {
		return bindings
	}
	for _, b := range bindings {
		if b.Key == HelpEsc {
			return bindings
		}
	}
	return append(bindings, KeyBinding{Key: HelpEsc, Help: "hub"})
}

// renderHelpOverlay renders the full, untruncated key-binding list (the
// same bindings footerBindings feeds the ribbon) over the body region,
// sized to exactly replace it — the full content width on a split tier.
func (m *Model) renderHelpOverlay() string {
	width, height := m.viewportDimensions()
	if m.splitLayout() {
		width = m.contentWidth()
	}

	bindings := m.footerBindings()
	hints := make([]components.KeyHint, len(bindings))
	for i, b := range bindings {
		hints[i] = components.KeyHint{Key: b.Key, Help: b.Help}
	}

	return components.RenderHelpOverlay(hints, width, height)
}

// renderHelpRow draws the footer's help row: a step's PinnedFooter text at
// the left (when implemented) with the key-binding ribbon right-aligned in
// the remaining width, or just the ribbon on its own otherwise.
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

// renderFooterRule draws the footer's top row: a "─" rule with the scroll
// indicator centred in it when the viewport overflows, plus the context
// badge pinned to the right. The row never exceeds the content width: on a
// tight fit it degrades to bare arrows, then to a plain rule, and drops the
// badge itself before it would wrap the frame.
func (m *Model) renderFooterRule() string {
	width := m.contentWidth()
	lineStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate700)

	contextBadge := m.renderContextBadge()
	var badgeStyled string
	badgeWidth := 0
	if contextBadge != "" {
		badgeStyled = lipgloss.NewStyle().
			Foreground(tui.ColorSuccess).
			Bold(true).
			Render(" " + tui.IconCaretRight + " " + contextBadge + " ")
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

// centreInRuleReserve is the room centreInRule needs around an indicator:
// a 3-column rule floor and one space on each side.
const centreInRuleReserve = 8

// centreInRule centres ind within avail columns of "─" rule, keeping the
// two sides within one column of each other; callers must ensure ind fits
// (its width plus centreInRuleReserve at most avail) or the row overflows.
func (m *Model) centreInRule(ind string, avail int) string {
	lineStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate700)
	indWidth := lipgloss.Width(ind)

	left := max((avail-indWidth-2)/2, 3)
	right := max(avail-left-indWidth-2, 3)

	return lineStyle.Render(strings.Repeat("─", left)) + " " + ind + " " + lineStyle.Render(strings.Repeat("─", right))
}

// scrollIndicator returns the footer's scroll-state arrows and message
// separately — so renderFooterRule can drop the message alone on a tight
// fit — and whether the viewport currently overflows its height; it returns
// ("", "", false) when the content fits without scrolling.
func (m *Model) scrollIndicator() (arrows, message string, scrollable bool) {
	if m.viewport.TotalLineCount() <= m.viewport.Height() {
		return "", "", false
	}

	scrollPercent := m.viewport.ScrollPercent()
	atTop := m.viewport.YOffset() == 0
	atBottom := scrollPercent >= 1.0

	arrowStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	dimArrowStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate600)
	textStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate400)

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

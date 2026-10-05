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

// tooSmallContent renders tooSmallNotice clamped to width×height: the one
// message telling the operator to resize must itself fit however small the
// terminal actually is, down to 1×1 — a wrapped-but-unclamped notice would
// otherwise be the one thing that overflows a terminal too small to show it.
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

// View implements tea.Model, rendering header, viewport, status row, and
// footer into a bordered box drawn at exactly the terminal width.
func (m *Model) View() tea.View {
	// ReportFocus is away mode's wire: BlurMsg drops the shared clock to
	// 1Hz, FocusMsg plays the catch-up sweep.
	v := tea.View{AltScreen: true, ReportFocus: true}
	v.WindowTitle = "okdctl · " + m.windowTitle()
	v.ProgressBar = m.terminalProgress()

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
	// The overlay is a modal moment: it replaces the whole body region.
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

// bodyWidth is the width a step's own content renders at: the whole frame
// for a step that owns the full width (frameWidthOwner), and the capped form
// measure otherwise.
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

// statusRow is always exactly one row so the frame never grows; blank when clear.
func (m *Model) statusRow() string {
	width := m.contentWidth()
	if m.discardPending {
		style := lipgloss.NewStyle().Foreground(tui.ColorWarning()).Inline(true)
		return "  " + style.Render(truncateTitle(tui.IconWarning+" "+discardPrompt, width-2))
	}
	if m.err == nil {
		return ""
	}
	// Padding is inert under Inline (lipgloss v2 skips it), so the 2-space
	// inset — matching the body's viewport inset — is prepended literally.
	// truncateTitle, not MaxWidth: a silent clip amputates exactly the
	// "— fix" suffix every error carries.
	style := lipgloss.NewStyle().Foreground(tui.ColorError()).Inline(true)
	return "  " + style.Render(truncateTitle(tui.IconError+" "+m.err.Error(), width-2))
}

// contentDimensions sizes a ResizableStep: bodyWidth by the same
// fixed-overhead height the viewport uses.
func (m *Model) contentDimensions() (width, height int) {
	width = m.bodyWidth()
	height = m.height - m.layoutOverhead()
	if height < 1 {
		height = 1
	}
	return width, height
}

// viewportDimensions sizes the scrollable viewport itself to bodyWidth.
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
	title := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorText()).Inline(true).
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

// renderStepTitle styles a step's DisplayTitle for the rare caller measuring
// or rendering it outside the persistent header row (headerTitle/renderHeader
// already show it there for every step); most steps never need this since
// their title lives in the header, not inline in the scrollable body.
func (m *Model) renderStepTitle(title string) string {
	return lipgloss.NewStyle().Foreground(tui.ColorText()).Bold(true).Render(title)
}

// progressInfo reports the current step's position among visible steps for
// FlowChrome.Trail hooks.
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
// The label names where esc actually lands rather than assuming it is
// always the hub: a flow chained off a non-hub screen (deployexec's finish
// screen opening manage nodes via its "n" key, say) returns esc there,
// not to the five-verb hub, and the ribbon must not promise what esc does
// not do. A step's own ShortHelp cannot know which case it is in — its own
// esc entry (e.g. a hardcoded "hub") is corrected in place rather than left
// alone, so the wording stays truthful regardless of what the step declared.
func (m *Model) withHubEscape(bindings []KeyBinding) []KeyBinding {
	if m.suspended == nil || m.currentStep != 0 {
		return bindings
	}
	label := m.escDestinationLabel()
	for i, b := range bindings {
		if b.Key == HelpEsc {
			bindings[i].Help = label
			return bindings
		}
	}
	return append(bindings, KeyBinding{Key: HelpEsc, Help: label})
}

// escDestinationLabel names what esc returns to from a swapped-in flow's
// first screen: "hub" only when the suspended flow's own return point is
// the hub screen itself, "back" for every other chained origin.
func (m *Model) escDestinationLabel() string {
	if m.suspended.currentStep >= 0 && m.suspended.currentStep < len(m.suspended.steps) {
		if m.suspended.steps[m.suspended.currentStep].ID() == StepIDWelcome {
			return "hub"
		}
	}
	return "back"
}

// windowTitler is implemented by steps whose terminal-tab title carries
// live state richer than the on-screen header (the install's percent and
// phase); an empty return falls back to the header title.
type windowTitler interface {
	WindowTitle() string
}

// terminalProgressStep is implemented by steps that drive the terminal's
// own progress indication (OSC 9;4 — taskbar and tab progress in the
// terminals that render it); ProgressBarNone means nothing to report.
type terminalProgressStep interface {
	TerminalProgress() (tea.ProgressBarState, int)
}

// terminalProgress asks the active step for its OSC 9;4 reading, gated on
// the color profile: a pipe or NO_COLOR run stays byte-identical, and
// terminals that don't understand the bytes ignore them silently. Nil — the
// renderer's reset — the moment no step reports one, so the indicator
// clears on exit.
func (m *Model) terminalProgress() *tea.ProgressBar {
	if !tui.ColorEnabled() {
		return nil
	}
	tp, ok := m.CurrentStep().(terminalProgressStep)
	if !ok {
		return nil
	}
	state, value := tp.TerminalProgress()
	if state == tea.ProgressBarNone {
		return nil
	}
	return tea.NewProgressBar(state, value)
}

// windowTitle names the terminal tab: the active step's own live title when
// it provides one, the header title otherwise.
func (m *Model) windowTitle() string {
	if wt, ok := m.CurrentStep().(windowTitler); ok {
		if t := wt.WindowTitle(); t != "" {
			return t
		}
	}
	return m.headerTitle()
}

// renderHelpOverlay renders the full, untruncated key-binding list — the
// same bindings footerBindings feeds the ribbon, plus the active step's own
// footer-silent extras (OverlayHelpProvider) and the wizard's footer-silent
// vim/jump vocabulary — over the body region, sized to exactly replace it.
func (m *Model) renderHelpOverlay() string {
	width, height := m.viewportDimensions()

	bindings := m.footerBindings()
	hints := make([]components.KeyHint, len(bindings), len(bindings)+8)
	for i, b := range bindings {
		hints[i] = components.KeyHint{Key: b.Key, Help: b.Help}
	}
	// A step's own footer-silent bindings (OverlayHelpProvider) surface only
	// here, alongside its ShortHelp entries in the same screen section.
	if len(m.steps) > 0 && m.currentStep >= 0 && m.currentStep < len(m.steps) {
		if h, ok := m.steps[m.currentStep].(OverlayHelpProvider); ok {
			for _, b := range h.OverlayHelp() {
				hints = append(hints, components.KeyHint{Key: b.Key, Help: b.Help})
			}
		}
	}
	// The footer-silent vim vocabulary surfaces only here, under its own
	// overlay group.
	hints = append(hints,
		components.KeyHint{Key: "j/k", Help: "scroll"},
		components.KeyHint{Key: "ctrl+d/u", Help: "half page"},
		components.KeyHint{Key: "gg/G", Help: "top/bottom"},
	)

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
	lineStyle := lipgloss.NewStyle().Foreground(tui.ColorRule())

	contextBadge := m.renderContextBadge()
	var badgeStyled string
	badgeWidth := 0
	if contextBadge != "" {
		badgeStyled = lipgloss.NewStyle().
			Foreground(tui.ColorSuccess()).
			Bold(true).Inline(true).MaxWidth(width / 3).
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
	lineStyle := lipgloss.NewStyle().Foreground(tui.ColorRule())
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

package wizard

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m *Model) handleResize(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height

	m.sizeCurrentStep()
	m.resizeViewport()
	m.syncViewportContent()
	m.notifyIfAtBottom()
	// Deliberately does not re-scroll to the focused field: a resize must
	// resync content/height without fighting a scroll position the operator
	// (or a test) just set deliberately — e.g. 'G'/'gg'/pgup/pgdn — which a
	// forced autoScrollToField here would silently overwrite on every
	// WindowSizeMsg, including a same-size one a render helper re-sends.
}

// sizeCurrentStep hands the active step the terminal's own dimensions and then
// the content box it renders into: a TerminalSizer's layout decision is stated
// in terminal columns and rows, which the frame's width caps flatten out of the
// content box.
func (m *Model) sizeCurrentStep() {
	if len(m.steps) == 0 || m.currentStep < 0 || m.currentStep >= len(m.steps) {
		return
	}
	step := m.steps[m.currentStep]
	if ts, ok := step.(TerminalSizer); ok {
		ts.SetTerminalSize(m.width, m.height)
	}
	contentWidth, contentHeight := m.contentDimensions()
	if r, ok := step.(ResizableStep); ok {
		r.SetSize(contentWidth, contentHeight)
	}
}

// resizeViewport re-fits the scrollable viewport to the active step's body
// width and the frame's fixed height budget, constructing it on first call.
// Called on every step transition as well as every terminal resize: a
// splitSuppressor step changes the body width without the terminal changing at
// all.
func (m *Model) resizeViewport() {
	width, height := m.viewportDimensions()
	if !m.ready {
		m.viewport = viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
		m.ready = true
		return
	}
	m.viewport.SetWidth(width)
	m.viewport.SetHeight(height)
}

// handleScrollKey scrolls the viewport on page/home/end/arrow keys — plus
// the footer-silent vim vocabulary (j/k, ctrl+d/u, gg/G) — and reports
// whether it consumed msg; home/end fall through to a focused text input
// (line-start/line-end cursor moves), pgup/pgdn to a step paging a log
// region of its own, the arrows and j/k scroll only for steps that opted in
// via arrowScroller, and every vim key falls through while a text input is
// focused — every other step keeps its own keys.
func (m *Model) handleScrollKey(msg tea.KeyPressMsg) bool {
	if !m.ready {
		return false
	}
	if handled, consumed := m.handleVimScrollKey(msg); handled {
		if consumed {
			m.notifyIfAtBottom()
		}
		return consumed
	}
	switch {
	case key.Matches(msg, m.keyMap.PageUp):
		if m.currentStepConsumesPaging() {
			return false
		}
		m.viewport.HalfPageUp()
	case key.Matches(msg, m.keyMap.PageDown):
		if m.currentStepConsumesPaging() {
			return false
		}
		m.viewport.HalfPageDown()
	case key.Matches(msg, m.keyMap.Home):
		if m.currentStepConsumesTextInput() {
			return false
		}
		m.viewport.GotoTop()
	case key.Matches(msg, m.keyMap.End):
		if m.currentStepConsumesTextInput() {
			return false
		}
		m.viewport.GotoBottom()
	case key.Matches(msg, m.keyMap.Up):
		if !m.currentStepScrollsWithArrows() {
			return false
		}
		m.viewport.ScrollUp(1)
	case key.Matches(msg, m.keyMap.Down):
		if !m.currentStepScrollsWithArrows() {
			return false
		}
		m.viewport.ScrollDown(1)
	default:
		return false
	}
	m.notifyIfAtBottom()
	return true
}

// handleVimScrollKey handles the footer-silent vim vocabulary: j/k mirror
// the arrow gate, ctrl+d/u the half-page keys, gg/G jump to the ends, and
// every key falls through while a text input is focused. handled reports
// whether msg was a vim key at all; consumed whether it acted (or, for a
// lone g, armed the gg chord).
func (m *Model) handleVimScrollKey(msg tea.KeyPressMsg) (handled, consumed bool) {
	pendingG := m.pendingG
	m.pendingG = false
	switch {
	case key.Matches(msg, m.keyMap.VimDown), key.Matches(msg, m.keyMap.VimUp):
		if !m.currentStepScrollsWithArrows() || m.currentStepConsumesTextInput() {
			return true, false
		}
		if key.Matches(msg, m.keyMap.VimDown) {
			m.viewport.ScrollDown(1)
		} else {
			m.viewport.ScrollUp(1)
		}
	case key.Matches(msg, m.keyMap.VimHalfDown), key.Matches(msg, m.keyMap.VimHalfUp):
		if m.currentStepConsumesPaging() || m.currentStepConsumesTextInput() {
			return true, false
		}
		if key.Matches(msg, m.keyMap.VimHalfDown) {
			m.viewport.HalfPageDown()
		} else {
			m.viewport.HalfPageUp()
		}
	case key.Matches(msg, m.keyMap.VimBottom):
		if m.currentStepConsumesTextInput() {
			return true, false
		}
		m.viewport.GotoBottom()
	case key.Matches(msg, m.keyMap.VimTop):
		if m.currentStepConsumesTextInput() {
			return true, false
		}
		if !pendingG {
			m.pendingG = true
			return true, true
		}
		m.viewport.GotoTop()
	default:
		return false, false
	}
	return true, true
}

// notifyIfAtBottom tells the active step, if it implements BottomNotifiable,
// that the viewport currently shows its last line — either because the
// content fits without scrolling or because the operator has scrolled all
// the way down. A step cannot see the viewport's own scroll offset, so this
// is the only signal it gets that content below an initial fold was ever
// actually displayed.
func (m *Model) notifyIfAtBottom() {
	if !m.ready || len(m.steps) == 0 || m.currentStep < 0 || m.currentStep >= len(m.steps) {
		return
	}
	atBottom := m.viewport.TotalLineCount() <= m.viewport.Height() || m.viewport.ScrollPercent() >= 1.0
	if !atBottom {
		return
	}
	if n, ok := m.steps[m.currentStep].(BottomNotifiable); ok {
		n.NotifyViewportAtBottom()
	}
}

// scrollToFocusedField scrolls the viewport so the active step's focused
// field sits entirely on screen; a field taller than the viewport is pinned
// to its first row.
func (m *Model) scrollToFocusedField() {
	if len(m.steps) == 0 || m.currentStep < 0 || m.currentStep >= len(m.steps) {
		return
	}
	step := m.steps[m.currentStep]
	// contentRows maps the step's own View lines; a centered step's content is
	// shifted by PaddingTop before it is measured, so the spans would not line
	// up. No centered step provides spans today.
	if c, ok := step.(centerable); ok && c.IsCentered() {
		return
	}
	provider, ok := step.(SpanProvider)
	if !ok {
		return
	}
	span, ok := provider.FocusedSpan()
	if !ok {
		return
	}

	start, end := m.viewportSpan(span)
	top, height := m.viewport.YOffset(), m.viewport.Height()
	spanHeight := end - start + 1

	switch {
	case spanHeight > height, start < top:
		m.viewport.SetYOffset(start)
	case end >= top+height:
		m.viewport.SetYOffset(end - height + 1)
	}
}

// viewportSpan maps a step-View line span onto the viewport rows it occupies
// after content padding wrapped any over-wide lines.
func (m *Model) viewportSpan(span LineSpan) (start, end int) {
	rows := m.contentRows
	if len(rows) < 2 {
		return 0, 0
	}
	last := len(rows) - 1
	start = rows[min(max(span.Start, 0), last)]
	end = rows[min(max(span.End+1, 0), last)] - 1
	return start, max(end, start)
}

// autoScrollToField scrolls by the focused field's exact pixel bounds
// (FocusedBounds) rather than scrollToFocusedField's line-span mapping —
// kept alongside it since not every focusable (e.g. lifecycle's
// components.Input/Selector-backed fields) implements SpanProvider.
func (m *Model) autoScrollToField(_, _ int) {
	step := m.CurrentStep()
	bounded, ok := step.(FocusedBounds)
	if !ok {
		return
	}
	top, bottom, ok := bounded.FocusBounds(max(40, m.contentWidth()-4), 1000)
	if !ok {
		return
	}
	if titled, ok := step.(displayTitler); ok && titled.DisplayTitle() != "" {
		offset := lipgloss.Height(m.renderStepTitle(titled.DisplayTitle())) + 1
		top += offset
		bottom += offset
	}
	offset := m.viewport.YOffset()
	if top < offset || bottom-top > m.viewport.Height() {
		m.viewport.SetYOffset(top)
	} else if bottom > offset+m.viewport.Height() {
		m.viewport.SetYOffset(bottom - m.viewport.Height())
	}
}

func (m *Model) goToNextStep() (tea.Model, tea.Cmd) {
	if len(m.steps) > 0 && m.currentStep < len(m.steps) {
		if a, ok := m.steps[m.currentStep].(ConfigApplier); ok {
			if err := a.Apply(m.config); err != nil {
				m.err = err
				return m, func() tea.Msg { return ErrorSetMsg{Error: err} }
			}
		}

		if ee, ok := m.steps[m.currentStep].(earlyExiter); ok {
			if ee.ShouldExitEarly() {
				action := ActionExit
				if ag, ok := m.steps[m.currentStep].(actionGetter); ok {
					action = ag.GetSelectedAction()
				}
				m.result = Result{
					Outcome:  OutcomeCompleted,
					Config:   m.config,
					Action:   action,
					ExitStep: m.steps[m.currentStep].ID(),
				}
				return m, tea.Quit
			}
		}

		if f, ok := m.steps[m.currentStep].(FocusableStep); ok {
			f.SetFocused(false)
		}

		if m.returnToReview {
			return m.jumpToReview()
		}
	}

	nextStep := m.currentStep + 1
	for nextStep < len(m.steps) {
		if stepShouldShow(m.steps[nextStep], m.config) {
			break
		}
		nextStep++
	}

	if nextStep >= len(m.steps) {
		action := ActionExit
		var exitStep StepID
		if len(m.steps) > 0 {
			currentStep := m.steps[m.currentStep]
			exitStep = currentStep.ID()
			if ag, ok := currentStep.(actionGetter); ok {
				action = ag.GetSelectedAction()
			}
		}
		m.result = Result{
			Outcome:  OutcomeCompleted,
			Config:   m.config,
			Action:   action,
			ExitStep: exitStep,
		}
		return m, tea.Quit
	}

	return m.focusStep(nextStep)
}

func (m *Model) goToPreviousStep() (tea.Model, tea.Cmd) {
	if m.currentStep == 0 {
		return m, nil
	}

	if len(m.steps) > 0 && m.currentStep < len(m.steps) {
		if f, ok := m.steps[m.currentStep].(FocusableStep); ok {
			f.SetFocused(false)
		}
	}

	if m.returnToReview {
		return m.jumpToReview()
	}

	prevStep := m.currentStep - 1
	for prevStep >= 0 {
		step := m.steps[prevStep]
		if stepShouldShow(step, m.config) && !stepAutoCompletes(step) {
			break
		}
		prevStep--
	}

	if prevStep < 0 {
		// Every earlier step is hidden or auto-completing; landing on a
		// screen ShouldShow hides would strand the user, so stay put.
		return m, nil
	}

	return m.focusStep(prevStep)
}

// jumpToReview clears returnToReview and focuses the review step directly,
// skipping intermediate steps.
func (m *Model) jumpToReview() (tea.Model, tea.Cmd) {
	m.returnToReview = false

	idx := m.indexOfStepByID(StepIDReview)
	if idx < 0 {
		idx = m.currentStep
	}

	return m.focusStep(idx)
}

// jumpToStep applies/blurs the current step, focuses id, and arms
// returnToReview so confirm/back returns here.
func (m *Model) jumpToStep(id StepID) (tea.Model, tea.Cmd) {
	idx := m.indexOfStepByID(id)
	if idx < 0 || !stepShouldShow(m.steps[idx], m.config) {
		return m, nil
	}

	if len(m.steps) > 0 && m.currentStep < len(m.steps) {
		if a, ok := m.steps[m.currentStep].(ConfigApplier); ok {
			if err := a.Apply(m.config); err != nil {
				m.err = err
				return m, func() tea.Msg { return ErrorSetMsg{Error: err} }
			}
		}
		if f, ok := m.steps[m.currentStep].(FocusableStep); ok {
			f.SetFocused(false)
		}
	}

	m.returnToReview = true

	return m.focusStep(idx)
}

func (m *Model) indexOfStepByID(id StepID) int {
	for i, s := range m.steps {
		if s.ID() == id {
			return i
		}
	}
	return -1
}

// focusStep is the shared tail of every step transition: resize, focus, refresh
// jump targets, resync viewport. It is also the single point every
// navigation-mutating message (StepCompleteMsg, StepBackMsg, SwapFlowMsg,
// JumpToStepMsg) funnels through, so it closes the help
// overlay first — the overlay gates only tea.KeyPressMsg, so one of those
// messages can otherwise arrive asynchronously while it is open and move the
// current step (or replace the whole step set, on SwapFlow) underneath it,
// leaving it rendering bindings for a step the user never navigated to.
func (m *Model) focusStep(idx int) (tea.Model, tea.Cmd) {
	m.helpOpen = false
	m.currentStep = idx
	m.beginVisit()

	m.sizeCurrentStep()
	if f, ok := m.steps[idx].(FocusableStep); ok {
		f.SetFocused(true)
	}
	// Init() is called unconditionally on every entry, including
	// re-entry while a step's own prior fetch is still in flight: a step
	// with an async fetch owns its own request identity (a generation
	// counter bumped per issue) and its own reuse-vs-refetch call, so a
	// superseded reply is simply discarded rather than clobbering newer
	// state, and re-entry never needs to be gated here.
	cmd := m.ownCommand(m.steps[idx].Init())

	m.syncJumpTargets()

	if m.ready {
		m.resizeViewport()
		m.viewport.GotoTop()
		m.syncViewportContent()
		m.notifyIfAtBottom()
	}

	m.autoScrollToField(0, 0)
	return m, cmd
}

// syncJumpTargets refreshes the review step's digit-jump table, compacting out
// hidden (ShouldShow false) targets.
func (m *Model) syncJumpTargets() {
	rj, ok := m.steps[m.currentStep].(ReviewJumper)
	if !ok {
		return
	}

	order := rj.JumpOrder()
	targets := make([]JumpTarget, 0, len(order))
	digit := 1
	for _, id := range order {
		// Digits are single keystrokes; nine is the practical ceiling.
		if digit > 9 {
			break
		}
		idx := m.indexOfStepByID(id)
		if idx < 0 || !stepShouldShow(m.steps[idx], m.config) {
			continue
		}
		targets = append(targets, JumpTarget{StepID: id, Digit: digit})
		digit++
	}

	rj.SetJumpTargets(targets)
}

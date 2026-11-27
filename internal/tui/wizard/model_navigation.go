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
	// Deliberately no re-scroll: a resize must not overwrite a scroll position
	// the operator just set, and render helpers re-send same-size resizes.
}

// sizeCurrentStep hands the step the terminal dimensions, then the content
// box; a TerminalSizer's gate is stated in terminal columns the frame's caps
// flatten.
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

// resizeViewport re-fits the viewport to the body width and height budget;
// a frameWidthOwner changes the body width without a terminal resize.
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

// handleScrollKey scrolls on page/home/end and the vim keys, reporting
// whether it consumed msg; home/end and the vim keys fall through to a
// focused text input.
func (m *Model) handleScrollKey(msg tea.KeyPressMsg) bool {
	if !m.ready {
		return false
	}
	if handled, consumed := m.handleVimScrollKey(msg); handled {
		return consumed
	}
	switch {
	case key.Matches(msg, m.keyMap.PageUp):
		m.viewport.HalfPageUp()
	case key.Matches(msg, m.keyMap.PageDown):
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
	default:
		return false
	}
	return true
}

// handleVimScrollKey handles ctrl+d/u and gg/G; handled reports a vim key,
// consumed whether it acted or armed the gg chord.
func (m *Model) handleVimScrollKey(msg tea.KeyPressMsg) (handled, consumed bool) {
	pendingG := m.pendingG
	m.pendingG = false
	switch {
	case key.Matches(msg, m.keyMap.VimHalfDown), key.Matches(msg, m.keyMap.VimHalfUp):
		if m.currentStepConsumesTextInput() {
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

// scrollToFocusedField brings the focused field's span on screen, pinning a
// field taller than the viewport to its first row.
func (m *Model) scrollToFocusedField() {
	if len(m.steps) == 0 || m.currentStep < 0 || m.currentStep >= len(m.steps) {
		return
	}
	step := m.steps[m.currentStep]
	// A centered step's content is shifted by PaddingTop before measuring, so
	// its spans would not line up.
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

// viewportSpan maps a View line span onto the viewport rows it occupies
// after wrapping.
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

// autoScrollToField scrolls by FocusedBounds for steps without SpanProvider.
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

		if ee, ok := m.steps[m.currentStep].(earlyExiter); ok && ee.ShouldExitEarly() {
			// Leaving through the hub would otherwise drop an edit session silently.
			if !m.saved.matches(m.config, m.steps) {
				m.discardPending = true
				return m, nil
			}
			m.result = Result{
				Outcome:  OutcomeCompleted,
				Config:   m.config,
				Action:   ActionExit,
				ExitStep: m.steps[m.currentStep].ID(),
			}
			return m, tea.Quit
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
		var exitStep StepID
		if len(m.steps) > 0 {
			exitStep = m.steps[m.currentStep].ID()
		}
		m.result = Result{
			Outcome:  OutcomeCompleted,
			Config:   m.config,
			Action:   ActionExit,
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

	// A jumped-to step has already written every keystroke into the config, so
	// leaving it invalid would hand review a config it cannot save.
	if v, ok := m.steps[m.currentStep].(interface{ Validate() error }); ok && m.returnToReview {
		if err := v.Validate(); err != nil {
			m.setError(err)
			return m, nil
		}
	}

	if f, ok := m.steps[m.currentStep].(FocusableStep); ok {
		f.SetFocused(false)
	}

	if m.returnToReview {
		return m.jumpToReview()
	}

	prevStep := m.currentStep - 1
	for prevStep >= 0 {
		if stepShouldShow(m.steps[prevStep], m.config) {
			break
		}
		prevStep--
	}

	if prevStep < 0 {
		// Every earlier step is hidden; stay put rather than land on one.
		return m, nil
	}

	return m.focusStep(prevStep)
}

// jumpToReview focuses the review step directly.
func (m *Model) jumpToReview() (tea.Model, tea.Cmd) {
	m.returnToReview = false

	idx := m.indexOfStepByID(StepIDReview)
	if idx < 0 {
		idx = m.currentStep
	}

	return m.focusStep(idx)
}

// jumpToStep applies the current step, focuses id and arms returnToReview.
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

// focusStep is the shared tail of every transition. It closes the help
// overlay first: a navigation message can arrive while the overlay is open
// and move the step underneath it.
func (m *Model) focusStep(idx int) (tea.Model, tea.Cmd) {
	m.helpOpen = false
	m.currentStep = idx
	m.beginVisit()

	m.sizeCurrentStep()
	if f, ok := m.steps[idx].(FocusableStep); ok {
		f.SetFocused(true)
	}
	// Init runs on every entry; a step with an async fetch owns its generation
	// counter and discards superseded replies, so re-entry needs no gate here.
	cmd := m.ownCommand(m.steps[idx].Init())

	m.syncJumpTargets()

	if m.ready {
		m.resizeViewport()
		m.viewport.GotoTop()
		m.syncViewportContent()
	}

	m.autoScrollToField(0, 0)
	return m, cmd
}

// syncJumpTargets refreshes the review step's digit table, skipping hidden
// steps.
func (m *Model) syncJumpTargets() {
	rj, ok := m.steps[m.currentStep].(ReviewJumper)
	if !ok {
		return
	}

	order := rj.JumpOrder()
	targets := make([]JumpTarget, 0, len(order))
	digit := 1
	for _, id := range order {
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

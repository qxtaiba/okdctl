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

	viewportWidth, viewportHeight := m.viewportDimensions()

	if !m.ready {
		m.viewport = viewport.New(viewport.WithWidth(viewportWidth), viewport.WithHeight(viewportHeight))
		m.ready = true
	} else {
		m.viewport.SetWidth(viewportWidth)
		m.viewport.SetHeight(viewportHeight)
	}

	if len(m.steps) > 0 && m.currentStep < len(m.steps) {
		contentWidth, contentHeight := m.contentDimensions()
		if r, ok := m.steps[m.currentStep].(ResizableStep); ok {
			r.SetSize(contentWidth, contentHeight)
		}
	}
	m.syncViewportContent()
	m.autoScrollToField(0, 0)
}

func (m *Model) handleScrollKey(msg tea.KeyPressMsg) bool {
	if !m.ready {
		return false
	}
	switch {
	case key.Matches(msg, m.keyMap.PageUp):
		m.viewport.HalfPageUp()
	case key.Matches(msg, m.keyMap.PageDown):
		m.viewport.HalfPageDown()
	case key.Matches(msg, m.keyMap.Home):
		m.viewport.GotoTop()
	case key.Matches(msg, m.keyMap.End):
		m.viewport.GotoBottom()
	default:
		return false
	}
	return true
}

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
					Outcome: OutcomeCompleted,
					Config:  m.config,
					Action:  action,
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
		if len(m.steps) > 0 {
			currentStep := m.steps[m.currentStep]
			if ag, ok := currentStep.(actionGetter); ok {
				action = ag.GetSelectedAction()
			}
		}
		m.result = Result{
			Outcome: OutcomeCompleted,
			Config:  m.config,
			Action:  action,
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
		prevStep = 0
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
// jump targets, resync viewport.
func (m *Model) focusStep(idx int) (tea.Model, tea.Cmd) {
	m.currentStep = idx
	m.beginVisit()

	contentWidth, contentHeight := m.contentDimensions()
	if r, ok := m.steps[idx].(ResizableStep); ok {
		r.SetSize(contentWidth, contentHeight)
	}
	if f, ok := m.steps[idx].(FocusableStep); ok {
		f.SetFocused(true)
	}

	cmd := m.ownCommand(m.steps[idx].Init())
	m.syncJumpTargets()

	if m.ready {
		m.viewport.GotoTop()
		m.syncViewportContent()
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

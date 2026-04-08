package wizard

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleResize(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height

	viewportWidth, viewportHeight := m.viewportDimensions()

	if !m.ready {
		m.viewport = viewport.New(viewportWidth, viewportHeight)
		m.viewport.MouseWheelEnabled = false
		m.viewport.YPosition = 0
		m.ready = true
	} else {
		m.viewport.Width = viewportWidth
		m.viewport.Height = viewportHeight
	}

	if len(m.steps) > 0 && m.currentStep < len(m.steps) {
		contentWidth, contentHeight := m.contentDimensions()
		if r, ok := m.steps[m.currentStep].(ResizableStep); ok {
			r.SetSize(contentWidth, contentHeight)
		}
	}
	m.syncViewportContent()
}

func (m *Model) handleScrollKey(msg tea.KeyMsg) bool {
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

// autoScrollToField uses percentage-based scrolling relative to field progress
// through the form to keep the focused field visible in the viewport.
func (m *Model) autoScrollToField(fieldIndex, totalFields int) {
	if totalFields == 0 {
		return
	}

	totalContent := m.viewport.TotalLineCount()
	viewportHeight := m.viewport.Height

	if totalContent <= viewportHeight {
		return
	}

	var progress float64
	if totalFields <= 1 {
		progress = 0
	} else {
		progress = float64(fieldIndex) / float64(totalFields-1)
	}

	maxOffset := totalContent - viewportHeight
	targetOffset := int(progress * float64(maxOffset))
	if targetOffset < 0 {
		targetOffset = 0
	}
	if targetOffset > maxOffset {
		targetOffset = maxOffset
	}

	m.viewport.SetYOffset(targetOffset)
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
					Completed: true,
					Config:    m.config,
					Action:    action,
				}
				return m, tea.Quit
			}
		}

		if f, ok := m.steps[m.currentStep].(FocusableStep); ok {
			f.SetFocused(false)
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
			Completed: true,
			Config:    m.config,
			Action:    action,
		}
		return m, tea.Quit
	}

	m.currentStep = nextStep

	contentWidth, contentHeight := m.contentDimensions()
	if r, ok := m.steps[m.currentStep].(ResizableStep); ok {
		r.SetSize(contentWidth, contentHeight)
	}
	if f, ok := m.steps[m.currentStep].(FocusableStep); ok {
		f.SetFocused(true)
	}

	if m.ready {
		m.viewport.GotoTop()
		m.syncViewportContent()
	}

	return m, m.steps[m.currentStep].Init()
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

	m.currentStep = prevStep

	contentWidth, contentHeight := m.contentDimensions()
	if r, ok := m.steps[m.currentStep].(ResizableStep); ok {
		r.SetSize(contentWidth, contentHeight)
	}
	if f, ok := m.steps[m.currentStep].(FocusableStep); ok {
		f.SetFocused(true)
	}

	if m.ready {
		m.viewport.GotoTop()
		m.syncViewportContent()
	}

	return m, m.steps[m.currentStep].Init()
}

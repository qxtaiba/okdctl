package wizard

import (
	"slices"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

const paletteResultLimit = 8

type paletteMatch struct {
	stepIndex int
	target    PaletteTarget
	score     int
}

func (m *Model) openPalette() {
	m.paletteOpen = true
	m.paletteQuery = ""
	m.paletteSelected = 0
	m.refreshPaletteMatches()
}

func (m *Model) closePalette() {
	m.paletteOpen = false
	m.paletteQuery = ""
	m.paletteMatches = nil
	m.paletteSelected = 0
}

func (m *Model) refreshPaletteMatches() {
	m.paletteMatches = nil
	query := strings.TrimSpace(m.paletteQuery)
	for i, step := range m.steps {
		if !stepShouldShow(step, m.config) {
			continue
		}
		stepID := step.ID()
		stepTarget := PaletteTarget{ID: string(stepID), Kind: "step", Label: step.Title(), Detail: "Go to step"}
		if score, ok := paletteScore(query, stepTarget.Label); ok {
			m.paletteMatches = append(m.paletteMatches, paletteMatch{stepIndex: i, target: stepTarget, score: score})
		}
		provider, ok := step.(PaletteProvider)
		if !ok {
			continue
		}
		for _, target := range provider.PaletteTargets() {
			text := target.Label + " " + target.Detail + " " + target.Kind
			if score, ok := paletteScore(query, text); ok {
				m.paletteMatches = append(m.paletteMatches, paletteMatch{stepIndex: i, target: target, score: score})
			}
		}
	}
	slices.SortStableFunc(m.paletteMatches, func(a, b paletteMatch) int {
		if a.score < b.score {
			return -1
		}
		if a.score > b.score {
			return 1
		}
		return 0
	})
	if m.paletteSelected >= len(m.paletteMatches) {
		m.paletteSelected = max(0, len(m.paletteMatches)-1)
	}
}

func paletteScore(query, label string) (int, bool) {
	query = strings.ToLower(strings.Join(strings.Fields(query), " "))
	label = strings.ToLower(strings.Join(strings.Fields(label), " "))
	if query == "" {
		return 0, true
	}
	if label == query {
		return 0, true
	}
	if strings.HasPrefix(label, query) {
		return 1, true
	}
	if strings.Contains(label, query) {
		return 2, true
	}
	if orderedRunes(label, query) {
		return 3, true
	}
	return 0, false
}

func orderedRunes(label, query string) bool {
	for _, wanted := range query {
		index := strings.IndexRune(label, wanted)
		if index < 0 {
			return false
		}
		label = label[index+len(string(wanted)):]
	}
	return true
}

func (m *Model) handlePaletteKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.Code {
	case tea.KeyEscape:
		m.closePalette()
		return m, nil, true
	case tea.KeyEnter:
		if len(m.paletteMatches) > 0 {
			return m.activatePaletteMatch(m.paletteMatches[m.paletteSelected])
		}
	case tea.KeyUp:
		if m.paletteSelected > 0 {
			m.paletteSelected--
		}
		return m, nil, true
	case tea.KeyDown:
		if m.paletteSelected+1 < len(m.paletteMatches) {
			m.paletteSelected++
		}
		return m, nil, true
	case tea.KeyBackspace, tea.KeyDelete:
		if len(m.paletteQuery) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.paletteQuery)
			m.paletteQuery = m.paletteQuery[:len(m.paletteQuery)-size]
			m.paletteSelected = 0
			m.refreshPaletteMatches()
		}
		return m, nil, true
	}
	if msg.Text != "" && msg.Mod == 0 {
		m.paletteQuery += msg.Text
		m.paletteSelected = 0
		m.refreshPaletteMatches()
	}
	return m, nil, true
}

func (m *Model) activatePaletteMatch(match paletteMatch) (tea.Model, tea.Cmd, bool) {
	m.closePalette()
	if match.stepIndex < 0 || match.stepIndex >= len(m.steps) {
		return m, nil, true
	}
	step := m.steps[match.stepIndex]
	if match.target.Kind == "step" {
		returnToReview := m.CurrentStep().ID() == StepIDReview
		model, cmd := m.jumpToStep(step.ID())
		if !returnToReview {
			model.(*Model).returnToReview = false
		}
		return model, cmd, true
	}
	provider, ok := step.(PaletteProvider)
	if !ok {
		return m, nil, true
	}
	if match.stepIndex != m.currentStep {
		returnToReview := m.CurrentStep().ID() == StepIDReview
		model, initCmd := m.jumpToStep(step.ID())
		focused := model.(*Model)
		if !returnToReview {
			focused.returnToReview = false
		}
		cmd := provider.FocusPaletteTarget(match.target.ID)
		focused.syncViewportContent()
		focused.scrollToFocusedField()
		return focused, tea.Batch(initCmd, cmd), true
	}
	cmd := provider.FocusPaletteTarget(match.target.ID)
	m.syncViewportContent()
	m.scrollToFocusedField()
	return m, cmd, true
}

func (m *Model) renderPalette() string {
	width := m.contentWidth()
	height := m.viewport.Height()
	panelWidth := min(width, 76)
	if panelWidth < 24 {
		panelWidth = width
	}
	inner := max(panelWidth-6, 1)
	queryStyle := lipgloss.NewStyle().Foreground(tui.ColorText()).Bold(true)
	detailStyle := lipgloss.NewStyle().Foreground(tui.ColorTextDim())
	selectedStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true)
	lines := []string{queryStyle.Render("Jump to"), "  " + m.paletteQuery + tui.IconTextCursor, ""}
	visible := min(len(m.paletteMatches), paletteResultLimit)
	for i := range visible {
		match := m.paletteMatches[i]
		label := match.target.Label
		if match.target.Kind == "step" {
			label = match.target.Label
		} else {
			label = match.target.Label + "  ·  " + match.target.Detail
		}
		label = lipgloss.NewStyle().MaxWidth(inner - 4).Render(label)
		if i == m.paletteSelected {
			lines = append(lines, selectedStyle.Render("› "+label))
		} else {
			lines = append(lines, detailStyle.Render("  "+label))
		}
	}
	if len(m.paletteMatches) == 0 {
		lines = append(lines, detailStyle.Render("  No matching destinations"))
	}
	lines = append(lines, "", detailStyle.Render("↑↓ select  enter open  esc close"))
	body := strings.Join(lines, "\n")
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorRule()).
		Padding(1, 2).
		Width(panelWidth).
		Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)
}

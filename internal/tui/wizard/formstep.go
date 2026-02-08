package wizard

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard/components"
)

type FormSection struct {
	Title string
	Note  string // Optional hint rendered below the title
	Group *components.InputGroup
}

func (s *FormSection) IsComplete() bool {
	if s.Group == nil {
		return false
	}
	for _, field := range s.Group.Fields() {
		if field.Value() == "" {
			return false
		}
	}
	return s.Group.IsValid()
}

type MultiFormStep struct {
	BaseStep

	sections       []FormSection
	currentSection int

	validateFn   func() error
	applyFn      func(*config.Config) error
	extraContent func(width int) string
	shouldShowFn func(*config.Config) bool
}

func NewMultiFormStep(id StepID, title, displayTitle, description string) *MultiFormStep {
	return &MultiFormStep{
		BaseStep: NewBaseStepWithDisplayTitle(id, title, displayTitle, description),
		sections: make([]FormSection, 0),
	}
}

func (s *MultiFormStep) AddSection(title string, group *components.InputGroup) *MultiFormStep {
	s.sections = append(s.sections, FormSection{Title: title, Group: group})
	return s
}

func (s *MultiFormStep) AddSectionWithNote(title, note string, group *components.InputGroup) *MultiFormStep {
	s.sections = append(s.sections, FormSection{Title: title, Note: note, Group: group})
	return s
}

func (s *MultiFormStep) WithValidation(fn func() error) *MultiFormStep {
	s.validateFn = fn
	return s
}

func (s *MultiFormStep) WithApply(fn func(*config.Config) error) *MultiFormStep {
	s.applyFn = fn
	return s
}

func (s *MultiFormStep) WithExtraContent(fn func(width int) string) *MultiFormStep {
	s.extraContent = fn
	return s
}

func (s *MultiFormStep) WithShouldShow(fn func(*config.Config) bool) *MultiFormStep {
	s.shouldShowFn = fn
	return s
}

func (s *MultiFormStep) Section(index int) *FormSection {
	if index >= 0 && index < len(s.sections) {
		return &s.sections[index]
	}
	return nil
}

func (s *MultiFormStep) CurrentSectionIndex() int {
	return s.currentSection
}

func (s *MultiFormStep) Init() tea.Cmd {
	if len(s.sections) > 0 {
		return s.sections[0].Group.Focus()
	}
	return nil
}

func (s *MultiFormStep) Update(msg tea.Msg) (WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if err := s.Validate(); err != nil {
				return s, nil
			}
			return s, func() tea.Msg {
				return StepCompleteMsg{StepID: s.ID()}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab", "down"))):
			group := s.sections[s.currentSection].Group
			currentIndex := group.FocusIndex()
			isLastField := currentIndex >= len(group.Fields())-1
			isLastSection := s.currentSection >= len(s.sections)-1

			if isLastField && isLastSection {
				return s, nil
			}

			if isLastField {
				group.Blur()
				s.currentSection++
				nextGroup := s.sections[s.currentSection].Group
				nextGroup.SetFocusIndex(0)
				cmd := nextGroup.Focus()
				return s, tea.Batch(cmd, s.emitFocusChanged())
			}

			var cmd tea.Cmd
			s.sections[s.currentSection].Group, cmd = group.Update(msg)
			return s, tea.Batch(cmd, s.emitFocusChanged())

		case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab", "up"))):
			group := s.sections[s.currentSection].Group
			isFirstField := group.FocusIndex() == 0
			isFirstSection := s.currentSection == 0

			if isFirstField && isFirstSection {
				return s, nil
			}

			if isFirstField {
				group.Blur()
				s.currentSection--
				prevGroup := s.sections[s.currentSection].Group
				prevGroup.SetFocusIndex(len(prevGroup.Fields()) - 1)
				cmd := prevGroup.Focus()
				return s, tea.Batch(cmd, s.emitFocusChanged())
			}

			var cmd tea.Cmd
			s.sections[s.currentSection].Group, cmd = group.Update(msg)
			return s, tea.Batch(cmd, s.emitFocusChanged())

		default:
			var cmd tea.Cmd
			s.sections[s.currentSection].Group, cmd = s.sections[s.currentSection].Group.Update(msg)
			return s, cmd
		}
	}

	var cmd tea.Cmd
	if len(s.sections) > 0 {
		s.sections[s.currentSection].Group, cmd = s.sections[s.currentSection].Group.Update(msg)
	}
	return s, cmd
}

func (s *MultiFormStep) emitFocusChanged() tea.Cmd {
	globalIndex := 0
	totalFields := 0

	for i := 0; i < s.currentSection; i++ {
		globalIndex += len(s.sections[i].Group.Fields())
	}
	if s.currentSection < len(s.sections) {
		globalIndex += s.sections[s.currentSection].Group.FocusIndex()
	}

	for _, section := range s.sections {
		totalFields += len(section.Group.Fields())
	}

	return func() tea.Msg {
		return FocusChangedMsg{
			FieldIndex:  globalIndex,
			TotalFields: totalFields,
		}
	}
}

func (s *MultiFormStep) View(width, height int) string {
	s.SetSize(width, height)

	innerWidth := width - 4
	if innerWidth < 40 {
		innerWidth = 40
	}

	var content strings.Builder

	sectionHeader := lipgloss.NewStyle().
		Foreground(tui.ColorCyan500).
		Bold(true)

	activeSectionStyle := lipgloss.NewStyle().
		Padding(1, 2)

	inactiveSectionStyle := lipgloss.NewStyle().
		Padding(1, 2)

	completedIndicator := lipgloss.NewStyle().
		Foreground(tui.ColorSuccess).
		Bold(true).
		Render("✓")

	activeIndicator := lipgloss.NewStyle().
		Foreground(tui.ColorPrimary).
		Bold(true).
		Render("●")

	pendingIndicator := lipgloss.NewStyle().
		Foreground(tui.ColorSlate600).
		Render("○")

	for i, section := range s.sections {
		section.Group.SetWidth(innerWidth)

		var style lipgloss.Style
		var indicator string

		if i == s.currentSection {
			style = activeSectionStyle
			indicator = activeIndicator
		} else if section.IsComplete() {
			style = inactiveSectionStyle
			indicator = completedIndicator
		} else {
			style = inactiveSectionStyle
			indicator = pendingIndicator
		}

		sectionTitle := indicator + " " + sectionHeader.Render(strings.ToLower(section.Title))
		var sectionContent string
		if section.Note != "" {
			noteStyle := lipgloss.NewStyle().
				Foreground(tui.ColorSlate500).
				Italic(true).
				PaddingLeft(2)
			sectionContent = sectionTitle + "\n" + noteStyle.Render(section.Note) + "\n\n" + section.Group.ViewCompact("")
		} else {
			sectionContent = sectionTitle + "\n\n" + section.Group.ViewCompact("")
		}
		content.WriteString(style.Render(sectionContent))
	}

	if s.extraContent != nil {
		content.WriteString(s.extraContent(width))
	}

	return content.String()
}

func (s *MultiFormStep) Validate() error {
	var firstErr error
	for _, section := range s.sections {
		if errs := section.Group.Validate(); len(errs) > 0 && firstErr == nil {
			firstErr = errs[0]
		}
	}
	if firstErr != nil {
		return firstErr
	}
	if s.validateFn != nil {
		return s.validateFn()
	}
	return nil
}

func (s *MultiFormStep) Apply(cfg *config.Config) error {
	if s.applyFn != nil {
		return s.applyFn(cfg)
	}
	return nil
}

func (s *MultiFormStep) ShouldShow(cfg *config.Config) bool {
	if s.shouldShowFn != nil {
		return s.shouldShowFn(cfg)
	}
	return true
}

func (s *MultiFormStep) ShortHelp() []KeyBinding {
	return []KeyBinding{
		{Key: "↑↓/tab", Help: "navigate"},
		{Key: "enter", Help: "continue"},
		{Key: "esc", Help: "back"},
	}
}

func (s *MultiFormStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if focused {
		s.currentSection = 0
		if len(s.sections) > 0 {
			_ = s.sections[0].Group.Focus() // Command executed during Init()
		}
	} else {
		for _, section := range s.sections {
			section.Group.Blur()
		}
	}
}

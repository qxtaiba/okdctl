package wizard

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
		if err := field.Validate(); err != nil {
			return false
		}
	}
	return true
}

type MultiFormStep struct {
	BaseStep

	sections       []FormSection
	currentSection int

	validateFn   func() error
	applyFn      func(*config.Config) error
	extraContent func(width int) string
	shouldShowFn func(*config.Config) bool

	// totalFieldsCache is the summed field count across all sections, used by
	// emitFocusChanged. -1 means "not yet computed"; it is invalidated in
	// AddSection/AddSectionWithNote whenever the section list mutates.
	totalFieldsCache int
}

func NewMultiFormStep(id StepID, title, displayTitle, description string) *MultiFormStep {
	return &MultiFormStep{
		BaseStep:         NewBaseStepWithDisplayTitle(id, title, displayTitle, description),
		sections:         make([]FormSection, 0),
		totalFieldsCache: -1,
	}
}

func (s *MultiFormStep) AddSection(title string, group *components.InputGroup) *MultiFormStep {
	s.sections = append(s.sections, FormSection{Title: title, Group: group})
	s.totalFieldsCache = -1
	return s
}

func (s *MultiFormStep) AddSectionWithNote(title, note string, group *components.InputGroup) *MultiFormStep {
	s.sections = append(s.sections, FormSection{Title: title, Note: note, Group: group})
	s.totalFieldsCache = -1
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

// currentGroup returns the Group of the currently-active section, or nil if
// the index is out of range or the section has no Group. Callers must handle
// the nil case.
func (s *MultiFormStep) currentGroup() *components.InputGroup {
	if s.currentSection < 0 || s.currentSection >= len(s.sections) {
		return nil
	}
	return s.sections[s.currentSection].Group
}

func (s *MultiFormStep) Init() tea.Cmd {
	if len(s.sections) > 0 && s.sections[0].Group != nil {
		return s.sections[0].Group.Focus()
	}
	return nil
}

func (s *MultiFormStep) Update(msg tea.Msg) (WizardStep, tea.Cmd) {
	group := s.currentGroup()
	if group == nil {
		return s, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if err := s.Validate(); err != nil {
				return s, nil
			}
			return s, func() tea.Msg {
				return StepCompleteMsg{StepID: s.ID()}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab", "down"))):
			currentIndex := group.FocusIndex()
			isLastField := currentIndex >= len(group.Fields())-1
			isLastSection := s.currentSection >= len(s.sections)-1

			if isLastField && isLastSection {
				return s, nil
			}

			if isLastField {
				group.Blur()
				s.currentSection++
				nextGroup := s.currentGroup()
				if nextGroup == nil {
					return s, s.emitFocusChanged()
				}
				nextGroup.SetFocusIndex(0)
				cmd := nextGroup.Focus()
				return s, tea.Batch(cmd, s.emitFocusChanged())
			}

			var cmd tea.Cmd
			s.sections[s.currentSection].Group, cmd = group.Update(msg)
			return s, tea.Batch(cmd, s.emitFocusChanged())

		case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab", "up"))):
			isFirstField := group.FocusIndex() == 0
			isFirstSection := s.currentSection == 0

			if isFirstField && isFirstSection {
				return s, nil
			}

			if isFirstField {
				group.Blur()
				s.currentSection--
				prevGroup := s.currentGroup()
				if prevGroup == nil {
					return s, s.emitFocusChanged()
				}
				prevGroup.SetFocusIndex(len(prevGroup.Fields()) - 1)
				cmd := prevGroup.Focus()
				return s, tea.Batch(cmd, s.emitFocusChanged())
			}

			var cmd tea.Cmd
			s.sections[s.currentSection].Group, cmd = group.Update(msg)
			return s, tea.Batch(cmd, s.emitFocusChanged())

		}
	}

	var cmd tea.Cmd
	s.sections[s.currentSection].Group, cmd = group.Update(msg)
	return s, cmd
}

func (s *MultiFormStep) emitFocusChanged() tea.Cmd {
	globalIndex := 0

	for i := range s.currentSection {
		if s.sections[i].Group == nil {
			continue
		}
		globalIndex += len(s.sections[i].Group.Fields())
	}
	if current := s.currentGroup(); current != nil {
		globalIndex += current.FocusIndex()
	}

	if s.totalFieldsCache < 0 {
		total := 0
		for _, section := range s.sections {
			if section.Group == nil {
				continue
			}
			total += len(section.Group.Fields())
		}
		s.totalFieldsCache = total
	}
	totalFields := s.totalFieldsCache

	return func() tea.Msg {
		return FocusChangedMsg{
			FieldIndex:  globalIndex,
			TotalFields: totalFields,
		}
	}
}

// formViewStyles holds pre-computed lipgloss styles for MultiFormStep.View.
// Caching is safe because tui.Color* values are set once during package init
// and never change.
var formViewStyles = struct {
	sectionHeader    lipgloss.Style
	activeSection    lipgloss.Style
	inactiveSection  lipgloss.Style
	completedRender  string
	activeRender     string
	pendingRender    string
	note             lipgloss.Style
}{
	sectionHeader: lipgloss.NewStyle().
		Foreground(tui.ColorCyan500).
		Bold(true),
	activeSection: lipgloss.NewStyle().
		Padding(1, 2),
	inactiveSection: lipgloss.NewStyle().
		Padding(1, 2),
	completedRender: lipgloss.NewStyle().
		Foreground(tui.ColorSuccess).
		Bold(true).
		Render("✓"),
	activeRender: lipgloss.NewStyle().
		Foreground(tui.ColorPrimary).
		Bold(true).
		Render("●"),
	pendingRender: lipgloss.NewStyle().
		Foreground(tui.ColorSlate600).
		Render("○"),
	note: lipgloss.NewStyle().
		Foreground(tui.ColorSlate500).
		Italic(true).
		PaddingLeft(2),
}

func (s *MultiFormStep) View(width, height int) string {
	s.SetSize(width, height)

	innerWidth := width - 4
	if innerWidth < 40 {
		innerWidth = 40
	}

	var content strings.Builder

	for i, section := range s.sections {
		if section.Group == nil {
			continue
		}
		section.Group.SetWidth(innerWidth)

		var style lipgloss.Style
		var indicator string

		if i == s.currentSection {
			style = formViewStyles.activeSection
			indicator = formViewStyles.activeRender
		} else if section.IsComplete() {
			style = formViewStyles.inactiveSection
			indicator = formViewStyles.completedRender
		} else {
			style = formViewStyles.inactiveSection
			indicator = formViewStyles.pendingRender
		}

		sectionTitle := indicator + " " + formViewStyles.sectionHeader.Render(strings.ToLower(section.Title))
		var sectionContent string
		if section.Note != "" {
			sectionContent = sectionTitle + "\n" + formViewStyles.note.Render(section.Note) + "\n\n" + section.Group.ViewCompact("")
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
		if section.Group == nil {
			continue
		}
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
		if len(s.sections) > 0 && s.sections[0].Group != nil {
			_ = s.sections[0].Group.Focus() // Command executed during Init()
		}
	} else {
		for _, section := range s.sections {
			if section.Group == nil {
				continue
			}
			section.Group.Blur()
		}
	}
}

package wizard

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

type FieldType int

const (
	FieldTypeText FieldType = iota
	FieldTypePassword
	FieldTypeNumber
	FieldTypeBool   // For yes/no fields
	FieldTypeSelect // Dropdown selector with predefined options
)

type (
	ConfigSetter func(cfg *config.Config, value string) error
	ConfigGetter func(cfg *config.Config) string
)

type FieldDefinition struct {
	Key      string
	Label    string
	Default  string
	Help     string
	Type     FieldType
	Options  []string // populated only when Type == FieldTypeSelect
	Required bool
	Validate func(string) error

	ConfigSet ConfigSetter
	ConfigGet ConfigGetter
}

type SectionDefinition struct {
	Title  string
	Note   string // e.g. prerequisites, shown below the title
	Fields []FieldDefinition
}

type StepDefinition struct {
	ID           StepID
	Title        string
	DisplayTitle string
	Description  string
	Sections     []SectionDefinition

	Validate     func(values map[string]string) error
	Apply        func(step *DataDrivenStep, cfg *config.Config) error // Custom apply (after auto-binding)
	ShouldShow   func(*config.Config) bool
	ExtraContent func(values map[string]string, width int) string
}

// formSection is the runtime counterpart to SectionDefinition, pairing a title
// with its built InputGroup.
type formSection struct {
	title string
	note  string
	group *components.InputGroup
}

func (s *formSection) isComplete() bool {
	if s.group == nil {
		return false
	}
	for _, field := range s.group.Fields() {
		if field.Value() == "" {
			return false
		}
		if err := field.Validate(); err != nil {
			return false
		}
	}
	return true
}

type fieldLocation struct {
	section int
	field   int
}

// DataDrivenStep renders a multi-section form built from a StepDefinition and
// implements the WizardStep interface.
type DataDrivenStep struct {
	BaseStep

	definition *StepDefinition
	fieldKeys  map[string]fieldLocation // maps Key -> section/field indices

	sections       []formSection
	currentSection int

	// customExtraContent, when non-nil, overrides definition.ExtraContent.
	// Set via WithExtraContentFunc.
	customExtraContent func(width int) string

	// totalFieldsCache is the summed field count across all sections, used by
	// emitFocusChanged. -1 means "not yet computed".
	totalFieldsCache int
}

func NewDataDrivenStep(def *StepDefinition) *DataDrivenStep {
	step := &DataDrivenStep{
		BaseStep:         NewBaseStepWithDisplayTitle(def.ID, def.Title, def.DisplayTitle, def.Description),
		definition:       def,
		fieldKeys:        make(map[string]fieldLocation),
		sections:         make([]formSection, 0, len(def.Sections)),
		totalFieldsCache: -1,
	}

	for sectionIdx := range def.Sections {
		sectionDef := &def.Sections[sectionIdx]
		fields := make([]components.FormField, 0, len(sectionDef.Fields))

		for fieldIdx := range sectionDef.Fields {
			fieldDef := &sectionDef.Fields[fieldIdx]
			fields = append(fields, buildFormField(fieldDef))
			step.fieldKeys[fieldDef.Key] = fieldLocation{
				section: sectionIdx,
				field:   fieldIdx,
			}
		}

		step.sections = append(step.sections, formSection{
			title: sectionDef.Title,
			note:  sectionDef.Note,
			group: components.NewInputGroup(sectionDef.Title, fields...),
		})
	}

	return step
}

func buildFormField(def *FieldDefinition) components.FormField {
	if def.Type == FieldTypeSelect {
		sf := components.NewSelectField(def.Label, def.Options)
		sf.Help = def.Help
		if def.Default != "" {
			sf.SetDefault(def.Default)
		}
		return sf
	}

	var field *components.InputField
	if def.Type == FieldTypePassword {
		field = components.NewPasswordField(def.Label, def.Default)
	} else {
		field = components.NewInputField(def.Label, def.Default)
	}
	field.Required = def.Required
	field.Help = def.Help
	field.Validator = def.Validate
	return field
}

// getField resolves a field key to its FormField by walking the
// fieldKeys → section → group → field chain, returning nil if any level
// is missing. Callers must handle the nil case. The parameter is named
// fieldKey rather than key to avoid shadowing the bubbles/v2/key import.
func (s *DataDrivenStep) getField(fieldKey string) components.FormField {
	loc, ok := s.fieldKeys[fieldKey]
	if !ok || loc.section < 0 || loc.section >= len(s.sections) {
		return nil
	}
	group := s.sections[loc.section].group
	if group == nil {
		return nil
	}
	return group.Field(loc.field)
}

func (s *DataDrivenStep) Value(fieldKey string) string {
	if field := s.getField(fieldKey); field != nil {
		return field.Value()
	}
	return ""
}

func (s *DataDrivenStep) ValueInt(fieldKey string, fallback int) int {
	v := s.Value(fieldKey)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}

func (s *DataDrivenStep) setValue(fieldKey, value string) {
	loc, ok := s.fieldKeys[fieldKey]
	if !ok || loc.section < 0 || loc.section >= len(s.sections) {
		return
	}
	group := s.sections[loc.section].group
	if group == nil {
		return
	}
	if field := group.Field(loc.field); field != nil {
		field.SetValue(value)
	}
}

func (s *DataDrivenStep) values() map[string]string {
	out := make(map[string]string, len(s.fieldKeys))
	for fieldKey := range s.fieldKeys {
		if field := s.getField(fieldKey); field != nil {
			out[fieldKey] = field.Value()
		}
	}
	return out
}

func (s *DataDrivenStep) LoadFromConfig(cfg *config.Config) {
	for sIdx := range s.definition.Sections {
		for fIdx := range s.definition.Sections[sIdx].Fields {
			fieldDef := &s.definition.Sections[sIdx].Fields[fIdx]
			if fieldDef.ConfigGet != nil {
				s.setValue(fieldDef.Key, fieldDef.ConfigGet(cfg))
			}
		}
	}
}

func (s *DataDrivenStep) WithExtraContentFunc(fn func(step *DataDrivenStep, width int) string) *DataDrivenStep {
	s.customExtraContent = func(width int) string {
		return fn(s, width)
	}
	return s
}

// currentGroup returns the Group of the currently-active section, or nil if
// the index is out of range or the section has no Group.
func (s *DataDrivenStep) currentGroup() *components.InputGroup {
	if s.currentSection < 0 || s.currentSection >= len(s.sections) {
		return nil
	}
	return s.sections[s.currentSection].group
}

func (s *DataDrivenStep) Init() tea.Cmd {
	if len(s.sections) > 0 && s.sections[0].group != nil {
		return s.sections[0].group.Focus()
	}
	return nil
}

func (s *DataDrivenStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if focused {
		s.currentSection = 0
		if len(s.sections) > 0 && s.sections[0].group != nil {
			_ = s.sections[0].group.Focus() // Command executed during Init()
		}
		return
	}
	for _, section := range s.sections {
		if section.group != nil {
			section.group.Blur()
		}
	}
}

func (s *DataDrivenStep) ShortHelp() []KeyBinding {
	return []KeyBinding{
		{Key: "↑↓/tab", Help: "navigate"},
		{Key: "enter", Help: "continue"},
		{Key: "esc", Help: "back"},
	}
}

func (s *DataDrivenStep) Update(msg tea.Msg) (WizardStep, tea.Cmd) {
	group := s.currentGroup()
	if group == nil {
		return s, nil
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter"))):
			if err := s.Validate(); err != nil {
				return s, nil
			}
			return s, func() tea.Msg {
				return StepCompleteMsg{StepID: s.ID()}
			}

		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("tab", "down"))):
			isLastField := group.FocusIndex() >= len(group.Fields())-1
			isLastSection := s.currentSection >= len(s.sections)-1

			if isLastField && isLastSection {
				return s, nil
			}

			if isLastField {
				group.Blur()
				s.currentSection++
				nextGroup := s.currentGroup()
				focusCmd := s.emitFocusChanged()
				if nextGroup == nil {
					return s, focusCmd
				}
				nextGroup.SetFocusIndex(0)
				return s, tea.Batch(nextGroup.Focus(), focusCmd)
			}

			var cmd tea.Cmd
			s.sections[s.currentSection].group, cmd = group.Update(msg)
			return s, tea.Batch(cmd, s.emitFocusChanged())

		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("shift+tab", "up"))):
			isFirstField := group.FocusIndex() == 0
			isFirstSection := s.currentSection == 0

			if isFirstField && isFirstSection {
				return s, nil
			}

			if isFirstField {
				group.Blur()
				s.currentSection--
				prevGroup := s.currentGroup()
				focusCmd := s.emitFocusChanged()
				if prevGroup == nil {
					return s, focusCmd
				}
				prevGroup.SetFocusIndex(len(prevGroup.Fields()) - 1)
				return s, tea.Batch(prevGroup.Focus(), focusCmd)
			}

			var cmd tea.Cmd
			s.sections[s.currentSection].group, cmd = group.Update(msg)
			return s, tea.Batch(cmd, s.emitFocusChanged())
		}
	}

	var cmd tea.Cmd
	s.sections[s.currentSection].group, cmd = group.Update(msg)
	return s, cmd
}

func (s *DataDrivenStep) emitFocusChanged() tea.Cmd {
	globalIndex := 0
	for i := range s.currentSection {
		if s.sections[i].group == nil {
			continue
		}
		globalIndex += len(s.sections[i].group.Fields())
	}
	if current := s.currentGroup(); current != nil {
		globalIndex += current.FocusIndex()
	}

	if s.totalFieldsCache < 0 {
		total := 0
		for _, section := range s.sections {
			if section.group != nil {
				total += len(section.group.Fields())
			}
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

func (s *DataDrivenStep) Validate() error {
	for _, section := range s.sections {
		if section.group == nil {
			continue
		}
		if errs := section.group.Validate(); len(errs) > 0 {
			return errs[0]
		}
	}
	if s.definition.Validate != nil {
		return s.definition.Validate(s.values())
	}
	return nil
}

func (s *DataDrivenStep) Apply(cfg *config.Config) error {
	for sIdx := range s.definition.Sections {
		for fIdx := range s.definition.Sections[sIdx].Fields {
			fieldDef := &s.definition.Sections[sIdx].Fields[fIdx]
			if fieldDef.ConfigSet == nil {
				continue
			}
			if err := fieldDef.ConfigSet(cfg, s.Value(fieldDef.Key)); err != nil {
				return fmt.Errorf("field %s: %w", fieldDef.Key, err)
			}
		}
	}
	if s.definition.Apply != nil {
		return s.definition.Apply(s, cfg)
	}
	return nil
}

func (s *DataDrivenStep) ShouldShow(cfg *config.Config) bool {
	if s.definition.ShouldShow != nil {
		return s.definition.ShouldShow(cfg)
	}
	return true
}

// formViewStyles holds pre-computed lipgloss styles for DataDrivenStep.View.
// Caching is safe because tui.Color* values are set once during package init
// and never change.
var formViewStyles = struct {
	sectionHeader   lipgloss.Style
	activeSection   lipgloss.Style
	inactiveSection lipgloss.Style
	completedRender string
	activeRender    string
	pendingRender   string
	note            lipgloss.Style
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

func (s *DataDrivenStep) View(width, height int) string {
	s.SetSize(width, height)

	innerWidth := width - 4
	if innerWidth < 40 {
		innerWidth = 40
	}

	var content strings.Builder

	for i, section := range s.sections {
		if section.group == nil {
			continue
		}
		section.group.SetWidth(innerWidth)

		var style lipgloss.Style
		var indicator string

		switch {
		case i == s.currentSection:
			style = formViewStyles.activeSection
			indicator = formViewStyles.activeRender
		case section.isComplete():
			style = formViewStyles.inactiveSection
			indicator = formViewStyles.completedRender
		default:
			style = formViewStyles.inactiveSection
			indicator = formViewStyles.pendingRender
		}

		sectionTitle := indicator + " " + formViewStyles.sectionHeader.Render(strings.ToLower(section.title))
		var sectionContent string
		if section.note != "" {
			sectionContent = sectionTitle + "\n" + formViewStyles.note.Render(section.note) + "\n\n" + section.group.ViewCompact("")
		} else {
			sectionContent = sectionTitle + "\n\n" + section.group.ViewCompact("")
		}
		content.WriteString(style.Render(sectionContent))
	}

	// customExtraContent (set via WithExtraContentFunc) takes precedence over
	// the definition's ExtraContent.
	switch {
	case s.customExtraContent != nil:
		content.WriteString(s.customExtraContent(width))
	case s.definition.ExtraContent != nil:
		content.WriteString(s.definition.ExtraContent(s.values(), width))
	}

	return content.String()
}

func SetString(setter func(cfg *config.Config, v string)) ConfigSetter {
	return func(cfg *config.Config, value string) error {
		setter(cfg, value)
		return nil
	}
}

func SetInt(setter func(cfg *config.Config, v int)) ConfigSetter {
	return func(cfg *config.Config, value string) error {
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer: %w", err)
		}
		setter(cfg, v)
		return nil
	}
}

func SetBool(setter func(cfg *config.Config, v bool)) ConfigSetter {
	return func(cfg *config.Config, value string) error {
		v := strings.ToLower(strings.TrimSpace(value))
		b := v == "yes" || v == "true" || v == "1" || v == "y"
		setter(cfg, b)
		return nil
	}
}

func GetString(getter func(cfg *config.Config) string) ConfigGetter {
	return getter
}

func GetInt(getter func(cfg *config.Config) int) ConfigGetter {
	return func(cfg *config.Config) string {
		return strconv.Itoa(getter(cfg))
	}
}

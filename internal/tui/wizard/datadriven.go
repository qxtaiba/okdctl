// Package wizard implements the bubbletea model and step orchestration for
// okdctl's interactive configuration wizard. Steps are declarative
// (StepDefinition + NewDataDrivenStep, preferred) or hand-rolled
// (WizardStep directly) for runtime-dependent sections.
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

// FieldType classifies how a FieldDefinition is rendered and validated.
type FieldType int

// Field type values for data-driven step definitions.
const (
	FieldTypeText FieldType = iota
	FieldTypePassword
	FieldTypeSelect      // dropdown selector
	FieldTypeMultiSelect // checklist, multiple toggles
	FieldTypeKeyValue    // editable key=value table
)

// ConfigSetter writes a field's value into a Config.
type ConfigSetter func(cfg *config.Config, value string) error

// ConfigGetter reads a field's value from a Config.
type ConfigGetter func(cfg *config.Config) string

// FieldDefinition declares a single wizard form field and how it binds to
// the Config struct.
type FieldDefinition struct {
	Key      string
	Label    string
	Default  string
	Help     string
	Type     FieldType
	Options  []string // used by FieldTypeSelect and FieldTypeMultiSelect
	Required bool
	Validate func(string) error

	ConfigSet ConfigSetter
	ConfigGet ConfigGetter
}

// SectionDefinition groups related fields under a shared title/note.
type SectionDefinition struct {
	Title  string
	Note   string // e.g. prerequisites, shown below the title
	Fields []FieldDefinition
}

// StepDefinition is the declarative description of a data-driven wizard step.
type StepDefinition struct {
	ID           StepID
	Title        string
	DisplayTitle string
	Description  string
	Sections     []SectionDefinition

	Validate     func(values map[string]string) error
	Apply        func(step *DataDrivenStep, cfg *config.Config) error // runs after auto-binding
	ShouldShow   func(*config.Config) bool
	ExtraContent func(values map[string]string, width int) string
}

// FormSection pairs a titled section with its built InputGroup — the
// runtime counterpart to SectionDefinition that MultiSectionForm navigates across.
type FormSection struct {
	Title string
	Note  string // e.g. prerequisites, shown below the title
	Group *components.InputGroup
}

func (s *FormSection) isComplete() bool {
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

// MultiSectionForm is a reusable multi-section input form with tab/shift-tab
// navigation and per-section status indicators. It is a widget, not a step:
// Update returns enterPressed rather than emitting StepCompleteMsg itself.
type MultiSectionForm struct {
	sections       []FormSection
	currentSection int

	// totalFieldsCache caches the field count for emitFocusChanged; -1 means uncomputed.
	totalFieldsCache int
}

// NewMultiSectionForm wraps sections in a form focused on the first section.
func NewMultiSectionForm(sections []FormSection) *MultiSectionForm {
	return &MultiSectionForm{
		sections:         sections,
		totalFieldsCache: -1,
	}
}

// CurrentSection returns the index of the section that currently owns focus.
func (f *MultiSectionForm) CurrentSection() int { return f.currentSection }

// FieldAt returns the field at the given section/field indices, or nil when
// either index is out of range or the section has no group.
func (f *MultiSectionForm) FieldAt(section, field int) components.FormField {
	if section < 0 || section >= len(f.sections) {
		return nil
	}
	group := f.sections[section].Group
	if group == nil {
		return nil
	}
	return group.Field(field)
}

// currentGroup returns the active section's Group, or nil if out of range or groupless.
func (f *MultiSectionForm) currentGroup() *components.InputGroup {
	if f.currentSection < 0 || f.currentSection >= len(f.sections) {
		return nil
	}
	return f.sections[f.currentSection].Group
}

// Init focuses the first input group so the user can type immediately.
func (f *MultiSectionForm) Init() tea.Cmd {
	if len(f.sections) > 0 && f.sections[0].Group != nil {
		return f.sections[0].Group.Focus()
	}
	return nil
}

// Focus resets navigation to the first section and focuses it.
func (f *MultiSectionForm) Focus() tea.Cmd {
	f.currentSection = 0
	if len(f.sections) > 0 && f.sections[0].Group != nil {
		return f.sections[0].Group.Focus()
	}
	return nil
}

// Blur removes focus from every section's group.
func (f *MultiSectionForm) Blur() {
	for _, section := range f.sections {
		if section.Group != nil {
			section.Group.Blur()
		}
	}
}

// Update handles tab/shift-tab section navigation and forwards other input
// to the focused group. On enter it reports enterPressed=true without
// validating or completing — the caller layers that.
func (f *MultiSectionForm) Update(msg tea.Msg) (cmd tea.Cmd, enterPressed bool) {
	group := f.currentGroup()
	if group == nil {
		return nil, false
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter"))):
			return nil, true

		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("tab", "down"))):
			isLastField := group.FocusIndex() >= len(group.Fields())-1
			isLastSection := f.currentSection >= len(f.sections)-1

			if isLastField && isLastSection {
				return nil, false
			}

			if isLastField {
				group.Blur()
				f.currentSection++
				nextGroup := f.currentGroup()
				focusCmd := f.emitFocusChanged()
				if nextGroup == nil {
					return focusCmd, false
				}
				nextGroup.SetFocusIndex(0)
				return tea.Batch(nextGroup.Focus(), focusCmd), false
			}

			var groupCmd tea.Cmd
			f.sections[f.currentSection].Group, groupCmd = group.Update(msg)
			return tea.Batch(groupCmd, f.emitFocusChanged()), false

		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("shift+tab", "up"))):
			isFirstField := group.FocusIndex() == 0
			isFirstSection := f.currentSection == 0

			if isFirstField && isFirstSection {
				return nil, false
			}

			if isFirstField {
				group.Blur()
				f.currentSection--
				prevGroup := f.currentGroup()
				focusCmd := f.emitFocusChanged()
				if prevGroup == nil {
					return focusCmd, false
				}
				prevGroup.SetFocusIndex(len(prevGroup.Fields()) - 1)
				return tea.Batch(prevGroup.Focus(), focusCmd), false
			}

			var groupCmd tea.Cmd
			f.sections[f.currentSection].Group, groupCmd = group.Update(msg)
			return tea.Batch(groupCmd, f.emitFocusChanged()), false
		}
	}

	var groupCmd tea.Cmd
	f.sections[f.currentSection].Group, groupCmd = group.Update(msg)
	return groupCmd, false
}

func (f *MultiSectionForm) emitFocusChanged() tea.Cmd {
	globalIndex := 0
	for i := range f.currentSection {
		if f.sections[i].Group == nil {
			continue
		}
		globalIndex += len(f.sections[i].Group.Fields())
	}
	if current := f.currentGroup(); current != nil {
		globalIndex += current.FocusIndex()
	}

	if f.totalFieldsCache < 0 {
		total := 0
		for _, section := range f.sections {
			if section.Group != nil {
				total += len(section.Group.Fields())
			}
		}
		f.totalFieldsCache = total
	}
	totalFields := f.totalFieldsCache

	return func() tea.Msg {
		return FocusChangedMsg{
			FieldIndex:  globalIndex,
			TotalFields: totalFields,
		}
	}
}

// Validate returns the first error from any section's group validation, or nil
// when every field is valid.
func (f *MultiSectionForm) Validate() error {
	for _, section := range f.sections {
		if section.Group == nil {
			continue
		}
		if errs := section.Group.Validate(); len(errs) > 0 {
			return errs[0]
		}
	}
	return nil
}

// View renders each section with its active/completed/pending indicator.
func (f *MultiSectionForm) View(width int) string {
	innerWidth := width - 4
	if innerWidth < 40 {
		innerWidth = 40
	}

	var content strings.Builder

	for i, section := range f.sections {
		if section.Group == nil {
			continue
		}
		section.Group.SetWidth(innerWidth)

		var style lipgloss.Style
		var indicator string

		switch {
		case i == f.currentSection:
			style = formViewStyles.activeSection
			indicator = formViewStyles.activeRender
		case section.isComplete():
			style = formViewStyles.inactiveSection
			indicator = formViewStyles.completedRender
		default:
			style = formViewStyles.inactiveSection
			indicator = formViewStyles.pendingRender
		}

		sectionTitle := indicator + " " + formViewStyles.sectionHeader.Render(strings.ToLower(section.Title))
		var sectionContent string
		if section.Note != "" {
			sectionContent = sectionTitle + "\n" + formViewStyles.note.Render(section.Note) + "\n\n" + section.Group.View()
		} else {
			sectionContent = sectionTitle + "\n\n" + section.Group.View()
		}
		content.WriteString(style.Render(sectionContent))
	}

	return content.String()
}

type fieldLocation struct {
	section int
	field   int
}

// DataDrivenStep renders a multi-section form built from a StepDefinition, implementing WizardStep.
type DataDrivenStep struct {
	BaseStep

	definition *StepDefinition
	fieldKeys  map[string]fieldLocation

	form *MultiSectionForm

	// customExtraContent, when non-nil, overrides definition.ExtraContent (set
	// via WithExtraContentFunc).
	customExtraContent func(width int) string
}

// NewDataDrivenStep builds a DataDrivenStep from a StepDefinition.
func NewDataDrivenStep(def *StepDefinition) *DataDrivenStep {
	step := &DataDrivenStep{
		BaseStep:   NewBaseStepWithDisplayTitle(def.ID, def.Title, def.DisplayTitle, def.Description),
		definition: def,
		fieldKeys:  make(map[string]fieldLocation),
	}

	sections := make([]FormSection, 0, len(def.Sections))
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

		sections = append(sections, FormSection{
			Title: sectionDef.Title,
			Note:  sectionDef.Note,
			Group: components.NewInputGroup(fields...),
		})
	}

	step.form = NewMultiSectionForm(sections)
	return step
}

func buildFormField(def *FieldDefinition) components.FormField {
	switch def.Type {
	case FieldTypeKeyValue:
		kv := components.NewKeyValueField(def.Label)
		kv.Help = def.Help
		if def.Validate != nil {
			kv.Validator = def.Validate
		}
		if def.Default != "" {
			kv.SetValue(def.Default)
		}
		return kv

	case FieldTypeMultiSelect:
		mf := components.NewMultiSelectField(def.Label, def.Options)
		mf.Help = def.Help
		if def.Default != "" {
			mf.SetValue(def.Default)
		}
		return mf

	case FieldTypeSelect:
		sf := components.NewSelectField(def.Label, def.Options)
		sf.Help = def.Help
		if def.Default != "" {
			sf.SetDefault(def.Default)
		}
		return sf

	default:
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
}

// getField resolves fieldKey to its FormField, or nil if unknown; callers must handle nil.
func (s *DataDrivenStep) getField(fieldKey string) components.FormField {
	loc, ok := s.fieldKeys[fieldKey]
	if !ok {
		return nil
	}
	return s.form.FieldAt(loc.section, loc.field)
}

// Value returns the current string value of the field named fieldKey.
func (s *DataDrivenStep) Value(fieldKey string) string {
	if field := s.getField(fieldKey); field != nil {
		return field.Value()
	}
	return ""
}

// ValueInt returns the integer value of fieldKey or fallback when empty
// or unparseable.
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
	if field := s.getField(fieldKey); field != nil {
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

// LoadFromConfig seeds field values from cfg using each field's ConfigGet.
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

// WithExtraContentFunc overrides the definition's ExtraContent with fn.
func (s *DataDrivenStep) WithExtraContentFunc(fn func(step *DataDrivenStep, width int) string) *DataDrivenStep {
	s.customExtraContent = func(width int) string {
		return fn(s, width)
	}
	return s
}

// Init focuses the first input group so the user can type immediately.
func (s *DataDrivenStep) Init() tea.Cmd {
	return s.form.Init()
}

// SetFocused toggles step focus; when re-focused, focus returns to the
// first section.
func (s *DataDrivenStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if focused {
		_ = s.form.Focus() // Command executed during Init()
		return
	}
	s.form.Blur()
}

// ShortHelp returns the key bindings shown in the step's help footer.
func (s *DataDrivenStep) ShortHelp() []KeyBinding {
	return []KeyBinding{
		{Key: "↑↓/tab", Help: HelpNavigate},
		{Key: HelpEnter, Help: HelpContinue},
		{Key: HelpEsc, Help: HelpBack},
	}
}

// Update forwards input to the embedded form and, on enter, runs
// definition-aware validation before emitting StepCompleteMsg.
func (s *DataDrivenStep) Update(msg tea.Msg) (WizardStep, tea.Cmd) {
	cmd, enterPressed := s.form.Update(msg)
	if !enterPressed {
		return s, cmd
	}
	if err := s.Validate(); err != nil {
		return s, nil
	}
	return s, func() tea.Msg {
		return StepCompleteMsg{StepID: s.ID()}
	}
}

// Validate runs the form's field validation, then the step-level Validate
// function if the definition provides one.
func (s *DataDrivenStep) Validate() error {
	if err := s.form.Validate(); err != nil {
		return err
	}
	if s.definition.Validate != nil {
		return s.definition.Validate(s.values())
	}
	return nil
}

// Apply writes each field's value into cfg using its ConfigSet, then runs
// the step-level Apply function if provided.
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

// ShouldShow reports whether this step is visible given the current cfg.
func (s *DataDrivenStep) ShouldShow(cfg *config.Config) bool {
	if s.definition.ShouldShow != nil {
		return s.definition.ShouldShow(cfg)
	}
	return true
}

// formViewStyles caches DataDrivenStep.View's lipgloss styles; safe since
// tui.Color* values never change after init.
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
		Render(tui.IconSuccess),
	activeRender: lipgloss.NewStyle().
		Foreground(tui.ColorPrimary).
		Bold(true).
		Render(tui.IconActive),
	pendingRender: lipgloss.NewStyle().
		Foreground(tui.ColorSlate600).
		Render(tui.IconPending),
	note: lipgloss.NewStyle().
		Foreground(tui.ColorSlate500).
		Italic(true).
		PaddingLeft(2),
}

// View renders the step's sections via the embedded form and appends any
// configured extra content.
func (s *DataDrivenStep) View(width, height int) string {
	s.SetSize(width, height)

	var content strings.Builder
	content.WriteString(s.form.View(width))

	switch {
	case s.customExtraContent != nil:
		content.WriteString(s.customExtraContent(width))
	case s.definition.ExtraContent != nil:
		content.WriteString(s.definition.ExtraContent(s.values(), width))
	}

	return content.String()
}

// SetString adapts a plain string setter into a ConfigSetter.
func SetString(setter func(cfg *config.Config, v string)) ConfigSetter {
	return func(cfg *config.Config, value string) error {
		setter(cfg, value)
		return nil
	}
}

// SetInt adapts an int setter into a ConfigSetter that parses the input.
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

// SetBool adapts a bool setter into a ConfigSetter that parses yes/no.
func SetBool(setter func(cfg *config.Config, v bool)) ConfigSetter {
	return func(cfg *config.Config, value string) error {
		v := strings.ToLower(strings.TrimSpace(value))
		b := v == "yes" || v == "true" || v == "1" || v == "y"
		setter(cfg, b)
		return nil
	}
}

// GetString adapts a string getter into a ConfigGetter.
func GetString(getter func(cfg *config.Config) string) ConfigGetter {
	return getter
}

// GetInt adapts an int getter into a ConfigGetter returning base-10 text.
func GetInt(getter func(cfg *config.Config) int) ConfigGetter {
	return func(cfg *config.Config) string {
		return strconv.Itoa(getter(cfg))
	}
}

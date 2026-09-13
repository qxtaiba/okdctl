// Package wizard implements the bubbletea model and step orchestration for
// okdctl's interactive configuration wizard. Steps are declarative
// (StepDefinition + NewDataDrivenStep, preferred) or hand-rolled
// (WizardStep directly) for runtime-dependent sections.
package wizard

import (
	"errors"
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

// errFixHighlighted is the status-row message shown when enter is pressed
// with invalid fields; FocusFirstInvalid has already moved focus and
// scrolled to the first one.
var errFixHighlighted = errors.New("fix the highlighted fields to continue")

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

// FieldWidth classifies how wide a field's input box renders, in columns,
// independent of the section's full available width.
type FieldWidth int

// Field width classes for data-driven step definitions.
const (
	FieldWidthAuto   FieldWidth = 0 // zero value — 32 columns
	FieldWidthNumber FieldWidth = 12
	FieldWidthPath   FieldWidth = 56
	FieldWidthFull   FieldWidth = -1 // the whole inner width
)

// Cols resolves w to a concrete box width in columns, clamped to avail;
// FieldWidthFull always returns avail itself.
func (w FieldWidth) Cols(avail int) int {
	if w == FieldWidthFull {
		return avail
	}
	if w == FieldWidthAuto {
		return min(32, avail)
	}
	return min(int(w), avail)
}

// fieldWidthSentinel stands in for "no limit" when buildFormField resolves
// a field's nominal box width at construction time, before the real
// available width is known; InputField's own min(boxWidth, width) clamp at
// render time still bounds it correctly on every resize.
const fieldWidthSentinel = 1 << 20

// FieldDefinition declares a single wizard form field and how it binds to
// the Config struct.
type FieldDefinition struct {
	Key         string
	Label       string
	Default     string
	Placeholder string
	Width       FieldWidth
	Help        string
	Type        FieldType
	Options     []string // used by FieldTypeSelect and FieldTypeMultiSelect
	Required    bool
	Validate    func(string) error

	ConfigSet ConfigSetter
	ConfigGet ConfigGetter
}

// SectionDefinition groups related fields under a shared title/note.
type SectionDefinition struct {
	Title   string
	Note    string // e.g. prerequisites, shown below the title
	Fields  []FieldDefinition
	Warning func(values map[string]string) string // non-empty return renders a warning block under the section's fields
}

// StepDefinition is the declarative description of a data-driven wizard step.
type StepDefinition struct {
	ID           StepID
	Title        string
	DisplayTitle string
	Description  string
	Sections     []SectionDefinition

	Validate          func(values map[string]string) error
	Apply             func(step *DataDrivenStep, cfg *config.Config) error // runs after auto-binding
	ShouldShow        func(*config.Config) bool
	ExtraContent      func(values map[string]string, width int) string
	ExtraContentTitle string // info card title used when ExtraContent renders non-empty content
}

// FormSection pairs a titled section with its built InputGroup — the
// runtime counterpart to SectionDefinition that MultiSectionForm navigates across.
type FormSection struct {
	Title   string
	Note    string // e.g. prerequisites, shown below the title
	Group   *components.InputGroup
	Warning func() string // non-empty return renders a warning block under the section's fields
}

func (s *FormSection) warningText() string {
	if s.Warning == nil {
		return ""
	}
	return s.Warning()
}

// isComplete reports whether every field in the section is non-empty and
// passes Check, using the pure check rather than Validate so computing a
// section-complete indicator on every render never paints error state onto
// a field the user hasn't touched.
func (s *FormSection) isComplete() bool {
	if s.Group == nil {
		return false
	}
	for _, field := range s.Group.Fields() {
		if field.Value() == "" {
			return false
		}
		if err := field.Check(); err != nil {
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

	// spans[section][field] is the line range that field occupied in the last
	// View; empty until the form has rendered once.
	spans [][]LineSpan
}

// NewMultiSectionForm wraps sections in a form focused on the first section.
func NewMultiSectionForm(sections []FormSection) *MultiSectionForm {
	return &MultiSectionForm{sections: sections}
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

// FocusedField returns the field owning focus in the current section, or nil
// when the form has no sections or the current section has no group.
func (f *MultiSectionForm) FocusedField() components.FormField {
	group := f.currentGroup()
	if group == nil {
		return nil
	}
	return group.Field(group.FocusIndex())
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
				if nextGroup == nil {
					return focusChanged, false
				}
				nextGroup.SetFocusIndex(0)
				return tea.Batch(nextGroup.Focus(), focusChanged), false
			}

			var groupCmd tea.Cmd
			f.sections[f.currentSection].Group, groupCmd = group.Update(msg)
			return tea.Batch(groupCmd, focusChanged), false

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
				if prevGroup == nil {
					return focusChanged, false
				}
				prevGroup.SetFocusIndex(len(prevGroup.Fields()) - 1)
				return tea.Batch(prevGroup.Focus(), focusChanged), false
			}

			var groupCmd tea.Cmd
			f.sections[f.currentSection].Group, groupCmd = group.Update(msg)
			return tea.Batch(groupCmd, focusChanged), false
		}
	}

	var groupCmd tea.Cmd
	f.sections[f.currentSection].Group, groupCmd = group.Update(msg)
	return groupCmd, false
}

// focusChanged is the tea.Cmd every focus-moving widget returns.
func focusChanged() tea.Msg { return FocusChangedMsg{} }

// FocusedSpan returns the line range the focused field occupied in the last
// View, or false when the form has not rendered or owns no focused field.
func (f *MultiSectionForm) FocusedSpan() (LineSpan, bool) {
	group := f.currentGroup()
	if group == nil || f.currentSection < 0 || f.currentSection >= len(f.spans) {
		return LineSpan{}, false
	}
	spans := f.spans[f.currentSection]
	index := group.FocusIndex()
	if index < 0 || index >= len(spans) {
		return LineSpan{}, false
	}
	return spans[index], true
}

// Validate returns every error from every section's group, running Check
// (via each field's Validate) across the whole form rather than stopping at
// the first invalid section, so every invalid field's error is current for
// View after a submission attempt.
func (f *MultiSectionForm) Validate() []error {
	var errs []error
	for _, section := range f.sections {
		if section.Group == nil {
			continue
		}
		errs = append(errs, section.Group.Validate()...)
	}
	return errs
}

// touchAll marks every field in every section touched and records its
// current error, ahead of Validate, so enter's forced submission attempt
// paints every field's real state rather than only the ones the user has
// visited.
func (f *MultiSectionForm) touchAll() {
	for _, section := range f.sections {
		if section.Group != nil {
			section.Group.TouchAll()
		}
	}
}

// FocusFirstInvalid moves focus to the first field (scanning sections in
// order) that fails Check, and returns a command that runs any focus side
// effect followed by FocusChangedMsg so the wizard re-syncs its viewport
// and scrolls the field into view.
func (f *MultiSectionForm) FocusFirstInvalid() tea.Cmd {
	for si := range f.sections {
		group := f.sections[si].Group
		if group == nil {
			continue
		}
		for fi, field := range group.Fields() {
			if field.Check() == nil {
				continue
			}
			if cur := f.currentGroup(); cur != nil {
				cur.Blur()
			}
			f.currentSection = si
			group.SetFocusIndex(fi)
			return tea.Batch(group.Focus(), focusChanged)
		}
	}
	return focusChanged
}

// innerWidth is the width left to a section's fields inside its horizontal padding.
func (f *MultiSectionForm) innerWidth(width int) int {
	return max(width-4, 40)
}

// sectionHead renders section i's status indicator, title, and optional
// note, wrapping the note to innerWidth.
func (f *MultiSectionForm) sectionHead(i, innerWidth int) string {
	section := &f.sections[i]

	var indicator string
	switch {
	case i == f.currentSection:
		indicator = formViewStyles.activeRender
	case section.isComplete():
		indicator = formViewStyles.completedRender
	default:
		indicator = formViewStyles.pendingRender
	}

	head := indicator + " " + formViewStyles.sectionHeader.Render(section.Title)
	if section.Note != "" {
		head += "\n" + formViewStyles.note.Width(innerWidth).Render(section.Note)
	}
	return head
}

// View renders each section as a head block followed by one block per field,
// one blank row apart, recording the line span every field occupies so the
// wizard can scroll the focused one into view.
func (f *MultiSectionForm) View(width int) string {
	f.spans = make([][]LineSpan, len(f.sections))
	innerWidth := f.innerWidth(width)

	var b strings.Builder
	// The form opens and closes with the blank row the section padding used
	// to contribute, so the first block starts at line 1.
	line := 1
	emit := func(block string) LineSpan {
		if b.Len() > 0 {
			b.WriteString("\n\n")
			line++
		}
		block = formViewStyles.section.Render(block)
		height := lipgloss.Height(block)
		b.WriteString(block)
		span := LineSpan{Start: line, End: line + height - 1}
		line += height
		return span
	}

	for i := range f.sections {
		group := f.sections[i].Group
		if group == nil {
			continue
		}
		group.SetWidth(innerWidth)

		_ = emit(f.sectionHead(i, innerWidth))

		views := group.FieldViews()
		f.spans[i] = make([]LineSpan, len(views))
		for j, view := range views {
			f.spans[i][j] = emit(view)
		}

		if w := f.sections[i].warningText(); w != "" {
			_ = emit(formViewStyles.warning.Width(innerWidth).Render(tui.IconWarning + " " + w))
		}
	}

	if b.Len() == 0 {
		return ""
	}
	return "\n" + b.String() + "\n"
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
	// via WithExtraContentFunc); customExtraContentTitle is its info card title.
	customExtraContent      func(width int) string
	customExtraContentTitle string
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

		var warning func() string
		if sectionDef.Warning != nil {
			warning = func() string { return sectionDef.Warning(step.values()) }
		}

		sections = append(sections, FormSection{
			Title:   sectionDef.Title,
			Note:    sectionDef.Note,
			Group:   components.NewInputGroup(fields...),
			Warning: warning,
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
		if def.Width != FieldWidthAuto {
			sf.SetBoxWidth(def.Width.Cols(fieldWidthSentinel))
		}
		return sf

	default:
		var field *components.InputField
		if def.Type == FieldTypePassword {
			field = components.NewPasswordField(def.Label, "")
		} else {
			field = components.NewInputField(def.Label, "")
		}
		field.SetPlaceholder(def.Placeholder)
		if def.Default != "" {
			field.SetDefault(def.Default)
		}
		field.Required = def.Required
		field.Help = def.Help
		field.Validator = def.Validate
		field.SetBoxWidth(def.Width.Cols(fieldWidthSentinel))
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

// LoadFromConfig seeds field values from cfg using each field's ConfigGet;
// when configExists is false, cfg is a synthetic defaults-only seed (e.g.
// config.DefaultConfig(), not a real saved file), so a zero-value read
// leaves a field's own constructed default in place instead of wiping it
// (a gap in DefaultConfig isn't an intentional blank), while configExists
// true trusts cfg as authoritative and clears a field on a real blank.
func (s *DataDrivenStep) LoadFromConfig(cfg *config.Config, configExists bool) {
	for sIdx := range s.definition.Sections {
		for fIdx := range s.definition.Sections[sIdx].Fields {
			fieldDef := &s.definition.Sections[sIdx].Fields[fIdx]
			if fieldDef.ConfigGet == nil {
				continue
			}
			value := fieldDef.ConfigGet(cfg)
			if value == "" && !configExists {
				continue
			}
			s.setValue(fieldDef.Key, value)
		}
	}
}

// WithExtraContentFunc overrides the definition's ExtraContent with fn,
// rendered under the given info card title.
func (s *DataDrivenStep) WithExtraContentFunc(title string, fn func(step *DataDrivenStep, width int) string) *DataDrivenStep {
	s.customExtraContentTitle = title
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

// ShortHelp returns the key bindings shown in the step's help footer, plus
// any key hints the focused field contributes.
func (s *DataDrivenStep) ShortHelp() []KeyBinding {
	bindings := []KeyBinding{
		{Key: "↑↓/tab", Help: HelpNavigate},
		{Key: HelpEnter, Help: HelpContinue},
		{Key: HelpEsc, Help: HelpBack},
	}
	if h, ok := s.form.FocusedField().(components.KeyHinter); ok {
		for _, hint := range h.KeyHints() {
			bindings = append(bindings, KeyBinding{Key: hint.Key, Help: hint.Help})
		}
	}
	return bindings
}

// Update forwards input to the embedded form and, on enter, touches and
// validates every field (scrolling to and reporting the first invalid one
// on failure) before running definition-aware validation and emitting
// StepCompleteMsg.
func (s *DataDrivenStep) Update(msg tea.Msg) (WizardStep, tea.Cmd) {
	cmd, enterPressed := s.form.Update(msg)
	if !enterPressed {
		return s, cmd
	}

	s.form.touchAll()
	if errs := s.form.Validate(); len(errs) > 0 {
		return s, tea.Batch(s.form.FocusFirstInvalid(), func() tea.Msg { return ErrorSetMsg{Error: errFixHighlighted} })
	}
	if s.definition.Validate != nil {
		if err := s.definition.Validate(s.values()); err != nil {
			return s, func() tea.Msg { return ErrorSetMsg{Error: err} }
		}
	}
	return s, func() tea.Msg {
		return StepCompleteMsg{StepID: s.ID()}
	}
}

// Validate runs the form's field validation, then the step-level Validate
// function if the definition provides one.
func (s *DataDrivenStep) Validate() error {
	if errs := s.form.Validate(); len(errs) > 0 {
		return errs[0]
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

// FocusedSpan reports the line range the focused field occupied in the last
// View; the form's blocks start at the step's own line 0, so no offset applies.
func (s *DataDrivenStep) FocusedSpan() (LineSpan, bool) {
	return s.form.FocusedSpan()
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
	section         lipgloss.Style
	completedRender string
	activeRender    string
	pendingRender   string
	note            lipgloss.Style
	warning         lipgloss.Style
}{
	sectionHeader: lipgloss.NewStyle().
		Foreground(tui.ColorCyan500).
		Bold(true),
	section: lipgloss.NewStyle().
		PaddingLeft(2),
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
	warning: lipgloss.NewStyle().
		Foreground(tui.ColorWarning),
}

// View renders the step's sections via the embedded form and appends any
// configured extra content as an info card.
func (s *DataDrivenStep) View(width, height int) string {
	s.SetSize(width, height)

	var content strings.Builder
	content.WriteString(s.form.View(width))

	var title, body string
	switch {
	case s.customExtraContent != nil:
		title, body = s.customExtraContentTitle, s.customExtraContent(width)
	case s.definition.ExtraContent != nil:
		title, body = s.definition.ExtraContentTitle, s.definition.ExtraContent(s.values(), width)
	}
	if body != "" {
		content.WriteString("\n\n")
		content.WriteString(RenderInfoCard(title, body, width))
	}

	return content.String()
}

// RenderInfoCard renders body as a bordered card titled title, exactly width columns wide.
func RenderInfoCard(title, body string, width int) string {
	return tui.Card(title, lipgloss.Wrap(body, width-4, ""), width, tui.ColorSlate600)
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

// Package wizard implements the bubbletea model and step orchestration for
// okdctl's configuration wizard; steps are declarative StepDefinitions or
// hand-rolled WizardSteps.
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

// ErrFixHighlighted is the status-row message when enter finds invalid
// fields; FocusFirstInvalid has already moved focus to the first one.
var ErrFixHighlighted = errors.New("fix the highlighted fields to continue")

// crossFieldError names the field keys a StepDefinition.Validate error
// implicates.
type crossFieldError struct {
	err  error
	keys []string
}

func (e *crossFieldError) Error() string { return e.err.Error() }
func (e *crossFieldError) Unwrap() error { return e.err }

// NewCrossFieldError wraps err so it implicates the given field keys.
func NewCrossFieldError(err error, keys ...string) error {
	return &crossFieldError{err: err, keys: keys}
}

// FieldType classifies how a FieldDefinition is rendered and validated.
type FieldType int

// Field types.
const (
	FieldTypeText FieldType = iota
	FieldTypePassword
	FieldTypeSelect // dropdown selector
)

// ConfigSetter writes a field's value into a Config.
type ConfigSetter func(cfg *config.Config, value string) error

// ConfigGetter reads a field's value from a Config.
type ConfigGetter func(cfg *config.Config) string

// FieldWidth sets an input box's preferred width, clamped to its section.
type FieldWidth int

// formFieldChrome is the section indent and row padding a box must clear
// beneath formMaxWidth.
const formFieldChrome = 8

// Field width classes.
const (
	FieldWidthAuto   FieldWidth = 0 // zero value — special-cased in Cols
	FieldWidthNumber FieldWidth = 16
	FieldWidthPath   FieldWidth = formMaxWidth - 4
	FieldWidthFull   FieldWidth = -1 // the whole inner width
)

// Cols resolves w to a box width clamped to avail; FieldWidthFull is avail
// itself.
func (w FieldWidth) Cols(avail int) int {
	if w == FieldWidthFull {
		return avail
	}
	if w == FieldWidthAuto {
		return min(formMaxWidth-formFieldChrome, avail)
	}
	return min(int(w), avail)
}

// fieldWidthSentinel stands in for "no limit" before the available width is
// known; the render-time clamp still bounds it.
const fieldWidthSentinel = 1 << 20

// FieldDefinition declares one form field and how it binds to the Config.
type FieldDefinition struct {
	Key         string
	Label       string
	Default     string
	Placeholder string
	Width       FieldWidth
	Help        string
	Type        FieldType
	Options     []string // used by FieldTypeSelect
	Required    bool
	Validate    func(string) error

	// PairKey renders adjacent fields sharing it on one row when the width
	// allows
	PairKey string

	ConfigSet ConfigSetter
	ConfigGet ConfigGetter
}

// SectionDefinition groups related fields under a shared title and note.
type SectionDefinition struct {
	Title   string
	Note    string // e.g. prerequisites, shown below the title
	Fields  []FieldDefinition
	Warning func(values map[string]string) string // non-empty return renders a warning block under the section's fields
	Visible func(values map[string]string) bool   // nil means always visible; false hides the section from render, navigation, and validation

	// Collapsible folds the rendering to one summary line; Validate, TouchAll
	// and FocusFirstInvalid never skip a folded section
	Collapsible bool
	// FoldSummary supplies the facts the collapsed row echoes; never return
	// credential material
	FoldSummary func(values map[string]string) []tui.FactRow

	// AbsenceNote explains a hidden section in its place; empty renders nothing
	AbsenceNote func(values map[string]string) string
}

// StepDefinition is the declarative description of a data-driven step.
type StepDefinition struct {
	ID           StepID
	Title        string
	DisplayTitle string
	Description  string
	Sections     []SectionDefinition

	Validate          func(values map[string]string) error
	Apply             func(step *DataDrivenStep, cfg *config.Config) error // runs after auto-binding
	Secrets           func(step *DataDrivenStep) Secrets                   // env-only values the caller writes to okdctl.env
	ShouldShow        func(*config.Config) bool
	ExtraContent      func(values map[string]string, width int) string
	ExtraContentTitle string // info card title used when ExtraContent renders non-empty content
}

// FormSection is the runtime counterpart of SectionDefinition that
// MultiSectionForm navigates across.
type FormSection struct {
	Title   string
	Note    string // e.g. prerequisites, shown below the title
	Group   *components.InputGroup
	Warning func() string // non-empty return renders a warning block under the section's fields
	Visible func() bool   // nil means always visible

	// Collapsible requires Group.Fields()[0] to be a components.Foldable
	Collapsible bool

	absenceNote func() string

	// pairKeys[j] is Fields[j].PairKey aligned to Group.Fields()
	pairKeys []string
}

func (s *FormSection) absenceText() string {
	if s.absenceNote == nil {
		return ""
	}
	return s.absenceNote()
}

func (s *FormSection) warningText() string {
	if s.Warning == nil {
		return ""
	}
	return s.Warning()
}

func (s *FormSection) isVisible() bool {
	if s.Visible == nil {
		return true
	}
	return s.Visible()
}

// isComplete uses the pure Check so a per-render indicator never paints
// error state onto an untouched field; only Required enforces non-empty.
func (s *FormSection) isComplete() bool {
	if s.Group == nil {
		return false
	}
	for _, field := range s.Group.Fields() {
		if err := field.Check(); err != nil {
			return false
		}
	}
	return true
}

// MultiSectionForm is a multi-section input form with tab navigation. It is
// a widget, not a step: Update reports enterPressed rather than emitting
// StepCompleteMsg.
type MultiSectionForm struct {
	sections       []FormSection
	currentSection int

	// spans[section][field] is the line range from the last View
	spans [][]LineSpan
}

// NewMultiSectionForm wraps sections in a form focused on the first.
func NewMultiSectionForm(sections []FormSection) *MultiSectionForm {
	return &MultiSectionForm{sections: sections}
}

// CurrentSection returns the index of the focused section.
func (f *MultiSectionForm) CurrentSection() int { return f.currentSection }

// FieldAt returns the field at section/field, or nil when out of range.
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

func (f *MultiSectionForm) currentGroup() *components.InputGroup {
	if f.currentSection < 0 || f.currentSection >= len(f.sections) {
		return nil
	}
	return f.sections[f.currentSection].Group
}

// FocusedField returns the focused field, or nil when there is none.
func (f *MultiSectionForm) FocusedField() components.FormField {
	group := f.currentGroup()
	if group == nil {
		return nil
	}
	return group.Field(group.FocusIndex())
}

// ConsumesTextInput reports whether the focused field takes "?" as typed
// text rather than the help-overlay toggle.
func (f *MultiSectionForm) ConsumesTextInput() bool {
	field := f.FocusedField()
	if field == nil {
		return false
	}
	tc, ok := field.(components.TextInputField)
	return ok && tc.ConsumesTextInput()
}

// Init focuses the first visible input group.
func (f *MultiSectionForm) Init() tea.Cmd {
	f.currentSection = f.firstVisible()
	if f.currentSection >= 0 && f.sections[f.currentSection].Group != nil {
		return f.sections[f.currentSection].Group.Focus()
	}
	return nil
}

func (f *MultiSectionForm) firstVisible() int {
	return f.nextVisible(-1)
}

func (f *MultiSectionForm) nextVisible(i int) int {
	for j := i + 1; j < len(f.sections); j++ {
		if f.sections[j].isVisible() {
			return j
		}
	}
	return -1
}

func (f *MultiSectionForm) prevVisible(i int) int {
	for j := i - 1; j >= 0; j-- {
		if f.sections[j].isVisible() {
			return j
		}
	}
	return -1
}

// Focus resets navigation to the first visible section and focuses it.
func (f *MultiSectionForm) Focus() tea.Cmd {
	f.currentSection = f.firstVisible()
	if f.currentSection >= 0 && f.sections[f.currentSection].Group != nil {
		return f.sections[f.currentSection].Group.Focus()
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

// Update handles tab navigation and forwards other input to the focused
// group; enter reports enterPressed for the caller to validate, unless the
// focused field is an EnterConsumer.
func (f *MultiSectionForm) Update(msg tea.Msg) (cmd tea.Cmd, enterPressed bool) {
	group := f.currentGroup()
	if group == nil {
		return nil, false
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter"))):
			if ec, isConsumer := f.FocusedField().(components.EnterConsumer); isConsumer && ec.ConsumesEnter() {
				var groupCmd tea.Cmd
				f.sections[f.currentSection].Group, groupCmd = group.Update(msg)
				return groupCmd, false
			}
			return nil, true

		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("tab", "down"))):
			isLastField := group.FocusIndex() >= len(group.Fields())-1
			next := f.nextVisible(f.currentSection)

			if isLastField && next == -1 {
				return nil, false
			}

			if isLastField {
				group.Blur()
				f.currentSection = next
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
			prev := f.prevVisible(f.currentSection)

			if isFirstField && prev == -1 {
				return nil, false
			}

			if isFirstField {
				group.Blur()
				f.currentSection = prev
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

func focusChanged() tea.Msg { return FocusChangedMsg{} }

// FocusedSpan returns the focused field's line range from the last View, or
// false before the first render.
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

// Validate returns every visible section's errors, not just the first, so
// each field's error is current for View; hidden sections never block.
func (f *MultiSectionForm) Validate() []error {
	var errs []error
	for _, section := range f.sections {
		if section.Group == nil || !section.isVisible() {
			continue
		}
		errs = append(errs, section.Group.Validate()...)
	}
	return errs
}

// TouchAll marks every visible field touched so enter paints each one's
// real state, not only the visited ones.
func (f *MultiSectionForm) TouchAll() {
	for _, section := range f.sections {
		if section.Group != nil && section.isVisible() {
			section.Group.TouchAll()
		}
	}
}

// FocusFirstInvalid focuses the first visible field failing Check and
// returns its focus command followed by FocusChangedMsg.
func (f *MultiSectionForm) FocusFirstInvalid() tea.Cmd {
	for si := range f.sections {
		if !f.sections[si].isVisible() {
			continue
		}
		group := f.sections[si].Group
		if group == nil {
			continue
		}
		for fi, field := range group.Fields() {
			if field.Check() == nil {
				continue
			}
			return f.focusAt(si, fi)
		}
	}
	return focusChanged
}

// focusAt blurs the current group and focuses section/field, returning the
// focus command followed by FocusChangedMsg.
func (f *MultiSectionForm) focusAt(section, field int) tea.Cmd {
	if section < 0 || section >= len(f.sections) || !f.sections[section].isVisible() {
		return focusChanged
	}
	group := f.sections[section].Group
	if group == nil || group.Field(field) == nil {
		return focusChanged
	}
	if cur := f.currentGroup(); cur != nil {
		cur.Blur()
	}
	f.currentSection = section
	group.SetFocusIndex(field)
	return tea.Batch(group.Focus(), focusChanged)
}

func (f *MultiSectionForm) innerWidth(width int) int {
	return max(width-4, 40)
}

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

// foldState reports whether section i renders in full (sticky latch, focus
// or an error inside) and sets the toggle's chrome; a non-fold or a
// construction bug reads as open, never as hidden fields.
func (f *MultiSectionForm) foldState(i int) (fold components.Foldable, open, ok bool) {
	section := &f.sections[i]
	if !section.Collapsible || section.Group == nil || len(section.Group.Fields()) == 0 {
		return nil, true, false
	}
	fold, ok = section.Group.Fields()[0].(components.Foldable)
	if !ok {
		return nil, true, false
	}
	open = i == f.currentSection || !section.isComplete() || fold.Expanded()
	fold.SetDisplayExpanded(open)
	return fold, open, true
}

// pairGap is the blank columns between a pair's rendered columns.
const pairGap = 4

var pairGapStr = strings.Repeat(" ", pairGap)

// pairMinWidth is the View width the 80-column tier hands a form; below it
// every declared pair renders single column.
const pairMinWidth = 70

// pairDefaultTagged is implemented by a paired field that may carry a
// "default" tag.
type pairDefaultTagged interface {
	HasDefaultTag() bool
}

// pairBoxWidthSetter is implemented by a paired field whose box width can
// be capped independently of SetWidth.
type pairBoxWidthSetter interface {
	SetBoxWidth(outer int)
}

// pairRuns partitions n field indexes into runs sharing one non-empty
// PairKey, every unpaired field a run of one.
func pairRuns(pairKeys []string, n int) [][]int {
	var runs [][]int
	for i := 0; i < n; {
		pairKey := ""
		if i < len(pairKeys) {
			pairKey = pairKeys[i]
		}
		j := i + 1
		if pairKey != "" {
			for j < n && j < len(pairKeys) && pairKeys[j] == pairKey {
				j++
			}
		}
		run := make([]int, j-i)
		for x := range run {
			run[x] = i + x
		}
		runs = append(runs, run)
		i = j
	}
	return runs
}

// mixedDefaultTags reports a run where only some fields reserve the tag's
// room, which would otherwise make one column's box narrower.
func mixedDefaultTags(fields []components.FormField, run []int) bool {
	has, hasNot := false, false
	for _, idx := range run {
		if tagged, ok := fields[idx].(pairDefaultTagged); ok && tagged.HasDefaultTag() {
			has = true
		} else {
			hasNot = true
		}
	}
	return has && hasNot
}

// applyPairWidths narrows each run of paired fields to an even share of
// innerWidth, reserving DefaultTagReserve in every member when the tags are
// mixed; below pairMinWidth every run stays single column.
func applyPairWidths(fields []components.FormField, pairKeys []string, width, innerWidth int) {
	if width < pairMinWidth {
		return
	}
	for _, run := range pairRuns(pairKeys, len(fields)) {
		if len(run) < 2 {
			continue
		}
		colWidth := max((innerWidth-pairGap*(len(run)-1))/len(run), 1)
		if mixedDefaultTags(fields, run) {
			boxCap := max(colWidth-components.DefaultTagReserve, 1)
			for _, idx := range run {
				if bw, ok := fields[idx].(pairBoxWidthSetter); ok {
					bw.SetBoxWidth(boxCap)
				}
			}
		}
		for _, idx := range run {
			fields[idx].SetWidth(colWidth)
		}
	}
}

// joinPairedViews joins each pairRuns run horizontally; covered[i] lists
// the field indexes block i represents so one LineSpan covers the run.
func joinPairedViews(views, pairKeys []string, width int) (blocks []string, covered [][]int) {
	if width < pairMinWidth {
		for i := range views {
			blocks = append(blocks, views[i])
			covered = append(covered, []int{i})
		}
		return blocks, covered
	}
	for _, run := range pairRuns(pairKeys, len(views)) {
		block := views[run[0]]
		for _, idx := range run[1:] {
			block = lipgloss.JoinHorizontal(lipgloss.Top, block, pairGapStr, views[idx])
		}
		blocks = append(blocks, block)
		covered = append(covered, run)
	}
	return blocks, covered
}

// View renders each visible section as a head block and one block per
// field, recording every field's line span.
func (f *MultiSectionForm) View(width int) string {
	f.spans = make([][]LineSpan, len(f.sections))
	innerWidth := f.innerWidth(width)

	var b strings.Builder
	// The form opens and closes with a blank row, so the first block is line 1.
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
		if !f.sections[i].isVisible() {
			if note := f.sections[i].absenceText(); note != "" {
				_ = emit(formViewStyles.note.Width(innerWidth).Render(note))
			}
			continue
		}
		group := f.sections[i].Group
		if group == nil {
			continue
		}
		group.SetWidth(innerWidth)
		applyPairWidths(group.Fields(), f.sections[i].pairKeys, width, innerWidth)

		_, open, isFold := f.foldState(i)
		if !isFold {
			_ = emit(f.sectionHead(i, innerWidth))
		}

		views := group.FieldViews()
		f.spans[i] = make([]LineSpan, len(views))
		if isFold && !open {
			// Collapsed: the fold row is the whole section; validation still reaches
			// the hidden fields.
			f.spans[i][0] = emit(views[0])
			continue
		}

		blocks, covered := joinPairedViews(views, f.sections[i].pairKeys, width)
		for bi, block := range blocks {
			span := emit(block)
			for _, idx := range covered[bi] {
				f.spans[i][idx] = span
			}
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

// DataDrivenStep renders a multi-section form built from a StepDefinition.
type DataDrivenStep struct {
	BaseStep

	definition *StepDefinition
	fieldKeys  map[string]fieldLocation

	form *MultiSectionForm

	// customExtraContent overrides definition.ExtraContent
	customExtraContent      func(width int) string
	customExtraContentTitle string
	customPinnedFooter      func(width int) string
}

// NewDataDrivenStep builds a DataDrivenStep from def.
func NewDataDrivenStep(def *StepDefinition) *DataDrivenStep {
	step := &DataDrivenStep{
		BaseStep:   NewBaseStepWithDisplayTitle(def.ID, def.Title, def.DisplayTitle, def.Description),
		definition: def,
		fieldKeys:  make(map[string]fieldLocation),
	}

	sections := make([]FormSection, 0, len(def.Sections))
	for sectionIdx := range def.Sections {
		sectionDef := &def.Sections[sectionIdx]
		fields := make([]components.FormField, 0, len(sectionDef.Fields)+1)
		pairKeys := make([]string, 0, len(sectionDef.Fields)+1)

		// A Collapsible section prepends a fold toggle, shifting every field index.
		fieldOffset := 0
		if sectionDef.Collapsible {
			fieldOffset = 1
			fields = append(fields, components.NewFoldToggleField(sectionDef.Title, foldSummaryFunc(sectionDef, step)))
			pairKeys = append(pairKeys, "")
		}

		for fieldIdx := range sectionDef.Fields {
			fieldDef := &sectionDef.Fields[fieldIdx]
			field := buildFormField(fieldDef)
			fields = append(fields, field)
			pairKeys = append(pairKeys, fieldDef.PairKey)
			step.fieldKeys[fieldDef.Key] = fieldLocation{
				section: sectionIdx,
				field:   fieldIdx + fieldOffset,
			}
		}

		var warning func() string
		if sectionDef.Warning != nil {
			warning = func() string { return sectionDef.Warning(step.values()) }
		}

		var visible func() bool
		if sectionDef.Visible != nil {
			visible = func() bool { return sectionDef.Visible(step.rawValues()) }
		}

		var absenceNote func() string
		if sectionDef.AbsenceNote != nil {
			absenceNote = func() string { return sectionDef.AbsenceNote(step.rawValues()) }
		}

		sections = append(sections, FormSection{
			Title:       sectionDef.Title,
			Note:        sectionDef.Note,
			Group:       components.NewInputGroup(fields...),
			Warning:     warning,
			Visible:     visible,
			Collapsible: sectionDef.Collapsible,
			absenceNote: absenceNote,
			pairKeys:    pairKeys,
		})
	}

	step.form = NewMultiSectionForm(sections)
	return step
}

func foldSummaryFunc(sectionDef *SectionDefinition, step *DataDrivenStep) func() []tui.FactRow {
	if sectionDef.FoldSummary == nil {
		return nil
	}
	return func() []tui.FactRow { return sectionDef.FoldSummary(step.rawValues()) }
}

func buildFormField(def *FieldDefinition) components.FormField {
	switch def.Type {
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

func (s *DataDrivenStep) getField(fieldKey string) components.FormField {
	loc, ok := s.fieldKeys[fieldKey]
	if !ok {
		return nil
	}
	return s.form.FieldAt(loc.section, loc.field)
}

// Value returns the current value of the field named fieldKey.
func (s *DataDrivenStep) Value(fieldKey string) string {
	if field := s.getField(fieldKey); field != nil {
		return field.Value()
	}
	return ""
}

// Definition returns the step's declarative definition.
func (s *DataDrivenStep) Definition() *StepDefinition { return s.definition }

// SetValue updates a field without touching focus or validation state.
func (s *DataDrivenStep) SetValue(fieldKey, value string) bool {
	if s.getField(fieldKey) == nil {
		return false
	}
	s.setValue(fieldKey, value)
	return true
}

// ValueInt returns fieldKey as an int, or fallback when empty or malformed.
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

// rawValues ignores visibility so a Visible predicate never recurses into
// values().
func (s *DataDrivenStep) rawValues() map[string]string {
	out := make(map[string]string, len(s.fieldKeys))
	for fieldKey := range s.fieldKeys {
		if field := s.getField(fieldKey); field != nil {
			out[fieldKey] = field.Value()
		}
	}
	return out
}

func (s *DataDrivenStep) values() map[string]string {
	out := make(map[string]string, len(s.fieldKeys))
	for fieldKey, loc := range s.fieldKeys {
		if !s.form.sections[loc.section].isVisible() {
			continue
		}
		if field := s.getField(fieldKey); field != nil {
			out[fieldKey] = field.Value()
		}
	}
	return out
}

// LoadFromConfig seeds fields from cfg. With configExists false cfg is a
// defaults-only seed, so a zero value leaves the field's own default in
// place instead of blanking it.
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

// WithExtraContentFunc overrides the definition's ExtraContent.
func (s *DataDrivenStep) WithExtraContentFunc(title string, fn func(step *DataDrivenStep, width int) string) *DataDrivenStep {
	s.customExtraContentTitle = title
	s.customExtraContent = func(width int) string {
		return fn(s, width)
	}
	return s
}

// WithPinnedFooterFunc renders step-specific live information in the help row.
func (s *DataDrivenStep) WithPinnedFooterFunc(fn func(step *DataDrivenStep, width int) string) *DataDrivenStep {
	s.customPinnedFooter = func(width int) string { return fn(s, width) }
	return s
}

// PinnedFooter returns the configured footer row, or empty when unset.
func (s *DataDrivenStep) PinnedFooter(width int) string {
	if s.customPinnedFooter == nil {
		return ""
	}
	return s.customPinnedFooter(width)
}

// Init focuses the first input group.
func (s *DataDrivenStep) Init() tea.Cmd {
	return s.form.Init()
}

// SetFocused toggles step focus; re-focusing returns to the first section.
func (s *DataDrivenStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if focused {
		_ = s.form.Focus()
		return
	}
	s.form.Blur()
}

// ShortHelp returns the footer bindings plus the focused field's hints.
func (s *DataDrivenStep) ShortHelp() []KeyBinding {
	bindings := []KeyBinding{
		{Key: "↑↓/tab", Help: HelpNavigate},
		{Key: HelpEnter, Help: HelpContinue},
		{Key: HelpEsc, Help: HelpBack},
		{Key: HelpCtrlC, Help: HelpQuit},
	}
	if h, ok := s.form.FocusedField().(components.KeyHinter); ok {
		for _, hint := range h.KeyHints() {
			bindings = append(bindings, KeyBinding{Key: hint.Key, Help: hint.Help})
		}
	}
	return bindings
}

// ConsumesTextInput reports whether the focused field is mid text entry.
func (s *DataDrivenStep) ConsumesTextInput() bool {
	return s.form.ConsumesTextInput()
}

// Update forwards input to the form and, on enter, touches and validates
// every field before the definition's Validate and StepCompleteMsg.
func (s *DataDrivenStep) Update(msg tea.Msg) (WizardStep, tea.Cmd) {
	cmd, enterPressed := s.form.Update(msg)
	if !enterPressed {
		switch msg.(type) {
		case tea.KeyPressMsg, tea.PasteMsg:
			return s, tea.Batch(cmd, func() tea.Msg { return ConfigSyncMsg{StepID: s.ID()} })
		}
		return s, cmd
	}

	s.form.TouchAll()
	if errs := s.form.Validate(); len(errs) > 0 {
		return s, tea.Batch(s.form.FocusFirstInvalid(), func() tea.Msg { return ErrorSetMsg{Error: ErrFixHighlighted} })
	}
	if s.definition.Validate != nil {
		if err := s.redactedDefinitionValidate(); err != nil {
			errCmd := func() tea.Msg { return ErrorSetMsg{Error: err} }
			var cfe *crossFieldError
			if errors.As(err, &cfe) {
				return s, tea.Batch(s.focusCrossFieldError(cfe), errCmd)
			}
			return s, errCmd
		}
	}
	return s, func() tea.Msg {
		return StepCompleteMsg{StepID: s.ID()}
	}
}

// focusCrossFieldError focuses the first implicated field, then marks every
// implicated field; focusAt's blur would otherwise re-validate and erase the
// error.
func (s *DataDrivenStep) focusCrossFieldError(cfe *crossFieldError) tea.Cmd {
	var first *fieldLocation
	for _, fieldKey := range cfe.keys {
		if loc, ok := s.fieldKeys[fieldKey]; ok {
			first = &loc
			break
		}
	}

	var cmd tea.Cmd = focusChanged
	if first != nil {
		cmd = s.form.focusAt(first.section, first.field)
	}

	for _, fieldKey := range cfe.keys {
		if setter, ok := s.getField(fieldKey).(components.FieldErrorSetter); ok {
			setter.SetError(cfe)
		}
	}

	return cmd
}

// IsComplete reports whether every visible field passes Check without
// painting error state.
func (s *DataDrivenStep) IsComplete() bool {
	for i := range s.form.sections {
		if s.form.sections[i].isVisible() && !s.form.sections[i].isComplete() {
			return false
		}
	}
	return true
}

// Validate runs the field validation, then the definition's Validate.
func (s *DataDrivenStep) Validate() error {
	if errs := s.form.Validate(); len(errs) > 0 {
		return errs[0]
	}
	if s.definition.Validate != nil {
		return s.redactedDefinitionValidate()
	}
	return nil
}

// redactedDefinitionValidate scrubs every password field's live value out of
// the definition's Validate error, keeping a crossFieldError's keys.
func (s *DataDrivenStep) redactedDefinitionValidate() error {
	err := s.definition.Validate(s.values())
	if err == nil {
		return nil
	}
	message := err.Error()
	redacted := false
	for _, section := range s.definition.Sections {
		for index := range section.Fields {
			field := &section.Fields[index]
			if field.Type != FieldTypePassword {
				continue
			}
			if v := s.Value(field.Key); v != "" && strings.Contains(message, v) {
				message = strings.ReplaceAll(message, v, "<redacted>")
				redacted = true
			}
		}
	}
	if !redacted {
		return err
	}
	var cfe *crossFieldError
	if errors.As(err, &cfe) {
		return &crossFieldError{err: errors.New(message), keys: cfe.keys}
	}
	return errors.New(message)
}

// Apply writes each valid field into cfg through its ConfigSet, then runs
// the definition's Apply. A field failing Check is skipped, so a
// per-keystroke sync never writes a half-typed value.
func (s *DataDrivenStep) Apply(cfg *config.Config) error {
	for sIdx := range s.definition.Sections {
		if !s.form.sections[sIdx].isVisible() {
			continue
		}
		for fIdx := range s.definition.Sections[sIdx].Fields {
			fieldDef := &s.definition.Sections[sIdx].Fields[fIdx]
			field := s.getField(fieldDef.Key)
			if fieldDef.ConfigSet == nil || field == nil || field.Check() != nil {
				continue
			}
			if err := fieldDef.ConfigSet(cfg, field.Value()); err != nil {
				return fmt.Errorf("field %s: %w", fieldDef.Key, err)
			}
		}
	}
	if s.definition.Apply != nil {
		return s.definition.Apply(s, cfg)
	}
	return nil
}

// Secrets returns the definition's env-only values, zero when none.
func (s *DataDrivenStep) Secrets() Secrets {
	if s.definition.Secrets == nil {
		return Secrets{}
	}
	return s.definition.Secrets(s)
}

// FocusedSpan reports the focused field's line range from the last View.
func (s *DataDrivenStep) FocusedSpan() (LineSpan, bool) {
	return s.form.FocusedSpan()
}

// ShouldShow reports whether the step is visible for cfg.
func (s *DataDrivenStep) ShouldShow(cfg *config.Config) bool {
	if s.definition.ShouldShow != nil {
		return s.definition.ShouldShow(cfg)
	}
	return true
}

// formViewStyles rebinds on the background flip, so rebuildFormViewStyles
// rebuilds it with the rest of the wizard styles.
var formViewStyles struct {
	sectionHeader   lipgloss.Style
	section         lipgloss.Style
	completedRender string
	activeRender    string
	pendingRender   string
	note            lipgloss.Style
	warning         lipgloss.Style
}

func rebuildFormViewStyles() {
	formViewStyles.sectionHeader = lipgloss.NewStyle().
		Foreground(tui.ColorAccent()).
		Bold(true)
	formViewStyles.section = lipgloss.NewStyle().
		PaddingLeft(2)
	formViewStyles.completedRender = lipgloss.NewStyle().
		Foreground(tui.ColorSuccess()).
		Bold(true).
		Render(tui.IconSuccess)
	formViewStyles.activeRender = lipgloss.NewStyle().
		Foreground(tui.ColorPrimary()).
		Bold(true).
		Render(tui.IconActive)
	formViewStyles.pendingRender = lipgloss.NewStyle().
		Foreground(tui.ColorSubtle()).
		Render(tui.IconPending)
	formViewStyles.note = lipgloss.NewStyle().
		Foreground(tui.ColorTextFaint()).
		Italic(true).
		PaddingLeft(2)
	formViewStyles.warning = lipgloss.NewStyle().
		Foreground(tui.ColorWarning())
}

// View renders the form and any extra content as an info card.
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

// RenderInfoCard renders body as a bordered card exactly width columns wide.
func RenderInfoCard(title, body string, width int) string {
	return tui.Card(title, lipgloss.Wrap(body, width-4, ""), width, tui.ColorSubtle())
}

// SetString adapts a string setter into a ConfigSetter.
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

// GetInt adapts an int getter into a ConfigGetter.
func GetInt(getter func(cfg *config.Config) int) ConfigGetter {
	return func(cfg *config.Config) string {
		return strconv.Itoa(getter(cfg))
	}
}

// SetSize propagates geometry to every section's input group.
func (s *DataDrivenStep) SetSize(width, height int) {
	s.BaseStep.SetSize(width, height)
	s.form.SetWidth(width)
}

// FocusBounds reports the focused field's span for the FocusedBounds
// interface; FocusedSpan is the same data as a LineSpan.
func (s *DataDrivenStep) FocusBounds(_, _ int) (top, bottom int, ok bool) {
	span, ok := s.FocusedSpan()
	if !ok {
		return 0, 0, false
	}
	return span.Start, span.End, true
}

// SetWidth propagates the content width to every section's group.
func (f *MultiSectionForm) SetWidth(width int) {
	innerWidth := f.innerWidth(width)
	for _, section := range f.sections {
		if section.Group != nil {
			section.Group.SetWidth(innerWidth)
		}
	}
}

// FocusBounds returns the focused field's span for the FocusedBounds
// interface.
func (f *MultiSectionForm) FocusBounds() (topBound, bottomBound int, ok bool) {
	span, ok := f.FocusedSpan()
	if !ok {
		return 0, 0, false
	}
	return span.Start, span.End, true
}

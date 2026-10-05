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
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

// ErrFixHighlighted is the status-row message shown when enter is pressed
// with invalid fields; FocusFirstInvalid has already moved focus and
// scrolled to the first one.
var ErrFixHighlighted = errors.New("fix the highlighted fields to continue")

// crossFieldError is a StepDefinition.Validate error that also names the
// field keys it implicates.
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

// FieldWidth sets an input box's preferred width, clamped to its section.
type FieldWidth int

// formFieldChrome is the section indent and field-row padding a box's
// preferred width must clear beneath formMaxWidth (model.go), so
// FieldWidthAuto and FieldWidthPath track the form column's own measure
// instead of a bigger guessed constant each time a box falls short of it.
const formFieldChrome = 8

// Field width classes for data-driven step definitions.
const (
	FieldWidthAuto   FieldWidth = 0 // zero value — special-cased in Cols
	FieldWidthNumber FieldWidth = 16
	FieldWidthPath   FieldWidth = formMaxWidth - 4
	FieldWidthFull   FieldWidth = -1 // the whole inner width
)

// Cols resolves w to a concrete box width in columns, clamped to avail;
// FieldWidthFull always returns avail itself, and FieldWidthAuto derives
// from formMaxWidth rather than a bare literal so a wide form column
// doesn't leave the box stranded short of the space it was given.
func (w FieldWidth) Cols(avail int) int {
	if w == FieldWidthFull {
		return avail
	}
	if w == FieldWidthAuto {
		return min(formMaxWidth-formFieldChrome, avail)
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

	// PairKey groups this field with the other fields in the same section
	// sharing a non-empty, identical PairKey into one visual row when the
	// available width comfortably fits the group (see pairMinInnerWidth);
	// declared per field, not automatic. Fields sharing a PairKey must be
	// adjacent in Fields.
	PairKey string

	ConfigSet ConfigSetter
	ConfigGet ConfigGetter
}

// SectionDefinition groups related fields under a shared title/note.
type SectionDefinition struct {
	Title   string
	Note    string // e.g. prerequisites, shown below the title
	Fields  []FieldDefinition
	Warning func(values map[string]string) string // non-empty return renders a warning block under the section's fields
	Visible func(values map[string]string) bool   // nil means always visible; false hides the section from render, navigation, and validation

	// Collapsible marks a section whose rendering may fold to one summary
	// line, orthogonal to Visible: Validate, TouchAll, and
	// FocusFirstInvalid never skip a Collapsible section — only View's
	// chrome changes — so a fold may never hide a required field or one
	// carrying an error from validation or focus.
	Collapsible bool
	// FoldSummary, when Collapsible is true, supplies the facts the
	// section's one-line collapsed summary echoes; nil renders the label
	// alone. Never return a fact whose Value is credential material.
	FoldSummary func(values map[string]string) []tui.FactRow

	// AbsenceNote, when the section is hidden (Visible returns false),
	// supplies one line of explanation rendered in its place; nil or an
	// empty return renders nothing, preserving the section's prior silent
	// skip.
	AbsenceNote func(values map[string]string) string
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

	// Answered, when set, summarizes the step's current values as facts for
	// the wide-terminal context pane's CONFIGURED section; a definition that
	// leaves it nil contributes nothing there.
	Answered func(values map[string]string) []render.Fact
}

// FormSection pairs a titled section with its built InputGroup — the
// runtime counterpart to SectionDefinition that MultiSectionForm navigates across.
type FormSection struct {
	Title   string
	Note    string // e.g. prerequisites, shown below the title
	Group   *components.InputGroup
	Warning func() string // non-empty return renders a warning block under the section's fields
	Visible func() bool   // nil means always visible

	// Collapsible mirrors SectionDefinition.Collapsible; when true,
	// Group.Fields()[0] must be a components.Foldable (NewDataDrivenStep
	// guarantees this by prepending a components.FoldToggleField).
	Collapsible bool

	// absenceNote mirrors SectionDefinition.AbsenceNote, resolved against
	// live values the same way Warning is.
	absenceNote func() string

	// pairKeys[j] is Fields[j].PairKey, aligned to Group.Fields() order —
	// View's pairing pass reads this instead of walking back to the
	// declarative FieldDefinition.
	pairKeys []string
}

// absenceText returns the section's AbsenceNote text, or "" when it has
// none — mirroring warningText's nil-safe pattern.
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

// isVisible reports whether the section should render, receive focus, and
// participate in validation; a nil Visible predicate means always visible.
func (s *FormSection) isVisible() bool {
	if s.Visible == nil {
		return true
	}
	return s.Visible()
}

// isComplete reports whether every field in the section passes Check, using
// the pure check rather than Validate so computing a section-complete
// indicator on every render never paints error state onto a field the user
// hasn't touched. An optional blank field (token id, vip, ntp server) is
// complete — only Required enforces non-emptiness — so a section is never
// pinned pending by a field that may legitimately stay empty.
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

// ConsumesTextInput reports whether the field currently holding focus would
// consume a "?" keystroke as literal typed text (components.TextInputField)
// rather than a keybinding — the answer DataDrivenStep and ParamsStep hand
// the wizard so it knows whether "?" should type or toggle the help
// overlay.
func (f *MultiSectionForm) ConsumesTextInput() bool {
	field := f.FocusedField()
	if field == nil {
		return false
	}
	tc, ok := field.(components.TextInputField)
	return ok && tc.ConsumesTextInput()
}

// Init focuses the first visible input group so the user can type immediately.
func (f *MultiSectionForm) Init() tea.Cmd {
	f.currentSection = f.firstVisible()
	if f.currentSection >= 0 && f.sections[f.currentSection].Group != nil {
		return f.sections[f.currentSection].Group.Focus()
	}
	return nil
}

// firstVisible returns the index of the first visible section, or -1 if none.
func (f *MultiSectionForm) firstVisible() int {
	return f.nextVisible(-1)
}

// nextVisible returns the index of the first visible section after i, or -1
// if none of the remaining sections are visible.
func (f *MultiSectionForm) nextVisible(i int) int {
	for j := i + 1; j < len(f.sections); j++ {
		if f.sections[j].isVisible() {
			return j
		}
	}
	return -1
}

// prevVisible returns the index of the first visible section before i, or -1
// if none of the preceding sections are visible.
func (f *MultiSectionForm) prevVisible(i int) int {
	for j := i - 1; j >= 0; j-- {
		if f.sections[j].isVisible() {
			return j
		}
	}
	return -1
}

// FocusField moves focus to a visible field by its section and field indexes.
func (f *MultiSectionForm) FocusField(section, field int) tea.Cmd {
	if section < 0 || section >= len(f.sections) || !f.sections[section].isVisible() {
		return nil
	}
	group := f.sections[section].Group
	if group == nil || group.Field(field) == nil {
		return nil
	}
	f.currentSection = section
	group.SetFocusIndex(field)
	return group.Focus()
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

// Update handles tab/shift-tab section navigation and forwards other input
// to the focused group. On enter it reports enterPressed=true without
// validating or completing — the caller layers that — unless the focused
// field consumes enter for its own editing flow (components.EnterConsumer),
// in which case the keystroke is routed to the field instead.
func (f *MultiSectionForm) Update(msg tea.Msg) (cmd tea.Cmd, enterPressed bool) {
	group := f.currentGroup()
	if group == nil {
		return nil, false
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if owner, ok := group.Field(group.FocusIndex()).(interface{ OwnsKey(tea.KeyPressMsg) bool }); ok && owner.OwnsKey(keyMsg) {
			_, cmd := group.Update(msg)
			return cmd, false
		}
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

// Validate returns every error from every visible section's group, running
// Check (via each field's Validate) across the whole form rather than
// stopping at the first invalid section, so every invalid field's error is
// current for View after a submission attempt. A hidden section's fields
// never block submission — the operator never saw them.
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

// TouchAll marks every field in every visible section touched and records
// its current error, ahead of Validate, so enter's forced submission attempt
// paints every field's real state rather than only the ones the user has
// visited. Hidden sections are left untouched.
func (f *MultiSectionForm) TouchAll() {
	for _, section := range f.sections {
		if section.Group != nil && section.isVisible() {
			section.Group.TouchAll()
		}
	}
}

// FocusFirstInvalid moves focus to the first field in a visible section
// (scanning sections in order) that fails Check, and returns a command that
// runs any focus side effect followed by FocusChangedMsg so the wizard
// re-syncs its viewport and scrolls the field into view.
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

// focusAt blurs the current group and focuses the field at section/field,
// returning a command that runs the focus side effect followed by
// FocusChangedMsg so the wizard re-syncs its viewport and scrolls the field
// into view — the same focus switch FocusFirstInvalid performs, reused by a
// cross-field error's implicated-field focus.
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

// foldState returns section i's fold toggle and whether the section is
// displaying in full this render — forced by the sticky expand latch, a
// focused field somewhere inside the section, or a validation error
// somewhere inside it (section.isComplete(), which never skips a field
// because it hasn't been visited the way Check vs. Validate already
// distinguishes elsewhere) — sets the toggle's display chrome to match,
// and reports ok=false when section isn't Collapsible or its first field
// isn't a components.Foldable, in which case open is unconditionally true:
// a construction bug must never read as "hide this section's fields".
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

// pairGap is the blank columns lipgloss.JoinHorizontal inserts between a
// declared field pair's rendered columns.
const pairGap = 4

// pairGapStr is pairGap rendered as blank columns.
var pairGapStr = strings.Repeat(" ", pairGap)

// pairMinWidth is the floor View's own width parameter needs before a
// declared pair renders side by side — exactly the width the 80-column
// "ordinary" floor hands View after the wizard's own chrome and viewport
// insets, once (the form section's own 4-column padding is still to come;
// comparing against View's width rather than the further-reduced innerWidth
// keeps this one constant stable across that one extra layer of
// subtraction). See formPaneWidths before changing this: past 150 columns
// the split layout's form column is fixed at formMaxWidth, so a wider
// terminal does not widen a pair's columns further. Below it (the 60-79
// "compact" tier) every declared pair falls back to single column.
const pairMinWidth = 70

// pairDefaultTagged is implemented by a paired field that may carry a
// "default" tag.
type pairDefaultTagged interface {
	HasDefaultTag() bool
}

// pairBoxWidthSetter is implemented by a paired field whose preferred box
// width can be capped independently of SetWidth's total column budget.
type pairBoxWidthSetter interface {
	SetBoxWidth(outer int)
}

// pairRuns partitions n field indexes into contiguous runs sharing one
// non-empty PairKey, with every unpaired field its own run of one;
// pairKeys shorter than n treats the missing tail as unpaired.
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

// mixedDefaultTags reports whether run contains at least one field that
// carries a "default" tag and at least one that doesn't — exactly the case
// where the tag would otherwise make one column's box narrower than its
// sibling's, since a field only reserves the tag's own room internally
// when it individually has one.
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

// applyPairWidths narrows each run of 2+ fields sharing a PairKey to an
// even share of innerWidth (minus pairGap between columns), overriding the
// blanket SetWidth every field in the section already received. A run
// mixing a default-tagged field with an untagged one reserves
// components.DefaultTagReserve in every member's box cap so the tag never
// makes one column's box wider than its sibling's (see mixedDefaultTags).
// Below pairMinWidth (checked against View's own width, not the
// further-reduced innerWidth — see pairMinWidth's doc), every run renders
// single column at full innerWidth instead.
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

// joinPairedViews joins views sharing a pairRuns run into one block per
// run, horizontally, so a declared 2- or 3-up pair renders as a single
// row; covered[i] lists the original field indexes block i represents, so
// the caller can record one combined LineSpan for the whole run. Below
// pairMinWidth every run is already length 1 (applyPairWidths declined to
// pair it), so this just emits each view on its own line.
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

// View renders each visible section as a head block followed by one block
// per field, one blank row apart, recording the line span every field
// occupies so the wizard can scroll the focused one into view. A hidden
// section (Visible returning false) contributes nothing.
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
			// Collapsed: the fold's own row is the section's entire
			// rendering — the HARD CONSTRAINT holds structurally, since
			// Validate/TouchAll/FocusFirstInvalid below never consult
			// Collapsible and so never skip the fields this hides.
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

// DataDrivenStep renders a multi-section form built from a StepDefinition, implementing WizardStep.
type DataDrivenStep struct {
	BaseStep

	definition    *StepDefinition
	draftFocusKey string
	fieldKeys     map[string]fieldLocation

	form *MultiSectionForm

	// customExtraContent, when non-nil, overrides definition.ExtraContent (set
	// via WithExtraContentFunc); customExtraContentTitle is its info card title.
	customExtraContent      func(width int) string
	customExtraContentTitle string
	customPinnedFooter      func(width int) string
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
		fields := make([]components.FormField, 0, len(sectionDef.Fields)+1)
		pairKeys := make([]string, 0, len(sectionDef.Fields)+1)

		// fieldOffset accounts for the synthetic fold toggle a Collapsible
		// section prepends: every declared FieldDefinition's real index in
		// Group.Fields() shifts by one past it.
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

// foldSummaryFunc adapts sectionDef.FoldSummary into the closure
// components.FoldToggleField calls each render, or nil when the section
// declares none.
func foldSummaryFunc(sectionDef *SectionDefinition, step *DataDrivenStep) func() []tui.FactRow {
	if sectionDef.FoldSummary == nil {
		return nil
	}
	return func() []tui.FactRow { return sectionDef.FoldSummary(step.rawValues()) }
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

// Definition returns the declarative fields and validation rules for s.
func (s *DataDrivenStep) Definition() *StepDefinition { return s.definition }

// SetValue updates a field without changing the step's focus or validation state.
func (s *DataDrivenStep) SetValue(fieldKey, value string) bool {
	if s.getField(fieldKey) == nil {
		return false
	}
	s.setValue(fieldKey, value)
	return true
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

// rawValues returns every field's current value regardless of section
// visibility; sectionDef.Visible predicates read this rather than values()
// so computing a section's visibility never recurses back into a values()
// that depends on visibility already being known.
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

// Init focuses the first input group so the user can type immediately.
func (s *DataDrivenStep) Init() tea.Cmd {
	if s.draftFocusKey == "" {
		return s.form.Init()
	}
	loc := s.fieldKeys[s.draftFocusKey]
	cmd := s.form.FocusField(loc.section, loc.field)
	s.draftFocusKey = ""
	return cmd
}

// SetFocused toggles step focus; when re-focused, focus returns to the
// first section.
func (s *DataDrivenStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if focused {
		_ = s.form.Focus()
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
		{Key: HelpCtrlC, Help: HelpQuit},
	}
	if h, ok := s.form.FocusedField().(components.KeyHinter); ok {
		for _, hint := range h.KeyHints() {
			bindings = append(bindings, KeyBinding{Key: hint.Key, Help: hint.Help})
		}
	}
	return bindings
}

// OwnsKey delegates cell-edit keys before flow navigation handles them.
func (s *DataDrivenStep) OwnsKey(msg tea.KeyPressMsg) bool {
	field := s.form.FocusedField()
	if field == nil {
		return false
	}
	owner, ok := field.(interface{ OwnsKey(tea.KeyPressMsg) bool })
	return ok && owner.OwnsKey(msg)
}

// ConsumesTextInput reports whether the focused field is mid-text-entry,
// per TextInputConsumer.
func (s *DataDrivenStep) ConsumesTextInput() bool {
	return s.form.ConsumesTextInput()
}

// Update forwards input to the embedded form and, on enter, touches and
// validates every field (scrolling to and reporting the first invalid one
// on failure) before running definition-aware validation and emitting
// StepCompleteMsg.
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

// focusCrossFieldError focuses/reveals the first field cfe implicates, then
// marks every implicated field invalid inline via components.FieldErrorSetter
// — in that order, since focusAt's blur pass would otherwise re-run
// Validate on a touched field and erase the error this sets.
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

// Validate runs the form's field validation, then the step-level Validate
// function if the definition provides one.
func (s *DataDrivenStep) Validate() error {
	if errs := s.form.Validate(); len(errs) > 0 {
		return errs[0]
	}
	if s.definition.Validate != nil {
		return s.redactedDefinitionValidate()
	}
	return nil
}

// redactedDefinitionValidate runs the step-level Validate function and
// scrubs any password field's live value out of the resulting error text —
// a cross-field Validate composes its message from arbitrary field values,
// and a password's raw value must never round-trip into a rendered error.
// A crossFieldError's field-implication list survives the scrub so
// focusCrossFieldError can still highlight the right fields.
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

// Apply writes each field's value into cfg using its ConfigSet, then runs
// the step-level Apply function if provided.
func (s *DataDrivenStep) Apply(cfg *config.Config) error {
	for sIdx := range s.definition.Sections {
		if !s.form.sections[sIdx].isVisible() {
			continue
		}
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

// DraftFieldKey returns the focused field key when it does not identify a credential.
func (s *DataDrivenStep) DraftFieldKey() string {
	section := s.form.currentSection
	group := s.form.currentGroup()
	if group == nil {
		return ""
	}
	index := group.FocusIndex()
	for key, loc := range s.fieldKeys {
		if loc.section == section && loc.field == index && s.draftSafeField(key) {
			return key
		}
	}
	return ""
}

// SetDraftFieldKey queues a safe field to focus when this step is resumed.
func (s *DataDrivenStep) SetDraftFieldKey(fieldKey string) bool {
	loc, ok := s.fieldKeys[fieldKey]
	if !ok || !s.draftSafeField(fieldKey) || !s.form.sections[loc.section].isVisible() {
		return false
	}
	s.draftFocusKey = fieldKey
	return true
}

func (s *DataDrivenStep) draftSafeField(fieldKey string) bool {
	lower := strings.ToLower(fieldKey)
	for _, sensitive := range []string{"password", "secret", "token", "credential", "username"} {
		if strings.Contains(lower, sensitive) {
			return false
		}
	}
	for sectionIdx := range s.definition.Sections {
		for fieldIdx := range s.definition.Sections[sectionIdx].Fields {
			field := &s.definition.Sections[sectionIdx].Fields[fieldIdx]
			if field.Key == fieldKey {
				return field.Type != FieldTypePassword
			}
		}
	}
	return false
}

// FocusedSpan reports the line range the focused field occupied in the last
// View; the form's blocks start at the step's own line 0, so no offset applies.
func (s *DataDrivenStep) FocusedSpan() (LineSpan, bool) {
	return s.form.FocusedSpan()
}

// Answered summarizes the step's current values via the definition's
// Answered hook, or reports nothing when the definition leaves it nil.
func (s *DataDrivenStep) Answered() []render.Fact {
	if s.definition.Answered == nil {
		return nil
	}
	return s.definition.Answered(s.values())
}

// FocusedFieldHelp reports the form's currently focused field's label and
// help text, or ok=false when there is no focused field or it carries no
// help text.
func (s *DataDrivenStep) FocusedFieldHelp() (label, help string, ok bool) {
	field := s.form.FocusedField()
	if field == nil {
		return "", "", false
	}
	lf, isLabeled := field.(components.LabeledField)
	if !isLabeled || lf.FieldHelp() == "" {
		return "", "", false
	}
	return lf.FieldLabel(), lf.FieldHelp(), true
}

// ShouldShow reports whether this step is visible given the current cfg.
func (s *DataDrivenStep) ShouldShow(cfg *config.Config) bool {
	if s.definition.ShouldShow != nil {
		return s.definition.ShouldShow(cfg)
	}
	return true
}

// formViewStyles caches DataDrivenStep.View's lipgloss styles; it captures
// accent, brand, and status roles that all rebind on the background flip,
// so rebuildFormViewStyles rebuilds it with the rest of the wizard styles.
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
	return tui.Card(title, lipgloss.Wrap(body, width-4, ""), width, tui.ColorSubtle())
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

// SetSize propagates geometry to every section's input group before rendering.
func (s *DataDrivenStep) SetSize(width, height int) {
	s.BaseStep.SetSize(width, height)
	s.form.SetWidth(width)
}

// FocusBounds reports the focused field's line span within the step's own
// View, for callers that only have the FocusedBounds-shaped interface
// (model_navigation.go's autoScrollToField); FocusedSpan is the same data
// in LineSpan form and the one the rest of the package uses directly.
func (s *DataDrivenStep) FocusBounds(_, _ int) (top, bottom int, ok bool) {
	span, ok := s.FocusedSpan()
	if !ok {
		return 0, 0, false
	}
	return span.Start, span.End, true
}

// SetWidth propagates the available content width to every section's group.
func (f *MultiSectionForm) SetWidth(width int) {
	innerWidth := f.innerWidth(width)
	for _, section := range f.sections {
		if section.Group != nil {
			section.Group.SetWidth(innerWidth)
		}
	}
}

// FocusBounds returns the focused field's span in the same LineSpan shape
// FocusedSpan reports, for callers restricted to the FocusedBounds interface.
func (f *MultiSectionForm) FocusBounds() (topBound, bottomBound int, ok bool) {
	span, ok := f.FocusedSpan()
	if !ok {
		return 0, 0, false
	}
	return span.Start, span.End, true
}

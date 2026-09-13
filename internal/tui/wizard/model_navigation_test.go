package wizard

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

// fakeStep is a minimal WizardStep double, avoiding an import cycle with the steps package.
type fakeStep struct {
	id         StepID
	focused    bool
	applyCalls int
	shouldShow func(cfg *config.Config) bool
}

func (f *fakeStep) ID() StepID                           { return f.id }
func (f *fakeStep) Title() string                        { return string(f.id) }
func (f *fakeStep) Init() tea.Cmd                        { return nil }
func (f *fakeStep) Update(tea.Msg) (WizardStep, tea.Cmd) { return f, nil }
func (f *fakeStep) View(int, int) string                 { return string(f.id) }
func (f *fakeStep) IsFocused() bool                      { return f.focused }
func (f *fakeStep) SetFocused(focused bool)              { f.focused = focused }
func (f *fakeStep) SetSize(int, int)                     {}

func (f *fakeStep) Apply(*config.Config) error {
	f.applyCalls++
	return nil
}

func (f *fakeStep) ShouldShow(cfg *config.Config) bool {
	if f.shouldShow == nil {
		return true
	}
	return f.shouldShow(cfg)
}

// growingStep is a SpanProvider whose View grows on demand, exercising the
// resync-then-scroll order on FocusChangedMsg.
type growingStep struct {
	BaseStep
	rows     int
	centered bool
}

func (g *growingStep) IsCentered() bool { return g.centered }

func (g *growingStep) Init() tea.Cmd                        { return nil }
func (g *growingStep) Update(tea.Msg) (WizardStep, tea.Cmd) { return g, nil }

func (g *growingStep) View(int, int) string {
	lines := make([]string, g.rows)
	for i := range lines {
		lines[i] = fmt.Sprintf("row %d", i)
	}
	return strings.Join(lines, "\n")
}

func (g *growingStep) FocusedSpan() (LineSpan, bool) {
	return LineSpan{Start: g.rows - 1, End: g.rows - 1}, true
}

// fakeReviewStep additionally implements ReviewJumper, standing in for steps.ReviewStep.
type fakeReviewStep struct {
	fakeStep
	order   []StepID
	targets []JumpTarget
}

func (r *fakeReviewStep) JumpOrder() []StepID           { return r.order }
func (r *fakeReviewStep) SetJumpTargets(t []JumpTarget) { r.targets = t }

func update(t *testing.T, m *Model, msg tea.Msg) *Model {
	t.Helper()
	mm, _ := m.Update(msg)
	return mm.(*Model)
}

// newNavTestSteps builds [basics, proxmox, networking, review]; review is
// returned to inspect computed jump targets.
func newNavTestSteps(order []StepID) ([]WizardStep, *fakeReviewStep) {
	review := &fakeReviewStep{fakeStep: fakeStep{id: StepIDReview}, order: order}
	steps := []WizardStep{
		&fakeStep{id: StepIDBasics},
		&fakeStep{id: StepIDProxmox},
		&fakeStep{id: StepIDNetworking},
		review,
	}
	return steps, review
}

// advanceToReview drives forward with confirms until review, tolerant of hidden intermediate steps.
func advanceToReview(t *testing.T, m *Model) *Model {
	t.Helper()
	for range len(m.steps) {
		if m.CurrentStep().ID() == StepIDReview {
			return m
		}
		m = update(t, m, StepCompleteMsg{})
	}
	t.Fatalf("setup: CurrentStep() = %v, want review", m.CurrentStep().ID())
	return m
}

func TestModel_JumpFromReview_ConfirmReturnsToReview(t *testing.T) {
	steps, _ := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
	m := NewModel(steps, &config.Config{})
	m = advanceToReview(t, m)

	m = update(t, m, JumpToStepMsg{StepID: StepIDProxmox})
	if got := m.CurrentStep().ID(); got != StepIDProxmox {
		t.Fatalf("after jump: CurrentStep() = %v, want proxmox", got)
	}

	m = update(t, m, StepCompleteMsg{StepID: StepIDProxmox})
	if got := m.CurrentStep().ID(); got != StepIDReview {
		t.Fatalf("after confirm: CurrentStep() = %v, want review (not the intermediate replay)", got)
	}
}

func TestModel_JumpFromReview_EscReturnsToReview(t *testing.T) {
	steps, _ := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
	m := NewModel(steps, &config.Config{})
	m = advanceToReview(t, m)

	m = update(t, m, JumpToStepMsg{StepID: StepIDProxmox})

	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := m.CurrentStep().ID(); got != StepIDReview {
		t.Fatalf("after esc: CurrentStep() = %v, want review (not one step back)", got)
	}
}

func TestModel_JumpFromReview_EscDoesNotApplyEditedStep(t *testing.T) {
	steps, _ := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
	m := NewModel(steps, &config.Config{})
	m = advanceToReview(t, m)

	m = update(t, m, JumpToStepMsg{StepID: StepIDProxmox})

	// advanceToReview already applies every step once; compare against a snapshot, not zero.
	proxmoxStep := steps[1].(*fakeStep)
	before := proxmoxStep.applyCalls

	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	if proxmoxStep.applyCalls != before {
		t.Errorf("Apply() called during esc (calls %d -> %d), want unchanged (esc discards edits everywhere else)", before, proxmoxStep.applyCalls)
	}
}

func TestModel_SyncJumpTargets_HiddenStepCompactsIndexes(t *testing.T) {
	steps, review := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
	steps[1].(*fakeStep).shouldShow = func(*config.Config) bool { return false }

	m := NewModel(steps, &config.Config{})
	advanceToReview(t, m)

	want := []JumpTarget{
		{StepID: StepIDBasics, Digit: 1},
		{StepID: StepIDNetworking, Digit: 2},
	}
	if len(review.targets) != len(want) {
		t.Fatalf("targets = %+v, want %+v", review.targets, want)
	}
	for i := range want {
		if review.targets[i] != want[i] {
			t.Errorf("targets[%d] = %+v, want %+v", i, review.targets[i], want[i])
		}
	}
}

func TestModel_JumpToStep_RefusesHiddenTarget(t *testing.T) {
	steps, _ := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
	steps[1].(*fakeStep).shouldShow = func(*config.Config) bool { return false }

	m := NewModel(steps, &config.Config{})
	m = advanceToReview(t, m)

	m = update(t, m, JumpToStepMsg{StepID: StepIDProxmox})
	if got := m.CurrentStep().ID(); got != StepIDReview {
		t.Fatalf("jump to hidden step: CurrentStep() = %v, want unchanged review", got)
	}
}

func TestModel_DigitKeyOutsideReviewUnaffected(t *testing.T) {
	steps, _ := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
	m := NewModel(steps, &config.Config{})

	m = update(t, m, tea.KeyPressMsg{Code: '2', Text: "2"})
	if got := m.CurrentStep().ID(); got != StepIDBasics {
		t.Fatalf("digit key on non-review step: CurrentStep() = %v, want basics (unaffected)", got)
	}
}

// scrollTestDefinition mirrors the addons step's shape: several sections of
// text and select fields, with help text long enough to wrap at 80 columns.
func scrollTestDefinition() *StepDefinition {
	sections := make([]SectionDefinition, 0, 4)
	for s := range 4 {
		fields := make([]FieldDefinition, 0, 4)
		for f := range 4 {
			def := FieldDefinition{
				Key:     fmt.Sprintf("s%df%d", s, f),
				Label:   fmt.Sprintf("section %d field %d", s, f),
				Default: fmt.Sprintf("value-%d-%d", s, f),
				Help:    "a deliberately long hint that wraps once the wizard renders it at eighty columns",
			}
			if f == 1 {
				def.Type = FieldTypeSelect
				def.Options = []string{"no", testValYes}
			}
			fields = append(fields, def)
		}
		sections = append(sections, SectionDefinition{
			Title:  fmt.Sprintf("section %d", s),
			Note:   "a note that is itself long enough to need a second row at eighty columns wide",
			Fields: fields,
		})
	}
	return &StepDefinition{
		ID:       StepIDAddons,
		Title:    "scroll fixture",
		Sections: sections,
	}
}

func TestModel_ScrollKeepsFocusedFieldFullyVisible(t *testing.T) {
	step := NewDataDrivenStep(scrollTestDefinition())
	m := NewModel([]WizardStep{step}, config.DefaultConfig())
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	if m.viewport.TotalLineCount() <= m.viewport.Height() {
		t.Fatal("setup: fixture step does not overflow the viewport")
	}

	for i := range 12 {
		m = update(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
		m = update(t, m, FocusChangedMsg{})

		provider, ok := m.steps[m.currentStep].(SpanProvider)
		if !ok {
			t.Fatal("step does not implement SpanProvider")
		}
		span, ok := provider.FocusedSpan()
		if !ok {
			t.Fatalf("tab %d: no focused span", i)
		}
		start, end := m.viewportSpan(span)
		top, height := m.viewport.YOffset(), m.viewport.Height()
		if start < top || end >= top+height {
			t.Fatalf("tab %d: span %+v maps to rows [%d,%d], outside viewport [%d,%d)", i, span, start, end, top, top+height)
		}

		label := fmt.Sprintf("section %d field %d", (i+1)/4, (i+1)%4)
		if !strings.Contains(m.View().Content, label) {
			t.Fatalf("tab %d: focused field %q is not on screen", i, label)
		}
	}
}

func TestModel_CenteredStepIsNotSpanScrolled(t *testing.T) {
	step := &growingStep{BaseStep: NewBaseStep(StepIDWelcome, "grower", ""), rows: 120, centered: true}
	m := NewModel([]WizardStep{step}, config.DefaultConfig())
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = update(t, m, FocusChangedMsg{})

	if got := m.viewport.YOffset(); got != 0 {
		t.Fatalf("YOffset() = %d; a centered step's rows do not map to its View lines, so it must not be span-scrolled", got)
	}
}

func TestModel_FocusChangedResyncsBeforeScroll(t *testing.T) {
	step := &growingStep{BaseStep: NewBaseStep(StepIDBasics, "grower", ""), rows: 5}
	m := NewModel([]WizardStep{step}, config.DefaultConfig())
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	before := m.viewport.TotalLineCount()
	step.rows = 120
	m = update(t, m, FocusChangedMsg{})

	if got := m.viewport.TotalLineCount(); got <= before {
		t.Fatalf("TotalLineCount() = %d after growth, want more than %d (content not resynced)", got, before)
	}
	if m.viewport.YOffset() == 0 {
		t.Fatal("YOffset() = 0; want the focused last row scrolled into view")
	}
}

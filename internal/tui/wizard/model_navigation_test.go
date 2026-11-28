package wizard

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

type fakeStep struct {
	id         StepID
	focused    bool
	applyCalls int
	shouldShow func(cfg *config.Config) bool
	validate   func() error
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

func advanceToReview(t *testing.T, m *Model) *Model {
	t.Helper()
	for range len(m.steps) {
		if m.CurrentStep().ID() == StepIDReview {
			return m
		}
		m = update(t, m, StepCompleteMsg{StepID: m.CurrentStep().ID()})
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

func TestModel_ReviewCompletionNamesReviewAsExitStep(t *testing.T) {
	steps, _ := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
	m := NewModel(steps, &config.Config{})
	m = advanceToReview(t, m)

	m = update(t, m, StepCompleteMsg{StepID: StepIDReview})

	res := m.Result()
	if res.Outcome != OutcomeCompleted {
		t.Fatalf("Outcome = %v, want completed", res.Outcome)
	}
	if res.ExitStep != StepIDReview {
		t.Errorf("ExitStep = %q, want %q", res.ExitStep, StepIDReview)
	}
}

func TestNavigationMessagesCloseTheHelpOverlay(t *testing.T) {
	navModel := func(t *testing.T) *Model {
		t.Helper()
		steps, _ := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
		m := NewModel(steps, &config.Config{})
		tuitest.RenderAt(t, m, 100, 30)
		return m
	}

	t.Run("StepCompleteMsg", func(t *testing.T) {
		m := navModel(t)
		m.helpOpen = true

		m = update(t, m, StepCompleteMsg{StepID: StepIDBasics})

		if got := m.CurrentStep().ID(); got != StepIDProxmox {
			t.Fatalf("CurrentStep() = %v, want proxmox", got)
		}
		if m.helpOpen {
			t.Fatal("help overlay must close when StepCompleteMsg moves the current step underneath it")
		}
	})

	t.Run("StepBackMsg", func(t *testing.T) {
		m := navModel(t)
		m = update(t, m, StepCompleteMsg{StepID: StepIDBasics})
		m.helpOpen = true

		m = update(t, m, StepBackMsg{})

		if got := m.CurrentStep().ID(); got != StepIDBasics {
			t.Fatalf("CurrentStep() = %v, want basics", got)
		}
		if m.helpOpen {
			t.Fatal("help overlay must close when StepBackMsg moves the current step underneath it")
		}
	})

	t.Run("JumpToStepMsg", func(t *testing.T) {
		m := navModel(t)
		m.helpOpen = true

		m = update(t, m, JumpToStepMsg{StepID: StepIDProxmox})

		if got := m.CurrentStep().ID(); got != StepIDProxmox {
			t.Fatalf("CurrentStep() = %v, want proxmox", got)
		}
		if m.helpOpen {
			t.Fatal("help overlay must close when JumpToStepMsg moves the current step underneath it")
		}
	})
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
		ID:       StepID("scroll-test"),
		Title:    "scroll fixture",
		Sections: sections,
	}
}

func TestModel_BackNeverLandsOnHiddenStep(t *testing.T) {
	steps, _ := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
	steps[0].(*fakeStep).shouldShow = func(*config.Config) bool { return false }
	m := NewModel(steps, &config.Config{})
	m = update(t, m, StepCompleteMsg{StepID: m.CurrentStep().ID()})
	if got := m.CurrentStep().ID(); got != StepIDProxmox {
		t.Fatalf("setup: CurrentStep() = %v, want proxmox", got)
	}

	m = update(t, m, StepBackMsg{})

	if got := m.CurrentStep().ID(); got != StepIDProxmox {
		t.Fatalf("back landed on %v, want to stay on proxmox (basics is hidden)", got)
	}
}

func TestModel_StaleStepCompleteIgnored(t *testing.T) {
	steps, _ := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
	m := NewModel(steps, &config.Config{})

	m = update(t, m, StepCompleteMsg{StepID: StepIDNetworking})
	if got := m.CurrentStep().ID(); got != StepIDBasics {
		t.Fatalf("stale completion advanced the wizard to %v, want basics", got)
	}

	m = update(t, m, StepCompleteMsg{StepID: StepIDBasics})
	if got := m.CurrentStep().ID(); got != StepIDProxmox {
		t.Fatalf("matching completion did not advance: %v", got)
	}
}

func TestModel_HomeEndReachFocusedTextInput(t *testing.T) {
	def := &StepDefinition{
		ID:    StepIDBasics,
		Title: "t",
		Sections: []SectionDefinition{{
			Fields: []FieldDefinition{{Key: "name", Label: "name"}},
		}},
	}
	step := NewDataDrivenStep(def)
	m := NewModel([]WizardStep{step}, config.DefaultConfig())
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	for _, r := range "abc" {
		m = update(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyHome})
	m = update(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := step.Value("name"); got != "xabc" {
		t.Fatalf("value after home+type = %q, want %q", got, "xabc")
	}

	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEnd})
	_ = update(t, m, tea.KeyPressMsg{Code: 'z', Text: "z"})
	if got := step.Value("name"); got != "xabcz" {
		t.Fatalf("value after end+type = %q, want %q", got, "xabcz")
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

type recordingArrowStep struct {
	growingStep
	received []tea.Msg
}

func (s *recordingArrowStep) Update(msg tea.Msg) (WizardStep, tea.Cmd) {
	s.received = append(s.received, msg)
	return s, nil
}

func TestModel_VimKeysScrollTheViewport(t *testing.T) {
	in := &growingStep{BaseStep: NewBaseStep(StepIDWelcome, "reader", ""), rows: 120}
	m := NewModel([]WizardStep{in}, config.DefaultConfig())
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	m = update(t, m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if m.viewport.YOffset() == 0 {
		t.Fatal("ctrl+d did not scroll the viewport")
	}
	m = update(t, m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if got := m.viewport.YOffset(); got != 0 {
		t.Fatalf("YOffset() = %d after ctrl+u, want 0", got)
	}

	m = update(t, m, tea.KeyPressMsg{Code: 'G', Text: "G", Mod: tea.ModShift})
	if !m.viewport.AtBottom() {
		t.Fatal("G did not reach the bottom")
	}
	m = update(t, m, tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = update(t, m, tea.KeyPressMsg{Code: 'g', Text: "g"})
	if got := m.viewport.YOffset(); got != 0 {
		t.Fatalf("YOffset() = %d after gg, want the top", got)
	}
}

func TestModel_VimGPendingClearsOnOtherKeys(t *testing.T) {
	in := &growingStep{BaseStep: NewBaseStep(StepIDWelcome, "reader", ""), rows: 120}
	m := NewModel([]WizardStep{in}, config.DefaultConfig())
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	m = update(t, m, tea.KeyPressMsg{Code: 'G', Text: "G", Mod: tea.ModShift})
	bottom := m.viewport.YOffset()

	m = update(t, m, tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = update(t, m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	half := m.viewport.YOffset()
	if half == 0 || half >= bottom {
		t.Fatalf("YOffset() = %d after g then ctrl+u, want a half page above %d — ctrl+u scrolls, no gg jump", half, bottom)
	}
	m = update(t, m, tea.KeyPressMsg{Code: 'g', Text: "g"})
	if got := m.viewport.YOffset(); got != half {
		t.Fatalf("YOffset() = %d after a fresh lone g, want %d unchanged", got, half)
	}
}

func TestModel_VimKeysFallThroughToTextInputs(t *testing.T) {
	out := &recordingTextStep{recordingArrowStep{growingStep: growingStep{BaseStep: NewBaseStep(StepIDBasics, "form", ""), rows: 120}}}
	m := NewModel([]WizardStep{out}, config.DefaultConfig())
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	for _, k := range []tea.KeyPressMsg{
		{Code: 'j', Text: "j"},
		{Code: 'g', Text: "g"},
		{Code: 'G', Text: "G", Mod: tea.ModShift},
	} {
		m = update(t, m, k)
	}
	if got := m.viewport.YOffset(); got != 0 {
		t.Fatalf("YOffset() = %d, want 0 — vim keys must not scroll under a text input", got)
	}
	if len(out.received) < 3 {
		t.Fatalf("step received %d messages, want all three keys delivered", len(out.received))
	}
}

type recordingTextStep struct{ recordingArrowStep }

func (s *recordingTextStep) ConsumesTextInput() bool { return true }

func (s *recordingTextStep) Update(msg tea.Msg) (WizardStep, tea.Cmd) {
	s.received = append(s.received, msg)
	return s, nil
}

func TestModel_JumpFromReview_EscRefusesAnInvalidStep(t *testing.T) {
	steps, _ := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
	invalid := errors.New("cluster name: use lowercase letters")
	steps[1].(*fakeStep).validate = func() error { return invalid }
	m := NewModel(steps, &config.Config{})
	m = advanceToReview(t, m)
	m = update(t, m, JumpToStepMsg{StepID: StepIDProxmox})

	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := m.CurrentStep().ID(); got != StepIDProxmox {
		t.Fatalf("esc left an invalid step for %v, want to stay on proxmox", got)
	}
	if !errors.Is(m.err, invalid) {
		t.Fatalf("status row = %v, want the step's validation error", m.err)
	}

	steps[1].(*fakeStep).validate = nil
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := m.CurrentStep().ID(); got != StepIDReview {
		t.Fatalf("esc from a valid step landed on %v, want review", got)
	}
}

func (f *fakeStep) Validate() error {
	if f.validate == nil {
		return nil
	}
	return f.validate()
}

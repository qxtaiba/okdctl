package wizard

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
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

type paletteFakeStep struct {
	fakeStep
	selected string
}

func (s *paletteFakeStep) PaletteTargets() []PaletteTarget {
	return []PaletteTarget{{ID: "host", Kind: PaletteTargetField, Label: "Proxmox host", Detail: "Connection"}}
}

func (s *paletteFakeStep) FocusPaletteTarget(id string) tea.Cmd {
	s.selected = id
	return nil
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

func TestCommandPaletteSearchesAndJumpsToField(t *testing.T) {
	fieldStep := &paletteFakeStep{fakeStep: fakeStep{id: StepIDProxmox}}
	m := NewModel([]WizardStep{&fakeStep{id: StepIDBasics}, fieldStep, &fakeStep{id: StepIDReview}}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)

	m = update(t, m, tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	if !m.paletteOpen {
		t.Fatal("ctrl+k must open the command palette")
	}
	for _, r := range "host" {
		m = update(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if len(m.paletteMatches) != 1 || m.paletteMatches[0].target.ID != "host" || m.paletteMatches[0].stepIndex != 1 {
		t.Fatalf("matches = %#v, want the Proxmox host field", m.paletteMatches)
	}
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.paletteOpen || m.CurrentStep().ID() != StepIDProxmox || fieldStep.selected != "host" {
		t.Fatalf("palette result = open:%v step:%q target:%q", m.paletteOpen, m.CurrentStep().ID(), fieldStep.selected)
	}
}

func TestCommandPaletteFitsCommonTerminalSizes(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {100, 30}, {180, 48}} {
		m := NewModel([]WizardStep{&paletteFakeStep{fakeStep: fakeStep{id: StepIDProxmox}}}, config.DefaultConfig())
		tuitest.RenderAt(t, m, size[0], size[1])
		m.openPalette()
		tuitest.AssertFits(t, m.renderPalette(), m.contentWidth(), m.viewport.Height())
	}
}

func TestCommandPaletteKeepsSelectedResultVisiblePastFirstPage(t *testing.T) {
	steps := make([]WizardStep, 12)
	for i := range steps {
		steps[i] = &fakeStep{id: StepID(fmt.Sprintf("step-%02d", i))}
	}
	m := NewModel(steps, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)
	m.openPalette()
	for range 9 {
		m = update(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.paletteSelected != 9 {
		t.Fatalf("selected = %d, want 9", m.paletteSelected)
	}
	view := tuitest.StripANSI(m.renderPalette())
	if !strings.Contains(view, "step-09") {
		t.Fatalf("selected result is not visible after scrolling: %s", view)
	}
}

// TestPaletteClosesWhenAsyncStepCompleteMovesCurrentStepUnderneath is the
// reviewer's exact repro: the palette gates tea.KeyPressMsg but not
// StepCompleteMsg, which can arrive asynchronously (a step's own Cmd) while
// the palette is open. Before the fix, currentStep advanced 0->1 with the
// palette still open and rendering matches computed against the old step;
// activating one could jump to the wrong step. The fix closes the palette
// outright, so there is no rendered modal left whose targets could be stale.
func TestPaletteClosesWhenAsyncStepCompleteMovesCurrentStepUnderneath(t *testing.T) {
	steps, _ := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
	m := NewModel(steps, &config.Config{})
	tuitest.RenderAt(t, m, 100, 30)

	m = update(t, m, tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	if !m.paletteOpen {
		t.Fatal("ctrl+k must open the command palette")
	}

	// A late async completion for the step the palette was opened over,
	// exactly as a step's own Cmd would deliver it.
	m = update(t, m, StepCompleteMsg{StepID: StepIDBasics})

	if got := m.CurrentStep().ID(); got != StepIDProxmox {
		t.Fatalf("CurrentStep() = %v, want proxmox (navigation must still occur)", got)
	}
	if m.paletteOpen {
		t.Fatal("palette must close when navigation moves the current step underneath it")
	}
}

// TestNavigationMessagesCloseAnOpenModal exercises every navigation-mutating
// message the wizard switches on (model.go's update()) against an open
// palette, confirming each funnels through the same close point (focusStep)
// rather than leaving the palette open over a step it does not describe.
func TestNavigationMessagesCloseAnOpenModal(t *testing.T) {
	t.Run("StepBackMsg", func(t *testing.T) {
		steps, _ := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
		m := NewModel(steps, &config.Config{})
		tuitest.RenderAt(t, m, 100, 30)
		m = update(t, m, StepCompleteMsg{StepID: StepIDBasics}) // off step 0, palette untouched
		m.openPalette()

		m = update(t, m, StepBackMsg{})

		if got := m.CurrentStep().ID(); got != StepIDBasics {
			t.Fatalf("CurrentStep() = %v, want basics", got)
		}
		if m.paletteOpen {
			t.Fatal("palette must close when StepBackMsg moves the current step underneath it")
		}
	})

	t.Run("JumpToStepMsg", func(t *testing.T) {
		steps, _ := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
		m := NewModel(steps, &config.Config{})
		tuitest.RenderAt(t, m, 100, 30)
		m.openPalette()

		m = update(t, m, JumpToStepMsg{StepID: StepIDProxmox})

		if got := m.CurrentStep().ID(); got != StepIDProxmox {
			t.Fatalf("CurrentStep() = %v, want proxmox", got)
		}
		if m.paletteOpen {
			t.Fatal("palette must close when JumpToStepMsg moves the current step underneath it")
		}
	})

	t.Run("DraftResumeMsg", func(t *testing.T) {
		steps, _ := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
		m := NewModel(steps, &config.Config{})
		tuitest.RenderAt(t, m, 100, 30)
		m.openPalette()

		m = update(t, m, DraftResumeMsg{StepID: StepIDProxmox})

		if got := m.CurrentStep().ID(); got != StepIDProxmox {
			t.Fatalf("CurrentStep() = %v, want proxmox", got)
		}
		if m.paletteOpen {
			t.Fatal("palette must close when DraftResumeMsg moves the current step underneath it")
		}
	})

	t.Run("SwapFlowMsg", func(t *testing.T) {
		m, hub := swapModel(t)
		m.openPalette()

		hub.builds++
		flow := hub.build()
		m2, _ := m.Update(SwapFlowMsg{Steps: flow, Chrome: FlowChrome{Tagline: "sub-flow"}})
		m = m2.(*Model)

		if got := m.CurrentStep().ID(); got != "sub-first" {
			t.Fatalf("CurrentStep() = %v, want sub-first", got)
		}
		if m.paletteOpen {
			t.Fatal("palette must close when SwapFlowMsg replaces the step set underneath it")
		}
	})

	t.Run("StepCompleteMsg closes the help overlay too", func(t *testing.T) {
		steps, _ := newNavTestSteps([]StepID{StepIDBasics, StepIDProxmox, StepIDNetworking})
		m := NewModel(steps, &config.Config{})
		tuitest.RenderAt(t, m, 100, 30)
		m.helpOpen = true

		m = update(t, m, StepCompleteMsg{StepID: StepIDBasics})

		if got := m.CurrentStep().ID(); got != StepIDProxmox {
			t.Fatalf("CurrentStep() = %v, want proxmox", got)
		}
		if m.helpOpen {
			t.Fatal("help overlay must close when navigation moves the current step underneath it")
		}
	})
}

func TestPaletteScoreOrdersExactPrefixSubstringAndFuzzy(t *testing.T) {
	queries := []struct {
		query string
		label string
		want  int
	}{
		{"networking", "networking", 0},
		{"net", "networking", 1},
		{"work", "networking", 2},
		{"nwr", "networking", 3},
	}
	for _, tt := range queries {
		got, ok := paletteScore(tt.query, tt.label)
		if !ok || got != tt.want {
			t.Errorf("paletteScore(%q, %q) = %d, %v; want %d, true", tt.query, tt.label, got, ok, tt.want)
		}
	}
	if _, ok := paletteScore("missing", "networking"); ok {
		t.Error("unmatched query must not produce a result")
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

// TestModel_BackNeverLandsOnHiddenStep pins bug 17: when every earlier step
// is ShouldShow-hidden, back stays put instead of focusing a hidden screen.
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

// TestModel_StaleStepCompleteIgnored pins bug 16: a StepCompleteMsg carrying
// a step ID other than the current one is a late async completion from a
// step the user already left, and must not advance (and Apply) the current
// step.
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

// TestModel_HomeEndReachFocusedTextInput pins bug 6: home/end while a text
// input holds focus are line-start/line-end cursor moves inside the field,
// not viewport scrolls — the same consumes-text-input guard "?" already has.
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

// arrowOptInStep is a growingStep that opts into arrowScroller, standing in
// for the read-only done screens.
type arrowOptInStep struct{ growingStep }

func (s *arrowOptInStep) ScrollsWithArrows() bool { return true }

// recordingArrowStep records every message reaching its Update, standing in
// for a form step whose fields own the arrow keys.
type recordingArrowStep struct {
	growingStep
	received []tea.Msg
}

func (s *recordingArrowStep) Update(msg tea.Msg) (WizardStep, tea.Cmd) {
	s.received = append(s.received, msg)
	return s, nil
}

// TestModel_ArrowScrollOnlyForOptedInSteps pins the arrowScroller gate: an
// opted-in read-only step gets one-line viewport scrolling, while every other
// step keeps ↑/↓ for its own navigation and the viewport stays put.
func TestModel_ArrowScrollOnlyForOptedInSteps(t *testing.T) {
	in := &arrowOptInStep{growingStep{BaseStep: NewBaseStep(StepIDWelcome, "reader", ""), rows: 120}}
	m := NewModel([]WizardStep{in}, config.DefaultConfig())
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	if got := m.viewport.YOffset(); got != 1 {
		t.Fatalf("YOffset() = %d after ↓ on an opted-in step, want 1", got)
	}
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	if got := m.viewport.YOffset(); got != 0 {
		t.Fatalf("YOffset() = %d after ↑, want back at 0", got)
	}

	out := &recordingArrowStep{growingStep: growingStep{BaseStep: NewBaseStep(StepIDBasics, "form", ""), rows: 120}}
	m = NewModel([]WizardStep{out}, config.DefaultConfig())
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	if got := m.viewport.YOffset(); got != 0 {
		t.Fatalf("YOffset() = %d after ↓ on a non-opted-in step, want 0 — the step owns the arrows", got)
	}
	for _, msg := range out.received {
		if key, ok := msg.(tea.KeyPressMsg); ok && key.Code == tea.KeyDown {
			return
		}
	}
	t.Fatal("↓ never reached the step's own Update")
}

// TestModel_VimKeysScrollOptedInViewport pins the additive vim vocabulary:
// j/k mirror the arrow gate, ctrl+d/u the half-page keys, and gg/G jump to
// the ends — all footer-silent, none stolen from a step that needs the keys.
func TestModel_VimKeysScrollOptedInViewport(t *testing.T) {
	in := &arrowOptInStep{growingStep{BaseStep: NewBaseStep(StepIDWelcome, "reader", ""), rows: 120}}
	m := NewModel([]WizardStep{in}, config.DefaultConfig())
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if got := m.viewport.YOffset(); got != 1 {
		t.Fatalf("YOffset() = %d after j, want 1", got)
	}
	m = update(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	if got := m.viewport.YOffset(); got != 0 {
		t.Fatalf("YOffset() = %d after k, want 0", got)
	}

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

// TestModel_VimGPendingClearsOnOtherKeys pins the gg chord: a lone g followed
// by any other key never jumps, and the interloper key still does its job.
func TestModel_VimGPendingClearsOnOtherKeys(t *testing.T) {
	in := &arrowOptInStep{growingStep{BaseStep: NewBaseStep(StepIDWelcome, "reader", ""), rows: 120}}
	m := NewModel([]WizardStep{in}, config.DefaultConfig())
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	m = update(t, m, tea.KeyPressMsg{Code: 'G', Text: "G", Mod: tea.ModShift})
	bottom := m.viewport.YOffset()

	m = update(t, m, tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = update(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	if got := m.viewport.YOffset(); got != bottom-1 {
		t.Fatalf("YOffset() = %d after g then k, want %d — k scrolls, no gg jump", got, bottom-1)
	}
	m = update(t, m, tea.KeyPressMsg{Code: 'g', Text: "g"})
	if got := m.viewport.YOffset(); got != bottom-1 {
		t.Fatalf("YOffset() = %d after a fresh lone g, want %d unchanged", got, bottom-1)
	}
}

// TestModel_VimKeysFallThroughToTextInputs pins the additive rule: a step
// whose focused field consumes text keeps j/k/g as typed characters.
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

// recordingTextStep is a recordingArrowStep whose focused field consumes
// typed text, standing in for a form with a live input. Update returns the
// outer type — the embedded Update would hand the model the inner step and
// silently drop the TextInputConsumer assertion.
type recordingTextStep struct{ recordingArrowStep }

func (s *recordingTextStep) ConsumesTextInput() bool { return true }

func (s *recordingTextStep) Update(msg tea.Msg) (WizardStep, tea.Cmd) {
	s.received = append(s.received, msg)
	return s, nil
}

type draftCursorStep struct {
	fakeStep
	fieldKey string
	pending  string
}

func (s *draftCursorStep) DraftFieldKey() string { return s.fieldKey }
func (s *draftCursorStep) SetDraftFieldKey(key string) bool {
	s.pending = key
	return true
}

func TestModelSavesDraftAfterStepTransition(t *testing.T) {
	target := &draftCursorStep{fakeStep: fakeStep{id: StepIDBasics}, fieldKey: "cluster_name"}
	m := NewModel([]WizardStep{&fakeStep{id: StepIDWelcome}, target}, config.DefaultConfig())
	var gotID StepID
	var gotField string
	m.draftSaver = func(_ *config.Config, id StepID, fieldKey string) error {
		gotID, gotField = id, fieldKey
		return nil
	}

	_ = update(t, m, StepCompleteMsg{StepID: StepIDWelcome})
	if gotID != StepIDBasics || gotField != "cluster_name" {
		t.Fatalf("saved cursor = %s/%s, want basics/cluster_name", gotID, gotField)
	}
}

func TestModelSkipsDraftSaveForSwappedUtilityFlow(t *testing.T) {
	status := &fakeStep{id: StepID("cluster-status")}
	m := NewModel([]WizardStep{&fakeStep{id: StepIDWelcome}, status}, config.DefaultConfig())
	saves := 0
	m.draftSaver = func(*config.Config, StepID, string) error {
		saves++
		return nil
	}

	_ = update(t, m, StepCompleteMsg{StepID: StepIDWelcome})
	if saves != 0 {
		t.Fatalf("draft saves = %d after entering cluster status, want none", saves)
	}
}

func TestModelResumesDraftAtStepAndField(t *testing.T) {
	target := &draftCursorStep{fakeStep: fakeStep{id: StepIDNetworking}}
	m := NewModel([]WizardStep{&fakeStep{id: StepIDWelcome}, target}, config.DefaultConfig())
	m = update(t, m, DraftResumeMsg{StepID: StepIDNetworking, FieldKey: "machine_cidr"})

	if got := m.CurrentStep().ID(); got != StepIDNetworking {
		t.Fatalf("CurrentStep() = %s, want networking", got)
	}
	if target.pending != "machine_cidr" {
		t.Errorf("pending field = %q, want machine_cidr", target.pending)
	}
	if m.returnToReview {
		t.Error("draft resume must use ordinary wizard back/next navigation")
	}
}

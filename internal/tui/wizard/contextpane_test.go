package wizard

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

type answeredNopStep struct {
	nopStep
	facts []render.Fact
}

func (s *answeredNopStep) Answered() []render.Fact { return s.facts }

type focusedFieldNopStep struct {
	nopStep
	label, help string
	ok          bool
}

func (s *focusedFieldNopStep) FocusedFieldHelp() (label, help string, ok bool) {
	return s.label, s.help, s.ok
}

func TestContextPane_StepsListMirrorsTrailState(t *testing.T) {
	steps := []WizardStep{newNopStep(), newNopStep(), newNopStep()}
	m := NewFlowModel(steps, config.DefaultConfig(), DefaultChrome())
	tuitest.RenderAt(t, m, 180, 40)
	m.Update(JumpToStepMsg{StepID: steps[1].ID()})

	p := m.progressInfo()
	rawLines := paneStepsLines(currentThemePtr(), p.Titles, p.Current-1, 40, 40)
	lines := make([]string, len(rawLines))
	for i, l := range rawLines {
		lines[i] = tuitest.StripANSI(l)
	}
	if len(lines) != 4 {
		t.Fatalf("paneStepsLines() = %d lines, want 4 (header + 3 steps):\n%s", len(lines), lines)
	}
	if !strings.HasPrefix(lines[1], tui.IconSuccess) {
		t.Errorf("step 0 (past) = %q, want an IconSuccess prefix", lines[1])
	}
	if !strings.HasPrefix(lines[2], tui.IconActive) {
		t.Errorf("step 1 (current) = %q, want an IconActive prefix", lines[2])
	}
	if !strings.HasPrefix(lines[3], tui.IconPending) {
		t.Errorf("step 2 (future) = %q, want an IconPending prefix", lines[3])
	}
}

func TestContextPane_AnsweredFactsRender(t *testing.T) {
	s := &answeredNopStep{nopStep: *newNopStep(), facts: []render.Fact{{Key: "host", Value: "10.0.0.1"}}}
	m := NewFlowModel([]WizardStep{s}, config.DefaultConfig(), DefaultChrome())
	tuitest.RenderAt(t, m, 180, 40)

	got := tuitest.StripANSI(m.renderContextPane(40, 40))
	if !strings.Contains(got, "host: 10.0.0.1") {
		t.Errorf("pane missing answered fact:\n%s", got)
	}
}

func TestContextPane_NoAnsweredMethodContributesNothing(t *testing.T) {
	m := NewFlowModel([]WizardStep{newNopStep()}, config.DefaultConfig(), DefaultChrome())
	tuitest.RenderAt(t, m, 180, 40)

	got := strings.ToUpper(tuitest.StripANSI(m.renderContextPane(40, 40)))
	if strings.Contains(got, "CONFIGURED") {
		t.Errorf("CONFIGURED section should be absent for a step without Answered():\n%s", got)
	}
}

func TestContextPane_FocusedFieldHelpEcho(t *testing.T) {
	s := &focusedFieldNopStep{nopStep: *newNopStep(), label: "host", help: "proxmox host ip", ok: true}
	m := NewFlowModel([]WizardStep{s}, config.DefaultConfig(), DefaultChrome())
	tuitest.RenderAt(t, m, 180, 40)

	got := tuitest.StripANSI(m.renderContextPane(40, 40))
	if !strings.Contains(got, "host") || !strings.Contains(got, "proxmox host ip") {
		t.Errorf("pane missing focused field echo:\n%s", got)
	}
}

func TestContextPane_NoFocusedFieldSectionWhenNotOK(t *testing.T) {
	s := &focusedFieldNopStep{nopStep: *newNopStep(), ok: false}
	m := NewFlowModel([]WizardStep{s}, config.DefaultConfig(), DefaultChrome())
	tuitest.RenderAt(t, m, 180, 40)

	got := strings.ToUpper(tuitest.StripANSI(m.renderContextPane(40, 40)))
	if strings.Contains(got, "FOCUSED FIELD") {
		t.Errorf("FOCUSED FIELD section should be absent when ok=false:\n%s", got)
	}
}

func TestContextPane_NeverRendersBelowSplitThreshold(t *testing.T) {
	s := &answeredNopStep{nopStep: *newNopStep(), facts: []render.Fact{{Key: "host", Value: "10.0.0.1"}}}
	m := NewFlowModel([]WizardStep{s}, config.DefaultConfig(), DefaultChrome())

	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 149, 40))
	if strings.Contains(frame, "10.0.0.1") {
		t.Errorf("pane content leaked into a sub-split frame:\n%s", frame)
	}
}

// richPaneStep carries both an Answered() fact list and a FocusedFieldHelp()
// pair for TestContextPane_DropOrderCascades; each fact and the help text
// render as exactly one line, so the height budgets in that test are exact.
type richPaneStep struct {
	nopStep
	facts []render.Fact
}

func (s *richPaneStep) Answered() []render.Fact { return s.facts }

func (s *richPaneStep) FocusedFieldHelp() (label, help string, ok bool) {
	return "host", "proxmox host", true
}

// fiveFacts is richPaneStep's fixture for TestContextPane_DropOrderCascades:
// deliberately more rows than the focused-field echo needs (7 vs 4, with the
// blank separator each carries), so a naive per-section fit check — rather
// than a true priority cascade — would let FOCUSED FIELD render even after
// CONFIGURED was dropped for space. That inversion is exactly what the test
// pins against.
var fiveFacts = []render.Fact{
	{Key: "a", Value: "1"},
	{Key: "b", Value: "2"},
	{Key: "c", Value: "3"},
	{Key: "d", Value: "4"},
	{Key: "e", Value: "5"},
}

// TestContextPane_DropOrderCascades pins the squeeze's drop order as a true
// priority cascade, not just an independent per-section fit check: with one
// step (PROGRESS = header + 1 row = 2 lines), five facts (CONFIGURED = blank +
// header + 5 rows = 7 lines), and a one-line focused-field echo (FOCUSED
// FIELD = blank + header + label + help = 4 lines), CONFIGURED needs strictly
// more room than FOCUSED FIELD — so at a height where CONFIGURED doesn't fit but
// FOCUSED FIELD's own need would, FOCUSED FIELD must still be absent: once a
// higher-priority section is dropped for space, no lower-priority section
// may render in its place. PROGRESS never drops, and the pane never exceeds
// its height budget.
func TestContextPane_DropOrderCascades(t *testing.T) {
	s := &richPaneStep{nopStep: *newNopStep(), facts: fiveFacts}
	m := NewFlowModel([]WizardStep{s}, config.DefaultConfig(), DefaultChrome())
	tuitest.RenderAt(t, m, 180, 40)

	cases := []struct {
		name        string
		height      int
		wantSoFar   bool
		wantFocused bool
	}{
		// remaining after PROGRESS(2) is 4/5/6 — all less than CONFIGURED's need
		// (7), but each would satisfy FOCUSED FIELD's need (4) on its own.
		// The cascade must drop both, not just CONFIGURED.
		{"configured doesn't fit, cascade drops focused field too (a)", 6, false, false},
		{"configured doesn't fit, cascade drops focused field too (b)", 7, false, false},
		{"configured doesn't fit, cascade drops focused field too (c)", 8, false, false},
		{"everything fits", 13, true, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.ToUpper(tuitest.StripANSI(m.renderContextPane(40, c.height)))
			if strings.Contains(got, "CONFIGURED") != c.wantSoFar {
				t.Errorf("height=%d: CONFIGURED present=%v, want %v:\n%s", c.height, strings.Contains(got, "CONFIGURED"), c.wantSoFar, got)
			}
			if strings.Contains(got, "FOCUSED FIELD") != c.wantFocused {
				t.Errorf("height=%d: FOCUSED FIELD present=%v, want %v:\n%s", c.height, strings.Contains(got, "FOCUSED FIELD"), c.wantFocused, got)
			}
			if !strings.Contains(got, "PROGRESS") {
				t.Errorf("height=%d: PROGRESS section missing:\n%s", c.height, got)
			}
			lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
			if len(lines) > c.height {
				t.Errorf("height=%d: pane rendered %d lines, want <= %d", c.height, len(lines), c.height)
			}
		})
	}
}

// stylePrefix renders "x" in fg and returns the ANSI sequence preceding it,
// so a test can assert a rendered fragment was styled with fg — goldens are
// ANSI-stripped and cannot pin these contrasts.
func stylePrefix(t *testing.T, fg color.Color) string {
	t.Helper()
	rendered := lipgloss.NewStyle().Foreground(fg).Render("x")
	i := strings.Index(rendered, "x")
	if i < 0 {
		t.Fatalf("rendered marker %q lost its %q", rendered, "x")
	}
	return rendered[:i]
}

func TestPaneSectionHeader_HouseLabelStyle(t *testing.T) {
	got := paneSectionHeader(currentThemePtr(), "progress")
	want := lipgloss.NewStyle().Foreground(tui.ColorAccent()).Bold(true).Render("PROGRESS")
	if got != want {
		t.Fatalf("paneSectionHeader = %q, want the SectionStyles.Header look %q", got, want)
	}
}

func TestPaneStepRows_StatusHierarchy(t *testing.T) {
	lines := paneStepsLines(currentThemePtr(), []string{"done step", "current step", "pending step"}, 1, 40, 40)

	if want := stylePrefix(t, tui.ColorSuccess()) + tui.IconSuccess; !strings.HasPrefix(lines[1], want) {
		t.Errorf("passed row = %q, want a %q glyph prefix", lines[1], want)
	}
	if !strings.Contains(lines[1], stylePrefix(t, tui.ColorTextFaint())+"done step") {
		t.Errorf("passed row = %q, want its label muted in Slate500", lines[1])
	}
	if want := stylePrefix(t, tui.ColorPrimary()) + tui.IconActive; !strings.HasPrefix(lines[2], want) {
		t.Errorf("current row = %q, want a %q glyph prefix", lines[2], want)
	}
	if !strings.Contains(lines[2], stylePrefix(t, tui.ColorText())+"current step") {
		t.Errorf("current row = %q, want its label bright in ColorText()", lines[2])
	}
	if want := stylePrefix(t, tui.ColorSubtle()) + tui.IconPending; !strings.HasPrefix(lines[3], want) {
		t.Errorf("pending row = %q, want a %q glyph prefix", lines[3], want)
	}
	if !strings.Contains(lines[3], stylePrefix(t, tui.ColorSubtle())+"pending step") {
		t.Errorf("pending row = %q, want its label dim in Slate600", lines[3])
	}
}

func TestPaneFactLines_KeyDimValueNormal(t *testing.T) {
	lines := paneFactLines(currentThemePtr(), 40, render.Fact{Key: "host", Value: "10.0.0.1"})
	if len(lines) != 1 {
		t.Fatalf("paneFactLines = %d lines, want 1: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], stylePrefix(t, tui.ColorTextFaint())+"host:") {
		t.Errorf("fact = %q, want its key dim in Slate500", lines[0])
	}
	if !strings.Contains(lines[0], stylePrefix(t, tui.ColorTextSoft())+" 10.0.0.1") {
		t.Errorf("fact = %q, want its value in Slate300 body text", lines[0])
	}
}

func TestPaneFactLines_WrapKeepsTheSeam(t *testing.T) {
	lines := paneFactLines(currentThemePtr(), 12, render.Fact{Key: "host", Value: "a-very-long-value-that-wraps"})
	if len(lines) < 2 {
		t.Fatalf("fixture must wrap: %v", lines)
	}
	if !strings.HasPrefix(lines[0], stylePrefix(t, tui.ColorTextFaint())+"host:") {
		t.Errorf("first row = %q, want the key dim", lines[0])
	}
	for i, l := range lines[1:] {
		if !strings.HasPrefix(l, stylePrefix(t, tui.ColorTextSoft())) {
			t.Errorf("continuation row %d = %q, want value styling only", i+1, l)
		}
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > 12 {
			t.Errorf("row %d is %d cols, want <= 12", i, w)
		}
	}
}

func TestPaneFocusedFieldLines_NameNormalHelpItalicDim(t *testing.T) {
	lines := paneFocusedFieldLines(currentThemePtr(), 40, "host", "proxmox host ip")
	if !strings.HasPrefix(lines[1], stylePrefix(t, tui.ColorTextSoft())+"host") {
		t.Errorf("field name = %q, want Slate300 body text", lines[1])
	}
	wantHelp := lipgloss.NewStyle().Foreground(tui.ColorTextFaint()).Italic(true).Render("proxmox host ip")
	if lines[2] != wantHelp {
		t.Errorf("help = %q, want dim italic %q", lines[2], wantHelp)
	}
}

// TestPaneStepsLines_TruncatesUnderAnExtremeHeightBudget exercises the
// defensive fallback the split-layout height gate is meant to make
// unreachable in practice: given far less room than the step count needs,
// paneStepsLines never exceeds maxHeight, and it never drops the header or
// the current step's own row.
func TestPaneStepsLines_TruncatesUnderAnExtremeHeightBudget(t *testing.T) {
	titles := make([]string, 20)
	for i := range titles {
		titles[i] = "step"
	}

	for _, maxHeight := range []int{1, 2, 3, 5, 10} {
		lines := paneStepsLines(currentThemePtr(), titles, 10, 40, maxHeight)
		if len(lines) > maxHeight {
			t.Errorf("maxHeight=%d: got %d lines", maxHeight, len(lines))
		}
	}

	lines := paneStepsLines(currentThemePtr(), titles, 10, 40, 5)
	plain := make([]string, len(lines))
	for i, l := range lines {
		plain[i] = tuitest.StripANSI(l)
	}
	joined := strings.Join(plain, "\n")
	if !strings.Contains(joined, "PROGRESS") {
		t.Fatalf("header missing: %v", plain)
	}
	if !strings.HasPrefix(plain[len(plain)-1], "…") && !strings.Contains(joined, "…") {
		t.Errorf("expected a dim … row when truncated: %v", plain)
	}
	foundCurrent := false
	for _, l := range plain[1:] {
		if strings.HasPrefix(l, tui.IconActive) {
			foundCurrent = true
		}
	}
	if !foundCurrent {
		t.Errorf("current step's row (IconActive) must survive truncation: %v", plain)
	}
}

func currentThemePtr() *tui.Theme {
	th := tui.CurrentTheme()
	return &th
}

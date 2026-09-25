package wizard

import (
	"errors"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

// TestModel_FrameWidthIsTerminalMinusFour pins the full-width frame at every
// tier: the bordered box always spans terminal width minus the outer padding
// — a capped form measure lives inside it, never as a shrunken frame — and
// every rendered row still spans the terminal exactly.
func TestModel_FrameWidthIsTerminalMinusFour(t *testing.T) {
	for _, w := range []int{80, 100, 116, 120, 149, 150, 151, 180, 200} {
		m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
		frame := tuitest.StripANSI(tuitest.RenderAt(t, m, w, 30))
		lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
		for i, line := range lines {
			if lw := lipgloss.Width(line); lw != w {
				t.Errorf("w=%d row %d width %d: %q", w, i, lw, line)
			}
		}
		top := "  ╭" + strings.Repeat("─", w-6) + "╮  "
		if lines[1] != top {
			t.Errorf("w=%d row 1 = %q, want the full-width border %q", w, lines[1], top)
		}
		for i := 2; i < len(lines)-2; i++ {
			runes := []rune(lines[i])
			if runes[2] != '│' || runes[w-3] != '│' {
				t.Errorf("w=%d row %d does not span the frame: %q", w, i, lines[i])
			}
		}
	}
}

func TestModel_BodyWidthCapsBelowSplitThreshold(t *testing.T) {
	cases := []struct{ w, want int }{
		{80, 74},
		{100, 94},
		{116, 110}, // cap boundary: uncapped and capped agree
		{117, 110}, // cap engages one column past the boundary
		{120, 110},
		{149, 110}, // widest terminal that still doesn't split
	}
	for _, c := range cases {
		m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
		tuitest.RenderAt(t, m, c.w, 40)
		if got := m.bodyWidth(); got != c.want {
			t.Errorf("w=%d bodyWidth() = %d, want %d", c.w, got, c.want)
		}
		if got, want := m.contentWidth(), c.w-outerHorizontalPadding-wizardBorderHorizontal; got != want {
			t.Errorf("w=%d contentWidth() = %d, want the full frame interior %d", c.w, got, want)
		}
	}
}

func TestModel_SplitLayoutEngagesAt150(t *testing.T) {
	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())

	tuitest.RenderAt(t, m, 149, 40)
	if m.splitLayout() {
		t.Fatal("149 cols should not split")
	}

	tuitest.RenderAt(t, m, 150, 40)
	if !m.splitLayout() {
		t.Fatal("150 cols should split")
	}
}

func TestSplitMinHeight_Derivation(t *testing.T) {
	cases := []struct{ stepCount, want int }{
		{1, 12},  // fixedLayoutOverhead(10) + header(1) + 1
		{7, 18},  // the lifecycle flow's step count
		{11, 22}, // the configure wizard's step count
	}
	for _, c := range cases {
		if got := splitMinHeight(c.stepCount); got != c.want {
			t.Errorf("splitMinHeight(%d) = %d, want %d", c.stepCount, got, c.want)
		}
	}
}

// TestModel_SplitLayoutGatedByHeight pins the exact boundary splitMinHeight
// derives for an 11-step wizard (floor 22, per TestSplitMinHeight_Derivation):
// one row short of it, the layout must not split no matter how wide the
// terminal is — a wide-enough-but-short terminal falls back to the capped
// single-column tier rather than a pane with no room for its own PROGRESS list.
func TestModel_SplitLayoutGatedByHeight(t *testing.T) {
	steps := make([]WizardStep, 11)
	for i := range steps {
		steps[i] = newNopStep()
	}

	m := NewModel(steps, config.DefaultConfig())
	tuitest.RenderAt(t, m, 150, 21)
	if m.splitLayout() {
		t.Fatal("150x21 (one row below the 11-step floor of 22) should not split")
	}
	if got := m.bodyWidth(); got != singleFormMaxWidth {
		t.Errorf("150x21: bodyWidth()=%d, want the single-column measure %d — split layout leaked into a gated frame", got, singleFormMaxWidth)
	}

	m2 := NewModel(steps, config.DefaultConfig())
	tuitest.RenderAt(t, m2, 150, 22)
	if !m2.splitLayout() {
		t.Fatal("150x22 (exactly the 11-step floor) should split")
	}
}

// TestModel_SplitLayoutHeightGateFrameFitsExactly confirms that when the
// height gate falls back to the single-column tier, the frame still renders
// exactly the requested rows — the same guarantee TestModel_TooSmallRendersNotice
// pins for the width floor, now covering the height floor too.
func TestModel_SplitLayoutHeightGateFrameFitsExactly(t *testing.T) {
	steps := make([]WizardStep, 11)
	for i := range steps {
		steps[i] = newNopStep()
	}
	m := NewModel(steps, config.DefaultConfig())
	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 150, 21))
	tuitest.AssertFits(t, frame, 150, 21)
	if got := strings.Count(frame, "\n") + 1; got != 21 {
		t.Errorf("150x21 frame = %d rows, want exactly 21", got)
	}
}

// TestModel_SplitLayoutFormPaneInvariant pins the form/rule/pane budget
// arithmetic: their widths always sum to exactly contentWidth, the form
// never exceeds formMaxWidth, and the pane absorbs the entire remainder —
// no ceiling, only the paneMinWidth floor — so a wider terminal widens the
// pane instead of leaving idle margin.
func TestModel_SplitLayoutFormPaneInvariant(t *testing.T) {
	for _, w := range []int{150, 151, 180, 200} {
		m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
		tuitest.RenderAt(t, m, w, 48)

		form, pane := m.formPaneWidths()
		if got, want := form+paneRuleWidth+pane, m.contentWidth(); got != want {
			t.Errorf("w=%d: form(%d)+rule(%d)+pane(%d)=%d, want contentWidth()=%d", w, form, paneRuleWidth, pane, got, want)
		}
		if form > formMaxWidth {
			t.Errorf("w=%d: form=%d exceeds formMaxWidth=%d", w, form, formMaxWidth)
		}
		if want := w - outerHorizontalPadding - wizardBorderHorizontal - formMaxWidth - paneRuleWidth; pane != want {
			t.Errorf("w=%d: pane=%d, want the full remainder %d", w, pane, want)
		}
		if pane < paneMinWidth {
			t.Errorf("w=%d: pane=%d below paneMinWidth=%d", w, pane, paneMinWidth)
		}
	}
}

// TestModel_WideRowsStillFillTerminalWidth extends
// TestModel_FrameWidthIsTerminalMinusFour past 120 cols: whether the cap
// (149) or the split layout (150, 180) is what's shrinking the frame, every
// row still renders at exactly the terminal width — the surplus becomes
// blank right-hand margin, never a partial row AltScreen would leave dirty.
func TestModel_WideRowsStillFillTerminalWidth(t *testing.T) {
	for _, w := range []int{149, 150, 151, 180} {
		m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
		frame := tuitest.StripANSI(tuitest.RenderAt(t, m, w, 48))
		for i, line := range strings.Split(strings.TrimRight(frame, "\n"), "\n") {
			if lw := lipgloss.Width(line); lw != w {
				t.Errorf("w=%d row %d width %d: %q", w, i, lw, line)
			}
		}
	}
}

func TestModel_TooSmallRendersNotice(t *testing.T) {
	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
	for _, sz := range [][2]int{{59, 24}, {80, 19}} {
		frame := tuitest.StripANSI(tuitest.RenderAt(t, m, sz[0], sz[1]))
		if !strings.Contains(frame, tooSmallNotice) || strings.Contains(frame, "╭") {
			t.Errorf("%v: %q", sz, frame)
		}
	}
}

func TestModel_ViewportHeightBudget(t *testing.T) {
	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
	for _, sz := range [][2]int{{80, 24}, {100, 30}, {120, 40}} {
		tuitest.RenderAt(t, m, sz[0], sz[1])
		if got := m.viewport.Height(); got != sz[1]-fixedLayoutOverhead {
			t.Errorf("%v viewport height %d", sz, got)
		}
	}
}

func TestModel_StatusRowIsBudgeted(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {100, 30}, {120, 40}} {
		m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
		before := strings.Count(tuitest.RenderAt(t, m, sz[0], sz[1]), "\n")
		m.Update(ErrorSetMsg{Error: errors.New("boom")})
		after := strings.Count(m.View().Content, "\n")
		if before != after {
			t.Fatalf("%v: rows %d → %d", sz, before, after)
		}
		if !strings.Contains(tuitest.StripANSI(m.View().Content), "✗ boom") {
			t.Fatalf("%v: status row missing", sz)
		}
	}
}

func TestModel_StatusRowClearsOnKeypress(t *testing.T) {
	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)
	m.Update(ErrorSetMsg{Error: errors.New("boom")})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if strings.Contains(tuitest.StripANSI(m.View().Content), "boom") {
		t.Fatal("status row did not clear")
	}
}

func TestModel_StatusRowTruncatesLongError(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {100, 30}, {120, 40}} {
		m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
		tuitest.RenderAt(t, m, sz[0], sz[1])
		m.Update(ErrorSetMsg{Error: errors.New(strings.Repeat("x", 300))})
		tuitest.AssertFits(t, m.View().Content, sz[0], sz[1])
	}
}

type titledStep struct {
	nopStep
	title string
}

func (s *titledStep) DisplayTitle() string { return s.title }

func TestModel_WindowTitleFollowsStep(t *testing.T) {
	first := &titledStep{nopStep: *newNopStep(), title: "welcome"}
	second := &titledStep{nopStep: *newNopStep(), title: "basics"}
	m := NewModel([]WizardStep{first, second}, config.DefaultConfig())

	if got, want := m.View().WindowTitle, "okdctl · welcome"; got != want {
		t.Errorf("WindowTitle = %q, want %q", got, want)
	}

	m.Update(JumpToStepMsg{StepID: second.ID()})
	if got, want := m.View().WindowTitle, "okdctl · basics"; got != want {
		t.Errorf("WindowTitle = %q, want %q", got, want)
	}
}

// lightSlate700ANSI and darkSlate700ANSI are the truecolor SGR sequences for
// ColorSlate700's light (#CBD5E1) and dark (#334155) tier values.
const (
	lightSlate700ANSI = "38;2;203;213;225"
	darkSlate700ANSI  = "38;2;51;65;85"
)

// resetPackageColorState restores the dark-tier default and rebuilds every
// package-level style cache that tracks it, mirroring the full call chain
// wizard.Model's BackgroundColorMsg case runs in production — SetDarkBackground
// alone leaves rebuildWizardStyles/components.RebuildStyles's own package
// vars on the light tier, leaking into whichever test runs next.
func resetPackageColorState() {
	tui.SetDarkBackground(true)
	rebuildWizardStyles()
	components.RebuildStyles()
}

func TestModel_BackgroundColorMsgRebuildsStyles(t *testing.T) {
	t.Cleanup(resetPackageColorState)

	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)

	m.Update(tea.BackgroundColorMsg{Color: color.White})

	frame := m.View().Content
	if !strings.Contains(frame, lightSlate700ANSI) {
		t.Errorf("light-tier ColorSlate700 (%s) not found in rendered frame:\n%s", lightSlate700ANSI, frame)
	}
	if strings.Contains(frame, darkSlate700ANSI) {
		t.Errorf("stale dark-tier ColorSlate700 (%s) still rendered:\n%s", darkSlate700ANSI, frame)
	}
}

// selectorStep is a WizardStep wrapping a components.Selector directly, so
// its View exercises Selector.getOptionStyles's cache without any of the
// other step machinery.
type selectorStep struct {
	nopStep
	sel *components.Selector
}

func newSelectorStep() *selectorStep {
	sel := components.NewSelector([]components.Option{
		{ID: "a", Title: "first"},
		{ID: "b", Title: "second"},
	})
	return &selectorStep{nopStep: *newNopStep(), sel: sel}
}

func (s *selectorStep) View(_, _ int) string { return s.sel.View() }

func TestModel_BackgroundColorMsgInvalidatesSelectorCache(t *testing.T) {
	t.Cleanup(resetPackageColorState)

	step := newSelectorStep()
	m := NewModel([]WizardStep{step}, config.DefaultConfig())

	before := step.View(0, 0)
	if !strings.Contains(before, darkSlate700ANSI) {
		t.Fatalf("setup: expected dark-tier ColorSlate700 (%s) in the selector's first render:\n%q", darkSlate700ANSI, before)
	}

	m.Update(tea.BackgroundColorMsg{Color: color.White})

	after := step.View(0, 0)
	if !strings.Contains(after, lightSlate700ANSI) {
		t.Errorf("light-tier ColorSlate700 (%s) not found after BackgroundColorMsg — Selector's cache was not invalidated:\n%q", lightSlate700ANSI, after)
	}
	if strings.Contains(after, darkSlate700ANSI) {
		t.Errorf("stale dark-tier ColorSlate700 (%s) still rendered — Selector's cache was not invalidated:\n%q", darkSlate700ANSI, after)
	}
}

func TestHeader_TitleRowContainsDisplayTitleAndStepCount(t *testing.T) {
	s := &titledStep{nopStep: *newNopStep(), title: "configure your cluster basics"}
	m := NewModel([]WizardStep{newNopStep(), newNopStep(), s}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)
	m.Update(JumpToStepMsg{StepID: s.ID()})
	frame := tuitest.StripANSI(m.View().Content)
	rows := strings.Split(frame, "\n")
	if !strings.Contains(rows[3], "configure your cluster basics") || !strings.Contains(rows[3], "step 3 of 3") {
		t.Fatalf("row 3 = %q", rows[3])
	}
	if strings.Contains(strings.Join(rows[5:], "\n"), "configure your cluster basics") {
		t.Fatal("title still in body")
	}
}

func TestHeader_TaglineOnlyOnFirstStep(t *testing.T) {
	second := newNopStep()
	chrome := FlowChrome{Tagline: "okd over proxmox, the easy way"}
	m := NewFlowModel([]WizardStep{newNopStep(), second}, config.DefaultConfig(), chrome)
	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 100, 30))
	if !strings.Contains(frame, chrome.Tagline) {
		t.Fatalf("tagline missing on step 1:\n%s", frame)
	}

	m.Update(JumpToStepMsg{StepID: second.ID()})
	frame = tuitest.StripANSI(m.View().Content)
	if strings.Contains(frame, chrome.Tagline) {
		t.Fatalf("tagline present on step 2:\n%s", frame)
	}
}

func TestHeader_TrailHookReplacesDots(t *testing.T) {
	chrome := FlowChrome{Trail: func(_ ProgressInfo) string { return "op › target" }}
	m := NewFlowModel([]WizardStep{newNopStep()}, config.DefaultConfig(), chrome)
	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 100, 30))
	if !strings.Contains(frame, "op › target") || strings.Contains(frame, "step 1 of 1") {
		t.Fatal(frame)
	}
}

func TestHeader_TitleTruncatesAt60Cols(t *testing.T) {
	long := strings.Repeat("configure your cluster basics ", 4)
	s := &titledStep{nopStep: *newNopStep(), title: long}
	m := NewModel([]WizardStep{s}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 60, 20)

	row2 := tuitest.StripANSI(strings.Split(m.renderHeader(), "\n")[1])
	if w := lipgloss.Width(row2); w > 54 {
		t.Fatalf("header row 2 width %d > 54: %q", w, row2)
	}
	if strings.Contains(row2, long) {
		t.Fatalf("row 2 shows the untruncated title: %q", row2)
	}

	ellipsis := strings.Index(row2, "…")
	ribbon := strings.Index(row2, "step ")
	if ellipsis < 0 {
		t.Fatalf("row 2 missing ellipsis: %q", row2)
	}
	if ribbon < 0 || ellipsis >= ribbon {
		t.Fatalf("ellipsis must precede the ribbon: %q", row2)
	}
}

// tallStep renders far more lines than any test terminal height, forcing
// the viewport (and its footer scroll indicator) into the scrollable state.
type tallStep struct{ nopStep }

func (s *tallStep) View(_, _ int) string {
	return strings.TrimRight(strings.Repeat("row\n", 60), "\n")
}

func newTallStep() *tallStep {
	return &tallStep{nopStep: *newNopStep()}
}

func TestFooterRule_IndicatorCentredWithinAvail(t *testing.T) {
	cfg := config.DefaultConfig()
	chrome := FlowChrome{Badge: func(*config.Config) string { return "cluster-badge" }}
	m := NewFlowModel([]WizardStep{newTallStep()}, cfg, chrome)
	tuitest.RenderAt(t, m, 100, 30)

	arrows, message, scrollable := m.scrollIndicator()
	if !scrollable {
		t.Fatal("expected a scrollable viewport")
	}
	indPlain := tuitest.StripANSI(arrows + "  " + message)

	rule := tuitest.StripANSI(m.renderFooterRule())
	idx := strings.Index(rule, indPlain)
	if idx < 0 {
		t.Fatalf("indicator %q not found in rule %q", indPlain, rule)
	}

	left := strings.Count(rule[:idx], "─")
	right := strings.Count(rule[idx+len(indPlain):], "─")
	if d := left - right; d < -1 || d > 1 {
		t.Fatalf("left=%d right=%d not centred: %q", left, right, rule)
	}
}

// TestFooterRule_ClampsAtNarrowWidths pins bug 7: at 60–63 columns the
// rule + indicator + badge row must never exceed the content width — an
// overflow wraps inside WizardBorderStyle and shatters the frame.
func TestFooterRule_ClampsAtNarrowWidths(t *testing.T) {
	cfg := config.DefaultConfig()
	chrome := FlowChrome{Badge: func(*config.Config) string { return "okd 4.22.0-okd-scos.7" }}
	for _, w := range []int{60, 61, 62, 63} {
		m := NewFlowModel([]WizardStep{newTallStep()}, cfg, chrome)
		frame := tuitest.RenderAt(t, m, w, 20)

		rule := tuitest.StripANSI(m.renderFooterRule())
		if got, want := lipgloss.Width(rule), m.contentWidth(); got > want {
			t.Fatalf("width %d: footer rule = %d cols, want <= %d: %q", w, got, want, rule)
		}
		tuitest.AssertFits(t, frame, w, 20)
	}
}

func TestFooter_AdvertisesPageKeysWhenScrollable(t *testing.T) {
	cfg := config.DefaultConfig()

	tall := NewModel([]WizardStep{newTallStep()}, cfg)
	tallFrame := tuitest.StripANSI(tuitest.RenderAt(t, tall, 100, 30))
	if !strings.Contains(tallFrame, "pgup/pgdn") {
		t.Fatalf("expected pgup/pgdn hint:\n%s", tallFrame)
	}

	short := NewModel([]WizardStep{newNopStep()}, cfg)
	shortFrame := tuitest.StripANSI(tuitest.RenderAt(t, short, 100, 30))
	if strings.Contains(shortFrame, "pgup/pgdn") {
		t.Fatalf("unexpected pgup/pgdn hint:\n%s", shortFrame)
	}
}

const pinnedFooterText = "custom footer text"

type pinnedFooterStep struct{ nopStep }

func (s *pinnedFooterStep) PinnedFooter(_ int) string { return pinnedFooterText }

func newPinnedFooterStep() *pinnedFooterStep {
	return &pinnedFooterStep{nopStep: *newNopStep()}
}

func TestFooter_PinnedFooterLeftOfRibbon(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewModel([]WizardStep{newPinnedFooterStep()}, cfg)
	tuitest.RenderAt(t, m, 100, 30)

	helpRow := tuitest.StripANSI(m.renderHelpRow())
	if strings.Contains(helpRow, "\n") {
		t.Fatalf("help row wrapped to a second line: %q", helpRow)
	}

	leftIdx := strings.Index(helpRow, pinnedFooterText)
	if leftIdx != 2 {
		t.Fatalf("pinned footer text at col %d, want 2: %q", leftIdx, helpRow)
	}

	// The ribbon's trailing "? help" hint (always reserved, per
	// helpEssentialKeys) is now the rightmost item, not "quit".
	helpIdx := strings.LastIndex(helpRow, "help")
	if helpIdx < leftIdx+len(pinnedFooterText) {
		t.Fatalf("ribbon not right of pinned text: %q", helpRow)
	}
	if trailing := len(helpRow) - (helpIdx + len("help")); trailing > 2 {
		t.Fatalf("ribbon not right-aligned, trailing=%d: %q", trailing, helpRow)
	}
}

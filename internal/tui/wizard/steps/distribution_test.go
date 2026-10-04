package steps

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/releases"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func TestDistributionStep_Apply(t *testing.T) {
	s := NewDistributionStep()
	s.SetSelectedVersion("4.18.0-okd-scos.10")

	cfg := &config.Config{}
	if err := s.Apply(cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if cfg.Distribution.Type != config.DistributionOKD {
		t.Errorf("Distribution.Type = %q, want %q", cfg.Distribution.Type, config.DistributionOKD)
	}
	if cfg.Distribution.Version != "4.18.0-okd-scos.10" {
		t.Errorf("Distribution.Version = %q, want 4.18.0-okd-scos.10", cfg.Distribution.Version)
	}
}

func TestDistributionStep_Answered(t *testing.T) {
	s := NewDistributionStep()
	if facts := s.Answered(); facts != nil {
		t.Fatalf("Answered() before selection = %+v, want nil", facts)
	}

	s.SetSelectedVersion("4.18.0-okd-scos.10")
	facts := s.Answered()
	if len(facts) != 1 || facts[0].Key != "version" || facts[0].Value != "4.18.0-okd-scos.10" {
		t.Fatalf("Answered() = %+v", facts)
	}
}

func TestDistributionStep_GetMinorFromOptionID(t *testing.T) {
	s := NewDistributionStep()
	cases := map[string]int{
		"minor:4.18": 18,
		"4.17":       17,
		"garbage":    -1,
		"":           -1,
	}
	for id, want := range cases {
		if got := s.getMinorFromOptionID(id); got != want {
			t.Errorf("getMinorFromOptionID(%q) = %d, want %d", id, got, want)
		}
	}
}

func TestDistributionStep_SetVersionFetcher_UsesFixture(t *testing.T) {
	s := NewDistributionStep()
	s.SetVersionFetcher(StaticVersionFetcher{Series: DemoReleaseSeries()})
	// fetchVersions runs synchronously for the fixture — no network round
	// trip — so calling it directly, bypassing Init's spinner-tick batch,
	// is sufficient to exercise the seam.
	msg := s.fetchVersions()
	loaded, ok := msg.(versionsLoadedMsg)
	if !ok || loaded.err != nil || len(loaded.series) != len(DemoReleaseSeries()) {
		t.Fatalf("fetchVersions = %#v", msg)
	}
}

func TestDistributionStep_SetVersionFetcher_UsesFixtureError(t *testing.T) {
	s := NewDistributionStep()
	wantErr := errors.New("demo: releases unavailable")
	s.SetVersionFetcher(StaticVersionFetcher{Err: wantErr})
	msg := s.fetchVersions()
	loaded, ok := msg.(versionsLoadedMsg)
	if !ok || !errors.Is(loaded.err, wantErr) || loaded.series != nil {
		t.Fatalf("fetchVersions = %#v", msg)
	}
}

// TestDistributionStep_ConfiguredPatchAnchorsCursor pins bug 3's defect
// half: a config carrying a patch version (not a series ID) must, once the
// catalog loads, expand its series and land the cursor on that exact patch
// row with a "current" chip — not silently sit on the newest series.
func TestDistributionStep_ConfiguredPatchAnchorsCursor(t *testing.T) {
	s := NewDistributionStep()
	s.SetSelectedVersion("4.19.4-okd-scos.3")

	step, _ := s.Update(versionsLoadedMsg{series: DemoReleaseSeries()})
	s = step.(*DistributionStep)

	if got := s.versionSelector.Selected().ID; got != "4.19.4-okd-scos.3" {
		t.Fatalf("Selected().ID = %q, want the configured patch", got)
	}
	if s.expandedMinor != 19 {
		t.Fatalf("expandedMinor = %d, want 19 (the configured series)", s.expandedMinor)
	}
	if view := s.View(80, 30); !strings.Contains(view, "current") {
		t.Fatalf("View() carries no current chip:\n%s", view)
	}
}

// TestDistributionStep_EnterEnterKeepsConfiguredVersion pins the routine
// that used to bump 4.19.x to the newest series: with a configured patch
// loaded, pressing enter must complete with that patch untouched.
func TestDistributionStep_EnterEnterKeepsConfiguredVersion(t *testing.T) {
	s := NewDistributionStep()
	s.SetSelectedVersion("4.19.4-okd-scos.3")
	step, _ := s.Update(versionsLoadedMsg{series: DemoReleaseSeries()})
	s = step.(*DistributionStep)

	step, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	s = step.(*DistributionStep)
	step, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	s = step.(*DistributionStep)

	cfg := &config.Config{}
	if err := s.Apply(cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if cfg.Distribution.Version != "4.19.4-okd-scos.3" {
		t.Fatalf("Distribution.Version = %q, want the configured 4.19.4-okd-scos.3", cfg.Distribution.Version)
	}
}

// TestDistributionStep_EnterOnCollapsedSeriesConfirms pins bug 3's UX half:
// the footer promises "enter confirm", so enter on a collapsed series row
// completes the step with that series' latest instead of expanding it.
func TestDistributionStep_EnterOnCollapsedSeriesConfirms(t *testing.T) {
	s := NewDistributionStep()
	step, _ := s.Update(versionsLoadedMsg{series: DemoReleaseSeries()})
	s = step.(*DistributionStep)

	step, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	s = step.(*DistributionStep)

	if cmd == nil {
		t.Fatal("enter on a collapsed series returned no command, want StepCompleteMsg")
	}
	if _, ok := cmd().(wizard.StepCompleteMsg); !ok {
		t.Fatalf("enter on a collapsed series = %T, want StepCompleteMsg", cmd())
	}
	if got := s.GetSelectedVersion(); got != "4.20.1-okd-scos.7" {
		t.Fatalf("GetSelectedVersion() = %q, want the series latest", got)
	}
}

func TestDistributionStep_FreshRunCarriesNoCurrentChip(t *testing.T) {
	s := NewDistributionStep()
	step, _ := s.Update(versionsLoadedMsg{series: DemoReleaseSeries()})
	s = step.(*DistributionStep)

	if view := s.View(80, 30); strings.Contains(view, "current") {
		t.Fatalf("fresh-run View() carries a current chip:\n%s", view)
	}
}

// loadedDistributionStep returns a step showing the demo catalog with the
// newest minor expanded into its patch dropdown.
func loadedDistributionStep(t *testing.T) *DistributionStep {
	t.Helper()
	s := NewDistributionStep()
	step, _ := s.Update(versionsLoadedMsg{series: DemoReleaseSeries()})
	s = step.(*DistributionStep)
	step, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	return step.(*DistributionStep)
}

func TestDistributionStep_FocusedSpanTracksExpandedPatch(t *testing.T) {
	s := loadedDistributionStep(t)
	step, _ := s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	s = step.(*DistributionStep)

	lines := strings.Split(s.View(70, 24), "\n")
	span, ok := s.FocusedSpan()
	if !ok {
		t.Fatal("FocusedSpan() reported no span inside the patch dropdown")
	}
	if span.Start < 0 || span.End >= len(lines) {
		t.Fatalf("span %+v is outside the %d rendered rows", span, len(lines))
	}
	block := strings.Join(lines[span.Start:span.End+1], "\n")
	if !strings.Contains(block, "4.20.1") {
		t.Fatalf("span %+v = %q, want the selected patch", span, block)
	}
}

func TestDistributionStep_NavigationInDropdownEmitsFocusChanged(t *testing.T) {
	s := loadedDistributionStep(t)
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if !containsFocusChanged(cmd) {
		t.Fatal("moving into the patch dropdown did not emit FocusChangedMsg")
	}
}

// containsFocusChanged runs cmd, flattening batches, and reports whether any
// resulting message is a wizard.FocusChangedMsg.
func containsFocusChanged(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case wizard.FocusChangedMsg:
		return true
	case tea.BatchMsg:
		for _, c := range msg {
			if containsFocusChanged(c) {
				return true
			}
		}
	}
	return false
}

// firstErrorSetMsg runs cmd, flattening batches, and returns the first
// resulting wizard.ErrorSetMsg it finds.
func firstErrorSetMsg(cmd tea.Cmd) (wizard.ErrorSetMsg, bool) {
	if cmd == nil {
		return wizard.ErrorSetMsg{}, false
	}
	switch msg := cmd().(type) {
	case wizard.ErrorSetMsg:
		return msg, true
	case tea.BatchMsg:
		for _, c := range msg {
			if errMsg, ok := firstErrorSetMsg(c); ok {
				return errMsg, true
			}
		}
	}
	return wizard.ErrorSetMsg{}, false
}

func TestDistributionStep_ErrorStateRendersEmptyState(t *testing.T) {
	s := NewDistributionStep()
	s.SetVersionFetcher(StaticVersionFetcher{Err: errors.New("dial tcp: connection refused")})
	step, _ := s.Update(s.fetchVersions())
	s = step.(*DistributionStep)

	view := s.View(80, 24)
	if !strings.Contains(view, "no releases loaded") {
		t.Fatalf("View() = %q, want to contain %q", view, "no releases loaded")
	}
	if strings.Contains(view, "✗") {
		t.Fatalf("View() = %q, want no literal ✗ (use tui.EmptyState's pending glyph)", view)
	}
}

// TestDistributionStep_ErrorStateDetailsLabeledOnce pins the R4 fix for the
// error phase: the raw fetch error renders under a "details:" label, and the
// retry/back keys appear only in the footer's ShortHelp ribbon, not a second
// time inline in the step's own EmptyState hint.
func TestDistributionStep_ErrorStateDetailsLabeledOnce(t *testing.T) {
	s := NewDistributionStep()
	s.SetVersionFetcher(StaticVersionFetcher{Err: errors.New("dial tcp: connection refused")})
	step, _ := s.Update(s.fetchVersions())
	s = step.(*DistributionStep)

	view := s.View(80, 24)
	const wantDetails = "details: dial tcp: connection refused"
	if !strings.Contains(view, wantDetails) {
		t.Fatalf("View() = %q, want to contain %q", view, wantDetails)
	}
	if strings.Contains(view, "r retry") {
		t.Fatalf("View() = %q, want no inline \"r retry\" (the footer's ShortHelp already shows it)", view)
	}
}

func TestDistributionStep_RetryRefetches(t *testing.T) {
	s := NewDistributionStep()
	s.SetVersionFetcher(StaticVersionFetcher{Err: errors.New("dial tcp: connection refused")})
	step, _ := s.Update(s.fetchVersions())
	s = step.(*DistributionStep)

	step, cmd := s.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	s = step.(*DistributionStep)

	if s.phase != phaseVersionLoading {
		t.Fatalf("phase after 'r' = %v, want phaseVersionLoading", s.phase)
	}
	if cmd == nil {
		t.Fatal("Update('r') on the error phase returned a nil cmd, want the refetch command")
	}
}

func TestDistributionStep_ShortHelpNeverEmpty(t *testing.T) {
	s := NewDistributionStep()
	if len(s.ShortHelp()) == 0 {
		t.Error("ShortHelp() while loading is empty")
	}

	s.SetVersionFetcher(StaticVersionFetcher{Err: errors.New("boom")})
	step, _ := s.Update(s.fetchVersions())
	s = step.(*DistributionStep)
	if len(s.ShortHelp()) == 0 {
		t.Error("ShortHelp() in the error phase is empty")
	}

	step, _ = s.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	s = step.(*DistributionStep)
	if len(s.ShortHelp()) == 0 {
		t.Error("ShortHelp() after retry (back to loading) is empty")
	}

	s.SetVersionFetcher(StaticVersionFetcher{Series: DemoReleaseSeries()})
	step, _ = s.Update(s.fetchVersions())
	s = step.(*DistributionStep)
	if len(s.ShortHelp()) == 0 {
		t.Error("ShortHelp() in the select phase is empty")
	}
}

func TestDistributionStep_EnterOnEmptyListReportsError(t *testing.T) {
	s := NewDistributionStep()
	s.SetVersionFetcher(StaticVersionFetcher{Series: nil})
	step, _ := s.Update(s.fetchVersions())
	s = step.(*DistributionStep)

	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	errMsg, ok := firstErrorSetMsg(cmd)
	if !ok {
		t.Fatal("Update(enter) on an empty release list did not report an ErrorSetMsg")
	}
	if errMsg.Error == nil || errMsg.Error.Error() != "pick a release first" {
		t.Fatalf("ErrorSetMsg.Error = %v, want %q", errMsg.Error, "pick a release first")
	}
}

// midListSeriesWithManyPatches builds 5 minor series (indices 0..4) where
// the middle one (index 2) carries patchCount patch versions — enough,
// once expanded, to exceed the dropdown's floor and expose any
// undercounted before/after chrome in applyDropdownBudget.
func midListSeriesWithManyPatches(patchCount int) []releases.OKDReleaseSeries {
	series := make([]releases.OKDReleaseSeries, 5)
	for i := range series {
		minor := 20 - i
		series[i] = releases.OKDReleaseSeries{
			Major: 4, Minor: minor,
			Latest: releases.OKDVersion{Version: fmt.Sprintf("4.%d.0", minor)},
		}
	}
	patches := make([]releases.OKDVersion, patchCount)
	for i := range patches {
		patches[i] = releases.OKDVersion{Version: fmt.Sprintf("4.%d.%d", series[2].Minor, i)}
	}
	series[2].Versions = patches
	series[2].Latest = patches[0]
	return series
}

func TestDistributionStep_MidListExpansionRespectsContentHeight(t *testing.T) {
	series := midListSeriesWithManyPatches(10)

	s := NewDistributionStep()
	s.SetVersionFetcher(StaticVersionFetcher{Series: series})
	step, _ := s.Update(s.fetchVersions())
	s = step.(*DistributionStep)

	// Expand the mid-list series (2 collapsed rows before it, 2 after) —
	// the case where the expanded row's own title+description, plus its
	// connector to the last collapsed row above it, must be charged
	// against the budget or the dropdown overflows contentHeight.
	s.expandedMinor = series[2].Minor
	s.updateVersionSelector()

	const contentHeight = 40
	s.SetSize(90, contentHeight)
	view := s.View(90, contentHeight)

	if h := lipgloss.Height(view); h > contentHeight {
		t.Errorf("rendered height %d exceeds contentHeight %d — the dropdown budget overflowed the viewport", h, contentHeight)
	}

	shown := 0
	for i := 0; i < len(series[2].Versions); i++ {
		if strings.Contains(view, fmt.Sprintf("4.%d.%d", series[2].Minor, i)) {
			shown++
		}
	}
	const wantShown = 7 // (avail+1)/3 once the expanded row's own chrome is charged correctly
	if shown != wantShown {
		t.Errorf("dropdown shows %d of %d patches, want exactly %d", shown, len(series[2].Versions), wantShown)
	}
}

// TestDistributionStep_CatalogAbsentVersionInjectsCurrentRow extends the
// bug-1 ruling to the version selector: a configured version older than the
// fetched release window anchors a synthetic, selectable row instead of
// anchoring nothing, and enter confirms the configured version verbatim.
func TestDistributionStep_CatalogAbsentVersionInjectsCurrentRow(t *testing.T) {
	s := NewDistributionStep()
	s.SetSelectedVersion("4.19.1-okd-scos.9")

	step, _ := s.Update(versionsLoadedMsg{series: DemoReleaseSeries()})
	s = step.(*DistributionStep)

	if got := s.versionSelector.Selected().ID; got != "4.19.1-okd-scos.9" {
		t.Fatalf("Selected().ID = %q, want the cursor anchored on the injected row", got)
	}
	view := tuitest.StripANSI(s.View(90, 30))
	if !strings.Contains(view, "4.19.1-okd-scos.9") || !strings.Contains(view, "(current)") {
		t.Fatalf("the injected row must name the configured version with its chip:\n%s", view)
	}
	if !strings.Contains(view, "not in the fetched catalog") {
		t.Fatalf("the injected row must say the catalog does not offer it:\n%s", view)
	}

	step, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	s = step.(*DistributionStep)
	if cmd == nil {
		t.Fatal("enter on the injected row must complete the step")
	}
	if _, ok := cmd().(wizard.StepCompleteMsg); !ok {
		t.Fatalf("enter emitted %T, want StepCompleteMsg", cmd())
	}
	cfg := &config.Config{}
	if err := s.Apply(cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if cfg.Distribution.Version != "4.19.1-okd-scos.9" {
		t.Fatalf("Distribution.Version = %q, want the configured version byte-identical", cfg.Distribution.Version)
	}
}

// TestDistributionStep_CatalogPresentVersionInjectsNothing keeps the
// synthetic row a last resort: an in-catalog version anchors its real patch
// row only.
func TestDistributionStep_CatalogPresentVersionInjectsNothing(t *testing.T) {
	s := NewDistributionStep()
	s.SetSelectedVersion("4.19.4-okd-scos.3")
	step, _ := s.Update(versionsLoadedMsg{series: DemoReleaseSeries()})
	s = step.(*DistributionStep)

	if view := tuitest.StripANSI(s.View(90, 30)); strings.Contains(view, "not in the fetched catalog") {
		t.Fatalf("an in-catalog version must not inject a synthetic row:\n%s", view)
	}
}

// TestDistributionStep_StaleSuccessCannotOverwriteNewer pins the request-
// identity contract: fake request A is issued, a newer request B is issued
// before A replies (the back-then-re-enter case), B's reply lands first,
// and A's now-stale successful reply must not overwrite it.
func TestDistributionStep_StaleSuccessCannotOverwriteNewer(t *testing.T) {
	s := NewDistributionStep()

	s.Init()
	genA := s.generation

	s.Init() // re-entry while A is still in flight
	genB := s.generation
	if genB == genA {
		t.Fatal("re-entry must issue a new generation")
	}

	seriesA := []releases.OKDReleaseSeries{{Major: 4, Minor: 10, Latest: releases.OKDVersion{Version: "4.10.0"}}}
	seriesB := DemoReleaseSeries()

	step, _ := s.Update(versionsLoadedMsg{generation: genB, series: seriesB})
	s = step.(*DistributionStep)
	step, _ = s.Update(versionsLoadedMsg{generation: genA, series: seriesA})
	s = step.(*DistributionStep)

	if len(s.okdSeries) != len(seriesB) || s.okdSeries[0].Minor != seriesB[0].Minor {
		t.Fatalf("stale reply A overwrote newer reply B: okdSeries = %+v", s.okdSeries)
	}
	if s.phase != phaseVersionSelect {
		t.Fatalf("phase = %v, want phaseVersionSelect (B's success)", s.phase)
	}
}

// TestDistributionStep_StaleErrorCannotOverwriteNewer repeats the above with
// A returning an error instead of a success: the stale failure must not
// replace B's good, newer result.
func TestDistributionStep_StaleErrorCannotOverwriteNewer(t *testing.T) {
	s := NewDistributionStep()

	s.Init()
	genA := s.generation

	s.Init() // re-entry while A is still in flight
	genB := s.generation

	seriesB := DemoReleaseSeries()
	step, _ := s.Update(versionsLoadedMsg{generation: genB, series: seriesB})
	s = step.(*DistributionStep)
	step, _ = s.Update(versionsLoadedMsg{generation: genA, err: errors.New("dial tcp: connection refused")})
	s = step.(*DistributionStep)

	if s.phase != phaseVersionSelect {
		t.Fatalf("stale error flipped phase to %v, want phaseVersionSelect (B's success) to survive", s.phase)
	}
	if s.loadError != nil {
		t.Fatalf("stale error reply set loadError = %v, want nil", s.loadError)
	}
	if len(s.okdSeries) != len(seriesB) {
		t.Fatalf("stale error reply altered okdSeries: %+v", s.okdSeries)
	}
}

// TestDistributionStep_ReentryAfterSuccessReusesCachedCatalog pins the
// reuse-on-re-entry decision: once the catalog has loaded, going back and
// re-entering the step must not re-issue the release fetch.
func TestDistributionStep_ReentryAfterSuccessReusesCachedCatalog(t *testing.T) {
	s := NewDistributionStep()
	s.SetVersionFetcher(StaticVersionFetcher{Series: DemoReleaseSeries()})

	cmd := s.Init()
	if cmd == nil {
		t.Fatal("first Init() must fetch")
	}
	step, _ := s.Update(cmd())
	s = step.(*DistributionStep)
	if s.phase != phaseVersionSelect {
		t.Fatalf("phase after first load = %v, want phaseVersionSelect", s.phase)
	}

	if cmd := s.Init(); cmd != nil {
		t.Fatal("re-entry after a successful load re-issued the fetch, want cached reuse")
	}
	if s.phase != phaseVersionSelect {
		t.Fatalf("re-entry after success flipped phase to %v, want it to stay phaseVersionSelect", s.phase)
	}
}

// TestDistributionStep_ReentryAfterErrorRefetches pins the other half of the
// reuse decision: a failed attempt is never cached, so re-entering after an
// error automatically retries instead of leaving the step stuck.
func TestDistributionStep_ReentryAfterErrorRefetches(t *testing.T) {
	s := NewDistributionStep()
	s.SetVersionFetcher(StaticVersionFetcher{Err: errors.New("dial tcp: connection refused")})

	cmd := s.Init()
	step, _ := s.Update(cmd())
	s = step.(*DistributionStep)
	if s.phase != phaseVersionError {
		t.Fatalf("phase after failed load = %v, want phaseVersionError", s.phase)
	}

	s.SetVersionFetcher(StaticVersionFetcher{Series: DemoReleaseSeries()})
	cmd = s.Init() // re-entry
	if cmd == nil {
		t.Fatal("re-entry after an error must retry, not get stuck")
	}
	step, _ = s.Update(cmd())
	s = step.(*DistributionStep)
	if s.phase != phaseVersionSelect {
		t.Fatalf("phase after re-entry's retry succeeded = %v, want phaseVersionSelect", s.phase)
	}
}

package steps

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/releases"
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

func TestDistributionStep_CatalogPresentVersionInjectsNothing(t *testing.T) {
	s := NewDistributionStep()
	s.SetSelectedVersion("4.19.4-okd-scos.3")
	step, _ := s.Update(versionsLoadedMsg{series: DemoReleaseSeries()})
	s = step.(*DistributionStep)

	if view := tuitest.StripANSI(s.View(90, 30)); strings.Contains(view, "not in the fetched catalog") {
		t.Fatalf("an in-catalog version must not inject a synthetic row:\n%s", view)
	}
}

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

func TestDistributionStep_OverlayHelp(t *testing.T) {
	s := NewDistributionStep()
	if got := s.OverlayHelp(); got != nil {
		t.Fatalf("OverlayHelp() while loading = %+v, want nil", got)
	}

	s.SetVersionFetcher(StaticVersionFetcher{Series: DemoReleaseSeries()})
	cmd := s.Init()
	step, _ := s.Update(cmd())
	s = step.(*DistributionStep)
	want := wizard.KeyBinding{Key: "r", Help: "refresh"}
	if got := s.OverlayHelp(); len(got) != 1 || got[0] != want {
		t.Fatalf("OverlayHelp() in select phase = %+v, want [%+v]", got, want)
	}

	s = NewDistributionStep()
	s.SetVersionFetcher(StaticVersionFetcher{Err: errors.New("dial tcp: connection refused")})
	cmd = s.Init()
	step, _ = s.Update(cmd())
	s = step.(*DistributionStep)
	if got := s.OverlayHelp(); got != nil {
		t.Fatalf("OverlayHelp() in error phase = %+v, want nil (ShortHelp already advertises \"r retry\")", got)
	}
}

func TestDistributionStep_HelpOverlay_RefreshDiscoverable(t *testing.T) {
	s := NewDistributionStep()
	s.SetVersionFetcher(StaticVersionFetcher{Series: DemoReleaseSeries()})
	cmd := s.Init()
	step, _ := s.Update(cmd())
	s = step.(*DistributionStep)

	m := wizard.NewModel([]wizard.WizardStep{s}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)
	mm, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = mm.(*wizard.Model)

	frame := tuitest.StripANSI(m.View().Content)
	if !strings.Contains(overlayScreenSection(t, frame), "r refresh") {
		t.Fatalf("select-phase overlay missing %q:\n%s", "r refresh", frame)
	}
}

func overlayScreenSection(t *testing.T, frame string) string {
	t.Helper()
	start := strings.Index(frame, "screen")
	end := strings.Index(frame, "vim")
	if start < 0 || end < 0 || end < start {
		t.Fatalf("could not locate the overlay's screen section:\n%s", frame)
	}
	return frame[start:end]
}

func TestDistributionStep_HelpOverlay_ErrorPhaseListsRetryOnce(t *testing.T) {
	s := NewDistributionStep()
	s.SetVersionFetcher(StaticVersionFetcher{Err: errors.New("dial tcp: connection refused")})
	cmd := s.Init()
	step, _ := s.Update(cmd())
	s = step.(*DistributionStep)

	m := wizard.NewModel([]wizard.WizardStep{s}, config.DefaultConfig())
	tuitest.RenderAt(t, m, 100, 30)
	mm, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = mm.(*wizard.Model)

	frame := tuitest.StripANSI(m.View().Content)
	screen := overlayScreenSection(t, frame)
	if n := strings.Count(screen, "r retry"); n != 1 {
		t.Fatalf("error-phase overlay shows %q %d times, want exactly 1:\n%s", "r retry", n, frame)
	}
	if strings.Contains(screen, "refresh") {
		t.Fatalf("error-phase overlay must not also advertise \"refresh\":\n%s", frame)
	}
}

type ctxRecordingFetcher struct{ ctx context.Context }

func (f *ctxRecordingFetcher) FetchVersions(ctx context.Context) ([]releases.OKDReleaseSeries, error) {
	f.ctx = ctx
	return nil, ctx.Err()
}

func TestDistributionStep_FetchRunsUnderTheVisitContext(t *testing.T) {
	s := NewDistributionStep()
	fetcher := &ctxRecordingFetcher{}
	s.SetVersionFetcher(fetcher)
	ctx, cancel := context.WithCancel(context.Background())
	s.SetVisitContext(ctx)

	cmd := s.startFetch()
	cancel()
	msg, ok := cmd().(versionsLoadedMsg)
	if !ok || !errors.Is(msg.err, context.Canceled) {
		t.Fatalf("fetch after the visit was cancelled = %#v, want a cancelled fetch", msg)
	}
	if fetcher.ctx == nil || fetcher.ctx.Err() == nil {
		t.Fatal("the fetcher ran under a context the visit does not own")
	}
}

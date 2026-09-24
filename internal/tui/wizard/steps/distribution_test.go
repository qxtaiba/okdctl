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

package steps

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
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

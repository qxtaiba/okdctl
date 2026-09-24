package steps

import (
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
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

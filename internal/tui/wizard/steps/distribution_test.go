package steps

import (
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

package steps

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// navDown advances the selector via the same key the UI uses; mode is derived
// from index, not stored.
func navDown(t *testing.T, s *WelcomeStep) {
	t.Helper()
	s.nav.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
}

func TestWelcomeStep_ApplyFreshResetsConfig(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true)
	navDown(t, s) // deploy -> edit
	navDown(t, s) // edit -> fresh
	if s.GetMode() != WelcomeModeFresh {
		t.Fatalf("GetMode() = %v, want WelcomeModeFresh", s.GetMode())
	}

	cfg := &config.Config{}
	cfg.Cluster.Name = "custom-cluster"

	if err := s.Apply(cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if cfg.Cluster.Name != "mycluster" {
		t.Errorf("Cluster.Name = %q after fresh reset, want the DefaultConfig() value mycluster", cfg.Cluster.Name)
	}
}

func TestWelcomeStep_ApplyDeployLeavesConfigUntouched(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true) // defaults mode to WelcomeModeDeploy

	cfg := &config.Config{}
	cfg.Cluster.Name = "untouched"

	if err := s.Apply(cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if cfg.Cluster.Name != "untouched" {
		t.Errorf("Cluster.Name = %q, want untouched (deploy mode must not mutate cfg)", cfg.Cluster.Name)
	}
}

func TestWelcomeStep_GetSelectedAction(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true)
	if got := s.GetSelectedAction(); got != wizard.ActionDeploy {
		t.Errorf("GetSelectedAction() = %v, want ActionDeploy", got)
	}
	if !s.ShouldExitEarly() {
		t.Error("ShouldExitEarly() = false for deploy mode, want true")
	}

	navDown(t, s) // deploy -> edit
	if got := s.GetSelectedAction(); got != wizard.ActionExit {
		t.Errorf("GetSelectedAction() = %v, want ActionExit", got)
	}
	if s.ShouldExitEarly() {
		t.Error("ShouldExitEarly() = true for edit mode, want false")
	}
}

func TestWelcomeStep_DefaultModeIsFresh(t *testing.T) {
	s := NewWelcomeStep()
	if s.GetMode() != WelcomeModeFresh {
		t.Errorf("GetMode() on fresh install = %v, want WelcomeModeFresh", s.GetMode())
	}
}

package steps

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func TestReviewStep_ValidateAndApplyAreNoOps(t *testing.T) {
	s := NewReviewStep()
	if err := s.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}

	cfg := &config.Config{}
	cfg.Cluster.Name = "unchanged"
	if err := s.Apply(cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if cfg.Cluster.Name != "unchanged" {
		t.Errorf("Apply mutated cfg: Cluster.Name = %q", cfg.Cluster.Name)
	}
}

func TestReviewStep_GetSelectedAction_DefaultsToDeploy(t *testing.T) {
	s := NewReviewStep()
	if got := s.GetSelectedAction(); got != wizard.ActionDeploy {
		t.Errorf("GetSelectedAction() default = %v, want ActionDeploy", got)
	}
}

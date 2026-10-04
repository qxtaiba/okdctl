package cli

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

func TestWizardAssemblySeedsConfiguredOrder(t *testing.T) {
	t.Setenv(wizardDemoEnv, "")
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "seeded-cluster"
	spec := wizard.DefaultConfig()
	spec.InitialConfig = cfg
	spec.ConfigExists = true
	built, welcome, err := buildWizardStepsWithState(spec)
	if err != nil {
		t.Fatal(err)
	}
	if welcome == nil || len(built.Steps) != len(spec.Steps) {
		t.Fatal("incomplete assembly")
	}
	for i, step := range built.Steps {
		if string(step.ID()) != string(spec.Steps[i].Type) {
			t.Fatalf("step %d: %s", i, step.ID())
		}
	}
	state, ok := built.States[wizard.StepTypeResources].(*steps.ResourcesStepState)
	if !ok || state.Cfg != cfg {
		t.Fatal("resource preview lost seeded config")
	}
}

func TestWizardAssemblyRejectsUnknownAndDuplicateSteps(t *testing.T) {
	for _, kinds := range [][]wizard.StepConfig{{{Type: "unknown"}}, {{Type: wizard.StepTypeBasics}, {Type: wizard.StepTypeBasics}}} {
		if _, _, err := buildWizardStepsWithState(wizard.Config{Steps: kinds}); err == nil {
			t.Fatalf("accepted invalid assembly: %v", kinds)
		}
	}
}

package cli

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

func TestReviewDiffBaselineRequiresAnExistingConfig(t *testing.T) {
	for _, tc := range []struct {
		name         string
		configExists bool
		wantDiff     bool
	}{
		{name: "fresh defaults"},
		{name: "saved config", configExists: true, wantDiff: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			wizardCfg := wizard.DefaultConfig()
			wizardCfg.InitialConfig = cfg
			wizardCfg.ConfigExists = tc.configExists
			built, err := buildWizardStepsWithState(wizardCfg)
			if err != nil {
				t.Fatal(err)
			}
			var review *steps.ReviewStep
			for _, step := range built.Steps {
				if candidate, ok := step.(*steps.ReviewStep); ok {
					review = candidate
					break
				}
			}
			if review == nil {
				t.Fatal("buildWizardStepsWithState returned no review step")
			}
			cfg.Cluster.Domain = "changed.example"
			got := strings.Contains(review.View(100, 100), "CONFIG CHANGES")
			if got != tc.wantDiff {
				t.Errorf("review diff = %v, want %v", got, tc.wantDiff)
			}
		})
	}
}

func TestSaveSlotStateIsConfiguredWithoutInfrastructure(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg := config.DefaultConfig()
	if got := saveSlotState(cfg); got != steps.SaveSlotConfigured {
		t.Errorf("saveSlotState() = %q in an empty workspace, want %q — the hub must never overstate a cluster",
			got, steps.SaveSlotConfigured)
	}
}

func TestWizardAssemblySeedsConfiguredOrder(t *testing.T) {
	t.Setenv(wizardDemoEnv, "")
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "seeded-cluster"
	spec := wizard.DefaultConfig()
	spec.InitialConfig = cfg
	spec.ConfigExists = true
	built, err := buildWizardStepsWithState(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Steps) == 0 {
		t.Fatal("incomplete assembly")
	}
	if _, ok := built.Steps[0].(*steps.WelcomeStep); !ok {
		t.Fatal("incomplete assembly: first step is not the welcome/hub step")
	}
	if len(built.Steps) != len(spec.Steps) {
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

func TestWizardAssemblyResourcesAndReviewTotalsAgree(t *testing.T) {
	totals := regexp.MustCompile(`(\d+) vcpu\D+(\d+) gb ram\D+(\d+) gb disk`)
	for _, tc := range []struct{ name, demo string }{{"interactive", ""}, {"demo mode", "1"}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(wizardDemoEnv, tc.demo)
			cfg := config.DefaultConfig()
			cfg.Topology.Bootstrap = config.NodeConfig{}
			spec := wizard.DefaultConfig()
			spec.InitialConfig = cfg
			spec.ConfigExists = true
			built, err := buildWizardStepsWithState(spec)
			if err != nil {
				t.Fatal(err)
			}

			var footer, review string
			for _, step := range built.Steps {
				switch s := step.(type) {
				case *wizard.DataDrivenStep:
					if s.ID() == wizard.StepIDResources {
						footer = tuitest.StripANSI(s.PinnedFooter(120))
					}
				case *steps.ReviewStep:
					review = tuitest.StripANSI(s.View(120, 100))
				}
			}

			got, want := totals.FindStringSubmatch(footer), totals.FindStringSubmatch(review)
			if got == nil || want == nil {
				t.Fatalf("totals missing: resources footer %q, review:\n%s", footer, review)
			}
			if !slices.Equal(got[1:], want[1:]) {
				t.Errorf("resources footer totals %v, review totals %v (vcpu, gb ram, gb disk)", got[1:], want[1:])
			}
		})
	}
}

func TestWizardAssemblyRejectsUnknownAndDuplicateSteps(t *testing.T) {
	for _, kinds := range [][]wizard.StepConfig{{{Type: "unknown"}}, {{Type: wizard.StepTypeBasics}, {Type: wizard.StepTypeBasics}}} {
		if _, err := buildWizardStepsWithState(wizard.Config{Steps: kinds}); err == nil {
			t.Fatalf("accepted invalid assembly: %v", kinds)
		}
	}
}

func TestSessionVerb(t *testing.T) {
	deployHighlighted := func() *steps.WelcomeStep {
		hub := steps.NewWelcomeStep()
		hub.SetExistingConfig(config.DefaultConfig(), steps.SaveSlotConfigured)
		if hub.SelectedVerb() != steps.HubVerbDeploy {
			t.Fatalf("hub highlights %v; want the deploy row", hub.SelectedVerb())
		}
		return hub
	}

	cases := []struct {
		name string
		hub  *steps.WelcomeStep
		exit wizard.StepID
		want steps.HubVerb
	}{
		{"hub ended the session on its highlighted verb", deployHighlighted(), wizard.StepIDWelcome, steps.HubVerbDeploy},
		{"review ended the session under a stale deploy highlight", deployHighlighted(), wizard.StepIDReview, steps.HubVerbGetStarted},
		{"no hub in the flow", nil, wizard.StepIDReview, steps.HubVerbGetStarted},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sessionVerb(wizard.Result{Outcome: wizard.OutcomeCompleted, ExitStep: c.exit}, c.hub)
			if got != c.want {
				t.Errorf("sessionVerb = %v; want %v", got, c.want)
			}
		})
	}
}

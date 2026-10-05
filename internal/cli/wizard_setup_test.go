package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/lifecycle"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

func TestWizardDraftSaveFnTakesTheProjectLock(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	configPath := filepath.Join(root, "okdctl.yaml")

	lock, err := runlock.Acquire(root, "deploy")
	if err != nil {
		t.Fatalf("acquire project lock: %v", err)
	}
	defer lock.Release()

	save := wizardDraftSaveFn(configPath)
	if err := save(config.DefaultConfig(), wizard.StepIDBasics, "", nil); err == nil {
		t.Fatal("draft save succeeded while another session held the project lock; it must serialize like saveConfig and persistWizardConfig")
	}
}

func TestDemoClusterStatusCarriesNoCredentials(t *testing.T) {
	st := demoClusterStatus()

	var rendered strings.Builder
	rendered.WriteString(string(st.Phase))
	for _, n := range st.Nodes {
		rendered.WriteString(" " + n.Name + " " + string(n.Role))
	}
	for _, a := range st.Addons {
		rendered.WriteString(" " + a.Name + " " + a.Error)
	}

	for _, forbidden := range []string{"password", "token", "secret", "@pam", "https://", "root@"} {
		if strings.Contains(strings.ToLower(rendered.String()), forbidden) {
			t.Errorf("demo status fixture leaks %q; it renders into screenshots: %s", forbidden, rendered.String())
		}
	}
	if len(st.Nodes) != 6 {
		t.Errorf("demo status fixture has %d nodes, want the lifecycle fixture's 6", len(st.Nodes))
	}
	if !strings.HasPrefix(st.Nodes[0].Name, lifecycle.DemoClusterName) {
		t.Errorf("node %q must be named after lifecycle.DemoClusterName so both demo screens agree", st.Nodes[0].Name)
	}
	if st.Nodes[0].Role != nodetypes.RoleMaster {
		t.Errorf("first node role = %q, want master", st.Nodes[0].Role)
	}
}

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

func TestReviewDiffKeepsTheSavedBaselineWhenResumingDraft(t *testing.T) {
	saved := config.DefaultConfig()
	saved.Cluster.Domain = "original.example"
	draft := config.DefaultConfig()
	draft.Cluster.Domain = "draft.example"
	wizardCfg := wizard.DefaultConfig()
	wizardCfg.InitialConfig = draft
	wizardCfg.ReviewBaseline = saved
	wizardCfg.ConfigExists = true
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
	out := strings.ReplaceAll(review.View(100, 100), "\r", "")
	if !strings.Contains(out, "original.example → draft.example") {
		t.Fatalf("review omitted the saved-to-draft change:\n%s", out)
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

package okd

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// TestDeploySteps_MatchesPerPhaseStepDefs locks DeploySteps' composition to
// calling each phase's own StepDefs directly — the property that prevents
// dry-run drift is that both routes call the same private xSteps() method,
// not this test; this test only guards DeploySteps' plumbing (right phases,
// right order, right conversion).
func TestDeploySteps_MatchesPerPhaseStepDefs(t *testing.T) {
	cfg := config.DefaultConfig()
	root := t.TempDir()

	p := New(WithProjectRoot(root), WithLogger(logutil.NopLogger))
	got := p.DeploySteps(cfg)

	setupOpts := setup.NewOptions(cfg, root)
	installOpts := install.NewOptions(cfg, root)
	postOpts := postinstall.NewOptions(cfg, root)

	var want []DeployStep
	for _, d := range setup.New(phase.WithLogger(logutil.NopLogger)).StepDefs(cfg, &setupOpts) {
		want = append(want, DeployStep{ID: d.ID, Name: d.Name})
	}
	for _, d := range install.New(phase.WithLogger(logutil.NopLogger)).StepDefs(cfg, &installOpts) {
		want = append(want, DeployStep{ID: d.ID, Name: d.Name})
	}
	for _, d := range postinstall.New(phase.WithLogger(logutil.NopLogger)).StepDefs(cfg, &postOpts) {
		want = append(want, DeployStep{ID: d.ID, Name: d.Name})
	}

	if len(got) != len(want) {
		t.Fatalf("len(DeploySteps) = %d, want %d", len(got), len(want))
	}
	for i, s := range got {
		if s != want[i] {
			t.Errorf("step[%d] = %+v, want %+v", i, s, want[i])
		}
	}
}

func TestDeploySteps_EveryStepHasIDAndName(t *testing.T) {
	cfg := config.DefaultConfig()
	root := t.TempDir()

	p := New(WithProjectRoot(root), WithLogger(logutil.NopLogger))
	steps := p.DeploySteps(cfg)

	if len(steps) == 0 {
		t.Fatal("expected at least one step")
	}
	for _, s := range steps {
		if s.ID == "" {
			t.Error("step has empty ID")
		}
		if s.Name == "" {
			t.Errorf("step %q has empty Name", s.ID)
		}
	}
}

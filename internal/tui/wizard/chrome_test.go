package wizard

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

type nopStep struct{ BaseStep }

func (s *nopStep) Init() tea.Cmd                        { return nil }
func (s *nopStep) Update(tea.Msg) (WizardStep, tea.Cmd) { return s, nil }
func (s *nopStep) View(_, _ int) string                 { return "body" }

var nopStepSeq int

// newNopStep returns a step with a fresh StepID each call, so multi-step
// test models can route JumpToStepMsg unambiguously.
func newNopStep() *nopStep {
	nopStepSeq++
	id := StepID(fmt.Sprintf("nop-%d", nopStepSeq))
	return &nopStep{BaseStep: NewBaseStep(id, "nop", "")}
}

func viewContent(t *testing.T, m *Model) string {
	t.Helper()
	return tuitest.RenderAt(t, m, 100, 30)
}

func TestCustomChromeRendersTaglineAndBadge(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "homelab"
	chrome := FlowChrome{
		Tagline: "cluster lifecycle",
		Badge:   func(c *config.Config) string { return c.Cluster.Name },
	}
	second := newNopStep()
	m := NewFlowModel([]WizardStep{newNopStep(), second}, cfg, chrome)
	out := viewContent(t, m)
	if !strings.Contains(out, "cluster lifecycle") {
		t.Errorf("custom tagline missing on step 1; view:\n%s", out)
	}
	if !strings.Contains(out, "homelab") {
		t.Errorf("custom badge missing; view:\n%s", out)
	}

	m.Update(JumpToStepMsg{StepID: second.ID()})
	out = tuitest.StripANSI(m.View().Content)
	if strings.Contains(out, "cluster lifecycle") {
		t.Errorf("custom tagline must not appear past step 1; view:\n%s", out)
	}
	if !strings.Contains(out, "homelab") {
		t.Errorf("custom badge missing on step 2; view:\n%s", out)
	}
}

func TestStagesTrail_BoldsCurrentStage(t *testing.T) {
	stages := []Stage{
		{Label: "op", Steps: []StepID{"op"}},
		{Label: "target", Steps: []StepID{"target"}},
		{Label: "done", Steps: []StepID{"done"}},
	}
	trail := StagesTrail(stages)

	got := trail(ProgressInfo{CurrentID: "target"})
	want := stageLabelStyle.Render("op") + stageSeparatorStyle.Render(" · ") +
		stageLabelCurrentStyle.Render("target") + stageSeparatorStyle.Render(" · ") +
		stageLabelStyle.Render("done")
	if got != want {
		t.Fatalf("trail = %q, want %q", got, want)
	}
	if stripped := tuitest.StripANSI(got); stripped != "op · target · done" {
		t.Fatalf("stripped trail = %q", stripped)
	}
}

func TestStagesTrail_UnknownCurrentIDToleratesRenderingAllDim(t *testing.T) {
	stages := []Stage{
		{Label: "op", Steps: []StepID{"op"}},
		{Label: "target", Steps: []StepID{"target"}},
	}
	trail := StagesTrail(stages)

	got := trail(ProgressInfo{CurrentID: "not-in-any-stage"})
	want := stageLabelStyle.Render("op") + stageSeparatorStyle.Render(" · ") + stageLabelStyle.Render("target")
	if got != want {
		t.Fatalf("trail = %q, want %q", got, want)
	}
}

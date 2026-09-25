package wizard

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
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

func TestStagesTrail_ActiveStageDotsTrackWithinPhasePosition(t *testing.T) {
	stages := []Stage{
		{Label: "connect", Steps: []StepID{"a", "b", "c"}},
		{Label: "cluster", Steps: []StepID{"d"}},
	}
	trail := StagesTrail(stages)

	got := trail(ProgressInfo{CurrentID: "b", VisibleIDs: []StepID{"a", "b", "c", "d"}})
	want := "connect " + tui.IconActive + tui.IconActive + tui.IconPending + " · cluster"
	if stripped := tuitest.StripANSI(got); stripped != want {
		t.Fatalf("trail = %q, want %q", stripped, want)
	}
}

func TestStagesTrail_HiddenStepExcludedFromDotCountAndPosition(t *testing.T) {
	stages := []Stage{{Label: "cluster", Steps: []StepID{"a", "b", "c", "d"}}}
	trail := StagesTrail(stages)

	// b is hidden: only a, c, d are visible, and c is now the second of three.
	got := trail(ProgressInfo{CurrentID: "c", VisibleIDs: []StepID{"a", "c", "d"}})
	want := "cluster " + tui.IconActive + tui.IconActive + tui.IconPending
	if stripped := tuitest.StripANSI(got); stripped != want {
		t.Fatalf("trail = %q, want %q", stripped, want)
	}
}

func TestStagesTrail_SingleVisibleStepStageRendersBareLabel(t *testing.T) {
	stages := []Stage{{Label: "review", Steps: []StepID{"only"}}}
	trail := StagesTrail(stages)

	got := trail(ProgressInfo{CurrentID: "only", VisibleIDs: []StepID{"only"}})
	if stripped := tuitest.StripANSI(got); stripped != "review" {
		t.Fatalf("trail = %q, want %q", stripped, "review")
	}
}

func TestStagesTrail_MultiStepStageWithOnlyOneVisibleRendersBareLabel(t *testing.T) {
	stages := []Stage{{Label: "connect", Steps: []StepID{"a", "b", "c"}}}
	trail := StagesTrail(stages)

	// b and c are hidden this run: only a is visible, so the stage has no
	// ribbon to show even though it lists 3 steps.
	got := trail(ProgressInfo{CurrentID: "a", VisibleIDs: []StepID{"a"}})
	if stripped := tuitest.StripANSI(got); stripped != "connect" {
		t.Fatalf("trail = %q, want %q", stripped, "connect")
	}
}

// heroNopStep renders the product wordmark itself, so the frame must drop its
// own header rather than printing a second identity above it.
type heroNopStep struct{ nopStep }

func (s *heroNopStep) RendersHero() bool { return true }

func TestHeroStepDropsBrandTaglineAndTrail(t *testing.T) {
	const w, h = 100, 30
	cfg := config.DefaultConfig()
	chrome := FlowChrome{
		Tagline: "okd over proxmox, the easy way",
		Trail:   func(ProgressInfo) string { return "connect · cluster · extras · review" },
	}

	hero := &heroNopStep{*newNopStep()}
	m := NewFlowModel([]WizardStep{hero, newNopStep()}, cfg, chrome)
	plain := tuitest.StripANSI(tuitest.RenderAt(t, m, w, h))

	for _, gone := range []string{"O K D C T L", chrome.Tagline, "connect · cluster", "nop"} {
		if strings.Contains(plain, gone) {
			t.Errorf("hero step must drop %q from the frame header:\n%s", gone, plain)
		}
	}
	if got, want := m.viewport.Height(), h-fixedLayoutOverhead+headerHeight; got != want {
		t.Errorf("viewport height = %d, want %d — the dropped header's rows belong to the body", got, want)
	}
	tuitest.AssertFits(t, tuitest.RenderAt(t, m, w, h), w, h)
}

func TestNonHeroStepKeepsTheFrameHeader(t *testing.T) {
	const w, h = 100, 30
	cfg := config.DefaultConfig()
	chrome := FlowChrome{
		Tagline: "okd over proxmox, the easy way",
		Trail:   func(ProgressInfo) string { return "connect · cluster · extras · review" },
	}

	m := NewFlowModel([]WizardStep{newNopStep(), newNopStep()}, cfg, chrome)
	plain := tuitest.StripANSI(tuitest.RenderAt(t, m, w, h))

	for _, want := range []string{"O K D C T L", chrome.Tagline, "connect · cluster"} {
		if !strings.Contains(plain, want) {
			t.Errorf("a step that renders no hero must keep %q:\n%s", want, plain)
		}
	}
	if got, want := m.viewport.Height(), h-fixedLayoutOverhead; got != want {
		t.Errorf("viewport height = %d, want the full-header %d", got, want)
	}
}

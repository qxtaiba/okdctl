package wizard

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

type nopStep struct{ BaseStep }

func (s *nopStep) Init() tea.Cmd                        { return nil }
func (s *nopStep) Update(tea.Msg) (WizardStep, tea.Cmd) { return s, nil }
func (s *nopStep) View(_, _ int) string                 { return "body" }

func newNopStep() *nopStep { return &nopStep{BaseStep: NewBaseStep("nop", "nop", "")} }

func viewContent(t *testing.T, m *Model) string {
	t.Helper()
	return update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30}).View().Content
}

func TestCustomChromeRendersTaglineAndBadge(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "homelab"
	chrome := FlowChrome{
		Tagline: "cluster lifecycle",
		Badge:   func(c *config.Config) string { return c.Cluster.Name },
	}
	m := NewFlowModel([]WizardStep{newNopStep()}, cfg, chrome)
	out := viewContent(t, m)
	if !strings.Contains(out, "cluster lifecycle") {
		t.Errorf("custom tagline missing; view:\n%s", out)
	}
	if !strings.Contains(out, "homelab") {
		t.Errorf("custom badge missing; view:\n%s", out)
	}
}

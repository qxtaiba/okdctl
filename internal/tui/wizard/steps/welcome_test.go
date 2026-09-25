package steps

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// press drives the hub through its real Update, the way a keystroke reaches it.
func press(t *testing.T, s *WelcomeStep, msg tea.KeyPressMsg) {
	t.Helper()
	s.Update(msg)
}

var (
	keyDown  = tea.KeyPressMsg{Code: 'j', Text: "j"}
	keyUp    = tea.KeyPressMsg{Code: 'k', Text: "k"}
	keyRight = tea.KeyPressMsg{Code: tea.KeyRight}
	keyLeft  = tea.KeyPressMsg{Code: tea.KeyLeft}
)

func hubConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "prod-cluster"
	cfg.Distribution.Version = "4.20.1-okd-scos.7"
	cfg.Topology.ControlPlane.Count = 3
	cfg.Topology.Workers.Count = 3
	return cfg
}

func TestHubFreshMenuIsGetStartedAndQuit(t *testing.T) {
	s := NewWelcomeStep()
	if got := s.SelectedVerb(); got != HubVerbGetStarted {
		t.Errorf("SelectedVerb() on a blank slate = %v, want HubVerbGetStarted", got)
	}
	press(t, s, keyDown)
	if got := s.SelectedVerb(); got != HubVerbQuit {
		t.Errorf("SelectedVerb() after down = %v, want HubVerbQuit", got)
	}
	press(t, s, keyDown)
	if got := s.SelectedVerb(); got != HubVerbQuit {
		t.Errorf("SelectedVerb() past the last entry = %v, want it clamped to HubVerbQuit", got)
	}
}

func TestHubExistingMenuIsTheFiveVerbs(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true)

	want := []HubVerb{HubVerbDeploy, HubVerbEditConfig, HubVerbManageNodes, HubVerbClusterStatus, HubVerbDestroy}
	for i, verb := range want {
		if got := s.SelectedVerb(); got != verb {
			t.Fatalf("entry %d = %v, want %v", i, got, verb)
		}
		press(t, s, keyDown)
	}
	if got := s.SelectedVerb(); got != HubVerbDestroy {
		t.Errorf("SelectedVerb() past the last verb = %v, want it clamped to HubVerbDestroy", got)
	}
	press(t, s, keyUp)
	if got := s.SelectedVerb(); got != HubVerbClusterStatus {
		t.Errorf("SelectedVerb() after up = %v, want HubVerbClusterStatus", got)
	}
}

func TestHubArrowsMoveThePointerVertically(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true)

	press(t, s, keyRight)
	if got := s.SelectedVerb(); got != HubVerbEditConfig {
		t.Fatalf("SelectedVerb() after right = %v, want HubVerbEditConfig", got)
	}
	press(t, s, keyLeft)
	if got := s.SelectedVerb(); got != HubVerbDeploy {
		t.Fatalf("SelectedVerb() after left = %v, want HubVerbDeploy", got)
	}
}

func TestHubSaveSlotLineFromConfig(t *testing.T) {
	s := NewWelcomeStep()
	s.SetExistingConfig(hubConfig(), SaveSlotDeployed)

	if !s.configExists {
		t.Fatal("SetExistingConfig did not switch the hub to its five-verb menu")
	}
	want := "prod-cluster · okd 4.20.1-okd-scos.7 · 6 nodes · deployed"
	if s.saveSlot != want {
		t.Errorf("saveSlot = %q, want %q", s.saveSlot, want)
	}
}

func TestHubSaveSlotLineSingularNode(t *testing.T) {
	cfg := hubConfig()
	cfg.Topology.ControlPlane.Count = 1
	cfg.Topology.Workers.Count = 0

	s := NewWelcomeStep()
	s.SetExistingConfig(cfg, SaveSlotConfigured)

	if !strings.Contains(s.saveSlot, "1 node ·") {
		t.Errorf("saveSlot = %q, want a singular \"1 node\" segment", s.saveSlot)
	}
}

func TestHubFreshSlateHasNoSaveSlotLine(t *testing.T) {
	s := NewWelcomeStep()
	if s.saveSlot != "" {
		t.Errorf("saveSlot = %q on a blank slate, want empty", s.saveSlot)
	}
}

func TestHubVerbsRouteToTerminalActions(t *testing.T) {
	cases := []struct {
		verb      HubVerb
		action    wizard.Action
		exitEarly bool
	}{
		{HubVerbDeploy, wizard.ActionDeploy, true},
		{HubVerbEditConfig, wizard.ActionExit, false},
		{HubVerbManageNodes, wizard.ActionExit, true},
		{HubVerbClusterStatus, wizard.ActionExit, true},
		{HubVerbDestroy, wizard.ActionExit, true},
	}

	for _, c := range cases {
		s := NewWelcomeStep()
		s.SetConfigExists(true)
		for s.SelectedVerb() != c.verb {
			press(t, s, keyDown)
		}
		if got := s.GetSelectedAction(); got != c.action {
			t.Errorf("verb %v: GetSelectedAction() = %v, want %v", c.verb, got, c.action)
		}
		if got := s.ShouldExitEarly(); got != c.exitEarly {
			t.Errorf("verb %v: ShouldExitEarly() = %v, want %v", c.verb, got, c.exitEarly)
		}
	}
}

func TestHubGetStartedWalksTheConfigureFlow(t *testing.T) {
	s := NewWelcomeStep()
	if s.ShouldExitEarly() {
		t.Error("ShouldExitEarly() = true for get started, want false so the flow advances")
	}
}

func TestHubConfigExistsFalseClearsSaveSlotFromView(t *testing.T) {
	s := NewWelcomeStep()
	s.SetExistingConfig(hubConfig(), SaveSlotDeployed)
	s.SetTerminalSize(80, 24)

	s.SetConfigExists(false)

	body := s.View(70, 14)
	if strings.Contains(body, "prod-cluster") {
		t.Errorf("hub body still renders the save-slot line after SetConfigExists(false):\n%s", body)
	}
}

func TestHubBodyCarriesOnlyHeroSlotAndVerbs(t *testing.T) {
	s := NewWelcomeStep()
	s.SetExistingConfig(hubConfig(), SaveSlotConfigured)
	s.SetTerminalSize(80, 24)

	body := s.View(70, 14)
	for _, gone := range []string{"answer a few questions", "you'll need", "it takes", "proxmox credentials"} {
		if strings.Contains(body, gone) {
			t.Errorf("hub body still renders %q; the tagline and checklist columns are deleted:\n%s", gone, body)
		}
	}
	for _, want := range []string{"deploy", "edit config", "manage nodes", "cluster status", "destroy", "prod-cluster"} {
		if !strings.Contains(body, want) {
			t.Errorf("hub body is missing %q:\n%s", want, body)
		}
	}
}

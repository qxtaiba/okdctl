package steps

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

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
	cfg.Workers.Count = 3
	return cfg
}

func TestHubShortHelpEnterLabelIsStart(t *testing.T) {
	s := NewWelcomeStep()
	for _, b := range s.ShortHelp() {
		if b.Key == wizard.HelpEnter {
			if b.Help != wizard.HelpStart {
				t.Errorf("enter label = %q, want %q", b.Help, wizard.HelpStart)
			}
			return
		}
	}
	t.Fatal("ShortHelp() carries no enter binding")
}

func TestHubMenusClampAtTheirEnds(t *testing.T) {
	for name, tc := range map[string]struct {
		exists bool
		verbs  []HubVerb
	}{
		"blank slate": {false, []HubVerb{HubVerbGetStarted, HubVerbQuit}},
		"existing":    {true, []HubVerb{HubVerbEditConfig, HubVerbQuit}},
	} {
		t.Run(name, func(t *testing.T) {
			s := NewWelcomeStep()
			s.SetConfigExists(tc.exists)
			for i, verb := range tc.verbs {
				if got := s.SelectedVerb(); got != verb {
					t.Fatalf("entry %d = %v, want %v", i, got, verb)
				}
				s.Update(keyDown)
			}
			if got := s.SelectedVerb(); got != HubVerbQuit {
				t.Errorf("SelectedVerb() past the last entry = %v, want it clamped to quit", got)
			}
			s.Update(keyUp)
			if got := s.SelectedVerb(); got != tc.verbs[0] {
				t.Errorf("SelectedVerb() after up = %v, want %v", got, tc.verbs[0])
			}
		})
	}
}

func TestHubBlankSlateSuppressesVersionBadge(t *testing.T) {
	s := NewWelcomeStep()
	if !s.SuppressesBadge() {
		t.Error("blank-slate hub must suppress the version badge")
	}
	s.SetConfigExists(true)
	if s.SuppressesBadge() {
		t.Error("a hub over a real config must show the badge")
	}
}

func TestHubArrowsMoveThePointerVertically(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true)

	s.Update(keyRight)
	if got := s.SelectedVerb(); got != HubVerbQuit {
		t.Fatalf("SelectedVerb() after right = %v, want quit", got)
	}
	s.Update(keyLeft)
	if got := s.SelectedVerb(); got != HubVerbEditConfig {
		t.Fatalf("SelectedVerb() after left = %v, want edit config", got)
	}
}

func TestHubSaveSlotLineFromConfig(t *testing.T) {
	s := NewWelcomeStep()
	s.SetExistingConfig(hubConfig())

	want := "prod-cluster · okd 4.20.1-okd-scos.7 · 6 nodes · configured"
	if !s.configExists || s.saveSlot != want {
		t.Errorf("configExists=%v saveSlot=%q, want the edit menu and %q", s.configExists, s.saveSlot, want)
	}
	s.SetConfigExists(false)
	if body := s.View(70, 14); strings.Contains(body, "prod-cluster") {
		t.Errorf("hub body still renders the save-slot line after SetConfigExists(false):\n%s", body)
	}
}

func TestHubExistingViewCarriesSlotAndVerbsOnly(t *testing.T) {
	s := NewWelcomeStep()
	s.SetExistingConfig(hubConfig())
	s.SetTerminalSize(80, 24)
	if !s.IsCentered() {
		t.Fatal("hub should keep the centered launcher")
	}

	view := tuitest.StripANSI(s.View(70, 14))
	for _, want := range []string{s.saveSlot, "[e] edit config", "    quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("hub is missing %q:\n%s", want, view)
		}
	}
	for _, gone := range []string{"deploy", "manage nodes", "cluster status", "destroy", "okdctl up"} {
		if strings.Contains(view, gone) {
			t.Errorf("hub advertises %q, a verb no command backs:\n%s", gone, view)
		}
	}
	if fresh := tuitest.StripANSI(NewWelcomeStep().View(70, 14)); strings.Contains(fresh, "[") {
		t.Errorf("blank-slate menu must carry no accelerator brackets:\n%s", fresh)
	}
}

func TestHubGetStartedPanelPromisesNoInstall(t *testing.T) {
	if panel := tuitest.StripANSI(renderGetStartedPanel()); strings.Contains(panel, "deploy") || strings.Contains(panel, "okdctl up") {
		t.Fatalf("get-started panel promises an install:\n%s", panel)
	}
}

func TestHubVerbsExitEarlyExceptTheConfigureFlows(t *testing.T) {
	for verb, exitEarly := range map[HubVerb]bool{HubVerbGetStarted: false, HubVerbEditConfig: false, HubVerbQuit: true} {
		s := NewWelcomeStep()
		s.SetConfigExists(verb != HubVerbGetStarted)
		for s.SelectedVerb() != verb {
			s.Update(keyDown)
		}
		if got := s.ShouldExitEarly(); got != exitEarly {
			t.Errorf("verb %v: ShouldExitEarly() = %v, want %v", verb, got, exitEarly)
		}
	}
}

func TestHubAcceleratorsConfirmDirectly(t *testing.T) {
	for _, key := range []string{"e", "1"} {
		s := NewWelcomeStep()
		s.SetConfigExists(true)
		s.Update(keyDown)
		_, cmd := s.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
		if s.SelectedVerb() != HubVerbEditConfig || cmd == nil {
			t.Errorf("key %q: verb %v cmd %v, want edit config confirmed", key, s.SelectedVerb(), cmd)
		}
	}
	for _, key := range []string{"q", "2"} {
		s := NewWelcomeStep()
		s.SetConfigExists(true)
		if _, cmd := s.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key}); cmd != nil || s.SelectedVerb() != HubVerbEditConfig {
			t.Errorf("key %q reached quit, want the pointer-only row untouched", key)
		}
	}
}

func TestHubQuitCompletesTheStepAndExitsTheFlow(t *testing.T) {
	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, 120, 40)
	m.CurrentStep().(*WelcomeStep).SetExistingConfig(hubConfig())
	m.Update(keyDown)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	pumpCmd(m, cmd)

	res := m.Result()
	if res.Outcome != wizard.OutcomeCompleted || res.ExitStep != wizard.StepIDWelcome {
		t.Fatalf("result = %+v; want a completed exit from welcome", res)
	}
}

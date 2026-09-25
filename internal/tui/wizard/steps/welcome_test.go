package steps

import (
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
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

// TestHubShortHelpEnterLabelIsStart guards E-C5: welcome is the one screen
// whose enter label reads "start" (see keymap_help.go's rule) — it begins
// the wizard rather than advancing a form or confirming a decision already
// made.
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

// selectVerb walks the pointer to verb with real keypresses.
func selectVerb(t *testing.T, s *WelcomeStep, verb HubVerb) {
	t.Helper()
	for range len(s.entries) {
		if s.SelectedVerb() == verb {
			return
		}
		press(t, s, keyDown)
	}
	t.Fatalf("verb %v is not on the menu", verb)
}

func TestHubManageVerbSwapsInItsFlow(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true)

	want := []wizard.WizardStep{NewStatusStep(nil)}
	calls := 0
	s.SetFlows(HubFlows{ManageNodes: func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
		calls++
		return want, wizard.FlowChrome{Tagline: "day-2 node operations"}, nil
	}})

	selectVerb(t, s, HubVerbManageNodes)
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirming manage nodes produced no command")
	}
	if s.opening != "manage nodes" {
		t.Errorf("opening = %q, want the verb being opened", s.opening)
	}

	swap, ok := resolveCmd(t, cmd).(wizard.SwapFlowMsg)
	if !ok {
		t.Fatalf("command produced %T, want wizard.SwapFlowMsg", resolveCmd(t, cmd))
	}
	if calls != 1 {
		t.Errorf("flow built %d times, want 1", calls)
	}
	if len(swap.Steps) != len(want) || swap.Chrome.Tagline != "day-2 node operations" {
		t.Errorf("swap carried %d steps with tagline %q, want the provider's flow", len(swap.Steps), swap.Chrome.Tagline)
	}
}

func TestHubSurfacesAFlowThatCannotBeBuilt(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true)
	s.SetFlows(HubFlows{ManageNodes: func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
		return nil, wizard.FlowChrome{}, errors.New("prepare node ops: no proxmox credentials")
	}})

	selectVerb(t, s, HubVerbManageNodes)
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	failed, ok := resolveCmd(t, cmd).(hubFlowFailedMsg)
	if !ok {
		t.Fatalf("command produced %T, want hubFlowFailedMsg", resolveCmd(t, cmd))
	}

	_, errCmd := s.Update(failed)
	if s.opening != "" {
		t.Errorf("opening = %q after a failed build, want it cleared", s.opening)
	}
	setErr, ok := errCmd().(wizard.ErrorSetMsg)
	if !ok {
		t.Fatalf("follow-up command produced %T, want wizard.ErrorSetMsg", errCmd())
	}
	if !strings.Contains(setErr.Error.Error(), "no proxmox credentials") {
		t.Errorf("surfaced error = %v, want the provider's", setErr.Error)
	}
}

// TestHubOpeningNoticeAnimatesASpinner guards NEW(T7): the "opening …"
// notice carries a live spinner (mirroring NodePlacementStep's discovery
// spinner) rather than sitting static — confirm() must arm the tick, View
// must render the glyph, and Update must keep re-arming it only while the
// notice is still up.
func TestHubOpeningNoticeAnimatesASpinner(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true)
	s.SetFlows(HubFlows{ManageNodes: func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
		return []wizard.WizardStep{NewStatusStep(nil)}, wizard.FlowChrome{}, nil
	}})
	selectVerb(t, s, HubVerbManageNodes)

	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirming manage nodes produced no command")
	}
	before := s.View(70, 14)
	if before == "" || !strings.Contains(before, "opening manage nodes") {
		t.Fatalf("View() before any tick = %q, want the opening notice present", before)
	}

	// Feed every tick message the initial batch produced (build cmd plus the
	// spinner's own tick) back through Update, the way the real run loop
	// would — the spinner frame must advance.
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("confirm's command produced %T, want a tea.BatchMsg batching the build with the spinner tick", cmd())
	}
	sawTick := false
	for _, c := range batch {
		msg := c()
		if _, isTick := msg.(spinner.TickMsg); isTick {
			sawTick = true
			_, tickCmd := s.Update(msg)
			if tickCmd == nil {
				t.Error("Update(spinner.TickMsg) while opening produced no re-arming command")
			}
		}
	}
	if !sawTick {
		t.Fatal("confirm's batch never produced a spinner.TickMsg")
	}

	// Once the notice clears (flow opened or failed), a stray tick must not
	// re-arm another one — the spinner is done animating.
	s.opening = ""
	if _, tickCmd := s.Update(spinner.TickMsg{}); tickCmd != nil {
		t.Error("Update(spinner.TickMsg) after opening cleared re-armed another tick")
	}
}

func TestHubVerbWithNoFlowCompletesTheStep(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true)
	selectVerb(t, s, HubVerbDestroy)

	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if _, ok := cmd().(wizard.StepCompleteMsg); !ok {
		t.Fatalf("command produced %T, want wizard.StepCompleteMsg", cmd())
	}
	if s.opening != "" {
		t.Errorf("opening = %q, want empty for a verb with no in-process flow", s.opening)
	}
}

func TestHubClearsTheOpeningNoticeOnReturn(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true)
	s.SetFlows(HubFlows{ClusterStatus: func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
		return []wizard.WizardStep{NewStatusStep(nil)}, wizard.FlowChrome{}, nil
	}})

	selectVerb(t, s, HubVerbClusterStatus)
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	s.SetFocused(false)
	s.SetFocused(true)

	if s.opening != "" {
		t.Errorf("opening = %q after the hub regained focus, want it cleared", s.opening)
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

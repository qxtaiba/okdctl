package steps

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
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

// TestHubBlankSlateSuppressesVersionBadge pins bug 37: the blank-slate hub
// must not advertise the defaults-seed okd version as if a cluster existed;
// the badge returns once a real configuration is loaded.
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
	if !s.IsCentered() {
		t.Error("blank-slate hub should retain its centered launcher")
	}
	if view := s.View(70, 14); strings.Contains(view, "CLUSTER OPERATIONS") {
		t.Errorf("blank-slate hub should not show an operations dashboard:\n%s", view)
	}
}

func TestHubDeployedViewUsesDashboardAndKeepsActionsVisible(t *testing.T) {
	s := NewWelcomeStep()
	s.SetExistingConfig(hubConfig(), SaveSlotDeployed)
	s.SetOpsDashboard(StaticStatusSource{Status: statusFixture()})
	s.opsStatus = &opsSnapshot{status: statusFixture(), updated: time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)}
	s.SetTerminalSize(180, 48)

	if s.IsCentered() {
		t.Fatal("deployed dashboard should use the available viewport")
	}
	view := tuitest.StripANSI(s.View(172, 41))
	for _, want := range []string{
		"CLUSTER OPERATIONS", "CLUSTER PHASE", "NODE FLEET", "homelab-master0",
		"ADD-ONS & OPERATORS", "HUB ACTIONS", "deploy", "edit config",
		"manage nodes", "cluster status", "destroy",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("deployed dashboard is missing %q:\n%s", want, view)
		}
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

func TestHubDashboardShortcutOpensNativeClusterStatus(t *testing.T) {
	s := NewWelcomeStep()
	s.SetExistingConfig(hubConfig(), SaveSlotDeployed)
	want := []wizard.WizardStep{NewStatusStep(StaticStatusSource{Status: statusFixture()})}
	s.SetFlows(HubFlows{ClusterStatus: func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
		return want, StatusChrome(), nil
	}})
	_, cmd := s.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if cmd == nil {
		t.Fatal("pressing the dashboard's s shortcut produced no status flow")
	}
	swap, ok := resolveCmd(t, cmd).(wizard.SwapFlowMsg)
	if !ok || len(swap.Steps) != 1 || swap.Steps[0].ID() != StepIDClusterStatus {
		t.Fatalf("dashboard shortcut opened %#v, want the native one-screen status flow", swap)
	}
}

func TestHubDraftChipIsAnActionableResumeEntry(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true)
	s.SetDraftResume(wizard.StepIDNetworking, "machine_cidr", "resume draft · at networking · 5m ago")
	if got := s.entries[0]; got.verb != HubVerbResumeDraft || !strings.Contains(got.label, "at networking") {
		t.Fatalf("first entry = %+v, want the saved draft as the first action", got)
	}
	view := s.View(80, 24)
	if !strings.Contains(view, "resume draft") || !strings.Contains(view, "5m ago") {
		t.Errorf("draft chip is missing its resume target or age:\n%s", view)
	}
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirming the draft chip produced no resume command")
	}
	msg, ok := resolveCmd(t, cmd).(wizard.DraftResumeMsg)
	if !ok || msg.StepID != wizard.StepIDNetworking || msg.FieldKey != "machine_cidr" {
		t.Fatalf("draft command = %#v, want networking/machine_cidr resume", msg)
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

// TestHubOpeningNoticeAnimatesASpinner guards NEW(T7): the "opening ..."
// notice carries a live spinner driven by the wizard's shared frame clock --
// Animating must report true exactly while the notice is up, and a FrameMsg
// must advance the rendered glyph.
func TestHubOpeningNoticeAnimatesASpinner(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true)
	s.SetFlows(HubFlows{ManageNodes: func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
		return []wizard.WizardStep{NewStatusStep(nil)}, wizard.FlowChrome{}, nil
	}})
	selectVerb(t, s, HubVerbManageNodes)

	if s.Animating() {
		t.Fatal("Animating() = true before any verb was confirmed")
	}
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirming manage nodes produced no command")
	}
	before := s.View(70, 14)
	if before == "" || !strings.Contains(before, "opening manage nodes") {
		t.Fatalf("View() before any frame = %q, want the opening notice present", before)
	}
	if !s.Animating() {
		t.Fatal("Animating() = false while the opening notice is up")
	}

	s.Update(wizard.FrameMsg{Frame: 3})
	after := s.View(70, 14)
	if after == before {
		t.Fatal("View() unchanged after a frame - the opening spinner is static")
	}

	// Once the notice clears (flow opened or failed), the step stops asking
	// for frames and the shared clock winds down.
	s.opening = ""
	if s.Animating() {
		t.Error("Animating() = true after the opening notice cleared")
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

// TestHubVerbAcceleratorsConfirmDirectly pins the hub accelerators: d/e/n/s
// and digits 1-4 select and confirm their verb in one keystroke, echoing
// review's [N] jump grammar.
func TestHubVerbAcceleratorsConfirmDirectly(t *testing.T) {
	cases := []struct {
		key  string
		verb HubVerb
	}{
		{"d", HubVerbDeploy},
		{"e", HubVerbEditConfig},
		{"n", HubVerbManageNodes},
		{"s", HubVerbClusterStatus},
		{"1", HubVerbDeploy},
		{"2", HubVerbEditConfig},
		{"3", HubVerbManageNodes},
		{"4", HubVerbClusterStatus},
	}
	for _, c := range cases {
		s := NewWelcomeStep()
		s.SetConfigExists(true)
		_, cmd := s.Update(tea.KeyPressMsg{Code: rune(c.key[0]), Text: c.key})
		if s.SelectedVerb() != c.verb {
			t.Errorf("key %q: SelectedVerb() = %v, want %v", c.key, s.SelectedVerb(), c.verb)
		}
		if cmd == nil {
			t.Errorf("key %q: no confirm command returned", c.key)
		}
	}
}

// TestHubDestroyHasNoSingleKeyAccelerator pins the friction ladder: destroy
// is reachable only by pointer + enter, never one keystroke.
func TestHubDestroyHasNoSingleKeyAccelerator(t *testing.T) {
	for _, key := range []string{"x", "5"} {
		s := NewWelcomeStep()
		s.SetConfigExists(true)
		_, cmd := s.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
		if s.SelectedVerb() == HubVerbDestroy {
			t.Errorf("key %q selected destroy", key)
		}
		if cmd != nil {
			t.Errorf("key %q confirmed a verb, want it inert", key)
		}
	}
}

// TestHubMenuRendersAcceleratorHints pins the [x] grammar on the five-verb
// menu — destroy indented with no bracket — and its absence from the
// blank-slate menu.
func TestHubMenuRendersAcceleratorHints(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true)
	view := s.View(70, 14)
	for _, want := range []string{"[d] deploy", "[e] edit config", "[n] manage nodes", "[s] cluster status", "    destroy"} {
		if !strings.Contains(view, want) {
			t.Errorf("five-verb menu missing %q:\n%s", want, view)
		}
	}

	fresh := NewWelcomeStep()
	fresh.SetConfigExists(false)
	if view := tuitest.StripANSI(fresh.View(70, 14)); strings.Contains(view, "[") {
		t.Errorf("blank-slate menu must carry no accelerator brackets:\n%s", view)
	}
}

func TestHubDraftResumeIsFirstAndTargetsSavedCursor(t *testing.T) {
	for _, exists := range []bool{false, true} {
		t.Run(fmt.Sprintf("config-exists-%t", exists), func(t *testing.T) {
			s := NewWelcomeStep()
			s.SetConfigExists(exists)
			label := "resume draft · at networking · edited 2h ago"
			s.SetDraftResume(wizard.StepIDNetworking, "machine_cidr", label)
			if got := s.SelectedVerb(); got != HubVerbResumeDraft {
				t.Fatalf("SelectedVerb() = %v, want HubVerbResumeDraft", got)
			}
			s.SetTerminalSize(100, 30)
			if body := s.View(90, 20); !strings.Contains(body, label) {
				t.Errorf("hub view does not show the resume affordance:\n%s", body)
			}
			_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("confirming resume draft returned no command")
			}
			msg, ok := resolveCmd(t, cmd).(wizard.DraftResumeMsg)
			if !ok || msg.StepID != wizard.StepIDNetworking || msg.FieldKey != "machine_cidr" {
				t.Fatalf("resume command = %#v, want networking/machine_cidr", resolveCmd(t, cmd))
			}
		})
	}
}

func TestHubConfirmEndsOnWelcome(t *testing.T) {
	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, 120, 40)
	seedHubSaveSlot(m)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	pumpCmd(m, cmd)

	res := m.Result()
	if res.Outcome != wizard.OutcomeCompleted || res.Action != wizard.ActionDeploy {
		t.Fatalf("result = outcome %v action %q; want a completed deploy", res.Outcome, res.Action)
	}
	if res.ExitStep != wizard.StepIDWelcome {
		t.Errorf("ExitStep = %q; want %q", res.ExitStep, wizard.StepIDWelcome)
	}
}

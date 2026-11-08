package steps

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

var placementHosts = []string{"pve1", "pve2", "pve3"}

func newPlacementConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Nodes = placementHosts
	return cfg
}

func TestNodePlacementBuildsOneHostSelectPerModelNode(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newPlacementConfig()
	s.buildInnerStep(nil, placementHosts)

	want := map[string]string{
		"lab-master0": "pve1", "lab-master1": "pve2", "lab-master2": "pve3",
		"lab-worker0": "pve1", "lab-worker1": "pve2",
	}
	if len(s.hostFields) != len(want) {
		t.Fatalf("built %d host fields, want %d: %v", len(s.hostFields), len(want), s.hostFields)
	}
	for name, host := range want {
		if got := s.hostFields[name].Value(); got != host {
			t.Errorf("%s defaults to %q, want the model's %q", name, got, host)
		}
	}
	if got := s.poolField.Value(); got != "pve1,pve2,pve3" {
		t.Errorf("pool = %q, want the config's nodes", got)
	}
}

func TestNodePlacementApplyWritesOnlyDifferingHostsAsOverrides(t *testing.T) {
	cfg := newPlacementConfig()
	cfg.Nodes = map[string]config.NodeOverride{
		"lab-master1": {Host: "pve1", MemoryMiB: new(32768)},
		"lab-worker1": {Host: "pve3"},
	}
	s := NewNodePlacementStep()
	s.cfg = cfg
	s.buildInnerStep(nil, placementHosts)

	if got := s.hostFields["lab-master1"].Value(); got != "pve1" {
		t.Fatalf("lab-master1 shows %q, want its saved override pve1", got)
	}
	s.hostFields["lab-master0"].SetValue("pve3")
	s.hostFields["lab-master1"].SetValue("pve2")
	s.hostFields["lab-worker1"].SetValue("pve2")

	if err := s.Apply(cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Nodes["lab-master0"].Host; got != "pve3" {
		t.Errorf("lab-master0 override = %q, want pve3", got)
	}
	if o := cfg.Nodes["lab-master1"]; o.Host != "" || o.MemoryMiB == nil || *o.MemoryMiB != 32768 {
		t.Errorf("lab-master1 back on its default must keep the memory override and drop the host: %+v", o)
	}
	if _, kept := cfg.Nodes["lab-worker1"]; kept {
		t.Errorf("lab-worker1 back on its default must lose its empty override: %+v", cfg.Nodes)
	}
	if len(cfg.Nodes) != 2 {
		t.Errorf("overrides = %+v, want exactly lab-master0 and lab-master1", cfg.Nodes)
	}
}

func TestNodePlacementApplyWritesThePoolAndReplacesItsDefaults(t *testing.T) {
	cfg := newPlacementConfig()
	s := NewNodePlacementStep()
	s.cfg = cfg
	s.buildInnerStep(nil, placementHosts)
	s.poolField.SetValue("pve2,pve3")
	s.hostFields["lab-master0"].SetValue("pve2")

	if err := s.Apply(cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Provider.Proxmox.Nodes; len(got) != 2 || got[0] != "pve2" || got[1] != "pve3" {
		t.Errorf("pool = %v, want [pve2 pve3]", got)
	}
	if _, overridden := cfg.Nodes["lab-master0"]; overridden {
		t.Errorf("pve2 is lab-master0's default on the new pool; no override expected: %+v", cfg.Nodes)
	}
	if got := cfg.Nodes["lab-master1"].Host; got != "pve2" {
		t.Errorf("lab-master1 kept pve2 from the old pool, so it needs an override on the new one; got %q", got)
	}
}

func TestNodePlacementApplySkipsNodesTheConfigNoLongerProduces(t *testing.T) {
	cfg := newPlacementConfig()
	s := NewNodePlacementStep()
	s.cfg = cfg
	s.buildInnerStep(nil, placementHosts)
	s.hostFields["lab-worker1"].SetValue("pve3")
	cfg.Workers.Count = 1

	if err := s.Apply(cfg); err != nil {
		t.Fatal(err)
	}
	if _, stale := cfg.Nodes["lab-worker1"]; stale {
		t.Errorf("a stale host field wrote an override for a node the config no longer has: %+v", cfg.Nodes)
	}
}

func TestNodePlacementStep_NoEndpointSettlesIntoError(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = &config.Config{}

	if cmd := s.Init(); cmd != nil {
		t.Fatal("Init with no endpoint returned a command, want none")
	}
	view := s.View(100, 30)
	if strings.Contains(view, "discovering") || !strings.Contains(view, "proxmox") {
		t.Fatalf("View must carry the explanatory error, not keep discovering:\n%s", view)
	}
}

func TestNodePlacementStep_UnbuildableModelExplainsItself(t *testing.T) {
	cfg := newPlacementConfig()
	cfg.Networking.MachineCIDR = "not-a-cidr"
	s := NewNodePlacementStep()
	s.cfg = cfg
	step, _ := s.Update(discoveryCompleteMsg{discovery: &proxmoxDiscovery{Nodes: []proxmoxNode{{Name: "pve1", Status: "online"}}}})
	s = step.(*NodePlacementStep)

	view := tuitest.StripANSI(s.View(100, 30))
	if !strings.Contains(view, "node placement unavailable") || len(s.hostFields) != 0 {
		t.Fatalf("an unbuildable model must be explained and leave only the pool:\n%s", view)
	}
}

func TestNodePlacementDemandFollowsSelection(t *testing.T) {
	cfg := newPlacementConfig()
	cfg.Provider.Proxmox.Nodes = []string{"pve1"}
	s := NewNodePlacementStep()
	s.cfg = cfg
	disc := &proxmoxDiscovery{Nodes: []proxmoxNode{
		{Name: "pve1", Status: "online", CPUs: 4, CPUsKnown: true, MemGB: 8, MemKnown: true},
		{Name: "pve2", Status: "offline", CPUs: 64, CPUsKnown: true, MemGB: 256, MemKnown: true},
	}}
	step, _ := s.Update(discoveryCompleteMsg{discovery: disc})
	s = step.(*NodePlacementStep)

	initial := tuitest.StripANSI(s.View(120, 40))
	for _, want := range []string{"discovered 2 node(s)", "pve1 ✓ online · 4c · 8g", "pve2 offline · 64c · 256g", "pve1: 28c/80g/1600gb", "oversubscribed"} {
		if !strings.Contains(initial, want) {
			t.Errorf("initial view does not contain %q:\n%s", want, initial)
		}
	}
	if got := tuitest.StripANSI(strings.Join(nodeDisplayOptions(disc.Nodes, []string{"pve2"}), "")); got != "pve2 — 64c/256g offline" {
		t.Errorf("offline node option = %q, want annotated value", got)
	}

	s.hostFields["lab-master0"].SetValue("pve2")
	updated := tuitest.StripANSI(s.View(120, 40))
	for _, want := range []string{"pve1: 24c/64g/1480gb", "pve2: 4c/16g/120gb"} {
		if !strings.Contains(updated, want) {
			t.Errorf("updated view has no %q:\n%s", want, updated)
		}
	}
}

func TestNodePlacementAssignmentDemandElidesVisiblyAtNarrowWidth(t *testing.T) {
	hosts := []string{"homelab-rack-a-node-001", "homelab-rack-b-node-002", "homelab-rack-c-node-003"}
	cfg := newPlacementConfig()
	cfg.Provider.Proxmox.Nodes = hosts
	s := NewNodePlacementStep()
	s.cfg = cfg
	nodes := make([]proxmoxNode, len(hosts))
	for i, h := range hosts {
		nodes[i] = proxmoxNode{Name: h, Status: "online", CPUs: 8, CPUsKnown: true, MemGB: 16, MemKnown: true}
	}
	step, _ := s.Update(discoveryCompleteMsg{discovery: &proxmoxDiscovery{Nodes: nodes}})
	s = step.(*NodePlacementStep)

	demand := s.assignmentDemand(78)
	if got := lipgloss.Width(demand); got > 78 || strings.Contains(demand, "\n") {
		t.Fatalf("demand row is %d columns / multi-line, want one elided line <= 78: %q", got, tuitest.StripANSI(demand))
	}
	plain := tuitest.StripANSI(demand)
	if !strings.Contains(plain, "assigned demand") || !strings.HasSuffix(strings.TrimRight(plain, " "), "…") {
		t.Fatalf("overflowing demand row lost its label or elision marker: %q", plain)
	}
	for _, line := range strings.Split(tuitest.StripANSI(s.View(80, 40)), "\n") {
		if got := lipgloss.Width(line); got > 80 {
			t.Fatalf("view line is %d columns wide, want <= 80: %q", got, line)
		}
	}
}

func TestNodePlacementStep_DiscoveryTransitionsToPlacingPhase(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newPlacementConfig()
	if s.phase != phaseDiscovering || s.inner != nil {
		t.Fatalf("initial phase = %v, inner = %v; want discovering with no form", s.phase, s.inner)
	}

	disc := &proxmoxDiscovery{Nodes: []proxmoxNode{{Name: "pve1"}, {Name: "pve2"}}}
	_, _ = s.Update(discoveryCompleteMsg{discovery: disc})

	if s.phase != phasePlacing || s.discovery != disc || s.inner == nil {
		t.Fatalf("after discovery: phase=%v discovery=%p inner=%v", s.phase, s.discovery, s.inner)
	}
}

func TestNodePlacementStep_TabAdvancesAcrossSections(t *testing.T) {
	cfg := newPlacementConfig()
	cfg.Workers.Count = 1
	s := NewNodePlacementStep()
	s.cfg = cfg
	s.buildInnerStep(nil, placementHosts)
	s.phase = phasePlacing
	s.SetFocused(true)

	tab := tea.KeyPressMsg{Code: tea.KeyTab}
	for i, want := range []int{0, 1, 1, 1, 2, 2} {
		_, _ = s.Update(tab)
		if got := s.inner.CurrentSection(); got != want {
			t.Fatalf("CurrentSection() after tab %d = %d, want %d", i+1, got, want)
		}
	}
}

func TestNodePlacementStep_ShortHelp(t *testing.T) {
	s := NewNodePlacementStep()
	if got := s.ShortHelp(); len(got) == 0 {
		t.Fatal("ShortHelp() while discovering is nil, want {esc back, ctrl+c quit}")
	}
	s.cfg = newPlacementConfig()
	s.phase = phasePlacing
	s.buildInnerStep(nil, placementHosts)
	for _, b := range s.ShortHelp() {
		if b.Key == wizard.HelpEnter {
			if b.Help != wizard.HelpContinue {
				t.Errorf("enter label = %q, want %q", b.Help, wizard.HelpContinue)
			}
			return
		}
	}
	t.Fatal("ShortHelp() while placing carries no enter binding")
}

func TestNodePlacementStep_EnterWithInvalidFieldFocusesAndReportsErrFixHighlighted(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newPlacementConfig()
	s.phase = phasePlacing
	required := components.NewInputField("test required", "")
	required.Required = true
	s.inner = wizard.NewMultiSectionForm([]wizard.FormSection{{Title: "test", Group: components.NewInputGroup(required)}})
	s.SetFocused(true)

	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Update(enter) with an invalid field: want a cmd, got nil")
	}
	errMsg, ok := firstErrorSetMsg(cmd)
	if !ok || !errors.Is(errMsg.Error, wizard.ErrFixHighlighted) {
		t.Fatalf("Update(enter) with an invalid field = %#v, want ErrorSetMsg(ErrFixHighlighted)", cmd())
	}
	if got := s.inner.FocusedField(); got != required {
		t.Fatalf("FocusedField() after enter = %v, want the invalid field focused", got)
	}
}

func TestNodePlacementStep_ShouldShow(t *testing.T) {
	if !NewNodePlacementStep().ShouldShow(newPlacementConfig()) {
		t.Error("ShouldShow(proxmox provider) = false, want true")
	}
	if NewNodePlacementStep().ShouldShow(&config.Config{Provider: config.ProviderConfig{Type: "aws"}}) {
		t.Error("ShouldShow(non-proxmox provider) = true, want false")
	}
}

func TestNodePlacementCredentialsPreferTheTypedToken(t *testing.T) {
	t.Setenv("PROXMOX_VE_API_TOKEN", "env@pam!okdctl=env")
	t.Setenv("PROXMOX_VE_INSECURE", "")
	s := NewNodePlacementStep()
	s.cfg = newPlacementConfig()

	if creds := s.credentials(); string(creds.APIToken) != "env@pam!okdctl=env" || creds.Insecure {
		t.Fatalf("without typed secrets creds = %+v, want the environment's token", creds.Redacted())
	}

	s.withSecrets(func() wizard.Secrets {
		var sec wizard.Secrets
		sec.ProxmoxToken.Set("typed@pam!okdctl=typed")
		sec.ProxmoxInsecure = true
		return sec
	})
	creds := s.credentials()
	if string(creds.APIToken) != "typed@pam!okdctl=typed" || !creds.Insecure || creds.Endpoint != s.cfg.Provider.Proxmox.Endpoint {
		t.Fatalf("creds = %+v / %q, want the typed token, insecure and the config endpoint", creds.Redacted(), creds.APIToken)
	}
}

func TestGolden_NodePlacementUnknownCapacity(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{80, 24}, {180, 48}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDNodePlacement})
			m.Config().Provider.Proxmox.Nodes = []string{"pve1"}
			gen := m.CurrentStep().(*NodePlacementStep).generation
			m.Update(discoveryCompleteMsg{generation: gen, discovery: &proxmoxDiscovery{Nodes: []proxmoxNode{{Name: "pve1", Status: "online"}}}})
			for range 2 {
				m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
				m.Update(wizard.FocusChangedMsg{})
			}

			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("node-placement-unknown-capacity_%dx%d", sz.w, sz.h), frame)
			tuitest.AssertFits(t, frame, sz.w, sz.h)

			plain := tuitest.StripANSI(frame)
			if !strings.Contains(plain, "pve1 — ?c/?g") {
				t.Errorf("view is missing the unknown-capacity option:\n%s", plain)
			}
		})
	}
}

func TestNodePlacementStep_StaleDiscoveryRepliesCannotOverwriteNewer(t *testing.T) {
	for _, stale := range []discoveryCompleteMsg{
		{discovery: &proxmoxDiscovery{Nodes: []proxmoxNode{{Name: "stale-node"}}}},
		{err: errors.New("connection refused")},
	} {
		s := NewNodePlacementStep()
		s.cfg = newPlacementConfig()
		s.Init()
		genA := s.generation
		s.Init()
		genB := s.generation
		if genB == genA {
			t.Fatal("re-entry must issue a new generation")
		}

		discB := demoDiscovery()
		step, _ := s.Update(discoveryCompleteMsg{generation: genB, discovery: discB})
		s = step.(*NodePlacementStep)
		stale.generation = genA
		step, _ = s.Update(stale)
		s = step.(*NodePlacementStep)

		if s.discovery != discB || s.discoveryErr != nil {
			t.Fatalf("stale reply %+v overwrote the newer one: discovery=%+v err=%v", stale, s.discovery, s.discoveryErr)
		}
	}
}

func TestNodePlacementStep_ReentryAfterSuccessRebuildsWithoutRefetching(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newPlacementConfig()
	if cmd := s.Init(); cmd == nil {
		t.Fatal("first Init() must fetch")
	}
	step, _ := s.Update(discoveryCompleteMsg{generation: s.generation, discovery: demoDiscovery()})
	s = step.(*NodePlacementStep)
	gen := s.generation

	s.cfg.Workers.Count = 3
	s.Init()

	if s.generation != gen || s.phase != phasePlacing {
		t.Fatalf("re-entry after success refetched (generation %d → %d) or left phase %v", gen, s.generation, s.phase)
	}
	if _, ok := s.hostFields["lab-worker2"]; !ok {
		t.Fatalf("re-entry did not rebuild the form for the new worker: %v", s.hostFields)
	}
}

func TestNodePlacementStep_ReentryAfterErrorRefetches(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newPlacementConfig()
	s.Init()
	genA := s.generation
	step, _ := s.Update(discoveryCompleteMsg{generation: genA, err: errors.New("connection refused")})
	s = step.(*NodePlacementStep)

	if cmd := s.Init(); cmd == nil || s.generation == genA {
		t.Fatal("re-entry after an error must retry with a new generation")
	}
	step, _ = s.Update(discoveryCompleteMsg{generation: s.generation, discovery: demoDiscovery()})
	s = step.(*NodePlacementStep)
	if s.discoveryErr != nil {
		t.Fatalf("discoveryErr = %v after the retry succeeded, want nil", s.discoveryErr)
	}
}

func TestNodePlacementStep_RefreshKeyRefetchesDiscovery(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newPlacementConfig()
	s.Init()
	step, _ := s.Update(discoveryCompleteMsg{generation: s.generation, discovery: demoDiscovery()})
	s = step.(*NodePlacementStep)
	genBefore := s.generation

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil || s.generation == genBefore {
		t.Fatal("'r' while placing must re-issue discovery with a new generation")
	}
}

func TestNodePlacementStep_OverlayHelp(t *testing.T) {
	s := NewNodePlacementStep()
	if got := s.OverlayHelp(); got != nil {
		t.Fatalf("OverlayHelp() while discovering = %+v, want nil", got)
	}
	noEndpoint := NewNodePlacementStep()
	noEndpoint.cfg = &config.Config{}
	noEndpoint.Init()
	if got := noEndpoint.OverlayHelp(); got != nil {
		t.Fatalf("OverlayHelp() with no built form = %+v, want nil", got)
	}

	want := wizard.KeyBinding{Key: "r", Help: "refresh"}
	for name, msg := range map[string]discoveryCompleteMsg{"success": {discovery: demoDiscovery()}, "network error": {err: errors.New("connection refused")}} {
		s := NewNodePlacementStep()
		s.cfg = newPlacementConfig()
		step, _ := s.Update(msg)
		s = step.(*NodePlacementStep)
		if got := s.OverlayHelp(); len(got) != 1 || got[0] != want {
			t.Fatalf("OverlayHelp() once placing (%s) = %+v, want [%+v]", name, got, want)
		}
	}
}

func TestNodePlacementStep_HelpOverlayListsRefreshOnlyOncePlacing(t *testing.T) {
	for _, tc := range []struct {
		name       string
		discovered bool
	}{{"placing", true}, {"discovering", false}} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewNodePlacementStep()
			s.cfg = newPlacementConfig()
			if tc.discovered {
				step, _ := s.Update(discoveryCompleteMsg{discovery: demoDiscovery()})
				s = step.(*NodePlacementStep)
			}
			m := wizard.NewModel([]wizard.WizardStep{s}, s.cfg)
			tuitest.RenderAt(t, m, 100, 30)
			mm, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
			frame := tuitest.StripANSI(mm.(*wizard.Model).View().Content)
			if got := strings.Contains(frame, "r refresh"); got != tc.discovered {
				t.Fatalf("overlay advertises refresh = %v, want %v:\n%s", got, tc.discovered, frame)
			}
		})
	}
}

func TestNodePlacementStep_SanitizesHostileProxmoxText(t *testing.T) {
	const payload = "\x1b[2J\x1b[H"

	t.Run("node names", func(t *testing.T) {
		s := NewNodePlacementStep()
		s.cfg = newPlacementConfig()
		disc := &proxmoxDiscovery{Nodes: []proxmoxNode{{Name: "pve1" + payload, Status: "online", CPUsKnown: true, CPUs: 4, MemKnown: true, MemGB: 8}}}
		step, _ := s.Update(discoveryCompleteMsg{discovery: disc})
		s = step.(*NodePlacementStep)

		view := s.View(100, 40)
		if strings.Contains(view, payload) || !strings.Contains(view, "�") {
			t.Fatalf("rendered view carries the raw payload or no sanitization marker:\n%q", view)
		}
	})

	t.Run("discovery error", func(t *testing.T) {
		s := NewNodePlacementStep()
		s.cfg = newPlacementConfig()
		s.phase = phasePlacing
		s.discoveryErr = errors.New("dial proxmox: " + payload)

		view := s.View(100, 40)
		if strings.Contains(view, payload) || !strings.Contains(view, "�") {
			t.Fatalf("rendered view carries the raw payload or no sanitization marker:\n%q", view)
		}
	})
}

func TestReviewJumpToProxmoxKeepsThePoolNodePlacementChose(t *testing.T) {
	tui.SetTerminalWidth(120)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })
	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, 120, 40)
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}

	m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDNodePlacement})
	placement := m.CurrentStep().(*NodePlacementStep)
	m.Update(discoveryCompleteMsg{generation: placement.generation, discovery: demoDiscovery()})
	placement.poolField.SetValue("pve1,pve2")
	_, cmd := m.Update(enter)
	pumpCmd(m, cmd)
	if got := m.Config().Provider.Proxmox.Nodes; !slices.Equal(got, []string{"pve1", "pve2"}) {
		t.Fatalf("node placement did not write the pool: %v", got)
	}

	m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDProxmox})
	_, cmd = m.Update(enter)
	pumpCmd(m, cmd)

	if got := m.Config().Provider.Proxmox.Nodes; !slices.Equal(got, []string{"pve1", "pve2"}) {
		t.Fatalf("confirming the proxmox step overwrote the pool node placement chose: %v", got)
	}
}

func TestNodePlacementTypedNodesJoinThePoolWithoutDiscovery(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewNodePlacementStep()
	s.cfg = cfg
	step, _ := s.Update(discoveryCompleteMsg{err: errors.New("connection refused")})
	s = step.(*NodePlacementStep)

	if got := s.poolField.Value(); got != "pve" {
		t.Fatalf("pool without discovery = %q, want the config's own list", got)
	}
	s.addField.SetValue("pve9, pve")
	if err := s.Apply(cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Provider.Proxmox.Nodes; !slices.Equal(got, []string{"pve", "pve9"}) {
		t.Fatalf("pool after typing a node = %v, want [pve pve9]", got)
	}
}

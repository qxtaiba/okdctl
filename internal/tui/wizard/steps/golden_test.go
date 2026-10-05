package steps

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/lifecycle"
)

// assertNoScrollIndicator fails t if frame's footer shows the viewport
// scroll hint, i.e. the step's content overflowed its height.
func assertNoScrollIndicator(t *testing.T, frame string) {
	t.Helper()
	if strings.Contains(tuitest.StripANSI(frame), "scroll") {
		t.Errorf("frame shows a scroll indicator, want the hub body to fit without scrolling:\n%s", frame)
	}
}

// forceHeroColor forces tui.ColorEnabled() true for the duration of t, so
// the hub's block-letter hero renders instead of its no-color
// fallback, restoring the prior color profile on cleanup.
func forceHeroColor(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })
	t.Setenv("CLICOLOR_FORCE", "1")
	tui.SetColorProfileFor(&bytes.Buffer{})
}

// seedHubSaveSlot puts the hub on its five-verb menu with the dim save-slot
// line a loaded configuration produces.
func seedHubSaveSlot(m *wizard.Model) {
	cfg := m.Config()
	cfg.Cluster.Name = "prod-cluster"
	cfg.Distribution.Version = "4.20.1-okd-scos.7"
	cfg.Topology.ControlPlane.Count = 3
	cfg.Topology.Workers.Count = 3
	m.CurrentStep().(*WelcomeStep).SetExistingConfig(cfg, SaveSlotDeployed)
}

type configureScenario struct {
	name     string
	id       wizard.StepID
	seed     func(*wizard.Model)
	interact tea.Msg
}

func configureScenarios() []configureScenario {
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	downKey := tea.KeyPressMsg{Code: 'j', Text: "j"}
	endKey := tea.KeyPressMsg{Code: tea.KeyEnd}

	return []configureScenario{
		{name: "hub_fresh", id: wizard.StepIDWelcome},
		{
			name:     "hub_existing",
			id:       wizard.StepIDWelcome,
			seed:     seedHubSaveSlot,
			interact: downKey,
		},
		{
			// Tab expands the newest series' patch dropdown (enter now
			// confirms, honouring the footer's promise), keeping the
			// dropdown chrome pinned by this golden.
			name: "distribution",
			id:   wizard.StepIDDistribution,
			seed: func(m *wizard.Model) {
				gen := m.CurrentStep().(*DistributionStep).generation
				m.Update(versionsLoadedMsg{generation: gen, series: DemoReleaseSeries()})
			},
			interact: tabKey,
		},
		{name: "proxmox", id: wizard.StepIDProxmox, interact: tabKey},
		{name: "basics", id: wizard.StepIDBasics, interact: tabKey},
		{
			name: "node-placement",
			id:   wizard.StepIDNodePlacement,
			seed: func(m *wizard.Model) {
				gen := m.CurrentStep().(*NodePlacementStep).generation
				m.Update(discoveryCompleteMsg{generation: gen, discovery: demoDiscovery()})
			},
			interact: tabKey,
		},
		{name: "networking", id: wizard.StepIDNetworking, interact: tabKey},
		{name: "resources", id: wizard.StepIDResources, interact: tabKey},
		{name: "addons", id: wizard.StepIDAddons, interact: tabKey},
		{name: "files", id: wizard.StepIDFiles, interact: tabKey},
		{name: "advanced", id: wizard.StepIDAdvanced, interact: tabKey},
		{
			name: "review",
			id:   wizard.StepIDReview,
			seed: func(m *wizard.Model) {
				m.CurrentStep().(*ReviewStep).SetConfig(m.Config())
			},
			interact: downKey,
		},
		{
			name: "review-edited",
			id:   wizard.StepIDReview,
			seed: seedReviewChanges,
		},
		{
			// Populates node placement and enables an addon on top of the
			// default config so every reviewJumpOrder section renders,
			// pinning the full 1-8 contiguous jump legend.
			name: "review-with-8-targets",
			id:   wizard.StepIDReview,
			seed: func(m *wizard.Model) {
				cfg := m.Config()
				cfg.Provider.Proxmox.ControlPlaneNodes = []string{"pve1", "pve2", "pve3"}
				cfg.Provider.Proxmox.WorkerNodes = []string{"pve1", "pve2", "pve3"}
				flux := cfg.Addons["flux"]
				flux.Enabled = true
				cfg.Addons["flux"] = flux
				m.CurrentStep().(*ReviewStep).SetConfig(cfg)
			},
			interact: endKey,
		},
		{
			// Same as review-with-8-targets but leaves every addon disabled
			// (the default), pinning that the addons-hidden gap renumbers
			// advanced to [7] rather than leaving [8] behind it.
			name: "review-with-addons-hidden",
			id:   wizard.StepIDReview,
			seed: func(m *wizard.Model) {
				cfg := m.Config()
				cfg.Provider.Proxmox.ControlPlaneNodes = []string{"pve1", "pve2", "pve3"}
				cfg.Provider.Proxmox.WorkerNodes = []string{"pve1", "pve2", "pve3"}
				m.CurrentStep().(*ReviewStep).SetConfig(cfg)
			},
			interact: endKey,
		},
		{
			// A saved baseline identical to the current config renders no
			// config-changes block.
			name: "review-no-changes",
			id:   wizard.StepIDReview,
			seed: func(m *wizard.Model) {
				cfg := m.Config()
				step := m.CurrentStep().(*ReviewStep)
				step.SetConfig(cfg)
				step.SetSavedConfig(cfg)
			},
		},
	}
}

func seedReviewChanges(m *wizard.Model) {
	cfg := m.Config()
	baseline := *cfg
	step := m.CurrentStep().(*ReviewStep)
	step.SetConfig(&baseline)
	step.SetSavedConfig(&baseline)
	cfg.Cluster.Domain = "prod.example"
	cfg.Topology.ControlPlane.CPU++
	step.SetConfig(cfg)
}

func newGoldenModel(t *testing.T) *wizard.Model {
	m, _ := newGoldenModelWithCapacity(t)
	return m
}

func newGoldenModelWithCapacity(t *testing.T) (*wizard.Model, *WizardCapacitySnapshot) {
	t.Helper()
	builder := wizard.NewStepBuilder()
	RegisterAll(builder)
	built, err := wizard.BuildSteps(wizard.DefaultConfig(), builder)
	if err != nil {
		t.Fatal(err)
	}
	capacity, _ := built.States[wizard.StepTypeReview].(*WizardCapacitySnapshot)
	cfg := config.DefaultConfig()
	for _, step := range built.Steps {
		if ds, ok := step.(*wizard.DataDrivenStep); ok {
			ds.LoadFromConfig(cfg, false)
		}
	}
	capacity.cfg = cfg
	built.States[wizard.StepTypeResources].(*ResourcesStepState).Cfg = cfg
	return wizard.NewFlowModel(built.Steps, cfg, Chrome()), capacity
}

func demoDiscovery() *proxmoxDiscovery {
	return &proxmoxDiscovery{
		Nodes: []proxmoxNode{
			{
				Name: "pve1", Status: "online", CPUs: 32, CPUsKnown: true, MemGB: 128, MemKnown: true,
				Storage: demoNodeStorage(), StorageKnown: true, Bridges: demoNodeBridges(), BridgesKnown: true,
			},
			{
				Name: "pve2", Status: "online", CPUs: 24, CPUsKnown: true, MemGB: 96, MemKnown: true,
				Storage: demoNodeStorage(), StorageKnown: true, Bridges: demoNodeBridges(), BridgesKnown: true,
			},
		},
		Storage: []proxmoxStorage{
			{Name: "local-lvm", Content: "images,rootdir", TotalGB: 1800},
			{Name: "local", Content: "iso,backup,vztmpl", TotalGB: 200},
			{Name: "tank", Content: "images", TotalGB: 7200},
		},
		Bridges: []proxmoxBridge{
			{Name: "vmbr0", CIDR: "192.168.1.2/24"},
			{Name: "vmbr1", CIDR: "10.10.0.1/24"},
		},
		ISOs: []string{"local:iso/fedora-coreos-live.x86_64.iso"},
	}
}

// demoDiscoverySingleNode pins the single-Proxmox-host case: storage and
// bridges stay multi-option (as demoDiscovery), but Nodes has exactly one
// entry — named to match the seeded config's node, so the configured value
// stays on-list — and every per-node select (bootstrap, control plane,
// workers) resolves to exactly one option.
func demoDiscoverySingleNode() *proxmoxDiscovery {
	disc := demoDiscovery()
	disc.Nodes = []proxmoxNode{
		{
			Name: "pve", Status: "online", CPUs: 32, CPUsKnown: true, MemGB: 128, MemKnown: true,
			Storage: demoNodeStorage(), StorageKnown: true, Bridges: demoNodeBridges(), BridgesKnown: true,
		},
	}
	return disc
}

func demoNodeStorage() []proxmoxStorage {
	return []proxmoxStorage{
		{Name: "local-lvm", Content: "images,rootdir", TotalGB: 1800, TotalKnown: true},
		{Name: "local", Content: "iso,backup,vztmpl", TotalGB: 200, TotalKnown: true},
		{Name: "tank", Content: "images", TotalGB: 7200, TotalKnown: true},
	}
}

func demoNodeBridges() []proxmoxBridge {
	return []proxmoxBridge{{Name: "vmbr0", CIDR: "192.168.1.2/24"}, {Name: "vmbr1", CIDR: "10.10.0.1/24"}}
}

var goldenSizes = []struct {
	w, h int
	fits bool
}{
	{80, 24, true},
	{100, 30, true},
	{120, 40, true},
}

func TestGolden_ConfigureSteps(t *testing.T) {
	for _, sz := range goldenSizes {
		for _, sc := range configureScenarios() {
			t.Run(fmt.Sprintf("%s_%dx%d", sc.name, sz.w, sz.h), func(t *testing.T) {
				if strings.HasPrefix(sc.name, "hub") {
					forceHeroColor(t)
				}
				base := fmt.Sprintf("%s_%dx%d", sc.name, sz.w, sz.h)

				// newGoldenModel builds via wizard.NewModel, which seeds
				// its initial size from the process's real terminal
				// rather than sz; pin it so the golden is independent of
				// that.
				tui.SetTerminalWidth(sz.w)
				t.Cleanup(func() { tui.SetTerminalWidth(0) })

				m := newGoldenModel(t)
				_ = tuitest.RenderAt(t, m, sz.w, sz.h)
				m.Update(wizard.JumpToStepMsg{StepID: sc.id})
				if sc.seed != nil {
					sc.seed(m)
				}
				frame := tuitest.RenderAt(t, m, sz.w, sz.h)
				tuitest.Golden(t, base+"_initial", frame)
				if sz.fits {
					tuitest.AssertFits(t, frame, sz.w, sz.h)
				}
				if strings.HasPrefix(sc.name, "hub") && sz.w == 80 && sz.h == 24 {
					assertNoScrollIndicator(t, frame)
				}

				if sc.interact != nil {
					m.Update(sc.interact)
					frame = tuitest.RenderAt(t, m, sz.w, sz.h)
					tuitest.Golden(t, base+"_interacted", frame)
					if sz.fits {
						tuitest.AssertFits(t, frame, sz.w, sz.h)
					}
				}
			})
		}
	}
}

func TestCapacityWizardFramesFitAllTiers(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {100, 30}, {120, 40}, {150, 24}, {180, 48}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			tui.SetTerminalWidth(size.width)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })
			m, capacity := newGoldenModelWithCapacity(t)
			capacity.discovery = demoDiscovery()
			_ = tuitest.RenderAt(t, m, size.width, size.height)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDNodePlacement})
			gen := m.CurrentStep().(*NodePlacementStep).generation
			m.Update(discoveryCompleteMsg{generation: gen, discovery: demoDiscovery()})
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDNetworking})
			networkFrame := tuitest.RenderAt(t, m, size.width, size.height)
			tuitest.AssertFits(t, networkFrame, size.width, size.height)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDResources})
			resourceFrame := tuitest.RenderAt(t, m, size.width, size.height)
			tuitest.AssertFits(t, resourceFrame, size.width, size.height)
			if !strings.Contains(tuitest.StripANSI(resourceFrame), "40 vcpu · 104 gb ram · 1850 gb disk") {
				t.Fatalf("resource totals are not pinned in the %dx%d frame:\n%s", size.width, size.height, resourceFrame)
			}
		})
	}
}

// resolveCmd runs cmd and returns the single message it produces, unwrapping
// the batch the wizard's Update wraps a step's command in.
func resolveCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command, got none")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return msg
	}
	for _, inner := range batch {
		if inner == nil {
			continue
		}
		if m := inner(); m != nil {
			return m
		}
	}
	t.Fatal("batch produced no message")
	return nil
}

// TestGolden_HubReachesManageNodes drives the hub's manage-nodes verb with
// real keystrokes: the dim "opening …" notice while the flow is still being
// assembled, then the lifecycle flow's first screen with its esc-to-hub
// ribbon, then the hub again once esc is pressed.
func TestGolden_HubReachesManageNodes(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{80, 24}, {100, 30}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			forceHeroColor(t)
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			seedHubSaveSlot(m)
			hub := m.CurrentStep().(*WelcomeStep)
			hub.SetFlows(HubFlows{ManageNodes: func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
				st := &lifecycle.State{Cfg: m.Config()}
				return lifecycle.NewSteps(st, lifecycle.DemoHooks(0)), lifecycle.Chrome(), nil
			}})

			downKey := tea.KeyPressMsg{Code: 'j', Text: "j"}
			for range 2 {
				m.Update(downKey)
			}
			_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

			opening := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("hub-opening_%dx%d", sz.w, sz.h), opening)
			tuitest.AssertFits(t, opening, sz.w, sz.h)
			if !strings.Contains(tuitest.StripANSI(opening), "opening manage nodes") {
				t.Errorf("the hub must say which flow it is opening, and the notice must fit the body budget:\n%s", opening)
			}
			if sz.w == 80 && sz.h == 24 {
				assertNoScrollIndicator(t, opening)
			}

			m.Update(resolveCmd(t, cmd))

			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("manage-nodes_%dx%d", sz.w, sz.h), frame)
			tuitest.AssertFits(t, frame, sz.w, sz.h)

			plain := tuitest.StripANSI(frame)
			if !strings.Contains(plain, "esc hub") {
				t.Errorf("the lifecycle entry screen must advertise the esc round-trip:\n%s", plain)
			}

			m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
			back := tuitest.StripANSI(tuitest.RenderAt(t, m, sz.w, sz.h))
			if !strings.Contains(back, "manage nodes") || !strings.Contains(back, "prod-cluster") {
				t.Errorf("esc must return to the hub:\n%s", back)
			}
			if strings.Contains(back, "opening") {
				t.Errorf("the hub's opening notice must clear on return:\n%s", back)
			}
		})
	}
}

// TestGolden_HubWideTiers pins the blank-slate hub at wide/tall terminals:
// the launcher earns a second column instead of sitting as a lone centered
// menu in an otherwise empty frame.
func TestGolden_HubWideTiers(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{150, 24}, {180, 48}} {
		t.Run(fmt.Sprintf("blank-slate_%dx%d", sz.w, sz.h), func(t *testing.T) {
			forceHeroColor(t)
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("hub-blank-slate_%dx%d", sz.w, sz.h), frame)
			tuitest.AssertFits(t, frame, sz.w, sz.h)

			plain := tuitest.StripANSI(frame)
			for _, want := range []string{"get started", "GET STARTED", "connect", "cluster", "extras", "review"} {
				if !strings.Contains(plain, want) {
					t.Errorf("blank-slate hub at %dx%d is missing %q — it should earn its space, not sit in a void:\n%s", sz.w, sz.h, want, plain)
				}
			}
		})
	}
}

// heroBlockRows counts the frame rows carrying block-letter hero cells, which
// is heroRows normally and tui.WordmarkRows*heroScale once the hero doubles.
func heroBlockRows(frame string) int {
	rows := 0
	for _, line := range strings.Split(tuitest.StripANSI(frame), "\n") {
		if strings.Contains(line, "█") {
			rows++
		}
	}
	return rows
}

// TestGolden_HubWideTerminals pins the hub on the two wide sizes goldenSizes
// doesn't cover, 140x40 and 180x48, where the hero draws at double scale and
// the launcher centers across the frame's whole width.
func TestGolden_HubWideTerminals(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{140, 40}, {180, 48}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			forceHeroColor(t)
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			seedHubSaveSlot(m)

			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("hub-wide_%dx%d", sz.w, sz.h), frame)
			tuitest.AssertFits(t, frame, sz.w, sz.h)

			if got, want := heroBlockRows(frame), tui.WordmarkRows*heroScale; got != want {
				t.Errorf("hero occupies %d rows at %dx%d, want the double-scale %d", got, sz.w, sz.h, want)
			}
		})
	}
}

var wideTerminalScenarios = map[string]bool{"proxmox": true, "node-placement": true, "review": true, "review-edited": true, "review-with-8-targets": true}

func TestGolden_WideTerminal(t *testing.T) {
	const w, h = 180, 48

	for _, sc := range configureScenarios() {
		if !wideTerminalScenarios[sc.name] {
			continue
		}
		t.Run(fmt.Sprintf("%s_%dx%d", sc.name, w, h), func(t *testing.T) {
			base := fmt.Sprintf("%s_%dx%d", sc.name, w, h)

			tui.SetTerminalWidth(w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, w, h)
			m.Update(wizard.JumpToStepMsg{StepID: sc.id})
			if sc.seed != nil {
				sc.seed(m)
			}
			frame := tuitest.RenderAt(t, m, w, h)
			tuitest.Golden(t, base+"_initial", frame)
			tuitest.AssertFits(t, frame, w, h)

			if sc.interact != nil {
				m.Update(sc.interact)
				frame = tuitest.RenderAt(t, m, w, h)
				tuitest.Golden(t, base+"_interacted", frame)
				tuitest.AssertFits(t, frame, w, h)
			}
		})
	}
}

// TestModel_PasswordNeverLeaksIntoTheFrame drives real keystrokes typing a
// sentinel password into the proxmox password field one character at a
// time, rendering the full composed frame after every keystroke: neither
// the sentinel nor any prefix of it 3 characters or longer may ever appear
// anywhere in the output.
func TestModel_PasswordNeverLeaksIntoTheFrame(t *testing.T) {
	const w, h = 180, 48
	const sentinel = "hunter2sentinel"

	tui.SetTerminalWidth(w)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, w, h)
	m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDProxmox})

	// host is focused first; two tabs reach username, then password.
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	for i, r := range sentinel {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})

		frame := tuitest.StripANSI(tuitest.RenderAt(t, m, w, h))
		typed := sentinel[:i+1]
		if len(typed) >= 3 && strings.Contains(frame, typed) {
			t.Fatalf("typed prefix %q leaked into the frame after keystroke %d:\n%s", typed, i, frame)
		}
	}

	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, w, h))
	if strings.Contains(frame, sentinel) {
		t.Fatalf("full sentinel %q leaked into the frame:\n%s", sentinel, frame)
	}
}

// TestGolden_HelpOverlay pins the "?" help overlay open on the addons step —
// key-event-driven, matching how a real terminal session would trigger it.
// The overlay must replace only the viewport region (header/footer chrome
// stays) and fit exactly at both sizes.
func TestGolden_HelpOverlay(t *testing.T) {
	questionMark := tea.KeyPressMsg{Code: '?', Text: "?"}

	for _, sz := range []struct{ w, h int }{{80, 24}, {100, 30}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDAddons})

			m.Update(questionMark)

			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("help-overlay-addons_%dx%d", sz.w, sz.h), frame)
			tuitest.AssertFits(t, frame, sz.w, sz.h)
		})
	}
}

// TestGolden_DistributionHelpOverlay pins the "?" overlay for the
// distribution step's select phase, at the narrowest tier and a wide
// terminal: its footer-silent "r refresh" entry (the loaded phase's reuse of
// the key the error phase already binds as "r retry", see handleKeyMsg)
// appears in the overlay's screen section at both extremes.
func TestGolden_DistributionHelpOverlay(t *testing.T) {
	questionMark := tea.KeyPressMsg{Code: '?', Text: "?"}

	for _, sz := range []struct{ w, h int }{{80, 24}, {180, 48}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDDistribution})
			gen := m.CurrentStep().(*DistributionStep).generation
			m.Update(versionsLoadedMsg{generation: gen, series: DemoReleaseSeries()})

			m.Update(questionMark)

			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("distribution-help-overlay_%dx%d", sz.w, sz.h), frame)
			tuitest.AssertFits(t, frame, sz.w, sz.h)
		})
	}
}

// TestGolden_NodePlacementSingleNode pins the single-Proxmox-host case:
// the bootstrap field's per-node select has exactly one option and must
// render the bare value with no cycle arrows. Tabs past the infrastructure
// section's 6 fields (bridge, additional networks, os/data/iso storage,
// fcos iso) so the bootstrap field is focused and scrolled into view.
func TestGolden_NodePlacementSingleNode(t *testing.T) {
	tui.SetTerminalWidth(100)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, 100, 30)
	m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDNodePlacement})
	gen := m.CurrentStep().(*NodePlacementStep).generation
	m.Update(discoveryCompleteMsg{generation: gen, discovery: demoDiscoverySingleNode()})

	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	for range 6 {
		m.Update(tabKey)
		m.Update(wizard.FocusChangedMsg{})
	}

	frame := tuitest.RenderAt(t, m, 100, 30)
	tuitest.Golden(t, "node-placement-single-node_100x30", frame)
	tuitest.AssertFits(t, frame, 100, 30)
}

// TestGolden_DistributionLoadingState pins the distribution step's loading
// phase (before versionsLoadedMsg arrives): the spinner, "fetching okd
// releases", and the dim "this can take a few seconds" hint.
func TestGolden_DistributionLoadingState(t *testing.T) {
	for _, sz := range goldenSizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDDistribution})

			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("distribution-loading_%dx%d", sz.w, sz.h), frame)
			if sz.fits {
				tuitest.AssertFits(t, frame, sz.w, sz.h)
			}
		})
	}
}

// TestGolden_DistributionErrorState pins the distribution step's error
// phase: a failed release fetch renders tui.EmptyState's "no releases
// loaded — check your connection" line plus the "r retry · esc back"
// ribbon and the wrapped error detail, never the raw ✗ failure banner.
func TestGolden_DistributionErrorState(t *testing.T) {
	for _, sz := range goldenSizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDDistribution})
			gen := m.CurrentStep().(*DistributionStep).generation
			m.Update(versionsLoadedMsg{generation: gen, err: errors.New("dial tcp: connection refused")})

			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("distribution-error_%dx%d", sz.w, sz.h), frame)
			if sz.fits {
				tuitest.AssertFits(t, frame, sz.w, sz.h)
			}
		})
	}
}

// TestGolden_AddonsVaultsEditMode pins the vaults key-value field's
// edit-mode composition: two fieldBox cells joined with
// lipgloss.JoinHorizontal side by side, rather than the old string-concat
// that interleaved their rows. Tabs past the 8 fields preceding vaults
// (flux's 4, secretstore common's 3, connect host) so it's focused and
// scrolled into view, then ctrl+e enters edit mode.
func TestGolden_AddonsVaultsEditMode(t *testing.T) {
	pinSopsOnPath(t, false)
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	rightKey := tea.KeyPressMsg{Code: tea.KeyRight}
	ctrlE := tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}

	for _, sz := range goldenSizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDAddons})

			// The settings fold behind the enable toggles: tab to the secret
			// store toggle, enable it, then tab to the unfolded vaults field.
			m.Update(tabKey)
			m.Update(wizard.FocusChangedMsg{})
			m.Update(rightKey)
			for range 4 {
				m.Update(tabKey)
				m.Update(wizard.FocusChangedMsg{})
			}
			m.Update(ctrlE)

			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("addons-vaults-edit_%dx%d", sz.w, sz.h), frame)
			if sz.fits {
				tuitest.AssertFits(t, frame, sz.w, sz.h)
			}
		})
	}
}

// TestGolden_AddonsFluxWarning pins the flux section's warning block: once
// flux is enabled with no ssh deploy key present, an amber ⚠ line renders
// after the flux_path field and before the secret store (common) section
// head, wrapped to the section's width, with the frame unsliced. Toggles
// the enabled field to yes, then tabs past flux's remaining 3 fields so
// the warning and the next section head scroll into view.
func TestGolden_AddonsFluxWarning(t *testing.T) {
	if system.FileExists(system.ExpandPath("~/.ssh/flux-deploy-key")) {
		t.Skip("~/.ssh/flux-deploy-key exists on this machine, so the warning this test checks for would not fire")
	}

	rightKey := tea.KeyPressMsg{Code: tea.KeyRight}
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}

	for _, sz := range goldenSizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDAddons})

			m.Update(rightKey)
			for range 4 {
				m.Update(tabKey)
				m.Update(wizard.FocusChangedMsg{})
			}

			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("addons-flux-warning_%dx%d", sz.w, sz.h), frame)
			if sz.fits {
				tuitest.AssertFits(t, frame, sz.w, sz.h)
			}
		})
	}
}

// TestGolden_AddonsProviderAbsenceNotes pins mechanism 4's other half: once
// secret store is enabled with onepassword selected, the inactive vault and
// bitwarden provider sections each show their own "<provider> settings
// appear when selected." line rather than vanishing silently — distinct
// from the "secret store settings appear when enabled." line those same
// sections show while secret store itself is off (TestGolden_ConfigureSteps
// already pins that case). Scrolls to the bottom (G) since the two notes
// sit below onepassword's own unfolded fields, with nothing focusable to
// tab the viewport toward.
func TestGolden_AddonsProviderAbsenceNotes(t *testing.T) {
	pinSopsOnPath(t, false)
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	rightKey := tea.KeyPressMsg{Code: tea.KeyRight}
	bottomKey := tea.KeyPressMsg{Code: 'G', Text: "G"}

	for _, sz := range []struct{ w, h int }{{80, 24}, {120, 40}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDAddons})

			// flux_enabled -> secretstore_enabled (toggle on with right).
			m.Update(tabKey)
			m.Update(wizard.FocusChangedMsg{})
			m.Update(rightKey)
			m.Update(bottomKey)

			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("addons-provider-absence-notes_%dx%d", sz.w, sz.h), frame)
			tuitest.AssertFits(t, frame, sz.w, sz.h)

			plain := tuitest.StripANSI(frame)
			if !strings.Contains(plain, "vault settings appear when selected.") {
				t.Errorf("frame is missing the vault absence note:\n%s", plain)
			}
			if !strings.Contains(plain, "bitwarden settings appear when selected.") {
				t.Errorf("frame is missing the bitwarden absence note:\n%s", plain)
			}
		})
	}
}

// newGoldenModelFreshDefaults mirrors newGoldenModel but skips
// LoadFromConfig (matching OKDCTL_WIZARD_DEMO=1's real code path in
// internal/cli/wizard_setup.go), so fields still carry their raw
// FieldDefinition defaults instead of config.DefaultConfig's — needed to
// pin the still-a-default rendering (dim text plus the "default" tag).
func newGoldenModelFreshDefaults(t *testing.T) *wizard.Model {
	t.Helper()
	builder := wizard.NewStepBuilder()
	RegisterAll(builder)
	built, err := wizard.BuildSteps(wizard.DefaultConfig(), builder)
	if err != nil {
		t.Fatal(err)
	}
	return wizard.NewFlowModel(built.Steps, config.DefaultConfig(), Chrome())
}

// pumpCmd recursively runs cmd, flattening tea.BatchMsg, and delivers every
// resulting message to m.Update, so a headless test observes the state a
// running tea.Program would reach once its returned commands complete.
func pumpCmd(m *wizard.Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			pumpCmd(m, c)
		}
		return
	}
	m.Update(cmd())
}

// TestGolden_BasicsDefaultAsRealValue pins the cluster-name field's default
// rendering: "mycluster" shows dim with a "default" tag beside the box
// before any input, and typing the first character replaces it outright
// rather than appending to it.
func TestGolden_BasicsDefaultAsRealValue(t *testing.T) {
	tui.SetTerminalWidth(100)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	m := newGoldenModelFreshDefaults(t)
	_ = tuitest.RenderAt(t, m, 100, 30)
	m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDBasics})

	frame := tuitest.RenderAt(t, m, 100, 30)
	tuitest.Golden(t, "basics-default-value_100x30_initial", frame)
	tuitest.AssertFits(t, frame, 100, 30)

	m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})

	frame = tuitest.RenderAt(t, m, 100, 30)
	tuitest.Golden(t, "basics-default-value_100x30_typed", frame)
	tuitest.AssertFits(t, frame, 100, 30)
}

// TestGolden_ProxmoxEnterHighlightsInvalidFields pins the honest-validation
// story end to end: tabbing onto the empty, required password field paints
// no error (it hasn't been left yet), but pressing enter forces the whole
// form's validation, paints a red box plus field error on password, shows
// the generic status-row message, and scrolls/focuses the first invalid
// field.
// TestGolden_NetworkingMalformedCIDRShowsOneCleanError pins the fix: typing
// a malformed machine CIDR and tabbing away must show exactly one honest,
// actionable error — never a second, raw netip parse failure alongside it.
func TestGolden_NetworkingMalformedCIDRShowsOneCleanError(t *testing.T) {
	tui.SetTerminalWidth(80)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, 80, 24)
	m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDNetworking})

	m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	for range 20 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	m.Update(tea.PasteMsg{Content: "999.999.1.0/99"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(wizard.FocusChangedMsg{})

	frame := tuitest.RenderAt(t, m, 80, 24)
	tuitest.Golden(t, "networking-bad-cidr_80x24", frame)
	tuitest.AssertFits(t, frame, 80, 24)

	plain := tuitest.StripANSI(frame)
	if !strings.Contains(plain, "invalid cidr format") {
		t.Errorf("malformed CIDR view is missing the clean per-field error:\n%s", plain)
	}
	if strings.Contains(plain, "netip.ParsePrefix") || strings.Contains(plain, "ParseAddr") {
		t.Errorf("malformed CIDR view leaked a raw netip error:\n%s", plain)
	}
}

// TestGolden_NetworkingPairedFieldsVisible pins mechanism 2's two declared
// networking pairs (start_ip+interface, bastion_ip+vip) once tabbed into
// view — both sections are below the fold in the step's default-focus
// goldens, so this is the only pinned coverage proving they actually render
// as one joined row rather than two.
func TestGolden_NetworkingPairedFieldsVisible(t *testing.T) {
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}

	for _, sz := range []struct{ w, h int }{{80, 24}, {120, 40}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDNetworking})

			// machine_cidr -> gateway -> dns_servers -> pod_cidr ->
			// service_cidr -> host_prefix -> start_ip (6 tabs).
			for range 6 {
				m.Update(tabKey)
			}
			m.Update(wizard.FocusChangedMsg{})
			staticIPFrame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("networking-paired-static-ip_%dx%d", sz.w, sz.h), staticIPFrame)
			tuitest.AssertFits(t, staticIPFrame, sz.w, sz.h)
			plain := tuitest.StripANSI(staticIPFrame)
			startIdx := strings.Index(plain, "start ip")
			ifaceIdx := strings.Index(plain, "interface")
			if startIdx < 0 || ifaceIdx < 0 {
				t.Fatalf("frame is missing start ip or interface:\n%s", plain)
			}
			if startLine, ifaceLine := lineOf(plain, startIdx), lineOf(plain, ifaceIdx); startLine != ifaceLine {
				t.Errorf("start ip and interface labels are on different rows, want them paired on one row:\n%s", plain)
			}

			// interface -> bastion_ip (2 more tabs).
			for range 2 {
				m.Update(tabKey)
			}
			m.Update(wizard.FocusChangedMsg{})
			loadBalancingFrame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("networking-paired-load-balancing_%dx%d", sz.w, sz.h), loadBalancingFrame)
			tuitest.AssertFits(t, loadBalancingFrame, sz.w, sz.h)
			plain = tuitest.StripANSI(loadBalancingFrame)
			bastionIdx := strings.Index(plain, "bastion ip")
			vipIdx := strings.Index(plain, "api vip")
			if bastionIdx < 0 || vipIdx < 0 {
				t.Fatalf("frame is missing bastion ip or api vip:\n%s", plain)
			}
			if bastionLine, vipLine := lineOf(plain, bastionIdx), lineOf(plain, vipIdx); bastionLine != vipLine {
				t.Errorf("bastion ip and api vip labels are on different rows, want them paired on one row:\n%s", plain)
			}
		})
	}
}

// lineOf returns the 0-based line number byteIdx falls on within s.
func lineOf(s string, byteIdx int) int {
	return strings.Count(s[:byteIdx], "\n")
}

// TestGolden_ProxmoxAdvancedFoldExpandedStaysOpenAfterFocusLeaves pins the
// fold's sticky expand latch at the real step/golden level (beyond
// TestDataDrivenStep_CollapsibleSectionEnterStickyExpand's synthetic
// definition): tab to the toggle, enter to expand it, then tab away to
// username — the HARD CONSTRAINT's focus-forced-open case (i ==
// currentSection) no longer applies once focus leaves, so this is the
// sticky latch specifically, not just a focused section rendering in full.
func TestGolden_ProxmoxAdvancedFoldExpandedStaysOpenAfterFocusLeaves(t *testing.T) {
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	shiftTabKey := tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}
	pageDown := tea.KeyPressMsg{Code: tea.KeyPgDown}

	for _, sz := range []struct{ w, h int }{{80, 24}, {120, 40}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDProxmox})

			// host -> username -> password -> the fold toggle (3 tabs).
			for range 3 {
				m.Update(tabKey)
				m.Update(wizard.FocusChangedMsg{})
			}
			m.Update(enterKey)
			// Tab away: shift+tab back onto password, so currentSection
			// returns to "credentials" and the fold is no longer focused —
			// then page down, since the now-expanded fields sit below
			// password's own scroll position at the cramped 80x24 tier.
			m.Update(shiftTabKey)
			m.Update(wizard.FocusChangedMsg{})
			m.Update(pageDown)

			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("proxmox-advanced-fold-expanded_%dx%d", sz.w, sz.h), frame)
			tuitest.AssertFits(t, frame, sz.w, sz.h)

			plain := tuitest.StripANSI(frame)
			if !strings.Contains(plain, labelTokenID) {
				t.Errorf("frame is missing the expanded fold's token id field, want it to stay open after focus left:\n%s", plain)
			}
			if !strings.Contains(plain, "skip tls verify") {
				t.Errorf("frame is missing the expanded fold's skip tls verify field:\n%s", plain)
			}
			if strings.Contains(plain, "verification:") {
				t.Errorf("frame still shows the collapsed summary's \"verification:\" fact, want the fold expanded:\n%s", plain)
			}
		})
	}
}

// TestGolden_ProxmoxAdvancedFoldCollapsedAt80x24 pins mechanism 3's
// collapsed state at the 80x24 "compact" tier: proxmox_80x24_initial
// (TestGolden_ConfigureSteps) never scrolls far enough to show the fold
// line itself (host plus the paired username/password row alone exceed
// the 15-row viewport), so this scrolls to the bottom (G, independent of
// field focus) to pin that the collapsed summary row still renders
// correctly at the narrowest supported tier, not just at 100x30/120x40.
func TestGolden_ProxmoxAdvancedFoldCollapsedAt80x24(t *testing.T) {
	// pgdn, not the vim "G", since G/gg fall through to a focused text
	// input (typed as literal text) while pgdn scrolls regardless of
	// focus — host stays focused throughout, matching the step's real
	// initial-focus state the way TestGolden_ConfigureSteps's own
	// proxmox_80x24_initial does.
	pageDown := tea.KeyPressMsg{Code: tea.KeyPgDown}

	tui.SetTerminalWidth(80)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, 80, 24)
	m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDProxmox})
	for range 3 {
		m.Update(pageDown)
	}

	frame := tuitest.RenderAt(t, m, 80, 24)
	tuitest.Golden(t, "proxmox-advanced-fold-collapsed_80x24", frame)
	tuitest.AssertFits(t, frame, 80, 24)

	plain := tuitest.StripANSI(frame)
	if !strings.Contains(plain, "advanced") || !strings.Contains(plain, "verification: enabled") {
		t.Errorf("frame is missing the collapsed fold's summary row:\n%s", plain)
	}
	if hasExactLine(plain, labelTokenID) {
		t.Errorf("collapsed fold leaks the token id field's own label row:\n%s", plain)
	}
}

func TestGolden_ProxmoxEnterHighlightsInvalidFields(t *testing.T) {
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}

	tui.SetTerminalWidth(100)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, 100, 30)
	m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDProxmox})

	for range 2 {
		m.Update(tabKey)
		m.Update(wizard.FocusChangedMsg{})
	}

	frame := tuitest.RenderAt(t, m, 100, 30)
	tuitest.Golden(t, "proxmox-enter-invalid_100x30_before", frame)
	tuitest.AssertFits(t, frame, 100, 30)

	_, cmd := m.Update(enterKey)
	pumpCmd(m, cmd)

	frame = tuitest.RenderAt(t, m, 100, 30)
	tuitest.Golden(t, "proxmox-enter-invalid_100x30_after", frame)
	tuitest.AssertFits(t, frame, 100, 30)
}

// pinSopsOnPath fixes the sops lookup so golden frames do not depend on
// whether the machine running the test happens to have sops installed.
func pinSopsOnPath(t *testing.T, found bool) {
	t.Helper()
	prev := sopsOnPath
	sopsOnPath = func() bool { return found }
	t.Cleanup(func() { sopsOnPath = prev })
}

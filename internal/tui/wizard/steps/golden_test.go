package steps

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func assertNoScrollIndicator(t *testing.T, frame string) {
	t.Helper()
	if strings.Contains(tuitest.StripANSI(frame), "scroll") {
		t.Errorf("frame shows a scroll indicator, want the hub body to fit without scrolling:\n%s", frame)
	}
}

// forceHeroColor forces tui.ColorEnabled() true for the duration of t, so
// the hub's block-letter hero renders instead of its no-color fallback.
func forceHeroColor(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })
	t.Setenv("CLICOLOR_FORCE", "1")
	tui.SetColorProfileFor(&bytes.Buffer{})
}

func seedHubSaveSlot(m *wizard.Model) {
	cfg := m.Config()
	cfg.Cluster.Name = "prod-cluster"
	cfg.Distribution.Version = "4.20.1-okd-scos.7"
	cfg.Workers.Count = 3
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
		{name: "hub_existing", id: wizard.StepIDWelcome, seed: seedHubSaveSlot, interact: downKey},
		{
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
				m.Config().Provider.Proxmox.Nodes = []string{"pve1", "pve2"}
				gen := m.CurrentStep().(*NodePlacementStep).generation
				m.Update(discoveryCompleteMsg{generation: gen, discovery: demoDiscovery()})
			},
			interact: tabKey,
		},
		{name: "networking", id: wizard.StepIDNetworking, interact: tabKey},
		{name: "resources", id: wizard.StepIDResources, interact: tabKey},
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
		{name: "review-edited", id: wizard.StepIDReview, seed: seedReviewChanges},
		{
			name: "review-overrides",
			id:   wizard.StepIDReview,
			seed: func(m *wizard.Model) {
				cfg := m.Config()
				cfg.Provider.Proxmox.Nodes = []string{"pve1", "pve2"}
				cfg.Nodes = map[string]config.NodeOverride{"lab-master0": {Host: "pve2", MemoryMiB: new(32768)}}
				m.CurrentStep().(*ReviewStep).SetConfig(cfg)
			},
			interact: endKey,
		},
		{
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
	cfg.ControlPlane.CPU++
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
	cfg.Files.SSHPublicKey = "./id_ed25519.pub"
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
	return &proxmoxDiscovery{Nodes: []proxmoxNode{
		{Name: "pve1", Status: "online", CPUs: 32, CPUsKnown: true, MemGB: 128, MemKnown: true},
		{Name: "pve2", Status: "online", CPUs: 24, CPUsKnown: true, MemGB: 96, MemKnown: true},
	}}
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
			if !strings.Contains(tuitest.StripANSI(resourceFrame), "28 vcpu · 80 gb ram · 1600 gb disk") {
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
					t.Errorf("blank-slate hub at %dx%d is missing %q:\n%s", sz.w, sz.h, want, plain)
				}
			}
		})
	}
}

func heroBlockRows(frame string) int {
	rows := 0
	for _, line := range strings.Split(tuitest.StripANSI(frame), "\n") {
		if strings.Contains(line, "█") {
			rows++
		}
	}
	return rows
}

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

var wideTerminalScenarios = map[string]bool{"proxmox": true, "node-placement": true, "review": true, "review-edited": true, "review-overrides": true}

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

// TestModel_TokenNeverLeaksIntoTheFrame types a sentinel token into the
// proxmox api token field one character at a time: neither the sentinel nor
// any prefix of it 3 characters or longer may appear anywhere in the frame.
func TestModel_TokenNeverLeaksIntoTheFrame(t *testing.T) {
	const w, h = 180, 48
	const sentinel = "hunter2sentinel"

	tui.SetTerminalWidth(w)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, w, h)
	m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDProxmox})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	for i, r := range sentinel {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})

		frame := tuitest.StripANSI(tuitest.RenderAt(t, m, w, h))
		typed := sentinel[:i+1]
		if len(typed) >= 3 && strings.Contains(frame, typed) {
			t.Fatalf("typed prefix %q leaked into the frame after keystroke %d:\n%s", typed, i, frame)
		}
	}
	if frame := tuitest.StripANSI(tuitest.RenderAt(t, m, w, h)); strings.Contains(frame, sentinel) {
		t.Fatalf("full sentinel %q leaked into the frame:\n%s", sentinel, frame)
	}
}

func TestGolden_HelpOverlay(t *testing.T) {
	questionMark := tea.KeyPressMsg{Code: '?', Text: "?"}

	for _, sz := range []struct{ w, h int }{{80, 24}, {100, 30}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDFiles})
			m.Update(questionMark)

			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("help-overlay-files_%dx%d", sz.w, sz.h), frame)
			tuitest.AssertFits(t, frame, sz.w, sz.h)
		})
	}
}

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

// TestGolden_NodePlacementSingleNode pins the single-host case: every host
// select has exactly one option and renders the bare value with no arrows.
func TestGolden_NodePlacementSingleNode(t *testing.T) {
	tui.SetTerminalWidth(100)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, 100, 30)
	m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDNodePlacement})
	gen := m.CurrentStep().(*NodePlacementStep).generation
	m.Update(discoveryCompleteMsg{generation: gen, discovery: &proxmoxDiscovery{Nodes: []proxmoxNode{
		{Name: "pve", Status: "online", CPUs: 32, CPUsKnown: true, MemGB: 128, MemKnown: true},
	}}})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(wizard.FocusChangedMsg{})

	frame := tuitest.RenderAt(t, m, 100, 30)
	tuitest.Golden(t, "node-placement-single-node_100x30", frame)
	tuitest.AssertFits(t, frame, 100, 30)
}

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

// newGoldenModelFreshDefaults skips LoadFromConfig, matching the
// OKDCTL_WIZARD_DEMO=1 code path, so fields still carry their raw
// FieldDefinition defaults.
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

// TestGolden_NetworkingPairedVIPsVisible pins the api/ingress vip pair once
// tabbed into view: the section sits below the fold in the default-focus
// goldens, so this is the only pinned proof it renders as one joined row.
func TestGolden_NetworkingPairedVIPsVisible(t *testing.T) {
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}

	for _, sz := range []struct{ w, h int }{{80, 24}, {120, 40}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDNetworking})
			for range 3 {
				m.Update(tabKey)
			}
			m.Update(wizard.FocusChangedMsg{})

			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("networking-paired-vips_%dx%d", sz.w, sz.h), frame)
			tuitest.AssertFits(t, frame, sz.w, sz.h)
			plain := tuitest.StripANSI(frame)
			apiIdx, ingressIdx := strings.Index(plain, "api vip"), strings.Index(plain, "ingress vip")
			if apiIdx < 0 || ingressIdx < 0 {
				t.Fatalf("frame is missing api vip or ingress vip:\n%s", plain)
			}
			if lineOf(plain, apiIdx) != lineOf(plain, ingressIdx) {
				t.Errorf("api vip and ingress vip labels are on different rows, want them paired on one row:\n%s", plain)
			}
		})
	}
}

func lineOf(s string, byteIdx int) int {
	return strings.Count(s[:byteIdx], "\n")
}

// TestGolden_ProxmoxAdvancedFoldExpandedStaysOpenAfterFocusLeaves pins the
// fold's sticky expand latch: tab to the toggle, enter to expand, then tab
// away so the section is no longer focus-forced open.
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

			// endpoint, api token, skip tls verify, nodes, storage, iso
			// storage, bridge, then the fold toggle (7 tabs).
			for range 7 {
				m.Update(tabKey)
				m.Update(wizard.FocusChangedMsg{})
			}
			m.Update(enterKey)
			m.Update(shiftTabKey)
			m.Update(wizard.FocusChangedMsg{})
			m.Update(pageDown)

			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("proxmox-advanced-fold-expanded_%dx%d", sz.w, sz.h), frame)
			tuitest.AssertFits(t, frame, sz.w, sz.h)

			plain := tuitest.StripANSI(frame)
			if !hasLabelRow(plain, "vlan") || !hasLabelRow(plain, "cpu type") {
				t.Errorf("frame is missing the expanded fold's fields, want them to stay open after focus left:\n%s", plain)
			}
			if strings.Contains(plain, "vlan: untagged") {
				t.Errorf("frame still shows the collapsed summary's fact, want the fold expanded:\n%s", plain)
			}
		})
	}
}

// TestGolden_ProxmoxAdvancedFoldCollapsedAt80x24 pins the collapsed summary
// row at the compact tier, scrolled into view with pgdn while the endpoint
// keeps focus.
func TestGolden_ProxmoxAdvancedFoldCollapsedAt80x24(t *testing.T) {
	tui.SetTerminalWidth(80)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, 80, 24)
	m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDProxmox})
	for range 6 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	}

	frame := tuitest.RenderAt(t, m, 80, 24)
	tuitest.Golden(t, "proxmox-advanced-fold-collapsed_80x24", frame)
	tuitest.AssertFits(t, frame, 80, 24)

	plain := tuitest.StripANSI(frame)
	if !strings.Contains(plain, "advanced") || !strings.Contains(plain, "vlan: untagged") {
		t.Errorf("frame is missing the collapsed fold's summary row:\n%s", plain)
	}
	if hasLabelRow(plain, "vlan") || hasLabelRow(plain, "vm id base") {
		t.Errorf("collapsed fold leaks a field's own label row:\n%s", plain)
	}
}

// TestGolden_ProxmoxEnterHighlightsInvalidFields pins honest validation:
// clearing the required endpoint paints no error until enter forces the
// form's validation, which paints the red box and the status-row message.
func TestGolden_ProxmoxEnterHighlightsInvalidFields(t *testing.T) {
	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}

	tui.SetTerminalWidth(100)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, 100, 30)
	m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDProxmox})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	for range 40 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}

	frame := tuitest.RenderAt(t, m, 100, 30)
	tuitest.Golden(t, "proxmox-enter-invalid_100x30_before", frame)
	tuitest.AssertFits(t, frame, 100, 30)

	_, cmd := m.Update(enterKey)
	pumpCmd(m, cmd)

	frame = tuitest.RenderAt(t, m, 100, 30)
	tuitest.Golden(t, "proxmox-enter-invalid_100x30_after", frame)
	tuitest.AssertFits(t, frame, 100, 30)
	if plain := tuitest.StripANSI(frame); !strings.Contains(plain, "required") {
		t.Errorf("enter on an empty endpoint painted no required error:\n%s", plain)
	}
}

// hasLabelRow reports whether a framed view carries a row holding only
// label, the shape of a field's own label line as opposed to prose or a
// fold fact that merely contains the same words.
func hasLabelRow(view, label string) bool {
	return regexp.MustCompile(`(?m)^\s*│\s+` + regexp.QuoteMeta(label) + `\s+│\s*$`).MatchString(view)
}

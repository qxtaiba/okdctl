package steps

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

type configureScenario struct {
	name     string
	id       wizard.StepID
	seed     func(*wizard.Model)
	interact tea.Msg
}

func configureScenarios() []configureScenario {
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	downKey := tea.KeyPressMsg{Code: 'j', Text: "j"}
	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}

	return []configureScenario{
		{name: "welcome_fresh", id: wizard.StepIDWelcome},
		{
			name: "welcome_existing",
			id:   wizard.StepIDWelcome,
			seed: func(m *wizard.Model) {
				m.CurrentStep().(*WelcomeStep).SetConfigExists(true)
			},
			interact: downKey,
		},
		{
			name: "distribution",
			id:   wizard.StepIDDistribution,
			seed: func(m *wizard.Model) {
				m.Update(versionsLoadedMsg{series: DemoReleaseSeries()})
			},
			interact: enterKey,
		},
		{name: "proxmox", id: wizard.StepIDProxmox, interact: tabKey},
		{name: "basics", id: wizard.StepIDBasics, interact: tabKey},
		{
			name: "node-placement",
			id:   wizard.StepIDNodePlacement,
			seed: func(m *wizard.Model) {
				m.Update(discoveryCompleteMsg{discovery: demoDiscovery()})
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
	}
}

func newGoldenModel(t *testing.T) *wizard.Model {
	t.Helper()
	builder := wizard.NewStepBuilder()
	RegisterAll(builder)
	built := wizard.BuildSteps(wizard.DefaultConfig(), builder)
	cfg := config.DefaultConfig()
	for _, step := range built.Steps {
		if ds, ok := step.(*wizard.DataDrivenStep); ok {
			ds.LoadFromConfig(cfg)
		}
	}
	return wizard.NewModel(built.Steps, cfg)
}

func demoDiscovery() *proxmoxDiscovery {
	return &proxmoxDiscovery{
		Nodes: []proxmoxNode{
			{Name: "pve1", Status: "online", CPUs: 32, MemGB: 128},
			{Name: "pve2", Status: "online", CPUs: 24, MemGB: 96},
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

var goldenSizes = []struct {
	w, h int
	fits bool
}{
	// fits=false: the frame now draws at exactly 80 cols (Task 3), but several
	// steps' scroll indicator (badge width) and review/confirm's help bar
	// still overflow onto a second row — the footer's width budget doesn't
	// yet clamp/truncate (Task 6 rewrites it).
	{80, 24, false},
	{100, 30, true},
	{120, 40, true},
}

func TestGolden_ConfigureSteps(t *testing.T) {
	for _, sz := range goldenSizes {
		for _, sc := range configureScenarios() {
			t.Run(fmt.Sprintf("%s_%dx%d", sc.name, sz.w, sz.h), func(t *testing.T) {
				base := fmt.Sprintf("%s_%dx%d", sc.name, sz.w, sz.h)

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

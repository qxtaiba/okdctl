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
)

// assertNoScrollIndicator fails t if frame's footer shows the viewport
// scroll hint, i.e. the step's content overflowed its height.
func assertNoScrollIndicator(t *testing.T, frame string) {
	t.Helper()
	if strings.Contains(tuitest.StripANSI(frame), "scroll") {
		t.Errorf("frame shows a scroll indicator, want the welcome body to fit without scrolling:\n%s", frame)
	}
}

// forceHeroColor forces tui.ColorEnabled() true for the duration of t, so
// the welcome step's block-letter hero renders instead of its no-color
// fallback, restoring the prior color profile on cleanup.
func forceHeroColor(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })
	t.Setenv("CLICOLOR_FORCE", "1")
	tui.SetColorProfileFor(&bytes.Buffer{})
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
	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}
	endKey := tea.KeyPressMsg{Code: tea.KeyEnd}

	return []configureScenario{
		{name: "welcome_fresh", id: wizard.StepIDWelcome},
		{
			name: "welcome_existing",
			id:   wizard.StepIDWelcome,
			seed: func(m *wizard.Model) {
				cfg := config.DefaultConfig()
				cfg.Cluster.Name = "prod-cluster"
				cfg.Topology.Workers.Count = 3
				m.CurrentStep().(*WelcomeStep).SetExistingConfig(cfg)
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
			ds.LoadFromConfig(cfg, false)
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

// demoDiscoverySingleNode pins the single-Proxmox-host case: storage and
// bridges stay multi-option (as demoDiscovery), but Nodes has exactly one
// entry, so every per-node select (bootstrap, control plane, workers)
// resolves to exactly one option.
func demoDiscoverySingleNode() *proxmoxDiscovery {
	disc := demoDiscovery()
	disc.Nodes = []proxmoxNode{
		{Name: "pve1", Status: "online", CPUs: 32, MemGB: 128},
	}
	return disc
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
				if strings.HasPrefix(sc.name, "welcome") {
					forceHeroColor(t)
				}
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
				if strings.HasPrefix(sc.name, "welcome") && sz.w == 80 && sz.h == 24 {
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

// TestGolden_NodePlacementSingleNode pins the single-Proxmox-host case:
// the bootstrap field's per-node select has exactly one option and must
// render the bare value with no cycle arrows. Tabs past the infrastructure
// section's 6 fields (bridge, additional networks, os/data/iso storage,
// fcos iso) so the bootstrap field is focused and scrolled into view.
func TestGolden_NodePlacementSingleNode(t *testing.T) {
	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, 100, 30)
	m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDNodePlacement})
	m.Update(discoveryCompleteMsg{discovery: demoDiscoverySingleNode()})

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
			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDDistribution})
			m.Update(versionsLoadedMsg{err: errors.New("dial tcp: connection refused")})

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
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	ctrlE := tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}

	for _, sz := range goldenSizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDAddons})

			for range 8 {
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

// newGoldenModelFreshDefaults mirrors newGoldenModel but skips
// LoadFromConfig (matching OKDCTL_WIZARD_DEMO=1's real code path in
// internal/cli/wizard_setup.go), so fields still carry their raw
// FieldDefinition defaults instead of config.DefaultConfig's — needed to
// pin the still-a-default rendering (dim text plus the "default" tag).
func newGoldenModelFreshDefaults(t *testing.T) *wizard.Model {
	t.Helper()
	builder := wizard.NewStepBuilder()
	RegisterAll(builder)
	built := wizard.BuildSteps(wizard.DefaultConfig(), builder)
	return wizard.NewModel(built.Steps, config.DefaultConfig())
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
func TestGolden_ProxmoxEnterHighlightsInvalidFields(t *testing.T) {
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}

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

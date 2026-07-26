package steps

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/addon/catalog/flux"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

// reviewJumpOrder lists the steps this screen's section headers may jump
// to, in on-screen order. welcome and distribution have no dedicated
// section here and are not reachable by digit jump.
var reviewJumpOrder = []wizard.StepID{
	wizard.StepIDBasics,
	wizard.StepIDProxmox,
	wizard.StepIDNodePlacement,
	wizard.StepIDNetworking,
	wizard.StepIDResources,
	wizard.StepIDFiles,
	wizard.StepIDAddons,
	wizard.StepIDAdvanced,
}

// ReviewStep renders the final configuration review and deploy/save action
// selector.
type ReviewStep struct {
	wizard.BaseStep
	cfg         *config.Config
	action      *wizard.SingleSelect
	jumpTargets []wizard.JumpTarget
}

// NewReviewStep constructs the review wizard step.
func NewReviewStep() *ReviewStep {
	actions := []string{
		"deploy now",
		"save and exit",
	}

	action := wizard.NewSingleSelect(wizard.StepIDReview, components.NewCompactSelector(actions), "enter")
	action.OnNav = func(index, total int) tea.Cmd {
		return func() tea.Msg {
			return wizard.FocusChangedMsg{FieldIndex: index, TotalFields: total}
		}
	}

	return &ReviewStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(
			wizard.StepIDReview,
			"review & deploy",
			"review your configuration",
			"review configuration and choose action",
		),
		action: action,
	}
}

// Init returns nil; the step has no async startup work.
func (s *ReviewStep) Init() tea.Cmd {
	return nil
}

// SetConfig stores the Config to be summarized on the review screen.
func (s *ReviewStep) SetConfig(cfg *config.Config) {
	s.cfg = cfg
}

// Update handles digit-jump keys, action-selector navigation, and the enter
// confirm key.
func (s *ReviewStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		for _, t := range s.jumpTargets {
			if key.Matches(keyMsg, key.NewBinding(key.WithKeys(strconv.Itoa(t.Digit)))) {
				id := t.StepID
				return s, func() tea.Msg { return wizard.JumpToStepMsg{StepID: id} }
			}
		}
	}
	return s, s.action.Update(msg)
}

// JumpOrder returns the steps this screen's section headers may route a
// digit keypress to; see reviewJumpOrder.
func (s *ReviewStep) JumpOrder() []wizard.StepID {
	return reviewJumpOrder
}

// SetJumpTargets records the compacted digit assignments the wizard model
// computed for reviewJumpOrder; called every time this step regains focus.
func (s *ReviewStep) SetJumpTargets(targets []wizard.JumpTarget) {
	s.jumpTargets = targets
}

// sectionTitle prefixes title with "[N] " when stepID has an active jump
// digit, so the review screen's own headers double as its jump legend.
func (s *ReviewStep) sectionTitle(title string, stepID wizard.StepID) string {
	for _, t := range s.jumpTargets {
		if t.StepID == stepID {
			return fmt.Sprintf("[%d] %s", t.Digit, title)
		}
	}
	return title
}

// View renders the full configuration summary and deploy-or-save selector.
func (s *ReviewStep) View(width, height int) string {
	s.SetSize(width, height)

	if s.cfg == nil {
		return "no configuration to review"
	}

	st := wizard.NewSectionStyles(width)
	var content strings.Builder

	content.WriteString(s.renderClusterIdentity(&st))
	content.WriteString(s.renderProxmox(&st))
	content.WriteString(s.renderNodePlacement(&st))
	content.WriteString(s.renderNetworking(&st))
	content.WriteString(s.renderCompute(&st))
	content.WriteString(s.renderFilesIgnition(&st))
	content.WriteString(s.renderFeatures(&st))
	content.WriteString(s.renderAdvanced(&st))

	content.WriteString(st.ThickSeparator)
	content.WriteString("\n\n")
	content.WriteString(s.action.View())

	return content.String()
}

func (s *ReviewStep) renderClusterIdentity(st *wizard.SectionStyles) string {
	distVersion := string(s.cfg.Distribution.Type)
	if s.cfg.Distribution.Version != "" {
		distVersion = "OKD " + s.cfg.Distribution.Version
	}
	return wizard.RenderSection(st, s.sectionTitle("cluster identity", wizard.StepIDBasics), []wizard.KVEntry{
		{Label: "name", Value: s.cfg.Cluster.Name},
		{Label: "domain", Value: s.cfg.Cluster.Domain},
		{Label: "distribution", Value: distVersion},
	})
}

func (s *ReviewStep) renderProxmox(st *wizard.SectionStyles) string {
	p := s.cfg.Provider.Proxmox
	if p == nil {
		return ""
	}
	bridges := make([]string, len(p.AdditionalNetworks))
	for i, n := range p.AdditionalNetworks {
		bridges[i] = n.Bridge
	}
	addlNetworks := strings.Join(bridges, ", ")
	return wizard.RenderSection(st, s.sectionTitle("proxmox", wizard.StepIDProxmox), []wizard.KVEntry{
		{Label: "host", Value: p.Host},
		{Label: "token id", Value: p.TokenID, Skip: p.TokenID == ""},
		{Label: "bootstrap node", Value: p.Node},
		{Label: "bridge", Value: p.Bridge},
		{Label: "storage", Value: p.Storage},
		{Label: "data storage", Value: p.DataStorage, Skip: p.DataStorage == "" || p.DataStorage == p.Storage},
		{Label: "iso storage", Value: p.ISOStorage, Skip: p.ISOStorage == ""},
		{Label: "fcos iso", Value: p.FCOSIso, Skip: p.FCOSIso == ""},
		{Label: "extra networks", Value: addlNetworks, Skip: len(p.AdditionalNetworks) == 0},
	})
}

// renderNodePlacement shares renderProxmox's ShouldShow gate (the
// NodePlacementStep only exists when the Proxmox provider is selected), so
// it disappears from the jump legend under the same conditions.
func (s *ReviewStep) renderNodePlacement(st *wizard.SectionStyles) string {
	p := s.cfg.Provider.Proxmox
	if p == nil {
		return ""
	}
	return wizard.RenderSection(st, s.sectionTitle("node placement", wizard.StepIDNodePlacement), []wizard.KVEntry{
		{Label: "control plane nodes", Value: strings.Join(p.ControlPlaneNodes, ", "), Skip: len(p.ControlPlaneNodes) == 0},
		{Label: "worker nodes", Value: strings.Join(p.WorkerNodes, ", "), Skip: len(p.WorkerNodes) == 0},
	})
}

func (s *ReviewStep) renderNetworking(st *wizard.SectionStyles) string {
	net := s.cfg.Networking
	noStatic := net.StaticIP.Start == ""
	vipValue := net.Bastion.VIP
	if vipValue == "" && !noStatic {
		if derived, err := netutil.DeriveVIPFromStaticIP(net.StaticIP.Start); err == nil {
			vipValue = derived + " (auto)"
		}
	}
	return wizard.RenderSection(st, s.sectionTitle("networking", wizard.StepIDNetworking), []wizard.KVEntry{
		{Label: "machine cidr", Value: net.MachineCIDR},
		{Label: "gateway", Value: net.Gateway},
		{Label: "upstream dns", Value: strings.Join(net.DNS, ", ")},
		{Label: "bastion", Value: net.Bastion.IP},
		{Label: "api vip", Value: vipValue, Skip: vipValue == ""},
		{Label: "host prefix", Value: fmt.Sprintf("%d", net.HostPrefix), Skip: net.HostPrefix == 0},
		{Label: "pod cidr", Value: net.PodCIDR, Skip: net.PodCIDR == ""},
		{Label: "service cidr", Value: net.ServiceCIDR, Skip: net.PodCIDR == ""},
		{Label: "static ip start", Value: net.StaticIP.Start, Skip: noStatic},
		{Label: "interface", Value: net.StaticIP.Interface, Skip: noStatic},
		{Label: "netmask", Value: net.StaticIP.Netmask + " (from cidr)", Skip: noStatic},
		{Label: "vm dns", Value: net.StaticIP.DNS + " (bastion/dnsmasq)", Skip: noStatic},
	})
}

func (s *ReviewStep) renderCompute(st *wizard.SectionStyles) string {
	var b strings.Builder

	b.WriteString(st.Header.Render(s.sectionTitle("compute", wizard.StepIDResources)))
	b.WriteString("\n")
	b.WriteString(st.Separator)
	b.WriteString("\n")

	cpCPU := s.cfg.Topology.ControlPlane.CPU
	cpMem := s.cfg.Topology.ControlPlane.MemoryMB / 1024
	cpDisk := s.cfg.Topology.ControlPlane.DiskGB
	cpCount := s.cfg.Topology.ControlPlane.Count

	cpSpec := fmt.Sprintf("%d × (%d vcpu, %d GB RAM, %d GB os disk)", cpCount, cpCPU, cpMem, cpDisk)
	b.WriteString(st.KVPair(roleLabelControlPlane, cpSpec))
	b.WriteString("\n")

	if s.cfg.Topology.Workers.Count > 0 {
		wCPU := s.cfg.Topology.Workers.CPU
		wMem := s.cfg.Topology.Workers.MemoryMB / 1024
		wDisk := s.cfg.Topology.Workers.DiskGB
		wCount := s.cfg.Topology.Workers.Count

		wSpec := fmt.Sprintf("%d × (%d vcpu, %d GB RAM, %d GB os disk)", wCount, wCPU, wMem, wDisk)
		b.WriteString(st.KVPair(roleLabelWorkers, wSpec))
		b.WriteString("\n")

		if s.cfg.Disks.WorkerDataSizeGB > 0 {
			cephSpec := fmt.Sprintf("%d gb per worker (%d gb total)", s.cfg.Disks.WorkerDataSizeGB, s.cfg.Disks.WorkerDataSizeGB*wCount)
			b.WriteString(st.KVPair("worker data disk", cephSpec))
			b.WriteString("\n")
		}
	}

	if s.cfg.Disks.ControlPlaneDataSizeGB > 0 {
		cephSpec := fmt.Sprintf("%d gb per control plane node (%d gb total)", s.cfg.Disks.ControlPlaneDataSizeGB, s.cfg.Disks.ControlPlaneDataSizeGB*cpCount)
		b.WriteString(st.KVPair("control plane data disk", cephSpec))
		b.WriteString("\n")
	}

	totalCPU := cpCPU*cpCount + 4                                              // +4 for bootstrap
	totalMemGB := (s.cfg.Topology.ControlPlane.MemoryMB*cpCount + 8192) / 1024 // +8192 for bootstrap
	totalOSDiskGB := cpDisk*cpCount + 50                                       // +50 for bootstrap
	totalDataDiskGB := 0

	wCount := 0
	if s.cfg.Topology.Workers.Count > 0 {
		wCount = s.cfg.Topology.Workers.Count
		totalCPU += s.cfg.Topology.Workers.CPU * wCount
		totalMemGB += (s.cfg.Topology.Workers.MemoryMB * wCount) / 1024
		totalOSDiskGB += s.cfg.Topology.Workers.DiskGB * wCount
	}

	if s.cfg.Disks.WorkerDataSizeGB > 0 {
		totalDataDiskGB += s.cfg.Disks.WorkerDataSizeGB * wCount
	}
	if s.cfg.Disks.ControlPlaneDataSizeGB > 0 {
		totalDataDiskGB += s.cfg.Disks.ControlPlaneDataSizeGB * cpCount
	}

	b.WriteString(st.Separator)
	b.WriteString("\n")
	totalSpec := fmt.Sprintf("%d vcpu, %d gb ram, %d gb disk", totalCPU, totalMemGB, totalOSDiskGB+totalDataDiskGB)
	b.WriteString(st.KVPair("total", totalSpec))
	b.WriteString("\n")

	warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning)
	nodeCount := countUniqueNodes(s.cfg)
	perHost := ""
	if nodeCount > 1 {
		perHost = fmt.Sprintf(" across %d nodes", nodeCount)
	}
	if totalMemGB > 64 {
		b.WriteString(warnStyle.Render(fmt.Sprintf("  total ram exceeds 64 gb%s — verify your proxmox host(s) have sufficient memory", perHost)))
		b.WriteString("\n")
	}
	if totalCPU > 32 {
		b.WriteString(warnStyle.Render(fmt.Sprintf("  total vcpu exceeds 32%s — verify your proxmox host(s) have sufficient cores", perHost)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	return b.String()
}

func (s *ReviewStep) renderFilesIgnition(st *wizard.SectionStyles) string {
	var b strings.Builder

	b.WriteString(st.Header.Render(s.sectionTitle("files & ignition", wizard.StepIDFiles)))
	b.WriteString("\n")
	b.WriteString(st.Separator)
	b.WriteString("\n")

	if s.cfg.Files.PullSecret != "" {
		b.WriteString(st.Label.Render("pull secret"))
		b.WriteString(st.Check.Render(tui.IconSuccess + " "))
		b.WriteString(st.Value.Render(truncatePath(s.cfg.Files.PullSecret, 40)))
		b.WriteString("\n")
	}
	if s.cfg.Files.SSHPublicKey != "" {
		b.WriteString(st.Label.Render("ssh key"))
		b.WriteString(st.Check.Render(tui.IconSuccess + " "))
		b.WriteString(st.Value.Render(truncatePath(s.cfg.Files.SSHPublicKey, 40)))
		b.WriteString("\n")
	}

	if s.cfg.HTTPServer.IgnitionServerIP != "" {
		ignitionURL := fmt.Sprintf("https://%s:%d", s.cfg.HTTPServer.IgnitionServerIP, s.cfg.HTTPServer.Port)
		b.WriteString(st.KVPair("ignition server", ignitionURL))
		b.WriteString("\n")
		b.WriteString(st.KVPair("web root", s.cfg.HTTPServer.Root))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	return b.String()
}

func (s *ReviewStep) renderFeatures(st *wizard.SectionStyles) string {
	if s.cfg.Addons == nil {
		return ""
	}

	anyEnabled := false
	for _, ac := range s.cfg.Addons {
		if ac.Enabled {
			anyEnabled = true
			break
		}
	}
	if !anyEnabled {
		return ""
	}

	var b strings.Builder

	b.WriteString(st.Header.Render(s.sectionTitle("addons", wizard.StepIDAddons)))
	b.WriteString("\n")
	b.WriteString(st.Separator)
	b.WriteString("\n")

	for name, ac := range s.cfg.Addons {
		if !ac.Enabled {
			continue
		}
		label := name
		if detail, ok := ac.Settings["type"]; ok && detail != "" {
			label = fmt.Sprintf("%s (%s)", name, detail)
		} else if repo, ok := ac.Settings[flux.SettingRepository]; ok && repo != "" {
			label = fmt.Sprintf("%s (%s)", name, repo)
		}
		b.WriteString(st.KVPair(name, label))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	return b.String()
}

func (s *ReviewStep) renderAdvanced(st *wizard.SectionStyles) string {
	bt := s.cfg.Deployment.BootstrapTimeout
	vmid := s.cfg.Topology.VMIDBase
	timeouts := ""
	if bt > 0 {
		timeouts = fmt.Sprintf("bootstrap %dm, install %dm", bt/60, s.cfg.Deployment.InstallTimeout/60)
	}
	dep := s.cfg.Deployment
	return wizard.RenderSection(st, s.sectionTitle("advanced", wizard.StepIDAdvanced), []wizard.KVEntry{
		{Label: "vm id base", Value: fmt.Sprintf("%d", vmid), Skip: vmid <= 0},
		{Label: "timeouts", Value: timeouts, Skip: bt <= 0},
		{Label: "terraform environment", Value: dep.TerraformEnv, Skip: dep.TerraformEnv == ""},
		{Label: "auto approve", Value: valYes, Skip: !dep.AutoApprove},
	})
}

func truncatePath(path string, maxLen int) string {
	if maxLen < 4 {
		maxLen = 4
	}
	if len(path) <= maxLen {
		return path
	}
	base := filepath.Base(path)
	if len(base) >= maxLen-3 {
		return "..." + base[len(base)-(maxLen-3):]
	}
	return ".../" + base
}

func countUniqueNodes(cfg *config.Config) int {
	if cfg.Provider.Proxmox == nil {
		return 1
	}
	seen := map[string]bool{}
	if cfg.Provider.Proxmox.Node != "" {
		seen[cfg.Provider.Proxmox.Node] = true
	}
	for _, n := range cfg.Provider.Proxmox.ControlPlaneNodes {
		if n != "" {
			seen[n] = true
		}
	}
	for _, n := range cfg.Provider.Proxmox.WorkerNodes {
		if n != "" {
			seen[n] = true
		}
	}
	if len(seen) == 0 {
		return 1
	}
	return len(seen)
}

// Validate always returns nil; the review step has no editable fields.
func (s *ReviewStep) Validate() error {
	return nil
}

// Apply is a no-op; the review step does not mutate the Config.
func (s *ReviewStep) Apply(_ *config.Config) error {
	return nil
}

// ShortHelp returns the review step's help bar.
func (s *ReviewStep) ShortHelp() []wizard.KeyBinding {
	bindings := []wizard.KeyBinding{
		{Key: "↑↓", Help: "select action"},
		{Key: wizard.HelpEnter, Help: wizard.HelpConfirm},
		{Key: wizard.HelpEsc, Help: wizard.HelpBack},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
	if len(s.jumpTargets) > 0 {
		bindings = append(bindings, wizard.KeyBinding{Key: "1-9", Help: wizard.HelpJump})
	}
	return bindings
}

// SetFocused propagates focus to the action selector.
func (s *ReviewStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	s.action.SetFocused(focused)
}

// GetSelectedAction returns the action the user chose on the review screen.
func (s *ReviewStep) GetSelectedAction() wizard.Action {
	switch s.action.SelectedIndex() {
	case 0:
		return wizard.ActionDeploy
	default:
		return wizard.ActionExit
	}
}

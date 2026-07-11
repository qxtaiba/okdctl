package steps

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/addon/catalog/flux"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

// kvEntry describes one label/value line within a review section. When skip
// is true, the entry is omitted entirely (not rendered as blank).
type kvEntry struct {
	label string
	value string
	skip  bool
}

// renderSection emits a titled block of kvEntry lines. Returns "" if every
// entry is skipped — lets callers short-circuit whole sections by filtering.
func renderSection(st *sectionStyles, title string, entries []kvEntry) string {
	visible := entries[:0:0]
	for _, e := range entries {
		if !e.skip {
			visible = append(visible, e)
		}
	}
	if len(visible) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(st.header.Render(title))
	b.WriteString("\n")
	b.WriteString(st.separator)
	b.WriteString("\n")
	for _, e := range visible {
		b.WriteString(st.kvPair(e.label, e.value))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

type sectionStyles struct {
	header         lipgloss.Style
	separator      string
	thickSeparator string
	label          lipgloss.Style
	value          lipgloss.Style
	check          lipgloss.Style
}

func newSectionStyles(width int) sectionStyles {
	return sectionStyles{
		header: lipgloss.NewStyle().
			Foreground(tui.ColorCyan500).
			Bold(true),
		separator: lipgloss.NewStyle().
			Foreground(tui.ColorSlate700).
			Render(strings.Repeat("┄", width-4)),
		thickSeparator: lipgloss.NewStyle().
			Foreground(tui.ColorSlate600).
			Render(strings.Repeat("═", width-4)),
		label: lipgloss.NewStyle().
			Foreground(tui.ColorSlate400).
			Width(18),
		value: lipgloss.NewStyle().
			Foreground(tui.ColorText),
		check: lipgloss.NewStyle().
			Foreground(tui.ColorSuccess),
	}
}

func (st *sectionStyles) kvPair(label, value string) string {
	return st.label.Render(label) + st.value.Render(value)
}

// ReviewStep renders the final configuration review and deploy/save action
// selector.
type ReviewStep struct {
	wizard.BaseStep
	cfg    *config.Config
	action *singleSelect
}

// NewReviewStep constructs the review wizard step.
func NewReviewStep() *ReviewStep {
	actions := []string{
		"deploy now",
		"save and exit",
	}

	action := newSingleSelect(wizard.StepIDReview, components.NewCompactSelector(actions), "enter")
	action.onNav = func(index, total int) tea.Cmd {
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

// Update handles action-selector navigation and the enter confirm key.
func (s *ReviewStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	return s, s.action.Update(msg)
}

// View renders the full configuration summary and deploy-or-save selector.
func (s *ReviewStep) View(width, height int) string {
	s.SetSize(width, height)

	if s.cfg == nil {
		return "no configuration to review"
	}

	st := newSectionStyles(width)
	var content strings.Builder

	content.WriteString(s.renderClusterIdentity(&st))
	content.WriteString(s.renderProxmox(&st))
	content.WriteString(s.renderNetworking(&st))
	content.WriteString(s.renderCompute(&st))
	content.WriteString(s.renderFilesIgnition(&st))
	content.WriteString(s.renderFeatures(&st))
	content.WriteString(s.renderAdvanced(&st))

	content.WriteString(st.thickSeparator)
	content.WriteString("\n\n")
	content.WriteString(s.action.View())

	return content.String()
}

func (s *ReviewStep) renderClusterIdentity(st *sectionStyles) string {
	distVersion := string(s.cfg.Distribution.Type)
	if s.cfg.Distribution.Version != "" {
		distVersion = "OKD " + s.cfg.Distribution.Version
	}
	return renderSection(st, "cluster identity", []kvEntry{
		{label: "name", value: s.cfg.Cluster.Name},
		{label: "domain", value: s.cfg.Cluster.Domain},
		{label: "distribution", value: distVersion},
	})
}

func (s *ReviewStep) renderProxmox(st *sectionStyles) string {
	p := s.cfg.Provider.Proxmox
	if p == nil {
		return ""
	}
	bridges := make([]string, len(p.AdditionalNetworks))
	for i, n := range p.AdditionalNetworks {
		bridges[i] = n.Bridge
	}
	addlNetworks := strings.Join(bridges, ", ")
	return renderSection(st, "proxmox", []kvEntry{
		{label: "host", value: p.Host},
		{label: "token id", value: p.TokenID, skip: p.TokenID == ""},
		{label: "bootstrap node", value: p.Node},
		{label: "control plane nodes", value: strings.Join(p.ControlPlaneNodes, ", "), skip: len(p.ControlPlaneNodes) == 0},
		{label: "worker nodes", value: strings.Join(p.WorkerNodes, ", "), skip: len(p.WorkerNodes) == 0},
		{label: "bridge", value: p.Bridge},
		{label: "storage", value: p.Storage},
		{label: "data storage", value: p.DataStorage, skip: p.DataStorage == "" || p.DataStorage == p.Storage},
		{label: "iso storage", value: p.ISOStorage, skip: p.ISOStorage == ""},
		{label: "fcos iso", value: p.FCOSIso, skip: p.FCOSIso == ""},
		{label: "extra networks", value: addlNetworks, skip: len(p.AdditionalNetworks) == 0},
	})
}

func (s *ReviewStep) renderNetworking(st *sectionStyles) string {
	net := s.cfg.Networking
	noStatic := net.StaticIP.Start == ""
	vipValue := net.Bastion.VIP
	if vipValue == "" && !noStatic {
		if derived, err := netutil.DeriveVIPFromStaticIP(net.StaticIP.Start); err == nil {
			vipValue = derived + " (auto)"
		}
	}
	return renderSection(st, "networking", []kvEntry{
		{label: "machine cidr", value: net.MachineCIDR},
		{label: "gateway", value: net.Gateway},
		{label: "upstream dns", value: strings.Join(net.DNS, ", ")},
		{label: "bastion", value: net.Bastion.IP},
		{label: "api vip", value: vipValue, skip: vipValue == ""},
		{label: "host prefix", value: fmt.Sprintf("%d", net.HostPrefix), skip: net.HostPrefix == 0},
		{label: "pod cidr", value: net.PodCIDR, skip: net.PodCIDR == ""},
		{label: "service cidr", value: net.ServiceCIDR, skip: net.PodCIDR == ""},
		{label: "static ip start", value: net.StaticIP.Start, skip: noStatic},
		{label: "interface", value: net.StaticIP.Interface, skip: noStatic},
		{label: "netmask", value: net.StaticIP.Netmask + " (from cidr)", skip: noStatic},
		{label: "vm dns", value: net.StaticIP.DNS + " (bastion/dnsmasq)", skip: noStatic},
	})
}

func (s *ReviewStep) renderCompute(st *sectionStyles) string {
	var b strings.Builder

	b.WriteString(st.header.Render("compute"))
	b.WriteString("\n")
	b.WriteString(st.separator)
	b.WriteString("\n")

	cpCPU := s.cfg.Topology.ControlPlane.CPU
	cpMem := s.cfg.Topology.ControlPlane.MemoryMB / 1024
	cpDisk := s.cfg.Topology.ControlPlane.DiskGB
	cpCount := s.cfg.Topology.ControlPlane.Count

	cpSpec := fmt.Sprintf("%d × (%d vcpu, %d GB RAM, %d GB os disk)", cpCount, cpCPU, cpMem, cpDisk)
	b.WriteString(st.kvPair("control plane", cpSpec))
	b.WriteString("\n")

	if s.cfg.Topology.Workers.Count > 0 {
		wCPU := s.cfg.Topology.Workers.CPU
		wMem := s.cfg.Topology.Workers.MemoryMB / 1024
		wDisk := s.cfg.Topology.Workers.DiskGB
		wCount := s.cfg.Topology.Workers.Count

		wSpec := fmt.Sprintf("%d × (%d vcpu, %d GB RAM, %d GB os disk)", wCount, wCPU, wMem, wDisk)
		b.WriteString(st.kvPair("workers", wSpec))
		b.WriteString("\n")

		if s.cfg.Disks.WorkerDataSizeGB > 0 {
			cephSpec := fmt.Sprintf("%d gb per worker (%d gb total)", s.cfg.Disks.WorkerDataSizeGB, s.cfg.Disks.WorkerDataSizeGB*wCount)
			b.WriteString(st.kvPair("worker data disk", cephSpec))
			b.WriteString("\n")
		}
	}

	if s.cfg.Disks.ControlPlaneDataSizeGB > 0 {
		cephSpec := fmt.Sprintf("%d gb per control plane node (%d gb total)", s.cfg.Disks.ControlPlaneDataSizeGB, s.cfg.Disks.ControlPlaneDataSizeGB*cpCount)
		b.WriteString(st.kvPair("control plane data disk", cephSpec))
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

	b.WriteString(st.separator)
	b.WriteString("\n")
	totalSpec := fmt.Sprintf("%d vcpu, %d gb ram, %d gb disk", totalCPU, totalMemGB, totalOSDiskGB+totalDataDiskGB)
	b.WriteString(st.kvPair("total", totalSpec))
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

func (s *ReviewStep) renderFilesIgnition(st *sectionStyles) string {
	var b strings.Builder

	b.WriteString(st.header.Render("files & ignition"))
	b.WriteString("\n")
	b.WriteString(st.separator)
	b.WriteString("\n")

	if s.cfg.Files.PullSecret != "" {
		b.WriteString(st.label.Render("pull secret"))
		b.WriteString(st.check.Render("✓ "))
		b.WriteString(st.value.Render(truncatePath(s.cfg.Files.PullSecret, 40)))
		b.WriteString("\n")
	}
	if s.cfg.Files.SSHPublicKey != "" {
		b.WriteString(st.label.Render("ssh key"))
		b.WriteString(st.check.Render("✓ "))
		b.WriteString(st.value.Render(truncatePath(s.cfg.Files.SSHPublicKey, 40)))
		b.WriteString("\n")
	}

	if s.cfg.HTTPServer.IgnitionServerIP != "" {
		ignitionURL := fmt.Sprintf("https://%s:%d", s.cfg.HTTPServer.IgnitionServerIP, s.cfg.HTTPServer.Port)
		b.WriteString(st.kvPair("ignition server", ignitionURL))
		b.WriteString("\n")
		b.WriteString(st.kvPair("web root", s.cfg.HTTPServer.Root))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	return b.String()
}

func (s *ReviewStep) renderFeatures(st *sectionStyles) string {
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

	b.WriteString(st.header.Render("addons"))
	b.WriteString("\n")
	b.WriteString(st.separator)
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
		b.WriteString(st.kvPair(name, label))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	return b.String()
}

func (s *ReviewStep) renderAdvanced(st *sectionStyles) string {
	bt := s.cfg.Deployment.BootstrapTimeout
	vmid := s.cfg.Topology.VMIDBase
	timeouts := ""
	if bt > 0 {
		timeouts = fmt.Sprintf("bootstrap %dm, install %dm", bt/60, s.cfg.Deployment.InstallTimeout/60)
	}
	dep := s.cfg.Deployment
	return renderSection(st, "advanced", []kvEntry{
		{label: "vm id base", value: fmt.Sprintf("%d", vmid), skip: vmid <= 0},
		{label: "timeouts", value: timeouts, skip: bt <= 0},
		{label: "terraform environment", value: dep.TerraformEnv, skip: dep.TerraformEnv == ""},
		{label: "auto approve", value: valYes, skip: !dep.AutoApprove},
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
	return []wizard.KeyBinding{
		{Key: "↑↓", Help: "select action"},
		{Key: wizard.HelpEnter, Help: wizard.HelpConfirm},
		{Key: wizard.HelpEsc, Help: wizard.HelpBack},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
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

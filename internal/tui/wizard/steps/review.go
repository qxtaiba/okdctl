package steps

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard/components"
)

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

func (st sectionStyles) kvPair(label, value string) string {
	return st.label.Render(label) + st.value.Render(value)
}

type ReviewStep struct {
	wizard.BaseStep
	cfg            *config.Config
	actionSelector *components.CompactSelector
}

func NewReviewStep() *ReviewStep {
	actions := []string{
		"deploy now",
		"save and exit",
	}

	return &ReviewStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(
			wizard.StepIDReview,
			"review & deploy",
			"review your configuration",
			"review configuration and choose action",
		),
		actionSelector: components.NewCompactSelector(actions),
	}
}

func (s *ReviewStep) Init() tea.Cmd {
	return nil
}

func (s *ReviewStep) SetConfig(cfg *config.Config) {
	s.cfg = cfg
}

func (s *ReviewStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			return s, func() tea.Msg {
				return wizard.StepCompleteMsg{StepID: s.ID()}
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k", "down", "j"))):
			var cmd tea.Cmd
			s.actionSelector, cmd = s.actionSelector.Update(msg)
			return s, tea.Batch(cmd, s.emitFocusChanged())
		}
	}
	return s, nil
}

func (s *ReviewStep) emitFocusChanged() tea.Cmd {
	index := s.actionSelector.SelectedIndex()
	totalActions := s.actionSelector.Len()

	return func() tea.Msg {
		return wizard.FocusChangedMsg{
			FieldIndex:  index,
			TotalFields: totalActions,
		}
	}
}

func (s *ReviewStep) View(width, height int) string {
	s.SetSize(width, height)

	if s.cfg == nil {
		return "no configuration to review"
	}

	st := newSectionStyles(width)
	var content strings.Builder

	content.WriteString(s.renderClusterIdentity(st))
	content.WriteString(s.renderProxmox(st))
	content.WriteString(s.renderNetworking(st))
	content.WriteString(s.renderCompute(st))
	content.WriteString(s.renderFilesIgnition(st))
	content.WriteString(s.renderFeatures(st))
	content.WriteString(s.renderAdvanced(st))

	content.WriteString(st.thickSeparator)
	content.WriteString("\n\n")
	content.WriteString(s.actionSelector.View())

	return content.String()
}

func (s *ReviewStep) renderClusterIdentity(st sectionStyles) string {
	var b strings.Builder

	b.WriteString(st.header.Render("cluster identity"))
	b.WriteString("\n")
	b.WriteString(st.separator)
	b.WriteString("\n")

	b.WriteString(st.kvPair("name", s.cfg.Cluster.Name))
	b.WriteString("\n")
	b.WriteString(st.kvPair("domain", s.cfg.Cluster.Domain))
	b.WriteString("\n")

	distVersion := string(s.cfg.Distribution.Type)
	if s.cfg.Distribution.Version != "" {
		distVersion = "OKD " + s.cfg.Distribution.Version
	}
	b.WriteString(st.kvPair("distribution", distVersion))
	b.WriteString("\n\n")

	return b.String()
}

func (s *ReviewStep) renderProxmox(st sectionStyles) string {
	if s.cfg.Provider.Proxmox == nil {
		return ""
	}

	p := s.cfg.Provider.Proxmox
	var b strings.Builder

	b.WriteString(st.header.Render("proxmox"))
	b.WriteString("\n")
	b.WriteString(st.separator)
	b.WriteString("\n")

	b.WriteString(st.kvPair("host", p.Host))
	b.WriteString("\n")
	b.WriteString(st.kvPair("node", p.Node))
	b.WriteString("\n")
	b.WriteString(st.kvPair("bridge", p.Bridge))
	b.WriteString("\n")
	b.WriteString(st.kvPair("storage", p.Storage))
	b.WriteString("\n")
	if p.DataStorage != "" && p.DataStorage != p.Storage {
		b.WriteString(st.kvPair("data storage", p.DataStorage))
		b.WriteString("\n")
	}
	if p.ISOStorage != "" {
		b.WriteString(st.kvPair("iso storage", p.ISOStorage))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	return b.String()
}

func (s *ReviewStep) renderNetworking(st sectionStyles) string {
	var b strings.Builder

	b.WriteString(st.header.Render("networking"))
	b.WriteString("\n")
	b.WriteString(st.separator)
	b.WriteString("\n")

	b.WriteString(st.kvPair("machine cidr", s.cfg.Networking.MachineCIDR))
	b.WriteString("\n")
	b.WriteString(st.kvPair("gateway", s.cfg.Networking.Gateway))
	b.WriteString("\n")
	b.WriteString(st.kvPair("upstream dns", strings.Join(s.cfg.Networking.DNS, ", ")))
	b.WriteString("\n")
	b.WriteString(st.kvPair("bastion", s.cfg.Networking.Bastion.IP))
	b.WriteString("\n")
	if s.cfg.Networking.PodCIDR != "" {
		b.WriteString(st.kvPair("pod cidr", s.cfg.Networking.PodCIDR))
		b.WriteString("\n")
		b.WriteString(st.kvPair("service cidr", s.cfg.Networking.ServiceCIDR))
		b.WriteString("\n")
	}
	if s.cfg.Networking.StaticIP.Start != "" {
		b.WriteString(st.kvPair("static ip start", s.cfg.Networking.StaticIP.Start))
		b.WriteString("\n")
		b.WriteString(st.kvPair("interface", s.cfg.Networking.StaticIP.Interface))
		b.WriteString("\n")
		b.WriteString(st.kvPair("netmask", s.cfg.Networking.StaticIP.Netmask+" (from cidr)"))
		b.WriteString("\n")
		b.WriteString(st.kvPair("vm dns", s.cfg.Networking.Bastion.IP+" (bastion/dnsmasq)"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	return b.String()
}

func (s *ReviewStep) renderCompute(st sectionStyles) string {
	var b strings.Builder

	b.WriteString(st.header.Render("compute"))
	b.WriteString("\n")
	b.WriteString(st.separator)
	b.WriteString("\n")

	cpCPU := s.cfg.Topology.ControlPlane.CPU
	cpMem := s.cfg.Topology.ControlPlane.Memory / 1024
	cpDisk := s.cfg.Topology.ControlPlane.Disk
	cpCount := s.cfg.Topology.ControlPlane.Count

	cpSpec := fmt.Sprintf("%d × (%d vcpu, %d GB RAM, %d GB os disk)", cpCount, cpCPU, cpMem, cpDisk)
	b.WriteString(st.kvPair("control plane", cpSpec))
	b.WriteString("\n")

	if s.cfg.Topology.Workers.Count > 0 {
		wCPU := s.cfg.Topology.Workers.CPU
		wMem := s.cfg.Topology.Workers.Memory / 1024
		wDisk := s.cfg.Topology.Workers.Disk
		wCount := s.cfg.Topology.Workers.Count

		wSpec := fmt.Sprintf("%d × (%d vcpu, %d GB RAM, %d GB os disk)", wCount, wCPU, wMem, wDisk)
		b.WriteString(st.kvPair("workers", wSpec))
		b.WriteString("\n")

		if s.cfg.Disks.DataSizeGB > 0 {
			cephSpec := fmt.Sprintf("%d GB per worker (%d GB total)", s.cfg.Disks.DataSizeGB, s.cfg.Disks.DataSizeGB*wCount)
			b.WriteString(st.kvPair("ceph data disk", cephSpec))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	return b.String()
}

func (s *ReviewStep) renderFilesIgnition(st sectionStyles) string {
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
		ignitionURL := fmt.Sprintf("http://%s:%d", s.cfg.HTTPServer.IgnitionServerIP, s.cfg.HTTPServer.Port)
		b.WriteString(st.kvPair("ignition server", ignitionURL))
		b.WriteString("\n")
		b.WriteString(st.kvPair("web root", s.cfg.HTTPServer.Root))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	return b.String()
}

func (s *ReviewStep) renderFeatures(st sectionStyles) string {
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
		} else if repo, ok := ac.Settings["repository"]; ok && repo != "" {
			label = fmt.Sprintf("%s (%s)", name, repo)
		}
		b.WriteString(st.kvPair(name, label))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	return b.String()
}

func (s *ReviewStep) renderAdvanced(st sectionStyles) string {
	if s.cfg.Topology.VMIDBase <= 0 && s.cfg.Deployment.BootstrapTimeout <= 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString(st.header.Render("advanced"))
	b.WriteString("\n")
	b.WriteString(st.separator)
	b.WriteString("\n")

	if s.cfg.Topology.VMIDBase > 0 {
		b.WriteString(st.kvPair("vm id base", fmt.Sprintf("%d", s.cfg.Topology.VMIDBase)))
		b.WriteString("\n")
	}
	if s.cfg.Deployment.BootstrapTimeout > 0 {
		bootstrapMin := s.cfg.Deployment.BootstrapTimeout / 60
		installMin := s.cfg.Deployment.InstallTimeout / 60
		timeouts := fmt.Sprintf("bootstrap %dm, install %dm", bootstrapMin, installMin)
		b.WriteString(st.kvPair("timeouts", timeouts))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	return b.String()
}

func truncatePath(path string, maxLen int) string {
	if maxLen < 4 {
		maxLen = 4
	}
	if len(path) <= maxLen {
		return path
	}
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		filename := parts[len(parts)-1]
		if len(filename) >= maxLen-3 {
			truncLen := maxLen - 3
			if truncLen > len(filename) {
				truncLen = len(filename)
			}
			return "..." + filename[len(filename)-truncLen:]
		}
		remaining := maxLen - len(filename) - 4 // 4 for ".../"
		if remaining > 0 && len(parts) > 1 {
			prefix := strings.Join(parts[:len(parts)-1], "/")
			if len(prefix) > remaining {
				prefix = prefix[:remaining]
			}
			return prefix + "/.../" + filename
		}
		return ".../" + filename
	}
	if maxLen > 3 && len(path) > maxLen-3 {
		return path[:maxLen-3] + "..."
	}
	return path
}

func (s *ReviewStep) Validate() error {
	return nil
}

func (s *ReviewStep) Apply(cfg *config.Config) error {
	return nil
}

func (s *ReviewStep) ShortHelp() []wizard.KeyBinding {
	return []wizard.KeyBinding{
		{Key: "↑↓", Help: "select action"},
		{Key: "enter", Help: "confirm"},
		{Key: "esc", Help: "back"},
		{Key: "ctrl+c", Help: "quit"},
	}
}

func (s *ReviewStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	s.actionSelector.SetFocused(focused)
}

func (s *ReviewStep) GetSelectedAction() wizard.Action {
	switch s.actionSelector.SelectedIndex() {
	case 0:
		return wizard.ActionDeploy
	default:
		return wizard.ActionExit
	}
}

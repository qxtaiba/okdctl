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

// reviewJumpOrder lists jumpable steps in on-screen order; welcome and
// distribution have no section here and aren't reachable by digit.
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

// ReviewStep renders the final configuration review and deploy/save action selector.
type ReviewStep struct {
	wizard.BaseStep
	cfg         *config.Config
	actions     *components.CompactSelector
	jumpTargets []wizard.JumpTarget
}

// NewReviewStep constructs the review wizard step.
func NewReviewStep() *ReviewStep {
	return &ReviewStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(
			wizard.StepIDReview,
			"review & deploy",
			"review your configuration",
			"review configuration and choose action",
		),
		actions: components.NewCompactSelector([]string{
			"deploy now",
			"save and exit",
		}),
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

// Update handles digit-jump keys, action-selector navigation, and enter to confirm.
func (s *ReviewStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}

	for _, t := range s.visibleTargets() {
		if key.Matches(keyMsg, key.NewBinding(key.WithKeys(strconv.Itoa(t.Digit)))) {
			id := t.StepID
			return s, func() tea.Msg { return wizard.JumpToStepMsg{StepID: id} }
		}
	}

	if keyMsg.Code == tea.KeyEnter {
		return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: wizard.StepIDReview} }
	}

	var cmd tea.Cmd
	s.actions, cmd = s.actions.Update(components.ArrowsAsVertical(keyMsg))
	return s, cmd
}

// JumpOrder returns the steps a digit keypress may route to; see reviewJumpOrder.
func (s *ReviewStep) JumpOrder() []wizard.StepID {
	return reviewJumpOrder
}

// SetJumpTargets records the digit assignments computed for reviewJumpOrder;
// called each time this step regains focus.
func (s *ReviewStep) SetJumpTargets(targets []wizard.JumpTarget) {
	s.jumpTargets = targets
}

// sectionTitle prefixes title with "[N] " when stepID has a visible jump
// digit, doubling headers as the jump legend.
func (s *ReviewStep) sectionTitle(title string, stepID wizard.StepID) string {
	for _, t := range s.visibleTargets() {
		if t.StepID == stepID {
			return fmt.Sprintf("[%d] %s", t.Digit, title)
		}
	}
	return title
}

// visibleTargets filters jumpTargets down to sections that will actually
// render and renumbers the survivors 1..N in on-screen order, so a section
// hidden by its own content (no addons enabled, no node placement chosen)
// never leaves a gap in the on-screen digit legend. It falls back to the raw
// jumpTargets when no config is set, since visibility can't be evaluated.
func (s *ReviewStep) visibleTargets() []wizard.JumpTarget {
	if s.cfg == nil {
		return s.jumpTargets
	}
	visible := make([]wizard.JumpTarget, 0, len(s.jumpTargets))
	for _, t := range s.jumpTargets {
		if !s.sectionVisible(t.StepID) {
			continue
		}
		visible = append(visible, wizard.JumpTarget{StepID: t.StepID, Digit: len(visible) + 1})
	}
	return visible
}

// sectionVisible reports whether stepID's review section renders non-empty
// content for the current config, mirroring each renderX method's own
// emptiness rule.
func (s *ReviewStep) sectionVisible(stepID wizard.StepID) bool {
	switch stepID {
	case wizard.StepIDProxmox:
		return s.cfg.Provider.Proxmox != nil
	case wizard.StepIDNodePlacement:
		p := s.cfg.Provider.Proxmox
		return p != nil && (len(p.ControlPlaneNodes) > 0 || len(p.WorkerNodes) > 0)
	case wizard.StepIDAddons:
		return s.anyAddonEnabled()
	case wizard.StepIDAdvanced:
		dep := s.cfg.Deployment
		return s.cfg.Topology.VMIDBase > 0 || dep.BootstrapTimeout > 0 || dep.TerraformEnv != "" || dep.AutoApprove
	default:
		// cluster identity, networking, compute, and files & ignition
		// always emit at least one always-shown KVEntry.
		return true
	}
}

// anyAddonEnabled reports whether any configured addon is enabled.
func (s *ReviewStep) anyAddonEnabled() bool {
	for _, ac := range s.cfg.Addons {
		if ac.Enabled {
			return true
		}
	}
	return false
}

// View renders the full configuration summary; the deploy-or-save action
// selector renders separately, pinned to the footer via PinnedFooter.
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
	content.WriteString(s.renderCompute(&st, width))
	content.WriteString(s.renderFilesIgnition(&st))
	content.WriteString(s.renderFeatures(&st))
	content.WriteString(s.renderAdvanced(&st))

	return strings.TrimRight(content.String(), "\n")
}

// PinnedFooter renders the deploy/save action selector inline on the help
// row, empty while there is no configuration loaded to act on.
func (s *ReviewStep) PinnedFooter(width int) string {
	if s.cfg == nil {
		return ""
	}
	// MaxWidth (not tui.Truncate) because ViewInline is already ANSI-styled;
	// lipgloss truncates styled text ANSI-safely, a rune slice would not.
	return lipgloss.NewStyle().MaxWidth(width).Render(s.actions.ViewInline())
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

// renderNodePlacement shares renderProxmox's Proxmox-only gate, so it also
// disappears from the jump legend when absent.
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
		{Label: "service cidr", Value: net.ServiceCIDR, Skip: net.ServiceCIDR == ""},
		{Label: "static ip start", Value: net.StaticIP.Start, Skip: noStatic},
		{Label: "interface", Value: net.StaticIP.Interface, Skip: noStatic},
		{Label: "netmask", Value: net.StaticIP.Netmask + " (from cidr)", Skip: noStatic},
		{Label: "vm dns", Value: net.StaticIP.DNS + " (bastion/dnsmasq)", Skip: noStatic},
	})
}

// computeLabels lists the labels renderCompute will emit, so its caller can
// fit the section's label column before rendering.
func (s *ReviewStep) computeLabels() []string {
	labels := []string{roleLabelControlPlane, "total"}
	if s.cfg.Topology.Workers.Count > 0 {
		labels = append(labels, roleLabelWorkers)
		if s.cfg.Disks.WorkerDataSizeGB > 0 {
			labels = append(labels, "worker data disk")
		}
	}
	if s.cfg.Disks.ControlPlaneDataSizeGB > 0 {
		labels = append(labels, "control plane data disk")
	}
	return labels
}

// renderCompute renders the compute section: the fitted specs table, the
// total row beneath its own separator, and any over-capacity warnings.
func (s *ReviewStep) renderCompute(st *wizard.SectionStyles, width int) string {
	fitted := st.ForLabels(s.computeLabels()...)

	var b strings.Builder
	b.WriteString(s.renderComputeSpecs(&fitted))

	totalCPU, totalMemGB, totalOSDiskGB, totalDataDiskGB := s.computeTotals()

	b.WriteString(fitted.Separator)
	b.WriteString("\n")
	totalSpec := fmt.Sprintf("%d vcpu, %d gb ram, %d gb disk", totalCPU, totalMemGB, totalOSDiskGB+totalDataDiskGB)
	b.WriteString(fitted.KVPair("total", totalSpec))
	b.WriteString("\n")

	b.WriteString(s.renderComputeWarnings(totalCPU, totalMemGB, width))
	b.WriteString("\n")

	return b.String()
}

// renderComputeSpecs renders the compute section's header and its
// control-plane/workers/data-disk spec rows, using the label column st was
// already fitted to.
func (s *ReviewStep) renderComputeSpecs(st *wizard.SectionStyles) string {
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

	return b.String()
}

// computeTotals sums control-plane, worker, and bootstrap allocations into
// the review's total vcpu, ram, os-disk, and data-disk figures.
func (s *ReviewStep) computeTotals() (totalCPU, totalMemGB, totalOSDiskGB, totalDataDiskGB int) {
	cpCPU := s.cfg.Topology.ControlPlane.CPU
	cpDisk := s.cfg.Topology.ControlPlane.DiskGB
	cpCount := s.cfg.Topology.ControlPlane.Count

	totalCPU = cpCPU*cpCount + 4                                              // +4 for bootstrap
	totalMemGB = (s.cfg.Topology.ControlPlane.MemoryMB*cpCount + 8192) / 1024 // +8192 for bootstrap
	totalOSDiskGB = cpDisk*cpCount + 50                                       // +50 for bootstrap

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

	return totalCPU, totalMemGB, totalOSDiskGB, totalDataDiskGB
}

// renderComputeWarnings renders the wrapped, amber ⚠ lines flagging totals
// that exceed the review's ram/vcpu thresholds, or "" when neither trips.
func (s *ReviewStep) renderComputeWarnings(totalCPU, totalMemGB, width int) string {
	warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning)
	nodeCount := countUniqueNodes(s.cfg)
	perHost := ""
	if nodeCount > 1 {
		perHost = fmt.Sprintf(" across %d nodes", nodeCount)
	}

	var b strings.Builder
	if totalMemGB > 64 {
		text := fmt.Sprintf("%s total ram exceeds 64 gb%s — verify your proxmox host(s) have sufficient memory", tui.IconWarning, perHost)
		b.WriteString(warnStyle.Render(lipgloss.Wrap(text, width, "")))
		b.WriteString("\n")
	}
	if totalCPU > 32 {
		text := fmt.Sprintf("%s total vcpu exceeds 32%s — verify your proxmox host(s) have sufficient cores", tui.IconWarning, perHost)
		b.WriteString(warnStyle.Render(lipgloss.Wrap(text, width, "")))
		b.WriteString("\n")
	}

	return b.String()
}

func (s *ReviewStep) renderFilesIgnition(st *wizard.SectionStyles) string {
	var labels []string
	if s.cfg.Files.PullSecret != "" {
		labels = append(labels, "pull secret")
	}
	if s.cfg.Files.SSHPublicKey != "" {
		labels = append(labels, "ssh key")
	}
	if s.cfg.HTTPServer.IgnitionServerIP != "" {
		labels = append(labels, "ignition server", "web root")
	}
	fitted := st.ForLabels(labels...)

	var b strings.Builder

	b.WriteString(fitted.Header.Render(s.sectionTitle("files & ignition", wizard.StepIDFiles)))
	b.WriteString("\n")
	b.WriteString(fitted.Separator)
	b.WriteString("\n")

	if s.cfg.Files.PullSecret != "" {
		b.WriteString(fitted.Label.Render("pull secret"))
		b.WriteString(fitted.Check.Render(tui.IconSuccess + " "))
		b.WriteString(fitted.Value.Render(truncatePath(s.cfg.Files.PullSecret, 40)))
		b.WriteString("\n")
	}
	if s.cfg.Files.SSHPublicKey != "" {
		b.WriteString(fitted.Label.Render("ssh key"))
		b.WriteString(fitted.Check.Render(tui.IconSuccess + " "))
		b.WriteString(fitted.Value.Render(truncatePath(s.cfg.Files.SSHPublicKey, 40)))
		b.WriteString("\n")
	}

	if s.cfg.HTTPServer.IgnitionServerIP != "" {
		ignitionURL := "https://" + s.cfg.HTTPServer.IgnitionServerIP
		b.WriteString(fitted.KVPair("ignition server", ignitionURL))
		b.WriteString("\n")
		b.WriteString(fitted.KVPair("web root", s.cfg.HTTPServer.Root))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	return b.String()
}

func (s *ReviewStep) renderFeatures(st *wizard.SectionStyles) string {
	if !s.anyAddonEnabled() {
		return ""
	}

	labels := make([]string, 0, len(s.cfg.Addons))
	for name, ac := range s.cfg.Addons {
		if ac.Enabled {
			labels = append(labels, name)
		}
	}
	fitted := st.ForLabels(labels...)

	var b strings.Builder

	b.WriteString(fitted.Header.Render(s.sectionTitle("addons", wizard.StepIDAddons)))
	b.WriteString("\n")
	b.WriteString(fitted.Separator)
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
		b.WriteString(fitted.KVPair(name, label))
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
		{Key: wizard.HelpLeftRight, Help: wizard.HelpChoose},
		{Key: wizard.HelpEnter, Help: wizard.HelpConfirm},
		{Key: wizard.HelpEsc, Help: wizard.HelpBack},
	}
	if len(s.visibleTargets()) > 0 {
		bindings = append(bindings, wizard.KeyBinding{Key: "1-9", Help: wizard.HelpJump})
	}
	return append(bindings, wizard.KeyBinding{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit})
}

// SetFocused propagates focus to the action selector.
func (s *ReviewStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	s.actions.SetFocused(focused)
}

// GetSelectedAction returns the action the user chose on the review screen.
func (s *ReviewStep) GetSelectedAction() wizard.Action {
	switch s.actions.SelectedIndex() {
	case 0:
		return wizard.ActionDeploy
	default:
		return wizard.ActionExit
	}
}

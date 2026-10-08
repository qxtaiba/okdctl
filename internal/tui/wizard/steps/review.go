package steps

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/addon/catalog/flux"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/platform"
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
	saved       map[string]string
	preflight   []reviewCheck
	configPath  string
	showPreview bool
	capacity    *WizardCapacitySnapshot
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
	s.preflight = reviewPreflight(cfg, s.capacity)
}

// SetSavedConfig snapshots non-secret review values for the edit-config diff.
// SetSavedConfig snapshots cfg through config.Effective before comparing —
// loading no longer bakes resolved values (e.g. a mirrored bootstrap disk
// size) into a saved config, so the raw saved and raw current configs can
// each carry an unresolved zero that would otherwise show as a spurious
// "0 → 50" change the operator never made.
func (s *ReviewStep) SetSavedConfig(cfg *config.Config) {
	s.saved = reviewConfigSnapshot(config.Effective(cfg))
}

// SetConfigPath records the file the deploy action will read or write.
func (s *ReviewStep) SetConfigPath(path string) {
	s.configPath = path
}

// SetCapacity supplies the shared Proxmox inventory for the review preflight.
func (s *ReviewStep) SetCapacity(capacity *WizardCapacitySnapshot) {
	s.capacity = capacity
	s.preflight = reviewPreflight(s.cfg, capacity)
}

// Update handles review shortcuts, action-selector navigation, and enter to confirm.
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
	if key.Matches(keyMsg, key.NewBinding(key.WithKeys("p"))) {
		s.showPreview = !s.showPreview
		return s, nil
	}

	if keyMsg.Code == tea.KeyEnter {
		return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: wizard.StepIDReview} }
	}

	var cmd tea.Cmd
	s.actions, cmd = s.actions.Update(components.ArrowsAsVertical(keyMsg))
	return s, cmd
}

// InterceptBack closes the install-config preview before navigating away.
func (s *ReviewStep) InterceptBack() bool {
	if !s.showPreview {
		return false
	}
	s.showPreview = false
	return true
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
		proxmoxTuned := false
		if p := s.cfg.Provider.Proxmox; p != nil {
			proxmoxTuned = (p.CPUType != "" && p.CPUType != cpuTypeHost) || p.NUMAEnabled || p.HAEnabled
		}
		return s.cfg.Topology.VMIDBase > 0 || dep.BootstrapTimeout > 0 || dep.TerraformEnv != "" ||
			dep.AutoApprove || proxmoxTuned || s.cfg.Networking.NTPServer != "" || dep.BinDir != ""
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
	if s.showPreview {
		return s.renderInstallConfigPreview(width)
	}

	resolved := *s
	resolved.cfg = config.Effective(s.cfg)
	return resolved.renderSummary(width)
}

func (s *ReviewStep) renderSummary(width int) string {
	st := wizard.NewSectionStyles(width)
	var content strings.Builder

	content.WriteString(renderReviewPreflight(s.preflight, width))
	content.WriteString(s.renderConfigChanges(width))
	content.WriteString(s.renderClusterIdentity(&st))
	content.WriteString(s.renderProxmox(&st))
	content.WriteString(s.renderNodePlacement(&st))
	content.WriteString(s.renderNetworking(&st))
	content.WriteString(s.renderCompute(&st, width))
	content.WriteString(s.renderFilesIgnition(&st))
	content.WriteString(s.renderFeatures(&st))
	content.WriteString(s.renderAdvanced(&st))
	content.WriteString(s.renderDeployOutcome(width))

	return strings.TrimRight(content.String(), "\n")
}

func (s *ReviewStep) renderDeployOutcome(width int) string {
	header := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorText())
	path := s.configPath
	if path == "" {
		path = "okdctl.yaml"
	}

	lines := []string{header.Render("DEPLOY PLAN")}
	for _, row := range reviewPlanRows(s.cfg) {
		lines = append(lines, tui.Truncate(row, width))
	}
	lines = append(lines, "", header.Render("WRITES"), tui.Truncate(path, width), "", header.Render("HEADLESS"))
	lines = append(lines, tui.WrapLines(reviewHeadlessCommand(path, s.cfg.Cluster.Name), width)...)
	return strings.Join(lines, "\n") + "\n"
}

func reviewPlanRows(cfg *config.Config) []string {
	cp := cfg.Topology.ControlPlane
	workers := cfg.Topology.Workers
	bootstrapCPU, bootstrapMemory := cp.CPU, cp.MemoryMB
	if cfg.Topology.Bootstrap.CPU > 0 {
		bootstrapCPU = cfg.Topology.Bootstrap.CPU
	}
	if cfg.Topology.Bootstrap.MemoryMB > 0 {
		bootstrapMemory = cfg.Topology.Bootstrap.MemoryMB
	}
	bootstrapDisk := cp.DiskGB
	if cfg.Topology.Bootstrap.DiskGB > 0 {
		bootstrapDisk = cfg.Topology.Bootstrap.DiskGB
	}
	rows := [][]string{
		{"bootstrap", "1", strconv.Itoa(bootstrapCPU), fmt.Sprintf("%d gb", bootstrapMemory/1024), fmt.Sprintf("%d gb", bootstrapDisk)},
		{"masters", strconv.Itoa(cp.Count), strconv.Itoa(cp.CPU), fmt.Sprintf("%d gb", cp.MemoryMB/1024), fmt.Sprintf("%d gb", cp.DiskGB)},
		{"workers", strconv.Itoa(workers.Count), strconv.Itoa(workers.CPU), fmt.Sprintf("%d gb", workers.MemoryMB/1024), fmt.Sprintf("%d gb", workers.DiskGB)},
	}
	columns := []tui.Column{
		{Header: "role", MinWidth: 9, MaxWidth: 12},
		{Header: "vms", Align: tui.AlignRight, MinWidth: 3},
		{Header: "vcpu", Align: tui.AlignRight, MinWidth: 4},
		{Header: "ram", Align: tui.AlignRight, MinWidth: 4},
		{Header: "os disk", Align: tui.AlignRight, MinWidth: 7},
	}
	return tui.ColumnTable(columns, []tui.RowGroup{{Rows: rows}}, tui.TableOptions{Width: 58, Gap: 1})
}

func reviewHeadlessCommand(configPath, clusterName string) string {
	return "okdctl deploy --config " + shellQuote(configPath) + " --yes --confirm-cluster " + shellQuote(clusterName)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (s *ReviewStep) renderInstallConfigPreview(width int) string {
	hostPrefix := s.cfg.Networking.HostPrefix
	if hostPrefix == 0 {
		hostPrefix = 23
	}
	content, err := templates.RenderInstallConfig(&templates.InstallConfigData{
		ClusterName: s.cfg.Cluster.Name, BaseDomain: s.cfg.Cluster.Domain,
		MasterReplicas: s.cfg.Topology.ControlPlane.Count,
		WorkerReplicas: s.cfg.Topology.Workers.Count,
		ClusterCIDR:    s.cfg.Networking.PodCIDR, HostPrefix: hostPrefix,
		MachineCIDR: s.cfg.Networking.MachineCIDR, ServiceCIDR: s.cfg.Networking.ServiceCIDR,
		PullSecret: "[redacted]", SSHKey: "[redacted]", Architecture: platform.ClusterArch,
	})
	if err != nil {
		return "install-config preview unavailable: " + err.Error()
	}
	lines := []string{"INSTALL-CONFIG PREVIEW · secrets redacted · p or esc to return"}
	for _, source := range strings.Split(strings.TrimSuffix(content, "\n"), "\n") {
		// WrapLines rebuilds the line from its words, dropping the YAML indentation.
		indent := source[:len(source)-len(strings.TrimLeft(source, " "))]
		for _, line := range tui.WrapLines(source, width-len(indent)) {
			lines = append(lines, indent+line)
		}
	}
	return strings.Join(lines, "\n")
}

func (s *ReviewStep) renderConfigChanges(width int) string {
	keys := s.changedConfigKeys()
	if len(keys) == 0 {
		return ""
	}
	current := reviewConfigSnapshot(s.cfg)

	var b strings.Builder
	header := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorText())
	b.WriteString(header.Render(fmt.Sprintf("CONFIG CHANGES · %d", len(keys))))
	b.WriteString("\n")
	for _, key := range keys {
		row := tui.RenderFacts([]tui.FactRow{{
			Key:       reviewChangeLabel(key),
			Value:     s.saved[key] + " → " + current[key],
			Highlight: true,
		}}, &tui.FactLayout{
			Leader:     tui.FactLeaderPad,
			KeyWidth:   min(24, max(width/3, 12)),
			TotalWidth: max(width-2, 1),
			Styles:     tui.DefaultFactStyles(),
		})
		for _, line := range row {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

func (s *ReviewStep) changedConfigKeys() []string {
	if s.cfg == nil || len(s.saved) == 0 {
		return nil
	}
	current := reviewConfigSnapshot(s.cfg)
	keys := make([]string, 0, len(s.saved)+len(current))
	seen := make(map[string]struct{}, len(s.saved)+len(current))
	for key := range s.saved {
		seen[key] = struct{}{}
	}
	for key := range current {
		seen[key] = struct{}{}
	}
	for key := range seen {
		if s.saved[key] != current[key] {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func reviewConfigSnapshot(cfg *config.Config) map[string]string {
	if cfg == nil {
		return nil
	}
	values := map[string]string{
		"provider.type":                      string(cfg.Provider.Type),
		"cluster.domain":                     cfg.Cluster.Domain,
		"cluster.name":                       cfg.Cluster.Name,
		"distribution.type":                  string(cfg.Distribution.Type),
		"distribution.version":               cfg.Distribution.Version,
		"topology.vm_id_base":                strconv.Itoa(cfg.Topology.VMIDBase),
		"topology.bootstrap.count":           strconv.Itoa(cfg.Topology.Bootstrap.Count),
		"topology.bootstrap.vcpus":           strconv.Itoa(cfg.Topology.Bootstrap.CPU),
		"topology.bootstrap.memory_mb":       strconv.Itoa(cfg.Topology.Bootstrap.MemoryMB),
		"topology.bootstrap.disk_gb":         strconv.Itoa(cfg.Topology.Bootstrap.DiskGB),
		"networking.machine_cidr":            cfg.Networking.MachineCIDR,
		"networking.ntp_server":              cfg.Networking.NTPServer,
		"networking.gateway":                 cfg.Networking.Gateway,
		"networking.upstream_dns":            strings.Join(cfg.Networking.DNS, ", "),
		"networking.pod_cidr":                cfg.Networking.PodCIDR,
		"networking.service_cidr":            cfg.Networking.ServiceCIDR,
		"networking.host_prefix":             strconv.Itoa(cfg.Networking.HostPrefix),
		"networking.static_ip":               cfg.Networking.StaticIP.Start,
		"networking.static_netmask":          cfg.Networking.StaticIP.Netmask,
		"networking.static_interface":        cfg.Networking.StaticIP.Interface,
		"networking.static_dns":              cfg.Networking.StaticIP.DNS,
		"networking.bastion_ip":              cfg.Networking.Bastion.IP,
		"networking.api_vip":                 cfg.Networking.Bastion.VIP,
		"compute.control_plane.vcpus":        strconv.Itoa(cfg.Topology.ControlPlane.CPU),
		"compute.control_plane.memory_mb":    strconv.Itoa(cfg.Topology.ControlPlane.MemoryMB),
		"compute.control_plane.disk_gb":      strconv.Itoa(cfg.Topology.ControlPlane.DiskGB),
		"compute.control_plane.count":        strconv.Itoa(cfg.Topology.ControlPlane.Count),
		"compute.workers.vcpus":              strconv.Itoa(cfg.Topology.Workers.CPU),
		"compute.workers.memory_mb":          strconv.Itoa(cfg.Topology.Workers.MemoryMB),
		"compute.workers.disk_gb":            strconv.Itoa(cfg.Topology.Workers.DiskGB),
		"compute.workers.count":              strconv.Itoa(cfg.Topology.Workers.Count),
		"compute.worker_data_disk_gb":        strconv.Itoa(cfg.Disks.WorkerDataSizeGB),
		"compute.control_plane_data_disk_gb": strconv.Itoa(cfg.Disks.ControlPlaneDataSizeGB),
		"files.pull_secret_path":             cfg.Files.PullSecret,
		"files.ssh_public_key_path":          cfg.Files.SSHPublicKey,
		"files.web_root":                     cfg.HTTPServer.Root,
		"files.ignition_server_ip":           cfg.HTTPServer.IgnitionServerIP,
		"deployment.terraform_environment":   cfg.Deployment.TerraformEnv,
		"deployment.auto_approve":            strconv.FormatBool(cfg.Deployment.AutoApprove),
		"deployment.bootstrap_timeout":       strconv.Itoa(cfg.Deployment.BootstrapTimeout),
		"deployment.install_timeout":         strconv.Itoa(cfg.Deployment.InstallTimeout),
		"deployment.bin_dir":                 cfg.Deployment.BinDir,
	}
	if p := cfg.Provider.Proxmox; p != nil {
		// Node/storage/bridge names trace back to a Proxmox discovery
		// response (see node_placement.go); every other field here is
		// operator-authored connection config, so only those are sanitized
		// before this snapshot feeds the change-summary and config-changes
		// diffs below.
		values["proxmox.host"] = p.Host
		values["proxmox.bootstrap_node"] = tui.SanitizeTerminalEscapes(p.Node)
		values["proxmox.storage"] = tui.SanitizeTerminalEscapes(p.Storage)
		values["proxmox.data_storage"] = tui.SanitizeTerminalEscapes(p.DataStorage)
		values["proxmox.iso_storage"] = tui.SanitizeTerminalEscapes(p.ISOStorage)
		values["proxmox.bridge"] = tui.SanitizeTerminalEscapes(p.Bridge)
		values["proxmox.token_id"] = p.TokenID
		values["proxmox.insecure"] = strconv.FormatBool(p.Insecure)
		values["proxmox.insecure_http"] = strconv.FormatBool(p.InsecureHTTP)
		values["proxmox.cpu_type"] = p.CPUType
		values["proxmox.numa"] = strconv.FormatBool(p.NUMAEnabled)
		values["proxmox.ha_anti_affinity"] = strconv.FormatBool(p.HAEnabled)
		values["proxmox.control_plane_nodes"] = tui.SanitizeTerminalEscapes(strings.Join(p.ControlPlaneNodes, ", "))
		values["proxmox.worker_nodes"] = tui.SanitizeTerminalEscapes(strings.Join(p.WorkerNodes, ", "))
		values["proxmox.ssh_host_fingerprint"] = p.SSHHostFingerprint
		values["proxmox.require_pinned_fingerprint"] = strconv.FormatBool(p.RequirePinnedFingerprint)
		for i, network := range p.AdditionalNetworks {
			fieldPath := fmt.Sprintf("proxmox.additional_networks.%d", i+1)
			values[fieldPath] = tui.SanitizeTerminalEscapes(fmt.Sprintf("%s / %s / vlan %d", network.Bridge, network.Model, network.VLANTag))
		}
	}
	values["disks.control_plane_mon_size_gb"] = strconv.Itoa(cfg.Disks.ControlPlaneMonSizeGB)
	for name, addon := range cfg.Addons {
		values["addons."+name+".enabled"] = strconv.FormatBool(addon.Enabled)
	}
	return values
}

func reviewChangeLabel(fieldPath string) string {
	labels := map[string]string{
		"provider.type":  "provider",
		"cluster.domain": fieldDomain, "cluster.name": "cluster name",
		"distribution.type": "distribution", "distribution.version": "version",
		"topology.vm_id_base": "vm id base", "topology.bootstrap.count": "bootstrap count",
		"topology.bootstrap.vcpus": "bootstrap vcpus", "topology.bootstrap.memory_mb": "bootstrap memory mb",
		"topology.bootstrap.disk_gb": "bootstrap os disk gb",
		"networking.machine_cidr":    "machine cidr", "networking.gateway": "gateway",
		"networking.ntp_server":   "ntp server",
		"networking.upstream_dns": "upstream dns", "networking.pod_cidr": "pod cidr",
		"networking.service_cidr": "service cidr", "networking.host_prefix": "host prefix",
		"networking.static_ip": "static ip start", "networking.static_netmask": "static ip netmask",
		"networking.static_interface": "static ip interface", "networking.static_dns": "static ip dns",
		"networking.bastion_ip": "bastion ip",
		"networking.api_vip":    "api vip", "compute.control_plane.vcpus": "control plane vcpus",
		"compute.control_plane.memory_mb": "control plane memory mb",
		"compute.control_plane.disk_gb":   "control plane os disk gb",
		"compute.control_plane.count":     "control plane count", "compute.workers.vcpus": "worker vcpus",
		"compute.workers.memory_mb": "worker memory mb", "compute.workers.disk_gb": "worker os disk gb",
		"compute.workers.count": "worker count", "compute.worker_data_disk_gb": "worker data disk gb",
		"compute.control_plane_data_disk_gb": "control plane data disk gb",
		"files.pull_secret_path":             "pull secret path", "files.ssh_public_key_path": "ssh key path",
		"files.web_root": "web root", "files.ignition_server_ip": "ignition server ip",
		"deployment.terraform_environment": "terraform environment", "deployment.auto_approve": "auto approve",
		"deployment.bootstrap_timeout": "bootstrap timeout", "deployment.install_timeout": "install timeout",
		"deployment.bin_dir": "binary directory",
		"proxmox.host":       "proxmox host", "proxmox.bootstrap_node": "bootstrap node",
		"proxmox.storage": "os storage", "proxmox.data_storage": "data storage",
		"proxmox.iso_storage": "iso storage", "proxmox.bridge": "network bridge",
		"proxmox.token_id": "token id",
		"proxmox.insecure": "allow insecure tls", "proxmox.insecure_http": "allow insecure http",
		"proxmox.cpu_type": "cpu type", "proxmox.numa": "numa",
		"proxmox.ha_anti_affinity":    "ha anti-affinity",
		"proxmox.control_plane_nodes": "control plane nodes", "proxmox.worker_nodes": "worker nodes",
		"proxmox.ssh_host_fingerprint":       "ssh host fingerprint",
		"proxmox.require_pinned_fingerprint": "require pinned fingerprint",
		"disks.control_plane_mon_size_gb":    "control plane mon disk gb",
	}
	if strings.HasPrefix(fieldPath, "proxmox.additional_networks.") {
		return strings.TrimPrefix(fieldPath, "proxmox.")
	}
	if strings.HasPrefix(fieldPath, "addons.") {
		return strings.TrimSuffix(strings.TrimPrefix(fieldPath, "addons."), ".enabled") + " enabled"
	}
	if label, ok := labels[fieldPath]; ok {
		return label
	}
	// An unmapped path falls back to itself rather than "", so a new config
	// field surfaces here visibly instead of vanishing from the diff.
	return fieldPath
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
	addlNetworks := tui.SanitizeTerminalEscapes(strings.Join(bridges, ", "))
	return wizard.RenderSection(st, s.sectionTitle("proxmox", wizard.StepIDProxmox), []wizard.KVEntry{
		{Label: "host", Value: p.Host},
		{Label: "token id", Value: p.TokenID, Skip: p.TokenID == ""},
		{Label: "bootstrap node", Value: tui.SanitizeTerminalEscapes(p.Node)},
		{Label: "bridge", Value: tui.SanitizeTerminalEscapes(p.Bridge)},
		{Label: "storage", Value: tui.SanitizeTerminalEscapes(p.Storage)},
		{Label: "data storage", Value: tui.SanitizeTerminalEscapes(p.DataStorage), Skip: p.DataStorage == "" || p.DataStorage == p.Storage},
		{Label: "iso storage", Value: tui.SanitizeTerminalEscapes(p.ISOStorage), Skip: p.ISOStorage == ""},
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
		{Label: "control plane nodes", Value: tui.SanitizeTerminalEscapes(strings.Join(p.ControlPlaneNodes, ", ")), Skip: len(p.ControlPlaneNodes) == 0},
		{Label: "worker nodes", Value: tui.SanitizeTerminalEscapes(strings.Join(p.WorkerNodes, ", ")), Skip: len(p.WorkerNodes) == 0},
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

	totals := ComputeEffectiveResourceTotals(&s.cfg.Topology, &s.cfg.Disks)
	totalMemGB := totals.MemoryMB / 1024

	b.WriteString(fitted.Separator)
	b.WriteString("\n")
	totalSpec := fmt.Sprintf("%d vcpu, %d gb ram, %d gb disk", totals.CPU, totalMemGB, totals.OSDiskGB+totals.DataDiskGB)
	b.WriteString(fitted.KVPair("total", totalSpec))
	b.WriteString("\n")

	b.WriteString(s.renderComputeWarnings(totals.CPU, totalMemGB, width))
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

// renderComputeWarnings renders the wrapped, amber ⚠ lines flagging totals
// that exceed the review's known online Proxmox capacity, or "" when
// capacity is unknown or the totals fit.
func (s *ReviewStep) renderComputeWarnings(totalCPU, totalMemGB, width int) string {
	warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning())
	nodeCount := countUniqueNodes(s.cfg)
	perHost := ""
	if nodeCount > 1 {
		perHost = fmt.Sprintf(" across %d nodes", nodeCount)
	}

	capacity := s.capacity.OnlineTotals()
	var b strings.Builder
	if capacity.MemoryKnown && totalMemGB > capacity.MemoryGB {
		text := fmt.Sprintf("%s total ram exceeds online capacity (%d gb)%s — verify your proxmox host(s) have sufficient memory", tui.IconWarning, capacity.MemoryGB, perHost)
		b.WriteString(warnStyle.Render(lipgloss.Wrap(text, width, "")))
		b.WriteString("\n")
	}
	if capacity.CPUsKnown && totalCPU > capacity.CPUs {
		text := fmt.Sprintf("%s total vcpu exceeds online capacity (%d)%s — verify your proxmox host(s) have sufficient cores", tui.IconWarning, capacity.CPUs, perHost)
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

// renderFeatures renders one row per enabled addon in sorted (deterministic)
// name order, valued by its most telling setting — provider type, then
// repository — or a plain "enabled" when it carries no detail.
func (s *ReviewStep) renderFeatures(st *wizard.SectionStyles) string {
	if !s.anyAddonEnabled() {
		return ""
	}

	names := make([]string, 0, len(s.cfg.Addons))
	for name, ac := range s.cfg.Addons {
		if ac.Enabled {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	fitted := st.ForLabels(names...)

	var b strings.Builder

	b.WriteString(fitted.Header.Render(s.sectionTitle("addons", wizard.StepIDAddons)))
	b.WriteString("\n")
	b.WriteString(fitted.Separator)
	b.WriteString("\n")

	for _, name := range names {
		ac := s.cfg.Addons[name]
		value := valEnabled
		if detail, ok := ac.Settings["type"]; ok && detail != "" {
			value = detail
		} else if repo, ok := ac.Settings[flux.SettingRepository]; ok && repo != "" {
			value = repo
		}
		b.WriteString(fitted.KVPair(name, value))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	return b.String()
}

// renderAdvanced renders every non-default advanced setting the wizard
// applies, so nothing changing deploy behavior is invisible at the gate.
func (s *ReviewStep) renderAdvanced(st *wizard.SectionStyles) string {
	bt := s.cfg.Deployment.BootstrapTimeout
	vmid := s.cfg.Topology.VMIDBase
	timeouts := ""
	if bt > 0 {
		timeouts = fmt.Sprintf("bootstrap %dm, install %dm", bt/60, s.cfg.Deployment.InstallTimeout/60)
	}
	dep := s.cfg.Deployment
	cpuType, numa, ha := "", false, false
	if p := s.cfg.Provider.Proxmox; p != nil {
		cpuType, numa, ha = p.CPUType, p.NUMAEnabled, p.HAEnabled
	}
	ntp := s.cfg.Networking.NTPServer
	return wizard.RenderSection(st, s.sectionTitle("advanced", wizard.StepIDAdvanced), []wizard.KVEntry{
		{Label: "vm id base", Value: fmt.Sprintf("%d", vmid), Skip: vmid <= 0},
		{Label: "cpu type", Value: cpuType, Skip: cpuType == "" || cpuType == cpuTypeHost},
		{Label: "numa", Value: valYes, Skip: !numa},
		{Label: "ha anti-affinity", Value: valYes, Skip: !ha},
		{Label: "ntp server", Value: ntp, Skip: ntp == ""},
		{Label: "timeouts", Value: timeouts, Skip: bt <= 0},
		{Label: "terraform environment", Value: dep.TerraformEnv, Skip: dep.TerraformEnv == ""},
		{Label: "auto approve", Value: valYes, Skip: !dep.AutoApprove},
		{Label: "bin dir", Value: dep.BinDir, Skip: dep.BinDir == ""},
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
	if s.showPreview {
		return []wizard.KeyBinding{
			{Key: "p/esc", Help: "close preview"},
			{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
		}
	}
	bindings := []wizard.KeyBinding{
		{Key: "p", Help: "preview install config"},
		{Key: wizard.HelpLeftRight, Help: wizard.HelpChoose},
		{Key: wizard.HelpEnter, Help: wizard.HelpConfirm},
		{Key: wizard.HelpEsc, Help: wizard.HelpBack},
	}
	if len(s.visibleTargets()) > 0 {
		bindings = append(bindings, wizard.KeyBinding{Key: "1-9", Help: wizard.HelpJump})
	}
	return append(bindings, wizard.KeyBinding{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit})
}

// SetFocused refreshes local preflight checks when the review screen opens.
func (s *ReviewStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	s.actions.SetFocused(focused)
	if focused {
		s.preflight = reviewPreflight(s.cfg, s.capacity)
	}
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

// FocusBounds keeps review actions visible beneath the configuration summary.
func (s *ReviewStep) FocusBounds(width, height int) (top, bottom int, ok bool) {
	if s.cfg == nil {
		return 0, 0, false
	}
	bottom = lipgloss.Height(lipgloss.NewStyle().Width(width).Render(s.View(width, height)))
	return bottom - lipgloss.Height(s.actions.View()), bottom, true
}

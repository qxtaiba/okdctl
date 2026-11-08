package steps

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/model"
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
	wizard.StepIDAdvanced,
}

// ReviewStep renders the configuration summary and the deploy-or-save selector.
type ReviewStep struct {
	wizard.BaseStep
	cfg         *config.Config
	saved       map[string]string
	preflight   []reviewCheck
	configPath  string
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
		actions: components.NewCompactSelector([]string{"deploy now", "save and exit"}),
	}
}

// Init returns nil; the step has no async startup work.
func (s *ReviewStep) Init() tea.Cmd {
	return nil
}

// SetConfig stores the Config summarized on the review screen.
func (s *ReviewStep) SetConfig(cfg *config.Config) {
	s.cfg = cfg
	s.preflight = reviewPreflight(cfg, s.capacity)
}

// SetSavedConfig snapshots the saved config for the config-changes diff.
func (s *ReviewStep) SetSavedConfig(cfg *config.Config) {
	s.saved = reviewConfigSnapshot(cfg)
}

// SetConfigPath records the file the save action writes.
func (s *ReviewStep) SetConfigPath(path string) {
	s.configPath = path
}

// SetCapacity supplies the shared Proxmox inventory for the review preflight.
func (s *ReviewStep) SetCapacity(capacity *WizardCapacitySnapshot) {
	s.capacity = capacity
	s.preflight = reviewPreflight(s.cfg, capacity)
}

// Update handles jump digits, action-selector navigation and enter to confirm.
func (s *ReviewStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	for _, t := range s.jumpTargets {
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

// JumpOrder returns the steps a digit keypress may route to.
func (s *ReviewStep) JumpOrder() []wizard.StepID {
	return reviewJumpOrder
}

// SetJumpTargets records the digit assignments; called on every focus.
func (s *ReviewStep) SetJumpTargets(targets []wizard.JumpTarget) {
	s.jumpTargets = targets
}

// sectionTitle prefixes title with "[N] " when stepID has a jump digit.
func (s *ReviewStep) sectionTitle(title string, stepID wizard.StepID) string {
	for _, t := range s.jumpTargets {
		if t.StepID == stepID {
			return fmt.Sprintf("[%d] %s", t.Digit, title)
		}
	}
	return title
}

// View renders the full configuration summary; the action selector renders
// separately through PinnedFooter.
func (s *ReviewStep) View(width, height int) string {
	s.SetSize(width, height)
	if s.cfg == nil {
		return "no configuration to review"
	}
	st := wizard.NewSectionStyles(width)
	var b strings.Builder
	b.WriteString(renderReviewPreflight(s.preflight, width))
	b.WriteString(s.renderConfigChanges(width))
	b.WriteString(s.renderClusterIdentity(&st))
	b.WriteString(s.renderProxmox(&st))
	b.WriteString(s.renderNodes(&st, width))
	b.WriteString(s.renderNetworking(&st))
	b.WriteString(s.renderCompute(&st, width))
	b.WriteString(s.renderFiles(&st))
	b.WriteString(s.renderAdvanced(&st))
	b.WriteString(s.renderWrites(width))
	return strings.TrimRight(b.String(), "\n")
}

func (s *ReviewStep) renderWrites(width int) string {
	path := s.configPath
	if path == "" {
		path = "okdctl.yaml"
	}
	header := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorText())
	return header.Render("WRITES") + "\n" + tui.Truncate(path, width) + "\n"
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
	for _, k := range keys {
		row := tui.RenderFacts([]tui.FactRow{{
			Key:       k,
			Value:     s.saved[k] + " → " + current[k],
			Highlight: true,
		}}, &tui.FactLayout{
			Leader:     tui.FactLeaderPad,
			KeyWidth:   min(36, max(width/3, 12)),
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
	seen := make(map[string]struct{}, len(current))
	for k := range s.saved {
		seen[k] = struct{}{}
	}
	for k := range current {
		seen[k] = struct{}{}
	}
	var keys []string
	for k := range seen {
		if s.saved[k] != current[k] {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// reviewConfigSnapshot flattens cfg's json form into dotted paths, the same
// paths the wizard's field keys use, so the diff needs no hand-kept table.
func reviewConfigSnapshot(cfg *config.Config) map[string]string {
	if cfg == nil {
		return nil
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil
	}
	var tree any
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil
	}
	out := make(map[string]string)
	flattenJSON("", tree, out)
	return out
}

func flattenJSON(prefix string, v any, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			flattenJSON(path, child, out)
		}
	case []any:
		for i, child := range t {
			if _, nested := child.(map[string]any); !nested {
				parts := make([]string, len(t))
				for j, item := range t {
					parts[j] = scalarText(item)
				}
				out[prefix] = tui.SanitizeTerminalEscapes(strings.Join(parts, ", "))
				return
			}
			flattenJSON(fmt.Sprintf("%s[%d]", prefix, i), child, out)
		}
	default:
		out[prefix] = tui.SanitizeTerminalEscapes(scalarText(t))
	}
}

func scalarText(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// PinnedFooter renders the action selector inline on the help row, empty
// while there is no configuration to act on.
func (s *ReviewStep) PinnedFooter(width int) string {
	if s.cfg == nil {
		return ""
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s.actions.ViewInline())
}

func (s *ReviewStep) renderClusterIdentity(st *wizard.SectionStyles) string {
	return wizard.RenderSection(st, s.sectionTitle("cluster identity", wizard.StepIDBasics), []wizard.KVEntry{
		{Label: "name", Value: s.cfg.Cluster.Name},
		{Label: "domain", Value: s.cfg.Cluster.Domain},
		{Label: "distribution", Value: "OKD " + s.cfg.Distribution.Version},
	})
}

func (s *ReviewStep) renderProxmox(st *wizard.SectionStyles) string {
	p := &s.cfg.Provider.Proxmox
	return wizard.RenderSection(st, s.sectionTitle("proxmox", wizard.StepIDProxmox), []wizard.KVEntry{
		{Label: "endpoint", Value: p.Endpoint},
		{Label: "nodes", Value: tui.SanitizeTerminalEscapes(strings.Join(p.Nodes, ", "))},
		{Label: "storage", Value: tui.SanitizeTerminalEscapes(p.Storage)},
		{Label: "iso storage", Value: tui.SanitizeTerminalEscapes(p.ISOStorage)},
		{Label: "bridge", Value: tui.SanitizeTerminalEscapes(p.Bridge)},
		{Label: labelVLAN, Value: strconv.Itoa(p.VLAN), Skip: p.VLAN == 0},
		{Label: labelCPUType, Value: p.CPUType},
		{Label: "pool", Value: tui.SanitizeTerminalEscapes(p.Pool), Skip: p.Pool == ""},
		{Label: "ha", Value: valYes, Skip: !p.HA},
		{Label: labelVMIDBase, Value: strconv.Itoa(p.VMIDBase)},
	})
}

// renderNodes tabulates every node the model derives from the config.
func (s *ReviewStep) renderNodes(st *wizard.SectionStyles, width int) string {
	var b strings.Builder
	b.WriteString(st.Header.Render(s.sectionTitle("nodes", wizard.StepIDNodePlacement)))
	b.WriteString("\n")
	b.WriteString(st.Separator)
	b.WriteString("\n")
	inv, err := model.Build(s.cfg, "")
	if err != nil {
		warn := lipgloss.NewStyle().Foreground(tui.ColorWarning())
		b.WriteString(warn.Render(tui.Truncate("node identities unavailable: "+err.Error(), width)))
		b.WriteString("\n\n")
		return b.String()
	}
	rows := make([][]string, 0, len(inv.Nodes))
	for i := range inv.Nodes {
		n := &inv.Nodes[i]
		rows = append(rows, []string{
			n.Name, strconv.Itoa(n.VMID), n.IP.String(), tui.SanitizeTerminalEscapes(n.Host),
			strconv.Itoa(n.CPU), fmt.Sprintf("%d gb", n.MemoryMiB/1024), disksText(n.Disks),
		})
	}
	columns := []tui.Column{
		{Header: "node", MinWidth: 6},
		{Header: "vmid", Align: tui.AlignRight, MinWidth: 4},
		{Header: "ip", MinWidth: 7},
		{Header: "host", MinWidth: 4},
		{Header: "vcpu", Align: tui.AlignRight, MinWidth: 4},
		{Header: "ram", Align: tui.AlignRight, MinWidth: 5},
		{Header: "disks", Align: tui.AlignRight, MinWidth: 5},
	}
	for _, line := range tui.ColumnTable(columns, []tui.RowGroup{{Rows: rows}}, tui.TableOptions{Gap: 1}) {
		b.WriteString(tui.Truncate(line, width))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

func disksText(disks []model.Disk) string {
	parts := make([]string, len(disks))
	for i, d := range disks {
		parts[i] = strconv.Itoa(d.SizeGiB)
	}
	return strings.Join(parts, "+") + " gb"
}

func (s *ReviewStep) renderNetworking(st *wizard.SectionStyles) string {
	n := &s.cfg.Networking
	return wizard.RenderSection(st, s.sectionTitle("networking", wizard.StepIDNetworking), []wizard.KVEntry{
		{Label: "machine cidr", Value: n.MachineCIDR},
		{Label: "gateway", Value: n.Gateway},
		{Label: "dns", Value: strings.Join(n.DNS, ", ")},
		{Label: "api vip", Value: n.APIVIP},
		{Label: "ingress vip", Value: n.IngressVIP},
		{Label: "node ip start", Value: n.NodeIPStart},
		{Label: "ntp", Value: strings.Join(n.NTP, ", "), Skip: len(n.NTP) == 0},
	})
}

func roleSpec(count int, d *config.NodeDefaults) string {
	sizes := make([]string, len(d.Disks))
	for i, disk := range d.Disks {
		sizes[i] = strconv.Itoa(disk.SizeGiB)
	}
	return fmt.Sprintf("%d × (%d vcpu, %d gb ram, disks %s gb)", count, d.CPU, d.MemoryMiB/1024, strings.Join(sizes, "+"))
}

// renderCompute renders the per-role sizing and the inventory total, with
// warnings when the total exceeds the discovered online capacity.
func (s *ReviewStep) renderCompute(st *wizard.SectionStyles, width int) string {
	entries := []wizard.KVEntry{
		{Label: roleLabelControlPlane, Value: roleSpec(config.MasterCount, &s.cfg.ControlPlane)},
		{Label: roleLabelWorkers, Value: roleSpec(s.cfg.Workers.Count, &s.cfg.Workers.NodeDefaults), Skip: s.cfg.Workers.Count == 0},
		{Label: roleLabelWorkers, Value: "none — the control plane runs the workloads", Skip: s.cfg.Workers.Count != 0},
	}
	warnings := ""
	if inv, err := model.Build(s.cfg, ""); err == nil {
		cpu, memoryMiB, diskGiB := inventoryTotals(&inv)
		entries = append(entries, wizard.KVEntry{Label: "total", Value: fmt.Sprintf("%d vcpu, %d gb ram, %d gb disk", cpu, memoryMiB/1024, diskGiB)})
		warnings = s.renderComputeWarnings(cpu, memoryMiB/1024, width)
	}
	section := wizard.RenderSection(st, s.sectionTitle("compute", wizard.StepIDResources), entries)
	if warnings == "" {
		return section
	}
	return strings.TrimRight(section, "\n") + "\n" + warnings + "\n"
}

func (s *ReviewStep) renderComputeWarnings(totalCPU, totalMemGB, width int) string {
	warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning())
	perHost := ""
	if n := len(s.cfg.Provider.Proxmox.Nodes); n > 1 {
		perHost = fmt.Sprintf(" across %d nodes", n)
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

func (s *ReviewStep) renderFiles(st *wizard.SectionStyles) string {
	return wizard.RenderSection(st, s.sectionTitle("files", wizard.StepIDFiles), []wizard.KVEntry{
		{Label: "pull secret", Value: truncatePath(s.cfg.Files.PullSecret, 40)},
		{Label: "ssh public key", Value: truncatePath(s.cfg.Files.SSHPublicKey, 40)},
	})
}

func (s *ReviewStep) renderAdvanced(st *wizard.SectionStyles) string {
	n, l := &s.cfg.Networking, &s.cfg.Lifecycle
	catalogs := "enabled"
	if s.cfg.Defaults.DisableRedHatCatalogs {
		catalogs = "disabled"
	}
	return wizard.RenderSection(st, s.sectionTitle("advanced", wizard.StepIDAdvanced), []wizard.KVEntry{
		{Label: "pod cidr", Value: n.PodCIDR},
		{Label: "service cidr", Value: n.ServiceCIDR},
		{Label: "host prefix", Value: strconv.Itoa(n.HostPrefix)},
		{Label: "before next node", Value: l.BeforeNextNode.Command + " (" + durationText(l.BeforeNextNode.Timeout) + ")", Skip: l.BeforeNextNode.Command == ""},
		{Label: "drain timeout", Value: durationText(l.DrainTimeout)},
		{Label: "handoff", Value: getBool(func(c *config.Config) bool { return c.Handoff.Enabled })(s.cfg)},
		{Label: "red hat catalogs", Value: catalogs},
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
	if len(s.jumpTargets) > 0 {
		bindings = append(bindings, wizard.KeyBinding{Key: "1-9", Help: wizard.HelpJump})
	}
	return append(bindings, wizard.KeyBinding{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit})
}

// SetFocused refreshes the preflight checks when the review screen opens.
func (s *ReviewStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	s.actions.SetFocused(focused)
	if focused {
		s.preflight = reviewPreflight(s.cfg, s.capacity)
	}
}

// GetSelectedAction returns the action the user chose on the review screen.
func (s *ReviewStep) GetSelectedAction() wizard.Action {
	if s.actions.SelectedIndex() == 0 {
		return wizard.ActionDeploy
	}
	return wizard.ActionExit
}

// FocusBounds keeps the actions visible beneath the configuration summary.
func (s *ReviewStep) FocusBounds(width, height int) (top, bottom int, ok bool) {
	if s.cfg == nil {
		return 0, 0, false
	}
	bottom = lipgloss.Height(lipgloss.NewStyle().Width(width).Render(s.View(width, height)))
	return bottom - lipgloss.Height(s.actions.View()), bottom, true
}

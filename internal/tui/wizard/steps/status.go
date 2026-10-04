package steps

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// StepIDClusterStatus identifies the hub's read-only cluster-status screen.
const StepIDClusterStatus wizard.StepID = "cluster-status"

// statusRefreshKey re-probes the cluster from the status screen.
const statusRefreshKey = "r"

const statusRefreshInterval = 30 * time.Second

// errNoStatusSource reports a status screen assembled with no source behind it.
var errNoStatusSource = errors.New("read cluster status: no source configured")

// StatusSource supplies the snapshot the status screen renders, with the
// collecting side owning its own context (as lifecycle.Hooks do) so the
// screen never fabricates one and a test or the demo seeds a fixture
// through the same seam.
type StatusSource interface {
	ClusterStatus(context.Context) (*okd.ClusterStatus, error)
}

// StaticStatusSource serves one fixed snapshot, or one fixed failure, as a StatusSource.
type StaticStatusSource struct {
	Status *okd.ClusterStatus
	Err    error
}

// ClusterStatus returns the fixture unchanged.
func (s StaticStatusSource) ClusterStatus(context.Context) (*okd.ClusterStatus, error) {
	return s.Status, s.Err
}

// statusLoadedMsg carries a finished StatusSource probe back to the step.
type statusLoadedMsg struct {
	generation uint64
	status     *okd.ClusterStatus
	err        error
}

type statusRefreshMsg struct{ generation uint64 }

// StatusStep renders okdctl status's own box read-only inside the wizard
// viewport; "r" re-probes the cluster and esc returns to the hub.
type StatusStep struct {
	wizard.BaseStep
	src             StatusSource
	status          *okd.ClusterStatus
	err             error
	loading         bool
	generation      uint64
	selectedNode    string
	detailOpen      bool
	termWidth       int
	termHeight      int
	lifecycleCtx    context.Context
	cancelLifecycle context.CancelFunc
	cancelProbe     context.CancelFunc
	cancelRefresh   context.CancelFunc
}

// NewStatusStep constructs the read-only cluster-status screen over src.
func NewStatusStep(src StatusSource) *StatusStep {
	return &StatusStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(StepIDClusterStatus, "status", "cluster status", ""),
		src:      src,
	}
}

// StatusFlow returns the single-screen flow the hub's cluster-status verb swaps in.
func StatusFlow(src StatusSource) ([]wizard.WizardStep, wizard.FlowChrome) {
	return []wizard.WizardStep{NewStatusStep(src)}, StatusChrome()
}

// StatusChrome returns the status screen's chrome: the cluster name as badge
// and an empty trail, since a single screen has no progress to report.
func StatusChrome() wizard.FlowChrome {
	return wizard.FlowChrome{
		Tagline: "read-only cluster snapshot",
		Badge:   func(cfg *config.Config) string { return cfg.Cluster.Name },
		Trail:   func(wizard.ProgressInfo) string { return "" },
	}
}

// Init starts the first probe.
func (s *StatusStep) Init() tea.Cmd {
	s.start()
	return s.probe()
}

func (s *StatusStep) start() {
	if s.lifecycleCtx != nil {
		return
	}
	// The step owns polling and cancels it when focus leaves.
	s.lifecycleCtx, s.cancelLifecycle = context.WithCancel(context.Background())
}

func (s *StatusStep) probe() tea.Cmd {
	s.start()
	s.loading = true
	s.generation++
	generation := s.generation
	if s.cancelProbe != nil {
		s.cancelProbe()
	}
	ctx, cancel := context.WithCancel(s.lifecycleCtx)
	s.cancelProbe = cancel
	src := s.src
	return func() tea.Msg {
		if src == nil {
			return statusLoadedMsg{generation: generation, err: errNoStatusSource}
		}
		status, err := src.ClusterStatus(ctx)
		return statusLoadedMsg{generation: generation, status: status, err: err}
	}
}

func (s *StatusStep) scheduleRefresh() tea.Cmd {
	if s.cancelRefresh != nil {
		s.cancelRefresh()
	}
	ctx, cancel := context.WithCancel(s.lifecycleCtx)
	s.cancelRefresh = cancel
	generation := s.generation
	return func() tea.Msg {
		timer := time.NewTimer(statusRefreshInterval)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			return statusRefreshMsg{generation: generation}
		}
	}
}

// SetFocused cancels an in-flight probe when the status screen loses focus.
func (s *StatusStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if !focused && s.lifecycleCtx != nil {
		s.cancelLifecycle()
		s.lifecycleCtx = nil
		s.cancelLifecycle = nil
		if s.cancelProbe != nil {
			s.cancelProbe()
			s.cancelProbe = nil
		}
		if s.cancelRefresh != nil {
			s.cancelRefresh()
			s.cancelRefresh = nil
		}
		s.generation++
		s.loading = false
	}
}

// Update records a finished probe and re-probes on the refresh key.
func (s *StatusStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case statusLoadedMsg:
		if msg.generation != s.generation {
			return s, nil
		}
		if s.cancelProbe != nil {
			s.cancelProbe()
			s.cancelProbe = nil
		}
		s.loading = false
		s.err = msg.err
		if msg.err == nil {
			s.status = msg.status
			s.reconcileSelection()
		}
		cmd := s.scheduleRefresh()
		return s, cmd
	case statusRefreshMsg:
		if msg.generation != s.generation || !s.IsFocused() {
			return s, nil
		}
		cmd := s.probe()
		return s, cmd
	case tea.KeyPressMsg:
		switch msg.Code {
		case tea.KeyUp:
			s.moveSelection(-1)
		case tea.KeyDown:
			s.moveSelection(1)
		case tea.KeyEnter:
			if s.selectedNode != "" {
				s.detailOpen = !s.detailOpen
			}
		default:
			if msg.String() == statusRefreshKey {
				cmd := s.probe()
				return s, cmd
			}
		}
	}
	return s, nil
}

// SuppressesSplit returns false so the selected node can use the wide context pane.
func (s *StatusStep) SuppressesSplit() bool {
	return false
}

// SetTerminalSize records the terminal's own dimensions, the frame's actual
// split-layout gate (wizard.SplitsFrame) — not a second, independently
// guessed content-width threshold that can disagree with it and strand the
// selected node's detail off-screen.
func (s *StatusStep) SetTerminalSize(width, height int) {
	s.termWidth, s.termHeight = width, height
}

// View renders the probe's box, its in-flight notice, or its failure.
func (s *StatusStep) View(width, height int) string {
	s.SetSize(width, height)
	if width < 1 || height < 1 {
		return ""
	}
	if s.status == nil {
		return fitStatusLines(statusEmptyLines(s.loading, s.err), width, height)
	}
	// 1: StatusFlow always wraps this step alone, so it's always the frame's
	// only step for splitMinHeight's purposes.
	split := wizard.SplitsFrame(s.termWidth, s.termHeight, 1)
	return fitStatusLines(statusBoardLines(s.status, s.loading, s.err, s.selectedNode, s.detailOpen, width, split), width, height)
}

// PaneContent renders selected-node and cluster facts beside the grouped node table.
func (s *StatusStep) PaneContent(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	if s.status == nil {
		return fitStatusLines(statusEmptyLines(s.loading, s.err), width, height)
	}
	return fitStatusLines(statusPaneLines(s.status, s.selectedNode, s.detailOpen, width, height), width, height)
}

func statusEmptyLines(loading bool, err error) []string {
	lines := []string{
		lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true).Render("CLUSTER STATUS"),
	}
	switch {
	case loading:
		lines = append(lines, lipgloss.NewStyle().Foreground(tui.ColorTextDim()).Render("reading cluster status…"))
	case err != nil:
		lines = append(lines,
			lipgloss.NewStyle().Foreground(tui.ColorError()).Render("cluster status unavailable"),
			lipgloss.NewStyle().Foreground(tui.ColorTextDim()).Render("press r to retry"),
			lipgloss.NewStyle().Foreground(tui.ColorTextFaint()).Render(err.Error()),
		)
	default:
		lines = append(lines, tui.EmptyState("no cluster status reported", "deploy a cluster with 'okdctl deploy'"))
	}
	return lines
}

func statusBoardLines(st *okd.ClusterStatus, loading bool, err error, selected string, detail bool, width int, split bool) []string {
	compact := width < 80
	lines := statusSummaryLines(st, compact)
	lines = append(lines, statusNodeSectionLines(st, selected, detail, width, split, compact)...)
	lines = append(lines, statusAddonSectionLines(st, compact)...)
	lines = statusPrependBanners(lines, loading, err)
	if !compact {
		lines = append(lines, "", "↑/↓ select node · enter details · r refresh")
	}
	return lines
}

// statusSummaryLines renders the header, the phase/API/node/operator summary
// line, and the NODES section heading shared by every width.
func statusSummaryLines(st *okd.ClusterStatus, compact bool) []string {
	phase := statusPhaseStyle(st.Phase).Render(string(st.Phase))
	api := lipgloss.NewStyle().Foreground(tui.ColorError()).Render(tui.IconError + " unavailable")
	if st.APIReachable {
		latency := ""
		if st.APILatencyAvailable {
			latency = " · " + st.APILatency.Round(time.Millisecond).String()
		}
		api = lipgloss.NewStyle().Foreground(tui.ColorSuccess()).Render(tui.IconSuccess + " healthy" + latency)
	} else if st.APIAvailable {
		api = lipgloss.NewStyle().Foreground(tui.ColorError()).Render(tui.IconError + " unreachable")
		if st.APILatencyAvailable {
			api += " · " + st.APILatency.Round(time.Millisecond).String()
		}
	}
	nodes := nodeCounts(st.Nodes)
	ready := 0
	for _, node := range st.Nodes {
		if node.Ready {
			ready++
		}
	}
	nodeReadiness := "NODES unavailable"
	if st.NodesAvailable {
		nodeReadiness = fmt.Sprintf("NODES %d/%d ready", ready, nodes.total)
	}
	operatorReadiness := "OPERATORS unavailable"
	if st.OperatorsAvailable {
		operatorReadiness = fmt.Sprintf("OPERATORS %d degraded", st.DegradedOperators)
	}
	summary := "API " + api + "   " + nodeReadiness + "   " + operatorReadiness
	if compact {
		summary = "API " + api + "   " + operatorReadiness
	}
	lines := []string{
		lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true).Render("CLUSTER STATUS") + " · " + phase,
		summary,
	}
	if !compact {
		lines = append(lines, "")
	}
	lines = append(lines, fmt.Sprintf("NODES · %d (%d master · %d worker)", nodes.total, nodes.masters, nodes.workers))
	return lines
}

// statusNodeSectionLines renders the node table, or its unavailable/empty/
// narrow-width fallbacks, including the selected node's detail rows.
func statusNodeSectionLines(st *okd.ClusterStatus, selected string, detail bool, width int, split, compact bool) []string {
	var lines []string
	switch {
	case !st.NodesAvailable:
		lines = append(lines, lipgloss.NewStyle().Foreground(tui.ColorTextFaint()).Render("node inventory unavailable"))
	case len(st.Nodes) == 0:
		lines = append(lines, lipgloss.NewStyle().Foreground(tui.ColorTextFaint()).Render("no nodes reported"))
	case width < 48:
		for _, node := range sortedStatusNodes(st.Nodes) {
			mark := tui.IconError
			if node.Ready {
				mark = tui.IconSuccess
			}
			lines = append(lines, mark+" "+node.Name+" · "+statusNodeRole(node)+" · "+statusReadiness(node))
		}
	default:
		ordered := sortedStatusNodes(st.Nodes)
		selectedIndex := slices.IndexFunc(ordered, func(node okd.NodeStatus) bool { return node.Name == selected })
		table := tui.ColumnTable(
			[]tui.Column{{Header: "", MinWidth: 2, MaxWidth: 2}, {Header: "NODE", Weight: 1}, {Header: "READINESS", MinWidth: 9}},
			statusNodeGroups(st.Nodes, selected), tui.TableOptions{Width: width, Gap: 2, MaxColWidth: max(width-16, 8), RowStyle: func(row int) (lipgloss.Style, bool) {
				if row == selectedIndex {
					return lipgloss.NewStyle().Foreground(tui.ColorAccent()).Bold(true), true
				}
				return lipgloss.NewStyle(), false
			}},
		)
		if compact && len(table) > 0 {
			table = table[1:]
		}
		lines = append(lines, table...)
		if detail && !split {
			for _, node := range st.Nodes {
				if node.Name == selected {
					lines = append(lines, statusNodeDetail(node, width)...)
				}
			}
		}
	}
	return lines
}

// statusAddonSectionLines renders the ADD-ONS section, or no lines at all
// when the cluster reports none.
func statusAddonSectionLines(st *okd.ClusterStatus, compact bool) []string {
	if len(st.Addons) == 0 {
		return nil
	}
	var lines []string
	if !compact {
		lines = append(lines, "", "ADD-ONS")
	}
	for i, addon := range st.Addons {
		mark, style := tui.IconError, tui.ColorError()
		if addon.Healthy {
			mark, style = tui.IconSuccess, tui.ColorSuccess()
		}
		prefix := ""
		if compact && i == 0 {
			prefix = "ADD-ONS · "
		}
		lines = append(lines, prefix+lipgloss.NewStyle().Foreground(style).Render(mark)+" "+addon.Name+" · "+addon.Label())
	}
	return lines
}

// statusPrependBanners prepends the in-flight and stale-snapshot banners
// ahead of lines, in that order, when loading and err call for them.
func statusPrependBanners(lines []string, loading bool, err error) []string {
	if loading {
		lines = append([]string{lipgloss.NewStyle().Foreground(tui.ColorAccent()).Render("refreshing cluster status…")}, lines...)
	}
	if err != nil {
		lines = append([]string{
			lipgloss.NewStyle().Foreground(tui.ColorWarning()).Render("refresh failed · showing last snapshot"),
			lipgloss.NewStyle().Foreground(tui.ColorTextFaint()).Render(err.Error()),
		}, lines...)
	}
	return lines
}

func statusPaneLines(st *okd.ClusterStatus, selected string, detail bool, width, height int) []string {
	var node *okd.NodeStatus
	for i := range st.Nodes {
		if st.Nodes[i].Name == selected {
			node = &st.Nodes[i]
			break
		}
	}
	cluster := []string{lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true).Render("CLUSTER FACTS")}
	cluster = append(cluster, statusClusterFactLines(st, width)...)
	addons := statusAddonLines(st)
	if node == nil {
		return distributeStatusSections([][]string{cluster, addons}, height)
	}

	lines := []string{lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true).Render("SELECTED NODE")}
	if detail {
		lines = append(lines, statusNodeDetail(*node, width)...)
	} else {
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render(node.Name),
			"  "+statusNodeRole(*node)+" · "+statusReadiness(*node))
	}
	return distributeStatusSections([][]string{lines, cluster, addons}, height)
}

func distributeStatusSections(sections [][]string, height int) []string {
	nonempty := sections[:0]
	for _, section := range sections {
		if len(section) > 0 {
			nonempty = append(nonempty, section)
		}
	}
	sections = nonempty
	total := 0
	for _, section := range sections {
		total += len(section)
	}
	separators := min(max(len(sections)-1, 0), max(height-total, 0))
	extra := max(height-total-separators, 0)
	var lines []string
	for i, section := range sections {
		lines = append(lines, section...)
		if i == len(sections)-1 || separators == 0 {
			continue
		}
		gap := 1
		switch len(sections) {
		case 2:
			gap += extra
		case 3:
			if i == 0 {
				gap += extra / 3
			} else {
				gap += extra - extra/3
			}
		}
		lines = append(lines, make([]string, gap)...)
	}
	return lines
}

func statusAddonLines(st *okd.ClusterStatus) []string {
	if len(st.Addons) == 0 && st.LastDeployAt.IsZero() {
		return nil
	}
	var lines []string
	if len(st.Addons) > 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true).Render("ADD-ONS"))
	}
	for _, addon := range st.Addons {
		mark, color := tui.IconError, tui.ColorError()
		if addon.Healthy {
			mark, color = tui.IconSuccess, tui.ColorSuccess()
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(color).Render(mark)+" "+addon.Name+" · "+addon.Label())
	}
	if !st.LastDeployAt.IsZero() {
		lines = append(lines, "", "Last deploy: "+st.LastDeployAt.Format("2006-01-02 15:04"))
	}
	return lines
}

func statusClusterFactLines(st *okd.ClusterStatus, width int) []string {
	api := statusUnavailable
	if st.APIAvailable {
		api = "unreachable"
		if st.APIReachable {
			api = "healthy"
		}
	}
	if st.APILatencyAvailable {
		api += " · " + st.APILatency.Round(time.Millisecond).String()
	}
	nodes := statusUnavailable
	var masters, workers int
	var readyMasters, readyWorkers int
	if st.NodesAvailable {
		ready := 0
		for _, node := range st.Nodes {
			if node.Ready {
				ready++
			}
			switch node.Role {
			case nodetypes.RoleMaster:
				masters++
				if node.Ready {
					readyMasters++
				}
			case nodetypes.RoleWorker:
				workers++
				if node.Ready {
					readyWorkers++
				}
			}
		}
		nodes = fmt.Sprintf("%d/%d ready", ready, len(st.Nodes))
	}
	operators := statusUnavailable
	if st.OperatorsAvailable {
		operators = fmt.Sprintf("%d degraded", st.DegradedOperators)
	}
	rows := []tui.FactRow{
		{Key: "Phase", Value: string(st.Phase)},
		{Key: "API", Value: api},
		{Key: "Nodes", Value: nodes},
		{Key: "Operators", Value: operators},
	}
	if st.NodesAvailable {
		if masters > 0 {
			rows = append(rows, tui.FactRow{Key: "Masters", Value: fmt.Sprintf("%d/%d ready", readyMasters, masters)})
		}
		if workers > 0 {
			rows = append(rows, tui.FactRow{Key: "Workers", Value: fmt.Sprintf("%d/%d ready", readyWorkers, workers)})
		}
	}
	lines := tui.RenderFacts(rows, &tui.FactLayout{
		Leader:     tui.FactLeaderColon,
		TotalWidth: width,
		Styles:     tui.DefaultFactStyles(),
	})
	return lines
}

func statusNodeGroups(nodes []okd.NodeStatus, selected string) []tui.RowGroup {
	groups := []tui.RowGroup{{Title: "master"}, {Title: "worker"}, {Title: "other"}}
	for _, node := range sortedStatusNodes(nodes) {
		role := statusNodeRole(node)
		group := 2
		switch role {
		case "master":
			group = 0
		case "worker":
			group = 1
		}
		marker := " "
		if node.Name == selected {
			marker = ">"
		}
		groups[group].Rows = append(groups[group].Rows, []string{marker, node.Name, statusReadiness(node)})
	}
	visible := groups[:0]
	for _, group := range groups {
		if len(group.Rows) > 0 {
			visible = append(visible, group)
		}
	}
	return visible
}

func sortedStatusNodes(nodes []okd.NodeStatus) []okd.NodeStatus {
	ordered := slices.Clone(nodes)
	slices.SortFunc(ordered, func(a, b okd.NodeStatus) int {
		if statusRoleOrder(a) != statusRoleOrder(b) {
			return statusRoleOrder(a) - statusRoleOrder(b)
		}
		return strings.Compare(a.Name, b.Name)
	})
	return ordered
}

func statusRoleOrder(node okd.NodeStatus) int {
	switch node.Role {
	case nodetypes.RoleMaster:
		return 0
	case nodetypes.RoleWorker:
		return 1
	default:
		return 2
	}
}

func statusNodeRole(node okd.NodeStatus) string {
	if node.Role == "" || node.Role == nodetypes.RoleUnknown {
		return "other"
	}
	return string(node.Role)
}

func statusReadiness(node okd.NodeStatus) string {
	if node.Ready {
		return "ready"
	}
	return "not ready"
}

func statusNodeDetail(node okd.NodeStatus, width int) []string {
	status := string(node.Status)
	if status == "" {
		status = statusReadiness(node)
	}
	return tui.RenderFacts([]tui.FactRow{{Key: "Selected node", Value: node.Name}, {Key: "Role", Value: statusNodeRole(node)}, {Key: "Readiness", Value: status}, {Key: "Ready condition", Value: statusReadiness(node)}}, &tui.FactLayout{Leader: tui.FactLeaderPad, KeyWidth: 18, TotalWidth: width, Styles: tui.DefaultFactStyles()})
}

func (s *StatusStep) reconcileSelection() {
	if s.status == nil || len(s.status.Nodes) == 0 {
		s.selectedNode = ""
		s.detailOpen = false
		return
	}
	for _, node := range s.status.Nodes {
		if node.Name == s.selectedNode {
			return
		}
	}
	s.selectedNode = sortedStatusNodes(s.status.Nodes)[0].Name
	s.detailOpen = false
}

func (s *StatusStep) moveSelection(delta int) {
	if s.status == nil || len(s.status.Nodes) == 0 {
		return
	}
	nodes := sortedStatusNodes(s.status.Nodes)
	index := slices.IndexFunc(nodes, func(node okd.NodeStatus) bool { return node.Name == s.selectedNode })
	index = (index + delta + len(nodes)) % len(nodes)
	s.selectedNode = nodes[index].Name
	s.detailOpen = false
}

type statusNodeCounts struct {
	masters int
	workers int
	total   int
}

func nodeCounts(nodes []okd.NodeStatus) statusNodeCounts {
	counts := statusNodeCounts{total: len(nodes)}
	for _, node := range nodes {
		switch node.Role {
		case nodetypes.RoleMaster:
			counts.masters++
		case nodetypes.RoleWorker:
			counts.workers++
		}
	}
	return counts
}

func statusPhaseStyle(phase okd.ClusterPhase) lipgloss.Style {
	color := tui.ColorTextDim()
	switch phase {
	case okd.PhaseRunning:
		color = tui.ColorSuccess()
	case okd.PhaseDegraded:
		color = tui.ColorWarning()
	case okd.PhaseStopped:
		color = tui.ColorError()
	case okd.PhaseInstalling:
		color = tui.ColorInfo()
	}
	return lipgloss.NewStyle().Foreground(color).Bold(true)
}

func fitStatusLines(lines []string, width, height int) string {
	if len(lines) > height {
		lines = append([]string(nil), lines[:height]...)
		lines[height-1] = lipgloss.NewStyle().Foreground(tui.ColorTextFaint()).Render("… more status details")
	}
	for i := range lines {
		lines[i] = lipgloss.NewStyle().MaxWidth(width).Render(lines[i])
	}
	return strings.Join(lines, "\n")
}

// ShortHelp returns the status screen's help bar.
func (s *StatusStep) ShortHelp() []wizard.KeyBinding {
	return []wizard.KeyBinding{
		{Key: wizard.HelpEsc, Help: "hub"},
		{Key: statusRefreshKey, Help: "refresh"},
		{Key: "↑↓", Help: "nodes"},
		{Key: wizard.HelpEnter, Help: "details"},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
}

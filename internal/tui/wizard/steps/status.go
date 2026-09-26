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

// StatusSource supplies the snapshot the status screen renders. The collecting
// side owns its own context (the way lifecycle.Hooks do), so the screen never
// fabricates one, and a test or the demo seeds a fixture through the same seam.
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

// SuppressesSplit returns true: the status box already owns the frame's width,
// and a context pane listing this flow's one step describes nothing.
func (s *StatusStep) SuppressesSplit() bool {
	return true
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
	return fitStatusLines(statusBoardLines(s.status, s.loading, s.err, s.selectedNode, s.detailOpen, width), width, height)
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

func statusBoardLines(st *okd.ClusterStatus, loading bool, err error, selected string, detail bool, width int) []string {
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
	lines := []string{
		lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true).Render("CLUSTER STATUS") + " · " + phase,
		"API " + api + "   " + nodeReadiness + "   " + operatorReadiness,
		"",
		fmt.Sprintf("NODES · %d (%d master · %d worker)", nodes.total, nodes.masters, nodes.workers),
	}
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
		lines = append(lines, tui.ColumnTable(
			[]tui.Column{{Header: "", MinWidth: 2, MaxWidth: 2}, {Header: "NODE", Weight: 1}, {Header: "READINESS", MinWidth: 9}},
			statusNodeGroups(st.Nodes, selected), tui.TableOptions{Width: width, Gap: 2, MaxColWidth: max(width-16, 8), RowStyle: func(row int) (lipgloss.Style, bool) {
				if row == selectedIndex {
					return lipgloss.NewStyle().Foreground(tui.ColorAccent()).Bold(true), true
				}
				return lipgloss.NewStyle(), false
			}},
		)...)
		if detail {
			for _, node := range st.Nodes {
				if node.Name == selected {
					lines = append(lines, statusNodeDetail(node, width)...)
				}
			}
		}
	}
	if len(st.Addons) > 0 {
		lines = append(lines, "", "ADD-ONS")
		for _, addon := range st.Addons {
			mark, style := tui.IconError, tui.ColorError()
			if addon.Healthy {
				mark, style = tui.IconSuccess, tui.ColorSuccess()
			}
			lines = append(lines, lipgloss.NewStyle().Foreground(style).Render(mark)+" "+addon.Name+" · "+addon.Label())
		}
	}
	if loading {
		lines = append([]string{lipgloss.NewStyle().Foreground(tui.ColorAccent()).Render("refreshing cluster status…")}, lines...)
	}
	if err != nil {
		lines = append([]string{
			lipgloss.NewStyle().Foreground(tui.ColorWarning()).Render("refresh failed · showing last snapshot"),
			lipgloss.NewStyle().Foreground(tui.ColorTextFaint()).Render(err.Error()),
		}, lines...)
	}
	lines = append(lines, "", "↑/↓ select node · enter details · r refresh")
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

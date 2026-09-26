package steps

import (
	"errors"
	"fmt"
	"strings"

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

// errNoStatusSource reports a status screen assembled with no source behind it.
var errNoStatusSource = errors.New("read cluster status: no source configured")

// StatusSource supplies the snapshot the status screen renders. The collecting
// side owns its own context (the way lifecycle.Hooks do), so the screen never
// fabricates one, and a test or the demo seeds a fixture through the same seam.
type StatusSource interface {
	ClusterStatus() (*okd.ClusterStatus, error)
}

// StaticStatusSource serves one fixed snapshot, or one fixed failure, as a StatusSource.
type StaticStatusSource struct {
	Status *okd.ClusterStatus
	Err    error
}

// ClusterStatus returns the fixture unchanged.
func (s StaticStatusSource) ClusterStatus() (*okd.ClusterStatus, error) {
	return s.Status, s.Err
}

// statusLoadedMsg carries a finished StatusSource probe back to the step.
type statusLoadedMsg struct {
	status *okd.ClusterStatus
	err    error
}

// StatusStep renders okdctl status's own box read-only inside the wizard
// viewport; "r" re-probes the cluster and esc returns to the hub.
type StatusStep struct {
	wizard.BaseStep
	src     StatusSource
	status  *okd.ClusterStatus
	err     error
	loading bool
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
	return s.probe()
}

// probe reads the source off the update loop and reports the result back as a statusLoadedMsg.
func (s *StatusStep) probe() tea.Cmd {
	s.loading = true
	src := s.src
	return func() tea.Msg {
		if src == nil {
			return statusLoadedMsg{err: errNoStatusSource}
		}
		status, err := src.ClusterStatus()
		return statusLoadedMsg{status: status, err: err}
	}
}

// Update records a finished probe and re-probes on the refresh key.
func (s *StatusStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case statusLoadedMsg:
		s.loading = false
		s.err = msg.err
		if msg.err == nil {
			s.status = msg.status
		}
		return s, nil
	case tea.KeyPressMsg:
		if msg.String() == statusRefreshKey {
			cmd := s.probe()
			return s, cmd
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
	return fitStatusLines(statusBoardLines(s.status, s.loading, s.err), width, height)
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

func statusBoardLines(st *okd.ClusterStatus, loading bool, err error) []string {
	phase := statusPhaseStyle(st.Phase).Render(string(st.Phase))
	api := lipgloss.NewStyle().Foreground(tui.ColorError()).Render(tui.IconError + " unreachable")
	if st.APIReachable {
		api = lipgloss.NewStyle().Foreground(tui.ColorSuccess()).Render(tui.IconSuccess + " reachable")
	}
	nodes := nodeCounts(st.Nodes)
	lines := []string{
		lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true).Render("CLUSTER STATUS") + " · " + phase,
		"API " + api + "  ·  Operators " + fmt.Sprintf("%d degraded", st.DegradedOperators),
		"",
		fmt.Sprintf("NODES · %d (%d master · %d worker)", nodes.total, nodes.masters, nodes.workers),
	}
	if len(st.Nodes) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(tui.ColorTextFaint()).Render("no nodes reported"))
	} else {
		for _, node := range st.Nodes {
			mark, style, readiness := tui.IconError, tui.ColorError(), "not ready"
			if node.Ready {
				mark, style, readiness = tui.IconSuccess, tui.ColorSuccess(), "ready"
			}
			role := string(node.Role)
			if role == "" || node.Role == nodetypes.RoleUnknown {
				role = "node"
			}
			lines = append(lines, lipgloss.NewStyle().Foreground(style).Render(mark)+" "+
				lipgloss.NewStyle().Foreground(tui.ColorText()).Render(node.Name)+" · "+role+" · "+readiness)
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
	return lines
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
		{Key: statusRefreshKey, Help: "refresh"},
		{Key: wizard.HelpEsc, Help: "hub"},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
}

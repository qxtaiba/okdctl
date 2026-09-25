package steps

import (
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/render"
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
		s.status, s.err = msg.status, msg.err
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

	switch {
	case s.loading:
		return lipgloss.NewStyle().Foreground(tui.ColorSlate400).Render("reading cluster status…")
	case s.err != nil:
		return tui.EmptyState("cluster status unavailable", "press r to retry") + "\n\n" +
			lipgloss.NewStyle().Foreground(tui.ColorSlate500).Render(lipgloss.Wrap(s.err.Error(), width, ""))
	case s.status == nil:
		return tui.EmptyState("no cluster status reported", "deploy a cluster with 'okdctl deploy'")
	default:
		return strings.Trim(render.ClusterStatusBoxWidth(s.status, min(width, tui.DefaultBoxWidth)), "\n")
	}
}

// ShortHelp returns the status screen's help bar.
func (s *StatusStep) ShortHelp() []wizard.KeyBinding {
	return []wizard.KeyBinding{
		{Key: statusRefreshKey, Help: "refresh"},
		{Key: wizard.HelpEsc, Help: "hub"},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
}

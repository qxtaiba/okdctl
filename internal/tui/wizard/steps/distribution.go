package steps

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/releases"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

// VersionFetcher resolves the OKD release catalog for the distribution step.
// *releases.OKDVersionFetcher is the production implementation;
// StaticVersionFetcher is a deterministic fixture for demo mode and tests.
type VersionFetcher interface {
	FetchVersions(ctx context.Context) ([]releases.OKDReleaseSeries, error)
}

// StaticVersionFetcher is a VersionFetcher fixture that returns a fixed
// release catalog (or a fixed error) without touching the network.
type StaticVersionFetcher struct {
	Series []releases.OKDReleaseSeries
	Err    error
}

// FetchVersions returns a copy of Series, or Err if it is set.
func (f StaticVersionFetcher) FetchVersions(_ context.Context) ([]releases.OKDReleaseSeries, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	series := make([]releases.OKDReleaseSeries, len(f.Series))
	copy(series, f.Series)
	return series, nil
}

type selectionPhase int

const (
	phaseVersionLoading selectionPhase = iota
	phaseVersionError
	phaseVersionSelect
)

// DistributionStep lets the user pick an OKD version via on-demand release
// fetching and grouped minor/patch display.
type DistributionStep struct {
	wizard.BaseStep
	versionSelector *components.Selector
	phase           selectionPhase
	selectedVersion string

	versionFetcher VersionFetcher
	okdSeries      []releases.OKDReleaseSeries
	expandedMinor  int // -1 = none expanded (show latest per minor)
	loadingSpinner spinner.Model
	loadError      error
}

// NewDistributionStep constructs the distribution step.
func NewDistributionStep() *DistributionStep {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(tui.ColorPrimary)

	selector := components.NewSelector(nil)

	return &DistributionStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(
			wizard.StepIDDistribution,
			"okd version",
			"which okd version would you like to deploy?",
			"select okd version",
		),
		versionSelector: selector,
		phase:           phaseVersionLoading,
		versionFetcher:  releases.NewOKDVersionFetcher(),
		expandedMinor:   -1,
		loadingSpinner:  s,
	}
}

// Init starts the release fetch and spins the loading indicator.
func (s *DistributionStep) Init() tea.Cmd {
	return tea.Batch(
		s.loadingSpinner.Tick,
		s.fetchVersions,
	)
}

// Update handles version-load messages, spinner ticks, and navigation keys.
func (s *DistributionStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case versionsLoadedMsg:
		s.okdSeries = msg.series
		s.loadError = msg.err
		if msg.err != nil {
			s.phase = phaseVersionError
			return s, nil
		}
		s.phase = phaseVersionSelect
		s.updateVersionSelector()
		s.versionSelector.SetFocused(true)
		return s, nil

	case spinner.TickMsg:
		if s.phase == phaseVersionLoading {
			var cmd tea.Cmd
			s.loadingSpinner, cmd = s.loadingSpinner.Update(msg)
			return s, cmd
		}

	case tea.KeyPressMsg:
		switch s.phase {
		case phaseVersionSelect:
			return s.handleKeyMsg(msg)
		case phaseVersionError:
			return s.handleErrorKeyMsg(msg)
		}
		return s, nil
	}
	return s, nil
}

func (s *DistributionStep) handleKeyMsg(msg tea.KeyPressMsg) (wizard.WizardStep, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		return s.handleEnterKey()
	case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
		return s.handleTabKey()
	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k", "down", "j"))):
		return s.handleNavigationKey(msg)
	}
	return s, nil
}

// handleErrorKeyMsg handles the error phase's only key: 'r' re-issues the
// release fetch.
func (s *DistributionStep) handleErrorKeyMsg(msg tea.KeyPressMsg) (wizard.WizardStep, tea.Cmd) {
	if key.Matches(msg, key.NewBinding(key.WithKeys("r"))) {
		return s.retry()
	}
	return s, nil
}

// retry resets the step to the loading phase and re-issues the release fetch.
func (s *DistributionStep) retry() (wizard.WizardStep, tea.Cmd) {
	s.phase = phaseVersionLoading
	s.loadError = nil
	return s, tea.Batch(s.loadingSpinner.Tick, s.fetchVersions)
}

func (s *DistributionStep) handleEnterKey() (wizard.WizardStep, tea.Cmd) {
	selected := s.versionSelector.Selected()

	if selected.ID == "" {
		return s, func() tea.Msg { return wizard.ErrorSetMsg{Error: errors.New("pick a release first")} }
	}

	if strings.HasPrefix(selected.ID, "minor:") {
		minor := s.getMinorFromOptionID(selected.ID)
		if s.expandedMinor == minor {
			for _, series := range s.okdSeries {
				if series.Minor == minor {
					s.selectedVersion = series.Latest.Version
					break
				}
			}
		} else {
			s.expandedMinor = minor
			s.updateVersionSelector()
			return s, nil
		}
	} else {
		s.selectedVersion = selected.ID
	}

	return s, func() tea.Msg {
		return wizard.StepCompleteMsg{StepID: s.ID()}
	}
}

func (s *DistributionStep) handleTabKey() (wizard.WizardStep, tea.Cmd) {
	selected := s.versionSelector.Selected()
	selectedMinor := s.getMinorFromOptionID(selected.ID)
	restoreID := selected.ID

	if s.expandedMinor == selectedMinor {
		s.expandedMinor = -1
		for _, series := range s.okdSeries {
			if series.Minor == selectedMinor {
				restoreID = fmt.Sprintf("minor:%d.%d", series.Major, series.Minor)
				break
			}
		}
	} else {
		s.expandedMinor = selectedMinor
	}

	s.updateVersionSelector()
	s.versionSelector.SetSelectedByID(restoreID)
	return s, func() tea.Msg { return wizard.FocusChangedMsg{} }
}

func (s *DistributionStep) handleNavigationKey(msg tea.KeyPressMsg) (wizard.WizardStep, tea.Cmd) {
	var cmd tea.Cmd
	s.versionSelector, cmd = s.versionSelector.Update(msg)
	selected := s.versionSelector.Selected()
	s.syncSelectedVersion(&selected)

	return s, tea.Batch(
		cmd,
		func() tea.Msg { return wizard.ConfigSyncMsg{StepID: s.ID()} },
		func() tea.Msg { return wizard.FocusChangedMsg{} },
	)
}

// FocusedSpan reports the lines the highlighted version occupies, patch rows
// inside the expanded dropdown included; the selector starts at line 0 of
// View, and there is nothing to focus outside the select phase.
func (s *DistributionStep) FocusedSpan() (wizard.LineSpan, bool) {
	if s.phase != phaseVersionSelect {
		return wizard.LineSpan{}, false
	}
	start, end, ok := s.versionSelector.SelectedSpan()
	if !ok {
		return wizard.LineSpan{}, false
	}
	return wizard.LineSpan{Start: start, End: end}, true
}

// syncSelectedVersion mirrors SelectField.Value(): latest patch for an
// unexpanded minor row, else the cursor's exact ID.
func (s *DistributionStep) syncSelectedVersion(selected *components.Option) {
	if selected == nil || selected.ID == "" {
		return
	}
	if strings.HasPrefix(selected.ID, "minor:") {
		minor := s.getMinorFromOptionID(selected.ID)
		for _, series := range s.okdSeries {
			if series.Minor == minor {
				s.selectedVersion = series.Latest.Version
				return
			}
		}
		return
	}
	s.selectedVersion = selected.ID
}

// View renders the loading indicator, the error state, or the version
// selector, depending on phase.
func (s *DistributionStep) View(width, height int) string {
	s.SetSize(width, height)

	switch s.phase {
	case phaseVersionLoading:
		return s.viewLoadingPhase()
	case phaseVersionError:
		return s.viewErrorPhase(width)
	case phaseVersionSelect:
		return s.viewVersionPhase()
	}

	return ""
}

// viewLoadingPhase renders the fetch-in-progress spinner and its dim
// this-can-take-a-few-seconds hint.
func (s *DistributionStep) viewLoadingPhase() string {
	var content strings.Builder
	content.WriteString(lipgloss.NewStyle().
		Foreground(tui.ColorSlate400).
		Render(s.loadingSpinner.View() + " fetching okd releases"))
	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().
		Foreground(tui.ColorSlate500).
		Italic(true).
		Render("this can take a few seconds"))
	return content.String()
}

// viewErrorPhase renders the empty state and retry ribbon shown when the
// release fetch failed, followed by the wrapped error detail.
func (s *DistributionStep) viewErrorPhase(width int) string {
	var content strings.Builder
	content.WriteString(tui.EmptyState("no releases loaded — check your connection", "r retry · esc back"))
	if s.loadError != nil {
		content.WriteString("\n\n")
		content.WriteString(lipgloss.NewStyle().
			Foreground(tui.ColorSlate500).
			Width(width - 2).
			Render(s.loadError.Error()))
	}
	return content.String()
}

func (s *DistributionStep) viewVersionPhase() string {
	var content strings.Builder

	content.WriteString(s.versionSelector.View())
	content.WriteString("\n\n")

	var hints []string
	if s.expandedMinor >= 0 {
		hints = append(hints,
			lipgloss.NewStyle().
				Foreground(tui.ColorSlate600).
				Render(fmt.Sprintf("showing patch versions for 4.%d", s.expandedMinor)),
			lipgloss.NewStyle().
				Foreground(tui.ColorSlate500).
				Italic(true).
				Render("press tab to collapse"),
		)
	} else {
		hints = append(hints, lipgloss.NewStyle().
			Foreground(tui.ColorSlate500).
			Italic(true).
			Render("press tab to expand patch versions"))
	}

	content.WriteString(strings.Join(hints, "\n"))

	return content.String()
}

// Validate always returns nil; any available version choice is valid.
func (s *DistributionStep) Validate() error {
	return nil
}

// Apply writes the selected OKD version into cfg.
func (s *DistributionStep) Apply(cfg *config.Config) error {
	cfg.Distribution.Type = config.DistributionOKD
	cfg.Distribution.Version = s.selectedVersion
	return nil
}

// ShortHelp returns the step's help bar, which differs by phase: the select
// phase's own bindings, or loading's {esc back, ctrl+c quit} with the error
// phase adding {r retry}.
func (s *DistributionStep) ShortHelp() []wizard.KeyBinding {
	if s.phase == phaseVersionSelect {
		return []wizard.KeyBinding{
			{Key: "↑↓", Help: "select"},
			{Key: "tab", Help: "expand/collapse"},
			{Key: wizard.HelpEnter, Help: wizard.HelpConfirm},
			{Key: wizard.HelpEsc, Help: wizard.HelpBack},
		}
	}

	help := []wizard.KeyBinding{
		{Key: wizard.HelpEsc, Help: wizard.HelpBack},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
	if s.phase == phaseVersionError {
		help = append(help, wizard.KeyBinding{Key: "r", Help: "retry"})
	}
	return help
}

// SetFocused toggles focus; the version selector is only focused once the
// release list has loaded.
func (s *DistributionStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if focused && s.phase == phaseVersionSelect {
		s.versionSelector.SetFocused(true)
	} else {
		s.versionSelector.SetFocused(false)
	}
}

// GetSelectedVersion returns the version the user has chosen.
func (s *DistributionStep) GetSelectedVersion() string {
	return s.selectedVersion
}

// SetSelectedVersion pre-selects a version, keeping the UI in sync.
func (s *DistributionStep) SetSelectedVersion(version string) {
	s.selectedVersion = version
	s.versionSelector.SetSelectedByID(version)
}

// SetVersionFetcher swaps the release-catalog fetcher, used by demo mode and
// tests to bypass the network with a deterministic fixture.
func (s *DistributionStep) SetVersionFetcher(f VersionFetcher) {
	s.versionFetcher = f
}

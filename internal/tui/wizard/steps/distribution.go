package steps

import (
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

type selectionPhase int

const (
	phaseVersionLoading selectionPhase = iota
	phaseVersionSelect
)

// DistributionStep is the bubbletea step that lets the user pick an OKD
// version, with on-demand release fetching and grouped minor/patch display.
type DistributionStep struct {
	wizard.BaseStep
	versionSelector *components.Selector
	phase           selectionPhase
	selectedVersion string

	// OKD version fetching
	versionFetcher *releases.OKDVersionFetcher
	okdSeries      []releases.OKDReleaseSeries
	expandedMinor  int // Which minor version is expanded (-1 = none, show only latest per minor)
	loadingSpinner spinner.Model
	loadError      error
}

// NewDistributionStep constructs the distribution step.
func NewDistributionStep() *DistributionStep {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(tui.ColorPrimary)

	selector := components.NewSelector(nil)
	selector.SetMaxDropdownVisible(5)

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
		if s.phase != phaseVersionSelect {
			return s, nil
		}
		return s.handleKeyMsg(msg)
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

func (s *DistributionStep) handleEnterKey() (wizard.WizardStep, tea.Cmd) {
	selected := s.versionSelector.Selected()

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
	return s, nil
}

func (s *DistributionStep) handleNavigationKey(msg tea.KeyPressMsg) (wizard.WizardStep, tea.Cmd) {
	var cmd tea.Cmd
	s.versionSelector, cmd = s.versionSelector.Update(msg)
	selected := s.versionSelector.Selected()
	if !selected.InDropdown {
		return s, tea.Batch(cmd, s.emitFocusChanged())
	}
	return s, cmd
}

// View renders either the loading indicator or the version selector.
func (s *DistributionStep) View(width, height int) string {
	s.SetSize(width, height)
	s.versionSelector.SetSize(width, height)

	switch s.phase {
	case phaseVersionLoading:
		return s.viewLoadingPhase()
	case phaseVersionSelect:
		return s.viewVersionPhase()
	}

	return ""
}

func (s *DistributionStep) viewLoadingPhase() string {
	loading := s.loadingSpinner.View() + " fetching available okd versions..."
	return lipgloss.NewStyle().
		Foreground(tui.ColorSlate400).
		Render(loading)
}

func (s *DistributionStep) viewVersionPhase() string {
	var content strings.Builder

	if s.loadError != nil {
		errMsg := lipgloss.NewStyle().
			Foreground(tui.ColorError).
			Bold(true).
			Render("✗ failed to fetch okd versions: " + s.loadError.Error())
		content.WriteString(errMsg)
		content.WriteString("\n\n")
		content.WriteString(lipgloss.NewStyle().
			Foreground(tui.ColorSlate500).
			Italic(true).
			Render("please check your network connection and try again."))
		content.WriteString("\n\n")
		return content.String()
	}

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

// ShortHelp returns the step's help bar, which differs by phase.
func (s *DistributionStep) ShortHelp() []wizard.KeyBinding {
	if s.phase == phaseVersionSelect {
		return []wizard.KeyBinding{
			{Key: "↑↓", Help: "select"},
			{Key: "tab", Help: "expand/collapse"},
			{Key: helpEnter, Help: helpConfirm},
			{Key: helpEsc, Help: helpBack},
		}
	}
	return []wizard.KeyBinding{
		{Key: helpEsc, Help: helpBack},
		{Key: helpCtrlC, Help: helpQuit},
	}
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

// DisplayTitle returns the header text for the step, suppressed while the
// release list is loading.
func (s *DistributionStep) DisplayTitle() string {
	switch s.phase {
	case phaseVersionLoading:
		return ""
	case phaseVersionSelect:
		return "which okd version would you like to deploy?"
	default:
		return s.BaseStep.DisplayTitle()
	}
}

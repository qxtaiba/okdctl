package steps

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/releases"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

// VersionFetcher resolves the OKD release catalog for the distribution step.
type VersionFetcher interface {
	FetchVersions(ctx context.Context) ([]releases.OKDReleaseSeries, error)
}

// StaticVersionFetcher returns a fixed catalog or error without the network.
type StaticVersionFetcher struct {
	Series []releases.OKDReleaseSeries
	Err    error
}

// FetchVersions returns a copy of Series, or Err when set.
func (f StaticVersionFetcher) FetchVersions(_ context.Context) ([]releases.OKDReleaseSeries, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	series := make([]releases.OKDReleaseSeries, len(f.Series))
	copy(series, f.Series)
	return series, nil
}

// helpRefresh is the shared help text for a binding that re-issues a loaded
// fetch.
const helpRefresh = "refresh"

type selectionPhase int

const (
	phaseVersionLoading selectionPhase = iota
	phaseVersionError
	phaseVersionSelect
)

// DistributionStep picks an OKD version from the fetched release catalog.
type DistributionStep struct {
	wizard.BaseStep
	versionSelector *components.Selector
	phase           selectionPhase
	selectedVersion string
	currentVersion  string // the version the loaded config already carries

	versionFetcher VersionFetcher
	okdSeries      []releases.OKDReleaseSeries
	expandedMinor  int // -1 = none expanded (show latest per minor)
	frame          uint64
	loadError      error

	// generation tags every fetch so a superseded reply is discarded unapplied
	generation uint64

	// contentHeight is the last genuine SetSize height; View re-runs with a
	// placeholder height on every viewport sync
	contentHeight int
}

// SetSize updates the step's size, remembering the real content height for
// the dropdown budget.
func (s *DistributionStep) SetSize(width, height int) {
	s.BaseStep.SetSize(width, height)
	s.contentHeight = height
}

// NewDistributionStep constructs the distribution step.
func NewDistributionStep() *DistributionStep {
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
	}
}

// Init starts the release fetch, reusing a loaded catalog on re-entry; a
// failed attempt always retries.
func (s *DistributionStep) Init() tea.Cmd {
	if s.phase == phaseVersionSelect {
		return nil
	}
	return s.startFetch()
}

// Animating reports whether the loading indicator needs frame ticks.
func (s *DistributionStep) Animating() bool {
	return s.phase == phaseVersionLoading
}

// Update handles version-load messages, frames and navigation keys.
func (s *DistributionStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case versionsLoadedMsg:
		if msg.generation != s.generation {
			return s, nil
		}
		s.okdSeries = msg.series
		s.loadError = msg.err
		if msg.err != nil {
			s.phase = phaseVersionError
			return s, nil
		}
		s.phase = phaseVersionSelect
		s.updateVersionSelector()
		s.anchorConfiguredVersion()
		s.versionSelector.SetFocused(true)
		return s, nil

	case wizard.FrameMsg:
		s.frame = msg.Frame

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
	// 'r' refreshes a loaded catalog, footer-silent since the error phase
	// already advertises the same key as retry.
	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		cmd := s.startFetch()
		return s, cmd
	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k", "down", "j"))):
		return s.handleNavigationKey(msg)
	}
	return s, nil
}

func (s *DistributionStep) handleErrorKeyMsg(msg tea.KeyPressMsg) (wizard.WizardStep, tea.Cmd) {
	if key.Matches(msg, key.NewBinding(key.WithKeys("r"))) {
		return s.retry()
	}
	return s, nil
}

func (s *DistributionStep) retry() (wizard.WizardStep, tea.Cmd) {
	cmd := s.startFetch()
	return s, cmd
}

// handleEnterKey confirms the highlighted release: a series row confirms its
// latest patch, a patch row itself; tab expands and collapses.
func (s *DistributionStep) handleEnterKey() (wizard.WizardStep, tea.Cmd) {
	selected := s.versionSelector.Selected()

	if selected.ID == "" {
		return s, func() tea.Msg { return wizard.ErrorSetMsg{Error: errors.New("pick a release first")} }
	}

	if strings.HasPrefix(selected.ID, "minor:") {
		minor := s.getMinorFromOptionID(selected.ID)
		for _, series := range s.okdSeries {
			if series.Minor == minor {
				s.selectedVersion = series.Latest.Version
				break
			}
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

// FocusedSpan reports the highlighted version's lines, dropdown rows
// included; the selector starts at line 0 of View.
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

// syncSelectedVersion mirrors SelectField.Value(): the latest patch for a
// collapsed minor row, else the cursor's exact ID.
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

// View renders the loading, error or selection phase.
func (s *DistributionStep) View(width, height int) string {
	s.BaseStep.SetSize(width, height)

	switch s.phase {
	case phaseVersionLoading:
		return s.viewLoadingPhase()
	case phaseVersionError:
		return s.viewErrorPhase(width)
	case phaseVersionSelect:
		s.applyDropdownBudget()
		return s.viewVersionPhase()
	}

	return ""
}

// applyDropdownBudget sizes the dropdown to the real content height around
// the collapsed rows, the expanded series' own row, the hint rows and the
// box borders; N patch rows cost 3N-1 lines.
func (s *DistributionStep) applyDropdownBudget() {
	if s.expandedMinor < 0 {
		s.versionSelector.SetDropdownBudget(0)
		return
	}

	expandedIdx := -1
	for i := range s.okdSeries {
		if s.okdSeries[i].Minor == s.expandedMinor {
			expandedIdx = i
			break
		}
	}
	if expandedIdx < 0 {
		s.versionSelector.SetDropdownBudget(0)
		return
	}

	before := expandedIdx
	after := len(s.okdSeries) - expandedIdx - 1
	rowLines := func(n int) int {
		lines := n * 2 // title + description per row
		if n > 1 {
			lines += n - 1 // a connector line between consecutive rows
		}
		return lines
	}

	// The expanded series' own row sits right above its dropdown, joined to the
	// collapsed row above by a connector; a synthetic current row adds three.
	above := rowLines(before) + 2
	if before > 0 {
		above++
	}
	if s.injectsCurrentVersion() {
		above += 3
	}
	below := rowLines(after)

	const (
		blankLine      = 1
		hintLines      = 2 // "showing patch versions..." + "press tab to collapse"
		dropdownChrome = 2 // top + bottom border; versionSelector never sets DropdownHeader
	)

	avail := s.contentHeight - above - below - blankLine - hintLines - dropdownChrome
	s.versionSelector.SetDropdownBudget((avail + 1) / 3)
}

func (s *DistributionStep) viewLoadingPhase() string {
	var content strings.Builder
	content.WriteString(lipgloss.NewStyle().
		Foreground(tui.ColorTextDim()).
		Render(wizard.Spinner(s.frame) + " fetching okd releases"))
	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().
		Foreground(tui.ColorTextFaint()).
		Italic(true).
		Render("this can take a few seconds"))
	return content.String()
}

// viewErrorPhase leaves the retry and back keys to the footer.
func (s *DistributionStep) viewErrorPhase(width int) string {
	var content strings.Builder
	content.WriteString(tui.EmptyState("no releases loaded — check your connection", ""))
	if s.loadError != nil {
		content.WriteString("\n\n")
		content.WriteString(lipgloss.NewStyle().
			Foreground(tui.ColorTextFaint()).
			Width(width - 2).
			Render("details: " + s.loadError.Error()))
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
				Foreground(tui.ColorSubtle()).
				Render(fmt.Sprintf("showing patch versions for 4.%d", s.expandedMinor)),
			lipgloss.NewStyle().
				Foreground(tui.ColorTextFaint()).
				Italic(true).
				Render("press tab to collapse"),
		)
	} else {
		hints = append(hints, lipgloss.NewStyle().
			Foreground(tui.ColorTextFaint()).
			Italic(true).
			Render("press tab to expand patch versions"))
	}

	content.WriteString(strings.Join(hints, "\n"))

	return content.String()
}

// Validate always returns nil; any offered version is valid.
func (s *DistributionStep) Validate() error {
	return nil
}

// Apply writes the selected OKD version into cfg.
func (s *DistributionStep) Apply(cfg *config.Config) error {
	cfg.Distribution.Type = config.DistributionOKD
	cfg.Distribution.Version = s.selectedVersion
	return nil
}

// ShortHelp returns the help bar for the current phase.
func (s *DistributionStep) ShortHelp() []wizard.KeyBinding {
	if s.phase == phaseVersionSelect {
		return []wizard.KeyBinding{
			{Key: "↑↓", Help: "select"},
			{Key: "tab", Help: "expand/collapse"},
			{Key: wizard.HelpEnter, Help: wizard.HelpConfirm},
			{Key: wizard.HelpEsc, Help: wizard.HelpBack},
			{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
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

// OverlayHelp lists the select phase's footer-silent "r" refresh; the error
// phase already advertises it in ShortHelp.
func (s *DistributionStep) OverlayHelp() []wizard.KeyBinding {
	if s.phase != phaseVersionSelect {
		return nil
	}
	return []wizard.KeyBinding{{Key: "r", Help: helpRefresh}}
}

// SetFocused focuses the selector only once the catalog has loaded.
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

// SetSelectedVersion pre-selects version as the loaded config's release;
// once the catalog is available the cursor anchors on its exact patch row.
func (s *DistributionStep) SetSelectedVersion(version string) {
	s.selectedVersion = version
	s.currentVersion = version
	if len(s.okdSeries) > 0 {
		s.updateVersionSelector()
		s.anchorConfiguredVersion()
		return
	}
	s.versionSelector.SetSelectedByID(version)
}

// anchorConfiguredVersion expands the series holding the configured version
// so the cursor lands on its patch row; a version outside the catalog is
// already the injected leading row.
func (s *DistributionStep) anchorConfiguredVersion() {
	if s.currentVersion == "" || strings.HasPrefix(s.currentVersion, "minor:") {
		return
	}
	for i := range s.okdSeries {
		for _, pv := range s.okdSeries[i].Versions {
			if pv.Version == s.currentVersion {
				s.expandedMinor = s.okdSeries[i].Minor
				s.updateVersionSelector()
				return
			}
		}
	}
}

// SetVersionFetcher swaps the release-catalog fetcher.
func (s *DistributionStep) SetVersionFetcher(f VersionFetcher) {
	s.versionFetcher = f
}

// FocusBounds keeps the selected release visible while navigating.
func (s *DistributionStep) FocusBounds(width, _ int) (top, bottom int, ok bool) {
	if s.phase != phaseVersionSelect || s.loadError != nil {
		return 0, 0, false
	}
	return s.versionSelector.FocusBounds(width)
}

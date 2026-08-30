package steps

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/releases"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

type versionsLoadedMsg struct {
	series []releases.OKDReleaseSeries
	err    error
}

func (s *DistributionStep) fetchVersions() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	series, err := s.versionFetcher.FetchVersions(ctx)
	return versionsLoadedMsg{series: series, err: err}
}

func (s *DistributionStep) updateVersionSelector() {
	var options []components.Option

	for i := range s.okdSeries {
		series := &s.okdSeries[i]
		v := series.Latest
		if v.Version == "" {
			continue
		}

		isExpanded := s.expandedMinor == series.Minor

		var desc string
		if isExpanded {
			desc = fmt.Sprintf("▼ %d patch versions available", len(series.Versions))
		} else {
			desc = fmt.Sprintf("▶ %s (tab to expand)", s.getSeriesDescription(series))
		}

		options = append(options, components.Option{
			ID:          fmt.Sprintf("minor:%d.%d", series.Major, series.Minor),
			Title:       fmt.Sprintf("okd %d.%d", series.Major, series.Minor),
			Description: desc,
			Recommended: i == 0,
			Style:       releaseTypeToOptionStyle(v.Type),
		})

		if isExpanded {
			for j, pv := range series.Versions {
				patchDesc := fmt.Sprintf("released: %s", pv.ReleaseDate.Format("Jan 2006"))
				if !pv.Stable {
					patchDesc = "preview release - not recommended for production"
				}

				options = append(options, components.Option{
					ID:          pv.Version,
					Title:       "  " + pv.DisplayName(),
					Description: patchDesc,
					Recommended: j == 0 && i == 0,
					Style:       releaseTypeToOptionStyle(pv.Type),
					InDropdown:  true,
				})
			}
		}
	}

	s.versionSelector.SetOptions(options)

	if s.selectedVersion == "" && len(s.okdSeries) > 0 {
		s.selectedVersion = fmt.Sprintf("minor:%d.%d", s.okdSeries[0].Major, s.okdSeries[0].Minor)
	}
	if len(s.okdSeries) == 0 {
		s.selectedVersion = ""
	}
	s.versionSelector.SetSelectedByID(s.selectedVersion)
}

func (s *DistributionStep) emitFocusChanged() tea.Cmd {
	selected := s.versionSelector.Selected()

	if selected.InDropdown {
		return nil
	}

	index := s.versionSelector.SelectedIndex()
	total := s.countVersionOptions()

	if total == 0 {
		return nil
	}

	return func() tea.Msg {
		return wizard.FocusChangedMsg{
			FieldIndex:  index,
			TotalFields: total,
		}
	}
}

func (s *DistributionStep) countVersionOptions() int {
	count := len(s.okdSeries)
	if s.expandedMinor >= 0 {
		for _, series := range s.okdSeries {
			if series.Minor == s.expandedMinor {
				count += len(series.Versions)
				break
			}
		}
	}
	return count
}

func (s *DistributionStep) getMinorFromOptionID(id string) int {
	var major, minor int
	if _, err := fmt.Sscanf(id, "minor:%d.%d", &major, &minor); err == nil {
		return minor
	}

	if _, err := fmt.Sscanf(id, "%d.%d", &major, &minor); err == nil {
		return minor
	}

	return -1
}

func releaseTypeToOptionStyle(rt releases.ReleaseType) components.OptionStyle {
	switch rt {
	case releases.ReleaseTypeLatestStable:
		return components.OptionStyleLatestStable
	case releases.ReleaseTypeStable:
		return components.OptionStyleStable
	case releases.ReleaseTypePreview:
		return components.OptionStylePreview
	case releases.ReleaseTypeLatestPreview:
		return components.OptionStyleLatestPreview
	case releases.ReleaseTypeLTS:
		return components.OptionStyleLTS
	default:
		return components.OptionStyleDefault
	}
}

func (s *DistributionStep) getSeriesDescription(series *releases.OKDReleaseSeries) string {
	v := series.Latest

	if len(s.okdSeries) == 0 {
		return fmt.Sprintf("stable (%s)", v.Version)
	}

	if series.Major == s.okdSeries[0].Major && series.Minor == s.okdSeries[0].Minor {
		return fmt.Sprintf("latest stable (%s)", v.Version)
	}

	if series.Minor <= s.okdSeries[0].Minor-2 {
		return fmt.Sprintf("lts candidate (%s)", v.Version)
	}

	return fmt.Sprintf("stable (%s)", v.Version)
}

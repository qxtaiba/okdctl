package wizard

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// ProgressInfo is the header's view of wizard progress, passed to a
// FlowChrome.Trail hook in place of the default dot ribbon.
type ProgressInfo struct {
	Current, Total int
	CurrentID      StepID
	Titles         []string
}

// FlowChrome parameterizes per-flow header chrome — tagline, an optional
// context badge on the footer divider, and a pluggable progress trail;
// Badge and Trail may be nil.
type FlowChrome struct {
	Tagline string
	Badge   func(cfg *config.Config) string
	// Trail renders the header's right-hand progress indicator; nil falls
	// back to the dot ribbon + "step N of M".
	Trail func(p ProgressInfo) string
}

// DefaultChrome returns the configure wizard's chrome.
func DefaultChrome() FlowChrome {
	return FlowChrome{Tagline: "okd over proxmox, the easy way", Badge: distributionBadge}
}

// Stage groups the StepIDs shown under one header-trail crumb label.
type Stage struct {
	Label string
	Steps []StepID
}

var (
	stageLabelStyle        = lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	stageLabelCurrentStyle = lipgloss.NewStyle().Bold(true).Foreground(tui.ColorText)
	stageSeparatorStyle    = lipgloss.NewStyle().Foreground(tui.ColorSlate600)
)

// StagesTrail returns a FlowChrome.Trail hook that renders stages as
// dot-separated crumbs, bolding whichever stage contains p.CurrentID; a
// CurrentID absent from every stage renders all stages dim.
func StagesTrail(stages []Stage) func(p ProgressInfo) string {
	return func(p ProgressInfo) string {
		current := -1
		for i, stage := range stages {
			for _, id := range stage.Steps {
				if id == p.CurrentID {
					current = i
				}
			}
		}

		parts := make([]string, len(stages))
		for i, stage := range stages {
			if i == current {
				parts[i] = stageLabelCurrentStyle.Render(stage.Label)
			} else {
				parts[i] = stageLabelStyle.Render(stage.Label)
			}
		}
		return strings.Join(parts, stageSeparatorStyle.Render(" · "))
	}
}

// PinnedFooter is implemented by steps that render their own full-width
// footer instead of the wizard's default help bar.
type PinnedFooter interface {
	PinnedFooter(width int) string
}

func distributionBadge(cfg *config.Config) string {
	if cfg.Distribution.Type == "" {
		return ""
	}
	badge := string(cfg.Distribution.Type)
	if cfg.Distribution.Version != "" {
		badge += " " + cfg.Distribution.Version
	}
	return badge
}

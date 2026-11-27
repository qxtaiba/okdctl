package wizard

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// ProgressInfo is the header's view of wizard progress for a Trail hook.
type ProgressInfo struct {
	Current, Total int
	CurrentID      StepID
	// VisibleIDs lists the visible steps in flow order
	VisibleIDs []StepID
}

// FlowChrome parameterizes per-flow header chrome; Badge and Trail may be nil.
type FlowChrome struct {
	Tagline string
	Badge   func(cfg *config.Config) string
	// Trail renders the progress indicator; nil uses the dot ribbon
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
	stageLabelStyle        lipgloss.Style
	stageLabelCurrentStyle lipgloss.Style
	stageSeparatorStyle    lipgloss.Style
)

// StagesTrail returns a Trail hook rendering stages as dot-separated crumbs,
// bolding the one holding p.CurrentID with an inline dot ribbon once it has
// two or more visible steps.
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
				parts[i] = renderActiveStage(stage, &p)
			} else {
				parts[i] = stageLabelStyle.Render(stage.Label)
			}
		}
		return strings.Join(parts, stageSeparatorStyle.Render(" · "))
	}
}

func renderActiveStage(stage Stage, p *ProgressInfo) string {
	label := stageLabelCurrentStyle.Render(stage.Label)

	inStage := make(map[StepID]bool, len(stage.Steps))
	for _, id := range stage.Steps {
		inStage[id] = true
	}

	position, total := -1, 0
	for _, id := range p.VisibleIDs {
		if !inStage[id] {
			continue
		}
		if id == p.CurrentID {
			position = total
		}
		total++
	}

	if total < 2 || position < 0 {
		return label
	}
	return label + " " + stageDots(position, total)
}

func stageDots(position, total int) string {
	dots := make([]string, total)
	for i := range total {
		switch {
		case i < position:
			dots[i] = StepDotCompletedStyle.Render(tui.IconSuccess)
		case i == position:
			dots[i] = StepDotCurrentStyle.Render(tui.IconActive)
		default:
			dots[i] = StepDotPendingStyle.Render(tui.IconPending)
		}
	}
	return strings.Join(dots, "")
}

// PinnedFooter is implemented by steps that render their own footer row.
type PinnedFooter interface {
	PinnedFooter(width int) string
}

// BadgeSuppressor is implemented by steps for which the context badge would
// mislead, such as the blank-slate hub.
type BadgeSuppressor interface {
	SuppressesBadge() bool
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

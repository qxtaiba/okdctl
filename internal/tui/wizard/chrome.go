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
	// VisibleIDs lists every currently visible step's ID in flow order, so a
	// Trail hook can work out a step's position within a subset (e.g. a
	// Stage) without knowing which steps a given config hides.
	VisibleIDs []StepID
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

// The stage-trail styles are assigned by rebuildWizardStyles so the
// background flip reaches them.
var (
	stageLabelStyle        lipgloss.Style
	stageLabelCurrentStyle lipgloss.Style
	stageSeparatorStyle    lipgloss.Style
)

// StagesTrail returns a FlowChrome.Trail hook that renders stages as
// dot-separated crumbs, bolding whichever stage contains p.CurrentID and
// appending that stage's inline dot ribbon once it has 2 or more visible
// steps; a CurrentID absent from every stage renders all stages dim.
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

// renderActiveStage renders stage's bolded label, appending an inline dot
// ribbon tracking p.CurrentID's position among stage's visible steps once
// there are 2 or more of them.
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

// stageDots renders total dots in RenderStepProgress's ✓/●/○ vocabulary,
// joined with no connector since the ribbon sits inline with its stage's
// label.
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

// PinnedFooter is implemented by steps that render their own full-width
// footer instead of the wizard's default help bar.
type PinnedFooter interface {
	PinnedFooter(width int) string
}

// BadgeSuppressor is implemented by steps that can declare the chrome's
// context badge misleading right now — the blank-slate hub, where a
// defaults-seed version would advertise a cluster that doesn't exist.
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
